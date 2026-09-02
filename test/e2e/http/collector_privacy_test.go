//go:build httpe2e

package httpe2e

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// protocolMeta is the _meta block protocol 2026-07-28 requires on every
// request.
//
// Without it the SDK answers -32602 before any handler runs, which is a
// perfectly good rejection and useless for these tests: they need a call that
// reaches the instrumentation. The first draft omitted it and spent three runs
// asserting that a rejected request records no action, which was true and not
// what was being tested.
const protocolMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

// authorizedCall is a request that reaches the MCP handler rather than being
// refused at the door, which is what these tests need to observe a span.
//
// Two things beyond the credential are required before a handler runs, and both
// were learned by watching a request fail rather than by reading a spec. The
// _meta block above carries the protocol version. And protocol 2026-07-28
// requires every parameter to be mirrored in an Mcp-Param-* header, so an
// action argument without an Mcp-Param-Action header is refused with -32020
// before any middleware sees it.
func authorizedCall(body string, extra map[string]string) request {
	headers := map[string]string{"PRIVATE-TOKEN": privacyClientToken}
	maps.Copy(headers, extra)
	return request{body: body, headers: headers}
}

// withAction adds the parameter header protocol 2026-07-28 requires alongside
// an action argument.
func withAction(action string) map[string]string {
	return map[string]string{"Mcp-Param-Action": action}
}

// executeAction spells a gitlab_execute_action call the way the dynamic surface
// declares it.
//
// The action's own arguments go **under params**. They used to be written as
// siblings of action here, and every call in this file was answered with
// `validating "arguments": ... unexpected additional properties`: the request
// never reached a handler, never called GitLab, and never produced the log
// record these tests exist to search. The identical mistake was found and fixed
// in the sibling collectore2e module a while ago; this copy was left behind, so
// the privacy gate that runs on every push was green while the executing path
// it grades was never driven.
func executeAction(id int, action, params string) request {
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{%s,"name":"gitlab_execute_action","arguments":{"action":%q,"params":%s}}}`,
		id, protocolMeta, action, params,
	)
	return authorizedCall(body, withAction(action))
}

// readResource spells a resources/read call, whose name header is the URI.
//
// resources/read is one of the three methods protocol 2026-07-28 requires an
// Mcp-Name header for, and the harness only sets it for tools/call. Without it
// the transport answers -32020 before the server sees the request, which is how
// the resource case here spent its life proving that a rejected request records
// no URI.
func readResource(id int, uri string) request {
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"resources/read","params":{%s,"uri":%q}}`,
		id, protocolMeta, uri,
	)
	return authorizedCall(body, map[string]string{"Mcp-Name": uri})
}

// privacyClientToken is the caller credential these tests send.
//
// The final four characters are distinctive on purpose. The server logs a
// masked "..."+last four as token_suffix, so a token ending in a common word
// (the previous value ended "alue") makes the assertion that the credential
// does not leave the process unable to fail even when a masked form does.
const privacyClientToken = "glpat-client-private-token-ZZZ9"

// maskedTokenSuffix is what the server logs instead of the token: the last four
// characters behind an ellipsis. It is a stable per-credential handle, so it
// belongs in the forbidden list beside the token itself.
const maskedTokenSuffix = "...ZZZ9"

// assertReached fails when a call never got past the transport or the schema.
//
// A JSON-RPC error means the request was refused before any handler ran, which
// is the failure mode that made this whole file vacuous: every assertion below
// is about what a *handler* exports, and a refusal exports the refusal instead
// and passes every privacy check trivially. Borrowed from the sibling
// collectore2e harness, which learned it the same way.
func assertReached(t *testing.T, what string, got response) {
	t.Helper()
	if strings.Contains(got.body, `"error":{`) {
		t.Fatalf("%s was refused before any handler ran, so nothing downstream of validation was exercised: %s",
			what, got.body)
	}
}

