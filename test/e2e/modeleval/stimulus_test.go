//go:build e2e

// stimulus_test.go checks what a model is shown, and that nothing in the file
// that builds it can see an answer.
//
// The second half is the one worth reading twice. The corpus keeps a case's
// key unexported and its own boundary test names every package allowed to call
// the accessor, which already excludes this one. This adds a second lock on
// the same door, at the file that composes the text a provider receives,
// because that is where a leak would have to live and because a test that
// names the file is a test a reviewer editing the file will see.

package modeleval

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestStimulus_TheFileThatComposesAPromptCannotReadAnAnswer parses stimulus.go
// and fails on any mention of the corpus's key accessor.
//
// It reads the syntax rather than the text, so a spelling a search would miss
// is caught too: an aliased import, a dot import, a method value taken without
// being called.
func TestStimulus_TheFileThatComposesAPromptCannotReadAnAnswer(t *testing.T) {
	const file = "stimulus.go"
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	ast.Inspect(parsed, func(node ast.Node) bool {
		ident, isIdent := node.(*ast.Ident)
		if isIdent && ident.Name == "Keys" {
			t.Errorf("%s names Keys at %v: the file that composes a stimulus may not read an answer",
				file, parsed.Name.NamePos)
		}
		return true
	})
}

// TestRenderStimulus_RendersOneSpellingAndRefusesAHole checks the two things a
// rendered prompt has to be: filled from the world, and filled completely.
func TestRenderStimulus_RendersOneSpellingAndRefusesAHole(t *testing.T) {
	one := modelcorpus.Stimulus{
		ID:     "MT-000",
		Prompt: "List the issues in " + modelcorpus.FactPlaceholder(modelcorpus.FactProjectPath) + ".",
	}
	world := World{Facts: map[string][]string{
		modelcorpus.FactProjectPath: {"org/tools", "42"},
		modelcorpus.FactProjectID:   {"42", "org/tools"},
	}}

	sent, err := renderStimulus(one, world)
	if err != nil {
		t.Fatalf("renderStimulus error = %v, want nil", err)
	}
	if sent.Text != "List the issues in org/tools." {
		t.Errorf("Text = %q, want the first spelling rendered", sent.Text)
	}
	if sent.Facts[modelcorpus.FactProjectID] != "42" {
		t.Errorf("Facts = %v, want one spelling of each", sent.Facts)
	}

	missing := modelcorpus.Stimulus{ID: "MT-001", Prompt: modelcorpus.FactPlaceholder("nothing_promises_this")}
	if _, refused := renderStimulus(missing, world); refused == nil {
		t.Errorf("renderStimulus of a prompt naming an absent fact returned no error")
	}
}

// TestRenderedFacts_SkipsAKeyWithNoSpelling checks that a fact with an empty
// list is absent rather than empty: an empty value would render as a hole in
// the sentence the model reads and the template would accept it.
func TestRenderedFacts_SkipsAKeyWithNoSpelling(t *testing.T) {
	rendered := renderedFacts(map[string][]string{"filled": {"value"}, "empty": {}})
	if rendered["filled"] != "value" {
		t.Errorf("a filled fact rendered as %q", rendered["filled"])
	}
	if _, present := rendered["empty"]; present {
		t.Errorf("a fact with no spelling reached the render as %q", rendered["empty"])
	}
}

// TestContractFor_EverySurfaceHasOneAndNothingElseDoes checks that a model is
// never given a surface with nothing said about it.
func TestContractFor_EverySurfaceHasOneAndNothingElseDoes(t *testing.T) {
	for _, surface := range harness.AllSurfaces() {
		t.Run(surface.String(), func(t *testing.T) {
			contract, err := contractFor(surface)
			if err != nil {
				t.Fatalf("contractFor(%s) error = %v", surface, err)
			}
			if strings.TrimSpace(contract) == "" {
				t.Errorf("contractFor(%s) is empty", surface)
			}
		})
	}
	if _, err := contractFor(harness.Surface("wireless")); err == nil {
		t.Errorf("contractFor of an unknown surface returned no error")
	}
}

// TestContractDigest_IsStable checks that the fingerprint two rows are compared
// by does not move on its own.
func TestContractDigest_IsStable(t *testing.T) {
	first := contractDigest()
	if first != contractDigest() {
		t.Errorf("contractDigest() returned %q and then %q", first, contractDigest())
	}
	if len(first) != digestLength {
		t.Errorf("contractDigest() = %q, %d characters, want %d", first, len(first), digestLength)
	}
}

// TestNeedsOf_TurnsACasesDeclarationIntoTheHarnessOwn checks the conversion
// that decides whether a case runs at all.
func TestNeedsOf_TurnsACasesDeclarationIntoTheHarnessOwn(t *testing.T) {
	tests := []struct {
		name  string
		needs modelcorpus.Needs
		want  []string
	}{
		{name: "nothing declared", needs: modelcorpus.Needs{}},
		{
			name:  "free is the absence of a tier need",
			needs: modelcorpus.Needs{Tier: modelcorpus.TierFree},
		},
		{
			name:  "premium",
			needs: modelcorpus.Needs{Tier: modelcorpus.TierPremium},
			want:  []string{harness.Tier(edition.Premium).String()},
		},
		{
			name:  "everything at once",
			needs: modelcorpus.Needs{Tier: modelcorpus.TierUltimate, Runner: true, Admin: true, FixtureService: true},
			want: []string{
				harness.Tier(edition.Ultimate).String(),
				harness.NeedRunner.String(),
				harness.NeedAdmin.String(),
				harness.NeedFixtureService.String(),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			declared, err := needsOf(tc.needs)
			if err != nil {
				t.Fatalf("needsOf error = %v, want nil", err)
			}
			named := make([]string, 0, len(declared))
			for _, need := range declared {
				named = append(named, need.String())
			}
			if strings.Join(named, ",") != strings.Join(tc.want, ",") {
				t.Errorf("needsOf = %v, want %v", named, tc.want)
			}
		})
	}

	if _, err := needsOf(modelcorpus.Needs{Tier: modelcorpus.Tier("diamond")}); err == nil {
		t.Errorf("needsOf of a tier nothing spells returned no error")
	}
}

