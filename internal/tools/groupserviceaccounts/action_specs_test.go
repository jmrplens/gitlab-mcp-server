// action_specs_test.go contains unit tests for the group service account [toolutil.ActionSpec] entries.
package groupserviceaccounts

import (
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerAccountJSON  = `{"id":1,"name":"svc","username":"svc-user","email":"svc@test.com"}`
	registerAccountsJSON = `[{"id":1,"name":"svc","username":"svc-user","email":"svc@test.com"}]`
	registerPATJSON      = `{"id":10,"name":"tok","scopes":["api"],"active":true,"revoked":false}`
	registerPATsJSON     = `[{"id":10,"name":"tok","scopes":["api"],"active":true,"revoked":false}]`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupserviceaccounts" {
			t.Errorf("OwnerPackage for %s = %q, want groupserviceaccounts", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupServiceAccountSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_group_service_account_list", "gitlab_group_service_account_pat_list"} {
		t.Run(name, func(t *testing.T) {
			if !byTool[name].ReadOnly {
				t.Errorf("%s should be read-only", name)
			}
		})
	}
	for _, name := range []string{"gitlab_group_service_account_delete", "gitlab_group_service_account_pat_revoke"} {
		t.Run(name, func(t *testing.T) {
			spec := byTool[name]
			if !spec.Destructive || !spec.Route.Destructive {
				t.Errorf("%s should be destructive", name)
			}
			if !spec.Idempotent {
				t.Errorf("%s should be idempotent", name)
			}
		})
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
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusOK, registerAccountsJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerPATsJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerPATJSON)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusCreated, registerAccountJSON)
		case r.Method == http.MethodPatch:
			testutil.RespondJSON(w, http.StatusOK, registerAccountJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupServiceAccountSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_service_account_list", map[string]any{"group_id": "mygroup"}},
		{"gitlab_group_service_account_create", map[string]any{"group_id": "mygroup", "name": "svc", "username": "svc-user"}},
		{"gitlab_group_service_account_update", map[string]any{"group_id": "mygroup", "service_account_id": 42, "name": "svc2"}},
		{"gitlab_group_service_account_delete", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_list", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_create", map[string]any{"group_id": "mygroup", "service_account_id": 42, "name": "tok", "scopes": []any{"api"}}},
		{"gitlab_group_service_account_pat_revoke", map[string]any{"group_id": "mygroup", "service_account_id": 42, "token_id": 10}},
		{"gitlab_group_service_account_pat_rotate", map[string]any{"group_id": "mygroup", "service_account_id": 42, "token_id": 10}},
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

// TestActionSpecs_CallRouteErrors validates the CallRouteErrors route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupServiceAccountSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_service_account_delete", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_revoke", map[string]any{"group_id": "mygroup", "service_account_id": 42, "token_id": 10}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
			if result != nil {
				t.Errorf("Route.Handler(%s) result = %#v, want nil", tt.name, result)
			}
		})
	}
}

// groupServiceAccountMetadataSpecs returns the eight specs with a client that
// answers nothing, for the assertions that read metadata rather than call a
// route. Every spec-building branch runs at construction, so no GitLab response
// is involved in any of them.
func groupServiceAccountMetadataSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	return groupServiceAccountSpecsByTool(t, ActionSpecs(client))
}

