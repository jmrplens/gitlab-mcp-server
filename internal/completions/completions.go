package completions

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// SPEC: MCP 2025-11-25 "completion/complete" requires `values` to be
// argument values (the literal that will replace the partial input), not
// human-readable labels. Helpers below therefore return the canonical
// identifier for each resource — never an "id: title" label.
const (
	maxCompletionResults = 10
)

// Handler provides GitLab-aware completion for prompt arguments and resource parameters.
type Handler struct {
	// client is the fallback credential, not the one a request necessarily
	// runs under. See [Handler.clientFor].
	client *gitlabclient.Client

	// prompts is the set of prompt names this server serves, or nil when
	// nothing has published one. See [Handler.PublishPrompts].
	prompts atomic.Pointer[map[string]struct{}]

	// excluded is the set of catalog actions the operator removed, or nil when
	// nothing has published one. See [Handler.PublishExcludedActions].
	excluded atomic.Pointer[map[string]struct{}]
}

// NewHandler creates a completion handler backed by the given GitLab client.
func NewHandler(client *gitlabclient.Client) *Handler {
	return &Handler{client: client}
}

// PublishPrompts records the prompt names this server serves, so a completion
// naming a prompt it does not serve is refused rather than answered.
//
// It is a method rather than a constructor argument because of the order the
// server is built in: the completion handler is part of the options
// [mcp.NewServer] is given, and the prompts are registered on the server that
// call returns, so the names do not exist yet when this handler does. Nothing
// reads the set until a request arrives, which is after both.
//
// Publishing an empty set is meaningful and is not the same as publishing
// nothing: it says this server serves no prompts, which is what the minimal
// capability surface does, and every prompt reference is then refused. A
// handler nobody published to answers as it always did, so a caller that never
// learned its prompt names is never made worse off.
func (h *Handler) PublishPrompts(names []string) {
	served := make(map[string]struct{}, len(names))
	for _, name := range names {
		served[name] = struct{}{}
	}
	h.prompts.Store(&served)
}

// servesPrompt reports whether a prompt reference names something this server
// serves. It answers true when nothing was published, since a handler with no
// catalog cannot contradict one.
func (h *Handler) servesPrompt(name string) bool {
	served := h.prompts.Load()
	if served == nil {
		return true
	}
	_, ok := (*served)[name]
	return ok
}

// clientFor returns the client this request must run against: the one bound to
// the request context, or the handler's own when nothing bound one.
//
// One MCP server is shared by every credential whose configuration hashes to
// the same shape, so this handler is built once per shape and the client it
// stores is the credential-less one that refuses every request. Binding the
// caller's own client to the request context is what makes a completion reach
// the caller's instance, and capturing the unbound client is what makes a
// completion reached without that binding fail closed rather than run under
// whichever credential happened to build the shape.
//
// On stdio, and in every test that builds the handler with a real client,
// nothing is ever bound and [gitlabclient.Client.For] returns the stored client
// unchanged, so the indirection is invisible.
func (h *Handler) clientFor(ctx context.Context) *gitlabclient.Client {
	return h.client.For(ctx)
}

