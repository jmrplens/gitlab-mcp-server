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
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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
		map[string]string{"GITLAB_MCP_TOOL_SURFACE": "dynamic"}, "--gitlab-url="+gitlab.url)))
	meta := countTools(listTools(t, startServer(t,
		map[string]string{"GITLAB_MCP_TOOL_SURFACE": "meta"}, "--gitlab-url="+gitlab.url)))
	individual := countTools(listTools(t, startServer(t,
		map[string]string{"GITLAB_MCP_TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url)))

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
	srv := startServer(t, map[string]string{"GITLAB_MCP_EXCLUDE_TOOLS": victim}, "--gitlab-url="+gitlab.url)

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

// TestLimit_ExcludeTools_StandaloneByCatalogSpelling verifies, on the real
// binary, that a guided flow is removed by the two spellings the catalog
// accepts beside its tool name, on the two surfaces that register the flows
// as tools of their own.
//
// Those surfaces used to remove a flow by its registered name alone, so an
// operator who wrote the canonical action ID, the one spelling that means the
// same thing on every surface, or the group name the documentation offers,
// kept every flow listed and was not told (issue 911). A unit test composes
// the pass with registration; this is the binary, reading the variable from
// its environment.
func TestLimit_ExcludeTools_StandaloneByCatalogSpelling(t *testing.T) {
	gitlab := acceptingGitLab(t)

	cases := []struct {
		name    string
		surface string
		exclude string
		gone    []string
		kept    []string
	}{
		{
			name: "the canonical action ID on meta", surface: "meta", exclude: "interactive.issue_create",
			gone: []string{"gitlab_interactive_issue_create"},
			kept: []string{"gitlab_interactive_mr_create", "gitlab_discover_project"},
		},
		{
			name: "the group name on individual", surface: "individual", exclude: "gitlab_interactive",
			gone: []string{"gitlab_interactive_issue_create", "gitlab_interactive_mr_create", "gitlab_interactive_project_create", "gitlab_interactive_release_create"},
			kept: []string{"gitlab_discover_project"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := listTools(t, startServer(t, map[string]string{
				"GITLAB_MCP_TOOL_SURFACE":  tc.surface,
				"GITLAB_MCP_EXCLUDE_TOOLS": tc.exclude,
			}, "--gitlab-url="+gitlab.url))

			for _, name := range tc.gone {
				if strings.Contains(body, `"name":"`+name+`"`) {
					t.Errorf("%s is still advertised on %s after excluding %s", name, tc.surface, tc.exclude)
				}
			}
			for _, name := range tc.kept {
				if !strings.Contains(body, `"name":"`+name+`"`) {
					t.Errorf("%s is no longer advertised on %s, although excluding %s does not name it", name, tc.surface, tc.exclude)
				}
			}
		})
	}
}

// TestLimit_ReadOnlyRemovesMutations verifies that --read-only takes the
// mutating surface away rather than relying on the model not to ask.
func TestLimit_ReadOnlyRemovesMutations(t *testing.T) {
	gitlab := acceptingGitLab(t)

	full := listTools(t, startServer(t,
		map[string]string{"GITLAB_MCP_TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url))
	readOnly := listTools(t, startServer(t,
		map[string]string{"GITLAB_MCP_TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url, "--read-only"))

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
		map[string]string{"GITLAB_MCP_TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url)))
	safe := countTools(listTools(t, startServer(t,
		map[string]string{"GITLAB_MCP_TOOL_SURFACE": "individual"}, "--gitlab-url="+gitlab.url, "--safe-mode")))

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
		map[string]string{"GITLAB_MCP_CAPABILITY_SURFACE": "full"}, "--gitlab-url="+gitlab.url))
	minimal := listResources(startServer(t,
		map[string]string{"GITLAB_MCP_CAPABILITY_SURFACE": "minimal"}, "--gitlab-url="+gitlab.url))

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

// The listing bucket the whole process shares (register row RTC-007), as the
// binary is built with it. Literals, because this module drives a binary it
// did not build and cannot read a constant out of it; what catches them going
// stale is the drive below, which is sized from them and fails when the bucket
// refuses where they say it should not, or does not refuse where they say it
// must.
const (
	processListingRate  = 3000
	processListingBurst = 48000
)

// The two lines a refused listing can write, one per bucket. A client is
// answered the same words by both, so that it is not told other callers are
// listing (INV-019), and the log is the one place the two are told apart.
const (
	processListingRefusalLine    = "listing refused: rate limit exceeded across the process"
	credentialListingRefusalLine = "tool call refused: rate limit exceeded"
)

// listingWorkers is how many listings are in flight at once while a test
// drives the process's bucket. Enough that the server marshals on every core a
// runner has, which is what it takes to list faster than the bucket refills.
const listingWorkers = 8

// listingOutcome is what one tools/list met.
type listingOutcome int

const (
	listingServed listingOutcome = iota
	listingRefused
	listingFailed
)

// listAs performs one tools/list as token and says what it met, with the
// number of tools a served listing carried and a description of anything that
// was neither served nor refused. It never touches a *testing.T, so the
// workers that call it may run off the test goroutine.
func listAs(ctx context.Context, srv *server, token string) (outcome listingOutcome, tools int, detail string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/mcp", strings.NewReader(toolsListBody))
	if err != nil {
		return listingFailed, 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "tools/list")
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := srv.httpClient().Do(req)
	if err != nil {
		return listingFailed, 0, err.Error()
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	body := string(raw)
	switch {
	case err != nil:
		return listingFailed, 0, err.Error()
	case strings.Contains(body, "-42900") && strings.Contains(body, "rate limit exceeded for tools/list; retry after a short backoff"):
		return listingRefused, 0, ""
	case resp.StatusCode == http.StatusOK && countTools(body) > 0:
		return listingServed, countTools(body), ""
	}
	return listingFailed, 0, fmt.Sprintf("status %d: %s", resp.StatusCode, truncate(body))
}

// listingDrive is what a set of workers listing against one server saw.
type listingDrive struct {
	served, servedTools, refused, failed atomic.Int64
	firstFailure                         atomic.Value
}

// drive lists with listingWorkers workers spread over credentials tokens until
// stop says so or ctx ends, and returns what they met once every worker has
// stopped. A request cut short because ctx ended is not a failure.
func drive(ctx context.Context, srv *server, credentials int, stop func(*listingDrive) bool) *listingDrive {
	var d listingDrive
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	for w := range listingWorkers {
		token := fmt.Sprintf("glpat-lister-%d", w%credentials)
		workers.Go(func() {
			for ctx.Err() == nil {
				outcome, tools, detail := listAs(ctx, srv, token)
				switch outcome {
				case listingServed:
					d.served.Add(1)
					d.servedTools.Add(int64(tools))
				case listingRefused:
					d.refused.Add(1)
				case listingFailed:
					if ctx.Err() == nil {
						d.failed.Add(1)
						d.firstFailure.CompareAndSwap(nil, detail)
					}
				}
				if stop(&d) {
					cancel()
				}
			}
		})
	}
	workers.Wait()
	return &d
}

// processBudget is how many tools the process's bucket can have granted by
// elapsed: its burst and its refill since, and nothing beside them. Every
// listing is charged what it carries before it is answered, the first one
// included, because the server lists itself while it starts and the bucket
// learns the size from that listing, so no listing is admitted on a guess
// and settled afterwards.
func processBudget(elapsed time.Duration) float64 {
	return processListingBurst + processListingRate*elapsed.Seconds()
}

// TestLimit_ProcessBoundsListingAcrossCredentials verifies on the wire that
// the listing bucket keyed on the process (register row RTC-007) bounds what
// every credential together may list, and that it follows the credential's
// own listing bucket, so it is off when --rate-limit-rps is 0 (issue 951).
//
// The credential's bucket is the one a caller multiplies by minting tokens:
// keyed on the entry, N credentials held N buckets against one processor, and
// a listing on the individual surface marshals about three megabytes. Each
// credential here is given a listing bucket of ten thousand that never
// refills, so any refusal the drive meets can only be the process's, and a
// credential that has never listed is refused while the others are listing,
// which no bucket of its own could explain. The server's log says the same
// from its side: a refusal line naming the process and none naming a
// credential, since the client is answered the same words by either.
//
// It is not parallel: what it measures is the server listing faster than the
// bucket refills, which needs the runner's processors to itself. For the same
// reason it does not run under the race detector, whose instrumented server
// lists about nine times more slowly: a runner's cores then serve fewer tools
// a second than the bucket refills, so neither arm could show anything. The
// bucket's own code is held by internal/toolutil's tests, which do run under
// the detector.
func TestLimit_ProcessBoundsListingAcrossCredentials(t *testing.T) {
	if raceDetector {
		t.Skip("the race detector's server lists more slowly than the process's bucket refills, so neither arm can show anything on it")
	}
	gitlab := acceptingGitLab(t)
	individual := []string{"--gitlab-url=" + gitlab.url, "--tool-surface=individual", "--tier=free"}

	t.Run("credentials together are refused once the process's bucket is spent", func(t *testing.T) {
		srv := startServer(t, nil, append(individual, "--rate-limit-rps=0.001", "--rate-limit-burst=10000")...)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()

		started := time.Now()
		d, freshRefused := driveAskingAFreshCredential(ctx, srv)
		elapsed := time.Since(started)
		t.Logf("%d listings served (%d tools) and %d refused in %s",
			d.served.Load(), d.servedTools.Load(), d.refused.Load(), elapsed.Round(time.Millisecond))

		if d.failed.Load() > 0 {
			t.Fatalf("%d listings failed, the first with %v", d.failed.Load(), d.firstFailure.Load())
		}
		if d.refused.Load() == 0 {
			t.Fatalf("%d listings of %d tools in %s were never refused: the process bounds nothing",
				d.served.Load(), d.servedTools.Load(), elapsed)
		}
		if budget := processBudget(elapsed); float64(d.servedTools.Load()) > budget {
			t.Errorf("%d tools listed in %s, more than the %.0f the process's bucket grants in that time",
				d.servedTools.Load(), elapsed, budget)
		}
		if !freshRefused {
			t.Error("a credential that had never listed was served while the process's bucket was spent: the bucket is not the process's")
		}
		assertOnlyTheProcessRefused(t, srv)
	})

	t.Run("off when --rate-limit-rps is 0", func(t *testing.T) {
		srv := startServer(t, nil, append(individual, "--rate-limit-rps=0")...)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()

		// Listed until the tools served exceed what the process's bucket would
		// have granted in the time taken, so that a bucket left on would have
		// refused something. A host too slow to outlist the refill runs out of
		// time instead, and says so.
		started := time.Now()
		d := drive(ctx, srv, 4, func(d *listingDrive) bool {
			outlisted := float64(d.servedTools.Load()) > processBudget(time.Since(started))
			return outlisted || d.failed.Load() > 0 || d.refused.Load() > 0
		})
		elapsed := time.Since(started)
		t.Logf("%d listings served (%d tools) in %s with nothing refused",
			d.served.Load(), d.servedTools.Load(), elapsed.Round(time.Millisecond))

		if d.failed.Load() > 0 {
			t.Fatalf("%d listings failed, the first with %v", d.failed.Load(), d.firstFailure.Load())
		}
		if refused := d.refused.Load(); refused > 0 {
			t.Fatalf("%d listings were refused with the rate limit off", refused)
		}
		if budget := processBudget(elapsed); float64(d.servedTools.Load()) <= budget {
			t.Fatalf("only %d tools listed in %s, within the %.0f the process's bucket would have granted: "+
				"this host cannot list fast enough for the test to show the bucket is off", d.servedTools.Load(), elapsed, budget)
		}
	})
}

// driveAskingAFreshCredential drives the process's bucket with four
// credentials until it is spent, and then, while the workers keep it spent,
// lists as a credential that has never listed and so holds its whole own
// bucket. The fresh credential is asked beside the workers rather than by one
// of them, since a worker that stopped to ask would let the bucket refill under
// it, and the drive ends once it has answered. It reports what the workers met
// and whether the fresh credential was refused.
func driveAskingAFreshCredential(ctx context.Context, srv *server) (*listingDrive, bool) {
	var fresh sync.WaitGroup
	var freshAsked, freshAnswered, freshRefused atomic.Bool
	d := drive(ctx, srv, 4, func(d *listingDrive) bool {
		if d.refused.Load() > 0 && freshAsked.CompareAndSwap(false, true) {
			fresh.Go(func() {
				freshRefused.Store(freshCredentialRefused(ctx, srv))
				freshAnswered.Store(true)
			})
		}
		return d.failed.Load() > 0 || freshAnswered.Load()
	})
	fresh.Wait()
	return d, freshRefused.Load()
}

// assertOnlyTheProcessRefused holds the server's log to what a drive whose
// credentials' own buckets were never near empty must leave: the process's
// refusal line, naming its scope, and no credential's. The log is the one
// place the two are told apart, since a client is answered the same words by
// either.
func assertOnlyTheProcessRefused(t *testing.T, srv *server) {
	t.Helper()
	logs := awaitLog(t, srv, processListingRefusalLine)
	if logs == "" {
		t.Fatalf("the process's refusals left no line naming it:\n%s", srv.logs())
	}
	if !strings.Contains(logs, `"scope":"process"`) {
		t.Errorf("the process's refusal line does not name its scope:\n%s", logs)
	}
	if strings.Contains(logs, credentialListingRefusalLine) {
		t.Errorf("a credential's own listing bucket refused; the drive is not measuring the process's:\n%s", logs)
	}
}

// freshCredentialRefused lists once as a credential nothing has listed with,
// and reports whether it was refused, which with its own bucket full only the
// process's can do. A few tries, since the bucket refills between the workers'
// listings and one may slip through.
func freshCredentialRefused(ctx context.Context, srv *server) bool {
	for range 5 {
		if outcome, _, _ := listAs(ctx, srv, "glpat-never-listed"); outcome == listingRefused {
			return true
		}
	}
	return false
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
