// main_test.go contains focused tests for the audit_metrics command. Tests use
// an httptest GitLab version endpoint so MCP resource registration can be
// inspected without external credentials.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// newAuditMetricsClient creates a [gitlabclient.Client] backed by a mock
// /api/v4/version endpoint for audit_metrics tests.
func newAuditMetricsClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	t.Cleanup(srv.Close)

	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "audit-token"})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	return client
}

// TestCountResources_IncludesToolManifest verifies resource metrics include the
// surface-aware tool manifest registration path used by the audit command.
func TestCountResources_IncludesToolManifest(t *testing.T) {
	static, templates := countResources(newAuditMetricsClient(t))
	if static == 0 {
		t.Fatal("countResources() static = 0, want registered resources")
	}
	if templates == 0 {
		t.Fatal("countResources() templates = 0, want registered templates")
	}
}

// TestListDynamicTools_ExposesTwoTools verifies audit metrics count the
// dynamic public surface independently from catalog action volume.
func TestListDynamicTools_ExposesTwoTools(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get": {Handler: func(_ context.Context, _ map[string]any) (any, error) { return map[string]any{"ok": true}, nil }},
		},
	}

	dynamicTools := listDynamicTools(actioncatalog.FromActionMaps(routes))
	if len(dynamicTools) != 2 {
		t.Fatalf("listDynamicTools() count = %d, want 2", len(dynamicTools))
	}
	names := make([]string, 0, len(dynamicTools))
	for _, tool := range dynamicTools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	for _, want := range []string{"gitlab_execute_action", "gitlab_find_action"} {
		if !slices.Contains(names, want) {
			t.Fatalf("listDynamicTools() names = %v, missing %q", names, want)
		}
	}
}

// TestCountActionRoutes_CountsCatalogActions verifies catalog route counting is
// independent from MCP tool advertisement.
func TestCountActionRoutes_CountsCatalogActions(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {"get": {}, "list": {}},
		"gitlab_issue":   {"create": {}},
	}

	if got := countActionRoutes(routes); got != 3 {
		t.Fatalf("countActionRoutes() = %d, want 3", got)
	}
}

// TestCountToolPackages_ReportsCatalogFirstPackages verifies package metrics do
// not depend on the removed package-local register.go convention.
func TestCountToolPackages_ReportsCatalogFirstPackages(t *testing.T) {
	toolsDir := filepath.Join(repositoryRoot(), "internal", "tools")
	want := countToolPackageDirsAt(toolsDir)
	if want == 0 {
		t.Fatalf("countToolPackageDirsAt(%s) = 0, want registered tool packages", toolsDir)
	}
	if got := countToolPackages(); got != want {
		t.Fatalf("countToolPackages() = %d, want %d", got, want)
	}
}

// TestCountToolPackageDirsAt_IncludesPackagesWithoutRegisterGo verifies the
// filesystem fallback counts Go packages even when no register.go file exists.
func TestCountToolPackageDirsAt_IncludesPackagesWithoutRegisterGo(t *testing.T) {
	toolsDir := t.TempDir()
	writeTestFile(t, toolsDir, "root.go")
	writeTestFile(t, filepath.Join(toolsDir, "alpha"), "alpha.go")
	writeTestFile(t, filepath.Join(toolsDir, "beta"), "beta_test.go")
	writeTestFile(t, filepath.Join(toolsDir, "nested", "gamma"), "gamma.go")
	if err := os.Mkdir(filepath.Join(toolsDir, "empty"), 0o750); err != nil {
		t.Fatalf("Mkdir(empty): %v", err)
	}

	if got := countToolPackageDirsAt(toolsDir); got != 4 {
		t.Fatalf("countToolPackageDirsAt() = %d, want 4", got)
	}
}

