//go:build e2e

// workflow_test.go contains end-to-end workflow tests that exercise the
// complete GitLab project lifecycle using individual MCP tools. Tests run
// sequentially through project creation, commits, branches, tags, releases,
// merge requests, notes, discussions, and cleanup.
package e2e

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Branch name and tag version used across sequential E2E workflow steps.
const (
	testE2EBranch     = "feature/e2e-changes"
	testTagV010       = "v0.1.0"
	msgIssueIIDNotSet = "issue IID not set"
)

// TestFullWorkflow runs a sequential E2E test that exercises the complete
// project lifecycle using individual MCP tools:
//
//   - User identity
//   - Project CRUD and configuration
//   - Commits, file operations, and commit inspection
//   - Branch management (create, get, list, protect, list protected, unprotect, delete)
//   - Repository tree listing and compare
//   - Tags and releases with asset links
//   - Issue lifecycle and notes
//   - Labels and milestones
//   - Project members
//   - Merge request lifecycle (create, review, approve, rebase, merge)
//   - MR commits and pipelines
//   - Notes and threaded discussions on MRs
//   - Search (code and merge requests)
//   - Project upload
func TestFullWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// User identity.
	t.Run("00_UserCurrent", func(t *testing.T) { testUserCurrent(ctx, t) })

	// Project setup.
	t.Run("01_CreateProject", func(t *testing.T) { testCreateProject(ctx, t) })
	t.Run("02_GetProject", func(t *testing.T) { testGetProject(ctx, t) })
	t.Run("03_UnprotectMain", func(t *testing.T) { testUnprotectMain(ctx, t) })

	// Repository: commits & files via MCP tools.
	t.Run("04_CommitCreateMainFile", func(t *testing.T) { testCommitCreateMainFile(ctx, t) })
	t.Run("05_FileGet", func(t *testing.T) { testFileGet(ctx, t) })

	// Commit inspection.
	t.Run("05a_CommitList", func(t *testing.T) { testCommitList(ctx, t) })
	t.Run("05b_CommitGet", func(t *testing.T) { testCommitGet(ctx, t) })
	t.Run("05c_CommitDiff", func(t *testing.T) { testCommitDiff(ctx, t) })

	// Repository tree & compare.
	t.Run("05d_RepositoryTree", func(t *testing.T) { testRepositoryTree(ctx, t) })

	// Branch management via MCP tools.
	t.Run("06_BranchCreate", func(t *testing.T) { testBranchCreate(ctx, t) })
	t.Run("06a_BranchGet", func(t *testing.T) { testBranchGet(ctx, t) })
	t.Run("07_BranchList", func(t *testing.T) { testBranchList(ctx, t) })
	t.Run("08_BranchProtect", func(t *testing.T) { testBranchProtect(ctx, t) })
	t.Run("09_ListProtectedBranches", func(t *testing.T) { testListProtectedBranches(ctx, t) })
	t.Run("10_BranchUnprotect", func(t *testing.T) { testBranchUnprotectFeature(ctx, t) })

	// Commit changes on feature branch for MR.
	t.Run("11_CommitFeatureChanges", func(t *testing.T) { testCommitFeatureChanges(ctx, t) })

	// Repository compare (main vs feature).
	t.Run("11a_RepositoryCompare", func(t *testing.T) { testRepositoryCompare(ctx, t) })

	// Tags & releases.
	t.Run("12_TagCreate", func(t *testing.T) { testTagCreate(ctx, t) })
	t.Run("12a_TagGet", func(t *testing.T) { testTagGet(ctx, t) })
	t.Run("13_TagList", func(t *testing.T) { testTagList(ctx, t) })
	t.Run("14_ReleaseCreate", func(t *testing.T) { testReleaseCreate(ctx, t) })
	t.Run("15_ReleaseGet", func(t *testing.T) { testReleaseGet(ctx, t) })
	t.Run("16_ReleaseUpdate", func(t *testing.T) { testReleaseUpdate(ctx, t) })
	t.Run("17_ReleaseList", func(t *testing.T) { testReleaseList(ctx, t) })
	t.Run("18_ReleaseLinkCreate", func(t *testing.T) { testReleaseLinkCreate(ctx, t) })
	t.Run("19_ReleaseLinkList", func(t *testing.T) { testReleaseLinkList(ctx, t) })
	t.Run("20_ReleaseLinkDelete", func(t *testing.T) { testReleaseLinkDelete(ctx, t) })
	t.Run("21_ReleaseDelete", func(t *testing.T) { testReleaseDelete(ctx, t) })
	t.Run("22_TagDelete", func(t *testing.T) { testTagDelete(ctx, t) })

	// Issue lifecycle.
	t.Run("22a_IssueCreate", func(t *testing.T) { testIssueCreate(ctx, t) })
	t.Run("22b_IssueGet", func(t *testing.T) { testIssueGet(ctx, t) })
	t.Run("22c_IssueList", func(t *testing.T) { testIssueList(ctx, t) })
	t.Run("22d_IssueUpdate", func(t *testing.T) { testIssueUpdate(ctx, t) })
	t.Run("22e_IssueNoteCreate", func(t *testing.T) { testIssueNoteCreate(ctx, t) })
	t.Run("22f_IssueNoteList", func(t *testing.T) { testIssueNoteList(ctx, t) })
	t.Run("22g_IssueDelete", func(t *testing.T) { testIssueDelete(ctx, t) })

	// Labels & milestones.
	t.Run("22h_LabelList", func(t *testing.T) { testLabelList(ctx, t) })
	t.Run("22i_MilestoneList", func(t *testing.T) { testMilestoneList(ctx, t) })

	// Project members.
	t.Run("22j_ProjectMembersList", func(t *testing.T) { testProjectMembersList(ctx, t) })

	// Project upload.
	t.Run("22k_ProjectUpload", func(t *testing.T) { testProjectUpload(ctx, t) })

	// Merge request lifecycle.
	t.Run("23_CreateMR", func(t *testing.T) { testCreateMR(ctx, t) })
	t.Run("24_GetMR", func(t *testing.T) { testGetMR(ctx, t) })
	t.Run("25_ListMRs", func(t *testing.T) { testListMRs(ctx, t) })
	t.Run("26_UpdateMR", func(t *testing.T) { testUpdateMR(ctx, t) })

	// MR commits & pipelines.
	t.Run("26a_MRCommits", func(t *testing.T) { testMRCommits(ctx, t) })
	t.Run("26b_MRPipelines", func(t *testing.T) { testMRPipelines(ctx, t) })

	// Notes (general comments).
	t.Run("27_AddNote", func(t *testing.T) { testAddNote(ctx, t) })
	t.Run("28_ListNotes", func(t *testing.T) { testListNotes(ctx, t) })
	t.Run("29_UpdateNote", func(t *testing.T) { testUpdateNote(ctx, t) })
	t.Run("30_DeleteNote", func(t *testing.T) { testDeleteNote(ctx, t) })

	// MR diffs & threaded discussions.
	t.Run("31_GetMRChanges", func(t *testing.T) { testGetMRChanges(ctx, t) })
	t.Run("32_CreateInlineDiscussion", func(t *testing.T) { testCreateInlineDiscussion(ctx, t) })
	t.Run("33_ReplyToDiscussion", func(t *testing.T) { testReplyToDiscussion(ctx, t) })
	t.Run("34_ResolveDiscussion", func(t *testing.T) { testResolveDiscussion(ctx, t) })
	t.Run("35_ListDiscussions", func(t *testing.T) { testListDiscussions(ctx, t) })

	// MR rebase (before merge, while MR is open).
	t.Run("35a_RebaseMR", func(t *testing.T) { testRebaseMR(ctx, t) })

	// Approve, merge, project update/list.
	t.Run("36_ApproveMR", func(t *testing.T) { testApproveMR(ctx, t) })
	t.Run("37_UnapproveMR", func(t *testing.T) { testUnapproveMR(ctx, t) })
	t.Run("38_MergeMR", func(t *testing.T) { testMergeMR(ctx, t) })

	// Search (after merge so content is on default branch).
	t.Run("38a_SearchCode", func(t *testing.T) { testSearchCode(ctx, t) })
	t.Run("38b_SearchMergeRequests", func(t *testing.T) { testSearchMergeRequests(ctx, t) })

	// Group tools (read-only, use whatever groups are accessible).
	t.Run("38c_GroupList", func(t *testing.T) { testGroupList(ctx, t) })
	t.Run("38d_GroupGet", func(t *testing.T) { testGroupGet(ctx, t) })
	t.Run("38e_GroupMembersList", func(t *testing.T) { testGroupMembersList(ctx, t) })
	t.Run("38f_SubgroupsList", func(t *testing.T) { testSubgroupsList(ctx, t) })

	// Pipeline list (read-only, may return empty without CI config).
	t.Run("38g_PipelineList", func(t *testing.T) { testPipelineList(ctx, t) })

	// Package lifecycle.
	t.Run("38h_PackagePublish", func(t *testing.T) { testPackagePublish(ctx, t) })
	t.Run("38i_PackageList", func(t *testing.T) { testPackageList(ctx, t) })
	t.Run("38j_PackageFileList", func(t *testing.T) { testPackageFileList(ctx, t) })
	t.Run("38k_PackageDownload", func(t *testing.T) { testPackageDownload(ctx, t) })
	t.Run("38l_PackageFileDelete", func(t *testing.T) { testPackageFileDelete(ctx, t) })
	t.Run("38m_PackageDelete", func(t *testing.T) { testPackageDelete(ctx, t) })

	// Upload with file_path.
	t.Run("38n_UploadFilePath", func(t *testing.T) { testUploadFilePath(ctx, t) })

	t.Run("39_UpdateProject", func(t *testing.T) { testUpdateProject(ctx, t) })
	t.Run("40_ListProjects", func(t *testing.T) { testListProjects(ctx, t) })

	// Push rules.
	t.Run("41_AddPushRule", func(t *testing.T) { testAddPushRule(ctx, t) })
	t.Run("42_GetPushRules", func(t *testing.T) { testGetPushRules(ctx, t) })
	t.Run("43_EditPushRule", func(t *testing.T) { testEditPushRule(ctx, t) })
	t.Run("44_DeletePushRule", func(t *testing.T) { testDeletePushRule(ctx, t) })

	// User-scoped project listings.
	t.Run("45_ListUserContributed", func(t *testing.T) { testListUserContributed(ctx, t) })
	t.Run("46_ListUserStarred", func(t *testing.T) { testListUserStarred(ctx, t) })

	// GraphQL tools (branch rules, CI catalog, vulnerabilities, custom emoji).
	t.Run("47_ListBranchRules", func(t *testing.T) { testListBranchRules(ctx, t) })
	t.Run("48_ListCatalogResources", func(t *testing.T) { testListCatalogResources(ctx, t) })
	t.Run("49_VulnerabilitySeverityCount", func(t *testing.T) { testVulnerabilitySeverityCount(ctx, t) })
	t.Run("50_ListVulnerabilities", func(t *testing.T) { testListVulnerabilities(ctx, t) })
	t.Run("51_ListCustomEmoji", func(t *testing.T) { testListCustomEmoji(ctx, t) })

	// Cleanup.
	t.Run("99_Cleanup_DeleteProject", func(t *testing.T) { testDeleteProject(ctx, t) })
}

