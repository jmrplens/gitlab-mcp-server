package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
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

// TestRunMain_FlagParsing_ReturnsTheExitCode verifies that a parse failure is
// an exit code this function returns rather than an os.Exit inside the flag
// package: an unknown flag is the usage exit, 2, and -h is the one parse
// failure that exits clean, which is what ExitOnError would have done for
// both. Neither reaches the render, which the absent ledger witnesses.
func TestRunMain_FlagParsing_ReturnsTheExitCode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
		text string
	}{
		{name: "an unknown flag is a usage error", args: []string{"-bogus"}, want: 2, text: "flag provided but not defined: -bogus"},
		{name: "asking for help exits clean", args: []string{"-h"}, want: 0, text: "-check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := checkoutOfItsOwn(t)

			var stderr bytes.Buffer
			if code := runMain(tt.args, &stderr); code != tt.want {
				t.Fatalf("runMain(%v) = %d, want %d (stderr %q)", tt.args, code, tt.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.text) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.text)
			}
			if _, err := os.Stat(filepath.Join(root, defaultOutputPath)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("os.Stat(ledger) = %v, want nothing written past a failed parse", err)
			}
		})
	}
}

// TestRunMain_WritesThenChecksInACheckoutOfItsOwn drives the command the way a
// process drives it, flags and working directory included, against a module
// root of its own: the ledger lands where the command resolves the root, and
// the check that follows accepts what the write produced.
func TestRunMain_WritesThenChecksInACheckoutOfItsOwn(t *testing.T) {
	root := checkoutOfItsOwn(t)

	var stderr bytes.Buffer
	if code := runMain(nil, &stderr); code != 0 {
		t.Fatalf("runMain() = %d, want 0 (stderr %q)", code, stderr.String())
	}
	written, err := os.ReadFile(filepath.Join(root, defaultOutputPath))
	if err != nil {
		t.Fatalf("reading the written ledger: %v", err)
	}
	if !strings.Contains(string(written), "Model evaluation corpus breadth") {
		t.Errorf("the ledger does not carry its own heading:\n%s", written)
	}
	if code := runMain([]string{"-check"}, &stderr); code != 0 {
		t.Errorf("runMain(-check) = %d over the ledger it had just written (stderr %q)", code, stderr.String())
	}
}

// TestRunMain_AFailingStage_ExitsOneAndNamesIt verifies the two failures that
// share the exit code 1 still say which they are, since they are fixed by
// different things: one means this is not a checkout, the other that the
// ledger no longer says what the corpus and the catalog produce.
func TestRunMain_AFailingStage_ExitsOneAndNamesIt(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T)
		args    []string
		want    string
	}{
		{
			name:    "no repository above the working directory",
			prepare: func(t *testing.T) { t.Helper(); t.Chdir(t.TempDir()) },
			want:    "find repository root: go.mod not found",
		},
		{
			name: "the ledger has drifted",
			prepare: func(t *testing.T) {
				t.Helper()
				root := checkoutOfItsOwn(t)
				path := filepath.Join(root, defaultOutputPath)
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatalf("making the ledger directory: %v", err)
				}
				if err := os.WriteFile(path, []byte("# Something else\n"), 0o600); err != nil {
					t.Fatalf("writing a stale ledger: %v", err)
				}
			},
			args: []string{"-check"},
			want: regenerate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.prepare(t)

			var stderr bytes.Buffer
			if code := runMain(tt.args, &stderr); code != 1 {
				t.Fatalf("runMain(%v) = %d, want 1 (stderr %q)", tt.args, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

// TestMain_HandsTheExitCodeToTheSeam verifies main wires runMain's result to
// the exit seam and reads its flags from os.Args past the program name, which
// is the only thing main does and the one line no other test here reaches.
// Both codes are driven, so the wiring cannot be a constant, and the check
// case is what holds the flags: read from os.Args whole, the program name
// would stop the parse before -check and the run would write instead.
func TestMain_HandsTheExitCodeToTheSeam(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		want       int
		wantLedger bool
	}{
		{name: "a run that writes the ledger exits clean", want: 0, wantLedger: true},
		{name: "a check with no ledger to read exits one", args: []string{"-check"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := checkoutOfItsOwn(t)
			oldArgs := os.Args
			os.Args = append([]string{toolName}, tt.args...)
			t.Cleanup(func() { os.Args = oldArgs })
			oldStderr := os.Stderr
			os.Stderr = devNull(t)
			t.Cleanup(func() { os.Stderr = oldStderr })
			code := -1
			osExit = func(got int) { code = got }
			t.Cleanup(func() { osExit = os.Exit })

			main()

			if code != tt.want {
				t.Errorf("main() exited %d, want %d", code, tt.want)
			}
			_, err := os.Stat(filepath.Join(root, defaultOutputPath))
			if tt.wantLedger && err != nil {
				t.Errorf("os.Stat(ledger) = %v, want the ledger written", err)
			}
			if !tt.wantLedger && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("os.Stat(ledger) = %v, want nothing written by a check", err)
			}
		})
	}
}

// TestReadCatalog_BuildsTheUltimateCatalogWithTheMCPGroup holds the two
// options the page's own claim rests on, since it says a share here is of the
// catalog at Ultimate: a Premium and an Ultimate action are in it, and so is
// the MCP group's status action, which the base catalog leaves out. It also
// holds the two strings of a catalogAction apart, both of them names, by
// asserting each action's domain and tier separately.
//
// The error readCatalog returns is reachable from no test, because the catalog
// is assembled from the groups compiled into this binary and cannot fail to
// build, which is why the condition gate reports its guard evaluated one way
// only. The property that makes it unreachable is asserted here rather than
// the guard being deleted, which would discard a returned error.
func TestReadCatalog_BuildsTheUltimateCatalogWithTheMCPGroup(t *testing.T) {
	facts, err := readCatalog()
	if err != nil {
		t.Fatalf("readCatalog() error = %v, want the catalog this binary carries", err)
	}

	tests := []struct {
		action string
		domain string
		tier   string
	}{
		{action: "issue.list", domain: "issue", tier: edition.Free.String()},
		{action: "merge_train.add", domain: "merge_train", tier: edition.Premium.String()},
		{action: "vulnerability.get", domain: "vulnerability", tier: edition.Ultimate.String()},
		{action: "server.status", domain: "server", tier: edition.Free.String()},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			got, known := facts.actions[tt.action]
			if !known {
				t.Fatalf("the catalog has no %s", tt.action)
			}
			if got.domain != tt.domain || got.tier != tt.tier {
				t.Errorf("%s = {domain %q, tier %q}, want {domain %q, tier %q}", tt.action, got.domain, got.tier, tt.domain, tt.tier)
			}
		})
	}

	counted := 0
	for _, count := range facts.domains {
		counted += count
	}
	if counted != len(facts.actions) {
		t.Errorf("the domain counts add up to %d, the catalog holds %d actions", counted, len(facts.actions))
	}
}

