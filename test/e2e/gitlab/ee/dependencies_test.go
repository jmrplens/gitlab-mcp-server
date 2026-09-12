//go:build e2e

// dependencies_test.go covers the dependency list of a project and the three
// export actions, which on a project that never ran a dependency scan can
// only be shown to route and to refuse an export that does not exist with
// the hint that says where one comes from.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingID is an identifier nothing on a fresh instance has, for the reads
// whose subject is the not-found they answer.
const missingID = int64(999999999)

// TestDependencies_UnscannedProject_ListsNothingAndRefusesMissingExports
// lists the dependencies of a project with no scan, plain and filtered by
// package manager, and asks for an export of a pipeline and an export ID
// that do not exist, on every surface.
//
// Replaces: TestMeta_Dependencies
func TestDependencies_UnscannedProject_ListsNothingAndRefusesMissingExports(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("deps"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		listed := harness.Do[dependencies.ListOutput](s, actionDependencyList, map[string]any{"project_id": project.IDParam()})
		if len(listed.Dependencies) != 0 {
			e.T.Errorf("a project with no scan lists %d dependencies: %+v", len(listed.Dependencies), listed.Dependencies)
		}
		filtered := harness.Do[dependencies.ListOutput](s, actionDependencyList,
			map[string]any{"project_id": project.IDParam(), "package_manager": "npm", "per_page": 10})
		if len(filtered.Dependencies) != 0 {
			e.T.Errorf("a project with no scan lists %d npm dependencies: %+v", len(filtered.Dependencies), filtered.Dependencies)
		}

		refused := harness.Refused(s, actionDependencyExportCreate, map[string]any{"pipeline_id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the export of a missing pipeline", refused, "pipeline_id", "gitlab_pipeline", "dependency scanning", "SBOM")

		refused = harness.Refused(s, actionDependencyExportGet, map[string]any{"export_id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the read of a missing export", refused, "export_id", "gitlab_create_dependency_list_export", "finished")

		refused = harness.Refused(s, actionDependencyExportDownload, map[string]any{"export_id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the download of a missing export", refused, "export_id", "gitlab_get_dependency_list_export", "CycloneDX")
	})
}
