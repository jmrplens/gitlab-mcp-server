// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

package httpe2e

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The discovery documents' refusal as cmd/server writes it (register row
// RTC-008), spelled here because this module cannot import package main.
const (
	discoveryRefusal     = "Too many requests for the discovery documents from this address. Retry later."
	discoveryRefusalCode = -42900
)

// discoveryHost is the host --public-url names in this file's deployments,
// which every request sends as its Host, as a proxy preserving it would.
const discoveryHost = "mcp.example.invalid"

// discoveryFetch sends one request to path in the name of caller, the address
// the trusted proxy names, with the deployment's public host.
func discoveryFetch(t *testing.T, srv *server, method, path, caller string, headers map[string]string) response {
	t.Helper()
	all := map[string]string{"Host": discoveryHost}
	if caller != "" {
		all["X-Forwarded-For"] = caller
	}
	maps.Copy(all, headers)
	return srv.do(t, request{method: method, path: path, headers: all})
}

// assertDiscoveryRefusal holds a response to the refusal RTC-008 declares: the
// gate's 429 with the register's Retry-After, a JSON-RPC error in 429's code
// and the register's words, readable from any origin.
func assertDiscoveryRefusal(t *testing.T, got response, what string) {
	t.Helper()
	if got.status != http.StatusTooManyRequests || got.header.Get("Retry-After") != "1" {
		t.Fatalf("%s = %d with Retry-After %q, want 429 and 1: %s", what, got.status, got.header.Get("Retry-After"), got.body)
	}
	var body struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(got.body), &body); err != nil {
		t.Fatalf("%s carried no JSON-RPC body: %v (%q)", what, err, got.body)
	}
	if body.Error.Code != discoveryRefusalCode || body.Error.Message != discoveryRefusal {
		t.Errorf("%s refused with %+v, want code %d and %q", what, body.Error, discoveryRefusalCode, discoveryRefusal)
	}
	if got.header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("%s carries Access-Control-Allow-Origin %q, want the documents' own *", what, got.header.Get("Access-Control-Allow-Origin"))
	}
}

// TestDiscoveryRate_EachDocumentDrawsOnItsAddressesBucket drives issue 1053's
// limit on the binary, in both auth modes and at every path a document is
// published at under a --public-url prefix: the SEP-2127 card at the root and
// after /mcp, the legacy card, and in oauth mode the RFC 9728 metadata. Each
// path is fetched by an address of its own from a bucket of two that does not
// refill within the test: the fetch is served, its revalidation is answered
// 304 and is the second request, and a CORS preflight is then refused with the
// register's 429. The preflight comes from the --public-url origin, which the
// binary trusts with no --trusted-origins and whose preflight its CORS layer
// answers with a 204 before the mux, so only a bucket outside that layer can
// refuse it. The spent address still reaches /health and the MCP endpoint,
// which the bucket does not charge.
func TestDiscoveryRate_EachDocumentDrawsOnItsAddressesBucket(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	cards := []string{
		"/server-card", "/gitlab/server-card", "/mcp/server-card", "/gitlab/mcp/server-card",
		"/.well-known/mcp/server-card.json", "/gitlab/.well-known/mcp/server-card.json",
	}
	for _, tc := range []struct {
		authMode string
		paths    []string
	}{
		{authMode: "legacy", paths: cards},
		{authMode: "oauth", paths: append(cards, "/.well-known/oauth-protected-resource/gitlab")},
	} {
		t.Run(tc.authMode, func(t *testing.T) {
			srv := startServer(t, nil,
				"--gitlab-url="+gitlab.url,
				"--auth-mode="+tc.authMode,
				"--public-url=https://"+discoveryHost+"/gitlab",
				"--trusted-proxy-header=X-Forwarded-For",
				"--trusted-proxies=127.0.0.1,::1",
				"--discovery-rate-limit-rps=0.001",
				"--discovery-rate-limit-burst=2",
			)
			for i, path := range tc.paths {
				t.Run(path, func(t *testing.T) {
					assertDocumentDrawsOnItsBucket(t, srv, path, "203.0.113."+strconv.Itoa(i+1))
				})
			}
		})
	}
}

// assertDocumentDrawsOnItsBucket spends caller's bucket of two on path, a fetch
// and its revalidation, and holds the preflight after them to the register's
// refusal, and /health and the MCP endpoint, which the bucket does not charge,
// to their own answers.
func assertDocumentDrawsOnItsBucket(t *testing.T, srv *server, path, caller string) {
	t.Helper()
	fetched := discoveryFetch(t, srv, http.MethodGet, path, caller, nil)
	if fetched.status != http.StatusOK || fetched.header.Get("ETag") == "" {
		t.Fatalf("GET %s = %d with ETag %q, want the document and its validator: %s",
			path, fetched.status, fetched.header.Get("ETag"), fetched.body)
	}
	revalidated := discoveryFetch(t, srv, http.MethodGet, path, caller, map[string]string{
		"If-None-Match": fetched.header.Get("ETag"),
	})
	if revalidated.status != http.StatusNotModified {
		t.Errorf("a revalidation of %s = %d, want 304 from the bucket's second request", path, revalidated.status)
	}
	preflight := discoveryFetch(t, srv, http.MethodOptions, path, caller, map[string]string{
		"Origin": "https://" + discoveryHost, "Access-Control-Request-Method": http.MethodGet,
	})
	assertDiscoveryRefusal(t, preflight, "a preflight of "+path+" after two requests")
	for _, other := range []string{"/health", "/gitlab/health"} {
		if got := discoveryFetch(t, srv, http.MethodGet, other, caller, nil); got.status != http.StatusOK {
			t.Errorf("GET %s from the spent address = %d, want 200", other, got.status)
		}
	}
	endpoint := srv.do(t, mcpPOST(map[string]string{"Host": discoveryHost, "X-Forwarded-For": caller}))
	if endpoint.status != http.StatusUnauthorized {
		t.Errorf("POST /mcp with no token from the spent address = %d, want the endpoint's own 401", endpoint.status)
	}
}

