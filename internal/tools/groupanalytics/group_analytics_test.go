package groupanalytics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// --- GetIssuesCount ---

// TestGetIssuesCount validates the GetIssuesCount handler with table-driven
// cases covering: successful retrieval, zero count, nested group paths,
// missing group_path validation, context cancellation, and various API errors.
func TestGetIssuesCount(t *testing.T) {
	tests := []struct {
		name       string
		input      IssuesCountInput
		cancelCtx  bool
		mockStatus int
		mockBody   string
		wantErr    bool
		wantCount  int64
		wantGroup  string
	}{
		{
			name:       "returns issues count for valid group",
			input:      IssuesCountInput{GroupPath: "my-group"},
			mockStatus: http.StatusOK,
			mockBody:   `{"issues_count":42}`,
			wantCount:  42,
			wantGroup:  "my-group",
		},
		{
			name:       "returns zero count",
			input:      IssuesCountInput{GroupPath: "empty-group"},
			mockStatus: http.StatusOK,
			mockBody:   `{"issues_count":0}`,
			wantCount:  0,
			wantGroup:  "empty-group",
		},
		{
			name:       "handles nested group path",
			input:      IssuesCountInput{GroupPath: "parent/child/grandchild"},
			mockStatus: http.StatusOK,
			mockBody:   `{"issues_count":7}`,
			wantCount:  7,
			wantGroup:  "parent/child/grandchild",
		},
		{
			name:    "returns error when group_path is empty",
			input:   IssuesCountInput{},
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     IssuesCountInput{GroupPath: "my-group"},
			cancelCtx: true,
			wantErr:   true,
		},
		{
			name:       "returns error on 403 forbidden",
			input:      IssuesCountInput{GroupPath: "forbidden-group"},
			mockStatus: http.StatusForbidden,
			mockBody:   `{"message":"403 Forbidden"}`,
			wantErr:    true,
		},
		{
			name:       "returns error on 404 not found",
			input:      IssuesCountInput{GroupPath: "nonexistent"},
			mockStatus: http.StatusNotFound,
			mockBody:   `{"message":"404 Group Not Found"}`,
			wantErr:    true,
		},
		{
			name:       "returns error on 500 server error",
			input:      IssuesCountInput{GroupPath: "error-group"},
			mockStatus: http.StatusInternalServerError,
			mockBody:   `{"message":"500 Internal Server Error"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/analytics/group_activity/issues_count")
				testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
			}))

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			out, err := GetIssuesCount(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetIssuesCount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if out.IssuesCount != tt.wantCount {
				t.Errorf("IssuesCount = %d, want %d", out.IssuesCount, tt.wantCount)
			}
			if out.GroupPath != tt.wantGroup {
				t.Errorf("GroupPath = %q, want %q", out.GroupPath, tt.wantGroup)
			}
		})
	}
}

// --- GetMRCount ---

// TestGetMRCount validates the GetMRCount handler with table-driven cases
// covering: successful retrieval, zero count, nested paths, missing input,
// context cancellation, and API errors (403, 404, 500).
func TestGetMRCount(t *testing.T) {
	tests := []struct {
		name       string
		input      MRCountInput
		cancelCtx  bool
		mockStatus int
		mockBody   string
		wantErr    bool
		wantCount  int64
		wantGroup  string
	}{
		{
			name:       "returns MR count for valid group",
			input:      MRCountInput{GroupPath: "parent/child"},
			mockStatus: http.StatusOK,
			mockBody:   `{"merge_requests_count":17}`,
			wantCount:  17,
			wantGroup:  "parent/child",
		},
		{
			name:       "returns zero MR count",
			input:      MRCountInput{GroupPath: "quiet-group"},
			mockStatus: http.StatusOK,
			mockBody:   `{"merge_requests_count":0}`,
			wantCount:  0,
			wantGroup:  "quiet-group",
		},
		{
			name:       "handles large count value",
			input:      MRCountInput{GroupPath: "busy-group"},
			mockStatus: http.StatusOK,
			mockBody:   `{"merge_requests_count":99999}`,
			wantCount:  99999,
			wantGroup:  "busy-group",
		},
		{
			name:    "returns error when group_path is empty",
			input:   MRCountInput{},
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     MRCountInput{GroupPath: "my-group"},
			cancelCtx: true,
			wantErr:   true,
		},
		{
			name:       "returns error on 403 forbidden",
			input:      MRCountInput{GroupPath: "forbidden"},
			mockStatus: http.StatusForbidden,
			mockBody:   `{"message":"403 Forbidden"}`,
			wantErr:    true,
		},
		{
			name:       "returns error on 404 not found",
			input:      MRCountInput{GroupPath: "missing"},
			mockStatus: http.StatusNotFound,
			mockBody:   `{"message":"404 Not Found"}`,
			wantErr:    true,
		},
		{
			name:       "returns error on 500 server error",
			input:      MRCountInput{GroupPath: "broken"},
			mockStatus: http.StatusInternalServerError,
			mockBody:   `{"message":"Internal Server Error"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/analytics/group_activity/merge_requests_count")
				testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
			}))

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			out, err := GetMRCount(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetMRCount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if out.MergeRequestsCount != tt.wantCount {
				t.Errorf("MergeRequestsCount = %d, want %d", out.MergeRequestsCount, tt.wantCount)
			}
			if out.GroupPath != tt.wantGroup {
				t.Errorf("GroupPath = %q, want %q", out.GroupPath, tt.wantGroup)
			}
		})
	}
}

