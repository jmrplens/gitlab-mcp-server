package auditevents

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown renders a single audit event as the card of one object.
//
// Every field the details carry is written, not just the five the card used to
// show: what a particular audit event says about itself lives in the singular
// detail fields (with, as, add, remove, change, from, to, the custom message,
// the failed login), and dropping them left a reader with an event name and
// nothing about what changed.
func FormatMarkdown(e Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Audit Event #%d", e.ID))
	c.Int("ID", e.ID)
	c.Field("Event Name", e.EventName)
	c.Field("Detail Event Name", e.Details.EventName)
	c.Field("Entity Type", e.EntityType)
	c.Int("Entity ID", e.EntityID)
	c.Field("Entity Path", e.Details.EntityPath)
	c.Time("Created", e.CreatedAt)
	c.Int("Author ID", e.AuthorID)
	c.Field("Author Name", e.Details.AuthorName)
	c.Field("Author Email", e.Details.AuthorEmail)
	c.Field("Author Class", e.Details.AuthorClass)
	c.Field("Target Type", e.Details.TargetType)
	c.Field("Target ID", e.Details.TargetID)
	c.Field("Target Details", e.Details.TargetDetails)
	// GitLab records the address the request arrived from, so it is an IP
	// literal, and a code span is where a value a reader copies belongs.
	c.Code("IP Address", e.Details.IPAddress)
	c.Field("Failed Login", e.Details.FailedLogin)
	c.Field("With", e.Details.With)
	c.Field("As", e.Details.As)
	c.Field("Add", e.Details.Add)
	c.Field("Remove", e.Details.Remove)
	c.Field("Change", e.Details.Change)
	c.Field("From", e.Details.From)
	c.Field("To", e.Details.To)
	c.Text("Custom Message", e.Details.CustomMessage)
	writeChanges(c, e.Details.Changes)
	writeChangeObject(c, e.Details.ChangeObject)
	c.End(
		toolutil.HintAction(actionListProject, "browse a project's audit events"),
		toolutil.HintAction(actionListGroup, "browse a group's audit events"),
		toolutil.HintAction(actionListInstance, "browse the instance's audit events"),
	)
	return b.String()
}

// writeChanges renders the plural changes array as the nested collection it
// is, and nothing when the event carries none.
func writeChanges(c *toolutil.Card, changes []ChangeEntry) {
	if len(changes) == 0 {
		return
	}
	table := c.Table("Changes", "Change", "From", "To")
	for _, change := range changes {
		table.Row(
			toolutil.EscapeMdTableCell(change.Change),
			toolutil.EscapeMdTableCell(change.From),
			toolutil.EscapeMdTableCell(change.To),
		)
	}
}

// writeChangeObject renders an object-valued change as JSON inside a fence
// sized to the document: the object echoes whatever the audited change
// carried, and JSON escaping leaves a backtick alone.
func writeChangeObject(c *toolutil.Card, object any) {
	if object == nil {
		return
	}
	raw, err := json.Marshal(object)
	if err != nil {
		return
	}
	c.Fence("Change (object)", "json", string(raw))
}

// FormatListMarkdown renders a page of audit events as a Markdown table: a
// collection of objects that share columns.
func FormatListMarkdown(out ListOutput) string {
	if len(out.AuditEvents) == 0 {
		return toolutil.EmptyMessage("audit events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Audit Events", len(out.AuditEvents), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Event Name", "Entity Type", "Entity ID", "Author ID", "Created"))
	for _, e := range out.AuditEvents {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.EventName),
			toolutil.EscapeMdTableCell(e.EntityType),
			strconv.FormatInt(e.EntityID, 10),
			strconv.FormatInt(e.AuthorID, 10),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionGetProject, "read one project event in full, details included"),
		toolutil.HintAction(actionGetGroup, "read one group event in full"),
		toolutil.HintAction(actionGetInstance, "read one instance event in full"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
