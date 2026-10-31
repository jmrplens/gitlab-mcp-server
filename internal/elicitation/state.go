// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// state.go signs and binds the opaque RequestState that
// carries a multi round-trip flow's accumulated answers between rounds: to the
// call it was issued for, to the principal the call ran as, and to an expiry.

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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// stateTTL is how long a request state stays usable.
//
// It bounds replay without getting in the way of a person answering a prompt:
// the flows this carries are a handful of questions, and a client that takes
// longer re-prompts rather than failing.
const stateTTL = tenancy.RequestStateTTL // register row IDN-011

// stateKey signs request state for the life of this process.
//
// A fresh random key per process is deliberate. There is nothing to persist:
// state older than [stateTTL] is refused anyway, so surviving a restart would
// buy nothing and would need somewhere safe to keep a secret.
//
// The key by itself draws no boundary between callers. In HTTP mode one
// process serves every credential in its pool, and since ADR-0020 the
// credentials of one configuration shape share one server, each request bound
// to its own credential, so the key one caller's state is signed with is every
// other caller's too. What confines a state to its caller is the principal
// binding beside the call binding (issue 953, register row IDN-011): the
// specification asks that state be bound to "the user session or principal",
// and a session would not do, because under the stateless transport MRTR
// exists for every POST is its own session with its own ID, which would change
// between the rounds of a single flow and break every legitimate one. The
// principal is what [WithPrincipal] put on the request, the pool entry's owner
// token on the HTTP server, which every round of one flow shares and no other
// credential does. An entry the pool rebuilds, once its credential reaches the
// age ceiling or after an eviction, has a new owner token, so a flow in
// progress across the rebuild is asked its questions again, which is the
// answer to any state that cannot be used.
var stateKey = newStateKey()

// principalKeyLabel opens the input [principalSigningKey] derives a key from.
// Its NUL keeps that input apart from every text [sealPayload] signs under the
// same key, which is base64url and dots and never holds one.
const principalKeyLabel = "gitlab-mcp-server request state principal\x00"

// newStateKey draws the key. crypto/rand.Read never returns an error since Go
// 1.24: a broken entropy source crashes the program, which is the one safe
// answer for a key that signs, so there is no failure left to handle here.
func newStateKey() []byte {
	key := make([]byte, 32)
	_, _ = cryptorand.Read(key)
	return key
}

// errStateTampered reports state that this server did not issue.
var errStateTampered = errors.New("elicitation: requestState failed its integrity check")

// signedState is the JSON payload that gets signed. It is flowState plus the
// two claims that bind it to a moment and to a call. The third binding, to the
// principal, is not a claim: it is a MAC beside the payload, so the payload
// names nobody (see [encodeState]).
type signedState struct {
	Version   int                     `json:"v"`
	Answers   map[string]answerRecord `json:"answers,omitempty"`
	ExpiresAt int64                   `json:"exp"`
	Request   string                  `json:"req"`
}

// stateBinding is what a request state is bound to besides its expiry.
type stateBinding struct {
	// request is the [requestDigest] of the call the state belongs to.
	request string
	// principal is who the call ran as, read from the request's context with
	// [PrincipalFrom]: the pool entry's owner token on the HTTP server, and
	// empty on stdio and on a request nothing bound to a credential.
	principal string
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
		if reencoded, marshalErr := canonicalJSON(canonicalizeNumbers(decoded)); marshalErr == nil {
			canonical = reencoded
		}
	}
	sum := sha256.Sum256(append([]byte(toolName+"\x00"), canonical...))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// canonicalJSON encodes the canonical form [requestDigest] hashes. That form
// always encodes, since [canonicalizeNumbers] turns every number into a marked
// string and leaves strings, booleans, nil, maps and slices alone, each of
// which encodes, so the branch reading its error guards against a change no
// input can bring about. It is a package variable so a test can take that
// branch.
var canonicalJSON = json.Marshal

