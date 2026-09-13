//go:build e2e

// projectcore_test.go covers what a project answers about itself beyond
// its own lifecycle: the listings it appears in and the ones it owns, the
// star and the archive flag a caller can turn on and off, the fork of it
// and the housekeeping it can be asked to start. The old suite drove all of
// this through the meta tool alone; here each scenario runs on every
// surface, the reads against one shared project and the state changes
// against a project of each surface's own.

package common

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectstatistics"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// voidStatusSuccess is what a void action answers in its status field when
// it ran.
const voidStatusSuccess = "success"

// projectIDsOf lists the ids of a project listing.
func projectIDsOf(listed []projects.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, project := range listed {
		ids = append(ids, project.ID)
	}
	return ids
}

// projectUserIDs lists the ids of a project's user listing.
func projectUserIDs(users []projects.ProjectUserOutput) []int64 {
	ids := make([]int64, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	return ids
}

// projectGroupIDs lists the ids of a project's group listing.
func projectGroupIDs(groups []projects.ProjectGroupOutput) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

// TestProjectReads_FixtureProject_ListsAndDescribes reads one shared
// project on every surface through every read the project tool offers
// beside get: the listings it appears in, filtered down to it by name; the
// users, groups, starrers and forks it has, which are the run's user and
// nothing else for a fresh personal project; its languages, its fetch
// statistics and the storage shard it lives on.
//
// Replaces: TestMeta_ProjectCore
func TestProjectReads_FixtureProject_ListsAndDescribes(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("projreads"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		user := e.Runtime()

		listed := harness.Do[projects.ListOutput](s, actionProjectList, map[string]any{"membership": true, "search": project.Name})
		if !containsID(projectIDsOf(listed.Projects), project.ID) {
			e.T.Errorf("the membership listing searched for %q holds %v, want project %d", project.Name, projectIDsOf(listed.Projects), project.ID)
		}
		owned := harness.Do[projects.ListOutput](s, actionProjectListUserProjects, map[string]any{"user_id": user.Username, "search": project.Name})
		if !containsID(projectIDsOf(owned.Projects), project.ID) {
			e.T.Errorf("the projects of %s searched for %q hold %v, want project %d", user.Username, project.Name, projectIDsOf(owned.Projects), project.ID)
		}

		users := harness.Do[projects.ListProjectUsersOutput](s, actionProjectListUsers, params)
		if !containsID(projectUserIDs(users.Users), user.UserID) {
			e.T.Errorf("the project's users %v do not hold its creator %d", projectUserIDs(users.Users), user.UserID)
		}
		groups := harness.Do[projects.ListProjectGroupsOutput](s, actionProjectListGroups, params)
		if len(groups.Groups) != 0 {
			e.T.Errorf("a personal project lists the groups %v, want none", projectGroupIDs(groups.Groups))
		}
		starrers := harness.Do[projects.ListProjectStarrersOutput](s, actionProjectListStarrers, params)
		if len(starrers.Starrers) != 0 {
			e.T.Errorf("a fresh project lists %d starrer(s), want none", len(starrers.Starrers))
		}
		forks := harness.Do[projects.ListForksOutput](s, actionProjectListForks, params)
		if len(forks.Forks) != 0 || len(forks.SimpleForks) != 0 {
			e.T.Errorf("a fresh project lists forks %v, want none", projectIDsOf(forks.Forks))
		}

		assertLanguagesAddUp(e, s, params)
		assertFetchStatisticsAgree(e, s, params)

		storage := harness.Do[projects.RepositoryStorageOutput](s, actionProjectRepositoryStorageGet, params)
		if storage.ProjectID != project.ID || storage.RepositoryStorage == "" {
			e.T.Errorf("repository_storage_get answered %+v, want project %d on a named shard", storage, project.ID)
		}
	})
}

// assertLanguagesAddUp reads the project's languages and holds them to what
// is true of any listing the detector can give.
//
// Whether a README-only repository names a language at all is the
// detector's business and differs by GitLab version, so the shares are the
// subject rather than the names: each is a real percentage, and the shares
// of a non-empty listing account for the whole repository.
func assertLanguagesAddUp(e *harness.Env, s *harness.Session, params map[string]any) {
	e.T.Helper()
	languages := harness.Do[projects.LanguagesOutput](s, actionProjectLanguages, params)
	total := float32(0)
	for _, language := range languages.Languages {
		if language.Name == "" || language.Percentage <= 0 || language.Percentage > 100 {
			e.T.Errorf("the project's languages hold %+v, want a named language with a share in (0, 100]", language)
		}
		total += language.Percentage
	}
	if len(languages.Languages) != 0 && (total < 99 || total > 101) {
		e.T.Errorf("the project's languages %+v add up to %.2f%%, want the whole repository", languages.Languages, total)
	}
}

