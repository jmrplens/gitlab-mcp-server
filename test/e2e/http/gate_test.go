//go:build httpe2e

// gate_test.go pins how the HTTP gate answers a request it will not serve:
// the status, the challenge, the JSON-RPC body, and what it costs upstream.
//
// The shape of a rejection is a contract. The SDK's own bearer middleware
// answers in plain text with a bare challenge, which conflates "authenticate",
// "ask for more scope" and "come back later" into one response a client cannot
// act on differently.
package httpe2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// jsonRPCError is the wire shape the gate emits for a transport-level
// rejection. Duplicated from cmd/server on purpose: a test that imported the
// server's own type would still pass if that type changed shape, which is the
// one thing this asserts.
type jsonRPCError struct {
	JSONRPC string `json:"jsonrpc"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeJSONRPCError parses the body and fails the test when it is not the
// documented shape.
func decodeJSONRPCError(t *testing.T, body string) jsonRPCError {
	t.Helper()
	var out jsonRPCError
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("rejection body is not JSON-RPC: %v\n%s", err, body)
	}
	if out.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want \"2.0\"", out.JSONRPC)
	}
	return out
}

// TestGate_MissingCredential_IsJSONRPCWithAChallenge verifies that an
// unauthenticated request is refused in the JSON-RPC shape the rest of the
// endpoint uses, with a WWW-Authenticate challenge and a message naming the
// headers this deployment accepts.
//
// The plain-text alternative matters for more than tidiness: a client that
// receives a body it cannot parse as JSON-RPC is told by the specification to
// conclude the server is initialization-era and downgrade, so an opaque
// rejection turns a missing header into a false protocol diagnosis.
func TestGate_MissingCredential_IsJSONRPCWithAChallenge(t *testing.T) {
	srv := startServer(t, nil, "--gitlab-url=https://gitlab.example.com")

	got := srv.do(t, mcpPOST(nil))

	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got.status, http.StatusUnauthorized)
	}
	if ct := got.header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	if challenge := got.header.Get("WWW-Authenticate"); !strings.HasPrefix(challenge, "Bearer ") {
		t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", challenge)
	}
	body := decodeJSONRPCError(t, got.body)
	if body.Error.Code != -40100 {
		t.Errorf("error code = %d, want -40100 (mirrors HTTP 401)", body.Error.Code)
	}
	if !strings.Contains(body.Error.Message, "PRIVATE-TOKEN") {
		t.Errorf("the message should name the headers legacy mode accepts, got %q", body.Error.Message)
	}
}

// TestGate_LegacyChallengeOmitsResourceMetadata verifies that legacy mode does
// not advertise an authorization server it does not have. Clients discover one
// through the resource_metadata parameter, and legacy mode mounts no RFC 9728
// endpoint, so including it would send a client into a discovery flow that
// cannot complete.
func TestGate_LegacyChallengeOmitsResourceMetadata(t *testing.T) {
	srv := startServer(t, nil, "--gitlab-url=https://gitlab.example.com")

	got := srv.do(t, mcpPOST(nil))

	if challenge := got.header.Get("WWW-Authenticate"); strings.Contains(challenge, "resource_metadata") {
		t.Errorf("legacy challenge advertises discovery it cannot serve: %q", challenge)
	}
}

// TestGate_MalformedGitLabURLHeader_IsRejectedWithDetail verifies that a
// client-supplied GITLAB-URL that cannot be parsed is a 400 naming the header,
// rather than something the caller has to guess at.
func TestGate_MalformedGitLabURLHeader_IsRejectedWithDetail(t *testing.T) {
	// No --gitlab-url, so the header is the only source and clients must send it.
	srv := startServer(t, nil)

	got := srv.do(t, mcpPOST(map[string]string{
		"PRIVATE-TOKEN": "glpat-whatever",
		"GITLAB-URL":    "://not a url",
	}))

	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got.status, http.StatusBadRequest)
	}
	body := decodeJSONRPCError(t, got.body)
	if !strings.Contains(strings.ToUpper(body.Error.Message), "GITLAB-URL") {
		t.Errorf("the message should name the offending header, got %q", body.Error.Message)
	}
}

// TestGate_RepeatedFailuresBlockTheAddress verifies the per-address failure
// budget: a stream of invented tokens is cut off with 429 and a Retry-After,
// rather than relayed upstream one for one.
//
// Without this a public deployment is an amplifier — unauthenticated traffic
// anyone can generate becomes load on someone else's API, charged to this
// server's address.
func TestGate_RepeatedFailuresBlockTheAddress(t *testing.T) {
	// A GitLab that answers 401 rather than one that is unreachable: only a
	// rejection is charged to the budget, because counting an outage would
	// lock out clients holding valid tokens.
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	var blocked response
	for i := range 15 {
		got := srv.do(t, mcpPOST(map[string]string{
			"PRIVATE-TOKEN": "glpat-invented-" + strconv.Itoa(i),
		}))
		if got.status == http.StatusTooManyRequests {
			blocked = got
			break
		}
	}

	if blocked.status != http.StatusTooManyRequests {
		t.Fatal("a stream of invalid tokens was never rate limited")
	}
	if blocked.header.Get("Retry-After") == "" {
		t.Error("a 429 must tell the caller when to come back")
	}
	body := decodeJSONRPCError(t, blocked.body)
	if body.Error.Code != -42900 {
		t.Errorf("error code = %d, want -42900 (mirrors HTTP 429)", body.Error.Code)
	}
}

// TestGate_FailureBudgetIsPerAddress verifies that the budget is charged to the
// caller, not to the deployment as a whole.
//
// Behind a reverse proxy every request arrives from the proxy, so without
// --trusted-proxy-header one noisy client would lock out everyone. Testing this
// through a proxy proves nothing — every address is the same — so the headers
// are forged directly against the binary, which is the only way to observe the
// two buckets separately.
func TestGate_FailureBudgetIsPerAddress(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--trusted-proxy-header=X-Real-IP",
	)

	// Spend the first address's budget.
	var exhausted bool
	for i := range 15 {
		got := srv.do(t, mcpPOST(map[string]string{
			"PRIVATE-TOKEN": "glpat-a-" + strconv.Itoa(i),
			"X-Real-IP":     "203.0.113.10",
		}))
		if got.status == http.StatusTooManyRequests {
			exhausted = true
			break
		}
	}
	if !exhausted {
		t.Fatal("the first address was never blocked")
	}

	// A different address must start clean.
	got := srv.do(t, mcpPOST(map[string]string{
		"PRIVATE-TOKEN": "glpat-b-1",
		"X-Real-IP":     "203.0.113.99",
	}))
	if got.status == http.StatusTooManyRequests {
		t.Error("a second address inherited the first one's exhausted budget; the limiter is counting the proxy, not the client")
	}
}

// TestGate_NonPostMethodsReachTheSDK verifies that GET and DELETE are answered
// 405 on a stateless deployment, which is what protocol 2026-07-28 prescribes,
// rather than being swallowed by anything mounted in front.
func TestGate_NonPostMethodsReachTheSDK(t *testing.T) {
	srv := startServer(t, nil, "--gitlab-url=https://gitlab.example.com")

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		got := srv.do(t, request{
			method:  method,
			path:    "/mcp",
			headers: map[string]string{"PRIVATE-TOKEN": "glpat-whatever", "MCP-Protocol-Version": protocolVersion},
		})
		if got.status != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp = %d, want %d", method, got.status, http.StatusMethodNotAllowed)
		}
	}
}

// TestGate_HealthAndCardNeedNoCredential verifies that the endpoints a load
// balancer and a registry read are not behind authentication.
func TestGate_HealthAndCardNeedNoCredential(t *testing.T) {
	srv := startServer(t, nil, "--gitlab-url=https://gitlab.example.com")

	for _, path := range []string{"/health", "/.well-known/mcp/server-card.json"} {
		got := srv.do(t, request{method: http.MethodGet, path: path})
		if got.status != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", path, got.status, http.StatusOK)
		}
	}
}

// TestGate_ParamHeaderMatchesTheDocumentedName verifies that a client sending
// the header this server documents is accepted.
//
// The SEP-2243 annotation carries the bare suffix and the transport prepends
// "Mcp-Param-" itself. Declaring the full name in the annotation produced
// "Mcp-Param-Mcp-Param-Action" on the wire, so a client sending the documented
// header was answered "header mismatch" and the annotation was unusable for
// the one purpose it exists for. A unit test asserting the schema value passed
// throughout; only sending the header finds it.
func TestGate_ParamHeaderMatchesTheDocumentedName(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	const body = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action","arguments":{"action":"user.get_current"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`

	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: body,
		headers: map[string]string{
			"PRIVATE-TOKEN":    "glpat-whatever",
			"Mcp-Method":       "tools/call",
			"Mcp-Name":         "gitlab_execute_action",
			"Mcp-Param-Action": "user.get_current",
		},
	})

	if strings.Contains(got.body, "header mismatch") {
		t.Fatalf("the documented header was rejected — the annotation is carrying a prefix the transport also adds: %s", got.body)
	}
	if strings.Contains(got.body, "Mcp-Param-Mcp-Param-") {
		t.Fatalf("the wire header name is doubled: %s", got.body)
	}
	// Absence of those two strings is necessary but not sufficient: a 400 or
	// 500 carrying neither would pass. The header validation is what is under
	// test, so the call must actually get through it — 200 with the tool
	// result, not merely "no known error string".
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want %d — the documented header should let the call through: %s", got.status, http.StatusOK, truncate(got.body))
	}
	if !strings.Contains(got.body, `"result"`) {
		t.Errorf("a successful tools/call must carry a JSON-RPC result, got: %s", truncate(got.body))
	}
}

