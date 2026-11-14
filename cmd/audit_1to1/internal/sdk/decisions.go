// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

// Service declaration categories. The first two mirror the vocabulary
// acceptedMissingMethods already uses one level up, so a reader who knows the
// method table can read this one; the last two are service-level only. There
// is no category for a service whose methods are handed on as values: the
// scanner records a method value as reached, so such a service is called.
const (
	// coveredRaw: reached through client.GL().NewRequest + Do rather than the
	// wrapper, for the reason the waits entry that replaces it gives.
	coveredRaw = "COVERED_RAW"
	// coveredGraphQL: the domain is reached over GraphQL (ADR-0006). The
	// decision about whether that is still right lives in graphqlDecisions,
	// per operation.
	coveredGraphQL = "COVERED_GRAPHQL"
	// supersededUpstream: the service wraps an API GitLab has replaced, and we
	// call the replacement's service instead.
	supersededUpstream = "SUPERSEDED_UPSTREAM"
	// unwrappedTracked: genuinely not exposed, with an issue that says so.
	unwrappedTracked = "UNWRAPPED_TRACKED"
)

// securityWrapperRefusalReading is the shared reason behind the eight security
// attribute and category mutations. Up to v3.46.0 the wrappers never read the
// response's top-level errors array, so a mutation GitLab refused with HTTP
// 200 came back as a success, an empty result or a bare ErrNotFound; v3.46.1
// returns that array as a *GraphQLResponseError (gitlab-org/api/client-go!3066,
// upstream-bugs row 19). What still keeps the handlers on their own documents
// is how the wrappers hand the refusal over: UpdateSecurityAttribute,
// CreateSecurityCategory and UpdateSecurityCategory return it as an error that
// also matches ErrNotFound, which toolutil.IsHTTPStatus reads as a 404 and
// toolutil.WriteAnsweredWithoutObject as a write GitLab ran and answered
// without its object, so a fine-grained session would be told a refused
// write was probably committed; DestroySecurityCategory selects no
// deletedAttributesGid, the attributes the delete reports; and the other four
// would change the text of every refusal the handlers' tests hold. Moving them
// is recorded as row 19's remaining work in docs/development/upstream-bugs.md.
const securityWrapperRefusalReading = "since v3.46.1 the wrapper returns the response's top-level GraphQL errors (client-go!3066), but three of the eight (UpdateSecurityAttribute, CreateSecurityCategory, UpdateSecurityCategory) wrap a refusal so it also matches ErrNotFound, which toolutil reads as a 404 and as a write answered without its object, DestroySecurityCategory selects no deletedAttributesGid, and the move of the rest changes every refusal's text; the raw handler reads the array through toolutil.GraphQLTopLevelError until that move is made. Recorded as row 19 of docs/development/upstream-bugs.md (https://github.com/jmrplens/gitlab-mcp-server/issues/430)"

// certificateClusters is the shared reason behind the three cluster services.
// GitLab deprecated the certificate-based Kubernetes integration for the agent
// for Kubernetes and still mounts its three APIs, so the group, instance and
// project wrappers over it are superseded by the same replacement and carry
// the same evidence.
const certificateClusters = "certificate-based cluster integration, marked in client-go 'Deprecated: in GitLab 14.5, to be removed in 19.0'; GitLab 19.5 still mounts the instance, group and project APIs and answers them with 404 unless the certificate_based_clusters ops flag, off by default, is on, or for a group or a project whose namespace was granted the integration (lib/api/project_clusters.rb, app/models/namespace.rb, app/models/clusters/instance.rb, config/feature_flags/ops/certificate_based_clusters.yml); the agent for Kubernetes replaces it, and internal/tools/clusteragents calls ClusterAgents instead"

// declaration adjudicates one client-go service no handler calls.
type declaration struct {
	Category string
	Reason   string
}

