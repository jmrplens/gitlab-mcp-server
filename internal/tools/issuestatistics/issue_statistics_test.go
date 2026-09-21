// issue_statistics_test.go contains unit tests for the issue statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuestatistics

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// newStats builds a [StatisticsOutput] with the nested statistics/counts
// structure populated from the given all/opened/closed values.
func newStats(all, opened, closed int64) StatisticsOutput {
	return StatisticsOutput{
		Statistics: StatisticsCountsOutput{
			Counts: CountsOutput{All: all, Opened: opened, Closed: closed},
		},
	}
}

// statsRequest returns a handler that checks the request reached path carrying
// exactly want as its query, then answers with body.
//
// Equality over the whole query, rather than one lookup per parameter, is what
// tells two filters apart. The assertions this replaced asked only whether a
// parameter was non-empty, so an option builder that read created_before into
// created_after, or milestone into search, produced a query every one of them
// accepted; neither gate can see such a crossing, because a straight-line
// assignment carries no branch to flip. It also fails on a filter sent that
// the caller never set, which a per-parameter lookup never looks for.
//
// It runs on the httptest server's own goroutine, so it reports with t.Errorf
// and answers regardless, leaving the calling test's assertions to report too.
func statsRequest(t *testing.T, path string, want url.Values, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, path)
		if got := r.URL.Query(); !reflect.DeepEqual(got, want) {
			t.Errorf("query = %v, want %v", got, want)
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}
}

// TestGet verifies Get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, pathGlobalStats)
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":10,"closed":3,"opened":7}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 10 || out.Statistics.Counts.Opened != 7 || out.Statistics.Counts.Closed != 3 {
		t.Errorf("unexpected counts: %+v", out)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetGroup verifies GetGroup.
func TestGetGroup(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/5/issues_statistics")
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":5,"closed":2,"opened":3}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 5 {
		t.Errorf("All = %d", out.Statistics.Counts.All)
	}
}

// TestGetGroup_Error verifies GetGroup when error.
func TestGetGroup_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetProject verifies GetProject.
func TestGetProject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/1/issues_statistics")
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":20,"closed":10,"opened":10}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.Opened != 10 {
		t.Errorf("Opened = %d", out.Statistics.Counts.Opened)
	}
}

