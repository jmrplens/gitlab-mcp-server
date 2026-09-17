package modelrecord

import (
	"strings"
	"testing"
	"time"
)

// The valid* builders below are the smallest payload of each type that says
// something: every field a reader joins on or publishes, and nothing else.
//
// They are here rather than inline in each test because a test about the
// envelope still has to hand validate() a payload that means something, and
// three tests were written against empty ones. That is the hole validate.go
// closes, so the fixtures had to stop demonstrating it.

func validRun() *Run {
	return &Run{
		Package:        "modeleval",
		RunID:          "run-1",
		StartedAt:      time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Edition:        "community",
		GitLabVersion:  "19.4.0",
		Tier:           "free",
		CorpusDigest:   "c0ffee",
		ContractDigest: "deadbeef",
		Repeat:         1,
	}
}

func validSession() *Session {
	return &Session{
		Label:        "dynamic/default",
		Surface:      "dynamic",
		Mode:         "default",
		Capabilities: "full",
		ServedTools:  2,
	}
}

func validAttempt() *Attempt {
	return &Attempt{
		ID:      "a1",
		Case:    "MS-001",
		Model:   "fake:perfect",
		Surface: "dynamic",
		Session: "dynamic/default",
		Repeat:  1,
		EndedBy: EndedCompleted,
	}
}

func validTurn() *Turn {
	return &Turn{Attempt: "a1", Index: 1, Try: 1, Status: TurnOK}
}

func validCall() *Call {
	return &Call{Attempt: "a1", Turn: 1, Index: 1, Tool: "gitlab_find_action", Outcome: OutcomeOK}
}

func validVerify() *Verify {
	return &Verify{Attempt: "a1", Name: "the issue exists", Passed: true}
}

// TestValidatePayload_AcceptsTheMinimalPayloadOfEveryType is the floor: the
// builders above have to pass, or every test that uses them is asserting
// against a fixture the reader would refuse.
func TestValidatePayload_AcceptsTheMinimalPayloadOfEveryType(t *testing.T) {
	for _, line := range []Line{validRun(), validSession(), validAttempt(), validTurn(), validCall(), validVerify()} {
		record := line.record()
		t.Run(record.Type, func(t *testing.T) {
			if err := record.validate(); err != nil {
				t.Errorf("the minimal %s payload does not validate: %v", record.Type, err)
			}
		})
	}
}

// TestValidatePayload_RefusesAPayloadThatDescribesNothing is the finding this
// file answers, stated as a test: an envelope can be perfect and carry nothing.
func TestValidatePayload_RefusesAPayloadThatDescribesNothing(t *testing.T) {
	for _, line := range []Line{&Run{}, &Session{}, &Attempt{}, &Turn{}, &Call{}, &Verify{}} {
		record := line.record()
		t.Run(record.Type, func(t *testing.T) {
			err := record.validate()
			if err == nil {
				t.Fatalf("an empty %s payload validated, so a scorer could be handed a line describing nothing", record.Type)
			}
			if !strings.Contains(err.Error(), record.Type+" line") {
				t.Errorf("the error does not name the line type: %v", err)
			}
		})
	}
}

// TestValidatePayload_LeavesAnEmptyRecordToTheEnvelopeCheck states how the two
// halves of validation divide the work between them: the envelope check in
// record.go is what refuses a line carrying no payload at all, and
// validatePayload judges only the payload the line's type names.
//
// It matters because the two run in one order and exactly one of them may own
// that refusal. The envelope's message is the one that can say which of the six
// payloads the type promised, and a shard is machine-written, so that message is
// the whole of what somebody debugging a stale artifact is handed. A payload
// half that also reported "no payload" would be a second answer to a question
// already answered, and one that dereferenced instead would take the run down
// with a panic where a sentence was owed.
func TestValidatePayload_LeavesAnEmptyRecordToTheEnvelopeCheck(t *testing.T) {
	empty := Record{Schema: SchemaVersion, Type: TypeVerify}

	if err := empty.validatePayload(); err != nil {
		t.Errorf("validatePayload of a record carrying nothing = %v, want nothing said", err)
	}

	err := empty.validate()
	if err == nil {
		t.Fatal("a record carrying no payload validated")
	}
	if !strings.Contains(err.Error(), "carries no payload") {
		t.Errorf("validate error = %v, want the envelope check to be the one that refuses it", err)
	}
}

