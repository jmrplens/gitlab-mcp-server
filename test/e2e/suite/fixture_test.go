//go:build e2e

// fixture_test.go provides self-contained GitLab resource builders for E2E
// tests. Each builder creates a real resource via MCP tools and registers
// automatic cleanup via t.Cleanup(). Domain test files use these builders
// instead of relying on mutable global state.
package suite

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ProjectFixture holds identifiers for a test project created by a fixture builder.
type ProjectFixture struct {
	ID   int64
	Path string
}

// GroupFixture holds identifiers for a test group created by a fixture builder.
type GroupFixture struct {
	ID   int64
	Path string
}

// gidStr returns the group ID as a plain string for use in meta-tool params.
func (f GroupFixture) gidStr() string {
	return strconv.FormatInt(f.ID, 10)
}

// pidOf returns the project ID as a StringOrInt for use in individual tool inputs.
func (f ProjectFixture) pidOf() toolutil.StringOrInt {
	return toolutil.StringOrInt(strconv.FormatInt(f.ID, 10))
}

// pidStr returns the project ID as a plain string for use in meta-tool params.
func (f ProjectFixture) pidStr() string {
	return strconv.FormatInt(f.ID, 10)
}

// BranchFixture holds a branch name and the commit SHA it points to.
type BranchFixture struct {
	Name     string
	CommitID string
}

// IssueFixture holds identifiers for a test issue.
type IssueFixture struct {
	IID int64
}

// MRFixture holds identifiers for a test merge request.
type MRFixture struct {
	IID int64
}

// CommitFixture holds the SHA of a test commit.
type CommitFixture struct {
	SHA     string
	ShortID string
}

// ---------------------------------------------------------------------------
// Retry helpers for flaky E2E operations
// ---------------------------------------------------------------------------

// isRetryableError checks whether an error is likely transient and worth
// retrying. Covers network-level failures, GitLab rate limits, server errors,
// and known eventual-consistency conditions (e.g. newly created Git refs not
// yet visible to subsequent API calls).
//
// Uses anchored patterns to avoid false positives from bare numeric substrings
// like "404" appearing in project IDs, commit SHAs, or resource names.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if isTransientNetworkError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	// GitLab eventual-consistency: branch not yet visible after creation.
	if strings.Contains(msg, "only create or edit files when you are on a branch") {
		return true
	}
	// 404 — newly created ref not yet propagated (anchored to avoid matching IDs).
	if strings.Contains(msg, "404 not found") {
		return true
	}
	// Rate limiting.
	if strings.Contains(msg, "429") {
		return true
	}
	// Server errors (transient by nature).
	if strings.Contains(msg, "500 internal server error") ||
		strings.Contains(msg, "502 bad gateway") ||
		strings.Contains(msg, "503 service unavailable") {
		return true
	}
	// GitLab NotificationSetting race: first_or_initialize_by collides with
	// concurrent upsert producing a UniqueConstraint on user_id.
	if strings.Contains(msg, "already exists in source") {
		return true
	}
	return false
}

// retryOnTransient calls fn up to maxRetries times with progressive backoff
// (1s, 2s, 3s…) when the returned error is retryable. Respects context
// cancellation between attempts. Returns the first successful result or
// the last error after all attempts are exhausted.
func retryOnTransient[O any](ctx context.Context, t *testing.T, label string, maxRetries int, fn func() (O, error)) (O, error) {
	t.Helper()
	return retryWithBackoff(ctx, t, label, maxRetries, func(int) (O, bool, string, error) {
		out, err := fn()
		if err == nil {
			return out, false, "", nil
		}
		return out, isRetryableError(err), "retryable error", err
	})
}

// ---------------------------------------------------------------------------
// Individual tool fixture builders (use sess.individual session)
// ---------------------------------------------------------------------------

// projectCreateRetries is the max number of attempts for project creation.
// GitLab CE has race conditions when many projects are created concurrently:
// spurious "already been taken" errors and transient connection resets.
const projectCreateRetries = 5

