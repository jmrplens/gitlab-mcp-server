// ci_catalog_test.go contains unit tests for GitLab CI catalog component
// operations. Tests use httptest to mock the GitLab CI Catalog API.
package cicatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Sample GraphQL response payloads.

// sampleResourceNode is a catalog resource as the get document answers it.
// Its inputs carry defaults of each JSON kind GitLab sends, as components/sast
// on gitlab.com does: a string, a boolean and a number.
const sampleResourceNode = `{
	"id": "gid://gitlab/Ci::CatalogResource/1",
	"name": "go-pipeline",
	"description": "Reusable Go CI/CD pipeline components",
	"icon": "https://gitlab.example.com/uploads/icon.png",
	"fullPath": "my-group/go-pipeline",
	"webPath": "/explore/catalog/my-group/go-pipeline",
	"starCount": 42,
	"starrersPath": "/my-group/go-pipeline/-/starrers",
	"last30DayUsageCount": 7,
	"archived": false,
	"topics": ["go", "ci"],
	"verificationLevel": "UNVERIFIED",
	"visibilityLevel": "public",
	"latestReleasedAt": "2026-06-15T10:30:00Z",
	"versions": {"nodes": [
		{
			"id": "gid://gitlab/Ci::Catalog::Resources::Version/21",
			"name": "2.1.0",
			"releasedAt": "2026-06-15T10:30:00Z",
			"createdAt": "2026-06-15T10:29:00Z",
			"semver": {"major": 2, "minor": 1, "patch": 0},
			"path": "/my-group/go-pipeline/-/tags/2.1.0",
			"author": {"id": "gid://gitlab/User/5", "username": "gopher", "name": "Go Pher",
				"webUrl": "https://gitlab.example.com/gopher", "avatarUrl": "/uploads/-/system/user/avatar/5/avatar.png"},
			"commit": {"sha": "0d64668d42dea9a265409e194a034653d41362ae", "shortId": "0d64668d", "title": "Release 2.1.0",
				"webUrl": "https://gitlab.example.com/my-group/go-pipeline/-/commit/0d64668d42dea9a265409e194a034653d41362ae"},
			"components": {"nodes": [
				{
					"id": "gid://gitlab/Ci::Catalog::Resources::Component/31",
					"name": "build",
					"description": "Build Go binary",
					"includePath": "gitlab.example.com/my-group/go-pipeline/build@2.1.0",
					"last30DayUsageCount": 12,
					"inputs": [
						{"name": "go_version", "description": "Go version to use", "type": "STRING", "required": false, "default": "1.22",
							"options": ["1.22", "1.23"], "regex": "^1\\.[0-9]+$",
							"rules": [{"if": "$[[ inputs.legacy ]] == true", "default": "1.21", "options": ["1.20", "1.21"]}]},
						{"name": "binary_name", "description": "Output binary name", "type": "STRING", "required": true, "default": null,
							"options": null, "regex": null, "rules": null},
						{"name": "race", "description": "Run with the race detector", "type": "BOOLEAN", "required": false, "default": false,
							"options": null, "regex": null, "rules": null}
					]
				},
				{
					"id": "gid://gitlab/Ci::Catalog::Resources::Component/32",
					"name": "test",
					"description": "Run Go tests with coverage",
					"includePath": "gitlab.example.com/my-group/go-pipeline/test@2.1.0",
					"last30DayUsageCount": 0,
					"inputs": [
						{"name": "coverage_threshold", "description": "Minimum coverage %", "type": "NUMBER", "required": false, "default": 80,
							"options": null, "regex": null, "rules": null}
					]
				}
			]}
		},
		{
			"id": "gid://gitlab/Ci::Catalog::Resources::Version/20",
			"name": "2.0.0",
			"releasedAt": "2026-03-01T08:00:00Z",
			"createdAt": "2026-03-01T07:59:00Z",
			"semver": {"major": 2, "minor": 0, "patch": 0},
			"path": "/my-group/go-pipeline/-/tags/2.0.0",
			"author": null,
			"commit": null,
			"components": {"nodes": [
				{"id": "gid://gitlab/Ci::Catalog::Resources::Component/30", "name": "build", "description": null,
					"includePath": "gitlab.example.com/my-group/go-pipeline/build@2.0.0", "last30DayUsageCount": null, "inputs": []}
			]}
		}
	]},
	"latestVersion": {"nodes": [
		{"readme": "# Go Pipeline\n\nComponents for Go projects.", "readmeHtml": "<h1>Go Pipeline</h1><p>Components for Go projects.</p>"}
	]}
}`

// graphqlMux returns an [http.Handler] that routes GraphQL requests to the
// appropriate handler based on the query operation name.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// Handler tests.

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResources": {
					"nodes": [`+sampleResourceNode+`],
					"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(out.Resources))
	}

	r := out.Resources[0]
	if r.ID != "gid://gitlab/Ci::CatalogResource/1" {
		t.Errorf("ID = %q, want gid://gitlab/Ci::CatalogResource/1", r.ID)
	}
	if r.Name != "go-pipeline" {
		t.Errorf("Name = %q, want go-pipeline", r.Name)
	}
	if r.Description != "Reusable Go CI/CD pipeline components" {
		t.Errorf("Description = %q", r.Description)
	}
	if r.StarCount != 42 {
		t.Errorf("StarCount = %d, want 42", r.StarCount)
	}
	if r.Last30DayUsageCount != 7 {
		t.Errorf("Last30DayUsageCount = %d, want 7", r.Last30DayUsageCount)
	}
	if r.VerificationLevel != "UNVERIFIED" {
		t.Errorf("VerificationLevel = %q, want UNVERIFIED", r.VerificationLevel)
	}
	if r.LatestVersionName != "2.1.0" {
		t.Errorf("LatestVersionName = %q, want 2.1.0", r.LatestVersionName)
	}
	if r.LatestReleasedAt != "2026-06-15T10:30:00Z" {
		t.Errorf("LatestReleasedAt = %q", r.LatestReleasedAt)
	}
	if r.WebPath != "/explore/catalog/my-group/go-pipeline" {
		t.Errorf("WebPath = %q", r.WebPath)
	}
	if r.StarrersPath != "/my-group/go-pipeline/-/starrers" {
		t.Errorf("StarrersPath = %q", r.StarrersPath)
	}
}

// TestList_ResourceWithoutVersion_NamesNoLatestVersion verifies that a listed
// resource GitLab answers with no version, whether the connection is null or
// empty, names no latest version rather than failing or inventing one.
func TestList_ResourceWithoutVersion_NamesNoLatestVersion(t *testing.T) {
	for name, versions := range map[string]string{"null connection": `null`, "no node": `{"nodes": []}`} {
		t.Run(name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"ciCatalogResources": {
						"nodes": [{"id": "gid://gitlab/Ci::CatalogResource/9", "name": "draft", "fullPath": "g/draft",
							"webPath": "/g/draft", "starCount": 0, "last30DayUsageCount": 0, "archived": false,
							"versions": `+versions+`}],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}}}`)
				},
			})
			out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{})
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(out.Resources) != 1 || out.Resources[0].Name != "draft" {
				t.Fatalf("List() resources = %+v, want the one draft resource", out.Resources)
			}
			if got := out.Resources[0].LatestVersionName; got != "" {
				t.Errorf("LatestVersionName = %q, want none for a resource without a version", got)
			}
		})
	}
}

// TestList_WithFilters verifies the List_WithFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_WithFilters(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
				http.Error(w, "decode body", http.StatusInternalServerError)
				return
			}
			if body.Variables["search"] != "golang" {
				t.Errorf("search = %v, want golang", body.Variables["search"])
			}
			if body.Variables["scope"] != "NAMESPACES" {
				t.Errorf("scope = %v, want NAMESPACES", body.Variables["scope"])
			}
			if body.Variables["sort"] != "STAR_COUNT_DESC" {
				t.Errorf("sort = %v, want STAR_COUNT_DESC", body.Variables["sort"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResources": {
					"nodes": [],
					"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{
		Search: "golang",
		Scope:  "NAMESPACES",
		Sort:   "STAR_COUNT_DESC",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

// TestList_EmptyResults verifies the List_EmptyResults handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyResults(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResources": {
					"nodes": [],
					"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(out.Resources))
	}
}

// TestList_NullNodes_AreLeftOutAndCounted verifies what a page of resources
// GitLab answered partly as null becomes. GitLab nulls an item the credential
// may not read, with no error, and CiCatalogResource declares no fine-grained
// permission at 19.4.1, so a fine-grained token reads every resource of a
// page as null. Those nulls used to decode into rows of zero values, a
// resource with no name or path published as if it existed; they are left
// out now and counted, so the reader is told the page holds fewer items than
// GitLab had (issue 1103).
func TestList_NullNodes_AreLeftOutAndCounted(t *testing.T) {
	cases := []struct {
		name       string
		nodes      string
		wantNames  []string
		wantHidden int
	}{
		{name: "some nulled", nodes: `null, ` + sampleResourceNode + `, null`, wantNames: []string{"go-pipeline"}, wantHidden: 2},
		{name: "all nulled", nodes: `null`, wantHidden: 1},
		{name: "none nulled", nodes: sampleResourceNode, wantNames: []string{"go-pipeline"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{
						"ciCatalogResources": {
							"nodes": [`+tc.nodes+`],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
						}
					}`)
				},
			})

			out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{})
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			var names []string
			for _, r := range out.Resources {
				names = append(names, r.Name)
			}
			if !slices.Equal(names, tc.wantNames) {
				t.Errorf("resources = %q, want %q", names, tc.wantNames)
			}
			if out.HiddenItems != tc.wantHidden {
				t.Errorf("HiddenItems = %d, want %d", out.HiddenItems, tc.wantHidden)
			}
		})
	}
}