// TestValidateAttempt_HoldsTheEndingToItsReason covers the two endings whose
// meaning is carried by the reason rather than by the value, which is where a
// skipped case and a case nobody wrote would otherwise become the same row.
func TestValidateAttempt_HoldsTheEndingToItsReason(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		endedBy string
		reason  string
		wantErr bool
	}{
		{name: "a skip says what was not met", endedBy: EndedSkipped, reason: "needs a licensed instance"},
		{name: "a skip with no reason is refused", endedBy: EndedSkipped, reason: "", wantErr: true},
		{name: "a completed attempt carries no reason", endedBy: EndedCompleted, reason: ""},
		{name: "a completed attempt with a reason is refused", endedBy: EndedCompleted, reason: "why", wantErr: true},
		{name: "a failure ending may explain itself", endedBy: EndedProviderError, reason: "429 after 5 tries"},
		{name: "an ending nobody declared is refused", endedBy: "gave_up", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			attempt := validAttempt()
			attempt.EndedBy = testCase.endedBy
			attempt.Reason = testCase.reason

			err := attempt.record().validate()

			if testCase.wantErr && err == nil {
				t.Errorf("ended_by %q with reason %q was accepted", testCase.endedBy, testCase.reason)
			}
			if !testCase.wantErr && err != nil {
				t.Errorf("ended_by %q with reason %q was refused: %v", testCase.endedBy, testCase.reason, err)
			}
		})
	}
}

// TestValidateAttempt_AsksForASessionOnlyWhereThereWasOne pins the two endings
// that precede the session, so a later tightening cannot refuse exactly the
// lines that exist to keep a failure from being lost.
func TestValidateAttempt_AsksForASessionOnlyWhereThereWasOne(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		endedBy string
		reason  string
		wantErr bool
	}{
		{name: "a skip never opened one", endedBy: EndedSkipped, reason: "needs a licensed instance"},
		{name: "a harness error may have died opening one", endedBy: EndedHarnessError, reason: "opening the session"},
		{name: "a completed attempt had one", endedBy: EndedCompleted, wantErr: true},
		{name: "an over-budget attempt had one", endedBy: EndedOverBudget, wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			attempt := validAttempt()
			attempt.Session = ""
			attempt.EndedBy, attempt.Reason = testCase.endedBy, testCase.reason

			err := attempt.record().validate()

			if testCase.wantErr && err == nil {
				t.Errorf("an attempt that ended %q with no session was accepted", testCase.endedBy)
			}
			if !testCase.wantErr && err != nil {
				t.Errorf("an attempt that ended %q with no session was refused: %v", testCase.endedBy, err)
			}
		})
	}
}

// TestValidateRun_KeepsTheEmptyDateReadable pins the other deliberate hole. The
// date a run that never started leaves is refused where it is published, by
// name, and refusing it here instead would make the shard unreadable rather
// than reportable.
func TestValidateRun_KeepsTheEmptyDateReadable(t *testing.T) {
	run := validRun()
	run.StartedAt = time.Time{}

	if err := run.record().validate(); err != nil {
		t.Errorf("a run that never started was refused at read time: %v", err)
	}
}

// TestValidateSession_RefusesACountNoListingCouldProduce covers the two numbers
// a session line carries about how much was served, and states where the line
// between a hole and a lie runs for each.
//
// Zero is a hole and stays legal on both: a session whose credential or whose
// exclusions left nothing to list served no tools, and a session that showed the
// whole set sliced nothing. Refusing zero would make the record unable to hold
// exactly the run somebody would open it to look at. Below zero is a lie: both
// numbers are read as the span an attempt's own shown_tools is compared against,
// so a negative one puts that span's floor where no session was.
func TestValidateSession_RefusesACountNoListingCouldProduce(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		servedTools int
		sliceSize   int
		wantErr     string
	}{
		{name: "a session that listed nothing", servedTools: 0, sliceSize: 0},
		{name: "a session that listed a slice", servedTools: 865, sliceSize: 128},
		{name: "a negative served count", servedTools: -1, wantErr: "served_tools is -1"},
		{name: "a negative slice size", servedTools: 2, sliceSize: -1, wantErr: "slice_size is -1"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			session := validSession()
			session.ServedTools, session.SliceSize = testCase.servedTools, testCase.sliceSize

			err := session.record().validate()

			if testCase.wantErr == "" {
				if err != nil {
					t.Errorf("served_tools=%d slice_size=%d was refused: %v", testCase.servedTools, testCase.sliceSize, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("served_tools=%d slice_size=%d was accepted", testCase.servedTools, testCase.sliceSize)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("validate error = %v, want it to say %q", err, testCase.wantErr)
			}
		})
	}
}

