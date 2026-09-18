// action_specs_test.go contains integration tests for the audit event tool closures
// in ActionSpecs routes with a mock GitLab API.
package auditevents

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerEventJSON     = `{"id":1,"author_id":10,"entity_id":42,"entity_type":"Project","details":{"change":"updated"},"created_at":"2026-01-01T00:00:00Z"}`
	registerEventListJSON = `[{"id":1,"author_id":10,"entity_id":42,"entity_type":"Project","details":{},"created_at":"2026-01-01T00:00:00Z"}]`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_DiscoveryMetadata verifies that every spec ActionSpecs
// returns carries metadata of its own: usage that replaced the shared
// placeholder, an individual-tool description, aliases beyond the tool name,
// and related actions naming other audit_event actions.
//
// Why it matters: the per-action metadata is filled by a switch over the
// individual tool name, with no default arm. A seventh action added without a
// case compiles, registers, and ships with "Use to execute auditevents domain
// action." and an empty description — which is what a model reads to decide
// the tool exists at all. Nothing else in the package notices that, since the
// route still answers.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	seenUsage := make(map[string]string, len(specs))
	seenDescription := make(map[string]string, len(specs))
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			assertSpecDecorated(t, spec)
			if other, dup := seenUsage[spec.Usage]; dup {
				t.Errorf("Usage is identical to %s: one case of the switch fills two actions", other)
			}
			seenUsage[spec.Usage] = spec.IndividualTool.Name
			if other, dup := seenDescription[spec.IndividualTool.Description]; dup {
				t.Errorf("IndividualTool.Description is identical to %s", other)
			}
			seenDescription[spec.IndividualTool.Description] = spec.IndividualTool.Name
		})
	}
}

// assertSpecDecorated holds one spec to the metadata its decoration case is
// there to add, and says for each field what a model loses without it.
func assertSpecDecorated(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	if spec.Usage == sharedAuditUsage {
		t.Errorf("Usage is the shared placeholder %q: this action was never decorated", sharedAuditUsage)
	}
	if spec.IndividualTool.Description == "" {
		t.Error("IndividualTool.Description is empty: the individual surface would list the tool with no description")
	}
	if len(spec.RelatedActions) == 0 {
		t.Error("RelatedActions is empty: discovery cannot walk from this action to its siblings")
	}
	for _, related := range spec.RelatedActions {
		if !strings.HasPrefix(related, "audit_event.") {
			t.Errorf("RelatedActions holds %q, want a canonical audit_event.<action> ID", related)
		}
		if related == "audit_event."+spec.Name {
			t.Errorf("RelatedActions names the action itself (%q)", related)
		}
	}
	if len(spec.Aliases) < 2 {
		t.Errorf("Aliases = %v, want the natural-language phrases the decoration adds", spec.Aliases)
	}
}

// TestDecorateAuditEventMeta_UnknownTool_KeepsTheSharedDefaults pins what the
// undecorated case looks like: a name the switch does not list keeps the
// shared read-only options untouched rather than picking up whichever action's
// metadata was written last.
//
// It is the other half of TestActionSpecs_DiscoveryMetadata: that test detects
// a forgotten case, and this one says what a forgotten case ships, so the
// detection is read as a real difference rather than as a missing default arm
// nobody ever exercised.
func TestDecorateAuditEventMeta_UnknownTool_KeepsTheSharedDefaults(t *testing.T) {
	options := toolutil.ActionSpecOptions{
		Usage:          sharedAuditUsage,
		Aliases:        []string{"gitlab_list_instance_audit_runs"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_instance_audit_runs"},
	}

	decorateAuditEventMeta(&options, "gitlab_list_instance_audit_runs")

	if options.Usage != sharedAuditUsage {
		t.Errorf("Usage = %q, want the shared default for a tool the switch does not list", options.Usage)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("IndividualTool.Description = %q, want empty", options.IndividualTool.Description)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %v, want none", options.RelatedActions)
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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
