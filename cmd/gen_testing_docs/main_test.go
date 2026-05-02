package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParsePackageCoverages_ExtractsGoTestCoverage verifies the parser accepts
// normal and cached go test coverage lines while ignoring packages without
// coverage data.
func TestParsePackageCoverages_ExtractsGoTestCoverage(t *testing.T) {
	output := strings.Join([]string{
		"ok  github.com/jmrplens/gitlab-mcp-server/internal/toolutil 0.018s coverage: 96.6% of statements",
		"ok  github.com/jmrplens/gitlab-mcp-server/cmd/server (cached) coverage: 62.5% of statements",
		"?   github.com/jmrplens/gitlab-mcp-server/cmd/add_docs [no test files]",
	}, "\n")

	coverages, err := parsePackageCoverages(output)
	if err != nil {
		t.Fatalf("parsePackageCoverages() error = %v", err)
	}
	if got := coverages["github.com/jmrplens/gitlab-mcp-server/internal/toolutil"].Percent; got != 96.6 {
		t.Fatalf("toolutil coverage = %.1f, want 96.6", got)
	}
	if got := coverages["github.com/jmrplens/gitlab-mcp-server/cmd/server"].Percent; got != 62.5 {
		t.Fatalf("cmd/server coverage = %.1f, want 62.5", got)
	}
	if _, ok := coverages["github.com/jmrplens/gitlab-mcp-server/cmd/add_docs"]; ok {
		t.Fatal("package without coverage should be ignored")
	}
}

// TestParseTotalCoverage_ExtractsTotal verifies go tool cover -func total line parsing.
func TestParseTotalCoverage_ExtractsTotal(t *testing.T) {
	output := "github.com/example/project/file.go:10:\tRun\t80.0%\n" +
		"total:\t\t\t\t(statements)\t93.7%\n"

	got, err := parseTotalCoverage(output)
	if err != nil {
		t.Fatalf("parseTotalCoverage() error = %v", err)
	}
	if got != 93.7 {
		t.Fatalf("parseTotalCoverage() = %.1f, want 93.7", got)
	}
}

// TestCountTests_ClassifiesTestNames verifies AST-based test counting and
// naming pattern classification for Go test entry points.
func TestCountTests_ClassifiesTestNames(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "sample_test.go", `package sample

import "testing"

func TestWidget_Success(t *testing.T) {}
func TestWidget_Create_Success(t *testing.T) {}
func TestWidget(t *testing.T) {}
func TestCovWidgetBranch(t *testing.T) {}
func Testhelper(t *testing.T) {}
func TestMain(m *testing.M) {}
`)

	tests, files, counts, err := countTests(dir)
	if err != nil {
		t.Fatalf("countTests() error = %v", err)
	}
	if tests != 4 {
		t.Fatalf("test function count = %d, want 4", tests)
	}
	if files != 1 {
		t.Fatalf("test file count = %d, want 1", files)
	}
	expected := map[string]int{
		pattern2Part:        1,
		pattern3Part:        1,
		patternNoUnderscore: 1,
		patternTestCov:      1,
	}
	for pattern, want := range expected {
		if got := counts[pattern]; got != want {
			t.Fatalf("pattern %s count = %d, want %d", pattern, got, want)
		}
	}
}

// TestCountMCPTools_CountsAddToolCalls verifies tool-count extraction from
// mcp.AddTool calls without executing registration code.
func TestCountMCPTools_CountsAddToolCalls(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "register.go", `package sample

func RegisterTools() {
	mcp.AddTool(server, toolA, handlerA)
	mcp.AddTool(server, toolB, handlerB)
	toolutil.AddMetaTool(server, toolC, routes, icons, handlerC)
}
`)

	got, err := countMCPTools(dir)
	if err != nil {
		t.Fatalf("countMCPTools() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("countMCPTools() = %d, want 2", got)
	}
}

// TestReplaceGeneratedBlock_MigratesLegacySection verifies first-run migration
// from the historic unmarked Overview/Coverage block to managed markers.
func TestReplaceGeneratedBlock_MigratesLegacySection(t *testing.T) {
	legacy := "# Testing\n\n## Overview\n\nold metrics\n\n## Test Types\n\nmanual content\n"

	updated, err := replaceGeneratedBlock(legacy, "## Overview\n\nnew metrics\n")
	if err != nil {
		t.Fatalf("replaceGeneratedBlock() error = %v", err)
	}
	for _, want := range []string{startMarker, "new metrics", endMarker, "## Test Types", "manual content"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated content missing %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "old metrics") {
		t.Fatalf("legacy metrics should have been replaced:\n%s", updated)
	}
}

// TestReplaceGeneratedBlock_ReplacesMarkedSection verifies subsequent runs only
// rewrite the managed marker block.
func TestReplaceGeneratedBlock_ReplacesMarkedSection(t *testing.T) {
	marked := "# Testing\n\n" + startMarker + "\n\nold\n" + endMarker + "\n\n## Test Types\n\nmanual\n"

	updated, err := replaceGeneratedBlock(marked, "new\n")
	if err != nil {
		t.Fatalf("replaceGeneratedBlock() error = %v", err)
	}
	if !strings.Contains(updated, startMarker+"\n\nnew\n\n"+endMarker) {
		t.Fatalf("marked section not replaced as expected:\n%s", updated)
	}
	if !strings.Contains(updated, "manual") {
		t.Fatalf("manual content should be preserved:\n%s", updated)
	}
}

// TestRelativePath_UsesRepositoryRoot verifies absolute go list paths are
// converted to module-relative paths before package layer classification.
func TestRelativePath_UsesRepositoryRoot(t *testing.T) {
	root := repositoryRoot()
	got := relativePath(filepath.Join(root, "internal", "toolutil"))
	if got != "internal/toolutil" {
		t.Fatalf("relativePath() = %q, want internal/toolutil", got)
	}
	if layer := classifyLayer(got); layer != "core" {
		t.Fatalf("classifyLayer(%q) = %q, want core", got, layer)
	}
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
