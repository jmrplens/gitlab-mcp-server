// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package integrations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// SetJiraInput is the input for configuring the Jira integration.
type SetJiraInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	URL       string               `json:"url" jsonschema:"Jira instance base URL,required"`
	// The optional texts and lists are pointers or nil-able slices so the
	// empty value that removes one reaches GitLab. Integrations::Jira allows
	// a blank API URL, issue prefix, issue regex and transition ID, and
	// requires the username only under basic authentication, which it
	// answers itself (app/models/integrations/jira.rb, issue 1306).
	Username                     *string  `json:"username,omitempty" jsonschema:"Jira username. Pass an empty string to remove it, which GitLab refuses under basic authentication. Omit the field to leave it untouched"`
	Password                     string   `json:"password,omitempty" jsonschema:"Jira password or API token. GitLab requires it on every update,required"`
	Active                       *bool    `json:"active,omitempty" jsonschema:"Enable or disable the integration"`
	APIURL                       *string  `json:"api_url,omitempty" jsonschema:"Jira API URL (overrides base URL). Pass an empty string to remove it. Omit the field to leave it untouched"`
	JiraAuthType                 *int64   `json:"jira_auth_type,omitempty" jsonschema:"Jira auth type (0=basic, 1=token)"`
	JiraIssuePrefix              *string  `json:"jira_issue_prefix,omitempty" jsonschema:"Jira issue key prefix. Pass an empty string to remove it. Omit the field to leave it untouched"`
	JiraIssueRegex               *string  `json:"jira_issue_regex,omitempty" jsonschema:"Custom regex for Jira issue keys. Pass an empty string to remove it. Omit the field to leave it untouched"`
	JiraIssueTransitionAutomatic *bool    `json:"jira_issue_transition_automatic,omitempty" jsonschema:"Auto-transition Jira issues"`
	JiraIssueTransitionID        *string  `json:"jira_issue_transition_id,omitempty" jsonschema:"Jira transition ID. Pass an empty string to remove it. Omit the field to leave it untouched"`
	CommitEvents                 *bool    `json:"commit_events,omitempty" jsonschema:"Trigger on commit events"`
	MergeRequestsEvents          *bool    `json:"merge_requests_events,omitempty" jsonschema:"Trigger on merge request events"`
	CommentOnEventEnabled        *bool    `json:"comment_on_event_enabled,omitempty" jsonschema:"Add comments on Jira issues for events"`
	IssuesEnabled                *bool    `json:"issues_enabled,omitempty" jsonschema:"Enable Jira issues integration"`
	ProjectKeys                  []string `json:"project_keys,omitempty" jsonschema:"Jira project keys whose issues GitLab shows and links (used when issues_enabled is true). Pass [] to remove the restriction, so the issues of every Jira project are shown. Omit the field to leave them untouched"`
	UseInheritedSettings         *bool    `json:"use_inherited_settings,omitempty" jsonschema:"Use inherited settings from group"`
	// Fields added in client-go v2.42.0.
	VulnerabilitiesEnabled      *bool   `json:"vulnerabilities_enabled,omitempty" jsonschema:"Create Jira issues for vulnerabilities"`
	VulnerabilitiesIssueType    *int64  `json:"vulnerabilities_issuetype,omitempty" jsonschema:"Jira issue type ID used for vulnerability tickets (when vulnerabilities_enabled is true)"`
	ProjectKey                  *string `json:"project_key,omitempty" jsonschema:"Jira project key where vulnerability tickets are created (when vulnerabilities_enabled is true). Pass an empty string to remove it. Omit the field to leave it untouched"`
	CustomizeJiraIssueEnabled   *bool   `json:"customize_jira_issue_enabled,omitempty" jsonschema:"Customize the Jira issue created for vulnerabilities"`
	JiraCheckEnabled            *bool   `json:"jira_check_enabled,omitempty" jsonschema:"Require commits/MRs to reference a Jira issue"`
	JiraExistsCheckEnabled      *bool   `json:"jira_exists_check_enabled,omitempty" jsonschema:"Require that the referenced Jira issue exists"`
	JiraAssigneeCheckEnabled    *bool   `json:"jira_assignee_check_enabled,omitempty" jsonschema:"Require the referenced Jira issue to have an assignee"`
	JiraStatusCheckEnabled      *bool   `json:"jira_status_check_enabled,omitempty" jsonschema:"Require the referenced Jira issue to be in an allowed status"`
	JiraAllowedStatusesAsString *string `json:"jira_allowed_statuses_as_string,omitempty" jsonschema:"Comma-separated list of allowed Jira statuses (used with jira_status_check_enabled). Pass an empty string to remove them. Omit the field to leave them untouched"`
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
		// The optional texts decide presence by nil, never by emptiness:
		// an empty one is the caller removing it (issue 1306).
		Username:                    input.Username,
		APIURL:                      input.APIURL,
		JiraIssuePrefix:             input.JiraIssuePrefix,
		JiraIssueRegex:              input.JiraIssueRegex,
		JiraIssueTransitionID:       input.JiraIssueTransitionID,
		ProjectKey:                  input.ProjectKey,
		JiraAllowedStatusesAsString: input.JiraAllowedStatusesAsString,
	}
	if input.Password != "" {
		opts.Password = new(input.Password)
	}
	if input.ProjectKeys != nil {
		opts.ProjectKeys = new(input.ProjectKeys)
	}

	svc, _, err := client.GL().Services.SetJiraService(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return SetJiraOutput{}, toolutil.WrapErrWithStatusHint("set_jira_integration", err, http.StatusNotFound, "verify project_id with project.get and ensure url is reachable")
	}
	return SetJiraOutput{Integration: integrationToItem(&svc.Service)}, nil
}