// TestGetProject_Error verifies GetProject when error.
func TestGetProject_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatMarkdown verifies FormatMarkdown renders the whole card: the
// scope-free heading, one row per count, and the guidance section naming the
// canonical action that lists the issues behind the counts.
func TestFormatMarkdown(t *testing.T) {
	want := "## Issue Statistics\n\n" +
		"- **All**: 10\n" +
		"- **Opened**: 7\n" +
		"- **Closed**: 3\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.list' to see the individual issues behind these counts\n"
	if got := FormatMarkdown(newStats(10, 7, 3)); got != want {
		t.Errorf("FormatMarkdown()\n got %q\nwant %q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// commonStatsJSON identifies the common stats JSON constant used by this package.
const commonStatsJSON = `{"statistics":{"counts":{"all":5,"closed":2,"opened":3}}}`

const (
	// errExpAPIErrResponse identifies the err exp API err response constant used by this package.
	errExpAPIErrResponse = "expected error for API error response"
	// pathGlobalStats is the instance-wide issue statistics endpoint.
	pathGlobalStats = "/api/v4/issues_statistics"
	// pathGroupStats is the issue statistics endpoint of the group these tests filter against.
	pathGroupStats = "/api/v4/groups/99/issues_statistics"
	// pathProjectStats is the issue statistics endpoint of the project these tests filter against.
	pathProjectStats = "/api/v4/projects/42/issues_statistics"
)

// ---------------------------------------------------------------------------
// FormatMarkdown
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_Populated checks the whole rendering of a populated
// statistics object, rows included: a substring check over a card is what let
// a table row survive a migration to list items and still pass.
func TestFormatMarkdown_Populated(t *testing.T) {
	want := "## Issue Statistics\n\n" +
		"- **All**: 100\n" +
		"- **Opened**: 60\n" +
		"- **Closed**: 40\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.list' to see the individual issues behind these counts\n"
	if got := FormatMarkdown(newStats(100, 60, 40)); got != want {
		t.Errorf("FormatMarkdown(populated)\n got %q\nwant %q", got, want)
	}
}

// TestFormatMarkdown_Empty checks that a statistics object of zeros renders
// every row: a zero count is an answer GitLab gave, not an absent value, so
// the rows stay and read "0".
func TestFormatMarkdown_Empty(t *testing.T) {
	want := "## Issue Statistics\n\n" +
		"- **All**: 0\n" +
		"- **Opened**: 0\n" +
		"- **Closed**: 0\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.list' to see the individual issues behind these counts\n"
	if got := FormatMarkdown(StatisticsOutput{}); got != want {
		t.Errorf("FormatMarkdown(zero)\n got %q\nwant %q", got, want)
	}
}

// TestFormatMarkdown_HeadingNamesNoScope checks that the heading claims no
// scope. One output type answers the instance, group and project routes, so
// the registry has one key for all three and a heading naming a scope would
// name the wrong one two times out of three.
func TestFormatMarkdown_HeadingNamesNoScope(t *testing.T) {
	md := FormatMarkdown(newStats(1, 1, 0))
	if first, _, _ := strings.Cut(md, "\n"); first != "## Issue Statistics" {
		t.Errorf("heading = %q, want %q", first, "## Issue Statistics")
	}
	for _, scope := range []string{"Global", "Group", "Project", "All Issue Statistics"} {
		t.Run(scope, func(t *testing.T) {
			if strings.Contains(md, scope) {
				t.Errorf("rendering names the scope %q:\n%s", scope, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// fromGL converter (tested indirectly via handlers)
// ---------------------------------------------------------------------------.

// TestFromGL_FullData verifies FromGL when full data.
func TestFromGL_FullData(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":250,"closed":100,"opened":150}}}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, resp)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 250 {
		t.Errorf("All = %d, want 250", out.Statistics.Counts.All)
	}
	if out.Statistics.Counts.Closed != 100 {
		t.Errorf("Closed = %d, want 100", out.Statistics.Counts.Closed)
	}
	if out.Statistics.Counts.Opened != 150 {
		t.Errorf("Opened = %d, want 150", out.Statistics.Counts.Opened)
	}
}

// TestFromGL_ZeroCounts verifies FromGL when zero counts.
func TestFromGL_ZeroCounts(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, resp)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 0 || out.Statistics.Counts.Closed != 0 || out.Statistics.Counts.Opened != 0 {
		t.Errorf("expected all zeros, got %+v", out)
	}
}

// ---------------------------------------------------------------------------
// Get (global) -- filter branches
// ---------------------------------------------------------------------------.

// TestGet_WithAllFilters checks that the four core filters reach GitLab as the
// caller spelled them, and as the only query the request carries.
func TestGet_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":10,"closed":3,"opened":7}}}`
	client := testutil.NewTestClient(t, statsRequest(t, pathGlobalStats, url.Values{
		"labels":    {"bug,critical"},
		"milestone": {"v1.0"},
		"scope":     {"all"},
		"search":    {"memory leak"},
	}, resp))

	out, err := Get(t.Context(), client, GetInput{
		Labels:    []string{"bug", "critical"},
		Milestone: "v1.0",
		Scope:     "all",
		Search:    "memory leak",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 10 {
		t.Errorf("All = %d, want 10", out.Statistics.Counts.All)
	}
}

// TestGet_WithLabelsOnly checks that a labels-only call sends the labels
// filter and nothing beside it, so a builder reading another field into
// Labels leaves the query empty rather than merely differently populated.
func TestGet_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGlobalStats, url.Values{"labels": {"bug"}}, commonStatsJSON))
	_, err := Get(t.Context(), client, GetInput{Labels: []string{"bug"}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_WithMilestoneOnly checks that a milestone-only call sends the
// milestone filter and nothing beside it.
func TestGet_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGlobalStats, url.Values{"milestone": {"v2.0"}}, commonStatsJSON))
	_, err := Get(t.Context(), client, GetInput{Milestone: "v2.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_WithScopeOnly checks that a scope-only call sends the scope filter
// and nothing beside it.
func TestGet_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGlobalStats, url.Values{"scope": {"created_by_me"}}, commonStatsJSON))
	_, err := Get(t.Context(), client, GetInput{Scope: "created_by_me"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_WithSearchOnly checks that a search-only call sends the search
// filter and nothing beside it.
func TestGet_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGlobalStats, url.Values{"search": {"timeout"}}, commonStatsJSON))
	_, err := Get(t.Context(), client, GetInput{Search: "timeout"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_ContextCancelled verifies Get when context cancelled.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGet_APIError500 verifies Get when API error 500.
func TestGet_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"boom"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGet_APIError403 verifies Get when API error 403.
func TestGet_APIError403(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

// ---------------------------------------------------------------------------
// GetGroup -- filter branches
// ---------------------------------------------------------------------------.

// TestGetGroup_WithAllFilters checks that the four core filters reach GitLab
// as the caller spelled them, and as the only query the request carries.
func TestGetGroup_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":30,"closed":10,"opened":20}}}`
	client := testutil.NewTestClient(t, statsRequest(t, pathGroupStats, url.Values{
		"labels":    {"feature,enhancement"},
		"milestone": {"sprint-3"},
		"scope":     {"assigned_to_me"},
		"search":    {"refactor"},
	}, resp))

	out, err := GetGroup(t.Context(), client, GetGroupInput{
		GroupID:   "99",
		Labels:    []string{"feature", "enhancement"},
		Milestone: "sprint-3",
		Scope:     "assigned_to_me",
		Search:    "refactor",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 30 {
		t.Errorf("All = %d, want 30", out.Statistics.Counts.All)
	}
	if out.Statistics.Counts.Opened != 20 {
		t.Errorf("Opened = %d, want 20", out.Statistics.Counts.Opened)
	}
}

// TestGetGroup_WithLabelsOnly checks that a labels-only call sends the labels
// filter and nothing beside it. This and its three siblings below looked at no
// request at all until this sweep: each named a filter and then asserted only
// that GitLab had not refused, so dropping the filter from the option builder
// failed none of them.
func TestGetGroup_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGroupStats, url.Values{"labels": {"bug"}}, commonStatsJSON))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Labels: []string{"bug"}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithMilestoneOnly checks that a milestone-only call sends the
// milestone filter and nothing beside it.
func TestGetGroup_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGroupStats, url.Values{"milestone": {"v1.0"}}, commonStatsJSON))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Milestone: "v1.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithScopeOnly checks that a scope-only call sends the scope
// filter and nothing beside it.
func TestGetGroup_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGroupStats, url.Values{"scope": {"all"}}, commonStatsJSON))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Scope: "all"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithSearchOnly checks that a search-only call sends the search
// filter and nothing beside it.
func TestGetGroup_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGroupStats, url.Values{"search": {"deploy"}}, commonStatsJSON))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Search: "deploy"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_ContextCancelled verifies GetGroup when context cancelled.
func TestGetGroup_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGroup(ctx, client, GetGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetGroup_APIError500 verifies GetGroup when API error 500.
func TestGetGroup_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"boom"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGetGroup_APIError404 verifies GetGroup when API error 404.
func TestGetGroup_APIError404(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"group not found"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// ---------------------------------------------------------------------------
// GetProject -- filter branches
// ---------------------------------------------------------------------------.

// TestGetProject_WithAllFilters checks that the four core filters reach GitLab
// as the caller spelled them, and as the only query the request carries.
func TestGetProject_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":50,"closed":20,"opened":30}}}`
	client := testutil.NewTestClient(t, statsRequest(t, pathProjectStats, url.Values{
		"labels":    {"bug,security"},
		"milestone": {"release-1"},
		"scope":     {"created_by_me"},
		"search":    {"crash"},
	}, resp))

	out, err := GetProject(t.Context(), client, GetProjectInput{
		ProjectID: "42",
		Labels:    []string{"bug", "security"},
		Milestone: "release-1",
		Scope:     "created_by_me",
		Search:    "crash",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 50 {
		t.Errorf("All = %d, want 50", out.Statistics.Counts.All)
	}
	if out.Statistics.Counts.Closed != 20 {
		t.Errorf("Closed = %d, want 20", out.Statistics.Counts.Closed)
	}
	if out.Statistics.Counts.Opened != 30 {
		t.Errorf("Opened = %d, want 30", out.Statistics.Counts.Opened)
	}
}

// TestGetProject_WithLabelsOnly checks that a labels-only call sends the
// labels filter and nothing beside it.
func TestGetProject_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathProjectStats, url.Values{"labels": {"bug"}}, commonStatsJSON))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Labels: []string{"bug"}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithMilestoneOnly checks that a milestone-only call sends the
// milestone filter and nothing beside it.
func TestGetProject_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathProjectStats, url.Values{"milestone": {"v3.0"}}, commonStatsJSON))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Milestone: "v3.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithScopeOnly checks that a scope-only call sends the scope
// filter and nothing beside it.
func TestGetProject_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathProjectStats, url.Values{"scope": {"all"}}, commonStatsJSON))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Scope: "all"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithSearchOnly checks that a search-only call sends the
// search filter and nothing beside it.
func TestGetProject_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathProjectStats, url.Values{"search": {"nil pointer"}}, commonStatsJSON))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Search: "nil pointer"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_ContextCancelled verifies GetProject when context cancelled.
func TestGetProject_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetProject(ctx, client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetProject_APIError500 verifies GetProject when API error 500.
func TestGetProject_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"boom"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGetProject_APIError401 verifies GetProject when API error 401.
func TestGetProject_APIError401(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"unauthorized"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

// ---------------------------------------------------------------------------
// The refusal each route hands a model
// ---------------------------------------------------------------------------.

// hintedRefusal describes the refusal one route was given: the operation it
// reports under, the status its hint is keyed on, a second status that must
// therefore go unhinted, the whole sentence the hint has to be, and what the
// other routes send a model to, neither subject nor verifying action of which
// may appear anywhere in this route's refusal.
type hintedRefusal struct {
	operation   string
	hintStatus  int
	otherStatus int
	hint        string
	notNames    []string
	call        func(ctx context.Context, client *gitlabclient.Client) error
}

// hintedRefusals pairs each route with the refusal it was written to give.
//
// The second status of each is another route's keyed status rather than an
// arbitrary one, since crossing the two constants is exactly the mistake a
// list of three near-identical handlers invites.
//
// The hint is held as the whole sentence rather than as the subject it names,
// because the subject is not the only half of it that can cross: "verify
// group_id with project.get" still names the group and still names no foreign
// subject, so a subject-only assertion passed it while the capability a model
// was sent to is the one that cannot answer for a group. Holding the sentence
// costs a rewording being repeated here, which is the price of the two action
// IDs being read by anything at all.
func hintedRefusals() []hintedRefusal {
	return []hintedRefusal{
		{
			operation: "gitlab_get_issue_statistics", hintStatus: http.StatusForbidden,
			otherStatus: http.StatusNotFound, hint: "verify your token has read_api scope",
			notNames: []string{"group_id", "project_id", "group.get", "project.get"},
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Get(ctx, client, GetInput{})
				return err
			},
		},
		{
			operation: "gitlab_get_group_issue_statistics", hintStatus: http.StatusNotFound,
			otherStatus: http.StatusForbidden, hint: "verify group_id with group.get",
			notNames: []string{"project_id", "read_api", "project.get"},
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GetGroup(ctx, client, GetGroupInput{GroupID: "99"})
				return err
			},
		},
		{
			operation: "gitlab_get_project_issue_statistics", hintStatus: http.StatusNotFound,
			otherStatus: http.StatusForbidden, hint: "verify project_id with project.get",
			notNames: []string{"group_id", "read_api", "group.get"},
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GetProject(ctx, client, GetProjectInput{ProjectID: "42"})
				return err
			},
		},
	}
}

