// Package completions provides a CompletionHandler for GitLab-aware autocomplete
// of prompt arguments and resource URI template parameters.
package completions

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

const (
	maxCompletionResults = 10
	entryFmt             = "%d: %s"
	entryWithStatusFmt   = "%d: %s (%s)"
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
	case "mr_iid":
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
		return h.completeWithProjectID(ctx, resolvedArgs, argValue, h.completeMilestoneTitle)
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
	case "mr_iid":
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
	values, err := searchProjects(ctx, h.client, query)
	if err != nil {
		slog.Debug("completion: project search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeGroupID searches groups matching the partial value.
func (h *Handler) completeGroupID(ctx context.Context, query string) (*mcp.CompleteResult, error) {
	values, err := searchGroups(ctx, h.client, query)
	if err != nil {
		slog.Debug("completion: group search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
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
	values, err := searchUsers(ctx, h.client, query)
	if err != nil {
		slog.Debug("completion: user search failed", "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeBranchOrTag returns branches and tags matching the partial value.
func (h *Handler) completeBranchOrTag(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	branches, err := searchBranches(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: branch search failed", "project", projectID, "query", query, "error", err)
		branches = nil
	}

	tags, err := searchTags(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: tag search failed", "project", projectID, "query", query, "error", err)
		tags = nil
	}

	branches = append(branches, tags...)
	return toResult(branches), nil
}

// completeTag returns tags matching the partial value.
func (h *Handler) completeTag(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchTags(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: tag search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
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
	values, err := searchBranches(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: branch search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeLabel returns project labels matching the partial value.
func (h *Handler) completeLabel(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchLabels(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: label search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeMilestoneID returns project milestones matching the partial value.
func (h *Handler) completeMilestoneID(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchMilestones(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: milestone search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
}

// completeMilestoneTitle returns project milestone titles matching the partial value.
// Used by the milestone_progress prompt's "milestone" argument (title-based, not ID-based).
func (h *Handler) completeMilestoneTitle(ctx context.Context, projectID, query string) (*mcp.CompleteResult, error) {
	values, err := searchMilestoneTitles(ctx, h.client, projectID, query)
	if err != nil {
		slog.Debug("completion: milestone title search failed", "project", projectID, "query", query, "error", err)
		return emptyResult(), nil
	}
	return toResult(values), nil
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

// toResult converts a string slice to a completion result, enforcing the max limit.
func toResult(values []string) *mcp.CompleteResult {
	hasMore := false
	total := len(values)
	if total > maxCompletionResults {
		values = values[:maxCompletionResults]
		hasMore = true
	}
	return &mcp.CompleteResult{
		Completion: mcp.CompletionResultDetails{
			Values:  values,
			HasMore: hasMore,
			Total:   total,
		},
	}
}

// formatProjectEntry formats a project as "id: path" for completion display.
func formatProjectEntry(id int64, pathWithNamespace string) string {
	return fmt.Sprintf(entryFmt, id, pathWithNamespace)
}

// formatGroupEntry formats a group as "id: full_path" for completion display.
func formatGroupEntry(id int64, fullPath string) string {
	return fmt.Sprintf(entryFmt, id, fullPath)
}

// formatMREntry formats a merge request as "iid: title" for completion display.
func formatMREntry(iid int64, title string) string {
	return fmt.Sprintf(entryFmt, iid, truncate(title, 60))
}

// formatIssueEntry formats an issue as "iid: title" for completion display.
func formatIssueEntry(iid int64, title string) string {
	return fmt.Sprintf(entryFmt, iid, truncate(title, 60))
}

// formatPipelineEntry formats a pipeline as "id: ref (status)" for completion display.
func formatPipelineEntry(id int64, ref, status string) string {
	return fmt.Sprintf(entryWithStatusFmt, id, ref, status)
}

// formatCommitEntry formats a commit as "short_id: title" for completion display.
func formatCommitEntry(shortID, title string) string {
	return fmt.Sprintf("%s: %s", shortID, truncate(title, 60))
}

// formatMilestoneEntry formats a milestone as "id: title" for completion display.
func formatMilestoneEntry(id int64, title string) string {
	return fmt.Sprintf(entryFmt, id, truncate(title, 60))
}

// formatJobEntry formats a job as "id: name (status)" for completion display.
func formatJobEntry(id int64, name, status string) string {
	return fmt.Sprintf(entryWithStatusFmt, id, name, status)
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
