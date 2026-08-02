package projects

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// hintVerifyProjectExists is the 404 hint shared by project tools.
const hintVerifyProjectExists = "verify the project exists with gitlab_project_get"

// boolToAccessLevel converts a bool pointer to an AccessControlValue pointer
// for bridging legacy bool-based tool inputs to the modern AccessLevel API.
func boolToAccessLevel(b *bool) *gl.AccessControlValue {
	if b == nil {
		return nil
	}
	if *b {
		return new(gl.EnabledAccessControl)
	}
	return new(gl.DisabledAccessControl)
}

// accessLevelEnabled returns true if the access level indicates the feature is not disabled.
func accessLevelEnabled(v gl.AccessControlValue) bool {
	return v != "" && v != gl.DisabledAccessControl
}

// ContainerExpirationPolicyInput mirrors the GitLab
// [gl.ContainerExpirationPolicyAttributes] options for configuring the
// container registry cleanup policy when creating or updating a project.
type ContainerExpirationPolicyInput struct {
	Cadence         string `json:"cadence,omitempty" jsonschema:"Cleanup cadence (1d, 7d, 14d, 1month, 3month)"`
	KeepN           *int64 `json:"keep_n,omitempty" jsonschema:"Number of most recent tags to always keep per image name"`
	OlderThan       string `json:"older_than,omitempty" jsonschema:"Only tags older than this duration are eligible for deletion (7d, 14d, 30d, 90d)"`
	NameRegexDelete string `json:"name_regex_delete,omitempty" jsonschema:"Regex matching tag names to delete"`
	NameRegexKeep   string `json:"name_regex_keep,omitempty" jsonschema:"Regex matching tag names to always keep"`
	Enabled         *bool  `json:"enabled,omitempty" jsonschema:"Enable the container expiration policy"`
}

// toGL converts the input into the GitLab
// [gl.ContainerExpirationPolicyAttributes] options, returning nil when no
// field is set.
func (c *ContainerExpirationPolicyInput) toGL() *gl.ContainerExpirationPolicyAttributes {
	if c == nil {
		return nil
	}
	out := &gl.ContainerExpirationPolicyAttributes{}
	set := false
	if c.Cadence != "" {
		out.Cadence = new(c.Cadence)
		set = true
	}
	if c.KeepN != nil {
		out.KeepN = c.KeepN
		set = true
	}
	if c.OlderThan != "" {
		out.OlderThan = new(c.OlderThan)
		set = true
	}
	if c.NameRegexDelete != "" {
		out.NameRegexDelete = new(c.NameRegexDelete)
		set = true
	}
	if c.NameRegexKeep != "" {
		out.NameRegexKeep = new(c.NameRegexKeep)
		set = true
	}
	if c.Enabled != nil {
		out.Enabled = c.Enabled
		set = true
	}
	if !set {
		return nil
	}
	return out
}

// CreateInput defines parameters for creating a GitLab project. Fields mirror
// the GitLab [gl.CreateProjectOptions] options 1:1.
type CreateInput struct {
	// Basic metadata
	Name                 string   `json:"name" jsonschema:"Project name,required"`
	Path                 string   `json:"path,omitempty" jsonschema:"Project path slug (defaults from name)"`
	NamespaceID          int      `json:"namespace_id,omitempty" jsonschema:"Namespace ID (defaults to personal namespace)"`
	Description          string   `json:"description,omitempty" jsonschema:"Project description"`
	Visibility           string   `json:"visibility,omitempty" jsonschema:"Visibility level (private, internal, public)"`
	InitializeWithReadme bool     `json:"initialize_with_readme,omitempty" jsonschema:"Initialize with a README"`
	DefaultBranch        string   `json:"default_branch,omitempty" jsonschema:"Default branch name"`
	Topics               []string `json:"topics,omitempty" jsonschema:"Topic tags for the project"`
	ImportURL            string   `json:"import_url,omitempty" jsonschema:"URL to import repository from"`
	RepositoryStorage    string   `json:"repository_storage,omitempty" jsonschema:"Repository storage shard name (admin only)"`

	// Template provisioning
	TemplateName                string `json:"template_name,omitempty" jsonschema:"Built-in project template name to use"`
	TemplateProjectID           int64  `json:"template_project_id,omitempty" jsonschema:"Custom project template ID to use"`
	UseCustomTemplate           *bool  `json:"use_custom_template,omitempty" jsonschema:"Use a custom group/instance project template"`
	GroupWithProjectTemplatesID int64  `json:"group_with_project_templates_id,omitempty" jsonschema:"Group ID that provides the custom project templates"`

	// Merge settings
	MergeMethod                               string `json:"merge_method,omitempty" jsonschema:"Merge method (merge, rebase_merge, ff)"`
	SquashOption                              string `json:"squash_option,omitempty" jsonschema:"Squash option (never, always, default_on, default_off)"`
	OnlyAllowMergeIfPipelineSucceeds          bool   `json:"only_allow_merge_if_pipeline_succeeds,omitempty" jsonschema:"Only allow merge when pipeline succeeds"`
	OnlyAllowMergeIfAllDiscussionsAreResolved bool   `json:"only_allow_merge_if_all_discussions_are_resolved,omitempty" jsonschema:"Only allow merge when all discussions are resolved"`
	OnlyAllowMergeIfAllStatusChecksPassed     *bool  `json:"only_allow_merge_if_all_status_checks_passed,omitempty" tier:"ultimate" jsonschema:"Only allow merge when all external status checks pass"`
	AllowMergeOnSkippedPipeline               *bool  `json:"allow_merge_on_skipped_pipeline,omitempty" jsonschema:"Allow merge when pipeline is skipped"`
	ProtectMergeRequestPipelines              *bool  `json:"protect_merge_request_pipelines,omitempty" jsonschema:"Prevent merge request pipeline settings from being modified by users with lower permissions"`
	RemoveSourceBranchAfterMerge              *bool  `json:"remove_source_branch_after_merge,omitempty" jsonschema:"Remove source branch after merge by default"`
	AutocloseReferencedIssues                 *bool  `json:"autoclose_referenced_issues,omitempty" jsonschema:"Auto-close referenced issues on merge"`
	ResolveOutdatedDiffDiscussions            *bool  `json:"resolve_outdated_diff_discussions,omitempty" jsonschema:"Auto-resolve outdated diff discussions"`
	PrintingMergeRequestLinkEnabled           *bool  `json:"printing_merge_request_link_enabled,omitempty" jsonschema:"Show the create-merge-request link after pushing"`
	MergePipelinesEnabled                     *bool  `json:"merge_pipelines_enabled,omitempty" jsonschema:"Enable merged results pipelines"`
	MergeTrainsEnabled                        *bool  `json:"merge_trains_enabled,omitempty" jsonschema:"Enable merge trains"`
	MergeTrainsSkipTrainAllowed               *bool  `json:"merge_trains_skip_train_allowed,omitempty" jsonschema:"Allow skipping the merge train"`
	MergeCommitTemplate                       string `json:"merge_commit_template,omitempty" jsonschema:"Template for merge commit messages"`
	SquashCommitTemplate                      string `json:"squash_commit_template,omitempty" jsonschema:"Template for squash commit messages"`
	MRDefaultTitleTemplate                    string `json:"mr_default_title_template,omitempty" jsonschema:"Template for default merge request titles"`
	SuggestionCommitMessage                   string `json:"suggestion_commit_message,omitempty" jsonschema:"Default commit message for suggestions"`
	IssueBranchTemplate                       string `json:"issue_branch_template,omitempty" jsonschema:"Template for branch names created from issues"`
	ApprovalsBeforeMerge                      int64  `json:"approvals_before_merge,omitempty" tier:"premium" jsonschema:"Number of approvals required before merge (deprecated: use Merge Request Approvals API)"`
	MergeRequestTitleRegex                    string `json:"merge_request_title_regex,omitempty" jsonschema:"Regex that MR titles must match"`
	MergeRequestTitleRegexDescription         string `json:"merge_request_title_regex_description,omitempty" jsonschema:"Human-readable description for the MR title regex"`
	ReviewerAssignmentStrategy                string `json:"reviewer_assignment_strategy,omitempty" tier:"premium" jsonschema:"Reviewer assignment strategy for merge requests (disabled, code_owners, dap_powered)"`

	// Feature toggles
	IssuesEnabled              *bool  `json:"issues_enabled,omitempty" jsonschema:"Enable issues feature (deprecated: use issues_access_level)"`
	MergeRequestsEnabled       *bool  `json:"merge_requests_enabled,omitempty" jsonschema:"Enable merge requests feature (deprecated: use merge_requests_access_level)"`
	WikiEnabled                *bool  `json:"wiki_enabled,omitempty" jsonschema:"Enable wiki feature (deprecated: use wiki_access_level)"`
	JobsEnabled                *bool  `json:"jobs_enabled,omitempty" jsonschema:"Enable CI/CD jobs (deprecated: use builds_access_level)"`
	SnippetsEnabled            *bool  `json:"snippets_enabled,omitempty" jsonschema:"Enable snippets feature (deprecated: use snippets_access_level)"`
	ContainerRegistryEnabled   *bool  `json:"container_registry_enabled,omitempty" jsonschema:"Enable container registry (deprecated: use container_registry_access_level)"`
	LFSEnabled                 *bool  `json:"lfs_enabled,omitempty" jsonschema:"Enable Git LFS"`
	ServiceDeskEnabled         *bool  `json:"service_desk_enabled,omitempty" jsonschema:"Enable Service Desk"`
	EmailsEnabled              *bool  `json:"emails_enabled,omitempty" jsonschema:"Enable email notifications"`
	EmailsDisabled             *bool  `json:"emails_disabled,omitempty" jsonschema:"Disable email notifications (deprecated: use emails_enabled)"`
	ShowDefaultAwardEmojis     *bool  `json:"show_default_award_emojis,omitempty" jsonschema:"Show default award emojis on issues/MRs"`
	PackagesEnabled            *bool  `json:"packages_enabled,omitempty" jsonschema:"Enable packages feature (deprecated: use package_registry_access_level)"`
	PackageRegistryAccessLevel string `json:"package_registry_access_level,omitempty" jsonschema:"Package registry access level (disabled, private, enabled)"`

	// Auto DevOps
	AutoDevopsEnabled        *bool  `json:"auto_devops_enabled,omitempty" jsonschema:"Enable Auto DevOps for the project"`
	AutoDevopsDeployStrategy string `json:"auto_devops_deploy_strategy,omitempty" jsonschema:"Auto DevOps deploy strategy (continuous, manual, timed_incremental)"`

	// CI/CD settings
	CIConfigPath                    string `json:"ci_config_path,omitempty" jsonschema:"Custom CI/CD configuration file path"`
	CICDCatalogEnabled              *bool  `json:"cicd_catalog_enabled,omitempty" jsonschema:"Publish the project as a CI/CD catalog resource"`
	BuildTimeout                    int64  `json:"build_timeout,omitempty" jsonschema:"Build timeout in seconds"`
	BuildGitStrategy                string `json:"build_git_strategy,omitempty" jsonschema:"Git strategy for builds (fetch, clone)"`
	BuildCoverageRegex              string `json:"build_coverage_regex,omitempty" jsonschema:"Regex used to extract test coverage from job logs"`
	AutoCancelPendingPipelines      string `json:"auto_cancel_pending_pipelines,omitempty" jsonschema:"Auto-cancel pending pipelines (enabled, disabled)"`
	CIForwardDeploymentEnabled      *bool  `json:"ci_forward_deployment_enabled,omitempty" jsonschema:"Enable CI/CD forward deployment (deprecated: no longer supported in recent versions)"`
	ResourceGroupDefaultProcessMode string `json:"resource_group_default_process_mode,omitempty" jsonschema:"Default resource group process mode (unordered, oldest_first, newest_first)"`
	SharedRunnersEnabled            *bool  `json:"shared_runners_enabled,omitempty" jsonschema:"Enable shared runners"`
	GroupRunnersEnabled             *bool  `json:"group_runners_enabled,omitempty" jsonschema:"Enable group runners"`
	PublicBuilds                    *bool  `json:"public_builds,omitempty" jsonschema:"Enable public access to pipelines (deprecated: use public_jobs)"`
	PublicJobs                      *bool  `json:"public_jobs,omitempty" jsonschema:"Enable public access to pipelines"`

	// Mirroring
	Mirror              *bool `json:"mirror,omitempty" jsonschema:"Enable pull mirroring from import_url"`
	MirrorTriggerBuilds *bool `json:"mirror_trigger_builds,omitempty" jsonschema:"Trigger pipelines for mirror updates"`

	// Container registry cleanup
	ContainerExpirationPolicyAttributes *ContainerExpirationPolicyInput `json:"container_expiration_policy_attributes,omitempty" jsonschema:"Container registry cleanup policy attributes"`

	// Compliance and security
	EnforceAuthChecksOnUploads               *bool  `json:"enforce_auth_checks_on_uploads,omitempty" jsonschema:"Enforce authorization checks on uploads"`
	ExternalAuthorizationClassificationLabel string `json:"external_authorization_classification_label,omitempty" jsonschema:"External authorization classification label"`

	// Access control
	RequestAccessEnabled             *bool  `json:"request_access_enabled,omitempty" jsonschema:"Allow users to request access"`
	PagesAccessLevel                 string `json:"pages_access_level,omitempty" jsonschema:"Pages access level (disabled, private, enabled, public)"`
	ContainerRegistryAccessLevel     string `json:"container_registry_access_level,omitempty" jsonschema:"Container registry access level (disabled, private, enabled)"`
	SnippetsAccessLevel              string `json:"snippets_access_level,omitempty" jsonschema:"Snippets access level (disabled, private, enabled)"`
	IssuesAccessLevel                string `json:"issues_access_level,omitempty" jsonschema:"Issues access level (disabled, private, enabled)"`
	MergeRequestsAccessLevel         string `json:"merge_requests_access_level,omitempty" jsonschema:"Merge requests access level (disabled, private, enabled)"`
	WikiAccessLevel                  string `json:"wiki_access_level,omitempty" jsonschema:"Wiki access level (disabled, private, enabled)"`
	BuildsAccessLevel                string `json:"builds_access_level,omitempty" jsonschema:"CI/CD builds access level (disabled, private, enabled)"`
	RepositoryAccessLevel            string `json:"repository_access_level,omitempty" jsonschema:"Repository access level (disabled, private, enabled)"`
	ForkingAccessLevel               string `json:"forking_access_level,omitempty" jsonschema:"Forking access level (disabled, private, enabled)"`
	AnalyticsAccessLevel             string `json:"analytics_access_level,omitempty" jsonschema:"Analytics access level (disabled, private, enabled)"`
	OperationsAccessLevel            string `json:"operations_access_level,omitempty" jsonschema:"Operations access level (disabled, private, enabled)"`
	ReleasesAccessLevel              string `json:"releases_access_level,omitempty" jsonschema:"Releases access level (disabled, private, enabled)"`
	EnvironmentsAccessLevel          string `json:"environments_access_level,omitempty" jsonschema:"Environments access level (disabled, private, enabled)"`
	FeatureFlagsAccessLevel          string `json:"feature_flags_access_level,omitempty" jsonschema:"Feature flags access level (disabled, private, enabled)"`
	InfrastructureAccessLevel        string `json:"infrastructure_access_level,omitempty" jsonschema:"Infrastructure access level (disabled, private, enabled)"`
	MonitorAccessLevel               string `json:"monitor_access_level,omitempty" jsonschema:"Monitor access level (disabled, private, enabled)"`
	RequirementsAccessLevel          string `json:"requirements_access_level,omitempty" jsonschema:"Requirements access level (disabled, private, enabled)"`
	SecurityAndComplianceAccessLevel string `json:"security_and_compliance_access_level,omitempty" jsonschema:"Security and compliance access level (disabled, private, enabled)"`
	ModelExperimentsAccessLevel      string `json:"model_experiments_access_level,omitempty" jsonschema:"Model experiments access level (disabled, private, enabled)"`
	ModelRegistryAccessLevel         string `json:"model_registry_access_level,omitempty" jsonschema:"Model registry access level (disabled, private, enabled)"`

	// Deprecated template fields surfaced for 1:1 SDK parity
	IssuesTemplate        string   `json:"issues_template,omitempty" jsonschema:"Default issue description template (deprecated: no longer supported in recent versions)"`
	MergeRequestsTemplate string   `json:"merge_requests_template,omitempty" jsonschema:"Default merge request description template (deprecated: no longer supported in recent versions)"`
	TagList               []string `json:"tag_list,omitempty" jsonschema:"Project tags (deprecated: use topics)"`
}

// Output is the common output for project operations. It mirrors gl.Project
// field-for-field (1:1 audit policy), surfacing every scalar plus full nested
// objects on their canonical keys (namespace, owner, permissions,
// forked_from_project, _links, statistics, shared_with_groups, license,
// container_expiration_policy, custom_attributes).
type Output struct {
	toolutil.HintableOutput
	ID                                        int64    `json:"id"`
	Name                                      string   `json:"name"`
	Path                                      string   `json:"path"`
	PathWithNamespace                         string   `json:"path_with_namespace"`
	NameWithNamespace                         string   `json:"name_with_namespace,omitempty"`
	Visibility                                string   `json:"visibility"`
	DefaultBranch                             string   `json:"default_branch"`
	WebURL                                    string   `json:"web_url"`
	Description                               string   `json:"description"`
	Archived                                  bool     `json:"archived"`
	EmptyRepo                                 bool     `json:"empty_repo,omitempty"`
	ForksCount                                int64    `json:"forks_count,omitempty"`
	StarCount                                 int64    `json:"star_count,omitempty"`
	OpenIssuesCount                           int64    `json:"open_issues_count,omitempty"`
	HTTPURLToRepo                             string   `json:"http_url_to_repo,omitempty"`
	SSHURLToRepo                              string   `json:"ssh_url_to_repo,omitempty"`
	Topics                                    []string `json:"topics"`
	MergeMethod                               string   `json:"merge_method,omitempty"`
	SquashOption                              string   `json:"squash_option,omitempty"`
	OnlyAllowMergeIfPipelineSucceeds          bool     `json:"only_allow_merge_if_pipeline_succeeds"`
	OnlyAllowMergeIfAllDiscussionsAreResolved bool     `json:"only_allow_merge_if_all_discussions_are_resolved"`
	RemoveSourceBranchAfterMerge              bool     `json:"remove_source_branch_after_merge"`
	MarkedForDeletionOn                       string   `json:"marked_for_deletion_on,omitempty"`
	CreatedAt                                 string   `json:"created_at"`
	UpdatedAt                                 string   `json:"updated_at,omitempty"`
	LastActivityAt                            string   `json:"last_activity_at,omitempty"`
	ReadmeURL                                 string   `json:"readme_url,omitempty"`
	AvatarURL                                 string   `json:"avatar_url,omitempty"`
	CreatorID                                 int64    `json:"creator_id,omitempty"`
	RequestAccessEnabled                      bool     `json:"request_access_enabled"`
	LFSEnabled                                bool     `json:"lfs_enabled"`
	CIConfigPath                              string   `json:"ci_config_path,omitempty"`
	AllowMergeOnSkippedPipeline               bool     `json:"allow_merge_on_skipped_pipeline"`
	MergePipelinesEnabled                     bool     `json:"merge_pipelines_enabled" tier:"premium"`
	MergeTrainsEnabled                        bool     `json:"merge_trains_enabled" tier:"premium"`
	ProtectMergeRequestPipelines              *bool    `json:"protect_merge_request_pipelines,omitempty"`
	MergeCommitTemplate                       string   `json:"merge_commit_template,omitempty"`
	SquashCommitTemplate                      string   `json:"squash_commit_template,omitempty"`
	MRDefaultTitleTemplate                    string   `json:"mr_default_title_template,omitempty"`
	AutocloseReferencedIssues                 bool     `json:"autoclose_referenced_issues"`
	ResolveOutdatedDiffDiscussions            bool     `json:"resolve_outdated_diff_discussions"`
	SharedRunnersEnabled                      bool     `json:"shared_runners_enabled,omitempty"`
	PackageRegistryAccessLevel                string   `json:"package_registry_access_level,omitempty"`
	BuildTimeout                              int64    `json:"build_timeout,omitempty"`
	SuggestionCommitMessage                   string   `json:"suggestion_commit_message,omitempty"`
	ComplianceFrameworks                      []string `json:"compliance_frameworks,omitempty" tier:"premium"`
	ImportURL                                 string   `json:"import_url,omitempty"`
	MergeRequestTitleRegex                    string   `json:"merge_request_title_regex,omitempty" tier:"premium"`
	MergeRequestTitleRegexDescription         string   `json:"merge_request_title_regex_description,omitempty" tier:"premium"`
	ReviewerAssignmentStrategy                string   `json:"reviewer_assignment_strategy,omitempty" tier:"premium"`

	// Access-level enums (1:1 SDK parity).
	ContainerRegistryAccessLevel     string `json:"container_registry_access_level,omitempty"`
	IssuesAccessLevel                string `json:"issues_access_level,omitempty"`
	ReleasesAccessLevel              string `json:"releases_access_level,omitempty"`
	RepositoryAccessLevel            string `json:"repository_access_level,omitempty"`
	MergeRequestsAccessLevel         string `json:"merge_requests_access_level,omitempty"`
	ForkingAccessLevel               string `json:"forking_access_level,omitempty"`
	WikiAccessLevel                  string `json:"wiki_access_level,omitempty"`
	BuildsAccessLevel                string `json:"builds_access_level,omitempty"`
	SnippetsAccessLevel              string `json:"snippets_access_level,omitempty"`
	PagesAccessLevel                 string `json:"pages_access_level,omitempty"`
	OperationsAccessLevel            string `json:"operations_access_level,omitempty"`
	AnalyticsAccessLevel             string `json:"analytics_access_level,omitempty"`
	EnvironmentsAccessLevel          string `json:"environments_access_level,omitempty"`
	FeatureFlagsAccessLevel          string `json:"feature_flags_access_level,omitempty"`
	InfrastructureAccessLevel        string `json:"infrastructure_access_level,omitempty"`
	MonitorAccessLevel               string `json:"monitor_access_level,omitempty"`
	RequirementsAccessLevel          string `json:"requirements_access_level,omitempty" tier:"ultimate"`
	SecurityAndComplianceAccessLevel string `json:"security_and_compliance_access_level,omitempty"`
	ModelExperimentsAccessLevel      string `json:"model_experiments_access_level,omitempty"`
	ModelRegistryAccessLevel         string `json:"model_registry_access_level,omitempty"`

	// CI/CD settings (1:1 SDK parity).
	CIDisplayPipelineVariables               bool     `json:"ci_display_pipeline_variables"`
	AllowPipelineTriggerApproveDeployment    bool     `json:"allow_pipeline_trigger_approve_deployment" tier:"premium"`
	PreventMergeWithoutJiraIssue             bool     `json:"prevent_merge_without_jira_issue" tier:"ultimate"`
	PrintingMergeRequestLinkEnabled          bool     `json:"printing_merge_request_link_enabled"`
	CICDCatalogEnabled                       bool     `json:"cicd_catalog_enabled"`
	CIDefaultGitDepth                        int64    `json:"ci_default_git_depth,omitempty"`
	CIDeletePipelinesInSeconds               int64    `json:"ci_delete_pipelines_in_seconds,omitempty"`
	CIForwardDeploymentEnabled               bool     `json:"ci_forward_deployment_enabled"`
	CIForwardDeploymentRollbackAllowed       bool     `json:"ci_forward_deployment_rollback_allowed"`
	CIPushRepositoryForJobTokenAllowed       bool     `json:"ci_push_repository_for_job_token_allowed"`
	CIIDTokenSubClaimComponents              []string `json:"ci_id_token_sub_claim_components,omitempty"`
	CISeparatedCaches                        bool     `json:"ci_separated_caches"`
	CIJobTokenScopeEnabled                   bool     `json:"ci_job_token_scope_enabled"`
	CIOptInJWT                               bool     `json:"ci_opt_in_jwt"`
	CIAllowForkPipelinesToRunInParentProject bool     `json:"ci_allow_fork_pipelines_to_run_in_parent_project"`
	CIRestrictPipelineCancellationRole       string   `json:"ci_restrict_pipeline_cancellation_role,omitempty" tier:"premium"`
	CIPipelineVariablesMinimumOverrideRole   string   `json:"ci_pipeline_variables_minimum_override_role,omitempty"`
	BuildCoverageRegex                       string   `json:"build_coverage_regex,omitempty"`
	BuildGitStrategy                         string   `json:"build_git_strategy,omitempty"`
	AutoCancelPendingPipelines               string   `json:"auto_cancel_pending_pipelines,omitempty"`
	AutoDevopsDeployStrategy                 string   `json:"auto_devops_deploy_strategy,omitempty"`
	AutoDevopsEnabled                        bool     `json:"auto_devops_enabled"`
	KeepLatestArtifact                       bool     `json:"keep_latest_artifact"`
	MergeTrainsSkipTrainAllowed              bool     `json:"merge_trains_skip_train_allowed" tier:"premium"`
	PublicJobs                               bool     `json:"public_jobs"`
	MaxArtifactsSize                         int64    `json:"max_artifacts_size,omitempty"`

	// Mirror settings (1:1 SDK parity).
	Mirror                           bool  `json:"mirror" tier:"premium"`
	MirrorUserID                     int64 `json:"mirror_user_id,omitempty" tier:"premium"`
	MirrorTriggerBuilds              bool  `json:"mirror_trigger_builds" tier:"premium"`
	OnlyMirrorProtectedBranches      bool  `json:"only_mirror_protected_branches" tier:"premium"`
	MirrorOverwritesDivergedBranches bool  `json:"mirror_overwrites_diverged_branches" tier:"premium"`

	// Import status (1:1 SDK parity).
	ImportType   string `json:"import_type,omitempty"`
	ImportStatus string `json:"import_status,omitempty"`
	ImportError  string `json:"import_error,omitempty"`

	// Container registry (1:1 SDK parity).
	ContainerRegistryImagePrefix string `json:"container_registry_image_prefix,omitempty"`

	// Security and compliance (1:1 SDK parity).
	SecurityAndComplianceEnabled     bool `json:"security_and_compliance_enabled" tier:"ultimate"`
	EnforceAuthChecksOnUploads       bool `json:"enforce_auth_checks_on_uploads,omitempty"`
	PreReceiveSecretDetectionEnabled bool `json:"pre_receive_secret_detection_enabled" tier:"ultimate"`
	AutoDuoCodeReviewEnabled         bool `json:"auto_duo_code_review_enabled"`

	// Service desk (1:1 SDK parity).
	ServiceDeskEnabled bool   `json:"service_desk_enabled"`
	ServiceDeskAddress string `json:"service_desk_address,omitempty"`

	// Templates and misc (1:1 SDK parity).
	IssuesTemplate                           string `json:"issues_template,omitempty" tier:"premium"`
	MergeRequestsTemplate                    string `json:"merge_requests_template,omitempty" tier:"premium"`
	IssueBranchTemplate                      string `json:"issue_branch_template,omitempty"`
	ExternalAuthorizationClassificationLabel string `json:"external_authorization_classification_label,omitempty" tier:"premium"`
	RequirementsEnabled                      bool   `json:"requirements_enabled" tier:"ultimate"`
	EmailsEnabled                            bool   `json:"emails_enabled"`
	GroupRunnersEnabled                      bool   `json:"group_runners_enabled"`
	ResourceGroupDefaultProcessMode          string `json:"resource_group_default_process_mode,omitempty"`
	RunnerTokenExpirationInterval            int64  `json:"runner_token_expiration_interval,omitempty"`
	RunnersToken                             string `json:"runners_token,omitempty"`
	RepositoryStorage                        string `json:"repository_storage,omitempty"`
	CanCreateMergeRequestIn                  bool   `json:"can_create_merge_request_in"`
	LicenseURL                               string `json:"license_url,omitempty"`
	MergeRequestDefaultTargetSelf            bool   `json:"mr_default_target_self"`

	// Nested objects (1:1 SDK parity, full mirrors).
	Namespace                 *NamespaceOutput                 `json:"namespace,omitempty"`
	Owner                     *OwnerOutput                     `json:"owner,omitempty"`
	Permissions               *PermissionsOutput               `json:"permissions,omitempty"`
	ForkedFromProject         *ForkParentOutput                `json:"forked_from_project,omitempty"`
	Links                     *LinksOutput                     `json:"_links,omitempty"`
	Statistics                *StatisticsOutput                `json:"statistics,omitempty"`
	SharedWithGroups          []SharedWithGroupOutput          `json:"shared_with_groups,omitempty"`
	License                   *LicenseOutput                   `json:"license,omitempty"`
	ContainerExpirationPolicy *ContainerExpirationPolicyOutput `json:"container_expiration_policy,omitempty"`
	CustomAttributes          []CustomAttributeOutput          `json:"custom_attributes,omitempty"`

	// Deprecated SDK fields, surfaced additively for 1:1 parity.
	IssuesEnabled                bool     `json:"issues_enabled"`
	MergeRequestsEnabled         bool     `json:"merge_requests_enabled"`
	WikiEnabled                  bool     `json:"wiki_enabled"`
	JobsEnabled                  bool     `json:"jobs_enabled"`
	SnippetsEnabled              bool     `json:"snippets_enabled,omitempty"`
	ContainerRegistryEnabled     bool     `json:"container_registry_enabled,omitempty"`
	PackagesEnabled              bool     `json:"packages_enabled,omitempty"`
	PublicBuilds                 bool     `json:"public_builds,omitempty"`
	ApprovalsBeforeMerge         int64    `json:"approvals_before_merge,omitempty" tier:"premium"`
	TagList                      []string `json:"tag_list,omitempty"`
	MarkedForDeletionAt          string   `json:"marked_for_deletion_at,omitempty"`
	RestrictUserDefinedVariables bool     `json:"restrict_user_defined_variables"`
	EmailsDisabled               bool     `json:"emails_disabled"`
}

