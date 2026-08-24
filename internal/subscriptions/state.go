package subscriptions

import (
	"encoding/json"
	"time"
)

// activity describes how likely a resource is to change again soon, which
// is what sets its polling cadence.
//
// Note what this is deliberately *not*: a "terminal" flag. GitLab has no
// terminal state for the resources worth watching. Retrying a pipeline
// reuses its ID and moves it from failed back to running
// (POST /projects/:id/pipelines/:pipeline_id/retry retries the jobs in that
// same pipeline), a closed issue can be reopened, and a merged merge
// request can still have its labels or title edited. A watcher that retired
// itself on "success" would therefore go permanently blind to a real change
// its subscriber asked to hear about. Backing off instead keeps the
// guarantee intact and only costs latency, and the subscription lease
// already bounds how long an abandoned watcher can live.
type activity uint8

const (
	// activityUnknown covers resources with no lifecycle field, and
	// content that could not be parsed. Both poll at the base interval.
	activityUnknown activity = iota
	// activityBusy means work is in flight and the next change is likely
	// seconds away.
	activityBusy
	// activitySettled means the resource has finished for now. It can
	// still change — see the type comment — so it is polled slowly rather
	// than dropped.
	activitySettled
)

// settledFactor multiplies the base interval for a settled resource. At the
// default 15s base that yields one poll a minute, which keeps a retried
// pipeline or a reopened issue detectable without holding a share of the
// API budget for something that is probably finished.
const settledFactor = 4

// busyStatuses are the GitLab pipeline and job statuses that mean work is
// in flight. Every other status is treated as settled.
//
// "manual" and "scheduled" sit on the settled side on purpose: both are
// waiting on something outside CI — a human clicking play, or a future
// timestamp — so polling them at the floor would spend the budget on a
// resource that cannot change until that happens.
var busyStatuses = map[string]bool{
	"created":              true,
	"waiting_for_resource": true,
	"preparing":            true,
	"pending":              true,
	"running":              true,
}

// busyStates are the merge-request and issue states that mean the object is
// still open to change. A locked merge request counts as busy because the
// lock is transient.
var busyStates = map[string]bool{
	"opened": true,
	"locked": true,
}

// activityOf reports how active a resource is, from the content its own
// read returned.
//
// Content is parsed permissively: an unrecognized shape yields
// activityUnknown and the base interval, never an error. Cadence is an
// optimization, so a parse failure must degrade to "poll normally" rather
// than break the subscription.
func (k Kind) activityOf(content []byte) activity {
	switch k {
	case KindPipeline, KindPipelineLatest, KindJob:
		var v struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(content, &v) != nil {
			return activityUnknown
		}
		return statusActivity(v.Status)

	case KindPipelineJobs:
		// A pipeline's job list is busy while any single job is.
		var vs []struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(content, &vs) != nil || len(vs) == 0 {
			return activityUnknown
		}
		settled := true
		for _, v := range vs {
			switch statusActivity(v.Status) {
			case activityBusy:
				return activityBusy
			case activityUnknown:
				settled = false
			case activitySettled:
			}
		}
		if settled {
			return activitySettled
		}
		return activityUnknown

	case KindMergeRequest, KindIssue:
		var v struct {
			State string `json:"state"`
		}
		if json.Unmarshal(content, &v) != nil {
			return activityUnknown
		}
		return stateActivity(v.State)

	default:
		// Everything else — wikis, files, branches, labels, boards and the
		// rest — has no lifecycle field to read, so there is nothing to
		// adapt to.
		return activityUnknown
	}
}

// statusActivity maps a pipeline or job status onto an activity.
func statusActivity(status string) activity {
	if status == "" {
		return activityUnknown
	}
	if busyStatuses[status] {
		return activityBusy
	}
	return activitySettled
}

// stateActivity maps a merge-request or issue state onto an activity.
func stateActivity(state string) activity {
	if state == "" {
		return activityUnknown
	}
	if busyStates[state] {
		return activityBusy
	}
	return activitySettled
}

// pollInterval returns how long to wait before re-reading a resource, given
// the content the last read produced.
//
// The floor exists because five seconds per subscription is defensible for
// one watcher and indefensible as a fleet default: a self-managed instance
// that enables the optional authenticated-API throttle gets 120 requests a
// minute, so ten watchers at five seconds would consume such a user's
// entire budget while that same user is making tool calls through it.
func (k Kind) pollInterval(content []byte, base, floor time.Duration) time.Duration {
	if floor > base {
		floor = base
	}
	switch k.activityOf(content) {
	case activityBusy:
		return floor
	case activitySettled:
		return base * settledFactor
	default:
		// activityUnknown, and any activity added later without a cadence
		// of its own: poll at the ordinary rate.
		return base
	}
}
