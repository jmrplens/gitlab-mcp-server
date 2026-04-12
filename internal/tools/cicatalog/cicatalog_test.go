package cicatalog

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Sample GraphQL response payloads.

const sampleResourceNode = `{
	"id": "gid://gitlab/Ci::CatalogResource/1",
	"name": "go-pipeline",
	"description": "Reusable Go CI/CD pipeline components",
	"icon": "https://gitlab.example.com/uploads/icon.png",
	"fullPath": "my-group/go-pipeline",
	"webUrl": "https://gitlab.example.com/my-group/go-pipeline",
	"starCount": 42,
	"forksCount": 5,
	"openIssuesCount": 3,
	"openMergeRequestsCount": 1,
	"latestReleasedAt": "2025-06-15T10:30:00Z",
	"latestVersion": {
		"name": "2.1.0",
		"releasedAt": "2025-06-15T10:30:00Z",
		"components": [
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
		]
	},
	"readmeHtml": "<h1>Go Pipeline</h1><p>Components for Go projects.</p>"
}`

// graphqlMux returns an [http.Handler] that routes GraphQL requests to the
// appropriate handler based on the query operation name.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// Handler tests.

// TestList_Success verifies that listing CI catalog resources returns the
// expected items when the GraphQL API responds with valid data.
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
	if r.ForksCount != 5 {
		t.Errorf("ForksCount = %d, want 5", r.ForksCount)
	}
	if r.LatestVersionName != "2.1.0" {
		t.Errorf("LatestVersionName = %q, want 2.1.0", r.LatestVersionName)
	}
	if r.LatestReleasedAt != "2025-06-15T10:30:00Z" {
		t.Errorf("LatestReleasedAt = %q", r.LatestReleasedAt)
	}
	if r.WebURL != "https://gitlab.example.com/my-group/go-pipeline" {
		t.Errorf("WebURL = %q", r.WebURL)
	}
}

// TestList_WithFilters verifies that search and scope filters are correctly
// forwarded to the GraphQL API when listing catalog resources.
func TestList_WithFilters(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
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

// TestList_EmptyResults verifies that listing CI catalog resources returns
// an empty result set when no resources match.
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

// TestList_ServerError verifies that listing CI catalog resources propagates
// errors when the GraphQL API returns a server error.
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

// TestList_Pagination verifies that cursor-based pagination parameters
// are correctly forwarded and page info is returned in the output.
func TestList_Pagination(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResources": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
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
				t.Fatalf("decode body: %v", err)
			}
			if body.Variables["fullPath"] != "my-group/go-pipeline" {
				t.Errorf("fullPath = %v, want my-group/go-pipeline", body.Variables["fullPath"])
			}

			detailNode := sampleResourceNode[:len(sampleResourceNode)-1] + `,
				"versions": {"nodes": [
					{
						"name": "2.1.0",
						"releasedAt": "2025-06-15T10:30:00Z",
						"components": [
							{"name": "build", "description": "Build Go binary", "includePath": "gitlab.example.com/my-group/go-pipeline/build@2.1.0", "inputs": []}
						]
					},
					{
						"name": "2.0.0",
						"releasedAt": "2025-03-01T08:00:00Z",
						"components": [
							{"name": "build", "description": null, "includePath": "gitlab.example.com/my-group/go-pipeline/build@2.0.0", "inputs": []}
						]
					}
				]}
			}`

			testutil.RespondGraphQL(w, http.StatusOK, `{
				"ciCatalogResource": `+detailNode+`
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

// TestGet_ByID verifies that retrieving a CI catalog resource by its
// numeric ID returns the expected detail.
func TestGet_ByID(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"ciCatalogResource": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
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

// TestGet_NotFound verifies that retrieving a non-existent CI catalog
// resource returns an error.
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
}

// TestGet_NullOptionalFields verifies that a CI catalog resource with null
// optional fields (description, icon, readme) is handled without errors.
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

// TestFormatListMarkdown_Empty verifies that formatting an empty catalog
// resource list produces the expected no-results Markdown message.
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
				Name:              "go-pipeline",
				WebURL:            "https://gitlab.example.com/g/go-pipeline",
				StarCount:         42,
				ForksCount:        5,
				LatestVersionName: "2.1.0",
				LatestReleasedAt:  "2025-06-15T10:30:00Z",
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
				WebURL:   "https://gitlab.example.com/my-group/go-pipeline",
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
				{Name: "2.1.0", ReleasedAt: "2025-06-15T10:30:00Z"},
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

// TestTruncate verifies that the truncate helper correctly shortens strings
// exceeding the maximum length and leaves shorter strings unchanged.
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

// TestFormatDate verifies that formatDate extracts the YYYY-MM-DD portion
// from ISO 8601 timestamps and handles empty and short inputs.
func TestFormatDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"iso", "2025-06-15T10:30:00Z", "2025-06-15"},
		{"short", "2025-06", "2025-06"},
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
