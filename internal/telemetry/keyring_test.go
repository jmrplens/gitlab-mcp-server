// keyring_test.go covers where a pseudonymisation key comes from and how long
// it lives, which is the choice an operator makes and the one this package
// exists to honor exactly.
package telemetry

import (
	"bytes"
	"crypto/fips140"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	xhkdf "golang.org/x/crypto/hkdf"
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

// TestKeyring_AConfiguredKeySurvivesABrokenRotation pins the precedence at the
// constructor: rotation does not apply to a configured key, so a rotation value
// that would be refused on its own must not veto the key.
//
// Before this, the range check ran first and an operator with a working key and
// a typo in a setting documented as ignored lost identity recording entirely.
func TestKeyring_AConfiguredKeySurvivesABrokenRotation(t *testing.T) {
	t.Parallel()

	ring, err := NewKeyring("a deployment-wide secret", -time.Hour)
	if err != nil {
		t.Fatalf("a configured key was refused over a rotation that does not apply to it: %v", err)
	}
	if !ring.Configured() || ring.Rotation() != 0 {
		t.Errorf("configured=%v rotation=%s, want configured with no rotation", ring.Configured(), ring.Rotation())
	}
}

// TestKeyring_AcceptsTheLongestIntervalItDocuments covers the far edge of the
// range check, which the refusal test cannot reach.
//
// The cap is inclusive: an operator who writes the documented maximum has
// written a legal value, and refusing it would make the documented number the
// one figure the server will not take. The literal is spelled out rather than
// referred back to MaxKeyRotation, so a change to the constant is a change to
// this assertion rather than a tautology that follows it.
func TestKeyring_AcceptsTheLongestIntervalItDocuments(t *testing.T) {
	t.Parallel()

	if MaxKeyRotation != 720*time.Hour {
		t.Errorf("MaxKeyRotation = %s, want 720h (30 days), which is the bound the documentation states", MaxKeyRotation)
	}

	ring, err := NewKeyring("", MaxKeyRotation)
	if err != nil {
		t.Fatalf("NewKeyring refused the documented maximum %s: %v", MaxKeyRotation, err)
	}
	if ring.Rotation() != MaxKeyRotation {
		t.Errorf("Rotation() = %s, want the %s it was built with", ring.Rotation(), MaxKeyRotation)
	}
}

// TestKeyring_ARotationRestartsTheInterval covers what happens after a key
// rotates, which is the half no test asserted.
//
// Two things are pinned here and both are about the moment the new key was
// installed being recorded. The interval is measured inclusively, so a key
// whose interval has exactly elapsed is due: reading it as "strictly past"
// would hold every key one poll longer than the operator asked. And the
// rotation must restart the clock, or the keyring stays permanently due and
// generates fresh keys on every single call, which correlates nothing at all
// while looking exactly like a working rotation from the outside.
func TestKeyring_ARotationRestartsTheInterval(t *testing.T) {
	t.Parallel()

	ring := keyringFrom(t, "", time.Hour)

	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ring.now = func() time.Time { return clock }
	ring.rotatedAt = clock

	before := ring.IdentityPseudonym("42")

	// Exactly the interval, not a minute past it.
	clock = clock.Add(time.Hour)
	rotated := ring.IdentityPseudonym("42")
	if rotated == before {
		t.Errorf("the digest %q survived an interval that had exactly elapsed", rotated)
	}

	// Well inside the interval that the rotation above started.
	clock = clock.Add(time.Minute)
	if again := ring.IdentityPseudonym("42"); again != rotated {
		t.Errorf("the digest moved again one minute after rotating (%q then %q): the rotation did not restart the interval",
			rotated, again)
	}
}

// TestKeyring_DueOnAKeyThatNeverGenerated_IsNotDue covers the guard that keeps
// a keyring holding no generation instant from rotating.
//
// Its rotatedAt is what the elapsed time is measured from, so a zero one would
// measure from the year zero and report every key as overdue on its first call.
func TestKeyring_DueOnAKeyThatNeverGenerated_IsNotDue(t *testing.T) {
	t.Parallel()

	ring := &Keyring{rotation: time.Hour, now: func() time.Time {
		return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	}}

	if ring.due() {
		t.Error("a keyring with no recorded generation instant reported itself due to rotate")
	}
}

// TestExpand_ReproducesRFC5869TestCase3 holds the derivation to the published
// HKDF-SHA256 vector that uses no salt and no info, which is the shape expand
// calls it in apart from the info string.
//
// expand returns 32 bytes and the vector asks for 42, so what is compared is
// the first 32: HKDF's output blocks are chained, so the first block does not
// depend on how many follow it.
func TestExpand_ReproducesRFC5869TestCase3(t *testing.T) {
	t.Parallel()

	ikm := bytes.Repeat([]byte{0x0b}, 22)
	const okmPrefix = "8da4e775a563c18f715f802a063c5a31b8a11f5c5ee1879ec3454e5f3c738d2d"

	got, err := expand(ikm, "")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if hex.EncodeToString(got) != okmPrefix {
		t.Errorf("expand(RFC 5869 test case 3) = %x, want %s", got, okmPrefix)
	}
}

// TestExpand_AgreesWithThePreviousDerivation holds the standard library's HKDF
// to golang.org/x/crypto/hkdf, which derived these keys until the server
// stopped linking that module, on the inputs expand is actually given.
//
// Agreement is the whole requirement. A configured secret is how several
// replicas, and one deployment across restarts, give a caller one pseudonym;
// a derivation that moved by one bit would split every caller in two on the
// day of the upgrade, and a distinct-user count would double without anybody
// having changed anything. The oracle is the previous code verbatim, so this
// test fails if either side ever stops producing the other's bytes.
func TestExpand_AgreesWithThePreviousDerivation(t *testing.T) {
	t.Parallel()

	for _, info := range []string{identityKeyInfo, resourceKeyInfo, ""} {
		t.Run(info, func(t *testing.T) {
			t.Parallel()

			for _, secret := range derivationInputs() {
				want := make([]byte, identitySaltBytes)
				if _, err := xhkdf.New(sha256.New, secret, nil, []byte(info)).Read(want); err != nil {
					t.Fatalf("the previous derivation failed on %x: %v", secret, err)
				}
				got, err := expand(secret, info)
				if err != nil {
					t.Fatalf("expand(%x): %v", secret, err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("expand(%x, %q) = %x, the previous derivation gave %x", secret, info, got, want)
				}
			}
		})
	}
}

// derivationInputs is what expand is handed in production, in each shape it
// arrives: 32 bytes of randomness for a generated key (drawn deterministically
// here, so a failure names an input it can be rerun on), and an operator's
// secret, which is text of any length, including lengths below HKDF's block
// and hash sizes and above the HMAC block size, where HMAC hashes the key.
func derivationInputs() [][]byte {
	var inputs [][]byte
	seed := sha256.Sum256([]byte("keyring derivation inputs"))
	for range 64 {
		seed = sha256.Sum256(seed[:])
		inputs = append(inputs, bytes.Clone(seed[:]))
	}
	for length := 1; length <= 130; length++ {
		inputs = append(inputs, bytes.Repeat([]byte("s"), length))
	}
	return append(inputs,
		[]byte("a deployment-wide secret"),
		[]byte("clé de pseudonymisation, ünïcödé"),
		bytes.Repeat([]byte{0}, 32),
		bytes.Repeat([]byte("0123456789abcdef"), 64),
	)
}

// TestKeyring_AConfiguredSecretKeepsTheSameKeysAndPseudonyms pins what one
// configured secret produced before the derivation moved to the standard
// library, all the way to the sixteen hex characters a collector stores.
//
// The oracle test above compares the two HKDF implementations; this one fixes
// the chain around them too (the info strings, the key length, the HMAC and
// its truncation), because a pseudonym already sitting in a dashboard depends
// on every one of them. The values were computed by the golang.org/x/crypto
// derivation on the commit before this change and agree with an independent
// HMAC-SHA256 implementation.
func TestKeyring_AConfiguredSecretKeepsTheSameKeysAndPseudonyms(t *testing.T) {
	t.Parallel()

	const secret = "a deployment-wide secret"
	for _, tc := range []struct {
		info string
		key  string
	}{
		{identityKeyInfo, "f16c1f2f65dcdcb85f93f3cc6af7c14e02e9ddcf5be3a23546f1ddf0cb9483bb"},
		{resourceKeyInfo, "f2d8e080372632beeb7559b481273e77e57407e1d07a369aac1c75fc46a4df20"},
	} {
		t.Run(tc.info, func(t *testing.T) {
			t.Parallel()

			got, err := expand([]byte(secret), tc.info)
			if err != nil {
				t.Fatalf("expand: %v", err)
			}
			if hex.EncodeToString(got) != tc.key {
				t.Errorf("the %q key = %x, want %s", tc.info, got, tc.key)
			}
		})
	}

	ring := keyringFrom(t, secret, 0)
	if got, want := ring.IdentityPseudonym("42"), "cd9ba365d6d5556a"; got != want {
		t.Errorf("IdentityPseudonym(42) = %q, want %q", got, want)
	}
	if got, want := ring.ResourcePseudonym("gitlab://project/7"), "fa61d7ea8d9101fa"; got != want {
		t.Errorf("ResourcePseudonym(gitlab://project/7) = %q, want %q", got, want)
	}
}

// fips140HelperEnv marks the copy of this test binary that
// TestNewKeyring_UnderFIPS140Only_RefusesAShortSecretWithAnError starts under
// GODEBUG=fips140=only. It is a marker for the test process and nothing else.
const fips140HelperEnv = "TELEMETRY_KEYRING_FIPS140_HELPER"

// TestNewKeyring_UnderFIPS140Only_RefusesAShortSecretWithAnError covers the
// one refusal HKDF has for an input expand can be given: a secret shorter than
// 112 bits under GODEBUG=fips140=only.
//
// The standard library returns it as an error, which NewKeyring hands to its
// caller and the caller turns into "recording nothing about callers".
// golang.org/x/crypto/hkdf, which derived these keys before, panicked on the
// same input, so a FIPS-only deployment with a short configured secret did not
// start at all. The mode is read once when the process starts, so the
// assertions run in a copy of this test binary started with it set, and the
// copy also shows that a 112-bit secret and a generated key are accepted.
func TestNewKeyring_UnderFIPS140Only_RefusesAShortSecretWithAnError(t *testing.T) {
	if os.Getenv(fips140HelperEnv) == "1" {
		if !fips140.Enforced() {
			t.Fatal("the helper runs without FIPS 140-only enforcement; GODEBUG did not reach it")
		}
		_, short := NewKeyring("13 bytes long", 0)
		if short == nil || !strings.Contains(short.Error(), "112 bits") {
			t.Errorf("NewKeyring with a 104-bit secret under fips140=only = %v, want the refusal naming 112 bits", short)
		}
		if _, enough := NewKeyring("fourteen bytes", 0); enough != nil {
			t.Errorf("NewKeyring refused a 112-bit secret under fips140=only: %v", enough)
		}
		if _, generated := NewKeyring("", 0); generated != nil {
			t.Errorf("NewKeyring refused a generated key under fips140=only: %v", generated)
		}
		return
	}

	t.Parallel()

	// #nosec G204 G702 -- the program is this test binary, started again under a GODEBUG the running process cannot take on
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1", "-test.v")
	// A binary built for coverage writes its counters where GOCOVERDIR says,
	// and warns on its output when nothing says.
	cmd.Env = append(os.Environ(), "GODEBUG=fips140=only", fips140HelperEnv+"=1", "GOCOVERDIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the fips140=only helper failed: %v\n%s", err, out)
	}
	// A pattern that matched nothing would exit 0 too, having asserted
	// nothing, so the helper has to be seen passing by name.
	if !bytes.Contains(out, []byte("--- PASS: "+t.Name())) {
		t.Fatalf("the fips140=only helper did not run the test:\n%s", out)
	}
}

// withReadRandom replaces the randomness seam for the duration of the test.
//
// A test using it or [withDeriveKey] must not run in parallel: the seam is a
// package variable every keyring reads.
func withReadRandom(t *testing.T, read func([]byte) (int, error)) {
	t.Helper()

	previous := readRandom
	readRandom = read
	t.Cleanup(func() { readRandom = previous })
}

// withDeriveKey replaces the HKDF seam for the duration of the test, failing
// every derivation whose info string is refuse and passing the rest through.
func withDeriveKey(t *testing.T, refuse string) {
	t.Helper()

	previous := deriveKey
	deriveKey = func(h func() hash.Hash, secret, salt []byte, info string, length int) ([]byte, error) {
		if info == refuse {
			return nil, errTestDerivation
		}
		return previous(h, secret, salt, info, length)
	}
	t.Cleanup(func() { deriveKey = previous })
}

// errTestDerivation is the failure the seams inject.
var errTestDerivation = errors.New("injected derivation failure")

// TestNewKeyring_AConfiguredSecretThatCannotBeDerived_IsRefused covers each of
// the two derivations failing, since a keyring holding one key and not the
// other would pseudonymize callers while silently recording no resources, or
// the reverse.
func TestNewKeyring_AConfiguredSecretThatCannotBeDerived_IsRefused(t *testing.T) {
	for _, info := range []string{identityKeyInfo, resourceKeyInfo} {
		t.Run(info, func(t *testing.T) {
			withDeriveKey(t, info)

			ring, err := NewKeyring("a deployment-wide secret", 0)
			if !errors.Is(err, errTestDerivation) {
				t.Fatalf("NewKeyring = %v, want the derivation failure", err)
			}
			if ring != nil {
				t.Errorf("NewKeyring returned a keyring beside the error: %+v", ring)
			}
			if !strings.Contains(err.Error(), info) {
				t.Errorf("the error %q does not name the key that failed, %q", err, info)
			}
		})
	}
}

// TestNewKeyring_AGeneratedKeyThatCannotBeMade_IsRefused covers the generated
// path's two failures: no randomness, and randomness HKDF would not take.
func TestNewKeyring_AGeneratedKeyThatCannotBeMade_IsRefused(t *testing.T) {
	t.Run("randomness", func(t *testing.T) {
		withReadRandom(t, func([]byte) (int, error) { return 0, errTestDerivation })

		ring, err := NewKeyring("", 0)
		if !errors.Is(err, errTestDerivation) || ring != nil {
			t.Fatalf("NewKeyring = %v, %v; want no keyring and the randomness failure", ring, err)
		}
		if !strings.Contains(err.Error(), "generating the pseudonymisation key") {
			t.Errorf("the error %q does not say it was generating the key", err)
		}
	})

	t.Run("derivation", func(t *testing.T) {
		withDeriveKey(t, identityKeyInfo)

		ring, err := NewKeyring("", 0)
		if !errors.Is(err, errTestDerivation) || ring != nil {
			t.Fatalf("NewKeyring = %v, %v; want no keyring and the derivation failure", ring, err)
		}
	})
}

// TestKeyring_ARotationThatFails_RecordsNothingUntilOneSucceeds covers the
// rotation branch of digest when the new key cannot be made.
//
// Carrying on with the expired key would hold a pseudonym past the life the
// operator asked for, so the answer is nothing, for both pseudonyms. And the
// failure must not stick: the instant of the last rotation is not moved, so
// the next call is still due and tries again, and a keyring whose randomness
// comes back records again.
func TestKeyring_ARotationThatFails_RecordsNothingUntilOneSucceeds(t *testing.T) {
	ring := keyringFrom(t, "", time.Hour)
	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ring.now = func() time.Time { return clock }
	ring.rotatedAt = clock
	before := ring.IdentityPseudonym("42")

	clock = clock.Add(2 * time.Hour)
	failing := true
	withReadRandom(t, func(b []byte) (int, error) {
		if failing {
			return 0, errTestDerivation
		}
		return rand.Read(b)
	})

	if got := ring.IdentityPseudonym("42"); got != "" {
		t.Errorf("IdentityPseudonym after a failed rotation = %q, want nothing", got)
	}
	if got := ring.ResourcePseudonym("gitlab://project/7"); got != "" {
		t.Errorf("ResourcePseudonym after a failed rotation = %q, want nothing", got)
	}

	failing = false
	after := ring.IdentityPseudonym("42")
	if after == "" || after == before {
		t.Errorf("IdentityPseudonym once randomness returned = %q (before the rotation %q), want a new pseudonym", after, before)
	}
}
