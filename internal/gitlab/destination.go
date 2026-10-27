// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/idna"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// ErrDestinationRefused reports that this server declined to open a connection
// to an address, before any packet was sent to it.
//
// It is a refusal by this server rather than an answer from anything upstream,
// which is the distinction a reader of the message needs: nothing was reached,
// nothing was disclosed, and the fix is configuration rather than credentials.
var ErrDestinationRefused = errors.New("outbound destination refused")

// ErrMetadataDestinationRefused is the refusal of tier A: a cloud metadata
// address, which no configuration permits.
//
// It is a sentinel of its own so that the next step offered with the refusal
// can differ from tier B's, which names the flag that permits a private
// address; naming that flag here only invites a reader to try it and find it
// did nothing. It unwraps to [ErrDestinationRefused], so everything that
// recognizes a refusal still does, and its text is that error's, so the
// message a reader sees is the one every refusal always had.
var ErrMetadataDestinationRefused error = metadataRefusalError{}

// metadataRefusalError is the type of [ErrMetadataDestinationRefused].
type metadataRefusalError struct{}

// Error returns the text every destination refusal starts with.
func (metadataRefusalError) Error() string { return ErrDestinationRefused.Error() }

// Unwrap makes a tier A refusal an [ErrDestinationRefused] as well.
func (metadataRefusalError) Unwrap() error { return ErrDestinationRefused }

// MetadataRefusedHint is the next step offered with a tier A refusal. It names
// no flag, because none changes the outcome, and sends the reader to where the
// address came from instead.
//
// One literal rather than two joined with +, so the mutation gate has no
// operator in a constant initializer to report as not covered.
const MetadataRefusedHint = "no setting permits a cloud metadata address, since nothing serves a GitLab API or object storage from one; check the instance URL and where the redirect that reached it pointed"

// DestinationRefusedHintFor returns the next step to offer with err:
// [MetadataRefusedHint] for a tier A refusal, and [DestinationRefusedHint],
// which names the flag, for every other.
func DestinationRefusedHintFor(err error) string {
	if errors.Is(err, ErrMetadataDestinationRefused) {
		return MetadataRefusedHint
	}
	return DestinationRefusedHint
}

// AllowPrivateInstancesEnv names the setting that permits a destination a
// caller or a redirect chose to resolve to a private, loopback, CGNAT,
// link-local, unique-local or unspecified address.
//
// It is the opt-out for tier B and only tier B: the cloud metadata addresses
// are refused whatever it says, because no value of this setting is a claim
// that a GitLab API or a presigned object store is served from one.
//
// The `--allow-private-instances` flag writes this variable, so there stays
// one reader; see cmd/server/env_flags.go for why that indirection exists.
const AllowPrivateInstancesEnv = "GITLAB_MCP_ALLOW_PRIVATE_INSTANCES"

// allowPrivateInstancesSetting is the same variable without its prefix, the
// name [config.TrimmedGetenv] reads it under. Handed the full name, which the
// configuration package's list cannot hold, the helper read it verbatim
// until issue 957 put the setting on the list.
const allowPrivateInstancesSetting = "ALLOW_PRIVATE_INSTANCES"

// allowPrivateAdvice is the one sentence that changes a tier B outcome, kept
// in one place so the dialer, the gate and the tool-error hint cannot come to
// spell the escape differently.
const allowPrivateAdvice = "pass --allow-private-instances (or set " + AllowPrivateInstancesEnv + "=true) " +
	"if that address is a GitLab or an object store on your own private network"

// DestinationRefusedHint is the next step to offer a model or an operator who
// has just been told a tier B destination was refused.
//
// It names the flag rather than describing the policy: the policy is already
// in the error, and what the reader does not have is the one string that
// changes it. A tier A refusal gets [MetadataRefusedHint] instead
// ([DestinationRefusedHintFor]), since no string changes that one.
const DestinationRefusedHint = allowPrivateAdvice

// metadataAddresses are the link-local and CGNAT addresses cloud providers
// answer instance credentials on.
//
// They are named one by one rather than derived from their enclosing ranges
// because tier A applies to every client and every hop, including the ordinary
// pinned deployment that tier B never touches, and a rule that broad must be
// as narrow as it can be. Nothing legitimate serves a GitLab API or a
// presigned object-storage URL from one of these, so the false-positive cost
// is as close to zero as a guard of this kind gets.
var metadataAddresses = map[netip.Addr]string{
	netip.MustParseAddr("169.254.169.254"): "the cloud instance metadata address",
	netip.MustParseAddr("169.254.170.2"):   "the AWS container credentials address",
	netip.MustParseAddr("fd00:ec2::254"):   "the AWS instance metadata address over IPv6",
	netip.MustParseAddr("100.100.100.200"): "the Alibaba Cloud instance metadata address",
}

// cgnatPrefix is RFC 6598 shared address space. It is not [netip.Addr.IsPrivate],
// which covers RFC 1918 and RFC 4193 only, and it is where a carrier-grade NAT
// deployment and several corporate VPNs put their internal hosts.
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// instanceLookupTimeout bounds the one DNS question this guard asks of its own
// accord: whether the configured instance itself sits on a private address.
//
// It is asked the first time a request leaves the host of an instance the
// operator named, since the answer decides which of the [destinationPools]
// that request is served from, and never again for the same policy. A
// deployment whose requests never leave the instance, one whose instance is
// spelled as an address, one that passed --allow-private-instances and a
// client whose instance a caller named never ask it at all.
const instanceLookupTimeout = 2 * time.Second

// addressLiteral reports whether host is spelled as an address rather than as
// a name, and what that address is.
//
// It is a helper rather than an inline parse because both callers act on the
// spelling and neither has anything to do with the parse error: a name is not
// a malformed address, it is the input this guard leaves to the resolver.
func addressLiteral(host string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr, true
}

