package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_FormatsDefaultMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "| A | B |\n| --- | ---: |\n| one | 2 |\n")
	writeTestFile(t, filepath.Join(root, "docs", "guide.md"), "| Name | Value |\n| --- | --- |\n| longer | x |\n")
	writeTestFile(t, filepath.Join(root, "docs", "ignored.txt"), "| A | B |\n| --- | --- |\n")

	var stdout bytes.Buffer
	if err := run([]string{"--root", root}, &stdout); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	readme := readTestFile(t, filepath.Join(root, "README.md"))
	if !strings.Contains(readme, "| one |    2 |") {
		t.Fatalf("README.md was not formatted:\n%s", readme)
	}
	guide := readTestFile(t, filepath.Join(root, "docs", "guide.md"))
	if !strings.Contains(guide, "| longer | x     |") {
		t.Fatalf("guide.md was not formatted:\n%s", guide)
	}
	if !strings.Contains(stdout.String(), "README.md") || !strings.Contains(stdout.String(), "docs/guide.md") {
		t.Fatalf("stdout = %q, want changed files", stdout.String())
	}
}

func TestRun_CheckFailsWithoutWriting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	original := "| A | B |\n| --- | --- |\n| longer | x |\n"
	writeTestFile(t, path, original)
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--root", root, "--check"}, &stdout)
	if err == nil {
		t.Fatal("run() error = nil, want check failure")
	}
	if !strings.Contains(err.Error(), "README.md") {
		t.Fatalf("run() error = %v, want README.md", err)
	}
	if got := readTestFile(t, path); got != original {
		t.Fatalf("check mode wrote file:\n%s", got)
	}
}

func TestRun_ExplicitPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "custom.md")
	writeTestFile(t, path, "| A | B |\n| --- | --- |\n| one | two |\n")

	var stdout bytes.Buffer
	if err := run([]string{"--root", root, "custom.md"}, &stdout); err != nil {
		t.Fatalf("run() error: %v", err)
	}
	if got := readTestFile(t, path); !strings.Contains(got, "| one | two |") {
		t.Fatalf("custom.md was not processed:\n%s", got)
	}
}

func TestRun_CheckSucceedsWhenFormatted(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Title\n")
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"--root", root, "--check"}, &stdout); err != nil {
		t.Fatalf("run() error: %v", err)
	}
	if !strings.Contains(stdout.String(), "up to date") {
		t.Fatalf("stdout = %q, want up to date", stdout.String())
	}
}

func TestRun_ReportsAlreadyFormatted(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Title\n")
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"--root", root}, &stdout); err != nil {
		t.Fatalf("run() error: %v", err)
	}
	if !strings.Contains(stdout.String(), "already formatted") {
		t.Fatalf("stdout = %q, want already formatted", stdout.String())
	}
}

func TestRun_RejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	err := run([]string{"--root", root, "../outside.md"}, &stdout)
	if err == nil {
		t.Fatal("run() error = nil, want root escape failure")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("run() error = %v, want escapes root", err)
	}
}

func TestDiscoverMarkdownFiles_SortsMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "docs", "b.md"), "# B\n")
	writeTestFile(t, filepath.Join(root, "docs", "a.md"), "# A\n")
	writeTestFile(t, filepath.Join(root, "docs", "ignored.txt"), "# Ignored\n")

	files, err := discoverMarkdownFiles(root, []string{"docs"})
	if err != nil {
		t.Fatalf("discoverMarkdownFiles() error: %v", err)
	}
	want := []string{
		filepath.Join(root, "docs", "a.md"),
		filepath.Join(root, "docs", "b.md"),
	}
	if strings.Join(files, "\n") != strings.Join(want, "\n") {
		t.Fatalf("discoverMarkdownFiles() = %#v, want %#v", files, want)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
