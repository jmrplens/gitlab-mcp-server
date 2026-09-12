// Package fixture is the GitLab state an end-to-end test stands on.
//
// Everything here is created through client-go rather than through the server
// under test, on purpose: fixture traffic never counts as coverage, and a
// broken tool then fails only the test that exercises it rather than every
// test that needs a project to start from. Each builder registers its own
// undo on the test's Env, so what a test creates is gone when the test ends,
// and the World a package shares is built once and torn down after the last
// test, with a digest that says whether anything changed it in between.
//
// The GitLab behavior the suite this replaces learned the hard way lives here
// with its reasons: the retries a project creation needs under load, the
// two-step permanent delete, the Sidekiq drain before a readiness wait, the
// waits themselves, and the facts about GitLab 19 that a request has to state.
//
// The builders carry the e2e build tag; this file is what a plain build sees
// of the package, the convention test/e2e/http/doc.go states. Like the
// harness, the package is importable only from test/e2e by Go's internal rule.
package fixture
