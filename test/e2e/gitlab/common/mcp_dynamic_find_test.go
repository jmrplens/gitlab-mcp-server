//go:build e2e

// mcp_dynamic_find_test.go covers the default dynamic surface: the two tools
// it registers, the find-then-execute workflow a model follows on it, and the
// destructive-action confirmation guard that surface enforces.
//
// The dynamic surface is where a scenario is cheapest to prove and hardest to
// see: it registers gitlab_find_action and gitlab_execute_action and nothing
// else, so the catalog actions a test drives are reached through the execute
// tool by canonical id. The standalone utilities (discover_project.resolve and
// the interactive flows) are folded into the dynamic catalog but not the base
// one the harness projection reads, so this file reaches discover_project
// through [harness.ExecuteStandalone], which spells the same execute call the
// projecting verbs would and credits it to the action.

package common

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branches"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// findActionTool is the discovery half of the dynamic surface; its execute
// half, executeActionTool, is declared alongside the interactive-flow constants
// in mcp_modes_test.go.
const findActionTool = "gitlab_find_action"

// TestDynamic_ExposesFindExecuteOnly checks that the default dynamic surface
// registers exactly the two tools its low-token contract promises and none of
// the domain dispatchers a meta surface would list.
//
// Replaces: TestDynamicToolSurface_ExposesFindExecuteOnly
func TestDynamic_ExposesFindExecuteOnly(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	tools := s.Tools()
	want := map[string]bool{findActionTool: false, executeActionTool: false}
	extra := make([]string, 0)
	for _, name := range tools {
		if _, expected := want[name]; expected {
			want[name] = true
			continue
		}
		extra = append(extra, name)
	}
	for name, seen := range want {
		if !seen {
			e.T.Errorf("the dynamic surface does not register %s; it served %v", name, tools)
		}
	}
	if len(extra) > 0 {
		e.T.Errorf("the dynamic surface registered tools beyond find and execute: %v", extra)
	}
	for _, dispatcher := range []string{"gitlab_project", "gitlab_issue", "gitlab_group"} {
		t.Run(dispatcher, func(t *testing.T) {
			for _, name := range tools {
				if name == dispatcher {
					t.Errorf("the dynamic surface exposed the meta dispatcher %s", dispatcher)
				}
			}
		})
	}
}

