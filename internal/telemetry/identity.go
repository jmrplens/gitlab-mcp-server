package telemetry

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// IdentityPolicy decides what a signal leaving this process may say about who
// made a call.
//
// It is deliberately not a log level. Verbosity is how much is recorded;
// identity is what each record contains, and fusing them fails both ways: at
// WARN the identity would disappear from exactly the records where it matters
// most, a refusal or a throttle or a failure, and at DEBUG it would arrive
// alongside a flood nobody asked for.
//
// It is also not scoped to logs. A span attribute and a metric label carry a
// user id just as easily, so a policy that governed only the log signal would
// leave identity flowing through traces while claiming to be off.
//
// # The rule
//
// What leaves the process is redacted unless the operator says otherwise; what
// stays in the operator's own stderr is not. That boundary is what keeps this
// from quietly undoing [ADR-0008], which put identity into the logs for audit:
// stderr is unchanged for every existing deployment, and identity does not
// cross to a collector until somebody asks for it.
//
// [ADR-0008]: https://jmrp.io/docs/gitlab-mcp-server/development/adr/adr-0008-universal-identity/
type IdentityPolicy string

const (
	// IdentityNone exports nothing about who made a call. The default,
	// because it is the only value that is safe for an operator who has not
	// thought about the question.
	IdentityNone IdentityPolicy = "none"
	// IdentityPseudonymous exports a stable per-user digest and no readable
	// identity. It keeps the one thing a shared endpoint genuinely needs,
	// telling one caller's traffic from another's so a burst can be attributed
	// and a session followed, without naming anyone. It is also the nearest
	// thing this server has to the per-call correlation ADR-0008 records as
	// missing.
	IdentityPseudonymous IdentityPolicy = "pseudonymous"
	// IdentityFull exports the user id and username already present in the
	// stderr log line. What an enterprise auditing its own users wants, and
	// what it is entitled to: those are its employees and its collector.
	IdentityFull IdentityPolicy = "full"
)

// EnvIdentityName is the environment variable that selects the identity policy.
//
// Prefixed, like every variable this server owns, so it cannot collide with
// whatever else a host has exported. The standard OTEL_* names are never
// prefixed, because the SDK reads those and we do not.
const EnvIdentityName = "GITLAB_MCP_TELEMETRY_IDENTITY"

// PolicyDescription says in plain words what a policy exports, for the startup
// log and the server card.
//
// It exists so that the one line an operator reads at startup says what will
// actually leave the process, rather than a mode name they would have to look
// up. "pseudonymous" means nothing on its own; "a per-process digest, no
// readable identity" is the sentence that lets somebody notice a mistake.
func PolicyDescription(policy IdentityPolicy) string {
	switch policy {
	case IdentityFull:
		return "the GitLab user id and username"
	case IdentityPseudonymous:
		return "a per-process digest, with no readable identity"
	case IdentityNone:
		return "nothing about who made a call"
	default:
		return "nothing about who made a call"
	}
}

// DefaultIdentityPolicy is what an operator gets without deciding.
const DefaultIdentityPolicy = IdentityNone

// ParseIdentityPolicy validates an operator-supplied value.
func ParseIdentityPolicy(value string) (IdentityPolicy, error) {
	switch IdentityPolicy(strings.TrimSpace(strings.ToLower(value))) {
	case "":
		return DefaultIdentityPolicy, nil
	case IdentityNone:
		return IdentityNone, nil
	case IdentityPseudonymous:
		return IdentityPseudonymous, nil
	case IdentityFull:
		return IdentityFull, nil
	default:
		return "", fmt.Errorf("unknown identity policy %q: use %q, %q or %q",
			value, IdentityNone, IdentityPseudonymous, IdentityFull)
	}
}

// The slog field names a tool-call record carries on the stderr leg.
//
// Declared here rather than where they are written, because the redactor has to
// find them to apply the policy to the exported copy, and two spellings of the
// same field would mean the policy silently applies to neither.
//
// They are not the user.* names below on purpose: those are the OpenTelemetry
// registry's, for what leaves the process, while these are what an operator
// reads in their own terminal. Reconciling the two is a separate change with a
// dual-emit window, since it breaks whatever parses stderr today.
const (
	LogFieldUser   = "user"
	LogFieldUserID = "user_id"
	// LogFieldTokenSuffix is the masked tail of a client credential the pool
	// and the refusal paths log. It is declared here for the same reason as the
	// two above: the export-side strip list has to find it by name, and it is
	// stripped rather than policy-governed because four characters of a token
	// are a correlation handle and not an identity anyone chose to publish.
	// The name of a field, never a credential: the value it names is four
	// masked characters, and this constant is what removes them.
	LogFieldTokenSuffix = "token_suffix" //nolint:gosec // a log field name, not a secret
)

