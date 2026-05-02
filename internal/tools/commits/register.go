package commits

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all commit-related MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_list",
		Title:       toolutil.TitleFromName("gitlab_commit_list"),
		Description: "List commits in a GitLab repository. Supports filtering by branch/tag (ref_name), date range (since/until in ISO 8601), file path, and author. Optionally includes commit stats (additions/deletions). Returns commit ID, title, author, date, and web URL with pagination.\n\nReturns: paginated list of commits with id, short_id, title, author_name, authored_date, and web_url. See also: gitlab_commit_get, gitlab_branch_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_create",
		Title:       toolutil.TitleFromName("gitlab_commit_create"),
		Description: "Create a Git commit with one or more file actions in a GitLab repository. Each action in the 'actions' array requires: 'action' (create/update/delete/move/chmod), 'file_path', and 'content' (required for create and update). For 'move' actions, also set 'previous_path'. Supports multi-file atomic commits on any branch with optional author override and start_branch to create new branches. Returns: id, short_id, title, author_name, committed_date, web_url. See also: gitlab_commit_get, gitlab_branch_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatOutputMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_get",
		Title:       toolutil.TitleFromName("gitlab_commit_get"),
		Description: "Retrieve a single commit by SHA from a GitLab project. Returns commit ID, title, full message, author/committer info, parent IDs, stats (additions/deletions/total), and web URL. See also: gitlab_commit_diff, gitlab_commit_merge_requests.\n\nReturns: id, short_id, title, message, author, committer, parent_ids, stats, and web_url.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, DetailOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_commit_get", start, nil)
			return toolutil.NotFoundResult("Commit", fmt.Sprintf("%s in project %s", input.SHA, input.ProjectID),
				"Use gitlab_commit_list with project_id to list recent commits",
				"Verify the SHA hash is correct and complete",
				"The commit may be in a different branch — try specifying ref_name in gitlab_commit_list",
			), DetailOutput{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_get", start, err)
		result := toolutil.ToolResultAnnotated(FormatDetailMarkdown(out), toolutil.ContentDetail)
		if err == nil && out.ID != "" && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/commit/%s", url.PathEscape(string(input.ProjectID)), out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_diff",
		Title:       toolutil.TitleFromName("gitlab_commit_diff"),
		Description: "List the diffs (changed files) for a specific commit in a GitLab project. Returns old/new paths, diff text, and flags for new/renamed/deleted files with pagination.\n\nReturns: paginated list of diffs with old_path, new_path, diff text, and new/renamed/deleted flags. See also: gitlab_commit_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DiffInput) (*mcp.CallToolResult, DiffOutput, error) {
		start := time.Now()
		out, err := Diff(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_diff", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDiffMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_refs",
		Title:       toolutil.TitleFromName("gitlab_commit_refs"),
		Description: "Get branches and tags a commit is pushed to. Returns ref type (branch/tag) and name. Supports filtering by type and pagination.\n\nReturns: paginated list of refs with type (branch/tag) and name. See also: gitlab_commit_get, gitlab_branch_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RefsInput) (*mcp.CallToolResult, RefsOutput, error) {
		start := time.Now()
		out, err := GetRefs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_refs", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRefsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_comments",
		Title:       toolutil.TitleFromName("gitlab_commit_comments"),
		Description: "List comments on a specific commit. Returns comment text, file path, line number, and author with pagination.\n\nReturns: paginated list of comments with note, path, line, and author. See also: gitlab_commit_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CommentsInput) (*mcp.CallToolResult, CommentsOutput, error) {
		start := time.Now()
		out, err := GetComments(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_comments", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCommentsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_comment_create",
		Title:       toolutil.TitleFromName("gitlab_commit_comment_create"),
		Description: "Post a comment on a commit. Supports file-level inline comments with path and line number.\n\nReturns: note, author, path, and line of the created comment. See also: gitlab_commit_comments.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PostCommentInput) (*mcp.CallToolResult, CommentOutput, error) {
		start := time.Now()
		out, err := PostComment(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_comment_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCommentMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_statuses",
		Title:       toolutil.TitleFromName("gitlab_commit_statuses"),
		Description: "List pipeline statuses of a commit. Returns status state, name, ref, description, and coverage. Supports filtering by ref, stage, name, and pipeline_id with pagination.\n\nReturns: paginated list of statuses with id, state, name, ref, description, and coverage. See also: gitlab_commit_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StatusesInput) (*mcp.CallToolResult, StatusesOutput, error) {
		start := time.Now()
		out, err := GetStatuses(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_statuses", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStatusesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_status_set",
		Title:       toolutil.TitleFromName("gitlab_commit_status_set"),
		Description: "Set the pipeline status of a commit. State can be: pending, running, success, failed, or canceled. Supports optional ref, name, target_url, description, coverage, and pipeline_id. Returns: id, sha, ref, status, name, description, coverage, pipeline_id. See also: gitlab_commit_statuses.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetStatusInput) (*mcp.CallToolResult, StatusOutput, error) {
		start := time.Now()
		out, err := SetStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_status_set", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStatusMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_merge_requests",
		Title:       toolutil.TitleFromName("gitlab_commit_merge_requests"),
		Description: "List merge requests associated with a commit. Returns MR IID, title, state, source/target branches, author, and web URL.\n\nReturns: list of merge requests with iid, title, state, source/target branches, author, and web_url. See also: gitlab_commit_get, gitlab_mr_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MRsByCommitInput) (*mcp.CallToolResult, MRsByCommitOutput, error) {
		start := time.Now()
		out, err := ListMRsByCommit(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_merge_requests", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMRsByCommitMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_cherry_pick",
		Title:       toolutil.TitleFromName("gitlab_commit_cherry_pick"),
		Description: "Cherry-pick a commit to a target branch. Supports dry_run to check for conflicts without creating the commit, and custom commit message. Returns: id, short_id, title, author_name, committed_date, web_url. See also: gitlab_commit_get, gitlab_branch_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CherryPickInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CherryPick(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_cherry_pick", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_revert",
		Title:       toolutil.TitleFromName("gitlab_commit_revert"),
		Description: "Revert a commit on a target branch. Creates a new commit that undoes the changes of the specified commit. Returns: id, short_id, title, author_name, committed_date, web_url. See also: gitlab_commit_get, gitlab_branch_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RevertInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Revert(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_revert", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_commit_signature",
		Title:       toolutil.TitleFromName("gitlab_commit_signature"),
		Description: "Get the GPG signature of a commit if it was signed. Returns verification status, key ID, user name, and email.\n\nReturns: verification_status, gpg_key_id, gpg_key_user_name, and gpg_key_user_email. See also: gitlab_commit_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GPGSignatureInput) (*mcp.CallToolResult, GPGSignatureOutput, error) {
		start := time.Now()
		out, err := GetGPGSignature(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_commit_signature", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGPGSignatureMarkdown(out)), out, err)
	})
}
