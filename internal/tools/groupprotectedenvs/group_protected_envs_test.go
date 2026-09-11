// group_protected_envs_test.go contains unit tests for the GitLab group protected environment MCP tool handlers.
package groupprotectedenvs

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	pathGroupProtEnvs = "/api/v4/groups/mygroup/protected_environments"
	pathGroupProtEnv  = "/api/v4/groups/mygroup/protected_environments/production"
)

// fullEnvJSON is a JSON response with deploy access levels and approval rules
// used across multiple tests.
const fullEnvJSON = `{
	"name":"production",
	"deploy_access_levels":[
		{"id":1,"access_level":40,"access_level_description":"Maintainers","user_id":10,"group_id":20,"group_inheritance_type":1}
	],
	"required_approval_count":2,
	"approval_rules":[
		{"id":5,"user_id":11,"group_id":21,"access_level":30,"access_level_description":"Developers","required_approvals":1,"group_inheritance_type":0}
	]
}`

// --- List tests ---

// TestList verifies the List handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	tests := []struct {
		name      string
		input     ListInput
		handler   http.HandlerFunc
		wantErr   bool
		wantCount int
		wantName  string
		cancelCtx bool
	}{
		{
			name:  "returns environments with deploy access levels and approval rules",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathGroupProtEnvs)
				testutil.RespondJSON(w, http.StatusOK, `[`+fullEnvJSON+`]`)
			}),
			wantCount: 1,
			wantName:  "production",
		},
		{
			name:  "returns paginated results",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSONWithPagination(
					w, http.StatusOK,
					`[{"name":"staging","deploy_access_levels":[],"required_approval_count":0,"approval_rules":[]}]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1", NextPage: ""},
				)
			}),
			wantCount: 1,
			wantName:  "staging",
		},
		{
			name:  "returns empty list when no environments exist",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			}),
			wantCount: 0,
		},
		{
			name: "forwards order_by, sort and keyset pagination query params",
			input: ListInput{
				GroupID:               "mygroup",
				OrderBy:               "name",
				Sort:                  "desc",
				KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "42"},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("order_by") != "name" || q.Get("sort") != "desc" ||
					q.Get("pagination") != "keyset" || q.Get("page_token") != "42" {
					t.Errorf("query = %q, missing order_by/sort/keyset params", r.URL.RawQuery)
				}
				testutil.RespondJSON(w, http.StatusOK, `[`+fullEnvJSON+`]`)
			}),
			wantCount: 1,
			wantName:  "production",
		},
		{
			name:  "returns error on API 500",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when group_id is empty",
			input:   ListInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     ListInput{GroupID: "mygroup"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			out, err := List(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(out.Environments) != tt.wantCount {
				t.Fatalf("len(Environments) = %d, want %d", len(out.Environments), tt.wantCount)
			}
			if tt.wantCount > 0 && out.Environments[0].Name != tt.wantName {
				t.Errorf("Name = %q, want %q", out.Environments[0].Name, tt.wantName)
			}
		})
	}
}

// TestList_FullOutputFields verifies the List_FullOutputFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_FullOutputFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+fullEnvJSON+`]`)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := out.Environments[0]
	if len(env.DeployAccessLevels) != 1 {
		t.Fatalf("DeployAccessLevels len = %d, want 1", len(env.DeployAccessLevels))
	}
	dal := env.DeployAccessLevels[0]
	if dal.ID != 1 || dal.AccessLevel != 40 || dal.UserID != 10 || dal.GroupID != 20 || dal.GroupInheritanceType != 1 {
		t.Errorf("DeployAccessLevel = %+v, unexpected field values", dal)
	}
	if len(env.ApprovalRules) != 1 {
		t.Fatalf("ApprovalRules len = %d, want 1", len(env.ApprovalRules))
	}
	ar := env.ApprovalRules[0]
	if ar.ID != 5 || ar.AccessLevel != 30 || ar.UserID != 11 || ar.GroupID != 21 || ar.RequiredApprovalCount != 1 {
		t.Errorf("ApprovalRule = %+v, unexpected field values", ar)
	}
}

// --- Get tests ---

// TestGet verifies the Get handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	tests := []struct {
		name      string
		input     GetInput
		handler   http.HandlerFunc
		wantErr   bool
		wantName  string
		cancelCtx bool
	}{
		{
			name:  "returns environment with approval rules",
			input: GetInput{GroupID: "mygroup", Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathGroupProtEnv)
				testutil.RespondJSON(w, http.StatusOK, fullEnvJSON)
			}),
			wantName: "production",
		},
		{
			name:  "returns error on 404",
			input: GetInput{GroupID: "mygroup", Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when group_id is empty",
			input:   GetInput{Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:    "returns error when environment is empty",
			input:   GetInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     GetInput{GroupID: "mygroup", Environment: "production"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			out, err := Get(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && out.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", out.Name, tt.wantName)
			}
		})
	}
}

// --- Protect tests ---

// TestProtect verifies the Protect handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProtect(t *testing.T) {
	accessLevel30 := 30
	accessLevel40 := 40
	userID := int64(10)
	groupID := int64(20)
	inheritType := int64(1)
	approvals := int64(2)

	tests := []struct {
		name      string
		input     ProtectInput
		handler   http.HandlerFunc
		wantErr   bool
		wantName  string
		cancelCtx bool
	}{
		{
			name:  "creates protected environment with minimal input",
			input: ProtectInput{GroupID: "mygroup", Name: "staging"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathGroupProtEnvs)
				testutil.RespondJSON(w, http.StatusCreated, `{"name":"staging","deploy_access_levels":[],"required_approval_count":0,"approval_rules":[]}`)
			}),
			wantName: "staging",
		},
		{
			name: "creates protected environment with deploy access levels and approval rules",
			input: ProtectInput{
				GroupID: "mygroup",
				Name:    "production",
				DeployAccessLevels: []DeployAccessLevelInput{
					{AccessLevel: &accessLevel40, UserID: &userID, GroupID: &groupID, GroupInheritanceType: &inheritType},
				},
				RequiredApprovalCount: &approvals,
				ApprovalRules: []ApprovalRuleInput{
					{AccessLevel: &accessLevel30, UserID: &userID, GroupID: &groupID, RequiredApprovalCount: &approvals, GroupInheritanceType: &inheritType},
				},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusCreated, fullEnvJSON)
			}),
			wantName: "production",
		},
		{
			name:  "returns error on API 403",
			input: ProtectInput{GroupID: "mygroup", Name: "staging"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when group_id is empty",
			input:   ProtectInput{Name: "staging"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:    "returns error when name is empty",
			input:   ProtectInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     ProtectInput{GroupID: "mygroup", Name: "staging"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			out, err := Protect(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Protect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && out.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", out.Name, tt.wantName)
			}
		})
	}
}

// TestProtect_InvalidTierIncludesActionableHint verifies GitLab validation errors
// guide the model toward the finite set of accepted group environment tiers.
func TestProtect_InvalidTierIncludesActionableHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, pathGroupProtEnvs)
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":["Name must be one of environment tiers: production, staging, testing, development, other."]}`)
	}))

	_, err := Protect(context.Background(), client, ProtectInput{GroupID: "mygroup", Name: "production-123"})
	if err == nil {
		t.Fatal("Protect() error = nil, want invalid tier error")
	}
	got := err.Error()
	for _, want := range []string{"valid group protected environment tiers", "production", "staging", "testing", "development", "other"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Fatalf("Protect() error = %q, want substring %q", got, want)
			}
		})
	}
}

