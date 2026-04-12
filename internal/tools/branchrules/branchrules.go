// Package branchrules implements MCP tool handlers for GitLab Branch Rules
// retrieval using the GraphQL API. Branch Rules provide an aggregated read-only
// view of branch protections, approval rules, and external status checks.
// Individual protected branch management continues to use REST via existing packages.
package branchrules

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// BranchRuleItem represents a branch rule summary.
type BranchRuleItem struct {
	Name                  string                `json:"name"`
	IsDefault             bool                  `json:"is_default"`
	IsProtected           bool                  `json:"is_protected"`
	MatchingBranchesCount int                   `json:"matching_branches_count"`
	CreatedAt             string                `json:"created_at,omitempty"`
	UpdatedAt             string                `json:"updated_at,omitempty"`
	BranchProtection      *BranchProtection     `json:"branch_protection,omitempty"`
	ApprovalRules         []ApprovalRule        `json:"approval_rules,omitempty"`
	ExternalStatusChecks  []ExternalStatusCheck `json:"external_status_checks,omitempty"`
}

// BranchProtection holds protection settings for a branch rule.
type BranchProtection struct {
	AllowForcePush            bool `json:"allow_force_push"`
	CodeOwnerApprovalRequired bool `json:"code_owner_approval_required"`
}

// ApprovalRule represents an approval rule associated with a branch rule.
type ApprovalRule struct {
	Name              string `json:"name"`
	ApprovalsRequired int    `json:"approvals_required"`
	Type              string `json:"type,omitempty"`
}

// ExternalStatusCheck represents an external status check on a branch rule.
type ExternalStatusCheck struct {
	Name        string `json:"name"`
	ExternalURL string `json:"external_url"`
}

// GraphQL query.

const queryListBranchRules = `
query($projectPath: ID!, $first: Int!, $after: String) {
  project(fullPath: $projectPath) {
    branchRules(first: $first, after: $after) {
      nodes {
        name
        isDefault
        isProtected
        matchingBranchesCount
        createdAt
        updatedAt
        branchProtection {
          allowForcePush
          codeOwnerApprovalRequired
        }
        approvalRules {
          nodes {
            name
            approvalsRequired
            type
          }
        }
        externalStatusChecks {
          nodes {
            name
            externalUrl
          }
        }
      }
      pageInfo {
        hasNextPage
        hasPreviousPage
        endCursor
        startCursor
      }
    }
  }
}
`

// GraphQL response structs.

type gqlBranchProtection struct {
	AllowForcePush            bool `json:"allowForcePush"`
	CodeOwnerApprovalRequired bool `json:"codeOwnerApprovalRequired"`
}

type gqlApprovalRule struct {
	Name              string  `json:"name"`
	ApprovalsRequired int     `json:"approvalsRequired"`
	Type              *string `json:"type"`
}

type gqlExternalStatusCheck struct {
	Name        string `json:"name"`
	ExternalURL string `json:"externalUrl"`
}

type gqlBranchRuleNode struct {
	Name                  string               `json:"name"`
	IsDefault             bool                 `json:"isDefault"`
	IsProtected           bool                 `json:"isProtected"`
	MatchingBranchesCount int                  `json:"matchingBranchesCount"`
	CreatedAt             *string              `json:"createdAt"`
	UpdatedAt             *string              `json:"updatedAt"`
	BranchProtection      *gqlBranchProtection `json:"branchProtection"`
	ApprovalRules         *struct {
		Nodes []gqlApprovalRule `json:"nodes"`
	} `json:"approvalRules"`
	ExternalStatusChecks *struct {
		Nodes []gqlExternalStatusCheck `json:"nodes"`
	} `json:"externalStatusChecks"`
}

// nodeToItem converts a raw GraphQL branch rule node into a [BranchRuleItem]
// output struct, extracting timestamps, approval rules, and external status checks.
func nodeToItem(n gqlBranchRuleNode) BranchRuleItem {
	item := BranchRuleItem{
		Name:                  n.Name,
		IsDefault:             n.IsDefault,
		IsProtected:           n.IsProtected,
		MatchingBranchesCount: n.MatchingBranchesCount,
	}
	if n.CreatedAt != nil {
		item.CreatedAt = *n.CreatedAt
	}
	if n.UpdatedAt != nil {
		item.UpdatedAt = *n.UpdatedAt
	}
	if n.BranchProtection != nil {
		item.BranchProtection = &BranchProtection{
			AllowForcePush:            n.BranchProtection.AllowForcePush,
			CodeOwnerApprovalRequired: n.BranchProtection.CodeOwnerApprovalRequired,
		}
	}
	if n.ApprovalRules != nil {
		for _, ar := range n.ApprovalRules.Nodes {
			rule := ApprovalRule{
				Name:              ar.Name,
				ApprovalsRequired: ar.ApprovalsRequired,
			}
			if ar.Type != nil {
				rule.Type = *ar.Type
			}
			item.ApprovalRules = append(item.ApprovalRules, rule)
		}
	}
	if n.ExternalStatusChecks != nil {
		for _, esc := range n.ExternalStatusChecks.Nodes {
			item.ExternalStatusChecks = append(item.ExternalStatusChecks, ExternalStatusCheck(esc))
		}
	}
	return item
}

// List.

// ListInput is the input for listing branch rules.
type ListInput struct {
	ProjectPath string `json:"project_path" jsonschema:"required,Project full path (e.g. my-group/my-project)"`
	toolutil.GraphQLPaginationInput
}

// ListOutput is the output for listing branch rules.
type ListOutput struct {
	toolutil.HintableOutput
	Rules      []BranchRuleItem                 `json:"rules"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// List retrieves branch rules for a project via the GitLab GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectPath == "" {
		return ListOutput{}, errors.New("list_branch_rules: project_path is required")
	}

	vars := input.GraphQLPaginationInput.Variables()
	vars["projectPath"] = input.ProjectPath

	var resp struct {
		Data struct {
			Project *struct {
				BranchRules struct {
					Nodes    []gqlBranchRuleNode         `json:"nodes"`
					PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
				} `json:"branchRules"`
			} `json:"project"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListBranchRules,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_branch_rules", err)
	}

	if resp.Data.Project == nil {
		return ListOutput{}, fmt.Errorf("list_branch_rules: project %q not found", input.ProjectPath)
	}

	items := make([]BranchRuleItem, 0, len(resp.Data.Project.BranchRules.Nodes))
	for _, n := range resp.Data.Project.BranchRules.Nodes {
		items = append(items, nodeToItem(n))
	}

	return ListOutput{
		Rules:      items,
		Pagination: toolutil.PageInfoToOutput(resp.Data.Project.BranchRules.PageInfo),
	}, nil
}