// Step 1: Create Project.

// defaultBranch is the default branch name used by E2E test projects.
const defaultBranch = "main"

// testCreateProject creates a new private GitLab project with a README
// and stores its ID and path in the global test state.
func testCreateProject(ctx context.Context, t *testing.T) {
	name := uniqueName("e2e-test")
	out, err := callTool[projects.Output](ctx, "gitlab_project_create", projects.CreateInput{
		Name:                 name,
		Description:          "E2E test project — will be deleted automatically",
		Visibility:           "private",
		InitializeWithReadme: true,
		DefaultBranch:        defaultBranch,
	})
	requireNoError(t, err, "create project")
	requireTrue(t, out.ID > 0, "project ID should be positive, got %d", out.ID)
	requireTrue(t, out.Name == name, "expected name %q, got %q", name, out.Name)

	state.projectID = out.ID
	state.projectPath = out.PathWithNamespace
	t.Logf("Created project: %s (ID=%d, default_branch=%s)", state.projectPath, state.projectID, out.DefaultBranch)

	waitForBranch(ctx, t, defaultBranch)
}

// Step 2: Get Project by ID.

// testGetProject retrieves the E2E test project by ID and verifies its
// path matches the expected value.
func testGetProject(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[projects.Output](ctx, "gitlab_project_get", projects.GetInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "get project")
	requireTrue(t, out.ID == state.projectID, "expected project ID %d, got %d", state.projectID, out.ID)
	requireTrue(t, out.PathWithNamespace == state.projectPath, "expected path %q, got %q", state.projectPath, out.PathWithNamespace)
	t.Logf("Got project %s (visibility=%s)", out.PathWithNamespace, out.Visibility)
}

// Step 3: Unprotect main branch so we can push files.

// testUnprotectMain removes protection from the main branch so subsequent
// commits can be pushed directly. It first waits for GitLab to apply the
// default branch protection (async job after project creation).
func testUnprotectMain(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	waitForBranchProtection(ctx, t, int(state.projectID), defaultBranch)
	err := callToolVoid(ctx, "gitlab_branch_unprotect", branches.UnprotectInput{
		ProjectID:  pidStr(),
		BranchName: defaultBranch,
	})
	requireNoError(t, err, "unprotect main branch")
	t.Logf("Unprotected %s branch", defaultBranch)
}

// Step 4: Create a file on main via MCP commit tool.

// testCommitCreateMainFile creates main.go on the default branch via the
// gitlab_commit_create MCP tool.
func testCommitCreateMainFile(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callTool[commits.Output](ctx, "gitlab_commit_create", commits.CreateInput{
		ProjectID:     pidStr(),
		Branch:        defaultBranch,
		CommitMessage: "feat: add main.go for E2E testing",
		Actions: []commits.Action{
			{
				Action:   "create",
				FilePath: testFileMainGo,
				Content: `package main

import "fmt"

func main() {
	fmt.Println("Hello, E2E!")
}

func add(a, b int) int {
	return a + b
}
`,
			},
		},
	})
	requireNoError(t, err, "commit create main.go")
	requireTrue(t, out.ID != "", msgCommitIDEmpty)

	state.lastCommitSHA = out.ID
	t.Logf("Committed main.go to %s (SHA=%s)", defaultBranch, out.ShortID)
}

// Step 5: Get file content via MCP tool.

// testFileGet retrieves main.go content via the gitlab_file_get MCP tool
// and verifies the file name and size.
func testFileGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[files.Output](ctx, "gitlab_file_get", files.GetInput{
		ProjectID: pidStr(),
		FilePath:  testFileMainGo,
		Ref:       defaultBranch,
	})
	requireNoError(t, err, "get file main.go")
	requireTrue(t, out.FileName == testFileMainGo, "expected file name main.go, got %q", out.FileName)
	requireTrue(t, out.Size > 0, "file size should be positive")
	t.Logf("Got file %s (%d bytes, ref=%s)", out.FileName, out.Size, out.Ref)
}

// Step 6: Create feature branch via MCP tool.

// testBranchCreate creates the feature/e2e-changes branch from the default
// branch via the gitlab_branch_create MCP tool.
func testBranchCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[branches.Output](ctx, "gitlab_branch_create", branches.CreateInput{
		ProjectID:  pidStr(),
		BranchName: testE2EBranch,
		Ref:        defaultBranch,
	})
	requireNoError(t, err, "create feature branch")
	requireTrue(t, out.Name == testE2EBranch, "expected branch name 'feature/e2e-changes', got %q", out.Name)
	t.Logf("Created branch %s (commit=%s)", out.Name, out.CommitID)
}

// Step 7: List branches.

// testBranchList lists all branches and verifies at least two exist
// (main and the feature branch).
func testBranchList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[branches.ListOutput](ctx, "gitlab_branch_list", branches.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list branches")
	requireTrue(t, len(out.Branches) >= 2, "expected at least 2 branches (main + feature), got %d", len(out.Branches))

	names := make([]string, len(out.Branches))
	for i, b := range out.Branches {
		names[i] = b.Name
	}
	t.Logf("Listed %d branches: %v", len(out.Branches), names)
}

// Step 8: Protect feature branch.

