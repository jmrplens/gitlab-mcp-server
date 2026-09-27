// Command peer is a stand-in for a running gitlab-mcp-server process, built by
// the --shutdown tests under the name they expect to find and then left to
// block until somebody stops it.
//
// It exists because copying a system binary did not survive contact with a
// second platform. The tests used to copy /bin/sleep for a peer that stops on
// SIGTERM and /bin/sh for one that ignores it; on macOS the shell copy started
// and never became visible under its new name, so the test saw no peers at all.
// Diagnosing what a given platform does with a copy of its own shell is not
// what those tests are for, and a program built from this repository behaves
// the same everywhere.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ignoreTermEnv makes the peer ignore SIGTERM, which is the process an operator
// is really trying to clear when a server is wedged: --shutdown must escalate
// to a kill rather than wait forever.
const ignoreTermEnv = "PEER_IGNORE_SIGTERM"

// lifetime is longer than any test that starts this process, so it is always
// the test that ends it and never a timeout that ends the test.
const lifetime = 5 * time.Minute

// readyLine is what the peer writes to stdout once its signal disposition is
// in place, so a test about to signal it knows the signal meets the
// disposition it asked for.
const readyLine = "ready"

func main() {
	if os.Getenv(ignoreTermEnv) == "1" {
		signal.Ignore(syscall.SIGTERM)
	}
	// Written only after the disposition above is set. A test that signaled
	// the peer as soon as the process table showed it could reach it before
	// signal.Ignore ran, and a peer meant to ignore SIGTERM then died of it,
	// which let the wedged case pass without ever reaching the kill.
	fmt.Println(readyLine)
	time.Sleep(lifetime)
}
