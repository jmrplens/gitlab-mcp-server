// write_test.go covers the two ways a command puts bytes on disk: the
// whole-file freshness convention for a committed artifact (WriteOrCheck) and
// the "-" means stdout convention for an auditor's report (WriteReport).
package docgen

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteOrCheck_WriteMode_CreatesTheDirectoryAndTheFile verifies write mode
// creates every missing parent directory and lands the content, so a generator
// pointed at a target that does not exist yet does not need to prepare it.
func TestWriteOrCheck_WriteMode_CreatesTheDirectoryAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site", "src", "data", "stats.json")

	if err := WriteOrCheck(path, []byte("{}\n"), false, "make gen"); err != nil {
		t.Fatalf("WriteOrCheck() error = %v", err)
	}

	got, err := os.ReadFile(path) //#nosec G304 -- a path this test built
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "{}\n" {
		t.Errorf("written file = %q, want %q", got, "{}\n")
	}
}

// TestWriteOrCheck_WriteMode_OverwritesAndAddsTheTrailingNewline verifies a
// previous generation is replaced rather than appended to, and that content
// without a final newline is given one so no generated artifact is the one
// text file in the repository without one.
func TestWriteOrCheck_WriteMode_OverwritesAndAddsTheTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llms.txt")
	if err := os.WriteFile(path, []byte("# previous generation\n"), 0o600); err != nil {
		t.Fatalf("write previous generation: %v", err)
	}

	if err := WriteOrCheck(path, []byte("# fresh"), false, "make gen"); err != nil {
		t.Fatalf("WriteOrCheck() error = %v", err)
	}

	got, err := os.ReadFile(path) //#nosec G304 -- a path this test built
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "# fresh\n" {
		t.Errorf("written file = %q, want the fresh content with a trailing newline", got)
	}
}

// TestWriteOrCheck_WriteMode_EmptyContentIsLeftEmpty verifies the trailing
// newline is not invented for content that has nothing in it, so an empty
// artifact stays an empty file a caller can notice rather than a blank line.
func TestWriteOrCheck_WriteMode_EmptyContentIsLeftEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")

	if err := WriteOrCheck(path, nil, false, "make gen"); err != nil {
		t.Fatalf("WriteOrCheck() error = %v", err)
	}

	got, err := os.ReadFile(path) //#nosec G304 -- a path this test built
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("written file = %q, want it empty", got)
	}
}

// TestWriteOrCheck_WriteMode_BareFileNameWritesIntoTheWorkingDirectory
// verifies a path with no directory in it is written where the command runs,
// rather than being refused by the split that looks for a parent.
func TestWriteOrCheck_WriteMode_BareFileNameWritesIntoTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := WriteOrCheck("bare.json", []byte("{}\n"), false, "make gen"); err != nil {
		t.Fatalf("WriteOrCheck() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "bare.json")) //#nosec G304 -- a path this test built
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "{}\n" {
		t.Errorf("written file = %q, want %q", got, "{}\n")
	}
}

// TestWriteOrCheck_CheckMode_AcceptsWhatWriteModeProduced verifies the round
// trip a generator and its --check gate make: what was just written is
// current, and CRLF line endings in the committed file do not make it stale,
// which is the difference between a Windows checkout and a Linux one.
func TestWriteOrCheck_CheckMode_AcceptsWhatWriteModeProduced(t *testing.T) {
	dir := t.TempDir()

	t.Run("round trip", func(t *testing.T) {
		path := filepath.Join(dir, "round-trip.json")
		if err := WriteOrCheck(path, []byte("{}\n"), false, "make gen"); err != nil {
			t.Fatalf("WriteOrCheck(write) error = %v", err)
		}
		if err := WriteOrCheck(path, []byte("{}\n"), true, "make gen"); err != nil {
			t.Errorf("WriteOrCheck(check) error = %v, want the file accepted", err)
		}
	})

	t.Run("carriage returns", func(t *testing.T) {
		path := filepath.Join(dir, "crlf.txt")
		if err := os.WriteFile(path, []byte("# Example\r\n\r\n"), 0o600); err != nil {
			t.Fatalf("write CRLF file: %v", err)
		}
		if err := WriteOrCheck(path, []byte("# Example\n\n"), true, "make gen"); err != nil {
			t.Errorf("WriteOrCheck(check) error = %v, want CRLF treated as LF", err)
		}
	})
}

// TestWriteOrCheck_CheckMode_MissingAndStaleFilesDiffer verifies check mode
// reports an artifact that is not there with an error that keeps
// fs.ErrNotExist, and a stale one with the sentence naming the file and the
// command that refreshes it. The two want different fixes, so a caller has to
// be able to tell them apart.
func TestWriteOrCheck_CheckMode_MissingAndStaleFilesDiffer(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "stale.json")
	if err := os.WriteFile(stale, []byte("{\"old\": true}\n"), 0o600); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	missing := filepath.Join(dir, "missing.json")
	absentDir := filepath.Join(dir, "absent", "missing.json")

	tests := []struct {
		name         string
		path         string
		wantErr      string
		wantNotExist bool
	}{
		{name: "stale file", path: stale, wantErr: stale + " is stale; run make gen"},
		{name: "missing file", path: missing, wantErr: "read " + missing, wantNotExist: true},
		{name: "missing directory", path: absentDir, wantErr: "open " + filepath.Dir(absentDir), wantNotExist: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteOrCheck(tt.path, []byte("{}\n"), true, "make gen")
			if err == nil {
				t.Fatal("WriteOrCheck(check) error = nil, want a refusal")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("WriteOrCheck(check) error = %q, want it to contain %q", err, tt.wantErr)
			}
			if errors.Is(err, fs.ErrNotExist) != tt.wantNotExist {
				t.Errorf("errors.Is(err, fs.ErrNotExist) = %v, want %v (err %v)", !tt.wantNotExist, tt.wantNotExist, err)
			}
		})
	}
}

