//go:build httpe2e

// limits_test.go checks that the HTTP flags which restrict something actually
// restrict it.
//
// A limit that does not limit is worse than no limit: the operator believes a
// bound is in place, sizes the deployment around it, and finds out otherwise
// under load or after an incident. Each case here sets the flag to a value
// small enough to observe from outside and then tries to exceed it.
package httpe2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// countTools returns how many tools a tools/list reply advertises.
func countTools(body string) int {
	return strings.Count(body, `"name":"gitlab_`)
}

// listTools performs a tools/list against a server that will accept the
// credential, and returns the reply body.
func listTools(t *testing.T, srv *server) string {
	t.Helper()
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: toolsListBody,
		headers: map[string]string{
			"PRIVATE-TOKEN": "glpat-x",
			"Mcp-Method":    "tools/list",
		},
	})
	if got.status != http.StatusOK {
		t.Fatalf("tools/list = %d: %s", got.status, truncate(got.body))
	}
	return got.body
}

// rateLimitedCall makes one tools/call, the method whose refusal the limiter
// reports as a tool result rather than a JSON-RPC error.
func rateLimitedCall(t *testing.T, srv *server) response {
	t.Helper()
	return srv.do(t, request{
		method: http.MethodPost, path: "/mcp",
		body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_find_action","arguments":{"query":"list projects"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`,
		headers: map[string]string{
			"PRIVATE-TOKEN": "glpat-x",
			"Mcp-Method":    "tools/call",
			"Mcp-Name":      "gitlab_find_action",
		},
	})
}

// acceptingGitLab is a fake instance that accepts the credential, so a request
// gets far enough for surface limits to be observable.
func acceptingGitLab(t *testing.T) *fakeGitLab {
	t.Helper()
	return startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
}