// GetInput defines parameters for retrieving a project.
type GetInput struct {
	ProjectID            toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path (e.g. 'user/repo' or 42),required"`
	Statistics           *bool                `json:"statistics,omitempty" jsonschema:"Include project statistics (commit count, storage sizes)"`
	License              *bool                `json:"license,omitempty" jsonschema:"Include license information in response"`
	WithCustomAttributes *bool                `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in response"`
}

// ListInput defines filters for listing projects.
type ListInput struct {
	Owned                    bool   `json:"owned,omitempty"      jsonschema:"Limit to projects explicitly owned by the current user"`
	Search                   string `json:"search,omitempty"     jsonschema:"Search query for project name"`
	Visibility               string `json:"visibility,omitempty" jsonschema:"Filter by visibility (private, internal, public)"`
	Archived                 *bool  `json:"archived,omitempty"   jsonschema:"Filter by archived status (true=only archived, false=only active)"`
	OrderBy                  string `json:"order_by,omitempty"   jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort                     string `json:"sort,omitempty"       jsonschema:"Sort direction (asc, desc)"`
	Topic                    string `json:"topic,omitempty"      jsonschema:"Filter by topic name"`
	Simple                   bool   `json:"simple,omitempty"     jsonschema:"Return only limited fields (faster for large result sets)"`
	MinAccessLevel           int    `json:"min_access_level,omitempty" jsonschema:"Filter by minimum access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	LastActivityAfter        string `json:"last_activity_after,omitempty"  jsonschema:"Return projects with last activity after date (ISO 8601 format)"`
	LastActivityBefore       string `json:"last_activity_before,omitempty" jsonschema:"Return projects with last activity before date (ISO 8601 format)"`
	Starred                  *bool  `json:"starred,omitempty"              jsonschema:"Limit to projects starred by the current user"`
	Membership               *bool  `json:"membership,omitempty"           jsonschema:"Limit to projects where current user is a member"`
	WithIssuesEnabled        *bool  `json:"with_issues_enabled,omitempty"  jsonschema:"Filter by projects with issues feature enabled"`
	WithMergeRequestsEnabled *bool  `json:"with_merge_requests_enabled,omitempty" jsonschema:"Filter by projects with merge requests enabled"`
	SearchNamespaces         *bool  `json:"search_namespaces,omitempty"    jsonschema:"Include namespace in search"`
	Statistics               *bool  `json:"statistics,omitempty"           jsonschema:"Include project statistics in response"`
	WithProgrammingLanguage  string `json:"with_programming_language,omitempty" jsonschema:"Filter by programming language name"`
	IncludePendingDelete     *bool  `json:"include_pending_delete,omitempty"    jsonschema:"Include projects that are marked/scheduled for deletion. Default false."`
	IncludeHidden            *bool  `json:"include_hidden,omitempty"            jsonschema:"Include hidden projects in results"`
	IDAfter                  int64  `json:"id_after,omitempty"                  jsonschema:"Return projects with ID greater than this value (keyset pagination)"`
	IDBefore                 int64  `json:"id_before,omitempty"                 jsonschema:"Return projects with ID less than this value (keyset pagination)"`

	// Additional filters (additive 1:1 SDK parity with ListProjectsOptions)
	Active                   *bool             `json:"active,omitempty"                    jsonschema:"Limit to active (non-archived, non-pending-deletion) projects"`
	Imported                 *bool             `json:"imported,omitempty"                  jsonschema:"Limit to projects imported from an external source by the current user"`
	RepositoryChecksumFailed *bool             `json:"repository_checksum_failed,omitempty" jsonschema:"Limit to projects where the repository checksum calculation failed (admin only)"`
	WikiChecksumFailed       *bool             `json:"wiki_checksum_failed,omitempty"       jsonschema:"Limit to projects where the wiki checksum calculation failed (admin only)"`
	RepositoryStorage        string            `json:"repository_storage,omitempty"         jsonschema:"Limit to projects on the given repository storage shard (admin only)"`
	WithCustomAttributes     *bool             `json:"with_custom_attributes,omitempty"     jsonschema:"Include custom attributes in the response (admin only)"`
	CustomAttributes         map[string]string `json:"custom_attributes,omitempty"     jsonschema:"Filter projects by custom attribute key/value pairs (admin only). Different from with_custom_attributes, which only controls whether attributes are returned"`

	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListOutput holds a paginated list of projects.
type ListOutput struct {
	toolutil.HintableOutput
	Projects   []Output                  `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// DeleteInput defines parameters for deleting a project.
type DeleteInput struct {
	ProjectID         toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PermanentlyRemove bool                 `json:"permanently_remove,omitempty" jsonschema:"If true, immediately and permanently delete the project bypassing delayed deletion. Requires admin permissions on some GitLab instances."`
	FullPath          string               `json:"full_path,omitempty" jsonschema:"Full path of the project to confirm permanent removal (required when permanently_remove is true)"`
}

// DeleteOutput holds the result of a project deletion request.
type DeleteOutput struct {
	toolutil.HintableOutput
	Status              string `json:"status"`
	Message             string `json:"message"`
	MarkedForDeletionOn string `json:"marked_for_deletion_on,omitempty"`
	PermanentlyRemoved  bool   `json:"permanently_removed"`
}

// RestoreInput defines parameters for restoring a project marked for deletion.
type RestoreInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path of the project marked for deletion,required"`
}

// UpdateInput defines parameters for updating project settings.
type UpdateInput struct {
	ProjectID                                 toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	Name                                      string               `json:"name,omitempty"          jsonschema:"New project name"`
	Description                               string               `json:"description,omitempty"   jsonschema:"New description"`
	Visibility                                string               `json:"visibility,omitempty"    jsonschema:"Visibility level (private, internal, public)"`
	DefaultBranch                             string               `json:"default_branch,omitempty" jsonschema:"New default branch"`
	MergeMethod                               string               `json:"merge_method,omitempty"  jsonschema:"Merge method (merge, rebase_merge, ff)"`
	Topics                                    []string             `json:"topics,omitempty"        jsonschema:"Topic tags for the project"`
	SquashOption                              string               `json:"squash_option,omitempty" jsonschema:"Squash option (never, always, default_on, default_off)"`
	OnlyAllowMergeIfPipelineSucceeds          *bool                `json:"only_allow_merge_if_pipeline_succeeds,omitempty" jsonschema:"Only allow merge when pipeline succeeds"`
	OnlyAllowMergeIfAllDiscussionsAreResolved *bool                `json:"only_allow_merge_if_all_discussions_are_resolved,omitempty" jsonschema:"Only allow merge when all discussions are resolved"`
	IssuesEnabled                             *bool                `json:"issues_enabled,omitempty"           jsonschema:"Enable/disable issues feature (use 'issues_enabled' not 'issues_access_level')"`
	MergeRequestsEnabled                      *bool                `json:"merge_requests_enabled,omitempty"   jsonschema:"Enable merge requests feature"`
	WikiEnabled                               *bool                `json:"wiki_enabled,omitempty"             jsonschema:"Enable wiki feature"`
	JobsEnabled                               *bool                `json:"jobs_enabled,omitempty"             jsonschema:"Enable CI/CD jobs"`
	LFSEnabled                                *bool                `json:"lfs_enabled,omitempty"              jsonschema:"Enable Git LFS"`
	RequestAccessEnabled                      *bool                `json:"request_access_enabled,omitempty"   jsonschema:"Allow users to request access"`
	SharedRunnersEnabled                      *bool                `json:"shared_runners_enabled,omitempty"   jsonschema:"Enable shared runners"`
	PublicBuilds                              *bool                `json:"public_builds,omitempty"            jsonschema:"Enable public access to pipelines"`
	PackagesEnabled                           *bool                `json:"packages_enabled,omitempty"         jsonschema:"Enable packages feature (deprecated: use package_registry_access_level)"`
	PackageRegistryAccessLevel                string               `json:"package_registry_access_level,omitempty" jsonschema:"Package registry access level (disabled, private, enabled)"`
	PagesAccessLevel                          string               `json:"pages_access_level,omitempty"       jsonschema:"Pages access level (disabled, private, enabled, public)"`
	ContainerRegistryAccessLevel              string               `json:"container_registry_access_level,omitempty" jsonschema:"Container registry access level (disabled, private, enabled)"`
	SnippetsAccessLevel                       string               `json:"snippets_access_level,omitempty"    jsonschema:"Snippets access level (disabled, private, enabled)"`
	CIConfigPath                              string               `json:"ci_config_path,omitempty"           jsonschema:"Custom CI/CD configuration file path"`
	AllowMergeOnSkippedPipeline               *bool                `json:"allow_merge_on_skipped_pipeline,omitempty" jsonschema:"Allow merge when pipeline is skipped"`
	RemoveSourceBranchAfterMerge              *bool                `json:"remove_source_branch_after_merge,omitempty" jsonschema:"Remove source branch after merge by default"`
	AutocloseReferencedIssues                 *bool                `json:"autoclose_referenced_issues,omitempty" jsonschema:"Auto-close referenced issues on merge"`
	MergeCommitTemplate                       string               `json:"merge_commit_template,omitempty"    jsonschema:"Template for merge commit messages"`
	SquashCommitTemplate                      string               `json:"squash_commit_template,omitempty"   jsonschema:"Template for squash commit messages"`
	MRDefaultTitleTemplate                    string               `json:"mr_default_title_template,omitempty" jsonschema:"Template for default merge request titles"`
	MergePipelinesEnabled                     *bool                `json:"merge_pipelines_enabled,omitempty"  jsonschema:"Enable merged results pipelines"`
	MergeTrainsEnabled                        *bool                `json:"merge_trains_enabled,omitempty"     jsonschema:"Enable merge trains"`
	ProtectMergeRequestPipelines              *bool                `json:"protect_merge_request_pipelines,omitempty" jsonschema:"Prevent merge request pipeline settings from being modified by users with lower permissions"`
	ResolveOutdatedDiffDiscussions            *bool                `json:"resolve_outdated_diff_discussions,omitempty" jsonschema:"Auto-resolve outdated diff discussions"`
	ApprovalsBeforeMerge                      int64                `json:"approvals_before_merge,omitempty"   tier:"premium" jsonschema:"Number of approvals required before merge"`
	MergeRequestTitleRegex                    string               `json:"merge_request_title_regex,omitempty" jsonschema:"Regex that MR titles must match"`
	MergeRequestTitleRegexDescription         string               `json:"merge_request_title_regex_description,omitempty" jsonschema:"Human-readable description for the MR title regex"`
	ReviewerAssignmentStrategy                string               `json:"reviewer_assignment_strategy,omitempty" tier:"premium" jsonschema:"Reviewer assignment strategy for merge requests (disabled, code_owners, dap_powered)"`

	// Basic metadata (additive 1:1 SDK parity)
	Path              string   `json:"path,omitempty" jsonschema:"New project path slug"`
	ImportURL         string   `json:"import_url,omitempty" jsonschema:"URL to import/mirror repository from"`
	RepositoryStorage string   `json:"repository_storage,omitempty" jsonschema:"Repository storage shard name (admin only)"`
	TagList           []string `json:"tag_list,omitempty" jsonschema:"Project tags (deprecated: use topics)"`

	// Merge settings (additive)
	OnlyAllowMergeIfAllStatusChecksPassed *bool  `json:"only_allow_merge_if_all_status_checks_passed,omitempty" tier:"ultimate" jsonschema:"Only allow merge when all external status checks pass"`
	AllowPipelineTriggerApproveDeployment *bool  `json:"allow_pipeline_trigger_approve_deployment,omitempty" jsonschema:"Allow pipeline triggerer to approve deployment"`
	PreventMergeWithoutJiraIssue          *bool  `json:"prevent_merge_without_jira_issue,omitempty" jsonschema:"Require an associated Jira issue before merge"`
	MergeRequestDefaultTargetSelf         *bool  `json:"mr_default_target_self,omitempty" jsonschema:"Default merge request target to the project itself (for forks)"`
	MergeTrainsSkipTrainAllowed           *bool  `json:"merge_trains_skip_train_allowed,omitempty" jsonschema:"Allow skipping the merge train"`
	PrintingMergeRequestLinkEnabled       *bool  `json:"printing_merge_request_link_enabled,omitempty" jsonschema:"Show the create-merge-request link after pushing"`
	SuggestionCommitMessage               string `json:"suggestion_commit_message,omitempty" jsonschema:"Default commit message for suggestions"`
	IssueBranchTemplate                   string `json:"issue_branch_template,omitempty" jsonschema:"Template for branch names created from issues"`
	IssuesTemplate                        string `json:"issues_template,omitempty" jsonschema:"Default issue description template"`
	MergeRequestsTemplate                 string `json:"merge_requests_template,omitempty" jsonschema:"Default merge request description template"`

	// Feature toggles (additive)
	SnippetsEnabled          *bool `json:"snippets_enabled,omitempty" jsonschema:"Enable snippets feature (deprecated: use snippets_access_level)"`
	ContainerRegistryEnabled *bool `json:"container_registry_enabled,omitempty" jsonschema:"Enable container registry (deprecated: use container_registry_access_level)"`
	ServiceDeskEnabled       *bool `json:"service_desk_enabled,omitempty" jsonschema:"Enable Service Desk"`
	EmailsEnabled            *bool `json:"emails_enabled,omitempty" jsonschema:"Enable email notifications"`
	EmailsDisabled           *bool `json:"emails_disabled,omitempty" jsonschema:"Disable email notifications (deprecated: use emails_enabled)"`
	ShowDefaultAwardEmojis   *bool `json:"show_default_award_emojis,omitempty" jsonschema:"Show default award emojis on issues/MRs"`
	GroupRunnersEnabled      *bool `json:"group_runners_enabled,omitempty" jsonschema:"Enable group runners"`
	KeepLatestArtifact       *bool `json:"keep_latest_artifact,omitempty" jsonschema:"Keep the latest artifact for each pipeline"`
	BuildTimeout             int64 `json:"build_timeout,omitempty" jsonschema:"Build timeout in seconds"`
	MaxArtifactsSize         int64 `json:"max_artifacts_size,omitempty" jsonschema:"Maximum artifacts size in MB"`

	// Auto DevOps (additive)
	AutoDevopsEnabled        *bool  `json:"auto_devops_enabled,omitempty" jsonschema:"Enable Auto DevOps for the project"`
	AutoDevopsDeployStrategy string `json:"auto_devops_deploy_strategy,omitempty" jsonschema:"Auto DevOps deploy strategy (continuous, manual, timed_incremental)"`
	AutoDuoCodeReviewEnabled *bool  `json:"auto_duo_code_review_enabled,omitempty" jsonschema:"Enable automatic GitLab Duo code review"`

	// CI/CD settings (additive)
	BuildGitStrategy                       string   `json:"build_git_strategy,omitempty" jsonschema:"Git strategy for builds (fetch, clone)"`
	BuildCoverageRegex                     string   `json:"build_coverage_regex,omitempty" jsonschema:"Regex used to extract test coverage from job logs"`
	AutoCancelPendingPipelines             string   `json:"auto_cancel_pending_pipelines,omitempty" jsonschema:"Auto-cancel pending pipelines (enabled, disabled)"`
	CICDCatalogEnabled                     *bool    `json:"cicd_catalog_enabled,omitempty" jsonschema:"Publish the project as a CI/CD catalog resource"`
	CIDefaultGitDepth                      int64    `json:"ci_default_git_depth,omitempty" jsonschema:"Default Git clone depth for CI/CD"`
	CIDeletePipelinesInSeconds             int64    `json:"ci_delete_pipelines_in_seconds,omitempty" jsonschema:"Auto-delete pipelines older than this many seconds"`
	CIDisplayPipelineVariables             *bool    `json:"ci_display_pipeline_variables,omitempty" jsonschema:"Display pipeline variables in the UI"`
	CIForwardDeploymentEnabled             *bool    `json:"ci_forward_deployment_enabled,omitempty" jsonschema:"Enable CI/CD forward deployment (skip outdated jobs)"`
	CIForwardDeploymentRollbackAllowed     *bool    `json:"ci_forward_deployment_rollback_allowed,omitempty" jsonschema:"Allow job retries even for outdated deployment jobs"`
	CIPushRepositoryForJobTokenAllowed     *bool    `json:"ci_push_repository_for_job_token_allowed,omitempty" jsonschema:"Allow CI job tokens to push to the repository"`
	CIIDTokenSubClaimComponents            []string `json:"ci_id_token_sub_claim_components,omitempty" jsonschema:"Components used to build the CI ID token sub claim"`
	CISeparatedCaches                      *bool    `json:"ci_separated_caches,omitempty" jsonschema:"Use separate caches for protected branches"`
	CIRestrictPipelineCancellationRole     string   `json:"ci_restrict_pipeline_cancellation_role,omitempty" jsonschema:"Role required to cancel pipelines/jobs (developer, maintainer, no_one)"`
	CIPipelineVariablesMinimumOverrideRole string   `json:"ci_pipeline_variables_minimum_override_role,omitempty" jsonschema:"Minimum role allowed to override pipeline variables (developer, maintainer, owner, no_one_allowed)"`
	RestrictUserDefinedVariables           *bool    `json:"restrict_user_defined_variables,omitempty" jsonschema:"Restrict user-defined pipeline variables (deprecated: use ci_pipeline_variables_minimum_override_role)"`
	ResourceGroupDefaultProcessMode        string   `json:"resource_group_default_process_mode,omitempty" jsonschema:"Default resource group process mode (unordered, oldest_first, newest_first)"`
	PublicJobs                             *bool    `json:"public_jobs,omitempty" jsonschema:"Enable public access to pipelines"`

	// Mirroring (additive)
	Mirror                           *bool  `json:"mirror,omitempty" jsonschema:"Enable pull mirroring from import_url"`
	MirrorTriggerBuilds              *bool  `json:"mirror_trigger_builds,omitempty" jsonschema:"Trigger pipelines for mirror updates"`
	MirrorBranchRegex                string `json:"mirror_branch_regex,omitempty" jsonschema:"Only mirror branches matching this regex"`
	MirrorOverwritesDivergedBranches *bool  `json:"mirror_overwrites_diverged_branches,omitempty" jsonschema:"Overwrite diverged branches when mirroring"`
	MirrorUserID                     int64  `json:"mirror_user_id,omitempty" jsonschema:"User ID used to run mirror updates (admin only)"`
	OnlyMirrorProtectedBranches      *bool  `json:"only_mirror_protected_branches,omitempty" jsonschema:"Only mirror protected branches"`

	// Container registry cleanup (additive)
	ContainerExpirationPolicyAttributes *ContainerExpirationPolicyInput `json:"container_expiration_policy_attributes,omitempty" jsonschema:"Container registry cleanup policy attributes"`

	// Compliance and security (additive)
	EnforceAuthChecksOnUploads               *bool  `json:"enforce_auth_checks_on_uploads,omitempty" jsonschema:"Enforce authorization checks on uploads"`
	ExternalAuthorizationClassificationLabel string `json:"external_authorization_classification_label,omitempty" jsonschema:"External authorization classification label"`

	// Access control (additive)
	IssuesAccessLevel                string `json:"issues_access_level,omitempty" jsonschema:"Issues access level (disabled, private, enabled)"`
	MergeRequestsAccessLevel         string `json:"merge_requests_access_level,omitempty" jsonschema:"Merge requests access level (disabled, private, enabled)"`
	WikiAccessLevel                  string `json:"wiki_access_level,omitempty" jsonschema:"Wiki access level (disabled, private, enabled)"`
	BuildsAccessLevel                string `json:"builds_access_level,omitempty" jsonschema:"CI/CD builds access level (disabled, private, enabled)"`
	RepositoryAccessLevel            string `json:"repository_access_level,omitempty" jsonschema:"Repository access level (disabled, private, enabled)"`
	ForkingAccessLevel               string `json:"forking_access_level,omitempty" jsonschema:"Forking access level (disabled, private, enabled)"`
	AnalyticsAccessLevel             string `json:"analytics_access_level,omitempty" jsonschema:"Analytics access level (disabled, private, enabled)"`
	OperationsAccessLevel            string `json:"operations_access_level,omitempty" jsonschema:"Operations access level (disabled, private, enabled)"`
	ReleasesAccessLevel              string `json:"releases_access_level,omitempty" jsonschema:"Releases access level (disabled, private, enabled)"`
	EnvironmentsAccessLevel          string `json:"environments_access_level,omitempty" jsonschema:"Environments access level (disabled, private, enabled)"`
	FeatureFlagsAccessLevel          string `json:"feature_flags_access_level,omitempty" jsonschema:"Feature flags access level (disabled, private, enabled)"`
	InfrastructureAccessLevel        string `json:"infrastructure_access_level,omitempty" jsonschema:"Infrastructure access level (disabled, private, enabled)"`
	MonitorAccessLevel               string `json:"monitor_access_level,omitempty" jsonschema:"Monitor access level (disabled, private, enabled)"`
	RequirementsAccessLevel          string `json:"requirements_access_level,omitempty" jsonschema:"Requirements access level (disabled, private, enabled)"`
	SecurityAndComplianceAccessLevel string `json:"security_and_compliance_access_level,omitempty" jsonschema:"Security and compliance access level (disabled, private, enabled)"`
	ModelExperimentsAccessLevel      string `json:"model_experiments_access_level,omitempty" jsonschema:"Model experiments access level (disabled, private, enabled)"`
	ModelRegistryAccessLevel         string `json:"model_registry_access_level,omitempty" jsonschema:"Model registry access level (disabled, private, enabled)"`
}

// ToOutput converts a GitLab API [gl.Project] to the MCP tool output format,
// mapping every SDK field 1:1 including full nested objects on their canonical
// keys (namespace, owner, permissions, forked_from_project, _links, statistics,
// shared_with_groups, license, container_expiration_policy, custom_attributes).
func ToOutput(p *gl.Project) Output {
	out := Output{
		ID:                               p.ID,
		Name:                             p.Name,
		Path:                             p.Path,
		PathWithNamespace:                p.PathWithNamespace,
		NameWithNamespace:                p.NameWithNamespace,
		Visibility:                       string(p.Visibility),
		DefaultBranch:                    p.DefaultBranch,
		WebURL:                           p.WebURL,
		Description:                      p.Description,
		Archived:                         p.Archived,
		EmptyRepo:                        p.EmptyRepo,
		ForksCount:                       p.ForksCount,
		StarCount:                        p.StarCount,
		OpenIssuesCount:                  p.OpenIssuesCount,
		HTTPURLToRepo:                    p.HTTPURLToRepo,
		SSHURLToRepo:                     p.SSHURLToRepo,
		Topics:                           p.Topics,
		MergeMethod:                      string(p.MergeMethod),
		SquashOption:                     string(p.SquashOption),
		OnlyAllowMergeIfPipelineSucceeds: p.OnlyAllowMergeIfPipelineSucceeds,
		OnlyAllowMergeIfAllDiscussionsAreResolved: p.OnlyAllowMergeIfAllDiscussionsAreResolved,
		RemoveSourceBranchAfterMerge:              p.RemoveSourceBranchAfterMerge,
		ReadmeURL:                                 p.ReadmeURL,
		AvatarURL:                                 p.AvatarURL,
		CreatorID:                                 p.CreatorID,
		RequestAccessEnabled:                      p.RequestAccessEnabled,
		LFSEnabled:                                p.LFSEnabled,
		CIConfigPath:                              p.CIConfigPath,
		AllowMergeOnSkippedPipeline:               p.AllowMergeOnSkippedPipeline,
		MergePipelinesEnabled:                     p.MergePipelinesEnabled,
		MergeTrainsEnabled:                        p.MergeTrainsEnabled,
		ProtectMergeRequestPipelines:              &p.ProtectMergeRequestPipelines,
		MergeCommitTemplate:                       p.MergeCommitTemplate,
		SquashCommitTemplate:                      p.SquashCommitTemplate,
		MRDefaultTitleTemplate:                    p.MRDefaultTitleTemplate,
		AutocloseReferencedIssues:                 p.AutocloseReferencedIssues,
		ResolveOutdatedDiffDiscussions:            p.ResolveOutdatedDiffDiscussions,
		SharedRunnersEnabled:                      p.SharedRunnersEnabled,
		PackageRegistryAccessLevel:                string(p.PackageRegistryAccessLevel),
		BuildTimeout:                              p.BuildTimeout,
		SuggestionCommitMessage:                   p.SuggestionCommitMessage,
		ComplianceFrameworks:                      p.ComplianceFrameworks,
		ImportURL:                                 p.ImportURL,
		MergeRequestTitleRegex:                    p.MergeRequestTitleRegex,
		MergeRequestTitleRegexDescription:         p.MergeRequestTitleRegexDescription,
		ReviewerAssignmentStrategy:                string(p.ReviewerAssignmentStrategy),

		ContainerRegistryAccessLevel:     string(p.ContainerRegistryAccessLevel),
		IssuesAccessLevel:                string(p.IssuesAccessLevel),
		ReleasesAccessLevel:              string(p.ReleasesAccessLevel),
		RepositoryAccessLevel:            string(p.RepositoryAccessLevel),
		MergeRequestsAccessLevel:         string(p.MergeRequestsAccessLevel),
		ForkingAccessLevel:               string(p.ForkingAccessLevel),
		WikiAccessLevel:                  string(p.WikiAccessLevel),
		BuildsAccessLevel:                string(p.BuildsAccessLevel),
		SnippetsAccessLevel:              string(p.SnippetsAccessLevel),
		PagesAccessLevel:                 string(p.PagesAccessLevel),
		OperationsAccessLevel:            string(p.OperationsAccessLevel),
		AnalyticsAccessLevel:             string(p.AnalyticsAccessLevel),
		EnvironmentsAccessLevel:          string(p.EnvironmentsAccessLevel),
		FeatureFlagsAccessLevel:          string(p.FeatureFlagsAccessLevel),
		InfrastructureAccessLevel:        string(p.InfrastructureAccessLevel),
		MonitorAccessLevel:               string(p.MonitorAccessLevel),
		RequirementsAccessLevel:          string(p.RequirementsAccessLevel),
		SecurityAndComplianceAccessLevel: string(p.SecurityAndComplianceAccessLevel),
		ModelExperimentsAccessLevel:      string(p.ModelExperimentsAccessLevel),
		ModelRegistryAccessLevel:         string(p.ModelRegistryAccessLevel),

		CIDisplayPipelineVariables:               p.CIDisplayPipelineVariables,
		AllowPipelineTriggerApproveDeployment:    p.AllowPipelineTriggerApproveDeployment,
		PreventMergeWithoutJiraIssue:             p.PreventMergeWithoutJiraIssue,
		PrintingMergeRequestLinkEnabled:          p.PrintingMergeRequestLinkEnabled,
		CICDCatalogEnabled:                       p.CICDCatalogEnabled,
		CIDefaultGitDepth:                        p.CIDefaultGitDepth,
		CIDeletePipelinesInSeconds:               p.CIDeletePipelinesInSeconds,
		CIForwardDeploymentEnabled:               p.CIForwardDeploymentEnabled,
		CIForwardDeploymentRollbackAllowed:       p.CIForwardDeploymentRollbackAllowed,
		CIPushRepositoryForJobTokenAllowed:       p.CIPushRepositoryForJobTokenAllowed,
		CIIDTokenSubClaimComponents:              p.CIIdTokenSubClaimComponents,
		CISeparatedCaches:                        p.CISeparatedCaches,
		CIJobTokenScopeEnabled:                   p.CIJobTokenScopeEnabled,
		CIOptInJWT:                               p.CIOptInJWT,
		CIAllowForkPipelinesToRunInParentProject: p.CIAllowForkPipelinesToRunInParentProject,
		CIRestrictPipelineCancellationRole:       string(p.CIRestrictPipelineCancellationRole),
		CIPipelineVariablesMinimumOverrideRole:   p.CIPipelineVariablesMinimumOverrideRole,
		BuildCoverageRegex:                       p.BuildCoverageRegex,
		BuildGitStrategy:                         p.BuildGitStrategy,
		AutoCancelPendingPipelines:               p.AutoCancelPendingPipelines,
		AutoDevopsDeployStrategy:                 p.AutoDevopsDeployStrategy,
		AutoDevopsEnabled:                        p.AutoDevopsEnabled,
		KeepLatestArtifact:                       p.KeepLatestArtifact,
		MergeTrainsSkipTrainAllowed:              p.MergeTrainsSkipTrainAllowed,
		PublicJobs:                               p.PublicJobs,
		MaxArtifactsSize:                         p.MaxArtifactsSize,

		Mirror:                           p.Mirror,
		MirrorUserID:                     p.MirrorUserID,
		MirrorTriggerBuilds:              p.MirrorTriggerBuilds,
		OnlyMirrorProtectedBranches:      p.OnlyMirrorProtectedBranches,
		MirrorOverwritesDivergedBranches: p.MirrorOverwritesDivergedBranches,

		ImportType:   p.ImportType,
		ImportStatus: p.ImportStatus,
		ImportError:  p.ImportError,

		ContainerRegistryImagePrefix: p.ContainerRegistryImagePrefix,

		SecurityAndComplianceEnabled:     p.SecurityAndComplianceEnabled,
		EnforceAuthChecksOnUploads:       p.EnforceAuthChecksOnUploads,
		PreReceiveSecretDetectionEnabled: p.PreReceiveSecretDetectionEnabled,
		AutoDuoCodeReviewEnabled:         p.AutoDuoCodeReviewEnabled,

		ServiceDeskEnabled: p.ServiceDeskEnabled,
		ServiceDeskAddress: p.ServiceDeskAddress,

		IssuesTemplate:                           p.IssuesTemplate,
		MergeRequestsTemplate:                    p.MergeRequestsTemplate,
		IssueBranchTemplate:                      p.IssueBranchTemplate,
		ExternalAuthorizationClassificationLabel: p.ExternalAuthorizationClassificationLabel,
		RequirementsEnabled:                      p.RequirementsEnabled,
		EmailsEnabled:                            p.EmailsEnabled,
		GroupRunnersEnabled:                      p.GroupRunnersEnabled,
		ResourceGroupDefaultProcessMode:          string(p.ResourceGroupDefaultProcessMode),
		RunnerTokenExpirationInterval:            p.RunnerTokenExpirationInterval,
		RunnersToken:                             p.RunnersToken,
		RepositoryStorage:                        p.RepositoryStorage,
		CanCreateMergeRequestIn:                  p.CanCreateMergeRequestIn,
		LicenseURL:                               p.LicenseURL,
		MergeRequestDefaultTargetSelf:            p.MergeRequestDefaultTargetSelf,

		Namespace:                 namespaceOutput(p.Namespace),
		Owner:                     ownerOutput(p.Owner),
		Permissions:               permissionsOutput(p.Permissions),
		ForkedFromProject:         forkParentOutput(p.ForkedFromProject),
		Links:                     linksOutput(p.Links),
		Statistics:                statisticsOutput(p.Statistics),
		SharedWithGroups:          sharedWithGroupsOutput(p.SharedWithGroups),
		License:                   licenseOutput(p.License),
		ContainerExpirationPolicy: containerExpirationPolicyOutput(p.ContainerExpirationPolicy),
		CustomAttributes:          customAttributesOutput(p.CustomAttributes),

		IssuesEnabled:                accessLevelEnabled(p.IssuesAccessLevel),
		MergeRequestsEnabled:         accessLevelEnabled(p.MergeRequestsAccessLevel),
		WikiEnabled:                  accessLevelEnabled(p.WikiAccessLevel),
		JobsEnabled:                  accessLevelEnabled(p.BuildsAccessLevel),
		ContainerRegistryEnabled:     accessLevelEnabled(p.ContainerRegistryAccessLevel),
		PublicBuilds:                 p.PublicJobs,
		SnippetsEnabled:              accessLevelEnabled(p.SnippetsAccessLevel),
		PackagesEnabled:              p.PackagesEnabled,              //nolint:staticcheck // Preserve backward-compatible field in output.
		ApprovalsBeforeMerge:         p.ApprovalsBeforeMerge,         //nolint:staticcheck // No replacement field on Project struct.
		TagList:                      p.TagList,                      //nolint:staticcheck // 1:1 SDK parity; deprecated, surfaced additively.
		RestrictUserDefinedVariables: p.RestrictUserDefinedVariables, //nolint:staticcheck // 1:1 SDK parity; deprecated, surfaced additively.
		EmailsDisabled:               p.EmailsDisabled,               //nolint:staticcheck // 1:1 SDK parity; deprecated, surfaced additively.
	}
	if out.Topics == nil {
		out.Topics = []string{}
	}
	if p.MarkedForDeletionOn != nil {
		out.MarkedForDeletionOn = time.Time(*p.MarkedForDeletionOn).Format(time.DateOnly)
	}
	if p.MarkedForDeletionAt != nil { //nolint:staticcheck // 1:1 SDK parity; deprecated, surfaced additively.
		out.MarkedForDeletionAt = time.Time(*p.MarkedForDeletionAt).Format(time.DateOnly) //nolint:staticcheck // 1:1 SDK parity; deprecated field surfaced additively.
	}
	if p.CreatedAt != nil {
		out.CreatedAt = p.CreatedAt.Format(time.RFC3339)
	}
	if p.UpdatedAt != nil {
		out.UpdatedAt = p.UpdatedAt.Format(time.RFC3339)
	}
	if p.LastActivityAt != nil {
		out.LastActivityAt = p.LastActivityAt.Format(time.RFC3339)
	}
	return out
}

// accessLevelPtr converts a non-empty string into an [gl.AccessControlValue]
// pointer, returning nil for the empty string so omitted inputs stay unset.
func accessLevelPtr(s string) *gl.AccessControlValue {
	if s == "" {
		return nil
	}
	return new(gl.AccessControlValue(s))
}

// buildCreateOpts maps CreateInput fields to the GitLab API create options.
func buildCreateOpts(input CreateInput) *gl.CreateProjectOptions {
	opts := &gl.CreateProjectOptions{Name: new(input.Name)}
	if input.NamespaceID != 0 {
		opts.NamespaceID = new(int64(input.NamespaceID))
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.InitializeWithReadme {
		opts.InitializeWithReadme = new(true)
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if len(input.Topics) > 0 {
		opts.Topics = &input.Topics
	}
	if input.MergeMethod != "" {
		opts.MergeMethod = new(gl.MergeMethodValue(input.MergeMethod))
	}
	if input.SquashOption != "" {
		opts.SquashOption = new(gl.SquashOptionValue(input.SquashOption))
	}
	if input.OnlyAllowMergeIfPipelineSucceeds {
		opts.OnlyAllowMergeIfPipelineSucceeds = new(true)
	}
	if input.OnlyAllowMergeIfAllDiscussionsAreResolved {
		opts.OnlyAllowMergeIfAllDiscussionsAreResolved = new(true)
	}
	applyCreateFeatureOpts(opts, input)
	return opts
}

// applyCreateFeatureOpts sets optional feature toggles on the create options.
func applyCreateFeatureOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	applyCreateFeatureToggles(opts, input)
	applyCreateBuildOpts(opts, input)
	applyCreateAccessLevels(opts, input)
	applyCreateTemplateOpts(opts, input)
	applyCreateMergeOpts(opts, input)
	applyCreatePipelineOpts(opts, input)
	applyCreateMirrorOpts(opts, input)
	applyCreateComplianceOpts(opts, input)
}

// applyCreateFeatureToggles applies create feature toggles transformations.
// The issues/merge-requests/wiki/jobs booleans are bridged to their modern
// access-level fields in applyCreateAccessLevels, so they are not re-applied to
// the deprecated boolean options here.
func applyCreateFeatureToggles(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.SnippetsEnabled != nil {
		opts.SnippetsEnabled = input.SnippetsEnabled //nolint:staticcheck // 1:1 SDK parity; prefer snippets_access_level.
	}
	if input.ContainerRegistryEnabled != nil {
		opts.ContainerRegistryEnabled = input.ContainerRegistryEnabled //nolint:staticcheck // 1:1 SDK parity; prefer container_registry_access_level.
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
	if input.ServiceDeskEnabled != nil {
		opts.ServiceDeskEnabled = input.ServiceDeskEnabled //nolint:staticcheck // 1:1 SDK parity; no longer supported in recent versions.
	}
	if input.EmailsEnabled != nil {
		opts.EmailsEnabled = input.EmailsEnabled
	}
	if input.EmailsDisabled != nil {
		opts.EmailsDisabled = input.EmailsDisabled //nolint:staticcheck // 1:1 SDK parity; prefer emails_enabled.
	}
	if input.ShowDefaultAwardEmojis != nil {
		opts.ShowDefaultAwardEmojis = input.ShowDefaultAwardEmojis
	}
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
}

// applyCreateBuildOpts applies create build and CI/CD transformations.
func applyCreateBuildOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.ImportURL != "" {
		opts.ImportURL = new(input.ImportURL)
	}
	if input.RepositoryStorage != "" {
		opts.RepositoryStorage = new(input.RepositoryStorage)
	}
	if input.CIConfigPath != "" {
		opts.CIConfigPath = new(input.CIConfigPath)
	}
	if input.CICDCatalogEnabled != nil {
		opts.CICDCatalogEnabled = input.CICDCatalogEnabled
	}
	if input.BuildTimeout > 0 {
		opts.BuildTimeout = new(input.BuildTimeout)
	}
	if input.BuildGitStrategy != "" {
		opts.BuildGitStrategy = new(input.BuildGitStrategy)
	}
	if input.BuildCoverageRegex != "" {
		opts.BuildCoverageRegex = new(input.BuildCoverageRegex)
	}
	if input.PackagesEnabled != nil {
		opts.PackagesEnabled = input.PackagesEnabled //nolint:staticcheck // Preserve backward-compatible input field.
	}
	if input.PackageRegistryAccessLevel != "" {
		opts.PackageRegistryAccessLevel = accessLevelPtr(input.PackageRegistryAccessLevel)
	}
}

// applyCreatePipelineOpts applies CI/CD pipeline and runner transformations.
func applyCreatePipelineOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.AutoDevopsEnabled != nil {
		opts.AutoDevopsEnabled = input.AutoDevopsEnabled
	}
	if input.AutoDevopsDeployStrategy != "" {
		opts.AutoDevopsDeployStrategy = new(input.AutoDevopsDeployStrategy)
	}
	if input.AutoCancelPendingPipelines != "" {
		opts.AutoCancelPendingPipelines = new(input.AutoCancelPendingPipelines)
	}
	if input.CIForwardDeploymentEnabled != nil {
		opts.CIForwardDeploymentEnabled = input.CIForwardDeploymentEnabled //nolint:staticcheck // 1:1 SDK parity; no longer supported in recent versions.
	}
	if input.ResourceGroupDefaultProcessMode != "" {
		opts.ResourceGroupDefaultProcessMode = new(gl.ResourceGroupProcessMode(input.ResourceGroupDefaultProcessMode))
	}
	if input.SharedRunnersEnabled != nil {
		opts.SharedRunnersEnabled = input.SharedRunnersEnabled
	}
	if input.GroupRunnersEnabled != nil {
		opts.GroupRunnersEnabled = input.GroupRunnersEnabled
	}
	if input.PublicBuilds != nil {
		opts.PublicJobs = input.PublicBuilds
	}
	if input.PublicJobs != nil {
		opts.PublicJobs = input.PublicJobs
	}
}

// applyCreateTemplateOpts applies template provisioning transformations.
func applyCreateTemplateOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.TemplateName != "" {
		opts.TemplateName = new(input.TemplateName)
	}
	if input.TemplateProjectID != 0 {
		opts.TemplateProjectID = new(input.TemplateProjectID)
	}
	if input.UseCustomTemplate != nil {
		opts.UseCustomTemplate = input.UseCustomTemplate
	}
	if input.GroupWithProjectTemplatesID != 0 {
		opts.GroupWithProjectTemplatesID = new(input.GroupWithProjectTemplatesID)
	}
	if input.IssuesTemplate != "" {
		opts.IssuesTemplate = new(input.IssuesTemplate) //nolint:staticcheck // 1:1 SDK parity; no longer supported in recent versions.
	}
	if input.MergeRequestsTemplate != "" {
		opts.MergeRequestsTemplate = new(input.MergeRequestsTemplate) //nolint:staticcheck // 1:1 SDK parity; no longer supported in recent versions.
	}
	if len(input.TagList) > 0 {
		opts.TagList = &input.TagList //nolint:staticcheck // 1:1 SDK parity; prefer topics.
	}
}

