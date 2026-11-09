// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// cloudflare_tunnel_test.go holds the remote deployment guide's Cloudflare
// Tunnel section to what can be run of it.
//
// The guide's config.yml names a tunnel and its credentials, which only a
// Cloudflare account has, so the configuration is checked offline by
// cloudflared itself: its ingress rules validate, and they send the guide's
// host, the MCP endpoint and the metadata at the host root to the server and
// everything else to a 404. That runs everywhere Docker does, with no
// network.
//
// The edge is exercised through a quick tunnel, which needs no account and
// opens a public trycloudflare.com address to a process on this machine.
// That runs only when E2E_CLOUDFLARE_QUICK_TUNNEL=true is set, on a
// developer machine and never on CI (decided on issue 1112, 31 October 2026).
// A quick tunnel is not the guide's named tunnel: it takes the origin from
// --url, so the config.yml above is not what it serves, and what it can show
// is what Cloudflare's edge does to a request on its way to the server.
package httpe2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// cloudflaredImage is the Cloudflare cases' image. cloudflared is released by
// date with no minor-version tag, so this names one release.
const cloudflaredImage = "cloudflare/cloudflared:2026.10.7"

// quickTunnelSwitch is the variable that lets the quick tunnel case open a
// public address.
const quickTunnelSwitch = "E2E_CLOUDFLARE_QUICK_TUNNEL"

// TestProxyGuide_CloudflareTunnelIngress validates the guide's config.yml
// with cloudflared and asks it where each request the server serves would
// go.
func TestProxyGuide_CloudflareTunnelIngress(t *testing.T) {
	t.Parallel()
	block := guideBlock(t, remoteDeploymentPage, "cloudflared")
	dir := containerDir(t)
	mounts := []string{writeContainerFile(t, dir, "config.yml", block) + ":/etc/cloudflared/config.yml:ro"}
	ingress := func(t *testing.T, args ...string) string {
		t.Helper()
		out, err := runOnce(t, cloudflaredImage, mounts, append([]string{"tunnel", "--config", "/etc/cloudflared/config.yml", "ingress"}, args...)...)
		if err != nil {
			t.Fatalf("cloudflared tunnel ingress %s refused the guide's config.yml (%v):\n%s", strings.Join(args, " "), err, out)
		}
		return out
	}

	t.Run("the ingress rules validate", func(t *testing.T) {
		t.Parallel()
		if out := ingress(t, "validate"); !strings.Contains(out, "OK") {
			t.Errorf("cloudflared did not report the rules valid:\n%s", out)
		}
	})

	for _, tc := range []struct {
		url     string
		rule    string
		service string
	}{
		{url: "https://" + publicHost + "/mcp", rule: "Matched rule #0", service: "service: http://127.0.0.1:8080"},
		{url: "https://" + publicHost + "/.well-known/oauth-protected-resource", rule: "Matched rule #0", service: "service: http://127.0.0.1:8080"},
		{url: "https://other.example.com/mcp", rule: "Matched rule #1", service: "service: http_status:404"},
	} {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			out := ingress(t, "rule", tc.url)
			if !strings.Contains(out, tc.rule) || !strings.Contains(out, tc.service) {
				t.Errorf("cloudflared sends %s elsewhere than %s, %s:\n%s", tc.url, tc.rule, tc.service, out)
			}
		})
	}
}

// quickTunnelHost matches a trycloudflare.com address in cloudflared's log.
// The quick tunnel's own is printed once it is up, and quickTunnelAddress
// keeps that one apart from the API's.
var quickTunnelHost = regexp.MustCompile(`https://([a-z0-9-]+\.trycloudflare\.com)`)

// quickTunnelAPIHost is where cloudflared asks for a quick tunnel. Its error
// lines name it, before any tunnel exists, and it is never the tunnel's own
// address.
const quickTunnelAPIHost = "api.trycloudflare.com"

// quickTunnelAddress returns the quick tunnel's host from cloudflared's log,
// or "" while the log names none.
func quickTunnelAddress(logs string) string {
	for _, match := range quickTunnelHost.FindAllStringSubmatch(logs, -1) {
		if match[1] != quickTunnelAPIHost {
			return match[1]
		}
	}
	return ""
}

// The credentials the quick tunnel case's GitLab accepts besides
// acceptedPrivateToken, each sent by one request only, so whether GitLab was
// asked about it says whether that request reached the server.
const (
	// forgedHeaderToken goes with a CF-Connecting-IP the client wrote.
	forgedHeaderToken = "glpat-sent-with-a-client-written-cf-connecting-ip"
	// directToken goes straight to the server, once the edge's client is
	// blocked. The server has not admitted it, and that matters: a
	// credential it already serves is admitted from a blocked address too
	// (cmd/server/auth_gate.go), so acceptedPrivateToken would answer 200
	// whichever address the block fell on.
	directToken = "glpat-sent-straight-to-the-server"
)

// TestProxyGuide_CloudflareQuickTunnel reaches the server through Cloudflare's
// edge and holds the rows of the requirement table a quick tunnel can show,
// with the CF-Connecting-IP pairing the guide names.
//
// The stream row is not among them: Cloudflare documents that a quick tunnel
// does not carry server-sent events, which a named tunnel does, so a held
// stream here would say nothing about the guide's tunnel. Neither is the
// OAuth metadata row, which the offline case above answers for the guide's
// ingress rules.
//
// Once the switch asks for the case, whatever keeps it from the edge fails
// it, with the reason: Docker, cloudflared, the tunnel's name and the edge
// itself. A skip would read as a pass in the log of a run started for this
// case alone, which is how its first run reported a container that did not
// come up when the name had not resolved.
//
// Run it with -timeout 20m. Its own waits add up to about eight and a half
// minutes at worst, close to go test's default of ten, and each port another
// listener takes before the server binds it adds a tunnel's start, at most
// four and a half minutes, twice at most (quickTunnelServer). A run the
// timeout or an interrupt ends runs no cleanup, so the cloudflared container
// would keep its public address open to a port another process can take.
func TestProxyGuide_CloudflareQuickTunnel(t *testing.T) {
	requireQuickTunnelSwitch(t)
	tunnel := startQuickTunnel(t)
	edge, gitlab := tunnel.edge, tunnel.gitlab

	t.Run("the edge forwards to the server", func(t *testing.T) {
		if got := edge.do(t, request{method: http.MethodGet, path: "/health"}); got.status != http.StatusOK {
			t.Errorf("GET /health through the edge = %d, want 200: %s", got.status, truncate(got.body))
		}
	})

	t.Run("PRIVATE-TOKEN reaches the server", func(t *testing.T) {
		got := edge.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": acceptedPrivateToken}))
		if got.status != http.StatusOK || gitlab.asked(acceptedPrivateToken) == 0 {
			t.Errorf("tools/list through the edge = %d with GitLab asked %d time(s), want 200 and the token checked: %s",
				got.status, gitlab.asked(acceptedPrivateToken), truncate(got.body))
		}
	})

	t.Run("the server is the only one to speak CORS", func(t *testing.T) {
		call := edge.do(t, mcpPOST(map[string]string{
			"PRIVATE-TOKEN":  acceptedPrivateToken,
			"Origin":         trustedOrigin,
			"Sec-Fetch-Site": "cross-site",
		}))
		if got := call.header.Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != trustedOrigin {
			t.Errorf("a cross-origin call through the edge carries Access-Control-Allow-Origin %q, want exactly [%q]", got, trustedOrigin)
		}
	})

	t.Run("a client-written CF-Connecting-IP is refused at the edge", func(t *testing.T) {
		assertForgedClientAddressRefused(t, tunnel)
	})

	t.Run("failures are charged to the address the edge writes", func(t *testing.T) {
		assertEdgeAddressCharged(t, tunnel)
	})
}

