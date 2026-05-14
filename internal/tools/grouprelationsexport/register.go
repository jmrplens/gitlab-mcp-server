package grouprelationsexport

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all group relations export tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	groupRelationsExportTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconImport})
	}

	mcp.AddTool(server, groupRelationsExportTool("gitlab_schedule_group_relations_export", "Schedule a new group relations export.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_group_relations_export_status, gitlab_schedule_group_export"), func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleExportInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := ScheduleExport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_group_relations_export", start, err)
		r, o, _ := toolutil.DeleteResult("group relations export")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})

	mcp.AddTool(server, groupRelationsExportTool("gitlab_list_group_relations_export_status", "List the status of group relations exports.\n\nReturns: JSON array of export statuses.\n\nSee also: gitlab_schedule_group_relations_export, gitlab_schedule_group_export"), func(ctx context.Context, req *mcp.CallToolRequest, input ListExportStatusInput) (*mcp.CallToolResult, *ListExportStatusOutput, error) {
		start := time.Now()
		out, err := ListExportStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_relations_export_status", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListExportStatus(out)), out, nil)
	})
}
