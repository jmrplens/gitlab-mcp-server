// action_specs_test.go holds every canonical action ID this package publishes
// against the catalog the server really builds.
//
// It is an external test package because internal/tools imports
// internal/tools/dockerfiletemplates, so the catalog is only reachable from
// outside the package under test.
package dockerfiletemplates_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dockerfiletemplates"
)

// ultimateCatalog builds the canonical action catalog at the highest tier, so
// no action a published ID could name is filtered out before the comparison.
func ultimateCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	return catalog
}

// TestDockerfileTemplateActionSpecs_PublishedActionIDs_NameCatalogActions
// asserts that every ID this package invites a model to call next resolves to
// a real catalog action.
//
// What it guards: these two IDs used to carry this package's own name as the
// domain ("dockerfiletemplates.dockerfile_list"). The specs are aggregated
// into the gitlab_template group alongside CI lint, gitignore, license and
// project templates, so the domain is "template" and the two real IDs are
// "template.dockerfile_list" and "template.dockerfile_get". A model following
// the old spelling was answered "unknown action".
//
// Why it asserts against the catalog rather than the literals: repeating the
// corrected string beside the constant would pass no matter what the catalog
// holds. Catalog.Action resolves canonical IDs only, never aliases, so an ID
// that is merely some action's alias fails here too.
func TestDockerfileTemplateActionSpecs_PublishedActionIDs_NameCatalogActions(t *testing.T) {
	catalog := ultimateCatalog(t)
	for _, id := range dockerfiletemplates.PublishedActionIDs {
		t.Run(id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action", id)
			}
		})
	}
}

// TestDockerfileTemplateActionSpecs_RelatedActions_NameCatalogActions asserts
// the same for the RelatedActions actually attached to each spec, which is
// what reaches the served surface. It covers IDs written as literals at the
// call site, such as the "repository.file_create" sibling a model reaches for
// after fetching a template, as well as the shared constants.
func TestDockerfileTemplateActionSpecs_RelatedActions_NameCatalogActions(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := dockerfiletemplates.ActionSpecs(client)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() returned no specs")
	}
	for _, spec := range specs {
		for _, related := range spec.RelatedActions {
			t.Run(spec.Name+"/"+related, func(t *testing.T) {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("%s: RelatedActions names %q, which resolves to no catalog action", spec.Name, related)
				}
			})
		}
	}
}
