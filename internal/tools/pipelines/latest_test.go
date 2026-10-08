// latest_test.go covers pipeline.latest: the latest pipeline GitLab reports
// for a ref, and what the action answers when GitLab answers that route 403.
package pipelines

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The commits and pipelines the fallback fixtures speak about: the head of
// the ref, which has no pipeline on it, the earlier commit the newest
// pipeline on the ref ran for, and the merge commit GitLab builds for a
// merge request from the ref.
const (
	latestHeadSHA    = "1111111111111111111111111111111111111111"
	latestEarlierSHA = "2222222222222222222222222222222222222222"
	latestMergeSHA   = "3333333333333333333333333333333333333333"
)

// The refs of merge request 3, whose source branch is main: GitLab's list
// asked for ref main returns their pipelines too.
const (
	latestMRHeadRef  = "refs/merge-requests/3/head"
	latestMRMergeRef = "refs/merge-requests/3/merge"
)

// The answers the fallback fixtures give by default.
const (
	latestPlainRefusal = `{"message":"403 Forbidden"}`
	latestScopeRefusal = `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token.","scope":"api"}`
	latestListedRow    = `[{"id":99,"status":"success","ref":"main","source":"push","sha":"` + latestEarlierSHA + `"}]`
	latestPipeline     = `{"id":99,"iid":7,"project_id":42,"status":"success","source":"push","ref":"main","sha":"` +
		latestEarlierSHA + `","web_url":"https://gitlab.example.com/g/p/-/pipelines/99"}`
)

// latestGitLab stands in for the five routes pipeline.latest can read. Each
// answer is a status and a body, a zero status meaning 200, and every request
// is recorded as "METHOD path?query" so a test can assert which routes were
// asked and with what. listPages, when set, replaces listBody with one body
// per page of the list, each but the last naming the next page as GitLab's
// X-Next-Page does. The pipeline route answers detailBody for any ID, so a
// test reads which pipeline was asked for off the recorded path.
type latestGitLab struct {
	latestStatus  int
	latestBody    string
	projectStatus int
	projectBody   string
	listStatus    int
	listBody      string
	listPages     []string
	headStatus    int
	headBody      string
	detailStatus  int
	detailBody    string

	mu   sync.Mutex
	seen []string
}

// newLatestGitLab returns the stand-in for the case the issue describes: the
// latest route refuses, the default branch is main, the newest pipeline on
// main ran for an earlier commit, and the head of main is another commit.
func newLatestGitLab() *latestGitLab {
	return &latestGitLab{
		latestStatus: http.StatusForbidden,
		latestBody:   latestPlainRefusal,
		projectBody:  `{"id":42,"default_branch":"main"}`,
		listBody:     latestListedRow,
		headBody:     `{"id":"` + latestHeadSHA + `"}`,
		detailBody:   latestPipeline,
	}
}

// ServeHTTP answers the five routes and records each request.
func (g *latestGitLab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.seen = append(g.seen, r.Method+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
	g.mu.Unlock()

	path := r.URL.EscapedPath()
	switch {
	case path == "/api/v4/projects/42/pipelines/latest":
		respondLatest(w, g.latestStatus, g.latestBody)
	case path == "/api/v4/projects/42":
		respondLatest(w, g.projectStatus, g.projectBody)
	case path == pathProjectPipelines:
		g.respondList(w, r)
	case strings.HasPrefix(path, "/api/v4/projects/42/repository/commits/"):
		respondLatest(w, g.headStatus, g.headBody)
	case strings.HasPrefix(path, "/api/v4/projects/42/pipelines/"):
		respondLatest(w, g.detailStatus, g.detailBody)
	default:
		http.NotFound(w, r)
	}
}

// respondList answers the pipeline list: listBody under listStatus, or the
// page of listPages the request asks for, the first when it names none.
func (g *latestGitLab) respondList(w http.ResponseWriter, r *http.Request) {
	if g.listPages == nil {
		respondLatest(w, g.listStatus, g.listBody)
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	if page > len(g.listPages) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
		return
	}
	next := ""
	if page < len(g.listPages) {
		next = strconv.Itoa(page + 1)
	}
	testutil.RespondJSONWithPagination(w, http.StatusOK, g.listPages[page-1], testutil.PaginationHeaders{NextPage: next})
}

// listRow is one row of the pipeline list, on ref, from source, for sha.
func listRow(id int, ref, source, sha string) string {
	return `{"id":` + strconv.Itoa(id) + `,"status":"success","ref":"` + ref + `","source":"` + source + `","sha":"` + sha + `"}`
}

// respondLatest writes body under status, 200 when status is zero.
func respondLatest(w http.ResponseWriter, status int, body string) {
	if status == 0 {
		status = http.StatusOK
	}
	testutil.RespondJSON(w, status, body)
}

// requests returns what the stand-in was asked, in order.
func (g *latestGitLab) requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.seen)
}

