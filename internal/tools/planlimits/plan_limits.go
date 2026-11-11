// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package planlimits

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// PlanLimitItem represents GitLab plan limits: every limit
// lib/api/entities/plan_limit.rb exposes, and the change history, which the
// entity exposes with no condition, so both plan limit routes answer with all
// of them. client-go's PlanLimit models each of them from v3.35.0, as a number
// that is not a pointer, so an instance older than a limit, which sends no key
// for it, reads it as 0.
type PlanLimitItem struct {
	ConanMaxFileSize                 int64         `json:"conan_max_file_size"`
	GenericPackagesMaxFileSize       int64         `json:"generic_packages_max_file_size"`
	HelmMaxFileSize                  int64         `json:"helm_max_file_size"`
	MavenMaxFileSize                 int64         `json:"maven_max_file_size"`
	NPMMaxFileSize                   int64         `json:"npm_max_file_size"`
	NugetMaxFileSize                 int64         `json:"nuget_max_file_size"`
	PyPiMaxFileSize                  int64         `json:"pypi_max_file_size"`
	TerraformModuleMaxFileSize       int64         `json:"terraform_module_max_file_size"`
	CargoMaxFileSize                 int64         `json:"cargo_max_file_size"`
	CIInstanceLevelVariables         int64         `json:"ci_instance_level_variables"`
	CIPipelineSize                   int64         `json:"ci_pipeline_size"`
	CIActiveJobs                     int64         `json:"ci_active_jobs"`
	CIProjectSubscriptions           int64         `json:"ci_project_subscriptions"`
	CIPipelineSchedules              int64         `json:"ci_pipeline_schedules"`
	CINeedsSizeLimit                 int64         `json:"ci_needs_size_limit"`
	CIRegisteredGroupRunners         int64         `json:"ci_registered_group_runners"`
	CIRegisteredProjectRunners       int64         `json:"ci_registered_project_runners"`
	DotenvVariables                  int64         `json:"dotenv_variables"`
	DotenvSize                       int64         `json:"dotenv_size"`
	EnforcementLimit                 int64         `json:"enforcement_limit"`
	NotificationLimit                int64         `json:"notification_limit"`
	StorageSizeLimit                 int64         `json:"storage_size_limit"`
	PipelineHierarchySize            int64         `json:"pipeline_hierarchy_size"`
	MaxPipelinesPerMergeTrain        int64         `json:"max_pipelines_per_merge_train"`
	ServiceDeskOutboundEmailsPerHour int64         `json:"service_desk_outbound_emails_per_hour"`
	ServiceDeskOutboundEmailsPerDay  int64         `json:"service_desk_outbound_emails_per_day"`
	WebHookCalls                     int64         `json:"web_hook_calls"`
	WebHookCallsLow                  int64         `json:"web_hook_calls_low"`
	WebHookCallsMid                  int64         `json:"web_hook_calls_mid"`
	LimitsHistory                    LimitsHistory `json:"limits_history,omitzero"`
}

// LimitsHistory is the change history GitLab keeps for four of a plan's
// limits, each oldest first: the three namespace storage limits, and the date
// the storage dashboard limit was enabled. It mirrors client-go's
// PlanLimitsHistory, and a limit with no recorded change is left out.
type LimitsHistory struct {
	EnforcementLimit        []LimitChange `json:"enforcement_limit,omitempty"`
	NotificationLimit       []LimitChange `json:"notification_limit,omitempty"`
	StorageSizeLimit        []LimitChange `json:"storage_size_limit,omitempty"`
	DashboardLimitEnabledAt []LimitChange `json:"dashboard_limit_enabled_at,omitempty"`
}

// LimitChange is one entry of a limit's change history: who set it to which
// value, and when, as a Unix timestamp in seconds. It mirrors client-go's
// PlanLimitHistoryEntry.
type LimitChange struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Timestamp int64  `json:"timestamp"`
	Value     int64  `json:"value"`
}

// ---------------------------------------------------------------------------
// GetCurrentPlanLimits
// ---------------------------------------------------------------------------.

