// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintGraphStatus is the next step the status card ends on, whichever of its
// two shapes it takes: cluster health says nothing about one namespace.
const hintGraphStatus = "inspect indexing status for a namespace or project"

// orbitNotFoundOutput is returned by MCP Orbit tools when the requested resource or feature is not found (HTTP 404).
// Used to provide actionable hints for missing Orbit endpoints or disabled features.
type orbitNotFoundOutput struct {
	Resource   string
	Identifier string
}

// init registers all Markdown formatters for Orbit MCP tool outputs.
//
// Each formatter converts a tool output struct into a Markdown summary
// suitable for both LLM and user-facing documentation. The formatter
// for [orbitNotFoundOutput] produces the standard 404 guidance.
func init() {
	toolutil.RegisterMarkdownResult(formatOrbitNotFound)
	toolutil.RegisterMarkdown[StatusOutput](FormatStatusMarkdown)
	toolutil.RegisterMarkdown[SchemaOutput](FormatSchemaMarkdown)
	toolutil.RegisterMarkdown[ToolsOutput](FormatToolsMarkdown)
	toolutil.RegisterMarkdown[DSLOutput](FormatDSLMarkdown)
	toolutil.RegisterMarkdown[QueryOutput](FormatQueryMarkdown)
	toolutil.RegisterMarkdown[GraphStatusOutput](FormatGraphStatusMarkdown)
}

// formatOrbitNotFound returns a [*mcp.CallToolResult] with actionable
// hints when an Orbit resource is not found. Used by all Orbit MCP
// tool handlers to provide LLM-friendly output for HTTP 404.
func formatOrbitNotFound(out orbitNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		out.Resource, out.Identifier,
		"Verify GitLab Orbit is enabled on GitLab.com for the requested token",
		"Check that the token can access a Knowledge Graph-enabled namespace or project",
	)
}

// FormatStatusMarkdown renders the health of the Orbit service as the card of
// one object: the overall state and version, then the components as a nested
// collection. A user Orbit is not available to never reaches the card: GitLab
// answers that user's status with null, which [Status] reports as an error.
func FormatStatusMarkdown(out StatusOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Status")

	if out.Status == "" && out.Version == "" && len(out.Components) == 0 {
		c.Note("No Orbit status data returned.")
		c.End(toolutil.HintAction(orbitActionGraphStatus, hintGraphStatus))
		return b.String()
	}

	c.Field("Status", out.Status)
	c.Field("Version", out.Version)

	if len(out.Components) > 0 {
		table := c.Table("Components", "Component", "Status")
		for _, component := range out.Components {
			// Both come back from GitLab's GraphQL API as a free string and an
			// enum this repository does not constrain.
			table.Row(
				toolutil.EscapeMdTableCell(component.Name),
				toolutil.EscapeMdTableCell(component.Status),
			)
		}
	}

	c.End(toolutil.HintAction(orbitActionGraphStatus, hintGraphStatus))
	return b.String()
}

