// target_branch_rules_test.go contains unit tests for GitLab project target
// branch rule operations (list, create, delete). The operations are backed by
// the GitLab GraphQL API, so tests use testutil.GraphQLHandler to mock the
// /api/graphql endpoint and verify both success and error/validation paths.
package projects

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// targetBranchRuleNode is a single GraphQL node for a target branch rule,
// using the gid:// global-ID format the SDK expects to unwrap into an int64.
const targetBranchRuleNode = `{
	"id": "gid://gitlab/Projects::TargetBranchRule/7",
	"name": "release/*",
	"targetBranch": "production",
	"createdAt": "2026-02-01T09:30:00Z"
}`

// TestListTargetBranchRules_Success verifies that ListTargetBranchRules maps a
// GraphQL connection of rules into the output shape, decoding the global ID,
// name, target branch, and creation timestamp.
func TestListTargetBranchRules_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"targetBranchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"project": {
					"targetBranchRules": {
						"nodes": [`+targetBranchRuleNode+`]
					}
				}
			}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListTargetBranchRules(context.Background(), client, ListTargetBranchRulesInput{ProjectID: "my-group/my-project"})
	if err != nil {
		t.Fatalf("ListTargetBranchRules() error = %v", err)
	}
	if len(out.TargetBranchRules) != 1 {
		t.Fatalf("len(TargetBranchRules) = %d, want 1", len(out.TargetBranchRules))
	}
	r := out.TargetBranchRules[0]
	if r.ID != 7 {
		t.Errorf("ID = %d, want 7", r.ID)
	}
	if r.Name != "release/*" {
		t.Errorf("Name = %q, want release/*", r.Name)
	}
	if r.TargetBranch != "production" {
		t.Errorf("TargetBranch = %q, want production", r.TargetBranch)
	}
	if !strings.HasPrefix(r.CreatedAt, "2026-02-01T09:30:00") {
		t.Errorf("CreatedAt = %q, want RFC3339 starting 2026-02-01T09:30:00", r.CreatedAt)
	}
}

// TestListTargetBranchRules_Empty verifies the empty-connection path yields an
// empty (non-nil) slice and no error.
func TestListTargetBranchRules_Empty(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"targetBranchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": {"targetBranchRules": {"nodes": []}}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListTargetBranchRules(context.Background(), client, ListTargetBranchRulesInput{ProjectID: "g/p"})
	if err != nil {
		t.Fatalf("ListTargetBranchRules() error = %v", err)
	}
	if len(out.TargetBranchRules) != 0 {
		t.Fatalf("len(TargetBranchRules) = %d, want 0", len(out.TargetBranchRules))
	}
}

// TestListTargetBranchRules_MissingProjectID verifies project_id validation.
func TestListTargetBranchRules_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListTargetBranchRules(context.Background(), client, ListTargetBranchRulesInput{})
	if err == nil || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("err = %v, want project_id is required", err)
	}
}

// TestListTargetBranchRules_ContextCanceled verifies the context guard.
func TestListTargetBranchRules_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListTargetBranchRules(ctx, client, ListTargetBranchRulesInput{ProjectID: "g/p"}); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// TestListTargetBranchRules_Error verifies that a project-not-found GraphQL
// response is wrapped with the path/Premium hint.
func TestListTargetBranchRules_Error(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"targetBranchRules": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project": null}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	if _, err := ListTargetBranchRules(context.Background(), client, ListTargetBranchRulesInput{ProjectID: "g/p"}); err == nil {
		t.Fatal("expected error for missing project")
	}
}

// TestCreateTargetBranchRule_Success verifies the create mutation maps the
// returned rule into the output shape.
func TestCreateTargetBranchRule_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"projectTargetBranchRuleCreate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"projectTargetBranchRuleCreate": {
					"targetBranchRule": `+targetBranchRuleNode+`,
					"errors": []
				}
			}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := CreateTargetBranchRule(context.Background(), client, CreateTargetBranchRuleInput{
		ProjectID:    "42",
		Name:         "release/*",
		TargetBranch: "production",
	})
	if err != nil {
		t.Fatalf("CreateTargetBranchRule() error = %v", err)
	}
	if out.ID != 7 || out.Name != "release/*" || out.TargetBranch != "production" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// TestCreateTargetBranchRule_Validation covers required-field and numeric-ID
// validation without reaching the API.
func TestCreateTargetBranchRule_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	tests := []struct {
		name  string
		input CreateTargetBranchRuleInput
		want  string
	}{
		{"missing project_id", CreateTargetBranchRuleInput{Name: "x", TargetBranch: "main"}, "project_id is required"},
		{"non-numeric project_id", CreateTargetBranchRuleInput{ProjectID: "g/p", Name: "x", TargetBranch: "main"}, "numeric project ID"},
		{"missing name", CreateTargetBranchRuleInput{ProjectID: "42", TargetBranch: "main"}, "name is required"},
		{"missing target_branch", CreateTargetBranchRuleInput{ProjectID: "42", Name: "x"}, "target_branch is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTargetBranchRule(context.Background(), client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want substring %q", err, tt.want)
			}
		})
	}
}

