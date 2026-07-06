package wizard

import (
	"context"
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

// TestDirectoryPickerCommand_WindowsBuilder verifies the Windows PowerShell
// FolderBrowserDialog command construction, including single-quote escaping
// of the start directory. The extracted builder makes the Windows branch
// testable on any host without opening a real dialog.
func TestDirectoryPickerCommand_WindowsBuilder(t *testing.T) {
	ctx := context.Background()

	t.Run("with start directory escapes single quotes", func(t *testing.T) {
		cmd, err := directoryPickerCommand(ctx, "windows", `C:\it's here`)
		if err != nil {
			t.Fatalf("directoryPickerCommand(windows) error = %v", err)
		}
		args := cmd.Args
		if len(args) != 5 || args[0] != "powershell" || args[1] != "-NoProfile" || args[2] != "-STA" || args[3] != "-Command" {
			t.Fatalf("directoryPickerCommand(windows) args = %v, want powershell -NoProfile -STA -Command <script>", args)
		}
		script := args[4]
		if !strings.Contains(script, `C:\it''s here`) {
			t.Errorf("script %q does not contain escaped start directory", script)
		}
		if !strings.Contains(script, "FolderBrowserDialog") {
			t.Errorf("script %q does not open FolderBrowserDialog", script)
		}
	})

	t.Run("without start directory keeps empty selection guard", func(t *testing.T) {
		cmd, err := directoryPickerCommand(ctx, "windows", "")
		if err != nil {
			t.Fatalf("directoryPickerCommand(windows) error = %v", err)
		}
		if !strings.Contains(cmd.Args[4], `if ('' -ne '')`) {
			t.Errorf("script %q does not guard empty start directory", cmd.Args[4])
		}
	})
}

// TestDirectoryPickerCommand_DarwinBuilder verifies the macOS osascript
// choose-folder command construction, including double-quote escaping and
// the default-location clause added only when a start directory is given.
// The extracted builder makes the darwin branch testable on any host.
func TestDirectoryPickerCommand_DarwinBuilder(t *testing.T) {
	ctx := context.Background()

	t.Run("without start directory", func(t *testing.T) {
		cmd, err := directoryPickerCommand(ctx, "darwin", "")
		if err != nil {
			t.Fatalf("directoryPickerCommand(darwin) error = %v", err)
		}
		want := `POSIX path of (choose folder with prompt "Select installation directory")`
		if len(cmd.Args) != 3 || cmd.Args[0] != "osascript" || cmd.Args[1] != "-e" || cmd.Args[2] != want {
			t.Fatalf("directoryPickerCommand(darwin) args = %v, want osascript -e %q", cmd.Args, want)
		}
	})

	t.Run("with start directory escapes double quotes", func(t *testing.T) {
		cmd, err := directoryPickerCommand(ctx, "darwin", `/tmp/"quoted"`)
		if err != nil {
			t.Fatalf("directoryPickerCommand(darwin) error = %v", err)
		}
		script := cmd.Args[2]
		if !strings.Contains(script, `default location POSIX file "/tmp/\"quoted\""`) {
			t.Errorf("script %q does not contain escaped default location", script)
		}
	})
}

// TestPickDirectoryOn_LinuxDispatch verifies the GOOS-parameterized
// pickDirectoryOn on the Linux code path from any host: missing dialog tools
// yield the install hint, a fake zenity script's stdout becomes the
// selection, a failing tool maps to the cancelled error, and empty output
// maps to the no-selection error. Windows is skipped because the fixtures
// are shell scripts.
func TestPickDirectoryOn_LinuxDispatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script dialog fixtures require a Unix host")
	}

	t.Run("missing dialog tools", func(t *testing.T) {
		stubLinuxDialogToolPaths(t, nil)
		_, err := pickDirectoryOn("linux", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no dialog tool available (install zenity or kdialog)") {
			t.Fatalf("pickDirectoryOn(linux) error = %v, want missing dialog tool", err)
		}
	})

	t.Run("zenity output becomes selection", func(t *testing.T) {
		binDir := t.TempDir()
		selected := t.TempDir()
		zenityPath := writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nprintf '%s\\n' \""+selected+"\"\n")
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": zenityPath})

		got, err := pickDirectoryOn("linux", t.TempDir())
		if err != nil {
			t.Fatalf("pickDirectoryOn(linux) error = %v", err)
		}
		if got != selected {
			t.Fatalf("pickDirectoryOn(linux) = %q, want %q", got, selected)
		}
	})

	t.Run("dialog failure", func(t *testing.T) {
		binDir := t.TempDir()
		zenityPath := writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nexit 1\n")
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": zenityPath})

		_, err := pickDirectoryOn("linux", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "dialog cancelled or failed") {
			t.Fatalf("pickDirectoryOn(linux) error = %v, want command failure", err)
		}
	})

	t.Run("empty selection", func(t *testing.T) {
		binDir := t.TempDir()
		zenityPath := writeFakeDialogTool(t, binDir, "zenity", "#!/bin/sh\nexit 0\n")
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": zenityPath})

		_, err := pickDirectoryOn("linux", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no directory selected") {
			t.Fatalf("pickDirectoryOn(linux) error = %v, want empty selection", err)
		}
	})
}

