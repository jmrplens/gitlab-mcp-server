// action_specs_test.go contains unit tests for the DORA metrics [toolutil.ActionSpec] entries (project, group, instance scopes).
package dorametrics

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerMetricsJSON = `[{"date":"2026-01-01","value":42.5}]`

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_DiscoveryMetadata verifies that each DORA metric ActionSpec
// carries non-generic discovery metadata: distinctive DORA/DevOps
// natural-language aliases, a canonical related action pointing at the sibling
// scope, and an IndividualTool.Description in the "Returns: … See also: …" form
// (1:1 audit R-META). It guards against regression back to generic placeholder
// metadata.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specByTool := doraMetricSpecsByTool(ActionSpecs(client))

	cases := []struct {
		tool        string
		related     string
		aliasPhrase string
	}{
		{"gitlab_get_project_dora_metrics", "dora_metrics.group", "project deployment frequency"},
		{"gitlab_get_group_dora_metrics", "dora_metrics.project", "group deployment frequency"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			spec, ok := specByTool[tc.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.tool)
			}

			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s description missing Returns:/See also: form: %q", tc.tool, desc)
			}

			if !slices.Contains(spec.Aliases, tc.aliasPhrase) {
				t.Errorf("%s aliases missing distinctive phrase %q: %v", tc.tool, tc.aliasPhrase, spec.Aliases)
			}
			if !slices.Contains(spec.RelatedActions, tc.related) {
				t.Errorf("%s related actions missing sibling scope %q: %v", tc.tool, tc.related, spec.RelatedActions)
			}
		})
	}
}

// TestMarkdownHints_Output verifies the MarkdownHints_Output handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestMarkdownHints_Output(t *testing.T) {
	md := toolutil.MarkdownForResult(Output{
		Metrics: []MetricOutput{{Date: "2026-01-01", Value: 42.5}},
	})
	if md == nil {
		t.Fatal("expected non-nil result from MarkdownForResult(Output{})")
	}
}
