package orbit

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionStatus      = "orbit.status"
	actionSchema      = "orbit.schema"
	actionTools       = "orbit.tools"
	actionDSL         = "orbit.dsl"
	actionQuery       = "orbit.query"
	actionGraphStatus = "orbit.graph_status"
)

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

// FormatStatusMarkdown renders Orbit cluster health as the card of one object:
// whether the caller reaches the graph at all, the cluster's own health, and
// the subsystems as a nested collection.
//
// Two of those rows were missing, and both are the answer to the question the
// action is asked. "Available to you" is the user-level access flag, which is
// what separates a healthy cluster the caller cannot query from one they can;
// "Error" is what the backend says when it cannot reach the gRPC cluster, which
// is exactly the case where "Status: unknown" alone explains nothing.
func FormatStatusMarkdown(out StatusOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Status")
	facts := readStatus(out)

	if facts.formattedText != "" {
		c.Fence("", "text", facts.formattedText)
		c.End(toolutil.HintAction(actionGraphStatus, "inspect indexing status for a namespace or project"))
		return b.String()
	}

	if out.User == nil && facts.empty() {
		c.Note("No Orbit status data returned.")
		c.End(toolutil.HintAction(actionGraphStatus, "inspect indexing status for a namespace or project"))
		return b.String()
	}

	if out.User != nil {
		c.Bool("Available to you", out.User.Available)
	}
	c.Field("Status", facts.status)
	c.Field("Version", facts.version)
	c.Time("Timestamp", facts.timestamp)
	c.Field("Error", facts.systemError)

	if len(facts.components) > 0 {
		table := c.Table("Components", "Component", "Status", "Replicas")
		for _, component := range facts.components {
			// Both come back from the Knowledge Graph API as free strings, and
			// nothing in this repository constrains either.
			table.Row(
				toolutil.EscapeMdTableCell(component.Name),
				toolutil.EscapeMdTableCell(component.Status),
				replicaCell(component.Replicas),
			)
		}
	}

	c.End(toolutil.HintAction(actionGraphStatus, "inspect indexing status for a namespace or project"))
	return b.String()
}

// statusFacts is the cluster health one status response carried, read from
// whichever of its two shapes carried it.
type statusFacts struct {
	formattedText string
	status        string
	version       string
	timestamp     string
	systemError   string
	components    []StatusComponent
}

// empty reports whether the response said nothing about the cluster at all.
func (f statusFacts) empty() bool {
	return f.status == "" && f.version == "" && f.timestamp == "" && f.systemError == "" && len(f.components) == 0
}

// readStatus reads the cluster health out of the response.
//
// Orbit answers in two shapes: the flat one fills the top-level fields, and the
// nested one leaves them empty and fills System instead. Reading only the flat
// fields rendered "no status data" for every nested answer, including the one
// that carries the backend error.
func readStatus(out StatusOutput) statusFacts {
	facts := statusFacts{
		formattedText: out.FormattedText,
		status:        out.Status,
		version:       out.Version,
		timestamp:     out.Timestamp,
		components:    out.Components,
	}
	if out.System == nil {
		return facts
	}
	facts.systemError = out.System.Error
	if facts.formattedText == "" {
		facts.formattedText = out.System.FormattedText
	}
	if facts.status == "" {
		facts.status = out.System.Status
	}
	if facts.version == "" {
		facts.version = out.System.Version
	}
	if facts.timestamp == "" {
		facts.timestamp = out.System.Timestamp
	}
	if len(facts.components) == 0 {
		facts.components = out.System.Components
	}
	return facts
}

// replicaCell renders a subsystem's ready and desired replica counts, or
// nothing for a stateless subsystem GitLab sends none for.
func replicaCell(replicas *StatusReplicas) string {
	if replicas == nil {
		return ""
	}
	return strconv.FormatInt(replicas.Ready, 10) + "/" + strconv.FormatInt(replicas.Desired, 10)
}

