package main

// The answers for the two GraphQL-only security domains, vulnerabilities and
// securityfindings, and for the shapes both decode through one struct of
// internal/toolutil (the location union, the person who changed a state, a
// leaked token's status).
//
// Issue 967 triaged the 99 fields the pinned schema offered at the objects
// these packages decode and no document of theirs selected. The ones a caller
// triaging a vulnerability needs are selected and published now: who changed
// its state and what they wrote, the CVSS and EPSS scores, whether a leaked
// secret still works, the report's links, the remediation a scanner proposed,
// the HTTP exchange a DAST scan recorded as evidence, and every member of the
// location union. What is left is answered below, each with the reason a
// reviewer can check against GitLab's own GraphQL reference or resolvers.

// Where the security findings are filed.
const (
	vulnerabilitiesPackage  = toolsDir + "/vulnerabilities"
	securityFindingsPackage = toolsDir + "/securityfindings"
)

// The schema types the declarations below are keyed by.
const (
	vulnerabilityType        = "Vulnerability"
	securityFindingType      = "PipelineSecurityReportFinding"
	vulnerabilityScannerType = "VulnerabilityScanner"
)

// securitySentDeclarations answers the sent findings of the two security
// domains and of the shapes they share.
func securitySentDeclarations() []sentDeclaration {
	declarations := []sentDeclaration{
		{
			Package:    vulnerabilitiesPackage,
			SchemaType: "Project",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason:     referenceStub("project", "gitlab_project"),
		},
		{
			Package:    vulnerabilitiesPackage,
			SchemaType: "MergeRequest",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason:     referenceStub("merge request that fixes the vulnerability", "gitlab_merge_request"),
		},
		{
			Package:    vulnerabilitiesPackage,
			SchemaType: "Issue",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason:     referenceStub("issue linked to the vulnerability", "gitlab_issue"),
		},
		{
			Package:    vulnerabilitiesPackage,
			SchemaType: "VulnerabilityIssueLink",
			Field:      "id",
			Category:   categoryUnusedIdentifier,
			Reason: "The link's own global id. No action of this server creates or removes a link between a " +
				"vulnerability and an issue, so the id is a handle nothing here takes; the link's type and the " +
				"issue it names are published as issue_links.",
		},
		{
			Package:    securityFindingsPackage,
			SchemaType: vulnerabilityType,
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: referenceStub("vulnerability a pipeline finding was promoted to", "gitlab_vulnerability") +
				" The vulnerabilities package sends its own documents against this same object, and is held to this " +
				"question on them.",
		},
		{
			Package:    securityFindingsPackage,
			SchemaType: securityFindingType,
			Field:      "project",
			Category:   categoryNotThisResponse,
			Reason: "Every finding of the pipeline belongs to the project the caller named as project_path, so the " +
				"project says nothing the request did not; its own fields are the projects domain's surface, " +
				"published by gitlab_project.",
		},
		{
			Package:    securityFindingsPackage,
			SchemaType: securityFindingType,
			Field:      "issueLinks",
			Category:   categoryPublishedElsewhere,
			Reason: "GitLab resolves a finding's issue links through the vulnerability the finding was promoted " +
				"to (Types::PipelineSecurityReportFindingType#issue_links loads Vulnerability.with_findings_by_uuid " +
				"and answers vulnerability.issue_links), so they are that vulnerability's links. This response names " +
				"the vulnerability as vulnerability_id, and vulnerability.get publishes its issue_links.",
		},
		{
			Package:    securityFindingsPackage,
			SchemaType: securityFindingType,
			Field:      "mergeRequest",
			Category:   categoryPublishedElsewhere,
			Reason: "GitLab resolves a finding's merge request through the vulnerability's merge request links " +
				"(Types::PipelineSecurityReportFindingType#merge_request reads Vulnerabilities::MergeRequestLink by " +
				"the finding's uuid), so it is the merge request of the vulnerability named here as " +
				"vulnerability_id, which vulnerability.get publishes as merge_request.",
		},
		{
			Package:    securityFindingsPackage,
			SchemaType: securityFindingType,
			Field:      "severityOverrides",
			Category:   categorySeparateAction,
			Reason: "The history of the finding's severity changes, paged as a connection. A history is a list a " +
				"caller asks for on its own, not a field every finding of a page carries; the severity before any " +
				"override is published as original_severity beside the current one.",
		},
		{
			Package:    toolutilDir,
			SchemaType: "UserCore",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: "The object is a person GitLab recorded against a vulnerability or a finding (who confirmed, " +
				"dismissed or resolved it), selected by username, name and profile URL so a reader knows who it was. " +
				"Its own fields are the users domain's surface, published through gitlab_user, and a vulnerability " +
				"answering with a person's saved replies, callouts and group memberships would be that domain twice.",
		},
		{
			Package:    toolutilDir,
			SchemaType: "ClusterAgent",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: referenceStub("cluster agent that ran a cluster image scan",
				"the admin.cluster_agent_get and admin.cluster_agent_list actions"),
		},
		{
			Package:    toolutilDir,
			SchemaType: "VulnerabilityFindingTokenStatus",
			Field:      "id",
			Category:   categoryUnusedIdentifier,
			Reason: "The token status record's own global id. What a caller needs of it is whether the leaked " +
				"secret still works and when GitLab last checked, which are published as token_status; no action " +
				"of this server takes the record's id.",
		},
	}
	for _, pkg := range []string{vulnerabilitiesPackage, securityFindingsPackage} {
		declarations = append(declarations, scannerDeclarations(pkg)...)
	}
	declarations = append(declarations, vulnerabilityDeclarations()...)
	return append(declarations, findingDeclarations()...)
}

