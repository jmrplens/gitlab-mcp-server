package mrcontextcommits

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// CommitItem is one context commit, the keys of lib/api/entities/commit.rb,
// which both context commit routes render.
//
// A context commit read back from the list is rebuilt from the row GitLab
// stored when it was pinned, and that row keeps no parents
// (MergeRequests::AddContextService drops them), so parent_ids arrives on the
// answer to create_context_commits and is empty on the list.
type CommitItem struct {
	ID               string            `json:"id"`
	ShortID          string            `json:"short_id"`
	Title            string            `json:"title"`
	Message          string            `json:"message,omitempty"`
	AuthorName       string            `json:"author_name"`
	AuthorEmail      string            `json:"author_email"`
	AuthoredDate     string            `json:"authored_date,omitempty"`
	CommitterName    string            `json:"committer_name,omitempty"`
	CommitterEmail   string            `json:"committer_email,omitempty"`
	CommittedDate    string            `json:"committed_date,omitempty"`
	CreatedAt        string            `json:"created_at,omitempty"`
	ParentIDs        []string          `json:"parent_ids,omitempty"`
	Trailers         map[string]string `json:"trailers,omitempty"`
	ExtendedTrailers map[string]string `json:"extended_trailers,omitempty"`
	WebURL           string            `json:"web_url,omitempty"`
}

// toCommitItems converts the commits either context commit route answers with,
// in order. The three instants go out in RFC 3339, the wire form every other
// date here takes; Go's default layout, which created_at used to carry, is one
// the Markdown time helper cannot parse back, so the table printed it raw.
//
// It takes the page rather than one commit because a converter of one
// gl.Commit is read by the R-PATH type grain as answering every route client-go
// decodes a Commit from (a commit's detail, a cherry-pick, a revert), whose
// CommitDetail keys these two routes never send.
func toCommitItems(commits []*gl.Commit) []CommitItem {
	items := make([]CommitItem, 0, len(commits))
	for _, c := range commits {
		items = append(items, CommitItem{
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
			ParentIDs:        c.ParentIDs,
			Trailers:         c.Trailers,
			ExtendedTrailers: c.ExtendedTrailers,
			WebURL:           c.WebURL,
		})
	}
	return items
}

// List.

// ListInput is the input for listing MR context commits.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"merge_request_iid"     jsonschema:"Merge request IID,required"`
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
		return ListOutput{}, toolutil.ErrRequiredInt64("list_mr_context_commits", "merge_request_iid")
	}
	commits, _, err := client.GL().MergeRequestContextCommits.ListMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_mr_context_commits", err, http.StatusNotFound, "verify project_id and merge_request_iid with merge_request.list")
	}
	return ListOutput{Commits: toCommitItems(commits)}, nil
}

// Create.

// CreateInput is the input for creating MR context commits.
type CreateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"merge_request_iid"     jsonschema:"Merge request IID,required"`
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
		return ListOutput{}, toolutil.ErrRequiredInt64("create_mr_context_commits", "merge_request_iid")
	}
	opts := &gl.CreateMergeRequestContextCommitsOptions{
		Commits: &input.Commits,
	}
	commits, _, err := client.GL().MergeRequestContextCommits.CreateMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("create_mr_context_commits", err, http.StatusBadRequest, "verify commit SHAs exist in the project repository")
	}
	return ListOutput{Commits: toCommitItems(commits)}, nil
}

// Delete.

// DeleteInput is the input for deleting MR context commits.
type DeleteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"merge_request_iid"     jsonschema:"Merge request IID,required"`
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
		return toolutil.ErrRequiredInt64("delete_mr_context_commits", "merge_request_iid")
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
