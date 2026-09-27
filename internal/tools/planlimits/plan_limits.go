package planlimits

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// PlanLimitItem represents GitLab plan limits.
//
// The eight package file sizes at the top are what client-go's PlanLimit
// models. The block after them is what lib/api/entities/plan_limit.rb sends
// beside those and client-go declares on no field, read from the captured
// response beside the SDK's decode (ADR-0021) and described on [limitExtra].
// The entity exposes every one of them with no condition, so both plan limit
// routes answer with all of them, and each stays nil when the instance sent no
// key for it.
type PlanLimitItem struct {
	ConanMaxFileSize           int64 `json:"conan_max_file_size"`
	GenericPackagesMaxFileSize int64 `json:"generic_packages_max_file_size"`
	HelmMaxFileSize            int64 `json:"helm_max_file_size"`
	MavenMaxFileSize           int64 `json:"maven_max_file_size"`
	NPMMaxFileSize             int64 `json:"npm_max_file_size"`
	NugetMaxFileSize           int64 `json:"nuget_max_file_size"`
	PyPiMaxFileSize            int64 `json:"pypi_max_file_size"`
	TerraformModuleMaxFileSize int64 `json:"terraform_module_max_file_size"`

	CargoMaxFileSize                 *int64                   `json:"cargo_max_file_size,omitempty"`
	CIInstanceLevelVariables         *int64                   `json:"ci_instance_level_variables,omitempty"`
	CIPipelineSize                   *int64                   `json:"ci_pipeline_size,omitempty"`
	CIActiveJobs                     *int64                   `json:"ci_active_jobs,omitempty"`
	CIProjectSubscriptions           *int64                   `json:"ci_project_subscriptions,omitempty"`
	CIPipelineSchedules              *int64                   `json:"ci_pipeline_schedules,omitempty"`
	CINeedsSizeLimit                 *int64                   `json:"ci_needs_size_limit,omitempty"`
	CIRegisteredGroupRunners         *int64                   `json:"ci_registered_group_runners,omitempty"`
	CIRegisteredProjectRunners       *int64                   `json:"ci_registered_project_runners,omitempty"`
	DotenvVariables                  *int64                   `json:"dotenv_variables,omitempty"`
	DotenvSize                       *int64                   `json:"dotenv_size,omitempty"`
	EnforcementLimit                 *int64                   `json:"enforcement_limit,omitempty"`
	NotificationLimit                *int64                   `json:"notification_limit,omitempty"`
	StorageSizeLimit                 *int64                   `json:"storage_size_limit,omitempty"`
	PipelineHierarchySize            *int64                   `json:"pipeline_hierarchy_size,omitempty"`
	MaxPipelinesPerMergeTrain        *int64                   `json:"max_pipelines_per_merge_train,omitempty"`
	ServiceDeskOutboundEmailsPerHour *int64                   `json:"service_desk_outbound_emails_per_hour,omitempty"`
	ServiceDeskOutboundEmailsPerDay  *int64                   `json:"service_desk_outbound_emails_per_day,omitempty"`
	WebHookCalls                     *int64                   `json:"web_hook_calls,omitempty"`
	WebHookCallsLow                  *int64                   `json:"web_hook_calls_low,omitempty"`
	WebHookCallsMid                  *int64                   `json:"web_hook_calls_mid,omitempty"`
	LimitsHistory                    map[string][]LimitChange `json:"limits_history,omitempty"`
}

// LimitChange is one entry of a limit's change history: who set it to which
// value, and when, as a Unix timestamp in seconds. GitLab keeps it for the
// three namespace storage limits and the date the storage dashboard limit was
// enabled, keyed by the limit's name.
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	limits, _, err := client.GL().PlanLimits.GetCurrentPlanLimits(opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_plan_limits", err, http.StatusForbidden, "plan limits require administrator access")
	}
	extra, err := capturedLimits(captured)
	if err != nil {
		return GetOutput{}, toolutil.WrapErr("get_plan_limits", err)
	}
	return GetOutput{
		PlanLimitItem: convertPlanLimit(limits, extra),
	}, nil
}

// ---------------------------------------------------------------------------
// ChangePlanLimits
// ---------------------------------------------------------------------------.

