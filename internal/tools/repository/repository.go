// Package repository implements MCP tool handlers for GitLab repository
// operations including tree listing, branch/tag/commit comparison, contributors,
// merge base, blob retrieval, changelog generation, and archive URLs.
// It wraps the Repositories service from client-go v2.
package repository

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TreeInput defines parameters for listing files in a repository tree.
type TreeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"          jsonschema:"Project ID or URL-encoded path,required"`
	Path      string               `json:"path,omitempty"      jsonschema:"Path inside the repository to list (default: root)"`
	Ref       string               `json:"ref,omitempty"       jsonschema:"Branch name, tag, or commit SHA (default: default branch)"`
	Recursive bool                 `json:"recursive,omitempty" jsonschema:"List files recursively through subdirectories"`
	toolutil.PaginationInput
}

// TreeNodeOutput represents a file or directory in the repository tree.
type TreeNodeOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Mode string `json:"mode"`
}

// TreeOutput holds a paginated list of tree nodes.
type TreeOutput struct {
	toolutil.HintableOutput
	Tree       []TreeNodeOutput          `json:"tree"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Tree retrieves the file/directory listing for a repository path.
func Tree(ctx context.Context, client *gitlabclient.Client, input TreeInput) (TreeOutput, error) {
	if err := ctx.Err(); err != nil {
		return TreeOutput{}, err
	}
	if input.ProjectID == "" {
		return TreeOutput{}, errors.New("repositoryTree: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.ListTreeOptions{}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	if input.Recursive {
		opts.Recursive = new(true)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	nodes, resp, err := client.GL().Repositories.ListTree(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return TreeOutput{}, toolutil.WrapErrWithMessage("repositoryTree", err)
	}

	out := make([]TreeNodeOutput, len(nodes))
	for i, n := range nodes {
		out[i] = TreeNodeOutput{
			ID:   n.ID,
			Name: n.Name,
			Type: n.Type,
			Path: n.Path,
			Mode: n.Mode,
		}
	}
	return TreeOutput{Tree: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// CompareInput defines parameters for comparing branches, tags, or commits.
type CompareInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"           jsonschema:"Project ID or URL-encoded path,required"`
	From      string               `json:"from"                 jsonschema:"Branch name, tag, or commit SHA to compare from,required"`
	To        string               `json:"to"                   jsonschema:"Branch name, tag, or commit SHA to compare to,required"`
	Straight  bool                 `json:"straight,omitempty"   jsonschema:"Use straight comparison (from..to) instead of merge-base (from...to)"`
	Unidiff   bool                 `json:"unidiff,omitempty"    jsonschema:"Return diffs in unified diff format"`
}

// DiffOutput is an alias for the shared diff type in toolutil.
type DiffOutput = toolutil.DiffOutput

// CompareOutput holds the comparison result.
type CompareOutput struct {
	toolutil.HintableOutput
	Commits        []commits.Output `json:"commits"`
	Diffs          []DiffOutput     `json:"diffs"`
	CompareTimeout bool             `json:"compare_timeout"`
	CompareSameRef bool             `json:"compare_same_ref"`
	WebURL         string           `json:"web_url"`
}

// Compare compares two branches, tags, or commits in a project.
func Compare(ctx context.Context, client *gitlabclient.Client, input CompareInput) (CompareOutput, error) {
	if err := ctx.Err(); err != nil {
		return CompareOutput{}, err
	}
	if input.ProjectID == "" {
		return CompareOutput{}, errors.New("repositoryCompare: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.CompareOptions{
		From: new(input.From),
		To:   new(input.To),
	}
	if input.Straight {
		opts.Straight = new(true)
	}
	if input.Unidiff {
		opts.Unidiff = new(true)
	}

	cmp, _, err := client.GL().Repositories.Compare(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return CompareOutput{}, toolutil.WrapErrWithMessage("repositoryCompare", err)
	}

	commitList := make([]commits.Output, len(cmp.Commits))
	for i, c := range cmp.Commits {
		commitList[i] = commits.ToOutput(c)
	}

	diffs := make([]toolutil.DiffOutput, len(cmp.Diffs))
	for i, d := range cmp.Diffs {
		diffs[i] = toolutil.DiffToOutput(d)
	}

	return CompareOutput{
		Commits:        commitList,
		Diffs:          diffs,
		CompareTimeout: cmp.CompareTimeout,
		CompareSameRef: cmp.CompareSameRef,
		WebURL:         cmp.WebURL,
	}, nil
}

// ---------------------------------------------------------------------------
// Contributors
// ---------------------------------------------------------------------------.

// ContributorsInput defines parameters for listing repository contributors.
type ContributorsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by: name, email, or commits (default: commits)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc (default: asc)"`
	toolutil.PaginationInput
}

