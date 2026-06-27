package snippets

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for personal and project snippet actions.
// Each spec is decorated with non-generic discovery metadata (usage, aliases,
// related actions, and a "Returns: … See also: …" individual-tool description)
// per the 1:1 audit R-META requirement so neither canonical action inherits the
// generic placeholder metadata from snippetOptions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	createRoute := toolutil.RouteAction(client, Create)
	createRoute.InputSchema = CreateInputSchemaMap()
	projectCreateRoute := toolutil.RouteAction(client, ProjectCreate)
	projectCreateRoute.InputSchema = ProjectCreateInputSchemaMap()

	return []toolutil.ActionSpec{
		snippetReadSpec("list", toolutil.RouteAction(client, List), "gitlab_snippet_list"),
		snippetReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_snippet_list_all"),
		snippetReadSpec("get", snippetGetRoute(client), "gitlab_snippet_get"),
		snippetReadSpec("content", toolutil.RouteAction(client, Content), "gitlab_snippet_content"),
		snippetReadSpec("file_content", toolutil.RouteAction(client, FileContent), "gitlab_snippet_file_content"),
		snippetCreateSpec("create", createRoute, "gitlab_snippet_create"),
		snippetUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_snippet_update"),
		snippetDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_snippet_delete"),
		snippetReadSpec("explore", toolutil.RouteAction(client, Explore), "gitlab_snippet_explore"),
		snippetReadSpec("project_list", toolutil.RouteAction(client, ProjectList), "gitlab_project_snippet_list"),
		snippetReadSpec("project_get", toolutil.RouteAction(client, ProjectGet), "gitlab_project_snippet_get"),
		snippetReadSpec("project_content", toolutil.RouteAction(client, ProjectContent), "gitlab_project_snippet_content"),
		snippetCreateSpec("project_create", projectCreateRoute, "gitlab_project_snippet_create"),
		snippetUpdateSpec("project_update", toolutil.RouteAction(client, ProjectUpdate), "gitlab_project_snippet_update"),
		snippetDeleteSpec("project_delete", toolutil.DestructiveVoidAction(client, ProjectDelete), "gitlab_project_snippet_delete"),
	}
}

func snippetGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return snippetNotFoundOutput{Identifier: fmt.Sprintf("ID %v", input["snippet_id"])}, nil
		}
		return result, err
	}
	return route
}

func snippetReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetOptions(individualTool)
	decorateSnippetMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func snippetCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetOptions(individualTool)
	decorateSnippetMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func snippetUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetOptions(individualTool)
	decorateSnippetMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func snippetDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetOptions(individualTool)
	decorateSnippetMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func snippetOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute snippets domain action.", Tags: []string{"snippet"},
		OpenWorld:      true,
		OwnerPackage:   "snippets",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateSnippetMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for a snippet action, replacing the generic placeholder metadata from
// snippetOptions. It is a no-op for tools with no metadata entry.
func decorateSnippetMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := snippetActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// snippetActionMetaEntry is the discovery metadata for one snippet action.
type snippetActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// Canonical action IDs referenced from RelatedActions, defined once to avoid
// stringly-typed drift across the metadata table.
const (
	actionSnippetGet            = "snippet.get"
	actionSnippetList           = "snippet.list"
	actionSnippetContent        = "snippet.content"
	actionSnippetProjectGet     = "snippet.project_get"
	actionSnippetProjectList    = "snippet.project_list"
	actionSnippetProjectContent = "snippet.project_content"
)

