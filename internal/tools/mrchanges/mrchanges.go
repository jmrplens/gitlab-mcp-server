// Package mrchanges implements retrieval of merge request file diffs, changes,
// and diff versions from the GitLab API. It exposes typed input/output structs
// and handler functions for listing changed files, diff versions, and
// individual diff version details in a merge request.
package mrchanges

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// hintVerifyMR is the 404 hint shared by MR-changes tools.
const hintVerifyMR = "verify project_id and mr_iid with gitlab_list_merge_requests"

// GetInput defines parameters for listing changed files in a merge request.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
}

// FileDiffOutput represents a single file diff in a merge request.
type FileDiffOutput struct {
	OldPath       string `json:"old_path"`
	NewPath       string `json:"new_path"`
	Diff          string `json:"diff"`
	NewFile       bool   `json:"new_file"`
	RenamedFile   bool   `json:"renamed_file"`
	DeletedFile   bool   `json:"deleted_file"`
	AMode         string `json:"a_mode"`
	BMode         string `json:"b_mode"`
	GeneratedFile bool   `json:"generated_file"`
}

// Output holds the list of file diffs for a merge request.
type Output struct {
	toolutil.HintableOutput
	MRIID          int64            `json:"mr_iid"`
	Changes        []FileDiffOutput `json:"changes"`
	TruncatedFiles []string         `json:"truncated_files,omitempty"`
}

// DiffToOutput converts a GitLab API [gl.MergeRequestDiff] to the MCP tool
// output format, preserving file paths, diff content, and file mode metadata.
func DiffToOutput(d *gl.MergeRequestDiff) FileDiffOutput {
	return FileDiffOutput{
		OldPath:       d.OldPath,
		NewPath:       d.NewPath,
		Diff:          d.Diff,
		NewFile:       d.NewFile,
		RenamedFile:   d.RenamedFile,
		DeletedFile:   d.DeletedFile,
		AMode:         d.AMode,
		BMode:         d.BMode,
		GeneratedFile: d.GeneratedFile,
	}
}

// Get retrieves the list of file diffs for a merge request by calling
// the GitLab Merge Request Diffs API. Returns all changed files with their
// diff content, old/new paths, and file status flags.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrChangesGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrChangesGet", "mr_iid")
	}
	diffs, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(string(input.ProjectID), input.MRIID, &gl.ListMergeRequestDiffsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrChangesGet", err, http.StatusNotFound, hintVerifyMR)
	}
	out := make([]FileDiffOutput, len(diffs))
	var truncated []string
	for i, d := range diffs {
		out[i] = DiffToOutput(d)
		if d.Diff == "" && !d.DeletedFile {
			truncated = append(truncated, d.NewPath)
		}
	}
	return Output{MRIID: input.MRIID, Changes: out, TruncatedFiles: truncated}, nil
}

// ---------------------------------------------------------------------------
// Markdown formatting
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Diff Versions
// ---------------------------------------------------------------------------.

// DiffVersionsListInput defines parameters for listing MR diff versions.
type DiffVersionsListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	toolutil.PaginationInput
}

// DiffVersionGetInput defines parameters for getting a single MR diff version.
type DiffVersionGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"      jsonschema:"Merge request internal ID,required"`
	VersionID int64                `json:"version_id"  jsonschema:"Diff version ID,required"`
	Unidiff   bool                 `json:"unidiff,omitempty" jsonschema:"Return diffs in unified diff format (default: false)"`
}

