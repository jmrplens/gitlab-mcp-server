package wizard

import "os"

// IsInteractiveTerminal reports whether stdin is connected to an interactive
// terminal (character device) rather than a pipe or file.
func IsInteractiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
