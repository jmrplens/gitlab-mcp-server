// branch_rules_test.go contains unit tests for GitLab branch rule operations.
// Tests use httptest to mock the GitLab Branch Rules API.
package branchrules

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// janeJSON and deployBotJSON are the two user references the fixtures name,
// spelled once so a grant, a deploy key and an approval rule read the same
// user back.
const (
	janeJSON = `{"id": "21", "username": "jane", "name": "Jane Doe", "publicEmail": "jane@example.com",
		"avatarUrl": "https://gitlab.example.com/uploads/jane.png", "webUrl": "https://gitlab.example.com/jane", "webPath": "/jane"}`
	deployBotJSON = `{"id": "22", "username": "deploy-bot", "name": "Deploy Bot", "publicEmail": null,
		"avatarUrl": null, "webUrl": "https://gitlab.example.com/deploy-bot", "webPath": "/deploy-bot"}`
)

// sampleBranchRuleNode is a branch rule as the Enterprise document answers it,
// with every kind of grant, every flag and both of the lists it carries.
const sampleBranchRuleNode = `{
	"id": "gid://gitlab/Projects::BranchRule/7",
	"name": "main",
	"isDefault": true,
	"isProtected": true,
	"isGroupLevel": false,
	"matchingBranchesCount": 1,
	"createdAt": "2026-01-15T10:00:00Z",
	"updatedAt": "2026-06-20T14:30:00Z",
	"squashOption": {"option": "Encourage", "helpText": "Checkbox is visible and selected by default."},
	"branchProtection": {
		"allowForcePush": false,
		"codeOwnerApprovalRequired": true,
		"isGroupLevel": true,
		"modificationBlockedByPolicy": true,
		"protectedFromPushBySecurityPolicy": false,
		"warnModificationBlockedByPolicy": true,
		"warnProtectedFromPushBySecurityPolicy": false,
		"pushAccessLevels": {"nodes": [
			{"accessLevel": 40, "accessLevelDescription": "Maintainers", "user": null, "group": null, "memberRole": null, "deployKey": null},
			{"accessLevel": 40, "accessLevelDescription": "Jane Doe", "user": ` + janeJSON + `, "group": null, "memberRole": null, "deployKey": null},
			{"accessLevel": 40, "accessLevelDescription": "Deploy key", "user": null, "group": null, "memberRole": null,
				"deployKey": {"id": "12", "title": "Release key", "expiresAt": "2027-01-01", "user": ` + deployBotJSON + `}}
		]},
		"mergeAccessLevels": {"nodes": [
			{"accessLevel": 30, "accessLevelDescription": "Developers + Maintainers", "user": null, "group": null, "memberRole": null},
			{"accessLevel": 30, "accessLevelDescription": "Platform", "user": null, "memberRole": null,
				"group": {"id": "9", "name": "Platform", "webUrl": "https://gitlab.example.com/groups/acme/platform", "avatarUrl": null,
					"parent": {"id": "3", "name": "Acme", "webUrl": "https://gitlab.example.com/groups/acme", "avatarUrl": "https://gitlab.example.com/uploads/acme.png"}}}
		]},
		"unprotectAccessLevels": {"nodes": [
			{"accessLevel": 40, "accessLevelDescription": "Release managers", "user": null, "group": null,
				"memberRole": {"id": "gid://gitlab/MemberRole/5", "name": "Release managers"}},
			{"accessLevel": 40, "accessLevelDescription": "Acme", "user": null, "memberRole": null,
				"group": {"id": "3", "name": "Acme", "webUrl": "https://gitlab.example.com/groups/acme", "avatarUrl": null, "parent": null}}
		]}
	},
	"approvalRules": {
		"nodes": [
			{"id": "gid://gitlab/ApprovalProjectRule/1", "name": "Security Review", "approvalsRequired": 2, "type": "REGULAR",
				"coverageMinimumThreshold": null, "eligibleApprovers": {"nodes": [` + janeJSON + `]}},
			{"id": "gid://gitlab/ApprovalProjectRule/2", "name": "Coverage-Check", "approvalsRequired": 1, "type": "REPORT_APPROVER",
				"coverageMinimumThreshold": 80.5, "eligibleApprovers": {"nodes": []}}
		]
	},
	"externalStatusChecks": {
		"nodes": [
			{"id": "gid://gitlab/MergeRequests::ExternalStatusCheck/3", "name": "SonarQube", "externalUrl": "https://sonar.example.com/check", "hmac": true}
		]
	}
}`

// sampleUnprotectedRuleNode is an Enterprise rule that protects nothing.
const sampleUnprotectedRuleNode = `{
	"id": null,
	"name": "feature/*",
	"isDefault": false,
	"isProtected": false,
	"isGroupLevel": false,
	"matchingBranchesCount": 5,
	"createdAt": "2026-03-01T08:00:00Z",
	"updatedAt": null,
	"squashOption": null,
	"branchProtection": null,
	"approvalRules": {"nodes": []},
	"externalStatusChecks": {"nodes": []}
}`

// jane and deployBot are what janeJSON and deployBotJSON decode to.
var (
	jane = UserRef{
		ID: "21", Username: "jane", Name: "Jane Doe", PublicEmail: "jane@example.com",
		AvatarURL: "https://gitlab.example.com/uploads/jane.png", WebURL: "https://gitlab.example.com/jane", WebPath: "/jane",
	}
	deployBot = UserRef{ID: "22", Username: "deploy-bot", Name: "Deploy Bot", WebURL: "https://gitlab.example.com/deploy-bot", WebPath: "/deploy-bot"}
)

