//go:build e2e

// geo_test.go covers the Geo site actions on an instance with no Geo
// deployment: a disabled secondary site can still be created, read, listed,
// edited, repaired and deleted, and the status actions answer what a site
// that never reported gives, an empty status list and a not-found for the
// site's own status.

package ee

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/geo"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// geoSiteURL is the address the fixture site claims to be at. Nothing ever
// connects to it: the site is created disabled.
const geoSiteURL = "https://geo-secondary.example.com/"

// geoRepositoryCapacity is the value the edit changes the site's backfill
// capacity to, which is read back as the proof the edit happened.
const geoRepositoryCapacity = int64(11)

// geoSiteIDs lists the IDs of a site listing.
func geoSiteIDs(sites []geo.Output) []int64 {
	ids := make([]int64, 0, len(sites))
	for _, site := range sites {
		ids = append(ids, site.ID)
	}
	return ids
}

// TestGeo_DisabledSecondarySite_Lifecycle walks one disabled site per
// surface through create, get, list, edit, the two status reads, repair and
// delete. Geo is an administrator's, so the run's token must be one.
//
// Replaces: TestMeta_Geo
func TestGeo_DisabledSecondarySite_Lifecycle(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		name := e.Name("geo")

		before := harness.Do[geo.ListOutput](s, actionGeoList, nil)
		e.T.Logf("%d Geo site(s) before the create", len(before.Sites))

		created := harness.Do[geo.Output](s, actionGeoCreate, map[string]any{
			"name": name, "url": geoSiteURL, "internal_url": geoSiteURL, "enabled": false,
		})
		if created.ID == 0 || created.Name != name || created.Enabled {
			e.T.Fatalf("create answered %+v, want a disabled site named %q with an ID", created, name)
		}
		e.Defer("geo site "+name, func(ctx context.Context) error {
			_, err := e.Client().GL().GeoSites.DeleteGeoSite(created.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		got := harness.Do[geo.Output](s, actionGeoGet, map[string]any{"id": created.ID})
		if got.ID != created.ID || got.Name != name {
			e.T.Errorf("get answered %+v, want site %d named %q", got, created.ID, name)
		}

		listed := harness.Do[geo.ListOutput](s, actionGeoList, nil)
		if !containsID(geoSiteIDs(listed.Sites), created.ID) {
			e.T.Errorf("the listing does not hold the created site %d: %v", created.ID, geoSiteIDs(listed.Sites))
		}

		edited := harness.Do[geo.Output](s, actionGeoEdit, map[string]any{
			"id": created.ID, "name": name + "-updated", "repos_max_capacity": geoRepositoryCapacity,
		})
		if edited.ID != created.ID || edited.Name != name+"-updated" || edited.ReposMaxCapacity != geoRepositoryCapacity {
			e.T.Errorf("edit answered %+v, want site %d renamed to %q with repos_max_capacity %d", edited, created.ID, name+"-updated", geoRepositoryCapacity)
		}

		statuses := harness.Do[geo.ListStatusOutput](s, actionGeoListStatus, nil)
		e.T.Logf("%d Geo status(es) reported", len(statuses.Statuses))

		// A site that never ran has reported nothing, and the read of its
		// status says so rather than answering an empty one.
		refused := harness.Refused(s, actionGeoGetStatus, map[string]any{"id": created.ID}, harness.FailureNotFound)
		assertMentions(e, "the status of a site that never reported", refused, "reported status", "gitlab_list_geo_sites")

		repaired := harness.Do[geo.Output](s, actionGeoRepair, map[string]any{"id": created.ID})
		if repaired.ID != created.ID {
			e.T.Errorf("repair answered site %d, want %d", repaired.ID, created.ID)
		}

		harness.DoVoid(s, actionGeoDelete, map[string]any{"id": created.ID})
		gone := harness.Do[geo.ListOutput](s, actionGeoList, nil)
		if containsID(geoSiteIDs(gone.Sites), created.ID) {
			e.T.Errorf("the listing still holds site %d after its delete", created.ID)
		}
	})
}