// numericHostAddress reads a host that ends in a number the way the WHATWG URL
// standard's IPv4 parser does, which is the inet_aton grammar a libc resolver
// applies: one to four dot-separated parts, each decimal, 0x hexadecimal or
// 0-prefixed octal, every part but the last one byte and the last filling the
// bytes the others leave. 2852039166, 0xa9fea9fe, 0251.0376.0251.0376,
// 169.254.43518 and 169.254.169.254. are each 169.254.169.254 to such a
// resolver, and none of them is an address to [netip.ParseAddr].
//
// endsInNumber reports whether host's last label (a trailing empty label
// dropped) is a number, which is the condition for reading it at all; a host
// that ends in a number and does not parse is reported with an error, since
// the standard makes it an invalid host and a proxy may read it anyway. A host
// that does not end in a number is a name, and is returned as neither.
//
// It is asked only where this server cannot see what a host resolves to: the
// destination behind a proxy and the instance a caller names at the door. The
// dialer needs none of it, because it judges the address a name resolved to,
// whatever spelling produced it.
func numericHostAddress(host string) (addr netip.Addr, endsInNumber bool, err error) {
	labels := strings.Split(host, ".")
	if len(labels) > 1 && labels[len(labels)-1] == "" {
		labels = labels[:len(labels)-1]
	}
	last := labels[len(labels)-1]
	if _, isNumber := ipv4Number(last); !isNumber && !allDigits(last) {
		return netip.Addr{}, false, nil
	}
	if len(labels) > 4 {
		return netip.Addr{}, true, fmt.Errorf("%q has more than four parts", host)
	}
	var value uint64
	for i, label := range labels {
		n, isNumber := ipv4Number(label)
		if !isNumber {
			return netip.Addr{}, true, fmt.Errorf("%q has a part that is not a number", host)
		}
		if i == len(labels)-1 {
			if n >= 1<<(8*(5-len(labels))) {
				return netip.Addr{}, true, fmt.Errorf("%q has a last part too large for the bytes it fills", host)
			}
			value += n
			continue
		}
		if n > 255 {
			return netip.Addr{}, true, fmt.Errorf("%q has a part larger than one byte", host)
		}
		value += n << (8 * (3 - i))
	}
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}), true, nil
}

// ipv4Number parses one part of a numeric host as the WHATWG IPv4 number
// parser does: a 0x or 0X prefix is hexadecimal and may be followed by
// nothing, a leading 0 before more digits is octal, and anything else is
// decimal. A part too large for 64 bits is no number, which leaves the host
// refused rather than wrapped onto a smaller address.
func ipv4Number(part string) (uint64, bool) {
	if part == "" {
		return 0, false
	}
	digits, base := ipv4NumberBase(part)
	if digits == "" {
		return 0, true
	}
	n, err := strconv.ParseUint(digits, base, 64)
	return n, err == nil
}

// ipv4NumberBase splits the radix prefix off a part that is not empty: 0x or
// 0X for hexadecimal, a leading 0 for octal, none for decimal. A lone 0 reads
// as an octal prefix with no digits after it, which is zero either way.
//
// Separate ifs in a function of their own rather than a tagless switch: a case
// expression carries no statement counter, so the mutation gate reports every
// mutant of one as not covered whatever the tests assert.
func ipv4NumberBase(part string) (digits string, base int) {
	if len(part) > 1 && (part[:2] == "0x" || part[:2] == "0X") {
		return part[2:], 16
	}
	if part[0] == '0' {
		return part[1:], 8
	}
	return part, 10
}

// allDigits reports whether s is one or more ASCII digits, which ends a host
// in a number however large that number is.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// judgedSpelling is what a host this guard cannot resolve is spelled as: an
// address, read by [netip.ParseAddr] or by [numericHostAddress], or a name
// (spelled false). A host that ends in a number and is no address is
// refused, since the WHATWG standard makes it invalid and a proxy may still
// read it as an address.
func judgedSpelling(host string) (addr netip.Addr, spelled bool, err error) {
	if literal, ok := addressLiteral(host); ok {
		return literal, true, nil
	}
	numeric, endsInNumber, parseErr := numericHostAddress(host)
	if parseErr != nil {
		return netip.Addr{}, true, fmt.Errorf("%w: %q ends in a number but is not an IPv4 address: %w", ErrDestinationRefused, host, parseErr)
	}
	return numeric, endsInNumber, nil
}

// dialHostname is the host net/http hands the dialer for host, and asks a
// SOCKS proxy or an HTTP CONNECT for: host itself when it is ASCII, and
// otherwise its IDNA form under the Lookup profile, or host as written when
// that profile refuses it. It is the rule of net/http's own idnaASCII, which
// its canonicalAddr applies to every request URL.
//
// It matters wherever this package asks what an address is spelled as,
// because the IDNA mapping turns spellings that are not addresses into ones
// that are: fullwidth digits and full stops, ideographic full stops and
// circled digits all map to ASCII, so 169.254.169.254 written in fullwidth
// digits is dialed as 169.254.169.254 and handed to a SOCKS proxy as an IPv4
// address. The dialer sees that address and judges it; a request sent through
// a proxy is never dialed, and was judged as a name.
//
// The mapping is golang.org/x/net/idna rather than the copy the standard
// library vendors, which cannot be imported. The two can drift apart when
// their Unicode tables do; TestDialHostname_IsWhatNetHTTPDials holds this to
// what net/http actually dials for every spelling it lists.
//
// A byte outside ASCII is one with its high bit set, asked as that bit rather
// than as a comparison with 0x80: the two agree on every byte, and the bit
// leaves no boundary to misplace.
func dialHostname(host string) string {
	for i := range len(host) {
		if host[i]&0x80 != 0 {
			mapped, err := idna.Lookup.ToASCII(host)
			if err != nil {
				return host
			}
			return mapped
		}
	}
	return host
}