// applyCreateMergeOpts applies merge-related transformations.
func applyCreateMergeOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.OnlyAllowMergeIfAllStatusChecksPassed != nil {
		opts.OnlyAllowMergeIfAllStatusChecksPassed = input.OnlyAllowMergeIfAllStatusChecksPassed
	}
	if input.AllowMergeOnSkippedPipeline != nil {
		opts.AllowMergeOnSkippedPipeline = input.AllowMergeOnSkippedPipeline
	}
	if input.ProtectMergeRequestPipelines != nil {
		opts.ProtectMergeRequestPipelines = input.ProtectMergeRequestPipelines
	}
	if input.RemoveSourceBranchAfterMerge != nil {
		opts.RemoveSourceBranchAfterMerge = input.RemoveSourceBranchAfterMerge
	}
	if input.AutocloseReferencedIssues != nil {
		opts.AutocloseReferencedIssues = input.AutocloseReferencedIssues
	}
	if input.ResolveOutdatedDiffDiscussions != nil {
		opts.ResolveOutdatedDiffDiscussions = input.ResolveOutdatedDiffDiscussions
	}
	if input.PrintingMergeRequestLinkEnabled != nil {
		opts.PrintingMergeRequestLinkEnabled = input.PrintingMergeRequestLinkEnabled
	}
	if input.MergePipelinesEnabled != nil {
		opts.MergePipelinesEnabled = input.MergePipelinesEnabled
	}
	if input.MergeTrainsEnabled != nil {
		opts.MergeTrainsEnabled = input.MergeTrainsEnabled
	}
	if input.MergeTrainsSkipTrainAllowed != nil {
		opts.MergeTrainsSkipTrainAllowed = input.MergeTrainsSkipTrainAllowed
	}
	applyCreateMergeTemplateOpts(opts, input)
}