// quickTunnel is a server reached through a quick tunnel, and the GitLab it
// asks about tokens.
type quickTunnel struct {
	// edge reaches the server through Cloudflare's edge, as a client does.
	edge *server
	// direct reaches the server on 127.0.0.1, where cloudflared does.
	direct *server
	gitlab *tokenGitLab
}

// requireQuickTunnelSwitch skips the quick tunnel case while the switch is
// unset, and fails it where the switch is set on CI or to anything but true:
// a value that reads as asking for the case is not taken for a skip.
func requireQuickTunnelSwitch(t *testing.T) {
	t.Helper()
	switch {
	case os.Getenv(quickTunnelSwitch) == "":
		t.Skipf("set %s=true on a developer machine to run this case: it opens a public trycloudflare.com address to a server on this machine, so it never runs by default and never on CI", quickTunnelSwitch)
	case os.Getenv("CI") != "":
		t.Fatalf("%s is set on CI, where this case never runs: it opens a public address to a process on the runner", quickTunnelSwitch)
	case os.Getenv(quickTunnelSwitch) != "true":
		t.Fatalf("%s=%q: only true runs this case, and unset skips it", quickTunnelSwitch, os.Getenv(quickTunnelSwitch))
	}
}

// startQuickTunnel opens a quick tunnel to a server started with the guide's
// CF-Connecting-IP pairing and waits until the edge forwards to it. Every
// way that can fail fails the case, but for a port another listener took
// before the server bound it, which moves to another port and another tunnel
// (quickTunnelServer).
func startQuickTunnel(t *testing.T) quickTunnel {
	t.Helper()
	if err := dockerUnusable(t.Context()); err != nil {
		t.Fatalf("%s=true asks for this case, and %v", quickTunnelSwitch, err)
	}
	gitlab := startTokenGitLab(t, acceptedPrivateToken, forgedHeaderToken, directToken)
	open := func(port int) (string, func() error) {
		name := quickTunnelContainer(port)
		if out, err := runContainer(t, name, cloudflaredImage, nil, "tunnel", "--no-autoupdate", "--url", fmt.Sprintf("http://127.0.0.1:%d", port)); err != nil {
			t.Fatalf("could not start %s (%v):\n%s", cloudflaredImage, err, out)
		}
		// The cleanup removes it, and a run the timeout or an interrupt ends
		// runs none. Removed by the ID its own run was given, and by hand by
		// its own name only: another run's container can be alive beside it.
		t.Logf("%s keeps a public trycloudflare.com address open until it is removed; if this run is interrupted: docker rm -f %s", name, name)
		return awaitQuickTunnel(t, name), func() error { return killContainer(name) }
	}
	start := func(port int, host string) (*server, error) {
		return tryStartServerOnPort(t, port, nil,
			"--gitlab-url="+gitlab.url,
			"--public-url=https://"+host,
			"--trusted-origins="+trustedOrigin,
			"--trusted-proxy-header=CF-Connecting-IP",
			"--trusted-proxies=127.0.0.1",
			"--auth-failure-limit=3",
		)
	}
	port, host, srv := quickTunnelServer(t, func() int { return freePort(t) }, open, start)
	name := quickTunnelContainer(port)
	resolver := newTunnelResolver(time.Now().Add(quickTunnelDNSWait))
	t.Cleanup(func() {
		for _, question := range resolver.systemQuestions() {
			t.Log(question)
		}
	})
	if _, err := resolver.lookup(t.Context(), host); err != nil {
		t.Fatalf("DNS: %v. cloudflared is up and printed the address, so this is the name, not the container", err)
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: resolver.transport()}
	t.Cleanup(client.CloseIdleConnections)
	edge := &server{baseURL: "https://" + host, logs: srv.logs, client: client}
	awaitEdge(t, edge, name)
	return quickTunnel{edge: edge, direct: srv, gitlab: gitlab}
}

// quickTunnelContainer names the cloudflared container of a quick tunnel to
// port.
func quickTunnelContainer(port int) string {
	return fmt.Sprintf("gitlab-mcp-httpe2e-guide-cloudflared-%d", port)
}

