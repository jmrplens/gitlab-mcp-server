package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/gatewaycompat"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/toolvisibility"
)

// withheldPrefix is the stable text a phase A refusal carries after the
// action it names (register row AUT-007).
const withheldPrefix = "exists but is not available to a fine-grained personal access token"

// individualFineGrainedShell builds and registers an individual-surface server
// for client, with the tool sets the fine-grained checks read published.
func individualFineGrainedShell(t *testing.T, client *gitlabclient.Client, cfg *config.ServerConfig) *serverShell {
	t.Helper()
	cfg.ToolSurface = config.ToolSurfaceIndividual
	shell, err := newServerShell(t.Context(), client, cfg)
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}
	if registerErr := shell.register(t.Context()); registerErr != nil {
		t.Fatalf("register: %v", registerErr)
	}
	shell.gate.markReady()
	return shell
}

// phaseAAuthority is a fine-grained token's authority over the generated table
// with its grant not evaluated and nothing to say why: what no fine-grained
// token reaches is withheld, and the rest is served.
func phaseAAuthority() *finegrained.Authority {
	return finegrained.Unevaluated(actiongrants.Table(), finegrained.FallbackNone, "")
}

// withheldTools names the registered tools a phase A authority lists none of
// the actions of, which is what a fine-grained listing must leave out.
func withheldTools(t *testing.T, shell *serverShell) []string {
	t.Helper()
	authority := phaseAAuthority()
	var names []string
	for name := range shell.toolActions.Load().ActionIDs() {
		if !shell.toolActions.Load().Listed(authority, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		t.Fatal("no registered tool is withheld from a fine-grained token; the generated table denies nothing this surface registers")
	}
	return names
}

// listedNames lists a session's tools by name.
func listedNames(t *testing.T, session *mcp.ClientSession) ([]string, []*mcp.Tool) {
	t.Helper()
	result, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names, result.Tools
}

// assertFinalized holds every tool of a listing to the schema the server
// finalizes on it: properties closed, page bounded, and the operator's
// description substitution applied.
func assertFinalized(t *testing.T, label string, tools []*mcp.Tool) {
	t.Helper()
	paged := 0
	for _, tool := range tools {
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("%s: %s input schema is %T", label, tool.Name, tool.InputSchema)
		}
		if closed, isBool := schema["additionalProperties"].(bool); !isBool || closed {
			t.Errorf("%s: %s additionalProperties = %v, want false", label, tool.Name, schema["additionalProperties"])
		}
		if properties, hasProperties := schema["properties"].(map[string]any); hasProperties {
			if page, hasPage := properties["page"].(map[string]any); hasPage {
				paged++
				if page["minimum"] == nil {
					t.Errorf("%s: %s page carries no minimum", label, tool.Name)
				}
			}
		}
		if strings.Contains(tool.Description, "GitLab") {
			t.Errorf("%s: %s description still says GitLab after the substitution", label, tool.Name)
		}
	}
	if paged == 0 {
		t.Errorf("%s: no listed tool takes a page, so the bound was not held to anything", label)
	}
}

// TestCreateServer_FineGrained_EveryListingNarrowedAndFinalized verifies on one
// server that a fine-grained session's listings leave out every tool it may
// run none of the actions of, and only those, and that every listing, the
// first and the next and a classic one after them, carries the schemas the
// server finalizes, with an operator's description substitution active too:
// the classic listing that follows lists the tools the narrow ones left out,
// locked down and bounded all the same. What it holds is the result, not the
// filter's place in the chain: the schema lockdown and the pagination bounds
// finalize the tools on the first listing they see, which is the server's own
// at registration, and the filter never narrows that one, so no client's
// listing decides it wherever the filter sits relative to them.
func TestCreateServer_FineGrained_EveryListingNarrowedAndFinalized(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "GitLab=Gitlab")
	client := newMockGitLabClient(t)
	client.SetAuthority(phaseAAuthority())
	shell := individualFineGrainedShell(t, client, &config.ServerConfig{})
	withheld := withheldTools(t, shell)

	// A session per listing: a client keeps a listing for the cache lifetime
	// its hints give it, so a second listing on one session is the first
	// served again rather than another the server answered.
	// sequential: the second listing is the one served after the first.
	for _, listing := range []string{"first", "second"} {
		names, tools := listedNames(t, newInMemorySession(t, shell.server))
		for _, name := range withheld {
			if slices.Contains(names, name) {
				t.Errorf("%s listing: %s is listed to a fine-grained session that may run none of its actions", listing, name)
			}
		}
		assertFinalized(t, listing+" listing", tools)
	}

	client.SetAuthority(nil)
	names, tools := listedNames(t, newInMemorySession(t, shell.server))
	for _, name := range withheld {
		if !slices.Contains(names, name) {
			t.Errorf("classic listing: %s is missing", name)
		}
	}
	assertFinalized(t, "classic listing", tools)
}

