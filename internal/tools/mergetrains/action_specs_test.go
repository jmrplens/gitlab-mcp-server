package mergetrains

import (
	"maps"
	"net/http"
	"reflect"
	"slices"
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

// TestActionSpecs_CallRoutes verifies each merge train tool reaches the GitLab
// endpoint its own action names, and not merely that some route answered.
//
// The pairing of a tool name with a route is a function value beside a string,
// and nothing about it is a branch either gate can flip: giving
// gitlab_get_merge_request_on_merge_train the add route leaves a read tool
// POSTing a merge request onto the train, and the version of this test that
// asked only for a non-nil result and no error stayed green while it did. The
// mock answers by the shape of the path it is given, so a crossed route gets a
// well-formed response and is caught by the recorded method and path rather
// than by a decoding accident.
func TestActionSpecs_CallRoutes(t *testing.T) {
	tools := []struct {
		name       string
		args       map[string]any
		wantMethod string
		wantPath   string
	}{
		{
			"gitlab_list_project_merge_trains",
			map[string]any{"project_id": "42"},
			http.MethodGet, "/api/v4/projects/42/merge_trains",
		},
		{
			"gitlab_list_merge_request_in_merge_train",
			map[string]any{"project_id": "42", "target_branch": "main"},
			http.MethodGet, "/api/v4/projects/42/merge_trains/main",
		},
		{
			"gitlab_get_merge_request_on_merge_train",
			map[string]any{"project_id": "42", "merge_request_iid": float64(10)},
			http.MethodGet, "/api/v4/projects/42/merge_trains/merge_requests/10",
		},
		{
			"gitlab_add_merge_request_to_merge_train",
			map[string]any{"project_id": "42", "merge_request_iid": float64(10)},
			http.MethodPost, "/api/v4/projects/42/merge_trains/merge_requests/10",
		},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				// Only the single-merge-request read answers with one object;
				// every other merge train route answers with a list.
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/merge_trains/merge_requests/") {
					testutil.RespondJSON(w, http.StatusOK, registerTrainJSON)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, registerTrainsJSON)
			}))
			spec, ok := specsByTool(ActionSpecs(client))[tt.name]
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
			if gotMethod != tt.wantMethod || gotPath != tt.wantPath {
				t.Errorf("%s reached %s %s, want %s %s", tt.name, gotMethod, gotPath, tt.wantMethod, tt.wantPath)
			}
		})
	}
}

// specsByTool indexes specs by the name their individual tool is registered
// under.
func specsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// mergeTrainSurfaceFacts is what the catalog reads off one spec to decide where
// the action is served and whether a session may run it.
type mergeTrainSurfaceFacts struct {
	action     string
	readOnly   bool
	idempotent bool
}

// TestActionSpecs_SurfaceMetadata pins the facts the catalog reads off each
// spec that no other test in this package reads back: the canonical action each
// tool name belongs to, whether the action mutates, and the tier, tags, owner
// and open-world hint the whole domain is registered under.
//
// None of it is a branch, so neither gate can be wrong about any of it, and
// each is one word away from a surface defect that compiles: building the add
// action with [mergeTrainReadSpec] serves a tool that enqueues a merge request
// to a --read-only deployment and to a read_api token, and an empty Edition
// serves this Premium domain to a Free instance.
func TestActionSpecs_SurfaceMetadata(t *testing.T) {
	want := map[string]mergeTrainSurfaceFacts{
		"gitlab_list_project_merge_trains":         {action: "list_project", readOnly: true, idempotent: true},
		"gitlab_list_merge_request_in_merge_train": {action: "list_branch", readOnly: true, idempotent: true},
		"gitlab_get_merge_request_on_merge_train":  {action: "get", readOnly: true, idempotent: true},
		"gitlab_add_merge_request_to_merge_train":  {action: "add", readOnly: false, idempotent: false},
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	gotTools := make([]string, 0, len(specs))
	for _, spec := range specs {
		gotTools = append(gotTools, spec.IndividualTool.Name)
	}
	slices.Sort(gotTools)
	if wantTools := slices.Sorted(maps.Keys(want)); !slices.Equal(gotTools, wantTools) {
		t.Fatalf("individual tools = %v, want %v", gotTools, wantTools)
	}
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			assertSurfaceFacts(t, spec, want[spec.IndividualTool.Name])
		})
	}
}

