package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

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