// asked reports whether any request's "METHOD path?query" starts with prefix.
func (g *latestGitLab) asked(prefix string) bool {
	return slices.ContainsFunc(g.requests(), func(seen string) bool { return strings.HasPrefix(seen, prefix) })
}

// listQuery returns the query string the pipeline list was asked with.
func (g *latestGitLab) listQuery(t *testing.T) string {
	t.Helper()
	for _, seen := range g.requests() {
		if query, ok := strings.CutPrefix(seen, "GET "+pathProjectPipelines+"?"); ok {
			return query
		}
	}
	t.Fatalf("the pipeline list was never asked: %v", g.requests())
	return ""
}

// callLatest runs GetLatest against the stand-in.
func callLatest(t *testing.T, g *latestGitLab, input GetLatestInput) (LatestOutput, error) {
	t.Helper()
	client := testutil.NewTestClient(t, g)
	if input.ProjectID == "" {
		input.ProjectID = "42"
	}
	return GetLatest(context.Background(), client, input)
}

// TestGetLatest_Answered_IsGitLabsLatestAndSaysNothingMore verifies the
// route's own answer: the pipeline of the commit at the head of the ref,
// presented as it came, with nothing read beside it and no note.
func TestGetLatest_Answered_IsGitLabsLatestAndSaysNothingMore(t *testing.T) {
	g := newLatestGitLab()
	g.latestStatus, g.latestBody = 0, pipelineDetailJSON

	out, err := callLatest(t, g, GetLatestInput{Ref: "main"})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if out.ID != 10 || out.Status != statusSuccess {
		t.Errorf("GetLatest() = pipeline %d %q, want pipeline 10 %q", out.ID, out.Status, statusSuccess)
	}
	if out.HeadSHA != "" || out.FallbackNote != "" {
		t.Errorf("GetLatest() head_sha = %q, fallback_note = %q, want both empty for GitLab's own answer", out.HeadSHA, out.FallbackNote)
	}
	if got := g.requests(); len(got) != 1 || got[0] != "GET /api/v4/projects/42/pipelines/latest?ref=main" {
		t.Errorf("requests = %v, want the latest route alone, asked for ref main", got)
	}
}

// TestGetLatest_Answered_ReadsArchived verifies the latest pipeline, which
// GitLab renders whole, carries the archived flag off the captured answer.
func TestGetLatest_Answered_ReadsArchived(t *testing.T) {
	g := newLatestGitLab()
	g.latestStatus, g.latestBody = 0, `{"id":10,"status":"success","archived":true}`

	out, err := callLatest(t, g, GetLatestInput{})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if !out.Archived {
		t.Errorf("GetLatest() = %+v, want archived read off the captured answer", out)
	}
}

// TestGetLatest_NoPipelineAtTheDefaultBranchHead_ShowsTheNewestOnThatBranch
// is the case of the issue: no ref, so GitLab was asked about the default
// branch, whose head commit has no pipeline. The fallback resolves the
// default branch, lists that branch alone, and says which commit the head is
// and which one the pipeline shown ran for.
func TestGetLatest_NoPipelineAtTheDefaultBranchHead_ShowsTheNewestOnThatBranch(t *testing.T) {
	g := newLatestGitLab()

	out, err := callLatest(t, g, GetLatestInput{})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if out.ID != 99 || out.SHA != latestEarlierSHA || out.Ref != "main" {
		t.Errorf("GetLatest() = pipeline %d on %q at %s, want pipeline 99 on main at %s", out.ID, out.Ref, out.SHA, latestEarlierSHA)
	}
	if out.HeadSHA != latestHeadSHA {
		t.Errorf("head_sha = %q, want %q", out.HeadSHA, latestHeadSHA)
	}
	if out.FallbackNote != noteHeadWithoutPipeline {
		t.Errorf("fallback_note = %q, want %q", out.FallbackNote, noteHeadWithoutPipeline)
	}
	query := g.listQuery(t)
	for _, param := range []string{"ref=main", "order_by=id", "sort=desc"} {
		t.Run(param, func(t *testing.T) {
			if !strings.Contains(query, param) {
				t.Errorf("list query = %q, want it to carry %s", query, param)
			}
		})
	}
	want := []string{
		"GET /api/v4/projects/42/pipelines/latest?",
		"GET /api/v4/projects/42?",
		"GET " + pathProjectPipelines + "?" + query,
		"GET /api/v4/projects/42/repository/commits/main?",
		"GET /api/v4/projects/42/pipelines/99?",
	}
	if got := g.requests(); !slices.Equal(got, want) {
		t.Errorf("requests = %v\nwant %v", got, want)
	}
}

