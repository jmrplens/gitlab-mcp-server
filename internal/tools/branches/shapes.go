package branches

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output/input shapes mirrored from client-go sub-objects. Per the
// 1:1 audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here as local types rather than imported from
// sibling packages to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers:
//   - CommitOutput (gl.Commit), surfaced on Output.commit;
//   - CommitStatsOutput (gl.CommitStats) and LastPipelineOutput (gl.PipelineInfo)
//     embedded in a commit payload;
//   - BranchAccessDescriptionOutput (gl.BranchAccessDescription), surfaced on
//     ProtectedOutput.{push,merge,unprotect}_access_levels;
//   - BranchPermissionInput (gl.BranchPermissionOptions), the nested
//     allowed_to_{push,merge,unprotect} permission input.

// CommitStatsOutput mirrors gl.CommitStats: line additions/deletions/total for
// a commit.
type CommitStatsOutput struct {
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Total     int64 `json:"total"`
}

// LastPipelineOutput mirrors gl.PipelineInfo, the pipeline summary embedded in a
// commit payload as last_pipeline. Canonical shape shared via toolutil.
type LastPipelineOutput = toolutil.LastPipelineOutput

// CommitOutput mirrors gl.Commit, the full commit object embedded on a branch
// payload as commit.
type CommitOutput struct {
	ID               string              `json:"id"`
	ShortID          string              `json:"short_id"`
	Title            string              `json:"title"`
	Message          string              `json:"message,omitempty"`
	AuthorName       string              `json:"author_name"`
	AuthorEmail      string              `json:"author_email"`
	AuthoredDate     string              `json:"authored_date,omitempty"`
	CommitterName    string              `json:"committer_name"`
	CommitterEmail   string              `json:"committer_email"`
	CommittedDate    string              `json:"committed_date,omitempty"`
	CreatedAt        string              `json:"created_at,omitempty"`
	WebURL           string              `json:"web_url"`
	ParentIDs        []string            `json:"parent_ids,omitempty"`
	Status           string              `json:"status,omitempty"`
	ProjectID        int64               `json:"project_id,omitempty"`
	Trailers         map[string]string   `json:"trailers,omitempty"`
	ExtendedTrailers map[string]string   `json:"extended_trailers,omitempty"`
	LastPipeline     *LastPipelineOutput `json:"last_pipeline,omitempty"`
	Stats            *CommitStatsOutput  `json:"stats,omitempty"`
}

// commitToOutput maps gl.Commit to *CommitOutput, or nil when the branch has no
// embedded commit.
func commitToOutput(c *gl.Commit) *CommitOutput {
	if c == nil {
		return nil
	}
	out := &CommitOutput{
		ID:               c.ID,
		ShortID:          c.ShortID,
		Title:            c.Title,
		Message:          c.Message,
		AuthorName:       c.AuthorName,
		AuthorEmail:      c.AuthorEmail,
		CommitterName:    c.CommitterName,
		CommitterEmail:   c.CommitterEmail,
		WebURL:           c.WebURL,
		ParentIDs:        c.ParentIDs,
		ProjectID:        c.ProjectID,
		Trailers:         c.Trailers,
		ExtendedTrailers: c.ExtendedTrailers,
		LastPipeline:     pipelineInfoToOutput(c.LastPipeline),
	}
	if c.AuthoredDate != nil {
		out.AuthoredDate = c.AuthoredDate.String()
	}
	if c.CommittedDate != nil {
		out.CommittedDate = c.CommittedDate.String()
	}
	if c.CreatedAt != nil {
		out.CreatedAt = c.CreatedAt.String()
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

// BranchAccessDescriptionOutput mirrors gl.BranchAccessDescription, an entry in
// a protected branch's push/merge/unprotect access-level arrays.
type BranchAccessDescriptionOutput struct {
	ID                     int64  `json:"id"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	DeployKeyID            int64  `json:"deploy_key_id,omitempty"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
}

// branchAccessDescriptionsToOutput maps a slice of gl.BranchAccessDescription to
// the output shape, returning nil for an empty or all-nil slice.
func branchAccessDescriptionsToOutput(in []*gl.BranchAccessDescription) []BranchAccessDescriptionOutput {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchAccessDescriptionOutput, 0, len(in))
	for _, d := range in {
		if d == nil {
			continue
		}
		out = append(out, BranchAccessDescriptionOutput{
			ID:                     d.ID,
			AccessLevel:            int(d.AccessLevel),
			AccessLevelDescription: d.AccessLevelDescription,
			DeployKeyID:            d.DeployKeyID,
			UserID:                 d.UserID,
			GroupID:                d.GroupID,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BranchPermissionInput mirrors gl.BranchPermissionOptions, a single fine-grained
// allowed_to_{push,merge,unprotect} permission entry. Each entry grants access
// either by a coarse access level or by a specific user, group, or deploy key.
type BranchPermissionInput struct {
	ID          *int64 `json:"id,omitempty"           jsonschema:"ID of an existing access entry to update"`
	UserID      *int64 `json:"user_id,omitempty"      jsonschema:"Grant access to a specific user by ID"`
	GroupID     *int64 `json:"group_id,omitempty"     jsonschema:"Grant access to a specific group by ID"`
	DeployKeyID *int64 `json:"deploy_key_id,omitempty" jsonschema:"Grant access via a specific deploy key by ID"`
	AccessLevel *int   `json:"access_level,omitempty" jsonschema:"Coarse access level (0=No access, 30=Developer, 40=Maintainer)"`
	Destroy     *bool  `json:"_destroy,omitempty"     jsonschema:"When true, remove this existing access entry"`
}

// branchPermissionOptions maps a slice of BranchPermissionInput to the SDK
// *[]*gl.BranchPermissionOptions form, returning nil when no entries are given.
func branchPermissionOptions(in []BranchPermissionInput) *[]*gl.BranchPermissionOptions {
	if len(in) == 0 {
		return nil
	}
	out := make([]*gl.BranchPermissionOptions, 0, len(in))
	for _, p := range in {
		opt := &gl.BranchPermissionOptions{
			ID:          p.ID,
			UserID:      p.UserID,
			GroupID:     p.GroupID,
			DeployKeyID: p.DeployKeyID,
			Destroy:     p.Destroy,
		}
		if p.AccessLevel != nil {
			opt.AccessLevel = new(gl.AccessLevelValue(*p.AccessLevel))
		}
		out = append(out, opt)
	}
	return &out
}

// pipelineInfoToOutput maps gl.PipelineInfo to *LastPipelineOutput, or nil when
// the commit has no associated pipeline.
func pipelineInfoToOutput(p *gl.PipelineInfo) *LastPipelineOutput {
	return toolutil.NewLastPipelineOutput(p)
}
