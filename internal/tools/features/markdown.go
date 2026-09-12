package features

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders the instance's feature flags as a Markdown table:
// a collection of objects that share columns.
func FormatListMarkdown(output ListOutput) *mcp.CallToolResult {
	if len(output.Features) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("feature flags"))
	}

	var sb strings.Builder
	// The endpoint pages nothing, so the heading counts what GitLab sent.
	toolutil.WriteListHeading(&sb, "Feature Flags", len(output.Features), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("Name", "State", "Gates"))
	for _, f := range output.Features {
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(f.Name),
			toolutil.EscapeMdTableCell(f.State),
			toolutil.EscapeMdTableCell(formatGates(f.Gates)),
		))
	}
	toolutil.WriteListFooter(&sb, toolutil.PaginationOutput{}, false,
		"Use `gitlab_set_feature_flag` to toggle a specific feature")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatListDefinitionsMarkdown renders the feature definitions GitLab ships
// as a Markdown table.
func FormatListDefinitionsMarkdown(output ListDefinitionsOutput) *mcp.CallToolResult {
	if len(output.Definitions) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("feature definitions"))
	}

	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Feature Definitions", len(output.Definitions), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("Name", "Type", "Group", "Milestone", "Default Enabled"))
	for _, d := range output.Definitions {
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(d.Name),
			toolutil.EscapeMdTableCell(d.Type),
			toolutil.EscapeMdTableCell(d.Group),
			toolutil.EscapeMdTableCell(d.Milestone),
			toolutil.BoolEmoji(d.DefaultEnabled),
		))
	}
	toolutil.WriteListFooter(&sb, toolutil.PaginationOutput{}, false,
		"Use `gitlab_set_feature_flag` to enable or disable a feature")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatFeatureMarkdown renders one feature flag as the card of one object,
// with the definition GitLab ships for it as a nested object.
func FormatFeatureMarkdown(output SetOutput) *mcp.CallToolResult {
	f := output.Feature
	var b strings.Builder
	// A feature flag name is the string the caller of feature.set supplied,
	// which GitLab creates on demand rather than validating against a list.
	c := toolutil.NewCard(&b, "Feature Flag: "+f.Name)
	c.Field("State", f.State)
	c.Field("Gates", formatGates(f.Gates))
	writeDefinition(c, f.Definition)
	c.End("Use `gitlab_set_feature_flag` to toggle this feature")
	return toolutil.ToolResultWithMarkdown(b.String())
}

// writeDefinition writes the flag's definition as a nested object, and writes
// nothing when GitLab shipped none. Everything under it is read out of the
// definition file in GitLab's own source tree, so no value here is from a set
// this server can know.
func writeDefinition(c *toolutil.Card, d *DefinitionItem) {
	if d == nil {
		return
	}
	def := c.Sub("Definition")
	def.Field("Type", d.Type)
	def.Field("Group", d.Group)
	def.Field("Milestone", d.Milestone)
	def.Bool("Default Enabled", d.DefaultEnabled)
	def.Bool("Log State Changes", d.LogStateChanges)
	def.Link("Introduced By", d.IntroducedByURL, d.IntroducedByURL)
	def.Link("Rollout Issue", d.RolloutIssueURL, d.RolloutIssueURL)
	def.Link("Feature Issue", d.FeatureIssueURL, d.FeatureIssueURL)
	def.Field("Intended To Roll Out By", d.IntendedToRolloutBy)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatListDefinitionsMarkdown)
	toolutil.RegisterMarkdownResult(FormatFeatureMarkdown)
}
