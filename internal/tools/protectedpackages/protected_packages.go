// Package protectedpackages implements GitLab package protection rule operations
// including list, create, update, and delete. Package protection rules restrict
// who can push or delete specific package patterns.
package protectedpackages

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds parameters for listing package protection rules.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetInput is not needed — GitLab API has no get-single-rule endpoint.

// CreateInput holds parameters for creating a package protection rule.
type CreateInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id"                              jsonschema:"Project ID or URL-encoded path,required"`
	PackageNamePattern          string               `json:"package_name_pattern"                    jsonschema:"Package name pattern with optional wildcards (e.g. @my-scope/my-pkg*),required"`
	PackageType                 string               `json:"package_type"                            jsonschema:"Package type (npm, pypi, maven, generic, etc.),required"`
	MinimumAccessLevelForPush   string               `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level for push (maintainer, owner, admin)"`
	MinimumAccessLevelForDelete string               `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level for delete (maintainer, owner, admin)"`
}

// UpdateInput holds parameters for updating a package protection rule.
type UpdateInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id"                              jsonschema:"Project ID or URL-encoded path,required"`
	RuleID                      int64                `json:"rule_id"                                 jsonschema:"Package protection rule ID,required"`
	PackageNamePattern          string               `json:"package_name_pattern,omitempty"          jsonschema:"Package name pattern with optional wildcards"`
	PackageType                 string               `json:"package_type,omitempty"                  jsonschema:"Package type (npm, pypi, maven, generic, etc.)"`
	MinimumAccessLevelForPush   string               `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level for push (maintainer, owner, admin)"`
	MinimumAccessLevelForDelete string               `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level for delete (maintainer, owner, admin)"`
}

// DeleteInput holds parameters for deleting a package protection rule.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RuleID    int64                `json:"rule_id"    jsonschema:"Package protection rule ID,required"`
}

// Output represents a package protection rule.
type Output struct {
	toolutil.HintableOutput
	ID                          int64  `json:"id"`
	ProjectID                   int64  `json:"project_id"`
	PackageNamePattern          string `json:"package_name_pattern"`
	PackageType                 string `json:"package_type"`
	MinimumAccessLevelForPush   string `json:"minimum_access_level_for_push,omitempty"`
	MinimumAccessLevelForDelete string `json:"minimum_access_level_for_delete,omitempty"`
}

// ListOutput contains a paginated list of package protection rules.
type ListOutput struct {
	toolutil.HintableOutput
	Rules      []Output                  `json:"rules"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(r *gl.PackageProtectionRule) Output {
	return Output{
		ID:                          r.ID,
		ProjectID:                   r.ProjectID,
		PackageNamePattern:          r.PackageNamePattern,
		PackageType:                 r.PackageType,
		MinimumAccessLevelForPush:   r.MinimumAccessLevelForPush,
		MinimumAccessLevelForDelete: r.MinimumAccessLevelForDelete,
	}
}

// List returns all package protection rules for a project.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("packageProtectionRuleList", err)
	}
	if in.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListPackageProtectionRulesOptions{}
	if in.Page > 0 {
		opts.Page = int64(in.Page)
	}
	if in.PerPage > 0 {
		opts.PerPage = int64(in.PerPage)
	}
	rules, resp, err := client.GL().ProtectedPackages.ListPackageProtectionRules(string(in.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("packageProtectionRuleList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; package protection rules require GitLab 16.7+ and the package_protection_rule feature")
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, r := range rules {
		out.Rules = append(out.Rules, toOutput(r))
	}
	return out, nil
}

// Create adds a new package protection rule to a project.
func Create(ctx context.Context, client *gitlabclient.Client, in CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("packageProtectionRuleCreate", err)
	}
	if in.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.PackageNamePattern == "" {
		return Output{}, errors.New("packageProtectionRuleCreate: package_name_pattern is required")
	}
	if in.PackageType == "" {
		return Output{}, errors.New("packageProtectionRuleCreate: package_type is required")
	}
	opts := &gl.CreatePackageProtectionRulesOptions{
		PackageNamePattern: new(in.PackageNamePattern),
		PackageType:        new(in.PackageType),
	}
	if in.MinimumAccessLevelForPush != "" {
		opts.MinimumAccessLevelForPush = gl.NewNullableWithValue(gl.ProtectionRuleAccessLevel(in.MinimumAccessLevelForPush))
	}
	if in.MinimumAccessLevelForDelete != "" {
		opts.MinimumAccessLevelForDelete = gl.NewNullableWithValue(gl.ProtectionRuleAccessLevel(in.MinimumAccessLevelForDelete))
	}
	rule, _, err := client.GL().ProtectedPackages.CreatePackageProtectionRules(string(in.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("packageProtectionRuleCreate", err, http.StatusBadRequest,
			"package_name_pattern must be a glob (e.g. com.example.*); package_type must be one of {npm, maven, conan, nuget, pypi, generic, golang, debian, rubygems, helm, terraform_module}; minimum_access_level_for_push and minimum_access_level_for_delete must be one of {maintainer, owner, admin}")
	}
	return toOutput(rule), nil
}

// Update modifies an existing package protection rule.
func Update(ctx context.Context, client *gitlabclient.Client, in UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("packageProtectionRuleUpdate", err)
	}
	if in.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.RuleID == 0 {
		return Output{}, toolutil.ErrFieldRequired("rule_id")
	}
	opts := &gl.UpdatePackageProtectionRulesOptions{}
	if in.PackageNamePattern != "" {
		opts.PackageNamePattern = new(in.PackageNamePattern)
	}
	if in.PackageType != "" {
		opts.PackageType = new(in.PackageType)
	}
	if in.MinimumAccessLevelForPush != "" {
		opts.MinimumAccessLevelForPush = gl.NewNullableWithValue(gl.ProtectionRuleAccessLevel(in.MinimumAccessLevelForPush))
	}
	if in.MinimumAccessLevelForDelete != "" {
		opts.MinimumAccessLevelForDelete = gl.NewNullableWithValue(gl.ProtectionRuleAccessLevel(in.MinimumAccessLevelForDelete))
	}
	rule, _, err := client.GL().ProtectedPackages.UpdatePackageProtectionRules(string(in.ProjectID), in.RuleID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("packageProtectionRuleUpdate", err, http.StatusNotFound,
			"verify rule_id with gitlab_package_protection_rule_list; pattern uniqueness still applies on rename")
	}
	return toOutput(rule), nil
}

// Delete removes a package protection rule from a project.
func Delete(ctx context.Context, client *gitlabclient.Client, in DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage("packageProtectionRuleDelete", err)
	}
	if in.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if in.RuleID == 0 {
		return toolutil.ErrFieldRequired("rule_id")
	}
	_, err := client.GL().ProtectedPackages.DeletePackageProtectionRules(string(in.ProjectID), in.RuleID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("packageProtectionRuleDelete", err, http.StatusNotFound,
			"verify rule_id with gitlab_package_protection_rule_list; managing protection rules requires Maintainer role or higher")
	}
	return nil
}
