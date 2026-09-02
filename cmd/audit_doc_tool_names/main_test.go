// Package main tests the documentation tool-name auditor: the in-memory name
// registry built from every surface, the per-file token scan with its
// exemptions, the root walk, the report ordering and exit codes, and the
// gate itself against the repository's own documentation.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// realRegistry memoizes the registered name set: building it registers the
// individual, meta and dynamic surfaces (~15s), and the set only depends on
// the compiled-in catalog, so one build serves every test in the package.
var realRegistry struct {
	once  sync.Once
	names map[string]struct{}
	err   error
}

// registeredNames returns the memoized registered name set.
func registeredNames(t *testing.T) map[string]struct{} {
	t.Helper()
	realRegistry.once.Do(func() {
		realRegistry.names, realRegistry.err = registeredToolNames()
	})
	if realRegistry.err != nil {
		t.Fatalf("registeredToolNames: %v", realRegistry.err)
	}
	return realRegistry.names
}

// writeDoc writes one documentation file under root at the slash-separated
// relative path rel, creating parent directories, and returns its path.
func writeDoc(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// danglingLink creates a symlink at rel under root that points nowhere, so a
// read of it fails while the walk still sees a file with that name.
func danglingLink(t *testing.T, root, rel string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.Symlink(filepath.Join(root, "missing-target.md"), path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return path
}

// stubRegistry is the small registered set the scan tests use in place of the
// real surfaces, so each case states which names count as registered.
var stubRegistry = map[string]struct{}{
	"gitlab_issue_list":   {},
	"gitlab_issue":        {},
	"gitlab_find_action":  {},
	"gitlab_orbit_status": {},
}

// TestRegisteredToolNames_AllSurfaces_UnionOfRegisteredNames verifies the
// registry is the union of what the three surfaces advertise: the dynamic
// pair, the meta domain tools and standalone tools, the individual
// domain-first and legacy verb-first names, and the server tools that
// cmd/server registers outside the catalog. The verb-first
// gitlab_list_issues that survived in the docs for so long must be absent,
// because no surface has ever registered it.
func TestRegisteredToolNames_AllSurfaces_UnionOfRegisteredNames(t *testing.T) {
	names := registeredNames(t)
	if len(names) < 1000 {
		t.Fatalf("registered %d names, want the whole individual surface (over 1000)", len(names))
	}
	cases := []struct {
		name       string
		tool       string
		registered bool
	}{
		{name: "dynamic_find", tool: "gitlab_find_action", registered: true},
		{name: "dynamic_execute", tool: "gitlab_execute_action", registered: true},
		{name: "meta_domain", tool: "gitlab_issue", registered: true},
		{name: "meta_standalone", tool: "gitlab_discover_project", registered: true},
		{name: "meta_server", tool: "gitlab_server", registered: true},
		{name: "individual_domain_first", tool: "gitlab_issue_list", registered: true},
		{name: "individual_verb_first_legacy", tool: "gitlab_list_issue_discussions", registered: true},
		{name: "individual_server_tool", tool: "gitlab_server_status", registered: true},
		{name: "never_registered_verb_first", tool: "gitlab_list_issues", registered: false},
		{name: "never_registered_typo", tool: "gitlab_issues_list", registered: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := names[tc.tool]; ok != tc.registered {
				t.Errorf("registered[%s] = %v, want %v", tc.tool, ok, tc.registered)
			}
		})
	}
}

// TestScanFile_Tokens_ReportsOnlyUnregisteredNames verifies the per-file
// scan: a registered name, an allowed non-tool token, an exempted family
// prefix and a wildcard-truncated prefix are all ignored, an unregistered
// name is recorded under the file once however often it appears, and a file
// with no tool-shaped token still counts as scanned.
func TestScanFile_Tokens_ReportsOnlyUnregisteredNames(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "no_tokens", content: "# Guide\n\nNothing to see.\n", want: nil},
		{name: "registered_name", content: "Call `gitlab_issue_list` to list issues.", want: nil},
		{name: "allowed_config_key", content: "Set gitlab_url in user_config.", want: nil},
		{name: "allowed_family_prefix", content: "gitlab_orbit_query needs GitLab.com.", want: nil},
		{name: "wildcard_prefix", content: "The gitlab_mr_approval_ tools.", want: nil},
		{name: "unregistered_name", content: "Use gitlab_list_issues here.", want: []string{"gitlab_list_issues"}},
		{
			name:    "repeated_mention_counted_once",
			content: "gitlab_list_issues and again gitlab_list_issues",
			want:    []string{"gitlab_list_issues"},
		},
		{
			name:    "mixed_content",
			content: "gitlab_issue, gitlab_find_action, gitlab_get_issue and gitlab_list_issues",
			want:    []string{"gitlab_get_issue", "gitlab_list_issues"},
		},
		{name: "uppercase_is_not_a_token", content: "GITLAB_LIST_ISSUES is an env var", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeDoc(t, t.TempDir(), "guide.md", tc.content)
			findings := map[string][]string{}
			n, err := scanFile(path, stubRegistry, findings)
			if err != nil {
				t.Fatalf("scanFile: %v", err)
			}
			if n != 1 {
				t.Errorf("scanned = %d, want 1", n)
			}
			if len(findings) != len(tc.want) {
				t.Fatalf("findings = %v, want names %v", findings, tc.want)
			}
			for _, name := range tc.want {
				files := findings[name]
				if len(files) != 1 || files[0] != filepath.ToSlash(path) {
					t.Errorf("findings[%s] = %v, want [%s]", name, files, filepath.ToSlash(path))
				}
			}
		})
	}
}

