package main

import (
	"bytes"
	"log/slog"
	"testing"
	"time"
)

// TestInstallSlogBridge_DoesNotRecurse is the regression for a hang whose stack
// trace explains nothing until you know the trick.
//
// slog.SetDefault does two things, and the second is easy to miss: it also
// points the standard library's log package at the new default handler. So a
// bridge that wraps whatever slog.Default() currently holds is safe only while
// that is a handler somebody installed. When it is still slog's own built-in
// handler, which writes through log.Print, the chain closes on itself:
//
//	fanOutHandler.Handle -> slog.defaultHandler.Handle -> log.Logger.output
//	  -> slog.handlerWriter.Write -> fanOutHandler.Handle -> ...
//
// It presents as a process that never finishes and produces no output, which is
// how it went unnoticed until a package's tests went from a hundred seconds to
// a timeout. In this binary main installs a JSON handler first so the cycle
// never fires in production, and depending on that ordering is precisely the
// fragility this test exists to prevent.
//
// The assertion is the timeout rather than the content: infinite recursion has
// no wrong value to compare against, only a call that does not come back.
func TestInstallSlogBridge_DoesNotRecurse(t *testing.T) {
	var buf bytes.Buffer
	restoreBase := baseLogHandler
	baseLogHandler = slog.NewJSONHandler(&buf, nil)
	t.Cleanup(func() { baseLogHandler = restoreBase })

	restore := installSlogBridge()
	t.Cleanup(restore)

	done := make(chan struct{})
	go func() {
		slog.Info("a record that must return")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("logging through the bridge did not return; the handler is calling itself")
	}

	if buf.Len() == 0 {
		t.Error("the record never reached the base handler")
	}
}

// TestInstallSlogBridge_WithoutABaseHandlerIsANoOp covers the path a caller
// reaches by starting telemetry before the logger is configured.
//
// There is no known-safe handler to wrap at that point, and wrapping the
// built-in one is what recurses. Skipping costs the log export and keeps the
// process alive, which is the right trade for an ordering nothing should rely
// on. Returning a usable restore function matters too: the caller defers it
// unconditionally.
func TestInstallSlogBridge_WithoutABaseHandlerIsANoOp(t *testing.T) {
	restoreBase := baseLogHandler
	baseLogHandler = nil
	t.Cleanup(func() { baseLogHandler = restoreBase })

	before := slog.Default()
	restore := installSlogBridge()
	if restore == nil {
		t.Fatal("a nil restore function would panic in the deferred call that always runs")
	}
	if slog.Default() != before {
		t.Error("the default logger was replaced with no base handler to wrap")
	}
	restore()
}

// TestInstallSlogBridge_RestoresThePreviousLogger pins the symmetry.
//
// The default logger is a process global, so a bridge installed and never
// removed outlives the provider it writes into. In production that is a stopped
// exporter still receiving records during shutdown; in a test binary it is one
// test's telemetry configuration poisoning every later test's logging, which is
// how a package first started timing out.
func TestInstallSlogBridge_RestoresThePreviousLogger(t *testing.T) {
	var buf bytes.Buffer
	restoreBase := baseLogHandler
	baseLogHandler = slog.NewJSONHandler(&buf, nil)
	t.Cleanup(func() { baseLogHandler = restoreBase })

	before := slog.Default()
	restore := installSlogBridge()
	if slog.Default() == before {
		t.Fatal("the bridge did not replace the default logger, so this test proves nothing")
	}

	restore()
	if slog.Default() != before {
		t.Error("the previous logger was not restored")
	}
}