// wantProtectedRule is sampleBranchRuleNode as the output publishes it.
func wantProtectedRule() BranchRuleItem {
	threshold := 80.5
	return BranchRuleItem{
		ID:                    "gid://gitlab/Projects::BranchRule/7",
		Name:                  "main",
		IsDefault:             true,
		IsProtected:           true,
		IsGroupLevel:          new(false),
		MatchingBranchesCount: 1,
		CreatedAt:             "2026-01-15T10:00:00Z",
		UpdatedAt:             "2026-06-20T14:30:00Z",
		SquashOption:          &SquashOption{Option: "Encourage", HelpText: "Checkbox is visible and selected by default."},
		BranchProtection: &BranchProtection{
			AllowForcePush:                        false,
			CodeOwnerApprovalRequired:             new(true),
			IsGroupLevel:                          new(true),
			ModificationBlockedByPolicy:           new(true),
			ProtectedFromPushBySecurityPolicy:     new(false),
			WarnModificationBlockedByPolicy:       new(true),
			WarnProtectedFromPushBySecurityPolicy: new(false),
			PushAccessLevels: []PushAccess{
				{AccessLevel: 40, AccessLevelDescription: "Maintainers"},
				{AccessLevel: 40, AccessLevelDescription: "Jane Doe", User: &jane},
				{
					AccessLevel: 40, AccessLevelDescription: "Deploy key",
					DeployKey: &AccessDeployKey{ID: "12", Title: "Release key", ExpiresAt: "2027-01-01", User: deployBot},
				},
			},
			MergeAccessLevels: []Access{
				{AccessLevel: 30, AccessLevelDescription: "Developers + Maintainers"},
				{AccessLevel: 30, AccessLevelDescription: "Platform", Group: &AccessGroup{
					ID: "9", Name: "Platform", WebURL: "https://gitlab.example.com/groups/acme/platform",
					Parent: &AccessGroupRef{
						ID: "3", Name: "Acme", WebURL: "https://gitlab.example.com/groups/acme",
						AvatarURL: "https://gitlab.example.com/uploads/acme.png",
					},
				}},
			},
			UnprotectAccessLevels: []Access{
				{AccessLevel: 40, AccessLevelDescription: "Release managers", MemberRole: &AccessMemberRole{
					ID: "gid://gitlab/MemberRole/5", Name: "Release managers",
				}},
				{AccessLevel: 40, AccessLevelDescription: "Acme", Group: &AccessGroup{
					ID: "3", Name: "Acme", WebURL: "https://gitlab.example.com/groups/acme",
				}},
			},
		},
		ApprovalRules: []ApprovalRule{
			{
				ID: "gid://gitlab/ApprovalProjectRule/1", Name: "Security Review", ApprovalsRequired: 2, Type: "REGULAR",
				EligibleApprovers: []UserRef{jane},
			},
			{
				ID: "gid://gitlab/ApprovalProjectRule/2", Name: "Coverage-Check", ApprovalsRequired: 1, Type: "REPORT_APPROVER",
				CoverageMinimumThreshold: &threshold, EligibleApprovers: []UserRef{},
			},
		},
		ExternalStatusChecks: []ExternalStatusCheck{
			{ID: "gid://gitlab/MergeRequests::ExternalStatusCheck/3", Name: "SonarQube", ExternalURL: "https://sonar.example.com/check", HMAC: true},
		},
	}
}

// wantUnprotectedRule is sampleUnprotectedRuleNode as the output publishes it.
func wantUnprotectedRule() BranchRuleItem {
	return BranchRuleItem{
		Name:                  "feature/*",
		IsGroupLevel:          new(false),
		MatchingBranchesCount: 5,
		CreatedAt:             "2026-03-01T08:00:00Z",
		ApprovalRules:         []ApprovalRule{},
		ExternalStatusChecks:  []ExternalStatusCheck{},
	}
}

// assertRule compares a published rule with the one expected, value for
// value, nil against empty included, and shows both as JSON when they differ.
func assertRule(t *testing.T, label string, got, want BranchRuleItem) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("%s: marshal the published rule: %v", label, err)
	}
	wantJSON, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("%s: marshal the expected rule: %v", label, err)
	}
	t.Errorf("%s mismatch:\ngot:\n%s\nwant:\n%s", label, gotJSON, wantJSON)
}

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, graphqlMux(map[string]http.HandlerFunc{}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	spec := specs[0]
	if spec.OwnerPackage != "branchrules" || !spec.ReadOnly || !spec.Idempotent {
		t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
	}
	if spec.Usage == "" {
		t.Fatalf("Usage for %s is empty", spec.Name)
	}
	if len(spec.Aliases) == 0 {
		t.Fatalf("Aliases for %s are empty", spec.Name)
	}
}

// graphqlMux returns an [http.Handler] that routes GraphQL requests to the
// appropriate handler based on the query operation name.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// Handler tests.

// TestList_Success verifies that an Enterprise listing publishes every field
// its document selects: the grants of each kind with the user, group, deploy
// key or custom role they name, every flag, the squash option, and the
// approval rules and status checks with the fields they gained.
func TestList_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [`+sampleBranchRuleNode+`, `+sampleUnprotectedRuleNode+`],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	client.SetEnterprise(true)
	out, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(out.Rules))
	}

	assertRule(t, "rule[0]", out.Rules[0], wantProtectedRule())
	assertRule(t, "rule[1]", out.Rules[1], wantUnprotectedRule())
}

