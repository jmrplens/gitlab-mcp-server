// Package planlimits implements MCP tools for GitLab Plan Limits API.
package planlimits

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// PlanLimitItem represents GitLab plan limits.
type PlanLimitItem struct {
	ConanMaxFileSize           int64 `json:"conan_max_file_size"`
	GenericPackagesMaxFileSize int64 `json:"generic_packages_max_file_size"`
	HelmMaxFileSize            int64 `json:"helm_max_file_size"`
	MavenMaxFileSize           int64 `json:"maven_max_file_size"`
	NPMMaxFileSize             int64 `json:"npm_max_file_size"`
	NugetMaxFileSize           int64 `json:"nuget_max_file_size"`
	PyPiMaxFileSize            int64 `json:"pypi_max_file_size"`
	TerraformModuleMaxFileSize int64 `json:"terraform_module_max_file_size"`
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

// convertPlanLimit is an internal helper for the planlimits package.
func convertPlanLimit(l *gl.PlanLimit) PlanLimitItem {
	return PlanLimitItem{
		ConanMaxFileSize:           l.ConanMaxFileSize,
		GenericPackagesMaxFileSize: l.GenericPackagesMaxFileSize,
		HelmMaxFileSize:            l.HelmMaxFileSize,
		MavenMaxFileSize:           l.MavenMaxFileSize,
		NPMMaxFileSize:             l.NPMMaxFileSize,
		NugetMaxFileSize:           l.NugetMaxFileSize,
		PyPiMaxFileSize:            l.PyPiMaxFileSize,
		TerraformModuleMaxFileSize: l.TerraformModuleMaxFileSize,
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
