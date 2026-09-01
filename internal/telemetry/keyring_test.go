// keyring_test.go covers where a pseudonymisation key comes from and how long
// it lives, which is the choice an operator makes and the one this package
// exists to honor exactly.
package telemetry

import (
	"testing"
	"time"
)

// TestKeyring_AConfiguredSecretGivesEveryReplicaTheSameKeys is the property the
// setting exists for.
//
// Three replicas behind a load balancer are three processes. Without a shared
// secret each generates its own, so one person carries three digests at once
// and a count of distinct users triples. Two keyrings built from the same
// secret stand in for two replicas: they have to agree, or the setting does
// nothing it was added to do.
func TestKeyring_AConfiguredSecretGivesEveryReplicaTheSameKeys(t *testing.T) {
	t.Parallel()

	const secret = "a deployment-wide secret"
	first, second := keyringFrom(t, secret, 0), keyringFrom(t, secret, 0)

	if a, b := first.IdentityPseudonym("42"), second.IdentityPseudonym("42"); a != b {
		t.Errorf("two replicas gave one caller two digests (%q, %q)", a, b)
	}
	if a, b := first.ResourcePseudonym("gitlab://project/7"),
		second.ResourcePseudonym("gitlab://project/7"); a != b {
		t.Errorf("two replicas gave one resource two digests (%q, %q)", a, b)
	}
}

// TestKeyring_GeneratedKeysDifferPerProcess is the default, and the reason the
// setting had to be added rather than assumed.
func TestKeyring_GeneratedKeysDifferPerProcess(t *testing.T) {
	t.Parallel()

	first, second := keyringFrom(t, "", 0), keyringFrom(t, "", 0)

	if a, b := first.IdentityPseudonym("42"), second.IdentityPseudonym("42"); a == b {
		t.Errorf("two processes produced the same digest %q from generated keys; the key is not per process", a)
	}
}

// TestKeyring_OneSecretYieldsTwoIndependentKeys covers what HKDF buys.
//
// Without the separate info strings both pseudonyms would be HMACs under the
// same key, so a digest of a user id and a digest of a resource URI could be
// compared against each other: anyone holding an export could tell that the
// project whose id happens to equal a user id is the same number. Distinct keys
// make the two spaces unrelated.
func TestKeyring_OneSecretYieldsTwoIndependentKeys(t *testing.T) {
	t.Parallel()

	ring := keyringFrom(t, "one secret", 0)

	// The same input through both, which is the only way to see the keys are
	// not the same key.
	const value = "42"
	if a, b := ring.IdentityPseudonym(value), ring.ResourcePseudonym(value); a == b {
		t.Errorf("the identity and resource keys are the same key: both gave %q", a)
	}
}

// TestKeyring_AGeneratedKeyRotatesOnItsInterval covers the mono-instance case,
// where a long-lived process would otherwise hold one pseudonym for months.
//
// The clock is injected rather than waited on: a test that slept for the
// interval would either be slow or would test an interval nobody would set.
func TestKeyring_AGeneratedKeyRotatesOnItsInterval(t *testing.T) {
	t.Parallel()

	ring := keyringFrom(t, "", time.Hour)

	// The whole timeline is the injected one, baseline included. Setting only
	// the clock leaves rotatedAt holding the wall time of construction, so
	// whether the key looks due depends on what time of day the suite runs:
	// this test passed before midnight and failed after it.
	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ring.now = func() time.Time { return clock }
	ring.rotatedAt = clock

	before := ring.IdentityPseudonym("42")

	// Inside the interval, nothing changes: a pseudonym that moved on every
	// call would correlate nothing at all.
	clock = clock.Add(59 * time.Minute)
	if during := ring.IdentityPseudonym("42"); during != before {
		t.Errorf("the digest changed inside the interval (%q then %q)", before, during)
	}

	clock = clock.Add(2 * time.Minute)
	if after := ring.IdentityPseudonym("42"); after == before {
		t.Errorf("the digest %q survived its rotation interval", after)
	}
}

// TestKeyring_AConfiguredKeyIgnoresRotation pins the precedence between two
// settings that cannot both hold.
//
// Rotating a secret its owner supplied would destroy the correlation they
// configured it for, and they cannot rotate it from outside if this server is
// also rotating it from inside. The key wins; the wiring layer says so in a
// warning where an operator will read it.
func TestKeyring_AConfiguredKeyIgnoresRotation(t *testing.T) {
	t.Parallel()

	ring := keyringFrom(t, "a deployment-wide secret", time.Hour)
	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ring.now = func() time.Time { return clock }
	ring.rotatedAt = clock

	before := ring.IdentityPseudonym("42")
	clock = clock.Add(72 * time.Hour)

	if after := ring.IdentityPseudonym("42"); after != before {
		t.Errorf("a configured key rotated (%q then %q)", before, after)
	}
	if ring.Rotation() != 0 {
		t.Errorf("Rotation() = %s on a configured keyring, want 0", ring.Rotation())
	}
}

// TestKeyring_RefusesAnIntervalOutOfRange covers the startup check, which is
// where a typo is meant to be caught.
func TestKeyring_RefusesAnIntervalOutOfRange(t *testing.T) {
	t.Parallel()

	for name, rotation := range map[string]time.Duration{
		"negative":     -time.Second,
		"past the cap": MaxKeyRotation + time.Hour,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewKeyring("", rotation); err == nil {
				t.Errorf("NewKeyring accepted %s", rotation)
			}
		})
	}
}

// TestKeyring_EmptyInputIsNotADigest covers the case that would otherwise
// produce a bucket meaning "several different things that were all absent".
func TestKeyring_EmptyInputIsNotADigest(t *testing.T) {
	t.Parallel()

	ring := keyringFrom(t, "", 0)
	if got := ring.IdentityPseudonym(""); got != "" {
		t.Errorf("an absent user produced the digest %q", got)
	}
	if got := ring.ResourcePseudonym(""); got != "" {
		t.Errorf("an absent resource produced the digest %q", got)
	}
}

// TestKeyring_NilRecordsNothing pins the answer a caller gets for never wiring
// one, which must be silence rather than a panic or a fake pseudonym.
func TestKeyring_NilRecordsNothing(t *testing.T) {
	t.Parallel()

	var ring *Keyring
	if got := ring.IdentityPseudonym("42"); got != "" {
		t.Errorf("a nil keyring produced %q", got)
	}
	if ring.Configured() || ring.Rotation() != 0 {
		t.Error("a nil keyring reports a configuration it does not have")
	}
}

// keyringFrom builds one, failing the test rather than returning an error every
// caller would have to check identically.
func keyringFrom(t *testing.T, secret string, rotation time.Duration) *Keyring {
	t.Helper()

	ring, err := NewKeyring(secret, rotation)
	if err != nil {
		t.Fatalf("NewKeyring(%q, %s): %v", secret, rotation, err)
	}
	return ring
}
