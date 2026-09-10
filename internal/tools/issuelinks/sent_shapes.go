package issuelinks

import (
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// RelationEpicOutput is the epic a related issue belongs to, as
// ee/app/serializers/epic_base_entity.rb renders it.
//
// It is deliberately not [EpicOutput]. That type mirrors gl.Epic, the whole
// epic the epics API answers with; the object beside `epic_iid` on a related
// issue comes from EpicBaseEntity, which is five keys and two more when the
// epic has dates. Reusing the large shape here would advertise twenty keys
// this endpoint has never sent.
//
// The two human-readable keys are rendered only when the epic carries the
// dates behind them, so both are omitted rather than published empty.
type RelationEpicOutput struct {
	ID                     int64  `json:"id"`
	IID                    int64  `json:"iid"`
	GroupID                int64  `json:"group_id"`
	Title                  string `json:"title"`
	URL                    string `json:"url"`
	HumanReadableEndDate   string `json:"human_readable_end_date,omitempty"`
	HumanReadableTimestamp string `json:"human_readable_timestamp,omitempty"`
}

// RelationLinksOutput is the `_links` object lib/api/entities/issue.rb
// renders on an issue: the four API URLs and, for an issue closed as a
// duplicate, the URL of the issue it duplicates.
//
// It is not [LinksOutput], which mirrors gl.IssueLinks and stops at four keys
// because client-go's struct does. The fifth is rendered by the same nested
// block as the others and is empty on an issue that duplicates nothing.
type RelationLinksOutput struct {
	Self                string `json:"self,omitempty"`
	Notes               string `json:"notes,omitempty"`
	AwardEmoji          string `json:"award_emoji,omitempty"`
	Project             string `json:"project,omitempty"`
	ClosedAsDuplicateOf string `json:"closed_as_duplicate_of,omitempty"`
}

// relationExtra is what lib/api/entities/related_issue.rb sends on a related
// issue that client-go's IssueRelation does not carry, read from the captured
// response beside the SDK's own decode (ADR-0021). The gap is recorded in
// docs/development/upstream-bugs.md.
//
// RelatedIssue inherits API::Entities::Issue, so nineteen of these are on
// every response of GET /projects/:id/issues/:issue_iid/links. Four are
// licensed: `epic` and `epic_iid` need the epics feature on the issue's
// group, `iteration` needs iterations, and `health_status` needs issuable
// health status. `task_status` is gated on the issue's own content rather
// than on a license or a permission.
//
// `subscribed` is exposed by the same entity and is not read here: that route
// passes `include_subscribed: false`, so the key is never on its responses.
// The finding is answered in cmd/audit_1to1/internal/paths/sent_declarations.go.
type relationExtra struct {
	Links                *RelationLinksOutput        `json:"_links"`
	BlockingIssuesCount  int64                       `json:"blocking_issues_count"`
	ClosedAt             *time.Time                  `json:"closed_at"`
	ClosedBy             *toolutil.UserBasicOutput   `json:"closed_by"`
	DiscussionLocked     bool                        `json:"discussion_locked"`
	Downvotes            int64                       `json:"downvotes"`
	Epic                 *RelationEpicOutput         `json:"epic"`
	EpicIID              int64                       `json:"epic_iid"`
	HasTasks             bool                        `json:"has_tasks"`
	HealthStatus         string                      `json:"health_status"`
	Imported             bool                        `json:"imported"`
	ImportedFrom         string                      `json:"imported_from"`
	IssueType            string                      `json:"issue_type"`
	Iteration            *IterationOutput            `json:"iteration"`
	MergeRequestsCount   int64                       `json:"merge_requests_count"`
	MovedToID            int64                       `json:"moved_to_id"`
	ServiceDeskReplyTo   string                      `json:"service_desk_reply_to"`
	Severity             string                      `json:"severity"`
	StartDate            string                      `json:"start_date"`
	TaskCompletionStatus *TaskCompletionStatusOutput `json:"task_completion_status"`
	TaskStatus           string                      `json:"task_status"`
	TimeStats            *TimeStatsOutput            `json:"time_stats"`
	Type                 string                      `json:"type"`
	Upvotes              int64                       `json:"upvotes"`
}

// capturedRelations reads the keys client-go's IssueRelation does not model
// off the answer to the relation list, one per relation in the list's order.
//
// The count is held to what the SDK decoded: the two read the same bytes, so
// a difference is a fault in this reader rather than in the answer.
func capturedRelations(capture *gitlabclient.ResponseCapture, decoded int) ([]relationExtra, error) {
	var extras []relationExtra
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d issue relations and the SDK decoded %d", len(extras), decoded)
	}
	return extras, nil
}