// TestLimit_ToolSurface verifies that --tool-surface changes how many tools are
// advertised, which is the whole point of the setting: the dynamic surface
// exists to keep a client's context small.
func TestLimit_ToolSurface(t *testing.T) {
	gitlab := acceptingGitLab(t)

	dynamic := countTools(listTools(t, startServer(t,
		map[string]string{"TOOL_SURFACE": "dynamic"}, "--gitlab-url="+gitlab.url)))
	meta := countTools(listTools(t, startServer(t,
		map[string]string{"TOOL_SURFACE": "meta"}, "--gitlab-url="+gitlab.url)))
	individual := countTools(listTools(t, startServer(t,
		map[string]string{"TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url)))

	if dynamic != 2 {
		t.Errorf("dynamic surface advertised %d tools, want 2 (find + execute)", dynamic)
	}
	if meta <= dynamic {
		t.Errorf("meta surface advertised %d tools, want more than dynamic's %d", meta, dynamic)
	}
	if individual <= meta {
		t.Errorf("individual surface advertised %d tools, want more than meta's %d", individual, meta)
	}
}

// TestLimit_ExcludeTools verifies that a named tool is actually removed rather
// than merely hidden from a listing.
func TestLimit_ExcludeTools(t *testing.T) {
	gitlab := acceptingGitLab(t)

	const victim = "gitlab_find_action"
	srv := startServer(t, map[string]string{"EXCLUDE_TOOLS": victim}, "--gitlab-url="+gitlab.url)

	body := listTools(t, srv)
	if strings.Contains(body, `"name":"`+victim+`"`) {
		t.Errorf("%s is still advertised despite EXCLUDE_TOOLS", victim)
	}

	// And calling it must fail, not merely be undiscoverable.
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp",
		body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + victim + `","arguments":{"query":"x"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`,
		headers: map[string]string{
			"PRIVATE-TOKEN": "glpat-x",
			"Mcp-Method":    "tools/call",
			"Mcp-Name":      victim,
		},
	})
	if !strings.Contains(got.body, "unknown tool") {
		t.Errorf("an excluded tool was still callable: %s", truncate(got.body))
	}
}

// TestLimit_ReadOnlyRemovesMutations verifies that --read-only takes the
// mutating surface away rather than relying on the model not to ask.
func TestLimit_ReadOnlyRemovesMutations(t *testing.T) {
	gitlab := acceptingGitLab(t)

	full := listTools(t, startServer(t,
		map[string]string{"TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url))
	readOnly := listTools(t, startServer(t,
		map[string]string{"TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url, "--read-only"))

	if countTools(readOnly) >= countTools(full) {
		t.Errorf("read-only advertised %d tools against %d for a writable server; nothing was removed",
			countTools(readOnly), countTools(full))
	}
	for _, mutating := range []string{`"name":"gitlab_create_`, `"name":"gitlab_delete_`, `"name":"gitlab_update_`} {
		t.Run(mutating, func(t *testing.T) {
			if strings.Contains(readOnly, mutating) {
				t.Errorf("a mutating tool matching %s survived --read-only", mutating)
			}
		})
	}
}

// TestLimit_SafeModeKeepsToolsButRefusesToActFor verifies the difference
// between the two protective modes, which is easy to conflate: read-only
// removes the tools, safe mode keeps them and intercepts the call.
func TestLimit_SafeModeKeepsToolsButRefusesToActFor(t *testing.T) {
	gitlab := acceptingGitLab(t)

	full := countTools(listTools(t, startServer(t,
		map[string]string{"TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url)))
	safe := countTools(listTools(t, startServer(t,
		map[string]string{"TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url, "--safe-mode")))

	if safe != full {
		t.Errorf("safe mode advertised %d tools against %d; it should keep the surface and intercept the call instead", safe, full)
	}
}

// TestLimit_CapabilitySurfaceMinimal verifies that the minimal capability
// surface actually serves fewer resources.
func TestLimit_CapabilitySurfaceMinimal(t *testing.T) {
	gitlab := acceptingGitLab(t)

	listResources := func(srv *server) int {
		got := srv.do(t, request{
			method: http.MethodPost, path: "/mcp",
			body: `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`,
			headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-x",
				"Mcp-Method":    "resources/list",
			},
		})
		if got.status != http.StatusOK {
			t.Fatalf("resources/list = %d: %s", got.status, truncate(got.body))
		}
		return strings.Count(got.body, `"uri":"gitlab://`)
	}

	full := listResources(startServer(t,
		map[string]string{"CAPABILITY_SURFACE": "full"}, "--gitlab-url="+gitlab.url))
	minimal := listResources(startServer(t,
		map[string]string{"CAPABILITY_SURFACE": "minimal"}, "--gitlab-url="+gitlab.url))

	if minimal >= full {
		t.Errorf("minimal served %d resources against %d for full; the surface was not reduced", minimal, full)
	}
	if minimal == 0 {
		t.Error("minimal served no resources at all; the tool manifest is meant to survive")
	}
}

// TestLimit_MaxHTTPClientsBoundsThePool verifies that --max-http-clients is a
// real bound: past it the least recently used entry is evicted, so a
// deployment cannot be made to hold one registered server per credential
// anyone cares to invent.
func TestLimit_MaxHTTPClientsBoundsThePool(t *testing.T) {
	gitlab := acceptingGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--max-http-clients=2")

	// Three distinct credentials against a bound of two.
	for i := range 3 {
		got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": fmt.Sprintf("glpat-bound-%d", i)}))
		if got.status >= http.StatusInternalServerError {
			t.Fatalf("credential %d got %d", i, got.status)
		}
	}
	afterFill := gitlab.calls()

	// The first credential must have been evicted, so using it again rebuilds
	// and costs another upstream round trip. If the bound were not enforced,
	// the entry would still be cached and cost nothing.
	srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-bound-0"}))
	if gitlab.calls() == afterFill {
		t.Error("the evicted credential was served from cache; --max-http-clients is not bounding the pool")
	}

	// And the most recent one must still be cached, or the pool is evicting
	// everything rather than the least recently used.
	before := gitlab.calls()
	srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-bound-0"}))
	if gitlab.calls() != before {
		t.Error("a freshly used credential was rebuilt; the pool is not retaining anything")
	}
}

// TestLimit_PoolIdleTimeoutIsAccepted verifies that the flag is honored at
// startup and the deployment keeps serving with it set.
//
// Reclamation itself is not asserted here on purpose. The sweep runs at a
// quarter of the timeout but never more often than once a minute, so a value
// small enough for a test to wait out is dominated by that floor — an entry
// configured to expire after a second can still be held for up to a minute.
// evictIdle is exercised directly in internal/serverpool, where the test
// controls time instead of racing it.
func TestLimit_PoolIdleTimeoutIsAccepted(t *testing.T) {
	gitlab := acceptingGitLab(t)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--pool-idle-timeout=30s",
		"--revalidate-interval=0",
	)

	got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-idle"}))
	if got.status >= http.StatusInternalServerError {
		t.Fatalf("status = %d with a short idle timeout set", got.status)
	}
	assertStillServing(t, srv, "a short --pool-idle-timeout")
}

// TestLimit_MaxRequestBodyBytes verifies both halves of the body cap: a
// configured value is enforced, and the default is not unlimited.
func TestLimit_MaxRequestBodyBytes(t *testing.T) {
	gitlab := acceptingGitLab(t)

	t.Run("configured value is enforced", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--max-request-body-bytes=2048")
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"pad":"` + strings.Repeat("A", 8192) + `"}}`
		got := srv.do(t, request{
			method: http.MethodPost, path: "/mcp", body: body,
			headers: map[string]string{"PRIVATE-TOKEN": "glpat-x", "Mcp-Method": "tools/list"},
		})
		if got.status == http.StatusOK {
			t.Errorf("an %d-byte body was accepted against a 2048-byte cap", len(body))
		}
	})

	t.Run("default is not unlimited", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url)
		// Comfortably past the SDK's 4 MiB default.
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"pad":"` + strings.Repeat("A", 12<<20) + `"}}`
		got := srv.do(t, request{
			method: http.MethodPost, path: "/mcp", body: body,
			headers: map[string]string{"PRIVATE-TOKEN": "glpat-x", "Mcp-Method": "tools/list"},
		})
		if got.status == http.StatusOK {
			t.Error("a 12 MiB body was accepted with no explicit cap; the default is unlimited")
		}
		assertStillServing(t, srv, "a 12 MiB body")
	})
}

// TestLimit_RateLimitRPS verifies that the tools/call rate limiter engages when
// configured, that HTTP mode has it on by default, and that a refusal is
// visible to an operator.
//
// This used to carry a subtest called "off by default" asserting the opposite.
// It passed for a reason that had nothing to do with the claim: HTTP mode
// defaults to 10 rps with a burst of 40, and fifteen serial calls never drain a
// bucket of forty. The suite documented a default that had not held since the
// flag gained one.
//
// It is a per-server limit rather than a per-address one, so it protects the
// GitLab instance behind this server from a client looping rather than
// protecting this server from many clients — which is why the failure budget
// exists separately.
func TestLimit_RateLimitRPS(t *testing.T) {
	gitlab := acceptingGitLab(t)

	t.Run("engages when configured", func(t *testing.T) {
		srv := startServer(t, nil,
			"--gitlab-url="+gitlab.url,
			"--rate-limit-rps=1",
			"--rate-limit-burst=1",
		)
		var limited bool
		for range 15 {
			body := rateLimitedCall(t, srv).body
			if strings.Contains(strings.ToLower(body), "rate limit") || strings.Contains(body, "-42900") {
				limited = true
				break
			}
		}
		if !limited {
			t.Error("15 tool calls at --rate-limit-rps=1 --rate-limit-burst=1 were never limited")
		}
	})

	t.Run("on by default in HTTP mode, and ordinary traffic passes", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url)
		for i := range 15 {
			body := rateLimitedCall(t, srv).body
			if strings.Contains(strings.ToLower(body), "rate limit") {
				t.Fatalf("call %d was rate limited inside the default burst: %s", i, truncate(body))
			}
		}
		// The claim the previous version of this case got wrong. Asserted from
		// the server's own startup line, because fifteen served calls are
		// equally consistent with a limiter of ten and with no limiter at all.
		if awaitLog(t, srv, "rate limit enabled") == "" {
			t.Errorf("HTTP mode started with no rate limit; the default is 10 rps, burst 40:\n%s", srv.logs())
		}
	})
}

// TestLimit_RateLimitCoversTheSubscriptionMethods verifies, on the wire, that
// resources/subscribe and subscriptions/listen draw on the bucket a tools/call
// spends, and are refused with the JSON-RPC code that mirrors HTTP 429 once it
// is empty. The unit test cannot show either: the SDK's client discards a
// subscribe response, and a listen is a request it leaves open.
//
// Both are refused by the limiter before the handler that would otherwise
// answer them, so the refusal is what comes back even on a stateless
// deployment, where resources/subscribe is refused for a different reason one
// layer further in. The legacy method is sent as a 2025-11-25 request on
// purpose: under protocol 2026-07-28 the SDK answers it itself, as a method
// the revision removed, before any middleware runs.
func TestLimit_RateLimitCoversTheSubscriptionMethods(t *testing.T) {
	gitlab := acceptingGitLab(t)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--rate-limit-rps=0.001",
		"--rate-limit-burst=1",
	)

	// One token in the bucket, spent by a tool call. A refill of one per
	// thousand seconds keeps it empty for the rest of the test.
	if body := rateLimitedCall(t, srv).body; strings.Contains(body, "-42900") {
		t.Fatalf("the first call was refused; the bucket should hold one token: %s", truncate(body))
	}

	cases := []struct {
		method   string
		protocol string
		body     string
	}{
		{
			method:   "resources/subscribe",
			protocol: "2025-11-25",
			body:     `{"jsonrpc":"2.0","id":2,"method":"resources/subscribe","params":{"uri":"gitlab://projects/1"}}`,
		},
		{
			method:   "subscriptions/listen",
			protocol: "2026-07-28",
			body:     shutdownListenBody(3),
		},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp",
				body: tc.body,
				headers: map[string]string{
					"PRIVATE-TOKEN":        "glpat-x",
					"MCP-Protocol-Version": tc.protocol,
					"Mcp-Method":           tc.method,
				},
			})
			if !strings.Contains(got.body, "-42900") || !strings.Contains(strings.ToLower(got.body), "rate limit") {
				t.Fatalf("%s with an empty bucket was not refused by the limiter (status %d): %s", tc.method, got.status, truncate(got.body))
			}
			if !strings.Contains(got.body, tc.method) {
				t.Errorf("the refusal does not name %s: %s", tc.method, truncate(got.body))
			}
		})
	}
}

// TestLimit_RateLimitMetersTheCatalogListing verifies on the wire that
// tools/list draws on a bucket of its own: an empty tool-call bucket answers a
// listing anyway, and a client listing in a loop is refused with the code that
// mirrors HTTP 429.
//
// This one is charged for a reason none of the other metered methods share. It
// reaches no GitLab at all. It marshals the whole catalog, about 3.2 MB on the
// individual surface, which is the majority of that surface's processor time,
// so in a process serving many tenants one of them listing in a loop spends the
// processor the others are waiting for.
//
// Both halves have to be asserted here rather than in a unit test. The unit
// test drives a server it builds itself; this one drives the binary with the
// flags an operator passes, which is where the burst of one that the divided
// bucket has to survive comes from.
func TestLimit_RateLimitMetersTheCatalogListing(t *testing.T) {
	gitlab := acceptingGitLab(t)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--rate-limit-rps=0.001",
		"--rate-limit-burst=1",
	)

	// One token in each bucket and a refill of one per thousand seconds, so
	// whichever bucket is emptied here stays empty for the rest of the test.
	var drained bool
	for range 15 {
		if strings.Contains(strings.ToLower(rateLimitedCall(t, srv).body), "rate limit") {
			drained = true
			break
		}
	}
	if !drained {
		t.Fatal("15 tool calls against a bucket of one that does not refill were never limited")
	}

	// Discovery is still answered with the tool-call bucket empty. That is the
	// property the exemption used to provide, and the separate bucket keeps it.
	if body := listTools(t, srv); countTools(body) == 0 {
		t.Fatalf("a listing was refused by the bucket the tool calls emptied: %s", truncate(body))
	}

	// And the listing bucket is a bucket: the next listing finds it empty.
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: toolsListBody,
		headers: map[string]string{
			"PRIVATE-TOKEN": "glpat-x",
			"Mcp-Method":    "tools/list",
		},
	})
	if !strings.Contains(got.body, "-42900") || !strings.Contains(strings.ToLower(got.body), "rate limit") {
		t.Fatalf("a second listing against an empty catalog bucket was not refused (status %d): %s", got.status, truncate(got.body))
	}
	if !strings.Contains(got.body, "tools/list") {
		t.Errorf("the refusal does not name tools/list: %s", truncate(got.body))
	}
}

// TestLimit_RateLimitRefusalIsVisible verifies that a throttled deployment says
// so.
//
// Reproduced on the shipped default before this existed: 150 concurrent calls
// gave 102 refusals in 1.07s and the log carried nothing about any of them, at
// any LOG_LEVEL. The 48 served and the 102 refused were identical in it, so the
// one thing an operator needs from a throttled deployment, that it is
// throttled, was the one thing it could not report.
func TestLimit_RateLimitRefusalIsVisible(t *testing.T) {
	gitlab := acceptingGitLab(t)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--rate-limit-rps=1",
		"--rate-limit-burst=1",
	)
	for range 15 {
		if strings.Contains(strings.ToLower(rateLimitedCall(t, srv).body), "rate limit") {
			break
		}
	}

	logs := awaitLog(t, srv, "tool call refused: rate limit exceeded")
	if logs == "" {
		t.Fatalf("refusals left no trace in the log:\n%s", srv.logs())
	}
	if !strings.Contains(logs, `"reason":"rate_limited"`) {
		t.Errorf("the refusal carries no reason to group by:\n%s", logs)
	}
	// Self-suppressed: refusals are unbounded, so one line each would be a
	// flood. One line per window, carrying the count it stands for.
	if got := strings.Count(logs, "tool call refused: rate limit exceeded"); got != 1 {
		t.Errorf("%d refusal lines inside one suppression window, want 1", got)
	}
}

