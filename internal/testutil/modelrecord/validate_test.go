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
