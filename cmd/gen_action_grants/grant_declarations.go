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
)

// grantDeclarations answer what the join cannot place on its own. A route
// declaration names the record route whose authorization the derived route
// carries; one whose derived route the record now carries, or that no action
// sends, is stale. A slug declaration is also held, on every run, to every
// route its placeholder stands for declaring the same authorization as the
// one it names.
var grantDeclarations = join.Declarations{
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
