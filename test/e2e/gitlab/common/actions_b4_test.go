//go:build e2e

// actions_b4_test.go names the catalog actions the repository, CI,
// environment, release and package families call, once each, for the same
// reason actions_test.go does: the static gate reads typed constants and a
// literal would be invisible to it. The constants a family shares with an
// earlier port (branch.create, repository.commit_create, environment.create,
// pipeline.create, group.create) stay where they were declared.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The repository reads and the two changelog writes, plus the submodule
// listing and the two submodule actions a project without a gitlink refuses.
const (
	actionRepositoryTree              harness.ActionID = "repository.tree"
	actionRepositoryCompare           harness.ActionID = "repository.compare"
	actionRepositoryBlob              harness.ActionID = "repository.blob"
	actionRepositoryRawBlob           harness.ActionID = "repository.raw_blob"
	actionRepositoryMergeBase         harness.ActionID = "repository.merge_base"
	actionRepositoryContributors      harness.ActionID = "repository.contributors"
	actionRepositoryArchive           harness.ActionID = "repository.archive"
	actionRepositoryChangelogGenerate harness.ActionID = "repository.changelog_generate"
	actionRepositoryChangelogAdd      harness.ActionID = "repository.changelog_add"
	actionRepositoryListSubmodules    harness.ActionID = "repository.list_submodules"
	actionRepositoryReadSubmoduleFile harness.ActionID = "repository.read_submodule_file"
	actionRepositoryUpdateSubmodule   harness.ActionID = "repository.update_submodule"
)

// A repository file's reads and its create, update and delete.
const (
	actionRepositoryFileGet         harness.ActionID = "repository.file_get"
	actionRepositoryFileRaw         harness.ActionID = "repository.file_raw"
	actionRepositoryFileMetadata    harness.ActionID = "repository.file_metadata"
	actionRepositoryFileRawMetadata harness.ActionID = "repository.file_raw_metadata"
	actionRepositoryFileBlame       harness.ActionID = "repository.file_blame"
	actionRepositoryFileCreate      harness.ActionID = "repository.file_create"
	actionRepositoryFileUpdate      harness.ActionID = "repository.file_update"
	actionRepositoryFileDelete      harness.ActionID = "repository.file_delete"
)

// A commit's reads, its comments and statuses, its signature, and the two
// writes that make a commit out of another: cherry-pick and revert.
const (
	actionRepositoryCommitList          harness.ActionID = "repository.commit_list"
	actionRepositoryCommitGet           harness.ActionID = "repository.commit_get"
	actionRepositoryCommitDiff          harness.ActionID = "repository.commit_diff"
	actionRepositoryCommitRefs          harness.ActionID = "repository.commit_refs"
	actionRepositoryCommitCommentCreate harness.ActionID = "repository.commit_comment_create"
	actionRepositoryCommitComments      harness.ActionID = "repository.commit_comments"
	actionRepositoryCommitStatusSet     harness.ActionID = "repository.commit_status_set"
	actionRepositoryCommitStatuses      harness.ActionID = "repository.commit_statuses"
	actionRepositoryCommitSignature     harness.ActionID = "repository.commit_signature"
	actionRepositoryCommitCherryPick    harness.ActionID = "repository.commit_cherry_pick"
	actionRepositoryCommitRevert        harness.ActionID = "repository.commit_revert"
	actionRepositoryCommitMergeRequests harness.ActionID = "repository.commit_merge_requests"
)

// A commit's discussion threads and their notes.
const (
	actionRepositoryCommitDiscussionCreate     harness.ActionID = "repository.commit_discussion_create"
	actionRepositoryCommitDiscussionList       harness.ActionID = "repository.commit_discussion_list"
	actionRepositoryCommitDiscussionGet        harness.ActionID = "repository.commit_discussion_get"
	actionRepositoryCommitDiscussionAddNote    harness.ActionID = "repository.commit_discussion_add_note"
	actionRepositoryCommitDiscussionUpdateNote harness.ActionID = "repository.commit_discussion_update_note"
	actionRepositoryCommitDiscussionDeleteNote harness.ActionID = "repository.commit_discussion_delete_note"
)

