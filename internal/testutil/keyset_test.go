// keyset_test.go holds the keyset listing mock and its driver to what they
// promise: the mock answers each page the way GitLab answers a keyset page and
// records what it was asked, and [FollowKeyset] passes a handler that follows
// the listing and reports each way one can fail to.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// keysetFixture returns a two-page listing of projects paged by id_after.
func keysetFixture() *KeysetListing {
	return &KeysetListing{
		Path:   "/api/v4/projects",
		Param:  "id_after",
		Value:  "2",
		First:  `[{"id":1},{"id":2}]`,
		Second: `[{"id":3}]`,
	}
}

// keysetReporterRecorder records what [FollowKeyset] reports, so a failure
// branch can be read without failing the test that drives it.
type keysetReporterRecorder struct {
	errors []string
}

// Helper satisfies [KeysetReporter].
func (r *keysetReporterRecorder) Helper() {}

// Errorf records the formatted failure.
func (r *keysetReporterRecorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

// get sends a GET for target to the listing and returns what it wrote.
func get(t *testing.T, listing *KeysetListing, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	listing.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return w
}

// TestKeysetListing_ServeHTTP_AnswersEachPageTheWayGitLabDoes verifies the
// four answers of the mock: the first page with the Link GitLab writes, every
// parameter of the request beside the position, and no offset header, the
// second page with no Link, 400 for a position the listing never named and
// 404 for another path, and the record of every query.
func TestKeysetListing_ServeHTTP_AnswersEachPageTheWayGitLabDoes(t *testing.T) {
	listing := keysetFixture()
	if got, want := listing.NextLink(), "https://gitlab.example.com/api/v4/projects?id_after=2"; got != want {
		t.Fatalf("NextLink() before any request = %q, want the position alone, %q", got, want)
	}

	wantLink := "https://gitlab.example.com/api/v4/projects?id_after=2&order_by=id&pagination=keyset&per_page=2"
	first := get(t, listing, "/api/v4/projects?order_by=id&pagination=keyset&per_page=2")
	if first.Code != http.StatusOK {
		t.Errorf("first page status = %d, want 200", first.Code)
	}
	if got := first.Header().Get("Link"); got != "<"+wantLink+`>; rel="next"` {
		t.Errorf("first page Link = %q, want the next link", got)
	}
	if got := first.Header().Get("X-Next-Page"); got != "" {
		t.Errorf("first page X-Next-Page = %q, want none under keyset", got)
	}
	if body := first.Body.String(); body != listing.First {
		t.Errorf("first page body = %s, want %s", body, listing.First)
	}

	second := get(t, listing, "/api/v4/projects?id_after=2&pagination=keyset")
	if got := second.Header().Get("Link"); got != "" {
		t.Errorf("second page Link = %q, want none on the last page", got)
	}
	if body := second.Body.String(); body != listing.Second {
		t.Errorf("second page body = %s, want %s", body, listing.Second)
	}

	if got := get(t, listing, "/api/v4/projects?id_after=9").Code; got != http.StatusBadRequest {
		t.Errorf("unknown position status = %d, want 400", got)
	}
	if got := get(t, listing, "/api/v4/groups").Code; got != http.StatusNotFound {
		t.Errorf("other path status = %d, want 404", got)
	}

	queries := listing.Queries()
	if len(queries) != 4 || queries[1].Get("id_after") != "2" || queries[2].Get("id_after") != "9" {
		t.Errorf("Queries() = %v, want the four queries in order", queries)
	}
}

// TestKeysetListing_Arrays_WritesTheBracketFormAndRefusesBoth verifies the
// mock's model of a parameter GitLab declares as an Array: the next link
// writes the comma list client-go sent in Rack's bracket form, one value per
// element, and a request that carries both forms of it is refused with 400
// before the listing is consulted, as Rack refuses to parse such a query.
// A parameter sent only in its bracket form is the link followed, and served.
func TestKeysetListing_Arrays_WritesTheBracketFormAndRefusesBoth(t *testing.T) {
	listing := keysetFixture()
	listing.Path = "/api/v4/projects/42/issues"
	listing.Param = "cursor"
	listing.Arrays = []string{"labels", "not[labels]"}

	first := get(t, listing, "/api/v4/projects/42/issues?labels=bug%2Cx&not%5Blabels%5D=wontfix&pagination=keyset")
	wantLink := "https://gitlab.example.com/api/v4/projects/42/issues?cursor=2&labels%5B%5D=bug&labels%5B%5D=x&not%5Blabels%5D%5B%5D=wontfix&pagination=keyset"
	if got := first.Header().Get("Link"); got != "<"+wantLink+`>; rel="next"` {
		t.Errorf("first page Link = %q, want %q", got, wantLink)
	}

	both := get(t, listing, "/api/v4/projects/42/issues?cursor=2&labels=bug%2Cx&labels%5B%5D=bug&labels%5B%5D=x&pagination=keyset")
	if both.Code != http.StatusBadRequest || strings.TrimSpace(both.Body.String()) != "Bad Request" {
		t.Errorf("both forms answered %d %q, want 400 Bad Request", both.Code, both.Body.String())
	}
	notBoth := get(t, listing, "/api/v4/projects/42/issues?cursor=2&not%5Blabels%5D=wontfix&not%5Blabels%5D%5B%5D=wontfix&pagination=keyset")
	if notBoth.Code != http.StatusBadRequest {
		t.Errorf("both forms of not[labels] answered %d, want 400", notBoth.Code)
	}

	link, err := url.Parse(wantLink)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	second := get(t, listing, "/api/v4/projects/42/issues?"+link.RawQuery)
	if second.Code != http.StatusOK || second.Body.String() != listing.Second {
		t.Errorf("the link followed answered %d %s, want 200 %s", second.Code, second.Body.String(), listing.Second)
	}
}

// TestFollowKeyset_Cases_ReportsEachWayAListingIsLost verifies the driver
// passes a handler that follows the listing and reports, once, each way a
// handler can fail to: an error on either page, a first page that publishes
// no continuation or the wrong rows, a second page that still names a
// successor, a second request that dropped the position or sent something
// beside the link, and a listing that never asked for keyset pagination.
func TestFollowKeyset_Cases_ReportsEachWayAListingIsLost(t *testing.T) {
	failure := errors.New("boom")
	cases := []struct {
		name string
		// call is the handler under test: it may ask the listing for a page
		// and reports what its output carried.
		call func(listing *KeysetListing, token string) (KeysetPage, error)
		want string
	}{
		{
			name: "a handler that follows the listing passes",
			call: followingHandler,
		},
		{
			name: "an error on the first page",
			call: func(*KeysetListing, string) (KeysetPage, error) { return KeysetPage{}, failure },
			want: "first page: boom",
		},
		{
			name: "a first page with no continuation",
			call: func(listing *KeysetListing, token string) (KeysetPage, error) {
				page, err := followingHandler(listing, token)
				page.Token = ""
				return page, err
			},
			want: "first page reported",
		},
		{
			name: "an error on the second page",
			call: func(listing *KeysetListing, token string) (KeysetPage, error) {
				if token != "" {
					return KeysetPage{}, failure
				}
				return followingHandler(listing, token)
			},
			want: "second page: boom",
		},
		{
			name: "a second page that still names a successor",
			call: func(listing *KeysetListing, token string) (KeysetPage, error) {
				page, err := followingHandler(listing, token)
				page.HasMore = true
				return page, err
			},
			want: "second page reported",
		},
		{
			name: "a second request that dropped the position",
			call: func(listing *KeysetListing, token string) (KeysetPage, error) {
				if token != "" {
					ask(listing, "/api/v4/projects?pagination=keyset")
					return KeysetPage{Rows: 1}, nil
				}
				return followingHandler(listing, token)
			},
			want: "want two, the second carrying exactly the query of the next link",
		},
		// A filter the caller sends again beside the token is a second form
		// of one the link already carries whenever GitLab declares it an
		// Array, which Rack refuses, so the second request is the link alone.
		{
			name: "a second request that sends a parameter beside the link",
			call: func(listing *KeysetListing, token string) (KeysetPage, error) {
				if token != "" {
					next, err := url.Parse(token)
					if err != nil {
						return KeysetPage{}, err
					}
					ask(listing, listing.Path+"?"+next.RawQuery+"&search=api")
					return KeysetPage{Rows: 1}, nil
				}
				return followingHandler(listing, token)
			},
			want: "want two, the second carrying exactly the query of the next link",
		},
		{
			name: "a handler that asks for a page more than once",
			call: func(listing *KeysetListing, token string) (KeysetPage, error) {
				if token == "" {
					ask(listing, "/api/v4/projects?pagination=keyset")
				}
				return followingHandler(listing, token)
			},
			want: "want two, the second carrying exactly the query of the next link",
		},
		{
			name: "a listing that never asked for keyset pagination",
			call: func(listing *KeysetListing, token string) (KeysetPage, error) {
				if token == "" {
					ask(listing, listing.Path)
					return KeysetPage{Token: listing.NextLink(), HasMore: true, Rows: 2}, nil
				}
				return followingHandler(listing, token)
			},
			want: `first request asked for pagination ""`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listing := keysetFixture()
			var r keysetReporterRecorder
			FollowKeyset(&r, listing, func(token string) (KeysetPage, error) { return tc.call(listing, token) })
			if tc.want == "" {
				if len(r.errors) != 0 {
					t.Errorf("FollowKeyset reported %q, want nothing", r.errors)
				}
				return
			}
			if len(r.errors) != 1 || !strings.Contains(r.errors[0], tc.want) {
				t.Errorf("FollowKeyset reported %q, want one report naming %q", r.errors, tc.want)
			}
		})
	}
}

