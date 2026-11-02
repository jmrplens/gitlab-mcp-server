// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// fetch_test.go covers the requests the package check sends: retried as
// curl --retry 2 retried them, failed on an answer of 400 or above as curl
// -f failed them, redirects followed only where curl -L followed them, each
// failure described where curl -S described it, and every download written
// to a file of its own as curl -o wrote it.
package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
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

// fetched is what one bounded fetch returned.
type fetched struct {
	got response
	err error
}

// fetchWithin sends a request and fails the test when the fetcher has not
// given up within ten seconds, so a fetcher that never stops retrying is a
// failure rather than a test that never ends.
func fetchWithin(t *testing.T, f *fetcher, req request) (response, error) {
	t.Helper()
	done := make(chan fetched, 1)
	go func() {
		got, err := f.fetch(req)
		done <- fetched{got: got, err: err}
	}()
	select {
	case result := <-done:
		return result.got, result.err
	case <-time.After(10 * time.Second):
		t.Errorf("%s %s was still being retried after ten seconds", req.method, req.url)
		return response{}, errNotAnswered
	}
}

// downloaded is what one bounded download returned.
type downloaded struct {
	got *artifact
	err error
}

// downloadWithin downloads a URL and fails the test when the fetcher has not
// given up within ten seconds, as fetchWithin does for a request.
func downloadWithin(t *testing.T, f *fetcher, url string) (*artifact, error) {
	t.Helper()
	done := make(chan downloaded, 1)
	go func() {
		got, err := f.download(url)
		done <- downloaded{got: got, err: err}
	}()
	select {
	case result := <-done:
		return result.got, result.err
	case <-time.After(10 * time.Second):
		t.Errorf("GET %s was still being retried after ten seconds", url)
		return nil, errNotAnswered
	}
}

// artifactBytes reads what a download holds.
func artifactBytes(t *testing.T, a *artifact) []byte {
	t.Helper()
	data, err := io.ReadAll(a.reader())
	if err != nil {
		t.Fatalf("reading the download: %v", err)
	}
	return data
}

// spooled is how many files a download left in its directory.
func spooled(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listing the download directory: %v", err)
	}
	return len(entries)
}

// answer is one answer of a sequencedServer: its status and body, and
// whether it is cut short, announcing more bytes than it sends.
type answer struct {
	status int
	body   string
	cut    bool
}

// sequencedServer answers each request with the next of answers, the last
// one from then on, and counts what it was sent.
func sequencedServer(t *testing.T, answers ...answer) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		next := answers[min(int(sent.Add(1))-1, len(answers)-1)]
		if next.cut {
			w.Header().Set("Content-Length", strconv.Itoa(len(next.body)+10))
		}
		w.WriteHeader(next.status)
		_, _ = w.Write([]byte(next.body))
	}))
	t.Cleanup(server.Close)
	return server, &sent
}

// TestFetcher_Downloads_AreWrittenToAFileOfTheirOwn covers what a download
// holds: the bytes of the answer that passed, written to a file of its own in
// the fetcher's directory rather than held in memory, with their size and
// SHA-256, and no file left once it is closed or has failed.
//
// The retried cases are what writing every attempt from the file's start
// has to get right: an attempt before the one that passed may have written
// more bytes than it, a failure's body or an answer cut short, and none of
// them may be read as the artifact's.
func TestFetcher_Downloads_AreWrittenToAFileOfTheirOwn(t *testing.T) {
	t.Parallel()

	const bundle = "the bundle's bytes"
	longer := strings.Repeat("a failure longer than the bundle ", 4)
	for _, tc := range []struct {
		name    string
		answers []answer
		sent    int32
		ok      bool
	}{
		{name: "an answer", answers: []answer{{status: http.StatusOK, body: bundle}}, sent: 1, ok: true},
		{name: "a longer failure, then the answer", answers: []answer{{status: http.StatusServiceUnavailable, body: longer}, {status: http.StatusOK, body: bundle}}, sent: 2, ok: true},
		{name: "a longer answer cut short, then the answer", answers: []answer{{status: http.StatusOK, body: longer, cut: true}, {status: http.StatusOK, body: bundle}}, sent: 2, ok: true},
		{name: "a refusal", answers: []answer{{status: http.StatusNotFound, body: longer}}, sent: 1},
		{name: "an answer cut short every time", answers: []answer{{status: http.StatusOK, body: longer, cut: true}}, sent: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server, sent := sequencedServer(t, tc.answers...)
			var diagnostics bytes.Buffer
			f := quickFetcher(&diagnostics)
			f.spoolDir = t.TempDir()
			got, err := downloadWithin(t, f, server.URL+"/bundle.mcpb")
			if (err == nil) != tc.ok || sent.Load() != tc.sent {
				t.Fatalf("err %v after %d requests, want ok=%v after %d", err, sent.Load(), tc.ok, tc.sent)
			}
			if !tc.ok {
				if got != nil || spooled(t, f.spoolDir) != 0 {
					t.Errorf("a failed download returned %v and left %d files, want nothing", got, spooled(t, f.spoolDir))
				}
				return
			}
			if data := artifactBytes(t, got); string(data) != bundle || got.size != int64(len(bundle)) || got.digest != releaseassets.Digest([]byte(bundle)) {
				t.Errorf("download holds %q, size %d, digest %s, want %q, %d and %s", data, got.size, got.digest, bundle, len(bundle), releaseassets.Digest([]byte(bundle)))
			}
			if dir := filepath.Dir(got.file.Name()); dir != f.spoolDir || spooled(t, f.spoolDir) != 1 {
				t.Errorf("download written to %s beside %d files, want one file in %s", dir, spooled(t, f.spoolDir), f.spoolDir)
			}
			got.close()
			if left := spooled(t, f.spoolDir); left != 0 {
				t.Errorf("closing the download left %d files, want none", left)
			}
		})
	}
}

