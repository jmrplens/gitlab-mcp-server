package join

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// fixtureSDL is the GraphQL schema the join's fixture documents validate
// against: one shape per rule the join reads, named after the GitLab types
// each stands for.
const fixtureSDL = `
schema { query: Query mutation: Mutation }

type Query {
  project(fullPath: ID!): Project
  namespace(fullPath: ID!): Namespace
  vulnerability(id: ID!): Vulnerability
  workItem(id: ID!): WorkItem
  users: UserCoreConnection
  plain: Plain
  viewer: UserCore!
  version: String
}

type Mutation {
  issueCreate(input: ThingInput!): IssueCreatePayload
  undeclaredThing(input: ThingInput!): IssueCreatePayload
  skippedThing(input: ThingInput!): IssueCreatePayload
  labelCreate(input: ThingInput!): LabelCreatePayload
  workItemUpdate(input: ThingInput!): WorkItemUpdatePayload
  emptyPayload(input: ThingInput!): EmptyPayload
  bareThing(input: ThingInput!): EmptyPayload
}

input ThingInput { title: String }

type IssueCreatePayload { errors: [String!]! issue: Issue }
type LabelCreatePayload { errors: [String!]! label: Label }
type WorkItemUpdatePayload { errors: [String!]! workItem: WorkItem }
type EmptyPayload { errors: [String!]! }

type Project {
  id: ID!
  name: String
  issues: IssueConnection
  branchRules: BranchRuleConnection
  counts: Counts!
}
type Counts { total: Int }
type IssueConnection { nodes: [Issue] edges: [IssueEdge] pageInfo: PageInfo! count: Int }
type IssueEdge { node: Issue cursor: String }
type PageInfo { hasNextPage: Boolean! }
type Issue { id: ID! title: String author: UserCore! labels: [Label!] notes: [Note] }
type Label { title: String }
type Note { body: String }
type UserCore { username: String }
type UserCoreConnection { nodes: [UserCore] }
type BranchRuleConnection { nodes: [BranchRule] }
type BranchRule { name: String }
type Namespace { id: ID! name: String }
type Vulnerability {
  id: ID!
  details: [VulnerabilityDetail!]!
  issueLinks: VulnerabilityIssueLinkConnection
  location: VulnerabilityLocation
}
union VulnerabilityDetail = DetailText | DetailCode
type DetailText { text: String }
type DetailCode { code: String }
union VulnerabilityLocation = LocationSast | LocationDast
type LocationSast { file: String dependency: Dependency }
type Dependency { name: String }
type LocationDast { host: String }
type VulnerabilityIssueLinkConnection { nodes: [VulnerabilityIssueLink] }
type VulnerabilityIssueLink { id: ID! }
type WorkItem { id: ID! title: String }
type Plain { name: String items: [PlainItem!]! node: PlainItem }
type PlainItem { name: String }
`

// directive is one GranularScope directive of the fixture.
func directive(group, boundary string, perms ...string) apilive.Directive {
	return apilive.Directive{Permissions: perms, BoundaryType: boundary, RequirementGroup: group}
}

// enforced is an enforced fixture type declaring directives.
func enforced(directives ...apilive.Directive) apilive.GraphQLType {
	return apilive.GraphQLType{Enforced: true, Granular: directives}
}

// withFields gives a type the object-field signatures the record carries.
func withFields(typ apilive.GraphQLType, fields map[string]string) apilive.GraphQLType {
	typ.ObjectFields = map[string]apilive.ObjectField{}
	for name, signature := range fields {
		typ.ObjectFields[name] = apilive.ObjectField{Type: signature}
	}
	return typ
}

// assignable is one assignable permission of the fixture vocabulary.
func assignable(name, display string, boundaries []string, perms ...string) apilive.Assignable {
	return apilive.Assignable{
		Name: name, Display: display, Boundaries: boundaries, Permissions: perms,
		AvailableFor: []string{"granular_access_token"},
	}
}

// authorized is a route authorization holding permissions at one boundary.
func authorized(boundary string, perms ...string) *apilive.RouteAuthorization {
	return &apilive.RouteAuthorization{Permissions: perms, BoundaryType: boundary}
}

// route is one record route under the API prefix.
func route(method, path string, auth *apilive.RouteAuthorization) apilive.Route {
	return apilive.Route{Method: method, Path: apilive.EndpointPrefix + path, Authorization: auth}
}

