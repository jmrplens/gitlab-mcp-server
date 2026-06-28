// Package structs verifies the 1:1 field mapping between the
// MCP tool input/output structs and the client-go SDK Options/result structs
// they wrap.
//
// For every package under internal/tools it resolves, with full Go type
// information:
//
//   - input pairs: each &gl.XxxOptions{} composite literal constructed inside a
//     handler is attributed to that handler's MCP input struct. The SDK Options
//     fields (by url/json tag) are diffed against the MCP input fields (by json
//     tag) to find SDK fields with no MCP counterpart (R-INPUT) and advisory
//     Go-type divergences.
//   - output pairs: each converter func(...src *gl.Y...) LocalStruct maps an SDK
//     result struct to an MCP output struct. The SDK result fields (by json tag)
//     are diffed against the MCP output fields (R-OUTPUT).
//
// The report is the mechanical backlog that drives the 1:1 audit batches. It is
// intentionally high-signal on *missing fields* (the gap class that the
// client-go bumps repeatedly introduced) and advisory on type divergences,
// because the domain legitimately maps SDK enum/time types onto scalars.
package structs

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	schemaVersion   = 1
	clientGoPkgPath = "gitlab.com/gitlab-org/api/client-go"
	toolsPkgInfix   = "/internal/tools/"
	optionsSuffix   = "Options"
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
	docCommitSignature  = "commits.md#get-the-signature-of-a-commit"
	docPipelineTriggers = "pipeline_triggers.md"
	docMRApprovals      = "merge_request_approvals.md"
	tagKeyJSON          = "json"
	typNameString       = "string"
	typNameInt64        = "int64"
)

// acceptedOutputRenames suppresses specific MCP output json tags from
// R-OUTPUT-EXTRA: deliberate 1:1 renames of a genuine scalar (1:1 means data
// fidelity, not byte-identical keys). It is intentionally conservative — only
// true scalar renames belong here, never flattened-duplication tags (those are
// genuine findings, e.g. branches `commit_id` flattening `commit.id`).
//
// Two forms are supported:
//   - "MCPType.tag": the rename is suppressed only for that MCP output type.
//   - "tag":         the rename is suppressed for any MCP output type (global).
//
// Add accepted renames here (with a one-line rationale) as the per-package
// audit adjudicates them.
var acceptedOutputRenames = map[string]bool{
	// branches Output renames the SDK `name` scalar to `branch_name` to match the
	// `branch_name` input parameter naming used across the branches tools.
	"Output.branch_name": true,
}