// Complete dispatches completion requests based on reference type and argument name.
// It returns empty results on errors to avoid blocking the client.
func (h *Handler) Complete(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	if req.Params.Ref == nil {
		return emptyResult(), nil
	}
	if h.clientFor(ctx).IsUnbound() {
		// Still an empty result, because the contract is that autocomplete is
		// never blocked and an IDE has nothing useful to do with a failure
		// here. But it is said out loud, at a level an operator sees by
		// default: every other cause of an empty completion is a GitLab hiccup
		// that fixes itself, while this one is a wiring defect on this side
		// that never will, and it presents as an editor with no suggestions,
		// forever, with nothing on the wire and nothing in the log.
		//
		// Unless the request is already over. An abandoned POST takes its
		// carrier with it, and the carrier is where the credential is read
		// from, so a client that stopped typing produces exactly this state
		// legitimately. Warning about it would fill an operator's log with
		// wiring defects that are editors closing a popup. The tools path gets
		// this apart for free, because ClassifyError checks cancellation first.
		if cause := context.Cause(ctx); cause != nil {
			slog.DebugContext(ctx, "completion: the request was over before it could be attributed",
				"ref", req.Params.Ref.Type, "argument", req.Params.Argument.Name, "cause", cause)
			return emptyResult(), nil
		}
		slog.WarnContext(ctx, "completion: "+toolutil.UnattributedRequestMessage,
			"ref", req.Params.Ref.Type, "argument", req.Params.Argument.Name)
		return emptyResult(), nil
	}

	switch req.Params.Ref.Type {
	case "ref/prompt":
		// The one failure this package answers with an error rather than an
		// empty list, and deliberately: every other cause of an empty
		// completion is a GitLab hiccup the caller can do nothing about, while
		// a prompt name this server does not serve is something the caller
		// sent and has to change. The specification names the code
		// ("Invalid prompt name: -32602"), prompts/get already answers it for
		// the same name, and without it a reference to a prompt that does not
		// exist was served live GitLab data, which reads to a client as
		// confirmation that it does.
		//
		// The resource reference is deliberately not checked the same way: the
		// specification says nothing about an unserved URI, and a client may
		// legitimately send a concrete URI where we hold only a template, so
		// refusing one would break a caller that is conforming.
		if !h.servesPrompt(req.Params.Ref.Name) {
			return nil, toolutil.InvalidParams(fmt.Errorf("unknown prompt %q", req.Params.Ref.Name))
		}
		return h.completePromptArg(ctx, req)
	case "ref/resource":
		return h.completeResourceArg(ctx, req)
	default:
		return emptyResult(), nil
	}
}

// completePromptArg completes arguments for known prompts.
func (h *Handler) completePromptArg(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	argName := req.Params.Argument.Name
	argValue := req.Params.Argument.Value
	resolvedArgs := resolvedArguments(req)

	if h.withholds(completionBackingActions[argName]...) {
		return emptyResult(), nil
	}

	switch argName {
	case "project_id":
		return h.completeProjectID(ctx, argValue)
	case "group_id":
		return h.completeGroupID(ctx, argValue)
	case "merge_request_iid":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeMRIID)
	case "issue_iid":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeIssueIID)
	case "username":
		return h.completeUsername(ctx, argValue)
	case "from", "to", "ref":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeBranchOrTag)
	case "tag":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeTag)
	case "pipeline_id":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completePipelineID)
	case "sha":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeSHA)
	case "branch", "source_branch", "target_branch":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeBranch)
	case "label":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeLabel)
	case "milestone_id":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeMilestoneID)
	case "milestone":
		return h.completeMilestoneArgument(ctx, resolvedArgs, argValue)
	case "job_id":
		return h.completeJobArgument(ctx, resolvedArgs, argValue)
	default:
		return emptyResult(), nil
	}
}

func (h *Handler) completeMilestoneArgument(ctx context.Context, resolvedArgs map[string]string, argValue string) (*mcp.CompleteResult, error) {
	// Each scope is asked about on its own, like the branch and tag halves
	// above: the dispatch withheld this argument only if both listings were
	// excluded, and which one this call would use depends on the arguments
	// resolved so far. The scope decides, so an excluded project listing
	// answers empty rather than falling through to the group's.
	if pid, ok := resolvedArgs["project_id"]; ok && pid != "" {
		if h.withholds(actionMilestoneList) {
			return emptyResult(), nil
		}
		return h.completeMilestoneTitle(ctx, pid, argValue)
	}
	if gid, ok := resolvedArgs["group_id"]; ok && gid != "" {
		if h.withholds(actionGroupMilestoneLst) {
			return emptyResult(), nil
		}
		return h.completeGroupMilestoneTitle(ctx, gid, argValue)
	}
	return emptyResult(), nil
}

