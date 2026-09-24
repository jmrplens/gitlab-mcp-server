// destination_test.go contains unit tests for the outbound destination guard:
// which addresses a client may open a connection to, and the exemption that
// keeps every self-hosted deployment working.
package gitlab

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// dialThroughBaseTransport dials address through the transport every client
// shares, with target stamped on the context the way [destinationTransport]
// stamps it, and returns whatever the dialer reported.
//
// The deadline is short and deliberate. A refused address never reaches the
// network at all — [guardDestination] runs before the connect — so a refusal
// is immediate; the deadline only bounds the one row that is meant to get
// past the guard, whose dial then fails for reasons this test does not care
// about.
func dialThroughBaseTransport(t *testing.T, target *dialTarget, address string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	if target != nil {
		ctx = withDialTarget(ctx, *target)
	}

	conn, err := newBaseTransport(nil).DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}

// callerChosenTarget is the stamp a request carries when the instance itself
// was named by a GITLAB-URL header under --allow-any-gitlab-url.
func callerChosenTarget() *dialTarget {
	return &dialTarget{policy: newDestinationPolicy("http://caller.example.com", true, false)}
}

// TestDestinationGuard_OperatorNamedInstance_IsExempt is the row that protects
// the ordinary self-hosted deployment, and it is written first on purpose.
//
// A great many people run this server against a GitLab on localhost, on 10.x,
// on 192.168.x, or behind a VPN on CGNAT. Every one of those resolves to an
// address tier B refuses, and every one of them must keep working: the guard
// constrains destinations the operator did NOT choose, and an address named by
// --gitlab-url or GITLAB_URL is exempt by construction rather than by an
// exception list. A change that "tightens" this breaks all of them at once,
// which is why the test drives a real request end to end rather than asking
// the policy a question.
func TestDestinationGuard_OperatorNamedInstance_IsExempt(t *testing.T) {
	t.Run("a pinned instance on loopback answers a real request", func(t *testing.T) {
		gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
		}))
		t.Cleanup(gitlab.Close)

		client, err := NewClient(&config.Config{GitLabURL: gitlab.URL, GitLabToken: "glpat-x", DisableRetries: true})
		if err != nil {
			t.Fatalf("NewClient() unexpected error: %v", err)
		}

		version, err := client.Initialize(t.Context())
		if err != nil {
			t.Fatalf("a GitLab the operator pinned on a loopback address was refused: %v", err)
		}
		if version != "17.0.0" {
			t.Errorf("version = %q, want %q", version, "17.0.0")
		}
	})

	t.Run("a pinned instance on an RFC 1918 address is dialed without complaint", func(t *testing.T) {
		policy := newDestinationPolicy("https://gitlab.internal", false, false)
		target := &dialTarget{policy: policy, offOrigin: false}

		if err := dialThroughBaseTransport(t, target, "10.0.0.1:443"); errors.Is(err, ErrDestinationRefused) {
			t.Fatalf("the guard refused an address the operator named: %v", err)
		}
	})

	t.Run("the same address is refused when a caller named the instance", func(t *testing.T) {
		if err := dialThroughBaseTransport(t, callerChosenTarget(), "10.0.0.1:443"); !errors.Is(err, ErrDestinationRefused) {
			t.Fatalf("err = %v, want a refusal: only the operator's own instance is exempt", err)
		}
	})
}

// TestBaseTransport_RefusesPrivateDestinations checks each address class tier B
// covers, through the transport every client dials with.
//
// The hostname row is the one that matters most and is easy to miss: the name
// looks ordinary and only the address it resolves to is loopback, so nothing
// that inspects the URL can refuse it. Only a check made after resolution can,
// which is the whole reason the guard is a ControlContext hook rather than a
// round tripper.
func TestBaseTransport_RefusesPrivateDestinations(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		wantRefused bool
	}{
		{name: "loopback ipv4", address: "127.0.0.1:9", wantRefused: true},
		{name: "loopback ipv6", address: "[::1]:9", wantRefused: true},
		{name: "rfc 1918", address: "10.0.0.1:9", wantRefused: true},
		{name: "cloud metadata", address: "169.254.169.254:80", wantRefused: true},
		{name: "cgnat", address: "100.64.0.1:9", wantRefused: true},
		{name: "hostname that resolves to loopback", address: "localhost:9", wantRefused: true},
		{name: "public address", address: "192.0.2.1:9", wantRefused: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dialThroughBaseTransport(t, callerChosenTarget(), tt.address)

			if got := errors.Is(err, ErrDestinationRefused); got != tt.wantRefused {
				t.Fatalf("refused = %v, want %v (err = %v)", got, tt.wantRefused, err)
			}
		})
	}
}

// TestGuardDestination_TierAAppliesWithNoPolicy verifies that a request
// carrying no policy at all is still refused a cloud metadata address, and
// still allowed everything else.
//
// Tier A is the half that applies to every client and every hop, so it cannot
// depend on a stamp being present: a transport borrowed through
// [HTTPTransport], or a future caller that forgets to stamp, must not become
// the way to reach an instance credentials endpoint.
func TestGuardDestination_TierAAppliesWithNoPolicy(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		wantRefused bool
	}{
		{name: "instance metadata", address: "169.254.169.254:80", wantRefused: true},
		{name: "container credentials", address: "169.254.170.2:80", wantRefused: true},
		{name: "instance metadata over ipv6", address: "[fd00:ec2::254]:80", wantRefused: true},
		{name: "alibaba cloud metadata", address: "100.100.100.200:80", wantRefused: true},
		{name: "metadata written as a mapped ipv4", address: "[::ffff:169.254.169.254]:80", wantRefused: true},
		{name: "an ordinary private address", address: "10.0.0.1:9", wantRefused: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guardDestination(t.Context(), "tcp", tt.address, nil)

			if got := errors.Is(err, ErrDestinationRefused); got != tt.wantRefused {
				t.Fatalf("refused = %v, want %v (err = %v)", got, tt.wantRefused, err)
			}
		})
	}
}

