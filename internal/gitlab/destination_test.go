// destination_test.go contains unit tests for the outbound destination guard:
// which addresses a client may open a connection to, and the exemption that
// keeps every self-hosted deployment working.
package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
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
func TestDestinationPolicy_CheckPrivate_TierBDecisions(t *testing.T) {
	tests := []struct {
		name         string
		instance     string
		callerChosen bool
		allowPrivate bool
		addr         string
		offOrigin    bool
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
			instance: "https://gitlab.example.com", addr: "10.0.0.1", offOrigin: true, wantRefused: true,
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
			// Asked again, the answer is memoized: the resolver is consulted on
			// the refusal path, and a client that keeps meeting refusals must
			// not keep paying for DNS.
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
				client.destination.Store(tt.policy)
			}
			var seen *dialTarget
			transport := &destinationTransport{
				base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if target, ok := dialTargetFrom(req.Context()); ok {
						seen = &target
					}
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
				}),
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