// CreateProject creates a private project via individual MCP tools and
// registers deletion in the per-test resource ledger.
func CreateProject(ctx context.Context, e2e *E2EContext, session *mcp.ClientSession) ProjectFixture {
	e2e.T.Helper()
	if session == nil {
		e2e.T.Skip("project fixture MCP session not configured")
	}
	t := e2e.T
	out, err := retryWithBackoff(ctx, t, "create project fixture", projectCreateRetries, func(int) (projects.Output, bool, string, error) {
		name := uniqueName(e2eProjectPrefix + sanitizeTestName(t.Name()))
		out, err := callToolOn[projects.Output](ctx, session, "gitlab_project_create", projects.CreateInput{
			Name:                 name,
			Description:          "E2E: " + t.Name(),
			Visibility:           "private",
			InitializeWithReadme: true,
			DefaultBranch:        defaultBranch,
		})
		if err == nil {
			return out, false, "", nil
		}
		retryable := strings.Contains(err.Error(), "already been taken") || isTransientNetworkError(err)
		return out, retryable, "name collision or transient network error", err
	})
	requireNoError(t, err, "create project fixture")

	e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindProject,
		ID:        strconv.FormatInt(out.ID, 10),
		Path:      out.PathWithNamespace,
		Name:      out.Name,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			return callToolVoidOn(cleanupCtx, session, "gitlab_project_delete", projects.DeleteInput{
				ProjectID:         toolutil.StringOrInt(strconv.FormatInt(out.ID, 10)),
				PermanentlyRemove: true,
				FullPath:          out.PathWithNamespace,
			})
		},
	})

	// Wait for the default branch to be available.
	waitForBranchOn(ctx, t, e2e.GitLab, out.ID, defaultBranch)

	return ProjectFixture{ID: out.ID, Path: out.PathWithNamespace}
}

// createProject keeps legacy call sites working while they migrate to E2EContext.
func createProject(ctx context.Context, t *testing.T, session *mcp.ClientSession) ProjectFixture {
	t.Helper()
	return CreateProject(ctx, NewE2EContext(t), session)
}

// CreateProjectMeta creates a private project via the gitlab_project meta-tool
// and registers deletion in the per-test resource ledger.
func CreateProjectMeta(ctx context.Context, e2e *E2EContext, session *mcp.ClientSession) ProjectFixture {
	e2e.T.Helper()
	if session == nil {
		e2e.T.Skip("project fixture MCP session not configured")
	}
	t := e2e.T
	out, err := retryWithBackoff(ctx, t, "create project fixture (meta)", projectCreateRetries, func(int) (projects.Output, bool, string, error) {
		name := uniqueName(e2eProjectPrefix + "meta-" + sanitizeTestName(t.Name()))
		out, err := callToolOn[projects.Output](ctx, session, "gitlab_project", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name":                   name,
				"description":            "E2E meta: " + t.Name(),
				"visibility":             "private",
				"initialize_with_readme": true,
				"default_branch":         defaultBranch,
			},
		})
		if err == nil {
			return out, false, "", nil
		}
		retryable := strings.Contains(err.Error(), "already been taken") || isTransientNetworkError(err)
		return out, retryable, "name collision or transient network error", err
	})
	requireNoError(t, err, "create project fixture (meta)")

	e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindProject,
		ID:        strconv.FormatInt(out.ID, 10),
		Path:      out.PathWithNamespace,
		Name:      out.Name,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			return callToolVoidOn(cleanupCtx, session, "gitlab_project", map[string]any{
				"action": "delete",
				"params": map[string]any{
					"project_id":         strconv.FormatInt(out.ID, 10),
					"permanently_remove": true,
					"full_path":          out.PathWithNamespace,
				},
			})
		},
	})

	// Wait for the default branch to be available.
	waitForBranchOn(ctx, t, e2e.GitLab, out.ID, defaultBranch)

	return ProjectFixture{ID: out.ID, Path: out.PathWithNamespace}
}

// createProjectMeta keeps legacy call sites working while they migrate to E2EContext.
func createProjectMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession) ProjectFixture {
	t.Helper()
	return CreateProjectMeta(ctx, NewE2EContext(t), session)
}

// CreateGroupMeta creates a group via the gitlab_group meta-tool and registers
// deletion in the per-test resource ledger.
func CreateGroupMeta(ctx context.Context, e2e *E2EContext, session *mcp.ClientSession, namePrefix string) GroupFixture {
	e2e.T.Helper()
	if session == nil {
		e2e.T.Skip("group fixture MCP session not configured")
	}
	t := e2e.T
	name := uniqueName(namePrefix)
	out, err := callToolOn[groups.Output](ctx, session, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name": name,
			"path": name,
		},
	})
	requireNoError(t, err, "create group fixture (meta)")

	id := strconv.FormatInt(out.ID, 10)
	e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindGroup,
		ID:        id,
		Path:      out.FullPath,
		Name:      out.Name,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			return callToolVoidOn(cleanupCtx, session, "gitlab_group", map[string]any{
				"action": "delete",
				"params": map[string]any{"group_id": id},
			})
		},
	})

	return GroupFixture{ID: out.ID, Path: out.FullPath}
}