// applyCreateMergeTemplateOpts applies merge commit template and regex fields.
func applyCreateMergeTemplateOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.MergeCommitTemplate != "" {
		opts.MergeCommitTemplate = new(input.MergeCommitTemplate)
	}
	if input.SquashCommitTemplate != "" {
		opts.SquashCommitTemplate = new(input.SquashCommitTemplate)
	}
	if input.MRDefaultTitleTemplate != "" {
		opts.MRDefaultTitleTemplate = new(input.MRDefaultTitleTemplate)
	}
	if input.SuggestionCommitMessage != "" {
		opts.SuggestionCommitMessage = new(input.SuggestionCommitMessage)
	}
	if input.IssueBranchTemplate != "" {
		opts.IssueBranchTemplate = new(input.IssueBranchTemplate)
	}
	if input.ApprovalsBeforeMerge > 0 {
		opts.ApprovalsBeforeMerge = new(input.ApprovalsBeforeMerge) //nolint:staticcheck // 1:1 SDK parity; prefer Merge Request Approvals API.
	}
	if input.MergeRequestTitleRegex != "" {
		opts.MergeRequestTitleRegex = new(input.MergeRequestTitleRegex)
	}
	if input.MergeRequestTitleRegexDescription != "" {
		opts.MergeRequestTitleRegexDescription = new(input.MergeRequestTitleRegexDescription)
	}
	if input.ReviewerAssignmentStrategy != "" {
		opts.ReviewerAssignmentStrategy = new(gl.ReviewerAssignmentStrategyValue(input.ReviewerAssignmentStrategy))
	}
}

// applyCreateMirrorOpts applies pull mirroring transformations.
func applyCreateMirrorOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.Mirror != nil {
		opts.Mirror = input.Mirror
	}
	if input.MirrorTriggerBuilds != nil {
		opts.MirrorTriggerBuilds = input.MirrorTriggerBuilds
	}
}

// applyCreateComplianceOpts applies compliance, security, and container policy
// transformations.
func applyCreateComplianceOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.EnforceAuthChecksOnUploads != nil {
		opts.EnforceAuthChecksOnUploads = input.EnforceAuthChecksOnUploads
	}
	if input.ExternalAuthorizationClassificationLabel != "" {
		opts.ExternalAuthorizationClassificationLabel = new(input.ExternalAuthorizationClassificationLabel)
	}
	opts.ContainerExpirationPolicyAttributes = input.ContainerExpirationPolicyAttributes.toGL()
}

// applyCreateAccessLevels applies access-level transformations.
func applyCreateAccessLevels(opts *gl.CreateProjectOptions, input CreateInput) {
	opts.PagesAccessLevel = accessLevelPtr(input.PagesAccessLevel)
	opts.ContainerRegistryAccessLevel = accessLevelPtr(input.ContainerRegistryAccessLevel)
	opts.SnippetsAccessLevel = accessLevelPtr(input.SnippetsAccessLevel)
	opts.IssuesAccessLevel = createAccessLevel(input.IssuesAccessLevel, input.IssuesEnabled)
	opts.MergeRequestsAccessLevel = createAccessLevel(input.MergeRequestsAccessLevel, input.MergeRequestsEnabled)
	opts.WikiAccessLevel = createAccessLevel(input.WikiAccessLevel, input.WikiEnabled)
	opts.BuildsAccessLevel = createAccessLevel(input.BuildsAccessLevel, input.JobsEnabled)
	opts.RepositoryAccessLevel = accessLevelPtr(input.RepositoryAccessLevel)
	opts.ForkingAccessLevel = accessLevelPtr(input.ForkingAccessLevel)
	opts.AnalyticsAccessLevel = accessLevelPtr(input.AnalyticsAccessLevel)
	opts.OperationsAccessLevel = accessLevelPtr(input.OperationsAccessLevel)
	opts.ReleasesAccessLevel = accessLevelPtr(input.ReleasesAccessLevel)
	opts.EnvironmentsAccessLevel = accessLevelPtr(input.EnvironmentsAccessLevel)
	opts.FeatureFlagsAccessLevel = accessLevelPtr(input.FeatureFlagsAccessLevel)
	opts.InfrastructureAccessLevel = accessLevelPtr(input.InfrastructureAccessLevel)
	opts.MonitorAccessLevel = accessLevelPtr(input.MonitorAccessLevel)
	opts.RequirementsAccessLevel = accessLevelPtr(input.RequirementsAccessLevel)
	opts.SecurityAndComplianceAccessLevel = accessLevelPtr(input.SecurityAndComplianceAccessLevel)
	opts.ModelExperimentsAccessLevel = accessLevelPtr(input.ModelExperimentsAccessLevel)
	opts.ModelRegistryAccessLevel = accessLevelPtr(input.ModelRegistryAccessLevel)
}

// createAccessLevel resolves an access level from its string form, falling back
// to the deprecated boolean toggle when the explicit access level is unset.
func createAccessLevel(level string, toggle *bool) *gl.AccessControlValue {
	if level != "" {
		return new(gl.AccessControlValue(level))
	}
	return boolToAccessLevel(toggle)
}

// Create creates a new GitLab project with the specified settings.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	opts := buildCreateOpts(input)
	p, _, err := client.GL().Projects.CreateProject(opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("projectCreate", err, "check that the project name/path is unique in the target namespace and all required fields are valid")
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("projectCreate", err, "a project with this name already exists in the namespace — use gitlab_project_list to verify")
		default:
			return Output{}, toolutil.WrapErrWithMessage("projectCreate", err)
		}
	}
	return ToOutput(p), nil
}

// Get retrieves a single GitLab project by its ID or URL-encoded path.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.GetProjectOptions{}
	if input.Statistics != nil {
		opts.Statistics = input.Statistics
	}
	if input.License != nil {
		opts.License = input.License
	}
	if input.WithCustomAttributes != nil {
		opts.WithCustomAttributes = input.WithCustomAttributes
	}
	p, _, err := client.GL().Projects.GetProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("projectGet", err,
				"verify project_id (numeric ID or URL-encoded full path like 'group%2Fsubgroup%2Fproject'); use gitlab_project_list with a search term to discover the correct ID")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectGet", err)
	}
	return ToOutput(p), nil
}

// buildListOpts maps ListInput fields to the GitLab API list options. ListInput,
// ListForksInput, and the user-scoped filters all target the same
// ListProjectsOptions filter surface, so option building is centralized in
// buildUserProjectOpts and each input only contributes a userProjectFilter.
func buildListOpts(input ListInput) *gl.ListProjectsOptions {
	return buildUserProjectOpts(input.toFilter())
}

// toFilter copies a ListInput into the shared userProjectFilter.
//
//nolint:dupl // Field-by-field copy into the shared filter; structurally parallel to the ListForksInput/userProjectFilterInput copies by design (distinct top-level MCP input structs, no shared embeddable without flattening JSON schemas).
func (input ListInput) toFilter() userProjectFilter {
	return userProjectFilter{
		Search: input.Search, Visibility: input.Visibility, Archived: input.Archived,
		OrderBy: input.OrderBy, Sort: input.Sort, Simple: input.Simple, Owned: input.Owned,
		Topic: input.Topic, MinAccessLevel: input.MinAccessLevel, Starred: input.Starred,
		Membership: input.Membership, WithIssuesEnabled: input.WithIssuesEnabled,
		WithMergeRequestsEnabled: input.WithMergeRequestsEnabled, SearchNamespaces: input.SearchNamespaces,
		Statistics: input.Statistics, WithProgrammingLanguage: input.WithProgrammingLanguage,
		LastActivityAfter: input.LastActivityAfter, LastActivityBefore: input.LastActivityBefore,
		IDAfter: input.IDAfter, IDBefore: input.IDBefore, IncludePendingDelete: input.IncludePendingDelete,
		IncludeHidden: input.IncludeHidden, Active: input.Active, Imported: input.Imported,
		RepositoryChecksumFailed: input.RepositoryChecksumFailed, WikiChecksumFailed: input.WikiChecksumFailed,
		RepositoryStorage: input.RepositoryStorage, WithCustomAttributes: input.WithCustomAttributes,
		CustomAttributes: input.CustomAttributes,
		PaginationInput:  input.PaginationInput, KeysetPaginationInput: input.KeysetPaginationInput,
	}
}

// applyKeysetOrder sets the keyset ordering column and direction on a
// gl.ListOptions, leaving unset values untouched. GitLab uses order_by/sort
// from the embedded ListOptions to drive keyset pagination on small list
// endpoints (users, groups, starrers, invited groups, hooks).
func applyKeysetOrder(opts *gl.ListOptions, orderBy, sort string) {
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
}

// List retrieves a paginated list of GitLab projects.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	opts := buildListOpts(input)
	projects, resp, err := client.GL().Projects.ListProjects(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("projectList", err)
	}
	out := make([]Output, len(projects))
	for i, p := range projects {
		out[i] = ToOutput(p)
	}
	return ListOutput{Projects: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Delete deletes a GitLab project by its ID or URL-encoded path.
// When the GitLab instance has delayed deletion enabled, the project is marked
// for deletion rather than removed immediately. When permanently_remove is true
// and the instance requires a two-step process (mark-then-remove), the handler
// performs both steps automatically.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	if err := ctx.Err(); err != nil {
		return DeleteOutput{}, err
	}
	if input.ProjectID == "" {
		return DeleteOutput{}, errors.New("projectDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.DeleteProjectOptions{}
	if input.PermanentlyRemove {
		opts.PermanentlyRemove = new(true)
		if input.FullPath != "" {
			opts.FullPath = new(input.FullPath)
		}
	}

	_, err := client.GL().Projects.DeleteProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.ContainsAny(err, "already been marked for deletion", "already marked for deletion") {
			return DeleteOutput{
				Status:             "already_scheduled",
				Message:            fmt.Sprintf("Project %s is already marked for deletion. Use permanently_remove=true with full_path to delete immediately, or use gitlab_project_restore to cancel the deletion.", input.ProjectID),
				PermanentlyRemoved: false,
			}, nil
		}

		// GitLab CE may require marking for deletion before permanent removal.
		if input.PermanentlyRemove && toolutil.ContainsAny(err, "must be marked for deletion first", "marked for deletion first") {
			return deleteTwoStep(ctx, client, input)
		}

		return DeleteOutput{}, toolutil.WrapErrWithMessage("projectDelete", err)
	}

	if input.PermanentlyRemove {
		return DeleteOutput{
			Status:             "success",
			Message:            fmt.Sprintf("Project %s has been permanently deleted.", input.ProjectID),
			PermanentlyRemoved: true,
		}, nil
	}

	// Check if the project still exists (marked for delayed deletion)
	p, _, getErr := client.GL().Projects.GetProject(string(input.ProjectID), &gl.GetProjectOptions{}, gl.WithContext(ctx))
	if getErr == nil && p.MarkedForDeletionOn != nil {
		deletionDate := time.Time(*p.MarkedForDeletionOn).Format(time.DateOnly)
		return DeleteOutput{
			Status:              "scheduled",
			Message:             fmt.Sprintf("Project %s is marked for deletion on %s. Use gitlab_project_delete with permanently_remove=true and full_path to delete immediately, or use gitlab_project_restore to cancel the deletion.", input.ProjectID, deletionDate),
			MarkedForDeletionOn: deletionDate,
			PermanentlyRemoved:  false,
		}, nil
	}

	return DeleteOutput{
		Status:             "success",
		Message:            fmt.Sprintf("Project %s has been deleted.", input.ProjectID),
		PermanentlyRemoved: true,
	}, nil
}

// deleteTwoStep handles GitLab instances that require marking a project
// for deletion before it can be permanently removed (e.g. CE with delayed deletion).
func deleteTwoStep(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	// Step 1: Mark for deletion (no options).
	_, err := client.GL().Projects.DeleteProject(string(input.ProjectID), nil, gl.WithContext(ctx))
	if err != nil {
		if !toolutil.ContainsAny(err, "already been marked for deletion", "already marked for deletion") {
			return DeleteOutput{}, toolutil.WrapErrWithMessage("projectDelete (mark)", err)
		}
	}

	// The project path changes after marking for deletion (GitLab appends "-deletion_scheduled-{ID}").
	// Re-fetch the current path.
	fullPath := input.FullPath
	if fullPath != "" {
		p, _, getErr := client.GL().Projects.GetProject(string(input.ProjectID), &gl.GetProjectOptions{}, gl.WithContext(ctx))
		if getErr == nil {
			fullPath = p.PathWithNamespace
		}
	}

	// Step 2: Permanently remove.
	permOpts := &gl.DeleteProjectOptions{
		PermanentlyRemove: new(true),
	}
	if fullPath != "" {
		permOpts.FullPath = &fullPath
	}
	_, err = client.GL().Projects.DeleteProject(string(input.ProjectID), permOpts, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithMessage("projectDelete (permanent)", err)
	}
	return DeleteOutput{
		Status:             "success",
		Message:            fmt.Sprintf("Project %s has been permanently deleted (two-step: marked then removed).", input.ProjectID),
		PermanentlyRemoved: true,
	}, nil
}

// Restore restores a GitLab project that has been marked for deletion.
func Restore(ctx context.Context, client *gitlabclient.Client, input RestoreInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectRestore: project_id is required. Use gitlab_project_list with include_pending_delete=true to find projects marked for deletion")
	}
	p, _, err := client.GL().Projects.RestoreProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return Output{}, toolutil.WrapErrWithHint("projectRestore", err,
				"project must be in pending_delete state to restore \u2014 use gitlab_project_list with include_pending_delete=true to verify, or gitlab_project_get to inspect marked_for_deletion_on")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("projectRestore", err, http.StatusNotFound,
			"verify project_id; restoring requires the project to be in pending_delete state \u2014 use gitlab_project_list with include_pending_delete=true")
	}
	return ToOutput(p), nil
}

// buildUpdateOpts maps UpdateInput fields to the GitLab API edit options.
func buildUpdateOpts(input UpdateInput) *gl.EditProjectOptions {
	opts := &gl.EditProjectOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}
	if input.MergeMethod != "" {
		opts.MergeMethod = new(gl.MergeMethodValue(input.MergeMethod))
	}
	if len(input.Topics) > 0 {
		opts.Topics = &input.Topics
	}
	if input.SquashOption != "" {
		opts.SquashOption = new(gl.SquashOptionValue(input.SquashOption))
	}
	if input.OnlyAllowMergeIfPipelineSucceeds != nil {
		opts.OnlyAllowMergeIfPipelineSucceeds = input.OnlyAllowMergeIfPipelineSucceeds
	}
	if input.OnlyAllowMergeIfAllDiscussionsAreResolved != nil {
		opts.OnlyAllowMergeIfAllDiscussionsAreResolved = input.OnlyAllowMergeIfAllDiscussionsAreResolved
	}
	if input.IssuesEnabled != nil {
		opts.IssuesAccessLevel = boolToAccessLevel(input.IssuesEnabled)
	}
	if input.MergeRequestsEnabled != nil {
		opts.MergeRequestsAccessLevel = boolToAccessLevel(input.MergeRequestsEnabled)
	}
	if input.WikiEnabled != nil {
		opts.WikiAccessLevel = boolToAccessLevel(input.WikiEnabled)
	}
	if input.JobsEnabled != nil {
		opts.BuildsAccessLevel = boolToAccessLevel(input.JobsEnabled)
	}
	applyUpdateFeatureOpts(opts, input)
	return opts
}

// applyUpdateFeatureOpts sets optional feature toggles and advanced merge settings.
func applyUpdateFeatureOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	applyUpdateMergeOpts(opts, input)
	applyUpdateAccessOpts(opts, input)
	applyUpdateMetadataOpts(opts, input)
	applyUpdateToggleOpts(opts, input)
	applyUpdatePipelineOpts(opts, input)
	applyUpdateCIOpts(opts, input)
	applyUpdateMirrorOpts(opts, input)
	applyUpdateComplianceOpts(opts, input)
	applyUpdateAccessLevelOpts(opts, input)
}

// applyUpdateMetadataOpts applies basic metadata and template transformations.
func applyUpdateMetadataOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.ImportURL != "" {
		opts.ImportURL = new(input.ImportURL)
	}
	if input.RepositoryStorage != "" {
		opts.RepositoryStorage = new(input.RepositoryStorage)
	}
	if len(input.TagList) > 0 {
		opts.TagList = &input.TagList //nolint:staticcheck // 1:1 SDK parity; prefer topics.
	}
	if input.SuggestionCommitMessage != "" {
		opts.SuggestionCommitMessage = new(input.SuggestionCommitMessage)
	}
	if input.IssueBranchTemplate != "" {
		opts.IssueBranchTemplate = new(input.IssueBranchTemplate)
	}
	if input.IssuesTemplate != "" {
		opts.IssuesTemplate = new(input.IssuesTemplate)
	}
	if input.MergeRequestsTemplate != "" {
		opts.MergeRequestsTemplate = new(input.MergeRequestsTemplate)
	}
}

// applyUpdateToggleOpts applies feature-toggle transformations.
func applyUpdateToggleOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.SnippetsEnabled != nil {
		opts.SnippetsEnabled = input.SnippetsEnabled //nolint:staticcheck // 1:1 SDK parity; prefer snippets_access_level.
	}
	if input.ContainerRegistryEnabled != nil {
		opts.ContainerRegistryEnabled = input.ContainerRegistryEnabled //nolint:staticcheck // 1:1 SDK parity; prefer container_registry_access_level.
	}
	if input.ServiceDeskEnabled != nil {
		opts.ServiceDeskEnabled = input.ServiceDeskEnabled
	}
	if input.EmailsEnabled != nil {
		opts.EmailsEnabled = input.EmailsEnabled
	}
	if input.EmailsDisabled != nil {
		opts.EmailsDisabled = input.EmailsDisabled //nolint:staticcheck // 1:1 SDK parity; prefer emails_enabled.
	}
	if input.ShowDefaultAwardEmojis != nil {
		opts.ShowDefaultAwardEmojis = input.ShowDefaultAwardEmojis
	}
	if input.GroupRunnersEnabled != nil {
		opts.GroupRunnersEnabled = input.GroupRunnersEnabled
	}
	if input.KeepLatestArtifact != nil {
		opts.KeepLatestArtifact = input.KeepLatestArtifact
	}
	if input.BuildTimeout > 0 {
		opts.BuildTimeout = new(input.BuildTimeout)
	}
	if input.MaxArtifactsSize > 0 {
		opts.MaxArtifactsSize = new(input.MaxArtifactsSize)
	}
}

// applyUpdatePipelineOpts applies Auto DevOps and pipeline transformations.
func applyUpdatePipelineOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.AutoDevopsEnabled != nil {
		opts.AutoDevopsEnabled = input.AutoDevopsEnabled
	}
	if input.AutoDevopsDeployStrategy != "" {
		opts.AutoDevopsDeployStrategy = new(input.AutoDevopsDeployStrategy)
	}
	if input.AutoDuoCodeReviewEnabled != nil {
		opts.AutoDuoCodeReviewEnabled = input.AutoDuoCodeReviewEnabled
	}
	if input.BuildGitStrategy != "" {
		opts.BuildGitStrategy = new(input.BuildGitStrategy)
	}
	if input.BuildCoverageRegex != "" {
		opts.BuildCoverageRegex = new(input.BuildCoverageRegex)
	}
	if input.AutoCancelPendingPipelines != "" {
		opts.AutoCancelPendingPipelines = new(input.AutoCancelPendingPipelines)
	}
	if input.ResourceGroupDefaultProcessMode != "" {
		opts.ResourceGroupDefaultProcessMode = new(gl.ResourceGroupProcessMode(input.ResourceGroupDefaultProcessMode))
	}
	if input.PublicJobs != nil {
		opts.PublicJobs = input.PublicJobs
	}
}

// applyUpdateCIOpts applies advanced CI/CD transformations.
func applyUpdateCIOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.CIDefaultGitDepth > 0 {
		opts.CIDefaultGitDepth = new(input.CIDefaultGitDepth)
	}
	if input.CICDCatalogEnabled != nil {
		opts.CICDCatalogEnabled = input.CICDCatalogEnabled
	}
	if input.CIDeletePipelinesInSeconds > 0 {
		opts.CIDeletePipelinesInSeconds = new(input.CIDeletePipelinesInSeconds)
	}
	if input.CIDisplayPipelineVariables != nil {
		opts.CIDisplayPipelineVariables = input.CIDisplayPipelineVariables
	}
	if input.CIForwardDeploymentEnabled != nil {
		opts.CIForwardDeploymentEnabled = input.CIForwardDeploymentEnabled
	}
	if input.CIForwardDeploymentRollbackAllowed != nil {
		opts.CIForwardDeploymentRollbackAllowed = input.CIForwardDeploymentRollbackAllowed
	}
	if input.CIPushRepositoryForJobTokenAllowed != nil {
		opts.CIPushRepositoryForJobTokenAllowed = input.CIPushRepositoryForJobTokenAllowed
	}
	if len(input.CIIDTokenSubClaimComponents) > 0 {
		opts.CIIdTokenSubClaimComponents = &input.CIIDTokenSubClaimComponents
	}
	if input.CISeparatedCaches != nil {
		opts.CISeparatedCaches = input.CISeparatedCaches
	}
	if input.CIRestrictPipelineCancellationRole != "" {
		opts.CIRestrictPipelineCancellationRole = new(gl.AccessControlValue(input.CIRestrictPipelineCancellationRole))
	}
	if input.CIPipelineVariablesMinimumOverrideRole != "" {
		opts.CIPipelineVariablesMinimumOverrideRole = new(input.CIPipelineVariablesMinimumOverrideRole)
	}
	if input.RestrictUserDefinedVariables != nil {
		opts.RestrictUserDefinedVariables = input.RestrictUserDefinedVariables //nolint:staticcheck // 1:1 SDK parity; prefer ci_pipeline_variables_minimum_override_role.
	}
}

// applyUpdateMirrorOpts applies pull-mirroring transformations.
func applyUpdateMirrorOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.Mirror != nil {
		opts.Mirror = input.Mirror
	}
	if input.MirrorTriggerBuilds != nil {
		opts.MirrorTriggerBuilds = input.MirrorTriggerBuilds
	}
	if input.MirrorBranchRegex != "" {
		opts.MirrorBranchRegex = new(input.MirrorBranchRegex)
	}
	if input.MirrorOverwritesDivergedBranches != nil {
		opts.MirrorOverwritesDivergedBranches = input.MirrorOverwritesDivergedBranches
	}
	if input.MirrorUserID > 0 {
		opts.MirrorUserID = new(input.MirrorUserID)
	}
	if input.OnlyMirrorProtectedBranches != nil {
		opts.OnlyMirrorProtectedBranches = input.OnlyMirrorProtectedBranches
	}
}

// applyUpdateComplianceOpts applies compliance, security, and additive merge
// transformations.
func applyUpdateComplianceOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.EnforceAuthChecksOnUploads != nil {
		opts.EnforceAuthChecksOnUploads = input.EnforceAuthChecksOnUploads
	}
	if input.ExternalAuthorizationClassificationLabel != "" {
		opts.ExternalAuthorizationClassificationLabel = new(input.ExternalAuthorizationClassificationLabel)
	}
	opts.ContainerExpirationPolicyAttributes = input.ContainerExpirationPolicyAttributes.toGL()
	if input.OnlyAllowMergeIfAllStatusChecksPassed != nil {
		opts.OnlyAllowMergeIfAllStatusChecksPassed = input.OnlyAllowMergeIfAllStatusChecksPassed
	}
	if input.AllowPipelineTriggerApproveDeployment != nil {
		opts.AllowPipelineTriggerApproveDeployment = input.AllowPipelineTriggerApproveDeployment
	}
	if input.PreventMergeWithoutJiraIssue != nil {
		opts.PreventMergeWithoutJiraIssue = input.PreventMergeWithoutJiraIssue
	}
	if input.MergeRequestDefaultTargetSelf != nil {
		opts.MergeRequestDefaultTargetSelf = input.MergeRequestDefaultTargetSelf
	}
	if input.MergeTrainsSkipTrainAllowed != nil {
		opts.MergeTrainsSkipTrainAllowed = input.MergeTrainsSkipTrainAllowed
	}
	if input.PrintingMergeRequestLinkEnabled != nil {
		opts.PrintingMergeRequestLinkEnabled = input.PrintingMergeRequestLinkEnabled
	}
}

