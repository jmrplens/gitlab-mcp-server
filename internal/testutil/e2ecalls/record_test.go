package e2ecalls

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
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

// TestRunIDDate_Cases verifies what the stamp of a run identifier is read as,
// and what is read as no stamp at all.
//
// The date of the committed coverage record is this parse and nothing else, so
// the two refusals matter as much as the reading: an identifier the harness did
// not stamp must leave the entry dateless and loudly so, rather than be dated
// by the clock of whoever rebuilt the file.
func TestRunIDDate_Cases(t *testing.T) {
	cases := []struct {
		name  string
		runID string
		want  time.Time
		read  bool
	}{
		{
			name:  "a minted identifier",
			runID: "20260914t191454z-a97711a8e8-ce",
			want:  time.Date(2026, 9, 14, 19, 14, 54, 0, time.UTC),
			read:  true,
		},
		{name: "no dash, so no stamp this package wrote", runID: "20260914t191454z"},
		{name: "an operator's own identifier", runID: "nightly-run-ce"},
		{name: "nothing at all", runID: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, read := RunIDDate(testCase.runID)

			if read != testCase.read {
				t.Fatalf("RunIDDate(%q) read = %t, want %t", testCase.runID, read, testCase.read)
			}
			if read && !got.Equal(testCase.want) {
				t.Errorf("RunIDDate(%q) = %s, want %s", testCase.runID, got, testCase.want)
			}
		})
	}
}

