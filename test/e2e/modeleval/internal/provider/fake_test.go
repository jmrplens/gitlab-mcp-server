//go:build e2e

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fakeFor builds one variant.
func fakeFor(t *testing.T, variant string) Provider {
	t.Helper()
	spec, err := ParseSpec(Fake + ":" + variant)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	built, err := New(Config{Spec: spec})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return built
}

// aCase finds a corpus case whose key's first step names a catalog action with
// at least one fact-bound argument, which is what the replay tests need to see
// a value in a call.
//
// It is chosen from the corpus rather than written here on purpose: the fake's
// job is to replay whatever the corpus holds, and a hand-written key would test
// the test.
func aCase(t *testing.T) (id string, step modelcorpus.Step) {
	t.Helper()
	keys := modelcorpus.Keys()
	for _, candidate := range modelcorpus.IDs() {
		key := keys[candidate]
		if len(key.Steps) == 0 {
			continue
		}
		first := key.Steps[0]
		if first.Action == "" {
			continue
		}
		for _, arg := range first.Args {
			if arg.Truth.Fact != "" {
				return candidate, first
			}
		}
	}
	t.Fatal("no corpus case has a first step with a fact-bound argument")
	return "", modelcorpus.Step{}
}

// factsFor invents a value for every fact one step's arguments name.
func factsFor(step modelcorpus.Step) map[string]string {
	facts := map[string]string{}
	for _, arg := range step.Args {
		if arg.Truth.Fact != "" {
			facts[arg.Truth.Fact] = "42"
		}
	}
	return facts
}

// callOf reads the one tool call of an answer.
func callOf(t *testing.T, answer Response) modelrecord.Block {
	t.Helper()
	calls := answer.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("the fake returned %d calls, want 1: %+v", len(calls), answer.Blocks)
	}
	return calls[0]
}

// argumentsOf decodes a call's arguments.
func argumentsOf(t *testing.T, call modelrecord.Block) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(call.Arguments, &decoded); err != nil {
		t.Fatalf("the call's arguments are not an object: %v", err)
	}
	return decoded
}

func TestFake_PerfectReplaysTheKeyOnEachSurfacesOwnSpelling(t *testing.T) {
	id, step := aCase(t)
	facts := factsFor(step)
	adapter := fakeFor(t, FakePerfect)

	for _, surface := range []string{
		config.ToolSurfaceDynamic, config.ToolSurfaceMeta, config.ToolSurfaceIndividual,
	} {
		t.Run(surface, func(t *testing.T) {
			answer, err := adapter.Call(t.Context(), Request{
				Replay: Replay{Case: id, Surface: surface, Facts: facts},
			})
			if err != nil {
				// An action no individual tool serves is a real answer, and
				// the runner reports it rather than pretending a call.
				if surface == config.ToolSurfaceIndividual &&
					strings.Contains(err.Error(), "not servable on the individual surface") {
					t.Skipf("%s: %v", id, err)
				}
				t.Fatalf("Call: %v", err)
			}

			call := callOf(t, answer)
			assertSurfaceSpelling(t, surface, step, call, argumentsOf(t, call))
		})
	}
}

// assertSurfaceSpelling checks that one call is spelled the way its surface
// takes it.
func assertSurfaceSpelling(
	t *testing.T,
	surface string,
	step modelcorpus.Step,
	call modelrecord.Block,
	arguments map[string]any,
) {
	t.Helper()
	switch surface {
	case config.ToolSurfaceDynamic:
		if call.Tool != dynamictools.ExecuteActionToolName {
			t.Errorf("tool = %q, want %q", call.Tool, dynamictools.ExecuteActionToolName)
		}
		if arguments["action"] != string(step.Action) {
			t.Errorf("action = %v, want %s", arguments["action"], step.Action)
		}
		if _, carried := arguments["params"]; !carried {
			t.Error("the dynamic call carries no params object, which its schema requires")
		}
	case config.ToolSurfaceMeta:
		if !strings.HasPrefix(call.Tool, "gitlab_") {
			t.Errorf("tool = %q, want the domain's meta tool", call.Tool)
		}
		if arguments["action"] == nil {
			t.Error("the meta call names no action")
		}
	default:
		if !strings.HasPrefix(call.Tool, "gitlab_") {
			t.Errorf("tool = %q, want the action's own tool", call.Tool)
		}
		if arguments["action"] != nil {
			t.Error("the individual call carries an action field, which its tool does not take")
		}
	}
}