func (h *Handler) completeJobArgument(ctx context.Context, resolvedArgs map[string]string, argValue string) (*mcp.CompleteResult, error) {
	pid, hasPID := resolvedArgs["project_id"]
	plID, hasPLID := resolvedArgs["pipeline_id"]
	if hasPID && pid != "" && hasPLID && plID != "" {
		return h.completeJobID(ctx, pid, plID, argValue)
	}
	return emptyResult(), nil
}

// completeResourceArg completes parameters in resource URI templates.
//
// The pipeline, job and commit templates are routed here because no served
// prompt declares pipeline_id, sha or job_id, so a template is the one way a
// client reaches those completers. The job template names no pipeline, so its
// job_id is completed across the project's jobs.
func (h *Handler) completeResourceArg(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	argName := req.Params.Argument.Name
	argValue := req.Params.Argument.Value
	resolvedArgs := resolvedArguments(req)

	if h.withholds(completionBackingActions[argName]...) {
		return emptyResult(), nil
	}

	switch argName {
	case "project_id":
		return h.completeProjectID(ctx, argValue)
	case "group_id":
		return h.completeGroupID(ctx, argValue)
	case "merge_request_iid":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeMRIID)
	case "issue_iid":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeIssueIID)
	case "pipeline_id":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completePipelineID)
	case "sha":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeSHA)
	case "job_id":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeProjectJobID)
	default:
		return emptyResult(), nil
	}
}

// completionFunc is a completion handler that receives a project ID and partial query.
type completionFunc func(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error)

// completeWithProjectID dispatches to fn only if a resolved project_id exists; returns empty otherwise.
func (h *Handler) completeWithProjectID(ctx context.Context, resolvedArgs map[string]string, argValue string, fn completionFunc) (*mcp.CompleteResult, error) {
	pid, ok := resolvedArgs["project_id"]
	if !ok || pid == "" {
		return emptyResult(), nil
	}
	return fn(ctx, pid, argValue)
}

