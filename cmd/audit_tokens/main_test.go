// main_test.go contains focused tests for the audit_tokens command. Tests use
// a local GitLab version mock and exercise the resource token measurement path
// that depends on the surface-aware tool manifest resources.
//
// Coverage spans the resource registration options (including the minimal
// candidate), the small pure helpers (domain parsing, total tokens, number
// formatting, table printing) that compose the audit report, the report and
// JSON renderers driven by a real measurement, the schema sizing table, the
// footprint write and check modes driven through the measurement seam, and
// every mode of the run entry point, whose reachable failures are the ones a
// caller can act on.
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
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// newAuditTokensClient returns the binary-lifetime [gitlabclient.Client]
// backed by a mock /api/v4/version endpoint, the client main builds. It is
// shared across tests (process teardown reclaims the httptest server) so the
// schema caches keyed on the client serve every registration after the first.
func newAuditTokensClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	sharedClientOnce.Do(func() {
		sharedClient, _ = auditclient.NewMock()
	})
	return sharedClient
}

// measuredFootprintRows returns the full tier x surface x mode measurement,
// taken once per process: it registers every surface three times over, so
// tests share the result and treat it as read-only.
func measuredFootprintRows(t *testing.T) []tokenFootprintRow {
	t.Helper()
	footprintOnce.Do(func() {
		footprintShared = measureTokenFootprintRows(newAuditTokensClient(t))
	})
	return footprintShared
}

// measuredTokenAudit returns the default report's measurement, taken once per
// process and shared read-only for the same reason as the footprint rows.
func measuredTokenAudit(t *testing.T) tokenAudit {
	t.Helper()
	tokenAuditOnce.Do(func() {
		tokenAuditShared = measureTokenAudit(newAuditTokensClient(t))
	})
	return tokenAuditShared
}

var (
	sharedClientOnce sync.Once
	sharedClient     *gitlabclient.Client
	footprintOnce    sync.Once
	footprintShared  []tokenFootprintRow
	tokenAuditOnce   sync.Once
	tokenAuditShared tokenAudit
)

// auditDynamicSurface returns the base action routes, the catalog built over
// them, and the tools the dynamic surface advertises for it. The three
// measurement tests all start from that trio.
func auditDynamicSurface(t *testing.T, client *gitlabclient.Client) (map[string]toolutil.ActionMap, *actioncatalog.Catalog, []*mcp.Tool) {
	t.Helper()
	routes := buildMetaActionMaps(client, false)
	catalog := actioncatalog.FromActionMaps(routes)
	return routes, catalog, listDynamicTools(catalog)
}

// TestMeasureResources_IncludesToolManifest verifies the token audit measures
// the surface-aware tool manifest in addition to static resources.
func TestMeasureResources_IncludesToolManifest(t *testing.T) {
	client := newAuditTokensClient(t)
	routes, dynamicCatalog, dynamicTools := auditDynamicSurface(t, client)
	manifestTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest: true,
		ToolSurface:  config.ToolSurfaceDynamic,
		ToolList:     dynamicTools,
		ToolCatalog:  dynamicCatalog,
	})
	bareTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{})
	if manifestTokens <= bareTokens {
		t.Fatalf("manifest resource tokens = %d, want greater than bare server %d", manifestTokens, bareTokens)
	}
}

// TestMeasureResourcesWithOptions_MinimalCandidate verifies the dynamic-minimal
// candidate keeps the tool manifest while dropping the
// heavier optional resource groups.
func TestMeasureResourcesWithOptions_MinimalCandidate(t *testing.T) {
	client := newAuditTokensClient(t)
	routes, dynamicCatalog, dynamicTools := auditDynamicSurface(t, client)
	fullDynamicTokens := measureResources(client, routes, dynamicCatalog, dynamicTools, config.ToolSurfaceDynamic)
	minimalTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest: true,
		ToolSurface:  config.ToolSurfaceDynamic,
		ToolList:     dynamicTools,
		ToolCatalog:  dynamicCatalog,
	})

	if minimalTokens <= 0 {
		t.Fatalf("minimal resource tokens = %d, want positive tool-manifest estimate", minimalTokens)
	}
	if minimalTokens >= fullDynamicTokens {
		t.Fatalf("minimal resource tokens = %d, want less than full dynamic %d", minimalTokens, fullDynamicTokens)
	}
}

// TestListDynamicTools_ExposesLowTokenSurface verifies the dynamic audit path
// measures the find/execute tools backed by the canonical action catalog.
func TestListDynamicTools_ExposesLowTokenSurface(t *testing.T) {
	client := newAuditTokensClient(t)
	routes, _, toolList := auditDynamicSurface(t, client)
	if countActions(routes) == 0 {
		t.Fatal("buildMetaActionMaps() returned no actions")
	}

	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_execute_action,gitlab_find_action" {
		t.Fatalf("dynamic tools = %q, want find/execute", got)
	}
}

// TestListTools_UnknownSurface_Panics verifies a surface name the audit does
// not know stops the run instead of measuring an empty server as if it were a
// surface. Every call site passes a config surface constant, so an
// unrecognized one is a programming error, and the panic names it.
func TestListTools_UnknownSurface_Panics(t *testing.T) {
	client := newAuditTokensClient(t)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		listTools(client, "bogus", false)
	}()

	if recovered == nil {
		t.Fatal("listTools(bogus) returned, want a panic naming the surface")
	}
	if got := fmt.Sprint(recovered); got != `audit_tokens: unknown tool surface "bogus"` {
		t.Fatalf("panic = %q, want the unknown surface message", got)
	}
}