// TestJSONArrayLength_NotAnArray_IsNoCount verifies that a body that is not a
// JSON array counts as -1, which no page reports, so a listing written with a
// malformed body fails the driver rather than matching an empty page.
func TestJSONArrayLength_NotAnArray_IsNoCount(t *testing.T) {
	if got := jsonArrayLength(`{"id":1}`); got != -1 {
		t.Errorf("jsonArrayLength(object) = %d, want -1", got)
	}
	if got := jsonArrayLength(`[]`); got != 0 {
		t.Errorf("jsonArrayLength([]) = %d, want 0", got)
	}
}

// ask sends target to the listing the way a handler would, discarding the
// answer.
func ask(listing *KeysetListing, target string) {
	listing.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil))
}

// followingHandler is a handler that follows the listing correctly: it asks
// for the first page with no position and for the next one by the query of
// the link it was handed, and reports what a correct pagination block says.
func followingHandler(listing *KeysetListing, token string) (KeysetPage, error) {
	if token == "" {
		ask(listing, listing.Path+"?pagination=keyset")
		return KeysetPage{Token: listing.NextLink(), HasMore: true, Rows: 2}, nil
	}
	next, err := url.Parse(token)
	if err != nil {
		return KeysetPage{}, err
	}
	ask(listing, listing.Path+"?"+next.RawQuery)
	return KeysetPage{Rows: 1}, nil
}