// applyUpdateAccessLevelOpts applies the additive string-based access-level
// transformations.
func applyUpdateAccessLevelOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	opts.IssuesAccessLevel = updateAccessLevel(input.IssuesAccessLevel, input.IssuesEnabled, opts.IssuesAccessLevel)
	opts.MergeRequestsAccessLevel = updateAccessLevel(input.MergeRequestsAccessLevel, input.MergeRequestsEnabled, opts.MergeRequestsAccessLevel)
	opts.WikiAccessLevel = updateAccessLevel(input.WikiAccessLevel, input.WikiEnabled, opts.WikiAccessLevel)
	opts.BuildsAccessLevel = updateAccessLevel(input.BuildsAccessLevel, input.JobsEnabled, opts.BuildsAccessLevel)
	setAccessLevel(&opts.RepositoryAccessLevel, input.RepositoryAccessLevel)
	setAccessLevel(&opts.ForkingAccessLevel, input.ForkingAccessLevel)
	setAccessLevel(&opts.AnalyticsAccessLevel, input.AnalyticsAccessLevel)
	setAccessLevel(&opts.OperationsAccessLevel, input.OperationsAccessLevel)
	setAccessLevel(&opts.ReleasesAccessLevel, input.ReleasesAccessLevel)
	setAccessLevel(&opts.EnvironmentsAccessLevel, input.EnvironmentsAccessLevel)
	setAccessLevel(&opts.FeatureFlagsAccessLevel, input.FeatureFlagsAccessLevel)
	setAccessLevel(&opts.InfrastructureAccessLevel, input.InfrastructureAccessLevel)
	setAccessLevel(&opts.MonitorAccessLevel, input.MonitorAccessLevel)
	setAccessLevel(&opts.RequirementsAccessLevel, input.RequirementsAccessLevel)
	setAccessLevel(&opts.SecurityAndComplianceAccessLevel, input.SecurityAndComplianceAccessLevel)
	setAccessLevel(&opts.ModelExperimentsAccessLevel, input.ModelExperimentsAccessLevel)
	setAccessLevel(&opts.ModelRegistryAccessLevel, input.ModelRegistryAccessLevel)
}

// setAccessLevel sets the target access-level pointer from a string when the
// string is non-empty, leaving the existing value untouched otherwise.
func setAccessLevel(target **gl.AccessControlValue, level string) {
	if level != "" {
		*target = new(gl.AccessControlValue(level))
	}
}

// updateAccessLevel resolves the access level for a feature, preferring the
// explicit string, then the existing value (set by the deprecated bool bridge),
// and falling back to the bool toggle.
func updateAccessLevel(level string, toggle *bool, existing *gl.AccessControlValue) *gl.AccessControlValue {
	if level != "" {
		return new(gl.AccessControlValue(level))
	}
	if existing != nil {
		return existing
	}
	return boolToAccessLevel(toggle)
}

func applyUpdateMergeOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.CIConfigPath != "" {
		opts.CIConfigPath = new(input.CIConfigPath)
	}
	if input.AllowMergeOnSkippedPipeline != nil {
		opts.AllowMergeOnSkippedPipeline = input.AllowMergeOnSkippedPipeline
	}
	if input.RemoveSourceBranchAfterMerge != nil {
		opts.RemoveSourceBranchAfterMerge = input.RemoveSourceBranchAfterMerge
	}
	if input.AutocloseReferencedIssues != nil {
		opts.AutocloseReferencedIssues = input.AutocloseReferencedIssues
	}
	if input.MergeCommitTemplate != "" {
		opts.MergeCommitTemplate = new(input.MergeCommitTemplate)
	}
	if input.SquashCommitTemplate != "" {
		opts.SquashCommitTemplate = new(input.SquashCommitTemplate)
	}
	if input.MRDefaultTitleTemplate != "" {
		opts.MRDefaultTitleTemplate = new(input.MRDefaultTitleTemplate)
	}
	if input.MergePipelinesEnabled != nil {
		opts.MergePipelinesEnabled = input.MergePipelinesEnabled
	}
	if input.MergeTrainsEnabled != nil {
		opts.MergeTrainsEnabled = input.MergeTrainsEnabled
	}
	if input.ProtectMergeRequestPipelines != nil {
		opts.ProtectMergeRequestPipelines = input.ProtectMergeRequestPipelines
	}
	if input.ResolveOutdatedDiffDiscussions != nil {
		opts.ResolveOutdatedDiffDiscussions = input.ResolveOutdatedDiffDiscussions
	}
	if input.ApprovalsBeforeMerge > 0 {
		opts.ApprovalsBeforeMerge = new(input.ApprovalsBeforeMerge) //nolint:staticcheck // No replacement field, needs Merge Request Approvals API.
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
}

func applyUpdateAccessOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
	if input.SharedRunnersEnabled != nil {
		opts.SharedRunnersEnabled = input.SharedRunnersEnabled
	}
	if input.PublicBuilds != nil {
		opts.PublicJobs = input.PublicBuilds
	}
	if input.PackagesEnabled != nil {
		opts.PackagesEnabled = input.PackagesEnabled //nolint:staticcheck // Preserve backward-compatible input field.
	}
	if input.PackageRegistryAccessLevel != "" {
		opts.PackageRegistryAccessLevel = new(gl.AccessControlValue(input.PackageRegistryAccessLevel))
	}
	if input.PagesAccessLevel != "" {
		opts.PagesAccessLevel = new(gl.AccessControlValue(input.PagesAccessLevel))
	}
	if input.ContainerRegistryAccessLevel != "" {
		opts.ContainerRegistryAccessLevel = new(gl.AccessControlValue(input.ContainerRegistryAccessLevel))
	}
	if input.SnippetsAccessLevel != "" {
		opts.SnippetsAccessLevel = new(gl.AccessControlValue(input.SnippetsAccessLevel))
	}
	if input.MergeRequestTitleRegex != "" {
		opts.MergeRequestTitleRegex = new(input.MergeRequestTitleRegex)
	}
	if input.MergeRequestTitleRegexDescription != "" {
		opts.MergeRequestTitleRegexDescription = new(input.MergeRequestTitleRegexDescription)
	}
	if input.ReviewerAssignmentStrategy != "" {
		opts.ReviewerAssignmentStrategy = new(gl.ReviewerAssignmentStrategyValue(input.ReviewerAssignmentStrategy))
	}
}

// Update modifies settings on an existing GitLab project.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := buildUpdateOpts(input)
	p, _, err := client.GL().Projects.EditProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("projectUpdate", err, "check that the project settings are valid — name/path must be unique in the namespace")
		case toolutil.IsHTTPStatus(err, http.StatusForbidden):
			return Output{}, toolutil.WrapErrWithHint("projectUpdate", err, "you need at least Maintainer role to update project settings")
		default:
			return Output{}, toolutil.WrapErrWithMessage("projectUpdate", err)
		}
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Fork
// ---------------------------------------------------------------------------.

// ForkInput defines parameters for forking a project.
type ForkInput struct {
	ProjectID                     toolutil.StringOrInt `json:"project_id" jsonschema:"Source project ID or URL-encoded path,required"`
	Name                          string               `json:"name,omitempty" jsonschema:"Name for the forked project"`
	Path                          string               `json:"path,omitempty" jsonschema:"Path slug for the forked project"`
	NamespaceID                   int64                `json:"namespace_id,omitempty" jsonschema:"Namespace ID to fork into"`
	NamespacePath                 string               `json:"namespace_path,omitempty" jsonschema:"Namespace path to fork into"`
	Namespace                     string               `json:"namespace,omitempty" jsonschema:"Namespace ID or path to fork into (deprecated: use namespace_id or namespace_path)"`
	Description                   string               `json:"description,omitempty" jsonschema:"Description for the forked project"`
	Visibility                    string               `json:"visibility,omitempty" jsonschema:"Visibility level (private, internal, public)"`
	Branches                      string               `json:"branches,omitempty" jsonschema:"Branches to fork (empty=all)"`
	MergeRequestDefaultTargetSelf *bool                `json:"mr_default_target_self,omitempty" jsonschema:"MR default target is the fork itself instead of upstream"`
}

// Fork creates a fork of an existing GitLab project.
func Fork(ctx context.Context, client *gitlabclient.Client, input ForkInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectFork: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ForkProjectOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.NamespaceID > 0 {
		opts.NamespaceID = new(input.NamespaceID)
	}
	if input.NamespacePath != "" {
		opts.NamespacePath = new(input.NamespacePath)
	}
	if input.Namespace != "" {
		opts.Namespace = new(input.Namespace) //nolint:staticcheck // 1:1 SDK parity; prefer namespace_id or namespace_path.
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Visibility != "" {
		v := gl.VisibilityValue(input.Visibility)
		opts.Visibility = &v
	}
	if input.Branches != "" {
		opts.Branches = new(input.Branches)
	}
	if input.MergeRequestDefaultTargetSelf != nil {
		opts.MergeRequestDefaultTargetSelf = input.MergeRequestDefaultTargetSelf
	}
	p, _, err := client.GL().Projects.ForkProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return Output{}, toolutil.WrapErrWithHint("projectFork", err, "a fork of this project already exists in your namespace — use gitlab_project_list to find it")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectFork", err)
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Star / Unstar
// ---------------------------------------------------------------------------.

// StarInput defines parameters for starring a project.
type StarInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Star adds a star to a project for the authenticated user.
func Star(ctx context.Context, client *gitlabclient.Client, input StarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectStar: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.StarProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return Get(ctx, client, GetInput{ProjectID: input.ProjectID})
		}
		return Output{}, toolutil.WrapErrWithStatusHint("projectStar", err, http.StatusNotModified,
			"project is already starred by the authenticated user \u2014 use gitlab_project_get to inspect star_count and gitlab_project_list_user_starred to list current stars")
	}
	return ToOutput(p), nil
}

// UnstarInput defines parameters for unstarring a project.
type UnstarInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Unstar removes the star from a project for the authenticated user.
func Unstar(ctx context.Context, client *gitlabclient.Client, input UnstarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUnstar: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.UnstarProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("projectUnstar", err, http.StatusNotModified,
			"project is not currently starred by the authenticated user \u2014 nothing to unstar")
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Archive / Unarchive
// ---------------------------------------------------------------------------.

// ArchiveInput defines parameters for archiving a project.
type ArchiveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Archive sets a project to archived (read-only) state.
func Archive(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectArchive: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.ArchiveProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("projectArchive", err, "only project owners can archive projects")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectArchive", err)
	}
	return ToOutput(p), nil
}

// UnarchiveInput defines parameters for unarchiving a project.
type UnarchiveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Unarchive removes the archived (read-only) state from a project.
func Unarchive(ctx context.Context, client *gitlabclient.Client, input UnarchiveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUnarchive: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.UnarchiveProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("projectUnarchive", err, "only project owners can unarchive projects")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectUnarchive", err)
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Transfer
// ---------------------------------------------------------------------------.

// TransferInput defines parameters for transferring a project to another namespace.
type TransferInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Namespace string               `json:"namespace" jsonschema:"Target namespace ID or path,required"`
}

// Transfer moves a project to a different namespace.
func Transfer(ctx context.Context, client *gitlabclient.Client, input TransferInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectTransfer: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.Namespace == "" {
		return Output{}, errors.New("projectTransfer: namespace is required. Provide the target namespace ID or path (e.g. 'my-group' or '42')")
	}
	opts := &gl.TransferProjectOptions{
		Namespace: input.Namespace,
	}
	p, _, err := client.GL().Projects.TransferProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusForbidden):
			return Output{}, toolutil.WrapErrWithHint("projectTransfer", err,
				"transferring a project requires Owner role on the source AND permission to create projects in the target namespace")
		case toolutil.IsHTTPStatus(err, http.StatusNotFound):
			return Output{}, toolutil.WrapErrWithHint("projectTransfer", err,
				"verify the target namespace exists \u2014 use gitlab_group_list or gitlab_get_user; namespace must be a numeric ID or full path")
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("projectTransfer", err,
				"target namespace may already contain a project with this name/path; consider renaming the project before transferring")
		default:
			return Output{}, toolutil.WrapErrWithMessage("projectTransfer", err)
		}
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// List Forks
// ---------------------------------------------------------------------------.

// ListForksInput defines parameters for listing project forks. The forks
// endpoint accepts the same filter set as the project list endpoint
// (ListProjectsOptions), so every filter is surfaced here for 1:1 SDK parity.
type ListForksInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Owned      bool                 `json:"owned,omitempty" jsonschema:"Limit to forks owned by the current user"`
	Search     string               `json:"search,omitempty" jsonschema:"Search query for fork name"`
	Visibility string               `json:"visibility,omitempty" jsonschema:"Filter by visibility (private, internal, public)"`
	OrderBy    string               `json:"order_by,omitempty" jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort       string               `json:"sort,omitempty" jsonschema:"Sort direction (asc, desc)"`

	// Additional filters (additive 1:1 SDK parity with ListProjectsOptions)
	Archived                 *bool             `json:"archived,omitempty"                    jsonschema:"Filter by archived status (true=only archived, false=only active)"`
	Topic                    string            `json:"topic,omitempty"                       jsonschema:"Filter by topic name"`
	Simple                   bool              `json:"simple,omitempty"                      jsonschema:"Return only limited fields (faster for large result sets)"`
	MinAccessLevel           int               `json:"min_access_level,omitempty"            jsonschema:"Filter by minimum access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	LastActivityAfter        string            `json:"last_activity_after,omitempty"         jsonschema:"Return forks with last activity after date (ISO 8601 format)"`
	LastActivityBefore       string            `json:"last_activity_before,omitempty"        jsonschema:"Return forks with last activity before date (ISO 8601 format)"`
	Starred                  *bool             `json:"starred,omitempty"                     jsonschema:"Limit to forks starred by the current user"`
	Membership               *bool             `json:"membership,omitempty"                  jsonschema:"Limit to forks where the current user is a member"`
	WithIssuesEnabled        *bool             `json:"with_issues_enabled,omitempty"         jsonschema:"Filter by forks with issues feature enabled"`
	WithMergeRequestsEnabled *bool             `json:"with_merge_requests_enabled,omitempty" jsonschema:"Filter by forks with merge requests enabled"`
	SearchNamespaces         *bool             `json:"search_namespaces,omitempty"           jsonschema:"Include namespace in search"`
	Statistics               *bool             `json:"statistics,omitempty"                  jsonschema:"Include project statistics in response"`
	WithProgrammingLanguage  string            `json:"with_programming_language,omitempty"   jsonschema:"Filter by programming language name"`
	IncludePendingDelete     *bool             `json:"include_pending_delete,omitempty"      jsonschema:"Include forks marked/scheduled for deletion. Default false."`
	IncludeHidden            *bool             `json:"include_hidden,omitempty"              jsonschema:"Include hidden forks in results"`
	IDAfter                  int64             `json:"id_after,omitempty"                    jsonschema:"Return forks with ID greater than this value (keyset pagination)"`
	IDBefore                 int64             `json:"id_before,omitempty"                   jsonschema:"Return forks with ID less than this value (keyset pagination)"`
	Active                   *bool             `json:"active,omitempty"                      jsonschema:"Limit to active (non-archived, non-pending-deletion) forks"`
	Imported                 *bool             `json:"imported,omitempty"                    jsonschema:"Limit to forks imported from an external source by the current user"`
	RepositoryChecksumFailed *bool             `json:"repository_checksum_failed,omitempty"  jsonschema:"Limit to forks where the repository checksum calculation failed (admin only)"`
	WikiChecksumFailed       *bool             `json:"wiki_checksum_failed,omitempty"        jsonschema:"Limit to forks where the wiki checksum calculation failed (admin only)"`
	RepositoryStorage        string            `json:"repository_storage,omitempty"          jsonschema:"Limit to forks on the given repository storage shard (admin only)"`
	WithCustomAttributes     *bool             `json:"with_custom_attributes,omitempty"      jsonschema:"Include custom attributes in the response (admin only)"`
	CustomAttributes         map[string]string `json:"custom_attributes,omitempty"      jsonschema:"Filter projects by custom attribute key/value pairs (admin only). Different from with_custom_attributes, which only controls whether attributes are returned"`

	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListForksOutput holds a paginated list of project forks.
