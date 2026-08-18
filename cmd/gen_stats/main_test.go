// main_test.go verifies the repository statistics collector used by
// cmd/gen_stats.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

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

// TestScanGoFile_SkipsRawStringFixtures verifies that fake test functions
// embedded in multi-line raw strings (fixture sources) are not counted.
func TestScanGoFile_SkipsRawStringFixtures(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\nconst fixture = `\nfunc TestFake(t *testing.T) {\n\tt.Run(\"sub\", nil)\n}\n`\n\nfunc TestReal(t *testing.T) {}\n"
	path := filepath.Join(dir, "x_test.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var s repoStats
	if _, err := scanGoFile(path, false, true, &s); err != nil {
		t.Fatalf("scanGoFile: %v", err)
	}
	if s.TestFuncs != 1 {
		t.Errorf("TestFuncs = %d, want 1 (fixture-embedded fake must not count)", s.TestFuncs)
	}
	if s.Subtests != 0 {
		t.Errorf("Subtests = %d, want 0", s.Subtests)
	}
}
