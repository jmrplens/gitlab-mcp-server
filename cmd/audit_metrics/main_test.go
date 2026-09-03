// main_test.go contains focused tests for the audit_metrics command. Tests use
// an httptest GitLab version endpoint so MCP resource registration can be
// inspected without external credentials.
//
// Coverage includes the dynamic two-tool surface, catalog-domain counting,
// search-index metrics, the enterprise-action-spec audit classification, the
// gathered metrics payload, and both report renderers (text and JSON).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// newAuditMetricsClient returns the binary-lifetime [gitlabclient.Client]
// backed by a mock /api/v4/version endpoint. Shared across tests (process
// teardown reclaims the httptest server) so listServerTools' per-client
// memo can serve every test from one surface registration, the same
// pattern cmd/server's createServer tests use.
func newAuditMetricsClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	sharedClientOnce.Do(func() {
		sharedClient, _ = auditclient.NewMock()
	})
	return sharedClient
}

// newGitLabComClient returns the binary-lifetime client that targets
// GitLab.com, the way main wires it, so the GitLab.com-only surfaces are
// registered once per process and served from the same memo afterwards.
func newGitLabComClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	gitLabComClientOnce.Do(func() {
		gitLabComClient, errGitLabComClient = gitlabclient.NewClient(&config.Config{
			GitLabURL:   config.DefaultGitLabURL,
			GitLabToken: "audit-token", //#nosec G101 -- audit-only dummy token, not a real credential
		})
	})
	if errGitLabComClient != nil {
		t.Fatalf("create gitlab.com client: %v", errGitLabComClient)
	}
	return gitLabComClient
}

// collectedMetrics returns the metrics payload main renders, gathered once
// per process from the shared clients: collecting it registers every surface
// the report counts, so tests must treat the result as read-only.
func collectedMetrics(t *testing.T) auditMetrics {
	t.Helper()
	metricsOnce.Do(func() {
		metricsShared = collectMetrics(newAuditMetricsClient(t), newGitLabComClient(t))
	})
	return metricsShared
}

var (
	sharedClientOnce    sync.Once
	sharedClient        *gitlabclient.Client
	gitLabComClientOnce sync.Once
	gitLabComClient     *gitlabclient.Client
	errGitLabComClient  error
	metricsOnce         sync.Once
	metricsShared       auditMetrics
)

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
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(names, want) {
				t.Fatalf("listDynamicTools() names = %v, missing %q", names, want)
			}
		})
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

// TestCountToolPackageDirsAt_MissingRoot_ReportsAndCountsZero verifies a root
// that cannot be walked is reported on stderr and counted as no packages
// rather than aborting the audit.
func TestCountToolPackageDirsAt_MissingRoot_ReportsAndCountsZero(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	got := 0
	stderr := captureStderr(t, func() {
		got = countToolPackageDirsAt(missing)
	})

	if got != 0 {
		t.Fatalf("countToolPackageDirsAt(missing) = %d, want 0", got)
	}
	if !strings.Contains(stderr, "WalkDir "+missing) {
		t.Fatalf("stderr = %q, want the walk failure naming %s", stderr, missing)
	}
}

// TestDirectoryHasGoFile_MissingDirectory_ReturnsFalse verifies an unreadable
// directory counts as one without Go files instead of surfacing the error.
func TestDirectoryHasGoFile_MissingDirectory_ReturnsFalse(t *testing.T) {
	if directoryHasGoFile(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("directoryHasGoFile(missing) = true, want false")
	}
}

// TestCountSourceFilesAt_MissingDirectory_ReportsWalkError verifies a
// directory that cannot be walked yields zero counts and a stderr diagnostic,
// the only failure the repository walk can produce.
func TestCountSourceFilesAt_MissingDirectory_ReportsWalkError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "internal")

	src, test := 0, 0
	stderr := captureStderr(t, func() {
		src, test = countSourceFilesAt(missing)
	})

	if src != 0 || test != 0 {
		t.Fatalf("countSourceFilesAt(missing) = (%d, %d), want (0, 0)", src, test)
	}
	if !strings.Contains(stderr, "Walk: ") {
		t.Fatalf("stderr = %q, want a Walk error", stderr)
	}
}