type ListForksOutput struct {
	toolutil.HintableOutput
	Forks      []Output                  `json:"forks"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// buildForkListOpts maps ListForksInput filters onto the shared
// ListProjectsOptions used by the project forks endpoint, delegating to the
// centralized buildUserProjectOpts.
func buildForkListOpts(input ListForksInput) *gl.ListProjectsOptions {
	return buildUserProjectOpts(input.toFilter())
}

// toFilter copies a ListForksInput into the shared userProjectFilter.
//
//nolint:dupl // Field-by-field copy into the shared filter; structurally parallel to the ListInput/userProjectFilterInput copies by design (distinct top-level MCP input structs, no shared embeddable without flattening JSON schemas).
func (input ListForksInput) toFilter() userProjectFilter {
	return userProjectFilter{
		Search: input.Search, Visibility: input.Visibility, Archived: input.Archived,
		OrderBy: input.OrderBy, Sort: input.Sort, Simple: input.Simple, Owned: input.Owned,
		Topic: input.Topic, MinAccessLevel: input.MinAccessLevel, Starred: input.Starred,
		Membership: input.Membership, WithIssuesEnabled: input.WithIssuesEnabled,
		WithMergeRequestsEnabled: input.WithMergeRequestsEnabled, SearchNamespaces: input.SearchNamespaces,
		Statistics: input.Statistics, WithProgrammingLanguage: input.WithProgrammingLanguage,
		LastActivityAfter: input.LastActivityAfter, LastActivityBefore: input.LastActivityBefore,
		IDAfter: input.IDAfter, IDBefore: input.IDBefore, IncludePendingDelete: input.IncludePendingDelete,
		IncludeHidden: input.IncludeHidden, Active: input.Active, Imported: input.Imported,
		RepositoryChecksumFailed: input.RepositoryChecksumFailed, WikiChecksumFailed: input.WikiChecksumFailed,
		RepositoryStorage: input.RepositoryStorage, WithCustomAttributes: input.WithCustomAttributes,
		CustomAttributes: input.CustomAttributes,
		PaginationInput:  input.PaginationInput, KeysetPaginationInput: input.KeysetPaginationInput,
	}
}

// ListForks retrieves a paginated list of forks for a project.
func ListForks(ctx context.Context, client *gitlabclient.Client, input ListForksInput) (ListForksOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListForksOutput{}, err
	}
	if input.ProjectID == "" {
		return ListForksOutput{}, errors.New("projectListForks: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := buildForkListOpts(input)
	forks, resp, err := client.GL().Projects.ListProjectForks(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListForksOutput{}, toolutil.WrapErrWithStatusHint("projectListForks", err, http.StatusNotFound,
			"verify the parent project exists with gitlab_project_get")
	}
	out := make([]Output, 0, len(forks))
	for _, f := range forks {
		out = append(out, ToOutput(f))
	}
	return ListForksOutput{
		Forks:      out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// Get Languages
// ---------------------------------------------------------------------------.

// GetLanguagesInput defines parameters for retrieving project languages.
type GetLanguagesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// LanguageEntry represents a single programming language and its percentage.
type LanguageEntry struct {
	Name       string  `json:"name"`
	Percentage float32 `json:"percentage"`
}

// LanguagesOutput holds the programming languages detected in a project.
type LanguagesOutput struct {
	toolutil.HintableOutput
	Languages []LanguageEntry `json:"languages"`
}

// GetLanguages retrieves the programming languages used in a project with percentages.
func GetLanguages(ctx context.Context, client *gitlabclient.Client, input GetLanguagesInput) (LanguagesOutput, error) {
	if err := ctx.Err(); err != nil {
		return LanguagesOutput{}, err
	}
	if input.ProjectID == "" {
		return LanguagesOutput{}, errors.New("projectGetLanguages: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	langs, _, err := client.GL().Projects.GetProjectLanguages(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return LanguagesOutput{}, toolutil.WrapErrWithStatusHint("projectGetLanguages", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get; language detection requires repository content (empty repos return no languages)")
	}
	entries := make([]LanguageEntry, 0, len(*langs))
	for name, pct := range *langs {
		entries = append(entries, LanguageEntry{Name: name, Percentage: pct})
	}
	return LanguagesOutput{Languages: entries}, nil
}

// ---------------------------------------------------------------------------
// Project Hooks (Webhooks)
// ---------------------------------------------------------------------------.

// HookOutput represents a project webhook.
type HookOutput struct {
	toolutil.HintableOutput
	ID                        int64              `json:"id"`
	URL                       string             `json:"url"`
	Name                      string             `json:"name,omitempty"`
	Description               string             `json:"description,omitempty"`
	ProjectID                 int64              `json:"project_id"`
	PushEvents                bool               `json:"push_events"`
	PushEventsBranchFilter    string             `json:"push_events_branch_filter,omitempty"`
	IssuesEvents              bool               `json:"issues_events"`
	ConfidentialIssuesEvents  bool               `json:"confidential_issues_events"`
	MergeRequestsEvents       bool               `json:"merge_requests_events"`
	TagPushEvents             bool               `json:"tag_push_events"`
	NoteEvents                bool               `json:"note_events"`
	ConfidentialNoteEvents    bool               `json:"confidential_note_events"`
	JobEvents                 bool               `json:"job_events"`
	PipelineEvents            bool               `json:"pipeline_events"`
	WikiPageEvents            bool               `json:"wiki_page_events"`
	DeploymentEvents          bool               `json:"deployment_events"`
	ReleasesEvents            bool               `json:"releases_events"`
	MilestoneEvents           bool               `json:"milestone_events"`
	FeatureFlagEvents         bool               `json:"feature_flag_events"`
	EmojiEvents               bool               `json:"emoji_events"`
	EnableSSLVerification     bool               `json:"enable_ssl_verification"`
	RepositoryUpdateEvents    bool               `json:"repository_update_events"`
	ResourceAccessTokenEvents bool               `json:"resource_access_token_events"`
	ResourceDeployTokenEvents bool               `json:"resource_deploy_token_events"`
	AlertStatus               string             `json:"alert_status,omitempty"`
	DisabledUntil             string             `json:"disabled_until,omitempty"`
	URLVariables              []HookURLVariable  `json:"url_variables,omitempty"`
	BranchFilterStrategy      string             `json:"branch_filter_strategy,omitempty"`
	CustomWebhookTemplate     string             `json:"custom_webhook_template,omitempty"`
	CustomHeaders             []HookCustomHeader `json:"custom_headers,omitempty"`
	VulnerabilityEvents       bool               `json:"vulnerability_events"`
	TokenPresent              bool               `json:"token_present"`
	SigningTokenPresent       bool               `json:"signing_token_present"`
	CreatedAt                 time.Time          `json:"created_at"`
}

// HookURLVariable mirrors gl.HookURLVariable, a webhook URL variable. The value
// is masked by the API on read (returned empty) and surfaced for 1:1 parity.
type HookURLVariable struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// HookCustomHeader mirrors gl.HookCustomHeader, a webhook custom header. The
// value is masked by the API on read (returned empty) and surfaced for 1:1
// parity.
type HookCustomHeader struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// hookOutputFromGL maps hook output from gl between API and evaluator models.
func hookOutputFromGL(h *gl.ProjectHook) HookOutput {
	out := HookOutput{
		ID:                        h.ID,
		URL:                       h.URL,
		Name:                      h.Name,
		Description:               h.Description,
		ProjectID:                 h.ProjectID,
		PushEvents:                h.PushEvents,
		PushEventsBranchFilter:    h.PushEventsBranchFilter,
		IssuesEvents:              h.IssuesEvents,
		ConfidentialIssuesEvents:  h.ConfidentialIssuesEvents,
		MergeRequestsEvents:       h.MergeRequestsEvents,
		TagPushEvents:             h.TagPushEvents,
		NoteEvents:                h.NoteEvents,
		ConfidentialNoteEvents:    h.ConfidentialNoteEvents,
		JobEvents:                 h.JobEvents,
		PipelineEvents:            h.PipelineEvents,
		WikiPageEvents:            h.WikiPageEvents,
		DeploymentEvents:          h.DeploymentEvents,
		ReleasesEvents:            h.ReleasesEvents,
		MilestoneEvents:           h.MilestoneEvents,
		FeatureFlagEvents:         h.FeatureFlagEvents,
		EmojiEvents:               h.EmojiEvents,
		EnableSSLVerification:     h.EnableSSLVerification,
		RepositoryUpdateEvents:    h.RepositoryUpdateEvents,
		ResourceAccessTokenEvents: h.ResourceAccessTokenEvents,
		AlertStatus:               h.AlertStatus,
		URLVariables:              projectHookURLVariablesToOutput(h.URLVariables),
		BranchFilterStrategy:      h.BranchFilterStrategy,
		CustomWebhookTemplate:     h.CustomWebhookTemplate,
		CustomHeaders:             projectHookCustomHeadersToOutput(h.CustomHeaders),
		VulnerabilityEvents:       h.VulnerabilityEvents,
		TokenPresent:              h.TokenPresent,
		SigningTokenPresent:       h.SigningTokenPresent,
	}
	if h.CreatedAt != nil {
		out.CreatedAt = *h.CreatedAt
	}
	if h.DisabledUntil != nil {
		out.DisabledUntil = h.DisabledUntil.Format(time.RFC3339)
	}
	return out
}

func projectHookURLVariablesToOutput(variables []gl.HookURLVariable) []HookURLVariable {
	if len(variables) == 0 {
		return nil
	}
	out := make([]HookURLVariable, len(variables))
	for i, variable := range variables {
		// Value is intentionally masked: webhook URL-variable values are
		// secret-bearing and never surfaced. The field exists for 1:1 SDK
		// shape parity but is left empty.
		out[i] = HookURLVariable{Key: variable.Key}
	}
	return out
}

func projectHookCustomHeadersToOutput(headers []*gl.HookCustomHeader) []HookCustomHeader {
	if len(headers) == 0 {
		return nil
	}
	out := make([]HookCustomHeader, 0, len(headers))
	for _, header := range headers {
		if header == nil {
			continue
		}
		// Value is intentionally masked: custom-header values are
		// secret-bearing and never surfaced. The field exists for 1:1 SDK
		// shape parity but is left empty.
		out = append(out, HookCustomHeader{Key: header.Key})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type projectHookAPI struct {
	gl.ProjectHook
	ResourceDeployTokenEvents bool `json:"resource_deploy_token_events"`
}

func projectHooksPath(projectID string) string {
	return fmt.Sprintf("projects/%s/hooks", gl.PathEscape(projectID))
}

func projectHookPath(projectID string, hookID int64) string {
	return fmt.Sprintf("%s/%d", projectHooksPath(projectID), hookID)
}

func doProjectHookRequest[T any](ctx context.Context, client *gitlabclient.Client, method, path string, opts any) (T, *gl.Response, error) {
	var out T
	req, err := client.GL().NewRequest(method, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return out, nil, err
	}
	resp, err := client.GL().Do(req, &out)
	return out, resp, err
}

func hookOutputFromAPI(h *projectHookAPI) HookOutput {
	out := hookOutputFromGL(&h.ProjectHook)
	out.ResourceDeployTokenEvents = h.ResourceDeployTokenEvents
	return out
}

// ListHooksInput defines parameters for listing project webhooks.
type ListHooksInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Keyset ordering column (for keyset pagination)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Keyset sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListHooksOutput holds a paginated list of project webhooks.
type ListHooksOutput struct {
	toolutil.HintableOutput
	Hooks      []HookOutput              `json:"hooks"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListHooks retrieves all webhooks for a project.
func ListHooks(ctx context.Context, client *gitlabclient.Client, input ListHooksInput) (ListHooksOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListHooksOutput{}, err
	}
	if input.ProjectID == "" {
		return ListHooksOutput{}, errors.New("projectListHooks: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectHooksOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyKeysetOrder(&opts.ListOptions, input.OrderBy, input.Sort)
	hooks, resp, err := doProjectHookRequest[[]projectHookAPI](ctx, client, http.MethodGet, projectHooksPath(string(input.ProjectID)), opts)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ListHooksOutput{}, toolutil.WrapErrWithHint("projectListHooks", err,
				"only Maintainers and Owners can list project webhooks \u2014 verify your role with gitlab_project_members_list")
		}
		return ListHooksOutput{}, toolutil.WrapErrWithStatusHint("projectListHooks", err, http.StatusNotFound,
			hintVerifyProjectExists)
	}
	out := make([]HookOutput, 0, len(hooks))
	for i := range hooks {
		out = append(out, hookOutputFromAPI(&hooks[i]))
	}
	return ListHooksOutput{Hooks: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetHookInput defines parameters for getting a single project webhook.
type GetHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
}

// GetHook retrieves a single project webhook by ID.
func GetHook(ctx context.Context, client *gitlabclient.Client, input GetHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.ProjectID == "" {
		return HookOutput{}, errors.New("projectGetHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return HookOutput{}, errors.New("projectGetHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	h, _, err := doProjectHookRequest[projectHookAPI](ctx, client, http.MethodGet, projectHookPath(string(input.ProjectID), input.HookID), nil)
	if err != nil {
		return HookOutput{}, toolutil.WrapErrWithStatusHint("projectGetHook", err, http.StatusNotFound,
			"webhook may have been deleted \u2014 use gitlab_project_hook_list to find current hook_id values")
	}
	return hookOutputFromAPI(&h), nil
}

// HookOptionsInput defines common project webhook settings shared by add and edit operations.
type HookOptionsInput struct {
	Name                      string `json:"name,omitempty" jsonschema:"Webhook name"`
	Description               string `json:"description,omitempty" jsonschema:"Webhook description"`
	Token                     string `json:"token,omitempty" jsonschema:"Secret token for validation"`
	SigningToken              string `json:"signing_token,omitempty" jsonschema:"Write-only signing token for webhook signature validation"`
	PushEventsBranchFilter    string `json:"push_events_branch_filter,omitempty" jsonschema:"Branch filter for push events"`
	CustomWebhookTemplate     string `json:"custom_webhook_template,omitempty" jsonschema:"Custom webhook payload template"`
	BranchFilterStrategy      string `json:"branch_filter_strategy,omitempty" jsonschema:"Branch filter strategy (wildcard, regex, all_branches)"`
	PushEvents                *bool  `json:"push_events,omitempty" jsonschema:"Trigger on push events"`
	IssuesEvents              *bool  `json:"issues_events,omitempty" jsonschema:"Trigger on issue events"`
	ConfidentialIssuesEvents  *bool  `json:"confidential_issues_events,omitempty" jsonschema:"Trigger on confidential issue events"`
	MergeRequestsEvents       *bool  `json:"merge_requests_events,omitempty" jsonschema:"Trigger on merge request events"`
	TagPushEvents             *bool  `json:"tag_push_events,omitempty" jsonschema:"Trigger on tag push events"`
	NoteEvents                *bool  `json:"note_events,omitempty" jsonschema:"Trigger on note/comment events"`
	ConfidentialNoteEvents    *bool  `json:"confidential_note_events,omitempty" jsonschema:"Trigger on confidential note events"`
	JobEvents                 *bool  `json:"job_events,omitempty" jsonschema:"Trigger on CI job events"`
	PipelineEvents            *bool  `json:"pipeline_events,omitempty" jsonschema:"Trigger on pipeline events"`
	WikiPageEvents            *bool  `json:"wiki_page_events,omitempty" jsonschema:"Trigger on wiki page events"`
	DeploymentEvents          *bool  `json:"deployment_events,omitempty" jsonschema:"Trigger on deployment events"`
	ReleasesEvents            *bool  `json:"releases_events,omitempty" jsonschema:"Trigger on release events"`
	MilestoneEvents           *bool  `json:"milestone_events,omitempty" jsonschema:"Trigger on milestone events"`
	FeatureFlagEvents         *bool  `json:"feature_flag_events,omitempty" jsonschema:"Trigger on feature flag events"`
	EmojiEvents               *bool  `json:"emoji_events,omitempty" jsonschema:"Trigger on emoji events"`
	ResourceAccessTokenEvents *bool  `json:"resource_access_token_events,omitempty" jsonschema:"Trigger on resource access token events"`
	ResourceDeployTokenEvents *bool  `json:"resource_deploy_token_events,omitempty" jsonschema:"Trigger on resource deploy token events"`
	VulnerabilityEvents       *bool  `json:"vulnerability_events,omitempty" jsonschema:"Trigger on vulnerability events"`
	EnableSSLVerification     *bool  `json:"enable_ssl_verification,omitempty" jsonschema:"Enable SSL verification for webhook"`

	CustomHeaders []HookCustomHeaderInput `json:"custom_headers,omitempty" jsonschema:"Custom HTTP headers sent with each webhook request (key/value pairs)"`
}

// HookCustomHeaderInput represents a single custom HTTP header for a webhook.
type HookCustomHeaderInput struct {
	Key   string `json:"key"   jsonschema:"Header name,required"`
	Value string `json:"value" jsonschema:"Header value (write-only; not returned by the API),required"`
}

// AddHookInput defines parameters for adding a webhook to a project.
type AddHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	URL       string               `json:"url" jsonschema:"Webhook URL,required"`
	HookOptionsInput
}

// AddHook adds a webhook to a project.
func AddHook(ctx context.Context, client *gitlabclient.Client, input AddHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.ProjectID == "" {
		return HookOutput{}, errors.New("projectAddHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.URL == "" {
		return HookOutput{}, errors.New("projectAddHook: url is required. Provide the URL that will receive webhook HTTP POST requests")
	}
	opts := addProjectHookOptions(input)
	h, _, err := doProjectHookRequest[projectHookAPI](ctx, client, http.MethodPost, projectHooksPath(string(input.ProjectID)), opts)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return HookOutput{}, toolutil.WrapErrWithHint("projectAddHook", err,
				"webhook URL must be a reachable HTTP/HTTPS URL (production GitLab requires HTTPS); check that at least one event flag is enabled and the token format is valid")
		}
		return HookOutput{}, toolutil.WrapErrWithStatusHint("projectAddHook", err, http.StatusForbidden,
			"adding webhooks requires at least Maintainer role on the project")
	}
	return hookOutputFromAPI(&h), nil
}

func applyProjectHookOptions(url string, input HookOptionsInput, opts *gl.AddProjectHookOptions) {
	if url != "" {
		opts.URL = new(url)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Token != "" {
		opts.Token = new(input.Token)
	}
	if input.SigningToken != "" {
		opts.SigningToken = new(input.SigningToken)
	}
	if input.PushEventsBranchFilter != "" {
		opts.PushEventsBranchFilter = new(input.PushEventsBranchFilter)
	}
	if input.CustomWebhookTemplate != "" {
		opts.CustomWebhookTemplate = new(input.CustomWebhookTemplate)
	}
	if input.BranchFilterStrategy != "" {
		opts.BranchFilterStrategy = new(input.BranchFilterStrategy)
	}
	if len(input.CustomHeaders) > 0 {
		headers := make([]*gl.HookCustomHeader, 0, len(input.CustomHeaders))
		for _, h := range input.CustomHeaders {
			headers = append(headers, &gl.HookCustomHeader{Key: h.Key, Value: h.Value})
		}
		opts.CustomHeaders = &headers
	}
	applyProjectHookEventOptions(input, opts)
}

func applyProjectHookEventOptions(input HookOptionsInput, opts *gl.AddProjectHookOptions) {
	if input.PushEvents != nil {
		opts.PushEvents = input.PushEvents
	}
	if input.IssuesEvents != nil {
		opts.IssuesEvents = input.IssuesEvents
	}
	if input.ConfidentialIssuesEvents != nil {
		opts.ConfidentialIssuesEvents = input.ConfidentialIssuesEvents
	}
	if input.MergeRequestsEvents != nil {
		opts.MergeRequestsEvents = input.MergeRequestsEvents
	}
	if input.TagPushEvents != nil {
		opts.TagPushEvents = input.TagPushEvents
	}
	if input.NoteEvents != nil {
		opts.NoteEvents = input.NoteEvents
	}
	if input.ConfidentialNoteEvents != nil {
		opts.ConfidentialNoteEvents = input.ConfidentialNoteEvents
	}
	if input.JobEvents != nil {
		opts.JobEvents = input.JobEvents
	}
	if input.PipelineEvents != nil {
		opts.PipelineEvents = input.PipelineEvents
	}
	if input.WikiPageEvents != nil {
		opts.WikiPageEvents = input.WikiPageEvents
	}
	if input.DeploymentEvents != nil {
		opts.DeploymentEvents = input.DeploymentEvents
	}
	if input.ReleasesEvents != nil {
		opts.ReleasesEvents = input.ReleasesEvents
	}
	if input.EmojiEvents != nil {
		opts.EmojiEvents = input.EmojiEvents
	}
	if input.ResourceAccessTokenEvents != nil {
		opts.ResourceAccessTokenEvents = input.ResourceAccessTokenEvents
	}
	if input.ResourceDeployTokenEvents != nil {
		opts.ResourceDeployTokenEvents = input.ResourceDeployTokenEvents
	}
	if input.MilestoneEvents != nil {
		opts.MilestoneEvents = input.MilestoneEvents
	}
	if input.FeatureFlagEvents != nil {
		opts.FeatureFlagEvents = input.FeatureFlagEvents
	}
	if input.VulnerabilityEvents != nil {
		opts.VulnerabilityEvents = input.VulnerabilityEvents
	}
	if input.EnableSSLVerification != nil {
		opts.EnableSSLVerification = input.EnableSSLVerification
	}
}

func addProjectHookOptions(input AddHookInput) *gl.AddProjectHookOptions {
	opts := &gl.AddProjectHookOptions{}
	applyProjectHookOptions(input.URL, input.HookOptionsInput, opts)
	return opts
}

func editProjectHookOptions(input EditHookInput) *gl.EditProjectHookOptions {
	addOptions := &gl.AddProjectHookOptions{}
	applyProjectHookOptions(input.URL, input.HookOptionsInput, addOptions)
	editOptions := gl.EditProjectHookOptions(*addOptions)
	return &editOptions
}

// EditHookInput defines parameters for editing a project webhook.
type EditHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID to edit,required"`
	URL       string               `json:"url,omitempty" jsonschema:"Updated webhook URL"`
	HookOptionsInput
}

// EditHook edits an existing project webhook.
func EditHook(ctx context.Context, client *gitlabclient.Client, input EditHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.ProjectID == "" {
		return HookOutput{}, errors.New("projectEditHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return HookOutput{}, errors.New("projectEditHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	opts := editProjectHookOptions(input)
	h, _, err := doProjectHookRequest[projectHookAPI](ctx, client, http.MethodPut, projectHookPath(string(input.ProjectID), input.HookID), opts)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return HookOutput{}, toolutil.WrapErrWithHint("projectEditHook", err,
				"updated URL must be valid HTTP/HTTPS; verify token format and event flags")
		}
		return HookOutput{}, toolutil.WrapErrWithStatusHint("projectEditHook", err, http.StatusNotFound,
			"hook may have been deleted \u2014 use gitlab_project_hook_list to find current hook_id")
	}
	return hookOutputFromAPI(&h), nil
}

// DeleteHookInput defines parameters for deleting a project webhook.
type DeleteHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID to delete,required"`
}

// DeleteHook deletes a project webhook.
func DeleteHook(ctx context.Context, client *gitlabclient.Client, input DeleteHookInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return errors.New("projectDeleteHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	_, err := client.GL().Projects.DeleteProjectHook(string(input.ProjectID), input.HookID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectDeleteHook", err, http.StatusNotFound,
			"hook already deleted or never existed \u2014 use gitlab_project_hook_list to verify hook_id")
	}
	return nil
}

// TriggerTestHookInput defines parameters for triggering a test webhook event.
type TriggerTestHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID to test,required"`
	Event     string               `json:"event" jsonschema:"Event type to trigger (push_events, issues_events, merge_requests_events, tag_push_events, note_events, job_events, pipeline_events, wiki_page_events, releases_events, emoji_events),required"`
}

// TriggerTestHookOutput holds the result of triggering a test webhook.
type TriggerTestHookOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// TriggerTestHook triggers a test event for a project webhook.
func TriggerTestHook(ctx context.Context, client *gitlabclient.Client, input TriggerTestHookInput) (TriggerTestHookOutput, error) {
	if err := ctx.Err(); err != nil {
		return TriggerTestHookOutput{}, err
	}
	if input.ProjectID == "" {
		return TriggerTestHookOutput{}, errors.New("projectTriggerTestHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return TriggerTestHookOutput{}, errors.New("projectTriggerTestHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	if input.Event == "" {
		return TriggerTestHookOutput{}, errors.New("projectTriggerTestHook: event is required. Valid events: push_events, tag_push_events, merge_requests_events, note_events, issues_events, job_events, pipeline_events, wiki_page_events, releases_events, emoji_events")
	}
	_, err := client.GL().Projects.TriggerTestProjectHook(string(input.ProjectID), input.HookID, gl.ProjectHookEvent(input.Event), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return TriggerTestHookOutput{}, toolutil.WrapErrWithHint("projectTriggerTestHook", err,
				"event must be one this hook subscribes to and the project must contain matching content (e.g. issues_events requires at least one issue) \u2014 use gitlab_project_hook_get to inspect enabled events")
		}
		return TriggerTestHookOutput{}, toolutil.WrapErrWithStatusHint("projectTriggerTestHook", err, http.StatusNotFound,
			"hook may have been deleted \u2014 use gitlab_project_hook_list to verify hook_id")
	}
	return TriggerTestHookOutput{Message: fmt.Sprintf("Test event '%s' triggered for hook %d", input.Event, input.HookID)}, nil
}

// boolIcon formats project boolean settings consistently in Markdown output.
func boolIcon(v bool) string {
	return toolutil.BoolEmoji(v)
}

// userNames extracts usernames from approval-rule user objects for compact
// Markdown rendering, skipping nil entries.
func userNames(users []*toolutil.BasicUserOutput) []string {
	if len(users) == 0 {
		return nil
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		names = append(names, u.Username)
	}
	return names
}

// groupNames extracts group names from approval-rule group objects for compact
// Markdown rendering, skipping nil entries.
func groupNames(groups []*ApprovalGroupOutput) []string {
	if len(groups) == 0 {
		return nil
	}
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		names = append(names, g.Name)
	}
	return names
}

// ---------------------------------------------------------------------------
// ListUserProjects — list projects owned by a user
// ---------------------------------------------------------------------------.

// ListUserProjectsInput defines parameters for listing projects owned by a specific user.
type ListUserProjectsInput struct {
	UserID toolutil.StringOrInt `json:"user_id" jsonschema:"User ID or username,required"`
	userProjectFilterInput
}

// ListUserProjects lists projects owned by the given user.
func ListUserProjects(ctx context.Context, client *gitlabclient.Client, input ListUserProjectsInput) (ListOutput, error) {
	return listUserScopedProjects(ctx, input.UserID, "projectListUserProjects", "projectListUserProjects: user_id is required. Use gitlab_get_user to find the user ID",
		"user not found - use gitlab_get_user to verify user_id (numeric ID or exact username)", input.toFilter(),
		client.GL().Projects.ListUserProjects)
}

