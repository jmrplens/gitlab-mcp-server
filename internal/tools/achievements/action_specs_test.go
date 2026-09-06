// action_specs_test.go asserts the canonical metadata every achievement action
// carries: the twelve routes, their declared individual-tool names, their
// annotations, and the discovery metadata the catalog audits require.
package achievements

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// specsByAction indexes the canonical specs by action name. The transport is
// never reached, because these tests read metadata rather than call routes.
func specsByAction(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byAction := map[string]toolutil.ActionSpec{}
	for _, spec := range ActionSpecs(client) {
		byAction[spec.Name] = spec
	}
	return byAction
}

// TestActionSpecs_CoversEveryServiceMethod asserts that each of the twelve
// AchievementsService methods has exactly one canonical action, under the
// declared individual-tool name. The names are declared rather than derived, so
// a rename has to be made here and in the documentation together.
func TestActionSpecs_CoversEveryServiceMethod(t *testing.T) {
	byAction := specsByAction(t)
	if len(byAction) != 12 {
		t.Fatalf("unique actions = %d, want 12", len(byAction))
	}

	cases := []struct {
		action string
		tool   string
	}{
		{action: "create", tool: toolCreate},
		{action: "update", tool: toolUpdate},
		{action: "delete", tool: toolDelete},
		{action: "award", tool: toolAward},
		{action: "revoke", tool: toolRevoke},
		{action: "user_achievement_update", tool: toolUserAchievementUpdate},
		{action: "user_achievement_delete", tool: toolUserAchievementDelete},
		{action: "user_achievement_reorder", tool: toolUserAchievementReorder},
		{action: "user_list", tool: toolUserList},
		{action: "list", tool: toolList},
		{action: "recipients", tool: toolRecipients},
		{action: "unique_users", tool: toolUniqueUsers},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			spec, ok := byAction[tc.action]
			if !ok {
				t.Fatalf("action %q is missing from ActionSpecs", tc.action)
			}
			if spec.IndividualTool.Name != tc.tool {
				t.Errorf("IndividualTool.Name = %q, want %q", spec.IndividualTool.Name, tc.tool)
			}
			if spec.IndividualTool.Title == "" {
				t.Error("IndividualTool.Title is empty, want a title derived from the tool name")
			}
			if spec.OwnerPackage != "achievements" {
				t.Errorf("OwnerPackage = %q, want achievements", spec.OwnerPackage)
			}
		})
	}
}

// TestActionSpecs_Annotations asserts the read-only, destructive and idempotent
// hints match what each action actually does. The hints are advisory, so their
// only effect is on what a model decides to do with them.
func TestActionSpecs_Annotations(t *testing.T) {
	byAction := specsByAction(t)
	cases := []struct {
		action          string
		wantReadOnly    bool
		wantDestructive bool
		wantIdempotent  bool
	}{
		{action: "list", wantReadOnly: true, wantIdempotent: true},
		{action: "user_list", wantReadOnly: true, wantIdempotent: true},
		{action: "recipients", wantReadOnly: true, wantIdempotent: true},
		{action: "unique_users", wantReadOnly: true, wantIdempotent: true},
		{action: "create"},
		{action: "award"},
		{action: "update", wantIdempotent: true},
		{action: "user_achievement_update", wantIdempotent: true},
		{action: "user_achievement_reorder", wantIdempotent: true},
		{action: "delete", wantDestructive: true, wantIdempotent: true},
		{action: "revoke", wantDestructive: true, wantIdempotent: true},
		{action: "user_achievement_delete", wantDestructive: true, wantIdempotent: true},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			spec := byAction[tc.action]
			if spec.ReadOnly != tc.wantReadOnly {
				t.Errorf("ReadOnly = %v, want %v", spec.ReadOnly, tc.wantReadOnly)
			}
			if spec.Destructive != tc.wantDestructive {
				t.Errorf("Destructive = %v, want %v", spec.Destructive, tc.wantDestructive)
			}
			if spec.Route.Destructive != tc.wantDestructive {
				t.Errorf("Route.Destructive = %v, want %v", spec.Route.Destructive, tc.wantDestructive)
			}
			if spec.Idempotent != tc.wantIdempotent {
				t.Errorf("Idempotent = %v, want %v", spec.Idempotent, tc.wantIdempotent)
			}
			if !spec.OpenWorld {
				t.Error("OpenWorld = false, want true for an action that reaches a GitLab instance")
			}
		})
	}
}

