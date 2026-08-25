// state_test.go validates the polling cadence: how a watcher reads a
// resource's own content to decide whether the next change is seconds away
// or probably never, and how that maps onto an interval.
//
// The load-bearing property is that no state is treated as terminal. GitLab
// reuses a pipeline's ID when it is retried, reopens issues, and lets a
// merged merge request be edited — so a watcher that stopped on "success"
// would go permanently blind to a change its subscriber asked for. Settling
// slows a watcher down; it never stops one.
package subscriptions

import (
	"testing"
	"time"
)

func TestActivityOf_PipelineStatus_MapsToActivity(t *testing.T) {
	tests := []struct {
		status string
		want   activity
	}{
		// In flight.
		{"created", activityBusy},
		{"waiting_for_resource", activityBusy},
		{"preparing", activityBusy},
		{"pending", activityBusy},
		{"running", activityBusy},

		// Finished, but every one of these can move again.
		{"success", activitySettled},
		{"failed", activitySettled},
		{"canceled", activitySettled},
		{"skipped", activitySettled},

		// Blocked on something outside CI, so polling fast buys nothing.
		{"manual", activitySettled},
		{"scheduled", activitySettled},

		// Unrecognized or absent.
		{"", activityUnknown},
		{"some_future_status", activitySettled},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			content := []byte(`{"id":1,"status":"` + tt.status + `"}`)
			if got := KindPipeline.activityOf(content); got != tt.want {
				t.Errorf("KindPipeline.activityOf(%s) = %v, want %v", content, got, tt.want)
			}
		})
	}
}

func TestActivityOf_MergeRequestAndIssueState_MapsToActivity(t *testing.T) {
	tests := []struct {
		name  string
		kind  Kind
		state string
		want  activity
	}{
		{"open mr", KindMergeRequest, "opened", activityBusy},
		{"locked mr", KindMergeRequest, "locked", activityBusy},
		{"merged mr", KindMergeRequest, "merged", activitySettled},
		{"closed mr", KindMergeRequest, "closed", activitySettled},
		{"open issue", KindIssue, "opened", activityBusy},
		{"closed issue", KindIssue, "closed", activitySettled},
		{"missing state", KindIssue, "", activityUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(`{"iid":1,"state":"` + tt.state + `"}`)
			if got := tt.kind.activityOf(content); got != tt.want {
				t.Errorf("%v.activityOf(%s) = %v, want %v", tt.kind, content, got, tt.want)
			}
		})
	}
}