// FormatSchemaMarkdown renders the Knowledge Graph schema as the card of one
// object: the three type counts, then the domains as a nested collection. The
// compact text GitLab answers the llm format with is the whole response, so it
// is rendered as the card's body instead.
//
// Its next steps lead to the query language and then to a query. They used to
// lead to orbit.tools for "the live query and tool manifest", which is the
// manifest of the MCP tools Orbit serves (list_commands and invoke_command on
// GitLab.com, read 2026-10-06) and describes no query: the grammar is what
// orbit.dsl returns, read from the query_graph command since issue 1414.
//
// The edge types and the nodes GitLab answered expanded are written out too
// (issue 1311). The card used to stop at the counts and the domains, so the
// relationship names and the properties a query is built from were in the
// structured output alone, and a client that hands the model the text, as
// most do on the default surface, left it to guess them while the last step
// spoke of "the node and edge types listed here". A schema with nodes and
// none of them expanded therefore adds a step saying how to ask for their
// properties.
func FormatSchemaMarkdown(out SchemaOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Schema")
	dslHint := toolutil.HintAction(orbitActionDSL, "read the grammar a query is written in")
	queryHint := toolutil.HintAction(orbitActionQuery, "run a query naming the node and edge types listed here")
	if out.FormattedText != "" {
		c.Fence("", "text", out.FormattedText)
		c.End(dslHint, queryHint)
		return b.String()
	}
	c.Int("Domains", int64(len(out.Domains)))
	c.Int("Nodes", int64(len(out.Nodes)))
	c.Int("Edges", int64(len(out.Edges)))
	if len(out.Domains) > 0 {
		table := c.Table("Domains", "Domain", "Nodes")
		for _, domain := range out.Domains {
			table.Row(
				toolutil.EscapeMdTableCell(domain.Name),
				toolutil.EscapeMdTableCell(strings.Join(domain.NodeNames, ", ")),
			)
		}
	}
	if len(out.Edges) > 0 {
		table := c.Table("Edge types", "Edge", "Source -> target", "Description")
		for _, edge := range out.Edges {
			table.Row(
				toolutil.EscapeMdTableCell(edge.Name),
				toolutil.EscapeMdTableCell(edgeVariants(edge.Variants)),
				toolutil.EscapeMdTableCell(edge.Description),
			)
		}
	}
	expanded := expandedNodes(out.Nodes)
	for _, node := range expanded {
		writeSchemaNode(c, node)
	}
	hints := []string{dslHint}
	if len(out.Nodes) > 0 && len(expanded) == 0 {
		hints = append(hints, toolutil.HintAction(orbitActionSchema,
			"read the properties of the node types a query reads: name them in expand, or set format=llm for the schema as compact text"))
	}
	c.End(append(hints, queryHint)...)
	return b.String()
}

// expandedNodes returns the nodes the schema carries properties for, which
// are the ones named in expand, in the order GitLab sent them.
func expandedNodes(nodes []SchemaNode) []SchemaNode {
	var expanded []SchemaNode
	for _, node := range nodes {
		if len(node.Properties) > 0 {
			expanded = append(expanded, node)
		}
	}
	return expanded
}

// writeSchemaNode writes one expanded node as a section of the schema card:
// its domain, description, key and label as rows, and its properties as a
// nested collection.
func writeSchemaNode(c *toolutil.Card, node SchemaNode) {
	section := c.Section("Node " + node.Name)
	section.Field("Domain", node.Domain)
	section.Field("Description", node.Description)
	section.Field("Primary key", node.PrimaryKey)
	section.Field("Label field", node.LabelField)
	table := section.Table("Properties", "Property", "Type", "Nullable", "Values")
	for _, property := range node.Properties {
		table.Row(
			toolutil.EscapeMdTableCell(property.Name),
			toolutil.EscapeMdTableCell(property.DataType),
			toolutil.BoolEmoji(property.Nullable),
			toolutil.EscapeMdTableCell(strings.Join(property.EnumValues, ", ")),
		)
	}
}

// edgeVariants renders the source and target pairs an edge type joins.
func edgeVariants(variants []SchemaEdgeVariant) string {
	pairs := make([]string, 0, len(variants))
	for _, variant := range variants {
		pairs = append(pairs, variant.SourceType+" -> "+variant.TargetType)
	}
	return strings.Join(pairs, ", ")
}

// FormatToolsMarkdown renders the Orbit tool manifest as a table: a collection
// of objects that share columns.
//
// The name goes in a code span through the cell form of the span writer. It
// used to be stripped of every backtick it held and then run through the cell
// escaper inside a hand-written span, which showed "&#124;" as those five
// characters and quietly deleted part of the name.
//
// The tools listed are Orbit's own, which a query sent through orbit.query
// does not use, so the next steps lead to the grammar and the names a query
// is built from rather than to the parameters these tools declare.
func FormatToolsMarkdown(out ToolsOutput) string {
	if len(out.Tools) == 0 {
		return toolutil.EmptyMessage("Orbit tools")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Orbit Tools", len(out.Tools), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Tool", "Description"))
	for _, tool := range out.Tools {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdCodeSpanCell(tool.Name),
			toolutil.EscapeMdTableCell(tool.Description),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(orbitActionDSL, "read the grammar a graph query is written in"),
		toolutil.HintAction(orbitActionSchema, "read the node and edge names a graph query names"))
	return b.String()
}

