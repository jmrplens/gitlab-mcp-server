// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package customattributes

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The next steps a custom-attribute result offers, each naming the canonical
// catalog ID every surface accepts rather than an individual tool name the
// default surface does not register.
var (
	hintSetAttribute    = toolutil.HintAction("admin.custom_attr_set", "add or update an attribute")
	hintUpdateAttribute = toolutil.HintAction("admin.custom_attr_set", "update this attribute")
	hintDeleteAttribute = toolutil.HintAction("admin.custom_attr_delete", "remove it")
	hintVerifyAttribute = toolutil.HintAction("admin.custom_attr_get", "verify the value")
)

// unresolvedOutput is what an action answers when the path or the username in
// resource_id names nothing GitLab could find.
type unresolvedOutput struct {
	ResourceType string `json:"resource_type"`
	Identifier   string `json:"identifier"`
}

// unresolvedAnswers says, for each resource type, what the not-found result
// calls it and where its name can be read from.
var unresolvedAnswers = map[string]struct {
	label string
	hints []string
}{
	resourceUser: {"User", []string{
		"Use user.list to search users by username or email",
		"Pass the user's numeric ID as resource_id if the username has changed",
	}},
	resourceGroup: {"Group", []string{
		"Use group.list to find the group's full path",
		"Pass the group's numeric ID as resource_id if the group has moved",
	}},
	resourceProject: {"Project", []string{
		"Use project.list to find the project's full path",
		"Pass the project's numeric ID as resource_id if the project has moved",
	}},
}

// formatUnresolved renders the not-found result of a name nothing answers to.
func formatUnresolved(out unresolvedOutput) *mcp.CallToolResult {
	answer := unresolvedAnswers[out.ResourceType]
	return toolutil.NotFoundResult(answer.label, out.Identifier, answer.hints...)
}

// FormatListMarkdown renders custom attributes as the collection they are: one
// table row per key and value, with both escaped, since an attribute is
// written by whoever administers the instance and its key is as free-form as
// its value.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Attributes) == 0 {
		return toolutil.EmptyMessage("custom attributes")
	}
	var b strings.Builder
	var pagination toolutil.PaginationOutput
	toolutil.WriteListHeading(&b, "Custom Attributes", len(out.Attributes), pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Key", "Value"))
	for _, a := range out.Attributes {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(a.Key),
			toolutil.EscapeMdTableCell(a.Value),
		))
	}
	toolutil.WriteListFooter(&b, pagination, false, hintSetAttribute)
	return b.String()
}

// FormatGetMarkdown renders one custom attribute as a card. The two values
// used to be written as bullet-less "**Key**: …" lines, which a Markdown
// reader runs together into one paragraph, and neither was escaped.
func FormatGetMarkdown(out GetOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "Custom Attribute")
	writeAttributeRows(card, out.AttributeItem)
	card.End(hintUpdateAttribute, hintDeleteAttribute)
	return b.String()
}

// FormatSetMarkdown renders the attribute the set action wrote: a set returns
// the object, so it is the same card its get counterpart renders.
func FormatSetMarkdown(out SetOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "Custom Attribute Set")
	writeAttributeRows(card, out.AttributeItem)
	card.End(hintVerifyAttribute)
	return b.String()
}

// writeAttributeRows writes the two rows an attribute is, shared by the get
// and set cards so the two cannot drift apart.
func writeAttributeRows(card *toolutil.Card, a AttributeItem) {
	card.Field("Key", a.Key)
	card.Field("Value", a.Value)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatSetMarkdown)
	toolutil.RegisterMarkdownResult(formatUnresolved)
}
