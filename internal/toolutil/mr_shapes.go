package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TimeStatsOutput mirrors gl.TimeStats (the time-tracking sub-object on merge
// requests and issues). It is the pure nested form — no next_steps — used when
// time_stats appears as a sub-field of another output type rather than as the
// standalone return value of a time-tracking handler. The standalone handlers
// (SetTimeEstimate, AddSpentTime, etc.) compose this with HintableOutput to add
// next_steps at the top level of their response.
type TimeStatsOutput struct {
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int64  `json:"time_estimate"`
	TotalTimeSpent      int64  `json:"total_time_spent"`
}

// NewTimeStatsOutput converts a gl.TimeStats pointer to the canonical pure
// TimeStatsOutput, returning nil when the source is nil.
func NewTimeStatsOutput(t *gl.TimeStats) *TimeStatsOutput {
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

// MergeRequestOutput mirrors gl.MergeRequest (the full merge-request payload
// returned by MR list/get/create/update endpoints). It is the canonical Output
// shape shared by the mergerequests and deploymentmergerequests domains via type
// aliases, eliminating the ~80-line duplicated struct block identified by
// SonarCloud while avoiding cross-package import cycles (ADR-0004).
//
// HintableOutput is embedded first so next_steps appears at the top of the
// serialized JSON, giving LLMs workflow guidance before parsing the payload.
type MergeRequestOutput struct {
	HintableOutput
	ID                          int64                       `json:"id"`
	IID                         int64                       `json:"iid"`
	ProjectID                   int64                       `json:"project_id"`
	SourceProjectID             int64                       `json:"source_project_id,omitempty"`
	TargetProjectID             int64                       `json:"target_project_id,omitempty"`
	Title                       string                      `json:"title"`
	Description                 string                      `json:"description"`
	State                       string                      `json:"state"`
	Imported                    bool                        `json:"imported,omitempty"`
	ImportedFrom                string                      `json:"imported_from,omitempty"`
	SourceBranch                string                      `json:"source_branch"`
	TargetBranch                string                      `json:"target_branch"`
	WebURL                      string                      `json:"web_url"`
	DetailedMergeStatus         string                      `json:"detailed_merge_status,omitempty"`
	Draft                       bool                        `json:"draft"`
	WorkInProgress              bool                        `json:"work_in_progress,omitempty"`
	HasConflicts                bool                        `json:"has_conflicts"`
	BlockingDiscussionsResolved bool                        `json:"blocking_discussions_resolved"`
	Squash                      bool                        `json:"squash,omitempty"`
	SquashOnMerge               bool                        `json:"squash_on_merge,omitempty"`
	MergeWhenPipelineSucceeds   bool                        `json:"merge_when_pipeline_succeeds,omitempty"`
	ShouldRemoveSourceBranch    bool                        `json:"should_remove_source_branch,omitempty"`
	AllowMaintainerToPush       bool                        `json:"allow_maintainer_to_push,omitempty"`
	DiscussionLocked            bool                        `json:"discussion_locked"`
	RebaseInProgress            bool                        `json:"rebase_in_progress,omitempty"`
	Author                      *BasicUserOutput            `json:"author,omitempty"`
	Assignee                    *BasicUserOutput            `json:"assignee,omitempty"`
	MergeUser                   *BasicUserOutput            `json:"merge_user,omitempty"`
	MergedBy                    *BasicUserOutput            `json:"merged_by,omitempty"`
	ClosedBy                    *BasicUserOutput            `json:"closed_by,omitempty"`
	Assignees                   []*BasicUserOutput          `json:"assignees"`
	Reviewers                   []*BasicUserOutput          `json:"reviewers"`
	Labels                      []string                    `json:"labels"`
	LabelDetails                []*LabelDetailsOutput       `json:"label_details,omitempty"`
	Milestone                   *MRMilestoneOutput          `json:"milestone,omitempty"`
	References                  *ReferencesOutput           `json:"references,omitempty"`
	SHA                         string                      `json:"sha,omitempty"`
	MergeCommitSHA              string                      `json:"merge_commit_sha,omitempty"`
	MergeError                  string                      `json:"merge_error,omitempty"`
	ChangesCount                string                      `json:"changes_count,omitempty"`
	DivergedCommitsCount        int64                       `json:"diverged_commits_count,omitempty"`
	Upvotes                     int64                       `json:"upvotes,omitempty"`
	Downvotes                   int64                       `json:"downvotes,omitempty"`
	SquashCommitSHA             string                      `json:"squash_commit_sha,omitempty"`
	ForceRemoveSourceBranch     bool                        `json:"force_remove_source_branch,omitempty"`
	AllowCollaboration          bool                        `json:"allow_collaboration,omitempty"`
	MergeAfter                  string                      `json:"merge_after,omitempty"`
	TaskCompletionStatus        *TaskCompletionStatusOutput `json:"task_completion_status,omitempty"`
	TimeStats                   *TimeStatsOutput            `json:"time_stats,omitempty"`
	Subscribed                  bool                        `json:"subscribed,omitempty"`
	FirstContribution           bool                        `json:"first_contribution,omitempty"`
	User                        *MergeRequestUserOutput     `json:"user,omitempty"`
	DiffRefs                    *DiffRefsOutput             `json:"diff_refs,omitempty"`
	Pipeline                    *PipelineInfoOutput         `json:"pipeline,omitempty"`
	HeadPipeline                *PipelineOutput             `json:"head_pipeline,omitempty"`
	LatestBuildStartedAt        string                      `json:"latest_build_started_at,omitempty"`
	LatestBuildFinishedAt       string                      `json:"latest_build_finished_at,omitempty"`
	FirstDeployedToProductionAt string                      `json:"first_deployed_to_production_at,omitempty"`
	CreatedAt                   string                      `json:"created_at"`
	UpdatedAt                   string                      `json:"updated_at"`
	MergedAt                    string                      `json:"merged_at,omitempty"`
	ClosedAt                    string                      `json:"closed_at,omitempty"`
	PreparedAt                  string                      `json:"prepared_at,omitempty"`
	UserNotesCount              int64                       `json:"user_notes_count,omitempty"`
}

// MRMilestoneOutput mirrors gl.Milestone as surfaced inside merge
// request and issue resources. Distinct from toolutil.MilestoneOutput
// (the release-milestone shape) — see file-level comment for the
// rationale and field-level deltas.
type MRMilestoneOutput struct {
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

// NewMRMilestoneOutputs converts a slice of gl.Milestone, skipping nil
// entries and returning nil for an empty / all-nil input.
func NewMRMilestoneOutputs(ms []*gl.Milestone) []*MRMilestoneOutput {
	if len(ms) == 0 {
		return nil
	}
	out := make([]*MRMilestoneOutput, 0, len(ms))
	for _, m := range ms {
		if m == nil {
			continue
		}
		out = append(out, &MRMilestoneOutput{
			ID:          m.ID,
			IID:         m.IID,
			GroupID:     m.GroupID,
			ProjectID:   m.ProjectID,
			Title:       m.Title,
			Description: m.Description,
			State:       m.State,
			WebURL:      m.WebURL,
			StartDate:   FormatISOTimePtr(m.StartDate),
			DueDate:     FormatISOTimePtr(m.DueDate),
			CreatedAt:   FormatTimePtr(m.CreatedAt),
			UpdatedAt:   FormatTimePtr(m.UpdatedAt),
			Expired:     m.Expired,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ReferencesOutput mirrors gl.References (the short / relative / full
// reference triplet surfaced inside issue and MR resources).
type ReferencesOutput struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

// NewReferencesOutput converts a gl.IssueReferences value into the
// canonical-key references object, returning nil when the source is
// the zero value.
func NewReferencesOutput(r *gl.IssueReferences) *ReferencesOutput {
	if r == nil {
		return nil
	}
	return &ReferencesOutput{Short: r.Short, Relative: r.Relative, Full: r.Full}
}

// LabelDetailsOutput mirrors gl.LabelDetails (the inline label details
// surfaced on labels array inside issue and MR resources).
type LabelDetailsOutput struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html"`
	TextColor       string `json:"text_color"`
}

// NewLabelDetailsOutputs converts a slice of gl.LabelDetails, skipping
// nil entries and returning nil for an empty / all-nil input.
func NewLabelDetailsOutputs(details []*gl.LabelDetails) []*LabelDetailsOutput {
	if len(details) == 0 {
		return nil
	}
	out := make([]*LabelDetailsOutput, 0, len(details))
	for _, d := range details {
		if d == nil {
			continue
		}
		out = append(out, &LabelDetailsOutput{
			ID:              d.ID,
			Name:            d.Name,
			Color:           d.Color,
			Description:     d.Description,
			DescriptionHTML: d.DescriptionHTML,
			TextColor:       d.TextColor,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TaskCompletionStatusOutput mirrors gl.TaskCompletionStatus (the
// task-count object surfaced on merge requests).
type TaskCompletionStatusOutput struct {
	Count          int64 `json:"count"`
	CompletedCount int64 `json:"completed_count"`
}

// NewTaskCompletionStatusOutput converts a gl.TasksCompletionStatus
// pointer into the canonical-key task status object, returning nil
// for a nil source.
func NewTaskCompletionStatusOutput(t *gl.TasksCompletionStatus) *TaskCompletionStatusOutput {
	if t == nil {
		return nil
	}
	return &TaskCompletionStatusOutput{Count: t.Count, CompletedCount: t.CompletedCount}
}

// PipelineInfoOutput mirrors gl.PipelineInfo (the compact pipeline
// summary surfaced on a merge request's head_pipeline / pipeline
// fields).
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

// NewPipelineInfoOutput converts a gl.PipelineInfo pointer into the
// canonical-key pipeline info object, returning nil for a nil source.
func NewPipelineInfoOutput(p *gl.PipelineInfo) *PipelineInfoOutput {
	if p == nil {
		return nil
	}
	return &PipelineInfoOutput{
		ID:        p.ID,
		IID:       p.IID,
		ProjectID: p.ProjectID,
		Status:    p.Status,
		Source:    p.Source,
		Ref:       p.Ref,
		SHA:       p.SHA,
		Name:      p.Name,
		WebURL:    p.WebURL,
		UpdatedAt: FormatTimePtr(p.UpdatedAt),
		CreatedAt: FormatTimePtr(p.CreatedAt),
	}
}

// DiffRefsOutput mirrors the diff_refs object on a merge request, carrying the
// base, head, and start SHAs of the diff. It is byte-identical across the
// mergerequests and deploymentmergerequests domains; the canonical copy lives
// here so both packages can alias it without introducing import cycles
// (ADR-0004).
type DiffRefsOutput struct {
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	StartSHA string `json:"start_sha"`
}

// MergeRequestUserOutput mirrors gl.MergeRequestUser (the user object
// inside MR.user_notes_count type contexts — only the CanMerge flag
// is part of the documented subset).
type MergeRequestUserOutput struct {
	CanMerge bool `json:"can_merge"`
}

// NewMergeRequestUserOutput converts a gl.MergeRequestUser pointer into
// the canonical-key MR user object, returning nil for a nil source.
func NewMergeRequestUserOutput(u *gl.MergeRequestUser) *MergeRequestUserOutput {
	if u == nil {
		return nil
	}
	return &MergeRequestUserOutput{CanMerge: u.CanMerge}
}
