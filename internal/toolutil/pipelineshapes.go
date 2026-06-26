// Package toolutil — pipeline shapes shared across the pipeline-cluster
// packages (mergerequests, mergetrains, deploymentmergerequests).
//
// The structs and converters here were originally duplicated in each
// pipeline-cluster package per the 1:1 audit policy (DEDUP-001 Option B
// consolidation). The shapes are byte-identical across the family, so
// they live here once.
package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// BasicUserOutput mirrors gl.BasicUser (the user shape returned in many
// pipeline / merge-train / deployment-MR sub-objects: head_pipeline.user,
// merge_train.pipeline.user, etc.).
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

// NewBasicUserOutput converts a *gl.BasicUser into the canonical-key basic
// user object, returning nil when the SDK value is nil.
func NewBasicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
		CreatedAt: FormatTimePtr(u.CreatedAt),
	}
}

// NewBasicUserOutputs converts a []*gl.BasicUser slice, skipping nil
// elements and returning nil for an empty input.
func NewBasicUserOutputs(users []*gl.BasicUser) []*BasicUserOutput {
	if len(users) == 0 {
		return nil
	}
	out := make([]*BasicUserOutput, 0, len(users))
	for _, u := range users {
		if converted := NewBasicUserOutput(u); converted != nil {
			out = append(out, converted)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// PipelineDetailedStatusIllustrationOutput mirrors
// gl.DetailedStatus.Illustration (the icon image sub-object).
type PipelineDetailedStatusIllustrationOutput struct {
	Image string `json:"image"`
}

// NewPipelineDetailedStatusIllustrationOutput is reserved for SDK
// versions that surface Illustration as a pointer; currently the SDK
// exposes it as a value type, so NewPipelineDetailedStatusOutput
// handles the conversion inline. Kept for forward compatibility.
func NewPipelineDetailedStatusIllustrationOutput(image string) *PipelineDetailedStatusIllustrationOutput {
	if image == "" {
		return nil
	}
	return &PipelineDetailedStatusIllustrationOutput{Image: image}
}

// PipelineDetailedStatusOutput mirrors gl.DetailedStatus (the
// icon / text / label / group / tooltip / details_path / illustration /
// favicon object that GitLab returns inside gl.Pipeline.DetailedStatus).
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

// NewPipelineDetailedStatusOutput converts a *gl.DetailedStatus into the
// canonical-key detailed-status object, returning nil when the SDK value
// is nil. The SDK's Illustration field is a non-pointer
// gl.DetailedStatusIllustration value; we surface the nested image only
// when populated, keeping the canonical nil-on-empty contract.
func NewPipelineDetailedStatusOutput(s *gl.DetailedStatus) *PipelineDetailedStatusOutput {
	if s == nil {
		return nil
	}
	out := &PipelineDetailedStatusOutput{
		Icon:        s.Icon,
		Text:        s.Text,
		Label:       s.Label,
		Group:       s.Group,
		Tooltip:     s.Tooltip,
		HasDetails:  s.HasDetails,
		DetailsPath: s.DetailsPath,
		Favicon:     s.Favicon,
	}
	if s.Illustration.Image != "" {
		out.Illustration = &PipelineDetailedStatusIllustrationOutput{Image: s.Illustration.Image}
	}
	return out
}

// PipelineOutput mirrors gl.Pipeline (the per-pipeline summary object
// returned by head_pipeline, merge_train.pipeline, deployment.merge_request
// .head_pipeline, etc.). It surfaces every field of the SDK struct that
// the documented GitLab API returns (1:1 audit policy) plus the nested
// user and detailed_status sub-objects on their canonical keys.
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

// NewPipelineOutput converts a *gl.Pipeline into the canonical-key
// pipeline object, returning nil when the SDK value is nil. Timestamps
// are surfaced as RFC 3339 strings via toolutil.FormatTimePtr.
func NewPipelineOutput(p *gl.Pipeline) *PipelineOutput {
	if p == nil {
		return nil
	}
	return &PipelineOutput{
		ID:             p.ID,
		IID:            p.IID,
		ProjectID:      p.ProjectID,
		Status:         p.Status,
		Source:         string(p.Source),
		Ref:            p.Ref,
		Name:           p.Name,
		SHA:            p.SHA,
		BeforeSHA:      p.BeforeSHA,
		Tag:            p.Tag,
		YamlErrors:     p.YamlErrors,
		User:           NewBasicUserOutput(p.User),
		UpdatedAt:      FormatTimePtr(p.UpdatedAt),
		CreatedAt:      FormatTimePtr(p.CreatedAt),
		StartedAt:      FormatTimePtr(p.StartedAt),
		FinishedAt:     FormatTimePtr(p.FinishedAt),
		CommittedAt:    FormatTimePtr(p.CommittedAt),
		Duration:       p.Duration,
		QueuedDuration: p.QueuedDuration,
		Coverage:       p.Coverage,
		WebURL:         p.WebURL,
		DetailedStatus: NewPipelineDetailedStatusOutput(p.DetailedStatus),
	}
}
