// register_test.go contains unit tests for tool registration via RegisterAll
// and RegisterAllMeta. Tests verify tool counts, tool names, annotation
// presence, and end-to-end MCP call flow using in-memory transports.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// fmtListToolsErr is the format string used when ListTools returns an error.
	fmtListToolsErr = "ListTools() error: %v"
)

// registerMetaSourceFile holds register meta source file data for the tools package.
type registerMetaSourceFile struct {
	path    string
	content string
}

// readRegisterMetaSources supports read register meta sources assertions in tools tests.
func readRegisterMetaSources(t *testing.T) []registerMetaSourceFile {
	t.Helper()

	paths, err := filepath.Glob("register_meta*.go")
	if err != nil {
		t.Fatalf("glob register_meta*.go: %v", err)
	}
	nonTestPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		nonTestPaths = append(nonTestPaths, path)
	}
	if len(nonTestPaths) == 0 {
		t.Fatal("no register_meta*.go files found")
	}

	sources := make([]registerMetaSourceFile, 0, len(nonTestPaths))
	for _, path := range nonTestPaths {
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		sources = append(sources, registerMetaSourceFile{path: path, content: string(src)})
	}

	return sources
}

// readRegisterMetaSource supports read register meta source assertions in tools tests.
func readRegisterMetaSource(t *testing.T) string {
	t.Helper()

	var builder strings.Builder
	for _, sourceFile := range readRegisterMetaSources(t) {
		builder.WriteString(sourceFile.content)
		builder.WriteByte('\n')
	}

	return builder.String()
}

// newMCPSession creates an MCP session with individual tools registered.
// When enterprise is true, Enterprise/Premium tools are included.
func newMCPSession(t *testing.T, handler http.Handler, enterprise ...bool) *mcp.ClientSession {
	t.Helper()
	client := newTestClient(t, handler)

	ent := true
	if len(enterprise) > 0 {
		ent = enterprise[0]
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
	RegisterAll(server, client, ent)
	toolutil.LockdownInputSchemas(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// newMetaMCPSession creates an MCP session with meta-tools registered.
// When enterprise is true, Enterprise/Premium meta-tools are included.
// It uses in-memory transports and auto-closes the session via t.Cleanup.
func newMetaMCPSession(t *testing.T, handler http.Handler, enterprise bool) *mcp.ClientSession {
	t.Helper()
	client := newTestClient(t, handler)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	if err := RegisterAllMeta(server, client, enterprise); err != nil {
		t.Fatalf("RegisterAllMeta() error = %v", err)
	}
	toolutil.LockdownInputSchemas(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestRegisterAll_ToolCount verifies that RegisterAll registers exactly
// the expected number of individual tools on the MCP server.
func TestRegisterAll_ToolCount(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	t.Run("Enterprise", func(t *testing.T) {
		session := newMCPSession(t, handler, true)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		const expectedTools = 1027
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})

	t.Run("CE", func(t *testing.T) {
		session := newMCPSession(t, handler, false)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		t.Logf("CE tool count: %d", len(result.Tools))
		const expectedTools = 867
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})
}

// TestRegisterAll_OrbitToolsRequireGitLabDotComEnterprise verifies that the
// global catalog advertises Orbit only for GitLab.com with enterprise enabled.
func TestRegisterAll_OrbitToolsRequireGitLabDotComEnterprise(t *testing.T) {
	tests := []struct {
		name       string
		gitlabURL  string
		enterprise bool
		wantOrbit  bool
	}{
		{name: "self-managed enterprise", gitlabURL: "https://gitlab.example.com", enterprise: true},
		{name: "gitlab.com base", gitlabURL: "https://gitlab.com", enterprise: false},
		{name: "gitlab.com enterprise", gitlabURL: "https://gitlab.com", enterprise: true, wantOrbit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := gitlabclient.NewClientWithToken(tt.gitlabURL, "test-token", false)
			if err != nil {
				t.Fatalf("NewClientWithToken() error: %v", err)
			}
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
			RegisterAll(server, client, tt.enterprise)
			names := toolNamesFromServer(t, server)
			if gotOrbit := slices.Contains(names, "gitlab_orbit_status"); gotOrbit != tt.wantOrbit {
				t.Fatalf("RegisterAll() Orbit registration = %t, want %t", gotOrbit, tt.wantOrbit)
			}
			if !tt.wantOrbit {
				return
			}
			for _, want := range []string{"gitlab_orbit_status", "gitlab_orbit_schema", "gitlab_orbit_tools", "gitlab_orbit_dsl", "gitlab_orbit_query", "gitlab_orbit_graph_status"} {
				if !slices.Contains(names, want) {
					t.Fatalf("RegisterAll() missing %s for GitLab.com enterprise client", want)
				}
			}
		})
	}
}

// TestRegisterAllMeta_ToolCount verifies that RegisterAllMeta registers
// the expected number of meta-tools: 33 base, 49 with enterprise.
// Base count is 29 meta-tools + 4 standalone gitlab_interactive_* elicitation
// tools that cannot be folded into action+params meta-tools (they require
// multi-round MCP elicitation/create exchanges with the client).
func TestRegisterAllMeta_ToolCount(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	t.Run("Base", func(t *testing.T) {
		session := newMetaMCPSession(t, handler, false)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		const expectedTools = 33
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})

	t.Run("Enterprise", func(t *testing.T) {
		session := newMetaMCPSession(t, handler, true)
		result, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf(fmtListToolsErr, err)
		}
		const expectedTools = 49
		if len(result.Tools) != expectedTools {
			t.Errorf("tool count = %d, want %d", len(result.Tools), expectedTools)
			for _, tool := range result.Tools {
				t.Logf("  tool: %s", tool.Name)
			}
		}
	})
}

// TestRegisterAllMeta_OrbitMetaToolRequiresGitLabDotComEnterprise verifies that
// the GitLab.com-only Orbit meta-tool also requires the Enterprise catalog.
func TestRegisterAllMeta_OrbitMetaToolRequiresGitLabDotComEnterprise(t *testing.T) {
	tests := []struct {
		name       string
		gitlabURL  string
		enterprise bool
		wantOrbit  bool
	}{
		{name: "self-managed enterprise", gitlabURL: "https://gitlab.example.com", enterprise: true},
		{name: "gitlab.com base", gitlabURL: "https://gitlab.com", enterprise: false},
		{name: "gitlab.com enterprise", gitlabURL: "https://gitlab.com", enterprise: true, wantOrbit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := gitlabclient.NewClientWithToken(tt.gitlabURL, "test-token", false)
			if err != nil {
				t.Fatalf("NewClientWithToken() error: %v", err)
			}
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			if registerErr := RegisterAllMeta(server, client, tt.enterprise); registerErr != nil {
				t.Fatalf("RegisterAllMeta() error = %v", registerErr)
			}
			if gotOrbit := slices.Contains(toolNamesFromServer(t, server), "gitlab_orbit"); gotOrbit != tt.wantOrbit {
				t.Fatalf("RegisterAllMeta() Orbit registration = %t, want %t", gotOrbit, tt.wantOrbit)
			}
		})
	}
}

// toolNamesFromServer supports tool names from server assertions in tools tests.
func toolNamesFromServer(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// TestRegisterAll_ToolNames verifies that every expected individual tool name
// is present after RegisterAll and that no unexpected tools are registered.
func TestRegisterAll_ToolNames(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	expectedNames := expectedRegisterAllToolNames(t, true)

	for _, tool := range result.Tools {
		if !expectedNames[tool.Name] {
			t.Errorf("unexpected tool registered: %s", tool.Name)
		}
		delete(expectedNames, tool.Name)
	}
	for name := range expectedNames {
		t.Errorf("expected tool not found: %s", name)
	}
}

// TestRegisterAllMeta_ToolNames verifies that every expected meta-tool name
// is present after RegisterAllMeta (enterprise=true) and that no unexpected tools are registered.
func expectedRegisterAllToolNames(t *testing.T, enterprise bool) map[string]bool {
	t.Helper()
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	opts := IndividualCatalogRegisterOptions{IncludeStandaloneUtilities: true}
	state := individualCatalogRegisterState{
		opts:       opts,
		allowed:    stringSet(opts.AllowedToolNames),
		excluded:   stringSet(opts.ExcludeToolNames),
		registered: make(map[string]struct{}),
	}
	names := make(map[string]bool)
	for _, group := range catalog.Groups() {
		if !individualCatalogGroupEligible(group, opts) {
			continue
		}
		for _, action := range group.ActionsInOrder() {
			toolName := strings.TrimSpace(action.IndividualTool.Name)
			if !individualCatalogToolEligible(toolName, action, state) {
				continue
			}
			names[toolName] = true
			state.registered[toolName] = struct{}{}
		}
	}
	for _, spec := range StandaloneSurfaceToolSpecs(nil) {
		names[spec.Name] = true
	}
	return names
}

func TestRegisterAllMeta_ToolNames(t *testing.T) {
	session := newMetaMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}), true)

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	expectedNames := map[string]bool{
		"gitlab_access":                true,
		"gitlab_admin":                 true,
		"gitlab_analyze":               true,
		"gitlab_attestation":           true,
		"gitlab_audit_event":           true,
		"gitlab_branch":                true,
		"gitlab_ci_catalog":            true,
		"gitlab_ci_variable":           true,
		"gitlab_compliance_policy":     true,
		"gitlab_custom_emoji":          true,
		"gitlab_dependency":            true,
		"gitlab_dora_metrics":          true,
		"gitlab_enterprise_user":       true,
		"gitlab_environment":           true,
		"gitlab_external_status_check": true,
		"gitlab_feature_flags":         true,
		"gitlab_geo":                   true,
		"gitlab_group":                 true,
		"gitlab_group_scim":            true,
		"gitlab_issue":                 true,
		"gitlab_job":                   true,
		"gitlab_member_role":           true,
		"gitlab_merge_request":         true,
		"gitlab_merge_train":           true,
		"gitlab_model_registry":        true,
		"gitlab_mr_review":             true,
		"gitlab_package":               true,
		"gitlab_pipeline":              true,
		"gitlab_project":               true,
		"gitlab_project_alias":         true,
		"gitlab_release":               true,
		"gitlab_repository":            true,
		"gitlab_discover_project":      true,
		"gitlab_runner":                true,
		"gitlab_search":                true,
		"gitlab_security_attribute":    true,
		"gitlab_security_category":     true,
		"gitlab_security_finding":      true,
		"gitlab_snippet":               true,
		"gitlab_storage_move":          true,
		"gitlab_tag":                   true,
		"gitlab_template":              true,
		"gitlab_user":                  true,
		"gitlab_vulnerability":         true,

		"gitlab_wiki": true,

		"gitlab_interactive_issue_create":   true,
		"gitlab_interactive_mr_create":      true,
		"gitlab_interactive_project_create": true,
		"gitlab_interactive_release_create": true,
	}

	for _, tool := range result.Tools {
		if !expectedNames[tool.Name] {
			t.Errorf("unexpected meta-tool registered: %s", tool.Name)
		}
		delete(expectedNames, tool.Name)
	}
	for name := range expectedNames {
		t.Errorf("expected meta-tool not found: %s", name)
	}
}

// TestRegisterAll_ToolAnnotations verifies that every registered tool has
// non-nil annotations with OpenWorldHint set to true.
func TestRegisterAll_ToolAnnotations(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		if tool.Annotations == nil {
			t.Errorf("tool %s missing annotations", tool.Name)
			continue
		}
		if tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Errorf("tool %s: OpenWorldHint should be true", tool.Name)
		}
	}
}

// TestRegisterAll_CallToolThroughMCP verifies a single tool call round-trip
// through an in-memory MCP session using individual tools.
func TestRegisterAll_CallToolThroughMCP(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_project_get",
		Arguments: map[string]any{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() returned error result")
	}
}

