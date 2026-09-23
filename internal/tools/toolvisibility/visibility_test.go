// visibility_test.go pins the one pass that decides which registered tool a
// narrowed deployment serves: what each of its three steps removes or wraps,
// what it leaves alone, which tools safe mode exempts on each surface, and
// what happens on a server it cannot list.
package toolvisibility

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// testSchemaCache is shared by every server these tests build, so the tools
// registered by name below resolve their schemas once per process.
var testSchemaCache = mcp.NewSchemaCache()

// The four interactive flows the meta surface registers outside the catalog,
// which are the tools the pass exists for.
var interactiveTools = []string{
	"gitlab_interactive_issue_create",
	"gitlab_interactive_mr_create",
	"gitlab_interactive_project_create",
	"gitlab_interactive_release_create",
}

// newServer builds an empty server the way every test here starts.
func newServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: "toolvisibility-test", Version: "0"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
}

// addTool registers a tool by name whose handler counts its calls and answers
// "ran", read-only or mutating as told, so a test can tell a wrapped tool from
// one left alone by what a call reaches.
func addTool(server *mcp.Server, name string, readOnly bool) *atomic.Int64 {
	var calls atomic.Int64
	server.AddTool(&mcp.Tool{
		Name:        name,
		Description: name,
		InputSchema: map[string]any{"type": "object"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ran"}}}, nil
	})
	return &calls
}

// listNames returns the sorted names tools/list serves on server.
func listNames(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	tools, err := toolutil.ListRegisteredTools(t.Context(), server, "toolvisibility-list")
	if err != nil {
		t.Fatalf("ListRegisteredTools() error = %v", err)
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// callText calls one tool over an in-memory session and returns its text, so
// a preview can be told from the handler's own answer.
func callText(t *testing.T, server *mcp.Server, name string, args map[string]any) string {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "toolvisibility-caller", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s) error = %v", name, err)
	}
	var text strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			text.WriteString(textContent.Text)
		}
	}
	return text.String()
}

// newGroupsCatalog builds a catalog with one read-only list action under each
// of the named group tools, which is all the exemption set reads of one.
func newGroupsCatalog(t *testing.T, groups ...string) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	for _, group := range groups {
		if err := catalog.AddAction(group, actioncatalog.Action{Name: "list", ReadOnly: true}); err != nil {
			t.Fatalf("AddAction(%s) error = %v", group, err)
		}
	}
	return catalog
}

// previewOf asserts text is a safe-mode preview and returns it.
func previewOf(t *testing.T, text string) toolutil.SafeModePreview {
	t.Helper()
	preview, isPreview := toolutil.ParseSafeModePreview(text)
	if !isPreview {
		t.Fatalf("answer = %q, want a safe-mode preview", text)
	}
	return preview
}

// TestApply_DefaultMode_TouchesNothing verifies a deployment that narrows
// nothing is served exactly what it registered and told nothing about it: the
// pass is a no-op rather than three no-ops each with a line in the log.
func TestApply_DefaultMode_TouchesNothing(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	server := newServer()
	writeCalls := addTool(server, "gitlab_write", false)
	addTool(server, "gitlab_read", true)

	Apply(t.Context(), server, &config.ServerConfig{}, config.ToolSurfaceIndividual, actioncatalog.NewCatalog())

	if got := listNames(t, server); !slices.Equal(got, []string{"gitlab_read", "gitlab_write"}) {
		t.Errorf("tools/list = %v, want both tools left alone", got)
	}
	if text := callText(t, server, "gitlab_write", map[string]any{}); text != "ran" || writeCalls.Load() != 1 {
		t.Errorf("the mutating tool answered %q after %d call(s), want its own handler to run", text, writeCalls.Load())
	}
	if logged.Len() != 0 {
		t.Errorf("log = %q, want nothing said about a pass that changed nothing", logged.String())
	}
}