// --- GetMembersCount ---

// TestGetMembersCount validates the GetMembersCount handler with table-driven
// cases covering: successful retrieval, zero count, nested paths, missing input,
// context cancellation, and API errors (403, 404, 500).
func TestGetMembersCount(t *testing.T) {
	tests := []struct {
		name       string
		input      MembersCountInput
		cancelCtx  bool
		mockStatus int
		mockBody   string
		wantErr    bool
		wantCount  int64
		wantGroup  string
	}{
		{
			name:       "returns members count for valid group",
			input:      MembersCountInput{GroupPath: "my-org"},
			mockStatus: http.StatusOK,
			mockBody:   `{"new_members_count":5}`,
			wantCount:  5,
			wantGroup:  "my-org",
		},
		{
			name:       "returns zero members count",
			input:      MembersCountInput{GroupPath: "stable-group"},
			mockStatus: http.StatusOK,
			mockBody:   `{"new_members_count":0}`,
			wantCount:  0,
			wantGroup:  "stable-group",
		},
		{
			name:       "handles deeply nested group path",
			input:      MembersCountInput{GroupPath: "a/b/c/d"},
			mockStatus: http.StatusOK,
			mockBody:   `{"new_members_count":3}`,
			wantCount:  3,
			wantGroup:  "a/b/c/d",
		},
		{
			name:    "returns error when group_path is empty",
			input:   MembersCountInput{},
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     MembersCountInput{GroupPath: "my-org"},
			cancelCtx: true,
			wantErr:   true,
		},
		{
			name:       "returns error on 403 forbidden",
			input:      MembersCountInput{GroupPath: "private"},
			mockStatus: http.StatusForbidden,
			mockBody:   `{"message":"403 Forbidden"}`,
			wantErr:    true,
		},
		{
			name:       "returns error on 404 not found",
			input:      MembersCountInput{GroupPath: "gone"},
			mockStatus: http.StatusNotFound,
			mockBody:   `{"message":"Not Found"}`,
			wantErr:    true,
		},
		{
			name:       "returns error on 500 server error",
			input:      MembersCountInput{GroupPath: "broken"},
			mockStatus: http.StatusInternalServerError,
			mockBody:   `{"message":"Internal Server Error"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/analytics/group_activity/new_members_count")
				testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
			}))

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			out, err := GetMembersCount(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetMembersCount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if out.NewMembersCount != tt.wantCount {
				t.Errorf("NewMembersCount = %d, want %d", out.NewMembersCount, tt.wantCount)
			}
			if out.GroupPath != tt.wantGroup {
				t.Errorf("GroupPath = %q, want %q", out.GroupPath, tt.wantGroup)
			}
		})
	}
}

// --- Markdown Formatters ---

// TestFormatIssuesCountMarkdown verifies the Markdown output for recently
// created issues count, checking header, table structure, values, and hints.
func TestFormatIssuesCountMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    IssuesCountOutput
		contains []string
	}{
		{
			name: "formats non-zero count",
			input: IssuesCountOutput{
				GroupPath:   "my-group",
				IssuesCount: 42,
			},
			contains: []string{
				"## Recently Created Issues Count",
				"| Group | `my-group` |",
				"| Issues Count (last 90 days) | **42** |",
				"gitlab_get_recently_created_mr_count",
				"gitlab_issue_list_group",
			},
		},
		{
			name: "formats zero count",
			input: IssuesCountOutput{
				GroupPath:   "empty-group",
				IssuesCount: 0,
			},
			contains: []string{
				"| Group | `empty-group` |",
				"| Issues Count (last 90 days) | **0** |",
			},
		},
		{
			name: "formats nested group path",
			input: IssuesCountOutput{
				GroupPath:   "parent/child",
				IssuesCount: 100,
			},
			contains: []string{
				"| Group | `parent/child` |",
				"**100**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatIssuesCountMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

// TestFormatMRCountMarkdown verifies the Markdown output for recently created
// merge requests count, checking header, table structure, and hints.
func TestFormatMRCountMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    MRCountOutput
		contains []string
	}{
		{
			name: "formats non-zero MR count",
			input: MRCountOutput{
				GroupPath:          "dev-team",
				MergeRequestsCount: 17,
			},
			contains: []string{
				"## Recently Created Merge Requests Count",
				"| Group | `dev-team` |",
				"| Merge Requests Count (last 90 days) | **17** |",
				"gitlab_get_recently_created_issues_count",
				"gitlab_mr_list_group",
			},
		},
		{
			name: "formats zero MR count",
			input: MRCountOutput{
				GroupPath:          "quiet-team",
				MergeRequestsCount: 0,
			},
			contains: []string{
				"| Group | `quiet-team` |",
				"**0**",
			},
		},
		{
			name: "formats large MR count",
			input: MRCountOutput{
				GroupPath:          "mega-corp/platform",
				MergeRequestsCount: 99999,
			},
			contains: []string{
				"| Group | `mega-corp/platform` |",
				"**99999**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMRCountMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

// TestFormatMembersCountMarkdown verifies the Markdown output for recently
// added members count, checking header, table structure, and hints.
func TestFormatMembersCountMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    MembersCountOutput
		contains []string
	}{
		{
			name: "formats non-zero members count",
			input: MembersCountOutput{
				GroupPath:       "my-org",
				NewMembersCount: 5,
			},
			contains: []string{
				"## Recently Added Members Count",
				"| Group | `my-org` |",
				"| New Members Count (last 90 days) | **5** |",
				"gitlab_group_members_list",
				"gitlab_get_recently_created_issues_count",
			},
		},
		{
			name: "formats zero members count",
			input: MembersCountOutput{
				GroupPath:       "stable-org",
				NewMembersCount: 0,
			},
			contains: []string{
				"| Group | `stable-org` |",
				"**0**",
			},
		},
		{
			name: "formats deeply nested group path",
			input: MembersCountOutput{
				GroupPath:       "a/b/c/d",
				NewMembersCount: 1,
			},
			contains: []string{
				"| Group | `a/b/c/d` |",
				"**1**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMembersCountMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}