// startTokenAwareGitLab is a fake GitLab whose /api/v4/user echoes the
// presented credential back as the username, so a test can tell which token a
// request was actually executed with rather than merely which one it carried.
func startTokenAwareGitLab(t *testing.T) *fakeGitLab {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("PRIVATE-TOKEN")
		w.Header().Set("Content-Type", "application/json")
		//#nosec G705 -- a test fake deliberately echoing the presented credential: the assertion is which token executed the call
		_, _ = w.Write([]byte(`{"id":1,"username":"` + token + `"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &fakeGitLab{url: srv.URL}
}

// TestGate_SessionIsBoundToTheCredentialThatMintedIt proves a session ID cannot
// be used by a caller other than the one it was issued to.
//
// The SDK short-circuits a stateful POST carrying Mcp-Session-Id straight to
// that session's own transport, without ever calling the server-resolution
// function (go-sdk streamable.go, serveStatefulPOST returns before getServer).
// Every pooled server carries its own GitLab client and token, so before the
// gate bound sessions to the credential that minted them, presenting any
// admitted credential plus a known session ID executed GitLab calls with the
// session owner's token — and the audit log recorded them under the owner's
// username, not the caller's.
//
// Only reachable with --stateless=false: the default mints no session IDs.
func TestGate_SessionIsBoundToTheCredentialThatMintedIt(t *testing.T) {
	gitlab := startTokenAwareGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--stateless=false")

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}`

	// Sessions exist only on the revisions that predate stateless streamable
	// HTTP: the SDK refuses both 2026-07-28 and 2025-11-25 unless the handler
	// is stateless, so this test speaks the newest revision that still has
	// sessions at all, with a body carrying none of the newer _meta.
	const statefulVersion = "2025-06-18"
	const statefulBody = `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`

	first := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: initialize,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-owner", "MCP-Protocol-Version": statefulVersion},
	})
	sessionID := first.header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("no session ID minted in stateful mode; status=%d body=%s", first.status, first.body)
	}

	// The owner may keep using it.
	owner := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: statefulBody,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-owner", "Mcp-Session-Id": sessionID, "MCP-Protocol-Version": statefulVersion},
	})
	if owner.status != http.StatusOK {
		t.Errorf("the session owner was refused its own session: status=%d body=%s", owner.status, owner.body)
	}

	// A different credential presenting the same session must not be served.
	intruder := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: statefulBody,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-intruder", "Mcp-Session-Id": sessionID, "MCP-Protocol-Version": statefulVersion},
	})
	if intruder.status == http.StatusOK {
		t.Fatalf("a different credential drove the session owner's server: status=%d body=%s", intruder.status, intruder.body)
	}
	if intruder.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — the terminated-session signal a conforming client recovers from", intruder.status)
	}
}

