// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// retry_later_test.go holds what issue 956 decided about the code a
// retry-later refusal carries, on the wire of the real binary: each refusal
// carries the code the standard assigns to what it says. One that tells the
// caller it made too many requests, or holds too many of something, is 429's
// -42900; one that says the server cannot serve the request right now is 503's
// -50300; and the legacy -32000, which MCP 2026-07-28 tells new
// implementations not to use and receivers not to read a meaning into, is
// answered by nothing.
//
// It also holds the tier half of the issue: an action the instance's licensing
// tier removed is answered, on the default surface, as existing and needing a
// higher tier, rather than as an unknown action with near-miss suggestions.
package httpe2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
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

// legacyCode is the code issue 956 retired from every refusal.
const legacyCode = "-32000"

// startThrottlingGitLab serves the startup probes and every numeric project,
// except throttledProject, which GitLab answers 429 with no reset header: the
// client retries it as it retries any 429 and then reports it, and the watcher
// reads that as GitLab's rate limit on the caller.
func startThrottlingGitLab(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone"}`))
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
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":%d,"name":"proj","path_with_namespace":"g/p","web_url":"http://example.invalid/g/p"}`, id)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// modernListen is a subscriptions/listen for every URI, as a 2026-07-28
// client sends it.
func modernListen(id int, uris ...string) string {
	const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}`
	quoted := make([]string, 0, len(uris))
	for _, uri := range uris {
		quoted = append(quoted, strconv.Quote(uri))
	}
	return fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"subscriptions/listen","params":{"notifications":{"resourceSubscriptions":[%s]},%s}}`,
		id, strings.Join(quoted, ","), meta,
	)
}

// projectURIs are count distinct project resources starting at first.
func projectURIs(first, count int) []string {
	uris := make([]string, 0, count)
	for i := range count {
		uris = append(uris, "gitlab://project/"+strconv.Itoa(first+i))
	}
	return uris
}

// refusalMessage reads the message of the JSON-RPC error a frame carries.
func refusalMessage(t *testing.T, frame string) string {
	t.Helper()
	var rpc struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(frame), &rpc); err != nil {
		t.Fatalf("the refusal is not a JSON-RPC message: %v: %s", err, frame)
	}
	return rpc.Error.Message
}

// assertRefusal holds one refusal to the code the standard assigns to what it
// says, to the words that say it, and to carrying the legacy code nowhere.
func assertRefusal(t *testing.T, got, code string, says ...string) {
	t.Helper()
	if !strings.Contains(got, `"code":`+code) {
		t.Errorf("the refusal does not carry %s:\n%s", code, got)
	}
	for _, want := range says {
		if !strings.Contains(got, want) {
			t.Errorf("the refusal does not say %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, legacyCode) {
		t.Errorf("the refusal carries the retired %s:\n%s", legacyCode, got)
	}
}

// TestRetryLater_ModernRefusals_CarryTheCodeOfWhatTheySay drives, at
// 2026-07-28, the refusals that say the caller made too many requests or holds
// too many of something: a listen past its caller's stream ceiling, a listen
// past its caller's watcher ceiling, a listen whose first read GitLab
// throttled, and a read past the caller's rate limit. Each carries -42900
// (issue 956). The process's own ceilings, which say the server cannot serve
// the request now and carry -50300, are driven by
// TestRetryLater_ServerCannotServe_CarriesTheCodeOfWhatItSays.
func TestRetryLater_ModernRefusals_CarryTheCodeOfWhatTheySay(t *testing.T) {
	gitlab := startThrottlingGitLab(t)

	t.Run("a listen past its caller's stream ceiling", func(t *testing.T) {
		srv := startServer(t, map[string]string{"GITLAB_MCP_MAX_LISTEN_STREAMS": "1"},
			"--gitlab-url="+gitlab.URL, "--capability-surface=full")
		held := openListen(t, srv, "glpat-whatever", "gitlab://project/900", 1)
		if _, seen, ok := held.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second); !ok {
			t.Fatalf("the first stream was never acknowledged; frames: %v", seen)
		}
		refused := openListen(t, srv, "glpat-whatever", "gitlab://project/901", 2)
		frame, seen, ok := refused.awaitFrame(t, "too many open subscriptions/listen streams", 20*time.Second)
		if !ok {
			t.Fatalf("a second stream was not refused past a ceiling of one; frames: %v", seen)
		}
		assertRefusal(t, frame, "-42900", "(per-credential limit 1); close one and retry")
	})

	t.Run("a listen past its caller's watcher ceiling", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full")
		frame := sseFrameContaining(t, srv, "subscriptions/listen", protocolVersion,
			modernListen(3, projectURIs(900, 11)...), "too many active subscriptions")
		assertRefusal(t, frame, "-42900", "limit 10; unsubscribe from one and retry")
	})

	t.Run("a listen whose first read GitLab throttled", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full")
		frame := sseFrameContaining(t, srv, "subscriptions/listen", protocolVersion,
			modernListen(4, "gitlab://project/"+strconv.Itoa(throttledProject)), "subscriptions: rate limited")
		assertRefusal(t, frame, "-42900")
		// GitLab's own words, read out of the message alone: the frame also
		// carries the code and the project's id, both of which spell 429.
		if message := refusalMessage(t, frame); !strings.Contains(message, "429 Too Many Requests") {
			t.Errorf("the refusal %q does not carry GitLab's 429 Too Many Requests", message)
		}
	})

	t.Run("a read past the caller's rate limit", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full",
			"--rate-limit-rps=0.001", "--rate-limit-burst=1")
		var refused string
		for id := 5; id < 20 && refused == ""; id++ {
			got := srv.do(t, readResource(id, "gitlab://project/900"))
			if strings.Contains(got.body, "rate limit exceeded") {
				refused = got.body
			}
		}
		if refused == "" {
			t.Fatal("fifteen reads against a bucket of one that does not refill were never refused")
		}
		assertRefusal(t, refused, "-42900", "rate limit exceeded for resources/read")
	})
}

