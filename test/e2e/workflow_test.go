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
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuediscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelineschedules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
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
	ctx, cancel := context.WithTimeout(context.Background(), 580*time.Second)
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

	// Group tools (create a group for deterministic testing).
	t.Run("38c_GroupCreate", func(t *testing.T) { testGroupCreate(ctx, t) })
	t.Run("38d_GroupList", func(t *testing.T) { testGroupList(ctx, t) })
	t.Run("38e_GroupGet", func(t *testing.T) { testGroupGet(ctx, t) })
	t.Run("38f_GroupMembersList", func(t *testing.T) { testGroupMembersList(ctx, t) })
	t.Run("38g_SubgroupsList", func(t *testing.T) { testSubgroupsList(ctx, t) })

	// Pipeline list (read-only, may return empty without CI config).
	t.Run("38h_PipelineList", func(t *testing.T) { testPipelineList(ctx, t) })

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

	// Push rules (Enterprise/Premium only).
	if state.enterprise {
		t.Run("41_AddPushRule", func(t *testing.T) { testAddPushRule(ctx, t) })
		t.Run("42_GetPushRules", func(t *testing.T) { testGetPushRules(ctx, t) })
		t.Run("43_EditPushRule", func(t *testing.T) { testEditPushRule(ctx, t) })
		t.Run("44_DeletePushRule", func(t *testing.T) { testDeletePushRule(ctx, t) })
	}

	// User-scoped project listings (require GITLAB_USER).
	if os.Getenv("GITLAB_USER") != "" {
		t.Run("45_ListUserContributed", func(t *testing.T) { testListUserContributed(ctx, t) })
		t.Run("46_ListUserStarred", func(t *testing.T) { testListUserStarred(ctx, t) })
	}

	// GraphQL tools (branch rules, CI catalog, custom emoji — CE; vulnerabilities — Enterprise).
	t.Run("47_ListBranchRules", func(t *testing.T) { testListBranchRules(ctx, t) })
	t.Run("48_ListCatalogResources", func(t *testing.T) { testListCatalogResources(ctx, t) })
	if state.enterprise {
		t.Run("49_VulnerabilitySeverityCount", func(t *testing.T) { testVulnerabilitySeverityCount(ctx, t) })
		t.Run("50_ListVulnerabilities", func(t *testing.T) { testListVulnerabilities(ctx, t) })
	}
	if state.groupPath != "" {
		t.Run("51_ListCustomEmoji", func(t *testing.T) { testListCustomEmoji(ctx, t) })
	}

	// Group cleanup (delete the group created in 38c).
	t.Run("51a_GroupDelete", func(t *testing.T) { testGroupDelete(ctx, t) })

	// --- Phase 3: New coverage domains ---

	// Wiki lifecycle.
	t.Run("52_WikiCreate", func(t *testing.T) { testWikiCreate(ctx, t) })
	t.Run("53_WikiGet", func(t *testing.T) { testWikiGet(ctx, t) })
	t.Run("54_WikiList", func(t *testing.T) { testWikiList(ctx, t) })
	t.Run("55_WikiUpdate", func(t *testing.T) { testWikiUpdate(ctx, t) })
	t.Run("56_WikiDelete", func(t *testing.T) { testWikiDelete(ctx, t) })

	// CI variables lifecycle.
	t.Run("57_CIVariableCreate", func(t *testing.T) { testCIVariableCreate(ctx, t) })
	t.Run("58_CIVariableGet", func(t *testing.T) { testCIVariableGet(ctx, t) })
	t.Run("59_CIVariableList", func(t *testing.T) { testCIVariableList(ctx, t) })
	t.Run("60_CIVariableUpdate", func(t *testing.T) { testCIVariableUpdate(ctx, t) })
	t.Run("61_CIVariableDelete", func(t *testing.T) { testCIVariableDelete(ctx, t) })

	// CI lint.
	t.Run("62_CILint", func(t *testing.T) { testCILint(ctx, t) })

	// Environment lifecycle.
	t.Run("63_EnvironmentCreate", func(t *testing.T) { testEnvironmentCreate(ctx, t) })
	t.Run("64_EnvironmentGet", func(t *testing.T) { testEnvironmentGet(ctx, t) })
	t.Run("65_EnvironmentList", func(t *testing.T) { testEnvironmentList(ctx, t) })
	t.Run("66_EnvironmentStop", func(t *testing.T) { testEnvironmentStop(ctx, t) })
	t.Run("67_EnvironmentDelete", func(t *testing.T) { testEnvironmentDelete(ctx, t) })

	// Label lifecycle (create, update, delete — list already tested at 22h).
	t.Run("68_LabelCreate", func(t *testing.T) { testLabelCreate(ctx, t) })
	t.Run("69_LabelUpdate", func(t *testing.T) { testLabelUpdate(ctx, t) })
	t.Run("70_LabelDelete", func(t *testing.T) { testLabelDelete(ctx, t) })

	// Milestone lifecycle (create, get, update, close, delete — list already tested at 22i).
	t.Run("71_MilestoneCreate", func(t *testing.T) { testMilestoneCreate(ctx, t) })
	t.Run("72_MilestoneGet", func(t *testing.T) { testMilestoneGet(ctx, t) })
	t.Run("73_MilestoneUpdate", func(t *testing.T) { testMilestoneUpdate(ctx, t) })
	t.Run("74_MilestoneDelete", func(t *testing.T) { testMilestoneDelete(ctx, t) })

	// Issue links (needs 2 issues).
	t.Run("75_IssueCreateSecond", func(t *testing.T) { testIssueCreateSecond(ctx, t) })
	t.Run("76_IssueLinkCreate", func(t *testing.T) { testIssueLinkCreate(ctx, t) })
	t.Run("77_IssueLinkList", func(t *testing.T) { testIssueLinkList(ctx, t) })
	t.Run("78_IssueLinkDelete", func(t *testing.T) { testIssueLinkDelete(ctx, t) })
	t.Run("79_IssueDeleteSecond", func(t *testing.T) { testIssueDeleteSecond(ctx, t) })

	// Todos (create from issue, list, mark done).
	t.Run("80_TodoCreateFromIssue", func(t *testing.T) { testTodoCreateFromIssue(ctx, t) })
	t.Run("81_TodoList", func(t *testing.T) { testTodoList(ctx, t) })
	t.Run("82_TodoMarkAllDone", func(t *testing.T) { testTodoMarkAllDone(ctx, t) })

	// Deploy Keys (CRUD).
	t.Run("83_DeployKeyCreate", func(t *testing.T) { testDeployKeyCreate(ctx, t) })
	t.Run("84_DeployKeyGet", func(t *testing.T) { testDeployKeyGet(ctx, t) })
	t.Run("85_DeployKeyList", func(t *testing.T) { testDeployKeyList(ctx, t) })
	t.Run("86_DeployKeyDelete", func(t *testing.T) { testDeployKeyDelete(ctx, t) })

	// Snippets (project CRUD).
	t.Run("87_ProjectSnippetCreate", func(t *testing.T) { testProjectSnippetCreate(ctx, t) })
	t.Run("88_ProjectSnippetGet", func(t *testing.T) { testProjectSnippetGet(ctx, t) })
	t.Run("89_ProjectSnippetList", func(t *testing.T) { testProjectSnippetList(ctx, t) })
	t.Run("90_ProjectSnippetUpdate", func(t *testing.T) { testProjectSnippetUpdate(ctx, t) })
	t.Run("91_ProjectSnippetDelete", func(t *testing.T) { testProjectSnippetDelete(ctx, t) })

	// Issue Discussions (create, list, reply, delete note).
	t.Run("92_IssueDiscussionCreate", func(t *testing.T) { testIssueDiscussionCreate(ctx, t) })
	t.Run("93_IssueDiscussionList", func(t *testing.T) { testIssueDiscussionList(ctx, t) })
	t.Run("94_IssueDiscussionAddNote", func(t *testing.T) { testIssueDiscussionAddNote(ctx, t) })
	t.Run("95_IssueDiscussionDeleteNote", func(t *testing.T) { testIssueDiscussionDeleteNote(ctx, t) })

	// MR Draft Notes (CRUD + publish).
	t.Run("96_MRDraftNoteCreate", func(t *testing.T) { testMRDraftNoteCreate(ctx, t) })
	t.Run("97_MRDraftNoteList", func(t *testing.T) { testMRDraftNoteList(ctx, t) })
	t.Run("98_MRDraftNoteUpdate", func(t *testing.T) { testMRDraftNoteUpdate(ctx, t) })
	t.Run("99_MRDraftNotePublishAll", func(t *testing.T) { testMRDraftNotePublishAll(ctx, t) })

	// Pipeline Schedules (CRUD).
	t.Run("100_PipelineScheduleCreate", func(t *testing.T) { testPipelineScheduleCreate(ctx, t) })
	t.Run("101_PipelineScheduleGet", func(t *testing.T) { testPipelineScheduleGet(ctx, t) })
	t.Run("102_PipelineScheduleList", func(t *testing.T) { testPipelineScheduleList(ctx, t) })
	t.Run("103_PipelineScheduleUpdate", func(t *testing.T) { testPipelineScheduleUpdate(ctx, t) })
	t.Run("104_PipelineScheduleDelete", func(t *testing.T) { testPipelineScheduleDelete(ctx, t) })

	// Badges (project CRUD).
	t.Run("105_BadgeCreate", func(t *testing.T) { testBadgeCreate(ctx, t) })
	t.Run("106_BadgeList", func(t *testing.T) { testBadgeList(ctx, t) })
	t.Run("107_BadgeUpdate", func(t *testing.T) { testBadgeUpdate(ctx, t) })
	t.Run("108_BadgeDelete", func(t *testing.T) { testBadgeDelete(ctx, t) })

	// Access Tokens (project: create, list, revoke).
	t.Run("109_AccessTokenCreate", func(t *testing.T) { testAccessTokenCreate(ctx, t) })
	t.Run("110_AccessTokenList", func(t *testing.T) { testAccessTokenList(ctx, t) })
	t.Run("111_AccessTokenRevoke", func(t *testing.T) { testAccessTokenRevoke(ctx, t) })

	// Award Emoji (on issue: create, list, delete).
	t.Run("112_AwardEmojiCreate", func(t *testing.T) { testAwardEmojiCreate(ctx, t) })
	t.Run("113_AwardEmojiList", func(t *testing.T) { testAwardEmojiList(ctx, t) })
	t.Run("114_AwardEmojiDelete", func(t *testing.T) { testAwardEmojiDelete(ctx, t) })

	// Pipeline & Job lifecycle (Docker mode only — requires CI runner).
	if hasRunner() {
		t.Run("200_PipelineCICommit", func(t *testing.T) { testPipelineCICommit(ctx, t) })
		t.Run("201_PipelineCreate", func(t *testing.T) { testPipelineCreate(ctx, t) })
		t.Run("202_PipelineGet", func(t *testing.T) { testPipelineGet(ctx, t) })
		t.Run("203_PipelineListWithCI", func(t *testing.T) { testPipelineListWithCI(ctx, t) })
		t.Run("204_PipelineWaitAndJobList", func(t *testing.T) { testPipelineWaitAndJobList(ctx, t) })
		t.Run("205_JobGet", func(t *testing.T) { testJobGet(ctx, t) })
		t.Run("206_JobTrace", func(t *testing.T) { testJobTrace(ctx, t) })
		t.Run("207_PipelineRetry", func(t *testing.T) { testPipelineRetry(ctx, t) })
		t.Run("207a_SamplingAnalyzePipelineFailure", func(t *testing.T) { testSamplingAnalyzePipelineFailure(ctx, t) })
		t.Run("208_PipelineDelete", func(t *testing.T) { testPipelineDelete(ctx, t) })
	}

	// Sampling tools (require sampling-enabled session — mock LLM handler).
	// Create temporary resources needed by sampling tools.
	t.Run("298_SamplingSetupIssue", func(t *testing.T) { testSamplingSetupIssue(ctx, t) })
	t.Run("299_SamplingSetupMilestone", func(t *testing.T) { testSamplingSetupMilestone(ctx, t) })
	t.Run("300_SamplingAnalyzeMRChanges", func(t *testing.T) { testSamplingAnalyzeMRChanges(ctx, t) })
	t.Run("301_SamplingSummarizeIssue", func(t *testing.T) { testSamplingSummarizeIssue(ctx, t) })
	t.Run("302_SamplingGenerateReleaseNotes", func(t *testing.T) { testSamplingGenerateReleaseNotes(ctx, t) })
	t.Run("303_SamplingSummarizeMRReview", func(t *testing.T) { testSamplingSummarizeMRReview(ctx, t) })
	t.Run("304_SamplingAnalyzeCIConfig", func(t *testing.T) { testSamplingAnalyzeCIConfig(ctx, t) })
	t.Run("305_SamplingAnalyzeIssueScope", func(t *testing.T) { testSamplingAnalyzeIssueScope(ctx, t) })
	t.Run("306_SamplingReviewMRSecurity", func(t *testing.T) { testSamplingReviewMRSecurity(ctx, t) })
	t.Run("307_SamplingFindTechnicalDebt", func(t *testing.T) { testSamplingFindTechnicalDebt(ctx, t) })
	t.Run("308_SamplingAnalyzeDeploymentHistory", func(t *testing.T) { testSamplingAnalyzeDeploymentHistory(ctx, t) })
	t.Run("309_SamplingGenerateMilestoneReport", func(t *testing.T) { testSamplingGenerateMilestoneReport(ctx, t) })

	// Elicitation tools (require elicitation-enabled session — auto-accept mock).
	t.Run("400_ElicitInteractiveIssueCreate", func(t *testing.T) { testElicitInteractiveIssueCreate(ctx, t) })

	// Cleanup.
	t.Run("999_Cleanup_DeleteProject", func(t *testing.T) { testDeleteProject(ctx, t) })
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
// commits can be pushed directly. The MCP tool is idempotent — calling it on
// an already-unprotected branch returns success without error.
func testUnprotectMain(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callTool[branches.UnprotectOutput](ctx, "gitlab_branch_unprotect", branches.UnprotectInput{
		ProjectID:  pidStr(),
		BranchName: defaultBranch,
	})
	requireNoError(t, err, "unprotect main branch")
	t.Logf("Unprotect result: status=%s", out.Status)
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
// commits can be pushed to it. After unprotecting, it verifies the branch is
// no longer in the protected list — GitLab CE may have a brief propagation
// delay between the unprotect API response and the commit authorization check.
func testBranchUnprotectFeature(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	err := callToolVoid(ctx, "gitlab_branch_unprotect", branches.UnprotectInput{
		ProjectID:  pidStr(),
		BranchName: testE2EBranch,
	})
	requireNoError(t, err, "unprotect feature branch")

	// Verify the branch is no longer protected (also serves as propagation delay).
	out, err := callTool[branches.ProtectedListOutput](ctx, "gitlab_protected_branches_list", branches.ProtectedListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "list protected after unprotect")
	for _, b := range out.Branches {
		if b.Name == testE2EBranch {
			t.Fatalf("branch %q still appears in protected list after unprotect", testE2EBranch)
		}
	}
	t.Log("Unprotected feature/e2e-changes branch (verified)")
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
// one commit exists. Retries briefly since GitLab may not expose MR commits
// immediately after creation.
func testMRCommits(ctx context.Context, t *testing.T) {
	requireMRIID(t)
	var out mergerequests.CommitsOutput
	var err error
	for range 3 {
		out, err = callTool[mergerequests.CommitsOutput](ctx, "gitlab_mr_commits", mergerequests.CommitsInput{
			ProjectID: pidStr(),
			MRIID:     state.mrIID,
		})
		requireNoError(t, err, "list MR commits")
		if len(out.Commits) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
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

// Group tools.

// testGroupCreate creates a group for deterministic E2E testing.
func testGroupCreate(ctx context.Context, t *testing.T) {
	out, err := callTool[groups.Output](ctx, "gitlab_group_create", groups.CreateInput{
		Name:       "e2e-test-group",
		Path:       "e2e-test-group",
		Visibility: "public",
	})
	if err != nil {
		// Group may already exist from a previous run; look it up instead of failing.
		listOut, listErr := callTool[groups.ListOutput](ctx, "gitlab_group_list", groups.ListInput{
			Search: "e2e-test-group",
		})
		if listErr == nil {
			for _, g := range listOut.Groups {
				if g.Path == "e2e-test-group" {
					state.groupID = g.ID
					state.groupPath = g.FullPath
					t.Logf("Group already exists, reusing group %d (%s)", g.ID, g.FullPath)
					return
				}
			}
		}
		requireNoError(t, err, "create group")
	}
	requireTrue(t, out.ID > 0, "group ID should be positive, got %d", out.ID)
	state.groupID = out.ID
	state.groupPath = out.FullPath
	t.Logf("Created group %d (%s)", out.ID, out.FullPath)
}

// testGroupList lists groups accessible to the authenticated user.
func testGroupList(ctx context.Context, t *testing.T) {
	requireTrue(t, state.groupID > 0, "group ID not set (group should have been created in testGroupCreate)")
	out, err := callTool[groups.ListOutput](ctx, "gitlab_group_list", groups.ListInput{})
	requireNoError(t, err, "list groups")
	requireTrue(t, len(out.Groups) > 0, "expected at least one group")
	t.Logf("Found %d groups", len(out.Groups))
}

// testGroupGet retrieves group details.
func testGroupGet(ctx context.Context, t *testing.T) {
	requireTrue(t, state.groupID > 0, "group ID not set")
	gid := strconv.FormatInt(state.groupID, 10)
	out, err := callTool[groups.Output](ctx, "gitlab_group_get", groups.GetInput{
		GroupID: toolutil.StringOrInt(gid),
	})
	requireNoError(t, err, "get group")
	requireTrue(t, out.ID == state.groupID, "expected group ID %d, got %d", state.groupID, out.ID)
	t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
}

// testGroupMembersList lists members of the test group.
func testGroupMembersList(ctx context.Context, t *testing.T) {
	requireTrue(t, state.groupID > 0, "group ID not set")
	gid := strconv.FormatInt(state.groupID, 10)
	out, err := callTool[groups.MemberListOutput](ctx, "gitlab_group_members_list", groups.MembersListInput{
		GroupID: toolutil.StringOrInt(gid),
	})
	requireNoError(t, err, "list group members")
	t.Logf("Group %d has %d members", state.groupID, len(out.Members))
}

// testSubgroupsList lists subgroups of the test group. May return empty.
func testSubgroupsList(ctx context.Context, t *testing.T) {
	requireTrue(t, state.groupID > 0, "group ID not set")
	gid := strconv.FormatInt(state.groupID, 10)
	out, err := callTool[groups.ListOutput](ctx, "gitlab_subgroups_list", groups.SubgroupsListInput{
		GroupID: toolutil.StringOrInt(gid),
	})
	requireNoError(t, err, "list subgroups")
	t.Logf("Group %d has %d subgroups", state.groupID, len(out.Groups))
}

// Pipeline list (read-only).

// testGroupDelete deletes the E2E test group, cleaning up after group tests.
func testGroupDelete(ctx context.Context, t *testing.T) {
	if state.groupID == 0 {
		t.Log("no group to delete — skipping cleanup")
		return
	}
	gid := strconv.FormatInt(state.groupID, 10)
	err := callToolVoid(ctx, "gitlab_group_delete", groups.DeleteInput{
		GroupID: toolutil.StringOrInt(gid),
	})
	requireNoError(t, err, "delete group")
	t.Logf("Deleted group %d (%s)", state.groupID, state.groupPath)
	state.groupID = 0
	state.groupPath = ""
}

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
		ShouldRemoveSourceBranch: new(true),
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
// Push rules require GitLab Premium/Ultimate — skipped on CE (404).
func testAddPushRule(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[projects.PushRuleOutput](ctx, "gitlab_project_add_push_rule", projects.AddPushRuleInput{
		ProjectID:          pidStr(),
		CommitMessageRegex: "^[A-Z].*",
		MaxFileSize:        int64Ptr(50),
	})
	if err != nil && strings.Contains(err.Error(), "404") {
		t.Log("push rules require GitLab Premium/Ultimate — 404 expected on CE")
		return
	}
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
	if err != nil && strings.Contains(err.Error(), "404") {
		t.Log("push rules require GitLab Premium/Ultimate — 404 expected on CE")
		return
	}
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
	if err != nil && strings.Contains(err.Error(), "404") {
		t.Log("push rules require GitLab Premium/Ultimate — 404 expected on CE")
		return
	}
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
	if err != nil && strings.Contains(err.Error(), "404") {
		t.Log("push rules require GitLab Premium/Ultimate — 404 expected on CE")
		return
	}
	requireNoError(t, err, "delete push rule")
	t.Logf("Deleted push rules from project %s", state.projectPath)
}

// User-scoped project listings.

// testListUserContributed lists projects the authenticated user has contributed to.
func testListUserContributed(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	out, err := callTool[projects.ListOutput](ctx, "gitlab_project_list_user_contributed", projects.ListUserContributedProjectsInput{
		UserID: toolutil.StringOrInt(user),
	})
	requireNoError(t, err, "list user contributed projects")
	t.Logf("User %s has contributed to %d projects", user, len(out.Projects))
}

// testListUserStarred lists projects the authenticated user has starred.
func testListUserStarred(ctx context.Context, t *testing.T) {
	user := os.Getenv("GITLAB_USER")
	out, err := callTool[projects.ListOutput](ctx, "gitlab_project_list_user_starred", projects.ListUserStarredProjectsInput{
		UserID: toolutil.StringOrInt(user),
	})
	requireNoError(t, err, "list user starred projects")
	t.Logf("User %s has starred %d projects", user, len(out.Projects))
}

// GraphQL tools (branch rules, CI catalog, vulnerabilities, custom emoji).

// testListBranchRules queries GraphQL branch rules for the E2E project.
// GraphQL route resolution may lag under load; retry a few times before failing.
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
	requireNoError(t, err, "vulnerability_severity_count")
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
	requireNoError(t, err, "list vulnerabilities")
	t.Logf("Project %s has %d vulnerabilities", state.projectPath, len(out.Vulnerabilities))
}

// testListCustomEmoji queries custom emoji for the discovered group via
// GraphQL. Skips if no group was found.
func testListCustomEmoji(ctx context.Context, t *testing.T) {
	requireTrue(t, state.groupPath != "", "group path must be set — testGroupCreate should have run first")
	out, err := callTool[customemoji.ListOutput](ctx, "gitlab_list_custom_emoji", customemoji.ListInput{
		GroupPath: state.groupPath,
	})
	requireNoError(t, err, "list custom emoji")
	t.Logf("Group %s has %d custom emoji", state.groupPath, len(out.Emoji))
}

// int64Ptr returns a pointer to the given int64.
func int64Ptr(v int64) *int64 { return &v } //nolint:modernize // used in multiple call sites

// testDeleteProject permanently deletes the E2E test project and verifies
// it is no longer accessible via the GitLab API.
func testDeleteProject(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	// Step 1: Delete — may return "scheduled" or "success" depending on GitLab config.
	out, err := callTool[projects.DeleteOutput](ctx, "gitlab_project_delete", projects.DeleteInput{
		ProjectID:         pidStr(),
		PermanentlyRemove: true,
		FullPath:          state.projectPath,
	})
	requireNoError(t, err, "delete project")
	t.Logf("Delete response: status=%s, permanently_removed=%v", out.Status, out.PermanentlyRemoved)

	// Step 2: Verify the project is gone (GET should return 404).
	pid := int(state.projectID)
	_, resp, getErr := state.glClient.GL().Projects.GetProject(strconv.Itoa(pid), &gl.GetProjectOptions{})
	if getErr == nil || (resp != nil && resp.StatusCode != http.StatusNotFound) {
		t.Fatalf("expected project %d to be deleted (404), but GET returned status %d", pid, resp.StatusCode)
	}
	t.Logf("Verified project %s (ID=%d) is permanently deleted", state.projectPath, state.projectID)
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

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — Wiki
// ---------------------------------------------------------------------------.

func testWikiCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[wikis.Output](ctx, "gitlab_wiki_create", wikis.CreateInput{
		ProjectID: pidStr(),
		Title:     "E2E Test Page",
		Content:   "This is an E2E wiki page.",
	})
	requireNoError(t, err, "wiki create")
	requireTrue(t, out.Slug != "", "wiki slug should not be empty")
	state.wikiSlug = out.Slug
	t.Logf("Created wiki page: %s (slug=%s)", out.Title, out.Slug)
}

func testWikiGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.wikiSlug != "", "wiki slug not set")
	out, err := callTool[wikis.Output](ctx, "gitlab_wiki_get", wikis.GetInput{
		ProjectID: pidStr(),
		Slug:      state.wikiSlug,
	})
	requireNoError(t, err, "wiki get")
	requireTrue(t, out.Slug == state.wikiSlug, "expected slug %q, got %q", state.wikiSlug, out.Slug)
	t.Logf("Got wiki page: %s", out.Title)
}

func testWikiList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[wikis.ListOutput](ctx, "gitlab_wiki_list", wikis.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "wiki list")
	requireTrue(t, len(out.WikiPages) >= 1, "expected at least 1 wiki page, got %d", len(out.WikiPages))
	t.Logf("Listed %d wiki pages", len(out.WikiPages))
}

func testWikiUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.wikiSlug != "", "wiki slug not set")
	out, err := callTool[wikis.Output](ctx, "gitlab_wiki_update", wikis.UpdateInput{
		ProjectID: pidStr(),
		Slug:      state.wikiSlug,
		Content:   "Updated E2E wiki content.",
	})
	requireNoError(t, err, "wiki update")
	t.Logf("Updated wiki page: %s", out.Title)
}

func testWikiDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.wikiSlug != "", "wiki slug not set")
	err := callToolVoid(ctx, "gitlab_wiki_delete", wikis.DeleteInput{
		ProjectID: pidStr(),
		Slug:      state.wikiSlug,
	})
	requireNoError(t, err, "wiki delete")
	state.wikiSlug = ""
	t.Logf("Deleted wiki page")
}

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — CI Variables
// ---------------------------------------------------------------------------.

func testCIVariableCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[civariables.Output](ctx, "gitlab_ci_variable_create", civariables.CreateInput{
		ProjectID: pidStr(),
		Key:       "E2E_TEST_VAR",
		Value:     "test-value-123",
	})
	requireNoError(t, err, "ci variable create")
	requireTrue(t, out.Key == "E2E_TEST_VAR", "expected key E2E_TEST_VAR, got %s", out.Key)
	t.Logf("Created CI variable: %s", out.Key)
}

func testCIVariableGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[civariables.Output](ctx, "gitlab_ci_variable_get", civariables.GetInput{
		ProjectID: pidStr(),
		Key:       "E2E_TEST_VAR",
	})
	requireNoError(t, err, "ci variable get")
	requireTrue(t, out.Value == "test-value-123", "expected value test-value-123, got %s", out.Value)
	t.Logf("Got CI variable: %s=%s", out.Key, out.Value)
}

func testCIVariableList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[civariables.ListOutput](ctx, "gitlab_ci_variable_list", civariables.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "ci variable list")
	requireTrue(t, len(out.Variables) >= 1, "expected at least 1 variable, got %d", len(out.Variables))
	t.Logf("Listed %d CI variables", len(out.Variables))
}

func testCIVariableUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[civariables.Output](ctx, "gitlab_ci_variable_update", civariables.UpdateInput{
		ProjectID: pidStr(),
		Key:       "E2E_TEST_VAR",
		Value:     "updated-value-456",
	})
	requireNoError(t, err, "ci variable update")
	requireTrue(t, out.Value == "updated-value-456", "expected updated value, got %s", out.Value)
	t.Logf("Updated CI variable: %s=%s", out.Key, out.Value)
}

func testCIVariableDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	err := callToolVoid(ctx, "gitlab_ci_variable_delete", civariables.DeleteInput{
		ProjectID: pidStr(),
		Key:       "E2E_TEST_VAR",
	})
	requireNoError(t, err, "ci variable delete")
	t.Logf("Deleted CI variable E2E_TEST_VAR")
}

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — CI Lint
// ---------------------------------------------------------------------------.

