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

// metadataAddress reports whether addr is a cloud metadata endpoint, and what
// to call it in the refusal.
//
// The zone is dropped as well as the mapping, for the same reason: the table
// holds each address once, bare, and fd00:ec2::254%eth0 is that address with
// an interface named. An operating system ignores the zone of a destination
// that is not link-local, so a lookup that kept it would let the spelling
// reach the endpoint the table exists to refuse.
func metadataAddress(addr netip.Addr) (string, bool) {
	name, ok := metadataAddresses[addr.WithZone("").Unmap()]
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
		return fmt.Errorf("%w: %s is %s, which never serves a GitLab API or object storage", ErrDestinationRefused, addr, name)
	}
	if target.policy == nil {
		return nil
	}
	return target.policy.checkPrivate(ctx, addr, target.offOrigin)
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
	return pool.RoundTrip(req.WithContext(withDialTarget(req.Context(), target)))
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
func judgeProxiedDestination(ctx context.Context, target dialTarget, dest *url.URL) error {
	addr, spelledAsAddress := addressLiteral(dest.Hostname())
	if !spelledAsAddress {
		return nil
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
