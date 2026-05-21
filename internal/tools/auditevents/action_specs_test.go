// action_specs_test.go contains integration tests for the audit event tool closures
// in ActionSpecs routes with a mock GitLab API.
package auditevents

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerEventJSON     = `{"id":1,"author_id":10,"entity_id":42,"entity_type":"Project","details":{"change":"updated"},"created_at":"2026-01-01T00:00:00Z"}`
	registerEventListJSON = `[{"id":1,"author_id":10,"entity_id":42,"entity_type":"Project","details":{},"created_at":"2026-01-01T00:00:00Z"}]`
)

// TestActionSpecs_Metadata verifies audit event action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if !spec.ReadOnly || !spec.Idempotent || spec.OwnerPackage != "auditevents" {
			t.Fatalf("unexpected ActionSpec semantics: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all 6 audit event canonical routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/audit_events") || strings.HasSuffix(path, "/audit_events/"):
			testutil.RespondJSON(w, http.StatusOK, registerEventListJSON)
		default:
			testutil.RespondJSON(w, http.StatusOK, registerEventJSON)
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
		{"gitlab_list_instance_audit_events", map[string]any{}},
		{"gitlab_get_instance_audit_event", map[string]any{"event_id": 1}},
		{"gitlab_list_group_audit_events", map[string]any{"group_id": "5"}},
		{"gitlab_get_group_audit_event", map[string]any{"group_id": "5", "event_id": 1}},
		{"gitlab_list_project_audit_events", map[string]any{"project_id": "42"}},
		{"gitlab_get_project_audit_event", map[string]any{"project_id": "42", "event_id": 1}},
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
