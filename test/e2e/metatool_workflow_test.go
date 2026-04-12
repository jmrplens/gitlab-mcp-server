//go:build e2e

// metatool_workflow_test.go contains end-to-end workflow tests that exercise
// the complete GitLab project lifecycle using domain meta-tools
// (gitlab_project, gitlab_branch, gitlab_repository, etc.) instead of
// individual MCP tools.
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
)

// Branch name, tag version, and assertion format used across meta-tool
// E2E workflow steps.
const (
	testMetaBranch        = "feature/meta-changes"
	testMetaTag           = "v0.1.0-meta"
	fmtExpectedTag        = "expected tag, got %q"
	msgMetaIssueIIDNotSet = "issueIID not set"
)

// metaState holds shared state for the meta-tool workflow.
// Separated from the individual-tools state to allow independent execution.
type metaState struct {
	projectID     int64
	projectPath   string
	mrIID         int64
	noteID        int64
	discussionID  string
	releaseLinkID int64
	lastCommitSHA string // SHA from most recent commit
	issueIID      int64  // issue IID for issue lifecycle tests
	issueNoteID   int64  // issue note ID for issue note tests
	groupID       int64  // group ID discovered via group list (0 if none)
	groupPath     string // group full path discovered via group list
	packageID     int64  // package ID for package lifecycle tests
	packageFileID int64  // package file ID for package file tests
}

// mState is the shared [metaState] instance used by [TestMetaToolWorkflow]
// sequential test steps.
var mState metaState

