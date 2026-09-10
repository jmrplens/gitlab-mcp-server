// sent_shapes_test.go covers the reader that takes off a captured response the
// keys API::Entities::RelatedIssue sends and client-go's IssueRelation does not
// model, and the two ways that reader refuses an answer it cannot hold.
package issuelinks

import (
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// relationCaptureBody is one related issue carrying every key the reader takes,
// spelled as GitLab spells it on the wire. The two licensed objects are here
// with the keys their own entities render, so a shape that drifted from
// EpicBaseEntity or API::Entities::Iteration fails on this body.
const relationCaptureBody = `[{
	"id": 100,
	"_links": {
		"self": "http://e.com/api/v4/projects/1/issues/8",
		"notes": "http://e.com/api/v4/projects/1/issues/8/notes",
		"award_emoji": "http://e.com/api/v4/projects/1/issues/8/award_emoji",
		"project": "http://e.com/api/v4/projects/1",
		"closed_as_duplicate_of": "http://e.com/api/v4/projects/1/issues/75"
	},
	"blocking_issues_count": 2,
	"closed_at": "2026-01-06T09:00:00Z",
	"closed_by": {"id": 4, "username": "cara", "public_email": "cara@e.com", "name": "Cara",
		"state": "active", "locked": false, "avatar_url": "http://e.com/a.png", "web_url": "http://e.com/cara"},
	"discussion_locked": true,
	"downvotes": 3,
	"epic": {"id": 7, "iid": 2, "group_id": 9, "title": "Quarter goal", "url": "/groups/g/-/epics/2",
		"human_readable_end_date": "Feb 1, 2026", "human_readable_timestamp": "Due in 3 weeks"},
	"epic_iid": 2,
	"has_tasks": true,
	"health_status": "on_track",
	"imported": true,
	"imported_from": "github",
	"issue_type": "issue",
	"iteration": {"id": 11, "iid": 3, "sequence": 4, "group_id": 9, "title": "Sprint 4",
		"description": "the fourth", "state": 2, "web_url": "http://e.com/it/3",
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z",
		"start_date": "2026-01-01", "due_date": "2026-01-14"},
	"merge_requests_count": 5,
	"moved_to_id": 44,
	"service_desk_reply_to": "desk@e.com",
	"severity": "HIGH",
	"start_date": "2026-01-03",
	"task_completion_status": {"count": 4, "completed_count": 2},
	"task_status": "2 of 4 tasks completed",
	"time_stats": {"time_estimate": 3600, "total_time_spent": 1800,
		"human_time_estimate": "1h", "human_total_time_spent": "30m"},
	"type": "ISSUE",
	"upvotes": 6
}]`

// TestCapturedRelations_ReadsEveryKeyTheEntitySends drives the reader over a
// body carrying all twenty-four keys and checks each one arrives with its
// value, including the two nested objects whose shapes are this package's own
// rather than the SDK's.
func TestCapturedRelations_ReadsEveryKeyTheEntitySends(t *testing.T) {
	extras, err := capturedRelations(gitlabclient.CapturedBody([]byte(relationCaptureBody)), 1)
	if err != nil {
		t.Fatalf("capturedRelations() error: %v", err)
	}
	if len(extras) != 1 {
		t.Fatalf("capturedRelations() returned %d relations, want 1", len(extras))
	}
	got := extras[0]

	objects := []struct {
		name string
		ok   func() bool
		show any
	}{
		{"_links", func() bool {
			return got.Links != nil && got.Links.ClosedAsDuplicateOf == "http://e.com/api/v4/projects/1/issues/75"
		}, got.Links},
		{"closed_by", func() bool {
			return got.ClosedBy != nil && got.ClosedBy.PublicEmail == "cara@e.com" && !got.ClosedBy.Locked
		}, got.ClosedBy},
		{"epic", func() bool {
			return got.Epic != nil && got.Epic.URL == "/groups/g/-/epics/2" && got.Epic.HumanReadableEndDate == "Feb 1, 2026"
		}, got.Epic},
		{"iteration", func() bool {
			return got.Iteration != nil && got.Iteration.Sequence == 4 && got.Iteration.State == 2
		}, got.Iteration},
		{"task_completion_status", func() bool {
			return got.TaskCompletionStatus != nil && got.TaskCompletionStatus.CompletedCount == 2
		}, got.TaskCompletionStatus},
		{"time_stats", func() bool {
			return got.TimeStats != nil && got.TimeStats.HumanTotalTimeSpent == "30m"
		}, got.TimeStats},
		{"closed_at", func() bool {
			return got.ClosedAt != nil && got.ClosedAt.Year() == 2026
		}, got.ClosedAt},
	}
	for _, tc := range objects {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.ok() {
				t.Errorf("%s = %+v, want the keys its own entity renders", tc.name, tc.show)
			}
		})
	}

	scalars := []struct {
		name string
		got  any
		want any
	}{
		{"blocking_issues_count", got.BlockingIssuesCount, int64(2)},
		{"discussion_locked", got.DiscussionLocked, true},
		{"downvotes", got.Downvotes, int64(3)},
		{"epic_iid", got.EpicIID, int64(2)},
		{"has_tasks", got.HasTasks, true},
		{"health_status", got.HealthStatus, "on_track"},
		{"imported", got.Imported, true},
		{"imported_from", got.ImportedFrom, "github"},
		{"issue_type", got.IssueType, "issue"},
		{"merge_requests_count", got.MergeRequestsCount, int64(5)},
		{"moved_to_id", got.MovedToID, int64(44)},
		{"service_desk_reply_to", got.ServiceDeskReplyTo, "desk@e.com"},
		{"severity", got.Severity, "HIGH"},
		{"start_date", got.StartDate, "2026-01-03"},
		{"task_status", got.TaskStatus, "2 of 4 tasks completed"},
		{"type", got.Type, "ISSUE"},
		{"upvotes", got.Upvotes, int64(6)},
	}
	for _, tc := range scalars {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestCapturedRelations_AbsentKeysStayZero checks the other side of every
// condition: a body with none of the licensed or content-gated keys leaves
// them empty rather than inventing a value, which is what keeps an unlicensed
// instance from being told an issue is on track.
func TestCapturedRelations_AbsentKeysStayZero(t *testing.T) {
	extras, err := capturedRelations(gitlabclient.CapturedBody([]byte(`[{"id": 1}]`)), 1)
	if err != nil {
		t.Fatalf("capturedRelations() error: %v", err)
	}
	got := extras[0]
	if got.Epic != nil || got.EpicIID != 0 || got.Iteration != nil || got.HealthStatus != "" || got.TaskStatus != "" {
		t.Errorf("capturedRelations() invented a gated key: %+v", got)
	}
	if got.Links != nil || got.ClosedBy != nil || got.ClosedAt != nil || got.TimeStats != nil {
		t.Errorf("capturedRelations() invented an object: %+v", got)
	}
}

// TestCapturedRelations_RefusesAnAnswerItCannotHold covers the reader's two
// refusals: a list whose length is not the one the SDK decoded, and a body
// that is not a list of relations at all.
func TestCapturedRelations_RefusesAnAnswerItCannotHold(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		decoded int
		want    string
	}{
		{
			name:    "count disagrees with the SDK",
			body:    `[{"id": 1}, {"id": 2}]`,
			decoded: 1,
			want:    "holds 2 issue relations and the SDK decoded 1",
		},
		{
			name:    "object where GitLab sends a list",
			body:    `{"id": 1}`,
			decoded: 1,
			want:    "cannot",
		},
		{
			name:    "a key typed as something it is not",
			body:    `[{"blocking_issues_count": "two"}]`,
			decoded: 1,
			want:    "cannot",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := capturedRelations(gitlabclient.CapturedBody([]byte(tc.body)), tc.decoded)
			if err == nil {
				t.Fatalf("capturedRelations() accepted %s", tc.name)
			}
			if tc.want == "cannot" {
				return
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("capturedRelations() error = %v, want it to name both counts", err)
			}
		})
	}
}

// TestCapturedRelations_NoResponseCaptured covers the path where nothing
// reached the transport under the capture, which is what a handler sees when
// the request was made with another context.
func TestCapturedRelations_NoResponseCaptured(t *testing.T) {
	_, err := capturedRelations(&gitlabclient.ResponseCapture{}, 0)
	if err == nil {
		t.Fatal("capturedRelations() accepted a capture that saw no response")
	}
}
