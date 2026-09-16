//go:build e2e

// geo.go registers a Geo secondary site, and reserves the name and URL a case
// registers one under.
//
// Nothing ever connects to the site: it is created disabled, which is what
// lets an instance with no Geo deployment hold a site record and answer every
// site action about it. GitLab holds both a site's name and its URL unique
// instance-wide, so neither can be a literal in a case: one case run three
// times against one instance would have every attempt after the first refused
// for a name the first took.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// geoSiteHost is the domain a fixture site claims to be under. It resolves to
// nothing, which is safe because a disabled site is never contacted.
const geoSiteHost = "https://%s.geo.example.invalid/"

// GeoSite is a Geo site a builder registered.
type GeoSite struct {
	// ID is what every Geo site action takes.
	ID int64
	// Name and URL are what it was registered as.
	Name string
	URL  string
}

// GeoSiteName is a name and URL no site on the instance holds, for the case
// that registers one itself.
type GeoSiteName struct {
	Name string
	URL  string
}

// NewGeoSite registers a disabled secondary site and registers its removal.
func NewGeoSite(e *harness.Env) GeoSite {
	e.T.Helper()

	reserved := ReserveGeoSiteName(e)
	site, err := retryTransient(e, "create geo site "+reserved.Name, createRetries, func() (GeoSite, error) {
		return createGeoSite(e.Ctx, e.Client(), reserved.Name, reserved.URL)
	})
	if err != nil {
		e.T.Fatalf("registering Geo site %q: %v", reserved.Name, err)
	}

	e.Defer("geo site "+reserved.Name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteGeoSite(ctx, e.Client(), site.ID)
	})
	return site
}

// ReserveGeoSiteName returns a name and URL nothing on the instance holds.
//
// It registers no removal: a site created under this name by a case is found
// by its name, not by an identifier this reservation could keep, and the
// run's own sweep is what clears what a case left behind.
func ReserveGeoSiteName(e *harness.Env) GeoSiteName {
	e.T.Helper()

	name := e.Name("geo")
	return GeoSiteName{Name: name, URL: fmt.Sprintf(geoSiteHost, name)}
}

// createGeoSite asks GitLab for the site.
func createGeoSite(ctx context.Context, client *gitlabclient.Client, name, url string) (GeoSite, error) {
	created, _, err := client.GL().GeoSites.CreateGeoSite(&gl.CreateGeoSitesOptions{
		Name:        new(name),
		URL:         new(url),
		InternalURL: new(url),
		Enabled:     new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return GeoSite{}, err
	}
	return GeoSite{ID: created.ID, Name: created.Name, URL: created.URL}, nil
}

// deleteGeoSite removes the site and tolerates one a case deleted.
func deleteGeoSite(ctx context.Context, client *gitlabclient.Client, siteID int64) error {
	_, err := client.GL().GeoSites.DeleteGeoSite(siteID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting Geo site %d: %w", siteID, err)
	}
	return nil
}
