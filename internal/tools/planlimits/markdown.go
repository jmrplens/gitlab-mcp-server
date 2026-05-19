package planlimits

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatGetMarkdown formats plan limits as markdown.
func FormatGetMarkdown(out GetOutput) string {
	return formatPlanLimitsMarkdown("Plan Limits", out.PlanLimitItem, "Use `gitlab_change_plan_limits` to modify these limits")
}

// FormatChangeMarkdown formats changed plan limits as markdown.
func FormatChangeMarkdown(out ChangeOutput) string {
	return formatPlanLimitsMarkdown("Updated Plan Limits", out.PlanLimitItem, "Verify changes with `gitlab_get_plan_limits`")
}

func formatPlanLimitsMarkdown(title string, limits PlanLimitItem, hint string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", title)
	sb.WriteString("| Limit | Value |\n")
	sb.WriteString("|---|---|\n")
	fmt.Fprintf(&sb, "| Conan Max File Size | %d |\n", limits.ConanMaxFileSize)
	fmt.Fprintf(&sb, "| Generic Packages Max File Size | %d |\n", limits.GenericPackagesMaxFileSize)
	fmt.Fprintf(&sb, "| Helm Max File Size | %d |\n", limits.HelmMaxFileSize)
	fmt.Fprintf(&sb, "| Maven Max File Size | %d |\n", limits.MavenMaxFileSize)
	fmt.Fprintf(&sb, "| NPM Max File Size | %d |\n", limits.NPMMaxFileSize)
	fmt.Fprintf(&sb, "| NuGet Max File Size | %d |\n", limits.NugetMaxFileSize)
	fmt.Fprintf(&sb, "| PyPI Max File Size | %d |\n", limits.PyPiMaxFileSize)
	fmt.Fprintf(&sb, "| Terraform Module Max File Size | %d |\n", limits.TerraformModuleMaxFileSize)
	toolutil.WriteHints(&sb, hint)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatChangeMarkdown)
}
