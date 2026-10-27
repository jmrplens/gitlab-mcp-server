// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import "sort"

// paginationDeclaration records why an action that reads a collection and
// publishes no pagination is not a defect.
//
// It is the same shape [endpointDeclaration], [shapeDeclaration] and
// [silentOwnerDeclaration] have, and it exists for the one reason this rule has
// a weakness at all: the join from an action to an endpoint runs through the
// package, so an action whose own GitLab route paginates nothing is asked about
// anyway when a sibling of its package calls a route that does. Every entry
// below names the route the live record holds for that action and what its
// params are, which is the evidence a later reader disagrees with.
//
// The params are not the whole of that evidence, and reading them as if they
// were is how four entries of this table came to excuse real findings (issue
// 1024). A route whose handler ends in paginate(...) is paged whether or not it
// declares page and per_page: GitLab's offset paginator reads both from the
// request's params, which Grape fills with every parameter the request
// carried, and only declared(params) narrows them. So a route declaring
// neither can still answer twenty rows and the headers of a longer list, as an
// issue's participants and a merge request's participants, reviewers,
// pipelines and context commits all do. An entry in the
// endpoint-declares-no-page category therefore has to say what the handler's
// body presents, read from GitLab's source, as well as what the record
// declares. That is an obligation on whoever writes the entry and not a
// check: nothing in R-PAGE reads the handler, so the stale check holds an
// entry to the record and never to the body, which is the blind spot that
// let the four stand.
//
// It is deliberately not a place to record "GitLab paginates this and we have
// decided not to". There is no such entry and there should not be one: that is
// the finding.
type paginationDeclaration struct {
	// Package owns the action, spelled as the inventory spells a package.
	Package string
	// Action is the canonical catalog ID.
	Action string
	// Category says what kind of non-defect this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// Pagination declaration categories.
const (
	// categoryEndpointNotPaginated is the package grain showing through: the
	// route GitLab mounts for this action declares neither page nor per_page,
	// its handler presents the collection without paginate(...), and the
	// paginated endpoint that put the action on the list belongs to another
	// action of the same package. The second condition is not in the record and
	// is the one that decides: a handler calling paginate pages whatever the
	// route declares.
	categoryEndpointNotPaginated = "endpoint-declares-no-page"

	// categoryGraphQLBacked is an action GitLab answers over GraphQL, so the
	// REST record holds no route for it at all and the package's REST endpoints
	// are the only reason it was asked about. A GraphQL connection pages with a
	// cursor, and where one is paged the output publishes the cursor block
	// instead.
	categoryGraphQLBacked = "graphql-backed"
)

// declaredUnpaginatedCollections holds every collection-reading action the
// package-grain join reports that GitLab does not paginate.
//
// The bar for an entry is the route and the handler behind it: the live record
// names the route and its params, and GitLab's source says whether the handler
// pages what it presents, which the params alone do not (see
// [paginationDeclaration]). Each reason below cites both, the handler as read
// in a 19.5.0-pre checkout (f23a2b35383). A finding that merely looks wrong is
// not one of these.
var declaredUnpaginatedCollections = []paginationDeclaration{
	{
		Package:  toolsDir + "/epics",
		Action:   "group.epic_get_links",
		Category: categoryEndpointNotPaginated,
		Reason: "the child epics of one epic come from GET /groups/:id/(-/)epics/:epic_iid/epics, which the record " +
			"holds with no page and no per_page, and whose handler presents child_epics, the finder's whole result, " +
			"without paginate (ee/lib/api/epic_links.rb). What put this on the list is the epic list endpoints of " +
			"the same package, GET /groups/:id/epics, which GitLab does paginate.",
	},
	{
		Package:  toolsDir + "/groupmilestones",
		Action:   "group.group_milestone_burndown",
		Category: categoryEndpointNotPaginated,
		Reason: "GET /groups/:id/milestones/:milestone_id/burndown_events declares no page and no per_page, and its " +
			"handler (ee/lib/ee/api/group_milestones.rb) calls milestone_burndown_events_for, which presents " +
			"Burndown.new(issues, start_date, due_date).as_json without paginate (ee/lib/ee/api/milestone_responses.rb): " +
			"GitLab sends every event of the milestone in one array. The endpoints that matched are the group milestone " +
			"list and the milestone's issues and merge requests, which the same package reads and GitLab paginates.",
	},
	{
		Package:  toolsDir + "/groupsaml",
		Action:   "group.saml_link_list",
		Category: categoryEndpointNotPaginated,
		Reason: "GET /groups/:id/saml_group_links declares no page and no per_page, and its handler presents " +
			"group.saml_group_links without paginate (ee/lib/api/saml_group_links.rb): GitLab renders every SAML " +
			"group link of a group in one array. The endpoint that matched is GET /groups/:id/saml_users, the SAML " +
			"user list of the same package, which is paginated.",
	},
	{
		Package:  toolsDir + "/milestones",
		Action:   "project.milestone_burndown",
		Category: categoryEndpointNotPaginated,
		Reason: "GET /projects/:id/milestones/:milestone_id/burndown_events declares no page and no per_page, and its " +
			"handler presents Burndown.new(issues, start_date, due_date).as_json without paginate " +
			"(ee/lib/ee/api/milestone_responses.rb): GitLab sends every event of the milestone in one array. The " +
			"endpoints that matched are the milestone list and the milestone's issues and merge requests, which the " +
			"same package reads and GitLab paginates.",
	},
	{
		Package:  toolsDir + "/pipelines",
		Action:   "pipeline.variables",
		Category: categoryEndpointNotPaginated,
		Reason: "GET /projects/:id/pipelines/:pipeline_id/variables declares neither param, which is worth reading " +
			"beside the three CI variable list endpoints that do (project, group and instance), and its handler " +
			"presents pipeline.variables without paginate (lib/api/ci/pipelines.rb). The variables a pipeline ran " +
			"with are fixed at the moment it started and GitLab sends all of them.",
	},
	{
		Package:  toolsDir + "/projects",
		Action:   "project.languages",
		Category: categoryEndpointNotPaginated,
		Reason: "GET /projects/:id/languages declares neither param and does not answer with an array at all: its " +
			"handler returns one hash mapping a language to its share of the repository (lib/api/projects.rb), " +
			"which this server publishes as a list so a model reads it in order. There is no next page to ask for.",
	},
	{
		Package:  toolsDir + "/projects",
		Action:   "project.target_branch_rule_list",
		Category: categoryGraphQLBacked,
		Reason: "the target branch rules are read through the GraphQL project(fullPath:) field, which is why the " +
			"action refuses a numeric project ID. The record holds no REST route for them, so the eighteen paginated " +
			"REST endpoints of the projects package are the only reason this was asked about. The connection returns " +
			"every rule in one response, which ListTargetBranchRulesOutput's own comment records.",
	},
	{
		Package:  toolsDir + "/runners",
		Action:   "runner.list_managers",
		Category: categoryEndpointNotPaginated,
		Reason: "GET /runners/:id/managers declares neither param, and its handler presents runner.runner_managers " +
			"without paginate (lib/api/ci/runners.rb). The paginated endpoints of the package are the runner lists, " +
			"GET /runners, GET /projects/:id/runners and GET /groups/:id/runners.",
	},
}

// declaredUnpaginated finds the declaration covering one finding.
func declaredUnpaginated(finding UnpaginatedCollection) (paginationDeclaration, bool) {
	for _, declaration := range declaredUnpaginatedCollections {
		if declaration.covers(finding) {
			return declaration, true
		}
	}
	return paginationDeclaration{}, false
}

// covers reports whether this declaration accounts for one finding.
func (d paginationDeclaration) covers(finding UnpaginatedCollection) bool {
	return d.Package == finding.Package && d.Action == finding.Action
}

// key names one declaration in a report, which is how a stale one is reported.
func (d paginationDeclaration) key() string { return d.Package + " " + d.Action }

// classifyUnpaginated attaches the declaration that accounts for each finding.
func classifyUnpaginated(found []UnpaginatedCollection) []UnpaginatedCollection {
	classified := make([]UnpaginatedCollection, 0, len(found))
	for _, finding := range found {
		if declaration, ok := declaredUnpaginated(finding); ok {
			finding.Category = declaration.Category
			finding.Reason = declaration.Reason
		}
		classified = append(classified, finding)
	}
	return classified
}

// keysetRouteDeclaration records a route the live record marks as reaching a
// keyset paginator where GitLab serves this server no keyset page it can
// follow, which the record cannot say: it records where a route's code calls a
// keyset paginator, and whether a page comes of it depends on what GitLab's
// handler hands that paginator and on who is asking ([apilive.KeysetSite]).
//
// A declared route is held to as one this server cannot page by keyset. An
// action offering page_token on it is therefore on the offset list, with the
// route named in its not_followable, since offering the parameter there is the
// finding; and a list sending it without offering page_token is declared, by
// this entry, rather than reported as a list that cannot ask for the cheaper
// way through it. The bar for an entry is the one [paginationDeclaration] sets:
// what GitLab's handler does, read from its source at the recorded release,
// with the file and line.
type keysetRouteDeclaration struct {
	// Route is spelled as the request record spells a route
	// ([apilive.RouteName]).
	Route string
	// Category says what kind of non-defect this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// categoryKeysetNotFollowable is the one keyset category: the record marks the
// route as reaching a keyset paginator, and no request this server sends gets
// a keyset page there whose next link leads anywhere.
const categoryKeysetNotFollowable = "keyset-not-followable"

// declaredKeysetRoutes are the routes the record marks as reaching a keyset
// paginator that this server cannot page by keyset, each read in GitLab's
// source at v19.5.0-ee, the release the record was taken from.
//
// Each reason is one literal rather than lines joined with +, for the reason
// [paginationGrain] gives: gremlins reads a + in a package-level initializer as
// arithmetic, reports its mutant as not covered, and the - it would become does
// not compile.
var declaredKeysetRoutes = []keysetRouteDeclaration{
	{
		Route:    "GET /groups",
		Category: categoryKeysetNotFollowable,
		Reason:   "the record finds paginate_with_strategies through present_groups_with_pagination_strategies (lib/api/groups.rb:181), and that helper returns present_groups, which pages by offset with paginate (lib/api/groups.rb:152), whenever current_user is present (line 171). Only an anonymous request reaches the keyset branch, and this server always signs in, so pagination=keyset and its cursor are ignored.",
	},
	{
		Route:    "GET /groups/:id/service_accounts/:user_id/personal_access_tokens",
		Category: categoryKeysetNotFollowable,
		Reason:   "the handler pages PersonalAccessTokensFinder's relation with paginate_with_strategies (lib/api/group_service_accounts.rb:240), and PersonalAccessToken declares no supported_keyset_orderings, so keyset_paginator takes Keyset::Pager, whose availability check admits Project alone, and answers every keyset request with 405 (lib/api/helpers/pagination_strategies.rb:30-43). The record's cursor is CursorPager's, which GitLab never reaches here.",
	},
	{
		Route:    "GET /projects/:id/service_accounts/:user_id/personal_access_tokens",
		Category: categoryKeysetNotFollowable,
		Reason:   "the handler pages PersonalAccessTokensFinder's relation with paginate_with_strategies (lib/api/project_service_accounts.rb:217), and PersonalAccessToken declares no supported_keyset_orderings, so keyset_paginator takes Keyset::Pager, whose availability check admits Project alone, and answers every keyset request with 405 (lib/api/helpers/pagination_strategies.rb:30-43). The record's cursor is CursorPager's, which GitLab never reaches here.",
	},
	{
		Route:    "GET /users/:user_id/contributed_projects",
		Category: categoryKeysetNotFollowable,
		Reason:   "present_projects reaches Keyset::Pager as on the other project lists (lib/api/projects.rb:248), but the handler builds ContributedProjectsFinder with no params (lib/api/projects.rb:396) and Keyset::Pager#paginate only limits the relation (lib/gitlab/pagination/keyset/pager.rb:13), so the id_after or id_before of the next link GitLab writes is never applied and every keyset page is the first again (upstream-bugs row 112).",
	},
}

// declaredKeysetRoute finds the declaration of one route.
func declaredKeysetRoute(route string) (keysetRouteDeclaration, bool) {
	for _, declaration := range declaredKeysetRoutes {
		if declaration.Route == route {
			return declaration, true
		}
	}
	return keysetRouteDeclaration{}, false
}

// unusedKeysetDeclarations names the route declarations no finding used: none
// names the route as one this server cannot follow, and none is a list the
// declaration answers. A declaration stops being used when the record stops
// marking its route, which is GitLab no longer reaching a keyset paginator
// there or the walk no longer seeing it, or when no action sends the route.
func unusedKeysetDeclarations(found []KeysetFinding) []string {
	used := map[string]bool{}
	for _, finding := range found {
		for _, route := range finding.NotFollowable {
			used[route] = true
		}
		if finding.declared() {
			used[finding.Routes[0]] = true
		}
	}
	unused := make([]string, 0)
	for _, declaration := range declaredKeysetRoutes {
		if used[declaration.Route] {
			continue
		}
		unused = append(unused, declaration.Route+
			" is declared a route this server cannot page by keyset ("+declaration.Category+
			") and no finding uses it: the record no longer marks it as reaching a keyset paginator, or no action sends it")
	}
	return unused
}

// staleDeclarations names the declarations that accounted for no finding.
//
// A declaration stops matching when the action gained its pagination, was
// renamed, moved package, or stopped reading a collection, and in each case the
// excuse now outlives the thing it excused. This is the half of the rule that
// gates, on the same terms every declaration table here is held to: the findings
// report and the table's own integrity does not. The keyset half's route
// declarations join it, from [KeysetPagination.UnusedDeclarations].
func (c PaginationCheck) staleDeclarations() []string {
	if !c.Ran {
		return nil
	}
	used := map[string]bool{}
	for _, finding := range c.Unpaginated {
		if declaration, ok := declaredUnpaginated(finding); ok {
			used[declaration.key()] = true
		}
	}
	stale := make([]string, 0)
	for _, declaration := range declaredUnpaginatedCollections {
		if used[declaration.key()] {
			continue
		}
		stale = append(stale, declaration.key()+
			" is declared to read a collection GitLab does not paginate ("+declaration.Category+
			") and matches no finding: it either publishes pagination now, stopped reading a collection, or was renamed")
	}
	stale = append(stale, c.Keyset.UnusedDeclarations...)
	sort.Strings(stale)
	return stale
}