// TestFixedLinuxDialogToolPath_SkipsNonFixedDirs verifies the search skips
// candidate directories that are world/group writable (not "fixed") or do
// not exist, and still finds an executable tool in a later fixed directory.
// The directory list is swapped for test-controlled temp directories.
func TestFixedLinuxDialogToolPath_SkipsNonFixedDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission fixtures require a Unix host")
	}

	writable := t.TempDir()
	if err := os.Chmod(writable, 0o777); err != nil { //nolint:gosec // world-writable fixture is required to trigger the skip branch
		t.Fatal(err)
	}
	fixed := t.TempDir()
	if err := os.Chmod(fixed, 0o755); err != nil { //nolint:gosec // non-writable-for-group directory fixture is required for the fixed-dir check
		t.Fatal(err)
	}
	writeFakeDialogTool(t, writable, "zenity", "#!/bin/sh\nexit 0\n")
	toolPath := writeFakeDialogTool(t, fixed, "zenity", "#!/bin/sh\nexit 0\n")

	original := linuxDialogToolDirs
	linuxDialogToolDirs = []string{writable, filepath.Join(fixed, "does-not-exist"), fixed}
	t.Cleanup(func() { linuxDialogToolDirs = original })

	got, ok := fixedLinuxDialogToolPath("zenity")
	if !ok {
		t.Fatal("fixedLinuxDialogToolPath(zenity) = false, want tool in fixed directory")
	}
	if got != toolPath {
		t.Fatalf("fixedLinuxDialogToolPath(zenity) = %q, want %q (writable dir must be skipped)", got, toolPath)
	}
}

// TestLinuxDirectoryPickerCommand_ArgumentShapes verifies the exact argument
// lists built for zenity and kdialog, with and without a start directory,
// plus the error when neither tool is available. The dialog tool lookup is
// stubbed so every branch runs on any host.
func TestLinuxDirectoryPickerCommand_ArgumentShapes(t *testing.T) {
	ctx := context.Background()

	assertArgs := func(t *testing.T, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("args = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("args = %v, want %v", got, want)
			}
		}
	}

	t.Run("zenity with start directory", func(t *testing.T) {
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": "/fixed/zenity"})
		cmd, err := linuxDirectoryPickerCommand(ctx, "/srv/data")
		if err != nil {
			t.Fatalf("linuxDirectoryPickerCommand error = %v", err)
		}
		assertArgs(t, cmd.Args, []string{"/fixed/zenity", "--file-selection", "--directory", "--title=Select installation directory", "--filename=/srv/data/"})
	})

	t.Run("zenity without start directory", func(t *testing.T) {
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": "/fixed/zenity"})
		cmd, err := linuxDirectoryPickerCommand(ctx, "")
		if err != nil {
			t.Fatalf("linuxDirectoryPickerCommand error = %v", err)
		}
		assertArgs(t, cmd.Args, []string{"/fixed/zenity", "--file-selection", "--directory", "--title=Select installation directory"})
	})

	t.Run("kdialog with start directory", func(t *testing.T) {
		stubLinuxDialogToolPaths(t, map[string]string{"kdialog": "/fixed/kdialog"})
		cmd, err := linuxDirectoryPickerCommand(ctx, "/srv/data")
		if err != nil {
			t.Fatalf("linuxDirectoryPickerCommand error = %v", err)
		}
		assertArgs(t, cmd.Args, []string{"/fixed/kdialog", "--getexistingdirectory", "/srv/data"})
	})

	t.Run("kdialog without start directory defaults to cwd", func(t *testing.T) {
		stubLinuxDialogToolPaths(t, map[string]string{"kdialog": "/fixed/kdialog"})
		cmd, err := linuxDirectoryPickerCommand(ctx, "")
		if err != nil {
			t.Fatalf("linuxDirectoryPickerCommand error = %v", err)
		}
		assertArgs(t, cmd.Args, []string{"/fixed/kdialog", "--getexistingdirectory", "."})
	})

	t.Run("no tool available", func(t *testing.T) {
		stubLinuxDialogToolPaths(t, nil)
		if _, err := linuxDirectoryPickerCommand(ctx, ""); err == nil {
			t.Fatal("linuxDirectoryPickerCommand error = nil, want no fixed dialog tool found")
		}
	})
}

// TestPickDirectory_UsesRuntimePlatform verifies the thin pickDirectory
// wrapper dispatches on the real runtime.GOOS. The host's dialog tooling is
// stubbed per platform (fake zenity lookup on Linux/BSD, fake osascript on
// PATH for macOS) so no real dialog opens; Windows is skipped because
// PowerShell cannot be safely shadowed.
func TestPickDirectory_UsesRuntimePlatform(t *testing.T) {
	selected := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"" + selected + "\"\n"

	switch runtime.GOOS {
	case "windows":
		t.Skip("cannot shadow the PowerShell dialog on Windows")
	case "darwin":
		binDir := t.TempDir()
		writeFakeDialogTool(t, binDir, "osascript", script)
		t.Setenv("PATH", binDir)
	default: // linux and BSDs
		binDir := t.TempDir()
		zenityPath := writeFakeDialogTool(t, binDir, "zenity", script)
		stubLinuxDialogToolPaths(t, map[string]string{"zenity": zenityPath})
	}

	got, err := pickDirectory("")
	if err != nil {
		t.Fatalf("pickDirectory() error = %v", err)
	}
	if got != selected {
		t.Fatalf("pickDirectory() = %q, want %q", got, selected)
	}
}
