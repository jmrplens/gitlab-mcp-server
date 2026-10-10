// The answers to what the schema offers at an object a client-go decoder
// reads and no document of its service selects: the one-time triage of
// issue 1022, against client-go v3.15.0 and the 19.5.0-pre pin.

package main

import "slices"

// The client-go services whose findings these tables answer.
var (
	achievementsService = sdkService("Achievements")
	scanProfilesService = sdkService("SecurityScanProfiles")
	terraformService    = sdkService("TerraformStates")
	savedViewsService   = sdkService("WorkItemSavedViews")
	workItemsService    = sdkService("WorkItems")
)

// sdkReference is why an object a client-go document names by a field or
// two is not answered with the rest of what GitLab offers on it.
func sdkReference(object, how, surface string) string {
	return "The object is a reference: client-go selects " + how + " to say which " + object + " the answer is " +
		"about, and its own fields are " + surface + ". Answering them under this service would be that surface a " +
		"second time, through a document nobody maintains against it."
}

// referenceObject declares the whole of an object a client-go document names
// only to say which one the answer is about, for the reason sdkReference gives.
func referenceObject(service, schemaType, object, how, surface string) sentDeclaration {
	return sentDeclaration{
		Package: service, SchemaType: schemaType, Field: declaredSegment, Category: categoryNotThisResponse,
		Reason: sdkReference(object, how, surface),
	}
}

// recorded is a field this server would publish if client-go selected it,
// recorded as a row of docs/development/upstream-bugs.md.
func recorded(row, what string) string {
	return "Upstream-bugs row " + row + " records it: " + what
}

// recordedField declares one field recorded upstream.
func recordedField(service, schemaType, field, row, what string) sentDeclaration {
	return sentDeclaration{Package: service, SchemaType: schemaType, Field: field, Category: categoryRecordedUpstream, Reason: recorded(row, what)}
}

// experimentField declares one field GitLab marks Status: Experiment.
func experimentField(service, schemaType, field, milestone, source string) sentDeclaration {
	return sentDeclaration{
		Package: service, SchemaType: schemaType, Field: field, Category: categoryExperiment,
		Reason: "GitLab marks the field an experiment (experiment: { milestone: '" + milestone + "' } in " + source + "), " +
			"which it may change or remove without notice, and GitLab refuses a whole document that names a field it " +
			"no longer has, so selecting it would stake every call of the service on the experiment.",
	}
}

// deprecatedField declares one field GitLab has deprecated.
func deprecatedField(service, schemaType, field, source, replacement string) sentDeclaration {
	return sentDeclaration{
		Package: service, SchemaType: schemaType, Field: field, Category: categoryDeprecated,
		Reason: "GitLab deprecates the field (deprecated: in " + source + ")" + replacement + ", so selecting it now " +
			"would select something GitLab is removing.",
	}
}

// workItemThirdPartyFeature is why a work item feature belongs to a part of
// GitLab this server serves no tool for.
func workItemThirdPartyFeature(feature, what string) sentDeclaration {
	return sentDeclaration{
		Package: workItemsService, SchemaType: "WorkItemFeatures", Field: feature, Category: categoryOutsideSurface,
		Reason: "The widget is " + what + ", a part of GitLab this server serves no tool for, so a work item tool " +
			"answering with it would start that surface inside another domain.",
	}
}

// declaredSDKSent answers the fields the schema offers at an object a client-go
// decoder reads that no document of its service selects.
//
// Where a field is answered by name and its object also by the star, the named
// answer comes first: classification takes the first declaration that covers a
// finding, and the named one is the one that goes stale when client-go starts
// selecting the field.
func declaredSDKSent() []sentDeclaration {
	return slices.Concat(
		achievementsSDKSent(),
		scanProfilesSDKSent(),
		terraformSDKSent(),
		savedViewsSDKSent(),
		workItemsSDKSent(),
		workItemWidgetsSDKSent(),
	)
}

