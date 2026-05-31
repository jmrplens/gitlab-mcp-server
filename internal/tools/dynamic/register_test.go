package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// TestSearch_ExplainIsOptIn verifies that ranking explanations are omitted by
// default and returned only when requested by the caller.
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

// TestSearch_IncludesCuratedRelatedActions verifies compact search results keep
// workflow hints in structured fields without enabling scoring explanations.
func TestSearch_IncludesCuratedRelatedActions(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "analyze.release_notes", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Results[0].ID != "analyze.release_notes" {
		t.Fatalf("Search() output = %+v, want analyze.release_notes", output)
	}
	if !slices.Contains(output.Results[0].RelatedActions, "repository.compare") {
		t.Fatalf("RelatedActions = %v, want repository.compare", output.Results[0].RelatedActions)
	}
	if output.Results[0].Explanation != nil {
		t.Fatalf("Explanation = %+v, want nil by default", output.Results[0].Explanation)
	}
}

// TestCompactParameterGuidance_PrioritizesRequiredParamsAndShowsTruncation verifies compact parameter hints surface the most useful items first.
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

// TestCompactParameterGuidanceItem_FormatsAvailableHints verifies each parameter guidance hint source has a compact Markdown form.
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

// TestSearch_ReturnsNextStep verifies search results guide the next selection
// step without forcing extra discovery.
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
	if !strings.Contains(markdown, "Next step:") {
		t.Fatalf("Search() markdown = %q, want next step guidance", markdown)
	}
	if strings.Contains(markdown, "extra discovery") {
		t.Fatalf("Search() markdown still forces extra discovery: %s", markdown)
	}
}

// TestSearch_NoMatchSuggestsNearbyTokens verifies empty searches still return a
// small recovery hint instead of dumping the full catalog.
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

// TestSearch_RequiresQuery verifies that Search returns an MCP tool error when
// the caller omits the query text.
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

// TestSearch_RanksAliasMatches verifies that human-friendly aliases such as
// "webhook create" rank the canonical project hook action first.
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

// TestSearch_UsesIntentSynonymsAndTags verifies that Search expands common
// intent words and tags before ranking dynamic actions.
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

// TestSearch_ExactCanonicalIDBeatsBroadText verifies that an exact canonical
// action ID outranks broader textual matches for the same domain.
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

// TestSearch_CurrentHighConfidenceQueriesRemainStable protects the current
// production-catalog top results before the ranker is refactored.
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
	baseCatalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	baseCatalog, err = AddStandaloneCatalog(baseCatalog, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	baseRegistry := NewRegistryFromCatalog(baseCatalog)
	enterpriseCatalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog(enterprise) error = %v", err)
	}
	enterpriseCatalog, err = AddStandaloneCatalog(enterpriseCatalog, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog(enterprise) error = %v", err)
	}
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

// TestAddStandaloneRoutes_AddsDynamicActions verifies that standalone dynamic
// routes are indexed alongside captured meta-tool routes.
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
		if _, ok := fromRoutes.resolveAction(actionID); !ok {
			t.Fatalf("route wrapper registry missing %s", actionID)
		}
		if _, ok := fromCatalog.resolveAction(actionID); !ok {
			t.Fatalf("catalog registry missing %s", actionID)
		}
	}
}

// TestAddStandaloneCatalog_NilCatalogWithExcludedInteractiveActions verifies
// nil catalogs are supported and no empty interactive group is added.
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

// TestNewRegistryFromCatalog_NilCatalog verifies callers can pass a nil catalog
// during transitional setup without panicking.
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

// TestDescribe_CanonicalizesStandaloneAlias verifies that Describe resolves a
// standalone MCP tool name to its canonical dynamic action ID.
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

// TestDescribe_ReturnsSchemaAndExample verifies that Describe returns action
// metadata, destructive hints, input schema, and an executable example.
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
	if description, _ := schema["description"].(string); !strings.Contains(description, "no captured parameter schema") {
		t.Fatalf("schema description = %q, want fallback guidance", description)
	}
}

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
}

// TestDescribe_IncludesOutputSchema verifies that dynamic descriptions expose
// the action result schema when the backing catalog route has one.
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