func listUserScopedProjects(ctx context.Context, userID toolutil.StringOrInt, operation, missingUserMsg, notFoundHint string, filters userProjectFilter, list func(any, *gl.ListProjectsOptions, ...gl.RequestOptionFunc) ([]*gl.Project, *gl.Response, error)) (ListOutput, error) {
	if userID == "" {
		return ListOutput{}, errors.New(missingUserMsg)
	}
	projects, resp, err := list(string(userID), buildUserProjectOpts(filters), gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(operation, err, http.StatusNotFound, notFoundHint)
	}
	out := make([]Output, len(projects))
	for i, project := range projects {
		out[i] = ToOutput(project)
	}
	return ListOutput{Projects: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// ListProjectUsers — list users of a project
// ---------------------------------------------------------------------------.

// ProjectUserOutput represents a project user.
type ProjectUserOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// projectUserOutputFromGL maps project user output from gl between API and evaluator models.
func projectUserOutputFromGL(u *gl.ProjectUser) ProjectUserOutput {
	return ProjectUserOutput{
		ID:        u.ID,
		Name:      u.Name,
		Username:  u.Username,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// ListProjectUsersInput defines parameters for listing users of a project.
type ListProjectUsersInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search    string               `json:"search,omitempty"   jsonschema:"Search by name or username"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Keyset ordering column (for keyset pagination)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Keyset sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListProjectUsersOutput holds a paginated list of project users.
type ListProjectUsersOutput struct {
	toolutil.HintableOutput
	Users      []ProjectUserOutput       `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProjectUsers lists users who are members of the given project.
func ListProjectUsers(ctx context.Context, client *gitlabclient.Client, input ListProjectUsersInput) (ListProjectUsersOutput, error) {
	if input.ProjectID == "" {
		return ListProjectUsersOutput{}, errors.New("projectListUsers: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectUserOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyKeysetOrder(&opts.ListOptions, input.OrderBy, input.Sort)
	users, resp, err := client.GL().Projects.ListProjectsUsers(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectUsersOutput{}, toolutil.WrapErrWithStatusHint("projectListUsers", err, http.StatusNotFound,
			hintVerifyProjectExists)
	}
	out := make([]ProjectUserOutput, len(users))
	for i, u := range users {
		out[i] = projectUserOutputFromGL(u)
	}
	return ListProjectUsersOutput{
		Users:      out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListProjectGroups — list ancestor groups of a project
// ---------------------------------------------------------------------------.

// ProjectGroupOutput represents a group associated with a project.
type ProjectGroupOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	FullName  string `json:"full_name"`
	FullPath  string `json:"full_path"`
}

// projectGroupOutputFromGL maps project group output from gl between API and evaluator models.
func projectGroupOutputFromGL(g *gl.ProjectGroup) ProjectGroupOutput {
	return ProjectGroupOutput{
		ID:        g.ID,
		Name:      g.Name,
		AvatarURL: g.AvatarURL,
		WebURL:    g.WebURL,
		FullName:  g.FullName,
		FullPath:  g.FullPath,
	}
}

// ListProjectGroupsInput defines parameters for listing a project's ancestor groups.
type ListProjectGroupsInput struct {
	ProjectID            toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search               string               `json:"search,omitempty"              jsonschema:"Search by group name"`
	WithShared           *bool                `json:"with_shared,omitempty"         jsonschema:"Include shared groups (default true)"`
	SharedVisibleOnly    *bool                `json:"shared_visible_only,omitempty" jsonschema:"Only show shared groups visible to the current user"`
	SkipGroups           []int64              `json:"skip_groups,omitempty"         jsonschema:"Array of group IDs to exclude"`
	SharedMinAccessLevel int                  `json:"shared_min_access_level,omitempty" jsonschema:"Filter by minimum access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	OrderBy              string               `json:"order_by,omitempty" jsonschema:"Keyset ordering column (for keyset pagination)"`
	Sort                 string               `json:"sort,omitempty"     jsonschema:"Keyset sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListProjectGroupsOutput holds a paginated list of project groups.
type ListProjectGroupsOutput struct {
	toolutil.HintableOutput
	Groups     []ProjectGroupOutput      `json:"groups"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProjectGroups lists the ancestor groups of the given project.
func ListProjectGroups(ctx context.Context, client *gitlabclient.Client, input ListProjectGroupsInput) (ListProjectGroupsOutput, error) {
	if input.ProjectID == "" {
		return ListProjectGroupsOutput{}, errors.New("projectListGroups: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectGroupOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.WithShared != nil {
		opts.WithShared = input.WithShared
	}
	if input.SharedVisibleOnly != nil {
		opts.SharedVisibleOnly = input.SharedVisibleOnly
	}
	if len(input.SkipGroups) > 0 {
		opts.SkipGroups = new(input.SkipGroups)
	}
	if input.SharedMinAccessLevel > 0 {
		opts.SharedMinAccessLevel = new(gl.AccessLevelValue(input.SharedMinAccessLevel))
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyKeysetOrder(&opts.ListOptions, input.OrderBy, input.Sort)
	groups, resp, err := client.GL().Projects.ListProjectsGroups(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectGroupsOutput{}, toolutil.WrapErrWithStatusHint("projectListGroups", err, http.StatusNotFound,
			hintVerifyProjectExists)
	}
	out := make([]ProjectGroupOutput, len(groups))
	for i, g := range groups {
		out[i] = projectGroupOutputFromGL(g)
	}
	return ListProjectGroupsOutput{
		Groups:     out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListProjectStarrers — list users who starred a project
// ---------------------------------------------------------------------------.

// StarrerOutput represents a user who starred a project.
type StarrerOutput struct {
	StarredSince string            `json:"starred_since"`
	User         ProjectUserOutput `json:"user"`
}

// ListProjectStarrersInput defines parameters for listing project starrers.
type ListProjectStarrersInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search    string               `json:"search,omitempty"   jsonschema:"Search by name or username"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Keyset ordering column (for keyset pagination)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Keyset sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListProjectStarrersOutput holds a paginated list of project starrers.
type ListProjectStarrersOutput struct {
	toolutil.HintableOutput
	Starrers   []StarrerOutput           `json:"starrers"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProjectStarrers lists users who have starred the given project.
func ListProjectStarrers(ctx context.Context, client *gitlabclient.Client, input ListProjectStarrersInput) (ListProjectStarrersOutput, error) {
	if input.ProjectID == "" {
		return ListProjectStarrersOutput{}, errors.New("projectListStarrers: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectStarrersOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyKeysetOrder(&opts.ListOptions, input.OrderBy, input.Sort)
	starrers, resp, err := client.GL().Projects.ListProjectStarrers(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectStarrersOutput{}, toolutil.WrapErrWithStatusHint("projectListStarrers", err, http.StatusNotFound,
			hintVerifyProjectExists)
	}
	out := make([]StarrerOutput, len(starrers))
	for i, s := range starrers {
		out[i] = StarrerOutput{
			StarredSince: s.StarredSince.Format(time.RFC3339),
			User:         projectUserOutputFromGL(&s.User),
		}
	}
	return ListProjectStarrersOutput{
		Starrers:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ShareProjectWithGroup
// ---------------------------------------------------------------------------.

// ShareProjectInput defines parameters for sharing a project with a group.
type ShareProjectInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	GroupID     int64                `json:"group_id"     jsonschema:"Group ID to share with,required"`
	GroupAccess int                  `json:"group_access" jsonschema:"Access level for the group (10=Guest 20=Reporter 30=Developer 40=Maintainer); 5=Minimal access, 15=Planner, 25=Security Manager, 60=Admin are not valid for project shares,required"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the share (YYYY-MM-DD)"`
}

// ShareProjectOutput holds the result of sharing a project.
type ShareProjectOutput struct {
	toolutil.HintableOutput
	Message     string `json:"message"`
	GroupID     int64  `json:"group_id,omitempty"`
	GroupAccess int    `json:"group_access,omitempty"`
	AccessRole  string `json:"access_role,omitempty"`
}

// accessLevelName returns the human-readable name for a GitLab access level.
func accessLevelName(level int) string {
	names := map[int]string{
		10: "Guest",
		20: "Reporter",
		25: "Security Manager",
		30: "Developer",
		40: "Maintainer",
		50: "Owner",
	}
	if name, ok := names[level]; ok {
		return name
	}
	return fmt.Sprintf("Level %d", level)
}

// ShareProjectWithGroup shares a project with the given group.
func ShareProjectWithGroup(ctx context.Context, client *gitlabclient.Client, input ShareProjectInput) (ShareProjectOutput, error) {
	if input.ProjectID == "" {
		return ShareProjectOutput{}, errors.New("projectShareWithGroup: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.GroupID == 0 {
		return ShareProjectOutput{}, errors.New("projectShareWithGroup: group_id is required. Use gitlab_group_list to find the group ID")
	}
	if input.GroupAccess == 0 {
		return ShareProjectOutput{}, errors.New("projectShareWithGroup: group_access is required. Valid levels: 10 (Guest), 20 (Reporter), 30 (Developer), 40 (Maintainer) — 25 (Security Manager) is not valid for project shares")
	}
	opts := &gl.ShareWithGroupOptions{
		GroupID:     new(input.GroupID),
		GroupAccess: new(gl.AccessLevelValue(input.GroupAccess)),
	}
	if input.ExpiresAt != "" {
		opts.ExpiresAt = new(input.ExpiresAt)
	}
	_, err := client.GL().Projects.ShareProjectWithGroup(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return ShareProjectOutput{}, toolutil.WrapErrWithHint("projectShareWithGroup", err,
				"group_access must be 10/20/30/40 (Guest/Reporter/Developer/Maintainer); 25 (Security Manager) is not valid for project shares; expires_at must be YYYY-MM-DD; the project may already be shared with this group \u2014 use gitlab_project_list_groups to verify")
		}
		return ShareProjectOutput{}, toolutil.WrapErrWithStatusHint("projectShareWithGroup", err, http.StatusNotFound,
			"verify project_id and group_id with gitlab_project_get and gitlab_group_get")
	}
	roleName := accessLevelName(input.GroupAccess)
	return ShareProjectOutput{
		Message:     fmt.Sprintf("Project %s shared with group %d as %s", input.ProjectID, input.GroupID, roleName),
		GroupID:     input.GroupID,
		GroupAccess: input.GroupAccess,
		AccessRole:  roleName,
	}, nil
}

// ---------------------------------------------------------------------------
// DeleteSharedProjectFromGroup
// ---------------------------------------------------------------------------.

// DeleteSharedGroupInput defines parameters for removing a group share from a project.
type DeleteSharedGroupInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	GroupID   int64                `json:"group_id"   jsonschema:"Group ID to remove from project sharing,required"`
}

// DeleteSharedProjectFromGroup removes a shared group link from a project.
func DeleteSharedProjectFromGroup(ctx context.Context, client *gitlabclient.Client, input DeleteSharedGroupInput) error {
	if input.ProjectID == "" {
		return errors.New("projectDeleteSharedGroup: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.GroupID == 0 {
		return errors.New("projectDeleteSharedGroup: group_id is required. Use gitlab_project_list_groups to find shared group IDs")
	}
	_, err := client.GL().Projects.DeleteSharedProjectFromGroup(string(input.ProjectID), input.GroupID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectDeleteSharedGroup", err, http.StatusNotFound,
			"group is not currently shared with this project \u2014 use gitlab_project_list_groups to verify")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ListInvitedGroups — list groups invited to a project
// ---------------------------------------------------------------------------.

// ListInvitedGroupsInput defines parameters for listing groups invited to a project.
type ListInvitedGroupsInput struct {
	ProjectID            toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search               string               `json:"search,omitempty"          jsonschema:"Search by group name"`
	MinAccessLevel       int                  `json:"min_access_level,omitempty" jsonschema:"Filter by minimum access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	Relation             []string             `json:"relation,omitempty"            jsonschema:"Filter by group relation to the project (e.g. direct, inherited)"`
	WithCustomAttributes *bool                `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response (admin only)"`
	OrderBy              string               `json:"order_by,omitempty"            jsonschema:"Keyset ordering column (for keyset pagination)"`
	Sort                 string               `json:"sort,omitempty"                jsonschema:"Keyset sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListInvitedGroups lists groups that have been invited to the given project.
func ListInvitedGroups(ctx context.Context, client *gitlabclient.Client, input ListInvitedGroupsInput) (ListProjectGroupsOutput, error) {
	if input.ProjectID == "" {
		return ListProjectGroupsOutput{}, errors.New("projectListInvitedGroups: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectInvitedGroupOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if len(input.Relation) > 0 {
		opts.Relation = &input.Relation
	}
	if input.WithCustomAttributes != nil {
		opts.WithCustomAttributes = input.WithCustomAttributes
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyKeysetOrder(&opts.ListOptions, input.OrderBy, input.Sort)
	groups, resp, err := client.GL().Projects.ListProjectsInvitedGroups(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectGroupsOutput{}, toolutil.WrapErrWithStatusHint("projectListInvitedGroups", err, http.StatusNotFound,
			hintVerifyProjectExists)
	}
	out := make([]ProjectGroupOutput, len(groups))
	for i, g := range groups {
		out[i] = projectGroupOutputFromGL(g)
	}
	return ListProjectGroupsOutput{
		Groups:     out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListUserContributedProjects — list projects a user has contributed to
// ---------------------------------------------------------------------------.

// ListUserContributedProjectsInput defines parameters for listing contributed projects.
type ListUserContributedProjectsInput struct {
	UserID toolutil.StringOrInt `json:"user_id" jsonschema:"User ID or username,required"`
	userProjectFilterInput
}

// ListUserContributedProjects lists projects a specific user has contributed to.
func ListUserContributedProjects(ctx context.Context, client *gitlabclient.Client, input ListUserContributedProjectsInput) (ListOutput, error) {
	return listUserScopedProjects(ctx, input.UserID, "projectListUserContributed", "projectListUserContributed: user_id is required. Use gitlab_get_user to find the user ID",
		"user not found - use gitlab_get_user to verify user_id", input.toFilter(),
		client.GL().Projects.ListUserContributedProjects)
}

// ---------------------------------------------------------------------------
// ListUserStarredProjects — list projects a user has starred
// ---------------------------------------------------------------------------.

// ListUserStarredProjectsInput defines parameters for listing starred projects.
type ListUserStarredProjectsInput struct {
	UserID toolutil.StringOrInt `json:"user_id" jsonschema:"User ID or username,required"`
	userProjectFilterInput
}

// ListUserStarredProjects lists projects starred by a specific user.
func ListUserStarredProjects(ctx context.Context, client *gitlabclient.Client, input ListUserStarredProjectsInput) (ListOutput, error) {
	return listUserScopedProjects(ctx, input.UserID, "projectListUserStarred", "projectListUserStarred: user_id is required. Use gitlab_get_user to find the user ID",
		"user not found - use gitlab_get_user to verify user_id", input.toFilter(),
		client.GL().Projects.ListUserStarredProjects)
}

// userProjectFilterInput is the embeddable, JSON-schema-bearing filter set
// shared by the user-scoped project list tools (owned, contributed, starred).
// All three endpoints accept the full ListProjectsOptions filter set, so they
// surface the complete filter surface for 1:1 SDK parity.
type userProjectFilterInput struct {
	Search                   string            `json:"search,omitempty"             jsonschema:"Search query for project name"`
	Visibility               string            `json:"visibility,omitempty"         jsonschema:"Filter by visibility (private, internal, public)"`
	Archived                 *bool             `json:"archived,omitempty"           jsonschema:"Filter by archived status (true=only archived, false=only active)"`
	OrderBy                  string            `json:"order_by,omitempty"           jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort                     string            `json:"sort,omitempty"               jsonschema:"Sort direction (asc, desc)"`
	Simple                   bool              `json:"simple,omitempty"             jsonschema:"Return only limited fields (faster)"`
	Owned                    bool              `json:"owned,omitempty"              jsonschema:"Limit to projects explicitly owned by the current user"`
	Topic                    string            `json:"topic,omitempty"              jsonschema:"Filter by topic name"`
	MinAccessLevel           int               `json:"min_access_level,omitempty"   jsonschema:"Filter by minimum access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	Starred                  *bool             `json:"starred,omitempty"            jsonschema:"Limit to projects starred by the current user"`
	Membership               *bool             `json:"membership,omitempty"         jsonschema:"Limit to projects where the current user is a member"`
	WithIssuesEnabled        *bool             `json:"with_issues_enabled,omitempty"         jsonschema:"Filter by projects with issues feature enabled"`
	WithMergeRequestsEnabled *bool             `json:"with_merge_requests_enabled,omitempty" jsonschema:"Filter by projects with merge requests enabled"`
	SearchNamespaces         *bool             `json:"search_namespaces,omitempty"  jsonschema:"Include namespace in search"`
	Statistics               *bool             `json:"statistics,omitempty"         jsonschema:"Include project statistics in response"`
	WithProgrammingLanguage  string            `json:"with_programming_language,omitempty"   jsonschema:"Filter by programming language name"`
	LastActivityAfter        string            `json:"last_activity_after,omitempty"         jsonschema:"Return projects with last activity after date (ISO 8601 format)"`
	LastActivityBefore       string            `json:"last_activity_before,omitempty"        jsonschema:"Return projects with last activity before date (ISO 8601 format)"`
	IDAfter                  int64             `json:"id_after,omitempty"           jsonschema:"Return projects with ID greater than this value (keyset pagination)"`
	IDBefore                 int64             `json:"id_before,omitempty"          jsonschema:"Return projects with ID less than this value (keyset pagination)"`
	IncludePendingDelete     *bool             `json:"include_pending_delete,omitempty"      jsonschema:"Include projects marked/scheduled for deletion. Default false."`
	IncludeHidden            *bool             `json:"include_hidden,omitempty"     jsonschema:"Include hidden projects in results"`
	Active                   *bool             `json:"active,omitempty"             jsonschema:"Limit to active (non-archived, non-pending-deletion) projects"`
	Imported                 *bool             `json:"imported,omitempty"           jsonschema:"Limit to projects imported from an external source by the current user"`
	RepositoryChecksumFailed *bool             `json:"repository_checksum_failed,omitempty"  jsonschema:"Limit to projects where the repository checksum calculation failed (admin only)"`
	WikiChecksumFailed       *bool             `json:"wiki_checksum_failed,omitempty"        jsonschema:"Limit to projects where the wiki checksum calculation failed (admin only)"`
	RepositoryStorage        string            `json:"repository_storage,omitempty"          jsonschema:"Limit to projects on the given repository storage shard (admin only)"`
	WithCustomAttributes     *bool             `json:"with_custom_attributes,omitempty"      jsonschema:"Include custom attributes in the response (admin only)"`
	CustomAttributes         map[string]string `json:"custom_attributes,omitempty"     jsonschema:"Filter projects by custom attribute key/value pairs (admin only). Different from with_custom_attributes, which only controls whether attributes are returned"`

	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// toFilter converts the embeddable input into the internal userProjectFilter
// consumed by buildUserProjectOpts. The two structs are field-identical (same
// names, types, and json tags) so a direct conversion suffices.
func (f userProjectFilterInput) toFilter() userProjectFilter {
	return userProjectFilter(f)
}

// userProjectFilter holds the resolved filter parameters for user-scoped
// project listings, mirroring the full ListProjectsOptions filter set.
//
// The json tags mirror the SDK url tags on gl.ListProjectsOptions so the
// struct-completeness auditor (R-INPUT) can confirm 1:1 parity against the SDK
// options this struct is translated into by buildUserProjectOpts. This struct is
// internal plumbing — the model-facing schema is carried by userProjectFilterInput
// (and ListInput / ListForksInput) — so the tags are documentation/audit metadata
// only and never affect serialization.
type userProjectFilter struct {
	Search                   string            `json:"search,omitempty"`
	Visibility               string            `json:"visibility,omitempty"`
	Archived                 *bool             `json:"archived,omitempty"`
	OrderBy                  string            `json:"order_by,omitempty"`
	Sort                     string            `json:"sort,omitempty"`
	Simple                   bool              `json:"simple,omitempty"`
	Owned                    bool              `json:"owned,omitempty"`
	Topic                    string            `json:"topic,omitempty"`
	MinAccessLevel           int               `json:"min_access_level,omitempty"`
	Starred                  *bool             `json:"starred,omitempty"`
	Membership               *bool             `json:"membership,omitempty"`
	WithIssuesEnabled        *bool             `json:"with_issues_enabled,omitempty"`
	WithMergeRequestsEnabled *bool             `json:"with_merge_requests_enabled,omitempty"`
	SearchNamespaces         *bool             `json:"search_namespaces,omitempty"`
	Statistics               *bool             `json:"statistics,omitempty"`
	WithProgrammingLanguage  string            `json:"with_programming_language,omitempty"`
	LastActivityAfter        string            `json:"last_activity_after,omitempty"`
	LastActivityBefore       string            `json:"last_activity_before,omitempty"`
	IDAfter                  int64             `json:"id_after,omitempty"`
	IDBefore                 int64             `json:"id_before,omitempty"`
	IncludePendingDelete     *bool             `json:"include_pending_delete,omitempty"`
	IncludeHidden            *bool             `json:"include_hidden,omitempty"`
	Active                   *bool             `json:"active,omitempty"`
	Imported                 *bool             `json:"imported,omitempty"`
	RepositoryChecksumFailed *bool             `json:"repository_checksum_failed,omitempty"`
	WikiChecksumFailed       *bool             `json:"wiki_checksum_failed,omitempty"`
	RepositoryStorage        string            `json:"repository_storage,omitempty"`
	WithCustomAttributes     *bool             `json:"with_custom_attributes,omitempty"`
	CustomAttributes         map[string]string `json:"custom_attributes,omitempty"`

	// Pagination and Keyset are embedded (not named) so their page/per_page/
	// page_token/pagination json tags are promoted, matching the embedded
	// ListOptions on gl.ListProjectsOptions for the R-INPUT tag diff.
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// buildUserProjectOpts centralizes option building for user-scoped project listings.
func buildUserProjectOpts(f userProjectFilter) *gl.ListProjectsOptions {
	opts := &gl.ListProjectsOptions{}
	if f.Search != "" {
		opts.Search = new(f.Search)
	}
	if f.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(f.Visibility))
	}
	if f.OrderBy != "" {
		opts.OrderBy = new(f.OrderBy)
	}
	if f.Sort != "" {
		opts.Sort = new(f.Sort)
	}
	if f.Simple {
		opts.Simple = new(true)
	}
	if f.Owned {
		opts.Owned = new(true)
	}
	if f.Topic != "" {
		opts.Topic = new(f.Topic)
	}
	if f.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(f.MinAccessLevel))
	}
	if f.WithProgrammingLanguage != "" {
		opts.WithProgrammingLanguage = new(f.WithProgrammingLanguage)
	}
	opts.LastActivityAfter = toolutil.ParseOptionalTime(f.LastActivityAfter)
	opts.LastActivityBefore = toolutil.ParseOptionalTime(f.LastActivityBefore)
	applyUserProjectFilterPtrs(opts, f)
	toolutil.ApplyListOptions(&opts.ListOptions, f.PaginationInput, f.KeysetPaginationInput)
	return opts
}

// applyUserProjectFilterPtrs sets the optional boolean-pointer and keyset-bound
// filters shared with the project list endpoint.
func applyUserProjectFilterPtrs(opts *gl.ListProjectsOptions, f userProjectFilter) {
	if f.Archived != nil {
		opts.Archived = f.Archived
	}
	if f.Starred != nil {
		opts.Starred = f.Starred
	}
	if f.Membership != nil {
		opts.Membership = f.Membership
	}
	if f.WithIssuesEnabled != nil {
		opts.WithIssuesEnabled = f.WithIssuesEnabled
	}
	if f.WithMergeRequestsEnabled != nil {
		opts.WithMergeRequestsEnabled = f.WithMergeRequestsEnabled
	}
	if f.SearchNamespaces != nil {
		opts.SearchNamespaces = f.SearchNamespaces
	}
	if f.Statistics != nil {
		opts.Statistics = f.Statistics
	}
	if f.IncludePendingDelete != nil {
		opts.IncludePendingDelete = f.IncludePendingDelete
	}
	if f.IncludeHidden != nil {
		opts.IncludeHidden = f.IncludeHidden
	}
	if f.IDAfter > 0 {
		opts.IDAfter = new(f.IDAfter)
	}
	if f.IDBefore > 0 {
		opts.IDBefore = new(f.IDBefore)
	}
	if f.Active != nil {
		opts.Active = f.Active
	}
	if f.Imported != nil {
		opts.Imported = f.Imported
	}
	if f.RepositoryChecksumFailed != nil {
		opts.RepositoryChecksumFailed = f.RepositoryChecksumFailed
	}
	if f.WikiChecksumFailed != nil {
		opts.WikiChecksumFailed = f.WikiChecksumFailed
	}
	if f.RepositoryStorage != "" {
		opts.RepositoryStorage = new(f.RepositoryStorage)
	}
	if f.WithCustomAttributes != nil {
		opts.WithCustomAttributes = f.WithCustomAttributes
	}
	if len(f.CustomAttributes) > 0 {
		opts.CustomAttributes = gl.CustomAttributesFilter(f.CustomAttributes)
	}
}

// ---------------------------------------------------------------------------
// Push Rules — get, add, edit, delete project push rule configuration
// ---------------------------------------------------------------------------.

// PushRuleOutput represents a project's push rule configuration.
type PushRuleOutput struct {
	toolutil.HintableOutput
	ID                         int64  `json:"id"`
	ProjectID                  int64  `json:"project_id"`
	CommitMessageRegex         string `json:"commit_message_regex,omitempty"`
	CommitMessageNegativeRegex string `json:"commit_message_negative_regex,omitempty"`
	BranchNameRegex            string `json:"branch_name_regex,omitempty"`
	DenyDeleteTag              bool   `json:"deny_delete_tag"`
	MemberCheck                bool   `json:"member_check"`
	PreventSecrets             bool   `json:"prevent_secrets"`
	AuthorEmailRegex           string `json:"author_email_regex,omitempty"`
	FileNameRegex              string `json:"file_name_regex,omitempty"`
	MaxFileSize                int64  `json:"max_file_size"`
	CommitCommitterCheck       bool   `json:"commit_committer_check"`
	CommitCommitterNameCheck   bool   `json:"commit_committer_name_check"`
	RejectUnsignedCommits      bool   `json:"reject_unsigned_commits"`
	RejectNonDCOCommits        bool   `json:"reject_non_dco_commits"`
	CreatedAt                  string `json:"created_at,omitempty"`
}

// pushRuleOutputFromGL maps push rule output from gl between API and evaluator models.
func pushRuleOutputFromGL(r *gl.ProjectPushRules) PushRuleOutput {
	out := PushRuleOutput{
		ID:                         r.ID,
		ProjectID:                  r.ProjectID,
		CommitMessageRegex:         r.CommitMessageRegex,
		CommitMessageNegativeRegex: r.CommitMessageNegativeRegex,
		BranchNameRegex:            r.BranchNameRegex,
		DenyDeleteTag:              r.DenyDeleteTag,
		MemberCheck:                r.MemberCheck,
		PreventSecrets:             r.PreventSecrets,
		AuthorEmailRegex:           r.AuthorEmailRegex,
		FileNameRegex:              r.FileNameRegex,
		MaxFileSize:                r.MaxFileSize,
		CommitCommitterCheck:       r.CommitCommitterCheck,
		CommitCommitterNameCheck:   r.CommitCommitterNameCheck,
		RejectUnsignedCommits:      r.RejectUnsignedCommits,
		RejectNonDCOCommits:        r.RejectNonDCOCommits,
	}
	if r.CreatedAt != nil {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// GetPushRulesInput defines parameters for getting project push rules.
type GetPushRulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// GetPushRules retrieves the push rule configuration for a project.
func GetPushRules(ctx context.Context, client *gitlabclient.Client, input GetPushRulesInput) (PushRuleOutput, error) {
	if input.ProjectID == "" {
		return PushRuleOutput{}, errors.New("projectGetPushRules: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	rule, _, err := client.GL().Projects.GetProjectPushRules(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return PushRuleOutput{}, toolutil.WrapErrWithHint("projectGetPushRules", err,
				"no push rules configured on this project, or the feature requires GitLab Premium/Ultimate \u2014 use gitlab_project_add_push_rule to create one")
		}
		return PushRuleOutput{}, toolutil.WrapErrWithStatusHint("projectGetPushRules", err, http.StatusForbidden,
			"reading push rules requires Premium/Ultimate licensing and at least Maintainer role on the project")
	}
	return pushRuleOutputFromGL(rule), nil
}

// AddPushRuleInput defines parameters for adding push rules to a project.
type AddPushRuleInput struct {
	ProjectID                  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AuthorEmailRegex           string               `json:"author_email_regex,omitempty" jsonschema:"Regex to validate author email addresses"`
	BranchNameRegex            string               `json:"branch_name_regex,omitempty" jsonschema:"Regex to validate branch names"`
	CommitCommitterCheck       *bool                `json:"commit_committer_check,omitempty" jsonschema:"Reject commits where committer is not a project member"`
	CommitCommitterNameCheck   *bool                `json:"commit_committer_name_check,omitempty" jsonschema:"Reject commits where committer name does not match user name"`
	CommitMessageNegativeRegex string               `json:"commit_message_negative_regex,omitempty" jsonschema:"Regex that commit messages must NOT match"`
	CommitMessageRegex         string               `json:"commit_message_regex,omitempty" jsonschema:"Regex that commit messages must match"`
	DenyDeleteTag              *bool                `json:"deny_delete_tag,omitempty" jsonschema:"Deny tag deletion"`
	FileNameRegex              string               `json:"file_name_regex,omitempty" jsonschema:"Regex for disallowed file names"`
	MaxFileSize                *int64               `json:"max_file_size,omitempty" jsonschema:"Maximum file size (MB). 0 means unlimited"`
	MemberCheck                *bool                `json:"member_check,omitempty" jsonschema:"Only allow commits from project members"`
	PreventSecrets             *bool                `json:"prevent_secrets,omitempty" jsonschema:"Reject files that are likely to contain secrets"`
	RejectUnsignedCommits      *bool                `json:"reject_unsigned_commits,omitempty" jsonschema:"Reject commits that are not GPG signed"`
	RejectNonDCOCommits        *bool                `json:"reject_non_dco_commits,omitempty" jsonschema:"Reject commits without DCO certification"`
}

// AddPushRule adds push rule configuration to a project.
func AddPushRule(ctx context.Context, client *gitlabclient.Client, input AddPushRuleInput) (PushRuleOutput, error) {
	if input.ProjectID == "" {
		return PushRuleOutput{}, errors.New("projectAddPushRule: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if !hasAddPushRuleSetting(input) {
		return PushRuleOutput{}, errors.New("projectAddPushRule: at least one push rule setting is required. Include params.commit_message_regex for commit message regex tasks, or another setting such as reject_unsigned_commits, prevent_secrets, branch_name_regex, deny_delete_tag, member_check, or max_file_size")
	}
	opts := &gl.AddProjectPushRuleOptions{}
	if input.AuthorEmailRegex != "" {
		opts.AuthorEmailRegex = new(input.AuthorEmailRegex)
	}
	if input.BranchNameRegex != "" {
		opts.BranchNameRegex = new(input.BranchNameRegex)
	}
	if input.CommitCommitterCheck != nil {
		opts.CommitCommitterCheck = input.CommitCommitterCheck
	}
	if input.CommitCommitterNameCheck != nil {
		opts.CommitCommitterNameCheck = input.CommitCommitterNameCheck
	}
	if input.CommitMessageNegativeRegex != "" {
		opts.CommitMessageNegativeRegex = new(input.CommitMessageNegativeRegex)
	}
	if input.CommitMessageRegex != "" {
		opts.CommitMessageRegex = new(input.CommitMessageRegex)
	}
	if input.DenyDeleteTag != nil {
		opts.DenyDeleteTag = input.DenyDeleteTag
	}
	if input.FileNameRegex != "" {
		opts.FileNameRegex = new(input.FileNameRegex)
	}
	if input.MaxFileSize != nil {
		opts.MaxFileSize = input.MaxFileSize
	}
	if input.MemberCheck != nil {
		opts.MemberCheck = input.MemberCheck
	}
	if input.PreventSecrets != nil {
		opts.PreventSecrets = input.PreventSecrets
	}
	if input.RejectUnsignedCommits != nil {
		opts.RejectUnsignedCommits = input.RejectUnsignedCommits
	}
	if input.RejectNonDCOCommits != nil {
		opts.RejectNonDCOCommits = input.RejectNonDCOCommits
	}
	rule, _, err := client.GL().Projects.AddProjectPushRule(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return PushRuleOutput{}, toolutil.WrapErrWithHint("projectAddPushRule", err,
				"push rules already exist on this project (use gitlab_project_edit_push_rule to update), one of the regex patterns is invalid, or the feature requires GitLab Premium/Ultimate")
		}
		return PushRuleOutput{}, toolutil.WrapErrWithStatusHint("projectAddPushRule", err, http.StatusForbidden,
			"adding push rules requires Maintainer/Owner role and Premium/Ultimate licensing")
	}
	return pushRuleOutputFromGL(rule), nil
}

func hasAddPushRuleSetting(input AddPushRuleInput) bool {
	return input.AuthorEmailRegex != "" ||
		input.BranchNameRegex != "" ||
		input.CommitCommitterCheck != nil ||
		input.CommitCommitterNameCheck != nil ||
		input.CommitMessageNegativeRegex != "" ||
		input.CommitMessageRegex != "" ||
		input.DenyDeleteTag != nil ||
		input.FileNameRegex != "" ||
		input.MaxFileSize != nil ||
		input.MemberCheck != nil ||
		input.PreventSecrets != nil ||
		input.RejectUnsignedCommits != nil ||
		input.RejectNonDCOCommits != nil
}

// EditPushRuleInput defines parameters for editing push rules on a project.
type EditPushRuleInput struct {
	ProjectID                  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AuthorEmailRegex           *string              `json:"author_email_regex,omitempty" jsonschema:"Regex to validate author email addresses"`
	BranchNameRegex            *string              `json:"branch_name_regex,omitempty" jsonschema:"Regex to validate branch names"`
	CommitCommitterCheck       *bool                `json:"commit_committer_check,omitempty" jsonschema:"Reject commits where committer is not a project member"`
	CommitCommitterNameCheck   *bool                `json:"commit_committer_name_check,omitempty" jsonschema:"Reject commits where committer name does not match user name"`
	CommitMessageNegativeRegex *string              `json:"commit_message_negative_regex,omitempty" jsonschema:"Regex that commit messages must NOT match"`
	CommitMessageRegex         *string              `json:"commit_message_regex,omitempty" jsonschema:"Regex that commit messages must match"`
	DenyDeleteTag              *bool                `json:"deny_delete_tag,omitempty" jsonschema:"Deny tag deletion"`
	FileNameRegex              *string              `json:"file_name_regex,omitempty" jsonschema:"Regex for disallowed file names"`
	MaxFileSize                *int64               `json:"max_file_size,omitempty" jsonschema:"Maximum file size (MB). 0 means unlimited"`
	MemberCheck                *bool                `json:"member_check,omitempty" jsonschema:"Only allow commits from project members"`
	PreventSecrets             *bool                `json:"prevent_secrets,omitempty" jsonschema:"Reject files that are likely to contain secrets"`
	RejectUnsignedCommits      *bool                `json:"reject_unsigned_commits,omitempty" jsonschema:"Reject commits that are not GPG signed"`
	RejectNonDCOCommits        *bool                `json:"reject_non_dco_commits,omitempty" jsonschema:"Reject commits without DCO certification"`
}

// EditPushRule modifies the push rule configuration for a project.
func EditPushRule(ctx context.Context, client *gitlabclient.Client, input EditPushRuleInput) (PushRuleOutput, error) {
	if input.ProjectID == "" {
		return PushRuleOutput{}, errors.New("projectEditPushRule: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.EditProjectPushRuleOptions{}
	if input.AuthorEmailRegex != nil {
		opts.AuthorEmailRegex = input.AuthorEmailRegex
	}
	if input.BranchNameRegex != nil {
		opts.BranchNameRegex = input.BranchNameRegex
	}
	if input.CommitCommitterCheck != nil {
		opts.CommitCommitterCheck = input.CommitCommitterCheck
	}
	if input.CommitCommitterNameCheck != nil {
		opts.CommitCommitterNameCheck = input.CommitCommitterNameCheck
	}
	if input.CommitMessageNegativeRegex != nil {
		opts.CommitMessageNegativeRegex = input.CommitMessageNegativeRegex
	}
	if input.CommitMessageRegex != nil {
		opts.CommitMessageRegex = input.CommitMessageRegex
	}
	if input.DenyDeleteTag != nil {
		opts.DenyDeleteTag = input.DenyDeleteTag
	}
	if input.FileNameRegex != nil {
		opts.FileNameRegex = input.FileNameRegex
	}
	if input.MaxFileSize != nil {
		opts.MaxFileSize = input.MaxFileSize
	}
	if input.MemberCheck != nil {
		opts.MemberCheck = input.MemberCheck
	}
	if input.PreventSecrets != nil {
		opts.PreventSecrets = input.PreventSecrets
	}
	if input.RejectUnsignedCommits != nil {
		opts.RejectUnsignedCommits = input.RejectUnsignedCommits
	}
	if input.RejectNonDCOCommits != nil {
		opts.RejectNonDCOCommits = input.RejectNonDCOCommits
	}
	rule, _, err := client.GL().Projects.EditProjectPushRule(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return PushRuleOutput{}, toolutil.WrapErrWithHint("projectEditPushRule", err,
				"one of the regex patterns is invalid (use a Go-compatible regex syntax), or the field requires Premium/Ultimate")
		}
		return PushRuleOutput{}, toolutil.WrapErrWithStatusHint("projectEditPushRule", err, http.StatusNotFound,
			"no push rules currently exist on this project \u2014 use gitlab_project_add_push_rule first")
	}
	return pushRuleOutputFromGL(rule), nil
}

// DeletePushRuleInput defines parameters for deleting push rules from a project.
type DeletePushRuleInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DeletePushRule deletes the push rule configuration from a project.
func DeletePushRule(ctx context.Context, client *gitlabclient.Client, input DeletePushRuleInput) error {
	if input.ProjectID == "" {
		return errors.New("projectDeletePushRule: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	_, err := client.GL().Projects.DeleteProjectPushRule(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectDeletePushRule", err, http.StatusNotFound,
			"no push rules currently configured on this project \u2014 nothing to delete")
	}
	return nil
}

// SetCustomHeaderInput defines parameters for setting a custom header on a webhook.
type SetCustomHeaderInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"Custom header key name,required"`
	Value     string               `json:"value" jsonschema:"Custom header value,required"`
}

// SetCustomHeader sets a custom header on a project webhook.
func SetCustomHeader(ctx context.Context, client *gitlabclient.Client, input SetCustomHeaderInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectSetCustomHeader: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectSetCustomHeader: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectSetCustomHeader: key is required")
	}
	opts := &gl.SetHookCustomHeaderOptions{
		Value: &input.Value,
	}
	_, err := client.GL().Projects.SetProjectCustomHeader(string(input.ProjectID), input.HookID, input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectSetCustomHeader", err, http.StatusNotFound,
			"webhook not found \u2014 use gitlab_project_hook_list to verify hook_id")
	}
	return nil
}

// DeleteCustomHeaderInput defines parameters for deleting a custom header from a webhook.
type DeleteCustomHeaderInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"Custom header key name to delete,required"`
}

// DeleteCustomHeader deletes a custom header from a project webhook.
func DeleteCustomHeader(ctx context.Context, client *gitlabclient.Client, input DeleteCustomHeaderInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteCustomHeader: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectDeleteCustomHeader: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectDeleteCustomHeader: key is required")
	}
	_, err := client.GL().Projects.DeleteProjectCustomHeader(string(input.ProjectID), input.HookID, input.Key, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectDeleteCustomHeader", err, http.StatusNotFound,
			"header key not currently set on this hook (or hook not found) \u2014 use gitlab_project_hook_get to inspect configured custom headers")
	}
	return nil
}

// SetWebhookURLVariableInput defines parameters for setting a URL variable on a webhook.
type SetWebhookURLVariableInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"URL variable key name — letters and underscores only; GitLab rejects keys containing digits,required"`
	Value     string               `json:"value" jsonschema:"URL variable value — must be non-empty,required"`
}

// SetWebhookURLVariable sets a URL variable on a project webhook.
func SetWebhookURLVariable(ctx context.Context, client *gitlabclient.Client, input SetWebhookURLVariableInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectSetWebhookURLVariable: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectSetWebhookURLVariable: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectSetWebhookURLVariable: key is required")
	}
	opts := &gl.SetProjectWebhookURLVariableOptions{
		Value: &input.Value,
	}
	_, err := client.GL().Projects.SetProjectWebhookURLVariable(string(input.ProjectID), input.HookID, input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return toolutil.WrapErrWithHint("projectSetWebhookURLVariable", err,
				"URL variable keys accept only letters and underscores (digits are rejected) and the value must be non-empty")
		}
		return toolutil.WrapErrWithStatusHint("projectSetWebhookURLVariable", err, http.StatusNotFound,
			"webhook not found \u2014 use gitlab_project_hook_list to verify hook_id")
	}
	return nil
}

// DeleteWebhookURLVariableInput defines parameters for deleting a URL variable from a webhook.
type DeleteWebhookURLVariableInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"URL variable key name to delete,required"`
}

// DeleteWebhookURLVariable deletes a URL variable from a project webhook.
func DeleteWebhookURLVariable(ctx context.Context, client *gitlabclient.Client, input DeleteWebhookURLVariableInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteWebhookURLVariable: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectDeleteWebhookURLVariable: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectDeleteWebhookURLVariable: key is required")
	}
	_, err := client.GL().Projects.DeleteProjectWebhookURLVariable(string(input.ProjectID), input.HookID, input.Key, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectDeleteWebhookURLVariable", err, http.StatusNotFound,
			"variable key not currently set on this hook (or hook not found) \u2014 use gitlab_project_hook_get to inspect configured URL variables")
	}
	return nil
}

// CreateForkRelationInput defines parameters for creating a fork relation.
type CreateForkRelationInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path of the forked project,required"`
	ForkedFromID int64                `json:"forked_from_id" jsonschema:"ID of the project to set as the fork source,required"`
}

// ForkRelationOutput holds the result of a fork relation operation.
type ForkRelationOutput struct {
	toolutil.HintableOutput
	ID                  int64  `json:"id"`
	ForkedToProjectID   int64  `json:"forked_to_project_id"`
	ForkedFromProjectID int64  `json:"forked_from_project_id"`
	CreatedAt           string `json:"created_at,omitempty"`
	UpdatedAt           string `json:"updated_at,omitempty"`
}

// forkRelationToOutput converts the GitLab API response to the tool output format.
func forkRelationToOutput(r *gl.ProjectForkRelation) ForkRelationOutput {
	out := ForkRelationOutput{
		ID:                  r.ID,
		ForkedToProjectID:   r.ForkedToProjectID,
		ForkedFromProjectID: r.ForkedFromProjectID,
	}
	if r.CreatedAt != nil {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if r.UpdatedAt != nil {
		out.UpdatedAt = r.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// CreateForkRelation creates a fork relation between two projects.
func CreateForkRelation(ctx context.Context, client *gitlabclient.Client, input CreateForkRelationInput) (ForkRelationOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForkRelationOutput{}, err
	}
	if input.ProjectID == "" {
		return ForkRelationOutput{}, errors.New("projectCreateForkRelation: project_id is required")
	}
	if input.ForkedFromID == 0 {
		return ForkRelationOutput{}, errors.New("projectCreateForkRelation: forked_from_id is required")
	}
	rel, _, err := client.GL().Projects.CreateProjectForkRelation(string(input.ProjectID), input.ForkedFromID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return ForkRelationOutput{}, toolutil.WrapErrWithHint("projectCreateForkRelation", err,
				"a fork relation already exists for this project \u2014 use gitlab_project_get to inspect forked_from_project, or call gitlab_project_delete_fork_relation first")
		}
		return ForkRelationOutput{}, toolutil.WrapErrWithStatusHint("projectCreateForkRelation", err, http.StatusNotFound,
			"verify both project_id and forked_from_id reference existing projects with gitlab_project_get")
	}
	out := forkRelationToOutput(rel)
	// GitLab 19 responds with the full project body instead of the relation
	// object, so the SDK decodes both relation IDs to zero even on success.
	if out.ForkedFromProjectID == 0 && out.ForkedToProjectID == 0 {
		out.NextSteps = []string{
			"The fork relation was created even though GitLab returned no relation IDs (GitLab 19 responds with the project body); confirm with gitlab_project_get and check forked_from_project",
		}
	}
	return out, nil
}

// DeleteForkRelationInput defines parameters for deleting a fork relation.
type DeleteForkRelationInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DeleteForkRelation removes the fork relationship from a project.
func DeleteForkRelation(ctx context.Context, client *gitlabclient.Client, input DeleteForkRelationInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteForkRelation: project_id is required")
	}
	_, err := client.GL().Projects.DeleteProjectForkRelation(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectDeleteForkRelation", err, http.StatusNotFound,
			"project is not currently a fork (no fork relation to remove) \u2014 use gitlab_project_get to inspect forked_from_project")
	}
	return nil
}

// UploadAvatarInput defines parameters for uploading a project avatar.
// Exactly one of FilePath or ContentBase64 must be provided.
type UploadAvatarInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Filename      string               `json:"filename" jsonschema:"Avatar filename (e.g. avatar.png),required"`
	FilePath      string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local image file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded image content. Only one of file_path or content_base64 should be provided."`
}

// UploadAvatar uploads or replaces the avatar for a project.
// Accepts either file_path (local file) or content_base64 (base64-encoded string).
func UploadAvatar(ctx context.Context, client *gitlabclient.Client, input UploadAvatarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUploadAvatar: project_id is required")
	}
	if input.Filename == "" {
		return Output{}, errors.New("projectUploadAvatar: filename is required")
	}

	reader, _, cleanup, err := toolutil.OpenFileOrBase64Source("projectUploadAvatar", input.FilePath, input.ContentBase64)
	if err != nil {
		return Output{}, err
	}
	defer cleanup()

	p, _, err := client.GL().Projects.UploadAvatar(string(input.ProjectID), reader, input.Filename, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("projectUploadAvatar", err,
				"avatar must be JPG/PNG/GIF and under 200 KB; verify filename has a valid image extension")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("projectUploadAvatar", err, http.StatusForbidden,
			"updating the project avatar requires Maintainer/Owner role")
	}
	return ToOutput(p), nil
}

// DownloadAvatarInput defines parameters for downloading a project avatar.
type DownloadAvatarInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DownloadAvatarOutput holds the result of downloading a project avatar.
type DownloadAvatarOutput struct {
	toolutil.HintableOutput
	ContentBase64 string `json:"content_base64"`
	SizeBytes     int    `json:"size_bytes"`
}

// DownloadAvatar downloads the avatar image for a project as base64-encoded data.
func DownloadAvatar(ctx context.Context, client *gitlabclient.Client, input DownloadAvatarInput) (DownloadAvatarOutput, error) {
	if err := ctx.Err(); err != nil {
		return DownloadAvatarOutput{}, err
	}
	if input.ProjectID == "" {
		return DownloadAvatarOutput{}, errors.New("projectDownloadAvatar: project_id is required")
	}
	reader, _, err := client.GL().Projects.DownloadAvatar(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return DownloadAvatarOutput{}, toolutil.WrapErrWithStatusHint("projectDownloadAvatar", err, http.StatusNotFound,
			"project has no custom avatar configured \u2014 GitLab serves a generated identicon when no avatar is set")
	}
	return downloadAvatarOutputFromReader(reader)
}

func downloadAvatarOutputFromReader(reader io.Reader) (DownloadAvatarOutput, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return DownloadAvatarOutput{}, fmt.Errorf("projectDownloadAvatar: reading response: %w", err)
	}
	return DownloadAvatarOutput{
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		SizeBytes:     len(data),
	}, nil
}