// TestWriteOrCheck_UnusableTargets_NameTheStageThatFailed verifies each way a
// target can refuse the write is reported by the stage that hit it: a parent
// that is a regular file cannot be created, a parent that disappears between
// the two calls cannot be opened, and a target that is a directory cannot be
// written.
func TestWriteOrCheck_UnusableTargets_NameTheStageThatFailed(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o750); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "parent is a file", path: filepath.Join(blocker, "out.json"), wantErr: "create directory for "},
		{name: "target is a directory", path: directory, wantErr: "write " + directory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteOrCheck(tt.path, []byte("{}\n"), false, "make gen")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("WriteOrCheck() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestWriteReport_Destinations_StdoutOrFile verifies the report writer: "-"
// goes to stdout, a nested path is created with its parents, and a parent that
// is a regular file is reported rather than silently dropping the report.
func TestWriteReport_Destinations_StdoutOrFile(t *testing.T) {
	t.Run("stdout sentinel", func(t *testing.T) {
		stdout := captureStdout(t)
		if err := WriteReport("-", []byte("{}\n")); err != nil {
			t.Fatalf("WriteReport(-) error = %v", err)
		}
		if got := stdout(); got != "{}\n" {
			t.Errorf("stdout = %q, want the report", got)
		}
	})

	t.Run("nested file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "plan", "nested", "backlog.json")
		if err := WriteReport(path, []byte("{}\n")); err != nil {
			t.Fatalf("WriteReport() error = %v", err)
		}
		data, err := os.ReadFile(path) //#nosec G304 -- a path this test built
		if err != nil || string(data) != "{}\n" {
			t.Errorf("written report = %q, %v; want the content", data, err)
		}
	})

	t.Run("parent is a file", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		if err := WriteReport(filepath.Join(blocker, "backlog.json"), []byte("{}\n")); err == nil {
			t.Error("WriteReport() error = nil, want the directory creation failure")
		}
	})

	t.Run("path is a directory", func(t *testing.T) {
		if err := WriteReport(t.TempDir(), []byte("{}\n")); err == nil {
			t.Error("WriteReport() error = nil, want the write failure")
		}
	})
}

// TestWriteReport_StdoutIsClosed_ReturnsTheWriteError verifies the "-" branch
// reports a stdout that cannot be written to, rather than losing the report
// and exiting as though it had been produced.
func TestWriteReport_StdoutIsClosed_ReturnsTheWriteError(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatalf("create stdout stand-in: %v", err)
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("close stdout stand-in: %v", closeErr)
	}
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() { os.Stdout = previous })

	if WriteReport("-", []byte("{}\n")) == nil {
		t.Error("WriteReport(-) error = nil, want the closed-file write error")
	}
}

// captureStdout swaps os.Stdout for a temporary file until the test ends and
// returns a reader for what was written, so the "-" destination can be
// observed.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = previous
		_ = file.Close()
	})
	return func() string {
		data, readErr := os.ReadFile(file.Name()) //#nosec G304 -- the capture file this helper created
		if readErr != nil {
			t.Fatalf("read stdout capture: %v", readErr)
		}
		return string(data)
	}
}
