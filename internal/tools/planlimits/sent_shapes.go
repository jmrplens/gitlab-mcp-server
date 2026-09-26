package planlimits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// limitExtra is what lib/api/entities/plan_limit.rb sends on a plan's limits
// that client-go's PlanLimit does not carry, read from the captured response
// beside the SDK's own decode (ADR-0021). The gap is recorded in
// docs/development/upstream-bugs.md.
//
// client-go models the eight package file sizes and nothing else, while the
// entity exposes twenty-nine limits and the change history, every one with no
// condition, so both plan limit routes answer with all of them. The API page
// documents every limit but the two webhook tiers GitLab.com uses among its
// update parameters, and neither of its example bodies carries the webhook
// limits or the history.
//
// Each limit is a pointer rather than a number because absent and zero are
// different answers here: GitLab adds limits most releases (cargo in 19.3, the
// two Service Desk ones in 19.4), an older instance sends no key for one, and
// zero is a value several of these read as unlimited.
type limitExtra struct {
	CargoMaxFileSize                 *int64                   `json:"cargo_max_file_size"`
	CIInstanceLevelVariables         *int64                   `json:"ci_instance_level_variables"`
	CIPipelineSize                   *int64                   `json:"ci_pipeline_size"`
	CIActiveJobs                     *int64                   `json:"ci_active_jobs"`
	CIProjectSubscriptions           *int64                   `json:"ci_project_subscriptions"`
	CIPipelineSchedules              *int64                   `json:"ci_pipeline_schedules"`
	CINeedsSizeLimit                 *int64                   `json:"ci_needs_size_limit"`
	CIRegisteredGroupRunners         *int64                   `json:"ci_registered_group_runners"`
	CIRegisteredProjectRunners       *int64                   `json:"ci_registered_project_runners"`
	DotenvVariables                  *int64                   `json:"dotenv_variables"`
	DotenvSize                       *int64                   `json:"dotenv_size"`
	EnforcementLimit                 *int64                   `json:"enforcement_limit"`
	NotificationLimit                *int64                   `json:"notification_limit"`
	StorageSizeLimit                 *int64                   `json:"storage_size_limit"`
	PipelineHierarchySize            *int64                   `json:"pipeline_hierarchy_size"`
	MaxPipelinesPerMergeTrain        *int64                   `json:"max_pipelines_per_merge_train"`
	ServiceDeskOutboundEmailsPerHour *int64                   `json:"service_desk_outbound_emails_per_hour"`
	ServiceDeskOutboundEmailsPerDay  *int64                   `json:"service_desk_outbound_emails_per_day"`
	WebHookCalls                     *int64                   `json:"web_hook_calls"`
	WebHookCallsLow                  *int64                   `json:"web_hook_calls_low"`
	WebHookCallsMid                  *int64                   `json:"web_hook_calls_mid"`
	LimitsHistory                    map[string][]LimitChange `json:"limits_history"`
}

// capturedLimits reads the limits client-go's PlanLimit does not model off the
// answer to a request for one plan's limits.
func capturedLimits(capture *gitlabclient.ResponseCapture) (limitExtra, error) {
	var extra limitExtra
	if err := capture.Decode(&extra); err != nil {
		return limitExtra{}, err
	}
	return extra, nil
}
