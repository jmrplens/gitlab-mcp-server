package projectaliases

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput holds the page of project aliases to list. GitLab pages
// GET /project_aliases, and client-go's ListProjectAliases takes no options
// struct, so the page travels as a request option.
type ListInput struct {
	toolutil.PaginationInput
}

// GetInput holds parameters for retrieving a specific project alias.
type GetInput struct {
	Name string `json:"name" jsonschema:"The alias name to look up,required"`
}

// CreateInput holds parameters for creating a new project alias.
type CreateInput struct {
	Name      string `json:"name" jsonschema:"The alias name to create,required"`
	ProjectID int64  `json:"project_id" jsonschema:"The numeric project ID to alias,required"`
}

// DeleteInput holds parameters for deleting a project alias.
type DeleteInput struct {
	Name string `json:"name" jsonschema:"The alias name to delete,required"`
}

// Output represents a single project alias.
type Output struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
}

// ListOutput represents one page of project aliases and the pagination GitLab
// sent with it.
type ListOutput struct {
	toolutil.HintableOutput
	Aliases    []Output                  `json:"aliases"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves one page of the instance's project aliases (admin-only).
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	aliases, resp, err := client.GL().ProjectAliases.ListProjectAliases(gl.WithContext(ctx), toolutil.PaginationRequestOption(in.PaginationInput))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list project aliases", err, http.StatusForbidden, "project aliases require administrator access")
	}

	out := ListOutput{
		Aliases:    make([]Output, 0, len(aliases)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, a := range aliases {
		out.Aliases = append(out.Aliases, toOutput(a))
	}
	return out, nil
}

// Get retrieves a specific project alias by name.
func Get(ctx context.Context, client *gitlabclient.Client, in GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}

	alias, _, err := client.GL().ProjectAliases.GetProjectAlias(in.Name, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get project alias", err, http.StatusNotFound, "verify the alias name with project_alias.list")
	}

	return toOutput(alias), nil
}

// Create creates a new project alias.
func Create(ctx context.Context, client *gitlabclient.Client, in CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if in.ProjectID == 0 {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := &gl.CreateProjectAliasOptions{
		Name:      new(in.Name),
		ProjectID: in.ProjectID,
	}
	alias, _, err := client.GL().ProjectAliases.CreateProjectAlias(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("create project alias", err, http.StatusBadRequest, "verify the project_id exists and alias name is unique. Requires administrator access")
	}

	return toOutput(alias), nil
}

// Delete removes a project alias by name.
func Delete(ctx context.Context, client *gitlabclient.Client, in DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.Name == "" {
		return toolutil.ErrFieldRequired("name")
	}

	_, err := client.GL().ProjectAliases.DeleteProjectAlias(in.Name, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete project alias", err, http.StatusNotFound, "verify the alias name with project_alias.list")
	}

	return nil
}

func toOutput(a *gl.ProjectAlias) Output {
	return Output{
		ID:        a.ID,
		ProjectID: a.ProjectID,
		Name:      a.Name,
	}
}
