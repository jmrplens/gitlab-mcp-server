// action_specs_test.go holds every canonical action ID this package publishes
// against the catalog the server really builds.
//
// It is an external test package because the catalog is assembled by
// internal/tools, which reaches this package through adminspecs: only a _test
// package may import back across that edge. The rule it enforces is the one
// this package broke with "admin.version" and "admin.health", two IDs that
// were never actions at all. RelatedActions is the whole published surface
// here: markdown.go writes prose and no canonical action ID, so there is no
// hint half to judge.
package dbmigrations_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dbmigrations"
)

// TestActionSpecs_RelatedActionsResolveInTheCatalog verifies every
// RelatedActions entry of the database migration spec names an action the
// catalog holds. IncludeMCP is set because one of them is the server group's
// health check, which the GitLab domains alone do not carry.
func TestActionSpecs_RelatedActionsResolveInTheCatalog(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	for _, spec := range dbmigrations.ActionSpecs(client) {
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