// TestRender_ActionsTable_SpellsTheTierAndTheDomainsTableTheDomain holds the
// rendered halves of the same pair: the tier column carries what edition
// spells a tier, all three of which the corpus reaches, and no domain row is
// named after one. Crossed, the two produce a page of exactly the same shape.
func TestRender_ActionsTable_SpellsTheTierAndTheDomainsTableTheDomain(t *testing.T) {
	page := renderedLedger(t)
	tiers := []string{edition.Free.String(), edition.Premium.String(), edition.Ultimate.String()}
	reached := map[string]bool{}
	for _, row := range markdownRows(t, page, "Actions the corpus names") {
		if !slices.Contains(tiers, row[1]) {
			t.Fatalf("the tier of %s reads %q, which is not a tier", row[0], row[1])
		}
		reached[row[1]] = true
	}
	if len(reached) != len(tiers) {
		t.Errorf("the tier column carries %v, want every tier of %v, or it cannot be told from a constant", reached, tiers)
	}
	for _, row := range markdownRows(t, page, "Domains the corpus names") {
		if slices.Contains(tiers, row[0]) {
			t.Errorf("the domains table names %q, which is a tier rather than a domain", row[0])
		}
	}
}

// TestRender_DomainsTable_NamesNoMoreThanTheCatalogHolds holds the two counts
// of a domain row apart, which are interchangeable integers: what the corpus
// names cannot exceed what the catalog holds, somewhere it is strictly less,
// and the named counts add up to the actions table, since every named action
// sits in exactly one domain.
func TestRender_DomainsTable_NamesNoMoreThanTheCatalogHolds(t *testing.T) {
	page := renderedLedger(t)
	named, narrower := 0, false
	for _, row := range markdownRows(t, page, "Domains the corpus names") {
		reached, held := mustAtoi(t, row[1]), mustAtoi(t, row[2])
		if reached > held {
			t.Errorf("%s names %d actions of the %d the catalog holds", row[0], reached, held)
		}
		if reached < held {
			narrower = true
		}
		named += reached
	}
	if !narrower {
		t.Error("no domain names fewer actions than it holds, so the two columns cannot be told apart")
	}
	if rows := len(markdownRows(t, page, "Actions the corpus names")); named != rows {
		t.Errorf("the domain rows account for %d actions, the actions table lists %d", named, rows)
	}
}