func testCILint(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[cilint.Output](ctx, "gitlab_ci_lint", cilint.ContentInput{
		ProjectID: pidStr(),
		Content: `stages:
  - test
hello:
  stage: test
  script:
    - echo "hello"`,
	})
	requireNoError(t, err, "ci lint")
	requireTrue(t, out.Valid, "expected valid CI config, got invalid: %v", out.Errors)
	t.Logf("CI lint result: valid=%v, warnings=%d", out.Valid, len(out.Warnings))
}

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — Environments
// ---------------------------------------------------------------------------.

func testEnvironmentCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[environments.Output](ctx, "gitlab_environment_create", environments.CreateInput{
		ProjectID: pidStr(),
		Name:      "e2e-staging",
	})
	requireNoError(t, err, "environment create")
	requireTrue(t, out.ID > 0, "environment ID should be positive")
	state.envID = out.ID
	t.Logf("Created environment: %s (ID=%d)", out.Name, out.ID)
}

func testEnvironmentGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.envID > 0, "environment ID not set")
	out, err := callTool[environments.Output](ctx, "gitlab_environment_get", environments.GetInput{
		ProjectID:     pidStr(),
		EnvironmentID: state.envID,
	})
	requireNoError(t, err, "environment get")
	requireTrue(t, out.Name == "e2e-staging", "expected name e2e-staging, got %s", out.Name)
	t.Logf("Got environment: %s (state=%s)", out.Name, out.State)
}

func testEnvironmentList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[environments.ListOutput](ctx, "gitlab_environment_list", environments.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "environment list")
	requireTrue(t, len(out.Environments) >= 1, "expected at least 1 environment, got %d", len(out.Environments))
	t.Logf("Listed %d environments", len(out.Environments))
}

func testEnvironmentStop(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.envID > 0, "environment ID not set")
	// Environment stop may fail if environment has no deployments — that's fine.
	_, err := callTool[environments.Output](ctx, "gitlab_environment_stop", environments.StopInput{
		ProjectID:     pidStr(),
		EnvironmentID: state.envID,
	})
	if err != nil {
		t.Logf("Environment stop returned error (expected without deployments): %v", err)
	} else {
		t.Logf("Stopped environment ID=%d", state.envID)
	}
}

func testEnvironmentDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.envID > 0, "environment ID not set")
	err := callToolVoid(ctx, "gitlab_environment_delete", environments.DeleteInput{
		ProjectID:     pidStr(),
		EnvironmentID: state.envID,
	})
	requireNoError(t, err, "environment delete")
	state.envID = 0
	t.Logf("Deleted environment")
}

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — Labels
// ---------------------------------------------------------------------------.

func testLabelCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[labels.Output](ctx, "gitlab_label_create", labels.CreateInput{
		ProjectID: pidStr(),
		Name:      "e2e-label",
		Color:     "#428BCA",
	})
	requireNoError(t, err, "label create")
	requireTrue(t, out.ID > 0, "label ID should be positive")
	state.labelID = out.ID
	t.Logf("Created label: %s (ID=%d, color=%s)", out.Name, out.ID, out.Color)
}

func testLabelUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.labelID > 0, "label ID not set")
	out, err := callTool[labels.Output](ctx, "gitlab_label_update", labels.UpdateInput{
		ProjectID:   pidStr(),
		LabelID:     toolutil.StringOrInt(strconv.FormatInt(state.labelID, 10)),
		Description: "Updated by E2E",
	})
	requireNoError(t, err, "label update")
	requireTrue(t, out.Description == "Updated by E2E", "expected updated description")
	t.Logf("Updated label: %s", out.Name)
}

func testLabelDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.labelID > 0, "label ID not set")
	err := callToolVoid(ctx, "gitlab_label_delete", labels.DeleteInput{
		ProjectID: pidStr(),
		LabelID:   toolutil.StringOrInt(strconv.FormatInt(state.labelID, 10)),
	})
	requireNoError(t, err, "label delete")
	state.labelID = 0
	t.Logf("Deleted label")
}

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — Milestones
// ---------------------------------------------------------------------------.

func testMilestoneCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[milestones.Output](ctx, "gitlab_milestone_create", milestones.CreateInput{
		ProjectID:   pidStr(),
		Title:       "e2e-milestone-v1",
		Description: "E2E test milestone",
	})
	requireNoError(t, err, "milestone create")
	requireTrue(t, out.IID > 0, "milestone IID should be positive")
	state.milestoneIID = out.IID
	t.Logf("Created milestone: %s (IID=%d)", out.Title, out.IID)
}

func testMilestoneGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.milestoneIID > 0, "milestone IID not set")
	out, err := callTool[milestones.Output](ctx, "gitlab_milestone_get", milestones.GetInput{
		ProjectID:    pidStr(),
		MilestoneIID: state.milestoneIID,
	})
	requireNoError(t, err, "milestone get")
	requireTrue(t, out.Title == "e2e-milestone-v1", "expected title e2e-milestone-v1, got %s", out.Title)
	t.Logf("Got milestone: %s (state=%s)", out.Title, out.State)
}

func testMilestoneUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.milestoneIID > 0, "milestone IID not set")
	out, err := callTool[milestones.Output](ctx, "gitlab_milestone_update", milestones.UpdateInput{
		ProjectID:    pidStr(),
		MilestoneIID: state.milestoneIID,
		Description:  "Updated by E2E test",
		StateEvent:   "close",
	})
	requireNoError(t, err, "milestone update")
	requireTrue(t, out.State == "closed", "expected state closed, got %s", out.State)
	t.Logf("Updated milestone: %s (state=%s)", out.Title, out.State)
}

func testMilestoneDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.milestoneIID > 0, "milestone IID not set")
	err := callToolVoid(ctx, "gitlab_milestone_delete", milestones.DeleteInput{
		ProjectID:    pidStr(),
		MilestoneIID: state.milestoneIID,
	})
	requireNoError(t, err, "milestone delete")
	state.milestoneIID = 0
	t.Logf("Deleted milestone")
}

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — Issue Links
// ---------------------------------------------------------------------------.

func testIssueCreateSecond(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	// Recreate the first issue if it was deleted earlier in the workflow.
	if state.issueIID == 0 {
		firstOut, firstErr := callTool[issues.Output](ctx, "gitlab_issue_create", issues.CreateInput{
			ProjectID:   pidStr(),
			Title:       "E2E first issue (recreated for linking)",
			Description: "Source issue for issue link test",
		})
		requireNoError(t, firstErr, "recreate first issue")
		requireTrue(t, firstOut.IID > 0, "first issue IID should be positive")
		state.issueIID = firstOut.IID
		t.Logf("Recreated first issue: #%d", firstOut.IID)
	}

	out, err := callTool[issues.Output](ctx, "gitlab_issue_create", issues.CreateInput{
		ProjectID:   pidStr(),
		Title:       "E2E second issue for linking",
		Description: "Target issue for issue link test",
	})
	requireNoError(t, err, "create second issue")
	requireTrue(t, out.IID > 0, "issue IID should be positive")
	state.issue2IID = out.IID
	t.Logf("Created second issue: #%d", out.IID)
}

func testIssueLinkCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, "issue IID not set")
	requireTrue(t, state.issue2IID > 0, "second issue IID not set")
	out, err := callTool[issuelinks.Output](ctx, "gitlab_issue_link_create", issuelinks.CreateInput{
		ProjectID:       pidStr(),
		IssueIID:        int(state.issueIID),
		TargetProjectID: strconv.FormatInt(state.projectID, 10),
		TargetIssueIID:  strconv.FormatInt(state.issue2IID, 10),
	})
	requireNoError(t, err, "issue link create")
	requireTrue(t, out.ID > 0, "issue link ID should be positive")
	t.Logf("Created issue link: ID=%d (source=#%d → target=#%d)", out.ID, state.issueIID, state.issue2IID)
}

func testIssueLinkList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, "issue IID not set")
	out, err := callTool[issuelinks.ListOutput](ctx, "gitlab_issue_link_list", issuelinks.ListInput{
		ProjectID: pidStr(),
		IssueIID:  int(state.issueIID),
	})
	requireNoError(t, err, "issue link list")
	requireTrue(t, len(out.Relations) >= 1, "expected at least 1 issue link, got %d", len(out.Relations))
	t.Logf("Listed %d issue links", len(out.Relations))
}

func testIssueLinkDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, "issue IID not set")
	// First, get the link ID from the list.
	listOut, err := callTool[issuelinks.ListOutput](ctx, "gitlab_issue_link_list", issuelinks.ListInput{
		ProjectID: pidStr(),
		IssueIID:  int(state.issueIID),
	})
	requireNoError(t, err, "list issue links for delete")
	requireTrue(t, len(listOut.Relations) >= 1, "no issue links found to delete")
	linkID := listOut.Relations[0].IssueLinkID

	err = callToolVoid(ctx, "gitlab_issue_link_delete", issuelinks.DeleteInput{
		ProjectID:   pidStr(),
		IssueIID:    int(state.issueIID),
		IssueLinkID: linkID,
	})
	requireNoError(t, err, "issue link delete")
	t.Logf("Deleted issue link ID=%d", linkID)
}

func testIssueDeleteSecond(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issue2IID > 0, "second issue IID not set")
	err := callToolVoid(ctx, "gitlab_issue_delete", issues.DeleteInput{
		ProjectID: pidStr(),
		IssueIID:  state.issue2IID,
	})
	requireNoError(t, err, "delete second issue")
	state.issue2IID = 0
	t.Logf("Deleted second issue")
}

// ---------------------------------------------------------------------------
// Phase 3: Extended coverage — Todos
// ---------------------------------------------------------------------------.

func testTodoCreateFromIssue(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	// Create a new issue to generate a todo (the existing issueIID may be deleted).
	out, err := callTool[issues.Output](ctx, "gitlab_issue_create", issues.CreateInput{
		ProjectID:   pidStr(),
		Title:       "E2E todo issue",
		Description: "Issue for testing todos",
	})
	requireNoError(t, err, "create issue for todo")
	state.issueIID = out.IID
	t.Logf("Created issue #%d for todo tests", out.IID)
}

func testTodoList(ctx context.Context, t *testing.T) {
	out, err := callTool[todos.ListOutput](ctx, "gitlab_todo_list", todos.ListInput{
		State: "pending",
	})
	requireNoError(t, err, "todo list")
	// May be 0 if the user has no pending todos — that's fine for now.
	t.Logf("Listed %d pending todos", len(out.Todos))
}

func testTodoMarkAllDone(ctx context.Context, t *testing.T) {
	out, err := callTool[todos.MarkAllDoneOutput](ctx, "gitlab_todo_mark_all_done", todos.MarkAllDoneInput{})
	requireNoError(t, err, "todo mark all done")
	t.Logf("Marked all todos done: %s", out.Message)
}

// ---------------------------------------------------------------------------
// Phase 3/4: Extended coverage — Deploy Keys
// ---------------------------------------------------------------------------.

// testDeployKeySSHPub is a disposable ED25519 public key for E2E tests only.
const testDeployKeySSHPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGcb4V7ZTNDiBUNBOYQFLxdBPTQ5iJqMXpB3cOU47Rl6 e2e-disposable-key"

func testDeployKeyCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[deploykeys.Output](ctx, "gitlab_deploy_key_add", deploykeys.AddInput{
		ProjectID: pidStr(),
		Title:     "E2E Deploy Key",
		Key:       testDeployKeySSHPub,
	})
	requireNoError(t, err, "deploy key add")
	requireTrue(t, out.ID > 0, "deploy key ID should be positive")
	state.deployKeyID = out.ID
	t.Logf("Created deploy key: %s (ID=%d)", out.Title, out.ID)
}

func testDeployKeyGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.deployKeyID > 0, "deploy key ID not set")
	out, err := callTool[deploykeys.Output](ctx, "gitlab_deploy_key_get", deploykeys.GetInput{
		ProjectID:   pidStr(),
		DeployKeyID: state.deployKeyID,
	})
	requireNoError(t, err, "deploy key get")
	requireTrue(t, out.Title == "E2E Deploy Key", "expected title 'E2E Deploy Key', got %q", out.Title)
	t.Logf("Got deploy key: %s (ID=%d)", out.Title, out.ID)
}

func testDeployKeyList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[deploykeys.ListOutput](ctx, "gitlab_deploy_key_list_project", deploykeys.ListProjectInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "deploy key list")
	requireTrue(t, len(out.DeployKeys) >= 1, "expected at least 1 deploy key, got %d", len(out.DeployKeys))
	t.Logf("Listed %d deploy keys", len(out.DeployKeys))
}

func testDeployKeyDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.deployKeyID > 0, "deploy key ID not set")
	err := callToolVoid(ctx, "gitlab_deploy_key_delete", deploykeys.DeleteInput{
		ProjectID:   pidStr(),
		DeployKeyID: state.deployKeyID,
	})
	requireNoError(t, err, "deploy key delete")
	state.deployKeyID = 0
	t.Logf("Deleted deploy key")
}

// ---------------------------------------------------------------------------
// Phase 3/4: Extended coverage — Project Snippets
// ---------------------------------------------------------------------------.

func testProjectSnippetCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[snippets.Output](ctx, "gitlab_project_snippet_create", snippets.ProjectCreateInput{
		ProjectID:   pidStr(),
		Title:       "E2E Snippet",
		FileName:    "e2e.txt",
		ContentBody: "Hello from E2E test",
		Visibility:  "private",
	})
	requireNoError(t, err, "project snippet create")
	requireTrue(t, out.ID > 0, "snippet ID should be positive")
	state.snippetID = out.ID
	t.Logf("Created project snippet: %s (ID=%d)", out.Title, out.ID)
}

func testProjectSnippetGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.snippetID > 0, "snippet ID not set")
	out, err := callTool[snippets.Output](ctx, "gitlab_project_snippet_get", snippets.ProjectGetInput{
		ProjectID: pidStr(),
		SnippetID: state.snippetID,
	})
	requireNoError(t, err, "project snippet get")
	requireTrue(t, out.Title == "E2E Snippet", "expected title 'E2E Snippet', got %q", out.Title)
	t.Logf("Got snippet: %s", out.Title)
}

func testProjectSnippetList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[snippets.ListOutput](ctx, "gitlab_project_snippet_list", snippets.ProjectListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "project snippet list")
	requireTrue(t, len(out.Snippets) >= 1, "expected at least 1 snippet, got %d", len(out.Snippets))
	t.Logf("Listed %d project snippets", len(out.Snippets))
}

func testProjectSnippetUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.snippetID > 0, "snippet ID not set")
	out, err := callTool[snippets.Output](ctx, "gitlab_project_snippet_update", snippets.ProjectUpdateInput{
		ProjectID: pidStr(),
		SnippetID: state.snippetID,
		Title:     "E2E Snippet Updated",
	})
	requireNoError(t, err, "project snippet update")
	requireTrue(t, out.Title == "E2E Snippet Updated", "expected updated title")
	t.Logf("Updated snippet: %s", out.Title)
}

func testProjectSnippetDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.snippetID > 0, "snippet ID not set")
	err := callToolVoid(ctx, "gitlab_project_snippet_delete", snippets.ProjectDeleteInput{
		ProjectID: pidStr(),
		SnippetID: state.snippetID,
	})
	requireNoError(t, err, "project snippet delete")
	state.snippetID = 0
	t.Logf("Deleted snippet")
}

// ---------------------------------------------------------------------------
// Phase 3/4: Extended coverage — Issue Discussions
// ---------------------------------------------------------------------------.

func testIssueDiscussionCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)
	out, err := callTool[issuediscussions.Output](ctx, "gitlab_create_issue_discussion", issuediscussions.CreateInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
		Body:      "E2E discussion thread",
	})
	requireNoError(t, err, "issue discussion create")
	requireTrue(t, out.ID != "", "discussion ID should not be empty")
	state.issueDiscussionID = out.ID
	if len(out.Notes) > 0 {
		state.issueDiscussionNoteID = out.Notes[0].ID
	}
	t.Logf("Created issue discussion: %s (%d notes)", out.ID, len(out.Notes))
}