// killContainer stops the container named name at once, leaving its removal
// to the cleanup runContainer registered, which removes it by its ID.
func killContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "docker", "kill", name).CombinedOutput(); err != nil {
		return fmt.Errorf("docker kill %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// fatalReporter is what quickTunnelServer needs of a test: a failure that
// ends it.
type fatalReporter interface {
	Helper()
	Fatalf(format string, args ...any)
}

// quickTunnelServer opens a quick tunnel to a port pick chooses, with open,
// and starts the server on that port, with start, once the tunnel has
// printed its host, which the server's --public-url names.
//
// The port is free for nobody from the moment pick releases it until the
// server binds it, which is as long as the tunnel takes to come up, so a
// server that lost it to another listener (errPortTaken) is started again on
// another port, up to portRaceAttempts ports, as startServerOnFreePort does.
// A tunnel names its origin's port when it starts, so each port has a tunnel
// of its own, and every tunnel but the one returned is stopped as soon as its
// server did not come up: until then its public address forwards to whatever
// took the port. Any other failure ends the case on the port it came on.
func quickTunnelServer(t fatalReporter, pick func() int, open func(port int) (host string, stop func() error), start func(port int, host string) (*server, error)) (port int, host string, srv *server) {
	t.Helper()
	for attempt := 1; ; attempt++ {
		port = pick()
		var stop func() error
		host, stop = open(port)
		var err error
		if srv, err = start(port, host); err == nil {
			return port, host, srv
		}
		if stopErr := stop(); stopErr != nil {
			t.Fatalf("stopping the tunnel to port %d, whose server did not come up (%v): %v", port, err, stopErr)
			return 0, "", nil
		}
		if !errors.Is(err, errPortTaken) || attempt == portRaceAttempts {
			t.Fatalf("starting the server on port %d: %v", port, err)
			return 0, "", nil
		}
	}
}

// assertForgedClientAddressRefused holds what the edge does to a
// CF-Connecting-IP a client wrote: it does not overwrite it, it refuses the
// request with its error 1000 before the tunnel, so the server never sees
// one. Measured on 4 November 2026, and with curl through a quick tunnel of
// its own.
func assertForgedClientAddressRefused(t *testing.T, tunnel quickTunnel) {
	t.Helper()
	got := tunnel.edge.do(t, mcpPOST(map[string]string{
		"PRIVATE-TOKEN":    forgedHeaderToken,
		"CF-Connecting-IP": "198.51.100.1",
	}))
	if got.status != http.StatusForbidden || !edgeError1000(got.body) {
		t.Errorf("a request carrying its own CF-Connecting-IP through the edge = %d, want the edge's 403 naming its error 1000:\n%s", got.status, got.body)
	}
	if got.header.Get("Content-Security-Policy") == serverCSP {
		t.Errorf("the answer to a request carrying its own CF-Connecting-IP has the server's Content-Security-Policy: the request reached the server")
	}
	if n := tunnel.gitlab.asked(forgedHeaderToken); n != 0 {
		t.Errorf("GitLab was asked %d time(s) about the token sent with a client-written CF-Connecting-IP: the request reached the server", n)
	}
}

// assertEdgeAddressCharged holds the pairing's other half: failures through
// the edge block the client's address, the one the edge writes in
// CF-Connecting-IP, and not cloudflared's.
func assertEdgeAddressCharged(t *testing.T, tunnel quickTunnel) {
	t.Helper()
	blocked := false
	for i := range 8 {
		got := tunnel.edge.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": fmt.Sprintf("glpat-wrong-%d", i)}))
		// The edge answers some requests itself, a 429 among them, which
		// Cloudflare documents for a quick tunnel carrying more than 200
		// requests at once, so only an answer carrying the server's own
		// header is a failure the server charged.
		if got.header.Get("Content-Security-Policy") != serverCSP {
			t.Fatalf("failure %d through the edge = %d without the server's Content-Security-Policy: the edge answered, not the server: %s", i, got.status, truncate(got.body))
		}
		if got.status == http.StatusTooManyRequests {
			if got.header.Get("Retry-After") == "" || !strings.Contains(got.body, "Too many failed authentication attempts") {
				t.Fatalf("failure %d through the edge = 429 with Retry-After %q, want the server's block of failed authentications: %s", i, got.header.Get("Retry-After"), truncate(got.body))
			}
			blocked = true
			break
		}
		if got.status != http.StatusUnauthorized {
			t.Fatalf("failure %d through the edge = %d, want 401 or 429: %s", i, got.status, truncate(got.body))
		}
	}
	if !blocked {
		t.Fatal("eight failures from this machine through the edge were never blocked with a limit of three: each was charged to another address")
	}
	assertBlockLoggedForTheEdgeAddress(t, tunnel.direct)
	// cloudflared reaches the server from 127.0.0.1, and a request without
	// the header is charged to that peer (clientIP in
	// cmd/server/client_ip.go), so a request straight to the server stands
	// for cloudflared's own address. It is admitted only if the block fell
	// elsewhere: on the address the edge wrote.
	direct := tunnel.direct.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": directToken}))
	if direct.status != http.StatusOK || tunnel.gitlab.asked(directToken) == 0 {
		t.Errorf("a credential the server has not admitted, sent straight to it from 127.0.0.1 after the block = %d with GitLab asked %d time(s), want 200 and the token checked: the failures were charged to cloudflared's address, not to the one the edge wrote in CF-Connecting-IP: %s",
			direct.status, tunnel.gitlab.asked(directToken), truncate(direct.body))
	}
}

// blockLogMessage is the line the server logs when it blocks an address for
// its failed authentications (cmd/server/auth_gate.go), with the address it
// charged as ip and the connection's own as source. The first one a process
// writes is never held back by the refusal log's throttle.
const blockLogMessage = "request blocked: too many authentication failures (legacy auth mode)"

// assertBlockLoggedForTheEdgeAddress reads the block from the server's own
// log: charged to an address that is not loopback, the one the edge wrote,
// on a connection from cloudflared's 127.0.0.1.
func assertBlockLoggedForTheEdgeAddress(t *testing.T, srv *server) {
	t.Helper()
	line := awaitLogLine(t, srv, blockLogMessage)
	var record struct {
		IP     string `json:"ip"`
		Source string `json:"source"`
	}
	switch {
	case line == "":
		t.Errorf("the server logged no %q line after the block", blockLogMessage)
	case json.Unmarshal([]byte(line), &record) != nil:
		t.Errorf("the server's block line is not a JSON record: %s", line)
	case record.Source != "127.0.0.1":
		t.Errorf("the block's source = %q, want cloudflared's 127.0.0.1: %s", record.Source, line)
	default:
		if addr, err := netip.ParseAddr(record.IP); err != nil || addr.IsLoopback() {
			t.Errorf("the block fell on %q, want the client's address the edge wrote in CF-Connecting-IP: %s", record.IP, line)
		}
	}
}

// edgeError1000 reports whether an answer of Cloudflare's edge names its
// error 1000, which is how it refuses a request that already carries a
// CF-Connecting-IP. It reads the code rather than a wording: the edge wrote
// "error code: 1000" for curl's Accept, what it writes for the MCP client's
// has not been seen yet, so the code is read wherever it lands, and the
// case's failure prints whichever came.
func edgeError1000(body string) bool {
	return edgeErrorCode.MatchString(body)
}

// edgeErrorCode matches 1000 as a number of its own.
var edgeErrorCode = regexp.MustCompile(`\b1000\b`)

// awaitQuickTunnel waits for cloudflared to print its public address and
// returns the host, failing the case when it prints none.
func awaitQuickTunnel(t *testing.T, container string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		if host := quickTunnelAddress(containerLogs(container)); host != "" {
			return host
		}
		time.Sleep(time.Second)
	}
	requireStillRunning(t, container)
	t.Fatalf("cloudflared printed no trycloudflare.com address in 90 seconds:\n%s", containerLogs(container))
	return ""
}

// quickTunnelEdgeWait is how long the edge is given to forward to the server
// once the tunnel's name resolves.
const quickTunnelEdgeWait = 90 * time.Second

// awaitEdge polls the server through Cloudflare's edge until an answer comes
// back with the server's own headers, and fails the case, saying which part
// of the way did not answer, when none does.
func awaitEdge(t *testing.T, edge *server, container string) {
	t.Helper()
	deadline := time.Now().Add(quickTunnelEdgeWait)
	var lastErr error
	lastStatus := 0
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, edge.baseURL+"/health", http.NoBody)
		if err != nil {
			t.Fatalf("building the readiness request: %v", err)
		}
		resp, err := edge.httpClient().Do(req)
		if err != nil {
			lastErr = err
		} else {
			_ = resp.Body.Close()
			if resp.Header.Get("Content-Security-Policy") == serverCSP {
				return
			}
			lastStatus = resp.StatusCode
		}
		time.Sleep(time.Second)
	}
	requireStillRunning(t, container)
	switch {
	case lastStatus != 0:
		t.Fatalf("the edge answered GET /health for %s and never with the server's answer (last status %d), with the server behind cloudflared serving.\n%s",
			quickTunnelEdgeWait, lastStatus, containerLogs(container))
	case isDNSFailure(lastErr):
		t.Fatalf("DNS: %v. The name resolved once and stopped, so the edge was never asked", lastErr)
	default:
		t.Fatalf("the edge never answered GET /health in %s (%v), with cloudflared running.\n%s", quickTunnelEdgeWait, lastErr, containerLogs(container))
	}
}

