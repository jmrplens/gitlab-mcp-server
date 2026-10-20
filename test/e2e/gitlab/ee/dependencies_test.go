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

// sbomPipeline publishes a one-component CycloneDX SBOM as the pipeline
// report GitLab's dependency list reads, so that an export of the pipeline
// has a pipeline to be made from.
const sbomPipeline = `stages:
  - test

sbom:
  stage: test
  image: alpine:latest
  variables:
    GIT_STRATEGY: none
  script:
    - |
      printf '%s' '{"bomFormat":"CycloneDX","specVersion":"1.4","serialNumber":"urn:uuid:3e671687-395b-41f5-a30f-a58921a69b79","version":1,"metadata":{"tools":[{"vendor":"gitlab-mcp-server","name":"e2e-fixture","version":"1.0.0"}]},"components":[{"name":"left-pad","version":"1.3.0","purl":"pkg:npm/left-pad@1.3.0","type":"library"}]}' > gl-sbom-npm-npm.cdx.json
  artifacts:
    when: always
    reports:
      cyclonedx: gl-sbom-npm-npm.cdx.json
`

// exportStatuses are the states GitLab names a dependency list export in.
var exportStatuses = map[string]bool{"created": true, "running": true, "finished": true, "failed": true}

// TestDependencies_ExportOfAPipeline_ReportsItsStatus runs a pipeline that
// publishes an SBOM, creates a dependency list export of it on every surface
// and reads the export back, asserting that both answers carry the status
// GitLab names it in, with has_finished agreeing, and the export's type. The
// status was not published at all, while the hints told a caller to wait for
// it: client-go's struct does not model it, and the handlers now read it from
// the captured response.
func TestDependencies_ExportOfAPipeline_ReportsItsStatus(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Pipeline {
		project := fixture.NewProject(e, fixture.WithNamePrefix("depexport"))
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, sbomPipeline, "ci: publish an SBOM "+fixture.SkipCIMarker)
		pipeline := fixture.NewPipeline(e, project, project.DefaultBranch)
		if pipeline.Status != "success" {
			e.T.Fatalf("the SBOM pipeline %d of %s ended %s, want success", pipeline.ID, project.Path, pipeline.Status)
		}
		return pipeline
	}, func(e *harness.Env, surface harness.Surface, pipeline fixture.Pipeline) {
		s := e.On(surface)

		created := harness.Do[dependencies.ExportOutput](s, actionDependencyExportCreate, map[string]any{"pipeline_id": pipeline.ID})
		if created.ID == 0 || !exportStatuses[created.Status] || created.ExportType == "" {
			e.T.Fatalf("the export create answered %+v, want an export with a status GitLab names and its type", created)
		}
		read := harness.Do[dependencies.ExportOutput](s, actionDependencyExportGet, map[string]any{"export_id": created.ID})
		if read.ID != created.ID || !exportStatuses[read.Status] || read.HasFinished != (read.Status == "finished") {
			e.T.Errorf("the export read answered %+v, want export %d with a status that agrees with has_finished", read, created.ID)
		}
	})
}

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
		assertMentions(e, "the export of a missing pipeline", refused, "pipeline_id", "pipeline.list", "dependency scanning", "SBOM")

		refused = harness.Refused(s, actionDependencyExportGet, map[string]any{"export_id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the read of a missing export", refused, "export_id", "dependency.export_create", "finished")

		refused = harness.Refused(s, actionDependencyExportDownload, map[string]any{"export_id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the download of a missing export", refused, "export_id", "dependency.export_get", "CycloneDX")
	})
}
