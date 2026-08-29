package elicitation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestDecodeState_RefusesWhatThisServerDidNotIssue pins the integrity check on
// the request state.
//
// "Servers MUST validate the integrity of the state on each round trip
// (e.g. via signing or encryption). Servers MAY skip this validation only when
// tampering can cause nothing worse than request failure."
//
// The state was plain JSON with no MAC, and any well-formed document was
// accepted, so a client could write its own. The exemption did not apply: the
// state carries business logic, and a forged one with no answers in it at all
// made a destructive action proceed as though it had been confirmed.
//
// The size of that is worth stating precisely, because the fix should not be
// oversold. The confirmation gate is an agent-facing guard, not an
// authorization boundary — the same delete succeeds in a single call with
// confirm=true, a documented parameter — so forging state reached nothing a
// client could not already reach on purpose, and crossed no privilege boundary.
// What it broke is the integrity requirement, and the meaning of the gate.
func TestDecodeState_RefusesWhatThisServerDidNotIssue(t *testing.T) {
	now := time.Now()
	const digest = "the-call"

	valid, err := encodeState(map[string]answerRecord{"confirm": {Action: "accept"}}, digest, now)
	if err != nil {
		t.Fatalf("encodeState: %v", err)
	}

	tests := []struct {
		name  string
		state string
	}{
		{
			name: "a document the client wrote itself",
			// What the old encoding accepted verbatim.
			state: `{"v":1,"answers":{"confirm":{"action":"accept"}}}`,
		},
		{
			name:  "the right shape with a forged signature",
			state: "v1." + strings.Split(valid, ".")[1] + ".Zm9yZ2VkLW1hYy12YWx1ZQ",
		},
		{
			name:  "a payload edited after signing",
			state: "v1." + base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"exp":99999999999,"req":"the-call","answers":{"confirm":{"action":"accept"}}}`)) + "." + strings.Split(valid, ".")[2],
		},
		{
			name:  "no signature at all",
			state: "v1." + strings.Split(valid, ".")[1],
		},
		{
			name:  "empty",
			state: "..",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answers, decodeErr := decodeState(tt.state, digest, now)
			if decodeErr == nil {
				t.Fatalf("accepted state this server did not issue, yielding %v", answers)
			}
			if answers != nil {
				t.Errorf("answers = %v, want none alongside the refusal", answers)
			}
		})
	}

	t.Run("state this server issued is accepted", func(t *testing.T) {
		answers, decodeErr := decodeState(valid, digest, now)
		if decodeErr != nil {
			t.Fatalf("refused its own state: %v", decodeErr)
		}
		if answers["confirm"].Action != "accept" {
			t.Errorf("answers = %v, want the recorded confirmation", answers)
		}
	})
}

// TestDecodeState_BindsStateToItsOwnCall pins the binding required alongside
// the signature.
//
// "Servers SHOULD bind the state to the user session or principal, include an
// expiry, and bind it to the specific request."
//
// Signing without binding is close to worthless, and the gap was reproducible
// with no tampering whatever: state the server itself issued for one wizard,
// echoed unmodified to a different wizard on a different project, was accepted
// and silently adopted three answers given to another call.
//
// Neither outcome here is an error. A user whose earlier answers cannot be
// applied should be asked again, not shown a failure they cannot act on, so an
// expired or foreign state yields no answers and the flow re-prompts.
func TestDecodeState_BindsStateToItsOwnCall(t *testing.T) {
	now := time.Now()
	answers := map[string]answerRecord{"title": {Action: "accept", Content: map[string]any{"title": "t"}}}

	issued, err := encodeState(answers, requestDigest("gitlab_interactive_issue_create", []byte(`{"project_id":"7"}`)), now)
	if err != nil {
		t.Fatalf("encodeState: %v", err)
	}

	tests := []struct {
		name   string
		digest string
		at     time.Time
	}{
		{
			name:   "presented on a different tool",
			digest: requestDigest("gitlab_interactive_mr_create", []byte(`{"project_id":"7"}`)),
			at:     now,
		},
		{
			name:   "presented on the same tool with different arguments",
			digest: requestDigest("gitlab_interactive_issue_create", []byte(`{"project_id":"99"}`)),
			at:     now,
		},
		{
			name:   "presented after it expired",
			digest: requestDigest("gitlab_interactive_issue_create", []byte(`{"project_id":"7"}`)),
			at:     now.Add(stateTTL + time.Minute),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, decodeErr := decodeState(issued, tt.digest, tt.at)
			if decodeErr != nil {
				t.Fatalf("err = %v, want the flow to re-prompt rather than fail", decodeErr)
			}
			if len(got) != 0 {
				t.Errorf("adopted %d answer(s) from another call or an expired state", len(got))
			}
		})
	}

	t.Run("the call it was issued for still gets its answers", func(t *testing.T) {
		got, decodeErr := decodeState(issued, requestDigest("gitlab_interactive_issue_create", []byte(`{"project_id":"7"}`)), now.Add(time.Minute))
		if decodeErr != nil {
			t.Fatalf("decodeState: %v", decodeErr)
		}
		if len(got) != 1 {
			t.Errorf("got %d answers, want the one recorded", len(got))
		}
	})
}

// TestRequestDigest_IgnoresReserialization checks that the binding survives a
// client re-encoding its own arguments.
//
// A client echoes the state back on a retry of the same call, but nothing
// obliges it to reproduce the argument bytes: key order in a JSON object
// carries no meaning, and a client that decodes and re-encodes is behaving
// correctly. Hashing the raw bytes would break those flows, which is a worse
// failure than the one the binding prevents.
func TestRequestDigest_IgnoresReserialization(t *testing.T) {
	const name = "gitlab_interactive_issue_create"

	first := requestDigest(name, []byte(`{"project_id":"7","title":"a"}`))
	reordered := requestDigest(name, []byte(`{"title":"a","project_id":"7"}`))
	if first != reordered {
		t.Error("re-ordering the argument keys changed the digest, so a well-behaved retry would lose its answers")
	}

	spaced := requestDigest(name, []byte(`{ "project_id" : "7", "title" : "a" }`))
	if first != spaced {
		t.Error("re-indenting the arguments changed the digest")
	}

	if first == requestDigest(name, []byte(`{"project_id":"8","title":"a"}`)) {
		t.Error("a different project produced the same digest")
	}
	if first == requestDigest("gitlab_interactive_mr_create", []byte(`{"project_id":"7","title":"a"}`)) {
		t.Error("a different tool produced the same digest")
	}
}

// TestDecodeState_MalformedIsAnIntegrityFailure checks the classification of a
// state that cannot be parsed at all, which is not the same as one whose
// version this build does not know.
func TestDecodeState_MalformedIsAnIntegrityFailure(t *testing.T) {
	_, err := decodeState("not-even-close", "d", time.Now())
	if !errors.Is(err, errStateTampered) {
		t.Errorf("err = %v, want an integrity failure", err)
	}

	_, err = decodeState("v99.eyJ2Ijo5OX0.bWFj", "d", time.Now())
	if errors.Is(err, errStateTampered) {
		t.Error("a future version was reported as tampering; it is simply not this encoding")
	}
	if err == nil || !strings.Contains(err.Error(), "unsupported requestState version") {
		t.Errorf("err = %v, want the unknown version named", err)
	}
}

// TestEncodeState_RoundTripsThroughJSON checks that the wire form survives the
// transport, which hands the state back as an ordinary JSON string.
func TestEncodeState_RoundTripsThroughJSON(t *testing.T) {
	now := time.Now()
	const digest = "call"

	issued, err := encodeState(map[string]answerRecord{"a": {Action: "accept"}}, digest, now)
	if err != nil {
		t.Fatalf("encodeState: %v", err)
	}

	encoded, err := json.Marshal(map[string]string{"requestState": issued})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]string
	if decodeErr := json.Unmarshal(encoded, &decoded); decodeErr != nil {
		t.Fatalf("unmarshal: %v", decodeErr)
	}

	answers, err := decodeState(decoded["requestState"], digest, now)
	if err != nil {
		t.Fatalf("the state did not survive a JSON round trip: %v", err)
	}
	if len(answers) != 1 {
		t.Errorf("got %d answers after the round trip, want 1", len(answers))
	}
}

// TestRequestDigest_KeepsNumericPrecision pins that two calls differing only in
// a large integer do not share a digest.
//
// Canonicalization decodes and re-encodes the arguments so a client may
// re-serialize them between rounds. Decoding into a plain any made every JSON
// number a float64, and float64 cannot tell 9007199254740992 from
// 9007199254740993 — so two different projects produced the same digest, and
// answers given for one would have been accepted for the other. That is the
// precise failure the binding exists to prevent, so losing it here would have
// made the binding decorative for large identifiers.
//
// json.Decoder.UseNumber keeps each number as the text the client sent, which
// costs nothing and removes the whole class.
func TestRequestDigest_KeepsNumericPrecision(t *testing.T) {
	const tool = "gitlab_interactive_issue_create"

	tests := []struct {
		name string
		a, b string
	}{
		{
			name: "consecutive integers beyond float64 precision",
			a:    `{"project_id":9007199254740992}`,
			b:    `{"project_id":9007199254740993}`,
		},
		{
			name: "large integers differing in the last digit",
			a:    `{"project_id":18014398509481984}`,
			b:    `{"project_id":18014398509481985}`,
		},
		{
			name: "small integers still differ, as they always did",
			a:    `{"project_id":7}`,
			b:    `{"project_id":8}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if requestDigest(tool, []byte(tt.a)) == requestDigest(tool, []byte(tt.b)) {
				t.Errorf("%s and %s share a digest, so answers given for one would be accepted for the other", tt.a, tt.b)
			}
		})
	}

	t.Run("a large identifier survives re-serialization", func(t *testing.T) {
		// The tolerance that has to hold alongside the precision: a client may
		// re-encode its own arguments between rounds, and doing so must not
		// change the digest, or a legitimate retry would lose its answers.
		// Keeping the number as text is what makes both true at once.
		compact := requestDigest(tool, []byte(`{"project_id":9007199254740993,"title":"t"}`))
		spaced := requestDigest(tool, []byte(`{ "title" : "t", "project_id" : 9007199254740993 }`))
		if compact != spaced {
			t.Error("re-indenting and re-ordering changed the digest, so a well-behaved retry would lose its answers")
		}
	})
}