// TestApply_ExcludeTools_RemovesTheRegisteredNamesItLists verifies the first
// step's exact-name rule, the one that reaches a tool registered outside the
// catalog that is no standalone utility: a name listed is removed when
// registered, a name nothing registered is ignored, and the startup line
// counts what this pass removed rather than what the catalog did, since on a
// surface whose exclusions were applied to the catalog the honest count here
// is zero. The standalone utilities are also reached by the catalog's rule,
// which the two tests after this one hold.
func TestApply_ExcludeTools_RemovesTheRegisteredNamesItLists(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	server := newServer()
	for _, name := range []string{"gitlab_issue", "gitlab_runner", "gitlab_project"} {
		addTool(server, name, false)
	}

	Apply(t.Context(), server, &config.ServerConfig{ExcludeTools: []string{"gitlab_runner", "gitlab_absent"}}, config.ToolSurfaceMeta, nil)

	if got := listNames(t, server); !slices.Equal(got, []string{"gitlab_issue", "gitlab_project"}) {
		t.Errorf("tools/list = %v, want the excluded tool gone and the other two left alone", got)
	}
	if log := logged.String(); !strings.Contains(log, `"excluded_registered_tools":1`) || !strings.Contains(log, "gitlab_absent") {
		t.Errorf("log = %q, want the one removal counted and the patterns named", log)
	}
}

// standaloneServer returns a server holding the standalone utilities as the
// meta and individual surfaces register them, beside one catalog tool, so a
// test can tell what an exclusion reached from what it left alone.
func standaloneServer(t *testing.T) *mcp.Server {
	t.Helper()
	server := newServer()
	gitlabtools.RegisterMetaStandaloneTools(server, testutil.NewTestClient(t, http.NotFoundHandler()))
	addTool(server, "gitlab_issue", false)
	return server
}

// TestApply_ExcludeTools_ReachesAStandaloneToolByEverySpelling verifies the
// case issue 911 is about, against the standalone tools the meta and
// individual surfaces really register: every spelling an operator may write
// for a standalone utility removes it from tools/list.
//
// Those two surfaces register the utilities as tools of their own, outside
// the catalog, so this pass is the only thing that can remove them, and it
// used to match registered names alone. The canonical ID and the group name
// therefore removed nothing here, while the documentation offered all three
// spellings and the canonical ID is the one that means the same thing on
// every surface.
func TestApply_ExcludeTools_ReachesAStandaloneToolByEverySpelling(t *testing.T) {
	everyTool := []string{
		"gitlab_discover_project",
		"gitlab_interactive_issue_create",
		"gitlab_interactive_mr_create",
		"gitlab_interactive_project_create",
		"gitlab_interactive_release_create",
		"gitlab_issue",
	}
	without := func(removed ...string) []string {
		return slices.DeleteFunc(slices.Clone(everyTool), func(name string) bool { return slices.Contains(removed, name) })
	}

	cases := []struct {
		name    string
		exclude []string
		want    []string
		// wantRemoved is the count the startup line must carry.
		wantRemoved string
	}{
		{name: "the tool name", exclude: []string{"gitlab_interactive_issue_create"}, want: without("gitlab_interactive_issue_create"), wantRemoved: `"excluded_registered_tools":1`},
		{name: "the canonical action ID", exclude: []string{"interactive.issue_create"}, want: without("gitlab_interactive_issue_create"), wantRemoved: `"excluded_registered_tools":1`},
		{name: "the group name", exclude: []string{"gitlab_interactive"}, want: without(interactiveTools...), wantRemoved: `"excluded_registered_tools":4`},
		{name: "discovery's canonical ID", exclude: []string{"discover_project.resolve"}, want: without("gitlab_discover_project"), wantRemoved: `"excluded_registered_tools":1`},
		{
			name:        "a spelling of each kind at once",
			exclude:     []string{"interactive.mr_create", "gitlab_discover_project", "gitlab_issue"},
			want:        without("gitlab_interactive_mr_create", "gitlab_discover_project", "gitlab_issue"),
			wantRemoved: `"excluded_registered_tools":3`,
		},
	}
	for _, surface := range []string{config.ToolSurfaceMeta, config.ToolSurfaceIndividual} {
		t.Run(surface, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					logged := testutil.CaptureSlog(t)
					server := standaloneServer(t)

					Apply(t.Context(), server, &config.ServerConfig{ExcludeTools: tc.exclude}, surface, nil)

					if got := listNames(t, server); !slices.Equal(got, tc.want) {
						t.Errorf("tools/list = %v, want %v", got, tc.want)
					}
					if log := logged.String(); !strings.Contains(log, tc.wantRemoved) {
						t.Errorf("log = %q, want %s", log, tc.wantRemoved)
					}
				})
			}
		})
	}
}