// testBranchProtect protects the feature branch with Maintainer push and
// Developer merge access levels.
func testBranchProtect(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[branches.ProtectedOutput](ctx, "gitlab_branch_protect", branches.ProtectInput{
		ProjectID:        pidStr(),
		BranchName:       testE2EBranch,
		PushAccessLevel:  40, // Maintainer
		MergeAccessLevel: 30, // Developer
	})
	requireNoError(t, err, "protect feature branch")
	requireTrue(t, out.Name == testE2EBranch, "expected protected branch name, got %q", out.Name)
	t.Logf("Protected branch %s (push=%d, merge=%d)", out.Name, out.PushAccessLevel, out.MergeAccessLevel)
}

// Step 9: List protected branches.

// testListProtectedBranches lists protected branches and verifies the
// feature branch appears in the result.
func testListProtectedBranches(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[branches.ProtectedListOutput](ctx, "gitlab_protected_branches_list", branches.ProtectedListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list protected branches")
	requireTrue(t, len(out.Branches) >= 1, "expected at least 1 protected branch, got %d", len(out.Branches))

	found := false
	for _, b := range out.Branches {
		if b.Name == testE2EBranch {
			found = true
			break
		}
	}
	requireTrue(t, found, "feature/e2e-changes not in protected branches list")
	t.Logf("Listed %d protected branches", len(out.Branches))
}

// Step 10: Unprotect feature branch (so we can push commits to it).

// testBranchUnprotectFeature removes protection from the feature branch so
// commits can be pushed to it.
func testBranchUnprotectFeature(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	err := callToolVoid(ctx, "gitlab_branch_unprotect", branches.UnprotectInput{
		ProjectID:  pidStr(),
		BranchName: testE2EBranch,
	})
	requireNoError(t, err, "unprotect feature branch")
	t.Log("Unprotected feature/e2e-changes branch")
}

// Step 11: Commit changes on the feature branch via MCP tool.

// testCommitFeatureChanges pushes an updated main.go with a multiply
// function to the feature branch.
func testCommitFeatureChanges(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[commits.Output](ctx, "gitlab_commit_create", commits.CreateInput{
		ProjectID:     pidStr(),
		Branch:        testE2EBranch,
		CommitMessage: "refactor: improve add function with multiply",
		Actions: []commits.Action{
			{
				Action:   "update",
				FilePath: testFileMainGo,
				Content: `package main

import "fmt"

func main() {
	fmt.Println("Hello, E2E Test!")
	result := multiply(3, 4)
	fmt.Println("3 * 4 =", result)
}

func add(a, b int) int {
	return a + b
}

func multiply(a, b int) int {
	return a * b
}
`,
			},
		},
	})
	requireNoError(t, err, "commit feature changes")
	requireTrue(t, out.ID != "", msgCommitIDEmpty)
	t.Logf("Committed feature changes (SHA=%s)", out.ShortID)
}

// Step 12: Create tag on main.

// testTagCreate creates tag v0.1.0 on the default branch via the
// gitlab_tag_create MCP tool.
func testTagCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[tags.Output](ctx, "gitlab_tag_create", tags.CreateInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
		Ref:       defaultBranch,
		Message:   "E2E test tag v0.1.0",
	})
	requireNoError(t, err, "create tag")
	requireTrue(t, out.Name == testTagV010, "expected tag name v0.1.0, got %q", out.Name)
	t.Logf("Created tag %s (target=%s)", out.Name, out.Target)
}

// Step 13: List tags.

// testTagList lists tags and verifies v0.1.0 appears in the result.
func testTagList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[tags.ListOutput](ctx, "gitlab_tag_list", tags.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list tags")
	requireTrue(t, len(out.Tags) >= 1, "expected at least 1 tag, got %d", len(out.Tags))

	found := false
	for _, tag := range out.Tags {
		if tag.Name == testTagV010 {
			found = true
			break
		}
	}
	requireTrue(t, found, "tag v0.1.0 not found in list")
	t.Logf("Listed %d tags", len(out.Tags))
}

// Step 14: Create release for the tag.

// testReleaseCreate creates a release for tag v0.1.0 via the
// gitlab_release_create MCP tool.
func testReleaseCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[releases.Output](ctx, "gitlab_release_create", releases.CreateInput{
		ProjectID:   pidStr(),
		TagName:     testTagV010,
		Name:        "E2E Test Release v0.1.0",
		Description: "Automated E2E test release with full lifecycle coverage.",
	})
	requireNoError(t, err, "create release")
	requireTrue(t, out.TagName == testTagV010, "expected release tag v0.1.0, got %q", out.TagName)
	t.Logf("Created release %s (%s)", out.Name, out.TagName)
}

// Step 15: Get release.

// testReleaseGet retrieves the release for v0.1.0 and verifies its name
// and tag.
func testReleaseGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[releases.Output](ctx, "gitlab_release_get", releases.GetInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
	})
	requireNoError(t, err, "get release")
	requireTrue(t, out.TagName == testTagV010, "expected release tag v0.1.0, got %q", out.TagName)
	requireTrue(t, out.Name == "E2E Test Release v0.1.0", "expected release name, got %q", out.Name)
	t.Logf("Got release %s (created=%s)", out.Name, out.CreatedAt)
}

// Step 16: Update release.

// testReleaseUpdate updates the release description via the
// gitlab_release_update MCP tool.
func testReleaseUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[releases.Output](ctx, "gitlab_release_update", releases.UpdateInput{
		ProjectID:   pidStr(),
		TagName:     testTagV010,
		Description: "Updated E2E test release — now with asset links.",
	})
	requireNoError(t, err, "update release")
	requireTrue(t, out.TagName == testTagV010, "expected tag v0.1.0, got %q", out.TagName)
	t.Logf("Updated release %s", out.Name)
}

// Step 17: List releases.

// testReleaseList lists releases and verifies at least one exists.
func testReleaseList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[releases.ListOutput](ctx, "gitlab_release_list", releases.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list releases")
	requireTrue(t, len(out.Releases) >= 1, "expected at least 1 release, got %d", len(out.Releases))
	t.Logf("Listed %d releases", len(out.Releases))
}

// Step 18: Create release asset link.

// testReleaseLinkCreate adds a package asset link to the v0.1.0 release
// and stores the link ID in the global test state.
func testReleaseLinkCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[releaselinks.Output](ctx, "gitlab_release_link_create", releaselinks.CreateInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
		Name:      "E2E Binary (Linux amd64)",
		URL:       "https://example.com/releases/v0.1.0/binary-linux-amd64",
		LinkType:  "package",
	})
	requireNoError(t, err, "create release link")
	requireTrue(t, out.ID > 0, "release link ID should be positive")
	requireTrue(t, out.Name == "E2E Binary (Linux amd64)", "expected link name, got %q", out.Name)

	state.releaseLinkID = out.ID
	t.Logf("Created release link ID=%d (%s)", out.ID, out.Name)
}

// Step 19: List release links.

// testReleaseLinkList lists release links for v0.1.0 and verifies the
// created link appears in the result.
func testReleaseLinkList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[releaselinks.ListOutput](ctx, "gitlab_release_link_list", releaselinks.ListInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
	})
	requireNoError(t, err, "list release links")
	requireTrue(t, len(out.Links) >= 1, "expected at least 1 release link, got %d", len(out.Links))

	found := false
	for _, l := range out.Links {
		if l.ID == state.releaseLinkID {
			found = true
			break
		}
	}
	requireTrue(t, found, "release link ID=%d not found in list", state.releaseLinkID)
	t.Logf("Listed %d release links", len(out.Links))
}

// Step 20: Delete release link.

// testReleaseLinkDelete deletes the release asset link created earlier.
func testReleaseLinkDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.releaseLinkID > 0, "release link ID not set")

	out, err := callTool[releaselinks.Output](ctx, "gitlab_release_link_delete", releaselinks.DeleteInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
		LinkID:    state.releaseLinkID,
	})
	requireNoError(t, err, "delete release link")
	requireTrue(t, out.ID == state.releaseLinkID, "expected link ID %d, got %d", state.releaseLinkID, out.ID)
	t.Logf("Deleted release link ID=%d", out.ID)
	state.releaseLinkID = 0
}

// Step 21: Delete release.

// testReleaseDelete deletes the v0.1.0 release via the
// gitlab_release_delete MCP tool.
func testReleaseDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[releases.Output](ctx, "gitlab_release_delete", releases.DeleteInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
	})
	requireNoError(t, err, "delete release")
	requireTrue(t, out.TagName == testTagV010, "expected deleted release tag v0.1.0, got %q", out.TagName)
	t.Logf("Deleted release %s", out.TagName)
}

