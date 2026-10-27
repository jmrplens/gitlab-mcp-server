// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestHTTPFetch_AServer_AnswersItsStatusAndHeaders verifies the fetch the
// command runs against the site: a GET whose status and headers come back as
// sent, after a redirect has been followed, with the body read and closed, and
// a transport failure returned as an error.
func TestHTTPFetch_AServer_AnswersItsStatusAndHeaders(t *testing.T) {
	t.Parallel()
	var methods atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods.Store(r.Method)
		switch r.URL.Path {
		case "/moved":
			http.Redirect(w, r, "/page/", http.StatusMovedPermanently)
			return
		case "/drop":
			// The connection ends with no response, which the client reports
			// as a transport error.
			if conn, _, err := http.NewResponseController(w).Hijack(); err == nil {
				_ = conn.Close()
			}
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, strings.Repeat("x", 4096))
	}))
	t.Cleanup(server.Close)
	fetch := httpFetch(server.Client())

	status, header, err := fetch(t.Context(), server.URL+"/moved")
	if err != nil || status != http.StatusOK || header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("fetch = %d, %v, %v; want 200 with X-Frame-Options DENY", status, header, err)
	}
	if got, _ := methods.Load().(string); got != http.MethodGet {
		t.Errorf("the request method was %q, want GET", got)
	}

	if _, _, err = fetch(t.Context(), server.URL+"/drop"); err == nil {
		t.Error("fetch of a connection dropped without a response returned no error")
	}
	if _, _, err = fetch(t.Context(), "https://bad host/"); err == nil {
		t.Error("fetch of a URL no request can be built for returned no error")
	}
}

// TestNewHTTPClient_Settings_BoundEveryRequest verifies the client the command
// builds: one overall timeout, so a site that never answers cannot hold the
// deploy job until the runner's limit.
func TestNewHTTPClient_Settings_BoundEveryRequest(t *testing.T) {
	t.Parallel()
	if got := newHTTPClient().Timeout; got != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", got)
	}
}

// response is one scripted answer of a fake fetch.
type response struct {
	status int
	header http.Header
	err    error
}

// scriptedFetch answers each call with the next response, recording the URLs
// it was asked for, and fails the test when called once too often.
func scriptedFetch(t *testing.T, responses []response, asked *[]string) fetchFunc {
	t.Helper()
	next := 0
	return func(_ context.Context, url string) (int, http.Header, error) {
		*asked = append(*asked, url)
		if next == len(responses) {
			t.Errorf("fetch called %d times, scripted for %d", next+1, len(responses))
			return 0, nil, errors.New("unscripted")
		}
		answer := responses[next]
		next++
		return answer.status, answer.header, answer.err
	}
}

// TestFetchWithRetry_Answers_RetryWhatDeployPropagationExplains verifies the
// retry around a fetch: a transport error and a 5xx are retried after the
// delay until an answer arrives or the attempts run out, the last outcome is
// returned, and any other answer, a 404 included, is returned at once.
func TestFetchWithRetry_Answers_RetryWhatDeployPropagationExplains(t *testing.T) {
	t.Parallel()
	ok := http.Header{"X-Frame-Options": {"DENY"}}
	refused := errors.New("connection refused")
	cases := []struct {
		name       string
		responses  []response
		attempts   int
		wantStatus int
		wantHeader http.Header
		wantErr    error
		wantWaits  int
	}{
		{name: "an answer at once", attempts: 3, responses: []response{{status: 200, header: ok}}, wantStatus: 200, wantHeader: ok},
		{name: "a transport error then an answer", attempts: 3, responses: []response{{err: refused}, {status: 200, header: ok}}, wantStatus: 200, wantHeader: ok, wantWaits: 1},
		{name: "a 503 then an answer", attempts: 3, responses: []response{{status: 503}, {status: 200, header: ok}}, wantStatus: 200, wantHeader: ok, wantWaits: 1},
		{name: "a 500 then an answer", attempts: 3, responses: []response{{status: 500}, {status: 200, header: ok}}, wantStatus: 200, wantHeader: ok, wantWaits: 1},
		{name: "a 404 is not retried", attempts: 3, responses: []response{{status: 404, header: ok}}, wantStatus: 404, wantHeader: ok},
		{name: "a 499 is not retried", attempts: 3, responses: []response{{status: 499}}, wantStatus: 499},
		{name: "errors until the attempts run out", attempts: 2, responses: []response{{err: refused}, {err: refused}}, wantErr: refused, wantWaits: 1},
		{name: "a 500 on the last attempt", attempts: 2, responses: []response{{err: refused}, {status: 500}}, wantStatus: 500, wantWaits: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var asked []string
			var waits []time.Duration
			wait := func(d time.Duration) { waits = append(waits, d) }
			status, header, err := fetchWithRetry(t.Context(), scriptedFetch(t, tc.responses, &asked), "https://site.example/", tc.attempts, 7*time.Second, wait)
			if status != tc.wantStatus || !reflect.DeepEqual(header, tc.wantHeader) || !errors.Is(err, tc.wantErr) {
				t.Errorf("fetchWithRetry = %d, %v, %v; want %d, %v, %v", status, header, err, tc.wantStatus, tc.wantHeader, tc.wantErr)
			}
			if len(asked) != len(tc.responses) {
				t.Errorf("fetched %d times, want %d", len(asked), len(tc.responses))
			}
			if len(waits) != tc.wantWaits {
				t.Errorf("waited %d times, want %d", len(waits), tc.wantWaits)
			}
			for _, each := range waits {
				if each != 7*time.Second {
					t.Errorf("waited %v, want the 7s delay", each)
				}
			}
		})
	}
}