// TestDynamic_FindExecuteReadWorkflow follows the read-only workflow a model
// runs on the dynamic surface: find an action to learn its schema, execute it
// by canonical id, and read the answer. It covers a project read, a repository
// file read, the standalone project-discovery utility, a multi-intent find,
// and the confirmation guard a destructive action is stopped by.
//
// Replaces: TestDynamicToolSurface_FindExecuteReadOnlyWorkflow
func TestDynamic_FindExecuteReadWorkflow(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("dynread"))
	s := e.On(harness.SurfaceDynamic)

	// project.get: find teaches the schema, execute returns the project.
	getResult := findAction(e, s, "project get by id", actionProjectGet)
	requireFindParam(e, getResult, "project_id")
	requireFindOutputParam(e, getResult, "id")
	got := harness.Do[projects.Output](s, actionProjectGet, map[string]any{"project_id": project.IDParam()})
	if got.ID != project.ID || got.PathWithNamespace != project.Path {
		e.T.Errorf("project.get answered %d (%s), want %d (%s)", got.ID, got.PathWithNamespace, project.ID, project.Path)
	}

	// repository.file_get: the README the project fixture created.
	fileResult := findAction(e, s, "download repository file content from a project ref", actionRepositoryFileGet)
	for _, param := range []string{"project_id", "file_path"} {
		requireFindParam(e, fileResult, param)
	}
	readme := harness.Do[files.Output](s, actionRepositoryFileGet, map[string]any{
		"project_id": project.IDParam(), "file_path": "README.md", "ref": got.DefaultBranch,
	})
	if readme.FilePath != "README.md" || strings.TrimSpace(readme.Content) == "" {
		e.T.Errorf("repository.file_get answered file_path=%q with %d bytes of content, want README.md non-empty", readme.FilePath, len(readme.Content))
	}

	// discover_project.resolve: a standalone utility, reached through the
	// execute tool but not projected by the base catalog.
	discoverResult := findAction(e, s, "discover a project from a git remote url", actionDiscoverProjectResolve)
	requireFindParam(e, discoverResult, "remote_url")
	if got.HTTPURLToRepo == "" {
		e.T.Fatal("project.get returned no http_url_to_repo to resolve")
	}
	resolved := harness.ExecuteStandalone[projectdiscovery.ResolveOutput](s, actionDiscoverProjectResolve, map[string]any{"remote_url": got.HTTPURLToRepo})
	if resolved.ID != project.ID || resolved.PathWithNamespace != project.Path {
		e.T.Errorf("discover_project.resolve answered %d (%s), want %d (%s)", resolved.ID, resolved.PathWithNamespace, project.ID, project.Path)
	}

	// A multi-intent query returns actions for each intent it names.
	multi := findActions(e, s, "discover project from remote url and list merge requests for the current user", 10)
	for _, want := range []harness.ActionID{actionDiscoverProjectResolve, actionMergeRequestList} {
		t.Run("multi-intent/"+string(want), func(t *testing.T) {
			if !findListed(multi, want) {
				t.Errorf("the multi-intent find did not return %s: %v", want, findIDs(multi))
			}
		})
	}

	// project.delete without a confirmation is refused, not run.
	refused := harness.Refused(s, actionProjectDelete, map[string]any{"project_id": project.IDParam()},
		harness.FailureNeedsConfirmation, harness.WithoutConfirmation())
	if !strings.Contains(strings.ToLower(refused), "confirm=true") {
		e.T.Errorf("the dynamic delete refusal does not tell the caller to re-send with confirm=true: %s", firstLine(refused))
	}
}

// TestDynamic_WriteActionConfirmation checks the other side of the guard: a
// destructive write executes when the execute call carries confirm=true.
//
// Replaces: TestDynamicToolSurface_WriteActionConfirmation
func TestDynamic_WriteActionConfirmation(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("dynwrite"))
	s := e.On(harness.SurfaceDynamic)

	findResult := findAction(e, s, "create a branch", actionBranchCreate)
	for _, param := range []string{"project_id", "branch_name", "ref"} {
		requireFindParam(e, findResult, param)
	}

	name := e.Name("dyn-branch")
	created := harness.Do[branches.Output](s, actionBranchCreate, map[string]any{
		"project_id": project.IDParam(), "branch_name": name, "ref": project.DefaultBranch,
	})
	if created.Name != name {
		e.T.Errorf("branch.create with confirm answered %q, want %q", created.Name, name)
	}
}

// TestDynamic_DomainCoverage drives find and execute across three domains,
// each discovered by a natural-language query and executed by canonical id: an
// issue create, a merge request list, and a pipeline create.
//
// Replaces: TestDynamicToolSurface_DomainCoverage
func TestDynamic_DomainCoverage(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("dyndomain"))
	s := e.On(harness.SurfaceDynamic)

	t.Run("issue.create", func(t *testing.T) {
		_ = findAction(e, s, "create an issue in a project", actionIssueCreate)
		const title = "dynamic surface issue"
		out := harness.Do[issues.Output](s, actionIssueCreate, map[string]any{"project_id": project.IDParam(), "title": title})
		if out.Title != title || out.IID == 0 {
			t.Errorf("issue.create answered title=%q iid=%d, want %q and a real IID", out.Title, out.IID, title)
		}
	})

	t.Run("merge_request.list", func(t *testing.T) {
		_ = findAction(e, s, "list the merge requests of a project", actionMergeRequestList)
		out := harness.Do[mergerequests.ListOutput](s, actionMergeRequestList, map[string]any{"project_id": project.IDParam()})
		if out.MergeRequests == nil {
			t.Error("merge_request.list answered a nil list rather than an empty one")
		}
	})

	t.Run("pipeline.create", func(t *testing.T) {
		fixture.CIFile(e, project)
		_ = findAction(e, s, "create a new pipeline for a project ref", actionPipelineCreate)
		out := harness.Do[pipelines.DetailOutput](s, actionPipelineCreate, map[string]any{
			"project_id": project.IDParam(), "ref": project.DefaultBranch,
		})
		if out.ID == 0 {
			t.Errorf("pipeline.create answered %+v, want a pipeline with an id", out)
		}
	})
}

