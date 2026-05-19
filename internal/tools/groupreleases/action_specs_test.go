package groupreleases

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerReleasesJSON = `[{"tag_name":"v1.0","description":"Release","name":"v1.0","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","_links":{"self":"https://gl.example.com"}}]`

// TestActionSpecs_Metadata verifies group release action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "groupreleases" || !specs[0].ReadOnly || !specs[0].Idempotent {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
}

// TestActionSpecs_CallRoute verifies the group release list route executes successfully.
func TestActionSpecs_CallRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, registerReleasesJSON)
		} else {
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	result, err := specByTool["gitlab_group_release_list"].Route.Handler(t.Context(), map[string]any{"group_id": "42"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}
