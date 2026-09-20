// action_specs_test.go contains unit tests for the group epic board [toolutil.ActionSpec] entries.
package groupepicboards

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerBoardJSON  = `{"id":1,"name":"Board","labels":[{"name":"bug"},null],"lists":[{"id":1,"position":0,"label":{"id":10,"name":"To Do"}},{"id":2,"position":1,"label":null},null]}`
	registerBoardsJSON = `[{"id":1,"name":"Board","labels":[{"name":"bug"}],"lists":[{"id":1,"position":0,"label":{"id":10,"name":"To Do"}}]}]`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_DiscoveryMetadata holds every published spec to the metadata
// table rather than to the generic fallback beside it, and holds the related
// action IDs the specs publish to the same constants the Markdown hints are
// built from. The two sets are written apart, in action_specs.go and in
// markdown.go, and have drifted in other domains; a related action naming an
// ID no surface resolves reads to a model as a capability that is not there.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			meta, ok := epicBoardMetaByName[spec.Name]
			if !ok {
				t.Fatalf("no discovery metadata for published action %q", spec.Name)
			}
			if spec.Usage != meta.usage {
				t.Errorf("Usage = %q, want the table's %q", spec.Usage, meta.usage)
			}
			if spec.IndividualTool.Description != meta.description {
				t.Errorf("Description = %q, want the table's %q", spec.IndividualTool.Description, meta.description)
			}
			if len(spec.Aliases) == 0 || spec.Aliases[0] != spec.IndividualTool.Name {
				t.Errorf("Aliases = %v, want the individual tool name first", spec.Aliases)
			}
			assertRelatedActionsUseTheHintConstants(t, spec.RelatedActions)
		})
	}
}

// assertRelatedActionsUseTheHintConstants holds each related ID to one of the
// three the package spells: the two board IDs the Markdown hints are built
// from and the epic list the specs share. Whether an ID resolves against the
// catalog is what make check-action-ids answers; what this answers is that the
// specs and the hints keep naming one spelling.
func assertRelatedActionsUseTheHintConstants(t *testing.T, related []string) {
	t.Helper()
	known := map[string]bool{
		actionEpicBoardList: true,
		actionEpicBoardGet:  true,
		actionGroupEpicList: true,
	}
	if len(related) == 0 {
		t.Fatal("RelatedActions is empty")
	}
	for _, id := range related {
		if !known[id] {
			t.Errorf("RelatedActions entry %q names no canonical ID this package spells", id)
		}
	}
}

// TestActionSpecs_UnknownNameKeepsTheGenericMetadata verifies the fallback the
// metadata lookup carries: an action name the table does not hold keeps the
// generic usage and the domain's own related action rather than inheriting
// another action's text. Nothing calls it with such a name today, which is
// what this pins.
func TestActionSpecs_UnknownNameKeepsTheGenericMetadata(t *testing.T) {
	opts := groupEpicBoardOptions("no_such_action", "gitlab_group_epic_board_list")
	if opts.Usage != "Use to execute groupepicboards domain action." {
		t.Errorf("Usage = %q, want the generic default", opts.Usage)
	}
	if len(opts.Aliases) != 1 || opts.Aliases[0] != "gitlab_group_epic_board_list" {
		t.Errorf("Aliases = %v, want the individual tool name alone", opts.Aliases)
	}
	if len(opts.RelatedActions) != 1 || opts.RelatedActions[0] != actionGroupEpicList {
		t.Errorf("RelatedActions = %v, want [%s]", opts.RelatedActions, actionGroupEpicList)
	}
	if opts.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want empty", opts.IndividualTool.Description)
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