// TestGuardDestination_StampedWithNoPolicy_GetsTierAAndNothingElse covers the
// half of the unstamped branch the absent-stamp test cannot reach: a request
// that WAS stamped, by a policy of nil.
//
// [destinationTransport.RoundTrip] produces exactly this when the client it
// wraps carries no policy, and the two halves of the guard's condition must
// therefore agree: a stamp whose policy is nil is as good as no stamp, because
// there is nothing to apply tier B with. Reading it the other way round would
// dereference the nil policy on every such dial.
//
// The private address is the assertion that matters. A public one would be
// allowed by tier B as well, so it could not tell "tier B was skipped" from
// "tier B ran and permitted it".
func TestGuardDestination_StampedWithNoPolicy_GetsTierAAndNothingElse(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		wantRefused bool
	}{
		{name: "a private address is allowed, since there is no tier B to apply", address: "10.0.0.1:9"},
		{name: "loopback likewise", address: "127.0.0.1:9"},
		{name: "a metadata address is still refused by tier A", address: "169.254.169.254:80", wantRefused: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := withDialTarget(t.Context(), dialTarget{policy: nil})

			err := guardDestination(ctx, "tcp", tt.address, nil)

			if got := errors.Is(err, ErrDestinationRefused); got != tt.wantRefused {
				t.Fatalf("refused = %v, want %v (err = %v)", got, tt.wantRefused, err)
			}
		})
	}
}

// TestGuardDestination_NonAddressInputs covers the two inputs that are not a
// destination: a network this server never dials, and an address string that
// cannot be classified.
//
// They go opposite ways on purpose. A unix socket carries a path and has no
// address to judge, so refusing it would break a transport nobody was worried
// about; an address that does not parse is a destination we cannot classify,
// and a guard that waves those through is one bad string away from having no
// opinion at all.
func TestGuardDestination_NonAddressInputs(t *testing.T) {
	tests := []struct {
		name        string
		network     string
		address     string
		wantRefused bool
	}{
		{name: "unix socket", network: "unix", address: "/run/gitlab.sock", wantRefused: false},
		{name: "tcp4 is still tcp", network: "tcp4", address: "169.254.169.254:80", wantRefused: true},
		{name: "unparseable address", network: "tcp", address: "not-an-address", wantRefused: true},
		{name: "host name where an address was promised", network: "tcp", address: "gitlab.example.com:443", wantRefused: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guardDestination(t.Context(), tt.network, tt.address, nil)

			if got := errors.Is(err, ErrDestinationRefused); got != tt.wantRefused {
				t.Fatalf("refused = %v, want %v (err = %v)", got, tt.wantRefused, err)
			}
		})
	}
}

// TestIsPrivateAddress_ClassifiesTheRefusedRanges pins the address classes the
// guard treats as private, including the two spellings that are easy to get
// wrong: an IPv4 address written as a mapped IPv6 one, and CGNAT, which
// [netip.Addr.IsPrivate] does not cover.
func TestIsPrivateAddress_ClassifiesTheRefusedRanges(t *testing.T) {
	tests := []struct {
		name    string
		addr    netip.Addr
		want    bool
		because string
	}{
		{name: "loopback ipv4", addr: netip.MustParseAddr("127.0.0.1"), want: true, because: "loopback"},
		{name: "loopback ipv6", addr: netip.MustParseAddr("::1"), want: true, because: "loopback"},
		{name: "rfc 1918 ten", addr: netip.MustParseAddr("10.1.2.3"), want: true, because: "RFC 1918"},
		{name: "rfc 1918 one nine two", addr: netip.MustParseAddr("192.168.1.1"), want: true, because: "RFC 1918"},
		{name: "rfc 1918 one seven two", addr: netip.MustParseAddr("172.16.0.1"), want: true, because: "RFC 1918"},
		{name: "cgnat", addr: netip.MustParseAddr("100.64.0.1"), want: true, because: "RFC 6598 shared address space"},
		{name: "cgnat upper bound", addr: netip.MustParseAddr("100.127.255.255"), want: true, because: "RFC 6598 shared address space"},
		{name: "link local unicast", addr: netip.MustParseAddr("169.254.1.1"), want: true, because: "link-local"},
		{name: "link local multicast", addr: netip.MustParseAddr("224.0.0.1"), want: true, because: "link-local multicast"},
		{name: "unique local", addr: netip.MustParseAddr("fd00::1"), want: true, because: "RFC 4193 unique local"},
		{name: "unspecified ipv4", addr: netip.MustParseAddr("0.0.0.0"), want: true, because: "unspecified"},
		{name: "unspecified ipv6", addr: netip.MustParseAddr("::"), want: true, because: "unspecified"},
		{name: "rfc 1918 as a mapped ipv6", addr: netip.MustParseAddr("::ffff:10.0.0.1"), want: true, because: "the same address written differently"},
		{name: "cgnat as a mapped ipv6", addr: netip.MustParseAddr("::ffff:100.64.0.1"), want: true, because: "the same address written differently"},
		{name: "invalid", addr: netip.Addr{}, want: true, because: "an address that cannot be classified is not one to dial"},
		{name: "public ipv4", addr: netip.MustParseAddr("192.0.2.1"), want: false, because: "documentation range, but publicly routable in form"},
		{name: "public ipv6", addr: netip.MustParseAddr("2001:db8::1"), want: false, because: "documentation range, but publicly routable in form"},
		{name: "just above cgnat", addr: netip.MustParseAddr("100.128.0.1"), want: false, because: "outside 100.64.0.0/10"},
		{name: "just below cgnat", addr: netip.MustParseAddr("100.63.255.255"), want: false, because: "outside 100.64.0.0/10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPrivateAddress(tt.addr); got != tt.want {
				t.Errorf("isPrivateAddress(%v) = %v, want %v (%s)", tt.addr, got, tt.want, tt.because)
			}
		})
	}
}