// refusalFrom drives one route against an instance that answers code, and
// returns the error text the caller is handed.
func refusalFrom(t *testing.T, route hintedRefusal, code int) string {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, code, `{"message":"refused"}`)
	}))
	err := route.call(t.Context(), client)
	if err == nil {
		t.Fatalf("%s: GitLab answered %d and the handler returned no error", route.operation, code)
	}
	return err.Error()
}

// TestHandlers_RefusalNamesItsOwnOperationAndSubject checks that each route
// reports a refusal under its own operation and, on the status its hint is
// keyed on, tells a model to check that route's own subject and no other's.
//
// Why that matters: every error assertion this package had stopped at "an
// error came back", so the operation label, the status the hint is keyed on
// and the sentence itself could each be crossed with another route's and
// nothing would fail. None of the three is a branch, so neither gate can see
// it either. The hint is what a model does next, so a project route answering
// "verify group_id" sends it to look up an object that was never the subject.
//
// The hint is compared as the whole sentence, terminator included, so that
// every token of it is held: the subject it tells a model to check and the
// action ID it sends the model to check it with. Either alone leaves the other
// free to be the sibling route's.
func TestHandlers_RefusalNamesItsOwnOperationAndSubject(t *testing.T) {
	for _, route := range hintedRefusals() {
		t.Run(route.operation, func(t *testing.T) {
			hinted := refusalFrom(t, route, route.hintStatus)
			if !strings.HasPrefix(hinted, route.operation+": ") {
				t.Errorf("refusal = %q, want it reported under %q", hinted, route.operation)
			}
			// The trailing colon is what ends the hint in the composed
			// message, so including it holds the sentence to its whole text
			// rather than to a prefix of it.
			wantHint := "Suggestion: " + route.hint + ":"
			if !strings.Contains(hinted, wantHint) {
				t.Errorf("refusal to a %d = %q, want it to carry %q", route.hintStatus, hinted, wantHint)
			}
			for _, foreign := range route.notNames {
				if strings.Contains(hinted, foreign) {
					t.Errorf("refusal = %q, which sends a model to %s, another route's subject", hinted, foreign)
				}
			}
		})
	}
}

