package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rehearsalRoot builds a throwaway tree carrying every page this command
// publishes, so a rehearsal can be run against it without touching the
// repository.
func rehearsalRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range pagePaths() {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("make the fixture page directory: %v", err)
		}
		if err := os.WriteFile(full, []byte("# "+path+"\n\nbody\n"), 0o600); err != nil {
			t.Fatalf("write the fixture page: %v", err)
		}
	}
	return root
}

// TestPrepareDryRun_CopiesThePagesAndLeavesNothingOfTheLastRehearsal covers the
// property the comment claims and nothing else could: what is under the scratch
// directory afterwards is one rehearsal's output, not two mixed.
//
// A rehearsal that kept an earlier run's file would publish a page drawn from
// figures nobody folded this time, and the pages are what a maintainer reads to
// decide whether to spend money on a real run.
func TestPrepareDryRun_CopiesThePagesAndLeavesNothingOfTheLastRehearsal(t *testing.T) {
	root := rehearsalRoot(t)

	scratch, prepErr := prepareDryRun(root, io.Discard)
	if prepErr != nil {
		t.Fatalf("prepareDryRun: %v", prepErr)
	}
	if want := filepath.Join(root, dryRunRelDir); scratch != want {
		t.Errorf("prepareDryRun returned %q, want %q", scratch, want)
	}

	stale := filepath.Join(scratch, "left-over-from-the-last-run.md")
	if writeErr := os.WriteFile(stale, []byte("stale\n"), 0o600); writeErr != nil {
		t.Fatalf("plant the stale file: %v", writeErr)
	}
	if _, againErr := prepareDryRun(root, io.Discard); againErr != nil {
		t.Fatalf("second prepareDryRun: %v", againErr)
	}
	if _, statErr := os.Stat(stale); !os.IsNotExist(statErr) {
		t.Errorf("the previous rehearsal's file survived: stat err = %v, want it gone", statErr)
	}

	for _, path := range pagePaths() {
		body, readErr := os.ReadFile(filepath.Join(scratch, path))
		if readErr != nil {
			t.Errorf("read the rehearsed %s: %v", path, readErr)
			continue
		}
		// Copied from the real page rather than written as a fixture: a
		// rehearsal that drew a page nobody publishes would rehearse nothing.
		if !strings.Contains(string(body), "# "+path) {
			t.Errorf("the rehearsed %s is not a copy of the published one: %q", path, body)
		}
	}
}

// TestPrepareDryRun_NamesThePageItCouldNotRead holds the failure a command run
// outside a checkout produces: it says which page it wanted, rather than
// rehearsing against a page that is not there.
func TestPrepareDryRun_NamesThePageItCouldNotRead(t *testing.T) {
	_, err := prepareDryRun(t.TempDir(), io.Discard)
	if err == nil {
		t.Fatal("prepareDryRun of a root carrying no pages succeeded, want a refusal")
	}
	first := pagePaths()[0]
	if !strings.Contains(err.Error(), first) {
		t.Errorf("prepareDryRun error = %q, want it to name %q", err, first)
	}
}

// TestBannerDryRun_MarksEveryPageItDrew covers the one thing that makes a
// rehearsed page safe to look at.
//
// The banner is prepended to the file rather than rendered into a block on
// purpose, so this asserts where it lands as well as that it lands: a banner
// inside a block would be a difference between what a rehearsal draws and what
// a real fold draws, which is the one difference a rehearsal must not have.
func TestBannerDryRun_MarksEveryPageItDrew(t *testing.T) {
	root := rehearsalRoot(t)
	scratch, prepErr := prepareDryRun(root, io.Discard)
	if prepErr != nil {
		t.Fatalf("prepareDryRun: %v", prepErr)
	}
	if bannerErr := bannerDryRun(scratch, io.Discard); bannerErr != nil {
		t.Fatalf("bannerDryRun: %v", bannerErr)
	}

	for _, path := range pagePaths() {
		body, readErr := os.ReadFile(filepath.Join(scratch, path))
		if readErr != nil {
			t.Errorf("read the rehearsed %s: %v", path, readErr)
			continue
		}
		if !strings.HasPrefix(string(body), dryRunBanner) {
			t.Errorf("the rehearsed %s does not open with the banner: %q", path, firstLine(string(body)))
		}
		if strings.Count(string(body), dryRunBanner) != 1 {
			t.Errorf("the rehearsed %s carries the banner %d times, want once",
				path, strings.Count(string(body), dryRunBanner))
		}
	}

	// The published pages are read and never written: a rehearsal that marked
	// the repository's own page would publish the banner on the next fold.
	for _, path := range pagePaths() {
		body, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil {
			t.Fatalf("read the published %s: %v", path, readErr)
		}
		if strings.Contains(string(body), dryRunBanner) {
			t.Errorf("the rehearsal marked the published %s", path)
		}
	}
}

// TestBannerDryRun_NamesThePageItCouldNotRead holds the refusal rather than a
// silently unmarked page: an unbannered rehearsal page reads as a measurement.
func TestBannerDryRun_NamesThePageItCouldNotRead(t *testing.T) {
	err := bannerDryRun(t.TempDir(), io.Discard)
	if err == nil {
		t.Fatal("bannerDryRun over an empty directory succeeded, want a refusal")
	}
	first := pagePaths()[0]
	if !strings.Contains(err.Error(), first) {
		t.Errorf("bannerDryRun error = %q, want it to name %q", err, first)
	}
}

// firstLine trims a body down to what an error message can carry.
func firstLine(body string) string {
	line, _, _ := strings.Cut(body, "\n")
	return line
}
