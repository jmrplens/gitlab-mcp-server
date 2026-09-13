//go:build e2e

// pages_test.go covers a project's Pages: the settings read and changed,
// the custom domains through their life, and the unpublish of a site a
// pipeline deployed. The Docker omnibus configuration enables Pages, so
// there every call is held to its answer; on another instance the first
// read says whether Pages is configured at all, and the test skips with
// that reason when it is not.

package common

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// pagesCIYAML publishes a minimal Pages site: the reserved pages job
// uploads a public directory as its artifact, which GitLab deploys.
const pagesCIYAML = `pages:
  script:
    - mkdir -p public
    - echo "e2e pages" > public/index.html
  artifacts:
    paths:
      - public
`

// How long the deployment a pipeline made is waited for, since GitLab
// creates it after the job's artifact is processed rather than when the
// pipeline reports success.
const (
	pagesDeployInterval = 3 * time.Second
	pagesDeployWait     = 2 * time.Minute
)

// pagesDomainNames lists the domains of a domain listing.
func pagesDomainNames(listed []pages.DomainOutput) []string {
	names := make([]string, 0, len(listed))
	for _, domain := range listed {
		names = append(names, domain.Domain)
	}
	return names
}

// pagesDomainFor spells a custom domain of the surface's own under a name
// nobody resolves. Each label is short enough for a hostname, which a
// run-scoped name in one label is not.
func pagesDomainFor(e *harness.Env, surface harness.Surface) string {
	return fmt.Sprintf("pages-%s.%s.example.com", surface, e.RunID())
}

// requirePages reads the project's Pages settings, which is the one call
// that says whether Pages is configured on the instance: the Docker stack
// enables it and a refusal there is a failure, while another instance may
// not, and the test skips naming the refusal.
func requirePages(e *harness.Env, s *harness.Session, project fixture.Project) pages.Output {
	e.T.Helper()
	params := map[string]any{"project_id": project.IDParam()}
	if e.DockerMode() {
		return harness.Do[pages.Output](s, actionProjectPagesGet, params)
	}
	settings, err := harness.Try[pages.Output](s, actionProjectPagesGet, params)
	if err != nil {
		e.Skipf("GitLab Pages is not configured on this instance: %s", firstLine(err.Error()))
	}
	return settings
}

// TestPages_SettingsAndDomains_ReadUpdateAndRoundTrip reads the Pages
// settings of a project of each surface's own, turns HTTPS-only off,
// lists the instance's domains and the project's, and walks a custom
// domain of the surface's own through create, get, update, list and
// delete.
//
// Replaces: TestMeta_ProjectPages, TestMeta_ProjectPagesWrite
func TestPages_SettingsAndDomains_ReadUpdateAndRoundTrip(t *testing.T) {
	e := harness.New(t)

	// A project of each surface's own, rather than one shared: the listing
	// before the create is asserted empty, and a shared project makes that
	// true only for whichever surface runs first and only while the others
	// reach their delete.
	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("pages"))
		params := map[string]any{"project_id": project.IDParam()}

		settings := requirePages(e, s, project)
		if settings.URL == "" {
			e.T.Errorf("pages_get answered %+v, want the site's URL", settings)
		}
		updated := harness.Do[pages.Output](s, actionProjectPagesUpdate, withParams(params, map[string]any{"pages_https_only": false}))
		if updated.ForceHTTPS {
			e.T.Errorf("pages_update answered %+v after turning HTTPS-only off, want force_https false", updated)
		}

		all := harness.Do[pages.ListAllDomainsOutput](s, actionProjectPagesDomainListAll, nil)
		e.T.Logf("the instance lists %d Pages domain(s)", len(all.Domains))
		before := harness.Do[pages.ListDomainsOutput](s, actionProjectPagesDomainList, params)
		if len(before.Domains) != 0 {
			e.T.Errorf("the project lists the domains %v before any was created, want none", pagesDomainNames(before.Domains))
		}

		domain := pagesDomainFor(e, surface)
		named := withParams(params, map[string]any{"domain": domain})
		// The create answers no project id: GitLab's create entity omits
		// it, and the association is read off the project's own listing
		// below instead.
		created := harness.Do[pages.DomainOutput](s, actionProjectPagesDomainCreate, named)
		if created.Domain != domain || created.URL == "" {
			e.T.Fatalf("pages_domain_create answered %+v, want the domain %q with its URL", created, domain)
		}
		got := harness.Do[pages.DomainOutput](s, actionProjectPagesDomainGet, named)
		if got.Domain != domain {
			e.T.Errorf("pages_domain_get answered %+v, want the domain %q", got, domain)
		}
		changed := harness.Do[pages.DomainOutput](s, actionProjectPagesDomainUpdate, withParams(named, map[string]any{"auto_ssl_enabled": false}))
		if changed.Domain != domain || changed.AutoSslEnabled {
			e.T.Errorf("pages_domain_update answered %+v, want the domain %q with automatic SSL off", changed, domain)
		}
		listed := harness.Do[pages.ListDomainsOutput](s, actionProjectPagesDomainList, params)
		if !containsKey(pagesDomainNames(listed.Domains), domain) {
			e.T.Errorf("the project lists the domains %v, want %q among them", pagesDomainNames(listed.Domains), domain)
		}

		harness.DoVoid(s, actionProjectPagesDomainDelete, named)
		refused := harness.Refused(s, actionProjectPagesDomainGet, named, harness.FailureNotFound)
		e.T.Logf("the read of the deleted domain is refused: %s", firstLine(refused))
	})
}

