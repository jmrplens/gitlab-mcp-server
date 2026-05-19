package main

import (
	"errors"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// TestTerminalOutputWrite_WritesFileAndPropagatesErrors verifies terminal output
// mirrors command progress to files and reports write failures.
func TestTerminalOutputWrite_WritesFileAndPropagatesErrors(t *testing.T) {
	var b strings.Builder
	out := terminalOutput{file: &b}
	n, err := out.Write([]byte("hello"))
	if err != nil || n != 5 || b.String() != "hello" {
		t.Fatalf("Write() = %d, %v, %q; want 5 nil hello", n, err, b.String())
	}
	failing := terminalOutput{file: failingWriter{}}
	_, failingErr := failing.Write([]byte("hello"))
	if failingErr == nil {
		t.Fatal("Write(failing writer) error = nil, want error")
	}
}

// TestShouldConfigureTerminalOutput_RespectsQuietCheckModes verifies terminal log
// setup is skipped for pure check commands unless explicitly requested.
func TestShouldConfigureTerminalOutput_RespectsQuietCheckModes(t *testing.T) {
	if !shouldConfigureTerminalOutput(options{}) {
		t.Fatal("shouldConfigureTerminalOutput(default) = false, want true")
	}
	if shouldConfigureTerminalOutput(options{CheckDocs: true}) {
		t.Fatal("shouldConfigureTerminalOutput(check docs) = true, want false")
	}
	if !shouldConfigureTerminalOutput(options{CheckDocs: true, PrintOutput: true}) {
		t.Fatal("shouldConfigureTerminalOutput(explicit print) = false, want true")
	}
}

// TestTerminalPrintHelpers_WriteToConfiguredLog verifies package-level terminal
// helpers write into the configured command output sink.
func TestTerminalPrintHelpers_WriteToConfiguredLog(t *testing.T) {
	var b strings.Builder
	previous := commandOutput
	commandOutput = terminalOutput{file: &b}
	t.Cleanup(func() { commandOutput = previous })
	terminalPrintf("hello %s", "there")
	terminalPrint("!")
	terminalLogPrintf(" log=%d", 1)
	if got := b.String(); got != "hello there! log=1" {
		t.Fatalf("terminal output = %q, want combined log", got)
	}
}
