// action_specs_test.go contains canonical-route tests for release asset link actions.
package releaselinks

import (
	"context"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const releaseLinkActionJSON = `{"id":10,"name":"Binary amd64","url":"https://example.com/bin/amd64","link_type":"package","external":true,"direct_asset_url":""}`

// TestActionSpecs_CallAllRoutes exercises every release link tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, releaseLinksActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_release_link_list", map[string]any{"project_id": "42", "tag_name": "v1.0.0"}},
		{"get", "gitlab_release_link_get", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10}},
		{"create", "gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "Binary", "url": "https://example.com/bin"}},
		{"update", "gitlab_release_link_update", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10, "name": "Updated"}},
		{"delete", "gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10}},
		{"create_batch", "gitlab_release_link_create_batch", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "links": []any{map[string]any{"name": "Binary", "url": "https://example.com/bin"}}}},
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
}

// TestActionSpecs_CreateBatchGuidance verifies batch release links expose
// package-asset parameter guidance and schema descriptions.
func TestActionSpecs_CreateBatchGuidance(t *testing.T) {
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, releaseLinksActionHandler())))
	spec := byTool["gitlab_release_link_create_batch"]

	if !strings.Contains(spec.Usage, "absolute URLs returned by package publish actions") {
		t.Fatalf("Usage = %q, want package URL guidance", spec.Usage)
	}
	guidance := spec.ParameterGuidance["links"]
	if guidance.SemanticRole != "release_asset_links" {
		t.Fatalf("links SemanticRole = %q, want release_asset_links", guidance.SemanticRole)
	}
	if !containsText(guidance.CommonConfusions, "direct_asset_path") {
		t.Fatalf("links CommonConfusions = %v, want direct_asset_path guidance", guidance.CommonConfusions)
	}
	description := schemaPropertyDescription(t, spec.Route.InputSchema, "links")
	if !strings.Contains(description, "name, url, link_type, direct_asset_path") {
		t.Fatalf("links schema description = %q, want full per-link field set", description)
	}
}

// TestActionSpecs_SingleLinkURLGuidance verifies single-link create/update
// actions explain GitLab's absolute URL requirement.
func TestActionSpecs_SingleLinkURLGuidance(t *testing.T) {
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, releaseLinksActionHandler())))
	for _, toolName := range []string{"gitlab_release_link_create", "gitlab_release_link_update"} {
		t.Run(toolName, func(t *testing.T) {
			spec := byTool[toolName]
			if !strings.Contains(spec.Usage, "absolute http, https, or ftp URL") {
				t.Fatalf("Usage = %q, want absolute URL guidance", spec.Usage)
			}
			guidance := spec.ParameterGuidance["url"]
			if guidance.SemanticRole != "release_asset_absolute_url" {
				t.Fatalf("url SemanticRole = %q, want release_asset_absolute_url", guidance.SemanticRole)
			}
			if !containsText(guidance.CommonConfusions, "local file paths") {
				t.Fatalf("url CommonConfusions = %v, want local path warning", guidance.CommonConfusions)
			}
			description := schemaPropertyDescription(t, spec.Route.InputSchema, "url")
			if !strings.Contains(description, "Absolute http, https, or ftp URL") {
				t.Fatalf("url schema description = %q, want absolute URL warning", description)
			}
		})
	}
}

// TestActionSpecs_MutationErrors verifies canonical routes propagate backend errors.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		default:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		}
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_release_link_get", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 999}},
		{"gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1}},
		{"gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "asset", "url": "https://example.com/file"}},
		{"gitlab_release_link_update", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1, "name": "new-name"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_release_link_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test release link destructive confirmation.",
		Icons:       toolutil.IconLink,
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
		Name:      "gitlab_release_link_delete",
		Arguments: map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestActionSpecs_EachActionCarriesItsOwnDiscoveryMetadata asserts that the six
// actions each keep the usage text, alias and parameter guidance written for
// them, and that no two share a usage.
//
// releaseLinkOptions builds all six from one options value and then overwrites
// the parts that differ, so every one of those overwrites is an `actionName ==`
// guard on a shared variable. Invert one and an action silently takes a
// sibling's discovery text — link_update telling a model to call
// link_create_batch instead, or link_get answering with the bare default — with
// nothing in the served surface to say which action it is describing.
func TestActionSpecs_EachActionCarriesItsOwnDiscoveryMetadata(t *testing.T) {
	tests := []struct {
		tool           string
		usage          string
		alias          string
		linkIDGuidance bool
	}{
		{"gitlab_release_link_get", "Get one release asset link by link_id", "get release link", false},
		{"gitlab_release_link_list", "List asset links attached to a release tag", "list release links", false},
		{"gitlab_release_link_delete", "Delete a release asset link by link_id", "delete release link", false},
		{"gitlab_release_link_create", "use link_create_batch instead", "create release link", false},
		{"gitlab_release_link_update", "Update an existing release asset link by link_id", "update release link", true},
		{"gitlab_release_link_create_batch", "Create multiple release asset links in one call", "batch release links", false},
	}

	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, releaseLinksActionHandler())))
	seen := make(map[string]string, len(tests))
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			spec := byTool[tt.tool]
			if !strings.Contains(spec.Usage, tt.usage) {
				t.Errorf("Usage = %q, want it to contain %q", spec.Usage, tt.usage)
			}
			if !slices.Contains(spec.Aliases, tt.alias) {
				t.Errorf("Aliases = %v, want %q among them", spec.Aliases, tt.alias)
			}
			_, hasLinkID := spec.ParameterGuidance["link_id"]
			if hasLinkID != tt.linkIDGuidance {
				t.Errorf("link_id guidance present = %t, want %t", hasLinkID, tt.linkIDGuidance)
			}
		})
		if other, duplicate := seen[byTool[tt.tool].Usage]; duplicate {
			t.Errorf("%s and %s share a usage: %q", other, tt.tool, byTool[tt.tool].Usage)
		}
		seen[byTool[tt.tool].Usage] = tt.tool
	}
}

