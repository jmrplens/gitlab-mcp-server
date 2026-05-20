package tools

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestLoadCatalogMetaToolDescriptions_SkipsIncompleteSnapshots verifies meta
// tool description loading ignores snapshot rows without names or descriptions.
//
// The test temporarily replaces the embedded snapshot JSON and expects only the
// complete gitlab_project entry to be returned, preserving robust catalog startup
// when generated snapshot rows are incomplete.
func TestLoadCatalogMetaToolDescriptions_SkipsIncompleteSnapshots(t *testing.T) {
	original := metaToolSnapshotJSON
	t.Cleanup(func() { metaToolSnapshotJSON = original })

	metaToolSnapshotJSON = []byte(`[
		{"name":"gitlab_project","description":"Project tools"},
		{"name":"","description":"missing name"},
		{"name":"gitlab_issue","description":""}
	]`)

	descriptions := loadCatalogMetaToolDescriptions()
	if len(descriptions) != 1 {
		t.Fatalf("descriptions length = %d, want 1", len(descriptions))
	}
	if descriptions["gitlab_project"] != "Project tools" {
		t.Fatalf("gitlab_project description = %q", descriptions["gitlab_project"])
	}
}

// TestLoadCatalogIndividualToolDescriptions_SkipsIncompleteSnapshots verifies
// individual tool description loading ignores incomplete snapshot rows.
//
// The test replaces the embedded individual snapshot with one complete entry and
// two incomplete entries. The loader should return only gitlab_get_project with
// its stored description.
func TestLoadCatalogIndividualToolDescriptions_SkipsIncompleteSnapshots(t *testing.T) {
	original := individualToolSnapshotJSON
	t.Cleanup(func() { individualToolSnapshotJSON = original })

	individualToolSnapshotJSON = []byte(`[
		{"name":"gitlab_get_project","description":"Get project"},
		{"name":"","description":"missing name"},
		{"name":"gitlab_list_projects","description":""}
	]`)

	descriptions := loadCatalogIndividualToolDescriptions()
	if len(descriptions) != 1 {
		t.Fatalf("descriptions length = %d, want 1", len(descriptions))
	}
	if descriptions["gitlab_get_project"] != "Get project" {
		t.Fatalf("gitlab_get_project description = %q", descriptions["gitlab_get_project"])
	}
}

// TestCatalogGroupDescription_StripsStoredMetaPrefix verifies generated meta
// descriptions do not duplicate the runtime action-envelope preamble.
//
// The stored description includes the usage and schema prefix already added at
// runtime. The expected result keeps only the base domain description so tool
// help remains concise and avoids repeated instructions.
func TestCatalogGroupDescription_StripsStoredMetaPrefix(t *testing.T) {
	original := catalogMetaToolDescriptions
	t.Cleanup(func() { catalogMetaToolDescriptions = original })

	catalogMetaToolDescriptions = map[string]string{
		"gitlab_widget": "Use {\"action\":\"archive\",\"params\":{...}}; only top-level keys are action and params.\nAction params schema: gitlab://tools/gitlab_widget.<action>.\n\nDetailed widget actions.",
	}
	routes := toolutil.ActionMap{"create": toolutil.Route(nil), "archive": toolutil.Route(nil)}

	if got := catalogGroupDescription("gitlab_widget", routes); got != "Detailed widget actions." {
		t.Fatalf("catalogGroupDescription() = %q, want stored base description", got)
	}
}

// TestLoadCatalogToolDescriptions_PanicOnInvalidJSON verifies embedded catalog
// description snapshots fail fast when their JSON is invalid.
//
// The meta and individual subtests temporarily corrupt their snapshot bytes and
// expect the loader to panic, making generated-data corruption visible during
// tests instead of silently omitting descriptions.
func TestLoadCatalogToolDescriptions_PanicOnInvalidJSON(t *testing.T) {
	t.Run("meta", func(t *testing.T) {
		original := metaToolSnapshotJSON
		t.Cleanup(func() { metaToolSnapshotJSON = original })
		metaToolSnapshotJSON = []byte(`{`)
		assertPanics(t, func() { _ = loadCatalogMetaToolDescriptions() })
	})

	t.Run("individual", func(t *testing.T) {
		original := individualToolSnapshotJSON
		t.Cleanup(func() { individualToolSnapshotJSON = original })
		individualToolSnapshotJSON = []byte(`{`)
		assertPanics(t, func() { _ = loadCatalogIndividualToolDescriptions() })
	})
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
