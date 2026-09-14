//go:build e2e

// project_snippet.go builds a snippet inside a project, which is the object
// the snippet notes, discussions and award emoji hang off. It goes with the
// project, so nothing is registered: a personal snippet (snippet.go) has no
// parent and registers its own deletion, a project snippet has one.

package fixture

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The one file every project snippet fixture carries, spelled once so that
// a test reading the snippet back knows what to expect.
const (
	projectSnippetFilePath = "e2e.txt"
	projectSnippetContent  = "e2e project snippet fixture\n"
)

// ProjectSnippet is a project snippet a builder created.
type ProjectSnippet struct {
	// ID is what every project snippet action takes as snippet_id.
	ID int64
	// ProjectID is the project it lives in.
	ProjectID int64
	// Title is what it was created as.
	Title string
	// FileName is the one file it carries.
	FileName string
}

// NewProjectSnippet creates a private snippet with one file in the project,
// titled with the run's own scoping.
func NewProjectSnippet(e *harness.Env, project Project) ProjectSnippet {
	e.T.Helper()

	title := e.Name("snippet")
	snippet, err := retryTransient(e, "create project snippet "+title, createRetries, func() (ProjectSnippet, error) {
		created, _, err := e.Client().GL().ProjectSnippets.CreateSnippet(project.ID, &gl.CreateProjectSnippetOptions{
			Title:       new(title),
			Description: new("e2e: " + e.T.Name()),
			Visibility:  new(gl.PrivateVisibility),
			Files: &[]*gl.CreateSnippetFileOptions{
				{FilePath: new(projectSnippetFilePath), Content: new(projectSnippetContent)},
			},
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return ProjectSnippet{}, err
		}
		return projectSnippetOf(created, project.ID), nil
	})
	if err != nil {
		e.T.Fatalf("creating project snippet %q in project %d: %v", title, project.ID, err)
	}
	return snippet
}

// projectSnippetOf reads what a test needs out of what GitLab returned. The
// project is the one the builder was asked for: GitLab answers a project
// snippet's project_id too, and an answer that omitted it would otherwise
// leave a test addressing project 0.
func projectSnippetOf(s *gl.Snippet, projectID int64) ProjectSnippet {
	snippet := ProjectSnippet{ID: s.ID, ProjectID: projectID, Title: s.Title, FileName: s.FileName}
	if snippet.FileName == "" && len(s.Files) > 0 {
		snippet.FileName = s.Files[0].Path
	}
	return snippet
}
