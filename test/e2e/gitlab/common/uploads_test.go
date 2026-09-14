//go:build e2e

// uploads_test.go covers a project's markdown uploads: sending one from
// base64 content and finding it in the listing afterwards.

package common

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestUpload_SendAndList_ListsWhatWasSent uploads a small text file to a
// shared project on every surface, reads the URL and the markdown the
// answer carries, and finds the file in the project's upload listing.
//
// Replaces: TestIndividual_Uploads, TestMeta_Uploads
func TestUpload_SendAndList_ListsWhatWasSent(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("uploads"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		filename := "e2e-" + string(surface) + ".txt"
		content := base64.StdEncoding.EncodeToString([]byte("e2e upload content on " + string(surface)))

		uploaded := harness.Do[uploads.UploadOutput](s, actionProjectUpload, map[string]any{
			"project_id": project.IDParam(), "filename": filename, "content_base64": content,
		})
		if !strings.HasSuffix(uploaded.URL, "/"+filename) || !strings.Contains(uploaded.Markdown, filename) {
			e.T.Errorf("upload answered %+v, want a URL ending in %s and markdown naming it", uploaded, filename)
		}

		listed := harness.Do[uploads.ListOutput](s, actionProjectUploadList, map[string]any{"project_id": project.IDParam()})
		if !slices.Contains(uploadFilenames(listed.Uploads), filename) {
			e.T.Errorf("the upload listing does not hold %s: %v", filename, uploadFilenames(listed.Uploads))
		}
	})
}
