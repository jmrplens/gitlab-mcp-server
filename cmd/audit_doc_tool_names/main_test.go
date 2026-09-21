// Package main tests the documentation tool-name auditor: the in-memory name
// registry built from every surface, the per-file token scan with its
// exemptions, the root walk, the report ordering and exit codes, and the
// gate itself against the repository's own documentation.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// realRegistry memoizes the registered name set: building it registers the
// individual, meta and dynamic surfaces (~15s), and the set only depends on
// the compiled-in catalog, so one build serves every test in the package.
var realRegistry struct {
	once  sync.Once
	names map[string]struct{}
}

// registeredNames returns the memoized registered name set.
func registeredNames(t *testing.T) map[string]struct{} {
	t.Helper()
	realRegistry.once.Do(func() {
		realRegistry.names = registeredToolNames()
	})
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

// stubIDs is the small catalog the scan tests judge dotted tokens against, so
// a case states which IDs exist rather than depending on what the tree
// happens to publish today.
func stubIDs() *actionids.IDs {
	return actionids.New(
		[]string{"issue.list", "issue.get", "issue.update", "project.get"},
		map[string]string{"project.fetch": "project.get"},
	)
}

// newStubScan is a scan over the two small sets above.
func newStubScan() *docScan { return newDocScan(stubRegistry, stubIDs()) }

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
			scan := newStubScan()
			if err := scan.scanFile(path); err != nil {
				t.Fatalf("scanFile: %v", err)
			}
			if scan.files != 1 {
				t.Errorf("scanned = %d, want 1", scan.files)
			}
			if len(scan.tools) != len(tc.want) {
				t.Fatalf("findings = %v, want names %v", scan.tools, tc.want)
			}
			for _, name := range tc.want {
				files := scan.tools[name]
				if len(files) != 1 || files[0] != filepath.ToSlash(path) {
					t.Errorf("findings[%s] = %v, want [%s]", name, files, filepath.ToSlash(path))
				}
			}
		})
	}
}

// TestScanFile_DottedTokens_ReportsOnlyIDsTheCatalogLacks verifies the other
// half of the same read: which dotted tokens a page offers as action IDs, and
// which of the four things that are not one each shape is.
//
// The two file-name cases are the ones that decide whether this rule is usable
// at all. Every documentation page writes file names, and the candidate test
// admits one whenever its stem is a catalog domain, so without the tail rule
// the report was 75 tokens of which about fifty were issue.rb and project.svg.
func TestScanFile_DottedTokens_ReportsOnlyIDsTheCatalogLacks(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "canonical_id", content: "Call `issue.list` first.", want: nil},
		{name: "neither_half_known", content: "See github.com and go.mod.", want: nil},
		{name: "file_name_under_a_domain", content: "Edit `issue.rb` and `project.svg`.", want: nil},
		{name: "meta_surface_entry", content: "Read `gitlab://tools/gitlab_issue.get`.", want: nil},
		{name: "declared_exception", content: "It emits `user.id` and `user.name`.", want: nil},
		{name: "family_prefix", content: "The `issue.work_item_*` actions.", want: nil},
		{name: "invented_domain", content: "Call `work_item.get` for one.", want: []string{"work_item.get"}},
		{name: "invented_action", content: "Call `issue.listt` for many.", want: []string{"issue.listt"}},
		{name: "registered_alias", content: "Call `project.fetch` to read one.", want: []string{"project.fetch"}},
		{
			name:    "repeated_mention_counted_once_per_file",
			content: "`work_item.get` and again `work_item.get`",
			want:    []string{"work_item.get"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeDoc(t, t.TempDir(), "guide.md", tc.content)
			scan := newStubScan()
			if err := scan.scanFile(path); err != nil {
				t.Fatalf("scanFile: %v", err)
			}
			if len(scan.actions) != len(tc.want) {
				t.Fatalf("action findings = %v, want %v", scan.actions, tc.want)
			}
			for _, token := range tc.want {
				finding, found := scan.actions[token]
				if !found {
					t.Fatalf("action findings = %v, want %q named", scan.actions, token)
				}
				if len(finding.Files) != 1 || finding.Files[0] != filepath.ToSlash(path) {
					t.Errorf("findings[%s].Files = %v, want [%s]", token, finding.Files, filepath.ToSlash(path))
				}
			}
		})
	}
}