// TestCountCatalogDomains_UsesCanonicalActionDomains verifies domain metrics are
// based on Action.Domain rather than individual tool name segments.
func TestCountCatalogDomains_UsesCanonicalActionDomains(t *testing.T) {
	catalog := catalogWithActions(
		t,
		catalogActionFixture{toolName: "gitlab_project", actionName: "get", specBacked: true},
		catalogActionFixture{toolName: "gitlab_project", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_issue", actionName: "get", specBacked: true},
	)

	domains := countCatalogDomains(catalog)
	if domains["project"] != 2 {
		t.Fatalf("domains[project] = %d, want 2", domains["project"])
	}
	if domains["issue"] != 1 {
		t.Fatalf("domains[issue] = %d, want 1", domains["issue"])
	}
}

// TestDynamicSearchMetrics_ReportsIndexAndAliasCounts verifies static dynamic
// search metrics are available without adding visible MCP tools.
func TestDynamicSearchMetrics_ReportsIndexAndAliasCounts(t *testing.T) {
	catalog := dynamicActionCatalog(newAuditMetricsClient(t), false)

	metrics := dynamicSearchMetrics(catalog)
	if metrics.ActionCount == 0 {
		t.Fatal("ActionCount is zero, want catalog actions")
	}
	if metrics.IndexTokenCount == 0 || metrics.IndexPostingCount == 0 {
		t.Fatalf("metrics = %+v, want populated search index metrics", metrics)
	}
	if metrics.AliasCount == 0 || metrics.SearchableAliasCount == 0 {
		t.Fatalf("metrics = %+v, want alias metrics", metrics)
	}
	if metrics.UnsearchableAliasCount == 0 {
		t.Fatalf("metrics = %+v, want non-zero unsearchable alias count", metrics)
	}
	if len(listDynamicTools(catalog)) != 2 {
		t.Fatal("dynamic metrics changed advertised dynamic tool count")
	}
}

// TestPrintDynamicSearchMetrics_IncludesAllSurfaces verifies the audit report
// prints dynamic index and alias rows for base, self-managed enterprise, and
// GitLab.com enterprise surfaces.
func TestPrintDynamicSearchMetrics_IncludesAllSurfaces(t *testing.T) {
	base := dynamictools.RegistryMetrics{IndexTokenCount: 1, IndexPostingCount: 2, AliasCount: 3, SearchableAliasCount: 4, UnsearchableAliasCount: 5, AmbiguousAliasCount: 6}
	enterprise := dynamictools.RegistryMetrics{IndexTokenCount: 7, IndexPostingCount: 8, AliasCount: 9, SearchableAliasCount: 10, UnsearchableAliasCount: 11, AmbiguousAliasCount: 12}
	gitLabCom := dynamictools.RegistryMetrics{IndexTokenCount: 13, IndexPostingCount: 14, AliasCount: 15, SearchableAliasCount: 16, UnsearchableAliasCount: 17, AmbiguousAliasCount: 18}

	output := captureStdout(t, func() {
		printDynamicSearchMetrics(base, enterprise, gitLabCom)
	})
	for _, want := range []string{
		"Dynamic search index tokens (base)",
		"Dynamic search index tokens (self-managed enterprise)",
		"Dynamic search index tokens (GitLab.com enterprise)",
		"Dynamic search index postings (GitLab.com enterprise)",
		"Dynamic aliases (GitLab.com enterprise)",
		"Dynamic aliases searchable (GitLab.com enterprise)",
		"Dynamic aliases unsearchable (GitLab.com enterprise)",
		"Dynamic aliases ambiguous (GitLab.com enterprise)",
		"18",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printDynamicSearchMetrics() output missing %q:\n%s", want, output)
		}
	}
}

