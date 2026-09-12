//go:build e2e

// project_test.go covers the projection: what each surface calls an action,
// and which actions a surface cannot reach at all.
//
// It runs against the real catalog rather than a fixture, because the thing
// worth pinning is the relationship between a canonical ID and the three
// spellings the product gives it, and a fixture would pin a relationship this
// package invented.

package harness

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// freeProjection returns the projection a Free self-managed instance serves,
// which is the catalog every one of these assertions is about.
func freeProjection(t *testing.T) *projection {
	t.Helper()

	made, err := newProjection(edition.Free, false)
	if err != nil {
		t.Fatalf("building the projection: %v", err)
	}
	return made
}

// TestProjection_OneActionOnEverySurface_SpellsTheSurfacesOwnCall pins what a
// test naming one canonical ID actually sends on each of the three surfaces.
//
// The three spellings are genuinely different, and the individual one is
// declared rather than derived: gitlab_issue_list is domain-first,
// gitlab_server_status is a diagnostics tool outside the GitLab API, and a
// large legacy set is verb-first. A projection that guessed would send a tool
// name no surface registers and the failure would read as a missing tool.
func TestProjection_OneActionOnEverySurface_SpellsTheSurfacesOwnCall(t *testing.T) {
	made := freeProjection(t)

	cases := []struct {
		name           string
		id             ActionID
		metaTool       string
		metaAction     string
		individualTool string
	}{
		{name: "issue list", id: "issue.list", metaTool: "gitlab_issue", metaAction: "list", individualTool: "gitlab_issue_list"},
		{name: "project get", id: "project.get", metaTool: "gitlab_project", metaAction: "get", individualTool: "gitlab_project_get"},
		{name: "branch create", id: "branch.create", metaTool: "gitlab_branch", metaAction: "create", individualTool: "gitlab_branch_create"},
		{name: "server status", id: "server.status", metaTool: "gitlab_server", metaAction: "status", individualTool: "gitlab_server_status"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			action, known := made.lookup(testCase.id)
			if !known {
				t.Fatalf("the Free catalog has no action %s", testCase.id)
			}

			assertDynamicCall(t, action)
			assertMetaCall(t, action, testCase.metaTool, testCase.metaAction)
			assertIndividualCall(t, action, testCase.individualTool)
		})
	}
}

// projectionParams are the parameters every projection assertion sends, so the
// three checks below differ only in where they expect to find them.
var projectionParams = map[string]any{"project_id": "group/project"}

// assertDynamicCall checks the call the default surface spells: the execute
// tool, the canonical ID in the action argument, and the parameters in an
// object under params.
func assertDynamicCall(t *testing.T, action projectedAction) {
	t.Helper()

	call, err := action.callOn(SurfaceDynamic, projectionParams, false)
	if err != nil {
		t.Fatalf("projecting %s onto the dynamic surface: %v", action.id, err)
	}
	if call.tool != dynamictools.ExecuteActionToolName {
		t.Errorf("dynamic tool = %q, want %q", call.tool, dynamictools.ExecuteActionToolName)
	}
	if call.arguments["action"] != string(action.id) {
		t.Errorf("dynamic action argument = %v, want %s", call.arguments["action"], action.id)
	}
	params, isObject := call.arguments["params"].(map[string]any)
	if !isObject || params["project_id"] != "group/project" {
		t.Errorf("dynamic params = %v, want the caller's parameters in an object", call.arguments["params"])
	}
}

// assertMetaCall checks the call the meta surface spells: the domain tool,
// with the operation in its action argument.
func assertMetaCall(t *testing.T, action projectedAction, wantTool, wantAction string) {
	t.Helper()

	call, err := action.callOn(SurfaceMeta, projectionParams, false)
	if err != nil {
		t.Fatalf("projecting %s onto the meta surface: %v", action.id, err)
	}
	if call.tool != wantTool || call.arguments["action"] != wantAction {
		t.Errorf("meta call = %s/%v, want %s/%s", call.tool, call.arguments["action"], wantTool, wantAction)
	}
}

