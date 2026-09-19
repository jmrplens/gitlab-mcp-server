// action_specs_test.go contains tests for the projectServiceAccountDescription function.
package projectserviceaccounts

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestProjectServiceAccountDescription_DefaultBranch verifies that
// projectServiceAccountDescription returns the generic fallback description
// when called with an unknown or empty action name.
func TestProjectServiceAccountDescription_DefaultBranch(t *testing.T) {
	for _, actionName := range []string{"", "unknown_action", "service_account_foo"} {
		t.Run(actionName, func(t *testing.T) {
			desc := projectServiceAccountDescription(actionName)
			if !strings.Contains(desc, "Manage GitLab project service accounts") {
				t.Errorf("expected fallback description for action %q, got %q", actionName, desc)
			}
		})
	}
}

// TestProjectServiceAccountAliases_UnknownAction_HasNone verifies the alias
// switch answers a name it does not know with no aliases at all. Every case of
// that switch is otherwise reached only by the one name it matches, so its last
// case is never once evaluated against anything else and a case that stopped
// being reachable would look exactly the same.
func TestProjectServiceAccountAliases_UnknownAction_HasNone(t *testing.T) {
	for _, actionName := range []string{"", "unknown_action", "service_account_foo"} {
		t.Run(actionName, func(t *testing.T) {
			if aliases := projectServiceAccountAliases(actionName); len(aliases) != 0 {
				t.Errorf("projectServiceAccountAliases(%q) = %v, want none", actionName, aliases)
			}
		})
	}
}

// TestActionSpecs_ConditionalMetadataReachesOnlyTheActionsThatNeedIt holds each
// of the four per-action additions to exactly the actions it is written for:
// the token_id guidance on the two that take one, the email caution on the two
// that accept an email, the state enum on the one list that filters by it, and
// the expiry caution on the two that set one. Each is a name compared with a
// literal, so a misspelling attaches the advice to the wrong action in silence,
// and advice about a parameter an action does not have costs a model the call
// it builds from it.
func TestActionSpecs_ConditionalMetadataReachesOnlyTheActionsThatNeedIt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	byName := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		byName[spec.Name] = spec
	}

	for _, tt := range []struct {
		name          string
		tokenGuidance bool
		emailCaution  bool
		expiryCaution bool
		stateEnum     string
	}{
		{name: "service_account_list"},
		{name: "service_account_create", emailCaution: true},
		{name: "service_account_update", emailCaution: true},
		{name: "service_account_delete"},
		{name: "service_account_pat_list", stateEnum: "active,inactive"},
		{name: "service_account_pat_create", expiryCaution: true},
		{name: "service_account_pat_rotate", tokenGuidance: true, expiryCaution: true},
		{name: "service_account_pat_revoke", tokenGuidance: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := byName[tt.name]
			if !ok {
				t.Fatalf("ActionSpecs has no %s", tt.name)
			}
			if _, has := spec.ParameterGuidance["token_id"]; has != tt.tokenGuidance {
				t.Errorf("token_id guidance = %v, want %v", has, tt.tokenGuidance)
			}
			if has := strings.Contains(spec.Usage, "Omit email"); has != tt.emailCaution {
				t.Errorf("email caution = %v, want %v", has, tt.emailCaution)
			}
			if has := strings.Contains(spec.Usage, "Omit expires_at"); has != tt.expiryCaution {
				t.Errorf("expires_at caution = %v, want %v", has, tt.expiryCaution)
			}
			if got := strings.Join(schemaEnumValues(spec, "state"), ","); got != tt.stateEnum {
				t.Errorf("state enum = %q, want %q", got, tt.stateEnum)
			}
		})
	}
}

// schemaEnumValues reads back the enum an input-schema override publishes for
// one property, so a test can state which values it names rather than only that
// an override is present.
func schemaEnumValues(spec toolutil.ActionSpec, property string) []string {
	for _, override := range spec.InputSchemaOverrides {
		if override.PropertyPath != property {
			continue
		}
		raw, _ := override.Values["enum"].([]any)
		values := make([]string, 0, len(raw))
		for _, value := range raw {
			text, _ := value.(string)
			values = append(values, text)
		}
		return values
	}
	return nil
}
