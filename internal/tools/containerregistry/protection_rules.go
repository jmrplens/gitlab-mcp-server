package containerregistry

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Protection Rule Output
// ---------------------------------------------------------------------------.

// ProtectionRuleOutput represents a container registry protection rule.
type ProtectionRuleOutput struct {
	toolutil.HintableOutput
	ID                          int64                        `json:"id"`
	ProjectID                   int64                        `json:"project_id"`
	RepositoryPathPattern       string                       `json:"repository_path_pattern"`
	MinimumAccessLevelForPush   gl.ProtectionRuleAccessLevel `json:"minimum_access_level_for_push"`
	MinimumAccessLevelForDelete gl.ProtectionRuleAccessLevel `json:"minimum_access_level_for_delete"`
}

// ProtectionRuleListOutput represents a list of protection rules.
type ProtectionRuleListOutput struct {
	toolutil.HintableOutput
	Rules      []ProtectionRuleOutput    `json:"rules"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// convertProtectionRule maps a GitLab container registry protection rule into MCP output.
func convertProtectionRule(r *gl.ContainerRegistryProtectionRule) ProtectionRuleOutput {
	return ProtectionRuleOutput{
		ID:                          r.ID,
		ProjectID:                   r.ProjectID,
		RepositoryPathPattern:       r.RepositoryPathPattern,
		MinimumAccessLevelForPush:   r.MinimumAccessLevelForPush,
		MinimumAccessLevelForDelete: r.MinimumAccessLevelForDelete,
	}
}

// ---------------------------------------------------------------------------
// ListContainerRegistryProtectionRules
// ---------------------------------------------------------------------------.

// ListProtectionRulesInput selects the project whose registry protection rules are listed.
type ListProtectionRulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
}

// ListProtectionRules lists container registry protection rules for a project.
func ListProtectionRules(ctx context.Context, client *gitlabclient.Client, input ListProtectionRulesInput) (ProtectionRuleListOutput, error) {
	if input.ProjectID == "" {
		return ProtectionRuleListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	rules, resp, err := client.GL().ContainerRegistryProtectionRules.ListContainerRegistryProtectionRules(
		string(input.ProjectID), gl.WithContext(ctx),
	)
	if err != nil {
		return ProtectionRuleListOutput{}, toolutil.WrapErrWithStatusHint("registry_protection_list", err, http.StatusNotFound,
			"verify project_id with project.get; container registry protection requires GitLab 16.7+ and may need feature flag")
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

// CreateProtectionRuleInput defines the repository path pattern and access thresholds for a new rule.
type CreateProtectionRuleInput struct {
	ProjectID                   toolutil.StringOrInt         `json:"project_id" jsonschema:"Project ID or path,required"`
	RepositoryPathPattern       string                       `json:"repository_path_pattern" jsonschema:"Repository path pattern: the project's full path in lower case, then the path of the images to protect, where * matches any characters (e.g. my-group/my-project/my-image*). GitLab refuses a pattern that does not start with the project's full path,required"`
	MinimumAccessLevelForPush   gl.ProtectionRuleAccessLevel `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level for push (maintainer, owner, admin). Send at least one of the two levels"`
	MinimumAccessLevelForDelete gl.ProtectionRuleAccessLevel `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level for delete (maintainer, owner, admin). Send at least one of the two levels"`
}

