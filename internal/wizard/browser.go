package wizard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// openBrowserFn is the function used internally to open a browser.
// Tests can swap this to prevent real browser windows.
var openBrowserFn = openBrowser

// hasDisplayFn checks for a graphical environment. Tests can override it.
var hasDisplayFn = hasDisplay

// hasDisplay reports whether the current environment has a graphical
// desktop capable of displaying a browser window.
// On Linux/FreeBSD it checks for X11 (DISPLAY) or Wayland (WAYLAND_DISPLAY).
// macOS and Windows are assumed to always have a desktop.
func hasDisplay() bool {
	return hasDisplayOn(runtime.GOOS, os.Getenv)
}

// hasDisplayOn is the platform-parameterized implementation of hasDisplay,
// extracted so every GOOS branch is testable on any host.
func hasDisplayOn(goos string, getenv func(string) string) bool {
	switch goos {
	case "linux", "freebsd", "openbsd", "netbsd":
		return getenv("DISPLAY") != "" || getenv("WAYLAND_DISPLAY") != ""
	default:
		return true
	}
}

// openBrowser opens the given URL in the user's default browser.
// Only called internally with http://127.0.0.1:<port> URLs.
func openBrowser(url string) error {
	cmd := browserCommand(context.Background(), runtime.GOOS, url)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	return nil
}

// browserCommand builds the platform-specific command that opens a URL in the
// default browser. Extracted from openBrowser so every GOOS branch is
// testable on any host.
func browserCommand(ctx context.Context, goos, url string) *exec.Cmd {
	switch goos {
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url) // #nosec G204 -- trusted internal URL
	case "darwin":
		return exec.CommandContext(ctx, "open", url) // #nosec G204 -- trusted internal URL
	default: // linux, freebsd, etc.
		return exec.CommandContext(ctx, "xdg-open", url) // #nosec G204 -- trusted internal URL
	}
}
