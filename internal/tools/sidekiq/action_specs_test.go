// action_specs_test.go contains canonical route error tests for Sidekiq metrics.
package sidekiq

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_CallRouteErrors validates Sidekiq route error paths.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	specByTool := sidekiqSpecsByTool(ActionSpecs(client))

	tools := []string{
		"gitlab_get_sidekiq_queue_metrics",
		"gitlab_get_sidekiq_process_metrics",
		"gitlab_get_sidekiq_job_stats",
		"gitlab_get_sidekiq_compound_metrics",
	}
	for _, name := range tools {
		t.Run(name, func(t *testing.T) {
			spec, ok := specByTool[name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", name)
			}
			if _, err := spec.Route.Handler(t.Context(), map[string]any{}); err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}