// TestExtractDomain_ParsesGitlabToolNames verifies the domain extractor returns
// the second segment for gitlab_{domain}_{action} names and "unknown" for
// malformed inputs.
func TestExtractDomain_ParsesGitlabToolNames(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "two segment", in: "gitlab_project", want: "project"},
		{name: "three segment", in: "gitlab_project_list", want: "project"},
		{name: "four segment", in: "gitlab_merge_request_approvals", want: "merge"},
		{name: "single segment", in: "gitlab", want: "unknown"},
		{name: "empty", in: "", want: "unknown"},
		{name: "no prefix", in: "project_list", want: "list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractDomain(tt.in); got != tt.want {
				t.Fatalf("extractDomain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestTotalTokens_SumsTokenEstimates verifies the aggregator sums the Tokens
// field across the provided tool info records.
func TestTotalTokens_SumsTokenEstimates(t *testing.T) {
	got := totalTokens([]toolTokenInfo{
		{Name: "a", Tokens: 10},
		{Name: "b", Tokens: 20},
		{Name: "c", Tokens: 30},
	})
	if got != 60 {
		t.Fatalf("totalTokens() = %d, want 60", got)
	}
}

// TestTotalTokens_EmptyInput verifies the aggregator returns zero for an
// empty input slice.
func TestTotalTokens_EmptyInput(t *testing.T) {
	if got := totalTokens(nil); got != 0 {
		t.Fatalf("totalTokens(nil) = %d, want 0", got)
	}
}

// TestTotalBytes_MeasuredRecords_SumsByteSizes verifies the byte aggregator sums the Bytes
// field, ignoring the token column, and yields zero for no records.
func TestTotalBytes_MeasuredRecords_SumsByteSizes(t *testing.T) {
	tests := []struct {
		name  string
		infos []toolTokenInfo
		want  int
	}{
		{name: "three records", infos: []toolTokenInfo{{Bytes: 400, Tokens: 1}, {Bytes: 250, Tokens: 2}, {Bytes: 1, Tokens: 3}}, want: 651},
		{name: "no records", infos: nil, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := totalBytes(tt.infos); got != tt.want {
				t.Fatalf("totalBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestCountActions_AggregatesAcrossRoutes verifies the route count aggregator
// sums action counts across all route keys.
func TestCountActions_AggregatesAcrossRoutes(t *testing.T) {
	// Build a route map with three actions split across two tools.
	noop := func(_ context.Context, _ map[string]any) (any, error) { return nil, nil } //nolint:nilnil // test fixture: always no-ops
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get":   {Handler: noop},
			"list":  {Handler: noop},
			"stats": {Handler: noop},
		},
		"gitlab_issue": {
			"create": {Handler: noop},
		},
	}
	if got := countActions(routes); got != 4 {
		t.Fatalf("countActions() = %d, want 4", got)
	}
}

// TestCountActions_EmptyRoutesReturnsZero verifies the aggregator returns zero
// for an empty route map.
func TestCountActions_EmptyRoutesReturnsZero(t *testing.T) {
	if got := countActions(map[string]toolutil.ActionMap{}); got != 0 {
		t.Fatalf("countActions() = %d, want 0", got)
	}
}

// TestFmtNum_AddsThousandsSeparators verifies the format helper inserts comma
// separators in the right positions for the supported ranges.
func TestFmtNum_AddsThousandsSeparators(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{in: 0, want: "0"},
		{in: 1, want: "1"},
		{in: 42, want: "42"},
		{in: 999, want: "999"},
		{in: 1000, want: "1,000"},
		{in: 1234, want: "1,234"},
		{in: 12345, want: "12,345"},
		{in: 123456, want: "123,456"},
		{in: 1234567, want: "1,234,567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := fmtNum(tt.in); got != tt.want {
				t.Fatalf("fmtNum(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestMeasureTools_AssignsDomainAndComputesTokens verifies the tool token
// estimator captures name, domain, byte length, and token count.
func TestMeasureTools_AssignsDomainAndComputesTokens(t *testing.T) {
	toolList := []*mcp.Tool{
		{Name: "gitlab_project_get", Description: "Get a project."},
		{Name: "gitlab_issue_create", Description: "Create an issue."},
	}

	got := measureTools(toolList)
	if len(got) != 2 {
		t.Fatalf("measureTools() returned %d items, want 2", len(got))
	}
	// Results are sorted descending by tokens; assert names appear with
	// the expected metadata.
	names := map[string]bool{}
	for _, info := range got {
		names[info.Name] = true
		if info.Tokens <= 0 {
			t.Errorf("info %q: Tokens=%d, want > 0 (real tokenizer)", info.Name, info.Tokens)
		}
		if info.Domain == "" {
			t.Errorf("info %q: Domain is empty", info.Name)
		}
		if info.Bytes <= 0 {
			t.Errorf("info %q: Bytes = %d, want positive", info.Name, info.Bytes)
		}
	}
	for _, want := range []string{"gitlab_project_get", "gitlab_issue_create"} {
		t.Run(want, func(t *testing.T) {
			if !names[want] {
				t.Errorf("measureTools() missing %q in results: %+v", want, got)
			}
		})
	}
}

// TestMeasureTools_EmptyInputReturnsEmpty verifies the estimator returns an
// empty slice for an empty tool list.
func TestMeasureTools_EmptyInputReturnsEmpty(t *testing.T) {
	if got := measureTools(nil); len(got) != 0 {
		t.Fatalf("measureTools(nil) = %d items, want 0", len(got))
	}
}

// TestMeasurePrompts_ReturnsTokenEstimateForRegisteredPrompts verifies the
// prompt token estimator produces a positive count for a real client.
func TestMeasurePrompts_ReturnsTokenEstimateForRegisteredPrompts(t *testing.T) {
	if got := measurePrompts(newAuditTokensClient(t)); got <= 0 {
		t.Fatalf("measurePrompts() = %d, want positive token estimate", got)
	}
}

// TestPrintTopTools_TruncatesToRequestedLimit verifies the printer caps the
// number of output rows at the requested n parameter.
func TestPrintTopTools_TruncatesToRequestedLimit(t *testing.T) {
	infos := []toolTokenInfo{
		{Name: "tool_a", Tokens: 300, Bytes: 1200},
		{Name: "tool_b", Tokens: 200, Bytes: 800},
		{Name: "tool_c", Tokens: 100, Bytes: 400},
	}

	output := captureStdoutAudit(t, func() {
		printTopTools(infos, 2)
	})

	if !strings.Contains(output, "tool_a") || !strings.Contains(output, "tool_b") {
		t.Fatalf("printTopTools() missing first two rows:\n%s", output)
	}
	if strings.Contains(output, "tool_c") {
		t.Fatalf("printTopTools() included rows beyond n=2:\n%s", output)
	}
}

// TestPrintTopTools_NLargerThanLength verifies the printer uses the full input
// length when n exceeds the available records.
func TestPrintTopTools_NLargerThanLength(t *testing.T) {
	infos := []toolTokenInfo{{Name: "only", Tokens: 10, Bytes: 40}}

	output := captureStdoutAudit(t, func() {
		printTopTools(infos, 100)
	})

	if !strings.Contains(output, "only") {
		t.Fatalf("printTopTools() missing single record:\n%s", output)
	}
}

// TestPrintDomainTotals_AggregatesAndSortsByTokenCost verifies the printer
// groups tools by domain, sums tokens, sorts descending, and limits rows.
func TestPrintDomainTotals_AggregatesAndSortsByTokenCost(t *testing.T) {
	infos := []toolTokenInfo{
		{Name: "gitlab_project_get", Domain: "project", Tokens: 100, Bytes: 400},
		{Name: "gitlab_project_list", Domain: "project", Tokens: 50, Bytes: 200},
		{Name: "gitlab_issue_get", Domain: "issue", Tokens: 30, Bytes: 120},
	}

	output := captureStdoutAudit(t, func() {
		printDomainTotals(infos, 10)
	})

	if !strings.Contains(output, "project") || !strings.Contains(output, "issue") {
		t.Fatalf("printDomainTotals() missing domain rows:\n%s", output)
	}
	// project should appear before issue because total tokens (150) > 30.
	assertBefore(t, output, "project", "issue")
	// Count column should be 2 for project (two tools) and 1 for issue.
	if !strings.Contains(output, "2") {
		t.Fatalf("printDomainTotals() output missing count column:\n%s", output)
	}
}

// TestPrintDomainTotals_RespectsLimit verifies the printer caps the row count
// at the requested n parameter.
func TestPrintDomainTotals_RespectsLimit(t *testing.T) {
	infos := []toolTokenInfo{
		{Name: "gitlab_a", Domain: "a", Tokens: 100},
		{Name: "gitlab_b", Domain: "b", Tokens: 80},
		{Name: "gitlab_c", Domain: "c", Tokens: 60},
	}

	output := captureStdoutAudit(t, func() {
		printDomainTotals(infos, 1)
	})

	if !strings.Contains(output, "a") {
		t.Fatalf("printDomainTotals() missing first row:\n%s", output)
	}
	if strings.Contains(output, "| b ") || strings.Contains(output, "| c ") {
		t.Fatalf("printDomainTotals() included rows beyond n=1:\n%s", output)
	}
}

// TestPrintDomainTotals_EmptyInput verifies the printer renders the table
// header without data rows for an empty input.
func TestPrintDomainTotals_EmptyInput(t *testing.T) {
	output := captureStdoutAudit(t, func() {
		printDomainTotals(nil, 5)
	})

	if !strings.Contains(output, "Domain") {
		t.Fatalf("printDomainTotals() missing header for empty input:\n%s", output)
	}
}

// captureStreamAudit redirects *stream (os.Stdout or os.Stderr) into a pipe
// while fn runs and returns everything fn wrote. The pipe is drained
// concurrently so a report larger than the pipe buffer cannot block the
// writer; the reader goroutine only records, every assertion stays on the
// test goroutine.
func captureStreamAudit(t *testing.T, stream **os.File, fn func()) string {
	t.Helper()
	original := *stream
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	*stream = w
	t.Cleanup(func() { *stream = original })

	var captured bytes.Buffer
	var readErr error
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, readErr = captured.ReadFrom(r)
	}()

	fn()
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error: %v", closeErr)
	}
	*stream = original
	<-drained
	if readErr != nil {
		t.Fatalf("drain pipe: %v", readErr)
	}
	if closeErr := r.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error: %v", closeErr)
	}
	return captured.String()
}

// captureStdoutAudit captures os.Stdout while fn runs and returns the result
// as a string.
func captureStdoutAudit(t *testing.T, fn func()) string {
	t.Helper()
	return captureStreamAudit(t, &os.Stdout, fn)
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

// assertBefore verifies that the before substring occurs before the after
// substring in s.
func assertBefore(t *testing.T, s, before, after string) {
	t.Helper()
	bi := strings.Index(s, before)
	if bi < 0 {
		t.Fatalf("%q not found in:\n%s", before, s)
	}
	ai := strings.Index(s, after)
	if ai < 0 {
		t.Fatalf("%q not found in:\n%s", after, s)
	}
	if bi >= ai {
		t.Fatalf("%q should appear before %q in:\n%s", before, after, s)
	}
}

// lineFields returns the whitespace-separated fields of the first line of s
// that starts with prefix, failing when no line does. It is how a tabwriter
// row is asserted without depending on the column padding.
func lineFields(t *testing.T, s, prefix string) []string {
	t.Helper()
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.Fields(line)
		}
	}
	t.Fatalf("no line starting with %q in:\n%s", prefix, s)
	return nil
}

// TestMeasureTokenAudit_RealSurfaces_AgreesWithFootprint verifies the default
// report's measurement against the footprint matrix, which measures the same
// surfaces through its own registrations: the individual surface is the
// Ultimate tier's, the dynamic surface is the base tier's with the same tool
// schema and shared costs, the reachable action counts match, and the meta
// surface carries the server maintenance tool.
func TestMeasureTokenAudit_RealSurfaces_AgreesWithFootprint(t *testing.T) {
	audit := measuredTokenAudit(t)
	rows := measuredFootprintRows(t)
	freeDynamic, freeMinimal, ultimateDynamic, ultimateIndividual := rows[0], rows[1], rows[18], rows[26]

	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "individual tools are the Ultimate individual surface", got: len(audit.individualInfo), want: ultimateIndividual.VisibleTools},
		{name: "individual tool tokens match the Ultimate individual row", got: totalTokens(audit.individualInfo), want: ultimateIndividual.ToolSchemaTokens},
		{name: "dynamic base is two tools", got: len(audit.dynamicBaseInfo), want: 2},
		{name: "dynamic enterprise is two tools", got: len(audit.dynamicEnterpriseInfo), want: 2},
		{name: "dynamic base tool tokens match the Free dynamic row", got: totalTokens(audit.dynamicBaseInfo), want: freeDynamic.ToolSchemaTokens},
		{name: "dynamic full shared cost matches the Free dynamic row", got: audit.dynamicBaseResourceTokens + audit.promptTokens, want: freeDynamic.SharedTokens},
		{name: "dynamic minimal shared cost matches the Free minimal row", got: audit.dynamicMinimalResourceTokens, want: freeMinimal.SharedTokens},
		{name: "base reachable actions match the Free tier", got: audit.baseReachableActions, want: freeDynamic.ReachableActions},
		{name: "enterprise reachable actions match the Ultimate tier", got: audit.enterpriseReachableActions, want: ultimateDynamic.ReachableActions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %d, want %d", tt.got, tt.want)
			}
		})
	}

	if audit.metaBaseCatalogActions >= audit.baseReachableActions || audit.metaEnterpriseCatalogActions >= audit.enterpriseReachableActions {
		t.Fatalf("catalog-only route counts (%d, %d) must be below the reachable counts (%d, %d) that fold standalone actions in",
			audit.metaBaseCatalogActions, audit.metaEnterpriseCatalogActions, audit.baseReachableActions, audit.enterpriseReachableActions)
	}
	if len(audit.metaBaseInfo) == 0 || len(audit.metaEnterpriseInfo) <= len(audit.metaBaseInfo) {
		t.Fatalf("meta surfaces = (%d, %d) tools, want a populated base surface and a larger enterprise one", len(audit.metaBaseInfo), len(audit.metaEnterpriseInfo))
	}
	if !slices.ContainsFunc(audit.metaBaseInfo, func(info toolTokenInfo) bool { return info.Name == "gitlab_server" }) {
		t.Fatal("meta base surface lacks gitlab_server, the MCP maintenance meta-tool")
	}
	if !slices.IsSortedFunc(audit.individualInfo, func(a, b toolTokenInfo) int { return b.Tokens - a.Tokens }) {
		t.Fatal("individual measurements are not sorted by descending token cost")
	}
	if audit.individualResourceTokens <= 0 || audit.metaBaseResourceTokens <= 0 || audit.promptTokens <= 0 {
		t.Fatalf("shared measurements (%d, %d, %d) must all be positive", audit.individualResourceTokens, audit.metaBaseResourceTokens, audit.promptTokens)
	}
}

