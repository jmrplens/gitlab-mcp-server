package modelcorpus

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

// The corpus gate. Every rule here is one a case could break by hand and
// nothing else would notice until a paid run spent its tokens finding out: an
// action that no longer exists, an argument the schema never had, a truth
// bound to a value no recipe builds, a literal the prompt never gave the model.
//
// The oracle is the catalog itself, read at Ultimate with the MCP group in, so
// this gate says what the server registers rather than what a second list here
// claims it registers.

// actionFacts is what the catalog says about one action the corpus names.
type actionFacts struct {
	domain         string
	metaTool       string
	metaAction     string
	individualTool string
	tier           Tier
	destructive    bool
	readOnly       bool
	arguments      map[string]string
}

// catalogFacts is the catalog as the gate and the prompt audit read it.
type catalogFacts struct {
	actions      map[Action]actionFacts
	standalone   map[string]map[string]string
	groupActions map[string][]string
}

var (
	catalogOnce sync.Once
	loadedFacts catalogFacts
	errCatalog  error
)

// catalog reads the shared base catalog once per test binary. Once, because
// building it is the expensive part of every test in this file and the answer
// cannot change between them.
func catalog(t *testing.T) catalogFacts {
	t.Helper()
	catalogOnce.Do(func() { loadedFacts, errCatalog = readCatalog() })
	if errCatalog != nil {
		t.Fatalf("reading the action catalog: %v", errCatalog)
	}
	return loadedFacts
}

// readCatalog builds the catalog the binary registers from and indexes what
// the corpus asks of it.
func readCatalog() (catalogFacts, error) {
	built, err := gitlabtools.SharedBaseCatalog(false, gitlabtools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		return catalogFacts{}, err
	}
	facts := catalogFacts{
		actions:      make(map[Action]actionFacts, built.CountActions()),
		standalone:   map[string]map[string]string{},
		groupActions: map[string][]string{},
	}
	for _, action := range built.Actions() {
		facts.actions[Action(action.ID)] = actionFacts{
			domain:         action.Domain,
			metaTool:       action.ToolName,
			metaAction:     action.Name,
			individualTool: strings.TrimSpace(action.IndividualTool.Name),
			tier:           Tier(edition.TierFromEdition(action.Edition).String()),
			destructive:    action.Destructive,
			readOnly:       action.ReadOnly,
			arguments:      schemaProperties(action.Route.InputSchema),
		}
		facts.groupActions[action.ToolName] = append(facts.groupActions[action.ToolName], action.Name)
	}
	for _, spec := range gitlabtools.StandaloneSurfaceToolSpecs(nil) {
		facts.standalone[spec.Name] = schemaProperties(spec.Route.InputSchema)
	}
	return facts, nil
}

// schemaProperties reads the top-level properties of an input schema: the
// argument names a call may carry, each with the one scalar type its schema
// declares.
//
// The type is empty wherever the schema does not state exactly one: an
// argument that takes either spelling of a project declares ["string",
// "integer"], and a value is judged against that by the server rather than
// here.
func schemaProperties(schema map[string]any) map[string]string {
	names := map[string]string{}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return names
	}
	for name, property := range properties {
		declared, _ := property.(map[string]any)
		scalar, _ := declared["type"].(string)
		names[name] = scalar
	}
	return names
}

// arguments returns the argument names a step's tool accepts, each with the
// scalar type its schema declares, and whether the tool is one the server
// registers at all.
func (f catalogFacts) arguments(one Step) (map[string]string, bool) {
	if one.Standalone != "" {
		names, known := f.standalone[one.Standalone]
		return names, known
	}
	action, known := f.actions[one.Action]
	return action.arguments, known
}

// TestCases_EveryStepNamesSomethingTheServerRegisters checks each step against
// the catalog and the standalone surface tools: an action that is not in the
// catalog at Ultimate, or a standalone tool that is not registered, is a case
// that can never be reached on any surface.
func TestCases_EveryStepNamesSomethingTheServerRegisters(t *testing.T) {
	facts := catalog(t)
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			for index, declared := range one.key.Steps {
				switch {
				case declared.Action != "" && declared.Standalone != "":
					t.Errorf("step %d names both action %q and standalone tool %q; exactly one is a call",
						index+1, declared.Action, declared.Standalone)
				case declared.Action == "" && declared.Standalone == "":
					t.Errorf("step %d names neither an action nor a standalone tool", index+1)
				case declared.Standalone != "":
					if _, known := facts.standalone[declared.Standalone]; !known {
						t.Errorf("step %d names standalone tool %q, which the server does not register",
							index+1, declared.Standalone)
					}
				default:
					if _, known := facts.actions[declared.Action]; !known {
						t.Errorf("step %d names action %q, which is not in the catalog at Ultimate",
							index+1, declared.Action)
					}
				}
			}
		})
	}
}