// TestRunIDStampLayout_FormatsAndParsesBack verifies that the layout is a
// round trip of itself, which is the property both sides rest on: the harness
// formats a run identifier's stamp with it and cmd/audit_e2e_coverage parses
// that stamp back with it to date the committed record.
func TestRunIDStampLayout_FormatsAndParsesBack(t *testing.T) {
	at := time.Date(2026, 9, 14, 19, 14, 54, 0, time.UTC)

	got, read := RunIDDate(at.Format(RunIDStampLayout) + "-a97711a8e8-ce")

	if !read || !got.Equal(at) {
		t.Errorf("RunIDDate(Format(%s)) = %s, %t; want %s and true", at, got, read, at)
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

// TestDispatch_RequestCount_SurvivesTheRoundTripAndIsOmittedAtZero pins the
// field R-PATH reads to answer "did this action issue a request" per action
// rather than per package.
//
// Both halves matter. A count that did not survive the round trip would leave
// the question unanswerable while every line still parsed, which is the
// silence the whole record exists to remove. And a zero must be absent rather
// than written: an action that reached no GitLab is the ordinary case for a
// refusal and a safe-mode preview, and a shard from before this field existed
// reads the same way, so the reader is told nothing rather than told zero.
func TestDispatch_RequestCount_SurvivesTheRoundTripAndIsOmittedAtZero(t *testing.T) {
	cases := []struct {
		name    string
		line    *Dispatch
		want    int
		written bool
	}{
		{
			name:    "a handler that called GitLab",
			line:    &Dispatch{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", Action: "issue.list", Requests: 2},
			want:    2,
			written: true,
		},
		{
			name: "a refusal that called nobody",
			line: &Dispatch{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", Action: "issue.delete", RefusalReason: "safe_mode"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(testCase.line.record())
			if err != nil {
				t.Fatalf("Marshal error = %v", err)
			}

			var decoded Record
			if unmarshalErr := json.Unmarshal(encoded, &decoded); unmarshalErr != nil {
				t.Fatalf("Unmarshal error = %v", unmarshalErr)
			}
			if validateErr := decoded.validate(); validateErr != nil {
				t.Fatalf("validate() error = %v, want nil", validateErr)
			}
			if decoded.Dispatch.Requests != testCase.want {
				t.Errorf("Requests = %d, want %d", decoded.Dispatch.Requests, testCase.want)
			}
			if strings.Contains(string(encoded), `"requests"`) != testCase.written {
				t.Errorf("encoded record = %s, want a requests field present = %t", encoded, testCase.written)
			}
		})
	}
}

// TestDirEnv_IsTheNameTheMakefileAndTheWorkflowExport pins the spelling of the
// variable that turns recording on.
//
// Nothing that sets it moves when this constant does: the Makefile exports it
// at five sites, .github/workflows/e2e.yml at one, and the testing reference
// documents it. Renaming the constant alone leaves all of them exporting a
// variable nobody reads, and the run then records nothing -- which is the
// documented default, so no Docker run would fail and the coverage record
// would simply stop being written.
func TestDirEnv_IsTheNameTheMakefileAndTheWorkflowExport(t *testing.T) {
	if want := "GITLAB_MCP_TEST_E2E_CALLS_DIR"; DirEnv != want {
		t.Errorf("DirEnv = %q, want %q: the Makefile and e2e.yml export that name", DirEnv, want)
	}
}

// TestVocabulary_SpellsEachValueAShardCarries pins the literal value of every
// constant a shard line is classified by.
//
// Each of these reaches cmd/audit_e2e_coverage as a string in a file, and the
// committed shard fixtures under its testdata spell them by hand. Crossing two
// members of one group is invisible to every other test here, because the rest
// of the package names them symbolically and a symmetric swap cancels out:
// crossing PurposeTest with PurposeCleanup would credit fixture teardown as
// asserted coverage and demote what the tests actually asserted, with both
// gates green. Only a literal holds them.
func TestVocabulary_SpellsEachValueAShardCarries(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "TypeRun", got: TypeRun, want: "run"},
		{name: "TypeSession", got: TypeSession, want: "session"},
		{name: "TypeCall", got: TypeCall, want: "call"},
		{name: "TypeDispatch", got: TypeDispatch, want: "dispatch"},
		{name: "TypeSkip", got: TypeSkip, want: "skip"},
		{name: "PurposeTest", got: PurposeTest, want: "test"},
		{name: "PurposeCleanup", got: PurposeCleanup, want: "cleanup"},
		{name: "PurposeSweep", got: PurposeSweep, want: "sweep"},
		{name: "PurposeRaw", got: PurposeRaw, want: "raw"},
		{name: "PurposeModel", got: PurposeModel, want: "model"},
		{name: "ExpectationOK", got: ExpectationOK, want: "ok"},
		{name: "ExpectationAny", got: ExpectationAny, want: "any"},
		{name: "OutcomeOK", got: OutcomeOK, want: "ok"},
		{name: "OutcomeToolError", got: OutcomeToolError, want: "tool_error"},
		{name: "OutcomeProtocolError", got: OutcomeProtocolError, want: "protocol_error"},
		{name: "OutcomeTransportError", got: OutcomeTransportError, want: "transport_error"},
		{name: "OutcomePreview", got: OutcomePreview, want: "preview"},
		{name: "OutcomeRefusedPrefix", got: OutcomeRefusedPrefix, want: "refused:"},
		{name: "RunStarted", got: RunStarted, want: "started"},
		{name: "RunRefused", got: RunRefused, want: "refused"},
		{name: "StatusPassed", got: StatusPassed, want: "passed"},
		{name: "StatusFailed", got: StatusFailed, want: "failed"},
		{name: "StatusSkipped", got: StatusSkipped, want: "skipped"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("%s = %q, want %q", testCase.name, testCase.got, testCase.want)
			}
		})
	}
}

// encodedRecord marshals a line in its envelope and returns the envelope and
// the payload object found under key, failing when there is none.
func encodedRecord(t *testing.T, line Line, key string) (map[string]any, map[string]any) {
	t.Helper()

	encoded, err := json.Marshal(line.record())
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var envelope map[string]any
	if unmarshalErr := json.Unmarshal(encoded, &envelope); unmarshalErr != nil {
		t.Fatalf("Unmarshal error = %v", unmarshalErr)
	}
	payload, ok := envelope[key].(map[string]any)
	if !ok {
		t.Fatalf("encoded record = %s, want a payload object under %q", encoded, key)
	}
	return envelope, payload
}

