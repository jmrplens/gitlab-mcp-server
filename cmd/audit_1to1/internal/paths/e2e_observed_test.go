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
func dispatchRecordOf(action string, requests int, refusal string) e2ecalls.Record {
	return e2ecalls.Record{
		Schema: e2ecalls.SchemaVersion,
		Type:   e2ecalls.TypeDispatch,
		Dispatch: &e2ecalls.Dispatch{
			TraceID:       "4bf92f3577b34da6a3ce929d0e0e4736",
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
			dispatchRecordOf("issue.list", 1, ""),
			dispatchRecordOf("issue.get", 0, ""),
			dispatchRecordOf("issue.delete", 0, "safe_mode"),
			dispatchRecordOf("issue.list", 0, ""),
			dispatchRecordOf("gone.list", 4, ""),
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
		t.Errorf("dispatches = %d, want the five dispatch lines and not the call line", observation.Dispatches)
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
