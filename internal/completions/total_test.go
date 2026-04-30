// total_test.go validates that server-side-filtered search functions
// propagate the X-Total pagination header into the completion result's
// `total` field. This guards the contract documented in toResultWithTotal:
// when GitLab reports a total greater than what we fetched, callers should
// surface that count and infer HasMore=true.
package completions

import (
	"context"
	"net/http"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// TestSearch_PropagatesXTotalHeader exercises every server-side-filtered
// search helper and confirms that the X-Total header is captured into the
// returned total. Mocked endpoints all return 1 row but advertise a much
// larger total so the assertion is unambiguous.
func TestSearch_PropagatesXTotalHeader(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		body    string
		invoke  func(client *testClientType) (int, error)
		wantTot int
	}{
		{
			name:    "projects",
			path:    "/api/v4/projects",
			body:    `[{"id":1,"path_with_namespace":"a/b"}]`,
			wantTot: 250,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchProjects(context.Background(), c, "x")
				return total, err
			},
		},
		{
			name:    "groups",
			path:    "/api/v4/groups",
			body:    `[{"id":1,"full_path":"a"}]`,
			wantTot: 99,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchGroups(context.Background(), c, "x")
				return total, err
			},
		},
		{
			name:    "users",
			path:    "/api/v4/users",
			body:    `[{"id":1,"username":"alice"}]`,
			wantTot: 47,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchUsers(context.Background(), c, "ali")
				return total, err
			},
		},
		{
			name:    "branches",
			path:    "/api/v4/projects/42/repository/branches",
			body:    `[{"name":"main"}]`,
			wantTot: 12,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchBranches(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "tags",
			path:    "/api/v4/projects/42/repository/tags",
			body:    `[{"name":"v1"}]`,
			wantTot: 8,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchTags(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "labels",
			path:    "/api/v4/projects/42/labels",
			body:    `[{"name":"bug"}]`,
			wantTot: 33,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchLabels(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "milestones",
			path:    "/api/v4/projects/42/milestones",
			body:    `[{"id":1,"title":"v1"}]`,
			wantTot: 5,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchMilestones(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "milestone titles",
			path:    "/api/v4/projects/42/milestones",
			body:    `[{"id":1,"title":"v1"}]`,
			wantTot: 5,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchMilestoneTitles(context.Background(), c, "42", "")
				return total, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("X-Total", itoa(tc.wantTot))
				respondJSON(w, http.StatusOK, tc.body)
			})
			client := newTestClient(t, handler)
			got, err := tc.invoke(client)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if got != tc.wantTot {
				t.Errorf("total = %d, want %d (X-Total propagation broken)", got, tc.wantTot)
			}
		})
	}
}

// TestTotalFromResponse_NilSafe documents that totalFromResponse returns 0
// for a nil *gitlab.Response (defensive: the helper is called from search
// fast-paths that may not always have a response object).
func TestTotalFromResponse_NilSafe(t *testing.T) {
	if got := totalFromResponse(nil); got != 0 {
		t.Errorf("totalFromResponse(nil) = %d, want 0", got)
	}
}

// itoa is an inline minimal int→ascii helper to avoid pulling strconv into
// this test file's already-broad assertion surface.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// testClientType is a type alias used by the tests above so the table can
// declare invoke closures without naming the gitlabclient package directly.
type testClientType = gitlabclient.Client
