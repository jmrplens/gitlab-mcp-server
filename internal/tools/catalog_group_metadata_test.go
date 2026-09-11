package tools

import (
	"testing"
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
		"gitlab_widget": "Use {\"action\":\"archive\",\"params\":{...}}. The only top-level keys are action and params.\nAction params schema: gitlab://tools/gitlab_widget.<action>.\n\nDetailed widget actions.",
	}

	if got := catalogGroupDescription("gitlab_widget"); got != "Detailed widget actions." {
		t.Fatalf("catalogGroupDescription() = %q, want stored base description", got)
	}
}

// TestCatalogGroupDescription_FallsBackWhenNothingIsLeftToUse covers the three
// ways the curated snapshot answers nothing usable, all of which must end at the
// derived sentence rather than at a description that is empty or is the runtime
// preamble a client has already been told.
//
// The preamble says how to call a meta-tool and where its per-action schema
// lives. Serving it as the group's own description would spend a client's
// context repeating the instructions the tool's schema already carries, and
// serving the empty string would leave the domain unexplained, so a snapshot row
// that is only the preamble, one that carries no preamble to remove, and a name
// with no row at all are all worth the derived sentence instead.
func TestCatalogGroupDescription_FallsBackWhenNothingIsLeftToUse(t *testing.T) {
	original := catalogMetaToolDescriptions
	t.Cleanup(func() { catalogMetaToolDescriptions = original })

	const preamble = "Use {\"action\":\"archive\",\"params\":{...}}. The only top-level keys are action and params.\nAction params schema: gitlab://tools/gitlab_widget.<action>."
	catalogMetaToolDescriptions = map[string]string{
		"gitlab_widget":       preamble,
		"gitlab_other_widget": "Widget actions with no runtime preamble in front of them.",
	}

	cases := []struct {
		name     string
		toolName string
		want     string
	}{
		{name: "a row that is only the runtime preamble", toolName: "gitlab_widget", want: "GitLab widget actions."},
		{name: "a row with no preamble to strip", toolName: "gitlab_other_widget", want: "GitLab other widget actions."},
		{name: "a tool name the snapshot does not carry", toolName: "gitlab_absent_widget", want: "GitLab absent widget actions."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := catalogGroupDescription(tc.toolName); got != tc.want {
				t.Errorf("catalogGroupDescription(%q) = %q, want the derived sentence %q", tc.toolName, got, tc.want)
			}
		})
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