// GetInput is the input for getting current plan limits.
type GetInput struct {
	PlanName string `json:"plan_name,omitempty" jsonschema:"Plan name to filter (e.g. default, free, bronze, silver, gold, premium, ultimate)"`
}

// GetOutput is the output for getting current plan limits.
type GetOutput struct {
	toolutil.HintableOutput
	PlanLimitItem
}

// Get retrieves current plan limits.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	opts := &gl.GetCurrentPlanLimitsOptions{}
	if input.PlanName != "" {
		opts.PlanName = new(input.PlanName)
	}

	limits, _, err := client.GL().PlanLimits.GetCurrentPlanLimits(opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_plan_limits", err, http.StatusForbidden, "plan limits require administrator access")
	}
	return GetOutput{
		PlanLimitItem: convertPlanLimit(limits),
	}, nil
}

// ---------------------------------------------------------------------------
// ChangePlanLimits
// ---------------------------------------------------------------------------.

// ChangeInput is the input for changing plan limits: every limit
// PUT /application/plan_limits takes, each a pointer so that 0, which GitLab
// stores as a limit of its own and reads as unlimited for the web hook and
// Service Desk limits, is sent when the caller gives it. client-go's
// ChangePlanLimitOptions models the twenty-one beside the package file sizes
// from v3.35.0.
type ChangeInput struct {
	PlanName                         string `json:"plan_name" jsonschema:"Plan name to update (e.g. default, free, bronze, silver, gold, premium, ultimate),required"`
	ConanMaxFileSize                 *int64 `json:"conan_max_file_size,omitempty" jsonschema:"Maximum Conan package file size in bytes"`
	GenericPackagesMaxFileSize       *int64 `json:"generic_packages_max_file_size,omitempty" jsonschema:"Maximum generic package file size in bytes"`
	HelmMaxFileSize                  *int64 `json:"helm_max_file_size,omitempty" jsonschema:"Maximum Helm chart file size in bytes"`
	MavenMaxFileSize                 *int64 `json:"maven_max_file_size,omitempty" jsonschema:"Maximum Maven package file size in bytes"`
	NPMMaxFileSize                   *int64 `json:"npm_max_file_size,omitempty" jsonschema:"Maximum NPM package file size in bytes"`
	NugetMaxFileSize                 *int64 `json:"nuget_max_file_size,omitempty" jsonschema:"Maximum NuGet package file size in bytes"`
	PyPiMaxFileSize                  *int64 `json:"pypi_max_file_size,omitempty" jsonschema:"Maximum PyPI package file size in bytes"`
	TerraformModuleMaxFileSize       *int64 `json:"terraform_module_max_file_size,omitempty" jsonschema:"Maximum Terraform module file size in bytes"`
	CargoMaxFileSize                 *int64 `json:"cargo_max_file_size,omitempty" jsonschema:"Maximum Cargo package file size in bytes"`
	CIActiveJobs                     *int64 `json:"ci_active_jobs,omitempty" jsonschema:"Total number of jobs in currently active pipelines"`
	CIInstanceLevelVariables         *int64 `json:"ci_instance_level_variables,omitempty" jsonschema:"Maximum number of instance-level CI/CD variables that can be defined"`
	CINeedsSizeLimit                 *int64 `json:"ci_needs_size_limit,omitempty" jsonschema:"Maximum number of needs dependencies a job can have"`
	CIPipelineSchedules              *int64 `json:"ci_pipeline_schedules,omitempty" jsonschema:"Maximum number of pipeline schedules"`
	CIPipelineSize                   *int64 `json:"ci_pipeline_size,omitempty" jsonschema:"Maximum number of jobs in a single pipeline"`
	CIProjectSubscriptions           *int64 `json:"ci_project_subscriptions,omitempty" jsonschema:"Maximum number of pipeline subscriptions to and from a project"`
	CIRegisteredGroupRunners         *int64 `json:"ci_registered_group_runners,omitempty" jsonschema:"Maximum number of runners created or active in a group during the past seven days"`
	CIRegisteredProjectRunners       *int64 `json:"ci_registered_project_runners,omitempty" jsonschema:"Maximum number of runners created or active in a project during the past seven days"`
	DotenvSize                       *int64 `json:"dotenv_size,omitempty" jsonschema:"Maximum size of a dotenv artifact in bytes"`
	DotenvVariables                  *int64 `json:"dotenv_variables,omitempty" jsonschema:"Maximum number of variables in a dotenv artifact"`
	EnforcementLimit                 *int64 `json:"enforcement_limit,omitempty" jsonschema:"Maximum storage size for the root namespace enforcement, in MiB"`
	MaxPipelinesPerMergeTrain        *int64 `json:"max_pipelines_per_merge_train,omitempty" jsonschema:"Maximum number of parallel pipelines per merge train"`
	NotificationLimit                *int64 `json:"notification_limit,omitempty" jsonschema:"Maximum storage size for the root namespace notifications, in MiB"`
	PipelineHierarchySize            *int64 `json:"pipeline_hierarchy_size,omitempty" jsonschema:"Maximum number of downstream pipelines in a pipeline's hierarchy tree"`
	ServiceDeskOutboundEmailsPerDay  *int64 `json:"service_desk_outbound_emails_per_day,omitempty" jsonschema:"Maximum number of outbound Service Desk emails per day, per top-level namespace. 0 for unlimited"`
	ServiceDeskOutboundEmailsPerHour *int64 `json:"service_desk_outbound_emails_per_hour,omitempty" jsonschema:"Maximum number of outbound Service Desk emails per hour, per top-level namespace. 0 for unlimited"`
	StorageSizeLimit                 *int64 `json:"storage_size_limit,omitempty" jsonschema:"Maximum storage size for the root namespace, in MiB"`
	WebHookCalls                     *int64 `json:"web_hook_calls,omitempty" jsonschema:"Maximum number of times a webhook can be called per minute, per top-level namespace. 0 for unlimited"`
	WebHookCallsLow                  *int64 `json:"web_hook_calls_low,omitempty" jsonschema:"Maximum number of times a webhook can be called per minute, per top-level namespace (the low limit, GitLab.com only). 0 for unlimited"`
	WebHookCallsMid                  *int64 `json:"web_hook_calls_mid,omitempty" jsonschema:"Maximum number of times a webhook can be called per minute, per top-level namespace (the mid limit, GitLab.com only). 0 for unlimited"`
}

