// telemetry_identity_test.go covers how this binary resolves the identity
// policy into the redactor every signal shares.
package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// TestTelemetryUsers_EveryPooledServerAgreesOnTheDigest is the regression for
// the shape a hosted deployment actually runs.
//
// telemetryUsers is called from createServer, and createServer runs once per
// entry in the HTTP server pool: one per token and GitLab URL, rebuilt whenever
// an entry is evicted by --pool-idle-timeout or by the size bound. Building a
// redactor there gave each entry its own salt, so one person carried a
// different digest per entry, inside one process, with nothing restarted.
//
// The collector data that prompted this shows five processes emitting two
// digests each. Two callers explains that innocently; an evicted and rebuilt
// entry explains it as a defect, and from the exported data alone the two are
// indistinguishable, which is exactly why this belongs in a test.
func TestTelemetryUsers_EveryPooledServerAgreesOnTheDigest(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityName, string(telemetry.IdentityPseudonymous))
	resetIdentityRedactor(t)

	// Through telemetryUsers, deliberately: that is the function createServer
	// calls, and calling the accessor directly would prove the accessor rather
	// than the path the pool takes.
	first, second := telemetryUsers(), telemetryUsers()
	if first == nil || second == nil {
		t.Fatal("no attributer was built for the pseudonymous policy")
	}

	// One caller, presented to each entry the way a request presents them.
	ctx := toolutil.IdentityToContext(context.Background(),
		toolutil.UserIdentity{UserID: "15767218", Username: "someone"})

	a := digestOf(t, first.UserAttributes(ctx, nil))
	b := digestOf(t, second.UserAttributes(ctx, nil))

	if a == "" {
		t.Fatal("the pseudonymous policy produced no digest at all")
	}
	if a != b {
		t.Errorf("one person carries two digests across pool entries (%q, %q); a dashboard grouping by user.hash counts them as two people",
			a, b)
	}
}

// digestOf pulls user.hash out of an attribute set.
func digestOf(t *testing.T, attrs []attribute.KeyValue) string {
	t.Helper()

	for _, attr := range attrs {
		if string(attr.Key) == telemetry.AttrUserHash {
			return attr.Value.AsString()
		}
	}
	return ""
}

// TestIdentityKeyring_AConfiguredKeyIsUsed covers the setting a multi-replica
// deployment needs, through the accessor rather than the constructor.
//
// The value of the setting is that every replica derives the same keys, so the
// assertion that matters is not "a keyring was built" but "this key produced
// that digest", which a second process can be checked against.
func TestIdentityKeyring_AConfiguredKeyIsUsed(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityKeyName, "a deployment-wide secret")
	resetIdentityKeyring(t)

	ring := identityKeyring()
	if ring == nil {
		t.Fatal("no keyring was built from a configured key")
	}
	if !ring.Configured() {
		t.Error("the keyring does not report the key as configured, so rotation would apply to it")
	}

	// What a second replica would compute from the same setting.
	elsewhere, err := telemetry.NewKeyring("a deployment-wide secret", 0)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	if a, b := ring.IdentityPseudonym("42"), elsewhere.IdentityPseudonym("42"); a != b {
		t.Errorf("two replicas with the same key gave one caller two digests (%q, %q)", a, b)
	}
}

// TestIdentityKeyring_WithoutAKeyNothingIsShared is the default, stated as the
// property rather than as the absence of a setting.
func TestIdentityKeyring_WithoutAKeyNothingIsShared(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityKeyName, "")
	resetIdentityKeyring(t)

	ring := identityKeyring()
	if ring == nil {
		t.Fatal("no keyring was built without a configured key")
	}
	if ring.Configured() {
		t.Error("a generated keyring reports itself as configured")
	}

	elsewhere, err := telemetry.NewKeyring("", 0)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	if a, b := ring.IdentityPseudonym("42"), elsewhere.IdentityPseudonym("42"); a == b {
		t.Errorf("two processes without a shared key produced the same digest %q", a)
	}
}

// TestIdentityKeyring_TheRotationIntervalIsParsed covers the second setting,
// including the failure direction: an unusable value records nothing rather
// than guessing a duration.
func TestIdentityKeyring_TheRotationIntervalIsParsed(t *testing.T) {
	t.Run("a duration is honored", func(t *testing.T) {
		t.Setenv(telemetry.EnvIdentityRotationName, "24h")
		resetIdentityKeyring(t)

		ring := identityKeyring()
		if ring == nil {
			t.Fatal("no keyring was built")
		}
		if got := ring.Rotation(); got != 24*time.Hour {
			t.Errorf("Rotation() = %s, want 24h", got)
		}
	})

	t.Run("an unusable value records nothing", func(t *testing.T) {
		t.Setenv(telemetry.EnvIdentityRotationName, "24")
		resetIdentityKeyring(t)

		if ring := identityKeyring(); ring != nil {
			t.Errorf("a bare number was accepted as a duration; Rotation() = %s", ring.Rotation())
		}
	})
}

// TestIdentityKeyring_OneKeyringServesEveryCaller is the same invariant the
// redactor has, one layer down: the keyring is where the secret lives now, so a
// second one would reintroduce the defect the shared redactor fixed.
func TestIdentityKeyring_OneKeyringServesEveryCaller(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityKeyName, "")
	resetIdentityKeyring(t)

	if first, second := identityKeyring(), identityKeyring(); first != second {
		t.Error("two calls produced two keyrings; one caller would carry two digests")
	}
}

// resetIdentityKeyring clears the memoized keyring so a test can build one
// under its own environment.
func resetIdentityKeyring(t *testing.T) {
	t.Helper()

	identityKeyringOnce = sync.Once{}
	identityKeyringVal = nil
	resetIdentityRedactor(t)
	t.Cleanup(func() {
		identityKeyringOnce = sync.Once{}
		identityKeyringVal = nil
	})
}

// TestIdentityKeyring_AConfiguredKeySurvivesABrokenRotation is the wiring half
// of the constructor test: the rotation variable is not even read when a key is
// configured, so an unparseable value cannot stop the keyring being built.
func TestIdentityKeyring_AConfiguredKeySurvivesABrokenRotation(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityKeyName, "a deployment-wide secret")
	t.Setenv(telemetry.EnvIdentityRotationName, "not-a-duration")
	resetIdentityKeyring(t)

	ring := identityKeyring()
	if ring == nil {
		t.Fatal("an unparseable rotation stopped a configured key from building the keyring")
	}
	if !ring.Configured() {
		t.Error("the keyring does not report the configured key")
	}
}
