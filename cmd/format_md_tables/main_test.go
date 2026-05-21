package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type runTableDrivenCase struct {
	name         string
	args         []string
	files        map[string]string
	dirs         []string
	wantErr      string
	wantStdout   []string
	wantContains map[string]string
	wantExact    map[string]string
}

// TestRun_TableDriven verifies the Markdown table formatter CLI handles default
// discovery, check mode, explicit paths, and invalid arguments.
//
// Each subtest builds a temporary repository root, writes only the files needed
// for the scenario, runs [run], and asserts stdout, file rewrites, or expected
// errors. This protects both the developer workflow and the non-mutating
// --check mode used by CI.
func TestRun_TableDriven(t *testing.T) {
	tests := []runTableDrivenCase{
		{
			name: "formats default markdown files",
			files: map[string]string{
				"README.md":        "| A | B |\n| --- | ---: |\n| one | 2 |\n",
				"docs/guide.md":    "| Name | Value |\n| --- | --- |\n| longer | x |\n",
				"docs/ignored.txt": "| A | B |\n| --- | --- |\n",
			},
			wantStdout: []string{"README.md", "docs/guide.md"},
			wantContains: map[string]string{
				"README.md":     "| one |    2 |",
				"docs/guide.md": "| longer | x     |",
			},
		},
		{
			name:    "check fails without writing",
			args:    []string{"--check"},
			files:   map[string]string{"README.md": "| A | B |\n| --- | --- |\n| longer | x |\n"},
			dirs:    []string{"docs"},
			wantErr: "README.md",
			wantExact: map[string]string{
				"README.md": "| A | B |\n| --- | --- |\n| longer | x |\n",
			},
		},
		{
			name: "formats explicit path",
			args: []string{"custom.md"},
			files: map[string]string{
				"custom.md": "| A | B |\n| --- | ---: |\n| one | 2 |\n",
			},
			wantContains: map[string]string{"custom.md": "| one |    2 |"},
		},
		{
			name:       "check succeeds when formatted",
			args:       []string{"--check"},
			files:      map[string]string{"README.md": "# Title\n"},
			dirs:       []string{"docs"},
			wantStdout: []string{"up to date"},
		},
		{
			name:       "reports already formatted",
			files:      map[string]string{"README.md": "# Title\n"},
			dirs:       []string{"docs"},
			wantStdout: []string{"already formatted"},
		},
		{
			name:    "rejects path outside root",
			args:    []string{"../outside.md"},
			wantErr: "escapes root",
		},
		{
			name:    "rejects invalid flag",
			args:    []string{"--missing"},
			wantErr: "flag provided but not defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFormatterCase(t, tt)
		})
	}
}

func runFormatterCase(t *testing.T, tt runTableDrivenCase) {
	t.Helper()
	root := t.TempDir()
	writeFormatterCaseFiles(t, root, tt)
	var stdout bytes.Buffer
	err := run(append([]string{"--root", root}, tt.args...), &stdout)
	assertFormatterCaseResult(t, root, stdout.String(), err, tt)
}

func writeFormatterCaseFiles(t *testing.T, root string, tt runTableDrivenCase) {
	t.Helper()
	for path, content := range tt.files {
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(path)), content)
	}
	for _, dir := range tt.dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
}

func assertFormatterCaseResult(t *testing.T, root, stdout string, err error, tt runTableDrivenCase) {
	t.Helper()
	if tt.wantErr != "" {
		assertRunError(t, err, tt.wantErr)
		return
	}
	if err != nil {
		t.Fatalf("run() error: %v", err)
	}
	assertStdoutContains(t, stdout, tt.wantStdout)
	assertFilesContain(t, root, tt.wantContains)
	assertFilesEqual(t, root, tt.wantExact)
}

func assertRunError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("run() error = %v, want %q", err, want)
	}
}

func assertStdoutContains(t *testing.T, stdout string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func assertFilesContain(t *testing.T, root string, wants map[string]string) {
	t.Helper()
	for path, want := range wants {
		got := readTestFile(t, filepath.Join(root, filepath.FromSlash(path)))
		if !strings.Contains(got, want) {
			t.Fatalf("%s =\n%s\nwant substring %q", path, got, want)
		}
	}
}

func assertFilesEqual(t *testing.T, root string, wants map[string]string) {
	t.Helper()
	for path, want := range wants {
		got := readTestFile(t, filepath.Join(root, filepath.FromSlash(path)))
		if got != want {
			t.Fatalf("%s =\n%s\nwant\n%s", path, got, want)
		}
	}
}

// TestDiscoverMarkdownFiles_SortsMarkdownFiles verifies recursive discovery
// returns only Markdown files in deterministic order.
//
// The test creates two docs files and one ignored text file, then expects the
// result to contain the .md paths sorted lexically. Stable ordering keeps CLI
// output and formatting diffs predictable.
func TestDiscoverMarkdownFiles_SortsMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "docs", "b.md"), "# B\n")
	writeTestFile(t, filepath.Join(root, "docs", "a.md"), "# A\n")
	writeTestFile(t, filepath.Join(root, "docs", "ignored.txt"), "# Ignored\n")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer rootFS.Close()

	files, err := discoverMarkdownFiles(rootFS, root, []string{"docs"})
	if err != nil {
		t.Fatalf("discoverMarkdownFiles() error: %v", err)
	}
	want := []string{
		filepath.Join("docs", "a.md"),
		filepath.Join("docs", "b.md"),
	}
	if strings.Join(files, "\n") != strings.Join(want, "\n") {
		t.Fatalf("discoverMarkdownFiles() = %#v, want %#v", files, want)
	}
}

// TestRun_RejectsSymlinkEscapingRoot verifies the formatter refuses symlinked
// Markdown files that resolve outside the configured repository root.
//
// The test creates a docs symlink pointing to a file in another temporary
// directory and expects [run] to report the escaping link. This guards the
// command against path traversal through Markdown discovery.
func TestRun_RejectsSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Title\n")
	writeTestFile(t, filepath.Join(outside, "target.md"), "| A | B |\n| --- | --- |\n")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "target.md"), filepath.Join(root, "docs", "link.md")); err != nil {
		t.Skipf("symlink not available: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--root", root}, &stdout)
	if err == nil {
		t.Fatal("run() error = nil, want symlink escape failure")
	}
	if !strings.Contains(err.Error(), "link.md") {
		t.Fatalf("run() error = %v, want link.md", err)
	}
}

// TestRun_ReturnsStdoutWriteErrors verifies CLI status output write failures are
// returned to the caller.
//
// The test uses an [errWriter] after creating an otherwise valid root. The
// expected error includes "write stdout", proving that output failures are not
// silently ignored after formatting completes.
func TestRun_ReturnsStdoutWriteErrors(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Title\n")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	err := run([]string{"--root", root}, errWriter{})
	if err == nil {
		t.Fatal("run() error = nil, want stdout write failure")
	}
	if !strings.Contains(err.Error(), "write stdout") {
		t.Fatalf("run() error = %v, want write stdout", err)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
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