// TestMetaToolWorkflow exercises the same lifecycle as TestFullWorkflow but
// through the 8 domain meta-tools (gitlab_project, gitlab_branch, etc.)
// instead of the 52 individual tools.
func TestMetaToolWorkflow(t *testing.T) {
	if state.metaSession == nil {
		t.Skip("meta session not configured — set META_TOOLS=true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// User identity.
	t.Run("00_UserCurrent", func(t *testing.T) { metaUserCurrent(ctx, t) })

	// Project.
	t.Run("01_CreateProject", func(t *testing.T) { metaCreateProject(ctx, t) })
	t.Run("02_GetProject", func(t *testing.T) { metaGetProject(ctx, t) })
	t.Run("03_UnprotectMain", func(t *testing.T) { metaUnprotectMain(ctx, t) })

	// Repository: commits & files.
	t.Run("04_CommitCreate", func(t *testing.T) { metaCommitCreate(ctx, t) })
	t.Run("05_FileGet", func(t *testing.T) { metaFileGet(ctx, t) })

	// Commit inspection.
	t.Run("05a_CommitList", func(t *testing.T) { metaCommitList(ctx, t) })
	t.Run("05b_CommitGet", func(t *testing.T) { metaCommitGet(ctx, t) })
	t.Run("05c_CommitDiff", func(t *testing.T) { metaCommitDiff(ctx, t) })

	// Repository tree.
	t.Run("05d_RepositoryTree", func(t *testing.T) { metaRepositoryTree(ctx, t) })

	// Branch management.
	t.Run("06_BranchCreate", func(t *testing.T) { metaBranchCreate(ctx, t) })
	t.Run("06a_BranchGet", func(t *testing.T) { metaBranchGet(ctx, t) })
	t.Run("07_BranchList", func(t *testing.T) { metaBranchList(ctx, t) })
	t.Run("08_BranchProtect", func(t *testing.T) { metaBranchProtect(ctx, t) })
	t.Run("09_ListProtectedBranches", func(t *testing.T) { metaListProtectedBranches(ctx, t) })
	t.Run("10_BranchUnprotect", func(t *testing.T) { metaBranchUnprotect(ctx, t) })

	// Commit feature changes.
	t.Run("11_CommitFeatureChanges", func(t *testing.T) { metaCommitFeatureChanges(ctx, t) })

	// Repository compare.
	t.Run("11a_RepositoryCompare", func(t *testing.T) { metaRepositoryCompare(ctx, t) })

	// Tags & releases.
	t.Run("12_TagCreate", func(t *testing.T) { metaTagCreate(ctx, t) })
	t.Run("12a_TagGet", func(t *testing.T) { metaTagGet(ctx, t) })
	t.Run("13_TagList", func(t *testing.T) { metaTagList(ctx, t) })
	t.Run("14_ReleaseCreate", func(t *testing.T) { metaReleaseCreate(ctx, t) })
	t.Run("15_ReleaseGet", func(t *testing.T) { metaReleaseGet(ctx, t) })
	t.Run("16_ReleaseUpdate", func(t *testing.T) { metaReleaseUpdate(ctx, t) })
	t.Run("17_ReleaseList", func(t *testing.T) { metaReleaseList(ctx, t) })
	t.Run("18_ReleaseLinkCreate", func(t *testing.T) { metaReleaseLinkCreate(ctx, t) })
	t.Run("19_ReleaseLinkList", func(t *testing.T) { metaReleaseLinkList(ctx, t) })
	t.Run("20_ReleaseLinkDelete", func(t *testing.T) { metaReleaseLinkDelete(ctx, t) })
	t.Run("21_ReleaseDelete", func(t *testing.T) { metaReleaseDelete(ctx, t) })
	t.Run("22_TagDelete", func(t *testing.T) { metaTagDelete(ctx, t) })

	// Issue lifecycle.
	t.Run("22a_IssueCreate", func(t *testing.T) { metaIssueCreate(ctx, t) })
	t.Run("22b_IssueGet", func(t *testing.T) { metaIssueGet(ctx, t) })
	t.Run("22c_IssueList", func(t *testing.T) { metaIssueList(ctx, t) })
	t.Run("22d_IssueUpdate", func(t *testing.T) { metaIssueUpdate(ctx, t) })
	t.Run("22e_IssueNoteCreate", func(t *testing.T) { metaIssueNoteCreate(ctx, t) })
	t.Run("22f_IssueNoteList", func(t *testing.T) { metaIssueNoteList(ctx, t) })
	t.Run("22g_IssueDelete", func(t *testing.T) { metaIssueDelete(ctx, t) })

	// Labels & milestones.
	t.Run("22h_LabelList", func(t *testing.T) { metaLabelList(ctx, t) })
	t.Run("22i_MilestoneList", func(t *testing.T) { metaMilestoneList(ctx, t) })

	// Project members.
	t.Run("22j_ProjectMembersList", func(t *testing.T) { metaProjectMembersList(ctx, t) })

	// Project upload.
	t.Run("22k_ProjectUpload", func(t *testing.T) { metaProjectUpload(ctx, t) })

	// Merge request lifecycle.
	t.Run("23_CreateMR", func(t *testing.T) { metaCreateMR(ctx, t) })
	t.Run("24_GetMR", func(t *testing.T) { metaGetMR(ctx, t) })
	t.Run("25_ListMRs", func(t *testing.T) { metaListMRs(ctx, t) })
	t.Run("26_UpdateMR", func(t *testing.T) { metaUpdateMR(ctx, t) })

	// MR commits & pipelines.
	t.Run("26a_MRCommits", func(t *testing.T) { metaMRCommits(ctx, t) })
	t.Run("26b_MRPipelines", func(t *testing.T) { metaMRPipelines(ctx, t) })

	// Notes (via gitlab_mr_review).
	t.Run("27_NoteCreate", func(t *testing.T) { metaNoteCreate(ctx, t) })
	t.Run("28_NoteList", func(t *testing.T) { metaNoteList(ctx, t) })
	t.Run("29_NoteUpdate", func(t *testing.T) { metaNoteUpdate(ctx, t) })
	t.Run("30_NoteDelete", func(t *testing.T) { metaNoteDelete(ctx, t) })

	// MR review: diffs & discussions.
	t.Run("31_ChangesGet", func(t *testing.T) { metaChangesGet(ctx, t) })
	t.Run("32_DiscussionCreate", func(t *testing.T) { metaDiscussionCreate(ctx, t) })
	t.Run("33_DiscussionReply", func(t *testing.T) { metaDiscussionReply(ctx, t) })
	t.Run("34_DiscussionResolve", func(t *testing.T) { metaDiscussionResolve(ctx, t) })
	t.Run("35_DiscussionList", func(t *testing.T) { metaDiscussionList(ctx, t) })

	// MR rebase (before merge).
	t.Run("35a_RebaseMR", func(t *testing.T) { metaRebaseMR(ctx, t) })

	// Approve, merge, project update/list.
	t.Run("36_ApproveMR", func(t *testing.T) { metaApproveMR(ctx, t) })
	t.Run("37_UnapproveMR", func(t *testing.T) { metaUnapproveMR(ctx, t) })
	t.Run("38_MergeMR", func(t *testing.T) { metaMergeMR(ctx, t) })

	// Search (after merge so content is on default branch).
	t.Run("38a_SearchCode", func(t *testing.T) { metaSearchCode(ctx, t) })
	t.Run("38b_SearchMergeRequests", func(t *testing.T) { metaSearchMergeRequests(ctx, t) })

	// Group tools (read-only, use whatever groups are accessible).
	t.Run("38c_GroupList", func(t *testing.T) { metaGroupList(ctx, t) })
	t.Run("38d_GroupGet", func(t *testing.T) { metaGroupGet(ctx, t) })
	t.Run("38e_GroupMembersList", func(t *testing.T) { metaGroupMembersList(ctx, t) })
	t.Run("38f_SubgroupsList", func(t *testing.T) { metaSubgroupsList(ctx, t) })
	t.Run("38g_GroupIssues", func(t *testing.T) { metaGroupIssues(ctx, t) })

	// Pipeline list (read-only, may return empty without CI config).
	t.Run("38h_PipelineList", func(t *testing.T) { metaPipelineList(ctx, t) })

	// Package lifecycle.
	t.Run("38i_PackagePublish", func(t *testing.T) { metaPackagePublish(ctx, t) })
	t.Run("38j_PackageList", func(t *testing.T) { metaPackageList(ctx, t) })
	t.Run("38k_PackageFileList", func(t *testing.T) { metaPackageFileList(ctx, t) })
	t.Run("38l_PackageDownload", func(t *testing.T) { metaPackageDownload(ctx, t) })
	t.Run("38m_PackageFileDelete", func(t *testing.T) { metaPackageFileDelete(ctx, t) })
	t.Run("38n_PackageDelete", func(t *testing.T) { metaPackageDelete(ctx, t) })

	// Upload with file_path (meta-tool).
	t.Run("38o_UploadFilePath", func(t *testing.T) { metaUploadFilePath(ctx, t) })

	t.Run("39_UpdateProject", func(t *testing.T) { metaUpdateProject(ctx, t) })
	t.Run("40_ListProjects", func(t *testing.T) { metaListProjects(ctx, t) })

	// Push rules.
	t.Run("41_AddPushRule", func(t *testing.T) { metaAddPushRule(ctx, t) })
	t.Run("42_GetPushRules", func(t *testing.T) { metaGetPushRules(ctx, t) })
	t.Run("43_EditPushRule", func(t *testing.T) { metaEditPushRule(ctx, t) })
	t.Run("44_DeletePushRule", func(t *testing.T) { metaDeletePushRule(ctx, t) })

	// User-scoped project listings.
	t.Run("45_ListUserContributed", func(t *testing.T) { metaListUserContributed(ctx, t) })
	t.Run("46_ListUserStarred", func(t *testing.T) { metaListUserStarred(ctx, t) })

	// GraphQL tools (branch rules, CI catalog, vulnerabilities, custom emoji).
	t.Run("47_BranchRuleList", func(t *testing.T) { metaBranchRuleList(ctx, t) })
	t.Run("48_CICatalogList", func(t *testing.T) { metaCICatalogList(ctx, t) })
	t.Run("49_VulnerabilitySeverityCount", func(t *testing.T) { metaVulnerabilitySeverityCount(ctx, t) })
	t.Run("50_VulnerabilityList", func(t *testing.T) { metaVulnerabilityList(ctx, t) })
	t.Run("51_CustomEmojiList", func(t *testing.T) { metaCustomEmojiList(ctx, t) })

	// Cleanup.
	t.Run("99_Cleanup_DeleteProject", func(t *testing.T) { metaDeleteProject(ctx, t) })
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// mPID returns the meta-tool test project ID as a string.
func mPID() string { return strconv.FormatInt(mState.projectID, 10) }

// requireMetaProjectID fails the test if the meta-tool project ID has not
// been set by a prior step.
func requireMetaProjectID(t *testing.T) {
	t.Helper()
	if mState.projectID == 0 {
		t.Fatal("meta project ID not set \u2014 CreateProject must run first")
	}
}

// requireMetaMRIID fails the test if the meta-tool merge request IID has
// not been set by a prior step.
func requireMetaMRIID(t *testing.T) {
	t.Helper()
	requireMetaProjectID(t)
	if mState.mrIID == 0 {
		t.Fatal("meta MR IID not set — CreateMR must run first")
	}
}

// ---------------------------------------------------------------------------
// Project meta-tool
// ---------------------------------------------------------------------------.

// metaCreateProject creates a new private GitLab project via the
// gitlab_project meta-tool and stores its ID and path in mState.
func metaCreateProject(ctx context.Context, t *testing.T) {
	name := uniqueName("e2e-meta")
	out, err := callMeta[projects.Output](ctx, "gitlab_project", "create", map[string]any{
		"name":                   name,
		"description":            "Meta-tool E2E test project — will be deleted automatically",
		"visibility":             "private",
		"initialize_with_readme": true,
		"default_branch":         defaultBranch,
	})
	requireNoError(t, err, "meta create project")
	requireTrue(t, out.ID > 0, "project ID should be positive")

	mState.projectID = out.ID
	mState.projectPath = out.PathWithNamespace
	t.Logf("Created project: %s (ID=%d)", mState.projectPath, mState.projectID)

	waitForBranchMeta(ctx, t, defaultBranch)
}

// metaGetProject retrieves the E2E test project by ID via the
// gitlab_project meta-tool and verifies its ID matches.
func metaGetProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.Output](ctx, "gitlab_project", "get", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta get project")
	requireTrue(t, out.ID == mState.projectID, "expected project ID %d, got %d", mState.projectID, out.ID)
	t.Logf("Got project %s", out.PathWithNamespace)
}

// metaUnprotectMain removes protection from the main branch via the
// gitlab_branch meta-tool so subsequent commits can be pushed directly.
// It first waits for GitLab to apply the default branch protection
// (async job after project creation) before attempting to unprotect.
func metaUnprotectMain(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	waitForBranchProtection(ctx, t, int(mState.projectID), defaultBranch)
	err := callMetaVoid(ctx, "gitlab_branch", "unprotect", map[string]any{
		"project_id":  mPID(),
		"branch_name": defaultBranch,
	})
	requireNoError(t, err, "meta unprotect main")
	t.Logf("Unprotected %s branch", defaultBranch)
}

// metaUpdateProject updates the project description via the
// gitlab_project meta-tool.
func metaUpdateProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.Output](ctx, "gitlab_project", "update", map[string]any{
		"project_id":  mPID(),
		"description": "Meta-tool E2E — UPDATED, ready for deletion",
	})
	requireNoError(t, err, "meta update project")
	requireTrue(t, out.ID == mState.projectID, "expected project ID %d, got %d", mState.projectID, out.ID)
	t.Logf("Updated project %s", mState.projectPath)
}

// metaListProjects lists owned projects via the gitlab_project meta-tool
// and verifies the test project appears in the result.
func metaListProjects(ctx context.Context, t *testing.T) {
	out, err := callMeta[projects.ListOutput](ctx, "gitlab_project", "list", map[string]any{
		"owned": true,
	})
	requireNoError(t, err, "meta list projects")
	requireTrue(t, len(out.Projects) >= 1, "expected at least 1 project")

	found := false
	for _, p := range out.Projects {
		if p.ID == mState.projectID {
			found = true
			break
		}
	}
	requireTrue(t, found, "project %d not in list", mState.projectID)
	t.Logf("Found %d owned projects", len(out.Projects))
}

