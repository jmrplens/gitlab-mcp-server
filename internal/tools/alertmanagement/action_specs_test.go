// action_specs_test.go contains canonical route tests for alert management actions.
// Tests exercise mutation error paths with a mock GitLab API.
package alertmanagement

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_MutationErrors verifies mutation route error paths.
func TestActionSpecs_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete, http.MethodPut:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{}`)
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
		{"gitlab_update_alert_metric_image", map[string]any{"project_id": "42", "alert_iid": 1, "metric_image_id": 1}},
		{"gitlab_upload_alert_metric_image", map[string]any{"project_id": "42", "alert_iid": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.name)
			}
		})
	}
}
