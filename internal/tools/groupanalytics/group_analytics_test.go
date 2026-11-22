// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// group_analytics_test.go contains unit tests for GitLab group analytics
// operations. Tests use httptest to mock the GitLab Group Analytics API.
package groupanalytics

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// TestCountHandlers_ACountTooLargeToCalculate_SaysSo verifies the too_large
// key 19.6 sends beside a null count is published by each of the three
// handlers, and is absent from an answer that counted, so a 0 the SDK reads
// off the null is never the whole answer.
func TestCountHandlers_ACountTooLargeToCalculate_SaysSo(t *testing.T) {
	calls := map[string]func(context.Context, *gitlabclient.Client) (*bool, error){
		"issues_count": func(ctx context.Context, c *gitlabclient.Client) (*bool, error) {
			out, err := GetIssuesCount(ctx, c, IssuesCountInput{GroupPath: "g"})
			return out.TooLarge, err
		},
		"merge_requests_count": func(ctx context.Context, c *gitlabclient.Client) (*bool, error) {
			out, err := GetMRCount(ctx, c, MRCountInput{GroupPath: "g"})
			return out.TooLarge, err
		},
		"new_members_count": func(ctx context.Context, c *gitlabclient.Client) (*bool, error) {
			out, err := GetMembersCount(ctx, c, MembersCountInput{GroupPath: "g"})
			return out.TooLarge, err
		},
	}
	for key, call := range calls {
		for _, tc := range []struct {
			name string
			body string
			want bool
		}{
			{"too large", `{"` + key + `":null,"too_large":true}`, true},
			{"counted", `{"` + key + `":3}`, false},
		} {
			t.Run(key+"/"+tc.name, func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusOK, tc.body)
				}))
				got, err := call(t.Context(), client)
				if err != nil {
					t.Fatalf("handler error = %v", err)
				}
				if (got != nil && *got) != tc.want || (got == nil) == tc.want {
					t.Errorf("too_large = %v, want present and true: %v", got, tc.want)
				}
			})
		}
	}
}