// TestHandlers_RefusalOnAnotherStatusCarriesNoHint checks that each route's
// hint reaches a model only on the status it was keyed on.
//
// This is the other half of the pairing above, and the half that pins the
// status constant: a hint written for a 403 says something true only of a 403,
// and a handler that offered it on every refusal would be advising a token
// scope to a model that was told the group does not exist.
func TestHandlers_RefusalOnAnotherStatusCarriesNoHint(t *testing.T) {
	for _, route := range hintedRefusals() {
		t.Run(route.operation, func(t *testing.T) {
			unhinted := refusalFrom(t, route, route.otherStatus)
			if !strings.HasPrefix(unhinted, route.operation+": ") {
				t.Errorf("refusal = %q, want it reported under %q", unhinted, route.operation)
			}
			if strings.Contains(unhinted, "Suggestion: ") {
				t.Errorf("refusal to a %d = %q, carrying a hint keyed on %d", route.otherStatus, unhinted, route.hintStatus)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// covStatsJSON identifies the cov stats JSON constant used by this package.
const covStatsJSON = `{"statistics":{"counts":{"all":100,"closed":40,"opened":60}}}`

// newIssueStatsRouteSpecs constructs issue stats route specs test fixtures.
func newIssueStatsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathGlobalStats:
			testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
		case r.Method == http.MethodGet && path == pathGroupStats:
			testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
		case r.Method == http.MethodGet && path == pathProjectStats:
			testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
		default:
			http.NotFound(w, r)
		}
	}))

	return issueStatsSpecsByTool(ActionSpecs(client))
}

// issueStatsSpecsByTool indexes action specs by individual tool name for route assertions.
func issueStatsSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}

// assertRouteCallSuccess checks route call success invariants for tests.
func assertRouteCallSuccess(t *testing.T, specByTool map[string]toolutil.ActionSpec, name string, args map[string]any) {
	t.Helper()
	spec, ok := specByTool[name]
	if !ok {
		t.Fatalf("missing ActionSpec for %s", name)
	}
	result, err := spec.Route.Handler(t.Context(), args)
	if err != nil {
		t.Fatalf("Route.Handler(%s) error: %v", name, err)
	}
	if result == nil {
		t.Fatalf("Route.Handler(%s) returned nil", name)
	}
}

