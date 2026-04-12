package toolutil

import (
	"strings"
	"testing"
)

// TestFormatGID verifies GID construction for various types.
func TestFormatGID(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		id       int64
		want     string
	}{
		{"vulnerability", "Vulnerability", 42, "gid://gitlab/Vulnerability/42"},
		{"project", "Project", 1, "gid://gitlab/Project/1"},
		{"work item", "WorkItem", 999, "gid://gitlab/WorkItem/999"},
		{"nested type", "WorkItems::Type", 5, "gid://gitlab/WorkItems::Type/5"},
		{"zero id", "Issue", 0, "gid://gitlab/Issue/0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGID(tt.typeName, tt.id)
			if got != tt.want {
				t.Errorf("FormatGID(%q, %d) = %q, want %q", tt.typeName, tt.id, got, tt.want)
			}
		})
	}
}

// TestParseGID_Valid verifies parsing of well-formed GIDs.
func TestParseGID_Valid(t *testing.T) {
	tests := []struct {
		name     string
		gid      string
		wantType string
		wantID   int64
	}{
		{"simple", "gid://gitlab/Vulnerability/42", "Vulnerability", 42},
		{"large id", "gid://gitlab/Project/123456", "Project", 123456},
		{"nested type", "gid://gitlab/WorkItems::Type/5", "WorkItems::Type", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeName, id, err := ParseGID(tt.gid)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if typeName != tt.wantType {
				t.Errorf("typeName = %q, want %q", typeName, tt.wantType)
			}
			if id != tt.wantID {
				t.Errorf("id = %d, want %d", id, tt.wantID)
			}
		})
	}
}

// TestParseGID_Invalid verifies that malformed GIDs produce errors.
func TestParseGID_Invalid(t *testing.T) {
	tests := []struct {
		name string
		gid  string
	}{
		{"empty", ""},
		{"no prefix", "Vulnerability/42"},
		{"wrong prefix", "gid://github/Issue/1"},
		{"missing id", "gid://gitlab/Vulnerability/"},
		{"missing type", "gid://gitlab//42"},
		{"no slash separator", "gid://gitlab/Vulnerability"},
		{"non-numeric id", "gid://gitlab/Vulnerability/abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseGID(tt.gid)
			if err == nil {
				t.Fatalf("expected error for GID %q, got nil", tt.gid)
			}
		})
	}
}

// TestParseGID_Roundtrip verifies FormatGID → ParseGID roundtrip.
func TestParseGID_Roundtrip(t *testing.T) {
	typeName := "Vulnerability"
	id := int64(42)
	gid := FormatGID(typeName, id)
	gotType, gotID, err := ParseGID(gid)
	if err != nil {
		t.Fatalf("roundtrip error: %v", err)
	}
	if gotType != typeName {
		t.Errorf("roundtrip typeName = %q, want %q", gotType, typeName)
	}
	if gotID != id {
		t.Errorf("roundtrip id = %d, want %d", gotID, id)
	}
}

