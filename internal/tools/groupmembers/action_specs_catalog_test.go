// action_specs_catalog_test.go holds the canonical action IDs this package
// publishes against the catalog the server builds.
//
// It is an external test package because the oracle is the catalog, and the
// catalog is assembled by internal/tools out of this package among 178 others:
// an internal test importing it would close an import cycle. The package's own
// unexported constants reach here through export_test.go, and the qualified
// file name is what Go's one-package-per-file-name rule leaves once
// action_specs_test.go is taken by the internal tests.

package groupmembers_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmembers"
)

// catalogActionIDs returns the canonical IDs of every action the catalog
// builds at the Ultimate tier, which is the set gitlab_execute_action accepts
// and gitlab_find_action publishes.
//
// Ultimate because the billable-member actions are Premium: at a lower tier
// they are filtered out of the catalog, and the test would then be asserting
// about the tier rather than about the IDs. The client is self-managed, which
// is what an httptest URL makes it, and that is the wider of the two builds
// for everything named here; only Orbit is contributed on GitLab.com alone,
// and no ID here is one of its six.
func catalogActionIDs(t *testing.T) map[string]struct{} {
	t.Helper()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("build action catalog: %v", err)
	}
	actions := catalog.Actions()
	if len(actions) == 0 {
		t.Fatal("action catalog is empty, so nothing below is being checked")
	}
	ids := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		ids[string(action.ID)] = struct{}{}
	}
	return ids
}

// TestPublishedActionIDs_EveryID_NamesACatalogAction checks that every action
// ID this package hands a model resolves to an action the catalog holds.
//
// What it guards: every RelatedActions entry here used to name the bare spec
// name, "group_member_edit" rather than "group.group_member_edit". It reads as
// a plausible ID, the package built, every other gate passed, and a model
// following one was told the action did not exist.
func TestPublishedActionIDs_EveryID_NamesACatalogAction(t *testing.T) {
	ids := catalogActionIDs(t)
	if len(groupmembers.PublishedActionIDs) == 0 {
		t.Fatal("PublishedActionIDs is empty, so this test asserts nothing")
	}
	for _, id := range groupmembers.PublishedActionIDs {
		t.Run(id, func(t *testing.T) {
			if _, found := ids[id]; !found {
				t.Errorf("published action ID %q resolves to no catalog action", id)
			}
		})
	}
}

// TestActionSpecs_RelatedActions_NameCatalogActions checks the same against
// the specs themselves rather than the constant block, so a related entry
// written as a literal is judged too.
func TestActionSpecs_RelatedActions_NameCatalogActions(t *testing.T) {
	ids := catalogActionIDs(t)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	specs := groupmembers.ActionSpecs(client)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs returned nothing, so this test asserts nothing")
	}
	for _, spec := range specs {
		for _, related := range spec.RelatedActions {
			t.Run(spec.Name+"/"+related, func(t *testing.T) {
				if _, found := ids[related]; !found {
					t.Errorf("action %q lists related action %q, which resolves to no catalog action", spec.Name, related)
				}
			})
		}
	}
}

// TestSpecNames_EveryName_PublishesUnderTheGroupDomain checks the premise the
// constant block rests on: that a spec registered under a bare name is
// published as the group domain plus that name. If the aggregation ever moved
// this package to another catalog group, every derived ID would be wrong at
// once and this is what would say so.
func TestSpecNames_EveryName_PublishesUnderTheGroupDomain(t *testing.T) {
	ids := catalogActionIDs(t)
	if len(groupmembers.SpecNames) == 0 {
		t.Fatal("SpecNames is empty, so this test asserts nothing")
	}
	for _, name := range groupmembers.SpecNames {
		t.Run(name, func(t *testing.T) {
			id := "group." + name
			if _, found := ids[id]; !found {
				t.Errorf("spec %q is not published as %q; the domain constant in action_specs.go is wrong", name, id)
			}
		})
	}
}