// TestGetLatest_NoPipelineAtTheRefHead_ShowsTheNewestOnTheRef verifies the
// same answer for a ref the caller named: the project is not read, the list
// and the head lookup take the ref as given, a slash in it included.
func TestGetLatest_NoPipelineAtTheRefHead_ShowsTheNewestOnTheRef(t *testing.T) {
	g := newLatestGitLab()
	g.listBody = `[` + listRow(99, "release/1.0", "push", latestEarlierSHA) + `]`

	out, err := callLatest(t, g, GetLatestInput{Ref: "release/1.0"})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if out.HeadSHA != latestHeadSHA || out.FallbackNote != noteHeadWithoutPipeline {
		t.Errorf("GetLatest() head_sha = %q, fallback_note = %q, want the head of release/1.0 and the note", out.HeadSHA, out.FallbackNote)
	}
	if g.asked("GET /api/v4/projects/42?") {
		t.Errorf("the project was read although the ref was given: %v", g.requests())
	}
	if query := g.listQuery(t); !strings.Contains(query, "ref=release%2F1.0") {
		t.Errorf("list query = %q, want ref=release%%2F1.0", query)
	}
	if !g.asked("GET /api/v4/projects/42/repository/commits/release%2F1.0?") {
		t.Errorf("the head of release/1.0 was not read: %v", g.requests())
	}
}

// fallbackListQuery drives GetLatest down its fallback for ref main and
// hands back the query the list request carried, so a test can state what
// the fallback asked GitLab for rather than only what it decoded.
func fallbackListQuery(t *testing.T, input GetLatestInput) url.Values {
	t.Helper()
	g := newLatestGitLab()
	input.Ref = "main"
	if _, err := callLatest(t, g, input); err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	query, err := url.ParseQuery(g.listQuery(t))
	if err != nil {
		t.Fatalf("parsing the list query: %v", err)
	}
	return query
}

// TestGetLatest_FallbackDefaults_AskNewestFirst states what the fallback has
// to ask for when the caller named no ordering. It stands in for
// /pipelines/latest, so it must ask for the pipelines newest first; any other
// ordering answers with a pipeline that is not the newest on the ref, and the
// decoded output looks the same either way. The page size is GitLab's own,
// because the first row on the ref is not always the first row the list
// gives, see [TestGetLatest_TheListAddsMergeRequestRefs_ShowsThePipelineOnTheRefItself].
func TestGetLatest_FallbackDefaults_AskNewestFirst(t *testing.T) {
	query := fallbackListQuery(t, GetLatestInput{})

	defaults := map[string]string{"order_by": "id", "sort": "desc"}
	for param, want := range defaults {
		t.Run(param, func(t *testing.T) {
			if got := query.Get(param); got != want {
				t.Errorf("fallback %s = %q, want %q", param, got, want)
			}
		})
	}
	if query.Has("per_page") {
		t.Errorf("fallback per_page = %q, want GitLab's own page size", query.Get("per_page"))
	}
}

// TestGetLatest_FallbackKeepsTheCallersOrdering is the other half: those three
// are defaults, so an order_by, sort or per_page the caller supplied has to
// reach GitLab unchanged rather than being overwritten by them.
func TestGetLatest_FallbackKeepsTheCallersOrdering(t *testing.T) {
	query := fallbackListQuery(t, GetLatestInput{
		OrderBy: "updated_at",
		Sort:    "asc",
		PerPage: 50,
	})

	supplied := map[string]string{"order_by": "updated_at", "sort": "asc", "per_page": "50"}
	for param, want := range supplied {
		t.Run(param, func(t *testing.T) {
			if got := query.Get(param); got != want {
				t.Errorf("fallback %s = %q, want the caller's %q", param, got, want)
			}
		})
	}
}

// TestGetLatest_FallbackKeysetAndFilters verifies that the list fallback honors
// keyset pagination and the additional ListProjectPipelinesOptions filters that
// GetLatestInput mirrors, beside the ref it lists.
func TestGetLatest_FallbackKeysetAndFilters(t *testing.T) {
	query := fallbackListQuery(t, GetLatestInput{
		Status: "success", Source: "push",
		Pagination: "keyset", PageToken: "cur-9",
	})

	supplied := map[string]string{"pagination": "keyset", "page_token": "cur-9", "status": "success", "source": "push", "ref": "main"}
	for param, want := range supplied {
		t.Run(param, func(t *testing.T) {
			if got := query.Get(param); got != want {
				t.Errorf("fallback %s = %q, want %q", param, got, want)
			}
		})
	}
}