// TestApply_ExcludeTools_GroupNameResolvesAndIsNotAPrefix verifies that the
// group name removes the guided flows because the catalog's rule resolves it
// to them, not because they share its prefix: a registered tool whose name
// starts the same way and is no guided flow is left alone.
//
// Removing the flows by prefix would pass every other test here and remove
// more than the operator named, which a model reading tools/list cannot tell
// from a tool that never existed.
func TestApply_ExcludeTools_GroupNameResolvesAndIsNotAPrefix(t *testing.T) {
	server := standaloneServer(t)
	addTool(server, "gitlab_interactive_custom", false)

	Apply(t.Context(), server, &config.ServerConfig{ExcludeTools: []string{"gitlab_interactive"}}, config.ToolSurfaceMeta, nil)

	want := []string{"gitlab_discover_project", "gitlab_interactive_custom", "gitlab_issue"}
	if got := listNames(t, server); !slices.Equal(got, want) {
		t.Errorf("tools/list = %v, want %v: the four flows gone and the look-alike kept", got, want)
	}
}

// TestApply_ReadOnly_RemovesEveryToolWithoutAReadOnlyHint verifies the second
// step: read-only mode withdraws every tool that does not advertise itself as
// read-only, including one with no annotations at all, keeps the reads, and
// wins over safe mode, which would have nothing left to wrap.
func TestApply_ReadOnly_RemovesEveryToolWithoutAReadOnlyHint(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	server := newServer()
	addTool(server, "gitlab_read", true)
	addTool(server, "gitlab_write", false)
	server.AddTool(&mcp.Tool{Name: "gitlab_unannotated", Description: "no annotations", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})

	Apply(t.Context(), server, &config.ServerConfig{ReadOnly: true, SafeMode: true}, config.ToolSurfaceIndividual, nil)

	if got := listNames(t, server); !slices.Equal(got, []string{"gitlab_read"}) {
		t.Errorf("tools/list = %v, want only the read-only tool", got)
	}
	if log := logged.String(); !strings.Contains(log, "read-only mode: removed write tools") || !strings.Contains(log, `"removed":2`) {
		t.Errorf("log = %q, want the two removals reported", log)
	}
	if strings.Contains(logged.String(), "safe mode") {
		t.Errorf("log = %q, want safe mode not to run once read-only mode has removed the writes", logged.String())
	}
}

// TestApply_SafeMode_WrapsWhatTheCatalogDoesNotPreview verifies the third
// step on each surface: a mutating tool the catalog does not stand behind is
// answered with a preview of itself, a read runs, and the tools the catalog
// already previews per action keep their handlers, because wrapping a
// dispatcher would block the reads it also serves.
func TestApply_SafeMode_WrapsWhatTheCatalogDoesNotPreview(t *testing.T) {
	catalog := newGroupsCatalog(t, "gitlab_issue")

	cases := []struct {
		name    string
		surface string
		catalog *actioncatalog.Catalog
		// exempt names a mutating tool the pass must leave alone on this
		// surface, or "" when nothing is exempt.
		exempt string
	}{
		{name: "individual exempts nothing", surface: config.ToolSurfaceIndividual, catalog: catalog, exempt: ""},
		{name: "meta exempts the catalog groups", surface: config.ToolSurfaceMeta, catalog: catalog, exempt: "gitlab_issue"},
		{name: "dynamic exempts the execute tool", surface: config.ToolSurfaceDynamic, catalog: nil, exempt: dynamictools.ExecuteActionToolName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logged := testutil.CaptureSlog(t)
			server := newServer()
			standaloneCalls := addTool(server, "gitlab_interactive_thing_create", false)
			readCalls := addTool(server, "gitlab_read", true)
			var exemptCalls *atomic.Int64
			if tc.exempt != "" {
				exemptCalls = addTool(server, tc.exempt, false)
			}

			Apply(t.Context(), server, &config.ServerConfig{SafeMode: true}, tc.surface, tc.catalog)

			preview := previewOf(t, callText(t, server, "gitlab_interactive_thing_create", map[string]any{"title": "x"}))
			if preview.Tool != "gitlab_interactive_thing_create" || preview.Status != "blocked" || standaloneCalls.Load() != 0 {
				t.Errorf("preview = %+v after %d real call(s), want the standalone tool blocked and its handler never run", preview, standaloneCalls.Load())
			}
			if text := callText(t, server, "gitlab_read", map[string]any{}); text != "ran" || readCalls.Load() != 1 {
				t.Errorf("the read answered %q after %d call(s), want it to run", text, readCalls.Load())
			}
			if tc.exempt != "" {
				if text := callText(t, server, tc.exempt, map[string]any{}); text != "ran" || exemptCalls.Load() != 1 {
					t.Errorf("the exempt %s answered %q after %d call(s), want its own handler to run", tc.exempt, text, exemptCalls.Load())
				}
			}
			if log := logged.String(); !strings.Contains(log, "safe mode: intercepted mutating operations") || !strings.Contains(log, `"wrapped_tools":1`) {
				t.Errorf("log = %q, want the one wrapped tool reported", log)
			}
		})
	}
}