// TestValidateCall_TakesARefusalWithItsReason pins the one outcome that is a
// prefix rather than a value, so a refusal whose reason was lost is refused
// while a real one is not.
func TestValidateCall_TakesARefusalWithItsReason(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		outcome string
		wantErr bool
	}{
		{name: "a refusal with its reason", outcome: OutcomeRefusedPrefix + "read_only"},
		{name: "a refusal with nothing after the prefix", outcome: OutcomeRefusedPrefix, wantErr: true},
		{name: "an outcome nobody declared", outcome: "shrugged", wantErr: true},
		{name: "a preview", outcome: OutcomePreview},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			call := validCall()
			call.Outcome = testCase.outcome

			err := call.record().validate()

			if testCase.wantErr && err == nil {
				t.Errorf("outcome %q was accepted", testCase.outcome)
			}
			if !testCase.wantErr && err != nil {
				t.Errorf("outcome %q was refused: %v", testCase.outcome, err)
			}
		})
	}
}

// TestValidateAttempt_RefusesANegativeShownCount covers the number a row's
// span is built from. A value below zero would put the span's floor somewhere
// no attempt was, and the span is what says an individual row measured above
// its own slice size.
func TestValidateAttempt_RefusesANegativeShownCount(t *testing.T) {
	attempt := validAttempt()
	attempt.ShownTools = -1

	if err := attempt.record().validate(); err == nil {
		t.Error("shown_tools of -1 was accepted, and a row's shown span would reach below every attempt")
	}

	// Zero stays legal: it is what an attempt that never reached a session
	// carries, and those are exactly the lines the record exists to keep.
	attempt.ShownTools = 0
	if err := attempt.record().validate(); err != nil {
		t.Errorf("an attempt that was shown nothing was refused: %v", err)
	}
}

// TestValidateTurn_RefusesAPositionNobodyWrote covers the zero value of a
// 1-based index, which is the shape a field left unset takes and reads as a
// real position.
func TestValidateTurn_RefusesAPositionNobodyWrote(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		index int
		try   int
	}{
		{name: "index unset", index: 0, try: 1},
		{name: "try unset", index: 1, try: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			turn := validTurn()
			turn.Index, turn.Try = testCase.index, testCase.try

			if err := turn.record().validate(); err == nil {
				t.Errorf("index=%d try=%d was accepted", testCase.index, testCase.try)
			}
		})
	}
}

// TestPositive_RefusesTheZeroOnEveryLineThatCountsFromOne covers the other three
// callers of the same helper the turn line's index and try go through.
//
// One check written once is not one check made: each caller has its own error
// branch, and a caller that dropped it would stay green while its own field went
// unjudged. Each of these is a field a reader joins or divides by. A run that
// repeated each attempt no times is the denominator of every average published
// from it; an attempt at repeat 0 cannot be told from the other runs of itself;
// and a call at turn 0 names a turn that is in no shard, so what the model read
// before making it can never be found. Zero is what all four carry when nobody
// wrote them, which is what makes it the value worth refusing.
func TestPositive_RefusesTheZeroOnEveryLineThatCountsFromOne(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		line  Line
		field string
	}{
		{name: "a run that repeated nothing", line: runWithRepeat(0), field: "repeat"},
		{name: "an attempt that is no run of itself", line: attemptWithRepeat(0), field: "repeat"},
		{name: "a call made in no turn", line: callAt(0, 1), field: "turn"},
		{name: "a call at no position among the attempt's calls", line: callAt(1, 0), field: "index"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.line.record().validate()

			if err == nil {
				t.Fatalf("%s of 0 was accepted, and it reads as a real position", testCase.field)
			}
			if !strings.Contains(err.Error(), testCase.field+" is 0") {
				t.Errorf("validate error = %v, want it to name %s and the value it held", err, testCase.field)
			}
		})
	}
}

// runWithRepeat, attemptWithRepeat and callAt are the builders above with one
// counter moved, kept out of the table so a case reads as the line it is.
func runWithRepeat(repeat int) *Run {
	run := validRun()
	run.Repeat = repeat
	return run
}

func attemptWithRepeat(repeat int) *Attempt {
	attempt := validAttempt()
	attempt.Repeat = repeat
	return attempt
}

func callAt(turn, index int) *Call {
	call := validCall()
	call.Turn, call.Index = turn, index
	return call
}

