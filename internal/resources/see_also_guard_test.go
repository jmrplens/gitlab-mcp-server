// see_also_guard_test.go is the drift guard for the "See also:" clauses in
// the tool manifest: every cross-reference an emitted description carries
// must resolve to an entry ID (or visible tool) of the surface that emitted
// it. The clauses are hand-written once, in individual-tool names, and
// projected per surface by rewriteSeeAlso — a reference that resolves on no
// surface is a stale name in an action spec, and this test names it.
package resources

import (
	"maps"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fullSurfaceCatalog builds the Ultimate-tier catalog the way the dynamic
// surface does at startup: the canonical actions plus the standalone
// surface tools (interactive elicitation, project discovery, server
// maintenance) — the live manifest lists those too, so a guard that skips
// them is blind to their descriptions.
func fullSurfaceCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{
		Enterprise: true,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog: %v", err)
	}
	withStandalone, err := dynamictools.AddStandaloneCatalog(catalog, nil, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog: %v", err)
	}
	return withStandalone
}

// TestToolManifest_SeeAlsoReferencesResolve_OnEverySurface builds the full
// Ultimate-tier catalog and, for each tool surface, asserts every "See
// also:" reference in every emitted manifest description names an entry of
// that same surface. 2,072+ references shipped with 36 stale names and no
// alarm before this guard existed.
func TestToolManifest_SeeAlsoReferencesResolve_OnEverySurface(t *testing.T) {
	catalog := fullSurfaceCatalog(t)

	surfaces := []struct {
		name string
		opts ToolSurfaceResourceOptions
	}{
		{
			name: toolSurfaceDynamic,
			opts: ToolSurfaceResourceOptions{
				Surface: toolSurfaceDynamic,
				Tools: []*mcp.Tool{
					{Name: "gitlab_execute_action", Title: "Execute"},
					{Name: "gitlab_find_action", Title: "Find"},
				},
				Catalog: catalog,
			},
		},
		{
			name: toolSurfaceMeta,
			opts: ToolSurfaceResourceOptions{
				Surface:    toolSurfaceMeta,
				Catalog:    catalog,
				MetaRoutes: catalog.ActionMaps(),
			},
		},
		{
			// A restricted meta surface (excluded tools, read-only
			// filtering) emits a subset of routes; a reference rewritten
			// to a hidden action's ID would name an entry that does not
			// exist. Dropping gitlab_project — the most referenced tool —
			// exercises exactly that path.
			name: toolSurfaceMeta + " restricted",
			opts: ToolSurfaceResourceOptions{
				Surface:    toolSurfaceMeta,
				Catalog:    catalog,
				MetaRoutes: withoutRoute(catalog.ActionMaps(), "gitlab_project"),
			},
		},
	}

	for _, surface := range surfaces {
		t.Run(surface.name, func(t *testing.T) {
			snapshot := newToolSurfaceSnapshot(surface.opts)
			valid := make(map[string]bool, len(snapshot.manifest.Entries))
			for _, entry := range snapshot.manifest.Entries {
				valid[entry.ID] = true
			}
			for _, tool := range snapshot.manifest.VisibleTools {
				valid[tool.Name] = true
			}
			assertSeeAlsoResolves(t, snapshot, valid)
		})
	}

	// The individual surface has no snapshot projection to check (its
	// descriptions pass through untouched), but its namespace is where the
	// clauses are written — so every referenced name must be a real
	// individual tool. This is the leg that catches stale hand-written
	// names at their source.
	t.Run(toolSurfaceIndividual, func(t *testing.T) {
		valid := make(map[string]bool)
		for _, action := range catalog.Actions() {
			if action.IndividualTool.Name != "" {
				valid[action.IndividualTool.Name] = true
			}
		}
		for _, action := range catalog.Actions() {
			for _, name := range seeAlsoNames(action.IndividualTool.Description) {
				if !valid[name] {
					t.Errorf("action %s references %q in its See-also clause, but no individual tool has that name — fix the spec", action.ID, name)
				}
			}
		}
	})
}

// withoutRoute returns routes with one tool's action map removed,
// modeling a restricted meta surface.
func withoutRoute(routes map[string]toolutil.ActionMap, toolName string) map[string]toolutil.ActionMap {
	delete(routes, toolName)
	return routes
}

// assertSeeAlsoResolves walks every emitted description and requires each
// See-also reference to be a member of the surface's own namespace.
func assertSeeAlsoResolves(t *testing.T, snapshot toolSurfaceSnapshot, valid map[string]bool) {
	t.Helper()
	for _, entry := range snapshot.manifest.Entries {
		for _, name := range seeAlsoNames(entry.Description) {
			if !valid[name] {
				t.Errorf("entry %s references %q in its See-also clause, which is not an entry of this surface", entry.ID, name)
			}
		}
	}
}

// seeAlsoNames extracts the referenced names from every "See also:"
// clause in a description — some descriptions carry more than one — using
// the same pattern the projection rewrites.
func seeAlsoNames(description string) []string {
	var names []string
	for _, match := range seeAlsoClause.FindAllStringSubmatch(description, -1) {
		names = append(names, strings.Split(match[1], ", ")...)
	}
	return names
}

// TestToolManifest_AliasEntriesDeclareAliasOf verifies the three deliberate
// alias pairs in the catalog are declared as such in the dynamic manifest,
// instead of presenting two entries a client cannot tell apart.
func TestToolManifest_AliasEntriesDeclareAliasOf(t *testing.T) {
	catalog := fullSurfaceCatalog(t)
	snapshot := newToolSurfaceSnapshot(ToolSurfaceResourceOptions{
		Surface: toolSurfaceDynamic,
		Tools:   []*mcp.Tool{{Name: "gitlab_execute_action"}, {Name: "gitlab_find_action"}},
		Catalog: catalog,
	})
	aliasOf := make(map[string]string)
	for _, entry := range snapshot.manifest.Entries {
		if entry.AliasOf != "" {
			aliasOf[entry.ID] = entry.AliasOf
		}
	}
	// Exact-set comparison: a new shared individual name would add an
	// unintended alias pair, and a primary declaring alias_of would show
	// up as an extra key — both must fail here, not slip through.
	want := map[string]string{
		"user.me":                 "user.current",
		"repository.file_history": "repository.commit_list",
		"issue.list_group":        "group.issues",
	}
	if !maps.Equal(aliasOf, want) {
		t.Errorf("alias_of map = %v, want exactly %v", aliasOf, want)
	}
}
