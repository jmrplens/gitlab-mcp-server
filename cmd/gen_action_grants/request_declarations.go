package main

import (
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
	// text/template at run time.
	categoryTemplate = "sdk-graphql-template"
	// categoryFormat is a client-go GraphQL document assembled with
	// fmt.Sprintf at run time.
	categoryFormat = "sdk-graphql-format"
	// categorySprintfPath is a request path client-go formats for a handler
	// that then sends the request itself.
	categorySprintfPath = "sdk-path-sprintf"
	// categoryPathFunction is a request path a handler builds through a
	// function value held in a struct field, which the walk does not bind.
	categoryPathFunction = "path-function-value"
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
// workItemTemplate), with UserCoreBasic spread into the author.
const workItemScalars = `id
	iid
	workItemType { name }
	state
	title
	description
	confidential
	author { ` + userCoreBasic + ` }
	createdAt
	updatedAt
	closedAt
	webUrl`

// workItemFeatures are the selections client-go keeps for each feature of a
// work item, by the name ReturnedFields and its field registry give it.
// Every object one selects is a position GitLab checks a fine-grained token
// at, so each is written out whole rather than cut to what decides today's
// verdict.
var workItemFeatures = map[string]string{
	"assignees":       `assignees { assignees { nodes { ` + userCoreBasic + ` } } }`,
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

// The work item fragments the handlers make client-go send. Get, create and
// update use client-go's static WorkItem template, which selects every
// feature; a list renders its fragment from the fields the handler asks for:
// issue.work_item_list the CE-safe default and, on an Enterprise instance,
// all five Enterprise features (workitems.listReturnedFields, the widest
// default, since a caller's returned_fields only narrows it), and
// group.epic_list the default with color, healthStatus and weight, an epic
// carrying neither a status nor an iteration (epics.buildWorkItemsListOptions).
var (
	staticWorkItemFragment = workItemFragment("assignees", "color", "healthStatus", "hierarchy", "iteration",
		"labels", "linkedItems", "milestone", "startAndDueDate", "status", "weight")
	workItemListFragment = workItemFragment(append(slices.Clone(workItemDefaultListFeatures),
		"color", "healthStatus", "iteration", "status", "weight")...)
	epicListFragment = workItemFragment(append(slices.Clone(workItemDefaultListFeatures),
		"color", "healthStatus", "weight")...)
)

// The work item documents client-go assembles from its templates, evaluated
// with what the method passes the template: nothing for get, create and
// update, and for a list the fragment its fields render.
var (
	getWorkItemDocument = `query GetWorkItem($fullPath: ID!, $iid: String!) {
	namespace(fullPath: $fullPath) { workItem(iid: $iid) { ` + staticWorkItemFragment + ` } }
}`
	listWorkItemsDocument  = listDocument(workItemListFragment)
	listEpicsDocument      = listDocument(epicListFragment)
	createWorkItemDocument = `mutation CreateWorkItem($input: WorkItemCreateInput!) {
	workItemCreate(input: $input) { workItem { ` + staticWorkItemFragment + ` } errors }
}`
	updateWorkItemDocument = `mutation UpdateWorkItem($input: WorkItemUpdateInput!) {
	workItemUpdate(input: $input) { workItem { ` + staticWorkItemFragment + ` } errors }
}`
)

// listDocument is client-go's ListWorkItems shell around one fragment.
func listDocument(fragment string) string {
	return `query ListWorkItems($fullPath: ID!) {
	namespace(fullPath: $fullPath) {
		workItems { nodes { ` + fragment + ` } pageInfo { endCursor hasNextPage startCursor hasPreviousPage } }
	}
}`
}

// The Terraform state documents client-go formats with the project path and
// the state name quoted in.
const (
	terraformStateListDocument = `query {
	project(fullPath: "group/project") {
		terraformStates {
			nodes { name createdAt deletedAt latestVersion { createdAt updatedAt downloadPath serial } updatedAt lockedAt }
		}
	}
}`
	terraformStateGetDocument = `query {
	project(fullPath: "group/project") {
		terraformState(name: "state") {
			name createdAt deletedAt latestVersion { createdAt updatedAt downloadPath serial } updatedAt lockedAt
		}
	}
}`
)

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
		Action: "package.download", Category: categorySprintfPath,
		Reason:   "the path is GenericPackages.FormatPackageURL's, which formats the generic package file route and sends nothing; the handler sends the GET itself to stream the body",
		Replaces: "raw-path packages.newDownloadRequest", Requests: []derive.Request{rest("GET", "/projects/:/packages/generic/:/:/:")},
	},
	workItemDeclaration("group.epic_create", "WorkItems.CreateWorkItem", "CreateWorkItem", createWorkItemDocument),
	workItemDeclaration("group.epic_get", "WorkItems.GetWorkItem", "GetWorkItem", getWorkItemDocument),
	workItemDeclaration("group.epic_list", "WorkItems.ListWorkItems", "ListWorkItems", listEpicsDocument),
	workItemDeclaration("group.epic_update", "WorkItems.UpdateWorkItem", "UpdateWorkItem", updateWorkItemDocument),
	workItemDeclaration("issue.work_item_create", "WorkItems.CreateWorkItem", "CreateWorkItem", createWorkItemDocument),
	workItemDeclaration("issue.work_item_get", "WorkItems.GetWorkItem", "GetWorkItem", getWorkItemDocument),
	workItemDeclaration("issue.work_item_list", "WorkItems.ListWorkItems", "ListWorkItems", listWorkItemsDocument),
	workItemDeclaration("issue.work_item_update", "WorkItems.GetWorkItem", "GetWorkItem", getWorkItemDocument),
	workItemDeclaration("issue.work_item_update", "WorkItems.UpdateWorkItem", "UpdateWorkItem", updateWorkItemDocument),
	{
		Action: "admin.terraform_state_get", Category: categoryFormat,
		Reason:   "client-go formats the project path and the state name into the query with %q",
		Replaces: categoryFormat + " TerraformStates.Get", Requests: []derive.Request{graphql("TerraformStates.Get", terraformStateGetDocument)},
	},
	{
		Action: "admin.terraform_state_list", Category: categoryFormat,
		Reason:   "client-go formats the project path into the query with %q",
		Replaces: categoryFormat + " TerraformStates.List", Requests: []derive.Request{graphql("TerraformStates.List", terraformStateListDocument)},
	},
}

// workItemDeclaration declares the document a client-go work item method
// assembles from its template. TestRequestDeclarations_DeclareWhatTheHandlerSends
// holds each to what the handler makes client-go send.
func workItemDeclaration(action, method, name, document string) derive.Declaration {
	return derive.Declaration{
		Action: action, Category: categoryTemplate,
		Reason:   "client-go executes the " + name + " text/template of workitems.go at run time; the document is that template evaluated with what the handler hands the method",
		Replaces: categoryTemplate + " " + method, Requests: []derive.Request{graphql(method, document)},
	}
}
