// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package elicitation

import (
	"context"
	"testing"
)

// TestPrincipalFrom_ReadsWhatWithPrincipalPut covers the context value a
// request's principal travels in, from the HTTP server's credential binding to
// [FlowFromRequest]. What was put is what is read, the empty principal
// included, and a context nothing put one on reads as the empty principal,
// which is what stdio and every request nothing bound to a credential run as.
func TestPrincipalFrom_ReadsWhatWithPrincipalPut(t *testing.T) {
	for _, tt := range []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "a principal was put", ctx: WithPrincipal(context.Background(), "entry-owner"), want: "entry-owner"},
		{name: "the empty principal was put", ctx: WithPrincipal(context.Background(), ""), want: ""},
		{name: "nothing was put", ctx: context.Background(), want: ""},
		{
			name: "a request rebound to another principal",
			ctx:  WithPrincipal(WithPrincipal(context.Background(), "entry-first"), "entry-second"),
			want: "entry-second",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrincipalFrom(tt.ctx); got != tt.want {
				t.Errorf("PrincipalFrom() = %q, want %q", got, tt.want)
			}
		})
	}
}