// achievementsSDKSent answers the achievements service, whose namespace and
// users client-go names by id alone until commit 5 of
// gitlab-org/api/client-go!3063 (row 27) adds what identifies them.
func achievementsSDKSent() []sentDeclaration {
	declarations := []sentDeclaration{
		recordedField(achievementsService, "UserAchievement", "awardMessageHtml", "27",
			"the rendered award message, which commit 5 of gitlab-org/api/client-go!3063 selects; the declaration goes "+
				"stale on the client-go bump that brings it, and achievements then publishes it."),
	}
	for _, field := range []string{"avatarUrl", "fullName", "fullPath", "name", "path", "webUrl"} {
		declarations = append(declarations, recordedField(achievementsService, "Namespace", field, "27",
			"commit 5 of gitlab-org/api/client-go!3063 selects the achievement's namespace by its name, paths, "+
				"avatar and page beside its id; the declaration goes stale on the client-go bump that brings it, and "+
				"achievements then publishes the namespace beside namespace_id."))
	}
	return append(declarations,
		referenceObject(achievementsService, "Namespace", "namespace",
			"its id (and, from row 27's commit, the identity fields named above)",
			"the group and project surface, which gitlab_group and gitlab_project publish"),
		referenceObject(achievementsService, "UserCore", "user",
			"the shared basic user fragment (id, username, name, state, avatar, page and "+
				"creation time; the award and revoke documents select the id until row 27's commit)",
			"the users surface, which gitlab_user publishes"),
	)
}

// scanProfilesSDKSent answers the scan profile service, whose status
// document selects the profile's id, name and type and nothing else.
func scanProfilesSDKSent() []sentDeclaration {
	const inDefault = "the status document selects the status and the profile's id, name and type. GitLab has served " +
		"the field wherever it serves scanProfileStatuses (19.0), so it could join that document, and whether it goes " +
		"to gitlab-org/api/client-go!3063 is not yet decided."
	const optIn = "projectCount is new in GitLab 19.5 and triggerSettings names union members 19.4 added, and an " +
		"earlier GitLab refuses a document naming either, so a commit prepared for gitlab-org/api/client-go!3063 makes " +
		"both selectable through ListProjectScanProfileStatusesWithOptions and leaves the default document without " +
		"them; the declaration is reviewed on the client-go bump that brings the commit."
	declarations := make([]sentDeclaration, 0, 12)
	for _, field := range []string{"buildId", "consecutiveFailureCount", "consecutiveSuccessCount", "lastScanAt"} {
		declarations = append(declarations, recordedField(scanProfilesService, "ScanProfileProjectStatus", field, "105", inDefault))
	}
	for _, field := range []string{"createdAt", "description", "gitlabRecommended", "updatedAt"} {
		declarations = append(declarations, recordedField(scanProfilesService, "ScanProfileType", field, "105", inDefault))
	}
	for _, field := range []string{"projectCount", "triggerSettings"} {
		declarations = append(declarations, recordedField(scanProfilesService, "ScanProfileType", field, "105", optIn))
	}
	return append(declarations,
		sentDeclaration{
			Package: scanProfilesService, SchemaType: "ScanProfileType", Field: "configuration", Category: categoryRestated,
			Reason: "A JSON object GitLab marks an experiment whose schema 'has not been decided yet' " +
				"(ee/app/graphql/types/security/scan_profile_type.rb): it is the effective configuration of the " +
				"sbom_ingested trigger alone, which triggerSettings carries typed, per trigger, as the configuration of " +
				"that trigger's setting.",
		},
		sentDeclaration{
			Package: scanProfilesService, SchemaType: "ScanProfileType", Field: "triggers", Category: categoryRestated,
			Reason: "The trigger types of the profile, which ScanProfileType resolves from the same scan_profile_triggers " +
				"triggerSettings reads (ee/app/graphql/types/security/scan_profile_type.rb), so each is the triggerType " +
				"of a trigger setting.",
		},
	)
}

