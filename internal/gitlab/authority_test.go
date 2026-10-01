package gitlab

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestClient_Authority_HoldsWhatWasAttached verifies a client answers the
// authority attached to it, replaced whole by a second attach and cleared by a
// nil one, and that a client nothing was attached to, or no client at all,
// answers nil, which is every credential that is not a fine-grained token.
func TestClient_Authority_HoldsWhatWasAttached(t *testing.T) {
	var none *Client
	if got := none.Authority(); got != nil {
		t.Errorf("(*Client)(nil).Authority() = %p, want nil", got)
	}
	client := NewUnboundClient("https://gitlab.example.com")
	if got := client.Authority(); got != nil {
		t.Errorf("Authority() before any attach = %p, want nil", got)
	}
	first := finegrained.Unevaluated(&finegrained.Table{}, finegrained.FallbackNone, "")
	second := finegrained.Unevaluated(&finegrained.Table{}, finegrained.FallbackNone, "")
	client.SetAuthority(first)
	if got := client.Authority(); got != first {
		t.Errorf("Authority() = %p, want the attached %p", got, first)
	}
	client.SetAuthority(second)
	if got := client.Authority(); got != second {
		t.Errorf("Authority() after a second attach = %p, want %p", got, second)
	}
	client.SetAuthority(nil)
	if got := client.Authority(); got != nil {
		t.Errorf("Authority() after a nil attach = %p, want nil", got)
	}
}

// TestAuthorityFrom_ReadsTheClientBoundToTheRequest verifies the authority is
// read from the client bound to the context alone: none when nothing is bound,
// none when the bound client carries none, and the bound client's when it does.
func TestAuthorityFrom_ReadsTheClientBoundToTheRequest(t *testing.T) {
	if got := AuthorityFrom(context.Background()); got != nil {
		t.Errorf("AuthorityFrom(unbound) = %p, want nil", got)
	}
	client := NewUnboundClient("https://gitlab.example.com")
	ctx := WithClient(context.Background(), client)
	if got := AuthorityFrom(ctx); got != nil {
		t.Errorf("AuthorityFrom(a classic client) = %p, want nil", got)
	}
	authority := finegrained.Unevaluated(&finegrained.Table{}, finegrained.FallbackNone, "")
	client.SetAuthority(authority)
	if got := AuthorityFrom(ctx); got != authority {
		t.Errorf("AuthorityFrom = %p, want the bound client's %p", got, authority)
	}
}
