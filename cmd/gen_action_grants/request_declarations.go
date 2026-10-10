package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
)

// The categories of a request declaration, each one a class of request the
// walk reaches and cannot read.
const (
	// categorySendsNothing is an action that builds something for the caller
	// and sends no request at all.
	categorySendsNothing = "sends-nothing"
	// categoryTemplate is a client-go GraphQL document assembled from a
	// text/template at run time out of values the handler hands the method,
	// which graphqldocs leaves a shell because no rendering of it exists
	// without them.
	categoryTemplate = "sdk-graphql-template"
	// categorySprintfPath is a request path client-go formats for a handler
	// that then sends the request itself.
	categorySprintfPath = "sdk-path-sprintf"
	// categoryPathFunction is a request path a handler builds through a
	// function value held in a struct field, which the walk does not bind.
	categoryPathFunction = "path-function-value"
	// categoryFollowsRedirect is a route GitLab answers with a redirect to
	// another of its API routes, which the HTTP client follows with the same
	// credential, so GitLab judges the credential on both.
	categoryFollowsRedirect = "follows-redirect"
)

// userCoreBasic is client-go's UserCoreBasic template (users.go), which every
// work item document below selects for a user.
const userCoreBasic = `id
		username
		name
		state
		createdAt
		avatarUrl
		webUrl`

// workItemScalars are the fields every WorkItem fragment of client-go's
// selects outside its features (workitems.go, workItemCEFields and
// workItemTemplate), with UserCoreBasic spread into the author. This and the
// documents below are assembled with fmt.Sprintf rather than +, because a
// package-level initializer is outside every coverage block: a + there is a
// mutant a mutation run can change and no test can be credited with killing.
var workItemScalars = fmt.Sprintf(`id
	iid
	workItemType { name }
	state
	title
	description
	confidential
	author { %s }
	createdAt
	updatedAt
	closedAt
	webUrl`, userCoreBasic)

// workItemFeatures are the selections client-go keeps for each feature of a
// work item, by the name ReturnedFields and its field registry give it.
// Every object one selects is a position GitLab checks a fine-grained token
// at, so each is written out whole rather than cut to what decides today's
// verdict.
var workItemFeatures = map[string]string{
	"assignees":       fmt.Sprintf(`assignees { assignees { nodes { %s } } }`, userCoreBasic),
	"color":           `color { color textColor }`,
	"healthStatus":    `healthStatus { healthStatus }`,
	"hierarchy":       `hierarchy { hasParent parent { iid namespace { fullPath } } hasChildren children { nodes { iid namespace { fullPath } } } }`,
	"iteration":       `iteration { iteration { id } }`,
	"labels":          `labels { labels { nodes { id title color description descriptionHtml textColor } } }`,
	"linkedItems":     `linkedItems { linkedItems { nodes { workItem { iid namespace { fullPath } } linkType } } }`,
	"milestone":       `milestone { milestone { id } }`,
	"startAndDueDate": `startAndDueDate { startDate dueDate }`,
	"status":          `status { status { name } }`,
	"weight":          `weight { weight }`,
}

// workItemDefaultListFeatures are the features of client-go's CE-safe
// default list field set (WorkItemDefaultListFields), which a list handler
// extends with the Enterprise ones it asks for.
var workItemDefaultListFeatures = []string{"assignees", "hierarchy", "labels", "linkedItems", "milestone", "startAndDueDate"}

// workItemFragment is a WorkItem fragment selecting the named features, in
// the order given.
func workItemFragment(features ...string) string {
	selections := make([]string, len(features))
	for i, feature := range features {
		selections[i] = workItemFeatures[feature]
	}
	return workItemScalars + "\n\tfeatures { " + strings.Join(selections, " ") + " }"
}

// The work item fragments the list handlers make client-go send, each rendered
// from the fields the handler asks for: issue.work_item_list the CE-safe
// default and, on an Enterprise instance, all five Enterprise features
// (workitems.listReturnedFields, the widest default, since a caller's
// returned_fields only narrows it), and group.epic_list the default with
// color, healthStatus and weight, an epic carrying neither a status nor an
// iteration (epics.buildWorkItemsListOptions). Get, create and update need no
// declaration: their templates read no value, so graphqldocs renders them
// from client-go's own source and the walk reads what they send.
var (
	workItemListFragment = workItemFragment(append(slices.Clone(workItemDefaultListFeatures),
		"color", "healthStatus", "iteration", "status", "weight")...)
	epicListFragment = workItemFragment(append(slices.Clone(workItemDefaultListFeatures),
		"color", "healthStatus", "weight")...)
)