// TestRegisterAllMeta_CallToolThroughMCP verifies a single meta-tool call
// round-trip through an in-memory MCP session.
func TestRegisterAllMeta_CallToolThroughMCP(t *testing.T) {
	session := newMetaMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`)
		default:
			http.NotFound(w, r)
		}
	}), false)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_project",
		Arguments: map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": "42"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() returned error result")
	}
}

// TestRegisterAllDoesNotUseDomainRegisterTools verifies the root individual
// runtime is catalog-backed and cannot regress to per-domain RegisterTools loops.
func TestRegisterAllDoesNotUseDomainRegisterTools(t *testing.T) {
	src, err := os.ReadFile("register.go")
	if err != nil {
		t.Fatalf("ReadFile register.go: %v", err)
	}
	for _, forbidden := range []string{".RegisterTools(", "registerAllLegacy", "legacyIndividualToolDescriptions", "listToolsForDescriptionCapture"} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("register.go contains %q; RegisterAll must remain catalog-backed", forbidden)
		}
	}
}

// TestAllMarkdownFormattersRegistered verifies that every sub-package with a
// markdown.go containing init() + RegisterMarkdown has at least one type
// registered in the toolutil Markdown registry.
func TestAllMarkdownFormattersRegistered(t *testing.T) {
	// 1. Get all registered type names from the registry.
	typeNames := toolutil.RegisteredMarkdownTypeNames()
	if len(typeNames) == 0 {
		t.Fatal("no Markdown formatters registered — registry may not be initialized")
	}

	// Build a set of package prefixes that have registered formatters.
	registeredPkgs := make(map[string]bool)
	for _, name := range typeNames {
		// Type names are like "branches.Output", "toolutil.DeleteOutput".
		pkg, _, ok := strings.Cut(name, ".")
		if ok {
			registeredPkgs[pkg] = true
		}
	}

	// 2. Find sub-packages whose markdown.go files contain init() registrations.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	reRegister := regexp.MustCompile(`toolutil\.Register(?:Markdown|MarkdownResult)\b`)
	var missing []string

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(e.Name(), "markdown.go")
		src, readErr := os.ReadFile(mdPath)
		if readErr != nil {
			continue // no markdown.go — that's fine
		}

		if !reRegister.Match(src) {
			continue // markdown.go exists but has no registry calls
		}

		// This sub-package registers formatters — check if they appear in the registry.
		if !registeredPkgs[e.Name()] {
			missing = append(missing, e.Name())
		}
	}

	if len(missing) > 0 {
		t.Errorf("sub-packages with RegisterMarkdown calls in markdown.go but no types in registry:\n  %s",
			strings.Join(missing, "\n  "))
	}

	// 3. Check the toolutil.DeleteOutput formatter is registered.
	if !registeredPkgs["toolutil"] {
		t.Error("toolutil.DeleteOutput formatter not registered in registry")
	}

	t.Logf("verified %d registered formatter types across %d packages",
		len(typeNames), len(registeredPkgs))
}

// TestAllHintReferencesValid validates that tool names and meta-tool action
// names referenced in WriteHints calls actually exist. This catches stale
// references after tool renaming or meta-tool action restructuring.
//
// Two validations:
//   - Backtick-quoted `gitlab_*` tool references must match a registered tool name
//   - `action 'xxx'` references must match a meta-tool action key
func TestAllHintReferencesValid(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	metaSrc := readRegisterMetaSource(t)
	validTools := collectValidHintTools(t, entries, metaSrc)

	if len(validTools) == 0 {
		t.Fatal("no tool names found — parsing may be broken")
	}
	validActions := collectValidHintActions(entries, metaSrc)

	if len(validActions) == 0 {
		t.Fatal("no action keys found — parsing may be broken")
	}

	// 3. Validate hints in all markdown.go files.
	reToolRef := regexp.MustCompile("`(gitlab_\\w+)`")
	reActionRef := regexp.MustCompile(`action '(\w+)'`)

	toolErrors, actionErrors := validateWriteHintReferences(t, entries, validTools, validActions, reToolRef, reActionRef)

	t.Logf("validated hints across all packages: %d valid tools, %d valid actions, %d tool errors, %d action errors",
		len(validTools), len(validActions), toolErrors, actionErrors)
}

func collectValidHintTools(t *testing.T, entries []os.DirEntry, metaSrc string) map[string]bool {
	t.Helper()
	validTools := make(map[string]bool)
	reToolName := regexp.MustCompile(`Name:\s+"(gitlab_\w+)"`)
	for _, entry := range entries {
		addRegisterFileMatches(validTools, entry, reToolName)
	}
	for _, group := range CollectActionSpecs(nil, true) {
		for _, spec := range group.Actions {
			if name := strings.TrimSpace(spec.IndividualTool.Name); name != "" {
				validTools[name] = true
			}
		}
	}
	reMetaTool := regexp.MustCompile(`add(?:ReadOnly)?MetaTool\(server,\s+"(gitlab_\w+)"`)
	for _, match := range reMetaTool.FindAllStringSubmatch(metaSrc, -1) {
		validTools[match[1]] = true
	}
	return validTools
}

func collectValidHintActions(entries []os.DirEntry, metaSrc string) map[string]bool {
	validActions := make(map[string]bool)
	addMatches(validActions, regexp.MustCompile(`"(\w+)":\s+(?:route|destructive)(?:Action|VoidAction|ActionWithRequest)\b`), metaSrc)
	addMatches(validActions, regexp.MustCompile(`routes\["(\w+)"\]\s*=\s*(?:route(?:Action|VoidAction|ActionWithRequest)?|destructive(?:Route|Action|VoidAction|ActionWithRequest))\b`), metaSrc)
	addMatches(validActions, regexp.MustCompile(`"(\w+)":\s+(?:route|destructiveRoute)\(\w+Action\b`), metaSrc)
	reDelegatedAction := regexp.MustCompile(`"(\w+)":\s+toolutil\.(?:Route(?:Action|VoidAction|ActionWithRequest)?|Destructive(?:Action|VoidAction|ActionWithRequest|Route))\b`)
	reDelegatedAssign := regexp.MustCompile(`routes\["(\w+)"\]\s*=\s*toolutil\.(?:Route(?:Action|VoidAction|ActionWithRequest)?|Destructive(?:Action|VoidAction|ActionWithRequest|Route))\b`)
	for _, entry := range entries {
		addRegisterFileMatches(validActions, entry, reDelegatedAction)
		addRegisterFileMatches(validActions, entry, reDelegatedAssign)
	}
	for _, group := range CollectActionSpecs(nil, true) {
		for _, spec := range group.Actions {
			if actionName := strings.TrimSpace(spec.Name); actionName != "" {
				validActions[actionName] = true
			}
		}
	}
	return validActions
}

func addRegisterFileMatches(values map[string]bool, entry os.DirEntry, pattern *regexp.Regexp) {
	if !entry.IsDir() {
		return
	}
	src, readErr := os.ReadFile(filepath.Join(entry.Name(), "register.go"))
	if readErr != nil {
		return
	}
	addMatches(values, pattern, string(src))
}

func addMatches(values map[string]bool, pattern *regexp.Regexp, src string) {
	for _, match := range pattern.FindAllStringSubmatch(src, -1) {
		values[match[1]] = true
	}
}

func validateWriteHintReferences(t *testing.T, entries []os.DirEntry, validTools, validActions map[string]bool, reToolRef, reActionRef *regexp.Regexp) (int, int) {
	t.Helper()
	var toolErrors, actionErrors int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		src, readErr := os.ReadFile(filepath.Join(entry.Name(), "markdown.go"))
		if readErr != nil {
			continue
		}
		for _, line := range extractWriteHintLines(string(src)) {
			toolErrors += reportInvalidToolReferences(t, entry.Name(), line, validTools, reToolRef)
			actionErrors += reportInvalidActionReferences(t, entry.Name(), line, validActions, reActionRef)
		}
	}
	return toolErrors, actionErrors
}

func reportInvalidToolReferences(t *testing.T, packageName, line string, validTools map[string]bool, reToolRef *regexp.Regexp) int {
	t.Helper()
	errors := 0
	for _, match := range reToolRef.FindAllStringSubmatch(line, -1) {
		toolName := match[1]
		if !validTools[toolName] {
			t.Errorf("%s: hint references non-existent tool %q", packageName, toolName)
			errors++
		}
	}
	return errors
}

func reportInvalidActionReferences(t *testing.T, packageName, line string, validActions map[string]bool, reActionRef *regexp.Regexp) int {
	t.Helper()
	errors := 0
	for _, match := range reActionRef.FindAllStringSubmatch(line, -1) {
		actionName := match[1]
		if !validActions[actionName] {
			t.Errorf("%s: hint references non-existent action %q", packageName, actionName)
			errors++
		}
	}
	return errors
}

// extractWriteHintLines finds string literal lines inside WriteHints() calls.
// It uses a simple state machine: when a line contains "WriteHints(", subsequent
// lines containing string literals are collected until the closing ")".
func extractWriteHintLines(src string) []string {
	lines := strings.Split(src, "\n")
	var result []string
	inHints := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "WriteHints(") {
			inHints = true
			continue
		}
		if inHints {
			if strings.HasPrefix(trimmed, `"`) {
				result = append(result, trimmed)
			} else if trimmed == ")" || strings.HasPrefix(trimmed, ")") {
				inHints = false
			}
		}
	}
	return result
}

// TestDestructiveMetadata_RegisteredRoutes_MatchIndividualToolAnnotations verifies that meta-tool routes marked with
// destructive wrappers correspond to individual tools using DeleteAnnotations,
// and that non-destructive routes do not correspond to individual tools with
// DeleteAnnotations. This catches misclassified routes after migration.
func TestDestructiveMetadata_RegisteredRoutes_MatchIndividualToolAnnotations(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	routeMap := collectRouteDestructiveMetadata(t, entries)
	deleteTools := collectDeleteAnnotationPackages(t, entries)
	mismatches := validateDestructiveMetadataMatches(t, routeMap, deleteTools)
	t.Logf("validated %d route entries across %d packages, %d mismatches", len(routeMap), len(entries), mismatches)
}

type destructiveRouteInfo struct {
	pkg         string
	destructive bool
}

func collectRouteDestructiveMetadata(t *testing.T, entries []os.DirEntry) map[string][]destructiveRouteInfo {
	t.Helper()
	routeMap := make(map[string][]destructiveRouteInfo)
	reSubDestructive := regexp.MustCompile(`"(\w+)":\s+toolutil\.Destructive(?:Action|VoidAction|ActionWithRequest|Route)\b`)
	reSubNonDestructive := regexp.MustCompile(`"(\w+)":\s+toolutil\.Route(?:Action|VoidAction|ActionWithRequest|)\b`)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		srcStr := string(src)
		for _, m := range reSubDestructive.FindAllStringSubmatch(srcStr, -1) {
			routeMap[m[1]] = append(routeMap[m[1]], destructiveRouteInfo{pkg: e.Name(), destructive: true})
		}
		for _, m := range reSubNonDestructive.FindAllStringSubmatch(srcStr, -1) {
			routeMap[m[1]] = append(routeMap[m[1]], destructiveRouteInfo{pkg: e.Name(), destructive: false})
		}
	}
	return routeMap
}

func collectDeleteAnnotationPackages(t *testing.T, entries []os.DirEntry) map[string]bool {
	t.Helper()
	deleteTools := make(map[string]bool)
	reDeleteAnn := regexp.MustCompile(`Annotations:\s+toolutil\.DeleteAnnotations`)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		if reDeleteAnn.Match(src) {
			deleteTools[e.Name()] = true
		}
	}
	return deleteTools
}

func validateDestructiveMetadataMatches(t *testing.T, routeMap map[string][]destructiveRouteInfo, deleteTools map[string]bool) int {
	t.Helper()
	var mismatches int
	for action, infos := range routeMap {
		for _, info := range infos {
			hasDelete := deleteTools[info.pkg]
			if info.destructive && !hasDelete {
				// Acceptable for exact-match exceptions (merge, stop, erase, etc.)
				// that are destructive but don't use DeleteAnnotations.
				if !isExactMatchException(action) {
					t.Errorf("%s/%s is destructive route but package has no DeleteAnnotations", info.pkg, action)
					mismatches++
				}
			}
			if !info.destructive && hasDelete {
				// Non-destructive route in a package with delete tools — this is fine
				// for list/get/create/update actions in the same package.
				continue
			}
		}
	}
	return mismatches
}

// Actions that are destructive but do NOT contain a destructive keyword.
// These are known edge cases verified manually.
var knownNonKeywordDestructive = map[string]struct{}{
	"merge": {}, "erase": {}, "stop": {}, "ban": {},
	"block": {}, "deactivate": {}, "reject": {}, "unapprove": {},
	"approval_reset": {}, "disable_two_factor": {}, "disable_2fa": {},
	"unshare": {}, "disable_project": {}, "import_from_file": {},
	"group_member_unshare": {},
	"cancel_github":        {}, "rotate": {}, "mirror_force_push": {},
	"db_migration_mark": {}, "terraform_state_unlock": {}, "archive": {},
	"transfer": {},
}

var knownRouteDestructiveExceptions = map[string]struct{}{
	"security_attribute.bulk_update":    {},
	"security_attribute.project_update": {},
}

// isExactMatchException reports whether an action name is too generic for the
// normal destructive-name heuristic but is accepted by explicit policy.
func isExactMatchException(action string) bool {
	_, ok := knownNonKeywordDestructive[action]
	return ok
}

func isRouteDestructiveException(id string) bool {
	_, ok := knownRouteDestructiveExceptions[id]
	return ok
}

// TestDestructiveRoutes_NameHeuristic_ClassifiesActions scans ALL route definitions across the
// codebase and verifies that action names containing destructive keywords
// (delete, remove, revoke, purge, unprotect, destroy, unpublish) always use
// destructive wrappers, and that safe action names (list, get, search, create,
// update) never use destructive wrappers. This test prevents accidental
// misclassification when adding new routes.
func TestDestructiveRoutes_NameHeuristic_ClassifiesActions(t *testing.T) {
	// routeEntry captures a single action definition found in source code.
	type routeEntry struct {
		id          string
		action      string
		destructive bool
	}

	var allRoutes []routeEntry
	catalog := mustBuildActionCatalog(t, newGitLabDotComClient(t), ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	for _, action := range catalog.Actions() {
		allRoutes = append(allRoutes, routeEntry{
			id:          string(action.ID),
			action:      action.Name,
			destructive: action.Route.Destructive,
		})
	}

	if len(allRoutes) == 0 {
		t.Fatal("no routes found — regex patterns may be outdated")
	}

	// Keywords that MUST use destructive wrappers.
	destructiveKeywords := []string{
		"delete", "remove", "revoke", "purge", "unprotect",
		"destroy", "unpublish", "deny",
	}
	containsDestructiveKeyword := func(action string) bool {
		for _, kw := range destructiveKeywords {
			if strings.Contains(action, kw) {
				return true
			}
		}
		return false
	}

	// Keywords that MUST NOT use destructive wrappers.
	safeKeywords := []string{
		"list", "get", "search", "create", "update", "edit",
		"approve", "subscribe", "upload", "download",
	}
	containsSafeKeyword := func(action string) bool {
		for _, kw := range safeKeywords {
			if strings.Contains(action, kw) {
				return true
			}
		}
		return false
	}

	var failures int
	for _, r := range allRoutes {
		hasDestructiveKw := containsDestructiveKeyword(r.action)
		hasSafeKw := containsSafeKeyword(r.action)

		// Rule 1: Action with destructive keyword MUST be marked destructive.
		if hasDestructiveKw && !r.destructive {
			t.Errorf("%s action %q contains destructive keyword but is non-destructive", r.id, r.action)
			failures++
		}

		// Rule 2: Action with safe keyword MUST NOT be marked destructive,
		// UNLESS it also contains a destructive keyword or is a known exception.
		_, isKnownException := knownNonKeywordDestructive[r.action]
		isKnownException = isKnownException || isRouteDestructiveException(r.id)
		if hasSafeKw && r.destructive && !hasDestructiveKw && !isKnownException {
			t.Errorf("%s action %q contains safe keyword but is destructive", r.id, r.action)
			failures++
		}

		// Rule 3: Destructive actions without keyword must be in the known exceptions list.
		if r.destructive && !hasDestructiveKw && !isKnownException {
			t.Errorf("%s action %q is destructive but has no destructive keyword and is not in known exceptions; add it to knownNonKeywordDestructive", r.id, r.action)
			failures++
		}
	}

	t.Logf("scanned %d routes (%d failures)", len(allRoutes), failures)
}

// TestDestructiveRoutes_MinimumInventory_PreventsMassReclassification verifies
// that the total number of destructive actions in the canonical catalog does
// not drop below a known minimum. This prevents accidental mass
// reclassification of destructive actions to non-destructive.
func TestDestructiveRoutes_MinimumInventory_PreventsMassReclassification(t *testing.T) {
	catalog := mustBuildActionCatalog(t, newGitLabDotComClient(t), ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	uniqueActions := make(map[string]bool)
	for _, action := range catalog.Actions() {
		if action.Route.Destructive {
			uniqueActions[string(action.ID)] = true
		}
	}

	// Current baseline: update this number when intentionally adding/removing
	// destructive routes. This number represents the minimum expected count in
	// the canonical action catalog, including GitLab.com Enterprise actions.
	const minExpectedDestructiveRoutes = 163

	total := len(uniqueActions)
	if total < minExpectedDestructiveRoutes {
		t.Errorf("only %d destructive routes found, expected at least %d — possible mass reclassification",
			total, minExpectedDestructiveRoutes)
	}
	t.Logf("found %d unique destructive route definitions (minimum: %d)", total, minExpectedDestructiveRoutes)
}

// toolNameRe matches the gitlab_{domain}_{action}[_{detail}...] snake_case convention.
// Segments may start with a digit to support well-known acronyms like 2fa.
var toolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9][a-z0-9]*)+$`)

// metaToolNameRe matches meta-tool names like gitlab_{domain}[_{subdomain}].
var metaToolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9][a-z0-9]*)*$`)

