//go:build e2e

// dynamic_test.go verifies the default dynamic tool surface against a live
// GitLab instance. The tests exercise the three-tool workflow exposed by
// TOOL_SURFACE=dynamic and TOOL_SURFACE=dynamic-3: search for an action,
// describe its exact parameter schema, then execute the selected action.
package suite

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
)

// TestDynamicToolSurface_ExposesSearchDescribeExecuteOnly verifies that the
// dynamic E2E session exposes only the default dynamic-3 public surface.
//
// The test lists tools from [sess.dynamic] and asserts that the visible MCP
// catalog contains exactly gitlab_search_tools, gitlab_describe_tools, and
// gitlab_execute_tool. It also checks that regular individual or meta tools
// such as gitlab_project and gitlab_repository are not exposed directly. This
// protects the low-token contract for TOOL_SURFACE=dynamic and dynamic-3.
func TestDynamicToolSurface_ExposesSearchDescribeExecuteOnly(t *testing.T) {
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

	want := []string{"gitlab_describe_tools", "gitlab_execute_tool", "gitlab_search_tools"}
	if !slices.Equal(names, want) {
		t.Fatalf("dynamic tool names = %v, want %v", names, want)
	}
	for _, catalogTool := range []string{"gitlab_project", "gitlab_repository", "gitlab_find_action"} {
		if slices.Contains(names, catalogTool) {
			t.Fatalf("dynamic surface exposed catalog or parked tool %q in %v", catalogTool, names)
		}
	}
}

// TestDynamicToolSurface_SearchDescribeExecuteReadOnlyWorkflow verifies the
// full dynamic-3 workflow against real GitLab project data.
//
// The test creates a private project through the individual session, then uses
// [sess.dynamic] to search, describe, and execute read-only actions. It covers
// a project read, repository file read, standalone project discovery action,
// natural multi-intent search, and the destructive-action confirmation guard.
// The expected outcome is that dynamic mode can find and execute real catalog
// actions without exposing the underlying meta-tool catalog directly.
func TestDynamicToolSurface_SearchDescribeExecuteReadOnlyWorkflow(t *testing.T) {
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

	multiIntent := dynamicSearch(ctx, t, "discover project from remote url merge request list current user open authored", 10)
	requireSearchResult(t, multiIntent, "discover_project.resolve")
	requireSearchResult(t, multiIntent, "merge_request.list")

	result, err := sess.dynamic.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_execute_tool",
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

// dynamicProjectGet runs the search, describe, and execute sequence for the
// project.get action and returns the decoded project output. The helper keeps
// the workflow explicit in E2E assertions while sharing the schema checks that
// every dynamic action should satisfy before execution.
func dynamicProjectGet(ctx context.Context, t *testing.T, proj ProjectFixture) projects.Output {
	t.Helper()

	search := dynamicSearch(ctx, t, "project get by id", 5)
	requireSearchResult(t, search, "project.get")

	description := dynamicDescribe(ctx, t, "project.get")
	requireDescriptionParam(t, description, "project_id")
	requireDescriptionOutputParam(t, description, "id")

	out, err := callToolOn[projects.Output](ctx, sess.dynamic, "gitlab_execute_tool", dynamictools.ExecuteInput{
		Action: "project.get",
		Params: map[string]any{"project_id": proj.pidStr()},
	})
	requireNoError(t, err, "dynamic execute project.get")
	return out
}

// dynamicRepositoryFileGet runs the dynamic workflow for repository.file_get
// and reads the README generated by the project fixture. This validates that
// dynamic search can resolve long repository-content phrasing and that execute
// forwards file-specific parameters to the underlying GitLab repository file
// handler.
func dynamicRepositoryFileGet(ctx context.Context, t *testing.T, proj ProjectFixture, branch string) files.Output {
	t.Helper()

	search := dynamicSearch(ctx, t, "download repository file content from project ref", 5)
	requireSearchResult(t, search, "repository.file_get")

	description := dynamicDescribe(ctx, t, "repository.file_get")
	for _, param := range []string{"project_id", "file_path"} {
		requireDescriptionParam(t, description, param)
	}

	out, err := callToolOn[files.Output](ctx, sess.dynamic, "gitlab_execute_tool", dynamictools.ExecuteInput{
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

	search := dynamicSearch(ctx, t, "discover project from remote url", 5)
	requireSearchResult(t, search, "discover_project.resolve")

	description := dynamicDescribe(ctx, t, "discover_project.resolve")
	requireDescriptionParam(t, description, "remote_url")

	out, err := callToolOn[projectdiscovery.ResolveOutput](ctx, sess.dynamic, "gitlab_execute_tool", dynamictools.ExecuteInput{
		Action: "discover_project.resolve",
		Params: map[string]any{"remote_url": remoteURL},
	})
	requireNoError(t, err, "dynamic execute discover_project.resolve")
	return out
}

// dynamicSearch calls gitlab_search_tools and fails the current test if the
// dynamic search tool cannot return structured results for query.
func dynamicSearch(ctx context.Context, t *testing.T, query string, limit int) dynamictools.SearchOutput {
	t.Helper()

	out, err := callToolOn[dynamictools.SearchOutput](ctx, sess.dynamic, "gitlab_search_tools", dynamictools.SearchInput{
		Query: query,
		Limit: limit,
	})
	requireNoError(t, err, "dynamic search "+query)
	return out
}

// dynamicDescribe calls gitlab_describe_tools for one canonical action and
// returns the action description.
func dynamicDescribe(ctx context.Context, t *testing.T, action string) dynamictools.ActionDescription {
	t.Helper()

	out, err := callToolOn[dynamictools.DescribeOutput](ctx, sess.dynamic, "gitlab_describe_tools", dynamictools.DescribeInput{
		Action: action,
	})
	requireNoError(t, err, "dynamic describe "+action)
	if out.Count != 1 || len(out.Actions) != 1 {
		t.Fatalf("describe %s returned count=%d actions=%d, want exactly 1", action, out.Count, len(out.Actions))
	}
	description := out.Actions[0]
	if description.ID != action {
		t.Fatalf("describe ID = %q, want %q", description.ID, action)
	}
	return description
}

// requireSearchResult fails the current test when results does not include
// the expected canonical dynamic action ID.
func requireSearchResult(t *testing.T, results dynamictools.SearchOutput, want string) {
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
	t.Fatalf("search %q results = %v, want %q", results.Query, ids, want)
}

// requireDescriptionParam fails the current test when description does not
// advertise param as a required parameter or as an input schema property.
func requireDescriptionParam(t *testing.T, description dynamictools.ActionDescription, param string) {
	t.Helper()

	if slices.Contains(description.RequiredParams, param) {
		return
	}
	properties, ok := description.InputSchema["properties"].(map[string]any)
	if ok {
		if _, exists := properties[param]; exists {
			return
		}
	}
	t.Fatalf("describe %s missing input parameter %q; required=%v schema=%v", description.ID, param, description.RequiredParams, description.InputSchema)
}

// requireDescriptionOutputParam fails the current test when description does
// not expose the expected structured output schema property.
func requireDescriptionOutputParam(t *testing.T, description dynamictools.ActionDescription, param string) {
	t.Helper()

	properties, ok := description.OutputSchema["properties"].(map[string]any)
	if ok {
		if _, exists := properties[param]; exists {
			return
		}
	}
	t.Fatalf("describe %s missing output parameter %q; schema=%v", description.ID, param, description.OutputSchema)
}
