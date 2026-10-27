// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package testutil

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
)

// keysetNextLinkHost is the host the next link of a [KeysetListing] names.
// It is not the mock's address on purpose: GitLab writes the link with its
// own configured host, which need not be the one the client reached it on,
// and a continuation is applied by its query alone.
const keysetNextLinkHost = "https://gitlab.example.com"

// KeysetListing mocks one list GitLab serves by keyset pagination, two pages
// long. The first page, asked for with no position, answers First with a Link
// header naming the next page by Param=Value and none of the offset headers,
// which is how GitLab answers every keyset page that has a successor. The
// second, asked for with that position, answers Second with no Link, which is
// how the last page reads. A request for another path is answered 404 and one
// carrying another position 400, so a handler that loses the continuation
// fails rather than reading the first page again.
//
// Every request's query is recorded, so a test can read what the handler
// sent after the fact instead of asserting off the test goroutine.
type KeysetListing struct {
	Path   string // request path, such as /api/v4/projects
	Param  string // the parameter the endpoint pages by: id_after, cursor, page_token or last
	Value  string // the position the first page's link names
	First  string // JSON array the first page answers with
	Second string // JSON array the second page answers with

	// Arrays names the parameters of the listing GitLab declares as an Array
	// of strings, such as labels, not[labels] or topic. client-go sends one
	// comma-separated under its bare name, GitLab writes it into the next
	// link in Rack's bracket form (labels[]=a&labels[]=b), and Rack refuses
	// with 400 a query that carries both forms of one, before the API runs.
	Arrays []string

	mu      sync.Mutex
	queries []url.Values
}

// NextLink returns the link the first page names in its Link header, written
// the way GitLab writes one: every parameter of the first request as GitLab
// read it, an [KeysetListing.Arrays] parameter in its bracket form, with the
// listing's position beside them. Before any request it carries the position
// alone.
func (k *KeysetListing) NextLink() string {
	return keysetNextLinkHost + k.Path + "?" + k.nextQuery().Encode()
}

// nextQuery returns the query of [KeysetListing.NextLink].
func (k *KeysetListing) nextQuery() url.Values {
	query := url.Values{}
	k.mu.Lock()
	if len(k.queries) > 0 {
		for key, values := range k.queries[0] {
			query[key] = append([]string(nil), values...)
		}
	}
	k.mu.Unlock()
	for _, name := range k.Arrays {
		for _, value := range query[name] {
			query[name+"[]"] = append(query[name+"[]"], strings.Split(value, ",")...)
		}
		delete(query, name)
	}
	query.Set(k.Param, k.Value)
	return query
}

// ServeHTTP serves the listing and records the query of each request.
//
// Separate ifs rather than a tagless switch: a case expression carries no
// statement counter, so the mutation gate reports every mutant of one as not
// covered whatever the tests assert.
func (k *KeysetListing) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	k.mu.Lock()
	k.queries = append(k.queries, query)
	k.mu.Unlock()

	if r.URL.Path != k.Path {
		http.NotFound(w, r)
		return
	}
	for _, name := range k.Arrays {
		if query.Has(name) && query.Has(name+"[]") {
			// GitLab's HandleMalformedStrings answers this way when Rack
			// cannot parse the query, in plain text and before any route.
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
	}
	if !query.Has(k.Param) {
		w.Header().Set("Link", "<"+k.NextLink()+`>; rel="next"`)
		RespondJSON(w, http.StatusOK, k.First)
		return
	}
	if query.Get(k.Param) == k.Value {
		RespondJSON(w, http.StatusOK, k.Second)
		return
	}
	http.Error(w, "unknown keyset position "+query.Get(k.Param), http.StatusBadRequest)
}

// Queries returns the query of every request served so far, in order.
func (k *KeysetListing) Queries() []url.Values {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]url.Values(nil), k.queries...)
}

// KeysetPage is what one call of a keyset listing reported: the continuation
// its pagination block published, whether it said more follow, and how many
// rows it carried.
type KeysetPage struct {
	Token   string
	HasMore bool
	Rows    int
}

// KeysetReporter is the part of [testing.T] [FollowKeyset] reports through,
// an interface so its failure branches can be tested with a recorder.
type KeysetReporter interface {
	// Helper marks the caller as a helper, as [testing.T.Helper] does.
	Helper()
	// Errorf reports a failure and lets the test go on, as [testing.T.Errorf]
	// does.
	Errorf(format string, args ...any)
}

// FollowKeyset reads a [KeysetListing] to its end through call, the way a
// caller follows a keyset list: once with no page_token, then once with the
// token the first call published. It reports through r unless the first page
// publishes the listing's next link and says more follow, the second request
// carries exactly the query of that link, the first request asks for keyset
// pagination, the second page publishes no token and says nothing follows,
// and each page carries the rows its body holds.
//
// The second request is held to the whole of the link because GitLab writes
// every parameter it read into it: one sent beside the token that the link
// does not carry is a second form of a parameter it does, which Rack refuses.
//
// call is the handler under test, invoked with the page_token to send and
// returning what its output reported.
func FollowKeyset(r KeysetReporter, listing *KeysetListing, call func(pageToken string) (KeysetPage, error)) {
	r.Helper()
	first, err := call("")
	if err != nil {
		r.Errorf("first page: %v", err)
		return
	}
	if want := (KeysetPage{Token: listing.NextLink(), HasMore: true, Rows: jsonArrayLength(listing.First)}); first != want {
		r.Errorf("first page reported %+v, want %+v", first, want)
		return
	}
	second, err := call(first.Token)
	if err != nil {
		r.Errorf("second page: %v", err)
		return
	}
	if want := (KeysetPage{Rows: jsonArrayLength(listing.Second)}); second != want {
		r.Errorf("second page reported %+v, want %+v", second, want)
	}
	queries := listing.Queries()
	if len(queries) != 2 || !maps.EqualFunc(queries[1], listing.nextQuery(), slices.Equal) {
		r.Errorf("requests %v, want two, the second carrying exactly the query of the next link %s", queries, first.Token)
		return
	}
	// The second request is the link, which carries what the first sent, so
	// the first is the one that decides how the listing was asked for.
	if got := queries[0].Get("pagination"); got != "keyset" {
		r.Errorf("first request asked for pagination %q, want keyset: %v", got, queries[0])
	}
}

// jsonArrayLength returns the number of elements of a JSON array, or -1 when
// body is not one, which no page can report.
func jsonArrayLength(body string) int {
	var rows []json.RawMessage
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		return -1
	}
	return len(rows)
}
