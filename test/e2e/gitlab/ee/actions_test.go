//go:build e2e

// actions_test.go names every catalog action this package calls, once.
//
// They are typed constants rather than string literals at the call sites
// because the push-time static gate reads constants of the harness's
// ActionID type out of the type checker's record, follows them through
// helper parameters, and checks each against the catalog and against the
// package's tier. A literal would be invisible to it. Keeping them together
// also means a renamed action fails to compile in one place.
//
// A Free action is here too where the scenario that drives it needs a
// license: the license endpoints, a merge request's approval reset, a
// deployment's approval and the group Datadog integration are Free in the
// catalog and answer only on a licensed instance.

package ee

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The license, the one thing this package changes about the instance.
const (
	actionLicenseGet    harness.ActionID = "admin.license_get"
	actionLicenseAdd    harness.ActionID = "admin.license_add"
	actionLicenseDelete harness.ActionID = "admin.license_delete"
)

// Audit events at their three scopes.
const (
	actionAuditEventListProject  harness.ActionID = "audit_event.list_project"
	actionAuditEventGetProject   harness.ActionID = "audit_event.get_project"
	actionAuditEventListGroup    harness.ActionID = "audit_event.list_group"
	actionAuditEventGetGroup     harness.ActionID = "audit_event.get_group"
	actionAuditEventListInstance harness.ActionID = "audit_event.list_instance"
	actionAuditEventGetInstance  harness.ActionID = "audit_event.get_instance"
)

// Merge trains.
const (
	actionMergeTrainListProject harness.ActionID = "merge_train.list_project"
	actionMergeTrainListBranch  harness.ActionID = "merge_train.list_branch"
	actionMergeTrainAdd         harness.ActionID = "merge_train.add"
	actionMergeTrainGet         harness.ActionID = "merge_train.get"
)

// DORA metrics.
const (
	actionDORAMetricsProject harness.ActionID = "dora_metrics.project"
	actionDORAMetricsGroup   harness.ActionID = "dora_metrics.group"
)

// Dependencies and their exports.
const (
	actionDependencyList           harness.ActionID = "dependency.list"
	actionDependencyExportCreate   harness.ActionID = "dependency.export_create"
	actionDependencyExportGet      harness.ActionID = "dependency.export_get"
	actionDependencyExportDownload harness.ActionID = "dependency.export_download"
)

// External status checks.
const (
	actionStatusCheckListProjectChecks   harness.ActionID = "external_status_check.list_project_checks"
	actionStatusCheckListProject         harness.ActionID = "external_status_check.list_project"
	actionStatusCheckCreateProject       harness.ActionID = "external_status_check.create_project"
	actionStatusCheckUpdateProject       harness.ActionID = "external_status_check.update_project"
	actionStatusCheckDeleteProject       harness.ActionID = "external_status_check.delete_project"
	actionStatusCheckListProjectMRChecks harness.ActionID = "external_status_check.list_project_mr_checks"
	actionStatusCheckSetProjectMRStatus  harness.ActionID = "external_status_check.set_project_mr_status"
	actionStatusCheckRetryProject        harness.ActionID = "external_status_check.retry_project"
)

// Custom member roles at both levels.
const (
	actionMemberRoleListInstance   harness.ActionID = "member_role.list_instance"
	actionMemberRoleCreateInstance harness.ActionID = "member_role.create_instance"
	actionMemberRoleDeleteInstance harness.ActionID = "member_role.delete_instance"
	actionMemberRoleListGroup      harness.ActionID = "member_role.list_group"
	actionMemberRoleCreateGroup    harness.ActionID = "member_role.create_group"
	actionMemberRoleDeleteGroup    harness.ActionID = "member_role.delete_group"
)

// Attestations.
const (
	actionAttestationList     harness.ActionID = "attestation.list"
	actionAttestationDownload harness.ActionID = "attestation.download"
)

// The compliance policy settings.
const (
	actionCompliancePolicyGet    harness.ActionID = "compliance_policy.get"
	actionCompliancePolicyUpdate harness.ActionID = "compliance_policy.update"
)

// Project aliases.
const (
	actionProjectAliasList   harness.ActionID = "project_alias.list"
	actionProjectAliasCreate harness.ActionID = "project_alias.create"
	actionProjectAliasGet    harness.ActionID = "project_alias.get"
	actionProjectAliasDelete harness.ActionID = "project_alias.delete"
)

