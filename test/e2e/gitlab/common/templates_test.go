//go:build e2e

// templates_test.go ports the instance template reads and the markdown render
// the old suite drove: the CI YAML, Dockerfile, .gitignore, license and
// project template listings and single-template reads, and rendering markdown
// to HTML.
//
// These are pure reads of what the instance ships, so each runs on all three
// surfaces: the same list and the same read reached through the dynamic
// find/execute pair, the domain tool and the declared individual tool are
// three code paths, and the old suite exercised only one of them per action.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/licensetemplates"
	markdowntool "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/markdown"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projecttemplates"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestTemplates_Instance lists and reads a known template from each of the
// instance-wide template families: CI YAML, Dockerfile, .gitignore and
// license.
//
// Replaces: TestMeta_TemplatesCIYml, TestMeta_TemplatesDockerfile, TestMeta_TemplatesGitignore, TestMeta_TemplatesLicense, TestMeta_Templates
func TestTemplates_Instance(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		ciList := harness.Do[ciyamltemplates.ListOutput](s, actionTemplateCIYmlList, nil)
		if len(ciList.Templates) == 0 {
			e.T.Errorf("ci_yml_list answered no templates")
		}
		ci := harness.Do[ciyamltemplates.GetOutput](s, actionTemplateCIYmlGet, map[string]any{"key": "Auto-DevOps"})
		if ci.Content == "" {
			e.T.Errorf("ci_yml_get answered empty content for Auto-DevOps")
		}

		dockerList := harness.Do[dockerfiletemplates.ListOutput](s, actionTemplateDockerfileList, nil)
		if len(dockerList.Templates) == 0 {
			e.T.Errorf("dockerfile_list answered no templates")
		}
		docker := harness.Do[dockerfiletemplates.GetOutput](s, actionTemplateDockerfileGet, map[string]any{"key": "Binary"})
		if docker.Content == "" {
			e.T.Errorf("dockerfile_get answered empty content for Binary")
		}

		gitignoreList := harness.Do[gitignoretemplates.ListOutput](s, actionTemplateGitignoreList, nil)
		if len(gitignoreList.Templates) == 0 {
			e.T.Errorf("gitignore_list answered no templates")
		}
		gitignore := harness.Do[gitignoretemplates.GetOutput](s, actionTemplateGitignoreGet, map[string]any{"key": "Go"})
		if gitignore.Content == "" {
			e.T.Errorf("gitignore_get answered empty content for Go")
		}

		licenseList := harness.Do[licensetemplates.ListOutput](s, actionTemplateLicenseList, nil)
		if len(licenseList.Licenses) == 0 {
			e.T.Errorf("license_list answered no licenses")
		}
		const licenseKey = "mit"
		license := harness.Do[licensetemplates.GetOutput](s, actionTemplateLicenseGet, map[string]any{"key": licenseKey})
		if license.Key != licenseKey {
			e.T.Errorf("license_get answered key %q, want %q", license.Key, licenseKey)
		}
	})
}

// TestTemplates_Project lists and reads a project-scoped .gitignore template,
// which needs a project to resolve against.
//
// Replaces: TestMeta_TemplatesProject
func TestTemplates_Project(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("templates"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		list := harness.Do[projecttemplates.ListOutput](s, actionTemplateProjectTmplList, map[string]any{
			"project_id": project.IDParam(), "template_type": "gitignores", "per_page": 100,
		})
		if len(list.Templates) == 0 {
			e.T.Errorf("project_template_list answered no gitignore templates")
		}

		got := harness.Do[projecttemplates.GetOutput](s, actionTemplateProjectTmplGet, map[string]any{
			"project_id": project.IDParam(), "template_type": "gitignores", "key": "Go",
		})
		if got.Name != "Go" || got.Content == "" {
			e.T.Errorf("project_template_get answered %+v, want the Go template with content", got.TemplateItem)
		}
	})
}

// TestMarkdown_Render renders a markdown document to HTML on every surface.
//
// Replaces: TestMeta_MarkdownRender, TestIndividual_MarkdownRender
func TestMarkdown_Render(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		rendered := harness.Do[markdowntool.RenderOutput](s, actionRepositoryMarkdownRender, map[string]any{"text": "**bold** text"})
		if !strings.Contains(rendered.HTML, "<strong>bold</strong>") || !strings.Contains(rendered.HTML, "text") {
			e.T.Errorf("markdown_render answered %q, want the bold element and the text it was given", rendered.HTML)
		}
	})
}
