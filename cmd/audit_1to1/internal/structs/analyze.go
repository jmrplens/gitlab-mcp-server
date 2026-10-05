package structs

import (
	"cmp"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

const (
	optionsSuffix = "Options"
	// responseStructName is the client-go pagination wrapper type. Converters that
	// take it as their SDK arg are list wrappers (the MCP list-output holds a
	// slice-of-element field plus pagination); they are validated through the
	// element converter, never paired against the wrapper. See nonResultSDKStruct.
	responseStructName = "Response"

	docJobsSingle       = "jobs.md#get-a-single-job"
	docGroupEpicBoards  = "group_epic_boards.md"
	docGroupBoards      = "group_boards.md"
	docEnvRetrieve      = "environments.md#retrieve-an-environment"
	docDeployments      = "deployments.md"
	docBoards           = "boards.md"
	docPipelineSched    = "pipeline_schedules.md"
	docPipelineTriggers = "pipeline_triggers.md"
	docMRApprovals      = "merge_request_approvals.md"
	docEpics            = "epics.md#list-all-group-epics"
	// The notes page prints imported and imported_from on every note example
	// and commands_changes on the wiki note examples alone, though
	// lib/api/entities/note.rb exposes all three on every note; the
	// discussions page prints suggestions in the merge request field table.
	docNotesIssueList    = "notes.md#list-all-issue-notes"
	docNotesWikiRetrieve = "notes.md#retrieve-a-wiki-page-note"
	docDiscussionsMRList = "discussions.md#list-all-merge-request-discussion-items"
	// The member pages print two_factor_enabled in their example bodies and
	// name the SAML and SCIM identities under their known issues; locked,
	// public_email, membership_state and override are exposed by
	// lib/api/entities/member.rb and printed on neither page.
	docGroupMembersList   = "group_members.md#list-all-group-members"
	docProjectMembersList = "project_members.md#list-all-members-of-a-project"
	// The pages for the six structs the review found short by a field each
	// print that field in their example bodies, except that the lint page
	// prints the jobs array on the existing-configuration example alone and
	// the runner pages print job_execution_status and never created_by.
	docKeysByID        = "keys.md#retrieve-user-by-ssh-key-id"
	docRunnersList     = "runners.md#list-all-runners"
	docRunnersDetails  = "runners.md#retrieve-runners-details"
	docLintExisting    = "lint.md#validate-existing-cicd-configuration"
	docLabelsList      = "labels.md#list-all-project-labels"
	docGroupLabelsList = "group_labels.md#list-group-labels"
	docPipelinesGet    = "pipelines.md#retrieve-a-single-pipeline"
	docTriggersRun     = "pipeline_triggers.md#trigger-a-pipeline-with-a-token"

	docGroupsList       = "groups.md#list-all-groups"
	docGroupsGet        = "groups.md#get-a-single-group"
	docGroupHooks       = "group_webhooks.md"
	docProjectHooks     = "project_webhooks.md"
	docNamespaces       = "namespaces.md"
	docIssuesList       = "issues.md#list-issues"
	docProjectsGet      = "projects.md#get-a-single-project"
	docProjectUsersList = "projects.md#list-a-projects-users"
	// The personal access tokens page prints granular_scopes and
	// last_used_ips in its list example and names both in its notes;
	// granular, impersonation and the resource pair are exposed by the
	// entities and printed on no page, which is why the entity is cited in
	// the comment beside each block below.
	// doc/api/merge_requests.md prints merge_status and reference in every
	// example body and names approvals_before_merge and work_in_progress among
	// the deprecated keys GitLab still sends; lib/api/entities/merge_request_basic.rb
	// exposes all four unconditionally. title_html and description_html wait on
	// render_html, which of the whole record only the single-merge-request GET
	// declares, and the blocked merge request is the other end of a dependency,
	// which client-go's MergeRequestDependency does not model at all.
	docMergeRequests      = "merge_requests.md"
	docMergeRequestRender = docMergeRequests + " (render_html, declared on GET /projects/:id/merge_requests/:merge_request_iid alone)"
	docMergeRequestBlocks = docMergeRequests + " (blocked_merge_request on GET /projects/:id/merge_requests/:merge_request_iid/blocks)"
	// doc/api/users.md prints all ten keys UserPublic adds to the user object
	// in its single-user and current-user example bodies, and bio_html beside
	// them on the single-user one. It is cited for the three group-scoped user
	// packages too, because GitLab presents their lists `with:
	// ::API::Entities::UserPublic` and so answers with that same object;
	// doc/api/group_enterprise_users.md prints most of it and leaves discord,
	// github and is_followed out of its examples, which the entity exposes on
	// every user regardless. The two enterprise keys are exposed by
	// ee/lib/ee/api/entities/user_with_admin.rb under the domain_verification
	// license and printed on no page, which is why the entity is named here
	// rather than a section. unconfirmed_email is not a user key at all:
	// doc/api/service_accounts.md documents it on the service account object
	// POST /service_accounts answers with.
	docUsers                = "users.md"
	docUsersEnterpriseGroup = docUsers + " (enterprise_group_id and enterprise_group_associated_at, exposed by " +
		"ee/lib/ee/api/entities/user_with_admin.rb under the domain_verification license and printed on no page)"
	docServiceAccounts = "service_accounts.md"
	// API::Entities::RelatedIssue inherits API::Entities::Issue and adds four
	// link keys, so the issue object is what the relation list answers with
	// and doc/api/issues.md prints that object in full: every key cited here
	// appears in its example bodies. doc/api/issue_links.md is not the
	// citation because its own list example is abbreviated to fourteen keys
	// and shows none of them. The four licensed keys are exposed by
	// ee/lib/ee/api/entities/issue.rb, so the feature gating them is named
	// beside the page rather than left to the reader.
	docIssueLinksRelation = "issues.md"
	docIssueLinksEpic     = docIssueLinksRelation + " (epic and epic_iid, exposed by ee/lib/ee/api/entities/issue.rb when the " +
		"issue's group has the epics licensed feature)"
	docIssueLinksLicensed = docIssueLinksRelation + " (health_status under the issuable_health_status licensed feature and " +
		"iteration under iterations, both exposed by ee/lib/ee/api/entities/issue.rb)"
	// The member role page is the one citation here that is deliberately not a
	// page reference to the fields themselves, because no page carries them.
	// doc/api/member_roles.md prints four permission keys in its example
	// bodies and sends the reader elsewhere for the rest, so the entity is the
	// oracle: ee/lib/api/entities/member_role.rb exposes every permission by
	// looping over a constant the running application assembles, with
	// `default: false` and no condition. The names are in no source a scan
	// could read, which is why the committed live record is where they came
	// from.
	docMemberRolePermissions = "member_roles.md (ee/lib/api/entities/member_role.rb exposes every " +
		"::MemberRole.all_customizable_permissions entry with default: false and no condition; the page prints four of the " +
		"forty-five in its examples and refers to user/custom_roles/abilities.md for the rest)"
	// The twelve keys API::Entities::Ci::Pipeline adds to the basic entity
	// reach pipelines.Output through one route, the merge request pipeline
	// creation, so its section on doc/api/merge_requests.md is the citation
	// rather than doc/api/pipelines.md, whose own pipeline endpoints fill
	// pipelines.DetailOutput. That example body prints eleven of the twelve
	// and leaves queued_duration out, which the entity exposes with no
	// condition beside the other eleven.
	docMRCreatePipeline      = docMergeRequests + "#create-merge-request-pipeline"
	docMRCreatePipelineQueue = docMRCreatePipeline + " (queued_duration, exposed unconditionally by " +
		"lib/api/entities/ci/pipeline.rb and absent from that section's example body)"
	// lib/api/entities/plan_limit.rb exposes twenty-nine limits and the change
	// history, every one with no condition, and client-go's PlanLimit models
	// the eight package file sizes. The page lists the rest among the update
	// parameters, except the two webhook tiers GitLab.com uses, and neither of
	// its example bodies carries the webhook limits or the history, so the
	// entity is the oracle for those four.
	docPlanLimits = "plan_limits.md (lib/api/entities/plan_limit.rb exposes every limit and limits_history " +
		"with no condition; web_hook_calls_low, web_hook_calls_mid and limits_history are on the entity and not on the page)"
	docOrbitSchemaText = "orbit.md, which defers to the generated reference, and that gives GET /orbit/schema no response " +
		"schema; get_graph_schema in ee/lib/analytics/knowledge_graph/grpc_client.rb answers the llm format with " +
		"`{ formatted_text: response.formatted_text }`, recorded in docs/development/orbit-responses.json as orbit.schema (llm)"
	docPATList            = "personal_access_tokens.md#list-all-personal-access-tokens"
	docProjectTokensList  = "project_access_tokens.md#list-all-project-access-tokens"
	docGroupTokensList    = "group_access_tokens.md"
	docImpersonationPage  = "user_tokens.md#list-all-impersonation-tokens-for-a-user"
	docServiceAccountPATs = "group_service_accounts.md"
	// The dual-shape labels array cannot be two types under one key in a typed
	// schema, so the object half is published beside the names the way issues
	// and merge requests publish theirs.
	docEpicsLabelDetails = docEpics + " (with_labels_details returns each label whole in the labels array)"
	// The OpenAPI record lists the properties of the epic and not those of the
	// user object inside it, so the author's own field set is read off the
	// response GitLab sends.
	docEpicsAuthor = docEpics + " (a live GET /api/v4/groups/gitlab-org/epics on 2026-09-07 answered with " +
		"eight author keys; gl.EpicAuthor declares six and gl.BasicUser seven, neither of them locked or public_email)"
	// epicPhantomWidget is the reason every widget-backed work item option an
	// Epic does not carry is absent from the epic inputs.
	epicPhantomWidget = "an Epic carries no STATUS, ITERATION or CRM_CONTACTS widget, so GitLab refuses the field; " +
		"exposed on internal/tools/workitems, where the type is the caller's to choose"
	// epicReleaseFilter is the reason the two release filters are absent, which
	// is not the widget one: the schema does accept them here, and it is the
	// epic that has nothing for them to match.
	epicReleaseFilter = "Group.workItems declares releaseTag and releaseTagWildcardId, so GitLab accepts them " +
		"rather than refusing them the way it refuses the widget filters; the schema puts releases on Project " +
		"and epic.list pins types to EPIC at group scope, so the filter would select epics by an association " +
		"only a project's work items can have. Exposed on internal/tools/workitems, where the type is the " +
		"caller's to choose"
	// deploymentMergeRequestsInert is the reason three merge request list
	// options are absent from the deployment merge request input.
	deploymentMergeRequestsInert = "declared by GET /projects/:id/deployments/:deployment_id/merge_requests through " +
		"merge_requests_base_params and read by nothing there: lib/api/deployments.rb presents MergeRequestBasic " +
		"with current_user alone, and only serializer_options_for in lib/api/merge_requests.rb turns the option " +
		"into a presenter option; exposed on the merge request list inputs, where it takes effect"
	// skypeDiscarded is the reason the user create and modify inputs offer no
	// skype, which client-go's options still carry.
	skypeDiscarded = "GitLab discards it: the users.skype column is ignored since 18.4 and neither " +
		"POST /users nor PUT /users/:id declares the param, so a caller who set it changed nothing"
	tagKeyJSON = "json"
	// tagKeyURL is the tag go-querystring names a query parameter by, which
	// client-go's Options structs carry beside their json tags.
	tagKeyURL = "url"
	// projectParamGitLabDropped is the reason a project option client-go still
	// carries is absent from the project inputs.
	projectParamGitLabDropped = "not declared by POST /projects or PUT /projects/:id in GitLab 19.3.1 " +
		"(lib/api/helpers/projects_helpers.rb at v19.3.1-ee) or 19.4.1 (gitlab-api-live.json), so GitLab drops " +
		"the value; client-go keeps the field"
	// mrApprovalRulesOffsetOnly is the reason the approval rules list offers
	// no ordering or keyset field of the gl.ListOptions it pages with.
	mrApprovalRulesOffsetOnly = "GET /projects/:id/merge_requests/:merge_request_iid/approval_rules declares only page " +
		"and per_page (gitlab-api-live.json, 19.4.1-ee); gl.ListOptions ordering and keyset fields are unused plumbing there"
	// achievementAvatarUpload is the reason the avatar upload is absent under
	// its SDK key.
	achievementAvatarUpload = "offered as AvatarInput (avatar_filename, avatar_content_type, and avatar_file_path or " +
		"avatar_content_base64), which the handler assembles into the *gl.GraphQLUpload opts.Avatar takes"
	typNameString = "string"
	typNameInt64  = "int64"
)

// acceptedOutputRenames suppresses specific MCP output json tags from
// R-OUTPUT-EXTRA: deliberate 1:1 renames of a genuine scalar (1:1 means data
// fidelity, not byte-identical keys). It is intentionally conservative — only
// true scalar renames belong here, never flattened-duplication tags (those are
// genuine findings, e.g. branches `commit_id` flattening `commit.id`).
//
// Key = "<package>.<MCP output type>.<tag>", the form the other five tables
// take, with a one-line rationale as the value. There is deliberately no form
// without a package: a key naming only a type and a tag excuses that tag on
// every package's type of that name, which is how a rename adjudicated for one
// package would silence an invented field in another.
//
// It is empty, and that is a healthy state rather than an unfinished one. Its
// one entry declared branches.Output's branch_name, which that type stopped
// publishing when it came to mirror gl.Branch under the SDK's own name key,
// and the entry excused nothing from then on.
var acceptedOutputRenames = &declarationTable{name: "acceptedOutputRenames", entries: map[string]string{}}

// isAcceptedRename reports whether the tag of an MCP output type is an
// allowlisted deliberate rename in that package.
func (r *diffRun) isAcceptedRename(pkg, mcpType, tag string) bool {
	return r.answer(acceptedOutputRenames, pkg+"."+mcpType+"."+tag)
}

// curatedRefSubsets marks MCP output types that are DOCUMENTED REFERENCE SUBSETS
// of a larger SDK struct: a nested object the GitLab REST API documents as
// returning only an identity subset for a given endpoint (e.g. a board's nested
// `group` returns id/name/web_url, not the full ~60-field Group). For these types
// the SDK-struct comparison would falsely count every omitted deep field as
// "missing output"; the official API doc is the 1:1 ground truth, so MissingFields
// is suppressed. ExtraFields (invented scalars) and TypeMismatches are STILL
// reported — we still forbid inventing fields and still type-check kept fields.
//
// Each entry cites the doc/api/<file> that justifies the subset, so the curation
// is criterion-based (what the endpoint documents) rather than arbitrary. The
// type also carries a `// Documented reference subset per doc/api/<file>` comment
// at its definition for inline traceability. Key = "<package>.<MCP output type>".
var curatedRefSubsets = &declarationTable{name: "curatedRefSubsets", entries: map[string]string{
	// Populated per package as outputs are reconciled to the official API docs
	// (https://gitlab.com/gitlab-org/gitlab/-/raw/master/doc/api/<file>).
	//
	// environments — nested objects of the "Retrieve an environment" response
	// (doc/api/environments.md#retrieve-an-environment).
	"environments.ClusterAgentOutput":     docEnvRetrieve,
	"environments.DeploymentOutput":       docEnvRetrieve,
	"environments.DeployableUserOutput":   docEnvRetrieve,
	"environments.DeployableCommitOutput": docEnvRetrieve,
	"environments.DeployableRunnerOutput": docEnvRetrieve,

	// jobs — nested objects of the job response (doc/api/jobs.md#get-a-single-job).
	"jobs.CommitObject":       docJobsSingle,
	"jobs.UserObject":         docJobsSingle,
	"jobs.ProjectObject":      docJobsSingle,
	"jobs.PipelineInfoObject": "jobs.md#list-pipeline-trigger-jobs",

	// boards — nested objects of the board / board-list responses (doc/api/boards.md).
	"boards.ProjectOutput":   docBoards,
	"boards.MilestoneOutput": docBoards,
	"boards.BasicUserOutput": docBoards,
	"boards.LabelOutput":     docBoards,

	// deployments — nested objects of the deployment response (doc/api/deployments.md).
	"deployments.EnvironmentOutput":      docDeployments,
	"deployments.DeployableOutput":       docDeployments,
	"deployments.DeployableUserOutput":   docDeployments,
	"deployments.DeployableCommitOutput": docDeployments,
	"deployments.DeployableRunnerOutput": docDeployments,

	// pipelineschedules — nested objects of the schedule response (doc/api/pipeline_schedules.md).
	"pipelineschedules.OwnerOutput":        docPipelineSched,
	"pipelineschedules.LastPipelineOutput": docPipelineSched,

	// mrapprovals — nested user/group reference objects of the approval-rule /
	// approval-state responses (doc/api/merge_request_approvals.md).
	"mrapprovals.BasicUserOutput": docMRApprovals,
	"mrapprovals.GroupOutput":     docMRApprovals,

	// projects — owner is a documented identity subset of the full gl.User
	// (doc/api/projects.md owner.* attribute table).
	"projects.OwnerOutput": "projects.md",

	// uploads / groupmarkdownuploads — uploaded_by is a documented {id,name,username}
	// subset (doc/api/project_markdown_uploads.md, doc/api/group_markdown_uploads.md).
	"uploads.UploadedByOutput":              "project_markdown_uploads.md",
	"groupmarkdownuploads.UploadedByOutput": "group_markdown_uploads.md",

	// groupboards — nested refs of the group-board responses (doc/api/group_boards.md).
	"groupboards.GroupRefOutput":  docGroupBoards,
	"groupboards.MilestoneOutput": docGroupBoards,
	"groupboards.BasicUserOutput": docGroupBoards,
	"groupboards.LabelOutput":     docGroupBoards,

	// groupepicboards — nested refs of the epic-board responses (doc/api/group_epic_boards.md).
	"groupepicboards.GroupRefOutput":  docGroupEpicBoards,
	"groupepicboards.ListLabelOutput": docGroupEpicBoards,

	// pipelinetriggers — owner/user are documented identity subsets of gl.User
	// (doc/api/pipeline_triggers.md).
	"pipelinetriggers.UserOutput":      docPipelineTriggers,
	"pipelinetriggers.BasicUserOutput": docPipelineTriggers,

	// groupreleases: the commit is a documented subset (doc/api/group_releases.md).
	"groupreleases.CommitOutput": "group_releases.md",

	// tags — the tag's nested commit is a documented identity subset (doc/api/tags.md).
	"tags.CommitOutput": "tags.md",
}}

// isCuratedRefSubset reports whether the MCP output type (scoped by package) is a
// doc-justified reference subset whose omitted-vs-full-SDK fields are not flagged.
func (r *diffRun) isCuratedRefSubset(pkg, mcpType string) bool {
	return r.answer(curatedRefSubsets, pkg+"."+mcpType)
}

// docOmittedFields lists individual top-level SDK result fields the official API
// doc does NOT include in the documented response for that endpoint, so the MCP
// output intentionally omits them: the doc is the 1:1 ground truth (we expose what
// the endpoint returns, not every field of the SDK struct). Unlike
// curatedRefSubsets (a whole nested reference type), this is per-field on a primary
// output type. Each entry cites the doc/api/<file>. Key = "<pkg>.<MCP type>.<tag>".
var docOmittedFields = &declarationTable{name: "docOmittedFields", entries: map[string]string{
	// mrapprovals and mergerequests: gl.MergeRequestApprovals declares
	// approvals_before_merge, and neither entity GitLab answers the approvals
	// GET, the approve or the unapprove with exposes it: not the four keys a
	// Community Edition instance sends, and not the approval state an
	// Enterprise one sends instead. The other nineteen keys the struct declares
	// beyond the four are published, read off the captured answer with their
	// presence, since every Enterprise build sends them.
	"mrapprovals.ConfigOutput.approvals_before_merge":    docApprovalsBeforeMergeNeverSent,
	"mergerequests.ApproveOutput.approvals_before_merge": docApprovalsBeforeMergeNeverSent,
	// runners: gl.RunnerDetails carries Token because client-go reuses one
	// struct for the runner endpoints, and GitLab mints a runner's
	// authentication token once, at registration. The runner details response
	// never carries it, so the field was the zero value on every call.
	"runners.DetailsOutput.token": docRunnerDetailsGET,
	// epics: gl.WorkItem is the struct of every work item type, and an Epic
	// carries neither the STATUS nor the ITERATION widget, so both keys were
	// null on every epic response. parent is exposed flattened, as the
	// parent_iid and parent_path pair the widget carries.
	//
	// The REST epic's own omissions (user_notes_count and url, which gl.Epic
	// declares and no epic endpoint sends) are not declared here: no converter
	// pairs an epic output with gl.Epic, so a key for either would answer
	// nothing. subscribed, reference and label_details would need no key even
	// then, since gl.Epic declares none of them. Why each is absent, and what to
	// declare if a converter takes gl.Epic again, is recorded beside the
	// converters in internal/tools/epics (toLinkItem).
	"epics.Output.status":       epicPhantomWidget,
	"epics.Output.iteration_id": epicPhantomWidget,
	"epics.Output.parent":       "exposed flattened as parent_iid + parent_path (the two fields of gl.WorkItemIID)",
}}

// docAPIShapesRecord names where the citations below were read.
//
// It used to be spelled from the package that held a pinned copy of that
// document in this repository. The copy is gone, replaced by a record taken
// from a booted GitLab, and the citations stay as they are because they are
// evidence about what was read and when, not a path a reader is invited to
// open here. GitLab publishes the document, so the citation names it there.
const docAPIShapesRecord = "GitLab's generated OpenAPI document (doc/api/openapi/openapi_v2.yaml) "

// docApprovalsBeforeMergeNeverSent cites the two entities the approvals GET,
// the approve and the unapprove are answered with, neither of which exposes
// the key client-go declares.
//
// It cites GitLab's source rather than doc/api/merge_request_approvals.md,
// the form most entries here take, because that page prints neither shape
// whole: its examples are a subset of the Enterprise answer, and it never
// shows the Community one. It is one literal rather than a concatenation, as
// is the one below, because gremlins mutates the operator of a constant
// expression and no test can execute a constant to catch it.
const docApprovalsBeforeMergeNeverSent = "lib/api/entities/merge_request_approvals.rb (the four keys present_approval renders on Community Edition) and ee/lib/api/entities/approval_state.rb (what ee/lib/ee/api/merge_request_approvals.rb renders on every Enterprise build instead) expose no approvals_before_merge; the key is the merge request's, exposed by ee/lib/ee/api/entities/merge_request_basic.rb"

// docApprovalStateInvalidRules cites the entity that sends the one key of the
// Enterprise approval state client-go does not declare.
const docApprovalStateInvalidRules = "ee/lib/api/entities/approval_state.rb exposes invalid_approvers_rules (Entities::ApprovalRuleShort: id, name, rule_type), which every Enterprise build answers the approvals GET, the approve and the unapprove with; gl.MergeRequestApprovals does not declare it, so it is read off the captured answer (ADR-0021)"

// docRunnerDetailsGET cites the record rather than the prose page:
// runners.md prints one example body for the whole page, so the prose
// cannot tell the registration response from the details one.
const docRunnerDetailsGET = docAPIShapesRecord +
	"(GET /api/v4/runners/{id} and PUT /api/v4/runners/{id} declare no token; POST /api/v4/runners, the " +
	"registration endpoint, answers with id, token and token_expires_at, which runners.Output carries)"

// isDocOmittedField reports whether an SDK field is a doc-justified intentional
// omission on a primary MCP output type.
func (r *diffRun) isDocOmittedField(pkg, mcpType, tag string) bool {
	return r.answer(docOmittedFields, pkg+"."+mcpType+"."+tag)
}

// docAddedFields is the symmetric carve-out to docOmittedFields: documented API
// response fields the client-go SDK struct does NOT expose, which we surface via a
// raw-REST/GraphQL fetch (client.GL().NewRequest+Do into a superset struct, per
// ADR-0006). Because the SDK struct lacks them, they would otherwise be flagged as
// R-OUTPUT-EXTRA ("invented"); they are NOT invented — the official API doc returns
// them. Each entry cites the doc/api/<file>. Key = "<pkg>.<MCPType>.<tag>". These
// fields carry `omitempty` so they degrade gracefully on older GitLab versions that
// don't return them.
var docAddedFields = &declarationTable{name: "docAddedFields", entries: map[string]string{
	// jobs — documented in doc/api/jobs.md but absent from gl.Job / gl.JobRunner;
	// fetched via raw REST (rawGetJob/rawListJobs into the jobAPI superset).
	"jobs.Output.archived":          docJobsSingle,
	"jobs.Output.runner_manager":    docJobsSingle,
	"jobs.RunnerObject.ip_address":  docJobsSingle,
	"jobs.RunnerObject.online":      docJobsSingle,
	"jobs.RunnerObject.paused":      docJobsSingle,
	"jobs.RunnerObject.runner_type": docJobsSingle,
	"jobs.RunnerObject.status":      docJobsSingle,

	// boards — limit_metric is documented in doc/api/boards.md on each board list
	// (all_metrics / issue_count / issue_weights, or null) but absent from
	// gl.BoardList; fetched via raw REST (rawGetBoard/rawListBoardLists into the
	// boardListAPI superset).
	"boards.BoardListOutput.limit_metric": "boards.md#list-all-board-lists-in-an-issue-board",

	// deployments — deployable.project {ci_job_token_scope_enabled} is documented in
	// doc/api/deployments.md but absent from gl.DeploymentDeployable; fetched via raw
	// REST (rawGetDeployment/rawListDeployments into the deploymentAPI superset).
	"deployments.DeployableOutput.project": docDeployments,

	// mrapprovals — approval rule `overridden` is documented (approval_state/list/
	// create/update rule responses) but absent from gl.MergeRequestApprovalRule;
	// fetched via raw REST (rawApprovalState/rawListApprovalRules/rawMutateApprovalRule).
	"mrapprovals.RuleOutput.overridden": docMRApprovals,

	// mrapprovals and mergerequests: the one key of the Enterprise approval
	// state client-go does not declare, read off the captured answer rather
	// than a raw fetch, and published on both outputs that carry the state.
	"mrapprovals.ConfigOutput.invalid_approvers_rules":    docApprovalStateInvalidRules,
	"mergerequests.ApproveOutput.invalid_approvers_rules": docApprovalStateInvalidRules,

	// invites: queued_users is documented in doc/api/invitations.md on the
	// add-a-member response for an instance with member promotion management
	// enabled, and absent from gl.InvitesResult; fetched via raw REST
	// (postInvitation into the invitesResultAPI superset). The page is spelled
	// with its doc/api prefix here so -validate-docs scans it: the table's own
	// values carry only the bare file name, which docCitationRE does not match.
	"invites.InviteResultOutput.queued_users": "invitations.md#add-a-member-to-a-group-or-project",

	// epics: the object half of the dual-shape labels array belongs to the
	// endpoint whose parameter asks for it, and the author carries two keys
	// neither client-go author struct declares. The twelve fields the REST epic
	// adds to gl.Epic are not declared here, for the reason the omissions above
	// give: no converter pairs an epic output with gl.Epic.
	"epics.Output.label_details":         docEpicsLabelDetails,
	"epics.BasicUserOutput.locked":       docEpicsAuthor,
	"epics.BasicUserOutput.public_email": docEpicsAuthor,

	// notes and discussions — four fields lib/api/entities/note.rb exposes that
	// gl.Note does not declare, and the two resolution flags
	// lib/api/entities/discussion.rb exposes that gl.Discussion does not. Read
	// from the captured response beside the SDK's own decode (ADR-0021,
	// toolutil.CapturedNote and CapturedDiscussion) rather than a raw fetch, so
	// the route, the options and the pagination stay client-go's. The entities
	// are the evidence; the pages print each field somewhere, and are cited for
	// where. Recorded in docs/development/upstream-bugs.md.
	"issuenotes.Output.imported":                    docNotesIssueList,
	"issuenotes.Output.imported_from":               docNotesIssueList,
	"issuenotes.Output.commands_changes":            docNotesWikiRetrieve,
	"issuenotes.Output.suggestions":                 docDiscussionsMRList,
	"mrnotes.Output.imported":                       docNotesIssueList,
	"mrnotes.Output.imported_from":                  docNotesIssueList,
	"mrnotes.Output.commands_changes":               docNotesWikiRetrieve,
	"mrnotes.Output.suggestions":                    docDiscussionsMRList,
	"snippetnotes.Output.imported":                  docNotesIssueList,
	"snippetnotes.Output.imported_from":             docNotesIssueList,
	"snippetnotes.Output.commands_changes":          docNotesWikiRetrieve,
	"snippetnotes.Output.suggestions":               docDiscussionsMRList,
	"commitdiscussions.NoteOutput.imported":         docNotesIssueList,
	"commitdiscussions.NoteOutput.imported_from":    docNotesIssueList,
	"commitdiscussions.NoteOutput.commands_changes": docNotesWikiRetrieve,
	"commitdiscussions.NoteOutput.suggestions":      docDiscussionsMRList,
	"commitdiscussions.Output.resolvable":           docDiscussionsMRList,
	"commitdiscussions.Output.resolved":             docDiscussionsMRList,
	"mrdiscussions.NoteOutput.imported":             docNotesIssueList,
	"mrdiscussions.NoteOutput.imported_from":        docNotesIssueList,
	"mrdiscussions.NoteOutput.commands_changes":     docNotesWikiRetrieve,
	"mrdiscussions.NoteOutput.suggestions":          docDiscussionsMRList,
	"mrdiscussions.Output.resolvable":               docDiscussionsMRList,
	"mrdiscussions.Output.resolved":                 docDiscussionsMRList,

	// members — what lib/api/entities/member.rb exposes that gl.GroupMember
	// and gl.ProjectMember do not declare, read from the captured response
	// (ADR-0021, toolutil.CapturedMember and CapturedMembers) in the three
	// packages that present a member. The project struct also lacks the
	// public_email and group_saml_identity the group struct carries.
	// Recorded in docs/development/upstream-bugs.md.
	"groupmembers.Output.locked":              docGroupMembersList,
	"groupmembers.Output.membership_state":    docGroupMembersList,
	"groupmembers.Output.two_factor_enabled":  docGroupMembersList,
	"groupmembers.Output.group_scim_identity": docGroupMembersList,
	"groupmembers.Output.override":            docGroupMembersList,
	"groups.MemberOutput.locked":              docGroupMembersList,
	"groups.MemberOutput.membership_state":    docGroupMembersList,
	"groups.MemberOutput.two_factor_enabled":  docGroupMembersList,
	"groups.MemberOutput.group_scim_identity": docGroupMembersList,
	"groups.MemberOutput.override":            docGroupMembersList,
	"members.Output.locked":                   docProjectMembersList,
	"members.Output.public_email":             docProjectMembersList,
	"members.Output.membership_state":         docProjectMembersList,
	"members.Output.group_saml_identity":      docProjectMembersList,
	"members.Output.group_scim_identity":      docProjectMembersList,
	"members.Output.override":                 docProjectMembersList,

	// keys, runners, cilint, labels, pipelines — one field each (three on a
	// key and on a runner) that the entity sends on every object and the
	// client-go struct does not declare, read from the captured response
	// (ADR-0021, the readers in toolutil/sent_shapes.go). Recorded in
	// docs/development/upstream-bugs.md.
	"keys.Output.expires_at":                     docKeysByID,
	"keys.Output.last_used_at":                   docKeysByID,
	"keys.Output.usage_type":                     docKeysByID,
	"runners.Output.created_at":                  docRunnersList,
	"runners.Output.created_by":                  docRunnersList,
	"runners.Output.job_execution_status":        docRunnersList,
	"runners.DetailsOutput.created_at":           docRunnersDetails,
	"runners.DetailsOutput.created_by":           docRunnersDetails,
	"runners.DetailsOutput.job_execution_status": docRunnersDetails,
	"cilint.Output.jobs":                         docLintExisting,
	"labeldata.Output.description_html":          docLabelsList,
	"labels.Output.description_html":             docLabelsList,
	"grouplabels.Output.description_html":        docGroupLabelsList,
	"pipelines.DetailOutput.archived":            docPipelinesGet,
	"pipelinetriggers.RunOutput.archived":        docTriggersRun,

	// groups, issues and projects — the three entity families whose Grape
	// entity sends keys the client-go struct does not declare, read from the
	// captured response (ADR-0021, the readers in toolutil/sent_shapes.go).
	// A key appears twice where one type embeds the other, because the diff
	// sees the promoted field on both. Recorded in
	// docs/development/upstream-bugs.md.
	"groups.Output.allow_personal_snippets":                                 docGroupsList,
	"groups.Output.auto_duo_code_review_enabled":                            docGroupsList,
	"groups.Output.built_in_project_templates_enabled":                      docGroupsList,
	"groups.Output.duo_core_features_enabled":                               docGroupsList,
	"groups.Output.duo_namespace_access_rules":                              docGroupsList,
	"groups.Output.lock_built_in_project_templates_enabled":                 docGroupsList,
	"groups.Output.lock_resource_access_token_notify_inherited":             docGroupsList,
	"groups.Output.resource_access_token_notify_inherited":                  docGroupsList,
	"groups.Output.show_diff_preview_in_email":                              docGroupsList,
	"groups.Output.web_based_commit_signing_enabled":                        docGroupsList,
	"groups.DetailOutput.allow_personal_snippets":                           docGroupsGet,
	"groups.DetailOutput.auto_ban_user_on_excessive_projects_download":      docGroupsGet,
	"groups.DetailOutput.auto_duo_code_review_enabled":                      docGroupsGet,
	"groups.DetailOutput.built_in_project_templates_enabled":                docGroupsGet,
	"groups.DetailOutput.duo_core_features_enabled":                         docGroupsGet,
	"groups.DetailOutput.duo_namespace_access_rules":                        docGroupsGet,
	"groups.DetailOutput.lock_built_in_project_templates_enabled":           docGroupsGet,
	"groups.DetailOutput.lock_resource_access_token_notify_inherited":       docGroupsGet,
	"groups.DetailOutput.resource_access_token_notify_inherited":            docGroupsGet,
	"groups.DetailOutput.service_access_tokens_expiration_enforced":         docGroupsGet,
	"groups.DetailOutput.show_diff_preview_in_email":                        docGroupsGet,
	"groups.DetailOutput.step_up_auth_required_oauth_provider":              docGroupsGet,
	"groups.DetailOutput.unique_project_download_limit":                     docGroupsGet,
	"groups.DetailOutput.unique_project_download_limit_alertlist":           docGroupsGet,
	"groups.DetailOutput.unique_project_download_limit_allowlist":           docGroupsGet,
	"groups.DetailOutput.unique_project_download_limit_interval_in_seconds": docGroupsGet,
	"groups.DetailOutput.web_based_commit_signing_enabled":                  docGroupsGet,
	"groups.HookOutput.repository_update_events":                            docGroupHooks,
	"groups.HookOutput.duo_flow_callback_enabled":                           docGroupHooks,

	"issues.BasicOutput.blocking_issues_count": docIssuesList,
	"issues.BasicOutput.start_date":            docIssuesList,
	"issues.BasicOutput.type":                  docIssuesList,
	"issues.Output.blocking_issues_count":      docIssuesList,
	"issues.Output.epic_iid":                   docIssuesList,
	"issues.Output.has_tasks":                  docIssuesList,
	"issues.Output.imported":                   docIssuesList,
	"issues.Output.imported_from":              docIssuesList,
	"issues.Output.severity":                   docIssuesList,
	"issues.Output.start_date":                 docIssuesList,
	"issues.Output.task_status":                docIssuesList,
	"issues.Output.type":                       docIssuesList,

	"projects.Output.description_html":                             docProjectsGet,
	"projects.Output.duo_dependency_bump_breaking_changes_enabled": docProjectsGet,
	"projects.Output.duo_foundational_flows_enabled":               docProjectsGet,
	"projects.Output.duo_remote_flows_enabled":                     docProjectsGet,
	"projects.Output.duo_sast_fp_detection_enabled":                docProjectsGet,
	"projects.Output.duo_sast_vr_workflow_enabled":                 docProjectsGet,
	"projects.Output.duo_secret_detection_fp_enabled":              docProjectsGet,
	"projects.Output.max_pipelines_per_merge_train":                docProjectsGet,
	"projects.Output.merge_train_enforcement":                      docProjectsGet,
	"projects.Output.only_allow_merge_if_all_status_checks_passed": docProjectsGet,
	"projects.Output.repository_object_format":                     docProjectsGet,
	"projects.Output.secret_push_protection_enabled":               docProjectsGet,
	"projects.Output.security_policy_pipeline_must_succeed":        docProjectsGet,
	"projects.Output.show_diff_preview_in_email":                   docProjectsGet,
	"projects.Output.spp_repository_pipeline_access":               docProjectsGet,
	"projects.Output.warn_about_potentially_unwanted_characters":   docProjectsGet,
	"projects.Output.web_based_commit_signing_enabled":             docProjectsGet,
	"projects.Output.ci_skip_branch_pipelines_for_mrs":             docProjectsGet,
	"projects.HookOutput.duo_flow_callback_enabled":                docProjectHooks,
	"projects.ApprovalRuleOutput.coverage_minimum_threshold":       docMRApprovals,
	"projects.ProjectUserOutput.locked":                            docProjectUsersList,
	"projects.ProjectUserOutput.public_email":                      docProjectUsersList,

	// namespaces: the compute-minute usage ee/lib/ee/api/entities/namespace.rb
	// renders through ee/lib/api/entities/ci/minutes/usage.rb for an owner of
	// a top-level namespace, which client-go's Namespace does not model, read
	// from the captured response (ADR-0021, toolutil.CapturedNamespace).
	// Recorded in docs/development/upstream-bugs.md.
	"namespaces.Output.ci_minutes_usage": docNamespaces,

	// tokens — lib/api/entities/personal_access_token.rb exposes granular on
	// every token, and the entities inheriting it add granular_scopes and
	// last_used_ips under a condition; an impersonation token adds its own
	// flag and the description and user_id gl.ImpersonationToken does not
	// model, and a project or group token adds the resource pair of
	// lib/api/entities/resource_access_token.rb. All are read from the
	// captured response (ADR-0021, the token readers in
	// toolutil/sent_shapes.go). Recorded in
	// docs/development/upstream-bugs.md.
	"users.CurrentUserPATOutput.granular":              docPATList,
	"users.CurrentUserPATOutput.granular_scopes":       docPATList,
	"users.CurrentUserPATOutput.last_used_ips":         docPATList,
	"impersonationtokens.PATOutput.granular":           docPATList,
	"impersonationtokens.PATOutput.granular_scopes":    docPATList,
	"impersonationtokens.PATOutput.last_used_ips":      docPATList,
	"impersonationtokens.Output.granular":              docImpersonationPage,
	"impersonationtokens.Output.granular_scopes":       docImpersonationPage,
	"impersonationtokens.Output.last_used_ips":         docImpersonationPage,
	"impersonationtokens.Output.impersonation":         docImpersonationPage,
	"impersonationtokens.Output.description":           docImpersonationPage,
	"impersonationtokens.Output.user_id":               docImpersonationPage,
	"groupserviceaccounts.PATOutput.granular":          docServiceAccountPATs,
	"groupserviceaccounts.PATOutput.granular_scopes":   docServiceAccountPATs,
	"groupserviceaccounts.PATOutput.last_used_ips":     docServiceAccountPATs,
	"projectserviceaccounts.PATOutput.granular":        docServiceAccountPATs,
	"projectserviceaccounts.PATOutput.granular_scopes": docServiceAccountPATs,
	"projectserviceaccounts.PATOutput.last_used_ips":   docServiceAccountPATs,
	"accesstokens.Output.granular":                     docProjectTokensList,
	"accesstokens.Output.granular_scopes":              docProjectTokensList,
	"accesstokens.Output.last_used_ips":                docProjectTokensList,
	"accesstokens.Output.resource_type":                docGroupTokensList,
	"accesstokens.Output.resource_id":                  docGroupTokensList,

	// merge requests: what lib/api/entities/merge_request_basic.rb sends on
	// every merge request that neither gl.BasicMergeRequest nor gl.MergeRequest
	// declares, read from the captured response (ADR-0021,
	// toolutil.CapturedMergeRequest and CapturedMergeRequests). Four of them are
	// the older spelling of a key the entity also sends under a newer name and
	// GitLab keeps sending both. work_in_progress is here only on the types
	// paired with the lean struct: gl.MergeRequest declares it and
	// gl.BasicMergeRequest does not, so a list published the key and filled it
	// from nothing. The rendered pair is on toolutil.MergeRequestOutput alone,
	// which the two merge request packages alias, because it is the only shape
	// serving the one route that declares render_html. Recorded in
	// docs/development/upstream-bugs.md.
	"mergerequests.Output.approvals_before_merge":           docMergeRequests,
	"mergerequests.Output.merge_status":                     docMergeRequests,
	"mergerequests.Output.reference":                        docMergeRequests,
	"mergerequests.Output.title_html":                       docMergeRequestRender,
	"mergerequests.Output.description_html":                 docMergeRequestRender,
	"mergerequests.DependencyOutput.blocked_merge_request":  docMergeRequestBlocks,
	"deploymentmergerequests.Output.approvals_before_merge": docMergeRequests,
	"deploymentmergerequests.Output.merge_status":           docMergeRequests,
	"deploymentmergerequests.Output.reference":              docMergeRequests,
	"deploymentmergerequests.Output.title_html":             docMergeRequestRender,
	"deploymentmergerequests.Output.description_html":       docMergeRequestRender,
	"issues.RelatedMROutput.approvals_before_merge":         docMergeRequests,
	"issues.RelatedMROutput.merge_status":                   docMergeRequests,
	"issues.RelatedMROutput.reference":                      docMergeRequests,
	"issues.RelatedMROutput.work_in_progress":               docMergeRequests,

	// users: what GitLab's user entities send on a user that client-go's User
	// declares on no field of its own, read from the captured response
	// (ADR-0021, toolutil.CapturedUser and CapturedInstanceUser). The ten
	// UserPublic keys are on all four types that publish a user, since every
	// route filling them presents that entity. The five beside them are on
	// internal/tools/users alone, which is the only package serving the routes
	// that send them: bio_html on GET /users/:id, the licensed trio on the two
	// administrator routes, and unconfirmed_email on POST /service_accounts.
	// The three group-scoped types are held to those same five by the audit and
	// answered in cmd/audit_1to1/internal/paths/sent_declarations.go instead.
	// Recorded in docs/development/upstream-bugs.md.
	"users.Output.commit_email":                       docUsers,
	"users.Output.discord":                            docUsers,
	"users.Output.github":                             docUsers,
	"users.Output.local_time":                         docUsers,
	"users.Output.preferred_language":                 docUsers,
	"users.Output.pronouns":                           docUsers,
	"users.Output.work_information":                   docUsers,
	"users.Output.followers":                          docUsers,
	"users.Output.following":                          docUsers,
	"users.Output.is_followed":                        docUsers,
	"users.Output.bio_html":                           docUsers,
	"users.Output.unconfirmed_email":                  docServiceAccounts,
	"users.Output.enterprise_group_id":                docUsersEnterpriseGroup,
	"users.Output.enterprise_group_associated_at":     docUsersEnterpriseGroup,
	"users.Output.provisioned_by_group_id":            docUsers,
	"users.Output.provisioned_by_project_id":          docUsers,
	"enterpriseusers.Output.commit_email":             docUsers,
	"enterpriseusers.Output.discord":                  docUsers,
	"enterpriseusers.Output.github":                   docUsers,
	"enterpriseusers.Output.local_time":               docUsers,
	"enterpriseusers.Output.preferred_language":       docUsers,
	"enterpriseusers.Output.pronouns":                 docUsers,
	"enterpriseusers.Output.work_information":         docUsers,
	"enterpriseusers.Output.followers":                docUsers,
	"enterpriseusers.Output.following":                docUsers,
	"enterpriseusers.Output.is_followed":              docUsers,
	"groups.ProvisionedUserOutput.commit_email":       docUsers,
	"groups.ProvisionedUserOutput.discord":            docUsers,
	"groups.ProvisionedUserOutput.github":             docUsers,
	"groups.ProvisionedUserOutput.local_time":         docUsers,
	"groups.ProvisionedUserOutput.preferred_language": docUsers,
	"groups.ProvisionedUserOutput.pronouns":           docUsers,
	"groups.ProvisionedUserOutput.work_information":   docUsers,
	"groups.ProvisionedUserOutput.followers":          docUsers,
	"groups.ProvisionedUserOutput.following":          docUsers,
	"groups.ProvisionedUserOutput.is_followed":        docUsers,
	"groupsaml.SAMLUserOutput.commit_email":           docUsers,
	"groupsaml.SAMLUserOutput.discord":                docUsers,
	"groupsaml.SAMLUserOutput.github":                 docUsers,
	"groupsaml.SAMLUserOutput.local_time":             docUsers,
	"groupsaml.SAMLUserOutput.preferred_language":     docUsers,
	"groupsaml.SAMLUserOutput.pronouns":               docUsers,
	"groupsaml.SAMLUserOutput.work_information":       docUsers,
	"groupsaml.SAMLUserOutput.followers":              docUsers,
	"groupsaml.SAMLUserOutput.following":              docUsers,
	"groupsaml.SAMLUserOutput.is_followed":            docUsers,

	// issuelinks: what API::Entities::RelatedIssue sends on a related issue
	// that client-go's IssueRelation declares on no field of its own, read
	// from the captured response (ADR-0021, issuelinks.capturedRelations).
	// RelatedIssue inherits API::Entities::Issue, so nineteen of these are on
	// every response of the one route this type serves; the four licensed
	// ones and task_status arrive under their own condition. `subscribed` is
	// the one key of that entity this type does not publish, because the
	// route turns its presenter option off, and it is answered in
	// cmd/audit_1to1/internal/paths/sent_declarations.go instead. Recorded in
	// docs/development/upstream-bugs.md.
	"issuelinks.RelationOutput._links":                 docIssueLinksRelation,
	"issuelinks.RelationOutput.blocking_issues_count":  docIssueLinksRelation,
	"issuelinks.RelationOutput.closed_at":              docIssueLinksRelation,
	"issuelinks.RelationOutput.closed_by":              docIssueLinksRelation,
	"issuelinks.RelationOutput.discussion_locked":      docIssueLinksRelation,
	"issuelinks.RelationOutput.downvotes":              docIssueLinksRelation,
	"issuelinks.RelationOutput.epic":                   docIssueLinksEpic,
	"issuelinks.RelationOutput.epic_iid":               docIssueLinksEpic,
	"issuelinks.RelationOutput.has_tasks":              docIssueLinksRelation,
	"issuelinks.RelationOutput.health_status":          docIssueLinksLicensed,
	"issuelinks.RelationOutput.imported":               docIssueLinksRelation,
	"issuelinks.RelationOutput.imported_from":          docIssueLinksRelation,
	"issuelinks.RelationOutput.issue_type":             docIssueLinksRelation,
	"issuelinks.RelationOutput.iteration":              docIssueLinksLicensed,
	"issuelinks.RelationOutput.merge_requests_count":   docIssueLinksRelation,
	"issuelinks.RelationOutput.moved_to_id":            docIssueLinksRelation,
	"issuelinks.RelationOutput.service_desk_reply_to":  docIssueLinksRelation,
	"issuelinks.RelationOutput.severity":               docIssueLinksRelation,
	"issuelinks.RelationOutput.start_date":             docIssueLinksRelation,
	"issuelinks.RelationOutput.task_completion_status": docIssueLinksRelation,
	"issuelinks.RelationOutput.task_status":            docIssueLinksRelation,
	"issuelinks.RelationOutput.time_stats":             docIssueLinksRelation,
	"issuelinks.RelationOutput.type":                   docIssueLinksRelation,
	"issuelinks.RelationOutput.upvotes":                docIssueLinksRelation,

	// issuelinks: what API::Entities::IssueBasic sends on either issue of a
	// link that client-go's Issue declares on no field of its own, read from
	// the captured response of the get and create calls (ADR-0021,
	// issuelinks.capturedLink). lib/api/entities/issue_link.rb renders both
	// positions using IssueBasic, and doc/api/issue_links.md abbreviates the
	// two objects, so the citation is the page that prints the basic issue's
	// keys in full. Recorded in docs/development/upstream-bugs.md.
	"issuelinks.IssueRefOutput.blocking_issues_count": docIssuesList,
	"issuelinks.IssueRefOutput.start_date":            docIssuesList,
	"issuelinks.IssueRefOutput.type":                  docIssuesList,

	// memberroles: the twenty-five customizable permissions
	// API::Entities::MemberRole sends that client-go's MemberRole declares no
	// field for, read from the captured response (ADR-0021,
	// memberroles.capturedRole and capturedRoles). All twenty-five are on
	// every response of all four member role routes. Recorded in
	// docs/development/upstream-bugs.md.
	"memberroles.Output.admin_ai_catalog_item":           docMemberRolePermissions,
	"memberroles.Output.admin_ai_catalog_item_consumer":  docMemberRolePermissions,
	"memberroles.Output.admin_integrations":              docMemberRolePermissions,
	"memberroles.Output.admin_protected_branch":          docMemberRolePermissions,
	"memberroles.Output.admin_protected_environments":    docMemberRolePermissions,
	"memberroles.Output.admin_runners":                   docMemberRolePermissions,
	"memberroles.Output.admin_security_attributes":       docMemberRolePermissions,
	"memberroles.Output.apply_security_scan_profiles":    docMemberRolePermissions,
	"memberroles.Output.create_security_scan_profiles":   docMemberRolePermissions,
	"memberroles.Output.delete_security_scan_profiles":   docMemberRolePermissions,
	"memberroles.Output.destroy_package":                 docMemberRolePermissions,
	"memberroles.Output.read_admin_cicd":                 docMemberRolePermissions,
	"memberroles.Output.read_admin_groups":               docMemberRolePermissions,
	"memberroles.Output.read_admin_monitoring":           docMemberRolePermissions,
	"memberroles.Output.read_admin_projects":             docMemberRolePermissions,
	"memberroles.Output.read_admin_subscription":         docMemberRolePermissions,
	"memberroles.Output.read_admin_users":                docMemberRolePermissions,
	"memberroles.Output.read_agent_artifacts":            docMemberRolePermissions,
	"memberroles.Output.read_compliance_dashboard":       docMemberRolePermissions,
	"memberroles.Output.read_crm_contact":                docMemberRolePermissions,
	"memberroles.Output.read_security_attribute":         docMemberRolePermissions,
	"memberroles.Output.read_security_scan_profiles":     docMemberRolePermissions,
	"memberroles.Output.read_virtual_registry":           docMemberRolePermissions,
	"memberroles.Output.update_sec_ai_workflow_settings": docMemberRolePermissions,
	"memberroles.Output.update_security_scan_profiles":   docMemberRolePermissions,

	// pipelines: the twelve keys API::Entities::Ci::Pipeline adds to
	// API::Entities::Ci::PipelineBasic, which is what gl.PipelineInfo models.
	// Read from the captured response (ADR-0021, pipelines.CapturedOutput) on
	// the one route filling this type that presents the full entity, the
	// merge request pipeline creation in internal/tools/mergerequests. The two
	// pipeline lists present the basic entity and send none of them, which is
	// why every one carries omitempty. Recorded in
	// docs/development/upstream-bugs.md.
	"pipelines.Output.before_sha":      docMRCreatePipeline,
	"pipelines.Output.tag":             docMRCreatePipeline,
	"pipelines.Output.yaml_errors":     docMRCreatePipeline,
	"pipelines.Output.user":            docMRCreatePipeline,
	"pipelines.Output.started_at":      docMRCreatePipeline,
	"pipelines.Output.finished_at":     docMRCreatePipeline,
	"pipelines.Output.committed_at":    docMRCreatePipeline,
	"pipelines.Output.duration":        docMRCreatePipeline,
	"pipelines.Output.queued_duration": docMRCreatePipelineQueue,
	"pipelines.Output.coverage":        docMRCreatePipeline,
	"pipelines.Output.detailed_status": docMRCreatePipeline,
	"pipelines.Output.archived":        docMRCreatePipeline,

	// planlimits: the twenty-one limits and the change history
	// API::Entities::PlanLimit sends beside the eight package file sizes
	// client-go's PlanLimit models, read from the captured response (ADR-0021,
	// planlimits.capturedLimits) on both plan limit routes. Recorded in
	// docs/development/upstream-bugs.md.
	"planlimits.PlanLimitItem.cargo_max_file_size":                   docPlanLimits,
	"planlimits.PlanLimitItem.ci_instance_level_variables":           docPlanLimits,
	"planlimits.PlanLimitItem.ci_pipeline_size":                      docPlanLimits,
	"planlimits.PlanLimitItem.ci_active_jobs":                        docPlanLimits,
	"planlimits.PlanLimitItem.ci_project_subscriptions":              docPlanLimits,
	"planlimits.PlanLimitItem.ci_pipeline_schedules":                 docPlanLimits,
	"planlimits.PlanLimitItem.ci_needs_size_limit":                   docPlanLimits,
	"planlimits.PlanLimitItem.ci_registered_group_runners":           docPlanLimits,
	"planlimits.PlanLimitItem.ci_registered_project_runners":         docPlanLimits,
	"planlimits.PlanLimitItem.dotenv_variables":                      docPlanLimits,
	"planlimits.PlanLimitItem.dotenv_size":                           docPlanLimits,
	"planlimits.PlanLimitItem.enforcement_limit":                     docPlanLimits,
	"planlimits.PlanLimitItem.notification_limit":                    docPlanLimits,
	"planlimits.PlanLimitItem.storage_size_limit":                    docPlanLimits,
	"planlimits.PlanLimitItem.pipeline_hierarchy_size":               docPlanLimits,
	"planlimits.PlanLimitItem.max_pipelines_per_merge_train":         docPlanLimits,
	"planlimits.PlanLimitItem.service_desk_outbound_emails_per_hour": docPlanLimits,
	"planlimits.PlanLimitItem.service_desk_outbound_emails_per_day":  docPlanLimits,
	"planlimits.PlanLimitItem.web_hook_calls":                        docPlanLimits,
	"planlimits.PlanLimitItem.web_hook_calls_low":                    docPlanLimits,
	"planlimits.PlanLimitItem.web_hook_calls_mid":                    docPlanLimits,
	"planlimits.PlanLimitItem.limits_history":                        docPlanLimits,

	// orbit: the compact text GitLab answers the schema route with for the llm
	// response format, a key client-go's OrbitSchema does not model, read from
	// the captured response (ADR-0021, orbit.Schema). Recorded in
	// docs/development/upstream-bugs.md.
	"orbit.SchemaOutput.formatted_text": docOrbitSchemaText,
}}

// isDocAddedField reports whether an MCP output field is a doc-justified field we
// surface via raw-API fetch despite the SDK struct lacking it.
func (r *diffRun) isDocAddedField(pkg, mcpType, tag string) bool {
	return r.answer(docAddedFields, pkg+"."+mcpType+"."+tag)
}

// acceptedExtraOutputs adjudicates the remaining R-OUTPUT-EXTRA fields that are
// legitimate but are NOT raw-fetched REST fields (so they don't belong in
// docAddedFields). Each entry carries an explicit rationale. Two key forms:
//   - "<pkg>.<MCPType>"        — whole-type accept (every field of a GraphQL-sourced
//     output whose SDK struct carries no json tags, so a REST-tag diff flags all
//     fields; the data is real and documented by the GraphQL schema).
//   - "<pkg>.<MCPType>.<tag>"  — single-field accept (a server-composed/derived field
//     that is not an API field, or an SDK field the SDK struct leaves json-untagged).
//
// This makes the auditor's extra-output total fully explained: every accepted extra
// is here with a reason, so any NEW extra surfaces as a genuine finding.
var acceptedExtraOutputs = &declarationTable{name: "acceptedExtraOutputs", entries: map[string]string{
	// GraphQL-sourced output type: epics maps a GraphQL response struct
	// (gl.WorkItem) that carries no REST json tags, so the REST-tag diff flags
	// every documented GraphQL field as extra. The fields are real and documented
	// by the GraphQL schema (https://docs.gitlab.com/api/graphql/reference/).
	"epics.Output": "GraphQL work-item-era epic fields (gl.Epic/gl.WorkItem GraphQL structs); documented by the GraphQL schema",

	// Server-composed convenience fields (not API fields): we derive these for the
	// model, they are additive and intentional.
	"events.ContributionEventOutput.target_url":     "server-composed clickable URL via toolutil.BuildTargetURL; not an API field",
	"events.ProjectEventOutput.target_url":          "server-composed clickable URL via toolutil.BuildTargetURL; not an API field",
	"orbit.QueryOutput.formatted_text":              "the text/plain body GitLab answers POST /orbit/query with for the llm format (writeLLMResultResponse in workhorse/internal/orbit/sendquery.go), read through QueryRaw and published under the name the other Orbit llm answers use; a body with no keys has no struct field to pair with",
	"mergerequests.ApproveOutput.approved_by_count": "server-counted length of the approved_by list GitLab sends with an approve or an unapprove; not an API field",

	// SDK-sourced field the SDK leaves json-untagged: gl.Feature.Gates exists and is
	// the documented feature-flag `gates` array; the SDK struct field carries no json
	// tag so the REST-tag diff flags the snake_case output key as extra.
	"features.FeatureItem.gates": "documented feature-flag gates array, sourced from gl.Feature.Gates (SDK field is json-untagged)",

	// SDK-sourced value the SDK exposes as a method rather than a field:
	// gl.WorkItemSavedView.GID() renders the GraphQL global ID the saved view
	// mutations address the view by, so the model needs it even though the
	// field diff has nothing to pair it with.
	"workitemsavedviews.Item.gid": "GraphQL global ID from gl.WorkItemSavedView.GID(); the SDK exposes it as a method, so there is no field to pair with",
}}

// isAcceptedExtraOutput reports whether an extra MCP output field/type is an
// adjudicated legitimate extra (GraphQL-sourced, server-derived, or SDK-untagged),
// checking the whole-type key first then the per-field key.
//
// The order is also what decides which key a finding uses up: a per-field key
// beside a whole-type key for the same type is never reached, so it answers
// nothing and is reported stale rather than kept as a second copy of a
// decision the whole-type key already made.
func (r *diffRun) isAcceptedExtraOutput(pkg, mcpType, tag string) bool {
	return r.answer(acceptedExtraOutputs, pkg+"."+mcpType) ||
		r.answer(acceptedExtraOutputs, pkg+"."+mcpType+"."+tag)
}

// acceptedMissingInputs adjudicates R-INPUT "missing" fields that are legitimate
// and intentional — the SDK *Options field IS exposed and wired, but the auditor's
// tag diff cannot see it. Each entry carries an explicit rationale. Two key forms:
//   - "<pkg>.<MCPType>"        — whole-type accept (e.g. an input that deliberately
//     exposes only a curated subset of a very large SDK options struct).
//   - "<pkg>.<MCPType>.<tag>"  — single-field accept (deliberate json-key rename,
//     a param modeled on a nested object/slice element, a deprecated param the
//     current endpoint replaced, or an SDK options field the endpoint doesn't accept).
//
// This makes the auditor's missing-input total fully explained: every accepted
// miss is here with a reason, so any NEW genuine gap still surfaces.
var acceptedMissingInputs = &declarationTable{name: "acceptedMissingInputs", entries: map[string]string{
	// Deliberate json-key renames: the SDK param is exposed under a clearer/doc-correct
	// MCP key and wired to the SDK field.
	"branches.CreateInput.branch":                   "exposed as branch_name (wired to opts.Branch)",
	"branches.ProtectInput.name":                    "exposed as branch_name (wired to opts.Name)",
	"tags.ProtectTagInput.name":                     "exposed as tag_name (wired to opts.Name)",
	"epics.ListInput.include_ancestor_groups":       "exposed as include_ancestors (wired to opts.IncludeAncestorGroups)",
	"epics.ListInput.include_descendant_groups":     "exposed as include_descendants (wired to opts.IncludeDescendantGroups)",
	"epics.ListInput.labels":                        "exposed as label_name []string (wired to opts.Labels)",
	"groupmilestones.ListInput.include_descendents": "exposed as the doc-correct include_descendants (the SDK url tag has the include_descendents typo); wired to opts.IncludeDescendents",

	// Phantom widget inputs: gl.*WorkItemOptions is the options struct of every
	// work item type, and an Epic carries neither the STATUS, ITERATION nor
	// CRM_CONTACTS widget, so GitLab refuses each of these on an epic. The
	// widget list gitlab.com answered on 2026-09-07 for
	// namespace(fullPath: "gitlab-org") { workItemTypes { nodes { name widgetDefinitions { type } } } }
	// gives Epic AI_SESSION, ASSIGNEES, AWARD_EMOJI, COLOR, CURRENT_USER_TODOS,
	// CUSTOM_FIELDS, DESCRIPTION, HEALTH_STATUS, HIERARCHY, LABELS,
	// LINKED_ITEMS, MILESTONE, NOTES, NOTIFICATIONS, PARTICIPANTS,
	// START_AND_DUE_DATE, TIME_TRACKING, VERIFICATION_STATUS and WEIGHT, while
	// Issue and Task carry the other three. internal/tools/workitems exposes
	// them, because there the type is the caller's to choose.
	"epics.CreateInput.status":              epicPhantomWidget,
	"epics.CreateInput.iteration_id":        epicPhantomWidget,
	"epics.CreateInput.crm_contact_ids":     epicPhantomWidget,
	"epics.UpdateInput.status":              epicPhantomWidget,
	"epics.UpdateInput.iteration_id":        epicPhantomWidget,
	"epics.UpdateInput.crm_contact_ids":     epicPhantomWidget,
	"epics.ListInput.iteration_id":          epicPhantomWidget,
	"epics.ListInput.iteration_cadence_id":  epicPhantomWidget,
	"epics.ListInput.iteration_wildcard_id": epicPhantomWidget,
	"epics.ListInput.crm_contact_id":        epicPhantomWidget,
	"epics.ListInput.crm_organization_id":   epicPhantomWidget,

	// The release filters are left out for the other reason: Group.workItems
	// declares both, so they are accepted here, and what an epic lacks is a
	// release to be filtered by rather than the widget.
	"epics.ListInput.release_tag":             epicReleaseFilter,
	"epics.ListInput.release_tag_wildcard_id": epicReleaseFilter,

	// Work item options this action pins rather than publishes.
	"epics.ListInput.types":           "pinned to EPIC: this action lists epics, and the type is what makes it that action rather than workitems.list",
	"epics.ListInput.returned_fields": "pinned to the fragment the epic output maps; a caller choosing it could select status or iteration, widgets an Epic does not carry",

	// Deprecated params the current endpoint replaced.
	"grouplabels.DeleteInput.name": "deprecated DELETE /groups/:id/labels name param; current endpoint uses label_id in path",
	"grouplabels.UpdateInput.name": "deprecated PUT /groups/:id/labels name param; current endpoint uses label_id in path",

	// Project params client-go still models and GitLab no longer declares:
	// neither POST /projects nor PUT /projects/:id carries them in the live
	// record (gitlab-api-live.json, 19.4.1-ee), and lib/api/helpers/
	// projects_helpers.rb at v19.3.1-ee names none of them. Grape drops an
	// undeclared param, so a value offered here would be sent and ignored.
	// automatic_rebase_enabled is not among them: PUT /projects/:id declares
	// it from 19.4, and project.update offers it.
	"projects.CreateInput.build_coverage_regex":    projectParamGitLabDropped,
	"projects.CreateInput.operations_access_level": projectParamGitLabDropped,
	"projects.UpdateInput.build_coverage_regex":    projectParamGitLabDropped,
	"projects.UpdateInput.operations_access_level": projectParamGitLabDropped,

	// A file upload offered in the shape every upload here takes rather than
	// as the SDK's *GraphQLUpload: avatar_filename and avatar_content_type
	// with either avatar_file_path or avatar_content_base64 (AvatarInput),
	// which the handler assembles into opts.Avatar.
	"achievements.CreateInput.avatar": achievementAvatarUpload,
	"achievements.UpdateInput.avatar": achievementAvatarUpload,

	// SDK options fields the endpoint does not accept (generic ListOptions plumbing).
	"groupsshcerts.ListInput.order_by": "group SSH certificates list accepts only id+pagination; gl.ListOptions ordering is unused plumbing",
	"groupsshcerts.ListInput.sort":     "group SSH certificates list accepts only id+pagination; gl.ListOptions ordering is unused plumbing",
	// The merge request approval rules list pages through a gl.ListOptions the
	// handler's own raw request encodes, and GET /projects/:id/merge_requests/
	// :merge_request_iid/approval_rules declares page and per_page and nothing
	// else of it (gitlab-api-live.json, 19.4.1-ee), so neither the ordering nor
	// the keyset fields would reach anything.
	"mrapprovals.RulesInput.order_by":   mrApprovalRulesOffsetOnly,
	"mrapprovals.RulesInput.sort":       mrApprovalRulesOffsetOnly,
	"mrapprovals.RulesInput.pagination": mrApprovalRulesOffsetOnly,
	"mrapprovals.RulesInput.page_token": mrApprovalRulesOffsetOnly,

	// SDK options fields the route declares and never reads. The deployment
	// merge request list takes merge_requests_base_params and presents
	// `Entities::MergeRequestBasic, current_user: current_user` whatever the
	// request said (lib/api/deployments.rb): the three are turned into
	// presenter options only by serializer_options_for in
	// lib/api/merge_requests.rb, which this route does not call, and
	// MergeRequestsFinder reads none of them. Offering them would promise a
	// model label objects, a simple view or a skipped recheck it cannot get.
	"deploymentmergerequests.ListInput.view":                      deploymentMergeRequestsInert,
	"deploymentmergerequests.ListInput.with_labels_details":       deploymentMergeRequestsInert,
	"deploymentmergerequests.ListInput.with_merge_status_recheck": deploymentMergeRequestsInert,
	// An SDK options field GitLab discards: app/models/user.rb ignores the
	// users.skype column since 18.4 and lib/api/users.rb declares no skype
	// param on POST /users or PUT /users/:id, so Grape drops the key. Recorded
	// in docs/development/upstream-bugs.md.
	"users.CreateInput.skype": skypeDiscarded,
	"users.ModifyInput.skype": skypeDiscarded,

	// Params modeled on a nested object / slice element per the full-nested-object
	// policy (the auditor flattens the SDK nested options into the parent input).
	"snippets.ProjectCreateInput.file_path":           "modeled on the nested files[] object (CreateFileInput.FilePath)",
	"snippets.ProjectUpdateInput.action":              "modeled on the nested files[] object (UpdateFileInput.Action)",
	"snippets.ProjectUpdateInput.file_path":           "modeled on the nested files[] object (UpdateFileInput.FilePath)",
	"snippets.ProjectUpdateInput.previous_path":       "modeled on the nested files[] object (UpdateFileInput.PreviousPath)",
	"releaselinks.CreateBatchInput.name":              "modeled on the links[] slice element (LinkEntry.Name); auditor does not recurse named slice types",
	"releaselinks.CreateBatchInput.url":               "modeled on the links[] slice element (LinkEntry.URL)",
	"releaselinks.CreateBatchInput.direct_asset_path": "modeled on the links[] slice element (LinkEntry.DirectAssetPath)",
	"releaselinks.CreateBatchInput.filepath":          "modeled on the links[] slice element (LinkEntry.FilePath, deprecated alias)",
	"releaselinks.CreateBatchInput.link_type":         "modeled on the links[] slice element (LinkEntry.LinkType)",

	// Whole-type curated subset: override_params accepts the full CreateProjectOptions
	// set (~79 fields); the import tool exposes the commonly-overridden subset — full
	// project configuration is available via the dedicated gitlab_project create/update
	// tools. (topics/tag_list are also excluded due to an SDK multipart []string bug.)
	"projectimportexport.ImportOverrideParamsInput": "override_params curated subset; full project config via gitlab_project create/update tools",
}}

// isAcceptedMissingInput reports whether an input pair's missing field is an
// adjudicated legitimate omission, checking the whole-type key first then
// per-field, with the same consequence for a shadowed per-field key that
// isAcceptedExtraOutput describes.
func (r *diffRun) isAcceptedMissingInput(pkg, mcpType, tag string) bool {
	return r.answer(acceptedMissingInputs, pkg+"."+mcpType) ||
		r.answer(acceptedMissingInputs, pkg+"."+mcpType+"."+tag)
}

// gap is one diffed MCP↔SDK struct pair under a package.
type gap struct {
	Kind           string         `json:"kind"` // "input" or "output"
	MCPType        string         `json:"mcp_type"`
	SDKType        string         `json:"sdk_type"`
	MissingFields  []missingField `json:"missing_fields,omitempty"`
	TypeMismatches []typeMismatch `json:"type_mismatches,omitempty"`
	// ExtraFields lists MCP output json tags with no SDK counterpart — invented
	// output scalars the 1:1 rule forbids (R-OUTPUT-EXTRA). Populated only for
	// output pairs; input pairs legitimately carry non-Options params (path ids).
	ExtraFields []extraField `json:"extra_fields,omitempty"`
}

type missingField struct {
	Tag     string `json:"tag"`
	SDKType string `json:"sdk_type"`
}

type typeMismatch struct {
	Tag     string `json:"tag"`
	MCPType string `json:"mcp_type"`
	SDKType string `json:"sdk_type"`
}

// extraField is one MCP output json tag with no SDK result counterpart.
type extraField struct {
	Tag     string `json:"tag"`
	MCPType string `json:"mcp_type"`
}

// envelopeKeys is the MCP-envelope carve-out per the 1:1 policy: resource output
// must mirror the SDK result, but MCP-protocol / structural keys (pagination
// envelope and LLM next-step hints) are legitimately additive and are therefore
// exempt from R-OUTPUT-EXTRA. A json tag of "-" is also exempt (not serialized).
var envelopeKeys = map[string]struct{}{
	"pagination":  {},
	"page":        {},
	"per_page":    {},
	"total":       {},
	"total_pages": {},
	"next_page":   {},
	"prev_page":   {},
	"next_steps":  {},
}

// packageReport aggregates the gap pairs for one internal/tools package.
type packageReport struct {
	Package            string `json:"package"`
	InputPairs         int    `json:"input_pairs"`
	OutputPairs        int    `json:"output_pairs"`
	MissingInputCount  int    `json:"missing_input_count"`
	MissingOutputCount int    `json:"missing_output_count"`
	ExtraOutputCount   int    `json:"extra_output_count"`
	Gaps               []gap  `json:"gaps"`
}

// hasFindings reports whether the package carries at least one finding of any
// of the three classes, which is what -gaps-only keeps. It is a method of its
// own because the repository is the only other input the filter ever sees,
// and a tree whose input rows are all answered cannot say whether the input
// count is read at all.
func (pr packageReport) hasFindings() bool {
	return pr.MissingInputCount != 0 || pr.MissingOutputCount != 0 || pr.ExtraOutputCount != 0
}

// report is the JSON document written to the output path.
type report struct {
	SchemaVersion int           `json:"schema_version"`
	ClientGoPath  string        `json:"client_go_path"`
	Summary       reportSummary `json:"summary"`
	// StaleDeclarations lists every declaration table key that answered no
	// candidate finding of the run. It is reported whatever -gaps-only says,
	// since it is a finding rather than a package with none.
	StaleDeclarations []staleDeclaration `json:"stale_declarations,omitempty"`
	Packages          []packageReport    `json:"packages"`
}

type reportSummary struct {
	Packages            int `json:"packages"`
	PackagesWithGaps    int `json:"packages_with_gaps"`
	InputPairs          int `json:"input_pairs"`
	OutputPairs         int `json:"output_pairs"`
	MissingInputFields  int `json:"missing_input_fields"`
	MissingOutputFields int `json:"missing_output_fields"`
	ExtraOutputFields   int `json:"extra_output_fields"`
	TypeMismatches      int `json:"type_mismatches"`
	StaleDeclarations   int `json:"stale_declarations"`
}

// marshalIndent is the JSON encoder, a variable so a test can reach the
// encoding failure branch that a report of strings and ints never produces.
var marshalIndent = json.MarshalIndent

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline). gapsOnly filters to entries with at
// least one finding, matching the original -gaps-only flag.
func Run(root string, gapsOnly bool) ([]byte, error) {
	rep, err := buildReport(root, gapsOnly)
	if err != nil {
		return nil, err
	}
	content, err := marshalIndent(rep, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), nil
}

