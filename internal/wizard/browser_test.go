package wizard

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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

// TestOpenBrowser_LinuxUsesXDGOpen verifies openBrowser invokes xdg-open from
// PATH on Linux without launching a real browser.
func TestOpenBrowser_LinuxUsesXDGOpen(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only xdg-open test")
	}

	binDir := t.TempDir()
	xdgOpen := filepath.Join(binDir, "xdg-open")
	if err := os.WriteFile(xdgOpen, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
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