// DiffVersionCommitOutput represents a commit summary in a diff version.
type DiffVersionCommitOutput struct {
	ID         string `json:"id"`
	ShortID    string `json:"short_id"`
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// DiffVersionOutput represents a single merge request diff version.
type DiffVersionOutput struct {
	toolutil.HintableOutput
	ID             int64                     `json:"id"`
	HeadCommitSHA  string                    `json:"head_commit_sha,omitempty"`
	BaseCommitSHA  string                    `json:"base_commit_sha,omitempty"`
	StartCommitSHA string                    `json:"start_commit_sha,omitempty"`
	CreatedAt      string                    `json:"created_at,omitempty"`
	MergeRequestID int64                     `json:"merge_request_id,omitempty"`
	State          string                    `json:"state,omitempty"`
	RealSize       string                    `json:"real_size,omitempty"`
	Commits        []DiffVersionCommitOutput `json:"commits,omitempty"`
	Diffs          []FileDiffOutput          `json:"diffs,omitempty"`
}

// DiffVersionsListOutput holds the paginated list of diff versions.
type DiffVersionsListOutput struct {
	toolutil.HintableOutput
	DiffVersions []DiffVersionOutput       `json:"diff_versions"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// diffVersionToOutput converts the GitLab API response to the tool output format.
func diffVersionToOutput(v *gl.MergeRequestDiffVersion) DiffVersionOutput {
	out := DiffVersionOutput{
		ID:             v.ID,
		HeadCommitSHA:  v.HeadCommitSHA,
		BaseCommitSHA:  v.BaseCommitSHA,
		StartCommitSHA: v.StartCommitSHA,
		MergeRequestID: v.MergeRequestID,
		State:          v.State,
		RealSize:       v.RealSize,
	}
	if v.CreatedAt != nil {
		out.CreatedAt = v.CreatedAt.Format(time.RFC3339)
	}
	for _, c := range v.Commits {
		co := DiffVersionCommitOutput{
			ID:         c.ID,
			ShortID:    c.ShortID,
			Title:      c.Title,
			AuthorName: c.AuthorName,
		}
		if c.CreatedAt != nil {
			co.CreatedAt = c.CreatedAt.Format(time.RFC3339)
		}
		out.Commits = append(out.Commits, co)
	}
	for _, d := range v.Diffs {
		out.Diffs = append(out.Diffs, FileDiffOutput{
			OldPath:     d.OldPath,
			NewPath:     d.NewPath,
			Diff:        d.Diff,
			NewFile:     d.NewFile,
			RenamedFile: d.RenamedFile,
			DeletedFile: d.DeletedFile,
			AMode:       d.AMode,
			BMode:       d.BMode,
		})
	}
	return out
}

// ListDiffVersions retrieves the list of diff versions for a merge request.
func ListDiffVersions(ctx context.Context, client *gitlabclient.Client, input DiffVersionsListInput) (DiffVersionsListOutput, error) {
	if err := ctx.Err(); err != nil {
		return DiffVersionsListOutput{}, err
	}
	if input.ProjectID == "" {
		return DiffVersionsListOutput{}, errors.New("mrDiffVersionsList: project_id is required")
	}
	if input.MRIID <= 0 {
		return DiffVersionsListOutput{}, toolutil.ErrRequiredInt64("mrDiffVersionsList", "mr_iid")
	}
	opts := &gl.GetMergeRequestDiffVersionsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	versions, resp, err := client.GL().MergeRequests.GetMergeRequestDiffVersions(
		string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return DiffVersionsListOutput{}, toolutil.WrapErrWithStatusHint("mrDiffVersionsList", err, http.StatusNotFound, hintVerifyMR)
	}
	out := make([]DiffVersionOutput, len(versions))
	for i, v := range versions {
		out[i] = diffVersionToOutput(v)
	}
	return DiffVersionsListOutput{
		DiffVersions: out,
		Pagination:   toolutil.PaginationFromResponse(resp),
	}, nil
}

// GetDiffVersion retrieves a single diff version with its commits and diffs.
func GetDiffVersion(ctx context.Context, client *gitlabclient.Client, input DiffVersionGetInput) (DiffVersionOutput, error) {
	if err := ctx.Err(); err != nil {
		return DiffVersionOutput{}, err
	}
	if input.ProjectID == "" {
		return DiffVersionOutput{}, errors.New("mrDiffVersionGet: project_id is required")
	}
	if input.MRIID <= 0 {
		return DiffVersionOutput{}, toolutil.ErrRequiredInt64("mrDiffVersionGet", "mr_iid")
	}
	if input.VersionID <= 0 {
		return DiffVersionOutput{}, toolutil.ErrRequiredInt64("mrDiffVersionGet", "version_id")
	}
	opts := &gl.GetSingleMergeRequestDiffVersionOptions{}
	if input.Unidiff {
		opts.Unidiff = new(true)
	}
	version, _, err := client.GL().MergeRequests.GetSingleMergeRequestDiffVersion(
		string(input.ProjectID), input.MRIID, input.VersionID, opts, gl.WithContext(ctx))
	if err != nil {
		return DiffVersionOutput{}, toolutil.WrapErrWithStatusHint("mrDiffVersionGet", err, http.StatusNotFound, "verify version_id with gitlab_mr_diff_versions_list")
	}
	return diffVersionToOutput(version), nil
}

// ---------------------------------------------------------------------------
// Diff Versions — Markdown formatting
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Raw Diffs
// ---------------------------------------------------------------------------.

// RawDiffsInput defines parameters for retrieving raw diffs of a merge request.
type RawDiffsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
}

// RawDiffsOutput holds the raw unified-diff output for a merge request.
type RawDiffsOutput struct {
	toolutil.HintableOutput
	MRIID   int64  `json:"mr_iid"`
	RawDiff string `json:"raw_diff"`
}

// RawDiffs retrieves the raw diff content for a merge request. The response is
// a plain-text unified diff that can be applied with git-apply(1).
func RawDiffs(ctx context.Context, client *gitlabclient.Client, input RawDiffsInput) (RawDiffsOutput, error) {
	if err := ctx.Err(); err != nil {
		return RawDiffsOutput{}, err
	}
	if input.ProjectID == "" {
		return RawDiffsOutput{}, errors.New("mrRawDiffs: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return RawDiffsOutput{}, toolutil.ErrRequiredInt64("mrRawDiffs", "mr_iid")
	}
	raw, _, err := client.GL().MergeRequests.ShowMergeRequestRawDiffs(
		string(input.ProjectID), input.MRIID, &gl.ShowMergeRequestRawDiffsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return RawDiffsOutput{}, toolutil.WrapErrWithStatusHint("mrRawDiffs", err, http.StatusNotFound, hintVerifyMR)
	}
	return RawDiffsOutput{MRIID: input.MRIID, RawDiff: string(raw)}, nil
}