func buildReport(root string, gapsOnly bool) (report, error) {
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		return report{}, err
	}
	run := newDiffRun()
	reports := make([]packageReport, 0, len(pkgs))
	for _, pkg := range pkgs {
		pr, ok := run.analyzePackage(pkg)
		if !ok {
			continue
		}
		if gapsOnly && !pr.hasFindings() {
			continue
		}
		reports = append(reports, pr)
	}
	slices.SortFunc(reports, func(a, b packageReport) int { return strings.Compare(a.Package, b.Package) })
	// Judged after the loop and over every package, including the clean ones
	// -gaps-only is about to drop from the report: a declaration that answers a
	// candidate in a package with no finding left is doing its job.
	stale := run.staleDeclarations(declarationTables)
	summary := summarize(reports)
	summary.StaleDeclarations = len(stale)
	return report{
		SchemaVersion:     shared.SchemaVersion,
		ClientGoPath:      shared.ClientGoPkgPath,
		Summary:           summary,
		StaleDeclarations: stale,
		Packages:          reports,
	}, nil
}

// Pair is one (MCP struct, SDK struct) pairing the field diff runs over,
// exported for the enum rule, which walks the same pairs and asks a different
// question of their fields: not whether a field exists on both sides, but
// whether the values a client-go enum type declares are the values we offer.
type Pair struct {
	// Kind is "input" (an MCP input struct against the SDK Options struct its
	// handler constructs) or "output" (an MCP output struct against the SDK
	// result struct its converter reads).
	Kind string
	// MCPName and MCPType are the MCP-side struct.
	MCPName string
	MCPType *types.Struct
	// MCPNamed is the named type behind MCPName once an alias is unwrapped.
	// For a local struct it lives in the tool package; for an alias such as
	// `type Output = toolutil.MergeRequestOutput` it is the target type, in
	// the target's package, which is the identity reflect reports for it.
	MCPNamed *types.Named
	// SDKName and SDKType are the client-go struct, the name qualified with
	// the last segment of its package path.
	SDKName string
	SDKType *types.Struct
	// SDKURLTags is true for an Options struct, whose fields are named by
	// their url tag first and json tag second.
	SDKURLTags bool
}

