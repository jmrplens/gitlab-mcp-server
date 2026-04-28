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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProjectFixture holds identifiers for a test project created by a fixture builder.
type ProjectFixture struct {
	ID   int64
	Path string
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
// sanitizeTestName converts a test name like
// "TestIndividual_Branches/Create" into a slug safe for GitLab project names.
// ---------------------------------------------------------------------------

// unsafeChars matches any character that is not lowercase alphanumeric or hyphen,
// used by [sanitizeTestName] to strip characters unsafe for GitLab project names.
var unsafeChars = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizeTestName converts a Go test name like
// "TestIndividual_Branches/Create" into a slug safe for GitLab project
// names by lowercasing, replacing separators with hyphens, stripping
// unsafe characters, and truncating to 40 characters.
func sanitizeTestName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = unsafeChars.ReplaceAllString(s, "")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
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
	var out O
	var err error
	for attempt := range maxRetries {
		out, err = fn()
		if err == nil {
			return out, nil
		}
		if attempt >= maxRetries-1 || !isRetryableError(err) {
			break
		}
		t.Logf("%s: attempt %d/%d failed (retrying): %v", label, attempt+1, maxRetries, err)
		select {
		case <-ctx.Done():
			return out, err
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return out, err
}

// ---------------------------------------------------------------------------
// Individual tool fixture builders (use sess.individual session)
// ---------------------------------------------------------------------------

// projectCreateRetries is the max number of attempts for project creation.
// GitLab CE has race conditions when many projects are created concurrently:
// spurious "already been taken" errors and transient connection resets.
const projectCreateRetries = 5

// createProject creates a private project via individual MCP tools and
// registers deletion in t.Cleanup.
func createProject(ctx context.Context, t *testing.T, session *mcp.ClientSession) ProjectFixture {
	t.Helper()
	var out projects.Output
	var err error
	for attempt := range projectCreateRetries {
		name := uniqueName(e2eProjectPrefix + sanitizeTestName(t.Name()))
		out, err = callToolOn[projects.Output](ctx, session, "gitlab_project_create", projects.CreateInput{
			Name:                 name,
			Description:          "E2E: " + t.Name(),
			Visibility:           "private",
			InitializeWithReadme: true,
			DefaultBranch:        defaultBranch,
		})
		if err == nil {
			break
		}
		retryable := strings.Contains(err.Error(), "already been taken") || isTransientNetworkError(err)
		if retryable && attempt < projectCreateRetries-1 {
			t.Logf("project create attempt %d failed, retrying: %v", attempt+1, err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
	}
	requireNoError(t, err, "create project fixture")

	t.Cleanup(func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = callToolVoidOn(delCtx, session, "gitlab_project_delete", projects.DeleteInput{
			ProjectID:         toolutil.StringOrInt(strconv.FormatInt(out.ID, 10)),
			PermanentlyRemove: true,
			FullPath:          out.PathWithNamespace,
		})
	})

	// Wait for the default branch to be available.
	waitForBranchOn(ctx, t, session, out.ID, defaultBranch)

	return ProjectFixture{ID: out.ID, Path: out.PathWithNamespace}
}

// createProjectMeta creates a private project via the gitlab_project meta-tool
// and registers deletion in t.Cleanup.
func createProjectMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession) ProjectFixture {
	t.Helper()
	var out projects.Output
	var err error
	for attempt := range projectCreateRetries {
		name := uniqueName(e2eProjectPrefix + "meta-" + sanitizeTestName(t.Name()))
		out, err = callToolOn[projects.Output](ctx, session, "gitlab_project", map[string]any{
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
			break
		}
		retryable := strings.Contains(err.Error(), "already been taken") || isTransientNetworkError(err)
		if retryable && attempt < projectCreateRetries-1 {
			t.Logf("project create attempt %d failed, retrying: %v", attempt+1, err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
	}
	requireNoError(t, err, "create project fixture (meta)")

	t.Cleanup(func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = callToolVoidOn(delCtx, session, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id":         strconv.FormatInt(out.ID, 10),
				"permanently_remove": true,
				"full_path":          out.PathWithNamespace,
			},
		})
	})

	// Wait for the default branch to be available.
	waitForBranchOn(ctx, t, session, out.ID, defaultBranch)

	return ProjectFixture{ID: out.ID, Path: out.PathWithNamespace}
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
	for attempt := range maxRetries {
		out, err := callToolOn[commits.Output](ctx, session, "gitlab_commit_create", commits.CreateInput{
			ProjectID:     proj.pidOf(),
			Branch:        branch,
			CommitMessage: message,
			Actions: []commits.Action{
				{Action: "create", FilePath: path, Content: content},
			},
		})
		if err == nil {
			return CommitFixture{SHA: out.ID, ShortID: out.ShortID}
		}
		if attempt < maxRetries-1 && strings.Contains(err.Error(), "only create or edit files when you are on a branch") {
			t.Logf("commitFile %s: retry %d/%d (branch not ready)", path, attempt+1, maxRetries)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		requireNoError(t, err, "commit file "+path)
	}
	t.Fatalf("commitFile %s: exhausted %d retries", path, maxRetries)
	return CommitFixture{}
}

// commitFileMeta creates a file via the gitlab_repository meta-tool.
// Retries on transient "not on a branch" errors caused by GitLab CE race conditions.
func commitFileMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branch, path, content, message string) CommitFixture {
	t.Helper()
	const maxRetries = 8
	needStartBranch := false
	for attempt := range maxRetries {
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
			return CommitFixture{SHA: out.ID, ShortID: out.ShortID}
		}
		if attempt < maxRetries-1 {
			errMsg := err.Error()
			if strings.Contains(errMsg, "only create or edit files when you are on a branch") {
				needStartBranch = true
				t.Logf("commitFileMeta %s: retry %d/%d (branch not ready, adding start_branch)", path, attempt+1, maxRetries)
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			if strings.Contains(errMsg, "already exists") {
				needStartBranch = false
				t.Logf("commitFileMeta %s: retry %d/%d (branch already exists, removing start_branch)", path, attempt+1, maxRetries)
				time.Sleep(time.Second)
				continue
			}
		}
		requireNoError(t, err, "commit file meta "+path)
	}
	t.Fatalf("commitFileMeta %s: exhausted %d retries", path, maxRetries)
	return CommitFixture{}
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
// progressive backoff. Transient errors (429, 5xx, network) are retried silently.
func waitForBranchOn(ctx context.Context, t *testing.T, _ *mcp.ClientSession, projectID int64, branch string) {
	t.Helper()
	drainSidekiq(ctx, t)
	pid := int(projectID)

	const maxWait = 90 * time.Second
	deadline := time.Now().Add(maxWait)
	delay := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		_, resp, err := sess.glClient.GL().Branches.GetBranch(pid, branch)
		if err == nil {
			t.Logf("Branch %q ready in project %d", branch, projectID)
			return
		}

		retryable := false
		if resp == nil {
			// Network-level error (EOF, connection reset) — always retryable.
			retryable = true
		} else {
			switch {
			case resp.StatusCode == http.StatusNotFound:
				retryable = true
			case resp.StatusCode == http.StatusTooManyRequests:
				retryable = true
			case resp.StatusCode >= 500:
				retryable = true
			}
		}
		if !retryable {
			requireNoError(t, err, fmt.Sprintf("get branch %s in project %d", branch, projectID))
		}

		select {
		case <-ctx.Done():
			t.Fatalf("context canceled waiting for branch %q: %v", branch, ctx.Err())
		case <-time.After(delay):
		}

		// Progressive backoff: 500ms → 1s → 2s (capped)
		if delay < 2*time.Second {
			delay *= 2
		}
	}
	t.Fatalf("branch %q not available in project %d after %s", branch, projectID, maxWait)
}

// waitForMRReady polls the MR until the GitLab DetailedMergeStatus leaves the
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
func waitForMRReady(ctx context.Context, t *testing.T, projectID, mrIID int64) {
t.Helper()
if sess.glClient == nil {
return
}
drainSidekiq(ctx, t)
const maxWait = 120 * time.Second
deadline := time.Now().Add(maxWait)
delay := 500 * time.Millisecond
for time.Now().Before(deadline) {
mr, _, err := sess.glClient.GL().MergeRequests.GetMergeRequest(int(projectID), mrIID, nil)
if err == nil {
switch mr.DetailedMergeStatus {
case "preparing", "checking", "unchecked", "":
// still computing; continue polling
default:
t.Logf("MR !%d ready in project %d: detailed_merge_status=%s", mrIID, projectID, mr.DetailedMergeStatus)
return
}
}
select {
case <-ctx.Done():
return
case <-time.After(delay):
}
if delay < 2*time.Second {
delay *= 2
}
}
t.Logf("MR !%d not ready after %s (continuing; final assertion will report state)", mrIID, maxWait)
}
