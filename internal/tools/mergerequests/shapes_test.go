// shapes_test.go holds the blocking merge request converter to the answer
// GitLab sends, one key at a time, through the dependencies listing that
// reaches it.
package mergerequests

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// blockingMergeRequestFor lists the dependencies of merge request 1 with one
// dependency whose blocking merge request is body, and returns that end of it.
func blockingMergeRequestFor(t *testing.T, body string) *BlockingMergeRequestOutput {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1+pathSuffixBlocks {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"project_id":42,"blocking_merge_request":`+body+`}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetDependencies(context.Background(), client, GetDependenciesInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("GetDependencies() unexpected error: %v", err)
	}
	if len(out.Dependencies) != 1 || out.Dependencies[0].BlockingMergeRequest == nil {
		t.Fatalf("GetDependencies() = %+v, want one dependency with its blocking merge request", out.Dependencies)
	}
	return out.Dependencies[0].BlockingMergeRequest
}

// TestBlockingMergeRequestOutput_EachFlagAlone_SetsOnlyItsOwnField sends the
// blocking merge request with one boolean key true at a time and holds the
// output to the baseline with that one field set. Twelve flags are assigned
// in a row here, four of them from deprecated fields the SDK keeps beside
// their replacements, and a fixture that sets them all cannot tell a
// converter reading merge_when_pipeline_succeeds into auto_merge from one
// reading each into its own.
func TestBlockingMergeRequestOutput_EachFlagAlone_SetsOnlyItsOwnField(t *testing.T) {
	baseline := blockingMergeRequestFor(t, `{"id":100,"iid":10}`)
	flags := []string{
		"draft", "auto_merge", "force_remove_source_branch", "squash", "has_conflicts",
		"blocking_discussions_resolved", "imported", "squash_on_merge", "work_in_progress",
		"merge_when_pipeline_succeeds", "should_remove_source_branch", "discussion_locked",
	}
	for _, flag := range flags {
		t.Run(flag, func(t *testing.T) {
			got := blockingMergeRequestFor(t, `{"id":100,"iid":10,"`+flag+`":true}`)
			want := *baseline
			if err := json.Unmarshal([]byte(`{"`+flag+`":true}`), &want); err != nil {
				t.Fatalf("decode the flag into the output type: %v", err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Errorf("with %s true the blocking merge request is\n%+v\nwant\n%+v", flag, *got, want)
			}
		})
	}
}

// TestBlockingMergeRequestOutput_EachScalarReadsItsOwnKey sends a blocking
// merge request in which no two keys share a value and holds every
// non-boolean field to the key it is named after, the deprecated pair
// (reference, merge_status) beside the fields that replaced them.
func TestBlockingMergeRequestOutput_EachScalarReadsItsOwnKey(t *testing.T) {
	got := blockingMergeRequestFor(t, `{
		"id":100,"iid":10,"project_id":42,"source_project_id":43,"target_project_id":44,
		"target_branch":"tgt","source_branch":"src","title":"the title","state":"locked","description":"the description",
		"upvotes":6,"downvotes":7,"user_notes_count":5,"approvals_before_merge":8,
		"author":{"username":"u-author"},"assignee":{"username":"u-assignee"},
		"assignees":[{"username":"u-assignee-1"},{"username":"u-assignee-2"}],"reviewers":[{"username":"u-reviewer"}],
		"closed_by":{"username":"u-closed-by"},"merge_user":{"username":"u-merge-user"},"merged_by":{"username":"u-merged-by"},
		"labels":["l-one","l-two"],"milestone":"m-title","detailed_merge_status":"need_rebase","merge_status":"cannot_be_merged",
		"sha":"sha-head","merge_commit_sha":"sha-merge","squash_commit_sha":"sha-squash",
		"web_url":"http://mr/10","references":{"short":"!10","full":"g/p!10"},"reference":"!10",
		"imported_from":"github","time_stats":{"time_estimate":35,"total_time_spent":36},
		"task_completion_status":{"count":33,"completed_count":34},
		"created_at":"2026-01-01T00:00:01Z","updated_at":"2026-01-01T00:00:02Z","merged_at":"2026-01-01T00:00:03Z",
		"closed_at":"2026-01-01T00:00:04Z","merge_after":"2026-01-01T00:00:05Z","prepared_at":"2026-01-01T00:00:06Z"
	}`)
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"id", got.ID, int64(100)},
		{"iid", got.IID, int64(10)},
		{"project_id", got.ProjectID, int64(42)},
		{"source_project_id", got.SourceProjectID, int64(43)},
		{"target_project_id", got.TargetProjectID, int64(44)},
		{"target_branch", got.TargetBranch, "tgt"},
		{"source_branch", got.SourceBranch, "src"},
		{"title", got.Title, "the title"},
		{"state", got.State, "locked"},
		{"description", got.Description, "the description"},
		{"upvotes", got.Upvotes, int64(6)},
		{"downvotes", got.Downvotes, int64(7)},
		{"user_notes_count", got.UserNotesCount, int64(5)},
		{"approvals_before_merge", got.ApprovalsBeforeMerge, new(int64(8))},
		{"author", userName(got.Author), "u-author"},
		{"assignee", userName(got.Assignee), "u-assignee"},
		{"assignees", userNames(got.Assignees), []string{"u-assignee-1", "u-assignee-2"}},
		{"reviewers", userNames(got.Reviewers), []string{"u-reviewer"}},
		{"closed_by", userName(got.ClosedBy), "u-closed-by"},
		{"merge_user", userName(got.MergeUser), "u-merge-user"},
		{"merged_by", userName(got.MergedBy), "u-merged-by"},
		{"labels", got.Labels, []string{"l-one", "l-two"}},
		{"milestone", got.Milestone, "m-title"},
		{"detailed_merge_status", got.DetailedMergeStatus, "need_rebase"},
		{"merge_status", got.MergeStatus, "cannot_be_merged"},
		{"sha", got.SHA, "sha-head"},
		{"merge_commit_sha", got.MergeCommitSHA, "sha-merge"},
		{"squash_commit_sha", got.SquashCommitSHA, "sha-squash"},
		{"web_url", got.WebURL, "http://mr/10"},
		{"references", func() any {
			if got.References == nil {
				return nil
			}
			return got.References.Full
		}(), "g/p!10"},
		{"reference", got.Reference, "!10"},
		{"imported_from", got.ImportedFrom, "github"},
		{"time_stats", func() any {
			if got.TimeStats == nil {
				return nil
			}
			return []int64{got.TimeStats.TimeEstimate, got.TimeStats.TotalTimeSpent}
		}(), []int64{35, 36}},
		{"task_completion_status", func() any {
			if got.TaskCompletionStatus == nil {
				return nil
			}
			return []int64{got.TaskCompletionStatus.Count, got.TaskCompletionStatus.CompletedCount}
		}(), []int64{33, 34}},
		{"created_at", got.CreatedAt, "2026-01-01T00:00:01Z"},
		{"updated_at", got.UpdatedAt, "2026-01-01T00:00:02Z"},
		{"merged_at", got.MergedAt, "2026-01-01T00:00:03Z"},
		{"closed_at", got.ClosedAt, "2026-01-01T00:00:04Z"},
		{"merge_after", got.MergeAfter, "2026-01-01T00:00:05Z"},
		{"prepared_at", got.PreparedAt, "2026-01-01T00:00:06Z"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !reflect.DeepEqual(c.got, c.want) {
				t.Errorf("%s = %#v, want %#v", c.name, c.got, c.want)
			}
		})
	}
}