func TestFake_PerfectWalksTheKeyOneStepPerAnsweredCall(t *testing.T) {
	// The step is read from the conversation rather than from a counter of
	// the adapter's own, so two attempts sharing one adapter cannot
	// interfere.
	keys := modelcorpus.Keys()
	var id string
	for _, candidate := range modelcorpus.IDs() {
		if len(keys[candidate].Steps) > 1 {
			id = candidate
			break
		}
	}
	if id == "" {
		t.Skip("no corpus case has more than one step")
	}

	adapter := fakeFor(t, FakePerfect)
	replay := Replay{Case: id, Surface: config.ToolSurfaceDynamic}
	first, err := adapter.Call(t.Context(), Request{Replay: replay})
	if err != nil {
		t.Fatalf("the first step: %v", err)
	}
	second, err := adapter.Call(t.Context(), Request{
		Replay: replay,
		Messages: []Message{
			{Role: RoleAssistant, Blocks: first.Blocks},
			{Role: RoleTool, Results: []ToolResult{{CallID: callOf(t, first).CallID, Content: "ok"}}},
		},
	})
	if err != nil {
		t.Fatalf("the second step: %v", err)
	}

	firstAction := argumentsOf(t, callOf(t, first))["action"]
	secondAction := argumentsOf(t, callOf(t, second))["action"]
	if firstAction == secondAction && len(keys[id].Steps) > 1 &&
		keys[id].Steps[0].Action != keys[id].Steps[1].Action {
		t.Errorf("the second turn replayed the first step again: %v", secondAction)
	}
}

