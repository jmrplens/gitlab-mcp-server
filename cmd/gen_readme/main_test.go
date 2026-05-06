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
// envelope/example descriptions.
func TestDescriptionSummary_StripsGeneratedMetaToolPrefix(t *testing.T) {
	description := "Use {\"action\":\"create\",\"params\":{...}}; only top-level keys are action and params.\n" +
		"Action params schema: gitlab://schema/meta/gitlab_issue/<action>.\n\n" +
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
	tool := testMetaTool("gitlab_issue", "Manage GitLab issues, notes, discussions, links, statistics, and issue emoji. Delete actions are destructive.", "create", "list")

	table := buildTable([]*mcp.Tool{tool}, []*mcp.Tool{tool}, []*mcp.Tool{tool})
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

func TestBuildTable_IncludesEnterpriseUnionAndPrefersGitLabCom(t *testing.T) {
	baseTool := testMetaTool("gitlab_issue", "Manage GitLab issues.", "list")
	selfManagedOnly := testMetaTool("gitlab_geo", "Manage self-managed Geo replication.", "list")
	selfManagedShared := testMetaTool("gitlab_dependency", "Self-managed dependency description.", "list")
	gitLabComShared := testMetaTool("gitlab_dependency", "GitLab.com dependency description.", "list", "get")
	gitLabComOnly := testMetaTool("gitlab_orbit", "Query GitLab.com Orbit.", "status", "query")

	table := buildTable(
		[]*mcp.Tool{baseTool},
		[]*mcp.Tool{baseTool, selfManagedOnly, selfManagedShared},
		[]*mcp.Tool{baseTool, gitLabComShared, gitLabComOnly},
	)

	for _, want := range []string{
		"| `gitlab_issue` | 1 | Manage GitLab issues. |",
		"| `gitlab_geo` 🏢 | 1 | Manage self-managed Geo replication. |",
		"| `gitlab_dependency` 🏢 | 2 | GitLab.com dependency description. |",
		"| `gitlab_orbit` 🏢 | 2 | Query GitLab.com Orbit. |",
		"**1 base** / **3 self-managed enterprise** / **3 GitLab.com Enterprise** meta-tools.",
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing %q:\n%s", want, table)
		}
	}
	if strings.Contains(table, "Self-managed dependency description") {
		t.Fatalf("table should prefer the GitLab.com definition for shared enterprise tools:\n%s", table)
	}
}

func testMetaTool(name, description string, actions ...string) *mcp.Tool {
	return &mcp.Tool{
		Name: name,
		Description: "Use {\"action\":\"create\",\"params\":{...}}; only top-level keys are action and params.\n" +
			"Action params schema: gitlab://schema/meta/" + name + "/<action>.\n\n" +
			description,
		InputSchema: map[string]any{
			"properties": map[string]any{
				"action": map[string]any{
					"enum": actions,
				},
			},
		},
	}
}
