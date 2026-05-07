// main_test.go verifies README generation helpers used by cmd/gen_readme.
package main

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestDescriptionSummary_TableDriven verifies summary extraction for generated
// meta-tool prefixes, standalone examples, and Markdown table escaping.
func TestDescriptionSummary_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "strips generated meta-tool prefix",
			description: testMetaTool("gitlab_issue", "Manage GitLab issues, notes, discussions, links, statistics, and issue emoji. Delete actions are destructive.", "create", "list").Description,
			want:        "Manage GitLab issues, notes, discussions, links, statistics, and issue emoji.",
		},
		{
			name:        "preserves standalone examples",
			description: "Example: resolve this remote before listing projects. More details follow.",
			want:        "Example: resolve this remote before listing projects.",
		},
		{
			name:        "escapes Markdown table pipes",
			description: "Manage group | project access. Extra details follow.",
			want:        "Manage group \\| project access.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := descriptionSummary(tt.description)
			if got != tt.want {
				t.Fatalf("descriptionSummary() = %q, want %q", got, tt.want)
			}
		})
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
	routes := make(toolutil.ActionMap, len(actions))
	for _, action := range actions {
		routes[action] = toolutil.Route(nil)
	}

	return &mcp.Tool{
		Name:        name,
		Description: toolutil.MetaToolDescriptionPrefix(name, routes) + description,
		InputSchema: map[string]any{
			"properties": map[string]any{
				"action": map[string]any{
					"enum": actions,
				},
			},
		},
	}
}
