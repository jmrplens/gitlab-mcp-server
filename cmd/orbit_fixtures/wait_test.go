// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// goldenWait is what the indexer wait of make orbit-wait-indexer did in one
// scenario, recorded from the recipe itself (its shell loop and the python3
// that read each answer) before the port replaced it: the timeout, the
// answers GitLab gave in order, every request the recipe sent, and what it
// wrote.
type goldenWait struct {
	Scenario string   `json:"scenario"`
	Timeout  string   `json:"timeout"`
	Answers  []string `json:"answers"`
	Requests []struct {
		URL     string   `json:"url"`
		Headers []string `json:"headers"`
		Body    string   `json:"body"`
	} `json:"requests"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Status int    `json:"status"`
}

// TestRunWait_NoAnswerOrACancelledWait_IsReadAsSuchOrEndsTheWait checks a
// read that gets no answer is reported as unread and read again, an instance
// URL that is no URL reads the same, and a wait cancelled between two reads
// ends the run with the reason.
func TestRunWait_NoAnswerOrACancelledWait_IsReadAsSuchOrEndsTheWait(t *testing.T) {
	cases := []struct {
		name       string
		gitlabURL  string
		sleepErr   error
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "an address nothing answers", gitlabURL: netfixture.DeadEndpoint(t), wantStdout: "[1/2] state=? indexed=0 not_indexed=0\n  [2/2] state=?", wantStderr: "did not catch up within 30s"},
		{name: "a URL that is none", gitlabURL: "http://bad host", wantStdout: "[1/2] state=?"},
		{name: "a wait cancelled", gitlabURL: netfixture.DeadEndpoint(t), sleepErr: context.Canceled, wantCode: 1, wantStderr: "wait-indexer: context canceled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := waitConfig{
				token: "x", gitlabURL: tc.gitlabURL, namespace: "plens1", timeoutSeconds: 30,
				client: http.DefaultClient,
				sleep:  func(context.Context, time.Duration) error { return tc.sleepErr },
			}
			var stdout, stderr bytes.Buffer
			if code := runWait(t.Context(), cfg, &stdout, &stderr); code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) || !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stdout %q, stderr %q; want %q and %q", stdout.String(), stderr.String(), tc.wantStdout, tc.wantStderr)
			}
		})
	}
}

// seenRequest is one request the stand-in GitLab received.
type seenRequest struct {
	path, authorization, contentType, body string
}

// TestWait_RecordedScenarios_DoWhatTheRecipeDid holds the port to what the
// recipe did, scenario by scenario: the same GraphQL request as many times,
// with the same headers and the same body (as a value), the same lines on
// stdout and stderr, and the same exit status. The one line that differs on
// purpose is the warning after a timeout, which the recipe wrote with an em
// dash and the port with a colon.
func TestWait_RecordedScenarios_DoWhatTheRecipeDid(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "wait", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no wait goldens under testdata/wait: %v", err)
	}
	for _, path := range paths {
		raw, readErr := os.ReadFile(path) //#nosec G304 -- a golden of this package's testdata
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		var golden goldenWait
		if jsonErr := json.Unmarshal(raw, &golden); jsonErr != nil {
			t.Fatalf("decoding %s: %v", path, jsonErr)
		}
		t.Run(golden.Scenario, func(t *testing.T) {
			server, seen := answerInOrder(t, golden.Answers)
			env := map[string]string{
				"GITLAB_COM_TOKEN":               "stand-in-token",
				"ORBIT_FIXTURES_NAMESPACE":       "plens1",
				"ORBIT_FIXTURES_GITLAB_URL":      server.URL,
				"ORBIT_FIXTURES_INDEXER_TIMEOUT": golden.Timeout,
			}
			cfg, cfgErr := waitFromEnv(func(key string) string { return env[key] })
			if cfgErr != nil {
				t.Fatalf("waitFromEnv: %v", cfgErr)
			}
			cfg.client = server.Client()
			cfg.sleep = func(_ context.Context, d time.Duration) error {
				if d != 15*time.Second {
					t.Errorf("waited %s between two reads, want 15s", d)
				}
				return nil
			}
			var stdout, stderr bytes.Buffer

			code := runWait(t.Context(), cfg, &stdout, &stderr)

			if code != golden.Status {
				t.Errorf("exit code = %d, want %d", code, golden.Status)
			}
			if stdout.String() != golden.Stdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), golden.Stdout)
			}
			if want := strings.ReplaceAll(golden.Stderr, " "+string(rune(0x2014))+" ", ": "); stderr.String() != want {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
			checkWaitRequests(t, seen(), golden)
		})
	}
}

// answerInOrder starts a stand-in GitLab that answers the nth request with
// the nth answer, and nothing past the last, and returns it with what it has
// received so far.
func answerInOrder(t *testing.T, answers []string) (*httptest.Server, func() []seenRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []seenRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, seenRequest{path: r.URL.Path, authorization: r.Header.Get("Authorization"), contentType: r.Header.Get("Content-Type"), body: string(body)})
		n := len(seen)
		mu.Unlock()
		if n <= len(answers) {
			_, _ = io.WriteString(w, answers[n-1])
		}
	}))
	t.Cleanup(server.Close)
	return server, func() []seenRequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

// checkWaitRequests holds the requests a wait sent to the golden's: as many,
// each to the same path with the same headers and the same body as a value.
func checkWaitRequests(t *testing.T, seen []seenRequest, golden goldenWait) {
	t.Helper()
	if len(seen) != len(golden.Requests) {
		t.Fatalf("%d requests sent, want %d", len(seen), len(golden.Requests))
	}
	for i, want := range golden.Requests {
		got := seen[i]
		if got.path != strings.TrimPrefix(want.URL, "{base}") {
			t.Errorf("request %d went to %s, want %s", i+1, got.path, want.URL)
		}
		headers := []string{"Authorization: " + got.authorization, "Content-Type: " + got.contentType}
		if !reflect.DeepEqual(headers, want.Headers) {
			t.Errorf("request %d carried %q, want %q", i+1, headers, want.Headers)
		}
		var gotBody, wantBody any
		if json.Unmarshal([]byte(got.body), &gotBody) != nil || json.Unmarshal([]byte(want.Body), &wantBody) != nil || !reflect.DeepEqual(gotBody, wantBody) {
			t.Errorf("request %d sent %s, want %s", i+1, got.body, want.Body)
		}
	}
}