// CollectPairs returns every pairing analyzePackage diffs for one tool
// package, phantom inputs already dropped, inputs before outputs and each in
// (MCP name, SDK name) order.
func CollectPairs(pkg *packages.Package) []Pair {
	inputPairs, outputPairs := discoverPairs(pkg)
	out := make([]Pair, 0, len(inputPairs)+len(outputPairs))
	for _, pair := range sortedPairs(inputPairs) {
		if disjointPhantomInput(pair, inputPairs) {
			continue
		}
		out = append(out, pair.exported("input"))
	}
	for _, pair := range sortedPairs(outputPairs) {
		out = append(out, pair.exported("output"))
	}
	return out
}

// discoverPairs walks every function declaration of pkg and records the
// converter-derived output pairs and the handler-derived input pairs.
func discoverPairs(pkg *packages.Package) (inputPairs, outputPairs map[[2]string]structPair) {
	inputPairs = map[[2]string]structPair{}
	outputPairs = map[[2]string]structPair{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			collectConverter(pkg, fn, outputPairs)
			collectHandlerInputs(pkg, fn, inputPairs)
		}
	}
	return inputPairs, outputPairs
}

func (r *diffRun) analyzePackage(pkg *packages.Package) (packageReport, bool) {
	inputPairs, outputPairs := discoverPairs(pkg)
	if len(inputPairs) == 0 && len(outputPairs) == 0 {
		return packageReport{}, false
	}
	pr := packageReport{Package: shortPackage(pkg.PkgPath)}
	for _, pair := range sortedPairs(inputPairs) {
		// FIX B: drop phantom input pairings where an Options composite-literal
		// built inside a secondary helper lookup (e.g. validatePosition building
		// ListMergeRequestDiffsOptions) is mis-attributed to an unrelated input
		// struct. See disjointPhantomInput for the exact (disjoint-and-not-sole)
		// rule that suppresses these without dropping genuine single-pairing gaps.
		if disjointPhantomInput(pair, inputPairs) {
			continue
		}
		pr.InputPairs++
		g := r.diffPair(pr.Package, "input", pair)
		pr.MissingInputCount += len(g.MissingFields)
		appendGapIfAny(&pr, g)
	}
	// FIX A: an MCP output type may be produced by MULTIPLE converters, each
	// pairing it to a DIFFERENT SDK result struct (e.g. mergerequests Output is
	// built from both BasicMergeRequest and the fuller MergeRequest). Diffing each
	// pairing independently falsely reports MergeRequest-only fields as EXTRA when
	// diffed vs the leaner BasicMergeRequest (and symmetrically MISSING). Group
	// output pairs by MCP type and diff once against the UNION of all paired SDK
	// structs so a field is MISSING/EXTRA only when absent from EVERY pairing.
	for _, group := range outputGroups(outputPairs) {
		pr.OutputPairs += len(group.pairs)
		g := r.diffOutputGroup(pr.Package, group)
		pr.MissingOutputCount += len(g.MissingFields)
		pr.ExtraOutputCount += len(g.ExtraFields)
		appendGapIfAny(&pr, g)
	}
	return pr, true
}

