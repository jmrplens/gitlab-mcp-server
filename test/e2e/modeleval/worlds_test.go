//go:build e2e

// worlds_test.go is the offline half of the join between the corpus and the
// fixture library.
//
// What it can check without a GitLab is the shape of the join: that every
// recipe a case names has a builder, that no builder exists for a recipe
// nothing names, that every recipe has been triaged into exactly one of the
// two verification tables, and that the fact comparison BuildWorld runs at run
// time says the right thing about the three ways a builder can disagree with
// the corpus.
//
// What it cannot check offline is the one thing a reader most wants: that each
// builder really produces the facts its recipe promises. Producing a fact
// means creating an object on a GitLab, so that comparison lives inside
// BuildWorld and fails the test that asked for the world. The check itself is
// a pure function, and it is driven here.

package modeleval

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
)

// TestWorlds_EveryRecipeTheCorpusNamesHasABuilder is the join in the direction
// that breaks a run: a case whose world nothing can raise would be sent to a
// model with a prompt full of render errors, or skipped silently.
func TestWorlds_EveryRecipeTheCorpusNamesHasABuilder(t *testing.T) {
	for _, recipe := range modelcorpus.Recipes() {
		t.Run(string(recipe), func(t *testing.T) {
			if _, known := builders[recipe]; !known {
				t.Errorf("the corpus names recipe %q and this package has no builder for it", recipe)
			}
		})
	}
}

// TestWorlds_EveryBuilderIsNamedByTheCorpus is the other direction: a builder
// for a recipe no case names is a world nobody asked for, and it goes stale
// where nothing reads it.
func TestWorlds_EveryBuilderIsNamedByTheCorpus(t *testing.T) {
	named := map[modelcorpus.Recipe]bool{}
	for _, recipe := range modelcorpus.Recipes() {
		named[recipe] = true
	}
	for _, recipe := range Recipes() {
		t.Run(string(recipe), func(t *testing.T) {
			if !named[recipe] {
				t.Errorf("this package builds recipe %q, which the corpus does not name", recipe)
			}
		})
	}
}

// TestWorlds_EveryRecipeIsTriagedForVerification holds the two verification
// tables to a partition of the corpus's recipes.
//
// It is the gate the verification judgement rests on: a recipe added to the
// corpus cannot reach a run without somebody deciding whether its world can be
// read back, and a recipe whose cases changed cannot keep a stale Verify
// without the reason beside it being edited.
func TestWorlds_EveryRecipeIsTriagedForVerification(t *testing.T) {
	for _, recipe := range modelcorpus.Recipes() {
		t.Run(string(recipe), func(t *testing.T) {
			assertion, verified := verifiedRecipes[recipe]
			reason, unverified := unverifiedRecipes[recipe]
			switch {
			case verified && unverified:
				t.Errorf("recipe %q is in both verification tables: %q and %q", recipe, assertion, reason)
			case !verified && !unverified:
				t.Errorf("recipe %q is in neither verification table: say what its Verify asserts, or why it has none", recipe)
			case verified && assertion == "":
				t.Errorf("recipe %q is declared verified and says nothing about what its Verify asserts", recipe)
			case unverified && reason == "":
				t.Errorf("recipe %q is declared unverified and gives no reason", recipe)
			}
		})
	}
}

// TestWorlds_AVerificationTableEntryNamesARecipeTheCorpusHas refuses a stale
// declaration, which is the rule every declaration table in this repository is
// held to: an entry that describes nothing is worse than no entry, because a
// reader counts it.
func TestWorlds_AVerificationTableEntryNamesARecipeTheCorpusHas(t *testing.T) {
	named := map[modelcorpus.Recipe]bool{}
	for _, recipe := range modelcorpus.Recipes() {
		named[recipe] = true
	}
	tables := map[string]map[modelcorpus.Recipe]string{
		"verified":   verifiedRecipes,
		"unverified": unverifiedRecipes,
	}
	for label, table := range tables {
		for recipe := range table {
			t.Run(label+"/"+string(recipe), func(t *testing.T) {
				if !named[recipe] {
					t.Errorf("the %s table names recipe %q, which the corpus does not have", label, recipe)
				}
			})
		}
	}
}

// TestWorlds_InteractiveRecipesAreTheOnesWhoseCasesElicit pins the one
// property the runner reads before it opens an Env.
//
// It is asserted as a set rather than derived, because deriving it would mean
// reading the keys, which this package may not do: which cases drive an
// interactive flow is in the answer, not in the stimulus. The set is small and
// a new interactive case is a deliberate act, so a list that has to be edited
// beside it is the honest shape.
func TestWorlds_InteractiveRecipesAreTheOnesWhoseCasesElicit(t *testing.T) {
	wantInteractive := map[modelcorpus.Recipe]bool{
		modelcorpus.RecipeWorld:              true,
		modelcorpus.RecipeProject:            true,
		modelcorpus.RecipeMergeRequestSource: true,
	}
	for _, recipe := range modelcorpus.Recipes() {
		t.Run(string(recipe), func(t *testing.T) {
			if got := Interactive(recipe); got != wantInteractive[recipe] {
				t.Errorf("Interactive(%q) = %t, want %t", recipe, got, wantInteractive[recipe])
			}
		})
	}
}