// assertIndividualCall checks the call the individual surface spells: the
// declared tool name, with the parameters at the top level.
func assertIndividualCall(t *testing.T, action projectedAction, wantTool string) {
	t.Helper()

	call, err := action.callOn(SurfaceIndividual, projectionParams, false)
	if err != nil {
		t.Fatalf("projecting %s onto the individual surface: %v", action.id, err)
	}
	if call.tool != wantTool {
		t.Errorf("individual tool = %q, want %q", call.tool, wantTool)
	}
	if call.arguments["project_id"] != "group/project" {
		t.Errorf("individual arguments = %v, want the parameters at the top level", call.arguments)
	}
}

// TestProjection_ShadowedIndividualName_IsUnservableAndNamesTheOwner is the
// case the projection exists for.
//
// repository.file_history and repository.commit_list both declare
// gitlab_commit_list, and registration binds the name to whichever comes
// first. A harness that sent the name for either would run one action while
// claiming to have run the other, and the test would pass. So the second one
// is unservable here, and the refusal says who owns the name.
func TestProjection_ShadowedIndividualName_IsUnservableAndNamesTheOwner(t *testing.T) {
	made := freeProjection(t)

	action, known := made.lookup("repository.file_history")
	if !known {
		t.Fatal("the Free catalog has no repository.file_history")
	}

	_, err := action.callOn(SurfaceIndividual, nil, false)
	if err == nil {
		t.Fatal("repository.file_history projected onto an individual tool, and gitlab_commit_list belongs to another action")
	}
	if !strings.Contains(err.Error(), "repository.commit_list") {
		t.Errorf("the refusal does not name the action that owns the tool name: %v", err)
	}

	// The other two surfaces reach it, which is what makes the individual one
	// a gap rather than the action being unreachable.
	for _, surface := range []Surface{SurfaceDynamic, SurfaceMeta} {
		t.Run(string(surface), func(t *testing.T) {
			if _, projectErr := action.callOn(surface, nil, false); projectErr != nil {
				t.Errorf("repository.file_history does not project onto %s: %v", surface, projectErr)
			}
		})
	}
}