// outputGroup is the set of (MCP output, SDK result) pairs that share the same
// MCP output type name. Diffing is done once per group against the union of the
// SDK field maps (FIX A: union multi-converter pairings).
type outputGroup struct {
	mcpName string
	mcpType *types.Struct
	pairs   []structPair
}

// outputGroups buckets output pairs by their MCP type name and returns the
// groups in deterministic (mcpName) order. The MCP struct is identical across a
// group's pairs (same converter return type), so the first pair's struct is
// used as the canonical MCP side.
func outputGroups(pairs map[[2]string]structPair) []outputGroup {
	byName := map[string]*outputGroup{}
	for _, pair := range sortedPairs(pairs) {
		grp, ok := byName[pair.mcpName]
		if !ok {
			grp = &outputGroup{mcpName: pair.mcpName, mcpType: pair.mcpType}
			byName[pair.mcpName] = grp
		}
		grp.pairs = append(grp.pairs, pair)
	}
	out := make([]outputGroup, 0, len(byName))
	for _, grp := range byName {
		out = append(out, *grp)
	}
	slices.SortFunc(out, func(a, b outputGroup) int { return strings.Compare(a.mcpName, b.mcpName) })
	return out
}

// diffOutputGroup diffs one MCP output type against the UNION of every SDK
// result struct paired to it. A json tag is MISSING only when absent from the
// union of all paired SDK fields, and EXTRA only when absent from the union
// (after shared.NormalizeSDKTag + envelope carve-out + accepted-rename allowlist). A
// TypeMismatch is reported for a tag only when the MCP type is incompatible with
// the SDK type in EVERY pairing that carries that tag, so a field that is
// compatible in at least one pairing is not double-flagged.
func (r *diffRun) diffOutputGroup(pkg string, group outputGroup) gap {
	mcpFields := flattenFields(group.mcpType, []string{tagKeyJSON})

	// unionSDK maps each SDK json tag to one representative SDK type string (used
	// for the missing-field SDKType label). sdkTypesByTag collects every SDK type
	// seen for a tag so a mismatch is reported only when ALL are incompatible.
	unionSDK := map[string]string{}
	sdkTypesByTag := map[string][]string{}
	sdkNames := make([]string, 0, len(group.pairs))
	for _, pair := range group.pairs {
		sdkNames = append(sdkNames, pair.sdkName)
		for tag, sdkType := range flattenFields(pair.sdkType, []string{tagKeyJSON}) {
			if _, ok := unionSDK[tag]; !ok {
				unionSDK[tag] = sdkType
			}
			sdkTypesByTag[tag] = append(sdkTypesByTag[tag], sdkType)
		}
	}
	sort.Strings(sdkNames)

	g := gap{Kind: "output", MCPType: group.mcpName, SDKType: strings.Join(sdkNames, "|")}
	tags := make([]string, 0, len(unionSDK))
	for tag := range unionSDK {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		mcpType, present := mcpFields[tag]
		if !present {
			if alt, ok := mcpFields[shared.NormalizeSDKTag(tag)]; ok {
				mcpType, present = alt, true
			}
		}
		if !present {
			// Doc-grounded omissions intentionally drop SDK fields the endpoint does
			// not return (the cited API doc is the 1:1 ground truth): whole nested
			// reference subsets, and individual top-level fields.
			if !r.isCuratedRefSubset(pkg, group.mcpName) && !r.isDocOmittedField(pkg, group.mcpName, tag) {
				g.MissingFields = append(g.MissingFields, missingField{Tag: tag, SDKType: unionSDK[tag]})
			}
			continue
		}
		// Report a mismatch only when the MCP type is incompatible with the SDK
		// type in EVERY pairing that carries this tag.
		if sdk, ok := allIncompatible(mcpType, sdkTypesByTag[tag]); ok {
			g.TypeMismatches = append(g.TypeMismatches, typeMismatch{Tag: tag, MCPType: mcpType, SDKType: sdk})
		}
	}
	g.ExtraFields = r.extraOutputFields(pkg, group.mcpName, mcpFields, unionSDK)
	return g
}