// TestActionSpecs_CallRoutes validates issue statistics canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newIssueStatsRouteSpecs(t)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_issue_statistics", map[string]any{
			"labels": []string{}, "milestone": "", "scope": "", "search": "",
		}},
		{"gitlab_get_group_issue_statistics", map[string]any{
			"group_id": "99", "labels": []string{}, "milestone": "", "scope": "", "search": "",
		}},
		{"gitlab_get_project_issue_statistics", map[string]any{
			"project_id": "42", "labels": []string{}, "milestone": "", "scope": "", "search": "",
		}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertRouteCallSuccess(t, specByTool, tt.name, tt.args)
		})
	}
}

// TestActionSpecs_Metadata verifies issue statistics action spec metadata.
//
// The client answers with [testutil.ForbiddenHandler], which fails the test if
// any request arrives: this reads metadata and calls no route, so a request
// would mean the assertions below had reached GitLab to make them.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "issuestatistics" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route error paths
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteErrors validates issue statistics route error paths.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	specByTool := issueStatsSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_issue_statistics", map[string]any{}},
		{"gitlab_get_group_issue_statistics", map[string]any{"group_id": "42"}},
		{"gitlab_get_project_issue_statistics", map[string]any{"project_id": "42"}},
	}
	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := specByTool[tc.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.name)
			}
			if _, err := spec.Route.Handler(t.Context(), tc.args); err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestMarkdownInit validates the init-registered markdown formatter is callable
// via the toolutil registry.
func TestMarkdownInit(t *testing.T) {
	out := newStats(10, 7, 3)
	res := toolutil.MarkdownForResult(out)
	if res == nil {
		t.Fatal("expected non-nil result from registered formatter")
	}
}

// ---------------------------------------------------------------------------
// Extended 1:1 filter coverage (assignee, author, confidential, dates, IIDs)
// ---------------------------------------------------------------------------.

// The four timestamps the extended-filter calls send. They differ in the month
// as well as in the day and the time of day, because two fields carrying one
// value are indistinguishable however the comparison is written: an option
// builder that read CreatedBefore into CreatedAfter would produce the same
// query as a faithful one if any pair agreed.
const (
	createdAfterFilter  = "2025-01-01T00:00:00Z"
	createdBeforeFilter = "2025-12-31T23:59:59Z"
	updatedAfterFilter  = "2025-02-01T00:00:00Z"
	updatedBeforeFilter = "2025-11-30T23:59:59Z"
)

// extendedStatsQuery returns the query the extended-filter calls below must
// produce, which the global route extends by the "in" parameter only it
// offers. Every value is distinct from every other for the reason the
// timestamps above are.
//
// It is built fresh per call because [url.Values] is a map and a shared one
// would carry one test's addition into the next.
func extendedStatsQuery() url.Values {
	return url.Values{
		"assignee_id":       {"7"},
		"assignee_username": {"alice"},
		"author_id":         {"9"},
		"author_username":   {"octocat"},
		"confidential":      {"true"},
		"created_after":     {createdAfterFilter},
		"created_before":    {createdBeforeFilter},
		"updated_after":     {updatedAfterFilter},
		"updated_before":    {updatedBeforeFilter},
		"iids[]":            {"1", "2"},
		"my_reaction_emoji": {"thumbsup"},
	}
}

