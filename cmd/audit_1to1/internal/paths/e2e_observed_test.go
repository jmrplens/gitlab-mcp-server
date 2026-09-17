package paths

import (
	"errors"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// withE2ERecord replaces the shard reader for the length of a test, so a case
// can describe a run without a Docker GitLab and four minutes of it.
func withE2ERecord(t *testing.T, read func(string) ([]e2ecalls.Record, error)) {
	t.Helper()
	original := readE2ECalls
	readE2ECalls = read
	t.Cleanup(func() { readE2ECalls = original })
}

// dispatchRecordOf wraps one dispatch line the way a shard holds it.
//
// The trace is a parameter rather than a constant because it is what a dispatch
// line is keyed on: one trace is one call, and a case that gave every line the
// same id would be describing one call five times while reading as five.
func dispatchRecordOf(traceID, action string, requests int, refusal string) e2ecalls.Record {
	return e2ecalls.Record{
		Schema: e2ecalls.SchemaVersion,
		Type:   e2ecalls.TypeDispatch,
		Dispatch: &e2ecalls.Dispatch{
			TraceID:       traceID,
			Action:        action,
			RefusalReason: refusal,
			Requests:      requests,
		},
	}
}

// TestE2EObservation_ClassifiesPerAction is the whole point of reading the
// end-to-end record here: the committed inventory answers "did this action's
// owning package issue anything", and this answers "did this action".
//
// Every case below is a distinction the package grain cannot make. Two actions
// of one package land on opposite sides of it; a refusal is kept off the lead
// list because the server declining to run something is not a handler that
// could not build a request; and an action seen issuing once is observed
// however many later calls of it issued nothing.
func TestE2EObservation_ClassifiesPerAction(t *testing.T) {
	withE2ERecord(t, func(string) ([]e2ecalls.Record, error) {
		return []e2ecalls.Record{
			dispatchRecordOf("4bf92f3577b34da6a3ce929d0e0e4731", "issue.list", 1, ""),
			dispatchRecordOf("4bf92f3577b34da6a3ce929d0e0e4732", "issue.get", 0, ""),
			dispatchRecordOf("4bf92f3577b34da6a3ce929d0e0e4733", "issue.delete", 0, "safe_mode"),
			dispatchRecordOf("4bf92f3577b34da6a3ce929d0e0e4734", "issue.list", 0, ""),
			dispatchRecordOf("4bf92f3577b34da6a3ce929d0e0e4735", "gone.list", 4, ""),
			{Schema: e2ecalls.SchemaVersion, Type: e2ecalls.TypeCall, Call: &e2ecalls.Call{Action: "issue.update"}},
		}, nil
	})
	actions := []requestinventory.Action{
		{ID: "issue.list", Owner: "issues"},
		{ID: "issue.get", Owner: "issues"},
		{ID: "issue.delete", Owner: "issues"},
		{ID: "issue.update", Owner: "issues"},
	}

	observation := e2eObservation("dist/e2e-calls", actions)

	if !observation.Ran || observation.Directory != "dist/e2e-calls" {
		t.Fatalf("observation = %+v, want a run over the named directory", observation)
	}
	if observation.Dispatches != 5 {
		t.Errorf("dispatches = %d, want the five traces and not the call line", observation.Dispatches)
	}
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "issuing", got: observation.Issuing, want: []string{"issue.list"}},
		{name: "silent", got: observation.Silent, want: []string{"issue.get"}},
		{name: "unmatched", got: observation.Unmatched, want: []string{"gone.list"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !slices.Equal(testCase.got, testCase.want) {
				t.Errorf("%s = %v, want %v", testCase.name, testCase.got, testCase.want)
			}
		})
	}
}

// TestE2EObservation_ATraceWrittenTwice_IsStillOneDispatch pins the count
// against the one shape the harness produces on purpose.
//
// A call's dispatch line is written when its test ends and every trace the
// receiver holds is re-offered at the end of the run, while the writer dedupes
// on the exact JSON text — so a trace whose client spans landed in between is
// written a second time with a larger request count, by design. Counting both
// would publish two traces where one call happened, in the field that is the
// denominator of both lists beside it, and the later line is the one to keep
// because a request count is a floor that only grows.
//
// The classification survived this before the fold existed, since one witness
// settles it, which is exactly why only the published number was wrong and
// nothing failed.
func TestE2EObservation_ATraceWrittenTwice_IsStillOneDispatch(t *testing.T) {
	const trace = "4bf92f3577b34da6a3ce929d0e0e4736"
	withE2ERecord(t, func(string) ([]e2ecalls.Record, error) {
		return []e2ecalls.Record{
			dispatchRecordOf(trace, "issue.list", 0, ""),
			dispatchRecordOf(trace, "issue.list", 2, ""),
			// A third line of the same trace, carrying fewer requests than the
			// one already folded: an export that arrived out of order must not
			// take the count back down, which is the half of "highest wins"
			// that a two-line case cannot show.
			dispatchRecordOf(trace, "issue.list", 1, ""),
			dispatchRecordOf("", "issue.get", 0, ""),
			dispatchRecordOf("", "issue.get", 0, ""),
		}, nil
	})
	actions := []requestinventory.Action{
		{ID: "issue.list", Owner: "issues"},
		{ID: "issue.get", Owner: "issues"},
	}

	observation := e2eObservation("dist/e2e-calls", actions)

	// Two for the trace-less pair: a line naming no trace is folded with
	// nothing, since merging those would join two calls that never said they
	// were one.
	if observation.Dispatches != 3 {
		t.Errorf("dispatches = %d, want the one folded trace and the two lines that name none", observation.Dispatches)
	}
	if !slices.Equal(observation.Issuing, []string{"issue.list"}) {
		t.Errorf("issuing = %v, want the trace's larger request count to have been kept", observation.Issuing)
	}
	if !slices.Equal(observation.Silent, []string{"issue.get"}) {
		t.Errorf("silent = %v, want only the action whose trace carried no request", observation.Silent)
	}
}