// TestScanFile_RegisteredAlias_IsNamedAsAnAlias holds the half of a finding
// that decides what the fix is. An alias resolves when a model follows it and
// appears in no listing, so the row has to say what it stands for rather than
// read as a dead end.
func TestScanFile_RegisteredAlias_IsNamedAsAnAlias(t *testing.T) {
	path := writeDoc(t, t.TempDir(), "guide.md", "Call `project.fetch`.")
	scan := newStubScan()
	if err := scan.scanFile(path); err != nil {
		t.Fatalf("scanFile: %v", err)
	}

	finding := scan.actions["project.fetch"]
	if finding.Canonical != "project.get" {
		t.Errorf("Canonical = %q, want project.get", finding.Canonical)
	}
	if got := finding.describe("project.fetch"); !strings.Contains(got, "registered alias of project.get") {
		t.Errorf("describe = %q, want the canonical ID named", got)
	}
	dead := idFinding{}
	if got := dead.describe("issue.gone"); got != "issue.gone names no action" {
		t.Errorf("describe of a dead ID = %q", got)
	}
}

// TestScanFile_HistoricalDocs_SkippedWithoutReading verifies a file under a
// historical tree (an ADR) is neither scanned nor counted even when it names
// a tool that no longer exists, because the record must keep its examples.
func TestScanFile_HistoricalDocs_SkippedWithoutReading(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeDoc(t, root, "docs/development/adr/adr-0001-example.md", "Decided on gitlab_list_issues.")

	scan := newStubScan()
	if err := scan.scanFile("docs/development/adr/adr-0001-example.md"); err != nil {
		t.Fatalf("scanFile: %v", err)
	}
	if scan.files != 0 || len(scan.tools) != 0 {
		t.Errorf("scanFile on a historical doc = (%d, %v), want (0, none)", scan.files, scan.tools)
	}
}

// TestScanFile_UnreadableFile_ReturnsError verifies a file the walk can name
// but not read surfaces as an error instead of a silently clean scan.
func TestScanFile_UnreadableFile_ReturnsError(t *testing.T) {
	path := danglingLink(t, t.TempDir(), "docs/broken.md")
	if err := newStubScan().scanFile(path); err == nil {
		t.Fatal("scanFile on a dangling symlink returned nil error")
	}
}

// rootBelowAFile returns a root path that sits below a regular file, so
// stat fails for a reason other than absence.
//
// Not on Windows: a path below a regular file is reported as not found
// there, which scanRoot rightly treats as an absent root, so the stat failure
// this case exercises cannot be produced by this construction.
func rootBelowAFile(t *testing.T, root string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a path below a regular file reports not-found on Windows")
	}
	file := writeDoc(t, root, "README.md", "x")
	return filepath.Join(file, "child")
}

// TestScanRoot_Roots_ScansFilesAndTrees verifies one docRoots entry may be
// a single file or a tree: a missing root is skipped, a tree yields every
// .md and .mdx file below it while other extensions, node_modules and
// everything sourcewalk prunes are ignored, and a root that cannot be stated
// or a file that cannot be read fails the scan.
//
// The pruning case plants both halves of that rule, because the walk asks for
// both: a dot-directory is excluded by its name, and a directory holding a
// .git is another checkout whatever it is called. A worktree of this
// repository under docs/ would otherwise have its pages read as ours and its
// tool names judged against a catalog built from this branch.
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
			name: "tree_root_prunes_dot_directories_and_nested_checkouts",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				writeDoc(t, root, "docs/kept.md", "gitlab_list_issues")
				writeDoc(t, root, "docs/.cache/hidden.md", "gitlab_hidden_in_a_dot_directory")
				writeDoc(t, root, "docs/worktree/.git", "gitdir: /elsewhere/.git/worktrees/one")
				writeDoc(t, root, "docs/worktree/copy.md", "gitlab_hidden_in_a_nested_checkout")
				return filepath.Join(root, "docs")
			},
			wantScanned: 1,
			wantNames:   []string{"gitlab_list_issues"},
		},
		{
			name:    "root_below_a_file_fails_stat",
			setup:   rootBelowAFile,
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
			scan := newStubScan()
			err := scan.scanRoot(root)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("scanRoot(%s) returned nil error", root)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanRoot(%s): %v", root, err)
			}
			if scan.files != tc.wantScanned {
				t.Errorf("scanned = %d, want %d", scan.files, tc.wantScanned)
			}
			if len(scan.tools) != len(tc.wantNames) {
				t.Fatalf("findings = %v, want names %v", scan.tools, tc.wantNames)
			}
			for _, name := range tc.wantNames {
				if len(scan.tools[name]) != 1 {
					t.Errorf("findings[%s] = %v, want exactly one file", name, scan.tools[name])
				}
			}
		})
	}
}

