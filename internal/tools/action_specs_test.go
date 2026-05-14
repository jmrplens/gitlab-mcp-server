package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestCollectedActionSpecs_MigratedMetaToolParity(t *testing.T) {
	testCases := []struct {
		name       string
		client     *gitlabclient.Client
		enterprise bool
	}{
		{name: "base"},
		{name: "self-managed enterprise", enterprise: true},
		{name: "gitlab.com enterprise", client: newGitLabDotComClient(t), enterprise: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			captured := toolutil.CaptureMetaToolDefinitions(func() {
				registerAllMetaGroups(nil, tc.client, tc.enterprise)
			})
			capturedByTool := make(map[string]toolutil.MetaToolDefinition, len(captured))
			for _, definition := range captured {
				capturedByTool[definition.Name] = definition
			}

			specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(tc.client, tc.enterprise))
			if err != nil {
				t.Fatalf("actionSpecGroupsByTool() error = %v", err)
			}

			toolNames := make([]string, 0, len(capturedByTool))
			for toolName := range capturedByTool {
				toolNames = append(toolNames, toolName)
			}
			sort.Strings(toolNames)

			for _, toolName := range toolNames {
				t.Run(toolName, func(t *testing.T) {
					definition := capturedByTool[toolName]
					specs, ok := specsByTool[toolName]
					if !ok {
						t.Fatalf("collected action specs missing %s", toolName)
					}
					specRoutes, routeErr := toolutil.ActionSpecsToMapWithError(specs)
					if routeErr != nil {
						t.Fatalf("ActionSpecsToMapWithError() error = %v", routeErr)
					}
					assertActionRouteParity(t, toolName, definition.Routes, specRoutes)
					assertSpecProjectionParity(t, toolName, specs)
				})
			}
		})
	}
}

func TestCollectedActionSpecs_KnownGuidancePreserved(t *testing.T) {
	specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(newGitLabDotComClient(t), true))
	if err != nil {
		t.Fatalf("actionSpecGroupsByTool() error = %v", err)
	}

	testCases := []struct {
		toolName string
		action   string
		keys     []string
	}{
		{toolName: "gitlab_merge_request", action: "create", keys: []string{"source_branch", "target_branch"}},
		{toolName: "gitlab_issue", action: "link_create", keys: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid"}},
		{toolName: "gitlab_group", action: "epic_issue_assign", keys: []string{"full_path", "child_project_path", "child_iid"}},
		{toolName: "gitlab_job", action: "token_scope_remove_project", keys: []string{"project_id", "target_project_id"}},
		{toolName: "gitlab_access", action: "deploy_token_delete_project", keys: []string{"project_id", "deploy_token_id"}},
	}

	for _, tc := range testCases {
		t.Run(tc.toolName+"/"+tc.action, func(t *testing.T) {
			routes, routeErr := toolutil.ActionSpecsToMapWithError(specsByTool[tc.toolName])
			if routeErr != nil {
				t.Fatalf("ActionSpecsToMapWithError() error = %v", routeErr)
			}
			route, ok := routes[tc.action]
			if !ok {
				t.Fatalf("%s specs missing action %q", tc.toolName, tc.action)
			}
			assertGuidanceKeys(t, tc.toolName, tc.action, route.ParameterGuidance, tc.keys)
		})
	}
}

func TestIndividualToolProjection_RepresentativeDomainParity(t *testing.T) {
	session := newMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}
	toolsByName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		toolsByName[tool.Name] = tool
	}

	specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(nil, true))
	if err != nil {
		t.Fatalf("actionSpecGroupsByTool() error = %v", err)
	}

	for _, toolName := range []string{"gitlab_project", "gitlab_issue", "gitlab_merge_request", "gitlab_job", "gitlab_group"} {
		t.Run(toolName, func(t *testing.T) {
			for _, spec := range specsByTool[toolName] {
				individualName := strings.TrimSpace(spec.IndividualTool.Name)
				actual, ok := toolsByName[individualName]
				if !ok {
					t.Fatalf("%s.%s individual tool %q is not registered", toolName, spec.Name, individualName)
				}
				projected, projectionErr := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{
					Description: actual.Description,
					Icons:       actual.Icons,
				})
				if projectionErr != nil {
					t.Fatalf("%s.%s projection error = %v", toolName, spec.Name, projectionErr)
				}
				assertProjectedToolParity(t, toolName, spec.Name, actual, projected)
			}
		})
	}
}