// TestCountSourceFilesAt_MixedTree_PartitionsByTestSuffix verifies the counter splits
// .go files by the _test.go suffix and ignores everything else.
func TestCountSourceFilesAt_MixedTree_PartitionsByTestSuffix(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go")
	writeTestFile(t, filepath.Join(dir, "pkg"), "b.go")
	writeTestFile(t, filepath.Join(dir, "pkg"), "b_test.go")
	writeTestFile(t, filepath.Join(dir, "pkg"), "README.md")

	src, test := countSourceFilesAt(dir)
	if src != 2 || test != 1 {
		t.Fatalf("countSourceFilesAt() = (%d, %d), want (2, 1)", src, test)
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

// TestCountCatalogDomains_NilCatalog_ReturnsEmpty verifies a missing catalog
// yields an empty breakdown rather than a nil dereference.
func TestCountCatalogDomains_NilCatalog_ReturnsEmpty(t *testing.T) {
	if got := countCatalogDomains(nil); len(got) != 0 {
		t.Fatalf("countCatalogDomains(nil) = %v, want empty", got)
	}
}

// TestCountCatalogDomains_ToolWithoutDomain_CountsAsUnknown verifies an action
// whose tool name carries no domain segment (so neither its Domain nor its ID
// prefix names one) is counted under "unknown" instead of an empty key.
func TestCountCatalogDomains_ToolWithoutDomain_CountsAsUnknown(t *testing.T) {
	catalog := catalogWithActions(t, catalogActionFixture{toolName: "gitlab_", actionName: "list", specBacked: true})

	domains := countCatalogDomains(catalog)
	if domains["unknown"] != 1 || len(domains) != 1 {
		t.Fatalf("countCatalogDomains() = %v, want map[unknown:1]", domains)
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Fatalf("printDynamicSearchMetrics() output missing %q:\n%s", want, output)
			}
		})
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

	audit := auditEnterpriseActionSpecs(
		dynamicActionCatalog(selfManagedClient, false),
		dynamicActionCatalog(selfManagedClient, true),
		dynamicActionCatalog(newGitLabComClient(t), true),
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Fatalf("printEnterpriseActionSpecAudit() output missing %q:\n%s", want, output)
			}
		})
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

// captureStream redirects *stream (os.Stdout or os.Stderr) into a pipe while
// fn runs and returns everything fn wrote. The pipe is drained concurrently so
// a report larger than the pipe buffer cannot block the writer; the reader
// goroutine only records, every assertion stays on the test goroutine.
func captureStream(t *testing.T, stream **os.File, fn func()) string {
	t.Helper()
	original := *stream
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	*stream = writer
	t.Cleanup(func() { *stream = original })

	var captured bytes.Buffer
	var readErr error
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, readErr = captured.ReadFrom(reader)
	}()

	fn()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error: %v", closeErr)
	}
	*stream = original
	<-drained
	if readErr != nil {
		t.Fatalf("drain pipe: %v", readErr)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error: %v", closeErr)
	}
	return captured.String()
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return captureStream(t, &os.Stdout, fn)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	return captureStream(t, &os.Stderr, fn)
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Fatalf("printActionIDList() missing %q:\n%s", want, output)
			}
		})
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Fatalf("printDomainTable() missing %q:\n%s", want, output)
			}
		})
	}
	if strings.Contains(output, "and") {
		t.Fatalf("printDomainTable() should not show overflow:\n%s", output)
	}
}

// TestPrintDomainTable_EqualCounts_SortsByName verifies domains with the same
// action count are ordered by name, so the table is stable across runs
// regardless of map iteration order.
func TestPrintDomainTable_EqualCounts_SortsByName(t *testing.T) {
	output := captureStdout(t, func() {
		printDomainTable(map[string]int{"issue": 2, "project": 2, "branch": 2})
	})

	want := "  Domain                    Tools\n" +
		"  ------------------------- -----\n" +
		"  branch                    2\n" +
		"  issue                     2\n" +
		"  project                   2\n"
	if output != want {
		t.Fatalf("printDomainTable() =\n%s\nwant\n%s", output, want)
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Fatalf("printMetaSchemaModes() missing %q:\n%s", want, output)
			}
		})
	}
}

