//go:build e2e

// actions_b1_test.go names the catalog actions the admin, runner, instance
// settings, templates and grab-bag misc scenarios of batch B1 call, and that
// actions_test.go did not already declare.
//
// They are typed harness.ActionID constants so the push-time static gate
// reads them out of the type checker's record and follows each through the
// helper parameters it is passed to. A constant this file shares with an
// existing one in actions_test.go is referenced there rather than redeclared:
// the runner reads, the project and environment lifecycles, the current-user
// read and the work-item type read all live in actions_test.go and are used
// here by their existing names.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The gitlab_admin group's instance settings and metadata reads and writes.
const (
	actionAdminSettingsGet    harness.ActionID = "admin.settings_get"
	actionAdminSettingsUpdate harness.ActionID = "admin.settings_update"
	actionAdminAppearanceGet  harness.ActionID = "admin.appearance_get"
	actionAdminAppearanceUpd  harness.ActionID = "admin.appearance_update"
	actionAdminAppStatsGet    harness.ActionID = "admin.app_statistics_get"
	actionAdminPlanLimitsGet  harness.ActionID = "admin.plan_limits_get"
	actionAdminPlanLimitsChg  harness.ActionID = "admin.plan_limits_change"
)

// The gitlab_admin topic CRUD.
const (
	actionAdminTopicList   harness.ActionID = "admin.topic_list"
	actionAdminTopicCreate harness.ActionID = "admin.topic_create"
	actionAdminTopicGet    harness.ActionID = "admin.topic_get"
	actionAdminTopicUpdate harness.ActionID = "admin.topic_update"
	actionAdminTopicDelete harness.ActionID = "admin.topic_delete"
)

// The gitlab_admin broadcast message CRUD.
const (
	actionAdminBroadcastList   harness.ActionID = "admin.broadcast_message_list"
	actionAdminBroadcastGet    harness.ActionID = "admin.broadcast_message_get"
	actionAdminBroadcastCreate harness.ActionID = "admin.broadcast_message_create"
	actionAdminBroadcastUpdate harness.ActionID = "admin.broadcast_message_update"
	actionAdminBroadcastDelete harness.ActionID = "admin.broadcast_message_delete"
)

// The gitlab_admin feature flag actions.
const (
	actionAdminFeatureList     harness.ActionID = "admin.feature_list"
	actionAdminFeatureListDefs harness.ActionID = "admin.feature_list_definitions"
	actionAdminFeatureSet      harness.ActionID = "admin.feature_set"
	actionAdminFeatureDelete   harness.ActionID = "admin.feature_delete"
)

// The gitlab_admin system hook actions, including the edit and URL-variable
// writes.
const (
	actionAdminSystemHookList      harness.ActionID = "admin.system_hook_list"
	actionAdminSystemHookGet       harness.ActionID = "admin.system_hook_get"
	actionAdminSystemHookAdd       harness.ActionID = "admin.system_hook_add"
	actionAdminSystemHookTest      harness.ActionID = "admin.system_hook_test"
	actionAdminSystemHookDelete    harness.ActionID = "admin.system_hook_delete"
	actionAdminSystemHookEdit      harness.ActionID = "admin.system_hook_edit"
	actionAdminSystemHookSetVar    harness.ActionID = "admin.system_hook_set_url_variable"
	actionAdminSystemHookDeleteVar harness.ActionID = "admin.system_hook_delete_url_variable"
)

// The gitlab_admin Sidekiq metrics reads.
const (
	actionAdminSidekiqQueue    harness.ActionID = "admin.sidekiq_queue_metrics"
	actionAdminSidekiqProcess  harness.ActionID = "admin.sidekiq_process_metrics"
	actionAdminSidekiqJobStats harness.ActionID = "admin.sidekiq_job_stats"
	actionAdminSidekiqCompound harness.ActionID = "admin.sidekiq_compound_metrics"
)

// The gitlab_admin OAuth application actions.
const (
	actionAdminApplicationList   harness.ActionID = "admin.application_list"
	actionAdminApplicationCreate harness.ActionID = "admin.application_create"
	actionAdminApplicationRenew  harness.ActionID = "admin.application_renew_secret"
	actionAdminApplicationDelete harness.ActionID = "admin.application_delete"
)

// The gitlab_admin custom attribute actions.
const (
	actionAdminCustomAttrList   harness.ActionID = "admin.custom_attr_list"
	actionAdminCustomAttrGet    harness.ActionID = "admin.custom_attr_get"
	actionAdminCustomAttrSet    harness.ActionID = "admin.custom_attr_set"
	actionAdminCustomAttrDelete harness.ActionID = "admin.custom_attr_delete"
)

