package finegrained

import (
	"net/http"
	"strings"
	"testing"
)

// routeTable is a table of REST operations written the way the live record
// spells routes, with a GraphQL operation and two names no route reader
// should take for a route among them, listed so that the template that wins a
// tie comes last.
func routeTable() *Table {
	return &Table{Operations: []Operation{
		{Name: "GET /projects/:id/things/:b"},
		{Name: "GET /projects/:id/things/:a"},
		{Name: "GET /projects/:id/issues"},
		{Name: "GET /projects/:id/issues/:issue_iid"},
		{Name: "GET /projects/:id/issues/statistics"},
		{Name: "GET /projects/:id/(-/)search"},
		{Name: "GET /projects/:id/jobs/:job_id/artifacts/*artifact_path"},
		{Name: "GET /projects/:id/repository/files/:file_path/raw"},
		{Name: "HEAD /projects/:id/repository/files/:file_path"},
		{Name: "GET /releases/permalink/latest(/)(*suffix_path)"},
		{Name: "GET /nested(/one(/two))"},
		{Name: "GET /projects/:id/(broken"},
		{Name: "GET /sidekiq/job_stats"},
		{Name: "query project (queryListBranchRulesCE)"},
		{Name: "GET"},
	}}
}

// TestRouteTemplate_NamesTheTableRouteARequestReached holds every rule the
// matcher reads a request path by: repeated slashes count as one, so an empty
// segment never fills a placeholder and a doubled slash still names its route,
// placeholders take one segment, a wildcard takes one or more, Grape's optional groups are
// matched both ways and nested, the most literal template wins and a tie goes
// to the one that sorts first, a HEAD with no route of its own is matched
// against the GETs, the GraphQL endpoint is named as itself, an instance's
// own path prefix in front of the API root is cut, and anything outside the
// v4 API or outside the table is named by nothing. The template it answers is
// the table's own spelling under /api/v4, never the path.
func TestRouteTemplate_NamesTheTableRouteARequestReached(t *testing.T) {
	table := routeTable()
	cases := []struct {
		name, method, path, want string
		known                    bool
	}{
		{"a collection", http.MethodGet, "/api/v4/projects/1/issues", "/api/v4/projects/:id/issues", true},
		{"an escaped path is one segment", http.MethodGet, "/api/v4/projects/acme%2Frepo/issues/7", "/api/v4/projects/:id/issues/:issue_iid", true},
		{"the literal route wins", http.MethodGet, "/api/v4/projects/1/issues/statistics", "/api/v4/projects/:id/issues/statistics", true},
		{"an optional group taken", http.MethodGet, "/api/v4/projects/1/-/search", "/api/v4/projects/:id/(-/)search", true},
		{"an optional group left out", http.MethodGet, "/api/v4/projects/1/search", "/api/v4/projects/:id/(-/)search", true},
		{"a wildcard takes several segments", http.MethodGet, "/api/v4/projects/1/jobs/2/artifacts/a/b/c.txt", "/api/v4/projects/:id/jobs/:job_id/artifacts/*artifact_path", true},
		{"a wildcard takes at least one", http.MethodGet, "/api/v4/projects/1/jobs/2/artifacts", "", false},
		{"a HEAD of its own", http.MethodHead, "/api/v4/projects/1/repository/files/README.md", "/api/v4/projects/:id/repository/files/:file_path", true},
		{"a HEAD answered from the GET", http.MethodHead, "/api/v4/projects/1/repository/files/README.md/raw", "/api/v4/projects/:id/repository/files/:file_path/raw", true},
		{"a HEAD no GET answers", http.MethodHead, "/api/v4/version", "", false},
		{"two optional groups taken", http.MethodGet, "/api/v4/releases/permalink/latest/downloads/bin", "/api/v4/releases/permalink/latest(/)(*suffix_path)", true},
		{"two optional groups left out", http.MethodGet, "/api/v4/releases/permalink/latest", "/api/v4/releases/permalink/latest(/)(*suffix_path)", true},
		{"a nested group taken whole", http.MethodGet, "/api/v4/nested/one/two", "/api/v4/nested(/one(/two))", true},
		{"a nested group taken in part", http.MethodGet, "/api/v4/nested/one", "/api/v4/nested(/one(/two))", true},
		{"a nested group left out", http.MethodGet, "/api/v4/nested", "/api/v4/nested(/one(/two))", true},
		{"a tie goes to the first in order", http.MethodGet, "/api/v4/projects/1/things/9", "/api/v4/projects/:id/things/:a", true},
		{"an empty segment fills no placeholder", http.MethodGet, "/api/v4/projects//issues", "", false},
		{"repeated slashes count as one", http.MethodGet, "/api/v4//sidekiq/job_stats", "/api/v4/sidekiq/job_stats", true},
		{"a trailing slash counts as none", http.MethodGet, "/api/v4/projects/1/issues/", "/api/v4/projects/:id/issues", true},
		{"an unbalanced group matches nothing", http.MethodGet, "/api/v4/projects/1/broken", "", false},
		{"the GraphQL endpoint", http.MethodPost, "/api/graphql", "/api/graphql", true},
		{"a route the table does not hold", http.MethodGet, "/api/v4/version", "", false},
		{"a method the table does not hold", http.MethodDelete, "/api/v4/projects/1/issues", "", false},
		{"the API root itself", http.MethodGet, "/api/v4", "", false},
		{"a prefix that only begins like the API", http.MethodGet, "/api/v4x/projects/1/issues", "", false},
		{"another API", http.MethodGet, "/api/v5/projects/1/issues", "", false},
		{"under a relative URL root", http.MethodGet, "/gitlab/api/v4/projects/1/issues", "/api/v4/projects/:id/issues", true},
		{"a doubled slash before the API root", http.MethodGet, "//api/v4/projects/1/issues", "/api/v4/projects/:id/issues", true},
		{"under a relative URL root of two segments", http.MethodGet, "/tools/gitlab/api/v4/projects/1/issues/statistics", "/api/v4/projects/:id/issues/statistics", true},
		{"the GraphQL endpoint under a relative URL root", http.MethodPost, "/gitlab/api/graphql", "/api/graphql", true},
		{"a HEAD under a relative URL root", http.MethodHead, "/gitlab/api/v4/projects/1/repository/files/README.md/raw", "/api/v4/projects/:id/repository/files/:file_path/raw", true},
		{"an encoded name holding the API segment", http.MethodGet, "/gitlab/api/v4/projects/acme%2Fapi%2Frepo/issues", "/api/v4/projects/:id/issues", true},
		{"a relative URL root holding the API segment", http.MethodGet, "/api/gitlab/api/v4/projects/1/issues", "", false},
		{"a relative URL root and no API", http.MethodGet, "/gitlab/projects/1/issues", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, known := table.RouteTemplate(tc.method, tc.path)
			if got != tc.want || known != tc.known {
				t.Errorf("RouteTemplate(%s %s) = %q, %t; want %q, %t", tc.method, tc.path, got, known, tc.want, tc.known)
			}
		})
	}
}

