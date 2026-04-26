// register.go wires packages MCP tools to the MCP server.

package packages

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers tools for the GitLab Generic Packages API.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_publish",
		Title:       toolutil.TitleFromName("gitlab_package_publish"),
		Description: "Upload a single file to the GitLab Generic Package Registry. Provide either file_path (absolute local path) or content_base64 (base64-encoded content), not both. Returns the package file ID, size, SHA256, and the real download URL in the 'url' field. Use that 'url' value with gitlab_release_link_create to attach the package to a release — do NOT construct package URLs manually. To upload multiple files at once, use gitlab_package_publish_directory instead. To publish and link to a release in one step, use gitlab_package_publish_and_link.\n\nSee also: gitlab_package_publish_and_link, gitlab_package_list\n\nReturns: JSON with the published package details and download URL.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishInput) (*mcp.CallToolResult, PublishOutput, error) {
		start := time.Now()
		out, err := Publish(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_publish", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPublishMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_download",
		Title:       toolutil.TitleFromName("gitlab_package_download"),
		Description: "Download a file from the GitLab Generic Package Registry and save it to a local path. Returns the output path, file size, and SHA256 checksum.\n\nSee also: gitlab_package_list, gitlab_package_publish\n\nReturns: JSON with output path, file size, and SHA256 checksum.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadInput) (*mcp.CallToolResult, DownloadOutput, error) {
		start := time.Now()
		out, err := Download(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_download", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDownloadMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_list",
		Title:       toolutil.TitleFromName("gitlab_package_list"),
		Description: "List packages in a GitLab project. Can filter by name, version, type, and supports pagination and sorting.\n\nSee also: gitlab_package_file_list, gitlab_package_publish\n\nReturns: JSON array of packages with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_file_list",
		Title:       toolutil.TitleFromName("gitlab_package_file_list"),
		Description: "List files within a specific package. Returns file ID, name, size, and SHA256 for each file with pagination.\n\nSee also: gitlab_package_list, gitlab_package_download\n\nReturns: JSON array of package files with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input FileListInput) (*mcp.CallToolResult, FileListOutput, error) {
		start := time.Now()
		out, err := FileList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_file_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFileListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_delete",
		Title:       toolutil.TitleFromName("gitlab_package_delete"),
		Description: "Delete a package and all its files from the GitLab Package Registry. This action cannot be undone.\n\nSee also: gitlab_package_list, gitlab_package_file_delete\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := elicitation.ConfirmAction(ctx, req, fmt.Sprintf("Delete package %s from project %s?", input.PackageID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("package %s from project %s", input.PackageID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_file_delete",
		Title:       toolutil.TitleFromName("gitlab_package_file_delete"),
		Description: "Delete a single file from a package in the GitLab Package Registry. This action cannot be undone.\n\nSee also: gitlab_package_file_list, gitlab_package_delete\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input FileDeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := elicitation.ConfirmAction(ctx, req, fmt.Sprintf("Delete file %s from package %s in project %s?", input.PackageFileID, input.PackageID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := FileDelete(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_file_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("file %s from package %s in project %s", input.PackageFileID, input.PackageID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_publish_and_link",
		Title:       toolutil.TitleFromName("gitlab_package_publish_and_link"),
		Description: "Upload a file to the Generic Package Registry and create a release asset link pointing to it in one step (recommended over separate publish + link calls). Provide either file_path or content_base64 for the file content. The release identified by tag_name must already exist. Automatically uses the real download URL — no manual URL construction needed.\n\nIMPORTANT: link_name MUST be the exact filename (e.g. 'checksums.txt.asc'). NEVER add descriptive suffixes like '(GPG signature)' — tools like go-selfupdate match release asset names exactly.\n\nSee also: gitlab_package_publish, gitlab_create_release_link\n\nReturns: JSON with the published package details and release link URL.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishAndLinkInput) (*mcp.CallToolResult, PublishAndLinkOutput, error) {
		start := time.Now()
		out, err := PublishAndLink(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_publish_and_link", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPublishAndLinkMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_package_publish_directory",
		Title:       toolutil.TitleFromName("gitlab_package_publish_directory"),
		Description: "Batch-upload all matching files from a local directory to the Generic Package Registry. Walks the directory (non-recursive), filters by an optional glob pattern (e.g. *.tar.gz, *.exe), and publishes each file. Ideal for uploading release binaries or build artifacts in bulk. Returns the list of published files with checksums and URLs.\n\nSee also: gitlab_package_publish, gitlab_package_publish_and_link\n\nReturns: JSON with the list of published files, checksums, and URLs.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishDirInput) (*mcp.CallToolResult, PublishDirOutput, error) {
		start := time.Now()
		out, err := PublishDirectory(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_package_publish_directory", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPublishDirMarkdown(out)), out, err)
	})
}

// RegisterMeta registers the gitlab_package meta-tool with actions:
// publish, download, list, file_list, delete, file_delete, publish_and_link, publish_directory.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	publishAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[PublishInput](params)
		if err != nil {
			return nil, err
		}
		return Publish(ctx, nil, client, input)
	}
	downloadAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[DownloadInput](params)
		if err != nil {
			return nil, err
		}
		return Download(ctx, nil, client, input)
	}
	deleteAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[DeleteInput](params)
		if err != nil {
			return nil, err
		}
		return nil, Delete(ctx, nil, client, input)
	}
	fileDeleteAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[FileDeleteInput](params)
		if err != nil {
			return nil, err
		}
		return nil, FileDelete(ctx, nil, client, input)
	}
	publishAndLinkAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[PublishAndLinkInput](params)
		if err != nil {
			return nil, err
		}
		return PublishAndLink(ctx, nil, client, input)
	}
	publishDirAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[PublishDirInput](params)
		if err != nil {
			return nil, err
		}
		return PublishDirectory(ctx, nil, client, input)
	}

	routes := toolutil.ActionMap{
		"publish":           toolutil.Route(publishAction),
		"download":          toolutil.Route(downloadAction),
		"list":              toolutil.RouteAction(client, List),
		"file_list":         toolutil.RouteAction(client, FileList),
		"delete":            toolutil.DestructiveRoute(deleteAction),
		"file_delete":       toolutil.DestructiveRoute(fileDeleteAction),
		"publish_and_link":  toolutil.Route(publishAndLinkAction),
		"publish_directory": toolutil.Route(publishDirAction),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_package",
		Title: toolutil.TitleFromName("gitlab_package"),
		Description: `Manage GitLab Generic Package Registry. Use 'action' to specify the operation and 'params' for action-specific parameters.

Upload actions:
- publish: Upload a single file to the package registry. Provide either file_path or content_base64, not both. Params: project_id (required), package_name (required), package_version (required), file_name (required), file_path or content_base64 (one required), status (optional: default/hidden)
- publish_directory: Batch-upload all matching files from a local directory (e.g. release binaries, build artifacts). Params: project_id (required), package_name (required), package_version (required), directory_path (required), include_pattern (optional glob, e.g. *.tar.gz), status (optional)
- publish_and_link: Publish a file AND create a release asset link in one step. Params: project_id (required), package_name (required), package_version (required), file_name (required), file_path or content_base64 (one required), tag_name (required), link_name (optional), link_type (optional: package/runbook/image/other), status (optional)

Query actions:
- list: List packages in a project with optional filtering. Params: project_id (required), package_name, package_version, package_type (generic/npm/maven/etc.), order_by (name/created_at/version/type), sort (asc/desc), page, per_page
- file_list: List files within a specific package. Params: project_id (required), package_id (required), page, per_page
- download: Download a package file to a local path. Params: project_id (required), package_name (required), package_version (required), file_name (required), output_path (required)

Delete actions:
- delete: Delete an entire package and all its files (irreversible). Params: project_id (required), package_id (required)
- file_delete: Delete a single file from a package (irreversible). Params: project_id (required), package_id (required), package_file_id (required)

Common workflow: list → publish_directory → publish_and_link (for release binaries)`,
		Annotations: toolutil.DeriveAnnotationsWithTitle("gitlab_package", routes),
		Icons:       toolutil.IconPackage,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_package", routes, nil))
}