func TestIndividualToolProjection_GoldenSnapshotParity(t *testing.T) {
	goldenPath := filepath.Join("testdata", "tools_individual.json")
	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}
	var golden []toolSnapshot
	if unmarshalErr := json.Unmarshal(goldenData, &golden); unmarshalErr != nil {
		t.Fatalf("parse golden file %s: %v", goldenPath, unmarshalErr)
	}
	goldenByName := make(map[string]toolSnapshot, len(golden))
	for _, snapshot := range golden {
		goldenByName[snapshot.Name] = snapshot
	}
	specsByIndividualName := individualSpecsByToolNameMap(CollectActionSpecs(nil, true))

	var projectedTools []*mcp.Tool
	missingSpecs := make([]string, 0)
	for _, snapshot := range golden {
		if _, ok := standaloneIndividualToolExceptions[snapshot.Name]; ok {
			continue
		}
		specs := specsByIndividualName[snapshot.Name]
		if len(specs) == 0 {
			missingSpecs = append(missingSpecs, snapshot.Name)
			continue
		}
		for _, spec := range specs {
			projected, projectionErr := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{Description: snapshot.Description})
			if projectionErr != nil {
				t.Fatalf("project %s from ActionSpec: %v", snapshot.Name, projectionErr)
			}
			projectedTools = append(projectedTools, projected)
		}
	}
	if len(missingSpecs) > 0 {
		sort.Strings(missingSpecs)
		t.Fatalf("golden individual tools missing ActionSpec projections: %v", missingSpecs)
	}

	projectedSnapshots := buildSnapshots(t, projectedTools)
	var wantSnapshots []toolSnapshot
	for _, projected := range projectedSnapshots {
		want, ok := goldenByName[projected.Name]
		if !ok {
			t.Fatalf("projected individual tool %q missing from golden snapshot", projected.Name)
		}
		wantSnapshots = append(wantSnapshots, want)
	}
	compareSnapshotSlices(t, goldenPath, wantSnapshots, projectedSnapshots)
}

func TestIndividualToolMetadata_CatalogBackedCoverage(t *testing.T) {
	session := newMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}
	toolsByName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		toolsByName[tool.Name] = tool
	}

	specNames := make(map[string]string)
	duplicateSpecNames := make([]string, 0)
	for _, group := range CollectActionSpecs(nil, true) {
		for _, spec := range group.Specs {
			name := strings.TrimSpace(spec.IndividualTool.Name)
			if name == "" {
				t.Fatalf("%s.%s missing individual tool name", group.ToolName, spec.Name)
			}
			if previous, exists := specNames[name]; exists {
				if _, ok := sharedIndividualToolSpecNames[name]; !ok {
					duplicateSpecNames = append(duplicateSpecNames, fmt.Sprintf("%s => %s, %s.%s", name, previous, group.ToolName, spec.Name))
				}
			} else {
				specNames[name] = group.ToolName + "." + spec.Name
			}
			if _, ok := toolsByName[name]; !ok {
				t.Fatalf("%s.%s references unregistered individual tool %q", group.ToolName, spec.Name, name)
			}
		}
	}
	if len(duplicateSpecNames) > 0 {
		sort.Strings(duplicateSpecNames)
		t.Fatalf("unexpected shared individual tool references: %v", duplicateSpecNames)
	}

	missingSpecs := make([]string, 0)
	for _, tool := range result.Tools {
		if _, ok := specNames[tool.Name]; ok {
			continue
		}
		if _, ok := standaloneIndividualToolExceptions[tool.Name]; ok {
			continue
		}
		missingSpecs = append(missingSpecs, tool.Name)
	}
	sort.Strings(missingSpecs)
	if len(missingSpecs) > 0 {
		t.Fatalf("individual tools missing ActionSpec metadata: %v", missingSpecs)
	}
}

