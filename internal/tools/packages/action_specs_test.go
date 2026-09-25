// action_specs_test.go contains canonical-route tests for package actions.
package packages

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every package tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))
	content64 := base64.StdEncoding.EncodeToString([]byte("test-data"))
	outDir := t.TempDir()

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"publish", "gitlab_package_publish", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "content_base64": content64,
		}},
		{"download", "gitlab_package_download", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "output_path": filepath.Join(outDir, "dl.bin"),
		}},
		{"list", "gitlab_package_list", map[string]any{"project_id": "1"}},
		{"group_list", "gitlab_list_group_packages", map[string]any{"group_id": "1"}},
		{"get", "gitlab_package_get", map[string]any{"project_id": "1", "package_id": "10"}},
		{"file_list", "gitlab_package_file_list", map[string]any{"project_id": "1", "package_id": "10"}},
		{"delete", "gitlab_package_delete", map[string]any{"project_id": "1", "package_id": "10"}},
		{"file_delete", "gitlab_package_file_delete", map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"}},
		{"publish_and_link", "gitlab_package_publish_and_link", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "content_base64": content64, "tag_name": "v1.0.0",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}

	t.Run("publish_directory", func(t *testing.T) {
		pubDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(pubDir, "test.bin"), []byte("data"), 0o600); err != nil {
			t.Fatalf("write package fixture: %v", err)
		}
		result, err := byTool["gitlab_package_publish_directory"].Route.Handler(t.Context(), map[string]any{
			"project_id":      "1",
			"package_name":    testPackageName,
			"package_version": "1.0.0",
			"directory_path":  pubDir,
		})
		if err != nil {
			t.Fatalf("Route.Handler(gitlab_package_publish_directory) error: %v", err)
		}
		if result == nil {
			t.Fatal("Route.Handler(gitlab_package_publish_directory) returned nil")
		}
	})
}

// TestActionSpecs_LocalWriteClassification verifies that every package action
// carries the read-only flag its handler deserves, and download in particular
// does not: it writes a file to the machine the server runs on, and read-only
// mode, safe mode, a read_api-scoped token and any client that auto-approves
// readOnlyHint:true all key on this flag rather than on what the handler does.
func TestActionSpecs_LocalWriteClassification(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))

	tests := []struct {
		name         string
		tool         string
		wantReadOnly bool
	}{
		{name: "download writes to local disk", tool: "gitlab_package_download"},
		{name: "publish reads local disk and writes the registry", tool: "gitlab_package_publish"},
		{name: "publish_directory reads local disk", tool: "gitlab_package_publish_directory"},
		{name: "list only reads the registry", tool: "gitlab_package_list", wantReadOnly: true},
		{name: "get only reads the registry", tool: "gitlab_package_get", wantReadOnly: true},
		{name: "file_list only reads the registry", tool: "gitlab_package_file_list", wantReadOnly: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := byTool[tt.tool]
			if !ok {
				t.Fatalf("ActionSpecs() has no %s", tt.tool)
			}
			if spec.ReadOnly != tt.wantReadOnly {
				t.Errorf("%s ReadOnly = %t, want %t", tt.tool, spec.ReadOnly, tt.wantReadOnly)
			}
			if spec.Destructive {
				t.Errorf("%s Destructive = true, want false", tt.tool)
			}
		})
	}
}

// TestActionSpecs_PublishDirectoryGuidance verifies directory publishing exposes
// LLM-facing guidance for include_pattern semantics.
func TestActionSpecs_PublishDirectoryGuidance(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))
	spec := byTool["gitlab_package_publish_directory"]

	if !strings.Contains(spec.Usage, "include_pattern is one glob") {
		t.Fatalf("Usage = %q, want single-glob guidance", spec.Usage)
	}
	guidance := spec.ParameterGuidance["include_pattern"]
	if guidance.SemanticRole != "single_glob_filter" {
		t.Fatalf("include_pattern SemanticRole = %q, want single_glob_filter", guidance.SemanticRole)
	}
	if !containsText(guidance.CommonConfusions, "comma-separated filenames") {
		t.Fatalf("include_pattern CommonConfusions = %v, want comma-separated warning", guidance.CommonConfusions)
	}
	description := schemaPropertyDescription(t, spec.Route.InputSchema, "include_pattern")
	if !strings.Contains(description, "Single glob pattern") || !strings.Contains(description, "Do not pass comma-separated filenames") {
		t.Fatalf("include_pattern schema description = %q, want single-glob warning", description)
	}
}

