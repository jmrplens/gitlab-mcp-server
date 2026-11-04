// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// proxy_apache_test.go serves the remote deployment guide's Apache
// VirtualHost and holds it to the guide's requirement table (guide_test.go).
package httpe2e

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// httpdImage is the Apache case's image, pinned to a minor version.
const httpdImage = "httpd:2.4-alpine"

// apacheFrame is the httpd.conf the guide's VirtualHost is included from: the
// listener, the process settings, the three modules every stock httpd.conf
// loads (an MPM, unixd for the user it runs as, and authz_core, without which
// httpd refuses every request it was not told how to authorize with AH00025),
// and then the modules the guide's own sentence names, loaded from what the
// sentence says.
const apacheFrame = `ServerRoot "/usr/local/apache2"
ServerName localhost
Listen %d
PidFile /tmp/httpd.pid
ErrorLog /proc/self/fd/2
LogLevel warn
User daemon
Group daemon
LoadModule mpm_event_module modules/mod_mpm_event.so
LoadModule unixd_module modules/mod_unixd.so
LoadModule authz_core_module modules/mod_authz_core.so
%sInclude conf/mcp.conf
`

// apacheModulesSentence is how the guide starts the sentence that lists the
// modules its VirtualHost needs.
const apacheModulesSentence = "Modules required"

// moduleName is one module named in that sentence.
var moduleName = regexp.MustCompile("`([a-z_]+)`")

// guideApacheModules returns the LoadModule lines for the modules the guide's
// sentence names, so a module the block needs and the sentence drops fails
// the case rather than being loaded by the case on its own.
func guideApacheModules(t *testing.T) string {
	t.Helper()
	sentence := guideProse(t, remoteDeploymentPage, apacheModulesSentence)
	_, list, _ := strings.Cut(sentence, apacheModulesSentence)
	var lines strings.Builder
	for _, match := range moduleName.FindAllStringSubmatch(list, -1) {
		fmt.Fprintf(&lines, "LoadModule %s_module modules/mod_%s.so\n", match[1], match[1])
	}
	if lines.Len() == 0 {
		t.Fatalf("the guide's sentence %q names no module", sentence)
	}
	return lines.String()
}

// guideApache is the guide's VirtualHost, served on the port and with the
// upstream a case chooses and any further edits a control makes to it. Its
// certificate lines are served as printed: the case mounts its certificate
// where they read it.
func guideApache(cert *publicCert, extra ...edit) documentedProxy {
	return documentedProxy{
		name:  "apache",
		roots: cert.pool,
		// mod_proxy_http sets X-Forwarded-For on every proxied request.
		clientHeader: "X-Forwarded-For",
		start: func(t *testing.T, upstream int) (string, int) {
			t.Helper()
			port := freePort(t)
			edits := append([]edit{
				{old: "<VirtualHost *:443>", new: fmt.Sprintf("<VirtualHost *:%d>", port), count: 1},
				{old: "http://127.0.0.1:8080/", new: fmt.Sprintf("http://127.0.0.1:%d/", upstream), count: 2},
			}, extra...)
			block := guideBlock(t, remoteDeploymentPage, "apache", edits...)

			dir := containerDir(t)
			name := fmt.Sprintf("gitlab-mcp-httpe2e-guide-apache-%d", port)
			startContainer(t, name, httpdImage, []string{
				writeContainerFile(t, dir, "httpd.conf", fmt.Sprintf(apacheFrame, port, guideApacheModules(t))) + ":/usr/local/apache2/conf/httpd.conf:ro",
				writeContainerFile(t, dir, "mcp.conf", block) + ":/usr/local/apache2/conf/mcp.conf:ro",
				cert.letsEncryptMount(),
			})
			return name, port
		},
	}
}

// TestProxyGuide_Apache serves the guide's VirtualHost under a base
// configuration that loads only the modules the guide names, holds it to the
// requirement table with the X-Forwarded-For pairing mod_proxy_http's own
// header gives it, and checks what the guide says flushpackets=on is for.
func TestProxyGuide_Apache(t *testing.T) {
	t.Parallel()
	cert := newPublicCert(t)

	assertProxyRequirements(t, guideApache(cert))

	t.Run("without flushpackets the stream still flows", func(t *testing.T) {
		t.Parallel()
		gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
		unflushed := guideApache(cert, edit{old: " flushpackets=on", new: "", count: 1})
		through := unflushed.behind(t, "--gitlab-url="+gitlab.url, "--public-url=https://"+publicHost).from(t, clientForHeaders)
		assertStreamIsNotHeld(t, through, map[string]string{"PRIVATE-TOKEN": "glpat-unflushed"})
	})
}
