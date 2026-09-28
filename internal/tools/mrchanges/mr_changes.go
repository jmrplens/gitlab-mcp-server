package mrchanges

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintVerifyMR is the 404 hint shared by MR-changes tools.
const hintVerifyMR = "verify project_id and merge_request_iid with merge_request.list"

// GetInput defines parameters for listing changed files in a merge request.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	Unidiff   bool                 `json:"unidiff,omitempty" jsonschema:"Return diffs in unified diff format (default: false)"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"For keyset pagination, the column to order results by"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	Collapsed     bool   `json:"collapsed,omitempty"`
	TooLarge      bool   `json:"too_large,omitempty"`
}

// Output holds the list of file diffs for a merge request.
type Output struct {
	toolutil.HintableOutput
	MRIID          int64            `json:"merge_request_iid"`
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
		Collapsed:     d.Collapsed,
		TooLarge:      d.TooLarge,
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
		return Output{}, errors.New("mrChangesGet: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrChangesGet", "merge_request_iid")
	}
	opts := &gl.ListMergeRequestDiffsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Unidiff {
		opts.Unidiff = new(true)
	}
	diffs, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
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
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"For keyset pagination, the column to order results by"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// DiffVersionGetInput defines parameters for getting a single MR diff version.
type DiffVersionGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"      jsonschema:"Merge request internal ID,required"`
	VersionID int64                `json:"version_id"  jsonschema:"Diff version ID,required"`
	Unidiff   bool                 `json:"unidiff,omitempty" jsonschema:"Return diffs in unified diff format (default: false)"`
}

// CommitStatsOutput mirrors gl.CommitStats: line additions/deletions for a commit.
type CommitStatsOutput struct {
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Total     int64 `json:"total"`
}

// DiffVersionCommitOutput is a full local mirror of gl.Commit (C-IMPORTS):
// every field returned by the GitLab commit payload inside a diff version is
// surfaced here. extended_trailers maps each trailer to the list of its
// values, which is how Gitlab::Git::Commit#parse_commit_trailers builds it and
// how lib/api/entities/commit.rb documents it; client-go's Commit declares it
// a map of strings, so it is read off the captured response
// ([capturedCommit]).
type DiffVersionCommitOutput struct {
	ID               string              `json:"id"`
	ShortID          string              `json:"short_id"`
	Title            string              `json:"title"`
	AuthorName       string              `json:"author_name"`
	AuthorEmail      string              `json:"author_email,omitempty"`
	AuthoredDate     string              `json:"authored_date,omitempty"`
	CommitterName    string              `json:"committer_name,omitempty"`
	CommitterEmail   string              `json:"committer_email,omitempty"`
	CommittedDate    string              `json:"committed_date,omitempty"`
	CreatedAt        string              `json:"created_at,omitempty"`
	Message          string              `json:"message,omitempty"`
	ParentIDs        []string            `json:"parent_ids,omitempty"`
	Stats            *CommitStatsOutput  `json:"stats,omitempty"`
	Status           string              `json:"status,omitempty"`
	ProjectID        int64               `json:"project_id,omitempty"`
	Trailers         map[string]string   `json:"trailers,omitempty"`
	ExtendedTrailers map[string][]string `json:"extended_trailers,omitempty"`
	WebURL           string              `json:"web_url,omitempty"`
}

// capturedDiffVersion is a merge request diff version as the single-version
// route sends it, read off the captured response (ADR-0021) as client-go's
// MergeRequestDiffVersion reads it, except for its commits, which the field
// here takes over so that each commit's extended_trailers can be read as
// GitLab sends them. encoding/json decodes a key into the shallowest field
// that names it, so the embedded version's own commits stay empty.
type capturedDiffVersion struct {
	gl.MergeRequestDiffVersion
	Commits []capturedCommit `json:"commits"`
}

// capturedCommit is one commit of a diff version as GitLab sends it:
// client-go's Commit, with extended_trailers taken over.
// lib/api/entities/commit.rb exposes it as each trailer mapped to the list of
// its values, and client-go's Commit declares it a map of strings, so a
// version holding one commit with a trailer fails in client-go's decoder as a
// whole. commits.Captured is the same type for the commit routes; it is
// mirrored here rather than imported (C-IMPORTS).
type capturedCommit struct {
	gl.Commit
	ExtendedTrailers map[string][]string `json:"extended_trailers"`
}

