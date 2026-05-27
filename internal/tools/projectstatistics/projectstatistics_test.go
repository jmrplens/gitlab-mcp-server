// projectstatistics_test.go contains unit tests for the project statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package projectstatistics

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestGet verifies Get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/statistics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"fetches":{"total":42,"days":[{"count":5,"date":"2026-01-01"}]}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.TotalFetches != 42 {
		t.Errorf("TotalFetches = %d", out.TotalFetches)
	}
	if len(out.Days) != 1 {
		t.Fatalf("Days len = %d", len(out.Days))
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatMarkdown verifies FormatMarkdown.
func TestFormatMarkdown(t *testing.T) {
	md := FormatMarkdown(GetOutput{TotalFetches: 42, Days: []DayStat{{Date: "2026-01-01", Count: 5}}})
	if !strings.Contains(md, "42") || !strings.Contains(md, "1 Jan 2026") {
		t.Error("missing content")
	}
}

// TestFormatMarkdown_Empty verifies the formatter handles empty days.
func TestFormatMarkdown_Empty(t *testing.T) {
	md := FormatMarkdown(GetOutput{TotalFetches: 0})
	if !strings.Contains(md, "0") {
		t.Error("expected zero total fetches")
	}
}

// TestGet_MissingProjectID verifies Get returns error for empty project_id.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestActionSpecs_Metadata verifies project statistics action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "projectstatistics" || specs[0].IndividualTool.Name != "gitlab_get_project_statistics" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if specs[0].Usage == "" {
		t.Fatal("project statistics ActionSpec should define usage")
	}
	if len(specs[0].Aliases) == 0 {
		t.Fatal("project statistics ActionSpec should define aliases")
	}
	if specs[0].ParameterGuidance["project_id"].SemanticRole == "" {
		t.Fatal("project statistics ActionSpec should define project_id parameter guidance")
	}
}

// TestActionSpecs_CallRoute verifies the project statistics canonical route.
func TestActionSpecs_CallRoute(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"fetches":{"total":42,"days":[{"count":5,"date":"2026-01-01"}]}}`)
	})
	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	result, err := spec.Route.Handler(t.Context(), map[string]any{"project_id": "1"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
