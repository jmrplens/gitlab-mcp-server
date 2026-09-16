package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
)

// TestRun_CheckModeAcceptsTheCommittedLedger is the gate CI runs: the page in
// the repository is what this tree's corpus and catalog produce now.
//
// It defers to the harness first, because a stacked pull request leaves every
// generated artifact stale on purpose and refreshes them once at its top; the
// comparison is not lost, it runs where the refresh happens. Every other test
// in this file works against a throwaway root and is never deferred, so a
// generator that stopped producing what it produces would still fail here on
// every run.
func TestRun_CheckModeAcceptsTheCommittedLedger(t *testing.T) {
	freshness.SkipIfDeferred(t)
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("find the repository root: %v", err)
	}
	if runErr := run(root, defaultOutputPath, true); runErr != nil {
		t.Errorf("the committed ledger is stale: %v", runErr)
	}
}

// TestRun_WritesThenChecksInAThrowawayRoot drives both halves of the command
// against a directory of its own: a write produces the ledger, a check then
// accepts it, and a check of a root with no ledger in it reports the file
// rather than reporting nothing.
func TestRun_WritesThenChecksInAThrowawayRoot(t *testing.T) {
	root := t.TempDir()
	if err := run(root, defaultOutputPath, true); err == nil {
		t.Error("check mode accepted a root with no ledger in it")
	}
	if err := run(root, defaultOutputPath, false); err != nil {
		t.Fatalf("writing the ledger: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(root, defaultOutputPath))
	if err != nil {
		t.Fatalf("reading the written ledger: %v", err)
	}
	page := string(written)
	if !strings.Contains(page, "breadth, not coverage") {
		t.Error("the ledger does not say that it is breadth rather than coverage")
	}
	for _, want := range []string{"issue.list", "gitlab_discover_project", "MT-001"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(page, want) {
				t.Errorf("the ledger does not name %q", want)
			}
		})
	}
	if recheckErr := run(root, defaultOutputPath, true); recheckErr != nil {
		t.Errorf("check mode refused the ledger it had just written: %v", recheckErr)
	}
}

// TestRun_ReportsAStaleLedger pins the sentence a reader acts on: the file and
// the command that refreshes it.
func TestRun_ReportsAStaleLedger(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, defaultOutputPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("making the ledger directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Something else\n"), 0o600); err != nil {
		t.Fatalf("writing a stale ledger: %v", err)
	}
	err := run(root, defaultOutputPath, true)
	if err == nil {
		t.Fatal("check mode accepted a ledger that is not what the corpus produces")
	}
	if !strings.Contains(err.Error(), regenerate) {
		t.Errorf("the failure is %q, want it to name %q", err, regenerate)
	}
}

// TestRender_RefusesAnActionTheCatalogDoesNotHave covers the one refusal this
// command makes of its own. The corpus gate catches the same thing on every
// push, and this is the second chance: a ledger that silently left out the
// actions it could not resolve would publish a breadth figure that is wrong in
// the direction that flatters it.
func TestRender_RefusesAnActionTheCatalogDoesNotHave(t *testing.T) {
	empty := catalogFacts{actions: map[string]catalogAction{}, domains: map[string]int{}}
	_, err := render(empty)
	if err == nil {
		t.Fatal("render accepted a catalog that has none of the actions the corpus names")
	}
	if !strings.Contains(err.Error(), "the catalog does not have") {
		t.Errorf("the failure is %q, want it to say the catalog does not have them", err)
	}
}

// TestRun_NamesTheStageThatFailed covers the two failures above the write: a
// catalog that cannot be read and a corpus the catalog cannot account for.
// Each reports what it was doing, because a generator that printed only the
// underlying error would leave a reader guessing which half of the join broke.
func TestRun_NamesTheStageThatFailed(t *testing.T) {
	tests := []struct {
		name   string
		reader func() (catalogFacts, error)
		want   string
	}{
		{
			name:   "the catalog cannot be read",
			reader: func() (catalogFacts, error) { return catalogFacts{}, os.ErrPermission },
			want:   "read the action catalog",
		},
		{
			name: "the catalog does not hold what the corpus names",
			reader: func() (catalogFacts, error) {
				return catalogFacts{actions: map[string]catalogAction{}, domains: map[string]int{}}, nil
			},
			want: "render the ledger",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := readCatalogFacts
			readCatalogFacts = tc.reader
			defer func() { readCatalogFacts = original }()

			err := run(t.TempDir(), defaultOutputPath, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("run() error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

// TestNeedsOf_SpellsEveryRequirement covers the grouping the needs table is
// built from, including the two no Free case sets today.
func TestNeedsOf_SpellsEveryRequirement(t *testing.T) {
	tests := []struct {
		name     string
		stimulus modelcorpus.Stimulus
		want     []string
	}{
		{
			name:     "nothing but a tier",
			stimulus: modelcorpus.Stimulus{Needs: modelcorpus.Needs{}},
			want:     []string{"tier free"},
		},
		{
			name: "everything at once",
			stimulus: modelcorpus.Stimulus{Needs: modelcorpus.Needs{
				Tier: modelcorpus.TierUltimate, Runner: true, Admin: true, FixtureService: true,
			}},
			want: []string{"tier ultimate", "a CI runner", "an administrator token", "the fixture service"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := needsOf(tc.stimulus)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("needsOf() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAppendOnce_KeepsOneMentionPerCase covers the de-duplication a case that
// names one action in two steps needs: MS-002 lists jobs and fetches a trace
// of one, and the ledger says it once.
func TestAppendOnce_KeepsOneMentionPerCase(t *testing.T) {
	values := appendOnce(appendOnce(appendOnce(nil, "MT-001"), "MT-002"), "MT-001")
	if strings.Join(values, ",") != "MT-001,MT-002" {
		t.Errorf("appendOnce produced %v, want each identifier once", values)
	}
}

// TestIndexRows_ListsOrCountsAndAlwaysSorts covers both shapes of the two
// column tables and the ordering every generated page depends on.
func TestIndexRows_ListsOrCountsAndAlwaysSorts(t *testing.T) {
	index := map[string][]string{
		"second": {"MT-002", "MT-001"},
		"first":  {"MT-003"},
	}
	listed := indexRows(index, asCode, true)
	if len(listed) != 2 || listed[0][0] != "`first`" || listed[1][1] != "MT-001, MT-002" {
		t.Errorf("indexRows(listing) = %v, want the names sorted and the cases listed sorted", listed)
	}
	counted := indexRows(index, asPlain, false)
	if len(counted) != 2 || counted[0][0] != "first" || counted[1][1] != "2" {
		t.Errorf("indexRows(counting) = %v, want the names plain and the cases counted", counted)
	}
}

// repositoryRoot walks up from the working directory to the module root, which
// is the root the command resolves for itself when it runs.
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