// TestPrintMetaSchemaModes_ReportsDistinctSizesPerMode verifies that the
// schema-modes section measures each META_PARAM_SCHEMA mode on its own
// registration rather than reusing one memoized tool list for all three:
// the opaque, compact and full strategies produce input schemas of
// different sizes, so three equal totals would mean the memo returned the
// first listing to every mode.
func TestPrintMetaSchemaModes_ReportsDistinctSizesPerMode(t *testing.T) {
	t.Setenv("META_PARAM_SCHEMA", "opaque")

	output := captureStdout(t, func() {
		printMetaSchemaModes(newAuditMetricsClient(t))
	})

	totals := map[string]string{}
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "opaque", "compact", "full":
			totals[fields[0]] = fields[1]
		}
	}
	if len(totals) != 3 {
		t.Fatalf("printMetaSchemaModes() reported %d modes, want 3:\n%s", len(totals), output)
	}
	if totals["opaque"] == totals["compact"] || totals["compact"] == totals["full"] || totals["opaque"] == totals["full"] {
		t.Fatalf("printMetaSchemaModes() reported equal totals across modes, want one measurement per mode: %v\n%s", totals, output)
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

// TestTotalInputSchemaBytes_MissingAndBrokenSchemas_CountsOnlySerializable verifies the
// sizing sum counts exactly the serialized schemas: a tool without one and a
// tool whose schema cannot be marshaled contribute nothing, and a real schema
// contributes its JSON length.
func TestTotalInputSchemaBytes_MissingAndBrokenSchemas_CountsOnlySerializable(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"action": map[string]any{"type": "string"}}}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal fixture schema: %v", err)
	}

	got := totalInputSchemaBytes([]*mcp.Tool{
		{Name: "gitlab_no_schema"},
		{Name: "gitlab_broken", InputSchema: make(chan int)},
		{Name: "gitlab_project", InputSchema: schema},
	})
	if got != len(raw) {
		t.Fatalf("totalInputSchemaBytes() = %d, want %d (only the serializable schema)", got, len(raw))
	}
}

// TestCollectMetrics_RealSurfaces_AgreesWithSiteStats verifies the payload the
// text report prints is derived from the same registrations the site stats
// publish: every count both renderers share is equal, the dynamic surface is
// two tools per deployment, the elicitation count matches the interactive
// tools in the individual surface, and every enterprise-only action is
// spec-backed.
func TestCollectMetrics_RealSurfaces_AgreesWithSiteStats(t *testing.T) {
	metrics := collectedMetrics(t)
	stats := newSiteStats(t)

	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "individual tools match the Ultimate self-managed count", got: len(metrics.individualTools), want: stats.Tools.UltimateSelfManaged},
		{name: "GitLab.com individual tools match the GitLab.com count", got: len(metrics.gitLabComIndividualTools), want: stats.Tools.GitLabCom},
		{name: "base meta-tools", got: len(metrics.metaBase), want: stats.Meta.Base},
		{name: "self-managed enterprise meta-tools", got: len(metrics.metaEnterprise), want: stats.Meta.SelfManagedEnterprise},
		{name: "GitLab.com enterprise meta-tools", got: len(metrics.metaGitLabComEnterprise), want: stats.Meta.GitLabCom},
		{name: "dynamic base tools", got: len(metrics.dynamicBase), want: 2},
		{name: "dynamic enterprise tools", got: len(metrics.dynamicEnterprise), want: 2},
		{name: "dynamic GitLab.com tools", got: len(metrics.dynamicGitLabComEnterprise), want: 2},
		{name: "base catalog actions", got: metrics.dynamicBaseActions, want: stats.CatalogActions.Free},
		{name: "self-managed enterprise catalog actions", got: metrics.dynamicEnterpriseActions, want: stats.CatalogActions.SelfManagedEnterprise},
		{name: "GitLab.com enterprise catalog actions", got: metrics.dynamicGitLabComEnterpriseActions, want: stats.CatalogActions.GitLabCom},
		{name: "resources", got: metrics.staticResources + metrics.templateResources, want: stats.Resources},
		{name: "prompts", got: metrics.promptCount, want: stats.Prompts},
		{name: "tool packages", got: metrics.toolPackages, want: stats.ToolPackages},
		{name: "enterprise actions missing a spec", got: len(metrics.enterpriseActionAudit.MissingSpec), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %d, want %d", tt.got, tt.want)
			}
		})
	}

	interactive := 0
	for _, tool := range metrics.individualTools {
		if strings.HasPrefix(tool.Name, "gitlab_interactive_") {
			interactive++
		}
	}
	if interactive == 0 || metrics.elicitationCount != interactive {
		t.Fatalf("elicitationCount = %d, want the %d interactive tools of the individual surface", metrics.elicitationCount, interactive)
	}
	if metrics.dynamicBaseMetrics.ActionCount != metrics.dynamicBaseActions {
		t.Fatalf("dynamic search metrics count %d actions, want the %d catalog routes", metrics.dynamicBaseMetrics.ActionCount, metrics.dynamicBaseActions)
	}
	if metrics.gitLabComEnterpriseDomains["orbit"] == 0 {
		t.Fatalf("GitLab.com domain breakdown %v lacks the orbit domain", metrics.gitLabComEnterpriseDomains)
	}
	if metrics.srcFiles == 0 || metrics.testFiles == 0 {
		t.Fatalf("codebase counts = (%d, %d), want both positive", metrics.srcFiles, metrics.testFiles)
	}
}

