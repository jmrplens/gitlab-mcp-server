// telemetry_identity_test.go covers how this binary resolves the identity
// policy into the redactor every signal shares.
package main

import (
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
)

// TestIdentityRedactor_OneInstanceServesEverySignal is a wiring test, and it
// exists because the failure it prevents is invisible in any single signal.
//
// Each redactor generates its own salt. Two of them would give the same person
// one user.hash on a span and a different one on a log record: one person
// appearing as two, correlated with nothing, and every individual signal
// looking perfectly correct on its own.
//
// It also keeps TestRedactor_PseudonymDoesNotSurviveARestart honest. That test
// uses "a new redactor" as a stand-in for "a new process", which is only a
// sound proxy while exactly one exists per process.
func TestIdentityRedactor_OneInstanceServesEverySignal(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityName, string(telemetry.IdentityPseudonymous))
	resetIdentityRedactor(t)

	first := identityRedactor()
	second := identityRedactor()

	if first == nil {
		t.Fatal("no redactor was built for the pseudonymous policy")
	}
	if first != second {
		t.Error("two calls produced two redactors; the same user would get two different digests")
	}
	if users := telemetryUsers(); users == nil {
		t.Error("the middleware was given no attributer while the policy names callers")
	}
}

// TestIdentityRedactor_NoneBuildsNothing pins the default, where the absence of
// a redactor is what every consumer reads as "record nobody".
//
// Nil rather than a redactor that returns nothing: the middleware has its own
// no-op path, and no salt is allocated for a policy that will not use one.
func TestIdentityRedactor_NoneBuildsNothing(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityName, string(telemetry.IdentityNone))
	resetIdentityRedactor(t)

	if got := identityRedactor(); got != nil {
		t.Errorf("policy none built a redactor (%v); nothing should name a caller", got)
	}
	if users := telemetryUsers(); users != nil {
		t.Error("policy none gave the middleware an attributer")
	}
}

// TestIdentityRedactor_AnUnusableValueRecordsNothing covers the failure
// direction, which is the only safe one: a policy that cannot be parsed must
// record less rather than guess or stop the process mid-startup.
func TestIdentityRedactor_AnUnusableValueRecordsNothing(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityName, "not-a-policy")
	resetIdentityRedactor(t)

	if got := identityRedactor(); got != nil {
		t.Errorf("an unparseable policy built a redactor (%v)", got)
	}
}

// resetIdentityRedactor clears the memoized redactor so a test can build one
// under its own environment. Package-level state is the price of building it
// once; a test that could not reset it would have to run first to be right.
func resetIdentityRedactor(t *testing.T) {
	t.Helper()

	identityRedactorOnce = sync.Once{}
	identityRedactorVal = nil
	t.Cleanup(func() {
		identityRedactorOnce = sync.Once{}
		identityRedactorVal = nil
	})
}