// TestDestinationPolicy_CheckPrivate_TierBDecisions walks every way tier B can
// resolve, so each branch is asserted in both directions rather than by the one
// case that happens to be interesting.
//
// A case whose instance is spelled as a name says what that name resolves to,
// in `resolves`, because the branch it lands in asks: an off-origin hop to a
// private address is refused only where the instance itself is not private,
// and answering that means resolving the instance host. Left to the real
// resolver, the one case that reaches it paid a two-second
// [instanceLookupTimeout] and then passed on the lookup having failed, which
// is the same verdict for a different reason — a resolver that started
// answering for gitlab.example.com would have decided the case instead.
func TestDestinationPolicy_CheckPrivate_TierBDecisions(t *testing.T) {
	tests := []struct {
		name         string
		instance     string
		callerChosen bool
		allowPrivate bool
		addr         string
		offOrigin    bool
		resolves     []string
		wantRefused  bool
	}{
		{
			name:     "operator named instance, on origin, private",
			instance: "https://gitlab.internal", addr: "10.0.0.1", wantRefused: false,
		},
		{
			name:     "operator named instance, on origin, public",
			instance: "https://gitlab.example.com", addr: "203.0.113.1", wantRefused: false,
		},
		{
			name:     "caller named instance, on origin, private",
			instance: "https://gitlab.internal", callerChosen: true, addr: "10.0.0.1", wantRefused: true,
		},
		{
			name:     "caller named instance, on origin, public",
			instance: "https://gitlab.example.com", callerChosen: true, addr: "203.0.113.1", wantRefused: false,
		},
		{
			name:     "caller named instance, private, opted out",
			instance: "https://gitlab.internal", callerChosen: true, allowPrivate: true, addr: "10.0.0.1", wantRefused: false,
		},
		{
			name:     "redirect off a public instance to a private address",
			instance: "https://gitlab.example.com", addr: "10.0.0.1", offOrigin: true,
			resolves: []string{"203.0.113.1"}, wantRefused: true,
		},
		{
			name:     "redirect off a public instance to a public address",
			instance: "https://gitlab.example.com", addr: "203.0.113.1", offOrigin: true, wantRefused: false,
		},
		{
			name:     "redirect off a private instance to a private address",
			instance: "https://127.0.0.1", addr: "10.0.0.1", offOrigin: true, wantRefused: false,
		},
		{
			name:     "redirect off a public instance to a private address, opted out",
			instance: "https://gitlab.example.com", allowPrivate: true, addr: "10.0.0.1", offOrigin: true, wantRefused: false,
		},
		{
			name:     "redirect with no instance host to compare against",
			instance: "not a url at all", addr: "10.0.0.1", offOrigin: true, wantRefused: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newDestinationPolicy(tt.instance, tt.callerChosen, tt.allowPrivate)
			// Never the real resolver, for any case: a lookup of a name this
			// test invented waits out instanceLookupTimeout and is then
			// decided by whatever the host's DNS happens to say. A case that
			// reaches one without declaring what it resolves to is reporting
			// that it lands in a branch it did not mean to.
			policy.lookupIP = func(_ context.Context, host string) ([]netip.Addr, error) {
				if len(tt.resolves) == 0 {
					t.Errorf("the policy resolved %q, and this case declares no addresses for it", host)
					return nil, errors.New("undeclared lookup")
				}
				addrs := make([]netip.Addr, 0, len(tt.resolves))
				for _, raw := range tt.resolves {
					addrs = append(addrs, netip.MustParseAddr(raw))
				}
				return addrs, nil
			}

			err := policy.checkPrivate(t.Context(), netip.MustParseAddr(tt.addr), tt.offOrigin)

			if got := errors.Is(err, ErrDestinationRefused); got != tt.wantRefused {
				t.Fatalf("refused = %v, want %v (err = %v)", got, tt.wantRefused, err)
			}
		})
	}
}