// terraformSDKSent answers the Terraform state service.
func terraformSDKSent() []sentDeclaration {
	const inDefault = "GitLab has served it on every release that serves Terraform states over GraphQL, so it could " +
		"join the List and Get documents, and whether it goes to gitlab-org/api/client-go!3063 is not yet decided."
	return []sentDeclaration{
		recordedField(terraformService, "TerraformState", "versions", "104",
			"a state's versions are new in GitLab 19.5, and a commit prepared for gitlab-org/api/client-go!3063 adds "+
				"ListVersions to page through them rather than selecting them in List and Get, which an earlier GitLab "+
				"would refuse."),
		recordedField(terraformService, "TerraformState", "permanentDeletionAt", "104",
			"the field is new in GitLab 19.5 and marked an experiment, so a commit prepared for "+
				"gitlab-org/api/client-go!3063 makes it selectable through ListWithOptions and GetWithOptions and leaves "+
				"List and Get without it, which an earlier GitLab would refuse."),
		recordedField(terraformService, "TerraformState", "lockedByUser", "104", "who holds the state's lock: "+inDefault),
		experimentField(terraformService, "TerraformState", "protectionRuleExists", "19.0", "app/graphql/types/terraform/state_type.rb"),
		{
			Package: terraformService, SchemaType: "TerraformState", Field: "id", Category: categoryUnusedIdentifier,
			Reason: "The state's GraphQL ID. Every client-go Terraform state method and every action of this server " +
				"names a state by its project path and name, and nothing takes the ID.",
		},
		recordedField(terraformService, "TerraformStateVersion", "createdByUser", "104", "who wrote the latest version: "+inDefault),
		recordedField(terraformService, "TerraformStateVersion", "job", "104", "the CI job that wrote the latest version, by its id, name and page: "+inDefault),
		{
			Package: terraformService, SchemaType: "TerraformStateVersion", Field: "id", Category: categoryUnusedIdentifier,
			Reason: "The version's GraphQL ID. A version is fetched by its serial (the download path ends in it), and " +
				"no client-go method and no action of this server takes the ID.",
		},
	}
}

// savedViewsSDKSent answers the saved view service.
func savedViewsSDKSent() []sentDeclaration {
	const source = "app/graphql/types/work_items/saved_views/saved_view_type.rb"
	what := "the saved view documents leave it out, and no client-go change is open for it."
	return []sentDeclaration{
		recordedField(savedViewsService, "WorkItemSavedViewType", "author", "108", "who created the view: "+what),
		recordedField(savedViewsService, "WorkItemSavedViewType", "updatedAt", "108", "when the view last changed: "+what),
		recordedField(savedViewsService, "WorkItemSavedViewType", "shareUrl", "108", "the URL that subscribes a user to the view: "+what),
		recordedField(savedViewsService, "WorkItemSavedViewType", "filterWarnings", "108",
			"the warnings about filter values GitLab could not apply, resolved once per request like the filters "+
				"GetWorkItemSavedView selects: "+what),
		experimentField(savedViewsService, "WorkItemSavedViewType", "lastUpdatedBy", "18.10", source),
		deprecatedField(savedViewsService, "WorkItemSavedViewType", "private", source+" in 18.10",
			", replaced by isPrivate, which every saved view document selects"),
		{
			Package: savedViewsService, SchemaType: "WorkItemSavedViewType", Field: "namespaceId", Category: categoryRestated,
			Reason: "The namespace the view belongs to, which every saved view method takes as the namespace path the " +
				"caller names, so the answer restates it.",
		},
		{
			Package: savedViewsService, SchemaType: "WorkItemSavedViewType", Field: "userPermissions", Category: categoryViewer,
			Reason: "What the token's own user may do with the view, which says nothing about the view, and which this " +
				"server answers by attempting the action and reporting GitLab's refusal.",
		},
		{
			Package: savedViewsService, SchemaType: "WorkItemSavedViewType", Field: "workItems", Category: categorySeparateAction,
			Reason: "The work items the view matches: a connection a caller pages through, resolved once per request and " +
				"marked an experiment in " + source + ", which listing work items with the view's filters answers.",
		},
		{
			Package: savedViewsService, SchemaType: "WorkItemSavedViewDeletePayload", Field: "savedView", Category: categoryRestated,
			Reason: "The view the mutation deleted, which the caller named by its ID to delete it; the payload hands back " +
				"a value the caller already holds.",
		},
	}
}

