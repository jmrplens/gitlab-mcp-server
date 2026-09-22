//go:build e2e

// project_test.go covers the projection: what each surface calls an action,
// and which actions a surface cannot reach at all.
//
// It runs against the real catalog rather than a fixture, because the thing
// worth pinning is the relationship between a canonical ID and the three
// spellings the product gives it, and a fixture would pin a relationship this
// package invented. The last test takes the standalone spellings to the real
// binary, since a spelling only the harness believes in is the defect this file
// exists to catch.

package harness

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamiccatalog"
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
		// A standalone utility is in the catalog the projection reads, at
		// Free, so a test walking the dynamic manifest has an answer for every
		// entry it lists.
		{name: "standalone utility", id: "interactive.issue_create", wantKnown: true},
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

// TestProjection_StandaloneAction_IsSpelledAsEachSurfaceRegistersIt pins the
// three spellings of a standalone utility, which is what issue 903 was about.
//
// The dynamic surface reaches one through gitlab_execute_action like any other
// action. The other two register it as a tool of its own under its declared
// name, with flat arguments: on meta that is not the domain tool plus an
// action argument, and the dynamic catalog's group name, gitlab_interactive,
// is registered nowhere, so the ordinary meta spelling would name a tool that
// does not exist or hand a locked schema an action it refuses.
func TestProjection_StandaloneAction_IsSpelledAsEachSurfaceRegistersIt(t *testing.T) {
	made := freeProjection(t)

	cases := []struct {
		name   string
		id     ActionID
		tool   string
		params map[string]any
	}{
		{
			name: "a guided flow on a project", id: "interactive.issue_create", tool: "gitlab_interactive_issue_create",
			params: map[string]any{"project_id": "group/project"},
		},
		{name: "a guided flow that needs nothing", id: "interactive.project_create", tool: "gitlab_interactive_project_create"},
		{
			name: "project discovery", id: "discover_project.resolve", tool: "gitlab_discover_project",
			params: map[string]any{"remote_url": "https://gitlab.test/group/project.git"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			action, known := made.lookup(testCase.id)
			if !known {
				t.Fatalf("the projection has no %s; it is read from a catalog without the standalone groups", testCase.id)
			}
			if !action.standalone {
				t.Errorf("%s is not marked standalone", testCase.id)
			}

			dynamic, err := action.callOn(SurfaceDynamic, testCase.params, true)
			if err != nil {
				t.Fatalf("projecting %s onto the dynamic surface: %v", testCase.id, err)
			}
			if dynamic.tool != dynamictools.ExecuteActionToolName || dynamic.arguments["action"] != string(testCase.id) {
				t.Errorf("dynamic call = %s/%v, want %s with the canonical id", dynamic.tool, dynamic.arguments["action"],
					dynamictools.ExecuteActionToolName)
			}

			for _, surface := range []Surface{SurfaceMeta, SurfaceIndividual} {
				t.Run(string(surface), func(t *testing.T) {
					assertStandaloneToolCall(t, action, surface, testCase.tool, testCase.params)
				})
			}
		})
	}
}

// assertStandaloneToolCall checks the call a standalone action takes on a
// surface that registers it as a tool of its own: the declared name, and the
// parameters flat with nothing added, no action argument and no params object.
func assertStandaloneToolCall(t *testing.T, action projectedAction, surface Surface, wantTool string, params map[string]any) {
	t.Helper()

	call, err := action.callOn(surface, params, true)
	if err != nil {
		t.Fatalf("projecting %s onto %s: %v", action.id, surface, err)
	}
	if call.tool != wantTool {
		t.Errorf("%s tool = %q, want the declared %q", surface, call.tool, wantTool)
	}
	if wantNames := argumentNames(params); !slices.Equal(call.argumentNames, wantNames) {
		t.Errorf("%s arguments = %v, want the parameters flat and nothing else, %v", surface, call.argumentNames, wantNames)
	}
}

