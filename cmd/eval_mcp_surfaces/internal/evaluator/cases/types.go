package cases

import "strings"

// Case is the data-only source of truth for one model-evaluation task.
type Case struct {
	ID               string
	Title            string
	Prompt           string
	PromptTemplate   PromptTemplate
	Steps            []Step
	Fixtures         []string
	Assertions       []Assertion
	Metrics          MetricsSpec
	Edition          string
	Presets          []string
	Partition        string
	Tags             []string
	Mutating         bool
	Destructive      bool
	CapabilityBridge bool
	SkipReasons      []string
	ReportGroup      string
}

// Step describes one expected MCP tool or action call within a case.
type Step struct {
	ExpectedTool    string
	ExpectedAction  string
	RequiredParams  []string
	OptionalParams  []string
	ForbiddenParams []string
	OptionalStep    bool
	Destructive     bool
	Simulation      string
	AllowedRepairs  []string
	ProducedValues  []string
}

// isEnterpriseDynamicCase reports whether the case id belongs to the
// MS-ENT-DYN-* family of cases that target the Enterprise + dynamic
// surface. Use this in edition-gating predicates to keep the prefix
// check consistent across helpers (capabilityEvalCase, errorRecoveryEvalCase,
// baseEnterpriseReadEvalCase).
func isEnterpriseDynamicCase(id string) bool {
	return strings.HasPrefix(id, enterpriseDynamicCasePrefix)
}

// PromptTemplate defines the prompt text rendered with fixture output values.
type PromptTemplate struct {
	Text      string
	Variables []string
}

// Assertion describes one post-call rule attached to a case.
type Assertion struct {
	Type        string
	Step        int
	Name        string
	Description string
	Required    bool
	Inputs      []string
	Expected    map[string]string
}

// MetricsSpec customizes how a case contributes to aggregate evaluator metrics.
type MetricsSpec struct {
	ExpectedModelCalls int
	ExpectedToolCalls  int
	FinalSuccess       bool
}

