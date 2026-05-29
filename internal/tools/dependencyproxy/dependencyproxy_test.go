// dependencyproxy_test.go contains unit tests for the dependencyproxy MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package dependencyproxy

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestPurge verifies Purge.
func TestPurge(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/5/dependency_proxy/cache" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPurge_Error verifies Purge when error.
func TestPurge_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestActionSpecs_Metadata verifies dependency proxy action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "dependencyproxy" || specs[0].IndividualTool.Name != "gitlab_purge_dependency_proxy" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if specs[0].Usage == "" {
		t.Fatal("dependency proxy ActionSpec should define usage")
	}
	if len(specs[0].Aliases) == 0 {
		t.Fatal("dependency proxy ActionSpec should define aliases")
	}
	if specs[0].ParameterGuidance["group_id"].SemanticRole == "" {
		t.Fatal("dependency proxy ActionSpec should define group_id parameter guidance")
	}
}

// TestActionSpecs_CallRoute verifies dependency proxy canonical route execution.
func TestActionSpecs_CallRoute(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	res, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "5"})
	if err != nil {
		t.Fatalf("Route.Handler: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestActionSpecs_CallRouteError verifies the dependency proxy route error path.
func TestActionSpecs_CallRouteError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	})

	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	if _, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "5"}); err == nil {
		t.Fatal("expected route error")
	}
}