// TestWriteJSONSummary_RealMetrics_EmitsDocumentedShape verifies the -json
// summary carries exactly the ten documented keys, each holding the count the
// text report prints for the same metric.
func TestWriteJSONSummary_RealMetrics_EmitsDocumentedShape(t *testing.T) {
	metrics := collectedMetrics(t)

	var out bytes.Buffer
	if err := writeJSONSummary(&out, metrics); err != nil {
		t.Fatalf("writeJSONSummary() error: %v", err)
	}
	if !strings.HasPrefix(out.String(), "{\n  \"individual_tools\": ") || !strings.HasSuffix(out.String(), "}\n") {
		t.Fatalf("writeJSONSummary() is not an indented object with a trailing newline:\n%s", out.String())
	}

	var got map[string]int
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	want := map[string]int{
		"individual_tools":   len(metrics.individualTools),
		"meta_base":          len(metrics.metaBase),
		"meta_enterprise":    len(metrics.metaEnterprise),
		"dynamic_base":       len(metrics.dynamicBase),
		"dynamic_enterprise": len(metrics.dynamicEnterprise),
		"resources":          metrics.staticResources + metrics.templateResources,
		"prompts":            metrics.promptCount,
		"tool_packages":      metrics.toolPackages,
		"source_files":       metrics.srcFiles,
		"test_files":         metrics.testFiles,
	}
	if len(got) != len(want) {
		t.Fatalf("summary keys = %v, want exactly %v", got, want)
	}
	for key, wantValue := range want {
		t.Run(key, func(t *testing.T) {
			gotValue, ok := got[key]
			if !ok || gotValue != wantValue {
				t.Fatalf("summary[%q] = %d (present=%t), want %d", key, gotValue, ok, wantValue)
			}
		})
	}
}

// failingWriter fails every write so an encoder error can be observed.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// TestWriteJSONSummary_WriterFails_ReturnsError verifies an encoder failure
// reaches the caller, which is what lets main report it instead of printing a
// truncated summary.
func TestWriteJSONSummary_WriterFails_ReturnsError(t *testing.T) {
	err := writeJSONSummary(failingWriter{}, auditMetrics{})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("writeJSONSummary() error = %v, want the writer failure", err)
	}
}

// metricRow renders one report row the way printRow does, so an assertion
// pins both the value and the column layout.
func metricRow(label string, value int) string {
	return fmt.Sprintf("  %-*s %d\n", metricLabelWidth, label, value)
}