// TestGraphQLPaginationInput_EffectiveFirst verifies page size defaults and clamping.
func TestGraphQLPaginationInput_EffectiveFirst(t *testing.T) {
	intPtr := func(n int) *int { return &n }
	tests := []struct {
		name  string
		input GraphQLPaginationInput
		want  int
	}{
		{"nil defaults to 20", GraphQLPaginationInput{}, GraphQLDefaultFirst},
		{"explicit 50", GraphQLPaginationInput{First: intPtr(50)}, 50},
		{"clamped to max", GraphQLPaginationInput{First: intPtr(200)}, GraphQLMaxFirst},
		{"clamped to min", GraphQLPaginationInput{First: intPtr(0)}, 1},
		{"negative clamped", GraphQLPaginationInput{First: intPtr(-5)}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.EffectiveFirst()
			if got != tt.want {
				t.Errorf("EffectiveFirst() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestGraphQLPaginationInput_Variables verifies variable map construction.
func TestGraphQLPaginationInput_Variables(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	t.Run("defaults only", func(t *testing.T) {
		v := GraphQLPaginationInput{}.Variables()
		if v["first"] != GraphQLDefaultFirst {
			t.Errorf("first = %v, want %d", v["first"], GraphQLDefaultFirst)
		}
		if _, ok := v["after"]; ok {
			t.Error("after should not be present")
		}
	})

	t.Run("with cursor", func(t *testing.T) {
		v := GraphQLPaginationInput{First: intPtr(10), After: "cursor123"}.Variables()
		if v["first"] != 10 {
			t.Errorf("first = %v, want 10", v["first"])
		}
		if v["after"] != "cursor123" {
			t.Errorf("after = %v, want cursor123", v["after"])
		}
	})

	t.Run("backward pagination", func(t *testing.T) {
		v := GraphQLPaginationInput{Last: intPtr(5), Before: "cursorABC"}.Variables()
		if v["last"] != 5 {
			t.Errorf("last = %v, want 5", v["last"])
		}
		if v["before"] != "cursorABC" {
			t.Errorf("before = %v, want cursorABC", v["before"])
		}
	})
}

// TestPageInfoToOutput verifies raw-to-output conversion.
func TestPageInfoToOutput(t *testing.T) {
	raw := GraphQLRawPageInfo{
		HasNextPage:     true,
		HasPreviousPage: false,
		EndCursor:       "abc123",
		StartCursor:     "xyz789",
	}
	out := PageInfoToOutput(raw)
	if !out.HasNextPage {
		t.Error("HasNextPage should be true")
	}
	if out.HasPreviousPage {
		t.Error("HasPreviousPage should be false")
	}
	if out.EndCursor != "abc123" {
		t.Errorf("EndCursor = %q, want %q", out.EndCursor, "abc123")
	}
	if out.StartCursor != "xyz789" {
		t.Errorf("StartCursor = %q, want %q", out.StartCursor, "xyz789")
	}
}

// TestFormatGraphQLPagination verifies Markdown pagination summary rendering.
func TestFormatGraphQLPagination(t *testing.T) {
	tests := []struct {
		name    string
		p       GraphQLPaginationOutput
		shown   int
		wantSub string
	}{
		{
			"has next page",
			GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cur1"},
			10,
			"next page cursor: `cur1`",
		},
		{
			"has previous page",
			GraphQLPaginationOutput{HasPreviousPage: true, StartCursor: "cur0"},
			5,
			"prev page cursor: `cur0`",
		},
		{
			"no more pages",
			GraphQLPaginationOutput{},
			3,
			"no more pages",
		},
		{
			"both directions",
			GraphQLPaginationOutput{HasNextPage: true, HasPreviousPage: true, EndCursor: "e", StartCursor: "s"},
			20,
			"next page cursor: `e`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGraphQLPagination(tt.p, tt.shown)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("FormatGraphQLPagination() = %q, want substring %q", got, tt.wantSub)
			}
		})
	}
}

// TestMergeVariables verifies map merging with override behavior.
func TestMergeVariables(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := MergeVariables()
		if len(result) != 0 {
			t.Errorf("expected empty map, got %v", result)
		}
	})

	t.Run("single map", func(t *testing.T) {
		result := MergeVariables(map[string]any{"a": 1})
		if result["a"] != 1 {
			t.Errorf("a = %v, want 1", result["a"])
		}
	})

	t.Run("override", func(t *testing.T) {
		result := MergeVariables(
			map[string]any{"a": 1, "b": 2},
			map[string]any{"b": 3, "c": 4},
		)
		if result["a"] != 1 {
			t.Errorf("a = %v, want 1", result["a"])
		}
		if result["b"] != 3 {
			t.Errorf("b = %v, want 3 (override)", result["b"])
		}
		if result["c"] != 4 {
			t.Errorf("c = %v, want 4", result["c"])
		}
	})
}
