package prompts

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// collectingRegistrar records what registerAll offers without a server.
type collectingRegistrar struct {
	names []string
}

func (r *collectingRegistrar) AddPrompt(prompt *mcp.Prompt, _ mcp.PromptHandler) {
	r.names = append(r.names, prompt.Name)
}

// registeredPromptNames returns every prompt registerAll offers, with the
// options applied.
func registeredPromptNames(client *gitlabclient.Client, opts ...RegisterOptions) []string {
	collector := &collectingRegistrar{}
	registerAll(registrarFor(collector, opts), client)
	return collector.names
}

// TestRegister_ExcludingAnActionRemovesThePromptServingTheSameData verifies
// that an operator's --exclude-tools reaches the prompt surface, which was the
// third request path to the same GitLab data with the same credential and the
// last one that took no options at all.
//
// Excluding gitlab_mr_changes_get used to leave review_mr returning the
// identical diffs, so the narrowing an operator configured was two thirds of a
// contract. Each subtest asserts both directions: the prompt whose data went is
// gone, and every prompt that shares no action with the exclusion is still
// there, because a filter that removes too much is its own defect.
func TestRegister_ExcludingAnActionRemovesThePromptServingTheSameData(t *testing.T) {
	tests := []struct {
		name    string
		opts    []RegisterOptions
		gone    []string
		present []string
	}{
		{
			name:    "no options registers everything",
			opts:    nil,
			present: []string{"review_mr", "team_overview", "release_cadence"},
		},
		{
			name:    "empty exclusion list registers everything",
			opts:    []RegisterOptions{{}},
			present: []string{"review_mr", "team_overview", "release_cadence"},
		},
		{
			name:    "excluding an action removes every prompt reading it",
			opts:    []RegisterOptions{{ExcludedActions: []string{"mr_review.changes_get"}}},
			gone:    []string{"review_mr", "summarize_mr_changes", "mr_risk_assessment", "mr_description_quality"},
			present: []string{"team_overview", "release_cadence", "label_distribution"},
		},
		{
			name:    "one of several backing actions is enough",
			opts:    []RegisterOptions{{ExcludedActions: []string{"branch.list"}}},
			gone:    []string{"project_health_check"},
			present: []string{"compare_branches", "review_mr"},
		},
		{
			name:    "excluding a group action removes the group prompts",
			opts:    []RegisterOptions{{ExcludedActions: []string{"merge_request.list_group"}}},
			gone:    []string{"team_overview", "group_mr_dashboard", "reviewer_workload", "weekly_team_recap"},
			present: []string{"review_mr", "label_distribution"},
		},
		{
			name: "several exclusions compose",
			opts: []RegisterOptions{{ExcludedActions: []string{"release.list", "repository.contributors"}}},
			gone: []string{"release_cadence", "project_contributors"},
			present: []string{
				"review_mr",
				"label_distribution",
			},
		},
		{
			name:    "blank and unknown entries change nothing",
			opts:    []RegisterOptions{{ExcludedActions: []string{"", "   ", "not.an_action"}}},
			present: []string{"review_mr", "team_overview", "release_cadence"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registered := registeredPromptNames(nil, tt.opts...)
			for _, name := range tt.gone {
				if slices.Contains(registered, name) {
					t.Errorf("prompt %q is still registered after excluding %v", name, tt.opts)
				}
			}
			for _, name := range tt.present {
				if !slices.Contains(registered, name) {
					t.Errorf("prompt %q was removed by excluding %v, which does not name it", name, tt.opts)
				}
			}
		})
	}
}