// ServiceDeclaration is how the service rule adjudicates a client-go service
// no handler calls: its category (COVERED_GRAPHQL, SUPERSEDED_UPSTREAM,
// UNWRAPPED_TRACKED) and why.
type ServiceDeclaration struct {
	Category string
	Reason   string
}

// DeclaredServices returns the service declarations as a copy a caller may
// keep, keyed by the service's bare name. R-ROUTE reads them to decide
// whether a route no action reaches inherits the decision about the service
// that sends it. The COVERED_RAW ones are not among them: they are derived
// from the waits table, which a reader takes them from with the entry that
// says why.
func DeclaredServices() map[string]ServiceDeclaration {
	out := make(map[string]ServiceDeclaration, len(declaredServices))
	for service, declared := range declaredServices {
		out[service] = ServiceDeclaration(declared)
	}
	return out
}

// declaredServices adjudicates the client-go services no handler calls. Every
// service on the Client struct is either called or listed here; anything else
// is a finding, which is the check that would have caught WorkItemSavedViews on
// the version bump instead of after it.
//
// Keys are bare service names (the interface name without its ServiceInterface
// suffix), NOT Client struct field names. The two differ often enough that
// writing the table against fields invents services: Boards and Avatar are
// fields over IssueBoards and AvatarRequests, both of which handlers call
// directly, so listing them here as exceptions would describe a gap that does
// not exist.
var declaredServices = map[string]declaration{
	// COVERED_RAW is not declared here: a service a raw request stands in
	// for whole is replaced by an entry of the waits table, which holds the
	// workaround to the pinned SDK, and withCoveredRawServices declares it
	// from there.

	// COVERED_GRAPHQL — see graphqlDecisions for the per-operation decision.
	"EpicIssues":             {coveredGraphQL, "internal/tools/epicissues drives the work-item hierarchy widget over GraphQL. The interface itself is documented upstream as 'Will be removed in v5 of the API, use Work Items API instead'"},
	"ProjectVulnerabilities": {coveredGraphQL, "internal/tools/vulnerabilities queries and mutates vulnerabilities over GraphQL, which is what the interface's own upstream doc comment directs to: 'Deprecated: use GraphQL Query.vulnerabilities instead'"},
	"SecurityAttributes":     {coveredGraphQL, "internal/tools/securityattributes issues the securityAttribute* mutations directly; the wrapper drives the same mutations and, since v3.46.1, returns their top-level GraphQL errors, but hands UpdateSecurityAttribute's refusal over as one that also matches ErrNotFound (see graphqlDecisions)"},
	"SecurityCategories":     {coveredGraphQL, "internal/tools/securitycategories issues the securityCategory* mutations directly; the wrapper drives the same mutations and, since v3.46.1, returns their top-level GraphQL errors, but hands the create and update refusals over as ones that also match ErrNotFound and selects no deletedAttributesGid on the destroy (see graphqlDecisions)"},

	// SUPERSEDED_UPSTREAM
	"GeoNodes":         {supersededUpstream, "upstream marks it 'Deprecated: will be removed in v5 of the API, use Geo Sites API instead'; internal/tools/geo calls GeoSites for all eight operations"},
	"GroupClusters":    {supersededUpstream, certificateClusters},
	"InstanceClusters": {supersededUpstream, certificateClusters},
	"ProjectClusters":  {supersededUpstream, certificateClusters},
	"Version":          {supersededUpstream, "GET /version is 'deprecated in 15.5. We recommend you instead use the Metadata API' (lib/api/metadata.rb), which serves the same Entities::Metadata on GET /metadata; client-go's Version struct decodes two of its four keys and Metadata all of them. internal/tools/health and internal/tools/metadata call Metadata.GetMetadata instead. The decision covers the tool surface: outside it, internal/gitlab's Client.Ping, which only the e2e harness calls, still reads the version through Version.GetVersion, and the startup probe and the edition detection read /api/v4/version with requests of their own"},

	// UNWRAPPED_TRACKED
	"GroupApplications":             {unwrappedTracked, "client-go v3.49.0 added it (client-go!2995) for GET and POST /groups/:id/applications and GET, PUT and DELETE /groups/:id/applications/:application_id, with which a group owner lists, creates, reads, renames or rescopes and deletes the OAuth applications the group owns; GitLab merged that API on 2026-11-13 for 19.6 (gitlab-org/gitlab!250319), so no released GitLab serves it and the live record (19.5.0) mounts none of its routes, which leaves an action over it no route, fine-grained permission or e2e instance to be held to until the re-pin; the actions that will reach it are tracked in https://github.com/jmrplens/gitlab-mcp-server/issues/1605"},
	"GroupPlaceholderReassignments": {unwrappedTracked, "client-go v3.37.0 added it for GET and POST /groups/:id/placeholder_reassignments (DownloadPendingReassignments and ReassignPlaceholders), with which a group owner downloads as a CSV file the placeholder users an import left to reassign and reassigns them by uploading it back; no action reaches either route yet, and the actions that will are tracked in https://github.com/jmrplens/gitlab-mcp-server/issues/1605"},
}

