package sdk

// Service declaration categories. The first four mirror the vocabulary
// acceptedMissingMethods already uses one level up, so a reader who knows the
// method table can read this one; the last two are service-level only.
const (
	// coveredRaw: reached through client.GL().NewRequest + Do rather than the
	// wrapper, for a reason the handler documents.
	coveredRaw = "COVERED_RAW"
	// coveredGeneric: the wrapper's methods are passed as VALUES into a generic
	// helper, so they are calls the call-expression scanner cannot see.
	coveredGeneric = "COVERED_GENERIC"
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
// GitLab removed the certificate-based Kubernetes integration outright, so the
// group, instance and project wrappers over it are all superseded by the same
// replacement and carry the same evidence.
const certificateClusters = "certificate-based cluster integration, marked upstream 'Deprecated: in GitLab 14.5, to be removed in 19.0' and removed by GitLab; internal/tools/clusteragents calls ClusterAgents instead"

// declaration adjudicates one client-go service no handler calls.
type declaration struct {
	Category string
	Reason   string
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
	// COVERED_RAW
	"ApplicationStatistics": {coveredRaw, "internal/tools/appstatistics.Get issues a raw GET application/statistics: client-go's ApplicationStatistics declares int64 fields and some GitLab versions answer with string-encoded numbers, so the wrapper cannot decode a successful response. Recorded in docs/development/upstream-bugs.md"},
	"GroupEpicBoards":       {coveredRaw, "internal/tools/groupepicboards issues raw GETs for groups/:id/epic_boards and .../:board_id: the wrapper's GroupEpicBoard type omits documented response fields (hide_backlog_list, hide_closed_list, labels, lists), and the raw request decodes the full documented shape"},

	// COVERED_GENERIC
	"Invites": {coveredGeneric, "internal/tools/invites passes all four methods as method VALUES into generic list/invite helpers (ListPendingProjectInvitations, ListPendingGroupInvitations, ProjectInvites, GroupInvites), which is a call the call-expression scanner cannot see"},

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

	// UNWRAPPED_TRACKED
	"SecurityDependencyFirewall": {unwrappedTracked, "EvaluatePackage is genuinely unexposed; tracked in https://github.com/jmrplens/gitlab-mcp-server/issues/429"},
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
	"epicissues.List":        {decisionKeep, "EpicIssues is documented upstream as 'Will be removed in v5 of the API, use Work Items API instead'; the handler reads the work-item hierarchy widget, which is that replacement"},
	"epicissues.Assign":      {decisionKeep, "EpicIssues.AssignEpicIssue drives the epic REST API being removed; the handler uses the workItemUpdate hierarchy mutation instead"},
	"epicissues.Remove":      {decisionKeep, "EpicIssues.RemoveEpicIssue drives the epic REST API being removed; the handler uses the workItemUpdate hierarchy mutation instead"},
	"epicissues.UpdateOrder": {decisionKeep, "EpicIssues.UpdateEpicIssueAssignment drives the epic REST API being removed; child ordering is a work-item hierarchy mutation"},

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
}
