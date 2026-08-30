//go:build httpe2e

package httpe2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCollectorHTTPSpan_ARefusalIsVisibleWhereTheMCPSpanIsNot is the reason the
// HTTP-level instrumentation exists at all.
//
// The MCP span starts after the credential check, so a refused request produces
// none, which is deliberate: anonymous traffic cannot drive an endpoint's
// telemetry volume. It also leaves the operator of a published endpoint unable
// to see the number they most need, which is how much traffic is being turned
// away. This asserts both halves at once: the HTTP span exists for a refusal,
// and no MCP method name appears alongside it.
func TestCollectorHTTPSpan_ARefusalIsVisibleWhereTheMCPSpanIsNot(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, collectorEnv(c))

	// No credential, so the request is refused before the MCP handler.
	for range 3 {
		got := srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
		if got.status == http.StatusOK {
			t.Skip("the request was not refused; this test needs a rejection to mean anything")
		}
	}

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	var sawTraces, sawMCPMethod bool
	for _, e := range c.received() {
		if e.path != "/v1/traces" {
			continue
		}
		sawTraces = true
		if strings.Contains(string(e.body), "mcp.method.name") {
			sawMCPMethod = true
		}
	}

	if !sawTraces {
		t.Error("a refused request produced no span at all; an operator cannot see how much traffic is being turned away")
	}
	if sawMCPMethod {
		t.Error("a refused request produced an MCP span; the instrumentation is outside the credential check and anonymous traffic can drive telemetry volume")
	}
}

// TestCollectorHTTPSpan_CarriesNoClientControlledValue pins the reason this
// middleware is written here rather than taken from contrib.
//
// otelhttp derives server.address from the Host header and client.address from
// X-Forwarded-For, neither of which is trustworthy on a published endpoint and
// both of which become metric dimensions. The convention says so about the
// first: "Since this attribute is based on HTTP headers, opting in to it may
// allow an attacker to trigger cardinality limits, degrading the usefulness of
// the metric."
//
// This sends values a caller controls and asserts none of them reach the
// collector. A path is included because recording url.path would be the obvious
// implementation and the one that lets a scanner mint a series per probe.
func TestCollectorHTTPSpan_CarriesNoClientControlledValue(t *testing.T) {
	const (
		spoofedHost = "attacker-chosen-host.example"
		spoofedIP   = "203.0.113.199"
		probePath   = "/wp-admin-probe-path"
	)

	c := startCollector(t)
	srv := startServer(t, collectorEnv(c))

	srv.do(t, request{
		method: http.MethodGet,
		path:   probePath,
		headers: map[string]string{
			"X-Forwarded-For":   spoofedIP,
			"X-Forwarded-Proto": "https",
		},
	})
	srv.do(t, request{
		body:    `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		headers: map[string]string{"Host": spoofedHost, "X-Forwarded-For": spoofedIP},
	})

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	c.assertNoPayloadContains(t, spoofedHost, spoofedIP, probePath)
}

// TestCollectorHTTPSpan_RecordsTheStatus is the other half: a span with nothing
// on it would satisfy the assertion above and answer no question.
//
// Method and status are what an HTTP-level view is for on this server, since it
// serves a handful of fixed paths and what was actually called is on the MCP
// span. Without the status there is no error rate, which is the number an
// operator watches.
func TestCollectorHTTPSpan_RecordsTheStatus(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, collectorEnv(c))

	for range 3 {
		srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
	}

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	var sawStatus, sawMethod, sawDuration bool
	for _, e := range c.received() {
		payload := string(e.body)
		if strings.Contains(payload, "http.response.status_code") {
			sawStatus = true
		}
		if strings.Contains(payload, "http.request.method") {
			sawMethod = true
		}
		if strings.Contains(payload, "http.server.request.duration") {
			sawDuration = true
		}
	}

	if !sawStatus {
		t.Error("no span carries http.response.status_code; there is no error rate to watch")
	}
	if !sawMethod {
		t.Error("no span carries http.request.method")
	}
	if !sawDuration {
		t.Error("the http.server.request.duration metric never arrived")
	}
}
