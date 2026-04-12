// protection_rules.go implements container registry protection rule operations.

package containerregistry

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Protection Rule Output
// ---------------------------------------------------------------------------.

// ProtectionRuleOutput represents a container registry protection rule.
type ProtectionRuleOutput struct {
	toolutil.HintableOutput
	ID                          int64  `json:"id"`
	ProjectID                   int64  `json:"project_id"`
	RepositoryPathPattern       string `json:"repository_path_pattern"`
	MinimumAccessLevelForPush   string `json:"minimum_access_level_for_push"`
	MinimumAccessLevelForDelete string `json:"minimum_access_level_for_delete"`
}

// ProtectionRuleListOutput represents a list of protection rules.
type ProtectionRuleListOutput struct {
	toolutil.HintableOutput
	Rules      []ProtectionRuleOutput    `json:"rules"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// convertProtectionRule is an internal helper for the containerregistry package.
func convertProtectionRule(r *gl.ContainerRegistryProtectionRule) ProtectionRuleOutput {
	return ProtectionRuleOutput{
		ID:                          r.ID,
		ProjectID:                   r.ProjectID,
		RepositoryPathPattern:       r.RepositoryPathPattern,
		MinimumAccessLevelForPush:   string(r.MinimumAccessLevelForPush),
		MinimumAccessLevelForDelete: string(r.MinimumAccessLevelForDelete),
	}
}

// ---------------------------------------------------------------------------
// ListContainerRegistryProtectionRules
// ---------------------------------------------------------------------------.

// ListProtectionRulesInput represents the input for listing protection rules.
type ListProtectionRulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
}

// ListProtectionRules lists container registry protection rules for a project.
func ListProtectionRules(ctx context.Context, client *gitlabclient.Client, input ListProtectionRulesInput) (ProtectionRuleListOutput, error) {
	if input.ProjectID == "" {
		return ProtectionRuleListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	rules, resp, err := client.GL().ContainerRegistryProtectionRules.ListContainerRegistryProtectionRules(
		string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ProtectionRuleListOutput{}, toolutil.WrapErrWithMessage("registry_protection_list", err)
	}
	out := ProtectionRuleListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, r := range rules {
		out.Rules = append(out.Rules, convertProtectionRule(r))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// CreateContainerRegistryProtectionRule
// ---------------------------------------------------------------------------.

// CreateProtectionRuleInput represents the input for creating a protection rule.
type CreateProtectionRuleInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RepositoryPathPattern       string               `json:"repository_path_pattern" jsonschema:"Repository path pattern (e.g. my-project/my-image*),required"`
	MinimumAccessLevelForPush   string               `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level for push (maintainer, owner, admin)"`
	MinimumAccessLevelForDelete string               `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level for delete (maintainer, owner, admin)"`
}

// CreateProtectionRule creates a container registry protection rule.
func CreateProtectionRule(ctx context.Context, client *gitlabclient.Client, input CreateProtectionRuleInput) (ProtectionRuleOutput, error) {
	if input.ProjectID == "" {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RepositoryPathPattern == "" {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("repository_path_pattern")
	}
	opts := &gl.CreateContainerRegistryProtectionRuleOptions{
		RepositoryPathPattern: new(input.RepositoryPathPattern),
	}
	if input.MinimumAccessLevelForPush != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForPush)
		opts.MinimumAccessLevelForPush = &lvl
	}
	if input.MinimumAccessLevelForDelete != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForDelete)
		opts.MinimumAccessLevelForDelete = &lvl
	}
	rule, _, err := client.GL().ContainerRegistryProtectionRules.CreateContainerRegistryProtectionRule(
		string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ProtectionRuleOutput{}, toolutil.WrapErrWithMessage("registry_protection_create", err)
	}
	return convertProtectionRule(rule), nil
}

// ---------------------------------------------------------------------------
// UpdateContainerRegistryProtectionRule
// ---------------------------------------------------------------------------.

// UpdateProtectionRuleInput represents the input for updating a protection rule.
type UpdateProtectionRuleInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RuleID                      int64                `json:"rule_id" jsonschema:"Protection rule ID,required"`
	RepositoryPathPattern       string               `json:"repository_path_pattern,omitempty" jsonschema:"Repository path pattern"`
	MinimumAccessLevelForPush   string               `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level for push (maintainer, owner, admin)"`
	MinimumAccessLevelForDelete string               `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level for delete (maintainer, owner, admin)"`
}

// UpdateProtectionRule updates a container registry protection rule.
func UpdateProtectionRule(ctx context.Context, client *gitlabclient.Client, input UpdateProtectionRuleInput) (ProtectionRuleOutput, error) {
	if input.ProjectID == "" {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RuleID == 0 {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("rule_id")
	}
	opts := &gl.UpdateContainerRegistryProtectionRuleOptions{}
	if input.RepositoryPathPattern != "" {
		opts.RepositoryPathPattern = new(input.RepositoryPathPattern)
	}
	if input.MinimumAccessLevelForPush != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForPush)
		opts.MinimumAccessLevelForPush = &lvl
	}
	if input.MinimumAccessLevelForDelete != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForDelete)
		opts.MinimumAccessLevelForDelete = &lvl
	}
	rule, _, err := client.GL().ContainerRegistryProtectionRules.UpdateContainerRegistryProtectionRule(
		string(input.ProjectID), input.RuleID, opts, gl.WithContext(ctx))
	if err != nil {
		return ProtectionRuleOutput{}, toolutil.WrapErrWithMessage("registry_protection_update", err)
	}
	return convertProtectionRule(rule), nil
}

// ---------------------------------------------------------------------------
// DeleteContainerRegistryProtectionRule
// ---------------------------------------------------------------------------.

// DeleteProtectionRuleInput represents the input for deleting a protection rule.
type DeleteProtectionRuleInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RuleID    int64                `json:"rule_id" jsonschema:"Protection rule ID,required"`
}

// DeleteProtectionRule deletes a container registry protection rule.
func DeleteProtectionRule(ctx context.Context, client *gitlabclient.Client, input DeleteProtectionRuleInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.RuleID == 0 {
		return toolutil.ErrFieldRequired("rule_id")
	}
	_, err := client.GL().ContainerRegistryProtectionRules.DeleteContainerRegistryProtectionRule(
		string(input.ProjectID), input.RuleID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("registry_protection_delete", err)
	}
	return nil
}
