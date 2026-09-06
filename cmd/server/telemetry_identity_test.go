// telemetry_identity_test.go covers how this binary resolves the identity
// policy into the redactor every signal shares.
package main

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"strings"
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
	if telemetryUsers() == nil {
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
	if telemetryUsers() != nil {
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

// TestAnnounceIdentityChoice_SaysOnlyWhatIsWorthReading covers the startup
// line describing what this deployment records about callers.
//
// The default records nobody, and a line saying so on every startup is noise;
// anything else is the line somebody skimming a log for surprises needs to see.
// The keyring is built before the announcement rather than after, because
// announcing that identity is recorded and then failing to build the thing that
// records it would be the announcement lying. And the announcement has to reach
// the collector as well as stderr, which is why it is separate from building
// the redactor: doing it there wrote both lines through the handler that was
// about to be replaced.
func TestAnnounceIdentityChoice_SaysOnlyWhatIsWorthReading(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		key      string
		rotation string
		want     []string
		unwanted []string
	}{
		{
			name:     "the default announces nothing",
			policy:   "",
			unwanted: []string{"telemetry records caller identity"},
		},
		{
			name:     "an unusable policy announces nothing",
			policy:   "hashed-with-pepper",
			unwanted: []string{"telemetry records caller identity"},
		},
		{
			name:   "full names what leaves the process",
			policy: "full",
			want:   []string{"telemetry records caller identity", "the GitLab user id and username"},
		},
		{
			name:   "a configured key is stable across replicas",
			policy: "pseudonymous",
			key:    "a deployment-wide secret",
			want:   []string{"stable across replicas and restarts"},
		},
		{
			name:     "a configured key cancels a requested rotation, out loud",
			policy:   "pseudonymous",
			key:      "a deployment-wide secret",
			rotation: "24h",
			want:     []string{"the rotation interval is ignored"},
		},
		{
			name:     "a generated key that rotates says so",
			policy:   "pseudonymous",
			rotation: "24h",
			want:     []string{"generated key that rotates"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(telemetry.EnvIdentityName, tt.policy)
			t.Setenv(telemetry.EnvIdentityKeyName, tt.key)
			t.Setenv(telemetry.EnvIdentityRotationName, tt.rotation)
			resetIdentityKeyring(t)

			var buf bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })

			announceIdentityChoice()

			logged := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(logged, want) {
					t.Errorf("startup log %q is missing %q", logged, want)
				}
			}
			for _, unwanted := range tt.unwanted {
				if strings.Contains(logged, unwanted) {
					t.Errorf("startup log %q announces %q for a policy that records nobody", logged, unwanted)
				}
			}
		})
	}
}

// TestTelemetryIdentityRotation_TheFlagBeatsTheEnvironment covers the precedence
// for the one identity setting that has a flag.
//
// The key deliberately has none — process arguments are readable through /proc
// by any local principal — but how long a generated key lives is not a secret,
// so it follows the same rule as every other flag: what the operator typed wins
// over what the environment happened to carry.
func TestTelemetryIdentityRotation_TheFlagBeatsTheEnvironment(t *testing.T) {
	withFreshFlagSet(t)
	t.Setenv(telemetry.EnvIdentityRotationName, "1h")

	previous := telemetryIdentityRotationFlag
	t.Cleanup(func() { telemetryIdentityRotationFlag = previous })
	telemetryIdentityRotationFlag = flag.String("telemetry-identity-rotation", "", "")
	if err := flag.CommandLine.Parse([]string{"-telemetry-identity-rotation=6h"}); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	rotation, err := telemetryIdentityRotation()
	if err != nil {
		t.Fatalf("telemetryIdentityRotation: %v", err)
	}
	if rotation != 6*time.Hour {
		t.Errorf("rotation = %s, want the flag's 6h rather than the environment's 1h", rotation)
	}
}