// TestAuditEnterpriseActionSpecs_ClassifiesEnterpriseDelta verifies the audit
// separates spec-backed enterprise actions from actions missing ActionSpecs.
func TestAuditEnterpriseActionSpecs_ClassifiesEnterpriseDelta(t *testing.T) {
	base := catalogWithActions(t, catalogActionFixture{toolName: "gitlab_project", actionName: "list", specBacked: true})
	selfManagedEnterprise := catalogWithActions(
		t,
		catalogActionFixture{toolName: "gitlab_project", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_geo", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_missing_spec", actionName: "list"},
	)
	gitLabComEnterprise := catalogWithActions(
		t,
		catalogActionFixture{toolName: "gitlab_project", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_geo", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_orbit", actionName: "status", specBacked: true},
	)

	audit := auditEnterpriseActionSpecs(base, selfManagedEnterprise, gitLabComEnterprise)
	if !slices.Equal(audit.SpecBacked, []string{"geo.list", "orbit.status"}) {
		t.Fatalf("SpecBacked = %v, want [geo.list orbit.status]", audit.SpecBacked)
	}
	if !slices.Equal(audit.MissingSpec, []string{"missing_spec.list"}) {
		t.Fatalf("MissingSpec = %v, want [missing_spec.list]", audit.MissingSpec)
	}
}

// TestAuditEnterpriseActionSpecs_RealCatalogHasNoLegacyRoutes verifies phase 7
// completion: every enterprise-only dynamic catalog action is spec-backed.
func TestAuditEnterpriseActionSpecs_RealCatalogHasNoLegacyRoutes(t *testing.T) {
	selfManagedClient := newAuditMetricsClient(t)
	gitLabComClient, err := gitlabclient.NewClientWithToken(config.DefaultGitLabURL, "audit-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken(gitlab.com) error: %v", err)
	}

	audit := auditEnterpriseActionSpecs(
		dynamicActionCatalog(selfManagedClient, false),
		dynamicActionCatalog(selfManagedClient, true),
		dynamicActionCatalog(gitLabComClient, true),
	)
	if len(audit.MissingSpec) != 0 {
		t.Fatalf("MissingSpec = %v, want none", audit.MissingSpec)
	}
	if len(audit.SpecBacked) == 0 {
		t.Fatal("SpecBacked is empty, want enterprise actions")
	}
	if !slices.Contains(audit.SpecBacked, "orbit.status") {
		t.Fatalf("SpecBacked = %v, want orbit.status", audit.SpecBacked)
	}
}

// TestPrintEnterpriseActionSpecAudit_IncludesMissingSpecZeroSection verifies the
// audit output includes both lists, including the explicit zero missing-spec state.
func TestPrintEnterpriseActionSpecAudit_IncludesMissingSpecZeroSection(t *testing.T) {
	output := captureStdout(t, func() {
		printEnterpriseActionSpecAudit(enterpriseActionSpecAudit{SpecBacked: []string{"geo.list"}})
	})
	for _, want := range []string{
		"Enterprise ActionSpec Audit",
		"Spec-backed enterprise actions (1)",
		"geo.list",
		"Enterprise actions missing ActionSpec (0)",
		"none",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printEnterpriseActionSpecAudit() output missing %q:\n%s", want, output)
		}
	}
}

type catalogActionFixture struct {
	toolName   string
	actionName string
	specBacked bool
}

func catalogWithActions(t *testing.T, fixtures ...catalogActionFixture) *actioncatalog.Catalog {
	t.Helper()
	groups := map[string]actioncatalog.Group{}
	for _, fixture := range fixtures {
		group := groups[fixture.toolName]
		if group.ToolName == "" {
			group = actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: fixture.toolName})
		}
		group.SetAction(actioncatalog.Action{Name: fixture.actionName, SpecBacked: fixture.specBacked})
		groups[fixture.toolName] = group
	}
	catalog := actioncatalog.NewCatalog()
	for _, group := range groups {
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup(%s) error: %v", group.ToolName, err)
		}
	}
	return catalog
}

func writeTestFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("package fixture\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = oldStdout })

	fn()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error: %v", closeErr)
	}
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error: %v", err)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error: %v", closeErr)
	}
	return string(output)
}

