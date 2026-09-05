// Package httpe2e drives the real gitlab-mcp-server binary over HTTP the way a
// client does: cross-origin decisions, preflight, the authentication modes,
// rate limiting, the JSON-RPC shape of a refusal, shutdown and draining, and
// the flags that restrict them. It needs no GitLab and no credentials, so it
// runs on every CI push.
//
// The tests carry the httpe2e build tag; this file is what a plain build sees
// of the package. The in-process suite under test/e2e/suite cannot observe any
// of this, since it builds the server directly and drives an in-memory
// transport, which is why transport behavior is tested here and not there.
package httpe2e