func testIssueDiscussionList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)
	out, err := callTool[issuediscussions.ListOutput](ctx, "gitlab_list_issue_discussions", issuediscussions.ListInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
	})
	requireNoError(t, err, "issue discussion list")
	requireTrue(t, len(out.Discussions) >= 1, "expected at least 1 discussion, got %d", len(out.Discussions))
	t.Logf("Listed %d issue discussions", len(out.Discussions))
}

func testIssueDiscussionAddNote(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueDiscussionID != "", "issue discussion ID not set")
	out, err := callTool[issuediscussions.NoteOutput](ctx, "gitlab_add_issue_discussion_note", issuediscussions.AddNoteInput{
		ProjectID:    pidStr(),
		IssueIID:     state.issueIID,
		DiscussionID: state.issueDiscussionID,
		Body:         "E2E reply note",
	})
	requireNoError(t, err, "issue discussion add note")
	requireTrue(t, out.ID > 0, "note ID should be positive")
	state.issueDiscussionNoteID = out.ID
	t.Logf("Added note to discussion: ID=%d", out.ID)
}

func testIssueDiscussionDeleteNote(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueDiscussionID != "", "issue discussion ID not set")
	requireTrue(t, state.issueDiscussionNoteID > 0, "issue discussion note ID not set")
	err := callToolVoid(ctx, "gitlab_delete_issue_discussion_note", issuediscussions.DeleteNoteInput{
		ProjectID:    pidStr(),
		IssueIID:     state.issueIID,
		DiscussionID: state.issueDiscussionID,
		NoteID:       state.issueDiscussionNoteID,
	})
	requireNoError(t, err, "issue discussion delete note")
	state.issueDiscussionNoteID = 0
	t.Logf("Deleted note from discussion")
}

// ---------------------------------------------------------------------------
// Phase 3/4: Extended coverage — MR Draft Notes
// ---------------------------------------------------------------------------.

func testMRDraftNoteCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.mrIID > 0, "MR IID not set")
	out, err := callTool[mrdraftnotes.Output](ctx, "gitlab_mr_draft_note_create", mrdraftnotes.CreateInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
		Note:      "E2E draft note review comment",
	})
	requireNoError(t, err, "MR draft note create")
	requireTrue(t, out.ID > 0, "draft note ID should be positive")
	state.draftNoteID = out.ID
	t.Logf("Created MR draft note: ID=%d", out.ID)
}

func testMRDraftNoteList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.mrIID > 0, "MR IID not set")
	out, err := callTool[mrdraftnotes.ListOutput](ctx, "gitlab_mr_draft_note_list", mrdraftnotes.ListInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "MR draft note list")
	requireTrue(t, len(out.DraftNotes) >= 1, "expected at least 1 draft note, got %d", len(out.DraftNotes))
	t.Logf("Listed %d MR draft notes", len(out.DraftNotes))
}

func testMRDraftNoteUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.draftNoteID > 0, "draft note ID not set")
	out, err := callTool[mrdraftnotes.Output](ctx, "gitlab_mr_draft_note_update", mrdraftnotes.UpdateInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
		NoteID:    state.draftNoteID,
		Note:      "Updated E2E draft note",
	})
	requireNoError(t, err, "MR draft note update")
	requireTrue(t, out.Note == "Updated E2E draft note", "expected updated note text")
	t.Logf("Updated MR draft note: ID=%d", out.ID)
}

func testMRDraftNotePublishAll(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.mrIID > 0, "MR IID not set")
	err := callToolVoid(ctx, "gitlab_mr_draft_note_publish_all", mrdraftnotes.PublishAllInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "MR draft note publish all")
	state.draftNoteID = 0
	t.Logf("Published all MR draft notes")
}

// ---------------------------------------------------------------------------
// Phase 4: Extended coverage — Pipeline Schedules
// ---------------------------------------------------------------------------.

func testPipelineScheduleCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule_create", pipelineschedules.CreateInput{
		ProjectID:   pidStr(),
		Description: "E2E nightly schedule",
		Ref:         defaultBranch,
		Cron:        "0 3 * * *",
	})
	requireNoError(t, err, "pipeline schedule create")
	requireTrue(t, out.ID > 0, "schedule ID should be positive")
	state.pipelineScheduleID = out.ID
	t.Logf("Created pipeline schedule: %s (ID=%d)", out.Description, out.ID)
}

func testPipelineScheduleGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineScheduleID > 0, "pipeline schedule ID not set")
	out, err := callTool[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule_get", pipelineschedules.GetInput{
		ProjectID:  pidStr(),
		ScheduleID: state.pipelineScheduleID,
	})
	requireNoError(t, err, "pipeline schedule get")
	requireTrue(t, out.Description == "E2E nightly schedule", "expected description match")
	t.Logf("Got pipeline schedule: %s (active=%v)", out.Description, out.Active)
}

func testPipelineScheduleList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[pipelineschedules.ListOutput](ctx, "gitlab_pipeline_schedule_list", pipelineschedules.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "pipeline schedule list")
	requireTrue(t, len(out.Schedules) >= 1, "expected at least 1 schedule, got %d", len(out.Schedules))
	t.Logf("Listed %d pipeline schedules", len(out.Schedules))
}

func testPipelineScheduleUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineScheduleID > 0, "pipeline schedule ID not set")
	out, err := callTool[pipelineschedules.Output](ctx, "gitlab_pipeline_schedule_update", pipelineschedules.UpdateInput{
		ProjectID:   pidStr(),
		ScheduleID:  state.pipelineScheduleID,
		Description: "E2E updated schedule",
		Cron:        "0 4 * * *",
	})
	requireNoError(t, err, "pipeline schedule update")
	requireTrue(t, out.Description == "E2E updated schedule", "expected updated description")
	t.Logf("Updated pipeline schedule: %s", out.Description)
}

func testPipelineScheduleDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineScheduleID > 0, "pipeline schedule ID not set")
	err := callToolVoid(ctx, "gitlab_pipeline_schedule_delete", pipelineschedules.DeleteInput{
		ProjectID:  pidStr(),
		ScheduleID: state.pipelineScheduleID,
	})
	requireNoError(t, err, "pipeline schedule delete")
	state.pipelineScheduleID = 0
	t.Logf("Deleted pipeline schedule")
}

// ---------------------------------------------------------------------------
// Phase 4: Extended coverage — Badges (project)
// ---------------------------------------------------------------------------.

func testBadgeCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[badges.AddProjectOutput](ctx, "gitlab_add_project_badge", badges.AddProjectInput{
		ProjectID: pidStr(),
		LinkURL:   "https://example.com/badge",
		ImageURL:  "https://img.shields.io/badge/e2e-passing-green",
		Name:      "E2E Badge",
	})
	requireNoError(t, err, "badge add")
	requireTrue(t, out.Badge.ID > 0, "badge ID should be positive")
	state.badgeID = out.Badge.ID
	t.Logf("Created badge: %s (ID=%d)", out.Badge.Name, out.Badge.ID)
}

func testBadgeList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[badges.ListProjectOutput](ctx, "gitlab_list_project_badges", badges.ListProjectInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "badge list")
	requireTrue(t, len(out.Badges) >= 1, "expected at least 1 badge, got %d", len(out.Badges))
	t.Logf("Listed %d project badges", len(out.Badges))
}

func testBadgeUpdate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.badgeID > 0, "badge ID not set")
	out, err := callTool[badges.EditProjectOutput](ctx, "gitlab_edit_project_badge", badges.EditProjectInput{
		ProjectID: pidStr(),
		BadgeID:   state.badgeID,
		Name:      "E2E Badge Updated",
	})
	requireNoError(t, err, "badge edit")
	requireTrue(t, out.Badge.Name == "E2E Badge Updated", "expected updated name")
	t.Logf("Updated badge: %s", out.Badge.Name)
}

func testBadgeDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.badgeID > 0, "badge ID not set")
	err := callToolVoid(ctx, "gitlab_delete_project_badge", badges.DeleteProjectInput{
		ProjectID: pidStr(),
		BadgeID:   state.badgeID,
	})
	requireNoError(t, err, "badge delete")
	state.badgeID = 0
	t.Logf("Deleted badge")
}

// ---------------------------------------------------------------------------
// Phase 4: Extended coverage — Access Tokens (project)
// ---------------------------------------------------------------------------.

func testAccessTokenCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	// ExpiresAt must be in the future (YYYY-MM-DD).
	expiry := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	out, err := callTool[accesstokens.Output](ctx, "gitlab_project_access_token_create", accesstokens.ProjectCreateInput{
		ProjectID:   pidStr(),
		Name:        "E2E Token",
		Scopes:      []string{"api"},
		AccessLevel: 30,
		ExpiresAt:   expiry,
	})
	requireNoError(t, err, "access token create")
	requireTrue(t, out.ID > 0, "token ID should be positive")
	state.accessTokenID = out.ID
	t.Logf("Created project access token: %s (ID=%d)", out.Name, out.ID)
}