// TestWriteTokenAuditJSON_RealMeasurements_EmitsDocumentedShape verifies the
// -json summary carries exactly the eleven documented keys, each holding the
// figure the text report prints for the same measurement.
func TestWriteTokenAuditJSON_RealMeasurements_EmitsDocumentedShape(t *testing.T) {
	audit := measuredTokenAudit(t)

	var out bytes.Buffer
	if err := writeTokenAuditJSON(&out, audit); err != nil {
		t.Fatalf("writeTokenAuditJSON() error: %v", err)
	}
	if !strings.HasPrefix(out.String(), "{\n  \"individual_tools\": ") || !strings.HasSuffix(out.String(), "}\n") {
		t.Fatalf("writeTokenAuditJSON() is not an indented object with a trailing newline:\n%s", out.String())
	}

	var got map[string]int
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	want := map[string]int{
		"individual_tools":          len(audit.individualInfo),
		"meta_base_tools":           len(audit.metaBaseInfo),
		"dynamic_base_tools":        len(audit.dynamicBaseInfo),
		"individual_tokens":         totalTokens(audit.individualInfo),
		"meta_base_tokens":          totalTokens(audit.metaBaseInfo),
		"meta_enterprise_tokens":    totalTokens(audit.metaEnterpriseInfo),
		"dynamic_base_tokens":       totalTokens(audit.dynamicBaseInfo),
		"dynamic_enterprise_tokens": totalTokens(audit.dynamicEnterpriseInfo),
		"base_reachable_actions":    audit.baseReachableActions,
		"resource_tokens":           audit.individualResourceTokens,
		"prompt_tokens":             audit.promptTokens,
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

// TestWriteTokenAuditJSON_WriterFails_ReturnsError verifies an encoder
// failure reaches the caller, which is what lets main report it.
func TestWriteTokenAuditJSON_WriterFails_ReturnsError(t *testing.T) {
	err := writeTokenAuditJSON(failingWriter{}, tokenAudit{})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("writeTokenAuditJSON() error = %v, want the writer failure", err)
	}
}

// TestPrintTokenAuditReport_RealMeasurements_PrintsTotals verifies the text
// report prints the mode comparison rows, the savings percentages, the shared
// overhead, the rankings and the grand totals with the figures of the
// measurement it was handed, every section in its documented order.
func TestPrintTokenAuditReport_RealMeasurements_PrintsTotals(t *testing.T) {
	audit := measuredTokenAudit(t)
	indTotal := totalTokens(audit.individualInfo)
	metaTotal := totalTokens(audit.metaBaseInfo)
	dynamicTotal := totalTokens(audit.dynamicBaseInfo)
	sharedFull := audit.dynamicBaseResourceTokens + audit.promptTokens

	output := captureStdoutAudit(t, func() {
		printTokenAuditReport(audit, 5, 3)
	})

	sections := []string{
		"  gitlab-mcp-server. Token Overhead Audit\n",
		"## Mode Comparison\n",
		"## Shared Overhead (Resources + Prompts)\n",
		"## Minimal Capability Candidate\n",
		"## Top 30 Individual Tools by Token Cost\n",
		"## Meta-Tools by Token Cost (base)\n",
		"## Dynamic Tools by Token Cost (base)\n",
		"## Domain Totals (Individual Mode, Top 20)\n",
		"## Grand Total (what an LLM sees)\n",
	}
	assertInOrder(t, output, sections...)

	rows := []struct {
		prefix string
		want   []string
	}{
		{prefix: "  Individual (all)", want: []string{"Individual", "(all)", strconv.Itoa(len(audit.individualInfo)), strconv.Itoa(len(audit.individualInfo)), fmtNum(indTotal), fmtNum(totalBytes(audit.individualInfo))}},
		{prefix: "  Meta-tools (base)", want: []string{"Meta-tools", "(base)", strconv.Itoa(len(audit.metaBaseInfo)), strconv.Itoa(audit.baseReachableActions), fmtNum(metaTotal), fmtNum(totalBytes(audit.metaBaseInfo))}},
		{prefix: "  Meta-tools (enterprise)", want: []string{"Meta-tools", "(enterprise)", strconv.Itoa(len(audit.metaEnterpriseInfo)), strconv.Itoa(audit.enterpriseReachableActions), fmtNum(totalTokens(audit.metaEnterpriseInfo)), fmtNum(totalBytes(audit.metaEnterpriseInfo))}},
		{prefix: "  Dynamic (base)", want: []string{"Dynamic", "(base)", "2", strconv.Itoa(audit.baseReachableActions), fmtNum(dynamicTotal), fmtNum(totalBytes(audit.dynamicBaseInfo))}},
		{prefix: "  Dynamic (enterprise)", want: []string{"Dynamic", "(enterprise)", "2", strconv.Itoa(audit.enterpriseReachableActions), fmtNum(totalTokens(audit.dynamicEnterpriseInfo)), fmtNum(totalBytes(audit.dynamicEnterpriseInfo))}},
	}
	for _, row := range rows {
		t.Run(strings.TrimSpace(row.prefix), func(t *testing.T) {
			if got := lineFields(t, output, row.prefix); !slices.Equal(got, row.want) {
				t.Fatalf("row fields = %v, want %v", got, row.want)
			}
		})
	}

	lines := []string{
		fmt.Sprintf("  Reachable action counts include %d standalone utility actions (project discovery + interactive flows) that are visible tools in meta mode and folded into the dynamic catalog.\n", audit.baseReachableActions-audit.metaBaseCatalogActions),
		fmt.Sprintf("  Catalog-only meta route counts: base %s / enterprise %s.\n", fmtNum(audit.metaBaseCatalogActions), fmtNum(audit.metaEnterpriseCatalogActions)),
		fmt.Sprintf("  Meta-tools reduce token overhead by %.1f%% vs individual mode\n", float64(indTotal-metaTotal)/float64(indTotal)*100),
		fmt.Sprintf("  Dynamic mode reduces visible tool token overhead by %.1f%% vs individual mode\n", float64(indTotal-dynamicTotal)/float64(indTotal)*100),
		fmt.Sprintf("  Resources (individual): ~%s tokens\n", fmtNum(audit.individualResourceTokens)),
		fmt.Sprintf("  Resources (dynamic-minimal): ~%s tokens\n", fmtNum(audit.dynamicMinimalResourceTokens)),
		fmt.Sprintf("  Prompts (full): ~%s tokens\n", fmtNum(audit.promptTokens)),
		"  Prompts (dynamic-minimal): ~0 tokens (0 bytes)\n",
		fmt.Sprintf("  Meta-tool total:  ~%s tokens\n", fmtNum(audit.metaBaseResourceTokens+audit.promptTokens)),
		fmt.Sprintf("  Shared-overhead reduction: %.1f%% vs full dynamic resources+prompts\n", float64(sharedFull-audit.dynamicMinimalResourceTokens)/float64(sharedFull)*100),
		fmt.Sprintf("  Individual mode: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n", fmtNum(indTotal), fmtNum(audit.individualResourceTokens+audit.promptTokens), fmtNum(indTotal+audit.individualResourceTokens+audit.promptTokens)),
		fmt.Sprintf("  Meta-tool mode:  ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n", fmtNum(metaTotal), fmtNum(audit.metaBaseResourceTokens+audit.promptTokens), fmtNum(metaTotal+audit.metaBaseResourceTokens+audit.promptTokens)),
		fmt.Sprintf("  Dynamic mode:    ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n", fmtNum(dynamicTotal), fmtNum(sharedFull), fmtNum(dynamicTotal+sharedFull)),
		fmt.Sprintf("  Dynamic minimal: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n", fmtNum(dynamicTotal), fmtNum(audit.dynamicMinimalResourceTokens), fmtNum(dynamicTotal+audit.dynamicMinimalResourceTokens)),
	}
	for _, line := range lines {
		t.Run(strings.TrimSpace(strings.SplitN(line, ":", 2)[0]), func(t *testing.T) {
			if !strings.Contains(output, line) {
				t.Fatalf("report lacks %q in:\n%s", line, output)
			}
		})
	}

	// The individual ranking is capped at the requested five rows, the meta
	// and dynamic rankings list every tool, and the domain ranking at three.
	topTools := lineFields(t, output, "  1  ")
	if len(topTools) != 4 || topTools[3] != audit.individualInfo[0].Name {
		t.Fatalf("first ranked tool = %v, want the most expensive individual tool %s", topTools, audit.individualInfo[0].Name)
	}
	if strings.Count(output, "  5  ") < 1 || strings.Contains(output, "  6  "+fmtNum(audit.individualInfo[5].Tokens)) {
		t.Fatalf("individual ranking is not capped at 5 rows:\n%s", output)
	}
	for _, info := range audit.dynamicBaseInfo {
		if !strings.Contains(output, "  "+info.Name+"\n") {
			t.Fatalf("dynamic ranking lacks %s:\n%s", info.Name, output)
		}
	}
	if !strings.Contains(output, "  1  "+topDomain(audit.individualInfo)+"  ") {
		t.Fatalf("domain ranking does not start with %s:\n%s", topDomain(audit.individualInfo), output)
	}
}

// topDomain returns the domain with the highest summed token cost.
func topDomain(infos []toolTokenInfo) string {
	totals := map[string]int{}
	for _, info := range infos {
		totals[info.Domain] += info.Tokens
	}
	best, bestTokens := "", -1
	for domain, tokens := range totals {
		if tokens > bestTokens || (tokens == bestTokens && domain < best) {
			best, bestTokens = domain, tokens
		}
	}
	return best
}

// TestPrintTokenAuditReport_ZeroMeasurements_OmitsRatios verifies an empty
// measurement prints the fixed sections and zero totals but none of the
// derived percentages, which would divide by zero.
func TestPrintTokenAuditReport_ZeroMeasurements_OmitsRatios(t *testing.T) {
	output := captureStdoutAudit(t, func() {
		printTokenAuditReport(tokenAudit{}, 30, 20)
	})

	for _, absent := range []string{"reduce token overhead", "reduces visible tool token overhead", "Reachable action counts include", "Shared-overhead reduction"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(output, absent) {
				t.Fatalf("report prints %q for an empty measurement:\n%s", absent, output)
			}
		})
	}
	if !strings.Contains(output, "  Individual mode: ~0 tokens (tools) + ~0 tokens (resources+prompts) = ~0 tokens\n") {
		t.Fatalf("report lacks the zero grand total:\n%s", output)
	}
}