// quickTunnelResolvers are the DNS servers the case asks for a quick tunnel's
// name, in order: Cloudflare's own, which serve trycloudflare.com.
//
// A quick tunnel's name is published a moment after cloudflared prints it, so
// the first lookup can be answered that the name does not exist, and a
// resolver keeps that answer. With systemd-resolved, the case's first run
// asked once, was told the name did not exist, and never reached the edge
// although the tunnel was up; with that cache flushed every two seconds, it
// reached it. 1.1.1.1 caches a name it answered as missing too, for the 60
// seconds of trycloudflare.com's negative TTL (its SOA on 4 November 2026);
// asking again for two minutes outlasts it, and a public resolver keeps the
// machine's own configuration out of it.
var quickTunnelResolvers = []string{"1.1.1.1:53", "1.0.0.1:53"}

// quickTunnelDNSWait is how long a quick tunnel's name is given to resolve
// after cloudflared prints it.
const quickTunnelDNSWait = 2 * time.Minute

// errNameNotFound is a resolver answering that the name does not exist.
var errNameNotFound = errors.New("the name does not exist yet")

// hostLookup is one resolver's net.Resolver.LookupHost, under the address
// that names it in an error.
type hostLookup struct {
	address string
	lookup  func(ctx context.Context, host string) ([]string, error)
}

// tunnelResolver resolves a quick tunnel's name through public resolvers, so
// no cache of the machine's stands between the case and the answer.
type tunnelResolver struct {
	// public is asked in order, and the first answer is taken.
	public []hostLookup
	// system is asked only when none of public could be reached, and for the
	// rooted name too.
	system func(ctx context.Context, host string) ([]string, error)
	// deadline ends the asking again while the answer is that the name
	// does not exist; any other answer ends it at once.
	deadline time.Time
	// retry is the pause before asking again.
	retry time.Duration

	// mu guards asked.
	mu sync.Mutex
	// asked says, for each question put to system, why it came to it. An
	// answer the system's resolver gives leaves no other trace, so the case
	// logs these when it ends.
	asked []string
}

// newTunnelResolver returns a resolver that asks quickTunnelResolvers and
// keeps asking until deadline while the name does not exist.
func newTunnelResolver(deadline time.Time) *tunnelResolver {
	r := &tunnelResolver{system: net.DefaultResolver.LookupHost, deadline: deadline, retry: 2 * time.Second}
	for _, address := range quickTunnelResolvers {
		r.public = append(r.public, hostLookup{address: address, lookup: publicResolver(address).LookupHost})
	}
	return r
}

// publicResolver is Go's own resolver sending every query to address.
func publicResolver(address string) *net.Resolver {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	}
}

// lookup returns host's addresses, asking again while the answer is that the
// name does not exist and the deadline has not passed.
func (r *tunnelResolver) lookup(ctx context.Context, host string) ([]string, error) {
	for {
		addrs, err := r.ask(ctx, host)
		if err == nil || !errors.Is(err, errNameNotFound) {
			return addrs, err
		}
		if !time.Now().Before(r.deadline) {
			return nil, fmt.Errorf("%w, and kept answering so until %s", err, r.deadline.Format(time.TimeOnly))
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w; stopped asking: %w", err, ctx.Err())
		case <-time.After(r.retry):
		}
	}
}

// ask puts the question once: to each public resolver in turn until one
// answers, and to the system's resolver only when none of them could be
// reached.
func (r *tunnelResolver) ask(ctx context.Context, host string) ([]string, error) {
	var unreachable []error
	// Rooted, so no resolver, the system's included, appends the machine's
	// search domains: a wildcard record under one would answer for the
	// tunnel's name while it is not published yet, or before it with ndots
	// set high.
	rooted := host + "."
	for _, resolver := range r.public {
		addrs, err := resolver.lookup(ctx, rooted)
		switch {
		case err == nil:
			return addrs, nil
		case isNotFound(err):
			return nil, fmt.Errorf("%s answered that %s does not exist: %w", resolver.address, host, errNameNotFound)
		}
		unreachable = append(unreachable, fmt.Errorf("%s: %w", resolver.address, err))
	}
	r.noteSystemQuestion(fmt.Sprintf("asked the system's resolver for %s, since no public one answered: %v", host, errors.Join(unreachable...)))
	addrs, err := r.system(ctx, rooted)
	switch {
	case err == nil:
		return addrs, nil
	case isNotFound(err):
		return nil, fmt.Errorf("the system's resolver, asked because no public one answered (%w), answered that %s does not exist: %w", errors.Join(unreachable...), host, errNameNotFound)
	}
	return nil, fmt.Errorf("no resolver answered for %s: %w", host, errors.Join(append(unreachable, fmt.Errorf("the system's resolver: %w", err))...))
}

// noteSystemQuestion records why a question went to the system's resolver.
func (r *tunnelResolver) noteSystemQuestion(why string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, why)
}

// systemQuestions returns why each question put to the system's resolver
// went to it.
func (r *tunnelResolver) systemQuestions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.asked)
}

// dialContext dials address, resolving a trycloudflare.com name with lookup
// and any other as the system does.
//
// Each address is dialed with a timeout of its own and in turn, without the
// standard dialer's fallback between IPv6 and IPv4, so an IPv6 route that
// takes the packets and never answers costs five seconds, not the client's
// thirty, before the next address is tried.
func (r *tunnelResolver) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	host, port, err := net.SplitHostPort(address)
	if err != nil || !strings.HasSuffix(host, ".trycloudflare.com") {
		return dialer.DialContext(ctx, network, address)
	}
	addrs, err := r.lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, addr := range addrs {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(addr, port))
		if dialErr == nil {
			return conn, nil
		}
		failures = append(failures, dialErr)
	}
	return nil, errors.Join(failures...)
}

// transport is an HTTP transport that dials through dialContext.
//
// It takes no proxy from the environment: with HTTPS_PROXY set, dialContext
// would be handed the proxy's address, the proxy would resolve the tunnel's
// name past the case's resolver, and the edge would write the proxy's address
// in CF-Connecting-IP.
func (r *tunnelResolver) transport() *http.Transport {
	return &http.Transport{
		Proxy:               nil,
		DialContext:         r.dialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
}

// isNotFound reports whether a resolver answered that the name does not
// exist, as opposed to not answering.
func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// isDNSFailure reports whether a request failed resolving its name.
func isDNSFailure(err error) bool {
	var dnsErr *net.DNSError
	return errors.Is(err, errNameNotFound) || errors.As(err, &dnsErr)
}

// TestQuickTunnelAddress_APIHostLogged_IsNotTaken holds the address the quick
// tunnel case reaches to the tunnel's own: an error line that names the API
// cloudflared asks for a tunnel is not an address to send requests to.
func TestQuickTunnelAddress_APIHostLogged_IsNotTaken(t *testing.T) {
	t.Parallel()
	const box = "INF |  Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):  |\n" +
		"INF |  https://seasonal-deck-organisms-sf.trycloudflare.com                                      |\n"
	const apiError = `ERR Error unmarshaling QuickTunnel response: unexpected end of JSON input url=https://api.trycloudflare.com/tunnel` + "\n"
	// The first line cloudflared 2026.10.7 printed in the case's run of 4
	// November 2026, which links two pages before any tunnel exists.
	const terms = "INF Thank you for trying Cloudflare Tunnel. Doing so, without a Cloudflare account, is a quick way to experiment and try it out. However, be aware that these account-less Tunnels have no uptime guarantee, are subject to the Cloudflare Online Services Terms of Use (https://www.cloudflare.com/website-terms/), and Cloudflare reserves the right to investigate your use of Tunnels for violations of such terms. If you intend to use Tunnels in production you should use a pre-created named tunnel by following: https://developers.cloudflare.com/cloudflare-one/connections/connect-apps\n"
	tests := []struct {
		name string
		logs string
		want string
	}{
		{name: "nothing printed yet", logs: "INF Requesting new quick Tunnel on trycloudflare.com...\n", want: ""},
		{name: "the API's address alone", logs: apiError, want: ""},
		{name: "the API's address before the tunnel's", logs: apiError + box, want: "seasonal-deck-organisms-sf.trycloudflare.com"},
		{name: "the tunnel's address", logs: box, want: "seasonal-deck-organisms-sf.trycloudflare.com"},
		{name: "the links cloudflared prints before the tunnel's", logs: terms + box, want: "seasonal-deck-organisms-sf.trycloudflare.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := quickTunnelAddress(tt.logs); got != tt.want {
				t.Errorf("quickTunnelAddress = %q, want %q", got, tt.want)
			}
		})
	}
}

