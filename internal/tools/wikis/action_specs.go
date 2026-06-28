package wikis

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionWikiList   = "wiki.list"
	actionWikiGet    = "wiki.get"
	actionWikiUpdate = "wiki.update"
)

// ActionSpecs returns canonical specs for project wiki actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_wiki_list — list wiki pages for a project with optional content.
		wikiReadSpec("list", toolutil.RouteAction(client, List), "gitlab_wiki_list"),
		// gitlab_wiki_get — fetch a single wiki page by slug (returns a structured not-found result on 404).
		wikiReadSpec("get", wikiGetRoute(client), "gitlab_wiki_get"),
		// gitlab_wiki_create — create a new wiki page.
		wikiCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_wiki_create"),
		// gitlab_wiki_update — update an existing wiki page by slug.
		wikiUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_wiki_update"),
		// gitlab_wiki_delete — delete a wiki page (destructive).
		wikiDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_wiki_delete"),
		// gitlab_wiki_upload_attachment — upload a file attachment to the project wiki.
		wikiCreateSpec("upload_attachment", toolutil.RouteAction(client, UploadAttachment), "gitlab_wiki_upload_attachment"),
	}
}

// wikiGetRoute wraps the canonical Get route to return a structured not-found output
// when GitLab responds with HTTP 404, mirroring the branches package behavior.
func wikiGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			slug, _ := input["slug"].(string)
			return wikiNotFoundOutput{Identifier: fmt.Sprintf("slug %q in project %v", slug, input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

// wikiReadSpec builds the canonical read-only spec for a wiki tool.
func wikiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := wikiOptions(individualTool)
	decorateWikiMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// wikiCreateSpec builds the canonical create spec for a wiki tool.
func wikiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := wikiOptions(individualTool)
	decorateWikiMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

// wikiUpdateSpec builds the canonical update spec for a wiki tool.
func wikiUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := wikiOptions(individualTool)
	decorateWikiMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

// wikiDeleteSpec builds the canonical destructive delete spec for a wiki tool.
func wikiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := wikiOptions(individualTool)
	decorateWikiMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func wikiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute wikis domain action.", Tags: []string{"wiki"},
		RelatedActions: []string{actionWikiList, actionWikiGet, "project.get", "repository.file_get"},
		OpenWorld:      true,
		OwnerPackage:   "wikis",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateWikiMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for each wiki action, replacing the generic placeholder metadata produced by
// wikiOptions (1:1 audit R-META). It is a no-op for tools with no metadata entry.
func decorateWikiMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := wikiActionMeta[individualTool]
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

// wikiActionMetaEntry is the discovery metadata for one wiki action.
type wikiActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// wikiActionMeta maps each individual wiki tool to its discovery metadata.
var wikiActionMeta = map[string]wikiActionMetaEntry{
	"gitlab_wiki_list": {
		usage:   "List the wiki pages of a project. Set with_content only when the page bodies are needed, since it returns the full content of every page.",
		aliases: []string{"list wiki pages", "show project wiki", "find wiki pages"},
		related: []string{actionWikiGet, "wiki.create", "project.get"},
		description: "List a project's wiki pages. Returns: each page's title, slug, and format (plus content when with_content is set), with hints to read or create pages. " +
			"See also: gitlab_wiki_get, gitlab_wiki_create.",
	},
	"gitlab_wiki_get": {
		usage:   "Fetch one wiki page by its slug. Use after wiki.list to read a page's content, optionally rendering HTML or retrieving a specific version SHA.",
		aliases: []string{"get wiki page", "read wiki page", "show wiki page content"},
		related: []string{actionWikiList, actionWikiUpdate, "wiki.delete"},
		description: "Get a single wiki page by slug. Returns: the page title, slug, format, content, and encoding. " +
			"See also: gitlab_wiki_list, gitlab_wiki_update, gitlab_wiki_delete.",
	},
	"gitlab_wiki_create": {
		usage:   "Create a new wiki page in a project. Provide a title and content; set format to markdown, rdoc, asciidoc, or org when the default markdown is not wanted.",
		aliases: []string{"create wiki page", "add wiki page", "new wiki page"},
		related: []string{actionWikiList, actionWikiGet, actionWikiUpdate},
		description: "Create a new wiki page. Returns: the created page with title, slug, format, content, and encoding. " +
			"See also: gitlab_wiki_get, gitlab_wiki_update, gitlab_wiki_list.",
	},
	"gitlab_wiki_update": {
		usage:   "Update an existing wiki page identified by slug. Provide at least one of title, content, or format to change.",
		aliases: []string{"update wiki page", "edit wiki page", "rename wiki page"},
		related: []string{actionWikiGet, actionWikiList, "wiki.delete"},
		description: "Update an existing wiki page by slug. Returns: the updated page with title, slug, format, content, and encoding. " +
			"See also: gitlab_wiki_get, gitlab_wiki_delete, gitlab_wiki_list.",
	},
	"gitlab_wiki_delete": {
		usage:   "Permanently delete a wiki page by slug. Destructive; requires Maintainer or Owner role and confirmation of the project and slug.",
		aliases: []string{"delete wiki page", "remove wiki page", "destroy wiki page", "drop wiki page"},
		related: []string{actionWikiGet, actionWikiList, actionWikiUpdate},
		description: "Delete a wiki page permanently. Returns: a success confirmation for the removed page. " +
			"See also: gitlab_wiki_get, gitlab_wiki_list.",
	},
	"gitlab_wiki_upload_attachment": {
		usage:   "Upload a file to the wiki's uploads folder so it can be referenced from wiki pages. Provide either content_base64 or file_path, plus a filename and optional branch.",
		aliases: []string{"upload wiki attachment", "attach file to wiki", "add wiki attachment"},
		related: []string{actionWikiGet, actionWikiList, actionWikiUpdate},
		description: "Upload an attachment to the project wiki repository. Returns: the attachment file name, file path, branch, URL, and ready-to-paste Markdown link. " +
			"See also: gitlab_wiki_get, gitlab_wiki_update.",
	},
}
