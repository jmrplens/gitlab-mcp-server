package vulnerabilities

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers vulnerability management tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	vulnerabilityTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconVulnerability})
	}

	mcp.AddTool(server, vulnerabilityTool("gitlab_list_vulnerabilities", "List project vulnerabilities (requires Ultimate/Premium). Supports filtering by severity, state, scanner, report type. Returns: paginated list with severity, state, scanner, and detected date. See also: gitlab_get_vulnerability, gitlab_vulnerability_severity_count, gitlab_list_security_findings."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_vulnerabilities", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, vulnerabilityTool("gitlab_get_vulnerability", "Get a single vulnerability by GID (requires Ultimate/Premium). Returns: full vulnerability details including identifiers, scanner, location, solution, and linked issues/MRs. See also: gitlab_list_vulnerabilities, gitlab_confirm_vulnerability, gitlab_dismiss_vulnerability."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_vulnerability", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, err)
	})

	mcp.AddTool(server, vulnerabilityTool("gitlab_dismiss_vulnerability", "Dismiss a vulnerability with optional comment and reason (requires Ultimate/Premium). Valid reasons: ACCEPTABLE_RISK, FALSE_POSITIVE, MITIGATING_CONTROL, USED_IN_TESTS, NOT_APPLICABLE. Returns: updated vulnerability state. See also: gitlab_get_vulnerability, gitlab_revert_vulnerability, gitlab_resolve_vulnerability."), func(ctx context.Context, req *mcp.CallToolRequest, input DismissInput) (*mcp.CallToolResult, MutationOutput, error) {
		start := time.Now()
		out, err := Dismiss(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_dismiss_vulnerability", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatMutationMarkdown(out, "dismissed"), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, vulnerabilityTool("gitlab_confirm_vulnerability", "Confirm a detected vulnerability (requires Ultimate/Premium). Changes state from DETECTED to CONFIRMED. Returns: updated vulnerability state. See also: gitlab_get_vulnerability, gitlab_dismiss_vulnerability, gitlab_resolve_vulnerability."), func(ctx context.Context, req *mcp.CallToolRequest, input ConfirmInput) (*mcp.CallToolResult, MutationOutput, error) {
		start := time.Now()
		out, err := Confirm(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_confirm_vulnerability", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatMutationMarkdown(out, "confirmed"), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, vulnerabilityTool("gitlab_resolve_vulnerability", "Resolve a vulnerability (requires Ultimate/Premium). Changes state to RESOLVED. Returns: updated vulnerability state. See also: gitlab_get_vulnerability, gitlab_confirm_vulnerability, gitlab_revert_vulnerability."), func(ctx context.Context, req *mcp.CallToolRequest, input ResolveInput) (*mcp.CallToolResult, MutationOutput, error) {
		start := time.Now()
		out, err := Resolve(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_resolve_vulnerability", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatMutationMarkdown(out, "resolved"), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, vulnerabilityTool("gitlab_revert_vulnerability", "Revert a vulnerability to detected state (requires Ultimate/Premium). Changes state back to DETECTED. Returns: updated vulnerability state. See also: gitlab_get_vulnerability, gitlab_dismiss_vulnerability, gitlab_resolve_vulnerability."), func(ctx context.Context, req *mcp.CallToolRequest, input RevertInput) (*mcp.CallToolResult, MutationOutput, error) {
		start := time.Now()
		out, err := Revert(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_revert_vulnerability", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatMutationMarkdown(out, "reverted to detected"), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, vulnerabilityTool("gitlab_vulnerability_severity_count", "Get vulnerability severity counts for a project (requires Ultimate/Premium). Returns: counts per severity level (critical, high, medium, low, info, unknown) and total. See also: gitlab_list_vulnerabilities, gitlab_pipeline_security_summary."), func(ctx context.Context, req *mcp.CallToolRequest, input SeverityCountInput) (*mcp.CallToolResult, SeverityCountOutput, error) {
		start := time.Now()
		out, err := SeverityCount(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_vulnerability_severity_count", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSeverityCountMarkdown(out)), out, err)
	})

	mcp.AddTool(server, vulnerabilityTool("gitlab_pipeline_security_summary", "Get security report summary for a pipeline (requires Ultimate/Premium). Returns: scanner-level breakdown (SAST, DAST, dependency scanning, container scanning, secret detection, coverage fuzzing, API fuzzing, cluster image scanning) with vulnerability counts. See also: gitlab_vulnerability_severity_count, gitlab_list_vulnerabilities, gitlab_list_security_findings."), func(ctx context.Context, req *mcp.CallToolRequest, input PipelineSecuritySummaryInput) (*mcp.CallToolResult, PipelineSecuritySummaryOutput, error) {
		start := time.Now()
		out, err := PipelineSecuritySummary(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_security_summary", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPipelineSecuritySummaryMarkdown(out)), out, err)
	})
}