// metaDeleteProject deletes the E2E test project via the gitlab_project
// meta-tool and resets the project ID.
func metaDeleteProject(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "delete", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta delete project")
	t.Logf("Deleted project %s (ID=%d)", mState.projectPath, mState.projectID)
	mState.projectID = 0
}

// Push Rules (meta-tool).

// metaAddPushRule adds push rules via the gitlab_project meta-tool.
func metaAddPushRule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.PushRuleOutput](ctx, "gitlab_project", "push_rule_add", map[string]any{
		"project_id":           mPID(),
		"commit_message_regex": "^[A-Z].*",
		"max_file_size":        50,
	})
	requireNoError(t, err, "meta add push rule")
	requireTrue(t, out.ID > 0, "push rule ID should be positive, got %d", out.ID)
	t.Logf("Added push rule %d via meta-tool", out.ID)
}

// metaGetPushRules retrieves push rules via the gitlab_project meta-tool.
func metaGetPushRules(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.PushRuleOutput](ctx, "gitlab_project", "push_rule_get", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta get push rules")
	requireTrue(t, out.ID > 0, "push rule ID should be positive")
	requireTrue(t, out.MaxFileSize == 50, "expected max_file_size=50, got %d", out.MaxFileSize)
	t.Logf("Got push rules via meta-tool: max_file_size=%d", out.MaxFileSize)
}

// metaEditPushRule modifies push rules via the gitlab_project meta-tool.
func metaEditPushRule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[projects.PushRuleOutput](ctx, "gitlab_project", "push_rule_edit", map[string]any{
		"project_id":    mPID(),
		"max_file_size": 100,
	})
	requireNoError(t, err, "meta edit push rule")
	requireTrue(t, out.MaxFileSize == 100, "expected max_file_size=100, got %d", out.MaxFileSize)
	t.Logf("Edited push rule via meta-tool: max_file_size=%d", out.MaxFileSize)
}

// metaDeletePushRule removes push rules via the gitlab_project meta-tool.
func metaDeletePushRule(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_project", "push_rule_delete", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta delete push rule")
	t.Logf("Deleted push rules via meta-tool")
}

// User-scoped project listings (meta-tool).

// metaListUserContributed lists user contributed projects via the gitlab_project meta-tool.
func metaListUserContributed(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	if user == "" {
		t.Skip("GITLAB_USER not set, skipping meta user contributed projects test")
	}
	out, err := callMeta[projects.ListOutput](ctx, "gitlab_project", "list_user_contributed", map[string]any{
		"user_id": user,
	})
	requireNoError(t, err, "meta list user contributed")
	t.Logf("User %s contributed to %d projects (via meta-tool)", user, len(out.Projects))
}

// metaListUserStarred lists user starred projects via the gitlab_project meta-tool.
func metaListUserStarred(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	if user == "" {
		t.Skip("GITLAB_USER not set, skipping meta user starred projects test")
	}
	out, err := callMeta[projects.ListOutput](ctx, "gitlab_project", "list_user_starred", map[string]any{
		"user_id": user,
	})
	requireNoError(t, err, "meta list user starred")
	t.Logf("User %s starred %d projects (via meta-tool)", user, len(out.Projects))
}

// GraphQL meta-tools (branch rules, CI catalog, vulnerabilities, custom emoji).

// metaBranchRuleList lists branch rules for the project via the
// gitlab_branch_rule meta-tool.
func metaBranchRuleList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branchrules.ListOutput](ctx, "gitlab_branch_rule", "list", map[string]any{
		"project_path": mState.projectPath,
	})
	requireNoError(t, err, "meta branch rule list")
	t.Logf("Project %s has %d branch rules (via meta-tool)", mState.projectPath, len(out.Rules))
}

// metaCICatalogList queries CI/CD Catalog resources via the gitlab_ci_catalog
// meta-tool.
func metaCICatalogList(ctx context.Context, t *testing.T) {
	out, err := callMeta[cicatalog.ListOutput](ctx, "gitlab_ci_catalog", "list", map[string]any{})
	requireNoError(t, err, "meta ci catalog list")
	t.Logf("Found %d CI/CD catalog resources (via meta-tool)", len(out.Resources))
}

// metaVulnerabilitySeverityCount queries vulnerability severity counts via
// the gitlab_vulnerability meta-tool. Skips if not available (requires Ultimate).
func metaVulnerabilitySeverityCount(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[vulnerabilities.SeverityCountOutput](ctx, "gitlab_vulnerability", "severity_count", map[string]any{
		"project_path": mState.projectPath,
	})
	if err != nil {
		t.Skipf("meta vulnerability severity_count not available (may require Ultimate): %v", err)
	}
	requireTrue(t, out.Total >= 0, "expected non-negative total, got %d", out.Total)
	t.Logf("Vulnerability severity counts via meta-tool: critical=%d high=%d medium=%d low=%d total=%d",
		out.Critical, out.High, out.Medium, out.Low, out.Total)
}

// metaVulnerabilityList lists vulnerabilities via the gitlab_vulnerability
// meta-tool. Skips if not available (requires Ultimate).
func metaVulnerabilityList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[vulnerabilities.ListOutput](ctx, "gitlab_vulnerability", "list", map[string]any{
		"project_path": mState.projectPath,
	})
	if err != nil {
		t.Skipf("meta vulnerability list not available (may require Ultimate): %v", err)
	}
	t.Logf("Project %s has %d vulnerabilities (via meta-tool)", mState.projectPath, len(out.Vulnerabilities))
}

// metaCustomEmojiList lists custom emoji for the discovered group via
// the gitlab_custom_emoji meta-tool. Skips if no group was found.
func metaCustomEmojiList(ctx context.Context, t *testing.T) {
	if mState.groupPath == "" {
		t.Skip("no groups available — skipping meta custom emoji list")
	}
	out, err := callMeta[customemoji.ListOutput](ctx, "gitlab_custom_emoji", "list", map[string]any{
		"group_path": mState.groupPath,
	})
	if err != nil {
		t.Skipf("meta custom emoji list not available (may require Premium): %v", err)
	}
	t.Logf("Group %s has %d custom emoji (via meta-tool)", mState.groupPath, len(out.Emoji))
}

// ---------------------------------------------------------------------------
// Repository meta-tool
// ---------------------------------------------------------------------------.

// metaCommitCreate creates main.go on the default branch via the
// gitlab_repository meta-tool commit_create action.
func metaCommitCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[commits.Output](ctx, "gitlab_repository", "commit_create", map[string]any{
		"project_id":     mPID(),
		"branch":         defaultBranch,
		"commit_message": "feat: add main.go via meta-tool E2E",
		"actions": []map[string]any{
			{
				"action":    "create",
				"file_path": testFileMainGo,
				"content":   "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, Meta-tool E2E!\")\n}\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n",
			},
		},
	})
	requireNoError(t, err, "meta commit create")
	requireTrue(t, out.ID != "", msgCommitIDEmpty)
	mState.lastCommitSHA = out.ID
	t.Logf("Committed main.go via meta-tool (SHA=%s)", out.ShortID)
}

// metaFileGet retrieves main.go content via the gitlab_repository meta-tool
// and verifies the file name and size.
func metaFileGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[files.Output](ctx, "gitlab_repository", "file_get", map[string]any{
		"project_id": mPID(),
		"file_path":  testFileMainGo,
		"ref":        defaultBranch,
	})
	requireNoError(t, err, "meta file get")
	requireTrue(t, out.FileName == testFileMainGo, "expected main.go, got %q", out.FileName)
	requireTrue(t, out.Size > 0, "file size should be positive")
	t.Logf("Got file %s (%d bytes)", out.FileName, out.Size)
}

// ---------------------------------------------------------------------------
// Branch meta-tool
// ---------------------------------------------------------------------------.

