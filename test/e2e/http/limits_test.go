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
		if strings.Contains(readOnly, mutating) {
			t.Errorf("a mutating tool matching %s survived --read-only", mutating)
		}
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
// configured, and that it is off by default.
//
// It is a per-server limit rather than a per-address one, so it protects the
// GitLab instance behind this server from a client looping rather than
// protecting this server from many clients — which is why the failure budget
// exists separately.
func TestLimit_RateLimitRPS(t *testing.T) {
	gitlab := acceptingGitLab(t)

	call := func(srv *server) response {
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

	t.Run("engages when configured", func(t *testing.T) {
		srv := startServer(t, nil,
			"--gitlab-url="+gitlab.url,
			"--rate-limit-rps=1",
			"--rate-limit-burst=1",
		)
		var limited bool
		for range 15 {
			body := call(srv).body
			if strings.Contains(strings.ToLower(body), "rate limit") || strings.Contains(body, "-42900") {
				limited = true
				break
			}
		}
		if !limited {
			t.Error("15 tool calls at --rate-limit-rps=1 --rate-limit-burst=1 were never limited")
		}
	})

	t.Run("off by default", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url)
		for i := range 15 {
			body := call(srv).body
			if strings.Contains(strings.ToLower(body), "rate limit") {
				t.Fatalf("call %d was rate limited with no limiter configured: %s", i, truncate(body))
			}
		}
	})
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
