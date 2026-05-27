// resourcegroups_test.go contains unit tests for the resource group MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package resourcegroups

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestListAll verifies ListAll.
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

// TestListAll_Error verifies ListAll when error.
func TestListAll_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListAll(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGet verifies Get.
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

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Key: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestEdit verifies Edit.
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

// TestEdit_Error verifies Edit when error.
func TestEdit_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Edit(t.Context(), client, EditInput{ProjectID: "1", Key: "x", ProcessMode: "invalid"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListUpcomingJobs verifies ListUpcomingJobs.
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

// TestListUpcomingJobs_Error verifies ListUpcomingJobs when error.
func TestListUpcomingJobs_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListUpcomingJobs(t.Context(), client, ListUpcomingJobsInput{ProjectID: "1", Key: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
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

// TestFormatGroupMarkdown verifies FormatGroupMarkdown.
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

// TestFormatJobsMarkdown_WithData verifies FormatJobsMarkdown when with data.
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

// TestFormatJobsMarkdown_Empty verifies FormatJobsMarkdown when empty.
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
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies resource group action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	specByTool := resourceGroupSpecsByTool(specs)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "resourcegroups" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_get_resource_group"].ParameterGuidance["key"].SemanticRole == "" {
		t.Fatal("gitlab_get_resource_group should define key parameter guidance")
	}
	if specByTool["gitlab_edit_resource_group"].ParameterGuidance["process_mode"].SemanticRole == "" {
		t.Fatal("gitlab_edit_resource_group should define process_mode parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ActionSpec route execution for all 4 individual tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates resource group canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := covNewResourceGroupsRouteSpecs(t)

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip for meta-tool (all 4 actions)
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ActionSpec route execution — API error paths
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteErrors validates resource group route API errors.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	specByTool := resourceGroupSpecsByTool(ActionSpecs(client))

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// covNewResourceGroupsRouteSpecs supports cov new resource groups route specs assertions in resourcegroups tests.
func covNewResourceGroupsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
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
	return resourceGroupSpecsByTool(ActionSpecs(client))
}

// resourceGroupSpecsByTool supports resource group specs by tool assertions in resourcegroups tests.
func resourceGroupSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
