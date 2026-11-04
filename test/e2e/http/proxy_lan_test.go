// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// proxy_lan_test.go serves the remote deployment guide's LAN block, nginx in
// plain HTTP in front of a server on loopback, together with the server
// command the guide prints beside it, and holds the pair to the rows of the
// requirement table a plain-HTTP deployment can answer.
//
// Two rows are not among them. The OAuth metadata is not served on this shape
// at all, since OAuth mode takes an https --public-url. And the server's own
// listener off loopback in plain HTTP, the shape the guide's last paragraph
// warns about, is held by TestListener_PlaintextOffLoopback_* in
// listener_test.go, which this case does not repeat.
package httpe2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// lanHost is the name the guide's LAN block serves.
const lanHost = "mcp.lan"

// guideServerFlags reads the server command printed under key and returns the
// flags the case starts the server with.
//
// The command's own start, the binary in HTTP mode on loopback, is what the
// harness does itself on a port of its choosing, and the instance it names is
// replaced by the case's stand-in; every other flag is the page's.
func guideServerFlags(t *testing.T, page, key, gitlabURL string) []string {
	t.Helper()
	folded := strings.Join(strings.Fields(strings.ReplaceAll(guideBlock(t, page, key), "\\\n", " ")), " ")
	command, err := applyEdits(folded, []edit{
		{old: "gitlab-mcp-server --http --http-addr=127.0.0.1:8080 ", new: "", count: 1},
		{old: "--gitlab-url=https://gitlab.example.com", new: "--gitlab-url=" + gitlabURL, count: 1},
	})
	if err != nil {
		t.Fatalf("%s, the command keyed e2e=%s: %v; the page changed what the case replaces, so the case has to change with it", page, key, err)
	}
	return strings.Fields(command)
}

// guideLANNginx is the guide's LAN block, listening where the case's client
// reaches it rather than on a LAN address this machine may not have.
func guideLANNginx() documentedProxy {
	return documentedProxy{
		name:         "nginx-lan",
		host:         lanHost,
		clientHeader: "X-Real-IP",
		start: func(t *testing.T, upstream int) (string, int) {
			t.Helper()
			port := freePort(t)
			block := guideBlock(t, remoteDeploymentPage, "nginx-lan",
				edit{old: "listen 192.168.1.10:80;", new: fmt.Sprintf("listen 127.0.0.1:%d;", port), count: 1},
				edit{old: "server 127.0.0.1:8080;", new: fmt.Sprintf("server 127.0.0.1:%d;", upstream), count: 1},
			)
			dir := containerDir(t)
			name := fmt.Sprintf("gitlab-mcp-httpe2e-guide-nginx-lan-%d", port)
			startContainer(t, name, nginxImage, []string{
				writeContainerFile(t, dir, "nginx.conf", nginxFrame) + ":/etc/nginx/nginx.conf:ro",
				writeContainerFile(t, dir, "mcp.conf", block) + ":/etc/nginx/conf.d/mcp.conf:ro",
			})
			return name, port
		},
	}
}

// TestProxyGuide_NginxLAN serves the guide's LAN block and its server command,
// and holds them to what the guide says of them: the trusted hop admits the
// LAN name that --public-url cannot declare, the stream and the credentials
// cross the proxy, and each LAN client is charged its own failures.
func TestProxyGuide_NginxLAN(t *testing.T) {
	t.Parallel()
	lan := guideLANNginx()

	t.Run("without the trusted hop the LAN name is refused", func(t *testing.T) {
		t.Parallel()
		gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
		refused := lan.behind(t, "--gitlab-url="+gitlab.url).from(t, clientForHeaders)
		got := refused.do(t, request{method: http.MethodGet, path: "/health"})
		if got.status != http.StatusForbidden {
			t.Errorf("GET /health for %s through a server that trusts no hop = %d, want 403: "+
				"the proxy forwarded a Host the server serves anyway, so the guide's pairing would be doing nothing: %s", lanHost, got.status, truncate(got.body))
		}
	})

	t.Run("the guide's command", func(t *testing.T) {
		t.Parallel()
		gitlab := startTokenGitLab(t, acceptedPrivateToken, acceptedBearerToken)
		flags := guideServerFlags(t, remoteDeploymentPage, "nginx-lan-server", gitlab.url)
		paired := lan.behind(t, flags...)
		through := paired.from(t, clientForHeaders)

		t.Run("the trusted hop admits the LAN name", func(t *testing.T) {
			if got := through.do(t, request{method: http.MethodGet, path: "/health"}); got.status != http.StatusOK {
				t.Errorf("GET /health for %s with the guide's command = %d, want 200: %s", lanHost, got.status, truncate(got.body))
			}
		})

		t.Run("a stream is delivered as it is written", func(t *testing.T) {
			assertStreamIsNotHeld(t, through, map[string]string{"PRIVATE-TOKEN": acceptedPrivateToken})
		})

		for _, header := range []struct{ name, value, token string }{
			{"PRIVATE-TOKEN", acceptedPrivateToken, acceptedPrivateToken},
			{"Authorization", "Bearer " + acceptedBearerToken, acceptedBearerToken},
		} {
			t.Run(header.name+" reaches the server", func(t *testing.T) {
				got := through.do(t, mcpPOST(map[string]string{header.name: header.value}))
				if got.status != http.StatusOK || gitlab.asked(header.token) == 0 {
					t.Errorf("tools/list with %s through the LAN proxy = %d with GitLab asked %d time(s), want 200 and the token checked: %s",
						header.name, got.status, gitlab.asked(header.token), truncate(got.body))
				}
			})
		}

		// The command sets no --auth-failure-limit, so the default of ten
		// failures a minute applies, and the client gets more attempts.
		assertFailuresChargeTheClient(t, paired, nil, 15)
	})
}
