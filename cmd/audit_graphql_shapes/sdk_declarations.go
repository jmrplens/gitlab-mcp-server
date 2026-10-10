// The answers the client-go run is held to: one table for each kind of
// finding it makes, and every entry stale the moment client-go or the schema
// stops producing what it answers.

package main

import (
	"slices"
)

// sdkModule is client-go's root package, the one every service and every
// decoder of its documents is declared in.
const sdkModule = "gitlab.com/gitlab-org/api/client-go/v3"

// sdkService spells the unit a client-go sent finding is filed under for one
// service, "WorkItems" naming WorkItemsService, the way [serviceOf] spells it.
func sdkService(name string) string { return sdkModule + "." + name + "Service" }

// Categories that only the client-go tables use. The sent table reuses the
// categories our own documents are answered with wherever one says the same
// thing of a client-go document; these say what only a dependency's document
// can be.
const (
	// categoryRecordedUpstream is a gap in client-go itself that this server
	// would surface if client-go selected or decoded it, recorded as a row of
	// docs/development/upstream-bugs.md. The reason names the row, and the
	// change to client-go that closes it when one is open; the declaration
	// goes stale on the bump that brings that change.
	categoryRecordedUpstream = "recorded-upstream"
	// categoryDecodedNeverRead is a field a client-go decoder declares, which
	// the document never selects and which client-go never reads either, so
	// no caller can see the value it is always left at.
	categoryDecodedNeverRead = "decoded-and-never-read"
	// categoryResolvedOncePerRequest is a field GitLab resolves for one object
	// per request and refuses on a second, which a list therefore leaves out.
	categoryResolvedOncePerRequest = "resolved-once-per-request"
	// categoryBuiltPerCall is a document client-go assembles from what a
	// caller hands the method, so no text exists to judge before a request is
	// made.
	categoryBuiltPerCall = "built-per-call"
)

// sdkShapeDeclaration answers one disagreement between a client-go document
// and the struct client-go decodes it into, at one response path of one method.
type sdkShapeDeclaration struct {
	// Method names the function holding the send the way [shortName] does.
	Method string
	// Path is the response path the disagreement is reported at.
	Path     string
	Category string
	Reason   string
}

// key names the declaration in a report.
func (d sdkShapeDeclaration) key() string { return d.Method + " " + d.Path }

// sdkSendDeclaration answers a client-go send this audit cannot pair with its
// decoder, which leaves both unjudged.
type sdkSendDeclaration struct {
	// Method names the function holding the send the way [shortName] does.
	Method   string
	Category string
	Reason   string
}

// key names the declaration in a report.
func (d sdkSendDeclaration) key() string { return d.Method }

// sdkTables are the three tables a client-go run is held to.
type sdkTables struct {
	shapes []sdkShapeDeclaration
	sends  []sdkSendDeclaration
	sent   []sentDeclaration
}

// declaredSDK is the committed answer to every finding the client-go run
// makes against the client-go go.mod requires.
var declaredSDK = sdkTables{
	shapes: declaredSDKShapes(),
	sends:  declaredSDKSends(),
	sent:   declaredSDKSent(),
}

// classifyShapes attaches the declaration answering each disagreement and
// names the declarations that answered none.
func classifyShapes(declarations []sdkShapeDeclaration, found []sdkShapeFinding) (classified []sdkShapeFinding, unused []string) {
	return classifyAgainst(declarations, found, func(declaration sdkShapeDeclaration, finding sdkShapeFinding) (sdkShapeFinding, bool) {
		if declaration.Method != finding.Method || declaration.Path != finding.Path {
			return finding, false
		}
		finding.Category, finding.Reason = declaration.Category, declaration.Reason
		return finding, true
	})
}

// classifySends attaches the declaration answering each unjudged send and
// names the declarations that answered none.
func classifySends(declarations []sdkSendDeclaration, found []sdkUnjudgedSend) (classified []sdkUnjudgedSend, unused []string) {
	return classifyAgainst(declarations, found, func(declaration sdkSendDeclaration, send sdkUnjudgedSend) (sdkUnjudgedSend, bool) {
		if declaration.Method != send.Method {
			return send, false
		}
		send.Category, send.Reason = declaration.Category, declaration.Reason
		return send, true
	})
}

// classifyAgainst holds findings to a table: each finding is handed back
// annotated by the first declaration that answers it, or as it was, and every
// declaration that answered none is named, sorted, and nil when there is none.
func classifyAgainst[D interface{ key() string }, F any](declarations []D, found []F, answer func(D, F) (F, bool)) (classified []F, unused []string) {
	used := map[string]bool{}
	for _, finding := range found {
		for _, declaration := range declarations {
			if answered, ok := answer(declaration, finding); ok {
				finding = answered
				used[declaration.key()] = true
				break
			}
		}
		classified = append(classified, finding)
	}
	for _, declaration := range declarations {
		if !used[declaration.key()] {
			unused = append(unused, declaration.key())
		}
	}
	slices.Sort(unused)
	return classified, unused
}

