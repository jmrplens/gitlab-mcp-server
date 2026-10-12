// search_test.go contains unit tests for the GitLab API search functions
// (projects, groups, users, merge requests, issues, branches, tags).
// Tests verify successful searches, empty queries, API errors, and context
// cancellation using httptest mocks.
package completions

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// Shared test assertion messages and subtest names for search tests.
const (
	msgExpectedAPIErr    = "expected error on API failure"
	fmtUnexpectedValue0  = "unexpected value[0]: %s"
	fmtExpected3Values   = "expected 3 values, got %d"
	subtestEmptyQueryAll = "empty query returns all"
)

// TestSearchProjects verifies that [searchProjects] returns formatted project
// entries matching the given query.
func TestSearchProjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"path_with_namespace":"group/alpha"},
				{"id":2,"path_with_namespace":"group/beta"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchProjects(context.Background(), client, "group")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "group/alpha" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchProjects_EmptyQuery verifies that [searchProjects] omits the search
// parameter when the query is empty.
func TestSearchProjects_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			if r.URL.Query().Get("search") != "" {
				t.Errorf("expected no search param for empty query, got %q", r.URL.Query().Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"path_with_namespace":"team/repo"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchProjects(context.Background(), client, "")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 1 {
		t.Fatalf(fmtExpected1Value, len(values))
	}
}

// gitlabSearchMatches answers for one column as GitLab's fuzzy search does
// (Gitlab::SQL::Pattern): every word of three characters or more has to occur
// in it, ignoring case, and a query with no such word has to equal it whole.
// It reads words only; the quoted phrases GitLab also takes as terms are held
// by TestSearchTerms_SplitsAsGitLabDoes.
func gitlabSearchMatches(column, query string) bool {
	column = strings.ToLower(column)
	words := strings.Fields(strings.ToLower(query))
	var long []string
	for _, word := range words {
		if len([]rune(word)) >= 3 {
			long = append(long, word)
		}
	}
	if len(long) == 0 {
		return column == strings.Join(words, " ")
	}
	for _, word := range long {
		if !strings.Contains(column, word) {
			return false
		}
	}
	return true
}

// TestSearchProjects_AFullPathOrAShortName_CompletesAsGitLabMatches verifies
// that what a caller types is matched against the full path the completion
// offers, and that a project's own short name typed whole still completes.
//
// GitLab's Project.search matches the path, the name and the description of a
// project, and matches the route (the full path and full name) instead when
// the request carries search_namespaces. Either way a term of three
// characters or more is matched anywhere in a column, and a query with no
// such term is compared with the column whole. The completion offers
// path_with_namespace, so without the flag typing that value back, or any
// part of it reaching into the namespace, matched nothing; with it a name of
// one or two characters typed whole matches nothing, since no full path or
// full name equals it. The mock answers by those rules.
func TestSearchProjects_AFullPathOrAShortName_CompletesAsGitLabMatches(t *testing.T) {
	const (
		fullPath = "rev-prompts/p1"
		fullName = "Rev Prompts / p1"
	)
	var flagged bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		search := query.Get("search")
		flagged = query.Get("search_namespaces") == "true"
		columns := []string{"p1", "p1"}
		if flagged {
			columns = []string{fullPath, fullName}
		}
		if !slices.ContainsFunc(columns, func(column string) bool { return gitlabSearchMatches(column, search) }) {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"path_with_namespace":"`+fullPath+`"}]`)
	}))

	cases := []struct {
		name     string
		typed    string
		found    bool
		flagSent bool
	}{
		{name: "the full path", typed: fullPath, found: true, flagSent: true},
		{name: "a part reaching into the namespace", typed: "rev-pr", found: true, flagSent: true},
		{name: "a long word beside a short one", typed: "rev p1", found: true, flagSent: true},
		{name: "a short name typed whole", typed: "p1", found: true, flagSent: false},
		{name: "a short name in capitals", typed: "P1", found: true, flagSent: false},
		{name: "a short part of nothing whole", typed: "ev", found: false, flagSent: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			values, _, err := searchProjects(context.Background(), client, testCase.typed)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if flagged != testCase.flagSent {
				t.Errorf("searchProjects(%q) sent search_namespaces = %t, want %t", testCase.typed, flagged, testCase.flagSent)
			}
			want := []string{}
			if testCase.found {
				want = []string{fullPath}
			}
			if !slices.Equal(values, want) {
				t.Errorf("searchProjects(%q) = %v, want %v", testCase.typed, values, want)
			}
		})
	}

	t.Run("nothing typed asks for no namespace matching", func(t *testing.T) {
		var sent bool
		empty := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sent = r.URL.Query().Has("search_namespaces")
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		}))
		if _, _, err := searchProjects(context.Background(), empty, ""); err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if sent {
			t.Error("search_namespaces was sent although nothing was typed to match")
		}
	})
}

// TestSearchTerms_SplitsAsGitLabDoes holds [searchTerms] to
// Gitlab::SQL::Pattern.split_query_to_search_terms on a squished query: the
// words, then the double-quoted phrases that stand on their own, without their
// quotes, and nothing else read as a phrase.
func TestSearchTerms_SplitsAsGitLabDoes(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "nothing", query: "", want: nil},
		{name: "words, whitespace collapsed", query: "  rev \t p1 ", want: []string{"rev", "p1"}},
		{name: "a phrase that is the whole query", query: `"rev p1"`, want: []string{"rev p1"}},
		{name: "a phrase between words", query: `x "a b" y`, want: []string{"x", "y", "a b"}},
		{name: "a phrase of one character", query: `"a" b`, want: []string{"b", "a"}},
		{name: "two phrases a space apart", query: `"a" "b"`, want: []string{"a", "b"}},
		{name: "empty quotes are a word", query: `"" ab`, want: []string{`""`, "ab"}},
		{name: "a quote inside a word opens nothing", query: `a"bc d"`, want: []string{`a"bc`, `d"`}},
		{name: "a closing quote inside a word closes nothing", query: `"ab"cd`, want: []string{`"ab"cd`}},
		{name: "a quote never closed", query: `"open`, want: []string{`"open`}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := searchTerms(testCase.query); !slices.Equal(got, testCase.want) {
				t.Errorf("searchTerms(%q) = %q, want %q", testCase.query, got, testCase.want)
			}
		})
	}
}

// TestMatchesInPart_NeedsATermOfThreeCharacters holds the line GitLab draws
// between a term it matches in part and a query it compares whole: three
// characters, counted as characters rather than bytes, in a word or a quoted
// phrase.
func TestMatchesInPart_NeedsATermOfThreeCharacters(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "two characters", query: "p1", want: false},
		{name: "three characters", query: "rev", want: true},
		{name: "short words only", query: "ab cd", want: false},
		{name: "one long word among short ones", query: "ab cde", want: true},
		{name: "two characters of two bytes each", query: "éé", want: false},
		{name: "three characters of two bytes each", query: "ééé", want: true},
		{name: "a quoted phrase of three characters", query: `"a b"`, want: true},
		{name: "a quoted phrase of two characters", query: `"ab"`, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := matchesInPart(testCase.query); got != testCase.want {
				t.Errorf("matchesInPart(%q) = %t, want %t", testCase.query, got, testCase.want)
			}
		})
	}
}

// TestSearchProjects_APIError verifies that [searchProjects] returns an error
// when the GitLab API responds with a failure status.
func TestSearchProjects_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchProjects(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchGroups verifies that [searchGroups] returns formatted group entries
// matching the given query.
func TestSearchGroups(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"full_path":"engineering/platform"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchGroups(context.Background(), client, "eng")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 1 {
		t.Fatalf(fmtExpected1Value, len(values))
	}
	if values[0] != "engineering/platform" {
		t.Errorf("unexpected value: %s", values[0])
	}
}

// TestSearchGroups_APIError verifies that [searchGroups] returns an error when
// the GitLab API responds with a failure status.
func TestSearchGroups_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchGroups(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchUsers verifies that [searchUsers] returns matching usernames.
func TestSearchUsers(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"alice"},{"id":2,"username":"alicia"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchUsers(context.Background(), client, "ali")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "alice" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchUsers_APIError verifies that [searchUsers] returns an error when
// the GitLab API responds with a failure status.
func TestSearchUsers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchUsers(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// iidSearches are the two IID completers, each reading its own listing.
var iidSearches = []struct {
	name   string
	search iidSearchFunc
}{
	{name: "merge requests", search: searchMRs},
	{name: "issues", search: searchIssues},
}

// iidRange spells the IIDs from first to last, inclusive, as a completion
// offers them.
func iidRange(first, last int) []string {
	values := make([]string, 0, last-first+1)
	for iid := first; iid <= last; iid++ {
		values = append(values, itoa(iid))
	}
	return values
}

// iidSearchFunc is the shape of the two IID completers.
type iidSearchFunc func(context.Context, *gitlabclient.Client, string, string) ([]string, int, bool, error)

// iidSearchCase is one IID completion over [iidListing]: the project's size,
// what was typed, and the values, total and whether more may match that the
// answer has to carry.
type iidSearchCase struct {
	name  string
	items int
	typed string
	want  []string
	total int
	more  bool
}

// runIIDSearch drives search over a fresh listing of testCase.items, holds the
// answer to testCase, and returns the query of every request the listing was
// sent, for the caller to hold too.
func runIIDSearch(t *testing.T, search iidSearchFunc, testCase iidSearchCase) []url.Values {
	t.Helper()
	listing := newIIDListing(t, testCase.items)
	client := testutil.NewTestClient(t, listing)

	values, total, more, err := search(context.Background(), client, "42", testCase.typed)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !slices.Equal(values, testCase.want) {
		t.Errorf("values = %v, want %v", values, testCase.want)
	}
	if total != testCase.total {
		t.Errorf("total = %d, want %d", total, testCase.total)
	}
	if more != testCase.more {
		t.Errorf("more = %t, want %t", more, testCase.more)
	}
	return listing.requests()
}

// assertCandidateQuery holds the one request a typed IID sends: every IID what
// was typed begins up to one digit longer, in any state, oldest first.
func assertCandidateQuery(t *testing.T, asked []url.Values) {
	t.Helper()
	if len(asked) != 1 {
		t.Fatalf("%d requests were sent, want one", len(asked))
	}
	query := asked[0]
	if got := len(query["iids[]"]); got != 11 {
		t.Errorf("iids[] named %d IIDs, want 11: what was typed and the ten one digit longer", got)
	}
	if query.Has("state") {
		t.Errorf("state = %q; a closed or merged item is completed too", query.Get("state"))
	}
	if query.Get("order_by") != "created_at" || query.Get("sort") != "asc" {
		t.Errorf("order_by = %q, sort = %q; want the oldest first, so the page holds the shortest matches",
			query.Get("order_by"), query.Get("sort"))
	}
}

// TestSearchIIDs_ATypedNumber_IsAskedForByIID types a number into both IID
// completers over a project holding more items than one page, two in three of
// them closed or merged, and holds the values offered, whether more may match,
// and the one request that was sent.
//
// The request names every IID what was typed begins, up to one digit longer,
// in any state and oldest first. Whether more may match comes from a next
// page, or from an IID one digit longer than what was typed coming back, since
// nothing longer was asked about. No total is reported: GitLab's counts the
// candidates named, not every match.
func TestSearchIIDs_ATypedNumber_IsAskedForByIID(t *testing.T) {
	cases := []iidSearchCase{
		{name: "nothing one digit longer exists", items: 119, typed: "12", want: []string{"12"}},
		{
			name: "an IID one digit longer came back", items: 500, typed: "12",
			want: append([]string{"12"}, iidRange(120, 129)...), more: true,
		},
		{
			name: "one digit typed", items: 1205, typed: "1",
			want: append([]string{"1"}, iidRange(10, 19)...), more: true,
		},
		{name: "nothing matches", items: 9, typed: "12", want: []string{}},
	}
	for _, search := range iidSearches {
		for _, testCase := range cases {
			t.Run(search.name+"/"+testCase.name, func(t *testing.T) {
				assertCandidateQuery(t, runIIDSearch(t, search.search, testCase))
			})
		}
	}
}

// TestSearchIIDs_NothingTyped_OffersTheNewestOfAnyState holds the listing a
// completion starts from: the newest items whatever their state, with the
// total GitLab counted, which for an empty query is every item.
func TestSearchIIDs_NothingTyped_OffersTheNewestOfAnyState(t *testing.T) {
	cases := []iidSearchCase{
		{name: "more than a page", items: 500, want: reversed(iidRange(481, 500)), total: 500, more: true},
		{name: "one page", items: 5, want: reversed(iidRange(1, 5)), total: 5},
	}
	for _, search := range iidSearches {
		for _, testCase := range cases {
			t.Run(search.name+"/"+testCase.name, func(t *testing.T) {
				for _, query := range runIIDSearch(t, search.search, testCase) {
					if query.Has("iids[]") || query.Has("state") || query.Has("sort") {
						t.Errorf("an empty query sent %v; it asks for the newest items, unfiltered", query)
					}
				}
			})
		}
	}
}

// TestSearchIIDs_NoIIDBeginsSo_NothingIsAsked types what no IID can begin with
// and requires an empty answer with no request: an IID is digits and never
// begins with a zero, and one too long for an integer names nothing.
func TestSearchIIDs_NoIIDBeginsSo_NothingIsAsked(t *testing.T) {
	for _, search := range iidSearches {
		for _, typed := range []string{"x", "1x", " 12", "0", "012", "9223372036854775808"} {
			t.Run(search.name+"/"+typed, func(t *testing.T) {
				listing := newIIDListing(t, 500)
				client := testutil.NewTestClient(t, listing)

				values, total, more, err := search.search(context.Background(), client, "42", typed)
				if err != nil {
					t.Fatalf(fmtUnexpectedErr, err)
				}
				if values == nil || len(values) != 0 || total != 0 || more {
					t.Errorf("got values=%v total=%d more=%t, want an empty array and nothing more", values, total, more)
				}
				if asked := listing.requests(); len(asked) != 0 {
					t.Errorf("%d requests were sent for %q, which no IID begins", len(asked), typed)
				}
			})
		}
	}
}

// TestSearchIIDs_ATypedNumberGitLabPages_MoreMayMatch holds a typed IID's
// answer to the next page GitLab names: the eleven IIDs asked about fit on one
// page of twenty, but a page GitLab says goes on is not the whole answer, so
// more may match although nothing one digit longer came back.
func TestSearchIIDs_ATypedNumberGitLabPages_MoreMayMatch(t *testing.T) {
	for _, search := range iidSearches {
		t.Run(search.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1012,"iid":12,"title":"Item","state":"opened"}]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", NextPage: "2"})
			}))

			values, total, more, err := search.search(context.Background(), client, "42", "12")
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if !slices.Equal(values, []string{"12"}) || total != 0 || !more {
				t.Errorf("got values=%v total=%d more=%t, want [12], no total and more", values, total, more)
			}
		})
	}
}

// TestSearchIIDs_TheLongestTypedIIDs_AreAnswered types the longest numbers an
// IID can begin with and requires an answer from the listing, which refuses,
// as GitLab's own nginx does, a query string longer than
// [iidListingQueryLimit]. GitLab repeats the query string in every link of
// its Link header, so the request a completion sends has to stay short
// whatever was typed: naming every IID two digits longer than 17 typed digits
// took it past 3 KB.
func TestSearchIIDs_TheLongestTypedIIDs_AreAnswered(t *testing.T) {
	for _, search := range iidSearches {
		for _, typed := range []string{"10000000000000000", "92233720368547758"} {
			t.Run(search.name+"/"+typed, func(t *testing.T) {
				listing := newIIDListing(t, 500)
				client := testutil.NewTestClient(t, listing)

				values, total, more, err := search.search(context.Background(), client, "42", typed)
				if err != nil {
					t.Fatalf("typing %q: %v", typed, err)
				}
				if len(values) != 0 || total != 0 || more {
					t.Errorf("got values=%v total=%d more=%t, want nothing, since no IID that long exists", values, total, more)
				}
				assertCandidateQuery(t, listing.requests())
			})
		}
	}
}

// TestIIDCandidates_EndAtTheFirstTooLongForAnInt64 holds the candidates to
// what fits in client-go's IID type: they ascend, so the first that does not
// fit ends the list.
func TestIIDCandidates_EndAtTheFirstTooLongForAnInt64(t *testing.T) {
	cases := []struct {
		name  string
		typed string
		want  []int64
	}{
		{name: "nothing typed", typed: "", want: nil},
		{name: "one digit", typed: "7", want: []int64{7, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79}},
		{
			name: "the last ones overflow", typed: "922337203685477580",
			want: []int64{
				922337203685477580, 9223372036854775800, 9223372036854775801, 9223372036854775802,
				9223372036854775803, 9223372036854775804, 9223372036854775805, 9223372036854775806,
				9223372036854775807,
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := iidCandidates(testCase.typed)
			if !slices.Equal(got, testCase.want) {
				t.Errorf("iidCandidates(%q) = %v, want %v", testCase.typed, got, testCase.want)
			}
		})
	}
}

// TestNamesNextPage_ReadsOnlyXNextPage holds the one header that says a
// listing goes on, including the absent response a lister may hand back.
func TestNamesNextPage_ReadsOnlyXNextPage(t *testing.T) {
	cases := []struct {
		name string
		resp *gl.Response
		want bool
	}{
		{name: "no response", resp: nil, want: false},
		{name: "the last page", resp: &gl.Response{TotalItems: 40}, want: false},
		{name: "a next page", resp: &gl.Response{NextPage: 2}, want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := namesNextPage(testCase.resp); got != testCase.want {
				t.Errorf("namesNextPage = %t, want %t", got, testCase.want)
			}
		})
	}
}

// bothHalves is an ID completer with nothing excluded.
var bothHalves = idHalves{lookup: true, list: true}

// idSearchCase is one ID completion over [idListing]: what was typed, the
// halves left, and what the answer and the requests have to be.
type idSearchCase struct {
	name   string
	items  int
	typed  string
	halves idHalves
	want   []string
	more   bool
	asked  []string
}

// runIDSearchCases drives search over each case against a fresh listing and
// holds the values, whether more may match, and every request made, in order.
func runIDSearchCases(t *testing.T, cases []idSearchCase, search func(context.Context, *gitlabclient.Client, string, idHalves) ([]string, bool, error)) {
	t.Helper()
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			listing := newIDListing(t, testCase.items)
			client := testutil.NewTestClient(t, listing)

			values, more, err := search(context.Background(), client, testCase.typed, testCase.halves)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if !slices.Equal(values, testCase.want) {
				t.Errorf("values = %v, want %v", values, testCase.want)
			}
			if more != testCase.more {
				t.Errorf("more = %t, want %t", more, testCase.more)
			}
			if asked := listing.requested(); !slices.Equal(asked, testCase.asked) {
				t.Errorf("requests = %v, want %v", asked, testCase.asked)
			}
		})
	}
}

// TestSearchPipelines_TheTypedPipeline_IsLookedUpBesideTheNewest holds the
// pipeline completer over a project of more pipelines than a page: what was
// typed is looked up whatever its age, the newest whose ID begins with it
// follow, and more may match when the listing names a next page.
func TestSearchPipelines_TheTypedPipeline_IsLookedUpBesideTheNewest(t *testing.T) {
	newest := reversed(iidRange(481, 500))
	runIDSearchCases(t, []idSearchCase{
		{name: "an old pipeline typed whole", items: 500, typed: "7", halves: bothHalves, want: []string{"7"}, more: true, asked: []string{"pipelines/7", "pipelines"}},
		{
			name: "the lookup ahead of the listing", items: 500, typed: "48", halves: bothHalves,
			want: append([]string{"48"}, reversed(iidRange(481, 489))...), more: true, asked: []string{"pipelines/48", "pipelines"},
		},
		{name: "found in both, offered once", items: 500, typed: "490", halves: bothHalves, want: []string{"490"}, more: true, asked: []string{"pipelines/490", "pipelines"}},
		{name: "no such pipeline", items: 500, typed: "9999", halves: bothHalves, want: []string{}, more: true, asked: []string{"pipelines/9999", "pipelines"}},
		{name: "too long to look up", items: 500, typed: "99999999999999999999", halves: bothHalves, want: []string{}, more: true, asked: []string{"pipelines"}},
		{name: "nothing typed", items: 500, typed: "", halves: bothHalves, want: newest, more: true, asked: []string{"pipelines"}},
		{name: "every pipeline on the page", items: 5, typed: "", halves: bothHalves, want: reversed(iidRange(1, 5)), asked: []string{"pipelines"}},
		{name: "nothing an ID begins", items: 500, typed: "x", halves: bothHalves, want: []string{}},
		{name: "a leading zero", items: 500, typed: "07", halves: bothHalves, want: []string{}},
		{name: "the listing excluded", items: 500, typed: "7", halves: idHalves{lookup: true}, want: []string{"7"}, asked: []string{"pipelines/7"}},
		{name: "the lookup excluded", items: 500, typed: "7", halves: idHalves{list: true}, want: []string{}, more: true, asked: []string{"pipelines"}},
	}, func(ctx context.Context, client *gitlabclient.Client, typed string, halves idHalves) ([]string, bool, error) {
		return searchPipelines(ctx, client, "42", typed, halves)
	})
}

// TestSearchCommits_TheTypedSHA_IsLookedUpOnAnyBranch holds the commit
// completer: a SHA of four characters or more is looked up as a commit of the
// project, in either case, beside the default branch's newest commits, and the
// value offered is the short SHA.
func TestSearchCommits_TheTypedSHA_IsLookedUpOnAnyBranch(t *testing.T) {
	runIDSearchCases(t, []idSearchCase{
		{name: "an old commit", items: 500, typed: "0000000c", halves: bothHalves, want: []string{"0000000c"}, more: true, asked: []string{"repository/commits/0000000c", "repository/commits"}},
		{name: "in capitals", items: 500, typed: "0000000C", halves: bothHalves, want: []string{"0000000c"}, more: true, asked: []string{"repository/commits/0000000c", "repository/commits"}},
		{name: "a whole SHA", items: 500, typed: idListingSHA(3), halves: bothHalves, want: []string{"00000003"}, more: true, asked: []string{"repository/commits/" + idListingSHA(3), "repository/commits"}},
		{name: "too short to name a commit", items: 500, typed: "fff", halves: bothHalves, want: []string{}, more: true, asked: []string{"repository/commits"}},
		{
			name: "too short to look up, matched in the listing", items: 500, typed: "000001f", halves: idHalves{list: true},
			want: []string{"000001f4", "000001f3", "000001f2", "000001f1", "000001f0"}, more: true, asked: []string{"repository/commits"},
		},
		{name: "no such commit", items: 500, typed: "ffff", halves: bothHalves, want: []string{}, more: true, asked: []string{"repository/commits/ffff", "repository/commits"}},
		{name: "not hexadecimal", items: 500, typed: "main", halves: bothHalves, want: []string{}},
		{name: "nothing typed", items: 3, typed: "", halves: bothHalves, want: []string{"00000003", "00000002", "00000001"}, asked: []string{"repository/commits"}},
	}, func(ctx context.Context, client *gitlabclient.Client, typed string, halves idHalves) ([]string, bool, error) {
		return searchCommits(ctx, client, "42", typed, halves)
	})
}

// TestSearchCommits_ALookupResolvingElsewhere_IsNotOffered holds the one case
// where a lookup finds something what was typed does not begin: GitLab reads a
// hexadecimal branch or tag name as that ref, and its head commit is no
// completion of the SHA prefix typed.
func TestSearchCommits_ALookupResolvingElsewhere_IsNotOffered(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/repository/commits/beef":
			testutil.RespondJSON(w, http.StatusOK, `{"id":"`+idListingSHA(9)+`","short_id":"00000009","title":"Head of beef"}`)
		case "/api/v4/projects/42/repository/commits":
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	values, more, err := searchCommits(context.Background(), client, "42", "beef", bothHalves)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 0 || more {
		t.Errorf("got values=%v more=%t; the branch named beef has a head commit that does not begin with beef", values, more)
	}
}

// TestSearchCommits_TheLookup_AsksForNoStats holds the commit lookup to
// stats=false. GitLab's single-commit route computes the commit's diff stats
// unless it is told not to, and the completer reads only the SHA and the
// title, so a lookup on every keystroke would have GitLab diff a commit,
// costliest on the largest, for nothing.
func TestSearchCommits_TheLookup_AsksForNoStats(t *testing.T) {
	var stats string
	var looked bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/repository/commits/beef":
			looked = true
			stats = r.URL.Query().Get("stats")
			testutil.RespondJSON(w, http.StatusOK, `{"id":"beef`+strings.Repeat("0", 36)+`","short_id":"beef0000","title":"Beef"}`)
		case "/api/v4/projects/42/repository/commits":
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	if _, _, err := searchCommits(context.Background(), client, "42", "beef", bothHalves); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !looked {
		t.Fatal("the commit typed was not looked up")
	}
	if stats != "false" {
		t.Errorf("the lookup sent stats=%q, want false", stats)
	}
}

// TestSearchJobs_TheTypedJob_IsLookedUpInItsScope holds the job completer in
// both of its scopes: across the project for the job template, which names no
// pipeline, and within one pipeline for a prompt, where a job the lookup finds
// in another pipeline is not offered.
func TestSearchJobs_TheTypedJob_IsLookedUpInItsScope(t *testing.T) {
	inProject := func(ctx context.Context, client *gitlabclient.Client, typed string, halves idHalves) ([]string, bool, error) {
		return searchJobs(ctx, client, "42", 0, typed, halves)
	}
	inPipeline6 := func(ctx context.Context, client *gitlabclient.Client, typed string, halves idHalves) ([]string, bool, error) {
		return searchJobs(ctx, client, "42", 6, typed, halves)
	}
	runIDSearchCases(t, []idSearchCase{
		{name: "an old job", items: 500, typed: "11", halves: bothHalves, want: []string{"11"}, more: true, asked: []string{"jobs/11", "jobs"}},
		{name: "nothing typed", items: 2, typed: "", halves: bothHalves, want: []string{"2", "1"}, asked: []string{"jobs"}},
	}, inProject)
	runIDSearchCases(t, []idSearchCase{
		{name: "a job of the pipeline", items: 500, typed: "12", halves: bothHalves, want: []string{"12"}, asked: []string{"jobs/12", "pipelines/6/jobs"}},
		{name: "a job of another pipeline", items: 500, typed: "3", halves: bothHalves, want: []string{}, asked: []string{"jobs/3", "pipelines/6/jobs"}},
		{name: "every job of the pipeline", items: 500, typed: "1", halves: bothHalves, want: []string{"12", "11"}, asked: []string{"jobs/1", "pipelines/6/jobs"}},
	}, inPipeline6)
}

// TestIDSearch_GitLabFails_IsAnError holds the two failures an ID completer
// reports rather than reads as an answer: a lookup refused for another reason
// than a 404, and a listing that fails.
func TestIDSearch_GitLabFails_IsAnError(t *testing.T) {
	cases := []struct {
		name   string
		failed string
		want   string
	}{
		{name: "the lookup", failed: "/api/v4/projects/42/pipelines/7", want: "look up"},
		{name: "the listing", failed: "/api/v4/projects/42/pipelines", want: "search pipelines"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			listing := newIDListing(t, 500)
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == testCase.failed {
					testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
					return
				}
				listing.ServeHTTP(w, r)
			}))

			values, more, err := searchPipelines(context.Background(), client, "42", "7", bothHalves)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want one naming %q", err, testCase.want)
			}
			if values != nil || more {
				t.Errorf("a failed search answered values=%v more=%t", values, more)
			}
		})
	}
}

// reversed returns values in the opposite order.
func reversed(values []string) []string {
	out := slices.Clone(values)
	slices.Reverse(out)
	return out
}

// TestSearchMRs_APIError verifies that [searchMRs] returns an error when the
// GitLab API responds with a failure status.
func TestSearchMRs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, _, err := searchMRs(context.Background(), client, "42", "1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchIssues_APIError verifies that [searchIssues] returns an error when
// the GitLab API responds with a failure status.
func TestSearchIssues_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, _, err := searchIssues(context.Background(), client, "42", "1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchBranches verifies that [searchBranches] returns branch names
// matching the given query.
func TestSearchBranches(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"name":"main","default":true},
				{"name":"feature/auth","default":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchBranches(context.Background(), client, "42", "main")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values (search is server-side), got %d", len(values))
	}
	if values[0] != "main" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchBranches_EmptyQuery verifies that [searchBranches] omits the search
// parameter when the query is empty.
func TestSearchBranches_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches" {
			if r.URL.Query().Get("search") != "" {
				t.Errorf("expected no search param for empty query")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"main","default":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchBranches(context.Background(), client, "42", "")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 1 {
		t.Fatalf(fmtExpected1Value, len(values))
	}
}

// TestSearchBranches_APIError verifies that [searchBranches] returns an error
// when the GitLab API responds with a failure status.
func TestSearchBranches_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchBranches(context.Background(), client, "42", "main")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchTags verifies that [searchTags] returns tag names matching the
// given query.
func TestSearchTags(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/tags" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"name":"v1.0.0"},
				{"name":"v1.1.0"},
				{"name":"v2.0.0"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchTags(context.Background(), client, "42", "v1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values (search is server-side), got %d", len(values))
	}
}

// TestSearchTags_APIError verifies that [searchTags] returns an error when the
// GitLab API responds with a failure status.
func TestSearchTags_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchTags(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearch_ContextCancelled uses table-driven subtests to verify that all
// search functions return a context cancellation error when given a canceled
// context.
func TestSearch_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"projects", func() error { _, _, err := searchProjects(ctx, client, "x"); return err }},
		{"groups", func() error { _, _, err := searchGroups(ctx, client, "x"); return err }},
		{"users", func() error { _, _, err := searchUsers(ctx, client, "x"); return err }},
		{"mrs", func() error { _, _, _, err := searchMRs(ctx, client, "42", "x"); return err }},
		{"issues", func() error { _, _, _, err := searchIssues(ctx, client, "42", "x"); return err }},
		{"branches", func() error { _, _, err := searchBranches(ctx, client, "42", "x"); return err }},
		{"tags", func() error { _, _, err := searchTags(ctx, client, "42", "x"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Error("expected context cancellation error")
			} else if !strings.Contains(err.Error(), "context canceled") {
				t.Errorf("expected context canceled error, got: %v", err)
			}
		})
	}
}

// TestSearchPipelines verifies that [searchPipelines] returns pipeline entries
// filtered by ID prefix.
func TestSearchPipelines(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":100,"ref":"main","status":"success"},
				{"id":101,"ref":"develop","status":"running"},
				{"id":23,"ref":"feature","status":"failed"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by ID prefix", func(t *testing.T) {
		values, _, err := searchPipelines(context.Background(), client, "42", "10", bothHalves)
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching prefix '10', got %d: %v", len(values), values)
		}
		if values[0] != "100" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run(subtestEmptyQueryAll, func(t *testing.T) {
		values, _, err := searchPipelines(context.Background(), client, "42", "", bothHalves)
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchPipelines_APIError verifies that [searchPipelines] returns an error
// when the GitLab API responds with a failure status.
func TestSearchPipelines_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchPipelines(context.Background(), client, "42", "1", bothHalves)
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchCommits verifies that [searchCommits] returns commit entries
// filtered by SHA prefix.
func TestSearchCommits(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/commits" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":"abc123def456","short_id":"abc123d","title":"Fix login bug"},
				{"id":"def789abc012","short_id":"def789a","title":"Add tests"},
				{"id":"abc999aaa111","short_id":"abc999a","title":"Update docs"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by short SHA prefix", func(t *testing.T) {
		values, _, err := searchCommits(context.Background(), client, "42", "abc", bothHalves)
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching prefix 'abc', got %d: %v", len(values), values)
		}
		if values[0] != "abc123d" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	// A pasted SHA is longer than the seven characters GitLab abbreviates to,
	// so matching only the abbreviation would drop the commit the caller has
	// in their clipboard. The value offered stays the short SHA either way,
	// since that is what the argument takes.
	t.Run("a prefix longer than the short SHA still finds the commit", func(t *testing.T) {
		values, _, err := searchCommits(context.Background(), client, "42", "abc123def4", bothHalves)
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 1 {
			t.Fatalf("expected 1 value matching the full SHA prefix, got %d: %v", len(values), values)
		}
		if values[0] != "abc123d" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run(subtestEmptyQueryAll, func(t *testing.T) {
		values, _, err := searchCommits(context.Background(), client, "42", "", bothHalves)
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchCommits_APIError verifies that [searchCommits] returns an error
// when the GitLab API responds with a failure status.
func TestSearchCommits_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchCommits(context.Background(), client, "42", "abc", bothHalves)
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchLabels verifies that [searchLabels] returns label names matching
// the query.
func TestSearchLabels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/labels" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"name":"bug","color":"#d9534f"},
				{"id":2,"name":"enhancement","color":"#5cb85c"},
				{"id":3,"name":"documentation","color":"#0275d8"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("search with query", func(t *testing.T) {
		values, _, err := searchLabels(context.Background(), client, "42", "bug")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		// Server-side search: mock returns all regardless, so we get 3
		if len(values) != 3 {
			t.Fatalf("expected 3 values (server-side search), got %d: %v", len(values), values)
		}
		if values[0] != "bug" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run("empty query", func(t *testing.T) {
		values, _, err := searchLabels(context.Background(), client, "42", "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchLabels_APIError verifies that [searchLabels] returns an error when
// the GitLab API responds with a failure status.
func TestSearchLabels_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchLabels(context.Background(), client, "42", "bug")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchMilestones verifies that [searchMilestones] returns milestone
// entries matching the query.
func TestSearchMilestones(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"v1.0","state":"active"},
				{"id":2,"title":"v2.0","state":"active"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchMilestones(context.Background(), client, "42", "v1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values (server-side search), got %d: %v", len(values), values)
	}
	if values[0] != "1" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchMilestones_APIError verifies that [searchMilestones] returns an
// error when the GitLab API responds with a failure status.
func TestSearchMilestones_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchMilestones(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchJobs verifies that [searchJobs] returns job entries for a pipeline,
// filtered by ID prefix.
func TestSearchJobs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines/10/jobs" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":501,"name":"build","status":"success","pipeline":{"id":10}},
				{"id":502,"name":"test","status":"running","pipeline":{"id":10}},
				{"id":601,"name":"deploy","status":"pending","pipeline":{"id":10}}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by ID prefix", func(t *testing.T) {
		values, _, err := searchJobs(context.Background(), client, "42", 10, "50", bothHalves)
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching prefix '50', got %d: %v", len(values), values)
		}
		if values[0] != "501" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run(subtestEmptyQueryAll, func(t *testing.T) {
		values, _, err := searchJobs(context.Background(), client, "42", 10, "", bothHalves)
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchJobs_APIError verifies that [searchJobs] returns an error when the
// GitLab API responds with a failure status.
func TestSearchJobs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchJobs(context.Background(), client, "42", 10, "1", bothHalves)
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchNew_ContextCancelled uses table-driven subtests to verify that
// the new search functions return a context cancellation error.
func TestSearchNew_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"pipelines", func() error { _, _, err := searchPipelines(ctx, client, "42", "x", bothHalves); return err }},
		{"commits", func() error { _, _, err := searchCommits(ctx, client, "42", "x", bothHalves); return err }},
		{"labels", func() error { _, _, err := searchLabels(ctx, client, "42", "x"); return err }},
		{"milestones", func() error { _, _, err := searchMilestones(ctx, client, "42", "x"); return err }},
		{"jobs", func() error { _, _, err := searchJobs(ctx, client, "42", 10, "x", bothHalves); return err }},
		{"milestone titles", func() error { _, _, err := searchMilestoneTitles(ctx, client, "42", "x"); return err }},
		{"group milestone titles", func() error { _, _, err := searchGroupMilestoneTitles(ctx, client, "99", "x"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Error("expected context cancellation error")
			} else if !strings.Contains(err.Error(), "context canceled") {
				t.Errorf("expected context canceled error, got: %v", err)
			}
		})
	}
}

// TestSearchMilestoneTitles verifies that [searchMilestoneTitles] returns
// plain milestone titles (not "id: title") and that the query parameter is
// forwarded to GitLab as the Search filter. The mock asserts the request URL
// includes ?search=v1, then returns a single milestone whose title is read
// from the response body.
func TestSearchMilestoneTitles(t *testing.T) {
	var gotSearch string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/milestones" {
			http.NotFound(w, r)
			return
		}
		gotSearch = r.URL.Query().Get("search")
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"title":"v1.0","state":"active"},
			{"id":2,"title":"v1.1","state":"active"}
		]`)
	}))

	values, _, err := searchMilestoneTitles(context.Background(), client, "42", "v1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if gotSearch != "v1" {
		t.Errorf("query param 'search' = %q, want %q (query branch not exercised)", gotSearch, "v1")
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 titles, got %d: %v", len(values), values)
	}
	if values[0] != "v1.0" || values[1] != "v1.1" {
		t.Errorf("titles = %v, want [v1.0 v1.1]", values)
	}
}