// privacyGitLab is a GitLab stand-in that records which paths were called.
//
// startFakeGitLab in the harness counts only /api/v4/user, which answers the
// credential probe and says nothing about whether a tool call reached the
// instance. These tests need exactly that: the whole point of the rewrite is to
// prove the request traveled all the way to GitLab and back, so "GitLab was
// asked" has to be assertable per path.
type privacyGitLab struct {
	url string

	mu   sync.Mutex
	hits map[string]int
	// raw keeps the request-target of every call, so a test can assert on the
	// query string GitLab was sent as well as on the path.
	raw []string
}

// startPrivacyGitLab serves the endpoints a tool call reaches, with routes
// under the caller's control.
//
// routes maps a path prefix to the handler for it. Anything unmatched answers
// 404, which every caller in this server handles.
func startPrivacyGitLab(t *testing.T, routes map[string]http.HandlerFunc) *privacyGitLab {
	t.Helper()

	g := &privacyGitLab{hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The escaped form, not r.URL.Path. GitLab addresses a project by its
		// URL-encoded path, so %2F is a character of the identifier rather than
		// a separator, and net/url hands the decoded spelling to r.URL.Path —
		// which matched no route here and made every call 404 while looking
		// like it had been served.
		path := r.URL.EscapedPath()

		g.mu.Lock()
		g.hits[path]++
		g.raw = append(g.raw, r.URL.RequestURI())
		g.mu.Unlock()

		for prefix, handler := range routes {
			if strings.HasPrefix(path, prefix) {
				handler(w, r)
				return
			}
		}
		switch path {
		case "/api/v4/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
		case "/api/v4/user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":7,"username":"someone"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	g.url = srv.URL
	return g
}

// hitsFor reports how many requests reached a path prefix.
func (g *privacyGitLab) hitsFor(prefix string) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	total := 0
	for path, n := range g.hits {
		if strings.HasPrefix(path, prefix) {
			total += n
		}
	}
	return total
}

// assertCalled fails when GitLab was never asked, which means the assertions
// that follow are about a path that never ran.
func (g *privacyGitLab) assertCalled(t *testing.T, prefix string) {
	t.Helper()
	if g.hitsFor(prefix) == 0 {
		g.mu.Lock()
		seen := make([]string, 0, len(g.raw))
		seen = append(seen, g.raw...)
		g.mu.Unlock()
		t.Fatalf("no request reached GitLab at %s, so the executing path was never driven; GitLab saw %v", prefix, seen)
	}
}

// TestCollectorPrivacy_NothingPrivateReachesTheCollector turns this project's
// stated privacy position into an assertion about bytes on a wire.
//
// Everything the design says is not recorded has, until now, been a claim you
// could only check by reading code: tool arguments, resource URIs, search
// queries, credentials. A reader can verify each one and still miss the field
// nobody thought about. This drives real traffic carrying distinctive values
// and then searches every exported payload for them.
//
// Searching raw protobuf is the point rather than a shortcut. Protobuf encodes
// strings as UTF-8 literals with no framing inside them, so a value that
// reached any attribute, any span name, any log body or any field added later
// appears verbatim in these bytes. The negative is proved across the whole
// payload at once, which a decoder driven by a list of fields cannot do.
//
// Every call here is asserted to have reached GitLab. Without that the test
// grades a validation refusal, which is what it did for the life of this file.
func TestCollectorPrivacy_NothingPrivateReachesTheCollector(t *testing.T) {
	// Values chosen to be unmistakable if they appear, and to stand for the
	// categories that matter: a private project path, a search query, the
	// caller's GitLab credential, and the credential this server uses to reach
	// its own collector.
	const (
		projectPath  = "acme-holdings/internal-secrets-repo"
		searchQuery  = "quarterly-layoff-plan"
		collectorPwd = "collector-password-value"
	)
	encodedPath := url.PathEscape(projectPath)
	// A resource URI addresses a project by its URL-encoded path, and client-go
	// escapes whatever identifier it is handed before putting it in a request
	// target. So the identifier that arrives at GitLab from the resource path
	// is escaped twice, and a fake serving only the once-escaped form answers
	// 404 while looking as though it had been asked.
	reEncodedPath := url.PathEscape(encodedPath)

	gitlab := startPrivacyGitLab(t, map[string]http.HandlerFunc{
		"/api/v4/projects/" + encodedPath + "/issues": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		},
		"/api/v4/search": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		},
		"/api/v4/projects/" + reEncodedPath: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"path_with_namespace":"` + projectPath + `","name":"internal-secrets-repo"}`))
		},
	})
	c := startCollector(t)
	env := collectorEnv(c)
	env["OTEL_EXPORTER_OTLP_HEADERS"] = "Authorization=Bearer%20" + collectorPwd
	srv := startServer(t, env, "--gitlab-url="+gitlab.url)

	// Calls carrying every category, each one reaching a handler and GitLab.
	calls := []struct {
		name  string
		call  request
		asked string
	}{
		{
			name:  "project_path_in_tool_arguments",
			call:  executeAction(1, "issue.list", `{"project_id":"`+projectPath+`"}`),
			asked: "/api/v4/projects/" + encodedPath + "/issues",
		},
		{
			name:  "search_query_in_tool_arguments",
			call:  executeAction(2, "search.code", `{"query":"`+searchQuery+`"}`),
			asked: "/api/v4/search",
		},
		{
			name:  "project_path_in_resource_uri",
			call:  readResource(3, "gitlab://project/"+encodedPath),
			asked: "/api/v4/projects/" + reEncodedPath,
		},
		{
			name: "client_token_in_header",
			call: request{
				body:    `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{` + protocolMeta + `}}`,
				headers: map[string]string{"PRIVATE-TOKEN": privacyClientToken},
			},
		},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			assertReached(t, tc.name, srv.do(t, tc.call))
			if tc.asked != "" {
				gitlab.assertCalled(t, tc.asked)
			}
		})
	}

	c.awaitExport(t, 20*time.Second)
	// A moment more, so later batches are included rather than only the first.
	time.Sleep(700 * time.Millisecond)

	c.assertNoPayloadContains(t,
		projectPath,
		// client-go reports Request.URL.RawPath, so a leak carries the
		// URL-encoded spelling and a literal-path assertion cannot match it.
		encodedPath,
		searchQuery,
		privacyClientToken,
		maskedTokenSuffix,
		collectorPwd,
	)
}

// TestCollectorPrivacy_AFailedCallExportsNeitherURLNorErrorBody covers the half
// of the privacy claim that only exists when GitLab says no.
//
// A failed tool call is logged at ERROR with the wrapped error, and the log
// bridge exports every record at INFO and above. client-go formats an API
// failure as "METHOD scheme://host/path: CODE body", so the exported record
// carried the URL-encoded project path, the query string and GitLab's own
// response text — under every identity policy, including the default that
// records nobody. docs/guides/telemetry.md promises the opposite, twice, and
// names a test as proof; this is that test.
//
// Two failure shapes, because they leak through different types: an API error
// carries the path and the response body, and a transport failure is a
// *url.Error whose text carries the full URL including the query string.
func TestCollectorPrivacy_AFailedCallExportsNeitherURLNorErrorBody(t *testing.T) {
	const (
		projectPath = "private-group/super-secret-project"
		searchQuery = "quarterly-layoff-plan"
		bodyMarker  = "SECRET-BODY-MARKER"
	)
	encodedPath := url.PathEscape(projectPath)

	gitlab := startPrivacyGitLab(t, map[string]http.HandlerFunc{
		"/api/v4/projects/" + encodedPath + "/merge_requests": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"403 Forbidden - insufficient_scope for ` + bodyMarker + `"}`))
		},
		"/api/v4/search": func(w http.ResponseWriter, r *http.Request) {
			// Hang up mid-response, which is what a *url.Error is made of.
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, hijackErr := hijacker.Hijack()
				if hijackErr == nil {
					_ = conn.Close()
					return
				}
			}
			// A server that cannot hijack still has to answer something; a 500
			// keeps the test meaningful rather than silently passing.
			t.Errorf("could not hijack %s, so the transport-failure leg was not exercised", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		},
	})
	c := startCollector(t)
	srv := startServer(t, collectorEnv(c), "--gitlab-url="+gitlab.url)

	failures := []struct {
		name  string
		call  request
		asked string
	}{
		{
			name:  "an api error carrying a path and a response body",
			call:  executeAction(1, "merge_request.list", `{"project_id":"`+projectPath+`"}`),
			asked: "/api/v4/projects/" + encodedPath + "/merge_requests",
		},
		{
			name:  "a transport failure carrying a full url",
			call:  executeAction(2, "search.code", `{"query":"`+searchQuery+`"}`),
			asked: "/api/v4/search",
		},
	}
	for _, tc := range failures {
		t.Run(tc.name, func(t *testing.T) {
			// The call is expected to fail at GitLab; what must not happen is a
			// refusal before the handler, which would leave the log record
			// under test unwritten.
			assertReached(t, tc.name, srv.do(t, tc.call))
			gitlab.assertCalled(t, tc.asked)
		})
	}

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	c.assertNoPayloadContains(t,
		projectPath,
		encodedPath,
		searchQuery,
		bodyMarker,
	)
}

