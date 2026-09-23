//go:build e2e

// mcp_manifest_test.go covers the tool manifest, the gitlab://tools index and
// its per-entry detail resource gitlab://tools/{id}.
//
// The index says what the surface exposes; the detail says how to call one
// entry: the tool and action a dispatcher routes it through, and the
// parameters a caller must supply. The resources sweep does not read either,
// because the {id} variable is a tool-surface id the World has no binding for
// and because the pair is the one resource whose content the shape decides:
// it lists what the session's tool surface registered after the read-only and
// safe passes, and carries a subscriptions section only on the full
// capability surface. The coverage command therefore counts the pair per
// surface x mode x capability surface, and TestManifest_EveryShape_ReadsTheIndexAndAnEntry
// is what reads it on each of those shapes. The rest of the file holds the
// meta surface's detail to the call it describes, which is ported from the
// old suite rather than dropped.

package common

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// toolsDetailTemplate is the resource template the detail resource is served
// under.
const toolsDetailTemplate = "gitlab://tools/{id}"

// manifestShape is one configuration the manifest is read on.
type manifestShape struct {
	surface      harness.Surface
	mode         harness.Mode
	capabilities harness.CapabilitySurface
}

// manifestShapes lists every shape the manifest is held to: the three
// surfaces in each protective mode on the full capability surface, and the
// three in the default mode on the minimal one, which keeps the manifest and
// drops every other resource.
func manifestShapes() []manifestShape {
	var shapes []manifestShape
	for _, surface := range harness.AllSurfaces() {
		for _, mode := range []harness.Mode{harness.ModeDefault, harness.ModeReadOnly, harness.ModeSafe} {
			shapes = append(shapes, manifestShape{surface: surface, mode: mode, capabilities: harness.CapabilitiesFull})
		}
		shapes = append(shapes, manifestShape{surface: surface, mode: harness.ModeDefault, capabilities: harness.CapabilitiesMinimal})
	}
	return shapes
}

// surfaceEntryKinds is the entry kind each surface projects its own actions
// as, spelled here because the resources package keeps the constants
// unexported and a test that took its expectation from the code it tests
// would agree with any spelling.
var surfaceEntryKinds = map[harness.Surface]string{
	harness.SurfaceDynamic:    "dynamic_action",
	harness.SurfaceMeta:       "meta_action",
	harness.SurfaceIndividual: "individual_tool",
}

// TestManifest_EveryShape_ReadsTheIndexAndAnEntry reads the manifest on every
// shape it varies along and follows one entry to its detail.
//
// Each shape reads the index, which must name the session's own surface, list
// entries, and carry the subscriptions section exactly when the capability
// surface is full; then the detail of the first entry of the surface's own
// kind, through the URI the index advertises for it. The detail must be that
// entry, spelled the way the surface spells a call: the canonical action id
// routed through gitlab_execute_action on dynamic, a dispatcher tool and an
// action joined by a dot on meta, and the tool's own name on individual.
//
// It replaces no old test by name: the old suite read the manifest on the
// meta surface alone, and the tests below still carry that port.
func TestManifest_EveryShape_ReadsTheIndexAndAnEntry(t *testing.T) {
	for _, shape := range manifestShapes() {
		t.Run(string(shape.surface)+"/"+shape.mode.String()+"/"+shape.capabilities.String(), func(t *testing.T) {
			t.Parallel()
			e := harness.New(t)
			s := e.Session(harness.ServerConfig{Surface: shape.surface, Mode: shape.mode, Capabilities: shape.capabilities})

			manifest := readToolsManifest(e, s)
			if manifest.Surface != string(shape.surface) {
				e.T.Errorf("the manifest describes the %q surface, want %q", manifest.Surface, shape.surface)
			}
			if wantSection := shape.capabilities == harness.CapabilitiesFull; (manifest.Subscriptions != nil) != wantSection {
				e.T.Errorf("the manifest carries a subscriptions section: %t, want %t on the %s capability surface",
					manifest.Subscriptions != nil, wantSection, shape.capabilities)
			}
			entry, found := firstEntryOfKind(manifest, surfaceEntryKinds[shape.surface])
			if !found {
				e.T.Fatalf("the manifest lists %d entries and none of kind %s", len(manifest.Entries), surfaceEntryKinds[shape.surface])
			}
			detail := readDetailAt(e, s, entry.DetailURI)
			if detail.ID != entry.ID || detail.Kind != entry.Kind {
				e.T.Errorf("the detail at %s is %s (%s), want the entry it was listed for, %s (%s)",
					entry.DetailURI, detail.ID, detail.Kind, entry.ID, entry.Kind)
			}
			checkEntrySpelling(e, s, shape.surface, entry, detail)
		})
	}
}

