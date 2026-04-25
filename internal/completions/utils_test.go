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
// Total is omitted (zero) because [toResult] does not know the upstream count.
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
	if result.Completion.Total != 0 {
		t.Errorf("expected Total=0 (unknown) when not provided, got %d", result.Completion.Total)
	}
}

// TestToResult_UnderLimit verifies that [toResult] returns all values and
// HasMore=false when input is within the limit. Total is omitted because
// [toResult] does not know the upstream count.
func TestToResult_UnderLimit(t *testing.T) {
	values := []string{"a", "b", "c"}
	result := toResult(values)
	if len(result.Completion.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(result.Completion.Values))
	}
	if result.Completion.HasMore {
		t.Error("expected HasMore=false when under limit")
	}
	if result.Completion.Total != 0 {
		t.Errorf("expected Total=0 (unknown), got %d", result.Completion.Total)
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

// TestFormatProjectEntry verifies that project entries are the canonical
// path-with-namespace (the value GitLab API accepts as project_id).
func TestFormatProjectEntry(t *testing.T) {
	got := formatProjectEntry(42, "team/backend")
	if got != "team/backend" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatGroupEntry verifies that group entries are the canonical full path
// (the value GitLab API accepts as group_id).
func TestFormatGroupEntry(t *testing.T) {
	got := formatGroupEntry(10, "engineering/platform")
	if got != "engineering/platform" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatMREntry verifies that merge request entries are the bare IID.
func TestFormatMREntry(t *testing.T) {
	got := formatMREntry(5, "Fix login issue")
	if got != "5" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatMREntry_LongTitleIgnored verifies that [formatMREntry] returns
// only the IID even when the title is long (titles are not part of the value).
func TestFormatMREntry_LongTitleIgnored(t *testing.T) {
	longTitle := "This is a very long merge request title that exceeds the sixty character limit for display"
	got := formatMREntry(1, longTitle)
	if got != "1" {
		t.Errorf("expected bare IID '1', got %q", got)
	}
}

// TestFormatIssueEntry verifies that issue entries are the bare IID.
func TestFormatIssueEntry(t *testing.T) {
	got := formatIssueEntry(10, "Bug report")
	if got != "10" {
		t.Errorf(fmtUnexpected, got)
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

// TestFormatPipelineEntry verifies that pipeline entries are the bare ID.
func TestFormatPipelineEntry(t *testing.T) {
	got := formatPipelineEntry(100, "main", "success")
	if got != "100" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatCommitEntry verifies that commit entries are the bare short SHA.
func TestFormatCommitEntry(t *testing.T) {
	got := formatCommitEntry("abc123d", "Fix login bug")
	if got != "abc123d" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatCommitEntry_LongTitleIgnored verifies that long titles do not
// alter the bare-SHA value.
func TestFormatCommitEntry_LongTitleIgnored(t *testing.T) {
	longTitle := "This is a very long commit title that exceeds the sixty character limit for display purposes"
	got := formatCommitEntry("abc123d", longTitle)
	if got != "abc123d" {
		t.Errorf("expected bare SHA 'abc123d', got %q", got)
	}
}

// TestFormatMilestoneEntry verifies that milestone entries are the bare ID.
func TestFormatMilestoneEntry(t *testing.T) {
	got := formatMilestoneEntry(5, "Sprint 1")
	if got != "5" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatJobEntry verifies that job entries are the bare ID.
func TestFormatJobEntry(t *testing.T) {
	got := formatJobEntry(501, "build", "success")
	if got != "501" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestToResultWithTotal verifies that [toResultWithTotal] propagates the
// upstream total and infers HasMore when total exceeds the cap. Spec
// 2025-11-25: total may exceed values length.
func TestToResultWithTotal(t *testing.T) {
	tests := []struct {
		name        string
		values      []string
		total       int
		wantValues  int
		wantHasMore bool
		wantTotal   int
	}{
		{"unknown total omitted", []string{"a"}, 0, 1, false, 0},
		{"negative total omitted", []string{"a"}, -5, 1, false, 0},
		{"total equal len", []string{"a", "b"}, 2, 2, false, 2},
		{"total exceeds values", []string{"a", "b"}, 50, 2, true, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := toResultWithTotal(tt.values, tt.total)
			if len(r.Completion.Values) != tt.wantValues {
				t.Errorf("values=%d, want %d", len(r.Completion.Values), tt.wantValues)
			}
			if r.Completion.HasMore != tt.wantHasMore {
				t.Errorf("hasMore=%v, want %v", r.Completion.HasMore, tt.wantHasMore)
			}
			if r.Completion.Total != tt.wantTotal {
				t.Errorf("total=%d, want %d", r.Completion.Total, tt.wantTotal)
			}
		})
	}
}

// TestFormatEntries_BareValuesSpec is a guard test asserting that completion
// entry helpers return bare canonical identifiers, never human-readable
// "id: title" labels. MCP spec 2025-11-25 requires `values` in completion
// results to be argument values (the literal that replaces the partial input),
// not labels. Regressing this would silently break chained completions because
// resolved arguments would arrive as e.g. "1234: group/p" instead of
// "group/p", and downstream GitLab API calls would reject the malformed ID.
func TestFormatEntries_BareValuesSpec(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"project", formatProjectEntry(99, "group/proj"), "group/proj"},
		{"group", formatGroupEntry(99, "g/sub"), "g/sub"},
		{"mr", formatMREntry(7, "title"), "7"},
		{"issue", formatIssueEntry(8, "title"), "8"},
		{"pipeline", formatPipelineEntry(9, "main", "ok"), "9"},
		{"commit", formatCommitEntry("abc1234", "title"), "abc1234"},
		{"milestone", formatMilestoneEntry(11, "v1"), "11"},
		{"job", formatJobEntry(12, "build", "ok"), "12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s entry = %q, want bare value %q (spec 2025-11-25 §completion/complete)", c.name, c.got, c.want)
			}
		})
	}
}
