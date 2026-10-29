// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// statusHints is the guidance section every Orbit status card closes with.
const statusHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'orbit.graph_status' to inspect indexing status for a namespace or project\n"

// queryHints is the guidance section every Orbit query card closes with.
const queryHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'orbit.graph_status' to check indexing when a result looks stale or incomplete\n" +
	"- Use action 'orbit.dsl' to read the grammar the next query is written in\n"

// schemaHints is the end of a schema card with a node expanded: read the
// grammar, then run a query.
const schemaHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
	"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n"

// schemaExpandHint is the step a schema answered with nothing expanded adds:
// the card lists no property, and orbit.query names them (issue 1311).
const schemaExpandHint = "- Use action 'orbit.schema' to read the properties of the node types a query reads: name them in expand, or set format=llm for the schema as compact text\n"

// graphStatusHints is the guidance section every indexing status card closes
// with.
const graphStatusHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'orbit.query' to query the graph once indexing reaches a healthy state\n" +
	"- Use action 'orbit.status' to check the cluster itself when indexing never starts\n"

// TestFormatStatusMarkdown_RendersTheWholeCard verifies that the service
// status renders as a card whose components are a nested collection.
func TestFormatStatusMarkdown_RendersTheWholeCard(t *testing.T) {
	out := StatusOutput{
		Status:  "HEALTHY",
		Version: "0.163.0",
		Components: []StatusComponent{
			{Name: "clickhouse-graph", Status: "DEGRADED"},
			{Name: "nats", Status: "HEALTHY"},
		},
	}

	want := "## Orbit Status\n\n" +
		"- **Status**: HEALTHY\n" +
		"- **Version**: 0.163.0\n\n" +
		"### Components\n\n" +
		"| Component | Status |\n| --- | --- |\n" +
		"| clickhouse-graph | DEGRADED |\n" +
		"| nats | HEALTHY |\n" +
		statusHints

	if got := FormatStatusMarkdown(out); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStatusMarkdown_OneFieldAlone_RendersItsRow verifies that each
// part of the status renders without the others, so a status with only a
// version is not read as no status at all.
func TestFormatStatusMarkdown_OneFieldAlone_RendersItsRow(t *testing.T) {
	tests := []struct {
		name string
		out  StatusOutput
		want string
	}{
		{name: "status", out: StatusOutput{Status: "UNKNOWN"}, want: "- **Status**: UNKNOWN\n"},
		{name: "version", out: StatusOutput{Version: "0.163.0"}, want: "- **Version**: 0.163.0\n"},
		{
			name: "components", out: StatusOutput{Components: []StatusComponent{{Name: "click|house", Status: "healthy\nenough"}}},
			want: "### Components\n\n| Component | Status |\n| --- | --- |\n| click&#124;house | healthy enough |\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "## Orbit Status\n\n" + tt.want + statusHints
			if got := FormatStatusMarkdown(tt.out); got != want {
				t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestFormatStatusMarkdown_NoData_SaysSo verifies that a status with nothing
// in it renders a sentence rather than an empty card.
func TestFormatStatusMarkdown_NoData_SaysSo(t *testing.T) {
	want := "## Orbit Status\n\nNo Orbit status data returned.\n" + statusHints
	if got := FormatStatusMarkdown(StatusOutput{}); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatSchemaMarkdown_RendersTheWholeCard verifies that the schema
// renders as a card with the three type counts as rows and the domains and
// edge kinds as nested collections, and that a schema with no node expanded
// offers the step that asks for properties.
func TestFormatSchemaMarkdown_RendersTheWholeCard(t *testing.T) {
	out := SchemaOutput{
		Domains: []SchemaDomain{{Name: "core|domain", NodeNames: []string{"User|Account", "Project"}}},
		Nodes:   []SchemaNode{{Name: "User|Account", Domain: "core|domain"}, {Name: "Project", Domain: "core|domain"}},
		Edges: []SchemaEdge{{
			Name:        "IN_GROUP",
			Description: "A project\nor subgroup in its group",
			Variants:    []SchemaEdgeVariant{{SourceType: "Project", TargetType: "Group"}, {SourceType: "Group", TargetType: "Group"}},
		}},
	}

	want := "## Orbit Schema\n\n" +
		"- **Domains**: 1\n" +
		"- **Nodes**: 2\n" +
		"- **Edges**: 1\n\n" +
		"### Domains\n\n" +
		"| Domain | Nodes |\n| --- | --- |\n" +
		"| core&#124;domain | User&#124;Account, Project |\n\n" +
		"### Edge types\n\n" +
		"| Edge | Source -> target | Description |\n| --- | --- | --- |\n" +
		"| IN_GROUP | Project -> Group, Group -> Group | A project or subgroup in its group |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.dsl' to read the grammar a query is written in\n" +
		schemaExpandHint +
		"- Use action 'orbit.query' to run a query naming the node and edge types listed here\n"

	if got := FormatSchemaMarkdown(out); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatSchemaMarkdown_ExpandedNode_ListsItsProperties verifies that the
// text answer carries the properties of the expanded nodes, each node a
// section with its domain, description, key and label field as rows, and that
// with one node expanded the step pointing at expand is not offered again
// (issue 1311). The node that was not expanded gets no section, and a row
// GitLab sent no value for is left out.
func TestFormatSchemaMarkdown_ExpandedNode_ListsItsProperties(t *testing.T) {
	out := SchemaOutput{
		Nodes: []SchemaNode{
			{Name: "User", Domain: "core"},
			{
				Name: "Project", Domain: "core", Description: "A GitLab project", PrimaryKey: "id", LabelField: "full_path",
				Properties: []SchemaProperty{
					{Name: "full_path", DataType: "String"},
					{Name: "visibility", DataType: "Enum", Nullable: true, EnumValues: []string{"private", "public"}},
				},
			},
			{Name: "Issue", Properties: []SchemaProperty{{Name: "iid", DataType: "Int"}}},
		},
	}

	want := "## Orbit Schema\n\n" +
		"- **Domains**: 0\n" +
		"- **Nodes**: 3\n" +
		"- **Edges**: 0\n\n" +
		"### Node Project\n\n" +
		"- **Domain**: core\n" +
		"- **Description**: A GitLab project\n" +
		"- **Primary key**: id\n" +
		"- **Label field**: full_path\n\n" +
		"#### Properties\n\n" +
		"| Property | Type | Nullable | Values |\n| --- | --- | --- | --- |\n" +
		"| full_path | String | ❌ |  |\n" +
		"| visibility | Enum | ✅ | private, public |\n\n" +
		"### Node Issue\n\n" +
		"#### Properties\n\n" +
		"| Property | Type | Nullable | Values |\n| --- | --- | --- | --- |\n" +
		"| iid | Int | ❌ |  |\n" +
		schemaHints

	if got := FormatSchemaMarkdown(out); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%s\nwant\n%s", got, want)
	}
}

// TestFormatSchemaMarkdown_FormattedText_RendersOneFence verifies that the
// compact text of the llm format is the whole body of the card.
func TestFormatSchemaMarkdown_FormattedText_RendersOneFence(t *testing.T) {
	want := "## Orbit Schema\n\n```text\ndomains[9]:\n```\n" + schemaHints
	if got := FormatSchemaMarkdown(SchemaOutput{FormattedText: "domains[9]:"}); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatSchemaMarkdown_NoNodes_OffersNoExpandStep verifies that an empty
// schema renders its counts and does not send the caller to expand nodes it
// does not have.
func TestFormatSchemaMarkdown_NoNodes_OffersNoExpandStep(t *testing.T) {
	want := "## Orbit Schema\n\n- **Domains**: 0\n- **Nodes**: 0\n- **Edges**: 0\n" + schemaHints
	if got := FormatSchemaMarkdown(SchemaOutput{}); got != want {
		t.Errorf("FormatSchemaMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatToolsMarkdown_RendersTheWholeTable verifies that the tool list
// renders as a table whose name column is a code span, and that its next steps
// lead to the grammar and the schema a graph query is built from. A name
// holding a backtick and a pipe stays whole inside its span.
func TestFormatToolsMarkdown_RendersTheWholeTable(t *testing.T) {
	out := ToolsOutput{Tools: []ToolDefinition{
		{Name: "list_commands", Description: "Discover Orbit commands"},
		{Name: "query`|graph", Description: "Run\nqueries"},
	}}

	want := "## Orbit Tools (2)\n\n" +
		"| Tool | Description |\n| --- | --- |\n" +
		"| `list_commands` | Discover Orbit commands |\n" +
		"| ``query`\\|graph`` | Run queries |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'orbit.dsl' to read the grammar a graph query is written in\n" +
		"- Use action 'orbit.schema' to read the node and edge names a graph query names\n"

	if got := FormatToolsMarkdown(out); got != want {
		t.Errorf("FormatToolsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatToolsMarkdown_NoTools_IsOneSentence verifies that an empty list
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
// command's input as indented JSON, each fence titled.
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
	out := DSLOutput{ResponseFormat: "llm", FormattedText: "commands[1]:\n  - name: query_graph"}

	want := "## Orbit Query Language\n\n" +
		"```text\ncommands[1]:\n  - name: query_graph\n```\n" +
		dslHints

	if got := FormatDSLMarkdown(out); got != want {
		t.Errorf("FormatDSLMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatDSLMarkdown_PartsAlone_RenderOnlyWhatCame verifies that each part
// of the command renders without the others.
func TestFormatDSLMarkdown_PartsAlone_RenderOnlyWhatCame(t *testing.T) {
	tests := []struct {
		name string
		out  DSLOutput
		want string
	}{
		{name: "a command alone", out: DSLOutput{Name: "query_graph"}, want: "- **Command**: query_graph\n"},
		{name: "a grammar alone", out: DSLOutput{Description: "MATCH ... RETURN"}, want: "### Grammar\n\n```text\nMATCH ... RETURN\n```\n"},
		{name: "an input schema alone", out: DSLOutput{Parameters: map[string]any{"type": "object"}}, want: "### Input schema\n\n```json\n{\n  \"type\": \"object\"\n}\n```\n"},
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
// card, and ends on the action that lists what a query is built from.
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
// bodies carry the rows as JSON.
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
// holding a run of three backticks cannot close the fence it sits in.
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
// status renders as a card: whether Orbit is on and since when, the project
// counts, the indexing state with the areas as a nested collection, and the
// per-domain counts as another. Names GitLab answered are escaped as cells.
func TestFormatGraphStatusMarkdown_RendersTheWholeCard(t *testing.T) {
	enabled := true
	out := GraphStatusOutput{
		Enabled:   &enabled,
		EnabledAt: "2026-06-11T23:19:22Z",
		Projects:  &GraphStatusProjects{Indexed: 2, TotalKnown: 3, Gaps: 1, NotIndexed: 1, Total: 3},
		Indexing: &GraphStatusIndexing{
			State:           "READY",
			LastCompletedAt: "2026-10-29T08:24:01Z",
			Areas:           []GraphStatusArea{{Name: "ci|cd", State: "READY"}},
		},
		Domains: []GraphStatusDomain{{
			Name:  "SDLC|core",
			Items: []GraphStatusDomainItem{{Name: "Issue|Bug", Count: 4}, {Name: "MergeRequest", Count: 7}},
		}},
	}

	want := "## Orbit Graph Status\n\n" +
		"- **Orbit enabled**: ✅\n" +
		"- **Enabled at**: 11 Jun 2026 23:19 UTC\n" +
		"- **Indexed projects**: 2\n" +
		"- **Total known projects**: 3\n" +
		"- **Projects not indexed**: 1\n" +
		"- **Projects out of indexing attempts**: 1\n" +
		"- **Indexing state**: READY\n" +
		"- **Last completed at**: 29 Oct 2026 08:24 UTC\n\n" +
		"### Areas\n\n" +
		"| Area | State |\n| --- | --- |\n" +
		"| ci&#124;cd | READY |\n\n" +
		"### Domains\n\n" +
		"| Domain | Counts |\n| --- | --- |\n" +
		"| SDLC&#124;core | Issue&#124;Bug: 4, MergeRequest: 7 |\n" +
		graphStatusHints

	if got := FormatGraphStatusMarkdown(out); got != want {
		t.Errorf("FormatGraphStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatGraphStatusMarkdown_PartsAlone_RenderOnlyWhatCame verifies that a
// project, for which GitLab sends no enabled flag, and an indexing state with
// no area render their rows and nothing for what GitLab did not send.
func TestFormatGraphStatusMarkdown_PartsAlone_RenderOnlyWhatCame(t *testing.T) {
	disabled := false
	tests := []struct {
		name string
		out  GraphStatusOutput
		want string
	}{
		{name: "orbit off", out: GraphStatusOutput{Enabled: &disabled}, want: "- **Orbit enabled**: ❌\n"},
		{
			name: "projects alone", out: GraphStatusOutput{Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 3}},
			want: "- **Indexed projects**: 2\n- **Total known projects**: 3\n- **Projects not indexed**: 0\n- **Projects out of indexing attempts**: 0\n",
		},
		{name: "indexing without areas", out: GraphStatusOutput{Indexing: &GraphStatusIndexing{State: "INDEXING"}}, want: "- **Indexing state**: INDEXING\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "## Orbit Graph Status\n\n" + tt.want + graphStatusHints
			if got := FormatGraphStatusMarkdown(tt.out); got != want {
				t.Errorf("FormatGraphStatusMarkdown() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// accessGateHints is the guidance section of an indexing status card for a
// group GitLab.com serves no index: the access, not a query.
const accessGateHints = "\n---\n💡 **Next steps:**\n" +
	"- Orbit access is set on the top-level group on GitLab.com: a license or a running trial there serves the index, which this status shows once it does\n"

// TestFormatGraphStatusMarkdown_TopLevelGroup_SaysWhatItsAccessMeans verifies
// the card of a top-level group, which carries its Orbit access and trial: the
// access as rows and the trial as a nested object, as GitLab spelled them,
// then a line of its own when the access explains the index. A group
// GitLab.com serves no index (jmrp-oss since 2026-10-29) is told the missing
// index is the access, and its one next step leads there; a licensed group
// with its index gets no such line, one with none is told why, and a running
// trial is told that it ends.
func TestFormatGraphStatusMarkdown_TopLevelGroup_SaysWhatItsAccessMeans(t *testing.T) {
	enabled, disabled := true, false
	zero, total, offerCredits := 0.0, 1000.0, 500.5
	thirty := int64(30)
	index := &GraphStatusIndexing{State: "READY"}
	tests := []struct {
		name string
		out  GraphStatusOutput
		want string
	}{
		{
			name: "trial not eligible, no index",
			out: GraphStatusOutput{
				Enabled: &enabled, EnabledAt: "2026-06-11T23:19:22Z",
				Access: &GraphStatusAccess{State: "TRIAL_NOT_ELIGIBLE"},
				Trial:  &GraphStatusTrial{State: "NOT_ELIGIBLE", Credits: &GraphStatusTrialCredits{Remaining: &zero}},
			},
			want: "- **Orbit enabled**: ✅\n" +
				"- **Enabled at**: 11 Jun 2026 23:19 UTC\n" +
				"- **Orbit access**: TRIAL_NOT_ELIGIBLE\n" +
				"- **Orbit trial**:\n" +
				"  - **State**: NOT_ELIGIBLE\n" +
				"  - **Credits remaining**: 0\n\n" +
				"Orbit access is TRIAL_NOT_ELIGIBLE: GitLab.com serves a group's Orbit index and queries only while its access is LICENSED or TRIAL_ACTIVE, " +
				"so an index missing or empty here reflects the access, not the graph.\n" +
				accessGateHints,
		},
		{
			name: "purchase required, Orbit off",
			out: GraphStatusOutput{
				Enabled: &disabled,
				Access:  &GraphStatusAccess{State: "UNLICENSED", Reason: "NO_ACCESS", TurnOnDenialReason: "PURCHASE_REQUIRED"},
			},
			want: "- **Orbit enabled**: ❌\n" +
				"- **Orbit access**: UNLICENSED\n" +
				"- **Access reason**: NO_ACCESS\n" +
				"- **Turn-on denial reason**: PURCHASE_REQUIRED\n\n" +
				"Orbit access is UNLICENSED: GitLab.com serves a group's Orbit index and queries only while its access is LICENSED or TRIAL_ACTIVE, " +
				"so an index missing or empty here reflects the access, not the graph.\n" +
				accessGateHints,
		},
		{
			name: "trial eligible with an offer",
			out: GraphStatusOutput{
				Access: &GraphStatusAccess{State: "TRIAL_ELIGIBLE", TurnOnDenialReason: "TRIAL_REQUIRED"},
				Trial: &GraphStatusTrial{
					State: "ELIGIBLE", Credits: &GraphStatusTrialCredits{},
					Offer: &GraphStatusTrialOffer{DurationDays: &thirty, InitialCredits: &offerCredits},
				},
			},
			want: "- **Orbit access**: TRIAL_ELIGIBLE\n" +
				"- **Turn-on denial reason**: TRIAL_REQUIRED\n" +
				"- **Orbit trial**:\n" +
				"  - **State**: ELIGIBLE\n" +
				"  - **Offered trial days**: 30\n" +
				"  - **Offered trial credits**: 500.5\n\n" +
				"Orbit access is TRIAL_ELIGIBLE: GitLab.com serves a group's Orbit index and queries only while its access is LICENSED or TRIAL_ACTIVE, " +
				"so an index missing or empty here reflects the access, not the graph.\n" +
				accessGateHints,
		},
		{
			name: "licensed with its index",
			out:  GraphStatusOutput{Access: &GraphStatusAccess{State: AccessLicensed, Reason: "PROMO_ADD_ON"}, Indexing: index},
			want: "- **Orbit access**: LICENSED\n" +
				"- **Access reason**: PROMO_ADD_ON\n" +
				"- **Indexing state**: READY\n" +
				graphStatusHints,
		},
		{
			name: "licensed with no index",
			out:  GraphStatusOutput{Enabled: &disabled, Access: &GraphStatusAccess{State: AccessLicensed, Reason: "PLAN"}},
			want: "- **Orbit enabled**: ❌\n" +
				"- **Orbit access**: LICENSED\n" +
				"- **Access reason**: PLAN\n\n" +
				"The group has Orbit access and GitLab answered no index: Orbit is off for the top-level group, or the token is below the Reporter role there.\n" +
				graphStatusHints,
		},
		{
			name: "trial running with its index",
			out: GraphStatusOutput{
				Access: &GraphStatusAccess{State: AccessTrialActive},
				Trial: &GraphStatusTrial{
					State: "ACTIVE", ExpiresOn: "2026-11-28",
					Credits: &GraphStatusTrialCredits{Remaining: &offerCredits, Total: &total},
					Offer:   &GraphStatusTrialOffer{},
				},
				Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 2},
			},
			want: "- **Orbit access**: TRIAL_ACTIVE\n" +
				"- **Orbit trial**:\n" +
				"  - **State**: ACTIVE\n" +
				"  - **Expires on**: 28 Nov 2026\n" +
				"  - **Credits remaining**: 500.5\n" +
				"  - **Credits total**: 1000\n" +
				"- **Indexed projects**: 2\n" +
				"- **Total known projects**: 2\n" +
				"- **Projects not indexed**: 0\n" +
				"- **Projects out of indexing attempts**: 0\n\n" +
				"Orbit is served on a trial, which ends on its last day or when its credits run out.\n" +
				graphStatusHints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "## Orbit Graph Status\n\n" + tt.want
			if got := FormatGraphStatusMarkdown(tt.out); got != want {
				t.Errorf("FormatGraphStatusMarkdown() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestFormatOrbitNotFound_NamesWhatOrbitNeeds verifies the result a 404 on an
// Orbit route renders as: informational, marked as an error, and naming the
// resource with the two checks that explain it.
func TestFormatOrbitNotFound_NamesWhatOrbitNeeds(t *testing.T) {
	result := formatOrbitNotFound(orbitNotFoundOutput{Resource: "GitLab Orbit Status", Identifier: "service status"})
	if !result.IsError || len(result.Content) == 0 {
		t.Fatalf("formatOrbitNotFound() = %+v, want an error result with content", result)
	}
	text, isText := result.Content[0].(*mcp.TextContent)
	if !isText || !strings.Contains(text.Text, "GitLab Orbit Status") || !strings.Contains(text.Text, "Verify GitLab Orbit is enabled") {
		t.Fatalf("formatOrbitNotFound() content = %+v, want the resource and the checks", result.Content)
	}
}

// TestPrettyAny_ValueThatDoesNotEncode_FallsBackToItsText verifies the one
// fallback of the JSON fences: a value JSON cannot encode is written as Go
// formats it rather than lost.
func TestPrettyAny_ValueThatDoesNotEncode_FallsBackToItsText(t *testing.T) {
	if got := prettyAny(func() {}); got == "" {
		t.Fatal("prettyAny() returned empty fallback")
	}
}