// TestDestinationPolicy_Refusal_NamesTheCause verifies that the two ways a
// destination can be refused read differently.
//
// An operator meeting one of these has a different thing to fix in each case:
// a header they pointed at their own network, or an instance redirecting
// somewhere they did not expect. A single message for both leaves them
// checking the wrong half.
//
// The cause names what happened and not what to do about it. The flag that
// changes the outcome is the hint's job, added once by
// [toolutil.WrapErr] and its siblings, which is where every other actionable
// suggestion in this server lives (ADR-0007). The one message that carries its
// own advice is [CheckCallerNamedInstance]'s, because an HTTP 400 body has no
// hint layer above it.
func TestDestinationPolicy_Refusal_NamesTheCause(t *testing.T) {
	tests := []struct {
		name      string
		instance  string
		offOrigin bool
		wantText  []string
	}{
		{
			name: "caller named instance", instance: "https://gitlab.example.com",
			wantText: []string{"GITLAB-URL", "10.0.0.1"},
		},
		{
			name: "redirect hop", instance: "https://gitlab.example.com", offOrigin: true,
			wantText: []string{"redirect", "gitlab.example.com", "10.0.0.1"},
		},
		{
			name: "redirect hop with no instance host", instance: "", offOrigin: true,
			wantText: []string{"redirect", "the configured instance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newDestinationPolicy(tt.instance, true, false)

			err := policy.refusal(netip.MustParseAddr("10.0.0.1"), tt.offOrigin)

			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal does not mention %q: %v", want, err)
				}
			}
		})
	}
}

// TestDestinationPolicy_InstanceIsPrivate answers the question the
// self-managed object-store exemption rests on: is the configured instance
// itself inside a private network?
//
// The mixed row is the one worth having. A name that answers with one private
// and one public address is not a private deployment, and deciding from the
// first address returned would make the exemption depend on resolver ordering.
func TestDestinationPolicy_InstanceIsPrivate(t *testing.T) {
	tests := []struct {
		name      string
		instance  string
		resolved  []string
		lookupErr error
		noLookup  bool
		want      bool
	}{
		{name: "literal private address", instance: "https://10.0.0.1", want: true},
		{name: "literal loopback address", instance: "http://127.0.0.1:8080", want: true},
		{name: "literal public address", instance: "https://203.0.113.1", want: false},
		{name: "no host at all", instance: "not a url at all", want: false},
		{name: "name resolving to a private address", instance: "https://gitlab.internal", resolved: []string{"10.0.0.1"}, want: true},
		{name: "name resolving to a public address", instance: "https://gitlab.example.com", resolved: []string{"203.0.113.1"}, want: false},
		{name: "name resolving to both", instance: "https://gitlab.example.com", resolved: []string{"10.0.0.1", "203.0.113.1"}, want: false},
		{name: "name that does not resolve", instance: "https://gitlab.example.com", lookupErr: errors.New("no such host"), want: false},
		{name: "name resolving to nothing", instance: "https://gitlab.example.com", resolved: []string{}, want: false},
		{name: "no resolver available", instance: "https://gitlab.example.com", noLookup: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newDestinationPolicy(tt.instance, false, false)
			calls := 0
			switch {
			case tt.noLookup:
				policy.lookupIP = nil
			default:
				policy.lookupIP = func(_ context.Context, _ string) ([]netip.Addr, error) {
					calls++
					if tt.lookupErr != nil {
						return nil, tt.lookupErr
					}
					addrs := make([]netip.Addr, 0, len(tt.resolved))
					for _, raw := range tt.resolved {
						addrs = append(addrs, netip.MustParseAddr(raw))
					}
					return addrs, nil
				}
			}

			if got := policy.instanceIsPrivate(t.Context()); got != tt.want {
				t.Fatalf("instanceIsPrivate() = %v, want %v", got, tt.want)
			}
			// Asked again, the answer is memoized: the resolver is consulted
			// whenever a hop that leaves the instance is routed, and a client
			// whose downloads keep leaving it must not keep paying for DNS.
			if got := policy.instanceIsPrivate(t.Context()); got != tt.want {
				t.Fatalf("instanceIsPrivate() on the second call = %v, want %v", got, tt.want)
			}
			if calls > 1 {
				t.Errorf("the resolver was consulted %d times, want at most one", calls)
			}
		})
	}
}

// TestDestinationPolicy_CoversInstance verifies which destinations count as
// the instance's own host, which is what decides whether a redirect hop is
// judged at all.
func TestDestinationPolicy_CoversInstance(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		dest     string
		want     bool
	}{
		{name: "the instance itself", instance: "https://gitlab.example.com", dest: "https://gitlab.example.com/x", want: true},
		{name: "a subdomain of it", instance: "https://gitlab.example.com", dest: "https://storage.gitlab.example.com/x", want: true},
		{name: "a different port", instance: "https://gitlab.example.com", dest: "https://gitlab.example.com:9443/x", want: true},
		{name: "an https to http downgrade", instance: "https://gitlab.example.com", dest: "http://gitlab.example.com/x", want: true},
		{name: "another host", instance: "https://gitlab.example.com", dest: "https://storage.example.net/x", want: false},
		{name: "a suffix without a dot boundary", instance: "https://gitlab.example.com", dest: "https://evilgitlab.example.com/x", want: false},
		{name: "an ipv6 zone spelled as a subdomain", instance: "https://gitlab.example.com", dest: "https://[::1%25.gitlab.example.com]/x", want: false},
		{name: "no instance host", instance: "", dest: "https://gitlab.example.com/x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newDestinationPolicy(tt.instance, false, false)
			dest, err := url.Parse(tt.dest)
			if err != nil {
				t.Fatalf("url.Parse(%q) unexpected error: %v", tt.dest, err)
			}

			if got := policy.coversInstance(dest); got != tt.want {
				t.Errorf("coversInstance(%q) = %v, want %v", tt.dest, got, tt.want)
			}
		})
	}
}