// A branch's reads, its protection, and the two deletes. The create is
// declared in actions_test.go.
const (
	actionBranchGet             harness.ActionID = "branch.get"
	actionBranchList            harness.ActionID = "branch.list"
	actionBranchGetProtected    harness.ActionID = "branch.get_protected"
	actionBranchUpdateProtected harness.ActionID = "branch.update_protected"
	actionBranchListProtected   harness.ActionID = "branch.list_protected"
	actionBranchUnprotect       harness.ActionID = "branch.unprotect"
	actionBranchDeleteMerged    harness.ActionID = "branch.delete_merged"
	actionBranchDelete          harness.ActionID = "branch.delete"
)

// A tag's lifecycle, its protection, and the signature an unsigned tag has
// none of.
const (
	actionTagGet           harness.ActionID = "tag.get"
	actionTagList          harness.ActionID = "tag.list"
	actionTagDelete        harness.ActionID = "tag.delete"
	actionTagProtect       harness.ActionID = "tag.protect"
	actionTagListProtected harness.ActionID = "tag.list_protected"
	actionTagGetProtected  harness.ActionID = "tag.get_protected"
	actionTagUnprotect     harness.ActionID = "tag.unprotect"
	actionTagGetSignature  harness.ActionID = "tag.get_signature"
)

// The two CI lint reads, which live in the template group.
const (
	actionTemplateLint        harness.ActionID = "template.lint"
	actionTemplateLintProject harness.ActionID = "template.lint_project"
)

// A project's CI variables and a group's, both through the ci_variable
// group.
const (
	actionCIVariableCreate      harness.ActionID = "ci_variable.create"
	actionCIVariableGet         harness.ActionID = "ci_variable.get"
	actionCIVariableList        harness.ActionID = "ci_variable.list"
	actionCIVariableUpdate      harness.ActionID = "ci_variable.update"
	actionCIVariableDelete      harness.ActionID = "ci_variable.delete"
	actionCIVariableGroupList   harness.ActionID = "ci_variable.group_list"
	actionCIVariableGroupCreate harness.ActionID = "ci_variable.group_create"
	actionCIVariableGroupGet    harness.ActionID = "ci_variable.group_get"
	actionCIVariableGroupUpdate harness.ActionID = "ci_variable.group_update"
	actionCIVariableGroupDelete harness.ActionID = "ci_variable.group_delete"
)

// A pipeline's reads and the writes that change one after it was created.
// The create is declared in actions_test.go.
const (
	actionPipelineLatest            harness.ActionID = "pipeline.latest"
	actionPipelineVariables         harness.ActionID = "pipeline.variables"
	actionPipelineUpdateMetadata    harness.ActionID = "pipeline.update_metadata"
	actionPipelineCancel            harness.ActionID = "pipeline.cancel"
	actionPipelineTestReport        harness.ActionID = "pipeline.test_report"
	actionPipelineTestReportSummary harness.ActionID = "pipeline.test_report_summary"
)

// A project's resource groups, which a pipeline job materializes.
const (
	actionPipelineResourceGroupList         harness.ActionID = "pipeline.resource_group_list"
	actionPipelineResourceGroupGet          harness.ActionID = "pipeline.resource_group_get"
	actionPipelineResourceGroupEdit         harness.ActionID = "pipeline.resource_group_edit"
	actionPipelineResourceGroupUpcomingJobs harness.ActionID = "pipeline.resource_group_upcoming_jobs"
)

