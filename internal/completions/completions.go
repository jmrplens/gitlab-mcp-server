// Package completions provides a CompletionHandler for GitLab-aware autocomplete
// of prompt arguments and resource URI template parameters.
//
// It queries GitLab search and project endpoints to return canonical argument
// values suitable for MCP completion results.
package completions

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
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
	client *gitlabclient.Client
}

// NewHandler creates a completion handler backed by the given GitLab client.
func NewHandler(client *gitlabclient.Client) *Handler {
	return &Handler{client: client}
}

// Complete dispatches completion requests based on reference type and argument name.
// It returns empty results on errors to avoid blocking the client.
func (h *Handler) Complete(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	if req.Params.Ref == nil {
		return emptyResult(), nil
	}

	switch req.Params.Ref.Type {
	case "ref/prompt":
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
		// Prefer project scope; fall back to group scope when no project_id
		// is resolved (e.g. group_milestone_progress prompt).
		if pid, ok := resolvedArgs["project_id"]; ok && pid != "" {
			return h.completeMilestoneTitle(ctx, pid, argValue)
		}
		if gid, ok := resolvedArgs["group_id"]; ok && gid != "" {
			return h.completeGroupMilestoneTitle(ctx, gid, argValue)
		}
		return emptyResult(), nil
	case "job_id":
		pid, hasPID := resolvedArgs["project_id"]
		plID, hasPLID := resolvedArgs["pipeline_id"]
		if hasPID && pid != "" && hasPLID && plID != "" {
			return h.completeJobID(ctx, pid, plID, argValue)
		}
		return emptyResult(), nil
	default:
		return emptyResult(), nil
	}
}

// completeResourceArg completes parameters in resource URI templates.
func (h *Handler) completeResourceArg(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	argName := req.Params.Argument.Name
	argValue := req.Params.Argument.Value
	resolvedArgs := resolvedArguments(req)

	switch argName {
	case "project_id":
		return h.completeProjectID(ctx, argValue)
	case "group_id":
		return h.completeGroupID(ctx, argValue)
	case "merge_request_iid":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeMRIID)
	case "issue_iid":
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeIssueIID)
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
	values, total, err := searchProjects(ctx, h.client, query)
	if err != nil {
		slog.Debug("completion: project search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeGroupID searches groups matching the partial value.
func (h *Handler) completeGroupID(ctx context.Context, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchGroups(ctx, h.client, query)
	if err != nil {
		slog.Debug("completion: group search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeMRIID lists open MRs for the given project and filters by IID prefix.
func (h *Handler) completeMRIID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchMRs(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: MR search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeIssueIID lists open issues for the given project and filters by IID prefix.
func (h *Handler) completeIssueIID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchIssues(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: issue search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeUsername searches GitLab users matching the partial value.
func (h *Handler) completeUsername(ctx context.Context, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchUsers(ctx, h.client, query)
	if err != nil {
		slog.Debug("completion: user search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeBranchOrTag returns branches and tags matching the partial value.
func (h *Handler) completeBranchOrTag(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	branches, branchTotal, err := searchBranches(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: branch search failed", "project", projectID, "query", query, "error", err)
		branches = nil
		branchTotal = 0
	}

	tags, tagTotal, err := searchTags(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: tag search failed", "project", projectID, "query", query, "error", err)
		tags = nil
		tagTotal = 0
	}

	branches = append(branches, tags...)
	return toResultWithTotal(branches, branchTotal+tagTotal), nil
}

// completeTag returns tags matching the partial value.
func (h *Handler) completeTag(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchTags(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: tag search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completePipelineID lists recent pipelines for a project, filtered by ID prefix.
func (h *Handler) completePipelineID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchPipelines(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: pipeline search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeSHA lists recent commits for a project, filtered by SHA prefix.
func (h *Handler) completeSHA(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchCommits(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: commit search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeBranch returns branches matching the partial value.
func (h *Handler) completeBranch(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchBranches(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: branch search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeLabel returns project labels matching the partial value.
func (h *Handler) completeLabel(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchLabels(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: label search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeMilestoneID returns project milestones matching the partial value.
func (h *Handler) completeMilestoneID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchMilestones(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: milestone search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeMilestoneTitle returns project milestone titles matching the partial value.
// Used by the milestone_progress prompt's "milestone" argument (title-based, not ID-based).
func (h *Handler) completeMilestoneTitle(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchMilestoneTitles(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: milestone title search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeGroupMilestoneTitle returns group milestone titles matching the partial value.
// Used by prompts whose milestone argument resolves against a group (for example
// group_milestone_progress) rather than a project.
func (h *Handler) completeGroupMilestoneTitle(ctx context.Context, groupID, query string) (*mcp.CompleteResult, error) {
	values, total, err := searchGroupMilestoneTitles(ctx, h.client, groupID, query)
	if err != nil {
		slog.Debug("completion: group milestone title search failed", "group", groupID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResultWithTotal(values, total), nil
}

// completeJobID lists jobs for a pipeline, filtered by ID prefix.
func (h *Handler) completeJobID(ctx context.Context, projectID, pipelineIDStr, query string) (*mcp.CompleteResult, error) {
	plID, err := strconv.ParseInt(pipelineIDStr, 10, 64)
	if err != nil {
		slog.Debug("completion: invalid pipeline_id for job search", "pipeline_id", pipelineIDStr, "error", err)
		return emptyResult(), nil
	}
	values, err := searchJobs(ctx, h.client, projectID, plID, query)
	if err != nil {
		slog.Debug("completion: job search failed", "project", projectID, "pipeline_id", plID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
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

// toResult converts a string slice to a completion result, enforcing the max
// limit. Sets HasMore=true when the input exceeds the cap. Total is left at 0
// (omitted from JSON) because the upstream slice does not carry a true count
// of all matching items — only what we fetched. Use [toResultWithTotal] when
// the upstream pagination header (X-Total) is available.
func toResult(values []string) *mcp.CompleteResult {
	return toResultWithTotal(values, 0)
}

// toResultWithTotal is like [toResult] but exposes the upstream total when
// known (e.g. from gitlab.Response.TotalItems). A non-positive total is
// treated as unknown and omitted.
func toResultWithTotal(values []string, total int) *mcp.CompleteResult {
	hasMore := false
	if len(values) > maxCompletionResults {
		values = values[:maxCompletionResults]
		hasMore = true
	}
	if total > len(values) {
		hasMore = true
	}
	res := &mcp.CompleteResult{
		Completion: mcp.CompletionResultDetails{
			Values:  values,
			HasMore: hasMore,
		},
	}
	if total > 0 {
		res.Completion.Total = total
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

// filterByPrefix returns only values that contain the query (case-insensitive).
func filterByPrefix(values []string, query string) []string {
	if query == "" {
		return values
	}
	q := strings.ToLower(query)
	var filtered []string
	for _, v := range values {
		if strings.Contains(strings.ToLower(v), q) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}