// snippetActionMeta maps each individual snippet tool to its discovery metadata.
var snippetActionMeta = map[string]snippetActionMetaEntry{
	"gitlab_snippet_list": {
		usage:       "List snippets owned by the authenticated user. Use order_by and sort for keyset ordering and page/per_page or page_token for pagination when the prompt asks for the caller's own snippets.",
		aliases:     []string{"list my snippets", "show my snippets", "list user snippets"},
		related:     []string{actionSnippetGet, "snippet.create", "snippet.explore"},
		description: "List snippets owned by the authenticated user. Returns: snippets with title, visibility, author, files, and pagination metadata. See also: gitlab_snippet_get, gitlab_snippet_create, gitlab_snippet_explore.",
	},
	"gitlab_snippet_list_all": {
		usage:       "List every snippet on the instance (admin only). Use created_after/created_before and repository_storage filters, plus order_by/sort and keyset pagination, for cross-instance audits.",
		aliases:     []string{"list all snippets", "admin list snippets", "list snippets across instance"},
		related:     []string{actionSnippetList, "snippet.explore", actionSnippetGet},
		description: "List all snippets on the instance (admin). Returns: snippets with author, visibility, repository storage, and pagination metadata. See also: gitlab_snippet_list, gitlab_snippet_explore, gitlab_snippet_get.",
	},
	"gitlab_snippet_get": {
		usage:       "Fetch one personal snippet by its global snippet_id. Use after a snippet list or when the prompt already names a concrete snippet ID; returns a structured not-found result on 404.",
		aliases:     []string{"get snippet", "show snippet details", "fetch snippet"},
		related:     []string{actionSnippetList, actionSnippetContent, "snippet.update", "snippet.delete"},
		description: "Get a single personal snippet by ID. Returns: snippet metadata, visibility, author, files, and web URL. See also: gitlab_snippet_list, gitlab_snippet_content, gitlab_snippet_update, gitlab_snippet_delete.",
	},
	"gitlab_snippet_content": {
		usage:       "Read the raw content of a single-file personal snippet by snippet_id. For multi-file snippets use snippet.file_content with a specific file path.",
		aliases:     []string{"get snippet content", "read snippet", "show snippet raw"},
		related:     []string{actionSnippetGet, "snippet.file_content"},
		description: "Read a personal snippet's raw content. Returns: the snippet ID and raw text content. See also: gitlab_snippet_get, gitlab_snippet_file_content.",
	},
	"gitlab_snippet_file_content": {
		usage:       "Read the raw content of one file inside a multi-file personal snippet at a given Git ref. Use when the snippet has multiple files and you need a specific one.",
		aliases:     []string{"get snippet file content", "read snippet file", "show snippet file at ref"},
		related:     []string{actionSnippetContent, actionSnippetGet},
		description: "Read one file from a multi-file personal snippet at a ref. Returns: the snippet ID, ref, file name, and raw content. See also: gitlab_snippet_content, gitlab_snippet_get.",
	},
	"gitlab_snippet_create": {
		usage:       "Create a personal snippet. Provide title plus either files[] (multi-file) or file_name and content (single-file). Set visibility to private, internal, or public.",
		aliases:     []string{"create snippet", "new snippet", "add personal snippet"},
		related:     []string{actionSnippetGet, "snippet.update", actionSnippetList},
		description: "Create a personal snippet. Returns: the created snippet with ID, visibility, author, files, and web URL. See also: gitlab_snippet_get, gitlab_snippet_update, gitlab_snippet_list.",
	},
	"gitlab_snippet_update": {
		usage:       "Update a personal snippet's metadata or files. Pass files[] with action set to create, update, delete, or move to modify content; use file_name/content only for single-file legacy snippets.",
		aliases:     []string{"update snippet", "edit snippet", "modify personal snippet"},
		related:     []string{actionSnippetGet, "snippet.delete", actionSnippetList},
		description: "Update a personal snippet. Returns: the updated snippet with metadata, files, and web URL. See also: gitlab_snippet_get, gitlab_snippet_delete, gitlab_snippet_list.",
	},
	"gitlab_snippet_delete": {
		usage:       "Permanently delete a personal snippet by snippet_id. Destructive; confirm the snippet ID before calling.",
		aliases:     []string{"delete snippet", "remove snippet", "destroy snippet", "drop snippet"},
		related:     []string{actionSnippetGet, actionSnippetList},
		description: "Delete a personal snippet permanently. Returns: a success confirmation. See also: gitlab_snippet_get, gitlab_snippet_list.",
	},
	"gitlab_snippet_explore": {
		usage:       "List public snippets visible across the instance. Use order_by/sort and keyset pagination to browse public snippets the user does not own.",
		aliases:     []string{"explore public snippets", "browse public snippets", "list public snippets"},
		related:     []string{actionSnippetList, actionSnippetGet},
		description: "List public snippets across the instance. Returns: public snippets with author, visibility, and pagination metadata. See also: gitlab_snippet_list, gitlab_snippet_get.",
	},
	"gitlab_project_snippet_list": {
		usage:       "List snippets belonging to one project. Provide project_id; use order_by/sort and keyset pagination when the prompt scopes snippets to a known project.",
		aliases:     []string{"list project snippets", "show project snippets", "find snippets in project"},
		related:     []string{actionSnippetProjectGet, "snippet.project_create", actionSnippetList},
		description: "List a project's snippets. Returns: snippets with title, visibility, author, files, and pagination metadata. See also: gitlab_project_snippet_get, gitlab_project_snippet_create, gitlab_snippet_list.",
	},
	"gitlab_project_snippet_get": {
		usage:       "Fetch one project snippet by project_id and snippet_id. Use after a project snippet list or when both identifiers are already known.",
		aliases:     []string{"get project snippet", "show project snippet details", "fetch project snippet"},
		related:     []string{actionSnippetProjectList, actionSnippetProjectContent, "snippet.project_update", "snippet.project_delete"},
		description: "Get a single project snippet. Returns: snippet metadata, visibility, author, files, and web URL. See also: gitlab_project_snippet_list, gitlab_project_snippet_content, gitlab_project_snippet_update, gitlab_project_snippet_delete.",
	},
	"gitlab_project_snippet_content": {
		usage:       "Read the raw content of a project snippet by project_id and snippet_id.",
		aliases:     []string{"get project snippet content", "read project snippet", "show project snippet raw"},
		related:     []string{actionSnippetProjectGet, actionSnippetProjectList},
		description: "Read a project snippet's raw content. Returns: the snippet ID and raw text content. See also: gitlab_project_snippet_get, gitlab_project_snippet_list.",
	},
	"gitlab_project_snippet_create": {
		usage:       "Create a project snippet. Provide project_id and title plus either files[] (multi-file) or file_name and content (single-file); set visibility to private, internal, or public.",
		aliases:     []string{"create project snippet", "new project snippet", "add snippet to project"},
		related:     []string{actionSnippetProjectGet, "snippet.project_update", actionSnippetProjectList},
		description: "Create a project snippet. Returns: the created snippet with ID, visibility, author, files, and web URL. See also: gitlab_project_snippet_get, gitlab_project_snippet_update, gitlab_project_snippet_list.",
	},
	"gitlab_project_snippet_update": {
		usage:       "Update a project snippet's metadata or files. Pass files[] with action set to create, update, delete, or move and use the file_path from project_get; use file_name/content only for single-file legacy snippets.",
		aliases:     []string{"update project snippet", "edit project snippet", "modify project snippet"},
		related:     []string{actionSnippetProjectGet, "snippet.project_delete", actionSnippetProjectList},
		description: "Update a project snippet. Returns: the updated snippet with metadata, files, and web URL. See also: gitlab_project_snippet_get, gitlab_project_snippet_delete, gitlab_project_snippet_list.",
	},
	"gitlab_project_snippet_delete": {
		usage:       "Permanently delete a project snippet by project_id and snippet_id. Destructive; confirm both identifiers before calling.",
		aliases:     []string{"delete project snippet", "remove project snippet", "destroy project snippet", "drop project snippet"},
		related:     []string{actionSnippetProjectGet, actionSnippetProjectList},
		description: "Delete a project snippet permanently. Returns: a success confirmation. See also: gitlab_project_snippet_get, gitlab_project_snippet_list.",
	},
}
