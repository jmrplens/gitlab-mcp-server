//go:build stdioe2e

// http_only_flags_test.go drives a stdio server given flags only HTTP mode
// reads, against the real binary: it names them on stderr and ignores them,
// quietly under --transport=auto for the ones stdio has no setting for, and
// refuses to start at all when ignoring one would cost more than a setting
// (issue 1045).
package stdioe2e

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The four lines cmd/server writes about HTTP-only flags on a stdio run,
// spelled here because this module cannot import package main.
const (
	stdioIgnoredFlagsLine = "these flags are read in HTTP mode only, and a stdio server takes its configuration " +
		"from the environment, so they have no effect"
	stdioAutoIgnoredFlagsLine = "--transport=auto chose stdio, which has none of the settings these flags configure, " +
		"so they have no effect on this run"
	stdioWithheldLine = "a flag withholding part of what this server serves was passed to a stdio server, which reads " +
		"it in HTTP mode only, so the flag has no effect here and this deployment will not be started with it: set " +
		"the variable named instead and remove the flag"
	stdioInstanceLine = "--gitlab-url names no instance this stdio server connects to: stdio connects to the one " +
		"GITLAB_URL names, https://gitlab.com when it is unset, and sends GITLAB_TOKEN there, so this deployment " +
		"will not be started with the flag: set GITLAB_URL to the instance and remove the flag"
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

// TestHTTPOnlyFlags_UnderTransportAuto_ListenerFlagsAreNamedAtInfo starts the
// binary with the container image's own command, --transport auto --http-addr
// 0.0.0.0:8080, over the pipe every MCP client connects, which is what
// `docker run -i` of the published image does.
//
// That operator wrote one command line for either transport, and stdio has no
// listener, so the flag is expected there rather than a mistake. A WARN on
// every stdio session of the image would teach the reader of that log to skip
// warnings; the line is written at INFO instead, and never missing.
func TestHTTPOnlyFlags_UnderTransportAuto_ListenerFlagsAreNamedAtInfo(t *testing.T) {
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
		t.Errorf("the run also warned, which a listener flag under --transport=auto is exempt from:\n%s", logs)
	}
}

// TestHTTPOnlyFlags_UnderTransportAuto_SettingFlagsStillWarn adds a setting
// stdio has a variable for to the image's command. Written once for either
// transport, it was plausibly meant for both, and stdio ignores it, so it is
// named at WARN beside the listener's INFO line rather than folded into it.
func TestHTTPOnlyFlags_UnderTransportAuto_SettingFlagsStillWarn(t *testing.T) {
	s := startSessionWithArgs(t, baseEnv(startFakeGitLab(t).URL),
		"--transport", "auto", "--http-addr", "0.0.0.0:8080", "--tool-surface", "meta")

	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("the tool call was not served: %v", got)
	}

	informed := awaitLogRecord(t, s, stdioAutoIgnoredFlagsLine, 10*time.Second)
	if got, want := informed["flags"], []any{"--http-addr (stdio has no such setting)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("flags named at INFO = %v, want %v", got, want)
	}
	warned := awaitLogRecord(t, s, stdioIgnoredFlagsLine, 5*time.Second)
	if level, _ := warned["level"].(string); level != "WARN" {
		t.Errorf("the line naming --tool-surface under --transport=auto is at %s, want WARN", level)
	}
	if got, want := warned["flags"], []any{"--tool-surface (on stdio set GITLAB_MCP_TOOL_SURFACE)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("flags named at WARN = %v, want %v", got, want)
	}
}

// awaitRefusal waits for a refused stdio start to end and returns its refusal
// line, holding what every refusal shares: the process exits non-zero before
// serving, having written nothing on stdout for a client to mistake for a
// server.
func awaitRefusal(t *testing.T, s *session, line string) map[string]any {
	t.Helper()

	code, exited := s.waitExit(t, 20*time.Second)
	if !exited {
		t.Fatalf("a stdio server given a flag it cannot ignore started instead of refusing\nstderr: %s", s.stderrText())
	}
	if code == 0 {
		t.Errorf("the refusal exited 0\nstderr: %s", s.stderrText())
	}
	refused := awaitLogRecord(t, s, line, 5*time.Second)
	if level, _ := refused["level"].(string); level != "ERROR" {
		t.Errorf("the refusal is at %s, want ERROR", level)
	}
	if logs := s.stderrText(); strings.Contains(logs, stdioStartedLine) {
		t.Errorf("the refused run started serving:\n%s", logs)
	}
	if b, err := s.stdout.ReadByte(); err == nil {
		t.Errorf("the refused run wrote %q on stdout, which only JSON-RPC may use", b)
	}
	return refused
}