// completeProjectID searches projects matching the partial value.
func (h *Handler) completeProjectID(ctx context.Context, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchProjects(ctx, h.clientFor(ctx), query)
	if err != nil {
		slog.DebugContext(ctx, "completion: project search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeGroupID searches groups matching the partial value.
func (h *Handler) completeGroupID(ctx context.Context, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchGroups(ctx, h.clientFor(ctx), query)
	if err != nil {
		slog.DebugContext(ctx, "completion: group search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeMRIID completes a merge request IID in the given project, in any
// state. See [searchIIDs].
func (h *Handler) completeMRIID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, more, err := searchMRs(ctx, h.clientFor(ctx), projectID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: MR search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithMore(values, total, more), nil
}

// completeIssueIID completes an issue IID in the given project, in any state.
// See [searchIIDs].
func (h *Handler) completeIssueIID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, more, err := searchIssues(ctx, h.clientFor(ctx), projectID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: issue search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithMore(values, total, more), nil
}

// completeUsername searches GitLab users matching the partial value.
func (h *Handler) completeUsername(ctx context.Context, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchUsers(ctx, h.clientFor(ctx), query)
	if err != nil {
		slog.DebugContext(ctx, "completion: user search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeBranchOrTag returns branches and tags matching the partial value.
func (h *Handler) completeBranchOrTag(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	// The dispatch already withheld this argument if both listings were
	// excluded. Here each half is asked about on its own, so an operator who
	// removed only one still gets the other: this is the one completer that
	// merges two sources, and withholding it whole would take away data the
	// operator never excluded.
	var branches []string
	var branchTotal int
	if !h.withholds(actionBranchList) {
		var err error
		branches, branchTotal, err = searchBranches(ctx, h.clientFor(ctx), projectID, query)
		if err != nil {
			slog.DebugContext(ctx, "completion: branch search failed", "project", projectID, "query", query, "error", err)
			branches = nil
			branchTotal = 0
		}
	}

	var tags []string
	var tagTotal int
	if !h.withholds(actionTagList) {
		var err error
		tags, tagTotal, err = searchTags(ctx, h.clientFor(ctx), projectID, query)
		if err != nil {
			slog.DebugContext(ctx, "completion: tag search failed", "project", projectID, "query", query, "error", err)
			tags = nil
			tagTotal = 0
		}
	}

	branches = append(branches, tags...)
	return toResultWithTotal(branches, branchTotal+tagTotal), nil
}

// completeTag returns tags matching the partial value.
func (h *Handler) completeTag(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchTags(ctx, h.clientFor(ctx), projectID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: tag search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// idHalvesOf says which of an ID completer's two requests the operator left:
// the lookup, which returns what get returns, and the listing, which returns
// what list returns.
func (h *Handler) idHalvesOf(get, list string) idHalves {
	return idHalves{lookup: !h.withholds(get), list: !h.withholds(list)}
}

// completePipelineID completes a pipeline ID in a project. See [idSearch].
func (h *Handler) completePipelineID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, more, err := searchPipelines(ctx, h.clientFor(ctx), projectID, query, h.idHalvesOf(actionPipelineGet, actionPipelineList))
	if err != nil {
		slog.DebugContext(ctx, "completion: pipeline search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithMore(values, 0, more), nil
}

// completeSHA completes a commit SHA in a project. See [searchCommits].
func (h *Handler) completeSHA(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, more, err := searchCommits(ctx, h.clientFor(ctx), projectID, query, h.idHalvesOf(actionCommitGet, actionCommitList))
	if err != nil {
		slog.DebugContext(ctx, "completion: commit search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithMore(values, 0, more), nil
}

// completeBranch returns branches matching the partial value.
func (h *Handler) completeBranch(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchBranches(ctx, h.clientFor(ctx), projectID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: branch search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeLabel returns project labels matching the partial value.
func (h *Handler) completeLabel(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchLabels(ctx, h.clientFor(ctx), projectID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: label search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeMilestoneID returns project milestones matching the partial value.
func (h *Handler) completeMilestoneID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchMilestones(ctx, h.clientFor(ctx), projectID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: milestone search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeMilestoneTitle returns project milestone titles matching the partial value.
// Used by the milestone_progress prompt's "milestone" argument (title-based, not ID-based).
func (h *Handler) completeMilestoneTitle(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchMilestoneTitles(ctx, h.clientFor(ctx), projectID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: milestone title search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeGroupMilestoneTitle returns group milestone titles matching the partial value.
// Used by prompts whose milestone argument resolves against a group (for example
// group_milestone_progress) rather than a project.
func (h *Handler) completeGroupMilestoneTitle(ctx context.Context, groupID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchGroupMilestoneTitles(ctx, h.clientFor(ctx), groupID, query)
	if err != nil {
		slog.DebugContext(ctx, "completion: group milestone title search failed", "group", groupID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeJobID completes a job ID within one pipeline of a project. A
// pipeline_id that names no pipeline, not a number or not a positive one,
// answers empty rather than widening the search to the whole project.
func (h *Handler) completeJobID(ctx context.Context, projectID, pipelineIDStr, query string) (*mcp.CompleteResult, error) {
	plID, err := strconv.ParseInt(pipelineIDStr, 10, 64)
	if err != nil || plID < 1 {
		slog.DebugContext(ctx, "completion: invalid pipeline_id for job search", "pipeline_id", pipelineIDStr, "error", err)
		return emptyResult(), nil
	}
	return h.completeJobs(ctx, projectID, plID, query)
}

// completeProjectJobID completes a job ID across a project's jobs, for the job
// resource template, which names no pipeline.
func (h *Handler) completeProjectJobID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	return h.completeJobs(ctx, projectID, 0, query)
}

// completeJobs completes a job ID in a project, within one pipeline when
// pipelineID is positive. See [searchJobs].
func (h *Handler) completeJobs(ctx context.Context, projectID string, pipelineID int64, query string) (*mcp.CompleteResult, error) {
	values, more, err := searchJobs(ctx, h.clientFor(ctx), projectID, pipelineID, query, h.idHalvesOf(actionJobGet, actionJobList))
	if err != nil {
		slog.DebugContext(ctx, "completion: job search failed", "project", projectID, "pipeline_id", pipelineID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithMore(values, 0, more), nil
}

// resolvedArguments returns previously resolved arguments from the request context.
func resolvedArguments(req *mcp.CompleteRequest) map[string]string {
	if req.Params.Context != nil && req.Params.Context.Arguments != nil {
		return req.Params.Context.Arguments
	}
	return map[string]string{}
}

// emptyResult returns a completion result with no values.
func emptyResult() *mcp.CompleteResult {
	return &mcp.CompleteResult{
		Completion: mcp.CompletionResultDetails{
			Values: []string{},
		},
	}
}

// toResultWithTotal converts a string slice to a completion result, enforcing
// the max limit and setting HasMore when the input exceeds the cap. It exposes
// the upstream total when known (from gitlab.Response.TotalItems); a
// non-positive total is treated as unknown and omitted.
func toResultWithTotal(values []string, total int) *mcp.CompleteResult {
	// A nil slice marshals to null, and the MCP schema requires values to be
	// an array: a client that validates the response rejects it instead of
	// showing an empty list, which is the opposite of what this package
	// promises on a transient GitLab error. Normalized here rather than at the
	// one completer that can produce a nil today, because every result is
	// built through this function and a later one would reopen the hole.
	if values == nil {
		values = []string{}
	}
	hasMore := false
	if len(values) > maxCompletionResults {
		values = values[:maxCompletionResults]
		hasMore = true
	}
	if total > len(values) {
		hasMore = true
	}
	// A total of zero is left out of the JSON by the field's omitempty, so
	// clamping is all a non-positive total needs.
	return &mcp.CompleteResult{
		Completion: mcp.CompletionResultDetails{
			Values:  values,
			HasMore: hasMore,
			Total:   max(total, 0),
		},
	}
}

// toResultWithMore is [toResultWithTotal] for a search that knows more may
// match than its total says: GitLab named a next page, which it does whether
// or not it counted the matches, or the search asked about fewer items than
// could match.
func toResultWithMore(values []string, total int, more bool) *mcp.CompleteResult {
	res := toResultWithTotal(values, total)
	if more {
		res.Completion.HasMore = true
	}
	return res
}

// formatProjectEntry returns the project's path-with-namespace, the canonical
// identifier accepted by every GitLab API endpoint that takes a project_id.
func formatProjectEntry(_ int64, pathWithNamespace string) string {
	return pathWithNamespace
}

// formatGroupEntry returns the group's full path, the canonical identifier
// accepted by GitLab API endpoints that take a group_id.
func formatGroupEntry(_ int64, fullPath string) string {
	return fullPath
}

// formatMREntry returns the merge request IID as a string.
func formatMREntry(iid int64, _ string) string {
	return strconv.FormatInt(iid, 10)
}

// formatIssueEntry returns the issue IID as a string.
func formatIssueEntry(iid int64, _ string) string {
	return strconv.FormatInt(iid, 10)
}

// formatPipelineEntry returns the pipeline ID as a string.
func formatPipelineEntry(id int64, _, _ string) string {
	return strconv.FormatInt(id, 10)
}

// formatCommitEntry returns the commit short SHA.
func formatCommitEntry(shortID, _ string) string {
	return shortID
}

// formatMilestoneEntry returns the milestone ID as a string.
func formatMilestoneEntry(id int64, _ string) string {
	return strconv.FormatInt(id, 10)
}

// formatJobEntry returns the job ID as a string.
func formatJobEntry(id int64, _, _ string) string {
	return strconv.FormatInt(id, 10)
}