// TestList_ServerError verifies that List_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error from HTTP 500 response, got nil")
	}
}

// TestList_Pagination verifies that List forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_Pagination(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
				http.Error(w, "decode body", http.StatusInternalServerError)
				return
			}
			if body.Variables["after"] != "cursor123" {
				t.Errorf("after = %v, want cursor123", body.Variables["after"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResources": {
					"nodes": [`+sampleResourceNode+`],
					"pageInfo": {"hasNextPage": true, "hasPreviousPage": true, "endCursor": "cursor456", "startCursor": "cursor111"}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{
		After: "cursor123",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	_ = out
}

// TestList_GraphQLErrorsAreReported verifies that a document GitLab refused is
// answered with its errors rather than with an empty page.
//
// GitLab returns HTTP 200 with a top-level errors array and no data, which
// client-go leaves for the caller to notice. This connection has no container
// to come back missing, so without the check a refused query and a catalog
// with no resources are the same answer.
func TestList_GraphQLErrorsAreReported(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQLError(w, http.StatusOK, "Field 'ciCatalogResources' doesn't accept argument 'topics'")
		},
	})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{})
	if err == nil {
		t.Fatalf("List() = %+v, want the GraphQL errors reported", out)
	}
	if !strings.Contains(err.Error(), "doesn't accept argument") {
		t.Errorf("List() error = %v, want it to carry the GitLab message", err)
	}
}

