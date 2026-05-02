package mrapprovalsettings

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func formatSetting(s SettingOutput) string {
	val := toolutil.BoolEmoji(s.Value)
	locked := toolutil.BoolEmoji(s.Locked)
	inherited := s.InheritedFrom
	if inherited == "" {
		inherited = "—"
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
	toolutil.WriteHints(&sb,
		"Use gitlab_update_"+strings.ToLower(scope)+"_mr_approval_settings to change settings",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(func(v Output) string { return FormatOutputMarkdown(v, "") })
}
