package settings

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatGetMarkdown formats application settings into a concise markdown summary.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	var sb strings.Builder
	sb.WriteString("# Application Settings\n\n")

	categories := []struct {
		title string
		keys  []string
	}{
		{"General", []string{
			"signup_enabled", "sign_in_text", "after_sign_out_path",
			"default_project_visibility", "default_group_visibility",
			"default_snippet_visibility", "restricted_visibility_levels",
			"can_create_group", "user_default_external",
		}},
		{"CI/CD", []string{
			"auto_devops_enabled", "auto_devops_domain",
			"shared_runners_enabled", "max_artifacts_size",
			"default_ci_config_path", "ci_max_includes",
		}},
		{"Authentication", []string{
			"password_authentication_enabled_for_web",
			"password_authentication_enabled_for_git",
			"two_factor_grace_period", "require_two_factor_authentication",
		}},
		{"Repository", []string{
			"default_branch_name", "default_branch_protection",
			"max_attachment_size", "max_import_size",
		}},
		{"Rate Limits", []string{
			"throttle_authenticated_api_enabled",
			"throttle_unauthenticated_api_enabled",
			"throttle_authenticated_api_requests_per_period",
			"throttle_unauthenticated_api_requests_per_period",
		}},
	}

	for _, cat := range categories {
		sb.WriteString("## ")
		sb.WriteString(cat.title)
		sb.WriteString("\n\n| Setting | Value |\n|---|---|\n")
		for _, key := range cat.keys {
			val, ok := out.Settings[key]
			if !ok {
				continue
			}
			sb.WriteString("| ")
			sb.WriteString(key)
			sb.WriteString(" | ")
			fmt.Fprintf(&sb, "%v", val)
			sb.WriteString(" |\n")
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "*Total settings: %d*\n", len(out.Settings))
	toolutil.WriteHints(&sb, "Use `gitlab_update_application_settings` to change settings")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatUpdateMarkdown formats the updated settings response.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	var sb strings.Builder
	sb.WriteString("# Application Settings Updated\n\n")
	fmt.Fprintf(&sb, "Settings updated successfully. Total settings: %d\n", len(out.Settings))
	toolutil.WriteHints(&sb, "Verify changes with `gitlab_get_application_settings`")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}
