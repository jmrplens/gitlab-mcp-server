// Package integrations implements MCP tool handlers for GitLab project
// integrations (services). It wraps the ServicesService from client-go v2.
//
// The generic List method returns all integrations. Get and Delete dispatch
// to the integration-specific client-go methods based on the slug parameter.
package integrations

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

// integrationToItem is an internal helper for the integrations package.
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
	var result *gl.Integration
	var err error

	switch input.Slug {
	case "jira":
		var s *gl.JiraService
		s, _, err = client.GL().Services.GetJiraService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "slack":
		var s *gl.SlackService
		s, _, err = client.GL().Services.GetSlackService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "discord":
		var s *gl.DiscordService
		s, _, err = client.GL().Services.GetDiscordService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "mattermost":
		var s *gl.MattermostService
		s, _, err = client.GL().Services.GetMattermostService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "microsoft-teams":
		var s *gl.MicrosoftTeamsService
		s, _, err = client.GL().Services.GetMicrosoftTeamsService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "telegram":
		var s *gl.TelegramService
		s, _, err = client.GL().Services.GetTelegramService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "datadog":
		var s *gl.DataDogService
		s, _, err = client.GL().Services.GetDataDogService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "jenkins":
		var s *gl.JenkinsCIService
		s, _, err = client.GL().Services.GetJenkinsCIService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "emails-on-push":
		var s *gl.EmailsOnPushService
		s, _, err = client.GL().Services.GetEmailsOnPushService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "pipelines-email":
		var s *gl.PipelinesEmailService
		s, _, err = client.GL().Services.GetPipelinesEmailService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "external-wiki":
		var s *gl.ExternalWikiService
		s, _, err = client.GL().Services.GetExternalWikiService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "custom-issue-tracker":
		var s *gl.CustomIssueTrackerService
		s, _, err = client.GL().Services.GetCustomIssueTrackerService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "drone-ci":
		var s *gl.DroneCIService
		s, _, err = client.GL().Services.GetDroneCIService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "github":
		var s *gl.GithubService
		s, _, err = client.GL().Services.GetGithubService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "harbor":
		var s *gl.HarborService
		s, _, err = client.GL().Services.GetHarborService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "matrix":
		var s *gl.MatrixService
		s, _, err = client.GL().Services.GetMatrixService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "redmine":
		var s *gl.RedmineService
		s, _, err = client.GL().Services.GetRedmineService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "youtrack":
		var s *gl.YouTrackService
		s, _, err = client.GL().Services.GetYouTrackService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "slack-slash-commands":
		var s *gl.SlackSlashCommandsService
		s, _, err = client.GL().Services.GetSlackSlashCommandsService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	case "mattermost-slash-commands":
		var s *gl.MattermostSlashCommandsService
		s, _, err = client.GL().Services.GetMattermostSlashCommandsService(string(input.ProjectID), gl.WithContext(ctx))
		if s != nil {
			result = &s.Service
		}
	default:
		return GetOutput{}, toolutil.WrapErrWithMessage("get_integration", fmt.Errorf("unsupported integration slug: %s", input.Slug))
	}

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
	svc := client.GL().Services
	var err error

	switch input.Slug {
	case "jira":
		_, err = svc.DeleteJiraService(string(input.ProjectID), gl.WithContext(ctx))
	case "slack":
		_, err = svc.DeleteSlackService(string(input.ProjectID), gl.WithContext(ctx))
	case "discord":
		_, err = svc.DeleteDiscordService(string(input.ProjectID), gl.WithContext(ctx))
	case "mattermost":
		_, err = svc.DeleteMattermostService(string(input.ProjectID), gl.WithContext(ctx))
	case "microsoft-teams":
		_, err = svc.DeleteMicrosoftTeamsService(string(input.ProjectID), gl.WithContext(ctx))
	case "telegram":
		_, err = svc.DeleteTelegramService(string(input.ProjectID), gl.WithContext(ctx))
	case "datadog":
		_, err = svc.DeleteDataDogService(string(input.ProjectID), gl.WithContext(ctx))
	case "jenkins":
		_, err = svc.DeleteJenkinsCIService(string(input.ProjectID), gl.WithContext(ctx))
	case "emails-on-push":
		_, err = svc.DeleteEmailsOnPushService(string(input.ProjectID), gl.WithContext(ctx))
	case "pipelines-email":
		_, err = svc.DeletePipelinesEmailService(string(input.ProjectID), gl.WithContext(ctx))
	case "external-wiki":
		_, err = svc.DeleteExternalWikiService(string(input.ProjectID), gl.WithContext(ctx))
	case "custom-issue-tracker":
		_, err = svc.DeleteCustomIssueTrackerService(string(input.ProjectID), gl.WithContext(ctx))
	case "drone-ci":
		_, err = svc.DeleteDroneCIService(string(input.ProjectID), gl.WithContext(ctx))
	case "github":
		_, err = svc.DeleteGithubService(string(input.ProjectID), gl.WithContext(ctx))
	case "harbor":
		_, err = svc.DeleteHarborService(string(input.ProjectID), gl.WithContext(ctx))
	case "matrix":
		_, err = svc.DeleteMatrixService(string(input.ProjectID), gl.WithContext(ctx))
	case "redmine":
		_, err = svc.DeleteRedmineService(string(input.ProjectID), gl.WithContext(ctx))
	case "youtrack":
		_, err = svc.DeleteYouTrackService(string(input.ProjectID), gl.WithContext(ctx))
	case "slack-slash-commands":
		_, err = svc.DeleteSlackSlashCommandsService(string(input.ProjectID), gl.WithContext(ctx))
	case "mattermost-slash-commands":
		_, err = svc.DeleteMattermostSlashCommandsService(string(input.ProjectID), gl.WithContext(ctx))
	case "slack-application":
		_, err = svc.DisableSlackApplication(string(input.ProjectID), gl.WithContext(ctx))
	default:
		return toolutil.WrapErrWithMessage("delete_integration", fmt.Errorf("unsupported integration slug: %s", input.Slug))
	}

	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_integration", err, http.StatusForbidden,
			"requires Maintainer role; deactivates the integration on the project; verify slug with gitlab_list_integrations; deletion is irreversible (configuration is removed)")
	}
	return nil
}

// Markdown Formatters.