// identitySaltBytes sizes the pseudonymisation key.
const identitySaltBytes = 32

// Redactor turns an identity into the attributes a signal may carry.
//
// The zero value redacts everything, so a caller that never configured one
// cannot leak by forgetting.
type Redactor struct {
	policy IdentityPolicy
	// keys holds the secret the digest is computed under, and decides how long
	// it lives. Shared with every other pseudonym this process emits, because
	// two keyrings would give one caller two digests that no single signal
	// could show to be the same person. See [Keyring].
	keys *Keyring
}

// NewRedactor builds the redactor for a policy, over the process's keyring.
//
// A nil keyring is accepted and yields no digest, which is the same answer the
// zero value gives: a caller that never wired one records nothing rather than
// emitting something that looks like a pseudonym and is not. Nothing here can
// fail: the policy was validated where it was parsed, and the keyring is
// whatever the caller built, nil included.
func NewRedactor(policy IdentityPolicy, keys *Keyring) *Redactor {
	return &Redactor{policy: policy, keys: keys}
}

// Policy reports what this redactor was built for.
func (r *Redactor) Policy() IdentityPolicy {
	if r == nil || r.policy == "" {
		return DefaultIdentityPolicy
	}
	return r.policy
}

// Attributes returns what may be recorded about a caller.
//
// userID and username are the fields the stderr log line already carries. An
// empty userID means an unauthenticated call and yields nothing under every
// policy: there is no identity to redact or to publish.
func (r *Redactor) Attributes(userID, username string) []attribute.KeyValue {
	if userID == "" {
		return nil
	}
	switch r.Policy() {
	case IdentityFull:
		attrs := []attribute.KeyValue{attribute.String(AttrUserID, userID)}
		if username != "" {
			attrs = append(attrs, attribute.String(AttrUserName, username))
		}
		return attrs
	case IdentityPseudonymous:
		digest := r.pseudonym(userID)
		if digest == "" {
			// A keyring that failed to build yields no digest, and an empty
			// user.hash would read as one anonymous person while meaning "the
			// keyring is broken". Absence is the honest value.
			return nil
		}
		return []attribute.KeyValue{attribute.String(AttrUserHash, digest)}
	default:
		return nil
	}
}

// The registry-defined keys this policy emits.
//
// All three are in the user.* namespace, and that uniformity is the point: the
// same three names appear on a span, on a log record and anywhere else identity
// is recorded, so an operator writes one query rather than three.
//
// The enduser.* namespace was the first choice and is the wrong one. Only two
// attributes live there, enduser.id and enduser.pseudo.id, and the registry
// tags them "contains sensitive (PII) information" and "contains sensitive
// (linkable PII) information" respectively, which is a heavier warning than the
// user.* pair carries for the same values. There is no enduser.name at all: an
// earlier version of this file invented one, which is precisely what the naming
// guidance rules out, since a key in a namespace OpenTelemetry owns can be
// given a different meaning by a future release.
const (
	// AttrUserID is "Unique identifier of the user". The GitLab numeric id.
	AttrUserID = "user.id"

	// AttrUserName is "Short name or login/username of the user".
	AttrUserName = "user.name"

	// AttrUserHash is "Unique user hash to correlate information for a user in
	// anonymized form", to be used "when user.id or user.name contain
	// confidential data". That is this policy's pseudonymous mode exactly.
	//
	// One objection to this key deserves an answer rather than a dismissal: a
	// hash over GitLab's numeric ids is reversible by enumeration, because the
	// input space is small and predictable. That is true of a plain digest and
	// false of what [Redactor.pseudonym] computes, which is an HMAC under a
	// 32-byte key generated per process and never written down. Without the key
	// there is nothing to enumerate against. What remains is inherent to
	// pseudonymity rather than to the construction: somebody who can correlate a
	// known user's activity with a digest can link the two, which is why the
	// mode is called pseudonymous and not anonymous.
	AttrUserHash = "user.hash"
)

// pseudonym derives the stable per-process digest for a user id.
//
// HMAC rather than a plain hash of salt and id concatenated: the construction
// is the one designed for keyed digests, and it removes any question about
// length-extension or about a salt boundary an attacker could straddle.
// Truncated to 16 hex characters, which is 64 bits: far past collision concern
// for the number of users one deployment sees, and short enough to read in a
// trace viewer.
func (r *Redactor) pseudonym(userID string) string {
	return r.keys.IdentityPseudonym(userID)
}
