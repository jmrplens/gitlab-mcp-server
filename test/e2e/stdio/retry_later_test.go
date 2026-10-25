//go:build stdioe2e

// retry_later_test.go holds, on stdio, what issue 956 decided about the code a
// retry-later refusal carries: each carries the code the standard assigns to
// what it says, 429's -42900 where the caller made too many requests or holds
// too many of something and 503's -50300 where the server cannot serve the
// request right now, and none carries the legacy -32000 any longer.
//
// Two of the -50300 refusals are reachable on stdio and driven here: the
// process's listen ceiling, which GITLAB_MCP_MAX_LISTEN_STREAMS=0 leaves the
// only ceiling a listen meets, and the readiness gate answering a request the
// catalog's build outlasted. The other three are not. The process's watcher
// cap counts every credential's watchers, and a stdio process serves one
// credential held to ten. A subscribe during shutdown meets a closed manager
// only in the instant between the manager closing as the session ends and a
// request already dispatched, which nothing outside the process can place.
// The held-call ceiling (HLD-011) counts calls arriving on HTTP POSTs, which
// stdio has none of. Those three are held by the unit tests of cmd/server and
// internal/subscriptions.
//
// It also holds the tier half of the issue: an action the instance's licensing
// tier removed is answered as existing and needing a higher tier rather than
// as an unknown action.
package stdioe2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// throttledProject is the one project the stand-in GitLab answers 429 for,
// which is how a watcher's first read meets GitLab's rate limit.
const throttledProject = 429