// assertSurfaceFacts holds one spec to the action it belongs to, its mutation
// classification and the domain-wide registration metadata.
func assertSurfaceFacts(t *testing.T, spec toolutil.ActionSpec, facts mergeTrainSurfaceFacts) {
	t.Helper()
	if spec.Name != facts.action {
		t.Errorf("Name = %q, want %q", spec.Name, facts.action)
	}
	if spec.ReadOnly != facts.readOnly {
		t.Errorf("ReadOnly = %v, want %v", spec.ReadOnly, facts.readOnly)
	}
	if spec.Idempotent != facts.idempotent {
		t.Errorf("Idempotent = %v, want %v", spec.Idempotent, facts.idempotent)
	}
	if spec.Destructive {
		t.Error("Destructive = true; no merge train action deletes anything")
	}
	if spec.Edition != "premium" {
		t.Errorf("Edition = %q, want %q: merge trains are a Premium feature", spec.Edition, "premium")
	}
	if spec.OwnerPackage != "mergetrains" {
		t.Errorf("OwnerPackage = %q, want %q", spec.OwnerPackage, "mergetrains")
	}
	if !spec.OpenWorld {
		t.Error("OpenWorld = false; every merge train action reaches GitLab")
	}
	if wantTags := []string{"merge_request", "merge_train"}; !slices.Equal(spec.Tags, wantTags) {
		t.Errorf("Tags = %v, want %v", spec.Tags, wantTags)
	}
}

// TestActionSpecs_ScopeEnum verifies the scope parameter is published as a
// closed vocabulary on the two actions that accept one, and that the other two
// publish no input-schema override at all.
//
// The enum is the only thing telling a model that scope takes "active" or
// "complete" rather than free text, it is applied by a switch over tool names
// that no mutation reaches, and it is attached to the wrong action as easily as
// to the right one.
func TestActionSpecs_ScopeEnum(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	wantScoped := map[string]bool{
		"gitlab_list_project_merge_trains":         true,
		"gitlab_list_merge_request_in_merge_train": true,
	}
	for _, spec := range ActionSpecs(client) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			if !wantScoped[spec.IndividualTool.Name] {
				if len(spec.InputSchemaOverrides) != 0 {
					t.Errorf("InputSchemaOverrides = %v, want none for an action with no scope parameter", spec.InputSchemaOverrides)
				}
				return
			}
			want := []toolutil.InputSchemaOverride{
				toolutil.SchemaPropertyOverride("scope", map[string]any{"enum": []any{"active", "complete"}}),
			}
			if !reflect.DeepEqual(spec.InputSchemaOverrides, want) {
				t.Errorf("InputSchemaOverrides = %+v, want %+v", spec.InputSchemaOverrides, want)
			}
		})
	}
}

// TestActionIDConstants_AreTheIDsTheSpecsRegister holds both blocks of
// canonical action IDs this package keeps (the one markdown.go builds its
// hints from and the one action_specs.go names related actions with) to the
// IDs [ActionSpecs] really registers, and to each other.
//
// A dotted ID is a string literal, so no mutation of a branch and no condition
// counter can be wrong about one, and a hint naming an action no surface
// resolves answers a model "unknown action" the moment it follows the advice.
// Two blocks are only safe while they agree, and elsewhere in this tree they
// have drifted.
func TestActionIDConstants_AreTheIDsTheSpecsRegister(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	registered := make(map[string]bool)
	for _, spec := range ActionSpecs(client) {
		registered["merge_train."+spec.Name] = true
	}
	pairs := []struct {
		hintID string
		specID string
	}{
		{actionListProject, actionMergeTrainListProject},
		{actionListBranch, actionMergeTrainListBranch},
		{actionGet, actionMergeTrainGet},
		{actionAdd, actionMergeTrainAdd},
	}
	for _, pair := range pairs {
		t.Run(pair.hintID, func(t *testing.T) {
			if pair.hintID != pair.specID {
				t.Errorf("markdown.go names %q where action_specs.go names %q", pair.hintID, pair.specID)
			}
			if !registered[pair.hintID] {
				t.Errorf("%q names no action ActionSpecs registers", pair.hintID)
			}
		})
	}
	if len(pairs) != len(registered) {
		t.Errorf("%d ID constant(s) for %d registered action(s); every action needs one", len(pairs), len(registered))
	}
}