// lookupAnswer is one answer a scripted resolver gives.
type lookupAnswer struct {
	addrs []string
	err   error
}

// scriptedLookup is a resolver that answers from a script, repeating its last
// answer, and records the names it was asked for.
type scriptedLookup struct {
	mu      sync.Mutex
	answers []lookupAnswer
	asked   []string
}

// lookup is the scripted resolver's net.Resolver.LookupHost.
func (s *scriptedLookup) lookup(_ context.Context, host string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, host)
	if len(s.answers) == 0 {
		return nil, errors.New("this resolver was not to be asked")
	}
	answer := s.answers[min(len(s.asked), len(s.answers))-1]
	return answer.addrs, answer.err
}

// names returns the names the scripted resolver was asked for.
func (s *scriptedLookup) names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.asked)
}

// TestTunnelResolver_Lookup_AsksThePublicResolversFirst holds where the quick
// tunnel case asks for the tunnel's name: the public resolvers in turn, again
// while the name is not published yet, and the system's resolver only when
// no public one can be reached. Each failure stays a DNS failure, so the case
// reports it as one.
func TestTunnelResolver_Lookup_AsksThePublicResolversFirst(t *testing.T) {
	t.Parallel()
	const host = "seasonal-deck-organisms-sf.trycloudflare.com"
	edge := []string{"104.16.230.132", "2606:4700::6810:e684"}
	notFound := &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	unreachable := &net.DNSError{Err: "i/o timeout", Name: host, IsTimeout: true}
	tests := []struct {
		name                  string
		first, second, system []lookupAnswer
		want                  []string
		wantNotFound          bool
		wantErr               bool
		// asked is how many times the first, the second and the system's
		// resolver were asked; firstAtLeast reads the first as a minimum,
		// for the case that asks until a deadline.
		asked        [3]int
		firstAtLeast bool
	}{
		{
			name:  "the first public resolver answers",
			first: []lookupAnswer{{addrs: edge}},
			want:  edge, asked: [3]int{1, 0, 0},
		},
		{
			name:   "the second answers when the first cannot be reached",
			first:  []lookupAnswer{{err: unreachable}},
			second: []lookupAnswer{{addrs: edge}},
			want:   edge, asked: [3]int{1, 1, 0},
		},
		{
			name:  "a name not published yet is asked for again until it is",
			first: []lookupAnswer{{err: notFound}, {err: notFound}, {addrs: edge}},
			want:  edge, asked: [3]int{3, 0, 0},
		},
		{
			name:    "a name never published ends at the deadline, not found",
			first:   []lookupAnswer{{err: notFound}},
			wantErr: true, wantNotFound: true, asked: [3]int{2, 0, 0}, firstAtLeast: true,
		},
		{
			name:   "the system's resolver when no public one can be reached",
			first:  []lookupAnswer{{err: unreachable}},
			second: []lookupAnswer{{err: unreachable}},
			system: []lookupAnswer{{addrs: edge}},
			want:   edge, asked: [3]int{1, 1, 1},
		},
		{
			name:    "no resolver reached is not a name not found",
			first:   []lookupAnswer{{err: unreachable}},
			second:  []lookupAnswer{{err: unreachable}},
			system:  []lookupAnswer{{err: unreachable}},
			wantErr: true, asked: [3]int{1, 1, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			first := &scriptedLookup{answers: tt.first}
			second := &scriptedLookup{answers: tt.second}
			system := &scriptedLookup{answers: tt.system}
			r := &tunnelResolver{
				public:   []hostLookup{{address: "1.1.1.1:53", lookup: first.lookup}, {address: "1.0.0.1:53", lookup: second.lookup}},
				system:   system.lookup,
				deadline: time.Now().Add(time.Second),
				retry:    10 * time.Millisecond,
			}
			got, err := r.lookup(t.Context(), host)
			switch {
			case tt.wantErr && err == nil:
				t.Fatalf("lookup = %v, want an error", got)
			case !tt.wantErr && err != nil:
				t.Fatalf("lookup: %v", err)
			case err != nil && !isDNSFailure(err):
				t.Errorf("lookup's error %v is not read as a DNS failure, so the case would not report it as one", err)
			case errors.Is(err, errNameNotFound) != tt.wantNotFound:
				t.Errorf("lookup's error %v wraps errNameNotFound = %v, want %v", err, errors.Is(err, errNameNotFound), tt.wantNotFound)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("lookup = %v, want %v", got, tt.want)
			}
			asked := [3]int{len(first.names()), len(second.names()), len(system.names())}
			if tt.firstAtLeast && asked[0] >= tt.asked[0] {
				asked[0] = tt.asked[0]
			}
			if asked != tt.asked {
				t.Errorf("the first, the second and the system's resolver were asked %v time(s), want %v", asked, tt.asked)
			}
			assertSystemQuestionsNoted(t, r, tt.asked[2])
			for _, name := range slices.Concat(first.names(), second.names(), system.names()) {
				if name != host+"." {
					t.Errorf("a resolver was asked for %q, want the rooted %q, which no search domain is appended to", name, host+".")
				}
			}
		})
	}
}

// assertSystemQuestionsNoted holds the resolver to noting each question it
// put to the system's resolver, which the case logs, since an answer from it
// leaves no other trace.
func assertSystemQuestionsNoted(t *testing.T, r *tunnelResolver, want int) {
	t.Helper()
	if questions := r.systemQuestions(); len(questions) != want {
		t.Errorf("the resolver noted %d question(s) put to the system's resolver, %q, want one for each of its %d", len(questions), questions, want)
	}
}