func TestFake_PerfectEndsInTextWhenTheKeyIsSpent(t *testing.T) {
	id, step := aCase(t)
	adapter := fakeFor(t, FakePerfect)
	answered := make([]Message, 0, len(modelcorpus.Keys()[id].Steps))
	for range modelcorpus.Keys()[id].Steps {
		answered = append(answered, Message{Role: RoleTool, Results: []ToolResult{{CallID: "c", Content: "ok"}}})
	}

	answer, err := adapter.Call(t.Context(), Request{
		Messages: answered,
		Replay:   Replay{Case: id, Surface: config.ToolSurfaceDynamic, Facts: factsFor(step)},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(answer.ToolCalls()) != 0 {
		t.Errorf("the fake called a step the key does not have: %+v", answer.Blocks)
	}
	if len(answer.Blocks) == 0 || answer.Blocks[0].Kind != modelrecord.BlockText {
		t.Errorf("a spent key did not end in text: %+v", answer.Blocks)
	}
}

func TestFake_WrongArgsPlantsAValueMissAndNothingElse(t *testing.T) {
	id, step := aCase(t)
	facts := factsFor(step)
	replay := Replay{Case: id, Surface: config.ToolSurfaceDynamic, Facts: facts}

	right := argumentsOf(t, callOf(t, callFake(t, FakePerfect, replay)))
	wrong := argumentsOf(t, callOf(t, callFake(t, FakeWrongArgs, replay)))
	if right["action"] != wrong["action"] {
		t.Errorf("the planted variant dispatched elsewhere: %v vs %v", wrong["action"], right["action"])
	}

	rightParams, _ := right["params"].(map[string]any)
	wrongParams, _ := wrong["params"].(map[string]any)
	if len(rightParams) != len(wrongParams) {
		t.Errorf("the planted variant sent %d parameters and the right one %d: a name miss is a "+
			"different finding from a value miss", len(wrongParams), len(rightParams))
	}
	planted := 0
	for name, value := range wrongParams {
		if value != rightParams[name] {
			planted++
			if value != plantedValue {
				t.Errorf("parameter %q differs but is not the planted value: %v", name, value)
			}
		}
	}
	if planted != 1 {
		t.Errorf("%d parameters were planted, want exactly 1", planted)
	}
}

func TestFake_NoConfirmOmitsTheApprovalThatPerfectCarries(t *testing.T) {
	keys := modelcorpus.Keys()
	var id string
	var step modelcorpus.Step
	for _, candidate := range modelcorpus.IDs() {
		key := keys[candidate]
		if len(key.Steps) == 0 || key.Steps[0].Action == "" {
			continue
		}
		action, known, err := lookupAction(string(key.Steps[0].Action))
		if err != nil {
			t.Fatalf("reading the catalog: %v", err)
		}
		if known && action.destructive {
			id, step = candidate, key.Steps[0]
			break
		}
	}
	if id == "" {
		t.Skip("no corpus case opens with a destructive action")
	}

	replay := Replay{Case: id, Surface: config.ToolSurfaceDynamic, Facts: factsFor(step)}
	approved := argumentsOf(t, callOf(t, callFake(t, FakePerfect, replay)))
	if approved["confirm"] != true {
		t.Errorf("the perfect variant did not approve a destructive action: %v", approved)
	}
	withheld := argumentsOf(t, callOf(t, callFake(t, FakeNoConfirm, replay)))
	if _, carried := withheld["confirm"]; carried {
		t.Errorf("the no-confirm variant approved the action anyway: %v", withheld)
	}

	// On the other two surfaces the approval travels with the action's own
	// arguments, which is where the server reads it.
	meta := argumentsOf(t, callOf(t, callFake(t, FakePerfect,
		Replay{Case: id, Surface: config.ToolSurfaceMeta, Facts: replay.Facts})))
	params, _ := meta["params"].(map[string]any)
	if params["confirm"] != true {
		t.Errorf("the meta call's approval is not inside params: %v", meta)
	}
	individual := argumentsOf(t, callOf(t, callFake(t, FakePerfect,
		Replay{Case: id, Surface: config.ToolSurfaceIndividual, Facts: replay.Facts})))
	if individual["confirm"] != true {
		t.Errorf("the individual call's approval is not among its arguments: %v", individual)
	}
}

func TestFake_DeclinesAnswersInTextAndCallsNothing(t *testing.T) {
	id, _ := aCase(t)
	answer := callFake(t, FakeDeclines, Replay{Case: id, Surface: config.ToolSurfaceDynamic})
	if len(answer.ToolCalls()) != 0 {
		t.Errorf("the declining variant called something: %+v", answer.Blocks)
	}
	if len(answer.Blocks) != 1 || answer.Blocks[0].Kind != modelrecord.BlockText {
		t.Fatalf("the declining variant answered %+v, want one block of prose", answer.Blocks)
	}
	// It declines without being asked which case it is on, because that is
	// what a read-only verification needs: the case exists, the action is
	// withheld, and the model says so.
	blind := callFake(t, FakeDeclines, Replay{Case: "MT-000-not-a-case"})
	if len(blind.Blocks) != 1 {
		t.Errorf("the declining variant needed a corpus case: %+v", blind.Blocks)
	}
}

func TestFake_BindsAnArgumentToAnEarlierStepsResult(t *testing.T) {
	// The produced results come from the replay rather than from the
	// conversation, because what a model reads is the text beside the
	// structured content and a Produced truth names a field of the content.
	value, found := producedValue(modelcorpus.Ref{Step: 1, Field: "issue.iid"}, Replay{
		Produced: []json.RawMessage{json.RawMessage(`{"issue":{"iid":7,"title":"t"}}`)},
	})
	if !found {
		t.Fatalf("producedValue = %v, %v", value, found)
	}
	if number, isNumber := value.(float64); !isNumber || number != 7 {
		t.Errorf("value = %#v, want 7", value)
	}

	for _, one := range []struct {
		name string
		ref  modelcorpus.Ref
		from []json.RawMessage
	}{
		{"a step that has not run", modelcorpus.Ref{Step: 2, Field: "a"}, nil},
		{
			"a result that is not JSON",
			modelcorpus.Ref{Step: 1, Field: "a"},
			[]json.RawMessage{json.RawMessage(`not json`)},
		},
		{
			"a field the result does not have",
			modelcorpus.Ref{Step: 1, Field: "b"},
			[]json.RawMessage{json.RawMessage(`{"a":1}`)},
		},
		{
			"a result that is not an object",
			modelcorpus.Ref{Step: 1, Field: "a"},
			[]json.RawMessage{json.RawMessage(`[]`)},
		},
		{
			"a path through a leaf",
			modelcorpus.Ref{Step: 1, Field: "a.b"},
			[]json.RawMessage{json.RawMessage(`{"a":1}`)},
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			if _, produced := producedValue(one.ref, Replay{Produced: one.from}); produced {
				t.Error("a value was produced from a result that does not carry one")
			}
		})
	}
}

func TestFake_GivesAFactTheTypeAModelWouldHaveWritten(t *testing.T) {
	// Facts are strings because a recipe renders them into a prompt. A schema
	// that says integer refuses "7", so a replay that sent every fact as a
	// string would fail on validation rather than on anything about the pipe.
	for _, one := range []struct {
		fact string
		want any
	}{
		{"7", int64(7)},
		{"true", true},
		{"false", false},
		{"group/project", "group/project"},
		{"1.0", "1.0"},
		{"", ""},
	} {
		t.Run(one.fact, func(t *testing.T) {
			if got := typedFact(one.fact); got != one.want {
				t.Errorf("typedFact(%q) = %#v, want %#v", one.fact, got, one.want)
			}
		})
	}
}

func TestFake_ResolvesEachOfTheFourTruthsOrOmitsTheArgument(t *testing.T) {
	replay := Replay{
		Facts:    map[string]string{"project_id": "42"},
		Produced: []json.RawMessage{json.RawMessage(`{"iid":7}`)},
	}
	for _, one := range []struct {
		name  string
		truth modelcorpus.Truth
		want  any
		found bool
	}{
		{"a fact the recipe produced", modelcorpus.Truth{Fact: "project_id"}, int64(42), true},
		{"a fact it did not", modelcorpus.Truth{Fact: "group_id"}, nil, false},
		{"a literal the prompt states", modelcorpus.Truth{Literal: "closed"}, "closed", true},
		{"a value the model authors", modelcorpus.Truth{Authored: true}, authoredValue, true},
		{
			"a field an earlier step produced",
			modelcorpus.Truth{Produced: modelcorpus.Ref{Step: 1, Field: "iid"}},
			float64(7), true,
		},
		{
			"a step before the first",
			modelcorpus.Truth{Produced: modelcorpus.Ref{Step: 0, Field: "iid"}},
			nil, false,
		},
		{
			"a step the replay has not reached",
			modelcorpus.Truth{Produced: modelcorpus.Ref{Step: 2, Field: "iid"}},
			nil, false,
		},
		{"nothing at all", modelcorpus.Truth{}, nil, false},
	} {
		t.Run(one.name, func(t *testing.T) {
			value, found := resolveTruth(one.truth, replay)
			if found != one.found || (found && value != one.want) {
				t.Errorf("resolveTruth = %#v, %v, want %#v, %v", value, found, one.want, one.found)
			}
		})
	}
}

// TestFakeArguments_PlantsOneFactAndLeavesEveryOtherValue holds what each
// variant writes for every argument of one step: the perfect replay the right
// value of each, and the wrong-args replay the planted value in place of the
// first fact alone, after a literal it leaves as it is and before a second
// fact it leaves right.
func TestFakeArguments_PlantsOneFactAndLeavesEveryOtherValue(t *testing.T) {
	step := modelcorpus.Step{Args: []modelcorpus.Arg{
		{Name: "state", Truth: modelcorpus.Truth{Literal: "closed"}},
		{Name: "project_id", Truth: modelcorpus.Truth{Fact: "project_id"}},
		{Name: "group_id", Truth: modelcorpus.Truth{Fact: "group_id"}},
	}}
	replay := Replay{Facts: map[string]string{"project_id": "42", "group_id": "7"}}
	for _, one := range []struct {
		variant string
		want    map[string]any
	}{
		{FakePerfect, map[string]any{"state": "closed", "project_id": int64(42), "group_id": int64(7)}},
		{FakeWrongArgs, map[string]any{"state": "closed", "project_id": plantedValue, "group_id": int64(7)}},
	} {
		t.Run(one.variant, func(t *testing.T) {
			got := fakeAdapter{variant: one.variant}.arguments(step, replay)
			if len(got) != len(one.want) {
				t.Fatalf("arguments = %#v, want %#v", got, one.want)
			}
			for name, want := range one.want {
				if got[name] != want {
					t.Errorf("argument %q = %#v, want %#v", name, got[name], want)
				}
			}
		})
	}
}

// TestFake_NamesEachCallForTheStepItReplays verifies the first step's call is
// named for its position, counted from one, which is the name its result
// answers it by.
func TestFake_NamesEachCallForTheStepItReplays(t *testing.T) {
	id, step := aCase(t)
	call := callOf(t, callFake(t, FakePerfect, Replay{Case: id, Surface: config.ToolSurfaceDynamic, Facts: factsFor(step)}))
	if call.CallID != "fake-call-1" {
		t.Errorf("the first call is named %q, want fake-call-1", call.CallID)
	}
}

// TestFake_AMetaCallWithNoArgumentsCarriesNoParams verifies a meta-tool call
// whose step has no argument is the action alone, with no empty params beside
// it.
func TestFake_AMetaCallWithNoArgumentsCarriesNoParams(t *testing.T) {
	adapter, _ := fakeFor(t, FakePerfect).(fakeAdapter)
	_, payload, err := adapter.project(modelcorpus.Step{Action: "issue.list"}, config.ToolSurfaceMeta, map[string]any{})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, present := payload["params"]; present || payload["action"] != "list" {
		t.Errorf("payload = %#v, want the action list and no params", payload)
	}
}

// TestBuildFakeCatalog_NamesAnIndividualToolOnlyForTheActionItIsRegisteredFor
// verifies the individual tool name an action declares is kept only when the
// call identifier resolves that name to the action itself: a name a sibling
// registered first, and a name the individual surface does not register at
// all, leave the action with no individual tool rather than calling somebody
// else's handler or a tool that does not exist.
func TestBuildFakeCatalog_NamesAnIndividualToolOnlyForTheActionItIsRegisteredFor(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	// sequential: each group is added to the one catalog the test reads after the loop
	for _, declared := range []struct {
		toolName, domain, individual string
		kind                         actioncatalog.SurfaceKind
	}{
		{"gitlab_fake_first", "first", "gitlab_fake_shared", actioncatalog.SurfaceKindMetaGroup},
		{"gitlab_fake_second", "second", "gitlab_fake_shared", actioncatalog.SurfaceKindMetaGroup},
		{"gitlab_fake_hidden", "hidden", "gitlab_fake_hidden_list", actioncatalog.SurfaceKindDynamicController},
	} {
		spec := toolutil.NewActionSpec("list", toolutil.RouteAction(nil,
			func(context.Context, *gitlabclient.Client, struct{}) (struct{}, error) { return struct{}{}, nil }),
			toolutil.ActionSpecOptions{
				ReadOnly:       true,
				OwnerPackage:   "provider",
				IndividualTool: toolutil.IndividualToolSpec{Name: declared.individual, Title: "Probe", Description: "Probe."},
			})
		group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
			ToolName:     declared.toolName,
			BaseDomain:   declared.domain,
			Title:        "Probe",
			Description:  "Probe group.",
			OwnerPackage: "provider",
			SurfaceKind:  declared.kind,
		}, []toolutil.ActionSpec{spec})
		if err != nil {
			t.Fatalf("GroupFromSpecs(%s): %v", declared.toolName, err)
		}
		if addErr := catalog.AddGroup(group); addErr != nil {
			t.Fatalf("AddGroup(%s): %v", declared.toolName, addErr)
		}
	}

	actions, err := buildFakeCatalog(func(bool, gitlabtools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		return catalog, nil
	})
	if err != nil {
		t.Fatalf("buildFakeCatalog: %v", err)
	}
	for id, want := range map[string]string{
		"first.list":  "gitlab_fake_shared",
		"second.list": "",
		"hidden.list": "",
	} {
		t.Run(id, func(t *testing.T) {
			if got := actions[id].individualTool; got != want {
				t.Errorf("%s has individual tool %q, want %q", id, got, want)
			}
		})
	}
}

