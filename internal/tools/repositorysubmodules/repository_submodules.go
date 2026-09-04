package repositorysubmodules

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// UpdateInput is the input for updating a submodule reference.
type UpdateInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Submodule     string               `json:"submodule" jsonschema:"URL-encoded full path to the submodule,required"`
	Branch        string               `json:"branch" jsonschema:"Branch name to commit the update to,required"`
	CommitSHA     string               `json:"commit_sha" jsonschema:"Full commit SHA to update the submodule to,required"`
	CommitMessage string               `json:"commit_message,omitempty" jsonschema:"Custom commit message (optional)"`
}

// UpdateOutput is the output for a submodule update. The GitLab
// "update submodule" endpoint returns a gitlab.SubmoduleCommit, so the fields
// mirror that type one-to-one (1:1 audit): identity, author and committer
// attribution, authored/committed/created dates, parents, message, and build
// status. SubmoduleCommit does not carry a web URL.
type UpdateOutput struct {
	toolutil.HintableOutput
	ID             string   `json:"id"`
	ShortID        string   `json:"short_id"`
	Title          string   `json:"title"`
	AuthorName     string   `json:"author_name"`
	AuthorEmail    string   `json:"author_email"`
	AuthoredDate   string   `json:"authored_date,omitempty"`
	CommitterName  string   `json:"committer_name,omitempty"`
	CommitterEmail string   `json:"committer_email,omitempty"`
	CommittedDate  string   `json:"committed_date,omitempty"`
	CreatedAt      string   `json:"created_at,omitempty"`
	Message        string   `json:"message"`
	ParentIDs      []string `json:"parent_ids,omitempty"`
	Status         string   `json:"status,omitempty"`
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
		return UpdateOutput{}, toolutil.WrapErrWithStatusHint("update_repository_submodule", err, http.StatusNotFound, "verify project_id with gitlab_project_get and submodule path exists")
	}

	out := UpdateOutput{
		ID:             commit.ID,
		ShortID:        commit.ShortID,
		Title:          commit.Title,
		AuthorName:     commit.AuthorName,
		AuthorEmail:    commit.AuthorEmail,
		CommitterName:  commit.CommitterName,
		CommitterEmail: commit.CommitterEmail,
		Message:        commit.Message,
		ParentIDs:      commit.ParentIDs,
	}
	if commit.Status != nil {
		out.Status = string(*commit.Status)
	}
	if commit.AuthoredDate != nil {
		out.AuthoredDate = commit.AuthoredDate.String()
	}
	if commit.CreatedAt != nil {
		out.CreatedAt = commit.CreatedAt.String()
	}
	if commit.CommittedDate != nil {
		out.CommittedDate = commit.CommittedDate.String()
	}
	return out, nil
}
