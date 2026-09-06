// registrar_test.go validates the seam that lets this server read its own
// resources: the recording registrars and the handler index a subscription
// watcher reads through.
//
// The wiring tests in cmd/server prove the index against a live server;
// what belongs here is the seam's own contract — every registration is
// captured, and Read's outcomes (missing handler, failing handler, empty
// result, text versus blob content) are each reported distinctly.
package resources

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// registrarTestClient builds a GitLab client for registration alone; no
// request is issued unless a handler is invoked.
func registrarTestClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:      "http://127.0.0.1:1", // deliberately unreachable: registration must not dial
		GitLabToken:    "glpat-test",
		DisableRetries: true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestNewHandlerIndex_CapturesEveryRegistration verifies the index built
// without a server carries a handler for both registration forms — static
// resources and templates — for everything registerAll installs.
//
// This is the property subscriptions depend on: a kind whose template was
// registered but not indexed would classify as subscribable and then fail
// every read.
func TestNewHandlerIndex_CapturesEveryRegistration(t *testing.T) {
	index := NewHandlerIndex(registrarTestClient(t))

	if len(index) == 0 {
		t.Fatal("NewHandlerIndex() returned an empty index")
	}
	for _, key := range []string{
		"gitlab://user/current",                                // static resource
		"gitlab://project/{project_id}/pipeline/{pipeline_id}", // template
	} {
		t.Run(key, func(t *testing.T) {
			if index[key] == nil {
				t.Errorf("index has no handler for %q", key)
			}
		})
	}
}

// TestHandlerIndexRead_ReportsEachFailureDistinctly verifies Read's error
// paths name what actually went wrong, since a watcher's stop decision
// hangs on telling them apart.
func TestHandlerIndexRead_ReportsEachFailureDistinctly(t *testing.T) {
	handlerErr := errors.New("backend said no")
	index := HandlerIndex{
		"tmpl/fails": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return nil, handlerErr
		},
		"tmpl/nil": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			// The nil,nil contract violation under test — Read must catch a
			// handler that returns neither result nor error.
			return nil, nil //nolint:nilnil // deliberately malformed handler
		},
		"tmpl/empty": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{}, nil
		},
	}

	tests := []struct {
		name     string
		template string
		wantIn   string
		wantErr  error
	}{
		{"unregistered template", "tmpl/missing", "no handler registered", nil},
		{"handler error is passed through", "tmpl/fails", "", handlerErr},
		{"nil result", "tmpl/nil", "returned no content", nil},
		{"empty contents", "tmpl/empty", "returned no content", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := index.Read(context.Background(), tt.template, "gitlab://whatever/1")
			if err == nil {
				t.Fatal("Read() error = nil, want a failure")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Read() error = %v, want it to wrap %v", err, tt.wantErr)
			}
			if tt.wantIn != "" && !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Read() error = %v, want it to mention %q", err, tt.wantIn)
			}
		})
	}
}

// TestHandlerIndexRead_ReturnsTextThenBlob verifies both content branches:
// text content is returned as its bytes, and a blob-only result is not
// silently read as empty.
func TestHandlerIndexRead_ReturnsTextThenBlob(t *testing.T) {
	index := HandlerIndex{
		"tmpl/text": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{Text: `{"ok":true}`}}}, nil
		},
		"tmpl/blob": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{Blob: []byte{0x1f, 0x8b}}}}, nil
		},
	}

	tests := []struct {
		name     string
		template string
		want     []byte
	}{
		{"text content", "tmpl/text", []byte(`{"ok":true}`)},
		{"blob content", "tmpl/blob", []byte{0x1f, 0x8b}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := index.Read(context.Background(), tt.template, "gitlab://x/1")
			if err != nil {
				t.Fatalf("Read(%s): %v", tt.template, err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Read(%s) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

// TestRecorder_RegistersAndIndexesTogether verifies the server-attached
// recorder does both halves of its job for both registration forms: the
// resource reaches the server and the handler reaches the index.
func TestRecorder_RegistersAndIndexesTogether(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	rec := &recorder{server: server, index: make(HandlerIndex)}
	handler := func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{Text: "x"}}}, nil
	}

	rec.AddResource(&mcp.Resource{URI: "gitlab://static", Name: "static", MIMEType: "application/json"}, handler)
	rec.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "gitlab://tmpl/{id}", Name: "tmpl", MIMEType: "application/json"}, handler)

	if rec.index["gitlab://static"] == nil {
		t.Error("AddResource registered on the server but not in the index")
	}
	if rec.index["gitlab://tmpl/{id}"] == nil {
		t.Error("AddResourceTemplate registered on the server but not in the index")
	}
}

