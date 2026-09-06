package gitlab

import "context"

// clientContextKey carries the client that serves one request.
//
// It is unexported and typed, so nothing outside this package can put a value
// under it: a request is bound to a credential by [WithClient] or not at all.
type clientContextKey struct{}

// WithClient returns ctx carrying the client every handler running under it
// must use.
//
// It is the channel that lets one [github.com/modelcontextprotocol/go-sdk/mcp.Server]
// serve many credentials. A server built once per configuration shape registers
// handlers that capture the credential-less client from [NewUnboundClient]; the
// HTTP layer resolves the pool entry for each request and installs that entry's
// client here, and [Client.For] is what every handler reads it back through.
//
// A nil client is not stored. "Bound to nothing" and "not bound" would then be
// indistinguishable to [Client.For], and the difference decides whether a
// handler runs under the caller's credential or under the one that refuses
// every request.
func WithClient(ctx context.Context, c *Client) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, clientContextKey{}, c)
}

// For returns the client bound to ctx, or the receiver when ctx carries none.
//
// The receiver is the fallback rather than the answer, which is what makes the
// shared-server arrangement fail closed: on a shape server the captured client
// is the unbound one, so a handler reached without a bound context refuses with
// [ErrUnboundClient] instead of running under whichever credential happened to
// build the shape. On stdio, and in every test that registers with a real
// client, no context ever carries one and the receiver is simply returned, so
// the call is invisible.
//
// A nil receiver is allowed and returns whatever the context carries, so a
// caller holding no fallback can still resolve one.
func (c *Client) For(ctx context.Context) *Client {
	if ctx == nil {
		return c
	}
	if bound, ok := ctx.Value(clientContextKey{}).(*Client); ok && bound != nil {
		return bound
	}
	return c
}

// ClientFrom returns the client bound to ctx, and whether there was one.
//
// [Client.For] is what handlers use; this is for the few callers that must
// distinguish "no credential is bound" from "the fallback was returned", such
// as a middleware deciding whether it has anything to install.
func ClientFrom(ctx context.Context) (*Client, bool) {
	if ctx == nil {
		return nil, false
	}
	bound, ok := ctx.Value(clientContextKey{}).(*Client)
	if !ok || bound == nil {
		return nil, false
	}
	return bound, true
}
