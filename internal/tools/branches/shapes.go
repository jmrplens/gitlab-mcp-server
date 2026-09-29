package branches

import (
	"encoding/json"
	"errors"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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
//     allowed_to_{push,merge,unprotect} permission input;
//   - capturedBranch and capturedCommit, a branch and its commit read off the
//     captured response, which is what the branch handlers convert.

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
// how lib/api/entities/commit.rb documents it; client-go's Commit declares it
// a map of strings, so it is read off the captured response
// ([capturedCommit]).
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

// capturedBranch is a branch as GitLab sends it, read off the captured
// response (ADR-0021) as client-go's Branch reads it, except for its commit,
// which the field here takes over so that the commit's extended_trailers can
// be read as GitLab sends them. encoding/json decodes a key into the
// shallowest field that names it, so the embedded Branch's own commit stays
// empty.
type capturedBranch struct {
	gl.Branch
	Commit *capturedCommit `json:"commit"`
}

// capturedCommit is a branch's commit as GitLab sends it: client-go's Commit,
// with extended_trailers taken over. lib/api/entities/commit.rb exposes it as
// each trailer mapped to the list of its values, and client-go's Commit
// declares it a map of strings, so a commit carrying a trailer fails in
// client-go's decoder, which then returns no branch at all, and a page holding
// one such branch fails as a whole. commits.Captured is the same type for the
// commit routes; it is mirrored here rather than imported (C-IMPORTS).
type capturedCommit struct {
	gl.Commit
	ExtendedTrailers map[string][]string `json:"extended_trailers"`
}

// misreadByClientGo reports whether err is client-go failing to decode an
// answer GitLab gave successfully into a struct of its own that cannot hold
// it, which is what a branch whose commit carries a trailer produces (see
// [capturedCommit]). Such an answer is read from the capture by a type that
// can hold it, so the failure is not the handler's; any other error is. When
// the answer does not fit that type either, decoding the capture fails and
// the handler reports that instead. commits.MisreadByClientGo is the same
// predicate for the commit routes.
func misreadByClientGo(err error) bool {
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

// outputFromCaptured converts a branch read off the captured response, its
// commit's extended_trailers included.
func outputFromCaptured(b *capturedBranch) Output {
	out := ToOutput(&b.Branch)
	if b.Commit != nil {
		out.Commit = commitToOutput(&b.Commit.Commit)
		out.Commit.ExtendedTrailers = b.Commit.ExtendedTrailers
	}
	return out
}

// capturedOutput decodes the one branch a route answered with from the
// captured response.
func capturedOutput(captured *gitlabclient.ResponseCapture) (Output, error) {
	var b capturedBranch
	if err := captured.Decode(&b); err != nil {
		return Output{}, err
	}
	return outputFromCaptured(&b), nil
}

// commitToOutput maps gl.Commit to *CommitOutput, or nil when the branch has no
// embedded commit. extended_trailers is not mapped: a Commit client-go decoded
// cannot hold it and so carried none; [outputFromCaptured] adds it.
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
		ID:             c.ID,
		ShortID:        c.ShortID,
		Title:          c.Title,
		Message:        c.Message,
		AuthorName:     c.AuthorName,
		AuthorEmail:    c.AuthorEmail,
		AuthoredDate:   toolutil.RFC3339Ptr(c.AuthoredDate),
		CommitterName:  c.CommitterName,
		CommitterEmail: c.CommitterEmail,
		CommittedDate:  toolutil.RFC3339Ptr(c.CommittedDate),
		CreatedAt:      toolutil.RFC3339Ptr(c.CreatedAt),
		WebURL:         c.WebURL,
		ParentIDs:      c.ParentIDs,
		ProjectID:      c.ProjectID,
		Trailers:       c.Trailers,
		LastPipeline:   pipelineInfoToOutput(c.LastPipeline),
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
