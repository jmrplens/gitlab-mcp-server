// shapes.go defines the additive nested output sub-objects surfaced on an
// environment (cluster_agent, last_deployment, project) and their converters
// from the client-go SDK shapes. Each mirror tracks the canonical, identifying
// fields of the corresponding gl.* type rather than its full shape, keeping the
// MCP output focused while remaining faithful to the API response.
package environments

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// formatTimePtr renders a *time.Time as an RFC3339 string, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// ClusterAgentOutput mirrors gl.Agent (the cluster agent associated with an
// environment via the `cluster_agent` key). It surfaces the agent identity and
// a compact reference to its configuration project.
type ClusterAgentOutput struct {
	ID              int64                `json:"id"`
	Name            string               `json:"name"`
	CreatedAt       string               `json:"created_at,omitempty"`
	CreatedByUserID int64                `json:"created_by_user_id,omitempty"`
	ConfigProject   *ConfigProjectOutput `json:"config_project,omitempty"`
}

// ConfigProjectOutput mirrors gl.ConfigProject (the cluster agent's
// configuration project reference).
type ConfigProjectOutput struct {
	ID                int64  `json:"id"`
	Description       string `json:"description,omitempty"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace,omitempty"`
	Path              string `json:"path,omitempty"`
	PathWithNamespace string `json:"path_with_namespace,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
}

func clusterAgentOutput(a *gl.Agent) *ClusterAgentOutput {
	if a == nil {
		return nil
	}
	out := &ClusterAgentOutput{
		ID:              a.ID,
		Name:            a.Name,
		CreatedAt:       formatTimePtr(a.CreatedAt),
		CreatedByUserID: a.CreatedByUserID,
	}
	cp := a.ConfigProject
	out.ConfigProject = &ConfigProjectOutput{
		ID:                cp.ID,
		Description:       cp.Description,
		Name:              cp.Name,
		NameWithNamespace: cp.NameWithNamespace,
		Path:              cp.Path,
		PathWithNamespace: cp.PathWithNamespace,
		CreatedAt:         formatTimePtr(cp.CreatedAt),
	}
	return out
}

// ProjectObject mirrors gl.Project (the project object embedded on an
// environment's `project` key). It surfaces the identifying fields of the
// owning project without importing the full gl.Project shape, which carries
// ~160 fields (statistics, permissions, settings, links) not relevant to an
// environment's project reference.
type ProjectObject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path,omitempty"`
	PathWithNamespace string `json:"path_with_namespace,omitempty"`
	WebURL            string `json:"web_url,omitempty"`
}

func projectObject(p *gl.Project) *ProjectObject {
	if p == nil {
		return nil
	}
	return &ProjectObject{
		ID:                p.ID,
		Name:              p.Name,
		Path:              p.Path,
		PathWithNamespace: p.PathWithNamespace,
		WebURL:            p.WebURL,
	}
}

// UserOutput mirrors gl.ProjectUser (the user that triggered a deployment).
type UserOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username,omitempty"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

func projectUserOutput(u *gl.ProjectUser) *UserOutput {
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

// DeploymentOutput mirrors gl.Deployment (the environment's `last_deployment`).
// It surfaces the deployment identity, ref/sha/status, timestamps, the
// triggering user, and a compact view of the deployable job and its pipeline.
// The deployment's back-reference to its environment is intentionally omitted
// to avoid the recursive Environment -> last_deployment -> environment cycle.
type DeploymentOutput struct {
	ID         int64             `json:"id"`
	IID        int64             `json:"iid,omitempty"`
	Ref        string            `json:"ref,omitempty"`
	SHA        string            `json:"sha,omitempty"`
	Status     string            `json:"status,omitempty"`
	CreatedAt  string            `json:"created_at,omitempty"`
	UpdatedAt  string            `json:"updated_at,omitempty"`
	User       *UserOutput       `json:"user,omitempty"`
	Deployable *DeployableOutput `json:"deployable,omitempty"`
}

// DeployableOutput mirrors gl.DeploymentDeployable (the job that performed the
// deployment) with its canonical scalars and a compact pipeline reference.
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
	Pipeline   *DeployablePipelineOutput `json:"pipeline,omitempty"`
}

// DeployablePipelineOutput mirrors gl.DeploymentDeployablePipeline (the
// pipeline reference embedded on a deployable).
type DeployablePipelineOutput struct {
	ID        int64  `json:"id"`
	SHA       string `json:"sha,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Status    string `json:"status,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func deploymentOutput(d *gl.Deployment) *DeploymentOutput {
	if d == nil {
		return nil
	}
	out := &DeploymentOutput{
		ID:        d.ID,
		IID:       d.IID,
		Ref:       d.Ref,
		SHA:       d.SHA,
		Status:    d.Status,
		CreatedAt: formatTimePtr(d.CreatedAt),
		UpdatedAt: formatTimePtr(d.UpdatedAt),
		User:      projectUserOutput(d.User),
	}
	out.Deployable = deployableOutput(d.Deployable)
	return out
}

// deployableOutput converts the value-typed gl.DeploymentDeployable, returning
// nil when it carries no data (zero ID and empty name).
func deployableOutput(d gl.DeploymentDeployable) *DeployableOutput {
	if d.ID == 0 && d.Name == "" {
		return nil
	}
	out := &DeployableOutput{
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
	}
	p := d.Pipeline
	if p.ID != 0 {
		out.Pipeline = &DeployablePipelineOutput{
			ID:        p.ID,
			SHA:       p.SHA,
			Ref:       p.Ref,
			Status:    p.Status,
			WebURL:    p.WebURL,
			CreatedAt: formatTimePtr(p.CreatedAt),
			UpdatedAt: formatTimePtr(p.UpdatedAt),
		}
	}
	return out
}