// TestActionSpecs_ListOrderingGuidance verifies package list exposes accepted
// order_by values so models avoid unsupported GitLab API sort fields.
func TestActionSpecs_ListOrderingGuidance(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))
	spec := byTool["gitlab_package_list"]

	if !strings.Contains(spec.Usage, "created_at, name, version, or type") {
		t.Fatalf("Usage = %q, want accepted order_by guidance", spec.Usage)
	}
	guidance := spec.ParameterGuidance["order_by"]
	if guidance.SemanticRole != "package_list_sort_field" {
		t.Fatalf("order_by SemanticRole = %q, want package_list_sort_field", guidance.SemanticRole)
	}
	if !containsText(guidance.CommonConfusions, "updated_at") {
		t.Fatalf("order_by CommonConfusions = %v, want unsupported field warning", guidance.CommonConfusions)
	}
	if got := schemaPropertyEnum(t, spec.Route.InputSchema, "order_by"); !sameStringSet(got, []string{"created_at", "name", "version", "type"}) {
		t.Fatalf("order_by enum = %v, want created_at/name/version/type", got)
	}
}

// TestActionSpecs_GetRoute_NotFoundIsAResultNotAnError verifies the get route
// turns GitLab's 404 into the structured not-found output naming the package
// and the project as the caller gave them, and hands every other refusal back
// as the error it is.
func TestActionSpecs_GetRoute_NotFoundIsAResultNotAnError(t *testing.T) {
	for _, tt := range []struct {
		name           string
		status         int
		input          map[string]any
		wantIdentifier string
	}{
		{
			name: "not found", status: http.StatusNotFound,
			input: map[string]any{"project_id": "1", "package_id": "10"}, wantIdentifier: "10 in project 1",
		},
		{
			// A JSON number reaches the route as a float64, and %v printed one
			// of a million or more in exponent form: 3.1234567e+07 in project
			// 1.2345678e+07.
			name: "not found, numbers of eight digits", status: http.StatusNotFound,
			input:          map[string]any{"project_id": float64(12345678), "package_id": float64(31234567)},
			wantIdentifier: "31234567 in project 12345678",
		},
		{name: "forbidden", status: http.StatusForbidden, input: map[string]any{"project_id": "1", "package_id": "10"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"refused"}`)
			}))))
			result, err := byTool["gitlab_package_get"].Route.Handler(t.Context(), tt.input)
			if tt.wantIdentifier == "" {
				if err == nil || !toolutil.IsHTTPStatus(err, tt.status) {
					t.Fatalf("Route.Handler error = %v (result %v), want status %d", err, result, tt.status)
				}
				return
			}
			if err != nil {
				t.Fatalf("Route.Handler error = %v, want the not-found result", err)
			}
			notFound, ok := result.(packageNotFoundOutput)
			if !ok || notFound.Identifier != tt.wantIdentifier {
				t.Fatalf("Route.Handler result = %#v, want packageNotFoundOutput for %s", result, tt.wantIdentifier)
			}
		})
	}
}

// TestActionSpecs_GetGuidance verifies the get action tells a model where a
// package_id comes from, which statuses GitLab reads here, and what the id is
// not, and links the listings it comes from by their canonical IDs. The
// statuses are the half a model cannot work out for itself: package.list
// shows a package in error status by default, and this read answers 404 for
// it while it still exists, so without them the recovery is to list again
// and be handed the same package_id.
func TestActionSpecs_GetGuidance(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))
	spec := byTool["gitlab_package_get"]

	if spec.Name != actionNameGet || !strings.Contains(spec.Usage, "other versions") {
		t.Fatalf("spec %q Usage = %q, want the get action naming the other versions", spec.Name, spec.Usage)
	}
	for _, want := range []string{"default or deprecated", "error, hidden, processing or pending_destruction", "every field package.list does"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(spec.Usage, want) {
				t.Errorf("Usage = %q, want it to say %q", spec.Usage, want)
			}
		})
	}
	guidance := spec.ParameterGuidance[paramPackageID]
	if guidance.SemanticRole != "package_registry_id" || !strings.Contains(guidance.ValueSource, actionPackageList) ||
		!strings.Contains(guidance.ValueSource, "default or deprecated") {
		t.Fatalf("package_id guidance = %+v, want its role, the listing it comes from and the statuses GitLab reads", guidance)
	}
	if !containsText(guidance.CommonConfusions, "package_file_id") {
		t.Fatalf("package_id CommonConfusions = %v, want the package file id warning", guidance.CommonConfusions)
	}
	if !sameStringSet(spec.RelatedActions, []string{actionPackageList, actionPackageGroupList, actionPackageFileList, actionPackageDelete}) {
		t.Fatalf("RelatedActions = %v, want the two listings, the file listing and the delete", spec.RelatedActions)
	}
}

// TestActionSpecs_DeleteOutputs verifies delete routes preserve their success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))

	packageResult, err := byTool["gitlab_package_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_package_delete) error: %v", err)
	}
	packageOut, ok := packageResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_package_delete) returned %T, want toolutil.DeleteOutput", packageResult)
	}
	if packageOut.Message != "Successfully deleted package 10 from project 1." {
		t.Fatalf("package delete message = %q", packageOut.Message)
	}

	fileResult, err := byTool["gitlab_package_file_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_package_file_delete) error: %v", err)
	}
	fileOut, ok := fileResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_package_file_delete) returned %T, want toolutil.DeleteOutput", fileResult)
	}
	if fileOut.Message != "Successfully deleted file 20 from package 10 in project 1." {
		t.Fatalf("file delete message = %q", fileOut.Message)
	}
}

// TestActionSpecs_DeleteError verifies delete route failures propagate.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_package_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10"})
	if err == nil {
		t.Fatal("expected route error")
	}

	_, err = byTool["gitlab_package_file_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"})
	if err == nil {
		t.Fatal("expected file delete route error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := packageSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_package_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test package destructive confirmation.",
		Icons:       toolutil.IconPackage,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_package_delete",
		Arguments: map[string]any{"project_id": "1", "package_id": "10"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func packageActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc(pathPutPkg1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 1, "package_id": 10, "file_name": "app.tar.gz",
			"size": 1024, "file_sha256": "abc", "file_md5": "md5",
			"file_sha1": "sha1", "file_store": 1,
			"created_at": "2026-01-01T00:00:00Z"
		}`)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/packages/generic/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 2, "package_id": 10, "file_name": "test.bin",
			"size": 4, "file_sha256": "def", "file_md5": "md5",
			"file_sha1": "sha1", "file_store": 1
		}`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/packages/generic/my-pkg/1.0.0/app.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdrContentType, mimeOctetStream)
		w.Write([]byte("file-data"))
	})
	handler.HandleFunc("GET /api/v4/projects/1/packages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default"}]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/packages/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default","versions":[]}`)
	})
	handler.HandleFunc("GET /api/v4/groups/1/packages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default","project_id":7,"project_path":"grp/proj"}]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/packages/10/package_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":20,"package_id":10,"file_name":"app.tar.gz","size":1024,"file_sha256":"abc"}]`)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10/package_files/20", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/projects/1/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 50, "name": "app.tar.gz",
			"url": "https://example.com/pkg", "link_type": "package", "external": true
		}`)
	})
	return handler
}

func packageSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}

func schemaPropertyDescription(t *testing.T, schema map[string]any, propertyName string) string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
	}
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q = %T, want map[string]any", propertyName, properties[propertyName])
	}
	description, ok := property["description"].(string)
	if !ok {
		t.Fatalf("schema property %q description = %T, want string", propertyName, property["description"])
	}
	return description
}

func schemaPropertyEnum(t *testing.T, schema map[string]any, propertyName string) []string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing or wrong type: %T", schema["properties"])
	}
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("property %q missing or wrong type: %T", propertyName, properties[propertyName])
	}
	values, ok := property["enum"].([]any)
	if !ok {
		t.Fatalf("property %q enum missing or wrong type: %T", propertyName, property["enum"])
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, isString := value.(string)
		if !isString {
			t.Fatalf("property %q enum contains non-string value %T", propertyName, value)
		}
		result = append(result, text)
	}
	return result
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(got))
	for _, value := range got {
		seen[value] = struct{}{}
	}
	for _, value := range want {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}
