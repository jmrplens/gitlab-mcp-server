package telemetry

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
)

// Keyring owns the secrets that turn an identifier into a pseudonym, and
// decides how long each one lives.
//
// # Why this is one object rather than two package variables
//
// Every pseudonym this server emits has to come from the same decision. Two
// independently generated secrets would give the same person one user.hash on
// a span and a different one on a log record, which reads as two people while
// each signal looks correct on its own. A hosted deployment produced exactly
// that, from a redactor built once per pooled server rather than once per
// process, and no single signal could show it.
//
// # The two lifetimes, and why the operator picks
//
// Nothing prescribes an answer here. The OpenTelemetry registry defines
// user.hash as a value "to correlate information for a user in anonymized
// form" and says nothing about how it is computed or how long it should hold;
// neither the specification nor ENISA offers guidance on the lifetime of a
// pseudonymisation secret. What the field does show is two coherent designs,
// and Matomo ships both at once: an installation-wide salt that never rotates
// where a pseudonym must persist, and a seed discarded every day where it must
// not.
//
// So this offers both, and the choice is the operator's:
//
//   - A configured secret. Every replica derives the same keys from it, so a
//     caller carries one pseudonym across the whole deployment and across
//     restarts. It never rotates here, because a key the operator supplied is
//     theirs to rotate, on their schedule, from outside.
//
//   - No secret. Keys are generated at startup from crypto/rand and never
//     written anywhere, which is what a single instance wants: nothing to
//     store, nothing to leak, and a pseudonym that dies with the process. A
//     rotation interval bounds it further.
//
// The EDPB calls the persistent form a person pseudonym, notes that it
// "requires long-term storage of the pseudonymisation secrets", and warns that
// "the risk of unauthorised attribution is comparatively high". That is the
// cost of the correlation, stated rather than hidden: with the key in hand,
// recovering a GitLab user id from a digest is an enumeration of eight-digit
// integers, which is about two minutes on one core.
//
// # Why HKDF rather than using the secret directly
//
// Two reasons, both cheap. The operator's secret is never used as a key
// itself, so a value reused elsewhere does not become an HMAC key here. And
// one supplied secret yields two independent keys, so a digest of a user and a
// digest of a resource cannot be compared against each other.
type Keyring struct {
	mu       sync.Mutex
	identity []byte
	resource []byte

	// configured records that the keys came from the operator. Rotation does
	// not apply to them: rotating a key somebody else supplied would destroy
	// the correlation they configured it for.
	configured bool

	// rotation is how long generated keys live. Zero means the life of the
	// process, which is the default.
	rotation  time.Duration
	rotatedAt time.Time

	// now is injectable so a test can age a keyring without sleeping.
	now func() time.Time
}

// The HKDF info strings. Distinct, so one secret yields two keys that say
// nothing about each other, and versioned, so changing the derivation later is
// a visible decision rather than a silent renumbering.
const (
	identityKeyInfo = "gitlab-mcp-server telemetry identity pseudonym v1"
	resourceKeyInfo = "gitlab-mcp-server telemetry resource pseudonym v1"
)

// EnvIdentityKeyName is the environment variable holding the operator's
// pseudonymisation secret.
//
// It is a secret in the GDPR sense: Article 4(5) calls it the "additional
// information" that allows attribution, and the EDPB says controllers "need to
// keep them separately and subject them to technical and organisational
// measures that ensure their confidentiality". Read from the environment
// because that is where this project's other secrets come from, with the same
// caveat every environment secret carries: it is visible to anything that can
// read the process environment.
const EnvIdentityKeyName = "GITLAB_MCP_TELEMETRY_IDENTITY_KEY"

// EnvIdentityRotationName is the environment variable holding the lifetime of
// a generated key.
const EnvIdentityRotationName = "GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION"

// DefaultKeyRotation is how long a generated key lives without a setting.
//
// Zero, meaning the life of the process, which is what this server did before
// the interval existed. Rotating by default would silently make the multi
// replica case worse than it is: replicas start at different moments, so they
// would rotate out of phase, and a count of distinct users would churn without
// anybody asking for it. Rotation is coherent on one instance and is opt-in
// for that reason.
const DefaultKeyRotation = time.Duration(0)

// MaxKeyRotation bounds the interval, for the same reason every other duration
// this server accepts is bounded: a value with a typo in it should be refused
// at startup rather than discovered a month later.
const MaxKeyRotation = 30 * 24 * time.Hour

// NewKeyring builds the keyring for a secret and a rotation interval.
//
// An empty secret generates keys instead. A rotation interval is honored only
// for generated keys; with a secret it is ignored, and the caller is expected
// to say so where an operator will read it.
func NewKeyring(secret string, rotation time.Duration) (*Keyring, error) {
	// The configured branch first, and before the rotation is judged: rotation
	// does not apply to a configured key, and refusing the pair would let a
	// setting that is documented as ignored veto the one the operator actually
	// relies on.
	if secret != "" {
		ring := &Keyring{configured: true, now: time.Now}
		if err := ring.derive([]byte(secret)); err != nil {
			return nil, err
		}
		return ring, nil
	}

	if rotation < 0 || rotation > MaxKeyRotation {
		return nil, fmt.Errorf("telemetry identity key rotation %s is out of range (0 disables it, maximum %s)",
			rotation, MaxKeyRotation)
	}

	ring := &Keyring{rotation: rotation, now: time.Now}

	if err := ring.generate(); err != nil {
		return nil, err
	}
	return ring, nil
}

// Configured reports whether the keys came from the operator.
func (k *Keyring) Configured() bool {
	if k == nil {
		return false
	}
	return k.configured
}

// Rotation reports the interval a generated key lives for, or zero.
func (k *Keyring) Rotation() time.Duration {
	if k == nil {
		return 0
	}
	return k.rotation
}

// due reports whether a generated key has outlived its interval. Called under
// the lock.
func (k *Keyring) due() bool {
	if k.configured || k.rotation <= 0 || k.rotatedAt.IsZero() {
		return false
	}
	return k.now().Sub(k.rotatedAt) >= k.rotation
}

// generate replaces both keys with fresh randomness. Called under the lock, or
// during construction before the keyring is shared.
func (k *Keyring) generate() error {
	root := make([]byte, identitySaltBytes)
	if _, err := rand.Read(root); err != nil {
		return fmt.Errorf("generating the pseudonymisation key: %w", err)
	}
	if err := k.derive(root); err != nil {
		return err
	}
	k.rotatedAt = k.now()
	return nil
}

// derive expands one secret into the two keys. Called under the lock, or
// during construction.
func (k *Keyring) derive(secret []byte) error {
	identity, err := expand(secret, identityKeyInfo)
	if err != nil {
		return err
	}
	resource, err := expand(secret, resourceKeyInfo)
	if err != nil {
		return err
	}
	k.identity, k.resource = identity, resource
	return nil
}

// expand runs HKDF-SHA256 over a secret for one purpose.
//
// No salt argument: HKDF's salt is optional and its purpose is to add entropy
// the input may lack, which matters for a key exchange and not here, where the
// input is either 32 bytes of crypto/rand or an operator's secret. The info
// string is what separates the two outputs, and that is the property being
// bought.
func expand(secret []byte, info string) ([]byte, error) {
	key := make([]byte, identitySaltBytes)
	if _, err := hkdf.New(sha256.New, secret, nil, []byte(info)).Read(key); err != nil {
		return nil, fmt.Errorf("deriving the %q key: %w", info, err)
	}
	return key, nil
}
