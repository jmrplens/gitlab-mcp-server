package wizard

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsInteractiveTerminal reports whether stdin is connected to an interactive
// terminal (character device) rather than a pipe or file.
func IsInteractiveTerminal() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}
