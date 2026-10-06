//go:build linux

package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// openPTY allocates a pseudo-terminal pair and returns both ends, the slave
// being what a person's shell would hand a program as its standard input.
func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal multiplexer here: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	if unlockErr := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); unlockErr != nil {
		t.Fatalf("unlocking the pty: %v", unlockErr)
	}
	number, err := unix.IoctlGetUint32(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatalf("reading the pty number: %v", err)
	}
	name := "/dev/pts/" + strconv.FormatUint(uint64(number), 10)
	slave, err = os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("the pty slave %s cannot be opened here: %v", name, err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave
}

// TestMain_StartedByHandWithoutCredentials_ExplainsAndWaits covers the screen
// shown to somebody who double-clicked the binary: stdin is a terminal and no
// credentials are configured, so main prints what the program is, which
// version it is and what it needs on stderr, waits for a line, and returns
// without starting a server.
//
// A pseudo-terminal is the only honest stdin for this, since the guard is a
// terminal check and a pipe is exactly what it must not match. The message
// goes to stderr because stdout is the protocol stream, so stderr is what is
// captured.
func TestMain_StartedByHandWithoutCredentials_ExplainsAndWaits(t *testing.T) {
	withFreshFlagSet(t)
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv(config.EnvFileVar, "")
	// The screen names the build it belongs to, and the commit is the value
	// that could stand in its place.
	withBuildIdentity(t, "9.8.7-guidance", "c0mm1t-guidance")

	master, slave := openPTY(t)
	originalStdin, originalArgs, originalLogger := os.Stdin, os.Args, slog.Default()
	os.Stdin = slave
	os.Args = []string{"gitlab-mcp-server"}
	t.Cleanup(func() {
		os.Stdin = originalStdin
		os.Args = originalArgs
		slog.SetDefault(originalLogger)
	})

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	var stderr bytes.Buffer
	var drained sync.WaitGroup
	drained.Go(func() { _, _ = io.Copy(&stderr, reader) })
	originalStderr := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = originalStderr })

	var exits []int
	originalExit := exitProcess
	exitProcess = func(code int) { exits = append(exits, code) }
	t.Cleanup(func() { exitProcess = originalExit })

	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
	}()

	// The line the screen waits for. Written to the terminal's master side,
	// where a person's Enter would come from; the tty buffers it if main has
	// not reached the read yet.
	if _, writeErr := master.WriteString("\n"); writeErr != nil {
		t.Fatalf("pressing Enter: %v", writeErr)
	}
	select {
	case <-done:
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("main did not return after Enter was pressed on the guidance screen")
	}
	os.Stderr = originalStderr
	_ = writer.Close()
	drained.Wait()

	if len(exits) != 0 {
		t.Errorf("exit codes = %v, want none: the screen returns rather than failing", exits)
	}
	if want := "gitlab-mcp-server 9.8.7-guidance\n"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want the screen to name the version, %q", stderr.String(), want)
	}
	for _, want := range []string{"GITLAB_URL", "GITLAB_TOKEN", "Press Enter to close."} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr = %q, want it to carry %q", stderr.String(), want)
			}
		})
	}
}

// TestMain_OnATerminal_TheGuidanceScreenAnswersEachMissingCredential covers
// each operand of the guard in front of the guidance screen, where the test
// above covers only the case that satisfies all of them at once.
//
// Either credential missing is enough for the screen, since supplying one
// without the other fails inside config.Load with a JSON line a
// double-clicked console closes over; and an HTTP deployment never gets it,
// since its credentials arrive per request. Every case also sets a retired
// variable that refuses startup, so a run that wrongly skips the screen ends
// at once with exit 1 instead of starting a server on the terminal.
func TestMain_OnATerminal_TheGuidanceScreenAnswersEachMissingCredential(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         []string
		token        string
		url          string
		wantGuidance bool
	}{
		{name: "a token without an instance", args: []string{"gitlab-mcp-server"}, token: "glpat-terminal", wantGuidance: true},
		{name: "an instance without a token", args: []string{"gitlab-mcp-server"}, url: "https://gitlab.example.test", wantGuidance: true},
		// Configured on a terminal is a person testing a working setup by hand,
		// not a first run, so the server starts (and here stops at once).
		{name: "both credentials", args: []string{"gitlab-mcp-server"}, token: "glpat-terminal", url: "https://gitlab.example.test"},
		{name: "an http deployment", args: []string{"gitlab-mcp-server", "-http"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withFreshFlagSet(t)
			t.Setenv("GITLAB_URL", tc.url)
			t.Setenv("GITLAB_TOKEN", tc.token)
			t.Setenv(config.EnvFileVar, "")
			t.Setenv("GITLAB_READ_ONLY", "true")

			stderr, exits := runMainOnATerminal(t, tc.args)

			shown := strings.Contains(stderr, "Press Enter to close.")
			if shown != tc.wantGuidance {
				t.Errorf("guidance shown = %t, want %t; stderr = %q", shown, tc.wantGuidance, stderr)
			}
			wantExits := []int{1}
			if tc.wantGuidance {
				wantExits = nil
			}
			if !slices.Equal(exits, wantExits) {
				t.Errorf("exit codes = %v, want %v", exits, wantExits)
			}
		})
	}
}

// runMainOnATerminal runs main with args and a pseudo-terminal as stdin,
// presses Enter on it, and returns what main wrote to stderr and the exit
// codes it asked for. The logger, the filesystem policy and the process
// globals main touches are restored when the test ends.
func runMainOnATerminal(t *testing.T, args []string) (stderr string, exits []int) {
	t.Helper()
	master, slave := openPTY(t)
	originalStdin, originalArgs, originalLogger, originalBase := os.Stdin, os.Args, slog.Default(), baseLogHandler
	os.Stdin = slave
	os.Args = args
	t.Cleanup(func() {
		os.Stdin = originalStdin
		os.Args = originalArgs
		slog.SetDefault(originalLogger)
		baseLogHandler = originalBase
		toolutil.SetLocalFilesystemAccess(true)
	})

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	var captured bytes.Buffer
	var drained sync.WaitGroup
	drained.Go(func() { _, _ = io.Copy(&captured, reader) })
	originalStderr := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = originalStderr })

	originalExit := exitProcess
	exitProcess = func(code int) { exits = append(exits, code) }
	t.Cleanup(func() { exitProcess = originalExit })

	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
	}()
	if _, writeErr := master.WriteString("\n"); writeErr != nil {
		t.Fatalf("pressing Enter: %v", writeErr)
	}
	select {
	case <-done:
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("main did not return on a terminal")
	}
	os.Stderr = originalStderr
	_ = writer.Close()
	drained.Wait()
	return captured.String(), exits
}
