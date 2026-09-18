// prompt_audit_test.go contains unit tests for audit MCP prompts.
package prompts

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// routeProject identifies the route project constant used by this package.
	routeProject = "GET /api/v4/projects/{project}"
	// routePushRule identifies the route push rule constant used by this package.
	routePushRule = "GET /api/v4/projects/{project}/push_rule"
	// routeProtectedBranches identifies the route protected branches constant used by this package.
	routeProtectedBranches = "GET /api/v4/projects/{project}/protected_branches"
	// routeMembersAll identifies the route members all constant used by this package.
	routeMembersAll = "GET /api/v4/projects/{project}/members/all"
	// routeLabels identifies the route labels constant used by this package.
	routeLabels = "GET /api/v4/projects/{project}/labels"
	// routeMilestones identifies the route milestones constant used by this package.
	routeMilestones = "GET /api/v4/projects/{project}/milestones"
	// routeTemplatesIssues identifies the route templates issues constant used by this package.
	routeTemplatesIssues = "GET /api/v4/projects/{project}/templates/issues"
	// routeTemplatesMRs identifies the route templates MRs constant used by this package.
	routeTemplatesMRs = "GET /api/v4/projects/{project}/templates/merge_requests"

	// testProjectPath identifies the test project path constant used by this package.
	testProjectPath = "group/my-project"
	// errMsgUnexpected identifies the err msg unexpected constant used by this package.
	errMsgUnexpected = "unexpected error: %v"
	// errMsgMissingID identifies the err msg missing ID constant used by this package.
	errMsgMissingID = "expected error for missing project_id"
	// assertContains identifies the assert contains constant used by this package.
	assertContains = "expected output to contain %q"
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

func testPromptRequest(args map[string]string) *mcp.GetPromptRequest {
	return &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Arguments: args}}
}

// TestAuditProject_Settings verifies AuditProject when settings.
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
				IssuesAccessLevel:                gl.EnabledAccessControl,
				MergeRequestsAccessLevel:         gl.EnabledAccessControl,
				WikiAccessLevel:                  gl.DisabledAccessControl,
				SnippetsAccessLevel:              gl.DisabledAccessControl,
				ContainerRegistryAccessLevel:     gl.DisabledAccessControl,
				PackageRegistryAccessLevel:       gl.EnabledAccessControl,
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

		client := newTestClient(t, mux)
		result, err := handleAuditProjectSettings(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
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
			// The pipe of the alternation is entity-encoded, because it would
			// otherwise be read as the end of the cell holding the rule.
			"^(feat&#124;fix&#124;docs):.*",
			"Storage Statistics",
		}
		for _, want := range checks {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(text, want) {
					t.Errorf(assertContains, want)
				}
			})
		}
	})

	t.Run("MissingProjectID", func(t *testing.T) {
		client := newTestClient(t, http.NewServeMux())
		_, err := handleAuditProjectSettings(t.Context(), client, testPromptRequest(map[string]string{}))
		if err == nil {
			t.Fatal(errMsgMissingID)
		}
	})

	t.Run("ProjectNotFound", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(routeProject, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusNotFound, `{"message": "404 Not Found"}`)
		})
		client := newTestClient(t, mux)
		_, err := handleAuditProjectSettings(t.Context(), client, testPromptRequest(map[string]string{"project_id": "999"}))
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

		client := newTestClient(t, mux)
		result, err := handleAuditProjectSettings(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text
		if !strings.Contains(text, "Push rules not available") {
			t.Error("expected fallback message when push rules are unavailable")
		}
	})
}

// TestAuditBranch_Protection verifies AuditBranch when protection.
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

		client := newTestClient(t, mux)
		result, err := handleAuditBranchProtection(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
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
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(text, want) {
					t.Errorf(assertContains, want)
				}
			})
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

		client := newTestClient(t, mux)
		result, err := handleAuditBranchProtection(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
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
		client := newTestClient(t, http.NewServeMux())
		_, err := handleAuditBranchProtection(t.Context(), client, testPromptRequest(map[string]string{}))
		if err == nil {
			t.Fatal(errMsgMissingID)
		}
	})
}

// TestAuditProject_Access verifies AuditProject when access.
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

		client := newTestClient(t, mux)
		result, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
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
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(text, want) {
					t.Errorf(assertContains, want)
				}
			})
		}
	})

	t.Run("MissingProjectID", func(t *testing.T) {
		client := newTestClient(t, http.NewServeMux())
		_, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{}))
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

		client := newTestClient(t, mux)
		result, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
		if err != nil {
			t.Fatalf(errMsgUnexpected, err)
		}

		text := result.Messages[0].Content.(*mcp.TextContent).Text
		if !strings.Contains(text, "**Total** | **0**") {
			t.Error("expected total of 0 members")
		}
	})
}