// unprotectMain removes protection from the main branch so commits can
// be pushed. Uses the GitLab client directly for efficiency.
func unprotectMain(ctx context.Context, t *testing.T, proj ProjectFixture) {
	t.Helper()
	_ = ctx
	_, err := sess.glClient.GL().ProtectedBranches.UnprotectRepositoryBranches(int(proj.ID), defaultBranch)
	if err != nil {
		t.Logf("unprotect main (non-fatal, may already be unprotected): %v", err)
	}
}

// commitFile creates a file via the gitlab_commit_create tool.
func commitFile(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branch, path, content, message string) CommitFixture {
	t.Helper()
	const maxRetries = 5
	fixture, err := retryWithBackoff(ctx, t, "commit file "+path, maxRetries, func(int) (CommitFixture, bool, string, error) {
		out, err := callToolOn[commits.Output](ctx, session, "gitlab_commit_create", commits.CreateInput{
			ProjectID:     proj.pidOf(),
			Branch:        branch,
			CommitMessage: message,
			Actions: []commits.Action{
				{Action: "create", FilePath: path, Content: content},
			},
		})
		if err == nil {
			return CommitFixture{SHA: out.ID, ShortID: out.ShortID}, false, "", nil
		}
		retryable := strings.Contains(err.Error(), "only create or edit files when you are on a branch")
		return CommitFixture{}, retryable, "branch not ready", err
	})
	requireNoError(t, err, "commit file "+path)
	return fixture
}

// commitFileMeta creates a file via the gitlab_repository meta-tool.
// Retries on transient "not on a branch" errors caused by GitLab CE race conditions.
func commitFileMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branch, path, content, message string) CommitFixture {
	t.Helper()
	const maxRetries = 8
	needStartBranch := false
	fixture, err := retryWithBackoff(ctx, t, "commit file meta "+path, maxRetries, func(int) (CommitFixture, bool, string, error) {
		params := map[string]any{
			"project_id":     proj.pidStr(),
			"branch":         branch,
			"commit_message": message,
			"actions": []map[string]any{
				{"action": "create", "file_path": path, "content": content},
			},
		}
		if needStartBranch && branch != defaultBranch {
			params["start_branch"] = defaultBranch
		}
		out, err := callToolOn[commits.Output](ctx, session, "gitlab_repository", map[string]any{
			"action": "commit_create",
			"params": params,
		})
		if err == nil {
			return CommitFixture{SHA: out.ID, ShortID: out.ShortID}, false, "", nil
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "only create or edit files when you are on a branch") {
			needStartBranch = true
			return CommitFixture{}, true, "branch not ready, adding start_branch", err
		}
		if strings.Contains(errMsg, "already exists") {
			needStartBranch = false
			return CommitFixture{}, true, "branch already exists, removing start_branch", err
		}
		return CommitFixture{}, false, "", err
	})
	requireNoError(t, err, "commit file meta "+path)
	return fixture
}

// createBranch creates a branch from the default branch via individual tools.
func createBranch(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branchName string) BranchFixture {
	t.Helper()
	out, err := callToolOn[branches.Output](ctx, session, "gitlab_branch_create", branches.CreateInput{
		ProjectID:  proj.pidOf(),
		BranchName: branchName,
		Ref:        defaultBranch,
	})
	requireNoError(t, err, "create branch "+branchName)
	return BranchFixture{Name: out.Name, CommitID: out.CommitID}
}

// createBranchMeta creates a branch via the gitlab_branch meta-tool.
func createBranchMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branchName string) BranchFixture {
	t.Helper()
	out, err := callToolOn[branches.Output](ctx, session, "gitlab_branch", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"branch_name": branchName,
			"ref":         defaultBranch,
		},
	})
	requireNoError(t, err, "create branch meta "+branchName)
	return BranchFixture{Name: out.Name, CommitID: out.CommitID}
}

// createIssue creates an issue via individual tools.
func createIssue(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, title string) IssueFixture {
	t.Helper()
	out, err := callToolOn[issues.Output](ctx, session, "gitlab_issue_create", issues.CreateInput{
		ProjectID:   proj.pidOf(),
		Title:       title,
		Description: "E2E test issue for " + t.Name(),
	})
	requireNoError(t, err, "create issue")
	return IssueFixture{IID: out.IID}
}

// createIssueMeta creates an issue via the gitlab_issue meta-tool.
func createIssueMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, title string) IssueFixture {
	t.Helper()
	out, err := callToolOn[issues.Output](ctx, session, "gitlab_issue", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"title":       title,
			"description": "E2E meta test issue for " + t.Name(),
		},
	})
	requireNoError(t, err, "create issue meta")
	return IssueFixture{IID: out.IID}
}

