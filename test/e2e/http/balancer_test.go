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
// Docker is required and both skip without it, because the point is to run
// the real balancer rather than to model one.
package httpe2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// balancerSalt stands in for the per-deployment secret the documented
// configurations hash the credential with.
//
// It is in the configuration and never in a log or a routing key, which is the
// property being demonstrated: the balancer routes on a digest that is
// meaningless outside this deployment, not on the credential itself.
const balancerSalt = "e2e-only-affinity-salt"

// affinityTokens are the credentials the affinity assertions are made with.
//
// Eight, because the assertion has two halves: every token must stay on one
// instance, and the eight together must not all land on the same one. With
// two backends a smaller set could satisfy the first half while failing the
// second by chance often enough to make the test flaky.
var affinityTokens = []string{
	"glpat-alpha", "glpat-bravo", "glpat-charlie", "glpat-delta",
	"glpat-echo", "glpat-foxtrot", "glpat-golf", "glpat-hotel",
}

// nginxBalancerConfig is the balancer from the enterprise deployment guide,
// with one addition: the backend that served each response is echoed in a
// header so the test can read the routing decision it is asserting about.
//
// The map chain is the documented one. The credential is normalized out of
// either header, an absent credential falls back to the client address so
// anonymous requests do not all collapse onto one instance, and the salt is
// concatenated into the hash key rather than becoming a variable of its own,
// which keeps it out of any log format that names variables.
const nginxBalancerConfig = `events { worker_connections 64; }
http {
  access_log off;

  map $host $mcp_salt {
    default "%s";
  }
  map $http_authorization $mcp_bearer {
    default                     "";
    "~*^Bearer[ ]+(?<tok>\S+)$" $tok;
  }
  map $mcp_bearer $mcp_credential {
    default $mcp_bearer;
    ""      $http_private_token;
  }
  map $mcp_credential $mcp_affinity {
    default $mcp_credential;
    ""      $remote_addr;
  }

  upstream gitlab_mcp {
    hash "$mcp_salt$mcp_affinity" consistent;
    server 127.0.0.1:%d max_fails=2 fail_timeout=10s;
    server 127.0.0.1:%d max_fails=2 fail_timeout=10s;
    keepalive 32;
  }

  server {
    listen %d;
    location / {
      proxy_pass http://gitlab_mcp;
      proxy_http_version 1.1;
      proxy_set_header Connection "";
      proxy_set_header Host              $host;
      proxy_set_header X-Real-IP         $remote_addr;
      proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
      proxy_buffering off;
      proxy_request_buffering off;
      proxy_read_timeout 1h;
      proxy_next_upstream error timeout;
      proxy_next_upstream_tries 2;
      add_header X-Served-By $upstream_addr always;
    }
  }
}
`

// haproxyBalancerConfig is the second balancer, the one that polls.
//
// Three things it does that nginx open source cannot. It hashes a digest of
// the credential rather than the credential, computed in the configuration
// through converters, so nothing has to be trusted to keep the raw value out
// of a routing key. It checks /health actively, which is what makes a drain
// window mean anything. And it refuses to retry a request that was delivered:
// "option redispatch" is absent and retries are left at connection failures,
// so a tools/call that created something is never replayed on a second
// instance.
const haproxyBalancerConfig = `global
  daemon
defaults
  mode http
  timeout connect 2s
  timeout client 1h
  timeout server 1h
  retries 1
  option http-server-close
frontend mcp
  bind *:%d
  http-request set-var(txn.cred) req.hdr(PRIVATE-TOKEN)
  http-request set-var(txn.cred) req.hdr(Authorization),regsub(^[Bb]earer\ +,) if !{ req.hdr(PRIVATE-TOKEN) -m found }
  http-request set-var(txn.cred) src,ipmask(32,128) if !{ var(txn.cred) -m found }
  default_backend mcp_servers
backend mcp_servers
  balance hash var(txn.cred),concat(%s),sha1,hex
  hash-type consistent
  option httpchk
  http-check send meth GET uri /health
  http-check expect status 200
  http-response set-header X-Served-By %%si:%%sp
  server one 127.0.0.1:%d check inter 500ms fall 2 rise 2
  server two 127.0.0.1:%d check inter 500ms fall 2 rise 2
`