// metaBranchCreate creates the feature/meta-changes branch from the
// default branch via the gitlab_branch meta-tool.
func metaBranchCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.Output](ctx, "gitlab_branch", "create", map[string]any{
		"project_id":  mPID(),
		"branch_name": testMetaBranch,
		"ref":         defaultBranch,
	})
	requireNoError(t, err, "meta branch create")
	requireTrue(t, out.Name == testMetaBranch, "expected branch name, got %q", out.Name)
	t.Logf("Created branch %s", out.Name)
}

// metaBranchList lists all branches via the gitlab_branch meta-tool and
// verifies at least two exist.
func metaBranchList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ListOutput](ctx, "gitlab_branch", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta branch list")
	requireTrue(t, len(out.Branches) >= 2, "expected at least 2 branches, got %d", len(out.Branches))
	t.Logf("Listed %d branches", len(out.Branches))
}

// metaBranchProtect protects the feature branch with specified access
// levels via the gitlab_branch meta-tool.
func metaBranchProtect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ProtectedOutput](ctx, "gitlab_branch", "protect", map[string]any{
		"project_id":         mPID(),
		"branch_name":        testMetaBranch,
		"push_access_level":  40,
		"merge_access_level": 30,
	})
	requireNoError(t, err, "meta branch protect")
	requireTrue(t, out.Name == testMetaBranch, "expected protected branch name, got %q", out.Name)
	t.Logf("Protected branch %s", out.Name)
}

// metaListProtectedBranches lists protected branches via the gitlab_branch
// meta-tool and verifies the feature branch appears.
func metaListProtectedBranches(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.ProtectedListOutput](ctx, "gitlab_branch", "list_protected", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta list protected branches")
	requireTrue(t, len(out.Branches) >= 1, "expected at least 1 protected branch")

	found := false
	for _, b := range out.Branches {
		if b.Name == testMetaBranch {
			found = true
			break
		}
	}
	requireTrue(t, found, "feature/meta-changes not in protected branches")
	t.Logf("Listed %d protected branches", len(out.Branches))
}

// metaBranchUnprotect removes protection from the feature branch via the
// gitlab_branch meta-tool so commits can be pushed to it.
func metaBranchUnprotect(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_branch", "unprotect", map[string]any{
		"project_id":  mPID(),
		"branch_name": testMetaBranch,
	})
	requireNoError(t, err, "meta branch unprotect")
	t.Log("Unprotected feature/meta-changes")
}

// metaCommitFeatureChanges pushes an updated main.go with a multiply
// function to the feature branch via the gitlab_repository meta-tool.
func metaCommitFeatureChanges(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[commits.Output](ctx, "gitlab_repository", "commit_create", map[string]any{
		"project_id":     mPID(),
		"branch":         testMetaBranch,
		"commit_message": "refactor: add multiply via meta-tool",
		"actions": []map[string]any{
			{
				"action":    "update",
				"file_path": testFileMainGo,
				"content":   "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, Meta-tool E2E!\")\n\tresult := multiply(3, 4)\n\tfmt.Println(\"3 * 4 =\", result)\n}\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc multiply(a, b int) int {\n\treturn a * b\n}\n",
			},
		},
	})
	requireNoError(t, err, "meta commit feature changes")
	requireTrue(t, out.ID != "", msgCommitIDEmpty)
	t.Logf("Committed feature changes via meta-tool (SHA=%s)", out.ShortID)
}

// ---------------------------------------------------------------------------
// Tag meta-tool
// ---------------------------------------------------------------------------.

// metaTagCreate creates tag v0.1.0-meta on the default branch via the
// gitlab_tag meta-tool.
func metaTagCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.Output](ctx, "gitlab_tag", "create", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"ref":        defaultBranch,
		"message":    "Meta-tool E2E tag",
	})
	requireNoError(t, err, "meta tag create")
	requireTrue(t, out.Name == testMetaTag, "expected tag name, got %q", out.Name)
	t.Logf("Created tag %s", out.Name)
}

// metaTagList lists tags via the gitlab_tag meta-tool and verifies
// v0.1.0-meta appears in the result.
func metaTagList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.ListOutput](ctx, "gitlab_tag", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta tag list")
	requireTrue(t, len(out.Tags) >= 1, "expected at least 1 tag")

	found := false
	for _, tag := range out.Tags {
		if tag.Name == testMetaTag {
			found = true
			break
		}
	}
	requireTrue(t, found, "tag v0.1.0-meta not found")
	t.Logf("Listed %d tags", len(out.Tags))
}

// metaTagDelete deletes tag v0.1.0-meta via the gitlab_tag meta-tool.
func metaTagDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	err := callMetaVoid(ctx, "gitlab_tag", "delete", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta tag delete")
	t.Log("Deleted tag v0.1.0-meta")
}

// ---------------------------------------------------------------------------
// Release meta-tool
// ---------------------------------------------------------------------------.

// metaReleaseCreate creates a release for tag v0.1.0-meta via the
// gitlab_release meta-tool.
func metaReleaseCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "create", map[string]any{
		"project_id":  mPID(),
		"tag_name":    testMetaTag,
		"name":        "Meta-tool E2E Release v0.1.0-meta",
		"description": "Release created via meta-tool E2E testing.",
	})
	requireNoError(t, err, "meta release create")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Created release %s", out.Name)
}

// metaReleaseGet retrieves the release for v0.1.0-meta via the
// gitlab_release meta-tool and verifies its tag.
func metaReleaseGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "get", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta release get")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Got release %s (created=%s)", out.Name, out.CreatedAt)
}

// metaReleaseUpdate updates the release description via the gitlab_release
// meta-tool.
func metaReleaseUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "update", map[string]any{
		"project_id":  mPID(),
		"tag_name":    testMetaTag,
		"description": "Updated meta-tool E2E release.",
	})
	requireNoError(t, err, "meta release update")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Updated release %s", out.Name)
}

// metaReleaseList lists releases via the gitlab_release meta-tool and
// verifies at least one exists.
func metaReleaseList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.ListOutput](ctx, "gitlab_release", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta release list")
	requireTrue(t, len(out.Releases) >= 1, "expected at least 1 release")
	t.Logf("Listed %d releases", len(out.Releases))
}

// metaReleaseLinkCreate adds a package asset link to the release via the
// gitlab_release meta-tool and stores the link ID in mState.
func metaReleaseLinkCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releaselinks.Output](ctx, "gitlab_release", "link_create", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"name":       "Meta Binary (Linux arm64)",
		"url":        "https://example.com/releases/v0.1.0-meta/binary-linux-arm64",
		"link_type":  "package",
	})
	requireNoError(t, err, "meta release link create")
	requireTrue(t, out.ID > 0, "link ID should be positive")

	mState.releaseLinkID = out.ID
	t.Logf("Created release link ID=%d", out.ID)
}

// metaReleaseLinkList lists release links via the gitlab_release meta-tool
// and verifies at least one exists.
func metaReleaseLinkList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releaselinks.ListOutput](ctx, "gitlab_release", "link_list", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta release link list")
	requireTrue(t, len(out.Links) >= 1, "expected at least 1 link")
	t.Logf("Listed %d release links", len(out.Links))
}

// metaReleaseLinkDelete deletes the release asset link via the
// gitlab_release meta-tool.
func metaReleaseLinkDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.releaseLinkID > 0, "release link ID not set")

	out, err := callMeta[releaselinks.Output](ctx, "gitlab_release", "link_delete", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
		"link_id":    mState.releaseLinkID,
	})
	requireNoError(t, err, "meta release link delete")
	requireTrue(t, out.ID == mState.releaseLinkID, "expected link ID %d, got %d", mState.releaseLinkID, out.ID)
	t.Logf("Deleted release link ID=%d", out.ID)
	mState.releaseLinkID = 0
}

