// markdown_test.go contains unit tests for every Markdown formatter function
// in markdown.go. Each test verifies that the rendered output contains expected
// headings, field values, table rows, and empty-state messages.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelineschedules"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Markdown test fixtures and assertion messages shared by formatter tests.
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
		t.Run(c, func(t *testing.T) {
			if !strings.Contains(md, c) {
				t.Errorf("missing %q in:\n%s", c, md)
			}
		})
	}
}

// TestFormatProject_ListMarkdown verifies table rendering for project lists
// and the empty-state message.
func TestFormatProject_ListMarkdown(t *testing.T) {
	t.Run("with projects", func(t *testing.T) {
		// Each fixture carries a web_url because the row is asserted to be a
		// link: toolutil.MdTitleLink renders a bare label when it has no
		// address to point at, rather than the empty link "[alpha]()" this
		// formatter used to write from the same fixture.
		out := projects.ListOutput{
			Projects: []projects.Output{
				{ID: 1, Name: "alpha", PathWithNamespace: "g/alpha", Visibility: "public", WebURL: "https://gl.example.com/g/alpha"},
				{ID: 2, Name: "beta", PathWithNamespace: "g/beta", Visibility: "private", WebURL: "https://gl.example.com/g/beta"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2, PerPage: 20},
		}
		md := projects.FormatListMarkdown(out)
		if !strings.Contains(md, "## Projects (2)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[alpha](https://gl.example.com/g/alpha)") {
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

// TestFormatBranch_Markdown pins the branch card as the reader sees it: the
// flags as glyphs and the head commit as a nested object.
func TestFormatBranch_Markdown(t *testing.T) {
	got := branches.FormatOutputMarkdown(branches.Output{
		Name: "feature-x", Protected: true, Commit: &branches.CommitOutput{ID: "abc123"},
	})

	want := "## Branch: feature-x\n\n" +
		"- **Protected**: ✅\n" +
		"- **Default**: ❌\n" +
		"- **Merged**: ❌\n" +
		"- **You Can Push**: ❌\n" +
		"- **Developers Can Push**: ❌\n" +
		"- **Developers Can Merge**: ❌\n" +
		"- **Commit**:\n" +
		"  - **SHA**: `abc123`\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'merge_request.create' to open a merge request from this branch\n" +
		"- Use action 'repository.commit_list' to see recent commits on this branch\n" +
		"- Use action 'branch.delete' to remove the branch after merging\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatBranch_ListMarkdown pins the branch table row.
func TestFormatBranch_ListMarkdown(t *testing.T) {
	got := branches.FormatListMarkdown(branches.ListOutput{
		Branches:   []branches.Output{{Name: "main", Protected: true, Default: true}},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	})

	want := "## Branches (1)\n\n" +
		"| Name | Protected | Default | Merged |\n" +
		"| --- | --- | --- | --- |\n" +
		"| main | ✅ | ✅ | ❌ |\n" +
		"\nPage 1 of 1 | 1 items total | 20 per page\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'branch.get' to see one branch in full\n" +
		"- Use action 'branch.create' to create a new branch\n" +
		"- Use action 'branch.protect' to protect a branch\n"

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatProtected_BranchMarkdown pins the protection card: the access
// levels as the role names they stand for rather than the numbers GitLab
// sends.
func TestFormatProtected_BranchMarkdown(t *testing.T) {
	got := branches.FormatProtectedMarkdown(branches.ProtectedOutput{
		ID:                1,
		Name:              "main",
		PushAccessLevels:  []branches.BranchAccessDescriptionOutput{{AccessLevel: 40}},
		MergeAccessLevels: []branches.BranchAccessDescriptionOutput{{AccessLevel: 30}},
	})

	want := "## Protected Branch: main\n\n" +
		"- **ID**: 1\n" +
		"- **Push Access Levels**: Maintainer\n" +
		"- **Merge Access Levels**: Developer\n" +
		"- **Unprotect Access Levels**: -\n" +
		"- **Allow Force Push**: ❌\n" +
		"- **Code Owner Approval Required**: ❌\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'branch.get_protected' to fetch this protection again before updating it\n" +
		"- Use action 'branch.update_protected' to change protection settings\n" +
		"- Use action 'branch.unprotect' to remove branch protection\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
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

// TestFormatTag_ListMarkdown pins the tag table row. The commit column is the
// commit the tag resolves to, and falls back to the tag object's own id where
// GitLab sent no commit.
func TestFormatTag_ListMarkdown(t *testing.T) {
	got := tags.FormatListMarkdownString(tags.ListOutput{
		Tags:       []tags.Output{{Name: "v1.0", Target: "abc", Protected: true}},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	})

	want := "## Tags (1)\n\n" +
		"| Name | Commit | Protected |\n" +
		"| --- | --- | --- |\n" +
		"| v1.0 | `abc` | ✅ |\n" +
		"\nPage 1 of 1 | 1 items total | 20 per page\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'tag.get' to see one tag in full\n" +
		"- Use action 'tag.create' to create a new tag\n"

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
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
	l := releaselinks.Output{ID: 1, Name: "binary", URL: "https://example.com/bin", LinkType: "package"}
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
		DetailedMergeStatus: "can_be_merged", Description: "Adds a feature",
		WebURL: "https://gitlab.example.com/mr/15",
		Author: &toolutil.BasicUserOutput{Username: "dev1"}, Labels: []string{"enhancement"},
		Assignees: []*toolutil.BasicUserOutput{{Username: "dev2"}},
		Reviewers: []*toolutil.BasicUserOutput{{Username: "dev3"}},
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	md := mergerequests.FormatMarkdown(mr)

	checks := []string{
		"MR !15: Add feature", "opened",
		"**Source**: feature", "**Target**: main",
		// The description is a labeled row now rather than an H3 section:
		// the card writes prose under its label, quoted when it spans lines.
		"- **Description**: Adds a feature",
		"@dev1", "enhancement", "@dev2", "@dev3",
	}
	for _, c := range checks {
		t.Run(c, func(t *testing.T) {
			if !strings.Contains(md, c) {
				t.Errorf(fmtMissing, c)
			}
		})
	}
}

// TestFormatMRMarkdownDraft_Conflicts verifies draft and conflict indicators.
func TestFormatMRMarkdownDraft_Conflicts(t *testing.T) {
	mr := mergerequests.Output{
		IID: 99, Title: "WIP", State: "opened",
		SourceBranch: "wip", TargetBranch: "main",
		DetailedMergeStatus: "cannot_be_merged", Draft: true, HasConflicts: true,
		WebURL: "https://gitlab.example.com/mr/99",
	}
	md := mergerequests.FormatMarkdown(mr)
	if !strings.Contains(md, "**Draft merge request**") {
		t.Error("missing draft indicator")
	}
	// A negative-polarity condition is marked with the warning sign rather
	// than with the tick a true would otherwise take.
	if !strings.Contains(md, "⚠️ **Has conflicts**") {
		t.Error("missing conflict indicator")
	}
}

// TestFormatMR_ListMarkdown verifies table rendering for merge request lists.
func TestFormatMR_ListMarkdown(t *testing.T) {
	out := mergerequests.ListOutput{
		MergeRequests: []mergerequests.Output{
			{IID: 1, Title: testTitleFixBug, State: "merged", SourceBranch: "fix", TargetBranch: "main", Author: &toolutil.BasicUserOutput{Username: "dev"}},
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

	if !strings.Contains(md, "**Approved**: ❌") {
		t.Error("missing approved field")
	}
	if !strings.Contains(md, "**Approvals Required**: 2") {
		t.Error("missing required field")
	}
}

// TestFormatMR_NoteMarkdown verifies MR note fields in Markdown and that
// non-system notes do not include the system note marker.
func TestFormatMR_NoteMarkdown(t *testing.T) {
	n := mrnotes.Output{ID: 10, Body: "LGTM", Author: &toolutil.NoteUserOutput{Username: "dev"}, CreatedAt: testDate20260101, System: false}
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

// TestFormatDiscussion_NoteMarkdown verifies discussion note fields in
// Markdown. A note nothing can resolve carries no resolution row at all: the
// "**Resolved**: false" this replaced was printed on every note, resolvable or
// not, which told a reader a thread was open that was never a thread.
func TestFormatDiscussion_NoteMarkdown(t *testing.T) {
	n := mrdiscussions.NoteOutput{ID: 5, Body: "Needs fix", Author: &toolutil.NoteUserOutput{Username: "reviewer"}, CreatedAt: testDate20260101}
	md := mrdiscussions.FormatNoteMarkdown(n)

	if !strings.Contains(md, "## Discussion Note #5") {
		t.Error(errMissingHeader)
	}
	if strings.Contains(md, "Resolvable") {
		t.Errorf("a note that is not resolvable carries a resolution row:\n%s", md)
	}

	t.Run("resolvable", func(t *testing.T) {
		n.Resolvable = true
		resolvable := mrdiscussions.FormatNoteMarkdown(n)
		if !strings.Contains(resolvable, "**Resolvable**: unresolved") {
			t.Errorf("missing resolution state:\n%s", resolvable)
		}
	})
}

// TestFormatMR_DiscussionMarkdown verifies that a discussion thread with
// multiple notes renders each note as a list item naming its author and its ID,
// with the body quoted underneath — the shape every discussion family shares.
// The "### Note N (by author)" heading this replaced put a name in a heading
// and the body at column zero, where it could add structure of its own.
func TestFormatMR_DiscussionMarkdown(t *testing.T) {
	d := mrdiscussions.Output{
		ID:             "abc123",
		IndividualNote: false,
		Notes: []*mrdiscussions.NoteOutput{
			{ID: 1, Body: "First note", Author: &toolutil.NoteUserOutput{Username: "dev1"}},
			{ID: 2, Body: "Reply", Author: &toolutil.NoteUserOutput{Username: "dev2"}},
		},
	}
	md := mrdiscussions.FormatOutputMarkdown(d)

	if !strings.Contains(md, "## Discussion abc123") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "- **@dev1** (, note 1):\n  > First note") {
		t.Errorf("missing first note:\n%s", md)
	}
	if !strings.Contains(md, "- **@dev2** (, note 2):\n  > Reply") {
		t.Errorf("missing second note:\n%s", md)
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

	if !strings.Contains(md, "## MR !15 Changes\n\n- **Files**: 4\n") {
		t.Errorf("%s:\n%s", errMissingHeader, md)
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

// TestFormatCommit_Markdown pins the commit card. The author's address is
// written in parentheses rather than in angle brackets, which GFM turns into a
// mailto autolink.
func TestFormatCommit_Markdown(t *testing.T) {
	got := commits.FormatOutputMarkdown(commits.Output{
		ID: "abc123full", ShortID: "abc123", Title: testTitleFixBug,
		AuthorName: "Dev", AuthorEmail: "dev@example.com",
		CommittedDate: testDate20260101, WebURL: "https://gitlab.example.com/commit/abc123",
	})

	want := "## Commit abc123\n\n" +
		"- **Title**: " + testTitleFixBug + "\n" +
		"- **Author**: Dev (dev@example.com)\n" +
		"- **Date**: 1 Jan 2026\n" +
		"- **URL**: [https://gitlab.example.com/commit/abc123](https://gitlab.example.com/commit/abc123)\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'repository.commit_get' to see this commit's full details and stats\n" +
		"- Use action 'repository.commit_diff' to see the file changes for this commit\n" +
		"- Use action 'repository.commit_refs' to see the branches and tags containing it\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatFile_Markdown pins the file card of a metadata-only read.
func TestFormatFile_Markdown(t *testing.T) {
	got := files.FormatOutputMarkdown(files.Output{
		FilePath: testFileSrcMainGo, Size: 1024, Ref: "main", Encoding: "base64", BlobID: "blob123",
	})

	want := "## File: src/main.go\n\n" +
		"- **Size (bytes)**: 1024\n" +
		"- **Ref**: main\n" +
		"- **Encoding**: base64\n" +
		"- **Blob ID**: `blob123`\n" +
		"- **Executable**: ❌\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'repository.file_update' to modify this file\n" +
		"- Use action 'repository.file_blame' to see who changed each line\n" +
		"- Use action 'repository.file_delete' to remove this file\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMember_ListMarkdown verifies table rendering for member lists
// and the empty-state message.
func TestFormatMember_ListMarkdown(t *testing.T) {
	t.Run("with members", func(t *testing.T) {
		out := members.ListOutput{
			Members: []members.Output{
				{Username: "dev1", Name: "Developer One", AccessLevel: 30, State: "active"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		want := "## Project Members (1)\n\n" +
			"| Username | Name | Access Level | State | Membership | Expires |\n" +
			"| --- | --- | --- | --- | --- | --- |\n" +
			"| @dev1 | Developer One | Developer (30) | active |  |  |\n" +
			"\nPage 1 of 1 | 1 items total | 20 per page\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'project.member_get' to see one member's details\n" +
			"- Use action 'project.member_add' to add a member to this project\n"
		if got := members.FormatListMarkdownString(out); got != want {
			t.Errorf("FormatListMarkdownString =\n%q\nwant\n%q", got, want)
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
		t.Run(c, func(t *testing.T) {
			if !strings.Contains(md, c) {
				t.Errorf(fmtMissing, c)
			}
		})
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
			{Username: "admin", Name: "Admin User", AccessLevel: 50, State: "active"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := groups.FormatMemberListMarkdown(out)
	if !strings.Contains(md, "| @admin | Admin User | Owner | active |") {
		t.Errorf("missing group member row:\n%s", md)
	}
}

// TestFormatIssue_Markdown verifies that all issue fields appear in Markdown output.
func TestFormatIssue_Markdown(t *testing.T) {
	i := issues.Output{
		ID: 1, IID: 5, Title: testTitleBugReport, State: "opened",
		Author: &toolutil.IssueUserOutput{Username: "reporter"}, Labels: []string{"bug", "critical"},
		Assignees: []*toolutil.IssueUserOutput{{Username: "dev1"}},
		Milestone: &toolutil.MRMilestoneOutput{Title: "v1.0"},
		DueDate:   "2026-03-01", CreatedAt: testDate20260101,
		Description: "Something is broken",
		WebURL:      "https://gitlab.example.com/issues/5",
	}
	md := issues.FormatMarkdown(i)

	checks := []string{
		"Issue #5: Bug report", "opened",
		"**Labels**: bug, critical", "@dev1",
		"**Milestone**: v1.0", "**Due Date**: 1 Mar 2026",
		// The issue card writes the description as a labeled row rather than
		// under a section heading of its own.
		"- **Description**: Something is broken",
	}
	for _, c := range checks {
		t.Run(c, func(t *testing.T) {
			if !strings.Contains(md, c) {
				t.Errorf(fmtMissing, c)
			}
		})
	}
}

// TestFormatIssue_MarkdownConfidentialTasks verifies confidential and task progress indicators.
func TestFormatIssue_MarkdownConfidentialTasks(t *testing.T) {
	i := issues.Output{
		IID: 42, Title: "Secret task", State: "opened",
		Author: &toolutil.IssueUserOutput{Username: "admin"}, Confidential: true,
		TaskCompletionStatus: &toolutil.TaskCompletionStatusOutput{CompletedCount: 2, Count: 5},
		UserNotesCount:       3,
		WebURL:               "https://gitlab.example.com/issues/42",
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
			{IID: 1, Title: "Feature req", State: "opened", Author: &toolutil.IssueUserOutput{Username: "user1"}, Labels: []string{"enhancement"}},
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
		n := issuenotes.Output{ID: 1, Body: "Comment", Author: &toolutil.NoteUserOutput{Username: "user"}, CreatedAt: testDate20260101, System: false, Internal: false}
		md := issuenotes.FormatOutputMarkdown(n)
		if !strings.Contains(md, "## Issue Note #1") {
			t.Error(errMissingHeader)
		}
		if strings.Contains(md, "System note") || strings.Contains(md, "Internal note") {
			t.Error("regular note should not have system/internal markers")
		}
	})

	t.Run("system internal note", func(t *testing.T) {
		n := issuenotes.Output{ID: 2, Body: "Label added", Author: &toolutil.NoteUserOutput{Username: "bot"}, CreatedAt: testDate20260101, System: true, Internal: true}
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
		YamlErrors: "", User: &toolutil.BasicUserOutput{Username: "admin"}, WebURL: "https://gl.example.com/p/100",
	}
	md := pipelines.FormatDetailMarkdown(p)
	checks := []string{"Pipeline #100", "success", "**Duration**: 120s", "**Coverage**: 85.5%", "**User**: @admin"}
	for _, c := range checks {
		t.Run(c, func(t *testing.T) {
			if !strings.Contains(md, c) {
				t.Errorf(fmtMissing, c)
			}
		})
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
			Commits: []commits.Output{{
				ShortID: "abc1234", Title: "fix bug", AuthorName: "dev",
				CommittedDate: testDate20260101, WebURL: "https://gl.example.com/c/abc1234",
			}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := commits.FormatListMarkdown(out)
		if !strings.Contains(md, "## Commits (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[abc1234](https://gl.example.com/c/abc1234)") {
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

// TestFormatCommit_DetailMarkdown pins the commit detail card, the message
// quoted under its own label where it says more than the title.
func TestFormatCommit_DetailMarkdown(t *testing.T) {
	got := commits.FormatDetailMarkdown(commits.DetailOutput{
		ShortID: "abc1234", Title: "feat: add feature", Message: "feat: add feature\n\nDetailed description",
		AuthorName: "dev", AuthorEmail: "dev@example.com", CommittedDate: testDate20260101,
		ParentIDs: []string{"parent1", "parent2"}, WebURL: "https://gl.example.com/commit/abc",
		Stats: &commits.CommitStatsOutput{Additions: 10, Deletions: 3, Total: 13},
	})

	want := "## Commit abc1234\n\n" +
		"- **Title**: feat: add feature\n" +
		"- **Author**: dev (dev@example.com)\n" +
		"- **Date**: 1 Jan 2026\n" +
		"- **Parents**: `parent1, parent2`\n" +
		"- **Stats**: +10 -3 (13 total)\n" +
		"- **URL**: [https://gl.example.com/commit/abc](https://gl.example.com/commit/abc)\n" +
		"- **Message**:\n" +
		"  > feat: add feature\n" +
		"  >\n" +
		"  > Detailed description\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'repository.commit_diff' to view the file changes\n" +
		"- Use action 'repository.commit_cherry_pick' to apply this commit to another branch\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
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
		Commits: []commits.Output{{
			ShortID: "abc", Title: "fix", AuthorName: "dev",
			CommittedDate: testDate20260101, WebURL: "https://gl.example.com/c/abc",
		}},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	}
	md := mergerequests.FormatCommitsMarkdown(out)
	if !strings.Contains(md, "## MR Commits (1)") {
		t.Error(errMissingHeader)
	}
	if !strings.Contains(md, "[abc](https://gl.example.com/c/abc)") {
		t.Error("missing commit row")
	}
}

// TestFormatMR_PipelinesMarkdown verifies MR pipelines table rendering.
func TestFormatMR_PipelinesMarkdown(t *testing.T) {
	t.Run("with pipelines", func(t *testing.T) {
		out := mergerequests.PipelinesOutput{
			Pipelines: []pipelines.Output{{
				ID: 50, Status: "success", Source: "push", Ref: "main",
				WebURL: "https://gl.example.com/p/50",
			}},
		}
		md := mergerequests.FormatPipelinesMarkdown(out)
		if !strings.Contains(md, "## MR Pipelines (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[#50](https://gl.example.com/p/50)") {
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
			Issues:     []issues.Output{{IID: 5, Title: "bug", State: "opened", Author: &toolutil.IssueUserOutput{Username: "user"}, Labels: []string{"bug", "critical"}}},
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
		want := "## Labels (1)\n\n" +
			"| Name | Color | Scope | Open Issues | Closed Issues | Open MRs |\n" +
			"| --- | --- | --- | --- | --- | --- |\n" +
			"| bug | #ff0000 | group | 5 | 2 | 1 |\n" +
			"\nPage 1 of 1 | 1 items total | 20 per page\n" +
			"\n---\n\U0001F4A1 **Next steps:**\n" +
			"- Use action 'label_get' with a label_id to see label details\n" +
			"- Use action 'label_create' to create a new label\n"
		if md != want {
			t.Errorf("label list:\n got %q\nwant %q", md, want)
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := labels.FormatListMarkdownString(labels.ListOutput{})
		if want := "No labels found.\n"; md != want {
			t.Errorf("empty label list = %q, want %q", md, want)
		}
	})
}

// Milestones.

// TestFormatMilestone_ListMarkdown verifies milestone list table rendering.
func TestFormatMilestone_ListMarkdown(t *testing.T) {
	t.Run("with milestones", func(t *testing.T) {
		out := milestones.ListOutput{
			Milestones: []milestones.Output{{
				IID: 1, Title: "v1.0", State: "active", DueDate: "2026-06-01",
				Expired: false, WebURL: "https://gl.example.com/m/1",
			}},
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
		if !strings.Contains(md, "| - |") {
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
		WebURL: "https://gl.example.com/j/200", User: &jobs.UserObject{Username: "ci-bot"},
	}
	md := jobs.FormatOutputMarkdown(j)
	checks := []string{"Job #200", "build", "**Stage**: build", "**Failure Reason**: script_failure", "45.5s"}
	for _, c := range checks {
		t.Run(c, func(t *testing.T) {
			if !strings.Contains(md, c) {
				t.Errorf(fmtMissing, c)
			}
		})
	}
}

// TestFormatJob_ListMarkdown verifies job list table rendering.
func TestFormatJob_ListMarkdown(t *testing.T) {
	t.Run("with jobs", func(t *testing.T) {
		out := jobs.ListOutput{
			Jobs: []jobs.Output{{
				ID: 1, Name: "test", Stage: "test", Status: "success",
				Duration: 10.0, WebURL: "https://gl.example.com/j/1",
			}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
		}
		md := jobs.FormatListMarkdown(out)
		if !strings.Contains(md, "## Jobs (1)") {
			t.Error(errMissingHeader)
		}
		if !strings.Contains(md, "[#1](https://gl.example.com/j/1)") {
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
		// The note names the end that is missing: the log is cut at the first
		// 100 KB and a failure is almost always at the end of it.
		if !strings.Contains(md, "Showing the first 100 KB of the log.") {
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
		if !strings.Contains(md, "feat -> main") {
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
				t.Fatalf("markdownForResult(%T) returned nil: type not matched in dispatch", tt.result)
			}
		})
	}
}

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
					pipes, tableStart, headerPipes,
				),
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
	return append([]markdownFixture(nil), allMarkdownFixtureData...)
}

var allMarkdownFixtureData = []markdownFixture{
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
	{"mergerequests.IssuesClosedOutput", mergerequests.IssuesClosedOutput{Issues: []issues.BasicOutput{{IID: 1}}}},
	{"mergerequests.TimeStatsOutput", mergerequests.TimeStatsOutput{HumanTimeEstimate: "1h"}},
	{"mergerequests.RelatedIssuesOutput", mergerequests.RelatedIssuesOutput{Issues: []issues.BasicOutput{{IID: 1}}}},
	{"mergerequests.CreateTodoOutput", mergerequests.CreateTodoOutput{ID: 1, ActionName: "marked"}},
	{"mergerequests.DependencyOutput", mergerequests.DependencyOutput{ID: 1, BlockingMergeRequest: &mergerequests.BlockingMergeRequestOutput{IID: 2}}},
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
	{"mrapprovals.StateOutput", mrapprovals.StateOutput{Rules: []mrapprovals.StateRuleOutput{{ID: 1, Name: "rule"}}}},
	{"mrapprovals.RulesOutput", mrapprovals.RulesOutput{Rules: []mrapprovals.RuleOutput{{ID: 1, Name: "rule"}}}},
	{"mrapprovals.ConfigOutput", mrapprovals.ConfigOutput{Approved: true}},
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
	{"issuelinks.Output", issuelinks.Output{ID: 1, SourceIssue: &issuelinks.IssueRefOutput{IID: 10}, TargetIssue: &issuelinks.IssueRefOutput{IID: 20}, LinkType: "relates_to"}},
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
				t.Logf("FINDING: markdownForResult returned nil: type not dispatched")
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

// TestIsTable_Separator covers IsTable with table-driven subtests for separator.
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

// TestValidateMarkdown_Tables verifies ValidateMarkdown when tables.
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

// rawISOTimestampRE detects raw ISO 8601 timestamps that should have been
// formatted by toolutil.FormatTime (e.g. "2026-01-15T10:30:00Z").
var rawISOTimestampRE = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)

// markdownQualityCase defines a test case for markdown quality validation.
type markdownQualityCase struct {
	name   string
	result any
	// expectWebURL indicates this entity type should include a web URL link.
	expectWebURL bool
	// simpleMessage indicates this is a short confirmation message (no header expected).
	simpleMessage bool
}

// TestMarkdownForResult_QualityPatterns validates that representative markdown
// formatters produce output with consistent structural quality: markdown headers,
// no raw ISO timestamps, and web URLs where expected.
func TestMarkdownForResult_QualityPatterns(t *testing.T) {
	cases := []markdownQualityCase{
		{
			name: "projects.Output",
			result: projects.Output{
				ID: 1, Name: "test-project", PathWithNamespace: "group/test-project",
				Visibility: "private", DefaultBranch: "main", WebURL: "https://gitlab.example.com/group/test-project",
				CreatedAt: "2026-01-15 10:30", LastActivityAt: "2026-06-01 15:00",
			},
			expectWebURL: true,
		},
		{
			name: "projects.ListOutput",
			result: projects.ListOutput{
				Projects: []projects.Output{
					{ID: 1, Name: "proj-1", PathWithNamespace: "g/p1", Visibility: "private"},
					{ID: 2, Name: "proj-2", PathWithNamespace: "g/p2", Visibility: "public"},
				},
			},
		},
		{
			name: "issues.Output",
			result: issues.Output{
				ID: 10, IID: 5, Title: "Fix login bug", State: "opened",
				WebURL: "https://gitlab.example.com/group/project/-/issues/5",
				Author: &toolutil.IssueUserOutput{Username: "alice"}, CreatedAt: "2026-02-01 09:00",
			},
			expectWebURL: true,
		},
		{
			name: "issues.ListOutput",
			result: issues.ListOutput{
				Issues: []issues.Output{
					{ID: 10, IID: 5, Title: "Fix login", State: "opened", Author: &toolutil.IssueUserOutput{Username: "alice"}},
					{ID: 11, IID: 6, Title: "Add tests", State: "closed", Author: &toolutil.IssueUserOutput{Username: "bob"}},
				},
			},
		},
		{
			name: "mergerequests.Output",
			result: mergerequests.Output{
				ID: 20, IID: 3, Title: "Feature branch", State: "opened",
				SourceBranch: "feature/x", TargetBranch: "main", Author: &toolutil.BasicUserOutput{Username: "alice"},
				WebURL:    "https://gitlab.example.com/group/project/-/merge_requests/3",
				CreatedAt: "2026-03-10 14:00",
			},
			expectWebURL: true,
		},
		{
			name: "mergerequests.ListOutput",
			result: mergerequests.ListOutput{
				MergeRequests: []mergerequests.Output{
					{ID: 20, IID: 3, Title: "MR 1", State: "opened", Author: &toolutil.BasicUserOutput{Username: "alice"}, SourceBranch: "f1", TargetBranch: "main"},
				},
			},
		},
		{
			name: "branches.Output",
			result: branches.Output{
				Name: "feature/auth", Protected: false, Merged: false,
				Commit: &branches.CommitOutput{ID: "abc123def"},
			},
		},
		{
			name: "branches.ListOutput",
			result: branches.ListOutput{
				Branches: []branches.Output{
					{Name: "main", Protected: true},
					{Name: "develop", Protected: false},
				},
			},
		},
		{
			name: "commits.Output",
			result: commits.Output{
				ID: "abc123def456", ShortID: "abc123d", Title: "Initial commit",
				AuthorName: "alice", WebURL: "https://gitlab.example.com/group/project/-/commit/abc123d",
				CommittedDate: "2026-04-01 12:00",
			},
			expectWebURL: true,
		},
		{
			name: "pipelines.DetailOutput",
			result: pipelines.DetailOutput{
				ID: 100, Status: "success", Ref: "main", SHA: "abc123",
				WebURL:    "https://gitlab.example.com/group/project/-/pipelines/100",
				CreatedAt: "2026-05-01 08:00",
			},
			expectWebURL: true,
		},
		{
			name: "tags.Output",
			result: tags.Output{
				Name: "v1.0.0", Commit: &tags.CommitOutput{ID: "abc123"}, Message: "Release v1.0.0",
			},
		},
		{
			name: "releases.Output",
			result: releases.Output{
				Name: "v1.0.0", TagName: "v1.0.0", Description: "First release",
				CreatedAt: "2026-06-01 10:00",
			},
		},
		{
			name: "labels.Output",
			result: labels.Output{
				ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug reports",
			},
		},
		{
			name: "milestones.Output",
			result: milestones.Output{
				ID: 1, IID: 1, Title: "v2.0", State: "active",
			},
		},
		{
			name: "groups.Output",
			result: groups.Output{
				ID: 1, Name: "Engineering", FullPath: "company/engineering",
				WebURL:     "https://gitlab.example.com/company/engineering",
				Visibility: "private",
			},
			expectWebURL: true,
		},
		{
			name: "users.Output",
			result: users.Output{
				ID: 1, Username: "alice", Name: "Alice Smith", State: "active",
				WebURL: "https://gitlab.example.com/alice",
			},
			expectWebURL: true,
		},
		{
			name: "members.Output",
			result: members.Output{
				ID: 1, Username: "alice", Name: "Alice Smith",
				AccessLevel: 30, State: "active",
			},
		},
		{
			name: "files.Output",
			result: files.Output{
				FileName: "README.md", FilePath: "README.md", Size: 1024,
				Content: "# Hello", Encoding: "text",
			},
		},
		{
			name: "health.Output",
			result: health.Output{
				GitLabVersion: "17.0.0", GitLabURL: "https://gitlab.example.com",
				GitLabRevision: "abc123", Authenticated: true,
			},
		},
		{
			name:          "nil_result",
			result:        nil,
			simpleMessage: true,
		},
		{
			name:          "delete_output",
			result:        toolutil.DeleteOutput{Message: "Branch 'feature/old' deleted"},
			simpleMessage: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := markdownForResult(tc.result)
			if got == nil {
				t.Fatal("markdownForResult returned nil")
			}

			md := extractTextContent(got)
			if md == "" {
				t.Fatal("CallToolResult has no TextContent")
			}

			// 1. Must have a markdown header (##) for entity formatters.
			if !tc.simpleMessage && !strings.Contains(md, "## ") && !strings.Contains(md, "# ") {
				t.Error("markdown output missing header (# or ##)")
			}

			// 2. No raw ISO timestamps (should be formatted by FormatTime).
			if rawISOTimestampRE.MatchString(md) {
				match := rawISOTimestampRE.FindString(md)
				t.Errorf("raw ISO timestamp found: %q. Use toolutil.FormatTime", match)
			}

			// 3. Web URL expected for detail entities.
			if tc.expectWebURL && !strings.Contains(md, "http") {
				t.Error("expected web URL in output but none found")
			}
		})
	}
}

// TestMarkdownForResult_NilIsHandled verifies the dispatcher returns a
// success message for nil results (void actions like delete).
func TestMarkdownForResult_NilIsHandled(t *testing.T) {
	got := markdownForResult(nil)
	if got == nil {
		t.Fatal("markdownForResult(nil) should return a success result")
	}
	md := extractTextContent(got)
	if md == "" || !strings.Contains(strings.ToLower(md), "ok") {
		t.Errorf("expected success message for nil result, got %q", md)
	}
}

// TestMarkdownForResult_DeleteOutput verifies delete messages are formatted.
func TestMarkdownForResult_DeleteOutput(t *testing.T) {
	got := markdownForResult(toolutil.DeleteOutput{Message: "Resource deleted"})
	if got == nil {
		t.Fatal("markdownForResult returned nil for DeleteOutput")
	}
	md := extractTextContent(got)
	if !strings.Contains(md, "Resource deleted") {
		t.Errorf("delete message not in output: %q", md)
	}
}

// The runtime structural gate.
//
// Every registered formatter is driven through MarkdownForResult with
// reflective fixtures and the text the client would receive is read with the
// GFM line model in internal/testutil, which is the only reader here that
// applies a renderer's block rules rather than a substring check. The rules
// are the ones the markdown audit proved the tree breaks: a table whose
// header lazily continues a hint bullet, a footer absorbed as a row, a card
// row written inside a table, a guidance section ExtractHints cannot read,
// a heading that counts the page rather than the total, a field GitLab
// omitted rendered as a glyph, and a hostile value that changes the shape of
// the document. Every rule reports and none gates in this layer: the
// findings are the migration's work list, the exception map below is the
// mechanism the layer that turns the gate on will use, and the one assertion
// made now is that the gate sees the two files the audit proved broken.

// mdGateCase is one thing the gate renders: a registered type, or a renderer
// outside the registry driven by hand.
type mdGateCase struct {
	// name is what a finding is filed under: the type, or the function.
	name string
	// pkg is the package the finding belongs to.
	pkg string
	// typ is the registered type, nil for a renderer outside the registry.
	typ reflect.Type
	// explicit renders a case outside the registry from the fixture options,
	// returning "" when the case has no render for that state.
	explicit func(opts testutil.FixtureOptions) string
}

// mdGateFinding is one finding the gate reports.
type mdGateFinding struct {
	kase   mdGateCase
	state  string
	rule   string
	line   int
	text   string
	detail string
}

// String renders a finding as one report line.
func (f mdGateFinding) String() string {
	where := ""
	if f.line > 0 {
		where = fmt.Sprintf(" line %d: %q", f.line, f.text)
	}
	return fmt.Sprintf("%s %s [%s]%s: %s", f.rule, f.kase.name, f.state, where, f.detail)
}

// mdGateExceptions names the cases the gate leaves out, each with the reason,
// keyed by the case name a finding prints. It is empty while the gate
// reports only; the layer that turns a rule on fills it with the cases that
// rule accepts, and an entry naming no case fails, so a fixed formatter
// cannot leave a stale exception behind.
var mdGateExceptions = map[string]string{}

// mdGateStates are the populated states every case is rendered in beside the
// zero value.
var mdGateStates = []testutil.FixtureState{testutil.FixtureZero, testutil.FixtureMultiPage, testutil.FixtureSinglePage}

// mdGateCases lists everything the gate renders: every registered type, and
// the renderers outside the registry the plan names.
func mdGateCases(t *testing.T) []mdGateCase {
	t.Helper()
	var cases []mdGateCase
	for _, typ := range toolutil.RegisteredMarkdownTypes() {
		cases = append(cases, mdGateCase{name: typ.String(), pkg: mdGatePackage(typ), typ: typ})
	}
	cases = append(cases, mdGateExplicitCases(t)...)
	return cases
}

// mdGatePackage names the package a type belongs to the way a finding does.
func mdGatePackage(typ reflect.Type) string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return filepath.Base(typ.PkgPath())
}

// mdGateExplicitCases are the renderers no registration reaches: the shared
// iteration renderers two packages wrap, the elicitation outcomes, and the
// dynamic surface's find, search and describe output.
func mdGateExplicitCases(t *testing.T) []mdGateCase {
	t.Helper()
	registry := dynamic.NewRegistryFromCatalog(mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true, IncludeMCP: true}))
	populated := func(opts testutil.FixtureOptions) bool { return opts.State != testutil.FixtureZero }
	iteration := func(opts testutil.FixtureOptions) iterationdata.Output {
		return testutil.FillFixture(reflect.TypeFor[iterationdata.Output](), opts).Interface().(iterationdata.Output)
	}
	return []mdGateCase{
		{name: "iterationdata.FormatOutputMarkdown", pkg: "iterationdata", explicit: func(opts testutil.FixtureOptions) string {
			return iterationdata.FormatOutputMarkdown(iteration(opts), "Use action 'iteration_list' to list iterations")
		}},
		{name: "iterationdata.FormatListMarkdown", pkg: "iterationdata", explicit: func(opts testutil.FixtureOptions) string {
			var items []iterationdata.Output
			if populated(opts) {
				items = []iterationdata.Output{iteration(opts), iteration(opts)}
			}
			pagination := testutil.FillFixture(reflect.TypeFor[toolutil.PaginationOutput](), opts).Interface().(toolutil.PaginationOutput)
			return iterationdata.FormatListMarkdown("Iterations", "No iterations found.", items, pagination)
		}},
		{name: "elicitationtools.UnsupportedResult", pkg: "elicitationtools", explicit: func(opts testutil.FixtureOptions) string {
			if !populated(opts) {
				return ""
			}
			return extractTextContent(elicitationtools.UnsupportedResult("gitlab_interactive_issue_create"))
		}},
		{name: "elicitationtools.CancelledResult", pkg: "elicitationtools", explicit: func(opts testutil.FixtureOptions) string {
			if !populated(opts) {
				return ""
			}
			return extractTextContent(elicitationtools.CancelledResult("Issue creation cancelled"))
		}},
		{name: "elicitationtools.FormatResult", pkg: "elicitationtools", explicit: func(opts testutil.FixtureOptions) string {
			issue := testutil.FillFixture(reflect.TypeFor[issues.Output](), opts).Interface().(issues.Output)
			return extractTextContent(elicitationtools.FormatResult(issue))
		}},
		{name: "dynamic.Registry.Search", pkg: "dynamic", explicit: func(opts testutil.FixtureOptions) string {
			if !populated(opts) {
				return ""
			}
			result, _, err := registry.Search(context.Background(), nil, dynamic.SearchInput{Query: "project create", Explain: true})
			if err != nil {
				return ""
			}
			return extractTextContent(result)
		}},
		{name: "dynamic.Registry.Find", pkg: "dynamic", explicit: func(opts testutil.FixtureOptions) string {
			if !populated(opts) {
				return ""
			}
			result, _, err := registry.Find(context.Background(), nil, dynamic.FindInput{Query: "merge request approve", Explain: true})
			if err != nil {
				return ""
			}
			return extractTextContent(result)
		}},
		{name: "dynamic.Registry.Describe", pkg: "dynamic", explicit: func(opts testutil.FixtureOptions) string {
			if !populated(opts) {
				return ""
			}
			result, _, err := registry.Describe(context.Background(), nil, dynamic.DescribeInput{Actions: []string{"project.create", "issue.list"}})
			if err != nil {
				return ""
			}
			return extractTextContent(result)
		}},
	}
}

// render produces the text the client would receive for one case in one
// state: through MarkdownForResult for a registered type, so the text is
// exactly what the dispatcher hands the client, and a panic is itself
// reported rather than ending the run.
func (c mdGateCase) render(opts testutil.FixtureOptions) (md string, rendered bool, panicked string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprint(r)
		}
	}()
	if c.explicit != nil {
		md = c.explicit(opts)
		return md, md != "", ""
	}
	value := testutil.FillFixture(c.typ, opts)
	result := toolutil.MarkdownForResult(value.Interface())
	if result == nil {
		return "", false, ""
	}
	md = extractTextContent(result)
	return md, md != "", ""
}

// mdGateOptional lists the fields a case's fixture can leave absent, for the
// differential; a case outside the registry has none.
func (c mdGateCase) optionalFields() []testutil.FixtureField {
	if c.typ == nil {
		return nil
	}
	return testutil.OptionalFields(c.typ)
}

// mdGateReport is one run over every case, built once and read by every
// rule's test.
type mdGateReport struct {
	cases    []mdGateCase
	findings []mdGateFinding
	rendered int
	silent   int
}

var (
	mdGateOnce   sync.Once
	mdGateShared *mdGateReport
)

// mdGateScan renders every case in every state and applies the rules that
// read one render: the table, block and hint rules of the line model, the
// hint agreement with ExtractHints, the preserve-links hint over a render
// with no link, the raw timestamp, and the list heading against the
// pagination it was rendered with.
func mdGateScan(t *testing.T) *mdGateReport {
	t.Helper()
	mdGateOnce.Do(func() {
		report := &mdGateReport{cases: mdGateCases(t)}
		for _, c := range report.cases {
			for _, state := range mdGateStates {
				report.scan(c, state)
			}
			report.scanListHeading(c)
		}
		mdGateShared = report
	})
	return mdGateShared
}

// scan renders one case in one state and applies the single-render rules.
func (r *mdGateReport) scan(c mdGateCase, state testutil.FixtureState) {
	md, rendered, panicked := c.render(testutil.FixtureOptions{State: state})
	if panicked != "" {
		r.add(c, state, "P0", 0, "", "the formatter panicked: "+panicked)
		return
	}
	if !rendered {
		r.silent++
		return
	}
	r.rendered++
	doc := testutil.ScanGFM(md)
	for _, f := range doc.Findings {
		r.add(c, state, f.Rule, f.Line, f.Text, f.Detail)
	}
	if len(doc.Hints) == 1 {
		if got := toolutil.ExtractHints(md); !mdGateSameHints(got, doc.Hints[0].Bullets) {
			r.add(c, state, "H1", doc.Hints[0].Line+1, doc.Lines[doc.Hints[0].Line],
				fmt.Sprintf("the guidance section holds %d hint(s) and ExtractHints reads %d: the section is neither leading nor trailing", len(doc.Hints[0].Bullets), len(got)))
		}
	}
	if strings.Contains(md, toolutil.HintPreserveLinks) {
		switch {
		case state == testutil.FixtureZero:
			r.add(c, state, "H2", 0, "", "HintPreserveLinks on the empty render, which has no link to preserve")
		case !mdGateHasFixtureLink(doc.Links):
			r.add(c, state, "H2", 0, "", "HintPreserveLinks over a render with no link outside the hints")
		}
	}
	if state != testutil.FixtureZero {
		for _, raw := range []string{"2001-02-03T04:05:06", "2001-02-03 04:05:06", "+0000 UTC"} {
			if strings.Contains(md, raw) {
				r.add(c, state, "R1", 0, "", "the sentinel instant renders as "+raw+" rather than through FormatTime")
				break
			}
		}
	}
}

// scanListHeading applies the list heading rule to a case whose output
// carries offset pagination: rendered with a total of 45 and two rows, the
// heading must count 45; rendered with no total and a next page, it must not
// count 0.
func (r *mdGateReport) scanListHeading(c mdGateCase) {
	if c.typ == nil || !mdGateHasPagination(c.typ) {
		return
	}
	multi, rendered, _ := c.render(testutil.FixtureOptions{State: testutil.FixtureMultiPage})
	if rendered {
		if n, counted := mdGateHeadingCount(multi); counted && n != 45 {
			r.add(c, testutil.FixtureMultiPage, "L1", 1, mdGateFirstLine(multi), fmt.Sprintf("the heading counts %d while the response reports 45 in all", n))
		}
	}
	keyset, rendered, _ := c.render(testutil.FixtureOptions{State: testutil.FixtureKeyset})
	if rendered {
		if n, counted := mdGateHeadingCount(keyset); counted && n == 0 {
			r.add(c, testutil.FixtureKeyset, "L2", 1, mdGateFirstLine(keyset), "the heading counts 0 while rows are shown and GitLab sent no total")
		}
	}
}

// add records one finding, unless the case is excepted.
func (r *mdGateReport) add(c mdGateCase, state testutil.FixtureState, rule string, line int, text, detail string) {
	if _, excepted := mdGateExceptions[c.name]; excepted {
		return
	}
	r.findings = append(r.findings, mdGateFinding{kase: c, state: state.String(), rule: rule, line: line, text: text, detail: detail})
}

// mdGateSortedKeys returns a set's members in order.
func mdGateSortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// mdGateSameHints compares the hints ExtractHints read with the bullets the
// line model found under the heading.
func mdGateSameHints(got, bullets []string) bool {
	if len(got) != len(bullets) {
		return false
	}
	for i := range got {
		if strings.TrimSpace(got[i]) != strings.TrimSpace(bullets[i]) {
			return false
		}
	}
	return true
}

// mdGateHasFixtureLink reports whether any link points at a fixture URL.
func mdGateHasFixtureLink(links []string) bool {
	for _, link := range links {
		if strings.HasPrefix(link, testutil.FixtureURLBase) {
			return true
		}
	}
	return false
}

// mdGateHasPagination reports whether a type carries offset pagination at
// its top level, which is what the list heading rule reads.
func mdGateHasPagination(typ reflect.Type) bool {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return false
	}
	for field := range typ.Fields() {
		if field.Type == reflect.TypeFor[toolutil.PaginationOutput]() {
			return true
		}
	}
	return false
}

var mdGateHeadingCountRe = regexp.MustCompile(`^## .*\((\d+)\)\s*$`)

// mdGateHeadingCount reads the count a list heading reports, when it
// reports one as a bare number.
func mdGateHeadingCount(md string) (int, bool) {
	m := mdGateHeadingCountRe.FindStringSubmatch(mdGateFirstLine(md))
	if m == nil {
		return 0, false
	}
	n := 0
	for _, r := range m[1] {
		n = n*10 + int(r-'0')
	}
	return n, true
}

// mdGateFirstLine returns the first line of a render.
func mdGateFirstLine(md string) string {
	line, _, _ := strings.Cut(md, "\n")
	return line
}

// mdGateLog writes a report's findings to the test log: the counts, the
// packages, and every finding on its own line, so a run with -v is the work
// list and a run without is the summary.
func mdGateLog(t *testing.T, title string, findings []mdGateFinding) {
	t.Helper()
	byRule := map[string]int{}
	ruleNames := map[string]bool{}
	pkgs := map[string]bool{}
	for _, f := range findings {
		byRule[f.rule]++
		ruleNames[f.rule] = true
		pkgs[f.kase.pkg] = true
	}
	var rules []string
	for _, rule := range mdGateSortedKeys(ruleNames) {
		rules = append(rules, fmt.Sprintf("%s %d", rule, byRule[rule]))
	}
	t.Logf("%s: %d finding(s) in %d package(s); by rule: %s", title, len(findings), len(pkgs), strings.Join(rules, ", "))
	t.Logf("%s: packages: %s", title, strings.Join(mdGateSortedKeys(pkgs), " "))
	for _, f := range findings {
		t.Logf("finding: %s", f)
	}
}

// TestMarkdownRegistry_EveryFormatter_KeepsTableBoundaries drives every
// registered formatter and the renderers outside the registry through the
// line model and reports what the client would render differently from what
// the formatter wrote. It reports rather than fails, apart from the two
// assertions that keep it honest.
//
// The first is that it still sees the class it exists for: iterationdata opens
// a table and writes a list row into it, which ends the table with no body and
// leaves every later row on the page as literal pipes, and it has not been
// migrated yet.
//
// The second is the other half of the same proof, and is what the fixed
// formatter is worth: mergetrains was the sibling the audit proved broken, the
// card migration moved it onto [toolutil.Card], and nothing about it may be
// reported again. An assertion that it is still broken would have to be
// deleted by whoever fixed it, which is how a gate stops proving anything.
func TestMarkdownRegistry_EveryFormatter_KeepsTableBoundaries(t *testing.T) {
	report := mdGateScan(t)

	mdGateLog(t, "structural scan", report.findings)
	t.Logf("structural scan: %d case(s), %d render(s), %d silent render(s)", len(report.cases), report.rendered, report.silent)
	t.Run("iterationdata is reported", func(t *testing.T) {
		for _, f := range report.findings {
			if f.kase.pkg == "iterationdata" {
				return
			}
		}
		t.Error("the gate reports nothing for iterationdata, whose tables the audit proved broken, so the gate does not see the defect it exists for")
	})
	t.Run("mergetrains keeps its table boundaries", func(t *testing.T) {
		for _, f := range report.findings {
			if f.kase.pkg == "mergetrains" {
				t.Errorf("mergetrains was migrated onto the card and is reported again: %s", f)
			}
		}
	})
	for _, f := range report.findings {
		if f.rule == "P0" {
			t.Errorf("%s", f)
		}
	}
}

// TestMarkdownRegistry_Exceptions_NameACaseEach checks the discipline every
// declaration table here is held to: an exception that matches no case
// fails, so a fixed formatter cannot leave a stale one behind.
func TestMarkdownRegistry_Exceptions_NameACaseEach(t *testing.T) {
	report := mdGateScan(t)
	names := map[string]bool{}
	for _, c := range report.cases {
		names[c.name] = true
	}

	for name, reason := range mdGateExceptions {
		t.Run(name, func(t *testing.T) {
			if reason == "" {
				t.Errorf("the exception for %s gives no reason", name)
			}
			if !names[name] {
				t.Errorf("the exception for %s names no case the gate renders", name)
			}
		})
	}
}

// TestMarkdownRegistry_Registrations_HaveNoUndeclaredProblems pins the
// registrations the registry refused or could only half honor to the ones
// the tree has today, a baseline that may only shrink, so a new duplicate or
// a new interface-typed registration fails while the known ones are retired
// by the migration. The audit knew of two; the record showed twelve, because
// the shared note and discussion shapes are registered by every domain that
// renders them and the first init to run wins for all of them, which is the
// same defect as the runner token with more surfaces behind it. Eleven are
// left: the one interface-typed registration, groupimportexport's dispatcher
// over `any`, went with that package's card migration.
func TestMarkdownRegistry_Registrations_HaveNoUndeclaredProblems(t *testing.T) {
	got := toolutil.MarkdownRegistrationProblems()

	want := []string{
		"duplicate Markdown formatter for iterationdata.Output: the first registration is kept",
		"duplicate Markdown formatter for labeldata.Output: the first registration is kept",
		"duplicate Markdown formatter for runners.AuthTokenOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.DiscussionThreadNoteOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.DiscussionThreadNoteOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.DiscussionThreadNoteOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.DiscussionThreadOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.DiscussionThreadOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.DiscussionThreadOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.NoteOutput: the first registration is kept",
		"duplicate Markdown formatter for toolutil.NoteOutput: the first registration is kept",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("registration problems:\n got %q\nwant %q", got, want)
	}
}

// mdGateGlyphRules are the glyphs an absent value must not render as, each
// with what it reads as.
var mdGateGlyphRules = []struct {
	re     *regexp.Regexp
	detail string
}{
	{regexp.MustCompile(`@(?:\s|$|\||\*|\))`), "an @ with no handle"},
	{regexp.MustCompile(`\[\]\(|\]\(\)`), "a link with an empty half"},
	{regexp.MustCompile(`(?:\*\*:|:\*\*)\s*$`), "a label with nothing after it"},
	{regexp.MustCompile(`^\|[^|]*[^|\s][^|]*\|\s*\|\s*$`), "a two-cell row whose value cell is empty"},
	{regexp.MustCompile(`(?:^|[^\w])[#!]0(?:\D|$)`), "a zero reference"},
	{regexp.MustCompile(`0001-01-01`), "the zero time"},
	{regexp.MustCompile(`Page 0 of 0`), "a zero page"},
}

var mdGateZeroIDRe = regexp.MustCompile(`(?:^|[\s|:])0(?:[\s|]|$)`)

// TestMarkdownRegistry_AbsentValue_RendersNoGlyph re-renders every registered
// type with one optional field zeroed at a time and reads the lines that
// changed for a glyph that reads as data: an @ with no handle, an empty link
// half, a label with nothing after it, a zero reference or ID, the zero
// time. The oracle for optional is the struct's own declaration. It reports
// rather than fails.
func TestMarkdownRegistry_AbsentValue_RendersNoGlyph(t *testing.T) {
	report := mdGateScan(t)
	var findings []mdGateFinding
	for _, c := range report.cases {
		fields := c.optionalFields()
		if len(fields) == 0 {
			continue
		}
		base, rendered, _ := c.render(testutil.FixtureOptions{State: testutil.FixtureMultiPage})
		if !rendered {
			continue
		}
		baseLines := map[string]bool{}
		for line := range strings.SplitSeq(base, "\n") {
			baseLines[line] = true
		}
		baseRules := map[string]bool{}
		for _, rule := range testutil.ScanGFM(base).Rules() {
			baseRules[rule] = true
		}
		for _, field := range fields {
			findings = append(findings, mdGateAbsent(c, field, baseLines, baseRules)...)
		}
	}

	mdGateLog(t, "absent-value differential", findings)
}

// mdGateAbsent renders one case with one field zeroed and judges the lines
// that changed.
func mdGateAbsent(c mdGateCase, field testutil.FixtureField, baseLines, baseRules map[string]bool) []mdGateFinding {
	value := testutil.FillFixture(c.typ, testutil.FixtureOptions{State: testutil.FixtureMultiPage})
	if !testutil.ZeroField(value, field.Path) {
		return nil
	}
	md, rendered, panicked := mdGateRenderValue(value)
	if panicked != "" {
		return []mdGateFinding{{kase: c, state: "absent " + field.Name, rule: "P0", detail: "the formatter panicked with the field absent: " + panicked}}
	}
	if !rendered {
		return nil
	}
	var findings []mdGateFinding
	for i, line := range strings.Split(md, "\n") {
		if baseLines[line] {
			continue
		}
		for _, rule := range mdGateGlyphRules {
			if rule.re.MatchString(line) {
				findings = append(findings, mdGateFinding{kase: c, state: "absent " + field.Name, rule: "A1", line: i + 1, text: line, detail: rule.detail})
			}
		}
		if strings.HasSuffix(field.Name, "ID") && mdGateZeroIDRe.MatchString(line) {
			findings = append(findings, mdGateFinding{kase: c, state: "absent " + field.Name, rule: "A1", line: i + 1, text: line, detail: "an ID of 0"})
		}
	}
	for _, f := range testutil.ScanGFM(md).Findings {
		if !baseRules[f.Rule] {
			findings = append(findings, mdGateFinding{kase: c, state: "absent " + field.Name, rule: "A2", line: f.Line, text: f.Text, detail: f.Rule + " appears only with the field absent: " + f.Detail})
		}
	}
	return findings
}

// mdGateRenderValue renders one filled value through the registry.
func mdGateRenderValue(value reflect.Value) (md string, rendered bool, panicked string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprint(r)
		}
	}()
	result := toolutil.MarkdownForResult(value.Interface())
	if result == nil {
		return "", false, ""
	}
	md = extractTextContent(result)
	return md, md != "", ""
}

// mdGateHostile are the values a GitLab-authored string is set to, each
// aimed at one construct: a cell, a heading, a list item, a link, raw HTML,
// a fence, and the server's own guidance section.
var mdGateHostile = []struct {
	name    string
	payload string
}{
	{"pipe", "x|y"},
	{"heading", "x\n## injected"},
	{"item", "x\n- injected"},
	{"link", "x](http://attacker.invalid/y)"},
	{"html", `<a href="http://attacker.invalid">x</a>`},
	{"fence", "x\n```\ninjected"},
	{"guidance", "x\n" + testutil.GFMHintsHeading + "\n- injected"},
}

// TestMarkdownRegistry_HostileValues_ChangeNoStructure renders every case
// twice, with benign sentinels and with one hostile payload in every string
// field a directive in the package does not declare safe, and compares the
// structure outside quotes and fences: the count of headings, top-level
// items, table rows and cells, the hints ExtractHints reads, and the link
// destinations. It is the dynamic twin of the escaping gate, reading the
// bytes the client receives rather than the source, so it does not share the
// static walk's blind spots. It reports rather than fails.
func TestMarkdownRegistry_HostileValues_ChangeNoStructure(t *testing.T) {
	report := mdGateScan(t)
	exempt := map[string]map[string]bool{}
	var findings []mdGateFinding
	for _, c := range report.cases {
		if c.typ == nil && !strings.HasPrefix(c.name, "iterationdata.") && c.name != "elicitationtools.FormatResult" {
			continue
		}
		base, rendered, _ := c.render(testutil.FixtureOptions{State: testutil.FixtureMultiPage})
		if !rendered {
			continue
		}
		if _, known := exempt[c.pkg]; !known {
			exempt[c.pkg] = mdGateDirectiveFields(t, c)
		}
		baseDoc := testutil.ScanGFM(base)
		baseHints := toolutil.ExtractHints(base)
		for _, hostile := range mdGateHostile {
			text := mdGateHostileText(exempt[c.pkg], hostile.payload)
			md, hostileRendered, panicked := c.render(testutil.FixtureOptions{State: testutil.FixtureMultiPage, Text: text})
			if panicked != "" {
				findings = append(findings, mdGateFinding{kase: c, state: "hostile " + hostile.name, rule: "P0", detail: "the formatter panicked: " + panicked})
				continue
			}
			if !hostileRendered {
				continue
			}
			findings = append(findings, mdGateCompare(c, hostile.name, baseDoc, baseHints, md)...)
		}
	}

	mdGateLog(t, "hostile values", findings)
}

// mdGateHostileText supplies the string values of one hostile render: the
// payload in every field, except one its package declared safe and one whose
// name says it holds an address. An address field keeps its fixture URL,
// because a payload in a destination is a destination and not an injection,
// and the rule asks whether a value that is not an address can open a link.
func mdGateHostileText(exempt map[string]bool, payload string) func(string) string {
	return func(path string) string {
		for _, name := range mdGateFieldNames(path) {
			if exempt[name] || testutil.FixtureURLShaped(name) {
				return testutil.FixtureText(path)
			}
		}
		return payload
	}
}

// mdGateCompare reports every way a hostile render's structure differs from
// the benign one.
//
// X2, the raw-tag rule, reads the bare lines of the render rather than its
// content, which settles two questions the rule used to inherit rather than
// decide. A code span is not judged, because CommonMark makes its contents
// literal and HTML-escaped, so a value a formatter deliberately moved into
// one is contained and reporting it would condemn the fix. A blockquote line
// is judged, and that is the decision: the quote is containment against
// structure, so a heading or a bullet inside one cannot reach the document,
// and it is no containment at all against a tag, which a client renders as a
// live anchor wherever it sits. Every offending line is reported rather than
// the first, since stopping at one hid two findings of the class the rule
// exists to find.
func mdGateCompare(c mdGateCase, hostile string, base *testutil.GFMDocument, baseHints []string, md string) []mdGateFinding {
	doc := testutil.ScanGFM(md)
	state := "hostile " + hostile
	var findings []mdGateFinding
	report := func(detail string) {
		findings = append(findings, mdGateFinding{kase: c, state: state, rule: "X1", detail: detail})
	}
	if len(doc.Headings) != len(base.Headings) {
		report(fmt.Sprintf("the value adds or removes a heading: %d became %d", len(base.Headings), len(doc.Headings)))
	}
	if len(doc.Items) != len(base.Items) {
		report(fmt.Sprintf("the value adds or removes a top-level list item: %d became %d", len(base.Items), len(doc.Items)))
	}
	if doc.Rows != base.Rows || doc.Cells != base.Cells {
		report(fmt.Sprintf("the value changes the table: %d row(s) and %d cell(s) became %d and %d", base.Rows, base.Cells, doc.Rows, doc.Cells))
	}
	if hints := toolutil.ExtractHints(md); len(hints) != len(baseHints) {
		report(fmt.Sprintf("the value adds or removes a hint: %d became %d", len(baseHints), len(hints)))
	}
	for _, dest := range mdGateNewForeignLinks(base.Links, doc.Links) {
		line, text := mdGateLinkLine(doc, dest)
		findings = append(findings, mdGateFinding{
			kase: c, state: state, rule: "X1", line: line, text: text,
			detail: "the value opens a link to " + dest + ", a destination the fixture never named",
		})
	}
	if hostile == "html" {
		for _, line := range doc.Bare {
			if strings.Contains(line.Bare, `<a href="http://attacker.invalid">`) {
				findings = append(findings, mdGateFinding{kase: c, state: state, rule: "X2", line: line.Line, text: line.Text, detail: "the value reaches the page as a raw tag"})
			}
		}
	}
	return findings
}

// mdGateNewForeignLinks names the destinations behind the links off the
// fixture's origin that the hostile render opens and the benign one did not.
//
// How many there are is still a count, and deliberately: a formatter whose
// destination half is a field the fixture does not recognize as an address
// links off-origin in both renders, once with the sentinel and once with the
// payload, and the value has opened nothing there. What the count cannot do
// is say which destination or which line, so the destinations the benign
// render did not carry are named, as many of them as the count says were
// added, and each is reported on its own line.
func mdGateNewForeignLinks(base, hostile []string) []string {
	carried := map[string]int{}
	before := 0
	for _, link := range base {
		if !strings.HasPrefix(link, testutil.FixtureURLBase) {
			carried[link]++
			before++
		}
	}
	after := 0
	var added []string
	for _, link := range hostile {
		if strings.HasPrefix(link, testutil.FixtureURLBase) {
			continue
		}
		after++
		if carried[link] > 0 {
			carried[link]--
			continue
		}
		added = append(added, link)
	}
	excess := after - before
	if excess <= 0 {
		return nil
	}
	return added[:min(excess, len(added))]
}

// mdGateLinkLine finds the line a destination is linked from, so the finding
// quotes the text a reader has to fix. It answers 0 and no text when no line
// carries the destination, which a link the model itself read cannot be.
func mdGateLinkLine(doc *testutil.GFMDocument, dest string) (int, string) {
	needle := "](" + dest + ")"
	for i, line := range doc.Lines {
		if strings.Contains(line, needle) {
			return i + 1, line
		}
	}
	return 0, ""
}

// mdGateFieldNames returns the names the last field of a fixture path may be
// declared safe under: the segment as written, and every segment left by
// removing its trailing digits one at a time.
//
// Both spellings are needed and neither is right alone. The filler appends a
// slice index to the path, so ".Items0" is the field Items and the digit has
// to go; but ".SHA256" is a field whose own name ends in digits, and trimming
// them looked it up as SHA, which silently un-exempted three outputs their
// packages had already declared safe. Generating both errs toward exempting
// too much, which is this join's policy: a spelling no field carries matches
// no declaration either.
func mdGateFieldNames(path string) []string {
	name := path
	if i := strings.LastIndex(path, "."); i >= 0 {
		name = path[i+1:]
	}
	names := []string{name}
	for trimmed := name; len(trimmed) > 0 && trimmed[len(trimmed)-1] >= '0' && trimmed[len(trimmed)-1] <= '9'; {
		trimmed = trimmed[:len(trimmed)-1]
		if trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

var mdGateDirectiveRe = regexp.MustCompile(`//gitlab:allow-unescaped\s+([^:]+):`)

var mdGateSelectorRe = regexp.MustCompile(`\.([A-Za-z_]\w*)`)

var mdGateIdentRe = regexp.MustCompile(`[A-Za-z_]\w*`)

// mdGateDirectiveFields reads the field names the escaping directives of a
// case's package declare safe, mapped by the selectors of each expression,
// which errs toward exempting too much rather than inventing a finding.
//
// A directive is often written on a local variable rather than on the field
// itself, because a directive's expression is cut at its first colon and a
// slice expression carries one. Such a declaration names no selector at all,
// so reading selectors alone mapped it to nothing and left the field it
// covers un-exempt: mrchanges declares the truncated commit SHA under the
// name it gave the local, and the field behind it is ShortID. The identifier
// is therefore looked up in the package's own source and the selectors of
// whatever it is assigned are declared safe too, which is as far as a join on
// names can follow it.
func mdGateDirectiveFields(t *testing.T, c mdGateCase) map[string]bool {
	t.Helper()
	dir := c.pkg
	if c.pkg == "toolutil" {
		dir = filepath.Join("..", "toolutil")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return map[string]bool{}
	}
	var sources []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			t.Fatalf("read %s: %v", entry.Name(), readErr)
		}
		sources = append(sources, string(src))
	}
	fields := map[string]bool{}
	for _, src := range sources {
		for _, m := range mdGateDirectiveRe.FindAllStringSubmatch(src, -1) {
			selectors := mdGateSelectorRe.FindAllStringSubmatch(m[1], -1)
			for _, sel := range selectors {
				fields[sel[1]] = true
			}
			if len(selectors) > 0 {
				continue
			}
			for _, ident := range mdGateIdentRe.FindAllString(m[1], -1) {
				fields[ident] = true
				for _, name := range mdGateAssignedFields(sources, ident) {
					fields[name] = true
				}
			}
		}
	}
	return fields
}

// mdGateAssignedFields returns the selectors of every value a local of this
// name is assigned in the package: "short := v.HeadCommitSHA" and
// "short = c.ShortID" declare both fields safe under the one name the
// directive could be written on. A line carrying a second "=" is left out, so
// a comparison is never read as an assignment.
func mdGateAssignedFields(sources []string, ident string) []string {
	assignment := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(ident) + `[ \t]*:?=[^=\n]*$`)
	var names []string
	for _, src := range sources {
		for _, line := range assignment.FindAllString(src, -1) {
			for _, sel := range mdGateSelectorRe.FindAllStringSubmatch(line, -1) {
				names = append(names, sel[1])
			}
		}
	}
	return names
}