// TestDestinationPolicy_CoversInstance_NilInputs verifies the two nil cases a
// table over URLs cannot express.
func TestDestinationPolicy_CoversInstance_NilInputs(t *testing.T) {
	t.Run("nil policy", func(t *testing.T) {
		var policy *destinationPolicy
		if policy.coversInstance(&url.URL{Host: "gitlab.example.com"}) {
			t.Error("a nil policy covers no instance")
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		policy := newDestinationPolicy("https://gitlab.example.com", false, false)
		if policy.coversInstance(nil) {
			t.Error("a request with no URL is not the instance's own host")
		}
	})
}

// TestClient_MarkInstanceCallerNamed verifies that the pool can move a client
// under tier B, and that a client with no policy is left alone rather than
// given one.
func TestClient_MarkInstanceCallerNamed(t *testing.T) {
	t.Run("a built client moves under tier B", func(t *testing.T) {
		client, err := NewClientWithToken("https://gitlab.example.com", "glpat-x", false)
		if err != nil {
			t.Fatalf("NewClientWithToken() unexpected error: %v", err)
		}
		if client.destinationPolicy().callerChosen {
			t.Fatal("a client is operator-named until the pool says otherwise")
		}

		client.MarkInstanceCallerNamed()

		policy := client.destinationPolicy()
		if !policy.callerChosen {
			t.Error("the client was not moved under tier B")
		}
		if policy.instanceHost != "gitlab.example.com" {
			t.Errorf("instanceHost = %q, want the client's own host", policy.instanceHost)
		}
	})

	t.Run("the opt-out survives the move", func(t *testing.T) {
		client, err := NewClientWithToken("https://gitlab.example.com", "glpat-x", false)
		if err != nil {
			t.Fatalf("NewClientWithToken() unexpected error: %v", err)
		}
		client.destination.Store(newDestinationPolicy("https://gitlab.example.com", false, true))

		client.MarkInstanceCallerNamed()

		if !client.destinationPolicy().allowPrivate {
			t.Error("marking the instance caller-named silently dropped --allow-private-instances")
		}
	})

	t.Run("a client with no policy is left alone", func(t *testing.T) {
		client := NewUnboundClient("https://gitlab.invalid")

		client.MarkInstanceCallerNamed()

		if client.destinationPolicy() != nil {
			t.Error("a client that dials nothing was given a policy")
		}
	})

	t.Run("a nil client does not panic", func(t *testing.T) {
		var client *Client
		client.MarkInstanceCallerNamed()
		if client.destinationPolicy() != nil {
			t.Error("a nil client has no policy")
		}
	})
}

// TestDestinationTransport_StampsThePolicy verifies that each request carries
// the decision the dialer will read, and that a client with no policy passes
// through untouched.
func TestDestinationTransport_StampsThePolicy(t *testing.T) {
	tests := []struct {
		name          string
		policy        *destinationPolicy
		requestURL    string
		wantStamped   bool
		wantOffOrigin bool
	}{
		{
			name:       "a request to the instance itself",
			policy:     newDestinationPolicy("https://gitlab.example.com", false, false),
			requestURL: "https://gitlab.example.com/api/v4/user", wantStamped: true, wantOffOrigin: false,
		},
		{
			name:       "a hop that left the instance",
			policy:     newDestinationPolicy("https://gitlab.example.com", false, false),
			requestURL: "https://storage.example.net/artifact", wantStamped: true, wantOffOrigin: true,
		},
		{
			name:       "a client with no policy",
			policy:     nil,
			requestURL: "https://gitlab.example.com/api/v4/user", wantStamped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			if tt.policy != nil {
				// Routing a hop that left the instance asks whether the
				// instance is itself private; the answer is irrelevant to the
				// stamp, and the real resolver is never what decides a unit
				// test.
				tt.policy.lookupIP = resolverAnswering("203.0.113.1")
				client.destination.Store(tt.policy)
			}
			var seen *dialTarget
			record := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if target, ok := dialTargetFrom(req.Context()); ok {
					seen = &target
				}
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})
			transport := &destinationTransport{
				pools:  destinationPools{permissive: record, strict: record},
				client: client,
			}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tt.requestURL, http.NoBody)
			if err != nil {
				t.Fatalf("http.NewRequestWithContext() unexpected error: %v", err)
			}
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() unexpected error: %v", err)
			}
			_ = resp.Body.Close()

			if (seen != nil) != tt.wantStamped {
				t.Fatalf("stamped = %v, want %v", seen != nil, tt.wantStamped)
			}
			if seen != nil && seen.offOrigin != tt.wantOffOrigin {
				t.Errorf("offOrigin = %v, want %v", seen.offOrigin, tt.wantOffOrigin)
			}
		})
	}
}

// roundTripFunc adapts a function to [http.RoundTripper].
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls the wrapped function.
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// resolverAnswering is a stand-in for the instance lookup that answers every
// name with addrs.
func resolverAnswering(addrs ...string) func(context.Context, string) ([]netip.Addr, error) {
	return func(context.Context, string) ([]netip.Addr, error) {
		parsed := make([]netip.Addr, 0, len(addrs))
		for _, raw := range addrs {
			parsed = append(parsed, netip.MustParseAddr(raw))
		}
		return parsed, nil
	}
}

