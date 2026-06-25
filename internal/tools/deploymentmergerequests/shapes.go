package deploymentmergerequests

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here as local types rather than imported from the
// sibling mergerequests package to preserve the zero-import-cycle constraint
// (C-IMPORTS). The deployment merge requests endpoint returns the full
// []*gl.MergeRequest payload, so Output mirrors gl.MergeRequest (which embeds
// gl.BasicMergeRequest) field-for-field.

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatISOTimePtr renders an optional ISO date (gl.ISOTime) as YYYY-MM-DD.
func formatISOTimePtr(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}

// BasicUserOutput mirrors gl.BasicUser, the compact user object embedded in
// merge-request payloads (author, assignee, assignees, reviewers, merge_user,
// closed_by, merged_by).
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

// basicUserOutput converts a single gl.BasicUser to its output shape, returning
// nil when the SDK value is nil.
func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: formatTimePtr(u.CreatedAt),
	}
}

// basicUserOutputs converts a slice of gl.BasicUser, skipping nil elements and
// returning an empty slice for an empty or all-nil slice.
func basicUserOutputs(users []*gl.BasicUser) []*BasicUserOutput {
	out := make([]*BasicUserOutput, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, basicUserOutput(u))
	}
	return out
}

// MilestoneOutput mirrors gl.Milestone (the merge-request milestone object).
type MilestoneOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	WebURL      string `json:"web_url"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Expired     *bool  `json:"expired,omitempty"`
}

func milestoneOutput(m *gl.Milestone) *MilestoneOutput {
	if m == nil {
		return nil
	}
	return &MilestoneOutput{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description, State: m.State, WebURL: m.WebURL,
		StartDate: formatISOTimePtr(m.StartDate), DueDate: formatISOTimePtr(m.DueDate),
		CreatedAt: formatTimePtr(m.CreatedAt), UpdatedAt: formatTimePtr(m.UpdatedAt),
		Expired: m.Expired,
	}
}

// ReferencesOutput mirrors gl.IssueReferences (the merge-request references object).
type ReferencesOutput struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

func referencesOutput(r *gl.IssueReferences) *ReferencesOutput {
	if r == nil {
		return nil
	}
	return &ReferencesOutput{Short: r.Short, Relative: r.Relative, Full: r.Full}
}

// LabelDetailsOutput mirrors gl.LabelDetails.
type LabelDetailsOutput struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html"`
	TextColor       string `json:"text_color"`
}

