package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFirstRunGuidance_BlocksUntilALineArrives is the assertion the screen
// exists for.
//
// On Windows a double-clicked console program closes its window the instant it
// returns, so a message printed and returned from is a message nobody reads.
// The wait is the feature. This drives it with a pipe whose write end is held
// open, so the function is genuinely blocked rather than reaching an immediate
// EOF, and then supplies the line.
func TestFirstRunGuidance_BlocksUntilALineArrives(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	var out bytes.Buffer
	returned := make(chan struct{})
	go func() {
		firstRunGuidance(&out, reader, "2.7.5")
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("firstRunGuidance returned without waiting; a double-clicked window would close unread")
	case <-time.After(100 * time.Millisecond):
	}

	if _, writeErr := writer.WriteString("\n"); writeErr != nil {
		t.Fatalf("writing the line: %v", writeErr)
	}
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("firstRunGuidance did not return after a line arrived")
	}
	_ = writer.Close()
}

// TestFirstRunGuidance_ReturnsOnAClosedInput covers the other end of the same
// read. A terminal whose input is closed, or a read that errors, must let the
// process exit rather than hang: the guard above this function is what keeps a
// client out, and if it is ever widened by accident an unbounded block would be
// a hang with no diagnostic.
func TestFirstRunGuidance_ReturnsOnAClosedInput(t *testing.T) {
	returned := make(chan struct{})
	go func() {
		firstRunGuidance(&bytes.Buffer{}, strings.NewReader(""), "2.7.5")
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("firstRunGuidance hung on an input that was already at EOF")
	}
}

// TestFirstRunGuidance_NamesWhatItNeeds pins the content, because a screen that
// waits and then says nothing useful is worse than no screen.
//
// The two variable names and a working documentation link are the whole payload.
// The link assertion is deliberately about the path shape: the site has a flat
// structure with no guides/ segment, and every URL this project has published
// with one has 404'd.
func TestFirstRunGuidance_NamesWhatItNeeds(t *testing.T) {
	var out bytes.Buffer
	firstRunGuidance(&out, strings.NewReader("\n"), "2.7.5")
	got := out.String()

	for _, want := range []string{"GITLAB_URL", "GITLAB_TOKEN", "2.7.5", "--help"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("the screen never mentions %q", want)
			}
		})
	}
	if strings.Contains(got, "/guides/") {
		t.Errorf("the screen links into /guides/, a path the documentation site does not serve:\n%s", got)
	}
}

// TestIsInteractiveTerminal_IsFalseForAPipe is the invariant that separates a
// person from a client, and the one whose failure mode is worst.
//
// An MCP client connects pipes. If this ever returned true for one, the server
// would print a human-readable screen and block forever on a read, and the
// client would wait for an initialize response that never comes. There is no
// error, no log line and no exit: it presents as a hang.
func TestIsInteractiveTerminal_IsFalseForAPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	restore := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() { os.Stdin = restore })

	if isInteractiveTerminal() {
		t.Error("a pipe was reported as an interactive terminal; an MCP client would hang here")
	}
}

// TestIsInteractiveTerminal_IsFalseForAFile covers the other way stdin arrives
// without a person behind it: a redirect from a file, which is how a harness or
// a shell script drives the binary.
func TestIsInteractiveTerminal_IsFalseForAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the file: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })

	restore := os.Stdin
	os.Stdin = file
	t.Cleanup(func() { os.Stdin = restore })

	if isInteractiveTerminal() {
		t.Error("a regular file was reported as an interactive terminal")
	}
}

// TestExecutableName_FallsBackToTheProjectName pins the fallback rather than the
// happy path, because the happy path is whatever the test binary is called.
// The name is printed for the reader to type, so an empty string would leave
// the line unusable.
func TestExecutableName_FallsBackToTheProjectName(t *testing.T) {
	if executableName() == "" {
		t.Error("executableName returned an empty string; the --help line would be unusable")
	}
}

// TestExecutableName_WhenTheLookupFails_NamesTheProject covers the fallback
// itself, which the happy path above cannot reach on any platform where the
// test binary knows its own path. Both ways the lookup can come back empty
// yield the documented name rather than an empty command the reader cannot
// type.
func TestExecutableName_WhenTheLookupFails_NamesTheProject(t *testing.T) {
	original := osExecutable
	t.Cleanup(func() { osExecutable = original })

	cases := []struct {
		name   string
		lookup func() (string, error)
	}{
		{name: "the lookup errors", lookup: func() (string, error) { return "", errors.New("procfs is not mounted") }},
		{name: "the lookup answers an empty path", lookup: func() (string, error) { return "", nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			osExecutable = tc.lookup
			if got := executableName(); got != "gitlab-mcp-server" {
				t.Errorf("executableName() = %q, want the project name as the fallback", got)
			}
		})
	}
}