const (
	editionCE         = "ce"
	editionEnterprise = "enterprise"

	// enterpriseDynamicCasePrefix is the case-id prefix that flags the
	// 10 Enterprise + dynamic surface cases (MS-ENT-DYN-1..10). They
	// target the GitLab EE runtime through the dynamic
	// gitlab_find_action / gitlab_execute_action chain.
	enterpriseDynamicCasePrefix = "MS-ENT-DYN-"

	presetSchemaEnterprise                = "schema-enterprise"
	presetDockerRead                      = "docker-read"
	presetDockerMutatingSafe              = "docker-mutating-safe"
	presetDockerDestructiveSafe           = "docker-destructive-safe"
	presetDockerEnterpriseRead            = "docker-enterprise-read"
	presetDockerEnterpriseMutatingSafe    = "docker-enterprise-mutating-safe"
	presetDockerEnterpriseDestructiveSafe = "docker-enterprise-destructive-safe"
	presetDockerCapabilityDiscovery       = "docker-capability-discovery"
	presetDockerErrorRecovery             = "docker-error-recovery"
	partitionBaseRead                     = "base-read"
	partitionBaseMutating                 = "base-mutating"
	partitionBaseDestructive              = "base-destructive"
	partitionEnterpriseRead               = "enterprise-read"
	partitionEnterpriseMutating           = "enterprise-mutating"
	partitionEnterpriseDestructive        = "enterprise-destructive"
	partitionCapabilityFallback           = "capability-fallback"
	partitionErrorRecovery                = "error-recovery"
	capabilityListTool                    = "gitlab_list_capabilities"
	resourceListTool                      = "gitlab_list_resources"
	resourceReadTool                      = "gitlab_read_resource"
	promptListTool                        = "gitlab_list_prompts"
	promptGetTool                         = "gitlab_get_prompt"
	completionTool                        = "gitlab_complete"
	taskFileCreateID                      = "MT-030"
	taskPackageReleaseID                  = "MS-038"
	fixtureBootstrapProject               = "bootstrap_project"
	fixtureAttemptNames                   = "attempt_names"
	fixtureBranch                         = "branch"
	fixtureFile                           = "file"
	fixtureIssue                          = "issue"
	fixtureMergeRequest                   = "merge_request"
	fixtureMergeRequestDiscussion         = "merge_request_discussion"
	fixturePipelineJob                    = "pipeline_job"
	fixtureRelease                        = "release"
	fixtureTag                            = "tag"
	fixtureCIVariable                     = "ci_variable"
	fixtureHook                           = "hook"
	fixtureBadge                          = "badge"
	fixtureWiki                           = "wiki"
	fixtureSnippet                        = "snippet"
	fixtureFeatureFlag                    = "feature_flag"
	fixtureDeployToken                    = "deploy_token"
	fixtureDeployKey                      = "deploy_key"
	fixturePackage                        = "package"
	fixturePackageRelease                 = "package_release"
	fixturePipelineTrigger                = "pipeline_trigger"
	fixturePipelineSchedule               = "pipeline_schedule"
	fixtureMember                         = "member"
	fixtureMergeRequestSource             = "merge_request_source"
	fixtureReleaseCreateSource            = "release_create_source"
	fixtureMergeableMergeRequest          = "mergeable_merge_request"
	// #nosec G101 -- Evaluator fixture identifier; this is not a credential.
	fixtureJobTokenScopeProject             = "job_token_scope_project"
	fixtureFailedJobArtifact                = "failed_job_artifact"
	fixtureMergeRequestAwardEmoji           = "merge_request_award_emoji"
	fixtureIssueAwardEmoji                  = "issue_award_emoji"
	fixtureGroupDelete                      = "group_delete"
	fixtureIssueDelete                      = "issue_delete"
	fixtureProjectCIVariableDelete          = "project_ci_variable_delete"
	fixtureRepositoryFileDelete             = "repository_file_delete"
	fixtureMilestoneDelete                  = "milestone_delete"
	fixtureReleaseDelete                    = "release_delete"
	fixtureProjectAccessTokenRevoke         = "project_access_token_revoke"
	fixtureProjectArchive                   = "project_archive"
	fixturePackageDelete                    = "package_delete"
	fixturePipelineDelete                   = "pipeline_delete"
	fixturePipelineTriggerDelete            = "pipeline_trigger_delete"
	fixturePipelineScheduleDelete           = "pipeline_schedule_delete"
	fixtureRunnerRemove                     = "runner_remove"
	fixtureEnvironmentStop                  = "environment_stop"
	fixtureSnippetDelete                    = "snippet_delete"
	fixtureBroadcastMessageDelete           = "broadcast_message_delete"
	fixtureProjectHookDelete                = "project_hook_delete"
	fixtureProjectBadgeDelete               = "project_badge_delete"
	fixtureDraftNotePublishAll              = "draft_note_publish_all"
	fixtureInstanceCIVariableDelete         = "instance_ci_variable_delete"
	fixtureBranchDelete                     = "branch_delete"
	fixtureTagDelete                        = "tag_delete"
	fixtureUserBlock                        = "user_block"
	fixtureFeatureFlagDelete                = "feature_flag_delete"
	fixtureWikiDelete                       = "wiki_delete"
	fixtureDeployKeyLifecycle               = "deploy_key_lifecycle"
	fixtureDeployKeyDelete                  = "deploy_key_delete"
	fixtureDeployTokenDelete                = "deploy_token_delete"
	fixtureCommitDiscussionDeleteNote       = "commit_discussion_delete_note"
	fixtureBranchProtectionLifecycle        = "branch_protection_lifecycle"
	fixtureProjectServiceAccount            = "project_service_account"
	fixtureEnterprisePushRuleProject        = "enterprise_push_rule_project"
	fixtureEnterprisePushRuleProjectSeeded  = "enterprise_push_rule_project_seeded"
	fixtureEnterpriseGroupServiceAccount    = "enterprise_group_service_account"
	fixtureEnterpriseGroupServiceAccountPAT = "enterprise_group_service_account_pat"
)
