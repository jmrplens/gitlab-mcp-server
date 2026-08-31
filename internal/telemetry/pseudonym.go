package telemetry

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// resourceSalt keys [ResourcePseudonym]. It is generated once per process and
// never written anywhere, for the same reason the identity salt is: a digest of
// a small predictable input is not a pseudonym, because every candidate can be
// hashed and compared in a moment. A GitLab project id is a low integer, so an
// unkeyed hash of "gitlab://project/82077663" is reversible by anyone who can
// count.
//
// Per process rather than persisted, again for the identity reason: a pseudonym
// that survives restarts is a durable identifier, which is the thing the mode
// exists to avoid.
var (
	resourceSaltOnce sync.Once
	resourceSalt     []byte
)

// ResourcePseudonym returns a stable per-process digest of a resource URI.
//
// # Why a digest rather than the URI
//
// This server's documented position is that resource URIs are never exported,
// because they embed project and group identifiers, and it holds that position
// even against the MCP semantic convention, which marks mcp.resource.uri
// Conditionally Required on a resources/read span. Declining it there and then
// writing the same value onto a subscription poll span would be the same
// disclosure through a different code path, and a worse one: a read happens
// once, while a poll repeats for the life of the watch, so one subscription
// would write a project id into a telemetry backend hundreds of times.
//
// # Why a digest rather than nothing
//
// Dropping the attribute entirely would leave an operator unable to tell two
// watchers of the same kind apart, so "one subscription is failing every poll"
// and "every subscription is failing" would look identical. The digest keeps
// that distinction and the correlation across a watcher's lifetime while naming
// nothing, which is exactly the trade the pseudonymous identity policy already
// makes for users.
//
// What remains is inherent to pseudonymity rather than to this construction:
// somebody who can correlate a known subscription with a digest can link the
// two. Sixteen hex characters is 64 bits, past collision concern for the number
// of watchers one deployment holds and short enough to read in a trace viewer.
func ResourcePseudonym(uri string) string {
	if uri == "" {
		return ""
	}
	resourceSaltOnce.Do(func() {
		resourceSalt = make([]byte, identitySaltBytes)
		if _, err := rand.Read(resourceSalt); err != nil {
			// A process that cannot read randomness cannot pseudonymize, and
			// falling back to an unkeyed digest would be worse than recording
			// nothing: it would look like a pseudonym while being reversible.
			resourceSalt = nil
		}
	})
	if resourceSalt == nil {
		return ""
	}

	mac := hmac.New(sha256.New, resourceSalt)
	_, _ = mac.Write([]byte(uri))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}
