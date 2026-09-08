package paths

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
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
	})
}

// TestCheckEndpoints_RecordedPaths_AreHeldToTheDocumentation verifies the
// comparison end to end, over the spellings the real documentation uses: a
// path written without its leading slash, one carrying the /api/v4 prefix, a
// placeholder where our recording carries a fixture value, and a glob standing
// for the rest of the path.
func TestCheckEndpoints_RecordedPaths_AreHeldToTheDocumentation(t *testing.T) {
	fetcher := docsServer(t, map[string]string{
		"branches": "## List branches\n\n```plaintext\nGET /projects/:id/repository/branches\n```\n\n" +
			"```plaintext\nDELETE /projects/:id/repository/branches/:branch\n```\n",
		"access_tokens":  "```plaintext\nGET projects/:id/access_tokens\n```\n",
		"model_registry": "```plaintext\nPUT /api/v4/projects/:id/packages/ml_models/:version/files/(*path)\n```\n",
	})
	rows := []requestinventory.Row{
		{Package: "internal/tools/branches", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id/repository/branches"},
		{Package: "internal/tools/branches", Kind: requestinventory.KindREST, Method: "DELETE", Path: "/projects/:id/repository/branches/main"},
		{Package: "internal/tools/accesstokens", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id/access_tokens"},
		{Package: "internal/tools/modelregistry", Kind: requestinventory.KindREST, Method: "PUT", Path: "/projects/:id/packages/ml_models/:iid/files/models/model.bin"},
		{Package: "internal/tools/integrations", Kind: requestinventory.KindREST, Method: "DELETE", Path: "/projects/:id/services/slack"},
		{Package: "internal/tools/epics", Kind: requestinventory.KindGraphQL, Method: "POST", Path: "/graphql", Operation: "query epics"},
	}

	check, err := checkEndpoints(t.Context(), fetcher, rows)
	if err != nil {
		t.Fatalf("checkEndpoints() error = %v, want nil", err)
	}
	if !check.Ran || check.Areas != 3 || len(check.UnreadAreas) != 0 {
		t.Fatalf("check = %+v, want three pages read and none unread", check)
	}
	if check.RecordedEndpoints != 5 {
		t.Errorf("recorded endpoints = %d, want the five REST rows and not the GraphQL one", check.RecordedEndpoints)
	}
	if len(check.Undocumented) != 1 {
		t.Fatalf("undocumented = %+v, want only the deprecated services endpoint", check.Undocumented)
	}
	if check.Undocumented[0].Path != "/projects/:id/services/slack" {
		t.Errorf("undocumented = %+v, want the services endpoint", check.Undocumented[0])
	}
}

// TestCheckEndpoints_APageItCannotRead_IsCountedNotFatal verifies the honesty
// half. A documentation page that will not fetch leaves a hole, and every
// endpoint documented only there is reported as undocumented, so a reader has
// to be told how big the hole is rather than have the run fail or the hole
// hidden.
func TestCheckEndpoints_APageItCannotRead_IsCountedNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tree") {
			if page, _ := strconv.Atoi(r.URL.Query().Get("page")); page > 1 {
				_, _ = w.Write([]byte("[]"))
				return
			}
			_, _ = w.Write([]byte(`[{"path":"doc/api/gone.md","type":"blob"},{"path":"doc/api/graphql/reference/_index.md","type":"blob"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	fetcher := apidocs.New(t.TempDir(), apidocs.Options{BaseURL: srv.URL + "/docs/", AreasURL: srv.URL + "/tree", CacheDir: t.TempDir()})

	check, err := checkEndpoints(t.Context(), fetcher, []requestinventory.Row{
		{Package: "internal/tools/issues", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id/issues"},
	})
	if err != nil {
		t.Fatalf("checkEndpoints() error = %v, want the hole reported instead", err)
	}
	if !slices.Equal(check.UnreadAreas, []string{"gone"}) {
		t.Errorf("unread areas = %v, want the page that would not fetch", check.UnreadAreas)
	}
	if check.Areas != 0 {
		t.Errorf("areas = %d, want none read", check.Areas)
	}
	if len(check.Undocumented) != 1 {
		t.Errorf("undocumented = %+v, want the recorded endpoint, since nothing documented it", check.Undocumented)
	}
}

// TestCheckEndpoints_ACancelledSweep_Fails verifies that an interrupted run
// stops rather than reporting the pages it never reached as documentation gaps.
// One Ctrl+C halfway through 250 pages would otherwise produce a page of
// findings about the interruption.
func TestCheckEndpoints_ACancelledSweep_Fails(t *testing.T) {
	// The listing is served from the cache, so the run gets past it and the
	// cancellation lands where it matters: on the page sweep.
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(cache, "_areas.json"), []byte(`["branches"]`), 0o600); err != nil {
		t.Fatalf("prepare the cache: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("```plaintext\nGET /projects/:id/repository/branches\n```\n"))
	}))
	t.Cleanup(srv.Close)
	fetcher := apidocs.New(t.TempDir(), apidocs.Options{BaseURL: srv.URL + "/docs/", AreasURL: srv.URL + "/tree", CacheDir: cache})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := checkEndpoints(ctx, fetcher, []requestinventory.Row{
		{Package: "internal/tools/branches", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id/repository/branches"},
	})

	if err == nil {
		t.Fatal("checkEndpoints() error = nil, want the cancellation reported")
	}
}

// TestCheckEndpoints_AListingItCannotGet_Fails verifies that a run which never
// learned what GitLab documents stops, rather than reporting every endpoint
// this server issues as undocumented.
func TestCheckEndpoints_AListingItCannotGet_Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	fetcher := apidocs.New(t.TempDir(), apidocs.Options{AreasURL: srv.URL + "/tree", CacheDir: t.TempDir()})

	_, err := checkEndpoints(t.Context(), fetcher, nil)

	if err == nil {
		t.Fatal("checkEndpoints() error = nil, want the listing failure")
	}
}

// TestMatchesShape_TheToleranceIsOneSided verifies the comparison itself. A
// documented placeholder matches whatever we put there, which is what makes a
// recorded fixture value comparable at all; nothing else is tolerated.
func TestMatchesShape_TheToleranceIsOneSided(t *testing.T) {
	cases := []struct {
		name       string
		recorded   string
		documented string
		want       bool
	}{
		{name: "the same path", recorded: "/projects/:id/issues", documented: "/projects/:id/issues", want: true},
		{name: "a fixture value under a placeholder", recorded: "/projects/:id/repository/branches/main", documented: "/projects/:id/repository/branches/:branch", want: true},
		{name: "a worked example in the documentation", recorded: "/projects/:id/issues/:iid", documented: "/projects/1/issues/80", want: true},
		{name: "an angle-bracket placeholder", recorded: "/projects/:id/foo", documented: "/projects/<id>/foo", want: true},
		{name: "a glob swallowing the rest", recorded: "/packages/:id/files/models/model.bin", documented: "/packages/:id/files/(*path)", want: true},
		{name: "a glob with nothing left to swallow", recorded: "/packages/:id/files", documented: "/packages/:id/files/(*path)", want: false},

		{name: "a different literal", recorded: "/projects/:id/services/slack", documented: "/projects/:id/integrations/:slug", want: false},
		{name: "one segment too many", recorded: "/projects/:id/issues/:iid", documented: "/projects/:id/issues", want: false},
		{name: "one segment too few", recorded: "/projects/:id", documented: "/projects/:id/issues", want: false},
		{name: "a placeholder of ours over a literal of theirs", recorded: "/projects/:id/:iid", documented: "/projects/:id/issues", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := matchesShape(normalizePath(testCase.recorded), normalizePath(testCase.documented))

			if got != testCase.want {
				t.Errorf("matchesShape(%q, %q) = %v, want %v", testCase.recorded, testCase.documented, got, testCase.want)
			}
		})
	}
}

// TestNormalizePath_TheSpellingsTheDocumentationUses_AreAllTheSamePath
// verifies the normalization, since GitLab writes the same endpoint four ways
// and each difference would otherwise be a false finding.
func TestNormalizePath_TheSpellingsTheDocumentationUses_AreAllTheSamePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string
	}{
		{name: "as recorded", path: "/projects/:id/issues", want: []string{"projects", ":id", "issues"}},
		{name: "without the leading slash", path: "projects/:id/issues", want: []string{"projects", ":id", "issues"}},
		{name: "with the api prefix", path: "/api/v4/projects/:id/issues", want: []string{"projects", ":id", "issues"}},
		{name: "with a trailing slash", path: "/projects/:id/issues/", want: []string{"projects", ":id", "issues"}},
		{name: "with a query", path: "/projects/:id/issues?state=opened", want: []string{"projects", ":id", "issues"}},
		{name: "with a fragment", path: "/projects/:id/issues#notes", want: []string{"projects", ":id", "issues"}},
		{name: "the root", path: "/", want: nil},
		{name: "nothing at all", path: "", want: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := normalizePath(testCase.path); !slices.Equal(got, testCase.want) {
				t.Errorf("normalizePath(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestAddDocumentedEndpoints_OnePage_YieldsItsEndpointsOnce verifies the
// reading of a page, including that a repeated endpoint is stored once and that
// prose which merely mentions a method is not read as one.
func TestAddDocumentedEndpoints_OnePage_YieldsItsEndpointsOnce(t *testing.T) {
	documented := map[string][][]string{}

	addDocumentedEndpoints(documented, strings.Join([]string{
		"GET /projects/:id/issues",
		"    GET /projects/:id/issues",
		"POST /projects/:id/issues",
		// The prefix on its own names no endpoint, and neither does a sentence
		// that happens to open with a method.
		"GET /api/v4",
		"GET /",
		"GET is the method this endpoint takes.",
		"Use the GET verb.",
	}, "\n"))

	if len(documented["GET"]) != 1 {
		t.Errorf("GET endpoints = %v, want the one path, stored once", documented["GET"])
	}
	if len(documented["POST"]) != 1 {
		t.Errorf("POST endpoints = %v, want the one path", documented["POST"])
	}
}

// TestRecordedEndpoints_OneEntryPerMethodAndPath verifies the folding, since a
// list that named one of the packages issuing an endpoint would send a reader
// to the wrong file.
func TestRecordedEndpoints_OneEntryPerMethodAndPath(t *testing.T) {
	endpoints := recordedEndpoints([]requestinventory.Row{
		{Package: "internal/tools/second", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "DELETE", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "GET", Path: "/groups/:id"},
		{Package: "internal/tools/epics", Kind: requestinventory.KindGraphQL, Method: "POST", Path: "/graphql"},
	})

	if len(endpoints) != 3 {
		t.Fatalf("recordedEndpoints() = %+v, want three REST endpoints", endpoints)
	}
	if endpoints[0].Path != "/groups/:id" || endpoints[1].Method != http.MethodDelete {
		t.Errorf("recordedEndpoints() = %+v, want them ordered by path then method", endpoints)
	}
	if !slices.Equal(endpoints[2].Packages, []string{"internal/tools/first", "internal/tools/second"}) {
		t.Errorf("packages = %v, want every package that issues it, sorted", endpoints[2].Packages)
	}
}

// TestEndpointString_ReadsAsALineAReportPrints verifies the rendering a text
// summary uses.
func TestEndpointString_ReadsAsALineAReportPrints(t *testing.T) {
	line := Endpoint{Method: "GET", Path: "/projects/:id", Packages: []string{"a", "b"}}.String()

	if line != "GET /projects/:id (a, b)" {
		t.Errorf("String() = %q, want the method, the path and the packages", line)
	}
}
