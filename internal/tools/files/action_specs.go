package files

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for repository file actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		fileReadSpec("file_get", fileGetRoute(client), "gitlab_file_get"),
		fileCreateSpec("file_create", toolutil.RouteAction(client, Create), "gitlab_file_create"),
		fileUpdateSpec("file_update", toolutil.RouteAction(client, Update), "gitlab_file_update"),
		fileDeleteSpec("file_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_file_delete"),
		fileReadSpec("file_blame", toolutil.RouteAction(client, Blame), "gitlab_file_blame"),
		fileReadSpec("file_metadata", toolutil.RouteAction(client, GetMetaData), "gitlab_file_metadata"),
		fileReadSpec("file_raw", toolutil.RouteAction(client, GetRaw), "gitlab_file_raw"),
		fileReadSpec("file_raw_metadata", toolutil.RouteAction(client, GetRawFileMetaData), "gitlab_file_raw_metadata"),
	}
}

func fileGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return fileNotFoundOutput{Identifier: fmt.Sprintf("%q in project %v", input["file_path"], input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("file %q from project %s", input.FilePath, input.ProjectID))
	return out, nil
}

func fileReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, fileOptions(individualTool))
}

func fileCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, fileOptions(individualTool))
}

func fileUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, fileOptions(individualTool))
}

func fileDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, fileOptions(individualTool))
}

func fileOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute files domain action.", Tags: []string{"repository", "file"},
		RelatedActions: []string{"repository.tree", "repository.commit_list"},
		OpenWorld:      true,
		OwnerPackage:   "files",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := fileActionMeta[individualTool]; ok {
		options.Aliases = meta.aliases
		options.Usage = meta.usage
		options.IndividualTool.Description = meta.description
	}
	switch individualTool {
	case "gitlab_file_create", "gitlab_file_update":
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("encoding", map[string]any{"enum": []any{"text", "base64"}}),
		}
	}
	return options
}

// fileActionMeta supplies non-generic discovery metadata (natural-language
// aliases, usage guidance, and the "Returns: … See also: …" individual-tool
// description) for each repository file action, mirroring the issues domain
// R-META form.
var fileActionMeta = map[string]struct {
	aliases     []string
	usage       string
	description string
}{
	"gitlab_file_get": {
		aliases:     []string{"get file", "read file", "show file at path", "fetch file"},
		usage:       "Read one file's content and metadata by project_id and file_path, optionally at a specific ref. Use when the user wants the contents of a known file; base64 content is decoded automatically.",
		description: "Get a single file's decoded content and metadata from a repository. Returns: file_name, file_path, size, encoding, content, content_sha256, ref, blob_id, commit_id, last_commit_id, and execute_filemode. See also: gitlab_file_raw, gitlab_file_metadata, gitlab_file_blame, gitlab_repository_tree.",
	},
	"gitlab_file_create": {
		aliases:     []string{"create file", "add file", "commit new file"},
		usage:       "Create a new file at file_path on branch with content and a commit_message. Use start_branch to create the target branch from another ref.",
		description: "Create a new file in a repository in a single commit. Returns: file_path, branch, commit_id, and last_commit_id. See also: gitlab_file_update, gitlab_file_get, gitlab_file_delete.",
	},
	"gitlab_file_update": {
		aliases:     []string{"update file", "edit file", "modify file", "commit file change"},
		usage:       "Update an existing file at file_path on branch with new content and a commit_message. Pass last_commit_id for optimistic locking to avoid overwriting concurrent changes.",
		description: "Update an existing file in a repository in a single commit. Returns: file_path, branch, commit_id, and last_commit_id. See also: gitlab_file_create, gitlab_file_get, gitlab_file_delete.",
	},
	"gitlab_file_delete": {
		aliases:     []string{"delete file", "remove file", "commit file deletion"},
		usage:       "Delete a file at file_path on branch with a commit_message. Destructive: requires confirmation. Pass last_commit_id for optimistic locking.",
		description: "Delete a file from a repository in a single commit. Returns: a success confirmation naming the file and project. See also: gitlab_file_get, gitlab_file_update.",
	},
	"gitlab_file_blame": {
		aliases:     []string{"file blame", "git blame", "who changed this file", "line authorship"},
		usage:       "Get per-line blame for a file, optionally restricted to a line range with range_start and range_end. Use to find which commit and author last touched each line.",
		description: "Get blame information for a file, optionally for a line range. Returns: file_path and blame ranges, each with commit id, message, author name/email, authored and committed dates, and the covered lines. See also: gitlab_file_get, gitlab_repository_commit_list.",
	},
	"gitlab_file_metadata": {
		aliases:     []string{"file metadata", "file info", "file stat", "check file exists"},
		usage:       "Get a file's metadata without its content via a HEAD request. Use to check existence or read size, encoding, blob and commit IDs cheaply.",
		description: "Get a file's metadata without content (HEAD request). Returns: file_name, file_path, size, encoding, ref, blob_id, commit_id, last_commit_id, execute_filemode, and content_sha256. See also: gitlab_file_get, gitlab_file_raw_metadata.",
	},
	"gitlab_file_raw": {
		aliases:     []string{"raw file", "download file", "get raw content"},
		usage:       "Get a file's raw bytes from the raw endpoint at an optional ref. Set lfs to fetch the target of an LFS pointer instead of the pointer file.",
		description: "Get the raw content of a file from a repository. Returns: file_path, size, content, and content_category. See also: gitlab_file_get, gitlab_file_raw_metadata.",
	},
	"gitlab_file_raw_metadata": {
		aliases:     []string{"raw file metadata", "raw file info", "raw file stat"},
		usage:       "Get a raw file's metadata without content via a HEAD request to the raw endpoint. Set lfs to resolve an LFS pointer's target. Use to check existence efficiently.",
		description: "Get raw file metadata without content (HEAD request to the raw endpoint). Returns: file_name, file_path, size, encoding, ref, blob_id, commit_id, last_commit_id, execute_filemode, and content_sha256. See also: gitlab_file_raw, gitlab_file_metadata.",
	},
}