// TestPages_Unpublish_RemovesTheDeployment deploys a Pages site in a
// project of each surface's own through a pipeline, reads the deployment
// off the settings, unpublishes the site and reads the settings again.
//
// Replaces: TestMeta_ProjectPagesWrite
func TestPages_Unpublish_RemovesTheDeployment(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("pagesdeploy"))
		params := map[string]any{"project_id": project.IDParam()}

		deployed := requirePages(e, s, project)
		if len(deployed.Deployments) != 0 {
			e.T.Errorf("a project nothing deployed to lists the deployments %+v, want none", deployed.Deployments)
		}
		deployPages(e, project)
		// GitLab creates the deployment after the job's artifact is
		// processed, which is not finished when the pipeline reports
		// success, so the read is waited for rather than taken once.
		published := harness.Eventually(s, actionProjectPagesGet, params, pagesDeployInterval, pagesDeployWait,
			func(settings pages.Output) bool { return len(settings.Deployments) != 0 })
		if published.URL == "" {
			e.T.Errorf("pages_get answered %+v after the pipeline, want the deployed site's URL", published)
		}

		unpublished := harness.Do[toolutil.DeleteOutput](s, actionProjectPagesUnpublish, params)
		if unpublished.Status != voidStatusSuccess {
			e.T.Errorf("pages_unpublish answered %+v, want a %s status", unpublished, voidStatusSuccess)
		}
		// GitLab answers the read of an unpublished site either way it can:
		// the settings with no deployment, or a not-found; both say the
		// deployment is gone, and anything else does not.
		after, err := harness.Try[pages.Output](s, actionProjectPagesGet, params)
		switch {
		case err != nil && strings.Contains(strings.ToLower(err.Error()), "not found"):
			e.T.Logf("the read after the unpublish is refused: %s", firstLine(err.Error()))
		case err != nil:
			e.T.Errorf("the read after the unpublish failed for another reason than not found: %v", err)
		case len(after.Deployments) != 0:
			e.T.Errorf("the site still lists the deployments %+v after the unpublish", after.Deployments)
		}
	})
}

// deployPages commits the Pages CI configuration to the project's default
// branch and runs a pipeline on it to success, so the project has a live
// deployment to unpublish.
func deployPages(e *harness.Env, project fixture.Project) {
	e.T.Helper()
	fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, pagesCIYAML, "ci: publish a pages site")
	pipeline := fixture.NewPipeline(e, project, project.DefaultBranch)
	if pipeline.Status != "success" {
		e.T.Fatalf("the pages pipeline %d ended in %q, want success", pipeline.ID, pipeline.Status)
	}
}