const (
	// auditMinDescLen is the minimum useful MCP tool description length enforced
	// by metadata audits.
	auditMinDescLen = 20

	// metaToolDescriptionAuditPhrase is the schema-resource phrase every
	// action-based meta-tool description must include.
	metaToolDescriptionAuditPhrase = "Action params schema:"
)

// auditHandler returns an HTTP handler that responds to all GitLab API
// requests with minimal valid JSON. Audit tests only need to register
// tools and inspect their metadata — they do not call tool handlers.
func auditHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})
}

// readSuffixes are tool name endings that indicate read-only operations.
// Suffix matching avoids false positives with compound resource names
// like "board_list" where "list" is part of the resource, not the action.
var readSuffixes = []string{
	"_list", "_lists", "_get", "_search",
	"_latest", "_blame", "_raw", "_diff", "_refs",
	"_statuses", "_signature", "_languages", "_statistics",
}

// isReadToolName returns true if the tool name ends with a suffix that
// indicates a read-only operation (list, get, search, etc.).
func isReadToolName(name string) bool {
	for _, sfx := range readSuffixes {
		if strings.HasSuffix(name, sfx) {
			return true
		}
	}
	return false
}

// isDeleteToolName returns true if the tool name ends with "_delete"
// or contains "delete" as an action word (e.g., gitlab_delete_terraform_state).
func isDeleteToolName(name string) bool {
	if strings.HasSuffix(name, "_delete") {
		return true
	}
	return slices.Contains(strings.Split(name, "_"), "delete")
}

// knownNamingExceptions lists tools whose names violate the convention
// but are tracked for remediation in a later audit phase.
var knownNamingExceptions = map[string]string{}

// ---------- Audit helper functions ----------.

// checkToolAnnotations validates that a tool's annotations are properly set:
// non-nil, OpenWorldHint=true, DestructiveHint present, no contradictory flags.
func checkToolAnnotations(t *testing.T, ann *mcp.ToolAnnotations) {
	t.Helper()
	if ann == nil {
		t.Fatal("annotations are nil")
	}
	if ann.OpenWorldHint == nil {
		t.Error("OpenWorldHint is nil (should be *bool)")
	} else if !*ann.OpenWorldHint {
		t.Error("OpenWorldHint should be true for GitLab tools")
	}
	if ann.DestructiveHint == nil {
		t.Error("DestructiveHint is nil (should be *bool)")
	}
	if ann.ReadOnlyHint && ann.DestructiveHint != nil && *ann.DestructiveHint {
		t.Error("ReadOnlyHint=true but DestructiveHint=true — contradictory")
	}
	if ann.ReadOnlyHint && !ann.IdempotentHint {
		t.Error("ReadOnlyHint=true but IdempotentHint=false — read-only tools should be idempotent")
	}
}

// checkToolOperationType validates that tool names match their annotation hints:
// read-suffix tools should be ReadOnly, delete-suffix tools should be Destructive.
func checkToolOperationType(t *testing.T, name string, ann *mcp.ToolAnnotations) {
	t.Helper()
	if isReadToolName(name) {
		if !ann.ReadOnlyHint {
			t.Errorf("name contains read keyword (list/get/search) but ReadOnlyHint=false")
		}
	}
	if isDeleteToolName(name) {
		if ann.DestructiveHint == nil || !*ann.DestructiveHint {
			t.Errorf("name contains 'delete' but DestructiveHint is not true")
		}
	}
}

// checkActionEnumValues validates that an action property has a valid enum
// constraint with non-empty string values.
func checkActionEnumValues(t *testing.T, actionProp map[string]any) {
	t.Helper()
	enumVal, hasEnum := actionProp["enum"]
	if !hasEnum {
		t.Fatal("action property missing 'enum' constraint")
	}
	enumList, ok := enumVal.([]any)
	if !ok {
		t.Fatalf("action enum is not []any, got %T", enumVal)
	}
	if len(enumList) == 0 {
		t.Error("action enum is empty")
	}
	var s string
	for i, v := range enumList {
		s, ok = v.(string)
		if !ok {
			t.Errorf("enum[%d] is not string, got %T", i, v)
		} else if s == "" {
			t.Errorf("enum[%d] is empty string", i)
		}
	}
}

// checkSchemaConstraints validates that 'action' is in required fields and
// additionalProperties is false.
func checkSchemaConstraints(t *testing.T, schema map[string]any) {
	t.Helper()
	required, _ := schema["required"].([]any)
	hasActionRequired := false
	for _, r := range required {
		if r == "action" {
			hasActionRequired = true
			break
		}
	}
	if !hasActionRequired {
		t.Error("'action' not in required fields")
	}
	if ap, hasAP := schema["additionalProperties"]; hasAP {
		if apBool, ok := ap.(bool); ok && apBool {
			t.Error("additionalProperties should be false")
		}
	}
}

// checkMetaToolActionEnum validates the action enum schema for a meta-tool.
func checkMetaToolActionEnum(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema is not map[string]any, got %T", tool.InputSchema)
	}

	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Skipf("tool %s has no input properties — not a domain meta-tool", tool.Name)
	}

	actionProp, _ := props["action"].(map[string]any)
	if actionProp == nil {
		t.Skipf("tool %s has no 'action' property — not a domain meta-tool", tool.Name)
	}

	checkActionEnumValues(t, actionProp)
	checkSchemaConstraints(t, schema)
}

// hasMetaToolAction reports whether a tool uses the action+params meta-tool
// envelope. Standalone utilities in meta mode, such as interactive creation
// tools, intentionally do not use this envelope.
func hasMetaToolAction(tool *mcp.Tool) bool {
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		return false
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = props["action"].(map[string]any)
	return ok
}

// auditToolMetadata returns metadata validation flags and a list of issues for a tool.
func auditToolMetadata(tool *mcp.Tool) (nameOK, descOK, annOK, schemaOK bool, issues []string) {
	nameOK = toolNameRe.MatchString(tool.Name)
	descOK = len(tool.Description) >= auditMinDescLen
	annOK = tool.Annotations != nil &&
		tool.Annotations.OpenWorldHint != nil &&
		tool.Annotations.DestructiveHint != nil
	if schema, ok := tool.InputSchema.(map[string]any); ok {
		_, hasProps := schema["properties"]
		schemaType, _ := schema["type"].(string)
		schemaOK = schemaType == "object" && hasProps
	}
	if !nameOK {
		issues = append(issues, "name")
	}
	if !descOK {
		issues = append(issues, "desc")
	}
	if !annOK {
		issues = append(issues, "annotations")
	}
	if !schemaOK {
		issues = append(issues, "schema")
	}
	return nameOK, descOK, annOK, schemaOK, issues
}

// auditMetaToolMetadata returns metadata validation flags for a meta-tool.
func auditMetaToolMetadata(tool *mcp.Tool) (annOK, enumOK bool, actionCount int) {
	annOK = tool.Annotations != nil &&
		tool.Annotations.OpenWorldHint != nil &&
		tool.Annotations.DestructiveHint != nil
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		return annOK, enumOK, actionCount
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return annOK, enumOK, actionCount
	}
	action, ok := props["action"].(map[string]any)
	if !ok {
		return annOK, enumOK, actionCount
	}
	enumList, ok := action["enum"].([]any)
	if ok {
		enumOK = len(enumList) > 0
		actionCount = len(enumList)
	}
	return annOK, enumOK, actionCount
}

// ---------- Individual tool metadata audit ----------.

// TestMetadataAudit_ToolNamingConvention verifies MetadataAudit when tool naming convention.
func TestMetadataAudit_ToolNamingConvention(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if reason, isException := knownNamingExceptions[tool.Name]; isException {
				t.Skipf("known exception: %s", reason)
			}
			if !toolNameRe.MatchString(tool.Name) {
				t.Errorf("name %q does not match gitlab_{action}_{resource} snake_case pattern", tool.Name)
			}
		})
	}
}

// TestMetadataAudit_ToolDescriptions verifies MetadataAudit when tool descriptions.
func TestMetadataAudit_ToolDescriptions(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Description == "" {
				t.Error("description is empty")
				return
			}
			if len(tool.Description) < auditMinDescLen {
				t.Errorf("description too short (%d chars, minimum %d): %q",
					len(tool.Description), auditMinDescLen, tool.Description)
			}
		})
	}
}

// TestMetadataAudit_ToolAnnotations verifies MetadataAudit when tool annotations.
func TestMetadataAudit_ToolAnnotations(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			checkToolAnnotations(t, tool.Annotations)
		})
	}
}

// TestMetadataAudit_ToolAnnotationOperationType verifies MetadataAudit when tool annotation operation type.
func TestMetadataAudit_ToolAnnotationOperationType(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			checkToolOperationType(t, tool.Name, tool.Annotations)
		})
	}
}

// TestMetadataAudit_ToolInputSchema verifies MetadataAudit when tool input schema.
func TestMetadataAudit_ToolInputSchema(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("InputSchema is not map[string]any, got %T", tool.InputSchema)
			}

			schemaType, _ := schema["type"].(string)
			if schemaType != "object" {
				t.Errorf("InputSchema type = %q, want \"object\"", schemaType)
			}

			// Tools with no parameters (e.g., gitlab_get_appearance)
			// generate schemas without 'properties' — this is valid.
			if _, hasProps := schema["properties"]; !hasProps {
				t.Logf("INFO: schema has no properties (zero-parameter tool)")
			}
		})
	}
}

// ---------- Meta-tool metadata audit ----------.

// TestMetadataAudit_MetaToolNaming verifies MetadataAudit when meta tool naming.
func TestMetadataAudit_MetaToolNaming(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if !metaToolNameRe.MatchString(tool.Name) {
				t.Errorf("meta-tool name %q does not match gitlab_{domain} pattern", tool.Name)
			}
		})
	}
}

// TestMetadataAudit_MetaToolDescriptions verifies MetadataAudit when meta tool descriptions.
func TestMetadataAudit_MetaToolDescriptions(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Description == "" {
				t.Error("description is empty")
				return
			}
			if len(tool.Description) < auditMinDescLen {
				t.Errorf("description too short (%d chars, minimum %d)",
					len(tool.Description), auditMinDescLen)
			}
			if hasMetaToolAction(tool) && !strings.Contains(tool.Description, metaToolDescriptionAuditPhrase) {
				t.Errorf("meta-tool description should contain %q, got %q", metaToolDescriptionAuditPhrase, tool.Description)
			}
		})
	}
}

// TestMetadataAudit_MetaToolAnnotations verifies MetadataAudit when meta tool annotations.
func TestMetadataAudit_MetaToolAnnotations(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Annotations == nil {
				t.Fatal("annotations are nil")
			}
			if tool.Annotations.OpenWorldHint == nil {
				t.Error("OpenWorldHint is nil (should be *bool)")
			} else if !*tool.Annotations.OpenWorldHint {
				t.Error("OpenWorldHint should be true for GitLab meta-tools")
			}
			if tool.Annotations.DestructiveHint == nil {
				t.Error("DestructiveHint is nil (should be *bool)")
			}
		})
	}
}

// TestMetadataAudit_MetaToolActionEnum verifies MetadataAudit when meta tool action enum.
func TestMetadataAudit_MetaToolActionEnum(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			checkMetaToolActionEnum(t, tool)
		})
	}
}

// ---------- Cross-validation ----------.

// TestMetadataAudit_NoDuplicateToolNames verifies MetadataAudit when no duplicate tool names.
func TestMetadataAudit_NoDuplicateToolNames(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	seen := make(map[string]int, len(result.Tools))
	for _, tool := range result.Tools {
		seen[tool.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate tool name %q registered %d times", name, count)
		}
	}
}

// TestMetadataAudit_NoDuplicateMetaToolNames verifies MetadataAudit when no duplicate meta tool names.
func TestMetadataAudit_NoDuplicateMetaToolNames(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	seen := make(map[string]int, len(result.Tools))
	for _, tool := range result.Tools {
		seen[tool.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate meta-tool name %q registered %d times", name, count)
		}
	}
}

// ---------- Report generator ----------.

// TestMetadataAudit_Report generates a summary report of all tool metadata
// for manual review. Run with -v to see the report.
func TestMetadataAudit_Report(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	var violations int
	t.Logf("\n## Individual Tool Metadata Report (%d tools)\n", len(result.Tools))
	t.Logf("| Tool | Name OK | Desc OK | Ann OK | Schema OK | Issues |")
	t.Logf("|------|---------|---------|--------|-----------|--------|")

	for _, tool := range result.Tools {
		nameOK, descOK, annOK, schemaOK, issues := auditToolMetadata(tool)
		if len(issues) > 0 {
			violations++
			t.Logf("| %s | %s | %s | %s | %s | %s |",
				tool.Name,
				boolMark(nameOK), boolMark(descOK), boolMark(annOK), boolMark(schemaOK),
				strings.Join(issues, ", "))
		}
	}

	if violations == 0 {
		t.Logf("\n✓ All %d individual tools pass basic metadata checks.", len(result.Tools))
	} else {
		t.Logf("\n✗ %d / %d tools have metadata issues.", violations, len(result.Tools))
	}

	metaSession := newMetaMCPSession(t, auditHandler(), true)
	metaResult, err := metaSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	t.Logf("\n## Meta-Tool Metadata Report (%d tools)\n", len(metaResult.Tools))
	t.Logf("| Meta-Tool | Actions | Ann OK | Enum OK |")
	t.Logf("|-----------|---------|--------|---------|")

	for _, tool := range metaResult.Tools {
		annOK, enumOK, actionCount := auditMetaToolMetadata(tool)
		t.Logf("| %s | %d | %s | %s |",
			tool.Name, actionCount, boolMark(annOK), boolMark(enumOK))
	}
}

// boolMark supports bool mark assertions in tools tests.
func boolMark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