// TestCases_EveryArgumentIsAPropertyOfItsSchema checks each argument name
// against the input schema of the tool the step calls. An argument the schema
// does not carry is a value the server would refuse, and the old corpus
// carried several: the name was written by hand and compared with nothing.
func TestCases_EveryArgumentIsAPropertyOfItsSchema(t *testing.T) {
	facts := catalog(t)
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			for index, declared := range one.key.Steps {
				accepted, known := facts.arguments(declared)
				if !known {
					continue // reported by the step test
				}
				seen := map[string]bool{}
				for _, arg := range declared.Args {
					if _, isProperty := accepted[arg.Name]; !isProperty {
						t.Errorf("step %d passes %q, which is not a property of %s's input schema",
							index+1, arg.Name, stepName(declared))
					}
					if seen[arg.Name] {
						t.Errorf("step %d declares %q twice", index+1, arg.Name)
					}
					seen[arg.Name] = true
				}
			}
		})
	}
}

// TestCases_EveryTruthIsExactlyOneOfFour checks the closed sum: a truth that
// sets two sources is ambiguous to the scorer, and one that sets none compares
// a value against nothing while still counting in the fidelity denominator.
func TestCases_EveryTruthIsExactlyOneOfFour(t *testing.T) {
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			for index, declared := range one.key.Steps {
				for _, arg := range declared.Args {
					checkTruth(t, one.key, index, arg)
				}
			}
		})
	}
}

// checkTruth holds one argument to the closed sum and, where it binds to an
// earlier step's answer, to a step and a field that step says it produces.
func checkTruth(t *testing.T, key Key, index int, arg Arg) {
	t.Helper()
	if sources := truthSources(arg.Truth); sources != 1 {
		t.Errorf("step %d argument %q sets %d truth sources, want exactly 1", index+1, arg.Name, sources)
	}
	if arg.Truth.Produced.IsZero() {
		return
	}
	if arg.Truth.Produced.Step < 1 || arg.Truth.Produced.Step > index {
		t.Errorf("step %d argument %q binds to step %d, which is not an earlier step",
			index+1, arg.Name, arg.Truth.Produced.Step)
		return
	}
	earlier := key.Steps[arg.Truth.Produced.Step-1]
	if !slices.Contains(earlier.Produces, arg.Truth.Produced.Field) {
		t.Errorf("step %d argument %q binds to field %q, which step %d does not produce",
			index+1, arg.Name, arg.Truth.Produced.Field, arg.Truth.Produced.Step)
	}
}

// truthSources counts how many of the four sources a truth sets.
func truthSources(truth Truth) int {
	count := 0
	for _, set := range []bool{truth.Fact != "", truth.Literal != "", !truth.Produced.IsZero(), truth.Authored} {
		if set {
			count++
		}
	}
	return count
}

// TestCases_EveryFactIsPromisedByTheRecipeAndInterpolated checks both halves
// of the fact contract. A truth may only name a key the case's own recipe
// builds, and the prompt must interpolate that key: a template that hard-codes
// what a recipe produces is a case bound to one run of one fixture, which is
// the defect the two-text case shape had, read from the other end.
func TestCases_EveryFactIsPromisedByTheRecipeAndInterpolated(t *testing.T) {
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			promised, known := Facts(one.Recipe)
			if !known {
				t.Fatalf("names recipe %q, which no builder promises facts for", one.Recipe)
			}
			interpolated, err := InterpolatedFacts(one.Prompt)
			if err != nil {
				t.Fatalf("reading the prompt template: %v", err)
			}
			for _, key := range interpolated {
				if !slices.Contains(promised, key) {
					t.Errorf("the prompt interpolates fact %q, which recipe %q does not promise",
						key, one.Recipe)
				}
			}
			for index, declared := range one.key.Steps {
				for _, arg := range declared.Args {
					checkFactTruth(t, one, index, arg, promised, interpolated)
				}
			}
		})
	}
}

