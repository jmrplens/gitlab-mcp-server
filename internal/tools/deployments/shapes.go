package deployments

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes reconciled 1:1 with the documented Deployments API
// responses (doc/api/deployments.md). Each nested *Output type is a documented
// reference subset: it surfaces exactly the fields the official API documents on
// the canonical json keys and is replicated here (rather than imported from
// sibling packages) to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the deployment sub-objects surfaced on the canonical json
// keys: user (gl.ProjectUser), environment (gl.Environment), and deployable
// (gl.DeploymentDeployable, including its nested user, commit, pipeline, and
// runner objects). The deployable.project sub-object ({ci_job_token_scope_enabled})
// is documented but absent from the SDK, so it is surfaced via a raw REST superset
// fetch (see rawGetDeployment/rawListDeployments in deployments.go).

// UserOutput is the documented reference subset of the deployment-level user
// object (sourced from gl.ProjectUser). Documented reference subset per
// doc/api/deployments.md.
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

// EnvironmentOutput is the documented reference subset of the deployment's
// environment object (sourced from gl.Environment). The API documents only
// id, name, and external_url on the deployment environment; the SDK's
// remaining scalars and back-references are intentionally not surfaced.
// Documented reference subset per doc/api/deployments.md.
type EnvironmentOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ExternalURL string `json:"external_url,omitempty"`
}

func environmentOutput(e *gitlab.Environment) *EnvironmentOutput {
	if e == nil {
		return nil
	}
	return &EnvironmentOutput{
		ID:          e.ID,
		Name:        e.Name,
		ExternalURL: e.ExternalURL,
	}
}

// DeployableUserOutput is the documented reference subset of the deployable
// (CI job) user object (sourced from gl.User). The API documents the richer
// user payload here (profile fields such as bio, location, and the social
// links). Documented reference subset per doc/api/deployments.md.
type DeployableUserOutput struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Name         string `json:"name"`
	State        string `json:"state,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	WebURL       string `json:"web_url,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	Bio          string `json:"bio,omitempty"`
	Location     string `json:"location,omitempty"`
	PublicEmail  string `json:"public_email,omitempty"`
	Linkedin     string `json:"linkedin,omitempty"`
	Twitter      string `json:"twitter,omitempty"`
	WebsiteURL   string `json:"website_url,omitempty"`
	Organization string `json:"organization,omitempty"`
}

func deployableUserOutput(u *gitlab.User) *DeployableUserOutput {
	if u == nil {
		return nil
	}
	return &DeployableUserOutput{
		ID:           u.ID,
		Username:     u.Username,
		Name:         u.Name,
		State:        u.State,
		AvatarURL:    u.AvatarURL,
		WebURL:       u.WebURL,
		CreatedAt:    toolutil.FormatTimePtr(u.CreatedAt),
		Bio:          u.Bio,
		Location:     u.Location,
		PublicEmail:  u.PublicEmail,
		Linkedin:     u.Linkedin,
		Twitter:      u.Twitter,
		WebsiteURL:   u.WebsiteURL,
		Organization: u.Organization,
	}
}

// DeployableCommitOutput is the documented reference subset of the deployable
// (CI job) commit object (sourced from gl.Commit). The API documents id,
// short_id, title, author_name, author_email, created_at, and message.
// Documented reference subset per doc/api/deployments.md.
type DeployableCommitOutput struct {
	ID          string `json:"id"`
	ShortID     string `json:"short_id,omitempty"`
	Title       string `json:"title,omitempty"`
	AuthorName  string `json:"author_name,omitempty"`
	AuthorEmail string `json:"author_email,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Message     string `json:"message,omitempty"`
}

func deployableCommitOutput(c *gitlab.Commit) *DeployableCommitOutput {
	if c == nil {
		return nil
	}
	return &DeployableCommitOutput{
		ID:          c.ID,
		ShortID:     c.ShortID,
		Title:       c.Title,
		AuthorName:  c.AuthorName,
		AuthorEmail: c.AuthorEmail,
		CreatedAt:   toolutil.FormatTimePtr(c.CreatedAt),
		Message:     c.Message,
	}
}

// DeployablePipelineOutput is the documented reference subset of the deployable
// (CI job) pipeline object (sourced from gl.DeploymentDeployablePipeline). The
// API documents created_at, id, ref, sha, status, updated_at, and web_url.
// Documented reference subset per doc/api/deployments.md.
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
		CreatedAt: toolutil.FormatTimePtr(p.CreatedAt),
		UpdatedAt: toolutil.FormatTimePtr(p.UpdatedAt),
	}
}

// DeployableRunnerOutput is the documented reference subset of the deployable
// (CI job) runner object (sourced from gl.Runner). The API documents the
// runner key on the deployable but only ever shows a null value, so this keeps
// the minimal runner identity fields surfaced when a runner is present.
// Documented reference subset per doc/api/deployments.md.
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

// DeployableProjectOutput is the documented reference subset of the deployable
// (CI job) project object. The Deployments API documents a single field on this
// object, ci_job_token_scope_enabled, indicating whether the project's CI/CD job
// token scope is enabled. gl.DeploymentDeployable has no Project field, so this
// is surfaced via a raw REST fetch (see rawGetDeployment/rawListDeployments).
// Documented reference subset per doc/api/deployments.md.
type DeployableProjectOutput struct {
	CIJobTokenScopeEnabled bool `json:"ci_job_token_scope_enabled"`
}

// DeployableOutput is the documented reference subset of gl.DeploymentDeployable
// (the CI job backing a deployment), including its nested user, commit,
// pipeline, runner, and project objects. The API documents id, status, stage,
// name, ref, tag, coverage, created_at, started_at, finished_at, plus the nested
// objects; the SDK's duration field is not documented and is not surfaced. The
// documented deployable.project object ({ci_job_token_scope_enabled}) has no
// counterpart on the SDK struct (gl.DeploymentDeployable.Project does not exist)
// and is therefore surfaced through a raw REST superset fetch.
// Documented reference subset per doc/api/deployments.md.
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
	Project    *DeployableProjectOutput  `json:"project,omitempty"`
	User       *DeployableUserOutput     `json:"user,omitempty"`
	Commit     *DeployableCommitOutput   `json:"commit,omitempty"`
	Pipeline   *DeployablePipelineOutput `json:"pipeline,omitempty"`
	Runner     *DeployableRunnerOutput   `json:"runner,omitempty"`
}

// deployableOutput converts gl.DeploymentDeployable to its output shape,
// returning nil when the value is empty (no backing job). The optional project
// argument carries the raw-fetched deployable.project object, which has no SDK
// counterpart; it is nil on older instances that omit the field.
func deployableOutput(d gitlab.DeploymentDeployable, project *DeployableProjectOutput) *DeployableOutput {
	pipeline := deployablePipelineOutput(d.Pipeline)
	if d.ID == 0 && d.Name == "" && d.Status == "" && d.Stage == "" && d.Ref == "" &&
		d.User == nil && d.Commit == nil && pipeline == nil && d.Runner == nil && project == nil {
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
		CreatedAt:  toolutil.FormatTimePtr(d.CreatedAt),
		StartedAt:  toolutil.FormatTimePtr(d.StartedAt),
		FinishedAt: toolutil.FormatTimePtr(d.FinishedAt),
		Project:    project,
		User:       deployableUserOutput(d.User),
		Commit:     deployableCommitOutput(d.Commit),
		Pipeline:   pipeline,
		Runner:     deployableRunnerOutput(d.Runner),
	}
}