func TestFake_SaysWhatItWasConfiguredFrom(t *testing.T) {
	if spec := fakeFor(t, FakePerfect).Spec(); spec.Raw != "fake:perfect" || spec.Provider != Fake {
		t.Errorf("Spec() = %+v, want the fake spec as configured", spec)
	}
}

func TestFake_RefusesAVariantAndACaseItDoesNotHave(t *testing.T) {
	spec, err := ParseSpec("fake:almost-perfect")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	_, buildErr := New(Config{Spec: spec})
	if buildErr == nil {
		t.Error("an unknown variant was built")
	} else if !strings.Contains(buildErr.Error(), FakePerfect) {
		t.Errorf("the refusal does not name the variants there are: %v", buildErr)
	}

	answer, err := fakeFor(t, FakePerfect).Call(t.Context(), Request{Replay: Replay{Case: "MT-000"}})
	if err == nil {
		t.Fatal("a case the corpus does not have was replayed anyway")
	}
	if answer.Status != modelrecord.TurnRequestError {
		t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnRequestError)
	}
}

func TestFake_RefusesASurfaceItCannotSpellACallOn(t *testing.T) {
	id, step := aCase(t)
	_, err := fakeFor(t, FakePerfect).Call(t.Context(), Request{
		Replay: Replay{Case: id, Surface: "telepathy", Facts: factsFor(step)},
	})
	if err == nil {
		t.Error("a call was spelled on a surface that does not exist")
	}
}

