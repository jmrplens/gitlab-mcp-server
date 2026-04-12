package securitysettings

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatProjectMarkdown renders project security settings as Markdown.
func FormatProjectMarkdown(o ProjectOutput) string {
	if o.ProjectID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Security Settings (Project %d)\n\n", o.ProjectID)
	fmt.Fprintf(&b, "| Setting | Enabled |\n")
	fmt.Fprintf(&b, "| ------- | :-----: |\n")
	fmt.Fprintf(&b, "| Secret Push Protection | %t |\n", o.SecretPushProtectionEnabled)
	fmt.Fprintf(&b, "| Continuous Vulnerability Scans | %t |\n", o.ContinuousVulnerabilityScansEnabled)
	fmt.Fprintf(&b, "| Container Scanning for Registry | %t |\n", o.ContainerScanningForRegistryEnabled)
	fmt.Fprintf(&b, "| Auto-fix SAST | %t |\n", o.AutoFixSAST)
	fmt.Fprintf(&b, "| Auto-fix DAST | %t |\n", o.AutoFixDAST)
	fmt.Fprintf(&b, "| Auto-fix Dependency Scanning | %t |\n", o.AutoFixDependencyScanning)
	fmt.Fprintf(&b, "| Auto-fix Container Scanning | %t |\n", o.AutoFixContainerScanning)
	if o.UpdatedAt != "" {
		fmt.Fprintf(&b, "\n**Updated**: %s\n", o.UpdatedAt)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_update_project_secret_push_protection` to toggle security features",
	)
	return b.String()
}

// FormatGroupMarkdown renders group security settings as Markdown.
func FormatGroupMarkdown(o GroupOutput) string {
	var b strings.Builder
	b.WriteString("## Group Security Settings\n\n")
	fmt.Fprintf(&b, "- **Secret Push Protection**: %t\n", o.SecretPushProtectionEnabled)
	if len(o.Errors) > 0 {
		b.WriteString("\n**Errors**:\n")
		for _, e := range o.Errors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_update_group_secret_push_protection` to toggle group security settings",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatProjectMarkdown)
	toolutil.RegisterMarkdown(FormatGroupMarkdown)
}