// metaReleaseDelete deletes the v0.1.0-meta release via the gitlab_release
// meta-tool.
func metaReleaseDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[releases.Output](ctx, "gitlab_release", "delete", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta release delete")
	requireTrue(t, out.TagName == testMetaTag, fmtExpectedTag, out.TagName)
	t.Logf("Deleted release %s", out.TagName)
}

// ---------------------------------------------------------------------------
// Merge request meta-tool
// ---------------------------------------------------------------------------.

// metaCreateMR creates a merge request from the feature branch to main via
// the gitlab_merge_request meta-tool and stores its IID in mState.
func metaCreateMR(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "create", map[string]any{
		"project_id":    mPID(),
		"source_branch": testMetaBranch,
		"target_branch": defaultBranch,
		"title":         "feat: add multiply function [meta E2E]",
		"description":   "MR created via meta-tool E2E.",
	})
	requireNoError(t, err, "meta create MR")
	requireTrue(t, out.IID > 0, "MR IID should be positive")
	requireTrue(t, out.State == "opened", "expected state opened, got %q", out.State)

	mState.mrIID = out.IID
	t.Logf("Created MR !%d via meta-tool", out.IID)
}

// metaGetMR retrieves the merge request by IID via the
// gitlab_merge_request meta-tool and verifies its IID matches.
func metaGetMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "get", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta get MR")
	requireTrue(t, out.IID == mState.mrIID, "expected MR IID %d, got %d", mState.mrIID, out.IID)
	t.Logf("Got MR !%d state=%s", out.IID, out.State)
}

// metaListMRs lists open merge requests via the gitlab_merge_request
// meta-tool and verifies the test MR appears in the result.
func metaListMRs(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[mergerequests.ListOutput](ctx, "gitlab_merge_request", "list", map[string]any{
		"project_id": mPID(),
		"state":      "opened",
	})
	requireNoError(t, err, "meta list MRs")
	requireTrue(t, len(out.MergeRequests) >= 1, "expected at least 1 MR")

	found := false
	for _, mr := range out.MergeRequests {
		if mr.IID == mState.mrIID {
			found = true
			break
		}
	}
	requireTrue(t, found, "MR !%d not in list", mState.mrIID)
	t.Logf("Listed %d MRs via meta-tool", len(out.MergeRequests))
}

// metaUpdateMR modifies the merge request title and description via the
// gitlab_merge_request meta-tool.
func metaUpdateMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "update", map[string]any{
		"project_id":  mPID(),
		"mr_iid":      mState.mrIID,
		"title":       "feat: add multiply function [meta E2E] (updated)",
		"description": "Updated via meta-tool E2E.",
	})
	requireNoError(t, err, "meta update MR")
	requireTrue(t, out.IID == mState.mrIID, "expected MR IID %d, got %d", mState.mrIID, out.IID)
	t.Logf("Updated MR !%d via meta-tool", out.IID)
}

// metaApproveMR approves the merge request via the gitlab_merge_request
// meta-tool.
func metaApproveMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.ApproveOutput](ctx, "gitlab_merge_request", "approve", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta approve MR")
	t.Logf("Approved MR !%d (approved=%v)", mState.mrIID, out.Approved)
}

// metaUnapproveMR revokes the approval from the merge request via the
// gitlab_merge_request meta-tool.
func metaUnapproveMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "unapprove", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta unapprove MR")
	t.Logf("Unapproved MR !%d", mState.mrIID)
}

// metaMergeMR merges the merge request with source branch removal via the
// gitlab_merge_request meta-tool and verifies the state is "merged".
func metaMergeMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.Output](ctx, "gitlab_merge_request", "merge", map[string]any{
		"project_id":                  mPID(),
		"mr_iid":                      mState.mrIID,
		"should_remove_source_branch": true,
	})
	requireNoError(t, err, "meta merge MR")
	requireTrue(t, out.State == "merged", "expected state merged, got %q", out.State)
	t.Logf("Merged MR !%d via meta-tool", mState.mrIID)
}

// ---------------------------------------------------------------------------
// MR review meta-tool
// ---------------------------------------------------------------------------.

// metaNoteCreate creates a general comment on the merge request via the
// gitlab_mr_review meta-tool and stores the note ID in mState.
func metaNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrnotes.Output](ctx, "gitlab_mr_review", "note_create", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"body":       "**Meta-tool E2E Bot**: This MR looks great!",
	})
	requireNoError(t, err, "meta note create")
	requireTrue(t, out.ID > 0, "note ID should be positive")

	mState.noteID = out.ID
	t.Logf("Created note ID=%d via meta-tool", out.ID)
}

// metaNoteList lists notes on the merge request via the gitlab_mr_review
// meta-tool and verifies the created note appears.
func metaNoteList(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrnotes.ListOutput](ctx, "gitlab_mr_review", "note_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta note list")

	found := false
	for _, n := range out.Notes {
		if n.ID == mState.noteID {
			found = true
			break
		}
	}
	requireTrue(t, found, "note ID=%d not in list", mState.noteID)
	t.Logf("Listed %d notes via meta-tool", len(out.Notes))
}

// metaNoteUpdate modifies the note body via the gitlab_mr_review meta-tool.
func metaNoteUpdate(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.noteID > 0, "note ID not set")

	out, err := callMeta[mrnotes.Output](ctx, "gitlab_mr_review", "note_update", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note_id":    mState.noteID,
		"body":       "**Meta-tool E2E Bot** (updated): LGTM!",
	})
	requireNoError(t, err, "meta note update")
	requireTrue(t, out.ID == mState.noteID, "expected note ID %d, got %d", mState.noteID, out.ID)
	t.Logf("Updated note ID=%d via meta-tool", out.ID)
}

// metaNoteDelete deletes the note from the merge request via the
// gitlab_mr_review meta-tool.
func metaNoteDelete(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.noteID > 0, "note ID not set")

	err := callMetaVoid(ctx, "gitlab_mr_review", "note_delete", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"note_id":    mState.noteID,
	})
	requireNoError(t, err, "meta note delete")
	t.Logf("Deleted note ID=%d via meta-tool", mState.noteID)
	mState.noteID = 0
}

// metaChangesGet retrieves the diff changes for the merge request via the
// gitlab_mr_review meta-tool and verifies at least one file changed.
func metaChangesGet(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrchanges.Output](ctx, "gitlab_mr_review", "changes_get", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta changes get")
	requireTrue(t, len(out.Changes) > 0, "expected at least 1 changed file")
	t.Logf("Got %d changed files via meta-tool", len(out.Changes))
}

// metaDiscussionCreate creates a code review discussion on a specific line
// of main.go via the gitlab_mr_review meta-tool using diff position metadata.
func metaDiscussionCreate(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)

	// Get diff SHAs via GitLab client (needed for inline positioning).
	versions, _, err := state.glClient.GL().MergeRequests.GetMergeRequestDiffVersions(
		int(mState.projectID), mState.mrIID, nil,
	)
	requireNoError(t, err, "get MR diff versions (meta)")
	requireTrue(t, len(versions) > 0, "expected at least 1 diff version")

	v := versions[0]
	out, err := callMeta[mrdiscussions.Output](ctx, "gitlab_mr_review", "discussion_create", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"body":       "**Meta Code Review**: Consider edge cases for multiply.",
		"position": map[string]any{
			"base_sha":  v.BaseCommitSHA,
			"start_sha": v.StartCommitSHA,
			"head_sha":  v.HeadCommitSHA,
			"new_path":  testFileMainGo,
			"new_line":  15,
		},
	})
	requireNoError(t, err, "meta discussion create")
	requireTrue(t, out.ID != "", "discussion ID should not be empty")

	mState.discussionID = out.ID
	t.Logf("Created inline discussion %s via meta-tool", out.ID)
}

// metaDiscussionReply adds a reply to the inline discussion via the
// gitlab_mr_review meta-tool.
func metaDiscussionReply(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.discussionID != "", "discussion ID not set")

	out, err := callMeta[mrdiscussions.NoteOutput](ctx, "gitlab_mr_review", "discussion_reply", map[string]any{
		"project_id":    mPID(),
		"mr_iid":        mState.mrIID,
		"discussion_id": mState.discussionID,
		"body":          "Acknowledged! Will address in follow-up.",
	})
	requireNoError(t, err, "meta discussion reply")
	requireTrue(t, out.ID > 0, "reply note ID should be positive")
	t.Logf("Replied to discussion %s via meta-tool", mState.discussionID)
}

