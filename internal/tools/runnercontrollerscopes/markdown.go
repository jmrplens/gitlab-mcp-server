// markdown.go provides Markdown formatting functions for runner controller scope MCP tool output.
package runnercontrollerscopes

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatScopesMarkdown renders scopes as Markdown.
func FormatScopesMarkdown(out ScopesOutput) string {
	var b strings.Builder
	b.WriteString("## Runner Controller Scopes\n\n")
	fmt.Fprintf(&b, "### Instance-Level Scopes (%d)\n\n", len(out.InstanceLevelScopings))
	if len(out.InstanceLevelScopings) == 0 {
		b.WriteString("No instance-level scopes configured.\n")
	} else {
		b.WriteString("| Created At | Updated At |\n")
		b.WriteString("| --- | --- |\n")
		for _, is := range out.InstanceLevelScopings {
			fmt.Fprintf(&b, "| %s | %s |\n", toolutil.FormatTime(is.CreatedAt), toolutil.FormatTime(is.UpdatedAt))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "### Runner-Level Scopes (%d)\n\n", len(out.RunnerLevelScopings))
	if len(out.RunnerLevelScopings) == 0 {
		b.WriteString("No runner-level scopes configured.\n")
	} else {
		b.WriteString("| Runner ID | Created At | Updated At |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, rs := range out.RunnerLevelScopings {
			fmt.Fprintf(&b, "| %d | %s | %s |\n", rs.RunnerID, toolutil.FormatTime(rs.CreatedAt), toolutil.FormatTime(rs.UpdatedAt))
		}
	}
	toolutil.WriteHints(&b, "Use `gitlab_runner_controller_scope_add_instance` to add a new scope")
	return b.String()
}

// FormatInstanceScopeMarkdown renders an instance scope result as Markdown.
func FormatInstanceScopeMarkdown(out InstanceScopeOutput) string {
	var b strings.Builder
	b.WriteString("## Instance-Level Scope\n\n")
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "- **Created At**: %s\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.UpdatedAt != "" {
		fmt.Fprintf(&b, "- **Updated At**: %s\n", toolutil.FormatTime(out.UpdatedAt))
	}
	toolutil.WriteHints(&b, "Use scope tools to manage this controller's scopes")
	return b.String()
}

// FormatRunnerScopeMarkdown renders a runner scope result as Markdown.
func FormatRunnerScopeMarkdown(out RunnerScopeOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Scope (Runner #%d)\n\n", out.RunnerID)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "- **Created At**: %s\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.UpdatedAt != "" {
		fmt.Fprintf(&b, "- **Updated At**: %s\n", toolutil.FormatTime(out.UpdatedAt))
	}
	toolutil.WriteHints(&b, "Use scope tools to manage this controller's scopes")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatScopesMarkdown)
	toolutil.RegisterMarkdown(FormatInstanceScopeMarkdown)
	toolutil.RegisterMarkdown(FormatRunnerScopeMarkdown)
}