// TestAuditProject_Workflow verifies AuditProject when workflow.
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

// TestAuditProject_Full verifies AuditProject when full.
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
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(text, want) {
					t.Errorf(assertContains, want)
				}
			})
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

// TestAuditProjectFull_SectionThatCouldNotBeRead_SaysSoInsteadOfScoringIt
// verifies that a full audit whose supporting calls fail reports the questions
// as unanswered rather than answering them from an empty slice.
//
// Every one of those calls used to be made with its error discarded, so a
// refusal produced an empty list and the empty list produced a verdict: no
// protected branches, no labels, no milestones, no templates, no webhooks and
// no push rules — a scorecard of eight failures on a project nobody managed to
// look at. A reader cannot tell that report from a genuinely unconfigured
// project, and the whole point of the scorecard is to be acted on.
func TestAuditProjectFull_SectionThatCouldNotBeRead_SaysSoInsteadOfScoringIt(t *testing.T) {
	mux := http.NewServeMux()
	// Only the project itself answers; every supporting call is refused, which
	// is what a token without the scope for them, or a Free instance asked for
	// push rules, produces.
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":42,"path_with_namespace":"group/my-project","default_branch":"main"}`)
	})

	text := getPromptText(t, mux, "audit_project_full", map[string]string{"project_id": "42"})

	for _, row := range []string{
		"| Default branch protected | " + unreadSection + " |",
		"| Push rules configured | " + unreadSection + " |",
		"| Labels configured | " + unreadSection + " |",
		"| Active milestones | " + unreadSection + " |",
		"| Issue templates | " + unreadSection + " |",
		"| MR templates | " + unreadSection + " |",
		"| Webhooks configured | " + unreadSection + " |",
	} {
		t.Run("the scorecard marks it unread: "+row, func(t *testing.T) {
			if !strings.Contains(text, row) {
				t.Errorf("scorecard is missing %q:\n%s", row, text)
			}
		})
	}

	for _, scored := range []string{
		"| Default branch protected | " + toolutil.EmojiCross + " |",
		"| Push rules configured | " + toolutil.EmojiCross + " |",
		"| Labels configured | " + toolutil.EmojiCross + " |",
		"| Webhooks configured | " + toolutil.EmojiCross + " |",
	} {
		t.Run("no verdict on an unread question: "+scored, func(t *testing.T) {
			if strings.Contains(text, scored) {
				t.Errorf("an unread question was answered with a verdict: %q\n%s", scored, text)
			}
		})
	}

	for _, absent := range []string{
		"**Protected branches:** 0",
		"**Total members:** 0",
		"**Total:** 0",
		"**Active:** 0",
		"**Configured:** 0",
		"Push rules not configured",
	} {
		t.Run("no empty answer reported: "+absent, func(t *testing.T) {
			if strings.Contains(text, absent) {
				t.Errorf("a section reported an empty answer it never received: %q\n%s", absent, text)
			}
		})
	}

	// The project fetch succeeded, so these two rows are real answers and must
	// stay verdicts rather than becoming unread with everything else.
	for _, row := range []string{
		"| Pipeline required for merge | " + toolutil.EmojiCross + " |",
		"| Discussions must be resolved | " + toolutil.EmojiCross + " |",
	} {
		t.Run("the project's own answer is still judged: "+row, func(t *testing.T) {
			if !strings.Contains(text, row) {
				t.Errorf("a row the project answered is missing %q:\n%s", row, text)
			}
		})
	}

	t.Run("the closing instruction says what an unread section means", func(t *testing.T) {
		if !strings.Contains(text, "was not audited") {
			t.Errorf("the closing instruction does not say what an unread section means:\n%s", text)
		}
	})
}