// metaDiscussionResolve marks the inline discussion as resolved via the
// gitlab_mr_review meta-tool.
func metaDiscussionResolve(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	requireTrue(t, mState.discussionID != "", "discussion ID not set")

	out, err := callMeta[mrdiscussions.Output](ctx, "gitlab_mr_review", "discussion_resolve", map[string]any{
		"project_id":    mPID(),
		"mr_iid":        mState.mrIID,
		"discussion_id": mState.discussionID,
		"resolved":      true,
	})
	requireNoError(t, err, "meta discussion resolve")
	requireTrue(t, out.ID == mState.discussionID, "expected discussion %s, got %s", mState.discussionID, out.ID)
	t.Logf("Resolved discussion %s via meta-tool", out.ID)
}

// metaDiscussionList lists discussions on the merge request via the
// gitlab_mr_review meta-tool and verifies at least one exists.
func metaDiscussionList(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mrdiscussions.ListOutput](ctx, "gitlab_mr_review", "discussion_list", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta discussion list")
	requireTrue(t, len(out.Discussions) >= 1, "expected at least 1 discussion")
	t.Logf("Listed %d discussions via meta-tool", len(out.Discussions))
}

// ---------------------------------------------------------------------------
// User meta-tool
// ---------------------------------------------------------------------------.

// metaUserCurrent retrieves the authenticated user via the gitlab_user
// meta-tool and verifies basic fields.
func metaUserCurrent(ctx context.Context, t *testing.T) {
	out, err := callMeta[users.Output](ctx, "gitlab_user", "current", map[string]any{})
	requireNoError(t, err, "meta user current")
	requireTrue(t, out.ID > 0, "user ID should be positive, got %d", out.ID)
	requireTrue(t, out.Username != "", "username should not be empty")
	requireTrue(t, out.State == "active", "expected state 'active', got %q", out.State)
	t.Logf("Current user via meta-tool: %s (ID=%d)", out.Username, out.ID)
}

// ---------------------------------------------------------------------------
// Additional commit inspection via gitlab_repository meta-tool
// ---------------------------------------------------------------------------.

// metaCommitList lists commits on the default branch and saves the latest SHA.
func metaCommitList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[commits.ListOutput](ctx, "gitlab_repository", "commit_list", map[string]any{
		"project_id": mPID(),
		"ref_name":   defaultBranch,
	})
	requireNoError(t, err, "meta commit list")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit, got %d", len(out.Commits))
	mState.lastCommitSHA = out.Commits[0].ID
	t.Logf("Listed %d commits via meta-tool (latest=%s)", len(out.Commits), out.Commits[0].ShortID)
}

// metaCommitGet retrieves a specific commit by SHA.
func metaCommitGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.lastCommitSHA != "", "lastCommitSHA not set — MetaCommitList must run first")

	out, err := callMeta[commits.DetailOutput](ctx, "gitlab_repository", "commit_get", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	requireNoError(t, err, "meta commit get")
	requireTrue(t, out.ID == mState.lastCommitSHA, "expected SHA %s, got %s", mState.lastCommitSHA, out.ID)
	requireTrue(t, out.Title != "", "commit title should not be empty")
	t.Logf("Got commit %s via meta-tool: %s", out.ShortID, out.Title)
}

// metaCommitDiff retrieves the diff for the latest commit.
func metaCommitDiff(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.lastCommitSHA != "", "lastCommitSHA not set")

	out, err := callMeta[commits.DiffOutput](ctx, "gitlab_repository", "commit_diff", map[string]any{
		"project_id": mPID(),
		"sha":        mState.lastCommitSHA,
	})
	requireNoError(t, err, "meta commit diff")
	requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff, got %d", len(out.Diffs))
	t.Logf("Commit %s has %d file diffs via meta-tool", mState.lastCommitSHA[:8], len(out.Diffs))
}

// metaRepositoryTree lists the repository tree and verifies main.go is present.
func metaRepositoryTree(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[repository.TreeOutput](ctx, "gitlab_repository", "tree", map[string]any{
		"project_id": mPID(),
		"ref":        defaultBranch,
	})
	requireNoError(t, err, "meta repository tree")
	requireTrue(t, len(out.Tree) >= 1, "expected at least 1 tree node, got %d", len(out.Tree))

	found := false
	for _, node := range out.Tree {
		if node.Name == testFileMainGo {
			found = true
			break
		}
	}
	requireTrue(t, found, "main.go not found in tree listing")
	t.Logf("Repository tree has %d entries via meta-tool", len(out.Tree))
}

// metaBranchGet retrieves a specific branch via the gitlab_branch meta-tool.
func metaBranchGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[branches.Output](ctx, "gitlab_branch", "get", map[string]any{
		"project_id":  mPID(),
		"branch_name": testMetaBranch,
	})
	requireNoError(t, err, "meta branch get")
	requireTrue(t, out.Name == testMetaBranch, "expected branch %q, got %q", testMetaBranch, out.Name)
	requireTrue(t, out.CommitID != "", msgCommitIDEmpty)
	t.Logf("Got branch %s via meta-tool (commit=%s)", out.Name, out.CommitID[:8])
}

// metaRepositoryCompare compares the default branch and the feature branch.
func metaRepositoryCompare(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[repository.CompareOutput](ctx, "gitlab_repository", "compare", map[string]any{
		"project_id": mPID(),
		"from":       defaultBranch,
		"to":         testMetaBranch,
	})
	requireNoError(t, err, "meta repository compare")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit in comparison, got %d", len(out.Commits))
	requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff in comparison, got %d", len(out.Diffs))
	t.Logf("Compared %s..%s via meta-tool: %d commits, %d diffs", defaultBranch, testMetaBranch, len(out.Commits), len(out.Diffs))
}

// metaTagGet retrieves a specific tag via the gitlab_tag meta-tool.
func metaTagGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[tags.Output](ctx, "gitlab_tag", "get", map[string]any{
		"project_id": mPID(),
		"tag_name":   testMetaTag,
	})
	requireNoError(t, err, "meta tag get")
	requireTrue(t, out.Name == testMetaTag, fmtExpectedTag, out.Name)
	requireTrue(t, out.Target != "", "tag target should not be empty")
	t.Logf("Got tag %s via meta-tool (target=%s)", out.Name, out.Target[:8])
}

// ---------------------------------------------------------------------------
// Issue lifecycle via gitlab_issue meta-tool
// ---------------------------------------------------------------------------.

// metaIssueCreate creates a test issue via the gitlab_issue meta-tool.
func metaIssueCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "create", map[string]any{
		"project_id":  mPID(),
		"title":       "Meta E2E test issue — automated lifecycle",
		"description": "Issue created via gitlab_issue meta-tool E2E.",
	})
	requireNoError(t, err, "meta issue create")
	requireTrue(t, out.IID > 0, "issue IID should be positive, got %d", out.IID)
	requireTrue(t, out.State == "opened", "expected state 'opened', got %q", out.State)

	mState.issueIID = out.IID
	t.Logf("Created issue #%d via meta-tool: %s", out.IID, out.Title)
}

// metaIssueGet retrieves the issue by IID.
func metaIssueGet(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, "issueIID not set — MetaIssueCreate must run first")

	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "get", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue get")
	requireTrue(t, out.IID == mState.issueIID, "expected issue IID %d, got %d", mState.issueIID, out.IID)
	t.Logf("Got issue #%d via meta-tool: %s", out.IID, out.Title)
}