// TestRenderReadmeFootprint_DynamicOnlyRows_KeepsOneRowPerTier verifies the README footprint
// table keeps only the dynamic surface (default configuration) across all
// tiers and links to the detailed reference, without the meta/individual rows.
func TestRenderReadmeFootprint_DynamicOnlyRows_KeepsOneRowPerTier(t *testing.T) {
	rows := []tokenFootprintRow{
		{Tier: "Free/CE", Configuration: "`dynamic` / `full` (default)", VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: "Free/CE", Configuration: "`dynamic` / `minimal`", VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 1088},
		{Tier: "Free/CE", Configuration: "`meta` / `full` (opaque)", MetaParamSchema: "opaque", VisibleTools: 33, ToolSchemaTokens: 136890, SharedTokens: 31758},
		{Tier: "Ultimate", Configuration: "`individual` / `full`", VisibleTools: 1061, ToolSchemaTokens: 964044, SharedTokens: 31758},
	}
	got := renderReadmeFootprint(rows)
	for _, want := range []string{
		"`dynamic` / `full` (default)",
		"Free/CE",
		"Token Footprint Reference",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Fatalf("renderReadmeFootprint() missing %q", want)
			}
		})
	}
	if strings.Contains(got, "`meta`") {
		t.Fatal("renderReadmeFootprint() should not include meta rows")
	}
	if strings.Contains(got, "`individual`") {
		t.Fatal("renderReadmeFootprint() should not include individual rows")
	}
}

// TestRenderReadmeFootprint_DynamicRowWithSchemaMode_RendersSchemaCell
// verifies a dynamic row that carries a META_PARAM_SCHEMA value renders it in
// the schema column instead of "n/a", so the column never hides a mode a
// future measurement records.
func TestRenderReadmeFootprint_DynamicRowWithSchemaMode_RendersSchemaCell(t *testing.T) {
	rows := []tokenFootprintRow{
		{Tier: "Free/CE", Configuration: "`dynamic` / `full` (default)", MetaParamSchema: "compact", VisibleTools: 2, ReachableActions: 851, ToolSchemaTokens: 1501, SharedTokens: 8832},
	}
	got := renderReadmeFootprint(rows)
	if !strings.Contains(got, "| `compact`") {
		t.Fatalf("renderReadmeFootprint() lacks the schema cell:\n%s", got)
	}
	if strings.Contains(got, "| n/a") {
		t.Fatalf("renderReadmeFootprint() rendered n/a for a row with a schema mode:\n%s", got)
	}
}

// TestFootprintStaleTargets_DriftPerTarget_NamesOnlyTheDivergingFile verifies
// the drift detector backing -footprint -check: it reports no stale targets
// when both README blocks, the detailed doc and the site data match the
// rendered content, and names exactly the target that diverges otherwise. A
// README that lacks the token-claim markers is a stale target, not an error,
// while missing footprint markers still abort the check.
func TestFootprintStaleTargets_DriftPerTarget_NamesOnlyTheDivergingFile(t *testing.T) {
	rows := completeFootprintRows()
	claim, claimErr := renderReadmeTokenClaim(rows)
	if claimErr != nil {
		t.Fatalf("renderReadmeTokenClaim() error: %v", claimErr)
	}
	claimBlock := claimStartMarker + "\n\n" + claim + "\n" + claimEndMarker + "\n"
	footprintBlock := footprintStartMarker + "\n\n" + renderReadmeFootprint(rows) + "\n" + footprintEndMarker + "\n"
	// A README whose two managed blocks already hold the rendered content.
	readme := claimBlock + "\n" + footprintBlock
	detailed := renderDetailedFootprint(rows)
	siteJSON, renderErr := renderSiteFootprintJSON(rows)
	if renderErr != nil {
		t.Fatalf("renderSiteFootprintJSON() error: %v", renderErr)
	}
	site := string(siteJSON)

	tests := []struct {
		name      string
		readme    string
		detailed  string
		site      string
		wantErr   bool
		wantStale string // the single stale target expected; "" means none
	}{
		{name: "current content reports no stale targets", readme: readme, detailed: detailed, site: site},
		{name: "README footprint drift reports only the footprint section", readme: strings.Replace(readme, "Rows use the base", "Rows use the stale", 1), detailed: detailed, site: site, wantStale: readmePath + " token-footprint section"},
		{name: "README claim drift reports only the claim block", readme: strings.Replace(readme, "Two tools reach", "Three tools reach", 1), detailed: detailed, site: site, wantStale: readmePath + " token-claim block"},
		{name: "missing claim markers reports the claim block as stale", readme: footprintBlock, detailed: detailed, site: site, wantStale: readmePath + " token-claim block (markers missing)"},
		{name: "detailed-doc drift reports only the detailed doc", readme: readme, detailed: detailed + "\nextra drift\n", site: site, wantStale: detailedFootprintPath},
		{name: "site JSON drift reports only the site data file", readme: readme, detailed: detailed, site: `{"tokenizer":"stale"}`, wantStale: siteFootprintPath},
		{name: "missing footprint markers returns an error", readme: claimBlock, detailed: detailed, site: site, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stale, err := footprintStaleTargets(tt.readme, tt.detailed, tt.site, rows)
			if tt.wantErr {
				if err == nil {
					t.Fatal("footprintStaleTargets() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("footprintStaleTargets() error: %v", err)
			}
			if tt.wantStale == "" {
				if len(stale) != 0 {
					t.Fatalf("stale = %v, want none", stale)
				}
				return
			}
			if len(stale) != 1 || stale[0] != tt.wantStale {
				t.Fatalf("stale = %v, want only %q", stale, tt.wantStale)
			}
		})
	}
}

// TestFootprintStaleTargets_UnrenderableRows_ReturnsError verifies the drift
// detector fails rather than reporting drift when the rows cannot render the
// token claim (no dynamic rows) or the site data (an individual tier
// missing), since a comparison against nothing would mislabel the README.
func TestFootprintStaleTargets_UnrenderableRows_ReturnsError(t *testing.T) {
	readme := claimStartMarker + "\n" + claimEndMarker + "\n" + footprintStartMarker + "\n" + footprintEndMarker + "\n"
	tests := []struct {
		name string
		rows []tokenFootprintRow
		want string
	}{
		{
			name: "no dynamic rows fails the claim",
			rows: []tokenFootprintRow{{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698, SharedTokens: 31758}},
			want: "token claim: no `dynamic` / `full` (default) row to quote",
		},
		{
			name: "missing individual tier fails the site data",
			rows: slices.DeleteFunc(completeFootprintRows(), func(r tokenFootprintRow) bool {
				return r.Tier == "Premium" && r.Configuration == individualConfiguration
			}),
			want: "expected 3 individual-surface rows, found 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stale, err := footprintStaleTargets(readme, "", "", tt.rows)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("footprintStaleTargets() = (%v, %v), want error containing %q", stale, err, tt.want)
			}
		})
	}
}

// TestRenderReadmeTokenClaim_TiersAgree_StatesOneFigure verifies the README
// headline claim quotes one startup total when every tier's dynamic rows cost
// the same, says that the tiers agree, quotes the minimal capability surface
// beside it, and points at the footprint section that backs it.
func TestRenderReadmeTokenClaim_TiersAgree_StatesOneFigure(t *testing.T) {
	got, err := renderReadmeTokenClaim(completeFootprintRows())
	if err != nil {
		t.Fatalf("renderReadmeTokenClaim() error: %v", err)
	}
	// 2,180 tool schema tokens + 31,758 shared (full) and + 1,088 shared (minimal).
	wantPrefix := "**33,938 tokens of startup context by default, the same on every GitLab tier (3,268 with `GITLAB_MCP_CAPABILITY_SURFACE=minimal`).**"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("renderReadmeTokenClaim() = %q, want prefix %q", got, wantPrefix)
	}
	for _, want := range []string{"cl100k_base", "[How it is measured](#token-footprint)"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("renderReadmeTokenClaim() missing %q in %q", want, got)
			}
		})
	}
	if strings.Contains(got, "from ") || strings.Contains(got, "From ") {
		t.Errorf("renderReadmeTokenClaim() = %q, must not render a span when the tiers agree", got)
	}
	// One trailing newline: ComputeReplacedSection adds the blank line before
	// the end marker itself, so a second one would leave the block drifting.
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("renderReadmeTokenClaim() = %q, want exactly one trailing newline", got)
	}
}