// workItemsSDKSent answers the work item service's own objects and the
// objects its documents name.
func workItemsSDKSent() []sentDeclaration {
	const row107 = "the work item documents leave it out, and no client-go change is open for it."
	const source = "app/graphql/types/work_item_type.rb"
	declarations := []sentDeclaration{
		recordedField(workItemsService, "WorkItemFeatures", "decisionLog", "106",
			"the decision log widget, added in GitLab 19.4 and Enterprise only; a commit prepared for "+
				"gitlab-org/api/client-go!3063 adds it as a field ReturnedFields selects, never by default, since an "+
				"earlier GitLab and Community Edition refuse it, and GitLab resolves its decisions for one work item per request."),
		recordedField(workItemsService, "WorkItemFeatures", "development", "106",
			"the development widget, whose closingMergeRequestsCount GitLab added in 19.4; a commit prepared for "+
				"gitlab-org/api/client-go!3063 adds it as a field ReturnedFields selects, never by default, since an "+
				"earlier GitLab refuses it."),
	}
	for _, field := range []string{"duplicatedToWorkItemUrl", "movedToWorkItemUrl", "promotedToEpicUrl", "hidden", "imported", "reference", "userDiscussionsCount"} {
		declarations = append(declarations, recordedField(workItemsService, "WorkItem", field, "107", row107))
	}
	for _, feature := range []string{"customFields", "description", "progress", "timeTracking"} {
		declarations = append(declarations, recordedField(workItemsService, "WorkItemFeatures", feature, "107", row107))
	}
	declarations = append(declarations,
		experimentField(workItemsService, "WorkItem", "archived", "16.5", source),
		experimentField(workItemsService, "WorkItem", "externalAuthor", "18.8", source),
		experimentField(workItemsService, "WorkItem", "project", "15.3", source),
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "showPlanUpgradePromotion", Category: categoryAffordance,
			Reason: "Whether the web UI shows a plan upgrade promotion on the work item, an experiment " +
				"(" + source + ") that says nothing about the work item.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "availableQuickActions", Category: categoryViewer,
			Reason: "The quick actions available to the current user on the work item (" + source + "), " +
				"which is what the token's user may type in a comment box rather than anything about the work item.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "createNoteEmail", Category: categoryViewer,
			Reason: "A user specific email address for replying to the work item by email (" + source + "), " +
				"minted for the token's user alone.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "userPermissions", Category: categoryViewer,
			Reason: "What the token's own user may do with the work item, which this server answers by attempting the " +
				"action and reporting GitLab's refusal.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "commentTemplatesPaths", Category: categoryWebRendering,
			Reason: "The paths of the comment templates the web UI's comment box offers, a value GitLab computes for its " +
				"own interface.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "descriptionHtml", Category: categoryWebRendering,
			Reason: "GitLab's HTML rendering of the description, which the documents select as Markdown.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "titleHtml", Category: categoryWebRendering,
			Reason: "GitLab's HTML rendering of the title (a markdown_field in " + source + "), " +
				"which the documents select as text.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "lockVersion", Category: categoryWebRendering,
			Reason: "The counter GitLab increments on every update, which an edit form sends back to detect a concurrent " +
				"change; no client-go method sends it.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "duoWorkflowLinks", Category: categoryOutsideSurface,
			Reason: "The Duo agent workflows linked to the work item, a part of GitLab this server serves no tool for.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "name", Category: categoryRestated,
			Reason: "The name or title the to-do interface declares for every todoable object " +
				"(app/graphql/types/todoable_interface.rb), which for a work item is its title, selected as title.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "webPath", Category: categoryRestated,
			Reason: "The path part of the work item's page, whose whole URL the documents select as webUrl.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItem", Field: "widgets", Category: categoryRestated,
			Reason: "The list form of the widgets, which the documents read by name under features, one field per widget.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItemDeletePayload", Field: declaredSegment, Category: categoryRestated,
			Reason: "The namespace or project the deleted work item belonged to, which the caller named to delete it.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "LinkedWorkItemType", Field: "linkId", Category: categoryUnusedIdentifier,
			Reason: "The ID of the link between two work items. client-go has no method that removes a link by its ID, " +
				"and no action of this server takes one.",
		},
		sentDeclaration{
			Package: workItemsService, SchemaType: "WorkItemType", Field: declaredSegment, Category: categoryAffordance,
			Reason: "How the web UI configures a work item type: which views it appears in, its icon, whether a project " +
				"selector or the issue view is shown, which conversions and moves it supports and which widgets its " +
				"forms draw (app/graphql/types/work_items/type_type.rb). A work item answers with its type's name, " +
				"which is what a caller reads and sets.",
		},
		referenceObject(workItemsService, "Iteration", "iteration", "its id",
			"the iterations surface, which gitlab_issue's iteration actions publish"),
		referenceObject(workItemsService, "Milestone", "milestone", "its id",
			"the milestones surface, which gitlab_project and gitlab_group publish"),
		referenceObject(workItemsService, "Label", "label", "its id, title, colors and description",
			"the labels surface, which gitlab_project and gitlab_group publish"),
		referenceObject(workItemsService, "WorkItemStatus", "status", "its name",
			"the work item status and lifecycle configuration of the namespace"),
		referenceObject(workItemsService, "Namespace", "namespace", "its full path, which is how every work item method names one",
			"the group and project surface, which gitlab_group and gitlab_project publish"),
		referenceObject(workItemsService, "UserCore", "user", "the shared basic user fragment (id, username, name, state, avatar, page and creation time)",
			"the users surface, which gitlab_user publishes"),
	)
	for _, field := range []string{"linkCreatedAt", "linkUpdatedAt", "workItemState"} {
		declarations = append(declarations, recordedField(workItemsService, "LinkedWorkItemType", field, "107", row107))
	}
	return declarations
}

