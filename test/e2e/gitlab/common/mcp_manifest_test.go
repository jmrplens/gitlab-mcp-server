//go:build e2e

// mcp_manifest_test.go covers the per-action detail resource gitlab://tools/{id},
// the companion to the gitlab://tools index the tier test reads.
//
// The index says what the surface exposes; the detail says how to call one
// entry: the tool and action a dispatcher routes it through, and the
// parameters a caller must supply. The reads sweep cannot reach it, because
// its {id} variable is a tool-surface id the World has no binding for, so the
// old suite's coverage of it is ported here rather than dropped. It runs on
// the meta surface, whose entries name a dispatcher tool and an action.

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