// TestProjection_ActionWithNoIndividualTool_IsUnservableThere covers the other
// half of the same rule.
//
// server.health_check declares no individual tool name at all, so the
// individual surface registers nothing for it. Saying so here is what keeps a
// coverage report from counting it as reached on a surface that never served
// it.
func TestProjection_ActionWithNoIndividualTool_IsUnservableThere(t *testing.T) {
	made := freeProjection(t)

	action, known := made.lookup("server.health_check")
	if !known {
		t.Fatal("the Free catalog has no server.health_check; the projection is built without IncludeMCP")
	}

	_, err := action.callOn(SurfaceIndividual, nil, false)
	if err == nil {
		t.Fatal("server.health_check projected onto an individual tool, and it declares none")
	}
	if !strings.Contains(err.Error(), "declares no individual tool") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

// TestProjection_DestructiveAction_CarriesConfirmWhereItsSurfaceReadsIt pins
// where each surface reads an explicit approval from.
//
// The dynamic execute tool reads it at the top level and says so in its own
// schema; the other two read it from the action's parameters. A confirmation
// put in the wrong place is not an error anywhere, it is simply not seen, and
// the destructive call is then refused for want of one.
func TestProjection_DestructiveAction_CarriesConfirmWhereItsSurfaceReadsIt(t *testing.T) {
	made := freeProjection(t)

	action, known := made.lookup("project.delete")
	if !known {
		t.Fatal("the Free catalog has no project.delete")
	}
	if !action.destructive {
		t.Fatal("project.delete is not classified destructive, so this test is asserting nothing")
	}

	dynamic, err := action.callOn(SurfaceDynamic, map[string]any{"project_id": "group/project"}, true)
	if err != nil {
		t.Fatalf("projecting project.delete onto the dynamic surface: %v", err)
	}
	if dynamic.arguments[confirmArgument] != true {
		t.Errorf("the dynamic call carries no top-level confirm: %v", dynamic.arguments)
	}
	if params, isObject := dynamic.arguments["params"].(map[string]any); isObject {
		if _, inParams := params[confirmArgument]; inParams {
			t.Errorf("the dynamic call also put confirm inside params, where the action reads it as a parameter: %v", params)
		}
	}

	for _, surface := range []Surface{SurfaceMeta, SurfaceIndividual} {
		t.Run(string(surface), func(t *testing.T) {
			call, projectErr := action.callOn(surface, map[string]any{"project_id": "group/project"}, true)
			if projectErr != nil {
				t.Fatalf("projecting project.delete onto %s: %v", surface, projectErr)
			}
			params := call.arguments
			if surface == SurfaceMeta {
				nested, isObject := call.arguments["params"].(map[string]any)
				if !isObject {
					t.Fatalf("the meta call carries no params object: %v", call.arguments)
				}
				params = nested
			}
			if params[confirmArgument] != true {
				t.Errorf("the %s call carries no confirm: %v", surface, params)
			}
		})
	}
}

// TestProjection_WithoutConfirmation_SendsNone checks that an unconfirmed call
// really is unconfirmed, which is what a test of the server's fail-closed
// behavior stands on.
func TestProjection_WithoutConfirmation_SendsNone(t *testing.T) {
	made := freeProjection(t)

	action, known := made.lookup("project.delete")
	if !known {
		t.Fatal("the Free catalog has no project.delete")
	}

	for _, surface := range AllSurfaces() {
		t.Run(string(surface), func(t *testing.T) {
			call, err := action.callOn(surface, map[string]any{"project_id": "group/project"}, false)
			if err != nil {
				t.Fatalf("projecting project.delete onto %s: %v", surface, err)
			}
			if slices.Contains(call.argumentNames, confirmArgument) {
				t.Errorf("an unconfirmed %s call carries confirm: %v", surface, call.arguments)
			}
		})
	}
}

// TestProjection_UnknownSurface_IsRefused checks that a surface nothing serves
// is reported rather than treated as one of the three.
func TestProjection_UnknownSurface_IsRefused(t *testing.T) {
	made := freeProjection(t)

	action, known := made.lookup("issue.list")
	if !known {
		t.Fatal("the Free catalog has no issue.list")
	}

	if _, err := action.callOn(Surface("http"), nil, false); err == nil {
		t.Fatal("callOn accepted a surface the binary does not serve")
	}
}

// TestProjection_TierDecidesWhatExists checks that the projection is built for
// the tier it was asked for.
//
// An Ultimate-only action is absent from the Free catalog, which is how a call
// to it from a package that declared no license fails with a message about the
// catalog rather than with a 403 from GitLab twenty lines later.
func TestProjection_TierDecidesWhatExists(t *testing.T) {
	free := freeProjection(t)
	ultimate, err := newProjection(edition.Ultimate, false)
	if err != nil {
		t.Fatalf("building the Ultimate projection: %v", err)
	}

	licensed := 0
	for id, action := range ultimate.actions {
		if action.minimumTier == edition.Free {
			continue
		}
		licensed++
		if _, known := free.lookup(id); known {
			t.Errorf("%s needs %s and is in the Free catalog", id, action.minimumTier)
		}
	}
	if licensed == 0 {
		t.Fatal("the Ultimate catalog carries no action above Free, so this test asserts nothing")
	}
	if len(free.actions) >= len(ultimate.actions) {
		t.Errorf("the Free catalog has %d actions and the Ultimate one %d, and Free is a subset",
			len(free.actions), len(ultimate.actions))
	}
}

// TestActionTier_ReadsTheWholeCatalog checks that a test can ask the tier of
// an action its own runtime does not serve.
//
// The instance behind the Env is Free, and the licensed action is still
// answered with its tier: that is what lets a common test assert, on every
// runtime, that a listing carries nothing above the tier the instance has.
func TestActionTier_ReadsTheWholeCatalog(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	licensed := licensedActionFor(t, inst)

	cases := []struct {
		name      string
		id        ActionID
		wantKnown bool
		wantAbove bool
	}{
		{name: "free action", id: "server.status", wantKnown: true},
		{name: "licensed action", id: licensed, wantKnown: true, wantAbove: true},
		{name: "not an action", id: "not_a_domain.not_an_action"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			tier, known := env.ActionTier(testCase.id)

			if known != testCase.wantKnown {
				t.Fatalf("ActionTier(%s) known = %t, want %t", testCase.id, known, testCase.wantKnown)
			}
			if above := tier > edition.Free; above != testCase.wantAbove {
				t.Errorf("ActionTier(%s) = %s, want above Free = %t", testCase.id, tier, testCase.wantAbove)
			}
		})
	}
}
