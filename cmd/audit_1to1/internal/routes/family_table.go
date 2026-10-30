// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import "net/http"

// The issues the tracked rows name, one per domain: thirty of the catalog's,
// and the virtual registries and GitLab Duo, two domains the catalog does not
// hold yet, whose issue opens it. Each tracks every route of its domain the
// table declares tracked.
const (
	// issuePipelineRoutes tracks the pipeline routes no action reaches.
	issuePipelineRoutes = 1604
	// issueGroupRoutes tracks the group routes no action reaches.
	issueGroupRoutes = 1605
	// issueProjectRoutes tracks the project routes no action reaches.
	issueProjectRoutes = 1606
	// issueIssueRoutes tracks the issue and work item routes no action reaches.
	issueIssueRoutes = 1607
	// issueMergeRequestRoutes tracks the merge request routes no action reaches.
	issueMergeRequestRoutes = 1608
	// issueMRReviewRoutes tracks the merge request review routes no action reaches.
	issueMRReviewRoutes = 1609
	// issueRepositoryRoutes tracks the repository routes no action reaches.
	issueRepositoryRoutes = 1610
	// issueBranchRoutes tracks the branch routes no action reaches.
	issueBranchRoutes = 1611
	// issueUserRoutes tracks the user routes no action reaches.
	issueUserRoutes = 1612
	// issueAccessRoutes tracks the access routes no action reaches.
	issueAccessRoutes = 1613
	// issueAdminRoutes tracks the administration routes no action reaches.
	issueAdminRoutes = 1614
	// issuePackageRoutes tracks the package routes no action reaches.
	issuePackageRoutes = 1615
	// issueVirtualRegistryRoutes tracks the virtual registry routes no action reaches.
	issueVirtualRegistryRoutes = 1616
	// issueVulnerabilityRoutes tracks the vulnerability routes no action reaches.
	issueVulnerabilityRoutes = 1617
	// issueCompliancePolicyRoutes tracks the security policy routes no action reaches.
	issueCompliancePolicyRoutes = 1618
	// issueOrbitRoutes tracks the Orbit routes no action reaches.
	issueOrbitRoutes = 1209
	// issueDuoRoutes tracks the GitLab Duo routes no action reaches.
	issueDuoRoutes = 1619
	// issueJobRoutes tracks the job routes no action reaches.
	issueJobRoutes = 1620
	// issueRunnerRoutes tracks the runner routes no action reaches.
	issueRunnerRoutes = 1621
	// issueWikiRoutes tracks the wiki routes no action reaches.
	issueWikiRoutes = 1622
	// issueSnippetRoutes tracks the snippet routes no action reaches.
	issueSnippetRoutes = 1623
	// issueReleaseRoutes tracks the release routes no action reaches.
	issueReleaseRoutes = 1624
	// issueEnvironmentRoutes tracks the environment routes no action reaches.
	issueEnvironmentRoutes = 1625
	// issueFeatureFlagsRoutes tracks the feature flag routes no action reaches.
	issueFeatureFlagsRoutes = 1626
	// issueSearchRoutes tracks the search routes no action reaches.
	issueSearchRoutes = 1627
	// issueGeoRoutes tracks the Geo routes no action reaches.
	issueGeoRoutes = 1628
	// issueMemberRoleRoutes tracks the member role routes no action reaches.
	issueMemberRoleRoutes = 1629
	// issueDORAMetricsRoutes tracks the DORA metrics routes no action reaches.
	issueDORAMetricsRoutes = 1630
	// issueCICatalogRoutes tracks the CI/CD catalog routes no action reaches.
	issueCICatalogRoutes = 1631
	// issueDependencyRoutes tracks the dependency routes no action reaches.
	issueDependencyRoutes = 1632
	// issueEnterpriseUserRoutes tracks the enterprise user routes no action reaches.
	issueEnterpriseUserRoutes = 1633
	// issueModelRegistryRoutes tracks the model registry routes no action reaches.
	issueModelRegistryRoutes = 1634
)

// on lists the verbs a row answers, every verb when it lists none.
func on(methods ...string) []string { return methods }

// tracked builds a tracked row: GitLab serves its routes to a caller with a
// token and no action sends them, which the issue tracks.
func tracked(issue, routes int, methods []string, pattern, reason string) routeFamily {
	return routeFamily{Methods: methods, Pattern: pattern, Category: categoryTracked, Reason: reason, Issue: issue, Routes: routes}
}

// downloadTail is why a download a client protocol serves is tracked.
const downloadTail = "; a download is tracked rather than client-protocol, so the action it needs stays in view, one that reads the file bounded by toolutil.MaxDownloadBytes as package.download reads a generic package's"

// download builds a tracked row of the GET routes that serve what a package
// or a model version holds (one of its files, its archive, its source or a
// file inside it), which a client protocol's row never answers: an action
// that downloads it is still to implement. An index or a metadata document a
// client reads to find and resolve packages, and the URL or the location of
// the content, are the protocol's.
func download(issue, routes int, pattern, what string) routeFamily {
	return tracked(issue, routes, on(http.MethodGet), pattern, what+downloadTail)
}

// publicTail is why a route GitLab serves without a token is tracked.
const publicTail = "; GitLab serves it with or without a token (public_endpoint), so it is tracked as any route a token reaches"

// publicEndpoint builds a tracked row of routes that skip the fine-grained
// check as public_endpoint, a protocol's among them: a token reaches them as
// any other route.
func publicEndpoint(issue, routes int, methods []string, pattern, what string) routeFamily {
	return tracked(issue, routes, methods, pattern, what+publicTail)
}

// protocol builds a client-protocol row: the routes a package manager's or a
// model registry's own client sends, named by the protocol.
func protocol(client string, routes int, methods []string, pattern, what string) routeFamily {
	return routeFamily{Methods: methods, Pattern: pattern, Category: categoryClientProtocol, Reason: "the " + client + " protocol: " + what, Routes: routes}
}

// overGraphQL builds a covered-by-graphql row: the actions do the routes'
// operation over GraphQL.
func overGraphQL(actions []string, routes int, methods []string, pattern, reason string) routeFamily {
	return routeFamily{Methods: methods, Pattern: pattern, Category: categoryCoveredByGraphQL, Reason: reason, Actions: actions, Routes: routes}
}

// The GraphQL actions several covered-by-graphql rows stand for.
const (
	actionEpicDiscussionGet = "group.epic_discussion_get"
	actionEpicNoteGet       = "group.epic_note_get"
)

// workItemPattern is one work item of a group, a project or a namespace at
// the REST API GitLab serves behind the work_item_rest_api flag, which a
// covered-by-graphql row per verb answers.
const workItemPattern = "/*/*/-/work_items/*"

// The client protocols several client-protocol rows name.
const (
	debianRepository        = "Debian repository"
	terraformModuleRegistry = "Terraform module registry"
)