// startBalancer runs a container with the given configuration mounted where
// the image expects it, and returns the base URL of the port it listens on.
//
// It skips rather than fails on every environmental problem, the way
// startProxy does: a missing Docker, an image that cannot be pulled and a
// container that never answers are all facts about the machine, not about the
// server under test.
func startBalancer(t *testing.T, image, confPath, confMountPath string, port int, args ...string) string {
	t.Helper()
	requireDocker(t)

	name := fmt.Sprintf("gitlab-mcp-httpe2e-lb-%d", port)
	runArgs := []string{
		"run", "-d", "--name", name, "--network", "host",
		"-v", confPath + ":" + confMountPath + ":ro",
		image,
	}
	runArgs = append(runArgs, args...)
	runCtx, runCancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer runCancel()
	out, err := exec.CommandContext(runCtx, "docker", runArgs...).CombinedOutput()
	if err != nil {
		t.Skipf("could not start %s (%v):\n%s", image, err, out)
	}
	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		_ = exec.CommandContext(rmCtx, "docker", "rm", "-f", name).Run()
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitBalancer(t, base, name)
	return base
}

// requireDocker skips the calling test unless Docker is present and usable.
func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available; a balancer is run rather than modeled, so this is skipped")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skip("docker is installed but not usable; skipping the balancer layer")
	}
}

