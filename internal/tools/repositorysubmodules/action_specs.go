package repositorysubmodules

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionListSubmodules    = "repositorysubmodules.list_submodules"
	actionReadSubmoduleFile = "repositorysubmodules.read_submodule_file"
	actionUpdateSubmodule   = "repositorysubmodules.update_submodule"
)

// ActionSpecs returns canonical specs for repository submodule actions.
// Each action carries non-generic discovery metadata (1:1 audit R-META):
// action-specific Usage, submodule-phrased natural-language Aliases,
// canonical repository.*/commit.* RelatedActions, and an individual-tool
// Description in "Returns: … See also: …" form.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		submoduleReadSpec("list_submodules", toolutil.RouteAction(client, List), "gitlab_list_repository_submodules"),
		submoduleReadSpec("read_submodule_file", toolutil.RouteAction(client, Read), "gitlab_read_repository_submodule_file"),
		submoduleUpdateSpec("update_submodule", toolutil.RouteAction(client, Update), "gitlab_update_repository_submodule"),
	}
}

func submoduleReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := submoduleOptions(individualTool)
	decorateSubmoduleMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func submoduleUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := submoduleOptions(individualTool)
	decorateSubmoduleMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func submoduleOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute repositorysubmodules domain action.", Tags: []string{"repository", "submodule"},
		RelatedActions: []string{"repository.tree", "repository.file_get"},
		OpenWorld:      true,
		OwnerPackage:   "repositorysubmodules",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateSubmoduleMeta replaces the generic placeholder Usage, toolname-only
// Aliases, and empty individual-tool Description for a submodule action with
// non-generic discovery metadata (1:1 audit R-META). It is a no-op for tools
// not present in submoduleActionMeta.
func decorateSubmoduleMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := submoduleActionMeta[individualTool]
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

// submoduleActionMetaEntry is the discovery metadata for one submodule action.
type submoduleActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// submoduleActionMeta maps each individual repository submodule tool to its
// non-generic discovery metadata. Aliases use distinctive submodule phrasing
// (".gitmodules", "submodule pointer", "pinned commit") so they do not collide
// with generic repository or file tools.
var submoduleActionMeta = map[string]submoduleActionMetaEntry{
	"gitlab_list_repository_submodules": {
		usage:       "List the Git submodules declared in a repository's .gitmodules, each with its path, remote URL, resolved GitLab project, and the commit SHA the parent currently pins. Use when the prompt asks which submodules a project has or where a submodule points.",
		aliases:     []string{"list git submodules", "show repository submodules", "list .gitmodules entries", "show submodule pointers"},
		related:     []string{actionReadSubmoduleFile, actionUpdateSubmodule, "repository.tree"},
		description: "List the Git submodules defined in a repository's .gitmodules. Returns: each submodule's name, path, remote url, resolved_project, and pinned commit_sha, plus a count. See also: gitlab_read_repository_submodule_file, gitlab_update_repository_submodule, gitlab_repository_tree.",
	},
	"gitlab_read_repository_submodule_file": {
		usage:       "Read a file from inside a submodule at the exact commit the parent repository pins. Resolves the submodule project via .gitmodules, finds the pinned commit SHA from the parent tree, then fetches the file from the submodule's project. Use when the prompt asks for content that lives inside a submodule rather than the parent repo.",
		aliases:     []string{"read submodule file", "view file inside submodule", "get submodule contents at pinned commit", "open file from a git submodule"},
		related:     []string{actionListSubmodules, actionUpdateSubmodule, "repository.file_get"},
		description: "Read a file from inside a submodule at the parent-pinned commit. Returns: file_name, file_path, submodule_path, resolved_project, commit_sha, size, decoded content, and encoding. See also: gitlab_list_repository_submodules, gitlab_file_get, gitlab_update_repository_submodule.",
	},
	"gitlab_update_repository_submodule": {
		usage:       "Move a submodule pointer to a new commit SHA, creating a commit on the target branch in the parent repository. Provide project_id, the submodule path, branch, and commit_sha; add commit_message to override the generated message. Use when bumping a submodule to a newer revision.",
		aliases:     []string{"update submodule pointer", "bump submodule commit", "set submodule reference", "point submodule at a new sha"},
		related:     []string{actionListSubmodules, actionReadSubmoduleFile, "commit.get"},
		description: "Update a submodule pointer to a new commit SHA on a branch. Returns: the created commit with id, short_id, title, author and committer details, dates, parent_ids, message, and web_url. See also: gitlab_list_repository_submodules, gitlab_read_repository_submodule_file, gitlab_commit_get.",
	},
}