// TestDestinationPolicy_PermitsPrivate_IsWhatTheDialerApplies walks every way
// tier B can answer the one question the pool is chosen by, and holds the
// dialer to the same answer.
//
// The agreement is the property the routing rests on. A request is served from
// the strict pool exactly when this answers false, and a connection is only
// safe to hand it if that connection was dialed under a false answer too; a
// predicate the dialer and the router each kept a copy of could drift, and the
// drift would reopen reuse as a way past the guard. So each row also asks the
// dialer's check about a private address, which must be refused exactly when
// this answers false, and about a public one, which no answer refuses.
//
// The lookups are counted because asking is not free: a request that stays on
// the operator's own instance, or a deployment that opted out, must not pay a
// DNS question to be routed, and one that does ask must ask once however many
// times its policy is consulted.
func TestDestinationPolicy_PermitsPrivate_IsWhatTheDialerApplies(t *testing.T) {
	tests := []struct {
		name         string
		instance     string
		callerChosen bool
		allowPrivate bool
		offOrigin    bool
		resolves     []string
		want         bool
		wantLookups  int
	}{
		{name: "the operator's own instance", instance: "https://gitlab.example.com", want: true},
		{name: "an instance a caller named", instance: "https://gitlab.example.com", callerChosen: true, want: false},
		{name: "an instance a caller named, opted out", instance: "https://gitlab.example.com", callerChosen: true, allowPrivate: true, want: true},
		{
			name: "a hop away from a public instance", instance: "https://gitlab.example.com", offOrigin: true,
			resolves: []string{"203.0.113.1"}, want: false, wantLookups: 1,
		},
		{
			name: "a hop away from a private instance", instance: "https://gitlab.internal", offOrigin: true,
			resolves: []string{"10.0.0.1"}, want: true, wantLookups: 1,
		},
		{name: "a hop away from an instance spelled as a private address", instance: "https://10.0.0.1", offOrigin: true, want: true},
		{name: "a hop away from an instance spelled as a public address", instance: "https://203.0.113.1", offOrigin: true, want: false},
		{name: "a hop away from a public instance, opted out", instance: "https://gitlab.example.com", allowPrivate: true, offOrigin: true, want: true},
		{
			name: "a hop away from an instance a caller named", instance: "https://gitlab.internal", callerChosen: true, offOrigin: true,
			resolves: []string{"10.0.0.1"}, want: true, wantLookups: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newDestinationPolicy(tt.instance, tt.callerChosen, tt.allowPrivate)
			lookups := 0
			policy.lookupIP = func(ctx context.Context, host string) ([]netip.Addr, error) {
				lookups++
				if len(tt.resolves) == 0 {
					t.Errorf("the policy resolved %q, and this case expects no question to be asked", host)
					return nil, errors.New("undeclared lookup")
				}
				return resolverAnswering(tt.resolves...)(ctx, host)
			}

			if got := policy.permitsPrivate(t.Context(), tt.offOrigin); got != tt.want {
				t.Fatalf("permitsPrivate() = %v, want %v", got, tt.want)
			}
			privateErr := policy.checkPrivate(t.Context(), netip.MustParseAddr("10.0.0.1"), tt.offOrigin)
			if refused := errors.Is(privateErr, ErrDestinationRefused); refused == tt.want {
				t.Errorf("the dialer refused a private address = %v while the router permits one = %v; the two must be one answer", refused, tt.want)
			}
			if err := policy.checkPrivate(t.Context(), netip.MustParseAddr("203.0.113.1"), tt.offOrigin); err != nil {
				t.Errorf("the dialer refused a public address: %v", err)
			}
			if lookups != tt.wantLookups {
				t.Errorf("the instance was resolved %d times, want %d", lookups, tt.wantLookups)
			}
		})
	}
}

// TestDestinationPolicy_InstanceIsPrivate_OutlivesTheCallersCancellation
// verifies that the first request to ask whether the instance is private
// cannot decide the answer by having given up.
//
// The answer is memoized for the life of the client and is asked when a
// redirect hop is routed, which net/http does without checking whether the
// hop's context has already ended. Inheriting that cancellation made the
// lookup fail, the failure was recorded as "not private", and a self-managed
// GitLab whose object store sits beside it on a private network then had every
// later artifact download refused, for a request nobody was waiting on.
func TestDestinationPolicy_InstanceIsPrivate_OutlivesTheCallersCancellation(t *testing.T) {
	policy := newDestinationPolicy("https://gitlab.internal", false, false)
	policy.lookupIP = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	if !policy.instanceIsPrivate(cancelled) {
		t.Fatal("instanceIsPrivate() = false: the lookup inherited a cancelled caller and recorded its failure as the answer")
	}
	if !policy.instanceIsPrivate(t.Context()) {
		t.Error("instanceIsPrivate() on a later request = false, want the answer the first one recorded")
	}
}