// throttlingGitLab serves the startup probes and every numeric project, except
// throttledProject, which it answers 429 with no reset header: the client
// retries it as it retries any 429 and then reports it, and the watcher reads
// that as GitLab's rate limit on the caller.
func throttlingGitLab(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"version":"17.0.0","revision":"abcdef"}`)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":7,"username":"someone","name":"Some One"}`)
	})
	mux.HandleFunc("/api/v4/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		switch {
		case err != nil:
			w.WriteHeader(http.StatusNotFound)
		case id == throttledProject:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"429 Too Many Requests"}`))
		default:
			writeJSON(w, fmt.Sprintf(`{"id":%d,"name":"proj","path_with_namespace":"g/p","web_url":"http://example.invalid/g/p"}`, id))
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// resourceURIs are count distinct project resources starting at first, as a
// JSON array.
func resourceURIs(first, count int) string {
	uris := make([]string, 0, count)
	for i := range count {
		uris = append(uris, strconv.Quote("gitlab://project/"+strconv.Itoa(first+i)))
	}
	return "[" + strings.Join(uris, ",") + "]"
}

// listenPastTheFirstStream opens one listen, waits for its acknowledgement,
// and returns the answer to a second one, the listen a ceiling of one stream
// refuses.
func listenPastTheFirstStream(t *testing.T, s *session) (string, map[string]any) {
	t.Helper()
	s.send(t, request(1, "subscriptions/listen", `{"notifications":{"resourceSubscriptions":["gitlab://project/900"]}}`))
	deadline := time.Now().Add(callTimeout)
	for acknowledged := false; !acknowledged; {
		if time.Now().After(deadline) {
			t.Fatalf("the first listen was never acknowledged\nstderr: %s", s.stderrText())
		}
		method, _ := s.readMessage(t, callTimeout)["method"].(string)
		acknowledged = method == "notifications/subscriptions/acknowledged"
	}
	return s.callRaw(t, request(2, "subscriptions/listen", `{"notifications":{"resourceSubscriptions":["gitlab://project/901"]}}`))
}

// assertStdioRefusal holds one answer to an in-band refusal carrying code and
// every one of says, and to carrying the retired -32000 nowhere on the line.
func assertStdioRefusal(t *testing.T, line string, got map[string]any, code int, says ...string) {
	t.Helper()
	rpcErr, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("the answer is not a refusal: %v", got)
	}
	if gotCode, _ := rpcErr["code"].(float64); int(gotCode) != code {
		t.Errorf("refused with code %v, want %d: %v", rpcErr["code"], code, rpcErr)
	}
	message, _ := rpcErr["message"].(string)
	for _, want := range says {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal %q does not say %q", message, want)
		}
	}
	if strings.Contains(line, "-32000") {
		t.Errorf("the refusal carries the retired -32000: %s", line)
	}
}

// TestRetryLater_ModernRefusals_CarryTheCodeOfWhatTheySay drives, at
// 2026-07-28 on stdio, the refusals that say the caller holds too many or made
// too many requests: a listen past the credential's stream ceiling, a listen
// past the watcher ceiling, and a listen whose first read GitLab throttled.
// Each carries -42900 (issue 956). The rate limit's refusal, which says the
// same and carries the same code, is held in rate_limit_test.go, and the
// refusals that say the server cannot serve the request now are held by
// TestRetryLater_ServerCannotServe_CarriesTheCodeOfWhatItSays.
func TestRetryLater_ModernRefusals_CarryTheCodeOfWhatTheySay(t *testing.T) {
	gitlab := throttlingGitLab(t)

	t.Run("a listen past the stream ceiling", func(t *testing.T) {
		env := baseEnv(gitlab)
		env["GITLAB_MCP_CAPABILITY_SURFACE"] = "full"
		env["GITLAB_MCP_MAX_LISTEN_STREAMS"] = "1"
		s := startSession(t, env)
		line, got := listenPastTheFirstStream(t, s)
		assertStdioRefusal(t, line, got, tenancy.CodeTooManyRequests,
			"too many open subscriptions/listen streams (per-credential limit 1); close one and retry")
	})

	for _, tc := range []struct {
		name string
		uris string
		says []string
	}{
		{
			name: "a listen past the watcher ceiling", uris: resourceURIs(900, 11),
			says: []string{"too many active subscriptions", "limit 10; unsubscribe from one and retry"},
		},
		{
			name: "a listen whose first read GitLab throttled", uris: resourceURIs(throttledProject, 1),
			says: []string{"subscriptions: rate limited"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(gitlab)
			env["GITLAB_MCP_CAPABILITY_SURFACE"] = "full"
			s := startSession(t, env)
			line, got := s.callRaw(t, request(3, "subscriptions/listen", `{"notifications":{"resourceSubscriptions":`+tc.uris+`}}`))
			assertStdioRefusal(t, line, got, tenancy.CodeTooManyRequests, tc.says...)
		})
	}
}

// listChangedListen is a subscriptions/listen asking for list-changed
// notifications alone, which opens a stream and no watcher.
const listChangedListen = `{"notifications":{"toolsListChanged":true}}`

// TestRetryLater_ServerCannotServe_CarriesTheCodeOfWhatItSays drives, on
// stdio, the two refusals that say the server cannot serve the request right
// now and that a stdio process can reach, each carrying 503's -50300 (issue
// 956). The first is the process's listen ceiling: with the credential's
// ceiling removed (GITLAB_MCP_MAX_LISTEN_STREAMS=0, which removes that one
// and nothing else) one session fills the process's 512 streams, and the next
// listen is refused in words of its own. The second is the readiness gate: a
// tools/list sent while the catalog is still being prepared, whose client
// cancels it before the catalog is ready, is answered rather than dropped,
// since the SDK writes a response even to a cancelled request. The instance
// holds the version the startup asks first, so the catalog cannot be ready
// before the cancellation arrives.
func TestRetryLater_ServerCannotServe_CarriesTheCodeOfWhatItSays(t *testing.T) {
	t.Run("a listen past the process's stream ceiling", func(t *testing.T) {
		env := baseEnv(throttlingGitLab(t))
		env["GITLAB_MCP_CAPABILITY_SURFACE"] = "full"
		env["GITLAB_MCP_MAX_LISTEN_STREAMS"] = "0"
		s := startSession(t, env)
		for id := 1; id <= tenancy.ListenStreamsPerProcess; id++ {
			s.send(t, request(id, "subscriptions/listen", listChangedListen))
		}
		deadline := time.Now().Add(callTimeout)
		for acknowledged := 0; acknowledged < tenancy.ListenStreamsPerProcess; {
			if time.Now().After(deadline) {
				t.Fatalf("%d of %d listens were acknowledged\nstderr: %s", acknowledged, tenancy.ListenStreamsPerProcess, s.stderrText())
			}
			got := s.readMessage(t, callTimeout)
			if got["error"] != nil {
				t.Fatalf("a listen within the process's ceiling was refused: %v", got)
			}
			if method, _ := got["method"].(string); method == "notifications/subscriptions/acknowledged" {
				acknowledged++
			}
		}
		line, got := s.callRaw(t, request(tenancy.ListenStreamsPerProcess+1, "subscriptions/listen", listChangedListen))
		assertStdioRefusal(t, line, got, tenancy.CodeUnavailable, fmt.Sprintf(
			"the server cannot open another subscriptions/listen stream now (server-wide limit %d); retry later",
			tenancy.ListenStreamsPerProcess,
		))
	})

	t.Run("a request the catalog's build outlasted", func(t *testing.T) {
		fake := startFakeGitLab(t)
		fake.versionHold = make(chan struct{})
		t.Cleanup(func() { close(fake.versionHold) })
		s := startSession(t, baseEnv(fake.URL))
		s.call(t, legacyInitialize(1))
		s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		s.send(t, legacyRequest(2, "tools/list", ""))
		line, got := s.callRaw(t, `{"jsonrpc":"2.0","id":3,"method":"ping"}`)
		if got["error"] != nil {
			t.Fatalf("a ping while the catalog is prepared was refused: %s", line)
		}
		if logs := s.stderrText(); strings.Contains(logs, catalogReadyLine) {
			t.Fatalf("the catalog was ready before the listing was cancelled, so the case proves nothing:\n%s", logs)
		}
		s.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2,"reason":"gave up"}}`)
		for {
			line, got = s.readRawMessage(t, callTimeout)
			if _, isNotification := got["method"]; !isNotification || got["id"] != nil {
				break
			}
		}
		if !sameID(got["id"], float64(2)) {
			t.Fatalf("the next response answers id %v, want the cancelled listing's 2: %s", got["id"], line)
		}
		assertStdioRefusal(t, line, got, tenancy.CodeUnavailable,
			"the server is still building its tool catalog and this request ended before it was ready; retry it")
	})
}

