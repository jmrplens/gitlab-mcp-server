// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// orbitContextOp is the operation an [EntityContext] failure is reported
// under.
const orbitContextOp = "orbit_context"

// contextPath is where GitLab resolves a reference to its context, below
// /api/v4. client-go has no method for it.
const contextPath = "orbit/context"

// maxContextTokens is how many tokens one orbit.context call takes: GitLab
// answers any other count 400 "tokens must be an array of exactly one string"
// (measured 2026-10-29), and the handler refuses it before the request.
const maxContextTokens = 1

// EntityContextInput names the one reference to resolve, whether to expand
// its graph neighborhood, and the format.
type EntityContextInput struct {
	// Tokens is a list of exactly one reference to a GitLab object, as GitLab
	// names the parameter.
	Tokens []string `json:"tokens" jsonschema:"Exactly one reference to a GitLab object: Type[id] (WorkItem[1], Job[2]), a GID, an instance URL, path!iid (merge request), path#iid (work item), path&iid (epic) or path@ref (branch or commit),required"`
	// Expand asks for the object's graph neighborhood beside its summary.
	Expand bool `json:"expand,omitempty" jsonschema:"Attach the graph neighborhood of a work item, merge request or job, which GitLab reads with one graph query billed like orbit.query; false reads the summary alone"`
	// ResponseFormat is raw or llm.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format: raw (structured JSON, the default) or llm (compact text). json is read as raw."`
}

// contextExample is the reference orbit.context offers as its example, the
// one its input schema carries and the dynamic surface fills the example call
// it offers with.
const contextExample = "gitlab-org/gitlab#7"

// contextInput is what orbit.context says about its parameters: an override of
// the tokens property whose one example is a list of one token.
func contextInput() orbitInput {
	return orbitInput{overrides: []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("tokens", map[string]any{"examples": []any{[]any{contextExample}}}),
	}}
}

// ContextUser is a user a context summary names.
type ContextUser struct {
	// ID is the user's ID.
	ID int64 `json:"id"`
	// Username is the user's username.
	Username string `json:"username,omitempty"`
	// Name is the user's display name.
	Name string `json:"name,omitempty"`
}

// ContextChangedFile is one file a merge request or a commit changes.
type ContextChangedFile struct {
	// Path is the file's path.
	Path string `json:"path"`
	// OldPath is the path a renamed file had.
	OldPath string `json:"old_path,omitempty"`
	// Status is the change: A, M, D or R.
	Status string `json:"status,omitempty"`
	// Lines are the changed line ranges, as GitLab writes them (16-21).
	Lines []string `json:"lines,omitempty"`
	// Diff is an excerpt of the file's diff, GitLab's own cut, since version
	// 1.4.0 of the answer.
	Diff string `json:"diff,omitempty"`
}

// ContextFailedTest is one failed test of a job's unit test report.
type ContextFailedTest struct {
	// Name is the test's name.
	Name string `json:"name,omitempty"`
	// ClassName is the test's class or suite.
	ClassName string `json:"classname,omitempty"`
	// File is the file the test is in.
	File string `json:"file,omitempty"`
	// Output is the start of the test's output, GitLab's own cut.
	Output string `json:"output,omitempty"`
}

// ContextPipelineJob is one job of a pipeline.
type ContextPipelineJob struct {
	// Name is the job's name.
	Name string `json:"name"`
	// Stage is the job's stage.
	Stage string `json:"stage,omitempty"`
	// Status is the job's status.
	Status string `json:"status,omitempty"`
	// AllowFailure says whether the job may fail.
	AllowFailure bool `json:"allow_failure"`
	// WebURL is the job's page.
	WebURL string `json:"web_url,omitempty"`
}

// ContextFailedJob is one failed job of a pipeline (a merge request's head
// pipeline, a branch's latest one or the pipeline itself), with an excerpt of
// its log and its failed tests.
type ContextFailedJob struct {
	// Name is the job's name.
	Name string `json:"name"`
	// Stage is the job's stage.
	Stage string `json:"stage,omitempty"`
	// FailureReason is why the job failed.
	FailureReason *string `json:"failure_reason"`
	// AllowFailure says whether the job may fail.
	AllowFailure bool `json:"allow_failure"`
	// WebURL is the job's page.
	WebURL string `json:"web_url,omitempty"`
	// Log is an excerpt of the job's log, GitLab's own cut.
	Log string `json:"log,omitempty"`
	// FailedTests are the job's failed tests.
	FailedTests []ContextFailedTest `json:"failed_tests,omitempty"`
	// Truncated names what GitLab cut from the job: failed_tests.
	Truncated []string `json:"truncated,omitempty"`
}

