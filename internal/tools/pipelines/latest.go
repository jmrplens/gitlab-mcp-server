package pipelines

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Operation names the latest-pipeline action reports its errors under, one
// per request it can send.
const (
	opGetLatest              = "pipelineGetLatest"
	opGetLatestFallback      = "pipelineGetLatest(fallback)"
	opGetLatestDefaultBranch = "pipelineGetLatest(default branch)"
	opGetLatestHead          = "pipelineGetLatest(head)"
)

// hintReadPipelines is what a refusal of the pipeline list says about the
// caller. The list authorizes read_pipeline and read_build on the project
// (lib/api/ci/pipelines.rb). The Reporter role grants both
// (config/authz/roles/reporter.yml). A Guest, and a Planner, which inherits
// Guest's permissions and adds none for CI/CD, holds them only through
// _read_public_pipeline and _read_public_build, which app/policies/
// project_policy.rb prevents while Project-based pipeline visibility
// (public_builds) is off; a non-member of a public project reads them the
// same way. Whatever the role, the policy prevents both while the project's
// CI/CD or repository feature is unavailable to the caller. The CI/CD table of
// doc/user/permissions.md marks Planner as reading pipelines unconditionally,
// and the policy is what GitLab enforces.
//
// It and the two notes below are single literals rather than concatenations:
// a constant expression has no statement for a coverage profile to count, so
// a mutation tool reports every + in one as never covered.
const hintReadPipelines = "reading a project's pipelines needs the Reporter role or higher. A Guest or Planner member, or a non-member of a public project, reads them only while the project's Project-based pipeline visibility setting is on, and no role reads them while the project's CI/CD or repository feature is unavailable to the caller"

// The notes the fallback writes beside the pipeline it shows, one for a head
// commit it read and one for a head GitLab would not show it. They are the
// server's own prose and name no value: the ref and the two commits are in
// the output's ref, head_sha and sha, where the card escapes them, and a ref
// may carry any character git check-ref-format allows.
const (
	noteHeadWithoutPipeline = "GitLab has no pipeline on this ref for the commit at its head (head_sha), so it reports no latest pipeline for the ref. This is the first pipeline on the ref that the pipeline list returns, and it ran for an earlier commit (sha)."
	noteHeadUnreadable      = "GitLab reports no latest pipeline for this ref, and the commit at its head could not be read: the ref does not exist, or the token cannot read the repository. This is the first pipeline on the ref that the pipeline list returns."
)

// danglingSources are the pipeline sources GitLab leaves out of the latest
// pipeline of a ref, because their pipelines do not set its status
// (Enums::Ci::Pipeline.dangling_sources, which Project#latest_pipeline leaves
// out through Project#ci_pipelines): a Web IDE terminal, a child pipeline, an
// on-demand DAST scan or site validation, and a security policy's own
// pipeline. The pipeline list returns all but the child pipelines unless a
// source is asked for.
var danglingSources = []string{"webide", "parent_pipeline", "ondemand_dast_scan", "ondemand_dast_validation", "security_orchestration_policy"}