// TestDescribe_MetaCatalogSchemas verifies that Describe returns input schemas
// and includes output schemas when route metadata provides them.
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
		if strings.Contains(markdown, notWant) {
			t.Fatalf("Describe() markdown contains %q: %s", notWant, markdown)
		}
	}
	if !strings.Contains(markdown, "**Input schema**") || !strings.Contains(markdown, "```json") || !strings.Contains(markdown, "properties") {
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
		"analyze.mr_changes",
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

// TestSearch_WhyThisActionOnlyAppearsForCloseAlternatives verifies compact
// action explanations do not appear on straightforward search results.
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
	for _, want := range []string{
		"Immediate next step: choose one row and call `gitlab_execute_action` now",
		"Next step: choose one row and call `gitlab_execute_action`",
		"before starting another catalog operation",
		"top-level `confirm:true`",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("Find() markdown = %q, want %q", markdown, want)
		}
	}
}

// TestFind_ExplainIsOptIn verifies that find keeps its default payload compact
// and exposes scoring reasons only when explicitly requested.
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

// TestFind_RequiresQuery verifies that Find returns an MCP tool error and empty
// output when the query is omitted.
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

// TestDescribe_UnknownActionReturnsToolError verifies that Describe reports an
// MCP tool error for action IDs that are not present in the registry.
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

// TestDescribe_CanonicalizesAlias verifies that Describe resolves compatibility
// aliases to the canonical action ID before returning metadata.
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
		"release.generate_notes":                    "analyze.release_notes",
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
		"group.ldap_link_delete":                    "group.ldap_link_delete_for_provider",
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

// TestDescribe_CanonicalizesProviderSpecificAliases verifies alternate action
// IDs observed in provider output against the real action catalog.
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

// TestDescribe_IncludesDisambiguationUsage verifies high-confusion actions carry
// usage notes that distinguish adjacent GitLab APIs.
func TestDescribe_IncludesDisambiguationUsage(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := map[string]string{
		"admin.settings_get":               "GitLab application settings",
		"access.deploy_key_list_project":   "deploy keys, not deploy tokens",
		"access.deploy_token_list_project": "deploy tokens/credentials",
		"environment.protected_get":        "protected environment",
		"environment.deployment_list":      "Lists deployments",
		"feature_flags.ff_user_list_get":   "user_list_iid",
		"issue.update":                     "state_event",
		"issue.note_get":                   "params.note_id",
		"job.download_single_artifact":     "one artifact file path",
		"merge_request.merge":              "auto_merge=true",
		"mr_review.draft_note_publish_all": "Publishes all pending draft MR review notes",
		"package.list":                     "created_at, name, version, or type",
		"pipeline.wait":                    "existing pipeline_id",
		"runner.remove":                    "numeric runner_id",
		"release.link_create":              "absolute http, https, or ftp URL",
		"release.link_get":                 "release asset link by link_id",
		"repository.compare":               "params.from and params.to",
		"search.code":                      "file contents",
		"search.projects":                  "project name",
		"analyze.release_notes":            "after requested release/compare",
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
			if actionID == "repository.compare" && !slices.Contains(description.RelatedActions, "analyze.release_notes") {
				t.Fatalf("RelatedActions = %v, want analyze.release_notes", description.RelatedActions)
			}
			if actionID == "repository.compare" && !strings.Contains(textContent(result), "Related actions") {
				t.Fatalf("Describe() markdown = %q, want related actions", textContent(result))
			}
		})
	}
}

// TestDescribe_IncludesConsolidatedRegisterMetaReplacementActions verifies Describe includes consolidated register meta replacement actions.
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

// TestDescribe_JobSingleArtifactRequiresArtifactPath verifies the dynamic
// schema exposes all values needed to download one artifact file.
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
		if !slices.Contains(description.RequiredParams, required) {
			t.Fatalf("required params = %v, want %s", description.RequiredParams, required)
		}
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

// TestExecute_UsesCatalogFormatter verifies that dynamic execution preserves
// the formatter attached to the backing catalog group.
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

// TestExecute_CanonicalizesAlias verifies that Execute resolves a compatibility
// alias before invoking the canonical action route.
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

// TestNormalizeActionScopedParamsWithExplanation verifies debug metadata is
// deterministic and records parameter names only.
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
		if !slices.ContainsFunc(aliases, func(alias actioncompat.ParameterAlias) bool { return alias.ActionID == actionID }) {
			t.Fatalf("ParameterAliases() = %+v, want action %s", aliases, actionID)
		}
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
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("register.go still owns compatibility policy table/helper %q; move policy to actioncompat", forbidden)
		}
	}
}

