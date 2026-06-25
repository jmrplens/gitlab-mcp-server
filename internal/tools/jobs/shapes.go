package jobs

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface the fields of the SDK
// struct on the canonical json keys (commit, pipeline, project, runner, user,
// artifacts, artifacts_file, downstream_pipeline) and are replicated here as
// local types rather than imported from sibling packages to preserve the
// zero-import-cycle constraint (C-IMPORTS).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// CommitStatsOutput mirrors gl.CommitStats: line additions/deletions for a
// commit.
type CommitStatsOutput struct {
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Total     int64 `json:"total"`
}

// CommitObject is a full local mirror of gl.Commit (C-IMPORTS): every field
// returned by the GitLab commit payload embedded on a job's `commit` key is
// surfaced here.
type CommitObject struct {
	ID               string             `json:"id"`
	ShortID          string             `json:"short_id"`
	Title            string             `json:"title"`
	AuthorName       string             `json:"author_name"`
	AuthorEmail      string             `json:"author_email,omitempty"`
	AuthoredDate     string             `json:"authored_date,omitempty"`
	CommitterName    string             `json:"committer_name,omitempty"`
	CommitterEmail   string             `json:"committer_email,omitempty"`
	CommittedDate    string             `json:"committed_date,omitempty"`
	CreatedAt        string             `json:"created_at,omitempty"`
	Message          string             `json:"message,omitempty"`
	ParentIDs        []string           `json:"parent_ids,omitempty"`
	Stats            *CommitStatsOutput `json:"stats,omitempty"`
	Status           string             `json:"status,omitempty"`
	ProjectID        int64              `json:"project_id,omitempty"`
	Trailers         map[string]string  `json:"trailers,omitempty"`
	ExtendedTrailers map[string]string  `json:"extended_trailers,omitempty"`
	WebURL           string             `json:"web_url,omitempty"`
}

// commitObject converts a gl.Commit to its output shape, returning nil when the
// SDK value is nil.
func commitObject(c *gl.Commit) *CommitObject {
	if c == nil {
		return nil
	}
	out := &CommitObject{
		ID:               c.ID,
		ShortID:          c.ShortID,
		Title:            c.Title,
		AuthorName:       c.AuthorName,
		AuthorEmail:      c.AuthorEmail,
		AuthoredDate:     formatTimePtr(c.AuthoredDate),
		CommitterName:    c.CommitterName,
		CommitterEmail:   c.CommitterEmail,
		CommittedDate:    formatTimePtr(c.CommittedDate),
		CreatedAt:        formatTimePtr(c.CreatedAt),
		Message:          c.Message,
		ParentIDs:        c.ParentIDs,
		ProjectID:        c.ProjectID,
		Trailers:         c.Trailers,
		ExtendedTrailers: c.ExtendedTrailers,
		WebURL:           c.WebURL,
	}
	if c.Status != nil {
		out.Status = string(*c.Status)
	}
	if c.Stats != nil {
		out.Stats = &CommitStatsOutput{
			Additions: c.Stats.Additions,
			Deletions: c.Stats.Deletions,
			Total:     c.Stats.Total,
		}
	}
	return out
}

// PipelineObject mirrors gl.JobPipeline (the compact pipeline object embedded on
// a job's `pipeline` key).
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

// PipelineInfoObject mirrors gl.PipelineInfo (the compact pipeline object
// embedded on a bridge's `pipeline` and `downstream_pipeline` keys).
type PipelineInfoObject struct {
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

// pipelineInfoObject converts a *gl.PipelineInfo to its output shape, returning
// nil when the SDK value is nil.
func pipelineInfoObject(p *gl.PipelineInfo) *PipelineInfoObject {
	if p == nil {
		return nil
	}
	return &PipelineInfoObject{
		ID: p.ID, IID: p.IID, ProjectID: p.ProjectID, Status: p.Status,
		Source: p.Source, Ref: p.Ref, SHA: p.SHA, Name: p.Name, WebURL: p.WebURL,
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

// UserObject mirrors gl.User (the user object embedded on a job's `user` key).
// It surfaces the identifying fields of the triggering user without importing
// the full gl.User shape, which carries many unrelated account fields (sign-in
// IPs, 2FA, license seats, identities) not relevant to a CI job actor — the
// same approach the pipelineschedules package takes for its schedule owner.
type UserObject struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// userObject converts a *gl.User to its output shape, returning nil when the
// SDK value is nil.
func userObject(u *gl.User) *UserObject {
	if u == nil {
		return nil
	}
	return &UserObject{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
}

// ProjectObject mirrors gl.Project (the project object embedded on a job's
// `project` key). It surfaces the identifying fields of the owning project
// without importing the full gl.Project shape, which carries ~160 fields
// (statistics, permissions, settings, links) not relevant to a job's project
// reference.
type ProjectObject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
}

// projectObject converts a *gl.Project to its output shape, returning nil when
// the SDK value is nil.
func projectObject(p *gl.Project) *ProjectObject {
	if p == nil {
		return nil
	}
	return &ProjectObject{
		ID: p.ID, Name: p.Name, Path: p.Path,
		PathWithNamespace: p.PathWithNamespace, WebURL: p.WebURL,
	}
}
