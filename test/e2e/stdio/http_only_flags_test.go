//go:build stdioe2e

// http_only_flags_test.go drives a stdio server given flags only HTTP mode
// reads, against the real binary: it names them on stderr and ignores them,
// quietly under --transport=auto, and refuses to start at all when one of them
// asks it to hold back writes (issue 1045).
package stdioe2e

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// The three lines cmd/server writes about HTTP-only flags on a stdio run,
// spelled here because this module cannot import package main.
const (
	stdioIgnoredFlagsLine = "these flags are read in HTTP mode only, and a stdio server takes its configuration " +
		"from the environment, so they have no effect"
	stdioAutoIgnoredFlagsLine = "--transport=auto chose stdio, so the flags given for HTTP mode have no effect on this run"
	stdioWithheldWritesLine   = "a flag asking to hold back writes was passed to a stdio server, which reads it in HTTP mode " +
		"only and would serve the writes, so this deployment will not be started under a capability it did not ask for"
)

// TestHTTPOnlyFlags_OnStdio_AreNamedAndIgnored is the case issue 1045 opened
// with: a stdio user who passes --rate-limit-rps to switch the limiter on.
//
// Both halves are held against the process. The flags are named on stderr at
// WARN, each with the variable stdio reads instead where there is one; and
// they are still ignored, which is what makes the line true: a bucket of one
// refilled once every thousand seconds would refuse the second of three tool
// calls, so all three being served, with the limiter's own startup line
// absent, is the flag having done nothing.
func TestHTTPOnlyFlags_OnStdio_AreNamedAndIgnored(t *testing.T) {
	s := startSessionWithArgs(t, baseEnv(startFakeGitLab(t).URL),
		"--rate-limit-rps=0.001", "--rate-limit-burst=1", "--http-addr=127.0.0.1:1")

	for i := 1; i <= 3; i++ {
		if got := s.call(t, currentUserCall(i)); !served(got) {
			t.Fatalf("call %d was not served; the ignored --rate-limit-rps would have refused it had it been read: %v", i, got)
		}
	}

	warned := awaitLogRecord(t, s, stdioIgnoredFlagsLine, 10*time.Second)
	if level, _ := warned["level"].(string); level != "WARN" {
		t.Errorf("the line naming the ignored flags is at %s, want WARN on a stdio run the operator chose", level)
	}
	want := []any{
		"--http-addr (stdio has no such setting)",
		"--rate-limit-burst (on stdio set GITLAB_MCP_RATE_LIMIT_BURST)",
		"--rate-limit-rps (on stdio set GITLAB_MCP_RATE_LIMIT_RPS)",
	}
	if got, _ := warned["flags"].([]any); !reflect.DeepEqual(got, want) {
		t.Errorf("flags named = %v, want %v", got, want)
	}

	logs := s.waitForStderr(t, stdioStartedLine, 5*time.Second)
	if strings.Contains(logs, rateLimitEnabledLine) {
		t.Errorf("stdio attached the limiter from --rate-limit-rps, which it reads in HTTP mode only:\n%s", logs)
	}
}

// TestHTTPOnlyFlags_UnderTransportAuto_AreNamedAtInfo starts the binary with
// the container image's own command, --transport auto --http-addr
// 0.0.0.0:8080, over the pipe every MCP client connects, which is what
// `docker run -i` of the published image does.
//
// That operator wrote one command line for either transport, so the listener
// flag is expected there rather than a mistake. A WARN on every stdio session
// of the image would teach the reader of that log to skip warnings; the line
// is written at INFO instead, and never missing.
func TestHTTPOnlyFlags_UnderTransportAuto_AreNamedAtInfo(t *testing.T) {
	s := startSessionWithArgs(t, baseEnv(startFakeGitLab(t).URL),
		"--transport", "auto", "--http-addr", "0.0.0.0:8080")

	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("the tool call was not served: %v", got)
	}

	informed := awaitLogRecord(t, s, stdioAutoIgnoredFlagsLine, 10*time.Second)
	if level, _ := informed["level"].(string); level != "INFO" {
		t.Errorf("the line naming the ignored flags under --transport=auto is at %s, want INFO", level)
	}
	if got, want := informed["flags"], []any{"--http-addr (stdio has no such setting)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("flags named = %v, want %v", got, want)
	}
	if logs := s.stderrText(); strings.Contains(logs, stdioIgnoredFlagsLine) {
		t.Errorf("the run also warned, which --transport=auto is exempt from:\n%s", logs)
	}
}

// TestHTTPOnlyFlags_ReadOnlyOnStdio_RefusesToStart pins the one case where a
// line is not enough. --read-only is read in HTTP mode only, so a stdio server
// started with it would serve every write the operator asked it to withhold,
// and the stdio servers most likely to carry it are the ones an MCP client
// started with nobody reading their stderr. It exits non-zero before serving,
// naming the flag and the variable to set instead, and writes nothing on
// stdout for a client to mistake for a server.
func TestHTTPOnlyFlags_ReadOnlyOnStdio_RefusesToStart(t *testing.T) {
	s := startSessionWithArgs(t, baseEnv(startFakeGitLab(t).URL), "--read-only")

	code, exited := s.waitExit(t, 20*time.Second)
	if !exited {
		t.Fatalf("a stdio server given --read-only started instead of refusing\nstderr: %s", s.stderrText())
	}
	if code == 0 {
		t.Errorf("the refusal exited 0\nstderr: %s", s.stderrText())
	}

	refused := awaitLogRecord(t, s, stdioWithheldWritesLine, 5*time.Second)
	for key, want := range map[string]string{
		"level":       "ERROR",
		"flag":        "--read-only",
		"set_instead": "GITLAB_MCP_READ_ONLY=true",
	} {
		t.Run(key, func(t *testing.T) {
			if got, _ := refused[key].(string); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		})
	}

	if logs := s.stderrText(); strings.Contains(logs, stdioStartedLine) {
		t.Errorf("the refused run started serving:\n%s", logs)
	}
	if b, err := s.stdout.ReadByte(); err == nil {
		t.Errorf("the refused run wrote %q on stdout, which only JSON-RPC may use", b)
	}
}
