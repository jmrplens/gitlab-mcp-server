package repositorysubmodules

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// UpdateInput is the input for updating a submodule reference.
type UpdateInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Submodule     string               `json:"submodule" jsonschema:"URL-encoded full path to the submodule,required"`
	Branch        string               `json:"branch" jsonschema:"Branch name to commit the update to,required"`
	CommitSHA     string               `json:"commit_sha" jsonschema:"Full commit SHA to update the submodule to,required"`
	CommitMessage string               `json:"commit_message,omitempty" jsonschema:"Custom commit message (optional)"`
}

// UpdateOutput is the commit a submodule update created, which the route
// renders through lib/api/entities/commit_detail.rb: identity, author and
// committer attribution, the three instants, parents, message, build status,
// the project, the web URL, the trailers and the commit's latest pipeline.
//
// client-go decodes the answer into a SubmoduleCommit, which models the first
// thirteen of those and none of the last five, so those are read off the
// captured response ([submoduleCommitExtra]). extended_trailers is typed as
// GitLab sends it, each key to the list of its values: client-go's Commit
// declares it a map of strings, which cannot hold a trailer GitLab has parsed.
type UpdateOutput struct {
	toolutil.HintableOutput
	ID               string                       `json:"id"`
	ShortID          string                       `json:"short_id"`
	Title            string                       `json:"title"`
	AuthorName       string                       `json:"author_name"`
	AuthorEmail      string                       `json:"author_email"`
	AuthoredDate     string                       `json:"authored_date,omitempty"`
	CommitterName    string                       `json:"committer_name,omitempty"`
	CommitterEmail   string                       `json:"committer_email,omitempty"`
	CommittedDate    string                       `json:"committed_date,omitempty"`
	CreatedAt        string                       `json:"created_at,omitempty"`
	Message          string                       `json:"message"`
	ParentIDs        []string                     `json:"parent_ids,omitempty"`
	Status           string                       `json:"status,omitempty"`
	ProjectID        int64                        `json:"project_id,omitempty"`
	WebURL           string                       `json:"web_url,omitempty"`
	Trailers         map[string]string            `json:"trailers,omitempty"`
	ExtendedTrailers map[string][]string          `json:"extended_trailers,omitempty"`
	LastPipeline     *toolutil.LastPipelineOutput `json:"last_pipeline,omitempty"`
}

// submoduleCommitExtra is what lib/api/entities/commit_detail.rb sends on the
// update route that client-go's SubmoduleCommit does not model. All five are
// exposed with no condition; last_pipeline is null while the commit has no
// pipeline the caller may read.
type submoduleCommitExtra struct {
	ProjectID        int64               `json:"project_id"`
	WebURL           string              `json:"web_url"`
	Trailers         map[string]string   `json:"trailers"`
	ExtendedTrailers map[string][]string `json:"extended_trailers"`
	LastPipeline     *gl.PipelineInfo    `json:"last_pipeline"`
}

// Update updates a submodule reference in a repository.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
	if string(input.ProjectID) == "" {
		return UpdateOutput{}, toolutil.ErrRequiredString("update_repository_submodule", "project_id")
	}
	opts := &gl.UpdateSubmoduleOptions{
		Branch:    new(input.Branch),
		CommitSHA: new(input.CommitSHA),
	}
	if input.CommitMessage != "" {
		opts.CommitMessage = new(input.CommitMessage)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	commit, _, err := client.GL().RepositorySubmodules.UpdateSubmodule(string(input.ProjectID), input.Submodule, opts, gl.WithContext(ctx))
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithStatusHint("update_repository_submodule", err, http.StatusNotFound, "verify project_id with project.get and submodule path exists")
	}
	var extra submoduleCommitExtra
	if err = captured.Decode(&extra); err != nil {
		return UpdateOutput{}, toolutil.WrapErr("update_repository_submodule", err)
	}

	// The three instants go out in RFC 3339, the wire form every other date
	// here takes. Go's default layout, which they used to carry, is one the
	// card's time helper cannot parse back, so the card printed it raw.
	out := UpdateOutput{
		ID:               commit.ID,
		ShortID:          commit.ShortID,
		Title:            commit.Title,
		AuthorName:       commit.AuthorName,
		AuthorEmail:      commit.AuthorEmail,
		AuthoredDate:     toolutil.RFC3339Ptr(commit.AuthoredDate),
		CommitterName:    commit.CommitterName,
		CommitterEmail:   commit.CommitterEmail,
		CommittedDate:    toolutil.RFC3339Ptr(commit.CommittedDate),
		CreatedAt:        toolutil.RFC3339Ptr(commit.CreatedAt),
		Message:          commit.Message,
		ParentIDs:        commit.ParentIDs,
		ProjectID:        extra.ProjectID,
		WebURL:           extra.WebURL,
		Trailers:         extra.Trailers,
		ExtendedTrailers: extra.ExtendedTrailers,
		LastPipeline:     toolutil.NewLastPipelineOutput(extra.LastPipeline),
	}
	if commit.Status != nil {
		out.Status = string(*commit.Status)
	}
	return out, nil
}
