package errortracking

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionKeyList   = "admin.error_tracking_list"
	actionKeyCreate = "admin.error_tracking_create"
	actionKeyDelete = "admin.error_tracking_delete"
	actionSettings  = "admin.error_tracking_get_settings"
)

// FormatSettingsMarkdown renders a project's error tracking settings as the
// card of one object.
func FormatSettingsMarkdown(out SettingsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Error Tracking Settings")
	c.Bool("Active", out.Active)
	c.Bool("Integrated", out.Integrated)
	c.Field("Project Name", out.ProjectName)
	// Both come from the Sentry integration a maintainer configured.
	c.Field("Sentry URL", out.SentryExternalURL)
	c.Field("API URL", out.APIURL)
	c.End(toolutil.HintAction(actionKeyList, "see the client keys this project publishes"))
	return b.String()
}

// FormatListKeysMarkdown renders a page of error tracking client keys as a
// Markdown table: a collection of objects that share columns.
func FormatListKeysMarkdown(out ListClientKeysOutput) string {
	if len(out.Keys) == 0 {
		return toolutil.EmptyMessage("client keys")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Error Tracking Client Keys", len(out.Keys), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Active", "Public Key"))
	for _, k := range out.Keys {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(k.ID, 10),
			toolutil.BoolEmoji(k.Active),
			// The key GitLab generated, hexadecimal digits, shown as the value
			// a reader copies.
			toolutil.MdCodeSpanCell(k.PublicKey),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		toolutil.HintAction(actionKeyCreate, "generate another key"),
		toolutil.HintAction(actionKeyDelete, "revoke one by its ID"),
	)
	return sb.String()
}

// FormatKeyMarkdown renders a single client key as the card of one object. The
// key and the DSN are code spans: both are values a reader has to copy into a
// client's configuration exactly as GitLab composed them.
func FormatKeyMarkdown(k ClientKeyItem) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Error Tracking Client Key #%d", k.ID))
	c.Int("ID", k.ID)
	c.Bool("Active", k.Active)
	c.Code("Public Key", k.PublicKey)
	c.Code("Sentry DSN", k.SentryDsn)
	c.End(
		toolutil.HintAction(actionKeyDelete, "revoke this key"),
		toolutil.HintAction(actionSettings, "check whether error tracking is enabled for the project"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatSettingsMarkdown)
	toolutil.RegisterMarkdown(FormatListKeysMarkdown)
	toolutil.RegisterMarkdown(FormatKeyMarkdown)
}
