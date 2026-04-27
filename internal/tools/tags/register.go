// register.go wires tags MCP tools to the MCP server.

package tags

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers Git tag tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_get",
		Title:       toolutil.TitleFromName("gitlab_tag_get"),
		Description: "Retrieve detailed information about a single Git tag. Returns tag name, target commit SHA, annotation message, and protection status.\n\nReturns: name, target, message, protected status, and commit details. See also: gitlab_tag_create, gitlab_release_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_tag_get", start, nil)
			return toolutil.NotFoundResult("Tag", fmt.Sprintf("%q in project %s", input.TagName, input.ProjectID),
				"Use gitlab_tag_list with project_id to list tags",
				"Verify the tag name is spelled correctly (case-sensitive)",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_get", start, err)
		result := FormatOutputMarkdown(out)
		if err == nil && out.Name != "" && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/tag/%s", url.PathEscape(string(input.ProjectID)), url.PathEscape(out.Name)),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_create",
		Title:       toolutil.TitleFromName("gitlab_tag_create"),
		Description: "Create a Git tag in a GitLab project pointing to a ref (branch, tag, or SHA). Optionally include an annotation message for an annotated tag. Returns: tag name, target, message, protected status, and commit SHA. See also: gitlab_release_create, gitlab_tag_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_create", start, err)
		return toolutil.WithHints(FormatOutputMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_delete",
		Title:       toolutil.TitleFromName("gitlab_tag_delete"),
		Description: "Delete a Git tag from a GitLab project. If a release is associated with the tag, the release is also removed.\n\nReturns: confirmation message. See also: gitlab_tag_list, gitlab_tag_create.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete tag %q from project %q? Associated releases will also be removed.", input.TagName, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("tag %q from project %s", input.TagName, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_list",
		Title:       toolutil.TitleFromName("gitlab_tag_list"),
		Description: "List Git tags in a GitLab project. Supports search by name pattern, ordering by name/updated/version, and sort direction (asc/desc). Returns paginated results.\n\nReturns: paginated list of tags with name, target SHA, message, and protected status. See also: gitlab_tag_get, gitlab_tag_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_get_signature",
		Title:       toolutil.TitleFromName("gitlab_tag_get_signature"),
		Description: "Get the X.509 signature of a tag. Returns signature type, verification status, and certificate details.\n\nReturns: signature_type, verification_status, and certificate details.\n\nSee also: gitlab_tag_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SignatureInput) (*mcp.CallToolResult, SignatureOutput, error) {
		start := time.Now()
		out, err := GetSignature(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_get_signature", start, err)
		return toolutil.WithHints(FormatSignatureMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_list_protected",
		Title:       toolutil.TitleFromName("gitlab_tag_list_protected"),
		Description: "List protected tags in a GitLab project with their create access levels. Returns paginated results.\n\nReturns: paginated list of protected tags with name and create access levels. See also: gitlab_tag_protect, gitlab_tag_get_protected.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProtectedTagsInput) (*mcp.CallToolResult, ListProtectedTagsOutput, error) {
		start := time.Now()
		out, err := ListProtectedTags(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_list_protected", start, err)
		return toolutil.WithHints(FormatListProtectedTagsMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_get_protected",
		Title:       toolutil.TitleFromName("gitlab_tag_get_protected"),
		Description: "Get a single protected tag by name. Returns: tag name and create access levels (access_level, user_id, group_id per entry). See also: gitlab_tag_list_protected, gitlab_tag_protect.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProtectedTagInput) (*mcp.CallToolResult, ProtectedTagOutput, error) {
		start := time.Now()
		out, err := GetProtectedTag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_get_protected", start, err)
		return toolutil.WithHints(FormatProtectedTagMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_protect",
		Title:       toolutil.TitleFromName("gitlab_tag_protect"),
		Description: "Protect a repository tag or wildcard pattern. Optionally set create access level or granular permissions (user, group, deploy key). Returns: protected tag name and create access levels. See also: gitlab_tag_unprotect, gitlab_tag_list_protected.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectTagInput) (*mcp.CallToolResult, ProtectedTagOutput, error) {
		start := time.Now()
		out, err := ProtectTag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_protect", start, err)
		return toolutil.WithHints(FormatProtectedTagMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_tag_unprotect",
		Title:       toolutil.TitleFromName("gitlab_tag_unprotect"),
		Description: "Remove protection from a repository tag. The tag itself is not deleted.\n\nReturns: confirmation message. See also: gitlab_tag_protect, gitlab_tag_list_protected.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectTagInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Unprotect tag %q from project %s?", input.TagName, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := UnprotectTag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_tag_unprotect", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("tag protection for %q in project %s", input.TagName, input.ProjectID))
	})
}
