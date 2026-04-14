//go:build e2e

// fixture_test.go provides self-contained GitLab resource builders for E2E
// tests. Each builder creates a real resource via MCP tools and registers
// automatic cleanup via t.Cleanup(). Domain test files use these builders
// instead of relying on mutable global state.
package e2e

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

var unsafeChars = regexp.MustCompile(`[^a-z0-9-]`)

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
// Individual tool fixture builders (use sess.individual session)
// ---------------------------------------------------------------------------

// createProject creates a private project via individual MCP tools and
// registers deletion in t.Cleanup.
func createProject(ctx context.Context, t *testing.T, session *mcp.ClientSession) ProjectFixture {
	t.Helper()
	name := uniqueName(e2eProjectPrefix + sanitizeTestName(t.Name()))
	out, err := callToolOn[projects.Output](ctx, session, "gitlab_project_create", projects.CreateInput{
		Name:                 name,
		Description:          "E2E: " + t.Name(),
		Visibility:           "private",
		InitializeWithReadme: true,
		DefaultBranch:        defaultBranch,
	})
	requireNoError(t, err, "create project fixture")

	t.Cleanup(func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = callToolVoidOn(delCtx, session, "gitlab_project_delete", projects.DeleteInput{
			ProjectID: toolutil.StringOrInt(strconv.FormatInt(out.ID, 10)),
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
	requireNoError(t, err, "create project fixture (meta)")

	t.Cleanup(func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = callToolVoidOn(delCtx, session, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": strconv.FormatInt(out.ID, 10),
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
	out, err := callToolOn[commits.Output](ctx, session, "gitlab_commit_create", commits.CreateInput{
		ProjectID:     proj.pidOf(),
		Branch:        branch,
		CommitMessage: message,
		Actions: []commits.Action{
			{Action: "create", FilePath: path, Content: content},
		},
	})
	requireNoError(t, err, "commit file "+path)
	return CommitFixture{SHA: out.ID, ShortID: out.ShortID}
}

// commitFileMeta creates a file via the gitlab_repository meta-tool.
func commitFileMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branch, path, content, message string) CommitFixture {
	t.Helper()
	out, err := callToolOn[commits.Output](ctx, session, "gitlab_repository", map[string]any{
		"action": "commit_create",
		"params": map[string]any{
			"project_id":     proj.pidStr(),
			"branch":         branch,
			"commit_message": message,
			"actions": []map[string]any{
				{"action": "create", "file_path": path, "content": content},
			},
		},
	})
	requireNoError(t, err, "commit file meta "+path)
	return CommitFixture{SHA: out.ID, ShortID: out.ShortID}
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
// given project or the context is canceled.
func waitForBranchOn(ctx context.Context, t *testing.T, _ *mcp.ClientSession, projectID int64, branch string) {
	t.Helper()
	pid := int(projectID)
	for range 15 {
		_, resp, err := sess.glClient.GL().Branches.GetBranch(pid, branch)
		if err == nil {
			t.Logf("Branch %q ready in project %d", branch, projectID)
			return
		}
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			select {
			case <-ctx.Done():
				t.Fatalf("context canceled waiting for branch %q: %v", branch, ctx.Err())
			case <-time.After(1 * time.Second):
			}
			continue
		}
		requireNoError(t, err, fmt.Sprintf("get branch %s in project %d", branch, projectID))
	}
	t.Fatalf("branch %q not available in project %d after 15s", branch, projectID)
}
