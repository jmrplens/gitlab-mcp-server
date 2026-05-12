// Package dynamic contains tests for dynamic alias governance auditing,
// including duplicate mappings, ambiguous compatibility aliases, and
// catalog-target validation behavior.
package dynamic

import (
	"context"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestAuditActionAliases_ReportsGovernanceFindings verifies that alias audit
// test data reports every governance finding type and remains deterministically
// sorted. It uses an in-memory action catalog fixture and no external services.
func TestAuditActionAliases_ReportsGovernanceFindings(t *testing.T) {
	catalog := actionregistry.NewCatalog()
	group := actionregistry.NewGroup(actionregistry.GroupOptions{ToolName: "gitlab_project"})
	route := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return nil, nil })
	group.SetAction(actionregistry.Action{Name: "get", Route: route})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	aliases := []actionAlias{
		{Alias: "project.lookup", Canonical: "project.get"},
		{Alias: "project.lookup", Canonical: "project.get"},
		{Alias: "project.get", Canonical: "project.get"},
		{Alias: "project.missing", Canonical: "project.missing"},
		{Alias: "project.compat", Canonical: "project.get", Source: aliasSourceDeprecated},
		{Alias: "project.ambiguous", Canonical: "project.get"},
		{Alias: "project.ambiguous", Canonical: "project.missing"},
	}

	findings := auditActionAliases(catalog, aliases)
	wantProblems := []string{
		"alias_equals_canonical",
		"ambiguous_compatibility_alias",
		"duplicate_alias",
		"non_canonical_target",
		"unsearchable_alias",
	}
	for _, problem := range wantProblems {
		if !slices.ContainsFunc(findings, func(finding AliasAuditFinding) bool { return finding.Problem == problem }) {
			t.Fatalf("findings = %+v, want problem %q", findings, problem)
		}
	}

	for index := 1; index < len(findings); index++ {
		previous := findings[index-1]
		current := findings[index]
		if previous.Severity > current.Severity ||
			(previous.Severity == current.Severity && previous.Problem > current.Problem) ||
			(previous.Severity == current.Severity && previous.Problem == current.Problem && previous.Alias > current.Alias) {
			t.Fatalf("findings not sorted at %d: %+v before %+v", index, previous, current)
		}
	}
}

// TestAuditDefaultActionAliases_ReturnsOnlyExpectedDefaultFindings verifies the
// default alias audit behavior when no catalog is available. It expects only
// informational unsearchable-alias findings with populated source metadata.
func TestAuditDefaultActionAliases_ReturnsOnlyExpectedDefaultFindings(t *testing.T) {
	findings := AuditDefaultActionAliases(nil)
	if len(findings) == 0 {
		t.Fatal("AuditDefaultActionAliases(nil) returned no findings; want informational unsearchable aliases")
	}
	for _, finding := range findings {
		if finding.Severity != "info" || finding.Problem != "unsearchable_alias" {
			t.Fatalf("finding = %+v, want only informational unsearchable aliases with nil catalog", finding)
		}
		if finding.Source == "" || finding.Message == "" {
			t.Fatalf("finding = %+v, want source and message", finding)
		}
	}
}
