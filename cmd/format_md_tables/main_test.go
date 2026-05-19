package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		files        map[string]string
		dirs         []string
		wantErr      string
		wantStdout   []string
		wantContains map[string]string
		wantExact    map[string]string
	}{
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
			root := t.TempDir()
			for path, content := range tt.files {
				writeTestFile(t, filepath.Join(root, filepath.FromSlash(path)), content)
			}
			for _, dir := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}

			var stdout bytes.Buffer
			args := append([]string{"--root", root}, tt.args...)
			err := run(args, &stdout)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("run() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("run() error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("run() error: %v", err)
			}

			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want %q", stdout.String(), want)
				}
			}
			for path, want := range tt.wantContains {
				got := readTestFile(t, filepath.Join(root, filepath.FromSlash(path)))
				if !strings.Contains(got, want) {
					t.Fatalf("%s =\n%s\nwant substring %q", path, got, want)
				}
			}
			for path, want := range tt.wantExact {
				got := readTestFile(t, filepath.Join(root, filepath.FromSlash(path)))
				if got != want {
					t.Fatalf("%s =\n%s\nwant\n%s", path, got, want)
				}
			}
		})
	}
}

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
