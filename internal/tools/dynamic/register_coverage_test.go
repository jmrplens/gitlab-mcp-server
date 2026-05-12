package dynamic

import (
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestRegistry_DefensiveBranches covers small validation and fallback branches
// in the dynamic registry dispatcher. These scenarios matter because the catalog
// action surface should return helpful tool errors for malformed calls instead
// of leaking empty or ambiguous execution attempts.
func TestRegistry_DefensiveBranches(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	t.Run("describe requires action", func(t *testing.T) {
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
	})

	t.Run("execute requires action", func(t *testing.T) {
		result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("Execute() result = %+v, want tool error", result)
		}
		if output != nil {
			t.Fatalf("Execute() output = %+v, want nil", output)
		}
	})

	t.Run("execute unknown action without suggestions", func(t *testing.T) {
		result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "zzzz"})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("Execute() result = %+v, want tool error", result)
		}
		if output != nil {
			t.Fatalf("Execute() output = %+v, want nil", output)
		}
		if strings.Contains(textContent(result), "Did you mean") {
			t.Fatalf("Execute() error text = %q, want no suggestions", textContent(result))
		}
	})

	t.Run("execute initializes nil params", func(t *testing.T) {
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
	})
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
		if got := registry.segmentedSearchMatches(normalizeSearchTerms("project get")); got != nil {
			t.Fatalf("segmentedSearchMatches(short query) = %v, want nil", got)
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
}

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

func TestSearchIndex_DefensiveBranches(t *testing.T) {
	var emptyIndex searchIndex
	if got := emptyIndex.candidateEntryIndexes(nil); got != nil {
		t.Fatalf("candidateEntryIndexes(empty index) = %v, want nil", got)
	}

	index := searchIndex{
		byToken: map[string][]int{},
		all:     []int{0, 1},
	}
	if got := index.candidateEntryIndexes(nil); strings.Join(intsToStrings(got), ",") != "0,1" {
		t.Fatalf("candidateEntryIndexes(empty terms) = %v, want all indexes", got)
	}
	index.addValues(index.byToken, []string{"", "project.delete", "project.delete"}, 0)
	if got := index.byToken["project.delete"]; len(got) != 1 || got[0] != 0 {
		t.Fatalf("byToken[project.delete] = %v, want single posting 0", got)
	}
	if got := index.candidateEntryIndexes(normalizeSearchTerms("unknown")); strings.Join(intsToStrings(got), ",") != "0,1" {
		t.Fatalf("candidateEntryIndexes(no candidates) = %v, want full fallback", got)
	}
}

func intsToStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}

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

func TestActionScopedParamValueConversions(t *testing.T) {
	stateCases := map[any]string{"closed": "close", "OPEN": "reopen"}
	for input, want := range stateCases {
		got, ok := issueStateEventValue(input)
		if !ok || got != want {
			t.Fatalf("issueStateEventValue(%v) = %q, %t; want %q, true", input, got, ok, want)
		}
	}
	if _, ok := issueStateEventValue(123); ok {
		t.Fatal("issueStateEventValue(non-string) converted unexpectedly")
	}
	if _, ok := issueStateEventValue("archived"); ok {
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
		got, ok := gitlabAccessLevelValue(input)
		if !ok || got != want {
			t.Fatalf("gitlabAccessLevelValue(%v) = %d, %t; want %d, true", input, got, ok, want)
		}
	}
	for _, input := range []any{float64(30.5), 70, int64(70), "70", "admin", true} {
		if got, ok := gitlabAccessLevelValue(input); ok {
			t.Fatalf("gitlabAccessLevelValue(%v) = %d, true; want false", input, got)
		}
	}

	if value, ok := boolStringValue(" true "); !ok || !value {
		t.Fatalf("boolStringValue(true) = %t, %t; want true, true", value, ok)
	}
	for _, input := range []any{true, "not-bool"} {
		if _, ok := boolStringValue(input); ok {
			t.Fatalf("boolStringValue(%v) converted unexpectedly", input)
		}
	}
}

func TestSnippetParamNormalization_DefensiveBranches(t *testing.T) {
	cloneCalls := 0
	params := map[string]any{"content": "body"}
	clone := func() map[string]any {
		cloneCalls++
		return params
	}
	if buildSnippetCreateFilesFromSingleFileParams(clone, params) {
		t.Fatal("buildSnippetCreateFilesFromSingleFileParams() converted without file_name")
	}
	if cloneCalls != 0 {
		t.Fatalf("cloneCalls = %d, want 0", cloneCalls)
	}

	files := map[string]any{"files": []any{"not-a-map", map[string]any{"file_name": "a.go"}}}
	if !normalizeSnippetFileNameFields(cloneMap(files), files) {
		t.Fatal("normalizeSnippetFileNameFields() = false, want true for map entry")
	}
	if got := files["files"].([]any)[0]; got != "not-a-map" {
		t.Fatalf("first file entry = %#v, want original non-map", got)
	}

	actions := map[string]any{"files": []any{"not-a-map", map[string]any{"action": "create", "file_path": "a.go"}}}
	if !stripSnippetCreateFileActions(cloneMap(actions), actions) {
		t.Fatal("stripSnippetCreateFileActions() = false, want true for create action")
	}
	if got := actions["files"].([]any)[0]; got != "not-a-map" {
		t.Fatalf("first action entry = %#v, want original non-map", got)
	}
}

func TestCompatibilityAliasAndDescriptionBranches(t *testing.T) {
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
	if got := describeEntry(registry.entries[0]); got.InputSchema["type"] == "" || got.Example.Tool != executeToolName {
		t.Fatalf("describeEntry(success) = %+v, want schema and dynamic execute example", got)
	}
	if got := compactSchemaJSON(nil); got != "" {
		t.Fatalf("compactSchemaJSON(nil) = %q, want empty", got)
	}
	if got := compactSchemaJSON(map[string]any{"bad": make(chan int)}); got != "" {
		t.Fatalf("compactSchemaJSON(unmarshalable) = %q, want empty", got)
	}
}

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

type testEnumStringer string

func (value testEnumStringer) String() string { return string(value) }

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

	if score := scoreSearchAlternative(actionEntry{Document: searchDocument{CanonicalID: "project.get", SchemaProperties: []string{"visibility_level"}}}, "visibility", "visibility_level"); score == 0 {
		t.Fatal("scoreSearchAlternative(schema property) = 0, want match")
	}
	if score := scoreFieldContainsFor("field", "field_extra"); score != scoreSynonymContains {
		t.Fatalf("scoreFieldContainsFor(synonym) = %d, want %d", score, scoreSynonymContains)
	}
	if score, explanation := scoreEntryWithExplanation(actionEntry{}, nil); score != 0 || len(explanation.Reasons) != 0 {
		t.Fatalf("scoreEntryWithExplanation(empty) = %d, %+v; want zero result", score, explanation)
	}
}

func TestRequiredParamAndPlaceholderBranches(t *testing.T) {
	schema := map[string]any{"anyOf": []any{"invalid", map[string]any{"required": []any{"project_id"}}}}
	if got := appendPreferredAlternativeRequiredParams(nil, schema); len(got) != 1 || got[0] != "project_id" {
		t.Fatalf("appendPreferredAlternativeRequiredParams() = %v, want project_id", got)
	}
	if got := placeholderForParam("group_id"); got != "group/subgroup" {
		t.Fatalf("placeholderForParam(group_id) = %v, want group/subgroup", got)
	}
}

func cloneMap(target map[string]any) func() map[string]any {
	return func() map[string]any { return target }
}

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