// metadataAddress reports whether addr is a cloud metadata endpoint, and what
// to call it in the refusal.
//
// The zone is dropped as well as the mapping, for the same reason: the table
// holds each address once, bare, and fd00:ec2::254%eth0 is that address with
// an interface named. An operating system ignores the zone of a destination
// that is not link-local, so a lookup that kept it would let the spelling
// reach the endpoint the table exists to refuse.
//
// An address behind a NAT64 prefix is judged by the IPv4 address it embeds as
// well ([nat64Embedded]), since a translator on the path reaches exactly that
// address: 64:ff9b::a9fe:a9fe is 169.254.169.254 wherever NAT64 is deployed.
func metadataAddress(addr netip.Addr) (string, bool) {
	addr = addr.WithZone("").Unmap()
	if name, ok := metadataAddresses[addr]; ok {
		return name, true
	}
	for _, embedded := range nat64Embedded(addr) {
		if name, ok := metadataAddresses[embedded]; ok {
			return name + " behind a NAT64 prefix", true
		}
	}
	return "", false
}

// isPrivateAddress reports whether addr is one of the classes a destination
// this server did not choose may not reach.
//
// The unmapping is not a detail: ::ffff:10.0.0.1 is 10.0.0.1 written as IPv6,
// and every predicate below answers false for that spelling. A guard that
// skipped the unmapping would be bypassed by writing the address differently.
// The zone is dropped for the same reason, since a prefix contains no address
// that carries one.
//
// Three IPv6 forms name an IPv4 address rather than a host of their own. An
// address behind a NAT64 prefix is private when any IPv4 address it can stand
// for is ([nat64Embedded]), since which prefix length a deployment chose is
// not written in the address and a refusal must hold whichever it was. The
// 6to4 prefix 2002::/16 (RFC 7526) and the deprecated IPv4-compatible prefix
// ::/96 (RFC 4291 section 2.5.5.1) name a tunnel endpoint, and are private
// whatever they embed: nothing a GitLab redirect has reason to reach is served
// from one, and judging the endpoint by the address it carries would be a rule
// about tunnels this guard has no business guessing at.
//
// Both rules decide what a destination may be refused, and neither is a fact
// about the address, so the exemption an instance that is itself private earns
// asks [isPrivateInstanceAddress] instead.
func isPrivateAddress(addr netip.Addr) bool {
	addr = addr.WithZone("").Unmap()
	if !addr.IsValid() {
		// An address that cannot be classified is not one to connect to.
		return true
	}
	if sixToFourPrefix.Contains(addr) || ipv4CompatiblePrefix.Contains(addr) {
		return true
	}
	return inPrivateClass(addr) || slices.ContainsFunc(nat64Embedded(addr), inPrivateClass)
}

// isPrivateInstanceAddress reports whether the operator's instance, reached at
// addr, is private in fact, which is what lets a redirect away from it reach a
// private address ([destinationPolicy.resolveInstancePrivate]).
//
// It answers the opposite question from [isPrivateAddress], and so reads the
// two cautious rules the other way. A NAT64 address counts only when every
// IPv4 address it can stand for is private, and the tunnel prefixes never
// count: a public instance called private here would let its redirects reach
// the private network, so a wrong answer is a weakening, where a wrong answer
// there refuses one hop.
func isPrivateInstanceAddress(addr netip.Addr) bool {
	addr = addr.WithZone("").Unmap()
	if inPrivateClass(addr) {
		return true
	}
	embedded := nat64Embedded(addr)
	return len(embedded) > 0 && !slices.ContainsFunc(embedded, isPublicClass)
}

// inPrivateClass reports whether addr, already unmapped and without a zone,
// is in one of the private classes by itself: loopback, unspecified, RFC 1918
// or unique-local, link-local unicast or multicast, or CGNAT.
func inPrivateClass(addr netip.Addr) bool {
	switch {
	case addr.IsLoopback(), addr.IsUnspecified(), addr.IsPrivate(),
		addr.IsLinkLocalUnicast(), addr.IsLinkLocalMulticast():
		return true
	}
	return cgnatPrefix.Contains(addr)
}

// isPublicClass is the negation of [inPrivateClass], for asking whether every
// address of a list is private.
func isPublicClass(addr netip.Addr) bool {
	return !inPrivateClass(addr)
}

// The IPv6 prefixes that carry an IPv4 address.
var (
	// nat64WellKnownPrefix is the RFC 6052 well-known prefix, which embeds the
	// IPv4 address in its last 32 bits.
	nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")
	// nat64LocalUsePrefix is the RFC 8215 local-use prefix, inside which an
	// operator may use any RFC 6052 prefix length from /48 on, so the IPv4
	// address can sit in any of [nat64Layouts].
	nat64LocalUsePrefix = netip.MustParsePrefix("64:ff9b:1::/48")
	// sixToFourPrefix is 6to4 (RFC 7526).
	sixToFourPrefix = netip.MustParsePrefix("2002::/16")
	// ipv4CompatiblePrefix is the IPv4-compatible form RFC 4291 deprecated.
	ipv4CompatiblePrefix = netip.MustParsePrefix("::/96")
)

// nat64Layout is where RFC 6052 section 2.2 puts the four IPv4 bytes for one
// prefix length, and the byte the all-zero suffix after them starts at.
type nat64Layout struct {
	octets     [4]int
	suffixFrom int
}

// nat64Layouts are the RFC 6052 layouts whose prefix fits inside
// [nat64LocalUsePrefix]: /48, /56, /64 and /96. Byte 8 is reserved and zero in
// every one of them, which is what skips the IPv4 bytes the /48 and /56
// layouts split around it.
var nat64Layouts = []nat64Layout{
	{octets: [4]int{6, 7, 9, 10}, suffixFrom: 11},
	{octets: [4]int{7, 9, 10, 11}, suffixFrom: 12},
	{octets: [4]int{9, 10, 11, 12}, suffixFrom: 13},
	{octets: [4]int{12, 13, 14, 15}, suffixFrom: 16},
}