// firstEntryOfKind returns the first entry of one kind, in the order the
// manifest lists them.
func firstEntryOfKind(manifest resources.ToolSurfaceManifest, kind string) (resources.ToolSurfaceEntry, bool) {
	for _, entry := range manifest.Entries {
		if entry.Kind == kind {
			return entry, true
		}
	}
	return resources.ToolSurfaceEntry{}, false
}

// readDetailAt reads a detail through the URI the manifest advertised for it,
// rather than through one this test builds, since the advertised URI is what
// a client follows.
func readDetailAt(e *harness.Env, s *harness.Session, uri string) resources.ToolSurfaceDetail {
	e.T.Helper()

	result := s.ReadResource(uri)
	if len(result.Contents) == 0 {
		e.T.Fatalf("%s answered no contents", uri)
	}
	var detail resources.ToolSurfaceDetail
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &detail); err != nil {
		e.T.Fatalf("decoding the detail at %s: %v", uri, err)
	}
	return detail
}

// checkEntrySpelling holds one entry and its detail to the way its surface
// spells a call, and the tool it names to one the session registers.
func checkEntrySpelling(e *harness.Env, s *harness.Session, surface harness.Surface, entry resources.ToolSurfaceEntry, detail resources.ToolSurfaceDetail) {
	e.T.Helper()

	switch surface {
	case harness.SurfaceDynamic:
		if strings.HasPrefix(entry.ID, "gitlab_") || !strings.Contains(entry.ID, ".") {
			e.T.Errorf("the dynamic entry %q is not a canonical action id", entry.ID)
		}
		if detail.Call.Tool != "gitlab_execute_action" || detail.Call.Action != entry.ID {
			e.T.Errorf("the dynamic detail calls %s/%s, want gitlab_execute_action/%s", detail.Call.Tool, detail.Call.Action, entry.ID)
		}
	case harness.SurfaceMeta:
		if entry.ID != entry.Tool+"."+entry.Action {
			e.T.Errorf("the meta entry %q is not its tool %q and action %q joined", entry.ID, entry.Tool, entry.Action)
		}
		if detail.Call.Tool != entry.Tool || detail.Call.Action != entry.Action {
			e.T.Errorf("the meta detail calls %s/%s, want %s/%s", detail.Call.Tool, detail.Call.Action, entry.Tool, entry.Action)
		}
	default:
		if entry.ID != entry.Tool || detail.Call.Tool != entry.ID || detail.Call.Action != "" {
			e.T.Errorf("the individual entry %q calls %s/%s, want the tool by its own name and no action",
				entry.ID, detail.Call.Tool, detail.Call.Action)
		}
	}
	if !slices.Contains(s.Tools(), detail.Call.Tool) {
		e.T.Errorf("the detail calls %s, which the session does not register", detail.Call.Tool)
	}
}

