// markdown_quality_test.go validates transversal quality patterns across all
// Markdown formatters: headers, timestamp formatting, and web URL presence.

package tools

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
)

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
				Author: "alice", CreatedAt: "2026-02-01 09:00",
			},
			expectWebURL: true,
		},
		{
			name: "issues.ListOutput",
			result: issues.ListOutput{
				Issues: []issues.Output{
					{ID: 10, IID: 5, Title: "Fix login", State: "opened", Author: "alice"},
					{ID: 11, IID: 6, Title: "Add tests", State: "closed", Author: "bob"},
				},
			},
		},
		{
			name: "mergerequests.Output",
			result: mergerequests.Output{
				ID: 20, IID: 3, Title: "Feature branch", State: "opened",
				SourceBranch: "feature/x", TargetBranch: "main", Author: "alice",
				WebURL:    "https://gitlab.example.com/group/project/-/merge_requests/3",
				CreatedAt: "2026-03-10 14:00",
			},
			expectWebURL: true,
		},
		{
			name: "mergerequests.ListOutput",
			result: mergerequests.ListOutput{
				MergeRequests: []mergerequests.Output{
					{ID: 20, IID: 3, Title: "MR 1", State: "opened", Author: "alice", SourceBranch: "f1", TargetBranch: "main"},
				},
			},
		},
		{
			name: "branches.Output",
			result: branches.Output{
				Name: "feature/auth", Protected: false, Merged: false,
				CommitID: "abc123def",
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
				Name: "v1.0.0", CommitSHA: "abc123", Message: "Release v1.0.0",
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
				t.Errorf("raw ISO timestamp found: %q — use toolutil.FormatTime", match)
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