// Geo sites.
const (
	actionGeoList       harness.ActionID = "geo.list"
	actionGeoCreate     harness.ActionID = "geo.create"
	actionGeoGet        harness.ActionID = "geo.get"
	actionGeoEdit       harness.ActionID = "geo.edit"
	actionGeoListStatus harness.ActionID = "geo.list_status"
	actionGeoGetStatus  harness.ActionID = "geo.get_status"
	actionGeoRepair     harness.ActionID = "geo.repair"
	actionGeoDelete     harness.ActionID = "geo.delete"
)

// Group storage moves, the licensed half of the storage move family.
const (
	actionStorageMoveRetrieveAllGroup harness.ActionID = "storage_move.retrieve_all_group"
	actionStorageMoveRetrieveGroup    harness.ActionID = "storage_move.retrieve_group"
	actionStorageMoveGetGroup         harness.ActionID = "storage_move.get_group"
	actionStorageMoveGetGroupForGroup harness.ActionID = "storage_move.get_group_for_group"
	actionStorageMoveScheduleGroup    harness.ActionID = "storage_move.schedule_group"
	actionStorageMoveScheduleAllGroup harness.ActionID = "storage_move.schedule_all_group"
)

// Security findings, categories, attributes and scan profiles.
const (
	actionSecurityFindingList            harness.ActionID = "security_finding.list"
	actionSecurityCategoryCreate         harness.ActionID = "security_category.create"
	actionSecurityCategoryUpdate         harness.ActionID = "security_category.update"
	actionSecurityCategoryDelete         harness.ActionID = "security_category.delete"
	actionSecurityAttributeCreate        harness.ActionID = "security_attribute.create"
	actionSecurityAttributeUpdate        harness.ActionID = "security_attribute.update"
	actionSecurityAttributeDelete        harness.ActionID = "security_attribute.delete"
	actionSecurityAttributeProjectUpdate harness.ActionID = "security_attribute.project_update"
	actionSecurityAttributeBulkUpdate    harness.ActionID = "security_attribute.bulk_update"
	actionScanProfileAttach              harness.ActionID = "security_scan_profile.attach"
	actionScanProfileDetach              harness.ActionID = "security_scan_profile.detach"
	actionScanProfileListProjectStatuses harness.ActionID = "security_scan_profile.list_project_statuses"
)

// Group SCIM identities.
const (
	actionGroupSCIMList   harness.ActionID = "group_scim.list"
	actionGroupSCIMGet    harness.ActionID = "group_scim.get"
	actionGroupSCIMUpdate harness.ActionID = "group_scim.update"
	actionGroupSCIMDelete harness.ActionID = "group_scim.delete"
)

// Enterprise users.
const (
	actionEnterpriseUserList       harness.ActionID = "enterprise_user.list"
	actionEnterpriseUserGet        harness.ActionID = "enterprise_user.get"
	actionEnterpriseUserDisable2FA harness.ActionID = "enterprise_user.disable_2fa"
	actionEnterpriseUserDelete     harness.ActionID = "enterprise_user.delete"
)

// The licensed reads and writes of the group tool that stand on a group
// alone, and the group Datadog integration the project tool routes.
const (
	actionGroupAnalyticsIssuesCount   harness.ActionID = "group.analytics_issues_count"
	actionGroupAnalyticsMRCount       harness.ActionID = "group.analytics_mr_count"
	actionGroupAnalyticsMembersCount  harness.ActionID = "group.analytics_members_count"
	actionGroupSecuritySettingsUpdate harness.ActionID = "group.security_settings_update"
	actionGroupDatadogSet             harness.ActionID = "project.integration_set_group_datadog"
	actionGroupDatadogGet             harness.ActionID = "project.integration_get_group_datadog"
	actionGroupDatadogDelete          harness.ActionID = "project.integration_delete_group_datadog"
)

// The group create, update and read, Free in the catalog and driven here for
// the Ultimate download-limit fields only an Ultimate schema carries.
const (
	actionGroupCreate harness.ActionID = "group.create"
	actionGroupUpdate harness.ActionID = "group.update"
	actionGroupGet    harness.ActionID = "group.get"
)

