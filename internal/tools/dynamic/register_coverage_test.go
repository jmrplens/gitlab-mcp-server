package dynamic

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
