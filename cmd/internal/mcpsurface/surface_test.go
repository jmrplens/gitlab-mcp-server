// surface_test.go covers the shared MCP introspection helpers the generator
// commands build on: the pinned dynamic two-tool contract, resource and prompt
// discovery over a real in-memory round-trip, and project-root resolution.
package mcpsurface

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// newStubClientForTest builds the offline GitLab client the surface helpers run
// against and registers its shutdown with the test.
func newStubClientForTest(t *testing.T) *gitlabclient.Client {
	t.Helper()
	client, cleanup := NewStubClient()
	t.Cleanup(cleanup)
	return client
}

// TestDynamicTools_ExposesFindAndExecute verifies the shared helper exposes only
// the find and execute tools in deterministic order.
//
// The test lists the dynamic surface against the offline stub client and checks
// the execute input schema for action, params, and confirm fields. This protects
// the low-token dynamic contract every generated artifact describes.
func TestDynamicTools_ExposesFindAndExecute(t *testing.T) {
	dynamicTools := DynamicTools(newStubClientForTest(t))
	if len(dynamicTools) != 2 {
		t.Fatalf("len(DynamicTools()) = %d, want 2", len(dynamicTools))
	}
	names := []string{dynamicTools[0].Name, dynamicTools[1].Name}
	if names[0] != DynamicFindToolName || names[1] != DynamicExecuteActionToolName {
		t.Fatalf("dynamic tools = %v, want find before execute", names)
	}

	executeSchema, ok := dynamicTools[1].InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("execute InputSchema has type %T, want map[string]any", dynamicTools[1].InputSchema)
	}
	executeProperties, ok := executeSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("execute InputSchema properties has type %T, want map[string]any", executeSchema["properties"])
	}
	for _, property := range []string{"action", "params", "confirm"} {
		t.Run(property, func(t *testing.T) {
			if _, exists := executeProperties[property]; !exists {
				t.Fatalf("execute InputSchema missing %q property: %v", property, executeProperties)
			}
		})
	}
	required, ok := executeSchema["required"].([]any)
	if !ok {
		t.Fatalf("execute InputSchema required has type %T, want []any", executeSchema["required"])
	}
	if !slices.Contains(required, any("action")) || !slices.Contains(required, any("params")) {
		t.Fatalf("execute InputSchema required = %v, want action and params", required)
	}
}

// TestSortDynamicTools_PutsFindBeforeExecute verifies the ordering helper is
// independent of registration order and sorts unknown names last.
//
// The input arrives reversed with an extra tool appended, and the expected
// result keeps find, then execute, then anything else by name. This keeps the
// generated files stable when the SDK changes list ordering.
func TestSortDynamicTools_PutsFindBeforeExecute(t *testing.T) {
	dynamicTools := []*mcp.Tool{
		{Name: "zzz_unexpected"},
		{Name: DynamicExecuteActionToolName},
		{Name: "aaa_unexpected"},
		{Name: DynamicFindToolName},
	}
	SortDynamicTools(dynamicTools)

	want := []string{DynamicFindToolName, DynamicExecuteActionToolName, "aaa_unexpected", "zzz_unexpected"}
	for i, name := range want {
		t.Run(name, func(t *testing.T) {
			if dynamicTools[i].Name != name {
				t.Fatalf("position %d = %q, want %q", i, dynamicTools[i].Name, name)
			}
		})
	}
}

// TestValidateDynamicToolContract_RejectsDrift verifies the dynamic tool
// contract accepts the canonical pair and fails on every way it can drift.
//
// A rename, a dropped tool, an extra tool, or a swapped order all have to abort
// generation: the alternative is silently rewriting every generated artifact
// around a surface nobody meant to change.
func TestValidateDynamicToolContract_RejectsDrift(t *testing.T) {
	tests := []struct {
		name    string
		tools   []*mcp.Tool
		wantErr bool
	}{
		{
			name:  "canonical pair",
			tools: []*mcp.Tool{{Name: DynamicFindToolName}, {Name: DynamicExecuteActionToolName}},
		},
		{
			name:    "missing tool",
			tools:   []*mcp.Tool{{Name: DynamicExecuteActionToolName}},
			wantErr: true,
		},
		{
			name:    "renamed tool",
			tools:   []*mcp.Tool{{Name: DynamicFindToolName}, {Name: "gitlab_renamed"}},
			wantErr: true,
		},
		{
			name:    "extra tool",
			tools:   []*mcp.Tool{{Name: DynamicFindToolName}, {Name: DynamicExecuteActionToolName}, {Name: "gitlab_extra"}},
			wantErr: true,
		},
		{
			name:    "swapped order",
			tools:   []*mcp.Tool{{Name: DynamicExecuteActionToolName}, {Name: DynamicFindToolName}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDynamicToolContract(tt.tools)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateDynamicToolContract(%s) error = nil, want an error", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateDynamicToolContract(%s) error = %v", tt.name, err)
			}
		})
	}
}

