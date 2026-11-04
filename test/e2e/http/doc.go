// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package httpe2e drives the real gitlab-mcp-server binary over HTTP the way a
// client does: cross-origin decisions, preflight, the authentication modes,
// rate limiting, the JSON-RPC shape of a refusal, shutdown and draining, and
// the flags that restrict them. It needs no GitLab and no credentials, so it
// runs on every CI push.
//
// With Docker it also runs the server behind real proxies and balancers:
// the hosted deployment's nginx (proxy_test.go), and every configuration
// the documentation prints, read from the pages at test time
// (guide_test.go): nginx, Caddy, Traefik and Apache from the remote
// deployment guide, its plain-HTTP LAN block, its Cloudflare Tunnel
// configuration checked by cloudflared offline, and the nginx and HAProxy
// balancers of that guide and of the load balancing page. A Cloudflare quick
// tunnel case reaches the server through Cloudflare's edge when
// E2E_CLOUDFLARE_QUICK_TUNNEL=true, never on CI.
//
// The tests carry the httpe2e build tag; this file is what a plain build sees
// of the package. The suite under test/e2e/gitlab drives the real binary over
// stdio, so it observes the stdio transport and none of the HTTP handler
// chain, which is why transport behavior for HTTP is tested here and not there.
package httpe2e
