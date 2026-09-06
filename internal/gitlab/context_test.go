package gitlab

import (
	"context"
	"testing"
)

// TestWithClient_BindsTheClientARequestRunsUnder verifies the seam that lets
// one MCP server serve many credentials: a handler holding the shared,
// credential-less client must be able to find the caller's own on the request
// context, and must fall back to the one it holds when nothing bound one.
func TestWithClient_BindsTheClientARequestRunsUnder(t *testing.T) {
	t.Parallel()

	captured := NewUnboundClient("https://gitlab.example.com")
	bound := NewUnboundClient("https://gitlab.com")

	cases := []struct {
		name string
		ctx  func() context.Context
		want *Client
	}{
		{
			name: "a bound context wins over the captured client",
			ctx:  func() context.Context { return WithClient(context.Background(), bound) },
			want: bound,
		},
		{
			name: "an unbound context falls back to the captured client",
			ctx:  context.Background,
			want: captured,
		},
		{
			name: "binding nil leaves the context alone rather than binding nothing",
			ctx:  func() context.Context { return WithClient(context.Background(), nil) },
			want: captured,
		},
		{
			name: "rebinding replaces the previous binding",
			ctx: func() context.Context {
				return WithClient(WithClient(context.Background(), captured), bound)
			},
			want: bound,
		},
		{
			// WithClient refuses to store a nil, so only a caller reaching
			// past it can produce this. The fallback still has to win:
			// returning the typed nil would hand a handler a client whose
			// every method call is a nil dereference, which is strictly
			// worse than the refusal ErrUnboundClient gives.
			name: "a typed-nil binding falls back to the captured client",
			ctx: func() context.Context {
				return context.WithValue(context.Background(), clientContextKey{}, (*Client)(nil))
			},
			want: captured,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := captured.For(tc.ctx()); got != tc.want {
				t.Errorf("For() = %p, want %p", got, tc.want)
			}
		})
	}
}

// TestFor_NilContextAndNilReceiver verifies the two degenerate calls a handler
// can make without the seam becoming a panic.
//
// The nil context is what a caller outside a request has: a watcher polling on
// a goroutine of its own, or a test invoking a handler directly. The nil
// receiver is a caller holding no fallback at all, which resolves whatever the
// context carries and nothing otherwise.
func TestFor_NilContextAndNilReceiver(t *testing.T) {
	t.Parallel()

	captured := NewUnboundClient("https://gitlab.example.com")
	bound := NewUnboundClient("https://gitlab.com")

	//nolint:staticcheck // a nil context is exactly what this asserts is survivable
	if got := captured.For(nil); got != captured {
		t.Errorf("For(nil) = %p, want the receiver %p", got, captured)
	}
	var absent *Client
	if got := absent.For(WithClient(context.Background(), bound)); got != bound {
		t.Errorf("(*Client)(nil).For(bound) = %p, want %p", got, bound)
	}
	if got := absent.For(context.Background()); got != nil {
		t.Errorf("(*Client)(nil).For(unbound) = %p, want nil", got)
	}
}

// TestClientFrom_SaysWhetherAnythingWasBound verifies the accessor a middleware
// needs, which has to tell "nothing is bound" apart from "the fallback was
// returned" before deciding whether it has anything to install.
func TestClientFrom_SaysWhetherAnythingWasBound(t *testing.T) {
	t.Parallel()

	bound := NewUnboundClient("https://gitlab.com")

	cases := []struct {
		name    string
		ctx     func() context.Context
		want    *Client
		wantOK  bool
		wantNil bool
	}{
		{
			name:   "a bound context",
			ctx:    func() context.Context { return WithClient(context.Background(), bound) },
			want:   bound,
			wantOK: true,
		},
		{
			name:    "an unbound context",
			ctx:     context.Background,
			wantNil: true,
		},
		{
			name:    "a nil context",
			ctx:     func() context.Context { return nil },
			wantNil: true,
		},
		{
			// "Bound to a nil client" is not "bound": a middleware told
			// otherwise would install nothing and report that it had.
			name: "a context carrying a typed-nil client",
			ctx: func() context.Context {
				return context.WithValue(context.Background(), clientContextKey{}, (*Client)(nil))
			},
			wantNil: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ClientFrom(tc.ctx())
			if ok != tc.wantOK {
				t.Errorf("ClientFrom() ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantNil {
				if got != nil {
					t.Errorf("ClientFrom() = %p, want nil", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("ClientFrom() = %p, want %p", got, tc.want)
			}
		})
	}
}
