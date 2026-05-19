package tools

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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

func TestCatalogGroupDescription_StripsStoredMetaPrefix(t *testing.T) {
	original := catalogMetaToolDescriptions
	t.Cleanup(func() { catalogMetaToolDescriptions = original })

	catalogMetaToolDescriptions = map[string]string{
		"gitlab_widget": "Use {\"action\":\"archive\",\"params\":{...}}; only top-level keys are action and params.\nAction params schema: gitlab://schema/meta/gitlab_widget/<action>.\n\nDetailed widget actions.",
	}
	routes := toolutil.ActionMap{"create": toolutil.Route(nil), "archive": toolutil.Route(nil)}

	if got := catalogGroupDescription("gitlab_widget", routes); got != "Detailed widget actions." {
		t.Fatalf("catalogGroupDescription() = %q, want stored base description", got)
	}
}

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
