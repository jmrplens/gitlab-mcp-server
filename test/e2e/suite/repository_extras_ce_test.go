//go:build e2e && !enterprise

// repository_extras_ce_test.go covers repository-domain actions that the main
// repository and commit suites do not exercise: blob and raw blob retrieval,
// merge base resolution, commit revert, merge requests associated with a
// commit, changelog generation and committing, and submodule listing plus the
// submodule file read path.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/repositorysubmodules"
)

// repoExtrasBlobSHA resolves the blob SHA of a file at the repository root by
// listing the tree, since the blob tools require a blob SHA rather than a
// file path.
func repoExtrasBlobSHA(ctx context.Context, t *testing.T, proj ProjectFixture, fileName string) string {
	t.Helper()
	tree, err := callToolOn[repository.TreeOutput](ctx, sess.individual, "gitlab_repository_tree", repository.TreeInput{
		ProjectID: proj.pidOf(),
		Ref:       defaultBranch,
	})
	requireNoError(t, err, "list repository tree for blob SHA")
	for _, node := range tree.Tree {
		if node.Name == fileName && node.Type == "blob" {
			return node.ID
		}
	}
	t.Fatalf("blob %q not found in repository tree of project %d", fileName, proj.ID)
	return ""
}

// TestIndividual_RepositoryBlobs exercises blob retrieval by SHA using
// individual MCP tools: gitlab_repository_blob and gitlab_repository_raw_blob.
//
// The test commits a small text file, resolves its blob SHA from the
// repository tree, and fetches the blob through both endpoints. Each subtest
// asserts the SHA round-trips and the decoded text content matches what was
// committed.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_RepositoryBlobs(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	const fileName = "blob-probe.txt"
	const fileContent = "E2E blob probe content\n"
	commitFile(ctx, t, sess.individual, proj, defaultBranch, fileName, fileContent, "add blob probe file")

	blobSHA := repoExtrasBlobSHA(ctx, t, proj, fileName)

	t.Run("Blob", func(t *testing.T) {
		out, err := callToolOn[repository.BlobOutput](ctx, sess.individual, "gitlab_repository_blob", repository.BlobInput{
			ProjectID: proj.pidOf(),
			SHA:       blobSHA,
		})
		requireNoError(t, err, "get repository blob")
		requireTruef(t, out.SHA == blobSHA, "expected blob SHA %q, got %q", blobSHA, out.SHA)
		requireTruef(t, strings.Contains(out.Content, "blob probe"), "expected blob content to contain committed text, got %q", out.Content)
		t.Logf("Got blob %s (%d bytes, category=%s)", out.SHA, out.Size, out.ContentCategory)
	})

	t.Run("RawBlob", func(t *testing.T) {
		out, err := callToolOn[repository.RawBlobContentOutput](ctx, sess.individual, "gitlab_repository_raw_blob", repository.BlobInput{
			ProjectID: proj.pidOf(),
			SHA:       blobSHA,
		})
		requireNoError(t, err, "get raw blob content")
		requireTruef(t, out.SHA == blobSHA, "expected blob SHA %q, got %q", blobSHA, out.SHA)
		requireTruef(t, strings.Contains(out.Content, "blob probe"), "expected raw blob content to contain committed text, got %q", out.Content)
		t.Logf("Got raw blob %s (%d bytes)", out.SHA, out.Size)
	})
}

// TestIndividual_RepositoryMergeBase exercises gitlab_repository_merge_base
// via the individual tool surface.
//
// The test creates a feature branch off the default branch, commits to it so
// the branches diverge, and asserts the reported merge base equals the commit
// the branch was created from.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_RepositoryMergeBase(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	const branch = "feature/merge-base-e2e"
	branchFix := createBranch(ctx, t, sess.individual, proj, branch)
	commitFile(ctx, t, sess.individual, proj, branch, "merge-base.txt", "diverging content", "diverge feature branch")

	out, err := callToolOn[commits.Output](ctx, sess.individual, "gitlab_repository_merge_base", repository.MergeBaseInput{
		ProjectID: proj.pidOf(),
		Refs:      []string{defaultBranch, branch},
	})
	requireNoError(t, err, "get merge base")
	requireTruef(t, out.ID == branchFix.CommitID, "expected merge base %q (branch point), got %q", branchFix.CommitID, out.ID)
	t.Logf("Merge base of %s and %s is %s", defaultBranch, branch, out.ID)
}

