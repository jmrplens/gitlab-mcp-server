package groupepicboards

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerBoardJSON  = `{"id":1,"name":"Board","labels":[{"name":"bug"},null],"lists":[{"id":1,"position":0,"label":{"id":10,"name":"To Do"}},{"id":2,"position":1,"label":null},null]}`
	registerBoardsJSON = `[{"id":1,"name":"Board","labels":[{"name":"bug"}],"lists":[{"id":1,"position":0,"label":{"id":10,"name":"To Do"}}]}]`
)

// TestActionSpecs_Metadata verifies group epic board action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupepicboards" || !spec.ReadOnly || !spec.Idempotent {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies both group epic board routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/epic_boards"):
			testutil.RespondJSON(w, http.StatusOK, registerBoardsJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/epic_boards/"):
			// Return board with nil entries to cover nil-check branches in toOutput
			testutil.RespondJSON(w, http.StatusOK, registerBoardJSON)
		default:
			http.NotFound(w, r)
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
		{"gitlab_group_epic_board_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_epic_board_get", map[string]any{"group_id": "42", "board_id": 1}},
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