// TestPrintReport_RealMetrics_PrintsEveryCountedRow verifies the text report
// prints each core, category and codebase row with the value from the
// gathered payload, every section in its documented order, and the meta-tool
// lists in full.
func TestPrintReport_RealMetrics_PrintsEveryCountedRow(t *testing.T) {
	t.Setenv("META_PARAM_SCHEMA", "opaque")
	metrics := collectedMetrics(t)

	output := captureStdout(t, func() {
		printReport(metrics, newAuditMetricsClient(t))
	})

	rows := []struct {
		label string
		value int
	}{
		{"Individual MCP tools (self-managed enterprise)", len(metrics.individualTools)},
		{"Individual MCP tools (GitLab.com enterprise)", len(metrics.gitLabComIndividualTools)},
		{"Meta-tools (base)", len(metrics.metaBase)},
		{"Meta-tools (self-managed enterprise)", len(metrics.metaEnterprise)},
		{"Meta-tools (GitLab.com enterprise)", len(metrics.metaGitLabComEnterprise)},
		{"Dynamic tools (base)", 2},
		{"Dynamic tools (self-managed enterprise)", 2},
		{"Dynamic tools (GitLab.com enterprise)", 2},
		{"Dynamic catalog actions (base)", metrics.dynamicBaseActions},
		{"Dynamic catalog actions (self-managed enterprise)", metrics.dynamicEnterpriseActions},
		{"Dynamic catalog actions (GitLab.com enterprise)", metrics.dynamicGitLabComEnterpriseActions},
		{"Dynamic search index tokens (base)", metrics.dynamicBaseMetrics.IndexTokenCount},
		{"Dynamic aliases ambiguous (GitLab.com enterprise)", metrics.dynamicGitLabComEnterpriseMetrics.AmbiguousAliasCount},
		{"Spec-backed enterprise catalog actions", len(metrics.enterpriseActionAudit.SpecBacked)},
		{"Enterprise catalog actions missing ActionSpec", 0},
		{"Enterprise-only meta-tools", len(metrics.metaEnterprise) - len(metrics.metaBase)},
		{"GitLab.com-only meta-tools", len(metrics.metaGitLabComEnterprise) - len(metrics.metaEnterprise)},
		{"GitLab.com-only individual tools", len(metrics.gitLabComIndividualTools) - len(metrics.individualTools)},
		{"MCP Resources (total)", metrics.staticResources + metrics.templateResources},
		{"  Static resources", metrics.staticResources},
		{"  Resource templates", metrics.templateResources},
		{"  Workspace roots", 1},
		{"MCP Prompts", metrics.promptCount},
		{"Elicitation tools", metrics.elicitationCount},
		{"Standard tools", len(metrics.individualTools) - metrics.elicitationCount},
		{"internal/tools Go packages", metrics.toolPackages},
		{"Source files (.go)", metrics.srcFiles},
		{"Test files (_test.go)", metrics.testFiles},
	}
	for _, row := range rows {
		t.Run(strings.TrimSpace(row.label), func(t *testing.T) {
			if want := metricRow(row.label, row.value); !strings.Contains(output, want) {
				t.Fatalf("report lacks row %q:\n%s", want, output)
			}
		})
	}

	assertInOrder(t, output,
		"  gitlab-mcp-server. MCP Server Metrics Audit\n",
		"## Core Metrics\n",
		"## Tool Categories\n",
		"## Meta-Tool Schema Modes\n",
		"  Active mode (env): opaque\n",
		"## Codebase Metrics\n",
		"## Catalog Domain Breakdown (GitLab.com enterprise, top 20)\n",
		"  Domain                    Tools\n",
		"## Enterprise ActionSpec Audit\n",
		"### Enterprise actions missing ActionSpec (0)\n  - none\n",
		"## Meta-tools List\n",
		fmt.Sprintf("### Base (%d)\n", len(metrics.metaBase)),
		fmt.Sprintf("### Enterprise-only (%d)\n", len(metrics.metaEnterprise)-len(metrics.metaBase)),
		fmt.Sprintf("### GitLab.com-only enterprise (%d)\n", len(metrics.metaGitLabComEnterprise)-len(metrics.metaEnterprise)),
	)
	if !strings.Contains(output, "  ... and ") {
		t.Fatalf("report lacks the domain overflow line for the %d-domain catalog:\n%s", len(metrics.gitLabComEnterpriseDomains), output)
	}

	baseList := sectionBetween(t, output, fmt.Sprintf("### Base (%d)\n", len(metrics.metaBase)), "\n### Enterprise-only (")
	for _, tool := range metrics.metaBase {
		if !strings.Contains(baseList, fmt.Sprintf(toolListFormat, tool.Name)) {
			t.Fatalf("base meta-tool list lacks %s:\n%s", tool.Name, baseList)
		}
	}
	for _, id := range metrics.enterpriseActionAudit.SpecBacked {
		if !strings.Contains(output, fmt.Sprintf(toolListFormat, id)) {
			t.Fatalf("spec-backed list lacks %s", id)
		}
	}
}