// listChangedListen is a subscriptions/listen asking for list-changed
// notifications alone, as a 2026-07-28 client sends it: a stream with no
// watcher behind it.
func listChangedListen(id int) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"subscriptions/listen","params":{`+
		`"notifications":{"toolsListChanged":true},`+
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",`+
		`"io.modelcontextprotocol/clientCapabilities":{},`+
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}}}`, id)
}

// watcherCredentials is how many credentials it takes to fill the process's
// watcher cap ten at a time, the per-credential cap: the cap divided by ten,
// rounded up.
const watcherCredentials = (tenancy.WatchersPerProcess + tenancy.WatchersPerCredential - 1) / tenancy.WatchersPerCredential

// fillWatchers subscribes as many credentials as it takes to hold the
// process's whole watcher cap, each to as many of projects 900 to 909 as the
// remaining cap leaves it, through subscribe, and fails at the first refusal:
// every one of them is within both caps. Two credentials watching one URI hold
// one watcher each, which is why the same ten projects serve every credential.
func fillWatchers(t *testing.T, subscribe func(t *testing.T, token string, uris []string)) {
	t.Helper()
	left := tenancy.WatchersPerProcess
	for i := range watcherCredentials {
		count := min(left, tenancy.WatchersPerCredential)
		subscribe(t, fmt.Sprintf("glpat-watcher-%02d", i), projectURIs(900, count))
		left -= count
	}
}

// TestRetryLater_ServerCannotServe_CarriesTheCodeOfWhatItSays drives the
// refusals that say the server cannot serve the request right now, each
// carrying 503's -50300 (issue 956) in words of its own: the process's listen
// ceiling, reached by one credential once GITLAB_MCP_MAX_LISTEN_STREAMS=0
// removes the credential's own (it removes that one and nothing else), and the
// process's watcher cap, which takes as many credentials as it takes to hold
// 512 watchers ten at a time, at 2026-07-28 and at 2025-11-25. The rate limit
// is off so no credential's bucket refuses first. The held-call ceiling's
// in-band refusal is driven in held_test.go and the readiness gate's in the
// stdio module: on HTTP a request the catalog's build outlasts is answered only
// to a stateful session's notifications/cancelled, through the same gate and
// the same function, while the default stateless transport ends it with its
// POST, leaving no stream to carry an answer. A subscribe during shutdown
// meets a closed manager only in the instant between an entry's eviction, or
// the process's close, and a request already dispatched on it, which nothing
// outside the process can place, so that one is held by the unit tests of
// cmd/server.
func TestRetryLater_ServerCannotServe_CarriesTheCodeOfWhatItSays(t *testing.T) {
	gitlab := startThrottlingGitLab(t)

	t.Run("a listen past the process's stream ceiling", func(t *testing.T) {
		srv := startServer(t, map[string]string{"GITLAB_MCP_MAX_LISTEN_STREAMS": "0"},
			"--gitlab-url="+gitlab.URL, "--capability-surface=full", "--rate-limit-rps=0")
		for id := 1; id <= tenancy.ListenStreamsPerProcess; id++ {
			stream := openListenBody(t, srv, "glpat-whatever", listChangedListen(id))
			if _, seen, ok := stream.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second); !ok {
				t.Fatalf("stream %d of %d, within the process's ceiling, was never acknowledged; frames: %v",
					id, tenancy.ListenStreamsPerProcess, seen)
			}
		}
		frame := sseFrameContaining(t, srv, "subscriptions/listen", protocolVersion,
			listChangedListen(tenancy.ListenStreamsPerProcess+1), "subscriptions/listen stream now")
		assertRefusal(t, frame, "-50300", fmt.Sprintf(
			"the server cannot open another subscriptions/listen stream now (server-wide limit %d); retry later",
			tenancy.ListenStreamsPerProcess,
		))
	})

	watcherRefusal := fmt.Sprintf("the server cannot watch another resource now: server-wide, %d watching, limit %d; retry later",
		tenancy.WatchersPerProcess, tenancy.WatchersPerProcess)

	t.Run("a listen past the process's watcher cap", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full", "--rate-limit-rps=0")
		fillWatchers(t, func(t *testing.T, token string, uris []string) {
			t.Helper()
			stream := openListenBody(t, srv, token, modernListen(1, uris...))
			if _, seen, ok := stream.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second); !ok {
				t.Fatalf("%s's listen to %d resources, within both caps, was never acknowledged; frames: %v", token, len(uris), seen)
			}
		})
		refused := openListenBody(t, srv, "glpat-watcher-past", modernListen(2, "gitlab://project/900"))
		frame, seen, ok := refused.awaitFrame(t, "the server cannot watch another resource now", 20*time.Second)
		if !ok {
			t.Fatalf("a listen past the process's %d watchers was not refused; frames: %v", tenancy.WatchersPerProcess, seen)
		}
		assertRefusal(t, frame, "-50300", watcherRefusal)
	})

	t.Run("a subscribe past the process's watcher cap", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full", "--rate-limit-rps=0", "--stateless=false")
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		t.Cleanup(cancel)
		client := srv.httpClient()
		subscribe := func(t *testing.T, token string, uris []string) response {
			t.Helper()
			session := openTokenSession(ctx, t, client, srv.baseURL, token)
			var last response
			for i, uri := range uris {
				got, err := tokenPost(ctx, client, srv.baseURL, token,
					`{"jsonrpc":"2.0","id":`+strconv.Itoa(10+i)+`,"method":"resources/subscribe","params":{"uri":`+strconv.Quote(uri)+`}}`,
					sessionHeader(session))
				if err != nil {
					t.Fatalf("%s subscribing to %s: %v", token, uri, err)
				}
				last = got
			}
			return last
		}
		fillWatchers(t, func(t *testing.T, token string, uris []string) {
			t.Helper()
			if got := subscribe(t, token, uris); strings.Contains(got.body, `"error"`) {
				t.Fatalf("%s's subscribes to %d resources, within both caps, were refused: %s", token, len(uris), got.body)
			}
		})
		assertRefusal(t, subscribe(t, "glpat-watcher-past", projectURIs(900, 1)).body, "-50300", watcherRefusal)
	})
}

// openTokenSession initializes a stateful session as token and completes its
// handshake, failing the test if the server refuses either.
func openTokenSession(ctx context.Context, t *testing.T, client *http.Client, baseURL, token string) string {
	t.Helper()
	opened, err := tokenPost(ctx, client, baseURL, token, sessionInitialize(1), sessionHeader(""))
	session := opened.header.Get("Mcp-Session-Id")
	if err != nil || opened.status != http.StatusOK || session == "" {
		t.Fatalf("opening a session as %s: %v (status %d, body %s)", token, err, opened.status, opened.body)
	}
	if _, err = tokenPost(ctx, client, baseURL, token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, sessionHeader(session)); err != nil {
		t.Fatalf("completing the handshake as %s: %v", token, err)
	}
	return session
}

// tokenPost is heldPost as a credential of the caller's choosing, for a test
// that needs many of them.
func tokenPost(ctx context.Context, client *http.Client, baseURL, token, body string, header http.Header) (response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return response{}, err
	}
	maps.Copy(req.Header, header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return response{status: resp.StatusCode, header: resp.Header, body: string(raw)}, err
}

// TestRetryLater_LegacyRefusals_CarryTheCodeOfWhatTheySay drives the same
// refusals at 2025-11-25, where a subscription is resources/subscribe on a
// stateful session: a subscribe past the caller's watcher ceiling and one whose
// first read GitLab throttled. Both carry -42900, as their 2026-07-28
// counterparts do, since what each says is the same in either revision.
func TestRetryLater_LegacyRefusals_CarryTheCodeOfWhatTheySay(t *testing.T) {
	gitlab := startThrottlingGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full", "--stateless=false")
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	client := srv.httpClient()

	subscribe := func(t *testing.T, session string, id int, uri string) response {
		t.Helper()
		got, err := heldPost(ctx, client, srv.baseURL,
			`{"jsonrpc":"2.0","id":`+strconv.Itoa(id)+`,"method":"resources/subscribe","params":{"uri":`+strconv.Quote(uri)+`}}`,
			sessionHeader(session))
		if err != nil {
			t.Fatalf("subscribing to %s: %v", uri, err)
		}
		return got
	}

	// One credential, so both sessions share its watchers: the throttled
	// subscribe goes first, while the credential holds none, and the ceiling
	// is filled after it.
	session, opened, err := openSession(ctx, client, srv.baseURL, 1)
	if err != nil || session == "" {
		t.Fatalf("opening a session: %v (status %d, body %s)", err, opened.status, opened.body)
	}

	t.Run("a subscribe whose first read GitLab throttled", func(t *testing.T) {
		assertRefusal(t, subscribe(t, session, 30, "gitlab://project/"+strconv.Itoa(throttledProject)).body, "-42900",
			"subscriptions: rate limited")
	})

	t.Run("a subscribe past its caller's watcher ceiling", func(t *testing.T) {
		for i, uri := range projectURIs(900, 10) {
			if got := subscribe(t, session, 10+i, uri); strings.Contains(got.body, `"error"`) {
				t.Fatalf("subscribe %d of 10, within the ceiling, was refused: %s", i+1, got.body)
			}
		}
		assertRefusal(t, subscribe(t, session, 20, "gitlab://project/910").body, "-42900",
			"too many active subscriptions", "limit 10; unsubscribe from one and retry")
	})
}

// TestTier_AnActionTheTierRemoved_IsAnsweredWithTheTierItNeeds calls a Premium
// action on the default surface of a Free instance. It used to be answered
// "unknown action" with near-miss suggestions, since the tier filter runs
// before the catalog is built and so never reached the bookkeeping the scope
// and operator narrowings feed; a model reading that concludes the server
// cannot do the thing. Issue 956 answers it as existing and needing a higher
// tier, which only the operator can change: where the tier was pinned the
// answer says so, and where it was detected it allows that the detection may
// be wrong.
func TestTier_AnActionTheTierRemoved_IsAnsweredWithTheTierItNeeds(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	for _, tc := range []struct {
		name  string
		flags []string
		says  string
	}{
		{name: "pinned", flags: []string{"--tier=free"}, says: "this deployment is configured for GitLab Free"},
		{name: "detected", says: "this instance's tier was detected as GitLab Free"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := startServer(t, nil, append([]string{"--gitlab-url=" + gitlab.url}, tc.flags...)...)
			got := srv.do(t, executeAction(1, "merge_train.list_project", `{"project_id":"1"}`)).body
			for _, want := range []string{
				`merge_train.list_project\" exists but is not available`, "it needs GitLab Premium or Ultimate", tc.says,
				"ask the operator", "GITLAB_MCP_TIER",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("the answer does not say %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "unknown action") || strings.Contains(got, "Did you mean") {
				t.Errorf("the answer calls a Premium action unknown on a Free instance:\n%s", got)
			}
		})
	}
}