// TestTelemetryResources_AnUnusablePolicy_RecordsNothing covers the redactor
// the middleware consults for resource URIs.
//
// A server that is already running must not stop mid-startup over a privacy
// setting it cannot parse, and it must not guess either: the safe answer is to
// record nothing, which nil is. The loud complaint about the same value belongs
// to the startup validation, which runs before any of this.
func TestTelemetryResources_AnUnusablePolicy_RecordsNothing(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityName, "hashed-with-pepper")
	resetIdentityKeyring(t)

	if got := telemetryResources(); got != nil {
		t.Errorf("telemetryResources() = %#v, want nil so nothing about a resource is recorded", got)
	}
}

// TestLogIdentityPolicy_SpeaksOnlyForANonDefaultPolicy pins the one line an
// operator skimming a log for surprises needs, and its absence for the default:
// "we are recording nobody" on every startup would be noise, "we are recording
// usernames" is the line that has to be there.
func TestLogIdentityPolicy_SpeaksOnlyForANonDefaultPolicy(t *testing.T) {
	cases := []struct {
		name   string
		policy telemetry.IdentityPolicy
		want   string
	}{
		{name: "the default says nothing", policy: telemetry.DefaultIdentityPolicy, want: ""},
		{name: "full names what is exported", policy: telemetry.IdentityFull, want: "telemetry records caller identity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })

			logIdentityPolicy(tc.policy)

			if tc.want == "" && buf.Len() != 0 {
				t.Errorf("the default policy logged %q, want silence", buf.String())
			}
			if tc.want != "" && !strings.Contains(buf.String(), tc.want) {
				t.Errorf("log = %q, want it to carry %q", buf.String(), tc.want)
			}
		})
	}
}

// TestTelemetryIdentityPolicy_TheFlagBeatsTheEnvironment covers the precedence
// for the policy the same way the rotation test does for its flag: what the
// operator typed wins over what the environment happened to carry.
func TestTelemetryIdentityPolicy_TheFlagBeatsTheEnvironment(t *testing.T) {
	withFreshFlagSet(t)
	t.Setenv(telemetry.EnvIdentityName, string(telemetry.IdentityNone))

	previous := telemetryIdentityFlag
	t.Cleanup(func() { telemetryIdentityFlag = previous })
	telemetryIdentityFlag = flag.String("telemetry-identity", string(telemetry.DefaultIdentityPolicy), "")
	if err := flag.CommandLine.Parse([]string{"-telemetry-identity=full"}); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	policy, err := telemetryIdentityPolicy()
	if err != nil {
		t.Fatalf("telemetryIdentityPolicy: %v", err)
	}
	if policy != telemetry.IdentityFull {
		t.Errorf("policy = %q, want the flag's full rather than the environment's none", policy)
	}
}

// TestIdentityKeyring_ARotationOutOfRange_RecordsNothingAndAnnouncesNothing
// covers a rotation that parses but that the keyring refuses: longer than the
// thirty days a generated key may live. The keyring is then absent, which
// every consumer reads as "record nothing", and the startup announcement says
// nothing either, because announcing that identity is recorded and then
// failing to build the thing that records it would be the announcement lying.
func TestIdentityKeyring_ARotationOutOfRange_RecordsNothingAndAnnouncesNothing(t *testing.T) {
	t.Setenv(telemetry.EnvIdentityName, string(telemetry.IdentityPseudonymous))
	t.Setenv(telemetry.EnvIdentityKeyName, "")
	t.Setenv(telemetry.EnvIdentityRotationName, (telemetry.MaxKeyRotation + time.Hour).String())
	resetIdentityKeyring(t)

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	if ring := identityKeyring(); ring != nil {
		t.Fatalf("identityKeyring() = %v, want nil for a rotation past the maximum", ring)
	}
	if !strings.Contains(buf.String(), "keyring could not be built") {
		t.Errorf("log = %q, want the keyring failure reported", buf.String())
	}

	buf.Reset()
	announceIdentityChoice()
	if strings.Contains(buf.String(), "telemetry records caller identity") {
		t.Errorf("startup log %q announces identity recording for a keyring that could not be built", buf.String())
	}
}
