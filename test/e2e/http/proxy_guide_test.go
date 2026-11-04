// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// proxy_guide_test.go runs the reverse proxies the remote deployment guide
// configures, each serving the block the guide prints, and holds each to the
// guide's requirement table (guide_test.go).
//
// proxy_test.go keeps the hosted deployment's nginx, whose CORS block is the
// collision a reader is told to avoid; the cases here are the configurations
// a reader is told to paste. Each runs in Docker on the host network, like the
// hosted nginx, from an image pinned to a minor version, and fails rather
// than skips when its container cannot start where the job says Docker is
// expected (dockerMustRun).
package httpe2e

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// nginxImage is every nginx case's image. A minor tag rather than a floating
// one, so the version under test moves when this line does.
const nginxImage = "nginx:1.31-alpine"

// nginxFrame is the nginx.conf a documented block is included from. The
// guide prints what goes inside http { }, as a file under conf.d does on a
// stock install, and the stock nginx.conf around it includes the MIME table
// the guide's AI Catalog location is written against.
const nginxFrame = `events { worker_connections 64; }
http {
  include /etc/nginx/mime.types;
  access_log off;
  include /etc/nginx/conf.d/mcp.conf;
}
`

// aiCatalog is the document the guide's nginx block serves itself from
// /etc/nginx/ai-catalog.json and never forwards.
const aiCatalog = `{"specVersion":"1.0","entries":[]}`

// guideNginx is the nginx block of the guide's reverse proxy section, served
// with the listen port and the upstream a case chooses, and any further
// edits a control makes to it.
//
// The certificate lines are served as printed: the case mounts its
// certificate where they read it.
func guideNginx(cert *publicCert, extra ...edit) documentedProxy {
	return documentedProxy{
		name:         "nginx",
		roots:        cert.pool,
		clientHeader: "X-Real-IP",
		start: func(t *testing.T, upstream int) (string, int) {
			t.Helper()
			port := freePort(t)
			edits := append([]edit{
				{old: "listen 443 ssl;", new: fmt.Sprintf("listen %s:%d ssl;", loopbackBind, port), count: 1},
				{old: "server 127.0.0.1:8080;", new: fmt.Sprintf("server 127.0.0.1:%d;", upstream), count: 1},
			}, extra...)
			block := guideBlock(t, remoteDeploymentPage, "nginx", edits...)

			dir := containerDir(t)
			name := fmt.Sprintf("gitlab-mcp-httpe2e-guide-nginx-%d", port)
			startContainer(t, name, nginxImage, []string{
				writeContainerFile(t, dir, "nginx.conf", nginxFrame) + ":/etc/nginx/nginx.conf:ro",
				writeContainerFile(t, dir, "mcp.conf", block) + ":/etc/nginx/conf.d/mcp.conf:ro",
				writeContainerFile(t, dir, "ai-catalog.json", aiCatalog) + ":/etc/nginx/ai-catalog.json:ro",
				cert.letsEncryptMount(),
			})
			return name, port
		},
	}
}

