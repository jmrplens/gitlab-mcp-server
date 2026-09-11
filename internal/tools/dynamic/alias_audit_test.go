// alias_audit_test.go contains unit tests for the dynamic-tool-surface alias audit logic.
package dynamic

import (
	"context"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestAuditActionAliases_ReportsGovernanceFindings verifies that alias audit
// test data reports every governance finding type and remains deterministically
// sorted. It uses an in-memory action catalog fixture and no external services.
func TestAuditActionAliases_ReportsGovernanceFindings(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
	route := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil })
	group.SetAction(actioncatalog.Action{Name: "get", Route: route})
	group.SetAction(actioncatalog.Action{Name: "list", Route: route})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	aliases := []actionAlias{
		{Alias: "project.lookup", Canonical: "project.get"},
		{Alias: "project.lookup", Canonical: "project.get"},
		{Alias: "project.get", Canonical: "project.get"},
		{Alias: "project.list", Canonical: "project.get"},
		{Alias: "project.missing", Canonical: "project.missing"},
		{Alias: "project.compat", Canonical: "project.get", Source: aliasSourceDeprecated},
		{Alias: "project.ambiguous", Canonical: "project.get"},
		{Alias: "project.ambiguous", Canonical: "project.missing"},
	}

	findings := auditActionAliases(catalog, aliases)
	wantProblems := []string{
		"alias_equals_canonical",
		"alias_names_another_action",
		"ambiguous_compatibility_alias",
		"duplicate_alias",
		"non_canonical_target",
		"unsearchable_alias",
	}
	for _, problem := range wantProblems {
		t.Run(problem, func(t *testing.T) {
			if !slices.ContainsFunc(findings, func(finding AliasAuditFinding) bool { return finding.Problem == problem }) {
				t.Fatalf("findings = %+v, want problem %q", findings, problem)
			}
		})
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
		t.Run(ignored, func(t *testing.T) {
			if slices.ContainsFunc(findings, func(finding CatalogDiscoveryFinding) bool { return finding.ID == ignored }) {
				t.Fatalf("findings = %+v, want %s ignored because it has discovery signals", findings, ignored)
			}
		})
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

// TestAuditActionAliases_AliasNamesAnotherActionNeedsBothConditions verifies
// that an alias is reported as naming another action only when it really is
// another action's canonical ID. An alias nobody can resolve to a catalog
// action is a different finding, and an alias equal to its own canonical
// target is the self-alias finding, so widening either half of the test would
// report one of those twice under the wrong problem.
func TestAuditActionAliases_AliasNamesAnotherActionNeedsBothConditions(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
	route := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil })
	group.SetAction(actioncatalog.Action{Name: "get", Route: route})
	group.SetAction(actioncatalog.Action{Name: "list", Route: route})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	findings := auditActionAliases(catalog, []actionAlias{
		{Alias: "project.list", Canonical: "project.get"},
		{Alias: "project.get", Canonical: "project.get"},
		{Alias: "project.lookup", Canonical: "project.get"},
	})

	want := []AliasAuditFinding{
		{Severity: "error", Problem: "alias_equals_canonical", Alias: "project.get", Canonical: "project.get"},
		{Severity: "error", Problem: "alias_names_another_action", Alias: "project.list", Canonical: "project.get"},
	}
	if len(findings) != len(want) {
		t.Fatalf("auditActionAliases() returned %d findings, want %d: %+v", len(findings), len(want), findings)
	}
	for index, expected := range want {
		t.Run(expected.Problem, func(t *testing.T) {
			got := findings[index]
			if got.Severity != expected.Severity || got.Problem != expected.Problem ||
				got.Alias != expected.Alias || got.Canonical != expected.Canonical {
				t.Errorf("findings[%d] = %+v, want %+v", index, got, expected)
			}
		})
	}
}

