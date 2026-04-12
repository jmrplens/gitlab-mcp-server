// prompt_audit_test.go contains unit tests for audit MCP prompts.
package prompts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	routeProject           = "GET /api/v4/projects/{project}"
	routePushRule          = "GET /api/v4/projects/{project}/push_rule"
	routeProtectedBranches = "GET /api/v4/projects/{project}/protected_branches"
	routeMembersAll        = "GET /api/v4/projects/{project}/members/all"
	routeLabels            = "GET /api/v4/projects/{project}/labels"
	routeMilestones        = "GET /api/v4/projects/{project}/milestones"
	routeTemplatesIssues   = "GET /api/v4/projects/{project}/templates/issues"
	routeTemplatesMRs      = "GET /api/v4/projects/{project}/templates/merge_requests"

	testProjectPath  = "group/my-project"
	errMsgUnexpected = "unexpected error: %v"
	errMsgMissingID  = "expected error for missing project_id"
	assertContains   = "expected output to contain %q"
)

// assertContainsAll checks that text contains all expected substrings.
func assertContainsAll(t *testing.T, text string, checks []string) {
	t.Helper()
	for _, want := range checks {
		if !strings.Contains(text, want) {
			t.Errorf(assertContains, want)
		}
	}
}

// TestAuditProject_Settings verifies the behavior of audit project settings.
func TestAuditProject_Settings(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{
				Name:                             "my-project",
				PathWithNamespace:                testProjectPath,
				Description:                      "A test project",
				Visibility:                       gl.PrivateVisibility,
				DefaultBranch:                    "main",
				IssuesEnabled:                    true,
				MergeRequestsEnabled:             true,
				WikiEnabled:                      false,
				SnippetsEnabled:                  false,
				ContainerRegistryEnabled:         false,
				PackagesEnabled:                  true,
				MergeMethod:                      gl.FastForwardMerge,
				SquashOption:                     "default_on",
				OnlyAllowMergeIfPipelineSucceeds: true,
				OnlyAllowMergeIfAllDiscussionsAreResolved: true,
				RemoveSourceBranchAfterMerge:              true,
				SharedRunnersEnabled:                      true,
				Statistics:                                &gl.Statistics{RepositorySize: 1048576, StorageSize: 2097152},
			}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routePushRule, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `{
				"deny_delete_tag": true,
				"member_check": true,
				"prevent_secrets": true,
				"commit_message_regex": "^(feat|fix|docs):.*",
				"max_file_size": 10
			}`)
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_settings",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text

		checks := []string{
			"Project Settings Audit",
			testProjectPath,
			"private",
			"main",
			"ff",
			"Merge Settings",
			"Push Rules",
			"Commit message regex",
			"^(feat|fix|docs):.*",
			"Storage Statistics",
		}
		for _, want := range checks {
			if !strings.Contains(text, want) {
				t.Errorf(assertContains, want)
			}
		}
	})

	t.Run("MissingProjectID", func(t *testing.T) {
		session := newMCPSession(t, http.NewServeMux())
		_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_settings",
			Arguments: map[string]string{},
		})
		if err == nil {
			t.Fatal(errMsgMissingID)
		}
	})

	t.Run("ProjectNotFound", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusNotFound, `{"message": "404 Not Found"}`)
		})
		session := newMCPSession(t, mux)
		_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_settings",
			Arguments: map[string]string{"project_id": "999"},
		})
		if err == nil {
			t.Fatal("expected error for non-existent project")
		}
	})

	t.Run("NoPushRules", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{
				Name:              "proj",
				PathWithNamespace: "group/proj",
				DefaultBranch:     "main",
				Visibility:        gl.PublicVisibility,
			}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routePushRule, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusForbidden, `{"message": "403 Forbidden"}`)
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_settings",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text
		if !strings.Contains(text, "Push rules not available") {
			t.Error("expected fallback message when push rules are unavailable")
		}
	})
}