// isAcceptedRename reports whether (mcpType, tag) is an allowlisted deliberate
// rename, checking the type-scoped key first then the global tag key.
func isAcceptedRename(mcpType, tag string) bool {
	if acceptedOutputRenames[mcpType+"."+tag] {
		return true
	}
	return acceptedOutputRenames[tag]
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
var curatedRefSubsets = map[string]string{
	// Populated per package as outputs are reconciled to the official API docs
	// (https://gitlab.com/gitlab-org/gitlab/-/raw/master/doc/api/<file>).
	//
	// environments — nested objects of the "Retrieve an environment" response
	// (doc/api/environments.md#retrieve-an-environment).
	"environments.ClusterAgentOutput":       docEnvRetrieve,
	"environments.ConfigProjectOutput":      docEnvRetrieve,
	"environments.DeploymentOutput":         docEnvRetrieve,
	"environments.DeploymentUserOutput":     docEnvRetrieve,
	"environments.DeployableOutput":         docEnvRetrieve,
	"environments.DeployableUserOutput":     docEnvRetrieve,
	"environments.DeployableCommitOutput":   docEnvRetrieve,
	"environments.DeployablePipelineOutput": docEnvRetrieve,
	"environments.DeployableRunnerOutput":   docEnvRetrieve,

	// jobs — nested objects of the job response (doc/api/jobs.md#get-a-single-job).
	"jobs.CommitObject":       docJobsSingle,
	"jobs.RunnerObject":       docJobsSingle,
	"jobs.UserObject":         docJobsSingle,
	"jobs.ProjectObject":      docJobsSingle,
	"jobs.PipelineObject":     docJobsSingle,
	"jobs.PipelineInfoObject": "jobs.md#list-pipeline-trigger-jobs",

	// boards — nested objects of the board / board-list responses (doc/api/boards.md).
	"boards.ProjectOutput":           docBoards,
	"boards.MilestoneOutput":         docBoards,
	"boards.BasicUserOutput":         docBoards,
	"boards.LabelOutput":             docBoards,
	"boards.LabelDetailsOutput":      docBoards,
	"boards.BoardListAssigneeOutput": docBoards,
	"boards.IterationOutput":         docBoards,

	// deployments — nested objects of the deployment response (doc/api/deployments.md).
	"deployments.UserOutput":               docDeployments,
	"deployments.EnvironmentOutput":        docDeployments,
	"deployments.DeployableOutput":         docDeployments,
	"deployments.DeployableUserOutput":     docDeployments,
	"deployments.DeployableCommitOutput":   docDeployments,
	"deployments.DeployablePipelineOutput": docDeployments,
	"deployments.DeployableRunnerOutput":   docDeployments,

	// pipelineschedules — nested objects of the schedule response (doc/api/pipeline_schedules.md).
	"pipelineschedules.OwnerOutput":             docPipelineSched,
	"pipelineschedules.LastPipelineOutput":      docPipelineSched,
	"pipelineschedules.VariableObject":          docPipelineSched,
	"pipelineschedules.TriggeredPipelineOutput": docPipelineSched,

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
	"groupboards.GroupRefOutput":     docGroupBoards,
	"groupboards.MilestoneOutput":    docGroupBoards,
	"groupboards.BasicUserOutput":    docGroupBoards,
	"groupboards.LabelDetailsOutput": docGroupBoards,
	"groupboards.LabelOutput":        docGroupBoards,

	// groupepicboards — nested refs of the epic-board responses (doc/api/group_epic_boards.md).
	"groupepicboards.GroupRefOutput":     docGroupEpicBoards,
	"groupepicboards.ListLabelOutput":    docGroupEpicBoards,
	"groupepicboards.LabelDetailsOutput": docGroupEpicBoards,
	"groupepicboards.BoardListOutput":    docGroupEpicBoards,

	// pipelinetriggers — owner/user are documented identity subsets of gl.User
	// (doc/api/pipeline_triggers.md).
	"pipelinetriggers.UserOutput":           docPipelineTriggers,
	"pipelinetriggers.BasicUserOutput":      docPipelineTriggers,
	"pipelinetriggers.DetailedStatusOutput": docPipelineTriggers,

	// releases / groupreleases — author and commit are documented subsets
	// (doc/api/releases/_index.md, doc/api/group_releases.md).
	"releases.AuthorOutput":      "releases/_index.md",
	"releases.CommitOutput":      "releases/_index.md",
	"groupreleases.AuthorOutput": "group_releases.md",
	"groupreleases.CommitOutput": "group_releases.md",

	// tags — the tag's nested commit is a documented identity subset (doc/api/tags.md).
	"tags.CommitOutput": "tags.md",
}

// isCuratedRefSubset reports whether the MCP output type (scoped by package) is a
// doc-justified reference subset whose omitted-vs-full-SDK fields are not flagged.
func isCuratedRefSubset(pkg, mcpType string) bool {
	_, ok := curatedRefSubsets[pkg+"."+mcpType]
	return ok
}

// docOmittedFields lists individual top-level SDK result fields the official API
// doc does NOT include in the documented response for that endpoint, so the MCP
// output intentionally omits them: the doc is the 1:1 ground truth (we expose what
// the endpoint returns, not every field of the SDK struct). Unlike
// curatedRefSubsets (a whole nested reference type), this is per-field on a primary
// output type. Each entry cites the doc/api/<file>. Key = "<pkg>.<MCP type>.<tag>".
var docOmittedFields = map[string]string{
	// environments: the environment response documents no nested `project` object
	// (environments are queried within a project, so it would be redundant);
	// gl.Environment.Project is never populated in the documented response.
	"environments.Output.project": "environments.md (list/get/create/update response)",
	// jobs: the job response nests pipeline.id; there is no top-level pipeline_id
	// (the old MCP pipeline_id was a flattened convenience scalar, now removed).
	"jobs.Output.pipeline_id": docJobsSingle,
}

// isDocOmittedField reports whether an SDK field is a doc-justified intentional
// omission on a primary MCP output type.
func isDocOmittedField(pkg, mcpType, tag string) bool {
	_, ok := docOmittedFields[pkg+"."+mcpType+"."+tag]
	return ok
}

// docAddedFields is the symmetric carve-out to docOmittedFields: documented API
// response fields the client-go SDK struct does NOT expose, which we surface via a
// raw-REST/GraphQL fetch (client.GL().NewRequest+Do into a superset struct, per
// ADR-0006). Because the SDK struct lacks them, they would otherwise be flagged as
// R-OUTPUT-EXTRA ("invented"); they are NOT invented — the official API doc returns
// them. Each entry cites the doc/api/<file>. Key = "<pkg>.<MCPType>.<tag>". These
// fields carry `omitempty` so they degrade gracefully on older GitLab versions that
// don't return them.
var docAddedFields = map[string]string{
	// jobs — documented in doc/api/jobs.md but absent from gl.Job / gl.JobRunner;
	// fetched via raw REST (rawGetJob/rawListJobs into the jobAPI superset).
	"jobs.Output.archived":          docJobsSingle,
	"jobs.Output.source":            docJobsSingle,
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

	// pipelineschedules — variables[].raw is documented on the single-schedule
	// response but absent from gl.PipelineVariable; fetched via raw REST
	// (rawGetSchedule into the rawScheduleAPI superset).
	"pipelineschedules.VariableObject.raw": docPipelineSched,

	// mrapprovals — approval rule `overridden` is documented (approval_state/list/
	// create/update rule responses) but absent from gl.MergeRequestApprovalRule;
	// fetched via raw REST (rawApprovalState/rawListApprovalRules/rawMutateApprovalRule).
	"mrapprovals.RuleOutput.overridden": docMRApprovals,

	// groupboards — documented in doc/api/group_boards.md but absent from
	// gl.GroupIssueBoard; fetched via raw REST (rawListGroupBoards/rawGetGroupBoard/
	// rawCreateGroupBoard/rawUpdateGroupBoard into the groupIssueBoardAPI superset).
	"groupboards.GroupBoardOutput.hide_backlog_list": docGroupBoards,
	"groupboards.GroupBoardOutput.hide_closed_list":  docGroupBoards,
	"groupboards.GroupBoardOutput.assignee":          docGroupBoards,
	"groupboards.GroupBoardOutput.weight":            docGroupBoards,

	// groupepicboards — documented in doc/api/group_epic_boards.md but absent from
	// gl.GroupEpicBoard / gl.BoardList / gl.LabelDetails; fetched via raw REST superset.
	"groupepicboards.Output.hide_backlog_list":      docGroupEpicBoards,
	"groupepicboards.Output.hide_closed_list":       docGroupEpicBoards,
	"groupepicboards.LabelDetailsOutput.title":      docGroupEpicBoards,
	"groupepicboards.LabelDetailsOutput.group_id":   docGroupEpicBoards,
	"groupepicboards.LabelDetailsOutput.project_id": docGroupEpicBoards,
	"groupepicboards.LabelDetailsOutput.template":   docGroupEpicBoards,
	"groupepicboards.LabelDetailsOutput.created_at": docGroupEpicBoards,
	"groupepicboards.LabelDetailsOutput.updated_at": docGroupEpicBoards,
	"groupepicboards.BoardListOutput.list_type":     docGroupEpicBoards,
	"groupepicboards.BoardListOutput.collapsed":     docGroupEpicBoards,

	// commits — the commit signature endpoint documents SSH/X.509 signature fields
	// absent from gl.GPGSignature; fetched via raw REST (rawGetGPGSignature into the
	// gpgSignatureAPI superset). Citation: commits.md#get-the-signature-of-a-commit.
	"commits.GPGSignatureOutput.signature_type":   docCommitSignature,
	"commits.GPGSignatureOutput.commit_source":    docCommitSignature,
	"commits.GPGSignatureOutput.key":              docCommitSignature,
	"commits.GPGSignatureOutput.x509_certificate": docCommitSignature,

	// projectimportexport — the import-status response documents `created_at`, but the
	// SDK gl.ImportStatus tags its timestamp `create_at` (upstream typo); we surface the
	// documented `created_at` via a raw-decode superset (importStatusAPI).
	"projectimportexport.ImportStatusOutput.created_at": "project_import_export.md",
}

// isDocAddedField reports whether an MCP output field is a doc-justified field we
// surface via raw-API fetch despite the SDK struct lacking it.
func isDocAddedField(pkg, mcpType, tag string) bool {
	_, ok := docAddedFields[pkg+"."+mcpType+"."+tag]
	return ok
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
var acceptedExtraOutputs = map[string]string{
	// GraphQL-sourced output types: workitems/epics map GraphQL response structs
	// (gl.WorkItem/gl.Epic) that carry no REST json tags, so the REST-tag diff flags
	// every documented GraphQL field as extra. The fields are real and documented by
	// the GraphQL schema (https://docs.gitlab.com/api/graphql/reference/).
	"workitems.WorkItemItem": "GraphQL-sourced work item fields (gl.WorkItem GraphQL struct has no REST json tags); documented by the GraphQL schema",
	"epics.Output":           "GraphQL work-item-era epic fields (gl.Epic/gl.WorkItem GraphQL structs); documented by the GraphQL schema",

	// Server-composed convenience fields (not API fields): we derive these for the
	// model, they are additive and intentional.
	"events.ContributionEventOutput.target_url": "server-composed clickable URL via toolutil.BuildTargetURL; not an API field",
	"events.ProjectEventOutput.target_url":      "server-composed clickable URL via toolutil.BuildTargetURL; not an API field",
	"orbit.QueryOutput.formatted_text":          "server-formatted convenience rendering of the Orbit query result; not an API field",

	// SDK-sourced field the SDK leaves json-untagged: gl.Feature.Gates exists and is
	// the documented feature-flag `gates` array; the SDK struct field carries no json
	// tag so the REST-tag diff flags the snake_case output key as extra.
	"features.FeatureItem.gates": "documented feature-flag gates array, sourced from gl.Feature.Gates (SDK field is json-untagged)",
}

// isAcceptedExtraOutput reports whether an extra MCP output field/type is an
// adjudicated legitimate extra (GraphQL-sourced, server-derived, or SDK-untagged),
// checking the whole-type key first then the per-field key.
func isAcceptedExtraOutput(pkg, mcpType, tag string) bool {
	if _, ok := acceptedExtraOutputs[pkg+"."+mcpType]; ok {
		return true
	}
	_, ok := acceptedExtraOutputs[pkg+"."+mcpType+"."+tag]
	return ok
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
var acceptedMissingInputs = map[string]string{
	// Deliberate json-key renames: the SDK param is exposed under a clearer/doc-correct
	// MCP key and wired to the SDK field.
	"branches.CreateInput.branch":                    "exposed as branch_name (wired to opts.Branch)",
	"branches.ProtectInput.name":                     "exposed as branch_name (wired to opts.Name)",
	"tags.ProtectTagInput.name":                      "exposed as tag_name (wired to opts.Name)",
	"epics.ListInput.include_ancestor_groups":        "exposed as include_ancestors (wired to opts.IncludeAncestorGroups)",
	"epics.ListInput.include_descendant_groups":      "exposed as include_descendants (wired to opts.IncludeDescendantGroups)",
	"epics.ListInput.labels":                         "exposed as label_name []string (wired to opts.Labels)",
	"workitems.ListWorkItemTypesInput.onlyAvailable": "exposed as only_available (snake_case of the SDK camelCase tag)",
	"groupmilestones.ListInput.include_descendents":  "exposed as the doc-correct include_descendants (the SDK url tag has the include_descendents typo); wired to opts.IncludeDescendents",

	// Deprecated params the current endpoint replaced.
	"grouplabels.DeleteInput.name": "deprecated DELETE /groups/:id/labels name param; current endpoint uses label_id in path",
	"grouplabels.UpdateInput.name": "deprecated PUT /groups/:id/labels name param; current endpoint uses label_id in path",

	// SDK options fields the endpoint does not accept (generic ListOptions plumbing).
	"groupsshcerts.ListInput.order_by": "group SSH certificates list accepts only id+pagination; gl.ListOptions ordering is unused plumbing",
	"groupsshcerts.ListInput.sort":     "group SSH certificates list accepts only id+pagination; gl.ListOptions ordering is unused plumbing",

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

	// Param present on all public create inputs; the auditor flagged it on an
	// unexported helper struct the &gl.Options{} literal is attributed to.
	"awardemoji.noteEmojiRequest.name": "name is exposed + wired on every public create input; flagged on an unexported helper struct",

	// Whole-type curated subset: override_params accepts the full CreateProjectOptions
	// set (~79 fields); the import tool exposes the commonly-overridden subset — full
	// project configuration is available via the dedicated gitlab_project create/update
	// tools. (topics/tag_list are also excluded due to an SDK multipart []string bug.)
	"projectimportexport.ImportOverrideParamsInput": "override_params curated subset; full project config via gitlab_project create/update tools",
}

// isAcceptedMissingInput reports whether an input pair's missing field is an
// adjudicated legitimate omission, checking the whole-type key first then per-field.
func isAcceptedMissingInput(pkg, mcpType, tag string) bool {
	if _, ok := acceptedMissingInputs[pkg+"."+mcpType]; ok {
		return true
	}
	_, ok := acceptedMissingInputs[pkg+"."+mcpType+"."+tag]
	return ok
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

// report is the JSON document written to the output path.
type report struct {
	SchemaVersion int             `json:"schema_version"`
	ClientGoPath  string          `json:"client_go_path"`
	Summary       reportSummary   `json:"summary"`
	Packages      []packageReport `json:"packages"`
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
}

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline). gapsOnly filters to entries with at
// least one finding, matching the original -gaps-only flag.
func Run(root string, gapsOnly bool) ([]byte, error) {
	rep, err := buildReport(root, gapsOnly)
	if err != nil {
		return nil, err
	}
	content, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), nil
}

func buildReport(root string, gapsOnly bool) (report, error) {
	pkgs, err := loadToolPackages(root)
	if err != nil {
		return report{}, err
	}
	reports := make([]packageReport, 0, len(pkgs))
	for _, pkg := range pkgs {
		pr, ok := analyzePackage(pkg)
		if !ok {
			continue
		}
		if gapsOnly && pr.MissingInputCount == 0 && pr.MissingOutputCount == 0 && pr.ExtraOutputCount == 0 {
			continue
		}
		reports = append(reports, pr)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Package < reports[j].Package })
	return report{
		SchemaVersion: schemaVersion,
		ClientGoPath:  clientGoPkgPath,
		Summary:       summarize(reports),
		Packages:      reports,
	}, nil
}

func loadToolPackages(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: root,
	}
	loaded, err := packages.Load(cfg, "./internal/tools/...")
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	var fatal []string
	out := make([]*packages.Package, 0, len(loaded))
	for _, pkg := range loaded {
		for _, perr := range pkg.Errors {
			fatal = append(fatal, perr.Error())
		}
		if !strings.Contains(pkg.PkgPath, toolsPkgInfix) {
			continue
		}
		if pkg.Types == nil || pkg.TypesInfo == nil {
			continue
		}
		out = append(out, pkg)
	}
	if len(fatal) > 0 {
		return nil, fmt.Errorf("package load errors:\n%s", strings.Join(fatal, "\n"))
	}
	return out, nil
}

