// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apidocs"
)

// docsServer serves a fixture documentation tree: a repository listing naming
// every page, and each page's markdown.
func docsServer(t *testing.T, pages map[string]string) *apidocs.Fetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tree") {
			entries := make([]map[string]string, 0, len(pages))
			for area := range pages {
				entries = append(entries, map[string]string{"path": "doc/api/" + area + ".md", "type": "blob"})
			}
			if page, _ := strconv.Atoi(r.URL.Query().Get("page")); page > 1 {
				entries = nil
			}
			_ = json.NewEncoder(w).Encode(entries)
			return
		}
		area := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/docs/"), ".md")
		body, ok := pages[area]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return apidocs.New(t.TempDir(), apidocs.Options{
		BaseURL:  srv.URL + "/docs/",
		AreasURL: srv.URL + "/tree",
		CacheDir: t.TempDir(),
		Spacing:  noSpacing,
		Strict:   true,
	})
}

// noSpacing removes the fetcher's post-download pause for the tests below.
//
// That pause is 500ms and exists to keep a 250-page sweep from tripping
// GitLab's rate limit. The server every test here fetches from is an httptest
// handler in this process, which rate-limits nothing, so the pause bought
// nothing and cost one wall-clock half-second per page. The pause itself is
// still the default everywhere a fetcher reaches gitlab.com, and apidocs' own
// TestRequest_Spacing_IsTheOptionTheCallerGave holds it to that.
const noSpacing = -1

// TestBuildDocEndpoints_ThePages_AreReadIntoTheOracle verifies the generator:
// every page is read but the generated GraphQL reference, each endpoint is
// written once however many pages and lines spell it, with the pages that do,
// and the file is ordered by path and then by method, so a regeneration is a
// diff of what the documentation changed and nothing else.
func TestBuildDocEndpoints_ThePages_AreReadIntoTheOracle(t *testing.T) {
	fetcher := docsServer(t, map[string]string{
		"branches": "```plaintext\nGET /projects/:id/repository/branches\n```\n" +
			"```plaintext\nPOST /projects/:id/repository/branches\nGET /projects/:id/repository/branches\n```\n",
		"protected_branches":       "```plaintext\nGET projects/:id/repository/branches\n```\n",
		"tags":                     "```plaintext\nDELETE /projects/:id/repository/tags/:tag_name\n```\n",
		"graphql/reference/_index": "```plaintext\nGET /never/read\n```\n",
	})

	content, err := BuildDocEndpoints(t.Context(), fetcher, fixtureRef)
	if err != nil {
		t.Fatalf("BuildDocEndpoints() error = %v", err)
	}

	if !bytes.HasSuffix(content, []byte("}\n")) {
		t.Error("the oracle does not end in a newline")
	}
	var oracle DocEndpoints
	if decodeErr := json.Unmarshal(content, &oracle); decodeErr != nil {
		t.Fatalf("the oracle is not JSON: %v", decodeErr)
	}
	wantSource := DocSource{Repository: docRepository, Ref: fixtureRef, Areas: 3, SkippedAreas: 1, Endpoints: 3}
	if oracle.SchemaVersion != docEndpointsSchema || oracle.Note != docEndpointsNote || oracle.Source != wantSource {
		t.Errorf("schema %d, source %+v; want schema %d and %+v", oracle.SchemaVersion, oracle.Source, docEndpointsSchema, wantSource)
	}
	want := []DocumentedEndpoint{
		{Method: "GET", Path: "/projects/:id/repository/branches", Areas: []string{"branches", "protected_branches"}},
		{Method: "POST", Path: "/projects/:id/repository/branches", Areas: []string{"branches"}},
		{Method: "DELETE", Path: "/projects/:id/repository/tags/:tag_name", Areas: []string{"tags"}},
	}
	if !reflect.DeepEqual(oracle.Endpoints, want) {
		t.Errorf("endpoints = %+v, want %+v", oracle.Endpoints, want)
	}
}

