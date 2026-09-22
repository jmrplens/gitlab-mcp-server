// action_specs_test.go contains unit tests for the DORA metrics
// [toolutil.ActionSpec] entries. GitLab serves DORA metrics at two scopes,
// project and group, and the package publishes one spec for each.
package dorametrics

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
//
// The alias list is compared whole rather than probed for one phrase. Only the
// leading scope word tells the two lists apart, and nothing outside this
// package holds them (no committed artifact carries aliases), so asserting a
// single phrase leaves the other five free to trade places between the scopes.
// They feed gitlab_find_action matching, where a crossed phrase points a model
// at the tool for the scope it did not ask for.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specByTool := doraMetricSpecsByTool(ActionSpecs(client))

	cases := []struct {
		tool    string
		related string
		aliases []string
	}{
		{
			tool:    "gitlab_get_project_dora_metrics",
			related: "dora_metrics.group",
			aliases: []string{
				"gitlab_get_project_dora_metrics",
				"project deployment frequency",
				"project lead time for changes",
				"project change failure rate",
				"project time to restore service",
				"project devops performance metrics",
			},
		},
		{
			tool:    "gitlab_get_group_dora_metrics",
			related: "dora_metrics.project",
			aliases: []string{
				"gitlab_get_group_dora_metrics",
				"group deployment frequency",
				"group lead time for changes",
				"group change failure rate",
				"group time to restore service",
				"group devops performance metrics",
			},
		},
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

			if !slices.Equal(spec.Aliases, tc.aliases) {
				t.Errorf("%s aliases =\n got %q\nwant %q", tc.tool, spec.Aliases, tc.aliases)
			}
			if !slices.Contains(spec.RelatedActions, tc.related) {
				t.Errorf("%s related actions missing sibling scope %q: %v", tc.tool, tc.related, spec.RelatedActions)
			}
		})
	}
}

// TestActionSpecs_ScopeProse verifies that the prose each spec serves names its
// own scope: the usage line names the id field a caller must pass and not its
// sibling's, and the description points a reader at the other scope's tool
// rather than at itself. The two scopes' prose differs by those words alone, so
// crossing them leaves every other assertion here passing and tells a model to
// pass a group id to the project tool.
func TestActionSpecs_ScopeProse(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specByTool := doraMetricSpecsByTool(ActionSpecs(client))

	cases := []struct {
		tool        string
		idField     string
		siblingID   string
		siblingTool string
	}{
		{"gitlab_get_project_dora_metrics", "project_id", "group_id", "gitlab_get_group_dora_metrics"},
		{"gitlab_get_group_dora_metrics", "group_id", "project_id", "gitlab_get_project_dora_metrics"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			spec, ok := specByTool[tc.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.tool)
			}
			if !strings.Contains(spec.Usage, tc.idField) {
				t.Errorf("%s usage does not name %s: %q", tc.tool, tc.idField, spec.Usage)
			}
			if strings.Contains(spec.Usage, tc.siblingID) {
				t.Errorf("%s usage names the sibling scope's %s: %q", tc.tool, tc.siblingID, spec.Usage)
			}
			if want := "See also: " + tc.siblingTool; !strings.Contains(spec.IndividualTool.Description, want) {
				t.Errorf("%s description does not carry %q: %q", tc.tool, want, spec.IndividualTool.Description)
			}
		})
	}
}

// TestDoraMetricReadSpec_ScopeDecidesTheProseAndNothingElse pins what the scope
// switch does and does not settle. Tags, edition, owner package, individual
// tool and the metric and interval enums are the same whatever the scope, and
// only the usage, the related action and the description are per scope, so a
// scope the switch has no case for publishes a tool that is registered and
// gated correctly and explains nothing, which is the state a third scope added
// without its case would ship in.
//
// The title is compared against the served spelling rather than against
// toolutil.TitleFromName of either argument: the scope name and the tool name
// are both strings in scope at that one call, so restating the call would pass
// with the arguments exchanged, which serves "Instance" as the title of a tool
// whose purpose the title is the only short statement of. The two enum lists
// are compared whole for the same reason, since each is a list of strings the
// other property would accept.
func TestDoraMetricReadSpec_ScopeDecidesTheProseAndNothingElse(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	spec := doraMetricReadSpec("instance", toolutil.RouteAction(client, GetProjectMetrics), "gitlab_get_instance_dora_metrics")

	if spec.OwnerPackage != "dorametrics" {
		t.Errorf("owner package = %q, want dorametrics", spec.OwnerPackage)
	}
	if spec.Edition != "premium" {
		t.Errorf("edition = %q, want premium", spec.Edition)
	}
	if spec.IndividualTool.Name != "gitlab_get_instance_dora_metrics" {
		t.Errorf("individual tool = %q, want gitlab_get_instance_dora_metrics", spec.IndividualTool.Name)
	}
	if want := "Get Instance Dora Metrics"; spec.IndividualTool.Title != want {
		t.Errorf("individual tool title = %q, want %q", spec.IndividualTool.Title, want)
	}
	if !slices.Contains(spec.Tags, "dora") || !slices.Contains(spec.Tags, "analytics") {
		t.Errorf("tags = %v, want dora and analytics", spec.Tags)
	}
	wantOverrides := []toolutil.InputSchemaOverride{
		{PropertyPath: "metric", Values: map[string]any{
			"enum": []any{"deployment_frequency", "lead_time_for_changes", "time_to_restore_service", "change_failure_rate"},
		}},
		{PropertyPath: "interval", Values: map[string]any{
			"enum": []any{"daily", "monthly", "all"},
		}},
	}
	if !reflect.DeepEqual(spec.InputSchemaOverrides, wantOverrides) {
		t.Errorf("input schema overrides =\n got %#v\nwant %#v", spec.InputSchemaOverrides, wantOverrides)
	}
	if spec.Usage != "" {
		t.Errorf("usage = %q, want empty for a scope the switch has no case for", spec.Usage)
	}
	if len(spec.RelatedActions) != 0 {
		t.Errorf("related actions = %v, want none for a scope the switch has no case for", spec.RelatedActions)
	}
	if spec.IndividualTool.Description != "" {
		t.Errorf("description = %q, want empty for a scope the switch has no case for", spec.IndividualTool.Description)
	}
}

// TestMarkdownHints_Output verifies that an Output reaching the shared Markdown
// registry renders the text the package's own formatter writes for a series
// whose metric is unknown. The registry is the path a tool result really takes,
// and it is registered with a closure: comparing the rendered text, rather than
// checking that something came back, is what holds that closure to the generic
// title the output alone can justify.
func TestMarkdownHints_Output(t *testing.T) {
	out := Output{Metrics: []MetricOutput{{Date: "2026-01-01", Value: 42.5}}}

	result := toolutil.MarkdownForResult(out)
	if result == nil {
		t.Fatal("expected non-nil result from MarkdownForResult(Output{})")
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
	}
	if want := FormatMarkdown(out, ""); text.Text != want {
		t.Errorf("registered formatter rendered\n got %q\nwant %q", text.Text, want)
	}
}
