package tools

import (
	"fmt"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
)

const (
	expectedBaseDynamicCatalogActions         = 855
	expectedEnterpriseDynamicCatalogActions   = 1010
	expectedGitLabComEnterpriseCatalogActions = 1015
)

func TestActionCatalog_BaselineCountsDoNotRegress(t *testing.T) {
	testCases := []struct {
		name       string
		client     *gitlabclient.Client
		enterprise bool
		want       int
	}{
		{name: "base", want: expectedBaseDynamicCatalogActions},
		{name: "self-managed enterprise", enterprise: true, want: expectedEnterpriseDynamicCatalogActions},
		{name: "gitlab.com enterprise", client: newGitLabDotComClient(t), enterprise: true, want: expectedGitLabComEnterpriseCatalogActions},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := mustBuildDynamicActionCatalogForTest(t, tc.client, tc.enterprise)
			if got := catalog.CountActions(); got != tc.want {
				t.Fatalf("dynamic catalog action count = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestActionSpecCoverage_AllCatalogRoutesClassified(t *testing.T) {
	catalog := mustBuildDynamicActionCatalogForTest(t, newGitLabDotComClient(t), true)
	missing := make([]actioncatalog.ActionID, 0)
	for _, action := range catalog.Actions() {
		if action.SpecBacked {
			continue
		}
		if _, ok := temporaryActionSpecMigrationAllowlist[action.ID]; ok {
			continue
		}
		missing = append(missing, action.ID)
	}
	if len(missing) > 0 {
		t.Fatalf("catalog actions must be spec-backed or explicitly allowlisted:\n%s", formatActionSpecMigrationAllowlist(missing))
	}
}

var temporaryActionSpecMigrationAllowlist = map[actioncatalog.ActionID]struct{}{
	"admin.alert_metric_image_delete":      {},
	"admin.alert_metric_image_list":        {},
	"admin.alert_metric_image_update":      {},
	"admin.alert_metric_image_upload":      {},
	"admin.app_statistics_get":             {},
	"admin.appearance_get":                 {},
	"admin.appearance_update":              {},
	"admin.application_create":             {},
	"admin.application_delete":             {},
	"admin.application_list":               {},
	"admin.broadcast_message_create":       {},
	"admin.broadcast_message_delete":       {},
	"admin.broadcast_message_get":          {},
	"admin.broadcast_message_list":         {},
	"admin.broadcast_message_update":       {},
	"admin.bulk_import_cancel":             {},
	"admin.bulk_import_entity_failures":    {},
	"admin.bulk_import_entity_get":         {},
	"admin.bulk_import_entity_list":        {},
	"admin.bulk_import_get":                {},
	"admin.bulk_import_list":               {},
	"admin.bulk_import_start":              {},
	"admin.cluster_agent_delete":           {},
	"admin.cluster_agent_get":              {},
	"admin.cluster_agent_list":             {},
	"admin.cluster_agent_register":         {},
	"admin.cluster_agent_token_create":     {},
	"admin.cluster_agent_token_get":        {},
	"admin.cluster_agent_token_list":       {},
	"admin.cluster_agent_token_revoke":     {},
	"admin.custom_attr_delete":             {},
	"admin.custom_attr_get":                {},
	"admin.custom_attr_list":               {},
	"admin.custom_attr_set":                {},
	"admin.db_migration_mark":              {},
	"admin.dependency_proxy_delete":        {},
	"admin.error_tracking_create":          {},
	"admin.error_tracking_delete":          {},
	"admin.error_tracking_get_settings":    {},
	"admin.error_tracking_list":            {},
	"admin.error_tracking_update_settings": {},
	"admin.feature_delete":                 {},
	"admin.feature_list":                   {},
	"admin.feature_list_definitions":       {},
	"admin.feature_set":                    {},
	"admin.import_bitbucket":               {},
	"admin.import_bitbucket_server":        {},
	"admin.import_cancel_github":           {},
	"admin.import_gists":                   {},
	"admin.import_github":                  {},
	"admin.license_add":                    {},
	"admin.license_delete":                 {},
	"admin.license_get":                    {},
	"admin.metadata_get":                   {},
	"admin.plan_limits_change":             {},
	"admin.plan_limits_get":                {},
	"admin.secure_file_create":             {},
	"admin.secure_file_delete":             {},
	"admin.secure_file_get":                {},
	"admin.secure_file_list":               {},
	"admin.settings_get":                   {},
	"admin.settings_update":                {},
	"admin.sidekiq_compound_metrics":       {},
	"admin.sidekiq_job_stats":              {},
	"admin.sidekiq_process_metrics":        {},
	"admin.sidekiq_queue_metrics":          {},
	"admin.system_hook_add":                {},
	"admin.system_hook_delete":             {},
	"admin.system_hook_get":                {},
	"admin.system_hook_list":               {},
	"admin.system_hook_test":               {},
	"admin.terraform_state_delete":         {},
	"admin.terraform_state_get":            {},
	"admin.terraform_state_list":           {},
	"admin.terraform_state_lock":           {},
	"admin.terraform_state_unlock":         {},
	"admin.terraform_version_delete":       {},
	"admin.topic_create":                   {},
	"admin.topic_delete":                   {},
	"admin.topic_get":                      {},
	"admin.topic_list":                     {},
	"admin.topic_update":                   {},
	"admin.usage_data_metric_definitions":  {},
	"admin.usage_data_non_sql_metrics":     {},
	"admin.usage_data_queries":             {},
	"admin.usage_data_service_ping":        {},
	"admin.usage_data_track_event":         {},
	"admin.usage_data_track_events":        {},
	"group.analytics_issues_count":         {},
	"group.analytics_members_count":        {},
	"group.analytics_mr_count":             {},
	"group.credential_delete_ssh_key":      {},
	"group.credential_list_pats":           {},
	"group.credential_list_ssh_keys":       {},
	"group.credential_revoke_pat":          {},
	"group.epic_board_get":                 {},
	"group.epic_board_list":                {},
	"group.epic_create":                    {},
	"group.epic_delete":                    {},
	"group.epic_discussion_add_note":       {},
	"group.epic_discussion_create":         {},
	"group.epic_discussion_delete_note":    {},
	"group.epic_discussion_get":            {},
	"group.epic_discussion_list":           {},
	"group.epic_discussion_update_note":    {},
	"group.epic_get":                       {},
	"group.epic_get_links":                 {},
	"group.epic_issue_list":                {},
	"group.epic_issue_remove":              {},
	"group.epic_issue_update":              {},
	"group.epic_list":                      {},
	"group.epic_note_create":               {},
	"group.epic_note_delete":               {},
	"group.epic_note_get":                  {},
	"group.epic_note_list":                 {},
	"group.epic_note_update":               {},
	"group.epic_update":                    {},
	"group.group_relations_list_status":    {},
	"group.group_relations_schedule":       {},
	"group.ldap_link_add":                  {},
	"group.ldap_link_delete":               {},
	"group.ldap_link_delete_for_provider":  {},
	"group.ldap_link_list":                 {},
	"group.protected_branch_get":           {},
	"group.protected_branch_list":          {},
	"group.protected_branch_protect":       {},
	"group.protected_branch_unprotect":     {},
	"group.protected_branch_update":        {},
	"group.protected_env_get":              {},
	"group.protected_env_list":             {},
	"group.protected_env_protect":          {},
	"group.protected_env_unprotect":        {},
	"group.protected_env_update":           {},
	"group.saml_link_add":                  {},
	"group.saml_link_delete":               {},
	"group.saml_link_get":                  {},
	"group.saml_link_list":                 {},
	"group.security_settings_update":       {},
	"group.service_account_create":         {},
	"group.service_account_delete":         {},
	"group.service_account_list":           {},
	"group.service_account_pat_create":     {},
	"group.service_account_pat_list":       {},
	"group.service_account_pat_revoke":     {},
	"group.service_account_update":         {},
	"group.ssh_cert_create":                {},
	"group.ssh_cert_delete":                {},
	"group.ssh_cert_list":                  {},
	"group.wiki_create":                    {},
	"group.wiki_delete":                    {},
	"group.wiki_edit":                      {},
	"group.wiki_get":                       {},
	"group.wiki_list":                      {},
}

func mustBuildDynamicActionCatalogForTest(t *testing.T, client *gitlabclient.Client, enterprise bool) *actioncatalog.Catalog {
	t.Helper()
	catalog := mustBuildActionCatalog(t, client, ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	catalog, err := dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	return catalog
}

func formatActionSpecMigrationAllowlist(ids []actioncatalog.ActionID) string {
	var builder strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&builder, "\t%q: {},\n", id)
	}
	return builder.String()
}