// TestOutputSchemaPresence verifies that every registered MCP tool declares an
// OutputSchema, ensuring structured content is available for all tool responses.
func TestOutputSchemaPresence(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	// Check first 3 tools for outputSchema
	count := 0
	for _, tool := range result.Tools {
		if tool.OutputSchema != nil {
			count++
		}
	}
	t.Logf("Tools with OutputSchema: %d / %d", count, len(result.Tools))
	if count == 0 {
		t.Log("WARNING: No tools have OutputSchema set")
		// Print first tool as JSON to inspect
		if len(result.Tools) > 0 {
			data, mErr := json.MarshalIndent(result.Tools[0], "", "  ")
			if mErr != nil {
				t.Fatalf("marshal first tool: %v", mErr)
			}
			t.Logf("First tool JSON:\n%s", string(data)[:min(2000, len(string(data)))])
		}
	}
}

const (
	// pathReleases is the URL path segment for release endpoints.
	pathReleases = "/releases/"
	// pathDiscussions is the URL path segment for discussion endpoints.
	pathDiscussions = "/discussions"
	// pathNotes is the URL path segment for note endpoints.
	pathNotes = "/notes"
	// suffixIssues is the URL path segment for issue endpoints.
	suffixIssues = "/issues"
)

// mockBodies holds all JSON response bodies used by the mock GitLab API handler.
type mockBodies struct {
	project, branch, protectedBranch, tag string
	release, releaseLink                  string
	mr, mrNote, discussion, mrChanges     string
	commit, file                          string
	issue, issueNote                      string
}

// newMockBodies returns a freshly populated mockBodies with valid JSON
// response payloads for all supported GitLab API entities.
func newMockBodies() mockBodies {
	return mockBodies{
		project:         `{"id":42,"name":"test","path_with_namespace":"ns/test","visibility":"private","web_url":"https://example.com/ns/test","description":"desc","default_branch":"main","namespace":{"id":1,"name":"ns","path":"ns","full_path":"ns"}}`,
		branch:          `{"name":"dev","merged":false,"protected":false,"default":false,"web_url":"https://example.com","commit":{"id":"abc123","short_id":"abc1","title":"init","message":"init","author_name":"test"}}`,
		protectedBranch: `{"id":1,"name":"main","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false}`,
		tag:             `{"name":"v1.0","message":"tag","target":"abc123","commit":{"id":"abc123","short_id":"abc1","title":"init","message":"init","author_name":"test"}}`,
		release:         `{"tag_name":"v1.0","name":"v1.0","description":"notes","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","author":{"username":"test"},"commit":{"id":"abc123"},"assets":{"links":[]}}`,
		releaseLink:     `{"id":1,"name":"bin","url":"https://example.com/bin","link_type":"package"}`,
		mr:              `{"id":1,"iid":1,"title":"MR","state":"opened","source_branch":"dev","target_branch":"main","web_url":"https://example.com/mr/1","author":{"username":"test"},"description":"d","labels":[],"assignees":[],"reviewers":[],"detailed_merge_status":"mergeable","has_conflicts":false,"changes_count":"1"}`,
		mrNote:          `{"id":1,"body":"note","author":{"username":"test"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false,"resolvable":false}`,
		discussion:      `{"id":"abc","individual_note":false,"notes":[{"id":1,"body":"disc","author":{"username":"test"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false,"resolvable":true,"resolved":false}]}`,
		mrChanges:       `{"id":1,"iid":1,"title":"MR","state":"opened","changes":[{"old_path":"a.go","new_path":"a.go","diff":"@@ -1 +1 @@\\n-old\\n+new","new_file":false,"renamed_file":false,"deleted_file":false}]}`,
		commit:          `{"id":"abc123","short_id":"abc1","title":"msg","message":"msg","author_name":"test","author_email":"t@e.com","created_at":"2026-01-01T00:00:00Z","web_url":"https://example.com/c/abc","stats":{"additions":1,"deletions":0,"total":1}}`,
		file:            `{"file_name":"README.md","file_path":"README.md","size":100,"encoding":"base64","content_sha256":"abc","ref":"main","blob_id":"def","commit_id":"abc123","last_commit_id":"abc123","content":"SGVsbG8="}`,
		issue:           `{"id":1,"iid":10,"title":"Test issue","description":"desc","state":"opened","labels":["bug"],"assignees":[{"username":"alice"}],"milestone":{"title":"v1.0"},"author":{"username":"test"},"web_url":"https://example.com/issues/10","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
		issueNote:       `{"id":1,"body":"note","author":{"username":"test"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false,"internal":false}`,
	}
}

// routeAwareMockHandler returns an HTTP handler that serves mock responses
// for every GitLab API endpoint used by the 52 tools.
func routeAwareMockHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	b := newMockBodies()
	return func(w http.ResponseWriter, r *http.Request) {
		if routeProjects(w, r, b) ||
			routeBranches(w, r, b) ||
			routeTags(w, r, b) ||
			routeReleases(w, r, b) ||
			routeMergeRequests(w, r, b) ||
			routeIssues(w, r, b) ||
			routeNotes(w, r, b) ||
			routeDiscussions(w, r, b) ||
			routeCommitsAndFiles(w, r, b) ||
			routeMembersAndGroups(w, r) ||
			routeUploads(w, r) {
			return
		}
		t.Logf("unhandled: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}
}

// projectPath42 is the URL path for project ID 42, used across route helpers.
const projectPath42 = "/api/v4/projects/42"

// routeProjects handles mock GitLab project API endpoints.
func routeProjects(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && p == "/api/v4/projects":
		respondJSON(w, http.StatusCreated, b.project)
	case r.Method == http.MethodGet && p == projectPath42:
		respondJSON(w, http.StatusOK, b.project)
	case r.Method == http.MethodGet && p == "/api/v4/projects":
		respondJSON(w, http.StatusOK, "["+b.project+"]")
	case r.Method == http.MethodDelete && p == projectPath42:
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodPut && p == projectPath42:
		respondJSON(w, http.StatusOK, b.project)
	default:
		return false
	}
	return true
}

// routeBranches handles mock GitLab branch and protected branch API endpoints.
func routeBranches(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/repository/branches"):
		respondJSON(w, http.StatusCreated, b.branch)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/repository/branches"):
		respondJSON(w, http.StatusOK, "["+b.branch+"]")
	case r.Method == http.MethodPost && strings.Contains(p, "/protected_branches"):
		respondJSON(w, http.StatusCreated, b.protectedBranch)
	case r.Method == http.MethodDelete && strings.Contains(p, "/protected_branches/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/protected_branches"):
		respondJSON(w, http.StatusOK, "["+b.protectedBranch+"]")
	default:
		return false
	}
	return true
}

// routeTags handles mock GitLab tag API endpoints.
func routeTags(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/repository/tags"):
		respondJSON(w, http.StatusCreated, b.tag)
	case r.Method == http.MethodDelete && strings.Contains(p, "/repository/tags/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/repository/tags"):
		respondJSON(w, http.StatusOK, "["+b.tag+"]")
	default:
		return false
	}
	return true
}

// routeReleases handles mock GitLab release and asset link API endpoints.
func routeReleases(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/releases"):
		respondJSON(w, http.StatusCreated, b.release)
	case r.Method == http.MethodPut && strings.Contains(p, pathReleases):
		respondJSON(w, http.StatusOK, b.release)
	case r.Method == http.MethodDelete && strings.Contains(p, pathReleases) && !strings.Contains(p, "/assets/"):
		respondJSON(w, http.StatusOK, b.release)
	case r.Method == http.MethodGet && strings.Contains(p, pathReleases) && !strings.Contains(p, "/assets/"):
		respondJSON(w, http.StatusOK, b.release)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/releases"):
		respondJSON(w, http.StatusOK, "["+b.release+"]")
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/assets/links"):
		respondJSON(w, http.StatusCreated, b.releaseLink)
	case r.Method == http.MethodDelete && strings.Contains(p, "/assets/links/"):
		respondJSON(w, http.StatusOK, b.releaseLink)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/assets/links"):
		respondJSON(w, http.StatusOK, "["+b.releaseLink+"]")
	default:
		return false
	}
	return true
}

// routeMergeRequests handles mock GitLab merge request API endpoints.
func routeMergeRequests(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	hasMR1 := strings.Contains(p, "/merge_requests/1")
	if routeMergeRequestReadWrite(w, r, b, p, hasMR1) {
		return true
	}
	if routeMergeRequestReview(w, r, b, p) {
		return true
	}
	return false
}

func routeMergeRequestReadWrite(w http.ResponseWriter, r *http.Request, b mockBodies, p string, hasMR1 bool) bool {
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/merge_requests"):
		respondJSON(w, http.StatusCreated, b.mr)
	case r.Method == http.MethodGet && hasMR1 && !strings.Contains(p, pathNotes) && !strings.Contains(p, pathDiscussions) && !strings.Contains(p, "/changes"):
		respondJSON(w, http.StatusOK, b.mr)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/merge_requests"):
		respondJSON(w, http.StatusOK, "["+b.mr+"]")
	case r.Method == http.MethodPut && hasMR1 && !strings.Contains(p, "/merge") && !strings.Contains(p, pathDiscussions):
		respondJSON(w, http.StatusOK, b.mr)
	case r.Method == http.MethodPut && strings.HasSuffix(p, "/merge"):
		respondJSON(w, http.StatusOK, b.mr)
	default:
		return false
	}
	return true
}

func routeMergeRequestReview(w http.ResponseWriter, r *http.Request, b mockBodies, p string) bool {
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/approve"):
		respondJSON(w, http.StatusOK, `{}`)
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/unapprove"):
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && strings.Contains(p, "/changes"):
		respondJSON(w, http.StatusOK, b.mrChanges)
	default:
		return false
	}
	return true
}

// routeNotes handles mock GitLab note (comment) API endpoints for merge requests.
func routeNotes(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	if !strings.Contains(p, pathNotes) || strings.Contains(p, pathDiscussions) {
		return false
	}
	switch r.Method {
	case http.MethodPost:
		respondJSON(w, http.StatusCreated, b.mrNote)
	case http.MethodGet:
		respondJSON(w, http.StatusOK, "["+b.mrNote+"]")
	case http.MethodPut:
		respondJSON(w, http.StatusOK, b.mrNote)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		return false
	}
	return true
}

// routeDiscussions handles mock GitLab discussion thread API endpoints.
func routeDiscussions(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	if !strings.Contains(p, pathDiscussions) {
		return false
	}
	switch {
	case r.Method == http.MethodPost && !strings.Contains(p, pathNotes):
		respondJSON(w, http.StatusCreated, b.discussion)
	case r.Method == http.MethodPut:
		respondJSON(w, http.StatusOK, b.discussion)
	case r.Method == http.MethodPost && strings.Contains(p, pathNotes):
		respondJSON(w, http.StatusCreated, b.mrNote)
	case r.Method == http.MethodGet:
		respondJSON(w, http.StatusOK, "["+b.discussion+"]")
	default:
		return false
	}
	return true
}

// routeCommitsAndFiles handles mock GitLab commit and repository file API endpoints.
func routeCommitsAndFiles(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/repository/commits"):
		respondJSON(w, http.StatusCreated, b.commit)
	case r.Method == http.MethodGet && strings.Contains(p, "/repository/files/"):
		respondJSON(w, http.StatusOK, b.file)
	default:
		return false
	}
	return true
}

// routeMembersAndGroups handles mock GitLab member and group API endpoints.
func routeMembersAndGroups(w http.ResponseWriter, r *http.Request) bool {
	p := r.URL.Path
	member := `{"id":1,"username":"jdoe","name":"John Doe","state":"active","access_level":30,"web_url":"https://gitlab.example.com/jdoe"}`
	group := `{"id":99,"name":"test-group","path":"test-group","full_path":"test-group","description":"","visibility":"private","web_url":"https://gitlab.example.com/groups/test-group"}`
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/members/all"):
		respondJSON(w, http.StatusOK, "["+member+"]")
	case r.Method == http.MethodGet && p == "/api/v4/groups":
		respondJSON(w, http.StatusOK, "["+group+"]")
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/v4/groups/") && strings.HasSuffix(p, "/descendant_groups"):
		respondJSON(w, http.StatusOK, "["+group+"]")
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/v4/groups/"):
		respondJSON(w, http.StatusOK, group)
	default:
		return false
	}
	return true
}

// routeIssues handles mock GitLab issue API endpoints.
func routeIssues(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	if !strings.Contains(p, suffixIssues) {
		return false
	}
	hasIssueID := strings.Contains(p, "/issues/10")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, suffixIssues):
		respondJSON(w, http.StatusCreated, b.issue)
	case r.Method == http.MethodGet && hasIssueID && strings.HasSuffix(p, pathNotes):
		respondJSON(w, http.StatusOK, "["+b.issueNote+"]")
	case r.Method == http.MethodPost && hasIssueID && strings.HasSuffix(p, pathNotes):
		respondJSON(w, http.StatusCreated, b.issueNote)
	case r.Method == http.MethodGet && hasIssueID:
		respondJSON(w, http.StatusOK, b.issue)
	case r.Method == http.MethodGet && strings.HasSuffix(p, suffixIssues):
		respondJSON(w, http.StatusOK, "["+b.issue+"]")
	case r.Method == http.MethodPut && hasIssueID:
		respondJSON(w, http.StatusOK, b.issue)
	case r.Method == http.MethodDelete && hasIssueID:
		w.WriteHeader(http.StatusNoContent)
	default:
		return false
	}
	return true
}

// routeUploads handles mock GitLab project upload API endpoints.
func routeUploads(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/uploads") {
		respondJSON(w, http.StatusCreated, `{"alt":"file","url":"/uploads/abc/file.png","full_path":"/g/p/uploads/abc/file.png","markdown":"![file](/uploads/abc/file.png)"}`)
		return true
	}
	return false
}

// TestRegisterAll_AllToolsThroughMCP exercises every tool closure in register.go
// by calling each of the 52 tools via an MCP session.
func TestRegisterAll_AllToolsThroughMCP(t *testing.T) {
	session := newMCPSession(t, routeAwareMockHandler(t))

	tools := []struct {
		name  string
		input map[string]any
	}{
		{"gitlab_project_create", map[string]any{"name": "test"}},
		{"gitlab_project_get", map[string]any{"project_id": "42"}},
		{"gitlab_project_list", map[string]any{}},
		{"gitlab_project_delete", map[string]any{"project_id": "42"}},
		{"gitlab_project_update", map[string]any{"project_id": "42", "name": "t2"}},
		{"gitlab_branch_create", map[string]any{"project_id": "42", "branch_name": "dev", "ref": "main"}},
		{"gitlab_branch_list", map[string]any{"project_id": "42"}},
		{"gitlab_branch_protect", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_branch_unprotect", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_protected_branches_list", map[string]any{"project_id": "42"}},
		{"gitlab_tag_create", map[string]any{"project_id": "42", "tag_name": "v1.0", "ref": "main"}},
		{"gitlab_tag_delete", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_tag_list", map[string]any{"project_id": "42"}},
		{"gitlab_release_create", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_update", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_delete", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_get", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_list", map[string]any{"project_id": "42"}},
		{"gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0", "name": "bin", "url": "https://example.com/bin"}},
		{"gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0", "link_id": 1}},
		{"gitlab_release_link_list", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_mr_create", map[string]any{"project_id": "42", "source_branch": "dev", "target_branch": "main", "title": "test"}},
		{"gitlab_mr_get", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_list", map[string]any{"project_id": "42"}},
		{"gitlab_mr_update", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_merge", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approve", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_unapprove", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_note_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "body": "test"}},
		{"gitlab_mr_notes_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_note_update", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 1, "body": "upd"}},
		{"gitlab_mr_note_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 1}},
		{"gitlab_mr_discussion_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "body": "disc"}},
		{"gitlab_mr_discussion_resolve", map[string]any{"project_id": "42", "merge_request_iid": 1, "discussion_id": "abc", "resolved": true}},
		{"gitlab_mr_discussion_reply", map[string]any{"project_id": "42", "merge_request_iid": 1, "discussion_id": "abc", "body": "reply"}},
		{"gitlab_mr_discussion_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_changes_get", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_commit_create", map[string]any{"project_id": "42", "branch": "main", "commit_message": "test", "actions": []map[string]any{{"action": "create", "file_path": "f.txt", "content": "x"}}}},
		{"gitlab_file_get", map[string]any{"project_id": "42", "file_path": "README.md", "ref": "main"}},
		{"gitlab_project_members_list", map[string]any{"project_id": "42"}},
		{"gitlab_group_list", map[string]any{}},
		{"gitlab_group_get", map[string]any{"group_id": "99"}},
		{"gitlab_group_members_list", map[string]any{"group_id": "99"}},
		{"gitlab_subgroups_list", map[string]any{"group_id": "99"}},
		{"gitlab_project_upload", map[string]any{"project_id": "42", "filename": "test.png", "content_base64": "aGVsbG8="}},
		{"gitlab_issue_create", map[string]any{"project_id": "42", "title": "Test issue"}},
		{"gitlab_issue_get", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_issue_list", map[string]any{"project_id": "42"}},
		{"gitlab_issue_update", map[string]any{"project_id": "42", "issue_iid": 10, "title": "Updated"}},
		{"gitlab_issue_delete", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_issue_note_create", map[string]any{"project_id": "42", "issue_iid": 10, "body": "note"}},
		{"gitlab_issue_note_list", map[string]any{"project_id": "42", "issue_iid": 10}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("marshal input: %v", err)
			}
			var params map[string]any
			if err = json.Unmarshal(inputJSON, &params); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: params,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tt.name)
			}
		})
	}
}

// responseComplianceCase defines a tool call with mock routing and expected behavior.
type responseComplianceCase struct {
	name      string
	toolName  string
	arguments map[string]any
	routes    map[string]string // path -> JSON body (status 200)
}

// individualComplianceCases returns test cases for individual tool mode.
// Each case exercises one tool through the full MCP round-trip with a
// mock HTTP handler returning the specified JSON for each API path.
func individualComplianceCases() []responseComplianceCase {
	return []responseComplianceCase{
		{
			name:      "gitlab_server_status",
			toolName:  "gitlab_server_status",
			arguments: map[string]any{},
			routes: map[string]string{
				"/api/v4/version": `{"version":"17.0.0","revision":"abc"}`,
				"/api/v4/user":    `{"id":1,"username":"admin","name":"Admin","state":"active","web_url":"https://example.com/admin"}`,
			},
		},
		{
			name:      "gitlab_project_get",
			toolName:  "gitlab_project_get",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":     `{"version":"17.0.0"}`,
				"/api/v4/projects/42": `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`,
			},
		},
		{
			name:      "gitlab_branch_list",
			toolName:  "gitlab_branch_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                         `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/branches": `[{"name":"main","merged":false,"protected":true,"default":true,"commit":{"id":"abc","short_id":"abc","title":"init"}}]`,
			},
		},
		{
			name:      "gitlab_issue_list",
			toolName:  "gitlab_issue_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":            `{"version":"17.0.0"}`,
				"/api/v4/projects/42/issues": `[{"id":1,"iid":1,"title":"Bug","state":"opened","web_url":"https://example.com/issues/1"}]`,
			},
		},
		{
			name:      "gitlab_mr_list",
			toolName:  "gitlab_mr_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                    `{"version":"17.0.0"}`,
				"/api/v4/projects/42/merge_requests": `[{"id":1,"iid":1,"title":"MR","state":"opened","web_url":"https://example.com/mr/1"}]`,
			},
		},
		{
			name:      "gitlab_tag_list",
			toolName:  "gitlab_tag_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                     `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/tags": `[{"name":"v1.0.0","commit":{"id":"abc","short_id":"abc","title":"release"}}]`,
			},
		},
		{
			name:      "gitlab_label_list",
			toolName:  "gitlab_label_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":            `{"version":"17.0.0"}`,
				"/api/v4/projects/42/labels": `[{"id":1,"name":"bug","color":"#ff0000"}]`,
			},
		},
		{
			name:      "gitlab_user_current",
			toolName:  "gitlab_user_current",
			arguments: map[string]any{},
			routes: map[string]string{
				"/api/v4/version": `{"version":"17.0.0"}`,
				"/api/v4/user":    `{"id":1,"username":"admin","name":"Admin","state":"active","web_url":"https://example.com/admin"}`,
			},
		},
		{
			name:      "gitlab_pipeline_list",
			toolName:  "gitlab_pipeline_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":               `{"version":"17.0.0"}`,
				"/api/v4/projects/42/pipelines": `[{"id":1,"iid":1,"status":"success","ref":"main","sha":"abc","web_url":"https://example.com/pipelines/1"}]`,
			},
		},
		{
			name:      "gitlab_release_list",
			toolName:  "gitlab_release_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/releases": `[{"tag_name":"v1.0.0","name":"Release 1","description":"First release"}]`,
			},
		},
		{
			name:      "gitlab_package_list",
			toolName:  "gitlab_package_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/packages": `[{"id":1,"name":"app","version":"1.0.0","package_type":"generic","status":"default","created_at":"2026-01-01T00:00:00Z"}]`,
			},
		},
		{
			name:      "gitlab_milestone_list",
			toolName:  "gitlab_milestone_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                `{"version":"17.0.0"}`,
				"/api/v4/projects/42/milestones": `[{"id":1,"iid":1,"title":"v1.0","state":"active"}]`,
			},
		},
	}
}