// TestFactsProblem_Disagreements names each of the three ways a builder can
// disagree with the corpus, which is the check BuildWorld runs against every
// world it raises and the one this file cannot run per recipe.
func TestFactsProblem_Disagreements(t *testing.T) {
	promised, known := modelcorpus.Facts(modelcorpus.RecipeSnippet)
	if !known || len(promised) == 0 {
		t.Fatalf("the corpus promises nothing for the snippet recipe: %v", promised)
	}
	complete := map[string][]string{}
	for _, key := range promised {
		complete[key] = []string{"a value"}
	}

	cases := []struct {
		name    string
		recipe  modelcorpus.Recipe
		facts   map[string][]string
		wantHas string
	}{
		{name: "agreed", recipe: modelcorpus.RecipeSnippet, facts: complete},
		{
			name: "missing", recipe: modelcorpus.RecipeSnippet, facts: map[string][]string{},
			wantHas: "produces nothing for " + promised[0],
		},
		{
			name:   "empty is missing",
			recipe: modelcorpus.RecipeSnippet,
			facts:  map[string][]string{promised[0]: {}},

			wantHas: "produces nothing for " + promised[0],
		},
		{
			name:   "surplus",
			recipe: modelcorpus.RecipeSnippet,
			facts:  with(complete, map[string][]string{"invented_key": {"a value"}}),

			wantHas: "produces invented_key, which it does not promise",
		},
		{
			name:    "missing and surplus",
			recipe:  modelcorpus.RecipeSnippet,
			facts:   map[string][]string{"invented_key": {"a value"}},
			wantHas: "and produces invented_key",
		},
		{
			name:    "unknown recipe",
			recipe:  "no-such-recipe",
			facts:   complete,
			wantHas: "the corpus names no such recipe",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := factsProblem(tc.recipe, tc.facts)
			if tc.wantHas == "" {
				if got != "" {
					t.Errorf("factsProblem() = %q, want no problem", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantHas) {
				t.Errorf("factsProblem() = %q, want it to mention %q", got, tc.wantHas)
			}
		})
	}
}

// TestWith_MergesWithoutTouchingWhatItWasGiven pins the property every builder
// composes its facts on: a recipe that merged into the map another recipe was
// handed would change that recipe's world too.
func TestWith_MergesWithoutTouchingWhatItWasGiven(t *testing.T) {
	base := map[string][]string{"a": {"1"}}
	merged := with(base, map[string][]string{"b": {"2"}})

	if len(base) != 1 {
		t.Errorf("with() wrote into the map it was given: %v", base)
	}
	if len(merged) != 2 || merged["a"][0] != "1" || merged["b"][0] != "2" {
		t.Errorf("with() = %v, want both entries", merged)
	}
}

// TestElicitedValue_AnswersEachKindOfProperty covers the responder's three
// decisions: a confirmation is accepted, a choice takes the first option the
// server offered, and the fields that name what is being created take the
// attempt's own unique name.
func TestElicitedValue_AnswersEachKindOfProperty(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		property map[string]any
		want     any
	}{
		{name: "confirmation", key: "confirmed", want: true},
		{
			name: "choice", key: "selection",
			property: map[string]any{"enum": []any{"first", "second"}}, want: "first",
		},
		{name: "choice with no options", key: "selection", want: "default"},
		{name: "name", key: "name", want: "unique-name"},
		{name: "title", key: "title", want: "unique-name"},
		{name: "description", key: "description", want: "Created by the model evaluation's scripted elicitation responder"},
		{name: "anything else", key: "labels", want: "e2e-labels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := elicitedValue(tc.key, tc.property, "unique-name"); got != tc.want {
				t.Errorf("elicitedValue(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

// TestElicitedContent_TheWorldAnswersBeforeTheSchemaDoes is the property the
// two interactive flows that name existing objects depend on: a source branch
// and a tag are things GitLab already holds, and an answer invented from the
// schema is one GitLab refuses. The unique name still answers everything the
// world says nothing about.
func TestElicitedContent_TheWorldAnswersBeforeTheSchemaDoes(t *testing.T) {
	request := elicitRequestFor("source_branch", "target_branch", "title", "confirmed")
	fromTheWorld := map[string]string{"source_branch": "feature/eval", "target_branch": "main"}

	content := elicitedContent(request, "unique-name", fromTheWorld)

	want := map[string]any{
		"source_branch": "feature/eval",
		"target_branch": "main",
		"title":         "unique-name",
		"confirmed":     true,
	}
	if !maps.Equal(content, want) {
		t.Errorf("elicitedContent() = %v, want %v", content, want)
	}
}

// TestElicitedContent_NothingToAnswer covers the five shapes a request can
// have that name no property this responder can fill, each of which is an
// empty answer rather than a panic.
func TestElicitedContent_NothingToAnswer(t *testing.T) {
	cases := []struct {
		name    string
		request *mcp.ElicitRequest
	}{
		{name: "no request"},
		{name: "no params", request: &mcp.ElicitRequest{}},
		{
			name:    "a schema that is not an object",
			request: &mcp.ElicitRequest{Params: &mcp.ElicitParams{RequestedSchema: "not a schema"}},
		},
		{
			name: "an object with no properties",
			request: &mcp.ElicitRequest{Params: &mcp.ElicitParams{
				RequestedSchema: map[string]any{"type": "object"},
			}},
		},
		{
			name: "a property that is not an object",
			request: &mcp.ElicitRequest{Params: &mcp.ElicitParams{
				RequestedSchema: map[string]any{"properties": map[string]any{"title": "not a property"}},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if content := elicitedContent(tc.request, "unique-name", nil); len(content) != 0 {
				t.Errorf("elicitedContent() = %v, want nothing", content)
			}
		})
	}
}

// TestElicitationResponder_Accepts checks the one thing the responder adds to
// the content: every elicitation of an evaluation attempt is accepted, since a
// declined one would report the flow as refused by the user.
func TestElicitationResponder_Accepts(t *testing.T) {
	respond := elicitationResponder("unique-name", map[string]string{"tag_name": "v1.2.3"})

	result, err := respond(t.Context(), elicitRequestFor("tag_name"))
	if err != nil {
		t.Fatalf("the responder returned an error: %v", err)
	}
	if result.Action != "accept" {
		t.Errorf("the responder answered %q, want accept", result.Action)
	}
	if result.Content["tag_name"] != "v1.2.3" {
		t.Errorf("the responder answered tag_name = %v, want the world's tag", result.Content["tag_name"])
	}
}

// elicitRequestFor builds the request a flow asking for these properties
// sends, which is the shape the wizard's own schema has: an object whose
// properties are strings.
func elicitRequestFor(keys ...string) *mcp.ElicitRequest {
	properties := map[string]any{}
	for _, key := range keys {
		properties[key] = map[string]any{"type": "string"}
	}
	return &mcp.ElicitRequest{Params: &mcp.ElicitParams{
		RequestedSchema: map[string]any{"type": "object", "properties": properties},
	}}
}

// TestStillThere_Verdicts covers the helper every predicate-backed
// verification is written through: the object being gone is the case having
// done its work, the object being there is the failure, and a read that could
// not be made is neither.
func TestStillThere_Verdicts(t *testing.T) {
	cases := []struct {
		name    string
		present bool
		err     error
		wantHas string
	}{
		{name: "gone"},
		{name: "still there", present: true, wantHas: "the tag is still there"},
		{name: "unreadable", err: errors.New("403 Forbidden"), wantHas: "403 Forbidden"},
		{
			name:    "unreadable and reported present",
			present: true, err: errors.New("403 Forbidden"),
			wantHas: "403 Forbidden",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := stillThere("the tag", tc.present, tc.err)
			assertVerdict(t, "stillThere", err, tc.wantHas)
		})
	}
}

// TestRemoved_Verdicts covers the same three verdicts read off a plain GitLab
// read rather than off a predicate.
func TestRemoved_Verdicts(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantHas string
	}{
		{name: "gone", err: gl.ErrNotFound},
		{name: "gone, as a response", err: statusError(http.StatusNotFound, "404 Release Not Found")},
		{name: "still there", wantHas: "the release is still there"},
		{
			name:    "unreadable",
			err:     statusError(http.StatusForbidden, "403 Forbidden"),
			wantHas: "reading the release back",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := removed("the release", tc.err)
			assertVerdict(t, "removed", err, tc.wantHas)
		})
	}
}

// statusError builds the structured error client-go returns for an HTTP
// refusal, with the request a real one carries so its message formats.
func statusError(code int, message string) error {
	request := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Scheme: "https", Host: "gitlab.example", Path: "/api/v4/projects/1"},
	}
	return &gl.ErrorResponse{
		Response: &http.Response{StatusCode: code, Request: request},
		Message:  message,
	}
}

// assertVerdict holds one verification verdict to what it should say: nothing
// when the change happened, and a message naming the reason otherwise.
func assertVerdict(t *testing.T, helper string, err error, wantHas string) {
	t.Helper()
	if wantHas == "" {
		if err != nil {
			t.Errorf("%s() = %v, want no verdict", helper, err)
		}
		return
	}
	if err == nil {
		t.Fatalf("%s() reported nothing, want a verdict mentioning %q", helper, wantHas)
	}
	if !strings.Contains(err.Error(), wantHas) {
		t.Errorf("%s() = %v, want it to mention %q", helper, err, wantHas)
	}
}
