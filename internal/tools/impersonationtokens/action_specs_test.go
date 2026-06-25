// action_specs_test.go contains unit tests for the impersonation token [toolutil.ActionSpec] entries.
package impersonationtokens

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerTokenJSON  = `{"id":1,"name":"tok","scopes":["api"],"active":true,"impersonation":true,"revoked":false}`
	registerTokensJSON = `[{"id":1,"name":"tok","scopes":["api"],"active":true,"impersonation":true,"revoked":false}]`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "impersonationtokens" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_RMetaDescriptions validates that every user-token action
// carries a non-generic individual-tool description in the "Returns: … See
// also: …" form and a RelatedActions list (R-META; 1:1 audit).
func TestActionSpecs_RMetaDescriptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, spec := range ActionSpecs(client) {
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: description missing Returns:/See also: form: %q", spec.IndividualTool.Name, desc)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: expected RelatedActions to be set", spec.IndividualTool.Name)
		}
		if spec.Usage == "Use to execute impersonationtokens domain action." {
			t.Errorf("%s: Usage is still the generic placeholder", spec.IndividualTool.Name)
		}
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/impersonation_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerTokensJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/impersonation_tokens/"):
			testutil.RespondJSON(w, http.StatusOK, registerTokenJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/impersonation_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerTokenJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerTokenJSON)
		case r.Method == http.MethodDelete:
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
		{"gitlab_list_impersonation_tokens", map[string]any{"user_id": 1}},
		{"gitlab_get_impersonation_token", map[string]any{"user_id": 1, "token_id": 1}},
		{"gitlab_create_impersonation_token", map[string]any{"user_id": 1, "name": "tok", "scopes": []any{"api"}}},
		{"gitlab_revoke_impersonation_token", map[string]any{"user_id": 1, "token_id": 1}},
		{"gitlab_create_personal_access_token", map[string]any{"user_id": 1, "name": "tok", "scopes": []any{"api"}}},
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