// Step 22: Delete tag.

// testTagDelete deletes tag v0.1.0 via the gitlab_tag_delete MCP tool.
func testTagDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	err := callToolVoid(ctx, "gitlab_tag_delete", tags.DeleteInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
	})
	requireNoError(t, err, "delete tag")
	t.Log("Deleted tag v0.1.0")
}

// User identity.

// testUserCurrent fetches the authenticated GitLab user and verifies basic
// fields are populated.
func testUserCurrent(ctx context.Context, t *testing.T) {
	out, err := callTool[users.Output](ctx, "gitlab_user_current", users.CurrentInput{})
	requireNoError(t, err, "get current user")
	requireTrue(t, out.ID > 0, "user ID should be positive, got %d", out.ID)
	requireTrue(t, out.Username != "", "username should not be empty")
	requireTrue(t, out.State == "active", "user state should be 'active', got %q", out.State)
	t.Logf("Current user: %s (ID=%d, email=%s)", out.Username, out.ID, out.Email)
}

// Commit inspection.

// testCommitList lists commits on the default branch and verifies at least
// one commit exists (the one we created earlier).
func testCommitList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[commits.ListOutput](ctx, "gitlab_commit_list", commits.ListInput{
		ProjectID: pidStr(),
		RefName:   defaultBranch,
	})
	requireNoError(t, err, "list commits")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit, got %d", len(out.Commits))

	// Store the most recent commit SHA for subsequent get/diff tests.
	state.lastCommitSHA = out.Commits[0].ID
	t.Logf("Listed %d commits on %s (latest=%s)", len(out.Commits), defaultBranch, out.Commits[0].ShortID)
}

// testCommitGet retrieves the latest commit by SHA and verifies its fields.
func testCommitGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.lastCommitSHA != "", "lastCommitSHA not set — CommitList must run first")

	out, err := callTool[commits.DetailOutput](ctx, "gitlab_commit_get", commits.GetInput{
		ProjectID: pidStr(),
		SHA:       state.lastCommitSHA,
	})
	requireNoError(t, err, "get commit")
	requireTrue(t, out.ID == state.lastCommitSHA, "expected SHA %s, got %s", state.lastCommitSHA, out.ID)
	requireTrue(t, out.Title != "", "commit title should not be empty")
	t.Logf("Got commit %s: %s (author=%s)", out.ShortID, out.Title, out.AuthorName)
}

// testCommitDiff retrieves the diff for the latest commit and verifies
// at least one file was changed.
func testCommitDiff(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.lastCommitSHA != "", "lastCommitSHA not set — CommitList must run first")

	out, err := callTool[commits.DiffOutput](ctx, "gitlab_commit_diff", commits.DiffInput{
		ProjectID: pidStr(),
		SHA:       state.lastCommitSHA,
	})
	requireNoError(t, err, "get commit diff")
	requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff, got %d", len(out.Diffs))
	t.Logf("Commit %s has %d file diffs", state.lastCommitSHA[:8], len(out.Diffs))
}

// Repository tree.

// testRepositoryTree lists the repository root tree and verifies main.go
// appears in the listing.
func testRepositoryTree(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[repository.TreeOutput](ctx, "gitlab_repository_tree", repository.TreeInput{
		ProjectID: pidStr(),
		Ref:       defaultBranch,
	})
	requireNoError(t, err, "list repository tree")
	requireTrue(t, len(out.Tree) >= 1, "expected at least 1 tree node, got %d", len(out.Tree))

	found := false
	for _, n := range out.Tree {
		if n.Name == testFileMainGo {
			found = true
			break
		}
	}
	requireTrue(t, found, "main.go not found in repository tree")
	t.Logf("Repository tree has %d entries", len(out.Tree))
}

// Branch get.

// testBranchGet retrieves the feature branch by name and verifies its fields.
func testBranchGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[branches.Output](ctx, "gitlab_branch_get", branches.GetInput{
		ProjectID:  pidStr(),
		BranchName: testE2EBranch,
	})
	requireNoError(t, err, "get branch")
	requireTrue(t, out.Name == testE2EBranch, "expected branch %q, got %q", testE2EBranch, out.Name)
	requireTrue(t, out.CommitID != "", msgCommitIDEmpty)
	t.Logf("Got branch %s (commit=%s)", out.Name, out.CommitID)
}

// Repository compare.

// testRepositoryCompare compares main vs feature branch and verifies
// differences are returned.
func testRepositoryCompare(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[repository.CompareOutput](ctx, "gitlab_repository_compare", repository.CompareInput{
		ProjectID: pidStr(),
		From:      defaultBranch,
		To:        testE2EBranch,
	})
	requireNoError(t, err, "compare repository")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit in diff, got %d", len(out.Commits))
	requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 file diff, got %d", len(out.Diffs))
	t.Logf("Compare %s..%s: %d commits, %d file diffs", defaultBranch, testE2EBranch, len(out.Commits), len(out.Diffs))
}

// Tag get.

// testTagGet retrieves tag v0.1.0 by name and verifies its fields.
func testTagGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[tags.Output](ctx, "gitlab_tag_get", tags.GetInput{
		ProjectID: pidStr(),
		TagName:   testTagV010,
	})
	requireNoError(t, err, "get tag")
	requireTrue(t, out.Name == testTagV010, "expected tag %q, got %q", testTagV010, out.Name)
	requireTrue(t, out.Target != "", "tag target should not be empty")
	t.Logf("Got tag %s (target=%s)", out.Name, out.Target)
}

// Issue lifecycle.

// testIssueCreate creates an issue in the test project and stores its IID.
func testIssueCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[issues.Output](ctx, "gitlab_issue_create", issues.CreateInput{
		ProjectID:   pidStr(),
		Title:       "E2E test issue — automated lifecycle",
		Description: "This issue is created by the E2E test suite and will be deleted automatically.",
	})
	requireNoError(t, err, "create issue")
	requireTrue(t, out.IID > 0, "issue IID should be positive, got %d", out.IID)
	requireTrue(t, out.State == "opened", "issue state should be 'opened', got %q", out.State)

	state.issueIID = out.IID
	t.Logf("Created issue #%d: %s", out.IID, out.Title)
}

// testIssueGet retrieves the issue by IID and verifies its fields.
func testIssueGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, "issue IID not set — IssueCreate must run first")

	out, err := callTool[issues.Output](ctx, "gitlab_issue_get", issues.GetInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
	})
	requireNoError(t, err, "get issue")
	requireTrue(t, out.IID == state.issueIID, "expected issue IID %d, got %d", state.issueIID, out.IID)
	requireTrue(t, out.State == "opened", "expected state 'opened', got %q", out.State)
	t.Logf("Got issue #%d: %s (author=%s)", out.IID, out.Title, out.Author)
}

// testIssueList lists issues and verifies the test issue appears.
func testIssueList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[issues.ListOutput](ctx, "gitlab_issue_list", issues.ListInput{
		ProjectID: pidStr(),
		State:     "opened",
	})
	requireNoError(t, err, "list issues")
	requireTrue(t, len(out.Issues) >= 1, "expected at least 1 issue, got %d", len(out.Issues))

	found := false
	for _, i := range out.Issues {
		if i.IID == state.issueIID {
			found = true
			break
		}
	}
	requireTrue(t, found, "issue #%d not found in list", state.issueIID)
	t.Logf("Listed %d open issues", len(out.Issues))
}

// testIssueUpdate modifies the issue title and description.
func testIssueUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)

	out, err := callTool[issues.Output](ctx, "gitlab_issue_update", issues.UpdateInput{
		ProjectID:   pidStr(),
		IssueIID:    state.issueIID,
		Title:       "E2E test issue — updated title",
		Description: "Updated description via gitlab_issue_update E2E test.",
	})
	requireNoError(t, err, "update issue")
	requireTrue(t, out.IID == state.issueIID, "expected issue IID %d, got %d", state.issueIID, out.IID)
	requireTrue(t, out.Title == "E2E test issue — updated title", "expected updated title, got %q", out.Title)
	t.Logf("Updated issue #%d", out.IID)
}

