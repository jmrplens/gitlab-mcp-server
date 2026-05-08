package dynamic

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRegistryDefensiveBranches covers small validation and fallback branches
// in the dynamic registry dispatcher. These scenarios matter because the catalog
// action surface should return helpful tool errors for malformed calls instead
// of leaking empty or ambiguous execution attempts.
func TestRegistryDefensiveBranches(t *testing.T) {
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

// TestRegistryHelperCoverage validates deterministic helper behavior used by
// search ranking, examples, confirmations, and Markdown formatting. The cases
// target defensive branches that are easy to regress while refactoring the low
// token dynamic action surface.
func TestRegistryHelperCoverage(t *testing.T) {
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
		for _, want := range []string{"repository file", "branch", "webhook", "close"} {
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

// TestScoreSearchAlternative covers every ranking branch in the exact search
// scorer. This keeps the weighting contract explicit while fuzzy fallback stays
// isolated to only zero-result searches.
func TestScoreSearchAlternative(t *testing.T) {
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

// TestNormalizationExampleAndFormattingBranches covers compact helpers that
// shape user-facing dynamic tool output. It verifies deduplication of described
// actions, placeholder selection, confirmation parsing, schema cloning failures,
// and empty-result Markdown messages.
func TestNormalizationExampleAndFormattingBranches(t *testing.T) {
	t.Run("normalize describe ids trims and deduplicates", func(t *testing.T) {
		got := normalizeDescribeIDs(DescribeInput{Action: " Project.Get ", Actions: []string{"project.get", "", "Issue.List"}})
		want := []string{"project.get", "issue.list"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("normalizeDescribeIDs() = %v, want %v", got, want)
		}
	})

	t.Run("placeholder selects dates and generic values", func(t *testing.T) {
		if got := placeholderForParam("due_date"); got != "2026-05-07" {
			t.Fatalf("placeholderForParam(date) = %v, want date", got)
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
			{params: map[string]any{"confirm": " yes "}, want: true},
			{params: map[string]any{"confirm": "no"}, want: false},
			{params: map[string]any{"confirm": 1}, want: false},
		}
		for _, tt := range cases {
			if got := hasExplicitConfirm(tt.params); got != tt.want {
				t.Fatalf("hasExplicitConfirm(%v) = %v, want %v", tt.params, got, tt.want)
			}
		}
	})

	t.Run("clone schema recursively copies maps and slices", func(t *testing.T) {
		if got := cloneSchema(nil); got != nil {
			t.Fatalf("cloneSchema(nil) = %v, want nil", got)
		}
		schema := map[string]any{
			"type":     "object",
			"required": []string{"project_id"},
			"properties": map[string]any{
				"labels": []any{"bug", map[string]any{"name": "critical"}},
			},
		}
		cloned := cloneSchema(schema)
		if cloned == nil || cloned["type"] != "object" {
			t.Fatalf("cloneSchema(valid) = %v, want cloned schema", cloned)
		}
		clonedProperties := cloned["properties"].(map[string]any)
		clonedLabels := clonedProperties["labels"].([]any)
		clonedLabels[0] = "changed"
		clonedRequired := cloned["required"].([]string)
		clonedRequired[0] = "changed"

		originalLabels := schema["properties"].(map[string]any)["labels"].([]any)
		if originalLabels[0] != "bug" {
			t.Fatalf("original labels = %v, want unchanged", originalLabels)
		}
		originalRequired := schema["required"].([]string)
		if originalRequired[0] != "project_id" {
			t.Fatalf("original required = %v, want unchanged", originalRequired)
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

// TestAnnotationsWithTitleCopiesBase verifies that annotation updates do not
// mutate the caller's base annotations. The dynamic tool registration uses this
// when assigning distinct titles to otherwise shared tool metadata.
func TestAnnotationsWithTitleCopiesBase(t *testing.T) {
	base := &mcp.ToolAnnotations{Title: "Original", ReadOnlyHint: true}
	got := annotationsWithTitle(base, "Updated")
	if got == nil || got.Title != "Updated" || !got.ReadOnlyHint {
		t.Fatalf("annotationsWithTitle(base) = %+v, want copied read-only annotation with updated title", got)
	}
	if base.Title != "Original" {
		t.Fatalf("base title = %q, want unchanged Original", base.Title)
	}
}