// allIncompatible reports whether mcpType is incompatible with EVERY SDK type in
// sdkTypes, returning a representative incompatible SDK type for the report. If
// the tag is compatible with at least one pairing it is not flagged (ok=false).
func allIncompatible(mcpType string, sdkTypes []string) (string, bool) {
	if len(sdkTypes) == 0 {
		return "", false
	}
	for _, sdk := range sdkTypes {
		if typesCompatible(mcpType, sdk) {
			return "", false
		}
	}
	return sdkTypes[0], true
}

func appendGapIfAny(pr *packageReport, g gap) {
	if len(g.MissingFields) > 0 || len(g.TypeMismatches) > 0 || len(g.ExtraFields) > 0 {
		pr.Gaps = append(pr.Gaps, g)
	}
}

// structPair links an MCP struct with the SDK struct it must mirror.
type structPair struct {
	mcpName  string
	mcpType  *types.Struct
	mcpNamed *types.Named
	sdkName  string
	sdkType  *types.Struct
	// tag preference for the SDK side: url first for Options, json for results.
	sdkURLTags bool
}

// exported renders the pair in its exported form.
func (p structPair) exported(kind string) Pair {
	return Pair{
		Kind:    kind,
		MCPName: p.mcpName, MCPType: p.mcpType, MCPNamed: p.mcpNamed,
		SDKName: p.sdkName, SDKType: p.sdkType,
		SDKURLTags: p.sdkURLTags,
	}
}

