//go:build e2e

// access.go is what the fixture library sees of an Env: the client the
// harness probed the instance with, what the probe learned, the run's
// configuration, and a moment after the last test.
//
// The fixture library builds GitLab state through client-go rather than
// through the server under test, so it needs the client and not a session.
// The state it builds once per process, the shared World, outlives every test,
// so it needs a moment after m.Run to check that nothing changed it and to
// tear it down; and the sweep of what a failed test left behind needs the same
// moment. None of that is the harness's business, which knows nothing about
// projects or merge requests, and all of it needs something only the harness
// holds. This file hands those things over and nothing else.

package harness

import (
	"context"
	"errors"
	"log"
	"slices"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// Runtime is what the probe learned about the instance under test, as a test
// or a fixture may read it.
//
// It mirrors the harness's own record field for field, and Env.Runtime
// converts one into the other, so a field added to one and not the other is a
// compile error rather than a silently empty value.
type Runtime struct {
	// URL is the instance the run is pointed at.
	URL string
	// Version is what GET /api/v4/version reported.
	Version string
	// Enterprise is that endpoint's own edition flag: an EE image reports
	// true whether or not a license is installed, which is why it is kept
	// apart from Tier.
	Enterprise bool
	// Tier is what the license resolves to, Free when there is none.
	Tier edition.Tier
	// TierConfirmed says the tier came from a license rather than from the
	// fallback.
	TierConfirmed bool
	// Admin says the token's user is an instance administrator.
	Admin bool
	// Username and UserID identify that user.
	Username string
	UserID   int64
	// Scopes are the token's own, nil when the instance would not say.
	Scopes []string
}

// Client returns the GitLab client the harness probed the instance with, its
// tier already set.
//
// It is how the fixture library reaches GitLab beside the server under test:
// state built through client-go never counts as coverage, and a broken tool
// then fails only its own test rather than every test that needs a project.
func (e *Env) Client() *gitlabclient.Client { return e.inst.client }

// Runtime returns what the probe learned about the instance this test runs
// on. The scopes are copied, so a caller cannot change what the harness holds.
func (e *Env) Runtime() Runtime {
	facts := Runtime(e.inst.facts)
	facts.Scopes = slices.Clone(facts.Scopes)
	return facts
}

// Setting returns one value of the run's configuration, or the empty string.
//
// It reads the map Main resolved and never the process environment: a value a
// dotenv file set is visible here and nowhere else in this process, which is
// what keeps a file this run reads from configuring the harness's own
// environment by accident.
func (e *Env) Setting(key string) string { return e.inst.settings.get(key) }

// DockerMode reports whether the run is against the ephemeral Docker stack,
// which is disposable and provisions fixtures of its own.
func (e *Env) DockerMode() bool { return e.inst.dockerMode() }

// Package returns the name of the package under test, which is the first half
// of the consumer slot a per-consumer seed is provisioned for.
func (e *Env) Package() string { return e.inst.pkg }

// HasRunner reports whether a CI runner can pick a job up, asking GitLab once
// per process. It is the same answer NeedRunner gives, offered to a fixture
// that would otherwise wait a whole budget for a pipeline nothing will run.
func (e *Env) HasRunner() bool { return e.inst.hasRunner() }

// RepoRoot returns the repository root every path the harness resolves
// starts from, for a fixture that reads a file the provisioning scripts left
// there. A path relative to the working directory would name a different
// file for each of the three packages, which is why the root is the one
// place to resolve from.
func (e *Env) RepoRoot() string {
	e.T.Helper()
	root, err := repoRoot()
	if err != nil {
		e.T.Fatalf("resolving the repository root: %v", err)
	}
	return root
}

// ReprobeTier asks the instance what tier it is now, the way the bootstrap
// asked, and returns the answer beside what the bootstrap found.
//
// It exists for the one test that changes the license: adding and removing
// one changes what every later session is served, so that test says
// afterwards that the instance is where it was found. Nothing here updates
// the harness's own record, on purpose: a tier that changed is a failure of
// the test that changed it, not a new fact for the next test to be built on.
func (e *Env) ReprobeTier() (now, before edition.Tier) {
	e.T.Helper()
	ctx, cancel := context.WithTimeout(e.Ctx, probeTimeout)
	defer cancel()
	return e.inst.client.DetectTier(ctx), e.inst.facts.Tier
}

// exitHooks is what runs after the last test of the package and before Main
// returns.
type exitHooks struct {
	mu   sync.Mutex
	fns  []func() error
	done bool
}

// hooks is this test binary's registry.
var hooks exitHooks

// AtExit registers fn to run once every test of the package has ended, before
// the sessions close and before Main returns. Hooks run last-registered
// first, like a cleanup ledger, and an error from any of them fails the run.
//
// It exists for state that outlives a test: the shared World the fixture
// library builds once per process, and the sweep of what a failed test left
// behind. A t.Cleanup would run them at the end of the first test that asked,
// which is exactly when the next test still needs them.
func AtExit(fn func() error) {
	hooks.mu.Lock()
	defer hooks.mu.Unlock()

	if hooks.done {
		log.Println("e2e: an exit hook was registered after the exit hooks ran, and will never run")
		return
	}
	hooks.fns = append(hooks.fns, fn)
}

// runExitHooks runs every registered hook once, last-registered first, and
// returns what failed. A second call runs nothing.
func runExitHooks() []error {
	hooks.mu.Lock()
	if hooks.done {
		hooks.mu.Unlock()
		return nil
	}
	hooks.done = true
	fns := slices.Clone(hooks.fns)
	hooks.mu.Unlock()

	var failures []error
	for _, fn := range slices.Backward(fns) {
		if err := fn(); err != nil {
			log.Printf("e2e: exit hook failed: %v", err)
			failures = append(failures, err)
		}
	}
	return failures
}

// errExitHooks is what a run whose exit hooks failed is failed with, so the
// cause reads as one thing in the log rather than as a bare exit code.
var errExitHooks = errors.New("an exit hook failed")

// exitCodeAfterHooks folds the hooks' outcome into the tests' verdict: a
// failed hook fails a passing run and leaves a failing one as it was.
func exitCodeAfterHooks(code int, failures []error) int {
	if len(failures) > 0 && code == 0 {
		log.Printf("e2e: %v; the run is failed", errExitHooks)
		return 1
	}
	return code
}
