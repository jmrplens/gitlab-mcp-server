package resourceevents

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	regIterationEventJSON  = `{"id":1,"action":"add","user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"iteration":{"id":5,"title":"Sprint 1","iid":1},"created_at":"2026-01-01T00:00:00Z"}`
	regIterationEventsJSON = `[` + regIterationEventJSON + `]`
	regWeightEventJSON     = `{"id":2,"user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"weight":5,"previous_weight":3,"created_at":"2026-01-01T00:00:00Z"}`
	regWeightEventsJSON    = `[` + regWeightEventJSON + `]`
	regLabelEventJSON      = `{"id":1,"action":"add","label":{"id":1,"name":"bug"},"user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"created_at":"2026-01-01T00:00:00Z"}`
	regLabelEventsJSON     = `[` + regLabelEventJSON + `]`
	regMilestoneEventJSON  = `{"id":1,"action":"add","milestone":{"id":1,"title":"v1.0","iid":1},"user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"created_at":"2026-01-01T00:00:00Z"}`
	regMilestoneEventsJSON = `[` + regMilestoneEventJSON + `]`
	regStateEventJSON      = `{"id":1,"state":"closed","user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"created_at":"2026-01-01T00:00:00Z"}`
	regStateEventsJSON     = `[` + regStateEventJSON + `]`
)

// TestActionSpecs_Metadata verifies canonical metadata for resource event actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specs := append(IssueActionSpecs(client), MergeRequestActionSpecs(client)...)

	if len(specs) != 15 {
		t.Fatalf("len(ActionSpecs) = %d, want 15", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "resourceevents" {
			t.Errorf("OwnerPackage for %s = %q, want resourceevents", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
		if !spec.ReadOnly || !spec.Idempotent {
			t.Errorf("%s should be read-only and idempotent", spec.Name)
		}
	}
}

// TestActionSpecs_CallThroughRoutes covers every resource event route.
func TestActionSpecs_CallThroughRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !respondResourceEventRoute(w, r, path) {
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := resourceEventSpecsByTool(t, append(IssueActionSpecs(client), MergeRequestActionSpecs(client)...))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_label_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
		{"gitlab_issue_label_event_get", map[string]any{"project_id": "42", "issue_iid": int64(1), "label_event_id": int64(1)}},
		{"gitlab_mr_label_event_list", map[string]any{"project_id": "42", "merge_request_iid": int64(1)}},
		{"gitlab_mr_label_event_get", map[string]any{"project_id": "42", "merge_request_iid": int64(1), "label_event_id": int64(1)}},
		{"gitlab_issue_milestone_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
		{"gitlab_issue_milestone_event_get", map[string]any{"project_id": "42", "issue_iid": int64(1), "milestone_event_id": int64(1)}},
		{"gitlab_mr_milestone_event_list", map[string]any{"project_id": "42", "merge_request_iid": int64(1)}},
		{"gitlab_mr_milestone_event_get", map[string]any{"project_id": "42", "merge_request_iid": int64(1), "milestone_event_id": int64(1)}},
		{"gitlab_issue_state_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
		{"gitlab_issue_state_event_get", map[string]any{"project_id": "42", "issue_iid": int64(1), "state_event_id": int64(1)}},
		{"gitlab_mr_state_event_list", map[string]any{"project_id": "42", "merge_request_iid": int64(1)}},
		{"gitlab_mr_state_event_get", map[string]any{"project_id": "42", "merge_request_iid": int64(1), "state_event_id": int64(1)}},
		{"gitlab_issue_iteration_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
		{"gitlab_issue_iteration_event_get", map[string]any{"project_id": "42", "issue_iid": int64(1), "iteration_event_id": int64(1)}},
		{"gitlab_issue_weight_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

func respondResourceEventRoute(w http.ResponseWriter, r *http.Request, path string) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch {
	case strings.Contains(path, "/resource_iteration_events/"):
		testutil.RespondJSON(w, http.StatusOK, regIterationEventJSON)
	case strings.HasSuffix(path, "/resource_iteration_events"):
		testutil.RespondJSON(w, http.StatusOK, regIterationEventsJSON)
	case strings.HasSuffix(path, "/resource_weight_events"):
		testutil.RespondJSON(w, http.StatusOK, regWeightEventsJSON)
	case strings.Contains(path, "/resource_label_events/"):
		testutil.RespondJSON(w, http.StatusOK, regLabelEventJSON)
	case strings.HasSuffix(path, "/resource_label_events"):
		testutil.RespondJSON(w, http.StatusOK, regLabelEventsJSON)
	case strings.Contains(path, "/resource_milestone_events/"):
		testutil.RespondJSON(w, http.StatusOK, regMilestoneEventJSON)
	case strings.HasSuffix(path, "/resource_milestone_events"):
		testutil.RespondJSON(w, http.StatusOK, regMilestoneEventsJSON)
	case strings.Contains(path, "/resource_state_events/"):
		testutil.RespondJSON(w, http.StatusOK, regStateEventJSON)
	case strings.HasSuffix(path, "/resource_state_events"):
		testutil.RespondJSON(w, http.StatusOK, regStateEventsJSON)
	default:
		return false
	}
	return true
}

func resourceEventSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestFormatIterationEventsMarkdown_NonEmpty verifies the iteration events formatter.
func TestFormatIterationEventsMarkdown_NonEmpty(t *testing.T) {
	md := FormatIterationEventsMarkdown(ListIterationEventsOutput{
		Events: []IterationEventOutput{
			{ID: 1, Action: "add", Iteration: IterationEventIterationOutput{ID: 5, Title: "Sprint 1"}, Username: "user", CreatedAt: "2026-01-01T00:00:00Z"},
		},
	})
	if md == "" || !strings.Contains(md, "Sprint 1") {
		t.Fatalf("unexpected markdown: %q", md)
	}
}

// TestFormatIterationEventMarkdown_NonEmpty verifies the single iteration event formatter.
func TestFormatIterationEventMarkdown_NonEmpty(t *testing.T) {
	md := FormatIterationEventMarkdown(IterationEventOutput{
		ID: 1, Action: "add", Iteration: IterationEventIterationOutput{ID: 5, Title: "Sprint 1"},
		Username: "user", ResourceType: "Issue", ResourceID: 10, CreatedAt: "2026-01-01T00:00:00Z",
	})
	if md == "" || !strings.Contains(md, "Sprint 1") {
		t.Fatalf("unexpected markdown: %q", md)
	}
}

// TestFormatWeightEventsMarkdown_NonEmpty verifies the weight events formatter.
func TestFormatWeightEventsMarkdown_NonEmpty(t *testing.T) {
	md := FormatWeightEventsMarkdown(ListWeightEventsOutput{
		Events: []WeightEventOutput{
			{ID: 2, Weight: 5, Username: "user", ResourceType: "Issue", ResourceID: 10, CreatedAt: "2026-01-01T00:00:00Z"},
		},
	})
	if md == "" || !strings.Contains(md, "5") {
		t.Fatalf("unexpected markdown: %q", md)
	}
}

// TestMarkdownHints_IterationAndWeight verifies the init() registered formatters
// for iteration and weight event types.
func TestMarkdownHints_IterationAndWeight(t *testing.T) {
	tests := []struct {
		name string
		val  any
	}{
		{"ListIterationEventsOutput", ListIterationEventsOutput{}},
		{"IterationEventOutput", IterationEventOutput{}},
		{"ListWeightEventsOutput", ListWeightEventsOutput{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toolutil.MarkdownForResult(tt.val)
			if result == nil {
				t.Fatalf("MarkdownForResult(%T) returned nil", tt.val)
			}
		})
	}
}