// TestTunnelResolver_DialContext_ResolvesOnlyTrycloudflareNames holds the
// edge client's dialer to its one job: a trycloudflare.com name is resolved
// by the case's resolver, and any other address is dialed as given.
func TestTunnelResolver_DialContext_ResolvesOnlyTrycloudflareNames(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("reading the listener's port: %v", err)
	}
	tests := []struct {
		name    string
		address string
		asked   int
	}{
		{name: "a trycloudflare.com name is resolved by the case's resolver", address: net.JoinHostPort("seasonal-deck-organisms-sf.trycloudflare.com", port), asked: 1},
		{name: "any other address is dialed as given", address: net.JoinHostPort("127.0.0.1", port), asked: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			public := &scriptedLookup{answers: []lookupAnswer{{addrs: []string{"127.0.0.1"}}}}
			r := &tunnelResolver{
				public:   []hostLookup{{address: "1.1.1.1:53", lookup: public.lookup}},
				system:   (&scriptedLookup{}).lookup,
				deadline: time.Now(),
				retry:    time.Millisecond,
			}
			conn, dialErr := r.dialContext(t.Context(), "tcp", tt.address)
			if dialErr != nil {
				t.Fatalf("dialContext(%s): %v", tt.address, dialErr)
			}
			_ = conn.Close()
			if got := len(public.names()); got != tt.asked {
				t.Errorf("dialing %s asked the case's resolver %d time(s), want %d", tt.address, got, tt.asked)
			}
		})
	}
}