// StartHousekeepingInput defines parameters for triggering project housekeeping.
type StartHousekeepingInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// StartHousekeeping triggers housekeeping (git gc/repack) for a project.
func StartHousekeeping(ctx context.Context, client *gitlabclient.Client, input StartHousekeepingInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectStartHousekeeping: project_id is required")
	}
	_, err := client.GL().Projects.StartHousekeepingProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return toolutil.WrapErrWithHint("projectStartHousekeeping", err,
				"housekeeping is already running on this project \u2014 wait for the current run to finish before triggering another")
		}
		return toolutil.WrapErrWithStatusHint("projectStartHousekeeping", err, http.StatusForbidden,
			"starting housekeeping requires Maintainer/Owner role")
	}
	return nil
}

// GetRepositoryStorageInput defines parameters for getting repository storage info.
type GetRepositoryStorageInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// RepositoryStorageOutput holds repository storage information.
type RepositoryStorageOutput struct {
	toolutil.HintableOutput
	ProjectID         int64  `json:"project_id"`
	DiskPath          string `json:"disk_path"`
	RepositoryStorage string `json:"repository_storage"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// GetRepositoryStorage retrieves repository storage information for a project.
func GetRepositoryStorage(ctx context.Context, client *gitlabclient.Client, input GetRepositoryStorageInput) (RepositoryStorageOutput, error) {
	if err := ctx.Err(); err != nil {
		return RepositoryStorageOutput{}, err
	}
	if input.ProjectID == "" {
		return RepositoryStorageOutput{}, errors.New("projectGetRepositoryStorage: project_id is required")
	}
	storage, _, err := client.GL().Projects.GetRepositoryStorage(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return RepositoryStorageOutput{}, toolutil.WrapErrWithStatusHint("projectGetRepositoryStorage", err, http.StatusForbidden,
			"reading repository storage info requires an admin token (instance-administrator scope)")
	}
	out := RepositoryStorageOutput{
		ProjectID:         storage.ProjectID,
		DiskPath:          storage.DiskPath,
		RepositoryStorage: storage.RepositoryStorage,
	}
	if storage.CreatedAt != nil {
		out.CreatedAt = storage.CreatedAt.Format(time.RFC3339)
	}
	return out, nil
}

// CreateForUserInput defines parameters for creating a project on behalf of a
// user. Besides UserID, it carries the full GitLab
// [gl.CreateProjectForUserOptions] (an alias of [gl.CreateProjectOptions])
// settings via an embedded [CreateInput].
type CreateForUserInput struct {
	UserID int64 `json:"user_id" jsonschema:"Target user ID who will own the project,required"`
	CreateInput
}

// CreateForUser creates a new project owned by the specified user (admin operation).
func CreateForUser(ctx context.Context, client *gitlabclient.Client, input CreateForUserInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.UserID == 0 {
		return Output{}, errors.New("projectCreateForUser: user_id is required")
	}
	if input.Name == "" {
		return Output{}, errors.New("projectCreateForUser: name is required")
	}
	createOpts := buildCreateOpts(input.CreateInput)
	opts := (*gl.CreateProjectForUserOptions)(createOpts)
	p, _, err := client.GL().Projects.CreateProjectForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("projectCreateForUser", err,
				"creating a project on behalf of another user requires an admin token (instance-administrator scope) \u2014 use gitlab_project_create for the authenticated user instead")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("projectCreateForUser", err, http.StatusNotFound,
			"target user_id not found \u2014 use gitlab_get_user to verify")
	}
	return ToOutput(p), nil
}