// ContributorOutput represents a repository contributor.
type ContributorOutput struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Commits   int64  `json:"commits"`
	Additions int64  `json:"additions"`
	Deletions int64  `json:"deletions"`
}

// ContributorsOutput holds a paginated list of contributors.
type ContributorsOutput struct {
	toolutil.HintableOutput
	Contributors []ContributorOutput       `json:"contributors"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// Contributors lists the repository contributors with commit/addition/deletion counts.
func Contributors(ctx context.Context, client *gitlabclient.Client, input ContributorsInput) (ContributorsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ContributorsOutput{}, err
	}
	if input.ProjectID == "" {
		return ContributorsOutput{}, errors.New("repositoryContributors: project_id is required")
	}
	opts := &gl.ListContributorsOptions{}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	contribs, resp, err := client.GL().Repositories.Contributors(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ContributorsOutput{}, toolutil.WrapErrWithMessage("repositoryContributors", err)
	}
	out := make([]ContributorOutput, len(contribs))
	for i, c := range contribs {
		out[i] = ContributorOutput{
			Name:      c.Name,
			Email:     c.Email,
			Commits:   c.Commits,
			Additions: c.Additions,
			Deletions: c.Deletions,
		}
	}
	return ContributorsOutput{Contributors: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// MergeBase
// ---------------------------------------------------------------------------.

// MergeBaseInput defines parameters for finding the merge base of two or more refs.
type MergeBaseInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Refs      []string             `json:"refs"       jsonschema:"Two or more branch names, tags, or commit SHAs to find the merge base of,required"`
}

// MergeBase finds the common ancestor (merge base) of two or more refs.
func MergeBase(ctx context.Context, client *gitlabclient.Client, input MergeBaseInput) (commits.Output, error) {
	if err := ctx.Err(); err != nil {
		return commits.Output{}, err
	}
	if input.ProjectID == "" {
		return commits.Output{}, errors.New("repositoryMergeBase: project_id is required")
	}
	if len(input.Refs) < 2 {
		return commits.Output{}, errors.New("repositoryMergeBase: at least 2 refs are required")
	}
	opts := &gl.MergeBaseOptions{
		Ref: new(input.Refs),
	}
	c, _, err := client.GL().Repositories.MergeBase(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return commits.Output{}, toolutil.WrapErrWithMessage("repositoryMergeBase", err)
	}
	return commits.ToOutput(c), nil
}

// ---------------------------------------------------------------------------
// Blob / RawBlobContent
// ---------------------------------------------------------------------------.

// BlobInput defines parameters for retrieving a git blob by SHA.
type BlobInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Blob SHA (from tree listing or commit diff),required"`
}

// BlobOutput holds the base64-encoded content of a git blob.
type BlobOutput struct {
	toolutil.HintableOutput
	SHA     string `json:"sha"`
	Size    int    `json:"size"`
	Content string `json:"content"`
}