func TestFake_AStandaloneToolIsNamedDirectlyOnEverySurface(t *testing.T) {
	adapter, _ := fakeFor(t, FakePerfect).(fakeAdapter)
	for _, surface := range []string{
		config.ToolSurfaceDynamic, config.ToolSurfaceMeta, config.ToolSurfaceIndividual,
	} {
		t.Run(surface, func(t *testing.T) {
			tool, payload, err := adapter.project(
				modelcorpus.Step{Standalone: "gitlab_discover_project"}, surface, map[string]any{"path": "."},
			)
			if err != nil {
				t.Fatalf("project: %v", err)
			}
			if tool != "gitlab_discover_project" {
				t.Errorf("tool = %q, want the standalone name", tool)
			}
			if payload["path"] != "." {
				t.Errorf("the arguments are not at the top level: %v", payload)
			}
		})
	}
}

func TestFake_RefusesAStepTheCatalogCannotSpell(t *testing.T) {
	adapter, _ := fakeFor(t, FakePerfect).(fakeAdapter)
	if _, _, err := adapter.project(
		modelcorpus.Step{Action: "nonesuch.action"}, config.ToolSurfaceDynamic, nil,
	); err == nil {
		t.Error("an action the catalog does not have was spelled as a call")
	}

	// An action the individual surface cannot serve is a refusal rather than
	// a call under somebody else's tool name. The corpus gate keeps such a
	// case off that surface, so this is the guard behind that rule.
	unservable := unservableOnIndividual(t)
	if unservable == "" {
		t.Skip("every catalog action is servable on the individual surface")
	}
	if _, _, err := adapter.project(
		modelcorpus.Step{Action: modelcorpus.Action(unservable)}, config.ToolSurfaceIndividual, nil,
	); err == nil {
		t.Errorf("%s was spelled as an individual call it has no tool for", unservable)
	}
}