// TestRenderReadmeTokenClaim_TiersDiffer_StatesTheSpan verifies the claim
// switches to a "from X to Y" span, and stops saying every tier agrees, as soon
// as one tier's dynamic total diverges. The default and minimal figures are
// spanned independently, so a divergence in one does not fabricate one in the
// other.
func TestRenderReadmeTokenClaim_TiersDiffer_StatesTheSpan(t *testing.T) {
	tests := []struct {
		name          string
		tier          string
		configuration string
		sharedDelta   int
		wantPrefix    string
	}{
		{
			name: "default total differs", tier: "Premium", configuration: dynamicDefaultConfiguration, sharedDelta: 100,
			wantPrefix: "**From 33,938 to 34,038 tokens of startup context by default, depending on the GitLab tier (3,268 with `GITLAB_MCP_CAPABILITY_SURFACE=minimal`).**",
		},
		{
			name: "minimal total differs", tier: "Free/CE", configuration: dynamicMinimalConfiguration, sharedDelta: 7,
			wantPrefix: "**33,938 tokens of startup context by default, the same on every GitLab tier (from 3,268 to 3,275 with `GITLAB_MCP_CAPABILITY_SURFACE=minimal`).**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := completeFootprintRows()
			for i := range rows {
				if rows[i].Tier == tt.tier && rows[i].Configuration == tt.configuration {
					rows[i].SharedTokens += tt.sharedDelta
				}
			}
			got, err := renderReadmeTokenClaim(rows)
			if err != nil {
				t.Fatalf("renderReadmeTokenClaim() error: %v", err)
			}
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("renderReadmeTokenClaim() = %q, want prefix %q", got, tt.wantPrefix)
			}
		})
	}
}

