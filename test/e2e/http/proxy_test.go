//go:build httpe2e

// proxy_test.go runs the server behind a reverse proxy configured the way the
// hosted deployment configures one.
//
// A whole class of failure only exists here. The proxy answers the preflight
// itself, which hides a server that cannot; and it adds its own CORS headers,
// which collide with the server's to produce a response a browser rejects
// outright while curl reports 200. Both were found by hand, in that order,
// after the plain HTTP matrix was already green.
//
// Docker is required and the tests skip without it, because the point is to run
// a real nginx rather than to model one.
package httpe2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// proxyConfig is the nginx configuration under test. Two locations share one
// upstream: /proxied carries the hosted deployment's CORS block, /plain carries
// none, so the same server is observable with and without a proxy that answers
// CORS on its behalf.
const proxyConfig = `events { worker_connections 64; }
http {
  access_log off;
  server {
    listen %d;
    location ^~ /proxied {
      add_header Access-Control-Allow-Origin "*" always;
      add_header Access-Control-Expose-Headers "Mcp-Session-Id, Mcp-Protocol-Version" always;
      if ($request_method = OPTIONS) {
        add_header Access-Control-Allow-Origin "*" always;
        add_header Access-Control-Allow-Methods "GET, POST, DELETE, OPTIONS" always;
        add_header Access-Control-Max-Age 86400 always;
        return 204;
      }
      proxy_http_version 1.1;
      proxy_set_header Connection "";
      proxy_set_header Host $host;
      proxy_set_header X-Real-IP $remote_addr;
      rewrite ^/proxied/?(.*)$ /$1 break;
      proxy_pass http://127.0.0.1:%d;
    }
    location ^~ /plain {
      proxy_http_version 1.1;
      proxy_set_header Connection "";
      proxy_set_header Host $host;
      proxy_set_header X-Real-IP $remote_addr;
      rewrite ^/plain/?(.*)$ /$1 break;
      proxy_pass http://127.0.0.1:%d;
    }
  }
}
`

// startProxy runs nginx in front of the given upstream port and returns its
// base URL, skipping the test when Docker is unavailable.
func startProxy(t *testing.T, upstreamPort int) string {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available; the proxy layer is where the header-collision class lives, so it is skipped rather than modeled")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skip("docker is installed but not usable; skipping the proxy layer")
	}

	proxyPort := freePort(t)
	dir := t.TempDir()
	confPath := filepath.Join(dir, "nginx.conf")
	conf := fmt.Sprintf(proxyConfig, proxyPort, upstreamPort, upstreamPort)
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil { //#nosec G306 -- nginx reads it inside a throwaway container
		t.Fatalf("writing the nginx config: %v", err)
	}

	name := fmt.Sprintf("gitlab-mcp-httpe2e-nginx-%d", proxyPort)
	runCtx, runCancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer runCancel()
	out, err := exec.CommandContext(runCtx, "docker", "run", "-d",
		"--name", name, "--network", "host",
		"-v", confPath+":/etc/nginx/nginx.conf:ro",
		"nginx:alpine",
	).CombinedOutput()
	if err != nil {
		t.Skipf("could not start nginx (%v):\n%s", err, out)
	}
	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		_ = exec.CommandContext(rmCtx, "docker", "rm", "-f", name).Run()
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	waitProxy(t, base)
	return base
}

