// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package branches

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
// payload as commit. extended_trailers maps each trailer to the list of its
// values, which is how Gitlab::Git::Commit#parse_commit_trailers builds it and
// how lib/api/entities/commit.rb documents it.
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
	ExtendedTrailers map[string][]string `json:"extended_trailers,omitempty"`
	LastPipeline     *LastPipelineOutput `json:"last_pipeline,omitempty"`
	Stats            *CommitStatsOutput  `json:"stats,omitempty"`
}

// commitToOutput maps gl.Commit to *CommitOutput, or nil when the branch has no
// embedded commit.
//
// The three timestamps go out in RFC 3339, the form every other date this
// server publishes takes and the one toolutil.FormatTime reads back. They used
// to be written with time.Time.String(), which is not RFC 3339, so nothing
// could parse them: the card's Committed line fell through to the display
// helper's escape branch and printed "2024-01-02 10:00:00 +0000 UTC" where
// every other card shows a date a reader can read.
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
		AuthoredDate:     toolutil.RFC3339Ptr(c.AuthoredDate),
		CommitterName:    c.CommitterName,
		CommitterEmail:   c.CommitterEmail,
		CommittedDate:    toolutil.RFC3339Ptr(c.CommittedDate),
		CreatedAt:        toolutil.RFC3339Ptr(c.CreatedAt),
		WebURL:           c.WebURL,
		ParentIDs:        c.ParentIDs,
		ProjectID:        c.ProjectID,
		Trailers:         c.Trailers,
		ExtendedTrailers: c.ExtendedTrailers,
		LastPipeline:     pipelineInfoToOutput(c.LastPipeline),
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
// a protected branch's push/merge/unprotect access-level arrays. GitLab sends
// member_role_id and member_role_name on an entry that grants a custom role,
// which needs an Ultimate license (ee/lib/ee/api/entities/protected_ref_access.rb).
// client-go does not model the name, so it is read off the captured response
// (toolutil.ProtectedRefAccessExtra); the gap is recorded in
// docs/development/upstream-bugs.md.
type BranchAccessDescriptionOutput struct {
	ID                     int64  `json:"id"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	DeployKeyID            int64  `json:"deploy_key_id,omitempty"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	MemberRoleID           int64  `json:"member_role_id,omitempty" tier:"ultimate"`
	MemberRoleName         string `json:"member_role_name,omitempty" tier:"ultimate"`
}

// branchAccessDescriptionsToOutput maps a slice of gl.BranchAccessDescription to
// the output shape, with the custom role name of each entry taken from the
// captured extra at the same index, returning nil for an empty or all-nil
// slice.
func branchAccessDescriptionsToOutput(in []*gl.BranchAccessDescription, extras []toolutil.ProtectedRefAccessExtra) []BranchAccessDescriptionOutput {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchAccessDescriptionOutput, 0, len(in))
	for i, d := range in {
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
			MemberRoleID:           d.MemberRoleID,
			MemberRoleName:         toolutil.MemberRoleNameAt(extras, i),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BranchPermissionInput mirrors gl.BranchPermissionOptions, a single fine-grained
// allowed_to_{push,merge,unprotect} permission entry. Each entry grants access
// either by a coarse access level or by a specific user, group, custom member
// role, or deploy key. A custom role needs an Ultimate license
// (doc/api/protected_branches.md).
type BranchPermissionInput struct {
	ID           *int64 `json:"id,omitempty"             jsonschema:"ID of an existing access entry to update"`
	UserID       *int64 `json:"user_id,omitempty"        jsonschema:"Grant access to a specific user by ID"`
	GroupID      *int64 `json:"group_id,omitempty"       jsonschema:"Grant access to a specific group by ID"`
	MemberRoleID *int64 `json:"member_role_id,omitempty" jsonschema:"Grant access to a custom member role by ID (Ultimate only)" tier:"ultimate"`
	DeployKeyID  *int64 `json:"deploy_key_id,omitempty"  jsonschema:"Grant access via a specific deploy key by ID"`
	AccessLevel  *int   `json:"access_level,omitempty"   jsonschema:"Coarse access level (0=No access, 30=Developer, 40=Maintainer)"`
	Destroy      *bool  `json:"_destroy,omitempty"       jsonschema:"When true, remove this existing access entry"`
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
			ID:           p.ID,
			UserID:       p.UserID,
			GroupID:      p.GroupID,
			MemberRoleID: p.MemberRoleID,
			DeployKeyID:  p.DeployKeyID,
			Destroy:      p.Destroy,
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