// metaIssueList lists issues and verifies the test issue appears.
func metaIssueList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[issues.ListOutput](ctx, "gitlab_issue", "list", map[string]any{
		"project_id": mPID(),
		"state":      "opened",
	})
	requireNoError(t, err, "meta issue list")
	requireTrue(t, len(out.Issues) >= 1, "expected at least 1 issue, got %d", len(out.Issues))

	found := false
	for _, i := range out.Issues {
		if i.IID == mState.issueIID {
			found = true
			break
		}
	}
	requireTrue(t, found, "issue #%d not in meta list", mState.issueIID)
	t.Logf("Listed %d open issues via meta-tool", len(out.Issues))
}

// metaIssueUpdate modifies the issue title.
func metaIssueUpdate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	out, err := callMeta[issues.Output](ctx, "gitlab_issue", "update", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
		"title":      "Meta E2E test issue — updated title",
	})
	requireNoError(t, err, "meta issue update")
	requireTrue(t, out.IID == mState.issueIID, "expected issue IID %d, got %d", mState.issueIID, out.IID)
	t.Logf("Updated issue #%d via meta-tool", out.IID)
}

// metaIssueNoteCreate adds a comment to the test issue via the gitlab_issue meta-tool.
func metaIssueNoteCreate(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	out, err := callMeta[issuenotes.Output](ctx, "gitlab_issue", "note_create", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
		"body":       "**Meta E2E Bot**: Automated comment via meta-tool.",
	})
	requireNoError(t, err, "meta issue note create")
	requireTrue(t, out.ID > 0, "note ID should be positive, got %d", out.ID)

	mState.issueNoteID = out.ID
	t.Logf("Created note %d on issue #%d via meta-tool", out.ID, mState.issueIID)
}

// metaIssueNoteList lists notes on the test issue.
func metaIssueNoteList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	out, err := callMeta[issuenotes.ListOutput](ctx, "gitlab_issue", "note_list", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue note list")
	requireTrue(t, len(out.Notes) >= 1, "expected at least 1 note, got %d", len(out.Notes))

	found := false
	for _, n := range out.Notes {
		if n.ID == mState.issueNoteID {
			found = true
			break
		}
	}
	requireTrue(t, found, "note %d not found in list", mState.issueNoteID)
	t.Logf("Listed %d notes on issue #%d via meta-tool", len(out.Notes), mState.issueIID)
}

// metaIssueDelete deletes the test issue.
func metaIssueDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.issueIID > 0, msgMetaIssueIIDNotSet)

	err := callMetaVoid(ctx, "gitlab_issue", "delete", map[string]any{
		"project_id": mPID(),
		"issue_iid":  mState.issueIID,
	})
	requireNoError(t, err, "meta issue delete")
	t.Logf("Deleted issue #%d via meta-tool", mState.issueIID)
	mState.issueIID = 0
}

// ---------------------------------------------------------------------------
// Additional project actions via gitlab_project meta-tool
// ---------------------------------------------------------------------------.

// metaLabelList lists project labels (may be empty for a fresh project).
func metaLabelList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[labels.ListOutput](ctx, "gitlab_project", "label_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta label list")
	t.Log("Label list via meta-tool succeeded")
}

// metaMilestoneList lists project milestones (may be empty for a fresh project).
func metaMilestoneList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[milestones.ListOutput](ctx, "gitlab_project", "milestone_list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta milestone list")
	t.Log("Milestone list via meta-tool succeeded")
}

// metaProjectMembersList lists project members and verifies at least 1 exists.
func metaProjectMembersList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[members.ListOutput](ctx, "gitlab_project", "members", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta project members list")
	requireTrue(t, len(out.Members) >= 1, "expected at least 1 member, got %d", len(out.Members))
	t.Logf("Listed %d project members via meta-tool", len(out.Members))
}

// metaProjectUpload uploads a small text file to the project.
func metaProjectUpload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	content := base64.StdEncoding.EncodeToString([]byte("meta e2e upload test"))
	out, err := callMeta[uploads.UploadOutput](ctx, "gitlab_project", "upload", map[string]any{
		"project_id":     mPID(),
		"filename":       "meta-e2e-test.txt",
		"content_base64": content,
	})
	requireNoError(t, err, "meta project upload")
	requireTrue(t, out.URL != "", "upload URL should not be empty")
	requireTrue(t, out.Markdown != "", "upload markdown should not be empty")
	t.Logf("Uploaded file via meta-tool: %s", out.URL)
}

// ---------------------------------------------------------------------------
// Additional MR actions via gitlab_merge_request meta-tool
// ---------------------------------------------------------------------------.

// metaMRCommits lists commits in the merge request.
func metaMRCommits(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	out, err := callMeta[mergerequests.CommitsOutput](ctx, "gitlab_merge_request", "commits", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta MR commits")
	requireTrue(t, len(out.Commits) >= 1, "expected at least 1 MR commit, got %d", len(out.Commits))
	t.Logf("MR !%d has %d commits via meta-tool", mState.mrIID, len(out.Commits))
}

// metaMRPipelines lists pipelines for the merge request (may be empty if no CI config).
func metaMRPipelines(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	_, err := callMeta[mergerequests.PipelinesOutput](ctx, "gitlab_merge_request", "pipelines", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
	})
	requireNoError(t, err, "meta MR pipelines")
	t.Logf("MR pipelines listed via meta-tool")
}

// metaRebaseMR rebases the merge request with skip_ci=true.
func metaRebaseMR(ctx context.Context, t *testing.T) {
	requireMetaMRIID(t)
	err := callMetaVoid(ctx, "gitlab_merge_request", "rebase", map[string]any{
		"project_id": mPID(),
		"mr_iid":     mState.mrIID,
		"skip_ci":    true,
	})
	requireNoError(t, err, "meta MR rebase")
	t.Logf("Rebased MR !%d via meta-tool", mState.mrIID)
}

// ---------------------------------------------------------------------------
// Search via gitlab_search meta-tool
// ---------------------------------------------------------------------------.

// metaSearchCode searches for code containing "add" in the project.
func metaSearchCode(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[search.CodeOutput](ctx, "gitlab_search", "code", map[string]any{
		"project_id": mPID(),
		"query":      "add",
	})
	// Search indexing may lag, so just verify the call doesn't error.
	requireNoError(t, err, "meta search code")
	t.Log("Search code via meta-tool succeeded")
}

// metaSearchMergeRequests searches for merge requests.
func metaSearchMergeRequests(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	_, err := callMeta[search.MergeRequestsOutput](ctx, "gitlab_search", "merge_requests", map[string]any{
		"project_id": mPID(),
		"query":      "multiply",
	})
	requireNoError(t, err, "meta search merge requests")
	t.Log("Search merge requests via meta-tool succeeded")
}

// Group tools (read-only, via gitlab_group meta-tool).

// metaGroupList lists accessible groups and stores the first group ID for
// subsequent tests. Skips dependent tests if no groups exist.
func metaGroupList(ctx context.Context, t *testing.T) {
	out, err := callMeta[groups.ListOutput](ctx, "gitlab_group", "list", map[string]any{})
	requireNoError(t, err, "meta group list")
	t.Logf("Found %d groups via meta-tool", len(out.Groups))
	if len(out.Groups) > 0 {
		mState.groupID = out.Groups[0].ID
		mState.groupPath = out.Groups[0].FullPath
		t.Logf("Using group %d (%s) for subsequent tests", mState.groupID, out.Groups[0].FullPath)
	}
}

