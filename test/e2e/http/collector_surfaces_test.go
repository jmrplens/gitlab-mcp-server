//go:build httpe2e

package httpe2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCollectorSurfaces_EachOneRecordsWhatIdentifiesACall covers the three tool
// surfaces over the wire, because the tool name means something different on
// each and only one of them makes it the operation.
//
//	dynamic     two tool names cover ~1000 operations; the action is everything
//	meta        the tool is a bare domain; the action completes it
//	individual  the tool IS the operation, declared per ActionSpec
//
// Unit tests already cover the resolver for all three. What they cannot cover
// is whether the surface a running server registered is the one the resolver
// was built for: those are configured in different places and wired together
// once, which is exactly the kind of seam that works in a test and not in a
// binary.
func TestCollectorSurfaces_EachOneRecordsWhatIdentifiesACall(t *testing.T) {
	tests := []struct {
		surface string
		body    string
		headers map[string]string
		// wants is the set of substrings that must appear in some exported
		// payload for a call on this surface to be identifiable.
		wants []string
	}{
		{
			surface: "dynamic",
			body:    `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` + protocolMeta + `,"name":"gitlab_execute_action","arguments":{"action":"issue.list","project_id":"a/b"}}}`,
			headers: withAction("issue.list"),
			wants:   []string{"gitlab_execute_action", "issue.list", "gitlab_mcp.action"},
		},
		{
			surface: "meta",
			body:    `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` + protocolMeta + `,"name":"gitlab_issue","arguments":{"action":"list","project_id":"a/b"}}}`,
			headers: withAction("list"),
			wants:   []string{"gitlab_issue", "issue.list", "gitlab_mcp.action"},
		},
		{
			surface: "individual",
			body:    `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` + protocolMeta + `,"name":"gitlab_issue_list","arguments":{"project_id":"a/b"}}}`,
			wants:   []string{"gitlab_issue_list", "issue.list", "gitlab_mcp.action"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.surface, func(t *testing.T) {
			gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
			c := startCollector(t)
			env := collectorEnv(c)
			env["TOOL_SURFACE"] = tc.surface
			// The tool name is dropped from metrics on the individual surface
			// by default, and this asserts it on the SPAN, so the policy is
			// pinned on to keep the two questions separate.
			env["GITLAB_MCP_TELEMETRY_TOOL_NAME"] = "on"
			srv := startServer(t, env, "--gitlab-url="+gitlab.url)

			for range 3 {
				srv.do(t, authorizedCall(tc.body, tc.headers))
			}

			c.awaitExport(t, 20*time.Second)
			time.Sleep(700 * time.Millisecond)

			var payloads strings.Builder
			for _, e := range c.received() {
				payloads.Write(e.body)
			}
			exported := payloads.String()

			for _, want := range tc.wants {
				if !strings.Contains(exported, want) {
					t.Errorf("%q never reached the collector on the %s surface; a call there cannot be identified",
						want, tc.surface)
				}
			}
		})
	}
}

// TestCollectorSurfaces_IndividualKeepsTheToolNameOnSpans is the half of the
// cardinality decision this level can actually see.
//
// The tool name is dropped from metric DIMENSIONS on the individual surface,
// where up to 1071 values would exhaust the SDK's per-instrument limit and
// collapse the long tail into an overflow bucket. It stays on spans, because
// one span carrying it costs one span while one metric dimension costs a series
// per tool, forever. Losing it from both would satisfy the cardinality goal and
// quietly destroy the information a trace exists for, so this asserts the half
// that is easy to break while fixing the other.
//
// # Why the dimension itself is asserted in a unit test and not here
//
// It cannot be seen from a raw payload. With the View active the tool name is
// absent from every data point's attribute set and still present in the
// exported bytes, because an exemplar records the attributes that were filtered
// out. Measured: setting OTEL_METRICS_EXEMPLAR_FILTER=always_off makes it
// disappear from the metrics payload entirely.
//
// That is not a hole in the cardinality argument, since an exemplar reservoir
// is bounded per data point and mints no series. It is a hole in any attempt to
// use a View to keep a value away from a collector, which this project does not
// do: privacy filtering here is done by never putting the value in an attribute
// at all. TestToolNameView_RemovesOnlyTheToolName asserts the dimension through
// a ManualReader, where the attribute set is visible.
func TestCollectorSurfaces_IndividualKeepsTheToolNameOnSpans(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	c := startCollector(t)
	env := collectorEnv(c)
	env["TOOL_SURFACE"] = "individual"
	srv := startServer(t, env, "--gitlab-url="+gitlab.url)

	for range 3 {
		srv.do(t, authorizedCall(
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+protocolMeta+`,"name":"gitlab_issue_list","arguments":{"project_id":"a/b"}}}`, nil,
		))
	}

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	var inTraces bool
	for _, e := range c.received() {
		if e.path == "/v1/traces" && strings.Contains(string(e.body), "gitlab_issue_list") {
			inTraces = true
		}
	}
	if !inTraces {
		t.Error("the tool name is absent from spans on the individual surface; a trace can no longer say what was called")
	}
}

// TestCollectorSurfaces_OAuthModeStillRecords covers the authentication mode a
// published deployment actually runs.
//
// The instrumentation sits inside the credential check, and OAuth replaces that
// check with a different one: a bearer guard, a token cache and a verifier that
// legacy mode does not use. Whether a span still comes out the other side is a
// property of that wiring rather than of the middleware, so it needs asserting
// against a server started the way a hosted endpoint starts.
//
// A refusal is the case worth asserting here, because it is the one an OAuth
// deployment sees most from the open internet, and because it exercises the
// path where the MCP handler is never reached.
func TestCollectorSurfaces_OAuthModeStillRecords(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	c := startCollector(t)

	flags := []string{
		"--auth-mode=oauth",
		"--gitlab-url=" + gitlab.url,
		"--public-url=" + publicURL,
	}
	srv := startServer(t, collectorEnv(c), flags...)

	for range 3 {
		got := srv.do(t, request{
			body:    `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
			headers: map[string]string{"Authorization": "Bearer not-a-real-token"},
		})
		if got.status == http.StatusOK {
			t.Skip("the token was accepted; this test needs a rejection")
		}
	}

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	var sawTraces bool
	for _, e := range c.received() {
		if e.path == "/v1/traces" {
			sawTraces = true
		}
	}
	if !sawTraces {
		t.Error("an OAuth-mode refusal produced no span; the HTTP instrumentation is not reached in this mode")
	}

	// And the credential a client sent must not be in any of it, which matters
	// more in OAuth mode than anywhere else: the token is in a header the
	// instrumentation sits beside.
	c.assertNoPayloadContains(t, "not-a-real-token")
}
