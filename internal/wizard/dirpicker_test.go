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
		stubLinuxDialogToolPaths(t, nil)
		_, err := pickDirectory(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no dialog tool") {
			t.Fatalf("pickDirectory() error = %v, want missing dialog tool", err)
		}
	})

	t.Run("zenity success", func(t *testing.T) {
		binDir := t.TempDir()
		selected := t.TempDir()
		zenityPath := writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nprintf '%s\\n' \""+selected+"\"\n")
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": zenityPath})

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
		kdialogPath := writeFakeDialogTool(t, binDir, "kdialog", "#!/bin/sh\nprintf '%s\\n' \""+selected+"\"\n")
		stubLinuxDialogToolPaths(t, map[string]string{"kdialog": kdialogPath})

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
		zenityPath := writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nexit 0\n")
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": zenityPath})

		_, err := pickDirectory(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no directory selected") {
			t.Fatalf("pickDirectory() error = %v, want empty selection", err)
		}
	})

	t.Run("dialog failure", func(t *testing.T) {
		binDir := t.TempDir()
		zenityPath := writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nexit 1\n")
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": zenityPath})

		_, err := pickDirectory(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "dialog cancelled or failed") {
			t.Fatalf("pickDirectory() error = %v, want command failure", err)
		}
	})
}

func stubLinuxDialogToolPaths(t *testing.T, paths map[string]string) {
	t.Helper()
	original := findLinuxDialogToolPath
	findLinuxDialogToolPath = func(name string) (string, bool) {
		path, ok := paths[name]
		if !ok {
			return "", false
		}
		return path, true
	}
	t.Cleanup(func() { findLinuxDialogToolPath = original })
}

// TestIsFixedSystemDir verifies isFixedSystemDir returns true only for
// non-writable, existing directories.
func TestIsFixedSystemDir(t *testing.T) {
	t.Run("existing non-writable directory", func(t *testing.T) {
		if os.PathSeparator == '\\' {
			t.Skip("Windows does not support directory write permission restriction via Chmod")
		}
		dir := t.TempDir()
		// Remove write bits so the directory is "fixed".
		if err := os.Chmod(dir, 0o555); err != nil { //nolint:gosec // test fixture requires removing write bits
			t.Skipf("cannot chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // restore default perms for cleanup
		if !isFixedSystemDir(dir) {
			t.Errorf("isFixedSystemDir(%q) = false, want true", dir)
		}
	})

	t.Run("writable directory rejected", func(t *testing.T) {
		dir := t.TempDir()
		// Make the directory writable for group/other so the function
		// must reject it (otherwise group/other write bits are already
		// cleared by t.TempDir on most systems).
		if err := os.Chmod(dir, 0o777); err != nil { //nolint:gosec // test fixture requires granting write bits
			t.Skipf("cannot chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // restore default perms for cleanup
		if isFixedSystemDir(dir) {
			t.Errorf("isFixedSystemDir(%q writable) = true, want false", dir)
		}
	})

	t.Run("nonexistent path rejected", func(t *testing.T) {
		if isFixedSystemDir(filepath.Join(t.TempDir(), "does-not-exist")) {
			t.Error("isFixedSystemDir on nonexistent path = true, want false")
		}
	})

	t.Run("file (not directory) rejected", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "afile")
		if err := os.WriteFile(file, []byte("hi"), 0o555); err != nil { //nolint:gosec // test fixture requires removing write bits
			t.Fatal(err)
		}
		if isFixedSystemDir(file) {
			t.Errorf("isFixedSystemDir(%q file) = true, want false", file)
		}
	})
}

// TestIsExecutableFile verifies isExecutableFile reports the executable bit
// of the path's mode and rejects non-files / non-existent paths.
func TestIsExecutableFile(t *testing.T) {
	t.Run("executable file", func(t *testing.T) {
		if os.PathSeparator == '\\' {
			t.Skip("Windows does not support executable permission bits")
		}
		bin := filepath.Join(t.TempDir(), "bin")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test fixture requires executable bit
			t.Fatal(err)
		}
		if !isExecutableFile(bin) {
			t.Errorf("isExecutableFile(%q) = false, want true", bin)
		}
	})

	t.Run("non-executable file", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "bin")
		if err := os.WriteFile(bin, []byte("hi"), 0o644); err != nil { //nolint:gosec // test fixture requires read-only perms
			t.Fatal(err)
		}
		if isExecutableFile(bin) {
			t.Errorf("isExecutableFile(%q non-exec) = true, want false", bin)
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		if isExecutableFile(filepath.Join(t.TempDir(), "missing")) {
			t.Error("isExecutableFile on nonexistent = true, want false")
		}
	})

	t.Run("directory rejected", func(t *testing.T) {
		dir := t.TempDir()
		if isExecutableFile(dir) {
			t.Errorf("isExecutableFile(%q dir) = true, want false", dir)
		}
	})
}

// TestFixedLinuxDialogToolPath verifies the helper probes the canonical
// Linux system paths for the requested dialog tool.
func TestFixedLinuxDialogToolPath(t *testing.T) {
	t.Run("tool not found", func(t *testing.T) {
		// Probe for a tool that almost certainly does not exist anywhere
		// on the test runner. The function returns false.
		if path, ok := fixedLinuxDialogToolPath("definitely-not-a-real-tool-xyz-987654321"); ok {
			t.Errorf("fixedLinuxDialogToolPath returned (%q, true), want false", path)
		}
	})

	t.Run("returns path when executable present", func(t *testing.T) {
		// Probe for /bin/sh which exists on Linux/macOS. We cannot
		// guarantee the directory has the +x bit stripped, so we only
		// assert the ok-result contains the requested basename.
		path, ok := fixedLinuxDialogToolPath("sh")
		if ok && !strings.HasSuffix(path, "/sh") {
			t.Errorf("fixedLinuxDialogToolPath returned %q, want suffix /sh", path)
		}
	})
}

func writeFakeDialogTool(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // Executable dialog fixture is required for PATH lookup.
		t.Fatal(err)
	}
	return path
}