// TestCatalogBackedToolNames_ExemptsWhatPreviewsPerAction verifies the
// exemption set per surface: nothing on the individual surface, where one
// tool is one action; the two dynamic tools, which route every action through
// a catalog that previews; the group tools of a meta catalog; and nothing for
// a meta surface with no catalog to read, since with no catalog the reasoning
// that exempts a tool does not hold and the alternative is a mutating tool
// executing for real in a deployment that asked for previews.
func TestCatalogBackedToolNames_ExemptsWhatPreviewsPerAction(t *testing.T) {
	t.Parallel()

	catalog := newGroupsCatalog(t, "gitlab_issue", "gitlab_project")

	cases := []struct {
		name    string
		surface string
		catalog *actioncatalog.Catalog
		want    []string
	}{
		{name: "individual", surface: config.ToolSurfaceIndividual, catalog: catalog, want: nil},
		{name: "dynamic", surface: config.ToolSurfaceDynamic, catalog: catalog, want: []string{dynamictools.ExecuteActionToolName, dynamictools.FindActionToolName}},
		{name: "meta", surface: config.ToolSurfaceMeta, catalog: catalog, want: []string{"gitlab_issue", "gitlab_project"}},
		{name: "meta without a catalog", surface: config.ToolSurfaceMeta, catalog: nil, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exempt := catalogBackedToolNames(tc.catalog, tc.surface)
			got := make([]string, 0, len(exempt))
			for name := range exempt {
				got = append(got, name)
			}
			sort.Strings(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("catalogBackedToolNames(%s) = %v, want %v", tc.surface, got, tc.want)
			}
		})
	}
}

// TestApply_WithoutAServer_ChangesNothingAndSaysWhy verifies each step on a
// server it cannot list: nothing is removed or wrapped, nothing panics, and
// the failure is logged rather than swallowed, because a deployment that
// asked for a narrowing and silently got none is worse than one that fails.
func TestApply_WithoutAServer_ChangesNothingAndSaysWhy(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.ServerConfig
		want string
	}{
		{name: "exclusions", cfg: &config.ServerConfig{ExcludeTools: []string{"gitlab_issue"}}, want: "exclude-tools: list registered tools failed"},
		{name: "read-only", cfg: &config.ServerConfig{ReadOnly: true}, want: "RemoveNonReadOnlyTools: list registered tools failed"},
		{name: "safe mode", cfg: &config.ServerConfig{SafeMode: true}, want: "WrapMutatingToolsForSafeMode: list registered tools failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logged := testutil.CaptureSlog(t)

			Apply(t.Context(), nil, tc.cfg, config.ToolSurfaceMeta, nil)

			if !strings.Contains(logged.String(), tc.want) {
				t.Errorf("log = %q, want %q", logged.String(), tc.want)
			}
		})
	}
}