// TestDiscoveryRate_TheForwardedAddressIsBelievedOnlyFromATrustedProxy covers
// the key, which is the authentication budgets': two callers a trusted proxy
// names are two buckets, and the same two names written by a client nobody
// trusts are the one peer they arrived from.
func TestDiscoveryRate_TheForwardedAddressIsBelievedOnlyFromATrustedProxy(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	for _, tc := range []struct {
		name    string
		trusted bool
		want    int
	}{
		{name: "named by a trusted proxy", trusted: true, want: http.StatusOK},
		{name: "written by a client nobody trusts", want: http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := []string{
				"--gitlab-url=" + gitlab.url, "--public-url=https://" + discoveryHost,
				"--discovery-rate-limit-rps=0.001", "--discovery-rate-limit-burst=1",
			}
			if tc.trusted {
				flags = append(flags, "--trusted-proxy-header=X-Forwarded-For", "--trusted-proxies=127.0.0.1,::1")
			}
			srv := startServer(t, nil, flags...)
			if first := discoveryFetch(t, srv, http.MethodGet, "/server-card", "203.0.113.1", nil); first.status != http.StatusOK {
				t.Fatalf("the first caller's fetch = %d, want 200", first.status)
			}
			if second := discoveryFetch(t, srv, http.MethodGet, "/server-card", "203.0.113.2", nil); second.status != tc.want {
				t.Errorf("the second caller's fetch = %d, want %d", second.status, tc.want)
			}
		})
	}
}

// TestDiscoveryRate_AZeroTurnsItOff covers the switch issue 1053 asked for, in
// both spellings: a deployment whose reverse proxy limits these paths already
// sets the rate or the burst to zero, through the flag or the variable, and
// the binary then serves every request for a document. A variable naming a
// positive bucket is read too, so the zero is the setting's and not the
// variable going unread.
func TestDiscoveryRate_AZeroTurnsItOff(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	for _, tc := range []struct {
		name    string
		env     map[string]string
		flags   []string
		refused bool
	}{
		{name: "a zero rate flag", flags: []string{"--discovery-rate-limit-rps=0", "--discovery-rate-limit-burst=1"}},
		{name: "a zero burst flag", flags: []string{"--discovery-rate-limit-rps=1", "--discovery-rate-limit-burst=0"}},
		{
			name: "a zero rate variable",
			env:  map[string]string{"GITLAB_MCP_DISCOVERY_RATE_LIMIT_RPS": "0", "GITLAB_MCP_DISCOVERY_RATE_LIMIT_BURST": "1"},
		},
		{
			name:    "a positive bucket from the variables",
			env:     map[string]string{"GITLAB_MCP_DISCOVERY_RATE_LIMIT_RPS": "0.001", "GITLAB_MCP_DISCOVERY_RATE_LIMIT_BURST": "1"},
			refused: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := startServer(t, tc.env, append([]string{"--gitlab-url=" + gitlab.url, "--public-url=https://" + discoveryHost}, tc.flags...)...)
			statuses := make([]int, 0, 3)
			for range 3 {
				statuses = append(statuses, discoveryFetch(t, srv, http.MethodGet, "/server-card", "", nil).status)
			}
			want := []int{http.StatusOK, http.StatusOK, http.StatusOK}
			if tc.refused {
				want = []int{http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests}
			}
			if !slices.Equal(statuses, want) {
				t.Errorf("three fetches of /server-card = %v, want %v", statuses, want)
			}
		})
	}
}

// TestDiscoveryRate_IsOnByDefaultWithTheShippedBurst covers the default the
// decision put in place: a deployment that says nothing about the bucket has
// it, at twenty a second with two hundred in hand, which it announces at
// startup, so one address is served the burst and then refused.
func TestDiscoveryRate_IsOnByDefaultWithTheShippedBurst(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--public-url=https://"+discoveryHost)

	started := startupRecord(t, srv)
	if started["discovery_rate_limit_rps"] != float64(20) || started["discovery_rate_limit_burst"] != float64(200) {
		t.Errorf("startup announced rate %v and burst %v, want the shipped 20 and 200",
			started["discovery_rate_limit_rps"], started["discovery_rate_limit_burst"])
	}
	// The bucket refills twenty a second while the burst is spent, so the
	// first refusal arrives some requests past two hundred, and before four
	// hundred on any host that answers this card in under twenty-five
	// milliseconds on average, the race detector included.
	for i := 1; i <= 400; i++ {
		got := discoveryFetch(t, srv, http.MethodGet, "/server-card", "", nil)
		if got.status == http.StatusTooManyRequests {
			if i <= 200 {
				t.Fatalf("request %d was refused, inside the burst of 200", i)
			}
			assertDiscoveryRefusal(t, got, "the request past the burst")
			return
		}
		if got.status != http.StatusOK {
			t.Fatalf("request %d = %d, want 200 or the bucket's 429", i, got.status)
		}
	}
	t.Error("four hundred requests from one address were all served; the shipped bucket was not in force")
}

// startupRecord is the server's "starting MCP server in HTTP mode" log record.
func startupRecord(t *testing.T, srv *server) map[string]any {
	t.Helper()
	for line := range strings.SplitSeq(srv.logs(), "\n") {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) == nil && record["msg"] == "starting MCP server in HTTP mode" {
			return record
		}
	}
	t.Fatalf("the server logged no startup record:\n%s", srv.logs())
	return nil
}