// TestScanFile_HistoricalDocs_SkippedWithoutReading verifies a file under a
// historical tree (an ADR) is neither scanned nor counted even when it names
// a tool that no longer exists, because the record must keep its examples.
func TestScanFile_HistoricalDocs_SkippedWithoutReading(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeDoc(t, root, "docs/development/adr/adr-0001-example.md", "Decided on gitlab_list_issues.")

	findings := map[string][]string{}
	n, err := scanFile("docs/development/adr/adr-0001-example.md", stubRegistry, findings)
	if err != nil {
		t.Fatalf("scanFile: %v", err)
	}
	if n != 0 || len(findings) != 0 {
		t.Errorf("scanFile on a historical doc = (%d, %v), want (0, none)", n, findings)
	}
}

// TestScanFile_UnreadableFile_ReturnsError verifies a file the walk can name
// but not read surfaces as an error instead of a silently clean scan.
func TestScanFile_UnreadableFile_ReturnsError(t *testing.T) {
	path := danglingLink(t, t.TempDir(), "docs/broken.md")
	findings := map[string][]string{}
	if _, err := scanFile(path, stubRegistry, findings); err == nil {
		t.Fatal("scanFile on a dangling symlink returned nil error")
	}
}

// TestScanRoot_Roots_ScansFilesAndTrees verifies one docRoots entry may be
// a single file or a tree: a missing root is skipped, a tree yields every
// .md and .mdx file below it while other extensions and node_modules are
// ignored, and a root that cannot be stated or a file that cannot be read
// fails the scan.
func TestScanRoot_Roots_ScansFilesAndTrees(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(t *testing.T, root string) string
		wantScanned int
		wantNames   []string
		wantErr     bool
	}{
		{
			name:        "missing_root_is_skipped",
			setup:       func(_ *testing.T, root string) string { return filepath.Join(root, "absent") },
			wantScanned: 0,
		},
		{
			name: "single_file_root",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				return writeDoc(t, root, "README.md", "See gitlab_list_issues.")
			},
			wantScanned: 1,
			wantNames:   []string{"gitlab_list_issues"},
		},
		{
			name: "tree_root_walks_markdown_only",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				writeDoc(t, root, "docs/a.md", "gitlab_list_issues")
				writeDoc(t, root, "docs/nested/b.mdx", "gitlab_get_issue")
				writeDoc(t, root, "docs/nested/notes.txt", "gitlab_ignored_in_txt")
				writeDoc(t, root, "docs/node_modules/pkg/README.md", "gitlab_ignored_in_node_modules")
				return filepath.Join(root, "docs")
			},
			wantScanned: 2,
			wantNames:   []string{"gitlab_list_issues", "gitlab_get_issue"},
		},
		{
			name: "root_below_a_file_fails_stat",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				file := writeDoc(t, root, "README.md", "x")
				return filepath.Join(file, "child")
			},
			wantErr: true,
		},
		{
			name: "unreadable_file_in_tree_fails",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				writeDoc(t, root, "docs/ok.md", "clean")
				danglingLink(t, root, "docs/zz-broken.md")
				return filepath.Join(root, "docs")
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.setup(t, t.TempDir())
			findings := map[string][]string{}
			n, err := scanRoot(root, stubRegistry, findings)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("scanRoot(%s) returned nil error", root)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanRoot(%s): %v", root, err)
			}
			if n != tc.wantScanned {
				t.Errorf("scanned = %d, want %d", n, tc.wantScanned)
			}
			if len(findings) != len(tc.wantNames) {
				t.Fatalf("findings = %v, want names %v", findings, tc.wantNames)
			}
			for _, name := range tc.wantNames {
				if len(findings[name]) != 1 {
					t.Errorf("findings[%s] = %v, want exactly one file", name, findings[name])
				}
			}
		})
	}
}

// TestScanDocs_Roots_AggregatesAcrossRoots verifies the top-level scan sums
// the files of every root and merges the files naming one unregistered tool
// into a single list, which is what the report groups by.
func TestScanDocs_Roots_AggregatesAcrossRoots(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "docs/guide.md", "gitlab_list_issues")
	writeDoc(t, root, "docs/other.md", "gitlab_issue_list only")
	readme := writeDoc(t, root, "README.md", "gitlab_list_issues and gitlab_get_issue")

	findings, scanned, err := scanDocs([]string{filepath.Join(root, "docs"), readme, filepath.Join(root, "absent.md")}, stubRegistry)
	if err != nil {
		t.Fatalf("scanDocs: %v", err)
	}
	if scanned != 3 {
		t.Errorf("scanned = %d, want 3", scanned)
	}
	if got := findings["gitlab_list_issues"]; len(got) != 2 {
		t.Errorf("gitlab_list_issues files = %v, want two", got)
	}
	if got := findings["gitlab_get_issue"]; len(got) != 1 || got[0] != filepath.ToSlash(readme) {
		t.Errorf("gitlab_get_issue files = %v, want [%s]", got, filepath.ToSlash(readme))
	}
}

