package e2ecalls

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRefusedOutcome_CarriesThePrefixAndTheReason verifies that a refusal is
// spelled as the prefix the reader classifies on followed by the reason the
// server gave.
//
// It matters because the coverage audit tells a refused call from a served one
// by that prefix alone. A second hand-written concatenation anywhere would be
// how the writer and the reader come to disagree about what a refusal looks
// like, so there is one function and this test pins what it produces.
func TestRefusedOutcome_CarriesThePrefixAndTheReason(t *testing.T) {
	got := RefusedOutcome("read_only")

	if want := "refused:read_only"; got != want {
		t.Errorf("RefusedOutcome = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, OutcomeRefusedPrefix) {
		t.Errorf("RefusedOutcome = %q, want it to start with %q", got, OutcomeRefusedPrefix)
	}
}

// TestLineRecord_WrapsEachLineInItsOwnEnvelope verifies that every line type
// declares the schema version, names its own type, and sets its own payload
// and no other.
//
// The envelope is the whole contract with the reader: it dispatches on the
// type and reads the payload that type names. A line wrapping itself as the
// wrong type, or leaving its payload unset, would be dropped by a reader that
// is right to refuse it, so each of the five is checked here rather than only
// through whichever ones a round-trip test happens to use.
func TestLineRecord_WrapsEachLineInItsOwnEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		line     Line
		wantType string
	}{
		{name: "run", line: &Run{Package: "common", Status: RunStarted}, wantType: TypeRun},
		{name: "session", line: &Session{Label: "dynamic"}, wantType: TypeSession},
		{name: "call", line: &Call{Test: "TestCommon_Issues"}, wantType: TypeCall},
		{name: "dispatch", line: &Dispatch{TraceID: "abc"}, wantType: TypeDispatch},
		{name: "skip", line: &Skip{Test: "TestCommon_Issues", Reason: "no runner"}, wantType: TypeSkip},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			record := testCase.line.record()

			if record.Schema != SchemaVersion {
				t.Errorf("Schema = %d, want %d", record.Schema, SchemaVersion)
			}
			if record.Type != testCase.wantType {
				t.Errorf("Type = %q, want %q", record.Type, testCase.wantType)
			}
			// validate is what the reader applies, and it is the check that
			// the payload named by the type is the one that was set.
			if err := record.validate(); err != nil {
				t.Errorf("validate() = %v, want nil for a line wrapping itself", err)
			}
		})
	}
}

// TestRecordValidate_RefusesALineItCannotRead verifies that a record whose
// schema, type or payload does not hold together is reported rather than
// accepted.
//
// A shard is machine-written, so each of these means the artifact is stale or
// truncated. Accepting one would let the coverage audit compute a figure over
// lines it did not understand, which is worse than refusing to compute one.
func TestRecordValidate_RefusesALineItCannotRead(t *testing.T) {
	cases := []struct {
		name    string
		record  Record
		wantErr string
	}{
		{
			name:    "another schema",
			record:  Record{Schema: SchemaVersion + 1, Type: TypeCall, Call: &Call{}},
			wantErr: "schema",
		},
		{
			name:    "unknown type",
			record:  Record{Schema: SchemaVersion, Type: "elicitation", Skip: &Skip{}},
			wantErr: "unknown line type",
		},
		{
			name:    "run without a payload",
			record:  Record{Schema: SchemaVersion, Type: TypeRun},
			wantErr: "carries no payload",
		},
		{
			name:    "session without a payload",
			record:  Record{Schema: SchemaVersion, Type: TypeSession},
			wantErr: "carries no payload",
		},
		{
			name:    "call without a payload",
			record:  Record{Schema: SchemaVersion, Type: TypeCall},
			wantErr: "carries no payload",
		},
		{
			name:    "dispatch without a payload",
			record:  Record{Schema: SchemaVersion, Type: TypeDispatch},
			wantErr: "carries no payload",
		},
		{
			name:    "skip without a payload",
			record:  Record{Schema: SchemaVersion, Type: TypeSkip},
			wantErr: "carries no payload",
		},
		// The named payload is there and so is another: the envelope holds
		// exactly one, and a reader that took the named half would have read
		// half a record. The three shapes below are the ones a stale or
		// hand-edited shard could plausibly hold.
		{
			name:    "call carrying a dispatch as well",
			record:  Record{Schema: SchemaVersion, Type: TypeCall, Call: &Call{}, Dispatch: &Dispatch{}},
			wantErr: "carries 2 payloads, want exactly one",
		},
		{
			name:    "run carrying a session as well",
			record:  Record{Schema: SchemaVersion, Type: TypeRun, Run: &Run{}, Session: &Session{}},
			wantErr: "carries 2 payloads, want exactly one",
		},
		{
			name:    "skip carrying every payload",
			record:  Record{Schema: SchemaVersion, Type: TypeSkip, Run: &Run{}, Session: &Session{}, Call: &Call{}, Dispatch: &Dispatch{}, Skip: &Skip{}},
			wantErr: "carries 5 payloads, want exactly one",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.record.validate()

			if err == nil {
				t.Fatalf("validate() = nil, want an error mentioning %q", testCase.wantErr)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("validate() = %v, want it to mention %q", err, testCase.wantErr)
			}
		})
	}
}

// TestRecord_KeepsTheFieldNamesTheReaderJoinsOn verifies that the JSON of a
// call line spells the fields cmd/audit_e2e_coverage reads by name.
//
// The Go type is shared, so a renamed field would move both halves together
// and compile. What would not move with it is a shard written by an older run,
// or a report someone reads by hand, and the trace ID is what the server's own
// span is joined on. Pinning the spelling makes the rename visible here rather
// than as an empty coverage column.
func TestRecord_KeepsTheFieldNamesTheReaderJoinsOn(t *testing.T) {
	line := &Call{
		Test:       "TestCommon_Issues",
		Purpose:    PurposeTest,
		Action:     "issue.list",
		Dispatched: "issue.list",
		Outcome:    OutcomeOK,
		TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
		TestStatus: StatusPassed,
	}

	encoded, err := json.Marshal(line.record())
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}

	var decoded map[string]any
	if unmarshalErr := json.Unmarshal(encoded, &decoded); unmarshalErr != nil {
		t.Fatalf("Unmarshal error = %v", unmarshalErr)
	}
	call, ok := decoded["call"].(map[string]any)
	if !ok {
		t.Fatalf("encoded record = %s, want a call object under \"call\"", encoded)
	}
	for _, field := range []string{"test", "purpose", "action", "dispatched", "outcome", "trace_id", "test_status"} {
		t.Run(field, func(t *testing.T) {
			if _, present := call[field]; !present {
				t.Errorf("call object = %v, want a %q field", call, field)
			}
		})
	}
}