// The work item list documents client-go assembles from its ListWorkItems
// shell, evaluated with the fragment each handler's fields render.
var (
	listWorkItemsDocument = listDocument(workItemListFragment)
	listEpicsDocument     = listDocument(epicListFragment)
)

// listDocument is client-go's ListWorkItems shell around one fragment.
func listDocument(fragment string) string {
	return `query ListWorkItems($fullPath: ID!) {
	namespace(fullPath: $fullPath) {
		workItems { nodes { ` + fragment + ` } pageInfo { endCursor hasNextPage startCursor hasPreviousPage } }
	}
}`
}

// graphql is a declared GraphQL request.
func graphql(name, document string) derive.Request {
	return derive.Request{Kind: derive.KindGraphQL, Document: document, Name: name}
}

// rest is a declared REST request, in the derivation's spelling.
func rest(method, path string) derive.Request {
	return derive.Request{Kind: derive.KindREST, Method: method, Path: path}
}

// requestDeclarations answer, action by action, what the walk reaches and
// cannot read. Each carries the requests it stands for, since the table needs
// the answer and not an excuse, and each is stale the moment the walk reads
// what it answers: a declaration that answers nothing fails the derivation
// check.
var requestDeclarations = []derive.Declaration{
	{
		Action: "repository.archive", Category: categorySendsNothing,
		Reason: "it returns the archive URL for the caller to fetch; the handler sends no request",
	},
	{
		Action: "access.invite_project", Category: categoryPathFunction,
		Reason:   "runInvitation formats the path through the path field of sendInvitationArgs, which inviteProject fills with the project invitations route",
		Replaces: "raw-path invites.postInvitation", Requests: []derive.Request{rest("POST", "/projects/:/invitations")},
	},
	{
		Action: "access.invite_group", Category: categoryPathFunction,
		Reason:   "runInvitation formats the path through the path field of sendInvitationArgs, which inviteGroup fills with the group invitations route",
		Replaces: "raw-path invites.postInvitation", Requests: []derive.Request{rest("POST", "/groups/:/invitations")},
	},
	{
		Action: "release.get_latest", Category: categoryFollowsRedirect,
		Reason:  "GitLab answers the latest-release permalink with a redirect to the release's own route (lib/api/releases.rb, redirect expose_path(redirect_url)), which the HTTP client follows with the token, and GitLab judges the token again there; a trace of the action on 19.4.1 reaches both routes",
		Follows: "GET /projects/:/releases/permalink/latest", Requests: []derive.Request{rest("GET", "/projects/:/releases/:")},
	},
	{
		Action: "package.download", Category: categorySprintfPath,
		Reason:   "the path is GenericPackages.FormatPackageURL's, which formats the generic package file route and sends nothing; the handler sends the GET itself to stream the body",
		Replaces: "raw-path packages.newDownloadRequest", Requests: []derive.Request{rest("GET", "/projects/:/packages/generic/:/:/:")},
	},
	workItemListDeclaration("group.epic_list", listEpicsDocument),
	workItemListDeclaration("issue.work_item_list", listWorkItemsDocument),
}

// workItemListDeclaration declares the document client-go's ListWorkItems
// assembles for one list handler. TestRequestDeclarations_DeclareWhatTheHandlerSends
// holds each to what the handler makes client-go send.
func workItemListDeclaration(action, document string) derive.Declaration {
	const method = "WorkItems.ListWorkItems"
	return derive.Declaration{
		Action: action, Category: categoryTemplate,
		Reason: "client-go parses the ListWorkItems shell of workitems.go at run time into a template set whose WorkItem fragment " +
			"is rendered from the fields the handler asks for and whose variables are the filters it sets; " +
			"the document is that template evaluated with what the handler hands the method",
		Replaces: categoryTemplate + " " + method, Requests: []derive.Request{graphql(method, document)},
	}
}