// A pipeline schedule's lifecycle, its variables, and what runs it.
const (
	actionPipelineScheduleCreate                 harness.ActionID = "pipeline.schedule_create"
	actionPipelineScheduleGet                    harness.ActionID = "pipeline.schedule_get"
	actionPipelineScheduleList                   harness.ActionID = "pipeline.schedule_list"
	actionPipelineScheduleUpdate                 harness.ActionID = "pipeline.schedule_update"
	actionPipelineScheduleCreateVariable         harness.ActionID = "pipeline.schedule_create_variable"
	actionPipelineScheduleEditVariable           harness.ActionID = "pipeline.schedule_edit_variable"
	actionPipelineScheduleDeleteVariable         harness.ActionID = "pipeline.schedule_delete_variable"
	actionPipelineScheduleTakeOwnership          harness.ActionID = "pipeline.schedule_take_ownership"
	actionPipelineScheduleRun                    harness.ActionID = "pipeline.schedule_run"
	actionPipelineScheduleListTriggeredPipelines harness.ActionID = "pipeline.schedule_list_triggered_pipelines"
	actionPipelineScheduleDelete                 harness.ActionID = "pipeline.schedule_delete"
)

// A pipeline trigger token's lifecycle and the pipeline it runs.
const (
	actionPipelineTriggerCreate harness.ActionID = "pipeline.trigger_create"
	actionPipelineTriggerList   harness.ActionID = "pipeline.trigger_list"
	actionPipelineTriggerGet    harness.ActionID = "pipeline.trigger_get"
	actionPipelineTriggerUpdate harness.ActionID = "pipeline.trigger_update"
	actionPipelineTriggerRun    harness.ActionID = "pipeline.trigger_run"
	actionPipelineTriggerDelete harness.ActionID = "pipeline.trigger_delete"
)

// A pipeline's jobs: the reads, the artifact downloads, and the writes on a
// manual job and on a finished one.
const (
	actionJobListBridges                 harness.ActionID = "job.list_bridges"
	actionJobArtifacts                   harness.ActionID = "job.artifacts"
	actionJobDownloadArtifacts           harness.ActionID = "job.download_artifacts"
	actionJobDownloadSingleArtifact      harness.ActionID = "job.download_single_artifact"
	actionJobDownloadSingleArtifactByRef harness.ActionID = "job.download_single_artifact_by_ref"
	actionJobKeepArtifacts               harness.ActionID = "job.keep_artifacts"
	actionJobPlay                        harness.ActionID = "job.play"
	actionJobCancel                      harness.ActionID = "job.cancel"
	actionJobRetry                       harness.ActionID = "job.retry"
	actionJobDeleteArtifacts             harness.ActionID = "job.delete_artifacts"
	actionJobErase                       harness.ActionID = "job.erase"
	actionJobDeleteProjectArtifacts      harness.ActionID = "job.delete_project_artifacts"
	actionJobTokenScopePatch             harness.ActionID = "job.token_scope_patch"
	actionJobTokenScopeListInbound       harness.ActionID = "job.token_scope_list_inbound"
	actionJobTokenScopeAddProject        harness.ActionID = "job.token_scope_add_project"
	actionJobTokenScopeRemoveProject     harness.ActionID = "job.token_scope_remove_project"
	actionJobTokenScopeListGroups        harness.ActionID = "job.token_scope_list_groups"
	actionJobTokenScopeAddGroup          harness.ActionID = "job.token_scope_add_group"
	actionJobTokenScopeRemoveGroup       harness.ActionID = "job.token_scope_remove_group"
)

// An environment's reads and the writes after its creation, which
// actions_test.go declares; the deployments into it; and the project's
// freeze periods, which the environment group carries.
const (
	actionEnvironmentGet              harness.ActionID = "environment.get"
	actionEnvironmentList             harness.ActionID = "environment.list"
	actionEnvironmentUpdate           harness.ActionID = "environment.update"
	actionEnvironmentStop             harness.ActionID = "environment.stop"
	actionEnvironmentDelete           harness.ActionID = "environment.delete"
	actionEnvironmentDeploymentGet    harness.ActionID = "environment.deployment_get"
	actionEnvironmentDeploymentUpdate harness.ActionID = "environment.deployment_update"
	actionEnvironmentDeploymentDelete harness.ActionID = "environment.deployment_delete"
	actionEnvironmentFreezeList       harness.ActionID = "environment.freeze_list"
	actionEnvironmentFreezeCreate     harness.ActionID = "environment.freeze_create"
	actionEnvironmentFreezeGet        harness.ActionID = "environment.freeze_get"
	actionEnvironmentFreezeUpdate     harness.ActionID = "environment.freeze_update"
	actionEnvironmentFreezeDelete     harness.ActionID = "environment.freeze_delete"
)