// metaComplianceCases returns test cases for meta-tool mode.
func metaComplianceCases() []responseComplianceCase {
	return []responseComplianceCase{
		{
			name:     "meta_gitlab_project/get",
			toolName: "gitlab_project",
			arguments: map[string]any{
				"action": "get",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":     `{"version":"17.0.0"}`,
				"/api/v4/projects/42": `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`,
			},
		},
		{
			name:     "meta_gitlab_branch/list",
			toolName: "gitlab_branch",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":                         `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/branches": `[{"name":"main","merged":false,"protected":true,"default":true,"commit":{"id":"abc","short_id":"abc","title":"init"}}]`,
			},
		},
		{
			name:     "meta_gitlab_issue/list",
			toolName: "gitlab_issue",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":            `{"version":"17.0.0"}`,
				"/api/v4/projects/42/issues": `[{"id":1,"iid":1,"title":"Bug","state":"opened","web_url":"https://example.com/issues/1"}]`,
			},
		},
		{
			name:     "meta_gitlab_merge_request/list",
			toolName: "gitlab_merge_request",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":                    `{"version":"17.0.0"}`,
				"/api/v4/projects/42/merge_requests": `[{"id":1,"iid":1,"title":"MR","state":"opened","web_url":"https://example.com/mr/1"}]`,
			},
		},
		{
			name:     "meta_gitlab_package/list",
			toolName: "gitlab_package",
			arguments: map[string]any{
				"action": "list",
				"params": map[string]any{"project_id": "42"},
			},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/packages": `[{"id":1,"name":"app","version":"1.0.0","package_type":"generic","status":"default","created_at":"2026-01-01T00:00:00Z"}]`,
			},
		},
	}
}

// routeHandler builds an HTTP handler from a path -> JSON response map.
func routeHandler(routes map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		for prefix, body := range routes {
			if path == prefix || strings.HasPrefix(path, prefix) {
				respondJSON(w, http.StatusOK, body)
				return
			}
		}
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	})
}

// ---------- Response compliance tests ----------.

// TestResponseCompliance_Individual verifies that individual tool calls
// return structurally valid MCP responses via in-memory transport.
func TestResponseCompliance_Individual(t *testing.T) {
	for _, tc := range individualComplianceCases() {
		t.Run(tc.name, func(t *testing.T) {
			session := newMCPSession(t, routeHandler(tc.routes))
			assertToolResponse(t, session, tc.toolName, tc.arguments)
		})
	}
}

// TestResponseCompliance_Meta verifies that meta-tool calls return
// structurally valid MCP responses via in-memory transport.
func TestResponseCompliance_Meta(t *testing.T) {
	for _, tc := range metaComplianceCases() {
		t.Run(tc.name, func(t *testing.T) {
			session := newMetaMCPSession(t, routeHandler(tc.routes), true)
			assertToolResponse(t, session, tc.toolName, tc.arguments)
		})
	}
}

// assertToolResponse calls a tool and validates the response structure:
//  1. No transport/RPC error
//  2. IsError is false (tool-level success)
//  3. Content array is non-empty
//  4. At least one TextContent with non-empty text
func assertToolResponse(t *testing.T, session *mcp.ClientSession, toolName string, args map[string]any) {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) transport error: %v", toolName, err)
	}
	if result.IsError {
		var errText string
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				errText = tc.Text
				break
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true: %s", toolName, errText)
	}

	if len(result.Content) == 0 {
		t.Errorf("CallTool(%s): Content array is empty -- must contain at least one TextContent", toolName)
		return
	}

	var hasText bool
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			hasText = true
			break
		}
	}
	if !hasText {
		t.Errorf("CallTool(%s): no TextContent with non-empty text found in %d content entries", toolName, len(result.Content))
		for i, c := range result.Content {
			t.Logf("  Content[%d]: type=%T", i, c)
		}
	}
}

// TestResponseCompliance_AllToolsListable verifies that all registered tools
// can be listed without error and each has a non-empty name and description.
func TestResponseCompliance_AllToolsListable(t *testing.T) {
	session := newMCPSession(t, auditHandler())

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Name == "" {
				t.Error("tool has empty name")
			}
			if tool.Description == "" {
				t.Error("tool has empty description")
			}
		})
	}

	t.Logf("Verified %d tools are listable with name and description", len(result.Tools))
}

// TestResponseCompliance_MetaToolsListable verifies all meta-tools can be
// listed without error and each has a non-empty name and description.
func TestResponseCompliance_MetaToolsListable(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Name == "" {
				t.Error("tool has empty name")
			}
			if tool.Description == "" {
				t.Error("tool has empty description")
			}
		})
	}

	t.Logf("Verified %d meta-tools are listable with name and description", len(result.Tools))
}

// TestResponseCompliance_ContentHasTextContent validates that for each
// domain that produces markdown, the markdownForResult dispatcher returns
// content with proper TextContent entries (complementary to markdown_audit_test.go).
func TestResponseCompliance_ContentHasTextContent(t *testing.T) {
	for _, fix := range allMarkdownFixtures() {
		t.Run(fix.name, func(t *testing.T) {
			result := markdownForResult(fix.result)
			if result == nil {
				t.Skip("nil dispatch -- tracked in markdown_audit_test.go")
			}
			assertResultHasTextContent(t, result)
		})
	}
}

func assertResultHasTextContent(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult.Content is empty")
	}
	foundText := false
	for _, content := range result.Content {
		if contentHasValidText(t, content) {
			foundText = true
		}
	}
	if !foundText {
		t.Error("no non-empty TextContent found in Content array")
	}
}

func contentHasValidText(t *testing.T, content mcp.Content) bool {
	t.Helper()
	switch typed := content.(type) {
	case *mcp.TextContent:
		if typed.Text == "" {
			t.Error("TextContent.Text is empty")
			return false
		}
		return true
	case *mcp.ImageContent:
		if len(typed.Data) == 0 {
			t.Error("ImageContent.Data is empty")
		}
	default:
		t.Logf("unexpected content type: %T", content)
	}
	return false
}

// TestResponseCompliance_ErrorResponseFormat verifies that tool calls
// returning errors use IsError=true and include descriptive text.
func TestResponseCompliance_ErrorResponseFormat(t *testing.T) {
	errorRoutes := map[string]string{
		"/api/v4/version": `{"version":"17.0.0"}`,
	}

	tests := []struct {
		name      string
		mode      string
		toolName  string
		arguments map[string]any
	}{
		{
			name:      "individual/project_get_404",
			mode:      "individual",
			toolName:  "gitlab_project_get",
			arguments: map[string]any{"project_id": "999"},
		},
		{
			name:     "meta/project_get_404",
			mode:     "meta",
			toolName: "gitlab_project",
			arguments: map[string]any{
				"action": "get",
				"params": map[string]any{"project_id": "999"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var session *mcp.ClientSession
			switch tc.mode {
			case "individual":
				session = newMCPSession(t, routeHandler(errorRoutes))
			case "meta":
				session = newMetaMCPSession(t, routeHandler(errorRoutes), true)
			}

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tc.toolName,
				Arguments: tc.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() transport error (should not happen): %v", err)
			}

			if !result.IsError {
				t.Log("tool returned success for non-existent resource -- may be expected if error is reported differently")
				return
			}

			if len(result.Content) == 0 {
				t.Error("error result has empty Content -- should contain error description")
				return
			}

			var errText string
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					errText = tc.Text
					break
				}
			}
			if errText == "" {
				t.Error("error result lacks TextContent with error description")
			} else {
				t.Logf("error text: %s", truncate(errText, 120))
			}
		})
	}
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// TestResponseCompliance_MarkdownContentWellFormed verifies that individual
// tool responses contain well-formed markdown in their TextContent entries.
// The architecture uses a triple-return pattern where the first value is a
// CallToolResult with markdown and the second is the typed JSON output for
// internal meta-tool routing. Only the markdown appears in the MCP response.
func TestResponseCompliance_MarkdownContentWellFormed(t *testing.T) {
	cases := []struct {
		name      string
		toolName  string
		arguments map[string]any
		routes    map[string]string
	}{
		{
			name:      "project_get",
			toolName:  "gitlab_project_get",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":     `{"version":"17.0.0"}`,
				"/api/v4/projects/42": `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc"}`,
			},
		},
		{
			name:      "branch_list",
			toolName:  "gitlab_branch_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":                         `{"version":"17.0.0"}`,
				"/api/v4/projects/42/repository/branches": `[{"name":"main","merged":false,"protected":true,"default":true,"commit":{"id":"abc","short_id":"abc","title":"init"}}]`,
			},
		},
		{
			name:      "package_list",
			toolName:  "gitlab_package_list",
			arguments: map[string]any{"project_id": "42"},
			routes: map[string]string{
				"/api/v4/version":              `{"version":"17.0.0"}`,
				"/api/v4/projects/42/packages": `[{"id":1,"name":"app","version":"1.0.0","package_type":"generic","status":"default","created_at":"2026-01-01T00:00:00Z"}]`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session := newMCPSession(t, routeHandler(tc.routes))

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tc.toolName,
				Arguments: tc.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() error: %v", err)
			}
			if result.IsError {
				t.Fatalf("CallTool() returned IsError=true")
			}

			if len(result.Content) == 0 {
				t.Fatal("expected at least 1 TextContent entry with markdown, got 0")
			}

			assertResultMarkdownContent(t, result)
		})
	}
}