var standaloneIndividualToolExceptions = map[string]string{
	"gitlab_discover_project":           "dynamic standalone project discovery helper",
	"gitlab_interactive_issue_create":   "elicitation standalone multi-step workflow",
	"gitlab_interactive_mr_create":      "elicitation standalone multi-step workflow",
	"gitlab_interactive_project_create": "elicitation standalone multi-step workflow",
	"gitlab_interactive_release_create": "elicitation standalone multi-step workflow",
	"gitlab_server_status":              "server diagnostic helper outside the GitLab API catalog",
}

var sharedIndividualToolSpecNames = map[string]string{
	"gitlab_commit_list":      "shared by gitlab_repository.commit_list and gitlab_repository.file_history",
	"gitlab_issue_list_group": "shared by gitlab_group.issues and gitlab_issue.list_group",
	"gitlab_user_current":     "shared by gitlab_user.current and gitlab_user.me",
}

func individualSpecsByToolNameMap(groups []ActionSpecGroup) map[string][]toolutil.ActionSpec {
	byName := make(map[string][]toolutil.ActionSpec)
	for _, group := range groups {
		for _, spec := range group.Specs {
			name := strings.TrimSpace(spec.IndividualTool.Name)
			if name != "" {
				byName[name] = append(byName[name], spec)
			}
		}
	}
	return byName
}

func compareSnapshotSlices(t *testing.T, goldenPath string, want, got []toolSnapshot) {
	t.Helper()
	sortToolSnapshots(want)
	sortToolSnapshots(got)
	if len(want) != len(got) {
		reportDiff(t, goldenPath, want, got)
		return
	}
	var diffs []string
	observedSchemaGaps := make(map[string]struct{})
	observedAnnotationGaps := make(map[string]struct{})
	for index := range want {
		name := want[index].Name
		if name != got[index].Name {
			diffs = append(diffs, fmt.Sprintf("%s projected name = %s", name, got[index].Name))
			continue
		}
		if want[index].Description != got[index].Description {
			diffs = append(diffs, "CHANGED "+name+" description")
		}
		if !schemaJSONEqual(t, name, want[index].InputSchema, got[index].InputSchema) {
			if _, ok := knownIndividualProjectionSchemaGaps[name]; !ok {
				diffs = append(diffs, "CHANGED "+name+" inputSchema")
			} else {
				observedSchemaGaps[name] = struct{}{}
			}
		}
		if !schemaJSONEqual(t, name, want[index].OutputSchema, got[index].OutputSchema) {
			diffs = append(diffs, "CHANGED "+name+" outputSchema")
		}
		if !annotationsEqual(t, name, want[index].Annotations, got[index].Annotations) {
			if _, ok := knownIndividualProjectionAnnotationGaps[name]; !ok {
				diffs = append(diffs, "CHANGED "+name+" annotations")
			} else {
				observedAnnotationGaps[name] = struct{}{}
			}
		}
	}
	appendStaleProjectionGapDiffs(&diffs, "schema", knownIndividualProjectionSchemaGaps, observedSchemaGaps)
	appendStaleProjectionGapDiffs(&diffs, "annotation", knownIndividualProjectionAnnotationGaps, observedAnnotationGaps)
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("generated individual snapshot parity drift against %s:\n%s", goldenPath, strings.Join(diffs, "\n"))
	}
}

