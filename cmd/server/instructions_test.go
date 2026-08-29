// instructions_test.go verifies the handshake instructions built by
// instructions.go. The instructions are injected into every model's system
// prompt, so a tool named there that the active surface does not expose sends
// the model straight at a tool call that cannot succeed — which is exactly the
// state this file was written to end.
package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// instructionSurfaces is every tool surface a server can register.
var instructionSurfaces = []string{
	config.ToolSurfaceDynamic,
	config.ToolSurfaceMeta,
	config.ToolSurfaceIndividual,
}

// TestBuildInstructions_NamesResolveOnEverySurface verifies that every tool
// name and action ID the handshake instructions mention is actually callable
// on the surface they were built for.
//
// It parses the rendered prose rather than trusting the reference table, so a
// hand-written name that never went through surfaceToolRef is caught too. Tool
// names are checked against tools/list; action IDs against the gitlab://tools
// manifest, which enumerates everything reachable through a dispatcher.
func TestBuildInstructions_NamesResolveOnEverySurface(t *testing.T) {
	client := newMockGitLabClient(t)
	for _, surface := range instructionSurfaces {
		t.Run(surface, func(t *testing.T) {
			server := mustCreateServer(t, client, &config.ServerConfig{
				MetaTools:         true,
				ToolSurface:       surface,
				CapabilitySurface: config.CapabilitySurfaceFull,
			})
			session := newInMemorySession(t, server)

			instructions := session.InitializeResult().Instructions
			if instructions == "" {
				t.Fatal("server advertised no instructions")
			}
			assertInstructionToolNamesExist(t, session, instructions)
			assertInstructionActionIDsExist(t, session, instructions)
		})
	}
}

// instructionToolNamePattern matches the gitlab_* tool names the prose cites.
var instructionToolNamePattern = regexp.MustCompile(`gitlab_[a-z0-9_]+`)

// instructionActionIDPattern matches the action="domain.action" call shapes the
// prose cites.
var instructionActionIDPattern = regexp.MustCompile(`action="([a-z0-9_]+\.[a-z0-9_]+)"`)

func assertInstructionToolNamesExist(t *testing.T, session *mcp.ClientSession, instructions string) {
	t.Helper()

	visible := map[string]bool{}
	for tool, err := range session.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		visible[tool.Name] = true
	}

	seen := map[string]bool{}
	for _, name := range instructionToolNamePattern.FindAllString(instructions, -1) {
		// The server names itself in the opening sentence; it is not a tool.
		if name == "gitlab_mcp_server" || seen[name] {
			continue
		}
		seen[name] = true
		if !visible[name] {
			t.Errorf("instructions name tool %q, which this surface does not expose", name)
		}
	}
	if len(seen) == 0 {
		t.Error("instructions name no tools at all — the guidance lost its call sites")
	}
}

func assertInstructionActionIDsExist(t *testing.T, session *mcp.ClientSession, instructions string) {
	t.Helper()

	matches := instructionActionIDPattern.FindAllStringSubmatch(instructions, -1)
	if len(matches) == 0 {
		// Only the dynamic and meta surfaces dispatch by action; the
		// individual surface names tools directly.
		return
	}

	reachable := readReachableActionIDs(t, session)
	for _, m := range matches {
		id := m[1]
		if !reachable[id] {
			t.Errorf("instructions reference action %q, which is not reachable on this surface", id)
		}
	}
}

