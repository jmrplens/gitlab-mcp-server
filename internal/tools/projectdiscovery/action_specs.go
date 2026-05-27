package projectdiscovery

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const discoverProjectDescription = "Resolve a full git remote URL to a GitLab project and return its project_id and metadata. " +
	"Read-only; performs a lookup against the GitLab Projects API; no side effects.\n\n" +
	"When to use: only when the user or workspace provides a complete git remote URL from .git/config ([remote \"origin\"] url = ...) or from 'git remote -v'. " +
	"If the prompt already provides a project path such as group/project or a numeric project ID, pass that value directly as params.project_id to the requested GitLab tool instead of calling discovery. " +
	"Do not synthesize, guess, or add .git to a project path to create a remote URL.\n" +
	"NOT for: searching projects by name (use gitlab_search action=projects), listing a user's projects (use gitlab_project action=list_user_projects), " +
	"verifying GitLab connectivity or authentication (use gitlab_server action=health_check), or pre-checking workflows where project_id is already known.\n\n" +
	"IMPORTANT: pass the complete URL exactly as it appears — do NOT strip the git@ prefix from SSH URLs. " +
	"Supported formats (a URL scheme or git@ user prefix is required):\n" +
	"- HTTPS: https://gitlab.example.com/group/project.git\n" +
	"- SSH shorthand: git@gitlab.example.com:group/project.git\n" +
	"- SSH protocol: ssh://git@gitlab.example.com/group/project.git\n\n" +
	"Returns: {id, name, path, path_with_namespace, web_url, description, default_branch, visibility, http_url_to_repo, ssh_url_to_repo, extracted_path}. " +
	"Errors: 404 not found (hint: project may be private — verify token permissions), 403 forbidden (hint: token lacks read_api scope).\n\n" +
	"See also: gitlab_project (full project CRUD/settings once id is known), gitlab_server (connectivity and version checks), gitlab_search (find projects by query)."

// ActionSpecs returns canonical specs for standalone project discovery actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("resolve", toolutil.RouteAction(client, Resolve), toolutil.ActionSpecOptions{
			Aliases: []string{"gitlab_discover_project"}, Tags: []string{"discovery", "project"},
			Usage:          "Resolve a complete git remote URL from .git/config or git remote -v to GitLab project metadata.",
			OpenWorld:      true,
			OwnerPackage:   "projectdiscovery",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_discover_project", Title: toolutil.TitleFromName("gitlab_discover_project"), Description: discoverProjectDescription},
		}),
	}
}
