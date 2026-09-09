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
