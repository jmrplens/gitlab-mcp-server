package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/gatewaycompat"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// withheldTools names the registered tools a phase A authority lists none of
// the actions of, which is what a fine-grained listing must leave out.
func withheldTools(t *testing.T, shell *serverShell) []string {
	t.Helper()
	authority := actiongrants.Build(true)
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
	client.SetAuthority(actiongrants.Build(true))
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
	client.SetAuthority(actiongrants.Build(true))
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