// TestScanRoot_ADirectoryThatCannotBeListed_FailsRatherThanBeingSkipped
// verifies a subtree the walk cannot list stops the scan instead of being read
// as a subtree that holds nothing.
//
// It is the walk's own error argument rather than a file that would not read,
// and the difference is what the case is for: everything else here fails
// through scanFile, so nothing held the arm that answers a directory. What it
// protects is the claim the command makes when it exits 0, which is that it
// read the documentation and not that it read what it could.
//
// Directory permissions are a POSIX mechanism a sufficiently privileged
// process ignores, so the case skips when the chmod did not in fact deny the
// read.
func TestScanRoot_ADirectoryThatCannotBeListed_FailsRatherThanBeingSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory read permission is not a Windows file mode")
	}
	root := t.TempDir()
	writeDoc(t, root, "docs/kept.md", "clean")
	closed := filepath.Join(root, "docs", "closed")
	if err := os.Mkdir(closed, 0o750); err != nil {
		t.Fatalf("creating the directory: %v", err)
	}
	if err := os.Chmod(closed, 0); err != nil {
		t.Skipf("this platform does not let the test close a directory: %v", err)
	}
	// Cleanups run in reverse, so this one gives the search bit back before
	// t.TempDir's removal, which cannot descend into a closed directory.
	t.Cleanup(func() { _ = os.Chmod(closed, 0o700) }) //#nosec G302 -- a directory needs its search bit back or t.TempDir cannot remove it
	if _, err := os.ReadDir(closed); err == nil {
		t.Skip("this process lists a directory with no permissions on it, so the walk cannot be made to fail")
	}

	err := newStubScan().scanRoot(filepath.Join(root, "docs"))
	if err == nil {
		t.Fatal("scanRoot walked past a directory it could not list")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("scanRoot error = %v, want the permission failure the walk reported", err)
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

	scan := newStubScan()
	if err := scan.scanDocs([]string{filepath.Join(root, "docs"), readme, filepath.Join(root, "absent.md")}); err != nil {
		t.Fatalf("scanDocs: %v", err)
	}
	if scan.files != 3 {
		t.Errorf("scanned = %d, want 3", scan.files)
	}
	if got := scan.tools["gitlab_list_issues"]; len(got) != 2 {
		t.Errorf("gitlab_list_issues files = %v, want two", got)
	}
	if got := scan.tools["gitlab_get_issue"]; len(got) != 1 || got[0] != filepath.ToSlash(readme) {
		t.Errorf("gitlab_get_issue files = %v, want [%s]", got, filepath.ToSlash(readme))
	}
}

// TestScanDocs_OneDottedToken_MergesTheFilesThatSpellIt holds the same
// aggregation for the ID half: one wrong spelling that spread over three pages
// is one row naming three files, which is what makes a report say how far a
// mistake spread.
func TestScanDocs_OneDottedToken_MergesTheFilesThatSpellIt(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "docs/a.md", "Call `work_item.get`.")
	writeDoc(t, root, "docs/b.md", "Call `work_item.get` here too.")
	writeDoc(t, root, "docs/c.md", "Only `issue.get` here.")

	scan := newStubScan()
	if err := scan.scanDocs([]string{filepath.Join(root, "docs")}); err != nil {
		t.Fatalf("scanDocs: %v", err)
	}
	if got := scan.actions["work_item.get"].Files; len(got) != 2 {
		t.Errorf("work_item.get files = %v, want the two pages that spell it", got)
	}
	if len(scan.actions) != 1 {
		t.Errorf("action findings = %v, want only the one wrong spelling", scan.actions)
	}
}

