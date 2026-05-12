package dynamic

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
	if defaultResult == nil || defaultResult.IsError {
		t.Fatalf("Search(default) result = %+v, want non-error", defaultResult)
	}
	if defaultOutput.Count == 0 {
		t.Fatal("Search(default) returned no matches")
	}
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
	if explainResult == nil || explainResult.IsError {
		t.Fatalf("Search(explain) result = %+v, want non-error", explainResult)
	}
	if explainOutput.Count == 0 {
		t.Fatal("Search(explain) returned no matches")
	}
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
	if !strings.Contains(textContent(explainResult), "| Why |") || !strings.Contains(textContent(explainResult), "matched") {
		t.Fatalf("Search(explain) markdown missing Why explanation: %s", textContent(explainResult))
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

// TestSearchIndex_CandidateGenerationPreservesFullScanTopResults verifies that
// the lightweight index narrows candidate entries without changing the top
// lexical results for the baseline query set.
func TestSearchIndex_CandidateGenerationPreservesFullScanTopResults(t *testing.T) {
	registry := realCatalogRegistry(t)

	queries := []string{
		"merge request list open author project",
		"list open issues",
		"pipeline run trigger",
		"ci variable secret",
		"project delete",
		"discover project from remote",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			terms := normalizeSearchTerms(query)
			indexed := sortAndLimitMatches(registry.scoredMatches(terms, scoreEntryWithExplanation), 5)
			fullScan := sortAndLimitMatches(fullScanScoredMatches(registry.entries, terms, scoreEntryWithExplanation), 5)
			if len(fullScan) == 0 {
				t.Fatalf("full scan returned no lexical matches for %q", query)
			}
			if !slices.Equal(scoredActionIDs(indexed), scoredActionIDs(fullScan)) {
				t.Fatalf("indexed matches = %v, full scan = %v", scoredActionIDs(indexed), scoredActionIDs(fullScan))
			}
		})
	}
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
	standaloneCatalog, err := AddStandaloneCatalog(actionregistry.FromActionMaps(routes), nil, StandaloneOptions{})
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
	catalog := actionregistry.NewCatalog()
	group := actionregistry.NewGroup(actionregistry.GroupOptions{ToolName: "gitlab_custom"})
	group.SetAction(actionregistry.Action{
		Name:    "inspect",
		Aliases: []string{"custom.lookup"},
		Tags:    []string{"bespoke"},
		Route: toolutil.ActionRoute{
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"target": params["target"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"target"},
				"properties": map[string]any{
					"target": map[string]any{"type": "string"},
				},
			},
		},
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	registry := NewRegistryFromCatalog(catalog)
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
	if action.Example.Arguments["confirm"] != true {
		t.Fatalf("example missing confirm param: %+v", action.Example)
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
	if found.Example.Tool != "gitlab_execute_tool" || found.Example.Arguments["confirm"] != true {
		t.Fatalf("example = %+v, want execute example with confirm", found.Example)
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
	RegisterCatalogFindExecuteTools(server, actionregistry.FromActionMaps(testRoutes(t)))

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
	if !slices.Contains(names, "gitlab_find_action") || !slices.Contains(names, "gitlab_execute_tool") {
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
		"issue_note.create":                          "issue.note_create",
		"issue_note.delete":                          "issue.note_delete",
		"issue_note.update":                          "issue.note_update",
		"release_link.link_list":                     "release.link_list",
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
		"admin.settings_get":            "current instance/application settings",
		"job.download_single_artifact":  "one artifact file path",
		"package.list":                  "package registry packages",
		"runner.remove":                 "numeric runner_id",
		"repository.compare":            "params.from and params.to",
		"analyze.release_notes":         "after requested release/compare",
		"package.registry_list_project": "container registry image repositories",
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
	catalog := actionregistry.NewCatalog()
	group := actionregistry.NewGroup(actionregistry.GroupOptions{
		ToolName: "gitlab_custom",
		FormatResult: func(any) *mcp.CallToolResult {
			return toolutil.ToolResultAnnotated("custom formatted result", toolutil.ContentDetail)
		},
	})
	group.SetAction(actionregistry.Action{
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

// TestExecute_NormalizesActionScopedParameterAliases verifies dynamic execute
// accepts ambiguous model aliases only for actions where the schema is clear.
func TestExecute_NormalizesActionScopedParameterAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := []struct {
		name   string
		input  ExecuteInput
		assert func(t *testing.T, output any)
	}{
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
		{
			name:  "issue link aliases same project target",
			input: ExecuteInput{Action: "issue.link_create", Params: map[string]any{"project_id": 123, "issue_iid": 1, "linked_issue_iid": 2}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["target_issue_iid"] != 2 || data["target_project_id"] != 123 {
					t.Fatalf("output = %#v, want target_issue_iid 2 and target_project_id 123", output)
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
		{
			name:  "group label update name alias",
			input: ExecuteInput{Action: "group.group_label_update", Params: map[string]any{"group_id": "my-org", "label_id": 31, "name": "next-label"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["new_name"] != "next-label" {
					t.Fatalf("output = %#v, want new_name next-label", output)
				}
				if _, ok := data["name"]; ok {
					t.Fatalf("output = %#v, want name alias removed", output)
				}
			},
		},
		{
			name:  "feature flag version alias",
			input: ExecuteInput{Action: "feature_flags.feature_flag_create", Params: map[string]any{"project_id": 123, "name": "eval", "new_version_flag": "new_version_flag"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["version"] != "new_version_flag" {
					t.Fatalf("output = %#v, want version new_version_flag", output)
				}
			},
		},
		{
			name:  "feature flag user list drops feature flag name",
			input: ExecuteInput{Action: "feature_flags.ff_user_list_list", Params: map[string]any{"project_id": 123, "name": "eval_flag", "per_page": 20}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if _, ok := data["name"]; ok {
					t.Fatalf("output = %#v, want name removed", output)
				}
				if data["per_page"] != 20 {
					t.Fatalf("output = %#v, want per_page 20", output)
				}
			},
		},
		{
			name:  "release link tag alias",
			input: ExecuteInput{Action: "release.link_create", Params: map[string]any{"project_id": 123, "release_tag_name": "v1.0.0", "name": "asset", "url": "https://example.com/asset"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["tag_name"] != "v1.0.0" {
					t.Fatalf("output = %#v, want tag_name v1.0.0", output)
				}
			},
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
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				files := data["files"].([]any)
				file := files[0].(map[string]any)
				if _, ok := file["action"]; ok {
					t.Fatalf("output = %#v, want files[0].action removed", output)
				}
			},
		},
		{
			name: "snippet create builds files from single file params",
			input: ExecuteInput{Action: "snippet.project_create", Params: map[string]any{
				"project_id": 123,
				"title":      "snippet",
				"file_name":  "snippet.md",
				"content":    "body",
			}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				files := data["files"].([]any)
				file := files[0].(map[string]any)
				if file["file_path"] != "snippet.md" || file["content"] != "body" {
					t.Fatalf("output = %#v, want files[0] with file_path and content", output)
				}
				if _, ok := data["file_name"]; ok {
					t.Fatalf("output = %#v, want top-level file_name removed", output)
				}
				if _, ok := data["content"]; ok {
					t.Fatalf("output = %#v, want top-level content removed", output)
				}
			},
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
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				files := data["files"].([]any)
				file := files[0].(map[string]any)
				if file["file_path"] != "snippet.md" {
					t.Fatalf("output = %#v, want files[0].file_path snippet.md", output)
				}
				if _, ok := file["file_name"]; ok {
					t.Fatalf("output = %#v, want files[0].file_name removed", output)
				}
			},
		},
		{
			name:  "runner paused string to bool",
			input: ExecuteInput{Action: "runner.update", Params: map[string]any{"runner_id": 99, "paused": "true"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["paused"] != true {
					t.Fatalf("output = %#v, want paused true", output)
				}
			},
		},
	}

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
	aliases := actionScopedParamAliases()
	wantActions := []string{
		"job.list",
		"repository.file_get",
		"issue.link_create",
		"issue.update",
		"pipeline.schedule_create",
		"pipeline.schedule_update",
		"branch.protect",
		"feature_flags.feature_flag_create",
		"feature_flags.ff_user_list_list",
		"group.group_label_update",
		"project.member_add",
		"project.member_edit",
		"release.link_create",
		"release.link_delete",
		"release.link_get",
		"release.link_list",
		"release.link_update",
		"runner.update",
		"snippet.project_create",
	}
	for _, actionID := range wantActions {
		if !slices.ContainsFunc(aliases, func(alias actionScopedParamAlias) bool { return alias.ActionID == actionID }) {
			t.Fatalf("actionScopedParamAliases() = %+v, want action %s", aliases, actionID)
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

// TestRegisterCatalogTools_ExposesThreeDynamicTools verifies that the full dynamic
// surface exposes search, describe, and execute through an MCP session.
func TestRegisterCatalogTools_ExposesThreeDynamicTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogTools(server, actionregistry.FromActionMaps(testRoutes(t)))

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
	if len(tools.Tools) != 3 {
		t.Fatalf("tool count = %d, want 3", len(tools.Tools))
	}
	executeSchema := listedToolInputSchema(t, tools.Tools, "gitlab_execute_tool")
	if !slices.Contains(schemaRequired(executeSchema), "params") {
		t.Fatalf("gitlab_execute_tool required = %v, want params", schemaRequired(executeSchema))
	}
	assertSchemaHasProperties(t, executeSchema, "action", "params", "confirm")
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

func realCatalogRegistry(t *testing.T) *Registry {
	t.Helper()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	catalog, err = AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	return NewRegistryFromCatalog(catalog)
}

func assertSearchResultsContain(t *testing.T, results []SearchResult, want ...string) {
	t.Helper()
	for _, actionID := range want {
		if slices.ContainsFunc(results, func(result SearchResult) bool { return result.ID == actionID }) {
			continue
		}
		t.Fatalf("Search() results = %+v, want %s", results, actionID)
	}
}

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

func scoredActionIDs(matches []scoredActionEntry) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.entry.ID)
	}
	return ids
}

func assertSchemaHasProperties(t *testing.T, schema map[string]any, names ...string) {
	t.Helper()
	properties := schemaProperties(schema)
	for _, name := range names {
		if _, ok := properties[name]; !ok {
			t.Fatalf("schema properties = %v, want %q", sortedPropertyNames(properties), name)
		}
	}
}

func schemaProperties(schema map[string]any) map[string]any {
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		return map[string]any{}
	}
	return properties
}

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

func sortedPropertyNames(properties map[string]any) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func testRoutes(t *testing.T) map[string]toolutil.ActionMap {
	t.Helper()
	return map[string]toolutil.ActionMap{
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
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"spent": "added"}, nil
				},
			},
			"emoji_mr_create": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"name": params["name"]}, nil
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
			"link_create": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"target_issue_iid": params["target_issue_iid"], "target_project_id": params["target_project_id"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "issue_iid", "target_project_id", "target_issue_iid"},
					"properties": map[string]any{
						"project_id":        map[string]any{"type": "integer"},
						"issue_iid":         map[string]any{"type": "integer"},
						"target_project_id": map[string]any{"type": "integer"},
						"target_issue_iid":  map[string]any{"type": "integer"},
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
}

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