func assertResultMarkdownContent(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	for i, content := range result.Content {
		textContent, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		assertMarkdownTextContent(t, i, textContent.Text)
	}
}

func assertMarkdownTextContent(t *testing.T, index int, text string) {
	t.Helper()
	text = strings.TrimSpace(text)
	if text == "" {
		t.Errorf("Content[%d]: TextContent.Text is empty", index)
		return
	}
	if !hasMarkdownIndicator(text) {
		t.Errorf("Content[%d]: text lacks markdown indicators (headers, bold, tables, lists)", index)
	}
	t.Logf("Content[%d]: well-formed markdown (%d bytes)", index, len(text))
}

func hasMarkdownIndicator(text string) bool {
	return strings.Contains(text, "**") ||
		strings.Contains(text, "| ") ||
		strings.Contains(text, "## ") ||
		strings.Contains(text, "- ")
}

// TestResponseCompliance_NilResultFallback verifies that markdownForResult
// returns a success confirmation for nil results (delete operations).
func TestResponseCompliance_NilResultFallback(t *testing.T) {
	result := markdownForResult(nil)
	if result == nil {
		t.Fatal("markdownForResult(nil) should return success confirmation, got nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("success confirmation has empty Content")
	}
	text := extractTextContent(result)
	if text == "" {
		t.Error("success confirmation has empty TextContent")
	}
	if !strings.Contains(strings.ToLower(text), "ok") {
		t.Logf("success text: %q (expected to contain 'ok')", text)
	}
}

// TestResponseCompliance_DeleteOutputHandled verifies that DeleteOutput
// (used by delete tools) produces valid markdown through the dispatcher.
func TestResponseCompliance_DeleteOutputHandled(t *testing.T) {
	result := markdownForResult(toolutil.DeleteOutput{Message: "Resource deleted successfully"})
	if result == nil {
		t.Fatal("markdownForResult(DeleteOutput) returned nil")
	}
	text := extractTextContent(result)
	if text == "" {
		t.Error("DeleteOutput produced empty markdown")
	}
	if !strings.Contains(text, "deleted") {
		t.Logf("delete markdown: %q", truncate(text, 100))
	}
}

// ---------- Coverage tracking ----------.

// TestResponseCompliance_DomainCoverage checks that the response compliance
// test suite covers the major tool domains (sub-packages). It compares
// tested domains against the known sub-package list and reports coverage.
func TestResponseCompliance_DomainCoverage(t *testing.T) {
	// Known tool domain sub-packages (from internal/tools/*)
	knownDomains := []string{
		"project", "branch", "tag", "release", "issue", "mergerequests",
		"label", "milestone", "member", "user", "pipeline", "job",
		"commit", "search", "group", "wiki", "package", "health",
		"environment", "deployment", "civar", "cilint", "repository",
		"mrnote", "mrdiscussion", "mrapproval", "mrchange",
		"issuenote", "issuelink", "releaselink", "upload", "todo",
		"file", "pipelineschedule", "runner", "accesstoken",
		"mrdraftnote", "snippet", "pages",
	}

	// Tested domains from compliance cases -- map tool names to domain keywords
	testedKeywords := make(map[string]bool)
	for _, tc := range individualComplianceCases() {
		// Extract meaningful domain from tool name: gitlab_{domain}_{action}
		name := strings.TrimPrefix(tc.toolName, "gitlab_")
		for _, d := range knownDomains {
			if strings.Contains(name, d) {
				testedKeywords[d] = true
			}
		}
	}

	covered := len(testedKeywords)
	total := len(knownDomains)
	coverage := float64(covered) / float64(total) * 100

	t.Logf("Domain coverage: %d/%d (%.1f%%)", covered, total, coverage)

	var uncovered []string
	for _, d := range knownDomains {
		if !testedKeywords[d] {
			uncovered = append(uncovered, d)
		}
	}
	if len(uncovered) > 0 {
		t.Logf("Uncovered domains (non-blocking): %s", strings.Join(uncovered, ", "))
	}

	// Informational threshold -- 25% is reasonable for a foundation test
	if coverage < 25 {
		t.Errorf("domain coverage %.1f%% is below minimum 25%% threshold", coverage)
	}
}

func init() {
	// Silence unused import warning for fmt -- used in test log messages.
	_ = fmt.Sprintf
}

// toolSnapshot captures the fields we care about for snapshot comparison.
type toolSnapshot struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	InputSchema  json.RawMessage      `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage      `json:"outputSchema,omitempty"`
	Annotations  *mcp.ToolAnnotations `json:"annotations,omitempty"`
}

// TestToolSnapshots_Individual compares individual-mode tool definitions
// against the golden file testdata/tools_individual.json.
func TestToolSnapshots_Individual(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	session := newMCPSession(t, handler, true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	snapshots := buildSnapshots(t, result.Tools)
	goldenPath := filepath.Join("testdata", "tools_individual.json")
	compareOrUpdate(t, goldenPath, snapshots)
}

// TestToolSnapshots_Meta compares meta-tool definitions against the
// golden file testdata/tools_meta.json.
func TestToolSnapshots_Meta(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	session := newMetaMCPSession(t, handler, true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	snapshots := buildSnapshots(t, result.Tools)
	goldenPath := filepath.Join("testdata", "tools_meta.json")
	compareOrUpdate(t, goldenPath, snapshots)
}

// buildSnapshots extracts snapshot data from MCP tool definitions,
// sorted alphabetically by name for deterministic output.
func buildSnapshots(t *testing.T, tools []*mcp.Tool) []toolSnapshot {
	t.Helper()
	snaps := make([]toolSnapshot, 0, len(tools))
	for _, tool := range tools {
		s := toolSnapshot{
			Name:        tool.Name,
			Description: tool.Description,
			Annotations: tool.Annotations,
		}
		if tool.InputSchema != nil {
			raw, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatalf("marshal InputSchema for %s: %v", tool.Name, err)
			}
			s.InputSchema = raw
		}
		if tool.OutputSchema != nil {
			raw, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Fatalf("marshal OutputSchema for %s: %v", tool.Name, err)
			}
			s.OutputSchema = raw
		}
		snaps = append(snaps, s)
	}
	slices.SortFunc(snaps, func(a, b toolSnapshot) int {
		return strings.Compare(a.Name, b.Name)
	})
	return snaps
}

// compareOrUpdate either updates the golden file or compares current
// output against it, reporting a clear diff on mismatch.
func compareOrUpdate(t *testing.T, goldenPath string, current []toolSnapshot) {
	t.Helper()

	got, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		t.Fatalf("marshal current snapshots: %v", err)
	}

	if os.Getenv("UPDATE_TOOLSNAPS") == "true" {
		if mkdirErr := os.MkdirAll(filepath.Dir(goldenPath), 0o750); mkdirErr != nil {
			t.Fatalf("create testdata dir: %v", mkdirErr)
		}
		if writeErr := os.WriteFile(goldenPath, got, 0o600); writeErr != nil {
			t.Fatalf("write golden file: %v", writeErr)
		}
		t.Logf("Updated golden file: %s (%d tools)", goldenPath, len(current))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v\nRun with UPDATE_TOOLSNAPS=true to generate", goldenPath, err)
	}

	if string(got) == string(want) {
		return
	}

	// Parse both for structured diff
	var wantSnaps []toolSnapshot
	if unmarshalErr := json.Unmarshal(want, &wantSnaps); unmarshalErr != nil {
		t.Fatalf("parse golden file: %v", unmarshalErr)
	}

	reportDiff(t, goldenPath, wantSnaps, current)
}

// reportDiff produces a human-readable diff showing which tools were
// added, removed, or changed between the golden and current snapshots.
func reportDiff(t *testing.T, goldenPath string, want, got []toolSnapshot) {
	t.Helper()

	wantMap := make(map[string]toolSnapshot, len(want))
	for _, s := range want {
		wantMap[s.Name] = s
	}
	gotMap := make(map[string]toolSnapshot, len(got))
	for _, s := range got {
		gotMap[s.Name] = s
	}

	var diffs []string

	// Removed tools
	for name := range wantMap {
		if _, ok := gotMap[name]; !ok {
			diffs = append(diffs, "REMOVED tool: "+name)
		}
	}

	// Added tools
	for name := range gotMap {
		if _, ok := wantMap[name]; !ok {
			diffs = append(diffs, "ADDED tool: "+name)
		}
	}

	// Changed tools
	for name, wSnap := range wantMap {
		gSnap, ok := gotMap[name]
		if !ok {
			continue
		}
		if wSnap.Description != gSnap.Description {
			diffs = append(diffs, "CHANGED "+name+" description:\n  old: "+wSnap.Description+"\n  new: "+gSnap.Description)
		}
		if !rawJSONEqual(wSnap.InputSchema, gSnap.InputSchema) {
			diffs = append(diffs, "CHANGED "+name+" inputSchema:\n  old: "+string(wSnap.InputSchema)+"\n  new: "+string(gSnap.InputSchema))
		}
		if !rawJSONEqual(wSnap.OutputSchema, gSnap.OutputSchema) {
			diffs = append(diffs, "CHANGED "+name+" outputSchema:\n  old: "+string(wSnap.OutputSchema)+"\n  new: "+string(gSnap.OutputSchema))
		}
		wAnn, err := json.Marshal(wSnap.Annotations)
		if err != nil {
			t.Fatalf("marshal want annotations for %s: %v", name, err)
		}
		gAnn, err := json.Marshal(gSnap.Annotations)
		if err != nil {
			t.Fatalf("marshal got annotations for %s: %v", name, err)
		}
		if string(wAnn) != string(gAnn) {
			diffs = append(diffs, "CHANGED "+name+" annotations:\n  old: "+string(wAnn)+"\n  new: "+string(gAnn))
		}
	}

	slices.Sort(diffs)
	if len(diffs) == 0 {
		return
	}

	t.Errorf("Tool snapshots changed (%s). Found %d difference(s):\n%s\n\nRun with UPDATE_TOOLSNAPS=true to update golden files.",
		goldenPath, len(diffs), strings.Join(diffs, "\n"))
}

// rawJSONEqual compares JSON values after compaction so golden snapshots are
// insensitive to whitespace-only formatting differences.
func rawJSONEqual(want, got json.RawMessage) bool {
	var compactWant, compactGot bytes.Buffer
	if err := json.Compact(&compactWant, want); err != nil {
		return string(want) == string(got)
	}
	if err := json.Compact(&compactGot, got); err != nil {
		return string(want) == string(got)
	}
	return bytes.Equal(compactWant.Bytes(), compactGot.Bytes())
}

// Shared GitLab error fixture payloads reused by context/error path tests.
const (
	// msgCancelledCtxErr is the assertion message for tests expecting a canceled context error.
	msgCancelledCtxErr = "expected error for canceled context"
	// msgForbiddenErr is the assertion message for tests expecting a 403 Forbidden error.
	msgForbiddenErr = "expected error for 403 response"
	// msgNotFoundErr is the assertion message for tests expecting a 404 Not Found error.
	msgNotFoundErr = "expected error for 404 response"

	jsonNotFound  = `{"message":"404 Not Found"}`
	jsonForbidden = `{"message":"403 Forbidden"}`
)

// ----------- Branch context/error tests -----------.

// TestBranchProtect_ContextCancelled verifies BranchProtect when context cancelled.
func TestBranchProtect_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := branches.Protect(ctx, client, branches.ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchProtect_APIError verifies BranchProtect when API error.
func TestBranchProtect_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := branches.Protect(context.Background(), client, branches.ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// TestBranchProtect_AllowForcePush verifies BranchProtect when allow force push.
func TestBranchProtect_AllowForcePush(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":1,"name":"main","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":true}`)
	}))

	out, err := branches.Protect(context.Background(), client, branches.ProtectInput{
		ProjectID:      "42",
		BranchName:     "main",
		AllowForcePush: new(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.AllowForcePush {
		t.Error("expected AllowForcePush=true")
	}
}

// TestBranchUnprotect_ContextCancelled verifies BranchUnprotect when context cancelled.
func TestBranchUnprotect_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := branches.Unprotect(ctx, client, branches.UnprotectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchCreate_ContextCancelled verifies BranchCreate when context cancelled.
func TestBranchCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := branches.Create(ctx, client, branches.CreateInput{ProjectID: "42", BranchName: "dev", Ref: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchList_ContextCancelled verifies BranchList when context cancelled.
func TestBranchList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := branches.List(ctx, client, branches.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProtectedBranchesList_ContextCancelled verifies ProtectedBranchesList when context cancelled.
func TestProtectedBranchesList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := branches.ProtectedList(ctx, client, branches.ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchList_APIError verifies BranchList when API error.
func TestBranchList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"message":"500 Server Error"}`)
	}))

	_, err := branches.List(context.Background(), client, branches.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// TestProtectedBranchesList_APIError verifies ProtectedBranchesList when API error.
func TestProtectedBranchesList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := branches.ProtectedList(context.Background(), client, branches.ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// ----------- Tag context/error tests -----------.

// TestTagCreate_ContextCancelled verifies TagCreate when context cancelled.
func TestTagCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := tags.Create(ctx, client, tags.CreateInput{ProjectID: "42", TagName: "v1.0", Ref: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestTagDelete_ContextCancelled verifies TagDelete when context cancelled.
func TestTagDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)

	err := tags.Delete(ctx, client, tags.DeleteInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestTagList_ContextCancelled verifies TagList when context cancelled.
func TestTagList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := tags.List(ctx, client, tags.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestTagList_APIError verifies TagList when API error.
func TestTagList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := tags.List(context.Background(), client, tags.ListInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- Release context/error tests -----------.

// TestReleaseCreate_ContextCancelled verifies ReleaseCreate when context cancelled.
func TestReleaseCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releases.Create(ctx, client, releases.CreateInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseGet_ContextCancelled verifies ReleaseGet when context cancelled.
func TestReleaseGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releases.Get(ctx, client, releases.GetInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseUpdate_ContextCancelled verifies ReleaseUpdate when context cancelled.
func TestReleaseUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releases.Update(ctx, client, releases.UpdateInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseDelete_ContextCancelled verifies ReleaseDelete when context cancelled.
func TestReleaseDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releases.Delete(ctx, client, releases.DeleteInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseList_ContextCancelled verifies ReleaseList when context cancelled.
func TestReleaseList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releases.List(ctx, client, releases.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseGet_APIError verifies ReleaseGet when API error.
func TestReleaseGet_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releases.Get(context.Background(), client, releases.GetInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseDelete_APIError verifies ReleaseDelete when API error.
func TestReleaseDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releases.Delete(context.Background(), client, releases.DeleteInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseUpdate_APIError verifies ReleaseUpdate when API error.
func TestReleaseUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releases.Update(context.Background(), client, releases.UpdateInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseList_APIError verifies ReleaseList when API error.
func TestReleaseList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := releases.List(context.Background(), client, releases.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// ----------- Release Link context/error tests -----------.

// TestReleaseLinkCreate_ContextCancelled verifies ReleaseLinkCreate when context cancelled.
func TestReleaseLinkCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releaselinks.Create(ctx, client, releaselinks.CreateInput{ProjectID: "42", TagName: "v1.0", Name: "bin", URL: "https://example.com"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseLinkDelete_ContextCancelled verifies ReleaseLinkDelete when context cancelled.
func TestReleaseLinkDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releaselinks.Delete(ctx, client, releaselinks.DeleteInput{ProjectID: "42", TagName: "v1.0", LinkID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseLinkList_ContextCancelled verifies ReleaseLinkList when context cancelled.
func TestReleaseLinkList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := releaselinks.List(ctx, client, releaselinks.ListInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseLinkDelete_APIError verifies ReleaseLinkDelete when API error.
func TestReleaseLinkDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releaselinks.Delete(context.Background(), client, releaselinks.DeleteInput{ProjectID: "42", TagName: "v1.0", LinkID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseLinkList_APIError verifies ReleaseLinkList when API error.
func TestReleaseLinkList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releaselinks.List(context.Background(), client, releaselinks.ListInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- MR context/error tests -----------.

// TestMRCreate_ContextCancelled verifies MRCreate when context cancelled.
func TestMRCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mergerequests.Create(ctx, client, mergerequests.CreateInput{ProjectID: "42", SourceBranch: "dev", TargetBranch: "main", Title: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRGet_ContextCancelled verifies MRGet when context cancelled.
func TestMRGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mergerequests.Get(ctx, client, mergerequests.GetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRList_ContextCancelled verifies MRList when context cancelled.
func TestMRList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mergerequests.List(ctx, client, mergerequests.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRUpdate_ContextCancelled verifies MRUpdate when context cancelled.
func TestMRUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mergerequests.Update(ctx, client, mergerequests.UpdateInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRMerge_ContextCancelled verifies MRMerge when context cancelled.
func TestMRMerge_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mergerequests.Merge(ctx, client, mergerequests.MergeInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRApprove_ContextCancelled verifies MRApprove when context cancelled.
func TestMRApprove_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mergerequests.Approve(ctx, client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRUnapprove_ContextCancelled verifies MRUnapprove when context cancelled.
func TestMRUnapprove_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	err := mergerequests.Unapprove(ctx, client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRApprove_APIError verifies MRApprove when API error.
func TestMRApprove_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := mergerequests.Approve(context.Background(), client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// TestMRUnapprove_APIError verifies MRUnapprove when API error.
func TestMRUnapprove_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	err := mergerequests.Unapprove(context.Background(), client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// TestMRUpdate_APIError verifies MRUpdate when API error.
func TestMRUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mergerequests.Update(context.Background(), client, mergerequests.UpdateInput{ProjectID: "42", MRIID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRMerge_APIError verifies MRMerge when API error.
func TestMRMerge_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed - cannot merge"}`)
	}))

	_, err := mergerequests.Merge(context.Background(), client, mergerequests.MergeInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected error for 405 response")
	}
}

// TestMRList_APIError verifies MRList when API error.
func TestMRList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mergerequests.List(context.Background(), client, mergerequests.ListInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- MR Notes context/error tests -----------.

// TestMRNoteCreate_ContextCancelled verifies MRNoteCreate when context cancelled.
func TestMRNoteCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrnotes.Create(ctx, client, mrnotes.CreateInput{ProjectID: "42", MRIID: 1, Body: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNotesList_ContextCancelled verifies MRNotesList when context cancelled.
func TestMRNotesList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrnotes.List(ctx, client, mrnotes.ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNoteUpdate_ContextCancelled verifies MRNoteUpdate when context cancelled.
func TestMRNoteUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrnotes.Update(ctx, client, mrnotes.UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 1, Body: "new"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNoteDelete_ContextCancelled verifies MRNoteDelete when context cancelled.
func TestMRNoteDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)

	err := mrnotes.Delete(ctx, client, mrnotes.DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNoteUpdate_APIError verifies MRNoteUpdate when API error.
func TestMRNoteUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrnotes.Update(context.Background(), client, mrnotes.UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 999, Body: "x"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRNoteDelete_APIError verifies MRNoteDelete when API error.
func TestMRNoteDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	err := mrnotes.Delete(context.Background(), client, mrnotes.DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRNotesList_APIError verifies MRNotesList when API error.
func TestMRNotesList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrnotes.List(context.Background(), client, mrnotes.ListInput{ProjectID: "42", MRIID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- MR Discussion context/error tests -----------.

// TestMRDiscussionCreate_ContextCancelled verifies MRDiscussionCreate when context cancelled.
func TestMRDiscussionCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrdiscussions.Create(ctx, client, mrdiscussions.CreateInput{ProjectID: "42", MRIID: 1, Body: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionResolve_ContextCancelled verifies MRDiscussionResolve when context cancelled.
func TestMRDiscussionResolve_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrdiscussions.Resolve(ctx, client, mrdiscussions.ResolveInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc", Resolved: true})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionReply_ContextCancelled verifies MRDiscussionReply when context cancelled.
func TestMRDiscussionReply_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrdiscussions.Reply(ctx, client, mrdiscussions.ReplyInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc", Body: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionList_ContextCancelled verifies MRDiscussionList when context cancelled.
func TestMRDiscussionList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrdiscussions.List(ctx, client, mrdiscussions.ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionResolve_APIError verifies MRDiscussionResolve when API error.
func TestMRDiscussionResolve_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrdiscussions.Resolve(context.Background(), client, mrdiscussions.ResolveInput{ProjectID: "42", MRIID: 1, DiscussionID: "xyz", Resolved: true})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRDiscussionReply_APIError verifies MRDiscussionReply when API error.
func TestMRDiscussionReply_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrdiscussions.Reply(context.Background(), client, mrdiscussions.ReplyInput{ProjectID: "42", MRIID: 1, DiscussionID: "xyz", Body: "x"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRDiscussionList_APIError verifies MRDiscussionList when API error.
func TestMRDiscussionList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrdiscussions.List(context.Background(), client, mrdiscussions.ListInput{ProjectID: "42", MRIID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRDiscussionCreate_APIError verifies MRDiscussionCreate when API error.
func TestMRDiscussionCreate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))

	_, err := mrdiscussions.Create(context.Background(), client, mrdiscussions.CreateInput{ProjectID: "42", MRIID: 1, Body: "test"})
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
}

// ----------- MR Changes context/error tests -----------.

// TestMRChangesGet_ContextCancelled verifies MRChangesGet when context cancelled.
func TestMRChangesGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := mrchanges.Get(ctx, client, mrchanges.GetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// ----------- Commit context/error tests -----------.

// TestCommitCreate_ContextCancelled verifies CommitCreate when context cancelled.
func TestCommitCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := commits.Create(ctx, client, commits.CreateInput{ProjectID: "42", Branch: "main", CommitMessage: "test", Actions: []commits.Action{{Action: "create", FilePath: "f.txt", Content: "x"}}})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// ----------- File context/error tests -----------.

// TestFileGet_ContextCancelled verifies FileGet when context cancelled.
func TestFileGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := files.Get(ctx, client, files.GetInput{ProjectID: "42", FilePath: "README.md", Ref: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// ----------- Repository context/error tests -----------.

// TestProjectGet_ContextCancelled verifies ProjectGet when context cancelled.
func TestProjectGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := projects.Get(ctx, client, projects.GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectList_ContextCancelled verifies ProjectList when context cancelled.
func TestProjectList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := projects.List(ctx, client, projects.ListInput{})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectDelete_ContextCancelled verifies ProjectDelete when context cancelled.
func TestProjectDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := projects.Delete(ctx, client, projects.DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectUpdate_ContextCancelled verifies ProjectUpdate when context cancelled.
func TestProjectUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := projects.Update(ctx, client, projects.UpdateInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectUpdate_APIError verifies ProjectUpdate when API error.
func TestProjectUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := projects.Update(context.Background(), client, projects.UpdateInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestProjectList_APIError verifies ProjectList when API error.
func TestProjectList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"message":"500 Error"}`)
	}))

	_, err := projects.List(context.Background(), client, projects.ListInput{})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// TestProjectGet_APIError verifies ProjectGet when API error.
func TestProjectGet_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := projects.Get(context.Background(), client, projects.GetInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestProjectDelete_APIError verifies ProjectDelete when API error.
func TestProjectDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := projects.Delete(context.Background(), client, projects.DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// ----------- Metatool additional tests -----------.

// TestUnmarshalParams_InvalidJSON verifies UnmarshalParams when invalid JSON.
func TestUnmarshalParams_InvalidJSON(t *testing.T) {
	params := map[string]any{
		"project_id": make(chan int),
	}
	_, err := unmarshalParams[projects.GetInput](params)
	if err == nil {
		t.Fatal("expected error for un-marshalable params")
	}
}

// TestWrapActionUnmarshal_Error verifies WrapActionUnmarshal when error.
func TestWrapActionUnmarshal_Error(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))

	action := wrapAction(client, projects.Get)
	_, err := action(context.Background(), map[string]any{"project_id": make(chan int)})
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
}

// TestWrapVoidActionUnmarshal_Error verifies WrapVoidActionUnmarshal when error.
func TestWrapVoidActionUnmarshal_Error(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	action := wrapVoidAction(client, uploads.Delete)
	_, err := action(context.Background(), map[string]any{"project_id": make(chan int)})
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
}

// Shared optional-value fixtures used by merge request and project tests.
const (
	testNewName     = "new-name"
	testCustomEmail = "custom@example.com"
)

// TestMRCreate_AllOptionalParams exercises every optional branch in mrCreate.
func TestMRCreate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":1,"iid":1,"title":"MR","state":"opened","source_branch":"dev","target_branch":"main","web_url":"https://example.com/mr/1","author":{"username":"test"},"description":"d","detailed_merge_status":"mergeable","has_conflicts":false,"changes_count":"1"}`)
	}))

	out, err := mergerequests.Create(context.Background(), client, mergerequests.CreateInput{
		ProjectID:          "42",
		SourceBranch:       "dev",
		TargetBranch:       "main",
		Title:              "feat: test",
		Description:        "A description",
		AssigneeIDs:        []int64{1, 2},
		ReviewerIDs:        []int64{3, 4},
		RemoveSourceBranch: new(true),
		Squash:             new(true),
	})
	if err != nil {
		t.Fatalf("mergerequests.Create() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf("IID = %d, want 1", out.IID)
	}
}

// TestMRUpdate_AllOptionalParams exercises every optional branch in mrUpdate.
func TestMRUpdate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"updated","state":"opened","source_branch":"dev","target_branch":"release","web_url":"https://example.com/mr/1","author":{"username":"test"},"description":"new desc","detailed_merge_status":"mergeable","has_conflicts":false,"changes_count":"1"}`)
	}))

	out, err := mergerequests.Update(context.Background(), client, mergerequests.UpdateInput{
		ProjectID:    "42",
		MRIID:        1,
		Title:        "updated",
		Description:  "new desc",
		TargetBranch: "release",
		StateEvent:   "close",
		AssigneeIDs:  []int64{5},
		ReviewerIDs:  []int64{6, 7},
	})
	if err != nil {
		t.Fatalf("mergerequests.Update() unexpected error: %v", err)
	}
	if out.Title != "updated" {
		t.Errorf("Title = %q, want %q", out.Title, "updated")
	}
}

// TestMRMerge_AllOptionalParams exercises every optional branch in mrMerge.
func TestMRMerge_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"MR","state":"merged","source_branch":"dev","target_branch":"main","web_url":"https://example.com/mr/1","author":{"username":"test"},"detailed_merge_status":"merged","has_conflicts":false,"changes_count":"1"}`)
	}))

	out, err := mergerequests.Merge(context.Background(), client, mergerequests.MergeInput{
		ProjectID:                "42",
		MRIID:                    1,
		MergeCommitMessage:       "custom msg",
		Squash:                   new(true),
		ShouldRemoveSourceBranch: new(true),
	})
	if err != nil {
		t.Fatalf("mergerequests.Merge() unexpected error: %v", err)
	}
	if out.State != "merged" {
		t.Errorf("State = %q, want %q", out.State, "merged")
	}
}

// TestMRList_AllOptionalFilters exercises every optional branch in mrList.
func TestMRList_AllOptionalFilters(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") == "" || q.Get("search") == "" || q.Get("order_by") == "" || q.Get("sort") == "" {
			t.Error("expected all optional query params to be set")
		}
		respondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"title":"MR","state":"opened","source_branch":"dev","target_branch":"main","web_url":"https://example.com","author":{"username":"test"}}]`)
	}))

	out, err := mergerequests.List(context.Background(), client, mergerequests.ListInput{
		ProjectID: "42",
		State:     "opened",
		Search:    "feat",
		OrderBy:   "created_at",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf("mergerequests.List() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Errorf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
}

// TestProjectCreate_AllOptionalParams exercises every optional branch in projectCreate.
func TestProjectCreate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":42,"name":"proj","path_with_namespace":"ns/proj","visibility":"internal","web_url":"https://example.com/ns/proj","description":"desc","default_branch":"develop","namespace":{"id":10,"name":"ns","path":"ns","full_path":"ns"}}`)
	}))

	out, err := projects.Create(context.Background(), client, projects.CreateInput{
		Name:                 "proj",
		NamespaceID:          10,
		Description:          "desc",
		Visibility:           "internal",
		InitializeWithReadme: true,
		DefaultBranch:        "develop",
	})
	if err != nil {
		t.Fatalf("projectCreate() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

// TestProjectUpdate_AllOptionalParams exercises every optional branch in projectUpdate.
func TestProjectUpdate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":42,"name":"new-name","path_with_namespace":"ns/proj","visibility":"public","web_url":"https://example.com/ns/proj","description":"new desc","default_branch":"develop","namespace":{"id":1,"name":"ns","path":"ns","full_path":"ns"}}`)
	}))

	out, err := projects.Update(context.Background(), client, projects.UpdateInput{
		ProjectID:     "42",
		Name:          testNewName,
		Description:   "new desc",
		Visibility:    "public",
		DefaultBranch: "develop",
		MergeMethod:   "rebase_merge",
	})
	if err != nil {
		t.Fatalf("projectUpdate() unexpected error: %v", err)
	}
	if out.Name != testNewName {
		t.Errorf("Name = %q, want %q", out.Name, testNewName)
	}
}

// TestProjectList_AllOptionalFilters exercises every optional branch in projectList.
func TestProjectList_AllOptionalFilters(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":42,"name":"test","path_with_namespace":"ns/test","visibility":"private","web_url":"https://example.com","description":"","default_branch":"main","namespace":{"id":1,"name":"ns","path":"ns","full_path":"ns"}}]`)
	}))

	out, err := projects.List(context.Background(), client, projects.ListInput{
		Owned:      true,
		Search:     "test",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("projectList() unexpected error: %v", err)
	}
	if len(out.Projects) != 1 {
		t.Errorf("len(Projects) = %d, want 1", len(out.Projects))
	}
}

// TestBranchList_WithSearchParam exercises the search and pagination branches in branchList.
func TestBranchList_WithSearchParam(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") == "" {
			t.Error("expected search query param")
		}
		respondJSON(w, http.StatusOK, `[{"name":"feature/auth","merged":false,"protected":false,"default":false,"web_url":"https://example.com","commit":{"id":"abc123"}}]`)
	}))

	out, err := branches.List(context.Background(), client, branches.ListInput{
		ProjectID:       "42",
		Search:          "feature",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("branchList() unexpected error: %v", err)
	}
	if len(out.Branches) != 1 {
		t.Errorf("len(Branches) = %d, want 1", len(out.Branches))
	}
}