// TestWriteIDFindings_AliasAndDeadID_CarryTheFixEachNeeds holds the two rows
// apart, because a reader acts on them differently. An alias resolves when a
// model follows it and appears in no listing, so its row names what it stands
// for and offers no spelling suggestion; an ID that resolves nowhere gets the
// nearest thing the catalog does publish, since the usual cause is a domain
// the page invented for a real action.
//
// Offering a near spelling beside an alias would be the worse of the two:
// it reads as a correction and would send a reader to rewrite a cross-link
// that already works.
func TestWriteIDFindings_AliasAndDeadID_CarryTheFixEachNeeds(t *testing.T) {
	var out bytes.Buffer
	writeIDFindings(&out, map[string]idFinding{
		"issue.listt":   {Files: []string{"docs/b.md", "docs/a.md"}},
		"project.fetch": {Canonical: "project.get", Files: []string{"docs/c.md"}},
	}, stubIDs())

	report := out.String()
	for _, want := range []string{
		"2 action ID(s) the catalog does not publish:\n",
		"issue.listt names no action; closest: issue.list\n",
		"project.fetch is a registered alias of project.get, not an action ID a listing publishes\n",
		"      docs/a.md\n      docs/b.md\n",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report = %q, want it to contain %q", report, want)
		}
	}
	if got := strings.Count(report, "; closest: "); got != 1 {
		t.Errorf("report names a nearest ID %d times, want it only where the ID resolves nowhere:\n%s", got, report)
	}
	if !inOrder(report, "issue.listt", "project.fetch") {
		t.Errorf("report = %q, want the spelling that reached two pages first", report)
	}
}

// TestReportOrder_MapIteration_DoesNotMoveTheReport holds the property both
// sorted reports exist for: the same findings print the same bytes whichever
// order the map hands them over in. A report that moves between runs cannot be
// diffed, and a reader comparing two runs would read the movement as a change
// in the documentation.
//
// It is asserted over repeated renders because that is the only way to observe
// it: Go randomizes map iteration, so each pass enters the two comparators
// from a different arrangement, and one pass proves nothing about the next.
// The findings are shaped so both comparisons are reached — two spreads that
// differ and two that tie, the tie resolved by name.
func TestReportOrder_MapIteration_DoesNotMoveTheReport(t *testing.T) {
	tools := map[string][]string{
		"gitlab_wide":   {"docs/a.md", "docs/b.md"},
		"gitlab_narrow": {"docs/c.md"},
		"gitlab_middle": {"docs/d.md"},
	}
	actions := map[string]idFinding{
		"issue.widest": {Files: []string{"docs/a.md", "docs/b.md"}},
		"issue.narrow": {Files: []string{"docs/c.md"}},
		"issue.middle": {Files: []string{"docs/d.md"}},
	}
	ids := stubIDs()

	// Enough renders that every arrangement Go's randomization produces is
	// seen; three findings admit three rotations of the map's own order.
	const passes = 64
	var wantTools, wantActions string
	for pass := range passes {
		var toolsOut, actionsOut bytes.Buffer
		writeToolFindings(&toolsOut, tools)
		writeIDFindings(&actionsOut, actions, ids)
		if pass == 0 {
			wantTools, wantActions = toolsOut.String(), actionsOut.String()
			continue
		}
		if toolsOut.String() != wantTools {
			t.Fatalf("pass %d printed the tool findings as\n%s\nwant\n%s", pass, toolsOut.String(), wantTools)
		}
		if actionsOut.String() != wantActions {
			t.Fatalf("pass %d printed the action findings as\n%s\nwant\n%s", pass, actionsOut.String(), wantActions)
		}
	}

	if !inOrder(wantTools, "gitlab_wide", "gitlab_middle", "gitlab_narrow") {
		t.Errorf("tool findings = %q, want the widest spread first and the tie broken by name", wantTools)
	}
	if !inOrder(wantActions, "issue.widest", "issue.middle", "issue.narrow") {
		t.Errorf("action findings = %q, want the widest spread first and the tie broken by name", wantActions)
	}
}