// scannerDeclarations answers the scanner fields both packages leave out, the
// same three in each.
func scannerDeclarations(pkg string) []sentDeclaration {
	return []sentDeclaration{
		{
			Package:    pkg,
			SchemaType: vulnerabilityScannerType,
			Field:      "id",
			Category:   categoryUnusedIdentifier,
			Reason: "The scanner record's own global id. The scanner filter this server offers takes the id the " +
				"report gives the scanner (externalId), which is published, and no action takes the global id, so " +
				"it would be a handle nothing here accepts.",
		},
		{
			Package:    pkg,
			SchemaType: vulnerabilityScannerType,
			Field:      "reportType",
			Category:   categoryPublishedElsewhere,
			Reason: "The report type the scanner reported under is the report type of the object it sits on, a " +
				"vulnerability or a finding, which that object already publishes as report_type.",
		},
		{
			Package:    pkg,
			SchemaType: vulnerabilityScannerType,
			Field:      "reportTypeHumanized",
			Category:   categoryPublishedElsewhere,
			Reason: "A display label of that same report type (Dependency Scanning for DEPENDENCY_SCANNING), which " +
				"report_type already publishes in the spelling the report_type filter takes.",
		},
	}
}

// vulnerabilityDeclarations answers the Vulnerability fields no document of
// the vulnerabilities package selects.
func vulnerabilityDeclarations() []sentDeclaration {
	declarations := make([]sentDeclaration, 0, 31)
	for _, experiment := range []struct{ field, introduced, meaning string }{
		{"aiWorkflows", "18.6", "the AI workflows triggered for the vulnerability"},
		{"archivalInformation", "17.11", "whether the vulnerability is about to be archived in the next month"},
		{"ascpComponent", "19.4", "the ASCP component the vulnerability belongs to, null while the ascp_component_vulnerability_association feature flag is off"},
		{"dependencies", "18.2", "the dependencies the vulnerability affects"},
		{"duoSastVrWorkflowEnabled", "19.1", "whether the SAST vulnerability review workflow is enabled for the project"},
		{"flags", "18.5", "the flags set on the vulnerability"},
		{"initialDetectedPipeline", "18.2", "the pipeline where the vulnerability was first detected"},
		{"latestDetectedPipeline", "18.2", "the pipeline where the vulnerability was last detected"},
		{"latestFlag", "18.5", "the latest flag set on the vulnerability"},
		{"latestNonClosedMergeRequest", "19.1", "the latest non-closed merge request linked to fix the vulnerability"},
		{"latestSecurityReportFinding", "18.4", "the latest security report finding, which GitLab warns can time out on a large project"},
		{"malware", "19.0", "whether the vulnerability is associated with a malware package"},
		{"policyAutoDismissed", "18.7", "whether a security policy dismissed the vulnerability"},
		{"policyViolations", "18.6", "the policy violation for the vulnerability"},
		{"reachability", "17.11", "whether the vulnerable code is reachable"},
		{"representationInformation", "17.7", "the commit the vulnerability was resolved in"},
		{"trackedRef", "18.10", "the branch or tag the vulnerability was detected on"},
	} {
		declarations = append(declarations, sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      experiment.field,
			Category:   categoryExperiment,
			Reason:     experimentReason(experiment.field, experiment.introduced, experiment.meaning),
		})
	}
	return append(declarations,
		aiResolutionDeclaration(vulnerabilitiesPackage, vulnerabilityType, "aiResolutionAvailable", "the type of vulnerability"),
		aiResolutionDeclaration(vulnerabilitiesPackage, vulnerabilityType, "aiResolutionEnabled", "this vulnerability"),
		sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      "userPermissions",
			Category:   categoryAffordance,
			Reason:     permissionsReason("nine booleans (admin, feedback, issue links, export)"),
		},
		notesDeclaration("commenters", "the users who commented"),
		notesDeclaration("discussions", "the discussion threads"),
		notesDeclaration("notes", "the notes"),
		sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      "externalIssueLinks",
			Category:   categorySeparateAction,
			Reason: "The links to issues in an external tracker (Jira), paged as a connection. Reading an external " +
				"tracker's issues is a list a caller asks for on its own; the GitLab issues linked to the " +
				"vulnerability are published as issue_links.",
		},
		sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      "mergeRequests",
			Category:   categorySeparateAction,
			Reason: "Every merge request linked to fix the vulnerability, paged as a connection. GitLab resolves " +
				"mergeRequest to the first of this same list (Types::VulnerabilityType#merge_request is " +
				"merge_requests.first), which is published as merge_request; the rest would be a list action.",
		},
		sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      "severityOverrides",
			Category:   categorySeparateAction,
			Reason: "The history of the vulnerability's severity changes, paged as a connection. A history is a " +
				"list a caller asks for on its own, not a field every vulnerability of a page carries.",
		},
		sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      "stateTransitions",
			Category:   categorySeparateAction,
			Reason: "The history of the vulnerability's state changes, paged as a connection. What a caller " +
				"reading the vulnerability needs of it is published: the latest transition's comment as " +
				"state_comment, and who confirmed, dismissed or resolved it with when.",
		},
		detailsDeclaration(vulnerabilitiesPackage, vulnerabilityType),
		htmlDeclaration(vulnerabilitiesPackage, vulnerabilityType, "descriptionHtml", "description"),
		sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      "name",
			Category:   categoryPublishedElsewhere,
			Reason: "Vulnerability implements Todoable, whose name field GitLab's GraphQL reference documents as " +
				"the name or title of the object: for a vulnerability it is the title, which is published as title.",
		},
		sentDeclaration{
			Package:    vulnerabilitiesPackage,
			SchemaType: vulnerabilityType,
			Field:      "vulnerabilityPath",
			Category:   categoryPublishedElsewhere,
			Reason: "The path of the vulnerability's details page, whose full address is published as web_url " +
				"(Vulnerability.webUrl); the relative path alone is nothing a caller can open.",
		},
	)
}

