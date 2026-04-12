// Package repositorysubmodules implements an MCP tool handler for updating
// Git submodule references in a GitLab repository. It wraps the
// RepositorySubmodulesService from client-go v2.
package repositorysubmodules

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// UpdateInput is the input for updating a submodule reference.
type UpdateInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Submodule     string               `json:"submodule" jsonschema:"URL-encoded full path to the submodule,required"`
	Branch        string               `json:"branch" jsonschema:"Branch name to commit the update to,required"`
	CommitSHA     string               `json:"commit_sha" jsonschema:"Full commit SHA to update the submodule to,required"`
	CommitMessage string               `json:"commit_message,omitempty" jsonschema:"Custom commit message (optional)"`
}

// UpdateOutput is the output for a submodule update.
type UpdateOutput struct {
	toolutil.HintableOutput
	ID            string `json:"id"`
	ShortID       string `json:"short_id"`
	Title         string `json:"title"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email"`
	Message       string `json:"message"`
	CreatedAt     string `json:"created_at,omitempty"`
	CommittedDate string `json:"committed_date,omitempty"`
}

// Update updates a submodule reference in a repository.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
	opts := &gl.UpdateSubmoduleOptions{
		Branch:    new(input.Branch),
		CommitSHA: new(input.CommitSHA),
	}
	if input.CommitMessage != "" {
		opts.CommitMessage = new(input.CommitMessage)
	}

	commit, _, err := client.GL().RepositorySubmodules.UpdateSubmodule(string(input.ProjectID), input.Submodule, opts, gl.WithContext(ctx))
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithMessage("update_repository_submodule", err)
	}

	out := UpdateOutput{
		ID:          commit.ID,
		ShortID:     commit.ShortID,
		Title:       commit.Title,
		AuthorName:  commit.AuthorName,
		AuthorEmail: commit.AuthorEmail,
		Message:     commit.Message,
	}
	if commit.CreatedAt != nil {
		out.CreatedAt = commit.CreatedAt.String()
	}
	if commit.CommittedDate != nil {
		out.CommittedDate = commit.CommittedDate.String()
	}
	return out, nil
}