// TestSurfacesFor_IsTheRunsChoiceNarrowedByTheCases checks the two ways a case
// runs on fewer than three surfaces.
func TestSurfacesFor_IsTheRunsChoiceNarrowedByTheCases(t *testing.T) {
	all := harness.AllSurfaces()
	configured := []harness.Surface{harness.SurfaceDynamic, harness.SurfaceMeta}

	unrestricted := modelcorpus.Stimulus{ID: "MT-000"}
	if got := surfacesFor(unrestricted, configured); len(got) != 2 {
		t.Errorf("an unrestricted case on a two-surface run got %v", got)
	}
	if got := surfacesFor(unrestricted, all); len(got) != 3 {
		t.Errorf("an unrestricted case on a three-surface run got %v", got)
	}

	metaOnly := modelcorpus.Stimulus{
		ID:       "MT-001",
		Surfaces: modelcorpus.Restrict{Only: []modelcorpus.Surface{modelcorpus.SurfaceMeta}, Reason: "the reason"},
	}
	got := surfacesFor(metaOnly, all)
	if len(got) != 1 || got[0] != harness.SurfaceMeta {
		t.Errorf("a meta-only case got %v", got)
	}
	if !restricted(metaOnly) || restrictionReason(metaOnly) != "the reason" {
		t.Errorf("the restriction was not read back: %t, %q", restricted(metaOnly), restrictionReason(metaOnly))
	}

	individualOnly := modelcorpus.Stimulus{
		ID:       "MT-002",
		Surfaces: modelcorpus.Restrict{Only: []modelcorpus.Surface{modelcorpus.SurfaceIndividual}, Reason: "r"},
	}
	if narrowed := surfacesFor(individualOnly, configured); len(narrowed) != 0 {
		t.Errorf("a case restricted to a surface the run does not measure got %v", narrowed)
	}
}

// TestSelectedStimuli_AsksTheCasesTheSelectionNames checks the filter a run is
// narrowed by, and the reading that says a selection asked for nothing.
func TestSelectedStimuli_AsksTheCasesTheSelectionNames(t *testing.T) {
	everything := selectedStimuli(caseSelection{})
	if len(everything) != len(modelcorpus.Stimuli()) {
		t.Errorf("the empty selection admitted %d of %d cases", len(everything), len(modelcorpus.Stimuli()))
	}
	if missing := unselectedCases(caseSelection{}, everything); len(missing) != 0 {
		t.Errorf("the empty selection left %v unasked", missing)
	}

	first := everything[0].ID
	one := selectedStimuli(caseSelection{ids: []string{first}})
	if len(one) != 1 || one[0].ID != first {
		t.Errorf("selecting %q admitted %d case(s)", first, len(one))
	}

	absent := caseSelection{ids: []string{"MS-nothing"}}
	if missing := unselectedCases(absent, selectedStimuli(absent)); len(missing) != 1 {
		t.Errorf("a selection naming nothing the corpus has left %v unasked, want one", missing)
	}
}

// TestModelName_IsSafeInASubtestNameAndKeepsTwoSpecsApart checks that two
// models cannot collapse into one name in the log.
func TestModelName_IsSafeInASubtestNameAndKeepsTwoSpecsApart(t *testing.T) {
	plain := modelName("anthropic:claude-haiku-4-5-20251001")
	optioned := modelName("anthropic:claude-haiku-4-5-20251001;temperature=default")
	if plain == optioned {
		t.Errorf("two specs differing in an option both name %q", plain)
	}
	for _, name := range []string{plain, optioned} {
		t.Run(name, func(t *testing.T) {
			if strings.ContainsAny(name, ":;= /") {
				t.Errorf("modelName produced %q, which the testing package rewrites", name)
			}
		})
	}
}

// TestAttemptID_NamesTheFiveThingsThatMakeAnAttemptDistinct checks that two
// attempts of one run cannot share an identifier.
func TestAttemptID_NamesTheFiveThingsThatMakeAnAttemptDistinct(t *testing.T) {
	base := attemptID("run-1", "MT-001", "fake:perfect", harness.SurfaceDynamic, 1)
	tests := []struct {
		name  string
		other string
	}{
		{name: "another run", other: attemptID("run-2", "MT-001", "fake:perfect", harness.SurfaceDynamic, 1)},
		{name: "another case", other: attemptID("run-1", "MT-002", "fake:perfect", harness.SurfaceDynamic, 1)},
		{name: "another model", other: attemptID("run-1", "MT-001", "fake:declines", harness.SurfaceDynamic, 1)},
		{name: "another surface", other: attemptID("run-1", "MT-001", "fake:perfect", harness.SurfaceMeta, 1)},
		{name: "another repeat", other: attemptID("run-1", "MT-001", "fake:perfect", harness.SurfaceDynamic, 2)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.other == base {
				t.Errorf("two attempts share the identifier %q", base)
			}
		})
	}
}
