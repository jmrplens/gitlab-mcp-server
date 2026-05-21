package integrations

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// IntegrationItem is a summary of an integration/service.
type IntegrationItem struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type integrationGetter func(context.Context, gl.ServicesServiceInterface, string) (*gl.Integration, error)

var integrationGetters = map[string]integrationGetter{
	"jira": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetJiraService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"slack": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetSlackService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"discord": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetDiscordService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"mattermost": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetMattermostService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"microsoft-teams": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetMicrosoftTeamsService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"telegram": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetTelegramService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"datadog": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetDataDogService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"jenkins": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetJenkinsCIService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"emails-on-push": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetEmailsOnPushService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"pipelines-email": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetPipelinesEmailService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"external-wiki": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetExternalWikiService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"custom-issue-tracker": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetCustomIssueTrackerService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"drone-ci": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetDroneCIService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"github": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetGithubService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"harbor": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetHarborService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"matrix": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetMatrixService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"redmine": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetRedmineService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"youtrack": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetYouTrackService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"slack-slash-commands": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetSlackSlashCommandsService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
	"mattermost-slash-commands": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) (*gl.Integration, error) {
		service, _, err := services.GetMattermostSlashCommandsService(projectID, gl.WithContext(ctx))
		return integrationFromService(service, err)
	},
}

type integrationDeleter func(context.Context, gl.ServicesServiceInterface, string) error

var integrationDeleters = map[string]integrationDeleter{
	"jira": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteJiraService(projectID, gl.WithContext(ctx))
		return err
	},
	"slack": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteSlackService(projectID, gl.WithContext(ctx))
		return err
	},
	"discord": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteDiscordService(projectID, gl.WithContext(ctx))
		return err
	},
	"mattermost": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteMattermostService(projectID, gl.WithContext(ctx))
		return err
	},
	"microsoft-teams": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteMicrosoftTeamsService(projectID, gl.WithContext(ctx))
		return err
	},
	"telegram": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteTelegramService(projectID, gl.WithContext(ctx))
		return err
	},
	"datadog": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteDataDogService(projectID, gl.WithContext(ctx))
		return err
	},
	"jenkins": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteJenkinsCIService(projectID, gl.WithContext(ctx))
		return err
	},
	"emails-on-push": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteEmailsOnPushService(projectID, gl.WithContext(ctx))
		return err
	},
	"pipelines-email": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeletePipelinesEmailService(projectID, gl.WithContext(ctx))
		return err
	},
	"external-wiki": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteExternalWikiService(projectID, gl.WithContext(ctx))
		return err
	},
	"custom-issue-tracker": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteCustomIssueTrackerService(projectID, gl.WithContext(ctx))
		return err
	},
	"drone-ci": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteDroneCIService(projectID, gl.WithContext(ctx))
		return err
	},
	"github": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteGithubService(projectID, gl.WithContext(ctx))
		return err
	},
	"harbor": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteHarborService(projectID, gl.WithContext(ctx))
		return err
	},
	"matrix": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteMatrixService(projectID, gl.WithContext(ctx))
		return err
	},
	"redmine": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteRedmineService(projectID, gl.WithContext(ctx))
		return err
	},
	"youtrack": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteYouTrackService(projectID, gl.WithContext(ctx))
		return err
	},
	"slack-slash-commands": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteSlackSlashCommandsService(projectID, gl.WithContext(ctx))
		return err
	},
	"mattermost-slash-commands": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DeleteMattermostSlashCommandsService(projectID, gl.WithContext(ctx))
		return err
	},
	"slack-application": func(ctx context.Context, services gl.ServicesServiceInterface, projectID string) error {
		_, err := services.DisableSlackApplication(projectID, gl.WithContext(ctx))
		return err
	},
}

func integrationFromService(service any, err error) (*gl.Integration, error) {
	if service == nil {
		return nil, err
	}
	value := reflect.ValueOf(service)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, err
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	field := value.FieldByName("Service")
	if !field.IsValid() || !field.CanAddr() {
		return nil, err
	}
	integration, ok := field.Addr().Interface().(*gl.Integration)
	if !ok {
		return nil, err
	}
	return integration, err
}

// integrationToItem maps integration to item between API and evaluator models.
func integrationToItem(s *gl.Integration) IntegrationItem {
	item := IntegrationItem{
		ID:     s.ID,
		Title:  s.Title,
		Slug:   s.Slug,
		Active: s.Active,
	}
	if s.CreatedAt != nil {
		item.CreatedAt = s.CreatedAt.String()
	}
	if s.UpdatedAt != nil {
		item.UpdatedAt = s.UpdatedAt.String()
	}
	return item
}

// List.

// ListInput is the input for listing project integrations.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ListOutput is the output for listing integrations.
type ListOutput struct {
	toolutil.HintableOutput
	Integrations []IntegrationItem `json:"integrations"`
}

// List returns all integrations for a project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	services, _, err := client.GL().Services.ListServices(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_integrations", err, http.StatusForbidden,
			"requires Maintainer role on the project; verify project_id with gitlab_project_list; lists active integrations only")
	}
	items := make([]IntegrationItem, 0, len(services))
	for _, s := range services {
		items = append(items, integrationToItem(s))
	}
	return ListOutput{Integrations: items}, nil
}

// Get (by slug).

// GetInput is the input for getting an integration by slug.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Slug      string               `json:"slug" jsonschema:"Integration slug (e.g. jira, slack, discord, mattermost, microsoft-teams, telegram, emails-on-push, pipelines-email, external-wiki, custom-issue-tracker, datadog, drone-ci, github, harbor, jenkins, matrix, redmine, youtrack, slack-slash-commands, mattermost-slash-commands),required"`
}

// GetOutput is the output for a single integration.
type GetOutput struct {
	toolutil.HintableOutput
	Integration IntegrationItem `json:"integration"`
}

// Get retrieves a specific integration by slug, dispatching to the typed client-go method.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	getter, ok := integrationGetters[input.Slug]
	if !ok {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_integration", fmt.Errorf("unsupported integration slug: %s", input.Slug))
	}
	result, err := getter(ctx, client.GL().Services, string(input.ProjectID))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_integration", err, http.StatusNotFound,
			"verify slug is a valid integration name (e.g. slack, jira, microsoft-teams, jenkins); integration must be active on the project; use gitlab_list_integrations to enumerate enabled integrations")
	}
	if result == nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_integration", fmt.Errorf("integration %s returned nil", input.Slug))
	}
	return GetOutput{Integration: integrationToItem(result)}, nil
}

// Delete (by slug).

// DeleteInput is the input for deleting/disabling an integration.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Slug      string               `json:"slug" jsonschema:"Integration slug (e.g. jira, slack, discord, mattermost, microsoft-teams, telegram, emails-on-push, pipelines-email, external-wiki, custom-issue-tracker, datadog, drone-ci, github, harbor, jenkins, matrix, redmine, youtrack, slack-slash-commands, mattermost-slash-commands),required"`
}

// Delete removes/disables a specific integration from a project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	deleter, ok := integrationDeleters[input.Slug]
	if !ok {
		return toolutil.WrapErrWithMessage("delete_integration", fmt.Errorf("unsupported integration slug: %s", input.Slug))
	}
	err := deleter(ctx, client.GL().Services, string(input.ProjectID))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_integration", err, http.StatusForbidden,
			"requires Maintainer role; deactivates the integration on the project; verify slug with gitlab_list_integrations; deletion is irreversible (configuration is removed)")
	}
	return nil
}

// Markdown Formatters.