// testIssueNoteCreate adds a comment to the test issue and stores its ID.
func testIssueNoteCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)

	out, err := callTool[issuenotes.Output](ctx, "gitlab_issue_note_create", issuenotes.CreateInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
		Body:      "**E2E Bot**: Automated comment on issue for testing.",
	})
	requireNoError(t, err, "create issue note")
	requireTrue(t, out.ID > 0, "issue note ID should be positive, got %d", out.ID)

	state.issueNoteID = out.ID
	t.Logf("Created issue note ID=%d on issue #%d", out.ID, state.issueIID)
}

// testIssueNoteList lists notes on the issue and verifies our note appears.
func testIssueNoteList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)

	out, err := callTool[issuenotes.ListOutput](ctx, "gitlab_issue_note_list", issuenotes.ListInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
	})
	requireNoError(t, err, "list issue notes")
	requireTrue(t, len(out.Notes) >= 1, "expected at least 1 note, got %d", len(out.Notes))

	found := false
	for _, n := range out.Notes {
		if n.ID == state.issueNoteID {
			found = true
			break
		}
	}
	requireTrue(t, found, "issue note ID=%d not found in list", state.issueNoteID)
	t.Logf("Listed %d notes on issue #%d", len(out.Notes), state.issueIID)
}

// testIssueDelete deletes the test issue.
func testIssueDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)

	err := callToolVoid(ctx, "gitlab_issue_delete", issues.DeleteInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
	})
	requireNoError(t, err, "delete issue")
	t.Logf("Deleted issue #%d", state.issueIID)
	state.issueIID = 0
}

// Labels.

// testLabelList lists project labels (may be empty for a new project).
func testLabelList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[labels.ListOutput](ctx, "gitlab_label_list", labels.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list labels")
	t.Logf("Listed %d labels", len(out.Labels))
}

// Milestones.

// testMilestoneList lists project milestones (may be empty for a new project).
func testMilestoneList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[milestones.ListOutput](ctx, "gitlab_milestone_list", milestones.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list milestones")
	t.Logf("Listed %d milestones", len(out.Milestones))
}

// Project members.

// testProjectMembersList lists project members and verifies at least the
// current user appears (as the owner).
func testProjectMembersList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[members.ListOutput](ctx, "gitlab_project_members_list", members.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list project members")
	requireTrue(t, len(out.Members) >= 1, "expected at least 1 member (project owner), got %d", len(out.Members))
	t.Logf("Listed %d project members", len(out.Members))
}

// Project upload.

// testProjectUpload uploads a small text file to the project and verifies
// the returned markdown reference.
func testProjectUpload(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	content := base64.StdEncoding.EncodeToString([]byte("E2E upload test content"))
	out, err := callTool[uploads.UploadOutput](ctx, "gitlab_project_upload", uploads.UploadInput{
		ProjectID:     pidStr(),
		Filename:      "e2e-test-upload.txt",
		ContentBase64: content,
	})
	requireNoError(t, err, "upload file to project")
	requireTrue(t, out.URL != "", "upload URL should not be empty")
	requireTrue(t, out.Markdown != "", "upload markdown should not be empty")
	t.Logf("Uploaded file: %s (markdown=%s)", out.URL, out.Markdown)
}

// MR commits & pipelines.

// testMRCommits lists commits in the merge request and verifies at least
// one commit exists.
func testMRCommits(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mergerequests.CommitsOutput](ctx, "gitlab_mr_commits", mergerequests.CommitsInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "list MR commits")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 MR commit, got %d", len(out.Commits))
	t.Logf("MR !%d has %d commits", state.mrIID, len(out.Commits))
}

// testMRPipelines lists pipelines for the merge request (may be empty
// if no CI config exists in the project).
func testMRPipelines(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mergerequests.PipelinesOutput](ctx, "gitlab_mr_pipelines", mergerequests.PipelinesInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "list MR pipelines")
	t.Logf("MR !%d has %d pipelines", state.mrIID, len(out.Pipelines))
}

// MR rebase.

// testRebaseMR triggers a rebase of the MR source branch while it is open.
func testRebaseMR(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mergerequests.RebaseOutput](ctx, "gitlab_mr_rebase", mergerequests.RebaseInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
		SkipCI:    true,
	})
	requireNoError(t, err, "rebase MR")
	t.Logf("Rebase MR !%d: in_progress=%v", state.mrIID, out.RebaseInProgress)
}

// Search.

// testSearchCode searches for code within the project and verifies results.
func testSearchCode(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[search.CodeOutput](ctx, "gitlab_search_code", search.CodeInput{
		ProjectID: pidStr(),
		Query:     "multiply",
	})
	requireNoError(t, err, "search code")
	// Search indexing may have a delay; just verify the call succeeds.
	t.Logf("Code search for 'multiply': %d results", len(out.Blobs))
}

// testSearchMergeRequests searches for merge requests within the project.
func testSearchMergeRequests(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[search.MergeRequestsOutput](ctx, "gitlab_search_merge_requests", search.MergeRequestsInput{
		ProjectID: pidStr(),
		Query:     "multiply",
	})
	requireNoError(t, err, "search merge requests")
	t.Logf("MR search for 'multiply': %d results", len(out.MergeRequests))
}

// Group tools (read-only).

// testGroupList lists groups accessible to the authenticated user and stores
// the first group ID for subsequent tests. Skips dependent tests if no groups exist.
func testGroupList(ctx context.Context, t *testing.T) {
	out, err := callTool[groups.ListOutput](ctx, "gitlab_group_list", groups.ListInput{})
	requireNoError(t, err, "list groups")
	t.Logf("Found %d groups", len(out.Groups))
	if len(out.Groups) > 0 {
		state.groupID = out.Groups[0].ID
		state.groupPath = out.Groups[0].FullPath
		t.Logf("Using group %d (%s) for subsequent tests", state.groupID, out.Groups[0].FullPath)
	}
}

// testGroupGet retrieves group details. Skips if no group was discovered.
func testGroupGet(ctx context.Context, t *testing.T) {
	if state.groupID == 0 {
		t.Skip("no groups available — skipping group_get")
	}
	gid := strconv.FormatInt(state.groupID, 10)
	out, err := callTool[groups.Output](ctx, "gitlab_group_get", groups.GetInput{
		GroupID: toolutil.StringOrInt(gid),
	})
	requireNoError(t, err, "get group")
	requireTrue(t, out.ID == state.groupID, "expected group ID %d, got %d", state.groupID, out.ID)
	t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
}

// testGroupMembersList lists members of the discovered group. Skips if none available.
func testGroupMembersList(ctx context.Context, t *testing.T) {
	if state.groupID == 0 {
		t.Skip("no groups available — skipping group_members_list")
	}
	gid := strconv.FormatInt(state.groupID, 10)
	out, err := callTool[groups.MemberListOutput](ctx, "gitlab_group_members_list", groups.MembersListInput{
		GroupID: toolutil.StringOrInt(gid),
	})
	requireNoError(t, err, "list group members")
	t.Logf("Group %d has %d members", state.groupID, len(out.Members))
}

// testSubgroupsList lists subgroups of the discovered group. May return empty.
func testSubgroupsList(ctx context.Context, t *testing.T) {
	if state.groupID == 0 {
		t.Skip("no groups available — skipping subgroups_list")
	}
	gid := strconv.FormatInt(state.groupID, 10)
	out, err := callTool[groups.ListOutput](ctx, "gitlab_subgroups_list", groups.SubgroupsListInput{
		GroupID: toolutil.StringOrInt(gid),
	})
	requireNoError(t, err, "list subgroups")
	t.Logf("Group %d has %d subgroups", state.groupID, len(out.Groups))
}

// Pipeline list (read-only).

// testPipelineList lists pipelines on the test project. May return empty if
// no CI configuration exists.
func testPipelineList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[pipelines.ListOutput](ctx, "gitlab_pipeline_list", pipelines.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list pipelines")
	t.Logf("Project has %d pipelines", len(out.Pipelines))
}

// Package lifecycle.

const (
	testPackageName    = "e2e-test-pkg"
	testPackageVersion = "1.0.0"
	testPackageFile    = "hello.txt"
	msgPackageIDNotSet = "package ID not set"
)