// callText calls a tool and returns the text of its answer, or of its error.
func callText(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) string {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return err.Error()
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

// TestCreateServer_FineGrained_WithheldCallIsAnsweredBeforeValidationAndSpendsItsToken
// verifies a fine-grained session's call to a tool it may not run is answered
// with the reason whatever its arguments, arguments the SDK's validation would
// refuse included, and that it spends its token of the credential's rate bucket
// like every other refused call: two withheld calls drain a bucket of two, and
// the next call, to a tool the session may run, is refused for the rate.
func TestCreateServer_FineGrained_WithheldCallIsAnsweredBeforeValidationAndSpendsItsToken(t *testing.T) {
	client := newMockGitLabClient(t)
	client.SetAuthority(phaseAAuthority())
	shell := individualFineGrainedShell(t, client, &config.ServerConfig{RateLimitRPS: 0.0001, RateLimitBurst: 2})
	withheld := withheldTools(t, shell)[0]
	session := newInMemorySession(t, shell.server)
	served := ""
	for name := range shell.toolActions.Load().ActionIDs() {
		if shell.toolActions.Load().Listed(client.Authority(), name) {
			served = name
			break
		}
	}

	// sequential: each call spends one token of a bucket of two.
	for _, arguments := range []map[string]any{{"not_a_parameter": map[string]any{"deep": []any{1}}}, {}} {
		if text := callText(t, session, withheld, arguments); !strings.Contains(text, withheldPrefix) {
			t.Errorf("call %s(%v) = %q, want the withheld answer", withheld, arguments, text)
		}
	}
	if text := callText(t, session, served, nil); !strings.Contains(text, "rate limit exceeded") {
		t.Errorf("third call, to %s = %q, want it refused for the rate: the two withheld calls should have spent the bucket", served, text)
	}
}

// TestPrepareStdioCatalog_FineGrainedToken_AListingParkedAtStartupIsAlreadyNarrowed
// verifies the stdio start attaches a fine-grained token's authority after
// registration and before the readiness gate opens: a tools/list the client
// sent while the catalog was still being prepared, which the gate held, is
// answered narrowed, since a client may keep that first listing for the
// listing's whole cache lifetime.
func TestPrepareStdioCatalog_FineGrainedToken_AListingParkedAtStartupIsAlreadyNarrowed(t *testing.T) {
	listingSent := make(chan struct{})
	var sentOnce atomic.Bool
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			<-listingSent
			testutil.RespondJSON(w, http.StatusOK, `{"version":"19.4.1-ee","revision":"abc"}`)
		case "/api/v4/user":
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"username":"testuser"}`)
		case "/api/v4/personal_access_tokens/self":
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"t","active":true,"scopes":["granular"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gitlab.Close)
	cfg := &config.Config{
		GitLabURL: gitlab.URL, GitLabToken: testToken, ToolSurface: config.ToolSurfaceIndividual,
		TierExplicit: true, DisableRetries: true,
	}
	client, serverCfg, shell := newStdioStartupShell(t, cfg)
	shell.server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" && sentOnce.CompareAndSwap(false, true) {
				close(listingSent)
			}
			return next(ctx, method, req)
		}
	})
	st, ct := mcp.NewInMemoryTransports()
	if _, err := shell.server.Connect(withReadinessGate(t.Context()), st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	// The handshake must not wait for the catalog, which is prepared only after
	// it; bounded, so a check that made it wait fails here instead of hanging.
	connectCtx, cancel := context.WithTimeout(t.Context(), gateReach)
	t.Cleanup(cancel)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil).Connect(connectCtx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	type listed struct {
		names []string
		err   error
	}
	// The listing waits through the whole catalog preparation, which under the
	// race detector takes most of gateReach on its own, so it is sent under the
	// test's context and bounded only once the preparation has returned.
	listCtx := t.Context()
	parked := make(chan listed, 1)
	go func() {
		result, listErr := session.ListTools(listCtx, nil)
		var names []string
		if result != nil {
			for _, tool := range result.Tools {
				names = append(names, tool.Name)
			}
		}
		parked <- listed{names: names, err: listErr}
	}()
	if prepErr := prepareStdioCatalog(t.Context(), client, cfg, serverCfg, shell, &deferredIdentity{}); prepErr != nil {
		t.Fatalf("prepareStdioCatalog: %v", prepErr)
	}
	if client.Authority() == nil {
		t.Fatal("the start attached no authority to a fine-grained token's client")
	}
	answerCtx, cancelAnswer := context.WithTimeout(t.Context(), gateReach)
	defer cancelAnswer()
	var got listed
	select {
	case got = <-parked:
	case <-answerCtx.Done():
		t.Fatalf("the listing parked at startup was not answered within %s of the catalog being prepared", gateReach)
	}
	if got.err != nil {
		t.Fatalf("the parked listing failed: %v", got.err)
	}
	for _, name := range withheldTools(t, shell) {
		if slices.Contains(got.names, name) {
			t.Errorf("the listing parked at startup lists %s, which a fine-grained token may run none of", name)
		}
	}
}

// TestFineGrainedCalls_WaitsForTheGateOnAGatedConnection verifies the call
// check waits for registration on a connection the readiness gate holds, so it
// decides with the tool sets and the authority in place, answers the gate's
// own refusal when registration failed, and does not wait on a connection the
// gate does not hold or for another method.
func TestFineGrainedCalls_WaitsForTheGateOnAGatedConnection(t *testing.T) {
	call := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "gitlab_demo", Arguments: json.RawMessage(`{}`)}}
	reached := &mcp.CallToolResult{}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return reached, nil }
	noActions := func() *toolvisibility.ToolActions { return nil }

	t.Run("waits, then passes", func(t *testing.T) {
		gate := newReadinessGate(t.Context())
		done := make(chan mcp.Result, 1)
		go func() {
			result, _ := fineGrainedCalls(gate, noActions)(next)(withReadinessGate(t.Context()), "tools/call", call)
			done <- result
		}()
		gate.markReady()
		if got := <-done; got != reached {
			t.Errorf("fineGrainedCalls = %+v, want the call to go on once the gate opened", got)
		}
	})
	t.Run("registration failed", func(t *testing.T) {
		gate := newReadinessGate(t.Context())
		gate.markFailed(errors.New("catalog failed"))
		if got, err := fineGrainedCalls(gate, noActions)(next)(withReadinessGate(t.Context()), "tools/call", call); err == nil || got != nil {
			t.Errorf("fineGrainedCalls = %+v, %v; want the gate's refusal", got, err)
		}
	})
	// The request's context has already ended, so a wait the check should not
	// make is answered at once with the gate's refusal instead of hanging on a
	// gate nobody opens.
	for name, run := range map[string]func(context.Context, mcp.MethodHandler) (mcp.Result, error){
		"an ungated connection": func(ctx context.Context, h mcp.MethodHandler) (mcp.Result, error) {
			return h(ctx, "tools/call", call)
		},
		"another method": func(ctx context.Context, h mcp.MethodHandler) (mcp.Result, error) {
			return h(withReadinessGate(ctx), "tools/list", call)
		},
	} {
		t.Run(name, func(t *testing.T) {
			gate := newReadinessGate(t.Context())
			ended, cancel := context.WithCancel(t.Context())
			cancel()
			if got, err := run(ended, fineGrainedCalls(gate, noActions)(next)); err != nil || got != reached {
				t.Errorf("fineGrainedCalls = %+v, %v; want the call to go on without waiting", got, err)
			}
		})
	}
}

// TestBindProcessClient_BindsTheServersOneClient verifies a server built for
// one client binds it to every request, which is where the layers holding no
// client of their own read a fine-grained token's authority on stdio.
func TestBindProcessClient_BindsTheServersOneClient(t *testing.T) {
	client := gitlabclient.NewUnboundClient("https://gitlab.example.com")
	authority := finegrained.Unevaluated(&finegrained.Table{}, finegrained.FallbackNone, "")
	client.SetAuthority(authority)
	var seen *finegrained.Authority
	next := func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		seen = gitlabclient.AuthorityFrom(ctx)
		return &mcp.ListToolsResult{}, nil
	}
	if _, err := bindProcessClient(client)(next)(context.Background(), "tools/list", nil); err != nil {
		t.Fatal(err)
	}
	if seen != authority {
		t.Errorf("the request read authority %p, want the bound client's %p", seen, authority)
	}
}

// phaseBGitLab is a stand-in instance serving four tokens: two fine-grained
// ones that may read their grants, of Project: Read on one project and of Work
// Item: Create (which expands to create_issue) on every membership, a classic
// one whose scopes it does not describe, and a classic one it describes as
// api. It reports version, which a test may change, and counts the version
// reads; it counts the token descriptions too, and answers them 503 while a
// test says so, and every request 503 while a test says the whole instance is
// down.
type phaseBGitLab struct {
	url      string
	version  atomic.Pointer[string]
	versions atomic.Int64
	selves   atomic.Int64
	selfDown atomic.Bool
	down     atomic.Bool
}

// phaseBTokens are the stand-in's tokens, by what they are.
const (
	phaseBProjectReader = "glpat-project-reader"
	phaseBIssueCreator  = "glpat-issue-creator"
	phaseBClassic       = "glpat-classic-unknown"
	phaseBClassicAPI    = "glpat-classic-api"
)

// newPhaseBGitLab starts the stand-in at 19.4.1-ee.
func newPhaseBGitLab(t *testing.T) *phaseBGitLab {
	t.Helper()
	g := &phaseBGitLab{}
	release := "19.4.1-ee"
	g.version.Store(&release)
	ids := map[string]int{phaseBProjectReader: 11, phaseBIssueCreator: 12}
	grants := map[string]string{
		"11": `{"access":"selected_memberships","permissions":["read_project"],"project_id":7}`,
		"12": `{"access":"all_memberships","permissions":["create_work_item"]}`,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":5,"username":"grantee"}`))
	})
	mux.HandleFunc("GET /api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		g.versions.Add(1)
		if v := *g.version.Load(); v != "" {
			_, _ = w.Write([]byte(`{"version":"` + v + `"}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, r *http.Request) {
		g.selves.Add(1)
		token := r.Header.Get("PRIVATE-TOKEN")
		id, fine := ids[token]
		switch {
		case g.selfDown.Load():
			w.WriteHeader(http.StatusServiceUnavailable)
		case token == phaseBClassicAPI:
			_, _ = w.Write([]byte(`{"id":13,"scopes":["api"],"active":true}`))
		case !fine:
			w.WriteHeader(http.StatusNotFound)
		default:
			_, _ = w.Write([]byte(`{"id":` + strconv.Itoa(id) + `,"scopes":["granular"],"active":true}`))
		}
	})
	mux.HandleFunc("GET /api/v4/personal_access_tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		scope, known := grants[r.PathValue("id")]
		if !known {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":` + r.PathValue("id") + `,"granular":true,"granular_scopes":[` + scope + `]}`))
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

// setVersion changes what the stand-in reports; "" answers 500.
func (g *phaseBGitLab) setVersion(v string) { g.version.Store(&v) }

// TestServerKeys_AGrantReachesNoKey verifies the grant decides what a
// credential is shown and nothing a server, a catalog or a manifest is keyed
// on (INV-010): two fine-grained tokens with different grants, and a classic
// token whose scopes are unknown, are built into configurations whose shape
// key, catalog filter key and manifest share key are all one, while the
// authorities their clients carry list different actions.
func TestServerKeys_AGrantReachesNoKey(t *testing.T) {
	g := newPhaseBGitLab(t)
	cfg := &config.Config{
		GitLabURL: g.url, GitLabToken: "unused", Tier: edition.Free, TierExplicit: true, DisableRetries: true,
		ToolSurface: config.ToolSurfaceIndividual,
	}
	type built struct {
		client *gitlabclient.Client
		cfg    *config.ServerConfig
	}
	var builds []built
	factory := func(client *gitlabclient.Client, entryCfg *config.ServerConfig) (*mcp.Server, error) {
		builds = append(builds, built{client, entryCfg})
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}
	pool := serverpool.New(cfg, factory)
	// sequential: each build appends to builds, which the assertions below read by position
	for _, token := range []string{phaseBProjectReader, phaseBIssueCreator, phaseBClassic} {
		if _, err := pool.GetOrCreateEntry(token, g.url, nil); err != nil {
			t.Fatalf("GetOrCreateEntry(%s): %v", token, err)
		}
	}
	reader, creator, classic := builds[0].client.Authority(), builds[1].client.Authority(), builds[2].client.Authority()
	if reader == nil || creator == nil || classic != nil || reader.Lists("issue.create") || !creator.Lists("issue.create") {
		t.Fatalf("authorities: reader %v, creator %v, classic %v; want two phase B ones that list issue.create apart, and none",
			reader, creator, classic)
	}
	keys := func(b built) [3]string {
		catalog, _, err := gitlabtools.SharedIndividualCatalog(b.client, b.cfg)
		if err != nil {
			t.Fatalf("SharedIndividualCatalog: %v", err)
		}
		return [3]string{
			serverShapeKey(b.cfg, false),
			gitlabtools.CatalogFilterKey(b.cfg),
			manifestShareKey(config.ToolSurfaceIndividual, config.CapabilitySurfaceFull, b.cfg, catalog),
		}
	}
	want := keys(builds[0])
	for i, b := range builds[1:] {
		if got := keys(b); got != want {
			t.Errorf("build %d keys = %q, want the first build's %q", i+1, got, want)
		}
	}
}

// TestStdioAuthority_JudgesTheGrantAtTheVersionAlreadyRead verifies a stdio
// start judges a fine-grained token at the version Initialize read rather
// than asking again, asks it itself when the start could not reach GitLab,
// and gives a classic token nothing.
func TestStdioAuthority_JudgesTheGrantAtTheVersionAlreadyRead(t *testing.T) {
	g := newPhaseBGitLab(t)
	newClient := func(t *testing.T, token string) *gitlabclient.Client {
		t.Helper()
		client, err := gitlabclient.NewClientWithTokenRetries(g.url, token, false, true)
		if err != nil {
			t.Fatalf("NewClientWithTokenRetries: %v", err)
		}
		return client
	}

	initialized := newClient(t, phaseBProjectReader)
	if _, err := initialized.Initialize(t.Context()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	asked := g.versions.Load()
	facts := gitlabclient.DetectToken(t.Context(), initialized.GL())
	authority := stdioAuthority(t.Context(), initialized, facts)
	if authority.Phase() != finegrained.PhaseGranted || authority.Reported() != "19.4.1-ee" || g.versions.Load() != asked {
		t.Errorf("initialized start: phase %v at %q after %d more version reads; want phase B and none", authority.Phase(), authority.Reported(), g.versions.Load()-asked)
	}

	degraded := newClient(t, phaseBProjectReader)
	if got := stdioAuthority(t.Context(), degraded, facts); got.Reported() != "19.4.1-ee" || g.versions.Load() != asked+1 {
		t.Errorf("degraded start: reported %q after %d version reads; want 19.4.1-ee read once", got.Reported(), g.versions.Load()-asked)
	}

	logs := captureFineGrainedLogs(t)
	unreadable := stdioAuthority(t.Context(), degraded, gitlabclient.TokenFacts{Scopes: []string{gitlabclient.ScopeGranular}, FineGrained: true})
	if unreadable.Phase() != finegrained.PhaseUnknown || !strings.Contains(logs.String(), "reason="+string(finegrained.FallbackGrantUnreadable)) {
		t.Errorf("unreadable grant: phase %v, log %q", unreadable.Phase(), logs.String())
	}
	if got := stdioAuthority(t.Context(), degraded, gitlabclient.TokenFacts{Scopes: []string{"api"}}); got != nil {
		t.Errorf("classic token authority = %+v, want nil", got)
	}
}

// captureFineGrainedLogs records the default logger's lines as text for the
// length of the test.
func captureFineGrainedLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return buf
}

// lockedBuffer is a buffer safe to write from the goroutine a test starts and
// read from the test's own.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p.
func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns what was written.
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestRefreshStdioAuthority_FollowsTheInstanceUntilTheContextEnds verifies the
// stdio timer re-reads at its interval: an upgrade to a release the table
// does not record moves the process's token to phase A and says so once, with
// the phase and the reason, however many re-reads find it there; a version
// read the instance does not answer keeps what it has and is logged once, with
// its reason; and the goroutine returns when the context ends. A token that
// may not read its grant is never re-read.
func TestRefreshStdioAuthority_FollowsTheInstanceUntilTheContextEnds(t *testing.T) {
	g := newPhaseBGitLab(t)
	client, err := gitlabclient.NewClientWithTokenRetries(g.url, phaseBProjectReader, false, true)
	if err != nil {
		t.Fatalf("NewClientWithTokenRetries: %v", err)
	}
	facts := gitlabclient.DetectToken(t.Context(), client.GL())
	client.SetAuthority(stdioAuthority(t.Context(), client, facts))
	logs := captureFineGrainedLogs(t)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshStdioAuthority(ctx, client, facts, 5*time.Millisecond, nil, nil)
	}()

	// Each wait is for the refresh goroutine: the upgrade moving the token to
	// phase A, a failed re-read being logged, and two more failed re-reads.
	g.setVersion("19.6.0-ee")
	waitFor(t, func() bool { return client.Authority().Fallback() == finegrained.FallbackVersionOutside })
	reads := g.versions.Load()
	waitFor(t, func() bool { return g.versions.Load() >= reads+2 })
	g.setVersion("")
	waitFor(t, func() bool { return strings.Contains(logs.String(), "keeping what it was shown") })
	reads = g.versions.Load()
	waitFor(t, func() bool { return g.versions.Load() >= reads+2 })
	cancel()
	<-done
	text := logs.String()
	if count := strings.Count(text, "keeping what it was shown"); count != 1 || !strings.Contains(text, "reason="+string(finegrained.FallbackVersionUnanswered)) {
		t.Errorf("the kept re-read was logged %d times, want once with its reason:\n%s", count, text)
	}
	moved := "re-read moved what it is shown\" phase=A reason=" + string(finegrained.FallbackVersionOutside) + " bucket=19.6"
	if count := strings.Count(text, "re-read moved what it is shown"); count != 1 || !strings.Contains(text, moved) {
		t.Errorf("the upgrade was logged %d times, want once as %q:\n%s", count, moved, text)
	}

	unread := make(chan struct{})
	go func() {
		defer close(unread)
		refreshStdioAuthority(context.Background(), client, gitlabclient.TokenFacts{FineGrained: true}, time.Millisecond, nil, nil)
	}()
	select {
	case <-unread:
	case <-time.After(5 * time.Second):
		t.Fatal("the refresh of a token that may not read its grant did not return")
	}
}

// TestRefreshStdioAuthority_AnUnknownKind_IsAskedUntilGitLabAnswers verifies
// the stdio timer asks again what a token is while the start could not tell:
// each round the self endpoint does not answer leaves the token served as a
// classic one, with no authority, and is logged at DEBUG and never warned
// about again; the round it answers that the token is a fine-grained one gives
// the client its authority, judged at the version the start already read, and
// says so once with the phase; and the rounds after that re-read the grant
// like any fine-grained token's.
func TestRefreshStdioAuthority_AnUnknownKind_IsAskedUntilGitLabAnswers(t *testing.T) {
	g := newPhaseBGitLab(t)
	g.selfDown.Store(true)
	client, err := gitlabclient.NewClientWithTokenRetries(g.url, phaseBProjectReader, false, true)
	if err != nil {
		t.Fatalf("NewClientWithTokenRetries: %v", err)
	}
	if _, initErr := client.Initialize(t.Context()); initErr != nil {
		t.Fatalf("Initialize: %v", initErr)
	}
	logs := captureFineGrainedLogs(t)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshStdioAuthority(ctx, client, gitlabclient.TokenFacts{KindUnknown: true}, 5*time.Millisecond, nil, nil)
	}()

	waitFor(t, func() bool { return g.selves.Load() >= 2 })
	if client.Authority() != nil {
		t.Error("a token the self endpoint did not describe was given an authority")
	}
	readsBefore := g.versions.Load()
	g.selfDown.Store(false)
	waitFor(t, func() bool { return client.Authority() != nil })
	if got := client.Authority(); got.Phase() != finegrained.PhaseGranted || got.Reported() != "19.4.1-ee" {
		t.Errorf("once described: phase %v at %q; want phase B at the version the start read", got.Phase(), got.Reported())
	}
	waitFor(t, func() bool { return g.versions.Load() > readsBefore })
	cancel()
	<-done

	text := logs.String()
	if strings.Contains(text, "level=WARN") || !strings.Contains(text, "level=DEBUG msg=\"the token's kind is still unknown\"") {
		t.Errorf("the unanswered rounds logged:\n%s\nwant DEBUG lines and no warning", text)
	}
	learned := "the token whose kind was not known is a fine-grained one; serving it what it may be shown\" phase=B"
	if count := strings.Count(text, "the token whose kind was not known"); count != 1 || !strings.Contains(text, learned) {
		t.Errorf("the kind was logged as learned %d times, want once as %q:\n%s", count, learned, text)
	}
}

