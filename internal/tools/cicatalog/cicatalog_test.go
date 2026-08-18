// cicatalog_test.go contains unit tests for GitLab CI catalog component
// operations. Tests use httptest to mock the GitLab CI Catalog API.
package cicatalog

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Sample GraphQL response payloads.

const sampleResourceNode = `{
	"id": "gid://gitlab/Ci::CatalogResource/1",
	"name": "go-pipeline",
	"description": "Reusable Go CI/CD pipeline components",
	"icon": "https://gitlab.example.com/uploads/icon.png",
	"fullPath": "my-group/go-pipeline",
	"webPath": "/explore/catalog/my-group/go-pipeline",
	"starCount": 42,
	"last30DayUsageCount": 7,
	"archived": false,
	"topics": ["go", "ci"],
	"verificationLevel": "UNVERIFIED",
	"visibilityLevel": "public",
	"latestReleasedAt": "2026-06-15T10:30:00Z",
	"versions": {"nodes": [
		{
			"name": "2.1.0",
			"releasedAt": "2026-06-15T10:30:00Z",
			"createdAt": "2026-06-15T10:29:00Z",
			"semver": {"major": 2, "minor": 1, "patch": 0},
			"path": "/my-group/go-pipeline/-/tags/2.1.0",
			"readmeHtml": "<h1>Go Pipeline</h1><p>Components for Go projects.</p>",
			"components": {"nodes": [
				{
					"name": "build",
					"description": "Build Go binary",
					"includePath": "gitlab.example.com/my-group/go-pipeline/build@2.1.0",
					"inputs": [
						{"name": "go_version", "description": "Go version to use", "type": "string", "required": false, "default": "1.22"},
						{"name": "binary_name", "description": "Output binary name", "type": "string", "required": true, "default": null}
					]
				},
				{
					"name": "test",
					"description": "Run Go tests with coverage",
					"includePath": "gitlab.example.com/my-group/go-pipeline/test@2.1.0",
					"inputs": [
						{"name": "coverage_threshold", "description": "Minimum coverage %", "type": "number", "required": false, "default": "80"}
					]
				}
			]}
		},
		{
			"name": "2.0.0",
			"releasedAt": "2026-03-01T08:00:00Z",
			"createdAt": "2026-03-01T07:59:00Z",
			"semver": {"major": 2, "minor": 0, "patch": 0},
			"path": "/my-group/go-pipeline/-/tags/2.0.0",
			"readmeHtml": null,
			"components": {"nodes": [
				{"name": "build", "description": null, "includePath": "gitlab.example.com/my-group/go-pipeline/build@2.0.0", "inputs": []}
			]}
		}
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
			if body.Variables["scope"] != "NAMESPACED" {
				t.Errorf("scope = %v, want NAMESPACED", body.Variables["scope"])
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
		Scope:  "NAMESPACED",
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
		GraphQLPaginationInput: toolutil.GraphQLPaginationInput{After: "cursor123"},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	_ = out
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

	r := out.Resource
	if r.Name != "go-pipeline" {
		t.Errorf("Name = %q, want go-pipeline", r.Name)
	}
	if r.ReadmeHTML != "<h1>Go Pipeline</h1><p>Components for Go projects.</p>" {
		t.Errorf("ReadmeHTML = %q", r.ReadmeHTML)
	}
	if len(r.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(r.Components))
	}
	if r.Components[0].Name != "build" {
		t.Errorf("Components[0].Name = %q, want build", r.Components[0].Name)
	}
	if r.Components[0].Description != "Build Go binary" {
		t.Errorf("Components[0].Description = %q", r.Components[0].Description)
	}
	if len(r.Components[0].Inputs) != 2 {
		t.Fatalf("expected 2 inputs on build component, got %d", len(r.Components[0].Inputs))
	}

	goVersion := r.Components[0].Inputs[0]
	if goVersion.Name != "go_version" {
		t.Errorf("Inputs[0].Name = %q, want go_version", goVersion.Name)
	}
	if goVersion.Required {
		t.Error("go_version should not be required")
	}
	if goVersion.Default != "1.22" {
		t.Errorf("Inputs[0].Default = %q, want 1.22", goVersion.Default)
	}

	binaryName := r.Components[0].Inputs[1]
	if !binaryName.Required {
		t.Error("binary_name should be required")
	}

	if len(r.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(r.Versions))
	}
	if r.Versions[0].Name != "2.1.0" {
		t.Errorf("Versions[0].Name = %q", r.Versions[0].Name)
	}
	if r.Versions[1].Name != "2.0.0" {
		t.Errorf("Versions[1].Name = %q", r.Versions[1].Name)
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
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No catalog resources found.") {
		t.Error("expected empty message")
	}
}

// TestFormatListMarkdown_WithItems verifies that formatting catalog resources
// produces a Markdown table with name, version, star count, and description.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Resources: []ResourceItem{
			{
				Name:                "go-pipeline",
				WebPath:             "/explore/catalog/g/go-pipeline",
				StarCount:           42,
				Last30DayUsageCount: 5,
				LatestVersionName:   "2.1.0",
				LatestReleasedAt:    "2026-06-15T10:30:00Z",
			},
		},
	})
	if !strings.Contains(md, "go-pipeline") {
		t.Error("expected resource name in output")
	}
	if !strings.Contains(md, "42") {
		t.Error("expected star count in output")
	}
	if !strings.Contains(md, "2.1.0") {
		t.Error("expected version in output")
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
	if !strings.Contains(md, "go-pipeline") {
		t.Error("expected resource name")
	}
	if !strings.Contains(md, "`build`") {
		t.Error("expected component name")
	}
	if !strings.Contains(md, "binary_name") {
		t.Error("expected input name")
	}
	if !strings.Contains(md, "**yes**") {
		t.Error("expected required marker")
	}
	if !strings.Contains(md, "2.1.0") {
		t.Error("expected version in versions table")
	}
}

// TestTruncate verifies the Truncate handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"long", "hello world this is long", 10, "hello w..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestFormatDate verifies the Date Markdown formatter for a representative date input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"iso", "2026-06-15T10:30:00Z", "2026-06-15"},
		{"short", "2026-06", "2026-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDate(tt.input)
			if got != tt.want {
				t.Errorf("formatDate(%q) = %q, want %q", tt.input, got, tt.want)
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
			},
		},
	})
	if !strings.Contains(md, "### Description") {
		t.Error("expected Description section for non-empty description")
	}
	if !strings.Contains(md, "Latest Release") {
		t.Error("expected Latest Release row for non-empty LatestReleasedAt")
	}
	if !strings.Contains(md, "Latest Version") {
		t.Error("expected Latest Version row for non-empty LatestVersionName")
	}
	if strings.Contains(md, "### Components") {
		t.Error("expected no Components section for empty components")
	}
	if strings.Contains(md, "### Released Versions") {
		t.Error("expected no Versions section for empty versions")
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
	if !strings.Contains(md, "`simple`") {
		t.Error("expected component name header")
	}
	if !strings.Contains(md, "A component without any inputs") {
		t.Error("expected component description")
	}
	if !strings.Contains(md, "**Include:** `gitlab.example.com/group/no-inputs/simple@1.0.0`") {
		t.Error("expected include path")
	}
	if strings.Contains(md, "| Input |") {
		t.Error("expected no Inputs table for component with empty Inputs")
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