// ContextSummary is the summary GitLab reads from PostgreSQL for a resolved
// object. Which fields it fills depends on the type: a WorkItem, a
// MergeRequest, a Job or a Branch, as GitLab.com answered on 2026-10-29, or a
// Pipeline or a Commit, whose fields are the ones ContextService writes at the
// same version of the answer. The lists GitLab answered empty for every
// fixture (discussions, assignees, reviewers, linked items, the merge requests
// of a job, branch, pipeline or commit) are published as they came.
type ContextSummary struct {
	// NamespacePath and ProjectPath place the object.
	NamespacePath string `json:"namespace_path,omitempty"`
	ProjectPath   string `json:"project_path,omitempty"`
	// WebURL is the object's page.
	WebURL string `json:"web_url,omitempty"`
	// IID, Title, State and Description describe a work item or a merge
	// request, and Title and Description a commit.
	IID         int64  `json:"iid,omitempty"`
	Title       string `json:"title,omitempty"`
	State       string `json:"state,omitempty"`
	Description string `json:"description,omitempty"`
	// WorkItemType is a work item's type, issue for one GitLab made from an
	// issue.
	WorkItemType string `json:"work_item_type,omitempty"`
	// Labels are a work item's labels.
	Labels []string `json:"labels,omitempty"`
	// Assignees, Milestone and LinkedMergeRequests belong to a work item.
	Assignees           any `json:"assignees,omitempty"`
	Milestone           any `json:"milestone,omitempty"`
	LinkedMergeRequests any `json:"linked_merge_requests,omitempty"`
	// Discussions are the object's discussions.
	Discussions any `json:"discussions,omitempty"`
	// Author, Reviewers and the branches belong to a merge request.
	Author       *ContextUser `json:"author,omitempty"`
	Reviewers    any          `json:"reviewers,omitempty"`
	SourceBranch string       `json:"source_branch,omitempty"`
	TargetBranch string       `json:"target_branch,omitempty"`
	// AuthorName, AuthoredDate and MergeRequests belong to a commit.
	AuthorName    string `json:"author_name,omitempty"`
	AuthoredDate  string `json:"authored_date,omitempty"`
	MergeRequests any    `json:"merge_requests,omitempty"`
	// ChangedFiles are the files a merge request or a commit changes.
	ChangedFiles []ContextChangedFile `json:"changed_files,omitempty"`
	// LinkedIssues are the issues a merge request links.
	LinkedIssues any `json:"linked_issues,omitempty"`
	// HeadPipelineStatus is a merge request's head pipeline status.
	HeadPipelineStatus string `json:"head_pipeline_status,omitempty"`
	// MergeCommitSHA is a merged merge request's commit.
	MergeCommitSHA *string `json:"merge_commit_sha,omitempty"`
	// FailedJobs are the failed jobs of a pipeline, or of a merge request's or
	// a branch's.
	FailedJobs []ContextFailedJob `json:"failed_jobs,omitempty"`
	// Source and Jobs belong to a pipeline: what started it, and its jobs.
	Source string               `json:"source,omitempty"`
	Jobs   []ContextPipelineJob `json:"jobs,omitempty"`
	// FailedTests are a failed job's failed tests.
	FailedTests []ContextFailedTest `json:"failed_tests,omitempty"`
	// ID, Name, Stage, Status, FailureReason, AllowFailure, Log, Ref and SHA
	// describe a job; ID, Status, Ref and SHA a pipeline; Name, SHA and
	// DefaultBranch a branch; SHA a commit.
	ID            int64   `json:"id,omitempty"`
	Name          string  `json:"name,omitempty"`
	Stage         string  `json:"stage,omitempty"`
	Status        string  `json:"status,omitempty"`
	FailureReason *string `json:"failure_reason,omitempty"`
	AllowFailure  *bool   `json:"allow_failure,omitempty"`
	Log           string  `json:"log,omitempty"`
	Ref           string  `json:"ref,omitempty"`
	SHA           string  `json:"sha,omitempty"`
	DefaultBranch *bool   `json:"default_branch,omitempty"`
	// PipelineID and PipelineStatus are a job's, a branch's or a commit's
	// pipeline.
	PipelineID     int64  `json:"pipeline_id,omitempty"`
	PipelineStatus string `json:"pipeline_status,omitempty"`
	// MergeRequest is the merge request a job, a branch or a pipeline belongs
	// to.
	MergeRequest any `json:"merge_request,omitempty"`
	// Truncated names the parts GitLab cut from the summary.
	Truncated []string `json:"truncated,omitempty"`
}