// unservableOnIndividual finds an action whose individual tool name a sibling
// took, or none.
func unservableOnIndividual(t *testing.T) string {
	t.Helper()
	if _, _, err := lookupAction("issue.list"); err != nil {
		t.Fatalf("reading the catalog: %v", err)
	}
	for id, action := range fakeCatalog {
		if action.individualTool == "" {
			return id
		}
	}
	return ""
}

// TestFake_ACatalogThatCouldNotBeBuilt_RefusesEveryActionStep verifies a
// catalog the fake could not read is the answer to every step that names an
// action, rather than a call spelled without it. It replaces the error the
// catalog was read with, so it does not run in parallel.
func TestFake_ACatalogThatCouldNotBeBuilt_RefusesEveryActionStep(t *testing.T) {
	if _, _, err := lookupAction("issue.list"); err != nil {
		t.Fatalf("reading the catalog: %v", err)
	}
	broken := errors.New("no catalog")
	saved := errFakeCatalog
	errFakeCatalog = broken
	t.Cleanup(func() { errFakeCatalog = saved })

	adapter, _ := fakeFor(t, FakePerfect).(fakeAdapter)
	if _, _, err := adapter.project(modelcorpus.Step{Action: "issue.list"}, config.ToolSurfaceDynamic, nil); !errors.Is(err, broken) {
		t.Errorf("project = %v, want the catalog's own error", err)
	}
}