// TestList_EmptyProject verifies the List_EmptyProject handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyProject(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/empty-project"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(out.Rules))
	}
}

// TestList_ProjectNotFound verifies that List_ProjectNotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_ProjectNotFound(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": null}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{ProjectPath: "does/not-exist"})
	if err == nil {
		t.Fatal("expected error for nil project, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
	}
}

// TestList_MissingProjectPath verifies that List_MissingProjectPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingProjectPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty project_path, got nil")
	}
	if !strings.Contains(err.Error(), "project_path is required") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "project_path is required")
	}
}

// TestList_GraphQLErrorsAreReported verifies that a document GitLab refused is
// answered with its errors rather than as a missing project.
//
// GitLab returns HTTP 200 with a top-level errors array and a null project,
// which client-go leaves for the caller to notice, so the nil check would
// otherwise blame the project path for a fault in the query.
func TestList_GraphQLErrorsAreReported(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQLError(w, http.StatusOK, "Field 'branchRules' doesn't accept argument 'before'")
		},
	})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{ProjectPath: "my-group/my-project"})
	if err == nil {
		t.Fatalf("List() = %+v, want the GraphQL errors reported", out)
	}
	if !strings.Contains(err.Error(), "doesn't accept argument") {
		t.Errorf("List() error = %v, want it to carry the GitLab message", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("List() error = %v, want it not to blame the project path", err)
	}
}

// TestListWith_UndeclaredPaginationVariable verifies that the list handler
// refuses to run an operation that declares fewer variables than its input can
// send, instead of sending one GitLab would discard.
//
// This is the defect the shared helper's signature exists to prevent: a
// variable an operation does not declare is ignored rather than rejected, so
// the caller would be answered with a page it did not ask for and no error.
// branchRules is forward-only upstream, so the document dropped here is the
// forward cursor rather than the backward pair.
//
// The shortened document is passed in rather than assigned over the package
// constant, so nothing a parallel neighbor reads changes underneath it.
func TestListWith_UndeclaredPaginationVariable(t *testing.T) {
	document := strings.Replace(queryListBranchRulesCE, ", $after: String", "", 1)

	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			t.Error("listWith() ran an operation that cannot receive its own cursor")
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": {"branchRules": {"nodes": []}}}`)
		},
	})

	_, err := listWith[gqlBranchRuleNodeCE](context.Background(), testutil.NewTestClient(t, handler), document,
		ListInput{ProjectPath: "my-group/my-project"})
	if err == nil {
		t.Fatal("listWith() error = nil, want a refusal naming the missing declaration")
	}
	if !strings.Contains(err.Error(), "$after") {
		t.Errorf("listWith() error = %v, want it to name $after", err)
	}
}

// TestList_ServerError verifies that List_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project"})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

// TestList_CE verifies that a Community listing publishes what its document
// selects, the push and merge grants and a deploy key among them, and leaves
// every Enterprise field absent rather than false: a Community instance was
// never asked whether code owners must approve, so it has not said no.
func TestList_CE(t *testing.T) {
	var sent string
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			sent = body.Query
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [{
							"id": "gid://gitlab/Projects::BranchRule/8",
							"name": "main",
							"isDefault": true,
							"isProtected": true,
							"matchingBranchesCount": 1,
							"createdAt": "2026-01-15T10:00:00Z",
							"updatedAt": null,
							"squashOption": null,
							"branchProtection": {
								"allowForcePush": false,
								"pushAccessLevels": {"nodes": [
									{"accessLevel": 40, "accessLevelDescription": "Maintainers", "deployKey": null},
									{"accessLevel": 40, "accessLevelDescription": "Deploy key",
										"deployKey": {"id": "12", "title": "Release key", "expiresAt": null, "user": `+deployBotJSON+`}}
								]},
								"mergeAccessLevels": {"nodes": [
									{"accessLevel": 30, "accessLevelDescription": "Developers + Maintainers"}
								]}
							}
						}],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if sent != queryListBranchRulesCE {
		t.Errorf("List() sent a document other than the Community one on a Free instance:\n%s", sent)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	assertRule(t, "rule", out.Rules[0], BranchRuleItem{
		ID:                    "gid://gitlab/Projects::BranchRule/8",
		Name:                  "main",
		IsDefault:             true,
		IsProtected:           true,
		MatchingBranchesCount: 1,
		CreatedAt:             "2026-01-15T10:00:00Z",
		BranchProtection: &BranchProtection{
			PushAccessLevels: []PushAccess{
				{AccessLevel: 40, AccessLevelDescription: "Maintainers"},
				{AccessLevel: 40, AccessLevelDescription: "Deploy key", DeployKey: &AccessDeployKey{
					ID: "12", Title: "Release key", User: deployBot,
				}},
			},
			MergeAccessLevels: []Access{{AccessLevel: 30, AccessLevelDescription: "Developers + Maintainers"}},
		},
	})
}

// TestList_EnterpriseSendsTheEnterpriseDocument verifies the other half of the
// tier decision: a licensed instance is sent the document that asks for the
// Enterprise fields, and a Free one never is.
func TestList_EnterpriseSendsTheEnterpriseDocument(t *testing.T) {
	var sent string
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			sent = body.Query
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": {"branchRules": {"nodes": [], "pageInfo": {"hasNextPage": false, "endCursor": null}}}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	client.SetEnterprise(true)
	if _, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project"}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if sent != queryListBranchRulesEE {
		t.Errorf("List() sent a document other than the Enterprise one on a licensed instance:\n%s", sent)
	}
}