// TestDiffByName_CountsOnlyUniqueDifferences verifies the diff helper returns
// the number of names in a missing from b, ignoring duplicates and overlap.
func TestDiffByName_CountsOnlyUniqueDifferences(t *testing.T) {
	a := []*mcp.Tool{
		{Name: "gitlab_x"},
		{Name: "gitlab_y"},
		{Name: "gitlab_z"},
		{Name: "gitlab_x"}, // duplicate
	}
	b := []*mcp.Tool{
		{Name: "gitlab_x"},
		{Name: "gitlab_w"},
	}

	if got := diffByName(a, b); got != 2 {
		t.Fatalf("diffByName() = %d, want 2 (y and z)", got)
	}
}

// TestDiffByName_EmptyInputsReturnZero verifies the diff helper returns zero
// for empty inputs.
func TestDiffByName_EmptyInputsReturnZero(t *testing.T) {
	if got := diffByName(nil, nil); got != 0 {
		t.Fatalf("diffByName(nil, nil) = %d, want 0", got)
	}
	if got := diffByName([]*mcp.Tool{{Name: "a"}}, nil); got != 1 {
		t.Fatalf("diffByName([a], nil) = %d, want 1", got)
	}
	if got := diffByName(nil, []*mcp.Tool{{Name: "a"}}); got != 0 {
		t.Fatalf("diffByName(nil, [a]) = %d, want 0", got)
	}
}

// TestPrintRow_FormatsWithLabelAndValue verifies the metric row printer uses
// the configured label width and a trailing newline.
func TestPrintRow_FormatsWithLabelAndValue(t *testing.T) {
	output := captureStdout(t, func() {
		printRow("Test metric", 42)
	})

	// Padding to metricLabelWidth characters followed by "42".
	if !strings.Contains(output, "  Test metric") {
		t.Fatalf("printRow() missing padded label:\n%q", output)
	}
	if !strings.Contains(output, "42\n") {
		t.Fatalf("printRow() missing value and newline:\n%q", output)
	}
	if !strings.HasPrefix(output, "  ") {
		t.Fatalf("printRow() output should start with two-space indent: %q", output)
	}
}

// TestPrintActionIDList_EmptyListWritesNone verifies the empty list path
// writes the "none" marker instead of a loop.
func TestPrintActionIDList_EmptyListWritesNone(t *testing.T) {
	output := captureStdout(t, func() {
		printActionIDList(nil)
	})

	if !strings.Contains(output, "- none") {
		t.Fatalf("printActionIDList() empty output = %q, want '- none'", output)
	}
}

// TestPrintActionIDList_EmptySliceWritesNone verifies the empty slice path
// (different from nil) also writes the "none" marker.
func TestPrintActionIDList_EmptySliceWritesNone(t *testing.T) {
	output := captureStdout(t, func() {
		printActionIDList([]string{})
	})

	if !strings.Contains(output, "- none") {
		t.Fatalf("printActionIDList() empty slice output = %q, want '- none'", output)
	}
}

// TestPrintActionIDList_ListsAllIDs verifies populated input renders each ID
// in order using the tool list format.
func TestPrintActionIDList_ListsAllIDs(t *testing.T) {
	output := captureStdout(t, func() {
		printActionIDList([]string{"alpha.list", "beta.get", "gamma.create"})
	})

	for _, want := range []string{"alpha.list", "beta.get", "gamma.create"} {
		if !strings.Contains(output, want) {
			t.Fatalf("printActionIDList() missing %q:\n%s", want, output)
		}
	}
}

// TestCountPrompts_ReturnsRegisteredPromptCount verifies the prompt counter
// returns a positive count for the live registration path.
func TestCountPrompts_ReturnsRegisteredPromptCount(t *testing.T) {
	got := countPrompts(newAuditMetricsClient(t))
	if got <= 0 {
		t.Fatalf("countPrompts() = %d, want positive registered prompt count", got)
	}
}

