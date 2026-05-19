package dorametrics

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerMetricsJSON = `[{"date":"2026-01-01","value":42.5}]`

// TestActionSpecs_CallRoutes verifies both DORA metrics canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/dora/metrics") {
			testutil.RespondJSON(w, http.StatusOK, registerMetricsJSON)
		} else {
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specByTool := doraMetricSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_project_dora_metrics", map[string]any{"project_id": "42", "metric": "deployment_frequency"}},
		{"gitlab_get_group_dora_metrics", map[string]any{"group_id": "42", "metric": "deployment_frequency"}},
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

func doraMetricSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}

// TestMarkdownHints_Output verifies the init()-registered markdown formatter
// for Output produces non-nil content via MarkdownForResult.
func TestMarkdownHints_Output(t *testing.T) {
	md := toolutil.MarkdownForResult(Output{
		Metrics: []MetricOutput{{Date: "2026-01-01", Value: 42.5}},
	})
	if md == nil {
		t.Fatal("expected non-nil result from MarkdownForResult(Output{})")
	}
}