// checkFactTruth holds one fact-bound argument to both halves of the contract:
// the recipe promises the key, and the prompt gives the model its value.
func checkFactTruth(t *testing.T, one Case, index int, arg Arg, promised, interpolated []string) {
	t.Helper()
	if arg.Truth.Fact == "" {
		return
	}
	if !slices.Contains(promised, arg.Truth.Fact) {
		t.Errorf("step %d argument %q binds to fact %q, which recipe %q does not promise",
			index+1, arg.Name, arg.Truth.Fact, one.Recipe)
	}
	if !slices.Contains(interpolated, arg.Truth.Fact) {
		t.Errorf("step %d argument %q binds to fact %q, which the prompt never gives the model: "+
			"it must interpolate %s", index+1, arg.Name, arg.Truth.Fact, FactPlaceholder(arg.Truth.Fact))
	}
}

// TestCases_EveryLiteralAppearsInTheRenderedPrompt checks that a value the key
// compares exactly is a value the stimulus stated. Otherwise the case expects
// something it never asked for, which is the defect that made the old corpus's
// argument figures unreadable.
//
// The comparison is case-insensitive because a prompt states a value in the
// sentence's own capitalisation: "Close issue 7" gives the model the value
// GitLab spells "close", and holding the case to the API's capitalisation
// would be holding the prose to the wrong oracle.
func TestCases_EveryLiteralAppearsInTheRenderedPrompt(t *testing.T) {
	facts := auditFacts()
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			rendered, err := Render(one.Prompt, facts)
			if err != nil {
				t.Fatalf("rendering the prompt: %v", err)
			}
			lowered := strings.ToLower(rendered)
			for index, declared := range one.key.Steps {
				for _, arg := range declared.Args {
					if arg.Truth.Literal == "" {
						continue
					}
					if !strings.Contains(lowered, strings.ToLower(arg.Truth.Literal)) {
						t.Errorf("step %d argument %q expects literal %q, which the prompt never states: %s",
							index+1, arg.Name, arg.Truth.Literal, rendered)
					}
				}
			}
		})
	}
}

// TestCases_EveryBoundValueFitsItsArgumentsType is the half of the fact
// contract nothing checked until MT-007 was found binding a group's path to an
// argument the schema types as an integer.
//
// A fact is a key whose accepted spellings are the recipe's business, so
// binding one says nothing about what the model is handed: the prompt
// interpolates the single value the fact renders as, and an argument declared a
// number refuses a path whatever else a scorer would accept for it. A case in
// that state has no completion at all, since the model can only fail with
// invalid_params or invent a lookup call the key does not name, and every rule
// above it passes: the action exists, the argument is a property of its schema,
// the truth is exactly one of four and the prompt interpolates the fact.
//
// It is judged against the offline table the prompt audit renders with, and
// only where the schema declares exactly one scalar type.
func TestCases_EveryBoundValueFitsItsArgumentsType(t *testing.T) {
	facts := catalog(t)
	values := auditFacts()
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			for index, declared := range one.key.Steps {
				accepted, known := facts.arguments(declared)
				if !known {
					continue // reported by the step test
				}
				for _, arg := range declared.Args {
					if finding := scalarTypeFinding(arg, accepted[arg.Name], values); finding != "" {
						t.Errorf("step %d %s", index+1, finding)
					}
				}
			}
		})
	}
}

// scalarTypeFinding complains about an argument whose bound value its schema's
// declared type refuses, and says nothing about every other one.
//
// Only a numeric type is judged. A string takes any value a corpus binds, and
// an argument that declares no single type takes whichever spellings the server
// accepts for it, which is the server's own question and not this one.
func scalarTypeFinding(arg Arg, declaredType string, values map[string]string) string {
	if declaredType != "integer" && declaredType != "number" {
		return ""
	}
	value, bound := boundValue(arg, values)
	if !bound || isNumber(value) {
		return ""
	}
	return fmt.Sprintf("argument %q is typed %s and binds to %q, which the schema refuses: "+
		"that value is what the prompt gives the model and it has no other", arg.Name, declaredType, value)
}

// boundValue returns the value a bound argument would carry offline, and
// whether the corpus knows it: a value an earlier step produced or the model
// authored is known to neither the corpus nor this rule.
func boundValue(arg Arg, values map[string]string) (string, bool) {
	switch {
	case arg.Truth.Fact != "":
		value, known := values[arg.Truth.Fact]
		return value, known
	case arg.Truth.Literal != "":
		return arg.Truth.Literal, true
	default:
		return "", false
	}
}

