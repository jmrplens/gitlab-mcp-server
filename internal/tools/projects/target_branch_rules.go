package projects

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Full project path such as my-group/my-project. The target branch rules GraphQL query requires the full path and does not accept a numeric ID,required"`
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
	captured, capture := gitlabclient.WithResponseCapture(ctx)
	rules, _, err := client.GL().Projects.ListProjectTargetBranchRules(input.ProjectID.String(), gl.WithContext(captured))
	if err != nil {
		return ListTargetBranchRulesOutput{}, toolutil.WrapErrWithStatusHint(
			opListTargetBranchRules, refusalOr(opListTargetBranchRules, err, capture), http.StatusNotFound,
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
		return TargetBranchRuleOutput{}, errors.New("projectCreateTargetBranchRule: project_id must be a numeric project ID for this action; use project.get to resolve a path to its ID")
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
	captured, capture := gitlabclient.WithResponseCapture(ctx)
	rule, resp, err := client.GL().Projects.CreateTargetBranchRule(pid, opts, gl.WithContext(captured))
	if err != nil {
		// A creation GitLab ran and answered without the rule is one a
		// fine-grained session is told probably committed (issue 1103): at
		// 19.4.1 ProjectTargetBranchRule declares no fine-grained permission,
		// and GitLab checks the payload's rule only after it exists. client-go
		// reports that null and a refusal alike, as its not-found sentinel, so
		// the refusal is read from the answer first.
		err = refusalOr(opCreateTargetBranchRule, err, capture)
		return TargetBranchRuleOutput{}, toolutil.UnconfirmedWrite(client, resp, opCreateTargetBranchRule, objectTargetBranchRule, err,
			toolutil.WrapErrWithMessage(opCreateTargetBranchRule, err))
	}
	return targetBranchRuleToOutput(rule), nil
}

// The operations the target branch rule handlers name in their errors, and
// the rule in a reader's words, for the answer a fine-grained session gets
// when GitLab ran a creation and answered without it.
const (
	opListTargetBranchRules  = "projectListTargetBranchRules"
	opCreateTargetBranchRule = "projectCreateTargetBranchRule"
	opDeleteTargetBranchRule = "projectDeleteTargetBranchRule"
	objectTargetBranchRule   = "target branch rule"
)

// refusalOr returns the refusal GitLab answered a target branch rule request
// with, read from the captured answer, or err when it answered none.
//
// client-go's three target branch rule methods decode the top-level errors
// GitLab refuses a request with and never read them, so a refused list and a
// refused creation reach a caller as the not-found sentinel the null beside
// the errors becomes, and a refused delete as a success (row 103 of
// docs/development/upstream-bugs.md). The capture holds the same bytes the
// SDK decoded (ADR-0021). An answer that is not GraphQL's, a 404 page of the
// endpoint among them, carries no refusal of GraphQL's, so err stands.
func refusalOr(operation string, err error, capture *gitlabclient.ResponseCapture) error {
	var answer struct {
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	if capture.Decode(&answer) != nil {
		return err
	}
	if refusal := toolutil.GraphQLTopLevelError(operation, answer.Errors); refusal != nil {
		return refusal
	}
	return err
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
	captured, capture := gitlabclient.WithResponseCapture(ctx)
	_, err := client.GL().Projects.DeleteTargetBranchRule(input.RuleID, gl.WithContext(captured))
	if err == nil {
		// client-go answers a refused delete as a success, so the refusal is
		// read from the answer.
		err = refusalOr(opDeleteTargetBranchRule, nil, capture)
	}
	if err != nil {
		return toolutil.WrapErrWithMessage(opDeleteTargetBranchRule, err)
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