// TestLimit_IgnoreScopesSkipsDetection verifies that --ignore-scopes actually
// stops the scope probe rather than merely ignoring its result, which is the
// difference between saving a round trip and not.
func TestLimit_IgnoreScopesSkipsDetection(t *testing.T) {
	gitlab := acceptingGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--ignore-scopes")

	got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-noscopes"}))
	if got.status >= http.StatusInternalServerError {
		t.Fatalf("status = %d", got.status)
	}
	if strings.Contains(srv.logs(), "detected PAT scopes") {
		t.Error("scope detection ran despite --ignore-scopes")
	}
}

// awaitLog returns the server's output once it contains want, or "" if it never
// does.
//
// Reading the log immediately after the response that provoked it is a race,
// and one that only shows under load: the server writes to a pipe, and the
// parent's copier goroutine appends to the buffer this reads. Asserting on a
// log line therefore has to wait for the line rather than for the response. The
// case that needed this passed alone and failed inside the full suite under
// -shuffle, which is the shape of every such race.
func awaitLog(t *testing.T, srv *server, want string) string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if logs := srv.logs(); strings.Contains(logs, want) {
			return logs
		}
		time.Sleep(20 * time.Millisecond)
	}
	return ""
}

// awaitLogLine returns the one captured line containing want, or "" if none
// arrives.
//
// awaitLog answers "did this happen at all" by returning the whole of the
// server's output, which is the wrong answer to "what did that line say": every
// server the harness starts prints two deprecation warnings before it does
// anything, so a level or a field asserted against the whole output matches a
// line nobody was asking about and the assertion cannot fail. Scope such an
// assertion to the line the message is on.
func awaitLogLine(t *testing.T, srv *server, want string) string {
	t.Helper()

	logs := awaitLog(t, srv, want)
	for line := range strings.SplitSeq(logs, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	return ""
}
