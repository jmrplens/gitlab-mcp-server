package groupwikis

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionGroupWikiList = "group.wiki_list"
	actionGroupWikiGet  = "group.wiki_get"
	actionGroupWikiEdit = "group.wiki_edit"
)

// ActionSpecs returns canonical specs for group wiki actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupWikiReadSpec("wiki_list", toolutil.RouteAction(client, List), "gitlab_group_wiki_list"),
		groupWikiReadSpec("wiki_get", toolutil.RouteAction(client, Get), "gitlab_group_wiki_get"),
		groupWikiCreateSpec("wiki_create", toolutil.RouteAction(client, Create), "gitlab_group_wiki_create"),
		groupWikiUpdateSpec("wiki_edit", toolutil.RouteAction(client, Edit), "gitlab_group_wiki_edit"),
		groupWikiDeleteSpec("wiki_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_wiki_delete"),
	}
}

func groupWikiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupWikiOptions(individualTool)
	decorateGroupWikiMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func groupWikiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupWikiOptions(individualTool)
	decorateGroupWikiMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func groupWikiUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupWikiOptions(individualTool)
	decorateGroupWikiMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func groupWikiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupWikiOptions(individualTool)
	decorateGroupWikiMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func groupWikiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupwikis domain action.", Tags: []string{"group", "wiki"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupwikis",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateGroupWikiMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for each group wiki action, replacing the generic placeholder metadata produced
// by groupWikiOptions (1:1 audit R-META). Aliases are group-wiki-specific to avoid
// collision with the project wikis package. It is a no-op for tools with no
// metadata entry.
func decorateGroupWikiMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupWikiActionMeta[individualTool]
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

// groupWikiActionMetaEntry is the discovery metadata for one group wiki action.
type groupWikiActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// groupWikiActionMeta maps each individual group wiki tool to its discovery
// metadata. Aliases are phrased for group wikis (a GitLab Premium feature)
// so they remain distinct from the project-level gitlab_wiki_* tools.
var groupWikiActionMeta = map[string]groupWikiActionMetaEntry{
	"gitlab_group_wiki_list": {
		usage:   "List the wiki pages of a group (GitLab Premium). Set with_content only when the page bodies are needed, since it returns the full content of every page.",
		aliases: []string{"list group wiki pages", "show group wiki", "find group wiki pages"},
		related: []string{actionGroupWikiGet, "group.wiki_create", "group.get"},
		description: "List a group's wiki pages (GitLab Premium). Returns: each page's title, slug, and format (plus content when with_content is set), with hints to read or create pages. " +
			"See also: gitlab_group_wiki_get, gitlab_group_wiki_create.",
	},
	"gitlab_group_wiki_get": {
		usage:   "Fetch one group wiki page by its slug. Use after group.wiki_list to read a page's content, optionally rendering HTML or retrieving a specific version SHA.",
		aliases: []string{"get group wiki page", "read group wiki page", "show group wiki page content"},
		related: []string{actionGroupWikiList, actionGroupWikiEdit, "group.wiki_delete"},
		description: "Get a single group wiki page by slug. Returns: the page title, slug, format, content, and encoding. " +
			"See also: gitlab_group_wiki_list, gitlab_group_wiki_edit, gitlab_group_wiki_delete.",
	},
	"gitlab_group_wiki_create": {
		usage:   "Create a new wiki page in a group (GitLab Premium). Provide a title and content. Set format to markdown, rdoc, asciidoc, or org when the default markdown is not wanted.",
		aliases: []string{"create group wiki page", "add group wiki page", "new group wiki page"},
		related: []string{actionGroupWikiList, actionGroupWikiGet, actionGroupWikiEdit},
		description: "Create a new group wiki page. Returns: the created page with title, slug, format, content, and encoding. " +
			"See also: gitlab_group_wiki_get, gitlab_group_wiki_edit, gitlab_group_wiki_list.",
	},
	"gitlab_group_wiki_edit": {
		usage:   "Update an existing group wiki page identified by slug. Provide at least one of title, content, or format to change.",
		aliases: []string{"edit group wiki page", "update group wiki page", "rename group wiki page"},
		related: []string{actionGroupWikiGet, actionGroupWikiList, "group.wiki_delete"},
		description: "Update an existing group wiki page by slug. Returns: the updated page with title, slug, format, content, and encoding. " +
			"See also: gitlab_group_wiki_get, gitlab_group_wiki_delete, gitlab_group_wiki_list.",
	},
	"gitlab_group_wiki_delete": {
		usage:   "Permanently delete a group wiki page by slug. Destructive. Requires Maintainer or Owner role and confirmation of the group and slug.",
		aliases: []string{"delete group wiki page", "remove group wiki page", "destroy group wiki page", "drop group wiki page"},
		related: []string{actionGroupWikiGet, actionGroupWikiList, actionGroupWikiEdit},
		description: "Delete a group wiki page permanently. Returns: a success confirmation for the removed page. " +
			"See also: gitlab_group_wiki_get, gitlab_group_wiki_list.",
	},
}
