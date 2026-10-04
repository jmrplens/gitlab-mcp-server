package gitlab

import (
	"context"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// SetAuthority attaches what this client's credential may do as a
// fine-grained personal access token, and nil for any other credential.
//
// The client is where it lives because the client is already the per-request
// carrier of the credential: an HTTP request binds its pool entry's client to
// the context ([WithClient]) and every handler reads it back through
// [Client.For], and on stdio the one client serves every request. So every
// place that decides what a fine-grained session is shown, the listing, the
// call check, the dispatchers, find and the manifest, reads one mechanism on
// both transports, and a classic credential's client carries nothing and pays
// nothing. It is replaced whole, never edited: an [finegrained.Authority] is
// immutable, so a reader holds a consistent one however a revalidation races
// it.
func (c *Client) SetAuthority(authority *finegrained.Authority) {
	c.authority.Store(authority)
}

// Authority returns what [Client.SetAuthority] attached, or nil when nothing
// was, which is every credential that is not a fine-grained token. A nil
// client has none either, so a caller holding no client asks without a guard.
func (c *Client) Authority() *finegrained.Authority {
	if c == nil {
		return nil
	}
	return c.authority.Load()
}

// AuthorityFrom returns the authority of the client bound to ctx, or nil when
// none is bound or the bound one carries none.
//
// It reads the binding alone, never a fallback client: the callers are the
// layers that decide what a fine-grained session is shown, and they hold no
// client of their own. Both transports bind one, HTTP the request's pool entry
// and stdio the process's single client, so a request with no binding is one
// nothing could attribute to a credential, which is served as unknown
// authority and refused further in by the unbound client.
func AuthorityFrom(ctx context.Context) *finegrained.Authority {
	client, _ := ClientFrom(ctx)
	return client.Authority()
}
