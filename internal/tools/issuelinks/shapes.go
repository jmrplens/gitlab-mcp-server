package issuelinks

import (
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).

// UserOutput mirrors gitlab.IssueAuthor / gitlab.IssueAssignee (they share the
// same shape). It surfaces the full user sub-object referenced by an issue
// relation's author and assignees.
type UserOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

// authorOutput converts a *gitlab.IssueAuthor to a *UserOutput, or nil when the
// SDK value is nil.
func authorOutput(a *gitlab.IssueAuthor) *UserOutput {
	if a == nil {
		return nil
	}
	return &UserOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// assigneeOutput converts a *gitlab.IssueAssignee to a *UserOutput, or nil when
// the SDK value is nil.
func assigneeOutput(a *gitlab.IssueAssignee) *UserOutput {
	if a == nil {
		return nil
	}
	return &UserOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// assigneeOutputs converts a slice of *gitlab.IssueAssignee to a slice of
// *UserOutput, skipping nil elements and returning nil for an empty input.
func assigneeOutputs(in []*gitlab.IssueAssignee) []*UserOutput {
	if len(in) == 0 {
		return nil
	}
	out := make([]*UserOutput, 0, len(in))
	for _, a := range in {
		if a == nil {
			continue
		}
		out = append(out, assigneeOutput(a))
	}
	return out
}

// closerOutput converts a *gitlab.IssueCloser to a *UserOutput, or nil when the
// SDK value is nil. IssueCloser shares the user shape mirrored by UserOutput.
func closerOutput(c *gitlab.IssueCloser) *UserOutput {
	if c == nil {
		return nil
	}
	return &UserOutput{
		ID: c.ID, State: c.State, WebURL: c.WebURL,
		Name: c.Name, AvatarURL: c.AvatarURL, Username: c.Username,
	}
}

// ReferencesOutput mirrors gitlab.IssueReferences (the issue references object).
type ReferencesOutput struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

// referencesOutput converts a *gitlab.IssueReferences to a *ReferencesOutput, or
// nil when the SDK value is nil.
func referencesOutput(r *gitlab.IssueReferences) *ReferencesOutput {
	if r == nil {
		return nil
	}
	return &ReferencesOutput{Short: r.Short, Relative: r.Relative, Full: r.Full}
}

// MilestoneOutput mirrors gitlab.Milestone (the milestone object attached to an
// issue relation).
type MilestoneOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	State       string `json:"state"`
	WebURL      string `json:"web_url"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Expired     *bool  `json:"expired,omitempty"`
}

// milestoneOutput converts a *gitlab.Milestone to a *MilestoneOutput, or nil
// when the SDK value is nil.
func milestoneOutput(m *gitlab.Milestone) *MilestoneOutput {
	if m == nil {
		return nil
	}
	return &MilestoneOutput{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description,
		StartDate: toolutil.FormatISOTimePtr(m.StartDate), DueDate: toolutil.FormatISOTimePtr(m.DueDate),
		State: m.State, WebURL: m.WebURL,
		UpdatedAt: toolutil.FormatTimePtr(m.UpdatedAt), CreatedAt: toolutil.FormatTimePtr(m.CreatedAt),
		Expired: m.Expired,
	}
}

// TimeStatsOutput mirrors gitlab.TimeStats (the issue time-tracking object).
type TimeStatsOutput struct {
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int64  `json:"time_estimate"`
	TotalTimeSpent      int64  `json:"total_time_spent"`
}

// timeStatsOutput converts a *gitlab.TimeStats to a *TimeStatsOutput, or nil
// when the SDK value is nil.
func timeStatsOutput(t *gitlab.TimeStats) *TimeStatsOutput {
	if t == nil {
		return nil
	}
	return &TimeStatsOutput{
		HumanTimeEstimate:   t.HumanTimeEstimate,
		HumanTotalTimeSpent: t.HumanTotalTimeSpent,
		TimeEstimate:        t.TimeEstimate,
		TotalTimeSpent:      t.TotalTimeSpent,
	}
}

// TaskCompletionStatusOutput mirrors gitlab.TasksCompletionStatus.
type TaskCompletionStatusOutput struct {
	Count          int64 `json:"count"`
	CompletedCount int64 `json:"completed_count"`
}

// taskCompletionStatusOutput converts a *gitlab.TasksCompletionStatus to a
// *TaskCompletionStatusOutput, or nil when the SDK value is nil.
func taskCompletionStatusOutput(t *gitlab.TasksCompletionStatus) *TaskCompletionStatusOutput {
	if t == nil {
		return nil
	}
	return &TaskCompletionStatusOutput{Count: t.Count, CompletedCount: t.CompletedCount}
}

// IterationOutput mirrors gitlab.GroupIteration (the iteration assigned to an
// issue, EE only). Canonical shape shared via toolutil.
type IterationOutput = toolutil.IterationOutput

// IssueRefOutput is one of the two issues a link joins, source_issue or
// target_issue, as lib/api/entities/issue_link.rb renders them: `using:
// IssueBasic`, the entity every issue listing shares, and not the whole issue
// the issues API answers with.
//
// It is deliberately not shaped like client-go's Issue, which is what
// IssueLink types both positions as and where this type used to come from.
// Twelve of the Issue's keys are ones IssueBasic never renders (external_id,
// health_status, moved_to_id, label_details, references, subscribed, _links,
// issue_link_id, epic_issue_id, epic, iteration and service_desk_reply_to),
// and `subscribed`, published without omitempty, said false on every link for
// something no response measured. The gap is recorded in
// docs/development/upstream-bugs.md.
//
// The last three keys are what IssueBasic sends that client-go's Issue does not
// model, read from the captured response beside the SDK's own decode
// (ADR-0021) and described on [toolutil.IssueBasicExtra]. weight arrives only
// where the issue weights feature is licensed; blocking_issues_count is
// exposed by ee/lib/ee/api/entities/issue_basic.rb with no condition, so an
// Enterprise Edition build sends it licensed or not and Community Edition
// never does, which is why it is a pointer and carries no tier.
type IssueRefOutput struct {
	ID                   int64                       `json:"id"`
	IID                  int64                       `json:"iid"`
	State                string                      `json:"state"`
	Description          string                      `json:"description,omitempty"`
	Author               *UserOutput                 `json:"author,omitempty"`
	Milestone            *MilestoneOutput            `json:"milestone,omitempty"`
	ProjectID            int64                       `json:"project_id"`
	Assignees            []*UserOutput               `json:"assignees,omitempty"`
	Assignee             *UserOutput                 `json:"assignee,omitempty"`
	UpdatedAt            string                      `json:"updated_at,omitempty"`
	ClosedAt             string                      `json:"closed_at,omitempty"`
	ClosedBy             *UserOutput                 `json:"closed_by,omitempty"`
	Title                string                      `json:"title"`
	CreatedAt            string                      `json:"created_at,omitempty"`
	Labels               []string                    `json:"labels,omitempty"`
	Upvotes              int64                       `json:"upvotes,omitempty"`
	Downvotes            int64                       `json:"downvotes,omitempty"`
	DueDate              string                      `json:"due_date,omitempty"`
	WebURL               string                      `json:"web_url"`
	TimeStats            *TimeStatsOutput            `json:"time_stats,omitempty"`
	Confidential         bool                        `json:"confidential"`
	Weight               int64                       `json:"weight,omitempty" tier:"premium"`
	DiscussionLocked     bool                        `json:"discussion_locked"`
	IssueType            string                      `json:"issue_type,omitempty"`
	UserNotesCount       int64                       `json:"user_notes_count,omitempty"`
	MergeRequestCount    int64                       `json:"merge_requests_count,omitempty"`
	TaskCompletionStatus *TaskCompletionStatusOutput `json:"task_completion_status,omitempty"`
	BlockingIssuesCount  *int64                      `json:"blocking_issues_count,omitempty"`
	StartDate            string                      `json:"start_date,omitempty"`
	Type                 string                      `json:"type,omitempty"`
}

// issueRefOutput converts one issue of a link to a *IssueRefOutput, or nil when
// the SDK value is nil, pairing what the SDK decoded with the keys the capture
// read for the same position. It dereferences the SDK's *string IssueType into
// the issue_type scalar.
func issueRefOutput(i *gitlab.Issue, extra toolutil.IssueBasicExtra) *IssueRefOutput {
	if i == nil {
		return nil
	}
	out := &IssueRefOutput{
		ID: i.ID, IID: i.IID, State: i.State,
		Description:          i.Description,
		Author:               authorOutput(i.Author),
		Milestone:            milestoneOutput(i.Milestone),
		ProjectID:            i.ProjectID,
		Assignees:            assigneeOutputs(i.Assignees),
		Assignee:             assigneeOutput(i.Assignee), //nolint:staticcheck // SA1019: IssueBasic still renders the first assignee under this key
		UpdatedAt:            toolutil.FormatTimePtr(i.UpdatedAt),
		ClosedAt:             toolutil.FormatTimePtr(i.ClosedAt),
		ClosedBy:             closerOutput(i.ClosedBy),
		Title:                i.Title,
		CreatedAt:            toolutil.FormatTimePtr(i.CreatedAt),
		Labels:               []string(i.Labels),
		Upvotes:              i.Upvotes,
		Downvotes:            i.Downvotes,
		DueDate:              toolutil.FormatISOTimePtr(i.DueDate),
		WebURL:               i.WebURL,
		TimeStats:            timeStatsOutput(i.TimeStats),
		Confidential:         i.Confidential,
		Weight:               i.Weight,
		DiscussionLocked:     i.DiscussionLocked,
		UserNotesCount:       i.UserNotesCount,
		MergeRequestCount:    i.MergeRequestCount,
		TaskCompletionStatus: taskCompletionStatusOutput(i.TaskCompletionStatus),
		BlockingIssuesCount:  extra.BlockingIssuesCount,
		StartDate:            extra.StartDate,
		Type:                 extra.Type,
	}
	if i.IssueType != nil {
		out.IssueType = *i.IssueType
	}
	return out
}