// TestProxyGuide_Nginx serves the guide's nginx block and holds it to the
// requirement table, and to the one claim the block makes in its own
// comments: the AI Catalog location answers with its own media type and the
// one CORS header nginx is allowed to send.
func TestProxyGuide_Nginx(t *testing.T) {
	t.Parallel()
	cert := newPublicCert(t)
	nginx := guideNginx(cert)

	assertProxyRequirements(t, nginx)

	t.Run("the AI Catalog is the proxy's own document", func(t *testing.T) {
		t.Parallel()
		gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
		catalog := nginx.behind(t, "--gitlab-url="+gitlab.url, "--public-url=https://"+publicHost).from(t, clientForHeaders)
		got := catalog.do(t, request{method: http.MethodGet, path: "/.well-known/ai-catalog.json"})
		if got.status != http.StatusOK || got.body != aiCatalog {
			t.Fatalf("GET /.well-known/ai-catalog.json = %d %q, want 200 and the file the block aliases", got.status, truncate(got.body))
		}
		if ct := got.header.Get("Content-Type"); ct != "application/ai-catalog+json" {
			t.Errorf("Content-Type = %q, want application/ai-catalog+json: the block's `types { }` is what keeps mime.types from sending application/json", ct)
		}
		if acao := got.header.Values("Access-Control-Allow-Origin"); len(acao) != 1 || acao[0] != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q, want exactly [*]: the proxy is the only thing that can send it for a document it serves itself", acao)
		}
	})

	// The control the stream assertion answers to, a proxy that holds the
	// stream, and what the block's "proxy_buffering off" is for.
	//
	// Dropping the line alone holds nothing. The server sends
	// X-Accel-Buffering: no on every stream, which turns nginx's buffering
	// off for that response whatever the configuration says, and an nginx
	// that buffers still passes an event on as soon as it reads it. What
	// holds a stream is compression with buffering on: nginx's gzip then
	// waits for enough of a body to compress (the server's
	// Cache-Control: no-transform does not stop it), while an unbuffered
	// nginx flushes the compressor on every read. So both copies compress
	// the stream and ignore the server's header, as a proxy in front of
	// nginx that strips it would make nginx do: the one that buffers must
	// fail the assertion, and the one that keeps the guide's line must
	// still stream.
	for _, tc := range []struct {
		name      string
		buffering string
		held      bool
	}{
		{name: "a compressing copy that buffers fails the stream assertion", buffering: "on", held: true},
		{name: "a compressing copy with the guide's proxy_buffering off still streams", buffering: "off", held: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
			compressing := guideNginx(cert, edit{
				old: "proxy_buffering off;",
				new: "proxy_buffering " + tc.buffering + ";\n        proxy_ignore_headers X-Accel-Buffering;\n" +
					"        gzip on;\n        gzip_types text/event-stream;\n        gzip_min_length 1;",
				count: 1,
			})
			through := compressing.behind(t, "--gitlab-url="+gitlab.url, "--public-url=https://"+publicHost).from(t, clientForHeaders)
			// The copy that streams gets the grace every stream assertion
			// gets, since a busy host can take seconds to answer a listen;
			// only the one that must hold is cut short.
			wait := streamGrace
			if tc.held {
				wait = heldWait
			}
			frame, err := firstListenFrame(t, through, map[string]string{"PRIVATE-TOKEN": "glpat-compressed"}, wait)
			switch {
			case tc.held && !errors.Is(err, errStreamHeld):
				t.Errorf("through a compressing nginx that buffers, the listen's first event = %q (%v), want nothing while the stream is open: "+
					"the stream assertion cannot tell a proxy that holds the stream from one that does not", truncate(frame), err)
			case !tc.held && err != nil:
				t.Errorf("through a compressing nginx with proxy_buffering off: %v; the line no longer keeps nginx flushing each event, and the guide says it does", err)
			}
		})
	}
}

// heldWait is how long the control waits for an event it must not see. The
// acknowledgement crosses a proxy that does not hold it in milliseconds, so
// this is far longer than a pass needs and far shorter than streamGrace,
// which is generous on purpose.
const heldWait = 3 * time.Second

// TestApplyEdits_TableNotMatchingTheBlock_IsRefused is the other control: a
// case whose substitution table no longer matches the block fails, naming the
// token, rather than serving a block it did not edit.
func TestApplyEdits_TableNotMatchingTheBlock_IsRefused(t *testing.T) {
	t.Parallel()
	const block = "listen 443 ssl;\nserver 127.0.0.1:8080;\nserver 127.0.0.1:8080;\n"
	tests := []struct {
		name  string
		edits []edit
		want  string
	}{
		{
			name:  "a token the block no longer carries",
			edits: []edit{{old: "listen 8443 ssl;", new: "listen 1 ssl;", count: 1}},
			want:  `"listen 8443 ssl;" appears 0 time(s) and the case replaces it 1 time(s)`,
		},
		{
			name:  "a token the block now carries twice",
			edits: []edit{{old: "server 127.0.0.1:8080;", new: "server 127.0.0.1:1;", count: 1}},
			want:  `"server 127.0.0.1:8080;" appears 2 time(s) and the case replaces it 1 time(s)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := applyEdits(block, tt.edits)
			if !errors.Is(err, errEditCount) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("applyEdits = %q, %v; want an error saying %s", got, err, tt.want)
			}
		})
	}

	t.Run("a table that matches is applied in one pass", func(t *testing.T) {
		t.Parallel()
		got, err := applyEdits(block, []edit{
			{old: "listen 443 ssl;", new: "listen 8080 ssl;", count: 1},
			{old: "127.0.0.1:8080", new: "127.0.0.1:9", count: 2},
		})
		if err != nil {
			t.Fatalf("applyEdits: %v", err)
		}
		// The first edit writes 8080 into the block; a second pass would
		// have rewritten it, and the count it was checked with would no
		// longer describe what was served.
		if want := "listen 8080 ssl;\nserver 127.0.0.1:9;\nserver 127.0.0.1:9;\n"; got != want {
			t.Errorf("applyEdits = %q, want %q", got, want)
		}
	})
}