// TestRecordJSON_SpellsEveryKeyAndValueAShardCarries encodes one fully
// populated line of each type and compares the whole object, key by key,
// against the spelling written down here.
//
// Every field is given a value no sibling of its type shares, which is the
// point: a JSON tag moved from one field to its neighbor keeps the document
// parsing and keeps every round trip passing, so only distinct values and a
// whole-object comparison can see it. Action and Dispatched are the case that
// prompted this -- the older test set both to "issue.list", which is exactly
// the fixture in which the two cannot be told apart.
func TestRecordJSON_SpellsEveryKeyAndValueAShardCarries(t *testing.T) {
	// wantType and wantKey carry the same string in every case and are two
	// fields rather than one, because they pin two things that only happen to
	// agree: the value the line's own Type constant writes into "type", and
	// the JSON tag the payload sits under. Crossing either alone leaves the
	// other right.
	cases := []struct {
		name     string
		line     Line
		wantType string
		wantKey  string
		want     map[string]any
	}{
		{
			name: "run",
			line: &Run{
				Package:       "package-common",
				Requirement:   "requirement-any",
				Edition:       "edition-enterprise",
				Tier:          "tier-ultimate",
				TierConfirmed: true,
				GitLabVersion: "gitlab_version-19.4.1",
				RunID:         "run_id-20260914t191454z-a97711a8e8-ce",
				Commit:        "commit-1c02ea39c",
				Filter:        "filter-TestIssue",
				// The four flags are all set so that every key must appear.
				// Which key each one lands under is the sibling test's
				// question, since no two booleans can carry distinct values.
				Fixtures: FixtureProfile{
					Runner:         true,
					FixtureService: true,
					Bitbucket:      true,
					GHToken:        true,
					Seeds:          []string{"seeds-issues"},
				},
				Status: "status-started",
				Reason: "reason-none",
			},
			wantType: "run",
			wantKey:  "run",
			want: map[string]any{
				"package":        "package-common",
				"requirement":    "requirement-any",
				"edition":        "edition-enterprise",
				"tier":           "tier-ultimate",
				"tier_confirmed": true,
				"gitlab_version": "gitlab_version-19.4.1",
				"run_id":         "run_id-20260914t191454z-a97711a8e8-ce",
				"commit":         "commit-1c02ea39c",
				"filter":         "filter-TestIssue",
				"fixtures": map[string]any{
					"runner":          true,
					"fixture_service": true,
					"bitbucket":       true,
					"gh_token":        true,
					"seeds":           []any{"seeds-issues"},
				},
				"status": "status-started",
				"reason": "reason-none",
			},
		},
		{
			name: "session",
			line: &Session{
				Label:             "label-dynamic-default",
				Surface:           "surface-dynamic",
				Mode:              "mode-default",
				Capabilities:      "capabilities-full",
				Transport:         "transport-stdio",
				Tools:             []string{"tools-gitlab_find_action"},
				Resources:         []string{"resources-gitlab://groups"},
				ResourceTemplates: []string{"resource_templates-gitlab://project/{project_id}"},
				Prompts:           []string{"prompts-summarize_issue"},
				Completions:       []string{"completions-ref/prompt"},
				SubscribableKinds: []string{"subscribable_kinds-issue"},
				DispatchObserved:  true,
				Idle:              true,
			},
			wantType: "session",
			wantKey:  "session",
			want: map[string]any{
				"label":              "label-dynamic-default",
				"surface":            "surface-dynamic",
				"mode":               "mode-default",
				"capabilities":       "capabilities-full",
				"transport":          "transport-stdio",
				"tools":              []any{"tools-gitlab_find_action"},
				"resources":          []any{"resources-gitlab://groups"},
				"resource_templates": []any{"resource_templates-gitlab://project/{project_id}"},
				"prompts":            []any{"prompts-summarize_issue"},
				"completions":        []any{"completions-ref/prompt"},
				"subscribable_kinds": []any{"subscribable_kinds-issue"},
				"dispatch_observed":  true,
				"idle":               true,
			},
		},
		{
			name: "call",
			line: &Call{
				Test:         "test-TestIssue_List",
				Purpose:      "purpose-test",
				Expectation:  "expectation-ok",
				Session:      "session-dynamic/default",
				Surface:      "surface-dynamic",
				Mode:         "mode-default",
				Capabilities: "capabilities-full",
				Requirement:  "requirement-any",
				Method:       "method-tools/call",
				Tool:         "tool-gitlab_execute_action",
				Action:       "action-issue.list",
				Dispatched:   "dispatched-issue.list",
				Target:       "target-gitlab://groups",
				Arguments:    []string{"arguments-project_id"},
				Outcome:      "outcome-ok",
				DurationMS:   12.5,
				TraceID:      "trace_id-4bf92f3577b34da6",
				TestStatus:   "test_status-passed",
			},
			wantType: "call",
			wantKey:  "call",
			want: map[string]any{
				"test":         "test-TestIssue_List",
				"purpose":      "purpose-test",
				"expectation":  "expectation-ok",
				"session":      "session-dynamic/default",
				"surface":      "surface-dynamic",
				"mode":         "mode-default",
				"capabilities": "capabilities-full",
				"requirement":  "requirement-any",
				"method":       "method-tools/call",
				"tool":         "tool-gitlab_execute_action",
				"action":       "action-issue.list",
				"dispatched":   "dispatched-issue.list",
				"target":       "target-gitlab://groups",
				"arguments":    []any{"arguments-project_id"},
				"outcome":      "outcome-ok",
				"duration_ms":  12.5,
				"trace_id":     "trace_id-4bf92f3577b34da6",
				"test_status":  "test_status-passed",
			},
		},
		{
			name: "dispatch",
			line: &Dispatch{
				TraceID:       "trace_id-4bf92f3577b34da6",
				Tool:          "tool-gitlab_execute_action",
				Action:        "action-issue.list",
				Domain:        "domain-issue",
				RefusalReason: "refusal_reason-read_only",
				ErrorType:     "error_type-tool_error",
				Status:        "status-error",
				Requests:      3,
			},
			wantType: "dispatch",
			wantKey:  "dispatch",
			want: map[string]any{
				"trace_id":       "trace_id-4bf92f3577b34da6",
				"tool":           "tool-gitlab_execute_action",
				"action":         "action-issue.list",
				"domain":         "domain-issue",
				"refusal_reason": "refusal_reason-read_only",
				"error_type":     "error_type-tool_error",
				"status":         "status-error",
				"requests":       float64(3),
			},
		},
		{
			name:     "skip",
			line:     &Skip{Test: "test-TestEE_Epics", Reason: "reason-no license"},
			wantType: "skip",
			wantKey:  "skip",
			want: map[string]any{
				"test":   "test-TestEE_Epics",
				"reason": "reason-no license",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			envelope, payload := encodedRecord(t, testCase.line, testCase.wantKey)

			// The envelope carries the schema, the type and the one payload,
			// so a second payload key would be a line the reader refuses.
			if len(envelope) != 3 {
				t.Errorf("envelope keys = %v, want exactly schema, type and %q", envelope, testCase.wantKey)
			}
			if envelope["type"] != testCase.wantType {
				t.Errorf("type = %v, want %q", envelope["type"], testCase.wantType)
			}
			if !reflect.DeepEqual(payload, testCase.want) {
				t.Errorf("%s payload =\n%#v\nwant\n%#v", testCase.wantKey, payload, testCase.want)
			}
		})
	}
}