// TestFormatAccessLevels_NamesTheLevelsThroughTheSharedTable verifies that the
// audit prompts name an access level with toolutil.AccessLevelDescription and
// no longer with a table of their own.
//
// The local table knew five levels, so a Planner, a Minimal-access member and
// an instance admin were all reported as "Unknown(<n>)" — a word that tells a
// reader nothing and hides that GitLab did name the level. The shared table
// names them, and renders one it does not know as the number GitLab sent, which
// is the one thing a reader can act on.
func TestFormatAccessLevels_NamesTheLevelsThroughTheSharedTable(t *testing.T) {
	tests := []struct {
		name   string
		levels []*gl.BranchAccessDescription
		want   string
	}{
		{name: "no levels is no restriction", levels: nil, want: "-"},
		{
			name:   "guest",
			levels: []*gl.BranchAccessDescription{{AccessLevel: gl.GuestPermissions}},
			want:   "Guest",
		},
		{
			name: "every named level in order",
			levels: []*gl.BranchAccessDescription{
				{AccessLevel: gl.ReporterPermissions},
				{AccessLevel: gl.DeveloperPermissions},
				{AccessLevel: gl.MaintainerPermissions},
				{AccessLevel: gl.OwnerPermissions},
			},
			want: "Reporter, Developer, Maintainer, Owner",
		},
		{
			name:   "a level the old local table called unknown",
			levels: []*gl.BranchAccessDescription{{AccessLevel: gl.PlannerPermissions}},
			want:   "Planner",
		},
		{
			name:   "a level no table names is its number",
			levels: []*gl.BranchAccessDescription{{AccessLevel: 99}},
			want:   "Level 99",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatAccessLevels(tc.levels); got != tc.want {
				t.Errorf("formatAccessLevels() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFormatBranchAccessLevel_NamesUserAndGroupGrants verifies that a grant to
// one user or one group is rendered with its id beside the level's name.
func TestFormatBranchAccessLevel_NamesUserAndGroupGrants(t *testing.T) {
	tests := []struct {
		name  string
		level *gl.BranchAccessDescription
		want  string
	}{
		{
			name:  "role grant",
			level: &gl.BranchAccessDescription{AccessLevel: gl.MaintainerPermissions},
			want:  "Maintainer",
		},
		{
			name:  "user grant",
			level: &gl.BranchAccessDescription{AccessLevel: gl.DeveloperPermissions, UserID: 7},
			want:  "User #7 (Developer)",
		},
		{
			name:  "group grant",
			level: &gl.BranchAccessDescription{AccessLevel: gl.ReporterPermissions, GroupID: 3},
			want:  "Group #3 (Reporter)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatBranchAccessLevel(tc.level); got != tc.want {
				t.Errorf("formatBranchAccessLevel() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEmptyDash verifies EmptyDash.
func TestEmptyDash(t *testing.T) {
	if got := emptyDash(""); got != "-" {
		t.Errorf("emptyDash(\"\") = %q, want \"-\"", got)
	}
	if got := emptyDash("hello"); got != "hello" {
		t.Errorf("emptyDash(\"hello\") = %q, want \"hello\"", got)
	}
}

// TestFormatBytes covers FormatBytes with table-driven subtests.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"below_one_kilobyte", 500, "500 B"},
		{"one_kilobyte", 1024, "1.0 KB"},
		{"one_megabyte", 1048576, "1.0 MB"},
		{"one_gigabyte", 1073741824, "1.0 GB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatBytes(tc.bytes)
			if got != tc.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}

// TestWriteFullWebhooksSection_URLWithCredentials_RendersOriginOnly verifies
// that a webhook row names the host it posts to and nothing else: no userinfo,
// no path, no query.
//
// It matters because the row used to be the first thirty characters of the
// URL, which is long enough to spell a userinfo out in full, and because a
// hook's path is routinely the secret (a Slack-style endpoint is a public host
// plus an unguessable path).
func TestWriteFullWebhooksSection_URLWithCredentials_RendersOriginOnly(t *testing.T) {
	hooks := []*gl.ProjectHook{
		{URL: "https://hookuser:hookpass@hooks.example.com/services/T000/B000/SECRETPATHVALUE"},
		{URL: "https://short.example.com/?token=querysecret"},
	}

	var b strings.Builder
	writeFullWebhooksSection(&b, hooks, nil)
	got := b.String()

	for _, secret := range []string{"hookpass", "hookuser", "SECRETPATHVALUE", "B000", "querysecret"} {
		t.Run("withholds "+secret, func(t *testing.T) {
			if strings.Contains(got, secret) {
				t.Errorf("webhook section leaked %q:\n%s", secret, got)
			}
		})
	}
	for _, want := range []string{"https://hooks.example.com/...", "https://short.example.com/..."} {
		t.Run("renders "+want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("webhook section missing redacted origin %q:\n%s", want, got)
			}
		})
	}
}

// TestAuditBranchProtection_ProjectAPIError_ReturnsError verifies the project-fetch error
// branch of the branch protection audit.
func TestAuditBranchProtection_ProjectAPIError_ReturnsError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	_, err := handleAuditBranchProtection(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when project API fails")
	}
}

// TestAuditBranchProtection_BranchesAPIError_ReturnsError verifies the protected-branches
// error branch of the branch protection audit (project fetch succeeds).
func TestAuditBranchProtection_BranchesAPIError_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"path_with_namespace":"group/p","default_branch":"main"}`)
	})
	mux.HandleFunc(routeProtectedBranches, func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	client := newTestClient(t, mux)
	_, err := handleAuditBranchProtection(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when protected branches API fails")
	}
}

// TestWriteSharedGroups_WithExpiry_RendersExpiryDate verifies the expiry-date branch of the
// shared-groups table.
func TestWriteSharedGroups_WithExpiry_RendersExpiryDate(t *testing.T) {
	expires := gl.ISOTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	var b strings.Builder
	writeSharedGroups(&b, []gl.ProjectSharedWithGroup{
		{GroupID: 7, GroupName: "team-x", GroupAccessLevel: 30, ExpiresAt: &expires},
	})
	if !strings.Contains(b.String(), "2026-01-01") {
		t.Errorf("expected expiry date in output, got: %s", b.String())
	}
}

// TestAuditProjectAccess_MembersAPIError_ReturnsError verifies the members-fetch error
// branch of the project access audit.
func TestAuditProjectAccess_MembersAPIError_ReturnsError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	_, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when members API fails")
	}
}

// TestAuditProjectAccess_ProjectAPIError_ReturnsError verifies the project-fetch error
// branch of the project access audit (members fetch succeeds).
func TestAuditProjectAccess_ProjectAPIError_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	client := newTestClient(t, mux)
	_, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when project API fails")
	}
}

// TestAuditProjectAccess_InactiveAccounts_RendersInactiveSection verifies the inactive-accounts
// section branch when a member is neither active nor blocked.
func TestAuditProjectAccess_InactiveAccounts_RendersInactiveSection(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":1,"username":"u1","name":"U One","access_level":30,"state":"awaiting"}]`)
	})
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"path_with_namespace":"group/p"}`)
	})

	client := newTestClient(t, mux)
	result, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err != nil {
		t.Fatalf(errMsgUnexpected, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Inactive Accounts") {
		t.Error("expected inactive accounts section")
	}
}

// TestAuditProjectWorkflow_ProjectAPIError_ReturnsError verifies the project-fetch error
// branch of the workflow audit.
func TestAuditProjectWorkflow_ProjectAPIError_ReturnsError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	_, err := handleAuditProjectWorkflow(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when project API fails")
	}
}

// TestAuditProjectWorkflow_SubResourceAPIErrors_StillRendersReport verifies that the workflow
// audit degrades gracefully (warn-and-continue) when the labels and template
// APIs fail while the project fetch succeeds.
func TestAuditProjectWorkflow_SubResourceAPIErrors_StillRendersReport(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"path_with_namespace":"group/p"}`)
	})
	mux.HandleFunc(routeMilestones, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	// Labels and both template endpoints fall through to 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	client := newTestClient(t, mux)
	result, err := handleAuditProjectWorkflow(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err != nil {
		t.Fatalf(errMsgUnexpected, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No labels configured") {
		t.Error("expected no-labels warning when labels API fails")
	}
	if !strings.Contains(text, "No templates found") {
		t.Error("expected no-templates warning when template APIs fail")
	}
}

// TestWriteBranchDetail_TheUnprotectLine_AppearsOnlyWhenThereAreGrants
// verifies the one optional line of a protected branch's detail block.
//
// The guard is a `len(...) > 0` whose false side no test ever took, so read as
// `>= 0` every branch gains an "Unprotect access: No restrictions" line — a
// sentence that in a branch-protection audit says the opposite of the truth,
// since GitLab sends no unprotect levels for a branch nobody may unprotect.
func TestWriteBranchDetail_TheUnprotectLine_AppearsOnlyWhenThereAreGrants(t *testing.T) {
	var b strings.Builder
	writeBranchDetail(&b, &gl.ProtectedBranch{Name: "main"}, "main")
	if strings.Contains(b.String(), "Unprotect access") {
		t.Errorf("a branch with no unprotect grants wrote an unprotect line:\n%s", b.String())
	}
}

// TestAccessLevelIcon_SaysWhichWayTheSettingIsSet replaces a check that only
// required the icon to be non-empty.
//
// Both branches return an emoji, so "not empty" was true whatever the function
// decided and the `&&` over the two comparisons could be read any way at all.
// The icon is the whole content of the cell: an audit row that reports a
// disabled access control as enabled is a finding a reader will not make.
func TestAccessLevelIcon_SaysWhichWayTheSettingIsSet(t *testing.T) {
	tests := []struct {
		name  string
		input gl.AccessControlValue
		want  string
	}{
		{name: "enabled", input: gl.EnabledAccessControl, want: toolutil.EmojiSuccess},
		{name: "private", input: gl.PrivateAccessControl, want: toolutil.EmojiSuccess},
		{name: "disabled", input: gl.DisabledAccessControl, want: toolutil.EmojiCross},
		{name: "unset", input: "", want: toolutil.EmojiCross},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accessLevelIcon(tt.input); got != tt.want {
				t.Errorf("accessLevelIcon(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// auditProjectAccessText runs the project-access audit over the members given
// and returns the message it produced.
//
// The handler is called directly because this audit is not one of the two
// prompts this file registers; it is reached through gitlab_project's own
// action, and the prompt package only exposes the handler.
func auditProjectAccessText(t *testing.T, members []*gl.ProjectMember) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":42,"path_with_namespace":"group/my-project"}`)
	})
	mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, _ *http.Request) {
		data, err := json.Marshal(members)
		if err != nil {
			t.Errorf("marshaling the member fixture: %v", err)
			respondJSON(w, http.StatusInternalServerError, `{}`)
			return
		}
		respondJSON(w, http.StatusOK, string(data))
	})

	client := newTestClient(t, mux)
	result, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err != nil {
		t.Fatalf(errMsgUnexpected, err)
	}
	return result.Messages[0].Content.(*mcp.TextContent).Text
}

// TestAuditProjectAccess_EachOptionalSection_AppearsOnlyForWhatItReportsOn
// pins the four conditions that decide which sections this audit writes.
//
// The attention block, its two sub-sections and the elevated-access block are
// all `len(...) > 0` comparisons joined by `||`, and every fixture until now
// satisfied several of them at once. Left as they were, a project whose members
// are all active and none privileged would publish a warning heading with
// nothing under it, and one whose only privileged members are maintainers would
// have the elevated section withheld — or, since the slice under it is sized
// `len(owners)+len(maintainers)`, read as a difference and refused by the
// runtime for a negative capacity.
func TestAuditProjectAccess_EachOptionalSectionAppearsOnlyForWhatItReportsOn(t *testing.T) {
	t.Run("nothing to report writes neither block", func(t *testing.T) {
		text := auditProjectAccessText(t, []*gl.ProjectMember{
			{Username: "dev", Name: "Dev", State: "active", AccessLevel: 30},
		})

		for _, unwanted := range []string{"Accounts Needing Attention", "### Blocked Accounts", "### Inactive Accounts", "## Elevated Access"} {
			t.Run(unwanted, func(t *testing.T) {
				if strings.Contains(text, unwanted) {
					t.Errorf("a project with one ordinary developer wrote %q:\n%s", unwanted, text)
				}
			})
		}
	})

	t.Run("a blocked account writes only the blocked sub-section", func(t *testing.T) {
		text := auditProjectAccessText(t, []*gl.ProjectMember{
			{Username: "gone", Name: "Gone", State: "blocked", AccessLevel: 30},
		})

		if !strings.Contains(text, "### Blocked Accounts") {
			t.Errorf("expected the blocked sub-section:\n%s", text)
		}
		if strings.Contains(text, "### Inactive Accounts") {
			t.Errorf("nobody is merely inactive:\n%s", text)
		}
	})

	t.Run("an awaiting account writes only the inactive sub-section", func(t *testing.T) {
		text := auditProjectAccessText(t, []*gl.ProjectMember{
			{Username: "pending", Name: "Pending", State: "awaiting", AccessLevel: 30},
		})

		if !strings.Contains(text, "### Inactive Accounts") {
			t.Errorf("expected the inactive sub-section:\n%s", text)
		}
		if strings.Contains(text, "### Blocked Accounts") {
			t.Errorf("nobody is blocked:\n%s", text)
		}
	})

	t.Run("maintainers alone are elevated access", func(t *testing.T) {
		text := auditProjectAccessText(t, []*gl.ProjectMember{
			{Username: "maint", Name: "Maint", State: "active", AccessLevel: 40},
		})

		if !strings.Contains(text, "## Elevated Access (Owner + Maintainer)") {
			t.Errorf("a project with a maintainer and no owner still has elevated access:\n%s", text)
		}
		if !strings.Contains(text, "| Maintainer | 1 |") {
			t.Errorf("expected the maintainer count:\n%s", text)
		}
	})

	t.Run("owners alone are elevated access", func(t *testing.T) {
		text := auditProjectAccessText(t, []*gl.ProjectMember{
			{Username: "owner", Name: "Owner", State: "active", AccessLevel: 50},
		})

		if !strings.Contains(text, "## Elevated Access (Owner + Maintainer)") {
			t.Errorf("a project with an owner has elevated access:\n%s", text)
		}
	})
}

// TestWriteLabelsAudit_ALabelWithoutADescription_IsMarkedMissing verifies the
// per-row description cell of the workflow audit.
//
// The cell is an `== ""` choosing between the description and a warning, and
// nothing asserted either side, so it could be read as `!= ""` and every label
// that documents itself would be reported as undocumented while the ones that
// do not would show an empty cell. Labels without descriptions are one of the
// three gaps this prompt exists to find.
func TestWriteLabelsAudit_ALabelWithoutADescription_IsMarkedMissing(t *testing.T) {
	var b strings.Builder
	writeLabelsAudit(&b, []*gl.Label{
		{Name: "bug", Color: "#d73a4a", Description: "Something is broken"},
		{Name: "todo", Color: "#ffffff"},
	})

	got := b.String()
	if !strings.Contains(got, "**Total:** 2 | **Without description:** 1") {
		t.Errorf("expected one label counted as undocumented:\n%s", got)
	}
	if !strings.Contains(got, "| bug | #d73a4a | Something is broken |") {
		t.Errorf("a documented label should show its description:\n%s", got)
	}
	if !strings.Contains(got, toolutil.EmojiWarning+" _missing_") {
		t.Errorf("an undocumented label should be marked missing:\n%s", got)
	}
}

// TestWriteMilestonesAudit_TheTotalAndTheActiveTable pins the two things the
// milestone section decides from the counts it is given.
//
// The total adds the active and the closed lists, and the detail table is
// written only when there are active milestones. No fixture ever passed both
// lists non-empty, where a sum and a difference agree, and none passed closed
// milestones alone, where the table heading would stand over nothing.
func TestWriteMilestonesAudit_TheTotalAndTheActiveTable(t *testing.T) {
	t.Run("the total counts both lists", func(t *testing.T) {
		var b strings.Builder
		writeMilestonesAudit(&b,
			[]*gl.Milestone{{Title: "v1.0"}, {Title: "v1.1"}},
			[]*gl.Milestone{{Title: "v0.9"}})
		got := b.String()
		if !strings.Contains(got, "**Active:** 2 | **Closed:** 1 | **Total:** 3") {
			t.Errorf("expected two active and one closed to total three:\n%s", got)
		}
	})

	t.Run("closed milestones alone write no active table", func(t *testing.T) {
		var b strings.Builder
		writeMilestonesAudit(&b, nil, []*gl.Milestone{{Title: "v0.9"}})
		got := b.String()
		if strings.Contains(got, "### Active Milestones") {
			t.Errorf("there are no active milestones to tabulate:\n%s", got)
		}
	})
}

// TestWriteTemplatesAudit_EachSection_FollowsItsOwnKind pins the three
// conditions the templates section is made of.
//
// The "no templates" warning is an `&&` over both counts and each sub-section a
// `len(...) > 0`; every fixture had both kinds behave the same way. Read as an
// `||`, a project with issue templates and no merge-request ones is told it has
// no templates at all, directly above the list of the ones it has.
func TestWriteTemplatesAudit_EachSectionFollowsItsOwnKind(t *testing.T) {
	t.Run("one kind present", func(t *testing.T) {
		var b strings.Builder
		writeTemplatesAudit(&b, []*gl.ProjectTemplate{{Name: "Bug Report"}}, nil)
		got := b.String()
		if !strings.Contains(got, "### Issue Templates") {
			t.Errorf("expected the issue template section:\n%s", got)
		}
		if strings.Contains(got, "### MR Templates") {
			t.Errorf("there are no MR templates to list:\n%s", got)
		}
		if strings.Contains(got, "No templates found") {
			t.Errorf("a project with issue templates has templates:\n%s", got)
		}
	})

	t.Run("the other kind present", func(t *testing.T) {
		var b strings.Builder
		writeTemplatesAudit(&b, nil, []*gl.ProjectTemplate{{Name: "Default"}})
		got := b.String()
		if !strings.Contains(got, "### MR Templates") {
			t.Errorf("expected the MR template section:\n%s", got)
		}
		if strings.Contains(got, "### Issue Templates") {
			t.Errorf("there are no issue templates to list:\n%s", got)
		}
	})

	t.Run("neither kind present", func(t *testing.T) {
		var b strings.Builder
		writeTemplatesAudit(&b, nil, nil)
		got := b.String()
		if !strings.Contains(got, "No templates found") {
			t.Errorf("expected the warning:\n%s", got)
		}
		for _, unwanted := range []string{"### Issue Templates", "### MR Templates"} {
			t.Run(unwanted, func(t *testing.T) {
				if strings.Contains(got, unwanted) {
					t.Errorf("there is nothing to list, yet %q was written:\n%s", unwanted, got)
				}
			})
		}
	})
}

// TestWriteFullScorecard_EachRow_AnswersItsOwnQuestion drives every verdict of
// the quick scorecard from both sides.
//
// Six of the nine rows are a `len(...) > 0` handed to [auditVerdict], and every
// fixture so far gave most of them the same answer, so each could be read as
// `>= 0` — turning the scorecard of a project with nothing configured into a
// column of ticks. The push-rule row is the one with structure: a conjunction
// over the pointer and a disjunction over three settings, where reading the
// conjunction as a disjunction dereferences a nil push rule and reading either
// disjunct as a conjunction reports a project that prevents secrets as one with
// no push rules at all.
func TestWriteFullScorecard_EachRowAnswersItsOwnQuestion(t *testing.T) {
	project := &gl.Project{DefaultBranch: "main"}

	t.Run("nothing is configured", func(t *testing.T) {
		var b strings.Builder
		writeFullScorecard(&b, scorecardData{project: project})
		got := b.String()
		for _, want := range []string{
			"| Default branch protected | " + toolutil.EmojiCross + " |",
			"| Push rules configured | " + toolutil.EmojiCross + " |",
			"| Labels configured | " + toolutil.EmojiCross + " |",
			"| Active milestones | " + toolutil.EmojiCross + " |",
			"| Issue templates | " + toolutil.EmojiCross + " |",
			"| MR templates | " + toolutil.EmojiCross + " |",
			"| Webhooks configured | " + toolutil.EmojiCross + " |",
		} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in:\n%s", want, got)
				}
			})
		}
	})

	t.Run("everything is configured", func(t *testing.T) {
		var b strings.Builder
		writeFullScorecard(&b, scorecardData{
			project:    project,
			branches:   []*gl.ProtectedBranch{{Name: "main"}},
			labels:     []*gl.Label{{Name: "bug"}},
			milestones: []*gl.Milestone{{Title: "v1.0"}},
			issueTPL:   []*gl.ProjectTemplate{{Name: "Bug Report"}},
			mrTPL:      []*gl.ProjectTemplate{{Name: "Default"}},
			webhooks:   []*gl.ProjectHook{{ID: 1}},
			pushRule:   &gl.ProjectPushRules{CommitMessageRegex: "^(feat|fix):"},
		})
		got := b.String()
		for _, want := range []string{
			"| Default branch protected | " + toolutil.EmojiSuccess + " |",
			"| Push rules configured | " + toolutil.EmojiSuccess + " |",
			"| Labels configured | " + toolutil.EmojiSuccess + " |",
			"| Active milestones | " + toolutil.EmojiSuccess + " |",
			"| Issue templates | " + toolutil.EmojiSuccess + " |",
			"| MR templates | " + toolutil.EmojiSuccess + " |",
			"| Webhooks configured | " + toolutil.EmojiSuccess + " |",
		} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in:\n%s", want, got)
				}
			})
		}
	})

	t.Run("each push rule setting counts on its own", func(t *testing.T) {
		tests := []struct {
			name string
			rule *gl.ProjectPushRules
			want string
		}{
			{name: "no push rule at all", rule: nil, want: toolutil.EmojiCross},
			{name: "a rule with nothing set", rule: &gl.ProjectPushRules{}, want: toolutil.EmojiCross},
			{name: "only a commit message regex", rule: &gl.ProjectPushRules{CommitMessageRegex: "^fix"}, want: toolutil.EmojiSuccess},
			{name: "only secret prevention", rule: &gl.ProjectPushRules{PreventSecrets: true}, want: toolutil.EmojiSuccess},
			{name: "only the member check", rule: &gl.ProjectPushRules{MemberCheck: true}, want: toolutil.EmojiSuccess},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var b strings.Builder
				writeFullScorecard(&b, scorecardData{project: project, pushRule: tt.rule})
				if !strings.Contains(b.String(), "| Push rules configured | "+tt.want+" |") {
					t.Errorf("expected %q:\n%s", tt.want, b.String())
				}
			})
		}
	})
}