// readReachableActionIDs returns the action IDs the gitlab://tools manifest
// advertises, plus the meta tool.action pairs, so both dispatch shapes can be
// checked with one lookup.
func readReachableActionIDs(t *testing.T, session *mcp.ClientSession) map[string]bool {
	t.Helper()

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools"})
	if err != nil {
		t.Fatalf("read gitlab://tools: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("gitlab://tools returned no contents")
	}

	var manifest struct {
		Entries []struct {
			ID     string `json:"id"`
			Tool   string `json:"tool"`
			Action string `json:"action"`
		} `json:"entries"`
	}
	if unmarshalErr := json.Unmarshal([]byte(result.Contents[0].Text), &manifest); unmarshalErr != nil {
		t.Fatalf("unmarshal tool manifest: %v", unmarshalErr)
	}

	reachable := make(map[string]bool, len(manifest.Entries)*2)
	for _, e := range manifest.Entries {
		reachable[e.ID] = true
		if e.Action != "" {
			reachable[e.Action] = true
		}
	}
	return reachable
}

// TestBuildInstructions_SurfacesDifferInNamesNotAdvice verifies that the three
// surfaces carry the same guidance sections while naming different call
// shapes. A surface that silently dropped a section would lose advice the
// other two rely on.
func TestBuildInstructions_SurfacesDifferInNamesNotAdvice(t *testing.T) {
	sections := []string{
		"PROJECT DISCOVERY",
		"DEFAULT BRANCH",
		"PACKAGE + RELEASE WORKFLOW",
		"RELEASE CREATION",
		"ID vs IID",
		"WATCHING RESOURCES",
	}

	rendered := make(map[string]string, len(instructionSurfaces))
	for _, surface := range instructionSurfaces {
		text := buildInstructions(surface, config.CapabilitySurfaceFull, false)
		rendered[surface] = text
		for _, section := range sections {
			if !strings.Contains(text, section) {
				t.Errorf("surface %q instructions are missing the %q section", surface, section)
			}
		}
	}

	for i := 1; i < len(instructionSurfaces); i++ {
		a, b := instructionSurfaces[i-1], instructionSurfaces[i]
		if rendered[a] == rendered[b] {
			t.Errorf("surfaces %q and %q produced identical instructions; names should differ", a, b)
		}
	}
}

// TestBuildInstructions_DynamicExplainsTheTwoToolWorkflow verifies the default
// surface tells the model how to reach the catalog. Dynamic mode exposes only
// find and execute, so without this a model sees action IDs and no way in.
func TestBuildInstructions_DynamicExplainsTheTwoToolWorkflow(t *testing.T) {
	dynamic := buildInstructions(config.ToolSurfaceDynamic, config.CapabilitySurfaceFull, false)
	for _, want := range []string{"gitlab_find_action", "gitlab_execute_action", "FINDING TOOLS"} {
		if !strings.Contains(dynamic, want) {
			t.Errorf("dynamic instructions missing %q", want)
		}
	}

	// The other surfaces must not advertise a workflow they cannot run.
	for _, surface := range []string{config.ToolSurfaceMeta, config.ToolSurfaceIndividual} {
		if strings.Contains(buildInstructions(surface, config.CapabilitySurfaceFull, false), "gitlab_find_action") {
			t.Errorf("surface %q instructions mention gitlab_find_action, which only dynamic mode exposes", surface)
		}
	}
}

// TestSurfaceToolRef_Render_PerSurfaceShapes verifies the three call shapes,
// including the standalone case: a utility with no meta dispatcher keeps its
// own tool name on the meta surface rather than being addressed by action.
func TestSurfaceToolRef_Render_PerSurfaceShapes(t *testing.T) {
	tests := []struct {
		name    string
		ref     surfaceToolRef
		surface string
		want    string
	}{
		{
			name:    "dynamic addresses the canonical action",
			ref:     refProjectGet,
			surface: config.ToolSurfaceDynamic,
			want:    `action="project.get"`,
		},
		{
			name:    "meta addresses the dispatcher and action",
			ref:     refProjectGet,
			surface: config.ToolSurfaceMeta,
			want:    `gitlab_project with action="get"`,
		},
		{
			name:    "individual names the tool directly",
			ref:     refProjectGet,
			surface: config.ToolSurfaceIndividual,
			want:    "gitlab_project_get",
		},
		{
			name:    "standalone utility keeps its tool name on the meta surface",
			ref:     refDiscoverProject,
			surface: config.ToolSurfaceMeta,
			want:    "gitlab_discover_project",
		},
		{
			name:    "standalone utility is still an action on the dynamic surface",
			ref:     refDiscoverProject,
			surface: config.ToolSurfaceDynamic,
			want:    `action="discover_project.resolve"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.render(tt.surface); got != tt.want {
				t.Errorf("render(%q) = %q, want %q", tt.surface, got, tt.want)
			}
		})
	}
}

// TestBuildInstructions_WatchingSection_FollowsCapabilitySurface verifies
// the model is only told about subscriptions where the server would honor
// one.
//
// The minimal surface does not advertise resources.subscribe, so
// instructions mentioning it there would teach the model to make requests
// this server refuses — advice worse than silence.
func TestBuildInstructions_WatchingSection_FollowsCapabilitySurface(t *testing.T) {
	full := buildInstructions(config.ToolSurfaceDynamic, config.CapabilitySurfaceFull, false)
	if !strings.Contains(full, "WATCHING RESOURCES") {
		t.Error("full capability surface instructions omit the watching section; the feature is invisible to the model")
	}
	if !strings.Contains(full, "via MCP resources/subscribe") {
		t.Error("the watching section never names resources/subscribe, so the model cannot map it to the protocol")
	}

	minimal := buildInstructions(config.ToolSurfaceDynamic, config.CapabilitySurfaceMinimal, false)
	if strings.Contains(minimal, "WATCHING RESOURCES") {
		t.Error("minimal surface instructions advertise subscriptions the server refuses there")
	}
}

// TestBuildInstructions_StatelessHTTP_NamesTheWorkingMethod verifies the
// instructions never teach a stateless-HTTP model the request that
// transport refuses.
//
// On stateless HTTP the legacy resources/subscribe is answered with an
// error, while subscriptions/listen works; instructions that named only the
// former would send the model straight into a refusal.
//
// This assertion was true of the text and false of the server for as long as
// the stateless refusal also killed subscriptions/listen: the sentence it pins
// promised a working method where none was. Keeping it is only worth anything
// alongside the wire test that drives the method, in
// test/e2e/http/subscriptions_test.go — a claim about a string cannot tell you
// whether the thing it names works.
//
// The revision is pinned too. A client that negotiated an earlier one cannot
// watch resources on this transport by any route, so instructions that named
// the method without saying which revision it belongs to would leave it
// looking for something it has no way to call.
func TestBuildInstructions_StatelessHTTP_NamesTheWorkingMethod(t *testing.T) {
	stateless := buildInstructions(config.ToolSurfaceDynamic, config.CapabilitySurfaceFull, true)
	if !strings.Contains(stateless, "WATCHING RESOURCES") {
		t.Fatal("stateless instructions dropped the watching section entirely; subscriptions/listen does work there")
	}
	if !strings.Contains(stateless, "subscriptions/listen") {
		t.Error("stateless instructions do not name subscriptions/listen, the one method that transport honors")
	}
	if !strings.Contains(stateless, "2026-07-28") {
		t.Error("stateless instructions name the method without the revision that introduced it")
	}
	if strings.Contains(stateless, "via MCP resources/subscribe") {
		t.Error("stateless instructions teach resources/subscribe, which that transport refuses")
	}
}