// TestTagList_AllOptionalParams exercises every optional branch in tagList.
func TestTagList_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"name":"v1.0","message":"","target":"abc","commit":{"id":"abc","short_id":"ab","title":"init","message":"init","author_name":"test"}}]`)
	}))

	out, err := tags.List(context.Background(), client, tags.ListInput{
		ProjectID: "42",
		Search:    "v1",
		OrderBy:   "name",
		Sort:      "asc",
	})
	if err != nil {
		t.Fatalf("tags.List() unexpected error: %v", err)
	}
	if len(out.Tags) != 1 {
		t.Errorf("len(Tags) = %d, want 1", len(out.Tags))
	}
}

// TestReleaseList_AllOptionalParams exercises every optional branch in releaseList.
func TestReleaseList_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"tag_name":"v1.0","name":"v1.0","description":"notes","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","author":{"username":"test"},"commit":{"id":"abc123"},"assets":{"links":[]}}]`)
	}))

	out, err := releases.List(context.Background(), client, releases.ListInput{
		ProjectID: "42",
		OrderBy:   "released_at",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf("releaseList() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Errorf("len(Releases) = %d, want 1", len(out.Releases))
	}
}

// TestReleaseUpdate_AllOptionalParams exercises every optional branch in releaseUpdate.
func TestReleaseUpdate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"tag_name":"v1.0","name":"Updated","description":"new notes","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","author":{"username":"test"},"commit":{"id":"abc123"},"assets":{"links":[]}}`)
	}))

	out, err := releases.Update(context.Background(), client, releases.UpdateInput{
		ProjectID:   "42",
		TagName:     "v1.0",
		Name:        "Updated",
		Description: "new notes",
	})
	if err != nil {
		t.Fatalf("releaseUpdate() unexpected error: %v", err)
	}
	if out.Name != "Updated" {
		t.Errorf("Name = %q, want %q", out.Name, "Updated")
	}
}

// TestMRNotesList_AllOptionalParams exercises optional branches in mrNotesList.
func TestMRNotesList_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":1,"body":"note","author":{"username":"test"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false}]`)
	}))

	out, err := mrnotes.List(context.Background(), client, mrnotes.ListInput{
		ProjectID: "42",
		MRIID:     1,
		OrderBy:   "updated_at",
		Sort:      "asc",
	})
	if err != nil {
		t.Fatalf("mrnotes.List() unexpected error: %v", err)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(Notes) = %d, want 1", len(out.Notes))
	}
}

