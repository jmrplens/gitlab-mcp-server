// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// groupmarkdownuploads_test.go covers the markdown uploads of a group: the
// listing of a fresh group and the two deletes refused for an upload that
// does not exist, and two files uploaded, listed and deleted the two ways
// GitLab addresses an upload. The upload is POST /groups/:id/uploads, which
// GitLab serves from 19.0 and client-go models from v3.42.0; the old suite,
// which had no such action, tried the raw endpoint on an older instance and
// found it answering 404.

package common

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestGroupUploads_UploadListAndDelete_ByIDAndBySecret uploads two text
// files to a group of each surface's own, finds them in the listing,
// deletes the first by id and the second by the secret and file name its
// URL carries, and checks the listing lets each go.
func TestGroupUploads_UploadListAndDelete_ByIDAndBySecret(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("uploads"))
		params := map[string]any{"group_id": group.IDParam()}

		byID := uploadGroupText(e, s, params, "by-id.txt", "delete me by id")
		if byID.ID == 0 || !strings.HasPrefix(byID.FullPath, "/-/group/") {
			e.T.Fatalf("group_upload answered %+v, want an id and a full path under /-/group/", byID)
		}
		bySecret := uploadGroupText(e, s, params, "by-secret.txt", "delete me by secret")
		listed := harness.Do[groupmarkdownuploads.ListOutput](s, actionGroupUploadList, params)
		if ids := groupUploadIDs(listed.Uploads); !containsID(ids, byID.ID) || !containsID(ids, bySecret.ID) {
			e.T.Errorf("the group lists the uploads %v, want both %d and %d", ids, byID.ID, bySecret.ID)
		}

		harness.DoVoid(s, actionGroupUploadDeleteByID, withParams(params, map[string]any{"upload_id": byID.ID}))
		afterID := harness.Do[groupmarkdownuploads.ListOutput](s, actionGroupUploadList, params)
		if containsID(groupUploadIDs(afterID.Uploads), byID.ID) {
			e.T.Errorf("the group still lists upload %d after its delete by id", byID.ID)
		}

		secret, filename := uploadSecret(e, bySecret.URL)
		harness.DoVoid(s, actionGroupUploadDeleteSecret, withParams(params, map[string]any{"secret": secret, "filename": filename}))
		afterSecret := harness.Do[groupmarkdownuploads.ListOutput](s, actionGroupUploadList, params)
		if containsID(groupUploadIDs(afterSecret.Uploads), bySecret.ID) {
			e.T.Errorf("the group still lists upload %d after its delete by secret", bySecret.ID)
		}
	})
}

// uploadGroupText uploads a small text file to a group and reads the upload
// off the answer.
func uploadGroupText(e *harness.Env, s *harness.Session, params map[string]any, filename, content string) groupmarkdownuploads.UploadOutput {
	e.T.Helper()
	uploaded := harness.Do[groupmarkdownuploads.UploadOutput](s, actionGroupUpload, withParams(params, map[string]any{
		"filename": filename, "content_base64": base64.StdEncoding.EncodeToString([]byte(content)),
	}))
	if uploaded.URL == "" || !strings.HasSuffix(uploaded.URL, "/"+filename) {
		e.T.Fatalf("group_upload answered %+v, want a URL ending in %q", uploaded, filename)
	}
	return uploaded
}

// groupUploadIDs lists the ids of a group upload listing.
func groupUploadIDs(listed []groupmarkdownuploads.UploadItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, upload := range listed {
		ids = append(ids, upload.ID)
	}
	return ids
}

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
