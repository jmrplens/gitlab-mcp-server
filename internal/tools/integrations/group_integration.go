package integrations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Group integration generic tools.
//
// GitLab's group integration API is uniform with the project one:
//
//   - GET    /groups/:id/integrations        (list active integrations)
//   - GET    /groups/:id/integrations/:slug  (read one)
//   - PUT    /groups/:id/integrations/:slug  (create/update with config body)
//   - DELETE /groups/:id/integrations/:slug  (disable/remove)
//
// These tools require GitLab Premium/Ultimate (self-managed EE or GitLab.com)
// for most slugs and Owner role on the group, mirroring the existing
// group-level Datadog tools.

// ListGroupIntegrations (read).

// ListGroupIntegrationsInput is the input for listing a group's active integrations.
type ListGroupIntegrationsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// ListGroupIntegrationsOutput is the output for listing group integrations.
type ListGroupIntegrationsOutput struct {
	toolutil.HintableOutput
	Integrations []IntegrationItem `json:"integrations"`
}

// ListGroupIntegrations returns all active integrations configured on a group.
func ListGroupIntegrations(ctx context.Context, client *gitlabclient.Client, input ListGroupIntegrationsInput) (ListGroupIntegrationsOutput, error) {
	integrations, _, err := client.GL().Integrations.ListActiveGroupIntegrations(string(input.GroupID), nil, gl.WithContext(ctx))
	if err != nil {
		return ListGroupIntegrationsOutput{}, toolutil.WrapErrWithStatusHint("list_group_integrations", err, http.StatusForbidden,
			"requires Owner role on the group; verify group_id with gitlab_group_get; lists active integrations only")
	}
	items := make([]IntegrationItem, 0, len(integrations))
	for _, s := range integrations {
		if s == nil {
			continue
		}
		items = append(items, integrationToItem(s))
	}
	return ListGroupIntegrationsOutput{Integrations: items}, nil
}

// GetGroupIntegration (read by slug).

// GetGroupIntegrationInput is the input for reading one group integration by slug.
type GetGroupIntegrationInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Slug    string               `json:"slug" jsonschema:"Integration slug to read (e.g. slack, jira, harbor). See doc/api/group_integrations.md for the supported list,required"`
}

// GetGroupIntegrationOutput is the output for a single group integration.
type GetGroupIntegrationOutput struct {
	toolutil.HintableOutput
	Integration IntegrationItem `json:"integration"`
}

// GetGroupIntegration reads one group integration by slug via raw REST.
func GetGroupIntegration(ctx context.Context, client *gitlabclient.Client, input GetGroupIntegrationInput) (GetGroupIntegrationOutput, error) {
	if input.Slug == "" {
		return GetGroupIntegrationOutput{}, toolutil.WrapErrWithMessage("get_group_integration", toolutil.ErrFieldRequired("slug"))
	}
	path := "groups/" + gl.PathEscape(string(input.GroupID)) + "/integrations/" + gl.PathEscape(input.Slug)
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return GetGroupIntegrationOutput{}, toolutil.WrapErrWithMessage("get_group_integration", err)
	}
	var integration gl.Integration
	if _, err = client.GL().Do(req, &integration); err != nil {
		return GetGroupIntegrationOutput{}, toolutil.WrapErrWithStatusHint("get_group_integration", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; the integration must be active on the group; use gitlab_list_group_integrations to enumerate enabled integrations; requires Owner role")
	}
	return GetGroupIntegrationOutput{Integration: integrationToItem(&integration)}, nil
}

// SetGroupIntegration (mutate).

// SetGroupIntegrationInput is the input for the generic group integration upsert.
type SetGroupIntegrationInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Slug    string               `json:"slug" jsonschema:"Integration slug to configure (e.g. slack, jira, harbor). See doc/api/group_integrations.md for the supported list and each integration's config fields,required"`
	Config  map[string]any       `json:"config,omitempty" jsonschema:"Integration configuration parameters as documented for the chosen slug in doc/api/integrations.md (sent verbatim as the request body)"`
}

// SetGroupIntegrationOutput is the output after upserting a group integration.
type SetGroupIntegrationOutput struct {
	toolutil.HintableOutput
	Integration IntegrationItem `json:"integration"`
}

// SetGroupIntegration creates or updates an arbitrary group integration by slug
// via raw REST PUT, passing the caller-supplied config object through as the
// request body.
func SetGroupIntegration(ctx context.Context, client *gitlabclient.Client, input SetGroupIntegrationInput) (SetGroupIntegrationOutput, error) {
	if input.Slug == "" {
		return SetGroupIntegrationOutput{}, toolutil.WrapErrWithMessage("set_group_integration", toolutil.ErrFieldRequired("slug"))
	}
	path := "groups/" + gl.PathEscape(string(input.GroupID)) + "/integrations/" + gl.PathEscape(input.Slug)
	body := integrationConfigBody(input.Config)

	req, err := client.GL().NewRequest(http.MethodPut, path, body, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return SetGroupIntegrationOutput{}, toolutil.WrapErrWithMessage("set_group_integration", err)
	}
	var integration gl.Integration
	if _, err = client.GL().Do(req, &integration); err != nil {
		return SetGroupIntegrationOutput{}, toolutil.WrapErrWithStatusHint("set_group_integration", err, http.StatusForbidden,
			"requires Owner role on the group; verify group_id with gitlab_group_get; verify slug is supported (see doc/api/group_integrations.md); supply the integration's documented config fields; some integrations require GitLab Premium/Ultimate")
	}
	return SetGroupIntegrationOutput{Integration: integrationToItem(&integration)}, nil
}

// DeleteGroupIntegration (destructive).

// DeleteGroupIntegrationInput is the input for disabling a group integration by slug.
type DeleteGroupIntegrationInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Slug    string               `json:"slug" jsonschema:"Integration slug to disable (e.g. slack, jira, harbor),required"`
}

// DeleteGroupIntegration disables/removes a group integration by slug via raw REST DELETE.
func DeleteGroupIntegration(ctx context.Context, client *gitlabclient.Client, input DeleteGroupIntegrationInput) error {
	if input.Slug == "" {
		return toolutil.WrapErrWithMessage("delete_group_integration", toolutil.ErrFieldRequired("slug"))
	}
	path := "groups/" + gl.PathEscape(string(input.GroupID)) + "/integrations/" + gl.PathEscape(input.Slug)
	req, err := client.GL().NewRequest(http.MethodDelete, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return toolutil.WrapErrWithMessage("delete_group_integration", err)
	}
	if _, err = client.GL().Do(req, nil); err != nil {
		return toolutil.WrapErrWithStatusHint("delete_group_integration", err, http.StatusForbidden,
			"requires Owner role on the group; verify slug with gitlab_list_group_integrations; deactivates the integration (configuration is removed)")
	}
	return nil
}
