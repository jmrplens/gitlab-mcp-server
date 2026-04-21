// markdown_audit_test.go validates structural quality of all Markdown output
// produced by the markdownForResult dispatcher. It checks:
//   - Every dispatched type returns a non-nil CallToolResult
//   - TextContent is non-empty
//   - Markdown tables have consistent column counts (header, separator, data rows)
//   - No extra pipe characters that indicate unescaped cell data
//
// Run with: go test ./internal/tools/ -run TestMarkdownAudit -count=1 -v.
package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- Markdown structural validators ----------.

// countPipes returns the number of pipe characters in a line.
func countPipes(line string) int {
	return strings.Count(line, "|")
}

// isTableSeparator returns true if the line is a markdown table separator
// (e.g., "| --- | --- | --- |").
func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}
	inner := strings.Trim(trimmed, "| ")
	for cell := range strings.SplitSeq(inner, "|") {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		cleaned := strings.ReplaceAll(cell, "-", "")
		cleaned = strings.ReplaceAll(cleaned, ":", "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" {
			return false
		}
	}
	return true
}

// isTableRow returns true if the line looks like a markdown table row.
func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}

// tableIssue describes a single markdown table structure issue.
type tableIssue struct {
	lineNum     int
	description string
}

// validateMarkdownTables scans markdown text for table blocks and validates
// that every row in each table has the same number of columns (pipe count).
// Returns a slice of issues found.
func validateMarkdownTables(md string) []tableIssue {
	var issues []tableIssue
	lines := strings.Split(md, "\n")

	inTable := false
	var headerPipes int
	var tableStart int

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		if !inTable {
			// Detect table start: a line with pipes that is a table row
			if isTableRow(trimmed) {
				inTable = true
				headerPipes = countPipes(trimmed)
				tableStart = lineNum
			}
			continue
		}

		// Inside a table
		if trimmed == "" || !isTableRow(trimmed) {
			// Table ended
			inTable = false
			continue
		}

		pipes := countPipes(trimmed)
		if pipes != headerPipes {
			issues = append(issues, tableIssue{
				lineNum: lineNum,
				description: fmt.Sprintf(
					"column mismatch: line has %d pipes, header (line %d) has %d pipes",
					pipes, tableStart, headerPipes),
			})
		}
	}

	return issues
}