// findingDeclarations answers the PipelineSecurityReportFinding fields no
// document of the securityfindings package selects, beyond the four answered
// alongside the shared ones.
func findingDeclarations() []sentDeclaration {
	return []sentDeclaration{
		aiResolutionDeclaration(securityFindingsPackage, securityFindingType, "aiResolutionAvailable", "the type of finding"),
		aiResolutionDeclaration(securityFindingsPackage, securityFindingType, "aiResolutionEnabled", "this finding"),
		{
			Package:    securityFindingsPackage,
			SchemaType: securityFindingType,
			Field:      "userPermissions",
			Category:   categoryAffordance,
			Reason:     permissionsReason("two booleans (admin the vulnerability, create an issue)"),
		},
		detailsDeclaration(securityFindingsPackage, securityFindingType),
		htmlDeclaration(securityFindingsPackage, securityFindingType, "descriptionHtml", "description"),
		htmlDeclaration(securityFindingsPackage, securityFindingType, "solutionHtml", "solution"),
	}
}

// experimentReason cites the entry of GitLab's GraphQL API reference that
// marks a field an experiment.
func experimentReason(field, introduced, meaning string) string {
	return "GitLab's GraphQL API reference marks " + field + " (" + meaning + ") \"Introduced in GitLab " +
		introduced + ". Status: Experiment.\" GitLab may change or remove an experiment without notice and refuses " +
		"a whole document naming a field it no longer has, so selecting it would stake the list, the get and the " +
		"four state changes, which share one selection, on the experiment. It is selected once GitLab declares it " +
		"generally available."
}

