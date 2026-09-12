package securityattributes

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionAttributeUpdate        = "security_attribute.update"
	actionAttributeProjectUpdate = "security_attribute.project_update"
	actionAttributeBulkUpdate    = "security_attribute.bulk_update"
	actionCategoryCreate         = "security_category.create"
)

// editableStateLocked is the SecurityCategoryEditableState a template-provided
// attribute carries. GitLab refuses to change such an attribute, so the card
// does not offer to.
const editableStateLocked = "LOCKED"

// FormatOutputMarkdown renders a security attribute as the card of one object:
// its identity and color, how much of it GitLab allows changing, and the
// category it belongs to as a nested object.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, attributeHeading(out.Name))
	writeAttributeRows(c, out)
	c.End(attributeHints(out.EditableState)...)
	return b.String()
}

// FormatCreateMarkdown renders the attributes one create request produced as a
// collection: several objects sharing columns are a table, not a card.
func FormatCreateMarkdown(out CreateOutput) string {
	if len(out.Attributes) == 0 {
		return toolutil.EmptyMessage("security attributes")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Security Attributes Created", len(out.Attributes), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Color", "Description", "Category", "Editable state"))
	for _, attribute := range out.Attributes {
		category := ""
		if attribute.SecurityCategory != nil {
			category = attribute.SecurityCategory.Name
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(attribute.ID, 10),
			toolutil.EscapeMdTableCell(attribute.Name),
			toolutil.MdCodeSpanCell(attribute.Color),
			toolutil.EscapeMdTableCell(attribute.Description),
			toolutil.EscapeMdTableCell(category),
			toolutil.MdCodeSpanCell(attribute.EditableState),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionAttributeProjectUpdate, "apply these attributes to a project"),
		toolutil.HintAction(actionAttributeBulkUpdate, "apply them to many groups or projects at once"),
	)
	return b.String()
}

// FormatProjectUpdateMarkdown renders what one project's attribute change did,
// as the card of one result. Both counts are written at zero: "nothing was
// added" is the answer to a request that asked for a removal.
func FormatProjectUpdateMarkdown(out ProjectUpdateOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project Security Attributes Updated")
	c.Int("Added", out.AddedCount)
	c.Int("Removed", out.RemovedCount)
	c.End(
		toolutil.HintAction(actionAttributeBulkUpdate, "apply the same change to many groups or projects at once"),
		toolutil.HintAction(actionAttributeProjectUpdate, "change this project's attributes again"),
	)
	return b.String()
}

// FormatBulkUpdateMarkdown renders what one bulk attribute change did, as the
// card of one result: the mode it ran in and the identifiers it named, each
// written as the list of numbers it is rather than as Go's container syntax.
func FormatBulkUpdateMarkdown(out BulkUpdateOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Security Attributes Updated in Bulk")
	c.Field("Status", out.Status)
	c.Field("Message", out.Message)
	c.Field("Mode", string(out.Mode))
	c.Field("Attributes", joinIDs(out.AttributeIDs))
	c.Field("Groups", joinIDs(out.GroupIDs))
	c.Field("Projects", joinIDs(out.ProjectIDs))
	c.End(
		toolutil.HintAction(actionAttributeProjectUpdate, "change one project's attributes instead"),
		toolutil.HintAction(actionCategoryCreate, "add a category to classify further"),
	)
	return b.String()
}

// writeAttributeRows writes the rows of one attribute onto a card, so the
// detail view and anything that extends it cannot drift apart.
func writeAttributeRows(c *toolutil.Card, out Output) {
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Code("Color", out.Color)
	c.Text("Description", out.Description)
	c.Code("Editable state", out.EditableState)
	if out.SecurityCategory != nil {
		category := c.Sub("Category")
		category.Int("ID", out.SecurityCategory.ID)
		category.Field("Name", out.SecurityCategory.Name)
		category.Text("Description", out.SecurityCategory.Description)
		category.Bool("Multiple selection", out.SecurityCategory.MultipleSelection)
		category.Code("Editable state", out.SecurityCategory.EditableState)
		category.Code("Template type", out.SecurityCategory.TemplateType)
	}
}

// attributeHeading names the attribute in the card's heading, or opens the
// generic one when GitLab sent no name.
func attributeHeading(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Security Attribute"
	}
	return "Security Attribute: " + name
}

// attributeHints returns the next steps this attribute's editable state
// allows. A locked attribute is one a template provides: it can still be
// applied to a project, and offering to edit it names a call GitLab refuses.
// An unset or unknown state offers everything and lets GitLab answer.
func attributeHints(state string) []string {
	var hints []string
	if strings.ToUpper(strings.TrimSpace(state)) != editableStateLocked {
		hints = append(hints, toolutil.HintAction(actionAttributeUpdate, "rename, re-describe or recolor this attribute"))
	}
	return append(hints,
		toolutil.HintAction(actionAttributeProjectUpdate, "apply this attribute to a project"),
		toolutil.HintAction(actionAttributeBulkUpdate, "apply it to many groups or projects at once"),
	)
}

// joinIDs renders a slice of identifiers as the comma-separated list a reader
// can act on. Go's own "[9 10]" is container syntax, not a value GitLab has or
// any call accepts, and it is what "%v" on the slice used to print.
func joinIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ", ")
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatCreateMarkdown)
	toolutil.RegisterMarkdown(FormatProjectUpdateMarkdown)
	toolutil.RegisterMarkdown(FormatBulkUpdateMarkdown)
}
