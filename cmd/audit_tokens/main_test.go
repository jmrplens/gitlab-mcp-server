// main_test.go contains focused tests for the audit_tokens command. Tests use
// a local GitLab version mock and exercise the resource token measurement path
// that depends on the surface-aware tool manifest resources.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// newAuditTokensClient creates a [gitlabclient.Client] backed by a mock
// /api/v4/version endpoint for audit_tokens tests.
func newAuditTokensClient(t *testing.T) *gitlabclient.Client {
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

// TestMeasureResources_IncludesToolManifest verifies the token audit measures
// the surface-aware tool manifest in addition to static resources.
func TestMeasureResources_IncludesToolManifest(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	dynamicCatalog := actioncatalog.FromActionMaps(routes)
	dynamicTools := listDynamicTools(dynamicCatalog)
	manifestTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest:   true,
		ToolSurface:    config.ToolSurfaceDynamic,
		ToolList:       dynamicTools,
		ToolCatalog:    dynamicCatalog,
		WorkspaceRoots: true,
	})
	rootOnlyTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{WorkspaceRoots: true})
	if rootOnlyTokens <= 0 {
		t.Fatalf("workspace root tokens = %d, want positive token estimate", rootOnlyTokens)
	}
	if manifestTokens <= rootOnlyTokens {
		t.Fatalf("manifest resource tokens = %d, want greater than roots-only %d", manifestTokens, rootOnlyTokens)
	}
}

// TestMeasureResourcesWithOptions_MinimalCandidate verifies the dynamic-minimal
// candidate keeps the tool manifest and workspace roots while dropping the
// heavier optional resource groups.
func TestMeasureResourcesWithOptions_MinimalCandidate(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	dynamicCatalog := actioncatalog.FromActionMaps(routes)
	dynamicTools := listDynamicTools(dynamicCatalog)
	fullDynamicTokens := measureResources(client, routes, dynamicCatalog, dynamicTools, config.ToolSurfaceDynamic)
	minimalTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest:   true,
		ToolSurface:    config.ToolSurfaceDynamic,
		ToolList:       dynamicTools,
		ToolCatalog:    dynamicCatalog,
		WorkspaceRoots: true,
	})

	if minimalTokens <= 0 {
		t.Fatalf("minimal resource tokens = %d, want positive workspace_roots estimate", minimalTokens)
	}
	if minimalTokens >= fullDynamicTokens {
		t.Fatalf("minimal resource tokens = %d, want less than full dynamic %d", minimalTokens, fullDynamicTokens)
	}
}

// TestListDynamicTools_ExposesLowTokenSurface verifies the dynamic audit path
// measures the find/execute tools backed by the canonical action catalog.
func TestListDynamicTools_ExposesLowTokenSurface(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	if countActions(routes) == 0 {
		t.Fatal("buildMetaActionMaps() returned no actions")
	}

	toolList := listDynamicTools(actioncatalog.FromActionMaps(routes))
	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_execute_action,gitlab_find_action" {
		t.Fatalf("dynamic tools = %q, want find/execute", got)
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
// estimator captures name, domain, byte length, and bytes/4 token estimate.
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
		if info.Tokens != info.Bytes/bytesPerTok {
			t.Errorf("info %q: Tokens=%d, want Bytes/4 = %d", info.Name, info.Tokens, info.Bytes/bytesPerTok)
		}
		if info.Domain == "" {
			t.Errorf("info %q: Domain is empty", info.Name)
		}
		if info.Bytes <= 0 {
			t.Errorf("info %q: Bytes = %d, want positive", info.Name, info.Bytes)
		}
	}
	for _, want := range []string{"gitlab_project_get", "gitlab_issue_create"} {
		if !names[want] {
			t.Errorf("measureTools() missing %q in results: %+v", want, got)
		}
	}
}

// TestMeasureTools_EmptyInputReturnsEmpty verifies the estimator returns an
// empty slice for an empty tool list.
func TestMeasureTools_EmptyInputReturnsEmpty(t *testing.T) {
	got := measureTools(nil)
	if len(got) != 0 {
		t.Fatalf("measureTools(nil) = %d items, want 0", len(got))
	}
}

// TestMeasurePrompts_ReturnsTokenEstimateForRegisteredPrompts verifies the
// prompt token estimator produces a positive count for a real client.
func TestMeasurePrompts_ReturnsTokenEstimateForRegisteredPrompts(t *testing.T) {
	got := measurePrompts(newAuditTokensClient(t))
	if got <= 0 {
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

// captureStdoutAudit captures os.Stdout while fn runs and returns the result
// as a string.
func captureStdoutAudit(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	fn()
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error: %v", closeErr)
	}
	os.Stdout = oldStdout
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("io.ReadAll() error: %v", err)
	}
	if closeErr := r.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error: %v", closeErr)
	}
	return string(out)
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
