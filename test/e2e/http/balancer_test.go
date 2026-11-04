// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// balancer_test.go runs several instances of the real binary behind a real
// balancer, which is the shape an enterprise deployment has and the one
// nothing else here covers.
//
// proxy_test.go answers "does a proxy in front of one instance break
// anything". This answers the questions that only exist once there is more
// than one: does the affinity a deployment configures actually pin a
// credential to one instance, do the instances agree on what they serve, and
// does a balancer take an instance out before it stops listening.
//
// Two balancers, because they can do different things. nginx open source has
// no active health check at all, so its ejection is passive and --drain-delay
// buys it nothing; HAProxy polls /health, which is what the drain window was
// added for. Documenting one and testing the other would have hidden exactly
// that difference.
//
// The balancers serve the blocks the pages print, read through
// internal/testutil/docblocks (guide_test.go): the nginx balancer of the load
// balancing page and the worked one of the remote deployment guide, and the
// load balancing page's HAProxy, each with the three backends it lists, its
// salt and its check interval as printed. What a case changes is its table of
// edits, the listen port and the backend addresses; what it adds is the
// response header that names the backend, outside the block.
//
// Docker is required and both skip without it, because the point is to run
// the real balancer rather than to model one; where the job says Docker is
// expected, they fail instead (environmentUnavailablef).
package httpe2e

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// haproxyImage is every HAProxy case's image, pinned to a minor version.
const haproxyImage = "haproxy:3.4-alpine"

// affinityTokens are the credentials the affinity assertions are made with.
//
// The assertion has two halves: every token must stay on one instance, and the
// tokens together must not all land on the same one. The second half is a
// probability, not a property, because the ring the balancer builds is keyed on
// each backend's name and those names carry a port this test picks freshly on
// every run, so which side of the ring a fixed token falls on is a fresh draw
// each time rather than a constant of the token.
//
// Eight was the first count, on the reasoning that a smaller set would fail the
// second half by chance; eight fails it too. Two backends split a ketama ring
// near enough to evenly that each token is close to a coin flip, so eight
// tokens land on one side about 2^-8 of the time per assertion, twice per test
// and in three balancer tests, which is often enough to be seen: it was, on
// PR 747. Twenty-four puts that at about 2^-24 and costs one more second. The
// documented balancers list three backends, which only lowers the odds.
var affinityTokens = []string{
	"glpat-alpha", "glpat-bravo", "glpat-charlie", "glpat-delta",
	"glpat-echo", "glpat-foxtrot", "glpat-golf", "glpat-hotel",
	"glpat-india", "glpat-juliett", "glpat-kilo", "glpat-lima",
	"glpat-mike", "glpat-november", "glpat-oscar", "glpat-papa",
	"glpat-quebec", "glpat-romeo", "glpat-sierra", "glpat-tango",
	"glpat-uniform", "glpat-victor", "glpat-whiskey", "glpat-xray",
}

// documentedBackends are the backends the documented balancers list, in
// order. A case starts one instance for each.
var documentedBackends = []string{"10.0.0.11:8080", "10.0.0.12:8080", "10.0.0.13:8080"}

// backendEdits replaces each documented backend with an instance the case
// started, once each.
func backendEdits(ports []int) []edit {
	edits := make([]edit, 0, len(documentedBackends))
	for i, backend := range documentedBackends {
		edits = append(edits, edit{old: backend, new: fmt.Sprintf("127.0.0.1:%d", ports[i]), count: 1})
	}
	return edits
}

// nginxBalancerFrame is the nginx.conf a documented balancer is included from.
//
// It carries the one addition the affinity assertions need: the backend that
// served each response, echoed in a header so the test can read the routing
// decision it is asserting about. It is set here, at the http level, and
// reaches the block's locations because nginx inherits add_header into a level
// that sets none of its own, which none of the documented ones do.
const nginxBalancerFrame = `events { worker_connections 64; }
http {
  access_log off;
  add_header X-Served-By $upstream_addr always;
  include /etc/nginx/conf.d/mcp.conf;
}
`

// haproxyServedBy is the same addition for HAProxy, appended to the block,
// whose last section is the backend it applies to.
const haproxyServedBy = "  http-response set-header X-Served-By %si:%sp\n"