// TestAuditBranch_Protection verifies the behavior of audit branch protection.
func TestAuditBranch_Protection(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{
				PathWithNamespace: testProjectPath,
				DefaultBranch:     "main",
			}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeProtectedBranches, func(w http.ResponseWriter, r *http.Request) {
			branches := []*gl.ProtectedBranch{
				{
					Name:                      "main",
					AllowForcePush:            false,
					CodeOwnerApprovalRequired: true,
					PushAccessLevels: []*gl.BranchAccessDescription{
						{AccessLevel: 40},
					},
					MergeAccessLevels: []*gl.BranchAccessDescription{
						{AccessLevel: 30},
					},
				},
				{
					Name:           "release/*",
					AllowForcePush: false,
					PushAccessLevels: []*gl.BranchAccessDescription{
						{AccessLevel: 40},
					},
				},
			}
			data, _ := json.Marshal(branches)
			respondJSON(w, http.StatusOK, string(data))
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_branch_protection",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text

		checks := []string{
			"Branch Protection Audit",
			"main",
			"(default)",
			"Protected branches | 2",
			"Default branch protected | ✅",
			"release/*",
			"Maintainer",
			"Developer",
			"Code owner approval required",
		}
		for _, want := range checks {
			if !strings.Contains(text, want) {
				t.Errorf(assertContains, want)
			}
		}
	})

	t.Run("NoProtectedBranches", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{
				PathWithNamespace: "group/risky-project",
				DefaultBranch:     "main",
			}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeProtectedBranches, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_branch_protection",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text
		if !strings.Contains(text, "No protected branches found") {
			t.Error("expected warning for no protected branches")
		}
		if !strings.Contains(text, "Default branch protected | ❌") {
			t.Error("expected default branch not protected indicator")
		}
	})

	t.Run("MissingProjectID", func(t *testing.T) {
		session := newMCPSession(t, http.NewServeMux())
		_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_branch_protection",
			Arguments: map[string]string{},
		})
		if err == nil {
			t.Fatal(errMsgMissingID)
		}
	})
}

// TestAuditProject_Access verifies the behavior of audit project access.
func TestAuditProject_Access(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{
				PathWithNamespace: testProjectPath,
				SharedWithGroups: []gl.ProjectSharedWithGroup{
					{GroupID: 10, GroupName: "devops-team", GroupAccessLevel: 30},
				},
			}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, r *http.Request) {
			members := []*gl.ProjectMember{
				{Username: "admin-user", Name: "Admin User", State: "active", AccessLevel: 50},
				{Username: "lead-dev", Name: "Lead Dev", State: "active", AccessLevel: 40},
				{Username: "dev1", Name: "Dev One", State: "active", AccessLevel: 30},
				{Username: "dev2", Name: "Dev Two", State: "active", AccessLevel: 30},
				{Username: "old-user", Name: "Old User", State: "blocked", AccessLevel: 30},
				{Username: "reporter", Name: "Reporter", State: "active", AccessLevel: 20},
			}
			data, _ := json.Marshal(members)
			respondJSON(w, http.StatusOK, string(data))
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_access",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text

		checks := []string{
			"Project Access Audit",
			"Owner | 1",
			"Maintainer | 1",
			"Developer | 3",
			"Reporter | 1",
			"**Total** | **6**",
			"Blocked Accounts",
			"old-user",
			"Elevated Access",
			"admin-user",
			"lead-dev",
			"devops-team",
		}
		for _, want := range checks {
			if !strings.Contains(text, want) {
				t.Errorf(assertContains, want)
			}
		}
	})

	t.Run("MissingProjectID", func(t *testing.T) {
		session := newMCPSession(t, http.NewServeMux())
		_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_access",
			Arguments: map[string]string{},
		})
		if err == nil {
			t.Fatal(errMsgMissingID)
		}
	})

	t.Run("NoMembers", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{PathWithNamespace: "group/empty-project"}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_access",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text
		if !strings.Contains(text, "**Total** | **0**") {
			t.Error("expected total of 0 members")
		}
	})
}

