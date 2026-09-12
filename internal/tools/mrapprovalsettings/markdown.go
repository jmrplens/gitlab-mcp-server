package mrapprovalsettings

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name. Both update actions are named when the
// formatter does not know which scope it is rendering, because the registry
// dispatches on the Go type and the group and project handlers answer with the
// same one.
const (
	actionGroupUpdate   = "merge_request.approval_settings_group_update"
	actionProjectUpdate = "merge_request.approval_settings_project_update"
)

// settingCells renders one setting as the three cells that follow its name: the
// value, whether a parent locked it, and the group it was inherited from.
//
// The escaping goes on the one value that is not this package's own: the name
// of the group a setting was inherited from.
func settingCells(s SettingOutput) []string {
	inherited := "-"
	if s.InheritedFrom != "" {
		inherited = toolutil.EscapeMdTableCell(s.InheritedFrom)
	}
	return []string{toolutil.BoolEmoji(s.Value), toolutil.BoolEmoji(s.Locked), inherited}
}

// FormatOutputMarkdown renders merge request approval settings: the settings
// are a collection of objects sharing columns, so they are a table under the
// card's heading.
//
// scope is "Group" or "Project" when the caller knows which of the two routes
// answered, and empty when it does not. The registry dispatches on the Go type
// and both routes answer with [Output], so the registered formatter passes
// none: an empty scope used to be interpolated into the heading and into a tool
// name, giving "##  MR Approval Settings" and a hint naming
// gitlab_update__mr_approval_settings, which no surface registers.
func FormatOutputMarkdown(out Output, scope string) string {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, strings.TrimSpace(scope+" MR Approval Settings"))
	t := c.Table("", "Setting", "Value", "Locked", "Inherited From")
	t.Row(append([]string{"Allow author approval"}, settingCells(out.AllowAuthorApproval)...)...)
	t.Row(append([]string{"Allow committer approval"}, settingCells(out.AllowCommitterApproval)...)...)
	t.Row(append([]string{"Allow approver list overrides"}, settingCells(out.AllowOverridesToApproverListPerMergeRequest)...)...)
	t.Row(append([]string{"Retain approvals on push"}, settingCells(out.RetainApprovalsOnPush)...)...)
	t.Row(append([]string{"Selective code owner removals"}, settingCells(out.SelectiveCodeOwnerRemovals)...)...)
	t.Row(append([]string{"Require password to approve"}, settingCells(out.RequirePasswordToApprove)...)...)
	t.Row(append([]string{"Require reauthentication"}, settingCells(out.RequireReauthenticationToApprove)...)...)
	c.End(updateHints(scope)...)
	return sb.String()
}

// updateHints names the update action for the scope that answered, or both when
// the scope is unknown.
func updateHints(scope string) []string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "group":
		return []string{toolutil.HintAction(actionGroupUpdate, "change these group settings")}
	case "project":
		return []string{toolutil.HintAction(actionProjectUpdate, "change these project settings")}
	default:
		return []string{
			toolutil.HintAction(actionGroupUpdate, "change a group's approval settings"),
			toolutil.HintAction(actionProjectUpdate, "change a project's approval settings"),
		}
	}
}

func init() {
	toolutil.RegisterMarkdown(func(v Output) string { return FormatOutputMarkdown(v, "") })
}