// FormatDSLMarkdown renders the Orbit query language as the card of one object:
// the command a query runs as, its grammar and the JSON Schema of its input,
// each inside a fence sized to it, or the compact text GitLab answers the llm
// format with as the whole body.
//
// The grammar names no entity, so every shape of the card ends on the action
// that lists them, and the card says so when GitLab described no query command
// at all, which is also what a caller is told while the route this action read
// until issue 1414, GET /orbit/schema/dsl, answers 404.
func FormatDSLMarkdown(out DSLOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Query Language")
	schemaHint := toolutil.HintAction(orbitActionSchema, "read the entities, properties and relationship types a query names")
	queryHint := toolutil.HintAction(orbitActionQuery, "run a query written in this language, sent as text")
	if out.FormattedText != "" {
		c.Fence("", "text", out.FormattedText)
		c.End(queryHint, schemaHint)
		return b.String()
	}
	if out.Name == "" && out.Description == "" && out.Parameters == nil {
		c.Note("GitLab described no query command, so no query language was returned.")
		c.End(schemaHint)
		return b.String()
	}
	c.Field("Command", out.Name)
	if out.Description != "" {
		c.Fence("Grammar", "text", out.Description)
	}
	if out.Parameters != nil {
		c.Fence("Input schema", "json", prettyAny(out.Parameters))
	}
	c.End(queryHint, schemaHint)
	return b.String()
}

// FormatQueryMarkdown renders one query result as the card of one object: what
// was asked and how much came back, then the query text and the rows as fenced
// bodies.
func FormatQueryMarkdown(out QueryOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Query Result")
	hints := []string{
		toolutil.HintAction(orbitActionGraphStatus, "check indexing when a result looks stale or incomplete"),
		toolutil.HintAction(orbitActionDSL, "read the grammar the next query is written in"),
	}

	if out.FormattedText != "" {
		c.Fence("", "text", out.FormattedText)
		c.End(hints...)
		return b.String()
	}

	c.Field("Query type", out.QueryType)
	c.Count("Row count", out.RowCount)
	if len(out.RawQueryStrings) > 0 {
		section := c.Section("Raw Query Strings")
		for _, raw := range out.RawQueryStrings {
			section.Fence("", "text", raw)
		}
	}
	if out.Result != nil {
		c.Fence("Result", "json", prettyAny(out.Result))
	}
	c.End(hints...)
	return b.String()
}

// hintAccessGate is the one next step of a group GitLab.com serves no index
// for: a query would be refused the same way, and the cluster's health says
// nothing about one group's access.
const hintAccessGate = "Orbit access is set on the top-level group on GitLab.com: a license or a running trial there serves the index, which this status shows once it does"

// FormatGraphStatusMarkdown renders Orbit indexing status as the card of one
// object: whether Orbit is on, the top-level group's Orbit access and trial,
// how much is indexed, how the last run went, and the per-domain node counts
// as a nested collection.
//
// Since Orbit's general availability GitLab.com serves a group's index only
// under LICENSED or TRIAL_ACTIVE access, and answers a top-level group under
// any other with its access and no index. The card then says so in a line of
// its own ([graphStatusAccessNote]), since a missing or empty index reads as
// an empty graph to a model that has not read the access row, and its next
// step leads to the access rather than to a query GitLab would refuse.
func FormatGraphStatusMarkdown(out GraphStatusOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Graph Status")
	hints := []string{
		toolutil.HintAction(orbitActionQuery, "query the graph once indexing reaches a healthy state"),
		toolutil.HintAction(orbitActionStatus, "check the cluster itself when indexing never starts"),
	}
	if out.Access != nil && !out.Access.Serves() {
		hints = []string{hintAccessGate}
	}

	if out.Enabled != nil {
		c.Bool("Orbit enabled", *out.Enabled)
	}
	c.Time("Enabled at", out.EnabledAt)
	writeGraphStatusAccess(c, out.Access, out.Trial)
	if out.Projects != nil {
		c.Int("Indexed projects", out.Projects.Indexed)
		c.Int("Total known projects", out.Projects.TotalKnown)
		c.Int("Projects not indexed", out.Projects.NotIndexed)
		c.Int("Projects out of indexing attempts", out.Projects.Gaps)
	}
	if out.Indexing != nil {
		c.Field("Indexing state", out.Indexing.State)
		c.Time("Last completed at", out.Indexing.LastCompletedAt)
		if len(out.Indexing.Areas) > 0 {
			table := c.Table("Areas", "Area", "State")
			for _, area := range out.Indexing.Areas {
				table.Row(toolutil.EscapeMdTableCell(area.Name), toolutil.EscapeMdTableCell(area.State))
			}
		}
	}
	if len(out.Domains) > 0 {
		table := c.Table("Domains", "Domain", "Counts")
		for _, domain := range out.Domains {
			table.Row(
				toolutil.EscapeMdTableCell(domain.Name),
				toolutil.EscapeMdTableCell(domainCounts(domain.Items)),
			)
		}
	}
	c.Note(graphStatusAccessNote(out))
	c.End(hints...)
	return b.String()
}

