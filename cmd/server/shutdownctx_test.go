package main

import (
	"context"
	"testing"
	"time"
)

// boundedShutdown returns a context a test can safely pass to Shutdown.
//
// Never context.Background(). The provider-level Shutdown honors only the
// caller's context: the SDK's own 30s default applies per export, not to the
// whole drain. So an unbounded context against a collector that is not there
// waits forever, and the failure presents as a test binary that never finishes
// rather than as an assertion anybody can read.
//
// This was not hypothetical. Adding the logs pipeline gave Shutdown something
// real to drain, and this package went from a hundred seconds to a timeout.
// Nothing had changed in those tests; they had simply been passing an unbounded
// context to a call that previously had nothing to wait for.
func boundedShutdown(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}
