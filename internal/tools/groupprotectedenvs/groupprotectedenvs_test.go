// Package groupprotectedenvs tests cover the five CRUD handlers (List, Get,
// Protect, Update, Unprotect), their input validation, context cancellation,
// API error propagation, option converter functions, and markdown formatters.
package groupprotectedenvs

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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

// TestList covers success, pagination, empty results, API errors, context
// cancellation, and missing group_id validation for the List handler.
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
				testutil.RespondJSONWithPagination(w, http.StatusOK,
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
			name:  "returns error on API 500",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
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

// TestList_FullOutputFields verifies that List correctly maps deploy access
// levels and approval rules from the API response.
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

// TestGet covers success, API errors, context cancellation, and missing field
// validation for the Get handler.
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

// TestProtect covers success (with and without access levels/rules),
// API errors, context cancellation, and input validation.
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

// --- Update tests ---

// TestUpdate covers success (with/without rename, access levels, rules),
// API errors, context cancellation, and input validation.
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
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
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

// --- Unprotect tests ---

// TestUnprotect covers success, API errors, context cancellation, and input
// validation for the Unprotect handler.
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

// --- Markdown formatter tests ---

// TestFormatOutputMarkdown verifies the single-environment markdown renderer
// including deploy access levels and approval rules tables.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		contains []string
	}{
		{
			name: "renders full environment with access levels and rules",
			input: Output{
				Name:                  "production",
				RequiredApprovalCount: 2,
				DeployAccessLevels: []AccessLevelOutput{
					{ID: 1, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
				},
				ApprovalRules: []ApprovalRuleOutput{
					{ID: 5, AccessLevel: 30, AccessLevelDescription: "Developers", RequiredApprovalCount: 1},
				},
			},
			contains: []string{
				"## Protected Environment: production",
				"**Required Approval Count**: 2",
				"### Deploy Access Levels",
				"| 1 | 40 | Maintainers |",
				"### Approval Rules",
				"| 5 | 30 | Developers | 1 |",
			},
		},
		{
			name: "renders environment without access levels or rules",
			input: Output{
				Name:                  "staging",
				RequiredApprovalCount: 0,
			},
			contains: []string{
				"## Protected Environment: staging",
				"**Required Approval Count**: 0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

// TestFormatOutputMarkdown_NoTables verifies the markdown renderer omits
// table sections when there are no deploy access levels or approval rules.
func TestFormatOutputMarkdown_NoTables(t *testing.T) {
	got := FormatOutputMarkdown(Output{Name: "dev", RequiredApprovalCount: 0})
	if strings.Contains(got, "### Deploy Access Levels") {
		t.Error("should not contain Deploy Access Levels section for empty list")
	}
	if strings.Contains(got, "### Approval Rules") {
		t.Error("should not contain Approval Rules section for empty list")
	}
}

// TestFormatListMarkdown verifies the list markdown renderer including the
// empty-list case and populated environments.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
	}{
		{
			name:     "returns message for empty list",
			input:    ListOutput{},
			contains: []string{"No group protected environments found."},
		},
		{
			name: "renders table with environments",
			input: ListOutput{
				Environments: []Output{
					{
						Name:                  "production",
						RequiredApprovalCount: 2,
						DeployAccessLevels:    []AccessLevelOutput{{ID: 1}},
						ApprovalRules:         []ApprovalRuleOutput{{ID: 5}},
					},
					{
						Name:                  "staging",
						RequiredApprovalCount: 0,
					},
				},
			},
			contains: []string{
				"| Name | Approval Count | Deploy Levels | Rules |",
				"| production | 2 | 1 | 1 |",
				"| staging | 0 | 0 | 0 |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRegisterTools_MCPRoundTrip verifies that all 5 group protected env
// tools can be called successfully through the MCP transport.
func TestRegisterTools_MCPRoundTrip(t *testing.T) {
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
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

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
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, callErr)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}
