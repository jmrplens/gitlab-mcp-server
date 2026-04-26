// set_jira.go implements the Jira integration configuration handler.

package integrations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// SetJiraInput is the input for configuring the Jira integration.
type SetJiraInput struct {
	ProjectID                    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	URL                          string               `json:"url" jsonschema:"Jira instance base URL,required"`
	Username                     string               `json:"username,omitempty" jsonschema:"Jira username"`
	Password                     string               `json:"password,omitempty" jsonschema:"Jira password or API token"`
	Active                       *bool                `json:"active,omitempty" jsonschema:"Enable or disable the integration"`
	APIURL                       string               `json:"api_url,omitempty" jsonschema:"Jira API URL (overrides base URL)"`
	JiraAuthType                 *int64               `json:"jira_auth_type,omitempty" jsonschema:"Jira auth type (0=basic, 1=token)"`
	JiraIssuePrefix              string               `json:"jira_issue_prefix,omitempty" jsonschema:"Jira issue key prefix"`
	JiraIssueRegex               string               `json:"jira_issue_regex,omitempty" jsonschema:"Custom regex for Jira issue keys"`
	JiraIssueTransitionAutomatic *bool                `json:"jira_issue_transition_automatic,omitempty" jsonschema:"Auto-transition Jira issues"`
	JiraIssueTransitionID        string               `json:"jira_issue_transition_id,omitempty" jsonschema:"Jira transition ID"`
	CommitEvents                 *bool                `json:"commit_events,omitempty" jsonschema:"Trigger on commit events"`
	MergeRequestsEvents          *bool                `json:"merge_requests_events,omitempty" jsonschema:"Trigger on merge request events"`
	CommentOnEventEnabled        *bool                `json:"comment_on_event_enabled,omitempty" jsonschema:"Add comments on Jira issues for events"`
	IssuesEnabled                *bool                `json:"issues_enabled,omitempty" jsonschema:"Enable Jira issues integration"`
	ProjectKeys                  []string             `json:"project_keys,omitempty" jsonschema:"Jira project keys to restrict"`
	UseInheritedSettings         *bool                `json:"use_inherited_settings,omitempty" jsonschema:"Use inherited settings from group"`
}

// SetJiraOutput is the output after configuring Jira.
type SetJiraOutput struct {
	toolutil.HintableOutput
	Integration IntegrationItem `json:"integration"`
}

// SetJira configures the Jira integration for a project.
func SetJira(ctx context.Context, client *gitlabclient.Client, input SetJiraInput) (SetJiraOutput, error) {
	opts := &gl.SetJiraServiceOptions{
		URL: new(input.URL),
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.Password != "" {
		opts.Password = new(input.Password)
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.APIURL != "" {
		opts.APIURL = new(input.APIURL)
	}
	if input.JiraAuthType != nil {
		opts.JiraAuthType = input.JiraAuthType
	}
	if input.JiraIssuePrefix != "" {
		opts.JiraIssuePrefix = new(input.JiraIssuePrefix)
	}
	if input.JiraIssueRegex != "" {
		opts.JiraIssueRegex = new(input.JiraIssueRegex)
	}
	if input.JiraIssueTransitionAutomatic != nil {
		opts.JiraIssueTransitionAutomatic = input.JiraIssueTransitionAutomatic
	}
	if input.JiraIssueTransitionID != "" {
		opts.JiraIssueTransitionID = new(input.JiraIssueTransitionID)
	}
	if input.CommitEvents != nil {
		opts.CommitEvents = input.CommitEvents
	}
	if input.MergeRequestsEvents != nil {
		opts.MergeRequestsEvents = input.MergeRequestsEvents
	}
	if input.CommentOnEventEnabled != nil {
		opts.CommentOnEventEnabled = input.CommentOnEventEnabled
	}
	if input.IssuesEnabled != nil {
		opts.IssuesEnabled = input.IssuesEnabled
	}
	if len(input.ProjectKeys) > 0 {
		opts.ProjectKeys = new(input.ProjectKeys)
	}
	if input.UseInheritedSettings != nil {
		opts.UseInheritedSettings = input.UseInheritedSettings
	}

	svc, _, err := client.GL().Services.SetJiraService(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return SetJiraOutput{}, toolutil.WrapErrWithStatusHint("set_jira_integration", err, http.StatusNotFound, "verify project_id with gitlab_project_get and ensure jira_url is reachable")
	}
	return SetJiraOutput{Integration: integrationToItem(&svc.Service)}, nil
}