// TestProjection_StandaloneGroups_AreTheTwoUtilityKindsAndNothingElse pins the
// classification the standalone spelling rests on, against the catalog the
// binary serves.
//
// gitlab_server is the group the rule could catch by accident: it is not a
// GitLab API domain either, and were it filed as a utility its actions would
// be spelled as tools of their own on meta, where the binary registers them
// as routes of gitlab_server.
func TestProjection_StandaloneGroups_AreTheTwoUtilityKindsAndNothingElse(t *testing.T) {
	catalog, _, err := dynamiccatalog.Build(gitlabtools.UnboundClient(false), &config.ServerConfig{Tier: edition.Free})
	if err != nil {
		t.Fatalf("assembling the served catalog: %v", err)
	}
	made := freeProjection(t)

	cases := []struct {
		name       string
		group      string
		action     ActionID
		wantKind   actioncatalog.SurfaceKind
		standalone bool
		metaTool   string
		metaAction string
	}{
		{
			name: "diagnostics", group: "gitlab_server", action: "server.status",
			wantKind: actioncatalog.SurfaceKindMetaGroup, metaTool: "gitlab_server", metaAction: "status",
		},
		{
			name: "a domain", group: "gitlab_issue", action: "issue.list",
			wantKind: actioncatalog.SurfaceKindMetaGroup, metaTool: "gitlab_issue", metaAction: "list",
		},
		{
			name: "project discovery", group: "gitlab_discover_project", action: "discover_project.resolve",
			wantKind: actioncatalog.SurfaceKindRuntimeUtility, standalone: true, metaTool: "gitlab_discover_project",
		},
		{
			name: "guided flows", group: "gitlab_interactive", action: "interactive.mr_create",
			wantKind: actioncatalog.SurfaceKindInteractiveUtility, standalone: true, metaTool: "gitlab_interactive_mr_create",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			group, found := catalog.Group(testCase.group)
			if !found {
				t.Fatalf("the served catalog has no group %s", testCase.group)
			}
			if group.SurfaceKind != testCase.wantKind {
				t.Errorf("%s is a %s group, want %s", testCase.group, group.SurfaceKind, testCase.wantKind)
			}
			action, known := made.lookup(testCase.action)
			if !known {
				t.Fatalf("the projection has no %s", testCase.action)
			}
			if action.standalone != testCase.standalone || action.metaTool != testCase.metaTool || action.metaAction != testCase.metaAction {
				t.Errorf("%s projects as standalone=%t %s/%q, want standalone=%t %s/%q", testCase.action,
					action.standalone, action.metaTool, action.metaAction, testCase.standalone, testCase.metaTool, testCase.metaAction)
			}
		})
	}
}

// TestStandaloneGroup_OnlyTheTwoUtilityKinds checks the rule one kind at a
// time, the dynamic controller included, which never reaches a catalog today
// and would not be a standalone utility if it did.
func TestStandaloneGroup_OnlyTheTwoUtilityKinds(t *testing.T) {
	cases := []struct {
		kind actioncatalog.SurfaceKind
		want bool
	}{
		{kind: actioncatalog.SurfaceKindGitLabAction, want: false},
		{kind: actioncatalog.SurfaceKindMetaGroup, want: false},
		{kind: actioncatalog.SurfaceKindDynamicController, want: false},
		{kind: actioncatalog.SurfaceKindRuntimeUtility, want: true},
		{kind: actioncatalog.SurfaceKindInteractiveUtility, want: true},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.kind), func(t *testing.T) {
			if got := standaloneGroup(actioncatalog.Group{SurfaceKind: testCase.kind}); got != testCase.want {
				t.Errorf("standaloneGroup(%s) = %t, want %t", testCase.kind, got, testCase.want)
			}
		})
	}
}

// TestProjection_MetaCallWithNoParameters_CarriesNoParamsObject checks that a
// domain tool is sent the action alone when the action takes nothing, rather
// than an empty params object beside it, which is the one shape a meta call
// can take that no other call of this projection sends.
func TestProjection_MetaCallWithNoParameters_CarriesNoParamsObject(t *testing.T) {
	action, known := freeProjection(t).lookup("server.status")
	if !known {
		t.Fatal("the Free catalog has no server.status")
	}

	call, err := action.callOn(SurfaceMeta, nil, false)
	if err != nil {
		t.Fatalf("projecting server.status onto meta: %v", err)
	}
	if !slices.Equal(call.argumentNames, []string{"action"}) {
		t.Errorf("the meta call carries %v, want the action argument alone", call.argumentNames)
	}
}

// TestProjection_MetaActionWithoutAPair_IsRefused checks the meta refusal an
// ordinary action with no tool and action pair gets, which the standalone
// branch sits in front of: a projection that sent it anyway would name a tool
// with an empty action and read as a server defect.
func TestProjection_MetaActionWithoutAPair_IsRefused(t *testing.T) {
	cases := []struct {
		name   string
		action projectedAction
	}{
		{name: "no tool", action: projectedAction{id: "ghost.list", metaAction: "list"}},
		{name: "no action", action: projectedAction{id: "ghost.list", metaTool: "gitlab_ghost"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := testCase.action.callOn(SurfaceMeta, nil, false)
			if err == nil || !strings.Contains(err.Error(), "declares no meta tool and action pair") {
				t.Errorf("callOn(meta) = %v, want the missing pair named", err)
			}
		})
	}
}