// TestGet_ExtendedFilters checks that every extended filter the global route
// offers reaches GitLab under its own name and with the caller's own value.
//
// The whole query is compared rather than each parameter looked up, because
// what the assertion this replaced could not see is a crossing: it asked only
// whether created_after, created_before, updated_after and updated_before were
// non-empty, so the four could be permuted freely and every one of them still
// passed while the caller's date window silently became another window.
func TestGet_ExtendedFilters(t *testing.T) {
	want := extendedStatsQuery()
	want.Set("in", "title")
	client := testutil.NewTestClient(t, statsRequest(t, pathGlobalStats, want, commonStatsJSON))
	_, err := Get(t.Context(), client, GetInput{
		AssigneeID:       new(int64(7)),
		AssigneeUsername: []string{"alice"},
		AuthorID:         new(int64(9)),
		AuthorUsername:   "octocat",
		Confidential:     new(true),
		CreatedAfter:     createdAfterFilter,
		CreatedBefore:    createdBeforeFilter,
		UpdatedAfter:     updatedAfterFilter,
		UpdatedBefore:    updatedBeforeFilter,
		IIDs:             []int64{1, 2},
		In:               "title",
		MyReactionEmoji:  "thumbsup",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_ExtendedFilters checks that every extended filter the group
// route offers reaches GitLab under its own name and with the caller's own
// value, and that the group route sends no "in" parameter, which GitLab does
// not offer there.
func TestGetGroup_ExtendedFilters(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathGroupStats, extendedStatsQuery(), commonStatsJSON))
	_, err := GetGroup(t.Context(), client, GetGroupInput{
		GroupID:          "99",
		AssigneeID:       new(int64(7)),
		AssigneeUsername: []string{"alice"},
		AuthorID:         new(int64(9)),
		AuthorUsername:   "octocat",
		Confidential:     new(true),
		CreatedAfter:     createdAfterFilter,
		CreatedBefore:    createdBeforeFilter,
		UpdatedAfter:     updatedAfterFilter,
		UpdatedBefore:    updatedBeforeFilter,
		IIDs:             []int64{1, 2},
		MyReactionEmoji:  "thumbsup",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_ExtendedFilters checks that every extended filter the project
// route offers reaches GitLab under its own name and with the caller's own
// value, and that the project route sends no "in" parameter either.
func TestGetProject_ExtendedFilters(t *testing.T) {
	client := testutil.NewTestClient(t, statsRequest(t, pathProjectStats, extendedStatsQuery(), commonStatsJSON))
	_, err := GetProject(t.Context(), client, GetProjectInput{
		ProjectID:        "42",
		AssigneeID:       new(int64(7)),
		AssigneeUsername: []string{"alice"},
		AuthorID:         new(int64(9)),
		AuthorUsername:   "octocat",
		Confidential:     new(true),
		CreatedAfter:     createdAfterFilter,
		CreatedBefore:    createdBeforeFilter,
		UpdatedAfter:     updatedAfterFilter,
		UpdatedBefore:    updatedBeforeFilter,
		IIDs:             []int64{1, 2},
		MyReactionEmoji:  "thumbsup",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// Discovery metadata (R-META)
// ---------------------------------------------------------------------------.

// issueStatsSpecs builds the package's action specs against a client that must
// not be asked anything: every assertion below reads metadata rather than
// calling a route, so [testutil.ForbiddenHandler] fails the test on any
// request instead of quietly answering one.
func issueStatsSpecs(t *testing.T) []toolutil.ActionSpec {
	t.Helper()
	return ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
}

// publishedJSONFields returns the json names a tool input struct publishes,
// which is the set an input-schema override may name.
func publishedJSONFields(input any) map[string]bool {
	fields := make(map[string]bool)
	for field := range reflect.TypeOf(input).Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}

// enumFor returns the enum an override publishes for the named input property.
func enumFor(overrides []toolutil.InputSchemaOverride, property string) ([]any, bool) {
	for _, override := range overrides {
		if override.PropertyPath != property {
			continue
		}
		values, ok := override.Values["enum"].([]any)
		return values, ok
	}
	return nil, false
}

// TestActionSpecs_EveryActionCarriesDecoratedMetadata checks that every action
// ActionSpecs registers has been through decorateIssueStatisticsMeta, by
// comparing each against the undecorated options an unrecognized tool name
// returns.
//
// Why that matters: decorateIssueStatisticsMeta is a switch with no default,
// so an action added to ActionSpecs without a case of its own ships silently
// with the generic package usage, no related actions, no individual-tool
// description and none of the enum constraints. That is the whole of what a
// model reads to find the tool and call it correctly, and nothing asserted any
// of it: the decorator could be neutered in full with this suite still green,
// which was verified by hand before this test existed.
//
// The undecorated baseline is computed rather than quoted, so it cannot drift
// from the source. It is asserted to be undecorated first, and that control is
// the load-bearing half: without it, a decorator that stopped decorating would
// move both sides of every comparison together and each assertion below would
// pass while asserting nothing.
func TestActionSpecs_EveryActionCarriesDecoratedMetadata(t *testing.T) {
	baseline := issueStatisticsOptions("gitlab_get_issue_statistics_no_such_tool")

	if len(baseline.RelatedActions) != 0 || len(baseline.InputSchemaOverrides) != 0 || baseline.IndividualTool.Description != "" {
		t.Fatalf("an unrecognized tool name came back decorated, so the comparisons below prove nothing: %+v", baseline)
	}

	for _, spec := range issueStatsSpecs(t) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			if spec.Usage == baseline.Usage {
				t.Errorf("Usage is the undecorated placeholder %q", spec.Usage)
			}
			if len(spec.Aliases) <= len(baseline.Aliases) {
				t.Errorf("Aliases = %v, want the tool name plus natural-language aliases", spec.Aliases)
			}
			if spec.IndividualTool.Description == "" {
				t.Error("IndividualTool.Description is empty, so the individual surface lists this tool with no description")
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("RelatedActions is empty, so nothing routes a model to the other scopes")
			}
			if len(spec.InputSchemaOverrides) == 0 {
				t.Error("InputSchemaOverrides is empty, so no fixed vocabulary reaches the served schema")
			}
		})
	}
}

// TestActionSpecs_RelatedActionsNameTheSiblingScopes checks that each action's
// RelatedActions names the canonical ID of the package's other two actions,
// never its own, and at least one action outside the package.
//
// Why that matters: the three actions differ only in scope, so the routing a
// model needs most from this metadata is the way to the other two. The IDs are
// hand-written in three near-identical switch cases, where a copy-paste leaves
// an action pointing at itself or naming one sibling twice, and a rename
// leaves a dangling reference that resolves to nothing. The expected IDs are
// built from the specs themselves, so this fails on the rename rather than
// preserving whatever the source happens to say today.
//
// The ID is built from [catalogDomain] and not from the spec's OwnerPackage.
// Those are different things: the owner package is where the handler lives,
// while the domain is the catalog group these specs are aggregated into. This
// test used to build it from the owner package, and so demanded of every
// action exactly the dead ID the package published, which is how six
// unresolvable related actions passed a test written to catch them. That the
// domain itself is right is not something this package can check; the external
// test in issue_statistics_catalog_test.go puts every published ID to the
// catalog.
func TestActionSpecs_RelatedActionsNameTheSiblingScopes(t *testing.T) {
	specs := issueStatsSpecs(t)
	canonical := func(spec toolutil.ActionSpec) string { return catalogDomain + "." + spec.Name }

	ours := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		ours[canonical(spec)] = struct{}{}
	}

	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			own := canonical(spec)
			if slices.Contains(spec.RelatedActions, own) {
				t.Errorf("RelatedActions = %v, names the action itself (%s)", spec.RelatedActions, own)
			}
			outside := 0
			for _, related := range spec.RelatedActions {
				// Membership of this package's own three, not a domain
				// prefix: the siblings and the way out of the counts now
				// share the issue domain, so a prefix test would pass on an
				// action that named nothing but its siblings.
				if _, mine := ours[related]; !mine {
					outside++
				}
			}
			if outside == 0 {
				t.Errorf("RelatedActions = %v, names no action outside %s to reach the issues behind the counts", spec.RelatedActions, spec.OwnerPackage)
			}
			for _, sibling := range specs {
				id := canonical(sibling)
				if id != own && !slices.Contains(spec.RelatedActions, id) {
					t.Errorf("RelatedActions = %v, missing sibling scope %s", spec.RelatedActions, id)
				}
			}
		})
	}
}