// The gitlab_admin dependency proxy purge.
const actionAdminDependencyProxyDelete harness.ActionID = "admin.dependency_proxy_delete"

// The gitlab_admin project CI secure file actions.
const (
	actionAdminSecureFileList   harness.ActionID = "admin.secure_file_list"
	actionAdminSecureFileGet    harness.ActionID = "admin.secure_file_get"
	actionAdminSecureFileCreate harness.ActionID = "admin.secure_file_create"
	actionAdminSecureFileDelete harness.ActionID = "admin.secure_file_delete"
)

// The gitlab_admin integrated error tracking actions.
const (
	actionAdminErrorTrackingGetSettings    harness.ActionID = "admin.error_tracking_get_settings"
	actionAdminErrorTrackingUpdateSettings harness.ActionID = "admin.error_tracking_update_settings"
	actionAdminErrorTrackingCreate         harness.ActionID = "admin.error_tracking_create"
	actionAdminErrorTrackingList           harness.ActionID = "admin.error_tracking_list"
	actionAdminErrorTrackingDelete         harness.ActionID = "admin.error_tracking_delete"
)

// The gitlab_admin alert metric image actions.
const (
	actionAdminAlertMetricImageUpload harness.ActionID = "admin.alert_metric_image_upload"
	actionAdminAlertMetricImageList   harness.ActionID = "admin.alert_metric_image_list"
	actionAdminAlertMetricImageUpdate harness.ActionID = "admin.alert_metric_image_update"
	actionAdminAlertMetricImageDelete harness.ActionID = "admin.alert_metric_image_delete"
)

// The gitlab_admin Terraform state actions.
const (
	actionAdminTerraformStateList   harness.ActionID = "admin.terraform_state_list"
	actionAdminTerraformStateGet    harness.ActionID = "admin.terraform_state_get"
	actionAdminTerraformStateLock   harness.ActionID = "admin.terraform_state_lock"
	actionAdminTerraformStateUnlock harness.ActionID = "admin.terraform_state_unlock"
	actionAdminTerraformStateDelete harness.ActionID = "admin.terraform_state_delete"
	actionAdminTerraformVersionDel  harness.ActionID = "admin.terraform_version_delete"
)

// The gitlab_admin usage-data actions.
const (
	actionAdminUsageDataMetricDefs  harness.ActionID = "admin.usage_data_metric_definitions"
	actionAdminUsageDataServicePing harness.ActionID = "admin.usage_data_service_ping"
	actionAdminUsageDataTrackEvent  harness.ActionID = "admin.usage_data_track_event"
	actionAdminUsageDataTrackEvents harness.ActionID = "admin.usage_data_track_events"
	actionAdminUsageDataQueries     harness.ActionID = "admin.usage_data_queries"
	actionAdminUsageDataNonSQL      harness.ActionID = "admin.usage_data_non_sql_metrics"
)

// The gitlab_admin schema migration mark.
const actionAdminDBMigrationMark harness.ActionID = "admin.db_migration_mark"

// The gitlab_admin direct-transfer (bulk import) actions.
const (
	actionAdminBulkImportStart          harness.ActionID = "admin.bulk_import_start"
	actionAdminBulkImportList           harness.ActionID = "admin.bulk_import_list"
	actionAdminBulkImportGet            harness.ActionID = "admin.bulk_import_get"
	actionAdminBulkImportCancel         harness.ActionID = "admin.bulk_import_cancel"
	actionAdminBulkImportEntityList     harness.ActionID = "admin.bulk_import_entity_list"
	actionAdminBulkImportEntityGet      harness.ActionID = "admin.bulk_import_entity_get"
	actionAdminBulkImportEntityFailures harness.ActionID = "admin.bulk_import_entity_failures"
)

// The gitlab_admin external importer actions.
const (
	actionAdminImportGitHub          harness.ActionID = "admin.import_github"
	actionAdminImportCancelGitHub    harness.ActionID = "admin.import_cancel_github"
	actionAdminImportGists           harness.ActionID = "admin.import_gists"
	actionAdminImportBitbucket       harness.ActionID = "admin.import_bitbucket"
	actionAdminImportBitbucketServer harness.ActionID = "admin.import_bitbucket_server"
)