// TestCollectorPrivacy_AnUnauthenticatedRequestCannotSizeAnExportedRecord pins
// the cost half of the same boundary.
//
// net/http accepts any token as a method, and the HTTP span recorded the
// original verbatim when it was not one the convention names — before
// authentication, on every bind including the container's default. A scanner
// could therefore push roughly a megabyte of chosen bytes per request into the
// operator's traces backend without holding a credential. The file's own
// comment used to say "the span is where an unbounded value is affordable",
// which is true of cardinality and false of bytes.
func TestCollectorPrivacy_AnUnauthenticatedRequestCannotSizeAnExportedRecord(t *testing.T) {
	const marker = "TAILMARKERZZ"
	// The marker sits at the end, so a bounded prefix cannot contain it and the
	// assertion measures the bound rather than the presence of a truncation.
	longMethod := strings.Repeat("Q", 64*1024) + marker

	c := startCollector(t)
	srv := startServer(t, collectorEnv(c))

	const requests = 20
	for range requests {
		srv.do(t, request{method: longMethod, path: "/mcp"})
	}

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	t.Run("the chosen bytes are not exported", func(t *testing.T) {
		c.assertNoPayloadContains(t, marker)
	})

	t.Run("the exported volume is not the caller's to choose", func(t *testing.T) {
		var total int
		for _, e := range c.received() {
			total += len(e.body)
		}
		// 20 requests carrying 64 KiB each is 1.3 MB of attacker traffic. Any
		// bound well under that proves the bytes are not relayed; this one is
		// generous so ordinary telemetry volume never trips it.
		const bound = 256 * 1024
		if total > bound {
			t.Errorf("%d requests produced %d exported bytes, over the %d bound: the caller sizes the export",
				requests, total, bound)
		}
	})
}