// TestCreateTargetBranchRule_ContextCanceled verifies the context guard.
func TestCreateTargetBranchRule_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CreateTargetBranchRule(ctx, client, CreateTargetBranchRuleInput{ProjectID: "42", Name: "x", TargetBranch: "main"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// TestCreateTargetBranchRule_Error verifies that mutation errors propagate.
func TestCreateTargetBranchRule_Error(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"projectTargetBranchRuleCreate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"projectTargetBranchRuleCreate": {"targetBranchRule": null, "errors": ["name has already been taken"]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	_, err := CreateTargetBranchRule(context.Background(), client, CreateTargetBranchRuleInput{ProjectID: "42", Name: "x", TargetBranch: "main"})
	if err == nil {
		t.Fatal("expected error from mutation errors")
	}
}

// TestDeleteTargetBranchRuleOutput_Success verifies the destructive delete path
// returns the legacy success-message shape.
func TestDeleteTargetBranchRuleOutput_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"projectTargetBranchRuleDestroy": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"projectTargetBranchRuleDestroy": {"errors": []}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := DeleteTargetBranchRuleOutput(context.Background(), client, DeleteTargetBranchRuleInput{RuleID: 7})
	if err != nil {
		t.Fatalf("DeleteTargetBranchRuleOutput() error = %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "7") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// TestDeleteTargetBranchRule_MissingRuleID verifies rule_id validation.
func TestDeleteTargetBranchRule_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteTargetBranchRule(context.Background(), client, DeleteTargetBranchRuleInput{})
	if err == nil || !strings.Contains(err.Error(), "rule_id is required") {
		t.Fatalf("err = %v, want rule_id is required", err)
	}
}

// TestDeleteTargetBranchRule_ContextCanceled verifies the context guard.
func TestDeleteTargetBranchRule_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := DeleteTargetBranchRule(ctx, client, DeleteTargetBranchRuleInput{RuleID: 7}); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// TestDeleteTargetBranchRuleOutput_Error verifies that destroy mutation errors
// propagate through the output wrapper.
func TestDeleteTargetBranchRuleOutput_Error(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"projectTargetBranchRuleDestroy": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"projectTargetBranchRuleDestroy": {"errors": ["rule not found"]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	if _, err := DeleteTargetBranchRuleOutput(context.Background(), client, DeleteTargetBranchRuleInput{RuleID: 99}); err == nil {
		t.Fatal("expected error from destroy mutation errors")
	}
}

// TestFormatListTargetBranchRulesMarkdown_WithRules verifies the table renders
// each rule and the action hints.
func TestFormatListTargetBranchRulesMarkdown_WithRules(t *testing.T) {
	out := ListTargetBranchRulesOutput{
		TargetBranchRules: []TargetBranchRuleOutput{
			{ID: 7, Name: "release/*", TargetBranch: "production", CreatedAt: "2026-02-01T09:30:00Z"},
			{ID: 8, Name: "hotfix/*", TargetBranch: "main"},
		},
	}
	md := FormatListTargetBranchRulesMarkdown(out)
	for _, want := range []string{"Target Branch Rules (2)", "release/*", "production", "hotfix/*", "target_branch_rule_create"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListTargetBranchRulesMarkdown_Empty verifies the empty branch.
func TestFormatListTargetBranchRulesMarkdown_Empty(t *testing.T) {
	md := FormatListTargetBranchRulesMarkdown(ListTargetBranchRulesOutput{})
	if !strings.Contains(md, "No target branch rules found.") {
		t.Errorf("markdown = %q, want empty notice", md)
	}
}

// TestFormatTargetBranchRuleMarkdown verifies single-rule rendering, including
// the created-at line.
func TestFormatTargetBranchRuleMarkdown(t *testing.T) {
	md := FormatTargetBranchRuleMarkdown(TargetBranchRuleOutput{ID: 7, Name: "release/*", TargetBranch: "production", CreatedAt: "2026-02-01T09:30:00Z"})
	for _, want := range []string{"Target Branch Rule: release/*", "production", "target_branch_rule_list"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	// A rule without CreatedAt should still render.
	mdNoCreated := FormatTargetBranchRuleMarkdown(TargetBranchRuleOutput{ID: 8, Name: "x", TargetBranch: "main"})
	if !strings.Contains(mdNoCreated, "main") {
		t.Errorf("markdown without created_at missing target branch:\n%s", mdNoCreated)
	}
}

// TestActionSpecs_TargetBranchRulesGated verifies the three target branch rule
// actions are present only under the enterprise catalog and carry the premium
// edition plus non-generic discovery metadata.
func TestActionSpecs_TargetBranchRulesGated(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	tools := []string{
		"gitlab_project_list_target_branch_rules",
		"gitlab_project_create_target_branch_rule",
		"gitlab_project_delete_target_branch_rule",
	}

	ce := ActionSpecs(client, false)
	for _, tool := range tools {
		if findProjectSpec(ce, tool) != nil {
			t.Errorf("%s should be gated out of the CE catalog", tool)
		}
	}

	ee := ActionSpecs(client, true)
	for _, tool := range tools {
		spec := findProjectSpec(ee, tool)
		if spec == nil {
			t.Fatalf("%s missing from enterprise catalog", tool)
		}
		if spec.Edition != "premium" {
			t.Errorf("%s Edition = %q, want premium", tool, spec.Edition)
		}
		if !strings.Contains(spec.IndividualTool.Description, "Returns:") || !strings.Contains(spec.IndividualTool.Description, "See also:") {
			t.Errorf("%s missing Returns/See also description: %q", tool, spec.IndividualTool.Description)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s has empty RelatedActions", tool)
		}
	}

	if listSpec := findProjectSpec(ee, "gitlab_project_list_target_branch_rules"); listSpec == nil || !listSpec.ReadOnly {
		t.Error("list target branch rules should be read-only")
	}
	if delSpec := findProjectSpec(ee, "gitlab_project_delete_target_branch_rule"); delSpec == nil || !delSpec.Destructive {
		t.Error("delete target branch rule should be destructive")
	}
}

// findProjectSpec returns the spec projecting the given individual tool, or nil.
func findProjectSpec(specs []toolutil.ActionSpec, tool string) *toolutil.ActionSpec {
	for i := range specs {
		if specs[i].IndividualTool.Name == tool {
			return &specs[i]
		}
	}
	return nil
}