// TestGetLatest_TheListAddsMergeRequestRefs_ShowsThePipelineOnTheRefItself
// verifies what the fallback keeps of a list asked for a branch. GitLab adds
// to it the pipelines on the refs of every merge request the branch is the
// source of, which Project#latest_pipeline never counts: the merge request's
// head pipeline of the head commit, and its merged-results pipeline of the
// merge commit. The fallback passes them over, reading the next page when the
// first holds nothing else, and answers with the pipeline on the branch
// itself, of the earlier commit, under the note that says so.
func TestGetLatest_TheListAddsMergeRequestRefs_ShowsThePipelineOnTheRefItself(t *testing.T) {
	g := newLatestGitLab()
	g.listPages = []string{
		`[` + listRow(102, latestMRHeadRef, "merge_request_event", latestHeadSHA) + `,` +
			listRow(101, latestMRMergeRef, "merge_request_event", latestMergeSHA) + `]`,
		`[` + listRow(99, "main", "push", latestEarlierSHA) + `]`,
	}

	out, err := callLatest(t, g, GetLatestInput{})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if !g.asked("GET /api/v4/projects/42/pipelines/99?") || g.asked("GET /api/v4/projects/42/pipelines/102?") || g.asked("GET /api/v4/projects/42/pipelines/101?") {
		t.Errorf("requests = %v, want pipeline 99 read and neither merge request pipeline", g.requests())
	}
	if !g.asked("GET "+pathProjectPipelines+"?") || !slices.ContainsFunc(g.requests(), func(seen string) bool {
		return strings.HasPrefix(seen, "GET "+pathProjectPipelines+"?") && strings.Contains(seen, "page=2")
	}) {
		t.Errorf("requests = %v, want the second page of the list read", g.requests())
	}
	if out.HeadSHA != latestHeadSHA || out.FallbackNote != noteHeadWithoutPipeline {
		t.Errorf("GetLatest() head_sha = %q, fallback_note = %q, want the head and the note", out.HeadSHA, out.FallbackNote)
	}
}

// TestGetLatest_NoPipelineOnTheRefItself_SaysSo verifies a branch whose only
// listed pipelines are on its merge request refs: none is on the branch, so
// the action answers that the ref has no pipeline rather than with one of
// them.
func TestGetLatest_NoPipelineOnTheRefItself_SaysSo(t *testing.T) {
	g := newLatestGitLab()
	g.listBody = `[` + listRow(102, latestMRHeadRef, "merge_request_event", latestHeadSHA) + `]`

	_, err := callLatest(t, g, GetLatestInput{Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), `no pipeline found on ref "main"`) {
		t.Fatalf("GetLatest() error = %v, want one saying main has no pipeline", err)
	}
	if g.asked("GET /api/v4/projects/42/pipelines/102?") {
		t.Errorf("the merge request pipeline was read: %v", g.requests())
	}
}

// TestGetLatest_DanglingSources_CountOnlyWhenAskedFor verifies the other half
// of what Project#latest_pipeline counts: the pipelines of the ref from a
// source that sets its status, which leaves out a Web IDE terminal, an
// on-demand DAST scan or validation, a security policy's own pipeline and a
// child pipeline. The fallback passes them over unless the caller asked for
// that source, in which case GitLab returned nothing else.
func TestGetLatest_DanglingSources_CountOnlyWhenAskedFor(t *testing.T) {
	rows := `[` + listRow(104, "main", "webide", latestEarlierSHA) + `,` +
		listRow(103, "main", "security_orchestration_policy", latestEarlierSHA) + `,` +
		listRow(100, "main", "ondemand_dast_scan", latestEarlierSHA) + `,` +
		listRow(101, "main", "ondemand_dast_validation", latestEarlierSHA) + `,` +
		listRow(105, "main", "parent_pipeline", latestEarlierSHA) + `,` +
		listRow(99, "main", "schedule", latestEarlierSHA) + `]`
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{name: "no source named", source: "", want: "99"},
		{name: "a dangling source named", source: "webide", want: "104"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			g.listBody = rows

			if _, err := callLatest(t, g, GetLatestInput{Ref: "main", Source: tc.source}); err != nil {
				t.Fatalf("GetLatest() unexpected error: %v", err)
			}
			if !g.asked("GET /api/v4/projects/42/pipelines/" + tc.want + "?") {
				t.Errorf("requests = %v, want pipeline %s read", g.requests(), tc.want)
			}
		})
	}
}

