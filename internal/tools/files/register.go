// register.go wires files MCP tools to the MCP server.

package files

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all file-related MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_get",
		Title:       toolutil.TitleFromName("gitlab_file_get"),
		Description: "Retrieve the decoded content of a single file from a GitLab repository at a specific ref (branch name, tag name, or commit SHA). Returns file content, size, encoding, and last commit ID. This is the primary tool for reading file contents. For raw text without metadata use gitlab_file_raw. For metadata only (no content) use gitlab_file_metadata.\n\nReturns: file_name, file_path, size, encoding, content, ref, blob_id, commit_id, and last_commit_id. See also: gitlab_file_create, gitlab_repository_tree.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_file_get", start, nil)
			return toolutil.NotFoundResult("File", fmt.Sprintf("%q in project %s", input.FilePath, input.ProjectID),
				"Use gitlab_repository_tree to browse the repository",
				"Verify the file_path and ref are correct (paths are case-sensitive)",
				"The file may not exist in the specified branch or ref",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_get", start, err)
		return toolutil.WithHints(fileGetResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_create",
		Title:       toolutil.TitleFromName("gitlab_file_create"),
		Description: "Create a new file in a GitLab repository. Requires branch and commit message. Optionally specify encoding (text/base64), start_branch, and author info. Returns: file_path and branch. See also: gitlab_commit_create, gitlab_file_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, FileInfoOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFileInfoMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_update",
		Title:       toolutil.TitleFromName("gitlab_file_update"),
		Description: "Update an existing file in a GitLab repository. Requires branch and commit message. Supports last_commit_id for optimistic locking. Returns: file_path and branch. See also: gitlab_file_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, FileInfoOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFileInfoMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_delete",
		Title:       toolutil.TitleFromName("gitlab_file_delete"),
		Description: "Delete a file from a GitLab repository. Requires branch and commit message. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_file_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete file %q from project %s?", input.FilePath, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("file %q from project %s", input.FilePath, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_blame",
		Title:       toolutil.TitleFromName("gitlab_file_blame"),
		Description: "Get blame information for a file in a GitLab repository. Shows which commit and author last modified each line range. Supports optional line range filtering. Returns: file_path and blame ranges with commit ID, author, message, and lines. See also: gitlab_file_get, gitlab_commit_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BlameInput) (*mcp.CallToolResult, BlameOutput, error) {
		start := time.Now()
		out, err := Blame(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_blame", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBlameMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_metadata",
		Title:       toolutil.TitleFromName("gitlab_file_metadata"),
		Description: "Get file metadata WITHOUT retrieving content. Use this to check file existence or properties; for content use gitlab_file_get. Returns: file_name, size, encoding, blob_id, commit_id, last_commit_id, and SHA-256.\n\nSee also: gitlab_file_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MetaDataInput) (*mcp.CallToolResult, MetaDataOutput, error) {
		start := time.Now()
		out, err := GetMetaData(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_metadata", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMetaDataMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_raw",
		Title:       toolutil.TitleFromName("gitlab_file_raw"),
		Description: "Get the raw content of a file from a GitLab repository as plain text without metadata. Use gitlab_file_get if you also need metadata. Returns: file_path, size, and content as plain text. See also: gitlab_file_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RawInput) (*mcp.CallToolResult, RawOutput, error) {
		start := time.Now()
		out, err := GetRaw(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_raw", start, err)
		return toolutil.WithHints(fileRawResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_file_raw_metadata",
		Title:       toolutil.TitleFromName("gitlab_file_raw_metadata"),
		Description: "Get file metadata via HEAD request to the raw file endpoint. Returns size, encoding, blob ID, commit IDs, and SHA-256 without retrieving content. Lighter than gitlab_file_metadata — uses a HEAD request instead of GET.\n\nReturns: file_name, size, encoding, blob_id, commit_id, last_commit_id, and SHA-256.\n\nSee also: gitlab_file_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RawMetaDataInput) (*mcp.CallToolResult, MetaDataOutput, error) {
		start := time.Now()
		out, err := GetRawFileMetaData(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_file_raw_metadata", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMetaDataMarkdown(out)), out, err)
	})
}

// fileGetResult builds a CallToolResult based on the content category of a file.
// Images return ImageContent (visible to multimodal LLMs), binary files return
// metadata only, and text files return the decoded content with metadata.
func fileGetResult(out Output) *mcp.CallToolResult {
	md := FormatOutputMarkdown(out)
	switch out.ContentCategory {
	case "image":
		return toolutil.ToolResultWithImage(md, toolutil.ContentDetail, out.ImageData, out.ImageMIMEType)
	case "binary":
		return toolutil.ToolResultAnnotated(md, toolutil.ContentDetail)
	default:
		return toolutil.ToolResultAnnotated(md, toolutil.ContentDetail)
	}
}

// fileRawResult builds a CallToolResult based on the content category of a raw file.
func fileRawResult(out RawOutput) *mcp.CallToolResult {
	switch out.ContentCategory {
	case "image":
		md := FormatRawImageMarkdown(out)
		return toolutil.ToolResultWithImage(md, toolutil.ContentAssistant, out.ImageData, out.ImageMIMEType)
	case "binary":
		md := FormatRawBinaryMarkdown(out)
		return toolutil.ToolResultAnnotated(md, toolutil.ContentAssistant)
	default:
		return toolutil.ToolResultAnnotated(FormatRawMarkdown(out), toolutil.ContentAssistant)
	}
}
