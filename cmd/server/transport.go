package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Transport selectors accepted by --transport.
const (
	transportStdio = "stdio"
	transportHTTP  = "http"
	transportAuto  = "auto"
)

// stdinStat is the stat of file descriptor 0, replaced in tests. It is a
// variable rather than a parameter because the thing under test is a property
// of the process, and threading it through main's signature would put a seam
// in production code that only tests would ever use.
var stdinStat = func() (os.FileInfo, error) { return os.Stdin.Stat() }

// devNullStat is the stat of the null device, replaced in tests alongside
// stdinStat so a case can make the two the same file or not.
var devNullStat = func() (os.FileInfo, error) { return os.Stat(os.DevNull) }

// resolveTransport turns the two transport selectors into the single boolean
// the rest of the program uses, and explains itself when it inferred rather
// than obeyed.
//
// --http stays exactly what it was, so every existing invocation keeps its
// meaning. --transport is the newer, three-valued spelling; when both are
// given, --transport wins and says so, because the operator who reached for
// the newer flag meant it.
//
// The reason this exists at all: the image's command has to serve HTTP, since
// that is what a container published on a port is for, and an MCP client that
// runs the same image with `docker run -i` and no arguments then gets an HTTP
// listener and waits at initialize forever. Every client configuration in the
// documentation therefore carries --http=false, which is a papercut copied
// into every one of them.
func resolveTransport(transport string, useHTTP bool, httpSet bool) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(transport)) {
	case "":
		return useHTTP, nil
	case transportStdio:
		if httpSet && useHTTP {
			slog.Warn("--transport=stdio overrides --http")
		}
		return false, nil
	case transportHTTP:
		if httpSet && !useHTTP {
			slog.Warn("--transport=http overrides --http=false")
		}
		return true, nil
	case transportAuto:
		if httpSet {
			slog.Warn("--transport=auto ignores --http; remove one of them to say plainly which transport you meant")
		}
		http, why := inferTransport()
		slog.Info("transport inferred from stdin", "transport", transportName(http), "reason", why)
		return http, nil
	default:
		return false, fmt.Errorf("--transport %q is not one of %s, %s, %s", transport, transportStdio, transportHTTP, transportAuto)
	}
}

// inferTransport reads the transport off file descriptor 0, and reports the
// observation it decided on so the choice is never silent.
//
// The question is not "is this a terminal", which cannot separate the two
// cases that matter: a terminal and /dev/null are both character devices. It
// is "did anybody hand this process a stdin at all". Docker answers that
// precisely. `docker run` without -i, and Compose without stdin_open, connect
// file descriptor 0 to /dev/null; `docker run -i`, which is what every MCP
// client configuration uses, connects a pipe. Nothing speaks JSON-RPC down
// /dev/null, so that is the one shape that means HTTP.
//
// Everything else means stdio: a pipe is a client, a terminal is a person
// trying it by hand, a regular file is a shell redirect replaying a session,
// and a socket is a supervisor. Choosing stdio for an unrecognized shape is
// also the safer error, because a stdio server that should have been HTTP is
// obvious within seconds, while an HTTP listener that should have been stdio
// is a client hanging with no output at all, which is the defect this closes.
func inferTransport() (http bool, reason string) {
	info, err := stdinStat()
	if err != nil {
		// Not a reason to refuse to start: a stdin that cannot be stat'ed is
		// not a stdin anybody is speaking to.
		return false, "stdin could not be examined (" + err.Error() + ")"
	}
	mode := info.Mode()
	switch {
	case mode&os.ModeNamedPipe != 0:
		return false, "stdin is a pipe"
	case mode.IsRegular():
		return false, "stdin is a regular file"
	}
	devNull, err := devNullStat()
	if err != nil {
		return false, "the null device could not be examined (" + err.Error() + ")"
	}
	if os.SameFile(info, devNull) {
		return true, "stdin is " + os.DevNull + ", so no client is speaking to this process"
	}
	return false, "stdin is " + mode.String()
}

// flagWasSet reports whether the operator typed the named flag, as opposed to
// it holding its default. A flag whose value happens to equal the default is
// still a choice, which is why this asks the flag set rather than comparing
// values.
func flagWasSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// transportName spells the resolved transport for a log line.
func transportName(http bool) string {
	if http {
		return transportHTTP
	}
	return transportStdio
}
