// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// proxy_caddy_test.go serves the remote deployment guide's Caddyfile and
// holds it to the guide's requirement table (guide_test.go).
package httpe2e

import (
	"fmt"
	"net/http"
	"testing"
)

// caddyImage is the Caddy case's image, pinned to a minor version.
const caddyImage = "caddy:2.11-alpine"

// caddyGlobal is the Caddyfile the guide's site block is imported from: the
// global options a test needs, and nothing inside the site block.
//
// The guide's site address has Caddy obtain a certificate from Let's
// Encrypt, which a test cannot do, so local_certs has Caddy's internal CA
// issue it instead, from a root the case generates and its clients trust.
// The ports are the case's, since every container here shares the host's
// network: Caddy's HTTPS port, the HTTP port it redirects from (80 is not
// the case's to take), and no admin endpoint, whose default port two Caddy
// containers would fight over.
const caddyGlobal = `{
	admin off
	https_port %d
	http_port %d
	local_certs
	skip_install_trust
	pki {
		ca local {
			root {
				cert /e2e/pki/ca.crt
				key /e2e/pki/ca.key
			}
		}
	}
}
import /etc/caddy/site.caddy
`

// guideCaddy is the guide's Caddyfile, served with the upstream a case
// chooses and any further edits a control makes to it.
func guideCaddy(cert *publicCert, extra ...edit) documentedProxy {
	return documentedProxy{
		name:         "caddy",
		roots:        cert.pool,
		clientHeader: "X-Forwarded-For",
		start: func(t *testing.T, upstream int) (string, int) {
			t.Helper()
			port, redirect := freePort(t), freePort(t)
			edits := append([]edit{
				{old: "127.0.0.1:8080", new: fmt.Sprintf("127.0.0.1:%d", upstream), count: 1},
			}, extra...)
			block := guideBlock(t, remoteDeploymentPage, "caddy", edits...)

			dir := containerDir(t)
			name := fmt.Sprintf("gitlab-mcp-httpe2e-guide-caddy-%d", port)
			startContainer(t, name, caddyImage, []string{
				writeContainerFile(t, dir, "Caddyfile", fmt.Sprintf(caddyGlobal, port, redirect)) + ":/etc/caddy/Caddyfile:ro",
				writeContainerFile(t, dir, "site.caddy", block) + ":/etc/caddy/site.caddy:ro",
				cert.dir + ":/e2e/pki:ro",
			})
			return name, port
		},
	}
}

// TestProxyGuide_Caddy serves the guide's Caddyfile and holds it to the
// requirement table, with the X-Forwarded-For pairing the guide names for
// it, and checks the one claim its prose makes about a line of the block:
// that Caddy flushes text/event-stream by itself, so flush_interval -1 only
// keeps that from depending on the upstream's content type.
func TestProxyGuide_Caddy(t *testing.T) {
	t.Parallel()
	cert := newPublicCert(t)

	assertProxyRequirements(t, guideCaddy(cert))

	t.Run("without flush_interval the stream still flows", func(t *testing.T) {
		t.Parallel()
		gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
		unflushed := guideCaddy(cert, edit{old: "        flush_interval -1\n", new: "", count: 1})
		through := unflushed.behind(t, "--gitlab-url="+gitlab.url, "--public-url=https://"+publicHost).from(t, clientForHeaders)
		assertStreamIsNotHeld(t, through, map[string]string{"PRIVATE-TOKEN": "glpat-unflushed"})
	})
}