// TestFetcher_ADownloadWithNowhereToGo_IsDescribed covers a download whose
// file cannot be made: nothing is sent, and the reason is described where a
// failed request is.
func TestFetcher_ADownloadWithNowhereToGo_IsDescribed(t *testing.T) {
	t.Parallel()

	server, sent := countingServer(t, http.StatusOK)
	var diagnostics bytes.Buffer
	f := quickFetcher(&diagnostics)
	f.spoolDir = filepath.Join(t.TempDir(), "missing")
	got, err := downloadWithin(t, f, server.URL+"/x")
	if want := "  GET " + server.URL + "/x: open " + f.spoolDir; err == nil || got != nil || sent.Load() != 0 || !strings.HasPrefix(diagnostics.String(), want) {
		t.Errorf("download = %v, %v after %d requests, diagnostics %q, want a failure before any request, described as %q", got, err, sent.Load(), diagnostics.String(), want)
	}
}

// TestBounds_AreCurlsBounds holds the bounds written as numbers to the
// seconds curl was given and to the two retries.
func TestBounds_AreCurlsBounds(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ got, want time.Duration }{
		"connect":  {got: connectTimeout, want: 10 * time.Second},
		"metadata": {got: metadataTimeout, want: 60 * time.Second},
		"download": {got: downloadTimeout, want: 300 * time.Second},
		"retries":  {got: fetchRetries, want: 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.got != tc.want {
				t.Errorf("%s = %v, want %v", name, tc.got, tc.want)
			}
		})
	}
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
		{name: "a redirect, which is an answer", statuses: []int{399}, sent: 1, ok: true},
		{name: "a transient failure, then an answer", statuses: []int{503, 502, 200}, sent: 3, ok: true},
		// A fourth request would be answered, so a fetcher that retried once
		// more than twice would pass where this one fails.
		{name: "a transient failure every time", statuses: []int{429, 429, 429, 200}, sent: 3},
		{name: "a refusal", statuses: []int{404}, sent: 1},
		{name: "the lowest refusal", statuses: []int{400}, sent: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server, sent := countingServer(t, tc.statuses...)
			var diagnostics bytes.Buffer
			got, err := fetchWithin(t, quickFetcher(&diagnostics), request{method: http.MethodGet, url: server.URL + "/x", timeout: 5 * time.Second})
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
	f.spoolDir = t.TempDir()
	followed, err := downloadWithin(t, f, server.URL+"/from")
	if err != nil {
		t.Fatalf("download = %v, want the redirect followed", err)
	}
	t.Cleanup(followed.close)
	if got := artifactBytes(t, followed); string(got) != "moved here" {
		t.Errorf("download = %q, want the redirect followed", got)
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
			if _, err := fetchWithin(t, quickFetcher(&diagnostics), tc.req); err == nil || !strings.Contains(diagnostics.String(), tc.wantErr) {
				t.Errorf("err %v and diagnostics %q, want a failure naming %q", err, diagnostics.String(), tc.wantErr)
			}
		})
	}
}
