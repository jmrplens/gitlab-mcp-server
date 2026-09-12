package orbit

import (
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
		"| core | Core entities | User, Project |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.tools' to inspect the live query and tool manifest\n" +
		"- Use action 'orbit.query' to run a query once you have chosen a shape from the manifest\n"

	if got := FormatSchemaMarkdown(out); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatToolsMarkdown_RendersTheWholeTable verifies that the tool manifest
// renders as a table whose name column is a code span.
//
// The name used to be stripped of every backtick it held and then escaped as a
// cell inside a hand-written span, which showed the entity as its five
// characters and quietly deleted part of the name. The span writer for a cell
// keeps the name whole and escapes only the pipe, which is the one character a
// span cannot contain in a table.
func TestFormatToolsMarkdown_RendersTheWholeTable(t *testing.T) {
	out := ToolsOutput{Tools: []ToolDefinition{
		{Name: "query_graph", Description: "Execute graph queries"},
		{Name: "count`nodes", Description: "Count nodes"},
	}}

	want := "## Orbit Tools (2)\n\n" +
		"| Tool | Description |\n| --- | --- |\n" +
		"| `query_graph` | Execute graph queries |\n" +
		"| ``count`nodes`` | Count nodes |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.query' to build a query from the parameters a tool declares\n" +
		"- Use action 'orbit.schema' to read the node and edge names those parameters take\n"

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

// TestFormatDSLMarkdown_ResponseFormats_ChooseTheInfoString verifies that the
// grammar is fenced, and that the fence's language follows the response format
// the caller asked for.
func TestFormatDSLMarkdown_ResponseFormats_ChooseTheInfoString(t *testing.T) {
	tests := []struct {
		name string
		out  DSLOutput
		want string
	}{
		{
			name: "llm format is text",
			out:  DSLOutput{ResponseFormat: "llm", Content: "@dsl\nquery_type: traversal"},
			want: "```text\n@dsl\nquery_type: traversal\n```\n",
		},
		{
			name: "any other format is json",
			out:  DSLOutput{ResponseFormat: "json", Content: `{"query_type":"traversal"}`},
			want: "```json\n{\"query_type\":\"traversal\"}\n```\n",
		},
	}

	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.query' to run a query once you have chosen a shape from the DSL\n" +
		"- Use action 'orbit.schema' to read the node and edge names a query names\n"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "## Orbit DSL\n\n" + tt.want + hints
			if got := FormatDSLMarkdown(tt.out); got != want {
				t.Errorf("FormatDSLMarkdown() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestFormatDSLMarkdown_NoContent_SaysSo verifies that an empty grammar renders
// a sentence rather than an empty fence.
func TestFormatDSLMarkdown_NoContent_SaysSo(t *testing.T) {
	want := "## Orbit DSL\n\nNo Orbit DSL data returned.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.schema' to read the node and edge names a query names\n"

	if got := FormatDSLMarkdown(DSLOutput{}); got != want {
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
		Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 3},
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
				"- Use action 'orbit.tools' to inspect the live query and tool manifest\n" +
				"- Use action 'orbit.query' to run a query once you have chosen a shape from the manifest\n",
		},
		{
			name: "tool manifest",
			got:  FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query`|graph", Description: "Run\nqueries"}}}),
			want: "## Orbit Tools (1)\n\n" +
				"| Tool | Description |\n| --- | --- |\n" +
				"| ``query`\\|graph`` | Run queries |\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'orbit.query' to build a query from the parameters a tool declares\n" +
				"- Use action 'orbit.schema' to read the node and edge names those parameters take\n",
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
