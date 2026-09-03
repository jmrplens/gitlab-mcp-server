//go:build httpe2e

package httpe2e

import (
	"maps"
	"net/http"
	"strings"
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
	headers := map[string]string{"PRIVATE-TOKEN": "glpat-e2e-authorized"}
	maps.Copy(headers, extra)
	return request{body: body, headers: headers}
}

// withAction adds the parameter header protocol 2026-07-28 requires alongside
// an action argument.
func withAction(action string) map[string]string {
	return map[string]string{"Mcp-Param-Action": action}
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
func TestCollectorPrivacy_NothingPrivateReachesTheCollector(t *testing.T) {
	// Values chosen to be unmistakable if they appear, and to stand for the
	// categories that matter: a private project path, a search query, the
	// caller's GitLab credential, and the credential this server uses to reach
	// its own collector.
	const (
		projectPath  = "acme-holdings/internal-secrets-repo"
		searchQuery  = "quarterly-layoff-plan"
		clientToken  = "glpat-client-private-token-value"
		collectorPwd = "collector-password-value"
	)

	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	c := startCollector(t)
	env := collectorEnv(c)
	env["OTEL_EXPORTER_OTLP_HEADERS"] = "Authorization=Bearer%20" + collectorPwd
	srv := startServer(t, env, "--gitlab-url="+gitlab.url)

	// Calls carrying every category. Whether each succeeds is beside the point:
	// a failing call is exactly where a server is most tempted to record what it
	// was given.
	calls := []struct {
		name string
		call request
	}{
		{"project_path_in_tool_arguments", authorizedCall(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+protocolMeta+`,"name":"gitlab_execute_action","arguments":{"action":"issue.list","project_id":"`+projectPath+`"}}}`, withAction("issue.list"))},
		{"search_query_in_tool_arguments", authorizedCall(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{`+protocolMeta+`,"name":"gitlab_execute_action","arguments":{"action":"search.code","search":"`+searchQuery+`"}}}`, withAction("search.code"))},
		{"project_path_in_resource_uri", authorizedCall(`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{`+protocolMeta+`,"uri":"gitlab://projects/`+projectPath+`/issues"}}`, nil)},
		{"client_token_in_header", request{body: `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{` + protocolMeta + `}}`, headers: map[string]string{"PRIVATE-TOKEN": clientToken}}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			srv.do(t, tc.call)
		})
	}

	c.awaitExport(t, 20*time.Second)
	// A moment more, so later batches are included rather than only the first.
	time.Sleep(700 * time.Millisecond)

	c.assertNoPayloadContains(t,
		projectPath,
		searchQuery,
		clientToken,
		collectorPwd,
	)
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

	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	c := startCollector(t)
	srv := startServer(t, collectorEnv(c), "--gitlab-url="+gitlab.url)

	for range 3 {
		srv.do(t, authorizedCall(
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+protocolMeta+`,"name":"gitlab_execute_action","arguments":{"action":"issue.list","project_id":"`+projectPath+`"}}}`, withAction("issue.list"),
		))
	}

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

	c.assertNoPayloadContains(t, projectPath)
}

// TestCollectorPrivacy_ARefusedRequestProducesNoMCPSpan pins a property that
// falls out of where the instrumentation is mounted, and that is worth having
// on purpose rather than by accident.
//
// The MCP middleware sits inside the authentication gate, so a request refused
// at the door never reaches it and no MCP span exists for it. The HTTP span for
// the refusal itself does exist, deliberately: refusals are the request rate an
// operator most wants to see, and that span carries method and status and
// nothing a caller chose. What an unauthenticated scanner cannot do is drive
// the expensive, attribute-rich MCP telemetry, which is where volume costs
// money on a published endpoint.
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