// GetLatestInput defines parameters for getting the latest pipeline.
//
// GET /projects/:id/pipelines/latest takes ref alone. The filters below
// mirror gl.ListProjectPipelinesOptions and apply only to the list that
// answers when that route has no pipeline for the head of the ref, see
// [GetLatest]; GitLab's own answer is never filtered.
type GetLatestInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Ref           string               `json:"ref,omitempty"           jsonschema:"Branch or tag whose latest pipeline is requested (defaults to the project's default branch)"`
	Scope         string               `json:"scope,omitempty"         jsonschema:"Filter by scope (running, pending, finished, branches, tags). Applies to the list fallback."`
	Status        string               `json:"status,omitempty"        jsonschema:"Filter by status (created, waiting_for_resource, preparing, pending, running, success, failed, canceled, skipped, manual, scheduled). Applies to the list fallback."`
	Source        string               `json:"source,omitempty"        jsonschema:"Filter by pipeline source (api, chat, external, external_pull_request_event, merge_request_event, ondemand_dast_scan, ondemand_dast_validation, parent_pipeline, pipeline, push, schedule, security_orchestration_policy, trigger, web, webide). Applies to the list fallback."`
	SHA           string               `json:"sha,omitempty"           jsonschema:"Filter by commit SHA. Applies to the list fallback."`
	Name          string               `json:"name,omitempty"          jsonschema:"Filter by pipeline name. Applies to the list fallback."`
	Username      string               `json:"username,omitempty"      jsonschema:"Filter by username that triggered the pipeline. Applies to the list fallback."`
	YamlErrors    bool                 `json:"yaml_errors,omitempty"   jsonschema:"Return only pipelines with YAML errors. Applies to the list fallback."`
	OrderBy       string               `json:"order_by,omitempty"      jsonschema:"Order by field (id, status, ref, updated_at, user_id). Applies to the list fallback."`
	Sort          string               `json:"sort,omitempty"          jsonschema:"Sort direction (asc, desc). Applies to the list fallback."`
	CreatedAfter  string               `json:"created_after,omitempty" jsonschema:"Return pipelines created after date (ISO 8601 format). Applies to the list fallback."`
	CreatedBefore string               `json:"created_before,omitempty" jsonschema:"Return pipelines created before date (ISO 8601 format). Applies to the list fallback."`
	UpdatedAfter  string               `json:"updated_after,omitempty" jsonschema:"Return pipelines updated after date (ISO 8601 format). Applies to the list fallback."`
	UpdatedBefore string               `json:"updated_before,omitempty" jsonschema:"Return pipelines updated before date (ISO 8601 format). Applies to the list fallback."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// LatestOutput is what pipeline.latest answers: the pipeline in full, and,
// when it is not GitLab's latest pipeline of the ref but the one the list
// fell back to, what tells the two apart.
//
// HeadSHA and FallbackNote are this server's own and no route sends them.
// Both are empty when GitLab answered the latest route itself, and when the
// pipeline the list returned did run for the commit at the head.
type LatestOutput struct {
	DetailOutput
	HeadSHA      string `json:"head_sha,omitempty"      jsonschema:"Commit at the head of the ref, set only when that commit has no pipeline on the ref and the pipeline shown ran for the earlier commit in sha"`
	FallbackNote string `json:"fallback_note,omitempty" jsonschema:"Set only when the pipeline shown is not GitLab's latest pipeline of the ref, saying why"`
}

// GetLatest retrieves the latest pipeline of a ref, the project's default
// branch when no ref is given: the newest pipeline GitLab has for the commit
// at the head of that ref, which is what Project#latest_pipeline returns.
//
// GitLab answers that route 403 for two reasons it does not tell apart. The
// route authorizes read_pipeline on the pipeline it found, and when the head
// commit has none it found nil and authorizing nil fails, which pipelines.md
// documents ("If no pipeline exists for the commit, a 403 status code is
// returned"): a push still being processed, or a commit workflow:rules kept
// from creating one. A caller who may not read the project's pipelines gets
// the same 403. A plain 403 is therefore answered from the pipeline list of
// the same ref, see [getLatestFallback], whose own refusal is the one that
// says it was the second. A 403 carrying an OAuth error code refuses the
// token itself and is neither, so it is reported as it came.
func GetLatest(ctx context.Context, client *gitlabclient.Client, input GetLatestInput) (LatestOutput, error) {
	if err := ctx.Err(); err != nil {
		return LatestOutput{}, err
	}
	if input.ProjectID == "" {
		return LatestOutput{}, errors.New("pipelineGetLatest: project_id is required")
	}
	opts := &gl.GetLatestPipelineOptions{}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	latestCtx, captured := gitlabclient.WithResponseCapture(ctx)
	p, _, err := client.GL().Pipelines.GetLatestPipeline(string(input.ProjectID), opts, gl.WithContext(latestCtx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) && toolutil.IsPermissionRefusal(err) {
			return getLatestFallback(ctx, client, input)
		}
		return LatestOutput{}, toolutil.WrapErrWithMessage(opGetLatest, err)
	}
	detail, err := capturedDetail(opGetLatest, p, captured)
	return LatestOutput{DetailOutput: detail}, err
}

// getLatestFallback answers for a latest route that refused: it finds the
// first pipeline on the ref GitLab was asked about, resolving the default
// branch when the caller named none, see [firstPipelineOnRef] for why the
// list alone does not keep the answer on that ref. It then reads the commit
// at the head of the ref and that pipeline in full, and says beside it that
// it ran for an earlier commit.
func getLatestFallback(ctx context.Context, client *gitlabclient.Client, input GetLatestInput) (LatestOutput, error) {
	ref := input.Ref
	if ref == "" {
		branch, err := defaultBranch(ctx, client, input.ProjectID)
		if err != nil {
			return LatestOutput{}, err
		}
		ref = branch
	}
	listed, err := firstPipelineOnRef(ctx, client, input, ref)
	if err != nil {
		return LatestOutput{}, err
	}
	head, err := headCommit(ctx, client, input.ProjectID, ref)
	if err != nil {
		return LatestOutput{}, err
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	p, _, err := client.GL().Pipelines.GetPipeline(string(input.ProjectID), listed, gl.WithContext(ctx))
	if err != nil {
		return LatestOutput{}, toolutil.WrapErrWithMessage(opGetLatestFallback, err)
	}
	detail, err := capturedDetail(opGetLatestFallback, p, captured)
	if err != nil {
		return LatestOutput{}, err
	}
	return fallbackOutput(detail, head), nil
}

// defaultBranch reads the project's default branch, the ref
// Project#latest_pipeline takes when none is given. GitLab sends it as
// default_branch_or_main, so a project whose repository is empty reports the
// instance's default branch name, and it leaves the key out for a caller who
// may not read the repository (read_code), a Guest of a private project among
// them (lib/api/entities/basic_project_details.rb); [refUnknown] answers that
// case.
//
// A refused project read comes after the latest route found the project, so
// it is a token that may read pipelines and not the project, as a
// fine-grained token granted Pipeline: Read alone may. With a ref the
// fallback does not read the project, so the error says to pass one. It is
// written here rather than as a hint, because a hint on a refusal of a
// fine-grained grant is dropped.
func defaultBranch(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt) (string, error) {
	project, _, err := client.GL().Projects.GetProject(string(projectID), nil, gl.WithContext(ctx))
	if err != nil {
		wrapped := toolutil.WrapErrWithMessage(opGetLatestDefaultBranch, err)
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return "", fmt.Errorf("%s: pass ref to name the branch or tag, because the project, which names the default branch, could not be read, and with a ref the fallback does not read it: %w", opGetLatest, wrapped)
		}
		return "", wrapped
	}
	if project.DefaultBranch == "" {
		return "", refUnknown(ctx, client, projectID)
	}
	return project.DefaultBranch, nil
}

// refUnknown answers a project read that named no default branch, which
// GitLab leaves out for a caller who cannot read the repository. Whether that
// caller can read the project's pipelines is still open, so one row of the
// project's whole pipeline list settles it: its refusal is the permission
// refusal the latest route's 403 could not name, and its answer leaves the
// ref as the one thing missing.
func refUnknown(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt) error {
	opts := &gl.ListProjectPipelinesOptions{PerPage: 1}
	if _, _, err := client.GL().Pipelines.ListProjectPipelines(string(projectID), opts, gl.WithContext(ctx)); err != nil {
		return listError(err)
	}
	return fmt.Errorf("%s: GitLab did not send the default branch of project %s, which it leaves out for a caller who cannot read the repository, so the ref whose latest pipeline is asked for is unknown. Pass ref to name a branch or tag", opGetLatest, projectID)
}

// firstPipelineOnRef returns the ID of the first pipeline on ref the
// project's list returns under the caller's filters, newest first unless the
// caller ordered it otherwise, reading the list a page at a time until one
// turns up.
//
// The list cannot be asked for that alone. For a branch it adds the pipelines
// on the refs of every merge request the branch is the source of
// (refs/merge-requests/N/head, /merge and /train), unless the source asked
// for excludes merge request pipelines
// (Ci::PipelineRefFilterIncludingReservedRefNames), and it includes the
// dangling sources. Project#latest_pipeline counts neither: it takes the
// pipelines whose ref is the ref, from a source that sets its status. A row
// is therefore taken when its ref is the ref and, unless the caller named a
// source and GitLab already filtered by it, its source is not dangling. A
// branch that runs merge request pipelines instead of branch pipelines, as
// GitLab's documented workflow:rules does once a merge request is open, can
// hold several pages of the first kind before a row of its own.
//
// A refusal of the list is the caller's permission, which the latest route's
// 403 could not say, and is returned with the role that reads pipelines.
func firstPipelineOnRef(ctx context.Context, client *gitlabclient.Client, input GetLatestInput, ref string) (int64, error) {
	listOpts := buildListOpts(ListInput{
		ProjectID:             input.ProjectID,
		Scope:                 input.Scope,
		Status:                input.Status,
		Source:                input.Source,
		Ref:                   ref,
		SHA:                   input.SHA,
		Name:                  input.Name,
		Username:              input.Username,
		YamlErrors:            input.YamlErrors,
		OrderBy:               input.OrderBy,
		Sort:                  input.Sort,
		CreatedAfter:          input.CreatedAfter,
		CreatedBefore:         input.CreatedBefore,
		UpdatedAfter:          input.UpdatedAfter,
		UpdatedBefore:         input.UpdatedBefore,
		PaginationInput:       input.PaginationInput,
		KeysetPaginationInput: input.KeysetPaginationInput,
	})
	if listOpts.OrderBy == nil {
		listOpts.OrderBy = new("id")
	}
	if listOpts.Sort == nil {
		listOpts.Sort = new("desc")
	}
	rows := gl.Scan2(func(page gl.PaginationOptionFunc) ([]*gl.PipelineInfo, *gl.Response, error) {
		return client.GL().Pipelines.ListProjectPipelines(string(input.ProjectID), listOpts, gl.WithContext(ctx), page)
	})
	for row, err := range rows {
		if err != nil {
			return 0, listError(err)
		}
		if row.Ref == ref && (input.Source != "" || !slices.Contains(danglingSources, row.Source)) {
			return row.ID, nil
		}
	}
	return 0, noPipelineOnRef(input, ref)
}

// listError is the error a refusal or a failure of the pipeline list
// returns. A refusal is the caller's permission and carries the role that
// reads pipelines.
func listError(err error) error {
	if toolutil.IsPermissionRefusal(err) {
		return toolutil.WrapErrWithHint(opGetLatestFallback, err, hintReadPipelines)
	}
	return toolutil.WrapErrWithMessage(opGetLatestFallback, err)
}

// noPipelineOnRef is the error for a list that held no pipeline on ref. When
// the caller's own filters may have emptied it, it names them, since the ref
// itself may well have pipelines; otherwise it is the ref that has none.
func noPipelineOnRef(input GetLatestInput, ref string) error {
	if filtersGiven(input) {
		return fmt.Errorf("%s: no pipeline found on ref %q under the filters given: GitLab reports none for the commit at its head, and the pipeline list returns none on that ref that matches them. Drop the filters to ask for the newest pipeline on the ref", opGetLatest, ref)
	}
	return fmt.Errorf("%s: no pipeline found on ref %q: GitLab reports none for the commit at its head, and the pipeline list returns none on that ref. Verify the ref with branch.get or tag.get", opGetLatest, ref)
}

// filtersGiven reports whether the caller narrowed the list: any filter of
// GetLatestInput, a page past the first, or a keyset cursor. The ordering,
// the page size and the pagination method only arrange the rows. It compares
// the input with those cleared against the zero value, so a filter added to
// GetLatestInput counts without a change here.
func filtersGiven(in GetLatestInput) bool {
	in.ProjectID, in.Ref, in.OrderBy, in.Sort, in.Pagination = "", "", "", "", ""
	in.PerPage = 0
	if in.Page == 1 {
		in.Page = 0
	}
	return in != GetLatestInput{}
}

// headCommit returns the SHA of the commit at the head of ref, or "" when
// GitLab answers 404 or 403: a ref that does not exist, a branch deleted
// after its pipelines ran included, or a token that may read pipelines and
// not the repository. Either way the pipeline found is still the answer, so
// only a failure GitLab did not explain is an error.
func headCommit(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt, ref string) (string, error) {
	commit, _, err := client.GL().Commits.GetCommit(string(projectID), ref, nil, gl.WithContext(ctx))
	if err == nil {
		return commit.ID, nil
	}
	if toolutil.IsHTTPStatus(err, http.StatusNotFound) || toolutil.IsHTTPStatus(err, http.StatusForbidden) {
		return "", nil
	}
	return "", toolutil.WrapErrWithMessage(opGetLatestHead, err)
}

// fallbackOutput is the pipeline the list returned on the ref, with what
// tells it apart from the latest pipeline GitLab did not have: the head
// commit and the note saying the pipeline ran for an earlier one, or the note
// that the head could not be read. A pipeline that did run for the head is
// the head's, as one created between the two requests is, and is shown as it
// is.
func fallbackOutput(detail DetailOutput, head string) LatestOutput {
	out := LatestOutput{DetailOutput: detail}
	if head == "" {
		out.FallbackNote = noteHeadUnreadable
		return out
	}
	if head != detail.SHA {
		out.HeadSHA = head
		out.FallbackNote = noteHeadWithoutPipeline
	}
	return out
}
