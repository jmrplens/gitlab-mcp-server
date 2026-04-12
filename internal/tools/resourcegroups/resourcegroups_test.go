// resourcegroups_test.go contains unit tests for the resource group MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package resourcegroups

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

const fmtUnexpErr = "unexpected error: %v"

// TestListAll verifies the behavior of list all.
func TestListAll(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"key":"production","process_mode":"unordered"}]`)
	}))
	out, err := ListAll(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 1 || out.Groups[0].Key != "production" {
		t.Errorf("unexpected groups: %+v", out.Groups)
	}
}

// TestListAll_Error verifies that ListAll handles the error scenario correctly.
func TestListAll_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListAll(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups/production" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"key":"production","process_mode":"unordered"}`)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", Key: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ProcessMode != "unordered" {
		t.Errorf("expected unordered, got %s", out.ProcessMode)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Key: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestEdit verifies the behavior of edit.
func TestEdit(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups/production" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"key":"production","process_mode":"newest_first"}`)
	}))
	out, err := Edit(t.Context(), client, EditInput{ProjectID: "1", Key: "production", ProcessMode: "newest_first"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ProcessMode != "newest_first" {
		t.Errorf("expected newest_first, got %s", out.ProcessMode)
	}
}

// TestEdit_Error verifies that Edit handles the error scenario correctly.
func TestEdit_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Edit(t.Context(), client, EditInput{ProjectID: "1", Key: "x", ProcessMode: "invalid"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListUpcomingJobs verifies the behavior of list upcoming jobs.
func TestListUpcomingJobs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/resource_groups/production/upcoming_jobs" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"deploy","status":"pending","stage":"deploy"}]`)
	}))
	out, err := ListUpcomingJobs(t.Context(), client, ListUpcomingJobsInput{ProjectID: "1", Key: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 1 || out.Jobs[0].Name != "deploy" {
		t.Errorf("unexpected jobs: %+v", out.Jobs)
	}
}

// TestListUpcomingJobs_Error verifies that ListUpcomingJobs handles the error scenario correctly.
func TestListUpcomingJobs_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListUpcomingJobs(t.Context(), client, ListUpcomingJobsInput{ProjectID: "1", Key: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Groups: []ResourceGroupItem{{ID: 1, Key: "prod", ProcessMode: "unordered"}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty groups
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the behavior of cov format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Groups: nil})
	if !strings.Contains(md, "No resource groups found") {
		t.Errorf("expected empty message, got:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatGroupMarkdown
// ---------------------------------------------------------------------------.

// TestFormatGroupMarkdown verifies the behavior of cov format group markdown.
func TestFormatGroupMarkdown(t *testing.T) {
	md := FormatGroupMarkdown(ResourceGroupItem{ID: 42, Key: "staging", ProcessMode: "oldest_first"})
	for _, want := range []string{
		"## Resource Group",
		"**ID**: 42",
		"**Key**: staging",
		"**Process Mode**: oldest_first",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatJobsMarkdown — with data and empty
// ---------------------------------------------------------------------------.

// TestFormatJobsMarkdown_WithData verifies the behavior of cov format jobs markdown with data.
func TestFormatJobsMarkdown_WithData(t *testing.T) {
	md := FormatJobsMarkdown(ListUpcomingJobsOutput{
		Jobs: []JobItem{
			{ID: 10, Name: "deploy", Status: "pending", Stage: "deploy"},
			{ID: 11, Name: "build", Status: "created", Stage: "build"},
		},
	})
	for _, want := range []string{
		"## Upcoming Jobs",
		"| ID |",
		"| 10 |",
		"| 11 |",
		"deploy",
		"build",
		"pending",
		"created",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatJobsMarkdown_Empty verifies the behavior of cov format jobs markdown empty.
func TestFormatJobsMarkdown_Empty(t *testing.T) {
	md := FormatJobsMarkdown(ListUpcomingJobsOutput{Jobs: nil})
	if !strings.Contains(md, "No upcoming jobs") {
		t.Errorf("expected empty message, got:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip for all 4 individual tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates cov register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := covNewResourceGroupsMCPSession(t, false)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_resource_groups", map[string]any{"project_id": "1"}},
		{"get", "gitlab_get_resource_group", map[string]any{"project_id": "1", "key": "production"}},
		{"edit", "gitlab_edit_resource_group", map[string]any{"project_id": "1", "key": "production", "process_mode": "newest_first"}},
		{"list_upcoming_jobs", "gitlab_list_resource_group_upcoming_jobs", map[string]any{"project_id": "1", "key": "production"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip for meta-tool (all 4 actions)
// ---------------------------------------------------------------------------.

// TestRegisterMeta_CallAllThroughMCP validates cov register meta call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterMeta_CallAllThroughMCP(t *testing.T) {
	session := covNewResourceGroupsMCPSession(t, true)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"list", "list", map[string]any{"project_id": "1"}},
		{"get", "get", map[string]any{"project_id": "1", "key": "production"}},
		{"edit", "edit", map[string]any{"project_id": "1", "key": "production", "process_mode": "newest_first"}},
		{"list_upcoming_jobs", "list_upcoming_jobs", map[string]any{"project_id": "1", "key": "production"}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_resource_group",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(gitlab_resource_group/%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(gitlab_resource_group/%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(gitlab_resource_group/%s) returned IsError=true", tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — API error paths through RegisterTools
// ---------------------------------------------------------------------------.

// TestRegisterTools_APIErrors validates cov register tools a p i errors across multiple scenarios using table-driven subtests.
func TestRegisterTools_APIErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_resource_groups", map[string]any{"project_id": "1"}},
		{"get_error", "gitlab_get_resource_group", map[string]any{"project_id": "1", "key": "x"}},
		{"edit_error", "gitlab_edit_resource_group", map[string]any{"project_id": "1", "key": "x", "process_mode": "bad"}},
		{"jobs_error", "gitlab_list_resource_group_upcoming_jobs", map[string]any{"project_id": "1", "key": "x"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) unexpected transport error: %v", tt.tool, err)
			}
			if !result.IsError {
				t.Errorf("CallTool(%s) expected IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// covNewResourceGroupsMCPSession is an internal helper for the resourcegroups package.
func covNewResourceGroupsMCPSession(t *testing.T, meta bool) *mcp.ClientSession {
	t.Helper()

	covGroupJSON := `{"id":1,"key":"production","process_mode":"unordered"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/resource_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covGroupJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/resource_groups/production", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covGroupJSON)
	})

	handler.HandleFunc("PUT /api/v4/projects/1/resource_groups/production", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"key":"production","process_mode":"newest_first"}`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/resource_groups/production/upcoming_jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"deploy","status":"pending","stage":"deploy"}]`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)

	if meta {
		RegisterMeta(server, client)
	} else {
		RegisterTools(server, client)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