// aiResolutionDeclaration answers a flag saying whether GitLab Duo can resolve
// a vulnerability or a finding.
func aiResolutionDeclaration(pkg, schemaType, field, subject string) sentDeclaration {
	return sentDeclaration{
		Package:    pkg,
		SchemaType: schemaType,
		Field:      field,
		Category:   categoryAffordance,
		Reason: "Whether " + subject + " can be resolved with AI: the web UI reads it to decide whether to offer " +
			"GitLab Duo's Resolve with AI. No action of this server starts an AI resolution, so the flag would " +
			"describe a control nothing here can press.",
	}
}

// permissionsReason answers a permissions object.
func permissionsReason(contents string) string {
	return "The permissions object tells the web UI which controls to draw for the viewer: " + contents + ". This " +
		"server answers the same question at the call, where GitLab refuses what the token may not do, and " +
		"publishing the object would hand a model a second, weaker answer to a question GitLab already settles."
}

// notesDeclaration answers one of the three noteable connections of a
// vulnerability.
func notesDeclaration(field, what string) sentDeclaration {
	return sentDeclaration{
		Package:    vulnerabilitiesPackage,
		SchemaType: vulnerabilityType,
		Field:      field,
		Category:   categorySeparateAction,
		Reason: "The vulnerability's conversation, " + what + ", paged as a connection. Their count is published as " +
			"user_notes_count; reading them is a notes action of its own, the way every other noteable in this " +
			"server is served, rather than a field every vulnerability of a list carries.",
	}
}

// detailsDeclaration answers the recursive details union.
func detailsDeclaration(pkg, schemaType string) sentDeclaration {
	return sentDeclaration{
		Package:    pkg,
		SchemaType: schemaType,
		Field:      "details",
		Category:   categoryRecursiveShape,
		Reason: "details is GitLab's VulnerabilityDetail union of fifteen shapes, and its list and table members " +
			"hold the union again inside themselves (VulnerabilityDetailList.items, VulnerabilityDetailTable." +
			"headers), so no document can select the whole of it: any selection stops at an arbitrary depth and " +
			"silently drops what a report nested below it. The fields that describe the object itself (title, " +
			"description, solution, identifiers, location, links, evidence) are published.",
	}
}

// htmlDeclaration answers the HTML rendering of a Markdown field that is
// published as written.
func htmlDeclaration(pkg, schemaType, field, source string) sentDeclaration {
	return sentDeclaration{
		Package:    pkg,
		SchemaType: schemaType,
		Field:      field,
		Category:   categoryPublishedElsewhere,
		Reason: "The rendering of " + source + " from GitLab Flavored Markdown to HTML, for the web UI. The " +
			"Markdown it is rendered from is published as " + source + ", and a second rendering of the same text " +
			"tells a caller nothing the first does not.",
	}
}
