package main

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
)

// The categories of a join declaration.
const (
	// categoryHeadInheritsGet is a HEAD request on a route GitLab mounts only
	// as a GET: Grape answers it from the GET's endpoint, whose settings carry
	// the authorization.
	categoryHeadInheritsGet = "head-inherits-get"
	// categorySlugFromInput is a route whose last segment the caller's input
	// names, where GitLab mounts one route per value, every one declaring the
	// same authorization.
	categorySlugFromInput = "slug-from-input"
	// categoryGroupWorkItem is a position holding an epic, which is a group's
	// work item. GitLab declares WorkItem at the project boundary only
	// (app/graphql/types/work_item_type.rb:13), so a group's work item
	// resolves no boundary (lib/gitlab/graphql/authz/boundary_extractor.rb:40-43)
	// and no fine-grained token passes it there: a read answers null, or drops
	// it from a list, with no error, and a write that answers with it commits
	// and loses its answer. The same document reaches project work items for
	// other actions (an epic's child issues, the work item tools), which is why
	// the declaration names the action and the position rather than the type.
	categoryGroupWorkItem = "group-work-item"
)

// The epic positions each epic action reaches, by the document it sends:
// queryResolveWorkItemGID and the list, discussion and note queries read the
// epic at namespace.workItem, client-go's work item documents read or answer
// with it at namespace.workItem, namespace.workItems.nodes or the write's
// workItem, and an epic issue assignment updates the epic itself.
// mutationRemoveParent and mutationReorderChild are left out: they update the
// child issue, a project's work item, which resolves.
var epicPositions = []struct {
	action string
	paths  []string
}{
	{"group.epic_create", []string{"workItemCreate.workItem"}},
	{"group.epic_delete", []string{"namespace.workItem"}},
	{"group.epic_discussion_add_note", []string{"namespace.workItem"}},
	{"group.epic_discussion_create", []string{"namespace.workItem"}},
	{"group.epic_discussion_get", []string{"namespace.workItem"}},
	{"group.epic_discussion_list", []string{"namespace.workItem"}},
	{"group.epic_get", []string{"namespace.workItem"}},
	{"group.epic_issue_assign", []string{"namespace.workItem", "workItemUpdate.workItem"}},
	{"group.epic_issue_list", []string{"namespace.workItem"}},
	{"group.epic_issue_remove", []string{"namespace.workItem"}},
	{"group.epic_issue_update", []string{"namespace.workItem"}},
	{"group.epic_list", []string{"namespace.workItems.nodes"}},
	{"group.epic_note_create", []string{"namespace.workItem"}},
	{"group.epic_note_get", []string{"namespace.workItem"}},
	{"group.epic_note_list", []string{"namespace.workItem"}},
	{"group.epic_update", []string{"namespace.workItem", "workItemUpdate.workItem"}},
}

// groupWorkItems declares every epic position as one no fine-grained token
// resolves a boundary for.
func groupWorkItems() []join.BoundaryDeclaration {
	var out []join.BoundaryDeclaration
	for _, epic := range epicPositions {
		for _, path := range epic.paths {
			out = append(out, join.BoundaryDeclaration{
				Action: epic.action, Path: path, Category: categoryGroupWorkItem,
				Reason: "the work item at " + path + " is the epic the action names, a group's work item",
			})
		}
	}
	return out
}

// grantDeclarations answer what the join cannot place on its own. A route
// declaration names the record route whose authorization the derived route
// carries; one whose derived route the record now carries, or that no action
// sends, is stale. A slug declaration is also held, on every run, to every
// route its placeholder stands for declaring the same authorization as the
// one it names. A boundary declaration names a position no fine-grained token
// passes for one action, and is stale when the action's documents no longer
// select it.
var grantDeclarations = join.Declarations{
	Unresolvable: groupWorkItems(),
	Routes: []join.RouteDeclaration{
		{
			Route: "HEAD /projects/:/repository/files/:/raw", Category: categoryHeadInheritsGet,
			Reason: "repository.file_raw_metadata asks for the raw file's headers only; GitLab mounts the route as a GET " +
				"(lib/api/files.rb), and Grape 2.4.0's Endpoint#mount_in answers the automatic HEAD from the same endpoint, " +
				"so the GET's route_setting is the one checked (read from the image's Grape source when the live record was taken)",
			Use: "GET /projects/:id/repository/files/:file_path/raw",
		},
		{
			Route: "PUT /projects/:/integrations/:", Category: categorySlugFromInput,
			Reason: "project.integration_set names the integration by the caller's slug, and GitLab mounts one PUT per " +
				"integration, all 49 declaring update_integration at a project or group boundary",
			Use: "PUT /projects/:id/integrations/apple-app-store",
		},
		{
			Route: "PUT /groups/:/integrations/:", Category: categorySlugFromInput,
			Reason: "project.integration_set_group names the integration by the caller's slug, and GitLab mounts one PUT per " +
				"integration, all 49 declaring update_integration at a project or group boundary",
			Use: "PUT /groups/:id/integrations/apple-app-store",
		},
	},
}
