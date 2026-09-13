//go:build e2e

// projectuploads_test.go covers the markdown uploads of a project: two
// small files uploaded, listed, and deleted the two ways GitLab addresses
// an upload, by its id and by the secret and file name its URL carries,
// which is the only place GitLab shows that secret.

package common

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// uploadIDs lists the ids of an upload listing.
func uploadIDs(listed []uploads.ListItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, upload := range listed {
		ids = append(ids, upload.ID)
	}
	return ids
}

// uploadFilenames lists the file names of an upload listing.
func uploadFilenames(listed []uploads.ListItem) []string {
	names := make([]string, 0, len(listed))
	for _, upload := range listed {
		names = append(names, upload.Filename)
	}
	return names
}

// uploadSecret reads the secret and the file name out of an upload's
// /uploads/<secret>/<filename> URL, failing the test on any other shape.
func uploadSecret(e *harness.Env, uploadURL string) (secret, filename string) {
	e.T.Helper()
	parts := strings.Split(strings.Trim(uploadURL, "/"), "/")
	if len(parts) < 3 || parts[len(parts)-3] != "uploads" {
		e.T.Fatalf("the upload URL %q is not of the shape .../uploads/<secret>/<filename>", uploadURL)
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

// TestProjectUploads_UploadListAndDelete_ByIDAndBySecret uploads two text
// files to a project of each surface's own, finds them in the listing,
// deletes the first by id and the second by its secret and file name, and
// checks the listing lets each go.
//
// Replaces: TestMeta_ProjectUploadDeletes
func TestProjectUploads_UploadListAndDelete_ByIDAndBySecret(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("uploads"))
		params := map[string]any{"project_id": project.IDParam()}

		byID := uploadText(e, s, params, "by-id.txt", "delete me by id")
		if byID.ID == 0 {
			e.T.Fatalf("upload answered %+v with no id, which GitLab has returned since 17.3", byID)
		}
		bySecret := uploadText(e, s, params, "by-secret.txt", "delete me by secret")
		listed := harness.Do[uploads.ListOutput](s, actionProjectUploadList, params)
		if ids := uploadIDs(listed.Uploads); !containsID(ids, byID.ID) || !containsID(ids, bySecret.ID) {
			e.T.Errorf("the project lists the uploads %v, want both %d and %d", ids, byID.ID, bySecret.ID)
		}

		harness.DoVoid(s, actionProjectUploadDelete, withParams(params, map[string]any{"upload_id": byID.ID}))
		afterID := harness.Do[uploads.ListOutput](s, actionProjectUploadList, params)
		if containsID(uploadIDs(afterID.Uploads), byID.ID) {
			e.T.Errorf("the project still lists upload %d after its delete by id", byID.ID)
		}

		secret, filename := uploadSecret(e, bySecret.URL)
		harness.DoVoid(s, actionProjectUploadDeleteBySecret, withParams(params, map[string]any{"secret": secret, "filename": filename}))
		afterSecret := harness.Do[uploads.ListOutput](s, actionProjectUploadList, params)
		if containsKey(uploadFilenames(afterSecret.Uploads), filename) {
			e.T.Errorf("the project still lists %q after its delete by secret", filename)
		}
	})
}

// uploadText uploads a small text file and reads the upload off the answer.
func uploadText(e *harness.Env, s *harness.Session, params map[string]any, filename, content string) uploads.UploadOutput {
	e.T.Helper()
	uploaded := harness.Do[uploads.UploadOutput](s, actionProjectUpload, withParams(params, map[string]any{
		"filename": filename, "content_base64": base64.StdEncoding.EncodeToString([]byte(content)),
	}))
	if uploaded.URL == "" || !strings.HasSuffix(uploaded.URL, "/"+filename) {
		e.T.Fatalf("upload answered %+v, want a URL ending in %q", uploaded, filename)
	}
	return uploaded
}
