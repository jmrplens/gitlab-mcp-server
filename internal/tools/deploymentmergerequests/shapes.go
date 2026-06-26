package deploymentmergerequests

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct. The four pipeline-cluster shapes that are byte-identical across
// mergerequests / mergetrains / deploymentmergerequests (BasicUserOutput,
// PipelineOutput, PipelineDetailedStatusOutput,
// PipelineDetailedStatusIllustrationOutput) live in
// internal/toolutil since DEDUP-001 wave 3a; the rest remain local because
// they are package-specific.

// MilestoneOutput mirrors gl.Milestone (the merge-request milestone object).

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
		StartDate: toolutil.FormatISOTimePtr(m.StartDate), DueDate: toolutil.FormatISOTimePtr(m.DueDate),
		CreatedAt: toolutil.FormatTimePtr(m.CreatedAt), UpdatedAt: toolutil.FormatTimePtr(m.UpdatedAt),
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
		UpdatedAt: toolutil.FormatTimePtr(p.UpdatedAt), CreatedAt: toolutil.FormatTimePtr(p.CreatedAt),
	}
}

// PipelineDetailedStatusIllustrationOutput, PipelineDetailedStatusOutput,
// and PipelineOutput have moved to toolutil.* (DEDUP-001 wave 3a). The
// local converter helpers were also removed in favor of
// toolutil.NewPipelineDetailedStatusOutput, toolutil.NewPipelineOutput, and
// toolutil.NewBasicUserOutput.
