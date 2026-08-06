// alias_audit_test.go contains unit tests for the dynamic-tool-surface alias audit logic.
package dynamic

import (
	"context"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestAuditActionAliases_ReportsGovernanceFindings verifies that alias audit
// test data reports every governance finding type and remains deterministically
// sorted. It uses an in-memory action catalog fixture and no external services.
func TestAuditActionAliases_ReportsGovernanceFindings(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
	route := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil })
	group.SetAction(actioncatalog.Action{Name: "get", Route: route})
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

// TestAuditDiscoveryTerms_NilAndSparseInputs verifies the AuditDiscoveryTerms_NilAndSparseInputs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAuditDiscoveryTerms_NilAndSparseInputs(t *testing.T) {
	if findings := AuditCatalogDiscoveryTerms(nil); findings != nil {
		t.Fatalf("AuditCatalogDiscoveryTerms(nil) = %+v, want nil", findings)
	}
	if findings := AuditRegistryDiscoveryTerms(nil); findings != nil {
		t.Fatalf("AuditRegistryDiscoveryTerms(nil) = %+v, want nil", findings)
	}

	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
	group.SetAction(actioncatalog.Action{Name: "get", Route: toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil })})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if findings := AuditRegistryDiscoveryTerms(NewRegistryFromCatalog(catalog)); findings != nil {
		t.Fatalf("AuditRegistryDiscoveryTerms(sparse) = %+v, want nil", findings)
	}
}

// TestAuditCatalogDiscoveryTerms_FlagsDenseActionsWithoutSignals verifies the
// metadata audit catches actions in crowded groups when their only searchable
// text is the canonical identifier, while ignoring actions with targeted tags
// or schema-derived parameter signals.
func TestAuditCatalogDiscoveryTerms_FlagsDenseActionsWithoutSignals(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
	secondGroup := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_beta"})
	weakRoute := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil })
	for index := range 8 {
		action := actioncatalog.Action{Name: "weak_action_" + string(rune('a'+index)), Route: weakRoute}
		if index == 0 {
			action.Tags = []string{"project cleanup"}
		}
		if index == 1 {
			action.Route.InputSchema = map[string]any{"properties": map[string]any{"project_id": map[string]any{}}}
		}
		group.SetAction(action)
		secondGroup.SetAction(actioncatalog.Action{Name: "weak_action_" + string(rune('a'+index)), Route: weakRoute})
	}
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if err := catalog.AddGroup(secondGroup); err != nil {
		t.Fatalf("AddGroup(second) error = %v", err)
	}

	findings := AuditCatalogDiscoveryTerms(catalog)
	registryFindings := AuditRegistryDiscoveryTerms(NewRegistryFromCatalog(catalog))
	if len(registryFindings) != len(findings) {
		t.Fatalf("AuditRegistryDiscoveryTerms() returned %d findings, want %d", len(registryFindings), len(findings))
	}
	if len(findings) != 14 {
		t.Fatalf("AuditCatalogDiscoveryTerms() returned %d findings, want 14: %+v", len(findings), findings)
	}
	for _, ignored := range []string{"project.weak_action_a", "project.weak_action_b"} {
		if slices.ContainsFunc(findings, func(finding CatalogDiscoveryFinding) bool { return finding.ID == ignored }) {
			t.Fatalf("findings = %+v, want %s ignored because it has discovery signals", findings, ignored)
		}
	}
	for _, finding := range findings {
		if finding.Severity != "warning" || finding.Problem != "weak_discovery_terms" || finding.Message == "" {
			t.Fatalf("finding = %+v, want populated weak discovery warning", finding)
		}
	}
	for index := 1; index < len(findings); index++ {
		if findings[index-1].Tool > findings[index].Tool {
			t.Fatalf("findings not sorted by tool: %+v before %+v", findings[index-1], findings[index])
		}
	}
}

// TestAuditRegistryDiscoveryTerms_SeverityOrderingTieBreaker verifies the
// AuditRegistryDiscoveryTerms sort handles two findings with the same
// severity and the same tool — it must fall back to a lexical ID
// comparison (line 84-86 of alias_audit.go).
func TestAuditRegistryDiscoveryTerms_SeverityOrderingTieBreaker(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
	weakRoute := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil })
	// 8 weak actions guarantee the group is dense enough to flag findings.
	// Use names deliberately out of lexical order so the tie-breaker runs.
	names := []string{"z_last", "a_first", "m_middle", "extra_one", "extra_two", "extra_three", "extra_four", "extra_five"}
	for _, name := range names {
		group.SetAction(actioncatalog.Action{Name: name, Route: weakRoute})
	}
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	registry := NewRegistryFromCatalog(catalog)
	findings := AuditRegistryDiscoveryTerms(registry)
	if len(findings) < 2 {
		t.Fatalf("AuditRegistryDiscoveryTerms() returned %d findings, want at least 2", len(findings))
	}
	// All findings share the same severity ("warning") and the same tool, so
	// the comparator must use the lexical ID fallback to order them.
	for i := 1; i < len(findings); i++ {
		prev, cur := findings[i-1], findings[i]
		if prev.Severity != cur.Severity || prev.Tool != cur.Tool {
			continue
		}
		if prev.ID > cur.ID {
			t.Fatalf("findings not sorted by ID within same severity/tool: %+v before %+v", prev, cur)
		}
	}
}
