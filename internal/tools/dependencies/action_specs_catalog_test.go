// action_specs_catalog_test.go holds every canonical action ID this package
// publishes against the catalog the server really builds.
//
// It is an external test package because the catalog is assembled by
// internal/tools, which imports this one: only a _test package may import back
// across that edge. The rule it enforces is the one this package broke with
// "security.vulnerability_list", a plausible-looking pair naming a domain the
// catalog has no group for. The check reads the RelatedActions the specs
// actually carry rather than the constants that produced them, so an entry
// added later is covered without this file being touched. RelatedActions is
// the whole published surface here: markdown.go names individual tool names in
// its footers and no canonical action ID, so there is no hint half to judge.
package dependencies_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dependencies"
)

// TestActionSpecs_RelatedActionsResolveInTheCatalog verifies every
// RelatedActions entry of every dependency spec names an action the catalog
// holds. The catalog is built at the Ultimate tier because the dependency group
// is licensed and would otherwise be absent along with the vulnerability
// actions it points at.
func TestActionSpecs_RelatedActionsResolveInTheCatalog(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	for _, spec := range dependencies.ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if len(spec.RelatedActions) == 0 {
				t.Fatal("spec publishes no related actions; this test would assert nothing")
			}
			for _, related := range spec.RelatedActions {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("RelatedActions %q resolves to no catalog action; a model following it is answered unknown action", related)
				}
			}
		})
	}
}