// --- Update tests ---

// TestUpdate verifies the Update handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdate(t *testing.T) {
	accessLevel40 := 40
	accessLevel30 := 30
	count := int64(3)
	id := int64(1)
	userID := int64(10)
	groupID := int64(20)
	inheritType := int64(1)
	approvals := int64(2)
	destroy := true

	tests := []struct {
		name      string
		input     UpdateInput
		handler   http.HandlerFunc
		wantErr   bool
		wantCount int64
		cancelCtx bool
	}{
		{
			name:  "updates approval count",
			input: UpdateInput{GroupID: "mygroup", Environment: "production", RequiredApprovalCount: &count},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				testutil.AssertRequestPath(t, r, pathGroupProtEnv)
				testutil.RespondJSON(w, http.StatusOK, `{"name":"production","deploy_access_levels":[],"required_approval_count":3,"approval_rules":[]}`)
			}),
			wantCount: 3,
		},
		{
			name: "updates with new name and deploy access levels and approval rules",
			input: UpdateInput{
				GroupID:     "mygroup",
				Environment: "production",
				Name:        "prod-v2",
				DeployAccessLevels: []UpdateDeployAccessLevelInput{
					{ID: &id, AccessLevel: &accessLevel40, UserID: &userID, GroupID: &groupID, GroupInheritanceType: &inheritType, Destroy: &destroy},
				},
				ApprovalRules: []UpdateApprovalRuleInput{
					{ID: &id, AccessLevel: &accessLevel30, UserID: &userID, GroupID: &groupID, RequiredApprovalCount: &approvals, GroupInheritanceType: &inheritType, Destroy: &destroy},
				},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"name":"prod-v2","deploy_access_levels":[],"required_approval_count":0,"approval_rules":[]}`)
			}),
			wantCount: 0,
		},
		{
			name:  "returns error on API 500",
			input: UpdateInput{GroupID: "mygroup", Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when group_id is empty",
			input:   UpdateInput{Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:    "returns error when environment is empty",
			input:   UpdateInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     UpdateInput{GroupID: "mygroup", Environment: "production"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			out, err := Update(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && out.RequiredApprovalCount != tt.wantCount {
				t.Errorf("RequiredApprovalCount = %d, want %d", out.RequiredApprovalCount, tt.wantCount)
			}
		})
	}
}

// TestUpdate_NotFoundIncludesActionableHint verifies that Update_NotFoundIncludesActionableHint returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_NotFoundIncludesActionableHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		testutil.AssertRequestPath(t, r, pathGroupProtEnv)
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{GroupID: "mygroup", Environment: "production"})
	if err == nil {
		t.Fatal("Update() error = nil, want not found error")
	}
	for _, want := range []string{"protected_env_list", "valid tiers", "partial updates merge"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("Update() error missing %q: %v", want, err)
			}
		})
	}
}

// --- Unprotect tests ---

// TestUnprotect verifies the Unprotect handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUnprotect(t *testing.T) {
	tests := []struct {
		name      string
		input     UnprotectInput
		handler   http.HandlerFunc
		wantErr   bool
		cancelCtx bool
	}{
		{
			name:  "removes protection successfully",
			input: UnprotectInput{GroupID: "mygroup", Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathGroupProtEnv)
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:  "returns error on 404",
			input: UnprotectInput{GroupID: "mygroup", Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when group_id is empty",
			input:   UnprotectInput{Environment: "production"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:    "returns error when environment is empty",
			input:   UnprotectInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     UnprotectInput{GroupID: "mygroup", Environment: "production"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			err := Unprotect(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unprotect() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestUnprotect_ServerErrorUsesGenericMessage verifies that Unprotect_ServerErrorUsesGenericMessage returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUnprotect_ServerErrorUsesGenericMessage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodDelete)
		testutil.AssertRequestPath(t, r, pathGroupProtEnv)
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
	}))

	err := Unprotect(context.Background(), client, UnprotectInput{GroupID: "mygroup", Environment: "production"})
	if err == nil {
		t.Fatal("Unprotect() error = nil, want server error")
	}
	if strings.Contains(err.Error(), "valid tiers") {
		t.Fatalf("unexpected tier hint for server error: %v", err)
	}
}

// --- Markdown formatter tests ---

// cardHints is the guidance section every protected-environment card closes
// with.
const cardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'group.protected_env_update' to change the deploy access levels or approval rules\n" +
	"- Use action 'group.protected_env_unprotect' to remove this protection from the group\n"

// listHints is the guidance section the listing closes with. The table carries
// no link, so the preserve-links instruction is not written.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'group.protected_env_get' to see one environment's rules in full\n" +
	"- Use action 'group.protected_env_protect' to protect another environment tier\n" +
	"- Use action 'group.protected_env_list' to page through the rest of the group's protected environments\n"

// TestFormatOutputMarkdown_WithRules pins the whole card of a protected
// environment that carries both tables: the headline defers to the per-rule
// counts below it, and each rule says which role it grants and to whom.
func TestFormatOutputMarkdown_WithRules(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		Name:                  "production",
		RequiredApprovalCount: 2,
		DeployAccessLevels: []AccessLevelOutput{
			{ID: 1, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
			{ID: 2, AccessLevel: 40, AccessLevelDescription: "Release managers", GroupID: 55, GroupInheritanceType: 1},
		},
		ApprovalRules: []ApprovalRuleOutput{
			{ID: 5, AccessLevel: 30, AccessLevelDescription: "Developers", RequiredApprovalCount: 1},
			{ID: 6, AccessLevelDescription: "Sam Bauch", UserID: 123, RequiredApprovalCount: 1},
		},
	})

	want := "## Protected Environment: production\n\n" +
		"- **Required Approvals**: per approval rule (see below)\n" +
		"\n### Deploy Access Levels\n\n" +
		"| ID | Level | Grantee | Description | Inheritance |\n| --- | --- | --- | --- | --- |\n" +
		"| 1 | Maintainer |  | Maintainers |  |\n" +
		"| 2 | - | group #55 | Release managers | inherited |\n" +
		"\n### Approval Rules\n\n" +
		"| ID | Level | Grantee | Description | Required | Inheritance |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 5 | Developer |  | Developers | 1 |  |\n" +
		"| 6 | - | user #123 | Sam Bauch | 1 |  |\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_NoRules pins the card of an environment GitLab sent
// with no rules at all: the headline count it did send, and no empty table
// under it.
func TestFormatOutputMarkdown_NoRules(t *testing.T) {
	got := FormatOutputMarkdown(Output{Name: "staging", RequiredApprovalCount: 0})

	want := "## Protected Environment: staging\n\n" +
		"- **Required Approvals**: 0\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_DeployLevelsOnly pins the card of an environment
// whose only rules are deploy access levels: the headline is the count GitLab
// sent, since no approval rule overrides it.
func TestFormatOutputMarkdown_DeployLevelsOnly(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		Name:                  "dev",
		RequiredApprovalCount: 1,
		DeployAccessLevels:    []AccessLevelOutput{{ID: 3, AccessLevel: 30, AccessLevelDescription: "Developers", GroupID: 7}},
	})

	want := "## Protected Environment: dev\n\n" +
		"- **Required Approvals**: 1\n" +
		"\n### Deploy Access Levels\n\n" +
		"| ID | Level | Grantee | Description | Inheritance |\n| --- | --- | --- | --- | --- |\n" +
		"| 3 | - | group #7 | Developers | direct |\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins that a group with no protected
// environments renders the one sentence and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	if want := "No group protected environments found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_WithEnvironments pins the whole listing: the heading
// falls back to the rows shown when GitLab sent no total, the table opens a
// block of its own, and one guidance section closes the document.
func TestFormatListMarkdown_WithEnvironments(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Environments: []Output{
			{
				Name:                  "production",
				RequiredApprovalCount: 2,
				DeployAccessLevels:    []AccessLevelOutput{{ID: 1}},
				ApprovalRules:         []ApprovalRuleOutput{{ID: 5}},
			},
			{Name: "staging", RequiredApprovalCount: 0},
		},
	})

	want := "## Group Protected Environments (2)\n\n" +
		"| Name | Required Approvals | Deploy Access Levels | Approval Rules |\n| --- | --- | --- | --- |\n" +
		"| production | 2 | 1 | 1 |\n" +
		"| staging | 0 | 0 | 0 |\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestActionSpecs_RoundTrip validates the RoundTrip route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_RoundTrip(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/groups/mygroup/protected_environments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+fullEnvJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/groups/mygroup/protected_environments/production", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fullEnvJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/mygroup/protected_environments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, fullEnvJSON)
	})
	handler.HandleFunc("PUT /api/v4/groups/mygroup/protected_environments/production", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fullEnvJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/mygroup/protected_environments/production", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_group_protected_environment_list", map[string]any{"group_id": "mygroup"}},
		{"get", "gitlab_group_protected_environment_get", map[string]any{"group_id": "mygroup", "environment": "production"}},
		{"protect", "gitlab_group_protected_environment_protect", map[string]any{"group_id": "mygroup", "name": "production", "deploy_access_levels": []any{map[string]any{"access_level": float64(40)}}}},
		{"update", "gitlab_group_protected_environment_update", map[string]any{"group_id": "mygroup", "environment": "production"}},
		{"unprotect", "gitlab_group_protected_environment_unprotect", map[string]any{"group_id": "mygroup", "environment": "production"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if callErr != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, callErr)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}
