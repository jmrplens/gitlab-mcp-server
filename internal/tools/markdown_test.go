// markdown_test.go contains unit tests for every Markdown formatter function
// in markdown.go. Each test verifies that the rendered output contains expected
// headings, field values, table rows, and empty-state messages.
package tools

import (
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/serverupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	errExpNonNilResult = "expected non-nil result"
	errMissingHeader   = "missing header"
	errMissingEmptyMsg = "missing empty message"
	testDate20260101   = "2026-01-01"
	mdDescriptionHdr   = "### Description"
	fmtMissing         = "missing %q"
	testTitleAddFeat   = "Add feature"
	testTitleFixBug    = "Fix bug"
	testFileSrcMainGo  = "src/main.go"
	testTitleBugReport = "Bug report"
	testEmojiQuestion  = "\u2753"
)

// TestMarkdownForResult verifies the registry-based dispatcher returns a success
// result for nil (void actions), nil for unknown types, and dispatches known
// output types correctly.
func TestMarkdownForResult(t *testing.T) {
	t.Run("nil result returns success", func(t *testing.T) {
		result := markdownForResult(nil)
		if result == nil {
			t.Fatal("expected non-nil success result for void actions")
		}
		if len(result.Content) == 0 {
			t.Fatal("expected content in success result")
		}
	})

	t.Run("unknown type returns nil", func(t *testing.T) {
		if markdownForResult("unexpected string") != nil {
			t.Fatal("expected nil for unknown type")
		}
	})
}

// TestFormatProject_Markdown verifies that all project fields appear in the
// rendered Markdown output.
func TestFormatProject_Markdown(t *testing.T) {
	p := projects.Output{
		ID: 42, Name: "my-project", PathWithNamespace: "group/my-project",
		Visibility: "private", DefaultBranch: "main",
		WebURL: "https://gitlab.example.com/group/my-project", Description: "A test project",
	}
	md := projects.FormatMarkdown(p)

	checks := []string{
		"## Project: my-project", "**ID**: 42", "**Path**: group/my-project",
		"**Visibility**: private", "**Default Branch**: main",
		"**Description**: A test project", "**URL**: [https://gitlab.example.com/group/my-project](https://gitlab.example.com/group/my-project)",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("missing %q in:\n%s", c, md)
		}
	}
}