// TestCountHandlers_ATooLargeTheTypeCannotHold_IsReported verifies each count
// handler reports a too_large in a shape the extra cannot hold.
func TestCountHandlers_ATooLargeTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"too_large":"yes"}`)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "issues", Call: func() error {
			_, err := GetIssuesCount(t.Context(), client, IssuesCountInput{GroupPath: "g"})
			return err
		}},
		{Name: "merge requests", Call: func() error {
			_, err := GetMRCount(t.Context(), client, MRCountInput{GroupPath: "g"})
			return err
		}},
		{Name: "members", Call: func() error {
			_, err := GetMembersCount(t.Context(), client, MembersCountInput{GroupPath: "g"})
			return err
		}},
	})
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

// The guidance section each of the three analytics cards closes with.
const (
	issuesCountHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.analytics_mr_count' to compare with merge request activity\n" +
		"- Use action 'issue.list_group' to read the issues themselves\n"
	mrCountHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.analytics_issues_count' to compare with issue activity\n" +
		"- Use action 'merge_request.list_group' to read the merge requests themselves\n"
	membersCountHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.members' to read the members themselves\n" +
		"- Use action 'group.analytics_issues_count' to see the group's development activity\n"
)

// assertAnalyticsCard compares a whole rendered card with what the formatter
// is meant to write, byte for byte.
func assertAnalyticsCard(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("analytics card:\n got %q\nwant %q", got, want)
	}
}

// TestFormatIssuesCountMarkdown verifies the whole card the issue count
// renders, a zero count included: zero is the answer GitLab gave, not an
// absence, so the row is written.
func TestFormatIssuesCountMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input IssuesCountOutput
		want  string
	}{
		{
			name:  "formats non-zero count",
			input: IssuesCountOutput{GroupPath: "my-group", IssuesCount: 42},
			want: "## Recently Created Issues Count\n\n" +
				"- **Group**: `my-group`\n" +
				"- **Issues Count (last 90 days)**: 42\n" +
				issuesCountHints,
		},
		{
			name:  "formats zero count",
			input: IssuesCountOutput{GroupPath: "empty-group"},
			want: "## Recently Created Issues Count\n\n" +
				"- **Group**: `empty-group`\n" +
				"- **Issues Count (last 90 days)**: 0\n" +
				issuesCountHints,
		},
		{
			name:  "formats nested group path",
			input: IssuesCountOutput{GroupPath: "parent/child", IssuesCount: 100},
			want: "## Recently Created Issues Count\n\n" +
				"- **Group**: `parent/child`\n" +
				"- **Issues Count (last 90 days)**: 100\n" +
				issuesCountHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertAnalyticsCard(t, FormatIssuesCountMarkdown(tt.input), tt.want)
		})
	}
}

// TestFormatCountMarkdown_ACountTooLargeToCalculate_SaysItIsUnknown verifies
// each of the three cards says GitLab could not count when the answer carries
// too_large, in place of the 0 the SDK reads off the null count, so a reader
// of the text is not told the group had no activity; a too_large GitLab sent
// as false leaves the count as it was.
func TestFormatCountMarkdown_ACountTooLargeToCalculate_SaysItIsUnknown(t *testing.T) {
	yes, no := true, false
	const unknown = "unknown, GitLab could not count it within its time limit"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "issues too large",
			got:  FormatIssuesCountMarkdown(IssuesCountOutput{GroupPath: "big", TooLarge: &yes}),
			want: "## Recently Created Issues Count\n\n" +
				"- **Group**: `big`\n" +
				"- **Issues Count (last 90 days)**: " + unknown + "\n" +
				issuesCountHints,
		},
		{
			name: "merge requests too large",
			got:  FormatMRCountMarkdown(MRCountOutput{GroupPath: "big", TooLarge: &yes}),
			want: "## Recently Created Merge Requests Count\n\n" +
				"- **Group**: `big`\n" +
				"- **Merge Requests Count (last 90 days)**: " + unknown + "\n" +
				mrCountHints,
		},
		{
			name: "members too large",
			got:  FormatMembersCountMarkdown(MembersCountOutput{GroupPath: "big", TooLarge: &yes}),
			want: "## Recently Added Members Count\n\n" +
				"- **Group**: `big`\n" +
				"- **New Members Count (last 90 days)**: " + unknown + "\n" +
				membersCountHints,
		},
		{
			name: "too_large sent as false",
			got:  FormatIssuesCountMarkdown(IssuesCountOutput{GroupPath: "g", IssuesCount: 4, TooLarge: &no}),
			want: "## Recently Created Issues Count\n\n" +
				"- **Group**: `g`\n" +
				"- **Issues Count (last 90 days)**: 4\n" +
				issuesCountHints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertAnalyticsCard(t, tt.got, tt.want)
		})
	}
}

// TestFormatMRCountMarkdown verifies the whole card the merge request count
// renders.
func TestFormatMRCountMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input MRCountOutput
		want  string
	}{
		{
			name:  "formats non-zero MR count",
			input: MRCountOutput{GroupPath: "dev-team", MergeRequestsCount: 17},
			want: "## Recently Created Merge Requests Count\n\n" +
				"- **Group**: `dev-team`\n" +
				"- **Merge Requests Count (last 90 days)**: 17\n" +
				mrCountHints,
		},
		{
			name:  "formats zero MR count",
			input: MRCountOutput{GroupPath: "quiet-team"},
			want: "## Recently Created Merge Requests Count\n\n" +
				"- **Group**: `quiet-team`\n" +
				"- **Merge Requests Count (last 90 days)**: 0\n" +
				mrCountHints,
		},
		{
			name:  "formats large MR count",
			input: MRCountOutput{GroupPath: "mega-corp/platform", MergeRequestsCount: 99999},
			want: "## Recently Created Merge Requests Count\n\n" +
				"- **Group**: `mega-corp/platform`\n" +
				"- **Merge Requests Count (last 90 days)**: 99999\n" +
				mrCountHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertAnalyticsCard(t, FormatMRCountMarkdown(tt.input), tt.want)
		})
	}
}

// TestFormatMembersCountMarkdown verifies the whole card the member count
// renders.
func TestFormatMembersCountMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input MembersCountOutput
		want  string
	}{
		{
			name:  "formats non-zero members count",
			input: MembersCountOutput{GroupPath: "my-org", NewMembersCount: 5},
			want: "## Recently Added Members Count\n\n" +
				"- **Group**: `my-org`\n" +
				"- **New Members Count (last 90 days)**: 5\n" +
				membersCountHints,
		},
		{
			name:  "formats zero members count",
			input: MembersCountOutput{GroupPath: "stable-org"},
			want: "## Recently Added Members Count\n\n" +
				"- **Group**: `stable-org`\n" +
				"- **New Members Count (last 90 days)**: 0\n" +
				membersCountHints,
		},
		{
			name:  "formats deeply nested group path",
			input: MembersCountOutput{GroupPath: "a/b/c/d", NewMembersCount: 1},
			want: "## Recently Added Members Count\n\n" +
				"- **Group**: `a/b/c/d`\n" +
				"- **New Members Count (last 90 days)**: 1\n" +
				membersCountHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertAnalyticsCard(t, FormatMembersCountMarkdown(tt.input), tt.want)
		})
	}
}