// TestApply_MetaSurface_ReachesTheInteractiveFlows verifies the case issue 617
// is about, against the registration cmd/server makes: the meta catalog the
// shared assembler builds plus the standalone flows. In read-only mode the
// four gitlab_interactive_* tools are withdrawn and a catalog read still
// reaches GitLab; in safe mode they answer with a preview and a catalog read
// still reaches GitLab.
//
// The defect it was written for belonged to an evaluator that registered a
// server of its own, where either mode kept the flows' real handlers. That
// evaluator is gone and the one under test/e2e drives the binary, so no second
// registration exists to drift; this test is what holds the policy itself,
// which cmd/server's exemption from the coverage rule would otherwise leave
// unasserted.
func TestApply_MetaSurface_ReachesTheInteractiveFlows(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/issues") {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"title":"reached gitlab"}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	// register builds the meta surface for cfg the way cmd/server does and
	// returns the server with the pass applied.
	register := func(t *testing.T, cfg *config.ServerConfig) *mcp.Server {
		t.Helper()
		cfg.Tier = edition.Free
		catalog, _, err := gitlabtools.SharedMetaCatalog(client, cfg)
		if err != nil {
			t.Fatalf("SharedMetaCatalog() error = %v", err)
		}
		server := newServer()
		gitlabtools.RegisterMetaCatalog(server, catalog)
		gitlabtools.RegisterMetaStandaloneTools(server, client)
		Apply(t.Context(), server, cfg, config.ToolSurfaceMeta, catalog)
		return server
	}
	// readReachesGitLab asserts the catalog read still runs after the pass.
	readReachesGitLab := func(t *testing.T, server *mcp.Server) {
		t.Helper()
		text := callText(t, server, "gitlab_issue", map[string]any{"action": "list", "params": map[string]any{"project_id": "1"}})
		if !strings.Contains(text, "reached gitlab") {
			t.Errorf("gitlab_issue list answered %q, want the issue the mock GitLab serves", text)
		}
	}

	t.Run("read-only withdraws the flows", func(t *testing.T) {
		server := register(t, &config.ServerConfig{ReadOnly: true})

		names := listNames(t, server)
		for _, flow := range interactiveTools {
			t.Run(flow, func(t *testing.T) {
				if slices.Contains(names, flow) {
					t.Errorf("tools/list still serves %s in read-only mode", flow)
				}
			})
		}
		if !slices.Contains(names, "gitlab_issue") {
			t.Errorf("tools/list = %v, want the catalog dispatcher kept", names)
		}
		readReachesGitLab(t, server)
	})

	t.Run("safe mode previews the flows", func(t *testing.T) {
		server := register(t, &config.ServerConfig{SafeMode: true})

		names := listNames(t, server)
		for _, flow := range interactiveTools {
			t.Run(flow, func(t *testing.T) {
				if !slices.Contains(names, flow) {
					t.Fatalf("tools/list = %v, want %s kept as a preview", names, flow)
				}
				preview := previewOf(t, callText(t, server, flow, map[string]any{}))
				if preview.Tool != flow || preview.Hint == "" {
					t.Errorf("preview of %s = %+v, want it to name the flow and say how to turn safe mode off", flow, preview)
				}
			})
		}
		readReachesGitLab(t, server)
	})
}

// TestApply_ExcludeTools_MatchesAWholeNameAndNotAPrefix verifies the word the
// first step's contract rests on: an entry is matched against a registered
// name in full, so it removes that one tool and no relative of it.
//
// It matters because one configuration is routinely reused across surfaces and
// tiers, and the names of the three surfaces nest inside one another: an
// operator who excludes the meta group `gitlab_issue` and points the same file
// at an individual deployment would, under any prefix or substring rule, lose
// every `gitlab_issue_*` tool the surface registers instead of nothing. A
// silent over-removal is the worse half of that, because a model reads
// tools/list to decide what is possible and cannot tell a withdrawn tool from
// one that never existed.
func TestApply_ExcludeTools_MatchesAWholeNameAndNotAPrefix(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	server := newServer()
	for _, name := range []string{"gitlab_issue", "gitlab_issue_list", "gitlab_issue_note_create"} {
		addTool(server, name, false)
	}

	Apply(t.Context(), server, &config.ServerConfig{ExcludeTools: []string{"gitlab_issue"}}, config.ToolSurfaceIndividual, nil)

	if got := listNames(t, server); !slices.Equal(got, []string{"gitlab_issue_list", "gitlab_issue_note_create"}) {
		t.Errorf("tools/list = %v, want only the exactly named tool removed and every longer name kept", got)
	}
	if log := logged.String(); !strings.Contains(log, `"excluded_registered_tools":1`) {
		t.Errorf("log = %q, want exactly one removal counted", log)
	}
}

