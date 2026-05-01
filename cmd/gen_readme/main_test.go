// main_test.go verifies README generation helpers used by cmd/gen_readme.
package main

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestDescriptionSummary_StripsGeneratedMetaToolPrefix verifies that README
// table summaries ignore the generated meta-tool usage header.
//
// The test uses the same two-line prefix emitted by toolutil.MetaToolDescriptionPrefix
// followed by a real domain description. It asserts that the summary starts at
// the domain text, protecting README generation from regressing to unhelpful
// "Example: ..." descriptions.
func TestDescriptionSummary_StripsGeneratedMetaToolPrefix(t *testing.T) {
	description := "Example: {\"action\":\"create\",\"params\":{...}}\n" +
		"For the params schema of any action, read the MCP resource gitlab://schema/meta/gitlab_issue/<action>.\n\n" +
		"Manage GitLab issues, notes, discussions, links, statistics, and issue emoji. Delete actions are destructive."

	got := descriptionSummary(description)
	want := "Manage GitLab issues, notes, discussions, links, statistics, and issue emoji."
	if got != want {
		t.Fatalf("descriptionSummary() = %q, want %q", got, want)
	}
}

// TestDescriptionSummary_PreservesStandaloneExampleDescriptions verifies that
// standalone tool descriptions are not stripped just because they begin with an
// example sentence.
//
// The generated meta-tool prefix is only removed when both its usage-example
// line and schema-resource hint are present. This keeps normal descriptions
// intact for tools that are listed next to meta-tools in README output.
func TestDescriptionSummary_PreservesStandaloneExampleDescriptions(t *testing.T) {
	description := "Example: resolve this remote before listing projects. More details follow."

	got := descriptionSummary(description)
	want := "Example: resolve this remote before listing projects."
	if got != want {
		t.Fatalf("descriptionSummary() = %q, want %q", got, want)
	}
}

// TestDescriptionSummary_EscapesMarkdownTablePipes verifies that summaries are
// safe for Markdown table cells.
//
// The generated README table uses pipe-delimited Markdown, so any literal pipe
// in a tool description must be escaped after the summary is extracted.
func TestDescriptionSummary_EscapesMarkdownTablePipes(t *testing.T) {
	description := "Manage group | project access. Extra details follow."

	got := descriptionSummary(description)
	want := "Manage group \\| project access."
	if got != want {
		t.Fatalf("descriptionSummary() = %q, want %q", got, want)
	}
}

// TestBuildTable_UsesRealMetaToolDescription verifies that the README meta-tool
// table renders the real domain summary, not the generated schema example.
//
// The test feeds buildTable an MCP tool with a generated meta-tool prefix and a
// two-action schema. It asserts the rendered table includes the useful domain
// sentence, excludes the generated example, and keeps the action count.
func TestBuildTable_UsesRealMetaToolDescription(t *testing.T) {
	tool := &mcp.Tool{
		Name: "gitlab_issue",
		Description: "Example: {\"action\":\"create\",\"params\":{...}}\n" +
			"For the params schema of any action, read the MCP resource gitlab://schema/meta/gitlab_issue/<action>.\n\n" +
			"Manage GitLab issues, notes, discussions, links, statistics, and issue emoji. Delete actions are destructive.",
		InputSchema: map[string]any{
			"properties": map[string]any{
				"action": map[string]any{
					"enum": []any{"create", "list"},
				},
			},
		},
	}

	table := buildTable([]*mcp.Tool{tool}, []*mcp.Tool{tool})
	if !strings.Contains(table, "Manage GitLab issues, notes, discussions, links, statistics, and issue emoji.") {
		t.Fatalf("table missing real description:\n%s", table)
	}
	if strings.Contains(table, "Example:") {
		t.Fatalf("table should not include generated example prefix:\n%s", table)
	}
	if !strings.Contains(table, "| `gitlab_issue` | 2 |") {
		t.Fatalf("table missing expected action count:\n%s", table)
	}
}
