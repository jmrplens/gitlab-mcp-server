// action_specs_test.go contains canonical route error tests for plan limits.
package planlimits

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_CallRouteErrors validates plan limit route error paths.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	specByTool := planLimitSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_plan_limits", map[string]any{}},
		{"gitlab_change_plan_limits", map[string]any{"plan_name": "default"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}