// nat64Embedded returns the IPv4 addresses addr can stand for behind a NAT64
// translator: the one in its last 32 bits under the well-known prefix, and
// under the local-use prefix the one of each layout addr is a well-formed
// instance of, its reserved byte and its suffix zero as RFC 6052 requires. An
// address that is no NAT64 address, or no well-formed one, embeds nothing.
//
// Every candidate is returned rather than one guessed at, because which prefix
// length a local-use deployment chose is not written in the address; a
// well-formed /64 address whose /96 reading is also well formed stands for
// both.
//
// A local-use reading of 0.0.0.0 is no candidate. A shorter layout leaves
// zeros where a longer one reads its IPv4 bytes, so the /96 reading of every
// /48 and /56 address is 0.0.0.0, which is that layout's suffix rather than an
// address: no translator forwards to 0.0.0.0 (RFC 6890 makes it no
// destination), and counting it would make every host behind those layouts
// private. The other readings of such an address stay, since each is a host
// some deployment reaches.
func nat64Embedded(addr netip.Addr) []netip.Addr {
	b := addr.As16()
	if nat64WellKnownPrefix.Contains(addr) {
		return []netip.Addr{netip.AddrFrom4([4]byte(b[12:16]))}
	}
	if !nat64LocalUsePrefix.Contains(addr) || b[8] != 0 {
		return nil
	}
	var embedded []netip.Addr
	for _, layout := range nat64Layouts {
		o := layout.octets
		v4 := [4]byte{b[o[0]], b[o[1]], b[o[2]], b[o[3]]}
		if !allZeroBytes(b[layout.suffixFrom:]) || v4 == [4]byte{} {
			continue
		}
		embedded = append(embedded, netip.AddrFrom4(v4))
	}
	return embedded
}