func labelDetailsOutputs(details []*gl.LabelDetails) []*LabelDetailsOutput {
	if len(details) == 0 {
		return nil
	}
	out := make([]*LabelDetailsOutput, 0, len(details))
	for _, d := range details {
		if d == nil {
			continue
		}
		out = append(out, &LabelDetailsOutput{
			ID: d.ID, Name: d.Name, Color: d.Color, Description: d.Description,
			DescriptionHTML: d.DescriptionHTML, TextColor: d.TextColor,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TaskCompletionStatusOutput mirrors gl.TasksCompletionStatus.
type TaskCompletionStatusOutput struct {
	Count          int64 `json:"count"`
	CompletedCount int64 `json:"completed_count"`
}

func taskCompletionStatusOutput(t *gl.TasksCompletionStatus) *TaskCompletionStatusOutput {
	if t == nil {
		return nil
	}
	return &TaskCompletionStatusOutput{Count: t.Count, CompletedCount: t.CompletedCount}
}

// TimeStatsOutput mirrors gl.TimeStats (the merge-request time_stats object).
type TimeStatsOutput struct {
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int64  `json:"time_estimate"`
	TotalTimeSpent      int64  `json:"total_time_spent"`
}

func timeStatsPtr(t *gl.TimeStats) *TimeStatsOutput {
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

// MergeRequestUserOutput mirrors gl.MergeRequestUser (the MR "user" object,
// describing the current user's relationship to the merge request).
type MergeRequestUserOutput struct {
	CanMerge bool `json:"can_merge"`
}

func mergeRequestUserOutput(u gl.MergeRequestUser) *MergeRequestUserOutput {
	return &MergeRequestUserOutput{CanMerge: u.CanMerge}
}

// PipelineInfoOutput mirrors gl.PipelineInfo (the compact pipeline object on the
// merge-request "pipeline" key).
type PipelineInfoOutput struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	Name      string `json:"name"`
	WebURL    string `json:"web_url"`
	UpdatedAt string `json:"updated_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func pipelineInfoOutput(p *gl.PipelineInfo) *PipelineInfoOutput {
	if p == nil {
		return nil
	}
	return &PipelineInfoOutput{
		ID: p.ID, IID: p.IID, ProjectID: p.ProjectID, Status: p.Status,
		Source: p.Source, Ref: p.Ref, SHA: p.SHA, Name: p.Name, WebURL: p.WebURL,
		UpdatedAt: formatTimePtr(p.UpdatedAt), CreatedAt: formatTimePtr(p.CreatedAt),
	}
}

// PipelineDetailedStatusIllustrationOutput mirrors gl.DetailedStatusIllustration
// (the detailed_status.illustration object).
type PipelineDetailedStatusIllustrationOutput struct {
	Image string `json:"image"`
}

// PipelineDetailedStatusOutput mirrors gl.DetailedStatus (the head_pipeline
// detailed_status object).
type PipelineDetailedStatusOutput struct {
	Icon         string                                    `json:"icon"`
	Text         string                                    `json:"text"`
	Label        string                                    `json:"label"`
	Group        string                                    `json:"group"`
	Tooltip      string                                    `json:"tooltip"`
	HasDetails   bool                                      `json:"has_details"`
	DetailsPath  string                                    `json:"details_path"`
	Illustration *PipelineDetailedStatusIllustrationOutput `json:"illustration,omitempty"`
	Favicon      string                                    `json:"favicon"`
}

func pipelineDetailedStatusOutput(s *gl.DetailedStatus) *PipelineDetailedStatusOutput {
	if s == nil {
		return nil
	}
	out := &PipelineDetailedStatusOutput{
		Icon: s.Icon, Text: s.Text, Label: s.Label, Group: s.Group,
		Tooltip: s.Tooltip, HasDetails: s.HasDetails, DetailsPath: s.DetailsPath, Favicon: s.Favicon,
	}
	if s.Illustration.Image != "" {
		out.Illustration = &PipelineDetailedStatusIllustrationOutput{Image: s.Illustration.Image}
	}
	return out
}

// PipelineOutput mirrors gl.Pipeline (the full head_pipeline object).
type PipelineOutput struct {
	ID             int64                         `json:"id"`
	IID            int64                         `json:"iid"`
	ProjectID      int64                         `json:"project_id"`
	Status         string                        `json:"status"`
	Source         string                        `json:"source"`
	Ref            string                        `json:"ref"`
	Name           string                        `json:"name"`
	SHA            string                        `json:"sha"`
	BeforeSHA      string                        `json:"before_sha"`
	Tag            bool                          `json:"tag"`
	YamlErrors     string                        `json:"yaml_errors"`
	User           *BasicUserOutput              `json:"user,omitempty"`
	UpdatedAt      string                        `json:"updated_at,omitempty"`
	CreatedAt      string                        `json:"created_at,omitempty"`
	StartedAt      string                        `json:"started_at,omitempty"`
	FinishedAt     string                        `json:"finished_at,omitempty"`
	CommittedAt    string                        `json:"committed_at,omitempty"`
	Duration       int64                         `json:"duration"`
	QueuedDuration int64                         `json:"queued_duration"`
	Coverage       string                        `json:"coverage"`
	WebURL         string                        `json:"web_url"`
	DetailedStatus *PipelineDetailedStatusOutput `json:"detailed_status,omitempty"`
}

func pipelineOutput(p *gl.Pipeline) *PipelineOutput {
	if p == nil {
		return nil
	}
	return &PipelineOutput{
		ID: p.ID, IID: p.IID, ProjectID: p.ProjectID, Status: p.Status,
		Source: string(p.Source), Ref: p.Ref, Name: p.Name, SHA: p.SHA, BeforeSHA: p.BeforeSHA,
		Tag: p.Tag, YamlErrors: p.YamlErrors, User: basicUserOutput(p.User),
		UpdatedAt: formatTimePtr(p.UpdatedAt), CreatedAt: formatTimePtr(p.CreatedAt),
		StartedAt: formatTimePtr(p.StartedAt), FinishedAt: formatTimePtr(p.FinishedAt),
		CommittedAt: formatTimePtr(p.CommittedAt), Duration: p.Duration, QueuedDuration: p.QueuedDuration,
		Coverage: p.Coverage, WebURL: p.WebURL, DetailedStatus: pipelineDetailedStatusOutput(p.DetailedStatus),
	}
}
