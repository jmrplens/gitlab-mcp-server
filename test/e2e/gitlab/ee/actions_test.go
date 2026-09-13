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