// selectionPaths lists every field path a document selects, from the node of
// the branch rules connection down, so two documents can be compared by what
// they ask for rather than by how they are spelled.
func selectionPaths(t *testing.T, document string) []string {
	t.Helper()
	parsed, err := parser.ParseQuery(&ast.Source{Input: document})
	if err != nil {
		t.Fatalf("parse document: %v", err)
	}
	var paths []string
	var walk func(prefix string, set ast.SelectionSet)
	walk = func(prefix string, set ast.SelectionSet) {
		for _, selection := range set {
			field, ok := selection.(*ast.Field)
			if !ok {
				t.Fatalf("selection %T under %q, want fields only", selection, prefix)
			}
			path := prefix + "." + field.Name
			paths = append(paths, path)
			walk(path, field.SelectionSet)
		}
	}
	walk("", parsed.Operations[0].SelectionSet)
	slices.Sort(paths)
	return paths
}

// TestQueryListBranchRules_EnterpriseAsksEverythingCommunityDoes verifies that
// the two documents differ only by addition, so a licensed instance is never
// told less than a Free one about the same rule.
func TestQueryListBranchRules_EnterpriseAsksEverythingCommunityDoes(t *testing.T) {
	enterprise := selectionPaths(t, queryListBranchRulesEE)
	for _, path := range selectionPaths(t, queryListBranchRulesCE) {
		if _, found := slices.BinarySearch(enterprise, path); !found {
			t.Errorf("the Community document asks for %s and the Enterprise one does not", path)
		}
	}
}

// TestQueryListBranchRulesCE_AsksNoEnterpriseField holds the Community
// document to the fields GitLab defines outside ee/. Each path below is one
// GitLab adds in its Enterprise edition, and a Community instance refuses the
// whole document for any one of them rather than answering it empty, which is
// the defect this tool shipped with before it had two documents. The pinned
// schema cannot catch it, being an Enterprise schema, so the list is kept
// here, with the file each field is defined in.
func TestQueryListBranchRulesCE_AsksNoEnterpriseField(t *testing.T) {
	const (
		rule       = ".project.branchRules.nodes"
		protection = rule + ".branchProtection"
	)
	enterpriseOnly := []string{
		rule + ".isGroupLevel",                                // ee/app/graphql/ee/types/projects/branch_rule_type.rb
		rule + ".approvalRules",                               // ee/app/graphql/ee/types/projects/branch_rule_type.rb
		rule + ".externalStatusChecks",                        // ee/app/graphql/ee/types/projects/branch_rule_type.rb
		protection + ".codeOwnerApprovalRequired",             // ee/app/graphql/ee/types/branch_rules/branch_protection_type.rb
		protection + ".isGroupLevel",                          // ee/app/graphql/ee/types/branch_rules/branch_protection_type.rb
		protection + ".modificationBlockedByPolicy",           // ee/app/graphql/ee/types/branch_rules/branch_protection_type.rb
		protection + ".protectedFromPushBySecurityPolicy",     // ee/app/graphql/ee/types/branch_rules/branch_protection_type.rb
		protection + ".warnModificationBlockedByPolicy",       // ee/app/graphql/ee/types/branch_rules/branch_protection_type.rb
		protection + ".warnProtectedFromPushBySecurityPolicy", // ee/app/graphql/ee/types/branch_rules/branch_protection_type.rb
		protection + ".unprotectAccessLevels",                 // ee/app/graphql/ee/types/branch_rules/branch_protection_type.rb
		protection + ".pushAccessLevels.nodes.user",           // ee/app/graphql/ee/types/branch_protections/base_access_level_type.rb
		protection + ".pushAccessLevels.nodes.group",          // ee/app/graphql/ee/types/branch_protections/base_access_level_type.rb
		protection + ".pushAccessLevels.nodes.memberRole",     // ee/app/graphql/ee/types/branch_protections/base_access_level_type.rb
		protection + ".mergeAccessLevels.nodes.user",          // ee/app/graphql/ee/types/branch_protections/base_access_level_type.rb
		protection + ".mergeAccessLevels.nodes.group",         // ee/app/graphql/ee/types/branch_protections/base_access_level_type.rb
		protection + ".mergeAccessLevels.nodes.memberRole",    // ee/app/graphql/ee/types/branch_protections/base_access_level_type.rb
	}
	community := selectionPaths(t, queryListBranchRulesCE)
	enterprise := selectionPaths(t, queryListBranchRulesEE)
	for _, path := range enterpriseOnly {
		if _, found := slices.BinarySearch(community, path); found {
			t.Errorf("the Community document asks for %s, which GitLab defines only in its Enterprise edition", path)
		}
		if _, found := slices.BinarySearch(enterprise, path); !found {
			t.Errorf("the Enterprise document does not ask for %s, so the entry above guards nothing", path)
		}
	}
}

