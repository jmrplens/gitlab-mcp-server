package apidocs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// treePage is the shape one page of the repository tree listing comes back as.
type treePage []treeEntry

// newTreeServer serves the given pages of a tree listing, one per ?page=, and
// counts how many requests reached it. A page number past the end is served as
// an empty list, which is what the real endpoint does.
func newTreeServer(t *testing.T, pages []treePage) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		body := treePage{}
		if page >= 1 && page <= len(pages) {
			body = pages[page-1]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// fullPage returns a page of exactly areasPerPage markdown blobs, so the
// pagination loop is driven by a full page rather than by a short one.
func fullPage(prefix string) treePage {
	page := make(treePage, 0, areasPerPage)
	for i := range areasPerPage {
		page = append(page, treeEntry{Path: areasPrefix + prefix + strconv.Itoa(i) + ".md", Type: "blob"})
	}
	return page
}

// newAreasFetcher returns a fetcher pointed at srv with its cache isolated to
// dir.
func newAreasFetcher(dir, areasURL string, opts Options) *Fetcher {
	opts.AreasURL = areasURL
	f := New(dir, opts)
	f.cacheDir = dir
	return f
}

// TestAreas_ListsEveryMarkdownPageUnderDocAPI verifies what an area is: a
// markdown blob below doc/api, named the way Fetch takes it. The entries that
// are not, a directory and a file of another kind, are what a listing actually
// carries and must not become areas nobody can fetch.
func TestAreas_ListsEveryMarkdownPageUnderDocAPI(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newTreeServer(t, []treePage{{
		{Path: "doc/api/packages", Type: "tree"},
		{Path: "doc/api/branches.md", Type: "blob"},
		{Path: "doc/api/packages/npm.md", Type: "blob"},
		{Path: "doc/api/img/diagram.png", Type: "blob"},
		{Path: "doc/development/rake_tasks.md", Type: "blob"},
		{Path: "doc/api/.md", Type: "blob"},
	}})
	f := newAreasFetcher(dir, srv.URL, Options{})

	areas, err := f.Areas(context.Background())
	if err != nil {
		t.Fatalf("Areas() error = %v, want nil", err)
	}
	if strings.Join(areas, ",") != "branches,packages/npm" {
		t.Errorf("Areas() = %v, want the two markdown pages under doc/api in sorted order", areas)
	}
	t.Run("the listing is cached", func(t *testing.T) {
		before := atomic.LoadInt32(hits)
		if _, secondErr := f.Areas(context.Background()); secondErr != nil {
			t.Fatalf("Areas() error = %v on the second call", secondErr)
		}
		if atomic.LoadInt32(hits) != before {
			t.Errorf("Areas() asked the network again after caching (%d requests, then %d)", before, atomic.LoadInt32(hits))
		}
	})
}

// TestAreas_AFullPage_IsFollowedByTheNext verifies the pagination, since the
// listing is three pages long and a run that read only the first would report
// two thirds of GitLab's documentation as missing.
func TestAreas_AFullPage_IsFollowedByTheNext(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newTreeServer(t, []treePage{
		fullPage("first"),
		{{Path: areasPrefix + "last.md", Type: "blob"}},
	})
	f := newAreasFetcher(dir, srv.URL, Options{})

	areas, err := f.Areas(context.Background())
	if err != nil {
		t.Fatalf("Areas() error = %v, want nil", err)
	}
	if len(areas) != areasPerPage+1 {
		t.Fatalf("Areas() returned %d area(s), want %d: the second page was not read", len(areas), areasPerPage+1)
	}
	if areas[len(areas)-1] != "last" {
		t.Errorf("Areas() ended with %q, want the entry from the second page", areas[len(areas)-1])
	}
}

// TestAreas_TheQueryTheCallerWrote_IsKept verifies that the paging parameters
// are added to the listing URL rather than replacing what is already there: the
// real URL carries the path and the recursive flag, and losing either would
// list the whole repository or only its top level.
func TestAreas_TheQueryTheCallerWrote_IsKept(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		{name: "a URL that already has a query", base: "https://gitlab.example.com/tree?path=doc/api", want: "path=doc/api&page=2&per_page=100"},
		{name: "a URL with no query", base: "https://gitlab.example.com/tree", want: "page=2&per_page=100"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := pageURL(testCase.base, 2)

			parsed := strings.SplitN(got, "?", 2)
			if len(parsed) != 2 {
				t.Fatalf("pageURL() = %q, want a query on it", got)
			}
			if parsed[1] != testCase.want {
				t.Errorf("pageURL() query = %q, want %q", parsed[1], testCase.want)
			}
		})
	}
}