// TestFixtureProfile_WritesEachFlagUnderItsOwnKey drives one flag at a time
// and compares the whole encoded profile against the one key that should have
// moved.
//
// A block of booleans has no fixture in which no two values agree, so setting
// them all true pins the four key names and tells none of them apart. One flag
// at a time does, and it is also what a runtime really answers: a run with a
// registered runner and no Bitbucket is the ordinary CE shape, and reading the
// two the wrong way round would report a scenario absent for want of a fixture
// that was there.
func TestFixtureProfile_WritesEachFlagUnderItsOwnKey(t *testing.T) {
	cases := []struct {
		name    string
		profile FixtureProfile
		wantKey string
	}{
		{name: "runner alone", profile: FixtureProfile{Runner: true}, wantKey: "runner"},
		{name: "fixture service alone", profile: FixtureProfile{FixtureService: true}, wantKey: "fixture_service"},
		{name: "bitbucket alone", profile: FixtureProfile{Bitbucket: true}, wantKey: "bitbucket"},
		{name: "github token alone", profile: FixtureProfile{GHToken: true}, wantKey: "gh_token"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, payload := encodedRecord(t, &Run{Fixtures: testCase.profile}, "run")

			fixtures, ok := payload["fixtures"].(map[string]any)
			if !ok {
				t.Fatalf("run payload = %v, want a fixtures object", payload)
			}
			want := map[string]any{
				"runner":          testCase.wantKey == "runner",
				"fixture_service": testCase.wantKey == "fixture_service",
				"bitbucket":       testCase.wantKey == "bitbucket",
				"gh_token":        testCase.wantKey == "gh_token",
			}
			if !reflect.DeepEqual(fixtures, want) {
				t.Errorf("fixtures = %#v, want only %q set: %#v", fixtures, testCase.wantKey, want)
			}
		})
	}
}