// TestProtocolVersion_UnsupportedGetsTheSpecifiedError pins the answer to a
// version this server does not implement.
//
// The transport spec requires 400 with an UnsupportedProtocolVersionError
// listing the supported versions. The SDK produces that only for versions
// sorting at or above 2026-07-28; anything older got a plain-text 400. That is
// not a cosmetic difference: the spec's backward-compatibility rule tells a
// client that a 400 whose body is not a recognizable JSON-RPC error means an
// initialization-era server, so it downgrades to the withdrawn HTTP+SSE
// transport, issues a GET, and a stateless deployment answers 405 — leaving it
// with no transport instead of one actionable retry.
func TestProtocolVersion_UnsupportedGetsTheSpecifiedError(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, version := range []string{"2024-01-01", "1999-12-31", "not-a-version"} {
		t.Run(version, func(t *testing.T) {
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: toolsListBody,
				headers: map[string]string{
					"PRIVATE-TOKEN":        "glpat-whatever",
					"MCP-Protocol-Version": version,
				},
			})

			if got.status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", got.status)
			}
			if ct := got.header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Fatalf("Content-Type = %q; a non-JSON body makes a client conclude the server is initialization-era", ct)
			}

			var body struct {
				Error struct {
					Code int `json:"code"`
					Data struct {
						Supported []string `json:"supported"`
						Requested string   `json:"requested"`
					} `json:"data"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(got.body), &body); err != nil {
				t.Fatalf("decode error body %q: %v", got.body, err)
			}
			if body.Error.Code != -32022 {
				t.Errorf("error code = %d, want -32022 (UnsupportedProtocolVersionError)", body.Error.Code)
			}
			if len(body.Error.Data.Supported) == 0 {
				t.Error("the error names no supported versions, so a client cannot retry")
			}
			if body.Error.Data.Requested != version {
				t.Errorf("requested = %q, want %q", body.Error.Data.Requested, version)
			}
			if got := requestIDOf(t, got.body); got != "1" {
				t.Errorf("id = %s, want 1 echoed back from the request", got)
			}
		})
	}
}

// TestJSONRPCError_CarriesTheRequestID pins that a refusal can be matched to
// what it refuses.
//
// "Error responses MUST include the same ID as the request they correspond to
// (except in error cases where the ID could not be read due a malformed
// request)." None of these refusals are that exception: each is decided from a
// header, on a well-formed body whose id is the second member.
//
// Both HTTP gates used to declare the id as a *string that nothing ever set, so
// every rejection went out as "id":null while the SDK's own errors, on the same
// deployment, echoed the id properly. Two defects in one: unmatchable by a
// client that routes on id, and not a legal RequestId, which under 2026-07-28
// is a string or an integer. The *string could not have carried a numeric id
// even if something had set it.
//
// The absent cases are the other half of the rule. Where there is genuinely no
// id to echo (a GET carries no request, a notification has none by
// definition), the member must be left out rather than sent as null, which is
// what the schema's optionality is for.
func TestJSONRPCError_CarriesTheRequestID(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	const stringIDBody = `{"jsonrpc":"2.0","id":"req-abc","method":"tools/list","params":{}}`
	const notificationBody = `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`

	cases := []struct {
		name    string
		req     request
		wantID  string
		wantOut bool // the member must be absent entirely
	}{
		{
			name: "unsupported version, numeric id",
			req: request{method: http.MethodPost, path: "/mcp", body: legacyToolsListBody, headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-whatever", "MCP-Protocol-Version": "1999-01-01",
			}},
			wantID: "1",
		},
		{
			name: "unsupported version, string id",
			req: request{method: http.MethodPost, path: "/mcp", body: stringIDBody, headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-whatever", "MCP-Protocol-Version": "1999-01-01",
			}},
			wantID: `"req-abc"`,
		},
		{
			// The auth gate is the same defect in a second place, which is why
			// one helper serves both: it was filed as a versioning problem and
			// is not one.
			name: "missing credential, numeric id",
			req: request{method: http.MethodPost, path: "/mcp", body: legacyToolsListBody, headers: map[string]string{
				"MCP-Protocol-Version": "2025-11-25",
			}},
			wantID: "1",
		},
		{
			name: "unsupported version, notification carries no id",
			req: request{method: http.MethodPost, path: "/mcp", body: notificationBody, headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-whatever", "MCP-Protocol-Version": "1999-01-01",
			}},
			wantOut: true,
		},
		{
			name: "unsupported version on a GET, no request to correlate with",
			req: request{method: http.MethodGet, path: "/mcp", headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-whatever", "MCP-Protocol-Version": "1999-01-01",
			}},
			wantOut: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := srv.do(t, tc.req)
			if got.status < 400 {
				t.Fatalf("status = %d, want a refusal; this case is not testing what it says", got.status)
			}

			var decoded map[string]json.RawMessage
			if err := json.Unmarshal([]byte(got.body), &decoded); err != nil {
				t.Fatalf("decode %q: %v", got.body, err)
			}
			id, present := decoded["id"]

			if tc.wantOut {
				if present {
					t.Errorf("id = %s, want the member omitted; null is not a legal RequestId", id)
				}
				return
			}
			if !present {
				t.Fatalf("no id in %s; a client routing on id cannot match this refusal", got.body)
			}
			if string(id) != tc.wantID {
				t.Errorf("id = %s, want %s", id, tc.wantID)
			}
		})
	}
}

// requestIDOf returns the raw id member of a JSON-RPC body, or "" when absent.
func requestIDOf(t *testing.T, body string) string {
	t.Helper()

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return string(decoded["id"])
}

// TestProtocolVersion_SupportedVersionsStillWork guards against the middleware
// rejecting a version the server does implement.
func TestProtocolVersion_SupportedVersionsStillWork(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, version := range []string{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"} {
		t.Run(version, func(t *testing.T) {
			body := strings.ReplaceAll(toolsListBody, protocolVersion, version)
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: body,
				headers: map[string]string{
					"PRIVATE-TOKEN":        "glpat-whatever",
					"MCP-Protocol-Version": version,
				},
			})
			if got.status == http.StatusBadRequest && strings.Contains(got.body, "-32022") {
				t.Errorf("version %s was rejected as unsupported: %s", version, got.body)
			}
		})
	}
}

// TestGate_StatefulGETAndDELETEAreAuthenticated pins that the two methods the
// gate used to wave through are protected when they can actually do something.
//
// The bypass exists for a good reason: on a stateless deployment the SDK
// answers GET and DELETE with 405 whatever they carry, so authenticating them
// would only replace the specified answer with a 401. On a stateful deployment
// they are not inert — GET opens a session's standalone SSE stream and reads
// the server-initiated messages meant for its owner, DELETE terminates the
// session — so anyone who learned a session ID could read another client's
// traffic or end their session with no credential at all.
func TestGate_StatefulGETAndDELETEAreAuthenticated(t *testing.T) {
	gitlab := startTokenAwareGitLab(t)

	t.Run("stateful refuses them without a credential", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--stateless=false")
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			t.Run(method, func(t *testing.T) {
				got := srv.do(t, request{method: method, path: "/mcp"})
				if got.status == http.StatusOK {
					t.Errorf("%s reached the session layer with no credential: %s", method, got.body)
				}
				if got.status != http.StatusUnauthorized {
					t.Errorf("%s status = %d, want %d", method, got.status, http.StatusUnauthorized)
				}
			})
		}
	})

	t.Run("stateless still answers the specified 405", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url)
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			t.Run(method, func(t *testing.T) {
				got := srv.do(t, request{method: method, path: "/mcp"})
				if got.status != http.StatusMethodNotAllowed {
					t.Errorf("%s status = %d, want %d — a stateless deployment must keep emitting the specified answer, not a 401",
						method, got.status, http.StatusMethodNotAllowed)
				}
			})
		}
	})
}

// TestDiscover_AnswersTheDeploymentItIsServedBy pins that server/discover is
// routed, describes this deployment, and is present exactly where the transport
// serves the revision it belongs to.
//
// It is the method 2026-07-28 put in place of the initialize handshake, and
// nothing in this repository exercised it: the only references anywhere are in
// internal/cachehints, whose test constructs a DiscoverResult by hand through
// the middleware and so proves the cache table, never that the method is
// reachable. No audit gate can see it either, because they all inspect
// []*mcp.Tool rather than driving a session.
//
// That matters because availability is not ours to control. The go-sdk decides
// whether the method exists at all, and the streamable transport serves the
// revision only when stateless, so an SDK bump or a transport change could
// remove a MUST-implement method and every gate in the repository would still
// pass.
//
// Both transports are driven, because "only when stateless" is the load-bearing
// half of that sentence and asserting the stateless case alone would leave it
// as a claim in a comment. Under --stateless=false the revision is refused at
// the version gate with -32022 and the method is never reached, which is the
// correct answer rather than a gap: a client that negotiated an older revision
// has no business calling a method that revision does not define.
func TestDiscover_AnswersTheDeploymentItIsServedBy(t *testing.T) {
	const discoverBody = `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`

	tests := []struct {
		name       string
		flags      []string
		wantStatus int
		check      func(t *testing.T, body string)
	}{
		{
			name:       "the stateless transport serves discover and describes this surface",
			flags:      nil,
			wantStatus: http.StatusOK,
			check:      requireDiscoverDescribesSurface,
		},
		{
			name:       "the stateful transport does not serve the revision discover belongs to",
			flags:      []string{"--stateless=false"},
			wantStatus: http.StatusBadRequest,
			check:      requireRevisionRefused,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
			srv := startServer(t, nil, append([]string{"--gitlab-url=" + gitlab.url}, tt.flags...)...)

			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: discoverBody,
				headers: map[string]string{"PRIVATE-TOKEN": "glpat-whatever"},
			})
			if got.status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", got.status, tt.wantStatus, got.body)
			}
			tt.check(t, got.body)
		})
	}
}

// requireDiscoverDescribesSurface asserts that a discover result reports the
// revision this deployment serves and the capabilities it actually registered.
// A capability it discovers but the server refuses is worse than one it never
// saw, so the absent one is asserted alongside the present ones.
func requireDiscoverDescribesSurface(t *testing.T, body string) {
	t.Helper()

	var decoded struct {
		Result struct {
			ResultType        string   `json:"resultType"`
			SupportedVersions []string `json:"supportedVersions"`
			Capabilities      struct {
				Tools     *struct{} `json:"tools"`
				Resources *struct{} `json:"resources"`
				Prompts   *struct{} `json:"prompts"`
				Logging   *struct{} `json:"logging"`
			} `json:"capabilities"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(jsonRPCPayload(t, body)), &decoded); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	if decoded.Error != nil {
		t.Fatalf("server/discover is not routed: %d %s", decoded.Error.Code, decoded.Error.Message)
	}

	if decoded.Result.ResultType != "complete" {
		t.Errorf("resultType = %q, want \"complete\"", decoded.Result.ResultType)
	}
	if !slices.Contains(decoded.Result.SupportedVersions, "2026-07-28") {
		t.Errorf("supportedVersions = %v, want it to include the revision this deployment serves", decoded.Result.SupportedVersions)
	}
	if decoded.Result.Capabilities.Tools == nil {
		t.Error("capabilities omit tools, which this deployment registers")
	}
	if decoded.Result.Capabilities.Resources == nil {
		t.Error("capabilities omit resources, which the full capability surface registers")
	}
	if decoded.Result.Capabilities.Logging != nil {
		t.Error("capabilities advertise logging, which this server does not implement")
	}
}

