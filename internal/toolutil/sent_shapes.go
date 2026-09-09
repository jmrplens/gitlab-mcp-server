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

// BroadcastMessageExtra is the colour a broadcast message is drawn in, which
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

// LicenseTemplateExtra is whether a licence template is one of the popular
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
	return capturedList[LicenseTemplateExtra](capture, decoded, "licence templates")
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
type DependencyExtra struct {
	Malware bool `json:"malware"`
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