func appendStaleProjectionGapDiffs(diffs *[]string, kind string, known map[string]string, observed map[string]struct{}) {
	for name := range known {
		if _, ok := observed[name]; !ok {
			*diffs = append(*diffs, fmt.Sprintf("STALE %s gap allowlist: %s", kind, name))
		}
	}
}

func sortToolSnapshots(snapshots []toolSnapshot) {
	sort.SliceStable(snapshots, func(left, right int) bool {
		return snapshots[left].Name < snapshots[right].Name
	})
}

func annotationsEqual(t *testing.T, name string, want, got *mcp.ToolAnnotations) bool {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want annotations for %s: %v", name, err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got annotations for %s: %v", name, err)
	}
	return string(wantJSON) == string(gotJSON)
}

func schemaJSONEqual(t *testing.T, name string, want, got json.RawMessage) bool {
	t.Helper()
	wantJSON, wantErr := normalizedSchemaJSON(want)
	if wantErr != nil {
		t.Fatalf("normalize want schema for %s: %v", name, wantErr)
	}
	gotJSON, gotErr := normalizedSchemaJSON(got)
	if gotErr != nil {
		t.Fatalf("normalize got schema for %s: %v", name, gotErr)
	}
	return string(wantJSON) == string(gotJSON)
}

func normalizedSchemaJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	normalizeSchemaValue("", value)
	return json.Marshal(value)
}

func normalizeSchemaValue(key string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			normalizeSchemaValue(childKey, childValue)
		}
	case []any:
		if key == "required" {
			required := make([]string, 0, len(typed))
			for _, item := range typed {
				field, ok := item.(string)
				if !ok {
					return
				}
				required = append(required, field)
			}
			sort.Strings(required)
			for index, field := range required {
				typed[index] = field
			}
			return
		}
		for _, childValue := range typed {
			normalizeSchemaValue("", childValue)
		}
	}
}

var knownIndividualProjectionAnnotationGaps = map[string]string{
	"gitlab_add_group_job_token_allowlist":   "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_add_project_job_token_allowlist": "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_ban_user":                        "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_block_user":                      "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_commit_status_set":               "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_deactivate_user":                 "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_deployment_approve_or_reject":    "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_disable_2fa_enterprise_user":     "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_disable_two_factor":              "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_environment_stop":                "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_force_push_mirror_update":        "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_group_transfer_project":          "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_group_unshare":                   "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_issue_move":                      "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_issue_spent_time_add":            "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_job_play":                        "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_job_retry":                       "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_mark_migration":                  "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_mr_add_spent_time":               "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_mr_approval_reset":               "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_mr_draft_note_publish":           "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_mr_draft_note_publish_all":       "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_mr_merge":                        "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_mr_unapprove":                    "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_pipeline_retry":                  "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_pipeline_schedule_run":           "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_set_custom_attribute":            "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_set_feature_flag":                "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_test_system_hook":                "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_unlock_terraform_state":          "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_wiki_delete":                     "ActionSpec marks idempotent, historical individual metadata does not",
	"gitlab_wiki_update":                     "ActionSpec marks idempotent, historical individual metadata does not",
}

