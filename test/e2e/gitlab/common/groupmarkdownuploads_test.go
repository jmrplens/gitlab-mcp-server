//go:build e2e

// groupmarkdownuploads_test.go covers the markdown uploads of a group as
// far as an instance lets them be driven: the listing of a fresh group,
// and the two deletes refused for an upload that does not exist. GitLab
// offers no API to create a group upload (the old suite tried the raw
// endpoint and found it answering 404), so the listing is exact at empty
// and the deletes are exercised on their refusals.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingGroupUploadID is an upload id nothing on a fresh instance holds.
const missingGroupUploadID = int64(999999999)

// missingUploadSecret has the 32-hex shape a secret takes and names no
// upload.
var missingUploadSecret = strings.Repeat("0123", 8)

// TestGroupUploads_FreshGroup_ListsNoneAndRefusesDeletes lists the uploads
// of a group of each surface's own, which holds none, and shows the delete
// by id and the delete by secret refused for an upload that does not
// exist, each in the shape GitLab gives it.
//
// The shapes are not a guess, and they differ. The old suite asserted only
// that an error came back; the call record its run left behind credits the
// delete by id as a refusal and the delete by secret as an error path,
// which the recorder can only have decided from the status in the text: a
// 404 for the first, and for the second neither 404 nor 403. Which status
// the second one is, the record does not say, so it is held to the
// operation that refused it, which every wrapped error names first.
//
// Replaces: TestMeta_GroupMarkdownUploads
func TestGroupUploads_FreshGroup_ListsNoneAndRefusesDeletes(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("uploads"))
		params := map[string]any{"group_id": group.IDParam()}

		listed := harness.Do[groupmarkdownuploads.ListOutput](s, actionGroupUploadList, params)
		if len(listed.Uploads) != 0 {
			e.T.Errorf("a fresh group lists %d upload(s): %+v, want none", len(listed.Uploads), listed.Uploads)
		}

		refused := harness.Refused(s, actionGroupUploadDeleteByID, withParams(params, map[string]any{"upload_id": missingGroupUploadID}), harness.FailureNotFound)
		assertMentions(e, "the delete by id of an upload that does not exist", refused, "upload_id")
		refused = harness.ExpectToolError(s, actionGroupUploadDeleteSecret, withParams(params, map[string]any{
			"secret": missingUploadSecret, "filename": "does-not-exist.txt",
		}), "delete_group_markdown_upload_by_secret")
		e.T.Logf("the delete by secret of an upload that does not exist is refused: %s", firstLine(refused))
	})
}
