package containerregistry //nolint:dupl // parallel CRUD wrapper for a distinct GitLab REST surface; mirrors protection_rules.go by design (different field names/types/endpoints, no shared SDK interface)

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Tag Protection Rule Output
// ---------------------------------------------------------------------------.

// TagProtectionRuleOutput represents a container registry tag protection rule.
type TagProtectionRuleOutput struct {
	toolutil.HintableOutput
	ID                          int64  `json:"id"`
	ProjectID                   int64  `json:"project_id"`
	TagNamePattern              string `json:"tag_name_pattern"`
	MinimumAccessLevelForPush   string `json:"minimum_access_level_for_push"`
	MinimumAccessLevelForDelete string `json:"minimum_access_level_for_delete"`
}

// TagProtectionRuleListOutput represents a list of tag protection rules.
type TagProtectionRuleListOutput struct {
	toolutil.HintableOutput
	Rules      []TagProtectionRuleOutput `json:"rules"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// convertTagProtectionRule maps a GitLab container registry tag protection rule into MCP output.
func convertTagProtectionRule(r *gl.ContainerRegistryTagProtectionRule) TagProtectionRuleOutput {
	return TagProtectionRuleOutput{
		ID:                          r.ID,
		ProjectID:                   r.ProjectID,
		TagNamePattern:              r.TagNamePattern,
		MinimumAccessLevelForPush:   string(r.MinimumAccessLevelForPush),
		MinimumAccessLevelForDelete: string(r.MinimumAccessLevelForDelete),
	}
}

// ---------------------------------------------------------------------------
// ListContainerRegistryTagProtectionRules
// ---------------------------------------------------------------------------.

// ListTagProtectionRulesInput selects the project whose registry tag protection rules are listed.
type ListTagProtectionRulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
}

// ListTagProtectionRules lists container registry tag protection rules for a project.
func ListTagProtectionRules(ctx context.Context, client *gitlabclient.Client, input ListTagProtectionRulesInput) (TagProtectionRuleListOutput, error) {
	if input.ProjectID == "" {
		return TagProtectionRuleListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	rules, resp, err := client.GL().ContainerRegistryTagProtectionRules.ListContainerRegistryTagProtectionRules(
		string(input.ProjectID), gl.WithContext(ctx),
	)
	if err != nil {
		return TagProtectionRuleListOutput{}, toolutil.WrapErrWithStatusHint("registry_tag_protection_list", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; container registry tag protection requires GitLab 17.8+ and may need the container_registry_immutable_tags feature")
	}
	out := TagProtectionRuleListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, r := range rules {
		out.Rules = append(out.Rules, convertTagProtectionRule(r))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// CreateContainerRegistryTagProtectionRule
// ---------------------------------------------------------------------------.

// CreateTagProtectionRuleInput defines the tag name pattern and access thresholds for a new rule.
type CreateTagProtectionRuleInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	TagNamePattern              string               `json:"tag_name_pattern" jsonschema:"Tag name pattern as a RE2 regular expression (e.g. v.+),required"`
	MinimumAccessLevelForPush   string               `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level to push matching tags (maintainer, owner, admin). Omit both push and delete levels to make matching tags immutable"`
	MinimumAccessLevelForDelete string               `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level to delete matching tags (maintainer, owner, admin). Omit both push and delete levels to make matching tags immutable"`
}

// CreateTagProtectionRule creates a container registry tag protection rule.
func CreateTagProtectionRule(ctx context.Context, client *gitlabclient.Client, input CreateTagProtectionRuleInput) (TagProtectionRuleOutput, error) {
	if input.ProjectID == "" {
		return TagProtectionRuleOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.TagNamePattern == "" {
		return TagProtectionRuleOutput{}, toolutil.ErrFieldRequired("tag_name_pattern")
	}
	opts := &gl.CreateContainerRegistryTagProtectionRuleOptions{
		TagNamePattern: new(input.TagNamePattern),
	}
	if input.MinimumAccessLevelForPush != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForPush)
		opts.MinimumAccessLevelForPush = &lvl
	}
	if input.MinimumAccessLevelForDelete != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForDelete)
		opts.MinimumAccessLevelForDelete = &lvl
	}
	rule, _, err := client.GL().ContainerRegistryTagProtectionRules.CreateContainerRegistryTagProtectionRule(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return TagProtectionRuleOutput{}, toolutil.WrapErrWithStatusHint("registry_tag_protection_create", err, http.StatusBadRequest,
			"tag_name_pattern must be a valid RE2 regular expression and unique within the project; minimum_access_level_for_push and minimum_access_level_for_delete must be one of {maintainer, owner, admin}, or omit both to make matching tags immutable")
	}
	return convertTagProtectionRule(rule), nil
}

