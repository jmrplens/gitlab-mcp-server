package jobs

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects on the canonical
// json keys (commit, pipeline, project, runner, user, artifacts,
// artifacts_file, downstream_pipeline). Per the 1:1 OUTPUT-reconciliation audit
// the nested reference objects (commit, pipeline, runner, user, project,
// downstream_pipeline) are trimmed to the field set the GitLab Jobs API
// documents for each sub-object (see doc/api/jobs.md), and are replicated here
// as local types rather than imported from sibling packages to preserve the
// zero-import-cycle constraint (C-IMPORTS).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// CommitObject mirrors the `commit` object embedded on a job, holding the
// fields the Jobs API documents for that sub-object.
//
// Documented reference subset per doc/api/jobs.md
// (https://docs.gitlab.com/api/jobs/#get-a-single-job — `commit`):
// id, short_id, title, author_name, author_email, created_at, message.
type CommitObject struct {
	ID          string `json:"id"`
	ShortID     string `json:"short_id"`
	Title       string `json:"title"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Message     string `json:"message,omitempty"`
}

// commitObject converts a gl.Commit to its output shape, returning nil when the
// SDK value is nil.
func commitObject(c *gl.Commit) *CommitObject {
	if c == nil {
		return nil
	}
	return &CommitObject{
		ID:          c.ID,
		ShortID:     c.ShortID,
		Title:       c.Title,
		AuthorName:  c.AuthorName,
		AuthorEmail: c.AuthorEmail,
		CreatedAt:   formatTimePtr(c.CreatedAt),
		Message:     c.Message,
	}
}

// PipelineObject mirrors gl.JobPipeline (the compact pipeline object embedded on
// a job's `pipeline` key).
//
// Documented reference subset per doc/api/jobs.md
// (https://docs.gitlab.com/api/jobs/#get-a-single-job — `pipeline`):
// id, project_id, ref, sha, status (full 1:1 with the documented shape).
type PipelineObject struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
}

// pipelineObject converts a gl.JobPipeline value to its output shape, returning
// nil when the pipeline is empty (zero-valued).
func pipelineObject(p gl.JobPipeline) *PipelineObject {
	if p == (gl.JobPipeline{}) {
		return nil
	}
	return &PipelineObject{
		ID: p.ID, ProjectID: p.ProjectID, Ref: p.Ref, SHA: p.Sha, Status: p.Status,
	}
}

// PipelineInfoObject mirrors the compact pipeline object embedded on a bridge's
// `pipeline` and `downstream_pipeline` keys, holding the fields the Jobs API
// documents for those sub-objects.
//
// Documented reference subset per doc/api/jobs.md
// (https://docs.gitlab.com/api/jobs/#list-pipeline-trigger-jobs — `pipeline`
// and `downstream_pipeline`): id, project_id, ref, sha, status, created_at,
// updated_at, web_url. (`project_id` appears on the `pipeline` shape only.)
type PipelineInfoObject struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id,omitempty"`
	Status    string `json:"status"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	WebURL    string `json:"web_url"`
	UpdatedAt string `json:"updated_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// pipelineInfoObject converts a *gl.PipelineInfo to its output shape, returning
// nil when the SDK value is nil.
func pipelineInfoObject(p *gl.PipelineInfo) *PipelineInfoObject {
	if p == nil {
		return nil
	}
	return &PipelineInfoObject{
		ID: p.ID, ProjectID: p.ProjectID, Status: p.Status,
		Ref: p.Ref, SHA: p.SHA, WebURL: p.WebURL,
		UpdatedAt: formatTimePtr(p.UpdatedAt), CreatedAt: formatTimePtr(p.CreatedAt),
	}
}

// pipelineInfoValueObject converts a gl.PipelineInfo value (the bridge
// `pipeline` key, which is non-pointer in the SDK) to its output shape,
// returning nil when zero-valued.
func pipelineInfoValueObject(p gl.PipelineInfo) *PipelineInfoObject {
	if p == (gl.PipelineInfo{}) {
		return nil
	}
	return pipelineInfoObject(&p)
}

// RunnerObject mirrors gl.JobRunner (the compact runner object embedded on a
// job's `runner` key).
//
// Documented reference subset per doc/api/jobs.md
// (https://docs.gitlab.com/api/jobs/#get-a-single-job — `runner`). The doc
// `runner` also lists ip_address, paused, runner_type, online, and status,
// but gl.JobRunner does not carry those fields, so they are not surfaced.
type RunnerObject struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	IsShared    bool   `json:"is_shared"`
	Name        string `json:"name"`
}

