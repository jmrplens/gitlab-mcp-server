package paths

import (
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// E2EObservation is what a recorded end-to-end run says about which actions
// were seen issuing a request, at the grain the committed inventory cannot
// reach.
//
// The observation check above joins on the owning package, because the unit
// suite's recorder sees a request on the httptest server's own goroutine where
// nothing names an action. An end-to-end run has the one thing that recording
// lacks: the harness stamps a trace id into each MCP call, the server's span
// carries it back with the route the dispatcher actually chose, and every
// GitLab request the handler made is a child of that span. One trace is one
// call is one action, so "this action issued a request" is a question the
// shards can answer and the inventory cannot.
//
// It is reported and never gated, for two reasons that are both about what a
// missing shard means. The shards are a byproduct of a Docker run that CI does
// not schedule and never commits, so an absent record is the ordinary case and
// failing on it would fail every push. And the counts are floors: the spans
// travel through a batching processor that drops silently when its queue
// overflows, so a trace whose client spans were dropped reads as an action
// that issued nothing. A positive claim here is therefore solid and a negative
// one is a lead.
type E2EObservation struct {
	// Ran is whether a record was read at all. Everything below is empty when
	// it is false, and Error says why when the reason was not "none was asked
	// for".
	Ran bool `json:"ran"`
	// Directory is where the shards were read from.
	Directory string `json:"directory,omitempty"`
	// Error is what stopped the read, when something did. It is a note rather
	// than a failure: this check reports.
	Error string `json:"error,omitempty"`
	// Dispatches is how many traces named an action, which is the denominator
	// of the two lists below. It is traces and not lines: one trace can be
	// written twice, and [foldDispatchesByTrace] is what keeps the count and
	// the word agreeing.
	Dispatches int `json:"dispatches"`
	// Issuing are the catalog actions a dispatch span named whose trace also
	// carried at least one GitLab request. This is the per-action observation,
	// and it is the one claim here a dropped span cannot invent.
	Issuing []string `json:"actions_issuing_requests"`
	// Silent are the actions that ran, were not refused, and whose trace
	// carried no request at all. This is the "issued NONE despite running"
	// lead: an action that reached no GitLab either does its work without one,
	// or could not build a request, or had its spans dropped.
	Silent []string `json:"actions_that_ran_and_issued_nothing"`
	// Unmatched are the dispatched ids the catalog does not hold. It exists so
	// a join that matched nothing says so: a record read against a catalog it
	// no longer describes would otherwise report every action silent and look
	// like a finding about the server.
	Unmatched []string `json:"dispatched_ids_not_in_the_catalog,omitempty"`
}

// readE2ECalls is a seam: the shards are written by a Docker run this process
// cannot perform, so a test hands the reader its own directory or its own
// failure.
var readE2ECalls = e2ecalls.Read

// e2eObservation folds the dispatch lines of a recorded run into the per-action
// answer, held against the catalog so an id nothing registers is named rather
// than counted.
//
// An empty directory is not an error: it means no end-to-end record was
// offered, which is what every run outside a Docker session looks like.
func e2eObservation(dir string, actions []requestinventory.Action) E2EObservation {
	if dir == "" {
		return E2EObservation{}
	}
	observation := E2EObservation{Ran: true, Directory: dir}
	records, err := readE2ECalls(dir)
	if err != nil {
		observation.Error = fmt.Sprintf("read the end-to-end call record: %v", err)
		return observation
	}

	catalog := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		catalog[action.ID] = struct{}{}
	}

	issuing, silent, unmatched := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, dispatch := range foldDispatchesByTrace(records) {
		observation.Dispatches++
		if _, known := catalog[dispatch.Action]; !known {
			unmatched[dispatch.Action] = struct{}{}
			continue
		}
		switch {
		case dispatch.Requests > 0:
			issuing[dispatch.Action] = struct{}{}
		case dispatch.RefusalReason != "":
			// The server declined to run it, so no request was ever going to
			// be made. A safe-mode preview and a confirmation guard both land
			// here, and reporting them as actions that issued nothing would
			// bury the leads that are about a handler under the modes that
			// exist to stop one.
		default:
			silent[dispatch.Action] = struct{}{}
		}
	}

	// An action seen issuing a request once is observed, whatever a second
	// call of it did: the claim is about the action's request having been
	// seen, and one witness is enough to answer it.
	for action := range issuing {
		delete(silent, action)
	}
	observation.Issuing = sortedActions(issuing)
	observation.Silent = sortedActions(silent)
	observation.Unmatched = sortedActions(unmatched)
	return observation
}

// foldDispatchesByTrace reduces a record to one dispatch per trace, keeping the
// line that saw the most requests, in the order the traces first appear.
//
// One trace is one call is one action, and a trace can legitimately be written
// twice: the harness flushes a dispatch line when its test ends and re-offers
// every trace the receiver holds at the end of the run, while the writer's
// dedupe is on the exact JSON text, so a line whose request count grew between
// the two writes is written again rather than replacing the first. Counting
// both would inflate [E2EObservation.Dispatches], which is published as a count
// of traces, and would classify a call from the earliest and least informed
// line the run produced.
//
// The highest count wins for the reason [e2ecalls.Dispatch.Requests] is
// documented as a floor: a later line saw more of the trace's client spans, and
// none of them un-happened.
//
// A line naming no trace is folded with nothing. Every line the harness writes
// carries a trace id, so one without comes from a record this reader does not
// know, and folding two of those together on an empty key would merge two calls
// into one.
func foldDispatchesByTrace(records []e2ecalls.Record) []*e2ecalls.Dispatch {
	folded := make([]*e2ecalls.Dispatch, 0, len(records))
	byTrace := make(map[string]int, len(records))
	for _, record := range records {
		if record.Type != e2ecalls.TypeDispatch || record.Dispatch == nil || record.Dispatch.Action == "" {
			continue
		}
		dispatch := record.Dispatch
		if dispatch.TraceID == "" {
			folded = append(folded, dispatch)
			continue
		}
		at, known := byTrace[dispatch.TraceID]
		if !known {
			byTrace[dispatch.TraceID] = len(folded)
			folded = append(folded, dispatch)
			continue
		}
		if dispatch.Requests > folded[at].Requests {
			folded[at] = dispatch
		}
	}
	return folded
}

// sortedActions renders a set of action ids as the sorted list a report prints.
func sortedActions(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