// TestCountSourceFiles_CountsGoFilesUnderInternal verifies the source/test
// file counters partition .go files by _test.go suffix.
func TestCountSourceFiles_CountsGoFilesUnderInternal(t *testing.T) {
	src, test := countSourceFiles()
	if src <= 0 {
		t.Fatalf("countSourceFiles() src = %d, want positive", src)
	}
	if test <= 0 {
		t.Fatalf("countSourceFiles() test = %d, want positive", test)
	}
}

// TestPrintDomainTable_LimitsToTop20AndShowsEllipsis verifies the table
// printer sorts entries by count and shows the ... overflow message.
func TestPrintDomainTable_LimitsToTop20AndShowsEllipsis(t *testing.T) {
	domains := map[string]int{}
	for i := range 25 {
		domains[fmt.Sprintf("domain%02d", i)] = 25 - i
	}

	output := captureStdout(t, func() {
		printDomainTable(domains)
	})

	// First domain alphabetically among highest count should be domain00 (25).
	if !strings.Contains(output, "domain00") {
		t.Fatalf("printDomainTable() missing highest-count row:\n%s", output)
	}
	// The table caps at 20 rows; the rest should be summarized.
	if !strings.Contains(output, "and 5 more domains") {
		t.Fatalf("printDomainTable() missing overflow line:\n%s", output)
	}
}

// TestPrintDomainTable_FewerThan20DomainsPrintsAll verifies the table
// includes all entries when below the 20-row cap.
func TestPrintDomainTable_FewerThan20DomainsPrintsAll(t *testing.T) {
	domains := map[string]int{
		"alpha": 3,
		"beta":  1,
		"gamma": 2,
	}

	output := captureStdout(t, func() {
		printDomainTable(domains)
	})

	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(output, want) {
			t.Fatalf("printDomainTable() missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "and") {
		t.Fatalf("printDomainTable() should not show overflow:\n%s", output)
	}
}

// TestListServerTools_IndividualAndMetaReturnsPopulatedLists verifies both
// surface modes register a non-empty tool list through the in-memory server.
func TestListServerTools_IndividualAndMetaReturnsPopulatedLists(t *testing.T) {
	client := newAuditMetricsClient(t)

	individual := listServerTools(client, false, false)
	if len(individual) == 0 {
		t.Fatal("listServerTools(individual) = 0, want registered tools")
	}
	meta := listServerTools(client, true, false)
	if len(meta) == 0 {
		t.Fatal("listServerTools(meta) = 0, want registered meta tools")
	}
	metaEnterprise := listServerTools(client, true, true)
	if len(metaEnterprise) < len(meta) {
		t.Fatalf("listServerTools(enterprise meta) = %d, want >= %d", len(metaEnterprise), len(meta))
	}
}

// TestPrintMetaSchemaModes_ListsActiveAndAllModes verifies the schema-mode
// reporter prints the active mode and the three documented modes.
func TestPrintMetaSchemaModes_ListsActiveAndAllModes(t *testing.T) {
	// The reporter resets the mode back to "opaque" on exit; capture the
	// current state to restore it after the test runs.
	t.Setenv("META_PARAM_SCHEMA", "compact")

	client := newAuditMetricsClient(t)
	output := captureStdout(t, func() {
		printMetaSchemaModes(client)
	})

	for _, want := range []string{
		"Active mode (env): compact",
		"opaque",
		"compact",
		"full",
		"mode",
		"total bytes",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printMetaSchemaModes() missing %q:\n%s", want, output)
		}
	}
}

// TestPrintMetaSchemaModes_DefaultsToOpaqueWhenUnset verifies the reporter
// falls back to opaque mode when META_PARAM_SCHEMA is empty or invalid.
func TestPrintMetaSchemaModes_DefaultsToOpaqueWhenUnset(t *testing.T) {
	t.Setenv("META_PARAM_SCHEMA", "bogus")

	output := captureStdout(t, func() {
		printMetaSchemaModes(newAuditMetricsClient(t))
	})

	if !strings.Contains(output, "Active mode (env): opaque") {
		t.Fatalf("printMetaSchemaModes() did not default to opaque:\n%s", output)
	}
}