// TestHTTPOnlyFlags_WithholdingFlagOnStdio_RefusesToStart pins the cases where
// a line is not enough. --read-only, --safe-mode and --exclude-tools are read
// in HTTP mode only, so a stdio server started with one would serve what the
// operator asked it to withhold, and the stdio servers most likely to carry
// one are the ones an MCP client started with nobody reading their stderr. It
// exits non-zero before serving, naming the flag and the setting to write
// instead, and does so even when that variable already asks for the same.
func TestHTTPOnlyFlags_WithholdingFlagOnStdio_RefusesToStart(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		wantFlag string
		wantSet  string
	}{
		{name: "read-only", args: []string{"--read-only"}, wantFlag: "--read-only", wantSet: "GITLAB_MCP_READ_ONLY=true"},
		{
			name: "read-only beside the variable asking the same", args: []string{"--read-only"},
			env:      map[string]string{"GITLAB_MCP_READ_ONLY": "true"},
			wantFlag: "--read-only", wantSet: "GITLAB_MCP_READ_ONLY=true",
		},
		{
			name: "exclude-tools", args: []string{"--exclude-tools", "project.delete"},
			wantFlag: "--exclude-tools", wantSet: "GITLAB_MCP_EXCLUDE_TOOLS=project.delete",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			maps.Copy(env, tc.env)
			s := startSessionWithArgs(t, env, tc.args...)

			refused := awaitRefusal(t, s, stdioWithheldLine)
			for key, want := range map[string]string{"flag": tc.wantFlag, "set_instead": tc.wantSet} {
				t.Run(key, func(t *testing.T) {
					if got, _ := refused[key].(string); got != want {
						t.Errorf("%s = %q, want %q", key, got, want)
					}
				})
			}
		})
	}
}

// TestHTTPOnlyFlags_GitLabURLNamingAnotherInstance_RefusesBeforeTheTokenLeaves
// is the case the review of issue 1045 found: an MCP client configured with
// --gitlab-url for a corporate instance and a corporate token in its
// environment. stdio reads GITLAB_URL alone, and connects to https://gitlab.com
// when it is unset, so the token would go to an instance the command line
// never named, at startup, before any tool is called.
//
// The instance GITLAB_URL names here counts every request that reaches it,
// standing in for the one the token must not reach: the run is refused before
// a single request is made to it.
func TestHTTPOnlyFlags_GitLabURLNamingAnotherInstance_RefusesBeforeTheTokenLeaves(t *testing.T) {
	var reached atomic.Int32
	unnamed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(unnamed.Close)

	s := startSessionWithArgs(t, baseEnv(unnamed.URL), "--gitlab-url", "https://gitlab.corp.example.invalid")

	refused := awaitRefusal(t, s, stdioInstanceLine)
	for key, want := range map[string]string{"flag": "--gitlab-url", "set_instead": "GITLAB_URL"} {
		t.Run(key, func(t *testing.T) {
			if got, _ := refused[key].(string); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		})
	}
	if n := reached.Load(); n != 0 {
		t.Errorf("the instance GITLAB_URL names received %d requests carrying the token before the refusal, want 0", n)
	}
}

// TestHTTPOnlyFlags_GitLabURLNamingTheInstanceItConnectsTo_IsOnlyNamed is the
// other half: a --gitlab-url that names the instance GITLAB_URL does, spelled
// differently, sends the token nowhere it did not name, so it costs nothing
// when ignored and the run serves, naming the flag at WARN.
func TestHTTPOnlyFlags_GitLabURLNamingTheInstanceItConnectsTo_IsOnlyNamed(t *testing.T) {
	fake := startFakeGitLab(t)
	s := startSessionWithArgs(t, baseEnv(fake.URL), "--gitlab-url", fake.URL+"/")

	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("the tool call was not served: %v", got)
	}
	warned := awaitLogRecord(t, s, stdioIgnoredFlagsLine, 10*time.Second)
	if got, want := warned["flags"], []any{"--gitlab-url (on stdio set GITLAB_URL)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("flags named = %v, want %v", got, want)
	}
}
