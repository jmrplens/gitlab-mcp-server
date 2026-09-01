package projects

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TargetBranchRuleOutput mirrors gl.TargetBranchRule 1:1. A target branch rule
// maps a source-branch name pattern to a default target branch for merge
// requests (the "merge request target branch workflow", a Premium/Ultimate
// feature exposed via GitLab GraphQL).
type TargetBranchRuleOutput struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	TargetBranch string `json:"target_branch"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// targetBranchRuleToOutput converts an SDK gl.TargetBranchRule into the
// MCP output shape, formatting the creation timestamp as RFC 3339.
func targetBranchRuleToOutput(r *gl.TargetBranchRule) TargetBranchRuleOutput {
	out := TargetBranchRuleOutput{
		ID:           r.ID,
		Name:         r.Name,
		TargetBranch: r.TargetBranch,
	}
	if !r.CreatedAt.IsZero() {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// ListTargetBranchRulesInput defines parameters for listing a project's target
// branch rules. The GitLab GraphQL project(fullPath:) field does not accept
// numeric IDs, so project_id must be the full namespace/project path.
type ListTargetBranchRulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Full project path (namespace/project). The target branch rules GraphQL query requires the full path and does not accept a numeric ID,required"`
}

// ListTargetBranchRulesOutput holds a project's target branch rules. The
// underlying GraphQL connection returns every rule in one response, so there is
// no pagination envelope to mirror.
type ListTargetBranchRulesOutput struct {
	toolutil.HintableOutput
	TargetBranchRules []TargetBranchRuleOutput `json:"target_branch_rules"`
}

// ListTargetBranchRules returns the target branch rules configured for a
// project. Premium/Ultimate.
func ListTargetBranchRules(ctx context.Context, client *gitlabclient.Client, input ListTargetBranchRulesInput) (ListTargetBranchRulesOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListTargetBranchRulesOutput{}, err
	}
	if input.ProjectID == "" {
		return ListTargetBranchRulesOutput{}, errors.New("projectListTargetBranchRules: project_id is required. Pass the full project path (namespace/project); the target branch rules query does not accept a numeric ID")
	}
	rules, _, err := client.GL().Projects.ListProjectTargetBranchRules(input.ProjectID.String(), gl.WithContext(ctx))
	if err != nil {
		return ListTargetBranchRulesOutput{}, toolutil.WrapErrWithStatusHint(
			"projectListTargetBranchRules", err, http.StatusNotFound,
			"pass the full project path (namespace/project), not a numeric ID. Target branch rules require Premium/Ultimate",
		)
	}
	out := make([]TargetBranchRuleOutput, 0, len(rules))
	for i := range rules {
		out = append(out, targetBranchRuleToOutput(&rules[i]))
	}
	return ListTargetBranchRulesOutput{TargetBranchRules: out}, nil
}

// CreateTargetBranchRuleInput defines parameters for creating a target branch
// rule. The create mutation takes a numeric project ID, so project_id must be
// numeric here (unlike the list query which requires the full path).
type CreateTargetBranchRuleInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Numeric project ID that owns the rule. The create mutation requires a numeric ID,required"`
	Name         string               `json:"name" jsonschema:"Source branch name or wildcard pattern that triggers the rule (e.g. release/*),required"`
	TargetBranch string               `json:"target_branch" jsonschema:"Default target branch merge requests opened from matching source branches will target,required"`
}

// CreateTargetBranchRule creates a target branch rule for a project.
// Premium/Ultimate. Not destructive.
func CreateTargetBranchRule(ctx context.Context, client *gitlabclient.Client, input CreateTargetBranchRuleInput) (TargetBranchRuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return TargetBranchRuleOutput{}, err
	}
	if input.ProjectID == "" {
		return TargetBranchRuleOutput{}, errors.New("projectCreateTargetBranchRule: project_id is required (numeric project ID)")
	}
	pid, err := input.ProjectID.Int64()
	if err != nil {
		return TargetBranchRuleOutput{}, errors.New("projectCreateTargetBranchRule: project_id must be a numeric project ID for this action; use gitlab_project_get to resolve a path to its ID")
	}
	if input.Name == "" {
		return TargetBranchRuleOutput{}, errors.New("projectCreateTargetBranchRule: name is required (the source branch name or wildcard pattern)")
	}
	if input.TargetBranch == "" {
		return TargetBranchRuleOutput{}, errors.New("projectCreateTargetBranchRule: target_branch is required")
	}
	opts := &gl.CreateTargetBranchRuleOptions{
		Name:         input.Name,
		TargetBranch: input.TargetBranch,
	}
	rule, _, err := client.GL().Projects.CreateTargetBranchRule(pid, opts, gl.WithContext(ctx))
	if err != nil {
		return TargetBranchRuleOutput{}, toolutil.WrapErrWithMessage("projectCreateTargetBranchRule", err)
	}
	return targetBranchRuleToOutput(rule), nil
}

// DeleteTargetBranchRuleInput defines parameters for deleting a target branch
// rule. The destroy mutation identifies the rule solely by its own ID.
type DeleteTargetBranchRuleInput struct {
	RuleID int64 `json:"rule_id" jsonschema:"Target branch rule ID to delete (from the list action),required"`
}

// DeleteTargetBranchRule deletes a target branch rule by its ID.
// Premium/Ultimate. Destructive.
func DeleteTargetBranchRule(ctx context.Context, client *gitlabclient.Client, input DeleteTargetBranchRuleInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.RuleID == 0 {
		return errors.New("projectDeleteTargetBranchRule: rule_id is required. Use the list action to find target branch rule IDs")
	}
	_, err := client.GL().Projects.DeleteTargetBranchRule(input.RuleID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectDeleteTargetBranchRule", err)
	}
	return nil
}

// DeleteTargetBranchRuleOutput deletes a target branch rule and returns the
// legacy success-message shape used by other destructive project actions.
func DeleteTargetBranchRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteTargetBranchRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTargetBranchRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted target branch rule %d.", input.RuleID)}, nil
}