// TestWriteFullAccessSection_TheLevelTableAndTheSharingLine_FollowWhatIsThere
// pins the two conditions this section writes under.
//
// The access-level table is an `&&` over the fetch error and the member count,
// and the sharing line a `len(...) > 0`; neither false side was ever taken. Read
// as an `||` the table is written for a members call that failed, and read with
// the counts as `>= 0` a project shared with nobody publishes
// "**Shared with 0 group(s):**" — a sentence about an absence that reads like a
// finding.
func TestWriteFullAccessSection_TheLevelTableAndTheSharingLineFollowWhatIsThere(t *testing.T) {
	t.Run("no members writes no level table", func(t *testing.T) {
		var b strings.Builder
		writeFullAccessSection(&b, nil, nil, nil)
		got := b.String()
		if !strings.Contains(got, "**Total members:** 0") {
			t.Errorf("expected the total line:\n%s", got)
		}
		if strings.Contains(got, "| Access Level | Count |") {
			t.Errorf("there are no members to tabulate:\n%s", got)
		}
		if strings.Contains(got, "Shared with") {
			t.Errorf("the project is shared with nobody:\n%s", got)
		}
	})

	t.Run("a refused members call writes no level table", func(t *testing.T) {
		var b strings.Builder
		writeFullAccessSection(&b, []*gl.ProjectMember{{AccessLevel: 30}}, nil, errors.New("403 Forbidden"))
		got := b.String()
		if !strings.Contains(got, unreadSection) {
			t.Errorf("expected the section to say it could not be read:\n%s", got)
		}
		if strings.Contains(got, "| Access Level | Count |") {
			t.Errorf("a refused call must not be tabulated:\n%s", got)
		}
	})

	t.Run("each level is counted", func(t *testing.T) {
		var b strings.Builder
		writeFullAccessSection(&b, []*gl.ProjectMember{
			{AccessLevel: 30}, {AccessLevel: 30}, {AccessLevel: 40},
		}, nil, nil)
		got := b.String()
		if !strings.Contains(got, "| Developer | 2 |") {
			t.Errorf("expected two developers:\n%s", got)
		}
		if !strings.Contains(got, "| Maintainer | 1 |") {
			t.Errorf("expected one maintainer:\n%s", got)
		}
	})
}