// TestRun_Findings_ReportsSortedAndReturnsExitCode verifies the command's
// contract: a clean tree prints the all-clear and exits 0, findings are
// listed most-referenced first and then alphabetically with their files
// sorted, -check turns findings into an error on stderr and exit 1, and a
// scan failure exits 1 with its cause.
func TestRun_Findings_ReportsSortedAndReturnsExitCode(t *testing.T) {
	okRegistry := func() map[string]struct{} { return stubRegistry }
	cases := []struct {
		name     string
		setup    func(t *testing.T, root string) []string
		collect  func() map[string]struct{}
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
			wantOut:  "no documentation names an unregistered tool or an action the catalog does not have\n",
		},
		{
			name: "action_id_the_catalog_lacks",
			setup: func(t *testing.T, root string) []string {
				t.Helper()
				return []string{writeDoc(t, root, "docs/a.md", "Call `work_item.get` to read one.")}
			},
			collect:  okRegistry,
			check:    true,
			wantCode: 1,
			wantOut:  "1 action ID(s) the catalog does not publish:\n  work_item.get",
			wantErr:  "and 1 action ID(s) the catalog does not publish",
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
			wantErr:  "ERROR: the documentation names 1 tool(s) the server does not register",
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

// TestRun_Summary_CountsItsThreeInputsApart pins the header line to the three
// places its numbers come from.
//
// They are three counts of the same kind on one line, so a transposition still
// reads as a plausible report and no assertion that checks one of them can see
// it. The fixture is what makes the line readable: a stub registry of four
// names, a catalog of its own size, and two documentation files, no two of
// which can take the same value.
func TestRun_Summary_CountsItsThreeInputsApart(t *testing.T) {
	ids, err := actionids.Build()
	if err != nil {
		t.Fatalf("build the action catalog: %v", err)
	}
	if len(stubRegistry) == ids.Count() || ids.Count() == 2 {
		t.Fatalf("the three counts must differ for this case to say anything, got %d, %d and 2", len(stubRegistry), ids.Count())
	}

	root := t.TempDir()
	writeDoc(t, root, "docs/a.md", "Call `gitlab_issue_list`.")
	writeDoc(t, root, "docs/b.md", "Or `issue.list` on the dynamic surface.")

	var out, errOut bytes.Buffer
	code := run(false, []string{filepath.Join(root, "docs")}, func() map[string]struct{} { return stubRegistry }, &out, &errOut)
	if code != 0 {
		t.Fatalf("run = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}

	want := fmt.Sprintf("audit_doc_tool_names: %d registered tool names, %d catalog action IDs, %d documentation files scanned\n",
		len(stubRegistry), ids.Count(), 2)
	if !strings.HasPrefix(out.String(), want) {
		t.Errorf("stdout = %q, want it to open with %q", out.String(), want)
	}
}

// TestRun_NoRootPresent_ReportsEveryDeclarationStale holds the second claim a
// declaration table makes at the level a reader meets it. staleAllowedIDs is
// held to both its directions on its own below; what this adds is that run
// asks it at all, that a stale entry suppresses the all-clear, and that its
// count reaches the error line and the exit code.
//
// Every root is absent, which is the one arrangement in which the whole table
// is stale at once and no later documentation edit can quietly make the case
// stop exercising it.
func TestRun_NoRootPresent_ReportsEveryDeclarationStale(t *testing.T) {
	t.Chdir(t.TempDir())

	var out, errOut bytes.Buffer
	code := run(true, docRoots, func() map[string]struct{} { return stubRegistry }, &out, &errOut)
	if code != 1 {
		t.Fatalf("run(-check) = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if want := fmt.Sprintf("\n%d declaration(s) excuse nothing:\n", len(allowedIDs)); !strings.Contains(out.String(), want) {
		t.Errorf("stdout = %q, want it to contain %q", out.String(), want)
	}
	for token := range allowedIDs {
		if want := "  " + token + " is no longer spelled in any documentation file."; !strings.Contains(out.String(), want) {
			t.Errorf("stdout = %q, want it to name the stale entry %q", out.String(), token)
		}
	}
	if strings.Contains(out.String(), "no documentation names an unregistered tool") {
		t.Errorf("stdout = %q, want no all-clear beside a stale declaration", out.String())
	}

	want := fmt.Sprintf("\nERROR: the documentation names 0 tool(s) the server does not register and 0 action ID(s) the catalog does not publish; %d declaration(s) excuse nothing\n", len(allowedIDs))
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

// TestDocRoots_EveryDeclaredRoot_IsPresent holds the list of audited trees to
// the repository it names. scanRoot passes over a root that is not there, so a
// page moved out from under one of these entries stops being audited and
// nothing says so: the run still prints a file count and an all-clear. The npm
// launcher's README is the entry this matters most for, since a wrong name
// there ships to a registry and is not fixable without republishing.
func TestDocRoots_EveryDeclaredRoot_IsPresent(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	for _, docRoot := range docRoots {
		t.Run(strings.ReplaceAll(docRoot, "/", "_"), func(t *testing.T) {
			if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(docRoot))); statErr != nil {
				t.Errorf("docRoots names %q, which this repository does not hold: %v", docRoot, statErr)
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
	got := run(true, docRoots, func() map[string]struct{} { return names }, &out, &errOut)
	if got != 0 {
		t.Fatalf("run(-check) on the repository docs = %d, want 0\nstdout:\n%s\nstderr:\n%s", got, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "no documentation names an unregistered tool or an action the catalog does not have") {
		t.Errorf("stdout = %q, want the all-clear line", out.String())
	}
	if !strings.Contains(out.String(), "documentation files scanned") || strings.Contains(out.String(), " 0 documentation files scanned") {
		t.Errorf("stdout = %q, want a non-zero scanned file count", out.String())
	}
}

// TestAllowed_EveryEntryStillExcusesSomething holds the allow-list to the
// tree it describes.
//
// An entry here is a judgement a person made about one token: it looks like a
// tool name and is not one. When the prose that carried the token goes, the
// entry stops being a judgement and becomes a claim about a document nobody
// can read, and the next reader has no way to tell one from the other. Five
// entries reached exactly that state when the evaluator whose documentation
// used MCP method names as `gitlab_*` tokens was deleted, and nothing here
// noticed, which is what this test is for.
//
// It walks the same roots the scan walks and skips the same historical
// documents, because an entry excusing a token only a historical document
// carries is excusing something the scan never sees.
func TestAllowed_EveryEntryStillExcusesSomething(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	t.Chdir(root)

	mentioned := make(map[string]struct{})
	for _, docRoot := range docRoots {
		walkErr := filepath.WalkDir(docRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			switch {
			case walkErr != nil:
				return walkErr
			case entry.IsDir():
				return nil
			}
			for _, prefix := range historicalDocs {
				if strings.HasPrefix(filepath.ToSlash(path), prefix) {
					return nil
				}
			}
			return collectTokens(path, mentioned)
		})
		if walkErr != nil && !os.IsNotExist(walkErr) {
			t.Fatalf("walking %s: %v", docRoot, walkErr)
		}
	}

	for token, reason := range allowed {
		if _, ok := mentioned[token]; !ok {
			t.Errorf("allowed[%q] (%s) excuses no token any scanned document carries; delete the entry", token, reason)
		}
	}
}

// TestAllowedIDs_EveryEntry_CarriesAReasonAndIsNotAnID holds the dotted table
// to two things the file-name and meta-entry rules above it cannot be held to,
// because those name classes and this names instances.
//
// A reason, because an entry is one person's judgement that a token which
// looks like an action ID is not one, and without the sentence the next reader
// cannot tell it from an oversight. And that the token really is not an ID:
// an entry naming an action the catalog publishes would silence a correct
// mention and, worse, would go on silencing it if the action were renamed.
func TestAllowedIDs_EveryEntry_CarriesAReasonAndIsNotAnID(t *testing.T) {
	ids, err := actionids.Build()
	if err != nil {
		t.Fatalf("build the action catalog: %v", err)
	}

	for token, reason := range allowedIDs {
		t.Run(token, func(t *testing.T) {
			if !strings.Contains(token, ".") {
				t.Errorf("%q is not a dotted token, so the ID rule never reaches it", token)
			}
			if len(strings.TrimSpace(reason)) < 20 {
				t.Errorf("reason for %q is %q, want a sentence a reviewer can judge", token, reason)
			}
			if ids.IsID(token) {
				t.Errorf("%q is a canonical catalog ID; excusing it hides a correct mention", token)
			}
		})
	}
}

// TestFileNameTails_NoTailIsAnActionName holds the class rule to the property
// that makes it safe to state as a class. A tail that were also an action name
// would excuse every wrong spelling ending in it, which is the one way a rule
// this broad could hide a real finding.
func TestFileNameTails_NoTailIsAnActionName(t *testing.T) {
	ids, err := actionids.Build()
	if err != nil {
		t.Fatalf("build the action catalog: %v", err)
	}

	for tail := range fileNameTails {
		t.Run(tail, func(t *testing.T) {
			if ids.HasMember(tail) {
				t.Errorf("%q is the right half of a catalog action ID, so the file-name rule would excuse a wrong domain under it", tail)
			}
		})
	}
}

// TestExemptID_EachShape_IsExcusedAndOnlyTheTableIsTracked holds the three
// exemptions apart, and holds that only the instance table is tracked: a class
// rule has no entry to go stale, so counting its uses would only invite a
// stale-declaration report about a rule.
func TestExemptID_EachShape_IsExcusedAndOnlyTheTableIsTracked(t *testing.T) {
	cases := []struct {
		name         string
		token        string
		wantDeclared bool
		wantTracked  bool
	}{
		{name: "declared_instance", token: "user.id", wantDeclared: true, wantTracked: true},
		{name: "file_name_tail", token: "issue.rb", wantDeclared: true},
		{name: "meta_surface_entry", token: "gitlab_issue.get", wantDeclared: true},
		{name: "undeclared", token: "work_item.get"},
		{name: "not_dotted", token: "issue"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared, tracked := exemptID(tc.token)
			if declared != tc.wantDeclared || tracked != tc.wantTracked {
				t.Errorf("exemptID(%q) = (%t, %t), want (%t, %t)", tc.token, declared, tracked, tc.wantDeclared, tc.wantTracked)
			}
		})
	}
}

// TestStaleAllowedIDs_UnusedEntry_IsReported holds both directions of the
// stale check, since one that reported everything or nothing would pass a test
// written only one way.
func TestStaleAllowedIDs_UnusedEntry_IsReported(t *testing.T) {
	used := map[string]struct{}{}
	for token := range allowedIDs {
		used[token] = struct{}{}
	}
	if stale := staleAllowedIDs(used); len(stale) != 0 {
		t.Errorf("stale = %v, want none when every entry excused something", stale)
	}

	stale := staleAllowedIDs(map[string]struct{}{})
	if len(stale) != len(allowedIDs) {
		t.Errorf("stale = %v, want every entry when none excused anything", stale)
	}
	if !slices.IsSorted(stale) {
		t.Errorf("stale = %v, want a stable order", stale)
	}
}

// TestSortedTokens_Findings_LeadWithTheWidestSpread holds the report order: a
// spelling that reached five pages is the one to fix first, and two of equal
// spread are ordered by name so the report does not move between runs.
func TestSortedTokens_Findings_LeadWithTheWidestSpread(t *testing.T) {
	got := sortedTokens(map[string]idFinding{
		"zebra.get":  {Files: []string{"a.md"}},
		"alpha.get":  {Files: []string{"a.md"}},
		"spread.get": {Files: []string{"a.md", "b.md"}},
	})
	want := []string{"spread.get", "alpha.get", "zebra.get"}
	if !slices.Equal(got, want) {
		t.Errorf("sortedTokens = %v, want %v", got, want)
	}
}

// inOrder reports whether each token first appears in text after the one
// before it, which is how a report's row order is asserted without repeating
// the column widths the format string chose.
func inOrder(text string, tokens ...string) bool {
	at := -1
	for _, token := range tokens {
		next := strings.Index(text, token)
		if next <= at {
			return false
		}
		at = next
	}
	return true
}

// collectTokens adds every tool-name-shaped token one file carries to
// mentioned.
//
// It is a function of its own rather than the body of the walk above for the
// reason scanFile is: reading a path a WalkDir callback was handed is a
// symlink race the linter is right to name, and both readers here answer it
// the same way, by doing the read outside the callback.
func collectTokens(path string, mentioned map[string]struct{}) error {
	data, err := os.ReadFile(path) //#nosec G304 -- test reading repository docs
	if err != nil {
		return err
	}
	for _, token := range toolToken.FindAllString(string(data), -1) {
		mentioned[token] = struct{}{}
	}
	return nil
}