// TestManifest_DetailTemplateIsAdvertised checks that the per-action detail
// template is listed among the session's resource templates.
//
// Replaces: TestToolManifestResource_ListsTemplate
func TestManifest_DetailTemplateIsAdvertised(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceMeta)

	if !slices.Contains(s.ResourceTemplates(), toolsDetailTemplate) {
		e.T.Errorf("the detail template %s is not among the served resource templates: %v", toolsDetailTemplate, s.ResourceTemplates())
	}
}

// TestManifest_DetailDescribesTheCall reads the detail for the merge request
// create entry and holds it to the call a caller must make: the dispatcher
// tool and action it routes through, and the parameters an MR needs.
//
// Replaces: TestToolManifestResource_ReadMergeRequestCreate
func TestManifest_DetailDescribesTheCall(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceMeta)

	manifest := readToolsManifest(e, s)
	entry, found := findManifestEntry(manifest, "gitlab_merge_request", "create")
	if !found {
		e.T.Fatalf("the meta manifest has no merge request create entry among %d entries", len(manifest.Entries))
	}

	detail, body := readToolDetail(e, s, entry.ID)
	if detail.Call.Tool != "gitlab_merge_request" || detail.Call.Action != "create" {
		e.T.Errorf("the detail call is %s/%s, want gitlab_merge_request/create", detail.Call.Tool, detail.Call.Action)
	}
	// The parameters a caller needs must be findable in the detail document,
	// whether the schema mode carries them as required params or inside the
	// per-action input schema; the old suite asserted their presence in the
	// body, and that stays the robust check across schema modes.
	for _, want := range []string{"project_id", "source_branch", "target_branch", "title"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("the merge request create detail does not mention %q", want)
			}
		})
	}
}

// TestManifest_UnknownEntryIsNotFound checks that the detail resource refuses
// an id no entry has, rather than answering an empty document.
//
// Replaces: TestToolManifestResource_NotFound
func TestManifest_UnknownEntryIsNotFound(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceMeta)

	_, err := s.TryReadResource("gitlab://tools/gitlab_merge_request.nonexistent_action")
	if err == nil {
		e.T.Fatal("reading an unknown detail id answered no error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		e.T.Errorf("the refusal does not say not found: %v", err)
	}
}

// TestManifest_IndexEnumeratesTheDispatchers checks that the index lists the
// canonical meta actions a client discovers the surface through.
//
// Replaces: TestToolManifestResource_IndexEnumeratesMetaTools
func TestManifest_IndexEnumeratesTheDispatchers(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceMeta)
	manifest := readToolsManifest(e, s)

	cases := []struct {
		tool   string
		action string
	}{
		{"gitlab_project", "get"},
		{"gitlab_merge_request", "create"},
		{"gitlab_issue", "get"},
	}
	for _, want := range cases {
		t.Run(want.tool+"."+want.action, func(t *testing.T) {
			if _, found := findManifestEntry(manifest, want.tool, want.action); !found {
				t.Errorf("the meta manifest does not list %s.%s", want.tool, want.action)
			}
		})
	}
}

// findManifestEntry returns the entry a dispatcher tool routes an action
// through.
func findManifestEntry(manifest resources.ToolSurfaceManifest, tool, action string) (resources.ToolSurfaceEntry, bool) {
	for _, entry := range manifest.Entries {
		if entry.Tool == tool && entry.Action == action {
			return entry, true
		}
	}
	return resources.ToolSurfaceEntry{}, false
}

// readToolDetail reads one gitlab://tools/{id} detail document, returning the
// decoded detail and its raw body.
func readToolDetail(e *harness.Env, s *harness.Session, id string) (resources.ToolSurfaceDetail, string) {
	e.T.Helper()

	result := s.ReadResource("gitlab://tools/" + id)
	if len(result.Contents) == 0 {
		e.T.Fatalf("gitlab://tools/%s answered no contents", id)
	}
	body := result.Contents[0].Text
	var detail resources.ToolSurfaceDetail
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		e.T.Fatalf("decoding the detail for %s: %v", id, err)
	}
	return detail, body
}
