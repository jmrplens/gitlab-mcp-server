// Package applications implements MCP tools for GitLab Applications API.
package applications

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput is the input for listing applications.
type ListInput struct {
	Page    int64 `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64 `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// ApplicationItem represents a single application.
type ApplicationItem struct {
	ID              int64  `json:"id"`
	ApplicationID   string `json:"application_id"`
	ApplicationName string `json:"application_name"`
	Secret          string `json:"secret"`
	CallbackURL     string `json:"callback_url"`
	Confidential    bool   `json:"confidential"`
}

// ListOutput is the output for listing applications.
type ListOutput struct {
	toolutil.HintableOutput
	Applications []ApplicationItem         `json:"applications"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves all applications (admin).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListApplicationsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}

	apps, resp, err := client.GL().Applications.ListApplications(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_applications", err)
	}

	items := make([]ApplicationItem, 0, len(apps))
	for _, a := range apps {
		items = append(items, toItem(a))
	}

	return ListOutput{
		Applications: items,
		Pagination:   toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------.

// CreateInput is the input for creating an application.
type CreateInput struct {
	Name         string `json:"name" jsonschema:"Application name,required"`
	RedirectURI  string `json:"redirect_uri" jsonschema:"OAuth2 redirect URI,required"`
	Scopes       string `json:"scopes" jsonschema:"Space-separated list of scopes,required"`
	Confidential *bool  `json:"confidential,omitempty" jsonschema:"Whether application is confidential"`
}

// CreateOutput is the output for creating an application.
type CreateOutput struct {
	toolutil.HintableOutput
	ApplicationItem
}

// Create creates a new OAuth2 application (admin).
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (CreateOutput, error) {
	opts := &gl.CreateApplicationOptions{
		Name:         new(input.Name),
		RedirectURI:  new(input.RedirectURI),
		Scopes:       new(input.Scopes),
		Confidential: input.Confidential,
	}

	app, _, err := client.GL().Applications.CreateApplication(opts, gl.WithContext(ctx))
	if err != nil {
		return CreateOutput{}, toolutil.WrapErrWithMessage("create_application", err)
	}

	return CreateOutput{ApplicationItem: toItem(app)}, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------.

// DeleteInput is the input for deleting an application.
type DeleteInput struct {
	ID int64 `json:"id" jsonschema:"Application ID to delete,required"`
}

// Delete deletes an application (admin).
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ID <= 0 {
		return toolutil.ErrRequiredInt64("delete_application", "id")
	}
	_, err := client.GL().Applications.DeleteApplication(input.ID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("delete_application", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------.

// toItem converts the GitLab API response to the tool output format.
func toItem(a *gl.Application) ApplicationItem {
	return ApplicationItem{
		ID:              a.ID,
		ApplicationID:   a.ApplicationID,
		ApplicationName: a.ApplicationName,
		Secret:          a.Secret,
		CallbackURL:     a.CallbackURL,
		Confidential:    a.Confidential,
	}
}
