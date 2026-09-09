package environments

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ClusterAgentOutput mirrors the environment `cluster_agent` object documented in
// the "Retrieve an environment" response (doc/api/environments.md): id, name,
// config_project, created_at, created_by_user_id.
//
// Documented reference subset per doc/api/environments.md
type ClusterAgentOutput struct {
	ID              int64                `json:"id"`
	Name            string               `json:"name"`
	ConfigProject   *ConfigProjectOutput `json:"config_project,omitempty"`
	CreatedAt       string               `json:"created_at,omitempty"`
	CreatedByUserID int64                `json:"created_by_user_id,omitempty"`
}

// ConfigProjectOutput mirrors the `cluster_agent.config_project` object
// documented in the "Retrieve an environment" response
// (doc/api/environments.md): id, description, name, name_with_namespace, path,
// path_with_namespace, created_at.
//
// Documented reference subset per doc/api/environments.md
type ConfigProjectOutput struct {
	ID                int64  `json:"id"`
	Description       string `json:"description"`
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
		CreatedAt:       toolutil.FormatTimePtr(a.CreatedAt),
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
		CreatedAt:         toolutil.FormatTimePtr(cp.CreatedAt),
	}
	return out
}

// DeploymentOutput mirrors the environment `last_deployment` object documented
// in the "Retrieve an environment" response (doc/api/environments.md): id, iid,
// ref, sha, created_at, status, user, deployable. The deployment's
// back-reference to its environment is intentionally omitted to avoid the
// recursive Environment -> last_deployment -> environment cycle, and `updated_at`
// is omitted because the documented last_deployment object does not include it.
//
// Documented reference subset per doc/api/environments.md
type DeploymentOutput struct {
	ID         int64                 `json:"id"`
	IID        int64                 `json:"iid,omitempty"`
	Ref        string                `json:"ref,omitempty"`
	SHA        string                `json:"sha,omitempty"`
	CreatedAt  string                `json:"created_at,omitempty"`
	Status     string                `json:"status,omitempty"`
	User       *DeploymentUserOutput `json:"user,omitempty"`
	Deployable *DeployableOutput     `json:"deployable,omitempty"`
}

// DeploymentUserOutput mirrors the `last_deployment.user` object documented in
// the "Retrieve an environment" response (doc/api/environments.md): id, name,
// state, username, avatar_url, web_url.
//
// Documented reference subset per doc/api/environments.md
type DeploymentUserOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