// TestPrepareStdioCatalog_ADegradedStart_AsksTheTokensKindOnceItRecovers
// covers the start the timer alone would leave open for its whole interval: a
// stdio process that could not reach GitLab registers the catalog without
// knowing what its token is, and the client's lazy re-initialization, once
// GitLab answers, has the token's kind asked at once, so a fine-grained token
// is given its authority then rather than at the next round, fifteen minutes
// on.
func TestPrepareStdioCatalog_ADegradedStart_AsksTheTokensKindOnceItRecovers(t *testing.T) {
	g := newPhaseBGitLab(t)
	g.down.Store(true)
	cfg := &config.Config{
		GitLabURL: g.url, GitLabToken: phaseBProjectReader, ToolSurface: config.ToolSurfaceDynamic,
		Tier: edition.Free, TierExplicit: true, IgnoreScopes: true, DisableRetries: true,
	}
	client, serverCfg, shell := newStdioStartupShell(t, cfg)

	if prepErr := prepareStdioCatalog(t.Context(), client, cfg, serverCfg, shell, &deferredIdentity{}); prepErr != nil {
		t.Fatalf("prepareStdioCatalog: %v", prepErr)
	}
	if client.IsInitialized() || client.Authority() != nil {
		t.Fatalf("a start against an unreachable GitLab: initialized %v, authority %v; want neither",
			client.IsInitialized(), client.Authority())
	}

	g.down.Store(false)
	client.EnsureInitialized(t.Context())
	waitFor(t, func() bool { return client.Authority() != nil })
	if got := client.Authority(); got.Phase() != finegrained.PhaseGranted || got.Reported() != "19.4.1-ee" {
		t.Errorf("once recovered: phase %v at %q; want phase B at 19.4.1-ee", got.Phase(), got.Reported())
	}
}

