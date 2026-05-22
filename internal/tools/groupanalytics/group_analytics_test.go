// group_analytics_test.go contains unit tests for GitLab group analytics
// operations. Tests use httptest to mock the GitLab Group Analytics API.
package groupanalytics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// --- Count handlers ---

type countHandlerCase struct {
	name       string
	groupPath  string
	cancelCtx  bool
	mockStatus int
	mockBody   string
	wantErr    bool
	wantCount  int64
}

type countHandler struct {
	name    string
	path    string
	jsonKey string
	call    func(context.Context, *gitlabclient.Client, string) (int64, string, error)
}

// TestCountHandlers validates the GitLab group analytics count handlers.
// It covers success, zero counts, nested group paths, missing input, context
// cancellation, and API errors for issues, merge requests, and new members.
func TestCountHandlers(t *testing.T) {
	handlers := []countHandler{
		{name: "issues", path: "/api/v4/analytics/group_activity/issues_count", jsonKey: "issues_count", call: callIssuesCount},
		{name: "merge requests", path: "/api/v4/analytics/group_activity/merge_requests_count", jsonKey: "merge_requests_count", call: callMRCount},
		{name: "members", path: "/api/v4/analytics/group_activity/new_members_count", jsonKey: "new_members_count", call: callMembersCount},
	}

	for _, handler := range handlers {
		t.Run(handler.name, func(t *testing.T) {
			runCountHandlerCases(t, handler)
		})
	}
}

func runCountHandlerCases(t *testing.T, handler countHandler) {
	t.Helper()
	tests := []countHandlerCase{
		{name: "returns count for valid group", groupPath: "my-group", mockStatus: http.StatusOK, mockBody: countBody(handler.jsonKey, 42), wantCount: 42},
		{name: "returns zero count", groupPath: "empty-group", mockStatus: http.StatusOK, mockBody: countBody(handler.jsonKey, 0), wantCount: 0},
		{name: "handles nested group path", groupPath: "parent/child/grandchild", mockStatus: http.StatusOK, mockBody: countBody(handler.jsonKey, 7), wantCount: 7},
		{name: "returns error when group_path is empty", wantErr: true},
		{name: "returns error when context is cancelled", groupPath: "my-group", cancelCtx: true, wantErr: true},
		{name: "returns error on 403 forbidden", groupPath: "forbidden-group", mockStatus: http.StatusForbidden, mockBody: `{"message":"403 Forbidden"}`, wantErr: true},
		{name: "returns error on 404 not found", groupPath: "nonexistent", mockStatus: http.StatusNotFound, mockBody: `{"message":"404 Group Not Found"}`, wantErr: true},
		{name: "returns error on 500 server error", groupPath: "error-group", mockStatus: http.StatusForbidden, mockBody: `{"message":"server error"}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, handler.path)
				testutil.RespondJSON(w, tt.mockStatus, tt.mockBody)
			}))

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			count, groupPath, err := handler.call(ctx, client, tt.groupPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("handler error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
			if groupPath != tt.groupPath {
				t.Errorf("GroupPath = %q, want %q", groupPath, tt.groupPath)
			}
		})
	}
}

func countBody(key string, count int64) string {
	return fmt.Sprintf(`{"%s":%d}`, key, count)
}

func callIssuesCount(ctx context.Context, client *gitlabclient.Client, groupPath string) (int64, string, error) {
	out, err := GetIssuesCount(ctx, client, IssuesCountInput{GroupPath: groupPath})
	return out.IssuesCount, out.GroupPath, err
}

func callMRCount(ctx context.Context, client *gitlabclient.Client, groupPath string) (int64, string, error) {
	out, err := GetMRCount(ctx, client, MRCountInput{GroupPath: groupPath})
	return out.MergeRequestsCount, out.GroupPath, err
}

func callMembersCount(ctx context.Context, client *gitlabclient.Client, groupPath string) (int64, string, error) {
	out, err := GetMembersCount(ctx, client, MembersCountInput{GroupPath: groupPath})
	return out.NewMembersCount, out.GroupPath, err
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
