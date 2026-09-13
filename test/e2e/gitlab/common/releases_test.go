//go:build e2e

// releases_test.go covers a release through the server: the tag it stands
// on, its lifecycle, and the asset links it carries, one at a time and in
// a batch.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// releaseTagNames returns the tag names of a release listing.
func releaseTagNames(listed []releases.Output) []string {
	names := make([]string, 0, len(listed))
	for _, release := range listed {
		names = append(names, release.TagName)
	}
	return names
}

// releaseLinkIDs returns the identifiers of a release link listing.
func releaseLinkIDs(listed []releaselinks.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, link := range listed {
		ids = append(ids, link.ID)
	}
	return ids
}

// releaseLinkLifecycle adds an asset link to a release, finds it in the
// listing, reads and renames it, adds two more links in one batch, deletes
// the first and asserts the listing is left with the batch's two.
func releaseLinkLifecycle(e *harness.Env, s *harness.Session, release map[string]any, tag string) {
	e.T.Helper()

	linkURL := "https://example.com/releases/" + tag + "/binary-linux-amd64"
	link := harness.Do[releaselinks.Output](s, actionReleaseLinkCreate, withParams(release, map[string]any{
		"name": "binary-linux-amd64", "url": linkURL, "link_type": "package",
	}))
	if link.ID == 0 || link.Name != "binary-linux-amd64" || link.URL != linkURL || link.LinkType != "package" {
		e.T.Fatalf("release link create answered %+v, want a package link to %s with an ID", link, linkURL)
	}
	byLink := withParams(release, map[string]any{"link_id": link.ID})

	links := harness.Do[releaselinks.ListOutput](s, actionReleaseLinkList, release)
	if !containsID(releaseLinkIDs(links.Links), link.ID) {
		e.T.Errorf("the link listing of %s does not hold %d: %v", tag, link.ID, releaseLinkIDs(links.Links))
	}
	linkGot := harness.Do[releaselinks.Output](s, actionReleaseLinkGet, byLink)
	if linkGot.ID != link.ID || linkGot.Name != link.Name {
		e.T.Errorf("release link get answered %+v, want link %d %q", linkGot, link.ID, link.Name)
	}
	linkUpdated := harness.Do[releaselinks.Output](s, actionReleaseLinkUpdate, withParams(byLink, map[string]any{"name": "binary-linux-amd64-renamed"}))
	if linkUpdated.ID != link.ID || linkUpdated.Name != "binary-linux-amd64-renamed" {
		e.T.Errorf("release link update answered %+v, want link %d renamed", linkUpdated, link.ID)
	}

	batch := harness.Do[releaselinks.CreateBatchOutput](s, actionReleaseLinkCreateBatch, withParams(release, map[string]any{
		"links": []map[string]any{
			{"name": "batch-one", "url": "https://example.com/releases/" + tag + "/batch-one.zip"},
			{"name": "batch-two", "url": "https://example.com/releases/" + tag + "/batch-two.zip"},
		},
	}))
	if len(batch.Created) != 2 {
		e.T.Errorf("release link create batch answered %+v, want the two links created", batch)
	}

	linkDeleted := harness.Do[releaselinks.Output](s, actionReleaseLinkDelete, byLink)
	if linkDeleted.ID != link.ID {
		e.T.Errorf("release link delete answered %+v, want link %d", linkDeleted, link.ID)
	}
	remaining := harness.Do[releaselinks.ListOutput](s, actionReleaseLinkList, release)
	if containsID(releaseLinkIDs(remaining.Links), link.ID) || len(remaining.Links) != 2 {
		e.T.Errorf("the link listing after the delete holds %v, want the two batch links and not %d", releaseLinkIDs(remaining.Links), link.ID)
	}
}

// TestRelease_Lifecycle_CreateGetUpdateListLinksDelete tags a shared
// project per surface, creates a release on the tag, reads and lists it,
// changes its description, adds an asset link and reads, lists, renames and
// deletes it, adds two links in one batch, deletes the release and asserts
// the read afterwards is refused as not found.
//
// Replaces: TestIndividual_Releases, TestMeta_Releases, TestMeta_ReleaseLinksExtended
func TestRelease_Lifecycle_CreateGetUpdateListLinksDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("releases"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		tag := createTag(e, s, project, e.Name("v1.0.0"))
		params := map[string]any{"project_id": project.IDParam(), "tag_name": tag.Name}
		title := "e2e release " + tag.Name

		created := harness.Do[releases.Output](s, actionReleaseCreate, withParams(params, map[string]any{"name": title, "description": "automated e2e release"}))
		if created.TagName != tag.Name || created.Name != title {
			e.T.Fatalf("release create answered %+v, want %q on %s", created, title, tag.Name)
		}
		got := harness.Do[releases.Output](s, actionReleaseGet, params)
		if got.TagName != tag.Name || got.Name != title || got.Description != "automated e2e release" {
			e.T.Errorf("release get answered %+v, want %q on %s with its description", got, title, tag.Name)
		}
		updated := harness.Do[releases.Output](s, actionReleaseUpdate, withParams(params, map[string]any{"description": "updated e2e release"}))
		if updated.TagName != tag.Name || updated.Description != "updated e2e release" {
			e.T.Errorf("release update answered %+v, want the release on %s with the new description", updated, tag.Name)
		}
		listed := harness.Do[releases.ListOutput](s, actionReleaseList, map[string]any{"project_id": project.IDParam()})
		if !slices.Contains(releaseTagNames(listed.Releases), tag.Name) {
			e.T.Errorf("the release listing does not hold %s: %v", tag.Name, releaseTagNames(listed.Releases))
		}

		releaseLinkLifecycle(e, s, params, tag.Name)

		deleted := harness.Do[releases.Output](s, actionReleaseDelete, params)
		if deleted.TagName != tag.Name {
			e.T.Errorf("release delete answered %+v, want the release on %s", deleted, tag.Name)
		}
		refused := harness.Refused(s, actionReleaseGet, params, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