// TestPrintReport_FixtureMetrics_ListsSurfaceDeltas verifies the report's
// derived sections against a hand-built payload where every expected line is
// known: the meta-tool deltas name exactly the tools one surface adds over the
// previous one, the domain table is sorted and complete, and the enterprise
// audit lists a missing spec when the payload carries one.
func TestPrintReport_FixtureMetrics_ListsSurfaceDeltas(t *testing.T) {
	t.Setenv("META_PARAM_SCHEMA", "full")
	metrics := auditMetrics{
		individualTools:                   namedTools("gitlab_project_get", "gitlab_interactive_create_issue"),
		gitLabComIndividualTools:          namedTools("gitlab_project_get", "gitlab_interactive_create_issue", "gitlab_orbit_status"),
		metaBase:                          namedTools("gitlab_issue", "gitlab_project"),
		metaEnterprise:                    namedTools("gitlab_geo", "gitlab_issue", "gitlab_project"),
		metaGitLabComEnterprise:           namedTools("gitlab_geo", "gitlab_issue", "gitlab_orbit", "gitlab_project"),
		dynamicBase:                       namedTools("gitlab_execute_action", "gitlab_find_action"),
		dynamicBaseActions:                10,
		dynamicEnterpriseActions:          12,
		dynamicGitLabComEnterpriseActions: 13,
		enterpriseActionAudit:             enterpriseActionSpecAudit{SpecBacked: []string{"geo.list"}, MissingSpec: []string{"legacy.route"}},
		gitLabComEnterpriseDomains:        map[string]int{"project": 3, "issue": 2, "orbit": 1},
		staticResources:                   4,
		templateResources:                 6,
		promptCount:                       7,
		toolPackages:                      8,
		srcFiles:                          9,
		testFiles:                         10,
		elicitationCount:                  1,
	}

	output := captureStdout(t, func() {
		printReport(metrics, newAuditMetricsClient(t))
	})

	rows := []struct {
		label string
		value int
	}{
		{"Dynamic tools (base)", 2},
		{"Dynamic tools (self-managed enterprise)", 0},
		{"Enterprise-only meta-tools", 1},
		{"GitLab.com-only meta-tools", 1},
		{"GitLab.com-only individual tools", 1},
		{"MCP Resources (total)", 10},
		{"Enterprise catalog actions missing ActionSpec", 1},
		{"Standard tools", 1},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			if want := metricRow(row.label, row.value); !strings.Contains(output, want) {
				t.Fatalf("report lacks row %q:\n%s", want, output)
			}
		})
	}

	wantDomains := "  Domain                    Tools\n" +
		"  ------------------------- -----\n" +
		"  project                   3\n" +
		"  issue                     2\n" +
		"  orbit                     1\n"
	if !strings.Contains(output, wantDomains) {
		t.Fatalf("report lacks the sorted domain table:\n%s", output)
	}
	wantAudit := "## Enterprise ActionSpec Audit\n\n" +
		"### Spec-backed enterprise actions (1)\n  - geo.list\n\n" +
		"### Enterprise actions missing ActionSpec (1)\n  - legacy.route\n"
	if !strings.Contains(output, wantAudit) {
		t.Fatalf("report lacks the enterprise audit sections:\n%s", output)
	}
	wantLists := "## Meta-tools List\n\n" +
		"### Base (2)\n  - gitlab_issue\n  - gitlab_project\n\n" +
		"### Enterprise-only (1)\n  - gitlab_geo\n\n" +
		"### GitLab.com-only enterprise (1)\n  - gitlab_orbit\n"
	if !strings.HasSuffix(output, wantLists) {
		t.Fatalf("report does not end with the meta-tool lists:\n%s", output)
	}
	if !strings.Contains(output, "  Active mode (env): full\n") {
		t.Fatalf("report lacks the active schema mode:\n%s", output)
	}
}

// namedTools builds tool definitions carrying only the names a list assertion
// needs.
func namedTools(names ...string) []*mcp.Tool {
	listed := make([]*mcp.Tool, 0, len(names))
	for _, name := range names {
		listed = append(listed, &mcp.Tool{Name: name})
	}
	return listed
}

// assertInOrder verifies every marker occurs in s, each after the previous
// one.
func assertInOrder(t *testing.T, s string, markers ...string) {
	t.Helper()
	offset := 0
	for _, marker := range markers {
		index := strings.Index(s[offset:], marker)
		if index < 0 {
			t.Fatalf("%q not found after offset %d in:\n%s", marker, offset, s)
		}
		offset += index + len(marker)
	}
}

