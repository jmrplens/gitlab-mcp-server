package grants

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// dispatch is one dispatch line of a recorded run.
func dispatch(action string, requests int, refusal string) e2ecalls.Record {
	return e2ecalls.Record{Type: e2ecalls.TypeDispatch, Dispatch: &e2ecalls.Dispatch{Action: action, Requests: requests, RefusalReason: refusal}}
}

// useE2ECalls points the shard reader at records or a failure for one test.
func useE2ECalls(t *testing.T, records []e2ecalls.Record, err error) {
	t.Helper()
	original := readE2ECalls
	t.Cleanup(func() { readE2ECalls = original })
	readE2ECalls = func(string) ([]e2ecalls.Record, error) { return records, err }
}

// TestE2ECheck_NoDirectory_RunsNothing verifies a run that named no shard
// directory reports the check as not run rather than as nothing compared.
func TestE2ECheck_NoDirectory_RunsNothing(t *testing.T) {
	if got := e2eCheck("", fixtureRecord()); !reflect.DeepEqual(got, E2ECheck{}) {
		t.Errorf("e2eCheck(\"\") = %+v, want nothing", got)
	}
}

// TestE2ECheck_AnUnreadableRecord_IsANoteNotAFailure verifies a directory that
// cannot be read is reported with why, and nothing is compared.
func TestE2ECheck_AnUnreadableRecord_IsANoteNotAFailure(t *testing.T) {
	useE2ECalls(t, nil, errors.New("no such directory"))
	got := e2eCheck("shards", fixtureRecord())
	want := E2ECheck{Ran: true, Directory: "shards", Grain: e2eGrain, Error: "read the end-to-end call record: no such directory"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("e2eCheck = %+v, want %+v", got, want)
	}
}

// TestE2ECheck_EachActionsBusiestTrace_IsHeldToItsMandatoryRequests verifies
// the per-action comparison: an action whose busiest trace carried at least
// its mandatory requests is consistent, retries and a lower second trace
// included; one carrying fewer is a lead; one the derivation says sends
// nothing seen sending is a lead of the other kind, while one seen sending
// nothing is consistent; only the mandatory requests are counted, so an
// action with two of them and an optional one is a lead at one request and
// consistent at two; a declined dispatch, a line of another type, a line
// naming no action and a line with no dispatch are passed over; and an id the
// record does not hold is named once.
func TestE2ECheck_EachActionsBusiestTrace_IsHeldToItsMandatoryRequests(t *testing.T) {
	useE2ECalls(t, []e2ecalls.Record{
		dispatch("issue.update", 3, ""),
		dispatch("issue.update", 1, ""),
		dispatch("two.step", 1, ""),
		dispatch("two.exact", 2, ""),
		dispatch("issue.get", 0, ""),
		dispatch("issue.get", 0, "safe_mode"),
		dispatch("topic.list", 1, ""),
		dispatch("later.get", 0, ""),
		dispatch("later.get", 1, ""),
		dispatch("namespace.list", 9, "confirmation"),
		dispatch("", 2, ""),
		dispatch("gone.action", 1, ""),
		dispatch("gone.action", 1, ""),
		dispatch("quiet.action", 0, ""),
		{Type: e2ecalls.TypeDispatch},
		{Type: e2ecalls.TypeCall},
	}, nil)
	record := fixtureRecord()
	twoMandatory := []actionrequests.RecordRequest{
		rest("GET /a", actionrequests.ClassMandatory),
		rest("GET /b", actionrequests.ClassMandatory),
		rest("GET /c", actionrequests.ClassOptional),
	}
	record.Actions = append(record.Actions,
		actionrequests.RecordAction{ID: "quiet.action"},
		actionrequests.RecordAction{ID: "two.step", Requests: twoMandatory},
		actionrequests.RecordAction{ID: "two.exact", Requests: twoMandatory},
	)
	got := e2eCheck("shards", record)
	want := E2ECheck{
		Ran: true, Directory: "shards", Grain: e2eGrain,
		Compared: 7, Consistent: 4,
		FewerThanMandatory: []CountLead{
			{Action: "issue.get", Mandatory: 1, Observed: 0},
			{Action: "two.step", Mandatory: 2, Observed: 1},
		},
		SentWhereNoneDerived: []CountLead{{Action: "topic.list", Mandatory: 0, Observed: 1}},
		NotInRecord:          []string{"gone.action"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("e2eCheck =\n%+v\nwant\n%+v", got, want)
	}
}
