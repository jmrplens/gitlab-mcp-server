//go:build e2e

// transport_test.go pins the address half of the rule that decides whether a
// fixture may put a credential on plain HTTP, which is the half a test can
// check without a GitLab and the half that decides the answer.

package fixture

import (
	"context"
	"net"
	"testing"
)

// TestPrivateAddress_ClassifiesTheOperatorsOwnWire checks the four address
// families a credential may cross in clear text, and that a public address is
// not one of them.
func TestPrivateAddress_ClassifiesTheOperatorsOwnWire(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "loopback v4", ip: "127.0.0.1", want: true},
		{name: "loopback v6", ip: "::1", want: true},
		{name: "private v4", ip: "192.168.0.40", want: true},
		{name: "unique local v6", ip: "fd00::1", want: true},
		{name: "link local", ip: "169.254.1.1", want: true},
		{name: "unspecified", ip: "0.0.0.0", want: true},
		{name: "public v4", ip: "8.8.8.8", want: false},
		{name: "public v6", ip: "2001:4860:4860::8888", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := privateAddress(net.ParseIP(testCase.ip)); got != testCase.want {
				t.Errorf("privateAddress(%s) = %v, want %v", testCase.ip, got, testCase.want)
			}
		})
	}
}

// TestLocalHost_AnswersForHostsItNeedsNoResolverFor checks the three answers
// the check reaches without asking a resolver: an empty host, a literal
// address on the operator's own wire, and a literal public address.
func TestLocalHost_AnswersForHostsItNeedsNoResolverFor(t *testing.T) {
	cases := []struct {
		name string
		host string
		want bool
	}{
		{name: "empty", host: "", want: false},
		{name: "loopback literal", host: "127.0.0.1", want: true},
		{name: "private literal", host: "10.1.2.3", want: true},
		{name: "public literal", host: "203.0.113.7", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := localHost(t.Context(), testCase.host); got != testCase.want {
				t.Errorf("localHost(%q) = %v, want %v", testCase.host, got, testCase.want)
			}
		})
	}
}

// TestLocalHost_NameThatResolvesToNothingIsNotLocal checks that a name the
// resolver refuses is not treated as the operator's own wire: the check must
// fail closed, since a credential is about to be sent over what it admits.
func TestLocalHost_NameThatResolvesToNothingIsNotLocal(t *testing.T) {
	if localHost(t.Context(), "e2e-fixture.invalid") {
		t.Error("localHost admitted a name nothing resolves, want it refused")
	}
}

// TestLocalHost_CancelledContextIsNotLocal checks the same fail-closed answer
// when the resolution cannot be made at all.
func TestLocalHost_CancelledContextIsNotLocal(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if localHost(ctx, "localhost") {
		t.Error("localHost admitted a name it could not resolve, want it refused")
	}
}