// TestExecute_ReportsUnknownAndMissingParamsBeforeDispatch verifies dynamic
// execute gives schema-aware repair guidance before route validation.
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
		if !strings.Contains(text, want) {
			t.Fatalf("Execute() error text = %q, want %q", text, want)
		}
	}
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
		if !strings.Contains(text, want) {
			t.Fatalf("Execute() error text = %q, want %q", text, want)
		}
	}
}

// TestExecute_RejectsIssueLifecycleAliasStateConflict verifies shorthand issue
// lifecycle aliases cannot execute the opposite state transition.
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

// TestExecute_UnknownActionSuggestsCanonicalIDs verifies that unknown actions
// return an MCP tool error with nearby canonical ID suggestions.
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

// TestExecute_RejectsAmbiguousAlias verifies that Execute refuses aliases that
// map to multiple canonical actions and reports the possible targets.
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

// TestDescribe_RejectsAmbiguousAlias verifies that Describe reports ambiguous
// aliases instead of choosing one canonical action arbitrarily.
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

// TestExecute_DestructiveActionRequiresConfirm verifies that destructive actions
// are blocked until the caller explicitly sets confirm=true.
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

// TestExecute_CurrentDestructiveSafetyRemainsStable protects the current
// destructive-action confirmation contract before ranker internals change.
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