// TestEdgeError1000_EdgeAnswers_AreReadByTheirCode holds the reading of the
// edge's refusal to its code: 1000 as a number of its own, and no other.
func TestEdgeError1000_EdgeAnswers_AreReadByTheirCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "the refusal curl was given", body: "error code: 1000", want: true},
		{name: "another error of the edge's", body: "error code: 1020", want: false},
		{name: "a longer number", body: "error code: 10000", want: false},
		{name: "no body", body: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := edgeError1000(tt.body); got != tt.want {
				t.Errorf("edgeError1000(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// TestQuickTunnelServer_PortTaken_StopsTheTunnelAndMovesToAnotherPort holds
// the quick tunnel case to the rule every other case here keeps: a server
// whose port another listener took in the meantime is started again on
// another port.
//
// The tunnel names its origin's port when it starts, and the server can only
// start once the tunnel has printed the host its --public-url names, so the
// port stays free for nobody for as long as cloudflared takes to come up, the
// longest such moment in the module. A quick tunnel cannot be pointed at
// another port, so moving means another tunnel, and the first one is stopped
// before it is: until then the public address forwards to whatever took the
// port.
func TestQuickTunnelServer_PortTaken_StopsTheTunnelAndMovesToAnotherPort(t *testing.T) {
	t.Parallel()
	portTaken := fmt.Errorf("%w. Output: bind: address already in use", errPortTaken)
	tests := []struct {
		name string
		// starts is what each start returns, the last repeated.
		starts   []error
		stopErr  error
		want     []string
		wantPort int
		wantFail string
	}{
		{
			name:   "a port another listener took moves to another port and another tunnel",
			starts: []error{portTaken, nil},
			want: []string{
				"open 40001", "start 40001 tunnel-1.trycloudflare.com", "stop tunnel-1.trycloudflare.com",
				"open 40002", "start 40002 tunnel-2.trycloudflare.com",
			},
			wantPort: 40002,
		},
		{
			name:     "any other failure ends the case on the first port",
			starts:   []error{errors.New("the server never became healthy")},
			want:     []string{"open 40001", "start 40001 tunnel-1.trycloudflare.com", "stop tunnel-1.trycloudflare.com"},
			wantFail: "never became healthy",
		},
		{
			name:   "a port taken every time ends the case after the attempts a race is given",
			starts: []error{portTaken},
			want: []string{
				"open 40001", "start 40001 tunnel-1.trycloudflare.com", "stop tunnel-1.trycloudflare.com",
				"open 40002", "start 40002 tunnel-2.trycloudflare.com", "stop tunnel-2.trycloudflare.com",
				"open 40003", "start 40003 tunnel-3.trycloudflare.com", "stop tunnel-3.trycloudflare.com",
			},
			wantFail: "the port was taken",
		},
		{
			name:     "a tunnel that cannot be stopped ends the case before another opens",
			starts:   []error{portTaken, nil},
			stopErr:  errors.New("docker kill: exit status 1"),
			want:     []string{"open 40001", "start 40001 tunnel-1.trycloudflare.com", "stop tunnel-1.trycloudflare.com"},
			wantFail: "docker kill",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var events []string
			next := 40000
			pick := func() int { next++; return next }
			opened := 0
			open := func(port int) (string, func() error) {
				opened++
				host := fmt.Sprintf("tunnel-%d.trycloudflare.com", opened)
				events = append(events, fmt.Sprintf("open %d", port))
				return host, func() error {
					events = append(events, "stop "+host)
					return tt.stopErr
				}
			}
			started := 0
			start := func(port int, host string) (*server, error) {
				events = append(events, fmt.Sprintf("start %d %s", port, host))
				err := tt.starts[min(started, len(tt.starts)-1)]
				started++
				if err != nil {
					return nil, err
				}
				return &server{baseURL: fmt.Sprintf("http://127.0.0.1:%d", port)}, nil
			}
			reporter := &fatalRecorder{}
			port, host, srv := quickTunnelServer(reporter, pick, open, start)
			if !slices.Equal(events, tt.want) {
				t.Errorf("events = %q, want %q", events, tt.want)
			}
			switch {
			case tt.wantFail == "" && len(reporter.failures) != 0:
				t.Errorf("failures = %q, want none", reporter.failures)
			case tt.wantFail != "" && (len(reporter.failures) != 1 || !strings.Contains(reporter.failures[0], tt.wantFail)):
				t.Errorf("failures = %q, want one naming %q", reporter.failures, tt.wantFail)
			}
			if tt.wantFail == "" {
				wantHost := fmt.Sprintf("tunnel-%d.trycloudflare.com", opened)
				if port != tt.wantPort || host != wantHost || srv == nil || srv.baseURL != fmt.Sprintf("http://127.0.0.1:%d", tt.wantPort) {
					t.Errorf("quickTunnelServer = (%d, %q, %v), want port %d, %s and the server started there", port, host, srv, tt.wantPort, wantHost)
				}
			}
		})
	}
}

// TestCloudflareTunnelTarget_OverlappingRuns_EachRemovesOnlyItsOwnContainer
// runs make test-e2e-cloudflare-tunnel's recipe twice at once in one
// checkout, against a stand-in gotestsum and a stand-in docker, and holds the
// run that fails or is interrupted to removing the container its own case
// named and no other.
//
// Every run read the names from one log path and deleted it before it
// started, so a second run unlinked the first one's log and wrote its own in
// its place: the first run, once it failed, removed the second run's
// cloudflared container, killing that tunnel mid-case, and left its own
// container, and its public address, running. Two runs beside each other are
// a state the case is built for, since it names its container after a port
// no other run holds.
//
// The recipe is rendered with make -n rather than read as text, so what runs
// is the command make would run. Linux only: the target needs POSIX job
// control (on Windows it runs under WSL), and macOS ships make 3.81, older
// than the 4.2 whose $(file <) the Makefile reads its version with; CI's
// e2e-http job runs this module on Linux.
func TestCloudflareTunnelTarget_OverlappingRuns_EachRemovesOnlyItsOwnContainer(t *testing.T) {
	t.Parallel()
	target := newTunnelTarget(t)
	tests := []struct {
		name string
		// interrupt sends the first run INT where it is otherwise left to
		// fail.
		interrupt  bool
		wantStatus int
		wantReason string
	}{
		{name: "the first run fails", wantStatus: 1, wantReason: "ended with status 1"},
		{name: "the first run is interrupted", interrupt: true, wantStatus: 130, wantReason: "was interrupted by INT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recipe := target.render(t)
			state := t.TempDir()
			first := target.start(t, recipe, state, "cf-first", 1)
			awaitFile(t, filepath.Join(state, "cf-first"))
			second := target.start(t, recipe, state, "cf-second", 0)
			awaitFile(t, filepath.Join(state, "cf-second"))

			if tt.interrupt {
				if err := first.cmd.Process.Signal(os.Interrupt); err != nil {
					t.Fatalf("interrupting the first run: %v", err)
				}
			} else {
				release(t, state, "cf-first")
			}
			if status := first.wait(t); status != tt.wantStatus {
				t.Errorf("the first run ended with status %d, want %d:\n%s", status, tt.wantStatus, first.stderr.String())
			}
			if want := "the run " + tt.wantReason + "; removing cf-first"; !strings.Contains(first.stderr.String(), want) {
				t.Errorf("the first run printed %q, want it to say %q", first.stderr.String(), want)
			}
			removed, err := os.ReadFile(filepath.Join(state, "removed-by-cf-first"))
			if err != nil || string(removed) != "cf-first\n" {
				t.Errorf("the first run removed %q (%v), want its own container, cf-first, alone", removed, err)
			}
			if _, statErr := os.Stat(filepath.Join(state, "cf-second")); statErr != nil {
				t.Errorf("the second run's container is gone while that run is still going (%v): the first run removed a container another run's case named", statErr)
			}

			release(t, state, "cf-second")
			if status := second.wait(t); status != 0 {
				t.Errorf("the second run ended with status %d, want 0:\n%s", status, second.stderr.String())
			}
		})
	}
}

// TestCloudflareTunnelTarget_Cover_HandsTheRunALeafOfItsOwn holds make
// test-e2e-cloudflare-tunnel to measuring what it runs under COVER=1, as
// every other run target does: the leaf cleared first and handed to the
// harness, which gives each server child a GOCOVERDIR under it.
//
// The target built the instrumented binary and handed it no leaf, so every
// server it started wrote nothing and printed "GOCOVERDIR not set" first,
// and make e2e-go-coverage left the run out without a word.
//
// The leaf is cleared by the Makefile's RM_RF, which took its path unquoted:
// under a coverage root such as "dist/e2e cover" it removed dist/e2e, the
// staged server binary in it, and a "cover/cloudflare-tunnel" nobody named,
// and left the leaf's stale counters in place.
func TestCloudflareTunnelTarget_Cover_HandsTheRunALeafOfItsOwn(t *testing.T) {
	t.Parallel()
	target := newTunnelTarget(t)
	leaf := filepath.Join(target.root, "dist", "e2e-cover", "cloudflare-tunnel")
	t.Run("with COVER=1", func(t *testing.T) {
		t.Parallel()
		rendered := target.renderAll(t, "COVER=1")
		if !strings.Contains(rendered, "rm -rf \"dist/e2e-cover/cloudflare-tunnel\"\n") {
			t.Errorf("the target does not clear its leaf before the run:\n%s", rendered)
		}
		if !strings.Contains(rendered, `E2E_COVER_DIR="`+leaf+`" `) {
			t.Errorf("the run is not handed E2E_COVER_DIR=%s:\n%s", leaf, rendered)
		}
	})
	t.Run("with a coverage root holding a space", func(t *testing.T) {
		t.Parallel()
		rendered := target.renderAll(t, "COVER=1", "E2E_COVER_ROOT=dist/e2e cover")
		if want := filepath.Join(target.root, "dist", "e2e cover", "cloudflare-tunnel"); !strings.Contains(rendered, `E2E_COVER_DIR="`+want+`" `) {
			t.Errorf("the run is not handed E2E_COVER_DIR=%s:\n%s", want, rendered)
		}
		// The removal runs in a tree of the test's own, beside the staged
		// server binary it has to leave in place.
		tree := t.TempDir()
		binary := filepath.Join(tree, "dist", "e2e", "gitlab-mcp-server")
		stale := filepath.Join(tree, "dist", "e2e cover", "cloudflare-tunnel", "covmeta.stale")
		touch(t, binary, stale)
		runTheRemoval(t, rendered, tree)
		if _, err := os.Stat(binary); err != nil {
			t.Errorf("clearing the leaf removed the staged server binary: %v", err)
		}
		if _, err := os.Stat(filepath.Dir(stale)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("clearing the leaf left it and its stale counters in place (%v)", err)
		}
	})
	t.Run("without it", func(t *testing.T) {
		t.Parallel()
		if rendered := target.renderAll(t); strings.Contains(rendered, "E2E_COVER_DIR") || strings.Contains(rendered, "e2e-cover") {
			t.Errorf("a run without COVER names a coverage leaf:\n%s", rendered)
		}
	})
}

// TestCloudflareTunnelTarget_CheckoutPathWithASpace_HandsTheRunWholePaths
// renders make test-e2e-cloudflare-tunnel in a checkout whose path holds a
// space and runs it against the stand-ins, holding the run to being handed
// the server binary and, under COVER=1, the coverage leaf, each as one path.
//
// The recipe joined both onto $(CURDIR) unquoted, so the shell split them at
// the space and ran the rest of the path as a command: the run ended with
// status 127 before gotestsum started. A checkout under a Windows home folder
// with a space in its name, reached from WSL, is such a path. The stand-in
// gotestsum sits under such a folder too (newTunnelTarget), as a GOBIN or a
// GOPATH under that home folder would put it, and the recipe ran it unquoted
// as well, which ended this run, and every other one the tests here start,
// with status 127 the same way.
func TestCloudflareTunnelTarget_CheckoutPathWithASpace_HandsTheRunWholePaths(t *testing.T) {
	t.Parallel()
	target := newTunnelTarget(t)
	checkout := checkoutWithASpace(t)
	target.root = checkout
	binary := filepath.Join(checkout, "dist", "e2e", "gitlab-mcp-server")

	tests := []struct {
		name string
		args []string
		// want is what the stand-in gotestsum was handed: the server binary
		// on one line and the coverage leaf on the next.
		want string
	}{
		{name: "without COVER", want: binary + "\n\n"},
		{name: "with COVER=1", args: []string{"COVER=1"}, want: binary + "\n" + filepath.Join(checkout, "dist", "e2e-cover", "cloudflare-tunnel") + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recipe := target.render(t, tt.args...)
			state := t.TempDir()
			// Released before it starts, so the stand-in ends at once.
			release(t, state, "cf-space")
			run := target.start(t, recipe, state, "cf-space", 0)
			if status := run.wait(t); status != 0 {
				t.Fatalf("the run ended with status %d, want 0:\n%s", status, run.stderr.String())
			}
			handed, err := os.ReadFile(filepath.Join(state, "cf-space")) //#nosec G304 -- the mark the stand-in wrote in this test's own directory
			if err != nil {
				t.Fatalf("the stand-in gotestsum left no mark: %v", err)
			}
			if string(handed) != tt.want {
				t.Errorf("the run was handed %q, want %q", handed, tt.want)
			}
		})
	}
}