var knownIndividualProjectionSchemaGaps = map[string]string{
	"gitlab_analyze_ci_configuration":          "ActionSpec schema preserves optional content_ref while historical individual snapshot marks it required",
	"gitlab_analyze_mr_changes":                "ActionSpec schema omits sampling fields that historical individual snapshot marks required",
	"gitlab_ci_lint":                           "ActionSpec schema preserves optional lint controls while historical individual snapshot marks them required",
	"gitlab_ci_lint_project":                   "ActionSpec schema preserves optional lint controls while historical individual snapshot marks them required",
	"gitlab_ci_variable_create":                "ActionSpec schema preserves optional variable controls while historical individual snapshot marks them required",
	"gitlab_ci_variable_delete":                "ActionSpec schema preserves optional environment_scope while historical individual snapshot marks it required",
	"gitlab_ci_variable_get":                   "ActionSpec schema preserves optional environment_scope while historical individual snapshot marks it required",
	"gitlab_ci_variable_update":                "ActionSpec schema preserves optional variable controls while historical individual snapshot marks them required",
	"gitlab_commit_cherry_pick":                "ActionSpec schema preserves optional branch controls while historical individual snapshot marks them required",
	"gitlab_commit_revert":                     "ActionSpec schema preserves optional branch controls while historical individual snapshot marks them required",
	"gitlab_create_cluster_agent_token":        "ActionSpec schema preserves optional token description while historical individual snapshot marks it required",
	"gitlab_current_user_status":               "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_deploy_token_create_group":         "ActionSpec schema preserves optional deploy token controls while historical individual snapshot marks them required",
	"gitlab_deploy_token_create_project":       "ActionSpec schema preserves optional deploy token controls while historical individual snapshot marks them required",
	"gitlab_deploy_token_list_all":             "ActionSpec schema preserves optional pagination and filter controls while historical individual snapshot marks them required",
	"gitlab_enable_disable_error_tracking":     "ActionSpec schema preserves optional setting values while historical individual snapshot marks them required",
	"gitlab_find_technical_debt":               "ActionSpec schema preserves optional ref while historical individual snapshot marks it required",
	"gitlab_generate_release_notes":            "ActionSpec schema preserves optional to ref while historical individual snapshot marks it required",
	"gitlab_get_appearance":                    "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_application_statistics":        "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_avatar":                        "ActionSpec schema preserves optional avatar inputs while historical individual snapshot marks them required",
	"gitlab_get_compliance_policy_settings":    "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_email":                         "ActionSpec schema preserves optional email ID shape differently from the historical individual snapshot",
	"gitlab_get_group_issue_statistics":        "ActionSpec schema preserves optional issue filters while historical individual snapshot marks them required",
	"gitlab_get_issue_statistics":              "ActionSpec schema preserves optional issue filters while historical individual snapshot marks them required",
	"gitlab_get_license":                       "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_metadata":                      "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_metric_definitions":            "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_non_sql_metrics":               "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_project_issue_statistics":      "ActionSpec schema preserves optional issue filters while historical individual snapshot marks them required",
	"gitlab_get_service_ping":                  "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_settings":                      "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_sidekiq_compound_metrics":      "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_sidekiq_job_stats":             "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_sidekiq_process_metrics":       "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_sidekiq_queue_metrics":         "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_get_usage_queries":                 "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_group_member_add":                  "ActionSpec schema preserves optional member fields while historical individual snapshot marks them required",
	"gitlab_group_share":                       "ActionSpec schema preserves optional expires_at while historical individual snapshot marks it required",
	"gitlab_group_variable_create":             "ActionSpec schema preserves optional variable controls while historical individual snapshot marks them required",
	"gitlab_instance_variable_create":          "ActionSpec schema preserves optional variable controls while historical individual snapshot marks them required",
	"gitlab_issue_link_create":                 "ActionSpec schema preserves optional target_project_id while historical individual snapshot marks it required",
	"gitlab_job_download_artifacts":            "ActionSpec schema preserves optional artifact inputs while historical individual snapshot marks them required",
	"gitlab_list_alert_metric_images":          "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_cluster_agent_tokens":         "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_cluster_agents":               "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_emails":                       "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_list_emails_for_user":              "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_error_tracking_client_keys":   "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_feature_definitions":          "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_list_features":                     "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_list_gpg_keys":                     "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_list_instance_member_roles":        "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_project_aliases":              "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_project_templates":            "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_secure_files":                 "ActionSpec schema preserves optional pagination shape differently from the historical individual snapshot",
	"gitlab_list_system_hooks":                 "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_mr_approval_rule_create":           "ActionSpec schema preserves optional approver fields while historical individual snapshot marks them required",
	"gitlab_mr_approval_rule_update":           "ActionSpec schema preserves optional approver fields while historical individual snapshot marks them required",
	"gitlab_mr_dependency_create":              "ActionSpec schema preserves dependency ID requirements differently from the historical individual snapshot",
	"gitlab_mr_dependency_delete":              "ActionSpec schema preserves dependency ID requirements differently from the historical individual snapshot",
	"gitlab_notification_global_get":           "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_pages_domain_list_all":             "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_personal_access_token_get":         "ActionSpec schema preserves token ID shape differently from the historical individual snapshot",
	"gitlab_personal_access_token_revoke_self": "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_pipeline_create":                   "ActionSpec schema preserves optional pipeline inputs while historical individual snapshot marks them required",
	"gitlab_pipeline_trigger_create":           "ActionSpec schema preserves optional description while historical individual snapshot marks it required",
	"gitlab_pipeline_trigger_run":              "ActionSpec schema preserves optional variables while historical individual snapshot marks them required",
	"gitlab_project_member_add":                "ActionSpec schema preserves optional member fields while historical individual snapshot marks them required",
	"gitlab_project_member_edit":               "ActionSpec schema preserves optional member fields while historical individual snapshot marks them required",
	"gitlab_project_snippet_create":            "ActionSpec schema preserves optional snippet fields while historical individual snapshot marks them required",
	"gitlab_runner_reset_instance_reg_token":   "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_search_code":                       "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_commits":                    "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_issues":                     "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_merge_requests":             "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_milestones":                 "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_notes":                      "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_projects":                   "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_snippets":                   "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_users":                      "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_search_wiki":                       "ActionSpec schema preserves search_type enum metadata differently from the historical individual snapshot",
	"gitlab_set_feature_flag":                  "ActionSpec schema preserves optional feature flag gates while historical individual snapshot marks them required",
	"gitlab_snippet_create":                    "ActionSpec schema preserves optional snippet fields while historical individual snapshot marks them required",
	"gitlab_summarize_issue":                   "ActionSpec schema preserves required sampling fields differently from the historical individual snapshot",
	"gitlab_todo_mark_all_done":                "ActionSpec schema preserves no-input shape differently from the historical individual snapshot",
	"gitlab_update_alert_metric_image":         "ActionSpec schema preserves optional URL fields while historical individual snapshot marks them required",
	"gitlab_update_settings":                   "ActionSpec schema preserves settings map shape differently from the historical individual snapshot",
	"gitlab_upload_alert_metric_image":         "ActionSpec schema preserves optional url_text while historical individual snapshot marks it required",
	"gitlab_user_current":                      "shared ActionSpec projections differ from the historical individual snapshot",
}

