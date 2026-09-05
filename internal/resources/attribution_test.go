// attribution_test.go covers the credential every resource read runs under.
//
// One MCP server is shared by every credential whose configuration hashes to the
// same shape, so these handlers are registered once with the credential-less
// client and each read resolves the caller's own from the request context. Two
// things follow, and neither is visible in a handler read on its own: a read
// that brought no credential must be refused rather than reported as a resource
// that is not there, and no handler may ever use the client it was registered
// with.
package resources

import (
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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
