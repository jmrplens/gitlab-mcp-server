package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// TestResourcePseudonym_DoesNotContainTheURI is the assertion the whole helper
// exists for.
//
// A subscription poll span used to carry gitlab://project/82077663 verbatim,
// found by subscribing on the hosted deployment and reading the collector. That
// contradicted this server's documented position, which declines resource URIs
// even where the MCP convention marks one Conditionally Required, and it did so
// on the worst possible span: a poll repeats for the life of the watch, so one
// subscription wrote a project id into a backend hundreds of times.
func TestResourcePseudonym_DoesNotContainTheURI(t *testing.T) {
	keys := testKeyring(t)

	const uri = "gitlab://project/82077663/mr/42"

	digest := keys.ResourcePseudonym(uri)
	if digest == "" {
		t.Fatal("no digest was produced")
	}

	// Only fragments long enough that a chance match is not credible. A digest
	// is 16 hex characters, so a two-character numeric fragment such as the "42"
	// in this URI appears in it by coincidence about one run in twenty: an
	// assertion including one fails periodically and says nothing when it does.
	// CI found that before this comment existed.
	//
	// The distinctive part is the project id, and eight hex characters landing
	// in the right order by chance is around one in four billion per position.
	for _, fragment := range []string{"gitlab", "project", "82077663"} {
		t.Run(fragment, func(t *testing.T) {
			if strings.Contains(digest, fragment) {
				t.Errorf("digest %q contains %q from the URI", digest, fragment)
			}
		})
	}
	if strings.Contains(uri, digest) {
		t.Errorf("digest %q is a substring of the URI it stands for", digest)
	}
}

// TestResourcePseudonym_IsStableWithinAProcess pins the property that makes the
// digest useful rather than merely safe: one watcher's polls have to be
// recognizable as one watcher's polls, or an operator cannot tell a single
// failing subscription from every subscription failing.
func TestResourcePseudonym_IsStableWithinAProcess(t *testing.T) {
	keys := testKeyring(t)

	const uri = "gitlab://project/82077663"

	if first, second := keys.ResourcePseudonym(uri), keys.ResourcePseudonym(uri); first != second {
		t.Errorf("the same URI produced %q and %q; polls of one watcher would not correlate", first, second)
	}
}

// TestResourcePseudonym_DistinguishesResources is the other half: two watchers
// must not collapse into one label, or the digest would hide exactly the
// distinction it was added to preserve.
func TestResourcePseudonym_DistinguishesResources(t *testing.T) {
	keys := testKeyring(t)

	a := keys.ResourcePseudonym("gitlab://project/1")
	b := keys.ResourcePseudonym("gitlab://project/2")
	if a == b {
		t.Errorf("two URIs produced the same digest %q", a)
	}
}

// TestResourcePseudonym_EmptyURIProducesNothing keeps an absent value from being
// recorded as a digest of the empty string, which would be a constant that looks
// like a real watcher.
func TestResourcePseudonym_EmptyURIProducesNothing(t *testing.T) {
	keys := testKeyring(t)

	if got := keys.ResourcePseudonym(""); got != "" {
		t.Errorf("keys.ResourcePseudonym(\"\") = %q, want the empty string", got)
	}
}

// TestResourcePseudonym_IsKeyedNotPlain guards the property that makes this a
// pseudonym rather than an obfuscation.
//
// A project id is a low integer, so an unkeyed digest of a URI containing one is
// reversible by anyone willing to hash the candidates. The test cannot see the
// salt, so it asserts the observable consequence: the digest is not the plain
// SHA-256 of the URI, which is what an implementation without a key would emit.
//
// The comparison value is computed here rather than pasted, so it stays true
// whatever the URI in this test becomes.
func TestResourcePseudonym_IsKeyedNotPlain(t *testing.T) {
	keys := testKeyring(t)

	const uri = "gitlab://project/1"

	sum := sha256.Sum256([]byte(uri))

	if keys.ResourcePseudonym(uri) == hex.EncodeToString(sum[:])[:16] {
		t.Error("the digest equals an unkeyed hash of the URI, so it is enumerable rather than pseudonymous")
	}
}

// TestIdentityPseudonym_AcrossARotation_StillAnswers covers what the digest
// must keep doing while the key underneath it is being replaced.
//
// The rotation branch nils both keys when it cannot read randomness, which is
// the honest answer to a process that can no longer pseudonymize: emitting
// nothing beats emitting something reversible. It is also, applied on a
// rotation that worked, a silent end to identity recording — the caller sees an
// empty attribute and no error, and a distinct-user count goes to zero looking
// like a deployment nobody used. Both pseudonyms are asserted because the
// branch nils them together and one of them would otherwise prove nothing about
// the other.
func TestIdentityPseudonym_AcrossARotation_StillAnswers(t *testing.T) {
	t.Parallel()

	ring, err := NewKeyring("", time.Hour)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}

	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ring.now = func() time.Time { return clock }
	ring.rotatedAt = clock

	before := ring.IdentityPseudonym("42")
	if before == "" {
		t.Fatal("a fresh keyring produced no pseudonym")
	}

	clock = clock.Add(2 * time.Hour)

	identity := ring.IdentityPseudonym("42")
	if identity == "" {
		t.Error("the identity pseudonym is empty after a rotation, so identity recording stopped rather than rotated")
	}
	if identity == before {
		t.Errorf("the identity pseudonym %q did not change across the rotation", identity)
	}
	if resource := ring.ResourcePseudonym("gitlab://project/42"); resource == "" {
		t.Error("the resource pseudonym is empty after a rotation, so resource recording stopped rather than rotated")
	}
}

// TestKeyring_WithoutDerivedKeys_RecordsNothing covers the last guard before
// the HMAC, on the one keyring that can reach it: a value whose keys were
// dropped.
//
// Falling back to an unkeyed digest there would look like a pseudonym while
// being reversible by anyone holding a list of user ids, which is the one
// outcome this package must never produce.
func TestKeyring_WithoutDerivedKeys_RecordsNothing(t *testing.T) {
	t.Parallel()

	var ring Keyring

	if got := ring.IdentityPseudonym("42"); got != "" {
		t.Errorf("IdentityPseudonym on a keyring holding no keys = %q, want nothing", got)
	}
	if got := ring.ResourcePseudonym("gitlab://project/42"); got != "" {
		t.Errorf("ResourcePseudonym on a keyring holding no keys = %q, want nothing", got)
	}
}

// testKeyring builds a generated keyring, which is what these tests assume:
// keys that exist, do not rotate, and are shared by everything the process
// pseudonymizes.
func testKeyring(t *testing.T) *Keyring {
	t.Helper()

	ring, err := NewKeyring("", 0)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	return ring
}