// allZeroBytes reports whether every byte of b is zero.
func allZeroBytes(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// allowPrivateInstances reads the tier B opt-out.
//
// A value that does not parse as a boolean is false rather than an error: this
// is a guard, and a typo in the variable that turns it off would be the one
// failure mode nobody notices. The flag is what an operator normally types,
// and a flag value that does not parse is theirs to see in the same startup.
func allowPrivateInstances() bool {
	value := config.TrimmedGetenv(allowPrivateInstancesSetting)
	if value == "" {
		return false
	}
	allowed, err := strconv.ParseBool(value)
	return err == nil && allowed
}

// destinationPolicy is what one client is allowed to dial.
//
// # The two tiers, and why there is never a third
//
// Tier A refuses the cloud metadata addresses on every hop for every client
// and is applied in [dialTarget.judge], above any policy, so a request that
// carries none is still covered.
//
// Tier B refuses the private address classes, and only for a destination this
// server's operator did not choose: an instance a caller named in the
// GITLAB-URL header under --allow-any-gitlab-url, or a redirect hop that left
// the instance's own host. An address reached because --gitlab-url or
// GITLAB_URL named it is exempt whatever it resolves to, and that exemption is
// the reason every self-hosted deployment keeps working: GitLab on localhost,
// on 10.x, on 192.168.x or behind a VPN on CGNAT is the ordinary case, not the
// attack. DNS rebinding is only a threat when the attacker controls the name,
// and here the operator chose it. See ADR-0022 before narrowing this.
type destinationPolicy struct {
	// instanceHost is the host of the instance this client talks to, as
	// [FoldHostCase] leaves it. Empty when the base URL carried none, which
	// makes every destination off-origin: without a host to compare against
	// there is no hop that can be shown to be the instance's own.
	instanceHost string

	// callerChosen records that the instance itself was named by a caller
	// rather than by the operator, which is the --allow-any-gitlab-url hatch.
	// It is what puts the very first hop under tier B.
	callerChosen bool

	// allowPrivate is the tier B opt-out, read once when the client is built.
	allowPrivate bool

	// lookupIP resolves the instance host for [destinationPolicy.instanceIsPrivate].
	// It is a field rather than a package variable so two policies in one
	// process never share a test's stub.
	lookupIP func(ctx context.Context, host string) ([]netip.Addr, error)

	privateOnce   sync.Once
	privateResult bool
}

// newDestinationPolicy builds the policy for a client talking to baseURL.
func newDestinationPolicy(baseURL string, callerChosen, allowPrivate bool) *destinationPolicy {
	host, _ := credentialScope(baseURL)
	return &destinationPolicy{
		instanceHost: host,
		callerChosen: callerChosen,
		allowPrivate: allowPrivate,
		lookupIP:     lookupHostAddrs,
	}
}

// lookupHostAddrs is the real resolver, kept apart so a policy built for a
// test can replace it without touching a package variable.
func lookupHostAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// coversInstance reports whether dest is the instance's own host: the host the
// operator named, exactly, with ASCII case folded as [FoldHostCase] folds it.
//
// A subdomain is not the instance. It is another name with addresses of its
// own, and the exemption tier B grants the instance rests on the operator
// having chosen that name: nobody chose x.gitlab.example.com, and under a
// split-horizon domain it can resolve to loopback or to the server's own
// network behind an instance that is public. A hop to one is therefore judged
// like any other host that left the instance, which still allows it when the
// instance is itself private (the self-managed object store) or under
// --allow-private-instances, at the cost of the one memoized lookup of the
// instance's name that every such hop pays.
//
// This is deliberately narrower than [credentialSafeRedirect], which lets the
// credential headers follow a subdomain the way net/http lets an
// Authorization header follow one. That relation answers who may read the
// token; this one answers which addresses a hop may reach, and the two are
// different questions. The scheme is not part of it, because an
// https-to-http downgrade does not change which address gets dialed.
func (p *destinationPolicy) coversInstance(dest *url.URL) bool {
	if p == nil || p.instanceHost == "" || dest == nil {
		return false
	}
	return FoldHostCase(dest.Hostname()) == p.instanceHost
}

// permitsPrivate reports whether tier B lets one request reach a private
// address at all.
//
// It is the one predicate both halves of the guard read:
// [destinationPolicy.checkPrivate] when a connection is dialed, and
// [destinationTransport.RoundTrip] when it chooses the pool a request is
// served from. They must never disagree, because on a reused connection the
// pool is all there is: nothing is dialed, so the connection a request is
// handed carries only the answer it was dialed under.
// Every input is fixed for the life of the policy, the instance's own privacy
// included once it has been resolved, so the routing and the dial of one
// request get the same answer from the same policy.
func (p *destinationPolicy) permitsPrivate(ctx context.Context, offOrigin bool) bool {
	if !offOrigin && !p.callerChosen {
		// The operator named this instance. Nothing here is checked, ever.
		return true
	}
	if p.allowPrivate {
		return true
	}
	// A self-managed GitLab redirecting an artifact download to object
	// storage on the same private network is ordinary, and a deployment whose
	// operator named an instance that is already private is inside that
	// network. Tier A still applies at the dial, so this cannot reach a
	// metadata address.
	//
	// Never for an instance a caller named. Its first hop is refused a private
	// address, so a caller-named instance this client reached answered the
	// dialer with a public one, and the only way the lookup here could then
	// call it private is a name answering differently the second time it is
	// asked: DNS rebinding, by whoever holds the name. The operator's own name
	// is exempt from that concern because the operator chose it; a caller's
	// is not, and granting this would turn --allow-any-gitlab-url into a
	// redirect onto the private network. A caller-named instance that really
	// is private is served through --allow-private-instances, answered above.
	return offOrigin && !p.callerChosen && p.instanceIsPrivate(ctx)
}

// checkPrivate applies tier B to one candidate address.
func (p *destinationPolicy) checkPrivate(ctx context.Context, addr netip.Addr, offOrigin bool) error {
	if !isPrivateAddress(addr) || p.permitsPrivate(ctx, offOrigin) {
		return nil
	}
	return p.refusal(addr, offOrigin)
}

// refusal builds the error for a tier B destination, saying which of the two
// ways of not being operator-chosen this one was.
func (p *destinationPolicy) refusal(addr netip.Addr, offOrigin bool) error {
	if offOrigin {
		instance := p.instanceHost
		if instance == "" {
			instance = "the configured instance"
		}
		return fmt.Errorf("%w: a redirect away from %s reached the private address %s", ErrDestinationRefused, instance, addr)
	}
	return fmt.Errorf("%w: the GITLAB-URL header named an instance on the private address %s", ErrDestinationRefused, addr)
}

// instanceIsPrivate answers, once, whether the configured instance itself sits
// on a private address.
//
// A literal host is decided without asking anybody. A name is resolved the
// first time a request leaves the instance, and a resolver that cannot answer
// leaves the destination refused rather than permitted.
//
// The lookup does not inherit the caller's cancellation, because its answer
// is kept for every later request of the client and describes the instance
// rather than the request that happened to ask first. net/http hands a
// redirect hop to the transport without checking whether its context has
// ended, so a hop routed after its caller gave up would otherwise record "not
// private" for the life of the client, and every later redirect to the
// instance's own object store would be refused for it. [instanceLookupTimeout]
// still bounds the wait.
func (p *destinationPolicy) instanceIsPrivate(ctx context.Context) bool {
	p.privateOnce.Do(func() {
		p.privateResult = p.resolveInstancePrivate(context.WithoutCancel(ctx))
	})
	return p.privateResult
}

// resolveInstancePrivate is the body of [destinationPolicy.instanceIsPrivate],
// separated so the memoization and the decision can be read apart.
//
// The question is asked of the host the dialer will connect to, its
// [dialHostname], and not of the configured spelling: an instance written as
// "gİtlab.corp.example" is dialed as xn--gitlab-qyd.corp.example, and the
// answer for any other name describes some other host.
func (p *destinationPolicy) resolveInstancePrivate(ctx context.Context) bool {
	if p.instanceHost == "" {
		return false
	}
	dialed := dialHostname(p.instanceHost)
	if addr, spelledAsAddress := addressLiteral(dialed); spelledAsAddress {
		return isPrivateInstanceAddress(addr)
	}
	if p.lookupIP == nil {
		return false
	}
	lookupCtx, cancel := context.WithTimeout(ctx, instanceLookupTimeout)
	defer cancel()
	addrs, err := p.lookupIP(lookupCtx, dialed)
	if err != nil || len(addrs) == 0 {
		return false
	}
	// Every address, not the first: a name that answers with one private and
	// one public address is not a private deployment, and taking the first
	// would make the answer depend on resolver ordering.
	for _, addr := range addrs {
		if !isPrivateInstanceAddress(addr) {
			return false
		}
	}
	return true
}

// dialTarget is what one request tells the dialer.
//
// It travels on the request context rather than on the transport because the
// transports are shared: every client that verifies certificates is routed
// between the same two, [sharedDestinationPools] (a client that skips
// verification has a pair of its own), and a transport per client would give
// each of up to --max-http-clients pool entries its own idle-connection set,
// which is the cost sharing them exists to avoid.
type dialTarget struct {
	policy *destinationPolicy
	// offOrigin reports that this request's URL is not the instance's own
	// host. For a client following a redirect that is exactly "the hop left
	// the instance", because net/http hands each hop to the transport as a
	// request of its own.
	offOrigin bool
	// proxy is the address the transport will dial for this request when it
	// sends it through a proxy rather than to its own host, spelled as
	// net/http spells the address it hands the dialer; empty when the request
	// is dialed directly. [guardedDial] reads it, because only the dialer
	// knows which address it was asked for before resolution.
	proxy string
}

// judge applies both tiers to one address this request would reach.
//
// Tier A comes first and needs nothing from the stamp, so a request that
// carries no policy is still refused a metadata address. Two dials carry
// none, and tier A is all either gets: a dial to the operator's proxy, which
// [guardedDial] re-stamps so, and one by a caller outside this server that
// borrows the transport through [HTTPTransport], which chose its own
// destination and is not the case this guard is about.
func (target dialTarget) judge(ctx context.Context, addr netip.Addr) error {
	if name, ok := metadataAddress(addr); ok {
		return fmt.Errorf("%w: %s is %s, which never serves a GitLab API or object storage", ErrMetadataDestinationRefused, addr, name)
	}
	if target.policy == nil {
		return nil
	}
	return target.policy.checkPrivate(ctx, addr, target.offOrigin)
}

// operatorNamed reports whether this request is to the instance the operator
// named: on that instance's own host, of an instance no caller named. Nothing
// tier B decides applies to it ([destinationPolicy.permitsPrivate]), and a
// request that carries no policy is no such request, since nothing says whose
// destination it is.
func (target dialTarget) operatorNamed() bool {
	return target.policy != nil && !target.offOrigin && !target.policy.callerChosen
}

// dialTargetKey is the private context key for [dialTarget].
type dialTargetKey struct{}

// withDialTarget stamps the destination decision for one request.
func withDialTarget(ctx context.Context, target dialTarget) context.Context {
	return context.WithValue(ctx, dialTargetKey{}, target)
}

// dialTargetFrom reads back what [withDialTarget] stamped.
func dialTargetFrom(ctx context.Context) (dialTarget, bool) {
	target, ok := ctx.Value(dialTargetKey{}).(dialTarget)
	return target, ok
}

// guardDestination is the [net.Dialer.ControlContext] hook every client dials
// through.
//
// # Why here and nowhere else
//
// It runs after resolution and once per candidate address, so a name that
// resolves to something else than it claims is judged by what it resolved to,
// and a name that answers with several addresses is judged on each. That is
// what makes DNS rebinding uninteresting: there is no window between the check
// and the connection, because this IS the connection.
//
// It is also the one place that covers the first request and every redirect
// hop with the same code, since every transport this package builds dials
// through a dialer carrying it ([baseDialer]).
//
// # What it cannot see, and what covers that
//
// A request served from an idle connection is not dialed, so this never runs
// for it. Go keys idle connections on scheme, host and port and knows nothing
// of the policy a connection was opened under: on one shared transport, a
// request this would refuse was handed, without a dial, the connection a
// request it permits had opened to the same host. [destinationTransport] closes
// that by routing each request to one of the [destinationPools] by the same
// predicate this applies, so a request refused a private address is only
// served connections that were dialed under that refusal, and every one of
// those leads to a public address or to the operator's proxy.
//
// Nor does it see the destination of a request sent through a proxy, since
// net/http then dials the proxy and hands this the proxy's address. That dial
// reaches it re-stamped by [guardedDial] with no policy, so the proxy gets
// tier A and nothing else, and the destination behind it is judged, as far as
// anything here can judge it, by [judgeProxiedDestination].
func guardDestination(ctx context.Context, network, address string, _ syscall.RawConn) error {
	if !strings.HasPrefix(network, "tcp") {
		// Everything this server dials is TCP. A unix socket carries a path
		// rather than an address and has no destination to classify.
		return nil
	}
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %q is not an address this server can classify", ErrDestinationRefused, address)
	}
	target, _ := dialTargetFrom(ctx)
	return target.judge(ctx, addrPort.Addr())
}

