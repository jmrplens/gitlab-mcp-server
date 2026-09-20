// action_specs_test.go contains integration tests for the user GPG key tool
// closures in ActionSpecs routes with a mock GitLab API.
package usergpgkeys

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// registerGPGKeyJSON is the body the route smoke test answers with. It carries
// the three keys GitLab's GpgKey entity sends and nothing else: the fixture
// used to name primary_key_id, key_id, public_key and a user object, none of
// which this endpoint sends and none of which client-go's GPGKey decodes, so
// every route ran against a body whose armored key arrived empty.
const registerGPGKeyJSON = `{"id":1,"key":"-----BEGIN PGP PUBLIC KEY BLOCK-----","created_at":"2026-01-01T00:00:00Z"}`

// genericGPGUsage is the placeholder every spec starts from, kept here so a
// test asserting that a spec carries usage of its own names the one string it
// must not be.
const genericGPGUsage = "Use to execute usergpgkeys domain action."

// TestActionSpecs_Metadata verifies that every GPG key spec carries the
// discovery metadata a model reads before it calls: usage of its own rather
// than the placeholder, natural-language aliases that are not the tool name,
// related actions on the user.* surface the catalog registers these under, and
// a "Returns: … See also: …" description. The four guards that copy these from
// the metadata table were unobservable until this asserted their result, so a
// spec that silently fell back to the placeholder passed.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	specs := ActionSpecs(client)
	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			assertGPGSpecMetadata(t, spec)
		})
	}
}

// assertGPGSpecMetadata holds one spec to the discovery metadata a GPG key
// action must carry.
func assertGPGSpecMetadata(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	tool := spec.IndividualTool.Name
	if spec.OwnerPackage != "usergpgkeys" || tool == "" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
	}
	if spec.Usage == genericGPGUsage || spec.Usage == "" {
		t.Errorf("Usage = %q, want usage written for this action", spec.Usage)
	}
	if len(spec.Aliases) < 2 {
		t.Errorf("Aliases = %v, want several natural-language phrases", spec.Aliases)
	}
	for _, alias := range spec.Aliases {
		if alias == tool {
			t.Errorf("alias %q repeats the tool name", alias)
		}
	}
	if len(spec.RelatedActions) == 0 {
		t.Error("RelatedActions is empty, so the card sends a model nowhere")
	}
	for _, related := range spec.RelatedActions {
		if !strings.HasPrefix(related, "user.") {
			t.Errorf("RelatedActions names %q, want the user.* surface these specs register under", related)
		}
	}
	desc := spec.IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Errorf("Description = %q, want the 'Returns: … See also: …' form", desc)
	}
}

// TestUserGPGOptions_ToolWithNoMetadataEntry_KeepsTheGenericOptions pins the
// fallback: a tool name the metadata table does not hold keeps the placeholder
// usage and the tool name as its only alias, rather than being served empty
// text. Every one of the eight names is in the table, so nothing else reaches
// this side of the lookup.
func TestUserGPGOptions_ToolWithNoMetadataEntry_KeepsTheGenericOptions(t *testing.T) {
	options := userGPGOptions("gitlab_not_a_gpg_key_tool")

	if options.Usage != genericGPGUsage {
		t.Errorf("Usage = %q, want the generic one", options.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_not_a_gpg_key_tool" {
		t.Errorf("Aliases = %v, want the tool name alone", options.Aliases)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %v, want none", options.RelatedActions)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want none", options.IndividualTool.Description)
	}
}

// TestUserGPGOptions_EntryFillsNothing_LeavesEveryGenericOptionAlone verifies
// that each of the four metadata fields is copied only when the entry supplies
// it. All eight real entries fill all four, so nothing else reaches the other
// side of those guards, and a GPG key action added with partial metadata would
// silently lose what the generic options already carried: an entry naming no
// aliases would drop the individual tool name and leave the action findable by
// its canonical ID alone.
func TestUserGPGOptions_EntryFillsNothing_LeavesEveryGenericOptionAlone(t *testing.T) {
	const probe = "gitlab_gpg_key_meta_probe"
	userGPGActionMeta[probe] = userGPGMeta{}
	t.Cleanup(func() { delete(userGPGActionMeta, probe) })

	options := userGPGOptions(probe)

	if options.Usage != genericGPGUsage {
		t.Errorf("Usage = %q, want the generic one untouched", options.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != probe {
		t.Errorf("Aliases = %v, want the generic one left as it was", options.Aliases)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %v, want none added", options.RelatedActions)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want none added", options.IndividualTool.Description)
	}
}

// TestActionSpecs_CallRoutes verifies all registered GPG key routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "/gpg_keys") {
				testutil.RespondJSON(w, http.StatusOK, `[`+registerGPGKeyJSON+`]`)
			} else {
				testutil.RespondJSON(w, http.StatusOK, registerGPGKeyJSON)
			}
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerGPGKeyJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
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
		{"gitlab_list_gpg_keys", map[string]any{}},
		{"gitlab_list_gpg_keys_for_user", map[string]any{"user_id": 1}},
		{"gitlab_get_gpg_key", map[string]any{"key_id": 1}},
		{"gitlab_get_gpg_key_for_user", map[string]any{"user_id": 1, "key_id": 1}},
		{"gitlab_add_gpg_key", map[string]any{"key": "-----BEGIN PGP PUBLIC KEY BLOCK-----"}},
		{"gitlab_add_gpg_key_for_user", map[string]any{"user_id": 1, "key": "-----BEGIN PGP PUBLIC KEY BLOCK-----"}},
		{"gitlab_delete_gpg_key", map[string]any{"key_id": 1}},
		{"gitlab_delete_gpg_key_for_user", map[string]any{"user_id": 1, "key_id": 1}},
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
