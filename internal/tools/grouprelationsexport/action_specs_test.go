package grouprelationsexport

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerExportStatusJSON = `[{"relation":"labels","status":0,"batched":false,"batches_count":0,"error":""}]`

// TestActionSpecs_Metadata verifies canonical metadata for group relations export actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "grouprelationsexport" {
			t.Errorf("OwnerPackage for %s = %q, want grouprelationsexport", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}
	if !groupRelationsSpecsByTool(t, specs)["gitlab_list_group_relations_export_status"].ReadOnly {
		t.Error("list status action should be read-only")
	}
}

// TestActionSpecs_CallRoutes verifies both group relations export routes execute through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, "/export_relations"):
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && strings.Contains(path, "/export_relations/status"):
			testutil.RespondJSON(w, http.StatusOK, registerExportStatusJSON)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupRelationsSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_schedule_group_relations_export", map[string]any{"group_id": "5"}},
		{"gitlab_list_group_relations_export_status", map[string]any{"group_id": "5"}},
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

func groupRelationsSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestFormatListExportStatusMarkdownString verifies the markdown formatter covers
// the FormatListExportStatusMarkdownString function registered via init().
func TestFormatListExportStatusMarkdownString(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		out := FormatListExportStatusMarkdownString(ListExportStatusOutput{})
		if out == "" {
			t.Fatal("expected non-empty markdown for empty list")
		}
	})
	t.Run("with statuses", func(t *testing.T) {
		out := FormatListExportStatusMarkdownString(ListExportStatusOutput{
			Statuses: []ExportStatusItem{{Relation: "labels", Status: 0, Batched: false, BatchesCount: 0}},
		})
		if out == "" {
			t.Fatal("expected non-empty markdown")
		}
	})
}

// TestMarkdownInit_Registry verifies the init() markdown formatter is registered.
func TestMarkdownInit_Registry(t *testing.T) {
	out := toolutil.MarkdownForResult(ListExportStatusOutput{})
	if out == nil {
		t.Fatal("expected non-nil result for ListExportStatusOutput")
	}
}