// TestHasActionDiscoverySignal_EachSignalCountsOnItsOwn verifies that any one
// searchable signal is enough to keep an action out of the weak-metadata
// report. The check is a chain of alternatives and each link matters on its
// own: an action carrying nothing but an enum value is still findable by that
// value, and demanding a second signal beside it would put every such action
// on a work list that has nothing to add.
func TestHasActionDiscoverySignal_EachSignalCountsOnItsOwn(t *testing.T) {
	const id = "zulu.weak_action"

	tests := []struct {
		name  string
		entry actionEntry
		want  bool
	}{
		{name: "nothing beyond the canonical id", entry: actionEntry{ID: id}},
		{name: "alias", entry: actionEntry{ID: id, Aliases: []string{"zulu list"}}, want: true},
		{name: "tag", entry: actionEntry{ID: id, Tags: []string{"zulu"}}, want: true},
		{name: "usage hint", entry: actionEntry{ID: id, Usage: "Use for the zulu resource."}, want: true},
		{name: "related action", entry: actionEntry{ID: id, RelatedActions: []string{"zulu.other"}}, want: true},
		{name: "required param", entry: actionEntry{ID: id, RequiredParams: []string{"project_id"}}, want: true},
		{name: "optional param", entry: actionEntry{ID: id, Document: searchDocument{OptionalParams: []string{"state"}}}, want: true},
		{name: "schema property", entry: actionEntry{ID: id, Document: searchDocument{SchemaProperties: []string{"state"}}}, want: true},
		{name: "schema enum", entry: actionEntry{ID: id, Document: searchDocument{SchemaEnums: []string{"opened"}}}, want: true},
		{name: "schema description term", entry: actionEntry{ID: id, Document: searchDocument{SchemaDescTerms: []string{"milestone"}}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasActionDiscoverySignal(tt.entry); got != tt.want {
				t.Errorf("hasActionDiscoverySignal(%+v) = %v, want %v", tt.entry, got, tt.want)
			}
		})
	}
}

// TestAuditRegistryDiscoveryTerms_OrdersByToolThenID verifies that the report
// is grouped by the tool a reader would open and ordered by canonical ID
// inside it, and that a group below the density threshold is left out of it
// entirely. The fixture separates the two keys on purpose: its domains
// contradict its tool names, so an order taken from the canonical ID alone
// would interleave the two groups and scatter the work list a maintainer is
// meant to walk one tool at a time.
func TestAuditRegistryDiscoveryTerms_OrdersByToolThenID(t *testing.T) {
	weakRoute := toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil })
	catalog := actioncatalog.NewCatalog()
	// sequential: the two groups one fixture is built from, not cases of their own
	for _, dense := range []struct {
		tool         string
		firstDomain  string
		secondDomain string
	}{
		{tool: "gitlab_alpha", firstDomain: "zulu", secondDomain: "mike"},
		{tool: "gitlab_zulu", firstDomain: "alpha", secondDomain: "alpha"},
	} {
		group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: dense.tool})
		for index := range 8 {
			domain := dense.firstDomain
			if index >= 4 {
				domain = dense.secondDomain
			}
			group.SetAction(actioncatalog.Action{
				Name:   "weak_action_" + string(rune('a'+index)),
				Domain: domain,
				Route:  weakRoute,
			})
		}
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup(%s) error = %v", dense.tool, err)
		}
	}
	sparse := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_sparse"})
	sparse.SetAction(actioncatalog.Action{Name: "weak_action_a", Domain: "sparse", Route: weakRoute})
	if err := catalog.AddGroup(sparse); err != nil {
		t.Fatalf("AddGroup(gitlab_sparse) error = %v", err)
	}

	findings := AuditRegistryDiscoveryTerms(NewRegistryFromCatalog(catalog))

	want := []CatalogDiscoveryFinding{
		{Tool: "gitlab_alpha", ID: "mike.weak_action_e"},
		{Tool: "gitlab_alpha", ID: "mike.weak_action_f"},
		{Tool: "gitlab_alpha", ID: "mike.weak_action_g"},
		{Tool: "gitlab_alpha", ID: "mike.weak_action_h"},
		{Tool: "gitlab_alpha", ID: "zulu.weak_action_a"},
		{Tool: "gitlab_alpha", ID: "zulu.weak_action_b"},
		{Tool: "gitlab_alpha", ID: "zulu.weak_action_c"},
		{Tool: "gitlab_alpha", ID: "zulu.weak_action_d"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_a"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_b"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_c"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_d"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_e"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_f"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_g"},
		{Tool: "gitlab_zulu", ID: "alpha.weak_action_h"},
	}
	if len(findings) != len(want) {
		t.Fatalf("AuditRegistryDiscoveryTerms() returned %d findings, want %d: %+v", len(findings), len(want), findings)
	}
	// sequential: the claim is the order of the whole list, one assertion in steps
	for index, expected := range want {
		if findings[index].Tool != expected.Tool || findings[index].ID != expected.ID {
			t.Errorf("findings[%d] = %s/%s, want %s/%s", index, findings[index].Tool, findings[index].ID, expected.Tool, expected.ID)
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