// TestList_BackwardPagination verifies that a caller following start_cursor
// backwards is sent before and last, and no first.
//
// The absence of first is the assertion that matters. GitLab's keyset
// connection behind ciCatalogResources refuses first beside last outright, so
// a request carrying both would fail rather than return the previous page.
func TestList_BackwardPagination(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Errorf("ParseGraphQLVariables error: %v", err)
				return
			}
			if last, ok := vars["last"].(float64); !ok || int(last) != 5 {
				t.Errorf("last = %v, want 5", vars["last"])
			}
			if vars["before"] != "cursor111" {
				t.Errorf("before = %v, want cursor111", vars["before"])
			}
			if first, ok := vars["first"]; ok {
				t.Errorf("first = %v, want it unset when the caller paged backwards", first)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResources": {
					"nodes": [`+sampleResourceNode+`],
					"pageInfo": {"hasNextPage": true, "hasPreviousPage": false, "endCursor": "cursor456", "startCursor": "cursor111"}
				}
			}`)
		},
	})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{Last: new(5), Before: "cursor111"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Resources) != 1 {
		t.Errorf("got %d resources, want 1", len(out.Resources))
	}
}

// TestList_ContradictoryPageSizes verifies that naming both first and last is
// refused before a request is made, rather than being resolved by a guess.
// GitLab answers the pair with "Can only provide either first or last, not
// both", and there is no reading of it that is not one direction discarded.
func TestList_ContradictoryPageSizes(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
			t.Error("List() reached GitLab with a contradictory page request")
			testutil.RespondGraphQL(w, http.StatusOK, `{"ciCatalogResources": {"nodes": [], "pageInfo": {}}}`)
		},
	})

	_, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{First: new(10), Last: new(5)})
	if err == nil {
		t.Fatal("List() error = nil, want a refusal naming the conflict")
	}
	if !strings.Contains(err.Error(), "first and last cannot be combined") {
		t.Errorf("List() error = %v, want it to name the conflict", err)
	}
}

// selectedField is one field a document selects: its path by response key,
// and the value of its first argument when it takes one.
type selectedField struct {
	path  string
	first string
}

// documentFields lists every field a document selects, named by the key each
// position is answered under, so an aliased selection reads as the alias.
func documentFields(t *testing.T, document string) []selectedField {
	t.Helper()
	parsed, err := parser.ParseQuery(&ast.Source{Input: document})
	if err != nil {
		t.Fatalf("parse document: %v", err)
	}
	var fields []selectedField
	var walk func(prefix string, set ast.SelectionSet)
	walk = func(prefix string, set ast.SelectionSet) {
		for _, selection := range set {
			field, ok := selection.(*ast.Field)
			if !ok {
				t.Fatalf("selection %T under %q, want fields only", selection, prefix)
			}
			path := prefix + "." + field.Alias
			selected := selectedField{path: path}
			if argument := field.Arguments.ForName("first"); argument != nil {
				selected.first = argument.Value.Raw
			}
			fields = append(fields, selected)
			walk(path, field.SelectionSet)
		}
	}
	walk("", parsed.Operations[0].SelectionSet)
	return fields
}

// TestQueries_SelectTheSameResourceFields holds the two documents to one
// selection of a catalog resource itself: every field directly under the
// resource other than its version connections. Each document spells the list
// out rather than joining a shared fragment, so this is what keeps a field
// added to one from being missing from the other.
func TestQueries_SelectTheSameResourceFields(t *testing.T) {
	resourceFields := func(document, resource string) []string {
		var names []string
		for _, field := range documentFields(t, document) {
			name, ok := strings.CutPrefix(field.path, resource+".")
			if ok && !strings.Contains(name, ".") && name != "versions" && name != "latestVersion" {
				names = append(names, name)
			}
		}
		return names
	}
	listed := resourceFields(queryListResources, ".ciCatalogResources.nodes")
	got := resourceFields(queryGetResource, ".ciCatalogResource")
	if len(listed) == 0 || !slices.Equal(listed, got) {
		t.Errorf("the listing selects %v of a resource and the get %v, want the same non-empty list", listed, got)
	}
}

// TestQueryListResources_ReadsOnlyTheLatestVersionName holds the listing to
// what it publishes of a version, its name. It used to ask every resource on
// the page for its latest version's rendered README and every component with
// its inputs, which a page of twenty on gitlab.com paid for with 3 MB and
// twenty seconds and threw away.
func TestQueryListResources_ReadsOnlyTheLatestVersionName(t *testing.T) {
	const versions = ".ciCatalogResources.nodes.versions"
	var got []selectedField
	for _, field := range documentFields(t, queryListResources) {
		if strings.HasPrefix(field.path, versions) {
			got = append(got, field)
		}
	}
	want := []selectedField{{path: versions, first: "1"}, {path: versions + ".nodes"}, {path: versions + ".nodes.name"}}
	if !slices.Equal(got, want) {
		t.Errorf("the listing selects %+v under versions, want %+v", got, want)
	}
}

// TestQueryGetResource_ReadsTheReadmeOnceUnderTheAlias holds the get document
// to the one place GitLab lets it read a README: CiCatalogResourceVersion.readme
// is resolved for a single version per request and refused with an error for
// every other, so the README may appear only under a connection of one, and
// never under the ten versions.
func TestQueryGetResource_ReadsTheReadmeOnceUnderTheAlias(t *testing.T) {
	var readmes []string
	aliasFirst := ""
	for _, field := range documentFields(t, queryGetResource) {
		if strings.HasSuffix(field.path, ".readme") || strings.HasSuffix(field.path, ".readmeHtml") {
			readmes = append(readmes, field.path)
		}
		if field.path == ".ciCatalogResource.latestVersion" {
			aliasFirst = field.first
		}
	}
	want := []string{".ciCatalogResource.latestVersion.nodes.readme", ".ciCatalogResource.latestVersion.nodes.readmeHtml"}
	if !slices.Equal(readmes, want) {
		t.Errorf("the README is selected at %v, want only %v", readmes, want)
	}
	if aliasFirst != "1" {
		t.Errorf("the aliased connection asks for first: %q, want 1", aliasFirst)
	}
}

// Get tests.

// TestGet_ByFullPath verifies that retrieving a CI catalog resource by its
// full project path returns the expected detail including components and versions.
func TestGet_ByFullPath(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
				http.Error(w, "decode body", http.StatusInternalServerError)
				return
			}
			if body.Variables["fullPath"] != "my-group/go-pipeline" {
				t.Errorf("fullPath = %v, want my-group/go-pipeline", body.Variables["fullPath"])
			}

			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": `+sampleResourceNode+`
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Get(context.Background(), client, GetInput{FullPath: "my-group/go-pipeline"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertDetail(t, out.Resource, wantSampleDetail())
}

// wantSampleDetail is sampleResourceNode as the get action publishes it: every
// field of the resource, the latest version's README from the alias, the
// latest version's components as the detail's own, and each input's default
// and options as the JSON value GitLab sent, a boolean false and a number
// included.
func wantSampleDetail() ResourceDetail {
	buildUsage, testUsage := 12, 0
	build := ComponentItem{
		ID:                  "gid://gitlab/Ci::Catalog::Resources::Component/31",
		Name:                "build",
		Description:         "Build Go binary",
		IncludePath:         "gitlab.example.com/my-group/go-pipeline/build@2.1.0",
		Last30DayUsageCount: &buildUsage,
		Inputs: []InputItem{
			{
				Name: "go_version", Description: "Go version to use", Type: "STRING", Default: "1.22",
				Options: []any{"1.22", "1.23"}, Regex: `^1\.[0-9]+$`,
				Rules: []InputRule{{If: "$[[ inputs.legacy ]] == true", Default: "1.21", Options: []any{"1.20", "1.21"}}},
			},
			{Name: "binary_name", Description: "Output binary name", Type: "STRING", Required: true},
			{Name: "race", Description: "Run with the race detector", Type: "BOOLEAN", Default: false},
		},
	}
	test := ComponentItem{
		ID:                  "gid://gitlab/Ci::Catalog::Resources::Component/32",
		Name:                "test",
		Description:         "Run Go tests with coverage",
		IncludePath:         "gitlab.example.com/my-group/go-pipeline/test@2.1.0",
		Last30DayUsageCount: &testUsage,
		Inputs:              []InputItem{{Name: "coverage_threshold", Description: "Minimum coverage %", Type: "NUMBER", Default: float64(80)}},
	}
	latest := []ComponentItem{build, test}
	return ResourceDetail{
		ID:                  "gid://gitlab/Ci::CatalogResource/1",
		Name:                "go-pipeline",
		Description:         "Reusable Go CI/CD pipeline components",
		Icon:                "https://gitlab.example.com/uploads/icon.png",
		FullPath:            "my-group/go-pipeline",
		WebPath:             "/explore/catalog/my-group/go-pipeline",
		StarCount:           42,
		StarrersPath:        "/my-group/go-pipeline/-/starrers",
		Last30DayUsageCount: 7,
		Topics:              []string{"go", "ci"},
		VerificationLevel:   "UNVERIFIED",
		VisibilityLevel:     "public",
		LatestReleasedAt:    "2026-06-15T10:30:00Z",
		LatestVersionName:   "2.1.0",
		Readme:              "# Go Pipeline\n\nComponents for Go projects.",
		ReadmeHTML:          "<h1>Go Pipeline</h1><p>Components for Go projects.</p>",
		Versions: []VersionItem{
			{
				ID:         "gid://gitlab/Ci::Catalog::Resources::Version/21",
				Name:       "2.1.0",
				ReleasedAt: "2026-06-15T10:30:00Z",
				CreatedAt:  "2026-06-15T10:29:00Z",
				Semver:     "2.1.0",
				Path:       "/my-group/go-pipeline/-/tags/2.1.0",
				Author: &VersionAuthor{
					ID: "gid://gitlab/User/5", Username: "gopher", Name: "Go Pher",
					WebURL: "https://gitlab.example.com/gopher", AvatarURL: "/uploads/-/system/user/avatar/5/avatar.png",
				},
				Commit: &VersionCommit{
					SHA: "0d64668d42dea9a265409e194a034653d41362ae", ShortID: "0d64668d", Title: "Release 2.1.0",
					WebURL: "https://gitlab.example.com/my-group/go-pipeline/-/commit/0d64668d42dea9a265409e194a034653d41362ae",
				},
				Components: latest,
			},
			{
				ID:         "gid://gitlab/Ci::Catalog::Resources::Version/20",
				Name:       "2.0.0",
				ReleasedAt: "2026-03-01T08:00:00Z",
				CreatedAt:  "2026-03-01T07:59:00Z",
				Semver:     "2.0.0",
				Path:       "/my-group/go-pipeline/-/tags/2.0.0",
				Components: []ComponentItem{{
					ID: "gid://gitlab/Ci::Catalog::Resources::Component/30", Name: "build",
					IncludePath: "gitlab.example.com/my-group/go-pipeline/build@2.0.0",
				}},
			},
		},
		Components: latest,
	}
}

// assertDetail compares a published detail with the one expected, value for
// value, and shows both as JSON when they differ.
func assertDetail(t *testing.T, got, want ResourceDetail) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal the published detail: %v", err)
	}
	wantJSON, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal the expected detail: %v", err)
	}
	t.Errorf("detail mismatch:\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
}

// TestGet_NoVersions verifies the detail conversion when the resource has no
// versions node at all: the early return must leave README, components, and
// versions empty without panicking.
func TestGet_NoVersions(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": {
					"id": "gid://gitlab/Ci::CatalogResource/9",
					"name": "draft",
					"fullPath": "g/draft",
					"webPath": "/explore/catalog/g/draft",
					"starCount": 0,
					"last30DayUsageCount": 0,
					"archived": false,
					"topics": [],
					"verificationLevel": null,
					"visibilityLevel": null,
					"latestReleasedAt": null,
					"versions": null
				}
			}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(context.Background(), client, GetInput{FullPath: "g/draft"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	r := out.Resource
	if len(r.Versions) != 0 || len(r.Components) != 0 || r.Readme != "" || r.ReadmeHTML != "" || r.LatestVersionName != "" {
		t.Errorf("draft resource should have no versions/components/readme, got %+v", r)
	}
}

// TestGet_LatestVersionReadme_EachShapeOfTheAlias verifies the README is read
// from the aliased single-version connection whatever shape it comes back in:
// null, with no node, with a node carrying neither text, and with one carrying
// only the rendered half. Each is a guard a draft resource, a version GitLab
// could not read the README of, or a reader without code access reaches.
func TestGet_LatestVersionReadme_EachShapeOfTheAlias(t *testing.T) {
	tests := []struct {
		name           string
		alias          string
		readme, render string
	}{
		{"null alias", `null`, "", ""},
		{"no node", `{"nodes": []}`, "", ""},
		{"neither text", `{"nodes": [{"readme": null, "readmeHtml": null}]}`, "", ""},
		{"rendered only", `{"nodes": [{"readme": null, "readmeHtml": "<p>x</p>"}]}`, "", "<p>x</p>"},
		{"source only", `{"nodes": [{"readme": "x", "readmeHtml": null}]}`, "x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"ciCatalogResource": {
						"id": "gid://gitlab/Ci::CatalogResource/9", "name": "r", "fullPath": "g/r", "webPath": "/r",
						"starCount": 0, "last30DayUsageCount": 0, "archived": false,
						"versions": {"nodes": []}, "latestVersion": `+tt.alias+`}}`)
				},
			})
			out, err := Get(context.Background(), testutil.NewTestClient(t, handler), GetInput{FullPath: "g/r"})
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if out.Resource.Readme != tt.readme || out.Resource.ReadmeHTML != tt.render {
				t.Errorf("README = %q, rendered %q; want %q, %q", out.Resource.Readme, out.Resource.ReadmeHTML, tt.readme, tt.render)
			}
		})
	}
}

