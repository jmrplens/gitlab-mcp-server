// main_test.go covers the format_md_tables command, which normalizes
// Markdown pipe tables in README.md and docs/.
//
// Tests use table-driven cases to validate default discovery, check mode,
// explicit paths, and invalid arguments. Symlink-escape rejection, write
// failures, and read errors are exercised end-to-end with temp roots.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
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
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
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
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
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

// TestRun_OpenRootError verifies that run reports an error when --root points
// to a non-existent directory.
//
// Without this guard the formatter would call filepath.Abs on an empty string
// and silently continue, masking CLI misuse. The expected error includes the
// failing path.
func TestRun_OpenRootError(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "does-not-exist")
	var stdout bytes.Buffer
	err := run([]string{"--root", missing}, &stdout)
	if err == nil {
		t.Fatal("run() error = nil, want open root failure")
	}
	if !strings.Contains(err.Error(), "open root") {
		t.Fatalf("run() error = %v, want open root", err)
	}
}

// TestRun_FormatFileReadError verifies that [formatMarkdownTableFile] surfaces
// a read error when a discovered Markdown file disappears between the
// discovery stat and the read step.
//
// This is exercised by calling formatMarkdownTableFile directly; the
// run/discover flow uses a separate stat call that would mask the inner
// read error.
func TestRun_FormatFileReadError(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ghost.md")
	writeTestFile(t, target, "| A | B |\n| --- | --- |\n| longer | x |\n")
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove: %v", err)
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer rootFS.Close()

	_, err = formatMarkdownTableFile(rootFS, "ghost.md", false)
	if err == nil {
		t.Fatal("formatMarkdownTableFile() error = nil, want read failure")
	}
	if !strings.Contains(err.Error(), "read ghost.md") {
		t.Fatalf("formatMarkdownTableFile() error = %v, want read ghost.md", err)
	}
}

// TestRun_StdoutWriteErrorsAfterFormatChange verifies that run surfaces
// Fprintf failures when emitting the per-file change list after formatting.
//
// The test stages a single Markdown file that requires normalization, then
// uses an errWriter that fails on the very first write. The expected error
// includes "write stdout" and must be returned to the caller, not swallowed.
func TestRun_StdoutWriteErrorsAfterFormatChange(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "| A | B |\n| --- | ---: |\n| one | 2 |\n")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	err := run([]string{"--root", root}, errWriter{})
	if err == nil {
		t.Fatal("run() error = nil, want stdout write failure after format change")
	}
	if !strings.Contains(err.Error(), "write stdout") {
		t.Fatalf("run() error = %v, want write stdout", err)
	}
}

// TestMarkdownFilesForInput_NonMarkdownFile verifies that an explicit .txt path
// returns an empty list without erroring.
//
// The formatter is invoked against README.md and a docs/ directory by default;
// non-Markdown files must be silently skipped to keep the CLI behavior
// predictable when callers pass arbitrary paths.
func TestMarkdownFilesForInput_NonMarkdownFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "ignored.txt"), "not markdown")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer rootFS.Close()

	files, err := markdownFilesForInput(rootFS, root, "ignored.txt")
	if err != nil {
		t.Fatalf("markdownFilesForInput() error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("markdownFilesForInput() = %v, want empty slice for .txt", files)
	}
}

// TestMarkdownFilesForInput_MissingPath verifies that referencing a
// non-existent explicit path returns a stat error that names the offending
// input.
//
// The error must reference the original (pre-resolution) input string so
// users can locate the bad argument in their invocation.
func TestMarkdownFilesForInput_MissingPath(t *testing.T) {
	root := t.TempDir()
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer rootFS.Close()

	_, err = markdownFilesForInput(rootFS, root, "missing.md")
	if err == nil {
		t.Fatal("markdownFilesForInput() error = nil, want stat failure")
	}
	if !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("markdownFilesForInput() error = %v, want missing.md in message", err)
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Fatalf("markdownFilesForInput() error = %v, want stat prefix", err)
	}
}

// TestFormatMarkdownTableFile_WriteError verifies that formatMarkdownTableFile
// returns a write error when the target file cannot be overwritten.
//
// The test stages a read-only Markdown file that requires normalization, then
// invokes formatMarkdownTableFile in non-check mode. The expected error wraps
// a "write" prefix and the failing path.
func TestFormatMarkdownTableFile_WriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce POSIX read-only via os.Chmod; cannot reliably trigger a write failure")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses POSIX file mode checks; cannot trigger a write failure via chmod")
	}
	root := t.TempDir()
	target := filepath.Join(root, "readonly.md")
	writeTestFile(t, target, "| A | B |\n| --- | ---: |\n| one | 2 |\n")
	if err := os.Chmod(target, 0o400); err != nil { // 0o400 makes the staging file read-only, forcing a write failure downstream
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o600) })
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer rootFS.Close()

	_, err = formatMarkdownTableFile(rootFS, "readonly.md", false)
	if err == nil {
		t.Fatal("formatMarkdownTableFile() error = nil, want write failure")
	}
	if !strings.Contains(err.Error(), "write readonly.md") {
		t.Fatalf("formatMarkdownTableFile() error = %v, want write readonly.md", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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