// TestRun_Findings_ReportsSortedAndReturnsExitCode verifies the command's
// contract: a clean tree prints the all-clear and exits 0, findings are
// listed most-referenced first and then alphabetically with their files
// sorted, -check turns findings into an error on stderr and exit 1, and a
// registry or scan failure exits 1 with its cause.
func TestRun_Findings_ReportsSortedAndReturnsExitCode(t *testing.T) {
	okRegistry := func() (map[string]struct{}, error) { return stubRegistry, nil }
	cases := []struct {
		name     string
		setup    func(t *testing.T, root string) []string
		collect  func() (map[string]struct{}, error)
		check    bool
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name: "clean_docs",
			setup: func(t *testing.T, root string) []string {
				t.Helper()
				return []string{writeDoc(t, root, "docs/a.md", "gitlab_issue_list")}
			},
			collect:  okRegistry,
			wantCode: 0,
			wantOut:  "audit_doc_tool_names: 4 registered tool names, 1 documentation files scanned\nno documentation names an unregistered tool\n",
		},
		{
			name: "findings_without_check",
			setup: func(t *testing.T, root string) []string {
				t.Helper()
				writeDoc(t, root, "docs/b.md", "gitlab_list_issues gitlab_zeta")
				writeDoc(t, root, "docs/a.md", "gitlab_list_issues gitlab_alpha")
				return []string{filepath.Join(root, "docs")}
			},
			collect:  okRegistry,
			wantCode: 0,
			wantOut: "\n3 unregistered tool name(s) referenced:\n" +
				"  gitlab_list_issues                     2 file(s)\n" +
				"      ROOT/docs/a.md\n" +
				"      ROOT/docs/b.md\n" +
				"  gitlab_alpha                           1 file(s)\n" +
				"      ROOT/docs/a.md\n" +
				"  gitlab_zeta                            1 file(s)\n" +
				"      ROOT/docs/b.md\n",
		},
		{
			name: "findings_with_check",
			setup: func(t *testing.T, root string) []string {
				t.Helper()
				return []string{writeDoc(t, root, "docs/a.md", "gitlab_list_issues")}
			},
			collect:  okRegistry,
			check:    true,
			wantCode: 1,
			wantOut:  "  gitlab_list_issues                     1 file(s)\n",
			wantErr:  "\nERROR: the documentation names 1 tool(s) the server does not register\n",
		},
		{
			name:     "registry_failure",
			setup:    func(_ *testing.T, _ string) []string { return nil },
			collect:  func() (map[string]struct{}, error) { return nil, errors.New("boom") },
			wantCode: 1,
			wantErr:  "collect tool names: boom\n",
		},
		{
			name: "scan_failure",
			setup: func(t *testing.T, root string) []string {
				t.Helper()
				writeDoc(t, root, "docs/ok.md", "clean")
				danglingLink(t, root, "docs/zz-broken.md")
				return []string{filepath.Join(root, "docs")}
			},
			collect:  okRegistry,
			wantCode: 1,
			wantErr:  "scan docs: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			roots := tc.setup(t, root)
			var out, errOut bytes.Buffer
			got := run(tc.check, roots, tc.collect, &out, &errOut)
			if got != tc.wantCode {
				t.Errorf("run returned %d, want %d\nstdout:\n%s\nstderr:\n%s", got, tc.wantCode, out.String(), errOut.String())
			}
			wantOut := strings.ReplaceAll(tc.wantOut, "ROOT", filepath.ToSlash(root))
			if !strings.Contains(out.String(), wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), wantOut)
			}
			if !strings.Contains(errOut.String(), tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tc.wantErr)
			}
			if tc.wantErr == "" && errOut.Len() != 0 {
				t.Errorf("stderr should stay empty, got %q", errOut.String())
			}
		})
	}
}

// TestRun_RepositoryDocs_NameOnlyRegisteredTools is the gate: every
// documentation root the command audits, scanned from the repository root
// against the real registered name set under -check, must exit 0. A tool name
// that no surface registers fails here before it fails a reader at runtime.
func TestRun_RepositoryDocs_NameOnlyRegisteredTools(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	names := registeredNames(t)
	t.Chdir(root)

	var out, errOut bytes.Buffer
	got := run(true, docRoots, func() (map[string]struct{}, error) { return names, nil }, &out, &errOut)
	if got != 0 {
		t.Fatalf("run(-check) on the repository docs = %d, want 0\nstdout:\n%s\nstderr:\n%s", got, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "no documentation names an unregistered tool") {
		t.Errorf("stdout = %q, want the all-clear line", out.String())
	}
	if !strings.Contains(out.String(), "documentation files scanned") || strings.Contains(out.String(), " 0 documentation files scanned") {
		t.Errorf("stdout = %q, want a non-zero scanned file count", out.String())
	}
}
