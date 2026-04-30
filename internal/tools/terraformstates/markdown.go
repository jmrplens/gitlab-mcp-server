// markdown.go provides Markdown formatting functions for Terraform state MCP tool output.
package terraformstates

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats Terraform states as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Terraform States\n\n")
	if len(out.States) == 0 {
		sb.WriteString("No Terraform states found.\n")
		return sb.String()
	}
	sb.WriteString("| Name | Latest Serial |\n|------|---------------|\n")
	for _, s := range out.States {
		fmt.Fprintf(&sb, "| %s | %d |\n", toolutil.EscapeMdTableCell(s.Name), s.LatestSerial)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_terraform_state` to view details of a specific state")
	return sb.String()
}

// FormatStateMarkdown formats a single Terraform state as markdown.
func FormatStateMarkdown(s StateItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Terraform State: %s\n\n- **Latest Serial**: %d\n- **Download Path**: %s\n",
		s.Name, s.LatestSerial, s.DownloadPath)
	toolutil.WriteHints(&b,
		"Use `gitlab_lock_terraform_state` to lock this state",
		"Use `gitlab_delete_terraform_state` to remove it",
	)
	return b.String()
}

// FormatLockMarkdown formats a lock/unlock result as markdown.
func FormatLockMarkdown(out LockOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Terraform State Lock\n\n- **Success**: %v\n- **Message**: %s\n", out.Success, out.Message)
	toolutil.WriteHints(&b, "Use `gitlab_get_terraform_state` to verify lock status")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatStateMarkdown)
	toolutil.RegisterMarkdown(FormatLockMarkdown)
}