// TestGetLatest_TheHeadHasThePipeline_SaysNothingMore verifies a list whose
// first pipeline did run for the head commit, as one created between the two
// requests did: that pipeline is the head's, so nothing is said beside it.
func TestGetLatest_TheHeadHasThePipeline_SaysNothingMore(t *testing.T) {
	g := newLatestGitLab()
	g.headBody = `{"id":"` + latestEarlierSHA + `"}`

	out, err := callLatest(t, g, GetLatestInput{Ref: "main"})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if out.ID != 99 || out.HeadSHA != "" || out.FallbackNote != "" {
		t.Errorf("GetLatest() = pipeline %d, head_sha %q, fallback_note %q, want pipeline 99 and neither", out.ID, out.HeadSHA, out.FallbackNote)
	}
}

// TestGetLatest_TheHeadCannotBeRead_SaysSo verifies the two answers GitLab
// gives a head lookup it will not serve, a ref that does not exist or a
// token that cannot read the repository: the pipeline is still shown, no
// head is claimed, and the note says the head could not be read.
func TestGetLatest_TheHeadCannotBeRead_SaysSo(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"message":"404 Commit Not Found"}`},
		{name: "refused", status: http.StatusForbidden, body: latestPlainRefusal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			g.headStatus, g.headBody = tc.status, tc.body

			out, err := callLatest(t, g, GetLatestInput{Ref: "main"})
			if err != nil {
				t.Fatalf("GetLatest() unexpected error: %v", err)
			}
			if out.ID != 99 || out.HeadSHA != "" {
				t.Errorf("GetLatest() = pipeline %d, head_sha %q, want pipeline 99 and no head", out.ID, out.HeadSHA)
			}
			if out.FallbackNote != noteHeadUnreadable {
				t.Errorf("fallback_note = %q, want %q", out.FallbackNote, noteHeadUnreadable)
			}
		})
	}
}

// TestGetLatest_TheHeadLookupFails_ReturnsTheError verifies that a head
// lookup GitLab failed rather than refused is an error of the action: an
// answer GitLab never gave is not one to fill in.
func TestGetLatest_TheHeadLookupFails_ReturnsTheError(t *testing.T) {
	g := newLatestGitLab()
	g.headStatus, g.headBody = http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`

	_, err := callLatest(t, g, GetLatestInput{Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), opGetLatestHead) {
		t.Fatalf("GetLatest() error = %v, want the head lookup's failure", err)
	}
}

// TestGetLatest_NoPipelineOnTheRef_SaysSo verifies a ref the list holds no
// pipeline for: the action answers with an error naming the ref, and reads
// neither its head nor a pipeline.
func TestGetLatest_NoPipelineOnTheRef_SaysSo(t *testing.T) {
	g := newLatestGitLab()
	g.listBody = `[]`

	_, err := callLatest(t, g, GetLatestInput{})
	if err == nil {
		t.Fatal("GetLatest() error = nil, want one saying main has no pipeline")
	}
	for _, part := range []string{`"main"`, "no pipeline", "branch.get"} {
		t.Run(part, func(t *testing.T) {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("error = %q, want it to name %q", err, part)
			}
		})
	}
	if g.asked("GET /api/v4/projects/42/repository/commits/") || g.asked("GET /api/v4/projects/42/pipelines/99") {
		t.Errorf("a head or a pipeline was read for a ref with none: %v", g.requests())
	}
}

// TestGetLatest_NoPipelineUnderTheFilters_SaysToDropThem verifies the answer
// for a list the caller's own filters emptied: the ref exists and may well
// have pipelines, so the error names the filters rather than the ref. Page 1
// is where the list starts anyway and filters nothing; the ordering and the
// page size only arrange the rows.
func TestGetLatest_NoPipelineUnderTheFilters_SaysToDropThem(t *testing.T) {
	cases := []struct {
		name     string
		input    GetLatestInput
		filtered bool
	}{
		{name: "status", input: GetLatestInput{Status: "success"}, filtered: true},
		{name: "created after", input: GetLatestInput{CreatedAfter: "2026-01-01T00:00:00Z"}, filtered: true},
		{name: "a later page", input: GetLatestInput{PaginationInput: toolutil.PaginationInput{Page: 2}}, filtered: true},
		{name: "a keyset cursor", input: GetLatestInput{KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "cur-9"}}, filtered: true},
		{name: "the first page", input: GetLatestInput{PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 50}}, filtered: false},
		{name: "an ordering", input: GetLatestInput{OrderBy: "updated_at", Sort: "asc", KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset"}}, filtered: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			g.listBody = `[]`
			tc.input.Ref = "main"

			_, err := callLatest(t, g, tc.input)
			if err == nil {
				t.Fatal("GetLatest() error = nil, want one saying main has no pipeline")
			}
			if got := strings.Contains(err.Error(), "Drop the filters"); got != tc.filtered {
				t.Errorf("error = %q, names the filters = %t, want %t", err, got, tc.filtered)
			}
			if got := strings.Contains(err.Error(), "branch.get"); got == tc.filtered {
				t.Errorf("error = %q, sends the caller to check the ref = %t, want %t", err, got, !tc.filtered)
			}
		})
	}
}