// runTheRemoval runs, with sh in dir, the one rm -rf line make -n printed in
// rendered, failing the test when it printed none or more than one, or when
// the line fails.
func runTheRemoval(t *testing.T, rendered, dir string) {
	t.Helper()
	var removals []string
	for line := range strings.Lines(rendered) {
		if strings.HasPrefix(line, "rm -rf ") {
			removals = append(removals, line)
		}
	}
	if len(removals) != 1 {
		t.Fatalf("make -n printed %d removals, want the leaf's alone:\n%s", len(removals), rendered)
	}
	//#nosec G204 -- a line make rendered from this repository's own Makefile
	removal := exec.CommandContext(t.Context(), "sh", "-c", removals[0])
	removal.Dir = dir
	if out, err := removal.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v: %s", strings.TrimSpace(removals[0]), err, out)
	}
}

// tunnelTarget is what rendering and running the test-e2e-cloudflare-tunnel
// recipe needs: make, the repository the Makefile sits in, and a directory
// holding the stand-in gotestsum and docker.
type tunnelTarget struct {
	makeBin, root, stubs string
}

// The stand-ins the recipe runs. The gotestsum one writes the log line the
// case prints to the file --jsonfile names, marks its container as existing,
// writing into the mark the server binary and the coverage leaf the run was
// handed, and waits for its release; the docker one answers an inspect from
// those marks and records each removal under the run that made it. Each reads
// the run it belongs to from FAKE_*.
const (
	stubGotestsum = `#!/bin/sh
log=
while [ $# -gt 0 ]; do
  if [ "$1" = --jsonfile ]; then log=$2; fi
  shift
done
printf '{"Action":"output","Output":"%s keeps a public trycloudflare.com address open until it is removed; if this run is interrupted: docker rm -f %s\\n"}\n' "$FAKE_NAME" "$FAKE_NAME" > "$log"
printf '%s\n%s\n' "$E2E_SERVER_BINARY" "${E2E_COVER_DIR-}" > "$FAKE_STATE/$FAKE_NAME"
while [ ! -e "$FAKE_STATE/release-$FAKE_NAME" ]; do sleep 0.05; done
exit "$FAKE_STATUS"
`
	stubDocker = `#!/bin/sh
case "$1 $2" in
"container inspect") [ -e "$FAKE_STATE/$3" ] ;;
"rm -f") rm -f "$FAKE_STATE/$3" && printf '%s\n' "$3" >> "$FAKE_STATE/removed-by-$FAKE_NAME" ;;
*) exit 2 ;;
esac
`
)

// newTunnelTarget finds make, sh and bash, skipping the test where one is
// missing or the platform is not Linux, and writes the stand-ins under a
// folder whose path holds a space, as a GOBIN, a GOPATH or a PATH directory
// can, so every recipe rendered here hands the shell such a gotestsum path.
func newTunnelTarget(t *testing.T) tunnelTarget {
	t.Helper()
	target := tunnelTarget{
		makeBin: makeOnLinux(t),
		root:    repoRoot(),
		stubs:   filepath.Join(t.TempDir(), "bin with space"),
	}
	for name, body := range map[string]string{"gotestsum": stubGotestsum, "docker": stubDocker} {
		writeStandIn(t, filepath.Join(target.stubs, name), body)
	}
	return target
}

// renderAll returns everything make -n prints for the target, the
// prerequisites' commands included, with the stand-in gotestsum and a
// reports directory of the test's own.
func (tt tunnelTarget) renderAll(t *testing.T, args ...string) string {
	t.Helper()
	return renderMake(t, tt.makeBin, tt.root, append([]string{
		"test-e2e-cloudflare-tunnel",
		"GOTESTSUM=" + filepath.Join(tt.stubs, "gotestsum"), "E2E_REPORT_DIR=" + t.TempDir(),
	}, args...)...)
}

// render returns the target's own recipe line, the one that runs the case,
// as make -n prints it with args.
func (tt tunnelTarget) render(t *testing.T, args ...string) string {
	t.Helper()
	rendered := tt.renderAll(t, args...)
	start := strings.Index(rendered, "exec bash -o pipefail -c '")
	end := strings.LastIndex(rendered, `exit "$status"'`)
	if start < 0 || end < start {
		t.Fatalf("make -n printed no recipe running the case under bash:\n%s", rendered)
	}
	return rendered[start : end+len(`exit "$status"'`)]
}

// tunnelRun is one run of the rendered recipe.
type tunnelRun struct {
	cmd    *exec.Cmd
	stderr *strings.Builder
	done   chan struct{}
	err    error
}

// start runs recipe the way make would, with sh, with the stand-ins first on
// the PATH, as the run whose case names the container name and whose
// gotestsum ends with status once released.
func (tt tunnelTarget) start(t *testing.T, recipe, state, name string, status int) *tunnelRun {
	t.Helper()
	//#nosec G204 -- the recipe is what make rendered from this repository's own Makefile
	cmd := exec.CommandContext(t.Context(), "sh", "-c", recipe)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PATH="+tt.stubs+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_STATE="+state, "FAKE_NAME="+name, fmt.Sprintf("FAKE_STATUS=%d", status))
	run := &tunnelRun{cmd: cmd, stderr: &strings.Builder{}, done: make(chan struct{})}
	cmd.Stdout = io.Discard
	cmd.Stderr = run.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the recipe: %v", err)
	}
	go func() {
		run.err = cmd.Wait()
		close(run.done)
	}()
	// Released however the test ends, so no stand-in outlives it waiting.
	t.Cleanup(func() {
		release(t, state, name)
		<-run.done
	})
	return run
}

// wait returns the run's exit status once it has ended, failing the test
// when it has not within a minute.
func (r *tunnelRun) wait(t *testing.T) int {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(time.Minute):
		// Its output is not read here: the run still writes it.
		t.Fatal("the run had not ended a minute after it was released")
	}
	if exit, ok := errors.AsType[*exec.ExitError](r.err); ok {
		return exit.ExitCode()
	}
	if r.err != nil {
		t.Fatalf("waiting for the run: %v", r.err)
	}
	return 0
}

// release lets the stand-in gotestsum of the run whose case names name end.
func release(t *testing.T, state, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(state, "release-"+name), nil, 0o600); err != nil {
		t.Errorf("releasing %s: %v", name, err)
	}
}

// awaitFile waits up to thirty seconds for path to exist.
func awaitFile(t *testing.T, path string) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
	t.Fatalf("%s did not appear within thirty seconds", path)
}

// fatalRecorder is a fatalReporter that keeps its failures for the test to
// read, where testing.T would end the test.
type fatalRecorder struct {
	failures []string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}
