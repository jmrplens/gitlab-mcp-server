// register.go wires alertmanagement MCP tools to the MCP server.

package alertmanagement

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all alert management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_alert_metric_images",
		Title:       toolutil.TitleFromName("gitlab_list_alert_metric_images"),
		Description: "List metric images for a GitLab alert.\n\nReturns: JSON array of metric images with pagination.\n\nSee also: gitlab_get_error_tracking_settings, gitlab_upload_alert_metric_image",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListMetricImagesInput) (*mcp.CallToolResult, ListMetricImagesOutput, error) {
		start := time.Now()
		out, err := ListMetricImages(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_alert_metric_images", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_upload_alert_metric_image",
		Title:       toolutil.TitleFromName("gitlab_upload_alert_metric_image"),
		Description: "Upload a metric image for a GitLab alert. Provide either file_path (absolute path to a local image file) or content_base64 (base64-encoded content), not both.\n\nReturns: JSON with the uploaded metric image details.\n\nSee also: gitlab_list_alert_metric_images, gitlab_update_alert_metric_image",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UploadMetricImageInput) (*mcp.CallToolResult, MetricImageItem, error) {
		start := time.Now()
		out, err := UploadMetricImage(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_upload_alert_metric_image", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatImageMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_alert_metric_image",
		Title:       toolutil.TitleFromName("gitlab_update_alert_metric_image"),
		Description: "Update a metric image for a GitLab alert.\n\nReturns: JSON with the updated metric image details.\n\nSee also: gitlab_list_alert_metric_images, gitlab_upload_alert_metric_image",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateMetricImageInput) (*mcp.CallToolResult, MetricImageItem, error) {
		start := time.Now()
		out, err := UpdateMetricImage(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_alert_metric_image", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatImageMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_alert_metric_image",
		Title:       toolutil.TitleFromName("gitlab_delete_alert_metric_image"),
		Description: "Delete a metric image from a GitLab alert.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_alert_metric_images, gitlab_upload_alert_metric_image",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteMetricImageInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete alert metric image %d from project %s?", input.ImageID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteMetricImage(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_alert_metric_image", start, err)
		r, o, _ := toolutil.DeleteResult("alert metric image")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}

// RegisterMeta registers the gitlab_alert_management meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_metric_images":  toolutil.RouteAction(client, ListMetricImages),
		"upload_metric_image": toolutil.RouteAction(client, UploadMetricImage),
		"update_metric_image": toolutil.RouteAction(client, UpdateMetricImage),
		"delete_metric_image": toolutil.DestructiveVoidAction(client, DeleteMetricImage),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_alert_management",
		Title: toolutil.TitleFromName("gitlab_alert_management"),
		Description: `Manage alert metric images in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_metric_images: List metric images for an alert. Params: project_id (required), alert_iid (required, int)
- upload_metric_image: Upload a metric image (base64). Params: project_id (required), alert_iid (required, int), filename (required), content (required, base64), url, url_text
- update_metric_image: Update a metric image. Params: project_id (required), alert_iid (required, int), image_id (required, int), filename, url, url_text
- delete_metric_image: Delete a metric image. Params: project_id (required), alert_iid (required, int), image_id (required, int)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconAlert,
	}, toolutil.MakeMetaHandler("gitlab_alert_management", routes, nil))
}