// TestAuditProject_Workflow verifies the behavior of audit project workflow.
func TestAuditProject_Workflow(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{PathWithNamespace: testProjectPath}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeLabels, func(w http.ResponseWriter, r *http.Request) {
			labels := []*gl.Label{
				{Name: "bug", Color: "#d73a4a", Description: "Something isn't working", OpenIssuesCount: 5, OpenMergeRequestsCount: 1},
				{Name: "enhancement", Color: "#a2eeef", Description: "", OpenIssuesCount: 3},
				{Name: "priority::high", Color: "#ff0000", Description: "High priority"},
			}
			data, _ := json.Marshal(labels)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeMilestones, func(w http.ResponseWriter, r *http.Request) {
			state := r.URL.Query().Get("state")
			if state == "active" {
				milestones := []*gl.Milestone{
					{Title: "v1.0", State: "active"},
					{Title: "v2.0", State: "active"},
				}
				data, _ := json.Marshal(milestones)
				respondJSON(w, http.StatusOK, string(data))
			} else {
				respondJSON(w, http.StatusOK, `[{"title": "v0.9", "state": "closed"}]`)
			}
		})
		mux.HandleFunc(routeTemplatesIssues, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[{"name": "Bug Report"}, {"name": "Feature Request"}]`)
		})
		mux.HandleFunc(routeTemplatesMRs, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[{"name": "Default MR"}]`)
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_workflow",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text

		checks := []string{
			"Workflow Audit",
			"bug",
			"enhancement",
			"_missing_",
			"Without description:** 1",
			"Active:** 2",
			"Closed:** 1",
			"v1.0",
			"Issue templates | 2",
			"MR templates | 1",
			"Bug Report",
			"Feature Request",
			"Default MR",
		}
		assertContainsAll(t, text, checks)
	})

	t.Run("Empty", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{PathWithNamespace: "group/empty-project"}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeLabels, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		mux.HandleFunc(routeMilestones, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		mux.HandleFunc(routeTemplatesIssues, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		mux.HandleFunc(routeTemplatesMRs, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_workflow",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text
		assertContainsAll(t, text, []string{
			"No labels configured",
			"No milestones configured",
			"No templates found",
		})
	})

	t.Run("MissingProjectID", func(t *testing.T) {
		session := newMCPSession(t, http.NewServeMux())
		_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_workflow",
			Arguments: map[string]string{},
		})
		if err == nil {
			t.Fatal(errMsgMissingID)
		}
	})
}