func sortedPairs(pairs map[[2]string]structPair) []structPair {
	out := make([]structPair, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, pair)
	}
	slices.SortFunc(out, func(a, b structPair) int {
		return cmp.Or(strings.Compare(a.mcpName, b.mcpName), strings.Compare(a.sdkName, b.sdkName))
	})
	return out
}

// collectConverter detects func(...src *gl.Y...) LocalStruct converters and
// records the (MCP output, SDK result) pair.
func collectConverter(pkg *packages.Package, fn *ast.FuncDecl, out map[[2]string]structPair) {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return
	}
	resultExpr := fn.Type.Results.List[0].Type
	resultType := pkg.TypesInfo.TypeOf(resultExpr)
	mcpNamed, mcpStruct, mcpName, ok := localOrAliasNamedStruct(pkg, resultExpr, resultType)
	if !ok {
		return
	}
	var sdkNamed *types.Named
	var sdkStruct *types.Struct
	sdkCount := 0
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			named, st, isSDK := clientGoNamedStruct(pkg.TypesInfo.TypeOf(field.Type))
			if !isSDK {
				continue
			}
			// Exclude non-result SDK arguments so the output pair is not formed
			// against the wrong struct (R-OUTPUT-EXTRA false positives):
			//   - the pagination Response wrapper of list converters (the data
			//     slice would look "extra");
			//   - *Options request structs and time value types (ISOTime/time.Time)
			//     a converter may also accept alongside no genuine result struct.
			if nonResultSDKStruct(named) {
				continue
			}
			sdkNamed, sdkStruct = named, st
			sdkCount++
		}
	}
	// When no genuine result struct remains after exclusion (e.g. a converter that
	// only took a Response wrapper or an *Options/time arg), skip the pair rather
	// than mis-pairing the MCP output against a non-result SDK struct.
	if sdkCount != 1 {
		return
	}
	// Skip unexported converter result types: they are internal mapping
	// intermediates (e.g. commits.commitFields, a shared struct embedded into the
	// real exported Output/DetailOutput), not serialized MCP output structs. Their
	// fields carry no json tags, so pairing them against the SDK result would flag
	// every SDK field as missing. Real MCP output structs are always exported.
	//
	// An empty name needs no check of its own: ast.IsExported("") is false, so
	// the name a resolver could not read is skipped by this same line.
	if !ast.IsExported(mcpName) {
		return
	}
	key := [2]string{mcpName, sdkNamed.Obj().Name()}
	out[key] = structPair{
		mcpName: mcpName, mcpType: mcpStruct, mcpNamed: mcpNamed,
		sdkName: sdkTypeName(sdkNamed), sdkType: sdkStruct, sdkURLTags: false,
	}
}

