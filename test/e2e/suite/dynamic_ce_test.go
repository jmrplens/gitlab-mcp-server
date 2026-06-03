//go:build e2e && !enterprise

// dynamic_ce_test.go verifies the default dynamic tool surface against a live
// GitLab instance. The tests exercise the default two-tool workflow exposed by
// TOOL_SURFACE=dynamic: find an action with its exact parameter schema, then
// execute the selected action.
package suite

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
)

// TestDynamicToolSurface_ExposesFindExecuteOnly verifies that the dynamic E2E
// session exposes only the default public surface.
//
// The test lists tools from [sess.dynamic] and asserts that the visible MCP
// catalog contains exactly gitlab_find_action and gitlab_execute_action. It also
// checks that regular individual or meta tools are not
// exposed directly. This protects the low-token contract for TOOL_SURFACE=dynamic.
func TestDynamicToolSurface_ExposesFindExecuteOnly(t *testing.T) {
	t.Parallel()
	if sess.dynamic == nil {
		t.Skip("dynamic session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := sess.dynamic.ListTools(ctx, nil)
	requireNoError(t, err, "list dynamic tools")

	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	want := []string{"gitlab_execute_action", "gitlab_find_action"}
	if !slices.Equal(names, want) {
		t.Fatalf("dynamic tool names = %v, want %v", names, want)
	}
	for _, catalogTool := range []string{"gitlab_project", "gitlab_repository"} {
		if slices.Contains(names, catalogTool) {
			t.Fatalf("dynamic surface exposed catalog tool %q in %v", catalogTool, names)
		}
	}
}

// TestDynamicToolSurface_FindExecuteReadOnlyWorkflow verifies the full default
// dynamic workflow against real GitLab project data.
//
// The test creates a private project through the individual session, then uses
// [sess.dynamic] to find and execute read-only actions. It covers
// a project read, repository file read, standalone project discovery action,
// natural multi-intent find, and the destructive-action confirmation guard.
// The expected outcome is that dynamic mode can find and execute real catalog
// actions without exposing the underlying meta-tool catalog directly.
func TestDynamicToolSurface_FindExecuteReadOnlyWorkflow(t *testing.T) {
	t.Parallel()
	if sess.dynamic == nil {
		t.Skip("dynamic session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	proj := CreateProject(ctx, e2e, sess.individual)

	project := dynamicProjectGet(ctx, t, proj)
	requireTruef(t, project.ID == proj.ID, "project.get ID = %d, want %d", project.ID, proj.ID)
	requireTruef(t, project.PathWithNamespace == proj.Path, "project.get path = %q, want %q", project.PathWithNamespace, proj.Path)

	readme := dynamicRepositoryFileGet(ctx, t, proj, project.DefaultBranch)
	requireTruef(t, readme.FilePath == "README.md", "file_get file_path = %q, want README.md", readme.FilePath)
	requireTruef(t, strings.TrimSpace(readme.Content) != "", "file_get content should not be empty")

	resolved := dynamicDiscoverProject(ctx, t, project.HTTPURLToRepo)
	requireTruef(t, resolved.ID == proj.ID, "discover_project.resolve ID = %d, want %d", resolved.ID, proj.ID)
	requireTruef(t, resolved.PathWithNamespace == proj.Path, "discover_project.resolve path = %q, want %q", resolved.PathWithNamespace, proj.Path)

	multiIntent := dynamicFind(ctx, t, "discover project from remote url merge request list current user open authored", 10)
	requireFindResult(t, multiIntent, "discover_project.resolve")
	requireFindResult(t, multiIntent, "merge_request.list")

	result, err := sess.dynamic.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_execute_action",
		Arguments: dynamictools.ExecuteInput{
			Action: "project.delete",
			Params: map[string]any{"project_id": proj.pidStr()},
		},
	})
	requireNoError(t, err, "call dynamic destructive action without confirm")
	if result == nil || !result.IsError {
		t.Fatalf("project.delete without confirm result = %+v, want tool error", result)
	}
}

// dynamicProjectGet runs the find and execute sequence for the project.get
// action and returns the decoded project output. The helper keeps the workflow
// explicit in E2E assertions while sharing schema checks that every dynamic
// action should satisfy before execution.
func dynamicProjectGet(ctx context.Context, t *testing.T, proj ProjectFixture) projects.Output {
	t.Helper()

	result := dynamicFindAction(ctx, t, "project get by id", "project.get")
	requireFindParam(t, result, "project_id")
	requireFindOutputParam(t, result, "id")

	out, err := callToolOn[projects.Output](ctx, sess.dynamic, "gitlab_execute_action", dynamictools.ExecuteInput{
		Action: "project.get",
		Params: map[string]any{"project_id": proj.pidStr()},
	})
	requireNoError(t, err, "dynamic execute project.get")
	return out
}

// dynamicRepositoryFileGet runs the dynamic workflow for repository.file_get
// and reads the README generated by the project fixture. This validates that
// dynamic find can resolve long repository-content phrasing and that execute
// forwards file-specific parameters to the underlying GitLab repository file
// handler.
func dynamicRepositoryFileGet(ctx context.Context, t *testing.T, proj ProjectFixture, branch string) files.Output {
	t.Helper()

	result := dynamicFindAction(ctx, t, "download repository file content from project ref", "repository.file_get")
	for _, param := range []string{"project_id", "file_path"} {
		requireFindParam(t, result, param)
	}

	out, err := callToolOn[files.Output](ctx, sess.dynamic, "gitlab_execute_action", dynamictools.ExecuteInput{
		Action: "repository.file_get",
		Params: map[string]any{
			"project_id": proj.pidStr(),
			"file_path":  "README.md",
			"ref":        branch,
		},
	})
	requireNoError(t, err, "dynamic execute repository.file_get")
	return out
}

// dynamicDiscoverProject runs the dynamic workflow for the standalone project
// discovery action. It uses the HTTP clone URL returned by project.get so the
// test exercises the same remote URL parsing path users hit from git remotes.
func dynamicDiscoverProject(ctx context.Context, t *testing.T, remoteURL string) projectdiscovery.ResolveOutput {
	t.Helper()
	requireTruef(t, remoteURL != "", "project HTTP clone URL should not be empty")

	result := dynamicFindAction(ctx, t, "discover project from remote url", "discover_project.resolve")
	requireFindParam(t, result, "remote_url")

	out, err := callToolOn[projectdiscovery.ResolveOutput](ctx, sess.dynamic, "gitlab_execute_action", dynamictools.ExecuteInput{
		Action: "discover_project.resolve",
		Params: map[string]any{"remote_url": remoteURL},
	})
	requireNoError(t, err, "dynamic execute discover_project.resolve")
	return out
}

// dynamicFind calls gitlab_find_action and fails the current test if the
// dynamic find tool cannot return structured results for query.
func dynamicFind(ctx context.Context, t *testing.T, query string, limit int) dynamictools.FindOutput {
	t.Helper()

	out, err := callToolOn[dynamictools.FindOutput](ctx, sess.dynamic, "gitlab_find_action", dynamictools.FindInput{
		Query: query,
		Limit: limit,
	})
	requireNoError(t, err, "dynamic find "+query)
	return out
}

// dynamicFindAction returns one canonical action result from gitlab_find_action.
func dynamicFindAction(ctx context.Context, t *testing.T, query, action string) dynamictools.FindResult {
	t.Helper()

	results := dynamicFind(ctx, t, query, 5)
	for _, result := range results.Results {
		if result.ID == action {
			return result
		}
	}
	ids := make([]string, 0, len(results.Results))
	for _, result := range results.Results {
		ids = append(ids, result.ID)
	}
	t.Fatalf("find %q results = %v, want %q", query, ids, action)
	return dynamictools.FindResult{}
}

// requireFindResult fails the current test when results does not include
// the expected canonical dynamic action ID.
func requireFindResult(t *testing.T, results dynamictools.FindOutput, want string) {
	t.Helper()

	for _, result := range results.Results {
		if result.ID == want {
			return
		}
	}
	ids := make([]string, 0, len(results.Results))
	for _, result := range results.Results {
		ids = append(ids, result.ID)
	}
	t.Fatalf("find %q results = %v, want %q", results.Query, ids, want)
}

// requireFindParam fails the current test when result does not advertise param
// as a required parameter or as an input schema property.
func requireFindParam(t *testing.T, result dynamictools.FindResult, param string) {
	t.Helper()

	if slices.Contains(result.RequiredParams, param) {
		return
	}
	properties, ok := result.InputSchema["properties"].(map[string]any)
	if ok {
		if _, exists := properties[param]; exists {
			return
		}
	}
	t.Fatalf("find %s missing input parameter %q; required=%v schema=%v", result.ID, param, result.RequiredParams, result.InputSchema)
}

// requireFindOutputParam fails the current test when result does not expose the
// expected structured output schema property.
func requireFindOutputParam(t *testing.T, result dynamictools.FindResult, param string) {
	t.Helper()

	properties, ok := result.OutputSchema["properties"].(map[string]any)
	if ok {
		if _, exists := properties[param]; exists {
			return
		}
	}
	t.Fatalf("find %s missing output parameter %q; schema=%v", result.ID, param, result.OutputSchema)
}

// TestDynamicToolSurface_WriteActionConfirmation verifies that gitlab_execute_action
// accepts a destructive/write action with confirm=true and succeeds.
// This validates the write-action confirmation guard for branch creation.
func TestDynamicToolSurface_WriteActionConfirmation(t *testing.T) {
	t.Parallel()
	if sess.dynamic == nil {
		t.Skip("dynamic session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	proj := CreateProject(ctx, e2e, sess.individual)
	branchRef := proj.DefaultBranch
	if branchRef == "" {
		branchRef = defaultBranch
	}

	// Find branch.create action and verify its required parameters.
	findResult := dynamicFindAction(ctx, t, "create branch", "branch.create")
	requireFindParam(t, findResult, "project_id")
	requireFindParam(t, findResult, "branch_name")
	requireFindParam(t, findResult, "ref")

	// Execute branch.create with confirm=true — should succeed.
	branchName := "e2e-dynamic-branch-" + sanitizeTestName(t.Name())
	_, err := callToolOn[any](ctx, sess.dynamic, "gitlab_execute_action", dynamictools.ExecuteInput{
		Action:  "branch.create",
		Confirm: true,
		Params: map[string]any{
			"project_id":  proj.pidStr(),
			"branch_name": branchName,
			"ref":         branchRef,
		},
	})
	requireNoError(t, err, "branch.create with confirm=true")
	t.Logf("Branch %q created successfully via dynamic surface", branchName)
}

// TestDynamicToolSurface_DomainCoverage verifies that dynamic find+execute works
// across multiple domain areas: issues, merge requests, and pipelines.
// Each action is discovered via natural-language query and executed successfully.
func TestDynamicToolSurface_DomainCoverage(t *testing.T) {
	t.Parallel()
	if sess.dynamic == nil {
		t.Skip("dynamic session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	proj := CreateProject(ctx, e2e, sess.individual)
	branchRef := proj.DefaultBranch
	if branchRef == "" {
		branchRef = defaultBranch
	}

	t.Run("Domain/Issue/Create", func(t *testing.T) {
		// Find and execute issue creation.
		_ = dynamicFindAction(ctx, t, "create issue in project", "issue.create")
		out, err := callToolOn[any](ctx, sess.dynamic, "gitlab_execute_action", dynamictools.ExecuteInput{
			Action: "issue.create",
			Params: map[string]any{
				"project_id": proj.pidStr(),
				"title":      "Test issue from dynamic surface",
			},
		})
		requireNoError(t, err, "issue.create via dynamic surface")
		t.Logf("Issue created via dynamic surface: %+v", out)
	})

	t.Run("Domain/MergeRequest/List", func(t *testing.T) {
		// Find and execute merge request listing.
		_ = dynamicFindAction(ctx, t, "list merge requests", "merge_request.list")
		out, err := callToolOn[any](ctx, sess.dynamic, "gitlab_execute_action", dynamictools.ExecuteInput{
			Action: "merge_request.list",
			Params: map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "merge_request.list via dynamic surface")
		t.Logf("Merge request list via dynamic surface: %+v", out)
	})

	t.Run("Domain/Pipeline/Create", func(t *testing.T) {
		// Commit a minimal .gitlab-ci.yml so the pipeline can run.
		commitFileMeta(ctx, t, sess.meta, proj, branchRef, ".gitlab-ci.yml",
			"image: alpine\ntest:\n  script: echo 'ok'\n", "Add minimal CI config")
		// Find and execute pipeline creation.
		// Use "create a pipeline" to avoid matching pipeline.trigger_* actions.
		_ = dynamicFindAction(ctx, t, "create a new pipeline for a project ref", "pipeline.create")
		_, err := callToolOn[any](ctx, sess.dynamic, "gitlab_execute_action", dynamictools.ExecuteInput{
			Action: "pipeline.create",
			Params: map[string]any{
				"project_id": proj.pidStr(),
				"ref":        branchRef,
			},
		})
		requireNoError(t, err, "pipeline.create via dynamic surface")
	})
}
