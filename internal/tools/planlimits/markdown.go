package planlimits

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatGetMarkdown formats plan limits as markdown.
func FormatGetMarkdown(out GetOutput) string {
	var sb strings.Builder
	sb.WriteString("## Plan Limits\n\n")
	sb.WriteString("| Limit | Value |\n")
	sb.WriteString("|---|---|\n")
	fmt.Fprintf(&sb, "| Conan Max File Size | %d |\n", out.ConanMaxFileSize)
	fmt.Fprintf(&sb, "| Generic Packages Max File Size | %d |\n", out.GenericPackagesMaxFileSize)
	fmt.Fprintf(&sb, "| Helm Max File Size | %d |\n", out.HelmMaxFileSize)
	fmt.Fprintf(&sb, "| Maven Max File Size | %d |\n", out.MavenMaxFileSize)
	fmt.Fprintf(&sb, "| NPM Max File Size | %d |\n", out.NPMMaxFileSize)
	fmt.Fprintf(&sb, "| NuGet Max File Size | %d |\n", out.NugetMaxFileSize)
	fmt.Fprintf(&sb, "| PyPI Max File Size | %d |\n", out.PyPiMaxFileSize)
	fmt.Fprintf(&sb, "| Terraform Module Max File Size | %d |\n", out.TerraformModuleMaxFileSize)
	toolutil.WriteHints(&sb, "Use `gitlab_change_plan_limits` to modify these limits")
	return sb.String()
}

// FormatChangeMarkdown formats changed plan limits as markdown.
func FormatChangeMarkdown(out ChangeOutput) string {
	var sb strings.Builder
	sb.WriteString("## Updated Plan Limits\n\n")
	sb.WriteString("| Limit | Value |\n")
	sb.WriteString("|---|---|\n")
	fmt.Fprintf(&sb, "| Conan Max File Size | %d |\n", out.ConanMaxFileSize)
	fmt.Fprintf(&sb, "| Generic Packages Max File Size | %d |\n", out.GenericPackagesMaxFileSize)
	fmt.Fprintf(&sb, "| Helm Max File Size | %d |\n", out.HelmMaxFileSize)
	fmt.Fprintf(&sb, "| Maven Max File Size | %d |\n", out.MavenMaxFileSize)
	fmt.Fprintf(&sb, "| NPM Max File Size | %d |\n", out.NPMMaxFileSize)
	fmt.Fprintf(&sb, "| NuGet Max File Size | %d |\n", out.NugetMaxFileSize)
	fmt.Fprintf(&sb, "| PyPI Max File Size | %d |\n", out.PyPiMaxFileSize)
	fmt.Fprintf(&sb, "| Terraform Module Max File Size | %d |\n", out.TerraformModuleMaxFileSize)
	toolutil.WriteHints(&sb, "Verify changes with `gitlab_get_plan_limits`")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatChangeMarkdown)
}
