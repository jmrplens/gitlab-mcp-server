package featureflags

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatFeatureFlagMarkdown formats a single feature flag as markdown.
func FormatFeatureFlagMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Feature Flag: %s\n\n", toolutil.EscapeMdTableCell(out.Name))
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	fmt.Fprintf(&b, "| Active | %t |\n", out.Active)
	fmt.Fprintf(&b, "| Version | %s |\n", toolutil.EscapeMdTableCell(out.Version))
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.UpdatedAt != "" {
		fmt.Fprintf(&b, "| Updated | %s |\n", toolutil.FormatTime(out.UpdatedAt))
	}
	if len(out.Strategies) > 0 {
		b.WriteString("\n### Strategies\n\n")
		b.WriteString("| ID | Name | Parameters | Scopes |\n|---|---|---|---|\n")
		for _, s := range out.Strategies {
			params := formatParameters(s.Parameters)
			scopes := formatScopes(s.Scopes)
			fmt.Fprintf(&b, "| %d | %s | %s | %s |\n",
				s.ID,
				toolutil.EscapeMdTableCell(s.Name),
				toolutil.EscapeMdTableCell(params),
				toolutil.EscapeMdTableCell(scopes))
		}
	}
	toolutil.WriteHints(
		&b,
		"Use action 'feature_flag_update' to toggle active/inactive",
		"Use action 'feature_flag_delete' to remove this feature flag",
	)
	return b.String()
}

// FormatListFeatureFlagsMarkdown formats a list of feature flags as markdown.
func FormatListFeatureFlagsMarkdown(out ListOutput) string {
	var b strings.Builder
	b.WriteString("## Feature Flags\n\n")
	toolutil.WriteListSummary(&b, len(out.FeatureFlags), out.Pagination)
	if len(out.FeatureFlags) == 0 {
		b.WriteString("No feature flags found.\n")
		return b.String()
	}
	b.WriteString("| Name | Active | Version | Strategies |\n|---|---|---|---|\n")
	for _, f := range out.FeatureFlags {
		fmt.Fprintf(&b, "| %s | %t | %s | %d |\n",
			toolutil.EscapeMdTableCell(f.Name),
			f.Active,
			toolutil.EscapeMdTableCell(f.Version),
			len(f.Strategies))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'feature_flag_get' with name for full flag details and strategies",
		"Use action 'feature_flag_create' to add a new feature flag",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatFeatureFlagMarkdown)
	toolutil.RegisterMarkdown(FormatListFeatureFlagsMarkdown)
}