// waitBalancer polls the balancer until it forwards a health check.
func waitBalancer(t *testing.T, base, container string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if status, _ := getThrough(t, base+"/health"); status == http.StatusOK {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	logCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	logs, _ := exec.CommandContext(logCtx, "docker", "logs", container).CombinedOutput()
	t.Skipf("the balancer never forwarded a request; skipping rather than failing on the environment:\n%s", logs)
}

// getThrough performs one GET and returns its status together with the
// backend the balancer says served it.
func getThrough(t *testing.T, url string, headers ...[2]string) (status int, servedBy string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	for _, h := range headers {
		req.Header.Set(h[0], h[1])
	}
	// A client per call: a kept-alive connection is already pinned to a
	// backend, which would make the affinity assertion pass without the
	// balancer ever hashing anything.
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get("X-Served-By")
}

// twoInstances starts two servers of one configuration against one fake
// GitLab and returns their ports.
func twoInstances(t *testing.T, flags ...string) (first, second int) {
	t.Helper()
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	first, second = freePort(t), freePort(t)
	for _, port := range []int{first, second} {
		startServerOnPort(t, port, nil, append([]string{"--gitlab-url=" + gitlab.url}, flags...)...)
	}
	return first, second
}

// TestBalancer_NginxConsistentHashPinsEachCredential proves the affinity the
// enterprise guide's nginx balancer claims.
//
// Sharing the MCP server per configuration shape made affinity an
// optimization rather than a requirement for ordinary calls, but it is still
// what keeps one credential's pool entry, its licensing-tier probe and its
// rate-limit bucket in one process instead of in every process. The claim is
// worth nothing unhashed: a configuration that names a hash key nginx cannot
// resolve falls back to round robin silently, which looks identical to a
// working deployment until someone counts.
func TestBalancer_NginxConsistentHashPinsEachCredential(t *testing.T) {
	first, second := twoInstances(t)
	port := freePort(t)
	conf := writeBalancerConfig(t, "nginx.conf",
		fmt.Sprintf(nginxBalancerConfig, balancerSalt, first, second, port))
	base := startBalancer(t, "nginx:alpine", conf, "/etc/nginx/nginx.conf", port)

	assertEachCredentialPins(t, base, func(token string) [2]string {
		return [2]string{"PRIVATE-TOKEN", token}
	})
	assertEachCredentialPins(t, base, func(token string) [2]string {
		return [2]string{"Authorization", "Bearer " + token}
	})
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
	first, second := twoInstances(t)
	port := freePort(t)
	conf := writeBalancerConfig(t, "haproxy.cfg",
		fmt.Sprintf(haproxyBalancerConfig, port, balancerSalt, first, second))
	base := startBalancer(t, "haproxy:3.0-alpine", conf, "/usr/local/etc/haproxy/haproxy.cfg", port)

	assertEachCredentialPins(t, base, func(token string) [2]string {
		return [2]string{"PRIVATE-TOKEN", token}
	})
}

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
// The assertion is on the outcome rather than on the balancer's internal
// state: every credential keeps getting served, including the ones the hash
// had pinned to the instance that is going away.
func TestBalancer_HAProxyEjectsADrainingInstanceBeforeItCloses(t *testing.T) {
	requireDocker(t)
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	drainDelay := 6 * time.Second
	draining := shutdownStartServer(t,
		"--gitlab-url="+gitlab.url,
		"--drain-delay="+drainDelay.String(),
	)
	first := hostPortOf(t, draining.probe.baseURL)
	second := freePort(t)
	startServerOnPort(t, second, nil, "--gitlab-url="+gitlab.url)

	port := freePort(t)
	conf := writeBalancerConfig(t, "haproxy.cfg",
		fmt.Sprintf(haproxyBalancerConfig, port, balancerSalt, first, second))
	base := startBalancer(t, "haproxy:3.0-alpine", conf, "/usr/local/etc/haproxy/haproxy.cfg", port)

	// Which credentials the hash sends to the instance about to drain. They
	// are the ones whose service has to survive, and knowing them is what
	// makes this an assertion about the drain rather than about luck.
	target := fmt.Sprintf("127.0.0.1:%d", first)
	var pinned []string
	for _, token := range affinityTokens {
		if _, servedBy := getThrough(t, base+"/health", [2]string{"PRIVATE-TOKEN", token}); servedBy == target {
			pinned = append(pinned, token)
		}
	}
	if len(pinned) == 0 {
		t.Skip("the hash sent no credential to the instance under test; nothing to observe")
	}

	if err := signalTermination(draining.cmd.Process); err != nil {
		t.Fatalf("signaling the draining instance: %v", err)
	}

	// Inside the drain window the balancer must have moved them, while the
	// instance is still listening. Two check intervals plus a margin: the
	// health check runs every 500ms and marks down after two failures.
	deadline := time.Now().Add(drainDelay - time.Second)
	for time.Now().Before(deadline) {
		moved := true
		for _, token := range pinned {
			status, servedBy := getThrough(t, base+"/health", [2]string{"PRIVATE-TOKEN", token})
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
	t.Errorf("the balancer was still routing to %s at the end of the drain window; "+
		"the instance answers /health with 503 while draining, so a polling balancer must eject it before the listener closes", target)
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
func assertEachCredentialPins(t *testing.T, base string, header func(token string) [2]string) {
	t.Helper()

	const requestsPerToken = 6
	backends := make(map[string]string, len(affinityTokens))
	for _, token := range affinityTokens {
		seen := map[string]struct{}{}
		for range requestsPerToken {
			status, servedBy := getThrough(t, base+"/health", header(token))
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

// writeBalancerConfig writes a balancer configuration into a directory the
// container can bind-mount.
func writeBalancerConfig(t *testing.T, name, body string) string {
	t.Helper()
	// Not t.TempDir: the container runs as another user and the per-test
	// directory is created 0700, so the bind mount would be unreadable.
	dir, err := os.MkdirTemp("", "mcp-lb") //nolint:usetesting // see above
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if chmodErr := os.Chmod(dir, 0o755); chmodErr != nil { //#nosec G302 -- a throwaway directory a container must read
		t.Fatalf("opening the config directory: %v", chmodErr)
	}
	path := filepath.Join(dir, name)
	if writeErr := os.WriteFile(path, []byte(body), 0o644); writeErr != nil { //#nosec G306 -- read by a container as another user
		t.Fatalf("writing %s: %v", name, writeErr)
	}
	return path
}
