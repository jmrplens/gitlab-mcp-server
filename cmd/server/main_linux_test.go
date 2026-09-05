//go:build linux

package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
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
// credentials are configured, so main prints what the program is and what it
// needs on stderr, waits for a line, and returns without starting a server.
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
	for _, want := range []string{"GITLAB_URL", "GITLAB_TOKEN", "Press Enter to close."} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr = %q, want it to carry %q", stderr.String(), want)
			}
		})
	}
}