// Blob retrieves a git blob by SHA and returns its content as base64.
func Blob(ctx context.Context, client *gitlabclient.Client, input BlobInput) (BlobOutput, error) {
	if err := ctx.Err(); err != nil {
		return BlobOutput{}, err
	}
	if input.ProjectID == "" {
		return BlobOutput{}, errors.New("repositoryBlob: project_id is required")
	}
	data, _, err := client.GL().Repositories.Blob(string(input.ProjectID), input.SHA, gl.WithContext(ctx))
	if err != nil {
		return BlobOutput{}, toolutil.WrapErrWithMessage("repositoryBlob", err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return BlobOutput{
		SHA:     input.SHA,
		Size:    len(data),
		Content: encoded,
	}, nil
}

// RawBlobContentOutput holds the raw text content of a git blob.
type RawBlobContentOutput struct {
	toolutil.HintableOutput
	SHA     string `json:"sha"`
	Size    int    `json:"size"`
	Content string `json:"content"`
}

// RawBlobContent retrieves the raw content of a git blob by SHA as text.
func RawBlobContent(ctx context.Context, client *gitlabclient.Client, input BlobInput) (RawBlobContentOutput, error) {
	if err := ctx.Err(); err != nil {
		return RawBlobContentOutput{}, err
	}
	if input.ProjectID == "" {
		return RawBlobContentOutput{}, errors.New("repositoryRawBlobContent: project_id is required")
	}
	data, _, err := client.GL().Repositories.RawBlobContent(string(input.ProjectID), input.SHA, gl.WithContext(ctx))
	if err != nil {
		return RawBlobContentOutput{}, toolutil.WrapErrWithMessage("repositoryRawBlobContent", err)
	}
	return RawBlobContentOutput{
		SHA:     input.SHA,
		Size:    len(data),
		Content: string(data),
	}, nil
}

// ---------------------------------------------------------------------------
// Archive (returns URL, not binary content)
// ---------------------------------------------------------------------------.

// ArchiveInput defines parameters for getting a repository archive URL.
type ArchiveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"      jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha,omitempty"   jsonschema:"Commit SHA, branch, or tag to archive (default: default branch)"`
	Format    string               `json:"format,omitempty" jsonschema:"Archive format: tar.gz, tar.bz2, tbz, tbz2, tb2, bz2, tar, zip (default: tar.gz)"`
	Path      string               `json:"path,omitempty"   jsonschema:"Subdirectory path to archive (omit for entire repo)"`
}

// ArchiveOutput holds archive metadata and download URL.
type ArchiveOutput struct {
	toolutil.HintableOutput
	ProjectID string `json:"project_id"`
	SHA       string `json:"sha,omitempty"`
	Format    string `json:"format"`
	URL       string `json:"url"`
}

// Archive generates the download URL for a repository archive.
// Binary content is not returned; use the URL to download the archive.
func Archive(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (ArchiveOutput, error) {
	if err := ctx.Err(); err != nil {
		return ArchiveOutput{}, err
	}
	if input.ProjectID == "" {
		return ArchiveOutput{}, errors.New("repositoryArchive: project_id is required")
	}
	format := input.Format
	if format == "" {
		format = "tar.gz"
	}
	baseURL := client.GL().BaseURL().String()
	pid := string(input.ProjectID)
	archiveURL := fmt.Sprintf("%sprojects/%s/repository/archive.%s", baseURL, pid, format)
	if input.SHA != "" {
		archiveURL += "?sha=" + input.SHA
	}
	return ArchiveOutput{
		ProjectID: pid,
		SHA:       input.SHA,
		Format:    format,
		URL:       archiveURL,
	}, nil
}

// ---------------------------------------------------------------------------
// AddChangelog
// ---------------------------------------------------------------------------.

// AddChangelogInput defines parameters for adding changelog data to a changelog file.
type AddChangelogInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"           jsonschema:"Project ID or URL-encoded path,required"`
	Version    string               `json:"version"              jsonschema:"Version string for the changelog,required"`
	Branch     string               `json:"branch,omitempty"     jsonschema:"Branch to commit the changelog to (default: default branch)"`
	ConfigFile string               `json:"config_file,omitempty" jsonschema:"Path to the changelog config file in the project"`
	File       string               `json:"file,omitempty"       jsonschema:"Path to the changelog file (default: CHANGELOG.md)"`
	From       string               `json:"from,omitempty"       jsonschema:"Start of the range (commit SHA or tag)"`
	To         string               `json:"to,omitempty"         jsonschema:"End of the range (commit SHA or tag, default: HEAD)"`
	Message    string               `json:"message,omitempty"    jsonschema:"Commit message for the changelog update"`
	Trailer    string               `json:"trailer,omitempty"    jsonschema:"Git trailer to use for changelog generation (default: Changelog)"`
}

// AddChangelogOutput confirms the changelog was added.
type AddChangelogOutput struct {
	toolutil.HintableOutput
	Success bool   `json:"success"`
	Version string `json:"version"`
}

// AddChangelog adds changelog data to a changelog file by creating a commit.
func AddChangelog(ctx context.Context, client *gitlabclient.Client, input AddChangelogInput) (AddChangelogOutput, error) {
	if err := ctx.Err(); err != nil {
		return AddChangelogOutput{}, err
	}
	if input.ProjectID == "" {
		return AddChangelogOutput{}, errors.New("addChangelog: project_id is required")
	}
	if input.Version == "" {
		return AddChangelogOutput{}, errors.New("addChangelog: version is required")
	}
	opts := &gl.AddChangelogOptions{
		Version: new(input.Version),
	}
	if input.Branch != "" {
		opts.Branch = new(input.Branch)
	}
	if input.ConfigFile != "" {
		opts.ConfigFile = new(input.ConfigFile)
	}
	if input.File != "" {
		opts.File = new(input.File)
	}
	if input.From != "" {
		opts.From = new(input.From)
	}
	if input.To != "" {
		opts.To = new(input.To)
	}
	if input.Message != "" {
		opts.Message = new(input.Message)
	}
	if input.Trailer != "" {
		opts.Trailer = new(input.Trailer)
	}
	_, err := client.GL().Repositories.AddChangelog(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return AddChangelogOutput{}, toolutil.WrapErrWithMessage("addChangelog", err)
	}
	return AddChangelogOutput{Success: true, Version: input.Version}, nil
}

// ---------------------------------------------------------------------------
// GenerateChangelogData
// ---------------------------------------------------------------------------.

// GenerateChangelogInput defines parameters for generating changelog data.
type GenerateChangelogInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	Version    string               `json:"version"               jsonschema:"Version string,required"`
	ConfigFile string               `json:"config_file,omitempty"  jsonschema:"Path to the changelog config file"`
	From       string               `json:"from,omitempty"        jsonschema:"Start of the range (commit SHA or tag)"`
	To         string               `json:"to,omitempty"          jsonschema:"End of the range (commit SHA or tag, default: HEAD)"`
	Trailer    string               `json:"trailer,omitempty"     jsonschema:"Git trailer to use (default: Changelog)"`
}

