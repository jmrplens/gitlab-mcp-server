package telemetry

import (
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

const (
	// AttrResourceURI is the MCP convention's own key, Conditionally Required
	// on a span "when the client executes a request type that includes a
	// resource URI parameter". It carries the URI exactly, so it is only ever
	// set under [IdentityFull].
	AttrResourceURI = "mcp.resource.uri"

	// AttrResourceRef is this server's key for the same resource named
	// indirectly. It is deliberately not the convention's key: a consumer
	// reading mcp.resource.uri is entitled to find a URI there, and putting a
	// digest under that name would be a lie told in the schema.
	AttrResourceRef = "gitlab_mcp.resource.ref"
)

// ResourceRedactor decides how much a span may say about which resource a
// request named.
//
// # Why this follows the identity policy
//
// A resource URI in this server embeds project and group identifiers, so it
// says what a caller is working on. That is the same class of disclosure as
// saying who the caller is, and an operator has already made exactly that
// decision once by choosing an identity policy. Asking them to make it twice,
// through a second flag with its own defaults, would produce deployments where
// the two disagree and nobody meant them to.
//
// So the mapping is the one the policy already implies:
//
//	none          a keyed digest: correlates polls and reads of one resource,
//	              names none of them
//	pseudonymous  the same digest, for the same reason it gives a user one
//	full          the URI itself, beside the user.id and user.name that policy
//	              already records
//
// The digest is the floor rather than nothing, which is where this differs from
// the identity treatment, and the difference is deliberate. A user digest still
// follows a person, so recording none is a meaningful default. A resource digest
// follows a thing the operator's own server was asked to watch, and without it
// "one subscription is failing every poll" and "every subscription is failing"
// are the same picture.
type ResourceRedactor struct {
	policy IdentityPolicy
	// keys is the same keyring the identity redactor uses, so one operator
	// secret governs both pseudonyms and neither can outlive the other.
	keys *Keyring
}

// NewResourceRedactor builds the redactor for a policy.
func NewResourceRedactor(policy IdentityPolicy, keys *Keyring) *ResourceRedactor {
	return &ResourceRedactor{policy: policy, keys: keys}
}

// ResourceAttributes returns what may be recorded about one resource URI.
//
// A nil receiver returns nothing, so a caller that never wired one records
// nothing rather than panicking, and an empty URI returns nothing, so an absent
// value is absent rather than a digest of the empty string that would look like
// a real resource.
func (r *ResourceRedactor) ResourceAttributes(uri string) []attribute.KeyValue {
	if r == nil || uri == "" {
		return nil
	}
	if r.policy == IdentityFull {
		return []attribute.KeyValue{attribute.String(AttrResourceURI, uri)}
	}
	digest := r.keys.ResourcePseudonym(uri)
	if digest == "" {
		return nil
	}
	return []attribute.KeyValue{attribute.String(AttrResourceRef, digest)}
}

// resourceURIScheme is the whole vocabulary of resources this server exposes,
// which is what makes matching them a scheme check rather than a guess.
const resourceURIScheme = "gitlab://"

// resourceURIPattern matches a resource URI wherever it appears in free text.
//
// The run stops at whitespace or a quote, which is how these appear when a
// message is assembled with %s, %w or a structured attribute.
var resourceURIPattern = regexp.MustCompile(resourceURIScheme + `[^\s"']+`)

// RedactResourceURIs replaces resource URIs with a marker.
//
// Exported because two places need it and neither can import the other: the
// span status description in internal/mcpotel, which imports the OpenTelemetry
// API and never the SDK, and the log handler here, which is built on the SDK.
// A test in internal/mcpotel asserts the two agree, since a rule stated twice is
// a rule that drifts.
func RedactResourceURIs(text string) string {
	if !strings.Contains(text, resourceURIScheme) {
		return text
	}
	return resourceURIPattern.ReplaceAllString(text, resourceURIScheme+"[redacted]")
}