// TestCollectorPrivacy_TheActionIsRecordedAndTheArgumentsAreNot is the other
// half, and without it the test above passes trivially.
//
// A server that exported nothing at all would satisfy every privacy assertion
// ever written. This asserts the payload carries what it is supposed to: the
// method and the canonical action, so an operator can see what the server did,
// alongside the absence of what it was given.
func TestCollectorPrivacy_TheActionIsRecordedAndTheArgumentsAreNot(t *testing.T) {
	const projectPath = "acme-holdings/another-private-repo"
	encodedPath := url.PathEscape(projectPath)

	gitlab := startPrivacyGitLab(t, map[string]http.HandlerFunc{
		"/api/v4/projects/" + encodedPath + "/issues": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		},
	})
	c := startCollector(t)
	srv := startServer(t, collectorEnv(c), "--gitlab-url="+gitlab.url)

	for i := range 3 {
		assertReached(t, "issue.list",
			srv.do(t, executeAction(i+1, "issue.list", `{"project_id":"`+projectPath+`"}`)))
	}
	gitlab.assertCalled(t, "/api/v4/projects/"+encodedPath+"/issues")

	c.awaitExport(t, 20*time.Second)
	time.Sleep(700 * time.Millisecond)

	var sawMethod, sawAction bool
	for _, e := range c.received() {
		payload := string(e.body)
		if strings.Contains(payload, "tools/call") {
			sawMethod = true
		}
		if strings.Contains(payload, "issue.list") {
			sawAction = true
		}
	}
	if !sawMethod {
		t.Errorf("no exported payload names the method; %d payloads arrived carrying nothing identifying the operation",
			len(c.received()))
	}
	if !sawAction {
		t.Error("no exported payload carries the canonical action; on the default surface the tool name is the same for every call, so this is the only thing that says what was done")
	}

	c.assertNoPayloadContains(t, projectPath, encodedPath)
}

