package applications

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput is the input for listing applications. It mirrors
// gl.ListApplicationsOptions, which embeds gl.ListOptions and therefore exposes
// both offset and keyset pagination plus order_by/sort.
type ListInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"For keyset pagination: name of the column by which to order results"`
	Sort    string `json:"sort,omitempty"     jsonschema:"For keyset pagination: sort order (asc or desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ApplicationItem represents a single application.
type ApplicationItem struct {
	ID              int64    `json:"id"`
	ApplicationID   string   `json:"application_id"`
	ApplicationName string   `json:"application_name"`
	Secret          string   `json:"secret"`
	CallbackURL     string   `json:"callback_url"`
	Confidential    bool     `json:"confidential"`
	Scopes          []string `json:"scopes,omitempty"`
}

// ListOutput is the output for listing applications.
type ListOutput struct {
	toolutil.HintableOutput
	Applications []ApplicationItem         `json:"applications"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves all applications (admin).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListApplicationsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}

	apps, resp, err := client.GL().Applications.ListApplications(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_applications", err, http.StatusForbidden, "requires administrator access")
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
		return CreateOutput{}, toolutil.WrapErrWithStatusHint("create_application", err, http.StatusBadRequest, "verify redirect_uri is a valid URL and scopes are valid. Requires administrator access")
	}

	return CreateOutput{ApplicationItem: toItem(app)}, nil
}

// ---------------------------------------------------------------------------
// Renew secret
// ---------------------------------------------------------------------------.

// RenewSecretInput is the input for renewing an application secret.
type RenewSecretInput struct {
	ID int64 `json:"id" jsonschema:"Application ID whose secret should be renewed,required"`
}

// RenewSecretOutput is the output for renewing an application secret. It carries
// the application record including the freshly generated secret.
type RenewSecretOutput struct {
	toolutil.HintableOutput
	ApplicationItem
}

// RenewSecret renews (rotates) the secret of an existing OAuth application
// (admin). The previous secret is invalidated, so any client using it must be
// updated with the new value returned here.
func RenewSecret(ctx context.Context, client *gitlabclient.Client, input RenewSecretInput) (RenewSecretOutput, error) {
	if input.ID <= 0 {
		return RenewSecretOutput{}, toolutil.ErrRequiredInt64("renew_application_secret", "id")
	}
	app, _, err := client.GL().Applications.RenewApplicationSecret(input.ID, gl.WithContext(ctx))
	if err != nil {
		return RenewSecretOutput{}, toolutil.WrapErrWithStatusHint("renew_application_secret", err, http.StatusNotFound, "verify application id with gitlab_list_applications — requires administrator access")
	}
	return RenewSecretOutput{ApplicationItem: toItem(app)}, nil
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
		return toolutil.WrapErrWithStatusHint("delete_application", err, http.StatusNotFound, "verify application_id with gitlab_list_applications")
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
		Scopes:          a.Scopes,
	}
}