// TestFormatProject_ListMarkdown verifies table rendering for project lists
// and the empty-state message.
func TestFormatProject_ListMarkdown(t *testing.T) {
	t.Run("with projects", func(t *testing.T) {
		out := projects.ListOutput{
			Projects: []projects.Output{
				{ID: 1, Name: "alpha", PathWithNamespace: "g/alpha", Visibility: "public"},
				{ID: 2, Name: "beta", PathWithNamespace: "g/beta", Visibility: "private"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2, PerPage: 20},
		}
		md := projects.FormatListMarkdown(out)
		if !strings.Contains(md, "## Projects (2)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[alpha]") {
			t.Error("missing project row")
		}
		if !strings.Contains(md, "Page 1 of 1") {
			t.Error("missing pagination")
		}
	})

	t.Run("empty list", func(t *testing.T) {
		md := projects.FormatListMarkdown(projects.ListOutput{})
		if !strings.Contains(md, "No projects found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatBranch_Markdown verifies that branch fields appear in Markdown output.
func TestFormatBranch_Markdown(t *testing.T) {
	br := branches.Output{Name: "feature-x", Protected: true, Default: false, Merged: false, CommitID: "abc123"}
	md := branches.FormatOutputMarkdown(br)

	if !strings.Contains(md, "## Branch: feature-x") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Protected**: true") {
		t.Error("missing protected field")
	}
	if !strings.Contains(md, "**Commit**: abc123") {
		t.Error("missing commit")
	}
}

// TestFormatBranch_ListMarkdown verifies table rendering for branch lists.
func TestFormatBranch_ListMarkdown(t *testing.T) {
	out := branches.ListOutput{
		Branches:   []branches.Output{{Name: "main", Protected: true, Default: true}},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := branches.FormatListMarkdown(out)
	if !strings.Contains(md, "| main | true | true |") {
		t.Error("missing branch row")
	}
}

// TestFormatProtected_BranchMarkdown verifies protected branch fields in Markdown.
func TestFormatProtected_BranchMarkdown(t *testing.T) {
	pb := branches.ProtectedOutput{ID: 1, Name: "main", PushAccessLevel: 40, MergeAccessLevel: 30, AllowForcePush: false}
	md := branches.FormatProtectedMarkdown(pb)

	if !strings.Contains(md, "## Protected Branch: main") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Push Access Level**: 40") {
		t.Error("missing push level")
	}
}

// TestFormatProtected_BranchesListMarkdown verifies the empty-state message
// for protected branch lists.
func TestFormatProtected_BranchesListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := branches.FormatProtectedListMarkdown(branches.ProtectedListOutput{})
		if !strings.Contains(md, "No protected branches found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatTag_Markdown verifies that tag fields appear in Markdown output.
func TestFormatTag_Markdown(t *testing.T) {
	tag := tags.Output{Name: "v1.0.0", Target: "abc123", Protected: false, Message: "Release v1"}
	md := tags.FormatOutputMarkdownString(tag)

	if !strings.Contains(md, "## Tag: v1.0.0") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Message**: Release v1") {
		t.Error("missing message")
	}
}

// TestFormatTag_ListMarkdown verifies table rendering for tag lists.
func TestFormatTag_ListMarkdown(t *testing.T) {
	out := tags.ListOutput{
		Tags:       []tags.Output{{Name: "v1.0", Target: "abc", Protected: true}},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := tags.FormatListMarkdownString(out)
	if !strings.Contains(md, "| v1.0 | abc | true |") {
		t.Error("missing tag row")
	}
}

// TestFormatRelease_Markdown verifies that release fields and description
// section appear in Markdown output.
func TestFormatRelease_Markdown(t *testing.T) {
	r := releases.Output{TagName: "v1.0", Name: "Version 1.0", Description: "Features", CreatedAt: testDate20260101, ReleasedAt: "2026-01-02"}
	md := releases.FormatMarkdown(r)

	if !strings.Contains(md, "## Release: Version 1.0") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, mdDescriptionHdr) {
		t.Error("missing description section")
	}
}

// TestFormatRelease_ListMarkdown verifies the empty-state message for release lists.
func TestFormatRelease_ListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := releases.FormatListMarkdown(releases.ListOutput{})
		if !strings.Contains(md, "No releases found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatRelease_LinkMarkdown verifies release link fields in Markdown output.
func TestFormatRelease_LinkMarkdown(t *testing.T) {
	l := releaselinks.Output{ID: 1, Name: "binary", URL: "https://example.com/bin", LinkType: "package", External: false}
	md := releaselinks.FormatOutputMarkdown(l)

	if !strings.Contains(md, "## Release Link: binary") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Type**: package") {
		t.Error("missing type")
	}
}

// TestFormatRelease_LinkListMarkdown verifies the empty-state message for link lists.
func TestFormatRelease_LinkListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := releaselinks.FormatListMarkdown(releaselinks.ListOutput{})
		if !strings.Contains(md, "No release links found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatMR_Markdown verifies that all merge request fields appear in Markdown.
func TestFormatMR_Markdown(t *testing.T) {
	mr := mergerequests.Output{
		ID: 1, IID: 15, Title: testTitleAddFeat, State: "opened",
		SourceBranch: "feature", TargetBranch: "main",
		MergeStatus: "can_be_merged", Description: "Adds a feature",
		WebURL: "https://gitlab.example.com/mr/15",
		Author: "dev1", Labels: []string{"enhancement"},
		Assignees: []string{"dev2"}, Reviewers: []string{"dev3"},
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	md := mergerequests.FormatMarkdown(mr)

	checks := []string{
		"MR !15: Add feature", "opened",
		"**Source**: feature", "**Target**: main",
		mdDescriptionHdr, "Adds a feature",
		"@dev1", "enhancement", "@dev2", "@dev3",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf(fmtMissing, c)
		}
	}
}

// TestFormatMRMarkdownDraft_Conflicts verifies draft and conflict indicators.
func TestFormatMRMarkdownDraft_Conflicts(t *testing.T) {
	mr := mergerequests.Output{
		IID: 99, Title: "WIP", State: "opened",
		SourceBranch: "wip", TargetBranch: "main",
		MergeStatus: "cannot_be_merged", Draft: true, HasConflicts: true,
		WebURL: "https://gitlab.example.com/mr/99",
	}
	md := mergerequests.FormatMarkdown(mr)
	if !strings.Contains(md, "Draft") {
		t.Error("missing draft indicator")
	}
	if !strings.Contains(md, "Conflicts") {
		t.Error("missing conflict indicator")
	}
}

// TestFormatMR_ListMarkdown verifies table rendering for merge request lists.
func TestFormatMR_ListMarkdown(t *testing.T) {
	out := mergerequests.ListOutput{
		MergeRequests: []mergerequests.Output{
			{IID: 1, Title: testTitleFixBug, State: "merged", SourceBranch: "fix", TargetBranch: "main", Author: "dev"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := mergerequests.FormatListMarkdown(out)
	if !strings.Contains(md, testTitleFixBug) || !strings.Contains(md, "merged") {
		t.Error("missing MR row content")
	}
	if !strings.Contains(md, "Author") {
		t.Error("missing Author column header")
	}
}

// TestFormatMR_ApproveMarkdown verifies MR approval status fields in Markdown.
func TestFormatMR_ApproveMarkdown(t *testing.T) {
	a := mergerequests.ApproveOutput{ApprovalsRequired: 2, ApprovedBy: 1, Approved: false}
	md := mergerequests.FormatApproveMarkdown(a)

	if !strings.Contains(md, "**Approved**: false") {
		t.Error("missing approved field")
	}
	if !strings.Contains(md, "**Approvals Required**: 2") {
		t.Error("missing required field")
	}
}

// TestFormatMR_NoteMarkdown verifies MR note fields in Markdown and that
// non-system notes do not include the system note marker.
func TestFormatMR_NoteMarkdown(t *testing.T) {
	n := mrnotes.Output{ID: 10, Body: "LGTM", Author: "dev", CreatedAt: testDate20260101, System: false}
	md := mrnotes.FormatOutputMarkdown(n)

	if !strings.Contains(md, "## MR Note #10") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "LGTM") {
		t.Error("missing body")
	}
	if strings.Contains(md, "System note") {
		t.Error("should not contain system note marker")
	}
}

// TestFormatMR_NotesListMarkdown verifies the empty-state message for MR note lists.
func TestFormatMR_NotesListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := mrnotes.FormatListMarkdown(mrnotes.ListOutput{})
		if !strings.Contains(md, "No merge request notes found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatDiscussion_NoteMarkdown verifies discussion note fields in Markdown.
func TestFormatDiscussion_NoteMarkdown(t *testing.T) {
	n := mrdiscussions.NoteOutput{ID: 5, Body: "Needs fix", Author: "reviewer", CreatedAt: testDate20260101, Resolved: false}
	md := mrdiscussions.FormatNoteMarkdown(n)

	if !strings.Contains(md, "## Discussion Note #5") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Resolved**: false") {
		t.Error("missing resolved field")
	}
}

// TestFormatMR_DiscussionMarkdown verifies that a discussion thread with
// multiple notes renders each note as a sub-heading.
func TestFormatMR_DiscussionMarkdown(t *testing.T) {
	d := mrdiscussions.Output{
		ID:             "abc123",
		IndividualNote: false,
		Notes: []mrdiscussions.NoteOutput{
			{ID: 1, Body: "First note", Author: "dev1"},
			{ID: 2, Body: "Reply", Author: "dev2"},
		},
	}
	md := mrdiscussions.FormatOutputMarkdown(d)

	if !strings.Contains(md, "## Discussion abc123") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "### Note 1 (by dev1)") {
		t.Error("missing first note")
	}
	if !strings.Contains(md, "### Note 2 (by dev2)") {
		t.Error("missing second note")
	}
}

// TestFormatMR_DiscussionListMarkdown verifies the empty-state message for
// discussion lists.
func TestFormatMR_DiscussionListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := mrdiscussions.FormatListMarkdown(mrdiscussions.ListOutput{})
		if !strings.Contains(md, "No merge request discussions found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatMR_ChangesMarkdown verifies that file change statuses (modified,
// added, deleted, renamed) are correctly rendered in the Markdown table.
func TestFormatMR_ChangesMarkdown(t *testing.T) {
	out := mrchanges.Output{
		MRIID: 15,
		Changes: []mrchanges.FileDiffOutput{
			{OldPath: "a.go", NewPath: "a.go", NewFile: false, DeletedFile: false, RenamedFile: false},
			{OldPath: "", NewPath: "b.go", NewFile: true},
			{OldPath: "c.go", NewPath: "c.go", DeletedFile: true},
			{OldPath: "old.go", NewPath: "new.go", RenamedFile: true},
		},
	}
	md := mrchanges.FormatOutputMarkdown(out)

	if !strings.Contains(md, "## MR !15 Changes (4 files)") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "| a.go | modified |") {
		t.Error("missing modified file")
	}
	if !strings.Contains(md, "| b.go | added |") {
		t.Error("missing added file")
	}
	if !strings.Contains(md, "| c.go | deleted |") {
		t.Error("missing deleted file")
	}
	if !strings.Contains(md, "renamed from old.go") {
		t.Error("missing renamed info")
	}
}

// TestFormatCommit_Markdown verifies that commit fields appear in Markdown.
func TestFormatCommit_Markdown(t *testing.T) {
	c := commits.Output{
		ID: "abc123full", ShortID: "abc123", Title: testTitleFixBug,
		AuthorName: "Dev", AuthorEmail: "dev@example.com",
		CommittedDate: testDate20260101, WebURL: "https://gitlab.example.com/commit/abc123",
	}
	md := commits.FormatOutputMarkdown(c)

	if !strings.Contains(md, "## Commit abc123") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Author**: Dev <dev@example.com>") {
		t.Error("missing author")
	}
}

// TestFormatFile_Markdown verifies that file metadata fields appear in Markdown.
func TestFormatFile_Markdown(t *testing.T) {
	f := files.Output{FilePath: testFileSrcMainGo, Size: 1024, Ref: "main", Encoding: "base64", BlobID: "blob123"}
	md := files.FormatOutputMarkdown(f)

	if !strings.Contains(md, "## File: src/main.go") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Size**: 1024 bytes") {
		t.Error("missing size")
	}
}

// TestFormatMember_ListMarkdown verifies table rendering for member lists
// and the empty-state message.
func TestFormatMember_ListMarkdown(t *testing.T) {
	t.Run("with members", func(t *testing.T) {
		out := members.ListOutput{
			Members: []members.Output{
				{Username: "dev1", Name: "Developer One", AccessLevelDescription: "Developer", State: "active"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := members.FormatListMarkdownString(out)
		if !strings.Contains(md, "| dev1 | Developer One | Developer | active |") {
			t.Error("missing member row")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := members.FormatListMarkdownString(members.ListOutput{})
		if !strings.Contains(md, "No members found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatGroup_Markdown verifies that all group fields appear in Markdown output.
func TestFormatGroup_Markdown(t *testing.T) {
	g := groups.Output{
		ID: 10, Name: "my-group", Path: "my-group", FullPath: "org/my-group",
		Visibility: "internal", Description: "Team group",
		WebURL: "https://gitlab.example.com/org/my-group", ParentID: 5,
	}
	md := groups.FormatOutputMarkdown(g)

	checks := []string{
		"## Group: my-group", "**ID**: 10", "**Path**: org/my-group",
		"**Visibility**: internal", "**Description**: Team group",
		"**Parent ID**: 5",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf(fmtMissing, c)
		}
	}
}

// TestFormatGroup_ListMarkdown verifies the empty-state message for group lists.
func TestFormatGroup_ListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := groups.FormatListMarkdown(groups.ListOutput{})
		if !strings.Contains(md, "No groups found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatGroup_MemberListMarkdown verifies table rendering for group member lists.
func TestFormatGroup_MemberListMarkdown(t *testing.T) {
	out := groups.MemberListOutput{
		Members: []groups.MemberOutput{
			{Username: "admin", Name: "Admin User", AccessLevelDescription: "Owner", State: "active"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := groups.FormatMemberListMarkdown(out)
	if !strings.Contains(md, "| admin | Admin User | Owner | active |") {
		t.Error("missing group member row")
	}
}

// TestFormatIssue_Markdown verifies that all issue fields appear in Markdown output.
func TestFormatIssue_Markdown(t *testing.T) {
	i := issues.Output{
		ID: 1, IID: 5, Title: testTitleBugReport, State: "opened",
		Author: "reporter", Labels: []string{"bug", "critical"},
		Assignees: []string{"dev1"}, Milestone: "v1.0",
		DueDate: "2026-03-01", CreatedAt: testDate20260101,
		Description: "Something is broken",
		WebURL:      "https://gitlab.example.com/issues/5",
	}
	md := issues.FormatMarkdown(i)

	checks := []string{
		"Issue #5: Bug report", "opened",
		"**Labels**: bug, critical", "@dev1",
		"**Milestone**: v1.0", "**Due Date**: 1 Mar 2026",
		mdDescriptionHdr, "Something is broken",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf(fmtMissing, c)
		}
	}
}

// TestFormatIssue_MarkdownConfidentialTasks verifies confidential and task progress indicators.
func TestFormatIssue_MarkdownConfidentialTasks(t *testing.T) {
	i := issues.Output{
		IID: 42, Title: "Secret task", State: "opened",
		Author: "admin", Confidential: true,
		TaskCompletionCount: 2, TaskCompletionTotal: 5,
		UserNotesCount: 3,
		WebURL:         "https://gitlab.example.com/issues/42",
	}
	md := issues.FormatMarkdown(i)
	if !strings.Contains(md, "Confidential") {
		t.Error("missing confidential indicator")
	}
	if !strings.Contains(md, "2/5 completed") {
		t.Error("missing task completion progress")
	}
	if !strings.Contains(md, "**Comments**: 3") {
		t.Error("missing comments count")
	}
}

// TestFormatIssue_ListMarkdown verifies table rendering for issue lists.
func TestFormatIssue_ListMarkdown(t *testing.T) {
	out := issues.ListOutput{
		Issues: []issues.Output{
			{IID: 1, Title: "Feature req", State: "opened", Author: "user1", Labels: []string{"enhancement"}},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := issues.FormatListMarkdown(out)
	if !strings.Contains(md, "Feature req") || !strings.Contains(md, "opened") || !strings.Contains(md, "user1") {
		t.Error("missing issue row content")
	}
}

// TestFormatIssue_NoteMarkdown verifies issue note fields and system/internal
// markers in Markdown output.
func TestFormatIssue_NoteMarkdown(t *testing.T) {
	t.Run("regular note", func(t *testing.T) {
		n := issuenotes.Output{ID: 1, Body: "Comment", Author: "user", CreatedAt: testDate20260101, System: false, Internal: false}
		md := issuenotes.FormatOutputMarkdown(n)
		if !strings.Contains(md, "## Issue Note #1") {
			t.Error(errMissingHeader)
		}
		if strings.Contains(md, "System note") || strings.Contains(md, "Internal note") {
			t.Error("regular note should not have system/internal markers")
		}
	})

	t.Run("system internal note", func(t *testing.T) {
		n := issuenotes.Output{ID: 2, Body: "Label added", Author: "bot", CreatedAt: testDate20260101, System: true, Internal: true}
		md := issuenotes.FormatOutputMarkdown(n)
		if !strings.Contains(md, "**System note**") {
			t.Error("missing system marker")
		}
		if !strings.Contains(md, "**Internal note**") {
			t.Error("missing internal marker")
		}
	})
}

// TestFormatIssue_NoteListMarkdown verifies the empty-state message for issue note lists.
func TestFormatIssue_NoteListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := issuenotes.FormatListMarkdown(issuenotes.ListOutput{})
		if !strings.Contains(md, "No issue notes found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatUpload_Markdown verifies that uploaded file fields appear in Markdown.
func TestFormatUpload_Markdown(t *testing.T) {
	u := uploads.UploadOutput{Alt: "screenshot", URL: "/uploads/hash/file.png", FullPath: "/full/path", Markdown: "![screenshot](/uploads/hash/file.png)"}
	md := uploads.FormatUploadMarkdown(u)

	if !strings.Contains(md, "## File Uploaded") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "**Alt**: screenshot") {
		t.Error("missing alt")
	}
}

// TestUploadToolResult_ImageFile verifies that image uploads include a Markdown
// image embed with the full URL in the text content.
func TestUploadToolResult_ImageFile(t *testing.T) {
	u := uploads.UploadOutput{
		Alt:      "screenshot.png",
		URL:      "/uploads/hash/screenshot.png",
		FullPath: "/group/project/uploads/hash/screenshot.png",
		Markdown: "![screenshot.png](/uploads/hash/screenshot.png)",
		FullURL:  "https://gitlab.example.com/group/project/uploads/hash/screenshot.png",
	}
	result := uploads.UploadToolResult(u)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content entry, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "![screenshot.png](https://gitlab.example.com") {
		t.Error("expected Markdown image embed with full URL")
	}
	if !strings.Contains(tc.Text, "## File Uploaded") {
		t.Error("expected file uploaded header in text")
	}
}

// TestUploadToolResult_NonImageFile verifies that non-image uploads do not
// include a Markdown image embed.
func TestUploadToolResult_NonImageFile(t *testing.T) {
	u := uploads.UploadOutput{
		Alt:      "notes.txt",
		URL:      "/uploads/hash/notes.txt",
		FullPath: "/group/project/uploads/hash/notes.txt",
		Markdown: "[notes.txt](/uploads/hash/notes.txt)",
		FullURL:  "https://gitlab.example.com/group/project/uploads/hash/notes.txt",
	}
	result := uploads.UploadToolResult(u)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content entry, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if strings.Contains(tc.Text, "![") {
		t.Error("non-image upload should not contain image embed")
	}
}

// Pipeline formatters.

// TestFormatPipeline_ListMarkdown verifies pipeline list table rendering.
func TestFormatPipeline_ListMarkdown(t *testing.T) {
	t.Run("with pipelines", func(t *testing.T) {
		out := pipelines.ListOutput{
			Pipelines: []pipelines.Output{
				{ID: 100, Status: "success", Source: "push", Ref: "main", SHA: "abc123def456", WebURL: "https://gl.example.com/p/100"},
				{ID: 101, Status: "failed", Source: "web", Ref: "dev", SHA: "xyz789", WebURL: "https://gl.example.com/p/101"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2, PerPage: 20},
		}
		md := pipelines.FormatListMarkdown(out)
		if !strings.Contains(md, "## Pipelines (2)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[#100]") {
			t.Error("missing pipeline row")
		}
		if !strings.Contains(md, "abc123de") {
			t.Error("SHA should be truncated to 8 chars")
		}
		if !strings.Contains(md, "Page 1 of 1") {
			t.Error("missing pagination")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := pipelines.FormatListMarkdown(pipelines.ListOutput{})
		if !strings.Contains(md, "No pipelines found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatPipeline_DetailMarkdown verifies pipeline detail fields.
func TestFormatPipeline_DetailMarkdown(t *testing.T) {
	p := pipelines.DetailOutput{
		ID: 100, IID: 10, Status: "success", Source: "push", Ref: "main",
		SHA: "abc123", Duration: 120, QueuedDuration: 5, Coverage: "85.5",
		YamlErrors: "", UserUsername: "admin", WebURL: "https://gl.example.com/p/100",
	}
	md := pipelines.FormatDetailMarkdown(p)
	checks := []string{"Pipeline #100", "success", "**Duration**: 120s", "**Coverage**: 85.5%", "**User**: admin"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf(fmtMissing, c)
		}
	}
}

// TestPipelineStatus_Emoji verifies emoji mapping for known statuses.
func TestPipelineStatus_Emoji(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"success", "\u2705"},
		{"failed", "\u274C"},
		{"running", "\U0001F535"},
		{"pending", "\U0001F7E1"},
		{"canceled", "\u26D4"},
		{"skipped", "\u23ED\uFE0F"},
		{"manual", "\u270B"},
		{"unknown", testEmojiQuestion},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := toolutil.PipelineStatusEmoji(tt.status)
			if got != tt.want {
				t.Errorf("PipelineStatusEmoji(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

// TestMRState_Emoji verifies emoji mapping for merge request states.
func TestMRState_Emoji(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{"opened", "\U0001f7e2"},
		{"merged", "\U0001f7e3"},
		{"closed", "\U0001f534"},
		{"unknown", testEmojiQuestion},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := toolutil.MRStateEmoji(tt.state)
			if got != tt.want {
				t.Errorf("MRStateEmoji(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// TestIssueState_Emoji verifies emoji mapping for issue states.
func TestIssueState_Emoji(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{"opened", "\U0001f7e2"},
		{"closed", "\U0001f534"},
		{"unknown", testEmojiQuestion},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := toolutil.IssueStateEmoji(tt.state)
			if got != tt.want {
				t.Errorf("IssueStateEmoji(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// Commit formatters.

// TestFormatCommit_ListMarkdown verifies commit list table rendering.
func TestFormatCommit_ListMarkdown(t *testing.T) {
	t.Run("with commits", func(t *testing.T) {
		out := commits.ListOutput{
			Commits:    []commits.Output{{ShortID: "abc1234", Title: "fix bug", AuthorName: "dev", CommittedDate: testDate20260101}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := commits.FormatListMarkdown(out)
		if !strings.Contains(md, "## Commits (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[abc1234]") {
			t.Error("missing commit row")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := commits.FormatListMarkdown(commits.ListOutput{})
		if !strings.Contains(md, "No commits found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatCommit_DetailMarkdown verifies commit detail fields including stats.
func TestFormatCommit_DetailMarkdown(t *testing.T) {
	c := commits.DetailOutput{
		ShortID: "abc1234", Title: "feat: add feature", Message: "feat: add feature\n\nDetailed description",
		AuthorName: "dev", AuthorEmail: "dev@example.com", CommittedDate: testDate20260101,
		ParentIDs: []string{"parent1", "parent2"}, WebURL: "https://gl.example.com/commit/abc",
		Stats: &commits.StatsOutput{Additions: 10, Deletions: 3, Total: 13},
	}
	md := commits.FormatDetailMarkdown(c)
	checks := []string{"## Commit abc1234", "+10 -3", "parent1, parent2", "### Message"}
	for _, ch := range checks {
		if !strings.Contains(md, ch) {
			t.Errorf(fmtMissing, ch)
		}
	}
}

// TestFormatCommit_DiffMarkdown verifies diff list table and status labels.
func TestFormatCommit_DiffMarkdown(t *testing.T) {
	out := commits.DiffOutput{
		Diffs: []toolutil.DiffOutput{
			{OldPath: "a.go", NewPath: "a.go", NewFile: false, DeletedFile: false, RenamedFile: false},
			{OldPath: "", NewPath: "b.go", NewFile: true},
			{OldPath: "c.go", NewPath: "c.go", DeletedFile: true},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 3, PerPage: 20},
	}
	md := commits.FormatDiffMarkdown(out)
	if !strings.Contains(md, "modified") {
		t.Error("missing modified status")
	}
	if !strings.Contains(md, "added") {
		t.Error("missing added status")
	}
	if !strings.Contains(md, "deleted") {
		t.Error("missing deleted status")
	}
}

// MR Commits / Pipelines / Rebase.

// TestFormatMR_CommitsMarkdown verifies MR commits table rendering.
func TestFormatMR_CommitsMarkdown(t *testing.T) {
	out := mergerequests.CommitsOutput{
		Commits:    []commits.Output{{ShortID: "abc", Title: "fix", AuthorName: "dev", CommittedDate: testDate20260101}},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := mergerequests.FormatCommitsMarkdown(out)
	if !strings.Contains(md, "## MR Commits (1)") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "[abc]") {
		t.Error("missing commit row")
	}
}

// TestFormatMR_PipelinesMarkdown verifies MR pipelines table rendering.
func TestFormatMR_PipelinesMarkdown(t *testing.T) {
	t.Run("with pipelines", func(t *testing.T) {
		out := mergerequests.PipelinesOutput{
			Pipelines: []pipelines.Output{{ID: 50, Status: "success", Source: "push", Ref: "main"}},
		}
		md := mergerequests.FormatPipelinesMarkdown(out)
		if !strings.Contains(md, "## MR Pipelines (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[#50]") {
			t.Error("missing pipeline row")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := mergerequests.FormatPipelinesMarkdown(mergerequests.PipelinesOutput{})
		if !strings.Contains(md, "No pipelines found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatMR_RebaseMarkdown verifies rebase status messages.
func TestFormatMR_RebaseMarkdown(t *testing.T) {
	t.Run("in progress", func(t *testing.T) {
		md := mergerequests.FormatRebaseMarkdown(mergerequests.RebaseOutput{RebaseInProgress: true})
		if !strings.Contains(md, "Rebase in progress") {
			t.Error("missing in-progress message")
		}
	})

	t.Run("completed", func(t *testing.T) {
		md := mergerequests.FormatRebaseMarkdown(mergerequests.RebaseOutput{RebaseInProgress: false})
		if !strings.Contains(md, "Rebase completed") {
			t.Error("missing completed message")
		}
	})
}

// Issue group list.

// TestFormatIssue_ListGroupMarkdown verifies group issue list table.
func TestFormatIssue_ListGroupMarkdown(t *testing.T) {
	t.Run("with issues", func(t *testing.T) {
		out := issues.ListGroupOutput{
			Issues:     []issues.Output{{IID: 5, Title: "bug", State: "opened", Author: "user", Labels: []string{"bug", "critical"}}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := issues.FormatListGroupMarkdown(out)
		if !strings.Contains(md, "## Group Issues (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "bug, critical") {
			t.Error("missing labels")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := issues.FormatListGroupMarkdown(issues.ListGroupOutput{})
		if !strings.Contains(md, "No issues found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// Labels.

// TestFormatLabel_ListMarkdown verifies label list table rendering.
func TestFormatLabel_ListMarkdown(t *testing.T) {
	t.Run("with labels", func(t *testing.T) {
		out := labels.ListOutput{
			Labels:     []labels.Output{{Name: "bug", Color: "#ff0000", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := labels.FormatListMarkdownString(out)
		if !strings.Contains(md, "## Labels (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "| bug | #ff0000 | 5 | 2 | 1 |") {
			t.Error("missing label row")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := labels.FormatListMarkdownString(labels.ListOutput{})
		if !strings.Contains(md, "No labels found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// Milestones.

// TestFormatMilestone_ListMarkdown verifies milestone list table rendering.
func TestFormatMilestone_ListMarkdown(t *testing.T) {
	t.Run("with milestones", func(t *testing.T) {
		out := milestones.ListOutput{
			Milestones: []milestones.Output{{IID: 1, Title: "v1.0", State: "active", DueDate: "2026-06-01", Expired: false}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := milestones.FormatListMarkdownString(out)
		if !strings.Contains(md, "[1](") || !strings.Contains(md, "v1.0") {
			t.Error("missing milestone row")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := milestones.FormatListMarkdownString(milestones.ListOutput{})
		if !strings.Contains(md, "No milestones found.") {
			t.Error(errMissingEmptyMsg)
		}
	})

	t.Run("no due date shows dash", func(t *testing.T) {
		out := milestones.ListOutput{
			Milestones: []milestones.Output{{IID: 2, Title: "backlog", State: "active", DueDate: ""}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := milestones.FormatListMarkdownString(out)
		if !strings.Contains(md, "| \u2014 |") {
			t.Error("missing dash for empty due date")
		}
	})
}

// Repository.

// TestFormatRepository_TreeMarkdown verifies tree table with type icons.
func TestFormatRepository_TreeMarkdown(t *testing.T) {
	t.Run("with entries", func(t *testing.T) {
		out := repository.TreeOutput{
			Tree: []repository.TreeNodeOutput{
				{Name: "src", Type: "tree", Path: "src"},
				{Name: "main.go", Type: "blob", Path: testFileSrcMainGo},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2, PerPage: 20},
		}
		md := repository.FormatTreeMarkdown(out)
		if !strings.Contains(md, "\U0001f4c1") {
			t.Error("missing folder icon")
		}
		if !strings.Contains(md, "\U0001f4c4") {
			t.Error("missing file icon")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := repository.FormatTreeMarkdown(repository.TreeOutput{})
		if !strings.Contains(md, "No files or directories found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatRepository_CompareMarkdown verifies comparison rendering.
func TestFormatRepository_CompareMarkdown(t *testing.T) {
	t.Run("normal compare", func(t *testing.T) {
		out := repository.CompareOutput{
			Commits: []commits.Output{{ShortID: "aaa", Title: "change", AuthorName: "dev"}},
			Diffs:   []toolutil.DiffOutput{{NewPath: "file.go", NewFile: true}},
			WebURL:  "https://gl.example.com/compare",
		}
		md := repository.FormatCompareMarkdown(out)
		if !strings.Contains(md, "## Repository Compare") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "### Commits") {
			t.Error("missing commits section")
		}
		if !strings.Contains(md, "added") {
			t.Error("missing added status")
		}
	})

	t.Run("same ref", func(t *testing.T) {
		md := repository.FormatCompareMarkdown(repository.CompareOutput{CompareSameRef: true})
		if !strings.Contains(md, "same ref") {
			t.Error("missing same ref message")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		md := repository.FormatCompareMarkdown(repository.CompareOutput{CompareTimeout: true})
		if !strings.Contains(md, "timeout") {
			t.Error("missing timeout message")
		}
	})
}

// Jobs.

// TestFormatJob_Markdown verifies job detail fields.
func TestFormatJob_Markdown(t *testing.T) {
	j := jobs.Output{
		ID: 200, Name: "build", Stage: "build", Status: "failed",
		Ref: "main", Duration: 45.5, FailureReason: "script_failure",
		WebURL: "https://gl.example.com/j/200", UserUsername: "ci-bot",
	}
	md := jobs.FormatOutputMarkdown(j)
	checks := []string{"Job #200", "build", "**Stage**: build", "**Failure Reason**: script_failure", "45.5s"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf(fmtMissing, c)
		}
	}
}

// TestFormatJob_ListMarkdown verifies job list table rendering.
func TestFormatJob_ListMarkdown(t *testing.T) {
	t.Run("with jobs", func(t *testing.T) {
		out := jobs.ListOutput{
			Jobs:       []jobs.Output{{ID: 1, Name: "test", Stage: "test", Status: "success", Duration: 10.0}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := jobs.FormatListMarkdown(out)
		if !strings.Contains(md, "## Jobs (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[#1]") {
			t.Error("missing job row")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := jobs.FormatListMarkdown(jobs.ListOutput{})
		if !strings.Contains(md, "No jobs found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatJob_TraceMarkdown verifies job trace code fence and truncation.
func TestFormatJob_TraceMarkdown(t *testing.T) {
	t.Run("normal trace", func(t *testing.T) {
		tr := jobs.TraceOutput{JobID: 99, Trace: "Running script...\nDone.", Truncated: false}
		md := jobs.FormatTraceMarkdown(tr)
		if !strings.Contains(md, "## Job #99 Trace") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "```\nRunning script...") {
			t.Error("missing code fence")
		}
	})

	t.Run("truncated trace", func(t *testing.T) {
		tr := jobs.TraceOutput{JobID: 99, Trace: "big output", Truncated: true}
		md := jobs.FormatTraceMarkdown(tr)
		if !strings.Contains(md, "truncated") {
			t.Error("missing truncation warning")
		}
	})
}

// Search.

// TestFormatSearch_CodeMarkdown verifies code search results table.
func TestFormatSearch_CodeMarkdown(t *testing.T) {
	t.Run("with results", func(t *testing.T) {
		out := search.CodeOutput{
			Blobs:      []search.BlobOutput{{Filename: "main.go", Path: testFileSrcMainGo, Ref: "main", Startline: 42}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := search.FormatCodeMarkdown(out)
		if !strings.Contains(md, "## Code Search Results (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "| main.go | src/main.go | main | 42 |") {
			t.Error("missing blob row")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := search.FormatCodeMarkdown(search.CodeOutput{})
		if !strings.Contains(md, "No code search results found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// TestFormatSearch_MRsMarkdown verifies MR search results table.
func TestFormatSearch_MRsMarkdown(t *testing.T) {
	t.Run("with results", func(t *testing.T) {
		out := search.MergeRequestsOutput{
			MergeRequests: []mergerequests.Output{{IID: 10, Title: "feature", State: "opened", SourceBranch: "feat", TargetBranch: "main"}},
			Pagination:    toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := search.FormatMRsMarkdown(out)
		if !strings.Contains(md, "## MR Search Results (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "feat \u2192 main") {
			t.Error("missing branch arrow")
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := search.FormatMRsMarkdown(search.MergeRequestsOutput{})
		if !strings.Contains(md, "No merge requests found.") {
			t.Error(errMissingEmptyMsg)
		}
	})
}

// Sampling.

// TestFormatAnalyze_MRChangesMarkdown verifies MR analysis rendering.
func TestFormatAnalyze_MRChangesMarkdown(t *testing.T) {
	a := samplingtools.AnalyzeMRChangesOutput{
		MRIID: 42, Title: testTitleAddFeat, Analysis: "This MR adds a new feature.",
		Model: "gpt-4o", Truncated: false,
	}
	md := samplingtools.FormatAnalyzeMRChangesMarkdown(a)
	checks := []string{"## MR Analysis: !42", testTitleAddFeat, "This MR adds a new feature.", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf(fmtMissing, c)
		}
	}
}

// TestFormatAnalyzeMRChangesMarkdown_Truncated verifies truncation warning.
func TestFormatAnalyze_MRChangesMarkdownTruncated(t *testing.T) {
	a := samplingtools.AnalyzeMRChangesOutput{MRIID: 1, Title: "x", Analysis: "text", Truncated: true}
	md := samplingtools.FormatAnalyzeMRChangesMarkdown(a)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestFormatSummarize_IssueMarkdown verifies issue summary rendering.
func TestFormatSummarize_IssueMarkdown(t *testing.T) {
	s := samplingtools.SummarizeIssueOutput{
		IssueIID: 10, Title: testTitleBugReport, Summary: "The issue describes a bug.",
		Model: "claude-4", Truncated: false,
	}
	md := samplingtools.FormatSummarizeIssueMarkdown(s)
	checks := []string{"## Issue Summary: #10", testTitleBugReport, "The issue describes a bug.", "*Model: claude-4*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf(fmtMissing, c)
		}
	}
}

// TestMarkdownForResult_SamplingTypes verifies that markdownForResult correctly
// dispatches all 11 sampling output types through the markdownForSamplingTypes
// switch. Each subtest passes a zero-value output struct and asserts that the
// dispatcher produces a non-nil CallToolResult (proving the type was matched).
func TestMarkdownForResult_SamplingTypes(t *testing.T) {
	tests := []struct {
		name   string
		result any
	}{
		{"AnalyzeMRChangesOutput", samplingtools.AnalyzeMRChangesOutput{MRIID: 1, Title: "t", Analysis: "a"}},
		{"SummarizeIssueOutput", samplingtools.SummarizeIssueOutput{IssueIID: 1, Title: "t", Summary: "s"}},
		{"GenerateReleaseNotesOutput", samplingtools.GenerateReleaseNotesOutput{From: "v1", ReleaseNotes: "notes"}},
		{"AnalyzePipelineFailureOutput", samplingtools.AnalyzePipelineFailureOutput{PipelineID: 1, Analysis: "a"}},
		{"SummarizeMRReviewOutput", samplingtools.SummarizeMRReviewOutput{MRIID: 1, Title: "t", Summary: "r"}},
		{"GenerateMilestoneReportOutput", samplingtools.GenerateMilestoneReportOutput{Title: "m", Report: "r"}},
		{"AnalyzeCIConfigOutput", samplingtools.AnalyzeCIConfigOutput{ProjectID: "1", Analysis: "a"}},
		{"AnalyzeIssueScopeOutput", samplingtools.AnalyzeIssueScopeOutput{IssueIID: 1, Title: "t", Analysis: "s"}},
		{"ReviewMRSecurityOutput", samplingtools.ReviewMRSecurityOutput{MRIID: 1, Title: "t", Review: "s"}},
		{"FindTechnicalDebtOutput", samplingtools.FindTechnicalDebtOutput{ProjectID: "1", Analysis: "d"}},
		{"AnalyzeDeploymentHistoryOutput", samplingtools.AnalyzeDeploymentHistoryOutput{ProjectID: "1", Analysis: "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := markdownForResult(tt.result)
			if result == nil {
				t.Fatal("markdownForResult returned nil — type not matched in dispatch")
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty content from dispatch")
			}
		})
	}
}

// TestMarkdownForResult_DispatchCompleteness exercises markdownForResult with
// one representative Output type from every sub-dispatch function, ensuring the
// full dispatch chain is wired correctly and no sub-function is accidentally
// disconnected. Each entry produces minimal but sufficient data for non-nil output.
func TestMarkdownForResult_DispatchCompleteness(t *testing.T) {
	tests := []struct {
		name   string
		result any
	}{
		// markdownForProjectBranchTagTypes
		{"projects.Output", projects.Output{ID: 1, Name: "p"}},
		{"projects.ListOutput", projects.ListOutput{Projects: []projects.Output{{ID: 1, Name: "p"}}}},
		{"branches.Output", branches.Output{Name: "main"}},
		{"tags.Output", tags.Output{Name: "v1.0"}},

		// markdownForFileWikiTodoTypes
		{"releases.Output", releases.Output{TagName: "v1.0"}},
		{"releaselinks.Output", releaselinks.Output{Name: "bin", URL: "https://x"}},
		{"commits.Output", commits.Output{ShortID: "abc1234", Title: "init"}},
		{"files.Output", files.Output{FileName: "main.go", FilePath: "main.go"}},
		{"toolutil.DeleteOutput", toolutil.DeleteOutput{Status: "success", Message: "deleted"}},
		{"wikis.Output", wikis.Output{Title: "Home"}},
		{"todos.Output", todos.Output{ID: 1, ActionName: "assigned"}},

		// markdownForMRTypes
		{"mergerequests.Output", mergerequests.Output{IID: 1, Title: "MR"}},
		{"mrnotes.Output", mrnotes.Output{ID: 1, Body: "note"}},
		{"mrdiscussions.Output", mrdiscussions.Output{ID: "d1"}},
		{"mrchanges.Output", mrchanges.Output{MRIID: 1}},
		{"mrapprovals.StateOutput", mrapprovals.StateOutput{ApprovalRulesOverwritten: true}},
		{"mrdraftnotes.Output", mrdraftnotes.Output{ID: 1, Note: "draft"}},

		// markdownForIssueGroupUserTypes
		{"issues.Output", issues.Output{IID: 1, Title: "bug"}},
		{"issuenotes.Output", issuenotes.Output{ID: 1, Body: "note"}},
		{"members.Output", members.Output{Username: "u1"}},
		{"groups.Output", groups.Output{ID: 1, Name: "g"}},
		{"users.Output", users.Output{Username: "u1"}},
		{"health.Output", health.Output{Status: "ok", GitLabVersion: "16.0"}},
		{"serverupdate.CheckOutput", serverupdate.CheckOutput{CurrentVersion: "1.0"}},

		// markdownForPipelineCommitMilestoneTypes
		{"pipelines.ListOutput", pipelines.ListOutput{Pipelines: []pipelines.Output{{ID: 1}}}},
		{"commits.ListOutput", commits.ListOutput{Commits: []commits.Output{{ShortID: "a", Title: "c"}}}},
		{"labels.Output", labels.Output{Name: "bug"}},
		{"milestones.Output", milestones.Output{Title: "v1"}},

		// markdownForRepoJobEnvTypes
		{"repository.TreeOutput", repository.TreeOutput{Tree: []repository.TreeNodeOutput{{Name: "f"}}}},
		{"jobs.Output", jobs.Output{ID: 1, Name: "build"}},
		{"search.CodeOutput", search.CodeOutput{Blobs: []search.BlobOutput{{Filename: "m.go"}}}},
		{"environments.Output", environments.Output{ID: 1, Name: "prod"}},
		{"deployments.Output", deployments.Output{ID: 1}},

		// markdownForCIRunnerPackageTypes
		{"pipelineschedules.Output", pipelineschedules.Output{ID: 1, Description: "nightly"}},
		{"civariables.Output", civariables.Output{Key: "K", Value: "V"}},
		{"issuelinks.Output", issuelinks.Output{ID: 1}},
		{"cilint.Output", cilint.Output{Valid: true}},
		{"runners.Output", runners.Output{ID: 1}},
		{"accesstokens.Output", accesstokens.Output{ID: 1, Name: "t"}},
		{"packages.ListOutput", packages.ListOutput{Packages: []packages.ListItem{{ID: 1, Name: "p"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify no panic and non-nil result from the dispatch chain.
			result := markdownForResult(tt.result)
			if result == nil {
				t.Fatalf("markdownForResult(%T) returned nil — type not matched in dispatch", tt.result)
			}
		})
	}
}