// workItemShapes answers the disagreements one work item document has with
// workItemGQL, rooted where that document reaches the work item. The get,
// create and update documents share one fragment, so each has the same nine.
//
// The tables of this file are built by functions rather than declared as
// package variables, as the sibling tables are, because the mutation tool
// cannot see a package-level initializer and reads every string joined there
// as never covered.
func workItemShapes(method, root string) []sdkShapeDeclaration {
	// unreadPageInfoReason is why a nested connection's page information is
	// decoded and never read: client-go decodes every connection through one
	// generic struct, and only a top-level list hands its page on.
	const unreadPageInfoReason = "connectionGQL (graphql.go) decodes every connection into Nodes and PageInfo, and a " +
		"connection nested inside an object selects its nodes alone. The unwrap of that object reads Nodes and " +
		"nothing else, and the only PageInfo client-go hands a caller, as Response.PageInfo, is the top-level " +
		"list's, so the zero value is never seen."
	// unreadLabelReason is why four label fields are decoded and never read
	// under a work item.
	const unreadLabelReason = "labelGQL (labels.go) declares archived, createdAt, lockOnMerge and updatedAt, and the " +
		"work item fragment selects a label's id, title, color, description, descriptionHtml and textColor. " +
		"labelGQL.unwrap copies exactly those six into LabelDetails, so the four are decoded empty and no caller " +
		"reads them."

	declarations := make([]sdkShapeDeclaration, 0, 9)
	for _, path := range []string{
		"features.assignees.assignees.pageInfo",
		"features.hierarchy.children.pageInfo",
		"features.labels.labels.pageInfo",
		"features.linkedItems.linkedItems.pageInfo",
	} {
		declarations = append(declarations, sdkShapeDeclaration{
			Method: method, Path: root + "." + path, Category: categoryDecodedNeverRead, Reason: unreadPageInfoReason,
		})
	}
	for _, field := range []string{"archived", "createdAt", "lockOnMerge", "updatedAt"} {
		declarations = append(declarations, sdkShapeDeclaration{
			Method: method, Path: root + ".features.labels.labels.nodes[]." + field, Category: categoryDecodedNeverRead, Reason: unreadLabelReason,
		})
	}
	return append(declarations, sdkShapeDeclaration{
		Method: method, Path: root + ".features.startAndDueDate.isFixed", Category: categoryDecodedNeverRead,
		Reason: "workItemWidgetStartAndDueDateGQL declares isFixed and the fragment selects startDate and dueDate alone. " +
			"Its unwrap returns the two dates and nothing else, so isFixed is decoded false and no caller reads it. " +
			"That the field is worth selecting is a separate gap, recorded in upstream-bugs row 107.",
	})
}

// declaredSDKShapes answers the disagreements between client-go's documents
// and its decoders at the sends this server reaches.
func declaredSDKShapes() []sdkShapeDeclaration {
	return slices.Concat([]sdkShapeDeclaration{
		{
			Method: "AchievementsService.ListAchievementRecipients", Path: "data.namespace.achievements.pageInfo",
			Category: categoryDecodedNeverRead,
			Reason: "The outer achievements connection is filtered to the one achievement asked about and decoded through " +
				"connectionGQL, whose PageInfo the document does not select. The method pages the recipients and hands on " +
				"achievement.UserAchievements.PageInfo, the inner connection's, so the outer one is never read.",
		},
		{
			Method: "AchievementsService.ListAchievementUniqueUsers", Path: "data.namespace.achievements.pageInfo",
			Category: categoryDecodedNeverRead,
			Reason: "The outer achievements connection is filtered to the one achievement asked about and decoded through " +
				"connectionGQL, whose PageInfo the document does not select. The method pages the users and hands on " +
				"achievement.UniqueUsers.PageInfo, the inner connection's, so the outer one is never read.",
		},
		{
			Method: "WorkItemSavedViewsService.ListWorkItemSavedViews", Path: "data.namespace.savedViews.nodes[].filters",
			Category: categoryResolvedOncePerRequest,
			Reason: "GitLab resolves a saved view's filters for one view per request (FieldCallCount, limit 1, in " +
				"app/graphql/types/work_items/saved_views/saved_view_type.rb), so the list leaves them out and client-go " +
				"documents Filters as always empty on a listed view, to be read with GetWorkItemSavedView. Our list " +
				"publishes no filters and its card tells the caller to read them by ID for the same reason.",
		},
	}, workItemShapes("WorkItemsService.GetWorkItem", "data.namespace.workItem"),
		workItemShapes("WorkItemsService.CreateWorkItem", "data.workItemCreate.workItem"),
		workItemShapes("WorkItemsService.UpdateWorkItem", "data.workItemUpdate.workItem"))
}

// declaredSDKSends answers the client-go sends this server reaches whose
// document cannot be read before a request is made.
func declaredSDKSends() []sdkSendDeclaration {
	return []sdkSendDeclaration{
		{
			Method: "WorkItemsService.ListWorkItems", Category: categoryBuiltPerCall,
			Reason: "buildListWorkItemsQuery parses the ListWorkItems shell of workitems.go at run time into a template " +
				"set whose fragment is rendered from the fields the caller asks for (ReturnedFields) and whose variables " +
				"are the filters it sets, so no document exists before a call. Every field the listing can select is one " +
				"the get fragment selects, which this run judges at GetWorkItem; R-PATH names the shell, and " +
				"cmd/gen_action_grants declares the document each of our two list handlers makes it send.",
		},
	}
}
