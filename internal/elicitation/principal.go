// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// principal.go carries the principal a request runs as, which the
// multi-round-trip request state issued during it is bound to.

package elicitation

import "context"

// principalKey is the context key [WithPrincipal] stores the principal under.
type principalKey struct{}

// WithPrincipal returns ctx naming principal as who the request runs as.
//
// The principal is opaque here and only ever compared: [FlowFromRequest] binds
// the state a flow hands its client to it, and state presented under any other
// principal yields no answers, so the flow asks its questions again. The HTTP
// server sets it, when it binds a request to the credential it was
// authenticated as, to the pool entry's owner token: a random value that every
// round of one flow shares, since each POST of it carries the same credential,
// and that no other credential has. Nothing sets it on stdio, where one process
// serves one credential, so the empty principal there is everyone the process
// serves.
//
// The principal is never put on the wire. It is mixed into the MAC the state
// carries, so the state names nobody a client could read back.
func WithPrincipal(ctx context.Context, principal string) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

// PrincipalFrom returns the principal [WithPrincipal] put on ctx, or the empty
// principal when nothing did.
func PrincipalFrom(ctx context.Context) string {
	principal, _ := ctx.Value(principalKey{}).(string)
	return principal
}
