package deployments

import (
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface the fields of the SDK
// structs on their canonical json keys and are replicated here rather than
// imported from sibling packages to preserve the zero-import-cycle constraint
// (C-IMPORTS).
//
// This file covers the deployment sub-objects surfaced on the canonical json
// keys: user (gl.ProjectUser), environment (gl.Environment), and deployable
// (gl.DeploymentDeployable, including its nested user, commit, pipeline, and
// runner objects).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// UserOutput mirrors gl.ProjectUser (the deployment-level user object).
type UserOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

func projectUserOutput(u *gitlab.ProjectUser) *UserOutput {
	if u == nil {
		return nil
	}
	return &UserOutput{
		ID:        u.ID,
		Name:      u.Name,
		Username:  u.Username,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// EnvironmentOutput mirrors the scalar fields of gl.Environment. The SDK's
// Project and LastDeployment back-references are intentionally omitted to avoid
// unbounded nesting cycles (deployment -> environment -> last_deployment -> ...).
type EnvironmentOutput struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Slug                string `json:"slug,omitempty"`
	Description         string `json:"description,omitempty"`
	State               string `json:"state,omitempty"`
	Tier                string `json:"tier,omitempty"`
	ExternalURL         string `json:"external_url,omitempty"`
	CreatedAt           string `json:"created_at,omitempty"`
	UpdatedAt           string `json:"updated_at,omitempty"`
	KubernetesNamespace string `json:"kubernetes_namespace,omitempty"`
	FluxResourcePath    string `json:"flux_resource_path,omitempty"`
	AutoStopAt          string `json:"auto_stop_at,omitempty"`
	AutoStopSetting     string `json:"auto_stop_setting,omitempty"`
}

func environmentOutput(e *gitlab.Environment) *EnvironmentOutput {
	if e == nil {
		return nil
	}
	return &EnvironmentOutput{
		ID:                  e.ID,
		Name:                e.Name,
		Slug:                e.Slug,
		Description:         e.Description,
		State:               e.State,
		Tier:                e.Tier,
		ExternalURL:         e.ExternalURL,
		CreatedAt:           formatTimePtr(e.CreatedAt),
		UpdatedAt:           formatTimePtr(e.UpdatedAt),
		KubernetesNamespace: e.KubernetesNamespace,
		FluxResourcePath:    e.FluxResourcePath,
		AutoStopAt:          formatTimePtr(e.AutoStopAt),
		AutoStopSetting:     e.AutoStopSetting,
	}
}

// DeployableUserOutput mirrors the fields of gl.User surfaced on a deployable.
type DeployableUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

func deployableUserOutput(u *gitlab.User) *DeployableUserOutput {
	if u == nil {
		return nil
	}
	return &DeployableUserOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// DeployableCommitOutput mirrors the fields of gl.Commit surfaced on a deployable.
type DeployableCommitOutput struct {
	ID           string `json:"id"`
	ShortID      string `json:"short_id,omitempty"`
	Title        string `json:"title,omitempty"`
	Message      string `json:"message,omitempty"`
	AuthorName   string `json:"author_name,omitempty"`
	AuthorEmail  string `json:"author_email,omitempty"`
	AuthoredDate string `json:"authored_date,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	WebURL       string `json:"web_url,omitempty"`
}

func deployableCommitOutput(c *gitlab.Commit) *DeployableCommitOutput {
	if c == nil {
		return nil
	}
	return &DeployableCommitOutput{
		ID:           c.ID,
		ShortID:      c.ShortID,
		Title:        c.Title,
		Message:      c.Message,
		AuthorName:   c.AuthorName,
		AuthorEmail:  c.AuthorEmail,
		AuthoredDate: formatTimePtr(c.AuthoredDate),
		CreatedAt:    formatTimePtr(c.CreatedAt),
		WebURL:       c.WebURL,
	}
}

// DeployablePipelineOutput mirrors gl.DeploymentDeployablePipeline.
type DeployablePipelineOutput struct {
	ID        int64  `json:"id"`
	SHA       string `json:"sha,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Status    string `json:"status,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func deployablePipelineOutput(p gitlab.DeploymentDeployablePipeline) *DeployablePipelineOutput {
	if p.ID == 0 && p.SHA == "" && p.Ref == "" && p.Status == "" && p.WebURL == "" {
		return nil
	}
	return &DeployablePipelineOutput{
		ID:        p.ID,
		SHA:       p.SHA,
		Ref:       p.Ref,
		Status:    p.Status,
		WebURL:    p.WebURL,
		CreatedAt: formatTimePtr(p.CreatedAt),
		UpdatedAt: formatTimePtr(p.UpdatedAt),
	}
}

// DeployableRunnerOutput mirrors the fields of gl.Runner surfaced on a deployable.
type DeployableRunnerOutput struct {
	ID          int64  `json:"id"`
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`
	RunnerType  string `json:"runner_type,omitempty"`
	Status      string `json:"status,omitempty"`
	Online      bool   `json:"online,omitempty"`
	Paused      bool   `json:"paused,omitempty"`
	IsShared    bool   `json:"is_shared,omitempty"`
}

func deployableRunnerOutput(r *gitlab.Runner) *DeployableRunnerOutput {
	if r == nil {
		return nil
	}
	return &DeployableRunnerOutput{
		ID:          r.ID,
		Description: r.Description,
		Name:        r.Name,
		RunnerType:  r.RunnerType,
		Status:      r.Status,
		Online:      r.Online,
		Paused:      r.Paused,
		IsShared:    r.IsShared,
	}
}

// DeployableOutput mirrors gl.DeploymentDeployable (the CI job backing a
// deployment), including its nested user, commit, pipeline, and runner objects.
type DeployableOutput struct {
	ID         int64                     `json:"id"`
	Status     string                    `json:"status,omitempty"`
	Stage      string                    `json:"stage,omitempty"`
	Name       string                    `json:"name,omitempty"`
	Ref        string                    `json:"ref,omitempty"`
	Tag        bool                      `json:"tag,omitempty"`
	Coverage   float64                   `json:"coverage,omitempty"`
	CreatedAt  string                    `json:"created_at,omitempty"`
	StartedAt  string                    `json:"started_at,omitempty"`
	FinishedAt string                    `json:"finished_at,omitempty"`
	Duration   float64                   `json:"duration,omitempty"`
	User       *DeployableUserOutput     `json:"user,omitempty"`
	Commit     *DeployableCommitOutput   `json:"commit,omitempty"`
	Pipeline   *DeployablePipelineOutput `json:"pipeline,omitempty"`
	Runner     *DeployableRunnerOutput   `json:"runner,omitempty"`
}

// deployableOutput converts gl.DeploymentDeployable to its output shape,
// returning nil when the value is empty (no backing job).
func deployableOutput(d gitlab.DeploymentDeployable) *DeployableOutput {
	pipeline := deployablePipelineOutput(d.Pipeline)
	if d.ID == 0 && d.Name == "" && d.Status == "" && d.Stage == "" && d.Ref == "" &&
		d.User == nil && d.Commit == nil && pipeline == nil && d.Runner == nil {
		return nil
	}
	return &DeployableOutput{
		ID:         d.ID,
		Status:     d.Status,
		Stage:      d.Stage,
		Name:       d.Name,
		Ref:        d.Ref,
		Tag:        d.Tag,
		Coverage:   d.Coverage,
		CreatedAt:  formatTimePtr(d.CreatedAt),
		StartedAt:  formatTimePtr(d.StartedAt),
		FinishedAt: formatTimePtr(d.FinishedAt),
		Duration:   d.Duration,
		User:       deployableUserOutput(d.User),
		Commit:     deployableCommitOutput(d.Commit),
		Pipeline:   pipeline,
		Runner:     deployableRunnerOutput(d.Runner),
	}
}