// metaGroupGet retrieves group details. Skips if no group was discovered.
func metaGroupGet(ctx context.Context, t *testing.T) {
	if mState.groupID == 0 {
		t.Skip("no groups available — skipping meta group get")
	}
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[groups.Output](ctx, "gitlab_group", "get", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta group get")
	requireTrue(t, out.ID == mState.groupID, "expected group ID %d, got %d", mState.groupID, out.ID)
	t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
}

// metaGroupMembersList lists members of the discovered group. Skips if none available.
func metaGroupMembersList(ctx context.Context, t *testing.T) {
	if mState.groupID == 0 {
		t.Skip("no groups available — skipping meta group members list")
	}
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[groups.MemberListOutput](ctx, "gitlab_group", "members", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta group members list")
	t.Logf("Group %d has %d members", mState.groupID, len(out.Members))
}

// metaSubgroupsList lists subgroups of the discovered group. May return empty.
func metaSubgroupsList(ctx context.Context, t *testing.T) {
	if mState.groupID == 0 {
		t.Skip("no groups available — skipping meta subgroups list")
	}
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[groups.ListOutput](ctx, "gitlab_group", "subgroups", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta subgroups list")
	t.Logf("Group %d has %d subgroups", mState.groupID, len(out.Groups))
}

// metaGroupIssues lists issues across all projects in the discovered group.
func metaGroupIssues(ctx context.Context, t *testing.T) {
	if mState.groupID == 0 {
		t.Skip("no groups available — skipping meta group issues")
	}
	gid := strconv.FormatInt(mState.groupID, 10)
	out, err := callMeta[issues.ListGroupOutput](ctx, "gitlab_group", "issues", map[string]any{
		"group_id": gid,
	})
	requireNoError(t, err, "meta group issues")
	t.Logf("Group %d has %d issues", mState.groupID, len(out.Issues))
}

// Pipeline list (read-only, via gitlab_pipeline meta-tool).

// metaPipelineList lists pipelines on the meta-tool test project. May return
// empty if no CI configuration exists.
func metaPipelineList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	out, err := callMeta[pipelines.ListOutput](ctx, "gitlab_pipeline", "list", map[string]any{
		"project_id": mPID(),
	})
	requireNoError(t, err, "meta pipeline list")
	t.Logf("Project has %d pipelines via meta-tool", len(out.Pipelines))
}

// Package lifecycle (via gitlab_package meta-tool).

const (
	metaPackageName    = "e2e-meta-pkg"
	metaPackageVersion = "1.0.0"
	metaPackageFile    = "meta-hello.txt"
)

// metaPackagePublish publishes a small file to the Generic Package Registry
// using the gitlab_package meta-tool and stores the package/file IDs.
func metaPackagePublish(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	content := base64.StdEncoding.EncodeToString([]byte("E2E meta package content"))
	out, err := callMeta[packages.PublishOutput](ctx, "gitlab_package", "publish", map[string]any{
		"project_id":      mPID(),
		"package_name":    metaPackageName,
		"package_version": metaPackageVersion,
		"file_name":       metaPackageFile,
		"content_base64":  content,
	})
	requireNoError(t, err, "meta publish package file")
	requireTrue(t, out.PackageID > 0, "package ID should be positive, got %d", out.PackageID)
	requireTrue(t, out.PackageFileID > 0, "package file ID should be positive, got %d", out.PackageFileID)
	mState.packageID = out.PackageID
	mState.packageFileID = out.PackageFileID
	t.Logf("Meta published package ID=%d file_id=%d (%s)", out.PackageID, out.PackageFileID, out.FileName)
}

// metaPackageList lists packages via the gitlab_package meta-tool and verifies
// the published package appears.
func metaPackageList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, "package ID not set — metaPackagePublish must run first")
	out, err := callMeta[packages.ListOutput](ctx, "gitlab_package", "list", map[string]any{
		"project_id":   mPID(),
		"package_name": metaPackageName,
	})
	requireNoError(t, err, "meta list packages")
	found := false
	for _, p := range out.Packages {
		if p.ID == mState.packageID {
			found = true
			break
		}
	}
	requireTrue(t, found, "package %d not found in meta list", mState.packageID)
	t.Logf("Meta listed %d packages, found ID=%d", len(out.Packages), mState.packageID)
}

// metaPackageFileList lists files within the published package via the
// gitlab_package meta-tool.
func metaPackageFileList(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, msgPackageIDNotSet)
	out, err := callMeta[packages.FileListOutput](ctx, "gitlab_package", "file_list", map[string]any{
		"project_id": mPID(),
		"package_id": mState.packageID,
	})
	requireNoError(t, err, "meta list package files")
	requireTrue(t, len(out.Files) >= 1, "expected at least 1 file, got %d", len(out.Files))
	found := false
	for _, f := range out.Files {
		if f.FileName == metaPackageFile {
			found = true
			break
		}
	}
	requireTrue(t, found, "file %q not found in meta package", metaPackageFile)
	t.Logf("Meta package %d has %d files", mState.packageID, len(out.Files))
}

// metaPackageDownload downloads the published package file via the
// gitlab_package meta-tool and verifies the content.
func metaPackageDownload(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, msgPackageIDNotSet)
	outputPath := filepath.Join(t.TempDir(), metaPackageFile)
	out, err := callMeta[packages.DownloadOutput](ctx, "gitlab_package", "download", map[string]any{
		"project_id":      mPID(),
		"package_name":    metaPackageName,
		"package_version": metaPackageVersion,
		"file_name":       metaPackageFile,
		"output_path":     outputPath,
	})
	requireNoError(t, err, "meta download package file")
	requireTrue(t, out.Size > 0, "downloaded file size should be positive, got %d", out.Size)
	requireTrue(t, out.SHA256 != "", "SHA256 should not be empty")

	data, err := os.ReadFile(outputPath)
	requireNoError(t, err, "read meta downloaded file")
	requireTrue(t, string(data) == "E2E meta package content", "expected original content, got %q", string(data))
	t.Logf("Meta downloaded %s (%d bytes, sha256=%s)", outputPath, out.Size, out.SHA256)
}

// metaPackageFileDelete deletes the file from the package via meta-tool.
func metaPackageFileDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageFileID > 0, "package file ID not set")
	err := callMetaVoid(ctx, "gitlab_package", "file_delete", map[string]any{
		"project_id":      mPID(),
		"package_id":      mState.packageID,
		"package_file_id": mState.packageFileID,
	})
	requireNoError(t, err, "meta delete package file")
	t.Logf("Meta deleted package file ID=%d from package %d", mState.packageFileID, mState.packageID)
	mState.packageFileID = 0
}

// metaPackageDelete deletes the package from the registry via meta-tool.
func metaPackageDelete(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	requireTrue(t, mState.packageID > 0, msgPackageIDNotSet)
	err := callMetaVoid(ctx, "gitlab_package", "delete", map[string]any{
		"project_id": mPID(),
		"package_id": mState.packageID,
	})
	requireNoError(t, err, "meta delete package")
	t.Logf("Meta deleted package ID=%d", mState.packageID)
	mState.packageID = 0
}

// Upload with file_path (via gitlab_project meta-tool).

// metaUploadFilePath uploads a file using a local file_path instead of base64
// through the gitlab_project meta-tool.
func metaUploadFilePath(ctx context.Context, t *testing.T) {
	requireMetaProjectID(t)
	tmpFile := filepath.Join(t.TempDir(), "e2e-meta-filepath-upload.txt")
	err := os.WriteFile(tmpFile, []byte("E2E meta file_path upload content"), 0o644)
	requireNoError(t, err, "create temp file for meta upload")

	out, err := callMeta[uploads.UploadOutput](ctx, "gitlab_project", "upload", map[string]any{
		"project_id": mPID(),
		"filename":   "e2e-meta-filepath-upload.txt",
		"file_path":  tmpFile,
	})
	requireNoError(t, err, "meta upload file via file_path")
	requireTrue(t, out.URL != "", "upload URL should not be empty")
	requireTrue(t, out.Markdown != "", "upload markdown should not be empty")
	t.Logf("Meta uploaded via file_path: %s (markdown=%s)", out.URL, out.Markdown)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// waitForBranchMeta polls until the branch exists (using the shared GitLab client).
func waitForBranchMeta(ctx context.Context, t *testing.T, branch string) {
	t.Helper()
	pid := int(mState.projectID)
	for range 15 {
		_, resp, err := state.glClient.GL().Branches.GetBranch(pid, branch)
		if err == nil {
			t.Logf("Branch %q ready (meta)", branch)
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
	t.Fatalf("branch %q not available after 15s (meta)", branch)
}