// TestActionSpecs_EditionIsFree asserts no action carries an edition tag. The
// documented availability is "Tier: Free, Premium, Ultimate" and the GraphQL
// reference marks no achievement field as Premium or Ultimate, so a tag here
// would hide the whole domain from Community Edition catalogs.
func TestActionSpecs_EditionIsFree(t *testing.T) {
	for action, spec := range specsByAction(t) {
		t.Run(action, func(t *testing.T) {
			if spec.Edition != "" {
				t.Errorf("Edition = %q, want empty so the action reaches Free instances", spec.Edition)
			}
			if spec.GitLabDotComOnly {
				t.Error("GitLabDotComOnly = true, want false: achievements are offered on Self-Managed too")
			}
		})
	}
}

// TestActionSpecs_DiscoveryMetadata asserts the four buckets the discovery
// completeness audit grades: at least three aliases that are not the tool name,
// a curated usage string, related actions, and an individual-tool description
// in the "Returns: ... See also: ..." form.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	for action, spec := range specsByAction(t) {
		t.Run(action, func(t *testing.T) {
			if natural := naturalAliasCount(spec); natural < 3 {
				t.Errorf("natural-language aliases = %d, want at least 3, got %v", natural, spec.Aliases)
			}
			if len(spec.Usage) < 40 {
				t.Errorf("Usage = %q, want a curated sentence rather than a placeholder", spec.Usage)
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("RelatedActions is empty, want sibling actions a model can move on to")
			}
			if len(spec.ParameterGuidance) == 0 {
				t.Error("ParameterGuidance is empty, want an entry for the confusable parameters")
			}
			if len(spec.Tags) == 0 {
				t.Error("Tags is empty, want search tags")
			}
			description := spec.IndividualTool.Description
			if !strings.Contains(description, "Returns:") || !strings.Contains(description, "See also:") {
				t.Errorf("IndividualTool.Description = %q, want the Returns/See also form", description)
			}
		})
	}
}

// naturalAliasCount counts the aliases that carry a natural-language signal,
// which is every alias that is neither the canonical ID nor the tool name. The
// discovery audit counts them the same way.
func naturalAliasCount(spec toolutil.ActionSpec) int {
	natural := 0
	for _, alias := range spec.Aliases {
		if alias != spec.Name && alias != spec.IndividualTool.Name {
			natural++
		}
	}
	return natural
}

// TestActionSpecs_RelatedActionsResolve asserts every related action either
// names a sibling in this domain or is qualified with another domain's prefix.
// A typo here is invisible at runtime and only shows up as a dead-end
// suggestion in a model's next step.
func TestActionSpecs_RelatedActionsResolve(t *testing.T) {
	byAction := specsByAction(t)
	known := map[string]bool{}
	for _, spec := range byAction {
		known["achievement."+spec.Name] = true
	}
	for action, spec := range byAction {
		t.Run(action, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				if !strings.HasPrefix(related, "achievement.") {
					continue
				}
				if !known[related] {
					t.Errorf("RelatedActions entry %q names no action in this domain", related)
				}
				if related == "achievement."+spec.Name {
					t.Errorf("RelatedActions entry %q points at the action itself", related)
				}
			}
		})
	}
}

// TestActionSpecs_InputSchemasDescribeEveryParameter asserts every input
// property carries a description, which is the field-level bucket of the
// discovery audit and what a model reads when choosing an argument.
func TestActionSpecs_InputSchemasDescribeEveryParameter(t *testing.T) {
	for action, spec := range specsByAction(t) {
		t.Run(action, func(t *testing.T) {
			properties, ok := spec.Route.InputSchema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("InputSchema has no properties map, got %#v", spec.Route.InputSchema)
			}
			if len(properties) == 0 {
				t.Fatal("InputSchema declares no properties, want at least the addressing parameter")
			}
			for name, raw := range properties {
				property, isMap := raw.(map[string]any)
				if !isMap {
					t.Errorf("property %q = %#v, want a schema object", name, raw)
					continue
				}
				if description, _ := property["description"].(string); strings.TrimSpace(description) == "" {
					t.Errorf("property %q has no description", name)
				}
			}
		})
	}
}