// ContextQuery is the named query GitLab ran for an expansion.
type ContextQuery struct {
	// Name is the named query (context_for_issue, context_for_review or
	// context_for_job).
	Name string `json:"name"`
	// Parameters are what GitLab ran it with.
	Parameters any `json:"parameters,omitempty"`
}

// ContextExpansion is an object's graph neighborhood: the named query GitLab
// ran or would have run and its answer, the result envelope orbit.query
// decodes, or why there is none. A failed expansion never fails the summary.
type ContextExpansion struct {
	// Query is the named query, absent for a type GitLab expands none for.
	Query *ContextQuery `json:"query,omitempty"`
	// Result is the graph answer, as GitLab sent it.
	Result any `json:"result,omitempty"`
	// Error says why GitLab answered no neighborhood.
	Error *ContextExpansionError `json:"error,omitempty"`
}

// ContextExpansionError says why GitLab expanded no neighborhood.
type ContextExpansionError struct {
	// Code is expansion_unsupported_type, outside_graph_scope,
	// billing_namespace_required, quota_exhausted, timeout or
	// execution_error.
	Code string `json:"code"`
	// Message, DeepLink and Candidates come with billing_namespace_required:
	// what GitLab asks of the token's user, where they choose the group Orbit
	// bills, and the groups they can choose.
	Message    string             `json:"message,omitempty"`
	DeepLink   string             `json:"deep_link,omitempty"`
	Candidates []BillingCandidate `json:"candidates,omitempty"`
}

// contextNotFoundCode is the code GitLab answers a token with when its object
// does not exist, the user cannot read it or the access token may not: a
// fine-grained one that lacks the permission, or a classic one where the
// object's top-level group enforces fine-grained tokens.
const contextNotFoundCode = "not_found"

// ContextError says why GitLab resolved nothing for a token.
type ContextError struct {
	// Code is invalid_token, unsupported_type or not_found
	// ([contextNotFoundCode]).
	Code string `json:"code"`
}

// ContextEntity is what GitLab resolved one token to.
type ContextEntity struct {
	// Token is the token as the caller sent it.
	Token string `json:"token"`
	// ResolvedToken is the token in GitLab's own form: Type[id], or path@ref
	// for a branch or a commit.
	ResolvedToken string `json:"resolved_token,omitempty"`
	// Type is the object's type, empty when GitLab resolved nothing.
	Type string `json:"type,omitempty"`
	// ID is the object's ID, as GitLab writes it, a string: a branch's name
	// and a commit's SHA too.
	ID string `json:"id,omitempty"`
	// Found says whether GitLab resolved the token.
	Found bool `json:"found"`
	// Summary is the object's summary.
	Summary *ContextSummary `json:"summary,omitempty"`
	// Expansion is the object's graph neighborhood, when expand was asked.
	Expansion *ContextExpansion `json:"expansion,omitempty"`
	// Error says why GitLab resolved nothing.
	Error *ContextError `json:"error,omitempty"`
}

// EntityContextOutput is what GitLab resolved the token to.
type EntityContextOutput struct {
	toolutil.HintableOutput
	// ResponseFormat is the format that produced the answer.
	ResponseFormat string `json:"response_format,omitempty"`
	// Version is the version of GitLab's answer.
	Version string `json:"version,omitempty"`
	// Entities are what GitLab resolved, one per token.
	Entities []ContextEntity `json:"entities,omitempty"`
	// FormattedText is the compact text GitLab answered the llm format with.
	FormattedText string `json:"formatted_text,omitempty"`
}