// Epics, their notes, their discussions, their child issues and their
// links, all reached through the group tool.
const (
	actionEpicCreate               harness.ActionID = "group.epic_create"
	actionEpicList                 harness.ActionID = "group.epic_list"
	actionEpicGet                  harness.ActionID = "group.epic_get"
	actionEpicUpdate               harness.ActionID = "group.epic_update"
	actionEpicDelete               harness.ActionID = "group.epic_delete"
	actionEpicGetLinks             harness.ActionID = "group.epic_get_links"
	actionEpicNoteCreate           harness.ActionID = "group.epic_note_create"
	actionEpicNoteList             harness.ActionID = "group.epic_note_list"
	actionEpicNoteGet              harness.ActionID = "group.epic_note_get"
	actionEpicNoteUpdate           harness.ActionID = "group.epic_note_update"
	actionEpicNoteDelete           harness.ActionID = "group.epic_note_delete"
	actionEpicDiscussionCreate     harness.ActionID = "group.epic_discussion_create"
	actionEpicDiscussionList       harness.ActionID = "group.epic_discussion_list"
	actionEpicDiscussionGet        harness.ActionID = "group.epic_discussion_get"
	actionEpicDiscussionAddNote    harness.ActionID = "group.epic_discussion_add_note"
	actionEpicDiscussionUpdateNote harness.ActionID = "group.epic_discussion_update_note"
	actionEpicDiscussionDeleteNote harness.ActionID = "group.epic_discussion_delete_note"
	actionEpicIssueAssign          harness.ActionID = "group.epic_issue_assign"
	actionEpicIssueList            harness.ActionID = "group.epic_issue_list"
	actionEpicIssueUpdate          harness.ActionID = "group.epic_issue_update"
	actionEpicIssueRemove          harness.ActionID = "group.epic_issue_remove"
	actionEpicBoardList            harness.ActionID = "group.epic_board_list"
	actionEpicBoardGet             harness.ActionID = "group.epic_board_get"
	actionEpicLabelEventList       harness.ActionID = "group.event_epic_label_list"
	actionEpicLabelEventGet        harness.ActionID = "group.event_epic_label_get"
)

// The group board create, the one Premium action of the board family; the
// reads and the column actions are Free and live in the common package.
const (
	actionGroupBoardCreate harness.ActionID = "group.group_board_create"
	actionGroupBoardList   harness.ActionID = "group.group_board_list"
	actionGroupBoardGet    harness.ActionID = "group.group_board_get"
	actionGroupBoardDelete harness.ActionID = "group.group_board_delete"
)

// Group wikis.
const (
	actionGroupWikiCreate harness.ActionID = "group.wiki_create"
	actionGroupWikiList   harness.ActionID = "group.wiki_list"
	actionGroupWikiGet    harness.ActionID = "group.wiki_get"
	actionGroupWikiEdit   harness.ActionID = "group.wiki_edit"
	actionGroupWikiDelete harness.ActionID = "group.wiki_delete"
)

// Group LDAP links and the LDAP sync.
const (
	actionGroupLDAPLinkAdd               harness.ActionID = "group.ldap_link_add"
	actionGroupLDAPLinkList              harness.ActionID = "group.ldap_link_list"
	actionGroupLDAPLinkDelete            harness.ActionID = "group.ldap_link_delete"
	actionGroupLDAPLinkDeleteForProvider harness.ActionID = "group.ldap_link_delete_for_provider"
	actionGroupLDAPSync                  harness.ActionID = "group.ldap_sync"
)

// Group SAML links and the SAML user listing.
const (
	actionGroupSAMLLinkList   harness.ActionID = "group.saml_link_list"
	actionGroupSAMLLinkAdd    harness.ActionID = "group.saml_link_add"
	actionGroupSAMLLinkGet    harness.ActionID = "group.saml_link_get"
	actionGroupSAMLLinkDelete harness.ActionID = "group.saml_link_delete"
	actionGroupSAMLUsersList  harness.ActionID = "group.saml_users_list"
)

// Group SSH certificates.
const (
	actionGroupSSHCertCreate harness.ActionID = "group.ssh_cert_create"
	actionGroupSSHCertList   harness.ActionID = "group.ssh_cert_list"
	actionGroupSSHCertDelete harness.ActionID = "group.ssh_cert_delete"
)

// The group credential inventory.
const (
	actionGroupCredentialListPATs     harness.ActionID = "group.credential_list_pats"
	actionGroupCredentialListSSHKeys  harness.ActionID = "group.credential_list_ssh_keys"
	actionGroupCredentialRevokePAT    harness.ActionID = "group.credential_revoke_pat"
	actionGroupCredentialDeleteSSHKey harness.ActionID = "group.credential_delete_ssh_key"
)