// TestIndividual_CommitRevertMergeRequests exercises
// gitlab_commit_merge_requests and gitlab_commit_revert via the individual
// tool surface.
//
// The test creates a merge request from a diverged feature branch, waits for
// GitLab to associate the branch head commit with the MR, and asserts the
// association is reported. It then reverts the feature commit on its own
// branch and asserts a new revert commit is produced. The MR association
// check runs before the revert so the MR diff is still non-empty when GitLab
// computes it.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_CommitRevertMergeRequests(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	const branch = "feature/revert-e2e"
	createBranch(ctx, t, sess.individual, proj, branch)
	commit := commitFile(ctx, t, sess.individual, proj, branch, "revert-me.txt", "content to revert", "add file to revert")

	mr := createMR(ctx, t, sess.individual, proj, branch, defaultBranch, "MR for commit association test")
	waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)

	t.Run("CommitMergeRequests", func(t *testing.T) {
		// The commit-to-MR association is computed asynchronously after MR
		// creation, so poll until the MR shows up for the branch head commit.
		var out commits.MRsByCommitOutput
		pollErr := Poll(ctx, 2*time.Second, 60*time.Second, func() (bool, string, error) {
			listed, err := callToolOn[commits.MRsByCommitOutput](ctx, sess.individual, "gitlab_commit_merge_requests", commits.MRsByCommitInput{
				ProjectID: proj.pidOf(),
				SHA:       commit.SHA,
			})
			if err != nil {
				//nolint:nilerr // transient lookup failures are retried until the poll deadline
				return false, "list merge requests by commit: " + err.Error(), nil
			}
			out = listed
			if len(listed.MergeRequests) == 0 {
				return false, "commit not yet associated with any merge request", nil
			}
			return true, "commit associated with merge request", nil
		})
		requireNoError(t, pollErr, "wait for commit-MR association")
		found := false
		for _, listed := range out.MergeRequests {
			if listed.IID == mr.IID {
				found = true
				break
			}
		}
		requireTruef(t, found, "expected MR !%d among %d merge requests for commit %s", mr.IID, len(out.MergeRequests), commit.SHA)
		t.Logf("Commit %s associated with %d merge request(s)", commit.ShortID, len(out.MergeRequests))
	})

	t.Run("Revert", func(t *testing.T) {
		out, err := callToolOn[commits.Output](ctx, sess.individual, "gitlab_commit_revert", commits.RevertInput{
			ProjectID: proj.pidOf(),
			SHA:       commit.SHA,
			Branch:    branch,
		})
		requireNoError(t, err, "revert commit")
		requireTruef(t, out.ID != "", "expected revert commit SHA, got empty")
		requireTruef(t, out.ID != commit.SHA, "expected revert to create a new commit, got the original SHA %s", out.ID)
		t.Logf("Reverted %s with new commit %s", commit.ShortID, out.ShortID)
	})
}

