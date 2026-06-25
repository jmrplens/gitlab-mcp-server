package integrations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// commonIntegrationSlugs documents the integration slugs accepted by the
// generic set/get/delete endpoints. The list is illustrative, not exhaustive:
// GitLab adds new integrations over time and the PUT endpoint accepts any
// currently-supported slug. See doc/api/integrations.md for the authoritative
// per-integration parameter reference.
const commonIntegrationSlugs = "slack, jira, discord, mattermost, microsoft-teams, telegram, " +
	"datadog, jenkins, harbor, drone-ci, github, emails-on-push, pipelines-email, " +
	"external-wiki, custom-issue-tracker, matrix, redmine, youtrack, " +
	"slack-slash-commands, mattermost-slash-commands, google-play, apple-app-store, " +
	"prometheus, bamboo, buildkite, campfire, confluence, packagist, pivotaltracker, " +
	"pumble, pushover, teamcity, unify-circuit, webex-teams, zentao, gitlab-slack-application"

// SetIntegrationInput is the input for the generic project integration upsert.
//
// Unlike the typed per-integration setters (e.g. set_jira_integration), this
// action accepts any integration slug plus a free-form config object carrying
// that integration's documented parameters. The config map is passed through
// verbatim as the PUT request body, so the model supplies exactly the fields
// listed for the chosen slug in doc/api/integrations.md.
type SetIntegrationInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Slug      string               `json:"slug" jsonschema:"Integration slug to configure (e.g. slack, harbor, jenkins, google-play). See doc/api/integrations.md for the full list and each integration's config fields,required"`
	Config    map[string]any       `json:"config,omitempty" jsonschema:"Integration configuration parameters as documented for the chosen slug in doc/api/integrations.md (sent verbatim as the request body). For example slack expects webhook; harbor expects url, project_name, username, password; jenkins expects jenkins_url, project_name, username, password"`
}

// SetIntegrationOutput is the output after upserting a project integration.
type SetIntegrationOutput struct {
	toolutil.HintableOutput
	Integration IntegrationItem `json:"integration"`
}

// SetIntegration creates or updates an arbitrary project integration by slug.
//
// It issues a raw REST PUT against projects/{id}/integrations/{slug} with the
// caller-supplied config object as the request body, then decodes the
// resulting integration into the shared IntegrationItem shape. A nil config is
// tolerated (sent as an empty body), so the handler never panics on missing
// configuration.
func SetIntegration(ctx context.Context, client *gitlabclient.Client, input SetIntegrationInput) (SetIntegrationOutput, error) {
	if input.Slug == "" {
		return SetIntegrationOutput{}, toolutil.WrapErrWithMessage("set_integration",
			toolutil.ErrFieldRequired("slug"))
	}

	path := "projects/" + gl.PathEscape(string(input.ProjectID)) + "/integrations/" + gl.PathEscape(input.Slug)
	body := integrationConfigBody(input.Config)

	req, err := client.GL().NewRequest(http.MethodPut, path, body, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return SetIntegrationOutput{}, toolutil.WrapErrWithMessage("set_integration", err)
	}
	var integration gl.Integration
	if _, err = client.GL().Do(req, &integration); err != nil {
		return SetIntegrationOutput{}, toolutil.WrapErrWithStatusHint("set_integration", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; verify slug is a supported integration (e.g. "+commonIntegrationSlugs+"); supply the integration's documented config fields (see doc/api/integrations.md); requires Maintainer role on the project")
	}
	return SetIntegrationOutput{Integration: integrationToItem(&integration)}, nil
}

// integrationConfigBody normalises a nil config map to an empty map so the
// request body is always a JSON object rather than null, keeping the GitLab
// API contract predictable.
func integrationConfigBody(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	return config
}
