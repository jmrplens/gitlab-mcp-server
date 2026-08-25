// instructions.go builds the MCP handshake instructions, the text every client
// injects into its model's system prompt. Because the same operation is named
// differently on each tool surface, the guidance is assembled per surface
// rather than written once: a single hardcoded text would name individual-mode
// tools to a dynamic-mode model that cannot see any of them.
package main

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// surfaceToolRef names one GitLab operation on each of the three tool
// surfaces. The same operation is reached through gitlab_execute_action on the
// dynamic surface, through an action-dispatching meta-tool on the meta
// surface, and through a dedicated tool on the individual surface.
//
// MetaTool is empty for standalone utilities, which keep their own tool name on
// the meta surface instead of being folded into a dispatcher.
type surfaceToolRef struct {
	Action     string
	MetaTool   string
	MetaAction string
	Individual string
}

// render returns how a model on surface should call the operation, phrased for
// inline use in a sentence.
//
// The dynamic form names the action alone rather than repeating
// "gitlab_execute_action with ..." at each of its nine call sites: the FINDING
// TOOLS preamble already establishes that action IDs are passed to
// gitlab_execute_action, and dynamic is the default surface, so the repetition
// would cost tokens in every session for no added clarity.
func (r surfaceToolRef) render(surface string) string {
	switch surface {
	case config.ToolSurfaceDynamic:
		return fmt.Sprintf("action=%q", r.Action)
	case config.ToolSurfaceMeta:
		if r.MetaTool == "" {
			return r.Individual
		}
		return fmt.Sprintf("%s with action=%q", r.MetaTool, r.MetaAction)
	default:
		return r.Individual
	}
}

// The operations the handshake instructions refer to by name. Every field is
// validated against the live catalog of each surface by
// TestBuildInstructions_NamesResolveOnEverySurface, so a renamed action breaks
// the build rather than shipping a name no model can call.
var (
	refDiscoverProject = surfaceToolRef{
		Action:     "discover_project.resolve",
		Individual: "gitlab_discover_project",
	}
	refProjectList = surfaceToolRef{
		Action:     "project.list",
		MetaTool:   "gitlab_project",
		MetaAction: "list",
		Individual: "gitlab_project_list",
	}
	refSearchProjects = surfaceToolRef{
		Action:     "search.projects",
		MetaTool:   "gitlab_search",
		MetaAction: "projects",
		Individual: "gitlab_search_projects",
	}
	refProjectGet = surfaceToolRef{
		Action:     "project.get",
		MetaTool:   "gitlab_project",
		MetaAction: "get",
		Individual: "gitlab_project_get",
	}
	refPackagePublishAndLink = surfaceToolRef{
		Action:     "package.publish_and_link",
		MetaTool:   "gitlab_package",
		MetaAction: "publish_and_link",
		Individual: "gitlab_package_publish_and_link",
	}
	refPackagePublish = surfaceToolRef{
		Action:     "package.publish",
		MetaTool:   "gitlab_package",
		MetaAction: "publish",
		Individual: "gitlab_package_publish",
	}
	refReleaseLinkCreate = surfaceToolRef{
		Action:     "release.link_create",
		MetaTool:   "gitlab_release",
		MetaAction: "link_create",
		Individual: "gitlab_release_link_create",
	}
	refReleaseCreate = surfaceToolRef{
		Action:     "release.create",
		MetaTool:   "gitlab_release",
		MetaAction: "create",
		Individual: "gitlab_release_create",
	}
	refIssueGetByID = surfaceToolRef{
		Action:     "issue.get_by_id",
		MetaTool:   "gitlab_issue",
		MetaAction: "get_by_id",
		Individual: "gitlab_issue_get_by_id",
	}
)