// requireRevisionRefused asserts that the stateful transport turns the request
// away at the version gate rather than reaching the method, and that it names
// what it does serve so a client can retry rather than guess.
func requireRevisionRefused(t *testing.T, body string) {
	t.Helper()

	var decoded struct {
		Error *struct {
			Code int `json:"code"`
			Data struct {
				Supported []string `json:"supported"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(jsonRPCPayload(t, body)), &decoded); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	if decoded.Error == nil {
		t.Fatalf("the stateful transport answered a revision it does not serve: %s", body)
	}
	if decoded.Error.Code != -32022 {
		t.Errorf("code = %d, want -32022 (UnsupportedProtocolVersionError)", decoded.Error.Code)
	}
	if slices.Contains(decoded.Error.Data.Supported, "2026-07-28") {
		t.Errorf("supported = %v, want it to exclude the revision that was just refused", decoded.Error.Data.Supported)
	}
	if len(decoded.Error.Data.Supported) == 0 {
		t.Error("the refusal names no revision the client could retry with")
	}
}

// TestElicitingToolCall_WithoutAHandshake_DoesNotKillTheServer is the wire
// regression for the crash that took the whole process down.
//
// FromRequest read the session's InitializeParams and guarded only against a
// nil params pointer, not a nil Capabilities pointer inside it. The SDK
// synthesizes InitializeParams carrying just a protocol version for a request
// that arrives without a handshake, so Capabilities was nil and the
// dereference panicked in the SDK's jsonrpc2 goroutine, where nothing recovers.
//
// It required no client misbehavior. Under the default stateless transport
// every POST is its own session with no handshake, so an ordinary
// pre-2026-07-28 client hit it on any tools/call that could elicit, and one
// such call killed the endpoint for every other tenant on it.
//
// The unit tests in internal/elicitation cannot see this: a ServerSession built
// in a test has nil InitializeParams, which the old guard already caught. Only
// a real session built by the SDK from a real request has the shape that
// crashed, so the regression has to be driven over the wire.
//
// The assertion is deliberately about survival rather than the answer. Whether
// this call ends in a confirmation prompt or a refusal to elicit is settled
// elsewhere; what is pinned here is that the process is still serving
// afterwards.
func TestElicitingToolCall_WithoutAHandshake_DoesNotKillTheServer(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	// An interactive action on the default dynamic surface. It builds an
	// elicitation client before it does anything else, and no initialize
	// precedes the call. The arguments have to satisfy the tool's schema or the
	// SDK rejects the call before the handler runs, which is exactly how an
	// earlier version of this test passed against the unfixed binary.
	const call = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action","arguments":{"action":"interactive.issue_create","params":{"project_id":"1"}}}}`

	// The legacy protocol version is what puts the SDK on the synthesizing
	// path: it builds InitializeParams for a request that never handshook and
	// leaves Capabilities, a pointer, nil.
	legacy := map[string]string{
		"PRIVATE-TOKEN":        "glpat-whatever",
		"MCP-Protocol-Version": "2025-11-25",
	}

	// The response itself is not the assertion: whatever the SDK answers to
	// the eliciting call, the process surviving it is what this test is for.
	_ = srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: call, headers: legacy,
	})
	// The process must still be serving. A panic in the SDK's reading goroutine
	// takes the listener with it, so the next request is the real assertion.
	after := srv.do(t, request{
		method: http.MethodPost, path: "/mcp",
		body:    `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		headers: legacy,
	})
	if after.status != http.StatusOK {
		t.Fatalf("the server stopped serving after the eliciting call: status = %d, body = %s", after.status, after.body)
	}
}
