package resourceevents

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	specs = append(specs, EpicActionSpecs(client)...)

	if len(specs) != 17 {
		t.Fatalf("len(ActionSpecs) = %d, want 17", len(specs))
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

// TestEpicActionSpecs_PremiumEdition verifies the group epic label-event specs
// are gated to the premium edition and carry non-generic discovery metadata.
func TestEpicActionSpecs_PremiumEdition(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specs := EpicActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(EpicActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.Edition != "premium" {
			t.Errorf("Edition for %s = %q, want premium", spec.Name, spec.Edition)
		}
		if len(spec.Aliases) < 2 {
			t.Errorf("Aliases for %s too generic: %v", spec.Name, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("RelatedActions for %s is empty", spec.Name)
		}
		if spec.Destructive {
			t.Errorf("%s should not be destructive", spec.Name)
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
	allSpecs := append(IssueActionSpecs(client), MergeRequestActionSpecs(client)...)
	allSpecs = append(allSpecs, EpicActionSpecs(client)...)
	byTool := resourceEventSpecsByTool(t, allSpecs)

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
		{"gitlab_list_group_epic_label_events", map[string]any{"group_id": "acme", "epic_iid": int64(1)}},
		{"gitlab_get_group_epic_label_event", map[string]any{"group_id": "acme", "epic_iid": int64(1), "label_event_id": int64(1)}},
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

// The guidance sections the iteration and weight event results end with, so
// each expectation below can pin the whole rendered document.
const (
	iterationListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use filters to narrow down iteration events by date or action\n"
	iterationCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `gitlab_issue_iteration_event_list` to see all iteration changes\n"
	weightListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use filters to narrow down weight events by date\n"
)

// TestFormatIterationEventsMarkdown_NonEmpty pins the whole list document of
// iteration events.
func TestFormatIterationEventsMarkdown_NonEmpty(t *testing.T) {
	got := FormatIterationEventsMarkdown(ListIterationEventsOutput{
		Events: []IterationEventOutput{
			{ID: 1, Action: "add", Iteration: &IterationOutput{ID: 5, Title: "Sprint 1"}, User: &EventUserOutput{Username: "user"}, CreatedAt: "2026-01-01T00:00:00Z"},
		},
	})

	want := "## Iteration Events (1)\n\n" +
		"| ID | Action | Iteration | User | Date |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 1 | add | Sprint 1 | user | 1 Jan 2026 00:00 UTC |\n" +
		iterationListHints
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatIterationEventMarkdown_NonEmpty pins the whole card of one
// iteration event.
func TestFormatIterationEventMarkdown_NonEmpty(t *testing.T) {
	got := FormatIterationEventMarkdown(IterationEventOutput{
		ID: 1, Action: "add", Iteration: &IterationOutput{ID: 5, Title: "Sprint 1"},
		User: &EventUserOutput{Username: "user"}, ResourceType: "Issue", ResourceID: 10, CreatedAt: "2026-01-01T00:00:00Z",
	})

	want := "## Iteration Event #1\n\n" +
		"- **Action**: add\n" +
		"- **Iteration**: Sprint 1\n" +
		"- **Iteration ID**: 5\n" +
		"- **User**: user\n" +
		"- **Resource Type**: Issue\n" +
		"- **Resource ID**: 10\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		iterationCardHints
	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatIterationEventMarkdown_DeletedIteration pins the card of an event
// whose iteration GitLab no longer sends: neither the title nor the ID is
// written, where the card used to read "(ID: 0)".
func TestFormatIterationEventMarkdown_DeletedIteration(t *testing.T) {
	got := FormatIterationEventMarkdown(IterationEventOutput{ID: 2, Action: "remove", User: &EventUserOutput{Username: "user"}})

	want := "## Iteration Event #2\n\n" +
		"- **Action**: remove\n" +
		"- **User**: user\n" +
		iterationCardHints
	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatWeightEventsMarkdown_NonEmpty pins the whole list document of
// weight events.
//
// The column this table used to render came from resource_type and resource_id,
// which the weight entity does not expose, so every row read as an empty type
// followed by #0; the issue the entity does name is rendered as the database ID
// it is, with no "#" in front of it to read as a per-project number.
func TestFormatWeightEventsMarkdown_NonEmpty(t *testing.T) {
	got := FormatWeightEventsMarkdown(ListWeightEventsOutput{
		Events: []WeightEventOutput{
			{ID: 2, Weight: 5, User: &EventUserOutput{Username: "user"}, IssueID: 10, CreatedAt: "2026-01-01T00:00:00Z"},
		},
	})

	want := "## Weight Events (1)\n\n" +
		"| ID | Weight | User | Issue ID | Date |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 2 | 5 | user | 10 | 1 Jan 2026 00:00 UTC |\n" +
		weightListHints
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatWeightEventsMarkdown_Empty pins the whole response of an issue with
// no weight events.
func TestFormatWeightEventsMarkdown_Empty(t *testing.T) {
	got := FormatWeightEventsMarkdown(ListWeightEventsOutput{})

	if want := "No weight events found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatIterationEventsMarkdown_Empty pins the whole response of an issue
// with no iteration events.
func TestFormatIterationEventsMarkdown_Empty(t *testing.T) {
	got := FormatIterationEventsMarkdown(ListIterationEventsOutput{})

	if want := "No iteration events found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%q\nwant:\n%q", got, want)
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