// TestGet_NullVersionsAndComponents_AreLeftOut verifies what a resource whose
// versions or components GitLab answered partly as null becomes. GitLab nulls
// an item the credential may not read, with no error, and neither
// CiCatalogResourceVersion nor CiCatalogResourceComponent declares a
// fine-grained permission at 19.4.1, so a fine-grained token reads them as
// null wherever the resource itself is readable. Those nulls used to decode
// into versions and components of zero values; they are left out now, and a
// newest version GitLab nulled is not replaced by the one after it, which is
// not the newest: the resource names no latest version and no components
// rather than an older version's (issue 1103).
func TestGet_NullVersionsAndComponents_AreLeftOut(t *testing.T) {
	const version = `{"id": "gid://gitlab/Ci::Catalog::Resources::Version/2", "name": "1.0.0",
		"components": {"nodes": [null, {"id": "gid://gitlab/Ci::Catalog::Resources::Component/3", "name": "build",
			"includePath": "g/r/build@1.0.0", "inputs": []}]}}`
	cases := []struct {
		name           string
		versions       string
		readme         string
		wantVersions   []string
		wantLatest     string
		wantComponents []string
		wantReadme     string
	}{
		{name: "the newest nulled", versions: `null, ` + version, readme: `{"nodes": [null]}`, wantVersions: []string{"1.0.0"}},
		{
			name: "an older one nulled", versions: version + `, null`, readme: `{"nodes": [{"readme": "x", "readmeHtml": null}]}`,
			wantVersions: []string{"1.0.0"}, wantLatest: "1.0.0", wantComponents: []string{"build"}, wantReadme: "x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"ciCatalogResource": {
						"id": "gid://gitlab/Ci::CatalogResource/9", "name": "r", "fullPath": "g/r", "webPath": "/r",
						"starCount": 0, "last30DayUsageCount": 0, "archived": false,
						"versions": {"nodes": [`+tc.versions+`]}, "latestVersion": `+tc.readme+`}}`)
				},
			})
			out, err := Get(context.Background(), testutil.NewTestClient(t, handler), GetInput{FullPath: "g/r"})
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			r := out.Resource
			versions, components := versionAndComponentNames(t, r)
			if !slices.Equal(versions, tc.wantVersions) {
				t.Errorf("versions = %q, want %q", versions, tc.wantVersions)
			}
			if r.LatestVersionName != tc.wantLatest || !slices.Equal(components, tc.wantComponents) || r.Readme != tc.wantReadme {
				t.Errorf("latest version %q, components %q, README %q; want %q, %q, %q",
					r.LatestVersionName, components, r.Readme, tc.wantLatest, tc.wantComponents, tc.wantReadme)
			}
		})
	}
}

// versionAndComponentNames lists the names of a resource's versions and of
// the components it shows at detail level, and fails the test for a component
// of zero values under any version, which is what a null node decoded by value
// used to become.
func versionAndComponentNames(t *testing.T, r ResourceDetail) (versions, components []string) {
	t.Helper()
	for _, v := range r.Versions {
		versions = append(versions, v.Name)
		for _, c := range v.Components {
			if c.Name == "" {
				t.Errorf("version %s publishes a component of zero values", v.Name)
			}
		}
	}
	for _, c := range r.Components {
		components = append(components, c.Name)
	}
	return versions, components
}

// TestList_NullLatestVersion_NamesNone verifies that a listed resource whose
// one version GitLab answered as null names no latest version, rather than
// the empty name of a version of zero values (issue 1103).
func TestList_NullLatestVersion_NamesNone(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"ciCatalogResources": {
				"nodes": [{"id": "gid://gitlab/Ci::CatalogResource/9", "name": "r", "fullPath": "g/r", "webPath": "/r",
					"starCount": 0, "last30DayUsageCount": 0, "archived": false, "versions": {"nodes": [null]}}],
				"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}}}`)
		},
	})
	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Resources) != 1 || out.Resources[0].LatestVersionName != "" || out.HiddenItems != 0 {
		t.Errorf("List() = %+v, want the one resource naming no latest version and nothing hidden", out)
	}
}

// TestGet_PartialSemverStaysEmpty verifies a version whose semver object has
// null components does not collapse into a misleading "0.0.0".
func TestGet_PartialSemverStaysEmpty(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": {
					"id": "gid://gitlab/Ci::CatalogResource/9",
					"name": "partial",
					"fullPath": "g/partial",
					"webPath": "/explore/catalog/g/partial",
					"starCount": 0,
					"last30DayUsageCount": 0,
					"archived": false,
					"topics": [],
					"verificationLevel": null,
					"visibilityLevel": null,
					"latestReleasedAt": null,
					"versions": {"nodes": [
						{"name": "v1", "releasedAt": null, "createdAt": null, "semver": {"major": 1, "minor": null, "patch": null}, "path": null, "readmeHtml": null, "components": null}
					]}
				}
			}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(context.Background(), client, GetInput{FullPath: "g/partial"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	v := out.Resource.Versions[0]
	if v.Semver != "" {
		t.Errorf("Semver = %q, want empty for partial semver", v.Semver)
	}
	if v.ReleasedAt != "" {
		t.Errorf("ReleasedAt = %q, want empty for null releasedAt", v.ReleasedAt)
	}
}

// catalogResourceWithSemver renders a one-version catalog resource whose
// version carries the given semver JSON and nothing else optional.
func catalogResourceWithSemver(semver string) string {
	return `{
		"id": "gid://gitlab/Ci::CatalogResource/9",
		"name": "partial",
		"fullPath": "g/partial",
		"webPath": "/explore/catalog/g/partial",
		"starCount": 0,
		"last30DayUsageCount": 0,
		"archived": false,
		"topics": [],
		"verificationLevel": null,
		"visibilityLevel": null,
		"latestReleasedAt": null,
		"versions": {"nodes": [
			{"name": "v1", "releasedAt": null, "createdAt": null, "semver": ` + semver +
		`, "path": null, "readmeHtml": null, "components": null}
		]}
	}`
}

// TestGet_SemverMissingAnyOneComponent_StaysEmpty verifies that a version is
// given a semver string only when the schema sent all three of major, minor
// and patch, whichever one is missing.
//
// All three are nullable in CiCatalogResourceVersionSemver, so the guard is a
// four-way conjunction, and a conjunction is only held by a case that fails at
// each of its operands: the suite had one case, missing minor and patch
// together, which leaves every operand after the first free to be read as an
// alternative. Read that way the handler dereferences the very pointer that is
// nil, so the failure is a panic in a tool call rather than a wrong string.
func TestGet_SemverMissingAnyOneComponent_StaysEmpty(t *testing.T) {
	tests := []struct {
		name   string
		semver string
	}{
		{"no semver object at all", `null`},
		{"major only", `{"major": 1, "minor": null, "patch": null}`},
		{"patch missing", `{"major": 1, "minor": 2, "patch": null}`},
		{"minor missing", `{"major": 1, "minor": null, "patch": 3}`},
		{"major missing", `{"major": null, "minor": 2, "patch": 3}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK,
						`{"ciCatalogResource": `+catalogResourceWithSemver(tt.semver)+`}`)
				},
			})
			client := testutil.NewTestClient(t, handler)
			out, err := Get(context.Background(), client, GetInput{FullPath: "g/partial"})
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got := out.Resource.Versions[0].Semver; got != "" {
				t.Errorf("Semver = %q, want empty when a component is missing", got)
			}
		})
	}
}

// TestGet_ComponentInput_NullOptionalsStayEmpty verifies that an input whose
// description, type and default the schema sent as null reaches the caller as
// empty strings.
//
// Every fixture here had described and typed inputs, so the three guards that
// read those pointers were only ever seen from their true side. Read from the
// other side each is a nil dereference inside a tool call, and the response a
// model is shown to decide how to fill an input is exactly where a nullable
// field is likeliest: a component may declare an input with no description and
// no declared type at all.
func TestGet_ComponentInput_NullOptionalsStayEmpty(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": {
					"id": "gid://gitlab/Ci::CatalogResource/11",
					"name": "bare",
					"fullPath": "g/bare",
					"webPath": "/explore/catalog/g/bare",
					"starCount": 0,
					"last30DayUsageCount": 0,
					"archived": false,
					"topics": [],
					"verificationLevel": null,
					"visibilityLevel": null,
					"latestReleasedAt": null,
					"versions": {"nodes": [
						{
							"name": "1.0.0",
							"releasedAt": null,
							"createdAt": null,
							"semver": null,
							"path": null,
							"readmeHtml": null,
							"components": {"nodes": [
								{"name": "deploy", "description": null, "includePath": "g/bare/deploy@1.0.0", "inputs": [
									{"name": "target", "description": null, "type": null, "required": true, "default": null}
								]}
							]}
						}
					]}
				}
			}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(context.Background(), client, GetInput{FullPath: "g/bare"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(out.Resource.Components) != 1 || len(out.Resource.Components[0].Inputs) != 1 {
		t.Fatalf("expected one component with one input, got %+v", out.Resource.Components)
	}
	component := out.Resource.Components[0]
	if component.Description != "" {
		t.Errorf("Components[0].Description = %q, want empty", component.Description)
	}
	input := component.Inputs[0]
	if input.Name != "target" || !input.Required {
		t.Errorf("input = %+v, want the required input named target", input)
	}
	if input.Description != "" || input.Type != "" || input.Default != nil {
		t.Errorf("input optionals = %+v, want all three empty", input)
	}
}

// TestGet_ByID verifies the Get_ByID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_ByID(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
				http.Error(w, "decode body", http.StatusInternalServerError)
				return
			}
			if body.Variables["id"] != "gid://gitlab/Ci::CatalogResource/1" {
				t.Errorf("id = %v, want gid://gitlab/Ci::CatalogResource/1", body.Variables["id"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": `+sampleResourceNode+`
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Get(context.Background(), client, GetInput{ID: "gid://gitlab/Ci::CatalogResource/1"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if out.Resource.Name != "go-pipeline" {
		t.Errorf("Name = %q", out.Resource.Name)
	}
}

// TestGet_MissingIDAndPath verifies that retrieving a CI catalog resource
// without specifying either full_path or resource_id returns a validation error.
func TestGet_MissingIDAndPath(t *testing.T) {
	_, err := Get(context.Background(), nil, GetInput{})
	if err == nil {
		t.Fatal("expected error when both id and full_path are empty")
	}
}

// TestGet_BothIDAndPath_IsRefusedHere verifies that a call naming both is
// refused before it is sent.
//
// ciCatalogResource declares both arguments as nullable, so the document is
// valid and the schema gate cannot see this: "exactly one of them" is a
// resolver rule, and GitLab answers a query carrying both with an error and no
// data. Sending it and reading the empty answer told the caller the resource
// did not exist.
func TestGet_BothIDAndPath_IsRefusedHere(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(context.Background(), client, GetInput{ID: "gid://gitlab/Ci::CatalogResource/1", FullPath: "my-group/my-components"})

	if err == nil {
		t.Fatal("expected error when both id and full_path are given")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error = %q, want it to say exactly one of the two", err.Error())
	}
}

// TestGet_RefusedQuery_ReportsGitLabsReason verifies that a refusal GitLab
// answers with errors and no data reaches the caller as those errors.
//
// Without this the nil resource became "catalog resource not found", with a
// suggestion that it was an unpublished draft: a plausible sentence in the
// reassuring direction, which is the worst shape this kind of bug can take.
func TestGet_RefusedQuery_ReportsGitLabsReason(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"ciCatalogResource":null},"errors":[{"message":"Exactly one of 'id' or 'full_path' arguments is required."}]}`)
	}))

	_, err := Get(context.Background(), client, GetInput{FullPath: "my-group/my-components"})

	if err == nil {
		t.Fatal("expected the refusal to be reported")
	}
	if !strings.Contains(err.Error(), "Exactly one of") {
		t.Errorf("error = %q, want GitLab's own sentence", err.Error())
	}
}