// TestRoutes_IndexesTheRESTTemplatesAlone verifies the index holds a method
// for each REST template and none for a GraphQL operation, whose name has a
// space and no path after it, or for a name with nothing after the method.
func TestRoutes_IndexesTheRESTTemplatesAlone(t *testing.T) {
	index := routeTable().routes()
	if len(index[http.MethodGet]) == 0 || len(index[http.MethodHead]) != 1 {
		t.Errorf("the index holds %d GET and %d HEAD variants, want the table's", len(index[http.MethodGet]), len(index[http.MethodHead]))
	}
	if _, found := index["query"]; found || len(index) != 2 {
		t.Errorf("the index holds the methods %v, want GET and HEAD alone", index)
	}
}

// TestMatchSegments_APlaceholderTakesOneSegmentThatIsNotEmpty verifies a
// placeholder is filled by a segment holding something and not by an empty
// one, which the path reader never hands it but the matcher refuses on its
// own. The segments are split from paths rather than written as slice
// literals, so gosec does not carry a literal length into the matcher's
// wildcard loop (a G602 false positive).
func TestMatchSegments_APlaceholderTakesOneSegmentThatIsNotEmpty(t *testing.T) {
	template := strings.Split("/projects/:id", "/")
	if !matchSegments(template, strings.Split("/projects/1", "/")) {
		t.Error("a placeholder refused a segment holding an identifier")
	}
	if matchSegments(template, strings.Split("/projects/", "/")) {
		t.Error("a placeholder took an empty segment")
	}
}

// TestRouteTemplate_BuildsEachTableIndexOnce verifies the index is built the
// first time a table is asked and read back afterwards, kept per table, so a
// second table is never answered from the first one's routes.
func TestRouteTemplate_BuildsEachTableIndexOnce(t *testing.T) {
	first := routeTable()
	if got, _ := first.RouteTemplate(http.MethodGet, "/api/v4/projects/1/issues"); got != "/api/v4/projects/:id/issues" {
		t.Fatalf("first read = %q", got)
	}
	built, ok := routeIndexes.Load(first)
	if !ok {
		t.Fatal("the first read built no index for the table")
	}
	if got, _ := first.RouteTemplate(http.MethodGet, "/api/v4/projects/1/issues"); got != "/api/v4/projects/:id/issues" {
		t.Errorf("second read = %q", got)
	}
	if again, _ := routeIndexes.Load(first); len(again.(map[string][]routeVariant)) != len(built.(map[string][]routeVariant)) {
		t.Error("the second read replaced the index the first one built")
	}
	other := &Table{Operations: []Operation{{Name: "GET /groups/:id"}}}
	if _, known := other.RouteTemplate(http.MethodGet, "/api/v4/projects/1/issues"); known {
		t.Error("a second table was answered from the first one's routes")
	}
}
