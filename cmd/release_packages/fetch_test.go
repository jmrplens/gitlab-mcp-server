// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// fetch_test.go covers the requests the package check sends: retried as
// curl --retry 2 retried them, failed on an answer of 400 or above as curl
// -f failed them, redirects followed only where curl -L followed them, and
// each failure described where curl -S described it.
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// countingServer answers each request with the next status of statuses,
// the last one from then on, and counts what it was sent.
func countingServer(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(sent.Add(1)) - 1
		w.WriteHeader(statuses[min(n, len(statuses)-1)])
		_, _ = w.Write([]byte("answer"))
	}))
	t.Cleanup(server.Close)
	return server, &sent
}

// quickFetcher is a fetcher over the network that does not wait to retry.
func quickFetcher(diagnostics *bytes.Buffer) *fetcher {
	f := newFetcher(diagnostics)
	f.retryDelay = 0
	return f
}

// TestFetcher_Answers_AreRetriedAndJudgedAsCurlJudgedThem covers each kind of
// answer: one that passes at once, a transient one that passes on a retry,
// one that never passes after two retries, and one that is not retried.
func TestFetcher_Answers_AreRetriedAndJudgedAsCurlJudgedThem(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		statuses []int
		sent     int32
		ok       bool
	}{
		{name: "an answer", statuses: []int{200}, sent: 1, ok: true},
		{name: "a transient failure, then an answer", statuses: []int{503, 502, 200}, sent: 3, ok: true},
		{name: "a transient failure every time", statuses: []int{429}, sent: 3},
		{name: "a refusal", statuses: []int{404}, sent: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server, sent := countingServer(t, tc.statuses...)
			var diagnostics bytes.Buffer
			got, err := quickFetcher(&diagnostics).get(server.URL+"/x", false, nil)
			if (err == nil) != tc.ok || sent.Load() != tc.sent {
				t.Errorf("err %v after %d requests, want ok=%v after %d", err, sent.Load(), tc.ok, tc.sent)
			}
			if tc.ok && string(got.body) != "answer" {
				t.Errorf("body %q, want the answer", got.body)
			}
			if want := "  GET " + server.URL + "/x: the server answered "; !tc.ok && !strings.HasPrefix(diagnostics.String(), want) {
				t.Errorf("diagnostics %q, want them to open with %q", diagnostics.String(), want)
			}
		})
	}
}

// TestFetcher_Redirects_AreFollowedOnlyWhereCurlLFollowedThem covers a
// redirect: followed for a download, and handed back as the answer for a
// request without -L, which curl -f did not count as a failure.
func TestFetcher_Redirects_AreFollowedOnlyWhereCurlLFollowedThem(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/to", http.StatusFound) })
	mux.HandleFunc("/to", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("moved here")) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	var diagnostics bytes.Buffer
	f := quickFetcher(&diagnostics)
	followed, err := f.download(server.URL + "/from")
	if err != nil || string(followed) != "moved here" {
		t.Errorf("download = %q, %v, want the redirect followed", followed, err)
	}
	kept, err := f.get(server.URL+"/from", false, nil)
	if err != nil || strings.Contains(string(kept.body), "moved here") || kept.header.Get("Location") != "/to" {
		t.Errorf("get without follow = %q, %v, want the redirect itself", kept.body, err)
	}
}

// TestFetcher_RequestsThatCannotBeSent_AreDescribed covers a URL that is not
// one, an address nothing answers, which is retried, an answer cut short,
// and an answer slower than the request's bound.
func TestFetcher_RequestsThatCannotBeSent_AreDescribed(t *testing.T) {
	t.Parallel()

	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	t.Cleanup(cut.Close)
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); slow.Close() })

	for _, tc := range []struct {
		name    string
		req     request
		wantErr string
	}{
		{name: "a URL that is not one", req: request{method: http.MethodGet, url: "http://[::1", timeout: time.Second}, wantErr: "missing ']' in host"},
		{name: "an address nothing answers", req: request{method: http.MethodGet, url: netfixture.DeadEndpoint(t) + "/x", timeout: 5 * time.Second}, wantErr: "  GET "},
		{name: "an answer cut short", req: request{method: http.MethodGet, url: cut.URL, timeout: 5 * time.Second}, wantErr: "unexpected EOF"},
		{name: "an answer slower than its bound", req: request{method: http.MethodGet, url: slow.URL, timeout: 20 * time.Millisecond}, wantErr: "context deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var diagnostics bytes.Buffer
			if _, err := quickFetcher(&diagnostics).fetch(tc.req); err == nil || !strings.Contains(diagnostics.String(), tc.wantErr) {
				t.Errorf("err %v and diagnostics %q, want a failure naming %q", err, diagnostics.String(), tc.wantErr)
			}
		})
	}
}
