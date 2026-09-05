// attribution_test.go covers the credential every prompt runs under.
//
// One MCP server is shared by every credential whose configuration hashes to the
// same shape, so these 37 handlers are registered once with the credential-less
// client and each resolves the caller's own from the request context. A handler
// that forgot to would read GitLab as nobody, which no test of that handler on
// its own can see, and a prompts/get that brought no credential at all has to
// say so rather than report whichever GitLab error its first read produced.
package prompts

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// handlerCollector captures what registerAll would register, handlers included,
// so a test can walk all 37 prompts rather than the handful anyone thought to
// name. Its sibling in exclusion_test.go keeps only the names, which is what
// that file asserts on.
type handlerCollector struct {
	handlers map[string]mcp.PromptHandler
}

func (r *handlerCollector) AddPrompt(prompt *mcp.Prompt, handler mcp.PromptHandler) {
	r.handlers[prompt.Name] = handler
}

// registeredPrompts returns every prompt registered against base, wrapped the
// way [Register] wraps them.
func registeredPrompts(base *gitlabclient.Client) map[string]mcp.PromptHandler {
	collector := &handlerCollector{handlers: make(map[string]mcp.PromptHandler)}
	registerAll(attributed(collector, base), base)
	return collector.handlers
}

// everyPromptArgument is a superset of the arguments these prompts read, so a
// handler reaches its GitLab calls rather than returning early for a missing
// one. Prompts that validate an argument they were not given are the point of
// the superset: what is under test is which client a read uses, not which
// arguments a prompt requires.
var everyPromptArgument = map[string]string{
	"project_id":        "42",
	"group_id":          "42",
	"merge_request_iid": "1",
	"issue_iid":         "1",
	"username":          "someone",
	"days":              "7",
	"state":             "opened",
	"target_branch":     "main",
	"milestone":         "v1",
	"from":              "main",
	"to":                "release",
	"ref":               "main",
	"tag":               "v1",
	"sha":               "abc123",
}

// promptRequest builds a prompts/get carrying every argument any of them reads.
func promptRequest(name string) *mcp.GetPromptRequest {
	return &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: name, Arguments: everyPromptArgument}}
}

// answeringGitLab answers every request with an object and a list at once, and
// counts what it was asked.
func answeringGitLab(t *testing.T) (*gitlabclient.Client, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "s") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"iid":1,"title":"t","name":"n","path_with_namespace":"g/p","web_url":"https://example.invalid"}`))
	}))
	return client, &hits
}

// TestRegisteredPrompts_NeverUseTheClientTheyWereRegisteredWith walks every
// prompt and requires each to read GitLab through the request's own credential.
//
// The resolution is repeated by hand in 37 closures. All are correct, and the
// 38th is the one this exists for: it would fail closed on every shared HTTP
// deployment while its own test, which registers a real client, passed.
func TestRegisteredPrompts_NeverUseTheClientTheyWereRegisteredWith(t *testing.T) {
	var captured atomic.Int64
	registered := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		captured.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	handlers := registeredPrompts(registered)
	if len(handlers) == 0 {
		t.Fatal("no prompts were registered, so this test proves nothing")
	}

	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			bound, hits := answeringGitLab(t)
			ctx := gitlabclient.WithClient(t.Context(), bound)

			// The result is not the subject: one fake answer for every path
			// makes some of these fail, and a failure that reached GitLab has
			// already shown which client it used.
			_, _ = handler(ctx, promptRequest(name))

			if hits.Load() == 0 {
				t.Errorf("%s reached no GitLab at all, so it cannot show which client it used", name)
			}
			if captured.Load() != 0 {
				t.Errorf("%s read the client captured at registration; on a shared server that client carries "+
					"no credential and refuses every request", name)
			}
		})
	}
}

// TestRegisteredPrompts_AnUnattributedRequest_IsRefusedAsSuch covers what a
// caller is told when the request brought no credential: the same sentence the
// tool and resource surfaces give, rather than whichever GitLab error the
// handler's first read happened to produce.
func TestRegisteredPrompts_AnUnattributedRequest_IsRefusedAsSuch(t *testing.T) {
	unbound := gitlabclient.NewUnboundClient("https://gitlab.invalid")

	for name, handler := range registeredPrompts(unbound) {
		t.Run(name, func(t *testing.T) {
			_, err := handler(t.Context(), promptRequest(name))

			if err == nil {
				t.Fatalf("%s answered a request with no credential bound", name)
			}
			if !strings.Contains(err.Error(), toolutil.UnattributedRequestMessage) {
				t.Errorf("%s answered %q, want the unattributed-request refusal", name, err)
			}
		})
	}
}

// TestRegisteredPrompts_ABoundRequest_ReachesTheHandler covers the other side of
// the guard, so that refusing everything could not pass for correct.
func TestRegisteredPrompts_ABoundRequest_ReachesTheHandler(t *testing.T) {
	unbound := gitlabclient.NewUnboundClient("https://gitlab.invalid")
	handlers := registeredPrompts(unbound)
	bound, hits := answeringGitLab(t)

	handler, ok := handlers["summarize_mr_changes"]
	if !ok {
		t.Fatal("summarize_mr_changes is no longer registered")
	}

	_, _ = handler(gitlabclient.WithClient(context.Background(), bound), promptRequest("summarize_mr_changes"))

	if hits.Load() == 0 {
		t.Error("a bound prompt never reached GitLab, so the guard refused a request that carried a credential")
	}
}
