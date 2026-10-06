package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// testSource is a source over the synthetic builds and data file, rooted in a
// directory of the test's own.
func testSource(t *testing.T) source {
	t.Helper()
	root := t.TempDir()
	return source{
		root:     func() (string, error) { return root, nil },
		builds:   func() ([]build, error) { return testBuilds(t), nil },
		surfaces: testSurfaces,
		scopes:   map[string][]string{"gitlab_widget": {"admin_mode"}},
		grants:   testGrants(),
		domains:  encode(t, testDomains()),
	}
}

// rootOf is the directory a source writes into.
func rootOf(t *testing.T, src source) string {
	t.Helper()
	root, err := src.root()
	if err != nil {
		t.Fatalf("root() error = %v", err)
	}
	return root
}

func TestRun_WriteThenCheck_WritesThePagesAndRemovesWhatItDidNotWrite(t *testing.T) {
	src := testSource(t)
	root := rootOf(t, src)
	dir := filepath.Join(root, filepath.FromSlash(english.dir))
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	// sequential: setup, writing the files the run below is judged against
	for _, name := range []string{"orphan.mdx", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := run(src, false, &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := out.String(); got != "wrote: 6 pages, 2 groups, 4 actions\n" {
		t.Errorf("summary = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "orphan.mdx")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("orphan.mdx survived the write: %v", err)
	}
	for _, kept := range []string{"notes.txt", "nested", "widget.mdx"} {
		t.Run(kept, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
				t.Errorf("%s is missing after the write: %v", kept, err)
			}
		})
	}
	out.Reset()
	if err := run(src, true, &out); err != nil {
		t.Fatalf("run(check) error = %v", err)
	}
	if got := out.String(); got != "current: 6 pages, 2 groups, 4 actions\n" {
		t.Errorf("check summary = %q", got)
	}
}

func TestRun_CheckOfAChangedTree_ReportsEveryStalePage(t *testing.T) {
	src := testSource(t)
	root := rootOf(t, src)
	if err := run(src, false, io.Discard); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	spanishDir := filepath.Join(root, filepath.FromSlash(spanish.dir))
	if err := os.WriteFile(filepath.Join(spanishDir, "widget.mdx"), []byte("edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// sequential: setup, writing the stray pages the check below reports
	for _, stray := range []string{"gone.mdx", "zzz.mdx"} {
		if err := os.WriteFile(filepath.Join(spanishDir, stray), []byte("old\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := run(src, true, io.Discard)
	if err == nil {
		t.Fatal("run(check) error = nil, want the stale and the stray pages reported")
	}
	for _, want := range []string{
		"widget.mdx is stale; run make gen-tool-reference",
		spanish.dir + "/gone.mdx is not a page the catalog generates; run make gen-tool-reference",
		spanish.dir + "/zzz.mdx is not a page the catalog generates",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("run(check) error = %v, want it to contain %q", err, want)
			}
		})
	}
	if _, statErr := os.Stat(filepath.Join(spanishDir, "gone.mdx")); statErr != nil {
		t.Errorf("a check removed gone.mdx: %v", statErr)
	}
}

func TestRun_CheckOfAnEmptyTree_ReportsTheMissingPages(t *testing.T) {
	err := run(testSource(t), true, io.Discard)
	if err == nil || !strings.Contains(err.Error(), filepath.FromSlash(english.dir)) || !strings.Contains(err.Error(), filepath.FromSlash(spanish.dir)) {
		t.Errorf("run(check) error = %v, want the missing directories named", err)
	}
}

func TestRun_AStageFails_ReturnsItsError(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		edit func(*source)
		want string
	}{
		{name: "root", edit: func(s *source) { s.root = func() (string, error) { return "", boom } }, want: "boom"},
		{name: "data file", edit: func(s *source) { s.domains = []byte("{") }, want: "parse domains.json"},
		{name: "builds", edit: func(s *source) { s.builds = func() ([]build, error) { return nil, boom } }, want: "boom"},
		{name: "surfaces", edit: func(s *source) { s.surfaces = func() surfaceNames { return surfaceNames{} } }, want: "the meta surface registers no tool"},
		{name: "coverage", edit: func(s *source) {
			d := testDomains()
			delete(d["groups"].(map[string]any), "gitlab_helper")
			d["categories"] = d["categories"].([]any)[:1]
			s.domains = encode(t, d)
		}, want: "the catalog builds gitlab_helper and domains.json does not describe it"},
		{name: "labels", edit: func(s *source) {
			s.builds = func() ([]build, error) {
				catalog := testCatalog(t,
					testGroup("gitlab_widget", actioncatalog.SurfaceKindMetaGroup, []string{"sampling"}, widgetList()),
					testGroup("gitlab_helper", actioncatalog.SurfaceKindRuntimeUtility, nil, helperResolve()))
				return []build{{tier: edition.Free, catalog: catalog}}, nil
			}
		}, want: "needs the sampling client capability"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := testSource(t)
			tt.edit(&src)
			err := run(src, false, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("run() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestRun_PruneFails_ReturnsItsError(t *testing.T) {
	src := testSource(t)
	root := rootOf(t, src)
	dir := filepath.Join(root, filepath.FromSlash(spanish.dir))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stray.mdx"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := removeFile
	t.Cleanup(func() { removeFile = original })
	removeFile = func(string) error { return errors.New("locked") }
	err := run(src, false, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "remove "+spanish.dir+"/stray.mdx: locked") {
		t.Errorf("run() error = %v, want the removal named", err)
	}
}

func TestPrune_DirectoryIsAFile_ReportsTheRead(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pages"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := prune(root, "pages", nil, false)
	if err == nil || !strings.Contains(err.Error(), "read pages") {
		t.Errorf("prune() error = %v, want the read named", err)
	}
	orphans, err := prune(root, "absent", nil, true)
	if err != nil || orphans != nil {
		t.Errorf("prune() of a missing directory = %v, %v, want nothing", orphans, err)
	}
}

// TestPrune_DanglingLinkAtTheDirectory_ReportsTheRead holds the case Windows
// takes for a file: reading the path as a directory answers "does not exist"
// while something is there. A dangling symbolic link gives Linux the same pair
// of answers, so the branch that refuses to read it as an empty directory runs
// on the system the coverage and condition gates measure.
func TestPrune_DanglingLinkAtTheDirectory_ReportsTheRead(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "pages")); err != nil {
		t.Skipf("this platform will not create a symbolic link here: %v", err)
	}
	_, err := prune(root, "pages", nil, false)
	if err == nil || !strings.Contains(err.Error(), "read pages") {
		t.Errorf("prune() error = %v, want the read named", err)
	}
}

func TestRunMain_Flags_MapToExitCodes(t *testing.T) {
	original := runReference
	t.Cleanup(func() { runReference = original })
	var gotCheck bool
	runReference = func(_ source, check bool, _ io.Writer) error {
		gotCheck = check
		return nil
	}
	var stderr bytes.Buffer
	if code := runMain([]string{"-check"}, io.Discard, &stderr); code != 0 || !gotCheck {
		t.Errorf("runMain(-check) = %d (check %t), want 0 with check passed on", code, gotCheck)
	}
	if code := runMain([]string{"-h"}, io.Discard, &stderr); code != 0 {
		t.Errorf("runMain(-h) = %d, want 0", code)
	}
	if code := runMain([]string{"-bogus"}, io.Discard, &stderr); code != 2 {
		t.Errorf("runMain(-bogus) = %d, want 2", code)
	}
	runReference = func(source, bool, io.Writer) error { return errors.New("stale") }
	stderr.Reset()
	if code := runMain(nil, io.Discard, &stderr); code != 1 || stderr.String() != "gen_tool_reference: stale\n" {
		t.Errorf("runMain() = %d, stderr %q, want 1 and the error", code, stderr.String())
	}
}

func TestMain_ExitCode_IsHandedToTheProcess(t *testing.T) {
	originalExit, originalRun, originalArgs := osExit, runReference, os.Args
	t.Cleanup(func() { osExit, runReference, os.Args = originalExit, originalRun, originalArgs })
	os.Args = []string{toolName}
	runReference = func(source, bool, io.Writer) error { return errors.New("stale") }
	code := -1
	osExit = func(c int) { code = c }
	main()
	if code != 1 {
		t.Errorf("main() exited %d, want 1", code)
	}
}

func TestDefaultSource_RealInputs_ReadTheRepository(t *testing.T) {
	src := defaultSource()
	root, err := src.root()
	if err != nil {
		t.Fatalf("root() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr != nil {
		t.Errorf("root %s holds no go.mod: %v", root, statErr)
	}
	if !bytes.Equal(src.domains, domainsJSON) || src.scopes["gitlab_admin"] == nil {
		t.Error("defaultSource() does not read the embedded data file and the scope map")
	}
	if src.builds == nil || src.surfaces == nil {
		t.Error("defaultSource() has no builds or surfaces")
	}
	if src.grants != actiongrants.Table() {
		t.Error("defaultSource() does not read the action grants table compiled into the binary")
	}
}

func TestVerb_Mode_NamesWhatTheRunDid(t *testing.T) {
	if verb(true) != "current" || verb(false) != "wrote" {
		t.Errorf("verb = %q, %q", verb(true), verb(false))
	}
}