// TestGetLatest_TheListIsRefused_IsThePermissionRefusal verifies the one
// answer that tells the two meanings of the first 403 apart: the list of the
// same project refused as well, so the caller cannot read its pipelines and
// the error says which role can, while a refusal of the token's scope names
// no role. The role is Reporter: a Planner holds pipelines only the way a
// Guest does, through Project-based pipeline visibility, so a hint naming
// Planner would send a refused Planner to the role it already has.
func TestGetLatest_TheListIsRefused_IsThePermissionRefusal(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantHint bool
	}{
		{name: "permission", body: latestPlainRefusal, wantHint: true},
		{name: "token scope", body: latestScopeRefusal, wantHint: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			g.listStatus, g.listBody = http.StatusForbidden, tc.body

			_, err := callLatest(t, g, GetLatestInput{Ref: "main"})
			if err == nil {
				t.Fatal("GetLatest() error = nil, want the list's refusal")
			}
			if !toolutil.IsHTTPStatus(err, http.StatusForbidden) {
				t.Errorf("error = %v, want GitLab's 403 kept", err)
			}
			if got := strings.Contains(err.Error(), "needs the Reporter role or higher"); got != tc.wantHint {
				t.Errorf("error = %q, role hint present = %t, want %t", err, got, tc.wantHint)
			}
			if g.asked("GET /api/v4/projects/42/repository/commits/") {
				t.Errorf("the head was read after the list was refused: %v", g.requests())
			}
		})
	}
}

// TestGetLatest_TheListFails_ReturnsTheError verifies a list GitLab failed
// rather than refused: the error carries GitLab's message and no role.
func TestGetLatest_TheListFails_ReturnsTheError(t *testing.T) {
	g := newLatestGitLab()
	g.listStatus, g.listBody = http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`

	_, err := callLatest(t, g, GetLatestInput{Ref: "main"})
	if err == nil || strings.Contains(err.Error(), "Reporter") || !strings.Contains(err.Error(), opGetLatestFallback) {
		t.Fatalf("GetLatest() error = %v, want the list's failure without a role hint", err)
	}
}

// TestGetLatest_RefusedForAnotherReason_DoesNotFallBack verifies the answers
// of the latest route that are not the route's own 403: a refusal of the
// token's scope, a 401 and another status. None of them is a missing
// pipeline, so nothing else is asked.
func TestGetLatest_RefusedForAnotherReason_DoesNotFallBack(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "token scope", status: http.StatusForbidden, body: latestScopeRefusal},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"message":"401 Unauthorized"}`},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"404 Project Not Found"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			g.latestStatus, g.latestBody = tc.status, tc.body

			_, err := callLatest(t, g, GetLatestInput{})
			if err == nil || !toolutil.IsHTTPStatus(err, tc.status) {
				t.Fatalf("GetLatest() error = %v, want GitLab's %d", err, tc.status)
			}
			if got := g.requests(); len(got) != 1 {
				t.Errorf("requests = %v, want the latest route alone", got)
			}
		})
	}
}

// TestGetLatest_TheProjectCannotBeRead_ReturnsTheError verifies that a
// default branch the project read did not give is an error of the action.
func TestGetLatest_TheProjectCannotBeRead_ReturnsTheError(t *testing.T) {
	g := newLatestGitLab()
	g.projectStatus, g.projectBody = http.StatusNotFound, `{"message":"404 Project Not Found"}`

	_, err := callLatest(t, g, GetLatestInput{})
	if err == nil || !strings.Contains(err.Error(), opGetLatestDefaultBranch) {
		t.Fatalf("GetLatest() error = %v, want the project read's failure", err)
	}
	if strings.Contains(err.Error(), "pass ref") {
		t.Errorf("error = %q, want no pointer to ref for a project GitLab did not find", err)
	}
	if g.asked("GET " + pathProjectPipelines + "?") {
		t.Errorf("the list was asked without a ref: %v", g.requests())
	}
}