// routeFamilies answers the routes no action reaches that no rule and no
// inherited decision answers, by the catalog domain whose issue tracks them.
// A row's count is pinned: a re-pin that adds a route under it, or an action
// that starts sending one, moves the count and the gate names the routes. A
// download a client protocol serves, what a package or a model version
// holds, is tracked rather than client-protocol, and so is a public_endpoint
// route, so every action still to implement stays a row whose issue names
// it.
var routeFamilies = []routeFamily{
	// The pipeline domain (issuePipelineRoutes).
	tracked(issuePipelineRoutes, 1, on(http.MethodGet), "/pipelines",
		"lists the pipelines the authenticated user triggered, across every project; client-go v3.30.0 has no method for it and no action lists them"),
	tracked(issuePipelineRoutes, 1, on(http.MethodPost), "/projects/*/create_ci_config",
		"opens a merge request that adds a CI/CD configuration to a project"),
	tracked(issuePipelineRoutes, 1, on(http.MethodGet), "/projects/*/pipeline_schedules/*/variables/*",
		"reads one variable of a pipeline schedule, which pipeline.schedule_create_variable, pipeline.schedule_edit_variable and pipeline.schedule_delete_variable write"),
	// The group domain (issueGroupRoutes).
	tracked(issueGroupRoutes, 2, on(http.MethodGet, http.MethodPost), "/groups/*/placeholder_reassignments",
		"a group owner reads the placeholder users an import left to reassign and reassigns them from a CSV file; client-go v3.30.0 has no method for either and no action reaches them"),
	tracked(issueGroupRoutes, 3, on(), "/groups/*/approval_rules/**",
		"lists, creates and updates a group's merge request approval rules; project.approval_rule_* manage a project's"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/billable_members/*/indirect",
		"lists the memberships a billable member holds through an invited group; group.group_billable_member_memberships_list lists the direct ones"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/hooks/*/events",
		"lists the events a group webhook delivered; group.hook_* manage the hook"),
	tracked(issueGroupRoutes, 1, on(http.MethodDelete), "/groups/*/ldap_group_links/*",
		"deletes an LDAP group link named by its cn alone, through Groups.DeleteGroupLDAPLink; group.ldap_link_delete deletes one by cn or filter in the body and group.ldap_link_delete_for_provider one by provider and cn, two other routes"),
	tracked(issueGroupRoutes, 4, on(), "/groups/*/manage/**",
		"a GitLab.com group's credentials inventory (assignable_when saas) lists, revokes and rotates the group and project access tokens under the group and rotates an enterprise user's personal access token; group.credential_* list and revoke the personal access tokens and SSH keys"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/members",
		"lists a group's direct members, through Groups.ListGroupMembers; group.members lists every member, inherited ones included (GET /groups/:id/members/all), another route"),
	tracked(issueGroupRoutes, 4, on(), "/groups/*/members/*/*",
		"sets and removes a member's LDAP access level override, approves a member awaiting approval and sets a user's membership state across the group's hierarchy"),
	tracked(issueGroupRoutes, 1, on(http.MethodPost), "/groups/*/members/approve_all",
		"approves every member of a group awaiting approval"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/pending_members",
		"lists the members of a group awaiting approval"),
	tracked(issueGroupRoutes, 4, on(), "/groups/*/saml/**",
		"lists a group's SAML identities, reads and deletes one and updates its extern_uid; group.saml_link_* manage the SAML group links"),
	tracked(issueGroupRoutes, 1, on(http.MethodPost), "/groups/*/secrets_manager/access_token",
		"creates an access token for a group's Secrets Manager"),
	tracked(issueGroupRoutes, 1, on(http.MethodDelete), "/groups/*/shared_projects/*",
		"removes a project shared with a group; group.shared_projects lists them and project.share_with_group shares one"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/subgroups",
		"lists a group's direct subgroups, through Groups.ListSubGroups; group.subgroups lists its descendant groups (GET /groups/:id/descendant_groups), another route"),
	tracked(issueGroupRoutes, 1, on(http.MethodPost), "/groups/*/transfer_to_organization",
		"moves a group to an organization; group.transfer moves it under another group"),
	tracked(issueGroupRoutes, 1, on(http.MethodPost), "/organizations",
		"creates an organization, the level above top-level groups, on GitLab.com (assignable_when saas)"),
	tracked(issueGroupRoutes, 1, on(http.MethodDelete), "/organizations/*",
		"soft-deletes an organization, the level above top-level groups"),
	tracked(issueGroupRoutes, 3, on(), "/groups/*/uploads/**",
		"uploads a file to a group's Markdown and downloads one by its ID or by its secret and name, through GroupMarkdownUploads; group.group_upload_* list and delete them, and a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/avatar",
		"downloads a group's avatar, which group.update uploads and removes, through Groups.DownloadAvatar: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/export_relations/download",
		"downloads a group's relations export, which group.group_relations_schedule schedules, through GroupRelationsExport.ExportDownload: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueGroupRoutes, 5, on(), "/groups/*/wiki_pages/*/notes/**",
		"lists, reads, creates, updates and deletes the comments on a group wiki page; group.wiki_* manage the pages"),
	tracked(issueGroupRoutes, 1, on(http.MethodPost), "/groups/*/wikis/attachments",
		"uploads an attachment to a group wiki; wiki.upload_attachment uploads one to a project's"),
	tracked(issueGroupRoutes, 2, on(http.MethodGet), "/groups/*/epic_boards/*/lists/**",
		"lists an epic board's lists and reads one; group.epic_board_get reads the board"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/groups/*/related_epic_links",
		"lists the related epic links of a group's epics"),
	tracked(issueGroupRoutes, 4, on(), "/groups/*/epics/*/award_emoji/**",
		"lists, reads, adds and deletes an epic's emoji reactions"),
	tracked(issueGroupRoutes, 4, on(), "/groups/*/epics/*/notes/*/award_emoji/**",
		"lists, reads, adds and deletes the emoji reactions on an epic's comment"),
	tracked(issueGroupRoutes, 3, on(), "/groups/*/epics/*/related_epics/**",
		"lists an epic's linked epics, links one and removes a link"),
	tracked(issueGroupRoutes, 2, on(http.MethodGet), "/groups/*/epics/*/resource_state_events/**",
		"lists an epic's state events and reads one; group.event_epic_label_* read its label events"),
	tracked(issueGroupRoutes, 1, on(http.MethodPost), "/groups/*/epics/*/todo",
		"creates a to-do item for an epic"),
	tracked(issueGroupRoutes, 1, on(http.MethodPut), "/groups/*/epics/*/discussions/*",
		"resolves or unresolves an epic discussion, which group.epic_discussion_* create, read and comment on over GraphQL"),
	overGraphQL([]string{"group.epic_list"}, 1, on(http.MethodGet), "/groups/*/-/epics",
		"lists a group's epics, the /-/ spelling of the list group.epic_list sends over GraphQL"),
	overGraphQL([]string{"group.epic_get"}, 1, on(http.MethodGet), "/groups/*/-/epics/*",
		"reads an epic, the /-/ spelling of the epic group.epic_get reads over GraphQL"),
	overGraphQL([]string{"group.epic_issue_list"}, 1, on(http.MethodGet), "/groups/*/-/epics/*/issues",
		"lists an epic's issues, the /-/ spelling of the children group.epic_issue_list lists over GraphQL"),
	overGraphQL([]string{"group.epic_create"}, 1, on(http.MethodPost), "/groups/*/epics/*/epics",
		"creates an epic under a parent epic, which group.epic_create does with parent_id"),
	overGraphQL([]string{"group.epic_update"}, 2, on(http.MethodPost, http.MethodDelete), "/groups/*/epics/*/epics/*",
		"makes an epic the child of another and removes it, which group.epic_update does by setting and clearing its parent_id"),
	overGraphQL([]string{"group.epic_issue_update"}, 1, on(http.MethodPut), "/groups/*/epics/*/epics/*",
		"reorders an epic's child epics, which group.epic_issue_update does for any child the epic's hierarchy holds"),
	overGraphQL([]string{actionEpicDiscussionGet}, 1, on(http.MethodGet), "/groups/*/epics/*/discussions/*/notes",
		"lists the comments of an epic discussion, which group.epic_discussion_get reads with the discussion"),
	overGraphQL([]string{actionEpicNoteGet}, 1, on(http.MethodGet), "/groups/*/epics/*/discussions/*/notes/*",
		"reads one comment of an epic discussion, which group.epic_note_get reads as any note of the epic"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/discover-cert-based-clusters",
		"lists the certificate-based clusters in a group's hierarchy, which GitLab deprecated for the agent for Kubernetes and whose routes the sdk scope declares SUPERSEDED_UPSTREAM"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/ldap/groups",
		"lists the LDAP groups of every provider, which a group's LDAP link names (group.ldap_link_add)"),
	tracked(issueGroupRoutes, 1, on(http.MethodGet), "/ldap/*/groups",
		"lists the LDAP groups of one provider, which a group's LDAP link names (group.ldap_link_add)"),
	// The project domain (issueProjectRoutes).
	tracked(issueProjectRoutes, 4, on(), "/projects/*/approval_settings/**",
		"lists a project's approval rules through the approval settings API and creates, updates and deletes one; project.approval_rule_* manage them through the approval rules API"),
	tracked(issueProjectRoutes, 1, on(http.MethodGet), "/projects/*/dependency_firewall/enablement",
		"reads whether the Dependency Firewall is on for a project; project.dependency_firewall_evaluate evaluates a dependency against it"),
	tracked(issueProjectRoutes, 3, on(), "/projects/*/export_relations/**",
		"schedules a project's relations export, reads its status and downloads it; group.group_relations_schedule and group.group_relations_list_status do the first two for a group"),
	tracked(issueProjectRoutes, 2, on(http.MethodGet), "/projects/*/google_cloud/setup/*",
		"downloads GitLab.com's (assignable_when saas) setup scripts of the Google Cloud integration and of runner provisioning"),
	tracked(issueProjectRoutes, 2, on(), "/projects/*/hooks/*/events/**",
		"lists the events a project webhook delivered and resends one; project.hook_* manage the hook"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/projects/*/import/git",
		"imports a repository into an existing project from a Git URL"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/projects/import-relation",
		"imports one relation of an exported project, such as its issues or merge requests, into an existing project"),
	tracked(issueProjectRoutes, 1, on(http.MethodGet), "/projects/*/relation-imports",
		"reads the status of a project's relation import"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/projects/*/import_project_members/*",
		"imports the members of another project into a project"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/projects/remote-import",
		"imports a project from an export archive at a URL; project.import_from_file imports one from a local file"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/projects/remote-import-s3",
		"imports a project from an export archive in an AWS S3 bucket"),
	tracked(issueProjectRoutes, 1, on(http.MethodGet), "/projects/*/integrations",
		"lists a project's active integrations; project.integration_list sends the GET /projects/:id/services spelling GitLab deprecates, so the capability is offered and this route is not sent"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/integrations/jira_connect/subscriptions",
		"subscribes a namespace to a GitLab for Jira Cloud installation, with the JWT the Jira app issues"),
	tracked(issueProjectRoutes, 1, on(http.MethodPut), "/projects/*/labels/promote",
		"promotes a project label named in the body to a group label; project.label_promote sends the PUT /projects/:id/labels/:name/promote spelling, which names it in the path"),
	tracked(issueProjectRoutes, 1, on(http.MethodGet), "/projects/*/members",
		"lists a project's direct members, through ProjectMembers.ListProjectMembers; project.members lists every member, inherited ones included (GET /projects/:id/members/all), another route"),
	tracked(issueProjectRoutes, 1, on(http.MethodPut), "/projects/*/pages/domains/*/verify",
		"verifies a Pages domain, which project.pages_domain_* create, read, update and delete"),
	tracked(issueProjectRoutes, 1, on(http.MethodGet), "/projects/*/pages_access",
		"checks whether the caller may read a project's Pages site"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/projects/*/repository_size",
		"starts a recalculation of a project's repository size"),
	tracked(issueProjectRoutes, 1, on(http.MethodPost), "/projects/*/secrets_manager/access_token",
		"creates an access token for a project's Secrets Manager"),
	tracked(issueProjectRoutes, 1, on(http.MethodGet), "/projects/*/share_locations",
		"lists the groups a project can be shared with, which project.share_with_group shares it with"),
	tracked(issueProjectRoutes, 1, on(http.MethodGet), "/projects/*/transfer_locations",
		"lists the namespaces a project can be transferred to; group.transfer_locations lists a group's and project.transfer transfers it"),
	tracked(issueProjectRoutes, 2, on(http.MethodGet), "/projects/*/uploads/**",
		"downloads a file uploaded to a project's Markdown by its ID or by its secret and name, through ProjectMarkdownUploads; project.upload_* upload, list and delete them, and a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	// The issue and work item domain (issueIssueRoutes).
	tracked(issueIssueRoutes, 4, on(), "/projects/*/issues/*/metric_images/**",
		"lists, uploads, updates and deletes an incident's metric images; admin.alert_metric_image_* do the same for an alert"),
	tracked(issueIssueRoutes, 1, on(http.MethodGet), "/projects/*/issues/*/resource_weight_events/*",
		"reads one weight event of an issue, which issue.event_issue_weight_list lists"),
	tracked(issueIssueRoutes, 1, on(http.MethodGet), "/projects/*/issues/*/user_agent_detail",
		"an administrator (assignable_when admin) reads the user agent an issue was created with, for spam review"),
	tracked(issueIssueRoutes, 2, on(http.MethodGet), "/projects/*/issues/*/discussions/*/notes/**",
		"lists the comments of an issue discussion and reads one; issue.discussion_get reads the discussion with its notes"),
	tracked(issueIssueRoutes, 1, on(http.MethodPost), "/projects/*/issues/*/clone",
		"clones an issue into another project; issue.move moves it"),
	tracked(issueIssueRoutes, 1, on(http.MethodPut), "/projects/*/issues/*/discussions/*",
		"resolves or unresolves an issue discussion, which issue.discussion_* create, read and comment on"),
	overGraphQL([]string{"issue.work_item_list"}, 3, on(http.MethodGet), "/*/*/-/work_items",
		"lists the work items of a group, a project or a namespace through the REST API GitLab serves behind the work_item_rest_api flag, which issue.work_item_list does over GraphQL"),
	overGraphQL([]string{"issue.work_item_create"}, 3, on(http.MethodPost), "/*/*/-/work_items",
		"creates a work item through the REST API GitLab serves behind the work_item_rest_api flag, which issue.work_item_create does over GraphQL"),
	overGraphQL([]string{"issue.work_item_get"}, 3, on(http.MethodGet), workItemPattern,
		"reads a work item through the REST API GitLab serves behind the work_item_rest_api flag, which issue.work_item_get does over GraphQL"),
	overGraphQL([]string{"issue.work_item_update"}, 3, on(http.MethodPatch), workItemPattern,
		"updates a work item through the REST API GitLab serves behind the work_item_rest_api flag, which issue.work_item_update does over GraphQL"),
	overGraphQL([]string{"issue.work_item_delete"}, 3, on(http.MethodDelete), workItemPattern,
		"deletes a work item through the REST API GitLab serves behind the work_item_rest_api flag, which issue.work_item_delete does over GraphQL"),
	overGraphQL([]string{"issue.work_item_update"}, 6, on(http.MethodPost, http.MethodDelete), "/*/*/-/work_items/*/children/*",
		"attaches a child work item and detaches it through the REST API GitLab serves behind the work_item_rest_api flag, which issue.work_item_update does over GraphQL by setting and clearing the child's parent_id"),
	overGraphQL([]string{"group.epic_discussion_list", "group.epic_discussion_create"}, 2, on(http.MethodGet, http.MethodPost), "/groups/*/-/work_items/*/discussions",
		"lists and starts the discussions of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_discussion_list and group.epic_discussion_create do over GraphQL, reading the group's work item by its iid whatever its type"),
	overGraphQL([]string{actionEpicDiscussionGet}, 1, on(http.MethodGet), "/groups/*/-/work_items/*/discussions/*",
		"reads a discussion of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_discussion_get does over GraphQL"),
	overGraphQL([]string{actionEpicDiscussionGet, "group.epic_discussion_add_note"}, 2, on(http.MethodGet, http.MethodPost), "/groups/*/-/work_items/*/discussions/*/notes",
		"lists the notes of a discussion of a group's work item and replies to it through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_discussion_get, reading the discussion with its notes, and group.epic_discussion_add_note do over GraphQL"),
	overGraphQL([]string{actionEpicNoteGet, "group.epic_discussion_update_note", "group.epic_discussion_delete_note"}, 3, on(http.MethodGet, http.MethodPut, http.MethodDelete), "/groups/*/-/work_items/*/discussions/*/notes/*",
		"reads, updates and deletes a note of a discussion of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_note_get, group.epic_discussion_update_note and group.epic_discussion_delete_note do over GraphQL"),
	overGraphQL([]string{"group.epic_note_list", "group.epic_note_create"}, 2, on(http.MethodGet, http.MethodPost), "/groups/*/-/work_items/*/notes",
		"lists and creates the notes of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_note_list and group.epic_note_create do over GraphQL"),
	overGraphQL([]string{actionEpicNoteGet}, 1, on(http.MethodGet), "/groups/*/-/work_items/*/notes/*",
		"reads a note of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_note_get does over GraphQL"),
	overGraphQL([]string{"group.epic_issue_list"}, 1, on(http.MethodGet), "/groups/*/-/work_items/*/children",
		"lists the children of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_issue_list does over GraphQL"),
	overGraphQL([]string{"group.epic_issue_update"}, 1, on(http.MethodPut), "/groups/*/-/work_items/*/children/*",
		"reorders a child of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag, which group.epic_issue_update does over GraphQL"),
	tracked(issueIssueRoutes, 1, on(http.MethodPut), "/groups/*/-/work_items/*/discussions/*",
		"resolves or unresolves a discussion of a group's work item through the REST API GitLab serves behind the work_item_rest_api flag; group.epic_discussion_* read, start and comment on it, and no action resolves one"),
	tracked(issueIssueRoutes, 9, on(), "/projects/*/-/work_items/*/discussions/**",
		"lists, starts, reads, resolves and replies to the discussions of a project's work item and reads, updates and deletes their notes, through the REST API GitLab serves behind the work_item_rest_api flag; issue.discussion_* do it for an issue through the issues API, and no action does it through this one"),
	tracked(issueIssueRoutes, 3, on(), "/projects/*/-/work_items/*/notes/**",
		"lists, creates and reads the notes of a project's work item through the REST API GitLab serves behind the work_item_rest_api flag; issue.note_* do it for an issue through the issues API, and no action does it through this one"),
	tracked(issueIssueRoutes, 2, on(http.MethodGet, http.MethodPut), "/projects/*/-/work_items/*/children/**",
		"lists and reorders the children of a project's work item through the REST API GitLab serves behind the work_item_rest_api flag; issue.work_item_get publishes the first page of them, and no action lists or reorders them"),
	tracked(issueIssueRoutes, 9, on(), "/namespaces/*/-/work_items/*/discussions/**",
		"lists, starts, reads, resolves and replies to the discussions of a work item of a namespace named by its ID, a project's or a user's as well as a group's, and reads, updates and deletes their notes, through the REST API GitLab serves behind the work_item_rest_api flag; no action does it through this API"),
	tracked(issueIssueRoutes, 3, on(), "/namespaces/*/-/work_items/*/notes/**",
		"lists, creates and reads the notes of a work item of a namespace named by its ID through the REST API GitLab serves behind the work_item_rest_api flag; no action does it through this API"),
	tracked(issueIssueRoutes, 2, on(http.MethodGet, http.MethodPut), "/namespaces/*/-/work_items/*/children/**",
		"lists and reorders the children of a work item of a namespace named by its ID through the REST API GitLab serves behind the work_item_rest_api flag; issue.work_item_get publishes the first page of them, and no action lists or reorders them through this API"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/award_emoji",
		"lists a work item's emoji reactions through the REST API GitLab serves behind the work_item_rest_api flag"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/closing_merge_requests",
		"lists the merge requests that close a work item through the REST API GitLab serves behind the work_item_rest_api flag"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/current_user_todos",
		"lists the caller's to-do items on a work item through the REST API GitLab serves behind the work_item_rest_api flag"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/email_participants",
		"lists a work item's email participants through the REST API GitLab serves behind the work_item_rest_api flag"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/feature_flags",
		"lists the feature flags linked to a work item through the REST API GitLab serves behind the work_item_rest_api flag"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/linked_items",
		"lists the items linked to a work item through the REST API GitLab serves behind the work_item_rest_api flag; issue.work_item_get publishes the first page of them, and no action lists them"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/linked_resources",
		"lists the resources linked to a work item through the REST API GitLab serves behind the work_item_rest_api flag"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/related_branches",
		"lists the branches related to a work item through the REST API GitLab serves behind the work_item_rest_api flag"),
	tracked(issueIssueRoutes, 3, on(http.MethodGet), "/*/*/-/work_items/*/related_merge_requests",
		"lists the merge requests related to a work item through the REST API GitLab serves behind the work_item_rest_api flag"),
	// The merge request domain (issueMergeRequestRoutes).
	tracked(issueMergeRequestRoutes, 1, on(http.MethodGet), "/projects/*/merge_requests/*/changes",
		"reads a merge request with its changes, through MergeRequests.GetMergeRequestChanges, which GitLab deprecates for the list of its diffs; mr_review.changes_get lists the diffs (GET /projects/:id/merge_requests/:merge_request_iid/diffs), another route"),
	tracked(issueMergeRequestRoutes, 1, on(http.MethodPost), "/projects/*/merge_requests/*/approvals",
		"changes the approvals a merge request requires, through MergeRequestApprovals.ChangeApprovalConfiguration, which GitLab deprecated in 16.0 for its approval rules (merge_request.approval_rule_*) and still serves"),
	tracked(issueMergeRequestRoutes, 1, on(http.MethodGet), "/projects/*/merge_requests/*/approval_rules/*",
		"reads one approval rule of a merge request, which merge_request.approval_rules lists"),
	tracked(issueMergeRequestRoutes, 1, on(http.MethodGet), "/projects/*/merge_requests/*/approval_settings",
		"lists a merge request's approval rules through the approval settings API"),
	tracked(issueMergeRequestRoutes, 1, on(http.MethodGet), "/projects/*/merge_requests/*/blockees",
		"lists the merge requests a merge request blocks; merge_request.dependencies_list lists those that block it"),
	tracked(issueMergeRequestRoutes, 1, on(http.MethodGet), "/projects/*/merge_requests/*/blocks/*",
		"reads one dependency of a merge request, which merge_request.dependency_create and merge_request.dependency_delete create and delete"),
	tracked(issueMergeRequestRoutes, 1, on(http.MethodGet), "/projects/*/merge_requests/*/merge_ref",
		"merges a merge request's changes into its merge ref (refs/merge-requests/:iid/merge) when they merge, which shows whether they do"),
	tracked(issueMergeRequestRoutes, 1, on(http.MethodGet), "/analytics/code_review",
		"lists a project's open merge requests with their review metrics (Code Review Analytics)"),
	// The merge request review domain (issueMRReviewRoutes).
	tracked(issueMRReviewRoutes, 2, on(http.MethodGet), "/projects/*/merge_requests/*/discussions/*/notes/**",
		"lists the comments of a merge request discussion and reads one; mr_review.discussion_get reads the discussion with its notes"),
	tracked(issueMRReviewRoutes, 2, on(http.MethodPut), "/suggestions/**",
		"applies a suggestion of a merge request comment, or several in one commit"),
	// The repository domain (issueRepositoryRoutes).
	tracked(issueRepositoryRoutes, 1, on(http.MethodGet), "/projects/*/repository/archive",
		"downloads a repository's archive; repository.archive builds this route's URL for the caller to fetch and sends nothing (the url-built waits entry), and no action calls Repositories.StreamArchive, which streams it: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodGet), "/projects/*/repository/changed_paths",
		"lists the files changed between two commits"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodGet), "/projects/*/repository/diff_stats",
		"reads the diff statistics between two commits"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodGet), "/projects/*/repository/diverging_commits",
		"counts the commits by which two refs diverge"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodGet), "/projects/*/repository/health",
		"reads a repository's health statistics"),
	tracked(issueRepositoryRoutes, 2, on(http.MethodGet), "/projects/*/repository/commits/*/discussions/*/notes/**",
		"lists the comments of a commit discussion and reads one; repository.commit_discussion_get reads the discussion with its notes"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodGet), "/projects/*/repository/commits/*/sequence",
		"reads a commit's sequence number, counted along its parent links"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodHead), "/projects/*/repository/files/*/blame",
		"reads a file's blame metadata without the body; repository.file_blame reads the blame"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodPost), "/projects/*/repository/blobs/batch",
		"reads the contents of several files in one request"),
	tracked(issueRepositoryRoutes, 1, on(http.MethodGet), "/projects/*/snapshot",
		"downloads a snapshot of a project's Git repository as a tar archive: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	publicEndpoint(issueRepositoryRoutes, 1, on(http.MethodGet), "/web_commits/public_key",
		"reads the public key GitLab signs commits made in the web interface with"),
	// The branch domain (issueBranchRoutes).
	tracked(issueBranchRoutes, 1, on(http.MethodHead), "/projects/*/repository/branches/*",
		"checks whether a branch exists without reading it; branch.get reads it"),
	tracked(issueBranchRoutes, 2, on(http.MethodPut), "/projects/*/repository/branches/*/*",
		"protects and unprotects a branch through the branches API; branch.protect and branch.unprotect do it through the protected branches API"),
	// The user domain (issueUserRoutes).
	tracked(issueUserRoutes, 5, on(), "/user/applications/**",
		"lists, reads, creates, updates and deletes the OAuth applications the caller owns; admin.application_* manage the instance's"),
	tracked(issueUserRoutes, 1, on(http.MethodPost), "/user/gpg_keys/*/revoke",
		"revokes one of the caller's GPG keys, which unverifies the commits it signed; user.delete_gpg_key deletes it"),
	tracked(issueUserRoutes, 1, on(http.MethodPost), "/users/*/gpg_keys/*/revoke",
		"an administrator revokes a user's GPG key; user.delete_gpg_key_for_user deletes it"),
	tracked(issueUserRoutes, 2, on(), "/user/preferences",
		"reads and updates the caller's preferences"),
	tracked(issueUserRoutes, 2, on(), "/user/push_subscriptions",
		"registers and unregisters a mobile device for push notifications"),
	tracked(issueUserRoutes, 1, on(http.MethodPatch), "/user/status",
		"updates the caller's status, leaving what it does not send as it is; user.set_status sends the PUT that replaces it"),
	tracked(issueUserRoutes, 2, on(), "/user/support_pin",
		"reads and creates the caller's Support PIN, which GitLab Support asks for to identify them"),
	tracked(issueUserRoutes, 2, on(), "/users/*/support_pin/**",
		"an administrator (assignable_when admin) reads a user's Support PIN and revokes it"),
	tracked(issueUserRoutes, 1, on(http.MethodGet), "/user_counts",
		"reads the caller's counts of assigned issues and merge requests, review requests and to-do items"),
	tracked(issueUserRoutes, 1, on(http.MethodPost), "/users/*/follow",
		"follows a user"),
	tracked(issueUserRoutes, 1, on(http.MethodPost), "/users/*/unfollow",
		"unfollows a user"),
	tracked(issueUserRoutes, 1, on(http.MethodGet), "/users/*/followers",
		"lists a user's followers"),
	tracked(issueUserRoutes, 1, on(http.MethodGet), "/users/*/following",
		"lists the users a user follows"),
	tracked(issueUserRoutes, 5, on(), "/vscode/settings_sync/**",
		"reads, writes and deletes the VS Code settings the caller syncs through GitLab: the manifest, a resource's references and one resource"),
	// The access domain (issueAccessRoutes).
	tracked(issueAccessRoutes, 4, on(http.MethodPut, http.MethodDelete), "/*/*/invitations/*",
		"updates and deletes a pending invitation to a group or a project, which access.invite_group and access.invite_project send and access.invite_list_* list"),
	tracked(issueAccessRoutes, 1, on(http.MethodGet), "/personal_access_tokens/self/associations",
		"lists the groups and projects the caller's token reaches"),
	tracked(issueAccessRoutes, 1, on(http.MethodPost), "/token_exchange",
		"exchanges a personal access token for a short-lived JWT of one modular service's audience; every personal access token may call it, which GitLab only keeps out of the fine-grained permissions (modular_service_token_exchange)"),
	// The administration domain (issueAdminRoutes).
	tracked(issueAdminRoutes, 7, on(), "/admin/active_context/**",
		"an administrator manages ActiveContext, the index of code embeddings GitLab Duo searches: lists its connections and activates or deactivates one, updates a collection's options and the namespaces it indexes, and clears or replays its dead queue"),
	tracked(issueAdminRoutes, 1, on(http.MethodPost), "/admin/ai_catalog/seed_external_agents",
		"an administrator seeds the AI catalog with the external agents GitLab manages"),
	tracked(issueAdminRoutes, 4, on(), "/admin/batched_background_migrations/**",
		"an administrator lists and reads a database's batched background migrations and pauses or resumes one"),
	tracked(issueAdminRoutes, 4, on(), "/admin/batched_background_operations/**",
		"an administrator lists and reads the batched background operations and restarts or stops one"),
	tracked(issueAdminRoutes, 4, on(), "/admin/data_management/**",
		"an administrator reads a data model's records and recalculates the checksum of one record or of all of them"),
	tracked(issueAdminRoutes, 1, on(http.MethodGet), "/admin/databases/*/dictionary/tables/*",
		"an administrator reads one table of a database's dictionary"),
	tracked(issueAdminRoutes, 1, on(http.MethodGet), "/databases/*/dictionary/tables",
		"lists the tables of a database's dictionary"),
	tracked(issueAdminRoutes, 3, on(), "/admin/knowledge_graph/namespaces/**",
		"an administrator lists the namespaces the Knowledge Graph indexes and enables or disables one"),
	tracked(issueAdminRoutes, 1, on(http.MethodGet), "/admin/migrations/pending",
		"an administrator lists the database migrations not yet run"),
	tracked(issueAdminRoutes, 2, on(http.MethodGet), "/admin/search/migrations/**",
		"an administrator lists the advanced search migrations and reads one"),
	tracked(issueAdminRoutes, 1, on(http.MethodDelete), "/admin/sidekiq/queues/*",
		"an administrator deletes the jobs of a Sidekiq queue that match the metadata given; admin.sidekiq_* read the Sidekiq metrics"),
	tracked(issueAdminRoutes, 3, on(), "/admin/ssh_certificates/**",
		"an administrator lists, adds and deletes the instance's SSH certificates"),
	tracked(issueAdminRoutes, 2, on(), "/admin/token",
		"an administrator identifies any token GitLab issued and revokes it, the token in the body"),
	tracked(issueAdminRoutes, 6, on(), "/admin/zoekt/**",
		"an administrator manages Zoekt, the exact code search: lists its nodes and the namespaces each indexes, adds or removes a namespace on a node, overrides a namespace's replica count and indexes a project"),
	tracked(issueAdminRoutes, 2, on(http.MethodPut), "/elasticsearch_indexed_namespaces/*",
		"an administrator rolls the namespaces Elasticsearch indexes out to, or back to, a percentage of all namespaces"),
	tracked(issueAdminRoutes, 5, on(), "/experiments/**",
		"a GitLab team member (assignable_when gitlab_team_member) lists GitLab's experiments, reads, forces or clears the variant a context is assigned and drops an experiment's cached assignments"),
	tracked(issueAdminRoutes, 3, on(), "/license/**",
		"an administrator reads a license by its ID, downloads the license usage export and recalculates a license's billable users; admin.license_get reads the current license, admin.license_add adds one and admin.license_delete deletes one"),
	tracked(issueAdminRoutes, 1, on(http.MethodGet), "/licenses",
		"an administrator lists every license the instance holds"),
	tracked(issueAdminRoutes, 3, on(), "/namespaces/*/gitlab_subscription",
		"reads a namespace's GitLab.com subscription, which an administrator (assignable_when admin) creates and updates"),
	tracked(issueAdminRoutes, 2, on(), "/namespaces/*/minutes/**",
		"an administrator records a namespace's purchase of compute minutes and moves purchased packs to another namespace"),
	tracked(issueAdminRoutes, 2, on(), "/namespaces/*/storage/limit_exclusion",
		"an administrator excludes a namespace from the storage limit and removes the exclusion"),
	tracked(issueAdminRoutes, 1, on(http.MethodGet), "/namespaces/storage/limit_exclusions",
		"an administrator lists the namespaces excluded from the storage limit"),
	tracked(issueAdminRoutes, 3, on(), "/namespaces/*/subscription_add_on_purchase/*",
		"an administrator reads, creates and updates a namespace's add-on purchase, such as its GitLab Duo seats"),
	tracked(issueAdminRoutes, 1, on(http.MethodPut), "/namespaces/*",
		"an administrator (assignable_when admin) updates a namespace's plan settings; user.namespace_get and user.namespace_list read namespaces"),
	tracked(issueAdminRoutes, 2, on(), "/hooks/*/custom_headers/*",
		"an administrator sets and deletes a system hook's custom header, beside the URL variables admin.system_hook_set_url_variable and admin.system_hook_delete_url_variable set and delete"),
	tracked(issueAdminRoutes, 1, on(http.MethodPost), "/topics/merge",
		"an administrator merges one topic into another, which admin.topic_* create, read, update and delete"),
	tracked(issueAdminRoutes, 2, on(http.MethodPost), "/usage_data/*",
		"tracks a Service Ping event for the caller, as a counter or as a unique user of the event; admin.usage_data_* read the metrics"),
	tracked(issueAdminRoutes, 1, on(http.MethodPut), "/user/*/credit_card_validation",
		"an administrator (assignable_when admin) records a user's credit card validation"),
	tracked(issueAdminRoutes, 3, on(), "/offline_exports/**",
		"starts, lists and reads an offline transfer export, which moves groups and projects to an instance that cannot reach this one; admin.bulk_import_* drive the direct transfer"),
	tracked(issueAdminRoutes, 1, on(http.MethodPost), "/offline_imports",
		"starts an offline transfer import of an export another instance produced"),
	tracked(issueAdminRoutes, 4, on(), "/projects/*/cluster_agents/*/url_configurations/**",
		"lists, reads, creates and deletes the URL configurations of an agent for Kubernetes, beside the agents and tokens admin.cluster_agent_* manage"),
	tracked(issueAdminRoutes, 1, on(http.MethodGet), "/projects/*/secure_files/*/download",
		"downloads a secure file, which admin.secure_file_* list, read, create and delete, through SecureFiles.DownloadSecureFile: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueAdminRoutes, 4, on(), "/projects/*/terraform/state_protection_rules/**",
		"lists, creates, updates and deletes a project's Terraform state protection rules"),
	tracked(issueAdminRoutes, 2, on(), "/projects/*/terraform/state/*",
		"downloads a Terraform state and writes one, the two requests of terraform's HTTP backend besides the lock admin.terraform_state_lock takes; the download, through TerraformStates.DownloadLatest, is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueAdminRoutes, 1, on(http.MethodGet), "/projects/*/terraform/state/*/versions/*",
		"downloads one version of a Terraform state, which admin.terraform_version_delete deletes, through TerraformStates.Download: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueAdminRoutes, 1, on(http.MethodPut), "/projects/*/error_tracking/settings",
		"creates a project's error tracking settings, through ErrorTracking.CreateErrorTrackingSettings; admin.error_tracking_update_settings sends the PATCH that updates them, another route"),
	// The package domain (issuePackageRoutes).
	protocol(debianRepository, 9, on(http.MethodGet), "/groups/*/-/packages/debian/dists/**",
		"the index files of a group's Debian distribution (Release, InRelease and its signature, the Packages and Sources indexes and their by-hash copies)"),
	download(issuePackageRoutes, 1, "/groups/*/-/packages/debian/pool/**",
		"downloads a Debian package file from a group's repository"),
	download(issuePackageRoutes, 1, "/groups/*/-/packages/maven/**",
		"downloads a Maven package file from a group's repository"),
	protocol("npm", 3, on(), "/groups/*/-/packages/npm/-/package/*/dist-tags/**",
		"the dist-tags of a package in a group's registry (npm dist-tag ls, add and rm)"),
	protocol("NuGet", 2, on(http.MethodGet), "/groups/*/-/packages/nuget/metadata/**",
		"the metadata of a package in a group's feed, by name and by version"),
	protocol("NuGet", 1, on(http.MethodGet), "/groups/*/-/packages/nuget/query",
		"the search of a group's feed"),
	publicEndpoint(issuePackageRoutes, 1, on(http.MethodGet), "/groups/*/-/packages/nuget/index",
		"reads the NuGet V3 service index of a group's feed"),
	publicEndpoint(issuePackageRoutes, 1, on(http.MethodGet), "/groups/*/-/packages/nuget/symbolfiles/**",
		"downloads a symbol file from a group's NuGet feed"),
	publicEndpoint(issuePackageRoutes, 2, on(http.MethodGet), "/groups/*/-/packages/nuget/v2/**",
		"reads the NuGet V2 service index of a group's feed and its $metadata document"),
	download(issuePackageRoutes, 1, "/groups/*/-/packages/pypi/files/**",
		"downloads a PyPI package file from a group's index"),
	protocol("PyPI", 2, on(http.MethodGet), "/groups/*/-/packages/pypi/simple/**",
		"the simple index of a group's packages and of one package"),
	protocol("Composer", 4, on(http.MethodGet), "/group/*/-/packages/composer/**",
		"the repository metadata of a group (packages.json, the p and p2 package metadata)"),
	protocol("Conan", 11, on(), "/packages/conan/v1/conans/**",
		"the v1 recipe and package references of the instance's remote: snapshots, manifests, download and upload URLs, search and the deletion of a recipe"),
	download(issuePackageRoutes, 2, "/packages/conan/v1/files/**",
		"downloads a recipe file or a package file from the instance's Conan remote"),
	protocol("Conan", 2, on(http.MethodPut), "/packages/conan/v1/files/**",
		"the upload of a recipe file or a package file to the instance's remote"),
	protocol("Conan", 2, on(http.MethodGet), "/packages/conan/v1/users/*",
		"the authentication of the instance's remote"),
	publicEndpoint(issuePackageRoutes, 1, on(http.MethodGet), "/packages/conan/v1/ping",
		"checks that the instance's Conan remote answers"),
	protocol("npm", 3, on(), "/packages/npm/-/package/*/dist-tags/**",
		"the dist-tags of a package in the instance's registry (npm dist-tag ls, add and rm)"),
	download(issuePackageRoutes, 1, "/packages/maven/**",
		"downloads a Maven package file from the instance's repository"),
	protocol(terraformModuleRegistry, 1, on(http.MethodGet), "/packages/terraform/modules/v1/*/*/*",
		"the latest version of a module in the instance's module registry"),
	protocol(terraformModuleRegistry, 3, on(http.MethodGet), "/packages/terraform/modules/v1/*/*/*/*",
		"a module version's details, the versions of a module and the download location of its latest version, in the instance's module registry"),
	protocol(terraformModuleRegistry, 1, on(http.MethodGet), "/packages/terraform/modules/v1/*/*/*/*/download",
		"the download location of a module version, which the registry answers with an X-Terraform-Get header"),
	download(issuePackageRoutes, 1, "/packages/terraform/modules/v1/*/*/*/*/file",
		"downloads a module version's archive from the instance's Terraform module registry"),
	protocol("Cargo", 2, on(http.MethodGet), "/projects/*/packages/cargo/*/*",
		"the sparse index of a crate whose name has one or two characters"),
	protocol("Cargo", 2, on(http.MethodGet), "/projects/*/packages/cargo/*/*/:package_name",
		"the sparse index of a crate whose name has three characters or more"),
	protocol("Cargo", 1, on(http.MethodGet), "/projects/*/packages/cargo/config.json",
		"the registry's config.json"),
	protocol("Cargo", 1, on(http.MethodPut), "/projects/*/packages/cargo/**",
		"the publication of a crate"),
	download(issuePackageRoutes, 1, "/projects/*/packages/cargo/*/*/download",
		"downloads a crate from a project's Cargo registry"),
	download(issuePackageRoutes, 1, "/projects/*/packages/composer/archives/**",
		"downloads a Composer package's archive from a project"),
	protocol("Composer", 1, on(http.MethodPost), "/projects/*/packages/composer",
		"the publication of a package from a tag or a branch"),
	protocol("Conan", 11, on(), "/projects/*/packages/conan/v1/conans/**",
		"the v1 recipe and package references of a project's remote: snapshots, manifests, download and upload URLs, search and the deletion of a recipe"),
	download(issuePackageRoutes, 2, "/projects/*/packages/conan/v1/files/**",
		"downloads a recipe file or a package file from a project's Conan v1 remote"),
	protocol("Conan", 2, on(http.MethodPut), "/projects/*/packages/conan/v1/files/**",
		"the upload of a recipe file or a package file to a project's v1 remote"),
	protocol("Conan", 2, on(http.MethodGet), "/projects/*/packages/conan/v1/users/*",
		"the authentication of a project's v1 remote"),
	publicEndpoint(issuePackageRoutes, 1, on(http.MethodGet), "/projects/*/packages/conan/v1/ping",
		"checks that a project's Conan remote answers"),
	protocol("Conan", 4, on(http.MethodDelete, http.MethodPut), "/projects/*/packages/conan/v2/**",
		"the upload of a v2 recipe file or package file and the deletion of a recipe or package revision"),
	protocol("Conan", 3, on(http.MethodGet), "/projects/*/packages/conan/v2/conans/*/*/*/*/*",
		"a v2 recipe's latest revision, its revisions and its package references"),
	protocol("Conan", 2, on(http.MethodGet), "/projects/*/packages/conan/v2/conans/*/*/*/*/revisions/*/*",
		"a v2 recipe revision's files and package references"),
	download(issuePackageRoutes, 1, "/projects/*/packages/conan/v2/conans/*/*/*/*/revisions/*/files/*",
		"downloads a recipe file from a project's Conan v2 remote"),
	protocol("Conan", 2, on(http.MethodGet), "/projects/*/packages/conan/v2/conans/*/*/*/*/revisions/*/packages/*/*",
		"a v2 package's latest revision and its revisions"),
	protocol("Conan", 1, on(http.MethodGet), "/projects/*/packages/conan/v2/conans/*/*/*/*/revisions/*/packages/*/revisions/*/files",
		"the files of a v2 package revision"),
	download(issuePackageRoutes, 1, "/projects/*/packages/conan/v2/conans/*/*/*/*/revisions/*/packages/*/revisions/*/files/*",
		"downloads a package file from a project's Conan v2 remote"),
	protocol("Conan", 1, on(http.MethodGet), "/projects/*/packages/conan/v2/conans/search",
		"the v2 search of a project's remote"),
	protocol("Conan", 2, on(http.MethodGet), "/projects/*/packages/conan/v2/users/*",
		"the authentication of a project's v2 remote"),
	protocol(debianRepository, 9, on(http.MethodGet), "/projects/*/packages/debian/dists/**",
		"the index files of a project's Debian distribution (Release, InRelease and its signature, the Packages and Sources indexes and their by-hash copies)"),
	download(issuePackageRoutes, 1, "/projects/*/packages/debian/pool/**",
		"downloads a Debian package file from a project's repository"),
	protocol(debianRepository, 1, on(http.MethodPut), "/projects/*/packages/debian/*",
		"the upload of a Debian package file"),
	protocol("Go module proxy", 1, on(http.MethodGet), "/projects/*/packages/go/*/@v/list",
		"the versions of a module in a project's proxy"),
	protocol("Go module proxy", 1, on(http.MethodGet), "/projects/*/packages/go/*/@v/:module_version.info",
		"a module version's metadata"),
	download(issuePackageRoutes, 1, "/projects/*/packages/go/*/@v/:module_version.mod",
		"downloads a Go module version's go.mod from a project's proxy"),
	download(issuePackageRoutes, 1, "/projects/*/packages/go/*/@v/:module_version.zip",
		"downloads a Go module version's source archive from a project's proxy"),
	download(issuePackageRoutes, 1, "/projects/*/packages/helm/*/charts/*",
		"downloads a Helm chart from a project's repository"),
	protocol("Helm", 1, on(http.MethodGet), "/projects/*/packages/helm/*/index.yaml",
		"a channel's chart index, which helm repo add and helm repo update read to find the charts"),
	protocol("Helm", 1, on(http.MethodPost), "/projects/*/packages/helm/api/*/charts",
		"the upload of a chart"),
	download(issuePackageRoutes, 1, "/projects/*/packages/maven/**",
		"downloads a Maven package file from a project's repository"),
	protocol("Maven", 1, on(http.MethodPut), "/projects/*/packages/maven/**",
		"the upload of a package file"),
	protocol("MLflow", 1, on(http.MethodPut), "/projects/*/packages/ml_models/**",
		"the upload of a model version's file to the model registry, whose files model_registry.download downloads"),
	protocol("npm", 2, on(http.MethodGet, http.MethodPut), "/projects/*/packages/npm/*",
		"a package's registry metadata and its publication or deprecation"),
	download(issuePackageRoutes, 1, "/projects/*/packages/npm/*/-/*",
		"downloads an npm package's tarball from a project's registry"),
	protocol("npm", 3, on(), "/projects/*/packages/npm/-/package/*/dist-tags/**",
		"the dist-tags of a package in a project's registry (npm dist-tag ls, add and rm)"),
	protocol("NuGet", 4, on(http.MethodDelete, http.MethodPut), "/projects/*/packages/nuget/**",
		"the push of a package, a symbol package or a V2 package to a project's feed and the deletion of a package"),
	download(issuePackageRoutes, 1, "/projects/*/packages/nuget/download/*/*/*",
		"downloads a NuGet package's content from a project's feed"),
	protocol("NuGet", 1, on(http.MethodGet), "/projects/*/packages/nuget/download/*/index",
		"the versions of a package in a project's feed"),
	protocol("NuGet", 2, on(http.MethodGet), "/projects/*/packages/nuget/metadata/**",
		"the metadata of a package in a project's feed, by name and by version"),
	protocol("NuGet", 1, on(http.MethodGet), "/projects/*/packages/nuget/query",
		"the search of a project's feed"),
	publicEndpoint(issuePackageRoutes, 1, on(http.MethodGet), "/projects/*/packages/nuget/index",
		"reads the NuGet V3 service index of a project's feed"),
	publicEndpoint(issuePackageRoutes, 1, on(http.MethodGet), "/projects/*/packages/nuget/symbolfiles/**",
		"downloads a symbol file from a project's NuGet feed"),
	publicEndpoint(issuePackageRoutes, 5, on(http.MethodGet), "/projects/*/packages/nuget/v2/**",
		"reads the NuGet V2 feed of a project: its service index, its $metadata document, the packages it holds and one package's metadata by ID and version"),
	tracked(issuePackageRoutes, 1, on(http.MethodGet), "/projects/*/packages/*/package_files/*/download",
		"downloads a file of a package by its ID; package.file_list lists them, and package.download downloads a generic package's file: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issuePackageRoutes, 1, on(http.MethodGet), "/projects/*/packages/*/pipelines",
		"lists the pipelines that built a package, which package.get reads"),
	download(issuePackageRoutes, 1, "/projects/*/packages/pypi/files/**",
		"downloads a PyPI package file from a project's index"),
	download(issuePackageRoutes, 1, "/projects/*/packages/pypi/forward/**",
		"downloads a PyPI package file a project's index forwards from PyPI"),
	protocol("PyPI", 2, on(http.MethodGet), "/projects/*/packages/pypi/simple/**",
		"the simple index of a project's packages and of one package"),
	protocol("PyPI", 1, on(http.MethodPost), "/projects/*/packages/pypi",
		"the upload of a package"),
	download(issuePackageRoutes, 1, "/projects/*/packages/rpm/*package_file_id/*",
		"downloads an RPM package file, named by its package file's ID, from a project's repository"),
	protocol("RPM repository", 1, on(http.MethodGet), "/projects/*/packages/rpm/repodata/*",
		"the repository's metadata files (repomd.xml and the primary, filelists and other indexes), which dnf and yum read to resolve packages"),
	protocol("RPM repository", 1, on(http.MethodPost), "/projects/*/packages/rpm",
		"the upload of a package"),
	protocol("RubyGems", 1, on(http.MethodGet), "/projects/*/packages/rubygems/*",
		"the spec indexes (specs, latest_specs and prerelease_specs), which gem reads to find the gems a repository holds"),
	download(issuePackageRoutes, 1, "/projects/*/packages/rubygems/gems/*",
		"downloads a gem from a project's repository"),
	protocol("RubyGems", 1, on(http.MethodGet), "/projects/*/packages/rubygems/quick/**",
		"a gem version's marshaled gemspec, which gem reads to resolve a gem before it downloads it"),
	protocol("RubyGems", 2, on(), "/projects/*/packages/rubygems/api/**",
		"the dependencies of the gems asked for, which Bundler reads, and the push of a gem"),
	download(issuePackageRoutes, 2, "/projects/*/packages/terraform/modules/**",
		"downloads a module's latest version or one version from a project's Terraform module registry"),
	protocol(terraformModuleRegistry, 1, on(http.MethodPut), "/projects/*/packages/terraform/modules/**",
		"the upload of a module version"),
	download(issuePackageRoutes, 2, "/projects/*/dependency_proxy/packages/**",
		"downloads a Maven package file or an npm tarball through a project's dependency proxy for packages, which fetches it from the upstream registry"),
	tracked(issuePackageRoutes, 6, on(), "/projects/*/debian_distributions/**",
		"lists, reads, creates, updates and deletes a project's Debian distributions and reads one's signing key, the API that configures the repository apt reads"),
	tracked(issuePackageRoutes, 6, on(), "/groups/*/-/debian_distributions/**",
		"lists, reads, creates, updates and deletes a group's Debian distributions and reads one's signing key, the API that configures the repository apt reads"),
	// The model registry domain (issueModelRegistryRoutes).
	protocol("MLflow", 28, on(), "/projects/*/ml/mlflow/api/2.0/mlflow/**",
		"the tracking and model registry API: experiments, runs and their metrics, parameters and tags, registered models, their versions and aliases"),
	protocol("MLflow", 1, on(http.MethodGet), "/projects/*/ml/mlflow/api/2.0/mlflow-artifacts/artifacts",
		"the list of a model version's artifacts"),
	download(issueModelRegistryRoutes, 1, "/projects/*/ml/mlflow/api/2.0/mlflow-artifacts/artifacts/*/*",
		"downloads a model version's artifact file through the MLflow artifacts API; model_registry.download downloads a model package's files"),
	// The virtual registry domain (issueVirtualRegistryRoutes).
	tracked(issueVirtualRegistryRoutes, 16, on(), "/virtual_registries/container/**",
		"manages the container virtual registries: a registry, its upstreams and their place in it, their caches and cache entries, and a test of an upstream's connection"),
	tracked(issueVirtualRegistryRoutes, 9, on(), "/virtual_registries/packages/npm/**",
		"manages the npm virtual registries: a registry, its upstreams and their place in it, their caches and cache entries"),
	tracked(issueVirtualRegistryRoutes, 1, on(), "/virtual_registries/packages/maven/cache_entries/**",
		"deletes a cache entry of a Maven virtual registry upstream"),
	tracked(issueVirtualRegistryRoutes, 3, on(), "/virtual_registries/packages/maven/local/**",
		"reads, updates and deletes a Maven virtual registry's local upstream"),
	tracked(issueVirtualRegistryRoutes, 7, on(), "/virtual_registries/packages/maven/registries/**",
		"reads, updates and deletes a Maven virtual registry, purges its cache and lists, adds and adds as local its upstreams"),
	tracked(issueVirtualRegistryRoutes, 3, on(), "/virtual_registries/packages/maven/registry_upstreams/**",
		"places an upstream in a Maven virtual registry, moves it and removes it"),
	tracked(issueVirtualRegistryRoutes, 7, on(), "/virtual_registries/packages/maven/upstreams/**",
		"reads, updates and deletes a Maven virtual registry upstream, purges its cache, lists its cache entries and tests its connection"),
	download(issueVirtualRegistryRoutes, 1, "/virtual_registries/packages/maven/:id/*path",
		"downloads a Maven package file through a Maven virtual registry"),
	protocol("Maven", 1, on(http.MethodPost), "/virtual_registries/packages/maven/:id/*path/upload",
		"the cache fill of a download through a Maven virtual registry, the file Workhorse fetched from the upstream, which only Workhorse may post (require_gitlab_workhorse!)"),
	tracked(issueVirtualRegistryRoutes, 14, on(), "/groups/*/-/virtual_registries/**",
		"lists and creates a group's container, Maven and npm virtual registries and upstreams, tests an upstream's connection and manages the group's virtual registry cleanup policy"),
	// The vulnerability domain (issueVulnerabilityRoutes).
	overGraphQL([]string{"vulnerability.confirm"}, 1, on(http.MethodPost), "/vulnerabilities/*/confirm",
		"confirms a vulnerability, which vulnerability.confirm does over GraphQL"),
	overGraphQL([]string{"vulnerability.dismiss"}, 1, on(http.MethodPost), "/vulnerabilities/*/dismiss",
		"dismisses a vulnerability, which vulnerability.dismiss does over GraphQL"),
	overGraphQL([]string{"vulnerability.resolve"}, 1, on(http.MethodPost), "/vulnerabilities/*/resolve",
		"resolves a vulnerability, which vulnerability.resolve does over GraphQL"),
	overGraphQL([]string{"vulnerability.revert"}, 1, on(http.MethodPost), "/vulnerabilities/*/revert",
		"reverts a vulnerability to detected, which vulnerability.revert does over GraphQL"),
	overGraphQL([]string{"vulnerability.get"}, 1, on(http.MethodGet), "/vulnerabilities/*",
		"reads a vulnerability, which vulnerability.get does over GraphQL"),
	overGraphQL([]string{"security_finding.list"}, 1, on(http.MethodGet), "/projects/*/vulnerability_findings",
		"lists the vulnerability findings of a project's pipeline, which security_finding.list does over GraphQL"),
	tracked(issueVulnerabilityRoutes, 3, on(), "/vulnerabilities/*/issue_links/**",
		"lists a vulnerability's related issues, links one and removes a link"),
	tracked(issueVulnerabilityRoutes, 1, on(http.MethodPost), "/vulnerabilities/*/flags/ai_detection",
		"records the result of AI false positive detection on a vulnerability"),
	tracked(issueVulnerabilityRoutes, 6, on(), "/projects/*/vulnerabilities/**",
		"creates a vulnerability from a finding and lists, reads, creates, updates and deletes a vulnerability's notes; ProjectVulnerabilities.CreateVulnerability is decided COVERED_GRAPHQL with its service, and no GraphQL action creates one"),
	tracked(issueVulnerabilityRoutes, 1, on(http.MethodGet), "/occurrences/vulnerabilities",
		"lists the vulnerabilities of a dependency occurrence; dependency.list lists the dependencies"),
	tracked(issueVulnerabilityRoutes, 1, on(http.MethodPost), "/projects/*/security_scans/sast/*",
		"scans a file's content with a SAST scanner service of GitLab.com (assignable_when saas)"),
	tracked(issueVulnerabilityRoutes, 3, on(), "/security/vulnerability_exports/**",
		"creates an export of the instance's vulnerabilities, reads an export and downloads it"),
	tracked(issueVulnerabilityRoutes, 2, on(), "/security/vulnerability_archive_exports/**",
		"reads an export of archived vulnerabilities and downloads it"),
	tracked(issueVulnerabilityRoutes, 2, on(http.MethodPost), "/security/*/*/vulnerability_exports",
		"creates an export of a group's or a project's vulnerabilities"),
	tracked(issueVulnerabilityRoutes, 1, on(http.MethodPost), "/security/*/*/vulnerability_archive_exports",
		"creates an export of a project's archived vulnerabilities"),
	// The security policy domain (issueCompliancePolicyRoutes).
	tracked(issueCompliancePolicyRoutes, 10, on(), "/*/*/security/policy_store/**",
		"lists, reads, creates, updates and deletes the security policies a group or an organization stores"),
	tracked(issueCompliancePolicyRoutes, 3, on(http.MethodGet), "/security/policy_store/*",
		"lists the actions, rules and triggers a security policy is built from"),
	// The Orbit domain (issueOrbitRoutes). The status, schema, tool, query
	// DSL and index routes the orbit.* actions sent until GitLab.com removed
	// them inherit SUPERSEDED_UPSTREAM; the actions read GraphQL and the POST
	// of the MCP endpoint instead (rows 144 to 148 of
	// docs/development/upstream-bugs.md).
	tracked(issueOrbitRoutes, 1, on(http.MethodGet), "/orbit/mcp",
		"the SSE listener of Orbit's MCP endpoint; orbit.tools, orbit.dsl and orbit.schema send its JSON-RPC POST through orbit.callMCP and read the one answer it gives"),
	tracked(issueOrbitRoutes, 1, on(http.MethodGet), "/orbit/query/templates",
		"lists the named query templates, which GitLab.com serves as GraphQL Query.orbit.queryTemplates since gitlab-org/gitlab!265633 removed the route"),
	tracked(issueOrbitRoutes, 1, on(http.MethodPost), "/orbit/query/*",
		"runs a named query template; orbit.query runs a query the caller writes"),
	tracked(issueOrbitRoutes, 1, on(http.MethodGet), "/orbit/schema/format",
		"reads the guidance on Orbit's response formats, which gitlab-org/gitlab!265633 removed from GitLab.com"),
	tracked(issueOrbitRoutes, 3, on(http.MethodGet), "/orbit/skills/**",
		"lists the deployed Orbit skills and reads one and one of its files"),
	tracked(issueOrbitRoutes, 2, on(), "/orbit/agent/commands/**",
		"lists the Orbit agent commands and invokes one; orbit.dsl read the list until gitlab-org/gitlab!266009 removed it from GitLab.com, and reads the query command from list_commands of the MCP endpoint since"),
	// The GitLab Duo domain (issueDuoRoutes).
	tracked(issueDuoRoutes, 23, on(), "/ai/duo_workflows/**",
		"the GitLab Duo Agent Platform's flows: creates, reads, updates, restarts, resumes and executes a workflow, writes and reads its checkpoints and events, reads its trace, the tools and agent privileges it may use and the connection details of its service and WebSocket, reads the Duo Code Review custom instructions, ingests audit events and revokes the workflow's token. Fourteen skip the fine-grained check for ai_workflows_oauth_auth or ai_workflows_token_auth, which a user's token with the api scope calls"),
	tracked(issueDuoRoutes, 1, on(http.MethodPost), "/ai/billable_usage/**",
		"posts GitLab Duo billing events as a Snowplow tracker payload, a route the record leaves without a fine-grained permission"),
	tracked(issueDuoRoutes, 1, on(http.MethodPost), "/ai/llm/git_command",
		"generates Git commands from a description in natural language"),
	tracked(issueDuoRoutes, 1, on(http.MethodPost), "/ai/third_party_agents/direct_access",
		"returns the connection details a third-party agent reaches the Duo Agent Platform with"),
	tracked(issueDuoRoutes, 1, on(http.MethodPost), "/chat/completions",
		"asks GitLab Duo Chat for a response"),
	tracked(issueDuoRoutes, 4, on(http.MethodPost), "/code_suggestions/*",
		"GitLab Duo Code Suggestions: generates completions, returns the connection details and the direct access to the AI gateway, and checks that code suggestions are enabled"),
	tracked(issueDuoRoutes, 2, on(), "/mcp",
		"GitLab's own MCP server, a JSON-RPC handler and its SSE listener"),
	tracked(issueDuoRoutes, 5, on(), "/projects/*/ai_agent/**",
		"registers an external agent's identity on a project, creates, lists and completes its sessions and ingests its audit events"),
	// The job domain (issueJobRoutes).
	tracked(issueJobRoutes, 1, on(http.MethodGet), "/projects/*/jobs/*/artifacts/tree",
		"lists the files in a job's artifacts archive; job.download_single_artifact downloads one"),
	// The runner domain (issueRunnerRoutes).
	tracked(issueRunnerRoutes, 1, on(http.MethodDelete), "/runners/managers",
		"deletes a registered runner manager with the runner's authentication token, which runner actions already send, so runner_token_auth excludes none of its routes"),
	tracked(issueRunnerRoutes, 1, on(http.MethodPost), "/jobs/request",
		"requests a job for a runner with its authentication token, which runner actions already send, so runner_token_auth excludes none of its routes"),
	tracked(issueRunnerRoutes, 1, on(http.MethodGet), "/runners/*/projects",
		"lists the projects a project runner is assigned to"),
	// The wiki domain (issueWikiRoutes).
	tracked(issueWikiRoutes, 5, on(), "/projects/*/wiki_pages/*/notes/**",
		"lists, reads, creates, updates and deletes the comments on a project wiki page; wiki.* manage the pages"),
	// The snippet domain (issueSnippetRoutes).
	tracked(issueSnippetRoutes, 2, on(http.MethodGet), "/projects/*/snippets/*/discussions/*/notes/**",
		"lists the comments of a project snippet discussion and reads one; snippet.discussion_get reads the discussion with its notes"),
	tracked(issueSnippetRoutes, 1, on(http.MethodGet), "/projects/*/snippets/*/files/*/*/raw",
		"reads a file of a project snippet's repository at a ref; snippet.file_content reads one of a personal snippet"),
	tracked(issueSnippetRoutes, 1, on(http.MethodGet), "/projects/*/snippets/*/user_agent_detail",
		"an administrator (assignable_when admin) reads the user agent a project snippet was created with, for spam review"),
	tracked(issueSnippetRoutes, 1, on(http.MethodGet), "/snippets/*/user_agent_detail",
		"an administrator (assignable_when admin) reads the user agent a personal snippet was created with, for spam review"),
	// The release domain (issueReleaseRoutes).
	tracked(issueReleaseRoutes, 1, on(http.MethodGet), "/projects/*/releases/*/downloads/**",
		"downloads a release asset by its direct asset path: a download is an action to implement, bounded by toolutil.MaxDownloadBytes"),
	tracked(issueReleaseRoutes, 1, on(http.MethodPost), "/projects/*/releases/*/evidence",
		"collects a release's evidence"),
	// The environment domain (issueEnvironmentRoutes).
	tracked(issueEnvironmentRoutes, 1, on(http.MethodDelete), "/projects/*/environments/review_apps",
		"schedules a project's stopped review apps for deletion"),
	tracked(issueEnvironmentRoutes, 1, on(http.MethodPost), "/projects/*/environments/stop_stale",
		"stops a project's environments last updated before a date"),
	// The feature flag domain (issueFeatureFlagsRoutes).
	tracked(issueFeatureFlagsRoutes, 2, on(), "/projects/*/feature_flags_settings",
		"reads and updates a project's feature flag settings"),
	// The search domain (issueSearchRoutes).
	tracked(issueSearchRoutes, 1, on(http.MethodPost), "/glql",
		"runs a query in GLQL, the GitLab Query Language, for which a fine-grained token needs read_glql"),
	publicEndpoint(issueSearchRoutes, 1, on(http.MethodGet), "/glql/schema",
		"reads the schema of GLQL, the GitLab Query Language"),
	tracked(issueSearchRoutes, 1, on(http.MethodGet), "/projects/*/search/semantic",
		"searches a project's code in natural language"),
	// The Geo domain (issueGeoRoutes).
	tracked(issueGeoRoutes, 1, on(http.MethodPost), "/geo/node_proxy/*/graphql",
		"queries the GraphQL endpoint of another Geo site through this one"),
	// The member role domain (issueMemberRoleRoutes).
	tracked(issueMemberRoleRoutes, 3, on(), "/admin_member_roles/**",
		"an administrator lists, creates and deletes the instance's admin roles; member_role.* manage the member roles"),
	// The DORA metrics domain (issueDORAMetricsRoutes).
	tracked(issueDORAMetricsRoutes, 1, on(http.MethodGet), "/projects/*/analytics/deployment_frequency",
		"lists a project's deployment frequencies; dora_metrics.project reads its DORA metrics"),
	// The CI/CD catalog domain (issueCICatalogRoutes).
	tracked(issueCICatalogRoutes, 1, on(http.MethodPost), "/projects/*/catalog/publish",
		"publishes a component project's release as a version of the CI/CD catalog, which ci_catalog.list and ci_catalog.get read"),
	// The dependency domain (issueDependencyRoutes).
	tracked(issueDependencyRoutes, 2, on(http.MethodPost), "/*/*/dependency_list_exports",
		"exports a group's or a project's dependency list; dependency.export_create exports a pipeline's"),
	// The enterprise user domain (issueEnterpriseUserRoutes).
	tracked(issueEnterpriseUserRoutes, 1, on(http.MethodPatch), "/groups/*/enterprise_users/*",
		"updates an enterprise user's attributes; enterprise_user.* read and delete one and disable its two-factor authentication"),
}
