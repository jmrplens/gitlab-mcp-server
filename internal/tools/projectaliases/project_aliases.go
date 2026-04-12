// Package projectaliases implements MCP tools for GitLab project alias management.
// Project aliases allow accessing projects via alternative names (admin-only feature).
package projectaliases

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds parameters for listing project aliases (no params needed).
type ListInput struct{}

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

// ListOutput represents a list of project aliases.
type ListOutput struct {
	toolutil.HintableOutput
	Aliases []Output `json:"aliases"`
}

// List retrieves all project aliases (admin-only).
func List(ctx context.Context, client *gitlabclient.Client, _ ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	aliases, _, err := client.GL().ProjectAliases.ListProjectAliases(gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list project aliases", err)
	}

	out := ListOutput{Aliases: make([]Output, 0, len(aliases))}
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
		return Output{}, toolutil.WrapErrWithMessage("get project alias", err)
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
		return Output{}, toolutil.WrapErrWithMessage("create project alias", err)
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
		return toolutil.WrapErrWithMessage("delete project alias", err)
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