// TestValidateTurn_RefusesAStatusNobodyDeclared holds the turn line's status to
// the constants rather than to a copy of them.
//
// The list validate.go matches against is a second spelling of the five Turn*
// constants, and nothing but this says the two are the same set. A status
// dropped from that list stops being refused, which loses a provider failure
// into the ok column; one dropped from the constants and left in the list
// refuses a line a real run writes, which loses the whole shard behind it.
func TestValidateTurn_RefusesAStatusNobodyDeclared(t *testing.T) {
	for _, status := range []string{
		TurnOK, TurnRateLimited, TurnServerError, TurnRequestError, TurnTransportError,
	} {
		t.Run(status, func(t *testing.T) {
			turn := validTurn()
			turn.Status = status

			if err := turn.record().validate(); err != nil {
				t.Errorf("a turn that ended %q was refused: %v", status, err)
			}
		})
	}

	for _, status := range []string{"", "refused", "OK"} {
		t.Run("undeclared status "+status, func(t *testing.T) {
			turn := validTurn()
			turn.Status = status

			err := turn.record().validate()

			if err == nil {
				t.Fatalf("status %q was accepted, and a reader would count it as neither served nor failed", status)
			}
			if !strings.Contains(err.Error(), "status is") {
				t.Errorf("validate error = %v, want it to name the status", err)
			}
		})
	}
}

// TestValidateTurn_RefusesABlockKindNobodyDeclared covers the other enum a turn
// line carries, and the position its message names.
//
// A kind outside the three is a provider block this side does not know it is
// holding: a reader that took it would attribute whatever it carries to prose,
// and a thinking block is billed for while a tool call is what the attempt is
// scored on. The position is asserted because it is the only landmark a person
// has inside a turn that emitted several blocks, and it is computed rather than
// read, so nothing else holds it to counting the way a reader counts.
func TestValidateTurn_RefusesABlockKindNobodyDeclared(t *testing.T) {
	turn := validTurn()
	turn.Blocks = []Block{
		{Kind: BlockThinking, Text: "which action lists issues"},
		{Kind: "function_call", Tool: "gitlab_execute_action"},
		{Kind: BlockText, Text: "Looking it up."},
	}

	err := turn.record().validate()

	if err == nil {
		t.Fatal(`a block of kind "function_call" was accepted, and nothing downstream knows what it holds`)
	}
	if !strings.Contains(err.Error(), "block 2") {
		t.Errorf("validate error = %v, want it to name the second block, counting as a reader of the line counts", err)
	}
	if !strings.Contains(err.Error(), "function_call") {
		t.Errorf("validate error = %v, want it to quote the kind it did not know", err)
	}

	// The three declared kinds are what a real turn is made of, so the check has
	// to let a turn carrying all three through.
	turn.Blocks = []Block{
		{Kind: BlockThinking, Text: "which action lists issues"},
		{Kind: BlockText, Text: "Looking it up."},
		{Kind: BlockToolCall, Tool: "gitlab_execute_action", CallID: "toolu_01"},
	}
	if declaredErr := turn.record().validate(); declaredErr != nil {
		t.Errorf("a turn carrying the three declared block kinds was refused: %v", declaredErr)
	}
}

// TestValidateTurn_RefusesNegativeUsage covers the counters both readers add
// up. A negative one does not merely misreport: it subtracts, so a run reads as
// cheaper than it was and the spend is what stops a paid run.
func TestValidateTurn_RefusesNegativeUsage(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		usage Usage
	}{
		{name: "input", usage: Usage{Input: -1}},
		{name: "output", usage: Usage{Output: -1}},
		{name: "cache created", usage: Usage{CacheCreated: -1}},
		{name: "cache read", usage: Usage{CacheRead: -1}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			turn := validTurn()
			turn.Usage = testCase.usage

			if err := turn.record().validate(); err == nil {
				t.Errorf("usage %+v was accepted, and both costOf and tokensOf would subtract it", testCase.usage)
			}
		})
	}

	turn := validTurn()
	turn.Usage = Usage{Input: 10, Output: 5, CacheCreated: 0, CacheRead: 3}
	if err := turn.record().validate(); err != nil {
		t.Errorf("an ordinary usage was refused: %v", err)
	}
}

// TestValidateVerify_RefusesAFailureWithNothingSaidAboutIt keeps the one field
// a reader acts on from being optional exactly where it matters.
func TestValidateVerify_RefusesAFailureWithNothingSaidAboutIt(t *testing.T) {
	verify := validVerify()
	verify.Passed, verify.Detail = false, ""

	if err := verify.record().validate(); err == nil {
		t.Error("a failed check carrying no detail was accepted")
	}

	verify.Detail = "the issue was not created"
	if err := verify.record().validate(); err != nil {
		t.Errorf("a failed check with a detail was refused: %v", err)
	}
}
