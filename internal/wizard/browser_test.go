package wizard

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestHasDisplay_NonLinuxAlwaysTrue verifies that non-Linux/BSD platforms
// always report a display (macOS/Windows).
func TestHasDisplay_NonLinuxAlwaysTrue(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" ||
		runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" {
		t.Skip("non-Linux/BSD test")
	}
	if !hasDisplay() {
		t.Fatal("hasDisplay() = false on non-Linux/BSD, want true")
	}
}

// TestHasDisplay_LinuxEnvironment verifies Linux display detection honors X11
// and Wayland environment variables.
func TestHasDisplay_LinuxEnvironment(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only display environment test")
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if hasDisplay() {
		t.Fatal("hasDisplay() = true without DISPLAY or WAYLAND_DISPLAY")
	}

	t.Setenv("DISPLAY", ":1")
	if !hasDisplay() {
		t.Fatal("hasDisplay() = false with DISPLAY set")
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-1")
	if !hasDisplay() {
		t.Fatal("hasDisplay() = false with WAYLAND_DISPLAY set")
	}
}

// TestHasDisplay_BSDEnvironment verifies BSD platforms use the same
// environment-variable convention as Linux.
func TestHasDisplay_BSDEnvironment(t *testing.T) {
	bsd := runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd"
	if !bsd {
		t.Skip("BSD-only display environment test")
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if hasDisplay() {
		t.Fatal("hasDisplay() = true without DISPLAY or WAYLAND_DISPLAY on BSD")
	}

	t.Setenv("DISPLAY", ":0")
	if !hasDisplay() {
		t.Fatal("hasDisplay() = false with DISPLAY set on BSD")
	}
}

// TestOpenBrowser_LinuxUsesXDGOpen verifies openBrowser invokes xdg-open from
// PATH on Linux without launching a real browser.
func TestOpenBrowser_LinuxUsesXDGOpen(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only xdg-open test")
	}

	binDir := t.TempDir()
	xdgOpen := filepath.Join(binDir, "xdg-open")
	if err := os.WriteFile(xdgOpen, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil { //nolint:gosec // Executable fixture is required for PATH lookup.
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	if err := openBrowser("http://127.0.0.1:12345"); err != nil {
		t.Fatalf("openBrowser() error = %v", err)
	}
}

// TestOpenBrowser_LinuxStartError verifies xdg-open startup failures are
// returned as actionable errors.
func TestOpenBrowser_LinuxStartError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only xdg-open test")
	}

	t.Setenv("PATH", t.TempDir())
	if err := openBrowser("http://127.0.0.1:12345"); err == nil {
		t.Fatal("openBrowser() error = nil, want missing xdg-open error")
	}
}

// TestOpenBrowser_StartErrorAllPlatforms exercises the cmd.Start() error
// branch in openBrowser by pointing PATH at a directory that has a stub
// for the platform's browser command, but the stub is a non-executable
// file. This causes exec.Cmd.Start to return an error.
func TestOpenBrowser_StartErrorAllPlatforms(t *testing.T) {
	binDir := t.TempDir()
	var stubName string
	switch runtime.GOOS {
	case "darwin":
		stubName = "open"
	case "windows":
		t.Skip("windows rundll32 path is not configurable via PATH")
	case "linux", "freebsd", "openbsd", "netbsd":
		stubName = "xdg-open"
	default:
		t.Skipf("unsupported GOOS: %s", runtime.GOOS)
	}

	// Write a non-executable stub so cmd.Start returns an EACCES-like
	// error (start fails because the binary cannot be exec'd).
	stubPath := filepath.Join(binDir, stubName)
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil { //nolint:gosec // non-executable stub is required to trigger start error.
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	if err := openBrowser("http://127.0.0.1:65535/test"); err == nil {
		t.Fatal("openBrowser() error = nil, want non-executable stub start error")
	}
}

// TestOpenBrowser_NonLinux verifies openBrowser uses the platform-specific
// binary (open on macOS, rundll32 on Windows) without actually launching a
// real browser. The test stubs the platform binary in PATH so the
// `exec.CommandContext(...).Start()` call succeeds against a no-op
// executable.
func TestOpenBrowser_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" ||
		runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" {
		t.Skip("non-Linux/BSD test")
	}

	binDir := t.TempDir()
	var stub string
	switch runtime.GOOS {
	case "darwin":
		stub = "open"
	case "windows":
		// Rundll32 lives in System32 and cannot be shadowed via PATH.
		// Calling openBrowser() here would spawn a real browser window
		// on the developer's host, which is a disruptive side effect
		// for a unit test. Skip the live launch; the Windows branch is
		// covered indirectly via compile-time type checking.
		t.Skip("Windows cannot shadow rundll32 via PATH; skipping live browser launch")
	default:
		t.Skipf("unexpected GOOS: %s", runtime.GOOS)
	}

	stubPath := filepath.Join(binDir, stub)
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil { //nolint:gosec // Executable fixture is required for PATH lookup.
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	if err := openBrowser("http://127.0.0.1:65535/test"); err != nil {
		t.Fatalf("openBrowser() error = %v, want nil from stub", err)
	}
}
