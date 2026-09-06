package mrapprovalsettings

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// formatSetting renders one setting as the three cells of its row.
//
// The pipes in the result are the column separators this row is made of, so
// the escaping goes on the one value that is not this package's own: the name
// of the group a setting was inherited from.
func formatSetting(s SettingOutput) string {
	val := toolutil.BoolEmoji(s.Value)
	locked := toolutil.BoolEmoji(s.Locked)
	inherited := "-"
	if s.InheritedFrom != "" {
		inherited = toolutil.EscapeMdTableCell(s.InheritedFrom)
	}
	return fmt.Sprintf("%s | %s | %s", val, locked, inherited)
}

// FormatOutputMarkdown renders MR approval settings as a Markdown table.
// scope should be "Group" or "Project".
func FormatOutputMarkdown(out Output, scope string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s MR Approval Settings\n\n", scope)
	sb.WriteString("| Setting | Value | Locked | Inherited From |\n")
	sb.WriteString("| ------- | ----- | ------ | -------------- |\n")
	fmt.Fprintf(&sb, "| Allow author approval | %s |\n", formatSetting(out.AllowAuthorApproval))
	fmt.Fprintf(&sb, "| Allow committer approval | %s |\n", formatSetting(out.AllowCommitterApproval))
	fmt.Fprintf(&sb, "| Allow approver list overrides | %s |\n", formatSetting(out.AllowOverridesToApproverListPerMergeRequest))
	fmt.Fprintf(&sb, "| Retain approvals on push | %s |\n", formatSetting(out.RetainApprovalsOnPush))
	fmt.Fprintf(&sb, "| Selective code owner removals | %s |\n", formatSetting(out.SelectiveCodeOwnerRemovals))
	fmt.Fprintf(&sb, "| Require password to approve | %s |\n", formatSetting(out.RequirePasswordToApprove))
	fmt.Fprintf(&sb, "| Require reauthentication | %s |\n", formatSetting(out.RequireReauthenticationToApprove))
	toolutil.WriteHints(
		&sb,
		"Use gitlab_update_"+strings.ToLower(scope)+"_mr_approval_settings to change settings",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(func(v Output) string { return FormatOutputMarkdown(v, "") })
}
