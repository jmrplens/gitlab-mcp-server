// branchrules_test.go contains unit tests for GitLab branch rule operations.
// Tests use httptest to mock the GitLab Branch Rules API.

package branchrules

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const sampleBranchRuleNode = `{
	"name": "main",
	"isDefault": true,
	"isProtected": true,
	"matchingBranchesCount": 1,
	"createdAt": "2026-01-15T10:00:00Z",
	"updatedAt": "2026-06-20T14:30:00Z",
	"branchProtection": {
		"allowForcePush": false,
		"codeOwnerApprovalRequired": true
	},
	"approvalRules": {
		"nodes": [
			{"name": "Security Review", "approvalsRequired": 2, "type": "REGULAR"},
			{"name": "CODEOWNERS", "approvalsRequired": 1, "type": "CODE_OWNER"}
		]
	},
	"externalStatusChecks": {
		"nodes": [
			{"name": "SonarQube", "externalUrl": "https://sonar.example.com/check"}
		]
	}
}`

const sampleUnprotectedRuleNode = `{
	"name": "feature/*",
	"isDefault": false,
	"isProtected": false,
	"matchingBranchesCount": 5,
	"createdAt": "2026-03-01T08:00:00Z",
	"updatedAt": null,
	"branchProtection": null,
	"approvalRules": {"nodes": []},
	"externalStatusChecks": {"nodes": []}
}`

// graphqlMux returns an [http.Handler] that routes GraphQL requests to the
// appropriate handler based on the query operation name.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// Handler tests.

// TestList_Success verifies that listing branch rules returns the expected
// items when the GraphQL API responds with valid branch rule data.
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

	// Verify protected rule.
	r := out.Rules[0]
	if r.Name != "main" {
		t.Errorf("rule[0].Name = %q, want %q", r.Name, "main")
	}
	if !r.IsDefault {
		t.Error("rule[0].IsDefault = false, want true")
	}
	if !r.IsProtected {
		t.Error("rule[0].IsProtected = false, want true")
	}
	if r.MatchingBranchesCount != 1 {
		t.Errorf("rule[0].MatchingBranchesCount = %d, want 1", r.MatchingBranchesCount)
	}
	if r.BranchProtection == nil {
		t.Fatal("rule[0].BranchProtection is nil")
	}
	if r.BranchProtection.AllowForcePush {
		t.Error("rule[0].BranchProtection.AllowForcePush = true, want false")
	}
	if !r.BranchProtection.CodeOwnerApprovalRequired {
		t.Error("rule[0].BranchProtection.CodeOwnerApprovalRequired = false, want true")
	}
	if len(r.ApprovalRules) != 2 {
		t.Fatalf("rule[0].ApprovalRules length = %d, want 2", len(r.ApprovalRules))
	}
	if r.ApprovalRules[0].Name != "Security Review" {
		t.Errorf("rule[0].ApprovalRules[0].Name = %q, want %q", r.ApprovalRules[0].Name, "Security Review")
	}
	if r.ApprovalRules[0].ApprovalsRequired != 2 {
		t.Errorf("rule[0].ApprovalRules[0].ApprovalsRequired = %d, want 2", r.ApprovalRules[0].ApprovalsRequired)
	}
	if len(r.ExternalStatusChecks) != 1 {
		t.Fatalf("rule[0].ExternalStatusChecks length = %d, want 1", len(r.ExternalStatusChecks))
	}
	if r.ExternalStatusChecks[0].Name != "SonarQube" {
		t.Errorf("rule[0].ExternalStatusChecks[0].Name = %q, want %q", r.ExternalStatusChecks[0].Name, "SonarQube")
	}

	// Verify unprotected rule.
	r2 := out.Rules[1]
	if r2.Name != "feature/*" {
		t.Errorf("rule[1].Name = %q, want %q", r2.Name, "feature/*")
	}
	if r2.IsDefault {
		t.Error("rule[1].IsDefault = true, want false")
	}
	if r2.IsProtected {
		t.Error("rule[1].IsProtected = true, want false")
	}
	if r2.BranchProtection != nil {
		t.Error("rule[1].BranchProtection should be nil")
	}
	if len(r2.ApprovalRules) != 0 {
		t.Errorf("rule[1].ApprovalRules length = %d, want 0", len(r2.ApprovalRules))
	}
	if len(r2.ExternalStatusChecks) != 0 {
		t.Errorf("rule[1].ExternalStatusChecks length = %d, want 0", len(r2.ExternalStatusChecks))
	}
}

// TestList_EmptyProject verifies that listing branch rules for a project
// with no rules returns an empty result set.
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

// TestList_ProjectNotFound verifies that listing branch rules returns an
// error when the specified project does not exist.
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

// TestList_MissingProjectPath verifies that listing branch rules returns
// a validation error when the required project_path parameter is missing.
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

// TestList_ServerError verifies that listing branch rules propagates
// errors when the GraphQL API returns a server error.
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

// TestList_CE verifies that CE clients use the CE-compatible query and
// correctly parse responses without EE-only fields.
func TestList_CE(t *testing.T) {
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
							"createdAt": "2026-01-15T10:00:00Z",
							"updatedAt": null,
							"branchProtection": {"allowForcePush": false}
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
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	r := out.Rules[0]
	if r.Name != "main" {
		t.Errorf("rule.Name = %q, want %q", r.Name, "main")
	}
	if r.BranchProtection == nil {
		t.Fatal("BranchProtection should not be nil")
	}
	if r.BranchProtection.AllowForcePush {
		t.Error("AllowForcePush = true, want false")
	}
	if r.BranchProtection.CodeOwnerApprovalRequired {
		t.Error("CodeOwnerApprovalRequired should be false on CE")
	}
	if len(r.ApprovalRules) != 0 {
		t.Errorf("ApprovalRules length = %d, want 0 on CE", len(r.ApprovalRules))
	}
	if len(r.ExternalStatusChecks) != 0 {
		t.Errorf("ExternalStatusChecks length = %d, want 0 on CE", len(r.ExternalStatusChecks))
	}
}

