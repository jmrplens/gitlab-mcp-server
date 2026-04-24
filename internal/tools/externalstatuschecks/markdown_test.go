// markdown_test.go contains unit tests for external status check Markdown
// formatting functions.
package externalstatuschecks

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestFormatMergeCheckMarkdown verifies single merge status check markdown
// rendering covers ID, name, external URL, and status fields plus hints.
func TestFormatMergeCheckMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    MergeStatusCheckOutput
		contains []string
	}{
		{
			name: "all fields present",
			input: MergeStatusCheckOutput{
				ID:          1,
				Name:        "CI Check",
				ExternalURL: "https://ci.example.com",
				Status:      "passed",
			},
			contains: []string{
				"CI Check",
				"1",
				"https://ci.example.com",
				"passed",
				"gitlab_set_project_mr_external_status_check_status",
				"gitlab_retry_failed_external_status_check_for_project_mr",
			},
		},
		{
			name: "failed status",
			input: MergeStatusCheckOutput{
				ID:          99,
				Name:        "Security Scan",
				ExternalURL: "https://scan.example.com",
				Status:      "failed",
			},
			contains: []string{"Security Scan", "99", "failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMergeCheckMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatProjectCheckMarkdown verifies single project status check
// markdown rendering covers all fields, protected branches, and hints.
func TestFormatProjectCheckMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ProjectStatusCheckOutput
		contains []string
		excludes []string
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
			contains: []string{
				"Security Scan",
				"42",
				"https://scan.example.com",
				"main",
				"develop",
				"Protected Branches",
				"gitlab_update_project_external_status_check",
				"gitlab_delete_project_external_status_check",
			},
		},
		{
			name: "without protected branches",
			input: ProjectStatusCheckOutput{
				ID:          10,
				Name:        "Lint Check",
				ProjectID:   5,
				ExternalURL: "https://lint.example.com",
				HMAC:        false,
			},
			contains: []string{"Lint Check", "10"},
			excludes: []string{"Protected Branches"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatProjectCheckMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatListMergeMarkdown verifies merge status check list rendering
// covers empty results, populated lists with table rows, and pagination.
func TestFormatListMergeMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListMergeStatusCheckOutput
		contains []string
	}{
		{
			name: "empty list",
			input: ListMergeStatusCheckOutput{
				Items:      nil,
				Pagination: toolutil.PaginationOutput{},
			},
			contains: []string{"No merge status checks found"},
		},
		{
			name: "populated list",
			input: ListMergeStatusCheckOutput{
				Items: []MergeStatusCheckOutput{
					{ID: 1, Name: "CI", ExternalURL: "https://ci.example.com", Status: "passed"},
					{ID: 2, Name: "Security", ExternalURL: "https://sec.example.com", Status: "failed"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, TotalPages: 1},
			},
			contains: []string{
				"Merge Status Checks (2)",
				"| ID | Name | External URL | Status |",
				"CI",
				"Security",
				"passed",
				"failed",
				"gitlab_set_project_mr_external_status_check_status",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMergeMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatListProjectMarkdown verifies project status check list rendering
// covers empty results, populated lists with HMAC/branches columns, and hints.
func TestFormatListProjectMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListProjectStatusCheckOutput
		contains []string
	}{
		{
			name: "empty list",
			input: ListProjectStatusCheckOutput{
				Items:      nil,
				Pagination: toolutil.PaginationOutput{},
			},
			contains: []string{"No project external status checks found"},
		},
		{
			name: "populated list",
			input: ListProjectStatusCheckOutput{
				Items: []ProjectStatusCheckOutput{
					{
						ID: 1, Name: "CI", ProjectID: 10,
						ExternalURL: "https://ci.example.com", HMAC: true,
						ProtectedBranches: []ProtectedBranchOutput{
							{ID: 100, Name: "main"},
						},
					},
					{
						ID: 2, Name: "Lint", ProjectID: 10,
						ExternalURL: "https://lint.example.com", HMAC: false,
					},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, TotalPages: 1},
			},
			contains: []string{
				"Project External Status Checks (2)",
				"| ID | Name | External URL | HMAC | Protected Branches |",
				"CI",
				"Lint",
				"gitlab_create_project_external_status_check",
				"gitlab_update_project_external_status_check",
				"gitlab_delete_project_external_status_check",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListProjectMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}