// collectHandlerInputs attributes every &gl.XxxOptions{} literal inside fn to
// fn's MCP input struct.
func collectHandlerInputs(pkg *packages.Package, fn *ast.FuncDecl, out map[[2]string]structPair) {
	if fn.Body == nil {
		return
	}
	mcpNamed, mcpStruct, ok := handlerInputStruct(pkg, fn)
	if !ok {
		return
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit {
			return true
		}
		named, st, isSDK := clientGoNamedStruct(pkg.TypesInfo.TypeOf(lit))
		if !isSDK || !strings.HasSuffix(named.Obj().Name(), optionsSuffix) {
			return true
		}
		key := [2]string{mcpNamed.Obj().Name(), named.Obj().Name()}
		out[key] = structPair{
			mcpName: mcpNamed.Obj().Name(), mcpType: mcpStruct, mcpNamed: mcpNamed,
			sdkName: sdkTypeName(named), sdkType: st, sdkURLTags: true,
		}
		return true
	})
}

// handlerInputStruct returns the first parameter whose type is a struct named
// in the handler's own package (the typed MCP input).
func handlerInputStruct(pkg *packages.Package, fn *ast.FuncDecl) (*types.Named, *types.Struct, bool) {
	if fn.Type.Params == nil {
		return nil, nil, false
	}
	for _, field := range fn.Type.Params.List {
		named, st, ok := localNamedStruct(pkg, pkg.TypesInfo.TypeOf(field.Type))
		if ok {
			return named, st, true
		}
	}
	return nil, nil, false
}

// diffPair diffs one input pair (the MCP handler input struct against the SDK
// Options struct its handler constructs). Output pairs are diffed by
// diffOutputGroup against the union of their SDK structs (FIX A); diffPair is
// input-only. SDK fields absent from the MCP struct are MISSING (R-INPUT);
// fields present but type-divergent are advisory TypeMismatches.
func (r *diffRun) diffPair(pkg, kind string, pair structPair) gap {
	mcpFields := flattenFields(pair.mcpType, []string{tagKeyJSON})
	sdkTagKeys := []string{tagKeyJSON}
	if pair.sdkURLTags {
		sdkTagKeys = []string{tagKeyURL, tagKeyJSON}
	}
	sdkFields := flattenFields(pair.sdkType, sdkTagKeys)

	g := gap{Kind: kind, MCPType: pair.mcpName, SDKType: pair.sdkName}
	tags := make([]string, 0, len(sdkFields))
	for tag := range sdkFields {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		sdkType := sdkFields[tag]
		mcpType, present := mcpFields[tag]
		if !present {
			// SDK url tags use array/negation notation (iids[], not[author_id])
			// that maps to snake_case MCP json names (iids, not_author_id).
			if alt, ok := mcpFields[shared.NormalizeSDKTag(tag)]; ok {
				mcpType, present = alt, true
			}
		}
		if !present {
			// Adjudicated legitimate input omission (deliberate rename, nested-object
			// modeling, deprecated/non-accepted param, or curated subset) — each
			// carries a rationale in acceptedMissingInputs.
			if kind == "input" && r.isAcceptedMissingInput(pkg, pair.mcpName, tag) {
				continue
			}
			g.MissingFields = append(g.MissingFields, missingField{Tag: tag, SDKType: sdkType})
			continue
		}
		if !typesCompatible(mcpType, sdkType) {
			g.TypeMismatches = append(g.TypeMismatches, typeMismatch{Tag: tag, MCPType: mcpType, SDKType: sdkType})
		}
	}
	return g
}

// inputPairTags returns the comparable tag sets for an input pair: the MCP
// struct's json tags and the SDK Options' tags (url-first, normalized to the
// snake_case form the MCP side uses). Used by disjointPhantomInput to measure
// field overlap.
//
// Neither set filters the unnamed and the excluded tag, because flattenFields
// keys nothing under either: it drops both before recording a field, and its
// untagged fallback keys by a Go field name, which can be neither. A second
// copy of that rule here would be a filter nothing can observe, so what holds
// the property is a test of the producer rather than a check of the consumer.
func inputPairTags(pair structPair) (mcp, sdk map[string]struct{}) {
	mcp = map[string]struct{}{}
	for tag := range flattenFields(pair.mcpType, []string{tagKeyJSON}) {
		mcp[tag] = struct{}{}
	}
	sdk = map[string]struct{}{}
	sdkKeys := []string{tagKeyJSON}
	if pair.sdkURLTags {
		sdkKeys = []string{tagKeyURL, tagKeyJSON}
	}
	for tag := range flattenFields(pair.sdkType, sdkKeys) {
		sdk[shared.NormalizeSDKTag(tag)] = struct{}{}
	}
	return mcp, sdk
}

// inputPairsOverlap reports whether the MCP and SDK tag sets of an input pair
// share at least one field name (after SDK-tag normalization).
func inputPairsOverlap(pair structPair) bool {
	mcp, sdk := inputPairTags(pair)
	for tag := range sdk {
		if _, ok := mcp[tag]; ok {
			return true
		}
	}
	return false
}

// disjointPhantomInput reports whether an input pairing is a phantom produced by
// an Options composite-literal that some secondary helper builds for an
// unrelated lookup, then mis-attributes to this MCP input struct.
//
// FIX B: the auditor attributes every &gl.XxxOptions{} literal in a function
// body to that function's local input struct. A helper such as
// mrdiscussions/mrdraftnotes validatePosition takes a *DiffPosition (which
// mirrors gl.PositionOptions) yet internally builds gl.ListMergeRequestDiffsOptions
// for a diff lookup, so DiffPosition is falsely diffed against the list options
// (phantom missing order_by/page/per_page/sort/...).
//
// The rule is deliberately narrow — disjoint-AND-not-sole — so it never drops a
// genuine single-pairing R-INPUT candidate (e.g. groups GetInput, whose only
// pairing is the path-id-only input vs the all-query GetGroupOptions): an input
// pairing is a phantom only when
//
//   - the MCP struct's tags share ZERO overlap with this SDK Options' tags, AND
//   - the SAME MCP input struct has at least one OTHER Options pairing that DOES
//     overlap (its genuine request options, e.g. DiffPosition↔PositionOptions).
//
// When the disjoint pairing is the input struct's only pairing it is kept, since
// a path-id-only input legitimately pairs against an options struct it does not
// surface and that remains a candidate gap a human adjudicates.
func disjointPhantomInput(pair structPair, all map[[2]string]structPair) bool {
	if inputPairsOverlap(pair) {
		return false
	}
	for _, other := range all {
		if other.mcpName != pair.mcpName || other.sdkName == pair.sdkName {
			continue
		}
		if inputPairsOverlap(other) {
			return true
		}
	}
	return false
}

