// utils_test.go contains unit tests for completion utility functions:
// [toResult], [emptyResult], format* helpers, [truncate], [filterByPrefix],
// and [resolvedArguments].

package completions

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Shared assertion format for unexpected values in format helper tests.
const fmtUnexpected = "unexpected: %s"

// TestToResult_LimitEnforced verifies that [toResult] caps values at
// [maxCompletionResults] and sets HasMore when input exceeds the limit.
func TestToResult_LimitEnforced(t *testing.T) {
	values := make([]string, 15)
	for i := range values {
		values[i] = "item"
	}

	result := toResult(values)
	if len(result.Completion.Values) != maxCompletionResults {
		t.Errorf("expected %d values, got %d", maxCompletionResults, len(result.Completion.Values))
	}
	if !result.Completion.HasMore {
		t.Error("expected HasMore=true when results exceed max")
	}
	if result.Completion.Total != 15 {
		t.Errorf("expected Total=15, got %d", result.Completion.Total)
	}
}

// TestToResult_UnderLimit verifies that [toResult] returns all values and
// HasMore=false when input is within the limit.
func TestToResult_UnderLimit(t *testing.T) {
	values := []string{"a", "b", "c"}
	result := toResult(values)
	if len(result.Completion.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(result.Completion.Values))
	}
	if result.Completion.HasMore {
		t.Error("expected HasMore=false when under limit")
	}
	if result.Completion.Total != 3 {
		t.Errorf("expected Total=3, got %d", result.Completion.Total)
	}
}

// TestToResult_Empty verifies that [toResult] handles an empty input slice.
func TestToResult_Empty(t *testing.T) {
	result := toResult([]string{})
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected 0 values, got %d", len(result.Completion.Values))
	}
	if result.Completion.HasMore {
		t.Error("expected HasMore=false for empty")
	}
}

// TestEmptyResult verifies that [emptyResult] returns a non-nil result with
// no completion values.
func TestEmptyResult(t *testing.T) {
	result := emptyResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected 0 values in empty result, got %d", len(result.Completion.Values))
	}
}

// TestFormatProjectEntry verifies the "id: path" formatting of project entries.
func TestFormatProjectEntry(t *testing.T) {
	got := formatProjectEntry(42, "team/backend")
	if got != "42: team/backend" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatGroupEntry verifies the "id: full_path" formatting of group entries.
func TestFormatGroupEntry(t *testing.T) {
	got := formatGroupEntry(10, "engineering/platform")
	if got != "10: engineering/platform" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatMREntry verifies the "iid: title" formatting of merge request entries.
func TestFormatMREntry(t *testing.T) {
	got := formatMREntry(5, "Fix login issue")
	if got != "5: Fix login issue" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatMREntry_Truncation verifies that [formatMREntry] truncates long
// titles to the 60-character display limit.
func TestFormatMREntry_Truncation(t *testing.T) {
	longTitle := "This is a very long merge request title that exceeds the sixty character limit for display"
	got := formatMREntry(1, longTitle)
	if len(got) > 70 {
		t.Errorf("expected truncated title, got length %d: %s", len(got), got)
	}
}

// TestFormatIssueEntry verifies the "iid: title" formatting of issue entries.
func TestFormatIssueEntry(t *testing.T) {
	got := formatIssueEntry(10, "Bug report")
	if got != "10: Bug report" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestTruncate uses table-driven subtests to verify that [truncate] preserves
// short strings and truncates long ones with an ellipsis.
func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestFilterByPrefix verifies that [filterByPrefix] performs case-insensitive
// substring matching and returns all values when the query is empty.
func TestFilterByPrefix(t *testing.T) {
	values := []string{"alpha", "beta", "GAMMA", "delta-alpha"}

	t.Run("match", func(t *testing.T) {
		got := filterByPrefix(values, "alpha")
		if len(got) != 2 {
			t.Fatalf("expected 2 matches, got %d: %v", len(got), got)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		got := filterByPrefix(values, "GAMMA")
		if len(got) != 1 {
			t.Fatalf("expected 1 match, got %d: %v", len(got), got)
		}
	})

	t.Run("empty query returns all", func(t *testing.T) {
		got := filterByPrefix(values, "")
		if len(got) != len(values) {
			t.Errorf("expected all values for empty query, got %d", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := filterByPrefix(values, "zeta")
		if len(got) != 0 {
			t.Errorf("expected 0 matches, got %d", len(got))
		}
	})
}

// TestResolvedArguments_Nil uses table-driven subtests to verify that
// [resolvedArguments] returns an empty map for nil context and nil arguments.
func TestResolvedArguments_Nil(t *testing.T) {
	tests := []struct {
		name string
		ctx  *mcp.CompleteContext
	}{
		{"nil context", nil},
		{"nil arguments", &mcp.CompleteContext{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &mcp.CompleteRequest{}
			req.Params = &mcp.CompleteParams{
				Context: tt.ctx,
			}
			got := resolvedArguments(req)
			if got == nil {
				t.Error("expected non-nil map")
			}
			if len(got) != 0 {
				t.Errorf("expected empty map, got %d entries", len(got))
			}
		})
	}
}

// TestFormatPipelineEntry verifies the "id: ref (status)" formatting.
func TestFormatPipelineEntry(t *testing.T) {
	got := formatPipelineEntry(100, "main", "success")
	if got != "100: main (success)" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatCommitEntry verifies the "short_id: title" formatting.
func TestFormatCommitEntry(t *testing.T) {
	got := formatCommitEntry("abc123d", "Fix login bug")
	if got != "abc123d: Fix login bug" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatCommitEntry_Truncation verifies long title truncation.
func TestFormatCommitEntry_Truncation(t *testing.T) {
	longTitle := "This is a very long commit title that exceeds the sixty character limit for display purposes"
	got := formatCommitEntry("abc123d", longTitle)
	if len(got) > 75 {
		t.Errorf("expected truncated entry, got length %d: %s", len(got), got)
	}
}

// TestFormatMilestoneEntry verifies the "id: title" formatting.
func TestFormatMilestoneEntry(t *testing.T) {
	got := formatMilestoneEntry(5, "Sprint 1")
	if got != "5: Sprint 1" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatJobEntry verifies the "id: name (status)" formatting.
func TestFormatJobEntry(t *testing.T) {
	got := formatJobEntry(501, "build", "success")
	if got != "501: build (success)" {
		t.Errorf(fmtUnexpected, got)
	}
}
