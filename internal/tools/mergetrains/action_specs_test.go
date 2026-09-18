package mergetrains

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerTrainJSON  = `{"id":1,"merge_request":{"iid":10,"title":"MR","web_url":"https://gl.example.com/mr/10"},"pipeline":{"id":100},"target_branch":"main","status":"idle"}`
	registerTrainsJSON = `[{"id":1,"merge_request":{"iid":10,"title":"MR","web_url":"https://gl.example.com/mr/10"},"pipeline":{"id":100},"target_branch":"main","status":"idle"}]`

	// genericMergeTrainUsage is the placeholder [mergeTrainOptions] gives every
	// action before its own metadata replaces it.
	genericMergeTrainUsage = "Use to execute mergetrains domain action."
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

// TestActionSpecs_DiscoveryMetadata_ReplacesTheGenericOptions verifies every
// merge train action reaches the catalog carrying its own discovery metadata
// rather than the placeholder [mergeTrainOptions] starts it with: a usage that
// says what the action is for, an alias a person would type, related actions
// named by canonical IDs this domain really registers and never by the action
// itself, and the "Returns: … See also: …" individual-tool description
// (1:1 audit R-META).
//
// Each of those four is applied by a guard in [decorateMergeTrainMeta] whose
// other side no entry in the table reaches, so a guard that stopped copying
// would leave a model searching the catalog with nothing but the generic
// sentence and the bare tool name to match on, and every other test here would
// still pass.
func TestActionSpecs_DiscoveryMetadata_ReplacesTheGenericOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	registered := make(map[string]bool, len(specs))
	for _, spec := range specs {
		registered["merge_train."+spec.Name] = true
	}
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			if spec.Usage == "" || spec.Usage == genericMergeTrainUsage {
				t.Errorf("Usage = %q, want a usage naming what this action does", spec.Usage)
			}
			if !hasNaturalAlias(spec) {
				t.Errorf("Aliases = %v, want one a person would type beyond the tool name", spec.Aliases)
			}
			assertRelatedActions(t, spec, registered)
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("IndividualTool.Description = %q, want the Returns: … See also: … form", desc)
			}
		})
	}
}

// hasNaturalAlias reports whether the spec carries an alias beyond the two
// names a caller already has: the individual tool's and the action's own.
func hasNaturalAlias(spec toolutil.ActionSpec) bool {
	for _, alias := range spec.Aliases {
		if alias != spec.IndividualTool.Name && alias != spec.Name {
			return true
		}
	}
	return false
}

// assertRelatedActions holds a spec's related actions to canonical IDs this
// domain registers, and never to the action itself: a related action a model
// cannot execute is worse than none, and one pointing back at the call it was
// given sends it round in a circle.
func assertRelatedActions(t *testing.T, spec toolutil.ActionSpec, registered map[string]bool) {
	t.Helper()
	if len(spec.RelatedActions) == 0 {
		t.Error("RelatedActions is empty, want the sibling merge train actions")
	}
	for _, related := range spec.RelatedActions {
		if !registered[related] {
			t.Errorf("RelatedActions names %q, which this domain registers no action under", related)
		}
		if related == "merge_train."+spec.Name {
			t.Errorf("RelatedActions names the action itself (%q)", related)
		}
	}
}

// TestDecorateMergeTrainMeta_EmptyEntry_LeavesEveryOptionAlone verifies each of
// the four metadata fields is copied only when the table entry supplies it.
// All four real entries fill all four fields, so nothing else reaches the other
// side of those guards, and a merge train action added later with partial
// metadata would otherwise have the usage, alias and related actions it already
// carried blanked rather than left in place.
func TestDecorateMergeTrainMeta_EmptyEntry_LeavesEveryOptionAlone(t *testing.T) {
	const probe = "gitlab_merge_train_meta_probe"
	mergeTrainActionMeta[probe] = mergeTrainActionMetaEntry{}
	t.Cleanup(func() { delete(mergeTrainActionMeta, probe) })

	options := mergeTrainOptions(probe)
	options.RelatedActions = []string{"merge_train.generic"}
	options.IndividualTool.Description = "generic description"
	decorateMergeTrainMeta(&options, probe)

	if options.Usage != genericMergeTrainUsage {
		t.Errorf("Usage = %q, want the generic one untouched", options.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != probe {
		t.Errorf("Aliases = %v, want [%s] left as it was", options.Aliases, probe)
	}
	if len(options.RelatedActions) != 1 || options.RelatedActions[0] != "merge_train.generic" {
		t.Errorf("RelatedActions = %v, want the one it already carried", options.RelatedActions)
	}
	if options.IndividualTool.Description != "generic description" {
		t.Errorf("IndividualTool.Description = %q, want it untouched", options.IndividualTool.Description)
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
