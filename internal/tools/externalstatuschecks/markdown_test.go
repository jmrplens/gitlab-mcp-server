// markdown_test.go contains unit tests for external status check Markdown
// formatting functions.
package externalstatuschecks

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The guidance sections the four renders close with, kept here so a test
// compares the whole output without repeating the wording four times.
const (
	mergeCheckHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'external_status_check.set_project_mr_status' to record this check's result on the merge request\n" +
		"- Use action 'external_status_check.retry_project' to retry a failed check\n" +
		"- Use action 'external_status_check.list_project_mr_checks' to see the merge request's other checks\n"

	projectCheckHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'external_status_check.update_project' to change this check's name, URL or branch scope\n" +
		"- Use action 'external_status_check.delete_project' to remove this check\n" +
		"- Use action 'external_status_check.list_project' to see the project's other checks\n"

	mergeListHints = "\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'external_status_check.set_project_mr_status' to record a check's result on the merge request\n" +
		"- Use action 'external_status_check.retry_project' to retry a failed check\n"

	projectListHints = "\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'external_status_check.create_project' to add a check to this project\n" +
		"- Use action 'external_status_check.update_project' to change one check's name, URL or branch scope\n" +
		"- Use action 'external_status_check.delete_project' to remove a check\n"
)

// TestFormatMergeCheckMarkdown verifies that one merge request status check
// renders as a card whose external URL is a link. The whole render is
// compared: a substring assertion passes on a row that landed outside the
// block it was meant for, which is the defect class this migration closes.
func TestFormatMergeCheckMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input MergeStatusCheckOutput
		want  string
	}{
		{
			name: "all fields present",
			input: MergeStatusCheckOutput{
				ID:          1,
				Name:        "CI Check",
				ExternalURL: "https://ci.example.com",
				Status:      "passed",
			},
			want: "## External Status Check: CI Check\n\n" +
				"- **ID**: 1\n" +
				"- **Name**: CI Check\n" +
				"- **Status**: passed\n" +
				"- **External URL**: [https://ci.example.com](https://ci.example.com)\n" +
				mergeCheckHints,
		},
		{
			name: "failed status",
			input: MergeStatusCheckOutput{
				ID:          99,
				Name:        "Security Scan",
				ExternalURL: "https://scan.example.com",
				Status:      "failed",
			},
			want: "## External Status Check: Security Scan\n\n" +
				"- **ID**: 99\n" +
				"- **Name**: Security Scan\n" +
				"- **Status**: failed\n" +
				"- **External URL**: [https://scan.example.com](https://scan.example.com)\n" +
				mergeCheckHints,
		},
		{
			name:  "no external URL",
			input: MergeStatusCheckOutput{ID: 3, Name: "Manual", Status: "pending"},
			want: "## External Status Check: Manual\n\n" +
				"- **ID**: 3\n" +
				"- **Name**: Manual\n" +
				"- **Status**: pending\n" +
				mergeCheckHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatMergeCheckMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatMergeCheckMarkdown() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestFormatProjectCheckMarkdown verifies that a project status check renders
// as a card whose branch scope is stated either way: the branches it names as
// a collection under their own heading, and "All branches" when it names none,
// which is what an empty list means and what the card used to render as
// nothing at all.
func TestFormatProjectCheckMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input ProjectStatusCheckOutput
		want  string
	}{
		{
			name: "with protected branches",
			input: ProjectStatusCheckOutput{
				ID:          42,
				Name:        "Security Scan",
				ProjectID:   1,
				ExternalURL: "https://scan.example.com",
				HMAC:        true,
				ProtectedBranches: []ProtectedBranchOutput{
					{ID: 100, ProjectID: 1, Name: "main", CodeOwnerApprovalRequired: false},
					{ID: 101, ProjectID: 1, Name: "develop", CodeOwnerApprovalRequired: true},
				},
			},
			want: "## External Status Check: Security Scan\n\n" +
				"- **ID**: 42\n" +
				"- **Name**: Security Scan\n" +
				"- **Project ID**: 1\n" +
				"- **External URL**: [https://scan.example.com](https://scan.example.com)\n" +
				"- **HMAC**: ✅\n\n" +
				"### Protected Branches\n\n" +
				"| ID | Name | Code Owner Approval |\n" +
				"| --- | --- | --- |\n" +
				"| 100 | main | ❌ |\n" +
				"| 101 | develop | ✅ |\n" +
				projectCheckHints,
		},
		{
			name: "without protected branches applies everywhere",
			input: ProjectStatusCheckOutput{
				ID:          10,
				Name:        "Lint Check",
				ProjectID:   5,
				ExternalURL: "https://lint.example.com",
				HMAC:        false,
			},
			want: "## External Status Check: Lint Check\n\n" +
				"- **ID**: 10\n" +
				"- **Name**: Lint Check\n" +
				"- **Project ID**: 5\n" +
				"- **External URL**: [https://lint.example.com](https://lint.example.com)\n" +
				"- **HMAC**: ❌\n" +
				"- **Protected Branches**: All branches\n" +
				projectCheckHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatProjectCheckMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatProjectCheckMarkdown() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestFormatProjectCheckMarkdown_HostileBranchName verifies that a branch name
// cannot end its cell or open a construct of its own: git check-ref-format
// permits the pipe and the angle bracket, and the cell escaper neutralizes
// both.
func TestFormatProjectCheckMarkdown_HostileBranchName(t *testing.T) {
	got := FormatProjectCheckMarkdown(ProjectStatusCheckOutput{
		ID:                1,
		Name:              "Check",
		ProjectID:         2,
		ProtectedBranches: []ProtectedBranchOutput{{ID: 5, Name: "a|b<c>[d](http://attacker.invalid)"}},
	})

	want := "## External Status Check: Check\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: Check\n" +
		"- **Project ID**: 2\n" +
		"- **HMAC**: ❌\n\n" +
		"### Protected Branches\n\n" +
		"| ID | Name | Code Owner Approval |\n" +
		"| --- | --- | --- |\n" +
		"| 5 | a&#124;b&lt;c>&#91;d](http://attacker.invalid) | ❌ |\n" +
		projectCheckHints

	if got != want {
		t.Errorf("FormatProjectCheckMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMergeMarkdown verifies that a page of merge request status
// checks renders as one table with a linked URL column, under a heading
// counting what the response reports.
func TestFormatListMergeMarkdown(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		got := FormatListMergeMarkdown(ListMergeStatusCheckOutput{})
		if want := "No merge status checks found.\n"; got != want {
			t.Errorf("FormatListMergeMarkdown() = %q, want %q", got, want)
		}
	})

	t.Run("populated list", func(t *testing.T) {
		got := FormatListMergeMarkdown(ListMergeStatusCheckOutput{
			Items: []MergeStatusCheckOutput{
				{ID: 1, Name: "CI", ExternalURL: "https://ci.example.com", Status: "passed"},
				{ID: 2, Name: "Security", ExternalURL: "https://sec.example.com", Status: "failed"},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, TotalPages: 1},
		})

		want := "## Merge Status Checks (2)\n\n" +
			"| ID | Name | External URL | Status |\n" +
			"| --- | --- | --- | --- |\n" +
			"| 1 | CI | [https://ci.example.com](https://ci.example.com) | passed |\n" +
			"| 2 | Security | [https://sec.example.com](https://sec.example.com) | failed |\n\n" +
			"Page 1 of 1 | 2 items total\n" +
			mergeListHints

		if got != want {
			t.Errorf("FormatListMergeMarkdown() =\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("keyset page counts what is shown", func(t *testing.T) {
		got := FormatListMergeMarkdown(ListMergeStatusCheckOutput{
			Items:      []MergeStatusCheckOutput{{ID: 1, Name: "CI", Status: "passed"}},
			Pagination: toolutil.PaginationOutput{Page: 1, HasMore: true},
		})
		if !strings.HasPrefix(got, "## Merge Status Checks (1 shown, more available)\n\n") {
			t.Errorf("FormatListMergeMarkdown() =\n%s", got)
		}
	})
}

// TestFormatListProjectMarkdown verifies that a page of project status checks
// renders as one table whose branch column says what the check is scoped to,
// so a check naming no branch reads as applying everywhere rather than
// nowhere.
func TestFormatListProjectMarkdown(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		got := FormatListProjectMarkdown(ListProjectStatusCheckOutput{})
		if want := "No project external status checks found.\n"; got != want {
			t.Errorf("FormatListProjectMarkdown() = %q, want %q", got, want)
		}
	})

	t.Run("populated list", func(t *testing.T) {
		got := FormatListProjectMarkdown(ListProjectStatusCheckOutput{
			Items: []ProjectStatusCheckOutput{
				{
					ID: 1, Name: "CI", ProjectID: 10,
					ExternalURL: "https://ci.example.com", HMAC: true,
					ProtectedBranches: []ProtectedBranchOutput{{ID: 100, Name: "main"}},
				},
				{
					ID: 2, Name: "Lint", ProjectID: 10,
					ExternalURL: "https://lint.example.com", HMAC: false,
				},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, TotalPages: 1},
		})

		want := "## Project External Status Checks (2)\n\n" +
			"| ID | Name | External URL | HMAC | Protected Branches |\n" +
			"| --- | --- | --- | --- | --- |\n" +
			"| 1 | CI | [https://ci.example.com](https://ci.example.com) | ✅ | 1 |\n" +
			"| 2 | Lint | [https://lint.example.com](https://lint.example.com) | ❌ | All branches |\n\n" +
			"Page 1 of 1 | 2 items total\n" +
			projectListHints

		if got != want {
			t.Errorf("FormatListProjectMarkdown() =\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("keyset page counts what is shown", func(t *testing.T) {
		got := FormatListProjectMarkdown(ListProjectStatusCheckOutput{
			Items:      []ProjectStatusCheckOutput{{ID: 1, Name: "CI", ProjectID: 10}},
			Pagination: toolutil.PaginationOutput{Page: 1, HasMore: true},
		})
		if !strings.HasPrefix(got, "## Project External Status Checks (1 shown, more available)\n\n") {
			t.Errorf("FormatListProjectMarkdown() =\n%s", got)
		}
	})
}
