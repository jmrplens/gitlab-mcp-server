package projectdiscovery

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// discoverProjectDescription is what a model reads before calling
// gitlab_discover_project.
//
// The "NOT for" clause names canonical action IDs rather than tool names, and
// that is not a style choice. This tool is registered on all three surfaces
// while a tool name belongs to one, and the only part of a description anything
// projects per surface is the "See also" clause: rewriteSeeAlso in
// internal/resources turns the names there into whatever the active surface
// calls them, which is why that clause stays in individual tool names here and
// everywhere else. Prose outside it is carried verbatim, so the meta spellings
// this paragraph used to hold (gitlab_search action=projects, gitlab_project
// action=list_user_projects, gitlab_server action=health_check) named nothing a
// model on the individual surface could call, in the one description whose
// whole job is to send a model somewhere else. A canonical ID resolves on every
// surface, and cmd/audit_action_ids reads the dotted IDs a description spells,
// so one of these going stale is reported rather than silent.
const discoverProjectDescription = "Resolve a full git remote URL to a GitLab project and return its project_id and metadata. " +
	"Read-only. Performs a lookup against the GitLab Projects API. No side effects.\n\n" +
	"When to use: only when the user or workspace provides a complete git remote URL from .git/config ([remote \"origin\"] url = ...) or from 'git remote -v'. " +
	"If the prompt already provides a project path such as group/project or a numeric project ID, pass that value directly as params.project_id to the requested GitLab tool instead of calling discovery. " +
	"Do not synthesize, guess, or add .git to a project path to create a remote URL.\n" +
	"NOT for: searching projects by name (use search.projects), listing a user's projects (use project.list_user_projects), " +
	"verifying GitLab connectivity or authentication (use server.status), or pre-checking workflows where project_id is already known.\n\n" +
	"IMPORTANT: pass the complete URL exactly as it appears. Do NOT strip the git@ prefix from SSH URLs. " +
	"Supported formats (a URL scheme or git@ user prefix is required):\n" +
	"- HTTPS: https://gitlab.example.com/group/project.git\n" +
	"- SSH shorthand: git@gitlab.example.com:group/project.git\n" +
	"- SSH protocol: ssh://git@gitlab.example.com/group/project.git\n\n" +
	"Returns: {id, name, path, path_with_namespace, web_url, description, default_branch, visibility, http_url_to_repo, ssh_url_to_repo, extracted_path}. " +
	"Errors: 404 not found (hint: project may be private, so verify token permissions), 403 forbidden (hint: token lacks read_api scope).\n\n" +
	"See also: gitlab_project_get, gitlab_server_status, gitlab_search_projects."

// ActionSpecs returns canonical specs for standalone project discovery actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("resolve", toolutil.RouteAction(client, Resolve), toolutil.ActionSpecOptions{
			Aliases: []string{"gitlab_discover_project"}, Tags: []string{"discovery", "project"},
			Usage:          "Resolve a complete git remote URL (e.g. git@host:group/project.git or https://host/group/project.git, from .git/config or git remote -v) to GitLab project metadata. Only use this when the input is a full git remote URL; for a namespace path like group/project or a numeric ID use project.get instead.",
			RelatedActions: []string{"project.get"},
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"remote_url": {
					SemanticRole:   "git_remote_url",
					ValueSource:    "A complete git remote URL from .git/config or `git remote -v`.",
					ExampleBinding: `params.remote_url:"git@gitlab.com:my-org/tools/gitlab-mcp-server.git"`,
					CommonConfusions: []string{
						"remote_url must be a full git URL, not a namespace path: a value like my-org/tools/gitlab-mcp-server is a path. Look it up with project.get (project_id), not discover_project.resolve.",
					},
				},
			},
			OpenWorld:      true,
			OwnerPackage:   "projectdiscovery",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_discover_project", Title: toolutil.TitleFromName("gitlab_discover_project"), Description: discoverProjectDescription},
		}),
	}
}
