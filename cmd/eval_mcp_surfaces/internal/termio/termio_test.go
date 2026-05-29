package termio

import (
	"errors"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestOutputWrite_WritesFileAndPropagatesErrors(t *testing.T) {
	var builder strings.Builder
	out := NewOutput(&builder, false)
	n, err := out.Write([]byte("hello"))
	if err != nil || n != 5 || builder.String() != "hello" {
		t.Fatalf("Write() = %d, %v, %q; want 5 nil hello", n, err, builder.String())
	}
	failing := NewOutput(failingWriter{}, false)
	_, failingErr := failing.Write([]byte("hello"))
	if failingErr == nil {
		t.Fatal("Write(failing writer) error = nil, want error")
	}
}

func TestPrintHelpers_WriteToConfiguredLog(t *testing.T) {
	var builder strings.Builder
	restore := SetOutputForTest(NewOutput(&builder, false))
	t.Cleanup(restore)
	Printf("hello %s", "there")
	Print("!")
	LogPrintf(" log=%d", 1)
	if got := builder.String(); got != "hello there! log=1" {
		t.Fatalf("terminal output = %q, want combined log", got)
	}
}

func TestShouldConfigure_RespectsQuietCheckModes(t *testing.T) {
	if !ShouldConfigure("", false, false, 0, 0) {
		t.Fatal("ShouldConfigure(default) = false, want true")
	}
	if ShouldConfigure("", false, true, 0, 0) {
		t.Fatal("ShouldConfigure(check docs) = true, want false")
	}
	if !ShouldConfigure("", true, true, 0, 0) {
		t.Fatal("ShouldConfigure(explicit print) = false, want true")
	}
}