// testPackagePublish publishes a small file to the Generic Package Registry
// using base64 content and stores the package/file IDs.
func testPackagePublish(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	content := base64.StdEncoding.EncodeToString([]byte("E2E package file content"))
	out, err := callTool[packages.PublishOutput](ctx, "gitlab_package_publish", packages.PublishInput{
		ProjectID:      pidStr(),
		PackageName:    testPackageName,
		PackageVersion: testPackageVersion,
		FileName:       testPackageFile,
		ContentBase64:  content,
	})
	requireNoError(t, err, "publish package file")
	requireTrue(t, out.PackageID > 0, "package ID should be positive, got %d", out.PackageID)
	requireTrue(t, out.PackageFileID > 0, "package file ID should be positive, got %d", out.PackageFileID)
	state.packageID = out.PackageID
	state.packageFileID = out.PackageFileID
	t.Logf("Published package ID=%d file_id=%d (%s)", out.PackageID, out.PackageFileID, out.FileName)
}

// testPackageList lists packages in the project and verifies the published
// package appears.
func testPackageList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.packageID > 0, "package ID not set — PackagePublish must run first")
	out, err := callTool[packages.ListOutput](ctx, "gitlab_package_list", packages.ListInput{
		ProjectID:   pidStr(),
		PackageName: testPackageName,
	})
	requireNoError(t, err, "list packages")
	found := false
	for _, p := range out.Packages {
		if p.ID == state.packageID {
			found = true
			break
		}
	}
	requireTrue(t, found, "package %d not found in list", state.packageID)
	t.Logf("Listed %d packages, found ID=%d", len(out.Packages), state.packageID)
}

// testPackageFileList lists files within the published package and verifies
// the uploaded file appears.
func testPackageFileList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.packageID > 0, msgPackageIDNotSet)
	out, err := callTool[packages.FileListOutput](ctx, "gitlab_package_file_list", packages.FileListInput{
		ProjectID: pidStr(),
		PackageID: toolutil.StringOrInt(strconv.FormatInt(state.packageID, 10)),
	})
	requireNoError(t, err, "list package files")
	requireTrue(t, len(out.Files) >= 1, "expected at least 1 file, got %d", len(out.Files))
	found := false
	for _, f := range out.Files {
		if f.FileName == testPackageFile {
			found = true
			break
		}
	}
	requireTrue(t, found, "file %q not found in package", testPackageFile)
	t.Logf("Package %d has %d files", state.packageID, len(out.Files))
}

// testPackageDownload downloads the published package file to a temp directory
// and verifies the content.
func testPackageDownload(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.packageID > 0, msgPackageIDNotSet)
	outputPath := filepath.Join(t.TempDir(), testPackageFile)
	out, err := callTool[packages.DownloadOutput](ctx, "gitlab_package_download", packages.DownloadInput{
		ProjectID:      pidStr(),
		PackageName:    testPackageName,
		PackageVersion: testPackageVersion,
		FileName:       testPackageFile,
		OutputPath:     outputPath,
	})
	requireNoError(t, err, "download package file")
	requireTrue(t, out.Size > 0, "downloaded file size should be positive, got %d", out.Size)
	requireTrue(t, out.SHA256 != "", "SHA256 should not be empty")

	data, err := os.ReadFile(outputPath)
	requireNoError(t, err, "read downloaded file")
	requireTrue(t, string(data) == "E2E package file content", "expected original content, got %q", string(data))
	t.Logf("Downloaded %s (%d bytes, sha256=%s)", outputPath, out.Size, out.SHA256)
}

// testPackageFileDelete deletes the file from the package.
func testPackageFileDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.packageFileID > 0, "package file ID not set")
	err := callToolVoid(ctx, "gitlab_package_file_delete", packages.FileDeleteInput{
		ProjectID:     pidStr(),
		PackageID:     toolutil.StringOrInt(strconv.FormatInt(state.packageID, 10)),
		PackageFileID: toolutil.StringOrInt(strconv.FormatInt(state.packageFileID, 10)),
	})
	requireNoError(t, err, "delete package file")
	t.Logf("Deleted package file ID=%d from package %d", state.packageFileID, state.packageID)
	state.packageFileID = 0
}

// testPackageDelete deletes the package from the registry.
func testPackageDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.packageID > 0, msgPackageIDNotSet)
	err := callToolVoid(ctx, "gitlab_package_delete", packages.DeleteInput{
		ProjectID: pidStr(),
		PackageID: toolutil.StringOrInt(strconv.FormatInt(state.packageID, 10)),
	})
	requireNoError(t, err, "delete package")
	t.Logf("Deleted package ID=%d", state.packageID)
	state.packageID = 0
}

// Upload with file_path.

// testUploadFilePath uploads a file using a local file_path instead of base64.
func testUploadFilePath(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	tmpFile := filepath.Join(t.TempDir(), "e2e-filepath-upload.txt")
	err := os.WriteFile(tmpFile, []byte("E2E file_path upload content"), 0o644)
	requireNoError(t, err, "create temp file for upload")

	out, err := callTool[uploads.UploadOutput](ctx, "gitlab_project_upload", uploads.UploadInput{
		ProjectID: pidStr(),
		Filename:  "e2e-filepath-upload.txt",
		FilePath:  tmpFile,
	})
	requireNoError(t, err, "upload file via file_path")
	requireTrue(t, out.URL != "", "upload URL should not be empty")
	requireTrue(t, out.Markdown != "", "upload markdown should not be empty")
	t.Logf("Uploaded via file_path: %s (markdown=%s)", out.URL, out.Markdown)
}

// Step 4: Create Merge Request.

// testCreateMR creates a merge request from the feature branch to main
// and stores its IID in the global test state.
func testCreateMR(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[mergerequests.Output](ctx, "gitlab_mr_create", mergerequests.CreateInput{
		ProjectID:    pidStr(),
		SourceBranch: testE2EBranch,
		TargetBranch: defaultBranch,
		Title:        "feat: add multiply function [E2E test]",
		Description:  "This MR adds a `multiply` function and updates main().\n\n**E2E automated test** — will be cleaned up.",
	})
	requireNoError(t, err, "create MR")
	requireTrue(t, out.IID > 0, "MR IID should be positive, got %d", out.IID)
	requireTrue(t, out.State == "opened", "MR state should be 'opened', got %q", out.State)

	state.mrIID = out.IID
	t.Logf("Created MR !%d: %s", out.IID, out.Title)
}

// Step 5: Get MR.

// testGetMR retrieves the merge request by IID and verifies its source
// branch.
func testGetMR(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mergerequests.Output](ctx, "gitlab_mr_get", mergerequests.GetInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "get MR")
	requireTrue(t, out.IID == state.mrIID, "expected MR IID %d, got %d", state.mrIID, out.IID)
	requireTrue(t, out.SourceBranch == testE2EBranch, "expected source branch 'feature/e2e-changes', got %q", out.SourceBranch)
	t.Logf("Got MR !%d state=%s merge_status=%s", out.IID, out.State, out.MergeStatus)
}

// Step 6: List MRs.

// testListMRs lists open merge requests and verifies the test MR appears
// in the result.
func testListMRs(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[mergerequests.ListOutput](ctx, "gitlab_mr_list", mergerequests.ListInput{
		ProjectID: pidStr(),
		State:     "opened",
	})
	requireNoError(t, err, "list MRs")
	requireTrue(t, len(out.MergeRequests) >= 1, "expected at least 1 open MR, got %d", len(out.MergeRequests))

	found := false
	for _, mr := range out.MergeRequests {
		if mr.IID == state.mrIID {
			found = true
			break
		}
	}
	requireTrue(t, found, "MR !%d not found in list", state.mrIID)
	t.Logf("Listed %d open MRs, found !%d", len(out.MergeRequests), state.mrIID)
}

// Update MR metadata (title & description).

// testUpdateMR modifies the merge request title and description via the
// gitlab_mr_update MCP tool.
func testUpdateMR(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mergerequests.Output](ctx, "gitlab_mr_update", mergerequests.UpdateInput{
		ProjectID:   pidStr(),
		MRIID:       state.mrIID,
		Title:       "feat: add multiply function [E2E test] (updated)",
		Description: "Updated description via `gitlab_mr_update` E2E test.",
	})
	requireNoError(t, err, "update MR")
	requireTrue(t, out.IID == state.mrIID, "expected MR IID %d, got %d", state.mrIID, out.IID)
	requireTrue(t, out.Title == "feat: add multiply function [E2E test] (updated)", "expected updated title, got %q", out.Title)
	t.Logf("Updated MR !%d title and description", state.mrIID)
}

// Step 7: Add Note (comment).

