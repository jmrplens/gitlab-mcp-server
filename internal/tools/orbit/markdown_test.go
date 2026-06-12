package orbit

import (
	"strings"
	"testing"
)

// TestFormatQueryMarkdown_IncludesPrettyJSON verifies that [FormatQueryMarkdown]
// renders structured result data inside a JSON code fence.
//
// The test asserts that the markdown output includes a JSON code block and the expected data.
func TestFormatQueryMarkdown_IncludesPrettyJSON(t *testing.T) {
	md := FormatQueryMarkdown(QueryOutput{
		QueryType: "traversal",
		RowCount:  1,
		Result:    []any{map[string]any{"name": "alpha"}},
	})
	if !strings.Contains(md, "```json") || !strings.Contains(md, "alpha") {
		t.Fatalf("FormatQueryMarkdown() = %q, want JSON result", md)
	}
}

// TestOrbitMarkdownFormatters_IncludeExpectedSections verifies that each Orbit
// markdown formatter emits the expected headings and key result sections.
//
// The test runs table-driven subtests for all formatters and asserts that the output contains the expected headings and content.
func TestOrbitMarkdownFormatters_IncludeExpectedSections(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []string
	}{
		{
			name: "status structured",
			md: FormatStatusMarkdown(StatusOutput{
				Status:  "healthy",
				Version: "0.5.0",
				Components: []StatusComponent{
					{Name: "clickhouse", Status: "healthy", Replicas: &StatusReplicas{Ready: 3, Desired: 3}},
				},
			}),
			want: []string{"Orbit Status", "clickhouse", "3/3"},
		},
		{
			name: "status formatted",
			md:   FormatStatusMarkdown(StatusOutput{FormattedText: "status: healthy"}),
			want: []string{"```text", "status: healthy"},
		},
		{
			name: "schema",
			md: FormatSchemaMarkdown(SchemaOutput{
				SchemaVersion: "1.0",
				Domains:       []SchemaDomain{{Name: "core", Description: "Core entities", NodeNames: []string{"User"}}},
				Nodes:         []any{map[string]any{"name": "User"}},
				Edges:         []SchemaEdge{{Name: "AUTHORED"}},
			}),
			want: []string{"Orbit Schema", "Schema version", "core"},
		},
		{
			name: "tools",
			md:   FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query_graph", Description: "Execute graph queries"}}}),
			want: []string{"Orbit Tools", "query_graph"},
		},
		{
			name: "dsl",
			md:   FormatDSLMarkdown(DSLOutput{ResponseFormat: "llm", Content: "@dsl\nquery_type: traversal"}),
			want: []string{"Orbit DSL", "```text", "query_type: traversal", "Use `gitlab_orbit_query`"},
		},
		{
			name: "query formatted",
			md:   FormatQueryMarkdown(QueryOutput{FormattedText: "@header\nProject(name: gitlab)"}),
			want: []string{"Orbit Query Result", "```text", "@header", "Use `gitlab_orbit_graph_status`"},
		},
		{
			name: "graph status structured",
			md: FormatGraphStatusMarkdown(GraphStatusOutput{
				Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 3},
				Domains:  []GraphStatusDomain{{Name: "SDLC", Items: []GraphStatusDomainItem{{Name: "Issue", Count: 4}}}},
				Indexing: &GraphStatusIndexing{State: "indexed", LastDurationMs: 5},
			}),
			want: []string{"Orbit Graph Status", "Indexed projects", "SDLC"},
		},
		{
			name: "graph status formatted",
			md:   FormatGraphStatusMarkdown(GraphStatusOutput{FormattedText: "indexing: indexed"}),
			want: []string{"```text", "indexing: indexed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.want {
				if !strings.Contains(tt.md, want) {
					t.Fatalf("markdown = %q, want substring %q", tt.md, want)
				}
			}
			if tt.name == "query formatted" && strings.Contains(tt.md, "Query type") {
				t.Fatalf("markdown = %q, want formatted text without structured query fields", tt.md)
			}
		})
	}
}

// TestOrbitMarkdownFormatters_UseSafeFences verifies that formatter output uses
// longer fences when embedded query text contains triple backticks.
//
// The test asserts that the output uses four-backtick fences for embedded triple-backtick content.
func TestOrbitMarkdownFormatters_UseSafeFences(t *testing.T) {
	md := FormatQueryMarkdown(QueryOutput{
		QueryType:       "traversal",
		RawQueryStrings: []string{"MATCH (n) RETURN ```"},
		Result:          map[string]any{"text": "contains ``` fenced text"},
	})

	if !strings.Contains(md, "````text\nMATCH (n) RETURN ```\n````") {
		t.Fatalf("FormatQueryMarkdown() = %q, want four-backtick text fence", md)
	}
	if !strings.Contains(md, "````json\n") || !strings.Contains(md, "contains ``` fenced text") {
		t.Fatalf("FormatQueryMarkdown() = %q, want four-backtick JSON fence", md)
	}
}

// TestOrbitMarkdownFormatters_AnonymousFence verifies that [fencedBlock] produces
// fenced code blocks without a language marker when requested.
func TestOrbitMarkdownFormatters_AnonymousFence(t *testing.T) {
	got := fencedBlock("", "plain text")
	if got != "```\nplain text\n```\n" {
		t.Fatalf("fencedBlock() = %q, want anonymous fence", got)
	}
}

// TestOrbitMarkdownFormatters_EscapeTableCells verifies that markdown table
// cells escape pipes and normalize newlines from Orbit data.
//
// The test runs table-driven subtests for schema, tools, and graph status tables and asserts that pipes are escaped and newlines normalized.
func TestOrbitMarkdownFormatters_EscapeTableCells(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "schema",
			md: FormatSchemaMarkdown(SchemaOutput{Domains: []SchemaDomain{{
				Name:        "core|domain",
				Description: "Core\nentities",
				NodeNames:   []string{"User|Account"},
			}}}),
			want: "core&#124;domain | Core entities | User&#124;Account",
		},
		{
			name: "tools",
			md:   FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query`|graph", Description: "Run\nqueries"}}}),
			want: "`query&#124;graph` | Run queries",
		},
		{
			name: "graph status",
			md:   FormatGraphStatusMarkdown(GraphStatusOutput{Domains: []GraphStatusDomain{{Name: "SDLC|core", Items: []GraphStatusDomainItem{{Name: "Issue|Bug", Count: 4}}}}}),
			want: "SDLC&#124;core | Issue&#124;Bug: 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.md, tt.want) {
				t.Fatalf("markdown = %q, want escaped substring %q", tt.md, tt.want)
			}
		})
	}
}