// TestIsDefaultBranchProtected_ABranchThatIsNotTheDefaultOne_DoesNotCount
// verifies the comparison behind the scorecard's first row.
//
// The walk returns true on the first name that matches the default branch, and
// no test ever gave it a protected branch with another name, so the comparison
// could be read as an inequality and a project whose only protected branch is a
// release branch would be scored as protecting its default. That row is the one
// this audit leads with.
func TestIsDefaultBranchProtected_ABranchThatIsNotTheDefaultOne_DoesNotCount(t *testing.T) {
	branches := []*gl.ProtectedBranch{{Name: "release/1.0"}}
	if isDefaultBranchProtected(branches, "main") {
		t.Error("a protected release branch does not protect main")
	}
	if !isDefaultBranchProtected(append(branches, &gl.ProtectedBranch{Name: "main"}), "main") {
		t.Error("main is protected once it is in the list")
	}
}

// TestAuditProjectSettings_APushRuleEndpointAnsweringNull_WritesNoSection
// verifies the nil branch of the push-rule section.
//
// GitLab answers 200 with a null body for a project that has no push rules, so
// the pointer can be nil without the call having failed; the section then
// writes nothing at all, which is different from both the table and the "may
// require GitLab Premium" line. Only the error side was ever driven.
func TestAuditProjectSettings_APushRuleEndpointAnsweringNull_WritesNoSection(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":42,"path_with_namespace":"group/my-project","default_branch":"main"}`)
	})
	mux.HandleFunc(routePushRule, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `null`)
	})

	client := newTestClient(t, mux)
	result, err := handleAuditProjectSettings(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err != nil {
		t.Fatalf(errMsgUnexpected, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if strings.Contains(text, "## Push Rules") {
		t.Errorf("a project with no push rules and no error writes no push-rule section:\n%s", text)
	}
}

// TestWriteMilestonesAudit_ADueDateWithNoExpiryFlag_ReadsAsNotExpired verifies
// the second half of the expiry cell.
//
// GitLab omits `expired` on a milestone it has not evaluated, and the cell then
// falls to "No"; every fixture so far sent the flag, so the nil side of the
// check was never taken and a missing flag could have been read as expired.
func TestWriteMilestonesAudit_ADueDateWithNoExpiryFlag_ReadsAsNotExpired(t *testing.T) {
	due := gl.ISOTime(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	var b strings.Builder
	writeMilestonesAudit(&b, []*gl.Milestone{{Title: "v3.0", DueDate: &due}}, nil)

	if !strings.Contains(b.String(), "| v3.0 | 2026-12-31 | No |") {
		t.Errorf("a milestone with a due date and no expiry flag is not expired:\n%s", b.String())
	}
}

// TestWriteFullWebhooksSection_NoWebhooks_WritesNoTable verifies the table is
// written only when there is a hook to put in it.
//
// The guard is a `len(...) > 0` no test saw false, so read as `>= 0` a project
// with no webhooks gets a header row and a separator with nothing beneath —
// a table of nothing in the section whose whole content is the count above it.
func TestWriteFullWebhooksSection_NoWebhooks_WritesNoTable(t *testing.T) {
	var b strings.Builder
	writeFullWebhooksSection(&b, nil, nil)
	got := b.String()
	if !strings.Contains(got, "**Configured:** 0") {
		t.Errorf("expected the count:\n%s", got)
	}
	if strings.Contains(got, "| URL | Push |") {
		t.Errorf("there are no webhooks to tabulate:\n%s", got)
	}
}

// prompt_cross_project.go error branches.