// workItemWidgetsSDKSent answers the work item features and the fields of
// each widget client-go reads.
func workItemWidgetsSDKSent() []sentDeclaration {
	const row107 = "the work item documents leave it out, and no client-go change is open for it."
	// The widget sources the table cites more than once.
	const (
		assigneesSource = "app/graphql/types/work_items/widgets/assignees_type.rb"
		hierarchySource = "app/graphql/types/work_items/widgets/hierarchy_type.rb"
		weightSource    = "ee/app/graphql/types/work_items/widgets/weight_type.rb"
	)
	declarations := []sentDeclaration{
		workItemThirdPartyFeature("agentPlan", "a Duo agent's plan"),
		workItemThirdPartyFeature("aiSession", "a Duo agent session"),
		workItemThirdPartyFeature("crmContacts", "the customer relations contacts of the work item"),
		workItemThirdPartyFeature("designs", "design management"),
		workItemThirdPartyFeature("emailParticipants", "the Service Desk email participants"),
		workItemThirdPartyFeature("linkedResources", "the incident linked resources (Zoom and similar links)"),
		workItemThirdPartyFeature("requirementLegacy", "the legacy requirements identifier"),
		workItemThirdPartyFeature("testReports", "the requirement test reports"),
		workItemThirdPartyFeature("verificationStatus", "the requirement verification status"),
		{
			Package: workItemsService, SchemaType: "WorkItemFeatures", Field: "awardEmoji", Category: categorySeparateAction,
			Reason: "The emoji reactions, a list a caller pages through and changes with mutations of its own, which the " +
				"award emoji tools answer.",
		},
		{
			Package: workItemsService, SchemaType: "WorkItemFeatures", Field: "notes", Category: categorySeparateAction,
			Reason: "The discussions and notes, a connection a caller pages through and writes with mutations of its own, " +
				"which the notes and discussion tools answer.",
		},
		{
			Package: workItemsService, SchemaType: "WorkItemFeatures", Field: "participants", Category: categorySeparateAction,
			Reason: "The participants, a connection of users a caller pages through, which the participants actions answer.",
		},
		{
			Package: workItemsService, SchemaType: "WorkItemFeatures", Field: "vulnerabilities", Category: categorySeparateAction,
			Reason: "The vulnerabilities related to the work item, a connection the vulnerability tools answer.",
		},
		{
			Package: workItemsService, SchemaType: "WorkItemFeatures", Field: "errorTracking", Category: categorySeparateAction,
			Reason: "The Sentry error the work item tracks, an identifier into error tracking, whose errors the error " +
				"tracking tools read by that identifier.",
		},
		{
			Package: workItemsService, SchemaType: "WorkItemFeatures", Field: "currentUserTodos", Category: categoryViewer,
			Reason: "The token's own user's to-do items for the work item, which the to-do tools answer.",
		},
		{
			Package: workItemsService, SchemaType: "WorkItemFeatures", Field: "notifications", Category: categoryViewer,
			Reason: "Whether the token's own user is subscribed to the work item's notifications, a property of the " +
				"caller rather than of the work item.",
		},
		deprecatedField(workItemsService, "WorkItemWidgetAssignees", "allowsMultipleAssignees", assigneesSource, ""),
		deprecatedField(workItemsService, "WorkItemWidgetAssignees", "canInviteMembers", assigneesSource, ""),
		deprecatedField(workItemsService, "WorkItemWidgetLabels", "allowsScopedLabels", "app/graphql/types/work_items/widgets/labels_type.rb", ""),
		deprecatedField(workItemsService, "WorkItemWidgetWeight", "widgetDefinition", weightSource, ""),
		experimentField(workItemsService, "WorkItemWidgetHealthStatus", "rolledUpHealthStatus", "17.3", "ee/app/graphql/types/work_items/widgets/health_status_type.rb"),
		experimentField(workItemsService, "WorkItemWidgetHierarchy", "rolledUpCountsByType", "17.3", hierarchySource),
		experimentField(workItemsService, "WorkItemWidgetHierarchy", "depthLimitReachedByType", "17.4", hierarchySource),
		experimentField(workItemsService, "WorkItemWidgetWeight", "rolledUpWeight", "17.2", weightSource),
		experimentField(workItemsService, "WorkItemWidgetWeight", "rolledUpCompletedWeight", "17.3", weightSource),
		{
			Package: workItemsService, SchemaType: "WorkItemWidgetHierarchy", Field: "ancestors", Category: categorySeparateAction,
			Reason: "Every ancestor of the work item, a connection a caller pages through; the documents select the parent.",
		},
		{
			Package: workItemsService, SchemaType: "WorkItemWidgetStartAndDueDate", Field: "rollUp", Category: categoryAffordance,
			Reason: "Whether the work item can use rolled up dates (can_rollup? in app/graphql/types/work_items/widgets/" +
				"start_and_due_date_type.rb), which decides what the date picker offers.",
		},
	}
	// widgetTypeReason is why a widget's own type is not selected.
	const widgetTypeReason = "The widget's type is the WorkItemWidgetType value naming which widget it is, and " +
		"client-go reads every widget under the WorkItemFeatures field that already names it (features.assignees, " +
		"features.labels and so on), so the value restates the key the widget was read under."
	for _, widget := range []string{
		"WorkItemWidgetAssignees", "WorkItemWidgetColor", "WorkItemWidgetHealthStatus", "WorkItemWidgetHierarchy",
		"WorkItemWidgetIteration", "WorkItemWidgetLabels", "WorkItemWidgetLinkedItems", "WorkItemWidgetMilestone",
		"WorkItemWidgetStartAndDueDate", "WorkItemWidgetStatus", "WorkItemWidgetWeight",
	} {
		declarations = append(declarations, sentDeclaration{Package: workItemsService, SchemaType: widget, Field: "type", Category: categoryRestated, Reason: widgetTypeReason})
	}
	for _, field := range []string{"blocked", "blockedByCount", "blockingCount"} {
		declarations = append(declarations, recordedField(workItemsService, "WorkItemWidgetLinkedItems", field, "107", row107))
	}
	for _, field := range []string{"isFixed", "dueDateSourcingMilestone", "dueDateSourcingWorkItem", "startDateSourcingMilestone", "startDateSourcingWorkItem"} {
		declarations = append(declarations, recordedField(workItemsService, "WorkItemWidgetStartAndDueDate", field, "107", row107))
	}
	return declarations
}
