// action_specs_test.go contains unit tests for the badge action specs: the
// discovery guidance each spec publishes, and the not-found wrapper the two get
// routes are built with.
package badges

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestBadgeGuidance_NoScopeTag_DefaultsToProject asserts that badgeGuidance
// reads no tag when it has none, and answers with the project scope.
//
// The guard it exercises, `len(options.Tags) > 0`, is what stops the line
// beside it indexing an empty slice. Every caller in this package supplies a
// tag, so nothing else in the suite ever reaches the guard, and without this
// test a change that dropped it would pass every other assertion and panic the
// first time a spec was built without one.
func TestBadgeGuidance_NoScopeTag_DefaultsToProject(t *testing.T) {
	got := badgeGuidance("badge_list", toolutil.ActionSpecOptions{})

	if !strings.Contains(got.Usage, "project badge operations") {
		t.Fatalf("Usage = %q, want the project scope", got.Usage)
	}
	if _, ok := got.ParameterGuidance["project_id"]; !ok {
		t.Fatalf("ParameterGuidance keys = %v, want project_id", mapKeys(got.ParameterGuidance))
	}
	if _, ok := got.ParameterGuidance["group_id"]; ok {
		t.Fatalf("ParameterGuidance keys = %v, want no group_id", mapKeys(got.ParameterGuidance))
	}
}

// TestBadgeGuidance_AliasesFollowTheScope asserts that a group action publishes
// the group phrasings and a project action the project ones.
//
// The aliases are what `gitlab_find_action` matches a natural-language request
// against, so a scope that picked the other branch's set would answer "list
// group badges" with the project tool and never say so. The scope is decided
// once, for both the wording and the parameter names, and nothing in the
// response of any handler reflects it.
func TestBadgeGuidance_AliasesFollowTheScope(t *testing.T) {
	byTool := badgeSpecsByTool(t, allBadgeActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))))

	cases := []struct {
		tool  string
		want  string
		wrong string
	}{
		{"gitlab_list_group_badges", "list group badges", "list project badges"},
		{"gitlab_get_group_badge", "get group badge", "get project badge"},
		{"gitlab_add_group_badge", "add group badge", "add project badge"},
		{"gitlab_delete_group_badge", "delete group badge", "delete project badge"},
		{"gitlab_preview_group_badge", "preview group badge", "preview project badge"},
		{"gitlab_list_project_badges", "list project badges", "list group badges"},
		{"gitlab_get_project_badge", "get project badge", "get group badge"},
		{"gitlab_add_project_badge", "add project badge", "add group badge"},
		{"gitlab_delete_project_badge", "delete project badge", "delete group badge"},
		{"gitlab_preview_project_badge", "preview project badge", "preview group badge"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			aliases := byTool[tc.tool].Aliases
			if !containsText(aliases, tc.want) {
				t.Fatalf("%s aliases = %v, want one containing %q", tc.tool, aliases, tc.want)
			}
			if containsText(aliases, tc.wrong) {
				t.Fatalf("%s aliases = %v, want none containing %q", tc.tool, aliases, tc.wrong)
			}
		})
	}
}

// TestBadgeGuidance_UnknownVerb_KeepsTheCallerAliasesAndTheGenericWording
// asserts that an action name outside the six badge verbs falls through both
// alias switches untouched and is described generically.
//
// It matters because the fallback is what a newly added badge action gets
// before anyone writes its wording: the spec has to stay usable rather than
// carry another verb's aliases, and the last arm of each switch is otherwise
// only ever reached by the verb it names.
func TestBadgeGuidance_UnknownVerb_KeepsTheCallerAliasesAndTheGenericWording(t *testing.T) {
	cases := []struct {
		scope string
		tag   string
	}{
		{"project", "project"},
		{"group", "group"},
	}
	for _, tc := range cases {
		t.Run(tc.scope, func(t *testing.T) {
			got := badgeGuidance("badge_archive", toolutil.ActionSpecOptions{
				Aliases: []string{"gitlab_archive_badge"},
				Tags:    []string{tc.tag, "badge"},
			})

			if !strings.HasPrefix(got.Usage, "Manage "+tc.scope+" badges.") {
				t.Fatalf("Usage = %q, want the generic %s wording", got.Usage, tc.scope)
			}
			if !strings.HasPrefix(got.IndividualTool.Description, "Manage "+tc.scope+" badges.") {
				t.Fatalf("Description = %q, want the generic %s wording", got.IndividualTool.Description, tc.scope)
			}
			if len(got.Aliases) != 1 || got.Aliases[0] != "gitlab_archive_badge" {
				t.Fatalf("Aliases = %v, want the caller's own left untouched", got.Aliases)
			}
		})
	}
}

// TestBadgeGetRoute_ErrorOtherThan404_IsReturnedRatherThanReportedMissing
// asserts that a get route answers a 403 or a 500 with the error, and only a
// 404 with the not-found card.
//
// The wrapper's condition is `err != nil && IsHTTPStatus(err, 404)`, and
// loosening it to an `||` costs the truth: every failure becomes "badge N in
// project 1 does not exist", so a model told the credential lacks permission
// instead goes off to create a badge that is already there. A 404 test cannot
// see this, because a 404 satisfies both spellings.
func TestBadgeGetRoute_ErrorOtherThan404_IsReturnedRatherThanReportedMissing(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"forbidden", http.StatusForbidden, `{"message":"403 Forbidden"}`},
		{"server error", http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`},
	}
	tools := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_get_project_badge", map[string]any{"project_id": "1", testBadgeIDField: float64(1)}},
		{"gitlab_get_group_badge", map[string]any{"group_id": "1", testBadgeIDField: float64(1)}},
	}
	for _, tc := range cases {
		for _, target := range tools {
			t.Run(tc.name+"/"+target.tool, func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, tc.status, tc.body)
				}))
				byTool := badgeSpecsByTool(t, allBadgeActionSpecs(client))

				result, err := byTool[target.tool].Route.Handler(t.Context(), target.args)
				if err == nil {
					t.Fatalf("Route.Handler(%s) on %d returned result %#v and no error", target.tool, tc.status, result)
				}
				if _, ok := result.(badgeNotFoundOutput); ok {
					t.Fatalf("Route.Handler(%s) on %d reported the badge missing", target.tool, tc.status)
				}
			})
		}
	}
}

// mapKeys lists a guidance map's keys for a failure message.
func mapKeys(guidance map[string]toolutil.ParameterGuidance) []string {
	keys := make([]string, 0, len(guidance))
	for key := range guidance {
		keys = append(keys, key)
	}
	return keys
}