// Group protected branches and protected environments.
const (
	actionGroupProtectedBranchProtect   harness.ActionID = "group.protected_branch_protect"
	actionGroupProtectedBranchList      harness.ActionID = "group.protected_branch_list"
	actionGroupProtectedBranchGet       harness.ActionID = "group.protected_branch_get"
	actionGroupProtectedBranchUpdate    harness.ActionID = "group.protected_branch_update"
	actionGroupProtectedBranchUnprotect harness.ActionID = "group.protected_branch_unprotect"
	actionGroupProtectedEnvProtect      harness.ActionID = "group.protected_env_protect"
	actionGroupProtectedEnvList         harness.ActionID = "group.protected_env_list"
	actionGroupProtectedEnvGet          harness.ActionID = "group.protected_env_get"
	actionGroupProtectedEnvUpdate       harness.ActionID = "group.protected_env_update"
	actionGroupProtectedEnvUnprotect    harness.ActionID = "group.protected_env_unprotect"
)

// Group push rules.
const (
	actionGroupPushRuleAdd    harness.ActionID = "group.push_rule_add"
	actionGroupPushRuleGet    harness.ActionID = "group.push_rule_get"
	actionGroupPushRuleEdit   harness.ActionID = "group.push_rule_edit"
	actionGroupPushRuleDelete harness.ActionID = "group.push_rule_delete"
)

// Billable members and provisioned users.
const (
	actionGroupBillableMembersList           harness.ActionID = "group.group_billable_members_list"
	actionGroupBillableMemberMembershipsList harness.ActionID = "group.group_billable_member_memberships_list"
	actionGroupBillableMemberRemove          harness.ActionID = "group.group_billable_member_remove"
	actionGroupListProvisionedUsers          harness.ActionID = "group.list_provisioned_users"
)

// Group webhooks and their sub-operations.
const (
	actionGroupHookAdd                harness.ActionID = "group.hook_add"
	actionGroupHookList               harness.ActionID = "group.hook_list"
	actionGroupHookGet                harness.ActionID = "group.hook_get"
	actionGroupHookEdit               harness.ActionID = "group.hook_edit"
	actionGroupHookDelete             harness.ActionID = "group.hook_delete"
	actionGroupHookSetCustomHeader    harness.ActionID = "group.hook_set_custom_header"
	actionGroupHookDeleteCustomHeader harness.ActionID = "group.hook_delete_custom_header"
	actionGroupHookSetURLVariable     harness.ActionID = "group.hook_set_url_variable"
	actionGroupHookDeleteURLVariable  harness.ActionID = "group.hook_delete_url_variable"
	actionGroupHookTest               harness.ActionID = "group.hook_test"
	actionGroupHookResendEvent        harness.ActionID = "group.hook_resend_event"
)

// The group milestone burndown, the one licensed read of the group
// milestone family, which the old suite kept behind an enterprise guard in
// its Community group file.
const actionGroupMilestoneBurndown harness.ActionID = "group.group_milestone_burndown"

// Protected environments and the deployment approval they gate.
const (
	actionProtectedEnvProtect       harness.ActionID = "environment.protected_protect"
	actionProtectedEnvList          harness.ActionID = "environment.protected_list"
	actionProtectedEnvGet           harness.ActionID = "environment.protected_get"
	actionProtectedEnvUpdate        harness.ActionID = "environment.protected_update"
	actionProtectedEnvUnprotect     harness.ActionID = "environment.protected_unprotect"
	actionDeploymentCreate          harness.ActionID = "environment.deployment_create"
	actionDeploymentApproveOrReject harness.ActionID = "environment.deployment_approve_or_reject"
)

// The licensed actions of the project tool.
const (
	actionPushRuleAdd                   harness.ActionID = "project.push_rule_add"
	actionPushRuleGet                   harness.ActionID = "project.push_rule_get"
	actionPushRuleEdit                  harness.ActionID = "project.push_rule_edit"
	actionPushRuleDelete                harness.ActionID = "project.push_rule_delete"
	actionTargetBranchRuleCreate        harness.ActionID = "project.target_branch_rule_create"
	actionTargetBranchRuleList          harness.ActionID = "project.target_branch_rule_list"
	actionTargetBranchRuleDelete        harness.ActionID = "project.target_branch_rule_delete"
	actionPullMirrorGet                 harness.ActionID = "project.pull_mirror_get"
	actionPullMirrorConfigure           harness.ActionID = "project.pull_mirror_configure"
	actionStartMirroring                harness.ActionID = "project.start_mirroring"
	actionProjectApprovalConfigGet      harness.ActionID = "project.approval_config_get"
	actionProjectApprovalConfigChange   harness.ActionID = "project.approval_config_change"
	actionProjectApprovalRuleCreate     harness.ActionID = "project.approval_rule_create"
	actionProjectApprovalRuleList       harness.ActionID = "project.approval_rule_list"
	actionProjectApprovalRuleGet        harness.ActionID = "project.approval_rule_get"
	actionProjectApprovalRuleUpdate     harness.ActionID = "project.approval_rule_update"
	actionProjectApprovalRuleDelete     harness.ActionID = "project.approval_rule_delete"
	actionProjectSecuritySettingsGet    harness.ActionID = "project.security_settings_get"
	actionProjectSecuritySettingsUpdate harness.ActionID = "project.security_settings_update"
)