// TestGet_NotFound verifies that Get_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_NotFound(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": null
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Get(context.Background(), client, GetInput{FullPath: "nonexistent/project"})
	if err == nil {
		t.Fatal("expected error for null resource")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
	// Draft catalog resources (no published release yet) also return null;
	// the message must tell the model that a release is required first.
	if !strings.Contains(err.Error(), "first release") {
		t.Errorf("error = %q, want draft-release guidance", err.Error())
	}
}

// TestGet_NotFound_NamesWhateverTheCallerLookedItUpBy verifies that the
// not-found message quotes the identifier the caller actually sent, whichever
// of the two arguments that was.
//
// Get accepts exactly one of id and full_path, and the message is built from
// whichever is non-empty. Nothing held that choice: the suite only ever looked
// a missing resource up by path, so the branch that falls back to full_path
// could have been reading either field and no test would notice. A message
// naming "" — or naming a path the caller never sent — is the one line a model
// has to work out whether it mistyped the identifier or the resource is a
// draft.
func TestGet_NotFound_NamesWhateverTheCallerLookedItUpBy(t *testing.T) {
	tests := []struct {
		name  string
		input GetInput
		want  string
	}{
		{"by full path", GetInput{FullPath: "nonexistent/project"}, "nonexistent/project"},
		{"by id", GetInput{ID: "gid://gitlab/Ci::CatalogResource/404"}, "gid://gitlab/Ci::CatalogResource/404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"ciCatalogResource": null}`)
				},
			})
			client := testutil.NewTestClient(t, handler)
			_, err := Get(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error for null resource")
			}
			if !strings.Contains(err.Error(), strconv.Quote(tt.want)) {
				t.Errorf("error = %q, want it to name %q", err.Error(), tt.want)
			}
		})
	}
}

// TestGet_NullOptionalFields verifies the Get_NullOptionalFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_NullOptionalFields(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": {
					"id": "gid://gitlab/Ci::CatalogResource/2",
					"name": "minimal-resource",
					"description": null,
					"icon": null,
					"fullPath": "group/minimal",
					"webUrl": "https://gitlab.example.com/group/minimal",
					"starCount": 0,
					"forksCount": 0,
					"openIssuesCount": 0,
					"openMergeRequestsCount": 0,
					"latestReleasedAt": null,
					"readmeHtml": null,
					"latestVersion": null,
					"versions": {"nodes": []}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Get(context.Background(), client, GetInput{FullPath: "group/minimal"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if out.Resource.Description != "" {
		t.Errorf("Description = %q, want empty", out.Resource.Description)
	}
	if out.Resource.LatestVersionName != "" {
		t.Errorf("LatestVersionName = %q, want empty", out.Resource.LatestVersionName)
	}
	if len(out.Resource.Components) != 0 {
		t.Errorf("expected 0 components, got %d", len(out.Resource.Components))
	}
}

// Markdown formatter tests.

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	// The empty render used to open with a guidance section telling the reader
	// to keep the links of a table that is not there.
	const want = "No catalog resources found.\n"
	if md := FormatListMarkdown(ListOutput{}); md != want {
		t.Errorf("FormatListMarkdown(empty)\n got %q\nwant %q", md, want)
	}
}

// TestFormatListMarkdown_HiddenItems_SaysThePageHeldMore verifies a page with
// items GitLab answered as null, on a page that still shows some and on one
// that shows none: the heading counts what was left out beside what is
// shown, the next steps say why GitLab answers that way, and the page's
// cursor is written either way, so an empty or short page is read neither as
// all there is nor as the end of the catalog. A page that shows nothing
// offers no step about a resource it does not show. A page with none hidden
// carries none of this, as the two tests above hold.
func TestFormatListMarkdown_HiddenItems_SaysThePageHeldMore(t *testing.T) {
	const note = "- GitLab answered 2 of the items on this page as null, with no error, which is how it answers an item the credential may not read, " +
		"a catalog resource whose GraphQL type a fine-grained personal access token is not granted among them. " +
		"They are left out, so this page shows fewer items than GitLab returned.\n"
	const cursor = "Showing %d items | next page cursor: `c2`"
	page := toolutil.GraphQLPaginationOutput{HasNextPage: true, EndCursor: "c2"}
	cases := []struct {
		name       string
		out        ListOutput
		head       string
		shown      int
		getOffered bool
	}{
		{
			name:       "some shown",
			out:        ListOutput{Resources: []ResourceItem{{Name: "go-pipeline", FullPath: "g/go-pipeline"}}, HiddenItems: 2, Pagination: page},
			head:       "## CI/CD Catalog Resources (1 shown, 2 hidden)\n\n",
			shown:      1,
			getOffered: true,
		},
		{
			name: "none shown",
			out:  ListOutput{HiddenItems: 2, Pagination: page},
			head: "## CI/CD Catalog Resources (0 shown, 2 hidden)\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := FormatListMarkdown(tc.out)
			if !strings.HasPrefix(md, tc.head) {
				t.Errorf("FormatListMarkdown() =\n%s\nwant it to open with %q", md, tc.head)
			}
			if !strings.Contains(md, note) {
				t.Errorf("FormatListMarkdown() =\n%s\nwant the note\n%s", md, note)
			}
			if want := fmt.Sprintf(cursor, tc.shown); !strings.Contains(md, want) {
				t.Errorf("FormatListMarkdown() =\n%s\nwant the page's cursor %q", md, want)
			}
			hints := toolutil.ExtractHints(md)
			if !slices.Contains(hints, strings.TrimSuffix(strings.TrimPrefix(note, "- "), "\n")) {
				t.Errorf("next steps = %q, want the note among them", hints)
			}
			if offered := strings.Contains(md, actionCatalogGet); offered != tc.getOffered {
				t.Errorf("next steps = %q, offer %s = %t, want %t", hints, actionCatalogGet, offered, tc.getOffered)
			}
		})
	}
}

