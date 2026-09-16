//go:build e2e

// digest.go is how a row can say which schemas a provider actually saw.
//
// Two of the four adapters this replaces rewrote the dynamic execute schema on
// their way out, injecting thirty-six parameter names for their provider alone.
// Nothing recorded that, so two columns of a published cross-vendor table were
// measuring a surface the other two never received and the table read as a
// comparison of models. A digest per provider is the smallest thing that makes
// such a rewrite impossible to hide: the tool list goes in once, each adapter
// hashes the names, descriptions and schemas it is about to send, and four
// digests that should agree and do not are a defect with a name on it.
//
// The hash is taken over the tool content and never over the provider's
// envelope. Anthropic sends input_schema, OpenAI a function object and Gemini a
// function declaration, and folding that in would make all four digests differ
// for reasons that are not about what the model was shown.

package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// digestLength is how much of the hash a digest carries.
//
// Sixteen hex characters is sixty-four bits, which is what the repository's
// other comparison fingerprints use: this is read by a person comparing two
// rows of a table, not by anything defending against a chosen collision.
const digestLength = 16

// toolEntry is one tool as an adapter is about to send it, in the one shape
// every adapter's digest is taken over.
type toolEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

// digestTools returns the digest of a tool list, in the order it is sent.
//
// Order is part of it because order is part of what the model was shown: a
// surface that lists two hundred tools in a different sequence is a different
// prompt, and a digest that ignored that would call two runs comparable when
// they are not.
func digestTools(entries []toolEntry) string {
	encoded, err := json.Marshal(entries)
	if err != nil {
		// Every field here is a string or a json.RawMessage this package
		// produced, so the only way to arrive is a raw message holding
		// invalid JSON, which NewTool refuses. Hashing the error keeps the
		// function total and makes such a digest differ from every real one
		// rather than colliding with the empty list.
		return digestBytes([]byte("undigestable: " + err.Error()))
	}
	return digestBytes(encoded)
}

// digestBytes is the one hash this package takes.
func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:digestLength]
}