// A release's lifecycle and its asset links.
const (
	actionReleaseGet             harness.ActionID = "release.get"
	actionReleaseUpdate          harness.ActionID = "release.update"
	actionReleaseList            harness.ActionID = "release.list"
	actionReleaseDelete          harness.ActionID = "release.delete"
	actionReleaseLinkCreate      harness.ActionID = "release.link_create"
	actionReleaseLinkCreateBatch harness.ActionID = "release.link_create_batch"
	actionReleaseLinkList        harness.ActionID = "release.link_list"
	actionReleaseLinkGet         harness.ActionID = "release.link_get"
	actionReleaseLinkUpdate      harness.ActionID = "release.link_update"
	actionReleaseLinkDelete      harness.ActionID = "release.link_delete"
)

// The generic package registry, its protection rules, the container
// registry's repositories and tags, and the two rule families on that
// registry.
const (
	actionPackagePublish               harness.ActionID = "package.publish"
	actionPackagePublishAndLink        harness.ActionID = "package.publish_and_link"
	actionPackagePublishDirectory      harness.ActionID = "package.publish_directory"
	actionPackageList                  harness.ActionID = "package.list"
	actionPackageGroupList             harness.ActionID = "package.group_list"
	actionPackageGet                   harness.ActionID = "package.get"
	actionPackageFileList              harness.ActionID = "package.file_list"
	actionPackageDownload              harness.ActionID = "package.download"
	actionPackageFileDelete            harness.ActionID = "package.file_delete"
	actionPackageDelete                harness.ActionID = "package.delete"
	actionPackageProtectionRuleList    harness.ActionID = "package.protection_rule_list"
	actionPackageProtectionRuleCreate  harness.ActionID = "package.protection_rule_create"
	actionPackageProtectionRuleUpdate  harness.ActionID = "package.protection_rule_update"
	actionPackageProtectionRuleDelete  harness.ActionID = "package.protection_rule_delete"
	actionPackageRegistryListProject   harness.ActionID = "package.registry_list_project"
	actionPackageRegistryListGroup     harness.ActionID = "package.registry_list_group"
	actionPackageRegistryGet           harness.ActionID = "package.registry_get"
	actionPackageRegistryDelete        harness.ActionID = "package.registry_delete"
	actionPackageRegistryTagList       harness.ActionID = "package.registry_tag_list"
	actionPackageRegistryTagGet        harness.ActionID = "package.registry_tag_get"
	actionPackageRegistryTagDelete     harness.ActionID = "package.registry_tag_delete"
	actionPackageRegistryTagDeleteBulk harness.ActionID = "package.registry_tag_delete_bulk"
	actionPackageRegistryRuleList      harness.ActionID = "package.registry_rule_list"
	actionPackageRegistryRuleCreate    harness.ActionID = "package.registry_rule_create"
	actionPackageRegistryRuleUpdate    harness.ActionID = "package.registry_rule_update"
	actionPackageRegistryRuleDelete    harness.ActionID = "package.registry_rule_delete"
	actionPackageRegistryTagRuleList   harness.ActionID = "package.registry_tag_rule_list"
	actionPackageRegistryTagRuleCreate harness.ActionID = "package.registry_tag_rule_create"
	actionPackageRegistryTagRuleUpdate harness.ActionID = "package.registry_tag_rule_update"
	actionPackageRegistryTagRuleDelete harness.ActionID = "package.registry_tag_rule_delete"
)

// A project's deploy keys, through the access group.
const (
	actionAccessDeployKeyGet         harness.ActionID = "access.deploy_key_get"
	actionAccessDeployKeyListProject harness.ActionID = "access.deploy_key_list_project"
	actionAccessDeployKeyUpdate      harness.ActionID = "access.deploy_key_update"
)
