// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// keep_alive_test.go holds what the binary puts on the wire while a tool call
// waits on GitLab and has nothing to report: on go-sdk v1.8.0, not a byte, its
// response headers included, until the result (register row 8).
package httpe2e

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// silentCallHold is how long the stand-in GitLab holds the read. It is longer
// than the server's 25-second heartbeat, so a heartbeat that reached the
// stream would show, and longer than the 30 seconds after which the go-sdk
// release following v1.8.0 writes a keep-alive of its own on a streamed POST
// (go-sdk#1477), so the dependency bump that brings that fails this test
// rather than passing it unread.
const silentCallHold = 35 * time.Second

// silentCallFix is what a failure here asks for.
const silentCallFix = "the binary now puts bytes on the wire of a tool call that has nothing to report before its result, " +
	"which on go-sdk v1.8.0 it does not: update row 8 of docs/development/upstream-bugs.md, the keep-alive paragraph of " +
	"site/src/content/docs/operations/http-server.mdx, the read timeout row and the Cloudflare paragraph of " +
	"site/src/content/docs/operations/remote-deployment.mdx, the proxy row of " +
	"site/src/content/docs/operations/troubleshooting.mdx, each with its Spanish twin, the comment of sseAwareWriter.Write in " +
	"cmd/server/main.go, TestSSEKeepAlive_ASilentToolCall_GetsNoByteBeforeItsResult and this test, in the same pull request."

// TestSSE_ASilentToolCall_GetsNoByteBeforeItsResult measures, against the
// binary, the stream the server's heartbeat does not reach: a streamed POST
// whose tool call waits on GitLab with no progress token and so sends nothing
// before its result.
//
// go-sdk v1.8.0 sets text/event-stream on that response and writes nothing
// until its first event, and the heartbeat starts when a response commits, so
// the call is silent on the wire for as long as GitLab takes. Behind nginx's
// default proxy_read_timeout of sixty seconds a longer call is cut, and the
// client waits for headers rather than for an event. Both revisions a client
// can send a tool call in are asked, since the stream is the SDK's either way.
func TestSSE_ASilentToolCall_GetsNoByteBeforeItsResult(t *testing.T) {
	t.Parallel()

	for _, revision := range []struct {
		name   string
		body   string
		header map[string]string
	}{
		{
			name: "2026-07-28",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
				`"arguments":{"action":"project.get","params":{"project_id":"1"}},` +
				`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`,
			header: map[string]string{
				"Mcp-Protocol-Version": protocolVersion,
				"Mcp-Method":           "tools/call",
				"Mcp-Name":             "gitlab_execute_action",
				"Mcp-Param-Action":     "project.get",
			},
		},
		{
			name: "2025-06-18",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
				`"arguments":{"action":"project.get","params":{"project_id":"1"}}}}`,
			header: map[string]string{"Mcp-Protocol-Version": "2025-06-18"},
		},
	} {
		t.Run(revision.name, func(t *testing.T) {
			t.Parallel()
			gitlab := startHoldingProjectsGitLab(t)
			srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

			ctx, cancel := context.WithTimeout(t.Context(), silentCallHold+time.Minute)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/mcp", strings.NewReader(revision.body))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", acceptHeader)
			req.Header.Set("PRIVATE-TOKEN", "glpat-silent")
			for name, value := range revision.header {
				req.Header.Set(name, value)
			}

			sent := time.Now()
			release := time.AfterFunc(silentCallHold, gitlab.release)
			defer release.Stop()
			resp, err := srv.httpClient().Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			headersAfter := time.Since(sent)
			t.Logf("headers after %v, GitLab held for %v", headersAfter.Round(time.Millisecond), silentCallHold)
			if headersAfter < silentCallHold {
				t.Errorf("the response headers arrived after %v, while GitLab held the call for %v. %s",
					headersAfter.Round(time.Millisecond), silentCallHold, silentCallFix)
			}

			first, err := bufio.NewReader(resp.Body).ReadString('\n')
			if err != nil {
				t.Fatalf("reading the stream: %v", err)
			}
			if first != "event: message\n" {
				t.Errorf("the stream's first line is %q, want the result's event. %s", first, silentCallFix)
			}
		})
	}
}