// guardedDial is the DialContext of every transport this package builds: d,
// told whether the dial is to the proxy the request is sent through.
//
// # Why the proxy gets tier A and nothing else
//
// When HTTP_PROXY or HTTPS_PROXY applies to a request, net/http dials the
// proxy's host and port in place of the destination's, and [guardDestination]
// is handed the address that resolved to. Judged as the destination, a proxy
// on a private address refused every request tier B refuses a private
// address, whatever that request was going to reach: an artifact download
// redirected from a public instance to its object store, behind an ordinary
// corporate proxy, failed on the proxy's address. The proxy is the operator's
// own configuration, as much as --gitlab-url is, so it is not a destination a
// caller or a redirect chose, and tier B has nothing to say about it. Tier A
// still has: no proxy is served from a cloud metadata address either.
//
// That holds for an instance a caller named as well. The caller chose the
// destination, not the proxy, and what the proxy is asked to reach is judged
// apart from the dial, by [judgeProxiedDestination].
//
// # Why here
//
// The guard sees only what an address resolved to, which cannot say it was a
// proxy; the address before resolution can, and this is the one place it is
// visible. It is compared with the address [destinationTransport.RoundTrip]
// recorded for the request's proxy, as a string, so a dial that is not that
// address keeps the request's stamp and is judged as a destination. A
// disagreement between the two spellings therefore falls back to judging the
// proxy as the destination, which is the rule every proxy dial got before
// this wrapper existed: it can refuse more than a match does, since a private
// proxy is then refused wherever tier B refuses the request a private
// address, but it never permits more, since tier A applies either way. An
// unstamped dial, or one whose request is not proxied, has no proxy address,
// and net/http never dials an empty one.
func guardedDial(d *net.Dialer) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if target, _ := dialTargetFrom(ctx); target.proxy == address {
			ctx = withDialTarget(ctx, dialTarget{})
		}
		return d.DialContext(ctx, network, address)
	}
}

