package toolutil

import (
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// The shapes in this file are what GitLab sends on an object that the
// client-go struct decoding it does not carry, each read from the captured
// response beside the SDK's own decode (ADR-0021) and named after the Grape
// entity that renders it. A reader takes the capture of one request and
// decodes it into the shape; a list reader holds its count to the SDK's.
// Every gap here is recorded in docs/development/upstream-bugs.md.

// UserBasicOutput mirrors lib/api/entities/user_basic.rb, the user object
// GitLab renders on a note's author and resolver and on a runner's creator.
type UserBasicOutput struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	PublicEmail string `json:"public_email,omitempty"`
	Name        string `json:"name"`
	State       string `json:"state,omitempty"`
	Locked      bool   `json:"locked"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	WebURL      string `json:"web_url,omitempty"`
}

// KeyExtra is what lib/api/entities/ssh_key.rb sends on a key that
// client-go's Key does not carry: the expiry, the last use and the usage
// type, all sent on every key.
type KeyExtra struct {
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	UsageType  string     `json:"usage_type"`
}

// CapturedKey reads, off the captured answer to a request for one key, the
// fields client-go's Key does not model.
func CapturedKey(capture *gitlabclient.ResponseCapture) (KeyExtra, error) {
	return capturedOne[KeyExtra](capture)
}

// CapturedKeys reads the same off a list answer, one extra per key in order,
// the count held to what the SDK decoded.
func CapturedKeys(capture *gitlabclient.ResponseCapture, decoded int) ([]KeyExtra, error) {
	return capturedList[KeyExtra](capture, decoded, "ssh keys")
}

// RunnerExtra is what lib/api/entities/ci/runner.rb sends on a runner that
// client-go's Runner and RunnerDetails do not carry: when it was created,
// who created it, sent to a caller allowed to read that user, and the job
// execution status.
type RunnerExtra struct {
	CreatedAt          *time.Time       `json:"created_at"`
	CreatedBy          *UserBasicOutput `json:"created_by"`
	JobExecutionStatus string           `json:"job_execution_status"`
}

// CapturedRunner reads, off the captured answer to a request for one
// runner, the fields client-go's runner structs do not model.
func CapturedRunner(capture *gitlabclient.ResponseCapture) (RunnerExtra, error) {
	return capturedOne[RunnerExtra](capture)
}

// CapturedRunners reads the same off a list answer, one extra per runner in
// order, the count held to what the SDK decoded.
func CapturedRunners(capture *gitlabclient.ResponseCapture, decoded int) ([]RunnerExtra, error) {
	return capturedList[RunnerExtra](capture, decoded, "runners")
}

// LintJobOutput is one job of the jobs array a CI lint answer carries when
// include_jobs is set, built by lib/gitlab/ci/lint.rb from the processed
// configuration; client-go's ProjectLintResult does not carry the array. A
// static check carries only, except and needs as the configuration spelled
// them, so those three keep the JSON shape GitLab sent, and environment is
// the name on a pipeline simulation and the configuration on a static check.
type LintJobOutput struct {
	Name         string   `json:"name"`
	Stage        string   `json:"stage"`
	BeforeScript []string `json:"before_script"`
	Script       []string `json:"script"`
	AfterScript  []string `json:"after_script"`
	TagList      []string `json:"tag_list"`
	Only         any      `json:"only,omitempty"`
	Except       any      `json:"except,omitempty"`
	Environment  any      `json:"environment,omitempty"`
	When         string   `json:"when"`
	AllowFailure bool     `json:"allow_failure"`
	Needs        any      `json:"needs,omitempty"`
}

// LintExtra is what lib/api/entities/ci/lint/result.rb sends that
// client-go's ProjectLintResult does not carry.
type LintExtra struct {
	Jobs []LintJobOutput `json:"jobs"`
}

// CapturedLint reads, off the captured answer to a lint request, the jobs
// array client-go does not model.
func CapturedLint(capture *gitlabclient.ResponseCapture) (LintExtra, error) {
	return capturedOne[LintExtra](capture)
}

// LabelExtra is what lib/api/entities/label.rb sends on a label that
// client-go's Label and GroupLabel do not carry: the description rendered
// as HTML, sent on every label.
type LabelExtra struct {
	DescriptionHTML string `json:"description_html"`
}

// CapturedLabel reads, off the captured answer to a request for one label,
// the field client-go's label structs do not model.
func CapturedLabel(capture *gitlabclient.ResponseCapture) (LabelExtra, error) {
	return capturedOne[LabelExtra](capture)
}

// CapturedLabels reads the same off a list answer, one extra per label in
// order, the count held to what the SDK decoded.
func CapturedLabels(capture *gitlabclient.ResponseCapture, decoded int) ([]LabelExtra, error) {
	return capturedList[LabelExtra](capture, decoded, "labels")
}

// TokenGranularScopeOutput mirrors
// lib/api/entities/personal_access_token_granular_scope.rb, one entry of the
// granular_scopes array a granular token carries. project_id is set only when
// the scope's namespace is a project and group_id only when it is a group, so
// each is absent rather than zero on the other kind.
type TokenGranularScopeOutput struct {
	Access      string   `json:"access"`
	Permissions []string `json:"permissions"`
	ProjectID   int64    `json:"project_id,omitempty"`
	GroupID     int64    `json:"group_id,omitempty"`
}

// TokenExtra is what lib/api/entities/personal_access_token.rb and the
// entities inheriting it send on a token that client-go's
// PersonalAccessToken does not carry: granular on every token,
// granular_scopes when the token is granular and the endpoint asked for them,
// and last_used_ips while the instance has the feature flag on.
type TokenExtra struct {
	Granular       bool                       `json:"granular"`
	GranularScopes []TokenGranularScopeOutput `json:"granular_scopes"`
	LastUsedIPs    []string                   `json:"last_used_ips"`
}

// CapturedToken reads, off the captured answer to a request for one token,
// the fields client-go's PersonalAccessToken does not model.
func CapturedToken(capture *gitlabclient.ResponseCapture) (TokenExtra, error) {
	return capturedOne[TokenExtra](capture)
}

// CapturedTokens reads the same off a list answer, one extra per token in
// order, the count held to what the SDK decoded.
func CapturedTokens(capture *gitlabclient.ResponseCapture, decoded int) ([]TokenExtra, error) {
	return capturedList[TokenExtra](capture, decoded, "tokens")
}

// ImpersonationTokenExtra is what lib/api/entities/impersonation_token.rb
// sends beyond [TokenExtra] that client-go's ImpersonationToken does not
// carry. Its impersonation flag is its own; description and user_id come from
// the personal access token entity it inherits, which the SDK models on
// PersonalAccessToken and not on ImpersonationToken.
type ImpersonationTokenExtra struct {
	TokenExtra
	Impersonation bool   `json:"impersonation"`
	Description   string `json:"description"`
	UserID        int64  `json:"user_id"`
}

// CapturedImpersonationToken reads, off the captured answer to a request for
// one impersonation token, the fields client-go's ImpersonationToken does not
// model.
func CapturedImpersonationToken(capture *gitlabclient.ResponseCapture) (ImpersonationTokenExtra, error) {
	return capturedOne[ImpersonationTokenExtra](capture)
}

// CapturedImpersonationTokens reads the same off a list answer, one extra per
// token in order, the count held to what the SDK decoded.
func CapturedImpersonationTokens(capture *gitlabclient.ResponseCapture, decoded int) ([]ImpersonationTokenExtra, error) {
	return capturedList[ImpersonationTokenExtra](capture, decoded, "impersonation tokens")
}

// ResourceTokenExtra is what lib/api/entities/resource_access_token.rb sends
// beyond [TokenExtra] on a project or group access token: which kind of
// resource the token belongs to and its id, the second absent when the bot
// user has no namespace.
type ResourceTokenExtra struct {
	TokenExtra
	ResourceType string `json:"resource_type"`
	ResourceID   int64  `json:"resource_id"`
}

// CapturedResourceToken reads, off the captured answer to a request for one
// project or group access token, the fields the SDK's resource access token
// does not model.
func CapturedResourceToken(capture *gitlabclient.ResponseCapture) (ResourceTokenExtra, error) {
	return capturedOne[ResourceTokenExtra](capture)
}

// CapturedResourceTokens reads the same off a list answer, one extra per
// token in order, the count held to what the SDK decoded.
func CapturedResourceTokens(capture *gitlabclient.ResponseCapture, decoded int) ([]ResourceTokenExtra, error) {
	return capturedList[ResourceTokenExtra](capture, decoded, "resource access tokens")
}

// PipelineExtra is what lib/api/entities/ci/pipeline.rb sends on a pipeline
// that client-go's Pipeline does not carry: whether it is archived, sent on
// every pipeline rendered whole.
type PipelineExtra struct {
	Archived bool `json:"archived"`
}

// CapturedPipeline reads, off the captured answer to a request for one
// pipeline, the field client-go's Pipeline does not model.
func CapturedPipeline(capture *gitlabclient.ResponseCapture) (PipelineExtra, error) {
	return capturedOne[PipelineExtra](capture)
}

// TopicExtra is the organization a topic belongs to, which GitLab's topic
// entity exposes under no condition and so sends on every topic.
type TopicExtra struct {
	OrganizationID int64 `json:"organization_id"`
}

// CapturedTopic reads it off the captured answer to a request for one topic.
func CapturedTopic(capture *gitlabclient.ResponseCapture) (TopicExtra, error) {
	return capturedOne[TopicExtra](capture)
}

// CapturedTopics reads the same off a list answer, one extra per topic in
// order, the count held to what the SDK decoded.
func CapturedTopics(capture *gitlabclient.ResponseCapture, decoded int) ([]TopicExtra, error) {
	return capturedList[TopicExtra](capture, decoded, "topics")
}

// AppearanceExtra is the instance's site name, which GitLab's appearance
// entity exposes under no condition.
type AppearanceExtra struct {
	SiteName string `json:"site_name"`
}

// CapturedAppearance reads it off the captured answer to an appearance
// request.
func CapturedAppearance(capture *gitlabclient.ResponseCapture) (AppearanceExtra, error) {
	return capturedOne[AppearanceExtra](capture)
}

// BroadcastMessageExtra is the color a broadcast message is drawn in, which
// GitLab's broadcast message entity exposes under no condition.
type BroadcastMessageExtra struct {
	Color string `json:"color"`
}

// CapturedBroadcastMessage reads it off the captured answer to a request for
// one message.
func CapturedBroadcastMessage(capture *gitlabclient.ResponseCapture) (BroadcastMessageExtra, error) {
	return capturedOne[BroadcastMessageExtra](capture)
}

// CapturedBroadcastMessages reads the same off a list answer, one extra per
// message in order, the count held to what the SDK decoded.
func CapturedBroadcastMessages(capture *gitlabclient.ResponseCapture, decoded int) ([]BroadcastMessageExtra, error) {
	return capturedList[BroadcastMessageExtra](capture, decoded, "broadcast messages")
}

// ClusterAgentExtra is whether an agent is receptive, meaning GitLab connects
// out to it rather than waiting for it to connect in. The agent entity exposes
// it under no condition.
type ClusterAgentExtra struct {
	IsReceptive bool `json:"is_receptive"`
}

// CapturedClusterAgent reads it off the captured answer to a request for one
// agent.
func CapturedClusterAgent(capture *gitlabclient.ResponseCapture) (ClusterAgentExtra, error) {
	return capturedOne[ClusterAgentExtra](capture)
}

// CapturedClusterAgents reads the same off a list answer, one extra per agent
// in order, the count held to what the SDK decoded.
func CapturedClusterAgents(capture *gitlabclient.ResponseCapture, decoded int) ([]ClusterAgentExtra, error) {
	return capturedList[ClusterAgentExtra](capture, decoded, "cluster agents")
}

// LicenseTemplateExtra is whether a license template is one of the popular
// ones GitLab offers first, exposed under no condition.
type LicenseTemplateExtra struct {
	Popular bool `json:"popular"`
}

// CapturedLicenseTemplate reads it off the captured answer to a request for
// one template.
func CapturedLicenseTemplate(capture *gitlabclient.ResponseCapture) (LicenseTemplateExtra, error) {
	return capturedOne[LicenseTemplateExtra](capture)
}

// CapturedLicenseTemplates reads the same off a list answer, one extra per
// template in order, the count held to what the SDK decoded.
func CapturedLicenseTemplates(capture *gitlabclient.ResponseCapture, decoded int) ([]LicenseTemplateExtra, error) {
	return capturedList[LicenseTemplateExtra](capture, decoded, "license templates")
}

// SecureFileExtra is a secure file's extension, exposed under no condition.
type SecureFileExtra struct {
	FileExtension string `json:"file_extension"`
}

// CapturedSecureFile reads it off the captured answer to a request for one
// secure file.
func CapturedSecureFile(capture *gitlabclient.ResponseCapture) (SecureFileExtra, error) {
	return capturedOne[SecureFileExtra](capture)
}

// CapturedSecureFiles reads the same off a list answer, one extra per file in
// order, the count held to what the SDK decoded.
func CapturedSecureFiles(capture *gitlabclient.ResponseCapture, decoded int) ([]SecureFileExtra, error) {
	return capturedList[SecureFileExtra](capture, decoded, "secure files")
}

// PipelineTriggerExtra is when a trigger token stops working, exposed under no
// condition and null on a token that never expires.
type PipelineTriggerExtra struct {
	ExpiresAt *time.Time `json:"expires_at"`
}

// CapturedPipelineTrigger reads it off the captured answer to a request for
// one trigger.
func CapturedPipelineTrigger(capture *gitlabclient.ResponseCapture) (PipelineTriggerExtra, error) {
	return capturedOne[PipelineTriggerExtra](capture)
}

// CapturedPipelineTriggers reads the same off a list answer, one extra per
// trigger in order, the count held to what the SDK decoded.
func CapturedPipelineTriggers(capture *gitlabclient.ResponseCapture, decoded int) ([]PipelineTriggerExtra, error) {
	return capturedList[PipelineTriggerExtra](capture, decoded, "pipeline triggers")
}

// ProtectedBranchExtra is whether a protected branch rule was inherited from
// the group rather than declared on the project, exposed under no condition.
type ProtectedBranchExtra struct {
	Inherited bool `json:"inherited"`
}

// CapturedProtectedBranch reads it off the captured answer to a request for
// one protected branch.
func CapturedProtectedBranch(capture *gitlabclient.ResponseCapture) (ProtectedBranchExtra, error) {
	return capturedOne[ProtectedBranchExtra](capture)
}

// CapturedProtectedBranches reads the same off a list answer, one extra per
// branch in order, the count held to what the SDK decoded.
func CapturedProtectedBranches(capture *gitlabclient.ResponseCapture, decoded int) ([]ProtectedBranchExtra, error) {
	return capturedList[ProtectedBranchExtra](capture, decoded, "protected branches")
}

// MergeRequestDiffExtra is the patch id of a merge request version, which
// identifies the change independently of the commits carrying it. The diff
// entity exposes it under no condition.
type MergeRequestDiffExtra struct {
	PatchIDSHA string `json:"patch_id_sha"`
}

// CapturedMergeRequestDiff reads it off the captured answer to a request for
// one version.
func CapturedMergeRequestDiff(capture *gitlabclient.ResponseCapture) (MergeRequestDiffExtra, error) {
	return capturedOne[MergeRequestDiffExtra](capture)
}

// CapturedMergeRequestDiffs reads the same off a list answer, one extra per
// version in order, the count held to what the SDK decoded.
func CapturedMergeRequestDiffs(capture *gitlabclient.ResponseCapture, decoded int) ([]MergeRequestDiffExtra, error) {
	return capturedList[MergeRequestDiffExtra](capture, decoded, "merge request versions")
}

// SCIMIdentityExtra is the identifier the SCIM provider knows a user by, which
// GitLab's identity detail entity exposes under no condition.
type SCIMIdentityExtra struct {
	ExternUID string `json:"extern_uid"`
}

// CapturedSCIMIdentity reads it off the captured answer to a request for one
// identity.
func CapturedSCIMIdentity(capture *gitlabclient.ResponseCapture) (SCIMIdentityExtra, error) {
	return capturedOne[SCIMIdentityExtra](capture)
}

// CapturedSCIMIdentities reads the same off a list answer, one extra per
// identity in order, the count held to what the SDK decoded.
func CapturedSCIMIdentities(capture *gitlabclient.ResponseCapture, decoded int) ([]SCIMIdentityExtra, error) {
	return capturedList[SCIMIdentityExtra](capture, decoded, "SCIM identities")
}

// RunnerManagerExtra is what the manager is doing now, exposed under no
// condition on the runner manager entity. It is the same key [RunnerExtra]
// carries on the runner itself and a different object: a runner's status is
// the aggregate of its managers'.
type RunnerManagerExtra struct {
	JobExecutionStatus string `json:"job_execution_status"`
}

// CapturedRunnerManagers reads it off the captured answer to a list of
// managers, one extra per manager in order, the count held to what the SDK
// decoded.
func CapturedRunnerManagers(capture *gitlabclient.ResponseCapture, decoded int) ([]RunnerManagerExtra, error) {
	return capturedList[RunnerManagerExtra](capture, decoded, "runner managers")
}

// FeatureDefinitionExtra is where a feature flag's definition points a reader:
// the issue that tracks it and the milestone it is meant to roll out by. The
// definition entity exposes both under no condition.
type FeatureDefinitionExtra struct {
	FeatureIssueURL     string `json:"feature_issue_url"`
	IntendedToRolloutBy string `json:"intended_to_rollout_by"`
}

// CapturedFeatureDefinitions reads them off the captured answer to a list of
// definitions, one extra per definition in order, the count held to what the
// SDK decoded.
func CapturedFeatureDefinitions(capture *gitlabclient.ResponseCapture, decoded int) ([]FeatureDefinitionExtra, error) {
	return capturedList[FeatureDefinitionExtra](capture, decoded, "feature definitions")
}

// FeatureExtra reaches the same two keys where a feature carries its
// definition under a key of its own, which is the shape the feature list
// answers with.
type FeatureExtra struct {
	Definition FeatureDefinitionExtra `json:"definition"`
}

// CapturedFeatures reads them off the captured answer to a list of features,
// one extra per feature in order, the count held to what the SDK decoded.
func CapturedFeatures(capture *gitlabclient.ResponseCapture, decoded int) ([]FeatureExtra, error) {
	return capturedList[FeatureExtra](capture, decoded, "features")
}

// CapturedFeature reads them off the captured answer to a request that set one
// feature flag.
func CapturedFeature(capture *gitlabclient.ResponseCapture) (FeatureExtra, error) {
	return capturedOne[FeatureExtra](capture)
}

// FeatureFlagUserListExtra is where a user list lives in GitLab's own web
// interface, both exposed under no condition.
type FeatureFlagUserListExtra struct {
	Path     string `json:"path"`
	EditPath string `json:"edit_path"`
}

// CapturedFeatureFlagUserList reads them off the captured answer to a request
// for one user list.
func CapturedFeatureFlagUserList(capture *gitlabclient.ResponseCapture) (FeatureFlagUserListExtra, error) {
	return capturedOne[FeatureFlagUserListExtra](capture)
}

// CapturedFeatureFlagUserLists reads the same off a list answer, one extra per
// user list in order, the count held to what the SDK decoded.
func CapturedFeatureFlagUserLists(capture *gitlabclient.ResponseCapture, decoded int) ([]FeatureFlagUserListExtra, error) {
	return capturedList[FeatureFlagUserListExtra](capture, decoded, "feature flag user lists")
}

// CommitCommentExtra is when a commit comment was written, which GitLab's
// commit note entity exposes under no condition.
type CommitCommentExtra struct {
	CreatedAt *time.Time `json:"created_at"`
}

// CapturedCommitComment reads it off the captured answer to a request that
// returned one comment.
func CapturedCommitComment(capture *gitlabclient.ResponseCapture) (CommitCommentExtra, error) {
	return capturedOne[CommitCommentExtra](capture)
}

// CapturedCommitComments reads the same off a list answer, one extra per
// comment in order, the count held to what the SDK decoded.
func CapturedCommitComments(capture *gitlabclient.ResponseCapture, decoded int) ([]CommitCommentExtra, error) {
	return capturedList[CommitCommentExtra](capture, decoded, "commit comments")
}

// DependencyExtra is whether a dependency is known malware, which GitLab
// exposes to a caller allowed to read the project's vulnerabilities and only
// while the instance has the feature enabled.
//
// The pointer carries the third state: true is a detection, false is a
// package the scan cleared, and absent is a scan that did not run. Read into
// a bool the last two would both arrive as false.
type DependencyExtra struct {
	Malware *bool `json:"malware"`
}

// CapturedDependencies reads it off the captured answer to a list of
// dependencies, one extra per dependency in order, the count held to what the
// SDK decoded.
func CapturedDependencies(capture *gitlabclient.ResponseCapture, decoded int) ([]DependencyExtra, error) {
	return capturedList[DependencyExtra](capture, decoded, "dependencies")
}

// DeploymentApprovalOutput is one approval or rejection recorded against a
// deployment, as lib/api/entities/deployments/approval.rb renders it.
type DeploymentApprovalOutput struct {
	User      *UserBasicOutput `json:"user,omitempty"`
	Status    string           `json:"status"`
	CreatedAt *time.Time       `json:"created_at"`
	Comment   string           `json:"comment,omitempty"`
}

// DeploymentApprovalRuleOutput is one rule of the approval summary: who may
// approve, how many approvals it needs, and what has been recorded against it.
type DeploymentApprovalRuleOutput struct {
	ID                     int64                      `json:"id"`
	UserID                 int64                      `json:"user_id,omitempty"`
	GroupID                int64                      `json:"group_id,omitempty"`
	AccessLevel            int64                      `json:"access_level,omitempty"`
	AccessLevelDescription string                     `json:"access_level_description,omitempty"`
	RequiredApprovals      int64                      `json:"required_approvals"`
	GroupInheritanceType   int64                      `json:"group_inheritance_type,omitempty"`
	DeploymentApprovals    []DeploymentApprovalOutput `json:"deployment_approvals,omitempty"`
}

// DeploymentApprovalSummaryOutput is the rules a deployment must satisfy
// before it may run.
type DeploymentApprovalSummaryOutput struct {
	Rules []DeploymentApprovalRuleOutput `json:"rules,omitempty"`
}

// DeploymentExtra is what GitLab's extended deployment entity sends that
// client-go's Deployment does not carry: the approvals recorded so far, how
// many are still outstanding, and the rules they are counted against. All
// three are exposed under no condition, so a deployment that needs no approval
// carries them empty rather than not at all.
type DeploymentExtra struct {
	Approvals            []DeploymentApprovalOutput       `json:"approvals"`
	ApprovalSummary      *DeploymentApprovalSummaryOutput `json:"approval_summary"`
	PendingApprovalCount int64                            `json:"pending_approval_count"`
}

// CapturedDeployment reads them off the captured answer to a request for one
// deployment.
func CapturedDeployment(capture *gitlabclient.ResponseCapture) (DeploymentExtra, error) {
	return capturedOne[DeploymentExtra](capture)
}

// CapturedDeployments reads the same off a list answer, one extra per
// deployment in order, the count held to what the SDK decoded.
func CapturedDeployments(capture *gitlabclient.ResponseCapture, decoded int) ([]DeploymentExtra, error) {
	return capturedList[DeploymentExtra](capture, decoded, "deployments")
}

// PagesCertificateExpirationOutput is when a Pages domain's certificate stops
// being valid, and whether it already has.
type PagesCertificateExpirationOutput struct {
	Expired    bool       `json:"expired"`
	Expiration *time.Time `json:"expiration"`
}

// PagesDomainExtra is that object on the domain, which GitLab exposes only on
// a domain that has a certificate at all.
type PagesDomainExtra struct {
	CertificateExpiration *PagesCertificateExpirationOutput `json:"certificate_expiration"`
}

// CapturedPagesDomain reads it off the captured answer to a request for one
// domain.
func CapturedPagesDomain(capture *gitlabclient.ResponseCapture) (PagesDomainExtra, error) {
	return capturedOne[PagesDomainExtra](capture)
}

// CapturedPagesDomains reads the same off a list answer, one extra per domain
// in order, the count held to what the SDK decoded.
func CapturedPagesDomains(capture *gitlabclient.ResponseCapture, decoded int) ([]PagesDomainExtra, error) {
	return capturedList[PagesDomainExtra](capture, decoded, "pages domains")
}

// ResourceStateEventExtra is what GitLab's resource state event entity sends
// that client-go's StateEvent does not carry: the commit that closed the
// issue, and the merge request that did, each empty when something else did.
type ResourceStateEventExtra struct {
	SourceCommit         string `json:"source_commit"`
	SourceMergeRequestID int64  `json:"source_merge_request_id"`
}

// CapturedResourceStateEvent reads them off the captured answer to a request
// for one event.
func CapturedResourceStateEvent(capture *gitlabclient.ResponseCapture) (ResourceStateEventExtra, error) {
	return capturedOne[ResourceStateEventExtra](capture)
}

// CapturedResourceStateEvents reads the same off a list answer, one extra per
// event in order, the count held to what the SDK decoded.
func CapturedResourceStateEvents(capture *gitlabclient.ResponseCapture, decoded int) ([]ResourceStateEventExtra, error) {
	return capturedList[ResourceStateEventExtra](capture, decoded, "resource state events")
}

// ResourceMilestoneEventExtra is the issue or merge request's own state at the
// moment the milestone changed, which the milestone event entity exposes under
// no condition.
type ResourceMilestoneEventExtra struct {
	State string `json:"state"`
}

// CapturedResourceMilestoneEvent reads it off the captured answer to a request
// for one event.
func CapturedResourceMilestoneEvent(capture *gitlabclient.ResponseCapture) (ResourceMilestoneEventExtra, error) {
	return capturedOne[ResourceMilestoneEventExtra](capture)
}

// CapturedResourceMilestoneEvents reads the same off a list answer, one extra
// per event in order, the count held to what the SDK decoded.
func CapturedResourceMilestoneEvents(capture *gitlabclient.ResponseCapture, decoded int) ([]ResourceMilestoneEventExtra, error) {
	return capturedList[ResourceMilestoneEventExtra](capture, decoded, "resource milestone events")
}

// NamespaceBasicOutput mirrors lib/api/entities/namespace_basic.rb, the group
// object GitLab renders on a todo raised in a group rather than a project.
type NamespaceBasicOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	FullPath  string `json:"full_path"`
	ParentID  int64  `json:"parent_id,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// TodoExtra is what GitLab's todo entity sends that client-go's Todo does not
// carry: when the todo last changed, and the group it belongs to, which is
// present only on a todo raised in a group rather than a project.
type TodoExtra struct {
	UpdatedAt *time.Time            `json:"updated_at"`
	Group     *NamespaceBasicOutput `json:"group"`
}

// CapturedTodo reads them off the captured answer to a request that returned
// one todo.
func CapturedTodo(capture *gitlabclient.ResponseCapture) (TodoExtra, error) {
	return capturedOne[TodoExtra](capture)
}

// CapturedTodos reads the same off a list answer, one extra per todo in order,
// the count held to what the SDK decoded.
func CapturedTodos(capture *gitlabclient.ResponseCapture, decoded int) ([]TodoExtra, error) {
	return capturedList[TodoExtra](capture, decoded, "todos")
}

// BasicGroupDetailsOutput is the group reference GitLab renders on a board,
// which carries three keys and not a whole group.
type BasicGroupDetailsOutput struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"web_url,omitempty"`
}

// BoardExtra is that reference on the board, which GitLab exposes under no
// condition and leaves null on a board that belongs to a project.
type BoardExtra struct {
	Group *BasicGroupDetailsOutput `json:"group"`
}

// CapturedBoard reads it off the captured answer to a request for one board.
func CapturedBoard(capture *gitlabclient.ResponseCapture) (BoardExtra, error) {
	return capturedOne[BoardExtra](capture)
}

// CapturedBoards reads the same off a list answer, one extra per board in
// order, the count held to what the SDK decoded.
func CapturedBoards(capture *gitlabclient.ResponseCapture, decoded int) ([]BoardExtra, error) {
	return capturedList[BoardExtra](capture, decoded, "boards")
}

// BridgeProjectOutput is the project object GitLab renders on a job, which
// carries one key: whether the job token can reach outside this project.
type BridgeProjectOutput struct {
	CIJobTokenScopeEnabled bool `json:"ci_job_token_scope_enabled"`
}

// BridgeExtra is that object on a bridge job, exposed under no condition.
type BridgeExtra struct {
	Project *BridgeProjectOutput `json:"project"`
}

// CapturedBridges reads it off the captured answer to a list of bridges, one
// extra per bridge in order, the count held to what the SDK decoded.
func CapturedBridges(capture *gitlabclient.ResponseCapture, decoded int) ([]BridgeExtra, error) {
	return capturedList[BridgeExtra](capture, decoded, "bridges")
}

// MilestoneExtra is what GitLab's milestone entity sends that a group
// milestone struct does not carry: the milestone's own page, exposed under no
// condition, and the project a project-scoped milestone belongs to, exposed
// only when there is one, so a group milestone leaves it zero.
type MilestoneExtra struct {
	WebURL    string `json:"web_url"`
	ProjectID int64  `json:"project_id"`
}

// CapturedMilestone reads them off the captured answer to a request for one
// milestone.
func CapturedMilestone(capture *gitlabclient.ResponseCapture) (MilestoneExtra, error) {
	return capturedOne[MilestoneExtra](capture)
}

// CapturedMilestones reads the same off a list answer, one extra per milestone
// in order, the count held to what the SDK decoded.
func CapturedMilestones(capture *gitlabclient.ResponseCapture, decoded int) ([]MilestoneExtra, error) {
	return capturedList[MilestoneExtra](capture, decoded, "milestones")
}

// BoardListExtra is the metric a board list limits its work in progress by,
// which GitLab exposes only on a list whose board has work-in-progress limits
// available, so it is empty everywhere else.
type BoardListExtra struct {
	LimitMetric string `json:"limit_metric"`
}

// CapturedBoardList reads it off the captured answer to a request for one
// board list.
func CapturedBoardList(capture *gitlabclient.ResponseCapture) (BoardListExtra, error) {
	return capturedOne[BoardListExtra](capture)
}

// CapturedBoardLists reads the same off a list answer, one extra per board
// list in order, the count held to what the SDK decoded.
func CapturedBoardLists(capture *gitlabclient.ResponseCapture, decoded int) ([]BoardListExtra, error) {
	return capturedList[BoardListExtra](capture, decoded, "board lists")
}

// CIVariableExtra is what GitLab's CI variable entity sends that the instance
// variable struct does not carry: the environments the value applies to, and
// whether the value is hidden from every reader once set. Both are exposed
// only where the variable's own model answers to them.
type CIVariableExtra struct {
	EnvironmentScope string `json:"environment_scope"`
	Hidden           bool   `json:"hidden"`
}

// CapturedCIVariable reads them off the captured answer to a request for one
// variable.
func CapturedCIVariable(capture *gitlabclient.ResponseCapture) (CIVariableExtra, error) {
	return capturedOne[CIVariableExtra](capture)
}

// CapturedCIVariables reads the same off a list answer, one extra per variable
// in order, the count held to what the SDK decoded.
func CapturedCIVariables(capture *gitlabclient.ResponseCapture, decoded int) ([]CIVariableExtra, error) {
	return capturedList[CIVariableExtra](capture, decoded, "CI variables")
}

// RegistryRepositoryExtra is what GitLab's container registry repository
// entity sends beside the name and the path: the size, when the caller asked
// for it, and the path a caller allowed to administer images deletes through.
type RegistryRepositoryExtra struct {
	Size          int64  `json:"size"`
	DeleteAPIPath string `json:"delete_api_path"`
}

// CapturedRegistryRepository reads them off the captured answer to a request
// for one repository.
func CapturedRegistryRepository(capture *gitlabclient.ResponseCapture) (RegistryRepositoryExtra, error) {
	return capturedOne[RegistryRepositoryExtra](capture)
}

// CapturedRegistryRepositories reads the same off a list answer, one extra per
// repository in order, the count held to what the SDK decoded.
func CapturedRegistryRepositories(capture *gitlabclient.ResponseCapture, decoded int) ([]RegistryRepositoryExtra, error) {
	return capturedList[RegistryRepositoryExtra](capture, decoded, "registry repositories")
}

// WikiExtra is what GitLab's wiki page entity sends beside the title and the
// content: the identifier of the page's metadata record, and the YAML front
// matter parsed out of the page, which is a map of whatever keys the author
// wrote. Both are exposed under no condition, and the front matter is empty on
// a page that has none.
type WikiExtra struct {
	WikiPageMetaID int64          `json:"wiki_page_meta_id"`
	FrontMatter    map[string]any `json:"front_matter"`
}

// CapturedWiki reads them off the captured answer to a request for one page.
func CapturedWiki(capture *gitlabclient.ResponseCapture) (WikiExtra, error) {
	return capturedOne[WikiExtra](capture)
}

// CapturedWikis reads the same off a list answer, one extra per page in order,
// the count held to what the SDK decoded.
func CapturedWikis(capture *gitlabclient.ResponseCapture, decoded int) ([]WikiExtra, error) {
	return capturedList[WikiExtra](capture, decoded, "wiki pages")
}

// StorageMoveExtra is why a repository storage move failed, exposed under no
// condition by the group, project and snippet storage move entities alike and
// empty on a move that did not fail.
type StorageMoveExtra struct {
	ErrorMessage string `json:"error_message"`
}

// CapturedStorageMove reads it off the captured answer to a request for one
// storage move.
func CapturedStorageMove(capture *gitlabclient.ResponseCapture) (StorageMoveExtra, error) {
	return capturedOne[StorageMoveExtra](capture)
}

// CapturedStorageMoves reads the same off a list answer, one extra per move in
// order, the count held to what the SDK decoded.
func CapturedStorageMoves(capture *gitlabclient.ResponseCapture, decoded int) ([]StorageMoveExtra, error) {
	return capturedList[StorageMoveExtra](capture, decoded, "storage moves")
}

// ServiceAccountExtra is what GitLab's service account entity sends beside the
// name and username: the public email always, and the unconfirmed one while a
// change of address is waiting to be confirmed.
//
// Both keys are read here for the group endpoint, whose GroupServiceAccount
// carries neither. The project endpoint's ServiceAccount already models the
// unconfirmed address, so that package takes it from the SDK and only the
// public email from here: the same GitLab entity, modeled twice upstream and
// unevenly.
type ServiceAccountExtra struct {
	PublicEmail      string `json:"public_email"`
	UnconfirmedEmail string `json:"unconfirmed_email"`
}

// CapturedServiceAccount reads them off the captured answer to a request for
// one service account.
func CapturedServiceAccount(capture *gitlabclient.ResponseCapture) (ServiceAccountExtra, error) {
	return capturedOne[ServiceAccountExtra](capture)
}

// CapturedServiceAccounts reads the same off a list answer, one extra per
// account in order, the count held to what the SDK decoded.
func CapturedServiceAccounts(capture *gitlabclient.ResponseCapture, decoded int) ([]ServiceAccountExtra, error) {
	return capturedList[ServiceAccountExtra](capture, decoded, "service accounts")
}

// HookHeaderOutput is one custom header a webhook sends with every delivery.
// Only the name is read: GitLab masks the value on the way out, and a header
// value is secret-bearing.
type HookHeaderOutput struct {
	Key string `json:"key"`
}

// SystemHookExtra is what GitLab's hook entity sends beside the event flags:
// which branches a push triggers on and how that filter is read, whether the
// hook has been disabled after failing and until when, the template its
// payload is rendered from, the custom headers configured on it, and the
// organization a system hook belongs to.
//
// All of them are exposed unconditionally except the headers, which a caller
// can ask to be left out, and the organization, which only a system hook has.
type SystemHookExtra struct {
	PushEventsBranchFilter string             `json:"push_events_branch_filter"`
	BranchFilterStrategy   string             `json:"branch_filter_strategy"`
	AlertStatus            string             `json:"alert_status"`
	DisabledUntil          *time.Time         `json:"disabled_until"`
	CustomWebhookTemplate  string             `json:"custom_webhook_template"`
	CustomHeaders          []HookHeaderOutput `json:"custom_headers"`
	OrganizationID         int64              `json:"organization_id"`
}

// CapturedSystemHook reads them off the captured answer to a request for one
// hook.
func CapturedSystemHook(capture *gitlabclient.ResponseCapture) (SystemHookExtra, error) {
	return capturedOne[SystemHookExtra](capture)
}

// CapturedSystemHooks reads the same off a list answer, one extra per hook in
// order, the count held to what the SDK decoded.
func CapturedSystemHooks(capture *gitlabclient.ResponseCapture, decoded int) ([]SystemHookExtra, error) {
	return capturedList[SystemHookExtra](capture, decoded, "system hooks")
}

// DeployKeyProjectOutput is one project a deploy key reaches, rendered as the
// project identity entity: the naming and the creation date, without the
// settings a full project carries.
type DeployKeyProjectOutput struct {
	ID                int64      `json:"id"`
	Description       string     `json:"description"`
	Name              string     `json:"name"`
	NameWithNamespace string     `json:"name_with_namespace"`
	Path              string     `json:"path"`
	PathWithNamespace string     `json:"path_with_namespace"`
	CreatedAt         *time.Time `json:"created_at"`
}

// DeployKeyExtra is what GitLab's deploy key entity sends that the key itself
// does not say: when the key was last used to reach the instance and what it
// may be used for, both unconditional, and the projects it can write to or
// only read from, which are sent when the request asks for them.
type DeployKeyExtra struct {
	LastUsedAt                 *time.Time               `json:"last_used_at"`
	UsageType                  string                   `json:"usage_type"`
	ProjectsWithWriteAccess    []DeployKeyProjectOutput `json:"projects_with_write_access"`
	ProjectsWithReadonlyAccess []DeployKeyProjectOutput `json:"projects_with_readonly_access"`
}

// CapturedDeployKey reads them off the captured answer to a request for one
// deploy key.
func CapturedDeployKey(capture *gitlabclient.ResponseCapture) (DeployKeyExtra, error) {
	return capturedOne[DeployKeyExtra](capture)
}

// CapturedDeployKeys reads the same off a list answer, one extra per key in
// order, the count held to what the SDK decoded.
func CapturedDeployKeys(capture *gitlabclient.ResponseCapture, decoded int) ([]DeployKeyExtra, error) {
	return capturedList[DeployKeyExtra](capture, decoded, "deploy keys")
}

// EventWikiPageOutput is the wiki page an event happened to, as the basic wiki
// page entity renders it: how the page is written, where it lives, its title,
// and the identifier of the record that survives a rename.
type EventWikiPageOutput struct {
	Format         string `json:"format"`
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	WikiPageMetaID int64  `json:"wiki_page_meta_id"`
}

// EventExtra is what GitLab's event entity sends that the event itself does
// not say: whether the event arrived with an import rather than happening
// here and which platform it came from, both unconditional, and the wiki page
// an event about a wiki names.
type EventExtra struct {
	Imported     bool                 `json:"imported"`
	ImportedFrom string               `json:"imported_from"`
	WikiPage     *EventWikiPageOutput `json:"wiki_page"`
}

// CapturedEvents reads them off the captured answer to a list of events, one
// extra per event in order, the count held to what the SDK decoded.
func CapturedEvents(capture *gitlabclient.ResponseCapture, decoded int) ([]EventExtra, error) {
	return capturedList[EventExtra](capture, decoded, "events")
}

// NamespaceExtra is what GitLab's namespace entity sends to a caller allowed
// to see it. An administrator asking about a group is told how many projects
// it holds and how much room their repositories take; a caller who may change
// the namespace's limits is told the compute minutes and the purchased
// storage; and a namespace with a subscription carries when that subscription
// ends and when its seat high-water mark last moved.
type NamespaceExtra struct {
	ProjectsCount                    int64      `json:"projects_count"`
	RootRepositorySize               int64      `json:"root_repository_size"`
	SharedRunnersMinutesLimit        *int64     `json:"shared_runners_minutes_limit"`
	ExtraSharedRunnersMinutesLimit   *int64     `json:"extra_shared_runners_minutes_limit"`
	AdditionalPurchasedStorageSize   *int64     `json:"additional_purchased_storage_size"`
	AdditionalPurchasedStorageEndsOn string     `json:"additional_purchased_storage_ends_on"`
	MaxSeatsUsedChangedAt            *time.Time `json:"max_seats_used_changed_at"`
	EndDate                          string     `json:"end_date"`
}

// CapturedNamespace reads them off the captured answer to a request for one
// namespace.
func CapturedNamespace(capture *gitlabclient.ResponseCapture) (NamespaceExtra, error) {
	return capturedOne[NamespaceExtra](capture)
}

// CapturedNamespaces reads the same off a list answer, one extra per namespace
// in order, the count held to what the SDK decoded.
func CapturedNamespaces(capture *gitlabclient.ResponseCapture, decoded int) ([]NamespaceExtra, error) {
	return capturedList[NamespaceExtra](capture, decoded, "namespaces")
}

// PackageTagOutput is one tag pointing at a package version.
type PackageTagOutput struct {
	ID        int64      `json:"id"`
	PackageID int64      `json:"package_id"`
	Name      string     `json:"name"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// PackagePipelineOutput is the pipeline that built a package version, sent to
// a caller allowed to read it.
type PackagePipelineOutput struct {
	ID        int64            `json:"id"`
	IID       int64            `json:"iid"`
	ProjectID int64            `json:"project_id"`
	SHA       string           `json:"sha"`
	Ref       string           `json:"ref"`
	Status    string           `json:"status"`
	Source    string           `json:"source"`
	CreatedAt *time.Time       `json:"created_at"`
	UpdatedAt *time.Time       `json:"updated_at"`
	WebURL    string           `json:"web_url"`
	User      *UserBasicOutput `json:"user"`
}

// PackageVersionOutput is one other version of the same package, with the tags
// pointing at it and the pipeline that built it.
type PackageVersionOutput struct {
	ID        int64                  `json:"id"`
	Version   string                 `json:"version"`
	CreatedAt *time.Time             `json:"created_at"`
	Tags      []PackageTagOutput     `json:"tags"`
	Pipeline  *PackagePipelineOutput `json:"pipeline"`
}

// PackageExtra is what GitLab's package entity sends that the package itself
// does not say: who published it, unconditionally; the Conan recipe's own name
// on a Conan package; the owning project's id and path, sent when the package
// is listed across a group; and the package's other versions, sent when one
// package is asked for rather than a page of them.
type PackageExtra struct {
	CreatorID        int64                  `json:"creator_id"`
	ConanPackageName string                 `json:"conan_package_name"`
	ProjectID        int64                  `json:"project_id"`
	ProjectPath      string                 `json:"project_path"`
	Versions         []PackageVersionOutput `json:"versions"`
}

// CapturedPackages reads them off the captured answer to a list of packages,
// one extra per package in order, the count held to what the SDK decoded.
func CapturedPackages(capture *gitlabclient.ResponseCapture, decoded int) ([]PackageExtra, error) {
	return capturedList[PackageExtra](capture, decoded, "packages")
}

// AccessRequestExtra is what lib/api/entities/access_requester.rb sends on an
// access request that client-go's AccessRequest does not carry. The entity
// inherits Member and merges UserBasic into it, so everything [MemberExtra]
// reads arrives here too; these are the keys beside them. The two URLs are
// unconditional; the address, the creator and the custom role each wait on
// their own condition, and the expiry is sent on every member.
//
// Three keys the entity can send are deliberately not read: is_using_seat,
// avatar_path and custom_attributes wait on presenter options the caller has
// to ask for, and no access-request route declares show_seat_info, only_path
// or with_custom_attributes, so GitLab never sends them here. The first is
// absent from this shape and the other two arrive through the embed unread.
type AccessRequestExtra struct {
	MemberExtra
	AvatarURL string            `json:"avatar_url"`
	WebURL    string            `json:"web_url"`
	CreatedBy *MemberUserOutput `json:"created_by"`
	// ExpiresAt is a date and not a timestamp, which is how GitLab spells a
	// membership expiry, so it is read as the string it arrives as.
	ExpiresAt  string            `json:"expires_at"`
	Email      string            `json:"email"`
	MemberRole *MemberRoleOutput `json:"member_role"`
}

// CapturedAccessRequest reads them off the captured answer to a request that
// returned one access request.
func CapturedAccessRequest(capture *gitlabclient.ResponseCapture) (AccessRequestExtra, error) {
	return capturedOne[AccessRequestExtra](capture)
}

// CapturedAccessRequests reads the same off a list answer, one extra per
// request in order, the count held to what the SDK decoded.
func CapturedAccessRequests(capture *gitlabclient.ResponseCapture, decoded int) ([]AccessRequestExtra, error) {
	return capturedList[AccessRequestExtra](capture, decoded, "access requests")
}

// BillableMemberExtra is what ee/lib/api/entities/billable_member.rb sends on a
// billable member that client-go's BillableGroupMember does not carry: whether
// the account is locked and the address the user publishes, both of them from
// the UserBasic the entity inherits and both on every member.
//
// The same UserBasic exposes avatar_path and custom_attributes, and neither is
// read: both wait on a presenter option, and the billable members route
// declares neither only_path nor with_custom_attributes, so GitLab has never
// sent either on this response.
//
// It is deliberately not [MemberExtra]. A billable member is a user who counts
// against the seat total and not a membership record, so it carries no access
// level, no expiry and no role. The endpoint's own desc annotates
// Entities::Member while its handler presents this entity, which is why the
// audit reads nine membership keys against a response that has never carried
// one of them.
type BillableMemberExtra struct {
	Locked      bool   `json:"locked"`
	PublicEmail string `json:"public_email"`
}

// CapturedBillableMembers reads them off the captured answer to a list of
// billable members, one extra per member in order, the count held to what the
// SDK decoded.
func CapturedBillableMembers(capture *gitlabclient.ResponseCapture, decoded int) ([]BillableMemberExtra, error) {
	return capturedList[BillableMemberExtra](capture, decoded, "billable members")
}

// InvitationExtra is the token lib/api/entities/invitation.rb exposes under no
// condition and client-go's PendingInvite does not model. It is what the
// invitation URL a recipient follows is built from, so a caller allowed to
// list a group's pending invitations can reissue one without the mail.
type InvitationExtra struct {
	InviteToken string `json:"invite_token"`
}

// CapturedPendingInvites reads it off the captured answer to a list of pending
// invitations, one extra per invitation in order, the count held to what the
// SDK decoded.
func CapturedPendingInvites(capture *gitlabclient.ResponseCapture, decoded int) ([]InvitationExtra, error) {
	return capturedList[InvitationExtra](capture, decoded, "invitations")
}

// SnippetExtra is what GitLab's snippet entity sends that the snippet itself
// does not say: whether it arrived with an import rather than being written
// here and which platform it came from, both unconditional, and the two clone
// URLs of its repository, sent once that repository exists.
type SnippetExtra struct {
	Imported      bool   `json:"imported"`
	ImportedFrom  string `json:"imported_from"`
	SSHURLToRepo  string `json:"ssh_url_to_repo"`
	HTTPURLToRepo string `json:"http_url_to_repo"`
}

// CapturedSnippet reads them off the captured answer to a request for one
// snippet.
func CapturedSnippet(capture *gitlabclient.ResponseCapture) (SnippetExtra, error) {
	return capturedOne[SnippetExtra](capture)
}

// CapturedSnippets reads the same off a list answer, one extra per snippet in
// order, the count held to what the SDK decoded.
func CapturedSnippets(capture *gitlabclient.ResponseCapture, decoded int) ([]SnippetExtra, error) {
	return capturedList[SnippetExtra](capture, decoded, "snippets")
}

// MergeRequestExtra is what lib/api/entities/merge_request_basic.rb sends that
// client-go's BasicMergeRequest does not carry.
//
// Four of the six are the older spelling of something the entity also sends
// under a newer name, and GitLab still sends both: merge_status beside
// detailed_merge_status, reference beside references, work_in_progress beside
// draft, and approvals_before_merge beside the approval rules API. Deprecated
// is not absent, and a caller reading a merge request through this server
// should see what GitLab put on the wire.
//
// title_html and description_html are the exception: the entity exposes them
// under the render_html presenter option, and of every route GitLab mounts
// only GET /projects/:id/merge_requests/:merge_request_iid declares
// render_html as a request parameter. They arrive on that one response, when
// the caller asked for them, and on no other, which is why the output shape
// publishes them omitempty and why a type serving only the other routes leaves
// them out altogether.
//
// approvals_before_merge is a pointer because GitLab sends null where no
// approval count applies, which a bare int64 would flatten into zero, a number
// that means something else.
type MergeRequestExtra struct {
	ApprovalsBeforeMerge *int64 `json:"approvals_before_merge"`
	MergeStatus          string `json:"merge_status"`
	Reference            string `json:"reference"`
	WorkInProgress       bool   `json:"work_in_progress"`
	TitleHTML            string `json:"title_html"`
	DescriptionHTML      string `json:"description_html"`
}

// CapturedMergeRequest reads them off the captured answer to a request that
// returned one merge request.
func CapturedMergeRequest(capture *gitlabclient.ResponseCapture) (MergeRequestExtra, error) {
	return capturedOne[MergeRequestExtra](capture)
}

// CapturedMergeRequests reads the same off a list answer, one extra per merge
// request in order, the count held to what the SDK decoded.
func CapturedMergeRequests(capture *gitlabclient.ResponseCapture, decoded int) ([]MergeRequestExtra, error) {
	return capturedList[MergeRequestExtra](capture, decoded, "merge requests")
}

// UserExtra is what lib/api/entities/user_public.rb, and the User it inherits,
// send on a user that client-go's User does not carry. It is the set every
// route presenting UserPublic answers with, which is what the group-scoped
// user lists (enterprise users, provisioned users, SAML users) and the
// instance-wide ones alike present, so the four packages publishing a user
// share it.
//
// Seven of the ten are exposed under no condition. The three counts are gated
// on Ability.allowed?(current_user, :read_user_profile, user) together with
// following_users_allowed, so the same endpoint sends them to a caller who may
// read the profile and to nobody else. Each is a pointer because zero
// followers and a profile this caller may not read are different answers, and
// a bare number would tell them apart from nothing.
type UserExtra struct {
	CommitEmail       string `json:"commit_email"`
	Discord           string `json:"discord"`
	GitHub            string `json:"github"`
	LocalTime         string `json:"local_time"`
	PreferredLanguage string `json:"preferred_language"`
	Pronouns          string `json:"pronouns"`
	WorkInformation   string `json:"work_information"`
	Followers         *int64 `json:"followers"`
	Following         *int64 `json:"following"`
	IsFollowed        *bool  `json:"is_followed"`
}

// CapturedUser reads, off the captured answer to a request for one user, the
// fields client-go's User does not model.
func CapturedUser(capture *gitlabclient.ResponseCapture) (UserExtra, error) {
	return capturedOne[UserExtra](capture)
}

// CapturedUsers reads the same off a list answer, one extra per user in order,
// the count held to what the SDK decoded.
func CapturedUsers(capture *gitlabclient.ResponseCapture, decoded int) ([]UserExtra, error) {
	return capturedList[UserExtra](capture, decoded, "users")
}

// InstanceUserExtra is [UserExtra] plus the five keys only the instance-wide
// user routes ever send, which is what separates internal/tools/users from the
// three group-scoped packages sharing the smaller shape: those serve
// GET /groups/:id/{enterprise_users,provisioned_users,saml_users}, and GitLab
// presents every one of them with UserPublic.
//
// bio_html comes from lib/api/entities/users/bio_html.rb, which only
// UserProfile includes, so of the routes here it is on GET /users/:id alone.
// The three license-gated keys come from ee/lib/ee/api/entities/user_with_admin.rb,
// which only POST /users and PUT /users/:id present. unconfirmed_email is not a
// user key at all: lib/api/entities/service_account.rb sends it, on the six-key
// object POST /service_accounts answers with, when the account has an address
// change waiting to be confirmed.
//
// The two identifiers are pointers for the reason the counts are: a license
// that does not carry the feature sends no key, and group 0 is not that.
type InstanceUserExtra struct {
	UserExtra
	BioHTML                     string     `json:"bio_html"`
	EnterpriseGroupID           *int64     `json:"enterprise_group_id"`
	EnterpriseGroupAssociatedAt *time.Time `json:"enterprise_group_associated_at"`
	ProvisionedByGroupID        *int64     `json:"provisioned_by_group_id"`
	UnconfirmedEmail            string     `json:"unconfirmed_email"`
}

// CapturedInstanceUser reads, off the captured answer to a request for one
// user on an instance-wide route, everything [CapturedUser] reads and the five
// keys beside it.
func CapturedInstanceUser(capture *gitlabclient.ResponseCapture) (InstanceUserExtra, error) {
	return capturedOne[InstanceUserExtra](capture)
}

// CapturedInstanceUsers reads the same off a list answer, one extra per user in
// order, the count held to what the SDK decoded.
func CapturedInstanceUsers(capture *gitlabclient.ResponseCapture, decoded int) ([]InstanceUserExtra, error) {
	return capturedList[InstanceUserExtra](capture, decoded, "users")
}