// writeGraphStatusAccess writes the top-level group's Orbit access as rows and
// its trial as a nested object, each as GitLab spelled it, and nothing for a
// subgroup or a project, for which GitLab answers both null.
func writeGraphStatusAccess(c *toolutil.Card, access *GraphStatusAccess, trial *GraphStatusTrial) {
	if access != nil {
		c.Field("Orbit access", access.State)
		c.Field("Access reason", access.Reason)
		c.Field("Turn-on denial reason", access.TurnOnDenialReason)
	}
	if trial == nil {
		return
	}
	sub := c.Sub("Orbit trial")
	sub.Field("State", trial.State)
	sub.Time("Expires on", trial.ExpiresOn)
	if trial.Credits != nil {
		sub.Field("Credits remaining", formatCredits(trial.Credits.Remaining))
		sub.Field("Credits total", formatCredits(trial.Credits.Total))
	}
	if trial.Offer != nil {
		if trial.Offer.DurationDays != nil {
			sub.Int("Offered trial days", *trial.Offer.DurationDays)
		}
		sub.Field("Offered trial credits", formatCredits(trial.Offer.InitialCredits))
	}
}

// formatCredits renders a credit count GitLab sends as a Float, with no
// trailing zeros, or nothing when GitLab sent null. Zero is written, since a
// trial with no credits left is an answer.
func formatCredits(credits *float64) string {
	if credits == nil {
		return ""
	}
	return strconv.FormatFloat(*credits, 'f', -1, 64)
}

// graphStatusAccessNote is the line the card ends on when the top-level group's
// Orbit access explains its index: a state GitLab.com serves no index under, a
// served state with no index answered, or a trial, which ends. It is empty for
// a licensed group with its index, and for a subgroup or a project, which carry
// no access.
func graphStatusAccessNote(out GraphStatusOutput) string {
	access := out.Access
	switch {
	case access == nil:
		return ""
	case !access.Serves():
		return "Orbit access is " + toolutil.EscapeMdTableCell(access.State) +
			": GitLab.com serves a group's Orbit index and queries only while its access is LICENSED or TRIAL_ACTIVE, " +
			"so an index missing or empty here reflects the access, not the graph."
	case out.Indexing == nil && out.Projects == nil && len(out.Domains) == 0:
		return "The group has Orbit access and GitLab answered no index: Orbit is off for the top-level group, or the token is below the Reporter role there."
	case access.State == AccessTrialActive:
		return "Orbit is served on a trial, which ends on its last day or when its credits run out."
	}
	return ""
}

// domainCounts renders one domain's node counts as the "Name: count" list the
// cell shows.
func domainCounts(items []GraphStatusDomainItem) string {
	counts := make([]string, 0, len(items))
	for _, item := range items {
		counts = append(counts, item.Name+": "+strconv.FormatInt(item.Count, 10))
	}
	return strings.Join(counts, ", ")
}

// prettyAny returns a pretty-printed JSON string for any value, or
// the result of [fmt.Sprint] when encoding fails. Used to render
// Orbit query results in Markdown.
func prettyAny(value any) string {
	buf, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(buf)
}