// TestIndividual_RepositoryChangelog exercises
// gitlab_repository_changelog_generate and gitlab_repository_changelog_add
// via the individual tool surface.
//
// The test seeds two commits on the default branch — a range anchor and a
// commit carrying a "Changelog: added" Git trailer — then generates changelog
// notes for the (from, to] range without committing, and finally commits the
// changelog to CHANGELOG.md. An explicit from SHA is passed because the
// fixture project has no previous semver tag for GitLab to infer the range
// start from.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_RepositoryChangelog(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	seed := commitFile(ctx, t, sess.individual, proj, defaultBranch, "changelog-seed.txt", "range anchor", "chore: changelog range anchor")
	trailer := commitFile(ctx, t, sess.individual, proj, defaultBranch, "changelog-feature.txt", "feature content",
		"feat: add changelog feature\n\nChangelog: added")

	const version = "1.0.0"

	t.Run("Generate", func(t *testing.T) {
		out, err := callToolOn[repository.ChangelogDataOutput](ctx, sess.individual, "gitlab_repository_changelog_generate", repository.GenerateChangelogInput{
			ProjectID: proj.pidOf(),
			Version:   version,
			From:      seed.SHA,
			To:        trailer.SHA,
		})
		requireNoError(t, err, "generate changelog data")
		requireTruef(t, strings.Contains(out.Notes, version), "expected notes to mention version %q, got %q", version, out.Notes)
		requireTruef(t, strings.Contains(out.Notes, "add changelog feature"), "expected notes to include the trailer commit title, got %q", out.Notes)
		t.Logf("Generated changelog notes (%d bytes)", len(out.Notes))
	})

	t.Run("Add", func(t *testing.T) {
		out, err := callToolOn[repository.AddChangelogOutput](ctx, sess.individual, "gitlab_repository_changelog_add", repository.AddChangelogInput{
			ProjectID: proj.pidOf(),
			Version:   version,
			From:      seed.SHA,
			To:        trailer.SHA,
		})
		requireNoError(t, err, "add changelog")
		requireTruef(t, out.Success, "expected changelog add to report success")
		requireTruef(t, out.Version == version, "expected version %q, got %q", version, out.Version)
		t.Log("Committed CHANGELOG.md via changelog API")
	})
}

// TestIndividual_RepositorySubmodules exercises
// gitlab_list_repository_submodules and gitlab_read_repository_submodule_file
// via the individual tool surface.
//
// LIMITATION: the GitLab REST surface offers no way to create a true gitlink
// (tree entry mode 160000) — the files and commits APIs only create blobs,
// and the submodule update API can only move an existing gitlink. The test
// therefore commits a .gitmodules file (which the list tool parses) and
// asserts the listing, then exercises the read tool against the configured
// submodule and asserts its deterministic failure when no gitlink tree entry
// exists. A positive read requires a repository with a real submodule, which
// cannot be provisioned through the available tools.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_RepositorySubmodules(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	const gitmodulesContent = `[submodule "libs/dep"]
	path = libs/dep
	url = https://gitlab.example.com/e2e/dep.git
`
	commitFile(ctx, t, sess.individual, proj, defaultBranch, ".gitmodules", gitmodulesContent, "add .gitmodules for submodule tests")

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[repositorysubmodules.ListOutput](ctx, sess.individual, "gitlab_list_repository_submodules", repositorysubmodules.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list repository submodules")
		requireTruef(t, out.Count == 1, "expected 1 submodule entry, got %d", out.Count)
		requireTruef(t, out.Submodules[0].Path == "libs/dep", "expected submodule path libs/dep, got %q", out.Submodules[0].Path)
		// No gitlink exists, so the commit SHA is legitimately empty here.
		t.Logf("Listed submodule %q (url=%s, commit_sha=%q)", out.Submodules[0].Name, out.Submodules[0].URL, out.Submodules[0].CommitSHA)
	})

	t.Run("ReadFileWithoutGitlink", func(t *testing.T) {
		_, err := callToolOn[repositorysubmodules.ReadOutput](ctx, sess.individual, "gitlab_read_repository_submodule_file", repositorysubmodules.ReadInput{
			ProjectID:     proj.pidOf(),
			SubmodulePath: "libs/dep",
			FilePath:      "README.md",
		})
		requireTruef(t, err != nil, "expected submodule file read to fail without a gitlink tree entry")
		t.Logf("Read correctly failed without a gitlink: %v", err)
	})
}