// fixtureRecord is the live record the join's fixture actions are held to.
func fixtureRecord() *apilive.Document {
	issue := enforced(directive("default", "project", "read_issue"))
	issue.Abilities = []string{"read_issue"}
	issue = withFields(issue, map[string]string{"author": "UserCore!", "labels": "[Label!]", "notes": "[Note]"})
	deprecatedIssue := assignable("read_issue_old", "Issue: Read (old)", []string{"project"}, "read_issue")
	deprecatedIssue.Deprecated = true
	roleOnly := assignable("read_role_only", "Role: Only", []string{"project"}, "read_role_only")
	roleOnly.AvailableFor = []string{"role"}
	public := &apilive.PublicAnonymous{Source: "unsaved", Project: []string{"read_issue", "not_a_permission"}, Group: []string{"read_project"}}
	callable := &apilive.Callable{Callable: true, Text: "-> { user_project }"}
	return &apilive.Document{
		Source: apilive.Source{Version: "19.4.1-ee"},
		Routes: []apilive.Route{
			route("GET", "/projects/:id/issues", authorized("project", "read_issue")),
			route("GET", "/groups/:id(/-)/epics", &apilive.RouteAuthorization{
				Permissions: []string{"read_epic"},
				Boundaries:  []apilive.Boundary{{BoundaryType: "group"}, {BoundaryType: "project"}},
			}),
			route("POST", "/projects/:id/things", &apilive.RouteAuthorization{
				Permissions: []string{"create_thing"},
				Boundaries:  []apilive.Boundary{{Boundary: callable}},
				AdditionalScopes: []apilive.AdditionalScope{
					{Permissions: []string{"read_user"}, BoundaryType: "user"},
					{Permissions: []string{"read_project"}, Boundary: callable},
					{Permissions: []string{"read_project"}},
				},
			}),
			route("GET", "/projects/:id/skipped", &apilive.RouteAuthorization{Skip: "catch_all"}),
			route("GET", "/projects/:id/later", &apilive.RouteAuthorization{Todo: "pending decision"}),
			route("GET", "/projects/:id/nothing", nil),
			route("GET", "/instance/things", authorized("", "read_user")),
			route("GET", "/projects/:id/unknown", authorized("project", "read_nowhere")),
			route("GET", "/projects/:id/files/:file_path/raw", authorized("project", "read_repository_file")),
			route("PUT", "/projects/:id/integrations/slack", authorized("project", "update_integration")),
			// Another method on the same path is no route the slug
			// declaration stands for, whatever it declares.
			route("DELETE", "/projects/:id/integrations/slack", authorized("group", "update_integration")),
			route("PUT", "/projects/:id/integrations/jira", authorized("project", "update_integration")),
			route("PUT", "/projects/:id/integrations/other", authorized("group", "update_integration")),
			route("PUT", "/projects/:id/hooks", authorized("group", "update_integration")),
			{Method: "GET", Path: "/oauth/token"},
			{Method: "PUT", Path: "/oauth/token"},
		},
		Granular: &apilive.Granular{
			Assignable: []apilive.Assignable{
				assignable("read_issue", "Issue: Read", []string{"project"}, "read_issue"),
				deprecatedIssue,
				roleOnly,
				assignable("read_project", "Project: Read", []string{"project", "group"}, "read_project", "read_project_counts"),
				assignable("read_user", "User: Read", []string{"user"}, "read_user"),
				assignable("read_epic", "Epic: Read", []string{"group", "project"}, "read_epic"),
				assignable("create_thing", "Thing: Create", []string{"project"}, "create_thing"),
				assignable("read_repository_file", "Repository File: Read", []string{"project"}, "read_repository_file"),
				assignable("update_integration", "Integration: Update", []string{"project", "group"}, "update_integration"),
				assignable("read_vulnerability", "Vulnerability: Read", []string{"project"}, "read_vulnerability", "read_dependency"),
				assignable("admin_vulnerability", "Vulnerability: Admin", []string{"project"}, "admin_vulnerability"),
				assignable("create_issue", "Issue: Create", []string{"project"}, "create_issue", "create_label"),
				assignable("read_work_item", "Work Item: Read", []string{"project"}, "read_work_item", "update_work_item"),
			},
			RawToAssignable: map[string]apilive.AssignableMatch{
				"read_issue":     {First: "read_issue_old", FirstAvailable: "read_issue"},
				"read_role_only": {First: "read_role_only"},
				"read_project":   {First: "read_project", FirstAvailable: "read_project"},
				"read_user":      {First: "read_user", FirstAvailable: "read_user"},
			},
			PublicAnonymous: public,
		},
		GraphQLAuthz: &apilive.GraphQLAuthz{
			Types: map[string]apilive.GraphQLType{
				"Query": withFields(apilive.GraphQLType{Enforced: true}, map[string]string{
					"project": "Project", "namespace": "Namespace", "vulnerability": "Vulnerability",
					"workItem": "WorkItem", "users": "UserCoreConnection", "viewer": "UserCore!",
				}),
				"Mutation": withFields(apilive.GraphQLType{Enforced: true}, map[string]string{
					"issueCreate": "IssueCreatePayload", "undeclaredThing": "IssueCreatePayload",
					"skippedThing": "IssueCreatePayload", "labelCreate": "LabelCreatePayload",
					"workItemUpdate": "WorkItemUpdatePayload", "emptyPayload": "EmptyPayload",
					"bareThing": "EmptyPayload",
				}),
				"IssueCreatePayload":    withFields(apilive.GraphQLType{}, map[string]string{"issue": "Issue"}),
				"LabelCreatePayload":    withFields(apilive.GraphQLType{}, map[string]string{"label": "Label"}),
				"WorkItemUpdatePayload": withFields(apilive.GraphQLType{}, map[string]string{"workItem": "WorkItem"}),
				"Project": withFields(enforced(directive("default", "project", "read_project")), map[string]string{
					"issues": "IssueConnection", "branchRules": "BranchRuleConnection", "counts": "Counts!",
				}),
				"Counts":               {},
				"IssueConnection":      withFields(apilive.GraphQLType{}, map[string]string{"nodes": "[Issue]", "edges": "[IssueEdge]", "pageInfo": "PageInfo!"}),
				"IssueEdge":            withFields(apilive.GraphQLType{}, map[string]string{"node": "Issue"}),
				"Issue":                issue,
				"Label":                enforced(),
				"Note":                 enforced(apilive.Directive{SkipReason: "not_a_resource"}),
				"UserCore":             enforced(directive("default", "user", "read_user")),
				"UserCoreConnection":   withFields(apilive.GraphQLType{}, map[string]string{"nodes": "[UserCore]"}),
				"BranchRuleConnection": withFields(apilive.GraphQLType{}, map[string]string{"nodes": "[BranchRule]"}),
				"BranchRule":           enforced(),
				"Namespace":            enforced(),
				"Vulnerability": withFields(enforced(
					directive("primary", "project", "read_vulnerability"),
					directive("primary", "group", "read_dependency"),
					directive("admin", "", "admin_vulnerability"),
				), map[string]string{
					"details": "[VulnerabilityDetail!]!", "issueLinks": "VulnerabilityIssueLinkConnection",
					"location": "VulnerabilityLocation",
				}),
				"VulnerabilityIssueLinkConnection": withFields(apilive.GraphQLType{}, map[string]string{"nodes": "[VulnerabilityIssueLink]"}),
				"VulnerabilityIssueLink":           enforced(),
				"DetailText":                       enforced(),
				"DetailCode":                       enforced(),
				"LocationSast": withFields(enforced(directive("default", "project", "read_vulnerability")), map[string]string{
					"dependency": "Dependency!",
				}),
				"Dependency":   enforced(directive("default", "project", "read_dependency")),
				"LocationDast": enforced(directive("default", "project", "read_vulnerability")),
				"WorkItem":     enforced(directive("default", "project", "read_work_item")),
			},
			Abstract: map[string]apilive.AbstractType{
				"VulnerabilityDetail":   {Kind: "union", PossibleTypes: []string{"DetailText", "DetailCode"}},
				"VulnerabilityLocation": {Kind: "union", PossibleTypes: []string{"LocationSast", "LocationDast"}},
			},
			Mutations: map[string]apilive.Mutation{
				"issueCreate":    {Name: "issueCreate", Granular: []apilive.Directive{directive("default", "project", "create_issue")}},
				"skippedThing":   {Name: "skippedThing", Granular: []apilive.Directive{{SkipReason: "internal"}}},
				"labelCreate":    {Name: "labelCreate", Granular: []apilive.Directive{directive("default", "project", "create_label")}},
				"workItemUpdate": {Name: "workItemUpdate", Granular: []apilive.Directive{directive("default", "project", "update_work_item")}},
				"emptyPayload":   {Name: "emptyPayload", Granular: []apilive.Directive{directive("default", "project", "create_issue")}},
				"bareThing":      {Name: "bareThing"},
			},
			Fields: map[string][]apilive.Directive{
				"Project.counts":          {directive("default", "project", "read_project_counts")},
				"LocationSast.dependency": {directive("default", "project", "admin_vulnerability")},
				"Issue.notes":             {{SkipReason: "not_a_resource"}},
			},
		},
	}
}

