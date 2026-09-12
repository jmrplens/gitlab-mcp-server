//go:build e2e

// mcp_tiers_test.go covers what the tier decides: which actions exist on the
// runtime under test, on every surface, as the server itself publishes them.
//
// The harness checks every session it starts against the server's own
// assemblers, so a surface serving a tool the catalog at this tier does not
// have fails the session before any test runs. What that check cannot see is
// the server's own account of its surface, which is what a client reads to
// decide what is possible: tools/list, and the gitlab://tools manifest that
// names the actions behind the dispatchers. This test reads both and holds
// them to the sessions and to the tier the probe found.

package common

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// toolsManifestURI is the resource that describes the active tool surface,
// served on every surface and on both capability surfaces.
const toolsManifestURI = "gitlab://tools"

// The entry kinds the manifest uses for the two dispatcher surfaces, spelled
// as the resource spells them.
const (
	manifestKindDynamicAction = "dynamic_action"
	manifestKindMetaAction    = "meta_action"
)

// TestTier_ServedSetMatchesCatalog checks, on every surface, that what the
// server publishes about itself agrees with the session the harness started
// and with the tier of the instance.
//
// On the dynamic surface the manifest names every action gitlab_execute_action
// can run, so it is held to the session's served set exactly and every entry
// is held at or below the runtime's tier; a licensed runtime must list an
// action above Free, or the license bought nothing. On the other two the
// manifest describes registered tools, and those are held to tools/list.
//
// Replaces: TestEnterpriseSecurityTools_NotRegisteredOnCE
func TestTier_ServedSetMatchesCatalog(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		manifest := readToolsManifest(e, s)
		tools := s.Tools()

		if manifest.Surface != string(surface) {
			e.T.Errorf("the manifest says the surface is %q, and the session was started as %s", manifest.Surface, surface)
		}
		if manifest.VisibleToolCount != len(tools) {
			e.T.Errorf("the manifest counts %d visible tools, and tools/list served %d", manifest.VisibleToolCount, len(tools))
		}
		for _, visible := range manifest.VisibleTools {
			if !slices.Contains(tools, visible.Name) {
				e.T.Errorf("the manifest lists %s as visible, and tools/list did not serve it", visible.Name)
			}
		}
		if !s.Serves(actionServerStatus) {
			e.T.Errorf("the %s session does not serve %s, which every runtime has", surface, actionServerStatus)
		}

		switch surface {
		case harness.SurfaceDynamic:
			assertDynamicEntriesAreTheServedSet(e, s, manifest)
		case harness.SurfaceMeta:
			assertEntryToolsAreListed(e, manifest, manifestKindMetaAction, tools)
		case harness.SurfaceIndividual:
			assertEntryToolsAreListed(e, manifest, "", tools)
		}
	})
}

// assertDynamicEntriesAreTheServedSet holds the dynamic manifest's action
// entries to the session's served set, both ways, and each entry to the tier.
func assertDynamicEntriesAreTheServedSet(e *harness.Env, s *harness.Session, manifest resources.ToolSurfaceManifest) {
	e.T.Helper()
	rt := e.Runtime()

	listed := make([]harness.ActionID, 0, len(manifest.Entries))
	aboveFree := 0
	for _, entry := range manifest.Entries {
		if entry.Kind != manifestKindDynamicAction {
			continue
		}
		id := harness.ActionID(entry.ID)
		listed = append(listed, id)
		if !s.Serves(id) {
			e.T.Errorf("the manifest lists %s, and the assemblers say the session does not serve it", id)
		}
		tier, known := e.ActionTier(id)
		if !known {
			// The standalone actions, discovery and the interactive flows,
			// live outside the base catalog and carry no tier; the served
			// set is the check on those.
			continue
		}
		if !rt.Tier.AtLeast(tier) {
			e.T.Errorf("the manifest lists %s, which needs %s, on a %s runtime", id, tier, rt.Tier)
		}
		if tier.IsEnterprise() {
			aboveFree++
		}
	}
	slices.Sort(listed)

	served := s.Actions()
	for _, id := range served {
		if _, found := slices.BinarySearch(listed, id); !found {
			e.T.Errorf("the assemblers say the session serves %s, and the manifest does not list it", id)
		}
	}
	if rt.Tier.IsEnterprise() && aboveFree == 0 {
		e.T.Errorf("the runtime is %s and the manifest lists no action above Free", rt.Tier)
	}
	if !rt.Tier.IsEnterprise() && aboveFree > 0 {
		e.T.Errorf("the runtime is Free and the manifest lists %d actions above it", aboveFree)
	}
	e.T.Logf("%d actions listed, %d above Free, runtime %s", len(listed), aboveFree, rt.Tier)
}

// assertEntryToolsAreListed holds every manifest entry of one kind, or of
// every kind when none is named, to the tools tools/list served.
func assertEntryToolsAreListed(e *harness.Env, manifest resources.ToolSurfaceManifest, kind string, tools []string) {
	e.T.Helper()

	entries := 0
	for _, entry := range manifest.Entries {
		if kind != "" && entry.Kind != kind {
			continue
		}
		entries++
		if !slices.Contains(tools, entry.Tool) {
			e.T.Errorf("manifest entry %s names tool %s, which tools/list did not serve", entry.ID, entry.Tool)
		}
	}
	if entries == 0 {
		e.T.Errorf("the manifest lists no entry of kind %q, so nothing above was checked", kind)
	}
}

// readToolsManifest reads gitlab://tools through the session and decodes it.
func readToolsManifest(e *harness.Env, s *harness.Session) resources.ToolSurfaceManifest {
	e.T.Helper()

	result := s.ReadResource(toolsManifestURI)
	if len(result.Contents) == 0 {
		e.T.Fatalf("%s answered with no contents", toolsManifestURI)
	}
	var manifest resources.ToolSurfaceManifest
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &manifest); err != nil {
		e.T.Fatalf("decoding %s: %v", toolsManifestURI, err)
	}
	return manifest
}