// balancer is a running balancer and the client that reaches it.
type balancer struct {
	container string
	// through reaches the balancer as a client reaching mcp.example.com does,
	// on a new connection every time: a kept-alive connection is already
	// pinned to a backend, which would make the affinity assertion pass
	// without the balancer ever hashing anything.
	through *server
}

// startBalancer runs a balancer container on a port it picks and waits until
// it forwards to the instances behind it, picking another port when a
// listener took the first. mounts writes the configuration for the port it is
// given and returns the bind mounts; roots verifies the balancer's
// certificate, and nil reaches it over plain HTTP.
func startBalancer(t *testing.T, kind, image string, roots *x509.CertPool, mounts func(port int) []string) balancer {
	t.Helper()
	scheme := "https"
	if roots == nil {
		scheme = "http"
	}
	for attempt := 1; ; attempt++ {
		port := freePort(t)
		name := fmt.Sprintf("gitlab-mcp-httpe2e-lb-%s-%d", kind, port)
		startContainer(t, name, image, mounts(port))
		b := balancer{container: name, through: throughProxy(t, scheme, port, roots, clientForHeaders, nil)}
		err := awaitForwarding(t, b.through, "/health", name)
		if err == nil {
			return b
		}
		if attempt == portRaceAttempts {
			t.Fatalf("%v, on %d ports in a row", err, attempt)
		}
		t.Logf("%v; starting it again on another port", err)
	}
}

// startDocumentedNginx serves the nginx balancer keyed key on page, in front of
// the instances on ports.
func startDocumentedNginx(t *testing.T, cert *publicCert, page, key string, ports []int) balancer {
	t.Helper()
	return startBalancer(t, "nginx", nginxImage, cert.pool, func(port int) []string {
		edits := append(backendEdits(ports), edit{old: "listen 443 ssl;", new: fmt.Sprintf("listen %s:%d ssl;", loopbackBind, port), count: 1})
		block := guideBlock(t, page, key, edits...)
		dir := containerDir(t)
		return []string{
			writeContainerFile(t, dir, "nginx.conf", nginxBalancerFrame) + ":/etc/nginx/nginx.conf:ro",
			writeContainerFile(t, dir, "mcp.conf", block) + ":/etc/nginx/conf.d/mcp.conf:ro",
			cert.letsEncryptMount(),
		}
	})
}

// startDocumentedHAProxy serves the load balancing page's HAProxy in front of
// the instances on ports. Its certificate line is served as printed: HAProxy
// reads one file holding the certificate and its key, and the case writes it
// where the line names it.
func startDocumentedHAProxy(t *testing.T, cert *publicCert, ports []int) balancer {
	t.Helper()
	return startBalancer(t, "haproxy", haproxyImage, cert.pool, func(port int) []string {
		edits := append(backendEdits(ports), edit{old: "bind *:443 ", new: fmt.Sprintf("bind %s:%d ", loopbackBind, port), count: 1})
		block := guideBlock(t, loadBalancingPage, "haproxy", edits...)
		dir := containerDir(t)
		pem := writeContainerFile(t, dir, "private/mcp.example.com.pem", cert.combinedPEM())
		return []string{
			writeContainerFile(t, dir, "haproxy.cfg", block+haproxyServedBy) + ":/usr/local/etc/haproxy/haproxy.cfg:ro",
			pem + ":/etc/ssl/private/mcp.example.com.pem:ro",
		}
	})
}

// get performs one GET through the balancer and returns its status together
// with the backend the balancer says served it.
func (b balancer) get(t *testing.T, path string, headers ...[2]string) (status int, servedBy string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, b.through.baseURL+path, http.NoBody)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	for _, h := range headers {
		req.Header.Set(h[0], h[1])
	}
	resp, err := b.through.httpClient().Do(req)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get("X-Served-By")
}