// ChangelogDataOutput holds the generated changelog notes.
type ChangelogDataOutput struct {
	toolutil.HintableOutput
	Notes string `json:"notes"`
}

// GenerateChangelogData generates changelog notes without committing.
func GenerateChangelogData(ctx context.Context, client *gitlabclient.Client, input GenerateChangelogInput) (ChangelogDataOutput, error) {
	if err := ctx.Err(); err != nil {
		return ChangelogDataOutput{}, err
	}
	if input.ProjectID == "" {
		return ChangelogDataOutput{}, errors.New("generateChangelogData: project_id is required")
	}
	if input.Version == "" {
		return ChangelogDataOutput{}, errors.New("generateChangelogData: version is required")
	}
	opts := gl.GenerateChangelogDataOptions{
		Version: new(input.Version),
	}
	if input.ConfigFile != "" {
		opts.ConfigFile = new(input.ConfigFile)
	}
	if input.From != "" {
		opts.From = new(input.From)
	}
	if input.To != "" {
		opts.To = new(input.To)
	}
	if input.Trailer != "" {
		opts.Trailer = new(input.Trailer)
	}
	data, _, err := client.GL().Repositories.GenerateChangelogData(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ChangelogDataOutput{}, toolutil.WrapErrWithMessage("generateChangelogData", err)
	}
	return ChangelogDataOutput{Notes: data.Notes}, nil
}