// TestAreas_OfflineWithACachedListing_ServesItAtAnyAge verifies the mode the
// audit runs in when it must not touch the network, and the refusal when there
// is nothing cached to serve.
func TestAreas_OfflineWithACachedListing_ServesItAtAnyAge(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, areasCacheFile)
	if err := os.WriteFile(cache, []byte(`["branches"]`), 0o600); err != nil {
		t.Fatalf("prepare the cache: %v", err)
	}
	old := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(cache, old, old); err != nil {
		t.Fatalf("age the cache: %v", err)
	}
	f := newAreasFetcher(dir, "http://127.0.0.1:0/unused", Options{Offline: true})

	areas, err := f.Areas(context.Background())
	if err != nil {
		t.Fatalf("Areas() error = %v, want the stale cache served", err)
	}
	if strings.Join(areas, ",") != "branches" {
		t.Errorf("Areas() = %v, want the cached listing", areas)
	}
}

// TestAreas_OfflineWithNothingCached_Fails verifies that an offline run says it
// has no listing rather than reporting an empty one, which every caller would
// read as "GitLab documents nothing".
func TestAreas_OfflineWithNothingCached_Fails(t *testing.T) {
	f := newAreasFetcher(t.TempDir(), "http://127.0.0.1:0/unused", Options{Offline: true})

	_, err := f.Areas(context.Background())

	if err == nil {
		t.Fatal("Areas() error = nil, want the offline refusal")
	}
	if !strings.Contains(err.Error(), "not cached") {
		t.Errorf("Areas() error = %q, want it to say the listing is not cached", err)
	}
}

// TestAreas_ARefreshOrAnUnreadableCache_GoesBackToTheNetwork verifies the two
// ways a cached listing is not used. A cache that will not parse is a derived
// artifact worth three requests, so it is treated as absent rather than as a
// failure a caller has to clear by hand.
func TestAreas_ARefreshOrAnUnreadableCache_GoesBackToTheNetwork(t *testing.T) {
	cases := []struct {
		name   string
		cached string
		opts   Options
	}{
		{name: "a forced refresh", cached: `["stale"]`, opts: Options{Refresh: true}},
		{name: "a cache that is not a list", cached: "{not json at all", opts: Options{}},
		{name: "a cache holding an empty list", cached: `[]`, opts: Options{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, areasCacheFile), []byte(testCase.cached), 0o600); err != nil {
				t.Fatalf("prepare the cache: %v", err)
			}
			srv, _ := newTreeServer(t, []treePage{{{Path: areasPrefix + "fresh.md", Type: "blob"}}})
			f := newAreasFetcher(dir, srv.URL, testCase.opts)

			areas, err := f.Areas(context.Background())
			if err != nil {
				t.Fatalf("Areas() error = %v, want nil", err)
			}
			if strings.Join(areas, ",") != "fresh" {
				t.Errorf("Areas() = %v, want the freshly downloaded listing", areas)
			}
		})
	}
}

// TestAreas_ListingFailures_AreReported verifies that every way the listing can
// come back useless is an error. An audit that treats a broken listing as an
// empty one reports every endpoint this server calls as undocumented, which is
// a page of false findings rather than a missing check.
func TestAreas_ListingFailures_AreReported(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name:    "a body that is not the listing",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>login</html>")) },
			want:    "parse the doc/api listing",
		},
		{
			name:    "a listing with nothing in it",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[]")) },
			want:    "came back empty",
		},
		{
			name:    "an endpoint that is gone",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			want:    "HTTP 404",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			srv := httptest.NewServer(testCase.handler)
			t.Cleanup(srv.Close)
			f := newAreasFetcher(t.TempDir(), srv.URL, Options{})

			_, err := f.Areas(context.Background())

			if err == nil {
				t.Fatalf("Areas() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Areas() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestAreas_ACacheItCannotWrite_Fails verifies that a listing which was
// downloaded and could not be kept says so, rather than being silently
// re-downloaded on every run of an audit that fetches 250 pages behind it.
func TestAreas_ACacheItCannotWrite_Fails(t *testing.T) {
	srv, _ := newTreeServer(t, []treePage{{{Path: areasPrefix + "branches.md", Type: "blob"}}})

	t.Run("the cache directory is a file", func(t *testing.T) {
		dir := t.TempDir()
		blocked := filepath.Join(dir, "cache")
		if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		f := newAreasFetcher(dir, srv.URL, Options{})
		f.cacheDir = blocked

		if _, err := f.Areas(context.Background()); err == nil || !strings.Contains(err.Error(), "create cache dir") {
			t.Errorf("Areas() error = %v, want it to name the cache directory", err)
		}
	})

	t.Run("the cache file is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, areasCacheFile), 0o750); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		f := newAreasFetcher(dir, srv.URL, Options{})

		if _, err := f.Areas(context.Background()); err == nil || !strings.Contains(err.Error(), "write cache") {
			t.Errorf("Areas() error = %v, want it to name the cache file", err)
		}
	})
}
