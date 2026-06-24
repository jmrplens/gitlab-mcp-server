package integrations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	ProjectKeys                  []string             `json:"project_keys,omitempty" jsonschema:"Jira project keys to restrict (used when issues_enabled is true)"`
	UseInheritedSettings         *bool                `json:"use_inherited_settings,omitempty" jsonschema:"Use inherited settings from group"`
	// Fields added in client-go v2.42.0.
	VulnerabilitiesEnabled      *bool  `json:"vulnerabilities_enabled,omitempty" jsonschema:"Create Jira issues for vulnerabilities"`
	VulnerabilitiesIssueType    *int64 `json:"vulnerabilities_issuetype,omitempty" jsonschema:"Jira issue type ID used for vulnerability tickets (when vulnerabilities_enabled is true)"`
	ProjectKey                  string `json:"project_key,omitempty" jsonschema:"Jira project key where vulnerability tickets are created (when vulnerabilities_enabled is true)"`
	CustomizeJiraIssueEnabled   *bool  `json:"customize_jira_issue_enabled,omitempty" jsonschema:"Customize the Jira issue created for vulnerabilities"`
	JiraCheckEnabled            *bool  `json:"jira_check_enabled,omitempty" jsonschema:"Require commits/MRs to reference a Jira issue"`
	JiraExistsCheckEnabled      *bool  `json:"jira_exists_check_enabled,omitempty" jsonschema:"Require that the referenced Jira issue exists"`
	JiraAssigneeCheckEnabled    *bool  `json:"jira_assignee_check_enabled,omitempty" jsonschema:"Require the referenced Jira issue to have an assignee"`
	JiraStatusCheckEnabled      *bool  `json:"jira_status_check_enabled,omitempty" jsonschema:"Require the referenced Jira issue to be in an allowed status"`
	JiraAllowedStatusesAsString string `json:"jira_allowed_statuses_as_string,omitempty" jsonschema:"Comma-separated list of allowed Jira statuses (used with jira_status_check_enabled)"`
}

// SetJiraOutput is the output after configuring Jira.
type SetJiraOutput struct {
	toolutil.HintableOutput
	Integration IntegrationItem `json:"integration"`
}

// SetJira configures the Jira integration for a project.
func SetJira(ctx context.Context, client *gitlabclient.Client, input SetJiraInput) (SetJiraOutput, error) {
	// Pointer fields are assigned directly: a nil input pointer leaves the
	// option unset, so a per-field nil check would be redundant.
	opts := &gl.SetJiraServiceOptions{
		URL:                          new(input.URL),
		Active:                       input.Active,
		JiraAuthType:                 input.JiraAuthType,
		JiraIssueTransitionAutomatic: input.JiraIssueTransitionAutomatic,
		CommitEvents:                 input.CommitEvents,
		MergeRequestsEvents:          input.MergeRequestsEvents,
		CommentOnEventEnabled:        input.CommentOnEventEnabled,
		IssuesEnabled:                input.IssuesEnabled,
		UseInheritedSettings:         input.UseInheritedSettings,
		VulnerabilitiesEnabled:       input.VulnerabilitiesEnabled,
		VulnerabilitiesIssueType:     input.VulnerabilitiesIssueType,
		CustomizeJiraIssueEnabled:    input.CustomizeJiraIssueEnabled,
		JiraCheckEnabled:             input.JiraCheckEnabled,
		JiraExistsCheckEnabled:       input.JiraExistsCheckEnabled,
		JiraAssigneeCheckEnabled:     input.JiraAssigneeCheckEnabled,
		JiraStatusCheckEnabled:       input.JiraStatusCheckEnabled,
	}
	// String/slice fields only set when non-empty so blank values are not sent.
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.Password != "" {
		opts.Password = new(input.Password)
	}
	if input.APIURL != "" {
		opts.APIURL = new(input.APIURL)
	}
	if input.JiraIssuePrefix != "" {
		opts.JiraIssuePrefix = new(input.JiraIssuePrefix)
	}
	if input.JiraIssueRegex != "" {
		opts.JiraIssueRegex = new(input.JiraIssueRegex)
	}
	if input.JiraIssueTransitionID != "" {
		opts.JiraIssueTransitionID = new(input.JiraIssueTransitionID)
	}
	if len(input.ProjectKeys) > 0 {
		opts.ProjectKeys = new(input.ProjectKeys)
	}
	if input.ProjectKey != "" {
		opts.ProjectKey = new(input.ProjectKey)
	}
	if input.JiraAllowedStatusesAsString != "" {
		opts.JiraAllowedStatusesAsString = new(input.JiraAllowedStatusesAsString)
	}

	svc, _, err := client.GL().Services.SetJiraService(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return SetJiraOutput{}, toolutil.WrapErrWithStatusHint("set_jira_integration", err, http.StatusNotFound, "verify project_id with gitlab_project_get and ensure url is reachable")
	}
	return SetJiraOutput{Integration: integrationToItem(&svc.Service)}, nil
}
