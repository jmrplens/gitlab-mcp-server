package tools

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// TestCatalogGroupIcons_EveryGroupTheCatalogBuilds_HasAnEntry holds the icon
// map to the groups the catalog actually serves, in both directions.
//
// A group the map does not name is not refused: catalogGroupIcons hands it
// IconServer, which is also the icon of gitlab_execute_action on the dynamic
// surface, so the meta-tool and every individual tool of that group are drawn
// as the server rather than as their domain and nothing reports it. That is
// how gitlab_achievement shipped. The catalog is built at every tier on a
// self-managed instance and on GitLab.com, because the set of groups differs
// between them (gitlab_orbit exists only on GitLab.com at Premium and above,
// and the paid groups only above Free), and each group is held to carry the
// map's entry rather than merely to have one, so a group whose icon came from
// anywhere else fails too.
//
// The reverse direction keeps the map from carrying a name no build serves:
// an entry left behind by a renamed group draws nothing, and the renamed group
// itself would be back on the fallback.
//
// All six catalogs are built, and the set of groups they serve collected,
// before any subtest runs, so that neither direction depends on which sibling
// subtests ran: an entry subtest selected on its own with -run reads the whole
// served set rather than an empty one.
func TestCatalogGroupIcons_EveryGroupTheCatalogBuilds_HasAnEntry(t *testing.T) {
	builds := catalogBuildsAtEveryTier(t)
	served := make(map[string]bool)
	for _, build := range builds {
		for _, group := range build.groups {
			served[group.ToolName] = true
		}
	}

	for _, build := range builds {
		t.Run(build.name, func(t *testing.T) {
			if len(build.groups) == 0 {
				t.Fatal("the catalog built no group, so there is nothing to hold the icon map to")
			}
			for _, group := range build.groups {
				t.Run(group.ToolName, func(t *testing.T) {
					want, ok := catalogGroupIconsByToolName[group.ToolName]
					if !ok {
						t.Fatalf("%s has no entry in catalogGroupIconsByToolName, so it and its individual tools are served IconServer, the icon of gitlab_execute_action", group.ToolName)
					}
					assertSameIcons(t, group.Icons, want)
				})
			}
		})
	}

	for toolName := range catalogGroupIconsByToolName {
		t.Run("entry "+toolName, func(t *testing.T) {
			if !served[toolName] {
				t.Errorf("catalogGroupIconsByToolName names %s, which no tier of either instance builds", toolName)
			}
		})
	}
}

// catalogBuild is one catalog the server can build, named by the instance and
// tier it was built for, with the groups it serves.
type catalogBuild struct {
	name   string
	groups []actioncatalog.Group
}

// catalogBuildsAtEveryTier builds the catalog at Free, Premium and Ultimate on
// a self-managed instance and on GitLab.com, the six builds whose sets of
// groups differ, and fails the calling test when one cannot be built, since
// nothing about the groups can be judged without it.
func catalogBuildsAtEveryTier(t *testing.T) []catalogBuild {
	t.Helper()
	instances := []struct {
		name   string
		client *gitlabclient.Client
	}{
		{name: "self-managed"},
		{name: "gitlab.com", client: newGitLabDotComClient(t)},
	}
	builds := make([]catalogBuild, 0, len(instances)*3)
	for _, instance := range instances {
		for _, tier := range []edition.Tier{edition.Free, edition.Premium, edition.Ultimate} {
			catalog := mustBuildActionCatalog(t, instance.client, ActionCatalogOptions{Tier: tier})
			builds = append(builds, catalogBuild{name: instance.name + "_" + tier.String(), groups: catalog.Groups()})
		}
	}
	return builds
}

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
		"gitlab_blank_widget": "   ",
	}

	cases := []struct {
		name     string
		toolName string
		want     string
	}{
		{name: "a row that is only the runtime preamble", toolName: "gitlab_widget", want: "GitLab widget actions."},
		{name: "a row with no preamble to strip", toolName: "gitlab_other_widget", want: "GitLab other widget actions."},
		// The one row that makes the emptiness check decide: a description of
		// nothing but spaces is too short for the prefix stripper to touch, so
		// it comes back unchanged and only the trimmed test tells it from prose.
		{name: "a row that is blank rather than absent", toolName: "gitlab_blank_widget", want: "GitLab blank widget actions."},
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
