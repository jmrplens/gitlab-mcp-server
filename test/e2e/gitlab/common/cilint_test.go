//go:build e2e

// cilint_test.go covers the two CI lint reads: a configuration given
// inline, and the configuration a project holds, which a project without
// one fails.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// validCIYAML is a configuration the linter accepts.
const validCIYAML = "stages:\n  - build\nbuild_job:\n  stage: build\n  script:\n    - echo hello\n"

// lintDefaults spells the optional arguments of both lint actions with the
// values their handlers treat as absent.
//
// They are spelled rather than omitted because the individual surface
// validates a call against the served schema, and these fields are marked
// required there: their json tags carry no omitempty, which is what the SDK
// derives requiredness from. The call means the same on every surface with
// them spelled this way, and a model that omits them is refused, which is
// the finding this batch reports rather than fixes.
var lintDefaults = map[string]any{"dry_run": false, "include_jobs": false, "ref": ""}

// TestCILint_ContentAndProject_JudgesBothShapes lints a valid configuration
// given inline on every surface, and lints a shared project that holds no
// .gitlab-ci.yml, which the linter reports as invalid.
//
// Replaces: TestIndividual_CILint, TestMeta_CILint
func TestCILint_ContentAndProject_JudgesBothShapes(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("cilint"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		inline := harness.Do[cilint.Output](s, actionTemplateLint, withParams(withParams(params, lintDefaults), map[string]any{"content": validCIYAML}))
		if !inline.Valid || len(inline.Errors) != 0 {
			e.T.Errorf("lint of a valid configuration answered %+v, want valid with no errors", inline)
		}

		// The project lint takes two more of the same kind.
		missing := harness.Do[cilint.Output](s, actionTemplateLintProject, withParams(withParams(params, lintDefaults), map[string]any{"content_ref": "", "dry_run_ref": ""}))
		if missing.Valid {
			e.T.Errorf("lint of a project without a CI configuration answered %+v, want invalid", missing)
		}
		e.T.Logf("the project without a configuration lints as invalid: %v", missing.Errors)
	})
}
