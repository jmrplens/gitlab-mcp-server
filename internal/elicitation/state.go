// Package elicitation: state.go signs and binds the opaque RequestState that
// carries a multi round-trip flow's accumulated answers between rounds.
package elicitation

import (
	"bytes"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// stateTTL is how long a request state stays usable.
//
// It bounds replay without getting in the way of a person answering a prompt:
// the flows this carries are a handful of questions, and a client that takes
// longer re-prompts rather than failing.
const stateTTL = 10 * time.Minute

// stateKey signs request state for the life of this process.
//
// A fresh random key per process is deliberate. There is nothing to persist:
// state older than [stateTTL] is refused anyway, so surviving a restart would
// buy nothing and would need somewhere safe to keep a secret. In HTTP mode the
// server pool builds one server per token and URL, so what this key protects is
// a boundary the pool has already drawn.
//
// What it deliberately does not carry is a principal claim. The specification
// asks that state be bound to "the user session or principal", and this package
// never sees one: under the stateless transport that MRTR exists for, every
// POST is its own session with its own ID, so a session identifier would change
// between the rounds of a single flow and break every legitimate one. The
// binding that is available and does hold is [requestDigest], which is what
// stopped the cross-tool reuse that was actually reproducible.
var stateKey = newStateKey()

func newStateKey() []byte {
	key := make([]byte, 32)
	if _, err := cryptorand.Read(key); err != nil {
		// crypto/rand.Read does not fail on any supported platform; if the
		// system entropy source is broken there is no safe way to continue
		// signing.
		panic(fmt.Sprintf("elicitation: cannot generate a request-state key: %v", err))
	}
	return key
}

// errStateTampered reports state that this server did not issue.
var errStateTampered = errors.New("elicitation: requestState failed its integrity check")

// signedState is the JSON payload that gets signed. It is flowState plus the
// two claims that bind it to a moment and to a call.
type signedState struct {
	Version   int                     `json:"v"`
	Answers   map[string]answerRecord `json:"answers,omitempty"`
	ExpiresAt int64                   `json:"exp"`
	Request   string                  `json:"req"`
}

// requestDigest identifies the call a state belongs to.
//
// The arguments are canonicalized rather than hashed as received: a client is
// free to re-serialize them between rounds, and key order in a JSON object
// carries no meaning, so hashing the bytes would break flows that are behaving
// correctly. Decoding and re-encoding through Go's map marshaling sorts the
// keys, which is enough to make the two rounds of one call agree.
//
// Malformed arguments hash as themselves; there is nothing to canonicalize and
// the call is going to fail its schema check anyway.
func requestDigest(toolName string, args json.RawMessage) string {
	canonical := args
	// UseNumber keeps each number as the text the client sent, because decoding
	// into plain any makes every number a float64 and two identifiers above 2^53
	// would then share a digest — 9007199254740992 and 9007199254740993 are the
	// same float64, which is the collision this binding exists to prevent.
	//
	// The text is then normalized by canonicalizeNumbers, because keeping it
	// raw trades that collision for the opposite problem: 1 and 1.0 are the
	// same number written two ways, and a client that re-serializes its own
	// arguments would lose its pending answers. Python writes 1.0 where Go
	// writes 1, so this is a difference between ordinary clients, not an
	// exotic case.
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.UseNumber()
	var decoded any
	if decoder.Decode(&decoded) == nil {
		if reencoded, marshalErr := json.Marshal(canonicalizeNumbers(decoded)); marshalErr == nil {
			canonical = reencoded
		}
	}
	sum := sha256.Sum256(append([]byte(toolName+"\x00"), canonical...))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// encodeState returns the signed, bound wire form of a flow's answers.
//
// The form is v<version>.<payload>.<mac>, so the version is readable before
// anything else is trusted and a future encoding can be told apart from this
// one without guessing.
func encodeState(answers map[string]answerRecord, digest string, now time.Time) (string, error) {
	payload, err := json.Marshal(signedState{
		Version:   flowStateVersion,
		Answers:   answers,
		ExpiresAt: now.Add(stateTTL).Unix(),
		Request:   digest,
	})
	if err != nil {
		return "", fmt.Errorf("elicitation: failed to encode request state: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return fmt.Sprintf("v%d.%s.%s", flowStateVersion, body, signPayload(body)), nil
}

func signPayload(body string) string {
	mac := hmac.New(sha256.New, stateKey)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// decodeState verifies a client-echoed state and returns the answers it is
// entitled to carry.
//
// Two outcomes are not errors. State that has expired, and state that belongs
// to a different call, both yield no answers and no error: the flow simply
// asks its questions again, which is the right thing to do when a user's
// earlier answers cannot be applied. Only state this server did not issue is an
// error, because at that point nothing about it can be believed.
//
// "Servers MUST validate the integrity of the state on each round trip." The
// state used to be plain JSON with no MAC at all, so a client could hand back
// a document it wrote itself. That mattered because the state carries business
// logic: a forged one made an unconfirmed destructive action proceed as though
// it had been confirmed. It is worth being precise about the size of that,
// though: the confirmation gate is an agent-facing guard, not an authorization
// boundary. The same delete succeeds in one call with confirm=true, a
// documented parameter, so forging state let a client reach nothing it could
// not already reach deliberately. What it broke was the integrity requirement
// and the meaning of the gate, not GitLab's own permissions.
func decodeState(raw, digest string, now time.Time) (map[string]answerRecord, error) {
	prefix, body, mac, ok := splitState(raw)
	if !ok {
		return nil, fmt.Errorf("%w: malformed", errStateTampered)
	}
	if prefix != fmt.Sprintf("v%d", flowStateVersion) {
		return nil, fmt.Errorf("elicitation: unsupported requestState version %q", prefix)
	}
	if subtle.ConstantTimeCompare([]byte(mac), []byte(signPayload(body))) != 1 {
		return nil, errStateTampered
	}

	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, fmt.Errorf("%w: undecodable payload", errStateTampered)
	}
	var st signedState
	if unmarshalErr := json.Unmarshal(payload, &st); unmarshalErr != nil {
		// Signed by this server and still unreadable: a bug here, not a client.
		return nil, fmt.Errorf("elicitation: invalid requestState: %w", unmarshalErr)
	}

	// An empty map rather than nil: no answers is a result, not an absence.
	if st.ExpiresAt <= now.Unix() {
		return map[string]answerRecord{}, nil
	}
	if st.Request != digest {
		// Answers given to one call, presented on another. Reproducible
		// without any tampering: state issued for one wizard was accepted by a
		// different one, which silently adopted answers meant for the first.
		return map[string]answerRecord{}, nil
	}
	return st.Answers, nil
}

// splitState splits the wire form into its three parts.
func splitState(raw string) (prefix, body, mac string, ok bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// numberMarker prefixes a canonicalized number so it cannot collide with a
// string that happens to look like one: without it, {"a":1} and {"a":"1"} would
// hash alike.
const numberMarker = "\x00num:"

// stringMarker prefixes every string for the same reason in the other
// direction: JSON strings may contain NUL, so a crafted string equal to a
// marked number would otherwise canonicalize identically to the number and two
// different argument sets would share a digest.
const stringMarker = "\x00str:"

// canonicalizeNumbers rewrites every json.Number in a decoded document into one
// exact textual form, leaving everything else untouched.
//
// Two numbers that are equal must hash alike however they were written, and two
// that differ must not — including beyond float64's range. big.Rat gives both:
// it is exact for any JSON number, so 1, 1.0 and 1e0 all become "1", 1.5
// becomes "3/2", and 9007199254740992 stays distinct from its successor.
//
// The result is a hashing key rather than a document, so it does not need to be
// valid JSON on its own; RatString is used because it is canonical, not because
// anyone reads it.
//
// A number big.Rat cannot parse is left as its original text. JSON's grammar
// admits none, so this is only reachable for input that was going to fail
// validation anyway, and passing it through unchanged is better than dropping
// it from the digest.
func canonicalizeNumbers(v any) any {
	switch value := v.(type) {
	case string:
		return stringMarker + value
	case json.Number:
		if rat, ok := new(big.Rat).SetString(value.String()); ok {
			return numberMarker + rat.RatString()
		}
		return numberMarker + value.String()
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[k] = canonicalizeNumbers(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = canonicalizeNumbers(item)
		}
		return out
	default:
		return v
	}
}
