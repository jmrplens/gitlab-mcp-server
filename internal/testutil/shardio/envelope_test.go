package shardio

import (
	"fmt"
	"strings"
	"testing"
)

// TestValidateEnvelope_AcceptsARecordThatHoldsTogether verifies that the one
// shape the rule is about, a known type carrying exactly its own payload under
// the current schema, is accepted.
func TestValidateEnvelope_AcceptsARecordThatHoldsTogether(t *testing.T) {
	record := fixtureRecord{Schema: fixtureSchema, Type: typeNote, Note: &note{Text: "a"}}

	if err := validateFixture(record); err != nil {
		t.Errorf("ValidateEnvelope error = %v, want nil for a record that holds together", err)
	}
}

// TestValidateEnvelope_RefusesARecordThatDoesNotHoldTogether verifies that a
// record whose schema, type or payload count is wrong is reported rather than
// accepted.
//
// A shard is machine-written, so each of these means the artifact is stale or
// truncated. Accepting one would let a reader compute a figure over lines it
// did not understand, which is worse than refusing to compute one. The last two
// cases are the reason the rule counts payloads instead of only checking the
// named one: a line carrying two is two claims under one type, and a reader
// that took the named half would have read half a record.
//
// The schema case names both numbers in order, because they are a pair no
// fixture can tell apart: crossed, the refusal tells a reader the shard was
// written at the version this package is, and that the version it is holds the
// shard, which sends them to look at the wrong side of the mismatch.
func TestValidateEnvelope_RefusesARecordThatDoesNotHoldTogether(t *testing.T) {
	cases := []struct {
		name    string
		record  fixtureRecord
		wantErr string
	}{
		{
			name:    "another schema",
			record:  fixtureRecord{Schema: fixtureSchema + 1, Type: typeNote, Note: &note{}},
			wantErr: fmt.Sprintf("schema %d is not %d", fixtureSchema+1, fixtureSchema),
		},
		{
			name:    "unknown type",
			record:  fixtureRecord{Schema: fixtureSchema, Type: "invented", Note: &note{}},
			wantErr: "unknown line type",
		},
		{
			name:    "type without its payload",
			record:  fixtureRecord{Schema: fixtureSchema, Type: typeNote},
			wantErr: "carries no payload",
		},
		{
			name:    "another payload beside the named one",
			record:  fixtureRecord{Schema: fixtureSchema, Type: typeNote, Note: &note{}, Measure: &measure{}},
			wantErr: "carries 2 payloads, want exactly one",
		},
		{
			name:    "every payload at once",
			record:  fixtureRecord{Schema: fixtureSchema, Type: typeNote, Note: &note{}, Measure: &measure{}, Carrier: &carrier{}},
			wantErr: "carries 3 payloads, want exactly one",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateFixture(testCase.record)

			if err == nil {
				t.Fatalf("ValidateEnvelope = nil, want an error mentioning %q", testCase.wantErr)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("ValidateEnvelope = %v, want it to mention %q", err, testCase.wantErr)
			}
		})
	}
}