// TestActivityOf_PipelineJobs_IsBusyWhileAnyJobIs verifies a job list
// follows its busiest member, so a pipeline whose last job is still running
// keeps polling fast even when every other job has finished.
func TestActivityOf_PipelineJobs_IsBusyWhileAnyJobIs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    activity
	}{
		{"all finished", `[{"status":"success"},{"status":"failed"}]`, activitySettled},
		{"one still running", `[{"status":"success"},{"status":"running"}]`, activityBusy},
		{"first still pending", `[{"status":"pending"},{"status":"success"}]`, activityBusy},
		{"empty list", `[]`, activityUnknown},
		{"one job with no status", `[{"status":"success"},{}]`, activityUnknown},
		{"not a list", `{"status":"success"}`, activityUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindPipelineJobs.activityOf([]byte(tt.content)); got != tt.want {
				t.Errorf("KindPipelineJobs.activityOf(%s) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// TestActivityOf_KindWithoutLifecycle_IsUnknown verifies resources with no
// state field poll at the base interval rather than being guessed at.
func TestActivityOf_KindWithoutLifecycle_IsUnknown(t *testing.T) {
	kinds := []Kind{
		KindProject, KindWiki, KindFile, KindBranch, KindTag,
		KindRelease, KindLabel, KindBoard, KindDeployKey,
		KindGroup, KindGroupLabel, KindSnippet,
	}
	// Deliberately carries a status field: these kinds must ignore it
	// rather than read a lifecycle into content that has none.
	content := []byte(`{"id":1,"status":"running"}`)
	for _, k := range kinds {
		t.Run(k.String(), func(t *testing.T) {
			if got := k.activityOf(content); got != activityUnknown {
				t.Errorf("%v.activityOf(...) = %v, want activityUnknown", k, got)
			}
		})
	}
}

// TestActivityOf_UnparseableContent_IsUnknown verifies malformed content
// degrades to the base interval instead of breaking the subscription.
//
// Cadence is an optimisation. A resource whose body this package cannot
// read must still be watched — just at the ordinary rate.
func TestActivityOf_UnparseableContent_IsUnknown(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		content string
	}{
		{"truncated json", KindPipeline, `{"status":`},
		{"not json at all", KindPipeline, `<html>500</html>`},
		{"empty", KindPipeline, ``},
		{"wrong type for status", KindPipeline, `{"status":123}`},
		{"array where object expected", KindPipeline, `[{"status":"running"}]`},
		{"truncated job list", KindPipelineJobs, `[{"status":"running"`},
		{"wrong type for state", KindIssue, `{"state":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.activityOf([]byte(tt.content)); got != activityUnknown {
				t.Errorf("%v.activityOf(%q) = %v, want activityUnknown", tt.kind, tt.content, got)
			}
		})
	}
}

func TestPollInterval_FollowsActivity(t *testing.T) {
	const (
		base  = 15 * time.Second
		floor = 5 * time.Second
	)
	tests := []struct {
		name    string
		kind    Kind
		content string
		want    time.Duration
	}{
		{"running pipeline polls at the floor", KindPipeline, `{"status":"running"}`, floor},
		{"finished pipeline backs off", KindPipeline, `{"status":"success"}`, base * settledFactor},
		{"failed pipeline backs off but keeps watching", KindPipeline, `{"status":"failed"}`, base * settledFactor},
		{"open issue polls at the floor", KindIssue, `{"state":"opened"}`, floor},
		{"closed issue backs off", KindIssue, `{"state":"closed"}`, base * settledFactor},
		{"lifecycle-free kind uses the base", KindWiki, `{"content":"x"}`, base},
		{"unparseable uses the base", KindPipeline, `nonsense`, base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.kind.pollInterval([]byte(tt.content), base, floor)
			if got != tt.want {
				t.Errorf("%v.pollInterval(%s) = %v, want %v", tt.kind, tt.content, got, tt.want)
			}
		})
	}
}

// TestPollInterval_NoStateIsTerminal verifies every finished state still
// yields a finite, pollable interval.
//
// This is the executable form of the design decision: because GitLab reuses
// a pipeline ID on retry and reopens issues, a watcher must never stop. Any
// future change that introduced a "stop polling" sentinel — a zero or
// negative interval — fails here.
func TestPollInterval_NoStateIsTerminal(t *testing.T) {
	const (
		base  = 15 * time.Second
		floor = 5 * time.Second
	)
	finished := []struct {
		kind    Kind
		content string
	}{
		{KindPipeline, `{"status":"success"}`},
		{KindPipeline, `{"status":"failed"}`},
		{KindPipeline, `{"status":"canceled"}`},
		{KindPipeline, `{"status":"skipped"}`},
		{KindJob, `{"status":"success"}`},
		{KindPipelineJobs, `[{"status":"success"}]`},
		{KindMergeRequest, `{"state":"merged"}`},
		{KindMergeRequest, `{"state":"closed"}`},
		{KindIssue, `{"state":"closed"}`},
	}
	for _, tt := range finished {
		t.Run(tt.kind.String()+"/"+tt.content, func(t *testing.T) {
			got := tt.kind.pollInterval([]byte(tt.content), base, floor)
			if got <= 0 {
				t.Fatalf("pollInterval = %v; a finished resource must still be polled, "+
					"because GitLab can revive it (pipeline retry reuses the ID, issues reopen)", got)
			}
			if got <= base {
				t.Errorf("pollInterval = %v, want a backed-off interval greater than the base %v", got, base)
			}
		})
	}
}

// TestPollInterval_FloorAboveBase_IsClamped verifies a misconfigured floor
// can never make a busy resource poll slower than a settled one.
func TestPollInterval_FloorAboveBase_IsClamped(t *testing.T) {
	const base = 10 * time.Second
	floor := 30 * time.Second // nonsensical: floor should never exceed base

	busy := KindPipeline.pollInterval([]byte(`{"status":"running"}`), base, floor)
	if busy != base {
		t.Errorf("busy interval = %v, want it clamped to the base %v", busy, base)
	}
	settled := KindPipeline.pollInterval([]byte(`{"status":"success"}`), base, floor)
	if busy > settled {
		t.Errorf("busy interval %v exceeds settled interval %v — the cadence is inverted", busy, settled)
	}
}