// assertFetchStatisticsAgree reads the last thirty days of fetches and holds
// the per-day counts to the total beside them.
//
// Nothing has cloned a project the fixtures built over the API, so the total
// is zero here; the assertion is the invariant rather than the zero, since
// one clone by anything else would not make the answer wrong.
func assertFetchStatisticsAgree(e *harness.Env, s *harness.Session, params map[string]any) {
	e.T.Helper()
	statistics := harness.Do[projectstatistics.GetOutput](s, actionProjectStatisticsGet, params)
	counted := int64(0)
	for _, day := range statistics.Days {
		if day.Date == "" || day.Count < 0 {
			e.T.Errorf("statistics_get answered the day %+v, want a dated count", day)
		}
		counted += day.Count
	}
	if counted != statistics.TotalFetches {
		e.T.Errorf("statistics_get answered %d total fetches over days adding up to %d, want the two to agree: %+v",
			statistics.TotalFetches, counted, statistics.Days)
	}
}

// TestProjectState_StarAndArchive_RoundTrip stars, unstars, archives and
// unarchives a project of each surface's own, reading each flag off the
// answer, and starts its housekeeping.
//
// Replaces: TestMeta_ProjectCore
func TestProjectState_StarAndArchive_RoundTrip(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("projstate"))
		params := map[string]any{"project_id": project.IDParam()}

		starred := harness.Do[projects.Output](s, actionProjectStar, params)
		if starred.ID != project.ID || starred.StarCount != 1 {
			e.T.Errorf("star answered project %d with %d star(s), want %d with one", starred.ID, starred.StarCount, project.ID)
		}
		unstarred := harness.Do[projects.Output](s, actionProjectUnstar, params)
		if unstarred.ID != project.ID || unstarred.StarCount != 0 {
			e.T.Errorf("unstar answered project %d with %d star(s), want %d with none", unstarred.ID, unstarred.StarCount, project.ID)
		}

		archived := harness.Do[projects.Output](s, actionProjectArchive, params)
		if archived.ID != project.ID || !archived.Archived {
			e.T.Errorf("archive answered %+v, want project %d archived", archived, project.ID)
		}
		unarchived := harness.Do[projects.Output](s, actionProjectUnarchive, params)
		if unarchived.ID != project.ID || unarchived.Archived {
			e.T.Errorf("unarchive answered %+v, want project %d active again", unarchived, project.ID)
		}

		housekeeping := harness.Do[toolutil.VoidOutput](s, actionProjectStartHousekeeping, params)
		if housekeeping.Status != voidStatusSuccess {
			e.T.Errorf("start_housekeeping answered %+v, want a %s status", housekeeping, voidStatusSuccess)
		}
	})
}

// TestProjectFork_CreateAndList_NamesTheUpstream forks one shared project
// under a name of each surface's own, reads the upstream off the answer,
// and finds the fork in the upstream's fork listing.
//
// Replaces: TestMeta_ProjectCore
func TestProjectFork_CreateAndList_NamesTheUpstream(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("upstream"))
	}, func(e *harness.Env, surface harness.Surface, upstream fixture.Project) {
		s := e.On(surface)
		name := e.Name("fork")

		fork := harness.Do[projects.Output](s, actionProjectFork, map[string]any{"project_id": upstream.IDParam(), "name": name, "path": name})
		if fork.ID == 0 || fork.Name != name {
			e.T.Fatalf("fork answered %+v, want a project named %q with an ID", fork, name)
		}
		e.Defer("fork "+fork.PathWithNamespace, func(ctx context.Context) error {
			return fixture.DeleteProject(ctx, e.Client(), fork.ID, fork.PathWithNamespace)
		})
		if fork.ForkedFromProject == nil || fork.ForkedFromProject.ID != upstream.ID {
			e.T.Errorf("the fork names %+v as its upstream, want project %d", fork.ForkedFromProject, upstream.ID)
		}

		forks := harness.Do[projects.ListForksOutput](s, actionProjectListForks, map[string]any{"project_id": upstream.IDParam()})
		if !containsID(projectIDsOf(forks.Forks), fork.ID) {
			e.T.Errorf("the upstream lists the forks %v, want the fork %d among them", projectIDsOf(forks.Forks), fork.ID)
		}
	})
}