// sectionBetween returns the text of s that follows start and precedes the
// next occurrence of end.
func sectionBetween(t *testing.T, s, start, end string) string {
	t.Helper()
	from := strings.Index(s, start)
	if from < 0 {
		t.Fatalf("%q not found in:\n%s", start, s)
	}
	from += len(start)
	to := strings.Index(s[from:], end)
	if to < 0 {
		t.Fatalf("%q not found after %q in:\n%s", end, start, s)
	}
	return s[from : from+to]
}

// TestRun_RejectsNegativeTopDomains verifies that the flag guard refuses a
// negative count before any catalog work starts, and says so on stderr.
func TestRun_RejectsNegativeTopDomains(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run(auditOptions{topDomains: -1}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("run() = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "-top-domains must be >= 0") {
		t.Errorf("stderr = %q, want the guard message", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing written before the guard fires", stdout.String())
	}
}

// TestRun_JSONMode_WritesTheSummaryToTheGivenWriter verifies that -json emits
// the JSON summary to the writer run was handed rather than to os.Stdout, and
// that the document carries the counts the report is built from.
func TestRun_JSONMode_WritesTheSummaryToTheGivenWriter(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run(auditOptions{topDomains: 5, jsonOut: true}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	var summary map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	for _, key := range []string{"individual_tools", "meta_base", "meta_enterprise", "dynamic_base", "resources", "prompts", "tool_packages"} {
		t.Run(key, func(t *testing.T) {
			if _, ok := summary[key]; !ok {
				t.Errorf("summary has no %q key: %v", key, summary)
			}
		})
	}
}

// TestRun_JSONMode_WriterFails_ReportsAndExitsOne verifies that a stdout which
// refuses the write is the one failure the JSON mode still has to report: the
// counting cannot fail, so the encoder is where run learns that the summary
// did not land, and it names it on stderr and exits non-zero rather than
// claiming success over a truncated document.
func TestRun_JSONMode_WriterFails_ReportsAndExitsOne(t *testing.T) {
	var stderr bytes.Buffer

	code := run(auditOptions{topDomains: 5, jsonOut: true}, failingWriter{}, &stderr)

	if code != 1 {
		t.Errorf("run() = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "encode json: disk full") {
		t.Errorf("stderr = %q, want the encoder failure", stderr.String())
	}
}

// TestRun_ReportMode_PrintsTheMarkdownReport verifies the default mode writes
// the human report and nothing to stderr.
func TestRun_ReportMode_PrintsTheMarkdownReport(t *testing.T) {
	var stderr bytes.Buffer

	var code int
	out := captureStdout(t, func() {
		code = run(auditOptions{topDomains: 3}, os.Stdout, &stderr)
	})

	if code != 0 {
		t.Fatalf("run() = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty on the happy path", stderr.String())
	}
	if !strings.Contains(out, "Tools") {
		t.Errorf("report does not look like the metrics report:\n%s", out)
	}
}

// TestRun_SiteStats_WritesAndThenVerifies verifies the site-stats mode writes
// the JSON document, and that running it again with checkOnly accepts the file
// it just wrote and rejects a modified one.
func TestRun_SiteStats_WritesAndThenVerifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	var stdout, stderr bytes.Buffer

	if code := run(auditOptions{topDomains: 1, siteStatsPath: path}, &stdout, &stderr); code != 0 {
		t.Fatalf("write mode = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	written, err := os.ReadFile(path) //#nosec G304 -- path is a temporary directory this test created
	if err != nil {
		t.Fatalf("read written stats: %v", err)
	}
	if !json.Valid(written) {
		t.Fatalf("written stats are not JSON:\n%s", written)
	}

	stderr.Reset()
	if code := run(auditOptions{topDomains: 1, siteStatsPath: path, checkOnly: true}, &stdout, &stderr); code != 0 {
		t.Fatalf("check mode on a fresh file = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	if writeErr := os.WriteFile(path, []byte(`{"version":"0.0.0"}`), 0o600); writeErr != nil {
		t.Fatalf("rewrite stats: %v", writeErr)
	}
	stderr.Reset()
	if code := run(auditOptions{topDomains: 1, siteStatsPath: path, checkOnly: true}, &stdout, &stderr); code != 1 {
		t.Errorf("check mode on a stale file = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Error("check mode failed without saying why")
	}
}