// misreadByClientGo reports whether err is client-go failing to decode an
// answer GitLab gave successfully into a struct of its own that cannot hold
// it, which is what a version holding a commit with a trailer produces (see
// [capturedCommit]). Such an answer is read from the capture by a type that
// can hold it, so the failure is not the handler's; any other error is. When
// the answer does not fit that type either, decoding the capture fails and
// the handler reports that instead. commits.MisreadByClientGo is the same
// predicate for the commit routes.
func misreadByClientGo(err error) bool {
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

// diffVersionCommits converts the commits of a diff version read off the
// captured response, in order.
func diffVersionCommits(rows []capturedCommit) []DiffVersionCommitOutput {
	var out []DiffVersionCommitOutput
	for i := range rows {
		c := &rows[i].Commit
		co := DiffVersionCommitOutput{
			ID:               c.ID,
			ShortID:          c.ShortID,
			Title:            c.Title,
			AuthorName:       c.AuthorName,
			AuthorEmail:      c.AuthorEmail,
			CommitterName:    c.CommitterName,
			CommitterEmail:   c.CommitterEmail,
			Message:          c.Message,
			ParentIDs:        c.ParentIDs,
			ProjectID:        c.ProjectID,
			Trailers:         c.Trailers,
			ExtendedTrailers: rows[i].ExtendedTrailers,
			WebURL:           c.WebURL,
		}
		if c.AuthoredDate != nil {
			co.AuthoredDate = c.AuthoredDate.Format(time.RFC3339)
		}
		if c.CommittedDate != nil {
			co.CommittedDate = c.CommittedDate.Format(time.RFC3339)
		}
		if c.CreatedAt != nil {
			co.CreatedAt = c.CreatedAt.Format(time.RFC3339)
		}
		if c.Status != nil {
			co.Status = string(*c.Status)
		}
		if c.Stats != nil {
			co.Stats = &CommitStatsOutput{
				Additions: c.Stats.Additions,
				Deletions: c.Stats.Deletions,
				Total:     c.Stats.Total,
			}
		}
		out = append(out, co)
	}
	return out
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
	PatchIDSHA     string                    `json:"patch_id_sha,omitempty"`
	Commits        []DiffVersionCommitOutput `json:"commits,omitempty"`
	Diffs          []FileDiffOutput          `json:"diffs,omitempty"`
}

// DiffVersionsListOutput holds the paginated list of diff versions.
type DiffVersionsListOutput struct {
	toolutil.HintableOutput
	DiffVersions []DiffVersionOutput       `json:"diff_versions"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// diffVersionToOutput converts the GitLab API response to the tool output
// format, filling from the decoded version and from what the capture read
// beside it. The version's commits are not among them: the list route sends
// none, and the single-version route's are read off the captured response
// ([diffVersionCommits]), since client-go's Commit cannot hold their
// extended_trailers.
func diffVersionToOutput(v *gl.MergeRequestDiffVersion, extra toolutil.MergeRequestDiffExtra) DiffVersionOutput {
	out := DiffVersionOutput{
		ID:             v.ID,
		HeadCommitSHA:  v.HeadCommitSHA,
		BaseCommitSHA:  v.BaseCommitSHA,
		StartCommitSHA: v.StartCommitSHA,
		MergeRequestID: v.MergeRequestID,
		State:          v.State,
		RealSize:       v.RealSize,
		PatchIDSHA:     extra.PatchIDSHA,
	}
	if v.CreatedAt != nil {
		out.CreatedAt = v.CreatedAt.Format(time.RFC3339)
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
		return DiffVersionsListOutput{}, toolutil.ErrRequiredInt64("mrDiffVersionsList", "merge_request_iid")
	}
	opts := &gl.GetMergeRequestDiffVersionsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	versions, resp, err := client.GL().MergeRequests.GetMergeRequestDiffVersions(
		string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return DiffVersionsListOutput{}, toolutil.WrapErrWithStatusHint("mrDiffVersionsList", err, http.StatusNotFound, hintVerifyMR)
	}
	extras, err := toolutil.CapturedMergeRequestDiffs(captured, len(versions))
	if err != nil {
		return DiffVersionsListOutput{}, toolutil.WrapErr("mrDiffVersionsList", err)
	}
	out := make([]DiffVersionOutput, len(versions))
	for i, v := range versions {
		out[i] = diffVersionToOutput(v, extras[i])
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
		return DiffVersionOutput{}, toolutil.ErrRequiredInt64("mrDiffVersionGet", "merge_request_iid")
	}
	if input.VersionID <= 0 {
		return DiffVersionOutput{}, toolutil.ErrRequiredInt64("mrDiffVersionGet", "version_id")
	}
	opts := &gl.GetSingleMergeRequestDiffVersionOptions{}
	if input.Unidiff {
		opts.Unidiff = new(true)
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _, err := client.GL().MergeRequests.GetSingleMergeRequestDiffVersion(
		string(input.ProjectID), input.MRIID, input.VersionID, opts, gl.WithContext(ctx),
	)
	if err != nil && !misreadByClientGo(err) {
		return DiffVersionOutput{}, toolutil.WrapErrWithStatusHint("mrDiffVersionGet", err, http.StatusNotFound, "verify version_id with mr_review.diff_versions_list")
	}
	var version capturedDiffVersion
	if err = captured.Decode(&version); err != nil {
		return DiffVersionOutput{}, toolutil.WrapErr("mrDiffVersionGet", err)
	}
	extra, err := toolutil.CapturedMergeRequestDiff(captured)
	if err != nil {
		return DiffVersionOutput{}, toolutil.WrapErr("mrDiffVersionGet", err)
	}
	out := diffVersionToOutput(&version.MergeRequestDiffVersion, extra)
	out.Commits = diffVersionCommits(version.Commits)
	return out, nil
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
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
}

// RawDiffsOutput holds the raw unified-diff output for a merge request.
type RawDiffsOutput struct {
	toolutil.HintableOutput
	MRIID   int64  `json:"merge_request_iid"`
	RawDiff string `json:"raw_diff"`
}

// RawDiffs retrieves the raw diff content for a merge request. The response is
// a plain-text unified diff that can be applied with git-apply(1).
func RawDiffs(ctx context.Context, client *gitlabclient.Client, input RawDiffsInput) (RawDiffsOutput, error) {
	if err := ctx.Err(); err != nil {
		return RawDiffsOutput{}, err
	}
	if input.ProjectID == "" {
		return RawDiffsOutput{}, errors.New("mrRawDiffs: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return RawDiffsOutput{}, toolutil.ErrRequiredInt64("mrRawDiffs", "merge_request_iid")
	}
	raw, _, err := client.GL().MergeRequests.ShowMergeRequestRawDiffs(
		string(input.ProjectID), input.MRIID, &gl.ShowMergeRequestRawDiffsOptions{}, gl.WithContext(ctx),
	)
	if err != nil {
		return RawDiffsOutput{}, toolutil.WrapErrWithStatusHint("mrRawDiffs", err, http.StatusNotFound, hintVerifyMR)
	}
	return RawDiffsOutput{MRIID: input.MRIID, RawDiff: string(raw)}, nil
}
