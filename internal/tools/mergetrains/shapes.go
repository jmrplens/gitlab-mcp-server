package mergetrains

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here as local types rather than imported from
// sibling packages to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the merge-train sub-objects surfaced on the canonical json
// keys: user (BasicUserOutput, mirrors gl.BasicUser) and pipeline
// (PipelineOutput, mirrors gl.Pipeline with its nested detailed_status). The
// merge_request sub-object (MergeRequestOutput) mirrors gl.MergeTrainMergeRequest
// and is defined alongside the handlers in merge_trains.go.

// BasicUserOutput mirrors gl.BasicUser, the compact user object embedded on the
// merge-train "user" key.
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
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: toolutil.FormatTimePtr(u.CreatedAt),
	}
}

// PipelineDetailedStatusIllustrationOutput mirrors gl.DetailedStatusIllustration
// (the detailed_status.illustration object).
type PipelineDetailedStatusIllustrationOutput struct {
	Image string `json:"image"`
}

// PipelineDetailedStatusOutput mirrors gl.DetailedStatus (the pipeline
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

// PipelineOutput mirrors gl.Pipeline (the full merge-train "pipeline" object).
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
		UpdatedAt: toolutil.FormatTimePtr(p.UpdatedAt), CreatedAt: toolutil.FormatTimePtr(p.CreatedAt),
		StartedAt: toolutil.FormatTimePtr(p.StartedAt), FinishedAt: toolutil.FormatTimePtr(p.FinishedAt),
		CommittedAt: toolutil.FormatTimePtr(p.CommittedAt), Duration: p.Duration, QueuedDuration: p.QueuedDuration,
		Coverage: p.Coverage, WebURL: p.WebURL, DetailedStatus: pipelineDetailedStatusOutput(p.DetailedStatus),
	}
}
