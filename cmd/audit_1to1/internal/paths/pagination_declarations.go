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
		Package:  toolsDir + "/groupsaml",
		Action:   "group.saml_link_list",
		Category: categoryEndpointNotPaginated,
		Reason: "GET /groups/:id/saml_group_links declares no page and no per_page, and its handler presents " +
			"group.saml_group_links without paginate (ee/lib/api/saml_group_links.rb): GitLab renders every SAML " +
			"group link of a group in one array. The endpoint that matched is GET /groups/:id/saml_users, the SAML " +
			"user list of the same package, which is paginated.",
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

// staleDeclarations names the declarations that accounted for no finding.
//
// A declaration stops matching when the action gained its pagination, was
// renamed, moved package, or stopped reading a collection, and in each case the
// excuse now outlives the thing it excused. This is the half of the rule that
// gates, on the same terms every declaration table here is held to: the findings
// report and the table's own integrity does not.
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
	sort.Strings(stale)
	return stale
}