// TestDestinationTransport_RoutesByWhatTierBAnswers verifies which pool each
// kind of request is served from.
//
// The operator's own instance is the row that protects the cost: it is every
// first-party request of every ordinary deployment, and it must stay on the
// one pool those requests always shared. The strict rows are the ones that
// protect the guard, since a request routed to the permissive pool can be
// handed a connection to a private address without a dial.
func TestDestinationTransport_RoutesByWhatTierBAnswers(t *testing.T) {
	const (
		instanceURL = "https://gitlab.example.com"
		ownRequest  = "https://gitlab.example.com/api/v4/version"
		hopRequest  = "https://storage.example.net/artifact"
	)
	tests := []struct {
		name         string
		noPolicy     bool
		callerChosen bool
		allowPrivate bool
		resolves     string
		requestURL   string
		want         string
	}{
		{name: "the operator's own instance", requestURL: ownRequest, want: "permissive"},
		{name: "an instance a caller named", callerChosen: true, requestURL: ownRequest, want: "strict"},
		{name: "an instance a caller named, opted out", callerChosen: true, allowPrivate: true, requestURL: ownRequest, want: "permissive"},
		{name: "a hop away from a public instance", resolves: "203.0.113.1", requestURL: hopRequest, want: "strict"},
		{name: "a hop away from a private instance", resolves: "10.0.0.1", requestURL: hopRequest, want: "permissive"},
		{name: "a hop away from a public instance, opted out", allowPrivate: true, requestURL: hopRequest, want: "permissive"},
		{name: "a client with no policy", noPolicy: true, requestURL: ownRequest, want: "permissive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			if !tt.noPolicy {
				policy := newDestinationPolicy(instanceURL, tt.callerChosen, tt.allowPrivate)
				policy.lookupIP = func(context.Context, string) ([]netip.Addr, error) {
					if tt.resolves == "" {
						t.Error("routing resolved the instance, and this case expects no question to be asked")
						return nil, errors.New("undeclared lookup")
					}
					return []netip.Addr{netip.MustParseAddr(tt.resolves)}, nil
				}
				client.destination.Store(policy)
			}
			served := ""
			poolNamed := func(name string) http.RoundTripper {
				return roundTripFunc(func(req *http.Request) (*http.Response, error) {
					served = name
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
				})
			}
			transport := &destinationTransport{
				pools:  destinationPools{permissive: poolNamed("permissive"), strict: poolNamed("strict")},
				client: client,
			}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tt.requestURL, http.NoBody)
			if err != nil {
				t.Fatalf("http.NewRequestWithContext() unexpected error: %v", err)
			}
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() unexpected error: %v", err)
			}
			_ = resp.Body.Close()

			if served != tt.want {
				t.Errorf("served from the %s pool, want the %s pool", served, tt.want)
			}
		})
	}
}

// versionAnswer is what the loopback stubs below answer every request with: a
// version document, which is all [Client.Ping] asks for.
func versionAnswer(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
}

// connCountingServer starts a loopback server that counts the connections
// opened to it, which is how the tests below tell a reused connection from a
// dialed one.
func connCountingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var opened atomic.Int64
	server := httptest.NewUnstartedServer(handler)
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	return server, &opened
}

// leaveIdleConnection pings url twice through an operator-named client, and
// fails unless the second ping reused the connection the first one opened.
//
// That premise is asserted rather than assumed because the tests below can
// only fail if there is an idle connection for the guard to be walked past: a
// transport that stopped keeping connections alive would make them pass for a
// reason that has nothing to do with the routing.
func leaveIdleConnection(t *testing.T, url string, opened *atomic.Int64) {
	t.Helper()
	holder, err := NewClientWithTokenRetries(url, "glpat-holder", false, true)
	if err != nil {
		t.Fatalf("NewClientWithTokenRetries() unexpected error: %v", err)
	}
	for range 2 {
		if _, err = holder.Ping(t.Context()); err != nil {
			t.Fatalf("the operator-named client was refused its own instance: %v", err)
		}
	}
	if n := opened.Load(); n != 1 {
		t.Fatalf("two pings opened %d connections, want 1 reused: with no idle connection left behind, nothing here tests reuse", n)
	}
}

// TestDestinationPools_CallerNamedClient_IsNotHandedAnOperatorNamedConnection
// is the reuse the destination guard used to miss, on the first hop.
//
// An operator-named client for a loopback GitLab leaves a connection idle, and
// a client for the same URL whose instance a caller named asks the same host.
// The dialer would refuse the second, since a caller-named instance on a
// private address is tier B's own case; but on one shared pool nothing was
// dialed, and the idle connection answered. The connection count is the other
// half of the claim: a refusal happens before the connect, so the loopback
// server must see no second connection.
func TestDestinationPools_CallerNamedClient_IsNotHandedAnOperatorNamedConnection(t *testing.T) {
	t.Setenv(AllowPrivateInstancesEnv, "")
	gitlab, opened := connCountingServer(t, versionAnswer)
	leaveIdleConnection(t, gitlab.URL, opened)

	caller, err := NewClientWithTokenRetries(gitlab.URL, "glpat-caller", false, true)
	if err != nil {
		t.Fatalf("NewClientWithTokenRetries() unexpected error: %v", err)
	}
	caller.MarkInstanceCallerNamed()

	_, err = caller.Ping(t.Context())

	if !errors.Is(err, ErrDestinationRefused) {
		t.Fatalf("err = %v, want a refusal: a caller-named instance on loopback was served the operator-named client's idle connection", err)
	}
	if n := opened.Load(); n != 1 {
		t.Errorf("the loopback server saw %d connections, want the 1 the operator-named client opened", n)
	}
}