// haproxyForwardforConfig is a balancer that tells the server who its client
// was the way HAProxy does by default: option forwardfor appends an
// X-Forwarded-For field line of its own, after any line the client sent,
// rather than extending the client's line as nginx's
// proxy_add_x_forwarded_for does.
//
// It is the one configuration here no page prints, because what it isolates
// is the header, not a deployment: the load balancing page tells a reader to
// add option forwardfor to its backend, and this is that line with nothing
// else around it. It connects to the server from 127.0.0.2 so the proxy and
// the client are different addresses on one loopback: --trusted-proxies can
// then name the proxy without also vouching for the client, which is the
// deployment the header exists for.
const haproxyForwardforConfig = `global
  daemon
defaults
  mode http
  timeout connect 2s
  timeout client 30s
  timeout server 30s
  option http-server-close
  option forwardfor
frontend mcp
  bind 127.0.0.1:%d
  default_backend mcp_servers
backend mcp_servers
  server one 127.0.0.1:%d source 127.0.0.2
`

// TestBalancer_HAProxyForwardforChargesTheRealClient is the deployment the
// header was being misread in. A client that sends its own X-Forwarded-For
// through HAProxy arrives with two field lines: the client's, then the one
// HAProxy appended. Reading only the first line charged each failure to
// whatever the client wrote there, so a client that changed the value on every
// request was never blocked; read as one list from the right, the proxy's line
// names the client every time and the failure budget engages.
func TestBalancer_HAProxyForwardforChargesTheRealClient(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	upstream, _ := startServerOnFreePort(t,
		"--gitlab-url="+gitlab.url,
		"--trusted-proxy-header=X-Forwarded-For",
		"--trusted-proxies=127.0.0.2",
		"--auth-failure-limit=3",
	)
	through := startBalancer(t, "forwardfor", haproxyImage, nil, func(port int) []string {
		conf := writeContainerFile(t, containerDir(t), "haproxy.cfg", fmt.Sprintf(haproxyForwardforConfig, port, upstream))
		return []string{conf + ":/usr/local/etc/haproxy/haproxy.cfg:ro"}
	}).through

	for i := range 8 {
		got := through.do(t, mcpPOST(map[string]string{
			"PRIVATE-TOKEN":   fmt.Sprintf("glpat-forwardfor-%d", i),
			"X-Forwarded-For": fmt.Sprintf("198.51.100.%d", i+1),
		}))
		if got.status == http.StatusTooManyRequests {
			return
		}
		if got.status != http.StatusUnauthorized {
			t.Fatalf("request %d through HAProxy = %d, want 401 or 429: %s", i, got.status, truncate(got.body))
		}
	}
	t.Error("eight failures through HAProxy were never blocked with a limit of three: " +
		"each was charged to the address the client wrote in its own X-Forwarded-For line instead of the one HAProxy appended")
}

// declaredHost is the flag every instance behind a documented balancer is
// started with: the balancers forward the client's Host, mcp.example.com,
// and an instance that does not declare it refuses it.
const declaredHost = "--public-url=https://" + publicHost

// instances starts one server per documented backend, of one configuration,
// against one fake GitLab, and returns their ports.
func instances(t *testing.T) []int {
	t.Helper()
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	ports := make([]int, len(documentedBackends))
	for i := range ports {
		ports[i], _ = startServerOnFreePort(t, "--gitlab-url="+gitlab.url, declaredHost)
	}
	return ports
}

// TestBalancer_NginxConsistentHashPinsEachCredential proves the affinity the
// nginx balancers of both pages claim.
//
// Sharing the MCP server per configuration shape made affinity an
// optimization rather than a requirement for ordinary calls, but it is still
// what keeps one credential's pool entry, its licensing-tier probe and its
// rate-limit bucket in one process instead of in every process. The claim is
// worth nothing unhashed: a configuration that names a hash key nginx cannot
// resolve falls back to round robin silently, which looks identical to a
// working deployment until someone counts.
func TestBalancer_NginxConsistentHashPinsEachCredential(t *testing.T) {
	t.Parallel()
	cert := newPublicCert(t)
	for _, tc := range []struct{ page, key string }{
		{page: loadBalancingPage, key: "nginx"},
		{page: remoteDeploymentPage, key: "nginx-balancer"},
	} {
		t.Run(tc.page, func(t *testing.T) {
			t.Parallel()
			b := startDocumentedNginx(t, cert, tc.page, tc.key, instances(t))
			assertEachCredentialPins(t, b, func(token string) [2]string {
				return [2]string{"PRIVATE-TOKEN", token}
			})
			assertEachCredentialPins(t, b, func(token string) [2]string {
				return [2]string{"Authorization", "Bearer " + token}
			})
		})
	}
}