// TestRefreshStdioAuthority_AKindLearned_EndsOrStartsTheReReads verifies what
// follows the round that answers: a classic token gets no authority and the
// goroutine returns, since nothing more is asked about it; and a closed
// recovered channel, which a degraded start's client closes once it recovers,
// asks at once rather than at the next round of a long interval.
func TestRefreshStdioAuthority_AKindLearned_EndsOrStartsTheReReads(t *testing.T) {
	g := newPhaseBGitLab(t)
	newClient := func(token string) *gitlabclient.Client {
		client, err := gitlabclient.NewClientWithTokenRetries(g.url, token, false, true)
		if err != nil {
			t.Fatalf("NewClientWithTokenRetries: %v", err)
		}
		return client
	}

	classic := newClient(phaseBClassicAPI)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		refreshStdioAuthority(t.Context(), classic, gitlabclient.TokenFacts{KindUnknown: true}, time.Millisecond, nil, nil)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the refresh of a token learned to be a classic one did not return")
	}
	if classic.Authority() != nil {
		t.Error("a token learned to be a classic one was given an authority")
	}

	fine := newClient(phaseBProjectReader)
	recovered := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshStdioAuthority(ctx, fine, gitlabclient.TokenFacts{KindUnknown: true}, time.Hour, recovered, nil)
	}()
	close(recovered)
	waitFor(t, func() bool { return fine.Authority() != nil })
	cancel()
	<-done
	if got := fine.Authority(); got.Phase() != finegrained.PhaseGranted {
		t.Errorf("asked on recovery: phase %v, want phase B", got.Phase())
	}
}