// TestAddResourceTemplate_SubscribableTemplates_MatchWhitelistExactly
// verifies the recorder annotates exactly the subscribable templates: every
// template in the subscription whitelist ends with the marker sentence and
// carries the vendor _meta key, and no other template does. Both markers
// are appended mechanically from subscriptions.Templates(), so this is the
// drift guard between what a client reads and the whitelist the
// SubscribeHandler enforces — the hand-written predecessor of the sentence
// covered 3 of 26 templates.
func TestAddResourceTemplate_SubscribableTemplates_MatchWhitelistExactly(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	result, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(result.ResourceTemplates) == 0 {
		t.Fatal("no resource templates registered")
	}

	subscribable := make(map[string]bool)
	for _, tmpl := range subscriptions.Templates() {
		subscribable[tmpl] = true
	}

	seen := 0
	for _, tmpl := range result.ResourceTemplates {
		want := subscribable[tmpl.URITemplate]
		if want {
			seen++
		}
		t.Run(tmpl.URITemplate, func(t *testing.T) {
			if marked := strings.HasSuffix(tmpl.Description, subscribableMarker); marked != want {
				t.Errorf("description marker = %t, want %t", marked, want)
			}
			if metaMarked, _ := tmpl.Meta[subscribableMetaKey].(bool); metaMarked != want {
				t.Errorf("%s meta key = %t, want %t", subscribableMetaKey, metaMarked, want)
			}
		})
	}
	if seen != len(subscribable) {
		t.Errorf("registered %d subscribable templates, whitelist has %d", seen, len(subscribable))
	}
}

// The tests below cover the credential every resource read runs under.
//
// One MCP server is shared by every credential whose configuration hashes to the
// same shape, so these handlers are registered once with the credential-less
// client and each read resolves the caller's own from the request context. Two
// things follow, and neither is visible in a handler read on its own: a read
// that brought no credential must be refused rather than reported as a resource
// that is not there, and no handler may ever use the client it was registered
// with.

// placeholder matches the {name} parameters of a URI template.
var placeholder = regexp.MustCompile(`\{[^}]+\}`)

// concreteURI turns a URI template into a URI a handler will accept, which for
// every parameter these templates take means something that reads as an id.
func concreteURI(template string) string {
	return placeholder.ReplaceAllString(template, "42")
}

// countingGitLab answers every request with an object GitLab could have
// returned, and counts what it was asked.
func countingGitLab(t *testing.T) (*gitlabclient.Client, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// Both shapes, because these handlers ask for objects and for lists,
		// and neither has to decode into anything in particular here: what is
		// under test is which client the request went to.
		_, _ = w.Write([]byte(`{"id":42,"iid":42,"name":"n","path_with_namespace":"g/p","title":"t","status":"success"}`))
	}))
	return client, &hits
}