// TestList_Pagination verifies that List forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_Pagination(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, r *http.Request) {
			vars, _ := testutil.ParseGraphQLVariables(r)
			after, _ := vars["after"].(string)
			if after == "cursor1" {
				testutil.RespondGraphQL(w, http.StatusOK, `{
					"project": {
						"branchRules": {
							"nodes": [`+sampleUnprotectedRuleNode+`],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": true, "endCursor": "cursor2", "startCursor": "cursor1"}
						}
					}
				}`)
				return
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [`+sampleBranchRuleNode+`],
						"pageInfo": {"hasNextPage": true, "hasPreviousPage": false, "endCursor": "cursor1", "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)

	// First page.
	out, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project"})
	if err != nil {
		t.Fatalf("List() page 1 error = %v", err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("page 1: expected 1 rule, got %d", len(out.Rules))
	}
	if out.Rules[0].Name != "main" {
		t.Errorf("page 1: rule name = %q, want %q", out.Rules[0].Name, "main")
	}
	if !out.Pagination.HasNextPage {
		t.Error("page 1: expected HasNextPage = true")
	}

	// Second page.
	out2, err := List(context.Background(), client, ListInput{
		ProjectPath: "my-group/my-project",
		After:       "cursor1",
	})
	if err != nil {
		t.Fatalf("List() page 2 error = %v", err)
	}
	if len(out2.Rules) != 1 {
		t.Fatalf("page 2: expected 1 rule, got %d", len(out2.Rules))
	}
	if out2.Rules[0].Name != "feature/*" {
		t.Errorf("page 2: rule name = %q, want %q", out2.Rules[0].Name, "feature/*")
	}
	if out2.Pagination.HasNextPage {
		t.Error("page 2: expected HasNextPage = false")
	}
	// The mock answers page two the way a keyset connection would, with a
	// previous page and a start cursor. This tool takes no before parameter,
	// so naming that cursor would offer a model a page it cannot ask for.
	if md := FormatListMarkdown(out2); strings.Contains(md, "prev page cursor") {
		t.Errorf("page 2 markdown = %q, want no previous page on a forward-only connection", md)
	}
}

// TestList_NullOptionalFields verifies that an Enterprise rule whose nullable
// fields GitLab sent as null publishes each as absent: the timestamps, the
// squash option, the three grant lists, the approval rules, the status checks
// and an approval rule's threshold and approvers.
func TestList_NullOptionalFields(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [{
							"id": "gid://gitlab/Projects::BranchRule/9",
							"name": "release/*",
							"isDefault": false,
							"isProtected": true,
							"isGroupLevel": true,
							"matchingBranchesCount": 3,
							"createdAt": null,
							"updatedAt": null,
							"squashOption": null,
							"branchProtection": {
								"allowForcePush": true,
								"codeOwnerApprovalRequired": false,
								"isGroupLevel": false,
								"modificationBlockedByPolicy": false,
								"protectedFromPushBySecurityPolicy": true,
								"warnModificationBlockedByPolicy": false,
								"warnProtectedFromPushBySecurityPolicy": true,
								"pushAccessLevels": null,
								"mergeAccessLevels": null,
								"unprotectAccessLevels": null
							},
							"approvalRules": null,
							"externalStatusChecks": null
						}],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	client.SetEnterprise(true)
	out, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	assertRule(t, "rule", out.Rules[0], BranchRuleItem{
		ID:                    "gid://gitlab/Projects::BranchRule/9",
		Name:                  "release/*",
		IsProtected:           true,
		IsGroupLevel:          new(true),
		MatchingBranchesCount: 3,
		BranchProtection: &BranchProtection{
			AllowForcePush:                        true,
			CodeOwnerApprovalRequired:             new(false),
			IsGroupLevel:                          new(false),
			ModificationBlockedByPolicy:           new(false),
			ProtectedFromPushBySecurityPolicy:     new(true),
			WarnModificationBlockedByPolicy:       new(false),
			WarnProtectedFromPushBySecurityPolicy: new(true),
		},
	})
}

// TestList_ApprovalRuleType_PublishedAsGitLabSentIt holds an approval rule's
// type to what the response carried, in both the shapes GitLab may send it in.
//
// ApprovalProjectRule.type is nullable in the pinned schema, so the guard
// around the dereference decides two things at once: a rule GitLab typed has to
// reach the caller carrying that type, and a rule it left untyped has to reach
// the caller at all rather than taking the process down on a nil pointer.
// Nothing asserted either half (every other fixture typed every rule, and no
// assertion read the field back), so the guard could be inverted with the
// whole suite still green, while a typed rule published no type and an untyped
// one panicked.
func TestList_ApprovalRuleType_PublishedAsGitLabSentIt(t *testing.T) {
	const sentType = "CODE_OWNER"

	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [{
							"name": "main",
							"isDefault": true,
							"isProtected": true,
							"matchingBranchesCount": 1,
							"createdAt": null,
							"updatedAt": null,
							"branchProtection": null,
							"approvalRules": {"nodes": [
								{"name": "Owners", "approvalsRequired": 2, "type": "`+sentType+`"},
								{"name": "Untyped", "approvalsRequired": 1, "type": null}
							]},
							"externalStatusChecks": {"nodes": []}
						}],
						"pageInfo": {"hasNextPage": false, "endCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	client.SetEnterprise(true)
	out, err := List(context.Background(), client, ListInput{ProjectPath: "my-group/my-project"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(out.Rules))
	}
	rules := out.Rules[0].ApprovalRules
	if len(rules) != 2 {
		t.Fatalf("len(ApprovalRules) = %d, want the typed and the untyped rule both published", len(rules))
	}
	if rules[0].Type != sentType {
		t.Errorf("ApprovalRules[0].Type = %q, want the type GitLab sent (%q)", rules[0].Type, sentType)
	}
	if rules[1].Type != "" {
		t.Errorf("ApprovalRules[1].Type = %q, want empty for a rule GitLab left untyped", rules[1].Type)
	}
	if rules[1].Name != "Untyped" || rules[1].ApprovalsRequired != 1 {
		t.Errorf("ApprovalRules[1] = %+v, want an untyped rule to keep its own name and count", rules[1])
	}
}

// Markdown formatter tests.

// branchRuleListHints is the guidance section a branch rule listing that
// links nothing closes with, pinned once so each whole-output expectation
// below names it rather than restating it.
const branchRuleListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'branch.get_protected' to see one rule's protection settings in full\n" +
	"- Use action 'branch.protect' to change what a branch pattern requires\n"

// branchRuleLinkedHints is the same section for a listing that renders a
// link, which it opens by asking for the links to be kept.
const branchRuleLinkedHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- " + toolutil.HintPreserveLinks + "\n" +
	"- Use action 'branch.get_protected' to see one rule's protection settings in full\n" +
	"- Use action 'branch.protect' to change what a branch pattern requires\n"

// branchRuleTableHeader is the header of the listing's main table.
const branchRuleTableHeader = "| Name | Default | Protected | Branches | Push | Merge | Force Push | CODEOWNERS | Approval Rules | Status Checks |\n" +
	"| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n"

// TestFormatListMarkdown_Empty pins the whole response of a project with no
// branch rules: one sentence, and no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	want := "No branch rules found.\n"

	if got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_WithRules pins the whole response of an Enterprise
// listing: the grant summaries in the table, then per rule the settings that
// are on and every grant with who it names, the approval rules with their
// threshold and approvers, and the status checks with their HMAC flag.
func TestFormatListMarkdown_WithRules(t *testing.T) {
	out := ListOutput{
		Rules:      []BranchRuleItem{wantProtectedRule(), wantUnprotectedRule()},
		Pagination: toolutil.GraphQLForwardPaginationOutput{HasNextPage: false},
	}

	got := FormatListMarkdown(out)

	want := "## Branch Rules (2)\n\n" +
		branchRuleTableHeader +
		"| main | ✅ | ✅ | 1 | Maintainers, Jane Doe, Deploy key | Developers + Maintainers, Platform | ❌ | ✅ | 2 (Security Review, Coverage-Check) | 1 (SonarQube) |\n" +
		"| feature/* | ❌ | ❌ | 5 | - | - | - | - | None | None |\n" +
		"\n### Protection for main\n\n" +
		"| Setting | Value |\n" +
		"| --- | --- |\n" +
		"| Squash option | Encourage |\n" +
		"| Protection created at the group level | ✅ |\n" +
		"| Modification blocked by a security policy | ✅ |\n" +
		"| Modification would be blocked by a warn-mode policy | ✅ |\n" +
		"\n| Grant | Allowed | Detail |\n" +
		"| --- | --- | --- |\n" +
		"| Push | Maintainers | - |\n" +
		"| Push | Jane Doe | user [@jane](https://gitlab.example.com/jane) |\n" +
		"| Push | Deploy key | deploy key Release key of [@deploy-bot](https://gitlab.example.com/deploy-bot), expires 2027-01-01 |\n" +
		"| Merge | Developers + Maintainers | - |\n" +
		"| Merge | Platform | group [Platform](https://gitlab.example.com/groups/acme/platform) |\n" +
		"| Unprotect | Release managers | custom role Release managers |\n" +
		"| Unprotect | Acme | group [Acme](https://gitlab.example.com/groups/acme) |\n" +
		"\n### Approval Rules for main\n\n" +
		"| Name | Approvals Required | Type | Coverage Threshold | Eligible Approvers |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| Security Review | 2 | REGULAR | - | [@jane](https://gitlab.example.com/jane) |\n" +
		"| Coverage-Check | 1 | REPORT_APPROVER | 80.5% | - |\n" +
		"\n### External Status Checks for main\n\n" +
		"| Name | URL | HMAC |\n" +
		"| --- | --- | --- |\n" +
		"| SonarQube | [https://sonar.example.com/check](https://sonar.example.com/check) | ✅ |\n" +
		"\nShowing 2 items | no more pages\n" +
		branchRuleLinkedHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_CommunityRule pins a Community rule: its grants are
// summarized and listed, the code owner column reads as not asked rather than
// as no, and a listing that links nothing does not ask for links to be kept.
func TestFormatListMarkdown_CommunityRule(t *testing.T) {
	got := FormatListMarkdown(ListOutput{Rules: []BranchRuleItem{{
		Name:                  "main",
		IsDefault:             true,
		IsProtected:           true,
		MatchingBranchesCount: 1,
		SquashOption:          &SquashOption{Option: "Do not allow", HelpText: "Squashing is never performed."},
		BranchProtection: &BranchProtection{
			AllowForcePush:    true,
			PushAccessLevels:  []PushAccess{{AccessLevel: 40, AccessLevelDescription: "Maintainers"}},
			MergeAccessLevels: []Access{{AccessLevel: 0, AccessLevelDescription: "No one"}},
		},
	}}})

	want := "## Branch Rules (1)\n\n" +
		branchRuleTableHeader +
		"| main | ✅ | ✅ | 1 | Maintainers | No one | ✅ | - | None | None |\n" +
		"\n### Protection for main\n\n" +
		"| Setting | Value |\n" +
		"| --- | --- |\n" +
		"| Squash option | Do not allow |\n" +
		"\n| Grant | Allowed | Detail |\n" +
		"| --- | --- | --- |\n" +
		"| Push | Maintainers | - |\n" +
		"| Merge | No one | - |\n" +
		"\nShowing 1 items | no more pages\n" +
		branchRuleListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_SettingsWithoutGrants pins a rule whose only content
// below the table is its settings: the grants table is not opened for nothing,
// and a rule-level group flag and a push-blocking policy both read as on.
func TestFormatListMarkdown_SettingsWithoutGrants(t *testing.T) {
	got := FormatListMarkdown(ListOutput{Rules: []BranchRuleItem{{
		Name:         "main",
		IsGroupLevel: new(true),
		BranchProtection: &BranchProtection{
			CodeOwnerApprovalRequired:             new(false),
			ProtectedFromPushBySecurityPolicy:     new(true),
			WarnProtectedFromPushBySecurityPolicy: new(true),
		},
	}}})

	want := "## Branch Rules (1)\n\n" +
		branchRuleTableHeader +
		"| main | ❌ | ❌ | 0 | - | - | ❌ | ❌ | None | None |\n" +
		"\n### Protection for main\n\n" +
		"| Setting | Value |\n" +
		"| --- | --- |\n" +
		"| Created at the group level | ✅ |\n" +
		"| Push blocked by a security policy | ✅ |\n" +
		"| Push would be blocked by a warn-mode policy | ✅ |\n" +
		"\nShowing 1 items | no more pages\n" +
		branchRuleListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_GrantsWithoutSettings pins a rule with grants and no
// setting on: the grants table opens the section directly, with no blank
// settings table and no stray line above it.
func TestFormatListMarkdown_GrantsWithoutSettings(t *testing.T) {
	got := FormatListMarkdown(ListOutput{Rules: []BranchRuleItem{{
		Name:         "main",
		IsGroupLevel: new(false),
		BranchProtection: &BranchProtection{
			MergeAccessLevels: []Access{{AccessLevel: 40, AccessLevelDescription: "Jane Doe", User: &jane}},
		},
	}}})

	want := "## Branch Rules (1)\n\n" +
		branchRuleTableHeader +
		"| main | ❌ | ❌ | 0 | - | Jane Doe | ❌ | - | None | None |\n" +
		"\n### Protection for main\n\n" +
		"| Grant | Allowed | Detail |\n" +
		"| --- | --- | --- |\n" +
		"| Merge | Jane Doe | user [@jane](https://gitlab.example.com/jane) |\n" +
		"\nShowing 1 items | no more pages\n" +
		branchRuleLinkedHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_NextPage pins that a connection with more to come
// says so in the heading and names the cursor to pass back, which is the only
// way a caller reaches the next page of a cursor-paginated list.
func TestFormatListMarkdown_NextPage(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Rules:      []BranchRuleItem{{Name: "main", IsDefault: true, IsProtected: true, MatchingBranchesCount: 1}},
		Pagination: toolutil.GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "cursor7"},
	})

	want := "## Branch Rules (1 shown, more available)\n\n" +
		branchRuleTableHeader +
		"| main | ✅ | ✅ | 1 | - | - | - | - | None | None |\n" +
		"\nShowing 1 items | next page cursor: `cursor7`\n" +
		branchRuleListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestRendersLink holds the decision to ask for links to be kept to the
// sections that actually render one, each kind of link on its own.
func TestRendersLink(t *testing.T) {
	group := &AccessGroup{Name: "Platform", WebURL: "https://gitlab.example.com/groups/platform"}
	key := &AccessDeployKey{Title: "Release key", User: deployBot}
	tests := []struct {
		name string
		rule BranchRuleItem
		want bool
	}{
		{"nothing below the table", BranchRuleItem{Name: "main"}, false},
		{"a role grant alone", BranchRuleItem{BranchProtection: &BranchProtection{
			MergeAccessLevels: []Access{{AccessLevelDescription: "Maintainers"}},
		}}, false},
		{"an approval rule with no approvers", BranchRuleItem{ApprovalRules: []ApprovalRule{{Name: "Review"}}}, false},
		{"a status check", BranchRuleItem{ExternalStatusChecks: []ExternalStatusCheck{{Name: "Sonar"}}}, true},
		{"an eligible approver", BranchRuleItem{ApprovalRules: []ApprovalRule{{Name: "Review"}, {EligibleApprovers: []UserRef{jane}}}}, true},
		{"a user grant", BranchRuleItem{BranchProtection: &BranchProtection{
			UnprotectAccessLevels: []Access{{User: &jane}},
		}}, true},
		{"a group grant", BranchRuleItem{BranchProtection: &BranchProtection{
			MergeAccessLevels: []Access{{}, {Group: group}},
		}}, true},
		{"a deploy key", BranchRuleItem{BranchProtection: &BranchProtection{
			PushAccessLevels: []PushAccess{{DeployKey: key}},
		}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rendersLink(tt.rule); got != tt.want {
				t.Errorf("rendersLink() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGrantDetail pins how each kind of grantee is named, and that a grant
// naming several is read out in full rather than by its first.
func TestGrantDetail(t *testing.T) {
	tests := []struct {
		name  string
		grant grant
		want  string
	}{
		{"a role", grant{access: Access{AccessLevelDescription: "Maintainers"}}, "-"},
		{"a user", grant{access: Access{User: &jane}}, "user [@jane](https://gitlab.example.com/jane)"},
		{"a group", grant{access: Access{Group: &AccessGroup{
			Name: "Plat|form", WebURL: "https://gitlab.example.com/groups/platform",
		}}}, "group [Plat&#124;form](https://gitlab.example.com/groups/platform)"},
		{"a custom role", grant{access: Access{MemberRole: &AccessMemberRole{Name: "Release|managers"}}}, "custom role Release&#124;managers"},
		{
			"a deploy key without expiry",
			grant{key: &AccessDeployKey{Title: "Key", User: deployBot}},
			"deploy key Key of [@deploy-bot](https://gitlab.example.com/deploy-bot)",
		},
		{"every grantee at once", grant{
			access: Access{User: &jane, Group: &AccessGroup{Name: "G", WebURL: "https://g"}, MemberRole: &AccessMemberRole{Name: "R"}},
			key:    &AccessDeployKey{Title: "K", ExpiresAt: "2027-01-01", User: deployBot},
		}, "user [@jane](https://gitlab.example.com/jane); group [G](https://g); custom role R; " +
			"deploy key K of [@deploy-bot](https://gitlab.example.com/deploy-bot), expires 2027-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := grantDetail(tt.grant); got != tt.want {
				t.Errorf("grantDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatAccessSummary pins the table's summary of one kind of grant: the
// dash for none, and GitLab's own descriptions joined and escaped otherwise.
func TestFormatAccessSummary(t *testing.T) {
	tests := []struct {
		name   string
		grants []Access
		want   string
	}{
		{"none", nil, "-"},
		{"one", []Access{{AccessLevelDescription: "Maintainers"}}, "Maintainers"},
		{"several, escaped", []Access{{AccessLevelDescription: "Maintainers"}, {AccessLevelDescription: "a|b"}}, "Maintainers, a&#124;b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatAccessSummary(tt.grants); got != tt.want {
				t.Errorf("formatAccessSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCoverageCell pins the threshold of a coverage-check rule and the dash of
// every other rule, fractions kept as GitLab sent them.
func TestCoverageCell(t *testing.T) {
	whole, fraction := 80.0, 72.25
	tests := []struct {
		name      string
		threshold *float64
		want      string
	}{
		{"not a coverage rule", nil, "-"},
		{"whole", &whole, "80%"},
		{"fraction", &fraction, "72.25%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := coverageCell(tt.threshold); got != tt.want {
				t.Errorf("coverageCell() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestApproversCell pins the approver list: the dash for none, every approver
// linked otherwise.
func TestApproversCell(t *testing.T) {
	if got := approversCell(nil); got != "-" {
		t.Errorf("approversCell(nil) = %q, want -", got)
	}
	got := approversCell([]UserRef{jane, deployBot})
	want := "[@jane](https://gitlab.example.com/jane), [@deploy-bot](https://gitlab.example.com/deploy-bot)"
	if got != want {
		t.Errorf("approversCell() = %q, want %q", got, want)
	}
}

// TestBoolPtrCell pins the three answers a flag the Community document never
// asks about can have.
func TestBoolPtrCell(t *testing.T) {
	tests := []struct {
		name string
		v    *bool
		want string
	}{
		{"not asked", nil, "-"},
		{"on", new(true), toolutil.BoolEmoji(true)},
		{"off", new(false), toolutil.BoolEmoji(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boolPtrCell(tt.v); got != tt.want {
				t.Errorf("boolPtrCell() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatApprovalRulesSummary verifies the ApprovalRulesSummary Markdown formatter for a representative approvalrulessummary input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatApprovalRulesSummary(t *testing.T) {
	tests := []struct {
		name  string
		rules []ApprovalRule
		want  string
	}{
		{"empty", nil, "None"},
		{"single", []ApprovalRule{{Name: "Review"}}, "1 (Review)"},
		{"multiple", []ApprovalRule{{Name: "A"}, {Name: "B"}}, "2 (A, B)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatApprovalRulesSummary(tt.rules)
			if got != tt.want {
				t.Errorf("formatApprovalRulesSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatStatusChecksSummary verifies the StatusChecksSummary Markdown formatter for a representative statuscheckssummary input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatStatusChecksSummary(t *testing.T) {
	tests := []struct {
		name   string
		checks []ExternalStatusCheck
		want   string
	}{
		{"empty", nil, "None"},
		{"single", []ExternalStatusCheck{{Name: "SonarQube"}}, "1 (SonarQube)"},
		{"multiple", []ExternalStatusCheck{{Name: "A"}, {Name: "B"}}, "2 (A, B)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatStatusChecksSummary(tt.checks)
			if got != tt.want {
				t.Errorf("formatStatusChecksSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestActionSpecs_CallRoute validates the CallRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoute(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [`+sampleBranchRuleNode+`],
						"pageInfo": {"hasNextPage": false, "endCursor": ""}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}

	res, err := specs[0].Route.Handler(t.Context(), map[string]any{"project_path": "my-group/my-project"})
	if err != nil {
		t.Fatalf("Route.Handler: %v", err)
	}
	out, ok := res.(ListOutput)
	if !ok {
		t.Fatalf("Route.Handler returned %T, want ListOutput", res)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(out.Rules))
	}
	if out.Rules[0].Name != "main" {
		t.Fatalf("Rules[0].Name = %q, want main", out.Rules[0].Name)
	}
}

// TestActionSpecs_CallRouteError validates the CallRouteError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_CallRouteError(t *testing.T) {
	client := testutil.NewTestClient(t, graphqlMux(map[string]http.HandlerFunc{}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}

	_, err := specs[0].Route.Handler(t.Context(), map[string]any{})
	if err == nil {
		t.Fatal("expected route error, got nil")
	}
}