// extractTextContent returns the markdown text from the first TextContent
// in a CallToolResult, or empty string if not found.
func extractTextContent(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// ---------- Markdown audit test fixtures ----------.

// markdownFixture represents a result type to test through the markdown formatter.
type markdownFixture struct {
	name   string
	result any
}

// allMarkdownFixtures returns zero-value or minimally populated instances
// for every type dispatched by markdownForResult.
func allMarkdownFixtures() []markdownFixture {
	return []markdownFixture{
		// nil → success confirmation
		{"nil_result", nil},

		// Projects
		{"projects.Output", projects.Output{ID: 1, Name: "test-project"}},
		{"projects.ListOutput", projects.ListOutput{Projects: []projects.Output{{ID: 1, Name: "p"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"projects.DeleteOutput", projects.DeleteOutput{Status: "ok", Message: "deleted"}},
		{"projects.ListForksOutput", projects.ListForksOutput{Forks: []projects.Output{{ID: 2}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"projects.LanguagesOutput", projects.LanguagesOutput{Languages: []projects.LanguageEntry{{Name: "Go", Percentage: 100.0}}}},
		{"projects.ListHooksOutput", projects.ListHooksOutput{Hooks: []projects.HookOutput{{ID: 1, URL: "https://hook"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"projects.HookOutput", projects.HookOutput{ID: 1, URL: "https://hook"}},
		{"projects.ListProjectUsersOutput", projects.ListProjectUsersOutput{Users: []projects.ProjectUserOutput{{ID: 1, Username: "u"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"projects.ListProjectGroupsOutput", projects.ListProjectGroupsOutput{Groups: []projects.ProjectGroupOutput{{ID: 1, Name: "g"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"projects.ListProjectStarrersOutput", projects.ListProjectStarrersOutput{Starrers: []projects.StarrerOutput{{User: projects.ProjectUserOutput{ID: 1, Username: "u"}}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"projects.PushRuleOutput", projects.PushRuleOutput{ID: 1}},

		// Uploads
		{"uploads.UploadOutput", uploads.UploadOutput{Alt: "file.txt", URL: "/uploads/file.txt", FullPath: "/uploads/file.txt", Markdown: "![file](url)"}},

		// Branches
		{"branches.Output", branches.Output{Name: "main", Merged: false}},
		{"branches.ListOutput", branches.ListOutput{Branches: []branches.Output{{Name: "main"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"branches.ProtectedOutput", branches.ProtectedOutput{Name: "main"}},
		{"branches.ProtectedListOutput", branches.ProtectedListOutput{Branches: []branches.ProtectedOutput{{Name: "main"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Tags
		{"tags.Output", tags.Output{Name: "v1.0.0"}},
		{"tags.ListOutput", tags.ListOutput{Tags: []tags.Output{{Name: "v1.0.0"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"tags.SignatureOutput", tags.SignatureOutput{VerificationStatus: "verified"}},
		{"tags.ProtectedTagOutput", tags.ProtectedTagOutput{Name: "v*"}},
		{"tags.ListProtectedTagsOutput", tags.ListProtectedTagsOutput{Tags: []tags.ProtectedTagOutput{{Name: "v*"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Releases
		{"releases.Output", releases.Output{TagName: "v1.0.0", Name: "Release 1"}},
		{"releases.ListOutput", releases.ListOutput{Releases: []releases.Output{{TagName: "v1.0.0"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Release Links
		{"releaselinks.Output", releaselinks.Output{ID: 1, Name: "binary", URL: "https://dl"}},
		{"releaselinks.CreateBatchOutput", releaselinks.CreateBatchOutput{Created: []releaselinks.Output{{ID: 1, Name: "bin", URL: "https://dl"}}}},
		{"releaselinks.ListOutput", releaselinks.ListOutput{Links: []releaselinks.Output{{ID: 1, Name: "bin"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Commits
		{"commits.Output", commits.Output{ShortID: "abc1234", Title: "fix: thing"}},
		{"commits.ListOutput", commits.ListOutput{Commits: []commits.Output{{ShortID: "abc"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"commits.DetailOutput", commits.DetailOutput{ShortID: "abc", Title: "t", WebURL: "u"}},
		{"commits.DiffOutput", commits.DiffOutput{Diffs: []toolutil.DiffOutput{{NewPath: "f"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"commits.RefsOutput", commits.RefsOutput{Refs: []commits.RefOutput{{Type: "branch", Name: "main"}}}},
		{"commits.CommentsOutput", commits.CommentsOutput{Comments: []commits.CommentOutput{{Note: "text"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"commits.CommentOutput", commits.CommentOutput{Note: "text"}},
		{"commits.StatusesOutput", commits.StatusesOutput{Statuses: []commits.StatusOutput{{Status: "success"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"commits.StatusOutput", commits.StatusOutput{Status: "success"}},
		{"commits.MRsByCommitOutput", commits.MRsByCommitOutput{MergeRequests: []commits.BasicMROutput{{IID: 1}}}},
		{"commits.GPGSignatureOutput", commits.GPGSignatureOutput{VerificationStatus: "verified"}},

		// Files
		{"files.Output", files.Output{FileName: "main.go", FilePath: "src/main.go"}},
		{"files.FileInfoOutput", files.FileInfoOutput{FilePath: "src/main.go", Branch: "main"}},
		{"files.BlameOutput", files.BlameOutput{FilePath: "main.go", Ranges: []files.BlameRangeOutput{{Commit: files.BlameRangeCommitOutput{ID: "abc"}, Lines: []string{"line1"}}}}},
		{"files.MetaDataOutput", files.MetaDataOutput{FileName: "f", Size: 100}},
		{"files.RawOutput", files.RawOutput{FilePath: "f", Content: "content"}},

		// Wikis
		{"wikis.Output", wikis.Output{Title: "Home", Slug: "home"}},
		{"wikis.ListOutput", wikis.ListOutput{WikiPages: []wikis.Output{{Title: "Home"}}}},
		{"wikis.AttachmentOutput", wikis.AttachmentOutput{FileName: "img.png", FilePath: "uploads/img.png"}},

		// Todos
		{"todos.Output", todos.Output{ID: 1, ActionName: "assigned"}},
		{"todos.ListOutput", todos.ListOutput{Todos: []todos.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"todos.MarkDoneOutput", todos.MarkDoneOutput{ID: 1, Message: "done"}},
		{"todos.MarkAllDoneOutput", todos.MarkAllDoneOutput{Message: "2 todos marked done"}},

		// Merge Requests
		{"mergerequests.Output", mergerequests.Output{IID: 1, Title: "MR title"}},
		{"mergerequests.ListOutput", mergerequests.ListOutput{MergeRequests: []mergerequests.Output{{IID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"mergerequests.ApproveOutput", mergerequests.ApproveOutput{Approved: true, ApprovedBy: 1}},
		{"mergerequests.CommitsOutput", mergerequests.CommitsOutput{Commits: []commits.Output{{ShortID: "a"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"mergerequests.PipelinesOutput", mergerequests.PipelinesOutput{Pipelines: []pipelines.Output{{ID: 1}}}},
		{"mergerequests.RebaseOutput", mergerequests.RebaseOutput{RebaseInProgress: true}},
		{"mergerequests.ParticipantsOutput", mergerequests.ParticipantsOutput{Participants: []mergerequests.ParticipantOutput{{Username: "u"}}}},
		{"mergerequests.ReviewersOutput", mergerequests.ReviewersOutput{Reviewers: []mergerequests.ReviewerOutput{{Username: "u"}}}},
		{"mergerequests.IssuesClosedOutput", mergerequests.IssuesClosedOutput{Issues: []issues.Output{{IID: 1}}}},
		{"mergerequests.TimeStatsOutput", mergerequests.TimeStatsOutput{HumanTimeEstimate: "1h"}},
		{"mergerequests.RelatedIssuesOutput", mergerequests.RelatedIssuesOutput{Issues: []issues.Output{{IID: 1}}}},
		{"mergerequests.CreateTodoOutput", mergerequests.CreateTodoOutput{ID: 1, ActionName: "marked"}},
		{"mergerequests.DependencyOutput", mergerequests.DependencyOutput{ID: 1, BlockingMRIID: 2}},
		{"mergerequests.DependenciesOutput", mergerequests.DependenciesOutput{Dependencies: []mergerequests.DependencyOutput{{ID: 1}}}},

		// MR Notes
		{"mrnotes.Output", mrnotes.Output{ID: 1, Body: "note text"}},
		{"mrnotes.ListOutput", mrnotes.ListOutput{Notes: []mrnotes.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// MR Discussions
		{"mrdiscussions.Output", mrdiscussions.Output{ID: "abc"}},
		{"mrdiscussions.ListOutput", mrdiscussions.ListOutput{Discussions: []mrdiscussions.Output{{ID: "abc"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"mrdiscussions.NoteOutput", mrdiscussions.NoteOutput{ID: 1, Body: "note"}},

		// MR Changes
		{"mrchanges.Output", mrchanges.Output{MRIID: 1}},
		{"mrchanges.DiffVersionsListOutput", mrchanges.DiffVersionsListOutput{DiffVersions: []mrchanges.DiffVersionOutput{{ID: 1}}}},
		{"mrchanges.DiffVersionOutput", mrchanges.DiffVersionOutput{ID: 1}},
		{"mrchanges.RawDiffsOutput", mrchanges.RawDiffsOutput{MRIID: 1, RawDiff: "diff content"}},

		// MR Approvals
		{"mrapprovals.StateOutput", mrapprovals.StateOutput{Rules: []mrapprovals.RuleOutput{{ID: 1, Name: "rule"}}}},
		{"mrapprovals.RulesOutput", mrapprovals.RulesOutput{Rules: []mrapprovals.RuleOutput{{ID: 1, Name: "rule"}}}},
		{"mrapprovals.ConfigOutput", mrapprovals.ConfigOutput{ApprovalsBeforeMerge: 1}},
		{"mrapprovals.RuleOutput", mrapprovals.RuleOutput{ID: 1, Name: "rule"}},

		// MR Draft Notes
		{"mrdraftnotes.Output", mrdraftnotes.Output{ID: 1, Note: "draft"}},
		{"mrdraftnotes.ListOutput", mrdraftnotes.ListOutput{DraftNotes: []mrdraftnotes.Output{{ID: 1}}}},

		// Issues
		{"issues.Output", issues.Output{IID: 1, Title: "Bug"}},
		{"issues.ListOutput", issues.ListOutput{Issues: []issues.Output{{IID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"issues.TodoOutput", issues.TodoOutput{ID: 1}},
		{"issues.TimeStatsOutput", issues.TimeStatsOutput{HumanTimeEstimate: "1h"}},
		{"issues.ParticipantsOutput", issues.ParticipantsOutput{Participants: []issues.ParticipantOutput{{Username: "u"}}}},
		{"issues.RelatedMRsOutput", issues.RelatedMRsOutput{MergeRequests: []issues.RelatedMROutput{{IID: 1}}}},
		{"issues.ListGroupOutput", issues.ListGroupOutput{Issues: []issues.Output{{IID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Issue Notes
		{"issuenotes.Output", issuenotes.Output{ID: 1, Body: "note"}},
		{"issuenotes.ListOutput", issuenotes.ListOutput{Notes: []issuenotes.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Issue Links
		{"issuelinks.Output", issuelinks.Output{ID: 1, SourceIssueIID: 10, TargetIssueIID: 20, LinkType: "relates_to"}},
		{"issuelinks.ListOutput", issuelinks.ListOutput{Relations: []issuelinks.RelationOutput{{ID: 1, IID: 10, Title: "issue", LinkType: "relates_to"}}}},

		// Members
		{"members.Output", members.Output{ID: 1, Username: "u"}},
		{"members.ListOutput", members.ListOutput{Members: []members.Output{{ID: 1, Username: "u"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Groups
		{"groups.Output", groups.Output{ID: 1, Name: "group"}},
		{"groups.ListOutput", groups.ListOutput{Groups: []groups.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"groups.MemberListOutput", groups.MemberListOutput{Members: []groups.MemberOutput{{ID: 1, Username: "u"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"groups.ListProjectsOutput", groups.ListProjectsOutput{Projects: []groups.ProjectItem{{ID: 1, Name: "proj"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"groups.HookOutput", groups.HookOutput{ID: 1, URL: "https://hook"}},
		{"groups.HookListOutput", groups.HookListOutput{Hooks: []groups.HookOutput{{ID: 1, URL: "u"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Users
		{"users.Output", users.Output{ID: 1, Username: "u"}},
		{"users.ListOutput", users.ListOutput{Users: []users.Output{{ID: 1}}}},
		{"users.StatusOutput", users.StatusOutput{Message: "busy"}},
		{"users.SSHKeyListOutput", users.SSHKeyListOutput{Keys: []users.SSHKeyOutput{{ID: 1, Title: "key"}}}},
		{"users.EmailListOutput", users.EmailListOutput{Emails: []users.EmailOutput{{ID: 1, Email: "a@b.com"}}}},
		{"users.ContributionEventsOutput", users.ContributionEventsOutput{Events: []users.ContributionEventOutput{{ActionName: "pushed"}}}},
		{"users.AssociationsCountOutput", users.AssociationsCountOutput{ProjectsCount: 5}},

		// Health
		{"health.Output", health.Output{Status: "ok", GitLabVersion: "17.0.0"}},

		// Labels
		{"labels.Output", labels.Output{Name: "bug", Color: "#ff0000"}},
		{"labels.ListOutput", labels.ListOutput{Labels: []labels.Output{{Name: "bug"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Milestones
		{"milestones.Output", milestones.Output{IID: 1, Title: "v1"}},
		{"milestones.ListOutput", milestones.ListOutput{Milestones: []milestones.Output{{IID: 1, Title: "v1"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"milestones.MilestoneIssuesOutput", milestones.MilestoneIssuesOutput{Issues: []milestones.IssueItem{{ID: 1, IID: 1, Title: "issue"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"milestones.MilestoneMergeRequestsOutput", milestones.MilestoneMergeRequestsOutput{MergeRequests: []milestones.MergeRequestItem{{ID: 1, IID: 1, Title: "mr"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Pipelines
		{"pipelines.ListOutput", pipelines.ListOutput{Pipelines: []pipelines.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"pipelines.DetailOutput", pipelines.DetailOutput{ID: 1, Status: "success", WebURL: "u"}},
		{"pipelines.VariablesOutput", pipelines.VariablesOutput{Variables: []pipelines.VariableOutput{{Key: "K", Value: "V"}}}},
		{"pipelines.TestReportOutput", pipelines.TestReportOutput{TotalCount: 10}},
		{"pipelines.TestReportSummaryOutput", pipelines.TestReportSummaryOutput{TotalCount: 10}},

		// Pipeline Schedules
		{"pipelineschedules.Output", pipelineschedules.Output{ID: 1, Description: "nightly"}},
		{"pipelineschedules.ListOutput", pipelineschedules.ListOutput{Schedules: []pipelineschedules.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"pipelineschedules.VariableOutput", pipelineschedules.VariableOutput{Key: "K", Value: "V"}},
		{"pipelineschedules.TriggeredPipelinesListOutput", pipelineschedules.TriggeredPipelinesListOutput{Pipelines: []pipelineschedules.TriggeredPipelineOutput{{ID: 1, Status: "success"}}}},

		// CI Variables
		{"civariables.Output", civariables.Output{Key: "K", Value: "V"}},
		{"civariables.ListOutput", civariables.ListOutput{Variables: []civariables.Output{{Key: "K"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// CI Lint
		{"cilint.Output", cilint.Output{Valid: true}},

		// Jobs
		{"jobs.Output", jobs.Output{ID: 1, Name: "build", Stage: "build", Status: "success", WebURL: "u"}},
		{"jobs.ListOutput", jobs.ListOutput{Jobs: []jobs.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"jobs.TraceOutput", jobs.TraceOutput{JobID: 1, Trace: "log output"}},
		{"jobs.BridgeListOutput", jobs.BridgeListOutput{Bridges: []jobs.BridgeOutput{{ID: 1, Name: "b"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"jobs.ArtifactsOutput", jobs.ArtifactsOutput{JobID: 1, Size: 1024, Content: "base64data"}},
		{"jobs.SingleArtifactOutput", jobs.SingleArtifactOutput{JobID: 1, ArtifactPath: "report.json", Size: 512}},

		// Search
		{"search.CodeOutput", search.CodeOutput{Blobs: []search.BlobOutput{{Filename: "f"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"search.MergeRequestsOutput", search.MergeRequestsOutput{MergeRequests: []mergerequests.Output{{IID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Sampling
		{"samplingtools.AnalyzeMRChangesOutput", samplingtools.AnalyzeMRChangesOutput{MRIID: 1, Analysis: "looks good"}},
		{"samplingtools.SummarizeIssueOutput", samplingtools.SummarizeIssueOutput{IssueIID: 1, Summary: "summary text"}},

		// Environments
		{"environments.Output", environments.Output{ID: 1, Name: "production"}},
		{"environments.ListOutput", environments.ListOutput{Environments: []environments.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Deployments
		{"deployments.Output", deployments.Output{ID: 1, Status: "success"}},
		{"deployments.ListOutput", deployments.ListOutput{Deployments: []deployments.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"deployments.ApproveOrRejectOutput", deployments.ApproveOrRejectOutput{Message: "approved"}},

		// Runners
		{"runners.Output", runners.Output{ID: 1, Description: "runner"}},
		{"runners.DetailsOutput", runners.DetailsOutput{ID: 1, Description: "runner"}},
		{"runners.ListOutput", runners.ListOutput{Runners: []runners.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Access Tokens
		{"accesstokens.Output", accesstokens.Output{ID: 1, Name: "token"}},
		{"accesstokens.ListOutput", accesstokens.ListOutput{Tokens: []accesstokens.Output{{ID: 1}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},

		// Repository
		{"repository.TreeOutput", repository.TreeOutput{Tree: []repository.TreeNodeOutput{{Name: "f"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"repository.CompareOutput", repository.CompareOutput{Commits: []commits.Output{{ShortID: "a"}}}},
		{"repository.ContributorsOutput", repository.ContributorsOutput{Contributors: []repository.ContributorOutput{{Name: "dev"}}}},
		{"repository.BlobOutput", repository.BlobOutput{SHA: "abc", Size: 100}},
		{"repository.RawBlobContentOutput", repository.RawBlobContentOutput{SHA: "abc", Content: "data"}},
		{"repository.ArchiveOutput", repository.ArchiveOutput{Format: "tar.gz", URL: "https://archive"}},
		{"repository.AddChangelogOutput", repository.AddChangelogOutput{Success: true, Version: "1.0.0"}},
		{"repository.ChangelogDataOutput", repository.ChangelogDataOutput{Notes: "changelog data"}},

		// Delete (internal type)
		{"DeleteOutput", toolutil.DeleteOutput{Message: "Resource deleted"}},

		// Packages
		{"packages.PublishOutput", packages.PublishOutput{PackageFileID: 1, PackageID: 10, FileName: "app.tar.gz", Size: 1024, SHA256: "abc123", URL: "https://pkg"}},
		{"packages.DownloadOutput", packages.DownloadOutput{OutputPath: "/tmp/app.tar.gz", Size: 1024, SHA256: "abc123"}},
		{"packages.ListOutput", packages.ListOutput{Packages: []packages.ListItem{{ID: 1, Name: "app", Version: "1.0.0", PackageType: "generic", Status: "default"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"packages.FileListOutput", packages.FileListOutput{Files: []packages.FileListItem{{PackageFileID: 1, PackageID: 10, FileName: "app.tar.gz", Size: 1024, SHA256: "abc123"}}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}},
		{"packages.PublishAndLinkOutput", packages.PublishAndLinkOutput{Package: packages.PublishOutput{PackageFileID: 1, FileName: "app.tar.gz", URL: "https://pkg"}, ReleaseLink: releaselinks.Output{ID: 1, Name: "app", URL: "https://dl"}}},
		{"packages.PublishDirOutput", packages.PublishDirOutput{Published: []packages.PublishDirItem{{FileName: "f.tar.gz", PackageFileID: 1, Size: 512, URL: "https://pkg"}}, TotalFiles: 1, TotalBytes: 512}},
	}
}

// ---------- Structural audit tests ----------.

// TestMarkdownAudit_DispatchCoverage verifies that every type dispatched
// by markdownForResult returns a non-nil CallToolResult with TextContent.
// Types not dispatched are logged as audit findings (not hard failures).
func TestMarkdownAudit_DispatchCoverage(t *testing.T) {
	var missing []string
	for _, fix := range allMarkdownFixtures() {
		t.Run(fix.name, func(t *testing.T) {
			result := markdownForResult(fix.result)
			if result == nil {
				missing = append(missing, fix.name)
				t.Logf("FINDING: markdownForResult returned nil — type not dispatched")
				return
			}
			if len(result.Content) == 0 {
				t.Error("CallToolResult has empty Content array")
			}
			md := extractTextContent(result)
			if md == "" {
				t.Error("TextContent.Text is empty")
			}
		})
	}
	if len(missing) > 0 {
		t.Logf("AUDIT SUMMARY: %d types lack markdown dispatch: %v", len(missing), missing)
	}
}

// TestMarkdownAudit_TableStructure verifies that markdown tables produced
// by all dispatched formatters have consistent column counts across header,
// separator, and data rows.
func TestMarkdownAudit_TableStructure(t *testing.T) {
	for _, fix := range allMarkdownFixtures() {
		t.Run(fix.name, func(t *testing.T) {
			result := markdownForResult(fix.result)
			if result == nil {
				t.Skip("nil result, not a markdown producer")
			}
			md := extractTextContent(result)
			if md == "" {
				t.Skip("empty markdown")
			}

			issues := validateMarkdownTables(md)
			for _, issue := range issues {
				t.Errorf("table issue at line %d: %s", issue.lineNum, issue.description)
			}
		})
	}
}

// TestMarkdownAudit_NoTrailingWhitespace checks that no markdown line
// ends with trailing spaces or tabs. Findings are logged, not hard failures.
func TestMarkdownAudit_NoTrailingWhitespace(t *testing.T) {
	var withIssues []string
	for _, fix := range allMarkdownFixtures() {
		t.Run(fix.name, func(t *testing.T) {
			result := markdownForResult(fix.result)
			if result == nil {
				t.Skip("nil result")
			}
			md := extractTextContent(result)
			lines := strings.Split(md, "\n")
			count := 0
			for i, line := range lines {
				if line != strings.TrimRight(line, " \t") {
					count++
					t.Logf("FINDING: line %d has trailing whitespace: %q", i+1, line)
				}
			}
			if count > 0 {
				withIssues = append(withIssues, fix.name)
				t.Logf("FINDING: %d lines with trailing whitespace", count)
			}
		})
	}
	if len(withIssues) > 0 {
		t.Logf("AUDIT SUMMARY: %d types have trailing whitespace: %v", len(withIssues), withIssues)
	}
}

// TestMarkdownAudit_NoEmptySections checks that markdown does not contain
// headers immediately followed by another header (empty section).
func TestMarkdownAudit_NoEmptySections(t *testing.T) {
	for _, fix := range allMarkdownFixtures() {
		t.Run(fix.name, func(t *testing.T) {
			result := markdownForResult(fix.result)
			if result == nil {
				t.Skip("nil result")
			}
			md := extractTextContent(result)
			lines := strings.Split(md, "\n")
			for i := range len(lines) - 1 {
				curr := strings.TrimSpace(lines[i])
				next := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(curr, "#") && strings.HasPrefix(next, "#") {
					t.Errorf("empty section at line %d: %q followed by %q", i+1, curr, next)
				}
			}
		})
	}
}

// ---------- Validator unit tests ----------.

// TestIsTable_Separator validates is table separator across multiple scenarios using table-driven subtests.
func TestIsTable_Separator(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"| --- | --- | --- |", true},
		{"| :--- | :---: | ---: |", true},
		{"| --- |", true},
		{"| data | data | data |", false},
		{"not a table", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			if got := isTableSeparator(tc.line); got != tc.want {
				t.Errorf("isTableSeparator(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

// TestValidateMarkdown_Tables verifies the behavior of validate markdown tables.
func TestValidateMarkdown_Tables(t *testing.T) {
	t.Run("consistent table passes", func(t *testing.T) {
		md := "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n"
		issues := validateMarkdownTables(md)
		if len(issues) != 0 {
			t.Errorf("expected no issues, got %d: %v", len(issues), issues)
		}
	})

	t.Run("inconsistent column count detected", func(t *testing.T) {
		md := "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 |\n"
		issues := validateMarkdownTables(md)
		if len(issues) == 0 {
			t.Error("expected column mismatch issue")
		}
	})
}
