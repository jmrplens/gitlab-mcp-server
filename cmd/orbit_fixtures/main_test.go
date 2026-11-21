// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// TestRunMain_Subcommands_DispatchOrRefuse checks what the command line
// decides before any request: no subcommand or an unknown one is a usage
// error, help, -h, -help and --help print the usage, a bad setup flag or a stray argument is a
// usage error, setup -h prints its flags, a timeout that is not a whole
// number of seconds refuses the wait, and so does any flag or argument after
// wait-indexer but -h, which prints the usage instead of waiting.
func TestRunMain_Subcommands_DispatchOrRefuse(t *testing.T) {
	noEnv := func(string) string { return "" }
	deadURL := netfixture.DeadEndpoint(t)
	deadInstance := func(key string) string {
		if key == "ORBIT_FIXTURES_GITLAB_URL" {
			return deadURL
		}
		return ""
	}
	pollEvery := func(seconds string) func(string) string {
		return func(key string) string {
			return map[string]string{"GITLAB_COM_TOKEN": standInToken, "ORBIT_FIXTURES_MERGE_POLL_SECONDS": seconds, "ORBIT_FIXTURES_GITLAB_URL": deadURL}[key]
		}
	}
	waitable := func(key string) string {
		return map[string]string{"GITLAB_COM_TOKEN": standInToken, "ORBIT_FIXTURES_INDEXER_TIMEOUT": "0", "ORBIT_FIXTURES_GITLAB_URL": deadURL}[key]
	}
	cases := []struct {
		name       string
		args       []string
		env        func(string) string
		want       int
		wantStdout string
		wantStderr string
	}{
		{name: "no subcommand", args: nil, want: 2, wantStderr: "usage: orbit_fixtures setup"},
		{name: "an unknown subcommand", args: []string{"provision"}, want: 2, wantStderr: `unknown subcommand "provision"`},
		{name: "help", args: []string{"help"}, want: 0, wantStdout: "usage: orbit_fixtures setup"},
		{name: "-h", args: []string{"-h"}, want: 0, wantStdout: "-root is the repository root"},
		{name: "-help", args: []string{"-help"}, want: 0, wantStdout: "usage: orbit_fixtures setup"},
		{name: "--help", args: []string{"--help"}, want: 0, wantStdout: "usage: orbit_fixtures setup"},
		{name: "an unknown setup flag", args: []string{"setup", "-nope"}, want: 2, wantStderr: "flag provided but not defined: -nope"},
		{name: "a stray setup argument", args: []string{"setup", "acme"}, want: 2, wantStderr: `unknown argument "acme"`},
		{name: "setup help", args: []string{"setup", "-h"}, want: 0, wantStderr: "-mirror-cli"},
		// Without a token, nothing is sent: the instance is one nothing
		// answers, so a run that went on anyway fails fast and says why.
		{name: "setup without a token", args: []string{"setup"}, env: deadInstance, want: 1, wantStderr: "GITLAB_COM_TOKEN is required"},
		// A poll interval that is not a number of seconds, zero or more, is a
		// usage error before anything is sent, never a wait of another length.
		{name: "a poll interval that is not a number", args: []string{"setup"}, env: pollEvery("soon"), want: 2, wantStderr: `setup: ORBIT_FIXTURES_MERGE_POLL_SECONDS="soon" is not a number of seconds, zero or more`},
		{name: "a negative poll interval", args: []string{"setup"}, env: pollEvery("-1"), want: 2, wantStderr: `ORBIT_FIXTURES_MERGE_POLL_SECONDS="-1" is not a number of seconds`},
		{name: "a poll interval that is no number", args: []string{"setup"}, env: pollEvery("NaN"), want: 2, wantStderr: `ORBIT_FIXTURES_MERGE_POLL_SECONDS="NaN" is not a number of seconds`},
		{name: "a poll interval past what a duration holds", args: []string{"setup"}, env: pollEvery("1e10"), want: 2, wantStderr: `ORBIT_FIXTURES_MERGE_POLL_SECONDS="1e10" is not a number of seconds`},
		{
			name: "a timeout that is not a number", args: []string{"wait-indexer"}, want: 2, wantStderr: `ORBIT_FIXTURES_INDEXER_TIMEOUT="soon" is not a whole number of seconds`,
			env: func(key string) string {
				if key == "ORBIT_FIXTURES_INDEXER_TIMEOUT" {
					return "soon"
				}
				return ""
			},
		},
		{
			name: "a negative timeout", args: []string{"wait-indexer"}, want: 2, wantStderr: "is not a whole number of seconds",
			env: func(key string) string {
				if key == "ORBIT_FIXTURES_INDEXER_TIMEOUT" {
					return "-15"
				}
				return ""
			},
		},
		// A timeout of zero is a whole number of seconds, which allows no
		// read: a wait that went on without a token would end at once.
		{
			name: "a wait without a token", args: []string{"wait-indexer"}, want: 1, wantStderr: "GITLAB_COM_TOKEN is required",
			env: func(key string) string {
				if key == "ORBIT_FIXTURES_INDEXER_TIMEOUT" {
					return "0"
				}
				return deadInstance(key)
			},
		},
		// The wait takes no flag and no argument. With a token and a timeout
		// that allows no read, a wait that went on anyway would end at once
		// without the usage and with another exit code.
		{name: "an unknown wait-indexer flag", args: []string{"wait-indexer", "-timeout", "5"}, env: waitable, want: 2, wantStderr: "flag provided but not defined: -timeout"},
		{name: "a stray wait-indexer argument", args: []string{"wait-indexer", "acme"}, env: waitable, want: 2, wantStderr: `wait-indexer: unknown argument "acme"`},
		{name: "wait-indexer help", args: []string{"wait-indexer", "--help"}, env: waitable, want: 0, wantStderr: "usage: orbit_fixtures setup"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.env
			if env == nil {
				env = noEnv
			}
			var stdout, stderr bytes.Buffer
			if got := runMain(t.Context(), tc.args, env, &stdout, &stderr); got != tc.want {
				t.Errorf("exit code = %d, want %d; stderr:\n%s", got, tc.want, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

// TestSetupFromArgs_Environment_FillsTheSettingsOrTheDefaults checks where
// each setting comes from: the environment when it is set, the default
// otherwise, the mirror only with -mirror-cli and never with -skip-mirror,
// and an instance URL without its trailing slash.
func TestSetupFromArgs_Environment_FillsTheSettingsOrTheDefaults(t *testing.T) {
	var stderr bytes.Buffer
	cfg, _, ok := setupFromArgs(nil, func(string) string { return "" }, &stderr)
	if !ok {
		t.Fatalf("no flags refused: %s", stderr.String())
	}
	if cfg.namespace != defaultNamespace || cfg.gitlabURL != defaultGitLabURL || cfg.visibility != "public" || cfg.mirror {
		t.Errorf("defaults = %q %q %q mirror %t", cfg.namespace, cfg.gitlabURL, cfg.visibility, cfg.mirror)
	}
	if cfg.api.root != "https://gitlab.com/api/v4" || cfg.mergePolls != 60 || cfg.mergeInterval != 2*time.Second {
		t.Errorf("api root %q, merge wait %d x %s", cfg.api.root, cfg.mergePolls, cfg.mergeInterval)
	}

	env := map[string]string{
		"GITLAB_COM_TOKEN":          "glpat-x",
		"ORBIT_FIXTURES_NAMESPACE":  "acme/fixtures",
		"ORBIT_FIXTURES_GITLAB_URL": "https://gitlab.example.test/",
		"ORBIT_FIXTURES_VISIBILITY": "private",
	}
	cfg, _, ok = setupFromArgs([]string{"--mirror-cli", "-root", "/repo"}, func(key string) string { return env[key] }, &stderr)
	if !ok {
		t.Fatalf("flags refused: %s", stderr.String())
	}
	if cfg.token != "glpat-x" || cfg.namespace != "acme/fixtures" || cfg.gitlabURL != "https://gitlab.example.test" || cfg.visibility != "private" || !cfg.mirror {
		t.Errorf("settings = %+v", cfg)
	}
	if cfg.fixtures != filepath.Join("/repo", "test", "fixtures", "orbit") {
		t.Errorf("fixtures = %q", cfg.fixtures)
	}
	cfg, _, _ = setupFromArgs([]string{"--mirror-cli", "--skip-mirror"}, func(string) string { return "" }, &stderr)
	if cfg.mirror {
		t.Error("-skip-mirror left the mirror on")
	}
}

// TestSetupFromArgs_MergePollSeconds_SetsTheWaitBetweenReads checks the wait
// between two reads of the fixture merge request while GitLab checks it comes
// from ORBIT_FIXTURES_MERGE_POLL_SECONDS, which scripts/setup-orbit-fixtures.sh
// documented and the port first dropped, read as the script's sleep read it:
// two seconds when unset, a fraction of one, and zero for no wait at all, the
// way a suite against a stand-in that answers at once keeps from waiting. The
// number of reads stays 60 whatever the interval.
func TestSetupFromArgs_MergePollSeconds_SetsTheWaitBetweenReads(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "unset", value: "", want: 2 * time.Second},
		{name: "whole seconds", value: "3", want: 3 * time.Second},
		{name: "a fraction", value: "0.5", want: 500 * time.Millisecond},
		{name: "zero", value: "0", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			env := map[string]string{"ORBIT_FIXTURES_MERGE_POLL_SECONDS": tc.value}
			cfg, _, ok := setupFromArgs(nil, func(key string) string { return env[key] }, &stderr)
			if !ok {
				t.Fatalf("refused: %s", stderr.String())
			}
			if cfg.mergeInterval != tc.want || cfg.mergePolls != 60 {
				t.Errorf("merge wait %d x %s, want 60 x %s", cfg.mergePolls, cfg.mergeInterval, tc.want)
			}
		})
	}
}

// TestMain_HandsTheExitCodeToTheProcess checks main passes what runMain
// decided to the process, through the seam over os.Exit.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousExit, previousArgs := exitProcess, os.Args
	t.Cleanup(func() { exitProcess, os.Args = previousExit, previousArgs })
	code := -1
	exitProcess = func(c int) { code = c }
	os.Args = []string{toolName, "provision"}
	main()
	if code != 2 {
		t.Errorf("main exit = %d, want 2 for an unknown subcommand", code)
	}
}

// TestSleepContext_EndsOnTheTimerOrTheContext checks the wait between two
// reads ends when its time is up, and sooner with the context's error when
// the run is cancelled.
func TestSleepContext_EndsOnTheTimerOrTheContext(t *testing.T) {
	if err := sleepContext(t.Context(), time.Millisecond); err != nil {
		t.Errorf("a finished wait = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sleepContext(ctx, time.Hour); err == nil {
		t.Error("a cancelled wait returned nil")
	}
}

// TestFirstNonEmpty_TakesTheFirstThatIsSet checks the precedence helper.
func TestFirstNonEmpty_TakesTheFirstThatIsSet(t *testing.T) {
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Errorf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty of nothing = %q", got)
	}
}

// TestHTTPClient_EveryRequest_HasADeadline checks the client a run sends
// every request with gives each two minutes, so a GitLab that accepts a
// connection and never answers ends the run.
func TestHTTPClient_EveryRequest_HasADeadline(t *testing.T) {
	if got := httpClient().Timeout; got != 2*time.Minute {
		t.Errorf("timeout = %s, want 2m0s", got)
	}
}

// TestHTTPClient_ARedirect_IsAnsweredAsItIs checks a request GitLab answers
// with a redirect reports that answer and is not sent again where the
// redirect points, as the script's curl did not follow one: net/http would
// send the PRIVATE-TOKEN header to any host the redirect named, and turn a
// write redirected with a 301 or 302 into a GET.
func TestHTTPClient_ARedirect_IsAnsweredAsItIs(t *testing.T) {
	var followed atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(elsewhere.Close)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/api/v4/projects/1", http.StatusFound)
	}))
	t.Cleanup(redirecting.Close)
	api := gitlabAPI{root: redirecting.URL + "/api/v4", token: "glpat-secret", client: httpClient()}

	got := api.do(t.Context(), http.MethodPut, "/projects/1", map[string]string{"visibility": "public"})

	if got.status != http.StatusFound || got.err != nil {
		t.Errorf("answer = %d %v, want the 302 itself", got.status, got.err)
	}
	if n := followed.Load(); n != 0 {
		t.Errorf("the redirect was followed %d times, with the token", n)
	}
}