// TestVerifiedFacts_CarriesTheVerifiersIDBesideTheScopes verifies the gate
// hands the pool what the OAuth layer learned: nothing when it learned no
// scopes, the scopes with the token's id when the verifier read one, and the
// kind unknown when the scopes were the verifier's assumption.
func TestVerifiedFacts_CarriesTheVerifiersIDBesideTheScopes(t *testing.T) {
	cases := []struct {
		name string
		info *auth.TokenInfo
		want *gitlabclient.TokenFacts
	}{
		{name: "no scopes", info: &auth.TokenInfo{UserID: "5", Expiration: time.Now().Add(time.Hour)}},
		{
			name: "a fine-grained token with its id",
			info: &auth.TokenInfo{
				UserID: "5", Scopes: []string{gitlabclient.ScopeGranular}, Expiration: time.Now().Add(time.Hour),
				Extra: map[string]any{oauth.TokenIDKey: int64(11)},
			},
			want: &gitlabclient.TokenFacts{Scopes: []string{gitlabclient.ScopeGranular}, ID: 11, FineGrained: true, GrantReadable: true},
		},
		{
			name: "an OAuth token with no id",
			info: &auth.TokenInfo{UserID: "5", Scopes: []string{"api"}, Expiration: time.Now().Add(time.Hour)},
			want: &gitlabclient.TokenFacts{Scopes: []string{"api"}},
		},
		{
			name: "scopes the verifier assumed",
			info: &auth.TokenInfo{
				UserID: "5", Scopes: []string{"api", "read_api"}, Expiration: time.Now().Add(time.Hour),
				Extra: map[string]any{oauth.ScopesAssumedKey: true},
			},
			want: &gitlabclient.TokenFacts{Scopes: []string{"api", "read_api"}, KindUnknown: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *gitlabclient.TokenFacts
			verify := func(context.Context, string, *http.Request) (*auth.TokenInfo, error) { return tc.info, nil }
			handler := auth.RequireBearerToken(verify, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = verifiedFacts(r)
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			req.Header.Set("Authorization", "Bearer token")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if (got == nil) != (tc.want == nil) || got != nil && (got.ID != tc.want.ID || got.GrantReadable != tc.want.GrantReadable ||
				got.FineGrained != tc.want.FineGrained || got.KindUnknown != tc.want.KindUnknown || !slices.Equal(got.Scopes, tc.want.Scopes)) {
				t.Errorf("verifiedFacts = %+v, want %+v", got, tc.want)
			}
		})
	}
}
