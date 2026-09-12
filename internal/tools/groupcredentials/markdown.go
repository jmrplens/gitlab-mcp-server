package groupcredentials

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs the hints name. The credential actions are
// routes on the group catalog group, so their domain is "group".
const (
	actionListPATs     = "group.credential_list_pats"
	actionRevokePAT    = "group.credential_revoke_pat"
	actionListSSHKeys  = "group.credential_list_ssh_keys"
	actionDeleteSSHKey = "group.credential_delete_ssh_key"

	// The two labels a card row and a table column both carry.
	labelUserID    = "User ID"
	labelExpiresAt = "Expires At"
)

// FormatPATMarkdown renders one enterprise personal access token as a card.
//
// Revoked is written as a warning rather than as a flag: a tick against
// "Revoked" reads as success, which is the opposite of what it says.
func FormatPATMarkdown(out PATOutput) string {
	var b strings.Builder
	// A token name is free text whoever created the token typed.
	c := toolutil.NewCard(&b, fmt.Sprintf("Personal Access Token: %s (ID: %d)", out.Name, out.ID))
	c.Int("ID", out.ID)
	c.Int(labelUserID, out.UserID)
	c.Field("Description", out.Description)
	c.Bool("Active", out.Active)
	c.Warn("Revoked", out.Revoked)
	c.Field("Scopes", strings.Join(out.Scopes, ", "))
	c.Time(labelExpiresAt, out.ExpiresAt)
	c.Time("Created", out.CreatedAt)
	c.Time("Last Used", out.LastUsedAt)
	c.End(
		toolutil.HintAction(actionListPATs, "see the group's other tokens"),
		toolutil.HintAction(actionRevokePAT, "revoke this token"),
	)
	return b.String()
}

// FormatPATListMarkdown renders a page of enterprise personal access tokens as
// a Markdown table.
//
// The hints used to be written between the heading and the table header, which
// left the header lazily continuing the guidance list: no table rendered at
// all, and ExtractHints found no section to read.
func FormatPATListMarkdown(out PATListOutput) string {
	if len(out.Tokens) == 0 {
		return toolutil.EmptyMessage("personal access tokens")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Personal Access Tokens", len(out.Tokens), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", labelUserID, "Active", "Revoked", "Scopes", labelExpiresAt))
	for _, t := range out.Tokens {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Name),
			strconv.FormatInt(t.UserID, 10),
			toolutil.BoolEmoji(t.Active),
			toolutil.BoolEmoji(t.Revoked),
			toolutil.EscapeMdTableCell(strings.Join(t.Scopes, ", ")),
			toolutil.FormatTime(t.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionRevokePAT, "revoke one of these tokens"),
	)
	return b.String()
}

// FormatSSHKeyMarkdown renders one enterprise SSH key as a card.
func FormatSSHKeyMarkdown(out SSHKeyOutput) string {
	var b strings.Builder
	// An SSH key title is the name its owner gave it, with GitLab falling back
	// to the key's own comment field, which the owner also wrote.
	c := toolutil.NewCard(&b, fmt.Sprintf("SSH Key: %s (ID: %d)", out.Title, out.ID))
	c.Int("ID", out.ID)
	c.Int(labelUserID, out.UserID)
	c.Field("Usage Type", out.UsageType)
	c.Time("Created", out.CreatedAt)
	c.Time(labelExpiresAt, out.ExpiresAt)
	c.Time("Last Used", out.LastUsedAt)
	c.End(
		toolutil.HintAction(actionListSSHKeys, "see the group's other keys"),
		toolutil.HintAction(actionDeleteSSHKey, "delete this key"),
	)
	return b.String()
}

// FormatSSHKeyListMarkdown renders a page of enterprise SSH keys as a Markdown
// table.
func FormatSSHKeyListMarkdown(out SSHKeyListOutput) string {
	if len(out.Keys) == 0 {
		return toolutil.EmptyMessage("SSH keys")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "SSH Keys", len(out.Keys), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Title", labelUserID, "Created", labelExpiresAt))
	for _, k := range out.Keys {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(k.ID, 10),
			toolutil.EscapeMdTableCell(k.Title),
			strconv.FormatInt(k.UserID, 10),
			toolutil.FormatTime(k.CreatedAt),
			toolutil.FormatTime(k.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionDeleteSSHKey, "delete one of these keys"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatPATMarkdown)
	toolutil.RegisterMarkdown(FormatPATListMarkdown)
	toolutil.RegisterMarkdown(FormatSSHKeyMarkdown)
	toolutil.RegisterMarkdown(FormatSSHKeyListMarkdown)
}