// TestFoldDispatchesByTrace_ALineThatIsNotADispatchLine_IsFoldedWithNothing
// verifies the three things the fold refuses to read, one at a time.
//
// The guard is one condition with three operands, and each of them answers a
// record the reader does not own the shape of: a shard is written by a Docker
// run of some version of the harness, so a line of another type, a dispatch
// line whose payload never arrived, and one naming no action are all states
// this has to pass over rather than trust. The second is the one that costs a
// panic if the guard is read any other way, since a nil payload is exactly
// what the third operand would dereference.
func TestFoldDispatchesByTrace_ALineThatIsNotADispatchLine_IsFoldedWithNothing(t *testing.T) {
	const trace = "4bf92f3577b34da6a3ce929d0e0e4737"
	kept := dispatchRecordOf(trace, "issue.list", 1, "")
	records := []e2ecalls.Record{
		// A line of another type that still carries a dispatch payload: the
		// type is what says whether the payload is this line's subject, and a
		// reader that took the payload's presence for the answer would count
		// somebody else's call as a dispatch.
		{Schema: e2ecalls.SchemaVersion, Type: e2ecalls.TypeCall, Dispatch: &e2ecalls.Dispatch{
			TraceID: "4bf92f3577b34da6a3ce929d0e0e4738", Action: "issue.update", Requests: 3,
		}},
		// A dispatch line whose payload is missing, which is the shape that
		// makes the order of the three operands load-bearing.
		{Schema: e2ecalls.SchemaVersion, Type: e2ecalls.TypeDispatch},
		// A dispatch line naming no action: nothing can be said about an
		// action the line does not name.
		dispatchRecordOf("4bf92f3577b34da6a3ce929d0e0e4739", "", 2, ""),
		kept,
	}

	folded := foldDispatchesByTrace(records)

	if len(folded) != 1 {
		t.Fatalf("folded %d dispatch(es), want only the one line that is a dispatch of a named action", len(folded))
	}
	if folded[0] != kept.Dispatch {
		t.Errorf("folded = %+v, want the %q line", folded[0], kept.Dispatch.Action)
	}
}

// TestFoldDispatchesByTrace_ALaterLineWithNoMoreRequests_DoesNotReplaceTheFirst
// pins the half of "the highest count wins" that decides a tie.
//
// A request count is documented as a floor: a later line replaces an earlier
// one because it saw more of the trace's client spans, and none of them
// un-happened. A later line that saw no more of them has therefore learned
// nothing, so it does not replace what is already folded, and the first line
// of the trace is what the run is classified from.
func TestFoldDispatchesByTrace_ALaterLineWithNoMoreRequests_DoesNotReplaceTheFirst(t *testing.T) {
	const trace = "4bf92f3577b34da6a3ce929d0e0e4740"
	first := dispatchRecordOf(trace, "issue.list", 2, "")
	first.Dispatch.Tool = "first"
	later := dispatchRecordOf(trace, "issue.list", 2, "")
	later.Dispatch.Tool = "later"

	folded := foldDispatchesByTrace([]e2ecalls.Record{first, later})

	if len(folded) != 1 {
		t.Fatalf("folded %d dispatch(es), want the one trace", len(folded))
	}
	if folded[0].Tool != "first" {
		t.Errorf("folded tool = %q, want %q: a tie keeps the line already folded", folded[0].Tool, "first")
	}
}

// TestE2EObservation_NoDirectory_AsksNothing pins the default every run
// outside a Docker session takes.
//
// The shards are a byproduct of a suite run that CI does not schedule and
// never commits, so "no record was offered" is the ordinary case and has to be
// distinguishable from "a record was read and said nothing". Ran is what tells
// them apart, and a reader of the report has no other way to.
func TestE2EObservation_NoDirectory_AsksNothing(t *testing.T) {
	withE2ERecord(t, func(string) ([]e2ecalls.Record, error) {
		t.Error("the reader was called for a run that named no shard directory")
		return nil, nil
	})

	observation := e2eObservation("", []requestinventory.Action{{ID: "issue.list", Owner: "issues"}})

	if observation.Ran || observation.Directory != "" || observation.Error != "" {
		t.Errorf("observation = %+v, want the zero value", observation)
	}
}

// TestE2EObservation_UnreadableRecord_IsANoteAndNotAFailure verifies the
// failure this check answers with rather than propagates.
//
// It reads a directory a Docker run wrote, which may be missing, half-written
// or from another version of the record. None of that is a fact about the
// server, and failing R-PATH on it would fail a gate about this repository for
// a reason outside it, so the reason is published and the run goes on.
func TestE2EObservation_UnreadableRecord_IsANoteAndNotAFailure(t *testing.T) {
	withE2ERecord(t, func(string) ([]e2ecalls.Record, error) {
		return nil, errors.New("no calls-*.jsonl shard under dist/e2e-calls")
	})

	observation := e2eObservation("dist/e2e-calls", nil)

	if !observation.Ran {
		t.Error("a read that failed was reported as no read at all")
	}
	if observation.Error == "" {
		t.Error("the read failure was absorbed rather than published")
	}
	if len(observation.Issuing) != 0 || len(observation.Silent) != 0 {
		t.Errorf("observation = %+v, want no claim about any action", observation)
	}
}
