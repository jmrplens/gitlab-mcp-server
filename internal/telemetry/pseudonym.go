package telemetry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// This file is the computation. Where the keys come from and how long they
// live is [Keyring], in keyring.go.

// IdentityPseudonym returns the digest naming a caller.
func (k *Keyring) IdentityPseudonym(userID string) string {
	return k.digest(userID, false)
}

// ResourcePseudonym returns the digest naming a resource URI.
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
// two.
func (k *Keyring) ResourcePseudonym(uri string) string {
	return k.digest(uri, true)
}

// digest is the one computation, so the two pseudonyms cannot drift apart in
// length, algorithm, or rotation behavior.
//
// Sixteen hex characters is 64 bits: past collision concern for the number of
// callers or watchers one deployment holds, and short enough to read in a
// trace viewer.
func (k *Keyring) digest(value string, forResource bool) string {
	if k == nil || value == "" {
		return ""
	}

	k.mu.Lock()
	if k.due() {
		if err := k.generate(); err != nil {
			// A process that cannot read randomness cannot pseudonymize, and
			// continuing with the expired key would hold a pseudonym past the
			// life the operator asked for. Emitting nothing is the honest
			// answer; falling back to an unkeyed digest would look like a
			// pseudonym while being reversible by anyone.
			k.identity, k.resource = nil, nil
		}
	}
	key := k.identity
	if forResource {
		key = k.resource
	}
	k.mu.Unlock()

	if key == nil {
		return ""
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}