// GraphQL operation decisions.
const (
	// decisionKeep: raw GraphQL stays, and the reason says why the wrapper's
	// existence does not retire the exemption.
	decisionKeep = "KEEP"
	// decisionMigrate: the wrapper covers the call and the operation should
	// move to it; the reason names where that is tracked.
	decisionMigrate = "MIGRATE"
)

// decision adjudicates one raw-GraphQL operation whose domain now has a
// client-go service wrapper.
type decision struct {
	Decision string
	Reason   string
	// Replaces names the client-go methods a KEEP operation reads in place
	// of, keyed "Service.Method" as acceptedMissingMethods keys them: a
	// method of a service the handlers call, which none of them calls
	// because this operation answers what it would. The service is the one
	// the operation's package resolves to, the one it is adjudicated
	// against, since an operation reads in place of that service's methods
	// and of no other's. R-ACTION adjudicates each one COVERED_GRAPHQL from
	// here ([CoveredGraphQLMethods]), so the reason is written once, beside
	// the operation it is about, and the method goes back to R-ACTION's list
	// the day the decision retires. A method of a service no handler calls
	// is answered by that service's declaration and is not named here.
	Replaces []string
}

// graphqlServiceAliases maps a tool package to the client-go service covering
// its domain where the two spellings differ. Without it the rule only sees the
// cases whose package name happens to equal the service name, which is how a
// domain as central as vulnerabilities would go unexamined.
//
// Deliberately absent: epicnotes, epicdiscussions and epicworkitems. Their
// GraphQL handlers do have SDK counterparts on Notes and Discussions, but
// those are per-METHOD gaps on services other packages call, and they are
// already adjudicated as COVERED_GRAPHQL in acceptedMissingMethods one level
// up. Aliasing them here would restate an existing decision in a second table,
// and two tables that can disagree about the same fact are worse than one.
var graphqlServiceAliases = map[string]string{
	"vulnerabilities": "ProjectVulnerabilities",
}