// testAddNote creates a general comment on the merge request and stores
// the note ID in the global test state.
func testAddNote(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mrnotes.Output](ctx, "gitlab_mr_note_create", mrnotes.CreateInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
		Body:      "**E2E Bot**: This merge request looks good! The `multiply` function is well-implemented.",
	})
	requireNoError(t, err, "create note")
	requireTrue(t, out.ID > 0, "note ID should be positive, got %d", out.ID)

	state.noteID = out.ID
	t.Logf("Created note ID=%d on MR !%d", out.ID, state.mrIID)
}

// Step 8: List Notes.

// testListNotes lists notes on the merge request and verifies the created
// note appears in the result.
func testListNotes(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mrnotes.ListOutput](ctx, "gitlab_mr_notes_list", mrnotes.ListInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "list notes")

	found := false
	for _, n := range out.Notes {
		if n.ID == state.noteID {
			found = true
			break
		}
	}
	requireTrue(t, found, "note ID=%d not found in list", state.noteID)
	t.Logf("Listed %d notes on MR !%d", len(out.Notes), state.mrIID)
}

// Step 9: Update Note.

// testUpdateNote modifies the note body via the gitlab_mr_note_update
// MCP tool.
func testUpdateNote(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	requireTrue(t, state.noteID > 0, "note ID not set")

	out, err := callTool[mrnotes.Output](ctx, "gitlab_mr_note_update", mrnotes.UpdateInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
		NoteID:    state.noteID,
		Body:      "**E2E Bot** (updated): LGTM! The `multiply` function is correct and well-tested.",
	})
	requireNoError(t, err, "update note")
	requireTrue(t, out.ID == state.noteID, "expected note ID %d, got %d", state.noteID, out.ID)
	t.Logf("Updated note ID=%d", out.ID)
}

// Step 10: Delete Note.

// testDeleteNote deletes the note from the merge request.
func testDeleteNote(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	requireTrue(t, state.noteID > 0, "note ID not set")

	err := callToolVoid(ctx, "gitlab_mr_note_delete", mrnotes.DeleteInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
		NoteID:    state.noteID,
	})
	requireNoError(t, err, "delete note")
	t.Logf("Deleted note ID=%d", state.noteID)
	state.noteID = 0
}

// Step 11: Get MR Changes (diffs).

// testGetMRChanges retrieves the diff changes for the merge request and
// verifies at least one file changed.
func testGetMRChanges(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mrchanges.Output](ctx, "gitlab_mr_changes_get", mrchanges.GetInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "get MR changes")
	requireTrue(t, len(out.Changes) > 0, "expected at least 1 changed file, got %d", len(out.Changes))

	for _, c := range out.Changes {
		t.Logf("  Changed file: %s (new=%v, deleted=%v)", c.NewPath, c.NewFile, c.DeletedFile)
	}
}

// Step 12: Create Inline Discussion (code review on specific line).

// testCreateInlineDiscussion creates a code review discussion on a specific
// line of main.go using diff position metadata from the MR versions API.
func testCreateInlineDiscussion(ctx context.Context, t *testing.T) {
	requireMRIID(t)

	// Get diff SHAs from the MR versions.
	versions, _, err := state.glClient.GL().MergeRequests.GetMergeRequestDiffVersions(
		int(state.projectID), state.mrIID, nil,
	)
	requireNoError(t, err, "get MR diff versions")
	requireTrue(t, len(versions) > 0, "expected at least 1 diff version")

	v := versions[0]
	t.Logf("Using diff version: base=%s start=%s head=%s", v.BaseCommitSHA, v.StartCommitSHA, v.HeadCommitSHA)

	out, err := callTool[mrdiscussions.Output](ctx, "gitlab_mr_discussion_create", mrdiscussions.CreateInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
		Body:      "**Code Review**: Consider adding input validation — what if `b` is zero and this evolves into a division function?",
		Position: &mrdiscussions.DiffPosition{
			BaseSHA:  v.BaseCommitSHA,
			StartSHA: v.StartCommitSHA,
			HeadSHA:  v.HeadCommitSHA,
			NewPath:  testFileMainGo,
			NewLine:  17,
		},
	})
	requireNoError(t, err, "create inline discussion")
	requireTrue(t, out.ID != "", "discussion ID should not be empty")

	state.discussionID = out.ID
	t.Logf("Created inline discussion %s on main.go:17", out.ID)
}

// Step 13: Reply to Discussion.

// testReplyToDiscussion adds a reply to the inline discussion created
// in the previous step.
func testReplyToDiscussion(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	requireTrue(t, state.discussionID != "", "discussion ID not set")

	out, err := callTool[mrdiscussions.NoteOutput](ctx, "gitlab_mr_discussion_reply", mrdiscussions.ReplyInput{
		ProjectID:    pidStr(),
		MRIID:        state.mrIID,
		DiscussionID: state.discussionID,
		Body:         "Good point! I'll add a guard clause in a follow-up commit. For now, multiply is safe since Go handles zero multiplication fine.",
	})
	requireNoError(t, err, "reply to discussion")
	requireTrue(t, out.ID > 0, "reply note ID should be positive")
	t.Logf("Replied to discussion %s with note ID=%d", state.discussionID, out.ID)
}

// Step 14: Resolve Discussion.

// testResolveDiscussion marks the inline discussion as resolved.
func testResolveDiscussion(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	requireTrue(t, state.discussionID != "", "discussion ID not set")

	out, err := callTool[mrdiscussions.Output](ctx, "gitlab_mr_discussion_resolve", mrdiscussions.ResolveInput{
		ProjectID:    pidStr(),
		MRIID:        state.mrIID,
		DiscussionID: state.discussionID,
		Resolved:     true,
	})
	requireNoError(t, err, "resolve discussion")
	requireTrue(t, out.ID == state.discussionID, "expected discussion %s, got %s", state.discussionID, out.ID)
	t.Logf("Resolved discussion %s", out.ID)
}

// Step 15: List Discussions.

// testListDiscussions lists discussions on the merge request and verifies
// at least one exists.
func testListDiscussions(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mrdiscussions.ListOutput](ctx, "gitlab_mr_discussion_list", mrdiscussions.ListInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "list discussions")
	requireTrue(t, len(out.Discussions) >= 1, "expected at least 1 discussion, got %d", len(out.Discussions))
	t.Logf("Listed %d discussions on MR !%d", len(out.Discussions), state.mrIID)
}

// Step 16: Approve MR.

// testApproveMR approves the merge request via the gitlab_mr_approve
// MCP tool.
func testApproveMR(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mergerequests.ApproveOutput](ctx, "gitlab_mr_approve", mergerequests.ApproveInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "approve MR")
	t.Logf("Approved MR !%d (approved=%v, approved_by=%d)", state.mrIID, out.Approved, out.ApprovedBy)
}

// Step 17: Unapprove MR.

// testUnapproveMR revokes the approval from the merge request.
func testUnapproveMR(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	err := callToolVoid(ctx, "gitlab_mr_unapprove", mergerequests.ApproveInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "unapprove MR")
	t.Logf("Unapproved MR !%d", state.mrIID)
}

// Step 18: Merge MR.

// testMergeMR merges the merge request with source branch removal and
// verifies the state is "merged".
func testMergeMR(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	out, err := callTool[mergerequests.Output](ctx, "gitlab_mr_merge", mergerequests.MergeInput{
		ProjectID:                pidStr(),
		MRIID:                    state.mrIID,
		ShouldRemoveSourceBranch: gl.Ptr(true), //nolint:modernize // gl.Ptr is the idiomatic GitLab client helper
	})
	requireNoError(t, err, "merge MR")
	requireTrue(t, out.State == "merged", "expected state 'merged', got %q", out.State)
	t.Logf("Merged MR !%d", state.mrIID)
}

// Step 19: Update Project.

// testUpdateProject updates the project description via the
// gitlab_project_update MCP tool.
func testUpdateProject(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[projects.Output](ctx, "gitlab_project_update", projects.UpdateInput{
		ProjectID:   pidStr(),
		Description: "E2E test project — UPDATED by automated tests, ready for deletion",
	})
	requireNoError(t, err, "update project")
	requireTrue(t, out.ID == state.projectID, "expected project ID %d, got %d", state.projectID, out.ID)
	t.Logf("Updated project %s description", state.projectPath)
}

// Step 20: List Projects.

