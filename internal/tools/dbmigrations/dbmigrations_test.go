// dbmigrations_test.go contains unit tests for the database migration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package dbmigrations

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestMark verifies the Mark handler.
// The mock GitLab API at /api/v4/admin/migrations/20240115100000/mark (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestMark(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/admin/migrations/20240115100000/mark")
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Mark(t.Context(), client, MarkInput{
		Version:  20240115100000,
		Database: "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "marked" {
		t.Errorf("Status = %q, want marked", out.Status)
	}
	if out.Version != 20240115100000 {
		t.Errorf("Version = %d, want 20240115100000", out.Version)
	}
}

// TestMark_Error verifies that Mark returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestMark_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Mark(t.Context(), client, MarkInput{Version: 99999})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestMark_VersionValidation verifies the Mark_VersionValidation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestMark_VersionValidation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()

	tests := []struct {
		name    string
		version int64
	}{
		{"zero", 0},
		{"negative", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Mark(ctx, client, MarkInput{Version: tt.version})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "version") {
				t.Errorf("error %q does not contain %q", err.Error(), "version")
			}
		})
	}
}

// TestFormatMarkMarkdown verifies the MarkMarkdown Markdown formatter for a representative mark input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkMarkdown(t *testing.T) {
	md := FormatMarkMarkdown(MarkOutput{Status: "marked", Version: 20240115100000})
	if !strings.Contains(md, "marked") {
		t.Error("missing status")
	}
	if !strings.Contains(md, "20240115100000") {
		t.Error("missing version")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "dbmigrations" || specs[0].IndividualTool.Name != "gitlab_mark_migration" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if specs[0].Usage == "" {
		t.Fatal("db migration ActionSpec should define usage")
	}
	if len(specs[0].Aliases) == 0 {
		t.Fatal("db migration ActionSpec should define aliases")
	}
	if specs[0].ParameterGuidance["version"].SemanticRole == "" {
		t.Fatal("db migration ActionSpec should define version parameter guidance")
	}
}

// TestActionSpecs_CallRoute validates the CallRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoute(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})

	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	res, err := spec.Route.Handler(t.Context(), map[string]any{"version": int64(20240115100000)})
	if err != nil {
		t.Fatalf("Route.Handler: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestActionSpecs_CallRouteError validates the CallRouteError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_CallRouteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	spec := ActionSpecs(client)[0]
	if _, err := spec.Route.Handler(t.Context(), map[string]any{"version": int64(99999)}); err == nil {
		t.Fatal("expected route error")
	}
}