// The licensed actions of the merge request tool, and the three Free ones
// its approval scenarios stand on.
const (
	actionApprovalSettingsProjectGet    harness.ActionID = "merge_request.approval_settings_project_get"
	actionApprovalSettingsProjectUpdate harness.ActionID = "merge_request.approval_settings_project_update"
	actionApprovalSettingsGroupGet      harness.ActionID = "merge_request.approval_settings_group_get"
	actionApprovalSettingsGroupUpdate   harness.ActionID = "merge_request.approval_settings_group_update"
	actionMRApprovalState               harness.ActionID = "merge_request.approval_state"
	actionMRApprovalRules               harness.ActionID = "merge_request.approval_rules"
	actionMRApprovalConfig              harness.ActionID = "merge_request.approval_config"
	actionMRApprovalRuleCreate          harness.ActionID = "merge_request.approval_rule_create"
	actionMRApprovalRuleUpdate          harness.ActionID = "merge_request.approval_rule_update"
	actionMRApprovalRuleDelete          harness.ActionID = "merge_request.approval_rule_delete"
	actionMRApprove                     harness.ActionID = "merge_request.approve"
	actionMRApprovalReset               harness.ActionID = "merge_request.approval_reset"
	actionMRDependenciesList            harness.ActionID = "merge_request.dependencies_list"
	actionMRDependencyCreate            harness.ActionID = "merge_request.dependency_create"
	actionMRDependencyDelete            harness.ActionID = "merge_request.dependency_delete"
)

// The licensed reads of the issue tool.
const (
	actionIssueWeightEventList    harness.ActionID = "issue.event_issue_weight_list"
	actionIterationListProject    harness.ActionID = "issue.iteration_list_project"
	actionIterationListGroup      harness.ActionID = "issue.iteration_list_group"
	actionIssueIterationEventList harness.ActionID = "issue.event_issue_iteration_list"
	actionIssueIterationEventGet  harness.ActionID = "issue.event_issue_iteration_get"
)

// Runner controllers, their tokens and their scopes.
const (
	actionRunnerControllerList                harness.ActionID = "runner.controller_list"
	actionRunnerControllerGet                 harness.ActionID = "runner.controller_get"
	actionRunnerControllerCreate              harness.ActionID = "runner.controller_create"
	actionRunnerControllerUpdate              harness.ActionID = "runner.controller_update"
	actionRunnerControllerDelete              harness.ActionID = "runner.controller_delete"
	actionRunnerControllerTokenCreate         harness.ActionID = "runner.controller_token_create"
	actionRunnerControllerTokenList           harness.ActionID = "runner.controller_token_list"
	actionRunnerControllerTokenGet            harness.ActionID = "runner.controller_token_get"
	actionRunnerControllerTokenRotate         harness.ActionID = "runner.controller_token_rotate"
	actionRunnerControllerTokenRevoke         harness.ActionID = "runner.controller_token_revoke"
	actionRunnerControllerScopeList           harness.ActionID = "runner.controller_scope_list"
	actionRunnerControllerScopeAddInstance    harness.ActionID = "runner.controller_scope_add_instance"
	actionRunnerControllerScopeRemoveInstance harness.ActionID = "runner.controller_scope_remove_instance"
	actionRunnerControllerScopeAddRunner      harness.ActionID = "runner.controller_scope_add_runner"
	actionRunnerControllerScopeRemoveRunner   harness.ActionID = "runner.controller_scope_remove_runner"
)

// Vulnerabilities.
const (
	actionVulnerabilitySeverityCount           harness.ActionID = "vulnerability.severity_count"
	actionVulnerabilityList                    harness.ActionID = "vulnerability.list"
	actionVulnerabilityPipelineSecuritySummary harness.ActionID = "vulnerability.pipeline_security_summary"
	actionVulnerabilityGet                     harness.ActionID = "vulnerability.get"
	actionVulnerabilityConfirm                 harness.ActionID = "vulnerability.confirm"
	actionVulnerabilityResolve                 harness.ActionID = "vulnerability.resolve"
	actionVulnerabilityRevert                  harness.ActionID = "vulnerability.revert"
	actionVulnerabilityDismiss                 harness.ActionID = "vulnerability.dismiss"
)