// TestActionSpecs_EnumOverridesConstrainPublishedFields checks that every
// input-schema override names a field its own action publishes, and that each
// fixed-vocabulary field an action publishes is constrained to GitLab's own
// vocabulary.
//
// Why that matters in both directions: an override whose PropertyPath matches
// no property patches nothing and says nothing about it, so the constraint
// never reaches the served schema and a model stays free to send a value
// GitLab refuses. The three inputs are deliberately not the same shape, since
// only the global route takes "in", so the copy-paste that would carry the
// "in" override onto the group action is exactly what the first half catches
// and the second half is what catches one being dropped. The vocabularies are
// GitLab's documented ones for the issue statistics endpoints rather than a
// copy of the source line, so the assertion and the code cannot drift together.
func TestActionSpecs_EnumOverridesConstrainPublishedFields(t *testing.T) {
	inputFields := map[string]map[string]bool{
		"gitlab_get_issue_statistics":         publishedJSONFields(GetInput{}),
		"gitlab_get_group_issue_statistics":   publishedJSONFields(GetGroupInput{}),
		"gitlab_get_project_issue_statistics": publishedJSONFields(GetProjectInput{}),
	}
	vocabularies := map[string][]any{
		"scope": {"created_by_me", "assigned_to_me", "all"},
		"in":    {"title", "description", "title,description"},
	}

	for _, spec := range issueStatsSpecs(t) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			published, known := inputFields[spec.IndividualTool.Name]
			if !known {
				t.Fatalf("no input struct is mapped to %s", spec.IndividualTool.Name)
			}
			assertOverridesNamePublishedFields(t, spec.InputSchemaOverrides, published)
			for field, want := range vocabularies {
				t.Run(field, func(t *testing.T) {
					assertVocabulary(t, spec.InputSchemaOverrides, published, field, want)
				})
			}
		})
	}
}

// assertOverridesNamePublishedFields checks that every override names a field
// the action's input publishes. One that does not patches nothing, silently.
func assertOverridesNamePublishedFields(t *testing.T, overrides []toolutil.InputSchemaOverride, published map[string]bool) {
	t.Helper()
	for _, override := range overrides {
		if !published[override.PropertyPath] {
			t.Errorf("override constrains %q, which this action's input does not publish, so it patches nothing", override.PropertyPath)
		}
	}
}

// statsScope is a scope one of these actions reads, in the two spellings its
// metadata uses: the bare word an alias is built from, and the phrase the
// prose claims it with.
type statsScope struct {
	word  string
	claim string
}

// statsScopes returns the scope the named tool reads and the scopes it must
// not claim, derived from the tool's own name.
//
// Deriving it from the name is what keeps the assertions below from being a
// copy of whatever the source says today: what is held is that a tool whose
// name says group describes a group. The global route reads no single scope,
// so its own is empty and it is held only to claiming neither of the others.
func statsScopes(tool string) (own statsScope, foreign []statsScope) {
	group := statsScope{word: "group", claim: "for a group"}
	project := statsScope{word: "project", claim: "for a single project"}
	switch {
	case strings.Contains(tool, "_group_"):
		return group, []statsScope{project}
	case strings.Contains(tool, "_project_"):
		return project, []statsScope{group}
	default:
		return statsScope{}, []statsScope{group, project}
	}
}