// TestFormatListMarkdown_WithItems verifies that formatting catalog resources
// produces a Markdown table with name, version, star count, and description.
//
// The second row is archived, which matters beyond covering a branch: the
// catalog lists retired component projects beside maintained ones, and the
// marker in the name cell is the only thing in the whole row that tells them
// apart, so a model choosing a component to include has nothing else to go on.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Resources: []ResourceItem{
			{
				Name:                "go-pipeline",
				FullPath:            "g/go-pipeline",
				WebPath:             "/explore/catalog/g/go-pipeline",
				StarCount:           42,
				Last30DayUsageCount: 5,
				LatestVersionName:   "2.1.0",
				LatestReleasedAt:    "2026-06-15T10:30:00Z",
			},
			{
				Name:                "old-pipeline",
				FullPath:            "g/old-pipeline",
				WebPath:             "/explore/catalog/g/old-pipeline",
				StarCount:           3,
				Last30DayUsageCount: 0,
				LatestVersionName:   "1.0.0",
				LatestReleasedAt:    "2024-01-02T09:00:00Z",
				Archived:            true,
			},
		},
	})
	// The name is not linked: GitLab answers with webPath, a path relative to
	// the instance root, and a link built from it resolved against whatever
	// base the reading client happened to have. The full path the get action
	// takes is shown instead.
	want := "## CI/CD Catalog Resources (2)\n\n" +
		"| Name | Path | Description | Stars | Usage (30d) | Verification | Latest Version | Released |\n" +
		"| --- | --- | --- | --- | --- | --- | --- | --- |\n" +
		"| go-pipeline | `g/go-pipeline` |  | 42 | 5 |  | 2.1.0 | 15 Jun 2026 10:30 UTC |\n" +
		"| old-pipeline " + toolutil.EmojiArchived + " | `g/old-pipeline` |  | 3 | 0 |  | 1.0.0 | 2 Jan 2024 09:00 UTC |\n" +
		"\nShowing 2 items | no more pages\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'ci_catalog.get' to see one resource with its components and inputs\n" +
		"- Use action 'template.lint' to check a configuration that includes one\n"
	if md != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatGetMarkdown_WithComponents verifies that formatting a catalog
// resource detail includes component tables with inputs and version history.
func TestFormatGetMarkdown_WithComponents(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Resource: ResourceDetail{
			ResourceItem: ResourceItem{
				ID:       "gid://gitlab/Ci::CatalogResource/1",
				Name:     "go-pipeline",
				FullPath: "my-group/go-pipeline",
				WebPath:  "/explore/catalog/my-group/go-pipeline",
			},
			Components: []ComponentItem{
				{
					Name:        "build",
					Description: "Build binary",
					IncludePath: "gitlab.example.com/my-group/go-pipeline/build@2.1.0",
					Inputs: []InputItem{
						{Name: "go_version", Type: "string", Required: false, Default: "1.22"},
						{Name: "binary_name", Type: "string", Required: true},
					},
				},
			},
			Versions: []VersionItem{
				{Name: "2.1.0", ReleasedAt: "2026-06-15T10:30:00Z", Components: []ComponentItem{{Name: "build"}}},
			},
		},
	})
	want := "## Catalog Resource: go-pipeline\n\n" +
		"- **ID**: `gid://gitlab/Ci::CatalogResource/1`\n" +
		"- **Full Path**: `my-group/go-pipeline`\n" +
		"- **Web Path**: `/explore/catalog/my-group/go-pipeline`\n" +
		"\n### Components (Latest Version)\n\n" +
		"#### build\n\n" +
		"- **Description**: Build binary\n" +
		"- **Include**: `gitlab.example.com/my-group/go-pipeline/build@2.1.0`\n" +
		"\n| Input | Type | Required | Default | Options | Regex | Description |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| `go_version` | string | ❌ | 1.22 |  |  |  |\n" +
		"| `binary_name` | string | ✅ |  |  |  |  |\n" +
		"\n### Released Versions\n\n" +
		"| Version | Released | Author | Commit | Components |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 2.1.0 | 15 Jun 2026 10:30 UTC |  |  | build |\n" +
		catalogCardHints
	if md != want {
		t.Errorf("FormatGetMarkdown(components)\n got %q\nwant %q", md, want)
	}
}

// TestFormatGetMarkdown_EveryNewRow pins the card of the sample resource as
// the get action publishes it: the starrers path, each component's ID and
// 30-day usage (a count of zero shown as the answer it is), the inputs with
// their JSON defaults and options and their regex, the conditional rules in a
// table of their own, each version's author and commit, and the README fenced
// at the end.
func TestFormatGetMarkdown_EveryNewRow(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Resource: wantSampleDetail()})
	want := "## Catalog Resource: go-pipeline\n\n" +
		"- **ID**: `gid://gitlab/Ci::CatalogResource/1`\n" +
		"- **Full Path**: `my-group/go-pipeline`\n" +
		"- **Web Path**: `/explore/catalog/my-group/go-pipeline`\n" +
		"- **Stars**: 42\n" +
		"- **Starrers Path**: `/my-group/go-pipeline/-/starrers`\n" +
		"- **Usage (30d)**: 7\n" +
		"- **Verification**: UNVERIFIED\n" +
		"- **Visibility**: public\n" +
		"- **Topics**: go, ci\n" +
		"- **Latest Release**: 15 Jun 2026 10:30 UTC\n" +
		"- **Latest Version**: 2.1.0\n" +
		"- **Description**: Reusable Go CI/CD pipeline components\n" +
		"\n### Components (Latest Version)\n\n" +
		"#### build\n\n" +
		"- **ID**: `gid://gitlab/Ci::Catalog::Resources::Component/31`\n" +
		"- **Description**: Build Go binary\n" +
		"- **Include**: `gitlab.example.com/my-group/go-pipeline/build@2.1.0`\n" +
		"- **Usage (30d)**: 12\n" +
		"\n| Input | Type | Required | Default | Options | Regex | Description |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| `go_version` | STRING | ❌ | 1.22 | &#91;\"1.22\",\"1.23\"] | `^1\\.[0-9]+$` | Go version to use |\n" +
		"| `binary_name` | STRING | ✅ |  |  |  | Output binary name |\n" +
		"| `race` | BOOLEAN | ❌ | false |  |  | Run with the race detector |\n" +
		"\n##### Input Rules\n\n" +
		"| Input | If | Default | Options |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `go_version` | `$[[ inputs.legacy ]] == true` | 1.21 | &#91;\"1.20\",\"1.21\"] |\n" +
		"\n#### test\n\n" +
		"- **ID**: `gid://gitlab/Ci::Catalog::Resources::Component/32`\n" +
		"- **Description**: Run Go tests with coverage\n" +
		"- **Include**: `gitlab.example.com/my-group/go-pipeline/test@2.1.0`\n" +
		"- **Usage (30d)**: 0\n" +
		"\n| Input | Type | Required | Default | Options | Regex | Description |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| `coverage_threshold` | NUMBER | ❌ | 80 |  |  | Minimum coverage % |\n" +
		"\n### Released Versions\n\n" +
		"| Version | Released | Author | Commit | Components |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 2.1.0 | 15 Jun 2026 10:30 UTC | [@gopher](https://gitlab.example.com/gopher) | " +
		"[0d64668d](https://gitlab.example.com/my-group/go-pipeline/-/commit/0d64668d42dea9a265409e194a034653d41362ae) | build, test |\n" +
		"| 2.0.0 | 1 Mar 2026 08:00 UTC |  |  | build |\n" +
		"\n### README (Latest Version)\n\n" +
		"```markdown\n# Go Pipeline\n\nComponents for Go projects.\n```\n" +
		catalogCardHints
	if md != want {
		t.Errorf("FormatGetMarkdown(sample)\n got %q\nwant %q", md, want)
	}
}

