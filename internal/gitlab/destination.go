package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// ErrDestinationRefused reports that this server declined to open a connection
// to an address, before any packet was sent to it.
//
// It is a refusal by this server rather than an answer from anything upstream,
// which is the distinction a reader of the message needs: nothing was reached,
// nothing was disclosed, and the fix is configuration rather than credentials.
var ErrDestinationRefused = errors.New("outbound destination refused")

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
const AllowPrivateInstancesEnv = config.EnvPrefix + "ALLOW_PRIVATE_INSTANCES"

// allowPrivateAdvice is the one sentence that changes a tier B outcome, kept
// in one place so the dialer, the gate and the tool-error hint cannot come to
// spell the escape differently.
const allowPrivateAdvice = "pass --allow-private-instances (or set " + AllowPrivateInstancesEnv + "=true) " +
	"if that address is a GitLab or an object store on your own private network"

// DestinationRefusedHint is the next step to offer a model or an operator who
// has just been told a destination was refused.
//
// It names the flag rather than describing the policy: the policy is already
// in the error, and what the reader does not have is the one string that
// changes it. Tier A has no such string, which the sentence says out loud
// rather than leaving a reader to try the flag and find it did nothing.
const DestinationRefusedHint = allowPrivateAdvice + ". Cloud metadata addresses stay refused whatever it is set to"

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
// The lookup happens on the refusal path only, so a slow resolver delays a
// request that was about to fail rather than every request that succeeds.
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

// metadataAddress reports whether addr is a cloud metadata endpoint, and what
// to call it in the refusal.
func metadataAddress(addr netip.Addr) (string, bool) {
	name, ok := metadataAddresses[addr.Unmap()]
	return name, ok
}

// isPrivateAddress reports whether addr is one of the classes a destination
// this server did not choose may not reach.
//
// The unmapping is not a detail: ::ffff:10.0.0.1 is 10.0.0.1 written as IPv6,
// and every predicate below answers false for that spelling. A guard that
// skipped the unmapping would be bypassed by writing the address differently.
func isPrivateAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		// An address that cannot be classified is not one to connect to.
		return true
	}
	switch {
	case addr.IsLoopback(), addr.IsUnspecified(), addr.IsPrivate(),
		addr.IsLinkLocalUnicast(), addr.IsLinkLocalMulticast():
		return true
	default:
		return cgnatPrefix.Contains(addr)
	}
}