// TestRender_Summary_AgreesWithTheTablesBelowIt holds each figure a reader
// sees first to the table it summarizes, and each share to the order it is
// read in: what the corpus names, then what the catalog holds. Crossing the
// halves of a share, or the case and step counts, leaves the same page shape.
func TestRender_Summary_AgreesWithTheTablesBelowIt(t *testing.T) {
	page := renderedLedger(t)
	summary := map[string]string{}
	for _, row := range markdownRows(t, page, "Summary") {
		summary[row[0]] = row[1]
	}

	tests := []struct {
		figure  string
		heading string
	}{
		{figure: "Catalog actions named", heading: "Actions the corpus names"},
		{figure: "Catalog domains named", heading: "Domains the corpus names"},
		{figure: "Worlds asked for", heading: "Worlds the corpus asks for"},
	}
	for _, tt := range tests {
		t.Run(tt.figure, func(t *testing.T) {
			reached, held := share(t, summary[tt.figure])
			if rows := len(markdownRows(t, page, tt.heading)); reached != rows {
				t.Errorf("%q reads %q, the %q table has %d rows", tt.figure, summary[tt.figure], tt.heading, rows)
			}
			if reached > held {
				t.Errorf("%q reads %q, which names more than there is to name", tt.figure, summary[tt.figure])
			}
		})
	}

	if tools, rows := mustAtoi(t, summary["Standalone tools named"]), len(markdownRows(t, page, "Standalone tools the corpus names")); tools != rows {
		t.Errorf("the summary counts %d standalone tools, the table lists %d", tools, rows)
	}
	if cases, steps := mustAtoi(t, summary["Cases"]), mustAtoi(t, summary["Steps declared"]); steps < cases {
		t.Errorf("the summary declares %d steps over %d cases, which is fewer steps than cases", steps, cases)
	}
}

// TestRender_StandaloneAndWorlds_AreNotEachOthersTable holds the two indexes
// apart: one names registered tools and lists the cases that reach each, the
// other names fixture recipes and counts them, so serving either table from
// the other's index renders a table of the same shape in the same place.
func TestRender_StandaloneAndWorlds_AreNotEachOthersTable(t *testing.T) {
	page := renderedLedger(t)
	for _, row := range markdownRows(t, page, "Standalone tools the corpus names") {
		if !strings.HasPrefix(row[0], "`gitlab_") {
			t.Errorf("the standalone table names %s, which is not a registered tool", row[0])
		}
		if _, err := strconv.Atoi(row[1]); err == nil {
			t.Errorf("the standalone table counts the cases of %s rather than naming them", row[0])
		}
	}
	for _, row := range markdownRows(t, page, "Worlds the corpus asks for") {
		if strings.HasPrefix(row[0], "`gitlab_") {
			t.Errorf("the worlds table names %s, which is a registered tool rather than a recipe", row[0])
		}
		if _, err := strconv.Atoi(row[1]); err != nil {
			t.Errorf("the worlds table gives %q as the case count of %s, want a number", row[1], row[0])
		}
	}
}

// checkoutOfItsOwn plants a module root and runs the test from it, so the
// command resolves that directory as the repository and writes its ledger
// there rather than into this checkout.
func checkoutOfItsOwn(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture go.mod: %v", err)
	}
	t.Chdir(root)
	return root
}

// devNull is the stderr main writes to while a test drives it, since main
// takes no writer and its own failures are asserted through the exit code.
func devNull(t *testing.T) *os.File {
	t.Helper()
	file, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// renderedLedger renders the page from the corpus and the catalog this tree
// carries, which is what the command writes, so what is asserted of it holds
// on a run that never reads the committed artifact.
func renderedLedger(t *testing.T) string {
	t.Helper()
	catalog, err := readCatalog()
	if err != nil {
		t.Fatalf("reading the catalog: %v", err)
	}
	page, err := render(catalog)
	if err != nil {
		t.Fatalf("rendering the ledger: %v", err)
	}
	return string(page)
}

// markdownRows returns the body rows of the table under heading, each split
// into trimmed cells, so an assertion names a column rather than matching a
// substring of the whole page.
func markdownRows(t *testing.T, page, heading string) [][]string {
	t.Helper()
	_, below, found := strings.Cut(page, "## "+heading+"\n")
	if !found {
		t.Fatalf("the ledger has no %q section", heading)
	}
	section, _, _ := strings.Cut(below, "\n## ")

	var rows [][]string
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for index := range cells {
			cells[index] = strings.TrimSpace(cells[index])
		}
		rows = append(rows, cells)
	}
	if len(rows) < 3 {
		t.Fatalf("the %q table has %d lines, want a header, an alignment row and a body", heading, len(rows))
	}
	return rows[2:]
}

// share reads a "N of M" cell as its two numbers.
func share(t *testing.T, cell string) (int, int) {
	t.Helper()
	reached, held, found := strings.Cut(cell, " of ")
	if !found {
		t.Fatalf("the cell %q is not a share", cell)
	}
	return mustAtoi(t, reached), mustAtoi(t, held)
}

// mustAtoi reads a table cell that has to be a number.
func mustAtoi(t *testing.T, cell string) int {
	t.Helper()
	value, err := strconv.Atoi(cell)
	if err != nil {
		t.Fatalf("the cell %q is not a number: %v", cell, err)
	}
	return value
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