// The throwaway-runner lifecycle and registration-token resets. The instance
// runner reads (list_all, get, list_project, list_group, list_managers) and
// the project enable and disable live in actions_test.go.
const (
	actionRunnerList                harness.ActionID = "runner.list"
	actionRunnerRegister            harness.ActionID = "runner.register"
	actionRunnerVerify              harness.ActionID = "runner.verify"
	actionRunnerJobs                harness.ActionID = "runner.jobs"
	actionRunnerUpdate              harness.ActionID = "runner.update"
	actionRunnerResetToken          harness.ActionID = "runner.reset_token"
	actionRunnerResetGroupRegTok    harness.ActionID = "runner.reset_group_reg_token"
	actionRunnerResetProjectRegTok  harness.ActionID = "runner.reset_project_reg_token"
	actionRunnerResetInstanceRegTok harness.ActionID = "runner.reset_instance_reg_token"
	actionRunnerDeleteByToken       harness.ActionID = "runner.delete_by_token"
	actionRunnerDeleteRegistered    harness.ActionID = "runner.delete_registered"
	actionRunnerRemove              harness.ActionID = "runner.remove"
)

// The CI pipeline and job lifecycle reads and writes. pipeline.create lives
// in actions_test.go.
const (
	actionPipelineGet    harness.ActionID = "pipeline.get"
	actionPipelineList   harness.ActionID = "pipeline.list"
	actionPipelineRetry  harness.ActionID = "pipeline.retry"
	actionPipelineDelete harness.ActionID = "pipeline.delete"
	actionJobList        harness.ActionID = "job.list"
	actionJobGet         harness.ActionID = "job.get"
	actionJobTrace       harness.ActionID = "job.trace"
)

// The job-token scope read and the project job listing.
const (
	actionJobListProject   harness.ActionID = "job.list_project"
	actionJobTokenScopeGet harness.ActionID = "job.token_scope_get"
)

// The instance-scoped CI variable actions.
const (
	actionCIVariableInstanceCreate harness.ActionID = "ci_variable.instance_create"
	actionCIVariableInstanceList   harness.ActionID = "ci_variable.instance_list"
	actionCIVariableInstanceGet    harness.ActionID = "ci_variable.instance_get"
	actionCIVariableInstanceUpdate harness.ActionID = "ci_variable.instance_update"
	actionCIVariableInstanceDelete harness.ActionID = "ci_variable.instance_delete"
)

// The instance template listings and reads.
const (
	actionTemplateCIYmlList       harness.ActionID = "template.ci_yml_list"
	actionTemplateCIYmlGet        harness.ActionID = "template.ci_yml_get"
	actionTemplateDockerfileList  harness.ActionID = "template.dockerfile_list"
	actionTemplateDockerfileGet   harness.ActionID = "template.dockerfile_get"
	actionTemplateGitignoreList   harness.ActionID = "template.gitignore_list"
	actionTemplateGitignoreGet    harness.ActionID = "template.gitignore_get"
	actionTemplateLicenseList     harness.ActionID = "template.license_list"
	actionTemplateLicenseGet      harness.ActionID = "template.license_get"
	actionTemplateProjectTmplList harness.ActionID = "template.project_template_list"
	actionTemplateProjectTmplGet  harness.ActionID = "template.project_template_get"
)

// The markdown render read.
const actionRepositoryMarkdownRender harness.ActionID = "repository.markdown_render"

// The feature flag and feature flag user list actions.
const (
	actionFeatureFlagList  harness.ActionID = "feature_flags.feature_flag_list"
	actionFFUserListCreate harness.ActionID = "feature_flags.ff_user_list_create"
	actionFFUserListList   harness.ActionID = "feature_flags.ff_user_list_list"
	actionFFUserListGet    harness.ActionID = "feature_flags.ff_user_list_get"
	actionFFUserListUpdate harness.ActionID = "feature_flags.ff_user_list_update"
	actionFFUserListDelete harness.ActionID = "feature_flags.ff_user_list_delete"
)

// The branch protection write and the branch rule listing.
const (
	actionBranchProtect  harness.ActionID = "branch.protect"
	actionBranchRuleList harness.ActionID = "branch.rule_list"
)

// The CI/CD catalog reads.
const (
	actionCICatalogList harness.ActionID = "ci_catalog.list"
	actionCICatalogGet  harness.ActionID = "ci_catalog.get"
)

// The deployment reads and the API deployment create.
const (
	actionEnvironmentDeploymentCreate        harness.ActionID = "environment.deployment_create"
	actionEnvironmentDeploymentList          harness.ActionID = "environment.deployment_list"
	actionEnvironmentDeploymentMergeRequests harness.ActionID = "environment.deployment_merge_requests"
)

// The release create and latest-release read.
const (
	actionReleaseCreate    harness.ActionID = "release.create"
	actionReleaseGetLatest harness.ActionID = "release.get_latest"
)

// The tag create that a release stands on.
const actionTagCreate harness.ActionID = "tag.create"

// The merge request raw diffs read.
const actionMRRawDiffs harness.ActionID = "mr_review.raw_diffs"

// The current user's key listings.
const (
	actionUserSSHKeys harness.ActionID = "user.ssh_keys"
	actionUserGPGKeys harness.ActionID = "user.gpg_keys"
)