// TestRegister_ExcludedPromptIsNotListedByTheServer verifies the same thing on
// the wire: a client's prompts/list must not offer what the operator excluded,
// since a listed prompt is one a model will try to get, and prompts/get must
// refuse it rather than run the handler anyway.
//
// Filtering only the listing would be the worse half of the fix. A model that
// remembers a prompt name, or a client working from a cached list, calls
// prompts/get directly, and the handler holds the same credential whether or
// not the prompt was advertised.
func TestRegister_ExcludedPromptIsNotListedByTheServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	Register(server, nil, RegisterOptions{ExcludedActions: []string{"mr_review.changes_get"}})

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "probe", Version: "0.0.1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	list, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts() error = %v", err)
	}
	names := make([]string, 0, len(list.Prompts))
	for _, prompt := range list.Prompts {
		names = append(names, prompt.Name)
	}

	t.Run("excluded prompt is not listed", func(t *testing.T) {
		if slices.Contains(names, "review_mr") {
			t.Error("prompts/list still offers review_mr after its diffs action was excluded")
		}
	})
	t.Run("unrelated prompt is still listed", func(t *testing.T) {
		if !slices.Contains(names, "team_overview") {
			t.Error("prompts/list no longer offers team_overview, which the exclusion does not name")
		}
	})
	t.Run("excluded prompt cannot be fetched", func(t *testing.T) {
		_, getErr := session.GetPrompt(ctx, &mcp.GetPromptParams{
			Name:      "review_mr",
			Arguments: map[string]string{argProjectID: "1", argMRIID: "1"},
		})
		if getErr == nil {
			t.Error("prompts/get served review_mr after it was excluded")
		}
	})
}

// TestPromptBackingActions_StaysAlignedWithBothSurfaces guards the two ways
// the hand-kept overlap table can rot.
//
// The table is the only thing relating a prompt to the tools serving the same
// data, so it fails in two directions: a prompt added to registerAll without an
// entry would silently escape every exclusion, and an action ID that no longer
// exists in the catalog would make an entry a dead string nobody notices.
// Neither is visible from either package alone.
func TestPromptBackingActions_StaysAlignedWithBothSurfaces(t *testing.T) {
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("every registered prompt is classified", func(t *testing.T) {
		for _, name := range registeredPromptNames(nil) {
			if _, ok := promptBackingActions[name]; !ok {
				t.Errorf("prompt %q has no entry in promptBackingActions, so --exclude-tools cannot reach it", name)
			}
		}
	})

	t.Run("no entry describes a prompt that is gone", func(t *testing.T) {
		registered := registeredPromptNames(nil)
		for name := range promptBackingActions {
			if !slices.Contains(registered, name) {
				t.Errorf("promptBackingActions names %q, which registerAll no longer registers", name)
			}
		}
	})

	t.Run("every named action exists in the catalog", func(t *testing.T) {
		for name, backing := range promptBackingActions {
			if len(backing) == 0 {
				t.Errorf("prompt %q lists no backing action; give it one or state why it has none", name)
				continue
			}
			for _, id := range backing {
				if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
					t.Errorf("prompt %q names action %q, which is not in the action catalog", name, id)
				}
			}
		}
	})
}

// The tests below cover the credential every prompt runs under.
//
// One MCP server is shared by every credential whose configuration hashes to the
// same shape, so these 37 handlers are registered once with the credential-less
// client and each resolves the caller's own from the request context. A handler
// that forgot to would read GitLab as nobody, which no test of that handler on
// its own can see, and a prompts/get that brought no credential at all has to
// say so rather than report whichever GitLab error its first read produced.

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

// TestRegisteredPrompts_AnAbandonedRequest_IsNotBlamedOnTheWiring covers the
// legitimate cause of an unattributed request.
//
// A POST the client abandoned takes its carrier with it, and the carrier is
// where the credential is read from, so a client that pressed stop reaches this
// guard in exactly the state a wiring defect reaches it in. The refusal asks
// the caller to report a bug, which is the wrong thing to say about a request
// that was already over.
func TestRegisteredPrompts_AnAbandonedRequest_IsNotBlamedOnTheWiring(t *testing.T) {
	unbound := gitlabclient.NewUnboundClient("https://gitlab.invalid")
	handlers := registeredPrompts(unbound)

	handler, ok := handlers["summarize_mr_changes"]
	if !ok {
		t.Fatal("summarize_mr_changes is no longer registered")
	}

	gone := errors.New("the caller went away")
	abandoned, cancel := context.WithCancelCause(context.Background())
	cancel(gone)

	_, err := handler(abandoned, promptRequest("summarize_mr_changes"))

	if !errors.Is(err, gone) {
		t.Errorf("an abandoned prompts/get was answered %v, want the reason it ended", err)
	}
	if err != nil && strings.Contains(err.Error(), toolutil.UnattributedRequestMessage) {
		t.Errorf("a client that went away was told to report a wiring defect: %v", err)
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