// TestSearchMilestoneTitles_EmptyQuery verifies that when query is empty,
// the Search option is NOT set on the request (covers the false branch of
// `if query != ""`).
func TestSearchMilestoneTitles_EmptyQuery(t *testing.T) {
	var hadSearch bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/milestones" {
			http.NotFound(w, r)
			return
		}
		_, hadSearch = r.URL.Query()["search"]
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, _, err := searchMilestoneTitles(context.Background(), client, "42", "")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if hadSearch {
		t.Error("expected no 'search' query param when query is empty")
	}
}

// TestSearchMilestoneTitles_APIError verifies that an error from the GitLab
// API is wrapped and returned. Uses 403 (not 5xx) to avoid client-go retries.
func TestSearchMilestoneTitles_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, _, err := searchMilestoneTitles(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
	if !strings.Contains(err.Error(), "search milestone titles") {
		t.Errorf("expected error to be wrapped with 'search milestone titles', got: %v", err)
	}
}

// TestSearchGroupMilestoneTitles verifies group milestone title search with
// and without a query filter.
func TestSearchGroupMilestoneTitles(t *testing.T) {
	t.Run("search with query", func(t *testing.T) {
		var gotSearch string
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v4/groups/99/milestones" {
				http.NotFound(w, r)
				return
			}
			gotSearch = r.URL.Query().Get("search")
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"v1.0","state":"active"},
				{"id":2,"title":"v1.1","state":"active"}
			]`)
		}))

		values, _, err := searchGroupMilestoneTitles(context.Background(), client, "99", "v1")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if gotSearch != "v1" {
			t.Errorf("query param 'search' = %q, want %q", gotSearch, "v1")
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 titles, got %d: %v", len(values), values)
		}
		if values[0] != "v1.0" || values[1] != "v1.1" {
			t.Errorf("titles = %v, want [v1.0 v1.1]", values)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		var hadSearch bool
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v4/groups/99/milestones" {
				http.NotFound(w, r)
				return
			}
			_, hadSearch = r.URL.Query()["search"]
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		}))

		_, _, err := searchGroupMilestoneTitles(context.Background(), client, "99", "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if hadSearch {
			t.Error("expected no 'search' query param when query is empty")
		}
	})
}

// TestSearchGroupMilestoneTitles_APIError verifies group milestone API errors
// are wrapped with search context.
func TestSearchGroupMilestoneTitles_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, _, err := searchGroupMilestoneTitles(context.Background(), client, "99", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
	if !strings.Contains(err.Error(), "search group milestone titles") {
		t.Errorf("expected error to be wrapped with 'search group milestone titles', got: %v", err)
	}
}

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
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			})
			client := testutil.NewTestClient(t, handler)
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

// searchParamSeen drives one search against a mock answering body at path and
// returns the `search` values GitLab was sent, plus whether the parameter was
// there at all. An absent parameter and an empty one are different requests,
// so the two are reported apart rather than collapsed into one string.
func searchParamSeen(t *testing.T, path, body string, search func(*testClientType, string) error, query string) (value string, present bool) {
	t.Helper()
	var values []string
	var reached bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		values, reached = r.URL.Query()["search"], true
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	if err := search(client, query); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !reached {
		t.Fatalf("no request reached %s, so nothing was measured", path)
	}
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

// TestSearch_SendsWhatWasTypedAsTheSearchParameter pins the half of the query
// branch that nothing asserted: that the partial value reaches GitLab.
//
// Six of these searches hand the query to GitLab as `search=`, and the tests
// around them only ever checked that an *empty* query sends no parameter. That
// pair cannot fail: a search that stopped setting the option altogether still
// sends no parameter when nothing was typed, and no test read the parameter
// back in the case that matters. What a caller would get is every project,
// group, user, branch, tag or label the token can see, in GitLab's own order,
// with what they typed ignored — an autocomplete that looks alive and narrows
// nothing.
//
// Both states are read off the wire here, so the claim is about the request
// this server built rather than about the fixture the mock happened to return.
func TestSearch_SendsWhatWasTypedAsTheSearchParameter(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		body   string
		search func(client *testClientType, query string) error
	}{
		{
			name: "projects", path: "/api/v4/projects", body: `[{"id":1,"path_with_namespace":"a/b"}]`,
			search: func(c *testClientType, q string) error {
				_, _, err := searchProjects(context.Background(), c, q)
				return err
			},
		},
		{
			name: "groups", path: "/api/v4/groups", body: `[{"id":1,"full_path":"a"}]`,
			search: func(c *testClientType, q string) error {
				_, _, err := searchGroups(context.Background(), c, q)
				return err
			},
		},
		{
			name: "users", path: "/api/v4/users", body: `[{"id":1,"username":"alpha"}]`,
			search: func(c *testClientType, q string) error {
				_, _, err := searchUsers(context.Background(), c, q)
				return err
			},
		},
		{
			name: "branches", path: pathRepoBranches, body: `[{"name":"alpha"}]`,
			search: func(c *testClientType, q string) error {
				_, _, err := searchBranches(context.Background(), c, "42", q)
				return err
			},
		},
		{
			name: "tags", path: pathRepoTags, body: `[{"name":"alpha"}]`,
			search: func(c *testClientType, q string) error {
				_, _, err := searchTags(context.Background(), c, "42", q)
				return err
			},
		},
		{
			name: "labels", path: "/api/v4/projects/42/labels", body: `[{"name":"alpha"}]`,
			search: func(c *testClientType, q string) error {
				_, _, err := searchLabels(context.Background(), c, "42", q)
				return err
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Run("what was typed is what GitLab is asked for", func(t *testing.T) {
				value, present := searchParamSeen(t, testCase.path, testCase.body, testCase.search, "alpha")
				if !present {
					t.Fatal("GitLab was sent no search parameter, so it answered with everything the token can see")
				}
				if value != "alpha" {
					t.Errorf("search = %q, want %q", value, "alpha")
				}
			})

			t.Run("nothing typed asks for no narrowing", func(t *testing.T) {
				_, present := searchParamSeen(t, testCase.path, testCase.body, testCase.search, "")
				if present {
					t.Error("GitLab was sent a search parameter although nothing was typed")
				}
			})
		})
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