// FormatSchemaMarkdown renders the Knowledge Graph ontology as the card of one
// object: the version and the three type counts, then the domains as a nested
// collection.
func FormatSchemaMarkdown(out SchemaOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Schema")
	c.Field("Schema version", out.SchemaVersion)
	c.Int("Domains", int64(len(out.Domains)))
	c.Int("Nodes", int64(len(out.Nodes)))
	c.Int("Edges", int64(len(out.Edges)))
	if len(out.Domains) > 0 {
		table := c.Table("Domains", "Domain", "Description", "Nodes")
		for _, domain := range out.Domains {
			table.Row(
				toolutil.EscapeMdTableCell(domain.Name),
				toolutil.EscapeMdTableCell(domain.Description),
				toolutil.EscapeMdTableCell(strings.Join(domain.NodeNames, ", ")),
			)
		}
	}
	c.End(
		toolutil.HintAction(actionTools, "inspect the live query and tool manifest"),
		toolutil.HintAction(actionQuery, "run a query once you have chosen a shape from the manifest"),
	)
	return b.String()
}

// FormatToolsMarkdown renders the Orbit tool manifest as a table: a collection
// of objects that share columns.
//
// The name goes in a code span through the cell form of the span writer. It
// used to be stripped of every backtick it held and then run through the cell
// escaper inside a hand-written span, which showed "&#124;" as those five
// characters and quietly deleted part of the name.
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
		toolutil.HintAction(actionQuery, "build a query from the parameters a tool declares"),
		toolutil.HintAction(actionSchema, "read the node and edge names those parameters take"))
	return b.String()
}

// FormatDSLMarkdown renders the Orbit query DSL as the card of one object whose
// body is the grammar inside a fence sized to it.
func FormatDSLMarkdown(out DSLOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit DSL")
	if out.Content == "" {
		c.Note("No Orbit DSL data returned.")
		c.End(toolutil.HintAction(actionSchema, "read the node and edge names a query names"))
		return b.String()
	}
	language := "json"
	if strings.EqualFold(out.ResponseFormat, string(gl.OrbitResponseFormatLLM)) {
		language = "text"
	}
	c.Fence("", language, out.Content)
	c.End(
		toolutil.HintAction(actionQuery, "run a query once you have chosen a shape from the DSL"),
		toolutil.HintAction(actionSchema, "read the node and edge names a query names"),
	)
	return b.String()
}

// FormatQueryMarkdown renders one query result as the card of one object: what
// was asked and how much came back, then the query text and the rows as fenced
// bodies.
func FormatQueryMarkdown(out QueryOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Query Result")
	hints := []string{
		toolutil.HintAction(actionGraphStatus, "check indexing when a result looks stale or incomplete"),
		toolutil.HintAction(actionDSL, "read the grammar the next query is written in"),
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

// FormatGraphStatusMarkdown renders Orbit indexing status as the card of one
// object: how much is indexed, how the last run went, and the per-domain node
// counts as a nested collection.
func FormatGraphStatusMarkdown(out GraphStatusOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Orbit Graph Status")
	hints := []string{
		toolutil.HintAction(actionQuery, "query the graph once indexing reaches a healthy state"),
		toolutil.HintAction(actionStatus, "check the cluster itself when indexing never starts"),
	}

	if out.FormattedText != "" {
		c.Fence("", "text", out.FormattedText)
		c.End(hints...)
		return b.String()
	}

	if out.Projects != nil {
		c.Int("Indexed projects", out.Projects.Indexed)
		c.Int("Total known projects", out.Projects.TotalKnown)
	}
	if out.Indexing != nil {
		c.Field("Indexing state", out.Indexing.State)
		c.Time("Last started at", out.Indexing.LastStartedAt)
		c.Time("Last completed at", out.Indexing.LastCompletedAt)
		c.Count("Last duration (ms)", out.Indexing.LastDurationMs)
		c.Field("Last error", out.Indexing.LastError)
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
	c.End(hints...)
	return b.String()
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