// TestRenderReadmeTokenClaim_MissingDynamicRows_ReturnsError verifies the claim
// refuses to render, rather than quoting zero tokens, when either dynamic
// configuration has no measured row to draw on.
func TestRenderReadmeTokenClaim_MissingDynamicRows_ReturnsError(t *testing.T) {
	tests := []struct {
		name string
		rows []tokenFootprintRow
	}{
		{name: "no rows at all", rows: nil},
		{
			name: "only the individual surface",
			rows: []tokenFootprintRow{{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698, SharedTokens: 31758}},
		},
		{
			name: "minimal capability surface missing",
			rows: slices.DeleteFunc(completeFootprintRows(), func(r tokenFootprintRow) bool {
				return r.Configuration == dynamicMinimalConfiguration
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := renderReadmeTokenClaim(tt.rows); err == nil {
				t.Fatal("renderReadmeTokenClaim() error = nil, want error")
			}
		})
	}
}

// completeFootprintRows returns a fixture covering every row the site-facing
// JSON needs: the dynamic default on the Ultimate tier plus one individual row
// per tier.
func completeFootprintRows() []tokenFootprintRow {
	return []tokenFootprintRow{
		{Tier: "Free/CE", Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: "Free/CE", Configuration: dynamicMinimalConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 1088},
		{Tier: "Free/CE", Configuration: individualConfiguration, VisibleTools: 847, ToolSchemaTokens: 767793, SharedTokens: 31758},
		{Tier: "Premium", Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: "Premium", Configuration: dynamicMinimalConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 1088},
		{Tier: "Premium", Configuration: individualConfiguration, VisibleTools: 999, ToolSchemaTokens: 917625, SharedTokens: 31758},
		{Tier: ultimateTierLabel, Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: ultimateTierLabel, Configuration: dynamicMinimalConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 1088},
		{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698, SharedTokens: 31758},
	}
}

// fullMatrixFootprintRows returns a 27-row fixture in the exact order and
// shape measureTokenFootprintRows produces: per tier, the two dynamic rows,
// the full and minimal meta rows under each schema mode, then the individual
// row.
func fullMatrixFootprintRows() []tokenFootprintRow {
	tiers := []struct {
		label      string
		reachable  int
		individual int
		tokens     int
	}{
		{"Free/CE", 851, 847, 494314},
		{"Premium", 1003, 999, 592723},
		{ultimateTierLabel, 1069, 1065, 621737},
	}
	metaTokens := map[string]int{"opaque": 136890, "compact": 210000, "full": 330000}
	var rows []tokenFootprintRow
	for _, tier := range tiers {
		rows = append(rows,
			tokenFootprintRow{Tier: tier.label, Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ReachableActions: tier.reachable, ToolSchemaTokens: 1501, SharedTokens: 8832},
			tokenFootprintRow{Tier: tier.label, Configuration: dynamicMinimalConfiguration, VisibleTools: 2, ReachableActions: tier.reachable, ToolSchemaTokens: 1501, SharedTokens: 170},
		)
		for _, mode := range []string{"opaque", "compact", "full"} {
			rows = append(rows,
				tokenFootprintRow{Tier: tier.label, Configuration: fmt.Sprintf("`meta` / `full` (%s)", mode), MetaParamSchema: mode, VisibleTools: 40, ReachableActions: tier.reachable, ToolSchemaTokens: metaTokens[mode], SharedTokens: 9000},
				tokenFootprintRow{Tier: tier.label, Configuration: fmt.Sprintf("`meta` / `minimal` (%s)", mode), MetaParamSchema: mode, VisibleTools: 40, ReachableActions: tier.reachable, ToolSchemaTokens: metaTokens[mode], SharedTokens: 300},
			)
		}
		rows = append(rows, tokenFootprintRow{Tier: tier.label, Configuration: individualConfiguration, VisibleTools: tier.individual, ReachableActions: tier.individual, ToolSchemaTokens: tier.tokens, SharedTokens: 8832})
	}
	return rows
}

// TestRenderDetailedFootprint_FullMatrix_RendersEveryRow verifies the
// reference doc lists all 27 configurations with their schema mode in the
// schema column (n/a for the surfaces without one) and the documented column
// legend before the table.
func TestRenderDetailedFootprint_FullMatrix_RendersEveryRow(t *testing.T) {
	rows := fullMatrixFootprintRows()
	got := renderDetailedFootprint(rows)

	if !strings.HasPrefix(got, "# Token Footprint Reference\n\n") {
		t.Fatalf("renderDetailedFootprint() does not start with the title:\n%s", got)
	}
	assertBefore(t, got, "## What each column means", "## Full matrix")
	assertBefore(t, got, "## Full matrix", "## Interpretation guide")
	for _, row := range rows {
		t.Run(row.Tier+" "+row.Configuration, func(t *testing.T) {
			schemaCell := "n/a"
			if row.MetaParamSchema != "" {
				schemaCell = "`" + row.MetaParamSchema + "`"
			}
			wantCells := []string{row.Configuration, row.Tier, fmtNum(row.VisibleTools), fmtNum(row.ReachableActions), schemaCell, fmtNum(row.ToolSchemaTokens), fmtNum(row.SharedTokens), fmtNum(row.totalTokens())}
			if !containsTableRow(got, wantCells) {
				t.Fatalf("renderDetailedFootprint() lacks the row %v:\n%s", wantCells, got)
			}
		})
	}
	if strings.Count(got, "\n| `") != len(rows) {
		t.Fatalf("renderDetailedFootprint() rendered %d configuration rows, want %d", strings.Count(got, "\n| `"), len(rows))
	}
}

// containsTableRow reports whether doc has a Markdown table row whose trimmed
// cells equal cells.
func containsTableRow(doc string, cells []string) bool {
	for line := range strings.SplitSeq(doc, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		if len(parts) != len(cells) {
			continue
		}
		match := true
		for i, part := range parts {
			if strings.TrimSpace(part) != cells[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestRenderSiteFootprintJSON_CompleteMatrix_DerivesReductionFactor verifies the site-facing headline extract: the
// dynamic surface is taken from the Ultimate tier, every tier gets an
// individual-surface entry, and the reduction factor is derived rather than
// restated. This is the number the docs site publishes as its headline claim,
// so a wrong ratio would propagate straight into AI answers.
func TestRenderSiteFootprintJSON_CompleteMatrix_DerivesReductionFactor(t *testing.T) {
	raw, renderErr := renderSiteFootprintJSON(completeFootprintRows())
	if renderErr != nil {
		t.Fatalf("renderSiteFootprintJSON() error: %v", renderErr)
	}

	var got siteFootprint
	if unmarshalErr := json.Unmarshal(raw, &got); unmarshalErr != nil {
		t.Fatalf("unmarshal site footprint: %v", unmarshalErr)
	}

	if got.Tokenizer != "cl100k_base" {
		t.Errorf("Tokenizer = %q, want cl100k_base", got.Tokenizer)
	}
	if got.Dynamic.VisibleTools != 2 || got.Dynamic.ToolSchemaTokens != 2180 {
		t.Errorf("Dynamic = %+v, want {2 2180}", got.Dynamic)
	}
	// The site quotes these two in prose; publishing a zero would read as
	// "resources and prompts are free" rather than as missing data.
	if got.Shared.Full != 31758 || got.Shared.Minimal != 1088 {
		t.Errorf("Shared = %+v, want {31758 1088}", got.Shared)
	}
	if len(got.Individual) != 3 {
		t.Fatalf("len(Individual) = %d, want 3", len(got.Individual))
	}
	// 966698 / 2180 = 443.4 -> 443
	if f := got.Individual["ultimate"].ReductionFactor; f != 443 {
		t.Errorf("ultimate ReductionFactor = %d, want 443", f)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Error("site footprint JSON must end with a trailing newline for prettier")
	}
}

// TestRenderSiteFootprintJSON_IncompleteMatrix_ReturnsError verifies generation fails
// loudly rather than publishing a partial headline claim.
func TestRenderSiteFootprintJSON_IncompleteMatrix_ReturnsError(t *testing.T) {
	tests := []struct {
		name string
		rows []tokenFootprintRow
		want string
	}{
		{
			name: "missing the ultimate dynamic row",
			rows: []tokenFootprintRow{
				{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698},
			},
			want: "no `dynamic` / `full` (default) row found for the Ultimate tier",
		},
		{
			name: "missing shared counts",
			rows: []tokenFootprintRow{
				{Tier: ultimateTierLabel, Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180},
				{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698},
			},
			want: "missing shared token counts for the Ultimate tier (full=0, minimal=0)",
		},
		{
			name: "missing the minimal capability-surface row",
			rows: slices.DeleteFunc(completeFootprintRows(), func(r tokenFootprintRow) bool {
				return r.Configuration == dynamicMinimalConfiguration
			}),
			want: "missing shared token counts for the Ultimate tier (full=31758, minimal=0)",
		},
		{
			name: "missing one individual tier",
			rows: slices.DeleteFunc(completeFootprintRows(), func(r tokenFootprintRow) bool {
				return r.Tier == "Free/CE" && r.Configuration == individualConfiguration
			}),
			want: "expected 3 individual-surface rows, found 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderSiteFootprintJSON(tt.rows)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("renderSiteFootprintJSON() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestRenderSiteFootprintJSON_TierDependentDynamic_ReturnsError verifies the
// site data refuses to quote one dynamic figure for every tier once any tier's
// dynamic row diverges from the Ultimate one: the landing page says in words
// that the default cost holds on every tier, and that sentence must fail to
// generate before it can become false.
func TestRenderSiteFootprintJSON_TierDependentDynamic_ReturnsError(t *testing.T) {
	tests := []struct {
		name          string
		configuration string
		field         string
	}{
		{name: "default shared tokens differ", configuration: dynamicDefaultConfiguration, field: "shared"},
		{name: "minimal shared tokens differ", configuration: dynamicMinimalConfiguration, field: "shared"},
		{name: "tool schema tokens differ", configuration: dynamicDefaultConfiguration, field: "schema"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := completeFootprintRows()
			for i := range rows {
				if rows[i].Tier != "Premium" || rows[i].Configuration != tt.configuration {
					continue
				}
				if tt.field == "shared" {
					rows[i].SharedTokens++
				} else {
					rows[i].ToolSchemaTokens++
				}
			}
			_, err := renderSiteFootprintJSON(rows)
			if err == nil {
				t.Fatal("renderSiteFootprintJSON() error = nil, want error")
			}
			if !strings.Contains(err.Error(), "Premium") {
				t.Fatalf("renderSiteFootprintJSON() error = %q, want it to name the diverging tier", err)
			}
		})
	}
}

// TestMeasureTokenFootprintRows_AllTiersAllModes_CoversEveryCombination verifies the full measurement
// matrix: 3 tiers \u00d7 9 configurations, tier ordering, meta schema-mode token
// scaling, and tier-based individual tool scaling.
func TestMeasureTokenFootprintRows_AllTiersAllModes_CoversEveryCombination(t *testing.T) {
	rows := measuredFootprintRows(t)
	if len(rows) != 27 {
		t.Fatalf("measureTokenFootprintRows() returned %d rows, want 27 (3 tiers \u00d7 9)", len(rows))
	}
	// Verify tier ordering: Free/CE first 9, Premium next 9, Ultimate last 9.
	tiers := []string{"Free/CE", "Premium", "Ultimate"}
	for ti, tier := range tiers {
		t.Run(tier, func(t *testing.T) {
			for i := range 9 {
				row := rows[ti*9+i]
				if row.Tier != tier {
					t.Fatalf("row[%d].Tier = %q, want %q", ti*9+i, row.Tier, tier)
				}
			}
		})
	}
	// Verify mode ordering within each tier: opaque < compact < full tokens.
	for ti, tier := range tiers {
		t.Run(tier+" meta modes increase", func(t *testing.T) {
			base := ti * 9
			opaque := rows[base+2].ToolSchemaTokens
			compact := rows[base+4].ToolSchemaTokens
			full := rows[base+6].ToolSchemaTokens
			if compact <= opaque || full <= compact {
				t.Fatalf("tier %s: meta tokens not increasing opaque(%d) < compact(%d) < full(%d)", tier, opaque, compact, full)
			}
		})
	}
	// Verify tier scaling: Free individual tools < Premium < Ultimate.
	freeIndiv := rows[8].VisibleTools
	premIndiv := rows[17].VisibleTools
	ultIndiv := rows[26].VisibleTools
	if freeIndiv >= premIndiv || premIndiv >= ultIndiv {
		t.Fatalf("individual tool count not increasing by tier: Free(%d) < Premium(%d) < Ultimate(%d)", freeIndiv, premIndiv, ultIndiv)
	}
}

// TestMeasureTierFootprintWithPrompts_NegativePromptTokens_MeasuresPromptsItself
// verifies a standalone caller that passes a negative prompt figure gets the
// prompts measured in place, and that the tier's rows then equal the ones the
// batch measurement produced with the prompt figure passed in.
func TestMeasureTierFootprintWithPrompts_NegativePromptTokens_MeasuresPromptsItself(t *testing.T) {
	want := measuredFootprintRows(t)[:9]

	got := measureTierFootprintWithPrompts(newAuditTokensClient(t), edition.Free, "Free/CE", -1)
	if !slices.Equal(got, want) {
		t.Fatalf("standalone Free tier rows =\n%+v\nwant the batch measurement's\n%+v", got, want)
	}
}

// TestMeasureToolSchemaTokens_RealTokenizer_ReturnsNonZeroCounts verifies the schema token
// estimator runs the cl100k_base tokenizer, not the bytes/4 fallback. It counts
// the same serialized tools both ways and asserts they differ, so a tokenizer
// init/encode regression (silently dropping to bytes/4) fails here. The
// tokenizer itself is unit-tested in tokens_test.go.
func TestMeasureToolSchemaTokens_RealTokenizer_ReturnsNonZeroCounts(t *testing.T) {
	toolList := []*mcp.Tool{{Name: "a"}, {Name: "bb"}, {Name: "ccc"}}

	got := measureToolSchemaTokens(toolList)
	if got <= 0 {
		t.Fatalf("measureToolSchemaTokens() = %d, want > 0", got)
	}

	fallback := 0
	for _, tl := range toolList {
		t.Run(tl.Name, func(t *testing.T) {
			b, marshalErr := json.Marshal(tl)
			if marshalErr != nil {
				t.Fatalf("marshal: %v", marshalErr)
			}
			fallback += len(b) / 4
		})
	}
	if got == fallback {
		t.Fatalf("measureToolSchemaTokens() = %d == bytes/4 fallback; cl100k_base tokenizer did not engage", got)
	}
}

// TestRun_CompareSchemasMode_PrintsSortedSizingTable verifies the
// meta-schema sizing builds the full enterprise meta-tool registry and prints
// one row per meta-tool sorted by full-schema size, a TOTAL row, and the two
// ratios above one that the compact and full strategies cost over opaque.
// Migrated from the former cmd/audit_meta_schema/main_test.go.
//
// It drives the mode through [run] rather than calling the sizing directly:
// the registry build is the expensive part and paying for it twice to cover
// two more statements would be the wrong trade.
func TestRun_CompareSchemasMode_PrintsSortedSizingTable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := 0
	output := captureStdoutAudit(t, func() {
		code = run(auditOptions{compareSchemas: true}, &stdout, &stderr)
	})
	assertRunPrintedToStdout(t, code, &stdout, &stderr)

	if !strings.Contains(output, " Meta-tool InputSchema sizing spike\n") {
		t.Fatalf("sizing output lacks its title:\n%s", output)
	}
	header := lineFields(t, output, "meta-tool ")
	if !slices.Equal(header, []string{"meta-tool", "actions", "opaque", "full", "compact", "delta", "full"}) {
		t.Fatalf("header fields = %v", header)
	}

	var fullBytes []float64
	names := map[string]bool{}
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.HasPrefix(line, "gitlab_") {
			continue
		}
		fields := strings.Fields(line)
		// name, actions, then four "<n> <unit>" pairs (opaque, full, compact, delta).
		if len(fields) != 10 {
			t.Fatalf("row %q has %d fields, want 10", line, len(fields))
		}
		names[fields[0]] = true
		if actions, convErr := strconv.Atoi(fields[1]); convErr != nil || actions <= 0 {
			t.Fatalf("row %q has no positive action count", line)
		}
		fullBytes = append(fullBytes, parseHumanBytes(t, fields[4]+" "+fields[5]))
	}
	if len(fullBytes) < 30 {
		t.Fatalf("sizing table has %d meta-tool rows, want the full enterprise registry", len(fullBytes))
	}
	for _, want := range []string{"gitlab_project", "gitlab_issue", "gitlab_merge_request", "gitlab_geo"} {
		t.Run(want, func(t *testing.T) {
			if !names[want] {
				t.Fatalf("sizing table lacks %s", want)
			}
		})
	}
	if !slices.IsSortedFunc(fullBytes, func(a, b float64) int { return int(b - a) }) {
		t.Fatalf("rows are not sorted by descending full-schema size: %v", fullBytes)
	}

	total := lineFields(t, output, "TOTAL")
	if len(total) != 7 || total[0] != "TOTAL" {
		t.Fatalf("TOTAL row fields = %v", total)
	}
	for _, ratioLine := range []string{"Full / opaque   ratio: ", "Compact / opaque ratio: "} {
		t.Run(strings.TrimSpace(ratioLine), func(t *testing.T) {
			fields := lineFields(t, output, ratioLine)
			ratio, convErr := strconv.ParseFloat(strings.TrimSuffix(fields[len(fields)-1], "x"), 64)
			if convErr != nil || ratio <= 1 {
				t.Fatalf("%q ratio = %v (%v), want a factor above 1", ratioLine, fields[len(fields)-1], convErr)
			}
		})
	}
}

// parseHumanBytes converts a humanBytes rendering back to a byte figure for
// ordering comparisons.
func parseHumanBytes(t *testing.T, s string) float64 {
	t.Helper()
	fields := strings.Fields(s)
	if len(fields) != 2 {
		t.Fatalf("%q is not a <number> <unit> size", s)
	}
	value, err := strconv.ParseFloat(strings.TrimPrefix(fields[0], "+"), 64)
	if err != nil {
		t.Fatalf("%q is not a size: %v", s, err)
	}
	switch fields[1] {
	case "B":
		return value
	case "KB":
		return value * 1024
	case "MB":
		return value * 1024 * 1024
	default:
		t.Fatalf("%q has an unknown unit", s)
		return 0
	}
}

// TestHumanBytes_AllMagnitudes_FormatsWithUnitSuffix verifies the humanBytes byte formatter emits
// expected B/KB/MB suffixes for the three supported magnitude ranges.
func TestHumanBytes_AllMagnitudes_FormatsWithUnitSuffix(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "sub-kilobyte renders as B", n: 512, want: "512 B"},
		{name: "kilobyte threshold renders as KB", n: 1024, want: "1.0 KB"},
		{name: "megabyte threshold renders as MB", n: 1024 * 1024, want: "1.0 MB"},
		{name: "large value renders as MB", n: 3 * 1024 * 1024, want: "3.0 MB"},
		{name: "zero renders as B", n: 0, want: "0 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanBytes(tt.n); got != tt.want {
				t.Fatalf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// --- Footprint write and check modes ------------------------------------------

// stubFootprintRows replaces the footprint measurement for the test with a
// function returning rows, restoring the real one afterwards.
func stubFootprintRows(t *testing.T, rows []tokenFootprintRow) {
	t.Helper()
	original := measureFootprintRows
	measureFootprintRows = func(*gitlabclient.Client) []tokenFootprintRow { return rows }
	t.Cleanup(func() { measureFootprintRows = original })
}

// footprintSkeleton is a README whose two managed blocks are empty.
const footprintSkeleton = "# Fixture\n\n" + claimStartMarker + "\n" + claimEndMarker + "\n\n## Token Footprint\n\n" + footprintStartMarker + "\n" + footprintEndMarker + "\n"

// renderedReadme returns the skeleton README with both managed blocks holding
// the content rendered from rows.
func renderedReadme(t *testing.T, rows []tokenFootprintRow) string {
	t.Helper()
	claim, err := renderReadmeTokenClaim(rows)
	if err != nil {
		t.Fatalf("renderReadmeTokenClaim() error: %v", err)
	}
	readme, err := docgen.ComputeReplacedSection(footprintSkeleton, claimStartMarker, claimEndMarker, claim)
	if err != nil {
		t.Fatalf("replace claim block: %v", err)
	}
	readme, err = docgen.ComputeReplacedSection(readme, footprintStartMarker, footprintEndMarker, renderReadmeFootprint(rows))
	if err != nil {
		t.Fatalf("replace footprint block: %v", err)
	}
	return readme
}

// footprintReplica creates a directory laid out like the repository root for
// the footprint targets, with the README, the detailed doc and the site data
// holding the given contents, and makes it the working directory for the rest
// of the test. An empty content leaves that file absent.
func footprintReplica(t *testing.T, readme, detailed, site string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{filepath.Dir(detailedFootprintPath), filepath.Dir(siteFootprintPath)} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for path, content := range map[string]string{readmePath: readme, detailedFootprintPath: detailed, siteFootprintPath: site} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	t.Chdir(root)
	return root
}

// readReplica returns the content of path under root.
func readReplica(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path)) //#nosec G304 -- temp path built by the test
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestRunFootprintCheck_CommittedTargets_AreCurrent verifies the check mode
// accepts the committed README blocks, reference doc and site data for the
// live measurement, which is the exact call `make check-footprint` makes, and
// says how many rows it compared.
func TestRunFootprintCheck_CommittedTargets_AreCurrent(t *testing.T) {
	rows := measuredFootprintRows(t)
	stubFootprintRows(t, rows)
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	t.Chdir(root)

	var checkErr error
	output := captureStdoutAudit(t, func() {
		checkErr = runFootprintCheck(newAuditTokensClient(t))
	})

	if checkErr != nil {
		t.Fatalf("runFootprintCheck() error: %v (regenerate with: go run ./cmd/audit_tokens/ -footprint)", checkErr)
	}
	if output != "Token footprint is current (27 rows across all tiers/surfaces/modes)\n" {
		t.Fatalf("runFootprintCheck() output = %q", output)
	}
}

// TestRunFootprintCheck_Failures_NameTheCause verifies each way the check can
// fail is reported by name: each target that cannot be read, a README without
// footprint markers, and every stale target listed together with the command
// that refreshes them. The measurement itself is not among them — it reads
// nothing but the catalog compiled into this binary and panics rather than
// returning.
func TestRunFootprintCheck_Failures_NameTheCause(t *testing.T) {
	rows := fullMatrixFootprintRows()
	readme := renderedReadme(t, rows)
	detailed := renderDetailedFootprint(rows)
	siteJSON, err := renderSiteFootprintJSON(rows)
	if err != nil {
		t.Fatalf("renderSiteFootprintJSON() error: %v", err)
	}
	site := string(siteJSON)

	tests := []struct {
		name     string
		readme   string
		detailed string
		site     string
		want     string
	}{
		{name: "README missing", detailed: detailed, site: site, want: "reading README.md: "},
		{name: "detailed doc missing", readme: readme, site: site, want: "reading docs/development/token-footprint.md: "},
		{name: "site data missing", readme: readme, detailed: detailed, want: "reading site/src/data/token-footprint.json: "},
		{name: "README without footprint markers", readme: "# Fixture\n" + claimStartMarker + "\n" + claimEndMarker + "\n", detailed: detailed, site: site, want: "start marker " + footprintStartMarker + " not found"},
		{name: "every target stale", readme: footprintSkeleton, detailed: "old reference\n", site: "{}\n", want: "token footprint is stale (README.md token-footprint section; README.md token-claim block; docs/development/token-footprint.md; site/src/data/token-footprint.json); run: go run ./cmd/audit_tokens/ -footprint"},
		{name: "only the site data stale", readme: readme, detailed: detailed, site: "{}\n", want: "token footprint is stale (site/src/data/token-footprint.json); run: go run ./cmd/audit_tokens/ -footprint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubFootprintRows(t, rows)
			footprintReplica(t, tt.readme, tt.detailed, tt.site)

			checkErr := runFootprintCheck(newAuditTokensClient(t))
			if checkErr == nil || !strings.Contains(checkErr.Error(), tt.want) {
				t.Fatalf("runFootprintCheck() error = %v, want it to contain %q", checkErr, tt.want)
			}
		})
	}
}

// TestRunFootprint_EmptyReplica_WritesEveryTarget verifies the write mode
// fills both README blocks, writes the reference doc and the site data from
// the rows it measured, reports what it updated, and leaves the tree in the
// state the check mode then accepts.
func TestRunFootprint_EmptyReplica_WritesEveryTarget(t *testing.T) {
	rows := fullMatrixFootprintRows()
	stubFootprintRows(t, rows)
	root := footprintReplica(t, footprintSkeleton, "stale\n", "{}\n")

	var runErr error
	output := captureStdoutAudit(t, func() {
		runErr = runFootprint(newAuditTokensClient(t))
	})
	if runErr != nil {
		t.Fatalf("runFootprint() error: %v", runErr)
	}
	if output != "Updated README.md token-claim block and token-footprint section, docs/development/token-footprint.md and site/src/data/token-footprint.json (27 rows across all tiers/surfaces/modes)\n" {
		t.Fatalf("runFootprint() output = %q", output)
	}

	if got, want := readReplica(t, root, readmePath), renderedReadme(t, rows); got != want {
		t.Fatalf("README =\n%s\nwant\n%s", got, want)
	}
	if got, want := readReplica(t, root, detailedFootprintPath), renderDetailedFootprint(rows); got != want {
		t.Fatalf("reference doc =\n%s\nwant\n%s", got, want)
	}
	wantSite, err := renderSiteFootprintJSON(rows)
	if err != nil {
		t.Fatalf("renderSiteFootprintJSON() error: %v", err)
	}
	if got := readReplica(t, root, siteFootprintPath); got != string(wantSite) {
		t.Fatalf("site data =\n%s\nwant\n%s", got, wantSite)
	}
	if checkErr := runFootprintCheck(newAuditTokensClient(t)); checkErr != nil {
		t.Fatalf("runFootprintCheck() after runFootprint() error: %v", checkErr)
	}
}

// TestRunFootprint_Failures_ReturnErrors verifies each failure of the write
// mode is returned rather than half-applied silently: rows the claim or the
// site data cannot be rendered from, a README that is missing or lacks the
// footprint markers, and target directories that do not exist.
func TestRunFootprint_Failures_ReturnErrors(t *testing.T) {
	rows := fullMatrixFootprintRows()
	tests := []struct {
		name      string
		rows      []tokenFootprintRow
		readme    string
		removeDir string
		want      string
	}{
		{name: "rows without a dynamic surface", rows: rows[8:9], readme: footprintSkeleton, want: "token claim: no `dynamic` / `full` (default) row to quote"},
		{name: "README missing", rows: rows, want: "reading README.md: "},
		{name: "README without footprint markers", rows: rows, readme: "# Fixture\n" + claimStartMarker + "\n" + claimEndMarker + "\n", want: "start marker " + footprintStartMarker + " not found"},
		{name: "reference doc directory missing", rows: rows, readme: footprintSkeleton, removeDir: filepath.Dir(detailedFootprintPath), want: "writing docs/development/token-footprint.md: "},
		{name: "rows missing an individual tier", rows: rows[:26], readme: footprintSkeleton, want: "expected 3 individual-surface rows, found 2"},
		{name: "site data directory missing", rows: rows, readme: footprintSkeleton, removeDir: filepath.Dir(siteFootprintPath), want: "writing site/src/data/token-footprint.json: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubFootprintRows(t, tt.rows)
			root := footprintReplica(t, tt.readme, "", "")
			if tt.removeDir != "" {
				if err := os.RemoveAll(filepath.Join(root, tt.removeDir)); err != nil {
					t.Fatalf("remove %s: %v", tt.removeDir, err)
				}
			}

			runErr := runFootprint(newAuditTokensClient(t))
			if runErr == nil || !strings.Contains(runErr.Error(), tt.want) {
				t.Fatalf("runFootprint() error = %v, want it to contain %q", runErr, tt.want)
			}
		})
	}
}

// TestRunFootprintMode_CheckFlag_SelectsCheckOrWrite verifies the entry point
// behind -footprint builds its own client and dispatches on the check flag:
// with it set the committed targets are verified and nothing is written,
// without it the targets are written.
func TestRunFootprintMode_CheckFlag_SelectsCheckOrWrite(t *testing.T) {
	t.Run("check verifies the committed targets", func(t *testing.T) {
		stubFootprintRows(t, measuredFootprintRows(t))
		root, err := cmdutil.RepositoryRoot(".")
		if err != nil {
			t.Fatalf("locate repository root: %v", err)
		}
		t.Chdir(root)
		before := readReplica(t, root, readmePath)

		var modeErr error
		output := captureStdoutAudit(t, func() {
			modeErr = runFootprintMode(true)
		})
		if modeErr != nil {
			t.Fatalf("runFootprintMode(check) error: %v", modeErr)
		}
		if !strings.HasPrefix(output, "Token footprint is current (") {
			t.Fatalf("runFootprintMode(check) output = %q", output)
		}
		if readReplica(t, root, readmePath) != before {
			t.Fatal("runFootprintMode(check) modified README.md")
		}
	})

	t.Run("write fills the targets", func(t *testing.T) {
		rows := fullMatrixFootprintRows()
		stubFootprintRows(t, rows)
		root := footprintReplica(t, footprintSkeleton, "", "")

		var modeErr error
		output := captureStdoutAudit(t, func() {
			modeErr = runFootprintMode(false)
		})
		if modeErr != nil {
			t.Fatalf("runFootprintMode(write) error: %v", modeErr)
		}
		if !strings.HasPrefix(output, "Updated README.md token-claim block") {
			t.Fatalf("runFootprintMode(write) output = %q", output)
		}
		if got, want := readReplica(t, root, readmePath), renderedReadme(t, rows); got != want {
			t.Fatalf("README =\n%s\nwant\n%s", got, want)
		}
		if got, want := readReplica(t, root, detailedFootprintPath), renderDetailedFootprint(rows); got != want {
			t.Fatalf("reference doc =\n%s\nwant\n%s", got, want)
		}
	})
}

// TestRun_FootprintMode_ReportsTheOutcomeAndExitCode verifies the one place
// that reports a failure does report it: a target run cannot read reaches the
// writer run was handed and becomes exit status 1, while a clean check writes
// its confirmation to stdout and exits 0.
func TestRun_FootprintMode_ReportsTheOutcomeAndExitCode(t *testing.T) {
	t.Run("unreadable target names the cause and exits one", func(t *testing.T) {
		stubFootprintRows(t, fullMatrixFootprintRows())
		footprintReplica(t, "", "", "")

		var stdout, stderr bytes.Buffer
		code := run(auditOptions{footprint: true, check: true}, &stdout, &stderr)

		if code != 1 {
			t.Fatalf("run(-footprint -check) = %d, want 1", code)
		}
		if got := stderr.String(); !strings.HasPrefix(got, "reading README.md: ") {
			t.Fatalf("stderr = %q, want the unreadable README named", got)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing written on failure", stdout.String())
		}
	})

	t.Run("current targets exit zero", func(t *testing.T) {
		stubFootprintRows(t, measuredFootprintRows(t))
		root, err := cmdutil.RepositoryRoot(".")
		if err != nil {
			t.Fatalf("locate repository root: %v", err)
		}
		t.Chdir(root)

		code := 0
		var stderr bytes.Buffer
		output := captureStdoutAudit(t, func() {
			code = run(auditOptions{footprint: true, check: true}, os.Stdout, &stderr)
		})

		if code != 0 {
			t.Fatalf("run(-footprint -check) = %d, want 0: %s", code, stderr.String())
		}
		if !strings.HasPrefix(output, "Token footprint is current (") {
			t.Fatalf("stdout = %q, want the currency confirmation", output)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing written on success", stderr.String())
		}
	})
}

// TestRun_JSONMode_WritesTheSummaryToItsWriter verifies the default audit path
// measures every surface and encodes the summary to the writer run was given,
// exiting 0. It exercises the whole chain: the measurement now panics rather
// than returning failures nothing could act on, so a clean run has to arrive
// back here intact.
func TestRun_JSONMode_WritesTheSummaryToItsWriter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(auditOptions{jsonOut: true}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run(-json) = %d, want 0: %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing written on success", stderr.String())
	}

	var summary map[string]int
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode run(-json) output %q: %v", stdout.String(), err)
	}
	for _, key := range []string{"individual_tools", "meta_base_tools", "dynamic_base_tools", "individual_tokens", "base_reachable_actions", "resource_tokens", "prompt_tokens"} {
		t.Run(key, func(t *testing.T) {
			if summary[key] <= 0 {
				t.Errorf("%s = %d, want a positive measurement", key, summary[key])
			}
		})
	}
}

// TestRun_JSONMode_EncoderFails_ReportsAndStillExitsZero verifies the one
// documented asymmetry of the entry point: a stdout that refuses the summary
// is named on stderr, and the exit code stays 0. That predates the error
// rerouting and is deliberately preserved, so it needs a test of its own to
// keep anyone from "fixing" it into a 1.
func TestRun_JSONMode_EncoderFails_ReportsAndStillExitsZero(t *testing.T) {
	var stderr bytes.Buffer
	code := run(auditOptions{jsonOut: true}, failingWriter{}, &stderr)

	if code != 0 {
		t.Fatalf("run(-json) with a failing writer = %d, want 0", code)
	}
	if got := stderr.String(); got != "encode json: disk full\n" {
		t.Fatalf("stderr = %q, want the encode failure named", got)
	}
}

// TestRun_ReportMode_PrintsTheMarkdownReport verifies the default invocation,
// the one with no flags at all: it measures every surface and prints the
// Markdown report to os.Stdout, capped at the requested ranking lengths, and
// exits 0 without writing to either writer it was handed.
func TestRun_ReportMode_PrintsTheMarkdownReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := 0
	output := captureStdoutAudit(t, func() {
		code = run(auditOptions{topTools: 3, topDomains: 2}, &stdout, &stderr)
	})
	assertRunPrintedToStdout(t, code, &stdout, &stderr)

	assertInOrder(t, output,
		"  gitlab-mcp-server. Token Overhead Audit\n",
		"## Mode Comparison\n",
		"## Top 30 Individual Tools by Token Cost\n",
		"## Grand Total (what an LLM sees)\n",
	)
	// The ranking caps come from the options, not from the section headings,
	// which name the defaults whatever was asked for.
	individual := sectionBetween(t, output, "## Top 30 Individual Tools by Token Cost\n", "## Meta-Tools by Token Cost (base)\n")
	if !strings.Contains(individual, "  3  ") || strings.Contains(individual, "  4  ") {
		t.Fatalf("individual ranking is not capped at the requested 3 rows:\n%s", individual)
	}
	domains := sectionBetween(t, output, "## Domain Totals (Individual Mode, Top 20)\n", "## Grand Total (what an LLM sees)\n")
	if !strings.Contains(domains, "  2  ") || strings.Contains(domains, "  3  ") {
		t.Fatalf("domain ranking is not capped at the requested 2 rows:\n%s", domains)
	}
}

// assertRunPrintedToStdout verifies run exited zero having written nothing to
// either writer it was handed: the modes that print a table or a report print
// it to os.Stdout, and only a failure reaches the writers.
func assertRunPrintedToStdout(t *testing.T, code int, stdout, stderr *bytes.Buffer) {
	t.Helper()
	if code != 0 {
		t.Fatalf("run() = %d, want 0: %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("run() wrote %q to its stdout writer, want the output on os.Stdout", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("run() wrote %q to its stderr writer, want nothing", stderr.String())
	}
}

// sectionBetween returns the part of s that follows start and precedes the
// first end after it, failing when either marker is absent.
func sectionBetween(t *testing.T, s, start, end string) string {
	t.Helper()
	from := strings.Index(s, start)
	if from < 0 {
		t.Fatalf("%q not found in:\n%s", start, s)
	}
	rest := s[from+len(start):]
	to := strings.Index(rest, end)
	if to < 0 {
		t.Fatalf("%q not found after %q in:\n%s", end, start, s)
	}
	return rest[:to]
}