// TestBlockingMergeRequestOutput_AbsentKeys_LeaveTheirFieldsEmpty holds a
// blocking merge request GitLab sent with none of the optional keys to empty
// fields rather than a zero time or a nil label list: the two non-pointer
// times format to nothing when zero, and labels are always a list.
func TestBlockingMergeRequestOutput_AbsentKeys_LeaveTheirFieldsEmpty(t *testing.T) {
	got := blockingMergeRequestFor(t, `{"id":100,"iid":10}`)
	if got.CreatedAt != "" || got.UpdatedAt != "" || got.MergeAfter != "" || got.MergedAt != "" || got.PreparedAt != "" {
		t.Errorf("times = %q %q %q %q %q, want every one empty", got.CreatedAt, got.UpdatedAt, got.MergeAfter, got.MergedAt, got.PreparedAt)
	}
	if got.Labels == nil || len(got.Labels) != 0 {
		t.Errorf("Labels = %#v, want an empty list", got.Labels)
	}
	if got.Milestone != "" || got.ApprovalsBeforeMerge != nil || got.ShouldRemoveSourceBranch != nil || got.DiscussionLocked != nil {
		t.Errorf("optional scalars = %q %v %v %v, want each unset", got.Milestone, got.ApprovalsBeforeMerge, got.ShouldRemoveSourceBranch, got.DiscussionLocked)
	}
}