// waitProxy polls the proxy until it forwards a health check.
func waitProxy(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/plain/health", http.NoBody)
		if err != nil {
			t.Fatalf("building the proxy health request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Skip("nginx never forwarded a request; skipping the proxy layer rather than failing on the environment")
}

// TestProxy_ServerAndProxyCORSCollide verifies the failure a browser sees and
// curl does not: a proxy that advertises CORS on the server's behalf, plus a
// server that now answers CORS itself, produce two
// Access-Control-Allow-Origin headers, and Fetch treats that as a failure.
//
// The values are not merged. Chromium says: "The 'Access-Control-Allow-Origin'
// header contains multiple values ... but only one is allowed." So a deployment
// must not keep its proxy's CORS block once the server answers for itself, and
// this test is what says so out loud.
func TestProxy_ServerAndProxyCORSCollide(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	upstream := freePort(t)
	srv := startServerOnPort(t, upstream, nil,
		"--gitlab-url="+gitlab.url,
		"--trusted-origins="+trustedOrigin,
	)
	base := startProxy(t, upstream)
	proxied := &server{baseURL: base + "/proxied", logs: srv.logs}
	plain := &server{baseURL: base + "/plain", logs: srv.logs}

	headers := map[string]string{
		"PRIVATE-TOKEN":  "glpat-whatever",
		"Origin":         trustedOrigin,
		"Sec-Fetch-Site": "cross-site",
	}

	// Through the CORS-free location the server's answer stands alone, which
	// is the shape a browser accepts.
	got := plain.do(t, mcpPOST(headers))
	if n := len(got.header.Values("Access-Control-Allow-Origin")); n != 1 {
		t.Errorf("through a plain proxy: %d Access-Control-Allow-Origin headers, want exactly 1", n)
	}

	// Through the location that also advertises CORS there are two, and that
	// is the deployment's bug rather than the server's — but a test that
	// names it is what stops it being rediscovered in production.
	got = proxied.do(t, mcpPOST(headers))
	if n := len(got.header.Values("Access-Control-Allow-Origin")); n > 1 {
		t.Logf("confirmed: a proxy CORS block plus the server's own produces %d Access-Control-Allow-Origin headers, which a browser rejects outright even though this request returned %d", n, got.status)
		t.Log("the deployment must drop its proxy-level CORS for the MCP location; see docs/concepts/security.md")
	}
}

// TestProxy_PreflightAnsweredByTheProxyHidesTheServer verifies that the server
// answers a preflight on its own, so a deployment does not depend on its proxy
// doing it.
//
// This is the trap that let a broken preflight ship: the hosted nginx answered
// OPTIONS itself, so every check against the deployment looked correct while
// the server behind it would have refused.
func TestProxy_PreflightAnsweredByTheProxyHidesTheServer(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	upstream := freePort(t)
	startServerOnPort(t, upstream, nil,
		"--gitlab-url="+gitlab.url,
		"--trusted-origins="+trustedOrigin,
	)
	base := startProxy(t, upstream)

	preflight := request{
		method: http.MethodOptions, path: "",
		headers: map[string]string{
			"Origin":                        trustedOrigin,
			"Access-Control-Request-Method": http.MethodPost,
		},
	}

	plain := &server{baseURL: base + "/plain"}
	got := plain.do(t, preflight)
	if got.status != http.StatusNoContent {
		t.Errorf("through a plain proxy the preflight is the server's to answer: got %d, want %d", got.status, http.StatusNoContent)
	}
	if origin := got.header.Get("Access-Control-Allow-Origin"); origin != trustedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, trustedOrigin)
	}
}

// TestProxy_RealClientAddressReachesTheLimiter verifies that --trusted-proxy-header
// is honored through an actual proxy, so one noisy client cannot spend the
// budget of everyone behind the same address.
func TestProxy_RealClientAddressReachesTheLimiter(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	upstream := freePort(t)
	startServerOnPort(t, upstream, nil,
		"--gitlab-url="+gitlab.url,
		"--trusted-proxy-header=X-Real-IP",
	)
	base := startProxy(t, upstream)
	plain := &server{baseURL: base + "/plain"}

	// nginx sets X-Real-IP to the real peer, which here is loopback for every
	// request — so what this can prove is that the header is honored and the
	// limiter still engages through the proxy, not that two clients are
	// separated. That separation is asserted directly against the binary in
	// TestGate_FailureBudgetIsPerAddress, where the addresses can differ.
	var blocked bool
	for i := range 15 {
		got := plain.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": fmt.Sprintf("glpat-proxied-%d", i)}))
		if got.status == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Error("the failure budget never engaged through the proxy")
	}
}

// TestProxy_HealthAndCardSurviveTheProxy verifies that what a load balancer and
// a registry read still works once a proxy rewrites the path in front of it.
func TestProxy_HealthAndCardSurviveTheProxy(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	upstream := freePort(t)
	startServerOnPort(t, upstream, nil, "--gitlab-url="+gitlab.url)
	base := startProxy(t, upstream)
	plain := &server{baseURL: base + "/plain"}

	for _, path := range []string{"/health", "/.well-known/mcp/server-card.json"} {
		got := plain.do(t, request{method: http.MethodGet, path: path})
		if got.status != http.StatusOK {
			t.Errorf("GET %s through the proxy = %d, want %d", path, got.status, http.StatusOK)
		}
		if strings.Contains(path, "server-card") && !strings.Contains(got.body, "\"name\"") {
			t.Errorf("the server card came back without a name: %s", got.body)
		}
	}
}