// TestAuditProject_Full verifies the behavior of audit project full.
func TestAuditProject_Full(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mux := http.NewServeMux()

		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			project := gl.Project{
				Name:                             "my-project",
				PathWithNamespace:                testProjectPath,
				Visibility:                       gl.PrivateVisibility,
				DefaultBranch:                    "main",
				MergeMethod:                      gl.RebaseMerge,
				SquashOption:                     "default_on",
				OnlyAllowMergeIfPipelineSucceeds: true,
				OnlyAllowMergeIfAllDiscussionsAreResolved: true,
				RemoveSourceBranchAfterMerge:              true,
			}
			data, _ := json.Marshal(project)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeProtectedBranches, func(w http.ResponseWriter, r *http.Request) {
			branches := []*gl.ProtectedBranch{
				{
					Name:              "main",
					PushAccessLevels:  []*gl.BranchAccessDescription{{AccessLevel: 40}},
					MergeAccessLevels: []*gl.BranchAccessDescription{{AccessLevel: 30}},
				},
			}
			data, _ := json.Marshal(branches)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, r *http.Request) {
			members := []*gl.ProjectMember{
				{Username: "admin", Name: "Admin", State: "active", AccessLevel: 50},
				{Username: "dev", Name: "Dev", State: "active", AccessLevel: 30},
			}
			data, _ := json.Marshal(members)
			respondJSON(w, http.StatusOK, string(data))
		})
		mux.HandleFunc(routeLabels, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[{"name": "bug", "color": "#d73a4a", "description": "Bug"}]`)
		})
		mux.HandleFunc(routeMilestones, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[{"title": "v1.0", "state": "active"}]`)
		})
		mux.HandleFunc(routeTemplatesIssues, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[{"name": "Bug Report"}]`)
		})
		mux.HandleFunc(routeTemplatesMRs, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		mux.HandleFunc(routePushRule, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `{"prevent_secrets": true, "commit_message_regex": "^(feat|fix):"}`)
		})
		mux.HandleFunc("GET /api/v4/projects/{project}/hooks", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, `[{"id": 1, "url": "https://ci.example.com/hook", "push_events": true, "merge_requests_events": true, "issues_events": false, "enable_ssl_verification": true}]`)
		})

		session := newMCPSession(t, mux)
		result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_full",
			Arguments: map[string]string{"project_id": "42"},
		})
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text

		checks := []string{
			"Full Project Audit",
			"Quick Scorecard",
			"Default branch protected | ✅",
			"Pipeline required for merge | ✅",
			"1. Project Settings",
			"2. Branch Protection",
			"3. Access & Members",
			"4. Labels",
			"5. Milestones",
			"6. Templates",
			"7. Webhooks",
			"8. Push Rules",
			"Maintainer",
			"Developer",
			"**Total:** 1",
			"v1.0",
			"**Issue templates:** 1",
			"Prevent secrets | ✅",
		}
		for _, want := range checks {
			if !strings.Contains(text, want) {
				t.Errorf(assertContains, want)
			}
		}
	})

	t.Run("MissingProjectID", func(t *testing.T) {
		session := newMCPSession(t, http.NewServeMux())
		_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_full",
			Arguments: map[string]string{},
		})
		if err == nil {
			t.Fatal(errMsgMissingID)
		}
	})

	t.Run("ProjectNotFound", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusNotFound, `{"message": "404 Not Found"}`)
		})
		session := newMCPSession(t, mux)
		_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      "audit_project_full",
			Arguments: map[string]string{"project_id": "999"},
		})
		if err == nil {
			t.Fatal("expected error for non-existent project")
		}
	})
}

// Helper tests.

// TestAccessLevelName validates access level name across multiple scenarios using table-driven subtests.
func TestAccessLevelName(t *testing.T) {
	tests := []struct {
		level gl.AccessLevelValue
		want  string
	}{
		{10, "Guest"},
		{20, "Reporter"},
		{30, "Developer"},
		{40, "Maintainer"},
		{50, "Owner"},
		{99, "Unknown(99)"},
	}
	for _, tc := range tests {
		got := accessLevelName(tc.level)
		if got != tc.want {
			t.Errorf("accessLevelName(%d) = %q, want %q", tc.level, got, tc.want)
		}
	}
}

// TestEmptyDash verifies the behavior of empty dash.
func TestEmptyDash(t *testing.T) {
	if got := emptyDash(""); got != "—" {
		t.Errorf("emptyDash(\"\") = %q, want \"—\"", got)
	}
	if got := emptyDash("hello"); got != "hello" {
		t.Errorf("emptyDash(\"hello\") = %q, want \"hello\"", got)
	}
}

// TestFormatBytes validates format bytes across multiple scenarios using table-driven subtests.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tc := range tests {
		got := formatBytes(tc.bytes)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

// TestMaskURL verifies the behavior of mask u r l.
func TestMaskURL(t *testing.T) {
	short := "http://example.com"
	if got := maskURL(short); got != short {
		t.Errorf("maskURL short = %q, want %q", got, short)
	}
	long := "https://very-long-webhook-url.example.com/path/to/endpoint"
	got := maskURL(long)
	if len(got) > 34 || !strings.HasSuffix(got, "...") {
		t.Errorf("maskURL long = %q, expected truncated with ...", got)
	}
}