// fixtureSchema loads the fixture SDL.
func fixtureSchema(t *testing.T) *gqlast.Schema {
	t.Helper()
	schema, err := graphqlschema.Load([]byte(fixtureSDL))
	if err != nil {
		t.Fatalf("load the fixture schema: %v", err)
	}
	return schema
}

// rest is a derived REST request.
func rest(method, path string) derive.Use {
	return derive.Use{Kind: derive.KindREST, Method: method, Path: path}
}

// graphQL is a derived GraphQL request.
func graphQL(document string) derive.Use {
	return derive.Use{Kind: derive.KindGraphQL, Document: document, Sites: []string{"fixture.Handler"}}
}

// action is a derived action with one path holding every request.
func action(id string, uses ...derive.Use) derive.Action {
	path := make([]int, len(uses))
	for i := range path {
		path[i] = i
	}
	return derive.Action{ID: id, Uses: uses, Paths: [][]int{path}}
}

// render words one joined action the way the golden expectations read it.
func render(table *finegrained.Table, act *Action) string {
	if act.Row == nil {
		return "no row\n"
	}
	var b strings.Builder
	if act.Row.Denied != nil {
		fmt.Fprintf(&b, "denied %s %s %s\n", act.Row.Denied.Cause, act.Row.Denied.Element, act.Row.Denied.Effect)
	}
	for _, way := range act.Row.DeniedWays {
		fmt.Fprintf(&b, "denied way %s %s %s\n", way.Cause, way.Element, way.Effect)
	}
	for _, path := range act.Row.Paths {
		b.WriteString("path\n")
		for _, op := range path {
			operation := &table.Operations[op]
			fmt.Fprintf(&b, "  %s%s\n", operation.Name, map[bool]string{true: " [skip]"}[operation.Skip])
			for _, group := range operation.Groups {
				fmt.Fprintf(&b, "    group %s\n", groupText(table, group))
			}
			for _, element := range operation.Spine {
				fmt.Fprintf(&b, "    spine %s\n", elementText(table, element))
			}
			for _, element := range operation.OffSpine {
				fmt.Fprintf(&b, "    off %s\n", elementText(table, element))
			}
		}
	}
	for _, element := range act.Row.Degraded {
		fmt.Fprintf(&b, "degraded %s\n", elementText(table, element))
	}
	if act.Row.GraphQL {
		b.WriteString("graphql")
		if act.Row.Collection {
			b.WriteString(" collection")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// groupText words one group by its raw permissions.
func groupText(table *finegrained.Table, index uint32) string {
	group := &table.Groups[index]
	perms := make([]string, len(group.Perms))
	for i, perm := range group.Perms {
		perms[i] = table.Permissions[perm]
	}
	return strings.Join(perms, ",") + " @ " + group.Any.String()
}

// elementText words one element.
func elementText(table *finegrained.Table, index uint32) string {
	element := &table.Elements[index]
	var text strings.Builder
	fmt.Fprintf(&text, "%s %s %s", element.Path, element.Type, element.Effect)
	if element.Undeclared {
		text.WriteString(" undeclared")
	}
	if len(element.Members) > 0 {
		text.WriteString(" members " + strings.Join(element.Members, ","))
	}
	for _, group := range element.Groups {
		text.WriteString(" [" + groupText(table, group) + "]")
	}
	return text.String()
}

// joined finds one joined action.
func joined(t *testing.T, result Result, id string) *Action {
	t.Helper()
	for i := range result.Actions {
		if result.Actions[i].ID == id {
			return &result.Actions[i]
		}
	}
	t.Fatalf("the join holds no %s", id)
	return nil
}

// fixtureActions are the derived actions the golden expectations are held to.
func fixtureActions() []derive.Action {
	return []derive.Action{
		action("rest.issues", rest("GET", "/projects/:/issues")),
		action("rest.epics_optional", rest("GET", "/groups/:/-/epics")),
		action("rest.epics_plain", rest("GET", "/groups/:/epics")),
		action("rest.things", rest("POST", "/projects/:/things")),
		action("rest.skipped", rest("GET", "/projects/:/skipped")),
		action("rest.later", rest("GET", "/projects/:/later")),
		action("rest.nothing", rest("GET", "/projects/:/nothing")),
		action("rest.unknown", rest("GET", "/projects/:/unknown")),
		action("rest.head", rest("HEAD", "/projects/:/files/:/raw")),
		action("rest.slug", rest("PUT", "/projects/:/integrations/:")),
		action("rest.missing", rest("GET", "/projects/:/missing")),
		action("rest.twice", rest("GET", "/projects/:/issues"), rest("GET", "/projects/:/issues")),
		{ID: "rest.alternatives", Uses: []derive.Use{rest("GET", "/projects/:/later"), rest("GET", "/projects/:/issues")}, Paths: [][]int{{0}, {1}}},
		{ID: "rest.denied_twice", Uses: []derive.Use{
			rest("GET", "/projects/:/later"), rest("GET", "/projects/:/issues"), rest("GET", "/projects/:/later"), rest("GET", "/projects/:/nothing"),
		}, Paths: [][]int{{0}, {1}, {2}, {3}}},
		{ID: "mixed.denied_graphql_way", Uses: []derive.Use{
			graphQL(`query { namespace(fullPath: "a") { id name } }`), rest("GET", "/groups/:/epics"),
		}, Paths: [][]int{{0}, {1}}},
		{ID: "mixed.denied_list_way", Uses: []derive.Use{
			rest("GET", "/projects/:/issues"), graphQL(`query { project(fullPath: "a") { branchRules { nodes { name } } } }`),
		}, Paths: [][]int{{0}, {1}}},
		{ID: "rest.declared_nothing", Declaration: "sends-nothing", Paths: [][]int{{}}},
		action("graphql.issues", graphQL(`query { project(fullPath: "a") { issues { nodes { id author { username } notes { body } } pageInfo { hasNextPage } count } } }`)),
		action("graphql.edges", graphQL(`query { project(fullPath: "a") { issues { edges { node { id } cursor } } } }`)),
		action("graphql.branch_rules", graphQL(`query { project(fullPath: "a") { branchRules { nodes { name } } } }`)),
		action("graphql.vulnerability", graphQL(`query { vulnerability(id: "1") { id issueLinks { nodes { id } } location { ... on LocationSast { file } } } }`)),
		action("graphql.member_field", graphQL(`query { vulnerability(id: "1") { id location { ... on LocationSast { dependency { name } } } } }`)),
		action("graphql.details", graphQL(`query { vulnerability(id: "1") { id details { ... on DetailText { text } } } }`)),
		action("graphql.namespace", graphQL(`query { namespace(fullPath: "a") { id name } }`)),
		action("graphql.counts", graphQL(`query { project(fullPath: "a") { counts { total } } }`)),
		action("graphql.users", graphQL(`query { users { nodes { username } } }`)),
		action("graphql.fragment", graphQL(`query { users { nodes { ...user } } } fragment user on UserCore { username }`)),
		action("graphql.plain", graphQL(`query { plain { name } }`)),
		action("graphql.plain_items", graphQL(`query { version plain { items { name } } }`)),
		action("graphql.viewer", graphQL(`query { viewer { username } }`)),
		action("graphql.labels", graphQL(`query { project(fullPath: "a") { name issues { nodes { id labels { title } } } } }`)),
		action("graphql.list_mid_spine", graphQL(`query { project(fullPath: "a") { issues { nodes { author { username } } } } }`)),
		action("graphql.list_root_first", graphQL(`query { users { nodes { username } } viewer { username } }`)),
		action("graphql.sdk_named", derive.Use{Kind: derive.KindGraphQL, Document: `query { plain { name items { name } } }`, SDKMethods: []string{"Things.Get"}}),
		action("graphql.document_named", derive.Use{
			Kind: derive.KindGraphQL, Document: `query { plain { items { name } name } }`, Name: "plainQuery",
			SDKMethods: []string{"Things.Get"}, Sites: []string{"fixture.Handler"},
		}),
		action("graphql.unplaced", derive.Use{Kind: derive.KindGraphQL, Document: `query { plain { node { name } } }`}),
		action("graphql.scalar_root", graphQL(`query { version }`)),
		action("graphql.partly_skipped", graphQL(`mutation { skippedThing(input: {title: "t"}) { errors } issueCreate(input: {title: "t"}) { errors issue { id } } }`)),
		action("graphql.bare", graphQL(`mutation { bareThing(input: {title: "t"}) { errors } }`)),
		action("graphql.two_undeclared", graphQL(`mutation { undeclaredThing(input: {title: "t"}) { errors } bareThing(input: {title: "t"}) { errors } }`)),
		action("graphql.undeclared_beside_payload", graphQL(`mutation { undeclaredThing(input: {title: "t"}) { errors } labelCreate(input: {title: "t"}) { errors label { title } } }`)),
		action("mixed.list_then_rest", graphQL(`query { users { nodes { username } } }`), rest("GET", "/projects/:/issues")),
		{ID: "rest.no_way"},
		{ID: "rest.either", Uses: []derive.Use{rest("GET", "/projects/:/issues"), rest("GET", "/groups/:/epics")}, Paths: [][]int{{0}, {1}}},
		{ID: "rest.superset", Uses: []derive.Use{rest("GET", "/projects/:/issues"), rest("GET", "/groups/:/epics")}, Paths: [][]int{{0}, {0, 1}}},
		action("rest.instance", rest("GET", "/instance/things")),
		action("graphql.issue_create", graphQL(`mutation { issueCreate(input: {title: "t"}) { errors issue { id } } }`)),
		action("graphql.undeclared", graphQL(`mutation { undeclaredThing(input: {title: "t"}) { errors } }`)),
		action("graphql.skipped", graphQL(`mutation { skippedThing(input: {title: "t"}) { errors } }`)),
		action("graphql.label_create", graphQL(`mutation { labelCreate(input: {title: "t"}) { errors label { title } } }`)),
		action("graphql.work_item_update", graphQL(`mutation { workItemUpdate(input: {title: "t"}) { errors workItem { id } } }`)),
		action("graphql.work_item_refused", graphQL(`mutation { workItemUpdate(input: {title: "t"}) { errors workItem { id } } }`)),
		action("graphql.project_work_item_update", graphQL(`mutation { workItemUpdate(input: {title: "t"}) { errors workItem { id } } }`)),
		action("graphql.work_item_read", graphQL(`query { workItem(id: "1") { id title } }`)),
		action("graphql.vulnerability_rule", graphQL(`query { vulnerability(id: "1") { id issueLinks { nodes { id } } location { ... on LocationSast { file } } } }`)),
		action("graphql.links_fatal", graphQL(`query { vulnerability(id: "1") { id issueLinks { nodes { id } } location { ... on LocationSast { file } } } }`)),
		action("graphql.branch_rules_lenient", graphQL(`query { project(fullPath: "a") { branchRules { nodes { name } } } }`)),
		action("graphql.lookup_then_write",
			graphQL(`query { namespace(fullPath: "a") { id name } }`),
			graphQL(`mutation { labelCreate(input: {title: "t"}) { errors label { title } } }`)),
		action("graphql.write_then_lookup",
			graphQL(`mutation { labelCreate(input: {title: "t"}) { errors label { title } } }`),
			graphQL(`query { namespace(fullPath: "a") { id name } }`)),
		action("graphql.empty_payload", graphQL(`mutation { emptyPayload(input: {title: "t"}) { errors } }`)),
		action("graphql.invalid", graphQL(`query { project(fullPath: "a") { nope } }`)),
		action("graphql.two", graphQL(`query one { plain { name } } query two { plain { name } }`)),
		action("graphql.again", graphQL(`query { project(fullPath: "a") { issues { nodes { id author { username } notes { body } } pageInfo { hasNextPage } count } } }`)),
		{ID: "unresolved.raw", Uses: []derive.Use{{Kind: derive.KindUnresolved, Reason: "raw-path x.Y"}}, Paths: [][]int{{0}}},
		{ID: "derive.finding", Findings: []string{"derive.finding expands into more than 256 paths"}},
	}
}

// fixtureDeclarations are the declarations the fixture actions use and two
// that answer nothing.
func fixtureDeclarations() Declarations {
	return Declarations{
		Routes: []RouteDeclaration{
			{Route: "HEAD /projects/:/files/:/raw", Category: "head-inherits-get", Use: "GET /projects/:id/files/:file_path/raw"},
			{Route: "PUT /projects/:/integrations/:", Category: "slug-from-input", Use: "PUT /projects/:id/integrations/slack"},
			{Route: "GET /projects/:/missing", Category: "head-inherits-get", Use: "GET /projects/:id/gone"},
			{Route: "GET /projects/:/never", Category: "head-inherits-get", Use: "GET /projects/:id/issues"},
		},
		Effects: []EffectDeclaration{
			{Action: "graphql.vulnerability", Path: "vulnerability.location", Category: "spine-override", Fatal: true},
			{Action: "graphql.links_fatal", Path: "vulnerability.issueLinks.nodes", Category: "spine-override", Fatal: true},
			{Action: "graphql.branch_rules_lenient", Path: "project.branchRules.nodes", Category: "spine-override"},
			{Action: "graphql.issues", Path: "project", Category: "spine-override", Fatal: true},
		},
		Unresolvable: []BoundaryDeclaration{
			{Action: "graphql.work_item_update", Path: "workItemUpdate.workItem", Category: "group-work-item"},
			{Action: "graphql.work_item_refused", Path: "workItemUpdate", Category: "group-work-item"},
			{Action: "graphql.work_item_read", Path: "workItem", Category: "group-work-item"},
			{Action: "graphql.namespace", Path: "nowhere", Category: "group-work-item"},
		},
	}
}

// TestJoin_Fixture_PlacesEachRequest holds the join to GitLab's rules, one
// fixture action per rule: a REST route matched with optional segments taken
// and left out, its primary and additional requirements, a skip, a todo and a
// route declaring nothing; a GraphQL answer spine through connections and
// edges, a declared element a non-null chain carries onto it (Issue.author),
// an undeclared one off it served empty, an abstract position judged as its
// worst member (Vulnerability.details), a field selected in a fragment on a
// union member read on the member (its record signature and its field-level
// declaration), a redacted connection, and the mutations a fine-grained token
// is refused or whose answer it loses; a group work item, declared at a
// boundary it never resolves to, read and written; an action one of whose
// ways no token passes, kept as a denied way beside the ways that run, each
// denial once, with the GraphQL and collection flags read from every way, and
// the collection flag from a list anywhere on any root's spine, one that does
// not end the spine and one under a root before the last included; a
// document named by its own name, its client-go method or its site, in that
// order, and by none; a field named like a connection's items that is no
// connection's; a document whose roots are all scalars, and mutations whose
// roots disagree, which are left to GitLab only when every root opts out and
// report the first refusal of several; and an action with no way at all.
func TestJoin_Fixture_PlacesEachRequest(t *testing.T) {
	result := Join(fixtureRecord(), fixtureSchema(t), fixtureActions(), fixtureDeclarations())
	issuesQuery := "path\n  query project (fixture.Handler)\n" +
		"    spine project Project null [read_project @ project]\n" +
		"    spine project.issues.nodes Issue removed-items [read_issue @ project]\n" +
		"    spine project.issues.nodes.author UserCore null [read_user @ user]\n" +
		"graphql collection\n"
	vulnerability := func(location string) string {
		return "path\n  query vulnerability (fixture.Handler)\n" +
			"    spine vulnerability Vulnerability null [read_vulnerability @ project or group] [admin_vulnerability @ project or group or user or instance]\n" +
			"    " + location + " vulnerability.location VulnerabilityLocation null members LocationSast,LocationDast [read_vulnerability @ project]\n" +
			"degraded vulnerability.issueLinks.nodes VulnerabilityIssueLink null undeclared\ngraphql\n"
	}
	issuesRoute := "path\n  GET /projects/:id/issues\n    group read_issue @ project\n"
	want := map[string]string{
		"rest.issues":         issuesRoute,
		"rest.epics_optional": "path\n  GET /groups/:id(/-)/epics\n    group read_epic @ project or group\n",
		"rest.epics_plain":    "path\n  GET /groups/:id(/-)/epics\n    group read_epic @ project or group\n",
		"rest.things":         "path\n  POST /projects/:id/things\n    group create_thing @ project or group or user or instance\n    group read_user @ user\n    group read_project @ project or group or user or instance\n",
		"rest.skipped":        "path\n  GET /projects/:id/skipped [skip]\n",
		"rest.later":          "denied rest-todo GET /projects/:id/later refused\n",
		"rest.nothing":        "denied rest-undeclared GET /projects/:id/nothing refused\n",
		"rest.unknown":        "path\n  GET /projects/:id/unknown\n    group  @ project\n",
		"rest.head":           "path\n  GET /projects/:id/files/:file_path/raw\n    group read_repository_file @ project\n",
		"rest.slug":           "path\n  PUT /projects/:id/integrations/slack\n    group update_integration @ project\n",
		"rest.missing":        "no row\n",
		"rest.twice":          issuesRoute,
		"rest.alternatives":   "denied way rest-todo GET /projects/:id/later refused\n" + issuesRoute,
		"rest.denied_twice": "denied way rest-todo GET /projects/:id/later refused\n" +
			"denied way rest-undeclared GET /projects/:id/nothing refused\n" + issuesRoute,
		"mixed.denied_graphql_way": "denied way graphql-type-undeclared Namespace null\n" +
			"path\n  GET /groups/:id(/-)/epics\n    group read_epic @ project or group\ngraphql\n",
		"mixed.denied_list_way": "denied way graphql-type-undeclared BranchRule null\n" + issuesRoute + "graphql collection\n",
		"rest.declared_nothing": "path\n",
		"graphql.issues":        issuesQuery,
		"graphql.again":         issuesQuery,
		"graphql.edges": "path\n  query project (fixture.Handler)\n" +
			"    spine project Project null [read_project @ project]\n" +
			"    spine project.issues.edges.node Issue removed-items [read_issue @ project]\ngraphql collection\n",
		"graphql.branch_rules":       "denied graphql-type-undeclared BranchRule null\ngraphql collection\n",
		"graphql.vulnerability":      vulnerability("spine"),
		"graphql.vulnerability_rule": vulnerability("off"),
		"graphql.member_field": "path\n  query vulnerability (fixture.Handler)\n" +
			"    spine vulnerability Vulnerability null [read_vulnerability @ project or group] [admin_vulnerability @ project or group or user or instance]\n" +
			"    off vulnerability.location VulnerabilityLocation null members LocationSast,LocationDast [read_vulnerability @ project]\n" +
			"    off vulnerability.location.dependency Dependency null [read_dependency @ project] [admin_vulnerability @ project]\ngraphql\n",
		"graphql.details":   "denied graphql-type-undeclared VulnerabilityDetail null\ngraphql\n",
		"graphql.namespace": "denied graphql-type-undeclared Namespace null\ngraphql\n",
		"graphql.counts": "path\n  query project (fixture.Handler)\n" +
			"    spine project Project null [read_project @ project]\n" +
			"    spine project.counts Counts null [read_project_counts @ project]\ngraphql\n",
		"graphql.users":          "path\n  query users (fixture.Handler)\n    spine users.nodes UserCore removed-items [read_user @ user]\ngraphql collection\n",
		"graphql.fragment":       "path\n  query users (fixture.Handler)\n    spine users.nodes UserCore removed-items [read_user @ user]\ngraphql collection\n",
		"graphql.plain":          "path\n  query plain (fixture.Handler)\ngraphql\n",
		"graphql.plain_items":    "path\n  query plain (fixture.Handler)\ngraphql collection\n",
		"graphql.sdk_named":      "path\n  query plain (Things.Get)\ngraphql\n",
		"graphql.document_named": "path\n  query plain (plainQuery)\ngraphql\n",
		"graphql.unplaced":       "path\n  query plain\ngraphql\n",
		"graphql.scalar_root":    "path\n  query  (fixture.Handler)\ngraphql\n",
		"graphql.partly_skipped": "path\n  mutation skippedThing issueCreate (fixture.Handler)\n    group create_issue @ project\n" +
			"    spine issueCreate.issue Issue null [read_issue @ project]\ngraphql\n",
		"graphql.bare":                      "denied graphql-mutation-undeclared bareThing refused\ngraphql\n",
		"graphql.two_undeclared":            "denied graphql-mutation-undeclared undeclaredThing refused\ngraphql\n",
		"graphql.undeclared_beside_payload": "denied graphql-mutation-undeclared undeclaredThing refused\ngraphql\n",
		"mixed.list_then_rest": issuesRoute +
			"  query users (fixture.Handler)\n    spine users.nodes UserCore removed-items [read_user @ user]\ngraphql collection\n",
		"rest.no_way":    "",
		"graphql.viewer": "path\n  query viewer (fixture.Handler)\n    spine viewer UserCore null [read_user @ user]\ngraphql\n",
		"graphql.labels": "path\n  query project (fixture.Handler)\n" +
			"    spine project Project null [read_project @ project]\n" +
			"    off project.issues.nodes Issue removed-items [read_issue @ project]\n" +
			"degraded project.issues.nodes.labels Label list-null undeclared\ngraphql\n",
		"graphql.list_mid_spine": issuesQuery,
		"graphql.list_root_first": "path\n  query users viewer (fixture.Handler)\n" +
			"    spine users.nodes UserCore removed-items [read_user @ user]\n" +
			"    spine viewer UserCore null [read_user @ user]\ngraphql collection\n",
		"rest.either":   issuesRoute + "path\n  GET /groups/:id(/-)/epics\n    group read_epic @ project or group\n",
		"rest.superset": issuesRoute,
		"rest.instance": "path\n  GET /instance/things\n    group read_user @ project or group or user or instance\n",
		"graphql.issue_create": "path\n  mutation issueCreate (fixture.Handler)\n    group create_issue @ project\n" +
			"    spine issueCreate.issue Issue null [read_issue @ project]\ngraphql\n",
		"graphql.undeclared":               "denied graphql-mutation-undeclared undeclaredThing refused\ngraphql\n",
		"graphql.skipped":                  "path\n  mutation skippedThing (fixture.Handler) [skip]\ngraphql\n",
		"graphql.label_create":             "denied graphql-payload-undeclared Label committed-then-null\ngraphql\n",
		"graphql.work_item_update":         "denied graphql-boundary-unresolvable WorkItem committed-then-null\ngraphql\n",
		"graphql.work_item_refused":        "denied graphql-boundary-unresolvable workItemUpdate refused\ngraphql\n",
		"graphql.work_item_read":           "denied graphql-boundary-unresolvable WorkItem null-or-empty\ngraphql\n",
		"graphql.project_work_item_update": "path\n  mutation workItemUpdate (fixture.Handler)\n    group update_work_item @ project\n    spine workItemUpdate.workItem WorkItem null [read_work_item @ project]\ngraphql\n",
		"graphql.links_fatal":              "denied graphql-type-undeclared VulnerabilityIssueLink null\ngraphql\n",
		"graphql.branch_rules_lenient":     "path\n  query project (fixture.Handler)\n    spine project Project null [read_project @ project]\ndegraded project.branchRules.nodes BranchRule null undeclared\ngraphql collection\n",
		"graphql.lookup_then_write":        "denied graphql-type-undeclared Namespace null\ngraphql\n",
		"graphql.write_then_lookup":        "denied graphql-payload-undeclared Label committed-then-null\ngraphql\n",
		"graphql.empty_payload":            "path\n  mutation emptyPayload (fixture.Handler)\n    group create_issue @ project\ngraphql\n",
		"graphql.invalid":                  "no row\n",
		"graphql.two":                      "no row\n",
		"unresolved.raw":                   "no row\n",
		"derive.finding":                   "no row\n",
	}
	for id, expected := range want {
		t.Run(id, func(t *testing.T) {
			if got := render(&result.Table, joined(t, result, id)); got != expected {
				t.Errorf("%s joins to\n%s\nwant\n%s", id, got, expected)
			}
		})
	}
	if len(want) != len(result.Actions) {
		t.Errorf("the fixture joins %d actions and %d are expected", len(result.Actions), len(want))
	}
	wantFindings := []string{
		"GET /projects/:id/unknown demands read_nowhere, which no assignable permission expands to",
		"PUT /projects/:/integrations/: stands for PUT /projects/:id/integrations/other, which declares another authorization than PUT /projects/:id/integrations/slack",
		"derive.finding expands into more than 256 paths",
		`graphql.invalid: document fixture.Handler does not validate against the pinned schema: Cannot query field "nope" on type "Project". Did you mean "name"?`,
		"graphql.two: document fixture.Handler holds 2 operations; one is what a request sends",
		"rest.missing: GET /projects/:/missing is no route of the live record; declare the route whose authorization it carries",
		"the group-work-item declaration of graphql.namespace at nowhere answers nothing: no position or mutation of the action is there",
		"the head-inherits-get declaration of GET /projects/:/missing names GET /projects/:id/gone, which is no route of the live record",
		"the head-inherits-get declaration of GET /projects/:/never answers nothing: no action sends it, or the record carries it",
		"the spine-override declaration of graphql.issues at project answers nothing: the spine rule already agrees, or no position is there",
		"unresolved.raw: unresolved request raw-path x.Y; declare what it sends",
	}
	if !slices.Equal(result.Findings, wantFindings) {
		t.Errorf("findings =\n%s\nwant\n%s", strings.Join(result.Findings, "\n"), strings.Join(wantFindings, "\n"))
	}
	// The record describes neither the plain root field nor Plain, so their
	// signatures are the pinned schema's: one document reaches plain alone,
	// three reach plain and its items and one plain and its node. The
	// dependency a union member selects is read on the member, which the
	// record describes, so it adds none.
	if result.Fallbacks != 9 {
		t.Errorf("fallbacks = %d, want 9", result.Fallbacks)
	}
	// The table is held in ID order, which is what Requirement's binary
	// search reads it by, so every row is found under its own ID.
	for i := range result.Actions {
		act := &result.Actions[i]
		if got := result.Table.Requirement(act.ID); (got == nil) != (act.Row == nil) || got != nil && got.ID != act.ID {
			t.Errorf("Requirement(%q) = %+v, want the row the join wrote for it", act.ID, got)
		}
	}
}

// TestMinimizeOps_KeepsTheShortestWaysInOrder verifies a way holding another
// is dropped and a repeated one kept once, and that what is kept is ordered
// shortest first, then by its operations.
func TestMinimizeOps_KeepsTheShortestWaysInOrder(t *testing.T) {
	got := minimizeOps([][]uint32{{0, 1}, {2}, {0}, {0, 1}, {1, 3}})
	want := [][]uint32{{0}, {2}, {1, 3}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("minimizeOps = %v, want %v", got, want)
	}
}

// TestJoin_Fixture_SharesAnOperationUntilADeclarationDeparts verifies every
// action sending one document shares one operation in the table, and an
// action a declaration departs for is given its own: the vulnerability read
// whose location is declared fatal does not move the location onto the spine
// of the action that keeps the rule.
func TestJoin_Fixture_SharesAnOperationUntilADeclarationDeparts(t *testing.T) {
	result := Join(fixtureRecord(), fixtureSchema(t), fixtureActions(), fixtureDeclarations())
	operation := func(id string) int { return joined(t, result, id).Requests[0].Operation }
	if operation("graphql.issues") != operation("graphql.again") {
		t.Errorf("two actions sending one document hold operations %d and %d, want one", operation("graphql.issues"), operation("graphql.again"))
	}
	if operation("graphql.vulnerability") == operation("graphql.vulnerability_rule") {
		t.Errorf("the declared vulnerability read shares operation %d with the one the rule decides", operation("graphql.vulnerability"))
	}
	rule := joined(t, result, "graphql.vulnerability_rule").Requests[0]
	if rule.Name != "query vulnerability (fixture.Handler)" || !slices.Equal(rule.RootFields, []string{"vulnerability"}) {
		t.Errorf("the request is named %q with root fields %q", rule.Name, rule.RootFields)
	}
	wantPositions := []string{"vulnerability", "vulnerability.issueLinks", "vulnerability.issueLinks.nodes", "vulnerability.location"}
	if !slices.Equal(rule.Positions, wantPositions) {
		t.Errorf("positions = %q, want %q", rule.Positions, wantPositions)
	}
}

// TestJoin_Fixture_ReadsTheVocabulary verifies the table carries the record's
// vocabulary: every raw permission an assignable expands to, sorted, with the
// words a grant names it by (the first assignable a token can be granted,
// never a deprecated one that comes before it), each assignable's boundaries
// and whether a token can be granted it, and the anonymous policy as bits
// over the permissions.
func TestJoin_Fixture_ReadsTheVocabulary(t *testing.T) {
	table := Join(fixtureRecord(), fixtureSchema(t), nil, Declarations{}).Table
	if table.Version != "19.4.1-ee" || table.Bucket == "" {
		t.Errorf("version %q bucket %q", table.Version, table.Bucket)
	}
	index := slices.Index(table.Permissions, "read_issue")
	if index < 0 || !slices.IsSorted(table.Permissions) {
		t.Fatalf("permissions %q are not sorted or lack read_issue", table.Permissions)
	}
	if table.Display[index] != "Issue: Read" {
		t.Errorf("read_issue is granted as %q, want Issue: Read rather than its deprecated first match", table.Display[index])
	}
	roleOnly := slices.IndexFunc(table.Assignables, func(a finegrained.Assignable) bool { return a.Name == "read_role_only" })
	if roleOnly < 0 || table.Assignables[roleOnly].Grantable {
		t.Errorf("an assignable available only to a role reads as grantable")
	}
	project := slices.IndexFunc(table.Assignables, func(a finegrained.Assignable) bool { return a.Name == "read_project" })
	if got := table.Assignables[project].Boundaries; got != finegrained.BoundaryProject|finegrained.BoundaryGroup {
		t.Errorf("read_project boundaries = %s, want project or group", got)
	}
	if !table.PublicKnown {
		t.Fatal("the anonymous policy the record carries is not known")
	}
	// Sixteen permissions fit one word, so each set is exactly one long.
	if len(table.Permissions) != 16 || len(table.PublicAnonymous[finegrained.PublicProject]) != 1 || len(table.PublicAnonymous[finegrained.PublicGroup]) != 1 {
		t.Errorf("%d permissions in sets of %d and %d words, want 16 in one word each",
			len(table.Permissions), len(table.PublicAnonymous[finegrained.PublicProject]), len(table.PublicAnonymous[finegrained.PublicGroup]))
	}
	bit := func(set []uint64, name string) bool {
		i := slices.Index(table.Permissions, name)
		return set[i/64]&(uint64(1)<<(i%64)) != 0
	}
	if !bit(table.PublicAnonymous[finegrained.PublicProject], "read_issue") || bit(table.PublicAnonymous[finegrained.PublicProject], "read_project") {
		t.Errorf("the public project policy is read wrong")
	}
	if !bit(table.PublicAnonymous[finegrained.PublicGroup], "read_project") {
		t.Errorf("the public group policy is read wrong")
	}
}

// TestJoin_NoAnonymousPolicy_IsUnknown verifies a record that carries no
// anonymous policy leaves it unknown rather than empty, which would read as
// "anonymous may do nothing".
func TestJoin_NoAnonymousPolicy_IsUnknown(t *testing.T) {
	record := fixtureRecord()
	record.Granular.PublicAnonymous = nil
	if table := Join(record, fixtureSchema(t), nil, Declarations{}).Table; table.PublicKnown {
		t.Error("a record with no anonymous policy reads as one that knows it")
	}
}