// TestGetLatest_TheProjectReadIsRefused_SaysToPassRef verifies a project
// read GitLab refused after its latest route found the project: a token
// that may read pipelines and not the project, as a fine-grained one granted
// Pipeline: Read alone may. The pipeline list and the commit read do not
// need the project, so the error says to pass ref, and says it whatever the
// refusal, since a hint on a fine-grained refusal would be dropped.
func TestGetLatest_TheProjectReadIsRefused_SaysToPassRef(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "plain", body: latestPlainRefusal},
		{name: "fine-grained", body: `{"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following project permissions: [Project: Read]."}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			g.projectStatus, g.projectBody = http.StatusForbidden, tc.body

			_, err := callLatest(t, g, GetLatestInput{})
			if err == nil || !strings.Contains(err.Error(), "pass ref") {
				t.Fatalf("GetLatest() error = %v, want one saying to pass ref", err)
			}
			if !toolutil.IsHTTPStatus(err, http.StatusForbidden) {
				t.Errorf("error = %v, want GitLab's 403 kept", err)
			}
			if g.asked("GET " + pathProjectPipelines + "?") {
				t.Errorf("the list was asked without a ref: %v", g.requests())
			}
		})
	}
}

// TestGetLatest_TheDefaultBranchIsWithheld_AsksTheListWhetherPipelinesCanBeRead
// verifies a project read that names no default branch. GitLab sends one for
// every project, the instance's default name for an empty repository, and
// leaves the key out only for a caller who cannot read the repository, a
// Guest of a private project among them. Whether that caller can read
// pipelines is still open, so the list of the whole project is asked for one
// row: its refusal is the permission refusal, and its answer leaves the ref
// as the one thing missing.
func TestGetLatest_TheDefaultBranchIsWithheld_AsksTheListWhetherPipelinesCanBeRead(t *testing.T) {
	cases := []struct {
		name       string
		listStatus int
		listBody   string
		want       string
	}{
		{name: "pipelines readable", listBody: latestListedRow, want: "Pass ref"},
		{name: "pipelines refused", listStatus: http.StatusForbidden, listBody: latestPlainRefusal, want: "needs the Reporter role or higher"},
		{name: "list failed", listStatus: http.StatusInternalServerError, listBody: `{"message":"500 Internal Server Error"}`, want: opGetLatestFallback},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			g.projectBody = `{"id":42}`
			g.listStatus, g.listBody = tc.listStatus, tc.listBody

			_, err := callLatest(t, g, GetLatestInput{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("GetLatest() error = %v, want it to say %q", err, tc.want)
			}
			query, perr := url.ParseQuery(g.listQuery(t))
			if perr != nil {
				t.Fatalf("parsing the list query: %v", perr)
			}
			if query.Has("ref") || query.Get("per_page") != "1" {
				t.Errorf("list query = %v, want one row of the whole project", query)
			}
			if g.asked("GET /api/v4/projects/42/repository/commits/") || g.asked("GET /api/v4/projects/42/pipelines/99") {
				t.Errorf("a head or a pipeline was read without a ref: %v", g.requests())
			}
		})
	}
}

// TestGetLatest_ThePipelineCannotBeRead_ReturnsTheError verifies the last
// read of the fallback, the listed pipeline in full.
func TestGetLatest_ThePipelineCannotBeRead_ReturnsTheError(t *testing.T) {
	g := newLatestGitLab()
	g.detailStatus, g.detailBody = http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`

	_, err := callLatest(t, g, GetLatestInput{Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), opGetLatestFallback) {
		t.Fatalf("GetLatest() error = %v, want the pipeline read's failure", err)
	}
}