// destinationPools are the two transports every request this package makes is
// routed between, split by what tier B answers for a private address.
//
// The split is what keeps connection reuse from walking past
// [guardDestination]. A reused connection is not dialed, so the guard never
// sees the request it serves, and the only judgement such a connection carries
// is the one made when it was opened. So a request is only handed connections
// opened under the same answer as its own:
//
//   - strict carries the requests tier B refuses a private address: an
//     instance a caller named, a redirect hop that left it, and a redirect hop
//     that left a public instance the operator named. Every connection in it
//     was dialed under that refusal, so it leads to a public address, which
//     every one of those requests may reach, or to a proxy (below).
//   - permissive carries the requests tier B allows one: the operator's own
//     instance, a deployment that passed --allow-private-instances, and a hop
//     from an instance the operator named that is itself private. A
//     connection in it may lead to a private address, and every request
//     routed to it would have been allowed to dial that address.
//
// Tier A needs no pool of its own, because no request of any policy can open a
// connection to a metadata address in the first place.
//
// A connection to a proxy is the one kind either pool holds whatever the
// proxy's address, since [guardedDial] gives a proxy dial tier A alone. That
// does not let a request past the split: net/http keys a proxied connection
// on the proxy it goes through, so it is only ever handed a request the same
// transport sends through the same proxy, whose own dial would have been
// judged the same way. What such a request asks the proxy to reach is judged
// per request, by [judgeProxiedDestination], and never by the connection.
//
// The ordinary deployment pays nothing for it in idle connections. Every
// request to the instance the operator named is permissive, so first-party
// traffic shares one idle pool exactly as it did when there was only one; the
// strict pool holds connections only for a caller-named instance and the hops
// away from it, and for redirect hops away from a public one. A client whose
// requests leave the instance the operator named does pay one memoized lookup
// of that instance's name, to know which pool the first such hop belongs in:
// see [instanceLookupTimeout].
type destinationPools struct {
	permissive http.RoundTripper
	strict     http.RoundTripper
}

// destinationTransport stamps every request with the policy in force for the
// client that made it, and routes it to the pool that policy's answer belongs
// in.
//
// It sits at the bottom of the chain, immediately around the base transports,
// so the stamp and the routing are structurally the last things that happen
// before the dial rather than something a later wrapper could be inserted in
// front of.
type destinationTransport struct {
	pools  destinationPools
	client *Client
}

// RoundTrip stamps the request and delegates to the pool its answer selects.
//
// A request the selected pool will send through a proxy is stamped with the
// proxy's address as well, so the dialer can tell that dial from the
// destination's, and its destination is judged here before anything is sent,
// because the dialer will never see it.
func (t *destinationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	policy := t.client.destinationPolicy()
	if policy == nil {
		// Tier A alone allows a private address, so an unstamped request
		// belongs with the permissive pool: nothing there was dialed under an
		// answer stricter than the one its own dial would get.
		return t.pools.permissive.RoundTrip(req)
	}
	target := dialTarget{policy: policy, offOrigin: !policy.coversInstance(req.URL)}
	pool := t.pools.strict
	if policy.permitsPrivate(req.Context(), target.offOrigin) {
		pool = t.pools.permissive
	}
	proxy, err := proxyDialAddress(pool, req)
	if err != nil {
		return nil, refuseUnsent(req, err)
	}
	if proxy != "" {
		target.proxy = proxy
		if err = judgeProxiedDestination(req.Context(), target, req.URL); err != nil {
			return nil, refuseUnsent(req, err)
		}
	}
	resp, err := pool.RoundTrip(req.WithContext(withDialTarget(req.Context(), target)))
	// No err != nil beside it: errors.Is is false for nil, and every other
	// dial error goes back as it came, *net.OpError included, so client-go
	// keeps retrying what is worth retrying.
	if errors.Is(err, ErrDestinationRefused) {
		return nil, refusalWithoutDialError(err)
	}
	return resp, err
}

// refusalWithoutDialError returns the guard's refusal out of the dial error
// net wrapped it in.
//
// The dialer reports a ControlContext refusal as a *net.OpError with Op
// "dial", and client-go's retry policy retries every dial error, on the
// reasoning that a dial which failed sent nothing. That is true here and
// beside the point: a refusal is final, so the instance was asked the same
// request again for every retry and the caller waited out the backoff to learn
// what had been decided before the first packet. Without the OpError the
// error reaches client-go as a *url.Error whose text says nothing about a
// refused connection, which its policy does not retry. The refusal keeps its
// own text and still wraps [ErrDestinationRefused].
//
// The loop declares its target on each pass and leaves on the negated
// condition, so no change to one operator in it can keep it spinning on an
// error it already unwrapped: the mutation gate cannot evaluate a mutant that
// never ends.
func refusalWithoutDialError(err error) error {
	for {
		var opErr *net.OpError
		if !errors.As(err, &opErr) || opErr.Err == nil {
			return err
		}
		err = opErr.Err
	}
}

// proxyDialAddress reports the address the transport serving req will dial
// when it sends req through a proxy, and "" when it will dial req's own host.
//
// It asks the transport's own Proxy function, which the transport will ask
// again for the same request, rather than reading the environment itself, so
// for a deterministic function the two come to one answer. The function every
// transport here carries is deterministic: [http.ProxyFromEnvironment] reads
// the environment once per process. One that is not is covered only as far
// as it fails: a function that fails here refuses the request unsent, while
// one that answers "direct" here and names a proxy when the transport asks
// again sends the request through a proxy nobody stamped, to a destination
// nothing judged. A pool that is not an [http.Transport] has no proxy this
// package can see, and none is assumed.
//
// The address is spelled as net/http spells the one it hands the dialer: the
// proxy's host, and its port or the default of its scheme. [guardedDial]
// compares the two as strings, so a host net/http would spell differently (a
// name it converts to its IDNA form, which this does not) matches no dial,
// and that dial is judged as a destination under the request's own stamp,
// which is the rule before the stamp existed. That can refuse a proxy a match
// would have let through; it cannot let through one a match would refuse.
func proxyDialAddress(pool http.RoundTripper, req *http.Request) (string, error) {
	transport, ok := pool.(*http.Transport)
	if !ok || transport.Proxy == nil {
		return "", nil
	}
	proxy, err := transport.Proxy(req)
	if err != nil || proxy == nil {
		return "", err
	}
	port := proxy.Port()
	if port == "" {
		port = proxySchemePorts[proxy.Scheme]
	}
	return net.JoinHostPort(proxy.Hostname(), port), nil
}

