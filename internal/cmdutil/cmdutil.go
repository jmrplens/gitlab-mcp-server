// Package cmdutil provides shared helpers for repository command utilities.
package cmdutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	fatalStderr io.Writer = os.Stderr
	exitProcess           = os.Exit
)

// RepositoryRoot walks upward from start until it finds the module root.
func RepositoryRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found from %s", start)
		}
		current = parent
	}
}

// Fatalf writes a formatted error message to stderr and exits with status 1.
func Fatalf(message string, args ...any) {
	fmt.Fprintf(fatalStderr, message+"\n", args...)
	exitProcess(1)
}