// ---------------------------------------------------------------------------
// UpdateContainerRegistryTagProtectionRule
// ---------------------------------------------------------------------------.

// UpdateTagProtectionRuleInput identifies a registry tag protection rule and the fields to change.
type UpdateTagProtectionRuleInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RuleID                      int64                `json:"rule_id" jsonschema:"Tag protection rule ID,required"`
	TagNamePattern              string               `json:"tag_name_pattern,omitempty" jsonschema:"Tag name pattern as a RE2 regular expression"`
	MinimumAccessLevelForPush   string               `json:"minimum_access_level_for_push,omitempty" jsonschema:"Minimum access level to push matching tags (maintainer, owner, admin)"`
	MinimumAccessLevelForDelete string               `json:"minimum_access_level_for_delete,omitempty" jsonschema:"Minimum access level to delete matching tags (maintainer, owner, admin)"`
}

// UpdateTagProtectionRule updates a container registry tag protection rule.
func UpdateTagProtectionRule(ctx context.Context, client *gitlabclient.Client, input UpdateTagProtectionRuleInput) (TagProtectionRuleOutput, error) {
	if input.ProjectID == "" {
		return TagProtectionRuleOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RuleID == 0 {
		return TagProtectionRuleOutput{}, toolutil.ErrFieldRequired("rule_id")
	}
	opts := &gl.UpdateContainerRegistryTagProtectionRuleOptions{}
	if input.TagNamePattern != "" {
		opts.TagNamePattern = new(input.TagNamePattern)
	}
	if input.MinimumAccessLevelForPush != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForPush)
		opts.MinimumAccessLevelForPush = &lvl
	}
	if input.MinimumAccessLevelForDelete != "" {
		lvl := gl.ProtectionRuleAccessLevel(input.MinimumAccessLevelForDelete)
		opts.MinimumAccessLevelForDelete = &lvl
	}
	rule, _, err := client.GL().ContainerRegistryTagProtectionRules.UpdateContainerRegistryTagProtectionRule(
		string(input.ProjectID), input.RuleID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return TagProtectionRuleOutput{}, toolutil.WrapErrWithStatusHint("registry_tag_protection_update", err, http.StatusNotFound,
			"verify rule_id with gitlab_registry_tag_protection_list; tag_name_pattern uniqueness still applies on rename")
	}
	return convertTagProtectionRule(rule), nil
}

// ---------------------------------------------------------------------------
// DeleteContainerRegistryTagProtectionRule
// ---------------------------------------------------------------------------.

// DeleteTagProtectionRuleInput identifies the registry tag protection rule to delete.
type DeleteTagProtectionRuleInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RuleID    int64                `json:"rule_id" jsonschema:"Tag protection rule ID,required"`
}

// DeleteTagProtectionRule deletes a container registry tag protection rule.
func DeleteTagProtectionRule(ctx context.Context, client *gitlabclient.Client, input DeleteTagProtectionRuleInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.RuleID == 0 {
		return toolutil.ErrFieldRequired("rule_id")
	}
	_, err := client.GL().ContainerRegistryTagProtectionRules.DeleteContainerRegistryTagProtectionRule(
		string(input.ProjectID), input.RuleID, gl.WithContext(ctx),
	)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("registry_tag_protection_delete", err, http.StatusNotFound,
			"verify rule_id with gitlab_registry_tag_protection_list; managing tag protection rules requires Maintainer role or higher")
	}
	return nil
}