func testAccessTokenList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[accesstokens.ListOutput](ctx, "gitlab_project_access_token_list", accesstokens.ProjectListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "access token list")
	requireTrue(t, len(out.Tokens) >= 1, "expected at least 1 token, got %d", len(out.Tokens))
	t.Logf("Listed %d project access tokens", len(out.Tokens))
}

func testAccessTokenRevoke(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.accessTokenID > 0, "access token ID not set")
	err := callToolVoid(ctx, "gitlab_project_access_token_revoke", accesstokens.ProjectRevokeInput{
		ProjectID: pidStr(),
		TokenID:   state.accessTokenID,
	})
	requireNoError(t, err, "access token revoke")
	state.accessTokenID = 0
	t.Logf("Revoked project access token")
}

// ---------------------------------------------------------------------------
// Phase 4: Extended coverage — Award Emoji (on issue)
// ---------------------------------------------------------------------------.

func testAwardEmojiCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)
	out, err := callTool[awardemoji.Output](ctx, "gitlab_issue_emoji_create", awardemoji.CreateInput{
		ProjectID: pidStr(),
		IID:       state.issueIID,
		Name:      "thumbsup",
	})
	requireNoError(t, err, "award emoji create")
	requireTrue(t, out.ID > 0, "award emoji ID should be positive")
	state.awardEmojiID = out.ID
	t.Logf("Created award emoji: %s (ID=%d)", out.Name, out.ID)
}

func testAwardEmojiList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)
	out, err := callTool[awardemoji.ListOutput](ctx, "gitlab_issue_emoji_list", awardemoji.ListInput{
		ProjectID: pidStr(),
		IID:       state.issueIID,
	})
	requireNoError(t, err, "award emoji list")
	requireTrue(t, len(out.AwardEmoji) >= 1, "expected at least 1 emoji, got %d", len(out.AwardEmoji))
	t.Logf("Listed %d award emoji", len(out.AwardEmoji))
}

func testAwardEmojiDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.awardEmojiID > 0, "award emoji ID not set")
	err := callToolVoid(ctx, "gitlab_issue_emoji_delete", awardemoji.DeleteInput{
		ProjectID: pidStr(),
		IID:       state.issueIID,
		AwardID:   state.awardEmojiID,
	})
	requireNoError(t, err, "award emoji delete")
	state.awardEmojiID = 0
	t.Logf("Deleted award emoji")
}

// ---------------------------------------------------------------------------
// Phase 4a: Pipeline & Job lifecycle (Docker mode only)
// ---------------------------------------------------------------------------
// These tests require a CI runner. In self-hosted mode without a runner they
// are skipped. The flow: commit .gitlab-ci.yml → trigger pipeline → wait →
// inspect jobs → retry → delete.
// ---------------------------------------------------------------------------.

// testCIYAML is a minimal CI configuration used by pipeline E2E tests.
const testCIYAML = `stages:
  - test

fast-pass:
  stage: test
  script:
    - echo "E2E fast-pass job"
  tags: []
`

// hasRunner returns true if a CI runner is available for pipeline tests.
// In Docker mode it always returns true; in self-hosted mode it checks the
// Runners API for registered instance runners.
func hasRunner() bool {
	if isDockerMode() {
		return true
	}
	runnerType := "instance_type"
	runners, _, err := state.glClient.GL().Runners.ListRunners(&gl.ListRunnersOptions{
		Type: &runnerType,
	})
	return err == nil && len(runners) > 0
}

func testPipelineCICommit(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	_, err := callTool[commits.Output](ctx, "gitlab_commit_create", commits.CreateInput{
		ProjectID:     pidStr(),
		Branch:        defaultBranch,
		CommitMessage: "ci: add .gitlab-ci.yml for E2E pipeline tests",
		Actions: []commits.Action{{
			Action:   "create",
			FilePath: ".gitlab-ci.yml",
			Content:  testCIYAML,
		}},
	})
	requireNoError(t, err, "commit CI config")
	t.Logf("Committed .gitlab-ci.yml to %s", defaultBranch)
}

func testPipelineCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callTool[pipelines.DetailOutput](ctx, "gitlab_pipeline_create", pipelines.CreateInput{
		ProjectID: pidStr(),
		Ref:       defaultBranch,
	})
	requireNoError(t, err, "pipeline create")
	requireTrue(t, out.ID > 0, "pipeline ID should be positive")
	state.pipelineID = out.ID
	t.Logf("Created pipeline: ID=%d status=%s ref=%s", out.ID, out.Status, out.Ref)
}

func testPipelineGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineID > 0, "pipeline ID not set")

	out, err := callTool[pipelines.DetailOutput](ctx, "gitlab_pipeline_get", pipelines.GetInput{
		ProjectID:  pidStr(),
		PipelineID: state.pipelineID,
	})
	requireNoError(t, err, "pipeline get")
	requireTrue(t, out.ID == state.pipelineID, "expected pipeline ID %d, got %d", state.pipelineID, out.ID)
	t.Logf("Got pipeline: ID=%d status=%s", out.ID, out.Status)
}

func testPipelineListWithCI(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callTool[pipelines.ListOutput](ctx, "gitlab_pipeline_list", pipelines.ListInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "pipeline list")
	requireTrue(t, len(out.Pipelines) >= 1, "expected at least 1 pipeline, got %d", len(out.Pipelines))
	t.Logf("Listed %d pipelines", len(out.Pipelines))
}

func testPipelineWaitAndJobList(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineID > 0, "pipeline ID not set")

	// Wait for pipeline to finish.
	status := waitForPipeline(t, state.projectID, state.pipelineID, 180*time.Second)
	t.Logf("Pipeline %d finished with status: %s", state.pipelineID, status)

	// List jobs in the pipeline.
	out, err := callTool[jobs.ListOutput](ctx, "gitlab_job_list", jobs.ListInput{
		ProjectID:  pidStr(),
		PipelineID: state.pipelineID,
	})
	requireNoError(t, err, "job list")
	requireTrue(t, len(out.Jobs) >= 1, "expected at least 1 job, got %d", len(out.Jobs))
	state.jobID = out.Jobs[0].ID
	t.Logf("Listed %d jobs; first job: ID=%d name=%s status=%s", len(out.Jobs), out.Jobs[0].ID, out.Jobs[0].Name, out.Jobs[0].Status)
}

func testJobGet(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.jobID > 0, "job ID not set")

	out, err := callTool[jobs.Output](ctx, "gitlab_job_get", jobs.GetInput{
		ProjectID: pidStr(),
		JobID:     state.jobID,
	})
	requireNoError(t, err, "job get")
	requireTrue(t, out.ID == state.jobID, "expected job ID %d, got %d", state.jobID, out.ID)
	t.Logf("Got job: ID=%d name=%s status=%s", out.ID, out.Name, out.Status)
}

func testJobTrace(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.jobID > 0, "job ID not set")

	out, err := callTool[jobs.TraceOutput](ctx, "gitlab_job_trace", jobs.TraceInput{
		ProjectID: pidStr(),
		JobID:     state.jobID,
	})
	requireNoError(t, err, "job trace")
	requireTrue(t, len(out.Trace) > 0, "expected non-empty job trace")
	t.Logf("Got job trace: %d chars (truncated=%v)", len(out.Trace), out.Truncated)
}

func testPipelineRetry(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineID > 0, "pipeline ID not set")

	out, err := callTool[pipelines.DetailOutput](ctx, "gitlab_pipeline_retry", pipelines.ActionInput{
		ProjectID:  pidStr(),
		PipelineID: state.pipelineID,
	})
	requireNoError(t, err, "pipeline retry")
	t.Logf("Retried pipeline: ID=%d status=%s", out.ID, out.Status)

	// Wait for retry to complete before deleting.
	waitForPipeline(t, state.projectID, state.pipelineID, 180*time.Second)
}

func testPipelineDelete(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineID > 0, "pipeline ID not set")

	err := callToolVoid(ctx, "gitlab_pipeline_delete", pipelines.DeleteInput{
		ProjectID:  pidStr(),
		PipelineID: state.pipelineID,
	})
	requireNoError(t, err, "pipeline delete")
	state.pipelineID = 0
	t.Logf("Deleted pipeline")
}

// ---------------------------------------------------------------------------
// Sampling tools (steps 298–310) — use mock LLM handler via samplingSession
// ---------------------------------------------------------------------------

// testSamplingSetupIssue creates a temporary issue so sampling tools that
// require an issue IID can operate after the original issue was deleted.
func testSamplingSetupIssue(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[issues.Output](ctx, "gitlab_issue_create", issues.CreateInput{
		ProjectID:   pidStr(),
		Title:       "Sampling test issue",
		Description: "Temporary issue for sampling tool E2E tests.",
	})
	requireNoError(t, err, "create sampling issue")
	state.issueIID = out.IID
	t.Logf("Created sampling issue IID=%d", state.issueIID)
}