// TestBuildDocEndpoints_WhatLeavesAHole_Fails verifies that the generator
// writes nothing it could not read whole: a listing it cannot get, a page that
// will not fetch, a sweep the caller cancelled, and pages none of which spells
// an endpoint line. A comparison run on the spot could count a missing page
// and carry on; a committed oracle with a hole in it would stand in the
// repository as findings about the fetch until the next regeneration.
func TestBuildDocEndpoints_WhatLeavesAHole_Fails(t *testing.T) {
	cases := []struct {
		name    string
		fetcher func(t *testing.T) *apidocs.Fetcher
		ctx     func(t *testing.T) context.Context
		want    string
	}{
		{
			name: "a listing it cannot get",
			fetcher: func(t *testing.T) *apidocs.Fetcher {
				t.Helper()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
				t.Cleanup(srv.Close)
				return apidocs.New(t.TempDir(), apidocs.Options{AreasURL: srv.URL + "/tree", CacheDir: t.TempDir(), Spacing: noSpacing})
			},
			want: "list doc/api at " + fixtureRef,
		},
		{
			name: "a page that will not fetch",
			fetcher: func(t *testing.T) *apidocs.Fetcher {
				t.Helper()
				// The listing names a page the server does not have.
				cache := t.TempDir()
				if err := os.WriteFile(filepath.Join(cache, "_areas.json"), []byte(`["branches","gone"]`), 0o600); err != nil {
					t.Fatalf("prepare the cache: %v", err)
				}
				return docsServerAt(t, cache, map[string]string{"branches": "```plaintext\nGET /projects/:id/repository/branches\n```\n"})
			},
			want: "read doc/api/gone.md at " + fixtureRef,
		},
		{
			name: "a sweep the caller cancelled",
			fetcher: func(t *testing.T) *apidocs.Fetcher {
				t.Helper()
				// The listing is served from the cache, so the run gets past it
				// and the cancellation lands where it matters: on the pages.
				cache := t.TempDir()
				if err := os.WriteFile(filepath.Join(cache, "_areas.json"), []byte(`["branches"]`), 0o600); err != nil {
					t.Fatalf("prepare the cache: %v", err)
				}
				return docsServerAt(t, cache, map[string]string{"branches": "```plaintext\nGET /projects/:id/repository/branches\n```\n"})
			},
			ctx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			want: "read doc/api/branches.md at " + fixtureRef,
		},
		{
			name: "pages that spell no endpoint line",
			fetcher: func(t *testing.T) *apidocs.Fetcher {
				t.Helper()
				return docsServer(t, map[string]string{"branches": "Branches are listed with GET, in prose.\n"})
			},
			want: "spells an endpoint line",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := t.Context()
			if testCase.ctx != nil {
				ctx = testCase.ctx(t)
			}

			content, err := BuildDocEndpoints(ctx, testCase.fetcher(t), fixtureRef)

			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("BuildDocEndpoints() error = %v, want one naming %q", err, testCase.want)
			}
			if content != nil {
				t.Errorf("BuildDocEndpoints() = %q, want nothing written", content)
			}
		})
	}
}

// docsServerAt is [docsServer] with the cache the case prepared, so a case can
// hand the fetcher a listing the server would not give.
func docsServerAt(t *testing.T, cache string, pages map[string]string) *apidocs.Fetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/docs/"), ".md")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return apidocs.New(t.TempDir(), apidocs.Options{
		BaseURL:  srv.URL + "/docs/",
		AreasURL: srv.URL + "/tree",
		CacheDir: cache,
		Spacing:  noSpacing,
		Strict:   true,
	})
}

// TestPageEndpoints_OnePage_YieldsItsEndpointsOnce verifies the reading of a
// page, including that a repeated endpoint is kept once and that prose which
// merely mentions a method is not read as one.
func TestPageEndpoints_OnePage_YieldsItsEndpointsOnce(t *testing.T) {
	found := pageEndpoints(strings.Join([]string{
		"GET /projects/:id/issues",
		"    GET /projects/:id/issues",
		"POST /api/v4/projects/:id/issues/",
		// The prefix on its own names no endpoint, and neither does a sentence
		// that happens to open with a method.
		"GET /api/v4",
		"GET /",
		"GET is the method this endpoint takes.",
		"Use the GET verb.",
	}, "\n"))

	want := []methodPath{
		{Method: "GET", Path: "/projects/:id/issues"},
		{Method: "POST", Path: "/projects/:id/issues"},
	}
	if !slices.Equal(found, want) {
		t.Errorf("pageEndpoints() = %v, want %v", found, want)
	}
}

