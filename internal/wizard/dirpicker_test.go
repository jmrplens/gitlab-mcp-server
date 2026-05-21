package wizard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPickDirectory_LinuxDialogTools verifies Linux dialog discovery, command
// output parsing, and missing-tool handling with fake binaries in PATH.
func TestPickDirectory_LinuxDialogTools(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only dialog tool test")
	}

	t.Run("missing dialog tools", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := pickDirectory(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no dialog tool") {
			t.Fatalf("pickDirectory() error = %v, want missing dialog tool", err)
		}
	})

	t.Run("zenity success", func(t *testing.T) {
		binDir := t.TempDir()
		selected := t.TempDir()
		writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nprintf '%s\\n' \""+selected+"\"\n")
		t.Setenv("PATH", binDir)

		got, err := pickDirectory(t.TempDir())
		if err != nil {
			t.Fatalf("pickDirectory() error = %v", err)
		}
		if got != selected {
			t.Fatalf("pickDirectory() = %q, want %q", got, selected)
		}
	})

	t.Run("kdialog fallback", func(t *testing.T) {
		binDir := t.TempDir()
		selected := t.TempDir()
		writeFakeDialogTool(t, binDir, "kdialog", "#!/bin/sh\nprintf '%s\\n' \""+selected+"\"\n")
		t.Setenv("PATH", binDir)

		got, err := pickDirectory("")
		if err != nil {
			t.Fatalf("pickDirectory() error = %v", err)
		}
		if got != selected {
			t.Fatalf("pickDirectory() = %q, want %q", got, selected)
		}
	})

	t.Run("empty selection", func(t *testing.T) {
		binDir := t.TempDir()
		writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nexit 0\n")
		t.Setenv("PATH", binDir)

		_, err := pickDirectory(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no directory selected") {
			t.Fatalf("pickDirectory() error = %v, want empty selection", err)
		}
	})

	t.Run("dialog failure", func(t *testing.T) {
		binDir := t.TempDir()
		writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nexit 1\n")
		t.Setenv("PATH", binDir)

		_, err := pickDirectory(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "dialog cancelled or failed") {
			t.Fatalf("pickDirectory() error = %v, want command failure", err)
		}
	})
}

func writeFakeDialogTool(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // Executable dialog fixture is required for PATH lookup.
		t.Fatal(err)
	}
}
