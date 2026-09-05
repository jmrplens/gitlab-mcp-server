package cmdutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	fatalStderr    io.Writer = os.Stderr
	progressStderr io.Writer = os.Stderr
	exitProcess              = os.Exit
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

// Must returns value, and panics when err is non-nil.
//
// It is for an operation that cannot fail where it is called and whose failure
// a caller could not act on anyway: connecting an in-memory MCP transport,
// building the action catalog compiled into this binary, marshaling a struct
// the program built a line earlier. Returning such an error adds a return path
// to every frame it crosses, none of which can ever execute, and every caller
// can only print it and stop, which is what this panic already does. It is the
// standard library's Must convention, regexp.MustCompile and template.Must,
// applied to this module's command utilities.
//
// Do not use it for a failure a caller can act on: a bad flag, a missing file,
// a generated artifact that has drifted. Those return an error so the command
// reports them cleanly and chooses its own exit code.
//
// The panic carries the error, so wrap the error where it is produced if it
// does not already say what was attempted. Go's call rules make a context
// argument impossible here: f(g()) compiles only when g's results are exactly
// f's parameters.
func Must[T any](value T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("cmdutil.Must: %v", err))
	}
	return value
}

// MustDo panics when err is non-nil. It is [Must] for an operation that
// returns no value, and carries the same contract: use it only where the
// failure cannot happen and a caller could not act on it.
func MustDo(err error) {
	if err != nil {
		panic(fmt.Sprintf("cmdutil.MustDo: %v", err))
	}
}

// Fatalf writes a formatted error message to stderr and exits with status 1.
func Fatalf(message string, args ...any) {
	fmt.Fprintf(fatalStderr, message+"\n", args...)
	exitProcess(1)
}

// Progressf writes a formatted progress line to stderr so long-running command
// utilities show activity instead of looking stale. It writes to stderr (never
// stdout) so it never pollutes JSON or generated output captured from stdout. A
// trailing newline is added automatically.
func Progressf(message string, args ...any) {
	fmt.Fprintf(progressStderr, message+"\n", args...)
}