// isNumber reports whether a value is one a numeric schema accepts.
func isNumber(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

// TestScalarTypeFinding_ReportsTheBindingMT007Had proves the rule can fail, on
// the binding it was written against: group.create types parent_id as an
// integer, and the group path fact renders "my-org". A gate that has never
// reported anything is a gate nobody has any reason to believe.
func TestScalarTypeFinding_ReportsTheBindingMT007Had(t *testing.T) {
	tests := []struct {
		name         string
		arg          Arg
		declaredType string
		want         bool
	}{
		{
			name:         "a path where the schema takes an integer",
			arg:          req("parent_id", fact(FactGroupPath)),
			declaredType: "integer",
			want:         true,
		},
		{
			name:         "the numeric fact it was fixed to",
			arg:          req("parent_id", fact(FactGroupID)),
			declaredType: "integer",
		},
		{
			name:         "a literal no number can be read out of",
			arg:          req("parent_id", literal("my-org")),
			declaredType: "number",
			want:         true,
		},
		{name: "a string argument takes a path", arg: project(), declaredType: "string"},
		{name: "an argument declaring no single type", arg: project()},
		{name: "a value the model authors", arg: req("title", authored()), declaredType: "integer"},
		{
			name:         "a value an earlier step produced",
			arg:          Arg{Name: "parent_id", Truth: Truth{Produced: Ref{Step: 1, Field: "id"}}},
			declaredType: "integer",
		},
		{
			name:         "a fact with no offline value",
			arg:          req("parent_id", fact("no_such_fact")),
			declaredType: "integer",
		},
	}
	values := auditFacts()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finding := scalarTypeFinding(tc.arg, tc.declaredType, values)
			if tc.want && finding == "" {
				t.Fatalf("scalarTypeFinding(%+v, %q) said nothing, want a finding", tc.arg, tc.declaredType)
			}
			if !tc.want && finding != "" {
				t.Errorf("scalarTypeFinding(%+v, %q) = %q, want nothing", tc.arg, tc.declaredType, finding)
			}
		})
	}
}

// TestCases_NeedsCoverTheTierEveryStepAsks checks that a case declares at
// least the tier its steps need. A case whose step is Premium and whose needs
// say Free runs on a Free instance, is refused by the server, and reads as a
// model failure.
func TestCases_NeedsCoverTheTierEveryStepAsks(t *testing.T) {
	facts := catalog(t)
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			declared := one.Needs.MinimumTier()
			if tierRank(declared) < 0 {
				t.Fatalf("declares tier %q, which is not one of free, premium or ultimate", declared)
			}
			for index, oneStep := range one.key.Steps {
				action, known := facts.actions[oneStep.Action]
				if !known {
					continue // reported by the step test
				}
				if tierRank(action.tier) > tierRank(declared) {
					t.Errorf("step %d needs tier %s and the case declares %s",
						index+1, action.tier, declared)
				}
			}
		})
	}
}

// adminModeScope is the PAT scope the catalog's scope filter reads, spelled
// here so this rule and [gitlabtools.MetaToolScopes] are talking about the
// same thing.
const adminModeScope = "admin_mode"

// TestCases_NeedsAdminWhereverTheScopeFilterWithholdsTheGroup is the rule
// beside the tier one, against the other thing that can deny a case its action
// before any model sees it.
//
// [gitlabtools.MetaToolScopes] withholds five catalog groups whole from a
// token with no admin_mode, and the filter is applied to the catalog rather
// than to registered tool names, so the removal reaches all three surfaces: on
// dynamic and meta the group a caller would name is gone, and on individual
// every tool projected from it is. A case naming one of those actions without
// Needs.Admin therefore runs on a non-admin token against a surface that never
// registered the action, and is scored as a model failure rather than skipped
// by the harness.
//
// It is the same class of defect the tier rule catches, read from the other
// side: there the instance cannot serve the action, here the server does not
// offer it.
func TestCases_NeedsAdminWhereverTheScopeFilterWithholdsTheGroup(t *testing.T) {
	facts := catalog(t)
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			if one.Needs.Admin {
				return
			}
			for index, oneStep := range one.key.Steps {
				action, known := facts.actions[oneStep.Action]
				if !known {
					continue // reported by the step test
				}
				if !groupNeedsAdminMode(action.metaTool) {
					continue
				}
				t.Errorf("step %d calls %s, whose catalog group %s the scope filter withholds from a "+
					"token without %s, and the case does not declare Needs.Admin",
					index+1, oneStep.Action, action.metaTool, adminModeScope)
			}
		})
	}
}