// proxySchemePorts are the ports net/http dials a proxy on when its URL names
// none, one per proxy scheme it supports.
var proxySchemePorts = map[string]string{
	"http":    "80",
	"https":   "443",
	"socks5":  "1080",
	"socks5h": "1080",
}

// judgeProxiedDestination applies the guard to the destination of a request
// sent through a proxy, which the dialer never sees: it dials the proxy.
//
// Only a destination spelled as an address can be judged here, and it is
// judged by exactly the rule the dialer would have applied to it, so a
// redirect to 169.254.169.254 is refused behind a proxy as it is without one,
// and so is a private address tier B refuses this request. A destination
// spelled as a name is resolved by the proxy and not by this server, so what
// it reaches is the proxy's decision and nothing here can see it. That is a
// limit rather than a boundary: any name that resolves to an address reaches
// that address through the proxy, and a deployment that proxies its outbound
// traffic has made the proxy the place a rule about names belongs. It is the
// same line [CheckCallerNamedInstance] draws at the door, for the same reason.
//
// It runs per request rather than per dial because behind any proxy the
// dialer only ever sees the proxy's address, and because net/http keys a
// plain-HTTP request sent through an http or https proxy on the proxy alone
// (connectMethod.key in its transport.go), so one such connection carries
// requests to many destinations.
//
// "Spelled as an address" is asked of the host's [dialHostname], since that
// is what net/http hands a SOCKS proxy and names in a CONNECT: a host written
// in fullwidth digits reached 169.254.169.254 through either while it was
// judged here as a name. A plain-HTTP request through an HTTP proxy names its
// host in the request line under another IDNA profile, which leaves such a
// spelling a name, so judging the mapped form there refuses more than the
// proxy would have reached and never less.
//
// "Spelled as an address" includes a host that ends in a number
// ([numericHostAddress]): a libc proxy reads 2852039166 or 169.254.43518 as
// 169.254.169.254 with inet_aton, so they are judged as that address, and one
// that ends in a number without being an address is refused, except as the
// instance the operator named. That spelling is no address, so nothing tier A
// knows can be in it, and an address --gitlab-url or GITLAB_URL named is
// never checked: such an instance is a name, left to the proxy to resolve as
// it was before numeric spellings were read at all.
func judgeProxiedDestination(ctx context.Context, target dialTarget, dest *url.URL) error {
	addr, spelledAsAddress, err := judgedSpelling(dialHostname(dest.Hostname()))
	if err != nil && target.operatorNamed() {
		return nil
	}
	if err != nil || !spelledAsAddress {
		return err
	}
	return target.judge(ctx, addr)
}

// refuseUnsent closes the body of a request this transport declines to send
// and returns err, since a RoundTripper owns the body it is handed, refusals
// included.
func refuseUnsent(req *http.Request, err error) error {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return err
}

// destinationPolicy returns the policy this client dials under, or nil.
func (c *Client) destinationPolicy() *destinationPolicy {
	if c == nil {
		return nil
	}
	return c.destination.Load()
}

// MarkInstanceCallerNamed records that this client's instance was named by a
// caller rather than by the operator, which is the --allow-any-gitlab-url
// hatch, and puts its very first hop under tier B.
//
// It is called by the server pool, which is the only place that knows: a
// client is built from a URL string, and the string looks identical whether
// --gitlab-url published it or a GITLAB-URL header did.
//
// The default is the other way round because the alternative breaks every
// ordinary deployment: a client whose instance nobody marked is one the
// operator configured, and refusing those would refuse GitLab on localhost.
func (c *Client) MarkInstanceCallerNamed() {
	if c == nil {
		return
	}
	current := c.destinationPolicy()
	if current == nil {
		return
	}
	c.destination.Store(newDestinationPolicy(c.baseURL, true, current.allowPrivate))
}

// CheckCallerNamedInstance refuses an instance URL a caller named that this
// server would decline to dial anyway.
//
// It exists to answer at the door instead of deep inside a tool call. The
// dialer is still the authority — this consults the same predicate and the
// same opt-out — and this is the friendlier half: an operator who pointed the
// header at their own private GitLab gets one 400 naming the flag, rather than
// a server that starts, admits the credential and fails every action.
//
// It resolves nothing. A host spelled as a literal address is judged here; a
// host spelled as a name is left entirely to the dialer, which sees what the
// name resolved to and is the only check that can. Asking DNS a question per
// request at the door would put a resolver timeout in front of every caller
// and would still not be authoritative. A spelling is read as net/http will
// dial it ([dialHostname]), so an address written in fullwidth digits is the
// address it reaches rather than a name, and a host that ends in a number is
// read as the address a libc proxy would make of it ([numericHostAddress]).
func CheckCallerNamedInstance(rawURL string) error {
	host, _ := credentialScope(rawURL)
	addr, spelledAsAddress, err := judgedSpelling(dialHostname(host))
	if err != nil || !spelledAsAddress {
		return err
	}
	if name, ok := metadataAddress(addr); ok {
		return fmt.Errorf("%w: %s is %s, which never serves a GitLab API", ErrMetadataDestinationRefused, addr, name)
	}
	if !isPrivateAddress(addr) || allowPrivateInstances() {
		return nil
	}
	return fmt.Errorf("%w: the GITLAB-URL header named an instance on the private address %s; %s",
		ErrDestinationRefused, addr, allowPrivateAdvice)
}
