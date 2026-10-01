package grants

import (
	"fmt"
	"slices"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// E2ECheck holds the derivation to what an end-to-end run saw each action
// send, which is the one grain finer than the package: the harness stamps a
// trace into each call, the server's span names the action the dispatcher
// ran, and every GitLab request the handler made is a client span of that
// trace (e2ecalls.Dispatch.Requests).
//
// It compares counts, not routes, until the dispatch line carries the route of
// each client span, and a count is a floor: the spans travel through a
// batching processor that drops silently when its queue overflows. So a count
// at or above what the derivation makes on every path is consistent and says
// nothing about which routes were sent, and one below it is a lead, either an
// over-approximation of the derivation or a dropped span. It reports and
// never gates, for the reasons R-PATH's end-to-end observation gives.
type E2ECheck struct {
	// Ran is whether a record was read at all; Error says why not, when one
	// was asked for and could not be read.
	Ran       bool   `json:"ran"`
	Directory string `json:"directory,omitempty"`
	Error     string `json:"error,omitempty"`
	Grain     string `json:"grain,omitempty"`
	// Compared counts the actions of the request record a dispatch ran and
	// the server did not decline, and Consistent those no lead below names.
	Compared   int `json:"actions_compared"`
	Consistent int `json:"actions_consistent"`
	// FewerThanMandatory are actions whose busiest trace carried fewer
	// requests than the derivation says every path makes.
	FewerThanMandatory []CountLead `json:"fewer_requests_than_mandatory,omitempty"`
	// SentWhereNoneDerived are actions the derivation says send nothing that
	// a trace saw sending.
	SentWhereNoneDerived []CountLead `json:"requests_where_none_derived,omitempty"`
	// NotInRecord are dispatched ids the request record does not hold, named
	// so a run read against a record it no longer describes says so.
	NotInRecord []string `json:"dispatched_ids_not_in_the_record,omitempty"`
}

// CountLead is one action whose observed request count disagrees with the
// derivation.
type CountLead struct {
	Action string `json:"action"`
	// Mandatory is how many requests the derivation says every path makes,
	// and Observed the most one trace of the action carried.
	Mandatory int `json:"mandatory"`
	Observed  int `json:"observed"`
}

// e2eGrain is what [E2ECheck.Grain] says, spelled once.
const e2eGrain = "action, by count: consistent when the most requests one trace of the action carried is at least the number the derivation makes on every path; a count cannot say which routes were sent"

// e2eCheck folds the dispatch lines of a recorded run into the per-action
// comparison. An empty directory is not an error: no end-to-end record was
// offered, which is every run outside a Docker session.
func e2eCheck(dir string, record actionrequests.Record) E2ECheck {
	if dir == "" {
		return E2ECheck{}
	}
	check := E2ECheck{Ran: true, Directory: dir, Grain: e2eGrain}
	records, err := readE2ECalls(dir)
	if err != nil {
		check.Error = fmt.Sprintf("read the end-to-end call record: %v", err)
		return check
	}
	mandatory := map[string]int{}
	sends := map[string]bool{}
	for _, action := range record.Actions {
		mandatory[action.ID] = 0
		sends[action.ID] = len(action.Requests) > 0
		for _, request := range action.Requests {
			if request.Class == actionrequests.ClassMandatory {
				mandatory[action.ID]++
			}
		}
	}
	observed, unknown := observedCounts(records, mandatory)
	check.NotInRecord = unknown
	ids := make([]string, 0, len(observed))
	for id := range observed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		check.Compared++
		lead := CountLead{Action: id, Mandatory: mandatory[id], Observed: observed[id]}
		switch {
		case !sends[id] && lead.Observed > 0:
			check.SentWhereNoneDerived = append(check.SentWhereNoneDerived, lead)
		case lead.Observed < lead.Mandatory:
			check.FewerThanMandatory = append(check.FewerThanMandatory, lead)
		default:
			check.Consistent++
		}
	}
	return check
}

// observedCounts is, per action of the record a dispatch ran and the server
// did not decline, the most requests one of its dispatch lines carried, with
// the dispatched ids the record does not hold. A trace written twice is read
// twice and changes nothing, since only the highest count is kept; a declined
// dispatch is passed over, because the server declining to run an action is
// not a handler that sent less.
func observedCounts(records []e2ecalls.Record, known map[string]int) (observed map[string]int, unknown []string) {
	observed = map[string]int{}
	for _, record := range records {
		dispatch := record.Dispatch
		if record.Type != e2ecalls.TypeDispatch || dispatch == nil || dispatch.Action == "" || dispatch.RefusalReason != "" {
			continue
		}
		if _, ok := known[dispatch.Action]; !ok {
			if !slices.Contains(unknown, dispatch.Action) {
				unknown = append(unknown, dispatch.Action)
			}
			continue
		}
		if count, seen := observed[dispatch.Action]; !seen || dispatch.Requests > count {
			observed[dispatch.Action] = dispatch.Requests
		}
	}
	sort.Strings(unknown)
	return observed, unknown
}
