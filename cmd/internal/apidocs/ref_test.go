// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package apidocs

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// testRef is the release tag the NewAtRef cases read at.
const testRef = "v19.5.0-ee"

// TestNewAtRef_APage_IsReadThroughTheFilesAPIAtTheRef verifies the request a
// fetcher at a ref sends for a page: the repository files API, with the file's
// whole path below doc/api escaped into one segment, and the ref named. The
// raw web route serves the same bytes and is what Cloudflare stops a sweep of,
// so the request has to be this one and not that.
func TestNewAtRef_APage_IsReadThroughTheFilesAPIAtTheRef(t *testing.T) {
	var path, ref string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ref = r.URL.EscapedPath(), r.URL.Query().Get("ref")
		_, _ = w.Write([]byte("# npm"))
	}))
	t.Cleanup(srv.Close)
	f, err := NewAtRef(t.TempDir(), testRef, Options{FilesURL: srv.URL + "/files/", CacheDir: t.TempDir(), Spacing: -1})
	if err != nil {
		t.Fatalf("NewAtRef() error = %v", err)
	}

	body, err := f.Fetch(t.Context(), "packages/npm")

	if err != nil || body != "# npm" {
		t.Fatalf("Fetch() = %q, %v; want the page", body, err)
	}
	if path != "/files/doc%2Fapi%2Fpackages%2Fnpm.md/raw" || ref != testRef {
		t.Errorf("request = %s with ref %q, want /files/doc%%2Fapi%%2Fpackages%%2Fnpm.md/raw with ref %s", path, ref, testRef)
	}
}

// TestNewAtRef_EmptyOptions_UseTheRefsDefaults verifies what a fetcher at a
// ref reads from when the caller names nothing: gitlab.com's files API at the
// ref, the doc/api listing at the ref, and a cache of the ref's own beside the
// shared one, so a page read at one release never answers for another. A
// fetcher without a ref reads under the raw root as it always did, whatever
// files URL it is handed.
func TestNewAtRef_EmptyOptions_UseTheRefsDefaults(t *testing.T) {
	root := t.TempDir()
	f, err := NewAtRef(root, testRef, Options{})
	if err != nil {
		t.Fatalf("NewAtRef() error = %v", err)
	}

	want := []string{
		DefaultFilesURL + "doc%2Fapi%2Fbranches.md/raw?ref=" + testRef,
		DefaultAreasURL + "&ref=" + testRef,
		CacheDir(root) + "@" + testRef,
	}
	if got := []string{f.pageURL("branches"), f.areasURL, f.cacheDir}; !reflect.DeepEqual(got, want) {
		t.Errorf("page, listing and cache = %q, want %q", got, want)
	}
	if got := New(root, Options{FilesURL: "https://example.test/files/"}).pageURL("branches"); got != DefaultBaseURL+"branches.md" {
		t.Errorf("a fetcher without a ref reads %q, want %q", got, DefaultBaseURL+"branches.md")
	}
}

// TestNewAtRef_TheOptionsGiven_Win verifies that the listing, the cache and
// the files API a caller names survive NewAtRef, which is how a test points a
// fetcher at a ref at a fixture.
func TestNewAtRef_TheOptionsGiven_Win(t *testing.T) {
	cache := t.TempDir()
	f, err := NewAtRef(t.TempDir(), testRef, Options{
		FilesURL: "https://example.test/files/",
		AreasURL: "https://example.test/tree",
		CacheDir: cache,
	})
	if err != nil {
		t.Fatalf("NewAtRef() error = %v", err)
	}

	want := []string{"https://example.test/files/doc%2Fapi%2Ftags.md/raw?ref=" + testRef, "https://example.test/tree", cache}
	if got := []string{f.pageURL("tags"), f.areasURL, f.cacheDir}; !reflect.DeepEqual(got, want) {
		t.Errorf("page, listing and cache = %q, want %q", got, want)
	}
}

// TestNewAtRef_ARefThatIsNotOneSegment_IsRefused verifies the spelling a ref
// is held to. The ref names a cache directory, so one carrying a slash or
// beginning with a dot would put that directory somewhere other than beside
// the shared cache; every release tag and master pass.
func TestNewAtRef_ARefThatIsNotOneSegment_IsRefused(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		want bool
	}{
		{name: "a release tag", ref: testRef, want: true},
		{name: "master", ref: "master", want: true},
		{name: "a stable branch", ref: "19-5-stable-ee", want: true},
		{name: "an underscore", ref: "a_b", want: true},
		{name: "no ref", ref: ""},
		{name: "a slash", ref: "feature/x"},
		{name: "a leading dot", ref: ".hidden"},
		{name: "a parent directory", ref: "../escape"},
		{name: "a leading dash", ref: "-flag"},
		{name: "a space", ref: "with space"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			f, err := NewAtRef(t.TempDir(), testCase.ref, Options{})

			if (err == nil) != testCase.want || (f != nil) != testCase.want {
				t.Errorf("NewAtRef(%q) = %v, %v; want accepted %t", testCase.ref, f, err, testCase.want)
			}
			if err != nil && !strings.Contains(err.Error(), "is not a ref") {
				t.Errorf("NewAtRef(%q) error = %v, want it to say why", testCase.ref, err)
			}
		})
	}
}
