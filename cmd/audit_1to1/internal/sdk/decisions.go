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

// securityWrapperSwallowsErrors is the shared reason behind the eight security
// attribute and category mutations. The wrappers select exactly the fields the
// handlers select, so the field-level case for migrating is sound; what stops
// it is one line they all lack. Each unmarshals into a struct embedding
// GenericGraphQLErrors and then never looks at it, and GraphQL.Do only errors
// on a non-2xx status, so a mutation GitLab refuses at the query level (an
// authorization refusal, a schema mismatch on an older instance) arrives as
// HTTP 200 with a null payload: destroy and bulk update return no error at all,
// create returns an empty list, and update degrades to a bare ErrNotFound that
// drops GitLab's message. The raw handlers read that array through
// toolutil.GraphQLTopLevelError, which is what internal/tools/securityattributes
// and internal/tools/securitycategories test under
// TestHandlers_WrapTopLevelGraphQLErrors.
const securityWrapperSwallowsErrors = "the wrapper selects the same fields but never reads the response's top-level GraphQL errors array, and GraphQL.Do only errors on a non-2xx status, so a mutation GitLab refuses with HTTP 200 is reported as success (destroy, bulk update), as an empty result (create), or as a bare ErrNotFound that drops GitLab's message (update); the raw handler surfaces it through toolutil.GraphQLTopLevelError. Recorded in docs/development/upstream-bugs.md (https://github.com/jmrplens/gitlab-mcp-server/issues/430)"

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
	"SecurityAttributes":     {coveredGraphQL, "internal/tools/securityattributes issues the securityAttribute* mutations directly; the wrapper drives the same mutations but discards the response's top-level GraphQL errors, so adopting it would report a refused mutation as a success (see graphqlDecisions)"},
	"SecurityCategories":     {coveredGraphQL, "internal/tools/securitycategories issues the securityCategory* mutations directly; the wrapper drives the same mutations but discards the response's top-level GraphQL errors, so adopting it would report a refused mutation as a success (see graphqlDecisions)"},

	// SUPERSEDED_UPSTREAM
	"GeoNodes":         {supersededUpstream, "upstream marks it 'Deprecated: will be removed in v5 of the API, use Geo Sites API instead'; internal/tools/geo calls GeoSites for all eight operations"},
	"GroupClusters":    {supersededUpstream, certificateClusters},
	"InstanceClusters": {supersededUpstream, certificateClusters},
	"ProjectClusters":  {supersededUpstream, certificateClusters},
	"Version":          {supersededUpstream, "GET /version is 'deprecated in 15.5. We recommend you instead use the Metadata API' (lib/api/metadata.rb), which serves the same Entities::Metadata on GET /metadata; client-go's Version struct decodes two of its four keys and Metadata all of them. internal/tools/health and internal/tools/metadata call Metadata.GetMetadata instead. The decision covers the tool surface: outside it, internal/gitlab's Client.Ping, which only the e2e harness calls, still reads the version through Version.GetVersion, and the startup probe and the edition detection read /api/v4/version with requests of their own"},
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
	"epicissues.List":             {decisionKeep, "EpicIssues is documented upstream as 'Will be removed in v5 of the API, use Work Items API instead'; the handler reads the work-item hierarchy widget, which is that replacement"},
	"epicissues.Assign":           {decisionKeep, "EpicIssues.AssignEpicIssue drives the epic REST API being removed; the handler uses the workItemUpdate hierarchy mutation instead"},
	"epicissues.Remove":           {decisionKeep, "EpicIssues.RemoveEpicIssue drives the epic REST API being removed; the handler uses the workItemUpdate hierarchy mutation instead"},
	"epicissues.UpdateOrder":      {decisionKeep, "EpicIssues.UpdateEpicIssueAssignment drives the epic REST API being removed; child ordering is a work-item hierarchy mutation"},
	"epicissues.checkChildOfEpic": {decisionKeep, "the guard Remove and UpdateOrder run before they write reads the item's parent from the work-item hierarchy widget; EpicIssues has no read of the epic one issue sits in, and its list is the REST API being removed"},

	"vulnerabilities.List":                     {decisionKeep, "ProjectVulnerabilities is documented upstream as 'Deprecated: use GraphQL Query.vulnerabilities instead', which is precisely what this handler does"},
	"vulnerabilities.Get":                      {decisionKeep, "same deprecation; the wrapper has no single-vulnerability read at all, only ListProjectVulnerabilities and CreateVulnerability"},
	"vulnerabilities.SeverityCount":            {decisionKeep, "severity counts are a GraphQL aggregate (vulnerabilitySeveritiesCount) with no REST or wrapper equivalent"},
	"vulnerabilities.PipelineSecuritySummary":  {decisionKeep, "the pipeline security report summary is GraphQL-only; the wrapper exposes nothing pipeline-scoped"},
	"vulnerabilities.runVulnerabilityMutation": {decisionKeep, "dismiss/confirm/resolve/revert are GraphQL mutations; the wrapper exposes only CreateVulnerability"},

	// KEEP — the security wrappers select the same fields but never read the
	// top-level GraphQL errors array, which is where GitLab reports a rejected
	// mutation with HTTP 200. Adopting them would turn a refusal into a
	// success. Recorded in docs/development/upstream-bugs.md and examined in
	// https://github.com/jmrplens/gitlab-mcp-server/issues/430.
	"securityattributes.createSecurityAttributes":       {decisionKeep, securityWrapperSwallowsErrors},
	"securityattributes.updateSecurityAttribute":        {decisionKeep, securityWrapperSwallowsErrors},
	"securityattributes.destroySecurityAttribute":       {decisionKeep, securityWrapperSwallowsErrors},
	"securityattributes.projectUpdateSecurityAttribute": {decisionKeep, securityWrapperSwallowsErrors},
	"securityattributes.bulkUpdateSecurityAttributes":   {decisionKeep, securityWrapperSwallowsErrors},

	"securitycategories.createSecurityCategory":  {decisionKeep, securityWrapperSwallowsErrors},
	"securitycategories.updateSecurityCategory":  {decisionKeep, securityWrapperSwallowsErrors},
	"securitycategories.destroySecurityCategory": {decisionKeep, securityWrapperSwallowsErrors},

	// KEEP: the wrapper cannot page. TerraformStates.List sends a document that
	// takes neither first nor after and selects neither count nor pageInfo, and
	// GitLab caps the connection at 100 nodes, so a project holding more states
	// read as holding 100 (issue 1341). The handler's document selects of each
	// state exactly what List and Get select; it retires when client-go's List
	// pages, which upstream-bugs row 104 records as a commit not yet sent: it
	// was to be added to gitlab-org/api/client-go!3063, closed unmerged on
	// 2026-10-21, and goes out instead in a small merge request on its topic.
	"terraformstates.listPage": {decisionKeep, "TerraformStates.List takes no first or after and selects no count or pageInfo, so a project with more than 100 states read as holding 100; the handler's paged document selects of each state what List and Get select, and retires when client-go's List pages (docs/development/upstream-bugs.md row 104)"},

	// KEEP: client-go's Orbit service wraps the REST routes GitLab.com removed
	// on 2026-10-27 and -28 (gitlab-org/gitlab!265633, !264745, !265998 and
	// !266009), and GitLab answers each of these through GraphQL instead, which
	// the service has no method for. Its Query and QueryRaw, the one route
	// still served, are what orbit.Query calls. Each method that sends a
	// removed route is a row of docs/development/upstream-bugs.md.
	"orbit.Status":                 {decisionKeep, orbitRoutesRemoved},
	"orbit.schemaPage":             {decisionKeep, orbitRoutesRemoved},
	"orbit.graphStatusByPath":      {decisionKeep, orbitRoutesRemoved},
	"orbit.graphStatusByGroupID":   {decisionKeep, orbitRoutesRemoved},
	"orbit.graphStatusByProjectID": {decisionKeep, orbitRoutesRemoved},
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