// TestRegisterCatalogFindExecuteTools_ExposesDynamicTools verifies that the
// dynamic surface exposes find and execute through an MCP session.
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
	if findTool.Description != findToolDescription || !strings.Contains(findTool.Description, "read-only") {
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

// TestSearch_PartialMatchLongQuery verifies that incidental query terms do not
// suppress otherwise relevant merge request matches.
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

// TestSearch_NaturalLLMQueriesReturnActions verifies natural-language queries
// observed from LLMs still return the intended dynamic actions.
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
		if !slices.ContainsFunc(output.Results, func(r SearchResult) bool { return r.ID == want }) {
			t.Fatalf("Search() results = %+v, want %s", output.Results, want)
		}
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

// TestSearch_QueryShapeMatrix_ReturnsExpectedActions verifies short, long,
// typo-heavy, alias-based, and mixed queries against expected action IDs.
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
		{name: "release notes alias", query: "release generate notes", want: []string{"analyze.release_notes"}},
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

// TestSearch_FuzzyRecoveryQueriesReturnExpectedCandidates verifies typo recovery
// for common GitLab resource and workflow phrases.
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

// TestSearch_ProviderConfusionQueries_ReturnExpectedActions locks in the
// production catalog ranking for phrases that confused evaluated models.
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
		{name: "compare refs before release notes", query: "list releases compare refs from v1.0.0 to main then generate release notes", limit: 8, want: []string{"release.list", "repository.compare", "analyze.release_notes"}},
		{name: "generic package list", query: "list package registry packages", want: []string{"package.list"}},
		{name: "runner removal by id", query: "remove runner by numeric runner_id", want: []string{"runner.remove"}},
		{name: "issue time tracking sequence", query: "issue time tracking set estimate add spent time reset spent time reset estimate", limit: 8, want: []string{"issue.time_estimate_set", "issue.spent_time_add", "issue.spent_time_reset", "issue.time_estimate_reset"}},
		{name: "deploy token inventory", query: "list project deploy tokens credentials not deploy keys", limit: 8, want: []string{"access.deploy_token_list_project"}},
		{name: "project access token creation", query: "project access token create eval-token read_api expires_at 2026-12-31 for project my-org/tools/gitlab-mcp-server", limit: 8, want: []string{"access.token_project_create"}},
		{name: "protected environment deployment approval", query: "protected environment deployment_list deployment approve_or_reject", limit: 12, want: []string{"environment.protected_get", "environment.deployment_list", "environment.deployment_approve_or_reject"}},
		{name: "feature flag user list lifecycle", query: "feature flag user list get user_list_iid update delete", limit: 8, want: []string{"feature_flags.ff_user_list_get", "feature_flags.ff_user_list_update", "feature_flags.ff_user_list_delete"}},
		{name: "issue note lifecycle", query: "issue note get by note_id update delete comment", limit: 8, want: []string{"issue.note_get", "issue.note_update", "issue.note_delete"}},
		{name: "mr security analyzer intent", query: "LLM-assisted security review analyzer for merge request 1 in project my-org/tools/gitlab-mcp-server", limit: 8, want: []string{"analyze.mr_security"}},
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

func TestSearch_ProviderConfusionQueries_PrioritizeExactTopResult_EnterpriseCatalog(t *testing.T) {
	enterpriseCatalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog(enterprise) error = %v", err)
	}
	enterpriseCatalog, err = AddStandaloneCatalog(enterpriseCatalog, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog(enterprise) error = %v", err)
	}
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

// TestSearch_TypoQueryReturnsRelevantActions verifies that the fuzzy fallback
// recovers relevant merge request actions from typo-heavy query terms.
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
	"gitlab_analyze": {
		"release_notes": {
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return map[string]any{"release_notes": true}, nil
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
func TestRegistry_HelperCoverage(t *testing.T) {
	t.Run("annotations with nil base", func(t *testing.T) {
		got := annotationsWithTitle(nil, "Dynamic Search")
		if got == nil || got.Title != "Dynamic Search" {
			t.Fatalf("annotationsWithTitle(nil) = %+v, want title", got)
		}
	})

	t.Run("dedupe strings trims empty and duplicates", func(t *testing.T) {
		got := dedupeStrings([]string{" Project ", "", "project", "Issue"})
		want := []string{"project", "issue"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("dedupeStrings() = %v, want %v", got, want)
		}
	})

	t.Run("action tags include schema property hints", func(t *testing.T) {
		schema := map[string]any{"properties": map[string]any{
			"state_event": map[string]any{},
			"ref":         map[string]any{},
			"file_path":   map[string]any{},
			"url":         map[string]any{},
		}}
		got := actionTags("repository.file_create", "repository", "file_create", schema)
		for _, want := range []string{"repository file", "branch", "url", "close"} {
			if !stringInSlice(got, want) {
				t.Fatalf("actionTags() = %v, want %q", got, want)
			}
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
		if got := registry.segmentedSearchMatchesWithScorer(normalizeSearchTerms("project get"), defaultLimit, scoreEntryWithoutExplanation); got != nil {
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

// TestScoreSearchAlternative_ReturnsReason verifies that explanation metadata
// is stable enough for structured search debugging.
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

// TestSearchRuntimeMetrics_RecordQualitySignals verifies process-local search
// counters capture quality events without storing query text.
func TestSearchRuntimeMetrics_RecordQualitySignals(t *testing.T) {
	ResetSearchRuntimeMetrics()
	t.Cleanup(ResetSearchRuntimeMetrics)
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	registry.searchMatches("zzzzzzzz", 5, false)
	registry.searchMatches("merje requesy", 5, false)
	registry.searchMatches("danger.delete", 5, false)

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

// TestActionScopedParamValueConversions verifies action-scoped conversion
// helpers for issue state events, GitLab access levels, and boolean strings. It
// covers accepted inputs and rejected edge cases without external fixtures.
func TestActionScopedParamValueConversions(t *testing.T) {
	stateCases := map[any]string{"closed": "close", "OPEN": "reopen"}
	for input, want := range stateCases {
		got, ok := actioncompat.IssueStateEventValue(input)
		if !ok || got != want {
			t.Fatalf("issueStateEventValue(%v) = %q, %t; want %q, true", input, got, ok, want)
		}
	}
	if _, ok := actioncompat.IssueStateEventValue(123); ok {
		t.Fatal("issueStateEventValue(non-string) converted unexpectedly")
	}
	if _, ok := actioncompat.IssueStateEventValue("archived"); ok {
		t.Fatal("issueStateEventValue(archived) converted unexpectedly")
	}

	accessCases := map[any]int{
		10:             10,
		int64(20):      20,
		float64(30):    30,
		"40":           40,
		"guest":        10,
		"reporter":     20,
		"developer":    30,
		" maintainer ": 40,
		"owner":        50,
	}
	for input, want := range accessCases {
		got, ok := actioncompat.GitLabAccessLevelValue(input)
		if !ok || got != want {
			t.Fatalf("gitlabAccessLevelValue(%v) = %d, %t; want %d, true", input, got, ok, want)
		}
	}
	for _, input := range []any{float64(30.5), 70, int64(70), "70", "admin", true} {
		if got, ok := actioncompat.GitLabAccessLevelValue(input); ok {
			t.Fatalf("gitlabAccessLevelValue(%v) = %d, true; want false", input, got)
		}
	}

	if value, ok := actioncompat.BoolStringValue(" true "); !ok || !value {
		t.Fatalf("boolStringValue(true) = %t, %t; want true, true", value, ok)
	}
	for _, input := range []any{true, "not-bool"} {
		if _, ok := actioncompat.BoolStringValue(input); ok {
			t.Fatalf("boolStringValue(%v) converted unexpectedly", input)
		}
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
		if got, ok := NormalizeCompatibilityActionAlias(actionID); ok || got != strings.ToLower(strings.TrimSpace(actionID)) {
			t.Fatalf("NormalizeCompatibilityActionAlias(%q) = %q, %t; want unchanged false", actionID, got, ok)
		}
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

// TestScoredMatchesAndDestructiveFuzzyBranches verifies corrupted-index
// resilience and destructive fuzzy-match safety checks. The fixture keeps the
// dynamic registry in memory and injects invalid postings directly.
func TestScoredMatchesAndDestructiveFuzzyBranches(t *testing.T) {
	registry := NewRegistry(testRoutes(t))
	registry.SearchIndex.byToken["project"] = []int{-1, 0, len(registry.entries)}
	matches := registry.scoredMatches(normalizeSearchTerms("project"), scoreEntryWithoutExplanation)
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

// testEnumStringer holds test enum stringer data for the dynamic package.
type testEnumStringer string

// String returns the display label for testEnumStringer.
func (value testEnumStringer) String() string { return string(value) }

// TestSchemaSearchTermHelpers_Branches verifies schema descriptions and enum
// values are extracted from mixed JSON-schema shapes. It covers non-object
// properties, empty descriptions, stringers, numbers, booleans, and strings.
func TestSchemaSearchTermHelpers_Branches(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{
		"plain":      "not-object",
		"empty_desc": map[string]any{"description": "   "},
		"state": map[string]any{
			"description": "Merge request state",
			"enum":        []any{"opened", testEnumStringer("closed"), 30, true, struct{}{}},
		},
		"kind": map[string]any{"enum": []string{"bug", "feature"}},
	}}
	if descriptions := schemaPropertyDescriptions(schema); strings.Join(descriptions, ",") != "merge request state" {
		t.Fatalf("schemaPropertyDescriptions() = %v, want merge request state", descriptions)
	}
	enums := strings.Join(schemaPropertyEnumValues(schema), ",")
	for _, want := range []string{"opened", "closed", "30", "true", "bug", "feature"} {
		if !strings.Contains(enums, want) {
			t.Fatalf("schemaPropertyEnumValues() = %q, missing %q", enums, want)
		}
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
			params map[string]any
			want   bool
		}{
			{params: nil, want: false},
			{params: map[string]any{"confirm": false}, want: false},
			{params: map[string]any{"confirm": true}, want: true},
			{params: map[string]any{"confirm": " true "}, want: true},
			{params: map[string]any{"confirm": "yes"}, want: false},
			{params: map[string]any{"confirm": "no"}, want: false},
			{params: map[string]any{"confirm": 1}, want: false},
			{params: map[string]any{"confirm": int64(1)}, want: false},
			{params: map[string]any{"confirm": 1.0}, want: false},
			{params: map[string]any{"confirm": 2}, want: false},
		}
		for _, tt := range cases {
			if got := hasExplicitConfirm(tt.params); got != tt.want {
				t.Fatalf("hasExplicitConfirm(%v) = %v, want %v", tt.params, got, tt.want)
			}
		}
	})

	t.Run("format empty outputs", func(t *testing.T) {
		searchText := formatSearchOutput(SearchOutput{Query: "zzzz"})
		if !strings.Contains(searchText, "No catalog actions matched") {
			t.Fatalf("formatSearchOutput(empty) = %q, want no-match message", searchText)
		}
		findText := formatFindOutput(FindOutput{Query: "zzzz"})
		if !strings.Contains(findText, "No catalog actions matched") {
			t.Fatalf("formatFindOutput(empty) = %q, want no-match message", findText)
		}
	})

	t.Run("format find output explains execute params envelope", func(t *testing.T) {
		findText := formatFindOutput(FindOutput{
			Query: "release link create package asset",
			Count: 1,
			Results: []FindResult{{
				ID:             "release.link_create_batch",
				RequiredParams: []string{"project_id", "tag_name", "links"},
			}},
		})
		for _, want := range []string{"top-level `action`", "one `params` object", "Required Params key below belongs inside `params`"} {
			if !strings.Contains(findText, want) {
				t.Fatalf("formatFindOutput() = %q, want %q", findText, want)
			}
		}
	})
}

// TestAnnotationsWithTitle_CopiesBase verifies that annotation updates do not
// mutate the caller's base annotations. The dynamic tool registration uses this
// when assigning distinct titles to otherwise shared tool metadata.
func TestAnnotationsWithTitle_CopiesBase(t *testing.T) {
	base := &mcp.ToolAnnotations{Title: "Original", ReadOnlyHint: true}
	got := annotationsWithTitle(base, "Updated")
	if got == nil || got.Title != "Updated" || !got.ReadOnlyHint {
		t.Fatalf("annotationsWithTitle(base) = %+v, want copied read-only annotation with updated title", got)
	}
	if base.Title != "Original" {
		t.Fatalf("base title = %q, want unchanged Original", base.Title)
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

// TestAddServiceAccountPATActionTags_TagShapes verifies that service account PAT
// action tags include the expected resource string and verb tags.
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
	if v := scoreMRSecurityIntentValue(unrelated, terms); v != 0 {
		t.Fatalf("scoreMRSecurityIntentValue(unrelated) = %d, want 0", v)
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
	if score, reason := scoreMRSecurityIntent(unrelated, terms); score != 0 || reason != (MatchReason{}) {
		t.Fatalf("scoreMRSecurityIntent(unrelated) = %d, %v, want 0, empty", score, reason)
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

	for _, query := range queries {
		terms := normalizeSearchTerms(query)
		b.Run(benchmarkName(query)+"/indexed", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				matches := sortAndLimitMatches(registry.scoredMatches(terms, scoreEntryWithExplanation), 20)
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
			var out SearchOutput
			_, out, err = reg.Search(context.Background(), nil, SearchInput{Query: query, Limit: 5})
			if err != nil {
				t.Fatalf("search: %v", err)
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
// ranks first for current-user phrasings (surface-eval task MT-114). The
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
			var out SearchOutput
			_, out, err = reg.Search(context.Background(), nil, SearchInput{Query: query, Limit: 5})
			if err != nil {
				t.Fatalf("search: %v", err)
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

// TestIntentBoost_AnalyzeMRChangesSurfacesForLLMReviewQueries verifies
// analyze.mr_changes ranks above mr_review.changes_get when the query carries
// an LLM/analyzer signal alongside an MR context. This was surface-eval task
// MT-093 where mr_review.changes_get outranked the intended action.
func TestIntentBoost_AnalyzeMRChangesSurfacesForLLMReviewQueries(t *testing.T) {
	t.Parallel()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	reg := NewRegistryFromCatalog(catalog)
	queries := []string{
		"LLM-assisted code review analyzer for merge request changes in project my-org/tools/gitlab-mcp-server",
		"analyze merge request 7 code changes using the LLM code review analyzer in project my-org/tools/gitlab-mcp-server",
	}
	for _, query := range queries {
		t.Run(query[:min(len(query), 60)], func(t *testing.T) {
			t.Parallel()
			var out SearchOutput
			_, out, err = reg.Search(context.Background(), nil, SearchInput{Query: query, Limit: 5})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(out.Results) == 0 {
				t.Fatal("no results")
			}
			if out.Results[0].ID != "analyze.mr_changes" {
				logRanking(t, query, out.Results)
				t.Errorf("top result = %q, want analyze.mr_changes", out.Results[0].ID)
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
		{"analyze pipeline failure root cause", "analyze.mr_changes"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			var out SearchOutput
			_, out, err = reg.Search(context.Background(), nil, SearchInput{Query: tc.query, Limit: 5})
			if err != nil {
				t.Fatalf("search: %v", err)
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

	t.Run("scoreAnalyzeMRChangesIntent", func(t *testing.T) {
		t.Parallel()
		e := find("analyze", "mr_changes")
		terms := normalizeSearchTerms("llm analyzer for merge request changes")
		score, reason := scoreAnalyzeMRChangesIntent(e, terms)
		if score == 0 {
			t.Error("expected non-zero score for analyze.mr_changes with llm+merge signal")
		}
		if reason.MatchedValue == "" {
			t.Error("expected non-empty MatchReason.MatchedValue")
		}
	})

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