// graphqlDecisions adjudicates the raw-GraphQL operations whose domain has a
// client-go service, keyed "<package>.<function>". ADR-0006 admits raw
// GraphQL.Do() for domains WITHOUT a wrapper, so the wrapper appearing later is
// what retires the exemption. Nothing checked for that, which made the
// exemption permanent in practice; this table is where it stops being.
//
// The unit is the operation, not the package: several packages use GraphQL for
// one operation and the SDK for the rest, and a package-level verdict would be
// mostly noise.
var graphqlDecisions = map[string]decision{
	// KEEP — the wrapper exists but wraps an API upstream has deprecated in
	// favor of the very GraphQL these handlers call, so migrating would move
	// us onto the path GitLab is removing.
	"epicissues.List":             {Decision: decisionKeep, Reason: "EpicIssues is documented upstream as 'Will be removed in v5 of the API, use Work Items API instead'; the handler reads the work-item hierarchy widget, which is that replacement"},
	"epicissues.Assign":           {Decision: decisionKeep, Reason: "EpicIssues.AssignEpicIssue drives the epic REST API being removed; the handler uses the workItemUpdate hierarchy mutation instead"},
	"epicissues.Remove":           {Decision: decisionKeep, Reason: "EpicIssues.RemoveEpicIssue drives the epic REST API being removed; the handler uses the workItemUpdate hierarchy mutation instead"},
	"epicissues.UpdateOrder":      {Decision: decisionKeep, Reason: "EpicIssues.UpdateEpicIssueAssignment drives the epic REST API being removed; child ordering is a work-item hierarchy mutation"},
	"epicissues.checkChildOfEpic": {Decision: decisionKeep, Reason: "the guard Remove and UpdateOrder run before they write reads the item's parent from the work-item hierarchy widget; EpicIssues has no read of the epic one issue sits in, and its list is the REST API being removed"},

	"vulnerabilities.List":                     {Decision: decisionKeep, Reason: "ProjectVulnerabilities is documented upstream as 'Deprecated: use GraphQL Query.vulnerabilities instead', which is precisely what this handler does"},
	"vulnerabilities.Get":                      {Decision: decisionKeep, Reason: "same deprecation; the wrapper has no single-vulnerability read at all, only ListProjectVulnerabilities and CreateVulnerability"},
	"vulnerabilities.SeverityCount":            {Decision: decisionKeep, Reason: "severity counts are a GraphQL aggregate (vulnerabilitySeveritiesCount) with no REST or wrapper equivalent"},
	"vulnerabilities.PipelineSecuritySummary":  {Decision: decisionKeep, Reason: "the pipeline security report summary is GraphQL-only; the wrapper exposes nothing pipeline-scoped"},
	"vulnerabilities.runVulnerabilityMutation": {Decision: decisionKeep, Reason: "dismiss/confirm/resolve/revert are GraphQL mutations; the wrapper exposes only CreateVulnerability"},

	// KEEP: the security wrappers return the top-level GraphQL errors array
	// since v3.46.1, where GitLab reports a rejected mutation with HTTP 200,
	// but three of them hand the refusal over as one that also matches
	// ErrNotFound and the category delete selects less than the handler
	// reports. Recorded as row 19 of docs/development/upstream-bugs.md and
	// examined in https://github.com/jmrplens/gitlab-mcp-server/issues/430.
	"securityattributes.createSecurityAttributes":       {Decision: decisionKeep, Reason: securityWrapperRefusalReading},
	"securityattributes.updateSecurityAttribute":        {Decision: decisionKeep, Reason: securityWrapperRefusalReading},
	"securityattributes.destroySecurityAttribute":       {Decision: decisionKeep, Reason: securityWrapperRefusalReading},
	"securityattributes.projectUpdateSecurityAttribute": {Decision: decisionKeep, Reason: securityWrapperRefusalReading},
	"securityattributes.bulkUpdateSecurityAttributes":   {Decision: decisionKeep, Reason: securityWrapperRefusalReading},

	"securitycategories.createSecurityCategory":  {Decision: decisionKeep, Reason: securityWrapperRefusalReading},
	"securitycategories.updateSecurityCategory":  {Decision: decisionKeep, Reason: securityWrapperRefusalReading},
	"securitycategories.destroySecurityCategory": {Decision: decisionKeep, Reason: securityWrapperRefusalReading},

	// KEEP: the wrapper cannot page. TerraformStates.List sends a document that
	// takes neither first nor after and selects neither count nor pageInfo, and
	// GitLab caps the connection at 100 nodes, so a project holding more states
	// read as holding 100 (issue 1341). The handler's document selects of each
	// state exactly what List and Get select; it retires when client-go's List
	// pages, which upstream-bugs row 104 records as a commit not yet sent: it
	// was to be added to gitlab-org/api/client-go!3063, closed unmerged on
	// 2026-10-21, and goes out instead in a small merge request on its topic.
	// The handlers call TerraformStates for every other read, so the decision
	// names List as the method it reads in place of, and R-ACTION does not list
	// it as an action to write.
	"terraformstates.listPage": {
		Decision: decisionKeep,
		Reason:   "TerraformStates.List takes no first or after and selects no count or pageInfo, so a project with more than 100 states read as holding 100; the handler's paged document selects of each state what List and Get select, and retires when client-go's List pages (docs/development/upstream-bugs.md row 104)",
		Replaces: []string{"TerraformStates.List"},
	},

	// KEEP: client-go's Orbit service wraps the REST routes GitLab.com removed
	// on 2026-10-27 and -28 (gitlab-org/gitlab!265633, !264745, !265998 and
	// !266009), and GitLab answers each of these through GraphQL instead, which
	// the service has no method for. Its Query and QueryRaw, the one route
	// still served, are what orbit.Query calls. Each method that sends a
	// removed route is a row of docs/development/upstream-bugs.md. These
	// replace no method here: acceptedMissingMethods answers the Orbit methods
	// as SUPERSEDED_UPSTREAM, the decision R-ROUTE carries onto the removed
	// routes, and R-ACTION refuses a method two tables answer.
	"orbit.Status":                 {Decision: decisionKeep, Reason: orbitRoutesRemoved},
	"orbit.schemaPage":             {Decision: decisionKeep, Reason: orbitRoutesRemoved},
	"orbit.graphStatusByPath":      {Decision: decisionKeep, Reason: orbitRoutesRemoved},
	"orbit.graphStatusByGroupID":   {Decision: decisionKeep, Reason: orbitRoutesRemoved},
	"orbit.graphStatusByProjectID": {Decision: decisionKeep, Reason: orbitRoutesRemoved},
}