// createMR creates a merge request via individual tools. Requires a feature
// branch with at least one commit different from the target branch.
func createMR(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, source, target, title string) MRFixture {
	t.Helper()
	out, err := callToolOn[mergerequests.Output](ctx, session, "gitlab_mr_create", mergerequests.CreateInput{
		ProjectID:    proj.pidOf(),
		SourceBranch: source,
		TargetBranch: target,
		Title:        title,
	})
	requireNoError(t, err, "create MR")
	return MRFixture{IID: out.IID}
}

// createMRMeta creates a merge request via the gitlab_merge_request meta-tool.
func createMRMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, source, target, title string) MRFixture {
	t.Helper()
	out, err := callToolOn[mergerequests.Output](ctx, session, "gitlab_merge_request", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":    proj.pidStr(),
			"source_branch": source,
			"target_branch": target,
			"title":         title,
		},
	})
	requireNoError(t, err, "create MR meta")
	return MRFixture{IID: out.IID}
}

// ---------------------------------------------------------------------------
// Wait helpers (session-parameterized)
// ---------------------------------------------------------------------------

// waitForBranchOn polls the GitLab API until the named branch exists in the
// given project or the context is canceled. Under parallel load (~60 projects)
// branch creation can take well over 30s, so we allow up to 90s with
// bounded polling. Transient errors (429, 5xx, network) are retried silently.
func waitForBranchOn(ctx context.Context, t *testing.T, client *gitlabclient.Client, projectID int64, branch string) {
	t.Helper()
	drainSidekiq(ctx, t, client)
	requireNoError(t, waitForBranch(ctx, client, projectID, branch), fmt.Sprintf("wait for branch %s in project %d", branch, projectID))
}

func waitForBranch(ctx context.Context, client *gitlabclient.Client, projectID int64, branch string) error {
	if client == nil {
		return fmt.Errorf("gitlab client not configured")
	}
	pid := int(projectID)

	const maxWait = 90 * time.Second
	pollCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	return Poll(pollCtx, 500*time.Millisecond, maxWait, func() (bool, string, error) {
		_, resp, err := client.GL().Branches.GetBranch(pid, branch, gl.WithContext(pollCtx))
		if err == nil {
			return true, fmt.Sprintf("branch %q ready in project %d", branch, projectID), nil
		}

		state := fmt.Sprintf("branch %q in project %d: %v", branch, projectID, err)
		if resp != nil {
			state = fmt.Sprintf("branch %q in project %d: HTTP %d", branch, projectID, resp.StatusCode)
		}
		if !retryableBranchResponse(resp) {
			return false, state, fmt.Errorf("get branch %q in project %d: %w", branch, projectID, err)
		}
		return false, state, nil
	})
}

func retryableBranchResponse(resp *gl.Response) bool {
	if resp == nil {
		return true
	}
	return resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
}

// waitForMRReady polls the MR until the GitLab DetailedMergeStatus leaves the
// transitional values ("preparing", "checking", "unchecked", "") that GitLab
// CE reports while still computing the diff and merge ref. Downstream
// endpoints such as /merge_requests/:iid/commits and /diff_versions return
// empty data while the MR is in those states, so calling this helper after
// MR creation prevents flaky failures on slow Docker GitLab CE environments.
//
// The helper is best-effort: it never fails the test on its own. If the MR
// never leaves the transitional state within maxWait, it logs and returns so
// that the test's own assertion produces the actionable failure message.
func waitForMRReady(ctx context.Context, t *testing.T, client *gitlabclient.Client, projectID, mrIID int64) {
	t.Helper()
	drainSidekiq(ctx, t, client)
	if err := waitForMRReadyState(ctx, client, projectID, mrIID); err != nil {
		t.Logf("MR !%d readiness wait in project %d ended: %v", mrIID, projectID, err)
	}
}

func waitForMRReadyState(ctx context.Context, client *gitlabclient.Client, projectID, mrIID int64) error {
	if client == nil {
		return nil
	}
	const maxWait = 120 * time.Second
	pollCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	return Poll(pollCtx, 500*time.Millisecond, maxWait, func() (bool, string, error) {
		mr, _, err := client.GL().MergeRequests.GetMergeRequest(int(projectID), mrIID, nil, gl.WithContext(pollCtx))
		if err == nil {
			switch mr.DetailedMergeStatus {
			case "preparing", "checking", "unchecked", "":
				return false, fmt.Sprintf("detailed_merge_status=%q", mr.DetailedMergeStatus), nil
			default:
				return true, fmt.Sprintf("detailed_merge_status=%q", mr.DetailedMergeStatus), nil
			}
		}
		if pollCtx.Err() != nil {
			return false, "", nil
		}
		return false, fmt.Sprintf("error polling MR !%d in project %d: %v", mrIID, projectID, err), nil
	})
}
