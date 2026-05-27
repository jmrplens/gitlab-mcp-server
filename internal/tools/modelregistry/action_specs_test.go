// action_specs_test.go contains integration tests for the model registry tool
// closures in ActionSpecs routes with a mock GitLab API.
package modelregistry

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_Metadata verifies model registry action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "modelregistry" || !specs[0].ReadOnly || !specs[0].Idempotent {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if specs[0].Usage == "" {
		t.Fatalf("Usage for %s is empty", specs[0].Name)
	}
	if len(specs[0].Aliases) == 0 {
		t.Fatalf("Aliases for %s are empty", specs[0].Name)
	}
}

// TestActionSpecs_CallRoute verifies the model registry download route executes successfully.
func TestActionSpecs_CallRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/ml_models/") {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("model-binary-data"))
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)

	result, err := specs[0].Route.Handler(t.Context(), map[string]any{
		"project_id":       "42",
		"model_version_id": "7",
		"path":             "models",
		"filename":         "model.bin",
	})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}