func analyzePackage(pkg *packages.Package) (packageReport, bool) {
	inputPairs := map[[2]string]structPair{}
	outputPairs := map[[2]string]structPair{}
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
		g := diffPair(pr.Package, "input", pair)
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
		g := diffOutputGroup(pr.Package, group)
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
	sort.Slice(out, func(i, j int) bool { return out[i].mcpName < out[j].mcpName })
	return out
}

// diffOutputGroup diffs one MCP output type against the UNION of every SDK
// result struct paired to it. A json tag is MISSING only when absent from the
// union of all paired SDK fields, and EXTRA only when absent from the union
// (after normalizeSDKTag + envelope carve-out + accepted-rename allowlist). A
// TypeMismatch is reported for a tag only when the MCP type is incompatible with
// the SDK type in EVERY pairing that carries that tag, so a field that is
// compatible in at least one pairing is not double-flagged.
func diffOutputGroup(pkg string, group outputGroup) gap {
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
			if alt, ok := mcpFields[normalizeSDKTag(tag)]; ok {
				mcpType, present = alt, true
			}
		}
		if !present {
			// Doc-grounded omissions intentionally drop SDK fields the endpoint does
			// not return (the cited API doc is the 1:1 ground truth): whole nested
			// reference subsets, and individual top-level fields.
			if !isCuratedRefSubset(pkg, group.mcpName) && !isDocOmittedField(pkg, group.mcpName, tag) {
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
	g.ExtraFields = extraOutputFields(pkg, group.mcpName, mcpFields, unionSDK)
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
	mcpName string
	mcpType *types.Struct
	sdkName string
	sdkType *types.Struct
	// tag preference for the SDK side: url first for Options, json for results.
	sdkURLTags bool
}

func sortedPairs(pairs map[[2]string]structPair) []structPair {
	out := make([]structPair, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, pair)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].mcpName != out[j].mcpName {
			return out[i].mcpName < out[j].mcpName
		}
		return out[i].sdkName < out[j].sdkName
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
	mcpStruct, mcpName, ok := localOrAliasNamedStruct(pkg, resultExpr, resultType)
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
	if mcpName == "" || !ast.IsExported(mcpName) {
		return
	}
	key := [2]string{mcpName, sdkNamed.Obj().Name()}
	out[key] = structPair{
		mcpName: mcpName, mcpType: mcpStruct,
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
			mcpName: mcpNamed.Obj().Name(), mcpType: mcpStruct,
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
func diffPair(pkg, kind string, pair structPair) gap {
	mcpFields := flattenFields(pair.mcpType, []string{tagKeyJSON})
	sdkTagKeys := []string{tagKeyJSON}
	if pair.sdkURLTags {
		sdkTagKeys = []string{"url", tagKeyJSON}
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
			if alt, ok := mcpFields[normalizeSDKTag(tag)]; ok {
				mcpType, present = alt, true
			}
		}
		if !present {
			// Adjudicated legitimate input omission (deliberate rename, nested-object
			// modeling, deprecated/non-accepted param, or curated subset) — each
			// carries a rationale in acceptedMissingInputs.
			if kind == "input" && isAcceptedMissingInput(pkg, pair.mcpName, tag) {
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
func inputPairTags(pair structPair) (mcp, sdk map[string]struct{}) {
	mcp = map[string]struct{}{}
	for tag := range flattenFields(pair.mcpType, []string{tagKeyJSON}) {
		if tag != "" && tag != "-" {
			mcp[tag] = struct{}{}
		}
	}
	sdk = map[string]struct{}{}
	sdkKeys := []string{tagKeyJSON}
	if pair.sdkURLTags {
		sdkKeys = []string{"url", tagKeyJSON}
	}
	for tag := range flattenFields(pair.sdkType, sdkKeys) {
		if tag != "" && tag != "-" {
			sdk[normalizeSDKTag(tag)] = struct{}{}
		}
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
// extra when it is neither a key of sdkFields nor the normalizeSDKTag image of
// any SDK key, is not the MCP-envelope carve-out, is not the "-" sentinel, and
// is not an allowlisted deliberate rename (per the 1:1 data-fidelity policy).
func extraOutputFields(pkg, mcpType string, mcpFields, sdkFields map[string]string) []extraField {
	sdkNorm := make(map[string]struct{}, len(sdkFields))
	for sdkTag := range sdkFields {
		sdkNorm[normalizeSDKTag(sdkTag)] = struct{}{}
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
		if isAcceptedRename(mcpType, tag) {
			continue
		}
		// Documented field surfaced via raw-API fetch (SDK struct lacks it): not
		// invented — the official API doc returns it.
		if isDocAddedField(pkg, mcpType, tag) {
			continue
		}
		// Adjudicated legitimate extra (GraphQL-sourced type, server-derived field,
		// or SDK-untagged field) — each carries a rationale in acceptedExtraOutputs.
		if isAcceptedExtraOutput(pkg, mcpType, tag) {
			continue
		}
		extras = append(extras, extraField{Tag: tag, MCPType: mcpFields[tag]})
	}
	return extras
}

// flattenFields walks a struct (recursing into embedded structs) and returns a
// map of tag-name → type string for every field carrying one of the tag keys.
func flattenFields(st *types.Struct, tagKeys []string) map[string]string {
	out := map[string]string{}
	flattenInto(st, tagKeys, out, 0)
	return out
}

func flattenInto(st *types.Struct, tagKeys []string, out map[string]string, depth int) {
	if st == nil || depth > 6 {
		return
	}
	for i := range st.NumFields() {
		field := st.Field(i)
		raw := reflect.StructTag(st.Tag(i))
		tagName := tagValue(raw, tagKeys)
		if field.Embedded() && tagName == "" {
			if embedded, ok := structUnder(field.Type()); ok {
				flattenInto(embedded, tagKeys, out, depth+1)
				continue
			}
		}
		if tagName == "" || tagName == "-" {
			continue
		}
		if _, exists := out[tagName]; !exists {
			out[tagName] = types.TypeString(field.Type(), shortQualifier)
		}
	}
}

// normalizeSDKTag maps a client-go url-tag name to the snake_case json name the
// MCP inputs use: trailing array notation ("iids[]" → "iids") and bracket
// negation notation ("not[author_id]" → "not_author_id").
func normalizeSDKTag(tag string) string {
	tag = strings.TrimSuffix(tag, "[]")
	tag = strings.ReplaceAll(tag, "[", "_")
	tag = strings.ReplaceAll(tag, "]", "")
	// GraphQL-backed SDK structs tag fields in camelCase (createdAt, targetBranch)
	// where the MCP output uses the project's snake_case convention. This runs only
	// in the fallback path (after an exact-tag match fails), so it can only ADD a
	// match (camelCase SDK tag <-> snake_case MCP tag), never break an exact one.
	return camelToSnake(tag)
}

// camelToSnake lowercases a camelCase identifier with underscore separators
// (createdAt -> created_at). A snake_case input is returned unchanged (no uppercase).
func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func tagValue(raw reflect.StructTag, keys []string) string {
	for _, key := range keys {
		if value, ok := raw.Lookup(key); ok {
			name, _, _ := strings.Cut(value, ",")
			if name != "" {
				return name
			}
		}
	}
	return ""
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
func localOrAliasNamedStruct(pkg *packages.Package, resultExpr ast.Expr, resultType types.Type) (*types.Struct, string, bool) {
	if named, st, ok := localNamedStruct(pkg, resultType); ok {
		return st, named.Obj().Name(), true
	}
	ident := identForExpr(resultExpr)
	if ident == nil {
		return nil, "", false
	}
	aliasObj, isTypeName := pkg.TypesInfo.Uses[ident].(*types.TypeName)
	if !isTypeName || !aliasObj.IsAlias() ||
		aliasObj.Pkg() == nil || aliasObj.Pkg().Path() != pkg.PkgPath {
		return nil, "", false
	}
	// Go materializes type aliases as *types.Alias; unwrap to the target named
	// struct (e.g. Output -> toolutil.MergeRequestOutput) before reading fields.
	_, st, ok := derefNamedStruct(types.Unalias(resultType))
	if !ok {
		return nil, "", false
	}
	return st, aliasObj.Name(), true
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
	if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), clientGoPkgPath) {
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
	_, after, ok := strings.Cut(pkgPath, toolsPkgInfix)
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