// ChangeInput is the input for changing plan limits.
type ChangeInput struct {
	PlanName                   string `json:"plan_name" jsonschema:"Plan name to update (e.g. default, free, bronze, silver, gold, premium, ultimate),required"`
	ConanMaxFileSize           *int64 `json:"conan_max_file_size,omitempty" jsonschema:"Maximum Conan package file size in bytes"`
	GenericPackagesMaxFileSize *int64 `json:"generic_packages_max_file_size,omitempty" jsonschema:"Maximum generic package file size in bytes"`
	HelmMaxFileSize            *int64 `json:"helm_max_file_size,omitempty" jsonschema:"Maximum Helm chart file size in bytes"`
	MavenMaxFileSize           *int64 `json:"maven_max_file_size,omitempty" jsonschema:"Maximum Maven package file size in bytes"`
	NPMMaxFileSize             *int64 `json:"npm_max_file_size,omitempty" jsonschema:"Maximum NPM package file size in bytes"`
	NugetMaxFileSize           *int64 `json:"nuget_max_file_size,omitempty" jsonschema:"Maximum NuGet package file size in bytes"`
	PyPiMaxFileSize            *int64 `json:"pypi_max_file_size,omitempty" jsonschema:"Maximum PyPI package file size in bytes"`
	TerraformModuleMaxFileSize *int64 `json:"terraform_module_max_file_size,omitempty" jsonschema:"Maximum Terraform module file size in bytes"`
}

// ChangeOutput is the output for changing plan limits.
type ChangeOutput struct {
	toolutil.HintableOutput
	PlanLimitItem
}

// Change modifies plan limits.
func Change(ctx context.Context, client *gitlabclient.Client, input ChangeInput) (ChangeOutput, error) {
	opts := &gl.ChangePlanLimitOptions{
		PlanName:                   new(input.PlanName),
		ConanMaxFileSize:           input.ConanMaxFileSize,
		GenericPackagesMaxFileSize: input.GenericPackagesMaxFileSize,
		HelmMaxFileSize:            input.HelmMaxFileSize,
		MavenMaxFileSize:           input.MavenMaxFileSize,
		NPMMaxFileSize:             input.NPMMaxFileSize,
		NugetMaxFileSize:           input.NugetMaxFileSize,
		PyPiMaxFileSize:            input.PyPiMaxFileSize,
		TerraformModuleMaxFileSize: input.TerraformModuleMaxFileSize,
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	limits, _, err := client.GL().PlanLimits.ChangePlanLimits(opts, gl.WithContext(ctx))
	if err != nil {
		return ChangeOutput{}, toolutil.WrapErrWithStatusHint("change_plan_limits", err, http.StatusForbidden, "changing plan limits requires administrator access")
	}
	extra, err := capturedLimits(captured)
	if err != nil {
		return ChangeOutput{}, toolutil.WrapErr("change_plan_limits", err)
	}
	return ChangeOutput{
		PlanLimitItem: convertPlanLimit(limits, extra),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// convertPlanLimit maps GitLab plan limit settings into MCP output: the eight
// file sizes the SDK decoded, and the rest from what the capture read.
func convertPlanLimit(l *gl.PlanLimit, extra limitExtra) PlanLimitItem {
	return PlanLimitItem{
		ConanMaxFileSize:           l.ConanMaxFileSize,
		GenericPackagesMaxFileSize: l.GenericPackagesMaxFileSize,
		HelmMaxFileSize:            l.HelmMaxFileSize,
		MavenMaxFileSize:           l.MavenMaxFileSize,
		NPMMaxFileSize:             l.NPMMaxFileSize,
		NugetMaxFileSize:           l.NugetMaxFileSize,
		PyPiMaxFileSize:            l.PyPiMaxFileSize,
		TerraformModuleMaxFileSize: l.TerraformModuleMaxFileSize,

		CargoMaxFileSize:                 extra.CargoMaxFileSize,
		CIInstanceLevelVariables:         extra.CIInstanceLevelVariables,
		CIPipelineSize:                   extra.CIPipelineSize,
		CIActiveJobs:                     extra.CIActiveJobs,
		CIProjectSubscriptions:           extra.CIProjectSubscriptions,
		CIPipelineSchedules:              extra.CIPipelineSchedules,
		CINeedsSizeLimit:                 extra.CINeedsSizeLimit,
		CIRegisteredGroupRunners:         extra.CIRegisteredGroupRunners,
		CIRegisteredProjectRunners:       extra.CIRegisteredProjectRunners,
		DotenvVariables:                  extra.DotenvVariables,
		DotenvSize:                       extra.DotenvSize,
		EnforcementLimit:                 extra.EnforcementLimit,
		NotificationLimit:                extra.NotificationLimit,
		StorageSizeLimit:                 extra.StorageSizeLimit,
		PipelineHierarchySize:            extra.PipelineHierarchySize,
		MaxPipelinesPerMergeTrain:        extra.MaxPipelinesPerMergeTrain,
		ServiceDeskOutboundEmailsPerHour: extra.ServiceDeskOutboundEmailsPerHour,
		ServiceDeskOutboundEmailsPerDay:  extra.ServiceDeskOutboundEmailsPerDay,
		WebHookCalls:                     extra.WebHookCalls,
		WebHookCallsLow:                  extra.WebHookCallsLow,
		WebHookCallsMid:                  extra.WebHookCallsMid,
		LimitsHistory:                    extra.LimitsHistory,
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
