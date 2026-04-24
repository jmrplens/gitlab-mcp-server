// browser.go opens the system default browser to the wizard web UI URL,
// enabling the graphical setup flow.

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
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	default:
		return true
	}
}

// openBrowser opens the given URL in the user's default browser.
// Only called internally with http://127.0.0.1:<port> URLs.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	ctx := context.Background()

	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url) // #nosec G204 -- trusted internal URL
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url) // #nosec G204 -- trusted internal URL
	default: // linux, freebsd, etc.
		cmd = exec.CommandContext(ctx, "xdg-open", url) // #nosec G204 -- trusted internal URL
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	return nil
}