// extraOutputFields reports MCP output json tags with no SDK result counterpart:
// invented output scalars the 1:1 rule forbids (R-OUTPUT-EXTRA). An MCP tag is
// extra when it is neither a key of sdkFields nor the shared.NormalizeSDKTag image of
// any SDK key, is not the MCP-envelope carve-out, is not the "-" sentinel, and
// is not an allowlisted deliberate rename (per the 1:1 data-fidelity policy).
func (r *diffRun) extraOutputFields(pkg, mcpType string, mcpFields, sdkFields map[string]string) []extraField {
	sdkNorm := make(map[string]struct{}, len(sdkFields))
	for sdkTag := range sdkFields {
		sdkNorm[shared.NormalizeSDKTag(sdkTag)] = struct{}{}
	}
	tags := make([]string, 0, len(mcpFields))
	for tag := range mcpFields {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	var extras []extraField
	for _, tag := range tags {
		if tag == "-" {
			continue
		}
		if _, exempt := envelopeKeys[tag]; exempt {
			continue
		}
		if _, ok := sdkFields[tag]; ok {
			continue
		}
		if _, ok := sdkNorm[tag]; ok {
			continue
		}
		if r.isAcceptedRename(pkg, mcpType, tag) {
			continue
		}
		// Documented field surfaced via raw-API fetch (SDK struct lacks it): not
		// invented — the official API doc returns it.
		if r.isDocAddedField(pkg, mcpType, tag) {
			continue
		}
		// Adjudicated legitimate extra (GraphQL-sourced type, server-derived field,
		// or SDK-untagged field) — each carries a rationale in acceptedExtraOutputs.
		if r.isAcceptedExtraOutput(pkg, mcpType, tag) {
			continue
		}
		extras = append(extras, extraField{Tag: tag, MCPType: mcpFields[tag]})
	}
	return extras
}

// maxEmbedDepth bounds the embedded-struct recursion of both walks: six levels
// of embedding is the deepest a field is still read from.
const maxEmbedDepth = 6

// encoder is the serializer whose output a flattened struct describes.
//
// Where several fields of one struct meet under one name, the two a client-go
// struct passes through resolve it differently, and a flattened name is
// labeled with the type of the field that is written, which is what the diff
// compares and what decides a type mismatch. Keeping the first field a walk
// met instead labeled an Options struct's sort and order_by with the plain
// string of the embedded ListOptions, declared ahead of the struct's own
// field, rather than with the field a handler sets.
type encoder int

const (
	// encodingJSON is encoding/json: of the fields sharing a name the
	// shallowest is written, a tagged one beats an untagged one at the same
	// depth, and any other tie at that depth writes none of them.
	encodingJSON encoder = iota
	// encodingQuery is go-querystring, which client-go builds a request's
	// query from an Options struct with. It writes every field it meets, a
	// struct's own fields before an embedded struct's, so the shallowest is the
	// one a handler sets and labels the name, and a tie at that depth keeps the
	// first declared rather than dropping a parameter the query still carries.
	encodingQuery
)

// encoderFor is the encoder a tag preference describes: url first is an
// Options struct read the way its query is built, anything else is json.
//
// An Options struct a request sends as a JSON body is written by encoding/json
// instead. The two differ in two places: a tie at the shallowest depth, which
// encoding/json drops and this keeps as the first declared, and a shallower
// untagged field of the name, which hides a deeper tagged one from
// encoding/json and hides nothing from a query. Both come to the same thing:
// the query reading keeps a key the JSON body might leave out, never the
// reverse, and labels a key both keep with the same field. The difference
// therefore reports a candidate for a person to adjudicate rather than hiding
// one.
func encoderFor(tagKeys []string) encoder {
	if len(tagKeys) > 0 && tagKeys[0] == tagKeyURL {
		return encodingQuery
	}
	return encodingJSON
}

// fieldCandidate is one field a walk met that an encoder could write under
// name: the type the diff labels it with, how deep in the embedding it sits,
// whether a tag named it, and whether the walk keys it at all.
//
// A field that is met without being keyed is an untagged field of a struct
// that tags others. The tag walk keys none of those, by design (see
// flattenFields), but encoding/json still writes one under its Go name, so it
// can still be the field that is written under a name some deeper tagged field
// spells alike, and then that deeper field is not written at all.
type fieldCandidate struct {
	name   string
	typ    string
	depth  int
	tagged bool
	keyed  bool
}

// flattenFields walks a struct (recursing into embedded structs) and returns a
// map of tag-name → type string for every field carrying one of the tag keys,
// each name labeled with the field the encoder the keys describe would write.
func flattenFields(st *types.Struct, tagKeys []string) map[string]string {
	enc := encoderFor(tagKeys)
	var met []fieldCandidate
	if flattenInto(st, tagKeys, &met, 0) {
		return promote(met, enc)
	}
	// A struct that carries no tag of the kind we index by cannot be compared
	// by tag at all: every field on our side is then reported as one the SDK
	// does not have, which says nothing about either. client-go's Achievement
	// and UserAchievement are exactly that, and they produced the only 18
	// entries left in the backlog while our field names matched theirs one for
	// one. encoding/json serializes such a field under its own name, so that is
	// what the comparison uses.
	//
	// The fallback is per struct rather than per field on purpose. An untagged
	// field in a struct that tags the rest is also serialized by its name, but
	// changing that case would re-open every gap somebody has already
	// adjudicated, for a shape nothing has yet been found in.
	met = met[:0]
	flattenNamesInto(st, &met, 0)
	return promote(met, enc)
}

// flattenNamesInto is flattenInto keyed by the Go field name, for a struct that
// tags nothing: every exported field is met, keyed and untagged, and an
// embedded struct is descended into whether or not its type is exported,
// since encoding/json promotes an unexported embed's exported fields.
func flattenNamesInto(st *types.Struct, met *[]fieldCandidate, depth int) {
	if st == nil || depth > maxEmbedDepth {
		return
	}
	for field := range st.Fields() {
		if field.Embedded() {
			if embedded, ok := structUnder(field.Type()); ok {
				flattenNamesInto(embedded, met, depth+1)
				continue
			}
		}
		if !field.Exported() {
			continue
		}
		*met = append(*met, fieldCandidate{
			name:  shared.FieldNameTag(field.Name()),
			typ:   types.TypeString(field.Type(), shortQualifier),
			depth: depth,
			keyed: true,
		})
	}
}

// flattenInto collects every field of st and of the untagged embeds under it
// that an encoder could write, and reports whether any of them carries one of
// the tag keys, which is what decides whether the struct is compared by tag at
// all.
//
// A tagged field is met under its tag and keyed. An exported untagged field is
// met under its Go name without being keyed, since encoding/json writes it and
// it can hide a deeper field of that name. An unexported field that is not an
// embed, and one tagged "-", are written by no encoder and not met at all.
func flattenInto(st *types.Struct, tagKeys []string, met *[]fieldCandidate, depth int) bool {
	if st == nil || depth > maxEmbedDepth {
		return false
	}
	tagged := false
	for i := range st.NumFields() {
		field := st.Field(i)
		tagName := shared.TagName(reflect.StructTag(st.Tag(i)), tagKeys)
		if field.Embedded() && tagName == "" {
			if embedded, ok := structUnder(field.Type()); ok {
				tagged = flattenInto(embedded, tagKeys, met, depth+1) || tagged
				continue
			}
		}
		if tagName == "-" || !field.Exported() {
			continue
		}
		name := tagName
		if name == "" {
			name = field.Name()
		}
		*met = append(*met, fieldCandidate{
			name:   name,
			typ:    types.TypeString(field.Type(), shortQualifier),
			depth:  depth,
			tagged: tagName != "",
			keyed:  tagName != "",
		})
		tagged = tagged || tagName != ""
	}
	return tagged
}

// promote keeps, for every name the walk met, the field enc writes under it,
// and returns the keyed ones labeled with their types.
//
// go-querystring lets no field hide another, so an unkeyed field does not take
// part under encodingQuery; under encodingJSON it does, and a name it wins is
// left out, since the field written there is one the walk keys nothing for.
func promote(met []fieldCandidate, enc encoder) map[string]string {
	byName := map[string][]fieldCandidate{}
	for _, candidate := range met {
		if enc == encodingQuery && !candidate.keyed {
			continue
		}
		byName[candidate.name] = append(byName[candidate.name], candidate)
	}
	out := make(map[string]string, len(byName))
	for name, group := range byName {
		if written, ok := dominant(group, enc); ok && written.keyed {
			out[name] = written.typ
		}
	}
	return out
}

// dominant is the field enc writes of a group sharing one name, in the order
// the walk met them, and false when it writes none.
//
// The group is ordered shallowest first and, at one depth, tagged first, with
// the walk's order kept otherwise; that is encoding/json's own ordering, and
// under go-querystring, where every field is tagged, it is the order the query
// is written in. encoding/json then writes the first unless the second sits at
// the same depth with the same tagging, which it drops as ambiguous.
func dominant(group []fieldCandidate, enc encoder) (fieldCandidate, bool) {
	slices.SortStableFunc(group, byDepthThenTag)
	first := group[0]
	if enc == encodingJSON && len(group) > 1 && group[1].depth == first.depth && group[1].tagged == first.tagged {
		return fieldCandidate{}, false
	}
	return first, true
}

// byDepthThenTag orders two candidates shallowest first and, at one depth, a
// tagged one before an untagged one.
func byDepthThenTag(a, b fieldCandidate) int {
	if byDepth := cmp.Compare(a.depth, b.depth); byDepth != 0 {
		return byDepth
	}
	switch {
	case a.tagged == b.tagged:
		return 0
	case a.tagged:
		return -1
	default:
		return 1
	}
}

// typesCompatible reports whether an MCP field type acceptably represents an
// SDK field type. Pointer-ness is ignored (optional inputs stay pointers in the
// SDK); known scalar projections (enum Value types → int/string, time types →
// string) are treated as compatible because the domain maps them deliberately.
func typesCompatible(mcpType, sdkType string) bool {
	mcp := normalizeType(mcpType)
	sdk := normalizeType(sdkType)
	if mcp == sdk {
		return true
	}
	if scalarLike(mcp) && scalarLike(sdk) {
		return true
	}
	// SDK enum/value types projected to scalars.
	if strings.HasSuffix(sdk, "value") && (mcp == "int" || mcp == typNameString || mcp == typNameInt64) {
		return true
	}
	if sdkTimeLike(sdk) && (mcp == typNameString || mcp == typNameInt64) {
		return true
	}
	// SDK label collections (LabelOptions/Labels, both defined as []string) are
	// represented as []string on the MCP side.
	if mcp == "[]string" && (strings.HasSuffix(sdk, "labeloptions") || strings.HasSuffix(sdk, "labels")) {
		return true
	}
	return false
}

// sdkTimeLike reports whether a normalized SDK type is one of the client-go
// time representations (ISOTime, time.Time) that the domain renders as strings.
func sdkTimeLike(sdk string) bool {
	return sdk == "time" || sdk == "time.time" || strings.HasSuffix(sdk, "isotime")
}

func normalizeType(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "*")
	return s
}

func scalarLike(s string) bool {
	switch s {
	case "int", typNameInt64, "int32", "float64", "float32", "bool", typNameString:
		return true
	default:
		return false
	}
}

// localNamedStruct returns the named struct if t (deref'd) is a struct declared
// in pkg itself.
func localNamedStruct(pkg *packages.Package, t types.Type) (*types.Named, *types.Struct, bool) {
	named, st, ok := derefNamedStruct(t)
	if !ok {
		return nil, nil, false
	}
	if named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != pkg.PkgPath {
		return nil, nil, false
	}
	return named, st, true
}

// localOrAliasNamedStruct resolves the converter result type to its underlying
// struct and the name to attribute the MCP output pair under. It accepts two
// shapes:
//
//   - a struct named in the converter's own package (the original case), or
//   - a struct reached through a LOCAL alias declared in that package, e.g.
//     `type Output = toolutil.MergeRequestOutput`. Shared output shapes were
//     lifted into internal/toolutil to remove duplication; without following
//     the alias the converter would be dropped and the type would silently fall
//     out of 1:1 audit coverage. The pair is attributed under the local alias
//     name (e.g. "Output"), keeping per-package accept-list keys stable.
//
// resultExpr is the AST result-type expression (used to detect the alias);
// resultType is its resolved go/types type (the alias target).
func localOrAliasNamedStruct(pkg *packages.Package, resultExpr ast.Expr, resultType types.Type) (*types.Named, *types.Struct, string, bool) {
	if named, st, ok := localNamedStruct(pkg, resultType); ok {
		return named, st, named.Obj().Name(), true
	}
	ident := identForExpr(resultExpr)
	if ident == nil {
		return nil, nil, "", false
	}
	aliasObj, isTypeName := pkg.TypesInfo.Uses[ident].(*types.TypeName)
	if !isTypeName || !aliasObj.IsAlias() ||
		aliasObj.Pkg() == nil || aliasObj.Pkg().Path() != pkg.PkgPath {
		return nil, nil, "", false
	}
	// Go materializes type aliases as *types.Alias; unwrap to the target named
	// struct (e.g. Output -> toolutil.MergeRequestOutput) before reading fields.
	// types.Unalias only unwraps a top-level alias, so a *Alias pointer result
	// (the thin-wrapper converter shape, e.g. *MemberUserOutput) must be
	// dereferenced first or the alias inside the pointer stays opaque and the
	// pair silently falls out of audit coverage.
	target := resultType
	if ptr, isPtr := target.(*types.Pointer); isPtr {
		target = ptr.Elem()
	}
	named, st, ok := derefNamedStruct(types.Unalias(target))
	if !ok {
		return nil, nil, "", false
	}
	// The alias keeps its local name for the report, and the target type
	// travels beside it: a rule that has to meet reflect on the same identity
	// (the enum rule's catalog index) keys on the target, not on the alias.
	return named, st, aliasObj.Name(), true
}

// identForExpr returns the identifier naming the type in a result expression,
// unwrapping a single pointer (so both `Output` and `*Output` resolve to the
// `Output` identifier). It returns nil for any other expression shape.
func identForExpr(expr ast.Expr) *ast.Ident {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident
	}
	return nil
}

// clientGoNamedStruct returns the named struct if t (deref'd) is a struct in the
// client-go SDK module.
func clientGoNamedStruct(t types.Type) (*types.Named, *types.Struct, bool) {
	named, st, ok := derefNamedStruct(t)
	if !ok {
		return nil, nil, false
	}
	if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), shared.ClientGoPkgPath) {
		return nil, nil, false
	}
	return named, st, true
}

// nonResultSDKStruct reports whether a client-go named struct is NOT a result
// type and must therefore be excluded when forming an OUTPUT pair. This covers
// three false-positive classes the converter param scan would otherwise pair
// against:
//
//   - the pagination Response wrapper (named "Response"): list converters take
//     it so the MCP list-output's element slice looks "extra";
//   - *Options request structs (name ends in "Options"): request inputs, not
//     results, sometimes accepted by GraphQL converters;
//   - time value types (ISOTime, time.Time, anything in the `time` package):
//     scalar value args, not result structs.
func nonResultSDKStruct(named *types.Named) bool {
	obj := named.Obj()
	if nonResultStructName(obj.Name()) {
		return true
	}
	if pkg := obj.Pkg(); pkg != nil && pkg.Path() == "time" {
		return true
	}
	return false
}

// nonResultStructName reports whether a bare struct name identifies a non-result
// SDK type (pagination wrapper, *Options request struct, or a time value type).
// Split out from nonResultSDKStruct so the name-based rule is unit-testable
// without synthesizing go/types objects.
func nonResultStructName(name string) bool {
	switch {
	case name == responseStructName:
		return true
	case strings.HasSuffix(name, optionsSuffix):
		return true
	case name == "ISOTime" || name == "Time":
		return true
	default:
		return false
	}
}

func derefNamedStruct(t types.Type) (*types.Named, *types.Struct, bool) {
	if t == nil {
		return nil, nil, false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil, nil, false
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, nil, false
	}
	return named, st, true
}

func structUnder(t types.Type) (*types.Struct, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		t = named.Underlying()
	}
	st, ok := t.(*types.Struct)
	return st, ok
}

func sdkTypeName(named *types.Named) string {
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return named.Obj().Name()
	}
	return lastPathSegment(pkg.Path()) + "." + named.Obj().Name()
}

func shortQualifier(pkg *types.Package) string {
	return lastPathSegment(pkg.Path())
}

func lastPathSegment(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func shortPackage(pkgPath string) string {
	_, after, ok := strings.Cut(pkgPath, shared.ToolsPkgInfix)
	if !ok {
		return lastPathSegment(pkgPath)
	}
	return after
}

func summarize(reports []packageReport) reportSummary {
	s := reportSummary{Packages: len(reports)}
	for _, pr := range reports {
		if pr.MissingInputCount > 0 || pr.MissingOutputCount > 0 || pr.ExtraOutputCount > 0 {
			s.PackagesWithGaps++
		}
		s.InputPairs += pr.InputPairs
		s.OutputPairs += pr.OutputPairs
		s.MissingInputFields += pr.MissingInputCount
		s.MissingOutputFields += pr.MissingOutputCount
		s.ExtraOutputFields += pr.ExtraOutputCount
		for _, g := range pr.Gaps {
			s.TypeMismatches += len(g.TypeMismatches)
		}
	}
	return s
}
