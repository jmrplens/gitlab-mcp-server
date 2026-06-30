package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
		if !found {
			t.Errorf("expected citation %q not found in %v", a, areas)
		}
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
