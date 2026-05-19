// action_specs_test.go contains integration tests for the user email tool
// closures in ActionSpecs routes with a mock GitLab API.
package useremails

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerEmailJSON = `{"id":1,"email":"test@example.com","confirmed_at":"2026-01-01T00:00:00Z"}`

// TestActionSpecs_Metadata verifies user email action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "useremails" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all registered user email routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "/emails") {
				testutil.RespondJSON(w, http.StatusOK, `[`+registerEmailJSON+`]`)
			} else {
				testutil.RespondJSON(w, http.StatusOK, registerEmailJSON)
			}
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerEmailJSON)
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
		{"gitlab_list_emails_for_user", map[string]any{"user_id": 1}},
		{"gitlab_get_email", map[string]any{"email_id": 1}},
		{"gitlab_add_email", map[string]any{"email": "new@example.com"}},
		{"gitlab_add_email_for_user", map[string]any{"user_id": 1, "email": "new@example.com"}},
		{"gitlab_delete_email", map[string]any{"email_id": 1}},
		{"gitlab_delete_email_for_user", map[string]any{"user_id": 1, "email_id": 1}},
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