// toolsWhere returns the sorted individual-tool names whose spec satisfies
// match. Collecting the whole set and comparing it once is what makes a
// pairing assertable: a branch that names the wrong tool both loses the tool it
// was written for and gains the seven it was not, and only the full set shows
// both halves.
func toolsWhere(byTool map[string]toolutil.ActionSpec, match func(toolutil.ActionSpec) bool) []string {
	var names []string
	for name, spec := range byTool {
		if match(spec) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// TestActionSpecs_UsageGuidance_ReachesExactlyTheToolsItIsWrittenFor holds the
// four per-tool `Usage +=` branches. What they append is advice a model reads
// before it shapes a call — when to leave email out, when to leave expires_at
// out, and what rotating does to the token it is handed — and the catalog is
// well formed either way, so nothing that counts the specs or reads their
// names can tell the advice landed on the wrong seven actions. The property is
// the pairing between a sentence and the tools that must carry it.
func TestActionSpecs_UsageGuidance_ReachesExactlyTheToolsItIsWrittenFor(t *testing.T) {
	byTool := groupServiceAccountMetadataSpecs(t)

	tests := []struct {
		name   string
		marker string
		owners []string
	}{
		{
			name:   "email guidance",
			marker: "Omit email unless the task gives an explicit valid email address.",
			owners: []string{"gitlab_group_service_account_create", "gitlab_group_service_account_update"},
		},
		{
			name:   "expiry guidance",
			marker: "Omit expires_at unless the task gives an explicit expiry date.",
			owners: []string{"gitlab_group_service_account_pat_create", "gitlab_group_service_account_pat_rotate"},
		},
		{
			name:   "rotation warning",
			marker: "Rotating revokes the supplied token_id and returns a brand-new token value.",
			owners: []string{"gitlab_group_service_account_pat_rotate"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolsWhere(byTool, func(spec toolutil.ActionSpec) bool {
				return strings.Contains(spec.Usage, tt.marker)
			})
			if !slices.Equal(got, tt.owners) {
				t.Errorf("tools whose usage carries %q = %v, want %v", tt.marker, got, tt.owners)
			}
		})
	}
}

// TestActionSpecs_RotateAlone_CarriesAnIndividualToolDescription holds the
// branch that gives the rotate tool a description of its own. It is the only
// action here whose result is a secret the caller can never fetch again, and
// the description is where that is said; given to the other seven instead it
// would tell a model that listing tokens returns a one-time value, and leave
// the action it was written for described by its generated title alone.
func TestActionSpecs_RotateAlone_CarriesAnIndividualToolDescription(t *testing.T) {
	byTool := groupServiceAccountMetadataSpecs(t)

	got := toolsWhere(byTool, func(spec toolutil.ActionSpec) bool {
		return spec.IndividualTool.Description != ""
	})
	want := []string{"gitlab_group_service_account_pat_rotate"}
	if !slices.Equal(got, want) {
		t.Errorf("tools carrying an individual-tool description = %v, want %v", got, want)
	}
	if desc := byTool["gitlab_group_service_account_pat_rotate"].IndividualTool.Description; !strings.Contains(desc, "revokes the supplied token_id") {
		t.Errorf("rotate description = %q, want it to say the supplied token is revoked", desc)
	}
}

// TestActionSpecs_TokenStateEnum_IsPublishedOnThePATListingAlone holds the
// input-schema override branch. `state` is not one of the parameters the
// central table gives an enum to, so this override is the only thing telling a
// model that active and inactive are the two values GitLab accepts; landed on
// the other seven actions it would constrain a parameter they do not have and
// leave the listing taking any string.
func TestActionSpecs_TokenStateEnum_IsPublishedOnThePATListingAlone(t *testing.T) {
	byTool := groupServiceAccountMetadataSpecs(t)

	got := toolsWhere(byTool, func(spec toolutil.ActionSpec) bool {
		return len(spec.InputSchemaOverrides) > 0
	})
	want := []string{"gitlab_group_service_account_pat_list"}
	if !slices.Equal(got, want) {
		t.Errorf("tools carrying an input-schema override = %v, want %v", got, want)
	}

	schema := byTool["gitlab_group_service_account_pat_list"].Route.InputSchema
	props, _ := schema["properties"].(map[string]any)
	state, _ := props["state"].(map[string]any)
	enum, _ := state["enum"].([]any)
	if !slices.Equal(enum, []any{"active", "inactive"}) {
		t.Errorf("pat_list state enum = %v, want [active inactive]", enum)
	}
}

// TestActionSpecs_TokenIDGuidance_GoesToTheTwoActionsThatTakeATokenID holds
// both operands of the guidance branch. The guidance exists because the two
// actions take a service_account_id and a token_id side by side and a model
// that confuses them revokes or rotates the wrong credential; given to the six
// actions that take no token_id it describes a parameter that is not there,
// and the two that need it are left with the confusion unaddressed.
func TestActionSpecs_TokenIDGuidance_GoesToTheTwoActionsThatTakeATokenID(t *testing.T) {
	byTool := groupServiceAccountMetadataSpecs(t)

	got := toolsWhere(byTool, func(spec toolutil.ActionSpec) bool {
		_, ok := spec.ParameterGuidance["token_id"]
		return ok
	})
	want := []string{"gitlab_group_service_account_pat_revoke", "gitlab_group_service_account_pat_rotate"}
	if !slices.Equal(got, want) {
		t.Errorf("tools carrying token_id guidance = %v, want %v", got, want)
	}
	for _, name := range want {
		t.Run(name, func(t *testing.T) {
			guidance := byTool[name].ParameterGuidance["token_id"]
			if guidance.SemanticRole != "access_token" {
				t.Errorf("token_id SemanticRole = %q, want access_token", guidance.SemanticRole)
			}
			if len(guidance.CommonConfusions) == 0 {
				t.Error("token_id guidance names no confusion, want the service_account_id one")
			}
		})
	}
}

// TestGroupServiceAccountAliases_EveryActionIsInTheTable_AndAnUnknownNameGetsNone
// holds the alias switch to the action names the specs actually use. Aliases
// are how a model reaches an action it cannot name exactly, and the switch is
// keyed by a string literal repeated from the spec list: an action renamed on
// one side and not the other still registers, still routes, and is simply
// undiscoverable, which is what the second half of this asserts is the only
// thing an unrecognized name can produce.
func TestGroupServiceAccountAliases_EveryActionIsInTheTable_AndAnUnknownNameGetsNone(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if len(spec.Aliases) == 0 {
				t.Errorf("action %q has no aliases; the alias table does not name it", spec.Name)
			}
		})
	}
	if got := groupServiceAccountAliases("service_account_not_an_action"); got != nil {
		t.Errorf("groupServiceAccountAliases(unknown name) = %v, want none", got)
	}
}

func groupServiceAccountSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
