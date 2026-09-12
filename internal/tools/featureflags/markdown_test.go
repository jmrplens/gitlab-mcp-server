// markdown_test.go asserts the whole Markdown document each feature flag
// formatter writes: the flag card with its strategies as the nested collection
// they are, and the list table.
package featureflags

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// flagCardHints is the guidance section every flag card closes with.
const flagCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'feature_flags.feature_flag_update' to toggle this flag active or inactive\n" +
	"- Use action 'feature_flags.feature_flag_delete' to remove this feature flag\n" +
	"- Use action 'feature_flags.ff_user_list_get' to read a user list a strategy targets\n"

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatFeatureFlagMarkdown verifies the flag card and the strategies table
// under it, including the target column that names the user list a
// gitlabUserList strategy applies to.
func TestFormatFeatureFlagMarkdown(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, FormatFeatureFlagMarkdown(Output{
			Name:        "my-flag",
			Description: "Test feature flag",
			Active:      true,
			Version:     "new_version_flag",
			CreatedAt:   "2026-01-01T00:00:00Z",
			UpdatedAt:   "2026-01-02T00:00:00Z",
			Scopes:      []ScopeOutput{{ID: 7, EnvironmentScope: "production"}},
			Strategies: []StrategyOutput{
				{
					ID:   1,
					Name: "gradualRolloutUserId",
					Parameters: &StrategyParameterOutput{
						Percentage: "50",
						GroupID:    "default",
						Stickiness: "default",
					},
					Scopes: []ScopeOutput{{ID: 10, EnvironmentScope: "production"}},
				},
				{
					ID:       2,
					Name:     "gitlabUserList",
					UserList: &StrategyUserListOutput{ID: 30, IID: 3, Name: "beta testers"},
				},
			},
		}),
			"## Feature Flag: my-flag\n\n"+
				"- **Name**: my-flag\n"+
				"- **Description**: Test feature flag\n"+
				"- **Active**: ✅\n"+
				"- **Version**: new_version_flag\n"+
				"- **Created**: 1 Jan 2026 00:00 UTC\n"+
				"- **Updated**: 2 Jan 2026 00:00 UTC\n"+
				"- **Scopes**: production\n\n"+
				"### Strategies\n\n"+
				"| ID | Name | Parameters | Target | Scopes |\n"+
				"| --- | --- | --- | --- | --- |\n"+
				"| 1 | gradualRolloutUserId | percentage=50, groupId=default, stickiness=default | - | production |\n"+
				"| 2 | gitlabUserList | - | beta testers (IID 3) | - |\n"+
				flagCardHints)
	})
	t.Run("optional rows and the strategies section are omitted", func(t *testing.T) {
		assertRendered(t, FormatFeatureFlagMarkdown(Output{Name: "bare-flag", Active: false, Version: "legacy_flag"}),
			"## Feature Flag: bare-flag\n\n"+
				"- **Name**: bare-flag\n"+
				"- **Active**: ❌\n"+
				"- **Version**: legacy_flag\n"+
				flagCardHints)
	})
}

// TestFormatListFeatureFlagsMarkdown verifies the flag table, the heading count
// that follows the response's own total, and the sentence an empty page renders
// instead of a heading counting zero.
func TestFormatListFeatureFlagsMarkdown(t *testing.T) {
	listHints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'feature_flags.feature_flag_get' to read one flag with its strategies and scopes\n" +
		"- Use action 'feature_flags.feature_flag_create' to add a new feature flag\n"
	t.Run("with flags", func(t *testing.T) {
		assertRendered(t, FormatListFeatureFlagsMarkdown(ListOutput{
			FeatureFlags: []Output{
				{Name: "flag-1", Active: true, Version: "new_version_flag"},
				{Name: "flag-2", Active: false, Version: "legacy_flag"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1},
		}),
			"## Feature Flags (2)\n\n"+
				"| Name | Active | Version | Strategies |\n"+
				"| --- | --- | --- | --- |\n"+
				"| flag-1 | ✅ | new_version_flag | 0 |\n"+
				"| flag-2 | ❌ | legacy_flag | 0 |\n"+
				"\nPage 1 of 1\n"+
				listHints)
	})
	t.Run("with pagination", func(t *testing.T) {
		assertRendered(t, FormatListFeatureFlagsMarkdown(ListOutput{
			FeatureFlags: []Output{{Name: "f1", Active: true, Version: "v1", Strategies: []StrategyOutput{{ID: 1}}}},
			Pagination:   toolutil.PaginationOutput{Page: 1, TotalPages: 3, TotalItems: 50, PerPage: 20},
		}),
			"## Feature Flags (50)\n\n"+
				"Showing 1 of 50 results (page 1 of 3)\n\n"+
				"| Name | Active | Version | Strategies |\n"+
				"| --- | --- | --- | --- |\n"+
				"| f1 | ✅ | v1 | 1 |\n"+
				"\nPage 1 of 3 | 50 items total | 20 per page\n"+
				listHints)
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatListFeatureFlagsMarkdown(ListOutput{FeatureFlags: []Output{}}), "No feature flags found.\n")
	})
}

// TestFeatureFlagFormattersAreRegistered verifies each output type resolves
// through the shared Markdown registry, which is how a tool result reaches its
// formatter at runtime.
func TestFeatureFlagFormattersAreRegistered(t *testing.T) {
	flag := Output{Name: "my-flag", Active: true, Version: "new_version_flag"}
	list := ListOutput{FeatureFlags: []Output{flag}}
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{name: "Output", output: flag, rendered: FormatFeatureFlagMarkdown(flag)},
		{name: "ListOutput", output: list, rendered: FormatListFeatureFlagsMarkdown(list)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := toolutil.MarkdownForResult(tc.output)
			if result == nil {
				t.Fatal("MarkdownForResult returned nil, want a registered formatter for the type")
			}
			if len(result.Content) != 1 {
				t.Fatalf("content blocks = %d, want 1", len(result.Content))
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
			}
			assertRendered(t, text.Text, tc.rendered)
		})
	}
}