// releaseLinkDomainPrefix is what the catalog puts in front of a spec name to
// reach the canonical ID: buildReleaseActionSpecs in
// internal/tools/action_specs.go folds this package into the gitlab_release
// group, so link_list is served as release.link_list on every surface.
const releaseLinkDomainPrefix = "release."

// declaredLinkActionIDs is the set of canonical IDs this package spells as
// constants, which is the one block both action_specs.go and markdown.go read.
func declaredLinkActionIDs() map[string]bool {
	return map[string]bool{
		actionLinkCreate: true, actionLinkCreateBatch: true, actionLinkGet: true,
		actionLinkList: true, actionLinkUpdate: true, actionLinkDelete: true,
	}
}

// TestActionSpecs_LinkActionIDsAreTheOnesTheCatalogProjects asserts that the
// constants and the actions this package declares are the same set, and that no
// RelatedActions cross-link names a link action outside it.
//
// Nothing else in the repository checks these strings: the discovery audit only
// reports an empty related list, so a misspelled ID passes every gate and
// answers a model "unknown action" the moment it follows the cross-link. This
// package carried one — "release_link.list", unused, five characters from the
// real ID — until the sweep removed it.
func TestActionSpecs_LinkActionIDsAreTheOnesTheCatalogProjects(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, releaseLinksActionHandler()))
	projected := make(map[string]bool, len(specs))
	for _, spec := range specs {
		projected[releaseLinkDomainPrefix+spec.Name] = true
	}
	declared := declaredLinkActionIDs()

	t.Run("every constant names an action this package declares", func(t *testing.T) {
		for id := range declared {
			if !projected[id] {
				t.Errorf("constant %q names no action; declared: %v", id, slices.Sorted(maps.Keys(projected)))
			}
		}
	})
	t.Run("every action this package declares has a constant", func(t *testing.T) {
		for id := range projected {
			if !declared[id] {
				t.Errorf("action %q has no constant, so a hint or cross-link would spell it by hand", id)
			}
		}
	})
	t.Run("every link cross-link is one of them", func(t *testing.T) {
		for _, spec := range specs {
			for _, related := range spec.RelatedActions {
				if strings.HasPrefix(related, releaseLinkDomainPrefix+"link_") && !declared[related] {
					t.Errorf("%s relates to %q, which this package does not declare", spec.Name, related)
				}
			}
		}
	})
}

// TestActionSpecs_LinkHintsNameActionsThisPackageDeclares sweeps the IDs the
// rendered hints actually carry, which is the text a model reads and acts on.
//
// The constants are shared with the specs, so this can only fail once somebody
// writes an ID by hand in a formatter — which is exactly how the two constant
// blocks this package used to keep would have drifted apart.
func TestActionSpecs_LinkHintsNameActionsThisPackageDeclares(t *testing.T) {
	link := Output{ID: 10, Name: "Binary", URL: "https://example.com/bin", LinkType: "package"}
	rendered := strings.Join([]string{
		FormatOutputMarkdown(link),
		FormatDeletedMarkdown(DeletedOutput(link)),
		FormatListMarkdown(ListOutput{Links: []Output{link}}),
		FormatBatchMarkdown(CreateBatchOutput{Created: []Output{link}}),
	}, "\n")

	declared := declaredLinkActionIDs()
	found := regexp.MustCompile(`release\.link_[a-z_]+`).FindAllString(rendered, -1)
	if len(found) == 0 {
		t.Fatal("no release.link_* hint found; the sweep would pass over a formatter that stopped hinting")
	}
	for _, id := range found {
		if !declared[id] {
			t.Errorf("a hint names %q, which this package does not declare", id)
		}
	}
}

func releaseLinksActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+releaseLinkActionJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseLinkActionJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, releaseLinkActionJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseLinkActionJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseLinkActionJSON)
	})
	return handler
}

func releaseLinkSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