// allowPrivateInstances reads the tier B opt-out.
//
// A value that does not parse as a boolean is false rather than an error: this
// is a guard, and a typo in the variable that turns it off would be the one
// failure mode nobody notices. The flag is what an operator normally types,
// and a flag value that does not parse is theirs to see in the same startup.
func allowPrivateInstances() bool {
	value := config.TrimmedGetenv(AllowPrivateInstancesEnv)
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
// and is applied in [guardDestination], above any policy, so a request that
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
	// instanceHost is the lower-cased host of the instance this client talks
	// to. Empty when the base URL carried none, which makes every destination
	// off-origin: without a host to compare against there is no hop that can
	// be shown to be the instance's own.
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

// coversInstance reports whether dest is the instance's own host.
//
// It is the relation [credentialSafeRedirect] already applies to the
// credential headers, IPv6-zone guard included, and deliberately so: two
// guards on the same redirect that disagree about what "left the instance"
// means would be two policies a reader has to hold at once. The scheme is not
// part of it here, because an https-to-http downgrade does not change which
// address gets dialed, only who can read the headers.
func (p *destinationPolicy) coversInstance(dest *url.URL) bool {
	if p == nil || p.instanceHost == "" || dest == nil {
		return false
	}
	return isDomainOrSubdomain(strings.ToLower(dest.Hostname()), p.instanceHost)
}

// checkPrivate applies tier B to one candidate address.
func (p *destinationPolicy) checkPrivate(ctx context.Context, addr netip.Addr, offOrigin bool) error {
	if !offOrigin && !p.callerChosen {
		// The operator named this instance. Nothing here is checked, ever.
		return nil
	}
	if !isPrivateAddress(addr) {
		return nil
	}
	if p.allowPrivate {
		return nil
	}
	if offOrigin && p.instanceIsPrivate(ctx) {
		// A self-managed GitLab redirecting an artifact download to object
		// storage on the same private network is ordinary, and a deployment
		// whose instance is already private is inside that network. Tier A
		// still applied above, so this cannot reach a metadata address.
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
// A literal host is decided without asking anybody. A name is resolved on the
// refusal path only, so the common case pays nothing, and a resolver that
// cannot answer leaves the destination refused rather than permitted.
func (p *destinationPolicy) instanceIsPrivate(ctx context.Context) bool {
	p.privateOnce.Do(func() {
		p.privateResult = p.resolveInstancePrivate(ctx)
	})
	return p.privateResult
}

// resolveInstancePrivate is the body of [destinationPolicy.instanceIsPrivate],
// separated so the memoization and the decision can be read apart.
func (p *destinationPolicy) resolveInstancePrivate(ctx context.Context) bool {
	if p.instanceHost == "" {
		return false
	}
	if addr, spelledAsAddress := addressLiteral(p.instanceHost); spelledAsAddress {
		return isPrivateAddress(addr)
	}
	if p.lookupIP == nil {
		return false
	}
	lookupCtx, cancel := context.WithTimeout(ctx, instanceLookupTimeout)
	defer cancel()
	addrs, err := p.lookupIP(lookupCtx, p.instanceHost)
	if err != nil || len(addrs) == 0 {
		return false
	}
	// Every address, not the first: a name that answers with one private and
	// one public address is not a private deployment, and taking the first
	// would make the answer depend on resolver ordering.
	for _, addr := range addrs {
		if !isPrivateAddress(addr) {
			return false
		}
	}
	return true
}

// dialTarget is what one request tells the dialer.
//
// It travels on the request context rather than on the transport because
// [sharedBaseTransport] is one process-wide transport: a transport per client
// would give each of up to --max-http-clients pool entries its own
// idle-connection set, which is the cost that transport exists to avoid.
type dialTarget struct {
	policy *destinationPolicy
	// offOrigin reports that this request's URL is not the instance's own
	// host. For a client following a redirect that is exactly "the hop left
	// the instance", because net/http hands each hop to the transport as a
	// request of its own.
	offOrigin bool
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
// hop with the same code, since both reach the same dialer.
//
// Connection reuse does not walk past it: Go keys idle connections on scheme,
// host and port, so a connection to one destination is never handed to a
// request for another.
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
	addr := addrPort.Addr()
	if name, ok := metadataAddress(addr); ok {
		return fmt.Errorf("%w: %s is %s, which never serves a GitLab API or object storage", ErrDestinationRefused, addr, name)
	}
	target, ok := dialTargetFrom(ctx)
	if !ok || target.policy == nil {
		// Tier A applied above and is all an unstamped request gets. Nothing
		// in this server dials without a policy; a caller outside it that
		// borrows the transport through [HTTPTransport] chose its own
		// destination and is not the case this guard is about.
		return nil
	}
	return target.policy.checkPrivate(ctx, addr, target.offOrigin)
}

// destinationTransport stamps every request with the policy in force for the
// client that made it.
//
// It sits at the bottom of the chain, immediately around the base transport,
// so the stamp is structurally the last thing that happens before the dial
// rather than something a later wrapper could be inserted in front of.
type destinationTransport struct {
	base   http.RoundTripper
	client *Client
}

// RoundTrip stamps the request and delegates.
func (t *destinationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	policy := t.client.destinationPolicy()
	if policy == nil {
		return t.base.RoundTrip(req)
	}
	target := dialTarget{policy: policy, offOrigin: !policy.coversInstance(req.URL)}
	return t.base.RoundTrip(req.WithContext(withDialTarget(req.Context(), target)))
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
// and would still not be authoritative.
func CheckCallerNamedInstance(rawURL string) error {
	host, _ := credentialScope(rawURL)
	addr, spelledAsAddress := addressLiteral(host)
	if !spelledAsAddress {
		return nil
	}
	if name, ok := metadataAddress(addr); ok {
		return fmt.Errorf("%w: %s is %s, which never serves a GitLab API", ErrDestinationRefused, addr, name)
	}
	if !isPrivateAddress(addr) || allowPrivateInstances() {
		return nil
	}
	return fmt.Errorf("%w: the GITLAB-URL header named an instance on the private address %s; %s",
		ErrDestinationRefused, addr, allowPrivateAdvice)
}
