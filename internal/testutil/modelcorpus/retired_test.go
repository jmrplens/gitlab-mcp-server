package modelcorpus

import (
	"slices"
	"testing"
)

// The retirement table's own gate. A retired identifier is the only trace an
// old report's reader has of a case that is gone, so the table has to stay
// readable by someone who was not here: every entry names a category that
// exists and gives a reason, no entry contradicts the live corpus, and no
// number is ever handed to a second case.

// TestRetired_NoneIsAlsoLive is the rule that makes the table mean anything. A
// case declared retired and still in the corpus would leave a reader holding
// two contradictory answers about the same identifier, and neither of them
// would be wrong on its own terms.
func TestRetired_NoneIsAlsoLive(t *testing.T) {
	live := map[string]bool{}
	for _, id := range IDs() {
		live[id] = true
	}
	for _, one := range Retired() {
		if live[one.ID] {
			t.Errorf("%s is retired as %s and is also a live case", one.ID, one.Category)
		}
	}
}

// TestRetired_EachOneIsWellFormed holds every entry to the shape a reader
// needs: an identifier, one of the three categories, and a reason.
func TestRetired_EachOneIsWellFormed(t *testing.T) {
	for _, one := range Retired() {
		t.Run(one.ID, func(t *testing.T) {
			if problem := retirementProblem(one); problem != "" {
				t.Error(problem)
			}
			if !caseIDPattern.MatchString(one.ID) {
				t.Errorf("%s does not spell an identifier of this corpus", one.ID)
			}
		})
	}
}

// TestRetirementProblem_NamesWhatIsWrong covers the predicate on each way an
// entry can be unusable, so a table that has never been wrong is still known
// to be checked.
func TestRetirementProblem_NamesWhatIsWrong(t *testing.T) {
	tests := []struct {
		name string
		one  Retirement
		want bool
	}{
		{
			name: "complete",
			one:  Retirement{ID: "MT-001", Category: RetiredBridgeTool, Reason: "because"},
		},
		{name: "no identifier", one: Retirement{Category: RetiredBridgeTool, Reason: "because"}, want: true},
		{name: "no category", one: Retirement{ID: "MT-001", Reason: "because"}, want: true},
		{
			name: "invented category",
			one:  Retirement{ID: "MT-001", Category: "because-i-said-so", Reason: "because"},
			want: true,
		},
		{name: "no reason", one: Retirement{ID: "MT-001", Category: RetiredNoRecipe}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problem := retirementProblem(tc.one)
			if tc.want && problem == "" {
				t.Errorf("retirementProblem(%+v) accepted it, want a complaint", tc.one)
			}
			if !tc.want && problem != "" {
				t.Errorf("retirementProblem(%+v) = %q, want none", tc.one, problem)
			}
		})
	}
}

// TestRetired_EachNumberIsAllocatedOnce checks the other half of the
// allocation rule: the table itself must not name one case twice, or a reader
// looking an identifier up would get whichever entry came first.
func TestRetired_EachNumberIsAllocatedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, one := range Retired() {
		if seen[one.ID] {
			t.Errorf("%s is retired twice", one.ID)
		}
		seen[one.ID] = true
	}
}

// TestRetired_CoversTheTwoClassesTheMoveCouldDecide pins what this table is
// for. The survivor rule has three clauses; two of them could be decided by
// reading the old corpus, and the third needs a builder to have been tried.
// A no-recipe entry appearing here before the fixture step exists would be a
// decision written before its evidence, so the category is declared and unused
// until then.
func TestRetired_CoversTheTwoClassesTheMoveCouldDecide(t *testing.T) {
	counts := map[string]int{}
	for _, one := range Retired() {
		counts[one.Category]++
	}
	if got := counts[RetiredBridgeTool]; got != 10 {
		t.Errorf("%d bridge-tool retirements, want the 10 the port found", got)
	}
	if got := counts[RetiredSimulatedResult]; got != 4 {
		t.Errorf("%d simulated-result retirements, want the 4 the port found", got)
	}
	if got := len(Retired()); got != 14 {
		t.Errorf("%d retirements in all, want 14", got)
	}
}

// TestRetired_PublishesTheLabelAnOldReportsReaderReads holds each category to
// the word [Retired] hands out, which nothing else here can see.
//
// Every other test in this file names the categories by their constants, so the
// two that are carried by entries can have their spellings exchanged and all of
// them still pass: the counts above are read through the same constants they
// were written with. What changes is only what a reader gets, and it is the one
// thing the table exists for — MT-201 would answer "simulated-result" for a
// case whose whole reason for retiring was that it called a tool no client
// registers.
//
// A representative of each carried category is named beside the spelling, so an
// entry relabelled without its reason being rewritten fails here too.
func TestRetired_PublishesTheLabelAnOldReportsReaderReads(t *testing.T) {
	tests := []struct {
		category  string
		label     string
		witness   string
		witnessed bool
	}{
		{category: RetiredBridgeTool, label: "bridge-tool", witness: "MT-201", witnessed: true},
		{category: RetiredSimulatedResult, label: "simulated-result", witness: "MF-001", witnessed: true},
		{category: RetiredNoRecipe, label: "no-recipe"},
	}
	if len(tests) != len(retiredCategories) {
		t.Fatalf("%d categories are held to a spelling and the table admits %d",
			len(tests), len(retiredCategories))
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			if tc.category != tc.label {
				t.Errorf("the category is published as %q, want %q", tc.category, tc.label)
			}
			if !tc.witnessed {
				return
			}
			for _, one := range Retired() {
				if one.ID != tc.witness {
					continue
				}
				if one.Category != tc.label {
					t.Errorf("%s is published as %q, want %q", tc.witness, one.Category, tc.label)
				}
				return
			}
			t.Errorf("%s is no longer retired, so this category has lost its witness", tc.witness)
		})
	}
}

// TestRetired_IsACopy checks that a caller cannot edit the table through the
// slice it is handed, which is the same rule [Facts] follows and for the same
// reason: the corpus is read by tests that run in any order.
func TestRetired_IsACopy(t *testing.T) {
	first := Retired()
	if len(first) == 0 {
		t.Fatal("Retired() returned nothing")
	}
	first[0].Reason = "edited"
	if second := Retired(); second[0].Reason == "edited" {
		t.Error("Retired() handed out the table itself, and a caller edited it")
	}
}

// TestRetired_IsInDeclarationOrder keeps the published order stable, so a
// reader comparing two versions of the breadth ledger sees what changed rather
// than a reshuffle.
func TestRetired_IsInDeclarationOrder(t *testing.T) {
	got := make([]string, 0, len(retirements))
	for _, one := range Retired() {
		got = append(got, one.ID)
	}
	want := make([]string, 0, len(retirements))
	for _, one := range retirements {
		want = append(want, one.ID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Retired() reordered the table: %v, want %v", got, want)
	}
}