// ChangeOutput is the output for changing plan limits.
type ChangeOutput struct {
	toolutil.HintableOutput
	PlanLimitItem
}

// Change modifies plan limits.
func Change(ctx context.Context, client *gitlabclient.Client, input ChangeInput) (ChangeOutput, error) {
	opts := &gl.ChangePlanLimitOptions{
		PlanName:                         new(input.PlanName),
		ConanMaxFileSize:                 input.ConanMaxFileSize,
		GenericPackagesMaxFileSize:       input.GenericPackagesMaxFileSize,
		HelmMaxFileSize:                  input.HelmMaxFileSize,
		MavenMaxFileSize:                 input.MavenMaxFileSize,
		NPMMaxFileSize:                   input.NPMMaxFileSize,
		NugetMaxFileSize:                 input.NugetMaxFileSize,
		PyPiMaxFileSize:                  input.PyPiMaxFileSize,
		TerraformModuleMaxFileSize:       input.TerraformModuleMaxFileSize,
		CargoMaxFileSize:                 input.CargoMaxFileSize,
		CIActiveJobs:                     input.CIActiveJobs,
		CIInstanceLevelVariables:         input.CIInstanceLevelVariables,
		CINeedsSizeLimit:                 input.CINeedsSizeLimit,
		CIPipelineSchedules:              input.CIPipelineSchedules,
		CIPipelineSize:                   input.CIPipelineSize,
		CIProjectSubscriptions:           input.CIProjectSubscriptions,
		CIRegisteredGroupRunners:         input.CIRegisteredGroupRunners,
		CIRegisteredProjectRunners:       input.CIRegisteredProjectRunners,
		DotenvSize:                       input.DotenvSize,
		DotenvVariables:                  input.DotenvVariables,
		EnforcementLimit:                 input.EnforcementLimit,
		MaxPipelinesPerMergeTrain:        input.MaxPipelinesPerMergeTrain,
		NotificationLimit:                input.NotificationLimit,
		PipelineHierarchySize:            input.PipelineHierarchySize,
		ServiceDeskOutboundEmailsPerDay:  input.ServiceDeskOutboundEmailsPerDay,
		ServiceDeskOutboundEmailsPerHour: input.ServiceDeskOutboundEmailsPerHour,
		StorageSizeLimit:                 input.StorageSizeLimit,
		WebHookCalls:                     input.WebHookCalls,
		WebHookCallsLow:                  input.WebHookCallsLow,
		WebHookCallsMid:                  input.WebHookCallsMid,
	}

	limits, _, err := client.GL().PlanLimits.ChangePlanLimits(opts, gl.WithContext(ctx))
	if err != nil {
		return ChangeOutput{}, toolutil.WrapErrWithStatusHint("change_plan_limits", err, http.StatusForbidden, "changing plan limits requires administrator access")
	}
	return ChangeOutput{
		PlanLimitItem: convertPlanLimit(limits),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// convertPlanLimit maps GitLab plan limit settings into MCP output.
func convertPlanLimit(l *gl.PlanLimit) PlanLimitItem {
	return PlanLimitItem{
		ConanMaxFileSize:                 l.ConanMaxFileSize,
		GenericPackagesMaxFileSize:       l.GenericPackagesMaxFileSize,
		HelmMaxFileSize:                  l.HelmMaxFileSize,
		MavenMaxFileSize:                 l.MavenMaxFileSize,
		NPMMaxFileSize:                   l.NPMMaxFileSize,
		NugetMaxFileSize:                 l.NugetMaxFileSize,
		PyPiMaxFileSize:                  l.PyPiMaxFileSize,
		TerraformModuleMaxFileSize:       l.TerraformModuleMaxFileSize,
		CargoMaxFileSize:                 l.CargoMaxFileSize,
		CIInstanceLevelVariables:         l.CIInstanceLevelVariables,
		CIPipelineSize:                   l.CIPipelineSize,
		CIActiveJobs:                     l.CIActiveJobs,
		CIProjectSubscriptions:           l.CIProjectSubscriptions,
		CIPipelineSchedules:              l.CIPipelineSchedules,
		CINeedsSizeLimit:                 l.CINeedsSizeLimit,
		CIRegisteredGroupRunners:         l.CIRegisteredGroupRunners,
		CIRegisteredProjectRunners:       l.CIRegisteredProjectRunners,
		DotenvVariables:                  l.DotenvVariables,
		DotenvSize:                       l.DotenvSize,
		EnforcementLimit:                 l.EnforcementLimit,
		NotificationLimit:                l.NotificationLimit,
		StorageSizeLimit:                 l.StorageSizeLimit,
		PipelineHierarchySize:            l.PipelineHierarchySize,
		MaxPipelinesPerMergeTrain:        l.MaxPipelinesPerMergeTrain,
		ServiceDeskOutboundEmailsPerHour: l.ServiceDeskOutboundEmailsPerHour,
		ServiceDeskOutboundEmailsPerDay:  l.ServiceDeskOutboundEmailsPerDay,
		WebHookCalls:                     l.WebHookCalls,
		WebHookCallsLow:                  l.WebHookCallsLow,
		WebHookCallsMid:                  l.WebHookCallsMid,
		LimitsHistory: LimitsHistory{
			EnforcementLimit:        limitChanges(l.LimitsHistory.EnforcementLimit),
			NotificationLimit:       limitChanges(l.LimitsHistory.NotificationLimit),
			StorageSizeLimit:        limitChanges(l.LimitsHistory.StorageSizeLimit),
			DashboardLimitEnabledAt: limitChanges(l.LimitsHistory.DashboardLimitEnabledAt),
		},
	}
}

// limitChanges maps one limit's change history, or nil when GitLab recorded
// none.
func limitChanges(entries []gl.PlanLimitHistoryEntry) []LimitChange {
	if len(entries) == 0 {
		return nil
	}
	out := make([]LimitChange, len(entries))
	for i, entry := range entries {
		out[i] = LimitChange(entry)
	}
	return out
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