// TestBuildProjection_CatalogThatCannotBeBuilt_IsReportedWithItsTier checks the
// one failure a projection can have, through the seam, since no tier the
// binary accepts makes the assembly fail. It calls buildProjection directly so
// nothing is cached under a real key.
func TestBuildProjection_CatalogThatCannotBeBuilt_IsReportedWithItsTier(t *testing.T) {
	cause := errors.New("the catalog would not assemble")
	previous := buildServedCatalog
	t.Cleanup(func() { buildServedCatalog = previous })
	buildServedCatalog = func(*gitlabclient.Client, *config.ServerConfig) (*actioncatalog.Catalog, gitlabtools.WithheldActions, error) {
		return nil, gitlabtools.WithheldActions{}, cause
	}

	made, err := buildProjection(edition.Premium, false)

	if made != nil {
		t.Errorf("a projection was returned from a catalog that could not be built: %+v", made)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("buildProjection() error = %v, want it to wrap the assembly's own", err)
	}
	if !strings.Contains(err.Error(), edition.Premium.String()) {
		t.Errorf("the error %q does not name the tier it was building", err)
	}
}

// standaloneStubProject is the project the stub GitLab answers with, for a
// create and for a lookup by path. It is TestProjectCreate_Success's fixture in
// internal/tools/projects, because the create handler decodes the captured body
// into toolutil.CapturedProject as well as client-go's Project (ADR-0021), and
// a body that fixture's test already holds to both is one this test can rely
// on rather than a shape invented here.
const standaloneStubProject = `{"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo","visibility":"private",` +
	`"default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/my-repo","description":""}`

// standaloneStubGitLab is a stub GitLab that answers the two standalone actions
// the harness test drives, and remembers the name a project was created with.
type standaloneStubGitLab struct {
	server *httptest.Server
	mu     sync.Mutex
	posted []string
}

// createdNames returns every name a project was created with, in order.
func (s *standaloneStubGitLab) createdNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.posted)
}