// groupNeedsAdminMode reports whether the scope filter withholds a catalog
// group from a token that does not administer the instance. It reads the
// server's own map rather than a list here, so a group added to it is covered
// by the rule without anybody remembering to widen this.
func groupNeedsAdminMode(metaTool string) bool {
	return slices.Contains(gitlabtools.MetaToolScopes[metaTool], adminModeScope)
}

// TestGroupNeedsAdminMode_ReadsTheServersOwnMap proves the predicate can say
// both words, on a group the filter really withholds and one it does not. A
// rule whose predicate has never answered "yes" in a test is a rule nobody has
// reason to believe holds the corpus to anything.
func TestGroupNeedsAdminMode_ReadsTheServersOwnMap(t *testing.T) {
	tests := []struct {
		name     string
		metaTool string
		want     bool
	}{
		{name: "a withheld group", metaTool: "gitlab_admin", want: true},
		{name: "a group anybody may call", metaTool: "gitlab_issue"},
		{name: "no group at all", metaTool: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupNeedsAdminMode(tc.metaTool); got != tc.want {
				t.Errorf("groupNeedsAdminMode(%q) = %t, want %t", tc.metaTool, got, tc.want)
			}
		})
	}
}

// tierRank orders the three tiers, and returns -1 for anything else.
func tierRank(tier Tier) int {
	switch tier {
	case TierFree:
		return 0
	case TierPremium:
		return 1
	case TierUltimate:
		return 2
	default:
		return -1
	}
}

// TestCases_ASurfaceRestrictionCarriesItsReason checks that a case which
// cannot run everywhere says why, which is the rule the harness's own
// OnSurfaces states and which the report publishes beside the row.
func TestCases_ASurfaceRestrictionCarriesItsReason(t *testing.T) {
	for _, one := range cases() {
		t.Run(one.ID, func(t *testing.T) {
			restrict := one.Surfaces
			if len(restrict.Only) == 0 {
				if restrict.Reason != "" {
					t.Errorf("gives a restriction reason and restricts nothing: %q", restrict.Reason)
				}
				return
			}
			if strings.TrimSpace(restrict.Reason) == "" {
				t.Error("restricts the surfaces it runs on and gives no reason")
			}
			for _, only := range restrict.Only {
				if _, known := Contract(only); !known {
					t.Errorf("restricts to surface %q, which is not one of the three", only)
				}
			}
		})
	}
}

// TestRestrictRuns_EmptyRunsEverywhereAndOnlyRunsWhereNamed covers the
// restriction predicate the runner reads, which no case in this partition
// exercises: the first restricted case arrives with the licensed partition and
// would otherwise be the first thing to run this code at all.
func TestRestrictRuns_EmptyRunsEverywhereAndOnlyRunsWhereNamed(t *testing.T) {
	tests := []struct {
		name     string
		restrict Restrict
		surface  Surface
		want     bool
	}{
		{name: "empty runs on dynamic", restrict: Restrict{}, surface: SurfaceDynamic, want: true},
		{name: "empty runs on individual", restrict: Restrict{}, surface: SurfaceIndividual, want: true},
		{
			name:     "named surface runs",
			restrict: Restrict{Only: []Surface{SurfaceDynamic}, Reason: "because"},
			surface:  SurfaceDynamic,
			want:     true,
		},
		{
			name:     "unnamed surface does not",
			restrict: Restrict{Only: []Surface{SurfaceDynamic}, Reason: "because"},
			surface:  SurfaceMeta,
			want:     false,
		},
		{
			name:     "one of several named surfaces runs",
			restrict: Restrict{Only: []Surface{SurfaceDynamic, SurfaceMeta}, Reason: "because"},
			surface:  SurfaceMeta,
			want:     true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.restrict.Runs(tc.surface); got != tc.want {
				t.Errorf("Runs(%q) = %t, want %t", tc.surface, got, tc.want)
			}
		})
	}
}

