// Package resourceevents register_test exercises the iteration/weight event
// handler closures in RegisterTools via MCP roundtrip, plus the uncovered
// markdown formatters for iteration and weight events.
package resourceevents

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const regIterationEventJSON = `{"id":1,"action":"add","user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"iteration":{"id":5,"title":"Sprint 1","iid":1},"created_at":"2026-01-01T00:00:00Z"}`
const regIterationEventsJSON = `[` + regIterationEventJSON + `]`
const regWeightEventJSON = `{"id":2,"user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"weight":5,"previous_weight":3,"created_at":"2026-01-01T00:00:00Z"}`
const regWeightEventsJSON = `[` + regWeightEventJSON + `]`
const regLabelEventJSON = `{"id":1,"action":"add","label":{"id":1,"name":"bug"},"user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"created_at":"2026-01-01T00:00:00Z"}`
const regLabelEventsJSON = `[` + regLabelEventJSON + `]`
const regMilestoneEventJSON = `{"id":1,"action":"add","milestone":{"id":1,"title":"v1.0","iid":1},"user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"created_at":"2026-01-01T00:00:00Z"}`
const regMilestoneEventsJSON = `[` + regMilestoneEventJSON + `]`
const regStateEventJSON = `{"id":1,"state":"closed","user":{"id":1,"username":"user"},"resource_type":"Issue","resource_id":10,"created_at":"2026-01-01T00:00:00Z"}`
const regStateEventsJSON = `[` + regStateEventJSON + `]`

// TestRegisterTools_CallThroughMCP exercises all handler closures in RegisterTools
// via MCP in-memory transport, covering iteration and weight event tools that
// existing tests miss.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		// Iteration events
		case r.Method == http.MethodGet && strings.Contains(path, "/resource_iteration_events/"):
			testutil.RespondJSON(w, http.StatusOK, regIterationEventJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/resource_iteration_events"):
			testutil.RespondJSON(w, http.StatusOK, regIterationEventsJSON)
		// Weight events
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/resource_weight_events"):
			testutil.RespondJSON(w, http.StatusOK, regWeightEventsJSON)
		// Label events
		case r.Method == http.MethodGet && strings.Contains(path, "/resource_label_events/"):
			testutil.RespondJSON(w, http.StatusOK, regLabelEventJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/resource_label_events"):
			testutil.RespondJSON(w, http.StatusOK, regLabelEventsJSON)
		// Milestone events
		case r.Method == http.MethodGet && strings.Contains(path, "/resource_milestone_events/"):
			testutil.RespondJSON(w, http.StatusOK, regMilestoneEventJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/resource_milestone_events"):
			testutil.RespondJSON(w, http.StatusOK, regMilestoneEventsJSON)
		// State events
		case r.Method == http.MethodGet && strings.Contains(path, "/resource_state_events/"):
			testutil.RespondJSON(w, http.StatusOK, regStateEventJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/resource_state_events"):
			testutil.RespondJSON(w, http.StatusOK, regStateEventsJSON)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		// Label events
		{"gitlab_issue_label_event_list", map[string]any{"project_id": "42", "issue_iid": float64(1)}},
		{"gitlab_issue_label_event_get", map[string]any{"project_id": "42", "issue_iid": float64(1), "label_event_id": float64(1)}},
		{"gitlab_mr_label_event_list", map[string]any{"project_id": "42", "mr_iid": float64(1)}},
		{"gitlab_mr_label_event_get", map[string]any{"project_id": "42", "mr_iid": float64(1), "label_event_id": float64(1)}},
		// Milestone events
		{"gitlab_issue_milestone_event_list", map[string]any{"project_id": "42", "issue_iid": float64(1)}},
		{"gitlab_issue_milestone_event_get", map[string]any{"project_id": "42", "issue_iid": float64(1), "milestone_event_id": float64(1)}},
		{"gitlab_mr_milestone_event_list", map[string]any{"project_id": "42", "mr_iid": float64(1)}},
		{"gitlab_mr_milestone_event_get", map[string]any{"project_id": "42", "mr_iid": float64(1), "milestone_event_id": float64(1)}},
		// State events
		{"gitlab_issue_state_event_list", map[string]any{"project_id": "42", "issue_iid": float64(1)}},
		{"gitlab_issue_state_event_get", map[string]any{"project_id": "42", "issue_iid": float64(1), "state_event_id": float64(1)}},
		{"gitlab_mr_state_event_list", map[string]any{"project_id": "42", "mr_iid": float64(1)}},
		{"gitlab_mr_state_event_get", map[string]any{"project_id": "42", "mr_iid": float64(1), "state_event_id": float64(1)}},
		// Iteration events (enterprise)
		{"gitlab_issue_iteration_event_list", map[string]any{"project_id": "42", "issue_iid": float64(1)}},
		{"gitlab_issue_iteration_event_get", map[string]any{"project_id": "42", "issue_iid": float64(1), "iteration_event_id": float64(1)}},
		// Weight events (enterprise)
		{"gitlab_issue_weight_event_list", map[string]any{"project_id": "42", "issue_iid": float64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil", tt.name)
			}
		})
	}
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
