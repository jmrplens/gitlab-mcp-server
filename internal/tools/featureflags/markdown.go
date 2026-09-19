package featureflags

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical action IDs the hints below name are declared once in
// action_specs.go, beside the specs that define them, so the hints and the
// RelatedActions metadata cannot drift apart again.

// strategyTargetCell names the user list a gitlabUserList strategy targets.
// Without it the strategy row said "gitlabUserList" and nothing about which
// list, which is the whole of what that strategy does.
func strategyTargetCell(list *StrategyUserListOutput) string {
	if list == nil {
		return "-"
	}
	return fmt.Sprintf("%s (IID %d)", toolutil.EscapeMdTableCell(list.Name), list.IID)
}

// writeStrategies writes the flag's activation strategies as the nested
// collection they are, under a heading of their own.
func writeStrategies(c *toolutil.Card, strategies []StrategyOutput) {
	if len(strategies) == 0 {
		return
	}
	table := c.Table("Strategies", "ID", "Name", "Parameters", "Target", "Scopes")
	for _, s := range strategies {
		table.Row(
			strconv.FormatInt(s.ID, 10),
			toolutil.EscapeMdTableCell(s.Name),
			toolutil.EscapeMdTableCell(formatParameters(s.Parameters)),
			strategyTargetCell(s.UserList),
			toolutil.EscapeMdTableCell(formatScopes(s.Scopes)),
		)
	}
}

// FormatFeatureFlagMarkdown renders one feature flag as the card of one
// object, with its strategies as a nested collection.
func FormatFeatureFlagMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Feature Flag: "+out.Name)
	c.Field("Name", out.Name)
	// The description is whatever a maintainer typed against the flag.
	c.Text("Description", out.Description)
	c.Bool("Active", out.Active)
	c.Field("Version", out.Version)
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	if len(out.Scopes) > 0 {
		c.Field("Scopes", formatScopes(out.Scopes))
	}
	writeStrategies(c, out.Strategies)
	c.End(
		toolutil.HintAction(actionFeatureFlagUpdate, "toggle this flag active or inactive"),
		toolutil.HintAction(actionFeatureFlagDelete, "remove this feature flag"),
		toolutil.HintAction(actionFeatureFlagUserListGet, "read a user list a strategy targets"),
	)
	return b.String()
}

// FormatListFeatureFlagsMarkdown renders a page of feature flags as a Markdown
// table: a collection of objects that share columns.
func FormatListFeatureFlagsMarkdown(out ListOutput) string {
	if len(out.FeatureFlags) == 0 {
		return toolutil.EmptyMessage("feature flags")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Feature Flags", len(out.FeatureFlags), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Active", "Version", "Strategies"))
	for _, f := range out.FeatureFlags {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(f.Name),
			toolutil.BoolEmoji(f.Active),
			toolutil.EscapeMdTableCell(f.Version),
			strconv.Itoa(len(f.Strategies)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionFeatureFlagGet, "read one flag with its strategies and scopes"),
		toolutil.HintAction(actionFeatureFlagCreate, "add a new feature flag"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatFeatureFlagMarkdown)
	toolutil.RegisterMarkdown(FormatListFeatureFlagsMarkdown)
}