// startStandaloneStubGitLab serves what the server asks at startup, a project
// create, and the project group/project, and a 404 for everything else.
//
// The handlers report through Errorf and answer, and never stop the test: they
// run on the httptest server's goroutines.
func startStandaloneStubGitLab(t *testing.T) *standaloneStubGitLab {
	t.Helper()

	stub := &standaloneStubGitLab{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/version":
			writeStubJSON(w, map[string]any{"version": "18.0.0", "revision": "abcdef", "enterprise": false})
		case r.URL.Path == "/api/v4/user":
			writeStubJSON(w, map[string]any{"id": 7, "username": "harness", "name": "Harness", "is_admin": true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects":
			stub.create(t, w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/group/project":
			stub.answer(t, w, http.StatusOK, map[string]any{"name": "project", "path": "project", "path_with_namespace": "group/project"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

// create records the name a project is created with and answers with it.
func (s *standaloneStubGitLab) create(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("the project create carried no JSON body the stub could read: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.posted = append(s.posted, body.Name)
	s.mu.Unlock()
	s.answer(t, w, http.StatusCreated, map[string]any{"name": body.Name})
}

// answer writes the fixture project with the given fields replaced.
func (s *standaloneStubGitLab) answer(t *testing.T, w http.ResponseWriter, status int, fields map[string]any) {
	t.Helper()
	var project map[string]any
	if err := json.Unmarshal([]byte(standaloneStubProject), &project); err != nil {
		t.Errorf("the stub's own fixture does not decode: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	maps.Copy(project, fields)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(project); err != nil {
		t.Errorf("writing the stub's project answer: %v", err)
	}
}

// projectFlowScript answers the guided project flow by the property each
// prompt asks for, and records what was asked, in order. It declines anything
// it was not told to expect, so an extra prompt fails the flow rather than
// being filled in. It runs on the SDK's goroutine, so it touches no testing.T.
type projectFlowScript struct {
	answers map[string]any
	mu      sync.Mutex
	asked   []string
}

// newProjectFlowScript returns the script that creates a project named name.
func newProjectFlowScript(name string) *projectFlowScript {
	return &projectFlowScript{answers: map[string]any{
		"name":           name,
		"description":    "made by the harness",
		"selection":      "private",
		"confirmed":      true,
		"default_branch": "main",
	}}
}

// respond answers one elicitation request.
func (s *projectFlowScript) respond(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	if req == nil || req.Params == nil {
		return &mcp.ElicitResult{Action: "decline"}, nil
	}
	schema, _ := req.Params.RequestedSchema.(map[string]any)
	properties, _ := schema["properties"].(map[string]any)

	s.mu.Lock()
	defer s.mu.Unlock()
	content := map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(properties)) {
		s.asked = append(s.asked, key)
		value, expected := s.answers[key]
		if !expected {
			return &mcp.ElicitResult{Action: "decline"}, nil
		}
		content[key] = value
	}
	return &mcp.ElicitResult{Action: "accept", Content: content}, nil
}

// askedProperties returns what the flow asked for, in the order it asked.
func (s *projectFlowScript) askedProperties() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.asked)
}

// projectFlowPrompts is what the guided project flow asks for, in order: the
// README question and the final confirmation are both yes-or-no prompts. The
// common package's scenario holds a real run to the same sequence, and this is
// where a replay of an answered prompt under the multi round trip, which would
// make that assertion fail against a real GitLab, is caught without one.
var projectFlowPrompts = []string{"name", "description", "selection", "confirmed", "default_branch", "confirmed"}

// createdProject is the part of a project answer this test reads.
type createdProject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
}

// TestProjection_StandaloneActions_RunThroughTheBinaryOnEverySurface takes the
// standalone spellings to the real binary, on all three surfaces, and reads
// back what the record says about them.
//
// It is the whole of issue 903 short of a GitLab: the call the projection
// spells is one the binary accepts, the session serves it, a guided flow's
// elicitation round trip completes under the harness's scripted client, and
// the recorded call names the action the server's own span says it ran. The
// last part is what a coverage record credits, and on meta and individual it
// rests on the server naming its standalone tools, which it did not do before.
func TestProjection_StandaloneActions_RunThroughTheBinaryOnEverySurface(t *testing.T) {
	stub := startStandaloneStubGitLab(t)
	inst := instanceForStub(t, stub.server)

	for _, surface := range AllSurfaces() {
		t.Run(string(surface), func(t *testing.T) {
			env := newEnv(t, inst)
			name := env.Name("elicited")
			script := newProjectFlowScript(name)
			// Scripted, and so private: the session is this subtest's and ends
			// with it, before the stub behind it is closed.
			session := env.Session(ServerConfig{
				Surface:      surface,
				Elicitation:  ElicitationScripted,
				Responder:    script.respond,
				Capabilities: CapabilitiesMinimal,
			})

			created := Do[createdProject](session, "interactive.project_create", nil)
			resolved := Do[createdProject](session, "discover_project.resolve",
				map[string]any{"remote_url": stub.server.URL + "/group/project.git"})

			if created.ID != 42 || created.Name != name {
				t.Errorf("the flow answered project %d named %q, want 42 named %q", created.ID, created.Name, name)
			}
			if asked := script.askedProperties(); !slices.Equal(asked, projectFlowPrompts) {
				t.Errorf("the flow asked for %v, want %v in that order, each once", asked, projectFlowPrompts)
			}
			if posted := stub.createdNames(); !slices.Contains(posted, name) {
				t.Errorf("GitLab was asked to create %v, want the elicited name %q among them", posted, name)
			}
			if resolved.ID != 42 || resolved.PathWithNamespace != "group/project" {
				t.Errorf("discovery answered %d %q, want 42 group/project", resolved.ID, resolved.PathWithNamespace)
			}

			assertStandaloneRecord(t, env, surface)
		})
	}
}

// assertStandaloneRecord flushes a test's record and checks what it says about
// the two standalone calls: each dispatched the action it named, the flow's
// elicitation was written down against the test, and nothing was failed.
func assertStandaloneRecord(t *testing.T, env *Env, surface Surface) {
	t.Helper()

	reporter := &capturedReporter{}
	lines := env.recorder.finish(reporter, e2ecalls.StatusPassed)
	for _, action := range []string{"interactive.project_create", "discover_project.resolve"} {
		t.Run(action, func(t *testing.T) {
			if call := callLineFor(t, lines, action); call.Dispatched != action {
				t.Errorf("%s on %s dispatched %q, want the action itself: the record credits nothing else",
					action, surface, call.Dispatched)
			}
		})
	}
	if !recordedElicitation(lines, "name") {
		t.Errorf("no elicitation asking for a name was recorded: %s", describeLines(lines))
	}
	if reporter.count() != 0 {
		t.Errorf("the flush failed calls that ran what they asked for: %s", reporter.reported())
	}
}

// recordedElicitation reports whether the record holds an elicitation that
// asked for the given property.
func recordedElicitation(lines []e2ecalls.Line, property string) bool {
	for _, line := range lines {
		call, isCall := line.(*e2ecalls.Call)
		if isCall && call.Method == methodElicit && slices.Contains(call.Arguments, property) {
			return true
		}
	}
	return false
}