// runnerObject converts a gl.JobRunner value to its output shape, returning nil
// when the runner is empty (zero-valued).
func runnerObject(r gl.JobRunner) *RunnerObject {
	if r == (gl.JobRunner{}) {
		return nil
	}
	return &RunnerObject{
		ID: r.ID, Description: r.Description, Active: r.Active,
		IsShared: r.IsShared, Name: r.Name,
	}
}

// ArtifactObject mirrors gl.JobArtifact (one entry of a job's `artifacts`
// array).
type ArtifactObject struct {
	FileType   string `json:"file_type"`
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
	FileFormat string `json:"file_format"`
}

// artifactObjects converts a slice of gl.JobArtifact to its output shape,
// returning nil for an empty slice.
func artifactObjects(arts []gl.JobArtifact) []ArtifactObject {
	if len(arts) == 0 {
		return nil
	}
	out := make([]ArtifactObject, len(arts))
	for i, a := range arts {
		out[i] = ArtifactObject{
			FileType: a.FileType, Filename: a.Filename, Size: a.Size, FileFormat: a.FileFormat,
		}
	}
	return out
}

// ArtifactsFileObject mirrors gl.JobArtifactsFile (the job's `artifacts_file`
// key).
type ArtifactsFileObject struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// artifactsFileObject converts a gl.JobArtifactsFile value to its output shape,
// returning nil when empty (zero-valued).
func artifactsFileObject(f gl.JobArtifactsFile) *ArtifactsFileObject {
	if f == (gl.JobArtifactsFile{}) {
		return nil
	}
	return &ArtifactsFileObject{Filename: f.Filename, Size: f.Size}
}

// UserObject mirrors the `user` object embedded on a job, holding the fields
// the Jobs API documents for that sub-object.
//
// Documented reference subset per doc/api/jobs.md
// (https://docs.gitlab.com/api/jobs/#get-a-single-job — `user`):
// id, name, username, state, avatar_url, web_url, created_at, bio, location,
// public_email, linkedin, twitter, website_url, organization.
type UserObject struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	State        string `json:"state"`
	AvatarURL    string `json:"avatar_url"`
	WebURL       string `json:"web_url"`
	CreatedAt    string `json:"created_at,omitempty"`
	Bio          string `json:"bio,omitempty"`
	Location     string `json:"location,omitempty"`
	PublicEmail  string `json:"public_email,omitempty"`
	Linkedin     string `json:"linkedin,omitempty"`
	Twitter      string `json:"twitter,omitempty"`
	WebsiteURL   string `json:"website_url,omitempty"`
	Organization string `json:"organization,omitempty"`
}

// userObject converts a *gl.User to its output shape, returning nil when the
// SDK value is nil.
func userObject(u *gl.User) *UserObject {
	if u == nil {
		return nil
	}
	return &UserObject{
		ID: u.ID, Name: u.Name, Username: u.Username, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
		CreatedAt:    formatTimePtr(u.CreatedAt),
		Bio:          u.Bio,
		Location:     u.Location,
		PublicEmail:  u.PublicEmail,
		Linkedin:     u.Linkedin,
		Twitter:      u.Twitter,
		WebsiteURL:   u.WebsiteURL,
		Organization: u.Organization,
	}
}

// ProjectObject mirrors the `project` object embedded on a job, holding the
// field the Jobs API documents for that sub-object.
//
// Documented reference subset per doc/api/jobs.md
// (https://docs.gitlab.com/api/jobs/#get-a-single-job — `project`):
// ci_job_token_scope_enabled.
type ProjectObject struct {
	CIJobTokenScopeEnabled bool `json:"ci_job_token_scope_enabled"`
}

// projectObject converts a *gl.Project to its output shape, returning nil when
// the SDK value is nil.
func projectObject(p *gl.Project) *ProjectObject {
	if p == nil {
		return nil
	}
	return &ProjectObject{
		CIJobTokenScopeEnabled: p.CIJobTokenScopeEnabled,
	}
}