// TestApply_ExcludeTools_MatchingNothingLeavesEveryToolRegistered verifies the
// case the startup line was renamed for: a list whose every entry was already
// applied to the catalog before registration matches no registered name, and
// the honest outcome is that the pass removes nothing and says zero.
//
// It matters because removing nothing is expressed here as handing the SDK an
// empty list, which is a claim about [mcp.Server.RemoveTools] rather than about
// this code: an empty removal has to be a no-op. Nothing else in this package
// asserts that, so the day a dependency bump made zero names mean every name,
// the deployments this branch exists for — the ones whose exclusions the
// catalog already handled — would lose their whole tool surface at startup
// while the log still read zero.
func TestApply_ExcludeTools_MatchingNothingLeavesEveryToolRegistered(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	server := newServer()
	registered := []string{"gitlab_group", "gitlab_issue", "gitlab_project"}
	for _, name := range registered {
		addTool(server, name, false)
	}

	Apply(t.Context(), server, &config.ServerConfig{ExcludeTools: []string{"gitlab_issue_delete"}}, config.ToolSurfaceMeta, nil)

	if got := listNames(t, server); !slices.Equal(got, registered) {
		t.Errorf("tools/list = %v, want every tool kept when no registered name matched", got)
	}
	if log := logged.String(); !strings.Contains(log, `"excluded_registered_tools":0`) {
		t.Errorf("log = %q, want the count to report the nothing this pass removed", log)
	}
}

// TestApply_ExcludeTools_AppliesUnderReadOnlyAndSafeMode verifies the order the
// three steps run in, which each of the tests above exercises one step at a
// time and none of them pins: the exclusions are applied first, so a name the
// operator removed is gone whichever narrowing mode is also on.
//
// Both halves are ways the pass could be reordered and stay green everywhere
// else. Read-only mode returns as soon as it has removed the writes, so an
// exclusion step moved below it would never run at all and an excluded *read*
// would be served. Safe mode does not remove anything, so an exclusion step
// skipped there would leave the excluded tool listed and answering a preview —
// which reads as a tool that exists and is merely held back, when the operator
// asked for it not to exist.
func TestApply_ExcludeTools_AppliesUnderReadOnlyAndSafeMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.ServerConfig
		// excluded is registered and named in cfg.ExcludeTools; readOnly says
		// how it is annotated, so each mode is given the tool its own step
		// would otherwise have kept.
		excluded string
		readOnly bool
		want     []string
	}{
		{
			name:     "read-only keeps no excluded read",
			cfg:      &config.ServerConfig{ReadOnly: true, ExcludeTools: []string{"gitlab_excluded_read"}},
			excluded: "gitlab_excluded_read",
			readOnly: true,
			want:     []string{"gitlab_read"},
		},
		{
			name:     "safe mode previews no excluded write",
			cfg:      &config.ServerConfig{SafeMode: true, ExcludeTools: []string{"gitlab_excluded_write"}},
			excluded: "gitlab_excluded_write",
			readOnly: false,
			want:     []string{"gitlab_read", "gitlab_write"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newServer()
			addTool(server, "gitlab_read", true)
			addTool(server, "gitlab_write", false)
			excludedCalls := addTool(server, tc.excluded, tc.readOnly)

			Apply(t.Context(), server, tc.cfg, config.ToolSurfaceIndividual, nil)

			if got := listNames(t, server); !slices.Equal(got, tc.want) {
				t.Errorf("tools/list = %v, want %v: the excluded tool is removed before this mode runs", got, tc.want)
			}
			if excludedCalls.Load() != 0 {
				t.Errorf("the excluded tool's handler ran %d time(s), want never", excludedCalls.Load())
			}
		})
	}
}

// TestApply_ExcludeTools_PatternsSurviveAsGiven verifies the exclusion list
// the pass reports is the operator's own list, unsorted and unfiltered, so
// the startup line can be read back against the configuration that produced
// it.
func TestApply_ExcludeTools_PatternsSurviveAsGiven(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	server := newServer()
	addTool(server, "gitlab_b", false)

	Apply(t.Context(), server, &config.ServerConfig{ExcludeTools: []string{"gitlab_b", "gitlab_a"}}, config.ToolSurfaceIndividual, nil)

	var line struct {
		Patterns []string `json:"patterns"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(logged.String())), &line); err != nil {
		t.Fatalf("log line %q is not one JSON record: %v", logged.String(), err)
	}
	if !slices.Equal(line.Patterns, []string{"gitlab_b", "gitlab_a"}) {
		t.Errorf("patterns = %v, want the operator's list as given", line.Patterns)
	}
}