// TestDestinationPools_RedirectOffAPublicInstance_IsNotHandedAnotherClientsConnection
// is the same reuse on a redirect hop, which is the shape that reaches it on a
// deployment nobody misconfigured.
//
// One client holds an idle connection to a private object store it was
// configured for. Another client's instance is public, and answers with a
// redirect to that store; the hop leaves the instance for a private address,
// which tier B refuses, and before the split the hop was served the first
// client's connection instead. The public instance is spelled as a name and
// told that name resolves publicly, because an instance spelled as a loopback
// address would be private and would rightly permit the hop.
func TestDestinationPools_RedirectOffAPublicInstance_IsNotHandedAnotherClientsConnection(t *testing.T) {
	t.Setenv(AllowPrivateInstancesEnv, "")
	store, opened := connCountingServer(t, versionAnswer)
	leaveIdleConnection(t, store.URL, opened)

	var redirects atomic.Int64
	// Every request is sent to the store's version document, which is the
	// one thing Ping asks for; a target built from the request would be an
	// open redirect in the stub itself.
	storeVersion := store.URL + versionAPIPath
	instance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirects.Add(1)
		http.Redirect(w, r, storeVersion, http.StatusFound)
	}))
	t.Cleanup(instance.Close)
	_, port, err := net.SplitHostPort(instance.Listener.Addr().String())
	if err != nil {
		t.Fatalf("net.SplitHostPort() unexpected error: %v", err)
	}
	instanceURL := "http://localhost:" + port

	public, err := NewClientWithTokenRetries(instanceURL, "glpat-public", false, true)
	if err != nil {
		t.Fatalf("NewClientWithTokenRetries() unexpected error: %v", err)
	}
	policy := newDestinationPolicy(instanceURL, false, false)
	policy.lookupIP = resolverAnswering("203.0.113.1")
	public.destination.Store(policy)

	_, err = public.Ping(t.Context())

	if !errors.Is(err, ErrDestinationRefused) {
		t.Fatalf("err = %v, want a refusal: a redirect off a public instance was served another client's idle connection to a private address", err)
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("refusal = %v, want the redirect named as its cause", err)
	}
	if n := redirects.Load(); n != 1 {
		t.Errorf("the public instance answered %d requests, want the 1 whose redirect was refused", n)
	}
	if n := opened.Load(); n != 1 {
		t.Errorf("the private store saw %d connections, want the 1 the client configured for it opened", n)
	}
}

// TestAllowPrivateInstances_ReadsTheEnvironment verifies the tier B opt-out,
// including the decision that a value which does not parse leaves the guard on.
//
// A typo in a variable that turns a guard off is the failure nobody notices,
// so the safe direction is the one an unreadable value takes.
func TestAllowPrivateInstances_ReadsTheEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "unset", value: "", want: false},
		{name: "true", value: "true", want: true},
		{name: "one", value: "1", want: true},
		{name: "padded", value: "  true  ", want: true},
		{name: "false", value: "false", want: false},
		{name: "nonsense", value: "yes please", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(AllowPrivateInstancesEnv, tt.value)

			if got := allowPrivateInstances(); got != tt.want {
				t.Errorf("allowPrivateInstances() = %v with %q, want %v", got, tt.value, tt.want)
			}
		})
	}
}

// TestCheckCallerNamedInstance_RefusesWhatTheDialerWould verifies the early
// refusal the HTTP gate makes, and the deliberate limit of it: a host spelled
// as a name is left entirely to the dialer, because only the dialer sees what
// the name resolved to.
func TestCheckCallerNamedInstance_RefusesWhatTheDialerWould(t *testing.T) {
	tests := []struct {
		name        string
		rawURL      string
		wantRefused bool
		wantText    string
	}{
		{name: "cloud metadata", rawURL: "http://169.254.169.254", wantRefused: true, wantText: "metadata"},
		{name: "loopback", rawURL: "http://127.0.0.1:8080", wantRefused: true, wantText: "--allow-private-instances"},
		{name: "rfc 1918", rawURL: "https://10.0.0.1", wantRefused: true, wantText: "--allow-private-instances"},
		{name: "cgnat", rawURL: "https://100.64.0.1", wantRefused: true, wantText: "--allow-private-instances"},
		{name: "public address", rawURL: "https://203.0.113.1", wantRefused: false},
		{name: "a host name, left to the dialer", rawURL: "https://localhost:8080", wantRefused: false},
		{name: "no host", rawURL: "not a url at all", wantRefused: false},
		{name: "empty", rawURL: "", wantRefused: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckCallerNamedInstance(tt.rawURL)

			if got := errors.Is(err, ErrDestinationRefused); got != tt.wantRefused {
				t.Fatalf("refused = %v, want %v (err = %v)", got, tt.wantRefused, err)
			}
			if tt.wantText != "" && !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("refusal does not mention %q: %v", tt.wantText, err)
			}
		})
	}
}

// TestCheckCallerNamedInstance_OptOut verifies that --allow-private-instances
// admits a private instance at the door and still does not admit a metadata
// address, which is the one thing that flag is not a claim about.
func TestCheckCallerNamedInstance_OptOut(t *testing.T) {
	t.Setenv(AllowPrivateInstancesEnv, "true")

	t.Run("a private instance is admitted", func(t *testing.T) {
		if err := CheckCallerNamedInstance("http://127.0.0.1:8080"); err != nil {
			t.Errorf("the opt-out did not admit a private instance: %v", err)
		}
	})

	t.Run("a metadata address is still refused", func(t *testing.T) {
		if err := CheckCallerNamedInstance("http://169.254.169.254"); !errors.Is(err, ErrDestinationRefused) {
			t.Errorf("err = %v, want a refusal: tier A is not what the flag permits", err)
		}
	})
}
