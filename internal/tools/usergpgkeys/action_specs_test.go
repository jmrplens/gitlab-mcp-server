// action_specs_test.go contains integration tests for the user GPG key tool
// closures in ActionSpecs routes with a mock GitLab API.
package usergpgkeys

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerGPGKeyJSON = `{"id":1,"primary_key_id":1,"key_id":"ABC123","public_key":"-----BEGIN PGP PUBLIC KEY BLOCK-----","created_at":"2026-01-01T00:00:00Z","user":{"id":1,"username":"admin"}}`

// TestActionSpecs_Metadata verifies user GPG key action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "usergpgkeys" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all registered GPG key routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "/gpg_keys") {
				testutil.RespondJSON(w, http.StatusOK, `[`+registerGPGKeyJSON+`]`)
			} else {
				testutil.RespondJSON(w, http.StatusOK, registerGPGKeyJSON)
			}
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerGPGKeyJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_gpg_keys", map[string]any{}},
		{"gitlab_list_gpg_keys_for_user", map[string]any{"user_id": 1}},
		{"gitlab_get_gpg_key", map[string]any{"key_id": 1}},
		{"gitlab_get_gpg_key_for_user", map[string]any{"user_id": 1, "key_id": 1}},
		{"gitlab_add_gpg_key", map[string]any{"key": "-----BEGIN PGP PUBLIC KEY BLOCK-----"}},
		{"gitlab_add_gpg_key_for_user", map[string]any{"user_id": 1, "key": "-----BEGIN PGP PUBLIC KEY BLOCK-----"}},
		{"gitlab_delete_gpg_key", map[string]any{"key_id": 1}},
		{"gitlab_delete_gpg_key_for_user", map[string]any{"user_id": 1, "key_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}