// TestReleaseRef_IsTheTagOfTheLiveRecordsRelease verifies the ref the
// generator reads at by default and the comparison holds the oracle to: the
// tag gitlab-org/gitlab cuts each release from, which is the record's version
// with a v in front.
func TestReleaseRef_IsTheTagOfTheLiveRecordsRelease(t *testing.T) {
	ref, err := ReleaseRef(recordIn(t, nil))
	if err != nil || ref != fixtureRef {
		t.Errorf("ReleaseRef() = %q, %v; want %q", ref, err, fixtureRef)
	}
}

// TestDocEndpoints_TheCommittedOracle_IsTheOneTheGeneratorWrites holds the
// committed oracle to what make gen-api-doc-endpoints writes, offline: read at
// the live record's release, in the order and form the generator gives it,
// every endpoint once and named by the pages that spell it, and the counts
// its source quotes the counts of what it holds. A hand edit that documents an
// endpoint the pages do not is still possible, and regenerating shows it as a
// diff; a hand edit that breaks the form is caught here.
//
// It compares two committed files and regenerates nothing, so it is not
// deferred: no layer of a stack can make it fail except by moving one without
// the other, which is what a re-pin that forgets the oracle does.
func TestDocEndpoints_TheCommittedOracle_IsTheOneTheGeneratorWrites(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(DocEndpointsFile)))
	if err != nil {
		t.Fatalf("read the committed oracle: %v", err)
	}
	oracle, err := readDocEndpoints(root)
	if err != nil {
		t.Fatalf("readDocEndpoints() error = %v", err)
	}
	release, err := ReleaseRef(root)
	if err != nil {
		t.Fatalf("ReleaseRef() error = %v", err)
	}
	if oracle.Source.Ref != release || oracle.Source.Repository != docRepository || oracle.Note != docEndpointsNote {
		t.Errorf("source = %+v, note %q; want %s read at %s with the generator's note", oracle.Source, oracle.Note, docRepository, release)
	}
	if oracle.Source.Endpoints != len(oracle.Endpoints) || oracle.Source.Areas == 0 {
		t.Errorf("source counts %d endpoints from %d pages and the file holds %d", oracle.Source.Endpoints, oracle.Source.Areas, len(oracle.Endpoints))
	}
	ordered := slices.IsSortedFunc(oracle.Endpoints, func(a, b DocumentedEndpoint) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Method, b.Method))
	})
	if !ordered {
		t.Error("the endpoints are not ordered by path and then by method")
	}
	for i, endpoint := range oracle.Endpoints {
		if i > 0 && endpoint.Method == oracle.Endpoints[i-1].Method && endpoint.Path == oracle.Endpoints[i-1].Path {
			t.Errorf("%s %s is written twice", endpoint.Method, endpoint.Path)
		}
		if len(endpoint.Areas) == 0 || !slices.IsSorted(endpoint.Areas) || len(slices.Compact(slices.Clone(endpoint.Areas))) != len(endpoint.Areas) {
			t.Errorf("%s %s names the pages %v, want them sorted, at least one and each once", endpoint.Method, endpoint.Path, endpoint.Areas)
		}
		if "/"+strings.Join(normalizePath(endpoint.Path), "/") != endpoint.Path {
			t.Errorf("%s %s is not written in the form the comparison normalizes to", endpoint.Method, endpoint.Path)
		}
	}
	encoded, err := json.MarshalIndent(oracle, "", "  ")
	if err != nil {
		t.Fatalf("re-encode the oracle: %v", err)
	}
	if !bytes.Equal(append(encoded, '\n'), raw) {
		t.Error("the committed oracle is not in the form the generator writes: regenerate it with make gen-api-doc-endpoints")
	}
}
