package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// TestScanDocCitations verifies the source scan finds the doc/api areas cited in
// the adjudication tables (the citations live in comments, so the scan keeps the
// validation set in sync automatically).
func TestScanDocCitations(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	areas, err := scanDocCitations(root)
	if err != nil {
		t.Fatalf("scanDocCitations: %v", err)
	}
	if len(areas) < 10 {
		t.Fatalf("found %d cited areas, want >= 10", len(areas))
	}
	want := map[string]bool{"boards": false, "tags": false, "environments": false, "projects": false}
	for _, a := range areas {
		if _, ok := want[a]; ok {
			want[a] = true
		}
	}
	for a, found := range want {
		t.Run(a, func(t *testing.T) {
			if !found {
				t.Errorf("expected citation %q not found in %v", a, areas)
			}
		})
	}
}

// TestRunValidateDocs_OKWhenCitedDocsCached seeds an isolated cache with every
// cited area and runs offline: all citations validate and the gate passes.
func TestRunValidateDocs_OKWhenCitedDocsCached(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	areas, err := scanDocCitations(root)
	if err != nil {
		t.Fatalf("scanDocCitations: %v", err)
	}

	cache := t.TempDir()
	for _, a := range areas {
		if writeErr := os.WriteFile(filepath.Join(cache, a+".md"), []byte("# "+a), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	fetcher := apidocs.New(root, apidocs.Options{Offline: true, CacheDir: cache})

	out, ok, err := runValidateDocs(context.Background(), root, fetcher)
	if err != nil {
		t.Fatalf("runValidateDocs: %v", err)
	}
	if !ok {
		t.Fatalf("gate failed despite all docs cached; report: %s", out)
	}
	var rep docValidationReport
	if jsonErr := json.Unmarshal(out, &rep); jsonErr != nil {
		t.Fatalf("unmarshal: %v", jsonErr)
	}
	if rep.OK != rep.Checked || len(rep.Stale) != 0 {
		t.Fatalf("report = %+v, want all OK", rep)
	}
}

// TestRunValidateDocs_StaleWhenMissing runs offline against an empty cache: every
// citation is reported stale and the gate fails.
func TestRunValidateDocs_StaleWhenMissing(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	fetcher := apidocs.New(root, apidocs.Options{Offline: true, CacheDir: t.TempDir()})

	out, ok, err := runValidateDocs(context.Background(), root, fetcher)
	if err != nil {
		t.Fatalf("runValidateDocs: %v", err)
	}
	if ok {
		t.Fatal("gate passed despite empty cache; want failure")
	}
	var rep docValidationReport
	if jsonErr := json.Unmarshal(out, &rep); jsonErr != nil {
		t.Fatalf("unmarshal: %v", jsonErr)
	}
	if rep.OK != 0 || len(rep.Stale) != rep.Checked {
		t.Fatalf("report = %+v, want all stale", rep)
	}
}

// TestRunValidateDocs_EmptyDocs_ReportedStale verifies a cached doc with no
// content counts as stale: an empty body cannot justify an adjudication any
// more than a missing one can.
func TestRunValidateDocs_EmptyDocs_ReportedStale(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	areas, err := scanDocCitations(root)
	if err != nil {
		t.Fatalf("scanDocCitations: %v", err)
	}
	cache := t.TempDir()
	for _, a := range areas {
		if writeErr := os.WriteFile(filepath.Join(cache, a+".md"), []byte("  \n"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	fetcher := apidocs.New(root, apidocs.Options{Offline: true, CacheDir: cache})

	out, ok, err := runValidateDocs(context.Background(), root, fetcher)
	if err != nil {
		t.Fatalf("runValidateDocs: %v", err)
	}
	if ok {
		t.Fatal("gate passed on empty docs; want failure")
	}
	var rep docValidationReport
	if jsonErr := json.Unmarshal(out, &rep); jsonErr != nil {
		t.Fatalf("unmarshal: %v", jsonErr)
	}
	if rep.OK != 0 || len(rep.Stale) != rep.Checked {
		t.Fatalf("report = %+v, want every citation stale", rep)
	}
	for _, issue := range rep.Stale {
		if issue.Error != "doc is empty" {
			t.Errorf("%s: error = %q, want \"doc is empty\"", issue.Area, issue.Error)
		}
	}
}

// writeTree writes each slash-separated relative path in files under dir,
// creating parent directories. It lives outside the Test function so the
// fixture loop is setup rather than a case table.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// TestScanDocCitations_SourceTree_SelectsGoSourceOnly verifies the citation
// scan reads non-test Go files anywhere under cmd/audit_1to1, ignores test
// files and non-Go files, and deduplicates and sorts the areas it finds.
func TestScanDocCitations_SourceTree_SelectsGoSourceOnly(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "cmd", "audit_1to1")
	files := map[string]string{
		"main.go":                  "package main // see doc/api/tags.md and doc/api/boards.md",
		"internal/x/analyze.go":    "package x // doc/api/boards.md again, plus doc/api/access_requests.md",
		"main_test.go":             "package main // doc/api/ignored_in_tests.md",
		"notes.txt":                "doc/api/ignored_in_text.md",
		"internal/x/data_gen.json": "doc/api/ignored_in_json.md",
	}
	writeTree(t, src, files)

	areas, err := scanDocCitations(root)
	if err != nil {
		t.Fatalf("scanDocCitations: %v", err)
	}
	want := []string{"access_requests", "boards", "tags"}
	if strings.Join(areas, ",") != strings.Join(want, ",") {
		t.Errorf("areas = %v, want %v", areas, want)
	}
}

// TestScanDocCitations_Failures_ReturnErrors verifies a missing auditor tree
// and a Go file that cannot be read both fail the scan, so a validation run
// cannot silently check an empty citation set.
func TestScanDocCitations_Failures_ReturnErrors(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{name: "missing_source_tree", setup: func(_ *testing.T, _ string) {}},
		{
			name: "unreadable_go_file",
			setup: func(t *testing.T, root string) {
				t.Helper()
				src := filepath.Join(root, "cmd", "audit_1to1")
				if err := os.MkdirAll(src, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "missing.go"), filepath.Join(src, "broken.go")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)
			_, err := scanDocCitations(root)
			if err == nil || !strings.HasPrefix(err.Error(), "scan doc citations: ") {
				t.Fatalf("scanDocCitations error = %v, want a scan doc citations error", err)
			}
		})
	}
}