func deploymentUserOutput(u *gl.ProjectUser) *DeploymentUserOutput {
	if u == nil {
		return nil
	}
	return &DeploymentUserOutput{
		ID:        u.ID,
		Name:      u.Name,
		State:     u.State,
		Username:  u.Username,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// DeployableOutput mirrors the `last_deployment.deployable` job object documented
// in the "Retrieve an environment" response (doc/api/environments.md): id,
// status, stage, name, ref, tag, coverage, created_at, started_at, finished_at,
// duration, user, commit, pipeline, runner. The documented `project`
// (ci_job_token_scope_enabled), `web_url`, `artifacts`, and `artifacts_expire_at`
// fields are not present on the client-go DeploymentDeployable struct (v2.42.0)
// and therefore cannot be surfaced.
//
// Documented reference subset per doc/api/environments.md
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

// DeployableUserOutput mirrors the `deployable.user` object documented in the
// "Retrieve an environment" response (doc/api/environments.md): id, name,
// username, state, avatar_url, web_url, created_at, bio, location, public_email,
// linkedin, twitter, website_url, organization.
//
// Documented reference subset per doc/api/environments.md
type DeployableUserOutput struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Username     string `json:"username,omitempty"`
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

func deployableUserOutput(u *gl.User) *DeployableUserOutput {
	if u == nil {
		return nil
	}
	return &DeployableUserOutput{
		ID:           u.ID,
		Name:         u.Name,
		Username:     u.Username,
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

// DeployableCommitOutput mirrors the `deployable.commit` object documented in
// the "Retrieve an environment" response (doc/api/environments.md): id,
// short_id, created_at, parent_ids, title, message, author_name, author_email,
// authored_date, committer_name, committer_email, committed_date.
//
// Documented reference subset per doc/api/environments.md
type DeployableCommitOutput struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"short_id,omitempty"`
	CreatedAt      string   `json:"created_at,omitempty"`
	ParentIDs      []string `json:"parent_ids,omitempty"`
	Title          string   `json:"title,omitempty"`
	Message        string   `json:"message,omitempty"`
	AuthorName     string   `json:"author_name,omitempty"`
	AuthorEmail    string   `json:"author_email,omitempty"`
	AuthoredDate   string   `json:"authored_date,omitempty"`
	CommitterName  string   `json:"committer_name,omitempty"`
	CommitterEmail string   `json:"committer_email,omitempty"`
	CommittedDate  string   `json:"committed_date,omitempty"`
}

func deployableCommitOutput(c *gl.Commit) *DeployableCommitOutput {
	if c == nil {
		return nil
	}
	return &DeployableCommitOutput{
		ID:             c.ID,
		ShortID:        c.ShortID,
		CreatedAt:      toolutil.FormatTimePtr(c.CreatedAt),
		ParentIDs:      c.ParentIDs,
		Title:          c.Title,
		Message:        c.Message,
		AuthorName:     c.AuthorName,
		AuthorEmail:    c.AuthorEmail,
		AuthoredDate:   toolutil.FormatTimePtr(c.AuthoredDate),
		CommitterName:  c.CommitterName,
		CommitterEmail: c.CommitterEmail,
		CommittedDate:  toolutil.FormatTimePtr(c.CommittedDate),
	}
}

// DeployablePipelineOutput mirrors the `deployable.pipeline` object documented in
// the "Retrieve an environment" response (doc/api/environments.md): id, sha,
// ref, status, web_url.
//
// Documented reference subset per doc/api/environments.md
type DeployablePipelineOutput struct {
	ID     int64  `json:"id"`
	SHA    string `json:"sha,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Status string `json:"status,omitempty"`
	WebURL string `json:"web_url,omitempty"`
}

// DeployableRunnerOutput mirrors the `deployable.runner` object documented in the
// "Retrieve an environment" response (doc/api/environments.md). The documented
// example shows `runner` as null, so its shape is reduced to the canonical
// runner identity fields exposed by the client-go Runner struct.
//
// Documented reference subset per doc/api/environments.md
type DeployableRunnerOutput struct {
	ID          int64  `json:"id"`
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`
	IsShared    bool   `json:"is_shared,omitempty"`
	RunnerType  string `json:"runner_type,omitempty"`
	Online      bool   `json:"online,omitempty"`
	Status      string `json:"status,omitempty"`
}

func deployableRunnerOutput(r *gl.Runner) *DeployableRunnerOutput {
	if r == nil {
		return nil
	}
	return &DeployableRunnerOutput{
		ID:          r.ID,
		Description: r.Description,
		Name:        r.Name,
		IsShared:    r.IsShared,
		RunnerType:  r.RunnerType,
		Online:      r.Online,
		Status:      r.Status,
	}
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
		CreatedAt: toolutil.FormatTimePtr(d.CreatedAt),
		Status:    d.Status,
		User:      deploymentUserOutput(d.User),
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
		CreatedAt:  toolutil.FormatTimePtr(d.CreatedAt),
		StartedAt:  toolutil.FormatTimePtr(d.StartedAt),
		FinishedAt: toolutil.FormatTimePtr(d.FinishedAt),
		Duration:   d.Duration,
		User:       deployableUserOutput(d.User),
		Commit:     deployableCommitOutput(d.Commit),
		Runner:     deployableRunnerOutput(d.Runner),
	}
	p := d.Pipeline
	if p.ID != 0 {
		out.Pipeline = &DeployablePipelineOutput{
			ID:     p.ID,
			SHA:    p.SHA,
			Ref:    p.Ref,
			Status: p.Status,
			WebURL: p.WebURL,
		}
	}
	return out
}

// ProjectOutput mirrors the project object GitLab renders on an environment,
// which is lib/api/entities/basic_project_details.rb and not a whole project:
// twenty-four keys, every one of them carried by the SDK's own Project.
type ProjectOutput struct {
	ID                int64                            `json:"id"`
	Description       string                           `json:"description,omitempty"`
	Name              string                           `json:"name"`
	NameWithNamespace string                           `json:"name_with_namespace,omitempty"`
	Path              string                           `json:"path,omitempty"`
	PathWithNamespace string                           `json:"path_with_namespace,omitempty"`
	CreatedAt         string                           `json:"created_at,omitempty"`
	DefaultBranch     string                           `json:"default_branch,omitempty"`
	TagList           []string                         `json:"tag_list,omitempty"`
	Topics            []string                         `json:"topics,omitempty"`
	SSHURLToRepo      string                           `json:"ssh_url_to_repo,omitempty"`
	HTTPURLToRepo     string                           `json:"http_url_to_repo,omitempty"`
	WebURL            string                           `json:"web_url,omitempty"`
	ReadmeURL         string                           `json:"readme_url,omitempty"`
	ForksCount        int64                            `json:"forks_count"`
	LicenseURL        string                           `json:"license_url,omitempty"`
	License           *ProjectLicenseOutput            `json:"license,omitempty"`
	AvatarURL         string                           `json:"avatar_url,omitempty"`
	StarCount         int64                            `json:"star_count"`
	LastActivityAt    string                           `json:"last_activity_at,omitempty"`
	Visibility        string                           `json:"visibility,omitempty"`
	Namespace         *ProjectNamespaceOutput          `json:"namespace,omitempty"`
	CustomAttributes  []toolutil.CustomAttributeOutput `json:"custom_attributes,omitempty"`
	RepositoryStorage string                           `json:"repository_storage,omitempty"`
}

// ProjectLicenseOutput mirrors the license object under that project.
type ProjectLicenseOutput struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Nickname  string `json:"nickname,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
}

// ProjectNamespaceOutput mirrors the namespace object under that project.
type ProjectNamespaceOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Kind      string `json:"kind,omitempty"`
	FullPath  string `json:"full_path,omitempty"`
	ParentID  int64  `json:"parent_id,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// projectOutput converts the SDK's project into the shape GitLab renders under
// an environment, returning nil when the environment carries none.
func projectOutput(p *gl.Project) *ProjectOutput {
	if p == nil {
		return nil
	}
	out := &ProjectOutput{
		ID:                p.ID,
		Description:       p.Description,
		Name:              p.Name,
		NameWithNamespace: p.NameWithNamespace,
		Path:              p.Path,
		PathWithNamespace: p.PathWithNamespace,
		CreatedAt:         toolutil.FormatTimePtr(p.CreatedAt),
		DefaultBranch:     p.DefaultBranch,
		//nolint:staticcheck // tag_list is documented (deprecated alias of topics) in doc/api/environments.md
		TagList:           p.TagList,
		Topics:            p.Topics,
		SSHURLToRepo:      p.SSHURLToRepo,
		HTTPURLToRepo:     p.HTTPURLToRepo,
		WebURL:            p.WebURL,
		ReadmeURL:         p.ReadmeURL,
		ForksCount:        p.ForksCount,
		LicenseURL:        p.LicenseURL,
		AvatarURL:         p.AvatarURL,
		StarCount:         p.StarCount,
		LastActivityAt:    toolutil.FormatTimePtr(p.LastActivityAt),
		Visibility:        string(p.Visibility),
		RepositoryStorage: p.RepositoryStorage,
	}
	if p.License != nil {
		out.License = &ProjectLicenseOutput{
			Key:       p.License.Key,
			Name:      p.License.Name,
			Nickname:  p.License.Nickname,
			HTMLURL:   p.License.HTMLURL,
			SourceURL: p.License.SourceURL,
		}
	}
	if p.Namespace != nil {
		out.Namespace = &ProjectNamespaceOutput{
			ID:        p.Namespace.ID,
			Name:      p.Namespace.Name,
			Path:      p.Namespace.Path,
			Kind:      p.Namespace.Kind,
			FullPath:  p.Namespace.FullPath,
			ParentID:  p.Namespace.ParentID,
			AvatarURL: p.Namespace.AvatarURL,
			WebURL:    p.Namespace.WebURL,
		}
	}
	out.CustomAttributes = toolutil.NewCustomAttributeOutputs(p.CustomAttributes)
	return out
}
