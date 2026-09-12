package securitycategories

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionCategoryUpdate         = "security_category.update"
	actionAttributeCreate        = "security_attribute.create"
	actionAttributeProjectUpdate = "security_attribute.project_update"
)

// The states GitLab's SecurityCategoryEditableState enum takes. They decide
// what a reader can do next, which is why the hints read them: a LOCKED
// category is one a template provides, and offering to rename it or to add an
// attribute under it names a call GitLab refuses.
const (
	editableStateEditableAttributes = "EDITABLE_ATTRIBUTES"
	editableStateLocked             = "LOCKED"
)

// FormatOutputMarkdown renders a security category as the card of one object:
// its identity, whether a project may carry several of its attributes at once,
// how much of it GitLab allows changing, and its attributes as a collection.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, categoryHeading(out.Name))
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Text("Description", out.Description)
	c.Bool("Multiple selection", out.MultipleSelection)
	c.Code("Editable state", out.EditableState)
	c.Code("Template type", out.TemplateType)
	if len(out.SecurityAttributes) > 0 {
		attributes := c.Table("Attributes", "ID", "Name", "Color", "Description", "Editable state")
		for _, a := range out.SecurityAttributes {
			attributes.Row(
				strconv.FormatInt(a.ID, 10),
				toolutil.EscapeMdTableCell(a.Name),
				toolutil.MdCodeSpanCell(a.Color),
				toolutil.EscapeMdTableCell(a.Description),
				toolutil.MdCodeSpanCell(a.EditableState),
			)
		}
	}
	c.End(categoryHints(out.EditableState)...)
	return b.String()
}

// categoryHeading names the category in the card's heading, or opens the
// generic one when GitLab sent no name.
func categoryHeading(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Security Category"
	}
	return "Security Category: " + name
}

// categoryHints returns the next steps this category's editable state allows,
// in reading order. A locked category can only have its attributes applied
// somewhere; one whose attributes alone are editable can also gain an
// attribute; an editable one can be renamed as well. An unset or unknown state
// offers everything and lets GitLab refuse with its own message, which is what
// a state this server has not heard of means.
func categoryHints(state string) []string {
	var hints []string
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case editableStateLocked:
	case editableStateEditableAttributes:
		hints = append(hints, toolutil.HintAction(actionAttributeCreate, "add an attribute under this category"))
	default:
		hints = append(hints,
			toolutil.HintAction(actionAttributeCreate, "add an attribute under this category"),
			toolutil.HintAction(actionCategoryUpdate, "rename this category or change its description"),
		)
	}
	return append(hints, toolutil.HintAction(actionAttributeProjectUpdate, "apply this category's attributes to a project"))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}