// testSamplingSetupMilestone creates a temporary milestone so the
// milestone report sampling tool can operate.
func testSamplingSetupMilestone(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	out, err := callTool[milestones.Output](ctx, "gitlab_milestone_create", milestones.CreateInput{
		ProjectID:   pidStr(),
		Title:       "Sampling test milestone",
		Description: "Temporary milestone for sampling tool E2E tests.",
	})
	requireNoError(t, err, "create sampling milestone")
	state.milestoneIID = out.IID
	t.Logf("Created sampling milestone IID=%d", state.milestoneIID)
}

// testSamplingAnalyzeMRChanges verifies the analyze_mr_changes sampling tool
// returns a non-empty analysis from the mock LLM.
func testSamplingAnalyzeMRChanges(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.mrIID > 0, "MR IID not set")

	out, err := callSamplingTool[samplingtools.AnalyzeMRChangesOutput](ctx, "gitlab_analyze_mr_changes", samplingtools.AnalyzeMRChangesInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "sampling analyze MR changes")
	requireTrue(t, out.Analysis != "", "expected non-empty analysis")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Analyzed MR changes: model=%s, analysis_len=%d", out.Model, len(out.Analysis))
}

// testSamplingSummarizeIssue verifies the summarize_issue sampling tool.
func testSamplingSummarizeIssue(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)

	out, err := callSamplingTool[samplingtools.SummarizeIssueOutput](ctx, "gitlab_summarize_issue", samplingtools.SummarizeIssueInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
	})
	requireNoError(t, err, "sampling summarize issue")
	requireTrue(t, out.Summary != "", "expected non-empty summary")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Summarized issue: model=%s, summary_len=%d", out.Model, len(out.Summary))
}

// testSamplingGenerateReleaseNotes verifies the generate_release_notes sampling tool.
func testSamplingGenerateReleaseNotes(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.lastCommitSHA != "", "last commit SHA not set")

	out, err := callSamplingTool[samplingtools.GenerateReleaseNotesOutput](ctx, "gitlab_generate_release_notes", samplingtools.GenerateReleaseNotesInput{
		ProjectID: pidStr(),
		From:      state.lastCommitSHA,
		To:        defaultBranch,
	})
	requireNoError(t, err, "sampling generate release notes")
	requireTrue(t, out.ReleaseNotes != "", "expected non-empty release notes")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Generated release notes: model=%s, notes_len=%d", out.Model, len(out.ReleaseNotes))
}

// testSamplingSummarizeMRReview verifies the summarize_mr_review sampling tool.
func testSamplingSummarizeMRReview(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.mrIID > 0, "MR IID not set")

	out, err := callSamplingTool[samplingtools.SummarizeMRReviewOutput](ctx, "gitlab_summarize_mr_review", samplingtools.SummarizeMRReviewInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "sampling summarize MR review")
	requireTrue(t, out.Summary != "", "expected non-empty summary")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Summarized MR review: model=%s, summary_len=%d", out.Model, len(out.Summary))
}

// testSamplingAnalyzeCIConfig verifies the analyze_ci_configuration sampling tool.
func testSamplingAnalyzeCIConfig(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callSamplingTool[samplingtools.AnalyzeCIConfigOutput](ctx, "gitlab_analyze_ci_configuration", samplingtools.AnalyzeCIConfigInput{
		ProjectID:  pidStr(),
		ContentRef: defaultBranch,
	})
	requireNoError(t, err, "sampling analyze CI config")
	requireTrue(t, out.Analysis != "", "expected non-empty analysis")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Analyzed CI config: model=%s, analysis_len=%d", out.Model, len(out.Analysis))
}

// testSamplingAnalyzeIssueScope verifies the analyze_issue_scope sampling tool.
func testSamplingAnalyzeIssueScope(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.issueIID > 0, msgIssueIIDNotSet)

	out, err := callSamplingTool[samplingtools.AnalyzeIssueScopeOutput](ctx, "gitlab_analyze_issue_scope", samplingtools.AnalyzeIssueScopeInput{
		ProjectID: pidStr(),
		IssueIID:  state.issueIID,
	})
	requireNoError(t, err, "sampling analyze issue scope")
	requireTrue(t, out.Analysis != "", "expected non-empty analysis")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Analyzed issue scope: model=%s, analysis_len=%d", out.Model, len(out.Analysis))
}

// testSamplingReviewMRSecurity verifies the review_mr_security sampling tool.
func testSamplingReviewMRSecurity(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.mrIID > 0, "MR IID not set")

	out, err := callSamplingTool[samplingtools.ReviewMRSecurityOutput](ctx, "gitlab_review_mr_security", samplingtools.ReviewMRSecurityInput{
		ProjectID: pidStr(),
		MRIID:     state.mrIID,
	})
	requireNoError(t, err, "sampling review MR security")
	requireTrue(t, out.Review != "", "expected non-empty review")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Reviewed MR security: model=%s, review_len=%d", out.Model, len(out.Review))
}

// testSamplingFindTechnicalDebt verifies the find_technical_debt sampling tool.
func testSamplingFindTechnicalDebt(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callSamplingTool[samplingtools.FindTechnicalDebtOutput](ctx, "gitlab_find_technical_debt", samplingtools.FindTechnicalDebtInput{
		ProjectID: pidStr(),
		Ref:       defaultBranch,
	})
	requireNoError(t, err, "sampling find technical debt")
	requireTrue(t, out.Analysis != "", "expected non-empty analysis")
	if strings.Contains(out.Analysis, "No technical debt markers") {
		t.Logf("No technical debt found (LLM not invoked): analysis=%q", out.Analysis)
	} else {
		requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
		t.Logf("Found technical debt: model=%s, analysis_len=%d", out.Model, len(out.Analysis))
	}
}

// testSamplingAnalyzeDeploymentHistory verifies the analyze_deployment_history sampling tool.
func testSamplingAnalyzeDeploymentHistory(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callSamplingTool[samplingtools.AnalyzeDeploymentHistoryOutput](ctx, "gitlab_analyze_deployment_history", samplingtools.AnalyzeDeploymentHistoryInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "sampling analyze deployment history")
	requireTrue(t, out.Analysis != "", "expected non-empty analysis")
	if strings.Contains(out.Analysis, "No deployments found") {
		t.Logf("No deployments found (LLM not invoked): analysis=%q", out.Analysis)
	} else {
		requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
		t.Logf("Analyzed deployment history: model=%s, analysis_len=%d", out.Model, len(out.Analysis))
	}
}

// testSamplingAnalyzePipelineFailure verifies the analyze_pipeline_failure sampling tool.
// Runs in the pipeline lifecycle section (after retry, before delete) so the pipeline exists.
func testSamplingAnalyzePipelineFailure(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.pipelineID > 0, "pipeline ID not set")

	out, err := callSamplingTool[samplingtools.AnalyzePipelineFailureOutput](ctx, "gitlab_analyze_pipeline_failure", samplingtools.AnalyzePipelineFailureInput{
		ProjectID:  pidStr(),
		PipelineID: state.pipelineID,
	})
	requireNoError(t, err, "sampling analyze pipeline failure")
	requireTrue(t, out.Analysis != "", "expected non-empty analysis")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Analyzed pipeline failure: model=%s, analysis_len=%d", out.Model, len(out.Analysis))
}

// testSamplingGenerateMilestoneReport verifies the generate_milestone_report sampling tool.
func testSamplingGenerateMilestoneReport(ctx context.Context, t *testing.T) {
	requireProjectID(t)
	requireTrue(t, state.milestoneIID > 0, "milestone IID not set")

	out, err := callSamplingTool[samplingtools.GenerateMilestoneReportOutput](ctx, "gitlab_generate_milestone_report", samplingtools.GenerateMilestoneReportInput{
		ProjectID:    pidStr(),
		MilestoneIID: state.milestoneIID,
	})
	requireNoError(t, err, "sampling generate milestone report")
	requireTrue(t, out.Report != "", "expected non-empty report")
	requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
	t.Logf("Generated milestone report: model=%s, report_len=%d", out.Model, len(out.Report))
}

// ---------------------------------------------------------------------------
// Elicitation tools (steps 400+) — use auto-accept mock via elicitSession
// ---------------------------------------------------------------------------

// testElicitInteractiveIssueCreate verifies the interactive issue creation tool
// works end-to-end with a mock elicitation handler that auto-accepts all prompts.
func testElicitInteractiveIssueCreate(ctx context.Context, t *testing.T) {
	requireProjectID(t)

	out, err := callElicitTool[issues.Output](ctx, "gitlab_interactive_issue_create", elicitationtools.IssueInput{
		ProjectID: pidStr(),
	})
	requireNoError(t, err, "elicit interactive issue create")
	requireTrue(t, out.IID > 0, "expected positive issue IID, got %d", out.IID)
	requireTrue(t, out.Title == "E2E elicitation test", "expected elicited title, got %q", out.Title)
	t.Logf("Created issue via elicitation: IID=%d, title=%q", out.IID, out.Title)
}