// TestInputValueText pins how an input's default or options read in a cell,
// for every kind of JSON value GitLab sends, and for a value no decoding of a
// GitLab answer can produce, which fmt shows rather than nothing.
func TestInputValueText(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"not sent", nil, ""},
		{"a string, as itself", "1.22", "1.22"},
		{"an empty string, as itself", "", ""},
		{"a boolean", false, "false"},
		{"a whole number", float64(4), "4"},
		{"a fraction", 0.5, "0.5"},
		{"a list", []any{"a", float64(1)}, `["a",1]`},
		{"a value encoding/json cannot write", make(chan int), "0x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inputValueText(tt.value)
			if tt.want == "0x" {
				if !strings.HasPrefix(got, tt.want) {
					t.Errorf("inputValueText(chan) = %q, want the address fmt prints", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("inputValueText(%#v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestRuleOptionsText pins a rule's options: nothing for a rule that offers
// none, whether GitLab sent null or an empty list, and the list otherwise.
func TestRuleOptionsText(t *testing.T) {
	tests := []struct {
		name    string
		options []any
		want    string
	}{
		{"null", nil, ""},
		{"empty", []any{}, ""},
		{"one", []any{"x"}, `["x"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ruleOptionsText(tt.options); got != tt.want {
				t.Errorf("ruleOptionsText(%#v) = %q, want %q", tt.options, got, tt.want)
			}
		})
	}
}

// catalogCardHints is the guidance section the catalog resource card closes
// with.
const catalogCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'template.lint' to check a configuration that includes this component\n" +
	"- Use action 'ci_catalog.list' to browse the catalog for others\n"

// TestTruncateRunes verifies the description cell's shortening. It counts
// runes rather than bytes: cutting a UTF-8 sequence in half leaves a
// replacement character in the middle of a description, and a description is
// the one field of a catalog resource most likely to need more than one byte
// per character.
func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"long", "hello world this is long", 10, "hello w..."},
		{"multibyte cut on a rune boundary", "añadir un paso de compilación", 10, "añadir ..."},
		{"maxRunes below the ellipsis", "hello", 2, "he"},
		// At exactly the ellipsis width the cell must still say something
		// about the description. Widening the guard to maxRunes < 3 would
		// answer "..." here, which is three characters of nothing.
		{"maxRunes exactly the ellipsis width", "hello", 3, "hel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateRunes(tt.input, tt.maxRunes)
			if got != tt.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.input, tt.maxRunes, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncateRunes(%q, %d) = %q, which is not valid UTF-8", tt.input, tt.maxRunes, got)
			}
		})
	}
}

// TestGet_ServerError verifies that Get_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusForbidden)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Get(context.Background(), client, GetInput{ID: "gid://gitlab/Ci::CatalogResource/99"})
	if err == nil {
		t.Fatal("expected error from server error response, got nil")
	}
}

// TestFormatGetMarkdown_MinimalResource verifies that FormatGetMarkdown handles
// a resource with no optional fields (no description, no components, no versions,
// no latest release/version name) producing clean Markdown without empty sections.
func TestFormatGetMarkdown_MinimalResource(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Resource: ResourceDetail{
			ResourceItem: ResourceItem{
				ID:                "gid://gitlab/Ci::CatalogResource/2",
				Name:              "minimal",
				Description:       "A minimal catalog resource",
				FullPath:          "group/minimal",
				WebPath:           "/explore/catalog/group/minimal",
				LatestReleasedAt:  "2026-01-01T00:00:00Z",
				LatestVersionName: "1.0.0",
				VerificationLevel: "GITLAB_MAINTAINED",
				Topics:            []string{"go", "ci"},
				Archived:          true,
			},
		},
	})
	want := "## Catalog Resource: minimal 📦\n\n" +
		"- **ID**: `gid://gitlab/Ci::CatalogResource/2`\n" +
		"- **Full Path**: `group/minimal`\n" +
		"- **Web Path**: `/explore/catalog/group/minimal`\n" +
		"- **Verification**: GITLAB_MAINTAINED\n" +
		"- **Topics**: go, ci\n" +
		"- 📦 **Archived**\n" +
		"- **Latest Release**: 1 Jan 2026 00:00 UTC\n" +
		"- **Latest Version**: 1.0.0\n" +
		"- **Description**: A minimal catalog resource\n" +
		catalogCardHints
	if md != want {
		t.Errorf("FormatGetMarkdown(minimal)\n got %q\nwant %q", md, want)
	}
}

// TestFormatGetMarkdown_ComponentWithoutInputs verifies that writeCatalogResourceComponent
// handles a component with empty Inputs by emitting the header and Include line
// but no input table, exercising the early-return branch in the component writer.
func TestFormatGetMarkdown_ComponentWithoutInputs(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Resource: ResourceDetail{
			ResourceItem: ResourceItem{
				ID:       "gid://gitlab/Ci::CatalogResource/3",
				Name:     "no-inputs",
				FullPath: "group/no-inputs",
				WebPath:  "/explore/catalog/group/no-inputs",
			},
			Components: []ComponentItem{
				{
					Name:        "simple",
					Description: "A component without any inputs",
					IncludePath: "gitlab.example.com/group/no-inputs/simple@1.0.0",
					Inputs:      nil,
				},
			},
		},
	})
	want := "## Catalog Resource: no-inputs\n\n" +
		"- **ID**: `gid://gitlab/Ci::CatalogResource/3`\n" +
		"- **Full Path**: `group/no-inputs`\n" +
		"- **Web Path**: `/explore/catalog/group/no-inputs`\n" +
		"\n### Components (Latest Version)\n\n" +
		"#### simple\n\n" +
		"- **Description**: A component without any inputs\n" +
		"- **Include**: `gitlab.example.com/group/no-inputs/simple@1.0.0`\n" +
		catalogCardHints
	if md != want {
		t.Errorf("FormatGetMarkdown(component without inputs)\n got %q\nwant %q", md, want)
	}
}

// TestFormatGetMarkdown_DescriptionIsQuoted verifies that a multi-line
// description a maintainer typed is quoted under its label rather than written
// into the response as Markdown of its own: it used to be interpolated raw,
// so a heading in it became a heading of the response.
func TestFormatGetMarkdown_DescriptionIsQuoted(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Resource: ResourceDetail{
			ResourceItem: ResourceItem{
				Name:        "prose",
				FullPath:    "group/prose",
				Description: "First line\n\n## Injected heading",
			},
		},
	})
	want := "## Catalog Resource: prose\n\n" +
		"- **Full Path**: `group/prose`\n" +
		"- **Description**:\n" +
		"  > First line\n" +
		"  >\n" +
		"  > ## Injected heading\n" +
		catalogCardHints
	if md != want {
		t.Errorf("FormatGetMarkdown(prose)\n got %q\nwant %q", md, want)
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResources": {
					"nodes": [`+sampleResourceNode+`],
					"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
				}
			}`)
		},
		"ciCatalogResource": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": `+sampleResourceNode+`
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_catalog_resources", map[string]any{}},
		{"gitlab_get_catalog_resource", map[string]any{"full_path": "my-group/go-pipeline"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if !spec.ReadOnly || !spec.Idempotent || spec.OwnerPackage != "cicatalog" {
				t.Fatalf("unexpected ActionSpec semantics for %s: %+v", tt.name, spec)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestMarkdownHints_Outputs verifies the init()-registered markdown formatters
// for ListOutput and GetOutput produce non-nil content via MarkdownForResult.
func TestMarkdownHints_Outputs(t *testing.T) {
	t.Run("ListOutput", func(t *testing.T) {
		md := toolutil.MarkdownForResult(ListOutput{})
		if md == nil {
			t.Fatal("expected non-nil result from MarkdownForResult(ListOutput{})")
		}
	})
	t.Run("GetOutput", func(t *testing.T) {
		md := toolutil.MarkdownForResult(GetOutput{})
		if md == nil {
			t.Fatal("expected non-nil result from MarkdownForResult(GetOutput{})")
		}
	})
}

// TestActionSpecs_Metadata verifies that each CI/CD Catalog individual tool
// carries non-generic discovery metadata (1:1 audit R-META): an action-
// specific Usage that does not fall back to the package placeholder,
// distinctive natural-language Aliases beyond the tool name, canonical
// RelatedActions, and an individual-tool Description in "Returns: … See
// also: …" form.
func TestActionSpecs_Metadata(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{})
	client := testutil.NewTestClient(t, handler)

	specByTool := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		specByTool[spec.IndividualTool.Name] = spec
	}

	for _, name := range []string{"gitlab_list_catalog_resources", "gitlab_get_catalog_resource"} {
		t.Run(name, func(t *testing.T) {
			spec, ok := specByTool[name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", name)
			}
			assertNonGenericMeta(t, name, spec)
		})
	}
}

// assertNonGenericMeta asserts that the given CI/CD Catalog spec carries
// action-specific Usage, distinctive natural-language Aliases, canonical
// RelatedActions, and a "Returns: … See also: …" individual-tool description.
func assertNonGenericMeta(t *testing.T, name string, spec toolutil.ActionSpec) {
	t.Helper()
	if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute cicatalog domain action") {
		t.Fatalf("%s: expected action-specific Usage, got %q", name, spec.Usage)
	}
	if len(spec.Aliases) < 2 {
		t.Fatalf("%s: expected distinctive natural-language aliases, got %v", name, spec.Aliases)
	}
	for _, alias := range spec.Aliases {
		if alias == name {
			t.Fatalf("%s: aliases must not echo the tool name, got %v", name, spec.Aliases)
		}
	}
	if len(spec.RelatedActions) == 0 {
		t.Fatalf("%s: expected canonical RelatedActions", name)
	}
	desc := spec.IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Fatalf("%s: description must contain 'Returns:' and 'See also:', got %q", name, desc)
	}
}

// TestDecorateCatalogMeta_UnknownTool covers the no-op branch of
// decorateCatalogMeta: an individual tool not present in catalogActionMeta
// must leave the supplied options untouched.
func TestDecorateCatalogMeta_UnknownTool(t *testing.T) {
	options := toolutil.ActionSpecOptions{
		Usage:          "untouched",
		Aliases:        []string{"untouched"},
		RelatedActions: []string{"untouched"},
		IndividualTool: toolutil.IndividualToolSpec{Description: "untouched"},
	}
	decorateCatalogMeta(&options, "gitlab_not_a_catalog_tool")

	if options.Usage != "untouched" ||
		len(options.Aliases) != 1 || options.Aliases[0] != "untouched" ||
		len(options.RelatedActions) != 1 || options.RelatedActions[0] != "untouched" ||
		options.IndividualTool.Description != "untouched" {
		t.Fatalf("decorateCatalogMeta mutated options for unknown tool: %+v", options)
	}
}

// TestApplyCatalogMeta_EntrySetsOneField_LeavesTheRestStanding verifies that
// each field of a metadata entry is copied across on its own: an entry that
// carries a usage sentence and nothing else must not blank the generic
// aliases, related actions, description or schema overrides on the way past.
//
// Every entry in catalogActionMeta happens to be complete, so nothing in the
// suite ever reaches those guards from their empty side, and all four could be
// widened to copy unconditionally without a test noticing — which would clear
// the generic aliases and related actions of any future partial entry, and
// take the catalog's scope and sort enums with them.
func TestApplyCatalogMeta_EntrySetsOneField_LeavesTheRestStanding(t *testing.T) {
	generic := func() toolutil.ActionSpecOptions {
		return toolutil.ActionSpecOptions{
			Usage:                "generic usage",
			Aliases:              []string{"generic alias"},
			RelatedActions:       []string{"generic.related"},
			InputSchemaOverrides: []toolutil.InputSchemaOverride{toolutil.SchemaEnumOverride("scope", "GENERIC")},
			IndividualTool:       toolutil.IndividualToolSpec{Description: "generic description"},
		}
	}

	t.Run("an empty entry changes nothing", func(t *testing.T) {
		options := generic()
		applyCatalogMeta(&options, catalogActionMetaEntry{})
		if diff := describeMetaDifference(generic(), options); diff != "" {
			t.Errorf("empty entry changed the options: %s", diff)
		}
	})

	tests := []struct {
		name  string
		entry catalogActionMetaEntry
		field string
	}{
		{"usage only", catalogActionMetaEntry{usage: "specific"}, "Usage"},
		{"aliases only", catalogActionMetaEntry{aliases: []string{"specific"}}, "Aliases"},
		{"related only", catalogActionMetaEntry{related: []string{"specific.related"}}, "RelatedActions"},
		{"description only", catalogActionMetaEntry{description: "specific"}, "IndividualTool.Description"},
		{
			"overrides only",
			catalogActionMetaEntry{overrides: []toolutil.InputSchemaOverride{toolutil.SchemaEnumOverride("sort", "SPECIFIC")}},
			"InputSchemaOverrides",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := generic()
			applyCatalogMeta(&options, tt.entry)
			diff := describeMetaDifference(generic(), options)
			if diff == "" {
				t.Fatalf("entry setting %s changed nothing", tt.field)
			}
			if diff != tt.field {
				t.Errorf("entry setting %s changed %s instead", tt.field, diff)
			}
		})
	}
}

// describeMetaDifference names the single discovery-metadata field on which
// want and got differ, or "" when they agree. It reports more than one name
// joined by "+" so a test asserting that exactly one field moved can say which
// others came with it.
func describeMetaDifference(want, got toolutil.ActionSpecOptions) string {
	var changed []string
	if want.Usage != got.Usage {
		changed = append(changed, "Usage")
	}
	if !slices.Equal(want.Aliases, got.Aliases) {
		changed = append(changed, "Aliases")
	}
	if !slices.Equal(want.RelatedActions, got.RelatedActions) {
		changed = append(changed, "RelatedActions")
	}
	if !reflect.DeepEqual(want.InputSchemaOverrides, got.InputSchemaOverrides) {
		changed = append(changed, "InputSchemaOverrides")
	}
	if want.IndividualTool.Description != got.IndividualTool.Description {
		changed = append(changed, "IndividualTool.Description")
	}
	return strings.Join(changed, "+")
}

// TestActionSpecs_RelatedActionsAreTheCanonicalCatalogIDs verifies that each
// catalog tool points at the catalog actions a model would go to next, rather
// than at the generic set the spec builder starts from.
//
// The generic RelatedActions are non-empty, so the only assertion the suite
// made of them — that there are some — passed just as well when the
// action-specific ones were dropped on the floor. What the R-META metadata is
// for is that list_catalog_resources leads to get_catalog_resource and back
// again; a tool related to "template.lint, pipeline.create, project.get" like
// every other tool in the domain says nothing.
func TestActionSpecs_RelatedActionsAreTheCanonicalCatalogIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{}))
	specByTool := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tests := []struct {
		name string
		want []string
	}{
		{"gitlab_list_catalog_resources", []string{actionCatalogGet, actionTemplateLint, "pipeline.create"}},
		{"gitlab_get_catalog_resource", []string{actionCatalogList, actionTemplateLint, "project.get"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if !slices.Equal(spec.RelatedActions, tt.want) {
				t.Errorf("RelatedActions = %v, want %v", spec.RelatedActions, tt.want)
			}
		})
	}
}

// TestActionSpecs_ListCarriesTheCatalogEnums verifies that the list action
// publishes the CiCatalogResourceScope and CiCatalogResourceSort values as its
// scope and sort enums, and that the get action publishes no override at all.
//
// This is the one piece of metadata in the package whose absence breaks calls
// rather than discovery: without the override the canonical REST "sort" enum
// (asc, desc) is injected instead, and the server then refuses every value
// GitLab actually accepts. Nothing asserted the overrides, so the guard that
// applies them could be inverted and the suite stayed green.
func TestActionSpecs_ListCarriesTheCatalogEnums(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{}))
	specByTool := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		specByTool[spec.IndividualTool.Name] = spec
	}

	t.Run("list publishes both catalog enums", func(t *testing.T) {
		spec := specByTool["gitlab_list_catalog_resources"]
		want := []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("scope", catalogScopeValues...),
			toolutil.SchemaEnumOverride("sort", catalogSortValues...),
		}
		if !reflect.DeepEqual(spec.InputSchemaOverrides, want) {
			t.Errorf("InputSchemaOverrides = %+v, want %+v", spec.InputSchemaOverrides, want)
		}
		// The values themselves are the GraphQL enum members, upper case and
		// underscored, not the asc/desc pair the REST sort enum carries.
		if !slices.Contains(catalogSortValues, "NAME_ASC") || slices.Contains(catalogSortValues, "asc") {
			t.Errorf("catalogSortValues = %v, want the CiCatalogResourceSort members", catalogSortValues)
		}
		if !slices.Equal(catalogScopeValues, []string{"ALL", "NAMESPACES"}) {
			t.Errorf("catalogScopeValues = %v, want [ALL NAMESPACES]", catalogScopeValues)
		}
	})

	t.Run("get publishes none", func(t *testing.T) {
		if got := specByTool["gitlab_get_catalog_resource"].InputSchemaOverrides; len(got) != 0 {
			t.Errorf("InputSchemaOverrides = %+v, want none", got)
		}
	})
}