// orbitRoutesRemoved is the shared reason behind the Orbit GraphQL operations.
const orbitRoutesRemoved = "client-go's Orbit.GetStatus, GetSchema and GetGraphStatus send GET /orbit/status, /orbit/schema and /orbit/graph_status, which GitLab.com answers 404 since gitlab-org/gitlab!265633 and !264745 removed them (2026-10-27 and -28); GitLab answers the same through GraphQL (Query.orbit.serviceStatus, Query.orbit.schema, Group.orbit and Project.orbit), which the service has no method for (docs/development/upstream-bugs.md)"

// optionStructDeclarations answers the option structs nothing fills, keyed by
// the struct's client-go name, and the fields a copy leaves out on purpose,
// keyed by [copyKey]. Every struct a request a handler sends carries is
// otherwise held to an input: paired with one by the field diff, converted
// from a held struct, or copied from one field by field.
var optionStructDeclarations = map[string]optionStructDeclaration{
	"UpdateSettingsOptions":         {categoryMapInput, "internal/tools/settings.Update decodes the input's settings map into it (json.Unmarshal), after refusing every key the struct does not model or PUT /application/settings does not declare; TestModeledSettingKeys_ReadTheOptionsStructRatherThanAList and TestUndeclaredSettingKeys_AreTheModeledKeysTheUpdateRouteDoesNotDeclare hold those keys to the struct's json tags"},
	"ListActiveIntegrationsOptions": {categorySDKModelsUnused, "it embeds gl.ListOptions and nothing else, and GET /groups/:id/integrations declares id alone (gitlab-api-live.json, 19.5.0-ee): GitLab answers the whole list unpaged, so internal/tools/integrations.ListGroupIntegrations sends nil"},
	"GroupAccessLevel":              {categoryScalarList, "DefaultBranchProtectionDefaultsOptions takes allowed_to_push and allowed_to_merge as lists of {access_level}, and groups.BranchProtectionDefaultsInput offers each as []int, one access level per element, which groups.accessLevelOptions turns into one struct each"},
}