// TestRetryLater_LegacyRefusals_CarryTheCodeOfWhatTheySay drives the same
// refusals at 2025-11-25 on stdio, where a subscription is resources/subscribe
// after initialize: a subscribe past the watcher ceiling and one whose first
// read GitLab throttled, each carrying -42900 as its 2026-07-28 counterpart
// does.
func TestRetryLater_LegacyRefusals_CarryTheCodeOfWhatTheySay(t *testing.T) {
	env := baseEnv(throttlingGitLab(t))
	env["GITLAB_MCP_CAPABILITY_SURFACE"] = "full"
	s := startSession(t, env)
	s.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"stdio-e2e","version":"1"}}}`)
	s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	subscribe := func(t *testing.T, id int, uri string) (string, map[string]any) {
		t.Helper()
		params, err := json.Marshal(map[string]string{"uri": uri})
		if err != nil {
			t.Fatalf("encoding the subscribe: %v", err)
		}
		return s.callRaw(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"resources/subscribe","params":%s}`, id, params))
	}

	t.Run("a subscribe whose first read GitLab throttled", func(t *testing.T) {
		line, got := subscribe(t, 2, "gitlab://project/"+strconv.Itoa(throttledProject))
		assertStdioRefusal(t, line, got, tenancy.CodeTooManyRequests, "subscriptions: rate limited")
	})

	t.Run("a subscribe past the watcher ceiling", func(t *testing.T) {
		for i := range 10 {
			if _, got := subscribe(t, 10+i, "gitlab://project/"+strconv.Itoa(900+i)); got["error"] != nil {
				t.Fatalf("subscribe %d of 10, within the ceiling, was refused: %v", i+1, got["error"])
			}
		}
		line, got := subscribe(t, 20, "gitlab://project/910")
		assertStdioRefusal(t, line, got, tenancy.CodeTooManyRequests,
			"too many active subscriptions", "limit 10; unsubscribe from one and retry")
	})
}

// TestTier_AnActionTheTierRemoved_IsAnsweredWithTheTierItNeeds calls a Premium
// action on the default surface of a Free instance over stdio. It used to be
// answered "unknown action" with near-miss suggestions; issue 956 answers it as
// existing and needing a higher tier, which only the operator can change,
// saying whether the tier was pinned or detected.
func TestTier_AnActionTheTierRemoved_IsAnsweredWithTheTierItNeeds(t *testing.T) {
	for _, tc := range []struct {
		name string
		tier string
		says string
	}{
		{name: "pinned", tier: "free", says: "this deployment is configured for GitLab Free"},
		{name: "detected", says: "this instance's tier was detected as GitLab Free"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			if tc.tier != "" {
				env["GITLAB_MCP_TIER"] = tc.tier
			}
			s := startSession(t, env)
			got := s.call(t, request(1, "tools/call",
				`{"name":"gitlab_execute_action","arguments":{"action":"merge_train.list_project","params":{"project_id":"1"}}}`))
			result, _ := got["result"].(map[string]any)
			content, _ := result["content"].([]any)
			if result["isError"] != true || len(content) == 0 {
				t.Fatalf("the call was not refused as a tool error: %v", got)
			}
			first, _ := content[0].(map[string]any)
			text, _ := first["text"].(string)
			for _, want := range []string{
				`"merge_train.list_project" exists but is not available`, "it needs GitLab Premium or Ultimate", tc.says,
				"ask the operator", "GITLAB_MCP_TIER",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("the answer %q does not say %q", text, want)
				}
			}
			if strings.Contains(text, "unknown action") {
				t.Errorf("the answer calls a Premium action unknown on a Free instance: %q", text)
			}
		})
	}
}