// testListProjects lists owned projects and verifies the test project
// appears in the result.
func testListProjects(ctx context.Context, t *testing.T) {
	out, err := callTool[projects.ListOutput](ctx, "gitlab_project_list", projects.ListInput{
		Owned: true,
	})
	requireNoError(t, err, "list projects")
	requireTrue(t, len(out.Projects) >= 1, "expected at least 1 project, got %d", len(out.Projects))

	found := false
	for _, p := range out.Projects {
		if p.ID == state.projectID {
			found = true
			break
		}
	}
	requireTrue(t, found, "project %d not found in owned projects list", state.projectID)
	t.Logf("Found %d owned projects, including test project", len(out.Projects))
}

// Cleanup: Delete Project.

// Push Rules.

// testAddPushRule adds a push rule configuration to the test project.
func testAddPushRule(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[projects.PushRuleOutput](ctx, "gitlab_project_add_push_rule", projects.AddPushRuleInput{
		ProjectID:          pidStr(),
		CommitMessageRegex: "^[A-Z].*",
		MaxFileSize:        int64Ptr(50),
	})
	requireNoError(t, err, "add push rule")
	requireTrue(t, out.ID > 0, "push rule ID should be positive, got %d", out.ID)
	t.Logf("Added push rule %d to project %s", out.ID, state.projectPath)
}

// testGetPushRules retrieves the push rule configuration from the test project.
func testGetPushRules(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[projects.PushRuleOutput](ctx, "gitlab_project_get_push_rules", projects.GetPushRulesInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "get push rules")
	requireTrue(t, out.ID > 0, "push rule ID should be positive, got %d", out.ID)
	requireTrue(t, out.MaxFileSize == 50, "expected max_file_size=50, got %d", out.MaxFileSize)
	t.Logf("Got push rules for project %s: max_file_size=%d", state.projectPath, out.MaxFileSize)
}

// testEditPushRule modifies the push rule configuration of the test project.
func testEditPushRule(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[projects.PushRuleOutput](ctx, "gitlab_project_edit_push_rule", projects.EditPushRuleInput{
		ProjectID:   pidStr(),
		MaxFileSize: int64Ptr(100),
	})
	requireNoError(t, err, "edit push rule")
	requireTrue(t, out.MaxFileSize == 100, "expected max_file_size=100 after edit, got %d", out.MaxFileSize)
	t.Logf("Edited push rule: max_file_size=%d", out.MaxFileSize)
}

// testDeletePushRule removes the push rule configuration from the test project.
func testDeletePushRule(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	err := callToolVoid(ctx, "gitlab_project_delete_push_rule", projects.DeletePushRuleInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "delete push rule")
	t.Logf("Deleted push rules from project %s", state.projectPath)
}

// User-scoped project listings.

// testListUserContributed lists projects the authenticated user has contributed to.
func testListUserContributed(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	if user == "" {
		t.Skip("GITLAB_USER not set, skipping user contributed projects test")
	}
	out, err := callTool[projects.ListOutput](ctx, "gitlab_project_list_user_contributed", projects.ListUserContributedProjectsInput{
		UserID: toolutil.StringOrInt(user),
	})
	requireNoError(t, err, "list user contributed projects")
	t.Logf("User %s has contributed to %d projects", user, len(out.Projects))
}

// testListUserStarred lists projects the authenticated user has starred.
func testListUserStarred(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	if user == "" {
		t.Skip("GITLAB_USER not set, skipping user starred projects test")
	}
	out, err := callTool[projects.ListOutput](ctx, "gitlab_project_list_user_starred", projects.ListUserStarredProjectsInput{
		UserID: toolutil.StringOrInt(user),
	})
	requireNoError(t, err, "list user starred projects")
	t.Logf("User %s has starred %d projects", user, len(out.Projects))
}

// GraphQL tools (branch rules, CI catalog, vulnerabilities, custom emoji).

// testListBranchRules queries GraphQL branch rules for the E2E project.
// The project should have at least the default branch protection rule.
func testListBranchRules(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[branchrules.ListOutput](ctx, "gitlab_list_branch_rules", branchrules.ListInput{
		ProjectPath: state.projectPath,
	})
	requireNoError(t, err, "list branch rules")
	t.Logf("Project %s has %d branch rules", state.projectPath, len(out.Rules))
}

// testListCatalogResources queries the CI/CD Catalog via GraphQL.
// The result may be empty if no catalog resources exist on the instance.
func testListCatalogResources(ctx context.Context, t *testing.T) {
	out, err := callTool[cicatalog.ListOutput](ctx, "gitlab_list_catalog_resources", cicatalog.ListInput{})
	requireNoError(t, err, "list catalog resources")
	t.Logf("Found %d CI/CD catalog resources", len(out.Resources))
}

// testVulnerabilitySeverityCount queries vulnerability severity counts via
// GraphQL. On instances without Ultimate license the GraphQL field may return
// zeros; the test verifies the call succeeds and the Total is non-negative.
func testVulnerabilitySeverityCount(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[vulnerabilities.SeverityCountOutput](ctx, "gitlab_vulnerability_severity_count", vulnerabilities.SeverityCountInput{
		ProjectPath: state.projectPath,
	})
	if err != nil {
		t.Skipf("vulnerability_severity_count not available (may require Ultimate): %v", err)
	}
	requireTrue(t, out.Total >= 0, "expected non-negative total, got %d", out.Total)
	t.Logf("Vulnerability severity counts: critical=%d high=%d medium=%d low=%d total=%d",
		out.Critical, out.High, out.Medium, out.Low, out.Total)
}

// testListVulnerabilities queries project vulnerabilities via GraphQL.
// On instances without security scanners or Ultimate license the list will be
// empty; the test verifies the call succeeds.
func testListVulnerabilities(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[vulnerabilities.ListOutput](ctx, "gitlab_list_vulnerabilities", vulnerabilities.ListInput{
		ProjectPath: state.projectPath,
	})
	if err != nil {
		t.Skipf("list_vulnerabilities not available (may require Ultimate): %v", err)
	}
	t.Logf("Project %s has %d vulnerabilities", state.projectPath, len(out.Vulnerabilities))
}

// testListCustomEmoji queries custom emoji for the discovered group via
// GraphQL. Skips if no group was found.
func testListCustomEmoji(ctx context.Context, t *testing.T) {
	if state.groupPath == "" {
		t.Skip("no groups available — skipping list custom emoji")
	}
	out, err := callTool[customemoji.ListOutput](ctx, "gitlab_list_custom_emoji", customemoji.ListInput{
		GroupPath: state.groupPath,
	})
	if err != nil {
		t.Skipf("list_custom_emoji not available (may require Premium): %v", err)
	}
	t.Logf("Group %s has %d custom emoji", state.groupPath, len(out.Emoji))
}

// int64Ptr returns a pointer to the given int64.
func int64Ptr(v int64) *int64 { return &v } //nolint:modernize // used in multiple call sites

// testDeleteProject deletes the E2E test project as cleanup and resets
// the project ID in the global test state.
func testDeleteProject(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	err := callToolVoid(ctx, "gitlab_project_delete", projects.DeleteInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "delete project")
	t.Logf("Deleted project %s (ID=%d)", state.projectPath, state.projectID)
	state.projectID = 0
}

// Helpers.

// pidStr returns the global test project ID as a StringOrInt.
func pidStr() toolutil.StringOrInt {
	return toolutil.StringOrInt(strconv.FormatInt(state.projectID, 10))
}

// requireProjectID fails the test if the project ID has not been set
// by a prior step.
func requireProjectID(t *testing.T) {
	t.Helper()
	if state.projectID == 0 {
		t.Fatal("project ID not set \u2014 CreateProject must run first")
	}
}

// requireMRIID fails the test if the merge request IID has not been set
// by a prior step.
func requireMRIID(t *testing.T) {
	t.Helper()
	requireProjectID(t)
	if state.mrIID == 0 {
		t.Fatal("MR IID not set — CreateMR must run first")
	}
}

// waitForBranch polls GitLab until the given branch exists in the test project.
func waitForBranch(ctx context.Context, t *testing.T, branch string) {
	t.Helper()
	pid := int(state.projectID)
	for range 15 {
		_, resp, err := state.glClient.GL().Branches.GetBranch(pid, branch)
		if err == nil {
			t.Logf("Branch %q ready", branch)
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
		requireNoError(t, err, "get branch "+branch)
	}
	t.Fatalf("branch %q not available after 15s", branch)
}