// TestList_Pagination verifies that cursor-based pagination parameters
// are correctly forwarded to the GraphQL API and page info is returned.
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
		ProjectPath:            "my-group/my-project",
		GraphQLPaginationInput: toolutil.GraphQLPaginationInput{After: "cursor1"},
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
}

// TestList_NullOptionalFields verifies that branch rules with null
// optional fields (timestamps, approval rules) are handled without errors.
func TestList_NullOptionalFields(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [{
							"name": "release/*",
							"isDefault": false,
							"isProtected": true,
							"matchingBranchesCount": 3,
							"createdAt": null,
							"updatedAt": null,
							"branchProtection": {
								"allowForcePush": true,
								"codeOwnerApprovalRequired": false
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
	r := out.Rules[0]
	if r.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", r.CreatedAt)
	}
	if r.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty", r.UpdatedAt)
	}
	if r.BranchProtection == nil {
		t.Fatal("BranchProtection should not be nil")
	}
	if !r.BranchProtection.AllowForcePush {
		t.Error("AllowForcePush = false, want true")
	}
	if len(r.ApprovalRules) != 0 {
		t.Errorf("ApprovalRules length = %d, want 0", len(r.ApprovalRules))
	}
	if len(r.ExternalStatusChecks) != 0 {
		t.Errorf("ExternalStatusChecks length = %d, want 0", len(r.ExternalStatusChecks))
	}
}

// Markdown formatter tests.

// TestFormatListMarkdown_Empty verifies that formatting an empty branch
// rule list produces the expected no-results Markdown message.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No branch rules found") {
		t.Error("empty output should contain 'No branch rules found'")
	}
}

// TestFormatListMarkdown_WithRules verifies that formatting branch rules
// produces a Markdown table with protection status, approval rules, and status checks.
func TestFormatListMarkdown_WithRules(t *testing.T) {
	out := ListOutput{
		Rules: []BranchRuleItem{
			{
				Name:                  "main",
				IsDefault:             true,
				IsProtected:           true,
				MatchingBranchesCount: 1,
				BranchProtection: &BranchProtection{
					AllowForcePush:            false,
					CodeOwnerApprovalRequired: true,
				},
				ApprovalRules: []ApprovalRule{
					{Name: "Security Review", ApprovalsRequired: 2, Type: "REGULAR"},
				},
				ExternalStatusChecks: []ExternalStatusCheck{
					{Name: "SonarQube", ExternalURL: "https://sonar.example.com"},
				},
			},
			{
				Name:                  "feature/*",
				IsDefault:             false,
				IsProtected:           false,
				MatchingBranchesCount: 5,
			},
		},
		Pagination: toolutil.GraphQLPaginationOutput{HasNextPage: false},
	}

	md := FormatListMarkdown(out)
	if !strings.Contains(md, "main") {
		t.Error("should contain 'main'")
	}
	if !strings.Contains(md, "feature/*") {
		t.Error("should contain 'feature/*'")
	}
	if !strings.Contains(md, "Security Review") {
		t.Error("should contain approval rule name")
	}
	if !strings.Contains(md, "SonarQube") {
		t.Error("should contain status check name")
	}
	if !strings.Contains(md, "Approval Rules for") {
		t.Error("should contain approval rules detail section")
	}
	if !strings.Contains(md, "External Status Checks for") {
		t.Error("should contain external status checks detail section")
	}
}

// TestBoolIcon verifies that boolIcon returns the correct Yes/No strings.
func TestBoolIcon(t *testing.T) {
	if boolIcon(true) != "Yes" {
		t.Errorf("boolIcon(true) = %q, want %q", boolIcon(true), "Yes")
	}
	if boolIcon(false) != "No" {
		t.Errorf("boolIcon(false) = %q, want %q", boolIcon(false), "No")
	}
}

// TestFormatApprovalRulesSummary verifies the formatting of approval rule
// summaries including empty, single, and multiple rule cases.
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

// TestFormatStatusChecksSummary verifies the formatting of external status
// check summaries including empty, single, and multiple check cases.
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

// TestMCPRoundTrip_RegisterTools validates the RegisterTools wiring
// via MCP round-trip with a mock GraphQL backend. It verifies that
// the handler closure in register.go is fully exercised including
// List, LogToolCallAll, and WithHints on the success path.
func TestMCPRoundTrip_RegisterTools(t *testing.T) {
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

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_list_branch_rules",
		Arguments: map[string]any{"project_path": "my-group/my-project"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}
	if !strings.Contains(text.Text, "main") {
		t.Error("response should contain 'main' branch rule")
	}
}

// TestMCPRoundTrip_RegisterTools_Error validates the error path in the
// RegisterTools handler closure when List returns an error (missing
// project_path).
func TestMCPRoundTrip_RegisterTools_Error(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"branchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"branchRules": {
						"nodes": [],
						"pageInfo": {"hasNextPage": false, "endCursor": ""}
					}
				}
			}`)
		},
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_list_branch_rules",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}
