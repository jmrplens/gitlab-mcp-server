// main_test.go verifies the repository statistics collector used by
// cmd/gen_stats.
package main

import "testing"

// TestCollectStats_NonZeroCounts verifies collectStats returns non-zero
// counts when run against the real repository root.
func TestCollectStats_NonZeroCounts(t *testing.T) {
	stats, err := collectStats(".")
	if err != nil {
		t.Fatalf("collectStats: %v", err)
	}
	if stats.SourceFiles == 0 {
		t.Error("SourceFiles = 0, want > 0")
	}
	if stats.SourceLines == 0 {
		t.Error("SourceLines = 0, want > 0")
	}
	if stats.Packages == 0 {
		t.Error("Packages = 0, want > 0")
	}
}

// TestFmtInt_AddsThousandsSeparators verifies the integer formatter inserts
// comma separators at the expected positions.
func TestFmtInt_AddsThousandsSeparators(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{in: 0, want: "0"},
		{in: 999, want: "999"},
		{in: 1000, want: "1,000"},
		{in: 1234567, want: "1,234,567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := fmtInt(tt.in); got != tt.want {
				t.Fatalf("fmtInt(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsTestFunctionName_ExcludesTestMain verifies TestMain is not counted as a
// test function while regular Test* entry points are.
func TestIsTestFunctionName_ExcludesTestMain(t *testing.T) {
	if isTestFunctionName("TestMain") {
		t.Error("isTestFunctionName(TestMain) = true, want false")
	}
	if !isTestFunctionName("TestFoo") {
		t.Error("isTestFunctionName(TestFoo) = false, want true")
	}
	if !isTestFunctionName("Test") {
		t.Error("isTestFunctionName(Test) = false, want true")
	}
	if isTestFunctionName("Testable") {
		t.Error("isTestFunctionName(Testable) = true, want false (lowercase-ish suffix)")
	}
}

// TestComputeReplacedText_ReplacesBetweenMarkers verifies the section
// replacement helper swaps content between markers and preserves them.
func TestComputeReplacedText_ReplacesBetweenMarkers(t *testing.T) {
	text := "<!-- START -->\nold content\n<!-- END -->\ntail"
	got, err := computeReplacedText(text, "<!-- START -->", "<!-- END -->", "NEW")
	if err != nil {
		t.Fatalf("computeReplacedText: %v", err)
	}
	if !contains(got, "<!-- START -->") || !contains(got, "<!-- END -->") {
		t.Fatalf("markers not preserved:\n%s", got)
	}
	if contains(got, "old content") {
		t.Fatalf("old content not replaced:\n%s", got)
	}
	if !contains(got, "tail") {
		t.Fatalf("trailing content lost:\n%s", got)
	}
}

// TestComputeReplacedText_MissingMarkerFailsFast verifies a missing start
// marker returns a descriptive error rather than silently succeeding.
func TestComputeReplacedText_MissingMarkerFailsFast(t *testing.T) {
	if _, err := computeReplacedText("no markers here", "<!-- START -->", "<!-- END -->", "NEW"); err == nil {
		t.Fatal("computeReplacedText missing start marker: error = nil, want descriptive error")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
