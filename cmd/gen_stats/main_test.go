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

// TestScanGoFile_SkipsRawStringFixtures verifies that a fake test embedded in
// a multi-line raw string does not reach the line-level counters. A fixture
// source holding a whole fake Go file must not inflate subtest, defer or
// error-check totals, which is what the backtick-parity skip is for.
//
// Declaration counting is no longer the line scanner's job — see
// [TestScanGoDecls_CountsDeclarationsNotText] — so TestFuncs stays zero here.
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
	if s.Subtests != 0 {
		t.Errorf("Subtests = %d, want 0 — the t.Run inside the raw string is data, not code", s.Subtests)
	}
	if s.TestFuncs != 0 {
		t.Errorf("TestFuncs = %d, want 0 — the line scanner no longer counts declarations", s.TestFuncs)
	}
}

// TestScanGoDecls_CountsDeclarationsNotText verifies that declarations are
// counted by parsing, which is what makes the fixture case correct without a
// heuristic.
//
// The line scanner skipped raw strings by counting backtick parity, and a
// single line holding an odd number of backticks flipped it into "inside a
// raw string" until the next such line — swallowing 225 real test
// declarations across this repository and putting these totals permanently at
// odds with cmd/gen_testing_docs. A parser has no such failure mode: the fake
// below is a string constant, and the real declaration after it is a
// declaration regardless of how many backticks preceded it.
func TestScanGoDecls_CountsDeclarationsNotText(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		isTest        bool
		isE2E         bool
		wantTestFuncs int
		wantE2EFuncs  int
		wantExported  int
		wantStructs   int
	}{
		{
			name:          "fake test inside a raw string is not a declaration",
			src:           "package p\n\nconst fixture = `\nfunc TestFake(t *testing.T) {}\n`\n\nfunc TestReal(t *testing.T) {}\n",
			isTest:        true,
			wantTestFuncs: 1,
		},
		{
			name: "an odd backtick count does not hide what follows",
			src: "package p\n\nfunc TestBefore(t *testing.T) {}\n\n" +
				"var doc = \"a ` lone backtick in an interpreted string\"\n\n" +
				"func TestAfter(t *testing.T) {}\n",
			isTest:        true,
			wantTestFuncs: 2,
		},
		{
			name:          "TestMain is not a test",
			src:           "package p\n\nfunc TestMain(m *testing.M) {}\n\nfunc TestReal(t *testing.T) {}\n",
			isTest:        true,
			wantTestFuncs: 1,
		},
		{
			name:         "e2e files count separately",
			src:          "package p\n\nfunc TestFlow(t *testing.T) {}\n",
			isE2E:        true,
			wantE2EFuncs: 1,
		},
		{
			name:         "source declarations, structs included",
			src:          "package p\n\ntype Thing struct{}\n\ntype Alias = int\n\nfunc Exported() {}\n\nfunc unexported() {}\n",
			wantExported: 1,
			wantStructs:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			name := "x.go"
			if tt.isTest || tt.isE2E {
				name = "x_test.go"
			}
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(tt.src), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			var s repoStats
			if err := scanGoDecls(path, tt.isE2E, tt.isTest, &s); err != nil {
				t.Fatalf("scanGoDecls: %v", err)
			}
			if s.TestFuncs != tt.wantTestFuncs {
				t.Errorf("TestFuncs = %d, want %d", s.TestFuncs, tt.wantTestFuncs)
			}
			if s.E2ETestFuncs != tt.wantE2EFuncs {
				t.Errorf("E2ETestFuncs = %d, want %d", s.E2ETestFuncs, tt.wantE2EFuncs)
			}
			if s.ExportedFuncs != tt.wantExported {
				t.Errorf("ExportedFuncs = %d, want %d", s.ExportedFuncs, tt.wantExported)
			}
			if s.StructTypes != tt.wantStructs {
				t.Errorf("StructTypes = %d, want %d", s.StructTypes, tt.wantStructs)
			}
		})
	}
}

// TestScanGoDecls_UnparseableFile_Reports verifies that a file the parser
// cannot read surfaces as an error rather than silently contributing zero.
// A generator that under-reports without saying so is what this whole change
// is correcting.
func TestScanGoDecls_UnparseableFile_Reports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.go")
	if err := os.WriteFile(path, []byte("package p\n\nfunc ("), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var s repoStats
	if err := scanGoDecls(path, false, false, &s); err == nil {
		t.Error("expected an error for an unparseable file")
	}
}