// CreateProtectionRule creates a container registry protection rule.
func CreateProtectionRule(ctx context.Context, client *gitlabclient.Client, input CreateProtectionRuleInput) (ProtectionRuleOutput, error) {
	if input.ProjectID == "" {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RepositoryPathPattern == "" {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("repository_path_pattern")
	}
	// GitLab's at_least_one_of refuses a rule with neither level. The
	// catalog's anyOf states the rule as data, and no tool's served input
	// schema may carry it at its root, which the Anthropic Messages API
	// refuses for a whole tool list, so this is what holds every surface to
	// it. An empty level is none, as the request below leaves it out.
	if input.MinimumAccessLevelForPush == "" && input.MinimumAccessLevelForDelete == "" {
		return ProtectionRuleOutput{}, errors.New("registry_protection_create: minimum_access_level_for_push or minimum_access_level_for_delete is required. Name at least one of the two levels a rule protects with: maintainer, owner or admin")
	}
	opts := &gl.CreateContainerRegistryProtectionRuleOptions{
		RepositoryPathPattern: new(input.RepositoryPathPattern),
	}
	if input.MinimumAccessLevelForPush != "" {
		opts.MinimumAccessLevelForPush = new(input.MinimumAccessLevelForPush)
	}
	if input.MinimumAccessLevelForDelete != "" {
		opts.MinimumAccessLevelForDelete = new(input.MinimumAccessLevelForDelete)
	}
	rule, _, err := client.GL().ContainerRegistryProtectionRules.CreateContainerRegistryProtectionRule(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ProtectionRuleOutput{}, toolutil.WrapErrWithStatusHint("registry_protection_create", err, http.StatusBadRequest,
			"repository_path_pattern must be a glob (e.g. project/path/*); minimum_access_level_for_push and minimum_access_level_for_delete must be one of {maintainer, owner, admin}; pattern must be unique within the project")
	}
	return convertProtectionRule(rule), nil
}

// ---------------------------------------------------------------------------
// UpdateContainerRegistryProtectionRule
// ---------------------------------------------------------------------------.

// UpdateProtectionRuleInput identifies a registry protection rule and the fields to change.
type UpdateProtectionRuleInput struct {
	ProjectID                   toolutil.StringOrInt          `json:"project_id" jsonschema:"Project ID or path,required"`
	RuleID                      int64                         `json:"rule_id" jsonschema:"Protection rule ID,required"`
	RepositoryPathPattern       string                        `json:"repository_path_pattern,omitempty" jsonschema:"Repository path pattern: the project's full path in lower case, then the path of the images to protect, where * matches any characters. GitLab refuses a pattern that does not start with the project's full path"`
	MinimumAccessLevelForPush   *gl.ProtectionRuleAccessLevel `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level for push (maintainer, owner, admin). Pass an empty string to remove it, which GitLab accepts while the delete level stays set. Omit the field to leave it untouched"`
	MinimumAccessLevelForDelete *gl.ProtectionRuleAccessLevel `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level for delete (maintainer, owner, admin). Pass an empty string to remove it, which GitLab accepts while the push level stays set. Omit the field to leave it untouched"`
}

// UpdateProtectionRule updates a container registry protection rule.
func UpdateProtectionRule(ctx context.Context, client *gitlabclient.Client, input UpdateProtectionRuleInput) (ProtectionRuleOutput, error) {
	if input.ProjectID == "" {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RuleID == 0 {
		return ProtectionRuleOutput{}, toolutil.ErrFieldRequired("rule_id")
	}
	// Presence decides, never emptiness: an empty access level is the caller
	// removing it (issue 1306).
	opts := &gl.UpdateContainerRegistryProtectionRuleOptions{
		MinimumAccessLevelForPush:   input.MinimumAccessLevelForPush,
		MinimumAccessLevelForDelete: input.MinimumAccessLevelForDelete,
	}
	if input.RepositoryPathPattern != "" {
		opts.RepositoryPathPattern = new(input.RepositoryPathPattern)
	}
	rule, _, err := client.GL().ContainerRegistryProtectionRules.UpdateContainerRegistryProtectionRule(
		string(input.ProjectID), input.RuleID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ProtectionRuleOutput{}, toolutil.WrapErrWithStatusHint("registry_protection_update", err, http.StatusNotFound,
			"verify rule_id with package.registry_rule_list; pattern uniqueness still applies on rename")
	}
	return convertProtectionRule(rule), nil
}

// ---------------------------------------------------------------------------
// DeleteContainerRegistryProtectionRule
// ---------------------------------------------------------------------------.

// DeleteProtectionRuleInput identifies the registry protection rule to delete.
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
		string(input.ProjectID), input.RuleID, gl.WithContext(ctx),
	)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("registry_protection_delete", err, http.StatusNotFound,
			"verify rule_id with package.registry_rule_list; managing protection rules requires Maintainer role or higher")
	}
	return nil
}