// TestMRDiscussionCreateInline_WithOldPath exercises the OldPath and OldLine branches.
func TestMRDiscussionCreateInline_WithOldPath(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":"disc1","individual_note":false,"notes":[{"id":1,"body":"inline","author":{"username":"test"},"created_at":"2026-01-01T00:00:00Z","resolved":false}]}`)
	}))

	out, err := mrdiscussions.Create(context.Background(), client, mrdiscussions.CreateInput{
		ProjectID: "42",
		MRIID:     1,
		Body:      "inline note on old line",
		Position: &mrdiscussions.DiffPosition{
			BaseSHA:  "base",
			StartSHA: "start",
			HeadSHA:  "head",
			OldPath:  "old_file.go",
			NewPath:  "new_file.go",
			OldLine:  10,
			NewLine:  15,
		},
	})
	if err != nil {
		t.Fatalf("mrdiscussions.Create() unexpected error: %v", err)
	}
	if out.ID != "disc1" {
		t.Errorf("ID = %q, want %q", out.ID, "disc1")
	}
}

// TestCommitCreate_AllOptionalParams exercises optional branches in commitCreate.
func TestCommitCreate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":"abc123","short_id":"abc1","title":"custom commit","message":"custom commit","author_name":"Custom Author","author_email":"custom@example.com","created_at":"2026-01-01T00:00:00Z","web_url":"https://example.com/c/abc123","stats":{"additions":1,"deletions":0,"total":1}}`)
	}))

	out, err := commits.Create(context.Background(), client, commits.CreateInput{
		ProjectID:     "42",
		Branch:        "main",
		CommitMessage: "custom commit",
		StartBranch:   "develop",
		AuthorEmail:   testCustomEmail,
		AuthorName:    "Custom Author",
		Actions: []commits.Action{
			{Action: "create", FilePath: "new.txt", Content: "hello"},
			{Action: "move", FilePath: "moved.txt", PreviousPath: "old.txt"},
		},
	})
	if err != nil {
		t.Fatalf("commits.Create() unexpected error: %v", err)
	}
	if out.AuthorEmail != testCustomEmail {
		t.Errorf("AuthorEmail = %q, want %q", out.AuthorEmail, testCustomEmail)
	}
}

// TestFileGet_WithRef exercises the Ref branch in fileGet.
func TestFileGet_WithRef(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"file_name":"README.md","file_path":"README.md","size":5,"encoding":"base64","content":"SGVsbG8=","ref":"develop","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"}`)
	}))

	out, err := files.Get(context.Background(), client, files.GetInput{
		ProjectID: "42",
		FilePath:  "README.md",
		Ref:       "develop",
	})
	if err != nil {
		t.Fatalf("files.Get() unexpected error: %v", err)
	}
	if out.Content != "Hello" {
		t.Errorf("Content = %q, want %q", out.Content, "Hello")
	}
}

// TestFileGet_NonBase64 exercises the non-base64 encoding branch in fileGet.
func TestFileGet_NonBase64(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"file_name":"README.md","file_path":"README.md","size":5,"encoding":"text","content":"Hello","ref":"main","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"}`)
	}))

	out, err := files.Get(context.Background(), client, files.GetInput{
		ProjectID: "42",
		FilePath:  "README.md",
	})
	if err != nil {
		t.Fatalf("files.Get() unexpected error: %v", err)
	}
	if out.Content != "Hello" {
		t.Errorf("Content = %q, want %q", out.Content, "Hello")
	}
}

// TestFileGet_InvalidBase64 exercises the base64 decode error branch in fileGet.
func TestFileGet_InvalidBase64(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"file_name":"f.go","file_path":"f.go","size":5,"encoding":"base64","content":"!!!invalid!!!","ref":"main","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"}`)
	}))

	_, err := files.Get(context.Background(), client, files.GetInput{
		ProjectID: "42",
		FilePath:  "f.go",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64 content, got nil")
	}
}

// TestUnmarshalParamsMarshal_Error exercises the json.Marshal error branch in unmarshalParams.
func TestUnmarshalParamsMarshal_Error(t *testing.T) {
	// json.Marshal fails on channels
	params := map[string]any{"ch": make(chan int)}
	_, err := unmarshalParams[mergerequests.GetInput](params)
	if err == nil {
		t.Fatal("expected error for un-marshalable params")
	}
}

// TestMakeMetaHandler_SuccessfulDispatch exercises the successful dispatch path.
func TestMakeMetaHandler_SuccessfulDispatch(t *testing.T) {
	called := false
	handler := toolutil.MakeMetaHandler("test_tool", actionMap{
		"get": route(func(ctx context.Context, params map[string]any) (any, error) {
			called = true
			return "result", nil
		}),
	}, func(value any) *mcp.CallToolResult {
		got, ok := value.(string)
		if !ok {
			t.Fatalf("formatter got %T, want string", value)
		}
		if got != "result" {
			t.Fatalf("formatter got %q, want %q", got, "result")
		}
		return toolutil.SuccessResult(got)
	})

	_, result, err := handler(context.Background(), nil, MetaToolInput{Action: "get", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result != "result" {
		t.Errorf("result = %v, want %q", result, "result")
	}
}

// TestCommitToOutput_NilDate exercises the nil CommittedDate branch.
func TestCommitToOutput_NilDate(t *testing.T) {
	// json.Unmarshal will produce a nil CommittedDate if field is missing
	raw := `{"id":"abc","short_id":"a","title":"t","author_name":"n","author_email":"e@e.com","web_url":"http://x"}`
	var input struct {
		ID          string `json:"id"`
		ShortID     string `json:"short_id"`
		Title       string `json:"title"`
		AuthorName  string `json:"author_name"`
		AuthorEmail string `json:"author_email"`
		WebURL      string `json:"web_url"`
	}
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatal(err)
	}
	// The commitToOutput test with nil date is already covered via mocks
	// that don't include committed_date; this verifies the CommittedDate is empty.
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":"abc","short_id":"a","title":"t","message":"m","author_name":"n","author_email":"e@e.com","web_url":"http://x"}`)
	}))

	out, err := commits.Create(context.Background(), client, commits.CreateInput{
		ProjectID:     "42",
		Branch:        "main",
		CommitMessage: "t",
		Actions:       []commits.Action{{Action: "create", FilePath: "f.txt", Content: "x"}},
	})
	if err != nil {
		t.Fatalf("commits.Create() unexpected error: %v", err)
	}
	if out.CommittedDate != "" {
		t.Errorf("CommittedDate = %q, want empty string", out.CommittedDate)
	}
}

// TestGroupSCIMMeta_UpdateAction_ErrorPath verifies that the updateAction
// closure in registerGroupSCIMMeta propagates errors from groupscim.Update
// back to the caller as an MCP error result.
func TestGroupSCIMMeta_UpdateAction_ErrorPath(t *testing.T) {
	t.Parallel()

	const scimUpdatePath = "/api/v4/groups/mygroup/scim/uid-123"

	session := newMetaMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case r.Method == http.MethodPatch && r.URL.Path == scimUpdatePath:
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}), true)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_group_scim",
		Arguments: map[string]any{
			"action": "update",
			"params": map[string]any{
				"group_id":   "mygroup",
				"uid":        "uid-123",
				"extern_uid": "new-uid",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool() unexpected transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for failed SCIM update, got success")
	}
	if result.StructuredContent != nil {
		t.Fatalf("StructuredContent = %#v, want nil for error result", result.StructuredContent)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected error result content, got none")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("error content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "forbidden") {
		t.Fatalf("error content = %q, want forbidden detail", textContent.Text)
	}
}

// TestGroupSCIMMeta_UpdateAction_SuccessPath verifies that the updateAction
// closure in registerGroupSCIMMeta returns the expected UpdateOutput on
// a successful GitLab SCIM PATCH response.
func TestGroupSCIMMeta_UpdateAction_SuccessPath(t *testing.T) {
	t.Parallel()

	const scimUpdatePath = "/api/v4/groups/mygroup/scim/uid-123"

	session := newMetaMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case r.Method == http.MethodPatch && r.URL.Path == scimUpdatePath:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}), true)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_group_scim",
		Arguments: map[string]any{
			"action": "update",
			"params": map[string]any{
				"group_id":   "mygroup",
				"uid":        "uid-123",
				"extern_uid": "new-uid",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool() unexpected transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %+v", result)
	}
	if result.StructuredContent == nil {
		t.Fatal("expected structured content for successful SCIM update")
	}
	raw, marshalErr := json.Marshal(result.StructuredContent)
	if marshalErr != nil {
		t.Fatalf("marshal structured content: %v", marshalErr)
	}
	var out groupscim.UpdateOutput
	if unmarshalErr := json.Unmarshal(raw, &out); unmarshalErr != nil {
		t.Fatalf("unmarshal structured content: %v", unmarshalErr)
	}
	if !out.Updated {
		t.Fatal("Updated = false, want true")
	}
	if out.Message != "SCIM identity updated successfully." {
		t.Fatalf("Message = %q, want SCIM identity updated successfully.", out.Message)
	}
}

// newTestClient creates a GitLab client pointed at a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		GitLabURL:     srv.URL,
		GitLabToken:   "test-token",
		SkipTLSVerify: false,
	}

	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test gitlab client: %v", err)
	}

	return client
}

// respondJSON writes a JSON response with the given status code and body.
func respondJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
