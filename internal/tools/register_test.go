// register_test.go contains unit tests for tool registration via RegisterAll
// and RegisterAllMeta. Tests verify tool counts, tool names, annotation
// presence, and end-to-end MCP call flow using in-memory transports.

package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// fmtListToolsErr is the format string used when ListTools returns an error.
	fmtListToolsErr = "ListTools() error: %v"
)

// newMCPSession creates an MCP session with individual tools registered.
// When enterprise is true, Enterprise/Premium tools are included.
func newMCPSession(t *testing.T, handler http.Handler, enterprise ...bool) *mcp.ClientSession {
	t.Helper()
	client := newTestClient(t, handler)

	ent := true
	if len(enterprise) > 0 {
		ent = enterprise[0]
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
	RegisterAll(server, client, ent)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// newMetaMCPSession creates an MCP session with meta-tools registered.
// When enterprise is true, Enterprise/Premium meta-tools are included.
// It uses in-memory transports and auto-closes the session via t.Cleanup.
func newMetaMCPSession(t *testing.T, handler http.Handler, enterprise bool) *mcp.ClientSession {
	t.Helper()
	client := newTestClient(t, handler)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterAllMeta(server, client, enterprise)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestRegisterAll_ToolCount verifies that RegisterAll registers exactly
// the expected number of individual tools on the MCP server.
func TestRegisterAll_ToolCount(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	t.Run("Enterprise", func(t *testing.T) {
		session := newMCPSession(t, handler, true)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		const expectedTools = 1006
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})

	t.Run("CE", func(t *testing.T) {
		session := newMCPSession(t, handler, false)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		t.Logf("CE tool count: %d", len(result.Tools))
		const expectedTools = 857
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})
}

// TestRegisterAllMeta_ToolCount verifies that RegisterAllMeta registers
// the expected number of meta-tools: 28 base, 43 with enterprise.
func TestRegisterAllMeta_ToolCount(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	t.Run("Base", func(t *testing.T) {
		session := newMetaMCPSession(t, handler, false)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		const expectedTools = 28
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})

	t.Run("Enterprise", func(t *testing.T) {
		session := newMetaMCPSession(t, handler, true)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		const expectedTools = 43
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})
}

// TestRegisterAll_ToolNames verifies that every expected individual tool name
// is present after RegisterAll and that no unexpected tools are registered.
func TestRegisterAll_ToolNames(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	expectedNames := map[string]bool{
		"gitlab_access_request_approve_group":         true,
		"gitlab_access_request_approve_project":       true,
		"gitlab_access_request_deny_group":            true,
		"gitlab_access_request_deny_project":          true,
		"gitlab_access_request_list_group":            true,
		"gitlab_access_request_list_project":          true,
		"gitlab_access_request_request_group":         true,
		"gitlab_access_request_request_project":       true,
		"gitlab_activate_user":                        true,
		"gitlab_add_commit_discussion_note":           true,
		"gitlab_add_email":                            true,
		"gitlab_add_email_for_user":                   true,
		"gitlab_add_epic_discussion_note":             true,
		"gitlab_add_gpg_key":                          true,
		"gitlab_add_gpg_key_for_user":                 true,
		"gitlab_add_group_badge":                      true,
		"gitlab_add_group_job_token_allowlist":        true,
		"gitlab_add_issue_discussion_note":            true,
		"gitlab_add_license":                          true,
		"gitlab_add_merge_request_to_merge_train":     true,
		"gitlab_add_project_badge":                    true,
		"gitlab_add_project_job_token_allowlist":      true,
		"gitlab_add_project_mirror":                   true,
		"gitlab_add_snippet_discussion_note":          true,
		"gitlab_add_ssh_key":                          true,
		"gitlab_add_ssh_key_for_user":                 true,
		"gitlab_add_system_hook":                      true,
		"gitlab_analyze_ci_configuration":             true,
		"gitlab_analyze_deployment_history":           true,
		"gitlab_analyze_issue_scope":                  true,
		"gitlab_analyze_mr_changes":                   true,
		"gitlab_analyze_pipeline_failure":             true,
		"gitlab_approve_user":                         true,
		"gitlab_ban_user":                             true,
		"gitlab_block_user":                           true,
		"gitlab_board_create":                         true,
		"gitlab_board_delete":                         true,
		"gitlab_board_get":                            true,
		"gitlab_board_list":                           true,
		"gitlab_board_list_create":                    true,
		"gitlab_board_list_delete":                    true,
		"gitlab_board_list_get":                       true,
		"gitlab_board_list_lists":                     true,
		"gitlab_board_list_update":                    true,
		"gitlab_board_update":                         true,
		"gitlab_branch_create":                        true,
		"gitlab_branch_delete":                        true,
		"gitlab_branch_delete_merged":                 true,
		"gitlab_branch_get":                           true,
		"gitlab_branch_list":                          true,
		"gitlab_branch_protect":                       true,
		"gitlab_branch_unprotect":                     true,
		"gitlab_cancel_github_import":                 true,
		"gitlab_change_plan_limits":                   true,
		"gitlab_ci_lint":                              true,
		"gitlab_ci_lint_project":                      true,
		"gitlab_ci_variable_create":                   true,
		"gitlab_ci_variable_delete":                   true,
		"gitlab_ci_variable_get":                      true,
		"gitlab_ci_variable_list":                     true,
		"gitlab_ci_variable_update":                   true,
		"gitlab_commit_cherry_pick":                   true,
		"gitlab_commit_comment_create":                true,
		"gitlab_commit_comments":                      true,
		"gitlab_commit_create":                        true,
		"gitlab_commit_diff":                          true,
		"gitlab_commit_get":                           true,
		"gitlab_commit_list":                          true,
		"gitlab_commit_merge_requests":                true,
		"gitlab_commit_refs":                          true,
		"gitlab_commit_revert":                        true,
		"gitlab_commit_signature":                     true,
		"gitlab_commit_status_set":                    true,
		"gitlab_commit_statuses":                      true,
		"gitlab_confirm_vulnerability":                true,
		"gitlab_create_application":                   true,
		"gitlab_create_broadcast_message":             true,
		"gitlab_create_cluster_agent_token":           true,
		"gitlab_create_commit_discussion":             true,
		"gitlab_create_current_user_pat":              true,
		"gitlab_create_custom_emoji":                  true,
		"gitlab_create_dependency_list_export":        true,
		"gitlab_create_epic_discussion":               true,
		"gitlab_create_error_tracking_client_key":     true,
		"gitlab_create_external_status_check":         true,
		"gitlab_create_freeze_period":                 true,
		"gitlab_create_geo_site":                      true,
		"gitlab_create_group_member_role":             true,
		"gitlab_create_group_ssh_certificate":         true,
		"gitlab_create_impersonation_token":           true,
		"gitlab_create_instance_member_role":          true,
		"gitlab_create_issue_discussion":              true,
		"gitlab_create_mr_context_commits":            true,
		"gitlab_create_package_protection_rule":       true,
		"gitlab_create_personal_access_token":         true,
		"gitlab_create_project_alias":                 true,
		"gitlab_create_project_external_status_check": true,
		"gitlab_create_secure_file":                   true,
		"gitlab_create_service_account":               true,
		"gitlab_create_snippet_discussion":            true,
		"gitlab_create_topic":                         true,
		"gitlab_create_user":                          true,
		"gitlab_create_user_runner":                   true,

		"gitlab_create_work_item":                       true,
		"gitlab_current_user_status":                    true,
		"gitlab_deactivate_user":                        true,
		"gitlab_delete_alert_metric_image":              true,
		"gitlab_delete_application":                     true,
		"gitlab_delete_broadcast_message":               true,
		"gitlab_delete_cluster_agent":                   true,
		"gitlab_delete_commit_discussion_note":          true,
		"gitlab_delete_custom_attribute":                true,
		"gitlab_delete_custom_emoji":                    true,
		"gitlab_delete_email":                           true,
		"gitlab_delete_email_for_user":                  true,
		"gitlab_delete_enterprise_user":                 true,
		"gitlab_delete_epic_discussion_note":            true,
		"gitlab_delete_error_tracking_client_key":       true,
		"gitlab_delete_external_status_check":           true,
		"gitlab_delete_feature_flag":                    true,
		"gitlab_delete_freeze_period":                   true,
		"gitlab_delete_geo_site":                        true,
		"gitlab_delete_gpg_key":                         true,
		"gitlab_delete_gpg_key_for_user":                true,
		"gitlab_delete_group_badge":                     true,
		"gitlab_delete_group_markdown_upload_by_id":     true,
		"gitlab_delete_group_markdown_upload_by_secret": true,
		"gitlab_delete_group_member_role":               true,
		"gitlab_delete_group_scim_identity":             true,
		"gitlab_delete_group_ssh_certificate":           true,
		"gitlab_delete_group_ssh_key":                   true,
		"gitlab_delete_instance_member_role":            true,
		"gitlab_delete_integration":                     true,
		"gitlab_delete_issue_discussion_note":           true,
		"gitlab_delete_license":                         true,
		"gitlab_delete_mr_context_commits":              true,
		"gitlab_delete_package_protection_rule":         true,
		"gitlab_delete_project_alias":                   true,
		"gitlab_delete_project_badge":                   true,
		"gitlab_delete_project_external_status_check":   true,
		"gitlab_delete_project_mirror":                  true,
		"gitlab_delete_snippet_discussion_note":         true,
		"gitlab_delete_ssh_key":                         true,
		"gitlab_delete_ssh_key_for_user":                true,
		"gitlab_delete_system_hook":                     true,
		"gitlab_delete_terraform_state":                 true,
		"gitlab_delete_terraform_state_version":         true,
		"gitlab_delete_topic":                           true,
		"gitlab_delete_user":                            true,
		"gitlab_delete_user_identity":                   true,
		"gitlab_delete_work_item":                       true,
		"gitlab_deploy_key_add":                         true,
		"gitlab_deploy_key_add_instance":                true,
		"gitlab_deploy_key_delete":                      true,
		"gitlab_deploy_key_enable":                      true,
		"gitlab_deploy_key_get":                         true,
		"gitlab_deploy_key_list_all":                    true,
		"gitlab_deploy_key_list_project":                true,
		"gitlab_deploy_key_list_user_project":           true,
		"gitlab_deploy_key_update":                      true,
		"gitlab_deploy_token_create_group":              true,
		"gitlab_deploy_token_create_project":            true,
		"gitlab_deploy_token_delete_group":              true,
		"gitlab_deploy_token_delete_project":            true,
		"gitlab_deploy_token_get_group":                 true,
		"gitlab_deploy_token_get_project":               true,
		"gitlab_deploy_token_list_all":                  true,
		"gitlab_deploy_token_list_group":                true,
		"gitlab_deploy_token_list_project":              true,
		"gitlab_deployment_approve_or_reject":           true,
		"gitlab_deployment_create":                      true,
		"gitlab_deployment_delete":                      true,
		"gitlab_deployment_get":                         true,
		"gitlab_deployment_list":                        true,
		"gitlab_deployment_update":                      true,
		"gitlab_disable_2fa_enterprise_user":            true,
		"gitlab_disable_two_factor":                     true,
		"gitlab_dismiss_vulnerability":                  true,
		"gitlab_download_attestation":                   true,
		"gitlab_download_dependency_list_export":        true,
		"gitlab_download_group_export":                  true,
		"gitlab_download_ml_model_package":              true,
		"gitlab_download_project_export":                true,
		"gitlab_edit_geo_site":                          true,
		"gitlab_edit_group_badge":                       true,
		"gitlab_edit_project_badge":                     true,
		"gitlab_edit_project_mirror":                    true,
		"gitlab_edit_resource_group":                    true,
		"gitlab_enable_disable_error_tracking":          true,
		"gitlab_environment_create":                     true,
		"gitlab_environment_delete":                     true,
		"gitlab_environment_get":                        true,
		"gitlab_environment_list":                       true,
		"gitlab_environment_stop":                       true,
		"gitlab_environment_update":                     true,
		"gitlab_epic_create":                            true,
		"gitlab_epic_delete":                            true,
		"gitlab_epic_get":                               true,
		"gitlab_epic_get_links":                         true,
		"gitlab_epic_issue_assign":                      true,
		"gitlab_epic_issue_list":                        true,
		"gitlab_epic_issue_remove":                      true,
		"gitlab_epic_issue_update":                      true,
		"gitlab_epic_list":                              true,
		"gitlab_epic_note_create":                       true,
		"gitlab_epic_note_delete":                       true,
		"gitlab_epic_note_get":                          true,
		"gitlab_epic_note_list":                         true,
		"gitlab_epic_note_update":                       true,
		"gitlab_epic_update":                            true,
		"gitlab_feature_flag_create":                    true,
		"gitlab_feature_flag_delete":                    true,
		"gitlab_feature_flag_get":                       true,
		"gitlab_feature_flag_list":                      true,
		"gitlab_feature_flag_update":                    true,
		"gitlab_ff_user_list_create":                    true,
		"gitlab_ff_user_list_delete":                    true,
		"gitlab_ff_user_list_get":                       true,
		"gitlab_ff_user_list_list":                      true,
		"gitlab_ff_user_list_update":                    true,
		"gitlab_file_blame":                             true,
		"gitlab_file_create":                            true,
		"gitlab_file_delete":                            true,
		"gitlab_file_get":                               true,
		"gitlab_file_metadata":                          true,
		"gitlab_file_raw":                               true,
		"gitlab_file_raw_metadata":                      true,
		"gitlab_file_update":                            true,
		"gitlab_find_technical_debt":                    true,
		"gitlab_force_push_mirror_update":               true,
		"gitlab_generate_milestone_report":              true,
		"gitlab_generate_release_notes":                 true,
		"gitlab_get_appearance":                         true,
		"gitlab_get_application_statistics":             true,
		"gitlab_get_avatar":                             true,
		"gitlab_get_broadcast_message":                  true,
		"gitlab_get_catalog_resource":                   true,
		"gitlab_get_ci_yml_template":                    true,
		"gitlab_get_cluster_agent":                      true,
		"gitlab_get_cluster_agent_token":                true,
		"gitlab_get_commit_discussion":                  true,
		"gitlab_get_compliance_policy_settings":         true,
		"gitlab_get_custom_attribute":                   true,
		"gitlab_get_dependency_list_export":             true,
		"gitlab_get_dockerfile_template":                true,
		"gitlab_get_email":                              true,
		"gitlab_get_enterprise_user":                    true,
		"gitlab_get_epic_discussion":                    true,
		"gitlab_get_error_tracking_settings":            true,
		"gitlab_get_freeze_period":                      true,
		"gitlab_get_geo_site":                           true,
		"gitlab_get_gitignore_template":                 true,
		"gitlab_get_gpg_key":                            true,
		"gitlab_get_gpg_key_for_user":                   true,
		"gitlab_get_group_audit_event":                  true,
		"gitlab_get_group_badge":                        true,
		"gitlab_get_group_dora_metrics":                 true,
		"gitlab_get_group_issue_statistics":             true,
		"gitlab_get_group_mr_approval_settings":         true,
		"gitlab_get_group_scim_identity":                true,
		"gitlab_get_group_storage_move":                 true,
		"gitlab_get_group_storage_move_for_group":       true,
		"gitlab_get_impersonation_token":                true,
		"gitlab_get_instance_audit_event":               true,
		"gitlab_get_integration":                        true,
		"gitlab_get_issue_discussion":                   true,
		"gitlab_get_issue_statistics":                   true,
		"gitlab_get_job_token_access_settings":          true,
		"gitlab_get_key_by_fingerprint":                 true,
		"gitlab_get_key_with_user":                      true,
		"gitlab_get_license":                            true,
		"gitlab_get_license_template":                   true,
		"gitlab_get_merge_request_on_merge_train":       true,
		"gitlab_get_metadata":                           true,
		"gitlab_get_metric_definitions":                 true,
		"gitlab_get_non_sql_metrics":                    true,
		"gitlab_get_plan_limits":                        true,
		"gitlab_get_project_alias":                      true,
		"gitlab_get_project_audit_event":                true,
		"gitlab_get_project_badge":                      true,
		"gitlab_get_project_dora_metrics":               true,
		"gitlab_get_project_export_status":              true,
		"gitlab_get_project_import_status":              true,
		"gitlab_get_project_issue_statistics":           true,
		"gitlab_get_project_mirror":                     true,
		"gitlab_get_project_mirror_public_key":          true,
		"gitlab_get_project_mr_approval_settings":       true,
		"gitlab_get_project_security_settings":          true,
		"gitlab_get_project_statistics":                 true,
		"gitlab_get_project_storage_move":               true,
		"gitlab_get_project_storage_move_for_project":   true,
		"gitlab_get_project_template":                   true,
		"gitlab_get_recently_added_members_count":       true,
		"gitlab_get_recently_created_issues_count":      true,
		"gitlab_get_recently_created_mr_count":          true,
		"gitlab_get_resource_group":                     true,
		"gitlab_get_service_ping":                       true,
		"gitlab_get_settings":                           true,
		"gitlab_get_sidekiq_compound_metrics":           true,
		"gitlab_get_sidekiq_job_stats":                  true,
		"gitlab_get_sidekiq_process_metrics":            true,
		"gitlab_get_sidekiq_queue_metrics":              true,
		"gitlab_get_snippet_discussion":                 true,
		"gitlab_get_snippet_storage_move":               true,
		"gitlab_get_snippet_storage_move_for_snippet":   true,
		"gitlab_get_ssh_key":                            true,
		"gitlab_get_ssh_key_for_user":                   true,
		"gitlab_get_status_geo_site":                    true,
		"gitlab_get_system_hook":                        true,
		"gitlab_get_terraform_state":                    true,
		"gitlab_get_topic":                              true,
		"gitlab_get_usage_queries":                      true,
		"gitlab_get_user":                               true,
		"gitlab_get_user_activities":                    true,
		"gitlab_get_user_associations_count":            true,
		"gitlab_get_user_memberships":                   true,
		"gitlab_get_user_status":                        true,
		"gitlab_get_vulnerability":                      true,
		"gitlab_get_work_item":                          true,
		"gitlab_group_access_token_create":              true,
		"gitlab_group_access_token_get":                 true,
		"gitlab_group_access_token_list":                true,
		"gitlab_group_access_token_revoke":              true,
		"gitlab_group_access_token_rotate":              true,
		"gitlab_group_access_token_rotate_self":         true,
		"gitlab_group_board_create":                     true,
		"gitlab_group_board_delete":                     true,
		"gitlab_group_board_get":                        true,
		"gitlab_group_board_list":                       true,
		"gitlab_group_board_list_create":                true,
		"gitlab_group_board_list_delete":                true,
		"gitlab_group_board_list_get":                   true,
		"gitlab_group_board_list_lists":                 true,
		"gitlab_group_board_list_update":                true,
		"gitlab_group_board_update":                     true,
		"gitlab_group_create":                           true,
		"gitlab_group_delete":                           true,
		"gitlab_group_epic_board_get":                   true,
		"gitlab_group_epic_board_list":                  true,
		"gitlab_group_get":                              true,
		"gitlab_group_hook_add":                         true,
		"gitlab_group_hook_delete":                      true,
		"gitlab_group_hook_edit":                        true,
		"gitlab_group_hook_get":                         true,
		"gitlab_group_hook_list":                        true,
		"gitlab_group_invite":                           true,
		"gitlab_group_invite_list_pending":              true,
		"gitlab_group_label_create":                     true,
		"gitlab_group_label_delete":                     true,
		"gitlab_group_label_get":                        true,
		"gitlab_group_label_list":                       true,
		"gitlab_group_label_subscribe":                  true,
		"gitlab_group_label_unsubscribe":                true,
		"gitlab_group_label_update":                     true,
		"gitlab_group_ldap_link_add":                    true,
		"gitlab_group_ldap_link_delete":                 true,
		"gitlab_group_ldap_link_delete_for_provider":    true,
		"gitlab_group_ldap_link_list":                   true,
		"gitlab_group_list":                             true,
		"gitlab_group_member_add":                       true,
		"gitlab_group_member_edit":                      true,
		"gitlab_group_member_get":                       true,
		"gitlab_group_member_get_inherited":             true,
		"gitlab_group_member_remove":                    true,
		"gitlab_group_members_list":                     true,
		"gitlab_group_milestone_burndown_events":        true,
		"gitlab_group_milestone_create":                 true,
		"gitlab_group_milestone_delete":                 true,
		"gitlab_group_milestone_get":                    true,
		"gitlab_group_milestone_issues":                 true,
		"gitlab_group_milestone_list":                   true,
		"gitlab_group_milestone_merge_requests":         true,
		"gitlab_group_milestone_update":                 true,
		"gitlab_group_projects":                         true,
		"gitlab_group_protected_branch_get":             true,
		"gitlab_group_protected_branch_list":            true,
		"gitlab_group_protected_branch_protect":         true,
		"gitlab_group_protected_branch_unprotect":       true,
		"gitlab_group_protected_branch_update":          true,
		"gitlab_group_protected_environment_get":        true,
		"gitlab_group_protected_environment_list":       true,
		"gitlab_group_protected_environment_protect":    true,
		"gitlab_group_protected_environment_unprotect":  true,
		"gitlab_group_protected_environment_update":     true,
		"gitlab_group_release_list":                     true,
		"gitlab_group_restore":                          true,
		"gitlab_group_archive":                          true,
		"gitlab_group_unarchive":                        true,
		"gitlab_group_saml_link_add":                    true,
		"gitlab_group_saml_link_delete":                 true,
		"gitlab_group_saml_link_get":                    true,
		"gitlab_group_saml_link_list":                   true,
		"gitlab_group_search":                           true,
		"gitlab_group_service_account_create":           true,
		"gitlab_group_service_account_delete":           true,
		"gitlab_group_service_account_list":             true,
		"gitlab_group_service_account_pat_create":       true,
		"gitlab_group_service_account_pat_list":         true,
		"gitlab_group_service_account_pat_revoke":       true,
		"gitlab_group_service_account_update":           true,
		"gitlab_group_share":                            true,
		"gitlab_group_transfer_project":                 true,
		"gitlab_group_unshare":                          true,
		"gitlab_group_update":                           true,
		"gitlab_group_variable_create":                  true,
		"gitlab_group_variable_delete":                  true,
		"gitlab_group_variable_get":                     true,
		"gitlab_group_variable_list":                    true,
		"gitlab_group_variable_update":                  true,
		"gitlab_group_wiki_create":                      true,
		"gitlab_group_wiki_delete":                      true,
		"gitlab_group_wiki_edit":                        true,
		"gitlab_group_wiki_get":                         true,
		"gitlab_group_wiki_list":                        true,
		"gitlab_import_from_bitbucket_cloud":            true,
		"gitlab_import_from_bitbucket_server":           true,
		"gitlab_import_from_github":                     true,
		"gitlab_import_github_gists":                    true,
		"gitlab_import_group_from_file":                 true,
		"gitlab_import_project_from_file":               true,
		"gitlab_instance_variable_create":               true,
		"gitlab_instance_variable_delete":               true,
		"gitlab_instance_variable_get":                  true,
		"gitlab_instance_variable_list":                 true,
		"gitlab_instance_variable_update":               true,
		"gitlab_interactive_issue_create":               true,
		"gitlab_interactive_mr_create":                  true,
		"gitlab_interactive_project_create":             true,
		"gitlab_interactive_release_create":             true,
		"gitlab_issue_create":                           true,
		"gitlab_issue_create_todo":                      true,
		"gitlab_issue_delete":                           true,
		"gitlab_issue_emoji_create":                     true,
		"gitlab_issue_emoji_delete":                     true,
		"gitlab_issue_emoji_get":                        true,
		"gitlab_issue_emoji_list":                       true,
		"gitlab_issue_get":                              true,
		"gitlab_issue_get_by_id":                        true,
		"gitlab_issue_iteration_event_get":              true,
		"gitlab_issue_iteration_event_list":             true,
		"gitlab_issue_label_event_get":                  true,
		"gitlab_issue_label_event_list":                 true,
		"gitlab_issue_link_create":                      true,
		"gitlab_issue_link_delete":                      true,
		"gitlab_issue_link_get":                         true,
		"gitlab_issue_link_list":                        true,
		"gitlab_issue_list":                             true,
		"gitlab_issue_list_all":                         true,
		"gitlab_issue_list_group":                       true,
		"gitlab_issue_milestone_event_get":              true,
		"gitlab_issue_milestone_event_list":             true,
		"gitlab_issue_move":                             true,
		"gitlab_issue_mrs_closing":                      true,
		"gitlab_issue_mrs_related":                      true,
		"gitlab_issue_note_create":                      true,
		"gitlab_issue_note_delete":                      true,
		"gitlab_issue_note_emoji_create":                true,
		"gitlab_issue_note_emoji_delete":                true,
		"gitlab_issue_note_emoji_get":                   true,
		"gitlab_issue_note_emoji_list":                  true,
		"gitlab_issue_note_get":                         true,
		"gitlab_issue_note_list":                        true,
		"gitlab_issue_note_update":                      true,
		"gitlab_issue_participants":                     true,
		"gitlab_issue_reorder":                          true,
		"gitlab_issue_spent_time_add":                   true,
		"gitlab_issue_spent_time_reset":                 true,
		"gitlab_issue_state_event_get":                  true,
		"gitlab_issue_state_event_list":                 true,
		"gitlab_issue_subscribe":                        true,
		"gitlab_issue_time_estimate_reset":              true,
		"gitlab_issue_time_estimate_set":                true,
		"gitlab_issue_time_stats_get":                   true,
		"gitlab_issue_unsubscribe":                      true,
		"gitlab_issue_update":                           true,
		"gitlab_issue_weight_event_list":                true,
		"gitlab_job_artifacts":                          true,
		"gitlab_job_cancel":                             true,
		"gitlab_job_delete_artifacts":                   true,
		"gitlab_job_delete_project_artifacts":           true,
		"gitlab_job_download_artifacts":                 true,
		"gitlab_job_download_single_artifact":           true,
		"gitlab_job_download_single_artifact_by_ref":    true,
		"gitlab_job_erase":                              true,
		"gitlab_job_get":                                true,
		"gitlab_job_keep_artifacts":                     true,
		"gitlab_job_list":                               true,
		"gitlab_job_list_bridges":                       true,
		"gitlab_job_list_project":                       true,
		"gitlab_job_play":                               true,
		"gitlab_job_retry":                              true,
		"gitlab_job_trace":                              true,
		"gitlab_job_wait":                               true,
		"gitlab_label_create":                           true,
		"gitlab_label_delete":                           true,
		"gitlab_label_get":                              true,
		"gitlab_label_list":                             true,
		"gitlab_label_promote":                          true,
		"gitlab_label_subscribe":                        true,
		"gitlab_label_unsubscribe":                      true,
		"gitlab_label_update":                           true,
		"gitlab_list_alert_metric_images":               true,
		"gitlab_list_applications":                      true,
		"gitlab_list_attestations":                      true,
		"gitlab_list_branch_rules":                      true,
		"gitlab_list_broadcast_messages":                true,
		"gitlab_list_catalog_resources":                 true,
		"gitlab_list_ci_yml_templates":                  true,
		"gitlab_list_cluster_agent_tokens":              true,
		"gitlab_list_cluster_agents":                    true,
		"gitlab_list_commit_discussions":                true,
		"gitlab_list_custom_attributes":                 true,
		"gitlab_list_custom_emoji":                      true,
		"gitlab_list_deployment_merge_requests":         true,
		"gitlab_list_dockerfile_templates":              true,
		"gitlab_list_emails":                            true,
		"gitlab_list_emails_for_user":                   true,
		"gitlab_list_enterprise_users":                  true,
		"gitlab_list_epic_discussions":                  true,
		"gitlab_list_error_tracking_client_keys":        true,
		"gitlab_list_feature_definitions":               true,
		"gitlab_list_features":                          true,
		"gitlab_list_freeze_periods":                    true,
		"gitlab_list_geo_sites":                         true,
		"gitlab_list_gitignore_templates":               true,
		"gitlab_list_gpg_keys":                          true,
		"gitlab_list_gpg_keys_for_user":                 true,
		"gitlab_list_group_audit_events":                true,
		"gitlab_list_group_badges":                      true,
		"gitlab_list_group_iterations":                  true,
		"gitlab_list_group_markdown_uploads":            true,
		"gitlab_list_group_member_roles":                true,
		"gitlab_list_group_personal_access_tokens":      true,
		"gitlab_list_group_relations_export_status":     true,
		"gitlab_list_group_scim_identities":             true,
		"gitlab_list_group_ssh_certificates":            true,
		"gitlab_list_group_ssh_keys":                    true,
		"gitlab_list_impersonation_tokens":              true,
		"gitlab_list_instance_audit_events":             true,
		"gitlab_list_instance_member_roles":             true,
		"gitlab_list_integrations":                      true,
		"gitlab_list_issue_discussions":                 true,
		"gitlab_list_job_token_group_allowlist":         true,
		"gitlab_list_job_token_inbound_allowlist":       true,
		"gitlab_list_license_templates":                 true,
		"gitlab_list_merge_request_in_merge_train":      true,
		"gitlab_list_merge_status_checks":               true,
		"gitlab_list_mr_context_commits":                true,
		"gitlab_list_package_protection_rules":          true,
		"gitlab_list_project_aliases":                   true,
		"gitlab_list_project_audit_events":              true,
		"gitlab_list_project_badges":                    true,
		"gitlab_list_project_dependencies":              true,
		"gitlab_list_project_external_status_checks":    true,
		"gitlab_list_project_iterations":                true,
		"gitlab_list_project_merge_trains":              true,
		"gitlab_list_project_mirrors":                   true,
		"gitlab_list_project_mr_external_status_checks": true,
		"gitlab_list_project_status_checks":             true,
		"gitlab_list_project_templates":                 true,

		"gitlab_list_repository_submodules":                        true,
		"gitlab_list_resource_group_upcoming_jobs":                 true,
		"gitlab_list_resource_groups":                              true,
		"gitlab_list_secure_files":                                 true,
		"gitlab_list_security_findings":                            true,
		"gitlab_list_service_accounts":                             true,
		"gitlab_list_snippet_discussions":                          true,
		"gitlab_list_ssh_keys":                                     true,
		"gitlab_list_ssh_keys_for_user":                            true,
		"gitlab_list_status_all_geo_sites":                         true,
		"gitlab_list_system_hooks":                                 true,
		"gitlab_list_terraform_states":                             true,
		"gitlab_list_topics":                                       true,
		"gitlab_list_user_contribution_events":                     true,
		"gitlab_list_users":                                        true,
		"gitlab_list_vulnerabilities":                              true,
		"gitlab_list_work_items":                                   true,
		"gitlab_lock_terraform_state":                              true,
		"gitlab_mark_migration":                                    true,
		"gitlab_server_status":                                     true,
		"gitlab_milestone_create":                                  true,
		"gitlab_milestone_delete":                                  true,
		"gitlab_milestone_get":                                     true,
		"gitlab_milestone_issues":                                  true,
		"gitlab_milestone_list":                                    true,
		"gitlab_milestone_merge_requests":                          true,
		"gitlab_milestone_update":                                  true,
		"gitlab_modify_user":                                       true,
		"gitlab_mr_add_spent_time":                                 true,
		"gitlab_mr_approval_config":                                true,
		"gitlab_mr_approval_reset":                                 true,
		"gitlab_mr_approval_rule_create":                           true,
		"gitlab_mr_approval_rule_delete":                           true,
		"gitlab_mr_approval_rule_update":                           true,
		"gitlab_mr_approval_rules":                                 true,
		"gitlab_mr_approval_state":                                 true,
		"gitlab_mr_approve":                                        true,
		"gitlab_mr_cancel_auto_merge":                              true,
		"gitlab_mr_changes_get":                                    true,
		"gitlab_mr_commits":                                        true,
		"gitlab_mr_create":                                         true,
		"gitlab_mr_create_pipeline":                                true,
		"gitlab_mr_create_todo":                                    true,
		"gitlab_mr_delete":                                         true,
		"gitlab_mr_dependencies_list":                              true,
		"gitlab_mr_dependency_create":                              true,
		"gitlab_mr_dependency_delete":                              true,
		"gitlab_mr_diff_version_get":                               true,
		"gitlab_mr_diff_versions_list":                             true,
		"gitlab_mr_discussion_create":                              true,
		"gitlab_mr_discussion_get":                                 true,
		"gitlab_mr_discussion_list":                                true,
		"gitlab_mr_discussion_note_delete":                         true,
		"gitlab_mr_discussion_note_update":                         true,
		"gitlab_mr_discussion_reply":                               true,
		"gitlab_mr_discussion_resolve":                             true,
		"gitlab_mr_draft_note_create":                              true,
		"gitlab_mr_draft_note_delete":                              true,
		"gitlab_mr_draft_note_get":                                 true,
		"gitlab_mr_draft_note_list":                                true,
		"gitlab_mr_draft_note_publish":                             true,
		"gitlab_mr_draft_note_publish_all":                         true,
		"gitlab_mr_draft_note_update":                              true,
		"gitlab_mr_emoji_create":                                   true,
		"gitlab_mr_emoji_delete":                                   true,
		"gitlab_mr_emoji_get":                                      true,
		"gitlab_mr_emoji_list":                                     true,
		"gitlab_mr_get":                                            true,
		"gitlab_mr_issues_closed":                                  true,
		"gitlab_mr_label_event_get":                                true,
		"gitlab_mr_label_event_list":                               true,
		"gitlab_mr_list":                                           true,
		"gitlab_mr_list_global":                                    true,
		"gitlab_mr_list_group":                                     true,
		"gitlab_mr_merge":                                          true,
		"gitlab_mr_milestone_event_get":                            true,
		"gitlab_mr_milestone_event_list":                           true,
		"gitlab_mr_note_create":                                    true,
		"gitlab_mr_note_delete":                                    true,
		"gitlab_mr_note_emoji_create":                              true,
		"gitlab_mr_note_emoji_delete":                              true,
		"gitlab_mr_note_emoji_get":                                 true,
		"gitlab_mr_note_emoji_list":                                true,
		"gitlab_mr_note_get":                                       true,
		"gitlab_mr_note_update":                                    true,
		"gitlab_mr_notes_list":                                     true,
		"gitlab_mr_participants":                                   true,
		"gitlab_mr_pipelines":                                      true,
		"gitlab_mr_raw_diffs":                                      true,
		"gitlab_mr_rebase":                                         true,
		"gitlab_mr_related_issues":                                 true,
		"gitlab_mr_reset_spent_time":                               true,
		"gitlab_mr_reset_time_estimate":                            true,
		"gitlab_mr_reviewers":                                      true,
		"gitlab_mr_set_time_estimate":                              true,
		"gitlab_mr_state_event_get":                                true,
		"gitlab_mr_state_event_list":                               true,
		"gitlab_mr_subscribe":                                      true,
		"gitlab_mr_time_stats":                                     true,
		"gitlab_mr_unapprove":                                      true,
		"gitlab_mr_unsubscribe":                                    true,
		"gitlab_mr_update":                                         true,
		"gitlab_namespace_exists":                                  true,
		"gitlab_namespace_get":                                     true,
		"gitlab_namespace_list":                                    true,
		"gitlab_namespace_search":                                  true,
		"gitlab_notification_global_get":                           true,
		"gitlab_notification_global_update":                        true,
		"gitlab_notification_group_get":                            true,
		"gitlab_notification_group_update":                         true,
		"gitlab_notification_project_get":                          true,
		"gitlab_notification_project_update":                       true,
		"gitlab_package_delete":                                    true,
		"gitlab_package_download":                                  true,
		"gitlab_package_file_delete":                               true,
		"gitlab_package_file_list":                                 true,
		"gitlab_package_list":                                      true,
		"gitlab_package_publish":                                   true,
		"gitlab_package_publish_and_link":                          true,
		"gitlab_package_publish_directory":                         true,
		"gitlab_pages_domain_create":                               true,
		"gitlab_pages_domain_delete":                               true,
		"gitlab_pages_domain_get":                                  true,
		"gitlab_pages_domain_list":                                 true,
		"gitlab_pages_domain_list_all":                             true,
		"gitlab_pages_domain_update":                               true,
		"gitlab_pages_get":                                         true,
		"gitlab_pages_unpublish":                                   true,
		"gitlab_pages_update":                                      true,
		"gitlab_patch_job_token_access_settings":                   true,
		"gitlab_personal_access_token_get":                         true,
		"gitlab_personal_access_token_list":                        true,
		"gitlab_personal_access_token_revoke":                      true,
		"gitlab_personal_access_token_revoke_self":                 true,
		"gitlab_personal_access_token_rotate":                      true,
		"gitlab_personal_access_token_rotate_self":                 true,
		"gitlab_pipeline_cancel":                                   true,
		"gitlab_pipeline_create":                                   true,
		"gitlab_pipeline_delete":                                   true,
		"gitlab_pipeline_get":                                      true,
		"gitlab_pipeline_latest":                                   true,
		"gitlab_pipeline_list":                                     true,
		"gitlab_pipeline_retry":                                    true,
		"gitlab_pipeline_security_summary":                         true,
		"gitlab_pipeline_schedule_create":                          true,
		"gitlab_pipeline_schedule_create_variable":                 true,
		"gitlab_pipeline_schedule_delete":                          true,
		"gitlab_pipeline_schedule_delete_variable":                 true,
		"gitlab_pipeline_schedule_edit_variable":                   true,
		"gitlab_pipeline_schedule_get":                             true,
		"gitlab_pipeline_schedule_list":                            true,
		"gitlab_pipeline_schedule_list_triggered_pipelines":        true,
		"gitlab_pipeline_schedule_run":                             true,
		"gitlab_pipeline_schedule_take_ownership":                  true,
		"gitlab_pipeline_schedule_update":                          true,
		"gitlab_pipeline_test_report":                              true,
		"gitlab_pipeline_test_report_summary":                      true,
		"gitlab_pipeline_trigger_create":                           true,
		"gitlab_pipeline_trigger_delete":                           true,
		"gitlab_pipeline_trigger_get":                              true,
		"gitlab_pipeline_trigger_list":                             true,
		"gitlab_pipeline_trigger_run":                              true,
		"gitlab_pipeline_trigger_update":                           true,
		"gitlab_pipeline_update_metadata":                          true,
		"gitlab_pipeline_variables":                                true,
		"gitlab_pipeline_wait":                                     true,
		"gitlab_preview_group_badge":                               true,
		"gitlab_preview_project_badge":                             true,
		"gitlab_project_access_token_create":                       true,
		"gitlab_project_access_token_get":                          true,
		"gitlab_project_access_token_list":                         true,
		"gitlab_project_access_token_revoke":                       true,
		"gitlab_project_access_token_rotate":                       true,
		"gitlab_project_access_token_rotate_self":                  true,
		"gitlab_project_add_push_rule":                             true,
		"gitlab_project_approval_config_change":                    true,
		"gitlab_project_approval_config_get":                       true,
		"gitlab_project_approval_rule_create":                      true,
		"gitlab_project_approval_rule_delete":                      true,
		"gitlab_project_approval_rule_get":                         true,
		"gitlab_project_approval_rule_list":                        true,
		"gitlab_project_approval_rule_update":                      true,
		"gitlab_project_archive":                                   true,
		"gitlab_project_create":                                    true,
		"gitlab_project_create_for_user":                           true,
		"gitlab_project_create_fork_relation":                      true,
		"gitlab_project_delete":                                    true,
		"gitlab_project_delete_fork_relation":                      true,
		"gitlab_project_delete_push_rule":                          true,
		"gitlab_project_delete_shared_group":                       true,
		"gitlab_project_download_avatar":                           true,
		"gitlab_project_edit_push_rule":                            true,
		"gitlab_project_event_list":                                true,
		"gitlab_project_fork":                                      true,
		"gitlab_project_get":                                       true,
		"gitlab_project_get_push_rules":                            true,
		"gitlab_project_hook_add":                                  true,
		"gitlab_project_hook_delete":                               true,
		"gitlab_project_hook_delete_custom_header":                 true,
		"gitlab_project_hook_delete_url_variable":                  true,
		"gitlab_project_hook_edit":                                 true,
		"gitlab_project_hook_get":                                  true,
		"gitlab_project_hook_list":                                 true,
		"gitlab_project_hook_set_custom_header":                    true,
		"gitlab_project_hook_set_url_variable":                     true,
		"gitlab_project_hook_test":                                 true,
		"gitlab_project_invite":                                    true,
		"gitlab_project_invite_list_pending":                       true,
		"gitlab_project_languages":                                 true,
		"gitlab_project_list":                                      true,
		"gitlab_project_list_forks":                                true,
		"gitlab_project_list_groups":                               true,
		"gitlab_project_list_invited_groups":                       true,
		"gitlab_project_list_starrers":                             true,
		"gitlab_project_list_user_contributed":                     true,
		"gitlab_project_list_user_projects":                        true,
		"gitlab_project_list_user_starred":                         true,
		"gitlab_project_list_users":                                true,
		"gitlab_project_member_add":                                true,
		"gitlab_project_member_delete":                             true,
		"gitlab_project_member_edit":                               true,
		"gitlab_project_member_get":                                true,
		"gitlab_project_member_get_inherited":                      true,
		"gitlab_project_members_list":                              true,
		"gitlab_project_pull_mirror_configure":                     true,
		"gitlab_project_pull_mirror_get":                           true,
		"gitlab_project_repository_storage_get":                    true,
		"gitlab_project_restore":                                   true,
		"gitlab_project_share_with_group":                          true,
		"gitlab_project_snippet_content":                           true,
		"gitlab_project_snippet_create":                            true,
		"gitlab_project_snippet_delete":                            true,
		"gitlab_project_snippet_get":                               true,
		"gitlab_project_snippet_list":                              true,
		"gitlab_project_snippet_update":                            true,
		"gitlab_project_star":                                      true,
		"gitlab_project_start_housekeeping":                        true,
		"gitlab_project_start_mirroring":                           true,
		"gitlab_project_transfer":                                  true,
		"gitlab_project_unarchive":                                 true,
		"gitlab_project_unstar":                                    true,
		"gitlab_project_update":                                    true,
		"gitlab_project_upload":                                    true,
		"gitlab_project_upload_avatar":                             true,
		"gitlab_project_upload_delete":                             true,
		"gitlab_project_upload_list":                               true,
		"gitlab_protected_branch_get":                              true,
		"gitlab_protected_branch_update":                           true,
		"gitlab_protected_branches_list":                           true,
		"gitlab_protected_environment_get":                         true,
		"gitlab_protected_environment_list":                        true,
		"gitlab_protected_environment_protect":                     true,
		"gitlab_protected_environment_unprotect":                   true,
		"gitlab_protected_environment_update":                      true,
		"gitlab_purge_dependency_proxy":                            true,
		"gitlab_read_repository_submodule_file":                    true,
		"gitlab_register_cluster_agent":                            true,
		"gitlab_registry_delete_repository":                        true,
		"gitlab_registry_delete_tag":                               true,
		"gitlab_registry_delete_tags_bulk":                         true,
		"gitlab_registry_get_repository":                           true,
		"gitlab_registry_get_tag":                                  true,
		"gitlab_registry_list_group":                               true,
		"gitlab_registry_list_project":                             true,
		"gitlab_registry_list_tags":                                true,
		"gitlab_registry_protection_create":                        true,
		"gitlab_registry_protection_delete":                        true,
		"gitlab_registry_protection_list":                          true,
		"gitlab_registry_protection_update":                        true,
		"gitlab_reject_user":                                       true,
		"gitlab_release_create":                                    true,
		"gitlab_release_delete":                                    true,
		"gitlab_release_get":                                       true,
		"gitlab_release_latest":                                    true,
		"gitlab_release_link_create":                               true,
		"gitlab_release_link_create_batch":                         true,
		"gitlab_release_link_delete":                               true,
		"gitlab_release_link_get":                                  true,
		"gitlab_release_link_list":                                 true,
		"gitlab_release_link_update":                               true,
		"gitlab_release_list":                                      true,
		"gitlab_release_update":                                    true,
		"gitlab_remove_group_job_token_allowlist":                  true,
		"gitlab_remove_project_job_token_allowlist":                true,
		"gitlab_remove_secure_file":                                true,
		"gitlab_render_markdown":                                   true,
		"gitlab_repair_geo_site":                                   true,
		"gitlab_repository_archive":                                true,
		"gitlab_repository_blob":                                   true,
		"gitlab_repository_changelog_add":                          true,
		"gitlab_repository_changelog_generate":                     true,
		"gitlab_repository_compare":                                true,
		"gitlab_repository_contributors":                           true,
		"gitlab_repository_merge_base":                             true,
		"gitlab_repository_raw_blob":                               true,
		"gitlab_repository_tree":                                   true,
		"gitlab_discover_project":                                  true,
		"gitlab_resolve_vulnerability":                             true,
		"gitlab_retrieve_all_group_storage_moves":                  true,
		"gitlab_retrieve_all_project_storage_moves":                true,
		"gitlab_retrieve_all_snippet_storage_moves":                true,
		"gitlab_retrieve_group_storage_moves":                      true,
		"gitlab_retrieve_project_storage_moves":                    true,
		"gitlab_retrieve_snippet_storage_moves":                    true,
		"gitlab_retry_failed_external_status_check_for_project_mr": true,
		"gitlab_retry_failed_status_check_for_mr":                  true,
		"gitlab_revert_vulnerability":                              true,
		"gitlab_review_mr_security":                                true,
		"gitlab_revoke_cluster_agent_token":                        true,
		"gitlab_revoke_group_personal_access_token":                true,
		"gitlab_revoke_impersonation_token":                        true,
		"gitlab_runner_controller_create":                          true,
		"gitlab_runner_controller_delete":                          true,
		"gitlab_runner_controller_get":                             true,
		"gitlab_runner_controller_list":                            true,
		"gitlab_runner_controller_scope_add_instance":              true,
		"gitlab_runner_controller_scope_add_runner":                true,
		"gitlab_runner_controller_scope_list":                      true,
		"gitlab_runner_controller_scope_remove_instance":           true,
		"gitlab_runner_controller_scope_remove_runner":             true,
		"gitlab_runner_controller_token_create":                    true,
		"gitlab_runner_controller_token_get":                       true,
		"gitlab_runner_controller_token_list":                      true,
		"gitlab_runner_controller_token_revoke":                    true,
		"gitlab_runner_controller_token_rotate":                    true,
		"gitlab_runner_controller_update":                          true,
		"gitlab_runner_delete_by_token":                            true,
		"gitlab_runner_delete_registered":                          true,
		"gitlab_runner_disable_project":                            true,
		"gitlab_runner_enable_project":                             true,
		"gitlab_runner_get":                                        true,
		"gitlab_runner_jobs":                                       true,
		"gitlab_runner_list":                                       true,
		"gitlab_runner_list_all":                                   true,
		"gitlab_runner_list_group":                                 true,
		"gitlab_runner_list_managers":                              true,
		"gitlab_runner_list_project":                               true,
		"gitlab_runner_register":                                   true,
		"gitlab_runner_remove":                                     true,
		"gitlab_runner_reset_group_reg_token":                      true,
		"gitlab_runner_reset_instance_reg_token":                   true,
		"gitlab_runner_reset_project_reg_token":                    true,
		"gitlab_runner_reset_token":                                true,
		"gitlab_runner_update":                                     true,
		"gitlab_runner_verify":                                     true,
		"gitlab_schedule_all_group_storage_moves":                  true,
		"gitlab_schedule_all_project_storage_moves":                true,
		"gitlab_schedule_all_snippet_storage_moves":                true,
		"gitlab_schedule_group_export":                             true,
		"gitlab_schedule_group_relations_export":                   true,
		"gitlab_schedule_group_storage_move":                       true,
		"gitlab_schedule_project_export":                           true,
		"gitlab_schedule_project_storage_move":                     true,
		"gitlab_schedule_snippet_storage_move":                     true,
		"gitlab_search_code":                                       true,
		"gitlab_search_commits":                                    true,
		"gitlab_search_issues":                                     true,
		"gitlab_search_merge_requests":                             true,
		"gitlab_search_milestones":                                 true,
		"gitlab_search_notes":                                      true,
		"gitlab_search_projects":                                   true,
		"gitlab_search_snippets":                                   true,
		"gitlab_search_users":                                      true,
		"gitlab_search_wiki":                                       true,
		"gitlab_set_custom_attribute":                              true,
		"gitlab_set_external_status_check_status":                  true,
		"gitlab_set_feature_flag":                                  true,
		"gitlab_set_jira_integration":                              true,
		"gitlab_set_project_mr_external_status_check_status":       true,
		"gitlab_set_user_status":                                   true,
		"gitlab_show_secure_file":                                  true,
		"gitlab_snippet_content":                                   true,
		"gitlab_snippet_create":                                    true,
		"gitlab_snippet_delete":                                    true,
		"gitlab_snippet_emoji_create":                              true,
		"gitlab_snippet_emoji_delete":                              true,
		"gitlab_snippet_emoji_get":                                 true,
		"gitlab_snippet_emoji_list":                                true,
		"gitlab_snippet_explore":                                   true,
		"gitlab_snippet_file_content":                              true,
		"gitlab_snippet_get":                                       true,
		"gitlab_snippet_list":                                      true,
		"gitlab_snippet_list_all":                                  true,
		"gitlab_snippet_note_create":                               true,
		"gitlab_snippet_note_delete":                               true,
		"gitlab_snippet_note_emoji_create":                         true,
		"gitlab_snippet_note_emoji_delete":                         true,
		"gitlab_snippet_note_emoji_get":                            true,
		"gitlab_snippet_note_emoji_list":                           true,
		"gitlab_snippet_note_get":                                  true,
		"gitlab_snippet_note_list":                                 true,
		"gitlab_snippet_note_update":                               true,
		"gitlab_snippet_update":                                    true,
		"gitlab_start_bulk_import":                                 true,
		"gitlab_subgroups_list":                                    true,
		"gitlab_summarize_issue":                                   true,
		"gitlab_summarize_mr_review":                               true,
		"gitlab_tag_create":                                        true,
		"gitlab_tag_delete":                                        true,
		"gitlab_tag_get":                                           true,
		"gitlab_tag_get_protected":                                 true,
		"gitlab_tag_get_signature":                                 true,
		"gitlab_tag_list":                                          true,
		"gitlab_tag_list_protected":                                true,
		"gitlab_tag_protect":                                       true,
		"gitlab_tag_unprotect":                                     true,
		"gitlab_test_system_hook":                                  true,
		"gitlab_todo_list":                                         true,
		"gitlab_todo_mark_all_done":                                true,
		"gitlab_todo_mark_done":                                    true,
		"gitlab_track_event":                                       true,
		"gitlab_track_events":                                      true,
		"gitlab_unban_user":                                        true,
		"gitlab_unblock_user":                                      true,
		"gitlab_unlock_terraform_state":                            true,
		"gitlab_update_alert_metric_image":                         true,
		"gitlab_update_appearance":                                 true,
		"gitlab_update_broadcast_message":                          true,
		"gitlab_update_commit_discussion_note":                     true,
		"gitlab_update_compliance_policy_settings":                 true,
		"gitlab_update_epic_discussion_note":                       true,
		"gitlab_update_external_status_check":                      true,
		"gitlab_update_freeze_period":                              true,
		"gitlab_update_group_mr_approval_settings":                 true,
		"gitlab_update_group_scim_identity":                        true,
		"gitlab_update_group_secret_push_protection":               true,
		"gitlab_update_issue_discussion_note":                      true,
		"gitlab_update_package_protection_rule":                    true,
		"gitlab_update_project_external_status_check":              true,
		"gitlab_update_project_mr_approval_settings":               true,
		"gitlab_update_project_secret_push_protection":             true,
		"gitlab_update_repository_submodule":                       true,
		"gitlab_update_settings":                                   true,
		"gitlab_update_snippet_discussion_note":                    true,
		"gitlab_update_topic":                                      true,
		"gitlab_update_work_item":                                  true,
		"gitlab_upload_alert_metric_image":                         true,
		"gitlab_user_contribution_event_list":                      true,
		"gitlab_user_current":                                      true,
		"gitlab_vulnerability_severity_count":                      true,
		"gitlab_wiki_create":                                       true,
		"gitlab_wiki_delete":                                       true,
		"gitlab_wiki_get":                                          true,
		"gitlab_wiki_list":                                         true,
		"gitlab_wiki_update":                                       true,
		"gitlab_wiki_upload_attachment":                            true,
	}

	for _, tool := range result.Tools {
		if !expectedNames[tool.Name] {
			t.Errorf("unexpected tool registered: %s", tool.Name)
		}
		delete(expectedNames, tool.Name)
	}
	for name := range expectedNames {
		t.Errorf("expected tool not found: %s", name)
	}
}

// TestRegisterAllMeta_ToolNames verifies that every expected meta-tool name
// is present after RegisterAllMeta (enterprise=true) and that no unexpected tools are registered.
func TestRegisterAllMeta_ToolNames(t *testing.T) {
	session := newMetaMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}), true)

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	expectedNames := map[string]bool{
		"gitlab_access":                true,
		"gitlab_admin":                 true,
		"gitlab_analyze":               true,
		"gitlab_attestation":           true,
		"gitlab_audit_event":           true,
		"gitlab_branch":                true,
		"gitlab_ci_catalog":            true,
		"gitlab_ci_variable":           true,
		"gitlab_compliance_policy":     true,
		"gitlab_custom_emoji":          true,
		"gitlab_dependency":            true,
		"gitlab_dora_metrics":          true,
		"gitlab_enterprise_user":       true,
		"gitlab_environment":           true,
		"gitlab_external_status_check": true,
		"gitlab_feature_flags":         true,
		"gitlab_geo":                   true,
		"gitlab_group":                 true,
		"gitlab_group_scim":            true,
		"gitlab_issue":                 true,
		"gitlab_job":                   true,
		"gitlab_member_role":           true,
		"gitlab_merge_request":         true,
		"gitlab_merge_train":           true,
		"gitlab_model_registry":        true,
		"gitlab_mr_review":             true,
		"gitlab_package":               true,
		"gitlab_pipeline":              true,
		"gitlab_project":               true,
		"gitlab_project_alias":         true,
		"gitlab_release":               true,
		"gitlab_repository":            true,
		"gitlab_discover_project":      true,
		"gitlab_runner":                true,
		"gitlab_search":                true,
		"gitlab_security_finding":      true,
		"gitlab_snippet":               true,
		"gitlab_storage_move":          true,
		"gitlab_tag":                   true,
		"gitlab_template":              true,
		"gitlab_user":                  true,
		"gitlab_vulnerability":         true,

		"gitlab_wiki": true,
	}

	for _, tool := range result.Tools {
		if !expectedNames[tool.Name] {
			t.Errorf("unexpected meta-tool registered: %s", tool.Name)
		}
		delete(expectedNames, tool.Name)
	}
	for name := range expectedNames {
		t.Errorf("expected meta-tool not found: %s", name)
	}
}

// TestRegisterAll_ToolAnnotations verifies that every registered tool has
// non-nil annotations with OpenWorldHint set to true.
func TestRegisterAll_ToolAnnotations(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		if tool.Annotations == nil {
			t.Errorf("tool %s missing annotations", tool.Name)
			continue
		}
		if tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Errorf("tool %s: OpenWorldHint should be true", tool.Name)
		}
	}
}

// TestRegisterAll_CallToolThroughMCP verifies a single tool call round-trip
// through an in-memory MCP session using individual tools.
func TestRegisterAll_CallToolThroughMCP(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_project_get",
		Arguments: map[string]any{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() returned error result")
	}
}

// TestRegisterAllMeta_CallToolThroughMCP verifies a single meta-tool call
// round-trip through an in-memory MCP session.
func TestRegisterAllMeta_CallToolThroughMCP(t *testing.T) {
	session := newMetaMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`)
		default:
			http.NotFound(w, r)
		}
	}), false)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_project",
		Arguments: map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": "42"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() returned error result")
	}
}