// TestRegisteredResources_NeverUseTheClientTheyWereRegisteredWith walks every
// registered resource and requires each to answer through the request's own
// credential.
//
// The 38 closures repeat that resolution by hand, one base.For(ctx) each. All 38
// are correct, and a 39th added later would fail closed on every shared HTTP
// deployment while the whole suite stayed green, because a handler read on its
// own cannot tell which client it used. This is the test that can: the client
// captured at registration points at a server of its own, and being asked
// anything at all is the failure.
func TestRegisteredResources_NeverUseTheClientTheyWereRegisteredWith(t *testing.T) {
	var captured atomic.Int64
	registered := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		captured.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	index := NewHandlerIndex(registered)
	if len(index) == 0 {
		t.Fatal("no resources were registered, so this test proves nothing")
	}

	for template, handler := range index {
		t.Run(template, func(t *testing.T) {
			bound, hits := countingGitLab(t)
			uri := concreteURI(template)
			ctx := gitlabclient.WithClient(t.Context(), bound)

			// The answer itself is not the subject: a fake GitLab returning one
			// object for every path makes some of these fail to decode, and a
			// failure that reached GitLab has already proved the point.
			_, _ = handler(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: uri}})

			if hits.Load() == 0 {
				t.Errorf("reading %s reached no GitLab at all, so it cannot show which client it used", uri)
			}
			if captured.Load() != 0 {
				t.Errorf("reading %s went to the client captured at registration; on a shared server that client "+
					"carries no credential and refuses every request", uri)
			}
		})
	}
}

// TestRegisteredResources_AnUnattributedRead_IsRefusedAsSuch covers what a
// caller is told when the request brought no credential.
//
// Nineteen of these handlers answer any GitLab failure with
// mcp.ResourceNotFoundError, so an unattributed read used to be reported as a
// resource that does not exist or a token that cannot see it. Both send someone
// to check a permission that is fine. The guard in front of every handler
// answers with what actually happened instead.
func TestRegisteredResources_AnUnattributedRead_IsRefusedAsSuch(t *testing.T) {
	unbound := gitlabclient.NewUnboundClient("https://gitlab.invalid")
	index := NewHandlerIndex(unbound)

	for template, handler := range index {
		t.Run(template, func(t *testing.T) {
			uri := concreteURI(template)

			_, err := handler(t.Context(), &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: uri}})

			if err == nil {
				t.Fatalf("reading %s with no credential bound succeeded", uri)
			}
			if !strings.Contains(err.Error(), toolutil.UnattributedRequestMessage) {
				t.Errorf("reading %s answered %q, want the unattributed-request refusal", uri, err)
			}
		})
	}
}

// TestRegisteredResources_AnAbandonedRead_IsNotBlamedOnTheWiring covers the
// legitimate cause of an unattributed read.
//
// A POST the client abandoned takes its carrier with it, and the carrier is
// where the credential is read from, so a client that went away reaches this
// guard in exactly the state a wiring defect reaches it in. The refusal asks
// the caller to report a bug, which is the wrong thing to say about a request
// that was already over.
func TestRegisteredResources_AnAbandonedRead_IsNotBlamedOnTheWiring(t *testing.T) {
	unbound := gitlabclient.NewUnboundClient("https://gitlab.invalid")
	index := NewHandlerIndex(unbound)

	handler, ok := index["gitlab://user/current"]
	if !ok {
		t.Fatal("the current-user resource is no longer registered under that URI")
	}

	gone := errors.New("the caller went away")
	abandoned, cancel := context.WithCancelCause(context.Background())
	cancel(gone)

	_, err := handler(abandoned, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitlab://user/current"}})

	if !errors.Is(err, gone) {
		t.Errorf("an abandoned read was answered %v, want the reason it ended", err)
	}
	if err != nil && strings.Contains(err.Error(), toolutil.UnattributedRequestMessage) {
		t.Errorf("a client that went away was told to report a wiring defect: %v", err)
	}
}

// TestRegisteredResources_ABoundRead_ReachesTheHandler covers the other side of
// the guard: a request that did bring a credential is not refused by it.
func TestRegisteredResources_ABoundRead_ReachesTheHandler(t *testing.T) {
	unbound := gitlabclient.NewUnboundClient("https://gitlab.invalid")
	index := NewHandlerIndex(unbound)
	bound, hits := countingGitLab(t)

	handler, ok := index["gitlab://user/current"]
	if !ok {
		t.Fatal("the current-user resource is no longer registered under that URI")
	}
	ctx := gitlabclient.WithClient(t.Context(), bound)

	if _, err := handler(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitlab://user/current"}}); err != nil {
		t.Fatalf("a bound read was refused: %v", err)
	}
	if hits.Load() == 0 {
		t.Error("the bound read never reached GitLab, so the guard refused a request that carried a credential")
	}
}