// TestActionSpecs_MetadataNamesTheScopeItsToolReads checks that the usage
// line, the individual-tool description and the natural-language aliases each
// action carries describe the scope that action actually reads.
//
// Why that matters: the three cases of decorateIssueStatisticsMeta are
// near-identical blocks of hand-written prose, and a copy-paste that leaves
// one action carrying another's sentences is invisible to both gates, text
// assigned to a field being no branch to flip. The tests around it asked only
// whether those fields were non-empty, so all three could have been permuted
// and stayed green. What a model reads to choose between three tools that
// differ in nothing but scope is exactly this text: a project tool promising
// counts "for a group and its descendant projects" is chosen for the wrong
// question, and the wrong counts come back with no sign that they are wrong.
func TestActionSpecs_MetadataNamesTheScopeItsToolReads(t *testing.T) {
	for _, spec := range issueStatsSpecs(t) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			own, foreign := statsScopes(spec.IndividualTool.Name)
			// The See-also tail names the sibling scopes on purpose, so the
			// claim is read from what precedes it.
			described, _, _ := strings.Cut(spec.IndividualTool.Description, "See also:")
			t.Run("Usage", func(t *testing.T) { assertScopeClaim(t, spec.Usage, own, foreign) })
			t.Run("Description", func(t *testing.T) { assertScopeClaim(t, described, own, foreign) })
			t.Run("Aliases", func(t *testing.T) { assertAliasScope(t, spec, own, foreign) })
		})
	}
}

// TestActionSpecs_UsageAndDescription_KeepTheirOwnShape checks that each
// action's usage line and individual-tool description keep the form of the
// surface that reads them, so the two cannot trade places.
//
// Why that matters: both strings of one action carry that action's scope
// phrase, so the scope test above is satisfied by either string in either
// place, and the pair is invisible to it as it is to both gates, one field's
// text assigned to another being no branch to flip. The two are not
// interchangeable to a reader. The usage line is what the dynamic and meta
// surfaces show a model choosing between actions; the description is the
// individual tool's own, and is the only one of the two that says what comes
// back and routes to the siblings. Crossed, the individual surface lists a tool
// whose description never names its return shape, and the finder offers a usage
// line that ends in a list of other tools' names.
func TestActionSpecs_UsageAndDescription_KeepTheirOwnShape(t *testing.T) {
	// The opening of the usage line and the tail of the description: the parts
	// each has and the other must not.
	const (
		usageOpening     = "Get aggregate issue counts (all, opened, closed) "
		describedReturns = "Returns: a statistics object with nested counts (all, opened, closed). See also: "
	)

	for _, spec := range issueStatsSpecs(t) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			t.Run("Usage", func(t *testing.T) {
				if !strings.HasPrefix(spec.Usage, usageOpening) {
					t.Errorf("Usage = %q, want it to open with %q", spec.Usage, usageOpening)
				}
				if strings.Contains(spec.Usage, "Returns:") || strings.Contains(spec.Usage, "See also:") {
					t.Errorf("Usage = %q, carrying a tail that belongs to the individual tool's description", spec.Usage)
				}
			})
			t.Run("Description", func(t *testing.T) {
				if !strings.Contains(spec.IndividualTool.Description, describedReturns) {
					t.Errorf("Description = %q, want it to carry %q", spec.IndividualTool.Description, describedReturns)
				}
			})
		})
	}
}

// assertScopeClaim checks that prose claims the action's own scope and none of
// the others'.
func assertScopeClaim(t *testing.T, prose string, own statsScope, foreign []statsScope) {
	t.Helper()
	if own.claim != "" && !strings.Contains(prose, own.claim) {
		t.Errorf("%q never says it reads %q", prose, own.claim)
	}
	for _, other := range foreign {
		if strings.Contains(prose, other.claim) {
			t.Errorf("%q claims to read %q, which is another action's scope", prose, other.claim)
		}
	}
}

// assertAliasScope checks that every natural-language alias names the scope
// its own tool reads, so a model searching for one scope is not handed
// another. The tool name itself is skipped: it is an alias of every action and
// carries the scope by construction.
func assertAliasScope(t *testing.T, spec toolutil.ActionSpec, own statsScope, foreign []statsScope) {
	t.Helper()
	for _, alias := range spec.Aliases {
		if alias == spec.IndividualTool.Name {
			continue
		}
		if own.word != "" && !strings.Contains(alias, own.word) {
			t.Errorf("alias %q does not name the %s scope it is registered under", alias, own.word)
		}
		for _, other := range foreign {
			if strings.Contains(alias, other.word) {
				t.Errorf("alias %q names the %s scope, which another action reads", alias, other.word)
			}
		}
	}
}

// assertVocabulary checks that a fixed-vocabulary field the action's input
// publishes is constrained to GitLab's own values.
func assertVocabulary(t *testing.T, overrides []toolutil.InputSchemaOverride, published map[string]bool, field string, want []any) {
	t.Helper()
	if !published[field] {
		return
	}
	got, ok := enumFor(overrides, field)
	if !ok {
		t.Fatalf("input publishes %q and no override constrains it to a vocabulary", field)
	}
	if !slices.Equal(got, want) {
		t.Errorf("%q enum = %v, want GitLab's vocabulary %v", field, got, want)
	}
}
