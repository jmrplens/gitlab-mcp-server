package branchrules

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// queryListBranchRulesEE includes Enterprise-only fields (codeOwnerApprovalRequired,
// approvalRules, externalStatusChecks). Used when the resolved tier is
// Premium or Ultimate (GITLAB_MCP_TIER=premium/ultimate or a detected EE license).
const queryListBranchRulesEE = `
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
        endCursor
      }
    }
  }
}
`

// queryListBranchRulesCE uses only fields available on GitLab Community Edition.
const queryListBranchRulesCE = `
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
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`

// GraphQL response structs.
//
// The two documents select two shapes, so there are two node types, one per
// document, rather than one node carrying fields the Community document can
// never fill: a decoder declaring a field its document does not select holds
// a value that is always empty, which make check-graphql-shapes refuses.

// gqlBranchProtectionCE is the protection every edition reports.
type gqlBranchProtectionCE struct {
	AllowForcePush bool `json:"allowForcePush"`
}

// gqlBranchProtection is the protection the Enterprise document selects.
type gqlBranchProtection struct {
	gqlBranchProtectionCE
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

// gqlBranchRuleFields are the fields both documents select.
type gqlBranchRuleFields struct {
	Name                  string  `json:"name"`
	IsDefault             bool    `json:"isDefault"`
	IsProtected           bool    `json:"isProtected"`
	MatchingBranchesCount int     `json:"matchingBranchesCount"`
	CreatedAt             *string `json:"createdAt"`
	UpdatedAt             *string `json:"updatedAt"`
}

// gqlBranchRuleNodeCE is a branch rule as the Community document selects it.
type gqlBranchRuleNodeCE struct {
	gqlBranchRuleFields
	BranchProtection *gqlBranchProtectionCE `json:"branchProtection"`
}

// gqlBranchRuleNode is a branch rule as the Enterprise document selects it.
type gqlBranchRuleNode struct {
	gqlBranchRuleFields
	BranchProtection *gqlBranchProtection `json:"branchProtection"`
	ApprovalRules    *struct {
		Nodes []gqlApprovalRule `json:"nodes"`
	} `json:"approvalRules"`
	ExternalStatusChecks *struct {
		Nodes []gqlExternalStatusCheck `json:"nodes"`
	} `json:"externalStatusChecks"`
}

// branchRuleNode is what the list decodes a node as: either edition's shape,
// each knowing how to become the one output item.
type branchRuleNode interface {
	item() BranchRuleItem
}

// item converts the fields both editions select into a [BranchRuleItem],
// extracting the timestamps.
func (n gqlBranchRuleFields) item() BranchRuleItem {
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
	return item
}

// item converts a Community node into a [BranchRuleItem].
func (n gqlBranchRuleNodeCE) item() BranchRuleItem {
	item := n.gqlBranchRuleFields.item()
	if n.BranchProtection != nil {
		item.BranchProtection = &BranchProtection{AllowForcePush: n.BranchProtection.AllowForcePush}
	}
	return item
}

// item converts an Enterprise node into a [BranchRuleItem], with its approval
// rules and external status checks.
func (n gqlBranchRuleNode) item() BranchRuleItem {
	item := n.gqlBranchRuleFields.item()
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
//
// Pagination is forward-only because Project.branchRules is: it accepts first
// and after alone, and reports neither a previous page nor a start cursor.
type ListOutput struct {
	toolutil.HintableOutput
	Rules      []BranchRuleItem                        `json:"rules"`
	Pagination toolutil.GraphQLForwardPaginationOutput `json:"pagination"`
}

// List retrieves branch rules for a project via the GitLab GraphQL API.
// It selects the EE query (with approval rules, external status checks, and
// code owner approval) when the client is configured for Enterprise, otherwise
// it uses the CE-compatible query that omits EE-only fields.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectPath == "" {
		return ListOutput{}, errors.New("list_branch_rules: project_path is required")
	}

	if client.IsEnterprise() {
		return listWith[gqlBranchRuleNode](ctx, client, queryListBranchRulesEE, input)
	}
	return listWith[gqlBranchRuleNodeCE](ctx, client, queryListBranchRulesCE, input)
}

// listWith runs one branch rules document against the variables the input
// resolves to, decoding each node as N, the shape that document selects.
//
// The document is a parameter rather than picked here so that a test can hand
// it one declaring too little and prove the pagination guard refuses it. The
// alternative, a package-level variable a test reassigns, would put a document
// under a parallel neighbor's feet, and the race detector would report it as a
// data race rather than as this guard.
//
// Building the variables here rather than in the caller is what keeps the check
// honest: the pair is checked against the document this call will actually
// send, not against the one a tier decision happened to pick first.
func listWith[N branchRuleNode](ctx context.Context, client *gitlabclient.Client, query string, input ListInput) (ListOutput, error) {
	vars, err := input.Variables(query)
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_branch_rules: %w", err)
	}
	vars["projectPath"] = input.ProjectPath

	return doGraphQLList[N](ctx, client, query, vars, input.ProjectPath)
}

// gqlBranchRulesConnection holds the paginated list of branch rule nodes.
//
// Pagination is the forward half alone, because both documents select that
// half alone: Project.branchRules accepts first and after and nothing else.
type gqlBranchRulesConnection[N branchRuleNode] struct {
	Nodes    []N                                `json:"nodes"`
	PageInfo toolutil.GraphQLRawForwardPageInfo `json:"pageInfo"`
}

// gqlProjectBranchRules wraps the branch rules connection inside a project.
type gqlProjectBranchRules[N branchRuleNode] struct {
	BranchRules gqlBranchRulesConnection[N] `json:"branchRules"`
}

// gqlResponse is the GraphQL response envelope for branch rules, with each
// node decoded as N.
type gqlResponse[N branchRuleNode] struct {
	Data struct {
		Project *gqlProjectBranchRules[N] `json:"project"`
	} `json:"data"`
	Errors []toolutil.GraphQLError `json:"errors"`
}

// doGraphQLList executes a branch rules GraphQL query and returns the output.
func doGraphQLList[N branchRuleNode](ctx context.Context, client *gitlabclient.Client, query string, vars map[string]any, projectPath string) (ListOutput, error) {
	var resp gqlResponse[N]

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     query,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("list_branch_rules", err, "verify the project fullPath is correct and your token has read_api scope")
	}

	// GitLab answers a rejected document with HTTP 200 and a top-level errors
	// array, which client-go does not turn into an error, so a query the
	// instance refused would otherwise be reported as a missing project.
	if resp.Data.Project == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError("list_branch_rules", resp.Errors); graphQLErr != nil {
			return ListOutput{}, graphQLErr
		}
		return ListOutput{}, fmt.Errorf("list_branch_rules: project %q not found", projectPath)
	}

	items := make([]BranchRuleItem, 0, len(resp.Data.Project.BranchRules.Nodes))
	for _, n := range resp.Data.Project.BranchRules.Nodes {
		items = append(items, n.item())
	}

	return ListOutput{
		Rules:      items,
		Pagination: toolutil.ForwardPageInfoToOutput(resp.Data.Project.BranchRules.PageInfo),
	}, nil
}
