// register.go wires mrapprovals MCP tools to the MCP server.

package mrapprovals

import (
	"context"
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers all MR approval tools on the given MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approval_state",
		Title:       toolutil.TitleFromName("gitlab_mr_approval_state"),
		Description: "Get the overall approval state of a merge request: whether it is approved, which rules are satisfied or pending, and whether rules have been overridden. This is the primary tool for checking 'can this MR be merged?'. For editing approval rules, use gitlab_mr_approval_rules. For who approved, use gitlab_mr_approval_config.\n\nReturns: JSON with the overall approval state and rule satisfaction status. See also: gitlab_mr_get, gitlab_mr_approval_rules.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StateInput) (*mcp.CallToolResult, StateOutput, error) {
		start := time.Now()
		out, err := State(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approval_state", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStateMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approval_rules",
		Title:       toolutil.TitleFromName("gitlab_mr_approval_rules"),
		Description: "List the approval rules configured for a merge request: rule names, types (regular, code_owner, any_approver), required approval count, current approvers, and eligible approvers. Use this to understand the rule configuration. For overall approval status, use gitlab_mr_approval_state instead.\n\nReturns: JSON with approval rules, required counts, and eligible approvers. See also: gitlab_mr_approval_config.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RulesInput) (*mcp.CallToolResult, RulesOutput, error) {
		start := time.Now()
		out, err := Rules(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approval_rules", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRulesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approval_config",
		Title:       toolutil.TitleFromName("gitlab_mr_approval_config"),
		Description: "Get the approval configuration for a merge request: total required approvals, list of users who already approved, suggested approvers, and whether the current user has approved. Use this to see who approved. For rule-level status, use gitlab_mr_approval_state.\n\nReturns: JSON with approval configuration, approvers, and approval status. See also: gitlab_mr_get, gitlab_mr_approval_state.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ConfigInput) (*mcp.CallToolResult, ConfigOutput, error) {
		start := time.Now()
		out, err := Config(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approval_config", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatConfigMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approval_reset",
		Title:       toolutil.TitleFromName("gitlab_mr_approval_reset"),
		Description: "Reset all approvals on a GitLab merge request. Requires project_id and merge_request_iid.\n\nReturns: confirmation message.\n\nSee also: gitlab_mr_approval_rules.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResetInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Reset(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approval_reset", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("MR approvals")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approval_rule_create",
		Title:       toolutil.TitleFromName("gitlab_mr_approval_rule_create"),
		Description: "Create an approval rule on a GitLab merge request. Specify the rule name, required approvals, and optionally user/group IDs.\n\nReturns: JSON with the created approval rule details. See also: gitlab_mr_approval_rules.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateRuleInput) (*mcp.CallToolResult, RuleOutput, error) {
		start := time.Now()
		out, err := CreateRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approval_rule_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approval_rule_update",
		Title:       toolutil.TitleFromName("gitlab_mr_approval_rule_update"),
		Description: "Update an existing approval rule on a GitLab merge request. Modify the rule name, required approvals, or user/group IDs.\n\nReturns: JSON with the updated approval rule details. See also: gitlab_mr_approval_rules.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateRuleInput) (*mcp.CallToolResult, RuleOutput, error) {
		start := time.Now()
		out, err := UpdateRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approval_rule_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approval_rule_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_approval_rule_delete"),
		Description: "Delete an approval rule from a GitLab merge request. Requires project_id, merge_request_iid, and approval_rule_id.\n\nReturns: confirmation message. See also: gitlab_mr_approval_rules.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteRuleInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete approval rule %d from MR !%d in project %q?", input.ApprovalRuleID, input.MRIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approval_rule_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("approval rule")
	})
}
