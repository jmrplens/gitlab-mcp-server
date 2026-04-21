package wizard

import (
	"fmt"
	"io"
	"os"
)

// UIMode represents the wizard UI mode.
type UIMode string

const (
	UIModeAuto UIMode = "auto" // try web → TUI → CLI
	UIModeWeb  UIMode = "web"
	UIModeTUI  UIMode = "tui"
	UIModeCLI  UIMode = "cli"
)

// Run executes the setup wizard using the selected UI mode.
// In "auto" mode it cascades: Web UI → Bubble Tea TUI → plain CLI.
func Run(version string, mode UIMode, r io.Reader, w io.Writer) error {
	switch mode {
	case UIModeWeb:
		return RunWebUI(version, w)
	case UIModeTUI:
		return RunTUI(version, w)
	case UIModeCLI:
		return RunCLI(version, r, w)
	case UIModeAuto:
		return runCascade(version, r, w)
	default:
		return fmt.Errorf("unknown UI mode: %s", mode)
	}
}

func runCascade(version string, r io.Reader, w io.Writer) error {
	// Try Web UI first — only when a graphical display is available.
	// On headless servers (no DISPLAY/WAYLAND_DISPLAY) xdg-open silently
	// fails after cmd.Start() returns nil, leaving RunWebUI blocked forever.
	if hasDisplayFn() {
		if err := RunWebUI(version, w); err == nil {
			return nil
		}
		fmt.Fprintln(w, "  Web UI unavailable, falling back to terminal UI...")
		fmt.Fprintln(w)
	}

	// Try Bubble Tea TUI — requires a real terminal
	if IsInteractiveTerminal() {
		if err := RunTUI(version, w); err == nil {
			return nil
		}
		fmt.Fprintln(w, "  TUI unavailable, falling back to plain CLI...")
		fmt.Fprintln(w)
	}

	// Final fallback: plain CLI
	if r == nil {
		r = os.Stdin
	}
	return RunCLI(version, r, w)
}