func assertActionRouteParity(t *testing.T, toolName string, captured, specRoutes toolutil.ActionMap) {
	t.Helper()
	if len(specRoutes) != len(captured) {
		t.Fatalf("%s specs route count = %d, want %d; missing: %v", toolName, len(specRoutes), len(captured), missingRouteNames(captured, specRoutes))
	}
	for actionName, capturedRoute := range captured {
		specRoute, ok := specRoutes[actionName]
		if !ok {
			t.Fatalf("%s spec routes missing action %q", toolName, actionName)
		}
		if specRoute.Destructive != capturedRoute.Destructive {
			t.Fatalf("%s.%s destructive = %t, want %t", toolName, actionName, specRoute.Destructive, capturedRoute.Destructive)
		}
		if specRoute.InputSchema == nil {
			t.Fatalf("%s.%s missing input schema", toolName, actionName)
		}
		if specRoute.OutputSchema == nil {
			t.Fatalf("%s.%s missing output schema", toolName, actionName)
		}
	}
}

func assertSpecProjectionParity(t *testing.T, toolName string, specs []toolutil.ActionSpec) {
	t.Helper()
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{ToolName: toolName}, specs)
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	if len(group.Actions) != len(specs) {
		t.Fatalf("%s projected action count = %d, want %d", toolName, len(group.Actions), len(specs))
	}
	for _, spec := range specs {
		action, ok := group.Actions[spec.Name]
		if !ok {
			t.Fatalf("%s projection missing action %q", toolName, spec.Name)
		}
		if !action.SpecBacked {
			t.Fatalf("%s.%s projection is not spec-backed", toolName, spec.Name)
		}
		if action.ReadOnly != spec.ReadOnly {
			t.Fatalf("%s.%s read-only = %t, want %t", toolName, spec.Name, action.ReadOnly, spec.ReadOnly)
		}
		if strings.TrimSpace(spec.IndividualTool.Name) == "" {
			t.Fatalf("%s.%s missing individual tool metadata", toolName, spec.Name)
		}
	}
}

