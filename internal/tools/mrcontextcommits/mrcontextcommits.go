// Package mrcontextcommits implements MCP tool handlers for managing
// merge request context commits in GitLab. It wraps the
// MergeRequestContextCommitsService from client-go v2.
package mrcontextcommits

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CommitItem is a summary of a commit.
type CommitItem struct {
	ID          string `json:"id"`
	ShortID     string `json:"short_id"`
	Title       string `json:"title"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// List.

// ListInput is the input for listing MR context commits.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"mr_iid"     jsonschema:"Merge request IID,required"`
}

// ListOutput is the output for listing MR context commits.
type ListOutput struct {
	toolutil.HintableOutput
	Commits []CommitItem `json:"commits"`
}

// List returns the context commits for a merge request.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("list_mr_context_commits: project_id is required")
	}
	if input.MergeRequest <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("list_mr_context_commits", "mr_iid")
	}
	commits, _, err := client.GL().MergeRequestContextCommits.ListMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_mr_context_commits", err, http.StatusNotFound, "verify project_id and merge_request_iid with gitlab_list_merge_requests")
	}
	items := make([]CommitItem, 0, len(commits))
	for _, c := range commits {
		item := CommitItem{
			ID:          c.ID,
			ShortID:     c.ShortID,
			Title:       c.Title,
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
		}
		if c.CreatedAt != nil {
			item.CreatedAt = c.CreatedAt.String()
		}
		items = append(items, item)
	}
	return ListOutput{Commits: items}, nil
}

// Create.

// CreateInput is the input for creating MR context commits.
type CreateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"mr_iid"     jsonschema:"Merge request IID,required"`
	Commits      []string             `json:"commits"    jsonschema:"List of commit SHAs to add as context,required"`
}

// Create adds context commits to a merge request.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("create_mr_context_commits: project_id is required")
	}
	if input.MergeRequest <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("create_mr_context_commits", "mr_iid")
	}
	opts := &gl.CreateMergeRequestContextCommitsOptions{
		Commits: &input.Commits,
	}
	commits, _, err := client.GL().MergeRequestContextCommits.CreateMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("create_mr_context_commits", err, http.StatusBadRequest, "verify commit SHAs exist in the project repository")
	}
	items := make([]CommitItem, 0, len(commits))
	for _, c := range commits {
		item := CommitItem{
			ID:          c.ID,
			ShortID:     c.ShortID,
			Title:       c.Title,
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
		}
		if c.CreatedAt != nil {
			item.CreatedAt = c.CreatedAt.String()
		}
		items = append(items, item)
	}
	return ListOutput{Commits: items}, nil
}

// Delete.

// DeleteInput is the input for deleting MR context commits.
type DeleteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"mr_iid"     jsonschema:"Merge request IID,required"`
	Commits      []string             `json:"commits"    jsonschema:"List of commit SHAs to remove from context,required"`
}

// Delete removes context commits from a merge request.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("delete_mr_context_commits: project_id is required")
	}
	if input.MergeRequest <= 0 {
		return toolutil.ErrRequiredInt64("delete_mr_context_commits", "mr_iid")
	}
	opts := &gl.DeleteMergeRequestContextCommitsOptions{
		Commits: &input.Commits,
	}
	_, err := client.GL().MergeRequestContextCommits.DeleteMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_mr_context_commits", err, http.StatusNotFound, "verify commit SHAs are valid context commits for this MR")
	}
	return nil
}

// Markdown Formatters.