// TestBalancer_HAProxyHashesADigestOfTheCredential is the second balancer,
// and the one that never sees a raw credential in a routing key.
//
// The digest is computed in the balancer's own configuration, so nothing has
// to be trusted to keep the token out of the hash bucket, the stick table or
// a log line naming the variable. The assertion is the same as nginx's,
// because the property being checked is that the digest is a function of the
// credential: the same token must reach the same instance every time, and
// different tokens must not all reach one.
func TestBalancer_HAProxyHashesADigestOfTheCredential(t *testing.T) {
	t.Parallel()
	b := startDocumentedHAProxy(t, newPublicCert(t), instances(t))
	assertEachCredentialPins(t, b, func(token string) [2]string {
		return [2]string{"PRIVATE-TOKEN", token}
	})
	assertEachCredentialPins(t, b, func(token string) [2]string {
		return [2]string{"Authorization", "Bearer " + token}
	})
}

// drainDelayFlag reads the drain window the load balancing page recommends
// for its HAProxy configuration.
var drainDelayFlag = regexp.MustCompile("--drain-delay=([0-9]+s)")

// TestBalancer_HAProxyEjectsADrainingInstanceBeforeItCloses is what
// --drain-delay exists for, and it can only be shown with a balancer that
// polls.
//
// On SIGTERM the process answers /health with 503 and "draining" first, and
// keeps the listener open for --drain-delay before closing it. A balancer
// checking /health therefore has a window in which to take the instance out
// while it is still answering, so the requests in that window succeed
// somewhere else instead of failing on a closed socket. Without the window
// the listener closes first and the balancer learns about it from a failed
// request, which is one failed request per client.
//
// The page's configuration checks every two seconds and marks a server down
// after two failures, and the page says the window that leaves is comfortable
// is the --drain-delay it names; the instance drains with that one, read from
// the page. The assertion is on the outcome rather than on the balancer's
// internal state: every credential keeps getting served, including the ones
// the hash had pinned to the instance that is going away.
func TestBalancer_HAProxyEjectsADrainingInstanceBeforeItCloses(t *testing.T) {
	t.Parallel()
	requireDocker(t)
	recommended := drainDelayFlag.FindStringSubmatch(guideProse(t, loadBalancingPage, "is comfortable"))
	if recommended == nil {
		t.Fatal("the load balancing page no longer names the --drain-delay its HAProxy configuration is comfortable with")
	}
	drainDelay, err := time.ParseDuration(recommended[1])
	if err != nil {
		t.Fatalf("the page's --drain-delay=%s: %v", recommended[1], err)
	}

	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	draining := shutdownStartServer(t,
		"--gitlab-url="+gitlab.url,
		declaredHost,
		"--drain-delay="+drainDelay.String(),
	)
	ports := []int{hostPortOf(t, draining.probe.baseURL), 0, 0}
	for i := range ports[1:] {
		ports[i+1], _ = startServerOnFreePort(t, "--gitlab-url="+gitlab.url, declaredHost)
	}
	b := startDocumentedHAProxy(t, newPublicCert(t), ports)

	// Which credentials the hash sends to the instance about to drain. They
	// are the ones whose service has to survive, and knowing them is what
	// makes this an assertion about the drain rather than about luck.
	target := fmt.Sprintf("127.0.0.1:%d", ports[0])
	var pinned []string
	for _, token := range affinityTokens {
		if _, servedBy := b.get(t, "/health", [2]string{"PRIVATE-TOKEN", token}); servedBy == target {
			pinned = append(pinned, token)
		}
	}
	// Every token on the other instances is the same event the affinity
	// assertions fail on, at smaller odds still, so it fails here too rather
	// than skipping: a skip would let a changed ring or token set retire the
	// drain assertion in silence.
	if len(pinned) == 0 {
		t.Fatalf("the hash sent none of the %d credentials to the instance under test; the ring or the token set no longer splits", len(affinityTokens))
	}

	if err = signalTermination(draining.cmd.Process); err != nil {
		t.Fatalf("signaling the draining instance: %v", err)
	}

	// Inside the drain window the balancer must have moved them, while the
	// instance is still listening. The page's checks need two failures two
	// seconds apart, so they land well inside the window it recommends.
	deadline := time.Now().Add(drainDelay - time.Second)
	for time.Now().Before(deadline) {
		moved := true
		for _, token := range pinned {
			status, servedBy := b.get(t, "/health", [2]string{"PRIVATE-TOKEN", token})
			if status != http.StatusOK || servedBy == target {
				moved = false
				break
			}
		}
		if moved {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Errorf("the balancer was still routing to %s at the end of the drain window the page recommends (%v); "+
		"the instance answers /health with 503 while draining, so a polling balancer must eject it before the listener closes", target, drainDelay)
}

// TestBalancer_InstancesBehindOneBalancerAgreeOnWhatTheyServe is the fleet
// check the guide tells an operator to run.
//
// config_digest covers the settings that decide which tools a client is shown.
// Two instances reporting different digests serve different catalogs to
// whichever clients reach them, and nothing else detects it: the calls all
// succeed, and only the set of available actions differs. This asserts both
// directions, because a digest that never differs would pass the first half
// while detecting nothing.
func TestBalancer_InstancesBehindOneBalancerAgreeOnWhatTheyServe(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	matching := []*server{
		startServer(t, nil, "--gitlab-url="+gitlab.url, "--tool-surface=meta"),
		startServer(t, nil, "--gitlab-url="+gitlab.url, "--tool-surface=meta"),
	}
	odd := startServer(t, nil, "--gitlab-url="+gitlab.url, "--tool-surface=meta", "--read-only")

	first := configDigestOf(t, matching[0])
	if second := configDigestOf(t, matching[1]); second != first {
		t.Errorf("two instances of one configuration report %q and %q; a fleet cannot be compared with a digest that is not a function of the configuration alone", first, second)
	}
	if different := configDigestOf(t, odd); different == first {
		t.Errorf("an instance serving a read-only catalog reports the same digest %q as one serving the full catalog; the comparison would not detect the misconfiguration it exists for", different)
	}
}

// configDigestOf reads one instance's /health and returns its config digest.
func configDigestOf(t *testing.T, srv *server) string {
	t.Helper()
	got := srv.do(t, request{method: http.MethodGet, path: "/health"})
	if got.status != http.StatusOK {
		t.Fatalf("/health = %d: %s", got.status, got.body)
	}
	var health struct {
		ConfigDigest string `json:"config_digest"`
	}
	if err := json.Unmarshal([]byte(got.body), &health); err != nil {
		t.Fatalf("decoding /health %q: %v", truncate(got.body), err)
	}
	if health.ConfigDigest == "" {
		t.Fatal("/health carries no config_digest")
	}
	return health.ConfigDigest
}

// assertEachCredentialPins drives every token through the balancer several
// times and checks both halves of affinity.
func assertEachCredentialPins(t *testing.T, b balancer, header func(token string) [2]string) {
	t.Helper()

	// Four, because round robin over several backends shows itself on the
	// second request and the count multiplies by every token: six across
	// twenty-four tokens would spend the time the wider set was meant to buy.
	const requestsPerToken = 4
	backends := make(map[string]string, len(affinityTokens))
	for _, token := range affinityTokens {
		seen := map[string]struct{}{}
		for range requestsPerToken {
			status, servedBy := b.get(t, "/health", header(token))
			if status != http.StatusOK {
				t.Fatalf("token %s: /health through the balancer = %d", token, status)
			}
			if servedBy == "" {
				t.Fatalf("token %s: the balancer reported no backend; the configuration under test is not the one being asserted about", token)
			}
			seen[servedBy] = struct{}{}
		}
		if len(seen) != 1 {
			t.Errorf("token %s reached %d backends across %d requests, want 1: the hash key is not resolving and the balancer has fallen back to round robin",
				token, len(seen), requestsPerToken)
		}
		for backend := range seen {
			backends[token] = backend
		}
	}

	distinct := map[string]struct{}{}
	for _, backend := range backends {
		distinct[backend] = struct{}{}
	}
	if len(distinct) < 2 {
		t.Errorf("all %d credentials landed on one backend (%v); the hash is pinning but not distributing", len(affinityTokens), distinct)
	}
}

// hostPortOf returns the port a harness base URL listens on.
func hostPortOf(t *testing.T, baseURL string) int {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parsing %q: %v", baseURL, err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("reading the port out of %q: %v", baseURL, err)
	}
	return port
}