// TestGetLatest_MissingProject verifies the one input the action requires.
func TestGetLatest_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetLatest(context.Background(), client, GetLatestInput{})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestGetLatest_CancelledContext verifies a call whose context has ended
// asks nothing.
func TestGetLatest_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetLatest(testutil.CancelledCtx(t), client, GetLatestInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetLatest_EveryRequestCarriesTheContext verifies each request the
// action can send is sent with the handler's context: the latest route, the
// project, the list (its first page, a later page, and the one row asked
// when the default branch is withheld), the head and the pipeline. The
// stand-in answers every request before the one under test, then ends the
// context as that one arrives and holds it: a request carrying the context is
// abandoned at once, and one sent without it sits out the hold, which the
// test counts. [testutil.CancelOnArrival] stops at the first request, so it
// cannot reach the ones the fallback sends after it.
func TestGetLatest_EveryRequestCarriesTheContext(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		query string
		setup func(g *latestGitLab)
	}{
		{name: "latest", path: "/api/v4/projects/42/pipelines/latest"},
		{name: "project", path: "/api/v4/projects/42"},
		{name: "list", path: pathProjectPipelines},
		{name: "list page 2", path: pathProjectPipelines, query: "page=2", setup: func(g *latestGitLab) {
			g.listPages = []string{`[` + listRow(102, latestMRHeadRef, "merge_request_event", latestHeadSHA) + `]`, latestListedRow}
		}},
		{name: "list without a ref", path: pathProjectPipelines, setup: func(g *latestGitLab) { g.projectBody = `{"id":42}` }},
		{name: "head", path: "/api/v4/projects/42/repository/commits/main"},
		{name: "pipeline", path: "/api/v4/projects/42/pipelines/99"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLatestGitLab()
			if tc.setup != nil {
				tc.setup(g)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var heldOut atomic.Int64
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != tc.path || !strings.Contains(r.URL.RawQuery, tc.query) {
					g.ServeHTTP(w, r)
					return
				}
				cancel()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				select {
				case <-r.Context().Done():
				case <-timer.C:
					heldOut.Add(1)
					g.ServeHTTP(w, r)
				}
			}))

			_, err := GetLatest(ctx, client, GetLatestInput{ProjectID: "42"})
			if err == nil {
				t.Fatal("GetLatest() error = nil, want the cancellation")
			}
			if n := heldOut.Load(); n != 0 {
				t.Errorf("the %s request sat out the hold %d time(s): it was sent without the handler's context", tc.name, n)
			}
		})
	}
}

// TestFormatLatestMarkdown_GitLabsLatest_IsThePipelineCard verifies that
// GitLab's own answer renders exactly as a pipeline does.
func TestFormatLatestMarkdown_GitLabsLatest_IsThePipelineCard(t *testing.T) {
	detail := DetailOutput{ID: 1, Status: "pending", Source: "web", Ref: "dev", SHA: "xyz", WebURL: "https://gitlab.example.com/-/pipelines/1"}
	if got, want := FormatLatestMarkdown(LatestOutput{DetailOutput: detail}), FormatDetailMarkdown(detail); got != want {
		t.Errorf("FormatLatestMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatLatestMarkdown_Fallback_ShowsTheHeadAndTheNote verifies the
// fallback's card: the head commit beside the pipeline's own and the note
// as a paragraph after the rows, before the hints.
func TestFormatLatestMarkdown_Fallback_ShowsTheHeadAndTheNote(t *testing.T) {
	detail := DetailOutput{ID: 99, Status: "success", Source: "push", Ref: "main", SHA: latestEarlierSHA, WebURL: "https://gitlab.example.com/-/pipelines/99"}
	want := "## ✅ Pipeline #99: success\n\n" +
		"- **IID**: 0\n" +
		"- **Source**: push\n" +
		"- **Ref**: main\n" +
		"- **Tag**: ❌\n" +
		"- **SHA**: `" + latestEarlierSHA + "`\n" +
		"- **Head SHA**: `" + latestHeadSHA + "`\n" +
		"- **URL**: [https://gitlab.example.com/-/pipelines/99](https://gitlab.example.com/-/pipelines/99)\n" +
		"\n" + noteHeadWithoutPipeline + "\n" +
		detailHints
	if got := FormatLatestMarkdown(fallbackOutput(detail, latestHeadSHA)); got != want {
		t.Errorf("FormatLatestMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatLatestMarkdown_Fallback_KeepsTheRefOutOfTheNote verifies a ref
// carrying Markdown and HTML metacharacters, which git check-ref-format
// permits in a branch name anyone who may push can create. The card escapes
// it in its row, and the note, which is the server's own prose and is written
// as it stands, never repeats it, whichever note the fallback wrote.
func TestFormatLatestMarkdown_Fallback_KeepsTheRefOutOfTheNote(t *testing.T) {
	const ref = "feat/<img src=x>_a_|b"
	for _, head := range []string{latestHeadSHA, ""} {
		t.Run("head "+strconv.Quote(head), func(t *testing.T) {
			out := fallbackOutput(DetailOutput{ID: 99, Status: "success", Ref: ref, SHA: latestEarlierSHA}, head)
			md := FormatLatestMarkdown(out)
			if out.FallbackNote == "" || !strings.Contains(md, out.FallbackNote) {
				t.Fatalf("FormatLatestMarkdown() = %q, want the note %q in it", md, out.FallbackNote)
			}
			if strings.Contains(md, "<img") || strings.Contains(md, "_a_|b") {
				t.Errorf("FormatLatestMarkdown() = %q, want the ref escaped wherever it appears", md)
			}
		})
	}
}