// EntityContext resolves one reference to a GitLab object and returns its
// summary and, with expand, its graph neighborhood.
//
// Source: POST /api/v4/orbit/context, which client-go has no method for.
// GitLab takes a JSON body alone, {"tokens": [one token], "expand": bool,
// "response_format": raw|llm}, and resolves an issue URL, path#iid,
// WorkItem[id], path!iid, path@ref and Job[id] (measured 2026-10-29), and a
// pipeline and a commit too (doc/api/orbit.md). A token it resolves nothing
// for is answered 200 with found false and an error code, which the output
// carries rather than an error. The route's own permission, read_orbit, opens
// none of them to a fine-grained access token: GitLab also checks inside the
// route that the token may read the object and each part of its summary on the
// object's project or group (doc/api/orbit.md at 19.6.1 lists the
// permissions), answers not_found for an object it may not and leaves out a
// part it may not. AuthorizeGranularScopesService, which decides it, refuses a
// classic personal access token of a person the same where the object's
// top-level group enforces fine-grained tokens. With expand, GitLab runs one
// named query for the neighborhood of a work item, a merge request or a job,
// billed as a graph query; what keeps it from answering one is an error code of
// the expansion, beside the summary.
func EntityContext(ctx context.Context, client *gitlabclient.Client, input EntityContextInput) (EntityContextOutput, error) {
	if err := ctx.Err(); err != nil {
		return EntityContextOutput{}, err
	}
	token, err := contextToken(input.Tokens)
	if err != nil {
		return EntityContextOutput{}, err
	}
	format, sent, err := mcpFormat(input.ResponseFormat)
	if err != nil {
		return EntityContextOutput{}, err
	}
	body := map[string]any{"tokens": []string{token}, "expand": input.Expand, "response_format": string(sent)}
	out := EntityContextOutput{ResponseFormat: responseFormatName(format)}
	if sent == gl.OrbitResponseFormatLLM {
		var text bytes.Buffer
		if err = restRequest(ctx, client, http.MethodPost, contextPath, body, &text); err != nil {
			return EntityContextOutput{}, wrapContextErr(err)
		}
		out.FormattedText = text.String()
		return out, nil
	}
	var answer struct {
		Version  string          `json:"version"`
		Entities []ContextEntity `json:"entities"`
	}
	if err = restRequest(ctx, client, http.MethodPost, contextPath, body, &answer); err != nil {
		return EntityContextOutput{}, wrapContextErr(err)
	}
	out.Version = answer.Version
	out.Entities = answer.Entities
	return out, nil
}

// errContextTokens is what [contextToken] answers for a list that is not
// exactly one token.
var errContextTokens = fmt.Errorf("%s: tokens takes exactly one token (%d); call orbit.context once per object", orbitContextOp, maxContextTokens)

// contextToken returns the one token of the list, trimmed, and refuses a list
// GitLab would refuse: none, more than [maxContextTokens] or a blank one.
func contextToken(tokens []string) (string, error) {
	if len(tokens) == 0 {
		return "", toolutil.ErrRequiredString(orbitContextOp, "tokens")
	}
	if len(tokens) > maxContextTokens {
		return "", errContextTokens
	}
	token := strings.TrimSpace(tokens[0])
	if token == "" {
		return "", toolutil.ErrRequiredString(orbitContextOp, "tokens")
	}
	return token, nil
}

// contextRefusedHint follows a context request GitLab refused with 400.
const contextRefusedHint = "orbit.context takes one token: Type[id], a GID, an instance URL, path!iid, path#iid, path&iid or path@ref"

// wrapContextErr classifies a failed [EntityContext]: a 400 is GitLab's
// refusal of the request, followed by [contextRefusedHint], and anything else
// is classified the way every Orbit failure is, by [wrapOrbitErr].
func wrapContextErr(err error) error {
	if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return toolutil.WrapErrWithHint(orbitContextOp, err, contextRefusedHint)
	}
	return wrapOrbitErr(orbitContextOp, err)
}
