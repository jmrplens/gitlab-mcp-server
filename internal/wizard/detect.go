// Package wizard implements an interactive setup wizard that launches when
// the binary runs in a terminal (double-click, direct execution) rather than
// as an MCP server via stdio pipe.
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