// TestNeedsMinimumTier_EmptyReadsAsFree covers the reader that lets the great
// majority of cases, which need no license, leave the field out.
func TestNeedsMinimumTier_EmptyReadsAsFree(t *testing.T) {
	tests := []struct {
		name  string
		needs Needs
		want  Tier
	}{
		{name: "empty", needs: Needs{}, want: TierFree},
		{name: "free", needs: Needs{Tier: TierFree}, want: TierFree},
		{name: "premium", needs: Needs{Tier: TierPremium}, want: TierPremium},
		{name: "ultimate", needs: Needs{Tier: TierUltimate}, want: TierUltimate},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.needs.MinimumTier(); got != tc.want {
				t.Errorf("MinimumTier() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRefIsZero_NamesNothingOnlyWhenBothHalvesAreEmpty covers the predicate
// every truth that is not a produced binding relies on.
func TestRefIsZero_NamesNothingOnlyWhenBothHalvesAreEmpty(t *testing.T) {
	tests := []struct {
		name string
		ref  Ref
		want bool
	}{
		{name: "empty", ref: Ref{}, want: true},
		{name: "step only", ref: Ref{Step: 1}, want: false},
		{name: "field only", ref: Ref{Field: "id"}, want: false},
		{name: "both", ref: Ref{Step: 1, Field: "id"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ref.IsZero(); got != tc.want {
				t.Errorf("IsZero() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestSurfaces_AreTheThreeTheContractsCover keeps the published order and the
// contract table from drifting apart: a fourth surface with no contract would
// render every stimulus without one.
func TestSurfaces_AreTheThreeTheContractsCover(t *testing.T) {
	surfaces := Surfaces()
	if len(surfaces) != len(contracts) {
		t.Fatalf("Surfaces() has %d entries and the contract table %d", len(surfaces), len(contracts))
	}
	for _, surface := range surfaces {
		if _, known := Contract(surface); !known {
			t.Errorf("surface %q has no contract", surface)
		}
	}
}

// TestSurfaces_AreSpelledAsTheServerSpellsThem pins the three strings this
// package hands to the runner, which converts them to harness.Surface by
// spelling and passes the result to the binary as GITLAB_MCP_TOOL_SURFACE.
//
// Nothing else here can see that join: this package is untagged and cannot
// import the harness, the contract table is keyed by these same constants, and
// the whole suite stays green with the meta and individual spellings exchanged
// — which would run every meta-restricted case on the individual surface, and
// introduce it with the wrong contract, in a run that costs money to take. The
// server's own names are the oracle rather than a second copy of the literals.
func TestSurfaces_AreSpelledAsTheServerSpellsThem(t *testing.T) {
	served := map[Surface]string{
		SurfaceDynamic:    config.ToolSurfaceDynamic,
		SurfaceMeta:       config.ToolSurfaceMeta,
		SurfaceIndividual: config.ToolSurfaceIndividual,
	}
	if len(served) != len(Surfaces()) {
		t.Fatalf("%d surfaces are held to the server's spelling and this package publishes %d",
			len(served), len(Surfaces()))
	}
	for surface, name := range served {
		t.Run(name, func(t *testing.T) {
			if string(surface) != name {
				t.Errorf("this package spells the surface %q and the server spells it %q",
					string(surface), name)
			}
		})
	}
}

// TestConstructors_SpellWhatTheCaseFilesMean covers the helpers the case files
// are written with, so a mutant that dropped Required or swapped a truth field
// would be seen here rather than in a paid run.
func TestConstructors_SpellWhatTheCaseFilesMean(t *testing.T) {
	made := step("issue.list", project(), opt("state", literal("opened")))
	if made.Action != "issue.list" || made.Standalone != "" {
		t.Errorf("step() = %+v, want action issue.list and no standalone tool", made)
	}
	if len(made.Args) != 2 {
		t.Fatalf("step() kept %d arguments, want 2", len(made.Args))
	}
	if first := made.Args[0]; first.Name != "project_id" || !first.Required || first.Truth.Fact != FactProjectPath {
		t.Errorf("project() = %+v, want a required project_id bound to the project path fact", first)
	}
	if second := made.Args[1]; second.Required || second.Truth.Literal != "opened" {
		t.Errorf("opt() = %+v, want an optional argument carrying the literal", second)
	}
	tool := standalone("gitlab_discover_project", req("remote_url", fact(FactRemoteURL)))
	if tool.Standalone != "gitlab_discover_project" || tool.Action != "" {
		t.Errorf("standalone() = %+v, want the tool name and no action", tool)
	}
	if !authored().Authored || authored().Literal != "" || authored().Fact != "" {
		t.Errorf("authored() = %+v, want only the authored flag", authored())
	}
}

// stepName spells a step for a failure message, whichever half it names.
func stepName(one Step) string {
	if one.Standalone != "" {
		return one.Standalone
	}
	return string(one.Action)
}