func assertGuidanceKeys(t *testing.T, toolName, actionName string, guidance map[string]toolutil.ParameterGuidance, want []string) {
	t.Helper()
	got := make([]string, 0, len(guidance))
	for key := range guidance {
		got = append(got, key)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s.%s guidance keys = %v, want %v", toolName, actionName, got, want)
	}
}

func assertProjectedToolParity(t *testing.T, toolName, actionName string, actual, projected *mcp.Tool) {
	t.Helper()
	if projected.Name != actual.Name {
		t.Fatalf("%s.%s projected name = %q, want %q", toolName, actionName, projected.Name, actual.Name)
	}
	if projected.Title != actual.Title {
		t.Fatalf("%s.%s projected title = %q, want %q", toolName, actionName, projected.Title, actual.Title)
	}
	if projected.Description != actual.Description {
		t.Fatalf("%s.%s projected description drift", toolName, actionName)
	}
	if projected.InputSchema == nil {
		t.Fatalf("%s.%s projected input schema is nil", toolName, actionName)
	}
	if projected.OutputSchema == nil {
		t.Fatalf("%s.%s projected output schema is nil", toolName, actionName)
	}
	assertProjectedToolAnnotations(t, toolName, actionName, projected.Annotations)
	assertToolIconsParity(t, toolName, actionName, actual.Icons, projected.Icons)
}

func assertProjectedToolAnnotations(t *testing.T, toolName, actionName string, projected *mcp.ToolAnnotations) {
	t.Helper()
	if projected == nil {
		t.Fatalf("%s.%s projected annotations are nil", toolName, actionName)
	}
	if projected.DestructiveHint == nil {
		t.Fatalf("%s.%s projected destructive annotation is nil", toolName, actionName)
	}
	if projected.OpenWorldHint == nil {
		t.Fatalf("%s.%s projected open-world annotation is nil", toolName, actionName)
	}
	if projected.ReadOnlyHint && *projected.DestructiveHint {
		t.Fatalf("%s.%s projected annotations are both read-only and destructive", toolName, actionName)
	}
}

func assertToolIconsParity(t *testing.T, toolName, actionName string, actual, projected []mcp.Icon) {
	t.Helper()
	if len(projected) != len(actual) {
		t.Fatalf("%s.%s projected icon count = %d, want %d", toolName, actionName, len(projected), len(actual))
	}
	for i := range actual {
		if projected[i].Source != actual[i].Source || projected[i].MIMEType != actual[i].MIMEType || strings.Join(projected[i].Sizes, ",") != strings.Join(actual[i].Sizes, ",") || projected[i].Theme != actual[i].Theme {
			t.Fatalf("%s.%s projected icon[%d] = %+v, want %+v", toolName, actionName, i, projected[i], actual[i])
		}
	}
}

func missingRouteNames(want, got toolutil.ActionMap) []string {
	missing := make([]string, 0)
	for actionName := range want {
		if _, ok := got[actionName]; !ok {
			missing = append(missing, actionName)
		}
	}
	sort.Strings(missing)
	return missing
}
