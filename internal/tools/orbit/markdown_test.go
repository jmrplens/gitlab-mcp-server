// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"math"
	"testing"
)

// statusHints is the guidance section every Orbit status card closes with.
const statusHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'orbit.graph_status' to inspect indexing status for a namespace or project\n"

// queryHints is the guidance section every Orbit query card closes with.
const queryHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'orbit.graph_status' to check indexing when a result looks stale or incomplete\n" +
	"- Use action 'orbit.dsl' to read the grammar the next query is written in\n"

// TestFormatStatusMarkdown_FlatShape_RendersTheWholeCard verifies that the flat
// status response renders as a card whose subsystems are a nested collection.
func TestFormatStatusMarkdown_FlatShape_RendersTheWholeCard(t *testing.T) {
	out := StatusOutput{
		Status:  "healthy",
		Version: "0.5.0",
		Components: []StatusComponent{
			{Name: "clickhouse", Status: "healthy", Replicas: &StatusReplicas{Ready: 3, Desired: 3}},
			{Name: "api", Status: "healthy"},
		},
	}

	want := "## Orbit Status\n\n" +
		"- **Status**: healthy\n" +
		"- **Version**: 0.5.0\n\n" +
		"### Components\n\n" +
		"| Component | Status | Replicas |\n| --- | --- | --- |\n" +
		"| clickhouse | healthy | 3/3 |\n" +
		"| api | healthy |  |\n" +
		statusHints

	if got := FormatStatusMarkdown(out); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStatusMarkdown_NestedShape_RendersAvailabilityAndError verifies
// that the nested response shape renders at all, and that it renders the two
// rows that answer the question the action is asked.
//
// Orbit answers in two shapes and the card used to read only the flat one, so
// every nested answer rendered "no status data" even when it carried the
// cluster's health. "Available to you" is the user-level access flag, which
// separates a healthy cluster the caller cannot query from one they can, and
// "Error" is what the backend says when it cannot reach the gRPC cluster, which
// is exactly the case where "Status: unknown" alone explains nothing.
func TestFormatStatusMarkdown_NestedShape_RendersAvailabilityAndError(t *testing.T) {
	out := StatusOutput{
		User: &StatusUser{Available: true},
		System: &StatusSystem{
			Status:    "unknown",
			Version:   "0.5.0",
			Timestamp: "2026-03-20T15:45:00Z",
			Error:     "cannot reach the gRPC cluster",
			Components: []StatusComponent{
				{Name: "clickhouse", Status: "unknown"},
			},
		},
	}

	want := "## Orbit Status\n\n" +
		"- **Available to you**: ✅\n" +
		"- **Status**: unknown\n" +
		"- **Version**: 0.5.0\n" +
		"- **Timestamp**: 20 Mar 2026 15:45 UTC\n" +
		"- **Error**: cannot reach the gRPC cluster\n\n" +
		"### Components\n\n" +
		"| Component | Status | Replicas |\n| --- | --- | --- |\n" +
		"| clickhouse | unknown |  |\n" +
		statusHints

	if got := FormatStatusMarkdown(out); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStatusMarkdown_UserWithoutAccess_SaysSo verifies that a caller the
// Knowledge Graph is not available to reads that as a row rather than as a card
// with nothing in it.
func TestFormatStatusMarkdown_UserWithoutAccess_SaysSo(t *testing.T) {
	want := "## Orbit Status\n\n- **Available to you**: ❌\n" + statusHints

	if got := FormatStatusMarkdown(StatusOutput{User: &StatusUser{}}); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStatusMarkdown_FormattedText_RendersOneFence verifies that the
// pre-formatted body the "llm" response format returns is rendered inside a
// fence and that none of the structured rows is written beside it.
func TestFormatStatusMarkdown_FormattedText_RendersOneFence(t *testing.T) {
	want := "## Orbit Status\n\n```text\nstatus: healthy\n```\n" + statusHints

	if got := FormatStatusMarkdown(StatusOutput{FormattedText: "status: healthy"}); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStatusMarkdown_NestedFormattedText_RendersOneFence verifies that
// the pre-formatted body is found in the nested shape too.
func TestFormatStatusMarkdown_NestedFormattedText_RendersOneFence(t *testing.T) {
	out := StatusOutput{System: &StatusSystem{FormattedText: "status: healthy"}}
	want := "## Orbit Status\n\n```text\nstatus: healthy\n```\n" + statusHints

	if got := FormatStatusMarkdown(out); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStatusMarkdown_NoData_SaysSo verifies that a response carrying
// nothing at all says so in one sentence.
func TestFormatStatusMarkdown_NoData_SaysSo(t *testing.T) {
	want := "## Orbit Status\n\nNo Orbit status data returned.\n" + statusHints

	if got := FormatStatusMarkdown(StatusOutput{}); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStatusMarkdown_OneFieldAlone_RendersItsRow verifies that "no data"
// is said only when every fact is missing: a response carrying one label and
// nothing else renders that one row. The emptiness test joins five facts, and
// a join that ignored any one of them would call a partial answer empty.
func TestFormatStatusMarkdown_OneFieldAlone_RendersItsRow(t *testing.T) {
	tests := []struct {
		name string
		out  StatusOutput
		row  string
	}{
		{name: "status", out: StatusOutput{Status: "degraded"}, row: "- **Status**: degraded\n"},
		{name: "version", out: StatusOutput{Version: "0.5.0"}, row: "- **Version**: 0.5.0\n"},
		{name: "timestamp", out: StatusOutput{Timestamp: "2026-03-20T15:45:00Z"}, row: "- **Timestamp**: 20 Mar 2026 15:45 UTC\n"},
		{name: "error", out: StatusOutput{System: &StatusSystem{Error: "cannot reach the gRPC cluster"}}, row: "- **Error**: cannot reach the gRPC cluster\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "## Orbit Status\n\n" + tt.row + statusHints
			if got := FormatStatusMarkdown(tt.out); got != want {
				t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestFormatSchemaMarkdown_RendersTheWholeCard verifies that the ontology
// renders as a card with the three type counts as rows and the domains as a
// nested collection.
func TestFormatSchemaMarkdown_RendersTheWholeCard(t *testing.T) {
	out := SchemaOutput{
		SchemaVersion: "1.0",
		Domains:       []SchemaDomain{{Name: "core", Description: "Core entities", NodeNames: []string{"User", "Project"}}},
		Nodes:         []any{map[string]any{"name": "User"}},
		Edges:         []SchemaEdge{{Name: "AUTHORED"}},
	}

	want := "## Orbit Schema\n\n" +
		"- **Schema version**: 1.0\n" +
		"- **Domains**: 1\n" +
		"- **Nodes**: 1\n" +
		"- **Edges**: 1\n\n" +
		"### Domains\n\n" +
		"| Domain | Description | Nodes |\n| --- | --- | --- |\n" +
		"| core | Core entities | User, Project |\n\n" +
		"### Edge types\n\n" +
		"| Edge | Source -> target | Description |\n| --- | --- | --- |\n" +
		"| AUTHORED |  |  |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
		schemaExpandHint +
		"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n"

	if got := FormatSchemaMarkdown(out); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// schemaExpandHint is the step a schema answered with nothing expanded ends
// with: the card lists no property, and orbit.query names them (issue 1311).
const schemaExpandHint = "- Use action 'orbit.schema' to read the properties and edges of the node types a query reads: name them in expand, or set format=llm for the whole ontology as compact text\n"

// TestFormatSchemaMarkdown_NoDomains_WritesNoTable verifies that an ontology
// with no domains renders its counts and no Domains section, rather than a
// heading over an empty table.
func TestFormatSchemaMarkdown_NoDomains_WritesNoTable(t *testing.T) {
	out := SchemaOutput{SchemaVersion: "1.0", Nodes: []any{map[string]any{"name": "User"}}}

	want := "## Orbit Schema\n\n" +
		"- **Schema version**: 1.0\n" +
		"- **Domains**: 0\n" +
		"- **Nodes**: 1\n" +
		"- **Edges**: 0\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
		schemaExpandHint +
		"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n"

	if got := FormatSchemaMarkdown(out); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatSchemaMarkdown_ExpandedNode_ListsItsPropertiesAndEdges verifies
// that the text answer carries what expand returns and the edge types every
// answer carries (issue 1311).
//
// The card used to write the version, three counts and the domains, and
// nothing a query is built from: the properties of the nodes the caller asked
// to expand and every edge type were in the structured output only, so a
// client that hands the model the text, as most do on the default surface,
// left it to guess them, while the next step told it to use "the node and
// edge types listed here". The node that was not expanded gets no section,
// and with one expanded the step pointing at expand is not offered again.
// GitLab-authored names are escaped like any other cell.
func TestFormatSchemaMarkdown_ExpandedNode_ListsItsPropertiesAndEdges(t *testing.T) {
	out := SchemaOutput{
		SchemaVersion: "0.1",
		Nodes: []any{
			map[string]any{"name": "User", "domain": "core"},
			map[string]any{
				"name":           "Project",
				"domain":         "core",
				"description":    "A GitLab project",
				"incoming_edges": []any{"IN_PROJECT"},
				"outgoing_edges": []any{"IN_GROUP", "OWNED|BY"},
				"properties": []any{
					map[string]any{"name": "full_path", "data_type": "String", "nullable": false},
					map[string]any{"name": "visibility", "data_type": "Enum", "nullable": true, "enum_values": []any{"private", "public"}},
				},
			},
		},
		Edges: []SchemaEdge{{
			Name:        "IN_GROUP",
			Description: "A project or subgroup in its group",
			Variants:    []SchemaEdgeVariant{{SourceType: "Project", TargetType: "Group"}, {SourceType: "Group", TargetType: "Group"}},
		}},
	}

	want := "## Orbit Schema\n\n" +
		"- **Schema version**: 0.1\n" +
		"- **Domains**: 0\n" +
		"- **Nodes**: 2\n" +
		"- **Edges**: 1\n\n" +
		"### Edge types\n\n" +
		"| Edge | Source -> target | Description |\n| --- | --- | --- |\n" +
		"| IN_GROUP | Project -> Group, Group -> Group | A project or subgroup in its group |\n\n" +
		"### Node Project\n\n" +
		"- **Domain**: core\n" +
		"- **Description**: A GitLab project\n" +
		"- **Incoming edges**: IN_PROJECT\n" +
		"- **Outgoing edges**: IN_GROUP, OWNED&#124;BY\n\n" +
		"#### Properties\n\n" +
		"| Property | Type | Nullable | Values |\n| --- | --- | --- | --- |\n" +
		"| full_path | String | ❌ |  |\n" +
		"| visibility | Enum | ✅ | private, public |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
		"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n"

	if got := FormatSchemaMarkdown(out); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%s\nwant\n%s", got, want)
	}
}

// TestFormatSchemaMarkdown_ExpandedNode_IsTheOneCarryingAnyExpandedKey
// verifies that a node counts as expanded when GitLab sent any one of the
// three keys only expand adds, each on its own, and that a node carrying no
// property writes no Properties table under its rows.
func TestFormatSchemaMarkdown_ExpandedNode_IsTheOneCarryingAnyExpandedKey(t *testing.T) {
	tests := []struct {
		name string
		node map[string]any
		want string
	}{
		{
			name: "properties alone",
			node: map[string]any{"name": "Issue", "properties": []any{map[string]any{"name": "iid", "data_type": "Int"}}},
			want: "### Node Issue\n\n" +
				"#### Properties\n\n" +
				"| Property | Type | Nullable | Values |\n| --- | --- | --- | --- |\n" +
				"| iid | Int | ❌ |  |\n",
		},
		{
			name: "incoming edges alone",
			node: map[string]any{"name": "Issue", "incoming_edges": []any{"AUTHORED"}},
			want: "### Node Issue\n\n- **Incoming edges**: AUTHORED\n",
		},
		{
			name: "outgoing edges alone",
			node: map[string]any{"name": "Issue", "outgoing_edges": []any{"IN_PROJECT"}},
			want: "### Node Issue\n\n- **Outgoing edges**: IN_PROJECT\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "## Orbit Schema\n\n" +
				"- **Domains**: 0\n" +
				"- **Nodes**: 1\n" +
				"- **Edges**: 0\n\n" +
				tt.want +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
				"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n"
			if got := FormatSchemaMarkdown(SchemaOutput{Nodes: []any{tt.node}}); got != want {
				t.Errorf("FormatSchemaMarkdown() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// TestFormatSchemaMarkdown_UndecodableNode_IsLeftToTheStructuredOutput
// verifies that a node definition that cannot be read as the card's node
// shape is not rendered from a guess: a value JSON cannot encode and a
// properties key holding a string both leave the node out of the sections,
// and with no node read as expanded the card offers the step that asks for
// properties.
func TestFormatSchemaMarkdown_UndecodableNode_IsLeftToTheStructuredOutput(t *testing.T) {
	out := SchemaOutput{Nodes: []any{
		math.NaN(),
		map[string]any{"name": "Issue", "properties": "iid"},
	}}

	want := "## Orbit Schema\n\n" +
		"- **Domains**: 0\n" +
		"- **Nodes**: 2\n" +
		"- **Edges**: 0\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
		schemaExpandHint +
		"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n"

	if got := FormatSchemaMarkdown(out); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%s\nwant\n%s", got, want)
	}
}

// TestFormatToolsMarkdown_RendersTheWholeTable verifies that the tool manifest
// renders as a table whose name column is a code span, and that its next steps
// lead to the grammar and the ontology a graph query is built from.
//
// The name used to be stripped of every backtick it held and then escaped as a
// cell inside a hand-written span, which showed the entity as its five
// characters and quietly deleted part of the name. The span writer for a cell
// keeps the name whole and escapes only the pipe, which is the one character a
// span cannot contain in a table. The first row is one of the two tools
// GitLab.com lists, neither of which carries the query DSL, so the next steps
// that used to send a model to the tools' parameters sent it nowhere useful.
func TestFormatToolsMarkdown_RendersTheWholeTable(t *testing.T) {
	out := ToolsOutput{Tools: []ToolDefinition{
		{Name: "list_commands", Description: "Discover Orbit commands"},
		{Name: "count`nodes", Description: "Count nodes"},
	}}

	want := "## Orbit Tools (2)\n\n" +
		"| Tool | Description |\n| --- | --- |\n" +
		"| `list_commands` | Discover Orbit commands |\n" +
		"| ``count`nodes`` | Count nodes |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.dsl' to read the grammar a graph query is written in\n" +
		"- Use action 'orbit.schema' to read the node and edge names a graph query names\n"

	if got := FormatToolsMarkdown(out); got != want {
		t.Errorf("FormatToolsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatToolsMarkdown_NoTools_IsOneSentence verifies that an empty manifest
// renders the one sentence an empty list renders.
func TestFormatToolsMarkdown_NoTools_IsOneSentence(t *testing.T) {
	want := "No Orbit tools found.\n"

	if got := FormatToolsMarkdown(ToolsOutput{}); got != want {
		t.Errorf("FormatToolsMarkdown() = %q, want %q", got, want)
	}
}

// dslHints is the end of every query language card that describes a
// language: run a query in it, and read the names one is built from.
const dslHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'orbit.query' to run a query written in this language, sent as text\n" +
	"- Use action 'orbit.schema' to read the entities, properties and relationship types a query names\n"

// TestFormatDSLMarkdown_QueryCommand_RendersGrammarAndInputSchema verifies
// that the raw description of the query command renders as the card of one
// object: the command as a row, the grammar as text, and the JSON Schema of the
// command's input as indented JSON, each fence titled, then the two next steps
// a reader of a grammar needs.
func TestFormatDSLMarkdown_QueryCommand_RendersGrammarAndInputSchema(t *testing.T) {
	out := DSLOutput{
		Name:        "query_graph",
		Description: "Read-only openCypher 9-based syntax (MATCH ... RETURN).",
		Parameters:  map[string]any{"type": "object", "required": []any{"query"}},
	}

	want := "## Orbit Query Language\n\n" +
		"- **Command**: query_graph\n\n" +
		"### Grammar\n\n" +
		"```text\nRead-only openCypher 9-based syntax (MATCH ... RETURN).\n```\n\n" +
		"### Input schema\n\n" +
		"```json\n{\n  \"required\": [\n    \"query\"\n  ],\n  \"type\": \"object\"\n}\n```\n" +
		dslHints

	if got := FormatDSLMarkdown(out); got != want {
		t.Errorf("FormatDSLMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatDSLMarkdown_FormattedText_RendersOneFence verifies that the
// compact text GitLab answers the llm format with is the whole body of the
// card, one text fence ending on the same two next steps.
func TestFormatDSLMarkdown_FormattedText_RendersOneFence(t *testing.T) {
	out := DSLOutput{ResponseFormat: "llm", FormattedText: "commands[1]{name,description}:\n  query_graph,Read-only openCypher 9-based syntax"}

	want := "## Orbit Query Language\n\n" +
		"```text\ncommands[1]{name,description}:\n  query_graph,Read-only openCypher 9-based syntax\n```\n" +
		dslHints

	if got := FormatDSLMarkdown(out); got != want {
		t.Errorf("FormatDSLMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatDSLMarkdown_PartsAlone_RenderOnlyWhatCame verifies that each part
// of the command renders without the others, so a command GitLab described
// with no input schema, or an input schema with no description, is not
// mistaken for no language at all.
func TestFormatDSLMarkdown_PartsAlone_RenderOnlyWhatCame(t *testing.T) {
	tests := []struct {
		name string
		out  DSLOutput
		want string
	}{
		{
			name: "a command alone",
			out:  DSLOutput{Name: "query_graph"},
			want: "- **Command**: query_graph\n",
		},
		{
			name: "a grammar alone",
			out:  DSLOutput{Description: "MATCH ... RETURN"},
			want: "### Grammar\n\n```text\nMATCH ... RETURN\n```\n",
		},
		{
			name: "an input schema alone",
			out:  DSLOutput{Parameters: map[string]any{"type": "object"}},
			want: "### Input schema\n\n```json\n{\n  \"type\": \"object\"\n}\n```\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "## Orbit Query Language\n\n" + tt.want + dslHints
			if got := FormatDSLMarkdown(tt.out); got != want {
				t.Errorf("FormatDSLMarkdown() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestFormatDSLMarkdown_NoQueryCommand_PointsAtTheSchema verifies that an
// answer describing no query command renders a sentence rather than an empty
// card, and ends on the action that lists what a query is built from, which is
// what a caller of orbit.dsl is told whatever GitLab answered.
func TestFormatDSLMarkdown_NoQueryCommand_PointsAtTheSchema(t *testing.T) {
	want := "## Orbit Query Language\n\nGitLab described no query command, so no query language was returned.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.schema' to read the entities, properties and relationship types a query names\n"

	if got := FormatDSLMarkdown(DSLOutput{ResponseFormat: "raw"}); got != want {
		t.Errorf("FormatDSLMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatQueryMarkdown_StructuredResult_RendersRowsThenFences verifies that
// a query result renders as a card whose rows say what was asked and whose
// bodies carry the query text and the rows as JSON.
func TestFormatQueryMarkdown_StructuredResult_RendersRowsThenFences(t *testing.T) {
	out := QueryOutput{
		QueryType: "traversal",
		RowCount:  1,
		Result:    []any{map[string]any{"name": "alpha"}},
	}

	want := "## Orbit Query Result\n\n" +
		"- **Query type**: traversal\n" +
		"- **Row count**: 1\n\n" +
		"### Result\n\n" +
		"```json\n[\n  {\n    \"name\": \"alpha\"\n  }\n]\n```\n" +
		queryHints

	if got := FormatQueryMarkdown(out); got != want {
		t.Errorf("FormatQueryMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatQueryMarkdown_FormattedText_RendersOneFence verifies that the
// pre-formatted body replaces the structured rows rather than joining them.
func TestFormatQueryMarkdown_FormattedText_RendersOneFence(t *testing.T) {
	want := "## Orbit Query Result\n\n```text\n@header\nProject(name: gitlab)\n```\n" + queryHints

	if got := FormatQueryMarkdown(QueryOutput{FormattedText: "@header\nProject(name: gitlab)"}); got != want {
		t.Errorf("FormatQueryMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatQueryMarkdown_BacktickRuns_WidenEveryFence verifies that a body
// holding a run of three backticks cannot close the fence it sits in: the fence
// is sized to the longest run inside it, in the raw query text and in the JSON
// alike.
func TestFormatQueryMarkdown_BacktickRuns_WidenEveryFence(t *testing.T) {
	out := QueryOutput{
		QueryType:       "traversal",
		RawQueryStrings: []string{"MATCH (n) RETURN ```"},
		Result:          map[string]any{"text": "contains ``` fenced text"},
	}

	want := "## Orbit Query Result\n\n" +
		"- **Query type**: traversal\n\n" +
		"### Raw Query Strings\n\n" +
		"````text\nMATCH (n) RETURN ```\n````\n\n" +
		"### Result\n\n" +
		"````json\n{\n  \"text\": \"contains ``` fenced text\"\n}\n````\n" +
		queryHints

	if got := FormatQueryMarkdown(out); got != want {
		t.Errorf("FormatQueryMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatGraphStatusMarkdown_RendersTheWholeCard verifies that indexing
// status renders as a card whose per-domain counts are a nested collection, and
// that a duration GitLab did not send writes no row.
func TestFormatGraphStatusMarkdown_RendersTheWholeCard(t *testing.T) {
	out := GraphStatusOutput{
		Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 3, Gaps: 1},
		Indexing: &GraphStatusIndexing{
			State:           "indexed",
			LastStartedAt:   "2026-03-20T15:45:00Z",
			LastCompletedAt: "2026-03-20T15:50:00Z",
			LastDurationMs:  5,
			LastError:       "one project could not be read",
		},
		Domains: []GraphStatusDomain{{
			Name:  "SDLC",
			Items: []GraphStatusDomainItem{{Name: "Issue", Count: 4}, {Name: "MergeRequest", Count: 7}},
		}},
	}

	want := "## Orbit Graph Status\n\n" +
		"- **Indexed projects**: 2\n" +
		"- **Total known projects**: 3\n" +
		"- **Projects out of indexing attempts**: 1\n" +
		"- **Indexing state**: indexed\n" +
		"- **Last started at**: 20 Mar 2026 15:45 UTC\n" +
		"- **Last completed at**: 20 Mar 2026 15:50 UTC\n" +
		"- **Last duration (ms)**: 5\n" +
		"- **Last error**: one project could not be read\n\n" +
		"### Domains\n\n" +
		"| Domain | Counts |\n| --- | --- |\n" +
		"| SDLC | Issue: 4, MergeRequest: 7 |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.query' to query the graph once indexing reaches a healthy state\n" +
		"- Use action 'orbit.status' to check the cluster itself when indexing never starts\n"

	if got := FormatGraphStatusMarkdown(out); got != want {
		t.Errorf("FormatGraphStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatGraphStatusMarkdown_NoDomains_WritesNoTable verifies that an
// indexing status carrying no per-domain counts renders its project rows and
// no Domains section, rather than a heading over an empty table. A count of
// zero projects out of indexing attempts is written too, since GitLab always
// sends the count and zero is the answer a healthy namespace gives.
func TestFormatGraphStatusMarkdown_NoDomains_WritesNoTable(t *testing.T) {
	out := GraphStatusOutput{Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 3}}

	want := "## Orbit Graph Status\n\n" +
		"- **Indexed projects**: 2\n" +
		"- **Total known projects**: 3\n" +
		"- **Projects out of indexing attempts**: 0\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.query' to query the graph once indexing reaches a healthy state\n" +
		"- Use action 'orbit.status' to check the cluster itself when indexing never starts\n"

	if got := FormatGraphStatusMarkdown(out); got != want {
		t.Errorf("FormatGraphStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatGraphStatusMarkdown_FormattedText_RendersOneFence verifies that the
// pre-formatted body replaces the structured rows here too.
func TestFormatGraphStatusMarkdown_FormattedText_RendersOneFence(t *testing.T) {
	want := "## Orbit Graph Status\n\n```text\nindexing: indexed\n```\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.query' to query the graph once indexing reaches a healthy state\n" +
		"- Use action 'orbit.status' to check the cluster itself when indexing never starts\n"

	if got := FormatGraphStatusMarkdown(GraphStatusOutput{FormattedText: "indexing: indexed"}); got != want {
		t.Errorf("FormatGraphStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestOrbitMarkdownFormatters_HostileValues_StayInsideTheirCells verifies that
// a name, a description or a status the Knowledge Graph API answered with
// cannot split a row or end a table: every one of them is a free string, and
// nothing in this repository constrains any of them.
func TestOrbitMarkdownFormatters_HostileValues_StayInsideTheirCells(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "status components",
			got: FormatStatusMarkdown(StatusOutput{Components: []StatusComponent{{
				Name:   "click|house",
				Status: "healthy\nenough",
			}}}),
			want: "## Orbit Status\n\n" +
				"### Components\n\n" +
				"| Component | Status | Replicas |\n| --- | --- | --- |\n" +
				"| click&#124;house | healthy enough |  |\n" +
				statusHints,
		},
		{
			name: "schema domains",
			got: FormatSchemaMarkdown(SchemaOutput{Domains: []SchemaDomain{{
				Name:        "core|domain",
				Description: "Core\nentities",
				NodeNames:   []string{"User|Account"},
			}}}),
			want: "## Orbit Schema\n\n" +
				"- **Domains**: 1\n" +
				"- **Nodes**: 0\n" +
				"- **Edges**: 0\n\n" +
				"### Domains\n\n" +
				"| Domain | Description | Nodes |\n| --- | --- | --- |\n" +
				"| core&#124;domain | Core entities | User&#124;Account |\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
				"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n",
		},
		{
			name: "tool manifest",
			got:  FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query`|graph", Description: "Run\nqueries"}}}),
			want: "## Orbit Tools (1)\n\n" +
				"| Tool | Description |\n| --- | --- |\n" +
				"| ``query`\\|graph`` | Run queries |\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'orbit.dsl' to read the grammar a graph query is written in\n" +
				"- Use action 'orbit.schema' to read the node and edge names a graph query names\n",
		},
		{
			name: "graph status domains",
			got: FormatGraphStatusMarkdown(GraphStatusOutput{Domains: []GraphStatusDomain{{
				Name:  "SDLC|core",
				Items: []GraphStatusDomainItem{{Name: "Issue|Bug", Count: 4}},
			}}}),
			want: "## Orbit Graph Status\n\n" +
				"### Domains\n\n" +
				"| Domain | Counts |\n| --- | --- |\n" +
				"| SDLC&#124;core | Issue&#124;Bug: 4 |\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'orbit.query' to query the graph once indexing reaches a healthy state\n" +
				"- Use action 'orbit.status' to check the cluster itself when indexing never starts\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("markdown =\n%q\nwant\n%q", tt.got, tt.want)
			}
		})
	}
}