// findAction runs gitlab_find_action for a query and returns the one result
// whose id is the wanted action, failing the test when the search does not
// surface it.
func findAction(e *harness.Env, s *harness.Session, query string, want harness.ActionID) dynamictools.FindResult {
	e.T.Helper()

	out := findActions(e, s, query, 5)
	for _, result := range out.Results {
		if result.ID == string(want) {
			return result
		}
	}
	e.T.Fatalf("find %q did not return %s: %v", query, want, findIDs(out))
	return dynamictools.FindResult{}
}

// findActions runs one gitlab_find_action call and decodes its answer. It goes
// through Raw because find is a standalone tool the projection does not spell,
// and its result is not coverage of an action: it is discovery, and the audit
// credits no action for it.
func findActions(e *harness.Env, s *harness.Session, query string, limit int) dynamictools.FindOutput {
	e.T.Helper()

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      findActionTool,
		Arguments: dynamictools.FindInput{Query: query, Limit: limit},
	})
	if err != nil {
		e.T.Fatalf("%s %q: %v", findActionTool, query, err)
	}
	if result == nil || result.IsError {
		e.T.Fatalf("%s %q answered an error: %s", findActionTool, query, rawText(result))
	}
	var out dynamictools.FindOutput
	if decodeErr := decodeStructured(result, &out); decodeErr != nil {
		e.T.Fatalf("decoding the %s answer for %q: %v", findActionTool, query, decodeErr)
	}
	return out
}

// findListed reports whether a find answer includes an action id.
func findListed(out dynamictools.FindOutput, want harness.ActionID) bool {
	for _, result := range out.Results {
		if result.ID == string(want) {
			return true
		}
	}
	return false
}

// findIDs lists the ids a find answer returned, for a failure message.
func findIDs(out dynamictools.FindOutput) []string {
	ids := make([]string, 0, len(out.Results))
	for _, result := range out.Results {
		ids = append(ids, result.ID)
	}
	return ids
}

// requireFindParam checks that a find result advertises a required parameter,
// either in its required list or as an input-schema property, which is what a
// model reads to build the execute call.
func requireFindParam(e *harness.Env, result dynamictools.FindResult, param string) {
	e.T.Helper()

	if slices.Contains(result.RequiredParams, param) {
		return
	}
	if properties, ok := result.InputSchema["properties"].(map[string]any); ok {
		if _, exists := properties[param]; exists {
			return
		}
	}
	e.T.Errorf("find %s does not advertise the parameter %q: required=%v", result.ID, param, result.RequiredParams)
}

// requireFindOutputParam checks that a find result advertises an output-schema
// property, which is what a model reads to know what the action returns.
func requireFindOutputParam(e *harness.Env, result dynamictools.FindResult, param string) {
	e.T.Helper()

	if properties, ok := result.OutputSchema["properties"].(map[string]any); ok {
		if _, exists := properties[param]; exists {
			return
		}
	}
	e.T.Errorf("find %s does not advertise the output field %q: schema=%v", result.ID, param, result.OutputSchema)
}

// decodeStructured decodes a raw tool result's structured content into a value.
func decodeStructured(result *mcp.CallToolResult, into any) error {
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, into)
}