// TestFake_ACallThatDoesNotEncode_FailsTheTurn verifies a payload that could
// not be encoded ends the turn as a request error naming the step, rather
// than a call sent without its arguments. It replaces the encoder, so it does
// not run in parallel.
func TestFake_ACallThatDoesNotEncode_FailsTheTurn(t *testing.T) {
	broken := errors.New("no encoding")
	saved := encodeCall
	encodeCall = func(any) ([]byte, error) { return nil, broken }
	t.Cleanup(func() { encodeCall = saved })

	id, step := aCase(t)
	answer, err := fakeFor(t, FakePerfect).Call(t.Context(), Request{
		Replay: Replay{Case: id, Surface: config.ToolSurfaceDynamic, Facts: factsFor(step)},
	})
	if !errors.Is(err, broken) || !strings.Contains(err.Error(), "fake: encode the call for step 1") {
		t.Fatalf("Call = %v, want the encoding failure of step 1", err)
	}
	if answer.Status != modelrecord.TurnRequestError {
		t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnRequestError)
	}
}

// TestBuildFakeCatalog_ABuilderThatFails_SaysSo verifies a catalog that could
// not be built is reported as that, wrapping the builder's own error.
func TestBuildFakeCatalog_ABuilderThatFails_SaysSo(t *testing.T) {
	broken := errors.New("no tools")
	_, err := buildFakeCatalog(func(bool, gitlabtools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		return nil, broken
	})
	if !errors.Is(err, broken) || !strings.Contains(err.Error(), "fake: build the action catalog") {
		t.Errorf("buildFakeCatalog = %v, want it to say the catalog could not be built and why", err)
	}
}

func TestFake_HashesTheToolListItWasGiven(t *testing.T) {
	tools := sampleTools(t)
	if fakeFor(t, FakePerfect).ToolDigest(tools) != adapterFor(t, "openai:gpt-5.4-nano", "").ToolDigest(tools) {
		t.Error("the fake publishes a different digest from the real adapters, so a fake run's session " +
			"line could not be compared with a paid one's")
	}
}

// callFake sends one turn to one variant.
func callFake(t *testing.T, variant string, replay Replay) Response {
	t.Helper()
	answer, err := fakeFor(t, variant).Call(t.Context(), Request{Replay: replay})
	if err != nil {
		t.Fatalf("the %s variant: %v", variant, err)
	}
	return answer
}