// TestCollectorPrivacy_ARefusedRequestProducesNoMCPSpan pins a property that
// falls out of where the instrumentation is mounted, and that is worth having
// on purpose rather than by accident.
//
// The MCP middleware sits inside the authentication gate, so a request refused
// at the door never reaches it and no MCP span exists for it. The HTTP span for
// the refusal itself does exist, deliberately: refusals are the request rate an
// operator most wants to see, and that span carries method, scheme and status,
// with an unrecognized method bounded to a short prefix. What an unauthenticated
// scanner cannot do is drive the expensive, attribute-rich MCP telemetry, which
// is where volume costs money on a published endpoint.
//
// The refusal is still visible, in the server's own structured log, so nothing
// is hidden. It is the exported telemetry that stays quiet.
func TestCollectorPrivacy_ARefusedRequestProducesNoMCPSpan(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, collectorEnv(c))

	for range 5 {
		got := srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action","arguments":{"action":"issue.list"}}}`})
		if got.status == http.StatusOK {
			t.Skip("the request was not refused; this test needs a rejection to be meaningful")
		}
	}

	// Long enough for a batch to have gone out had there been anything in it.
	time.Sleep(1500 * time.Millisecond)

	for _, e := range c.received() {
		if strings.Contains(string(e.body), "tools/call") {
			t.Errorf("a refused request produced an MCP span; anonymous traffic can drive telemetry volume\n%s", e.path)
		}
	}
}

// TestCollectorPrivacy_TheDetectorWorks guards the tests above from passing
// because the search is broken rather than because nothing leaked.
//
// A privacy assertion that cannot fail is worse than no assertion: it reads as
// evidence and is decoration. This sends a value that IS expected in the
// payload and asserts the search finds it, which proves the same search would
// find a project path if one were there.
func TestCollectorPrivacy_TheDetectorWorks(t *testing.T) {
	c := startCollector(t)
	env := collectorEnv(c)
	// service.name reaches the resource on every payload, so it is a value the
	// search must find. If it does not, the search is looking in the wrong
	// place and every other privacy assertion here is meaningless.
	env["OTEL_SERVICE_NAME"] = "canary-service-name-value"
	srv := startServer(t, env)

	srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + protocolMeta + `}}`})
	c.awaitExport(t, 20*time.Second)
	time.Sleep(500 * time.Millisecond)

	var found bool
	for _, e := range c.received() {
		if strings.Contains(string(e.body), "canary-service-name-value") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("a value known to be in the payload was not found; the leak search is broken and the privacy tests prove nothing")
	}
}