// encodeState returns the signed, bound wire form of a flow's answers.
//
// The form is v<version>.<payload>.<binding>.<seal>, so the version is
// readable before anything else is trusted and a future encoding can be told
// apart from this one without guessing (see [wireState]).
//
// The payload is the same bytes whichever principal the state was issued
// under. The principal is bound by the binding, a MAC of the payload under a
// key derived from it, and nothing readable on the wire names it: a principal
// carried as a claim, even as a keyed digest, would be a stable identifier of
// the pool entry handed to every client, while the binding changes with every
// payload it covers.
func encodeState(answers map[string]answerRecord, bound stateBinding, now time.Time) (string, error) {
	payload, err := json.Marshal(signedState{
		Version:   flowStateVersion,
		Answers:   answers,
		ExpiresAt: now.Add(stateTTL).Unix(),
		Request:   bound.request,
	})
	if err != nil {
		return "", fmt.Errorf("elicitation: failed to encode request state: %w", err)
	}
	return wireState(base64.RawURLEncoding.EncodeToString(payload), bound.principal), nil
}

// wireState assembles the wire form of an encoded payload issued under
// principal: the version, the payload, the payload's principal binding, and the
// seal over the payload and the binding together.
//
// Two MACs rather than one are what let [decodeState] tell the two answers
// apart. A single MAC under a per-principal key would fail alike for a state
// this server never issued and for one it issued to someone else, and the two
// are answered differently: the first is an error, the second asks the
// questions again.
func wireState(body, principal string) string {
	bind := bindPayload(body, principal)
	return fmt.Sprintf("v%d.%s.%s.%s", flowStateVersion, body, bind, sealPayload(body, bind))
}

// sealPayload is the MAC that proves this process issued a payload and its
// binding: a binding moved from another state breaks it as surely as an edited
// payload does.
func sealPayload(body, bind string) string {
	return macOf(stateKey, body+"."+bind)
}

// bindPayload is the MAC that ties a payload to the principal it was issued
// under.
func bindPayload(body, principal string) string {
	return macOf(principalSigningKey(principal), body)
}

// principalSigningKey derives the key a principal's payloads are bound under
// from the process key, so no principal's key is ever stored and none can be
// computed without the process key.
func principalSigningKey(principal string) []byte {
	mac := hmac.New(sha256.New, stateKey)
	mac.Write([]byte(principalKeyLabel + principal))
	return mac.Sum(nil)
}

// macOf returns the HMAC-SHA256 of text under key, in the wire form's
// base64url.
func macOf(key []byte, text string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(text))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// decodeState verifies a client-echoed state and returns the answers it is
// entitled to carry.
//
// Three outcomes are not errors. State that has expired, state that belongs to
// a different call, and state issued under a different principal all yield no
// answers and no error: the flow simply asks its questions again, which is the
// right thing to do when a user's earlier answers cannot be applied. Only state
// this server did not issue is an error, because at that point nothing about
// it can be believed.
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
func decodeState(raw string, bound stateBinding, now time.Time) (map[string]answerRecord, error) {
	prefix, rest, found := strings.Cut(raw, ".")
	if !found || !strings.HasPrefix(prefix, "v") {
		return nil, fmt.Errorf("%w: malformed", errStateTampered)
	}
	// The version is judged before the rest of the shape, so the encoding of
	// another build, version 1's three parts included, is refused for what it
	// is rather than for a part it never had.
	if prefix != fmt.Sprintf("v%d", flowStateVersion) {
		return nil, fmt.Errorf("elicitation: unsupported requestState version %q", prefix)
	}
	body, bind, seal, ok := splitState(rest)
	if !ok {
		return nil, fmt.Errorf("%w: malformed", errStateTampered)
	}
	if subtle.ConstantTimeCompare([]byte(seal), []byte(sealPayload(body, bind))) != 1 {
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
	if st.Request != bound.request {
		// Answers given to one call, presented on another. Reproducible
		// without any tampering: state issued for one wizard was accepted by a
		// different one, which silently adopted answers meant for the first.
		return map[string]answerRecord{}, nil
	}
	if subtle.ConstantTimeCompare([]byte(bind), []byte(bindPayload(body, bound.principal))) != 1 {
		// Answers given under one principal, presented under another. On a
		// shared server state issued to one credential verified for any other
		// of its shape, which was handed the first caller's answers for the
		// same tool and arguments (issue 953).
		return map[string]answerRecord{}, nil
	}
	return st.Answers, nil
}

// splitState splits what follows the version into the payload, its binding
// and its seal.
func splitState(rest string) (body, bind, seal string, ok bool) {
	parts := strings.Split(rest, ".")
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
