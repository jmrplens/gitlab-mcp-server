package mergetrains

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerTrainJSON  = `{"id":1,"merge_request":{"iid":10,"title":"MR","web_url":"https://gl.example.com/mr/10"},"pipeline":{"id":100},"target_branch":"main","status":"idle"}`
	registerTrainsJSON = `[{"id":1,"merge_request":{"iid":10,"title":"MR","web_url":"https://gl.example.com/mr/10"},"pipeline":{"id":100},"target_branch":"main","status":"idle"}]`
)

// TestActionSpecs_Metadata verifies merge train action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "mergetrains" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all 4 merge train routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/merge_trains/merge_requests/"):
			testutil.RespondJSON(w, http.StatusOK, registerTrainJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/merge_trains/"):
			testutil.RespondJSON(w, http.StatusOK, registerTrainsJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/merge_trains"):
			testutil.RespondJSON(w, http.StatusOK, registerTrainsJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/merge_trains/merge_requests/"):
			testutil.RespondJSON(w, http.StatusCreated, registerTrainsJSON)
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
		{"gitlab_list_project_merge_trains", map[string]any{"project_id": "42"}},
		{"gitlab_list_merge_request_in_merge_train", map[string]any{"project_id": "42", "target_branch": "main"}},
		{"gitlab_get_merge_request_on_merge_train", map[string]any{"project_id": "42", "merge_request_iid": float64(10)}},
		{"gitlab_add_merge_request_to_merge_train", map[string]any{"project_id": "42", "merge_request_iid": float64(10)}},
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