// TestResources_IncludesToolManifestTemplate verifies resource discovery sees
// the unified tool manifest template alongside the regular resources.
func TestResources_IncludesToolManifestTemplate(t *testing.T) {
	res, templates := Resources(newStubClientForTest(t))
	if len(res) == 0 {
		t.Fatal("Resources() returned no static resources")
	}
	wantTemplates := map[string]bool{
		"gitlab://tools/{id}": false,
	}
	for _, template := range templates {
		if template.URITemplate == "gitlab://schema/meta/{tool}/{action}" || template.URITemplate == "gitlab://schema/dynamic/{action}" {
			t.Fatalf("Resources() exposed legacy schema template %s: %v", template.URITemplate, templates)
		}
		if _, ok := wantTemplates[template.URITemplate]; ok {
			wantTemplates[template.URITemplate] = true
		}
	}
	for uri, found := range wantTemplates {
		if !found {
			t.Fatalf("Resources() templates missing %s: %v", uri, templates)
		}
	}
}

// TestPrompts_ReturnsDescribedPrompts verifies prompt discovery returns the
// registered set with the metadata generated artifacts render.
//
// Every prompt must carry a name and a description, because a blank one would
// ship to the marketplace listing and to llms.txt as an empty entry.
func TestPrompts_ReturnsDescribedPrompts(t *testing.T) {
	list := Prompts(newStubClientForTest(t))
	if len(list) == 0 {
		t.Fatal("Prompts() returned no prompts, want the registered set")
	}
	for _, prompt := range list {
		if prompt.Name == "" || prompt.Description == "" {
			t.Fatalf("prompt %q is missing name or description", prompt.Name)
		}
	}
}

// TestSession_ListsWhatSetupRegistered verifies the session is connected to the
// server setup ran against, and that the returned cleanup shuts both ends down.
//
// The setup callback registers one throwaway tool rather than a real catalog, so
// this asserts the wiring alone: what setup put on the server is what a
// tools/list over the returned session comes back with.
func TestSession_ListsWhatSetupRegistered(t *testing.T) {
	session, cleanup := Session(func(server *mcp.Server) {
		mcp.AddTool(server,
			&mcp.Tool{Name: "probe", Description: "A tool registered only by this test."},
			func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
				return nil, nil, nil
			})
	})
	defer cleanup()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "probe" {
		t.Fatalf("ListTools() = %v, want only the tool setup registered", result.Tools)
	}
}

// TestNewGitLabComClient_UsesPublicHost verifies the GitLab.com client is pinned
// to the public host rather than to whatever GITLAB_URL points at, which is what
// keeps generated output identical between a developer machine and CI.
func TestNewGitLabComClient_UsesPublicHost(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")

	client := NewGitLabComClient()
	if !client.IsGitLabDotCom() {
		t.Error("NewGitLabComClient() is not configured for GitLab.com")
	}
	got := client.GL().BaseURL().String()
	if !strings.HasPrefix(got, config.DefaultGitLabURL) {
		t.Errorf("base URL = %q, want it under %q", got, config.DefaultGitLabURL)
	}
}

// TestProjectRoot_FindsGoMod verifies the walk up from the package directory
// lands on the directory holding go.mod.
func TestProjectRoot_FindsGoMod(t *testing.T) {
	root, err := ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot() error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr != nil {
		t.Fatalf("ProjectRoot() = %q, which holds no go.mod: %v", root, statErr)
	}
}

// TestProjectRoot_NotFound verifies the walk reports an error instead of
// returning the filesystem root when no go.mod exists above the caller.
func TestProjectRoot_NotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := ProjectRoot(); err == nil {
		t.Fatal("ProjectRoot() error = nil, want an error in a tree without go.mod")
	}
}

// TestProjectRoot_RemovedWorkingDirectory_ReturnsError verifies the walk
// reports the os.Getwd failure when the process's working directory was
// removed: getcwd(3) then fails with ENOENT regardless of privilege, which
// is what makes the branch reproducible in a test.
func TestProjectRoot_RemovedWorkingDirectory_ReturnsError(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(gone)
	// Windows refuses to remove a process's working directory, and macOS
	// keeps answering getcwd from the path it remembers, so on neither can
	// the failure be produced this way: both skip rather than report the
	// operating system's design as a defect here.
	if err := os.RemoveAll(gone); err != nil {
		t.Skipf("this platform will not remove the working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed")
	}

	_, err := ProjectRoot()
	if err == nil || !strings.Contains(err.Error(), "get working directory") {
		t.Fatalf("ProjectRoot() error = %v, want the working-directory error", err)
	}
}
