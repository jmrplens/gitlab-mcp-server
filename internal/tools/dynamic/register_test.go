// register_test.go contains unit tests for registering the dynamic tool surface with the MCP server.
package dynamic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// dynamicSearchCorpusCase describes one dynamic search corpus fixture row.
type dynamicSearchCorpusCase struct {
	Category             string                     `json:"category"`
	Query                string                     `json:"query"`
	WantTop              string                     `json:"want_top"`
	WantTopN             []string                   `json:"want_top_n"`
	Limit                int                        `json:"limit"`
	Enterprise           bool                       `json:"enterprise"`
	CustomAliases        []dynamicSearchCorpusAlias `json:"custom_aliases"`
	ExpectZero           bool                       `json:"expect_zero"`
	ExpectAmbiguous      bool                       `json:"expect_ambiguous"`
	ExpectDestructiveTop bool                       `json:"expect_destructive_top"`
	ForbidDestructiveTop bool                       `json:"forbid_destructive_top"`
	Notes                string                     `json:"notes"`
}

// dynamicSearchCorpusAlias maps an ad hoc query alias to its canonical action.
type dynamicSearchCorpusAlias struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
}

// TestSearch_RanksMatchingActions verifies that Search prioritizes the most
// specific destructive action when query terms match both the domain and action.
func TestSearch_RanksMatchingActions(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project delete", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches")
	}
	if output.Results[0].ID != "project.delete" {
		t.Fatalf("top result ID = %q, want project.delete", output.Results[0].ID)
	}
	if !output.Results[0].Destructive {
		t.Fatal("top result Destructive = false, want true")
	}
}

// TestSearch_ExplainIsOptIn verifies the Search_ExplainIsOptIn handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_ExplainIsOptIn(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	defaultResult, defaultOutput, err := registry.Search(t.Context(), nil, SearchInput{Query: "project delete", Limit: 3})
	if err != nil {
		t.Fatalf("Search(default) error = %v", err)
	}
	assertSearchNonError(t, "default", defaultResult, defaultOutput)
	if defaultOutput.Results[0].Explanation != nil {
		t.Fatalf("Search(default) explanation = %+v, want nil", defaultOutput.Results[0].Explanation)
	}
	if strings.Contains(textContent(defaultResult), "| Why |") {
		t.Fatalf("Search(default) markdown includes Why column: %s", textContent(defaultResult))
	}

	explainResult, explainOutput, err := registry.Search(t.Context(), nil, SearchInput{Query: "project delete", Limit: 3, Explain: true})
	if err != nil {
		t.Fatalf("Search(explain) error = %v", err)
	}
	assertSearchNonError(t, "explain", explainResult, explainOutput)
	assertSearchExplanation(t, explainOutput)
	if !strings.Contains(textContent(explainResult), "| Why |") || !strings.Contains(textContent(explainResult), "matched") {
		t.Fatalf("Search(explain) markdown missing Why explanation: %s", textContent(explainResult))
	}
}

func assertSearchNonError(t *testing.T, label string, result *mcp.CallToolResult, output SearchOutput) {
	t.Helper()
	if result == nil || result.IsError {
		t.Fatalf("Search(%s) result = %+v, want non-error", label, result)
	}
	if output.Count == 0 {
		t.Fatalf("Search(%s) returned no matches", label)
	}
}

func assertSearchExplanation(t *testing.T, explainOutput SearchOutput) {
	t.Helper()
	explanation := explainOutput.Results[0].Explanation
	if explanation == nil {
		t.Fatal("Search(explain) explanation is nil")
	}
	if explanation.TotalScore != explainOutput.Results[0].Score {
		t.Fatalf("explanation TotalScore = %d, want result score %d", explanation.TotalScore, explainOutput.Results[0].Score)
	}
	if explanation.MatchedTerms == 0 || explanation.RequiredTerms == 0 || len(explanation.Reasons) == 0 {
		t.Fatalf("Search(explain) explanation = %+v, want matched terms and reasons", explanation)
	}
	if explanation.Reasons[0].Field == "" || explanation.Reasons[0].QueryTerm == "" || explanation.Reasons[0].MatchedValue == "" {
		t.Fatalf("Search(explain) first reason = %+v, want field, query term, and matched value", explanation.Reasons[0])
	}
}

// TestCompactParameterGuidance_PrioritizesRequiredParamsAndShowsTruncation verifies that CompactParameterGuidance_PrioritizesRequiredParamsAndShowsTruncation forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestCompactParameterGuidance_PrioritizesRequiredParamsAndShowsTruncation(t *testing.T) {
	guidance := map[string]toolutil.ParameterGuidance{
		"zeta":       {ValueSource: "generated by the server"},
		"branch":     {SemanticRole: "target branch name"},
		"project_id": {CommonConfusions: []string{"Use the project ID or URL-encoded path."}},
	}

	got := compactParameterGuidance(guidance, 2, "branch")
	if !strings.Contains(got, "`project_id`: Use the project ID or URL-encoded path.") {
		t.Fatalf("compactParameterGuidance() = %q, want common confusion included", got)
	}
	branchIdx := strings.Index(got, "`branch`")
	projectIDIdx := strings.Index(got, "`project_id`")
	if branchIdx == -1 || projectIDIdx == -1 || branchIdx > projectIDIdx {
		t.Fatalf("compactParameterGuidance() = %q, want required params before confused params", got)
	}
	if strings.Contains(got, "`zeta`") {
		t.Fatalf("compactParameterGuidance() = %q, want zeta truncated", got)
	}
	if !strings.Contains(got, "...and 1 more params.") {
		t.Fatalf("compactParameterGuidance() = %q, want truncation indicator", got)
	}
}

// TestCompactParameterGuidanceItem_FormatsAvailableHints verifies the CompactParameterGuidanceItem_FormatsAvailableHints handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCompactParameterGuidanceItem_FormatsAvailableHints(t *testing.T) {
	tests := []struct {
		name string
		item toolutil.ParameterGuidance
		want string
	}{
		{name: "example binding", item: toolutil.ParameterGuidance{ExampleBinding: "from `project_id`"}, want: "`param` example from `project_id`."},
		{name: "value source", item: toolutil.ParameterGuidance{ValueSource: "provided by GitLab"}, want: "`param`: provided by GitLab."},
		{name: "semantic role", item: toolutil.ParameterGuidance{SemanticRole: "target branch"}, want: "`param`: target branch."},
		{name: "common confusion", item: toolutil.ParameterGuidance{CommonConfusions: []string{"Use the URL-encoded path."}}, want: "`param`: Use the URL-encoded path."},
		{name: "fallback", item: toolutil.ParameterGuidance{}, want: "`param` has action-specific guidance."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactParameterGuidanceItem("param", tt.item); got != tt.want {
				t.Fatalf("compactParameterGuidanceItem() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSearch_ReturnsNextStep verifies the Search_ReturnsNextStep handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_ReturnsNextStep(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project delete", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Results[0].ID != "project.delete" {
		t.Fatalf("Search() output = %+v, want project.delete", output)
	}
	if !strings.Contains(output.NextStep, "Use its exact parameter schema before executing") {
		t.Fatalf("NextStep = %q, want schema-aware execution guidance", output.NextStep)
	}
	markdown := textContent(result)
	if hints := toolutil.ExtractHints(markdown); !slices.Contains(hints, output.NextStep) {
		t.Fatalf("Search() hints = %q, want the computed next step %q", hints, output.NextStep)
	}
	if strings.Contains(markdown, "extra discovery") {
		t.Fatalf("Search() markdown still forces extra discovery: %s", markdown)
	}
}

// TestSearch_NoMatchSuggestsNearbyTokens verifies the Search_NoMatchSuggestsNearbyTokens handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_NoMatchSuggestsNearbyTokens(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "nonsenseonlyzz", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count != 0 {
		t.Fatalf("Search() Count = %d, want 0", output.Count)
	}
	if len(output.Suggestions) == 0 || len(output.Suggestions) > 6 {
		t.Fatalf("Suggestions = %v, want 1..6 values", output.Suggestions)
	}
	if !strings.Contains(textContent(result), "Try:") {
		t.Fatalf("Search() markdown = %q, want no-match suggestions", textContent(result))
	}
}

// TestSearch_RequiresQuery verifies the Search_RequiresQuery handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_RequiresQuery(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, _, err := registry.Search(t.Context(), nil, SearchInput{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Search() result = %+v, want tool error", result)
	}
}

// TestSearch_RanksAliasMatches verifies the Search_RanksAliasMatches handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_RanksAliasMatches(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "webhook create", Limit: 3})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 || output.Results[0].ID != "project.hook_add" {
		t.Fatalf("top result = %+v, want project.hook_add", output.Results)
	}
}

// TestSearch_UsesIntentSynonymsAndTags verifies the Search_UsesIntentSynonymsAndTags handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_UsesIntentSynonymsAndTags(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "merge request abbreviation", query: "mr approve", want: "merge_request.approve"},
		{name: "issue close intent", query: "close issue", want: "issue.update"},
		{name: "ci secret intent", query: "ci secret", want: "ci_variable.create"},
		{name: "project metadata intent", query: "project metadata", want: "project.get"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: 3})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			if output.Count == 0 || output.Results[0].ID != tt.want {
				t.Fatalf("top result = %+v, want %s", output.Results, tt.want)
			}
		})
	}
}

// TestSearch_ExactCanonicalIDBeatsBroadText verifies the Search_ExactCanonicalIDBeatsBroadText handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_ExactCanonicalIDBeatsBroadText(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project.list", Limit: 3})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 || output.Results[0].ID != "project.list" {
		t.Fatalf("top result = %+v, want project.list", output.Results)
	}
}

// TestSearch_CurrentHighConfidenceQueriesRemainStable verifies the Search_CurrentHighConfidenceQueriesRemainStable handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_CurrentHighConfidenceQueriesRemainStable(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := []struct {
		name         string
		query        string
		limit        int
		wantTop      string
		wantContains string
	}{
		{name: "merge request list", query: "merge request list open author project", wantTop: "merge_request.list"},
		{name: "open issues", query: "list open issues", limit: 10},
		{name: "pipeline trigger", query: "pipeline run trigger", wantContains: "pipeline.trigger_create"},
		{name: "ci variable secret", query: "ci variable secret", wantTop: "ci_variable.create"},
		{name: "project access token list", query: "project access tokens", wantTop: "access.token_project_list"},
		{name: "project access token create", query: "project access token create eval-token read_api expires_at 2026-12-31 for project my-org/tools/gitlab-mcp-server", wantTop: "access.token_project_create"},
		{name: "project deploy key list", query: "project deploy keys", wantTop: "access.deploy_key_list_project"},
		{name: "project deploy token create", query: "project deploy token create read_repository", wantTop: "access.deploy_token_create_project"},
		{name: "merge when pipeline succeeds", query: "merge when pipeline succeeds", wantTop: "merge_request.merge"},
		{name: "wait for pipeline", query: "wait for pipeline", wantTop: "pipeline.wait"},
		{name: "project delete", query: "project delete", wantTop: "project.delete"},
		{name: "project discovery", query: "discover project from remote", wantTop: "discover_project.resolve"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := tt.limit
			if limit == 0 {
				limit = 5
			}
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: limit})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			if output.Count == 0 {
				t.Fatalf("Search(%q) returned no matches", tt.query)
			}
			if tt.wantTop != "" && output.Results[0].ID != tt.wantTop {
				t.Fatalf("Search(%q) top result = %+v, want %s", tt.query, output.Results, tt.wantTop)
			}
			if tt.wantContains != "" && !slices.ContainsFunc(output.Results, func(result SearchResult) bool { return result.ID == tt.wantContains }) {
				t.Fatalf("Search(%q) results = %+v, want %s", tt.query, output.Results, tt.wantContains)
			}
		})
	}
}

// TestDynamicSearchCorpus validates the versioned dynamic search query corpus
// with table-driven cases loaded from testdata. Each entry describes a query,
// expected top actions, ambiguity/destructive expectations, and optional custom
// aliases; the test builds an in-memory catalog and registry per case.
// Run it directly with:
//
//	go test ./internal/tools/dynamic/ -run TestDynamicSearchCorpus -count=1
func TestDynamicSearchCorpus(t *testing.T) {
	cases := loadDynamicSearchCorpus(t)
	baseCatalog := mustCachedCatalog(t, false)
	baseRegistry := NewRegistryFromCatalog(baseCatalog)
	enterpriseCatalog := mustCachedCatalog(t, true)
	enterpriseRegistry := NewRegistryFromCatalog(enterpriseCatalog)

	for _, tc := range cases {
		t.Run(tc.Category, func(t *testing.T) {
			registry := registryForCorpusCase(baseRegistry, baseCatalog, enterpriseRegistry, enterpriseCatalog, tc)

			_, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tc.Query, Limit: tc.Limit})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			assertDynamicSearchCorpusCase(t, tc, output)
		})
	}
}

func registryForCorpusCase(baseRegistry *Registry, baseCatalog *actioncatalog.Catalog, enterpriseRegistry *Registry, enterpriseCatalog *actioncatalog.Catalog, tc dynamicSearchCorpusCase) *Registry {
	registry := baseRegistry
	catalog := baseCatalog
	if tc.Enterprise {
		registry = enterpriseRegistry
		catalog = enterpriseCatalog
	}
	if len(tc.CustomAliases) == 0 {
		return registry
	}
	aliases := append([]actionAlias(nil), actionAliases()...)
	for _, customAlias := range tc.CustomAliases {
		aliases = append(aliases, actionAlias{Alias: customAlias.Alias, Canonical: customAlias.Canonical, Source: aliasSourceCompatibility, Searchable: true})
	}
	return newRegistryFromCatalog(catalog, aliases)
}

func assertDynamicSearchCorpusCase(t *testing.T, tc dynamicSearchCorpusCase, output SearchOutput) {
	t.Helper()
	if tc.ExpectZero {
		if len(output.Results) != 0 {
			t.Fatalf("Search(%q) results = %+v, want zero results", tc.Query, output.Results)
		}
		return
	}
	assertDynamicSearchCorpusResults(t, tc, output)
}

func assertDynamicSearchCorpusResults(t *testing.T, tc dynamicSearchCorpusCase, output SearchOutput) {
	t.Helper()
	if len(output.Results) == 0 {
		t.Fatalf("Search(%q) returned no results; notes: %s", tc.Query, tc.Notes)
	}
	if tc.Limit > 0 && len(output.Results) > tc.Limit {
		t.Fatalf("Search(%q) returned %d results, want at most limit=%d", tc.Query, len(output.Results), tc.Limit)
	}
	if tc.WantTop != "" && output.Results[0].ID != tc.WantTop {
		t.Fatalf("Search(%q) top = %s, want %s; results = %+v", tc.Query, output.Results[0].ID, tc.WantTop, output.Results)
	}
	assertDynamicSearchCorpusExpectations(t, tc, output)
}

func assertDynamicSearchCorpusExpectations(t *testing.T, tc dynamicSearchCorpusCase, output SearchOutput) {
	t.Helper()
	for _, want := range tc.WantTopN {
		if !slices.ContainsFunc(output.Results, func(result SearchResult) bool { return result.ID == want }) {
			t.Fatalf("Search(%q) results = %+v, want top-N action %s", tc.Query, output.Results, want)
		}
	}
	if tc.ExpectAmbiguous && !slices.ContainsFunc(output.Results, func(result SearchResult) bool { return len(result.AmbiguousWith) > 0 }) {
		t.Fatalf("Search(%q) results = %+v, want ambiguity annotation", tc.Query, output.Results)
	}
	if tc.ExpectDestructiveTop && !output.Results[0].Destructive {
		t.Fatalf("Search(%q) top = %+v, want destructive top result", tc.Query, output.Results[0])
	}
	if tc.ForbidDestructiveTop && output.Results[0].Destructive {
		t.Fatalf("Search(%q) top = %+v, want non-destructive top result", tc.Query, output.Results[0])
	}
}

// loadDynamicSearchCorpus loads dynamic search corpus fixture data for tests.
func loadDynamicSearchCorpus(t *testing.T) []dynamicSearchCorpusCase {
	t.Helper()
	content, err := os.ReadFile("testdata/dynamic_search_queries.json")
	if err != nil {
		t.Fatalf("ReadFile(dynamic_search_queries.json) error = %v", err)
	}
	var cases []dynamicSearchCorpusCase
	if unmarshalErr := json.Unmarshal(content, &cases); unmarshalErr != nil {
		t.Fatalf("Unmarshal(dynamic_search_queries.json) error = %v", unmarshalErr)
	}
	if len(cases) == 0 {
		t.Fatal("dynamic search corpus is empty")
	}
	return cases
}

// TestSearch_AnnotatesAmbiguousAlias verifies that exact ambiguous aliases are
// surfaced in search results before the model reaches describe or execute.
func TestSearch_AnnotatesAmbiguousAlias(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "danger.delete", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches")
	}
	annotated := 0
	for _, searchResult := range output.Results {
		if slices.Contains(searchResult.AmbiguousWith, "project.delete") && slices.Contains(searchResult.AmbiguousWith, "package.delete") {
			annotated++
		}
	}
	if annotated == 0 {
		t.Fatalf("Search() results = %+v, want ambiguous alias annotations", output.Results)
	}
	text := textContent(result)
	if !strings.Contains(text, "Use one canonical action ID explicitly") || !strings.Contains(text, "`project.delete`") || !strings.Contains(text, "`package.delete`") {
		t.Fatalf("Search() markdown = %q, want ambiguous alias guidance", text)
	}
}

// TestSearch_ConfidenceAnnotations verifies close-score low confidence and a
// clear high-confidence top result. The thresholds are score >= 80 and margin >= 15.
func TestSearch_ConfidenceAnnotations(t *testing.T) {
	registry := realCatalogRegistry(t)

	lowResult, lowOutput, err := registry.Search(t.Context(), nil, SearchInput{Query: "project", Limit: 5, Explain: true})
	if err != nil {
		t.Fatalf("Search(low) error = %v", err)
	}
	if lowResult == nil || lowResult.IsError || lowOutput.Count == 0 {
		t.Fatalf("Search(low) result/output = %+v %+v, want matches", lowResult, lowOutput)
	}
	if !lowOutput.Results[0].LowConfidence {
		t.Fatalf("Search(project) top result = %+v, want low confidence", lowOutput.Results[0])
	}
	if lowOutput.Results[0].Explanation == nil || !lowOutput.Results[0].Explanation.LowConfidence {
		t.Fatalf("Search(project) explanation = %+v, want low confidence", lowOutput.Results[0].Explanation)
	}

	highResult, highOutput, err := registry.Search(t.Context(), nil, SearchInput{Query: "project delete", Limit: 5, Explain: true})
	if err != nil {
		t.Fatalf("Search(high) error = %v", err)
	}
	if highResult == nil || highResult.IsError || highOutput.Count == 0 {
		t.Fatalf("Search(high) result/output = %+v %+v, want matches", highResult, highOutput)
	}
	if highOutput.Results[0].ID != "project.delete" || highOutput.Results[0].LowConfidence {
		t.Fatalf("Search(project delete) top result = %+v, want high-confidence project.delete", highOutput.Results[0])
	}
}

// TestAddStandaloneRoutes_AddsDynamicActions verifies the AddStandaloneRoutes_AddsDynamicActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddStandaloneRoutes_AddsDynamicActions(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []string{
		"discover_project.resolve",
		"interactive.issue_create",
		"interactive.mr_create",
		"interactive.project_create",
		"interactive.release_create",
	}
	for _, actionID := range tests {
		t.Run(actionID, func(t *testing.T) {
			if _, ok := registry.resolveAction(actionID); !ok {
				t.Fatalf("resolveAction(%q) = false, want true", actionID)
			}
		})
	}
}

// TestAddStandaloneRoutes_HonorsReadOnlyAndExclusions verifies that standalone
// route registration respects read-only mode and explicit tool exclusions.
func TestAddStandaloneRoutes_HonorsReadOnlyAndExclusions(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{
		ReadOnly:     true,
		ExcludeTools: []string{"gitlab_discover_project"},
	})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	if _, ok := registry.resolveAction("discover_project.resolve"); ok {
		t.Fatal("discover_project.resolve is present, want excluded")
	}
	if _, ok := registry.resolveAction("interactive.issue_create"); ok {
		t.Fatal("interactive.issue_create is present in read-only mode")
	}
}

// TestAddStandaloneCatalog_MatchesRouteCompatibilityWrapper verifies that the
// catalog-native standalone builder preserves the old route-map wrapper output.
func TestAddStandaloneCatalog_MatchesRouteCompatibilityWrapper(t *testing.T) {
	routes := testRoutes(t)
	standaloneRoutes, err := AddStandaloneRoutes(routes, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	standaloneCatalog, err := AddStandaloneCatalog(actioncatalog.FromActionMaps(routes), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	fromRoutes := NewRegistry(standaloneRoutes)
	fromCatalog := NewRegistryFromCatalog(standaloneCatalog)

	for _, actionID := range []string{"project.list", "discover_project.resolve", "interactive.issue_create"} {
		t.Run(actionID, func(t *testing.T) {
			if _, ok := fromRoutes.resolveAction(actionID); !ok {
				t.Fatalf("route wrapper registry missing %s", actionID)
			}
			if _, ok := fromCatalog.resolveAction(actionID); !ok {
				t.Fatalf("catalog registry missing %s", actionID)
			}
		})
	}
}

// TestAddStandaloneCatalog_NilCatalogWithExcludedInteractiveActions verifies the AddStandaloneCatalog_NilCatalogWithExcludedInteractiveActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddStandaloneCatalog_NilCatalogWithExcludedInteractiveActions(t *testing.T) {
	catalog, err := AddStandaloneCatalog(nil, nil, StandaloneOptions{ExcludeTools: []string{
		"gitlab_interactive_issue_create",
		"gitlab_interactive_mr_create",
		"gitlab_interactive_project_create",
		"gitlab_interactive_release_create",
	}})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	registry := NewRegistryFromCatalog(catalog)

	if _, ok := registry.resolveAction("discover_project.resolve"); !ok {
		t.Fatal("discover_project.resolve missing")
	}
	if _, ok := registry.resolveAction("interactive.issue_create"); ok {
		t.Fatal("interactive.issue_create present, want excluded")
	}
}

// TestNewRegistryFromCatalog_UsesCatalogAliasesAndTags verifies that dynamic
// mode can consume registry-native action metadata without rebuilding it from
// legacy route maps.
func TestNewRegistryFromCatalog_UsesCatalogAliasesAndTags(t *testing.T) {
	registry := NewRegistryFromCatalog(customCatalogForDynamicTest(t))
	assertCustomCatalogSearch(t, registry)
	assertCustomCatalogDescribe(t, registry)
	assertCustomCatalogFind(t, registry)
}

func customCatalogForDynamicTest(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_custom"})
	group.SetAction(actioncatalog.Action{Name: "inspect", Aliases: []string{"custom.lookup"}, Tags: []string{"bespoke"}, Route: customCatalogRouteForDynamicTest()})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	return catalog
}

func customCatalogRouteForDynamicTest() toolutil.ActionRoute {
	return toolutil.ActionRoute{
		Handler: func(_ context.Context, params map[string]any) (any, error) {
			return map[string]any{"target": params["target"]}, nil
		},
		InputSchema: map[string]any{"type": "object", "required": []any{"target"}, "properties": map[string]any{"target": map[string]any{"type": "string"}}},
	}.
		WithUsage("Use for custom catalog metadata.").
		WithRelatedActions("custom.audit").
		WithParameterGuidance(map[string]toolutil.ParameterGuidance{"target": {SemanticRole: "custom_target", CommonConfusions: []string{"Do not use source."}}})
}

func assertCustomCatalogSearch(t *testing.T, registry *Registry) {
	t.Helper()
	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "bespoke", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Results[0].ID != "custom.inspect" {
		t.Fatalf("Search() output = %+v, want custom.inspect", output)
	}
}

func assertCustomCatalogDescribe(t *testing.T, registry *Registry) {
	t.Helper()
	result, described, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "custom.lookup"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if described.Count != 1 || described.Actions[0].ID != "custom.inspect" {
		t.Fatalf("Describe() output = %+v, want custom.inspect", described)
	}
	description := described.Actions[0]
	if description.Usage != "Use for custom catalog metadata." || len(description.RelatedActions) != 1 || description.RelatedActions[0] != "custom.audit" {
		t.Fatalf("Describe() metadata = %+v, want route-derived usage and related actions", description)
	}
	description.ParamGuidance["target"] = toolutil.ParameterGuidance{SemanticRole: "changed"}
	_, describedAgain, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "custom.lookup"})
	if err != nil {
		t.Fatalf("Describe() second call error = %v", err)
	}
	if got := describedAgain.Actions[0].ParamGuidance["target"].SemanticRole; got != "custom_target" {
		t.Fatalf("second describe ParamGuidance target role = %q, want cloned custom_target", got)
	}
}

func assertCustomCatalogFind(t *testing.T, registry *Registry) {
	t.Helper()
	findResult, found, err := registry.Find(t.Context(), nil, FindInput{Query: "bespoke", Limit: 1})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if findResult == nil || findResult.IsError {
		t.Fatalf("Find() result = %+v, want non-error", findResult)
	}
	if found.Count != 1 || found.Results[0].ID != "custom.inspect" {
		t.Fatalf("Find() output = %+v, want custom.inspect", found)
	}
	findText := textContent(findResult)
	if !strings.Contains(findText, "Guidance") || !strings.Contains(findText, "Use for custom catalog metadata") || !strings.Contains(findText, "`target`") {
		t.Fatalf("Find() text = %q, want compact usage and parameter guidance", findText)
	}
}

// TestNewRegistryFromCatalog_NilCatalog verifies the NewRegistryFromCatalog_NilCatalog handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestNewRegistryFromCatalog_NilCatalog(t *testing.T) {
	registry := NewRegistryFromCatalog(nil)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project", Limit: 3})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error empty result", result)
	}
	if output.Count != 0 {
		t.Fatalf("Search() Count = %d, want 0", output.Count)
	}
}

// TestDescribe_CanonicalizesStandaloneAlias verifies the Describe_CanonicalizesStandaloneAlias handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_CanonicalizesStandaloneAlias(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "gitlab_interactive_issue_create"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Actions[0].ID != "interactive.issue_create" {
		t.Fatalf("actions = %+v, want canonical interactive.issue_create", output.Actions)
	}
}

// TestDescribe_ReturnsSchemaAndExample verifies the Describe_ReturnsSchemaAndExample handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_ReturnsSchemaAndExample(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project.delete"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 1 {
		t.Fatalf("Describe() Count = %d, want 1", output.Count)
	}
	action := output.Actions[0]
	if action.ID != "project.delete" || !action.Destructive {
		t.Fatalf("action = %+v, want project.delete destructive", action)
	}
	if _, ok := action.InputSchema["x_destructive"]; !ok {
		t.Fatalf("InputSchema missing x_destructive: %+v", action.InputSchema)
	}
	if confirmation, ok := action.InputSchema["x_confirmation"].(map[string]any); !ok || confirmation["location"] != "gitlab_execute_action.confirm" {
		t.Fatalf("InputSchema x_confirmation = %+v, want dynamic top-level confirm guidance", action.InputSchema["x_confirmation"])
	}
	if _, hasConfirmParam := schemaProperties(action.InputSchema)["confirm"]; hasConfirmParam {
		t.Fatalf("InputSchema includes params.confirm for dynamic action: %+v", action.InputSchema)
	}
	if required, _ := action.InputSchema["required"].([]any); slices.Contains(required, any("confirm")) {
		t.Fatalf("InputSchema requires params.confirm for dynamic action: %+v", action.InputSchema)
	}
	if action.Example.Arguments["confirm"] != true {
		t.Fatalf("example missing confirm param: %+v", action.Example)
	}
	if action.SchemaURI != "gitlab://tools/project.delete" {
		t.Fatalf("SchemaURI = %q, want tool detail URI", action.SchemaURI)
	}
}

// TestDynamicInputSchema_RemovesConfirmFromRequired verifies dynamic action
// schemas keep destructive confirmation at gitlab_execute_action.confirm only.
func TestDynamicInputSchema_RemovesConfirmFromRequired(t *testing.T) {
	schema := dynamicInputSchema(actionEntry{
		ID:          "project.delete",
		Tool:        "gitlab_project",
		Action:      "delete",
		Destructive: true,
		Route: toolutil.ActionRoute{InputSchema: map[string]any{
			"type":     "object",
			"required": []any{"project_id", "confirm"},
			"properties": map[string]any{
				"project_id": map[string]any{"type": "integer"},
				"confirm":    map[string]any{"type": "boolean"},
			},
		}},
	})
	if _, hasConfirm := schemaProperties(schema)["confirm"]; hasConfirm {
		t.Fatalf("schema properties include confirm: %+v", schema)
	}
	if required, _ := schema["required"].([]any); slices.Contains(required, any("confirm")) {
		t.Fatalf("schema required includes confirm: %+v", schema)
	}
}

// TestDynamicInputSchema_DefaultsWhenRouteSchemaMissing verifies that
// dynamicInputSchema falls back to a permissive object schema (type=object,
// additionalProperties=true, plus explanatory description) when the action route
// carries no captured parameter schema, so execution is never blocked by a missing schema.
func TestDynamicInputSchema_DefaultsWhenRouteSchemaMissing(t *testing.T) {
	schema := dynamicInputSchema(actionEntry{
		ID:     "widget.ping",
		Tool:   "gitlab_widget",
		Action: "ping",
		Route:  toolutil.ActionRoute{},
	})
	if schema["type"] != "object" || schema["additionalProperties"] != true {
		t.Fatalf("schema = %+v, want permissive object fallback", schema)
	}
	// The wording has to be the dynamic one, not the meta-tool sentence
	// toolutil.MetaActionSchema already writes for a schemaless route: on this
	// surface the arguments travel inside gitlab_execute_action's params
	// object, and "send an empty object {}" tells a caller to send the wrong
	// thing. Both sentences share "no captured parameter schema", so only the
	// half that differs says which one was served.
	if description, _ := schema["description"].(string); !strings.Contains(description, "no captured parameter schema") ||
		!strings.Contains(description, "empty params object") {
		t.Fatalf("schema description = %q, want the dynamic fallback guidance", description)
	}

	withSchema := dynamicInputSchema(actionEntry{
		ID:     "widget.poke",
		Tool:   "gitlab_widget",
		Action: "poke",
		Route: toolutil.ActionRoute{InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"widget_id": map[string]any{"type": "integer"}},
		}},
	})
	if description, _ := withSchema["description"].(string); description != "" {
		t.Fatalf("schema description = %q, want none for a route that captured its parameters", description)
	}
}

// TestRemoveDynamicRequiredConfirmParam_HandlesStringRequiredLists verifies that
// removeDynamicRequiredConfirmParam strips the "confirm" entry from a schema's
// required list for both []any and []string representations, deleting the key
// entirely when "confirm" was the only required field and otherwise preserving the rest.
func TestRemoveDynamicRequiredConfirmParam_HandlesStringRequiredLists(t *testing.T) {
	anySchema := map[string]any{"required": []any{"confirm"}}
	removeDynamicRequiredConfirmParam(anySchema)
	if _, ok := anySchema["required"]; ok {
		t.Fatalf("required should be deleted for []any when empty: %+v", anySchema)
	}

	schema := map[string]any{"required": []string{"project_id", "confirm"}}
	removeDynamicRequiredConfirmParam(schema)
	if required, _ := schema["required"].([]string); len(required) != 1 || required[0] != "project_id" {
		t.Fatalf("required = %+v, want project_id", schema["required"])
	}

	schema = map[string]any{"required": []string{"confirm"}}
	removeDynamicRequiredConfirmParam(schema)
	if _, ok := schema["required"]; ok {
		t.Fatalf("required should be deleted when empty: %+v", schema)
	}

	// Only "confirm" leaves the list. The []any case reads each element with a
	// type assertion, and the two halves of that test have to hold together:
	// dropping an element because it is a string, or because it is not one,
	// deletes required parameters the action cannot run without.
	mixed := map[string]any{"required": []any{"project_id", 42, "confirm"}}
	removeDynamicRequiredConfirmParam(mixed)
	if required, _ := mixed["required"].([]any); !slices.Equal(required, []any{any("project_id"), any(42)}) {
		t.Fatalf("required = %+v, want project_id and the non-string entry kept", mixed["required"])
	}
}

// TestDescribe_IncludesOutputSchema verifies the Describe_IncludesOutputSchema handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_IncludesOutputSchema(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project.get"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	description := output.Actions[0]
	properties := schemaProperties(description.OutputSchema)
	if _, ok := properties["project_id"]; !ok {
		t.Fatalf("OutputSchema properties = %v, want project_id", properties)
	}
}

// TestDescribe_MetaCatalogSchemas verifies the Describe_MetaCatalogSchemas handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_MetaCatalogSchemas(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Actions: []string{
		"project.list",
		"merge_request.list",
		"user.current_user_status",
		"user.list",
	}})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 4 {
		t.Fatalf("Describe() Count = %d, want 4", output.Count)
	}
	structured, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal(DescribeOutput) error = %v", err)
	}
	if !strings.Contains(string(structured), "output_schema") {
		t.Fatalf("DescribeOutput JSON missing output_schema: %s", structured)
	}
	markdown := textContent(result)
	for _, notWant := range []string{"input_schema", "output_schema"} {
		t.Run(notWant, func(t *testing.T) {
			if strings.Contains(markdown, notWant) {
				t.Fatalf("Describe() markdown contains %q: %s", notWant, markdown)
			}
		})
	}
	if !strings.Contains(markdown, "#### Input schema") || !strings.Contains(markdown, "```json") || !strings.Contains(markdown, "properties") {
		t.Fatalf("Describe() markdown missing compact input schema: %s", markdown)
	}

	projectList := actionDescriptionByID(t, output, "project.list")
	assertSchemaHasProperties(t, projectList.InputSchema, "search", "owned", "per_page")
	if projectList.OutputSchema == nil {
		t.Fatal("project.list OutputSchema is nil")
	}
	if len(projectList.RequiredParams) != 0 {
		t.Fatalf("project.list RequiredParams = %v, want none", projectList.RequiredParams)
	}

	mergeRequestList := actionDescriptionByID(t, output, "merge_request.list")
	assertSchemaHasProperties(t, mergeRequestList.InputSchema, "project_id", "state", "author_username", "scope")
	if mergeRequestList.OutputSchema == nil {
		t.Fatal("merge_request.list OutputSchema is nil")
	}
	if !slices.Contains(mergeRequestList.RequiredParams, "project_id") {
		t.Fatalf("merge_request.list RequiredParams = %v, want project_id", mergeRequestList.RequiredParams)
	}
	if got := mergeRequestList.Example.Arguments["params"].(map[string]any)["project_id"]; got != "group/project" {
		t.Fatalf("merge_request.list example project_id = %v, want group/project", got)
	}

	currentUserStatus := actionDescriptionByID(t, output, "user.current_user_status")
	if len(schemaProperties(currentUserStatus.InputSchema)) != 0 {
		t.Fatalf("user.current_user_status input properties = %v, want none", schemaProperties(currentUserStatus.InputSchema))
	}
	if currentUserStatus.OutputSchema == nil {
		t.Fatal("user.current_user_status OutputSchema is nil")
	}

	userList := actionDescriptionByID(t, output, "user.list")
	assertSchemaHasProperties(t, userList.InputSchema, "search", "username", "per_page")
	if userList.OutputSchema == nil {
		t.Fatal("user.list OutputSchema is nil")
	}
}

// TestDynamicCatalog_DelegatedSpecBackedDomainsPreserveIDsAndSchemas verifies
// that delegated meta-tool domains migrated to ActionSpec remain discoverable
// through Dynamic search and describe with the same catalog-backed schemas.
func TestDynamicCatalog_DelegatedSpecBackedDomainsPreserveIDsAndSchemas(t *testing.T) {
	catalog, registry := gitLabDotComEnterpriseRegistry(t)
	actionIDs := []string{
		"search.code",
		"runner.enable_project",
		"orbit.dsl",
	}

	for _, actionID := range actionIDs {
		t.Run("search/"+actionID, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: actionID, Limit: 20})
			if err != nil {
				t.Fatalf("Search(%q) error = %v", actionID, err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search(%q) result = %+v, want non-error", actionID, result)
			}
			assertSearchResultsContain(t, output.Results, actionID)
		})
	}

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Actions: actionIDs})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != len(actionIDs) {
		t.Fatalf("Describe() Count = %d, want %d", output.Count, len(actionIDs))
	}

	for _, actionID := range actionIDs {
		t.Run(actionID, func(t *testing.T) {
			description := actionDescriptionByID(t, output, actionID)
			catalogAction, ok := catalog.Action(actioncatalog.ActionID(actionID))
			if !ok {
				t.Fatalf("catalog missing %s", actionID)
			}
			if !catalogAction.SpecBacked {
				t.Fatalf("%s SpecBacked = false, want true", actionID)
			}
			assertSchemaPropertyNamesEqual(t, actionID, description.InputSchema, catalogAction.Route.InputSchema)
			if !slices.Equal(description.RequiredParams, requiredParams(catalogAction.Route.InputSchema)) {
				t.Fatalf("%s RequiredParams = %v, want %v", actionID, description.RequiredParams, requiredParams(catalogAction.Route.InputSchema))
			}
			assertSchemaPropertyNamesEqual(t, actionID+" output", description.OutputSchema, catalogAction.Route.OutputSchema)
		})
	}

	searchCode := actionDescriptionByID(t, output, "search.code")
	assertSchemaHasProperties(t, searchCode.InputSchema, "query", "search_type")

	runnerEnableProject := actionDescriptionByID(t, output, "runner.enable_project")
	if got := runnerEnableProject.ParamGuidance["runner_id"].SemanticRole; got != "runner_identifier" {
		t.Fatalf("runner.enable_project runner_id semantic role = %q, want runner_identifier", got)
	}
	if got := runnerEnableProject.ParamGuidance["project_id"].SemanticRole; got != "scope_owner_project" {
		t.Fatalf("runner.enable_project project_id semantic role = %q, want scope_owner_project", got)
	}
}

// TestDescribe_IncludesParameterGuidance verifies catalog-level guidance for
// role-sensitive params reaches Dynamic describe and its schema extension.
func TestDescribe_IncludesParameterGuidance(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "job.token_scope_remove_project"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	description := actionDescriptionByID(t, output, "job.token_scope_remove_project")
	projectGuidance, ok := description.ParamGuidance["project_id"]
	if !ok {
		t.Fatalf("ParamGuidance = %#v, want project_id", description.ParamGuidance)
	}
	if projectGuidance.SemanticRole != "scope_owner_project" {
		t.Fatalf("project_id semantic role = %q, want scope_owner_project", projectGuidance.SemanticRole)
	}
	extension, ok := description.InputSchema["x_parameter_guidance"].(map[string]any)
	if !ok {
		t.Fatalf("InputSchema missing x_parameter_guidance: %#v", description.InputSchema)
	}
	if _, hasTargetProjectID := extension["target_project_id"]; !hasTargetProjectID {
		t.Fatalf("x_parameter_guidance = %#v, want target_project_id", extension)
	}
}

// TestSearch_WhyThisActionOnlyAppearsForCloseAlternatives verifies the Search_WhyThisActionOnlyAppearsForCloseAlternatives handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_WhyThisActionOnlyAppearsForCloseAlternatives(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	_, straightforward, err := registry.Search(t.Context(), nil, SearchInput{Query: "project delete", Limit: 1})
	if err != nil {
		t.Fatalf("Search(straightforward) error = %v", err)
	}
	if straightforward.Count == 0 || len(straightforward.Results) == 0 {
		t.Fatalf("Search(straightforward) returned no matches: %+v", straightforward)
	}
	if straightforward.Results[0].WhyThisAction != "" {
		t.Fatalf("straightforward WhyThisAction = %q, want empty", straightforward.Results[0].WhyThisAction)
	}

	_, closeAlternatives, err := registry.Search(t.Context(), nil, SearchInput{Query: "project", Limit: 5})
	if err != nil {
		t.Fatalf("Search(closeAlternatives) error = %v", err)
	}
	if !slices.ContainsFunc(closeAlternatives.Results, func(result SearchResult) bool { return result.WhyThisAction != "" }) {
		t.Fatalf("close alternatives = %+v, want at least one why_this_action", closeAlternatives.Results)
	}
}

// TestFind_ReturnsSchemaAndExecuteExample verifies that Find combines search
// ranking with the input schema and execute example needed to call an action.
func TestFind_ReturnsSchemaAndExecuteExample(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Find(t.Context(), nil, FindInput{Query: "project delete", Limit: 3})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Find() result = %+v, want non-error", result)
	}
	if output.Count == 0 || output.Results[0].ID != "project.delete" {
		t.Fatalf("top result = %+v, want project.delete", output.Results)
	}
	found := output.Results[0]
	if !found.Destructive || found.InputSchema == nil {
		t.Fatalf("found result = %+v, want destructive action with schema", found)
	}
	if found.OutputSchema != nil {
		t.Fatalf("found OutputSchema = %v, want nil for route without output schema", found.OutputSchema)
	}
	if found.Example.Tool != "gitlab_execute_action" || found.Example.Arguments["confirm"] != true {
		t.Fatalf("example = %+v, want execute example with confirm", found.Example)
	}
}

// TestFind_MarkdownGuidesImmediateExecuteAndConfirm verifies the visible finder
// output discourages batching future searches and keeps destructive confirmation
// in the table text for models that ignore structured examples.
func TestFind_MarkdownGuidesImmediateExecuteAndConfirm(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Find(t.Context(), nil, FindInput{Query: "project delete", Limit: 1})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Find() result = %+v, want non-error", result)
	}
	if output.Count == 0 || output.Results[0].ID != "project.delete" {
		t.Fatalf("top result = %+v, want project.delete", output.Results)
	}
	markdown := textContent(result)
	hints := toolutil.ExtractHints(markdown)
	for _, want := range []string{wantExecuteNowHint, dynamicExecuteEnvelopeHint, wantStructuredResultsHint} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(hints, want) {
				t.Fatalf("Find() hints = %q, want %q", hints, want)
			}
		})
	}
	// The confirmation a destructive action needs stays in the table text for
	// a model that reads the rows and not the guidance.
	if !strings.Contains(markdown, wantDestructiveConfirmCell) {
		t.Fatalf("Find() markdown = %q, want the confirm guidance in the row", markdown)
	}
}

// TestFind_ExplainIsOptIn verifies the Find_ExplainIsOptIn handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestFind_ExplainIsOptIn(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	defaultResult, defaultOutput, err := registry.Find(t.Context(), nil, FindInput{Query: "project delete", Limit: 3})
	if err != nil {
		t.Fatalf("Find(default) error = %v", err)
	}
	if defaultResult == nil || defaultResult.IsError {
		t.Fatalf("Find(default) result = %+v, want non-error", defaultResult)
	}
	if defaultOutput.Count == 0 {
		t.Fatal("Find(default) returned no matches")
	}
	if defaultOutput.Results[0].Explanation != nil {
		t.Fatalf("Find(default) explanation = %+v, want nil", defaultOutput.Results[0].Explanation)
	}
	if strings.Contains(textContent(defaultResult), "| Why |") {
		t.Fatalf("Find(default) markdown includes Why column: %s", textContent(defaultResult))
	}

	explainResult, explainOutput, err := registry.Find(t.Context(), nil, FindInput{Query: "project delete", Limit: 3, Explain: true})
	if err != nil {
		t.Fatalf("Find(explain) error = %v", err)
	}
	if explainResult == nil || explainResult.IsError {
		t.Fatalf("Find(explain) result = %+v, want non-error", explainResult)
	}
	if explainOutput.Count == 0 {
		t.Fatal("Find(explain) returned no matches")
	}
	explanation := explainOutput.Results[0].Explanation
	if explanation == nil {
		t.Fatal("Find(explain) explanation is nil")
	}
	if explanation.TotalScore != explainOutput.Results[0].Score || len(explanation.Reasons) == 0 {
		t.Fatalf("Find(explain) explanation = %+v, want score and reasons", explanation)
	}
	if !strings.Contains(textContent(explainResult), "| Why |") || !strings.Contains(textContent(explainResult), "matched") {
		t.Fatalf("Find(explain) markdown missing Why explanation: %s", textContent(explainResult))
	}
}

// TestFind_RequiresQuery verifies the Find_RequiresQuery handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestFind_RequiresQuery(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Find(t.Context(), nil, FindInput{})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Find() result = %+v, want tool error", result)
	}
	if output.Count != 0 || len(output.Results) != 0 {
		t.Fatalf("Find() output = %+v, want empty output", output)
	}
}

// TestRegisterCatalogFindExecuteTools_ExposesTwoDynamicTools verifies that the dynamic
// two-tool surface exposes only find and execute through an MCP session.
func TestRegisterCatalogFindExecuteTools_ExposesTwoDynamicTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogFindExecuteTools(server, actioncatalog.FromActionMaps(testRoutes(t)))

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(tools.Tools))
	}
	names := []string{tools.Tools[0].Name, tools.Tools[1].Name}
	if !slices.Contains(names, "gitlab_find_action") || !slices.Contains(names, "gitlab_execute_action") {
		t.Fatalf("tools = %v, want find/execute", names)
	}
}

// TestDescribe_UnknownActionReturnsToolError verifies that Describe_UnknownActionReturnsToolError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDescribe_UnknownActionReturnsToolError(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, _, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project.missing"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Describe() result = %+v, want tool error", result)
	}
}

// TestDescribe_CanonicalizesAlias verifies the Describe_CanonicalizesAlias handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_CanonicalizesAlias(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project_access_token.create"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Actions[0].ID != "access.token_project_create" {
		t.Fatalf("Describe() output = %+v, want access.token_project_create", output)
	}
}

// TestUnsearchableAlias_CanonicalizesWithoutRanking verifies compatibility
// aliases can remain valid for describe/execute without influencing search.
func TestUnsearchableAlias_CanonicalizesWithoutRanking(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "hidden.lookup", Canonical: "project.get", Source: aliasSourceCompatibility, Searchable: false, Notes: "test-only hidden alias"},
	})

	describeResult, describeOutput, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "hidden.lookup"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if describeResult == nil || describeResult.IsError || describeOutput.Count != 1 || describeOutput.Actions[0].ID != "project.get" {
		t.Fatalf("Describe() result/output = %+v %+v, want project.get", describeResult, describeOutput)
	}

	executeResult, executeOutput, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "hidden.lookup", Params: map[string]any{"project_id": 123}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if executeResult == nil || executeResult.IsError || executeOutput == nil {
		t.Fatalf("Execute() result/output = %+v %+v, want non-error output", executeResult, executeOutput)
	}

	searchResult, searchOutput, err := registry.Search(t.Context(), nil, SearchInput{Query: "hidden lookup", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if searchResult == nil || searchResult.IsError {
		t.Fatalf("Search() result = %+v, want non-error", searchResult)
	}
	if slices.ContainsFunc(searchOutput.Results, func(result SearchResult) bool { return result.ID == "project.get" }) {
		t.Fatalf("Search() results = %+v, want hidden alias not to rank project.get", searchOutput.Results)
	}
}

// TestRequiredParams_IncludesPreferredAlternative verifies that schemas using
// anyOf still produce a useful example branch for search and describe output.
func TestRequiredParams_IncludesPreferredAlternative(t *testing.T) {
	schema := map[string]any{
		"required": []any{"project_id", "title"},
		"anyOf": []any{
			map[string]any{"required": []any{"file_name", "content"}},
			map[string]any{"required": []any{"files"}},
		},
	}

	got := strings.Join(requiredParams(schema), ",")
	if got != "content,file_name,files,project_id,title" {
		t.Fatalf("requiredParams() = %q", got)
	}
}

// TestBuildSearchDocument_CapturesTypedFields verifies that the dynamic ranker
// builds typed metadata fields while preserving the flat text compatibility
// fallback used by the current scorer.
func TestBuildSearchDocument_CapturesTypedFields(t *testing.T) {
	schema := map[string]any{
		"required": []any{"project_id"},
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project path or numeric identifier"},
			"author_username": map[string]any{"type": "string"},
			"state":           map[string]any{"type": "string", "enum": []any{"opened", "closed"}},
		},
	}

	document := buildSearchDocument(
		"repository.tree",
		"gitlab_repository",
		"repository",
		"tree",
		[]string{"repository_tree", "repo.files"},
		[]string{"read", "tree"},
		schema,
	)

	if document.CanonicalID != "repository.tree" {
		t.Fatalf("CanonicalID = %q, want repository.tree", document.CanonicalID)
	}
	assertSearchDocumentIdentity(t, document)
	assertSearchDocumentText(t, document)
	assertSearchDocumentSchemaFields(t, document)
}

func assertSearchDocumentIdentity(t *testing.T, document searchDocument) {
	t.Helper()
	for _, want := range []string{"repository", "tree"} {
		if !slices.Contains(document.IDWords, want) {
			t.Fatalf("IDWords = %v, want %q", document.IDWords, want)
		}
	}
	if document.Tool != "gitlab_repository" || document.Domain != "repository" || document.Action != "tree" {
		t.Fatalf("document identity fields = %+v", document)
	}
	if document.Backend != "gitlab" || document.Capability != "source_control" || document.Resource != "repository" || document.Operation != "tree" || document.Scope != "project" {
		t.Fatalf("document cross-backend fields = %+v", document)
	}
}

func assertSearchDocumentText(t *testing.T, document searchDocument) {
	t.Helper()
	if !slices.Contains(document.Aliases, "repository_tree") || !slices.Contains(document.Aliases, "repo.files") {
		t.Fatalf("Aliases = %v, want hidden and visible aliases", document.Aliases)
	}
	if !strings.Contains(document.FlatText, "repository_tree") {
		t.Fatalf("FlatText = %q, want explicitly supplied aliases to be searchable", document.FlatText)
	}
	for _, want := range []string{"gitlab", "source_control", "project", "repo.files", "read", "project_id", "author_username", "author username", "opened", "closed", "project path or numeric identifier"} {
		if !strings.Contains(document.FlatText, want) {
			t.Fatalf("FlatText = %q, want %q", document.FlatText, want)
		}
	}
	if !slices.Contains(document.Tags, "read") || !slices.Contains(document.RequiredParams, "project_id") {
		t.Fatalf("document tags/required params = %+v", document)
	}
}

func assertSearchDocumentSchemaFields(t *testing.T, document searchDocument) {
	t.Helper()
	if strings.Join(document.OptionalParams, ",") != "author_username,state" {
		t.Fatalf("OptionalParams = %v, want sorted author_username,state", document.OptionalParams)
	}
	if strings.Join(document.SchemaProperties, ",") != "author_username,project_id,state" {
		t.Fatalf("SchemaProperties = %v, want sorted author_username,project_id,state", document.SchemaProperties)
	}
	if strings.Join(document.SchemaEnums, ",") != "closed,opened" {
		t.Fatalf("SchemaEnums = %v, want closed,opened", document.SchemaEnums)
	}
	if strings.Join(document.SchemaDescTerms, ",") != "project path or numeric identifier" {
		t.Fatalf("SchemaDescTerms = %v, want project path or numeric identifier", document.SchemaDescTerms)
	}
}

// TestInferCapability_GroupsEveryDomainUnderOneCapability verifies the
// capability a search document carries for each family of domains, including
// the ones reached by prefix or by the action name rather than by an exact
// domain match.
//
// Capability is a search field: a domain that falls into the wrong one, or into
// the catch-all when a family exists for it, is ranked against the wrong
// queries. The prefixed and action-name cases are the ones a new domain lands
// in, which is why each is named here rather than left to the exact matches.
func TestInferCapability_GroupsEveryDomainUnderOneCapability(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		action string
		want   string
	}{
		{name: "merge request domain", domain: "merge_request", action: "list", want: "code_review"},
		{name: "merge request review domain", domain: "mr_review", action: "changes_get", want: "code_review"},
		{name: "an mr prefixed domain", domain: "mr_note", action: "list", want: "code_review"},
		{name: "issue domain", domain: "issue", action: "list", want: "work_item"},
		{name: "issue named in the action", domain: "epic", action: "issue_list", want: "work_item"},
		{name: "pipeline domain", domain: "pipeline", action: "list", want: "ci_cd"},
		{name: "job domain", domain: "job", action: "list", want: "ci_cd"},
		{name: "a ci prefixed domain", domain: "ci_variable", action: "list", want: "ci_cd"},
		{name: "repository domain", domain: "repository", action: "tree", want: "source_control"},
		{name: "branch domain", domain: "branch", action: "list", want: "source_control"},
		{name: "tag domain", domain: "tag", action: "list", want: "source_control"},
		{name: "commit domain", domain: "commit", action: "list", want: "source_control"},
		{name: "release domain", domain: "release", action: "list", want: "delivery"},
		{name: "package domain", domain: "package", action: "list", want: "delivery"},
		{name: "project domain", domain: "project", action: "get", want: "collaboration"},
		{name: "group domain", domain: "group", action: "get", want: "collaboration"},
		{name: "user domain", domain: "user", action: "current", want: "collaboration"},
		{name: "a domain with no family is its own capability", domain: "snippet", action: "list", want: "snippet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferCapability(tc.domain, tc.action); got != tc.want {
				t.Errorf("inferCapability(%q, %q) = %q, want %q", tc.domain, tc.action, got, tc.want)
			}
		})
	}
}

// TestInferActionScope_PrefersTheSchemaThenTheDomain verifies that an action's
// scope is read from the params it takes when it takes one that says so, and
// from its domain otherwise.
//
// The schema is the stronger evidence: a group action that takes a project_id
// really is project-scoped. The domain fallback carries the two spellings of
// the user domain, since both reach the catalog.
func TestInferActionScope_PrefersTheSchemaThenTheDomain(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		schema map[string]any
		want   string
	}{
		{name: "a project_id param", domain: "group", schema: schemaWithProperties("project_id"), want: "project"},
		{name: "a group_id param", domain: "project", schema: schemaWithProperties("group_id"), want: "group"},
		{name: "admin domain", domain: "admin", want: "instance"},
		{name: "server domain", domain: "server", want: "instance"},
		{name: "user domain", domain: "user", want: "user"},
		{name: "users domain", domain: "Users ", want: "user"},
		{name: "any other domain", domain: "snippet", want: "gitlab"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferActionScope(tc.domain, tc.schema); got != tc.want {
				t.Errorf("inferActionScope(%q) = %q, want %q", tc.domain, got, tc.want)
			}
		})
	}
}

// TestDescribe_CanonicalizesObservedModelAliases verifies aliases observed in
// model output so dynamic execution remains tolerant of alternate naming.
func TestDescribe_CanonicalizesObservedModelAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := map[string]string{
		"issue.notes":                               "issue.note_list",
		"issue.notes.list":                          "issue.note_list",
		"pipeline.jobs":                             "job.list",
		"project.schedule_storage_move":             "storage_move.schedule_project",
		"merge_request.changes":                     "mr_review.changes_get",
		"merge_request.accept":                      "merge_request.merge",
		"project.hooks.list":                        "project.hook_list",
		"merge_request.emoji_award_create":          "merge_request.emoji_mr_create",
		"merge_request.emoji_award_delete":          "merge_request.emoji_mr_delete",
		"project.status_check_list":                 "external_status_check.list_project",
		"project.status_checks.list":                "external_status_check.list_project",
		"ci_job_token_scope.inbound_allowlist.list": "job.token_scope_list_inbound",
		"package.files":                             "package.file_list",
		"group.audit_events":                        "audit_event.list_group",
		"project.releases.list":                     "release.list",
		"deploy_token.create":                       "access.deploy_token_create_project",
		"deploy_key.create":                         "access.deploy_key_add",
		"deploy_key.delete":                         "access.deploy_key_delete",
		"deploy_key.get":                            "access.deploy_key_get",
		"deploy_key.update":                         "access.deploy_key_update",
		"branch.protected_list":                     "branch.get_protected",
		"branch.update_protection":                  "branch.update_protected",
		"issue.close":                               "issue.update",
		"issue.reopen":                              "issue.update",
		"merge_request.set_time_estimate":           "merge_request.time_estimate_set",
		"merge_request.time_estimate":               "merge_request.time_estimate_set",
		"merge_request.time_spent_add":              "merge_request.spent_time_add",
		"mr_review.draft_notes_publish":             "mr_review.draft_note_publish_all",
		"mr_review.publish":                         "mr_review.draft_note_publish_all",
		"package.list_generic":                      "package.list",
		"variable.create":                           "ci_variable.create",
		"group.variable.create":                     "ci_variable.group_create",
		"project_member.update":                     "project.member_edit",
		"project.member_remove":                     "project.member_delete",
		"project_member.remove":                     "project.member_delete",
		"webhook.add":                               "project.hook_add",
		"release.create_link":                       "release.link_create",
		"package.list_project":                      "package.list",
	}
	for alias, want := range tests {
		t.Run(alias, func(t *testing.T) {
			result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: alias})
			if err != nil {
				t.Fatalf("Describe() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Describe() result = %+v, want non-error", result)
			}
			if output.Count != 1 || output.Actions[0].ID != want {
				t.Fatalf("Describe() output = %+v, want %s", output, want)
			}
		})
	}
}

// TestDescribe_CanonicalizesProviderSpecificAliases verifies the Describe_CanonicalizesProviderSpecificAliases handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_CanonicalizesProviderSpecificAliases(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := map[string]string{
		"feature_flag_user_list.create":              "feature_flags.ff_user_list_create",
		"feature_flag_user_list.delete":              "feature_flags.ff_user_list_delete",
		"feature_flags.feature_flag_user_list":       "feature_flags.ff_user_list_list",
		"feature_flags.feature_flag_user_list_list":  "feature_flags.ff_user_list_list",
		"feature_flags.feature_flag_user_lists_list": "feature_flags.ff_user_list_list",
		"gitlab_issue.create":                        "issue.create",
		"gitlab_server.health_check":                 "server.health_check",
		"job.artifact_download":                      "job.download_single_artifact",
		"issue.link":                                 "issue.link_create",
		"issue.note.create":                          "issue.note_create",
		"issue.note.delete":                          "issue.note_delete",
		"issue.note.get":                             "issue.note_get",
		"issue.note.list":                            "issue.note_list",
		"issue.note.update":                          "issue.note_update",
		"issue_note.get":                             "issue.note_get",
		"issue_note.list":                            "issue.note_list",
		"repository_tree":                            "repository.tree",
		"repository_tree.list":                       "repository.tree",
		"repository_file.get":                        "repository.file_get",
		"repository_file.read":                       "repository.file_get",
		"repository_files.get_raw_file":              "repository.file_raw",
		"pipeline.schedule_variable_create":          "pipeline.schedule_create_variable",
		"pipeline.schedule_variable_delete":          "pipeline.schedule_delete_variable",
		"pipeline.schedule_variable_update":          "pipeline.schedule_edit_variable",
		"project.badge_update":                       "project.badge_edit",
		"merge_request.time_spent_reset":             "merge_request.spent_time_reset",
		"merge_request.emoji_mr_award_create":        "merge_request.emoji_mr_create",
		"merge_request.emoji_mr_award_delete":        "merge_request.emoji_mr_delete",
		"generic_package.list":                       "package.list",
		"job.token_scope_remove_inbound":             "job.token_scope_remove_project",
		"issue_note.create":                          "issue.note_create",
		"issue_note.delete":                          "issue.note_delete",
		"issue_note.update":                          "issue.note_update",
		"mr_review.draft_notes_publish_all":          "mr_review.draft_note_publish_all",
		"package.list_project_packages":              "package.list",
		"release.asset_link.delete":                  "release.link_delete",
		"release.asset_link.get":                     "release.link_get",
		"release.asset_link.list":                    "release.link_list",
		"release.asset_link.update":                  "release.link_update",
		"release_link.link_list":                     "release.link_list",
		"repository.tag.delete":                      "tag.delete",
		"wiki.show":                                  "wiki.get",
		"gitlab_interactive_issue.create":            "interactive.issue_create",
	}

	for alias, want := range tests {
		t.Run(alias, func(t *testing.T) {
			result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: alias})
			if err != nil {
				t.Fatalf("Describe() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Describe() result = %+v, want non-error", result)
			}
			if output.Count != 1 || output.Actions[0].ID != want {
				t.Fatalf("Describe() output = %+v, want %s", output, want)
			}
		})
	}
}

// TestDescribe_IncludesDisambiguationUsage verifies the Describe_IncludesDisambiguationUsage handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_IncludesDisambiguationUsage(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := map[string]string{
		"admin.settings_get":               "GitLab application settings",
		"access.deploy_key_list_project":   "deploy keys, not deploy tokens",
		"access.deploy_token_list_project": "registry/repository deploy credentials",
		// environment.protected_get is Premium (protected environments) and absent
		// from this Free realCatalogRegistry; covered by the tier tests instead.
		"environment.deployment_list":      "Lists deployments",
		"feature_flags.ff_user_list_get":   "user_list_iid",
		"issue.update":                     "state_event",
		"issue.note_get":                   "params.note_id",
		"job.download_single_artifact":     "one artifact file path",
		"merge_request.merge":              "auto_merge=true",
		"mr_review.draft_note_publish_all": "pending draft notes on a merge request in one call",
		"package.list":                     "created_at, name, version, or type",
		"pipeline.wait":                    "existing pipeline_id",
		"runner.remove":                    "numeric runner_id",
		"release.link_create":              "absolute http, https, or ftp URL",
		"release.link_get":                 "release asset link by link_id",
		"repository.compare":               "params.from to the base ref and params.to to the target ref",
		"search.code":                      "file contents",
		"search.projects":                  "project name",
		"package.registry_list_project":    "container registry image repositories",
	}

	for actionID, wantSubstring := range tests {
		t.Run(actionID, func(t *testing.T) {
			result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: actionID})
			if err != nil {
				t.Fatalf("Describe() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Describe() result = %+v, want non-error", result)
			}
			description := actionDescriptionByID(t, output, actionID)
			if !strings.Contains(description.Usage, wantSubstring) {
				t.Fatalf("usage = %q, want substring %q", description.Usage, wantSubstring)
			}
			if actionID == "repository.compare" && !strings.Contains(textContent(result), "Related actions") {
				t.Fatalf("Describe() markdown = %q, want related actions", textContent(result))
			}
		})
	}
}

// TestDescribe_IncludesConsolidatedRegisterMetaReplacementActions verifies the Describe_IncludesConsolidatedRegisterMetaReplacementActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_IncludesConsolidatedRegisterMetaReplacementActions(t *testing.T) {
	registry := realCatalogRegistry(t)

	actionIDs := []string{
		"feature_flags.feature_flag_list",
		"access.request_list_project",
		"package.registry_list_project",
		"snippet.project_get",
	}

	for _, actionID := range actionIDs {
		t.Run(actionID, func(t *testing.T) {
			result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: actionID})
			if err != nil {
				t.Fatalf("Describe() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Describe() result = %+v, want non-error", result)
			}
			if output.Count != 1 || output.Actions[0].ID != actionID {
				t.Fatalf("Describe() output = %+v, want %s", output, actionID)
			}
		})
	}
}

// TestDescribe_JobSingleArtifactRequiresArtifactPath verifies the Describe_JobSingleArtifactRequiresArtifactPath handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_JobSingleArtifactRequiresArtifactPath(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "job.download_single_artifact"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	description := actionDescriptionByID(t, output, "job.download_single_artifact")
	for _, required := range []string{"artifact_path", "job_id", "project_id"} {
		t.Run(required, func(t *testing.T) {
			if !slices.Contains(description.RequiredParams, required) {
				t.Fatalf("required params = %v, want %s", description.RequiredParams, required)
			}
		})
	}
	if params, ok := description.Example.Arguments["params"].(map[string]any); !ok || params["artifact_path"] == nil {
		t.Fatalf("example arguments = %#v, want artifact_path in params", description.Example.Arguments)
	}
}

// TestExecute_NormalizesCommonParameterAliases verifies that Execute rewrites
// common parameter aliases before dispatching to the canonical handler.
func TestExecute_NormalizesCommonParameterAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{
		Action: "project.schedule_storage_move",
		Params: map[string]any{"project_id": 123, "shard": "default"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["destination_storage_name"] != "default" {
		t.Fatalf("destination_storage_name = %v, want default", data["destination_storage_name"])
	}
}

// TestExecute_DispatchesReadOnlyAction verifies that Execute forwards read-only
// action parameters to the registered route handler and returns its output.
func TestExecute_DispatchesReadOnlyAction(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.list", Params: map[string]any{"owned": true}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["owned"] != true {
		t.Fatalf("owned = %v, want true", data["owned"])
	}
}

// TestExecute_UsesCatalogFormatter verifies the Execute_UsesCatalogFormatter handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestExecute_UsesCatalogFormatter(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{
		ToolName: "gitlab_custom",
		FormatResult: func(any) *mcp.CallToolResult {
			return toolutil.ToolResultAnnotated("custom formatted result", toolutil.ContentDetail)
		},
	})
	group.SetAction(actioncatalog.Action{
		Name: "get",
		Route: toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		}),
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	registry := NewRegistryFromCatalog(catalog)

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "custom.get", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	if text := textContent(result); text != "custom formatted result" {
		t.Fatalf("Execute() text = %q, want custom formatter output", text)
	}
	if data, ok := output.(map[string]any); !ok || data["ok"] != true {
		t.Fatalf("Execute() output = %#v, want route output", output)
	}
}

// TestExecute_CanonicalizesAlias verifies the Execute_CanonicalizesAlias handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestExecute_CanonicalizesAlias(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "repository_file.get", Params: map[string]any{"project_id": 123, "file_path": "README.md", "ref": "main"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["action"] != "repository.file_get" {
		t.Fatalf("action = %v, want repository.file_get", data["action"])
	}
}

type executeNormalizationCase struct {
	name   string
	input  ExecuteInput
	assert func(t *testing.T, output any)
}

// TestExecute_NormalizesActionScopedParameterAliases verifies dynamic execute
// accepts ambiguous model aliases only for actions where the schema is clear.
func TestExecute_NormalizesActionScopedParameterAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	runExecuteNormalizationCases(t, registry, coreActionScopedParameterAliasCases())
	runExecuteNormalizationCases(t, registry, resourceActionScopedParameterAliasCases())
}

func coreActionScopedParameterAliasCases() []executeNormalizationCase {
	cases := append([]executeNormalizationCase{}, coreJobAndRepositoryAliasCases()...)
	cases = append(cases, coreProjectMemberAliasCases()...)
	cases = append(cases, coreIssueAliasCases()...)
	cases = append(cases, coreMergeRequestAndPipelineAliasCases()...)
	cases = append(cases, coreBranchAliasCases()...)
	return cases
}

func coreJobAndRepositoryAliasCases() []executeNormalizationCase {
	return []executeNormalizationCase{
		{
			name:  "job status to scope",
			input: ExecuteInput{Action: "job.list", Params: map[string]any{"project_id": 123, "pipeline_id": 456, "status": "failed"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["scope"] != "failed" {
					t.Fatalf("output = %#v, want scope failed", output)
				}
			},
		},
		{
			name:  "repository branch to ref",
			input: ExecuteInput{Action: "repository.file_get", Params: map[string]any{"project_id": 123, "file_path": "README.md", "branch": "main"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["ref"] != "main" {
					t.Fatalf("output = %#v, want ref main", output)
				}
			},
		},
	}
}

func coreProjectMemberAliasCases() []executeNormalizationCase {
	return []executeNormalizationCase{
		{
			name:  "project member role to numeric access level",
			input: ExecuteInput{Action: "project.member_add", Params: map[string]any{"project_id": 123, "user_id": 5, "access_level": "Reporter"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["access_level"] != 20 {
					t.Fatalf("output = %#v, want access_level 20", output)
				}
			},
		},
		{
			name:  "project member numeric string access level",
			input: ExecuteInput{Action: "project.member_edit", Params: map[string]any{"project_id": 123, "user_id": 5, "access_level": "30"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["access_level"] != 30 {
					t.Fatalf("output = %#v, want access_level 30", output)
				}
			},
		},
	}
}

func coreIssueAliasCases() []executeNormalizationCase {
	return []executeNormalizationCase{
		{
			name:  "issue link aliases same project target",
			input: ExecuteInput{Action: "issue.link_create", Params: map[string]any{"project_id": 123, "issue_iid": 1, "linked_issue_iid": 2, "type": "relates_to"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["target_issue_iid"] != 2 || data["target_project_id"] != 123 || data["link_type"] != "relates_to" {
					t.Fatalf("output = %#v, want target_issue_iid 2, target_project_id 123, and link_type relates_to", output)
				}
			},
		},
		{
			name:  "issue spent time note alias",
			input: ExecuteInput{Action: "issue.spent_time_add", Params: map[string]any{"project_id": 123, "issue_iid": 1, "duration": "30m", "note": "pairing"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["summary"] != "pairing" {
					t.Fatalf("output = %#v, want summary pairing", output)
				}
				if _, ok := data["note"]; ok {
					t.Fatalf("output = %#v, want note alias removed", output)
				}
			},
		},
		{
			name:  "issue update closed state event",
			input: ExecuteInput{Action: "issue.update", Params: map[string]any{"project_id": 123, "issue_iid": 1, "state_event": "closed"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["state_event"] != "close" {
					t.Fatalf("output = %#v, want state_event close", output)
				}
			},
		},
		{
			name:  "issue close alias injects state event",
			input: ExecuteInput{Action: "issue.close", Params: map[string]any{"project_id": 123, "issue_iid": 1}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["state_event"] != "close" {
					t.Fatalf("output = %#v, want state_event close", output)
				}
			},
		},
		{
			name:  "issue reopen alias injects state event",
			input: ExecuteInput{Action: "issue.reopen", Params: map[string]any{"project_id": 123, "issue_iid": 1}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["state_event"] != "reopen" {
					t.Fatalf("output = %#v, want state_event reopen", output)
				}
			},
		},
	}
}

func coreMergeRequestAndPipelineAliasCases() []executeNormalizationCase {
	return []executeNormalizationCase{
		{
			name:  "merge request emoji drops stale duration",
			input: ExecuteInput{Action: "merge_request.emoji_mr_create", Params: map[string]any{"project_id": 123, "merge_request_iid": 3, "name": "eyes", "duration": "15m"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["name"] != "eyes" {
					t.Fatalf("output = %#v, want name eyes", output)
				}
				if _, ok := data["duration"]; ok {
					t.Fatalf("output = %#v, want duration removed", output)
				}
			},
		},
		{
			name:  "pipeline schedule name to description",
			input: ExecuteInput{Action: "pipeline.schedule_create", Params: map[string]any{"project_id": 123, "name": "nightly", "ref": "main", "cron": "0 1 * * *"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["description"] != "nightly" {
					t.Fatalf("output = %#v, want description nightly", output)
				}
				if _, ok := data["name"]; ok {
					t.Fatalf("output = %#v, want name alias removed", output)
				}
			},
		},
	}
}

func coreBranchAliasCases() []executeNormalizationCase {
	return []executeNormalizationCase{
		{
			name: "branch protect role access levels",
			input: ExecuteInput{Action: "branch.protect", Params: map[string]any{
				"project_id":         123,
				"branch_name":        "main",
				"push_access_level":  "maintainer",
				"merge_access_level": "maintainer",
				"allow_force_push":   false,
			}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["push_access_level"] != 40 || data["merge_access_level"] != 40 {
					t.Fatalf("output = %#v, want access levels 40", output)
				}
			},
		},
	}
}

func resourceActionScopedParameterAliasCases() []executeNormalizationCase {
	return []executeNormalizationCase{
		{
			name:   "group label update name alias",
			input:  ExecuteInput{Action: "group.group_label_update", Params: map[string]any{"group_id": "my-org", "label_id": 31, "name": "next-label"}},
			assert: assertOutputAll(assertOutputField("new_name", "next-label"), assertOutputMissing("name")),
		},
		{
			name:   "feature flag version alias",
			input:  ExecuteInput{Action: "feature_flags.feature_flag_create", Params: map[string]any{"project_id": 123, "name": "eval", "new_version_flag": "new_version_flag"}},
			assert: assertOutputField("version", "new_version_flag"),
		},
		{
			name:   "feature flag user list drops feature flag name",
			input:  ExecuteInput{Action: "feature_flags.ff_user_list_list", Params: map[string]any{"project_id": 123, "name": "eval_flag", "per_page": 20}},
			assert: assertOutputAll(assertOutputMissing("name"), assertOutputField("per_page", 20)),
		},
		{
			name:   "release link tag alias",
			input:  ExecuteInput{Action: "release.link_create", Params: map[string]any{"project_id": 123, "release_tag_name": "v1.0.0", "name": "asset", "url": "https://example.com/asset"}},
			assert: assertOutputField("tag_name", "v1.0.0"),
		},
		{
			name: "snippet create drops file action",
			input: ExecuteInput{Action: "snippet.project_create", Params: map[string]any{
				"project_id": 123,
				"title":      "snippet",
				"files": []any{map[string]any{
					"action":    "create",
					"file_path": "snippet.md",
					"content":   "body",
				}},
			}},
			assert: assertOutputNestedFileMissing("action"),
		},
		{
			name: "snippet create builds files from single file params",
			input: ExecuteInput{Action: "snippet.project_create", Params: map[string]any{
				"project_id": 123,
				"title":      "snippet",
				"file_name":  "snippet.md",
				"content":    "body",
			}},
			assert: assertOutputAll(assertOutputNestedFileField("file_path", "snippet.md"), assertOutputNestedFileField("content", "body"), assertOutputMissing("file_name"), assertOutputMissing("content")),
		},
		{
			name: "snippet create normalizes nested file name",
			input: ExecuteInput{Action: "snippet.project_create", Params: map[string]any{
				"project_id": 123,
				"title":      "snippet",
				"files": []any{map[string]any{
					"file_name": "snippet.md",
					"content":   "body",
				}},
			}},
			assert: assertOutputAll(assertOutputNestedFileField("file_path", "snippet.md"), assertOutputNestedFileMissing("file_name")),
		},
		{
			name:   "runner paused string to bool",
			input:  ExecuteInput{Action: "runner.update", Params: map[string]any{"runner_id": 99, "paused": "true"}},
			assert: assertOutputField("paused", true),
		},
	}
}

func assertOutputAll(assertions ...func(*testing.T, any)) func(*testing.T, any) {
	return func(t *testing.T, output any) {
		t.Helper()
		for _, assertion := range assertions {
			assertion(t, output)
		}
	}
}

func assertOutputField(key string, want any) func(*testing.T, any) {
	return func(t *testing.T, output any) {
		t.Helper()
		data := output.(map[string]any)
		if data[key] != want {
			t.Fatalf("output = %#v, want %s %v", output, key, want)
		}
	}
}

func assertOutputMissing(key string) func(*testing.T, any) {
	return func(t *testing.T, output any) {
		t.Helper()
		data := output.(map[string]any)
		if _, ok := data[key]; ok {
			t.Fatalf("output = %#v, want %s removed", output, key)
		}
	}
}

func assertOutputNestedFileField(key string, want any) func(*testing.T, any) {
	return func(t *testing.T, output any) {
		t.Helper()
		file := firstOutputFile(output)
		if file[key] != want {
			t.Fatalf("output = %#v, want files[0].%s %v", output, key, want)
		}
	}
}

func assertOutputNestedFileMissing(key string) func(*testing.T, any) {
	return func(t *testing.T, output any) {
		t.Helper()
		file := firstOutputFile(output)
		if _, ok := file[key]; ok {
			t.Fatalf("output = %#v, want files[0].%s removed", output, key)
		}
	}
}

func firstOutputFile(output any) map[string]any {
	data := output.(map[string]any)
	files := data["files"].([]any)
	return files[0].(map[string]any)
}

func runExecuteNormalizationCases(t *testing.T, registry *Registry, tests []executeNormalizationCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Execute(t.Context(), nil, tt.input)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Execute() result = %+v, want non-error", result)
			}
			tt.assert(t, output)
		})
	}
}

// TestNormalizeActionScopedParamsWithExplanation verifies that NormalizeActionScopedParamsWithExplanation forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestNormalizeActionScopedParamsWithExplanation(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"project_id", "file_path", "ref"},
		"properties": map[string]any{
			"project_id": map[string]any{"type": "integer"},
			"file_path":  map[string]any{"type": "string"},
			"ref":        map[string]any{"type": "string"},
		},
	}
	params := map[string]any{"project_id": 123, "file_path": "README.md", "branch": "main"}

	normalized, explanations := NormalizeActionScopedParamsWithExplanation("repository.file_get", params, schema)
	if normalized["ref"] != "main" {
		t.Fatalf("normalized = %#v, want ref main", normalized)
	}
	if _, ok := normalized["branch"]; ok {
		t.Fatalf("normalized = %#v, want branch removed", normalized)
	}
	if len(explanations) != 1 {
		t.Fatalf("explanations = %+v, want one explanation", explanations)
	}
	if explanations[0].Alias != "branch" || explanations[0].Canonical != "ref" || explanations[0].Source != "dynamic_action_scoped" {
		t.Fatalf("explanations = %+v, want branch -> ref action-scoped explanation", explanations)
	}
}

// TestNormalizeActionScopedParamsWithExplanation_KeepsValidSnippetCreateParams
// verifies snippet create keeps top-level file_name/content when the selected
// schema already accepts them.
func TestNormalizeActionScopedParamsWithExplanation_KeepsValidSnippetCreateParams(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"project_id", "title", "file_name", "content"},
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
			"title":      map[string]any{"type": "string"},
			"file_name":  map[string]any{"type": "string"},
			"content":    map[string]any{"type": "string"},
			"files":      map[string]any{"type": "array"},
		},
	}
	params := map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "title": "snippet", "file_name": "snippet.md", "content": "body"}

	normalized, explanations := NormalizeActionScopedParamsWithExplanation("snippet.project_create", params, schema)
	if normalized["file_name"] != "snippet.md" || normalized["content"] != "body" {
		t.Fatalf("normalized = %#v, want top-level file_name/content preserved", normalized)
	}
	if _, ok := normalized["files"]; ok {
		t.Fatalf("normalized = %#v, want files not synthesized", normalized)
	}
	if len(explanations) != 0 {
		t.Fatalf("explanations = %+v, want no normalization explanation", explanations)
	}
}

// TestActionScopedParamAliases_CoversDocumentedActions verifies the declarative
// metadata includes every action currently normalized by dynamic execute.
func TestActionScopedParamAliases_CoversDocumentedActions(t *testing.T) {
	aliases := actioncompat.ParameterAliases()
	wantActions := []string{
		"job.list",
		"repository.file_get",
		"issue.link_create",
		"issue.spent_time_add",
		"issue.update",
		"merge_request.emoji_mr_create",
		"pipeline.schedule_create",
		"pipeline.schedule_update",
		"branch.protect",
		"feature_flags.feature_flag_create",
		"feature_flags.ff_user_list_list",
		"group.group_label_update",
		"project.member_add",
		"project.member_edit",
		"release.link_create",
		"release.link_create_batch",
		"release.link_delete",
		"release.link_get",
		"release.link_list",
		"release.link_update",
		"runner.update",
		"snippet.project_create",
	}
	for _, actionID := range wantActions {
		t.Run(actionID, func(t *testing.T) {
			if !slices.ContainsFunc(aliases, func(alias actioncompat.ParameterAlias) bool { return alias.ActionID == actionID }) {
				t.Fatalf("ParameterAliases() = %+v, want action %s", aliases, actionID)
			}
		})
	}
}

// TestDynamicRegister_DoesNotOwnCompatibilityPolicyTables guards the
// catalog-first boundary: Dynamic may adapt compatibility metadata, but the
// source policy tables belong to actioncompat and ActionSpec projection.
func TestDynamicRegister_DoesNotOwnCompatibilityPolicyTables(t *testing.T) {
	source, err := os.ReadFile("register.go")
	if err != nil {
		t.Fatalf("ReadFile(register.go) error = %v", err)
	}
	for _, forbidden := range []string{
		"return annotateCompatibilityAliases([]actionAlias{",
		"func buildSnippetCreateFilesFromSingleFileParams(",
		"func gitlabAccessLevelValue(",
		"func boolStringValue(",
	} {
		t.Run(forbidden, func(t *testing.T) {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("register.go still owns compatibility policy table/helper %q; move policy to actioncompat", forbidden)
			}
		})
	}
}

// TestExecute_ReportsUnknownAndMissingParamsBeforeDispatch verifies that Execute_ReportsUnknownAndMissingParamsBeforeDispatch returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestExecute_ReportsUnknownAndMissingParamsBeforeDispatch(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "repository.file_get", Params: map[string]any{"project_id": 123, "file_path": "README.md", "reff": "main"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	text := textContent(result)
	for _, want := range []string{"Unknown params: reff", "Did you mean reff -> ref", "Missing required params: ref", "Valid params: file_path, project_id, ref"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Fatalf("Execute() error text = %q, want %q", text, want)
			}
		})
	}
}

// TestExecute_InvalidParamsReportOnlyTheSectionsThatApply verifies that the
// invalid-params refusal carries the unknown list, the suggestions and the
// missing list only when each has something to say.
//
// The three sections are independent guards over the same message. A missing
// required param must not be announced under "Unknown params", an unknown one
// that resembles nothing must not be answered "Did you mean ?", and a call that
// supplied every required param must not be told something is missing: each of
// those reads as a different mistake than the one the caller made.
func TestExecute_InvalidParamsReportOnlyTheSectionsThatApply(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	t.Run("a missing param is not announced as an unknown one", func(t *testing.T) {
		result, _, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.get", Params: map[string]any{}})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("Execute() result = %+v, want tool error", result)
		}
		text := textContent(result)
		if !strings.Contains(text, "Missing required params: project_id.") {
			t.Fatalf("Execute() error text = %q, want the missing param named", text)
		}
		if strings.Contains(text, "Unknown params") {
			t.Fatalf("Execute() error text = %q, want no unknown-params section", text)
		}
	})

	t.Run("an unknown param that resembles nothing gets no suggestion", func(t *testing.T) {
		result, _, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.get", Params: map[string]any{"project_id": 123, "zzzzzzzz": true}})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("Execute() result = %+v, want tool error", result)
		}
		text := textContent(result)
		if !strings.Contains(text, "Unknown params: zzzzzzzz.") {
			t.Fatalf("Execute() error text = %q, want the unknown param named", text)
		}
		if strings.Contains(text, "Did you mean") {
			t.Fatalf("Execute() error text = %q, want no suggestion section", text)
		}
		if strings.Contains(text, "Missing required params") {
			t.Fatalf("Execute() error text = %q, want no missing-params section", text)
		}
	})
}

// TestExecute_RejectsUnsupportedPipelineScheduleVariableSecurityFields verifies
// dynamic execute does not silently drop user-supplied security intent.
func TestExecute_RejectsUnsupportedPipelineScheduleVariableSecurityFields(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "pipeline.schedule_create_variable", Params: map[string]any{
		"project_id":  123,
		"schedule_id": 109,
		"key":         "SCHEDULE_CRUD_TOKEN",
		"value":       "secret",
		"masked":      true,
		"protected":   true,
	}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	text := textContent(result)
	for _, want := range []string{"Unknown params: masked, protected", "Valid params: key, project_id, schedule_id, value, variable_type"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Fatalf("Execute() error text = %q, want %q", text, want)
			}
		})
	}
}

// TestExecute_RejectsIssueLifecycleAliasStateConflict verifies the Execute_RejectsIssueLifecycleAliasStateConflict handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestExecute_RejectsIssueLifecycleAliasStateConflict(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "issue.close", Params: map[string]any{"project_id": 123, "issue_iid": 1, "state_event": "reopen"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	if text := textContent(result); !strings.Contains(text, "implies state_event") || !strings.Contains(text, "issue.update") {
		t.Fatalf("Execute() error text = %q, want conflict guidance", text)
	}
}

// TestExecute_IssueLifecycleAliasAcceptsAnAgreeingOrUnreadableStateEvent
// verifies that the conflict guard refuses a state_event only when this server
// can read it AND it disagrees with the alias.
//
// Either half alone must let the call through: a caller who spells issue.close
// with state_event=closed has asked for the same thing twice, and a value the
// alias table does not know is GitLab's to reject rather than ours to
// reinterpret. Turning the conjunction into a disjunction would refuse both,
// which is why the two cases are asserted to dispatch rather than error.
func TestExecute_IssueLifecycleAliasAcceptsAnAgreeingOrUnreadableStateEvent(t *testing.T) {
	cases := []struct {
		name       string
		stateEvent string
		want       string
	}{
		{name: "an agreeing spelling is normalized and dispatched", stateEvent: "closed", want: "close"},
		{name: "a value this server cannot read is passed through", stateEvent: "archived", want: "archived"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewRegistry(testRoutes(t))

			result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "issue.close", Params: map[string]any{"project_id": 123, "issue_iid": 1, "state_event": tc.stateEvent}})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Execute() result = %+v, want a dispatched call", result)
			}
			data, ok := output.(map[string]any)
			if !ok {
				t.Fatalf("Execute() output type = %T, want map[string]any", output)
			}
			if data["state_event"] != tc.want {
				t.Fatalf("Execute() state_event = %v, want %q", data["state_event"], tc.want)
			}
		})
	}
}

// TestExecute_IssueLifecycleAliasInjectsStateEventOnlyIntoIssueUpdate verifies
// that the lifecycle alias fills state_event only when the action it resolved
// to really is issue.update.
//
// The alias exists because issue.close is a compatibility spelling of
// issue.update, so the injected parameter is meaningful only for that handler.
// A catalog that registers issue.close as an action of its own must be
// dispatched with the params the caller sent and nothing added.
func TestExecute_IssueLifecycleAliasInjectsStateEventOnlyIntoIssueUpdate(t *testing.T) {
	var captured map[string]any
	registry := NewRegistry(map[string]toolutil.ActionMap{
		"gitlab_issue": {
			"close": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					captured = maps.Clone(params)
					return map[string]any{"closed": true}, nil
				},
			},
		},
	})

	result, _, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "issue.close", Params: map[string]any{"project_id": 123, "issue_iid": 1}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want a dispatched call", result)
	}
	if _, injected := captured["state_event"]; injected {
		t.Fatalf("Execute() dispatched params = %v, want no injected state_event", captured)
	}
}

// TestExecute_LogsTheNormalizationCountOnlyWhenThereIsOne verifies the debug
// record that says how many compatibility normalizations a dispatch applied:
// it is written when there was at least one, carries their number, and is not
// written at all for a call that needed none.
//
// The count is the sum of the common and action-scoped normalizations, and the
// record is the only place either is observable. A call whose params were taken
// verbatim must stay silent, or the line stops meaning anything.
func TestExecute_LogsTheNormalizationCountOnlyWhenThereIsOne(t *testing.T) {
	logs := captureDebugSlog(t)
	registry := NewRegistry(testRoutes(t))

	result, _, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.get", Params: map[string]any{"project_id": 123}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("Execute(project.get) result = %+v, err = %v, want a dispatched call", result, err)
	}
	if counts := normalizationLogCounts(t, logs); len(counts) != 0 {
		t.Fatalf("normalization records for a call that needed none = %v, want none", counts)
	}

	result, _, err = registry.Execute(t.Context(), nil, ExecuteInput{Action: "issue.close", Params: map[string]any{"project_id": 123, "issue_iid": 1}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("Execute(issue.close) result = %+v, err = %v, want a dispatched call", result, err)
	}
	counts := normalizationLogCounts(t, logs)
	if len(counts) != 1 {
		t.Fatalf("normalization records for the lifecycle alias = %v, want exactly one", counts)
	}
	if counts[0] != 1 {
		t.Fatalf("normalizations = %d, want 1 (the state_event the alias filled in)", counts[0])
	}
}

// captureDebugSlog redirects the default logger to a buffer at debug level for
// the rest of the test, so a record the server writes below the default level
// can be asserted on.
func captureDebugSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// normalizationLogCounts returns the normalizations attribute of every param
// normalization record written to buf so far, in order.
func normalizationLogCounts(t *testing.T, buf *bytes.Buffer) []int {
	t.Helper()
	counts := make([]int, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record struct {
			Msg            string `json:"msg"`
			Normalizations int    `json:"normalizations"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		if record.Msg == "normalized dynamic action params" {
			counts = append(counts, record.Normalizations)
		}
	}
	return counts
}

// TestExecuteCallName_FallsBackToTheToolNameForAnEmptyAction verifies how a
// dynamic dispatch is named in a log record: with the action when there is one,
// and with the bare tool name when there is not.
//
// A refusal is logged before the action is known, so the empty case is real:
// naming it "gitlab_execute_action/" would read as an action whose ID is blank.
func TestExecuteCallName_FallsBackToTheToolNameForAnEmptyAction(t *testing.T) {
	if got := executeCallName(""); got != ExecuteActionToolName {
		t.Errorf("executeCallName(\"\") = %q, want %q", got, ExecuteActionToolName)
	}
	if got := executeCallName("issue.list"); got != ExecuteActionToolName+"/issue.list" {
		t.Errorf("executeCallName(issue.list) = %q, want the tool name and the action", got)
	}
}

// TestMissingDynamicRequiredParams_AcceptsAnyOfAlternatives verifies execute
// validation accepts either single-file or multi-file snippet creation shapes.
func TestMissingDynamicRequiredParams_AcceptsAnyOfAlternatives(t *testing.T) {
	schema := map[string]any{
		"required": []any{"project_id", "title"},
		"anyOf": []any{
			map[string]any{"required": []any{"file_name", "content"}},
			map[string]any{"required": []any{"files"}},
		},
	}

	if got := missingDynamicRequiredParams(schema, map[string]any{"project_id": "p", "title": "t", "file_name": "a.md", "content": "body"}); len(got) != 0 {
		t.Fatalf("missingDynamicRequiredParams(single-file) = %v, want none", got)
	}
	if got := missingDynamicRequiredParams(schema, map[string]any{"project_id": "p", "title": "t", "files": []any{map[string]any{"file_path": "a.md", "content": "body"}}}); len(got) != 0 {
		t.Fatalf("missingDynamicRequiredParams(files) = %v, want none", got)
	}
	if got := missingDynamicRequiredParams(schema, map[string]any{"project_id": "p", "title": "t", "file_name": "a.md"}); !slices.Equal(got, []string{"content"}) {
		t.Fatalf("missingDynamicRequiredParams(partial) = %v, want content", got)
	}
}

// TestMissingAlternativeRequiredParams_ReportsTheCheapestUnsatisfiedGroup
// verifies that when no alternative is satisfied the refusal names the one
// closest to being satisfied, not the first one declared.
//
// The caller is being told what to send next, so the shortest list of missing
// params is the useful answer: reporting "file_name, content" when adding
// "files" alone would do sends them the long way round.
func TestMissingAlternativeRequiredParams_ReportsTheCheapestUnsatisfiedGroup(t *testing.T) {
	schema := map[string]any{"anyOf": []any{
		map[string]any{"required": []any{"file_name", "content"}},
		map[string]any{"required": []any{"files"}},
	}}

	if got := missingAlternativeRequiredParams(schema, map[string]any{"project_id": "p"}); !slices.Equal(got, []string{"files"}) {
		t.Fatalf("missingAlternativeRequiredParams() = %v, want the single-param alternative", got)
	}
}

// TestAlternativeRequiredParamGroups_KeywordAndGroupSelection verifies which
// alternatives the validator reads out of a schema: an anyOf that declares no
// alternatives leaves oneOf to answer, and an alternative that requires nothing
// contributes no group.
//
// Both shapes look like "there are alternatives" from the outside and mean the
// opposite. An empty anyOf that stopped the search would hide a real oneOf, and
// an empty group would satisfy every call, which turns the whole check off.
func TestAlternativeRequiredParamGroups_KeywordAndGroupSelection(t *testing.T) {
	t.Run("an empty anyOf falls back to oneOf", func(t *testing.T) {
		groups := alternativeRequiredParamGroups(map[string]any{
			"anyOf": []any{},
			"oneOf": []any{map[string]any{"required": []any{"files"}}},
		})
		if len(groups) != 1 || !slices.Equal(groups[0], []string{"files"}) {
			t.Fatalf("alternativeRequiredParamGroups() = %v, want the oneOf group", groups)
		}
	})

	t.Run("an alternative requiring nothing is skipped", func(t *testing.T) {
		groups := alternativeRequiredParamGroups(map[string]any{"anyOf": []any{
			map[string]any{"required": []any{}},
			map[string]any{"required": []any{"files"}},
		}})
		if len(groups) != 1 || !slices.Equal(groups[0], []string{"files"}) {
			t.Fatalf("alternativeRequiredParamGroups() = %v, want only the group that requires something", groups)
		}
	})
}

// TestExecute_UnknownActionSuggestsCanonicalIDs verifies the Execute_UnknownActionSuggestsCanonicalIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestExecute_UnknownActionSuggestsCanonicalIDs(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.destroy"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	if !strings.Contains(textContent(result), "`project.delete`") {
		t.Fatalf("Execute() error text = %q, want project.delete suggestion", textContent(result))
	}
}

// TestSuggestActionIDs_BreaksAScoreTieByCanonicalID verifies the order the "did
// you mean" list comes back in when several actions score the same.
//
// The list is cut to five, so the tie-break decides which suggestions a caller
// is shown at all and not merely in what order. Ascending canonical ID is what
// makes that answer the same on every call and in every process: the scores
// come out of a map walk, so a comparison that reported a tie as a difference,
// or ordered one the other way, would let one mistyped action produce
// different advice on two servers of one fleet.
func TestSuggestActionIDs_BreaksAScoreTieByCanonicalID(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	// Every member action matches this one term the same way, through a word of
	// its action name, so the whole set ties and only the ID separates them.
	got := registry.suggestActionIDs("member", 10)
	members := make([]string, 0, len(got))
	for _, suggestion := range got {
		if strings.Contains(suggestion, "member") {
			members = append(members, suggestion)
		}
	}

	if len(members) < 2 {
		t.Fatalf("suggestActionIDs(member) = %v, want at least two member actions to tie", got)
	}
	if !slices.IsSorted(members) {
		t.Fatalf("member suggestions = %v, want them in ascending canonical ID order", members)
	}
}

// TestExecute_RejectsAmbiguousAlias verifies the Execute_RejectsAmbiguousAlias handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestExecute_RejectsAmbiguousAlias(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "danger.delete", Params: map[string]any{"project_id": 123}, Confirm: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	text := textContent(result)
	if !strings.Contains(text, "ambiguous") || !strings.Contains(text, "`project.delete`") || !strings.Contains(text, "`package.delete`") {
		t.Fatalf("Execute() error text = %q, want ambiguous canonical suggestions", text)
	}
}

// TestDescribe_RejectsAmbiguousAlias verifies the Describe_RejectsAmbiguousAlias handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDescribe_RejectsAmbiguousAlias(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "danger.delete"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Describe() result = %+v, want tool error", result)
	}
	if output.Count != 0 || len(output.Actions) != 0 {
		t.Fatalf("Describe() output = %+v, want empty output", output)
	}
}

// TestDescribe_CurrentAmbiguousAliasBehaviorRemainsStable protects the current
// contract that ambiguous aliases are reported with canonical repair targets.
func TestDescribe_CurrentAmbiguousAliasBehaviorRemainsStable(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "resource.remove", Canonical: "project.delete"},
		{Alias: "resource.remove", Canonical: "package.delete"},
	})

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "resource.remove"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Describe() result = %+v, want tool error", result)
	}
	if output.Count != 0 || len(output.Actions) != 0 {
		t.Fatalf("Describe() output = %+v, want empty output", output)
	}
	text := textContent(result)
	if !strings.Contains(text, "ambiguous") || !strings.Contains(text, "`project.delete`") || !strings.Contains(text, "`package.delete`") {
		t.Fatalf("Describe() text = %q, want ambiguous canonical repair guidance", text)
	}
}

// TestExecute_DestructiveActionRequiresConfirm verifies the Execute_DestructiveActionRequiresConfirm handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestExecute_DestructiveActionRequiresConfirm(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.delete", Params: map[string]any{"project_id": 123}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	if !strings.Contains(textContent(result), "confirm=true") {
		t.Fatalf("Execute() error text = %q, want confirm=true hint", textContent(result))
	}
}

// TestExecute_DestructiveActionExecutesWithConfirm verifies that destructive
// actions dispatch normally once the caller provides explicit confirmation.
func TestExecute_DestructiveActionExecutesWithConfirm(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.delete", Params: map[string]any{"project_id": 123}, Confirm: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["confirm"] != true {
		t.Fatalf("confirm = %v, want true", data["confirm"])
	}
}

// TestExecute_CurrentDestructiveSafetyRemainsStable verifies the Execute_CurrentDestructiveSafetyRemainsStable handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestExecute_CurrentDestructiveSafetyRemainsStable(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	blocked, blockedOutput, blockedErr := registry.Execute(t.Context(), nil, ExecuteInput{
		Action: "project.delete",
		Params: map[string]any{"project_id": 123},
	})
	if blockedErr != nil {
		t.Fatalf("Execute(blocked) error = %v", blockedErr)
	}
	if blocked == nil || !blocked.IsError {
		t.Fatalf("Execute(blocked) result = %+v, want tool error", blocked)
	}
	if blockedOutput != nil {
		t.Fatalf("Execute(blocked) output = %+v, want nil", blockedOutput)
	}
	if !strings.Contains(textContent(blocked), "confirm=true") {
		t.Fatalf("Execute(blocked) text = %q, want confirm guidance", textContent(blocked))
	}

	allowed, allowedOutput, allowedErr := registry.Execute(t.Context(), nil, ExecuteInput{
		Action:  "project.delete",
		Params:  map[string]any{"project_id": 123},
		Confirm: true,
	})
	if allowedErr != nil {
		t.Fatalf("Execute(allowed) error = %v", allowedErr)
	}
	if allowed == nil || allowed.IsError {
		t.Fatalf("Execute(allowed) result = %+v, want non-error", allowed)
	}
	data, ok := allowedOutput.(map[string]any)
	if !ok {
		t.Fatalf("Execute(allowed) output = %T, want map[string]any", allowedOutput)
	}
	if data["confirm"] != true {
		t.Fatalf("Execute(allowed) confirm = %v, want true", data["confirm"])
	}
}

// TestRegisterCatalogFindExecuteTools_ExposesDynamicTools verifies the RegisterCatalogFindExecuteTools_ExposesDynamicTools handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRegisterCatalogFindExecuteTools_ExposesDynamicTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogFindExecuteTools(server, actioncatalog.FromActionMaps(testRoutes(t)))

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(tools.Tools))
	}
	findTool := listedTool(t, tools.Tools, findToolName)
	if findTool.Description != findToolDescription || !strings.Contains(findTool.Description, "Read-only") {
		t.Fatalf("gitlab_find_action description = %q, want read-only lookup guidance", findTool.Description)
	}
	findSchema := listedToolInputSchema(t, tools.Tools, findToolName)
	if description := schemaPropertyDescription(findSchema, "query"); !strings.Contains(description, "domain or resource with a verb") {
		t.Fatalf("gitlab_find_action query description = %q, want semantic query guidance", description)
	}
	executeTool := listedTool(t, tools.Tools, executeActionToolName)
	if executeTool.Description != executeActionToolDescription || !strings.Contains(executeTool.Description, "top-level confirm=true") || !strings.Contains(executeTool.Description, "Use find first only") {
		t.Fatalf("gitlab_execute_action description = %q, want compact confirmation guidance", executeTool.Description)
	}
	executeSchema := listedToolInputSchema(t, tools.Tools, "gitlab_execute_action")
	if !slices.Contains(schemaRequired(executeSchema), "params") {
		t.Fatalf("gitlab_execute_action required = %v, want params", schemaRequired(executeSchema))
	}
	assertSchemaHasProperties(t, executeSchema, "action", "params", "confirm")
	if description := schemaPropertyDescription(executeSchema, "action"); !strings.Contains(description, "returned by gitlab_find_action") {
		t.Fatalf("gitlab_execute_action action description = %q, want find linkage", description)
	}
	if description := schemaPropertyDescription(executeSchema, "confirm"); !strings.Contains(description, "top-level confirm=true") {
		t.Fatalf("gitlab_execute_action confirm description = %q, want top-level confirm guidance", description)
	}

	executeOutputSchema := listedToolOutputSchema(t, tools.Tools, "gitlab_execute_action")
	if executeOutputSchema["type"] != "object" || executeOutputSchema["additionalProperties"] != true {
		t.Fatalf("gitlab_execute_action output schema = %v, want open object schema", executeOutputSchema)
	}
	assertSchemaHasProperties(t, executeOutputSchema, "next_steps", "pagination")
}

// TestExecuteActionSchema_XMCPHeader_AnnotatesActionParam verifies that the
// gitlab_execute_action input schema carries the SEP-2243 x-mcp-header
// annotation on its action property, and that the annotation survives JSON
// serialization of the schema — that is how MCP-aware gateways read it, so a
// non-serialized annotation would be invisible to them.
func TestExecuteActionSchema_XMCPHeader_AnnotatesActionParam(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogFindExecuteTools(server, actioncatalog.FromActionMaps(testRoutes(t)))

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	executeSchema := listedToolInputSchema(t, tools.Tools, executeActionToolName)
	properties, ok := executeSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("gitlab_execute_action schema has no properties object: %v", executeSchema)
	}
	action, ok := properties["action"].(map[string]any)
	if !ok {
		t.Fatalf("gitlab_execute_action schema has no action property: %v", properties)
	}
	// The annotation carries the suffix, not the full header: the SDK
	// prepends "Mcp-Param-" on both the sending and validating side, so
	// declaring the full name here put "Mcp-Param-Mcp-Param-Action" on the
	// wire and made the documented header a mismatch.
	if got := action["x-mcp-header"]; got != executeActionHeaderSuffix {
		t.Fatalf("action x-mcp-header = %v, want %q", got, executeActionHeaderSuffix)
	}
	// And the wire header a client must send is the prefixed form. Naming it
	// here is what makes the relationship legible: the schema value and the
	// header are not the same string, and conflating them is the bug this
	// pair of assertions exists to prevent.
	if ExecuteActionHeaderName != "Mcp-Param-"+executeActionHeaderSuffix {
		t.Errorf("wire header = %q, want the annotation prefixed with Mcp-Param-", ExecuteActionHeaderName)
	}
	if params, paramsOK := properties["params"].(map[string]any); !paramsOK {
		t.Fatalf("gitlab_execute_action schema has no params property: %v", properties)
	} else if _, annotated := params["x-mcp-header"]; annotated {
		t.Error("params must not carry an x-mcp-header annotation; only the action ID is routable")
	}
}

// TestAnnotateActionHeader_DegenerateSchemas_ReturnedUnchanged verifies the
// defensive paths of the x-mcp-header annotation helper: a nil schema, a
// schema without an action property, and one whose action property is nil are
// all returned untouched rather than panicking, so a future change to
// [ExecuteInput] degrades to an unannotated tool.
func TestAnnotateActionHeader_DegenerateSchemas_ReturnedUnchanged(t *testing.T) {
	if got := annotateActionHeader(nil); got != nil {
		t.Errorf("annotateActionHeader(nil) = %v, want nil", got)
	}

	noAction := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{"params": {Type: "object"}},
	}
	if got := annotateActionHeader(noAction); got != noAction {
		t.Errorf("annotateActionHeader(schema without action) = %v, want the same schema", got)
	}
	if _, annotated := noAction.Properties["params"].Extra[xMCPHeaderKeyword]; annotated {
		t.Error("params must not be annotated when the action property is missing")
	}

	nilAction := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{executeActionActionParam: nil},
	}
	if got := annotateActionHeader(nilAction); got != nilAction {
		t.Errorf("annotateActionHeader(schema with nil action) = %v, want the same schema", got)
	}
}

// TestAnnotateActionHeader_KeepsAnnotationsTheActionPropertyAlreadyCarries
// verifies that the x-mcp-header annotation is added to the action property's
// existing Extra map rather than replacing it.
//
// Extra marshals inline, so every keyword in it is published in tools/list.
// Allocating a fresh map whenever one is already there would silently drop
// whatever another annotation put on the same property.
func TestAnnotateActionHeader_KeepsAnnotationsTheActionPropertyAlreadyCarries(t *testing.T) {
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			executeActionActionParam: {Type: "string", Extra: map[string]any{"x-other-keyword": "kept"}},
		},
	}

	annotated := annotateActionHeader(schema)
	extra := annotated.Properties[executeActionActionParam].Extra
	if extra[xMCPHeaderKeyword] != executeActionHeaderSuffix {
		t.Errorf("action %s = %v, want %q", xMCPHeaderKeyword, extra[xMCPHeaderKeyword], executeActionHeaderSuffix)
	}
	if extra["x-other-keyword"] != "kept" {
		t.Errorf("action extra = %v, want the pre-existing keyword kept", extra)
	}
}

// longQuery returns a query of the given character length made of words that
// match nothing, which is the shape whose cost the length bound exists to
// hold down.
func longQuery(t *testing.T, length int) string {
	t.Helper()
	query := strings.TrimSpace(strings.Repeat("zqxvfh ", length/7+2))
	return query[:length]
}

// TestFind_AnOverLongQueryIsRefusedNotTruncated pins the bound on what a
// single call may cost.
//
// A find scores the query's word count against the whole catalog three times
// over, and nothing bounded the query: a request body may be 4 MiB, which is
// hundreds of thousands of words. Truncating would be worse than refusing,
// because the caller would be answered a question they did not ask and could
// not tell.
func TestFind_AnOverLongQueryIsRefusedNotTruncated(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := []struct {
		name    string
		query   string
		refused bool
	}{
		{name: "at the limit", query: longQuery(t, MaxSearchQueryLength), refused: false},
		{name: "one character over", query: longQuery(t, MaxSearchQueryLength+1), refused: true},
		{name: "far over", query: longQuery(t, MaxSearchQueryLength*40), refused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Find(t.Context(), nil, FindInput{Query: tt.query, Limit: 5})
			if err != nil {
				t.Fatalf("Find() error = %v", err)
			}
			if result == nil {
				t.Fatal("Find() result = nil")
			}
			if result.IsError != tt.refused {
				t.Fatalf("Find() IsError = %v, want %v", result.IsError, tt.refused)
			}
			if !tt.refused {
				return
			}
			text := textContent(result)
			if !strings.Contains(text, "too long") || !strings.Contains(text, strconv.Itoa(MaxSearchQueryLength)) {
				t.Errorf("refusal = %q, want it to say the query is too long and name the limit", text)
			}
			if output.Count != 0 || len(output.Results) != 0 {
				t.Errorf("Find() output = %+v, want nothing searched", output)
			}
		})
	}
}

// TestFind_TheBoundIsMeasuredOnTheQueryAsItArrived pins which string the bound
// is applied to, on both entry points.
//
// The handler trims the query before searching, and the bound used to be
// measured after that. The schema publishes the same number as maxLength,
// which a validating client or gateway applies to the raw string, so a
// 257-character query whose surrounding whitespace left 256 was refused before
// it arrived by one deployment and answered by another. Whatever the two do,
// they have to do the same thing.
func TestFind_TheBoundIsMeasuredOnTheQueryAsItArrived(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	// One character over the bound, of which the last is a space: trimming
	// would bring it back to the limit exactly.
	padded := longQuery(t, MaxSearchQueryLength) + " "

	t.Run("find", func(t *testing.T) {
		result, output, err := registry.Find(t.Context(), nil, FindInput{Query: padded, Limit: 5})
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("Find() result = %+v, want a refusal", result)
		}
		if text := textContent(result); !strings.Contains(text, "too long") {
			t.Errorf("refusal = %q, want it to say the query is too long", text)
		}
		if output.Count != 0 {
			t.Errorf("Find() output.Count = %d, want nothing searched", output.Count)
		}
	})

	t.Run("search", func(t *testing.T) {
		result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: padded})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("Search() result = %+v, want a refusal", result)
		}
		if output.Count != 0 {
			t.Errorf("Search() output.Count = %d, want nothing searched", output.Count)
		}
	})
}

// TestSearch_AnOverLongQueryIsRefused pins the same bound on the other entry
// point into the same scorer, so the cost cannot be reached around the find
// tool.
func TestSearch_AnOverLongQueryIsRefused(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: longQuery(t, MaxSearchQueryLength+1)})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Search() result = %+v, want a refusal", result)
	}
	if !strings.Contains(textContent(result), "too long") {
		t.Errorf("refusal = %q, want it to say the query is too long", textContent(result))
	}
	if output.Count != 0 {
		t.Errorf("Search() output.Count = %d, want 0", output.Count)
	}
}

// TestFindInputSchema_PublishesTheQueryLengthBound pins that a client can see
// the limit before it sends anything, rather than discovering it from a
// refusal.
func TestFindInputSchema_PublishesTheQueryLengthBound(t *testing.T) {
	schema := findInputSchema()
	if schema == nil {
		t.Fatal("findInputSchema() = nil")
	}
	query, ok := schema.Properties[findQueryParam]
	if !ok || query == nil {
		t.Fatalf("schema has no %q property: %+v", findQueryParam, schema.Properties)
	}
	if query.MaxLength == nil {
		t.Fatal("query.MaxLength = nil, want the published bound")
	}
	if *query.MaxLength != MaxSearchQueryLength {
		t.Errorf("query.MaxLength = %d, want %d", *query.MaxLength, MaxSearchQueryLength)
	}
	if !strings.Contains(query.Description, strconv.Itoa(MaxSearchQueryLength)) {
		t.Errorf("query description = %q, want it to name the limit as well", query.Description)
	}
}

// TestAnnotateQueryLength_ASchemaWithoutTheQueryPropertyIsUnchanged covers the
// degradation path: a shape change leaves the bound unpublished rather than
// panicking, and the handler refuses over-long queries either way.
func TestAnnotateQueryLength_ASchemaWithoutTheQueryPropertyIsUnchanged(t *testing.T) {
	if got := annotateQueryLength(nil); got != nil {
		t.Errorf("annotateQueryLength(nil) = %v, want nil", got)
	}

	empty := &jsonschema.Schema{Type: "object"}
	if got := annotateQueryLength(empty); got != empty {
		t.Errorf("annotateQueryLength(schema without query) = %v, want the same schema", got)
	}

	nilQuery := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{findQueryParam: nil},
	}
	if got := annotateQueryLength(nilQuery); got != nilQuery {
		t.Errorf("annotateQueryLength(schema with nil query) = %v, want the same schema", got)
	}
}

// TestFind_ObservesCancellation pins that an abandoned call stops costing.
//
// The scoring passes are the whole cost of a find and they never looked at the
// context, so a client that hung up was scored for anyway, and the action
// deadline could not end one either. The error is returned rather than an
// empty result: "no matches" is a different answer from "never finished".
func TestFind_ObservesCancellation(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result, output, err := registry.Find(ctx, nil, FindInput{Query: "project delete", Limit: 5})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Find(cancelled) error = %v, want context.Canceled", err)
	}
	if result != nil {
		t.Errorf("Find(cancelled) result = %+v, want nil", result)
	}
	if output.Count != 0 {
		t.Errorf("Find(cancelled) output.Count = %d, want 0", output.Count)
	}
}

// TestSearch_ObservesCancellation pins the same on the second entry point.
func TestSearch_ObservesCancellation(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, _, err := registry.Search(ctx, nil, SearchInput{Query: "project delete"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Search(cancelled) error = %v, want context.Canceled", err)
	}
}

// TestFind_RunsUnderTheActionDeadline pins that the deadline every catalog
// action runs under reaches this one too.
//
// gitlab_find_action is registered directly rather than through one of the
// WrapAction functions, because its work is the catalog rather than a GitLab
// call, and that left the entry point of the default surface as the one
// registered tool a caller could keep running for as long as they liked.
func TestFind_RunsUnderTheActionDeadline(t *testing.T) {
	previous := toolutil.ActionTimeout()
	t.Cleanup(func() { toolutil.SetActionTimeout(previous) })
	toolutil.SetActionTimeout(time.Nanosecond)

	registry := NewRegistry(testRoutes(t))

	// A deadline in the past is not the same as a context that reports one:
	// both `Done` and `Err` on a timer context wait for the timer to fire, and
	// on a platform whose timer resolution is coarser than a scan of this
	// registry the search finishes first. Passing a context that has already
	// expired asks the question the name asks, whether the search observes the
	// deadline it is given, and answers it the same way on every platform.
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, _, err := registry.Find(expired, nil, FindInput{Query: "project delete", Limit: 5})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Find() error = %v, want context.DeadlineExceeded", err)
	}
}

// TestWithActionDeadline_BoundsTheSearchWhenOneIsConfigured covers the other
// half, which the test above deliberately does not: that the search is given a
// deadline at all when one is configured, and is left alone when none is.
func TestWithActionDeadline_BoundsTheSearchWhenOneIsConfigured(t *testing.T) {
	previous := toolutil.ActionTimeout()
	t.Cleanup(func() { toolutil.SetActionTimeout(previous) })

	cases := []struct {
		name         string
		timeout      time.Duration
		wantDeadline bool
	}{
		{name: "configured", timeout: time.Hour, wantDeadline: true},
		{name: "disabled", timeout: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toolutil.SetActionTimeout(tc.timeout)
			bounded, cancel := toolutil.WithActionDeadline(context.Background())
			defer cancel()

			if _, ok := bounded.Deadline(); ok != tc.wantDeadline {
				t.Errorf("the search context carries a deadline: %v, want %v", ok, tc.wantDeadline)
			}
		})
	}
}

// TestScoredMatches_StopsOnCancellation covers the loop the deadline has to
// reach: the one that scores candidates, which is where a long query spends
// its time.
func TestScoredMatches_StopsOnCancellation(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := registry.scoredMatches(ctx, normalizeSearchTerms("project"), scoreEntryWithoutExplanation); !errors.Is(err, context.Canceled) {
		t.Fatalf("scoredMatches(cancelled) error = %v, want context.Canceled", err)
	}
}

// TestScoredMatches_LooksAtTheContextOncePerBatch pins the cadence of the
// cancellation check: the first candidate is looked at, and then one candidate
// every cancellationCheckInterval after it.
//
// The error alone cannot tell one cadence from another, because every cadence
// that looks at the context at all ends up returning one; how many candidates
// were scored first is what separates them. Both directions matter. Looking
// more often would put a lock on the hottest loop in the package, which the
// interval exists to avoid, and looking only once would let an abandoned
// search walk the rest of a thousand-action catalog after its client is gone.
func TestScoredMatches_LooksAtTheContextOncePerBatch(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	registry := NewRegistryFromCatalog(catalog)
	if len(registry.entries) <= cancellationCheckInterval {
		t.Fatalf("catalog carries %d actions, want more than %d so a second look falls inside the walk", len(registry.entries), cancellationCheckInterval)
	}

	// One allowance: the look at the first candidate passes, the next one
	// cancels. Nil terms make every entry a candidate.
	ctx := &countdownContext{Context: t.Context(), remaining: 1}
	scored := 0
	scorer := func(actionEntry, []searchTerm) (int, ScoringExplanation) {
		scored++
		return 0, ScoringExplanation{}
	}

	if _, scoreErr := registry.scoredMatches(ctx, nil, scorer); !errors.Is(scoreErr, context.Canceled) {
		t.Fatalf("scoredMatches(cancelled at the second look) error = %v, want context.Canceled", scoreErr)
	}
	if scored != cancellationCheckInterval {
		t.Fatalf("scored %d candidates before the second look at the context, want %d", scored, cancellationCheckInterval)
	}
}

// TestSegmentedSearchMatches_StopsOnCancellation covers the pass that
// multiplies the catalog by the query's window count, which is what makes a
// long query expensive.
func TestSegmentedSearchMatches_StopsOnCancellation(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	terms := normalizeSearchTerms("merge request approve project delete pipeline retry")
	if _, err := registry.segmentedSearchMatchesWithScorer(ctx, terms, defaultLimit, scoreEntryWithoutExplanation); !errors.Is(err, context.Canceled) {
		t.Fatalf("segmentedSearchMatchesWithScorer(cancelled) error = %v, want context.Canceled", err)
	}
}

// searchTermsFromWords builds terms the way a caller inside the package can
// hand them to a scorer: one term per word, each its own only alternative,
// without the synonym expansion normalizeSearchTerms adds. A test that needs a
// known number of terms, or terms that match nothing in the catalog, cannot get
// either from a query string.
func searchTermsFromWords(words ...string) []searchTerm {
	terms := make([]searchTerm, 0, len(words))
	for _, word := range words {
		terms = append(terms, searchTerm{Raw: word, Alternatives: []string{word}})
	}
	return terms
}

// TestSegmentedSearchMatches_ScoresEveryWindowAndTheWidestKeepsATie pins the
// two halves of the segmented walk: which windows of the query are scored, and
// which of two windows that reach the same score for one action is kept.
//
// The narrowest window is the one that recovers a short intent buried in a long
// prompt ("delete the merge request in the project we discussed"), so a walk
// that stopped one size early would silently lose exactly the queries segmented
// search exists for. The tie rule matters because each window adds its own
// width to the score: when two windows land on the same total the wider one
// matched more of what the caller actually asked for, and it is the one seen
// first, so a tie must not overwrite it.
func TestSegmentedSearchMatches_ScoresEveryWindowAndTheWidestKeepsATie(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	terms := searchTermsFromWords("alfa", "bravo", "charlie", "delta", "echo")

	var windows []string
	// The scorer pays every window's width back, so each one reaches the same
	// total once segmentedSearchMatchesWithScorer adds it again, and records
	// the width in the explanation so the kept match names its window.
	scorer := func(entry actionEntry, window []searchTerm) (int, ScoringExplanation) {
		raws := make([]string, 0, len(window))
		for _, term := range window {
			raws = append(raws, term.Raw)
		}
		windows = append(windows, strings.Join(raws, "-"))
		if entry.ID != "project.get" {
			return 0, ScoringExplanation{}
		}
		return 1000 - len(window)*segmentTermBoost, ScoringExplanation{MatchedTerms: len(window)}
	}

	matches, err := registry.segmentedSearchMatchesWithScorer(t.Context(), terms, defaultLimit, scorer)
	if err != nil {
		t.Fatalf("segmentedSearchMatchesWithScorer() error = %v", err)
	}

	seen := make(map[string]struct{}, len(windows))
	for _, window := range windows {
		seen[window] = struct{}{}
	}
	got := slices.Collect(maps.Keys(seen))
	slices.Sort(got)
	want := []string{
		"alfa-bravo-charlie",
		"alfa-bravo-charlie-delta",
		"alfa-bravo-charlie-delta-echo",
		"bravo-charlie-delta",
		"bravo-charlie-delta-echo",
		"charlie-delta-echo",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("scored windows = %v, want every window from %d terms down to %d", got, len(terms), minSegmentTerms)
	}

	if len(matches) != 1 {
		t.Fatalf("matches = %+v, want the one action the scorer scored", matches)
	}
	if matches[0].score != 1000 {
		t.Fatalf("match score = %d, want 1000 from every window", matches[0].score)
	}
	if matches[0].explanation.MatchedTerms != len(terms) {
		t.Fatalf("kept the window of %d terms, want the widest one of %d", matches[0].explanation.MatchedTerms, len(terms))
	}
}

// TestShouldRunSegmentedSearch_OnlyForQueriesLongEnoughToHaveSegments pins the
// query lengths that pay for the extra passes.
//
// Each window is a full scoring pass over the catalog, so this predicate is
// what keeps an ordinary three or four word query from costing several times
// what it should; a four-term query admitted here runs three more passes than
// it needs, on every search.
func TestShouldRunSegmentedSearch_OnlyForQueriesLongEnoughToHaveSegments(t *testing.T) {
	cases := []struct {
		name  string
		words []string
		want  bool
	}{
		{name: "two terms", words: []string{"alfa", "bravo"}},
		{name: "three terms", words: []string{"alfa", "bravo", "charlie"}},
		{name: "four terms", words: []string{"alfa", "bravo", "charlie", "delta"}},
		{name: "five terms", words: []string{"alfa", "bravo", "charlie", "delta", "echo"}, want: true},
		{name: "six terms", words: []string{"alfa", "bravo", "charlie", "delta", "echo", "foxtrot"}, want: true},
		{name: "seven terms", words: []string{"alfa", "bravo", "charlie", "delta", "echo", "foxtrot", "golf"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRunSegmentedSearch(searchTermsFromWords(tc.words...), defaultLimit); got != tc.want {
				t.Fatalf("shouldRunSegmentedSearch(%d terms) = %t, want %t", len(tc.words), got, tc.want)
			}
		})
	}
}

// countdownContext is cancelled after a fixed number of checks, so a test can
// place the cancellation between two of a search's passes.
//
// A real deadline cannot: whichever pass happens to be running when the clock
// fires is the one that stops, and which pass that is depends on the machine.
// A search runs a lexical pass, then a fuzzy pass when the lexical one found
// too little, then one pass per segment window, and each of them has to stop.
type countdownContext struct {
	context.Context
	remaining int
}

// Err reports cancellation once the allowance is spent. It is not safe for
// concurrent use, which a search does not need: the passes run in sequence.
func (c *countdownContext) Err() error {
	if c.remaining > 0 {
		c.remaining--
		return nil
	}
	return context.Canceled
}

// TestSearchMatches_EveryPassStopsOnCancellation pins that the cancellation
// reaches each pass of a search rather than only the first.
//
// The lexical pass is the one a cancelled context stops before anything else
// runs, so it is the only one a plain cancelled context can cover. The other
// two are where a long query actually spends its time.
func TestSearchMatches_EveryPassStopsOnCancellation(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := []struct {
		name string
		// query decides which passes run: one that matches nothing falls
		// through to the fuzzy pass, and one with five or more terms runs the
		// segmented pass.
		query string
		// allowance is how many context checks succeed before cancellation,
		// which places it inside a chosen pass.
		allowance int
	}{
		{name: "the lexical pass", query: "project delete", allowance: 0},
		{name: "the fuzzy pass", query: "zzzzzzzz", allowance: 1},
		// Two: the lexical pass and the fuzzy pass this query also runs, so
		// the cancellation lands on the first segment window.
		{name: "the segmented pass", query: "merge request approve project delete pipeline retry", allowance: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &countdownContext{Context: t.Context(), remaining: tt.allowance}
			if _, err := registry.searchMatches(ctx, tt.query, defaultLimit, false); !errors.Is(err, context.Canceled) {
				t.Fatalf("searchMatches(%q) error = %v, want context.Canceled", tt.query, err)
			}
		})
	}
}

// TestRegisterCatalogFindExecuteTools_FindAcceptsNaturalLanguageAndReturnsSchema
// verifies that the registered MCP tool accepts plain search phrases and returns
// the schema payload needed for the next execute call.
func TestRegisterCatalogFindExecuteTools_FindAcceptsNaturalLanguageAndReturnsSchema(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogFindExecuteTools(server, actioncatalog.FromActionMaps(testRoutes(t)))

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: findToolName,
		Arguments: map[string]any{
			"query": "please remove a project",
			"limit": 3,
		},
	})
	if err != nil {
		t.Fatalf("CallTool(gitlab_find_action) error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("CallTool(gitlab_find_action) result = %+v, want non-error", result)
	}
	if result.StructuredContent == nil {
		t.Fatal("CallTool(gitlab_find_action) StructuredContent is nil")
	}

	data := unmarshalStructuredContentMap(t, result.StructuredContent)
	results, ok := data["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("StructuredContent results = %+v, want at least one match", data["results"])
	}
	first, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("first result = %+v, want object", results[0])
	}
	if first["id"] != "project.delete" {
		t.Fatalf("first id = %v, want project.delete", first["id"])
	}
	if first["schema_uri"] != "gitlab://tools/project.delete" {
		t.Fatalf("first schema_uri = %v, want gitlab://tools/project.delete", first["schema_uri"])
	}
	if _, hasInputSchema := first["input_schema"].(map[string]any); !hasInputSchema {
		t.Fatalf("first input_schema = %+v, want schema object", first["input_schema"])
	}
	example, ok := first["example"].(map[string]any)
	if !ok || example["tool"] != executeActionToolName {
		t.Fatalf("first example = %+v, want gitlab_execute_action", first["example"])
	}
}

// TestRegisterCatalogFindExecuteTools_ExecuteOutputSchemaAcceptsActionOutput verifies that
// the protocol-level execute tool output schema remains permissive enough for
// action-dependent structured content.
func TestRegisterCatalogFindExecuteTools_ExecuteOutputSchemaAcceptsActionOutput(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogFindExecuteTools(server, actioncatalog.FromActionMaps(testRoutes(t)))

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "gitlab_execute_action",
		Arguments: map[string]any{
			"action": "project.list",
			"params": map[string]any{"owned": true},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(gitlab_execute_action) error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("CallTool(gitlab_execute_action) result = %+v, want non-error", result)
	}
	if result.StructuredContent == nil {
		t.Fatal("CallTool(gitlab_execute_action) StructuredContent is nil")
	}
	data := unmarshalStructuredContentMap(t, result.StructuredContent)
	if data["owned"] != true {
		t.Fatalf("StructuredContent = %+v, want owned=true", data)
	}
}

// TestSearch_PartialMatchLongQuery verifies the Search_PartialMatchLongQuery handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_PartialMatchLongQuery(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	// Simulate a realistic LLM query that includes incidental words ("open") that
	// do not map to any tool name but should not suppress relevant results.
	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "merge request list open", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches for partial query, want at least one merge_request result")
	}
	found := slices.ContainsFunc(output.Results, func(r SearchResult) bool {
		return strings.HasPrefix(r.ID, "merge_request.")
	})
	if !found {
		t.Fatalf("Search() results = %+v, want at least one merge_request.* result", output.Results)
	}
}

// TestSearch_NaturalLLMQueriesReturnActions verifies the Search_NaturalLLMQueriesReturnActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_NaturalLLMQueriesReturnActions(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "discover project from remote url", query: "discover project from remote url", want: "discover_project.resolve"},
		{name: "merge request list open authored by me project", query: "merge request list open authored by me project", want: "merge_request.list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: 5})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			if !slices.ContainsFunc(output.Results, func(r SearchResult) bool { return r.ID == tt.want }) {
				t.Fatalf("Search(%q) results = %+v, want %s", tt.query, output.Results, tt.want)
			}
		})
	}
}

// TestSearch_MultiIntentLongQuery_ReturnsSegmentMatches verifies that a long
// query containing multiple intents is segmented into actionable matches.
func TestSearch_MultiIntentLongQuery_ReturnsSegmentMatches(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{
		Query: "discover project from remote url merge request list current user open authored",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	for _, want := range []string{"discover_project.resolve", "merge_request.list"} {
		t.Run(want, func(t *testing.T) {
			if !slices.ContainsFunc(output.Results, func(r SearchResult) bool { return r.ID == want }) {
				t.Fatalf("Search() results = %+v, want %s", output.Results, want)
			}
		})
	}
}

// TestSearch_MultiIntentLongQueryOnMetaCatalog_ReturnsSegmentMatches verifies
// the observed long dynamic query against the real captured meta catalog.
//
// The full catalog already has global matches for the merge-request terms, so
// this test protects the segment merge path that keeps the standalone project
// discovery action in the first page of results.
func TestSearch_MultiIntentLongQueryOnMetaCatalog_ReturnsSegmentMatches(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{
		Query: "discover project from remote url merge request list current user open authored",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	assertSearchResultsContain(t, output.Results, "discover_project.resolve", "merge_request.list")
}

// TestSearch_QueryShapeMatrix_ReturnsExpectedActions verifies the Search_QueryShapeMatrix_ReturnsExpectedActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_QueryShapeMatrix_ReturnsExpectedActions(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []struct {
		name  string
		query string
		limit int
		want  []string
	}{
		{name: "short canonical action", query: "project list", want: []string{"project.list"}},
		{name: "short synonym intent", query: "project info", want: []string{"project.get"}},
		{name: "short alias intent", query: "deploy key", want: []string{"access.deploy_key_add"}},
		{name: "typo phrase", query: "merje requesy list", want: []string{"merge_request.list"}},
		{name: "long polite metadata phrase", query: "please find project metadata details using id", want: []string{"project.get"}},
		{name: "long repository content phrase", query: "download repository file content from project ref", want: []string{"repository.file_get"}},
		{name: "observed authored current user phrase", query: "current user open authored merge request list", want: []string{"merge_request.list"}},
		{name: "standalone discovery without verb", query: "project remote url lookup", want: []string{"discover_project.resolve"}},
		{name: "pipeline jobs alias", query: "pipeline jobs list", want: []string{"job.list"}},
		{name: "ci secret create", query: "create ci secret variable", want: []string{"ci_variable.create"}},
		{name: "package remove intent", query: "remove package", want: []string{"package.delete"}},
		{name: "project status checks alias", query: "project status checks list", want: []string{"external_status_check.list_project"}},
		{name: "group audit events alias", query: "group audit events", want: []string{"audit_event.list_group"}},
		{name: "mixed webhook and repository", query: "webhook create repository file read", limit: 10, want: []string{"project.hook_add", "repository.file_get"}},
		{name: "mixed deploy key and package", query: "deploy key create package delete", limit: 10, want: []string{"access.deploy_key_add", "package.delete"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: tt.limit})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultsContain(t, output.Results, tt.want...)
		})
	}
}

// TestSearch_FuzzyRecoveryQueriesReturnExpectedCandidates verifies the Search_FuzzyRecoveryQueriesReturnExpectedCandidates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_FuzzyRecoveryQueriesReturnExpectedCandidates(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := []struct {
		query      string
		want       string
		wantPrefix string
	}{
		{query: "merje request", wantPrefix: "merge_request."},
		{query: "merge requesy", wantPrefix: "merge_request."},
		{query: "pipline retry", want: "pipeline.retry"},
		{query: "brnch protect", want: "branch.protect"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: 10, Explain: true})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			if tt.want != "" {
				assertSearchResultsContain(t, output.Results, tt.want)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(output.Results[0].ID, tt.wantPrefix) {
				t.Fatalf("Search(%q) top result = %+v, want prefix %s", tt.query, output.Results[0], tt.wantPrefix)
			}
		})
	}
}

// TestSearch_FuzzyRecoveryIncludesReasonMetadata verifies fuzzy explanations
// include fuzzy=true and edit distance metadata when fuzzy fallback supplies a result.
func TestSearch_FuzzyRecoveryIncludesReasonMetadata(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "merje requesy", Limit: 10, Explain: true})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches")
	}
	for _, searchResult := range output.Results {
		if searchResult.Explanation == nil {
			continue
		}
		for _, reason := range searchResult.Explanation.Reasons {
			if reason.Fuzzy && reason.Distance > 0 {
				return
			}
		}
	}
	t.Fatalf("Search() results = %+v, want at least one fuzzy reason with edit distance", output.Results)
}

// TestSearch_FuzzyRecoveryDoesNotElevateWeakDestructiveTypo verifies typo-only
// destructive-looking queries cannot push a destructive action above safer matches.
func TestSearch_FuzzyRecoveryDoesNotElevateWeakDestructiveTypo(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "projec list delet", Limit: 5, Explain: true})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches")
	}
	if output.Results[0].ID == "project.delete" {
		t.Fatalf("Search() top result = %+v, want non-destructive candidate above project.delete", output.Results[0])
	}
}

// TestSearch_DomainVerbParameterIntentSignals_ReturnExpectedActions verifies
// semantic intent signals for confusing cross-domain GitLab task phrasing.
func TestSearch_DomainVerbParameterIntentSignals_ReturnExpectedActions(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := []struct {
		query string
		want  string
	}{
		{query: "release link create", want: "release.link_create"},
		{query: "package list project", want: "package.list"},
		{query: "pipeline jobs", want: "job.list"},
		{query: "project member remove", want: "project.member_delete"},
		{query: "group variable create", want: "ci_variable.group_create"},
		{query: "repository file read", want: "repository.file_get"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: 10, Explain: true})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultsContain(t, output.Results, tt.want)
		})
	}
}

// TestSearch_ProviderConfusionQueries_ReturnExpectedActions verifies the Search_ProviderConfusionQueries_ReturnExpectedActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_ProviderConfusionQueries_ReturnExpectedActions(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := []struct {
		name  string
		query string
		limit int
		want  []string
	}{
		{name: "single artifact by numeric job", query: "download coverage/report.xml single artifact file from numeric job id", want: []string{"job.download_single_artifact"}},
		{name: "current instance settings", query: "read current instance settings before creating broadcast message", want: []string{"admin.settings_get"}},
		{name: "release cleanup first steps", query: "verify tag release asset links before deleting release and tag", limit: 8, want: []string{"tag.get", "release.get", "release.link_list"}},
		{name: "generic package list", query: "list package registry packages", want: []string{"package.list"}},
		{name: "runner removal by id", query: "remove runner by numeric runner_id", want: []string{"runner.remove"}},
		{name: "issue time tracking sequence", query: "issue time tracking set estimate add spent time reset spent time reset estimate", limit: 8, want: []string{"issue.time_estimate_set", "issue.spent_time_add", "issue.spent_time_reset", "issue.time_estimate_reset"}},
		{name: "deploy token inventory", query: "list project deploy tokens credentials not deploy keys", limit: 8, want: []string{"access.deploy_token_list_project"}},
		{name: "project access token creation", query: "project access token create eval-token read_api expires_at 2026-12-31 for project my-org/tools/gitlab-mcp-server", limit: 8, want: []string{"access.token_project_create"}},
		// environment.protected_get is Premium (protected environments), so it is
		// absent from this Free realCatalogRegistry; only the Free deployment
		// actions are expected here.
		{name: "protected environment deployment approval", query: "protected environment deployment_list deployment approve_or_reject", limit: 12, want: []string{"environment.deployment_list", "environment.deployment_approve_or_reject"}},
		{name: "feature flag user list lifecycle", query: "feature flag user list get user_list_iid update delete", limit: 8, want: []string{"feature_flags.ff_user_list_get", "feature_flags.ff_user_list_update", "feature_flags.ff_user_list_delete"}},
		{name: "issue note lifecycle", query: "issue note get by note_id update delete comment", limit: 8, want: []string{"issue.note_get", "issue.note_update", "issue.note_delete"}},
		{name: "discover project by path or url", query: "project find by path or url", limit: 8, want: []string{"discover_project.resolve"}},
		{name: "project get by path", query: "project show by path my-org/tools/gitlab-mcp-server", limit: 8, want: []string{"project.get"}},
		{name: "search projects intent", query: "project list search gitlab-mcp-server", limit: 8, want: []string{"search.projects"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: tt.limit})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultsContain(t, output.Results, tt.want...)
		})
	}
}

// TestSearch_ProviderConfusionQueries_PrioritizeExactTopResult verifies that a
// natural-language project-search query returns the canonical search.projects action
// as the top result against the real catalog registry, guarding against provider/domain
// confusion that would rank a less-specific action higher.
func TestSearch_ProviderConfusionQueries_PrioritizeExactTopResult(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := []struct {
		name  string
		query string
		limit int
		want  string
	}{
		{name: "search projects top result", query: "project list search gitlab-mcp-server", limit: 8, want: "search.projects"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: tt.limit})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			if len(output.Results) == 0 || output.Results[0].ID != tt.want {
				t.Fatalf("Search(%q) top result = %+v, want %s", tt.query, output.Results, tt.want)
			}
		})
	}
}

// TestSearch_ProviderConfusionQueries_PrioritizeExactTopResult_EnterpriseCatalog verifies
// that the same project-search query still ranks search.projects as the top result when the
// registry is built from the larger enterprise catalog, ensuring the extra enterprise actions
// do not displace the exact match.
func TestSearch_ProviderConfusionQueries_PrioritizeExactTopResult_EnterpriseCatalog(t *testing.T) {
	enterpriseCatalog := mustCachedCatalog(t, true)
	registry := NewRegistryFromCatalog(enterpriseCatalog)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project list search gitlab-mcp-server", Limit: 20})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if len(output.Results) == 0 || output.Results[0].ID != "search.projects" {
		t.Fatalf("Search top result = %+v, want search.projects", output.Results)
	}
}

// TestSearch_CrossBackendTermsStayGitLabOnly verifies non-GitLab vocabulary is
// normalized to current GitLab capabilities without exposing foreign action IDs.
func TestSearch_CrossBackendTermsStayGitLabOnly(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "github pull request", query: "github pr list open", want: "merge_request.list"},
		{name: "jira ticket", query: "jira ticket list open", want: "issue.list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: 10})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultsContain(t, output.Results, tt.want)
			for _, searchResult := range output.Results {
				if strings.HasPrefix(searchResult.ID, "github.") || strings.HasPrefix(searchResult.ID, "jira.") {
					t.Fatalf("Search() results = %+v, want GitLab-only action IDs", output.Results)
				}
			}
		})
	}
}

// TestSearch_MixedQueriesWithTightLimit_ReturnExactActionSet verifies that mixed
// intent queries return the expected action set even when the limit is tight.
func TestSearch_MixedQueriesWithTightLimit_ReturnExactActionSet(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "discover and merge request lookup",
			query: "discover project from remote url merge request list current user open authored",
			want:  []string{"merge_request.list", "discover_project.resolve"},
		},
		{
			name:  "webhook creation and repository read",
			query: "webhook create repository file read",
			want:  []string{"repository.file_get", "project.hook_add"},
		},
		{
			name:  "release link creation and package deletion",
			query: "release link create package remove",
			want:  []string{"release.link_create", "package.delete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: len(tt.want)})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultIDsEqual(t, output.Results, tt.want...)
		})
	}
}

// TestSearch_TypoQueryReturnsRelevantActions verifies the Search_TypoQueryReturnsRelevantActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearch_TypoQueryReturnsRelevantActions(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "merje requesy list", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches for typo query, want at least one merge_request result")
	}
	if !slices.ContainsFunc(output.Results, func(r SearchResult) bool {
		return strings.HasPrefix(r.ID, "merge_request.")
	}) {
		t.Fatalf("Search() results = %+v, want at least one merge_request.* result", output.Results)
	}
}

// TestSearch_TypoQueryReturnsResultsOnMetaCatalog verifies that fuzzy matching
// works against the real captured meta-tool catalog, not only test fixtures.
func TestSearch_TypoQueryReturnsResultsOnMetaCatalog(t *testing.T) {
	registry := realCatalogRegistry(t)
	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "merje requesy", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches for typo query on meta catalog")
	}
}

// actionDescriptionByID supports action description by ID assertions in dynamic tests.
func actionDescriptionByID(t *testing.T, output DescribeOutput, id string) ActionDescription {
	t.Helper()
	for _, action := range output.Actions {
		if action.ID == id {
			return action
		}
	}
	t.Fatalf("DescribeOutput missing action %q: %+v", id, output.Actions)
	return ActionDescription{}
}

// cachedBaseCatalog and cachedEnterpriseCatalog memoize the two standard
// full catalogs (base and enterprise, standalone actions included) that the
// search tests exercise read-only: building one resolves ~850+ action
// schemas, and dozens of tests used to rebuild it each. Tests that mutate a
// catalog keep building their own.
var cachedBaseCatalog = sync.OnceValues(func() (*actioncatalog.Catalog, error) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("BuildActionCatalog() error = %w", err)
	}
	return AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
})

var cachedEnterpriseCatalog = sync.OnceValues(func() (*actioncatalog.Catalog, error) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("BuildActionCatalog(enterprise) error = %w", err)
	}
	return AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
})

// mustCachedCatalog unwraps one of the cached catalogs for a test.
func mustCachedCatalog(t *testing.T, enterprise bool) *actioncatalog.Catalog {
	t.Helper()
	get := cachedBaseCatalog
	if enterprise {
		get = cachedEnterpriseCatalog
	}
	catalog, err := get()
	if err != nil {
		t.Fatalf("cached catalog: %v", err)
	}
	return catalog
}

var cachedRealCatalogRegistry = sync.OnceValues(func() (*Registry, error) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("BuildActionCatalog() error = %w", err)
	}
	catalog, err = AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("AddStandaloneCatalog() error = %w", err)
	}
	return NewRegistryFromCatalog(catalog), nil
})

// realCatalogRegistry supports real catalog registry assertions in dynamic tests.
func realCatalogRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := cachedRealCatalogRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// gitLabDotComEnterpriseRegistry supports GitLab dot com enterprise registry assertions in dynamic tests.
func gitLabDotComEnterpriseRegistry(t *testing.T) (*actioncatalog.Catalog, *Registry) {
	t.Helper()
	client, err := gitlabclient.NewClientWithToken("https://gitlab.com", "test-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken(gitlab.com) error = %v", err)
	}
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog(gitlab.com enterprise) error = %v", err)
	}
	catalog, err = AddStandaloneCatalog(catalog, client, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog(gitlab.com enterprise) error = %v", err)
	}
	return catalog, NewRegistryFromCatalog(catalog)
}

// assertSearchResultsContain checks search results contain invariants for tests.
func assertSearchResultsContain(t *testing.T, results []SearchResult, want ...string) {
	t.Helper()
	for _, actionID := range want {
		if slices.ContainsFunc(results, func(result SearchResult) bool { return result.ID == actionID }) {
			continue
		}
		t.Fatalf("Search() results = %+v, want %s", results, actionID)
	}
}

// assertSearchResultIDsEqual checks search result IDs equal invariants for tests.
func assertSearchResultIDsEqual(t *testing.T, results []SearchResult, want ...string) {
	t.Helper()
	if len(results) != len(want) {
		t.Fatalf("Search() results = %+v, want exactly %v", results, want)
	}
	gotIDs := make([]string, 0, len(results))
	for _, result := range results {
		gotIDs = append(gotIDs, result.ID)
	}
	slices.Sort(gotIDs)
	wantIDs := append([]string(nil), want...)
	slices.Sort(wantIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("Search() result IDs = %v, want exactly %v", gotIDs, wantIDs)
	}
}

// fullScanScoredMatches performs a scorer-based full scan and keeps only
// positive-score entries for comparison against indexed candidate results.
func fullScanScoredMatches(entries []actionEntry, terms []searchTerm, scorer searchScorer) []scoredActionEntry {
	matches := make([]scoredActionEntry, 0)
	for _, entry := range entries {
		score, explanation := scorer(entry, terms)
		if score > 0 {
			matches = append(matches, scoredActionEntry{entry: entry, score: score, explanation: explanation})
		}
	}
	return matches
}

// scoredActionIDs extracts canonical action IDs from scored search matches.
func scoredActionIDs(matches []scoredActionEntry) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.entry.ID)
	}
	return ids
}

// assertSchemaHasProperties checks schema has properties invariants for tests.
func assertSchemaHasProperties(t *testing.T, schema map[string]any, names ...string) {
	t.Helper()
	properties := schemaProperties(schema)
	for _, name := range names {
		if _, ok := properties[name]; !ok {
			t.Fatalf("schema properties = %v, want %q", sortedPropertyNames(properties), name)
		}
	}
}

// assertSchemaPropertyNamesEqual checks schema property names equal invariants for tests.
func assertSchemaPropertyNamesEqual(t *testing.T, actionID string, gotSchema, wantSchema map[string]any) {
	t.Helper()
	gotNames := sortedPropertyNames(schemaProperties(gotSchema))
	wantNames := sortedPropertyNames(schemaProperties(wantSchema))
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("%s schema properties = %v, want %v", actionID, gotNames, wantNames)
	}
}

// schemaProperties extracts schema properties details for schema assertions.
func schemaProperties(schema map[string]any) map[string]any {
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		return map[string]any{}
	}
	return properties
}

func schemaPropertyDescription(schema map[string]any, name string) string {
	property, _ := schemaProperties(schema)[name].(map[string]any)
	description, _ := property["description"].(string)
	return description
}

// schemaRequired extracts schema required details for schema assertions.
func schemaRequired(schema map[string]any) []string {
	var required []string
	switch values := schema["required"].(type) {
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok {
				required = append(required, name)
			}
		}
	case []string:
		required = append(required, values...)
	}
	slices.Sort(required)
	return required
}

// listedToolInputSchema supports listed tool input schema assertions in dynamic tests.
func listedToolInputSchema(t *testing.T, tools []*mcp.Tool, name string) map[string]any {
	t.Helper()
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		data, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s input schema: %v", name, err)
		}
		var schema map[string]any
		if unmarshalErr := json.Unmarshal(data, &schema); unmarshalErr != nil {
			t.Fatalf("unmarshal %s input schema: %v", name, unmarshalErr)
		}
		return schema
	}
	t.Fatalf("tool %s not listed", name)
	return nil
}

// listedTool locates a listed MCP tool by name for metadata assertions.
func listedTool(t *testing.T, tools []*mcp.Tool, name string) *mcp.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %s not listed", name)
	return nil
}

// listedToolOutputSchema supports listed tool output schema assertions in dynamic tests.
func listedToolOutputSchema(t *testing.T, tools []*mcp.Tool, name string) map[string]any {
	t.Helper()
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		if tool.OutputSchema == nil {
			t.Fatalf("tool %s output schema is nil", name)
		}
		data, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshal %s output schema: %v", name, err)
		}
		var schema map[string]any
		if unmarshalErr := json.Unmarshal(data, &schema); unmarshalErr != nil {
			t.Fatalf("unmarshal %s output schema: %v", name, unmarshalErr)
		}
		return schema
	}
	t.Fatalf("tool %s not listed", name)
	return nil
}

// unmarshalStructuredContentMap decodes protocol structured content for dynamic tests.
func unmarshalStructuredContentMap(t *testing.T, structured any) map[string]any {
	t.Helper()
	data, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out map[string]any
	if unmarshalErr := json.Unmarshal(data, &out); unmarshalErr != nil {
		t.Fatalf("unmarshal structured content: %v", unmarshalErr)
	}
	return out
}

// sortedPropertyNames sorts ed property names fixtures into deterministic order.
func sortedPropertyNames(properties map[string]any) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// testRoutes supports test routes assertions in dynamic tests.
func testRoutes(t *testing.T) map[string]toolutil.ActionMap {
	t.Helper()
	return cloneTestRoutes(testRouteFixtures)
}

func cloneTestRoutes(routes map[string]toolutil.ActionMap) map[string]toolutil.ActionMap {
	cloned := make(map[string]toolutil.ActionMap, len(routes))
	for toolName, actions := range routes {
		cloned[toolName] = maps.Clone(actions)
	}
	return cloned
}

var testRouteFixtures = map[string]toolutil.ActionMap{
	"gitlab_project": {
		"get": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"project_id": params["project_id"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
				},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
				},
			},
		},
		"hook_list": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"hooks": true}, nil
			},
		},
		"hook_add": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"url": params["url"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "url"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"url":        map[string]any{"type": "string"},
				},
			},
		},
		"member_edit": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"member": "edited", "access_level": params["access_level"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "user_id", "access_level"},
				"properties": map[string]any{
					"project_id":   map[string]any{"type": "integer"},
					"user_id":      map[string]any{"type": "integer"},
					"access_level": map[string]any{"type": "integer"},
				},
			},
		},
		"member_add": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"member": "added", "access_level": params["access_level"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "user_id", "access_level"},
				"properties": map[string]any{
					"project_id":   map[string]any{"type": "integer"},
					"user_id":      map[string]any{"type": "integer"},
					"access_level": map[string]any{"type": "integer"},
				},
			},
		},
		"member_delete": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"member": "deleted"}, nil
			},
		},
		"delete": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"deleted": true, "confirm": params["confirm"]}, nil
			},
			Destructive: true,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
				},
			},
		},
		"list": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"owned": params["owned"]}, nil
			},
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owned": map[string]any{"type": "boolean"},
				},
			},
		},
	},
	"gitlab_merge_request": {
		"list": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"state": params["state"], "author_username": params["author_username"]}, nil
			},
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":      map[string]any{"type": "integer"},
					"state":           map[string]any{"type": "string"},
					"author_username": map[string]any{"type": "string"},
				},
			},
		},
		"approve": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"approved": true}, nil
			},
		},
		"merge": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"merged": true}, nil
			},
		},
		"time_estimate_set": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"time": "set"}, nil
			},
		},
		"spent_time_add": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"spent": "added", "summary": params["summary"], "note": params["note"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "merge_request_iid", "duration"},
				"properties": map[string]any{
					"project_id":        map[string]any{"type": "integer"},
					"merge_request_iid": map[string]any{"type": "integer"},
					"duration":          map[string]any{"type": "string"},
					"summary":           map[string]any{"type": "string"},
				},
			},
		},
		"emoji_mr_create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return maps.Clone(params), nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "merge_request_iid", "name"},
				"properties": map[string]any{
					"project_id":        map[string]any{"type": "integer"},
					"merge_request_iid": map[string]any{"type": "integer"},
					"name":              map[string]any{"type": "string"},
				},
			},
		},
		"emoji_mr_delete": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"award_id": params["award_id"]}, nil
			},
			Destructive: true,
		},
	},
	"gitlab_issue": {
		"note_list": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"notes": true}, nil
			},
		},
		"spent_time_add": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return maps.Clone(params), nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "issue_iid", "duration"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"issue_iid":  map[string]any{"type": "integer"},
					"duration":   map[string]any{"type": "string"},
					"summary":    map[string]any{"type": "string"},
				},
			},
		},
		"link_create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"target_issue_iid": params["target_issue_iid"], "target_project_id": params["target_project_id"], "link_type": params["link_type"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "issue_iid", "target_project_id", "target_issue_iid"},
				"properties": map[string]any{
					"project_id":        map[string]any{"type": "integer"},
					"issue_iid":         map[string]any{"type": "integer"},
					"target_project_id": map[string]any{"type": "integer"},
					"target_issue_iid":  map[string]any{"type": "integer"},
					"link_type":         map[string]any{"type": "string"},
				},
			},
		},
		"update": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"state_event": params["state_event"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "issue_iid", "state_event"},
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "integer"},
					"issue_iid":   map[string]any{"type": "integer"},
					"state_event": map[string]any{"type": "string"},
				},
			},
		},
	},
	"gitlab_ci_variable": {
		"create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"key": params["key"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "key", "value"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"key":        map[string]any{"type": "string"},
					"value":      map[string]any{"type": "string"},
				},
			},
		},
		"group_create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"key": params["key"]}, nil
			},
		},
	},
	"gitlab_branch": {
		"get_protected": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"branch_name": params["branch_name"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "branch_name"},
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "integer"},
					"branch_name": map[string]any{"type": "string"},
				},
			},
		},
		"protect": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{
					"push_access_level":  params["push_access_level"],
					"merge_access_level": params["merge_access_level"],
				}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "branch_name"},
				"properties": map[string]any{
					"project_id":         map[string]any{"type": "integer"},
					"branch_name":        map[string]any{"type": "string"},
					"push_access_level":  map[string]any{"type": "integer"},
					"merge_access_level": map[string]any{"type": "integer"},
					"allow_force_push":   map[string]any{"type": "boolean"},
				},
			},
		},
		"update_protected": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"allow_force_push": params["allow_force_push"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "branch_name"},
				"properties": map[string]any{
					"project_id":       map[string]any{"type": "integer"},
					"branch_name":      map[string]any{"type": "string"},
					"allow_force_push": map[string]any{"type": "boolean"},
				},
			},
		},
	},
	"gitlab_pipeline": {
		"schedule_create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return params, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "description", "ref", "cron"},
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "integer"},
					"description": map[string]any{"type": "string"},
					"ref":         map[string]any{"type": "string"},
					"cron":        map[string]any{"type": "string"},
					"active":      map[string]any{"type": "boolean"},
				},
			},
		},
		"schedule_create_variable": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return params, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "schedule_id", "key", "value"},
				"properties": map[string]any{
					"project_id":    map[string]any{"type": "integer"},
					"schedule_id":   map[string]any{"type": "integer"},
					"key":           map[string]any{"type": "string"},
					"value":         map[string]any{"type": "string"},
					"variable_type": map[string]any{"type": "string"},
				},
			},
		},
		"schedule_edit_variable": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return params, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "schedule_id", "key", "value"},
				"properties": map[string]any{
					"project_id":    map[string]any{"type": "integer"},
					"schedule_id":   map[string]any{"type": "integer"},
					"key":           map[string]any{"type": "string"},
					"value":         map[string]any{"type": "string"},
					"variable_type": map[string]any{"type": "string"},
				},
			},
		},
	},
	"gitlab_repository": {
		"file_get": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"action": "repository.file_get", "file_path": params["file_path"], "ref": params["ref"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "file_path", "ref"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"file_path":  map[string]any{"type": "string"},
					"ref":        map[string]any{"type": "string"},
				},
			},
		},
	},
	"gitlab_access": {
		"deploy_key_add": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"deploy_key": "added"}, nil
			},
		},
		"deploy_key_delete": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"deploy_key_id": params["deploy_key_id"], "deleted": true}, nil
			},
			Destructive: true,
		},
		"deploy_key_get": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"deploy_key_id": params["deploy_key_id"]}, nil
			},
		},
		"deploy_key_update": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"deploy_key_id": params["deploy_key_id"], "updated": true}, nil
			},
		},
		"token_project_create": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"token": "created"}, nil
			},
		},
		"deploy_token_create_project": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"deploy_token": "created"}, nil
			},
		},
	},
	"gitlab_runner": {
		"update": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"paused": params["paused"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"runner_id", "paused"},
				"properties": map[string]any{
					"runner_id": map[string]any{"type": "integer"},
					"paused":    map[string]any{"type": "boolean"},
				},
			},
		},
	},
	"gitlab_group": {
		"group_label_update": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return params, nil
			},
		},
		"ldap_link_delete_for_provider": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"deleted": true}, nil
			},
		},
	},
	"gitlab_storage_move": {
		"schedule_project": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"destination_storage_name": params["destination_storage_name"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id"},
				"properties": map[string]any{
					"project_id":               map[string]any{"type": "integer"},
					"destination_storage_name": map[string]any{"type": "string"},
				},
			},
		},
	},
	"gitlab_mr_review": {
		"changes_get": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"changes": true}, nil
			},
		},
		"draft_note_publish_all": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"published": true}, nil
			},
		},
	},
	"gitlab_external_status_check": {
		"list_project": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"checks": true}, nil
			},
		},
	},
	"gitlab_package": {
		"list": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"packages": true}, nil
			},
		},
		"file_list": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"files": true}, nil
			},
		},
		"delete": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"deleted": true}, nil
			},
			Destructive: true,
		},
	},
	"gitlab_audit_event": {
		"list_group": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"events": true}, nil
			},
		},
	},
	"gitlab_job": {
		"list": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"jobs": true, "scope": params["scope"]}, nil
			},
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "integer"},
					"pipeline_id": map[string]any{"type": "integer"},
					"scope":       map[string]any{"type": "string"},
				},
			},
		},
		"token_scope_list_inbound": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"allowlist": true}, nil
			},
		},
	},
	"gitlab_release": {
		"list": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"releases": true}, nil
			},
		},
		"link_create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"link": "created", "tag_name": params["tag_name"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "tag_name", "name", "url"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"tag_name":   map[string]any{"type": "string"},
					"name":       map[string]any{"type": "string"},
					"url":        map[string]any{"type": "string"},
				},
			},
		},
	},
	"gitlab_feature_flags": {
		"feature_flag_create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"version": params["version"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "name", "version"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"name":       map[string]any{"type": "string"},
					"version":    map[string]any{"type": "string"},
				},
			},
		},
		"ff_user_list_list": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return params, nil
			},
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"page":       map[string]any{"type": "integer"},
					"per_page":   map[string]any{"type": "integer"},
				},
			},
		},
	},
	"gitlab_snippet": {
		"project_create": {
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"files": params["files"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"project_id", "title"},
				"properties": map[string]any{
					"project_id": map[string]any{"type": "integer"},
					"title":      map[string]any{"type": "string"},
					"files": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"file_path": map[string]any{"type": "string"},
								"content":   map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	},
}

// textContent extracts text content from MCP result content for assertions.
func textContent(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil {
		return ""
	}
	return text.Text
}

// TestRegistry_DefensiveBranches covers small validation and fallback branches
// in the dynamic registry dispatcher. These scenarios matter because the catalog
// action surface should return helpful tool errors for malformed calls instead
// of leaking empty or ambiguous execution attempts. The cases preserve coverage
// migrated from the former register_coverage_test.go file.
func TestRegistry_DefensiveBranches(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	t.Run("describe requires action", func(t *testing.T) {
		assertDescribeRequiresAction(t, registry)
	})

	t.Run("execute requires action", func(t *testing.T) {
		assertExecuteToolError(t, registry, ExecuteInput{}, false)
	})

	t.Run("execute unknown action without suggestions", func(t *testing.T) {
		assertExecuteToolError(t, registry, ExecuteInput{Action: "zzzz"}, true)
	})

	t.Run("execute initializes nil params", func(t *testing.T) {
		assertExecuteInitializesNilParams(t, registry)
	})
}

// TestRegistry_AllActionsReadOnly verifies AllActionsReadOnly's three
// branches: a nil receiver (used defensively wherever a registry might not
// have been built yet) counts as read-only since it can dispatch nothing;
// an empty registry counts as read-only for the same reason; and a
// registry is read-only only when every entry is, so one mutating entry
// among otherwise-read-only ones flips the result to false. This value
// feeds the gitlab_execute_action tool's ReadOnlyHint/DestructiveHint
// annotations (addExecuteActionTool), so a wrong answer here would
// mislabel the tool's safety annotations to MCP clients.
func TestRegistry_AllActionsReadOnly(t *testing.T) {
	t.Run("nil registry", func(t *testing.T) {
		var registry *Registry
		if !registry.AllActionsReadOnly() {
			t.Fatal("AllActionsReadOnly() on nil registry = false, want true")
		}
	})

	t.Run("empty registry", func(t *testing.T) {
		registry := &Registry{}
		if !registry.AllActionsReadOnly() {
			t.Fatal("AllActionsReadOnly() on empty registry = false, want true")
		}
	})

	t.Run("all entries read-only", func(t *testing.T) {
		registry := &Registry{entries: []actionEntry{{ID: "a.get", ReadOnly: true}, {ID: "a.list", ReadOnly: true}}}
		if !registry.AllActionsReadOnly() {
			t.Fatal("AllActionsReadOnly() with all read-only entries = false, want true")
		}
	})

	t.Run("one mutating entry flips result to false", func(t *testing.T) {
		registry := &Registry{entries: []actionEntry{{ID: "a.get", ReadOnly: true}, {ID: "a.delete", ReadOnly: false}}}
		if registry.AllActionsReadOnly() {
			t.Fatal("AllActionsReadOnly() with a mutating entry = true, want false")
		}
	})
}

// TestRegisterCatalogFindExecuteTools_AnnotatesExecuteFromWhatItCanReach
// verifies that the execute tool's annotations follow the catalog it was
// registered with: read-only and non-destructive when every action it can
// dispatch is a read, and the reverse when one of them can mutate.
//
// This is what keeps a client that prunes by ReadOnlyHint from dropping the
// only way to run the reads a narrowed credential is still entitled to, so the
// annotation has to be derived rather than fixed.
func TestRegisterCatalogFindExecuteTools_AnnotatesExecuteFromWhatItCanReach(t *testing.T) {
	cases := []struct {
		name          string
		updateIsARead bool
	}{
		{name: "every action is a read", updateIsARead: true},
		{name: "one action can mutate", updateIsARead: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
			noopRoute := toolutil.ActionRoute{Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"ok": true}, nil
			}}
			group.SetAction(actioncatalog.Action{Name: "get", ReadOnly: true, Route: noopRoute})
			group.SetAction(actioncatalog.Action{Name: "update", ReadOnly: tc.updateIsARead, Route: noopRoute})
			catalog := actioncatalog.NewCatalog()
			if err := catalog.AddGroup(group); err != nil {
				t.Fatalf("AddGroup() error = %v", err)
			}

			server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
			RegisterCatalogFindExecuteTools(server, catalog)
			executeTool := listedTool(t, listDynamicTools(t, server), executeActionToolName)

			if executeTool.Annotations == nil {
				t.Fatalf("gitlab_execute_action annotations = nil, want derived annotations")
			}
			if executeTool.Annotations.ReadOnlyHint != tc.updateIsARead {
				t.Errorf("gitlab_execute_action ReadOnlyHint = %t, want %t", executeTool.Annotations.ReadOnlyHint, tc.updateIsARead)
			}
			if executeTool.Annotations.DestructiveHint == nil {
				t.Fatalf("gitlab_execute_action DestructiveHint = nil, want the negation of the read-only hint")
			}
			if *executeTool.Annotations.DestructiveHint == tc.updateIsARead {
				t.Errorf("gitlab_execute_action DestructiveHint = %t, want %t", *executeTool.Annotations.DestructiveHint, !tc.updateIsARead)
			}
		})
	}
}

// listDynamicTools connects an in-memory client to server and returns what
// tools/list answers, which is the surface a real client sees.
func listDynamicTools(t *testing.T, server *mcp.Server) []*mcp.Tool {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	return tools.Tools
}

func assertDescribeRequiresAction(t *testing.T, registry *Registry) {
	t.Helper()
	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Describe() result = %+v, want tool error", result)
	}
	if output.Count != 0 || len(output.Actions) != 0 {
		t.Fatalf("Describe() output = %+v, want empty output", output)
	}
}

func assertExecuteToolError(t *testing.T, registry *Registry, input ExecuteInput, rejectSuggestions bool) {
	t.Helper()
	result, output, err := registry.Execute(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	if rejectSuggestions && strings.Contains(textContent(result), "Did you mean") {
		t.Fatalf("Execute() error text = %q, want no suggestions", textContent(result))
	}
}

func assertExecuteInitializesNilParams(t *testing.T, registry *Registry) {
	t.Helper()
	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.hook_list"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["hooks"] != true {
		t.Fatalf("Execute() output = %+v, want hooks=true", data)
	}
}

// TestRegistry_HelperCoverage validates deterministic helper behavior used by
// search ranking, examples, confirmations, and Markdown formatting. The cases
// target defensive branches that are easy to regress while refactoring the low
// token dynamic action surface.
// TestRegistry_ActionTagHints verifies the tag derivation helpers: schema
// property names contribute their hint words, and protected-environment
// and member-role actions carry their domain phrases.
func TestRegistry_ActionTagHints(t *testing.T) {
	t.Run("action tags include schema property hints", func(t *testing.T) {
		schema := map[string]any{"properties": map[string]any{
			"state_event": map[string]any{},
			"ref":         map[string]any{},
			"file_path":   map[string]any{},
			"url":         map[string]any{},
		}}
		got := actionTags("repository.file_create", "repository", "file_create", schema)
		for _, want := range []string{"repository file", "branch", "url", "close"} {
			t.Run(want, func(t *testing.T) {
				if !stringInSlice(got, want) {
					t.Fatalf("actionTags() = %v, want %q", got, want)
				}
			})
		}
	})

	t.Run("action tags include protected environment and member role hints", func(t *testing.T) {
		protected := actionTags("group.protected_environment_create", "group", "protected_environment_create", nil)
		if !stringInSlice(protected, "protected environment") {
			t.Fatalf("actionTags(protected environment) = %v, want protected environment", protected)
		}
		memberRole := actionTags("member_role.create", "member_role", "create", nil)
		if !stringInSlice(memberRole, "member role") {
			t.Fatalf("actionTags(member role) = %v, want member role", memberRole)
		}
	})
}

func TestRegistry_HelperCoverage(t *testing.T) {
	t.Run("annotations with nil base", func(t *testing.T) {
		got := copyAnnotations(nil)
		if got == nil {
			t.Fatal("copyAnnotations(nil) = nil, want zero-value annotations")
		}
		if got.Title != "" {
			t.Fatalf("copyAnnotations(nil).Title = %q, want empty", got.Title)
		}
	})

	t.Run("dedupe strings trims empty and duplicates", func(t *testing.T) {
		got := dedupeStrings([]string{" Project ", "", "project", "Issue"})
		want := []string{"project", "issue"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("dedupeStrings() = %v, want %v", got, want)
		}
	})

	t.Run("normalized limit clamps low and high values", func(t *testing.T) {
		if got := normalizedLimit(0); got != defaultLimit {
			t.Fatalf("normalizedLimit(0) = %d, want %d", got, defaultLimit)
		}
		if got := normalizedLimit(maxLimit + 1); got != maxLimit {
			t.Fatalf("normalizedLimit(max+1) = %d, want %d", got, maxLimit)
		}
	})

	t.Run("suggest action ids returns nil for empty terms", func(t *testing.T) {
		registry := NewRegistry(testRoutes(t))
		if got := registry.suggestActionIDs("   ", 5); got != nil {
			t.Fatalf("suggestActionIDs(empty) = %v, want nil", got)
		}
	})

	t.Run("score entry rejects empty terms", func(t *testing.T) {
		if got := scoreEntry(actionEntry{ID: "project.get"}, nil); got != 0 {
			t.Fatalf("scoreEntry(empty terms) = %d, want 0", got)
		}
	})

	t.Run("segmented search ignores short queries", func(t *testing.T) {
		registry := NewRegistry(testRoutes(t))
		got, err := registry.segmentedSearchMatchesWithScorer(t.Context(), normalizeSearchTerms("project get"), defaultLimit, scoreEntryWithoutExplanation)
		if err != nil {
			t.Fatalf("segmentedSearchMatchesWithScorer(short query) error = %v", err)
		}
		if got != nil {
			t.Fatalf("segmentedSearchMatchesWithScorer(short query) = %v, want nil", got)
		}
	})
}

// TestScoreSearch_Alternative covers every ranking branch in the exact search
// scorer. This keeps the weighting contract explicit while fuzzy fallback stays
// isolated to only zero-result searches.
func TestScoreSearch_Alternative(t *testing.T) {
	base := actionEntry{
		ID:         "project.delete",
		Domain:     "project",
		Action:     "delete",
		Aliases:    []string{"project.destroy"},
		Tags:       []string{"danger"},
		SearchText: "project delete owner",
	}

	tests := []struct {
		name        string
		entry       actionEntry
		raw         string
		alternative string
		want        int
	}{
		{name: "canonical id", entry: base, raw: "project.delete", alternative: "project.delete", want: 120},
		{name: "alias", entry: base, raw: "project.destroy", alternative: "project.destroy", want: 100},
		{name: "tag", entry: base, raw: "danger", alternative: "danger", want: 90},
		{name: "action", entry: base, raw: "delete", alternative: "delete", want: 80},
		{name: "id contains", entry: base, raw: "ject.del", alternative: "ject.del", want: 55},
		{name: "domain contains", entry: actionEntry{ID: "x.y", Domain: "project", Action: "remove"}, raw: "proj", alternative: "proj", want: 45},
		{name: "raw search text", entry: actionEntry{ID: "x.y", Domain: "x", Action: "y", SearchText: "owner filter"}, raw: "owner", alternative: "owner", want: 25},
		{name: "synonym search text", entry: actionEntry{ID: "x.y", Domain: "x", Action: "y", SearchText: "owner filter"}, raw: "owned", alternative: "owner", want: 18},
		{name: "no match", entry: base, raw: "missing", alternative: "missing", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreSearchAlternative(tt.entry, tt.raw, tt.alternative)
			if got != tt.want {
				t.Fatalf("scoreSearchAlternative() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestScoreSearchAlternative_WeightOrdering documents the intended precedence
// between exact metadata fields so future tuning can change weights deliberately.
func TestScoreSearchAlternative_WeightOrdering(t *testing.T) {
	entry := actionEntry{
		ID:         "project.delete",
		Domain:     "project",
		Action:     "delete",
		Aliases:    []string{"project.destroy"},
		Tags:       []string{"danger"},
		SearchText: "project delete owner",
	}

	scores := []int{
		scoreSearchAlternative(entry, "project.delete", "project.delete"),
		scoreSearchAlternative(entry, "project.destroy", "project.destroy"),
		scoreSearchAlternative(entry, "danger", "danger"),
		scoreSearchAlternative(entry, "delete", "delete"),
		scoreSearchAlternative(entry, "owner", "owner"),
	}
	for index := 1; index < len(scores); index++ {
		if scores[index-1] <= scores[index] {
			t.Fatalf("scores = %v, want strictly descending precedence", scores)
		}
	}
}

// TestScoreSearchAlternative_ReturnsReason verifies the ScoreSearchAlternative_ReturnsReason handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestScoreSearchAlternative_ReturnsReason(t *testing.T) {
	entry := actionEntry{
		ID:         "issue.list",
		Domain:     "issue",
		Action:     "list",
		SearchText: "issue list author_username",
	}

	score, reason := scoreSearchAlternativeWithReason(entry, "author", "author_username")
	if score == 0 {
		t.Fatal("scoreSearchAlternativeWithReason() score = 0, want match")
	}
	if reason.Field == "" || reason.QueryTerm == "" || reason.MatchedValue == "" {
		t.Fatalf("reason = %+v, want non-empty field, query term, and matched value", reason)
	}
	if reason.QueryTerm != "author" || reason.Alternative != "author_username" {
		t.Fatalf("reason = %+v, want original term and synonym alternative", reason)
	}
}

// TestScoreSearchAlternative_SchemaParamWeights verifies schema-aware ranking
// prefers required params over optional params while still considering enum and
// description values as weak repair signals.
func TestScoreSearchAlternative_SchemaParamWeights(t *testing.T) {
	document := searchDocument{
		CanonicalID:    "issue.list",
		Domain:         "issue",
		Action:         "list",
		RequiredParams: []string{"project_id"},
		OptionalParams: []string{"state"},
		SchemaEnums:    []string{"opened"},
		SchemaDescTerms: []string{
			"filter issues by assignee username",
		},
		FlatText: "issue list project_id state opened filter issues by assignee username",
	}
	entry := actionEntry{ID: "issue.list", Domain: "issue", Action: "list", Document: document}

	required := scoreSearchAlternative(entry, "project_id", "project_id")
	optional := scoreSearchAlternative(entry, "state", "state")
	enumValue := scoreSearchAlternative(entry, "opened", "opened")
	description := scoreSearchAlternative(entry, "assignee", "assignee")

	if required <= enumValue || enumValue <= optional || optional <= description {
		t.Fatalf("scores required=%d enum=%d optional=%d description=%d, want required > enum > optional > description", required, enumValue, optional, description)
	}
}

// TestComputeConfidence_Thresholds documents the current high-confidence gates:
// score must be at least 80 and the top-result margin must be at least 15.
func TestComputeConfidence_Thresholds(t *testing.T) {
	tests := []struct {
		name    string
		matches []scoredActionEntry
		wantLow bool
	}{
		{
			name: "high confidence at thresholds",
			matches: []scoredActionEntry{
				{score: minimumHighConfidenceScore},
				{score: minimumHighConfidenceScore - minimumHighConfidenceMargin},
			},
		},
		{
			name: "low score",
			matches: []scoredActionEntry{
				{score: minimumHighConfidenceScore - 1},
			},
			wantLow: true,
		},
		{
			name: "close margin",
			matches: []scoredActionEntry{
				{score: 100},
				{score: 100 - minimumHighConfidenceMargin + 1},
			},
			wantLow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeConfidence(tt.matches)
			if got[0].lowConfidence != tt.wantLow {
				t.Fatalf("lowConfidence = %t, want %t", got[0].lowConfidence, tt.wantLow)
			}
			if got[0].explanation.LowConfidence != tt.wantLow {
				t.Fatalf("explanation.LowConfidence = %t, want %t", got[0].explanation.LowConfidence, tt.wantLow)
			}
		})
	}
}

// TestMergeBestMatches_KeepsTheFirstGroupOnATie verifies which of two matches
// for one action survives the merge when both carry the same score.
//
// The groups arrive in a fixed order: the lexical pass first, then the fuzzy
// recovery, then the segmented windows. A tie therefore has a right answer,
// the lexical match, whose explanation says the query matched the action as
// written rather than at an edit distance; letting a later group overwrite an
// equal score would hand the caller a fuzzy explanation for a match that was
// never a typo.
func TestMergeBestMatches_KeepsTheFirstGroupOnATie(t *testing.T) {
	lexical := []scoredActionEntry{{
		entry:       actionEntry{ID: "project.get"},
		score:       100,
		explanation: ScoringExplanation{TotalScore: 100, Reasons: []MatchReason{{Field: searchFieldCanonicalID}}},
	}}
	fuzzy := []scoredActionEntry{{
		entry:       actionEntry{ID: "project.get"},
		score:       100,
		explanation: ScoringExplanation{TotalScore: 100, Reasons: []MatchReason{{Field: searchFieldFuzzyToken, Fuzzy: true}}},
	}}

	merged := mergeBestMatches(lexical, fuzzy)
	if len(merged) != 1 {
		t.Fatalf("mergeBestMatches() = %+v, want one match per action ID", merged)
	}
	if merged[0].explanation.Reasons[0].Field != searchFieldCanonicalID {
		t.Fatalf("merged reason field = %q, want the first group's %q on a tie", merged[0].explanation.Reasons[0].Field, searchFieldCanonicalID)
	}

	better := []scoredActionEntry{{entry: actionEntry{ID: "project.get"}, score: 101, explanation: ScoringExplanation{TotalScore: 101}}}
	if withBetter := mergeBestMatches(lexical, better); withBetter[0].score != 101 {
		t.Fatalf("mergeBestMatches() score = %d, want the higher-scoring later group to win", withBetter[0].score)
	}
}

// TestSearchRuntimeMetrics_RecordQualitySignals verifies the SearchRuntimeMetrics_RecordQualitySignals handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSearchRuntimeMetrics_RecordQualitySignals(t *testing.T) {
	ResetSearchRuntimeMetrics()
	t.Cleanup(ResetSearchRuntimeMetrics)
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	// sequential: three searches accumulating into one metrics snapshot.
	for _, query := range []string{"zzzzzzzz", "merje requesy", "danger.delete"} {
		if _, err := registry.searchMatches(t.Context(), query, 5, false); err != nil {
			t.Fatalf("searchMatches(%q) error = %v", query, err)
		}
	}

	metrics := SearchRuntimeMetricsSnapshot()
	if metrics.Searches != 3 {
		t.Fatalf("Searches = %d, want 3", metrics.Searches)
	}
	if metrics.ZeroResultSearches == 0 {
		t.Fatalf("metrics = %+v, want zero-result search recorded", metrics)
	}
	if metrics.FuzzyFallbackSearches == 0 {
		t.Fatalf("metrics = %+v, want fuzzy fallback recorded", metrics)
	}
	if metrics.AmbiguousAliasQueries == 0 {
		t.Fatalf("metrics = %+v, want ambiguous alias query recorded", metrics)
	}
	if metrics.LowConfidenceSearches == 0 {
		t.Fatalf("metrics = %+v, want low-confidence search recorded", metrics)
	}
	destructiveSuppressions := metrics.DestructiveFuzzySuppressions
	recordSearchRuntimeMetrics(1, false, false, false, -1)
	if got := SearchRuntimeMetricsSnapshot().DestructiveFuzzySuppressions; got != destructiveSuppressions {
		t.Fatalf("DestructiveFuzzySuppressions after negative input = %d, want %d", got, destructiveSuppressions)
	}
}

// TestSearchMatches_RecordsAQualitySignalOnlyForTheSearchThatShowsIt counts the
// fuzzy-fallback and ambiguous-alias signals one search at a time.
//
// These counters are read to decide whether the catalog's wording is working,
// so a signal that fires on every search says nothing at all. The two searches
// that must not raise the fuzzy counter are the interesting ones: a query that
// matched exactly never runs the fuzzy pass, and a query that ran it and
// recovered nothing ran it for nothing. A snapshot taken over several searches
// cannot tell either apart from a recovery, which is why each search here gets
// its own.
func TestSearchMatches_RecordsAQualitySignalOnlyForTheSearchThatShowsIt(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	cases := []struct {
		name          string
		query         string
		wantFuzzy     uint64
		wantAmbiguous uint64
	}{
		{name: "a canonical ID matches outright", query: "project.get"},
		{name: "a typo the fuzzy pass recovers", query: "merje requesy", wantFuzzy: 1},
		{name: "a query the fuzzy pass recovers nothing for", query: "zzzzzzzz"},
		{name: "an alias two actions claim", query: "danger.delete", wantFuzzy: 1, wantAmbiguous: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ResetSearchRuntimeMetrics()
			t.Cleanup(ResetSearchRuntimeMetrics)

			if _, err := registry.searchMatches(t.Context(), tc.query, defaultLimit, false); err != nil {
				t.Fatalf("searchMatches(%q) error = %v", tc.query, err)
			}

			metrics := SearchRuntimeMetricsSnapshot()
			if metrics.FuzzyFallbackSearches != tc.wantFuzzy {
				t.Errorf("FuzzyFallbackSearches = %d, want %d", metrics.FuzzyFallbackSearches, tc.wantFuzzy)
			}
			if metrics.AmbiguousAliasQueries != tc.wantAmbiguous {
				t.Errorf("AmbiguousAliasQueries = %d, want %d", metrics.AmbiguousAliasQueries, tc.wantAmbiguous)
			}
		})
	}
}

// TestSearchMatches_CountsTheFuzzyMatchesItSuppressedNotTheOnesItSaw verifies
// the destructive-suppression counter reports what the safety filter removed.
//
// The counter is the only record that fuzzy recovery reached a destructive
// action and was stopped, and it is read against the number of searches to
// judge whether the filter is too eager. Counting the matches the pass produced
// instead of the ones it dropped would inflate it by everything the filter
// allowed through, which is most of them.
func TestSearchMatches_CountsTheFuzzyMatchesItSuppressedNotTheOnesItSaw(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	const query = "delet projekt"
	terms := normalizeSearchTerms(query)

	fuzzy, err := registry.scoredMatches(t.Context(), terms, fuzzyScoreEntryWithoutExplanation)
	if err != nil {
		t.Fatalf("scoredMatches(fuzzy) error = %v", err)
	}
	kept := filterUnsafeFuzzyMatches(terms, fuzzy)
	if len(fuzzy) <= len(kept) || len(kept) == 0 {
		t.Fatalf("fuzzy pass for %q produced %d matches and kept %d, want some suppressed and some kept so the two counts differ", query, len(fuzzy), len(kept))
	}
	want := uint64(len(fuzzy)) - uint64(len(kept))

	ResetSearchRuntimeMetrics()
	t.Cleanup(ResetSearchRuntimeMetrics)
	if _, searchErr := registry.searchMatches(t.Context(), query, defaultLimit, false); searchErr != nil {
		t.Fatalf("searchMatches(%q) error = %v", query, searchErr)
	}

	if got := SearchRuntimeMetricsSnapshot().DestructiveFuzzySuppressions; got != want {
		t.Fatalf("DestructiveFuzzySuppressions = %d, want %d (%d fuzzy matches less the %d kept)", got, want, len(fuzzy), len(kept))
	}
}

// TestRegistryMetrics_SummarizesRegistryAndIndex verifies that registry metrics
// report action, index, alias, and ambiguity counts. The fixture includes a
// deprecated alias and an ambiguous alias to catch mapping-count regressions.
func TestRegistryMetrics_SummarizesRegistryAndIndex(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "project.lookup", Canonical: "project.get"},
		{Alias: "project.compat", Canonical: "project.get", Source: aliasSourceDeprecated},
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	metrics := registry.Metrics()
	if metrics.ActionCount != len(registry.entries) {
		t.Fatalf("ActionCount = %d, want %d", metrics.ActionCount, len(registry.entries))
	}
	if metrics.IndexTokenCount == 0 || metrics.IndexPostingCount == 0 {
		t.Fatalf("metrics = %+v, want populated search index metrics", metrics)
	}
	if metrics.AliasCount != 4 || metrics.SearchableAliasCount != 2 || metrics.UnsearchableAliasCount != 1 || metrics.AmbiguousAliasCount != 1 {
		t.Fatalf("metrics = %+v, want alias count 4, searchable names 2, unsearchable mappings 1, ambiguous aliases 1", metrics)
	}
}

// TestExplanationSummary_FallbacksAndEscaping verifies that scoring summaries
// fall back to stable placeholders and escape table-breaking characters. It
// covers nil, empty, fuzzy, and query-term fallback explanations.
func TestExplanationSummary_FallbacksAndEscaping(t *testing.T) {
	if got := explanationSummary(nil); got != "-" {
		t.Fatalf("explanationSummary(nil) = %q, want dash", got)
	}
	if got := explanationSummary(&ScoringExplanation{}); got != "-" {
		t.Fatalf("explanationSummary(empty) = %q, want dash", got)
	}

	summary := explanationSummary(&ScoringExplanation{Reasons: []MatchReason{{
		Field:       searchFieldFuzzyToken,
		QueryTerm:   "project|delete\nnow",
		Alternative: "project.delete",
		Fuzzy:       true,
	}}})
	if !strings.Contains(summary, "fuzzy-matched") || strings.Contains(summary, "|") || strings.Contains(summary, "\n") {
		t.Fatalf("explanationSummary(fuzzy) = %q, want escaped single-line fuzzy summary", summary)
	}

	queryFallback := explanationSummary(&ScoringExplanation{Reasons: []MatchReason{{Field: searchFieldAlias, QueryTerm: "project.get"}}})
	if !strings.Contains(queryFallback, "project.get") {
		t.Fatalf("explanationSummary(query fallback) = %q, want query term", queryFallback)
	}
}

// TestDynamicParamValidation_DefensiveBranches verifies defensive helpers for
// dynamic parameter normalization and unknown-parameter detection. It covers nil
// schemas, alternative required groups, confirm bypasses, and nearest-name hints.
func TestDynamicParamValidation_DefensiveBranches(t *testing.T) {
	if got := NormalizeActionScopedParams("job.list", map[string]any{"status": "failed"}, schemaWithProperties("scope")); got["scope"] != "failed" {
		t.Fatalf("NormalizeActionScopedParams() = %#v, want scope alias", got)
	}
	if got := unknownDynamicParamNames(nil, []string{"project_id"}); got != nil {
		t.Fatalf("unknownDynamicParamNames(nil) = %v, want nil", got)
	}
	if got := unknownDynamicParamNames(map[string]any{"confirm": true}, []string{"project_id"}); len(got) != 0 {
		t.Fatalf("unknownDynamicParamNames(confirm) = %v, want empty", got)
	}
	if got := unknownDynamicParamNames(map[string]any{"project_id": 1}, nil); got != nil {
		t.Fatalf("unknownDynamicParamNames(no valid params) = %v, want nil", got)
	}
	if got := rootRequiredParams(nil); got != nil {
		t.Fatalf("rootRequiredParams(nil) = %v, want nil", got)
	}
	if got := alternativeRequiredParamGroups(map[string]any{"anyOf": []any{"invalid", map[string]any{"required": []any{"file_path"}}}}); len(got) != 1 || got[0][0] != "file_path" {
		t.Fatalf("alternativeRequiredParamGroups() = %v, want file_path group", got)
	}
	if got := alternativeRequiredParamGroups(map[string]any{"anyOf": "invalid", "oneOf": []any{map[string]any{"required": []any{"content"}}}}); len(got) != 1 || got[0][0] != "content" {
		t.Fatalf("alternativeRequiredParamGroups(oneOf fallback) = %v, want content group", got)
	}
	if got := alternativeRequiredParamGroups(nil); got != nil {
		t.Fatalf("alternativeRequiredParamGroups(nil) = %v, want nil", got)
	}
	if got := closestDynamicParamName("proj", []string{"project_id"}); got != "project_id" {
		t.Fatalf("closestDynamicParamName() = %q, want project_id", got)
	}
}

// TestClosestDynamicParamName_NearestWinsAndAFarNameGetsNoSuggestion verifies
// which candidate a typo is corrected to: the nearest one, the first of several
// equally near ones, and none at all when nothing is close.
//
// The suggestion is printed as "you meant this", so a wrong one is worse than
// silence: it sends the caller to a param that has nothing to do with what they
// typed. The tie case pins the first-wins rule, which is what makes the
// suggestion stable for a caller who retries the same call.
func TestClosestDynamicParamName_NearestWinsAndAFarNameGetsNoSuggestion(t *testing.T) {
	if got := closestDynamicParamName("aaa", []string{"aab", "aac"}); got != "aab" {
		t.Errorf("closestDynamicParamName(equally near candidates) = %q, want the first of them", got)
	}
	if got := closestDynamicParamName("zzzzzzzz", []string{"project_id"}); got != "" {
		t.Errorf("closestDynamicParamName(nothing close) = %q, want no suggestion", got)
	}
}

// TestActionScopedParamValueConversions verifies action-scoped conversion
// helpers for issue state events, GitLab access levels, and boolean strings. It
// covers accepted inputs and rejected edge cases without external fixtures.
func TestActionScopedParamValueConversions(t *testing.T) {
	stateCases := []struct {
		name  string
		input any
		want  string
	}{
		{name: "state_closed_becomes_close", input: "closed", want: "close"},
		{name: "state_uppercase_open_becomes_reopen", input: "OPEN", want: "reopen"},
	}
	for _, tc := range stateCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := actioncompat.IssueStateEventValue(tc.input)
			if !ok || got != tc.want {
				t.Fatalf("issueStateEventValue(%v) = %q, %t; want %q, true", tc.input, got, ok, tc.want)
			}
		})
	}
	if _, ok := actioncompat.IssueStateEventValue(123); ok {
		t.Fatal("issueStateEventValue(non-string) converted unexpectedly")
	}
	if _, ok := actioncompat.IssueStateEventValue("archived"); ok {
		t.Fatal("issueStateEventValue(archived) converted unexpectedly")
	}

	accessCases := []struct {
		name  string
		input any
		want  int
	}{
		{name: "access_int", input: 10, want: 10},
		{name: "access_int64", input: int64(20), want: 20},
		{name: "access_float64", input: float64(30), want: 30},
		{name: "access_numeric_string", input: "40", want: 40},
		{name: "access_guest", input: "guest", want: 10},
		{name: "access_reporter", input: "reporter", want: 20},
		{name: "access_developer", input: "developer", want: 30},
		{name: "access_padded_maintainer", input: " maintainer ", want: 40},
		{name: "access_owner", input: "owner", want: 50},
	}
	for _, tc := range accessCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := actioncompat.GitLabAccessLevelValue(tc.input)
			if !ok || got != tc.want {
				t.Fatalf("gitlabAccessLevelValue(%v) = %d, %t; want %d, true", tc.input, got, ok, tc.want)
			}
		})
	}
	rejectedAccess := []struct {
		name  string
		input any
	}{
		{name: "access_rejects_fractional_float", input: float64(30.5)},
		{name: "access_rejects_unknown_int", input: 70},
		{name: "access_rejects_unknown_int64", input: int64(70)},
		{name: "access_rejects_unknown_numeric_string", input: "70"},
		{name: "access_rejects_admin", input: "admin"},
		{name: "access_rejects_bool", input: true},
	}
	for _, tc := range rejectedAccess {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := actioncompat.GitLabAccessLevelValue(tc.input); ok {
				t.Fatalf("gitlabAccessLevelValue(%v) = %d, true; want false", tc.input, got)
			}
		})
	}

	if value, ok := actioncompat.BoolStringValue(" true "); !ok || !value {
		t.Fatalf("boolStringValue(true) = %t, %t; want true, true", value, ok)
	}
	rejectedBools := []struct {
		name  string
		input any
	}{
		{name: "bool_rejects_true_not_string", input: true},
		{name: "bool_rejects_unrecognized_string", input: "not-bool"},
	}
	for _, tc := range rejectedBools {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := actioncompat.BoolStringValue(tc.input); ok {
				t.Fatalf("boolStringValue(%v) converted unexpectedly", tc.input)
			}
		})
	}
}

// TestSnippetParamNormalization_DefensiveBranches verifies snippet file
// normalization helpers preserve invalid entries and only clone maps when a
// conversion is possible. It uses in-memory parameter maps as fixtures.
func TestSnippetParamNormalization_DefensiveBranches(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"files": map[string]any{}}}
	params := map[string]any{"content": "body"}
	normalized, explanations := NormalizeActionScopedParamsWithExplanation("snippet.project_create", params, schema)
	if _, hasFiles := normalized["files"]; hasFiles || len(explanations) != 0 {
		t.Fatalf("NormalizeActionScopedParamsWithExplanation() = %+v, %+v; want no snippet conversion without file_name", normalized, explanations)
	}

	files := map[string]any{"files": []any{"not-a-map", map[string]any{"file_name": "a.go"}}}
	normalized, explanations = NormalizeActionScopedParamsWithExplanation("snippet.project_create", files, schema)
	if len(explanations) == 0 {
		t.Fatal("NormalizeActionScopedParamsWithExplanation() produced no explanation, want files.file_name normalization")
	}
	if got := normalized["files"].([]any)[0]; got != "not-a-map" {
		t.Fatalf("first file entry = %#v, want original non-map", got)
	}

	actions := map[string]any{"files": []any{"not-a-map", map[string]any{"action": "create", "file_path": "a.go"}}}
	normalized, explanations = NormalizeActionScopedParamsWithExplanation("snippet.project_create", actions, schema)
	if len(explanations) == 0 {
		t.Fatal("NormalizeActionScopedParamsWithExplanation() produced no explanation, want files.action normalization")
	}
	if got := normalized["files"].([]any)[0]; got != "not-a-map" {
		t.Fatalf("first action entry = %#v, want original non-map", got)
	}
}

// TestCompatibilityAliasAndDescriptionBranches verifies compatibility alias
// normalization, alias deduplication, dynamic description fallback behavior, and
// compact schema rendering for nil or unmarshalable schemas.
func TestCompatibilityAliasAndDescriptionBranches(t *testing.T) {
	if got := catalogActionAliases(nil); got != nil {
		t.Fatalf("catalogActionAliases(nil) = %+v, want nil", got)
	}
	if got := sourceForCompatibilityAlias("", false); got != aliasSourceCompatibility {
		t.Fatalf("sourceForCompatibilityAlias(empty) = %q, want compatibility", got)
	}
	if got := sourceForCompatibilityAlias(" provider_observed ", false); got != aliasSourceProviderObserved {
		t.Fatalf("sourceForCompatibilityAlias(provider) = %q, want provider_observed", got)
	}
	if got := sourceForCompatibilityAlias("catalog", true); got != aliasSourceDeprecated {
		t.Fatalf("sourceForCompatibilityAlias(deprecated) = %q, want deprecated", got)
	}

	if got, ok := NormalizeCompatibilityActionAlias(" FEATURE_FLAG_USER_LIST.CREATE "); !ok || got != "feature_flags.ff_user_list_create" {
		t.Fatalf("NormalizeCompatibilityActionAlias() = %q, %t; want feature_flags.ff_user_list_create, true", got, ok)
	}
	for _, actionID := range []string{"", "project.get", "project.unknown"} {
		t.Run(actionID, func(t *testing.T) {
			if got, ok := NormalizeCompatibilityActionAlias(actionID); ok || got != strings.ToLower(strings.TrimSpace(actionID)) {
				t.Fatalf("NormalizeCompatibilityActionAlias(%q) = %q, %t; want unchanged false", actionID, got, ok)
			}
		})
	}

	aliases := dedupeActionAliases([]actionAlias{{Alias: "", Canonical: "project.get"}, {Alias: "project.lookup", Canonical: "project.get"}, {Alias: "project.lookup", Canonical: "project.get"}})
	if len(aliases) != 1 || aliases[0].Alias != "project.lookup" {
		t.Fatalf("dedupeActionAliases() = %+v, want one normalized alias", aliases)
	}

	description := describeEntry(actionEntry{ID: "missing.action", Tool: "gitlab_missing", Domain: "missing", Action: "action", Route: toolutil.ActionRoute{OutputSchema: map[string]any{"type": "object"}}})
	if description.InputSchema["additionalProperties"] != true || description.OutputSchema["type"] != "object" {
		t.Fatalf("describeEntry(fallback) = %+v, want fallback input schema and cloned output schema", description)
	}
	registry := NewRegistry(testRoutes(t))
	if got := describeEntry(registry.entries[0]); got.InputSchema["type"] == "" || got.Example.Tool != executeActionToolName {
		t.Fatalf("describeEntry(success) = %+v, want schema and dynamic execute example", got)
	}
	if got := compactSchemaJSON(nil); got != "" {
		t.Fatalf("compactSchemaJSON(nil) = %q, want empty", got)
	}
	if got := compactSchemaJSON(map[string]any{"bad": make(chan int)}); got != "" {
		t.Fatalf("compactSchemaJSON(unmarshalable) = %q, want empty", got)
	}
}

// TestDedupeActionAliases_DropsAnAliasWithNoCanonicalTarget verifies the other
// half of the emptiness guard over the compatibility alias table: an entry that
// names an alias but no action is dropped, as one naming an action but no alias
// already was.
//
// The table is hand-written, and the canonical side is the half that rots: an
// action renamed in its ActionSpec leaves the alias pointing at nothing. Such
// an entry has to go rather than be indexed, because the registry would
// otherwise hold an alias resolving to the empty action ID, which no lookup can
// answer and which the ambiguity table would happily collect duplicates under.
// The blank spelling is deliberate: what makes the entry empty is the trim this
// function applies, not the literal in the table.
func TestDedupeActionAliases_DropsAnAliasWithNoCanonicalTarget(t *testing.T) {
	aliases := dedupeActionAliases(actionCompatAliases([]actioncompat.ActionAlias{
		{Alias: "project.lookup", Canonical: "project.get"},
		{Alias: "project.fetch", Canonical: "   "},
	}))
	if len(aliases) != 1 || aliases[0].Alias != "project.lookup" || aliases[0].Canonical != "project.get" {
		t.Fatalf("dedupeActionAliases() = %+v, want only the alias that names an action", aliases)
	}
}

// TestRelatedActionsForEntry_FallsBackToTheMetadataTable verifies where the
// related actions of a find result come from: the entry when the catalog
// carries them, and the hand-written table when it does not.
//
// The fallback is the whole reason the table exists. These are the pointers
// that keep a model from calling package.list for a container registry or
// job.artifacts for one artifact path, and an entry that declares none of its
// own has to reach them, so a guard that answered "the entry has none" with an
// empty list would silently drop every pointer the table holds.
func TestRelatedActionsForEntry_FallsBackToTheMetadataTable(t *testing.T) {
	t.Run("the entry carries its own", func(t *testing.T) {
		entry := actionEntry{ID: "project.get", RelatedActions: []string{"project.list"}}
		if got := relatedActionsForEntry(entry); !slices.Equal(got, []string{"project.list"}) {
			t.Fatalf("relatedActionsForEntry() = %v, want the entry's own list", got)
		}
	})

	t.Run("an entry with none takes the table's", func(t *testing.T) {
		want := actionUXMetadataByID["project.get"].RelatedActions
		if len(want) == 0 {
			t.Fatal("actionUXMetadataByID no longer carries related actions for project.get")
		}
		if got := relatedActionsForEntry(actionEntry{ID: "project.get"}); !slices.Equal(got, want) {
			t.Fatalf("relatedActionsForEntry() = %v, want the table's %v", got, want)
		}
	})

	t.Run("an action the table does not know", func(t *testing.T) {
		if got := relatedActionsForEntry(actionEntry{ID: "widget.ping"}); len(got) != 0 {
			t.Fatalf("relatedActionsForEntry() = %v, want none", got)
		}
	})
}

// TestScoredMatchesAndDestructiveFuzzyBranches verifies corrupted-index
// resilience and destructive fuzzy-match safety checks. The fixture keeps the
// dynamic registry in memory and injects invalid postings directly.
func TestScoredMatchesAndDestructiveFuzzyBranches(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	registry.SearchIndex.byToken["project"] = []int{-1, 0, len(registry.entries)}
	matches, err := registry.scoredMatches(t.Context(), normalizeSearchTerms("project"), scoreEntryWithoutExplanation)
	if err != nil {
		t.Fatalf("scoredMatches(corrupted index) error = %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("scoredMatches(corrupted index) = %+v, want valid matches", matches)
	}

	entry := actionEntry{ID: "project.delete", Domain: "project", Action: "delete", Destructive: true}
	if allowsDestructiveFuzzyMatch(normalizeSearchTerms("purge"), entry) {
		t.Fatal("allowsDestructiveFuzzyMatch(purge without resource) = true, want false")
	}
	if !allowsDestructiveFuzzyMatch(normalizeSearchTerms("delete project"), entry) {
		t.Fatal("allowsDestructiveFuzzyMatch(delete project) = false, want true")
	}
}

// TestHasExactDestructiveVerb_ReadsEachNamedVerbAndNothingElse verifies the
// first half of the gate a fuzzy match has to pass before it may reach a
// destructive action: the query spells one of the five verbs exactly.
//
// Each verb gets a query of its own because they are read in order and any one
// of them answers for the whole list. Until this existed, only "delete" and
// "purge" had ever been the one that answered, so a fuzzy query saying
// "destroy", "remove" or "revoke" was let through by nothing that had been
// exercised, and dropping any of those three from the list would have gone
// unnoticed. The near misses are here for the other direction: the gate reads
// the raw term, never a stem or a synonym, so a query saying "deletes" or
// "removal" is not a destructive verb and a fuzzy match on a destructive
// action stays refused for it.
func TestHasExactDestructiveVerb_ReadsEachNamedVerbAndNothingElse(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "delete", query: "delete project", want: true},
		{name: "destroy", query: "destroy project", want: true},
		{name: "remove", query: "remove project", want: true},
		{name: "revoke", query: "revoke personal access token", want: true},
		{name: "purge", query: "purge project", want: true},
		{name: "a verb that only looks like one of them", query: "deletes project"},
		{name: "a noun built from one of them", query: "removal of a project"},
		{name: "no verb at all", query: "project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasExactDestructiveVerb(normalizeSearchTerms(tc.query)); got != tc.want {
				t.Errorf("hasExactDestructiveVerb(%q) = %t, want %t", tc.query, got, tc.want)
			}
		})
	}
}

// TestTermMatchesResourceSignal_ReadsAWholeNameAndItsWordsAlike verifies the
// resource signal a destructive fuzzy match has to carry: the query names the
// domain or the action, either as the identifier itself or as one of its words.
//
// Both halves are load-bearing for a multi-word identifier, and each is the
// only one that fires for its own query shape. "merge_request" as a caller
// types a canonical ID matches the name and none of the words; "merge" as a
// caller types a sentence matches a word and not the name. Keeping only one of
// them would let "delete the merge request" through and refuse
// "delete merge_request", or the reverse.
func TestTermMatchesResourceSignal_ReadsAWholeNameAndItsWordsAlike(t *testing.T) {
	document := searchDocument{
		Domain:      "merge_request",
		DomainWords: splitSearchFieldWords("merge_request"),
		Action:      "note_create",
		ActionWords: splitSearchFieldWords("note_create"),
		Tags:        []string{"mr note"},
	}

	cases := []struct {
		name string
		term string
		want bool
	}{
		{name: "the domain as written", term: "merge_request", want: true},
		{name: "a word of the domain", term: "merge", want: true},
		{name: "the action as written", term: "note_create", want: true},
		{name: "a word of the action", term: "create", want: true},
		{name: "a tag", term: "mr note", want: true},
		{name: "a term the action is not about", term: "pipeline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := termMatchesResourceSignal(tc.term, document); got != tc.want {
				t.Fatalf("termMatchesResourceSignal(%q) = %t, want %t", tc.term, got, tc.want)
			}
		})
	}
}

// TestFuzzyModeForMatches_ReadsTheConfidenceOfTheBestMatch verifies when the
// fuzzy pass is worth running: never when the lexical pass already produced a
// confident best match, always when it produced none, and on low confidence in
// between.
//
// The last subtest is about the guard rather than the decision. The preview is
// empty only when the limit keeps nothing, which no caller asks for today
// because normalizedLimit runs first, so the guard is what stands between a
// future caller with a zero limit and an index out of range on the line after.
func TestFuzzyModeForMatches_ReadsTheConfidenceOfTheBestMatch(t *testing.T) {
	confident := []scoredActionEntry{
		{entry: actionEntry{ID: "project.get"}, score: minimumHighConfidenceScore + 100},
		{entry: actionEntry{ID: "project.list"}, score: minimumHighConfidenceScore},
	}

	t.Run("no matches at all", func(t *testing.T) {
		if got := fuzzyModeForMatches(nil, defaultLimit); got != fuzzyZeroResults {
			t.Fatalf("fuzzyModeForMatches(none) = %v, want %v", got, fuzzyZeroResults)
		}
	})

	t.Run("a confident best match", func(t *testing.T) {
		if got := fuzzyModeForMatches(confident, defaultLimit); got != fuzzyDisabled {
			t.Fatalf("fuzzyModeForMatches(confident) = %v, want %v", got, fuzzyDisabled)
		}
	})

	t.Run("a best match below the score floor", func(t *testing.T) {
		matches := []scoredActionEntry{{entry: actionEntry{ID: "project.get"}, score: minimumHighConfidenceScore - 1}}
		if got := fuzzyModeForMatches(matches, defaultLimit); got != fuzzyLowConfidence {
			t.Fatalf("fuzzyModeForMatches(low confidence) = %v, want %v", got, fuzzyLowConfidence)
		}
	})

	t.Run("a limit that keeps nothing", func(t *testing.T) {
		if got := fuzzyModeForMatches(confident, 0); got != fuzzyDisabled {
			t.Fatalf("fuzzyModeForMatches(limit 0) = %v, want %v", got, fuzzyDisabled)
		}
	})
}

// testEnumStringer holds test enum stringer data for the dynamic package.
type testEnumStringer string

// String returns the display label for testEnumStringer.
func (value testEnumStringer) String() string { return string(value) }

// TestSchemaSearchTermHelpers_Branches verifies schema descriptions and enum
// values are extracted from mixed JSON-schema shapes. It covers non-object
// properties, empty descriptions, stringers, numbers, booleans, and strings.
func TestSchemaSearchTermHelpers_Branches(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{
		"plain":        "not-object",
		"empty_desc":   map[string]any{"description": "   "},
		"numeric_desc": map[string]any{"description": 42},
		"state": map[string]any{
			"description": "Merge request state",
			"enum":        []any{"opened", testEnumStringer("closed"), 30, int64(64), 1.5, true, struct{}{}},
		},
		"kind": map[string]any{"enum": []string{"bug", "feature"}},
	}}
	if descriptions := schemaPropertyDescriptions(schema); strings.Join(descriptions, ",") != "merge request state" {
		t.Fatalf("schemaPropertyDescriptions() = %v, want merge request state", descriptions)
	}
	enums := strings.Join(schemaPropertyEnumValues(schema), ",")
	for _, want := range []string{"opened", "closed", "30", "64", "1.5", "true", "bug", "feature"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(enums, want) {
				t.Fatalf("schemaPropertyEnumValues() = %q, missing %q", enums, want)
			}
		})
	}
}

// TestSuggestSearchTokens_Branches verifies token suggestion limit handling,
// fuzzy ordering, tie-breaking, deduplication, and static fallbacks. It uses the
// standard test registry plus a tiny custom index for deterministic ties.
func TestSuggestSearchTokens_Branches(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	if got := registry.suggestSearchTokens("project", 0); got != nil {
		t.Fatalf("suggestSearchTokens(limit 0) = %v, want nil", got)
	}
	near := registry.suggestSearchTokens("projec", 3)
	if len(near) == 0 || near[0] != "project" {
		t.Fatalf("suggestSearchTokens(projec) = %v, want project first", near)
	}
	withTie := (&Registry{SearchIndex: searchIndex{byToken: map[string][]int{"abc": {0}, "abd": {1}}, all: []int{0, 1}}}).suggestSearchTokens("abe", 1)
	if len(withTie) != 1 || withTie[0] != "abc" {
		t.Fatalf("suggestSearchTokens(tie/limit) = %v, want abc", withTie)
	}
	withDuplicateFallback := registry.suggestSearchTokens("projec", 10)
	if strings.Count(strings.Join(withDuplicateFallback, ","), "project") != 1 {
		t.Fatalf("suggestSearchTokens(fallback dedupe) = %v, want project once", withDuplicateFallback)
	}
	fallbacks := registry.suggestSearchTokens("zzzz", 2)
	if len(fallbacks) != 2 || fallbacks[0] != "project" || fallbacks[1] != "issue" {
		t.Fatalf("suggestSearchTokens(fallback) = %v, want first two fallbacks", fallbacks)
	}
}

// TestSuggestSearchTokens_OrdersByDistanceBeforeName verifies that the tokens
// offered after a search that found nothing are ordered by how close they are
// to the query, and that a token more than two edits away is not offered at all.
//
// The candidates are walked in alphabetical order, so ordering by name alone
// would put a far token above a near one and the first suggestion a model reads
// would be the worse guess. The cut-off is what keeps the list to tokens the
// caller plausibly meant; every term of the query gets its own distance, and the
// nearest of them decides.
func TestSuggestSearchTokens_OrdersByDistanceBeforeName(t *testing.T) {
	registry := &Registry{SearchIndex: searchIndex{byToken: map[string][]int{
		"aabcc":  {0}, // two edits from "abc", and alphabetically first
		"abcd":   {1}, // one edit from "abc"
		"zzzzzz": {2}, // further than the cut-off, so never offered
	}, all: []int{0, 1, 2}}}

	got := registry.suggestSearchTokens("abc abcdd", 2)
	if !slices.Equal(got, []string{"abcd", "aabcc"}) {
		t.Fatalf("suggestSearchTokens() = %v, want the nearer token first", got)
	}
}

// TestScoreSearchAlternativeWithReason_Branches verifies field-specific scoring
// explanations for canonical IDs, aliases, tags, schema fields, enums, and flat
// text. It uses table-driven subtests plus explicit fallback edge cases.
func TestScoreSearchAlternativeWithReason_Branches(t *testing.T) {
	entry := actionEntry{Document: searchDocument{
		CanonicalID:      "project.get",
		Tool:             "gitlab_project",
		Domain:           "project",
		DomainWords:      []string{"project"},
		Action:           "get",
		ActionWords:      []string{"get"},
		Aliases:          []string{"project.lookup"},
		Tags:             []string{"project details"},
		RequiredParams:   []string{"project_id"},
		OptionalParams:   []string{"statistics"},
		SchemaProperties: []string{"visibility_level"},
		SchemaEnums:      []string{"private"},
		SchemaDescTerms:  []string{"repository visibility"},
		FlatText:         "gitlab project lookup read repository visibility",
	}}

	tests := []struct {
		name        string
		raw         string
		alternative string
		wantField   string
	}{
		{name: "canonical", raw: "project.get", alternative: "project.get", wantField: searchFieldCanonicalID},
		{name: "alias", raw: "lookup", alternative: "project.lookup", wantField: searchFieldAlias},
		{name: "tag", raw: "details", alternative: "project details", wantField: searchFieldTag},
		{name: "action", raw: "get", alternative: "get", wantField: searchFieldAction},
		{name: "domain", raw: "project", alternative: "project", wantField: searchFieldDomain},
		{name: "id contains", raw: "proj", alternative: "project.g", wantField: searchFieldIDContains},
		{name: "tool", raw: "gitlab", alternative: "gitlab", wantField: searchFieldTool},
		{name: "required", raw: "project_id", alternative: "project_id", wantField: searchFieldRequiredParam},
		{name: "optional", raw: "statistics", alternative: "statistics", wantField: searchFieldOptionalParam},
		{name: "enum", raw: "private", alternative: "private", wantField: searchFieldSchemaEnum},
		{name: "description", raw: "visibility", alternative: "visibility", wantField: searchFieldSchemaDesc},
		{name: "property", raw: "visibility", alternative: "visibility_level", wantField: searchFieldSchemaProperty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, reason := scoreSearchAlternativeWithReason(entry, tt.raw, tt.alternative)
			if score == 0 || reason.Field != tt.wantField {
				t.Fatalf("scoreSearchAlternativeWithReason() = %d, %+v; want field %s", score, reason, tt.wantField)
			}
		})
	}
	if score, reason := scoreSearchAlternativeWithReason(entry, "missing", "missing"); score != 0 || reason.Field != "" {
		t.Fatalf("scoreSearchAlternativeWithReason(missing) = %d, %+v; want zero result", score, reason)
	}
	if score, reason := scoreSearchAlternativeWithReason(actionEntry{Document: searchDocument{CanonicalID: "ticket.list", Domain: "work_item", DomainWords: []string{"work item"}}}, "work", "work"); score == 0 || reason.Field != searchFieldDomainContains {
		t.Fatalf("scoreSearchAlternativeWithReason(domain contains) = %d, %+v; want domain contains", score, reason)
	}
	if score, reason := scoreSearchAlternativeWithReason(actionEntry{Document: searchDocument{CanonicalID: "ticket.list", Action: "schedule_project", ActionWords: []string{"schedule project"}}}, "sched", "sched"); score == 0 || reason.Field != searchFieldActionContains {
		t.Fatalf("scoreSearchAlternativeWithReason(action contains) = %d, %+v; want action contains", score, reason)
	}
	if score, reason := scoreSearchAlternativeWithReason(actionEntry{Document: searchDocument{CanonicalID: "project.get", FlatText: "read repository"}}, "read", "read"); score == 0 || reason.Field != searchFieldFlatText {
		t.Fatalf("scoreSearchAlternativeWithReason(flat exact) = %d, %+v; want flat text", score, reason)
	}
	if score, reason := scoreSearchAlternativeWithReason(actionEntry{Document: searchDocument{CanonicalID: "project.get", FlatText: "read repository"}}, "repo", "repository"); score == 0 || reason.Field != searchFieldFlatText {
		t.Fatalf("scoreSearchAlternativeWithReason(flat synonym) = %d, %+v; want flat text", score, reason)
	}

	schemaPropertyEntry := actionEntry{Document: searchDocument{
		CanonicalID:      "project.get",
		SchemaProperties: []string{"visibility_level"},
	}}
	if scoreSearchAlternative(schemaPropertyEntry, "visibility", "visibility_level") == 0 {
		t.Fatal("scoreSearchAlternative(schema property) = 0, want match")
	}
	if score := scoreFieldContainsFor("field", "field_extra"); score != scoreSynonymContains {
		t.Fatalf("scoreFieldContainsFor(synonym) = %d, want %d", score, scoreSynonymContains)
	}
	if score, explanation := scoreEntryWithExplanation(actionEntry{}, nil); score != 0 || len(explanation.Reasons) != 0 {
		t.Fatalf("scoreEntryWithExplanation(empty) = %d, %+v; want zero result", score, explanation)
	}
}

// TestScoreEntryWithExplanation_ScoresExactlyWhatTheSilentScorerScores holds
// the two scorers to one ranking over the whole catalog.
//
// They are the same function written twice, once returning the score alone and
// once returning it with its reasons, and a search picks between them on the
// explain flag. Nothing else pins them together: every rule they share, the
// match-ratio scaling, the minimum-matched-terms threshold with its explicit
// intent bypass, and each intent adjustment, is written out in both. If they
// drift, a caller who asks why an action ranked where it did is told about a
// ranking that is not the one they were served.
func TestScoreEntryWithExplanation_ScoresExactlyWhatTheSilentScorerScores(t *testing.T) {
	t.Parallel()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	entries := NewRegistryFromCatalog(catalog).entries

	queries := []string{
		"list open merge requests",
		"delete the project",
		"compare refs between two branches",
		"create a group service account",
		"search code in project my-org/tools for func Foo",
		"who am i current user profile",
		"retry the failed pipeline job",
		"protected branch rule for the group",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			terms := normalizeSearchTerms(query)
			for _, entry := range entries {
				silent := scoreEntry(entry, terms)
				explained, explanation := scoreEntryWithExplanation(entry, terms)
				if silent != explained {
					t.Fatalf("scoreEntry(%s) = %d, scoreEntryWithExplanation() = %d; want one ranking", entry.ID, silent, explained)
				}
				if explained > 0 && explanation.TotalScore != explained {
					t.Fatalf("explanation.TotalScore for %s = %d, want the score it was returned with, %d", entry.ID, explanation.TotalScore, explained)
				}
			}
		})
	}
}

// TestScoreEntry_TheExplicitIntentBypassNeedsBothTwoMatchesAndTheIntent pins
// the one exception to the minimum-matched-term threshold, in both scorers.
//
// A long prompt inflates the threshold: "search code in project my-org/tools
// for func Foo" is eight terms, of which search.code matches two, so the rule
// that a query must match nearly all of its terms would drop the one action
// the caller named outright. The bypass lets those two through, and it is
// narrow on purpose, in two directions that have to hold together. Two matched
// terms is the floor, because one term matching is a word in common rather
// than an intent. And the intent itself is required, or every action matching
// any two terms of a long query would be promoted, which is the ranking this
// threshold exists to prevent.
func TestScoreEntry_TheExplicitIntentBypassNeedsBothTwoMatchesAndTheIntent(t *testing.T) {
	t.Parallel()
	// Four terms, of which each entry matches exactly two: one below the
	// minimum of three that a four-term query asks for.
	withIntent := scoringEntry("search.code", "search", "code")
	withIntentTerms := searchTermsFromWords("search", "code", "zzzzzzzz", "yyyyyyyy")
	withoutIntent := scoringEntry("issue.list", "issue", "list")
	withoutIntentTerms := searchTermsFromWords("issue", "list", "zzzzzzzz", "yyyyyyyy")

	if got := minimumMatchedTermCount(withIntent, withIntentTerms); got != 3 {
		t.Fatalf("minimumMatchedTermCount() = %d, want 3 so the two matched terms are below it", got)
	}

	t.Run("an action carrying the intent is let through", func(t *testing.T) {
		t.Parallel()
		if got := scoreEntry(withIntent, withIntentTerms); got <= 0 {
			t.Errorf("scoreEntry(search.code) = %d, want a positive score through the intent bypass", got)
		}
		if got, _ := scoreEntryWithExplanation(withIntent, withIntentTerms); got <= 0 {
			t.Errorf("scoreEntryWithExplanation(search.code) = %d, want a positive score through the intent bypass", got)
		}
	})

	t.Run("an action carrying none is not", func(t *testing.T) {
		t.Parallel()
		if got := scoreEntry(withoutIntent, withoutIntentTerms); got != 0 {
			t.Errorf("scoreEntry(issue.list) = %d, want 0 below the minimum with no intent to bypass it", got)
		}
		if got, _ := scoreEntryWithExplanation(withoutIntent, withoutIntentTerms); got != 0 {
			t.Errorf("scoreEntryWithExplanation(issue.list) = %d, want 0 below the minimum with no intent to bypass it", got)
		}
	})
}

// TestScoreEntryWithExplanation_KeepsTheFirstAlternativeOfATie verifies which
// expansion of one query term is reported when two of them score the same.
//
// A term is scored through its synonyms and the best one is kept, and the term
// as the caller typed it comes first. On a tie it is the one that explains the
// match: telling a caller who typed "mr" that the action matched "merge
// request" describes the synonym table rather than their query.
func TestScoreEntryWithExplanation_KeepsTheFirstAlternativeOfATie(t *testing.T) {
	t.Parallel()
	entry := actionEntry{Document: searchDocument{
		CanonicalID: "merge_request.list",
		Tags:        []string{"mr", "merge request"},
	}}
	terms := []searchTerm{{Raw: "mr", Alternatives: []string{"mr", "merge request"}}}

	score, explanation := scoreEntryWithExplanation(entry, terms)
	if score == 0 || len(explanation.Reasons) == 0 {
		t.Fatalf("scoreEntryWithExplanation() = %d, %+v; want a scored match with reasons", score, explanation)
	}
	if explanation.Reasons[0].Field != searchFieldTag || explanation.Reasons[0].MatchedValue != "mr" {
		t.Fatalf("first reason = %+v, want the tag matched by the term as typed", explanation.Reasons[0])
	}
}

// TestScoreEntryWithExplanation_AZeroScoreCarriesNoExplanation verifies that an
// entry the adjustments take down to zero is returned as no match at all, with
// the zero explanation rather than the reasons that cancelled each other.
//
// The fixture lands on exactly zero: the canonical id matches outright for 120,
// and two action words the query never named take 60 each back off. The caller
// in the search path appends only what scores above zero and throws the
// explanation away, so nothing there can tell a populated explanation from an
// empty one; the explain surface calls this function directly, and would report
// a match with reasons and a total of zero.
func TestScoreEntryWithExplanation_AZeroScoreCarriesNoExplanation(t *testing.T) {
	t.Parallel()
	entry := actionEntry{ID: "widget", Document: searchDocument{
		CanonicalID: "widget",
		Domain:      "gadget",
		DomainWords: []string{"gadget"},
		Action:      "widget_alpha_beta",
		ActionWords: []string{"widget", "alpha", "beta"},
	}}
	terms := []searchTerm{{Raw: "widget", Alternatives: []string{"widget"}}}

	score, explanation := scoreEntryWithExplanation(entry, terms)
	if score != 0 {
		t.Fatalf("scoreEntryWithExplanation() score = %d, want 0 for an entry the adjustments cancel", score)
	}
	if !reflect.DeepEqual(explanation, ScoringExplanation{}) {
		t.Errorf("explanation = %+v, want the zero value: a score of zero is not a match to explain", explanation)
	}
}

// TestWordizeSearchDocument_OnlyADocumentWithValuesGetsTheMap verifies the
// precomputed word forms a search reads, and that a document with nothing to
// precompute is left with no map at all.
//
// Nothing else in the suite looks at WordizedValues: the search recomputes the
// same word form on the fly when the map is missing, so dropping the precompute
// changes no answer and only spends the CPU it exists to save. The nil case is
// the half that says the work was skipped rather than done into an empty map.
func TestWordizeSearchDocument_OnlyADocumentWithValuesGetsTheMap(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		document searchDocument
		want     map[string]string
	}{
		"a document with values": {
			document: searchDocument{
				DomainWords:    []string{"merge_request"},
				RequiredParams: []string{"project_id"},
			},
			want: map[string]string{"merge_request": "merge request", "project_id": "project id"},
		},
		"a document with none of the seven fields": {
			document: searchDocument{CanonicalID: "widget.list", Domain: "widget"},
			want:     nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			document := tt.document
			wordizeSearchDocument(&document)
			if !reflect.DeepEqual(document.WordizedValues, tt.want) {
				t.Errorf("WordizedValues = %#v, want %#v", document.WordizedValues, tt.want)
			}
		})
	}
}

// TestRequiredParamAndPlaceholderBranches verifies preferred required-parameter
// extraction from alternative schema groups and parameter placeholder selection.
// It uses small schema fixtures with no external setup.
func TestRequiredParamAndPlaceholderBranches(t *testing.T) {
	schema := map[string]any{"anyOf": []any{"invalid", map[string]any{"required": []any{"project_id"}}}}
	if got := appendPreferredAlternativeRequiredParams(nil, schema); len(got) != 1 || got[0] != "project_id" {
		t.Fatalf("appendPreferredAlternativeRequiredParams() = %v, want project_id", got)
	}
	if got := placeholderForParam("group_id"); got != "group/subgroup" {
		t.Fatalf("placeholderForParam(group_id) = %v, want group/subgroup", got)
	}
}

// schemaWithProperties extracts schema with properties details for schema assertions.
func schemaWithProperties(names ...string) map[string]any {
	properties := make(map[string]any, len(names))
	for _, name := range names {
		properties[name] = map[string]any{"type": "string"}
	}
	return map[string]any{"properties": properties}
}

// TestNormalization_FormattingBranches covers compact helpers that
// shape user-facing dynamic tool output. It verifies deduplication of described
// actions, placeholder selection, confirmation parsing, schema cloning failures,
// and empty-result Markdown messages.
func TestNormalization_FormattingBranches(t *testing.T) {
	t.Run("normalize describe ids trims and deduplicates", func(t *testing.T) {
		got := normalizeDescribeIDs(DescribeInput{Action: " Project.Get ", Actions: []string{"project.get", "", "Issue.List"}})
		want := []string{"project.get", "issue.list"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("normalizeDescribeIDs() = %v, want %v", got, want)
		}
	})

	t.Run("placeholder selects dates and generic values", func(t *testing.T) {
		if got := placeholderForParam("due_date"); got != "YYYY-MM-DD" {
			t.Fatalf("placeholderForParam(date) = %v, want YYYY-MM-DD", got)
		}
		if got := placeholderForParam("project_id"); got != "group/project" {
			t.Fatalf("placeholderForParam(project_id) = %v, want group/project", got)
		}
		if got := placeholderForParam("title"); got != "value" {
			t.Fatalf("placeholderForParam(title) = %v, want value", got)
		}
	})

	t.Run("explicit confirm parses supported values", func(t *testing.T) {
		cases := []struct {
			name   string
			params map[string]any
			want   bool
		}{
			{name: "nil_params", params: nil, want: false},
			{name: "bool_false", params: map[string]any{"confirm": false}, want: false},
			{name: "bool_true", params: map[string]any{"confirm": true}, want: true},
			{name: "padded_true_string", params: map[string]any{"confirm": " true "}, want: true},
			{name: "yes_string", params: map[string]any{"confirm": "yes"}, want: false},
			{name: "no_string", params: map[string]any{"confirm": "no"}, want: false},
			{name: "int_one", params: map[string]any{"confirm": 1}, want: false},
			{name: "int64_one", params: map[string]any{"confirm": int64(1)}, want: false},
			{name: "float_one", params: map[string]any{"confirm": 1.0}, want: false},
			{name: "int_two", params: map[string]any{"confirm": 2}, want: false},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				if got := hasExplicitConfirm(tt.params); got != tt.want {
					t.Fatalf("hasExplicitConfirm(%v) = %v, want %v", tt.params, got, tt.want)
				}
			})
		}
	})

	t.Run("an empty search names the query and what to try instead", func(t *testing.T) {
		want := "## GitLab Catalog: no matching action\n\n" +
			"- **Query**: `zzzz`\n" +
			wantHintsBlock(wantBroaderTermsHint)
		if got := formatSearchOutput(SearchOutput{Query: "zzzz"}); got != want {
			t.Fatalf("formatSearchOutput(empty) = %q, want %q", got, want)
		}
	})

	t.Run("an empty find names the query and what to try instead", func(t *testing.T) {
		want := "## GitLab Catalog: no matching action\n\n" +
			"- **Query**: `zzzz`\n" +
			wantHintsBlock(wantBroaderTermsHint)
		if got := formatFindOutput(FindOutput{Query: "zzzz"}, nil); got != want {
			t.Fatalf("formatFindOutput(empty) = %q, want %q", got, want)
		}
	})

	t.Run("find closes with the execute envelope the caller has to send", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n" +
			"- **Query**: `release link create package asset`\n\n" +
			"| Action ID | Score | Destructive | Required Params |\n" +
			"| --- | --- | --- | --- |\n" +
			"| `release.link_create_batch` | 0 | - | `project_id`, `tag_name`, `links` |\n" +
			wantHintsBlock(wantExecuteNowHint, dynamicExecuteEnvelopeHint, wantStructuredResultsHint)
		got := formatFindOutput(FindOutput{
			Query: "release link create package asset",
			Count: 1,
			Results: []FindResult{{
				ID:             "release.link_create_batch",
				RequiredParams: []string{"project_id", "tag_name", "links"},
			}},
		}, nil)
		if got != want {
			t.Fatalf("formatFindOutput() = %q, want %q", got, want)
		}
	})
}

// TestCopyAnnotations_CopiesBase verifies that the returned annotations are a
// distinct value carrying the base hints, so a caller mutating the result
// cannot reach the shared annotation singletons the dynamic tools register
// from.
func TestCopyAnnotations_CopiesBase(t *testing.T) {
	base := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	got := copyAnnotations(base)
	if got == nil || !got.ReadOnlyHint || !got.IdempotentHint {
		t.Fatalf("copyAnnotations(base) = %+v, want copied read-only annotation", got)
	}
	if got == base {
		t.Fatal("copyAnnotations returned the base pointer instead of a copy")
	}
	got.ReadOnlyHint = false
	if !base.ReadOnlyHint {
		t.Fatal("mutating the copy changed the base annotations")
	}
}

// TestAddServiceAccountActionTags_TagShapes verifies that service account action
// tags include the expected resource identifier prefix and verb-specific tags.
func TestAddServiceAccountActionTags_TagShapes(t *testing.T) {
	cases := []struct {
		domain string
		action string
		want   []string
	}{
		{
			domain: "group",
			action: "service_account_list",
			want:   []string{"group service account", "group service accounts", "group service account list", "list group service accounts"},
		},
		{
			domain: "project",
			action: "service_account_create",
			want:   []string{"project service account", "project service accounts", "project service account create", "create project service account"},
		},
		{
			domain: "group",
			action: "service_account_update",
			want:   []string{"group service account", "group service accounts", "group service account update", "update group service account"},
		},
		{
			domain: "group",
			action: "service_account_delete",
			want:   []string{"group service account", "group service accounts", "group service account delete", "delete group service account"},
		},
		{
			// Unknown action still produces base resource tags.
			domain: "project",
			action: "service_account_unknown",
			want:   []string{"project service account", "project service accounts"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.domain+"/"+tc.action, func(t *testing.T) {
			var got []string
			add := func(values ...string) { got = append(got, values...) }
			addServiceAccountActionTags(add, tc.domain, tc.action)
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Fatalf("addServiceAccountActionTags() tags = %v, want %q", got, want)
				}
			}
		})
	}
}

// TestAddServiceAccountPATActionTags_TagShapes verifies the AddServiceAccountPATActionTags_TagShapes handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddServiceAccountPATActionTags_TagShapes(t *testing.T) {
	cases := []struct {
		domain string
		action string
		want   []string
	}{
		{
			domain: "group",
			action: "service_account_pat_rotate",
			want:   []string{"group service account personal access token", "group service account pat rotate"},
		},
		{
			domain: "project",
			action: "service_account_pat_list",
			want:   []string{"project service account personal access token", "project service account pat list"},
		},
		{
			// No recognized verb suffix — still produces base tags.
			domain: "group",
			action: "service_account_pat",
			want:   []string{"group service account personal access token"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.domain+"/"+tc.action, func(t *testing.T) {
			var got []string
			add := func(values ...string) { got = append(got, values...) }
			addServiceAccountPATActionTags(add, tc.domain, tc.action)
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Fatalf("addServiceAccountPATActionTags() tags = %v, want %q", got, want)
				}
			}
		})
	}
}

// TestAddServiceAccountPATActionTags_StopsAtTheBaseTagsWithoutAVerb verifies
// that an action with no service_account_pat_ verb suffix, and one whose suffix
// is empty, both stop after the base tags.
//
// The verb is spliced into five phrases, so going on without one publishes tags
// with a hole in them ("group service account pat ") and phrases that begin
// with a blank word, which are matched by queries they have nothing to do with.
func TestAddServiceAccountPATActionTags_StopsAtTheBaseTagsWithoutAVerb(t *testing.T) {
	base := []string{
		"group service account personal access token",
		"group service account personal access tokens",
		"group service account pat",
		"group service account token",
	}
	cases := []struct {
		name   string
		action string
	}{
		{name: "no verb suffix at all", action: "service_account_pat"},
		{name: "an empty verb suffix", action: "service_account_pat_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			add := func(values ...string) { got = append(got, values...) }
			addServiceAccountPATActionTags(add, "group", tc.action)
			if !slices.Equal(got, base) {
				t.Fatalf("addServiceAccountPATActionTags(%q) tags = %v, want only the base tags %v", tc.action, got, base)
			}
		})
	}
}

// TestScoreIntentFunctions_ReturnFalseForNonMatchingEntries verifies that each
// intent-scoring helper returns zero when the entry does not match the domain or
// action it targets.
func TestScoreIntentFunctions_ReturnFalseForNonMatchingEntries(t *testing.T) {
	unrelated := actionEntry{
		Domain: "issue",
		Action: "list",
		Document: searchDocument{
			Domain:      "issue",
			Action:      "list",
			CanonicalID: "issue.list",
		},
	}
	unrelated.Document.DomainWords = splitSearchFieldWords("issue")
	unrelated.Document.ActionWords = splitSearchFieldWords("list")

	terms := normalizeSearchTerms("compare refs release list security review discover project search projects")

	if v := scoreCompareRefsIntentValue(unrelated, terms); v != 0 {
		t.Fatalf("scoreCompareRefsIntentValue(unrelated) = %d, want 0", v)
	}
	if v := scoreReleaseListIntentValue(unrelated, terms); v != 0 {
		t.Fatalf("scoreReleaseListIntentValue(unrelated) = %d, want 0", v)
	}
	if v := scoreProjectGetIntentValue(unrelated, terms); v != 0 {
		t.Fatalf("scoreProjectGetIntentValue(unrelated) = %d, want 0", v)
	}
	if v := scoreSearchProjectsIntentValue(unrelated, terms); v != 0 {
		t.Fatalf("scoreSearchProjectsIntentValue(unrelated) = %d, want 0", v)
	}
	if v := scoreServiceAccountIntentValue(unrelated, terms); v != 0 {
		t.Fatalf("scoreServiceAccountIntentValue(unrelated) = %d, want 0", v)
	}

	// Test the (int, MatchReason) variants return zero and empty reason.
	if score, reason := scoreCompareRefsIntent(unrelated, terms); score != 0 || reason != (MatchReason{}) {
		t.Fatalf("scoreCompareRefsIntent(unrelated) = %d, %v, want 0, empty", score, reason)
	}
	if score, reason := scoreReleaseListIntent(unrelated, terms); score != 0 || reason != (MatchReason{}) {
		t.Fatalf("scoreReleaseListIntent(unrelated) = %d, %v, want 0, empty", score, reason)
	}
	if score, reason := scoreProjectGetIntent(unrelated, terms); score != 0 || reason != (MatchReason{}) {
		t.Fatalf("scoreProjectGetIntent(unrelated) = %d, %v, want 0, empty", score, reason)
	}
	if score, reason := scoreSearchProjectsIntent(unrelated, terms); score != 0 || reason != (MatchReason{}) {
		t.Fatalf("scoreSearchProjectsIntent(unrelated) = %d, %v, want 0, empty", score, reason)
	}
	if score, reason := scoreServiceAccountIntent(unrelated, terms); score != 0 || reason != (MatchReason{}) {
		t.Fatalf("scoreServiceAccountIntent(unrelated) = %d, %v, want 0, empty", score, reason)
	}
}

// TestScoreVerbIntentFor_AdjustsByWhatTheQueryAskedAndWhatTheActionDoes pins
// each verb-intent branch at the value it adjusts by, and pins that an
// adjustment of zero comes back with no reason attached.
//
// This is the adjustment that keeps "show me the projects" from surfacing
// project.delete, so the sizes are the behavior: a read intent penalizes a
// destructive action by the full penalty and a merely writing one by half,
// which is what lets an update still rank behind a read without being buried
// under it. The diagnostic branch answers to two different tests because a
// diagnostic action is not always a read one: "lint" and "health" carry no
// read verb, and a branch that required both would leave a query for a broken
// pipeline ranking the lint action no higher than anything else.
func TestScoreVerbIntentFor_AdjustsByWhatTheQueryAskedAndWhatTheActionDoes(t *testing.T) {
	t.Parallel()
	destructive := scoringEntry("project.delete", "project", "delete")
	destructive.Destructive = true

	cases := []struct {
		name   string
		entry  actionEntry
		intent verbIntent
		want   int
	}{
		{name: "a query with no verb at all", entry: scoringEntry("project.get", "project", "get")},
		{name: "a read intent on a read action", entry: scoringEntry("project.get", "project", "get"), intent: verbIntentRead, want: scoreVerbIntentBoost},
		{name: "a read intent on a writing action", entry: scoringEntry("project.create", "project", "create"), intent: verbIntentRead, want: scoreVerbIntentPenalty / 2},
		{name: "a read intent on a destructive action", entry: destructive, intent: verbIntentRead, want: scoreVerbIntentPenalty},
		{name: "a write intent on a writing action", entry: scoringEntry("project.create", "project", "create"), intent: verbIntentWrite, want: scoreVerbIntentBoost},
		{name: "a write intent on a read action", entry: scoringEntry("project.get", "project", "get"), intent: verbIntentWrite},
		{name: "a workflow intent on a workflow action", entry: scoringEntry("pipeline.retry", "pipeline", "retry"), intent: verbIntentWorkflow, want: scoreVerbIntentBoost},
		{name: "a workflow intent on a read action", entry: scoringEntry("project.get", "project", "get"), intent: verbIntentWorkflow},
		{name: "a diagnostic intent on a diagnostic action that is no read", entry: scoringEntry("ci_lint.lint", "ci_lint", "lint"), intent: verbIntentDiagnostic, want: scoreVerbIntentBoost},
		{name: "a diagnostic intent on a read action", entry: scoringEntry("project.get", "project", "get"), intent: verbIntentDiagnostic, want: scoreVerbIntentBoost},
		{name: "a diagnostic intent on neither", entry: scoringEntry("project.create", "project", "create"), intent: verbIntentDiagnostic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			adjustment, reason := scoreVerbIntentFor(tc.entry, tc.intent, nil)
			if adjustment != tc.want {
				t.Fatalf("scoreVerbIntentFor(%q, %s) = %d, want %d", tc.entry.ID, tc.intent, adjustment, tc.want)
			}
			if tc.want == 0 {
				if reason != (MatchReason{}) {
					t.Fatalf("reason = %+v, want none for an adjustment of zero", reason)
				}
				return
			}
			if reason.Field != searchFieldVerbIntent || reason.Score != tc.want || reason.MatchedValue != tc.entry.Action {
				t.Fatalf("reason = %+v, want the verb intent field carrying %d for %q", reason, tc.want, tc.entry.Action)
			}
		})
	}
}

// TestScoreDestructiveVerbAdjustment_SeparatesTheNamedVerbsFromTheRest pins
// what a destructive query does to an action: nothing when the action destroys
// nothing, a penalty when the query names no resource the action is about, the
// plain boost for a destructive action, and the tripled boost for the three
// verbs a caller types when they mean exactly this action.
//
// Two facts about the first guard are worth keeping apart, because either
// alone would drop half the destructive catalog: an action can be marked
// destructive under a name that says nothing ("purge_all"), and an action can
// be named "delete" without the catalog marking it so. The query that names no
// resource is the one protecting a bare "delete" from ranking every destructive
// action in the catalog above everything else.
func TestScoreDestructiveVerbAdjustment_SeparatesTheNamedVerbsFromTheRest(t *testing.T) {
	t.Parallel()
	markedButNotNamed := scoringEntry("project.purge_all", "project", "purge_all")
	markedButNotNamed.Destructive = true

	cases := []struct {
		name  string
		entry actionEntry
		terms []searchTerm
		want  int
	}{
		{name: "an action that destroys nothing", entry: scoringEntry("project.get", "project", "get"), terms: searchTermsFromWords("delete", "project")},
		{name: "a destructive name the catalog did not mark", entry: scoringEntry("project.delete", "project", "delete"), terms: searchTermsFromWords("delete", "project"), want: scoreVerbIntentBoost * 3},
		{name: "a marked action under a name that says nothing", entry: markedButNotNamed, terms: searchTermsFromWords("purge", "project"), want: scoreVerbIntentBoost},
		{name: "a query naming no resource at all", entry: scoringEntry("project.delete", "project", "delete"), terms: searchTermsFromWords("zzzzzzzz"), want: scoreVerbIntentPenalty},
		{name: "remove", entry: scoringEntry("runner.remove", "runner", "remove"), terms: searchTermsFromWords("remove", "runner"), want: scoreVerbIntentBoost * 3},
		{name: "revoke", entry: scoringEntry("token.revoke", "token", "revoke"), terms: searchTermsFromWords("revoke", "token"), want: scoreVerbIntentBoost * 3},
		{name: "destroy, which is destructive but is not one of the three", entry: scoringEntry("widget.destroy", "widget", "destroy"), terms: searchTermsFromWords("destroy", "widget"), want: scoreVerbIntentBoost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scoreDestructiveVerbAdjustment(tc.entry, tc.terms, documentForEntry(tc.entry))
			if got != tc.want {
				t.Fatalf("scoreDestructiveVerbAdjustment(%q) = %d, want %d", tc.entry.ID, got, tc.want)
			}
		})
	}
}

// TestScoreRequiredParamSignals_WeighsARequiredParamAboveAnOptionalOne pins the
// parameter signal at both grains: the score the silent scorer adds and the
// reasons the explaining one reports beside it.
//
// A query that names a parameter is naming the call it wants to make, and a
// required parameter says so twice as loudly as an optional one, which is what
// separates issue.get from issue.list when the caller mentions an issue_iid.
// The reason carries the synonym it matched through only when that differs from
// what the caller typed, so an explanation never claims the caller wrote a word
// the expansion supplied.
func TestScoreRequiredParamSignals_WeighsARequiredParamAboveAnOptionalOne(t *testing.T) {
	t.Parallel()
	entry := actionEntry{Document: searchDocument{
		CanonicalID:    "issue.list",
		Domain:         "issue",
		Action:         "list",
		RequiredParams: []string{"project_id"},
		OptionalParams: []string{"search"},
	}}

	cases := []struct {
		name            string
		terms           []searchTerm
		want            int
		wantField       string
		wantMatched     string
		wantAlternative string
	}{
		{
			name:        "a required parameter the caller named",
			terms:       searchTermsFromWords("project_id"),
			want:        scoreRequiredParamBoost,
			wantField:   searchFieldRequiredParam,
			wantMatched: "project_id",
		},
		{
			name:        "an optional parameter the caller named",
			terms:       searchTermsFromWords("search"),
			want:        scoreRequiredParamBoost / 2,
			wantField:   searchFieldOptionalParam,
			wantMatched: "search",
		},
		{
			name:            "a required parameter reached through a synonym",
			terms:           []searchTerm{{Raw: "identifier", Alternatives: []string{"project_id"}}},
			want:            scoreRequiredParamBoost,
			wantField:       searchFieldRequiredParam,
			wantMatched:     "project_id",
			wantAlternative: "project_id",
		},
		{
			name:  "a term naming no parameter",
			terms: searchTermsFromWords("zzzzzzzz"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := scoreRequiredParamSignalValue(entry, tc.terms); got != tc.want {
				t.Fatalf("scoreRequiredParamSignalValue() = %d, want %d", got, tc.want)
			}
			total, reasons := scoreRequiredParamSignals(entry, tc.terms)
			if total != tc.want {
				t.Fatalf("scoreRequiredParamSignals() total = %d, want %d", total, tc.want)
			}
			if tc.want == 0 {
				if len(reasons) != 0 {
					t.Fatalf("reasons = %+v, want none", reasons)
				}
				return
			}
			if len(reasons) != 1 {
				t.Fatalf("reasons = %+v, want one", reasons)
			}
			want := MatchReason{
				Field:        tc.wantField,
				QueryTerm:    tc.terms[0].Raw,
				MatchedValue: tc.wantMatched,
				Alternative:  tc.wantAlternative,
				Score:        tc.want,
			}
			if reasons[0] != want {
				t.Fatalf("reason = %+v, want %+v", reasons[0], want)
			}
		})
	}
}

// TestCompactParamList_EdgeCases verifies all branches of compactParamList:
// empty params, within limit, exactly at limit, and truncated with overflow.
func TestCompactParamList_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		params []string
		limit  int
		want   string
	}{
		{
			name:   "empty params returns none",
			params: []string{},
			limit:  5,
			want:   "none",
		},
		{
			name:   "nil params returns none",
			params: nil,
			limit:  5,
			want:   "none",
		},
		{
			name:   "params within limit returns backtick list",
			params: []string{"project_id", "issue_iid"},
			limit:  5,
			want:   "`project_id`, `issue_iid`",
		},
		{
			name:   "params at limit returns full list",
			params: []string{"a", "b", "c"},
			limit:  3,
			want:   "`a`, `b`, `c`",
		},
		{
			name:   "params over limit truncates with and N more",
			params: []string{"a", "b", "c", "d"},
			limit:  2,
			want:   "`a`, `b`, and 2 more",
		},
		{
			name:   "limit zero returns full list",
			params: []string{"x", "y"},
			limit:  0,
			want:   "`x`, `y`",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := compactParamList(tc.params, tc.limit)
			if got != tc.want {
				t.Fatalf("compactParamList(%v, %d) = %q, want %q", tc.params, tc.limit, got, tc.want)
			}
		})
	}
}

// BenchmarkFind_PathologicalQuery measures the worst query a caller can now
// send: words that match nothing, so the lexical pass returns nothing, the
// fuzzy pass runs over the whole catalog, and the segmented pass runs one more
// pass per window.
//
// It exists because that cost was unbounded. A query was accepted at whatever
// length a 4 MiB request body allows, which is hundreds of thousands of words,
// and the cost grows with the word count times the size of the catalog. The
// case at [MaxSearchQueryLength] is the ceiling a caller can now reach, and it
// is here so a change to the scorer cannot quietly raise it: the shorter cases
// beside it are the shape of the growth.
func BenchmarkFind_PathologicalQuery(b *testing.B) {
	registry := benchmarkRegistry(b)
	ctx := context.Background()

	// Nonsense words of a realistic length, none of which is a catalog token,
	// so nothing prunes the candidate set at any stage.
	word := func(i int) string { return fmt.Sprintf("zqx%04dvfh", i) }
	queryOf := func(words int) string {
		parts := make([]string, 0, words)
		for i := range words {
			parts = append(parts, word(i))
		}
		return strings.Join(parts, " ")
	}
	// The longest query the tool accepts, built by filling to the character
	// bound rather than by counting words, since that bound is what a caller
	// is held to.
	atLimit := func() string {
		query := ""
		for i := 0; ; i++ {
			next := query
			if next != "" {
				next += " "
			}
			next += word(i)
			if len(next) > MaxSearchQueryLength {
				return query
			}
			query = next
		}
	}

	cases := []struct {
		name  string
		query string
	}{
		{name: "two_words", query: queryOf(2)},
		{name: "eight_words", query: queryOf(8)},
		{name: "at_the_length_limit", query: atLimit()},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			if overLength(tc.query) {
				b.Fatalf("benchmark query is %d characters, above the %d the tool accepts", utf8.RuneCountInString(tc.query), MaxSearchQueryLength)
			}
			input := FindInput{Query: tc.query, Limit: 20}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, _, err := registry.Find(ctx, nil, input); err != nil {
					b.Fatalf("Find() error: %v", err)
				}
			}
		})
	}
}

// BenchmarkSearch_BaselineMetaCatalog measures dynamic search throughput and
// allocations against the captured meta catalog plus standalone routes. It
// preserves the benchmark coverage migrated from register_benchmark_test.go.
func BenchmarkSearch_BaselineMetaCatalog(b *testing.B) {
	registry := benchmarkRegistry(b)
	ctx := context.Background()

	queries := []string{
		"merge request list open author project",
		"list open issues",
		"pipeline run trigger",
		"ci variable secret",
		"project delete",
		"discover project from remote",
		"merje requesy", // Known low-signal typo-heavy query kept in the baseline until fuzzy matching handles both misspelled terms.
	}
	allowZero := map[string]bool{
		// TODO(dynamic-search): remove this exception when fuzzy matching can recover both malformed terms.
		"merje requesy": true,
	}

	for _, query := range queries {
		b.Run(benchmarkName(query), func(b *testing.B) {
			input := SearchInput{Query: query, Limit: 20}
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				result, output, err := registry.Search(ctx, nil, input)
				if err != nil {
					b.Fatalf("Search() error: %v", err)
				}
				if result == nil || result.IsError {
					b.Fatalf("Search() result = %+v, want non-error", result)
				}
				if output.Count == 0 && !allowZero[query] {
					b.Fatalf("Search() output.Count = 0 for query %q", query)
				}
			}
		})
	}
}

// BenchmarkSearch_FieldAwareIndex compares indexed candidate scoring against
// the previous full-scan scoring path for representative lexical queries.
func BenchmarkSearch_FieldAwareIndex(b *testing.B) {
	registry := benchmarkRegistry(b)
	queries := []string{
		"merge request list open author project",
		"list open issues",
		"project delete",
	}

	ctx := b.Context()
	for _, query := range queries {
		terms := normalizeSearchTerms(query)
		b.Run(benchmarkName(query)+"/indexed", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				scored, err := registry.scoredMatches(ctx, terms, scoreEntryWithExplanation)
				if err != nil {
					b.Fatalf("scoredMatches(%q) error: %v", query, err)
				}
				matches := sortAndLimitMatches(scored, 20)
				if len(matches) == 0 {
					b.Fatalf("indexed search returned no matches for %q", query)
				}
			}
		})
		b.Run(benchmarkName(query)+"/full_scan", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				matches := sortAndLimitMatches(fullScanScoredMatches(registry.entries, terms, scoreEntryWithExplanation), 20)
				if len(matches) == 0 {
					b.Fatalf("full scan returned no matches for %q", query)
				}
			}
		})
	}
}

// benchmarkRegistry supports benchmark registry assertions in dynamic tests.
func benchmarkRegistry(b *testing.B) *Registry {
	b.Helper()

	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		b.Fatalf("BuildActionCatalog() error: %v", err)
	}
	catalog, err = AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
	if err != nil {
		b.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	registry := NewRegistryFromCatalog(catalog)
	if len(registry.entries) == 0 {
		b.Fatal("benchmark registry is empty")
	}

	b.Logf("benchmark registry entries: %d", len(registry.entries))
	return registry
}

// benchmarkName supports benchmark name assertions in dynamic tests.
func benchmarkName(query string) string {
	parts := strings.Fields(strings.ToLower(query))
	if len(parts) == 0 {
		return "empty"
	}
	return "q_" + strings.Join(parts, "_")
}

// logRanking renders the ranked head of a result set for failure diagnostics.
func logRanking(t *testing.T, query string, results []SearchResult) {
	t.Helper()
	t.Logf("query %q ranking:", query)
	for i, r := range results {
		t.Logf("  %d. %s (score=%d)", i+1, r.ID, r.Score)
	}
}

// TestIntentBoost_SearchCodeSurfacesForCodeQueries verifies that explicit code
// searches rank search.code first even when the query also names a project or
// repository path. These are the exact phrasings from surface-eval task MT-032,
// where match-ratio scaling previously buried search.code below search.projects
// and repository.tree.
func TestIntentBoost_SearchCodeSurfacesForCodeQueries(t *testing.T) {
	t.Parallel()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	reg := NewRegistryFromCatalog(catalog)
	queries := []string{
		"search code in project my-org/tools/gitlab-mcp-server for RegisterMCPMeta using project_id",
		"search code for func RegisterMCPMeta in project my-org/tools/gitlab-mcp-server",
		"search code contents for RegisterMCPMeta in repository my-org/tools/gitlab-mcp-server",
	}
	for _, query := range queries {
		t.Run(query[:min(len(query), 60)], func(t *testing.T) {
			t.Parallel()
			_, out, searchErr := reg.Search(context.Background(), nil, SearchInput{Query: query, Limit: 5})
			if searchErr != nil {
				t.Fatalf("search: %v", searchErr)
			}
			if len(out.Results) == 0 {
				t.Fatal("no results")
			}
			if out.Results[0].ID != "search.code" {
				logRanking(t, query, out.Results)
				t.Errorf("top result = %q, want search.code", out.Results[0].ID)
			}
		})
	}
}

// TestIntentBoost_CurrentUserSurfacesForIdentityQueries verifies user.current
// ranks first for current-user phrasings (surface-eval task MT-203). The
// canonical alias "current user" is multi-word and never reaches the
// exact-alias score on word-tokenized queries, so user.get and member-get
// actions previously outranked it.
func TestIntentBoost_CurrentUserSurfacesForIdentityQueries(t *testing.T) {
	t.Parallel()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	reg := NewRegistryFromCatalog(catalog)
	queries := []string{
		"current user info get",
		"get current user profile",
		"show the authenticated user account",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			_, out, searchErr := reg.Search(context.Background(), nil, SearchInput{Query: query, Limit: 5})
			if searchErr != nil {
				t.Fatalf("search: %v", searchErr)
			}
			if len(out.Results) == 0 {
				t.Fatal("no results")
			}
			if out.Results[0].ID != "user.current" {
				logRanking(t, query, out.Results)
				t.Errorf("top result = %q, want user.current", out.Results[0].ID)
			}
		})
	}
}

// TestIntentBoost_ControlsNotHijacked verifies the new explicit-intent boosts do
// not fire for queries whose intent is genuinely project search, get-user-by-id,
// or user listing. It asserts the boosted action is not promoted to the top,
// rather than a specific winner, so it stays robust to unrelated ranking
// changes.
func TestIntentBoost_ControlsNotHijacked(t *testing.T) {
	t.Parallel()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	reg := NewRegistryFromCatalog(catalog)
	cases := []struct {
		query     string
		forbidden string
	}{
		{"search projects named platform", "search.code"},
		{"find repositories matching backend", "search.code"},
		{"get user by id 42", "user.current"},
		{"list all users in the group", "user.current"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			_, out, searchErr := reg.Search(context.Background(), nil, SearchInput{Query: tc.query, Limit: 5})
			if searchErr != nil {
				t.Fatalf("search: %v", searchErr)
			}
			if len(out.Results) == 0 {
				t.Fatal("no results")
			}
			if out.Results[0].ID == tc.forbidden {
				logRanking(t, tc.query, out.Results)
				t.Errorf("query %q: top result must not be %q", tc.query, tc.forbidden)
			}
		})
	}
}

// TestIntentScorers_MatchReasonReturnedWhenIntentFires verifies that the
// intent scorer functions return a non-empty MatchReason when the intent
// signal fires. This covers the MatchReason construction paths that integration
// tests exercise implicitly but unit-coverage tools only see when called
// directly with a matching entry.
func TestIntentScorers_MatchReasonReturnedWhenIntentFires(t *testing.T) {
	t.Parallel()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	entries := NewRegistryFromCatalog(catalog).entries

	find := func(domain, action string) actionEntry {
		t.Helper()
		for _, e := range entries {
			d := documentForEntry(e)
			if d.Domain == domain && d.Action == action {
				return e
			}
		}
		t.Fatalf("entry %s.%s not found in catalog", domain, action)
		return actionEntry{}
	}

	t.Run("scoreSearchCodeIntent", func(t *testing.T) {
		t.Parallel()
		e := find("search", "code")
		terms := normalizeSearchTerms("search code in project my-org/tools for func Foo")
		score, reason := scoreSearchCodeIntent(e, terms)
		if score == 0 {
			t.Error("expected non-zero score for search.code with search+code signal")
		}
		if reason.MatchedValue == "" {
			t.Error("expected non-empty MatchReason.MatchedValue")
		}
	})

	t.Run("scoreCurrentUserIntent", func(t *testing.T) {
		t.Parallel()
		e := find("user", "current")
		terms := normalizeSearchTerms("current user profile")
		score, reason := scoreCurrentUserIntent(e, terms)
		if score == 0 {
			t.Error("expected non-zero score for user.current with current+user signal")
		}
		if reason.MatchedValue == "" {
			t.Error("expected non-empty MatchReason.MatchedValue")
		}
	})

	t.Run("scoreCurrentUserIntentValue_noIdentity", func(t *testing.T) {
		t.Parallel()
		e := find("user", "current")
		// "current" present but no identity noun → must return 0
		terms := normalizeSearchTerms("show current pipeline status")
		score := scoreCurrentUserIntentValue(e, terms)
		if score != 0 {
			t.Errorf("expected 0 for current without identity noun, got %d", score)
		}
	})
}

// scoringEntry builds a searchDocument-shaped actionEntry suitable for
// invoking intent-scoring helpers without requiring the full registry fixture.
// The Document is populated so documentForEntry returns the supplied fields.
func scoringEntry(canonicalID, domain, action string) actionEntry {
	entry := actionEntry{
		ID:     canonicalID,
		Domain: domain,
		Action: action,
		Document: searchDocument{
			CanonicalID: canonicalID,
			Domain:      domain,
			Action:      action,
		},
	}
	entry.Document.DomainWords = splitSearchFieldWords(domain)
	entry.Document.ActionWords = splitSearchFieldWords(action)
	entry.Document.IDWords = splitSearchFieldWords(canonicalID)
	return entry
}

// TestAddProtectionTags_NonGroupProtectedEnv verifies that protected
// environment actions outside the group domain still register the protected
// environment aliases (aliasProtectedEnvironment/aliasEnvironmentProtection)
// without falling through to the default branch.
func TestAddProtectionTags_NonGroupProtectedEnv(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		domain string
		action string
		want   []string
	}{
		{
			name:   "project protected environment uses alias only",
			id:     "project.protected_environment_list",
			domain: "project",
			action: "protected_environment_list",
			want:   []string{"protected environment", "environment protection"},
		},
		{
			name:   "branch protect action",
			id:     "branch.protect",
			domain: "branch",
			action: "protect",
			want:   []string{"protected branch", "branch protection"},
		},
		{
			name:   "branch get_protected action",
			id:     "branch.get_protected",
			domain: "branch",
			action: "get_protected",
			want:   []string{"protected branch", "branch protection"},
		},
		{
			name:   "branch unprotect action",
			id:     "branch.unprotect",
			domain: "branch",
			action: "unprotect",
			want:   []string{"protected branch", "branch protection"},
		},
		{
			name:   "branch update_protected action",
			id:     "branch.update_protected",
			domain: "branch",
			action: "update_protected",
			want:   []string{"protected branch", "branch protection"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			add := func(values ...string) { got = append(got, values...) }
			if !addProtectionTags(add, tc.id, tc.domain, tc.action) {
				t.Fatalf("addProtectionTags() returned false, want true for id %q", tc.id)
			}
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Fatalf("addProtectionTags() tags = %v, want %q", got, want)
				}
			}
		})
	}
}

// actionTaggerCase describes one branch of an action tagger: the identifiers it
// is handed, whether the tagger should claim them, and tags that must or must
// not come out of it.
type actionTaggerCase struct {
	name    string
	id      string
	domain  string
	action  string
	matched bool
	want    []string
	absent  []string
}

// runActionTaggerCases drives one tagger over its cases.
//
// Each tagger claims an action by returning true, which stops the chain, so
// what it claims matters as much as what it tags: a tagger that claims an
// action belonging to a later one silently takes that action's tags away.
func runActionTaggerCases(t *testing.T, tagger actionTagger, cases []actionTaggerCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			matched := tagger(func(values ...string) { got = append(got, values...) }, tc.id, tc.domain, tc.action)
			if matched != tc.matched {
				t.Fatalf("tagger(%q, %q, %q) claimed = %t, want %t (tags %v)", tc.id, tc.domain, tc.action, matched, tc.matched, got)
			}
			if !matched && len(got) != 0 {
				t.Errorf("tags = %v, want none from a tagger that did not claim the action", got)
			}
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Errorf("tags = %v, want %q", got, want)
				}
			}
			for _, absent := range tc.absent {
				if slices.Contains(got, absent) {
					t.Errorf("tags = %v, want %q absent", got, absent)
				}
			}
		})
	}
}

// TestTagAppender_NormalizesValuesAndDropsBlankOnes verifies the collector every
// tagger writes through: it trims and lowercases each value and keeps nothing
// that is blank.
//
// Tags are matched against a lowercased query, so an untrimmed or capitalized
// tag matches nothing, and a blank one sits in the index costing a comparison
// per search while matching nothing either.
func TestTagAppender_NormalizesValuesAndDropsBlankOnes(t *testing.T) {
	var tags []string
	add := tagAppender(&tags)

	add("  Protected Environment ", "", "   ", "MR")

	if !slices.Equal(tags, []string{"protected environment", "mr"}) {
		t.Fatalf("tagAppender() tags = %v, want the normalized non-blank values", tags)
	}
}

// TestAddIDPatternTags_ClaimsOnlyTheIDShapesItRecognizes verifies the tagger
// keyed on the canonical ID: each pattern claims its own actions, and the
// scoped ones claim them only in the domain they name.
//
// Two of these branches read an ID pattern AND a domain, which is what keeps a
// group member action out of the project member phrases and a service account
// action out of both when it belongs to neither scope. Losing either half sends
// a search for "group membership" to the project action.
func TestAddIDPatternTags_ClaimsOnlyTheIDShapesItRecognizes(t *testing.T) {
	runActionTaggerCases(t, addIDPatternTags, []actionTaggerCase{
		{name: "hook id", id: "project.hook_add", domain: "project", action: "hook_add", matched: true, want: []string{"webhook"}},
		{name: "deploy key id", id: "project.deploy_key_list", domain: "project", action: "deploy_key_list", matched: true, want: []string{"deploy key"}},
		{name: "deploy token id", id: "project.deploy_token_list", domain: "project", action: "deploy_token_list", matched: true, want: []string{"deploy token"}},
		{name: "member id in the project domain", id: "project.member_list", domain: "project", action: "member_list", matched: true, want: []string{"project member"}, absent: []string{"group member"}},
		{name: "member id in the group domain", id: "group.member_list", domain: "group", action: "member_list", matched: true, want: []string{"group member"}, absent: []string{"project member"}},
		{name: "member id in an unrelated domain", id: "epic.member_list", domain: "epic", action: "member_list"},
		{name: "service account pat id", id: "group.service_account_pat_rotate", domain: "group", action: "service_account_pat_rotate", matched: true, want: []string{"group service account pat rotate"}},
		{name: "service account id in the project domain", id: "project.service_account_list", domain: "project", action: "service_account_list", matched: true, want: []string{"project service account"}},
		{name: "service account id in the group domain", id: "group.service_account_list", domain: "group", action: "service_account_list", matched: true, want: []string{"group service account"}},
		{name: "service account id in an unrelated domain", id: "user.service_account_list", domain: "user", action: "service_account_list"},
		{name: "discover project domain", id: "discover_project.resolve", domain: "discover_project", action: "resolve", matched: true, want: []string{"project discovery"}},
		{name: "interactive domain with a known flow", id: "interactive.release_create", domain: "interactive", action: "release_create", matched: true, want: []string{"wizard", "guided release creation"}},
		{name: "interactive domain with an unknown flow", id: "interactive.epic_create", domain: "interactive", action: "epic_create", matched: true, want: []string{"wizard", "epic create"}, absent: []string{"guided release creation"}},
		{name: "access token id", id: "project.access_token_project_create", domain: "project", action: "access_token_project_create", matched: true, want: []string{"project access token"}},
		{name: "a project action with no id pattern", id: "project.star", domain: "project", action: "star"},
		{name: "a group action with no id pattern", id: "group.epic_list", domain: "group", action: "epic_list"},
	})
}

// TestAddCoreDomainTags_ClaimsOnlyItsOwnDomains verifies the tagger keyed on the
// core domains: each domain claims its own actions, and the two branches that
// also read the action claim only the action they name.
//
// Every case that must not be claimed is a real risk here rather than a
// hypothetical one: this tagger runs second, so an action it claims by mistake
// never reaches the environment, release, package or protection taggers below
// it and loses every tag they would have given it.
func TestAddCoreDomainTags_ClaimsOnlyItsOwnDomains(t *testing.T) {
	runActionTaggerCases(t, addCoreDomainTags, []actionTaggerCase{
		{name: "the current user action", id: "user.current", domain: "user", action: "current", matched: true, want: []string{"whoami"}},
		{name: "a user action that is not current", id: "user.list", domain: "user", action: "list"},
		{name: "a current action in another domain", id: "project.current", domain: "project", action: "current", matched: true, absent: []string{"whoami"}},
		{name: "project star", id: "project.star", domain: "project", action: "star", matched: true, want: []string{"star project"}},
		{name: "project unstar", id: "project.unstar", domain: "project", action: "unstar", matched: true, want: []string{"unstar project"}},
		{name: "a repository file action", id: "repository.file_get", domain: "repository", action: "file_get", matched: true, want: []string{"repository file"}, absent: []string{"repository tree"}},
		{name: "the repository tree action", id: "repository.tree", domain: "repository", action: "tree", matched: true, want: []string{"repository tree"}, absent: []string{"repository file"}},
		{name: "a repository action that is neither", id: "repository.compare", domain: "repository", action: "compare"},
		{name: "a file action in another domain", id: "snippet.file_get", domain: "snippet", action: "file_get"},
		{name: "a tree action in another domain", id: "group.tree", domain: "group", action: "tree"},
		{name: "the projects search action", id: "search.projects", domain: "search", action: "projects", matched: true, want: []string{"search projects"}},
		{name: "a search action with no phrases of its own", id: "search.blobs", domain: "search", action: "blobs", matched: true, absent: []string{"search projects"}},
		{name: "the server health check action", id: "server.health_check", domain: "server", action: "health_check", matched: true, want: []string{"health check"}},
		{name: "the server status action", id: "server.status", domain: "server", action: "status", matched: true, want: []string{"server status"}},
		{name: "a server action with no phrases of its own", id: "server.version", domain: "server", action: "version", matched: true, absent: []string{"server status", "health check"}},
		{name: "the ci catalog list action", id: "ci_catalog.list", domain: "ci_catalog", action: "list", matched: true, want: []string{"ci catalog"}},
		{name: "merge request domain", id: "merge_request.list", domain: "merge_request", action: "list", matched: true, want: []string{"mr", aliasMergeRequest}},
		{name: "merge request review domain", id: "mr_review.changes_get", domain: "mr_review", action: "changes_get", matched: true, want: []string{"mr changes"}},
		{name: "ci variable domain", id: "ci_variable.create", domain: "ci_variable", action: "create", matched: true, want: []string{"ci variable", "project ci variable"}},
		{name: "a domain this tagger does not own", id: "snippet.list", domain: "snippet", action: "list"},
	})
}

// TestAddEnvironmentAndCITags_ClaimsOnlyItsOwnDomains verifies the environment,
// feature flag, job and pipeline branches, including the two that read the
// action as well as the domain.
//
// The pipeline schedule phrases need both halves of the action name: a schedule
// action that is not about a variable must not be tagged "schedule variable",
// or every schedule query ranks the variable actions alongside the ones the
// caller meant.
func TestAddEnvironmentAndCITags_ClaimsOnlyItsOwnDomains(t *testing.T) {
	runActionTaggerCases(t, addEnvironmentAndCITags, []actionTaggerCase{
		{name: "a protected environment action", id: "environment.protected_protect", domain: "environment", action: "protected_protect", matched: true, want: []string{"env", aliasProtectedEnvironment, "protect environment"}},
		{name: "a protected environment action with no phrases of its own", id: "environment.protected_refresh", domain: "environment", action: "protected_refresh", matched: true, want: []string{aliasProtectedEnvironment}, absent: []string{"unprotect environment"}},
		{name: "an environment deployment action", id: "environment.deployment_list", domain: "environment", action: "deployment_list", matched: true, want: []string{"environment deployment"}},
		{name: "a feature flag user list action", id: "feature_flags.ff_user_list_get", domain: "feature_flags", action: "ff_user_list_get", matched: true, want: []string{"feature flag user list"}},
		{name: "a feature flag action that is not a user list", id: "feature_flags.list", domain: "feature_flags", action: "list"},
		{name: "a user list action in another domain", id: "project.ff_user_list_get", domain: "project", action: "ff_user_list_get"},
		{name: "a job action", id: "job.artifacts", domain: "job", action: "artifacts", matched: true, want: []string{"ci job", "whole artifact archive"}},
		{name: "a pipeline trigger action", id: "pipeline.trigger_create", domain: "pipeline", action: "trigger_create", matched: true, want: []string{"ci pipeline", "pipeline trigger", "pipeline trigger create"}},
		{name: "a pipeline schedule variable action", id: "pipeline.schedule_create_variable", domain: "pipeline", action: "schedule_create_variable", matched: true, want: []string{"pipeline schedule variable"}},
		{name: "a pipeline schedule action that is not about a variable", id: "pipeline.schedule_list", domain: "pipeline", action: "schedule_list", matched: true, want: []string{"ci pipeline"}, absent: []string{"pipeline schedule variable"}},
		{name: "a domain this tagger does not own", id: "snippet.list", domain: "snippet", action: "list"},
	})
}

// TestAddAdminReleaseTags_ClaimsOnlyItsOwnDomains verifies the admin, tag,
// release and repository-compare branches.
//
// The compare branch is the one that reads both the domain and the action: the
// phrases it adds are about comparing two refs of a repository, and a tree or a
// branch action tagged with them would be offered for "diff between refs".
func TestAddAdminReleaseTags_ClaimsOnlyItsOwnDomains(t *testing.T) {
	runActionTaggerCases(t, addAdminReleaseTags, []actionTaggerCase{
		{name: "the admin settings action", id: "admin.settings_get", domain: "admin", action: "settings_get", matched: true, want: []string{"instance settings"}},
		{name: "an admin action with no phrases of its own", id: "admin.user_list", domain: "admin", action: "user_list", matched: true, absent: []string{"instance settings"}},
		{name: "the tag get action", id: "tag.get", domain: "tag", action: "get", matched: true, want: []string{"verify tag"}},
		{name: "a tag action that is not get", id: "tag.list", domain: "tag", action: "list", matched: true, absent: []string{"verify tag"}},
		{name: "the release create action", id: "release.create", domain: "release", action: "create", matched: true, want: []string{"create release"}},
		{name: "the repository compare action", id: "repository.compare", domain: "repository", action: "compare", matched: true, want: []string{"compare refs"}},
		{name: "a repository action that is not compare", id: "repository.tree", domain: "repository", action: "tree"},
		{name: "a compare action in another domain", id: "branch.compare", domain: "branch", action: "compare"},
		{name: "a domain this tagger does not own", id: "snippet.list", domain: "snippet", action: "list"},
	})
}

// TestAddProtectionTags_ClaimsOnlyGroupAndProtectionShapes verifies the two
// group-scoped protection branches and the member role branch.
//
// Both group branches read the domain and the ID together. The group phrases
// say "group" in them, so claiming a project action here would publish tags
// that name the wrong scope, and claiming a group action that is about neither
// protection would publish them for an action with no protection at all.
//
// The branch domain is read together with a list of four actions for the same
// reason: branch.list is a branch action about no protection at all, and
// tagging it "protected branch" would offer it for every query about branch
// protection.
//
// The last case is a group protected branch action the per-action phrase table
// does not name. The table lists the five actions the catalog carries today, so
// nothing in it could ever fail to match while that stays true; a sixth action
// must still receive the domain phrases and none of another action's, which is
// what would break if the last entry of the table were read as a default.
func TestAddProtectionTags_ClaimsOnlyGroupAndProtectionShapes(t *testing.T) {
	runActionTaggerCases(t, addProtectionTags, []actionTaggerCase{
		{name: "a group protected branch action", id: "group.protected_branch_list", domain: "group", action: "protected_branch_list", matched: true, want: []string{"group protected branch", "list group protected branches"}},
		{name: "a group protected branch action the phrase table does not name", id: "group.protected_branch_rename", domain: "group", action: "protected_branch_rename", matched: true, want: []string{"group protected branch"}, absent: []string{"unprotect group branch", "remove group protected branch"}},
		{name: "a protected branch id in another domain", id: "project.protected_branch_list", domain: "project", action: "protected_branch_list"},
		{name: "a group protected environment action", id: "group.protected_env_list", domain: "group", action: "protected_env_list", matched: true, want: []string{"group protected environment"}},
		{name: "a group action about neither protection", id: "group.epic_list", domain: "group", action: "epic_list"},
		{name: "a branch action about no protection", id: "branch.list", domain: "branch", action: "list"},
		{name: "a member role id", id: "member_role.create", domain: "member_role", action: "create", matched: true, want: []string{"custom role"}},
	})
}

// TestServiceAccountQueryVerb_AllVerbs verifies serviceAccountQueryVerb
// returns the first recognized verb from a query and the empty string when
// none of the supported verbs are present.
func TestServiceAccountQueryVerb_AllVerbs(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "create verb", query: "service account create", want: "create"},
		{name: "list verb", query: "service account list", want: "list"},
		{name: "update verb", query: "service account update", want: "update"},
		{name: "delete verb", query: "service account delete", want: "delete"},
		{name: "rotate verb", query: "service account rotate", want: "rotate"},
		{name: "revoke verb matches as delete synonym (verb ordering)", query: "service account revoke", want: "delete"},
		{name: "no verb", query: "service account", want: ""},
		// "list" appears in the verb list before "delete", so a generic "show" query still maps
		// to "list" via the searchTermsContainWord alternative-walk. The function intentionally
		// returns the first matching verb regardless of whether the query was service-account scoped.
		{name: "no service account context with verb still returns verb", query: "project list", want: "list"},
		// "rotate" verb in a non-service-account context still returns "rotate" (no gate).
		{name: "rotate verb in non service account context", query: "rotate token", want: "rotate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serviceAccountQueryVerb(normalizeSearchTerms(tc.query)); got != tc.want {
				t.Fatalf("serviceAccountQueryVerb(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

// TestAdjustServiceAccountVerbScores_AllBranches verifies the verb-score
// adjuster covers the mismatch path (negative score floored at zero), the
// matched-verb boost path, the explanation.TotalScore update path, the
// no-verb early return, and the no-service-account-context early return.
func TestAdjustServiceAccountVerbScores_AllBranches(t *testing.T) {
	entry := scoringEntry("group.service_account_list", "group", "service_account_list")
	makeMatches := func(scores ...int) []scoredActionEntry {
		out := make([]scoredActionEntry, 0, len(scores))
		for _, s := range scores {
			out = append(out, scoredActionEntry{
				entry: entry,
				score: s,
				explanation: ScoringExplanation{
					TotalScore: s,
				},
			})
		}
		return out
	}

	t.Run("no service account context returns matches unchanged", func(t *testing.T) {
		matches := makeMatches(10, 20)
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("project list"))
		if len(got) != 2 || got[0].score != 10 || got[1].score != 20 {
			t.Fatalf("adjustServiceAccountVerbScores() = %+v, want unchanged scores", got)
		}
	})

	t.Run("service account query with no recognized verb returns matches unchanged", func(t *testing.T) {
		matches := makeMatches(15)
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account"))
		if got[0].score != 15 {
			t.Fatalf("adjustServiceAccountVerbScores() score = %d, want 15 (unchanged)", got[0].score)
		}
	})

	t.Run("mismatched verb subtracts boost and floors at zero", func(t *testing.T) {
		matches := makeMatches(50) // 50 - 2*80 = -110 -> 0
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account delete"))
		if got[0].score != 0 {
			t.Fatalf("adjustServiceAccountVerbScores() floored score = %d, want 0", got[0].score)
		}
	})

	t.Run("mismatched verb subtracts boost without flooring when result stays positive", func(t *testing.T) {
		matches := makeMatches(200) // 200 - 2*80 = 40
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account delete"))
		if got[0].score != 40 {
			t.Fatalf("adjustServiceAccountVerbScores() score = %d, want 40", got[0].score)
		}
	})

	t.Run("matched verb adds boost", func(t *testing.T) {
		matches := makeMatches(50)
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account list"))
		if got[0].score != 130 {
			t.Fatalf("adjustServiceAccountVerbScores() score = %d, want 130", got[0].score)
		}
	})

	t.Run("non-service-account entry is skipped", func(t *testing.T) {
		other := scoringEntry("project.list", "project", "list")
		matches := []scoredActionEntry{{entry: other, score: 42}}
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account list"))
		if got[0].score != 42 {
			t.Fatalf("adjustServiceAccountVerbScores() score = %d, want 42 (skipped)", got[0].score)
		}
	})

	t.Run("action verb empty is skipped (unrecognized suffix)", func(t *testing.T) {
		unknown := scoringEntry("group.service_account_unknown", "group", "service_account_unknown")
		matches := []scoredActionEntry{{entry: unknown, score: 50}}
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account list"))
		if got[0].score != 50 {
			t.Fatalf("adjustServiceAccountVerbScores() score = %d, want 50 (skipped)", got[0].score)
		}
	})

	t.Run("a filled explanation follows the adjusted score", func(t *testing.T) {
		matches := makeMatches(50)
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account list"))
		if got[0].explanation.TotalScore != got[0].score {
			t.Fatalf("explanation.TotalScore = %d, want the adjusted score %d", got[0].explanation.TotalScore, got[0].score)
		}
	})

	// A search that was not asked to explain itself scores through
	// scoreEntryWithoutExplanation, which leaves the explanation zero. Writing
	// the adjusted score into it here would publish a total with no reasons
	// under it, and Search reports an explanation whenever one carries a score.
	t.Run("an explanation nobody asked for stays empty", func(t *testing.T) {
		matches := []scoredActionEntry{{entry: entry, score: 50}}
		got := adjustServiceAccountVerbScores(matches, normalizeSearchTerms("service account list"))
		if got[0].score != 130 {
			t.Fatalf("adjustServiceAccountVerbScores() score = %d, want 130", got[0].score)
		}
		if got[0].explanation.TotalScore != 0 || len(got[0].explanation.Reasons) != 0 {
			t.Fatalf("explanation = %+v, want it left empty", got[0].explanation)
		}
	})
}

// TestMinimumMatchedTermCount_EdgeCases verifies the minimum-match threshold
// edges: short queries, queries with no compound tags, queries with compound
// tags that lower the requirement, and the floor at 1.
func TestMinimumMatchedTermCount_EdgeCases(t *testing.T) {
	// Entry that shares a compound tag with the terms (e.g., "group service account").
	entry := actionEntry{
		ID: "group.service_account_list",
		Document: searchDocument{
			CanonicalID: "group.service_account_list",
			Tags:        []string{"group service account"},
		},
	}

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{name: "empty query returns 1", query: "", want: 1},
		{name: "single term returns 1", query: "service", want: 1},
		{name: "two terms returns 2", query: "service account", want: 2},
		{name: "three terms no compound returns 2", query: "service account list", want: 2},
		// A compound tag lowers the requirement only above three terms: at three
		// it would drop to one, and a single matched term is not an intent.
		{name: "three terms with compound returns 2", query: "group service account", want: 2},
		{name: "four terms no compound returns 3", query: "service account list all", want: 3},
		{name: "four terms with compound returns 2", query: "group service account list", want: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := minimumMatchedTermCount(entry, normalizeSearchTerms(tc.query))
			if got != tc.want {
				t.Fatalf("minimumMatchedTermCount() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestMatchedCompoundTagCount_CountsTagsOfTwoWordsUpwards verifies which tags
// count as the compound evidence that lowers the minimum-matched-term
// threshold: a tag of at least two words, every one of which the query carries.
//
// Two words is where a tag stops being a single token that a long query could
// brush against by accident and starts being a phrase the caller wrote, which
// is the whole warrant for lowering the threshold. Counting one-word tags would
// lower it for any query mentioning "group"; not counting two-word ones would
// leave the commonest phrases in the catalog ("merge request", "service
// account") carrying no evidence at all.
func TestMatchedCompoundTagCount_CountsTagsOfTwoWordsUpwards(t *testing.T) {
	t.Parallel()
	entry := actionEntry{Document: searchDocument{
		CanonicalID: "group.service_account_list",
		Tags:        []string{"group", "service account", "group service account"},
	}}

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{name: "the two-word tag alone", query: "service account list", want: 1},
		{name: "both multi-word tags", query: "group service account list", want: 2},
		{name: "a query carrying none of them", query: "project list", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := matchedCompoundTagCount(entry, normalizeSearchTerms(tc.query)); got != tc.want {
				t.Fatalf("matchedCompoundTagCount(%q) = %d, want %d", tc.query, got, tc.want)
			}
		})
	}
}

// TestScoreCompoundTagSignals_RepositoryCompareBoost verifies the
// repository.compare branch that adds scoreRequiredParamBoost on top of the
// regular compound tag boost when the tag contains compare/ref words.
func TestScoreCompoundTagSignals_RepositoryCompareBoost(t *testing.T) {
	entry := actionEntry{
		ID: "repository.compare",
		Document: searchDocument{
			CanonicalID: "repository.compare",
			Domain:      "repository",
			Action:      "compare",
			Tags:        []string{"compare refs", "diff refs"},
		},
	}
	terms := normalizeSearchTerms("compare refs")
	total, reasons := scoreCompoundTagSignals(entry, terms)
	if total == 0 {
		t.Fatal("scoreCompoundTagSignals() = 0, want positive score")
	}
	// At least one reason should have an elevated score (compound + required-param bonus).
	if reasons[0].Score != scoreCompoundTagBoost+scoreRequiredParamBoost {
		t.Fatalf("scoreCompoundTagSignals() first reason score = %d, want %d", reasons[0].Score,
			scoreCompoundTagBoost+scoreRequiredParamBoost)
	}
}

// TestScoreCompoundTagSignals_TheCompareBonusNeedsEveryPartOfItsCondition pins
// the four things that have to hold together before a matched compound tag is
// worth more than a compound tag: the repository domain, the compare action,
// a ref word in the tag, and the word compare in it.
//
// The bonus exists to settle one confusion, "diff between two refs" reaching
// repository.compare rather than a branch or tag listing, so each part is what
// keeps it from paying out somewhere it decides nothing: an action that lists
// branches in the repository domain, or a compare action in another domain,
// would take the bonus from the action the caller meant. The singular "ref"
// and the plural "refs" are both spelled because a tag may carry either, and
// the catalog happens to carry only the plural today, so nothing else would
// notice if the singular stopped being read.
//
// Both scorers are driven over the same table because they are the same rule
// written twice, once returning the total and once the reasons behind it.
func TestScoreCompoundTagSignals_TheCompareBonusNeedsEveryPartOfItsCondition(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		domain string
		action string
		tag    string
		query  string
		want   int
	}{
		{name: "the plural ref spelling", domain: "repository", action: "compare", tag: "compare refs", query: "compare refs", want: scoreCompoundTagBoost + scoreRequiredParamBoost},
		{name: "the singular ref spelling", domain: "repository", action: "compare", tag: "ref compare", query: "ref compare", want: scoreCompoundTagBoost + scoreRequiredParamBoost},
		{name: "a tag naming no ref", domain: "repository", action: "compare", tag: "compare branches", query: "compare branches", want: scoreCompoundTagBoost},
		{name: "a tag not naming the comparison", domain: "repository", action: "compare", tag: "diff refs", query: "diff refs", want: scoreCompoundTagBoost},
		{name: "the same tag in another domain", domain: "branch", action: "compare", tag: "compare refs", query: "compare refs", want: scoreCompoundTagBoost},
		{name: "the same tag on another action", domain: "repository", action: "list", tag: "compare refs", query: "compare refs", want: scoreCompoundTagBoost},
		{name: "a tag the query does not carry", domain: "repository", action: "compare", tag: "compare refs", query: "list branches", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entry := actionEntry{Document: searchDocument{
				CanonicalID: tc.domain + "." + tc.action,
				Domain:      tc.domain,
				Action:      tc.action,
				Tags:        []string{tc.tag},
			}}
			terms := normalizeSearchTerms(tc.query)

			if got := scoreCompoundTagSignalValue(entry, terms); got != tc.want {
				t.Fatalf("scoreCompoundTagSignalValue(%q on %s.%s) = %d, want %d", tc.tag, tc.domain, tc.action, got, tc.want)
			}
			total, reasons := scoreCompoundTagSignals(entry, terms)
			if total != tc.want {
				t.Fatalf("scoreCompoundTagSignals(%q on %s.%s) total = %d, want %d", tc.tag, tc.domain, tc.action, total, tc.want)
			}
			if tc.want == 0 {
				if len(reasons) != 0 {
					t.Fatalf("reasons = %+v, want none", reasons)
				}
				return
			}
			if len(reasons) != 1 || reasons[0].Score != tc.want || reasons[0].MatchedValue != tc.tag {
				t.Fatalf("reasons = %+v, want one carrying %d for %q", reasons, tc.want, tc.tag)
			}
		})
	}
}

// TestScoreServiceAccountIntent_PositiveCase verifies the scoring helpers
// return a positive score and a populated MatchReason for queries that match
// the service-account intent (with and without the PAT/verb bonuses).
func TestScoreServiceAccountIntent_PositiveCase(t *testing.T) {
	entry := scoringEntry("group.service_account_create", "group", "service_account_create")

	score, reason := scoreServiceAccountIntent(entry, normalizeSearchTerms("group service account create"))
	if score == 0 || reason == (MatchReason{}) {
		t.Fatalf("scoreServiceAccountIntent() = %d, %+v, want positive score with populated reason", score, reason)
	}
	if reason.Field != searchFieldServiceAccount {
		t.Fatalf("scoreServiceAccountIntent() field = %q, want %q", reason.Field, searchFieldServiceAccount)
	}
	if reason.QueryTerm != "service account" {
		t.Fatalf("scoreServiceAccountIntent() query term = %q, want %q", reason.QueryTerm, "service account")
	}

	// PAT case adds an extra boost.
	patEntry := scoringEntry("group.service_account_pat_create", "group", "service_account_pat_create")
	patScore := scoreServiceAccountIntentValue(patEntry, normalizeSearchTerms("service account personal access token create"))
	if patScore <= score {
		t.Fatalf("scoreServiceAccountIntentValue(pat) = %d, want greater than non-PAT score %d", patScore, score)
	}
}

// TestScoreServiceAccountIntentValue_AddsTheScopeBonusOnlyForTheDomainNamed
// verifies the scope half of the service-account score at the value it adds.
//
// Service accounts exist at group scope and at project scope under nearly the
// same names, so "group service account" and "project service account" are one
// word apart and the bonus for that word is what separates them. Paying it to
// an action whose scope the caller never named would flatten that difference
// back out, and withholding it from the one they did name would leave the two
// scopes tied.
func TestScoreServiceAccountIntentValue_AddsTheScopeBonusOnlyForTheDomainNamed(t *testing.T) {
	t.Parallel()
	entry := scoringEntry("group.service_account_list", "group", "service_account_list")

	// Boost for the intent, the scope bonus for naming the group, and a second
	// boost for the verb the action carries.
	const withScope = scoreServiceAccountBoost + scoreServiceAccountScope + scoreServiceAccountBoost
	if got := scoreServiceAccountIntentValue(entry, normalizeSearchTerms("group service account list")); got != withScope {
		t.Fatalf("scoreServiceAccountIntentValue(scoped query) = %d, want %d", got, withScope)
	}
	if got := scoreServiceAccountIntentValue(entry, normalizeSearchTerms("service account list")); got != withScope-scoreServiceAccountScope {
		t.Fatalf("scoreServiceAccountIntentValue(unscoped query) = %d, want %d", got, withScope-scoreServiceAccountScope)
	}
}

// TestScoreCompareRefsIntent_PositiveCase verifies the compare-refs scoring
// helper returns a positive score and a populated MatchReason for matching
// queries.
func TestScoreCompareRefsIntent_PositiveCase(t *testing.T) {
	entry := scoringEntry("repository.compare", "repository", "compare")

	score, reason := scoreCompareRefsIntent(entry, normalizeSearchTerms("compare refs"))
	if score == 0 || reason == (MatchReason{}) {
		t.Fatalf("scoreCompareRefsIntent() = %d, %+v, want positive score with populated reason", score, reason)
	}
	if reason.Field != searchFieldCompareIntent {
		t.Fatalf("scoreCompareRefsIntent() field = %q, want %q", reason.Field, searchFieldCompareIntent)
	}
	if reason.QueryTerm != "compare refs" {
		t.Fatalf("scoreCompareRefsIntent() query term = %q, want %q", reason.QueryTerm, "compare refs")
	}

	// "refs" plural alternative should still match.
	if scoreCompareRefsIntentValue(entry, normalizeSearchTerms("compare refs")) == 0 {
		t.Fatal("scoreCompareRefsIntentValue(refs) = 0, want positive score")
	}

	// The second guard in scoreCompareRefsIntentValue (the "ref"/"refs"
	// disambiguator check) is unreachable from normalizeSearchTerms-produced
	// terms specifically, because the "compare" entry in searchSynonymsMap
	// lists "ref"/"refs" as alternatives: any query containing the literal
	// word "compare" also satisfies the "ref"/"refs" check through that
	// synonym expansion. It is reachable, though, by any caller that
	// constructs a []searchTerm directly instead of going through
	// normalizeSearchTerms' synonym walk — searchTermsContainWord only
	// checks each term's own Raw/Alternatives, so a hand-built term for
	// "compare" without a "ref"/"refs" alternative exercises the guard's
	// zero-score branch directly.
	compareOnly := []searchTerm{{Raw: "compare", Alternatives: []string{"compare"}}}
	if got := scoreCompareRefsIntentValue(entry, compareOnly); got != 0 {
		t.Fatalf("scoreCompareRefsIntentValue(compare without ref/refs) = %d, want 0", got)
	}
}

// TestScoreReleaseListIntent_PositiveCase verifies the ScoreReleaseListIntent_PositiveCase handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestScoreReleaseListIntent_PositiveCase(t *testing.T) {
	entry := scoringEntry("release.list", "release", "list")

	score, reason := scoreReleaseListIntent(entry, normalizeSearchTerms("list releases"))
	if score == 0 || reason == (MatchReason{}) {
		t.Fatalf("scoreReleaseListIntent() = %d, %+v, want positive score with populated reason", score, reason)
	}
	if reason.Field != searchFieldReleaseIntent {
		t.Fatalf("scoreReleaseListIntent() field = %q, want %q", reason.Field, searchFieldReleaseIntent)
	}
	if reason.QueryTerm != "list releases" {
		t.Fatalf("scoreReleaseListIntent() query term = %q, want %q", reason.QueryTerm, "list releases")
	}
}

// TestScoreDiscoverProjectIntentValue_Branches verifies all four trigger
// words (url/remote/origin/git) and all five disambiguation words
// (project/path/resolve/discover/find) feed the discover-project intent.
func TestScoreDiscoverProjectIntentValue_Branches(t *testing.T) {
	entry := scoringEntry("discover_project.resolve", "discover_project", "resolve")

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{name: "url trigger + project disambiguator", query: "url project", want: scoreDiscoverIntentBoost},
		{name: "remote trigger + path disambiguator", query: "remote path", want: scoreDiscoverIntentBoost},
		{name: "origin trigger + resolve disambiguator", query: "origin resolve", want: scoreDiscoverIntentBoost},
		{name: "git trigger + discover disambiguator", query: "git discover", want: scoreDiscoverIntentBoost},
		{name: "url trigger + find disambiguator", query: "url find", want: scoreDiscoverIntentBoost},
		{name: "no trigger returns zero", query: "branch", want: 0},
		// "git" is a trigger with no synonym entries, so the disambiguator check fails.
		{name: "trigger present but no disambiguator returns zero", query: "git help", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scoreDiscoverProjectIntentValue(entry, normalizeSearchTerms(tc.query))
			if got != tc.want {
				t.Fatalf("scoreDiscoverProjectIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestScoreProjectGetIntent_PositiveCase verifies the project-get-by-path
// scoring helper covers the show/find/path/id disambiguation alternatives
// and returns a populated MatchReason.
func TestScoreProjectGetIntent_PositiveCase(t *testing.T) {
	entry := scoringEntry("project.get", "project", "get")

	score, reason := scoreProjectGetIntent(entry, normalizeSearchTerms("project show"))
	if score == 0 || reason == (MatchReason{}) {
		t.Fatalf("scoreProjectGetIntent() = %d, %+v, want positive score with populated reason", score, reason)
	}
	if reason.Field != searchFieldProjectIntent {
		t.Fatalf("scoreProjectGetIntent() field = %q, want %q", reason.Field, searchFieldProjectIntent)
	}
	if reason.QueryTerm != "project get by path" {
		t.Fatalf("scoreProjectGetIntent() query term = %q, want %q", reason.QueryTerm, "project get by path")
	}

	// "find", "path", "id" alternatives all return the boost.
	for _, q := range []string{"project find", "project path", "project id"} {
		t.Run(q, func(t *testing.T) {
			if v := scoreProjectGetIntentValue(entry, normalizeSearchTerms(q)); v != scoreProjectGetIntentBoost {
				t.Fatalf("scoreProjectGetIntentValue(%q) = %d, want %d", q, v, scoreProjectGetIntentBoost)
			}
		})
	}
}

// TestScoreSearchProjectsIntent_PositiveCase verifies the search-projects
// scoring helper returns a positive score and a populated MatchReason for
// matching queries, including the concrete-needle boost.
func TestScoreSearchProjectsIntent_PositiveCase(t *testing.T) {
	entry := scoringEntry("search.projects", "search", "projects")

	score, reason := scoreSearchProjectsIntent(entry, normalizeSearchTerms("search projects"))
	if score == 0 || reason == (MatchReason{}) {
		t.Fatalf("scoreSearchProjectsIntent() = %d, %+v, want positive score with populated reason", score, reason)
	}
	if reason.Field != searchFieldSearchIntent {
		t.Fatalf("scoreSearchProjectsIntent() field = %q, want %q", reason.Field, searchFieldSearchIntent)
	}
	if reason.QueryTerm != "search projects" {
		t.Fatalf("scoreSearchProjectsIntent() query term = %q, want %q", reason.QueryTerm, "search projects")
	}

	// Concrete needle (non-stopword) adds the extra boost.
	concrete := scoreSearchProjectsIntentValue(entry, normalizeSearchTerms("search projects gitlab-mcp-server"))
	plain := scoreSearchProjectsIntentValue(entry, normalizeSearchTerms("search projects"))
	if concrete <= plain {
		t.Fatalf("scoreSearchProjectsIntentValue(concrete) = %d, want greater than plain %d", concrete, plain)
	}
	if plain != scoreSearchProjectsBoost {
		t.Fatalf("scoreSearchProjectsIntentValue(plain) = %d, want %d", plain, scoreSearchProjectsBoost)
	}
}

// The guidance lines the three catalog discovery cards end with, spelled out
// here so that a whole-output expectation reads as the document a model
// receives and a change to any of them is a visible change to this file.
const (
	wantBroaderTermsHint       = "Try broader terms such as project, issue, merge request, pipeline, branch, or user."
	wantFullSchemaHint         = "Use `gitlab_find_action` when the chosen action's full schema is still needed."
	wantExecuteNowHint         = "Choose one row and call `gitlab_execute_action` with that row's schema and example now, before starting another catalog operation; do not call `gitlab_find_action` again until that execute call returns."
	wantStructuredResultsHint  = "Structured results carry the exact `input_schema` and a `gitlab_execute_action` example for every action listed."
	wantDescribeExecuteHint    = "Call `gitlab_execute_action` with the action ID this card names and the params its input schema requires."
	wantDestructiveConfirmCell = "Execute destructive actions with top-level `confirm:true`."
)

// wantHintsBlock renders the guidance section a card ends with, so a
// whole-output expectation can be written as the document rather than as a
// run of escape sequences. It is the shape toolutil.WriteHints produces after
// a body that ends in a newline: one blank line, the rule, the heading, and
// one bullet per hint.
func wantHintsBlock(hints ...string) string {
	var b strings.Builder
	b.WriteString("\n---\n\U0001F4A1 **Next steps:**\n")
	for _, hint := range hints {
		b.WriteString("- " + hint + "\n")
	}
	return b.String()
}

// TestFormatSearchOutput_EmptyWithSuggestions verifies the whole document an
// empty search answers with when the registry found nearby tokens: the
// heading says nothing matched, the query is echoed as a code span, no table
// is opened, and the suggestions are the guidance, since a suggestion is a
// next step rather than a remark.
func TestFormatSearchOutput_EmptyWithSuggestions(t *testing.T) {
	want := "## GitLab Catalog: no matching action\n\n" +
		"- **Query**: `nonsenseonlyzz`\n" +
		wantHintsBlock("Try: `project`, `issue`.")
	got := formatSearchOutput(SearchOutput{
		Query:       "nonsenseonlyzz",
		Count:       0,
		Suggestions: []string{"project", "issue"},
	})
	if got != want {
		t.Fatalf("formatSearchOutput() = %q, want %q", got, want)
	}
}

// TestFormatSearchOutput_NextStepFallback verifies that the schema-aware next
// step the registry computed is what the card ends with, and that a result
// set carrying none falls back to the find_action hint. Both are guidance
// rather than prose, so ExtractHints lifts whichever one was written.
func TestFormatSearchOutput_NextStepFallback(t *testing.T) {
	result := SearchResult{ID: "project.get", Score: 120, RequiredParams: []string{"project_id"}}
	body := func(query string) string {
		return "## GitLab Catalog: 1 matching action\n\n" +
			"- **Query**: `" + query + "`\n\n" +
			"| Action ID | Score | Destructive | Required Params |\n" +
			"| --- | --- | --- | --- |\n" +
			"| `project.get` | 120 | - | `project_id` |\n"
	}

	t.Run("the computed next step replaces the fallback", func(t *testing.T) {
		want := body("with next") + wantHintsBlock("Use its exact parameter schema before executing", dynamicExecuteEnvelopeHint)
		got := formatSearchOutput(SearchOutput{
			Query:    "with next",
			Count:    1,
			NextStep: "Use its exact parameter schema before executing",
			Results:  []SearchResult{result},
		})
		if got != want {
			t.Fatalf("formatSearchOutput(next) = %q, want %q", got, want)
		}
	})

	t.Run("no next step falls back to the schema hint", func(t *testing.T) {
		want := body("without next") + wantHintsBlock(wantFullSchemaHint, dynamicExecuteEnvelopeHint)
		got := formatSearchOutput(SearchOutput{Query: "without next", Count: 1, Results: []SearchResult{result}})
		if got != want {
			t.Fatalf("formatSearchOutput(no next) = %q, want %q", got, want)
		}
	})
}

// TestFormatFindOutput_AllTableShapes verifies the whole document each of the
// four column shapes produces: neither optional column, guidance alone,
// explanations alone, and both. The header and the row are filtered from one
// column order, so the shape is the only thing that varies between them.
func TestFormatFindOutput_AllTableShapes(t *testing.T) {
	canonicalReason := MatchReason{
		Field:        searchFieldCanonicalID,
		QueryTerm:    "project",
		MatchedValue: "project.get",
		Score:        120,
	}
	canonicalExplanation := &ScoringExplanation{
		TotalScore:   200,
		MatchedTerms: 2,
		Reasons:      []MatchReason{canonicalReason},
	}
	const (
		why      = `canonical_id matched "project.get"`
		guidance = "destructive project action " + wantDestructiveConfirmCell
	)
	head := "## GitLab Catalog: 1 matching action\n\n- **Query**: `project get`\n\n"
	hints := wantHintsBlock(wantExecuteNowHint, dynamicExecuteEnvelopeHint, wantStructuredResultsHint)

	cases := []struct {
		name   string
		result FindResult
		want   string
	}{
		{
			name: "neither optional column is opened",
			result: FindResult{
				ID:             "project.get",
				Score:          200,
				Destructive:    false,
				RequiredParams: []string{"project_id"},
			},
			want: head +
				"| Action ID | Score | Destructive | Required Params |\n" +
				"| --- | --- | --- | --- |\n" +
				"| `project.get` | 200 | - | `project_id` |\n" + hints,
		},
		{
			name: "a usage note and a destructive flag open the guidance column",
			result: FindResult{
				ID:             "project.get",
				Score:          200,
				Destructive:    true,
				RequiredParams: []string{"project_id"},
				Usage:          "destructive project action",
			},
			want: head +
				"| Action ID | Score | Destructive | Required Params | Guidance |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| `project.get` | 200 | " + toolutil.EmojiWarning + " yes | `project_id` | " + guidance + " |\n" + hints,
		},
		{
			name: "an explanation opens the why column",
			result: FindResult{
				ID:             "project.get",
				Score:          200,
				Destructive:    false,
				RequiredParams: []string{"project_id"},
				Explanation:    canonicalExplanation,
			},
			want: head +
				"| Action ID | Score | Destructive | Required Params | Why |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| `project.get` | 200 | - | `project_id` | " + why + " |\n" + hints,
		},
		{
			name: "both optional columns are opened together",
			result: FindResult{
				ID:             "project.get",
				Score:          200,
				Destructive:    true,
				RequiredParams: []string{"project_id"},
				Usage:          "destructive project action",
				Explanation:    canonicalExplanation,
			},
			want: head +
				"| Action ID | Score | Destructive | Required Params | Guidance | Why |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| `project.get` | 200 | " + toolutil.EmojiWarning + " yes | `project_id` | " + guidance + " | " + why + " |\n" + hints,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatFindOutput(FindOutput{
				Query:   "project get",
				Count:   1,
				Results: []FindResult{tc.result},
			}, nil)
			if got != tc.want {
				t.Fatalf("formatFindOutput() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCompactParameterGuidance_AlphabeticalTieBreaker verifies the third
// comparator branch in the sort: when both items have equal required status
// and equal CommonConfusions length, the result falls through to a lexical
// name comparison.
func TestCompactParameterGuidance_AlphabeticalTieBreaker(t *testing.T) {
	guidance := map[string]toolutil.ParameterGuidance{
		"zeta":  {ValueSource: "alpha"},
		"alpha": {ValueSource: "beta"},
	}
	got := compactParameterGuidance(guidance, 5)
	alphaIdx := strings.Index(got, "`alpha`")
	zetaIdx := strings.Index(got, "`zeta`")
	if alphaIdx == -1 || zetaIdx == -1 || alphaIdx > zetaIdx {
		t.Fatalf("compactParameterGuidance() = %q, want alpha before zeta", got)
	}
}

// TestRegistrySearch_ProjectListSearchQuery_RanksSearchProjectsFirst pins the
// ranking outcome three print-only diagnostics used to investigate.
//
// Those diagnostics dumped score components for "project list search" and
// always passed, so a regression in scoring, ranking, or Search error handling
// went unreported. The end-to-end outcome is the part worth pinning: component
// scores move whenever the scorer is tuned, but search.projects must stay the
// top hit for this query, and it must not depend on the result window — the
// limit-sensitivity that prompted the investigation in the first place.
func TestRegistrySearch_ProjectListSearchQuery_RanksSearchProjectsFirst(t *testing.T) {
	catalog := mustCachedCatalog(t, true)
	reg := NewRegistryFromCatalog(catalog)

	const query = "project list search gitlab-mcp-server"
	for _, limit := range []int{8, 20} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			_, out, searchErr := reg.Search(context.Background(), nil, SearchInput{Query: query, Limit: limit})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if len(out.Results) == 0 {
				t.Fatal("Search() returned no results")
			}
			if out.Results[0].ID != "search.projects" {
				t.Errorf("Search() top result = %q, want search.projects", out.Results[0].ID)
			}
		})
	}
}

// TestExecute_WithheldActionNamesTheCauseInsteadOfCallingItUnknown pins that an
// action removed from the catalog by a narrowing filter is reported as withheld
// rather than as unknown.
//
// A read_api token asking for a write action used to get the generic
// unknown-action answer, complete with five real read-only near misses. The
// reply is authoritative-looking and wrong in the one way that matters: the
// reader concludes the server cannot do the thing, when the truth is that the
// credential is narrow and reauthorizing would fix it. The two causes carry
// different remedies, so they get different sentences, and the control case
// keeps the old message where it is still correct.
// narrowedCatalog is the standard test catalog with project.hook_add removed,
// the shape a withheld action leaves behind.
func narrowedCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	routes := testRoutes(t)
	delete(routes["gitlab_project"], "hook_add")
	return actioncatalog.FromActionMaps(routes)
}

// executeText runs an action that must fail as a tool error and returns the
// error text the client would read.
func executeText(t *testing.T, registry *Registry, action string) string {
	t.Helper()
	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: action})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	return textContent(result)
}

func TestExecute_WithheldActionNamesTheCauseInsteadOfCallingItUnknown(t *testing.T) {
	t.Run("token scope says reauthorize", func(t *testing.T) {
		registry := newCatalogRegistry(narrowedCatalog(t),
			WithWithheldActions([]string{"project.hook_add"}, nil))
		text := executeText(t, registry, "project.hook_add")
		for _, want := range []string{"exists but is not available", "credential in use", "api scope"} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(text, want) {
					t.Errorf("Execute() error text = %q, want it to contain %q", text, want)
				}
			})
		}
		for _, unwanted := range []string{"unknown action", "Did you mean"} {
			t.Run(unwanted, func(t *testing.T) {
				if strings.Contains(text, unwanted) {
					t.Errorf("Execute() error text = %q, must not contain %q: the action is not unknown", text, unwanted)
				}
			})
		}
	})

	t.Run("operator setting says ask the operator", func(t *testing.T) {
		registry := newCatalogRegistry(narrowedCatalog(t),
			WithWithheldActions(nil, []string{"project.hook_add"}))
		text := executeText(t, registry, "project.hook_add")
		if !strings.Contains(text, "Ask the operator") {
			t.Errorf("Execute() error text = %q, want it to name the operator as the remedy", text)
		}
		if strings.Contains(text, "Reauthorize") {
			t.Errorf("Execute() error text = %q, must not tell a client to reauthorize a credential that is not the cause", text)
		}
	})

	t.Run("an alias of a withheld action is withheld too", func(t *testing.T) {
		registry := newCatalogRegistry(narrowedCatalog(t),
			WithWithheldActions([]string{"project.hook_add", "add project hook"}, nil))
		text := executeText(t, registry, "Add Project Hook")
		if !strings.Contains(text, "exists but is not available") {
			t.Errorf("Execute() error text = %q, want the withheld explanation for an alias", text)
		}
	})

	t.Run("nothing withheld keeps the unknown-action answer", func(t *testing.T) {
		registry := newCatalogRegistry(narrowedCatalog(t))
		text := executeText(t, registry, "project.hook_add")
		if !strings.Contains(text, "unknown action") {
			t.Errorf("Execute() error text = %q, want the unknown-action message when nothing was withheld", text)
		}
	})
}

// TestWithheldKeySet_NormalizesKeysAndDropsBlankOnes verifies how the withheld
// list is indexed: each key is lowercased and trimmed, and a key that is blank
// or only spaces is not indexed at all.
//
// The set is looked up with the lowercased action a caller asked for, so an
// untrimmed key would never be found. A blank one is worse than useless: it
// would answer a caller who sent an empty action with the withheld explanation
// instead of the "action is required" refusal.
func TestWithheldKeySet_NormalizesKeysAndDropsBlankOnes(t *testing.T) {
	set := withheldKeySet([]string{"  Project.Hook_Add ", "", "   "})

	if _, ok := set["project.hook_add"]; !ok {
		t.Errorf("withheldKeySet() = %v, want the normalized key indexed", set)
	}
	if len(set) != 1 {
		t.Errorf("withheldKeySet() = %v, want the blank keys dropped", set)
	}
	if got := withheldKeySet(nil); got != nil {
		t.Errorf("withheldKeySet(nil) = %v, want nil", got)
	}
}

// TestRegistryShapeFor_SharedCatalogReusesOneShape verifies the split the
// pool relies on: every registry built over one shared catalog takes the one
// shape cached for that catalog's origin, a registry built with extra
// aliases or over a catalog nobody shared builds its own, and the handlers
// are built per registry over the bound catalog rather than the origin.
func TestRegistryShapeFor_SharedCatalogReusesOneShape(t *testing.T) {
	shared, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	if shared.SharedOrigin() == nil {
		t.Fatal("BuildActionCatalog() returned a catalog with no shared origin")
	}
	first := registryShapeFor(shared, nil)
	second := registryShapeFor(shared.SharedOrigin().BindTo(nil), nil)
	if first != second {
		t.Fatal("two catalogs bound from one origin got two shapes, want one")
	}
	if registryShapeFor(shared, actionAliases()) == first {
		t.Fatal("a registry with extra aliases took the cached shape, want its own")
	}
	private := actioncatalog.FromActionMaps(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": toolutil.Route(func(context.Context, map[string]any) (any, error) { return map[string]any{}, nil })},
	})
	firstPrivate := registryShapeFor(private, nil)
	secondPrivate := registryShapeFor(private, nil)
	if firstPrivate == secondPrivate {
		t.Fatal("a catalog nobody shared got a cached shape")
	}

	registryA := NewRegistryFromCatalog(shared)
	registryB := NewRegistryFromCatalog(shared.SharedOrigin().BindTo(nil))
	if len(registryA.entries) != len(registryB.entries) || len(registryA.handlers) != len(registryB.handlers) || len(registryA.handlers) == 0 {
		t.Fatalf("registries over one origin differ: %d/%d entries, %d/%d handlers", len(registryA.entries), len(registryB.entries), len(registryA.handlers), len(registryB.handlers))
	}
	if &registryA.entries[0] != &registryB.entries[0] {
		t.Fatal("the two registries hold separate entry slices, want the shared shape's")
	}
}

// TestFindAndExecuteInputSchemas_AreSharedAndFallBackWhenDerivationFails
// verifies the two schemas every dynamic server registers are one shared
// pointer each and match what the SDK derives itself, and that a derivation
// failure leaves the field nil for the SDK to fill.
//
// The find schema is the SDK's derivation plus one keyword: the query's
// maxLength, which the struct tag cannot carry and which is set on the derived
// schema. The comparison spells that difference out rather than applying the
// annotator it is checking.
func TestFindAndExecuteInputSchemas_AreSharedAndFallBackWhenDerivationFails(t *testing.T) {
	findSchema := findInputSchema()
	if !toolutil.SchemaShared(findSchema) || findSchema != findInputSchema() {
		t.Error("the find input schema is not one shared pointer")
	}
	executeSchema := executeActionInputSchema()
	if !toolutil.SchemaShared(executeSchema) || executeSchema != executeActionInputSchema() {
		t.Error("the execute input schema is not one shared pointer")
	}
	want, err := jsonschema.For[FindInput](nil)
	if err != nil {
		t.Fatalf("jsonschema.For[FindInput]() error = %v", err)
	}
	maxLength := MaxSearchQueryLength
	want.Properties[findQueryParam].MaxLength = &maxLength
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("encoding the SDK's find schema: %v", err)
	}
	gotJSON, err := json.Marshal(findSchema)
	if err != nil {
		t.Fatalf("encoding the shared find schema: %v", err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Errorf("find input schema = %s, want the SDK's own derivation %s", gotJSON, wantJSON)
	}

	forced := errors.New("forced derivation failure")
	originalFind, originalExecute := findSchemaFor, executeSchemaFor
	t.Cleanup(func() { findSchemaFor, executeSchemaFor = originalFind, originalExecute })
	findSchemaFor = func(*jsonschema.ForOptions) (*jsonschema.Schema, error) { return nil, forced }
	executeSchemaFor = func(*jsonschema.ForOptions) (*jsonschema.Schema, error) { return nil, forced }
	if buildFindInputSchema() != nil || buildExecuteActionInputSchema() != nil {
		t.Error("a failed derivation returned a schema, want nil so the SDK derives its own")
	}
}

// TestCompactParameterGuidance_ZeroLimitEmptyGuidanceAndNoTruncation covers the
// answers the compactor gives when nothing is truncated. A zero limit means no
// guidance at all and is decided before the names are collected, an empty map
// means the same, and a list that fits within the limit must carry no "and N
// more" tail: that tail is written from a counter which is zero here, and a
// suffix claiming zero further parameters would be printed on every find row.
// The exactly-at-the-limit case is listed beside the shorter one because both
// spellings of the truncation test agree there, truncating zero names being a
// no-op that the tail counter then suppresses.
func TestCompactParameterGuidance_ZeroLimitEmptyGuidanceAndNoTruncation(t *testing.T) {
	guidance := map[string]toolutil.ParameterGuidance{
		"alpha": {ValueSource: "generated by the server"},
		"beta":  {SemanticRole: "target branch name"},
	}

	t.Run("zero limit returns nothing even with guidance", func(t *testing.T) {
		if got := compactParameterGuidance(guidance, 0); got != "" {
			t.Fatalf("compactParameterGuidance(limit 0) = %q, want empty", got)
		}
	})

	t.Run("empty guidance returns nothing", func(t *testing.T) {
		if got := compactParameterGuidance(nil, 2); got != "" {
			t.Fatalf("compactParameterGuidance(nil) = %q, want empty", got)
		}
	})

	t.Run("list below the limit carries no truncation tail", func(t *testing.T) {
		got := compactParameterGuidance(guidance, 5)
		if !strings.Contains(got, "`alpha`") || !strings.Contains(got, "`beta`") {
			t.Fatalf("compactParameterGuidance() = %q, want both parameters", got)
		}
		if strings.Contains(got, "more params") {
			t.Fatalf("compactParameterGuidance() = %q, want no truncation tail", got)
		}
	})

	t.Run("list exactly at the limit carries no truncation tail", func(t *testing.T) {
		got := compactParameterGuidance(guidance, 2)
		if !strings.Contains(got, "`alpha`") || !strings.Contains(got, "`beta`") {
			t.Fatalf("compactParameterGuidance() = %q, want both parameters", got)
		}
		if strings.Contains(got, "more params") {
			t.Fatalf("compactParameterGuidance() = %q, want no truncation tail", got)
		}
	})
}

// TestCompactParameterGuidance_OrdersByConfusionCount verifies the second
// comparator branch: with required status equal, the parameter carrying more
// recorded confusions is listed first, because that is the one a model is most
// likely to get wrong and the list is truncated from the end.
func TestCompactParameterGuidance_OrdersByConfusionCount(t *testing.T) {
	guidance := map[string]toolutil.ParameterGuidance{
		"alpha": {CommonConfusions: []string{"first", "second", "third"}},
		"bravo": {CommonConfusions: []string{"first", "second"}},
		"delta": {CommonConfusions: []string{"first"}},
		"echo":  {SemanticRole: "no recorded confusion"},
	}

	got := compactParameterGuidance(guidance, 4)
	previous := -1
	for _, name := range []string{"`alpha`", "`bravo`", "`delta`", "`echo`"} {
		t.Run(name, func(t *testing.T) {
			index := strings.Index(got, name)
			if index <= previous {
				t.Fatalf("compactParameterGuidance() = %q, want alpha, bravo, delta, echo in that order", got)
			}
			previous = index
		})
	}
}

// TestSearchNextStep_AdviceForEachTopResultShape verifies the four answers the
// next-step line gives, and in particular that the two confirmation signals are
// independent. A top result can be ambiguous while scoring high, and can score
// low while being the only candidate, so either signal alone must produce the
// confirmation advice: a model that executes an ambiguous top hit without
// asking has acted on an action the user never named.
func TestSearchNextStep_AdviceForEachTopResultShape(t *testing.T) {
	cases := []struct {
		name    string
		results []SearchResult
		want    string
		deny    string
	}{
		{
			name:    "no results say nothing",
			results: nil,
			want:    "",
		},
		{
			name:    "low confidence alone asks for confirmation",
			results: []SearchResult{{ID: "project.get", LowConfidence: true, RequiredParams: []string{"project_id"}}},
			want:    "needs confirmation",
		},
		{
			name:    "ambiguity alone asks for confirmation",
			results: []SearchResult{{ID: "project.get", AmbiguousWith: []string{"group.get"}, RequiredParams: []string{"project_id"}}},
			want:    "needs confirmation",
		},
		{
			name:    "no required params invites an empty params object",
			results: []SearchResult{{ID: "user.current"}},
			want:    "has no required params",
		},
		{
			name:    "destructive high confidence names the params and the confirm flag",
			results: []SearchResult{{ID: "project.delete", Destructive: true, RequiredParams: []string{"project_id"}}},
			want:    "confirm:true",
			deny:    "needs confirmation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := searchNextStep(tc.results)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("searchNextStep() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("searchNextStep() = %q, want %q", got, tc.want)
			}
			if tc.deny != "" && strings.Contains(got, tc.deny) {
				t.Fatalf("searchNextStep() = %q, want no %q", got, tc.deny)
			}
		})
	}
}

// TestScoreSearchAlternative_DomainAndActionWordBranches verifies the four
// domain-or-action tests that the table above leaves half covered. Each of them
// pairs a domain test with an action test, and only one side of each pair is
// exercised there, so a term that matches through the other side has to be
// scored at the same weight: a query naming a multi-word domain by one of its
// words, or naming the operation and nothing else, would otherwise fall through
// to a weaker match and rank below actions that matched less.
func TestScoreSearchAlternative_DomainAndActionWordBranches(t *testing.T) {
	cases := []struct {
		name        string
		entry       actionEntry
		alternative string
		want        int
	}{
		{
			name:        "the domain exactly",
			entry:       actionEntry{ID: "x.y", Domain: "pipeline", Action: "schedule_create"},
			alternative: "pipeline",
			want:        scoreDomainActionExact,
		},
		{
			name:        "one word of a multi-word action",
			entry:       actionEntry{ID: "x.y", Domain: "pipeline", Action: "schedule_create"},
			alternative: "schedule",
			want:        scoreDomainActionWord,
		},
		{
			name:        "one word of a multi-word domain",
			entry:       actionEntry{ID: "x.y", Domain: "merge_request", Action: "list"},
			alternative: "merge",
			want:        scoreDomainActionWord,
		},
		{
			name:        "a prefix of an action word",
			entry:       actionEntry{ID: "x.y", Domain: "pipeline", Action: "schedule_create"},
			alternative: "sched",
			want:        scoreDomainActionContains,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreSearchAlternative(tc.entry, tc.alternative, tc.alternative); got != tc.want {
				t.Fatalf("scoreSearchAlternative(%q) = %d, want %d", tc.alternative, got, tc.want)
			}
		})
	}
}

// TestScoreSearchAlternativeWithReason_ExactMatchWeights verifies that each
// exact match reports its own field at its own weight. Two pairs of cases
// report the same field from different tests, the domain and one word of it,
// and the action and one word of it, so a test that reads only the field cannot
// tell them apart. The weight is the half that decides the ranking: an exact
// domain scored as a word match ranks below an action that matched one word.
func TestScoreSearchAlternativeWithReason_ExactMatchWeights(t *testing.T) {
	entry := actionEntry{Document: searchDocument{
		CanonicalID: "merge_request.approval_list",
		Domain:      "merge_request",
		DomainWords: []string{"merge", "request"},
		Action:      "approval_list",
		ActionWords: []string{"approval", "list"},
		Aliases:     []string{"mr.approvals"},
		Tags:        []string{"merge request approvals"},
	}}

	cases := []struct {
		name        string
		alternative string
		wantField   string
		wantScore   int
	}{
		{name: "canonical id", alternative: "merge_request.approval_list", wantField: searchFieldCanonicalID, wantScore: scoreCanonicalExact},
		{name: "alias", alternative: "mr.approvals", wantField: searchFieldAlias, wantScore: scoreAliasExact},
		{name: "tag", alternative: "merge request approvals", wantField: searchFieldTag, wantScore: scoreTagExact},
		{name: "the action", alternative: "approval_list", wantField: searchFieldAction, wantScore: scoreDomainActionExact},
		{name: "the domain", alternative: "merge_request", wantField: searchFieldDomain, wantScore: scoreDomainActionExact},
		{name: "one action word", alternative: "approval", wantField: searchFieldAction, wantScore: scoreDomainActionWord},
		{name: "one domain word", alternative: "request", wantField: searchFieldDomain, wantScore: scoreDomainActionWord},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score, reason := scoreSearchAlternativeWithReason(entry, tc.alternative, tc.alternative)
			if score != tc.wantScore || reason.Field != tc.wantField {
				t.Fatalf("scoreSearchAlternativeWithReason(%q) = %d, %+v; want %d on %s", tc.alternative, score, reason, tc.wantScore, tc.wantField)
			}
		})
	}
}

// TestScoreSearchAlternativeWithReason_FlatTextExactOutscoresSynonym verifies
// the one score decision inside the flat-text case. Both halves report the same
// field, so a test that reads only the field cannot tell them apart, and a
// synonym scoring as high as the word the caller actually typed would let an
// expansion outrank an exact term on the weakest evidence the scorer has.
func TestScoreSearchAlternativeWithReason_FlatTextExactOutscoresSynonym(t *testing.T) {
	entry := actionEntry{Document: searchDocument{CanonicalID: "project.get", FlatText: "read repository"}}

	t.Run("the typed word", func(t *testing.T) {
		score, reason := scoreSearchAlternativeWithReason(entry, "read", "read")
		if score != scoreFieldContains || reason.Field != searchFieldFlatText {
			t.Fatalf("scoreSearchAlternativeWithReason(exact) = %d, %+v; want %d on %s", score, reason, scoreFieldContains, searchFieldFlatText)
		}
	})

	t.Run("a synonym of it", func(t *testing.T) {
		score, reason := scoreSearchAlternativeWithReason(entry, "repo", "repository")
		if score != scoreSynonymContains || reason.Field != searchFieldFlatText {
			t.Fatalf("scoreSearchAlternativeWithReason(synonym) = %d, %+v; want %d on %s", score, reason, scoreSynonymContains, searchFieldFlatText)
		}
	})
}

// TestMatchedSearchValue_SubstringAndWordFormAreSeparateTests verifies the two
// ways a catalog value can carry the term a caller sent, because each answers a
// query the other does not. The synonym "merge_request", which is what "mr"
// expands to, finds the required param "merge_request_iid" by substring and not
// through the word form, where the separator has become a space. The word form
// answers the reverse shape, a term whose own words straddle a separator; no
// synonym table entry spells one today, so the case is here to pin the
// behavior the precomputed word map was built to preserve.
func TestMatchedSearchValue_SubstringAndWordFormAreSeparateTests(t *testing.T) {
	cases := []struct {
		name        string
		values      []string
		alternative string
		want        string
	}{
		{name: "value equals the term", values: []string{"project_id"}, alternative: "project_id", want: "project_id"},
		{name: "substring of the raw value only", values: []string{"merge_request_iid"}, alternative: "merge_request", want: "merge_request_iid"},
		{name: "word form only", values: []string{"merge_request_iid"}, alternative: "merge request", want: "merge_request_iid"},
		{name: "no value carries it", values: []string{"project_id"}, alternative: "issue", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchedSearchValue(nil, tc.values, tc.alternative); got != tc.want {
				t.Fatalf("matchedSearchValue(%v, %q) = %q, want %q", tc.values, tc.alternative, got, tc.want)
			}
		})
	}
}

// TestDocumentForEntry_PrebuiltDocumentIsUsedAsIs verifies that either half of
// the prebuilt check keeps the document the registry already built. The flat
// text half matters for an entry whose document was assembled from search text
// alone: rebuilt from the entry, it would lose that text and every field the
// builder computed with it.
func TestDocumentForEntry_PrebuiltDocumentIsUsedAsIs(t *testing.T) {
	t.Run("recognized by canonical id", func(t *testing.T) {
		entry := actionEntry{ID: "project.get", Document: searchDocument{CanonicalID: "already.built"}}
		if got := documentForEntry(entry); got.CanonicalID != "already.built" {
			t.Fatalf("documentForEntry() = %+v, want the prebuilt document", got)
		}
	})

	t.Run("recognized by flat text alone", func(t *testing.T) {
		entry := actionEntry{ID: "project.get", Document: searchDocument{FlatText: "prebuilt flat text"}}
		got := documentForEntry(entry)
		if got.CanonicalID != "" || got.FlatText != "prebuilt flat text" {
			t.Fatalf("documentForEntry() = %+v, want the prebuilt document", got)
		}
	})

	t.Run("an empty document is rebuilt from the entry", func(t *testing.T) {
		entry := actionEntry{ID: "Project.Get", Domain: "project", Action: "get"}
		if got := documentForEntry(entry); got.CanonicalID != "project.get" {
			t.Fatalf("documentForEntry() = %+v, want a document rebuilt from the entry", got)
		}
	})
}

// TestScoreParamContainsFor_KeepsAScoreWeakerThanTheSynonymFloor verifies the
// cap applied to a synonym match. A synonym is worth no more than
// scoreSynonymContains, but a field whose exact score already sits below that
// value keeps its own weaker score instead of being promoted by the cap:
// schema description terms are that case, and promoting them would make a
// synonym that only reached a description outrank an optional parameter the
// caller named outright.
func TestScoreParamContainsFor_KeepsAScoreWeakerThanTheSynonymFloor(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		alternative string
		exactScore  int
		want        int
	}{
		{name: "typed term keeps the field score", raw: "state", alternative: "state", exactScore: scoreRequiredParamMatch, want: scoreRequiredParamMatch},
		{name: "synonym is capped at the synonym score", raw: "status", alternative: "state", exactScore: scoreRequiredParamMatch, want: scoreSynonymContains},
		{name: "synonym below the cap keeps the weaker score", raw: "status", alternative: "state", exactScore: scoreSchemaDescMatch, want: scoreSchemaDescMatch},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreParamContainsFor(tc.raw, tc.alternative, tc.exactScore); got != tc.want {
				t.Fatalf("scoreParamContainsFor(%q, %q, %d) = %d, want %d", tc.raw, tc.alternative, tc.exactScore, got, tc.want)
			}
		})
	}
}

// TestAppendRequiredParamNames_SkipsNonStringsAndEmptyNames verifies both
// halves of the guard over a JSON "required" array. The array is decoded from a
// schema, so an element can be any JSON value, and an empty name is a name no
// caller can supply: either one reaching the required list would be published
// as a parameter and reported missing on every call of that action.
func TestAppendRequiredParamNames_SkipsNonStringsAndEmptyNames(t *testing.T) {
	t.Run("any-typed array", func(t *testing.T) {
		got := appendRequiredParamNames(nil, []any{"project_id", 42, "", map[string]any{}, "issue_iid"})
		if !slices.Equal(got, []string{"project_id", "issue_iid"}) {
			t.Fatalf("appendRequiredParamNames() = %v, want only the non-empty strings", got)
		}
	})

	t.Run("already typed as strings", func(t *testing.T) {
		got := appendRequiredParamNames(nil, []string{"project_id"})
		if !slices.Equal(got, []string{"project_id"}) {
			t.Fatalf("appendRequiredParamNames() = %v, want project_id", got)
		}
	})

	t.Run("anything else contributes nothing", func(t *testing.T) {
		if got := appendRequiredParamNames(nil, "project_id"); got != nil {
			t.Fatalf("appendRequiredParamNames(string) = %v, want nil", got)
		}
	})
}

// TestAppendPreferredAlternativeRequiredParams_EmptyAnyOfFallsThroughToOneOf
// verifies that an alternative keyword present but empty does not claim the
// schema. Only the first keyword carrying alternatives is read, so an empty
// anyOf that counted as present would stop the walk and leave the action with
// no required parameters at all, while its oneOf group names them.
func TestAppendPreferredAlternativeRequiredParams_EmptyAnyOfFallsThroughToOneOf(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{},
		"oneOf": []any{map[string]any{"required": []any{"group_id"}}},
	}
	got := appendPreferredAlternativeRequiredParams(nil, schema)
	if !slices.Equal(got, []string{"group_id"}) {
		t.Fatalf("appendPreferredAlternativeRequiredParams() = %v, want group_id from oneOf", got)
	}
}

// TestPlaceholderForParam_EveryNamedShape verifies the example value published
// for each parameter name the switch recognizes, plus the three suffix rules
// under it. The example is what a model copies into its first call, so a name
// that falls through to the generic "value" placeholder teaches it to send a
// string where GitLab expects an ID or a branch.
func TestPlaceholderForParam_EveryNamedShape(t *testing.T) {
	cases := []struct {
		name string
		want any
	}{
		{name: "project_id", want: "group/project"},
		{name: "target_project_id", want: "group/project"},
		{name: "group_id", want: "group/subgroup"},
		{name: "namespace_id", want: "group/subgroup"},
		{name: "file_path", want: "path/to/file"},
		{name: "artifact_path", want: "path/to/file"},
		{name: "ref", want: "main"},
		{name: "branch", want: "main"},
		{name: "branch_name", want: "main"},
		{name: "target_branch", want: "main"},
		{name: "source_branch", want: "main"},
		{name: "url", want: "https://example.com"},
		{name: "remote_url", want: "https://example.com"},
		{name: "external_url", want: "https://example.com"},
		{name: "web_url", want: "https://example.com"},
		{name: "id", want: 123},
		{name: "issue_id", want: 123},
		{name: "issue_iid", want: 123},
		{name: "due_date", want: "YYYY-MM-DD"},
		{name: "title", want: "value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := placeholderForParam(tc.name); got != tc.want {
				t.Fatalf("placeholderForParam(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestScoreSearchCodeIntentValue_VerbAndNounPairings verifies the two-part
// signal that promotes search.code, one noun at a time. "grep" is specific
// enough on its own, while "search" needs a code noun beside it to be told from
// a project or issue search, and each noun is its own test because a caller
// uses one of them, not all four: whichever word they reach for has to find the
// action.
func TestScoreSearchCodeIntentValue_VerbAndNounPairings(t *testing.T) {
	entry := scoringEntry("search.code", "search", "code")

	cases := []struct {
		name  string
		terms []searchTerm
		want  int
	}{
		{name: "grep alone", terms: searchTermsFromWords("grep"), want: scoreSearchCodeIntentBoost},
		{name: "search with code", terms: searchTermsFromWords("search", "code"), want: scoreSearchCodeIntentBoost},
		{name: "search with blob", terms: searchTermsFromWords("search", "blob"), want: scoreSearchCodeIntentBoost},
		{name: "search with blobs", terms: searchTermsFromWords("search", "blobs"), want: scoreSearchCodeIntentBoost},
		{name: "search with source", terms: searchTermsFromWords("search", "source"), want: scoreSearchCodeIntentBoost},
		{name: "search without a code noun", terms: searchTermsFromWords("search", "wikis"), want: 0},
		{name: "code noun without a search verb", terms: searchTermsFromWords("code"), want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreSearchCodeIntentValue(entry, tc.terms); got != tc.want {
				t.Fatalf("scoreSearchCodeIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("another search action is not promoted", func(t *testing.T) {
		other := scoringEntry("search.projects", "search", "projects")
		if got := scoreSearchCodeIntentValue(other, searchTermsFromWords("grep")); got != 0 {
			t.Fatalf("scoreSearchCodeIntentValue(search.projects) = %d, want 0", got)
		}
	})
}

// TestScoreCurrentUserIntentValue_SelfSignalAndIdentityNoun verifies each self
// signal and each identity noun on its own. The scorer wants one of each, and
// the nouns are read from the raw terms rather than from synonym expansions, so
// a noun that stopped being recognized would silently drop user.current back
// below user.get for the phrasing that uses it.
func TestScoreCurrentUserIntentValue_SelfSignalAndIdentityNoun(t *testing.T) {
	entry := scoringEntry("user.current", "user", "current")

	cases := []struct {
		name  string
		terms []searchTerm
		want  int
	}{
		{name: "current with user", terms: searchTermsFromWords("current", "user"), want: scoreCurrentUserIntentBoost},
		{name: "authenticated with account", terms: searchTermsFromWords("authenticated", "account"), want: scoreCurrentUserIntentBoost},
		{name: "whoami is both halves at once", terms: searchTermsFromWords("whoami"), want: scoreCurrentUserIntentBoost},
		{name: "current with profile", terms: searchTermsFromWords("current", "profile"), want: scoreCurrentUserIntentBoost},
		{name: "current with identity", terms: searchTermsFromWords("current", "identity"), want: scoreCurrentUserIntentBoost},
		{name: "current with me", terms: searchTermsFromWords("current", "me"), want: scoreCurrentUserIntentBoost},
		{name: "current with myself", terms: searchTermsFromWords("current", "myself"), want: scoreCurrentUserIntentBoost},
		{name: "self signal without an identity noun", terms: searchTermsFromWords("current", "pipeline"), want: 0},
		{name: "identity noun without a self signal", terms: searchTermsFromWords("user"), want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreCurrentUserIntentValue(entry, tc.terms); got != tc.want {
				t.Fatalf("scoreCurrentUserIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestQualifiesForExplicitIntentBypass_EitherIntentIsEnough verifies that the
// match-ratio bypass fires on either intent alone. Each of the two actions
// matches only two or three tokens of a long query, so demanding both signals
// at once would mean neither action ever passes the filter, which is the state
// the bypass exists to end.
func TestQualifiesForExplicitIntentBypass_EitherIntentIsEnough(t *testing.T) {
	codeTerms := searchTermsFromWords("grep")
	identityTerms := searchTermsFromWords("current", "user")

	t.Run("code intent alone", func(t *testing.T) {
		if !qualifiesForExplicitIntentBypass(scoringEntry("search.code", "search", "code"), codeTerms) {
			t.Fatal("qualifiesForExplicitIntentBypass(search.code) = false, want true")
		}
	})

	t.Run("identity intent alone", func(t *testing.T) {
		if !qualifiesForExplicitIntentBypass(scoringEntry("user.current", "user", "current"), identityTerms) {
			t.Fatal("qualifiesForExplicitIntentBypass(user.current) = false, want true")
		}
	})

	t.Run("neither intent", func(t *testing.T) {
		if qualifiesForExplicitIntentBypass(scoringEntry("project.get", "project", "get"), codeTerms) {
			t.Fatal("qualifiesForExplicitIntentBypass(project.get) = true, want false")
		}
	})
}

// TestScoreServiceAccountIntentValue_EachBonusIsConditional pins the three
// bonuses stacked on the base service-account score at the value each is worth,
// and pins that each one needs both of its conditions. A bonus that fired on
// half its condition would pay a token query on an action about no token, and a
// create query on the delete action, which is the ranking this scorer exists to
// prevent: the verb bonus is what keeps the six service-account actions apart.
func TestScoreServiceAccountIntentValue_EachBonusIsConditional(t *testing.T) {
	cases := []struct {
		name  string
		entry actionEntry
		terms []searchTerm
		want  int
	}{
		{
			name:  "base score alone",
			entry: scoringEntry("group.service_account_create", "group", "service_account_create"),
			terms: searchTermsFromWords("service", "account"),
			want:  scoreServiceAccountBoost,
		},
		{
			name:  "token word on an action that is not about a token",
			entry: scoringEntry("group.service_account_create", "group", "service_account_create"),
			terms: searchTermsFromWords("service", "account", "pat"),
			want:  scoreServiceAccountBoost,
		},
		{
			name:  "pat on the token action",
			entry: scoringEntry("group.service_account_pat_create", "group", "service_account_pat_create"),
			terms: searchTermsFromWords("service", "account", "pat"),
			want:  2 * scoreServiceAccountBoost,
		},
		{
			name:  "personal access token spelled out",
			entry: scoringEntry("group.service_account_pat_create", "group", "service_account_pat_create"),
			terms: searchTermsFromWords("service", "account", "personal", "access", "token"),
			want:  2 * scoreServiceAccountBoost,
		},
		{
			name:  "the action verb the query asked for",
			entry: scoringEntry("group.service_account_list", "group", "service_account_list"),
			terms: searchTermsFromWords("service", "account", "list"),
			want:  2 * scoreServiceAccountBoost,
		},
		{
			name:  "a verb the action does not carry",
			entry: scoringEntry("group.service_account_create", "group", "service_account_create"),
			terms: searchTermsFromWords("service", "account", "list"),
			want:  scoreServiceAccountBoost,
		},
		{
			name:  "an action whose suffix names no verb",
			entry: scoringEntry("group.service_account_unknown", "group", "service_account_unknown"),
			terms: searchTermsFromWords("service", "account", "list"),
			want:  scoreServiceAccountBoost,
		},
		{
			name:  "the domain named beside the verb",
			entry: scoringEntry("group.service_account_list", "group", "service_account_list"),
			terms: searchTermsFromWords("group", "service", "account", "list"),
			want:  2*scoreServiceAccountBoost + scoreServiceAccountScope,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreServiceAccountIntentValue(tc.entry, tc.terms); got != tc.want {
				t.Fatalf("scoreServiceAccountIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestScoreScopeIntent_MatchesTheDomainOrTheScope verifies the scope boost and
// the MatchReason that comes with it. The reason is what the explain output
// prints, so it has to name the scope the query asked for, and an entry that
// scored nothing must carry no reason at all: a zero-scored reason would appear
// in the explanation of every action the query never matched.
func TestScoreScopeIntent_MatchesTheDomainOrTheScope(t *testing.T) {
	t.Run("the domain is the scope", func(t *testing.T) {
		score, reason := scoreScopeIntent(scoringEntry("project.get", "project", "get"), searchTermsFromWords("project"))
		if score != scoreScopeIntentBoost {
			t.Fatalf("scoreScopeIntent() = %d, want %d", score, scoreScopeIntentBoost)
		}
		if reason.Field != searchFieldScopeIntent || reason.QueryTerm != "project" || reason.MatchedValue != "project.get" {
			t.Fatalf("scoreScopeIntent() reason = %+v, want the scope intent reason for project.get", reason)
		}
	})

	t.Run("the scope field is the scope", func(t *testing.T) {
		entry := scoringEntry("epic.list", "epic", "list")
		entry.Document.Scope = "group"
		if score := scoreScopeIntentValue(entry, searchTermsFromWords("group")); score != scoreScopeIntentBoost {
			t.Fatalf("scoreScopeIntentValue() = %d, want %d", score, scoreScopeIntentBoost)
		}
	})

	t.Run("group wins when the query names both", func(t *testing.T) {
		terms := searchTermsFromWords("project", "group")
		if score := scoreScopeIntentValue(scoringEntry("group.get", "group", "get"), terms); score != scoreScopeIntentBoost {
			t.Fatalf("scoreScopeIntentValue(group.get) = %d, want %d", score, scoreScopeIntentBoost)
		}
		if score := scoreScopeIntentValue(scoringEntry("project.get", "project", "get"), terms); score != 0 {
			t.Fatalf("scoreScopeIntentValue(project.get) = %d, want 0", score)
		}
	})

	t.Run("a query naming no scope scores nothing", func(t *testing.T) {
		score, reason := scoreScopeIntent(scoringEntry("project.get", "project", "get"), searchTermsFromWords("issue"))
		if score != 0 || reason != (MatchReason{}) {
			t.Fatalf("scoreScopeIntent() = %d, %+v; want zero result", score, reason)
		}
	})
}

// TestScoreCompareRefsIntentValue_GuardsAreIndependent verifies that the boost
// needs the action as well as the domain, and that either spelling of ref is
// enough. The singular and the plural are separate tests of separate words, so
// a query that says "refs" must reach the boost without the singular being
// present: it is the only one of the two most callers type.
func TestScoreCompareRefsIntentValue_GuardsAreIndependent(t *testing.T) {
	cases := []struct {
		name  string
		entry actionEntry
		terms []searchTerm
		want  int
	}{
		{
			name:  "singular ref",
			entry: scoringEntry("repository.compare", "repository", "compare"),
			terms: searchTermsFromWords("compare", "ref"),
			want:  scoreCompareRefsIntentBoost,
		},
		{
			name:  "plural refs alone",
			entry: scoringEntry("repository.compare", "repository", "compare"),
			terms: searchTermsFromWords("compare", "refs"),
			want:  scoreCompareRefsIntentBoost,
		},
		{
			name:  "the right domain with another action",
			entry: scoringEntry("repository.diff", "repository", "diff"),
			terms: searchTermsFromWords("compare", "refs"),
			want:  0,
		},
		{
			name:  "the compare action in another domain",
			entry: scoringEntry("branch.compare", "branch", "compare"),
			terms: searchTermsFromWords("compare", "refs"),
			want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreCompareRefsIntentValue(tc.entry, tc.terms); got != tc.want {
				t.Fatalf("scoreCompareRefsIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestScoreReleaseListIntentValue_GuardsAreIndependent verifies the three
// conditions the release-list boost needs, one at a time. The plural is the
// word a caller actually types ("list releases"), and it is tested without the
// singular beside it because the two are separate checks: if only the singular
// were read, the everyday phrasing would score nothing.
func TestScoreReleaseListIntentValue_GuardsAreIndependent(t *testing.T) {
	cases := []struct {
		name  string
		entry actionEntry
		terms []searchTerm
		want  int
	}{
		{
			name:  "singular release",
			entry: scoringEntry("release.list", "release", "list"),
			terms: searchTermsFromWords("list", "release"),
			want:  scoreReleaseListIntentBoost,
		},
		{
			name:  "plural releases alone",
			entry: scoringEntry("release.list", "release", "list"),
			terms: searchTermsFromWords("list", "releases"),
			want:  scoreReleaseListIntentBoost,
		},
		{
			name:  "the release domain with another action",
			entry: scoringEntry("release.get", "release", "get"),
			terms: searchTermsFromWords("list", "releases"),
			want:  0,
		},
		{
			name:  "no list verb",
			entry: scoringEntry("release.list", "release", "list"),
			terms: searchTermsFromWords("releases"),
			want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreReleaseListIntentValue(tc.entry, tc.terms); got != tc.want {
				t.Fatalf("scoreReleaseListIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestScoreDiscoverProjectIntent_OneTriggerAndOneDisambiguator verifies each
// trigger word and each disambiguator on its own, and the two guards on the
// action itself. The table above drives the same scorer through the normalizer,
// which expands one typed word into several of these at once, so it cannot show
// that any single word carries the intent: a caller says "remote" or "origin",
// not both, and the reason line has to name the action either way.
func TestScoreDiscoverProjectIntent_OneTriggerAndOneDisambiguator(t *testing.T) {
	resolve := scoringEntry("discover_project.resolve", "discover_project", "resolve")

	cases := []struct {
		name  string
		entry actionEntry
		terms []searchTerm
		want  int
	}{
		{name: "url trigger", entry: resolve, terms: searchTermsFromWords("url", "project"), want: scoreDiscoverIntentBoost},
		{name: "remote trigger", entry: resolve, terms: searchTermsFromWords("remote", "project"), want: scoreDiscoverIntentBoost},
		{name: "origin trigger", entry: resolve, terms: searchTermsFromWords("origin", "project"), want: scoreDiscoverIntentBoost},
		{name: "git trigger", entry: resolve, terms: searchTermsFromWords("git", "project"), want: scoreDiscoverIntentBoost},
		{name: "path disambiguator", entry: resolve, terms: searchTermsFromWords("url", "path"), want: scoreDiscoverIntentBoost},
		{name: "resolve disambiguator", entry: resolve, terms: searchTermsFromWords("url", "resolve"), want: scoreDiscoverIntentBoost},
		{name: "discover disambiguator", entry: resolve, terms: searchTermsFromWords("url", "discover"), want: scoreDiscoverIntentBoost},
		{name: "find disambiguator", entry: resolve, terms: searchTermsFromWords("url", "find"), want: scoreDiscoverIntentBoost},
		{name: "no trigger", entry: resolve, terms: searchTermsFromWords("project"), want: 0},
		{name: "no disambiguator", entry: resolve, terms: searchTermsFromWords("url"), want: 0},
		{
			name:  "the discover domain with another action",
			entry: scoringEntry("discover_project.lookup", "discover_project", "lookup"),
			terms: searchTermsFromWords("url", "project"),
			want:  0,
		},
		{
			name:  "a resolve action in another domain",
			entry: scoringEntry("project.resolve", "project", "resolve"),
			terms: searchTermsFromWords("url", "project"),
			want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreDiscoverProjectIntentValue(tc.entry, tc.terms); got != tc.want {
				t.Fatalf("scoreDiscoverProjectIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("the reason names the action", func(t *testing.T) {
		score, reason := scoreDiscoverProjectIntent(resolve, searchTermsFromWords("remote", "project"))
		if score != scoreDiscoverIntentBoost {
			t.Fatalf("scoreDiscoverProjectIntent() = %d, want %d", score, scoreDiscoverIntentBoost)
		}
		if reason.Field != searchFieldDiscoverIntent || reason.MatchedValue != "discover_project.resolve" {
			t.Fatalf("scoreDiscoverProjectIntent() reason = %+v, want the discover intent reason", reason)
		}
	})

	t.Run("a query that does not discover carries no reason", func(t *testing.T) {
		score, reason := scoreDiscoverProjectIntent(resolve, searchTermsFromWords("project"))
		if score != 0 || reason != (MatchReason{}) {
			t.Fatalf("scoreDiscoverProjectIntent() = %d, %+v; want zero result", score, reason)
		}
	})
}

// TestScoreProjectGetIntentValue_EachDisambiguatorAlone verifies each of the
// five words that pair with "project" to mean the single-project read, one word
// at a time. Driven through the normalizer, "show" drags "get" and "list" in
// with it, so the loop above cannot tell which word carried the boost, and a
// disambiguator that stopped counting would only show up on the phrasing that
// uses it.
func TestScoreProjectGetIntentValue_EachDisambiguatorAlone(t *testing.T) {
	entry := scoringEntry("project.get", "project", "get")

	cases := []struct {
		name  string
		entry actionEntry
		terms []searchTerm
		want  int
	}{
		{name: "get", entry: entry, terms: searchTermsFromWords("project", "get"), want: scoreProjectGetIntentBoost},
		{name: "show", entry: entry, terms: searchTermsFromWords("project", "show"), want: scoreProjectGetIntentBoost},
		{name: "find", entry: entry, terms: searchTermsFromWords("project", "find"), want: scoreProjectGetIntentBoost},
		{name: "path", entry: entry, terms: searchTermsFromWords("project", "path"), want: scoreProjectGetIntentBoost},
		{name: "id", entry: entry, terms: searchTermsFromWords("project", "id"), want: scoreProjectGetIntentBoost},
		{name: "project alone", entry: entry, terms: searchTermsFromWords("project"), want: 0},
		{name: "a disambiguator without the project noun", entry: entry, terms: searchTermsFromWords("show"), want: 0},
		{
			name:  "the project domain with another action",
			entry: scoringEntry("project.list", "project", "list"),
			terms: searchTermsFromWords("project", "show"),
			want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreProjectGetIntentValue(tc.entry, tc.terms); got != tc.want {
				t.Fatalf("scoreProjectGetIntentValue() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestScoreSearchProjectsIntentValue_ConcreteNeedleSumsThreeBoosts pins the
// concrete-needle score at the sum of its three parts rather than at "more than
// the plain score". That sum is what has to beat project.get on a query naming
// a project by name, so a term dropped from it, or subtracted instead of added,
// leaves the search ranked below the single-project read while still scoring
// higher than the plain case.
func TestScoreSearchProjectsIntentValue_ConcreteNeedleSumsThreeBoosts(t *testing.T) {
	entry := scoringEntry("search.projects", "search", "projects")

	got := scoreSearchProjectsIntentValue(entry, searchTermsFromWords("search", "projects", "platform"))
	want := scoreSearchProjectsBoost + scoreProjectGetIntentBoost + scoreCompoundTagBoost
	if got != want {
		t.Fatalf("scoreSearchProjectsIntentValue(concrete needle) = %d, want %d", got, want)
	}
}

// TestSearchProjectsQueryHasConcreteNeedle_GenericWordsAlone verifies that each
// generic word is passed over on its own. The needle test decides whether a
// query names something to search for, and a generic word counted as a needle
// would give "show all projects" the boost meant for a query that names one.
func TestSearchProjectsQueryHasConcreteNeedle_GenericWordsAlone(t *testing.T) {
	generic := []string{"project", "projects", "search", "list", "find", "all", "show", "get", "read"}

	for _, word := range generic {
		t.Run(word, func(t *testing.T) {
			if searchProjectsQueryHasConcreteNeedle(searchTermsFromWords(word)) {
				t.Fatalf("searchProjectsQueryHasConcreteNeedle(%q) = true, want false", word)
			}
		})
	}

	t.Run("all of them together", func(t *testing.T) {
		if searchProjectsQueryHasConcreteNeedle(searchTermsFromWords(generic...)) {
			t.Fatal("searchProjectsQueryHasConcreteNeedle(generic) = true, want false")
		}
	})

	t.Run("no terms at all", func(t *testing.T) {
		if searchProjectsQueryHasConcreteNeedle(nil) {
			t.Fatal("searchProjectsQueryHasConcreteNeedle(nil) = true, want false")
		}
	})

	t.Run("a name among the generic words", func(t *testing.T) {
		if !searchProjectsQueryHasConcreteNeedle(searchTermsFromWords("search", "projects", "platform")) {
			t.Fatal("searchProjectsQueryHasConcreteNeedle(named project) = false, want true")
		}
	})
}

// TestScoreActionSpecificity_PenalizesOnlyUnmatchedMultiWordActions pins the
// specificity penalty at its exact value and pins the two cases that must not
// be penalized. The penalty is one unit per action word the query never
// mentioned, so it has to grow with the count: charged once for two unmatched
// words, a three-word action would rank level with a one-word action the query
// matched. The single-word case is excluded on purpose, since the domain
// already carries that word, and an action whose every word was named is not
// unspecific at all, which is why it also carries no reason line.
func TestScoreActionSpecificity_PenalizesOnlyUnmatchedMultiWordActions(t *testing.T) {
	multiWord := scoringEntry("group.service_account_list", "group", "service_account_list")
	singleWord := scoringEntry("project.get", "project", "get")

	t.Run("two of three action words unmatched", func(t *testing.T) {
		terms := searchTermsFromWords("list")
		want := 2 * scoreUnmatchedActionWord
		if got := scoreActionSpecificityValue(multiWord, terms); got != want {
			t.Fatalf("scoreActionSpecificityValue() = %d, want %d", got, want)
		}
		score, reason := scoreActionSpecificity(multiWord, terms)
		if score != want {
			t.Fatalf("scoreActionSpecificity() = %d, want %d", score, want)
		}
		if reason.Field != searchFieldSpecificity || reason.MatchedValue != "service_account_list" || reason.Score != want {
			t.Fatalf("scoreActionSpecificity() reason = %+v, want the specificity reason at %d", reason, want)
		}
	})

	t.Run("one of three action words unmatched", func(t *testing.T) {
		terms := searchTermsFromWords("service", "account")
		if got := scoreActionSpecificityValue(multiWord, terms); got != scoreUnmatchedActionWord {
			t.Fatalf("scoreActionSpecificityValue() = %d, want %d", got, scoreUnmatchedActionWord)
		}
	})

	t.Run("every action word matched", func(t *testing.T) {
		terms := searchTermsFromWords("service", "account", "list")
		if got := scoreActionSpecificityValue(multiWord, terms); got != 0 {
			t.Fatalf("scoreActionSpecificityValue() = %d, want 0", got)
		}
		score, reason := scoreActionSpecificity(multiWord, terms)
		if score != 0 || reason != (MatchReason{}) {
			t.Fatalf("scoreActionSpecificity() = %d, %+v; want zero result", score, reason)
		}
	})

	t.Run("a single-word action is never penalized", func(t *testing.T) {
		terms := searchTermsFromWords("issue")
		if got := scoreActionSpecificityValue(singleWord, terms); got != 0 {
			t.Fatalf("scoreActionSpecificityValue() = %d, want 0", got)
		}
		score, reason := scoreActionSpecificity(singleWord, terms)
		if score != 0 || reason != (MatchReason{}) {
			t.Fatalf("scoreActionSpecificity() = %d, %+v; want zero result", score, reason)
		}
	})
}

// TestClassifyVerbIntent_EveryVerbItRecognizes pins the class of each verb the
// classifier knows. The class decides which actions a query is nudged toward,
// and a verb that fell out of its list is silently reclassified as no intent at
// all, which removes the nudge instead of changing it: nothing else in the
// scorer would report the loss.
func TestClassifyVerbIntent_EveryVerbItRecognizes(t *testing.T) {
	cases := []struct {
		intent verbIntent
		verbs  []string
	}{
		{intent: verbIntentRead, verbs: []string{"get", "list", "read", "show", "fetch", "find", "search", "download"}},
		{intent: verbIntentWrite, verbs: []string{"create", "add", "new", "update", "edit", "set", "enable", "register"}},
		{intent: verbIntentDestructive, verbs: []string{"delete", "destroy", "remove", "revoke", "purge"}},
		{intent: verbIntentWorkflow, verbs: []string{"run", "rerun", "retry", "trigger", "play", "start", "cancel", "stop", "merge", "protect", "unprotect"}},
		{intent: verbIntentDiagnostic, verbs: []string{"debug", "diagnose", "inspect", "status", "log", "logs", "trace", "lint", "test"}},
	}

	for _, tc := range cases {
		for _, verb := range tc.verbs {
			t.Run(string(tc.intent)+"/"+verb, func(t *testing.T) {
				if got := classifyVerbIntent(verb); got != tc.intent {
					t.Fatalf("classifyVerbIntent(%q) = %q, want %q", verb, got, tc.intent)
				}
			})
		}
	}

	t.Run("a word that is not a verb", func(t *testing.T) {
		if got := classifyVerbIntent("pipeline"); got != "" {
			t.Fatalf("classifyVerbIntent(pipeline) = %q, want no intent", got)
		}
	})
}

// TestQueryVerbIntent_HighestPrecedenceWins verifies that a query carrying
// several verbs is read as the most consequential one, whichever order they
// arrive in. A destructive word anywhere in the query has to outrank the read
// verb beside it, or "get the pipeline and delete it" is scored as a read and
// the destructive action it names is never confirmed.
func TestQueryVerbIntent_HighestPrecedenceWins(t *testing.T) {
	cases := []struct {
		name  string
		terms []searchTerm
		want  verbIntent
	}{
		{name: "one read verb", terms: searchTermsFromWords("get"), want: verbIntentRead},
		{name: "one write verb", terms: searchTermsFromWords("create"), want: verbIntentWrite},
		{name: "read then destructive", terms: searchTermsFromWords("get", "delete"), want: verbIntentDestructive},
		{name: "destructive then read", terms: searchTermsFromWords("delete", "get"), want: verbIntentDestructive},
		{name: "destructive outranks diagnostic", terms: searchTermsFromWords("log", "delete"), want: verbIntentDestructive},
		{name: "workflow outranks write", terms: searchTermsFromWords("update", "retry"), want: verbIntentWorkflow},
		{name: "no verb at all", terms: searchTermsFromWords("pipeline", "project"), want: ""},
		{name: "no terms", terms: nil, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := queryVerbIntent(tc.terms); got != tc.want {
				t.Fatalf("queryVerbIntent() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestActionNameClassifiers_EachKeywordStandsAlone verifies that every keyword
// each action-name classifier looks for is enough on its own. These five
// classifiers decide which verb intent an action answers, so a keyword that
// stopped counting would quietly stop matching the actions named after it, and
// only for the domains that spell the operation that way.
func TestActionNameClassifiers_EachKeywordStandsAlone(t *testing.T) {
	cases := []struct {
		name       string
		classifier func(string) bool
		trueFor    []string
		falseFor   []string
	}{
		{
			name:       "isReadAction",
			classifier: isReadAction,
			trueFor:    []string{"get", "list", "get_config", "list_items", "status", "log", "report", "raw", "content"},
			falseFor:   []string{"create", "delete", "merge"},
		},
		{
			name:       "isWriteAction",
			classifier: isWriteAction,
			trueFor:    []string{"create", "add", "update", "edit", "set", "enable", "register", "approve"},
			falseFor:   []string{"get", "delete", "retry"},
		},
		{
			name:       "isDestructiveActionName",
			classifier: isDestructiveActionName,
			trueFor:    []string{"delete", "remove", "revoke", "destroy"},
			falseFor:   []string{"get", "create", "retry"},
		},
		{
			name:       "isWorkflowAction",
			classifier: isWorkflowAction,
			trueFor:    []string{"retry", "trigger", "play", "run", "merge", "protect", "cancel"},
			falseFor:   []string{"get", "list", "delete"},
		},
		{
			name:       "isDiagnosticAction",
			classifier: isDiagnosticAction,
			trueFor:    []string{"status", "log", "trace", "lint", "test", "health"},
			falseFor:   []string{"get", "create", "delete"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, action := range tc.trueFor {
				t.Run("matches "+action, func(t *testing.T) {
					if !tc.classifier(action) {
						t.Fatalf("%s(%q) = false, want true", tc.name, action)
					}
				})
			}
			for _, action := range tc.falseFor {
				t.Run("passes over "+action, func(t *testing.T) {
					if tc.classifier(action) {
						t.Fatalf("%s(%q) = true, want false", tc.name, action)
					}
				})
			}
		})
	}
}

// TestFormatSearchOutput_OmittedSectionsAndEmptyCells verifies the three places
// the search table chooses between a value and nothing at all. Each of them
// reads a length, and a section rendered from an empty list prints its label
// over nothing: an empty "Try:" line, a disambiguation line naming no action,
// or a Required Params cell that reads as if the action took none.
func TestFormatSearchOutput_OmittedSectionsAndEmptyCells(t *testing.T) {
	const header = "| Action ID | Score | Destructive | Required Params |\n| --- | --- | --- | --- |\n"

	t.Run("no suggestions falls back to the broader-terms hint", func(t *testing.T) {
		want := "## GitLab Catalog: no matching action\n\n- **Query**: `zzzz`\n" + wantHintsBlock(wantBroaderTermsHint)
		if got := formatSearchOutput(SearchOutput{Query: "zzzz"}); got != want {
			t.Fatalf("formatSearchOutput() = %q, want %q", got, want)
		}
	})

	t.Run("unambiguous results carry no disambiguation hint", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n- **Query**: `project get`\n\n" + header +
			"| `project.get` | 0 | - | `project_id` |\n" +
			wantHintsBlock(wantFullSchemaHint, dynamicExecuteEnvelopeHint)
		got := formatSearchOutput(SearchOutput{
			Query:   "project get",
			Count:   1,
			Results: []SearchResult{{ID: "project.get", RequiredParams: []string{"project_id"}}},
		})
		if got != want {
			t.Fatalf("formatSearchOutput() = %q, want %q", got, want)
		}
	})

	t.Run("ambiguous results name the targets", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n- **Query**: `list`\n\n" + header +
			"| `project.list` | 0 | - | - |\n" +
			wantHintsBlock("Use one canonical action ID explicitly: `group.list`.", wantFullSchemaHint, dynamicExecuteEnvelopeHint)
		got := formatSearchOutput(SearchOutput{
			Query:   "list",
			Count:   1,
			Results: []SearchResult{{ID: "project.list", AmbiguousWith: []string{"group.list"}}},
		})
		if got != want {
			t.Fatalf("formatSearchOutput() = %q, want %q", got, want)
		}
	})

	t.Run("an action with no required params shows a dash", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n- **Query**: `current user`\n\n" + header +
			"| `user.current` | 0 | - | - |\n" +
			wantHintsBlock(wantFullSchemaHint, dynamicExecuteEnvelopeHint)
		got := formatSearchOutput(SearchOutput{
			Query:   "current user",
			Count:   1,
			Results: []SearchResult{{ID: "user.current"}},
		})
		if got != want {
			t.Fatalf("formatSearchOutput() = %q, want %q", got, want)
		}
	})
}

// TestFormatDescribeOutput_OptionalSections verifies that each optional line of
// a described action is written when it has content and left out when it does
// not. A label with nothing after it is worse than a missing line here: the
// description is what a model reads before its first call, and an empty
// "Required params" line reads as an action that takes none.
func TestFormatDescribeOutput_OptionalSections(t *testing.T) {
	hints := wantHintsBlock(wantDescribeExecuteHint, dynamicExecuteEnvelopeHint)

	t.Run("everything the action carries is rendered", func(t *testing.T) {
		want := "## GitLab Catalog: 1 action described\n\n" +
			"### project.create\n\n" +
			"- **Tool**: `gitlab_project`\n" +
			"- **Action**: `create`\n" +
			"- **Usage**: creates a project in a namespace\n" +
			"- **Required params**: `name`, `path`\n" +
			"- **Related actions**: `project.get`\n" +
			"- **Parameter guidance**: `path`: the URL slug.\n" +
			"- **Schema URI**: `gitlab://tools/project.create`\n\n" +
			"#### Input schema\n\n" +
			"```json\n{\"type\":\"object\"}\n```\n\n" +
			"#### Example call\n\n" +
			"```json\n{\"tool\":\"gitlab_execute_action\",\"arguments\":{\"action\":\"project.create\",\"params\":{\"name\":\"demo\"}}}\n```\n" +
			hints
		got := formatDescribeOutput(DescribeOutput{Count: 1, Actions: []ActionDescription{{
			ID:             "project.create",
			Tool:           "gitlab_project",
			Action:         "create",
			Destructive:    false,
			Usage:          "creates a project in a namespace",
			RequiredParams: []string{"name", "path"},
			RelatedActions: []string{"project.get"},
			ParamGuidance:  map[string]toolutil.ParameterGuidance{"path": {ValueSource: "the URL slug"}},
			SchemaURI:      "gitlab://tools/project.create",
			InputSchema:    map[string]any{"type": "object"},
			Example: ActionExample{
				Tool:      "gitlab_execute_action",
				Arguments: map[string]any{"action": "project.create", "params": map[string]any{"name": "demo"}},
			},
		}}})
		if got != want {
			t.Fatalf("formatDescribeOutput() = %q, want %q", got, want)
		}
	})

	t.Run("what the action does not carry is left out", func(t *testing.T) {
		want := "## GitLab Catalog: 1 action described\n\n" +
			"### user.current\n\n" +
			"- **Tool**: `gitlab_user`\n" +
			"- **Action**: `current`\n" +
			"- **Schema URI**: `gitlab://tools/user.current`\n" +
			hints
		got := formatDescribeOutput(DescribeOutput{Count: 1, Actions: []ActionDescription{{
			ID:        "user.current",
			Tool:      "gitlab_user",
			Action:    "current",
			SchemaURI: "gitlab://tools/user.current",
		}}})
		if got != want {
			t.Fatalf("formatDescribeOutput() = %q, want %q", got, want)
		}
	})

	t.Run("a destructive action is marked with the warning sign", func(t *testing.T) {
		want := "## GitLab Catalog: 1 action described\n\n" +
			"### project.delete\n\n" +
			"- **Tool**: `gitlab_project`\n" +
			"- **Action**: `delete`\n" +
			"- " + toolutil.EmojiWarning + " **Destructive**\n" +
			"- **Schema URI**: `gitlab://tools/project.delete`\n" +
			hints
		got := formatDescribeOutput(DescribeOutput{Count: 1, Actions: []ActionDescription{{
			ID:          "project.delete",
			Tool:        "gitlab_project",
			Action:      "delete",
			Destructive: true,
			SchemaURI:   "gitlab://tools/project.delete",
		}}})
		if got != want {
			t.Fatalf("formatDescribeOutput() = %q, want %q", got, want)
		}
	})
}

// TestFormatFindOutput_RequiredParamsCell verifies the one cell of the find
// table that is written from a length. An action with no required parameters
// has to say so with a dash: an empty cell in a Markdown table reads as a
// missing value rather than as "none needed", and the row beside it is what a
// model copies its first call from.
func TestFormatFindOutput_RequiredParamsCell(t *testing.T) {
	const header = "| Action ID | Score | Destructive | Required Params |\n| --- | --- | --- | --- |\n"
	hints := wantHintsBlock(wantExecuteNowHint, dynamicExecuteEnvelopeHint, wantStructuredResultsHint)

	t.Run("no required params", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n- **Query**: `current user`\n\n" + header +
			"| `user.current` | 10 | - | - |\n" + hints
		got := formatFindOutput(FindOutput{
			Query:   "current user",
			Count:   1,
			Results: []FindResult{{ID: "user.current", Score: 10}},
		}, nil)
		if got != want {
			t.Fatalf("formatFindOutput() = %q, want %q", got, want)
		}
	})

	t.Run("required params are listed", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n- **Query**: `project get`\n\n" + header +
			"| `project.get` | 10 | - | `project_id` |\n" + hints
		got := formatFindOutput(FindOutput{
			Query:   "project get",
			Count:   1,
			Results: []FindResult{{ID: "project.get", Score: 10, RequiredParams: []string{"project_id"}}},
		}, nil)
		if got != want {
			t.Fatalf("formatFindOutput() = %q, want %q", got, want)
		}
	})
}

// TestFormatFindOutput_LowConfidenceAndAmbiguityAreSaidOutLoud verifies that a
// finder telling the model to execute the top row now also says when that row
// is a guess. The registry computes LowConfidence and AmbiguousWith and the
// table shows neither, so without these two hints the card's first sentence
// sends the model to act on a match the server already knows is uncertain.
func TestFormatFindOutput_LowConfidenceAndAmbiguityAreSaidOutLoud(t *testing.T) {
	const header = "| Action ID | Score | Destructive | Required Params |\n| --- | --- | --- | --- |\n"

	t.Run("a low-confidence top result is named", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n- **Query**: `list`\n\n" + header +
			"| `project.list` | 10 | - | - |\n" +
			wantHintsBlock(
				wantExecuteNowHint,
				"Top result `project.list` is low confidence: read the rows and pick the intended action rather than taking the first.",
				dynamicExecuteEnvelopeHint,
				wantStructuredResultsHint,
			)
		got := formatFindOutput(FindOutput{
			Query:   "list",
			Count:   1,
			Results: []FindResult{{ID: "project.list", Score: 10, LowConfidence: true}},
		}, nil)
		if got != want {
			t.Fatalf("formatFindOutput() = %q, want %q", got, want)
		}
	})

	t.Run("an ambiguous alias names the actions it resolves to", func(t *testing.T) {
		want := "## GitLab Catalog: 1 matching action\n\n- **Query**: `list`\n\n" + header +
			"| `project.list` | 10 | - | - |\n" +
			wantHintsBlock(
				wantExecuteNowHint,
				"Use one canonical action ID explicitly: `group.list`, `project.list`.",
				dynamicExecuteEnvelopeHint,
				wantStructuredResultsHint,
			)
		got := formatFindOutput(FindOutput{
			Query:   "list",
			Count:   1,
			Results: []FindResult{{ID: "project.list", Score: 10, AmbiguousWith: []string{"project.list", "group.list"}}},
		}, nil)
		if got != want {
			t.Fatalf("formatFindOutput() = %q, want %q", got, want)
		}
	})
}

// TestSelectColumns_HeaderAndRowAreFilteredIdentically verifies the one
// mechanism that keeps a row from carrying a cell its header never declared:
// both go through selectColumns with the same flags, over one column order.
// A row with an extra cell renders as a broken table, and a reader that lays
// a table out by its header drops the extra value on the floor.
func TestSelectColumns_HeaderAndRowAreFilteredIdentically(t *testing.T) {
	row := actionRow{id: "project.get", score: 10, requiredParams: []string{"project_id"}, guidance: "g", why: "w"}
	cases := []struct {
		name string
		cols actionColumns
		want []string
	}{
		{name: "neither", cols: actionColumns{}, want: []string{"Action ID", "Score", "Destructive", "Required Params"}},
		{name: "guidance", cols: actionColumns{guidance: true}, want: []string{"Action ID", "Score", "Destructive", "Required Params", "Guidance"}},
		{name: "why", cols: actionColumns{why: true}, want: []string{"Action ID", "Score", "Destructive", "Required Params", "Why"}},
		{name: "both", cols: actionColumns{guidance: true, why: true}, want: actionMatchColumns},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header := selectColumns(actionMatchColumns, tc.cols)
			if !slices.Equal(header, tc.want) {
				t.Fatalf("selectColumns(header) = %v, want %v", header, tc.want)
			}
			if cells := actionRowCells(row, tc.cols); len(cells) != len(header) {
				t.Fatalf("actionRowCells() = %v cells, want the header's %d", cells, len(header))
			}
		})
	}

	t.Run("every column has a keep decision", func(t *testing.T) {
		if got, want := len(actionColumns{}.kept()), len(actionMatchColumns); got != want {
			t.Fatalf("kept() = %d decisions, want one per column (%d)", got, want)
		}
	})
}

// TestFormatFindOutput_EveryRowHasAsManyCellsAsTheHeader verifies the same
// invariant end to end, on the document the finder writes: whatever the
// result carries, the rendered row has the cells the rendered header
// declared.
func TestFormatFindOutput_EveryRowHasAsManyCellsAsTheHeader(t *testing.T) {
	explanation := &ScoringExplanation{
		TotalScore:   200,
		MatchedTerms: 2,
		Reasons: []MatchReason{{
			Field:        searchFieldCanonicalID,
			QueryTerm:    "project",
			MatchedValue: "project.get",
			Score:        scoreCanonicalExact,
		}},
	}

	cases := []struct {
		name   string
		result FindResult
	}{
		{name: "neither guidance nor explanations", result: FindResult{ID: "project.get", Score: 10}},
		{name: "guidance only", result: FindResult{ID: "project.delete", Score: 10, Destructive: true}},
		{name: "explanations only", result: FindResult{ID: "project.get", Score: 10, Explanation: explanation}},
		{name: "both", result: FindResult{ID: "project.delete", Score: 10, Destructive: true, Explanation: explanation}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := formatFindOutput(FindOutput{Query: "project", Count: 1, Results: []FindResult{tc.result}}, nil)

			headerCells := 0
			for line := range strings.SplitSeq(out, "\n") {
				switch {
				case strings.HasPrefix(line, "| Action ID "):
					headerCells = strings.Count(line, "|") - 1
				case strings.HasPrefix(line, "| `"):
					if cells := strings.Count(line, "|") - 1; cells != headerCells {
						t.Fatalf("row %q has %d cells, want the header's %d, in %q", line, cells, headerCells, out)
					}
				}
			}
			if headerCells == 0 {
				t.Fatalf("formatFindOutput() = %q, want a table header", out)
			}
		})
	}
}

// TestHasFindGuidance_EachSignalAlone verifies that any one of the three
// signals opens the Guidance column. Parameter guidance without a usage note or
// a destructive flag is the case the column exists for, and a result carrying
// only that would otherwise have its guidance computed and then dropped,
// because the column it belongs in was never opened.
func TestHasFindGuidance_EachSignalAlone(t *testing.T) {
	cases := []struct {
		name   string
		result FindResult
		want   bool
	}{
		{name: "destructive alone", result: FindResult{ID: "project.delete", Destructive: true}, want: true},
		{name: "usage alone", result: FindResult{ID: "project.get", Usage: "reads one project"}, want: true},
		{name: "blank usage is not a signal", result: FindResult{ID: "project.get", Usage: "   "}, want: false},
		{
			name: "parameter guidance alone",
			result: FindResult{
				ID:            "project.get",
				ParamGuidance: map[string]toolutil.ParameterGuidance{"project_id": {ValueSource: "the project path"}},
			},
			want: true,
		},
		{name: "nothing to say", result: FindResult{ID: "project.get"}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasFindGuidance([]FindResult{tc.result}); got != tc.want {
				t.Fatalf("hasFindGuidance() = %v, want %v", got, tc.want)
			}
		})
	}
}