// TestActionSpecs_AvatarActionsDeclareBothFileRoutes asserts create and update
// both offer the local path and the inline bytes, and that the guidance says a
// caller must pick exactly one. The path half is refused over HTTP, so a model
// that cannot see the base64 half has no way to send an avatar at all there.
func TestActionSpecs_AvatarActionsDeclareBothFileRoutes(t *testing.T) {
	byAction := specsByAction(t)
	for _, action := range []string{"create", "update"} {
		t.Run(action, func(t *testing.T) {
			spec := byAction[action]
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			for _, field := range []string{"avatar_file_path", "avatar_content_base64", "avatar_filename", "avatar_content_type"} {
				if _, ok := properties[field]; !ok {
					t.Errorf("input schema is missing %q", field)
				}
			}
			guidance, ok := spec.ParameterGuidance["avatar_file_path"]
			if !ok {
				t.Fatal("avatar_file_path has no parameter guidance")
			}
			confusions := strings.Join(guidance.CommonConfusions, " ")
			if !strings.Contains(confusions, "never both") {
				t.Errorf("avatar_file_path confusions = %q, want the exactly-one rule", confusions)
			}
			if !strings.Contains(confusions, "HTTP") {
				t.Errorf("avatar_file_path confusions = %q, want the HTTP-mode refusal named", confusions)
			}
		})
	}
}

// TestActionSpecs_IDGuidanceSeparatesTheTwoIdentifiers asserts every action
// addressed by an award ID says so, because confusing an achievement_id with a
// user_achievement_id is the mistake this domain invites.
func TestActionSpecs_IDGuidanceSeparatesTheTwoIdentifiers(t *testing.T) {
	byAction := specsByAction(t)
	cases := []struct {
		action string
		param  string
	}{
		{action: "revoke", param: "user_achievement_id"},
		{action: "user_achievement_update", param: "user_achievement_id"},
		{action: "user_achievement_delete", param: "user_achievement_id"},
		{action: "update", param: "achievement_id"},
		{action: "delete", param: "achievement_id"},
		{action: "award", param: "achievement_id"},
		{action: "recipients", param: "achievement_id"},
		{action: "unique_users", param: "achievement_id"},
	}
	for _, tc := range cases {
		t.Run(tc.action+"/"+tc.param, func(t *testing.T) {
			guidance, ok := byAction[tc.action].ParameterGuidance[tc.param]
			if !ok {
				t.Fatalf("%s has no guidance for %s", tc.action, tc.param)
			}
			if !strings.Contains(strings.Join(guidance.CommonConfusions, " "), "different numbers") {
				t.Errorf("%s guidance does not distinguish the two identifiers: %v", tc.param, guidance.CommonConfusions)
			}
		})
	}
}

// TestActionSpecs_ListActionsAreListContent asserts the paginated reads declare
// the list content kind, which is what drives the next-step hints and the
// Markdown formatter lookup for a list result.
func TestActionSpecs_ListActionsAreListContent(t *testing.T) {
	byAction := specsByAction(t)
	for _, action := range []string{"list", "user_list", "recipients", "unique_users"} {
		t.Run(action, func(t *testing.T) {
			if got := byAction[action].ContentKind; got != toolutil.ActionSpecContentList {
				t.Errorf("ContentKind = %q, want %q", got, toolutil.ActionSpecContentList)
			}
		})
	}
	for _, action := range []string{"create", "update", "delete", "award", "revoke"} {
		t.Run(action, func(t *testing.T) {
			if got := byAction[action].ContentKind; got != toolutil.ActionSpecContentMutate {
				t.Errorf("ContentKind = %q, want %q", got, toolutil.ActionSpecContentMutate)
			}
		})
	}
}

// TestActionSpecs_OutputsHaveMarkdownFormatters asserts every route's output
// type is registered with the Markdown registry, so no action falls back to raw
// JSON when a client asks for Markdown.
func TestActionSpecs_OutputsHaveMarkdownFormatters(t *testing.T) {
	for action, spec := range specsByAction(t) {
		t.Run(action, func(t *testing.T) {
			if spec.Route.OutputType == nil {
				t.Fatal("Route.OutputType is nil, want a typed output")
			}
			if !toolutil.HasRegisteredMarkdownFormatter(spec.Route.OutputType) {
				t.Errorf("output type %s has no registered Markdown formatter", spec.Route.OutputType)
			}
		})
	}
}