// buildInstructions returns the handshake instructions for toolSurface and
// capabilitySurface. The tool advice is identical across tool surfaces —
// only the tool and action names change — while the watching section exists
// only where subscriptions are actually offered, so the model is never told
// about a capability this server would refuse. statelessHTTP further shapes
// that section: on a sessionless transport the legacy resources/subscribe
// request is refused, so the instructions name subscriptions/listen there
// instead of teaching the model a method that cannot work.
func buildInstructions(toolSurface, capabilitySurface string, statelessHTTP bool) string {
	var b strings.Builder

	b.WriteString("gitlab-mcp-server exposes GitLab projects, merge requests, issues, branches, " +
		"tags, releases, repositories, commits, files, groups, members, and uploads.\n\n")

	if toolSurface == config.ToolSurfaceDynamic {
		b.WriteString("FINDING TOOLS — This server exposes two tools that reach the whole GitLab API:\n" +
			"1. Call gitlab_find_action with a natural-language description of the task to get matching " +
			"action IDs and their input schemas.\n" +
			"2. Call gitlab_execute_action with that action ID, its parameters under 'params'.\n" +
			"3. Every action=\"...\" named below is a canonical action ID: pass it to gitlab_execute_action " +
			"directly, no find step needed.\n\n")
	}

	fmt.Fprintf(&b, "PROJECT DISCOVERY — To find the project_id needed for most operations:\n"+
		"1. Read the .git/config file from the workspace to find [remote \"origin\"] url = ...\n"+
		"2. Call %s with that URL to get the project_id.\n"+
		"3. Alternatively, use %s (owned=true) or %s to find projects by name.\n\n",
		refDiscoverProject.render(toolSurface),
		refProjectList.render(toolSurface),
		refSearchProjects.render(toolSurface))

	fmt.Fprintf(&b, "DEFAULT BRANCH — When generating URLs to repository files or branches:\n"+
		"1. Call %s to retrieve the project metadata, which includes the default_branch field.\n"+
		"2. ALWAYS use the returned default_branch value (e.g. develop, master) instead of assuming 'main'.\n"+
		"3. Projects can use any branch as default, so NEVER hardcode 'main' in URLs.\n\n",
		refProjectGet.render(toolSurface))

	fmt.Fprintf(&b, "PACKAGE + RELEASE WORKFLOW — When uploading packages and linking them to releases:\n"+
		"1. Preferred: Use %s to upload a file and create the release link in one step.\n"+
		"2. Alternative: Use %s first, then use the 'url' field from its response as the URL for %s.\n"+
		"3. NEVER construct package download URLs manually — always use the actual URL returned by the publish tool.\n"+
		"4. RELEASE LINK NAMING: The link_name MUST be the exact filename (e.g. 'checksums.txt.asc'), "+
		"NEVER add descriptive suffixes like '(GPG signature)'. go-selfupdate and other tools match asset names exactly.\n\n",
		refPackagePublishAndLink.render(toolSurface),
		refPackagePublish.render(toolSurface),
		refReleaseLinkCreate.render(toolSurface))

	fmt.Fprintf(&b, "RELEASE CREATION — When creating releases:\n"+
		"1. You do NOT need to create the tag first. Provide 'ref' (branch or SHA) in %s and GitLab auto-creates the tag.\n"+
		"2. The response includes 'assets_sources' with auto-generated tar.gz/zip archive URLs — use those, "+
		"never construct source archive URLs.\n"+
		"3. Use 'tag_message' to create an annotated tag instead of a lightweight one.\n\n",
		refReleaseCreate.render(toolSurface))

	fmt.Fprintf(&b, "ID vs IID — GitLab uses two identifiers for issues and merge requests:\n"+
		"1. IID is the project-scoped number shown in URLs and UI (e.g. issue #3, MR !5). Most operations expect IID.\n"+
		"2. ID is the global numeric identifier. Only use %s when you have a global ID from another API response.",
		refIssueGetByID.render(toolSurface))

	if capabilitySurface == config.CapabilitySurfaceFull {
		subscribeMethod := "MCP resources/subscribe"
		if statelessHTTP {
			// Each stateless POST's session closes with the response, so the
			// legacy request is refused there; only the long-lived
			// subscriptions/listen form can be honored.
			subscribeMethod = "MCP subscriptions/listen (protocol 2026-07-28; the legacy resources/subscribe is refused on this transport)"
		}
		fmt.Fprintf(&b, "\n\nWATCHING RESOURCES — Instead of re-reading a resource in a loop to detect change:\n"+
			"1. Single-object resources (a pipeline, an issue, a merge request, a file, a wiki page, ...) can be "+
			"watched for change notifications via %s; collections (issue lists, branch lists) cannot.\n"+
			"2. Example: subscribe to gitlab://project/{project_id}/pipelines/latest to be notified when a pipeline's "+
			"state changes, instead of polling it yourself — the server watches GitLab and sends "+
			"notifications/resources/updated only when the content actually changed.", subscribeMethod)
	}

	return b.String()
}
