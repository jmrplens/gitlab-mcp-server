//go:build e2e

// groupreleases_test.go covers the release listing of a group, which
// aggregates the releases of its projects: empty for a group whose project
// has none, then holding the release the fixture library makes, in the
// full and the simple shape.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupreleases"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupReleaseTagNames lists the tag names of a release listing.
func groupReleaseTagNames(listed []groupreleases.Output) []string {
	names := make([]string, 0, len(listed))
	for _, release := range listed {
		names = append(names, release.TagName)
	}
	return names
}

// TestGroupReleases_List_AggregatesTheProjectsReleases lists the releases
// of a group of each surface's own before and after its project gets one,
// and once more in the simple shape.
//
// Replaces: TestMeta_GroupReleases
func TestGroupReleases_List_AggregatesTheProjectsReleases(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("releases"))
		project := fixture.NewProject(e, fixture.WithNamePrefix("released"), fixture.InGroup(group))
		params := map[string]any{"group_id": group.IDParam()}

		empty := harness.Do[groupreleases.ListOutput](s, actionGroupReleaseList, params)
		if len(empty.Releases) != 0 {
			e.T.Errorf("a group whose project has no release lists %v, want none", groupReleaseTagNames(empty.Releases))
		}

		release := fixture.NewRelease(e, project, "v")
		listed := harness.Do[groupreleases.ListOutput](s, actionGroupReleaseList, params)
		if names := groupReleaseTagNames(listed.Releases); len(names) != 1 || names[0] != release.TagName {
			e.T.Errorf("the group lists the releases %v, want exactly the tag %q", names, release.TagName)
		}
		simple := harness.Do[groupreleases.ListOutput](s, actionGroupReleaseList, withParams(params, map[string]any{"simple": true}))
		if names := groupReleaseTagNames(simple.Releases); len(names) != 1 || names[0] != release.TagName {
			e.T.Errorf("the simple listing holds %v, want exactly the tag %q", names, release.TagName)
		}
	})
}
