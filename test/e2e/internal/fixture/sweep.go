//go:build e2e

// sweep.go deletes what a run left behind, and only what this run left
// behind.
//
// Every name a builder hands out carries the run ID, so a sweep can recognize
// its own leftovers on an instance three packages share and other people use.
// The suite this replaces matched a prefix alone, which on a shared instance
// deleted another package's live projects.
//
// Two sweeps exist for a person to run on purpose, and nothing runs either by
// itself. The default one takes what a run left once that run is certainly
// over: it knows no run ID, so it reads the one each name carries and the
// second that run started, and leaves every run younger than the floor it is
// given. The prefix one is the explicit override, for whatever the default
// cannot date.

package fixture

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The sweep's bounds.
const (
	sweepBudget   = 5 * time.Minute
	sweepPageSize = 100
)

// SweepReport says what a sweep found and what it could not remove.
type SweepReport struct {
	// Projects, Groups, Snippets and Users name what was deleted: a project
	// or a group by its path, a personal snippet by its title and a user by
	// the username.
	Projects []string
	Groups   []string
	Snippets []string
	Users    []string
	// Errors holds every deletion that failed and every listing that could
	// not be read.
	Errors []error
}

// Found reports whether the sweep had anything to delete.
func (r SweepReport) Found() bool {
	return len(r.Removed()) > 0
}

// Removed names every object the sweep deleted, each after its kind, in the
// order the sweep deleted them. It is the one list both the exit hook and the
// on-demand sweep log, so neither can leave a kind out of its log.
func (r SweepReport) Removed() []string {
	var removed []string
	for _, kind := range []struct {
		name  string
		names []string
	}{
		{name: "project", names: r.Projects},
		{name: "group", names: r.Groups},
		{name: "snippet", names: r.Snippets},
		{name: "user", names: r.Users},
	} {
		for _, name := range kind.names {
			removed = append(removed, kind.name+" "+name)
		}
	}
	return removed
}

// Err folds the failures into one error, nil when there were none.
func (r SweepReport) Err() error {
	return errors.Join(r.Errors...)
}

// String summarizes the report for a log line.
func (r SweepReport) String() string {
	return fmt.Sprintf("%d project(s), %d group(s), %d snippet(s), %d user(s) removed; %d error(s)",
		len(r.Projects), len(r.Groups), len(r.Snippets), len(r.Users), len(r.Errors))
}

// matcher decides whether an object the listing returned is one to delete.
type matcher func(name, path string) bool

// belongsToRun reports whether an object's name or path carries runID, which
// every name a builder hands out does.
func belongsToRun(name, path, runID string) bool {
	if runID == "" {
		return false
	}
	return strings.Contains(name, runID) || strings.Contains(path, runID)
}

// hasPrefix reports whether an object's name, or the last segment of its
// path, opens with prefix.
func hasPrefix(name, path, prefix string) bool {
	if prefix == "" {
		return false
	}
	return strings.HasPrefix(name, prefix) || strings.HasPrefix(lastSegment(path), prefix)
}

// lastSegment returns the part of a path after its last slash.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// startedBefore reports whether an object's name or path carries a run
// identifier the harness minted for a run that started before cutoff.
func startedBefore(name, path string, cutoff time.Time) bool {
	return mintedBefore(name, cutoff) || mintedBefore(path, cutoff)
}

// mintedBefore reports whether s carries a minted run identifier whose stamp
// is earlier than cutoff.
func mintedBefore(s string, cutoff time.Time) bool {
	started, found := harness.MintedRunStart(s)
	return found && started.Before(cutoff)
}

// SweepRun permanently deletes every project, group and personal snippet,
// and with an administrator's token every user, whose name carries runID. It
// is what the exit hook runs; a person runs SweepRunsStartedBefore or
// SweepPrefix instead.
func SweepRun(ctx context.Context, client *gitlabclient.Client, runID string, admin bool) SweepReport {
	if runID == "" {
		return SweepReport{Errors: []error{errors.New("a sweep needs a run ID to scope itself to")}}
	}
	return sweep(ctx, client, runID, func(name, path string) bool { return belongsToRun(name, path, runID) }, admin)
}

// SweepRunsStartedBefore permanently deletes every owned project and group,
// every personal snippet, and with an administrator's token every user,
// whose name or path carries a run identifier the harness minted for a run
// that started before cutoff, whatever run that was. It is the on-demand
// sweep's default, for the leftovers of runs that were killed and so never
// reached their own exit sweep.
//
// The cutoff is what keeps it off a run still going: a run lives no longer
// than the test timeout it was given, so a cutoff further back than the
// longest one reaches only runs that have ended. A run whose identifier
// E2E_RUN_ID replaced carries no stamp and is never reached here.
//
// No listing can be searched for a shape, so all four are read whole: every
// project and group the token owns, every snippet of its user, and with an
// administrator's token every user on the instance.
func SweepRunsStartedBefore(ctx context.Context, client *gitlabclient.Client, cutoff time.Time, admin bool) SweepReport {
	return sweep(ctx, client, "", func(name, path string) bool { return startedBefore(name, path, cutoff) }, admin)
}

// SweepPrefix permanently deletes every owned project and group, every
// personal snippet, and with an administrator's token every user, whose name
// opens with prefix, whatever run made it and however recently. It is the
// explicit override of the on-demand sweep, never a run's own exit.
//
// The one prefix is applied to all four kinds, so a prefix meant for one kind
// reaches the others too, and it reaches the token user's own objects as
// readily as the suite's.
func SweepPrefix(ctx context.Context, client *gitlabclient.Client, prefix string, admin bool) SweepReport {
	if prefix == "" {
		return SweepReport{Errors: []error{errors.New("a prefix sweep needs a prefix, or it would delete everything the token owns")}}
	}
	return sweep(ctx, client, prefix, func(name, path string) bool { return hasPrefix(name, path, prefix) }, admin)
}

// sweep lists what search finds and deletes what match accepts: projects
// first, since the ones under a group go with it either way and a project
// that fails to delete then says so on its own line; then groups; then the
// token user's personal snippets, which no deletion of a project or a group
// takes along; then users, which only an administrator can list or delete.
// An empty search lists every object of a kind.
func sweep(ctx context.Context, client *gitlabclient.Client, search string, match matcher, admin bool) SweepReport {
	var report SweepReport
	report.Projects, report.Errors = sweepKind(ctx, projectTarget(client, search), match, report.Errors)
	report.Groups, report.Errors = sweepKind(ctx, groupTarget(client, search), match, report.Errors)
	report.Snippets, report.Errors = sweepKind(ctx, snippetTarget(client), match, report.Errors)
	if admin {
		report.Users, report.Errors = sweepKind(ctx, userTarget(client, search), match, report.Errors)
	}
	return report
}

// searchParam is the search a listing sends, and none for an empty one:
// what GitLab makes of an empty search is its own to decide, while no search
// is a listing of everything by definition.
func searchParam(search string) *string {
	if search == "" {
		return nil
	}
	return new(search)
}

// sweepItem is one object a listing returned, as the matcher and the
// report see it. An object with no path, which is what a snippet is, is
// matched on its name alone and reported by it.
type sweepItem struct {
	id   int64
	name string
	path string
}

// sweepTarget is one kind of object a sweep lists page by page and deletes.
type sweepTarget struct {
	// kind names the objects in an error.
	kind string
	// list returns one page and the number of the next, zero at the end.
	list func(ctx context.Context, page int64) ([]sweepItem, int64, error)
	// remove deletes one object permanently.
	remove func(ctx context.Context, item sweepItem) error
}

// sweepKind walks every page of one target, then deletes what match accepted,
// and returns what it removed, by path or by name when there is no path,
// beside the errors, appended to those given.
//
// Every page is read before anything is deleted, because GitLab pages by
// offset: deleting the first page's matches before asking for the second
// shifts the list under it, and the first rows of what was the second page
// are then never read. That loses the most exactly when leftovers have piled
// up and the matches are dense, which is the case a sweep exists for. A page
// that cannot be read ends the listing, and what the earlier pages matched is
// still deleted.
func sweepKind(ctx context.Context, target sweepTarget, match matcher, errs []error) (removed []string, failures []error) {
	failures = errs
	var matched []sweepItem
	for page := int64(1); page != 0; {
		items, next, err := target.list(ctx, page)
		if err != nil {
			failures = append(failures, fmt.Errorf("listing %ss (page %d): %w", target.kind, page, err))
			break
		}
		for _, item := range items {
			if match(item.name, item.path) {
				matched = append(matched, item)
			}
		}
		page = next
	}
	for _, item := range matched {
		if removeErr := target.remove(ctx, item); removeErr != nil {
			failures = append(failures, removeErr)
			continue
		}
		removed = append(removed, cmp.Or(item.path, item.name))
	}
	return removed, failures
}

// projectTarget lists the owned projects search finds, pending deletion
// included since a project marked and never removed is the most common
// leftover, and removes them through DeleteProject.
func projectTarget(client *gitlabclient.Client, search string) sweepTarget {
	return sweepTarget{
		kind: "project",
		list: func(ctx context.Context, page int64) ([]sweepItem, int64, error) {
			opts := &gl.ListProjectsOptions{Owned: new(true), IncludePendingDelete: new(true), Search: searchParam(search)}
			opts.Page = page
			opts.PerPage = sweepPageSize
			projects, resp, err := client.GL().Projects.ListProjects(opts, gl.WithContext(ctx))
			if err != nil {
				return nil, 0, err
			}
			items := make([]sweepItem, 0, len(projects))
			for _, project := range projects {
				items = append(items, sweepItem{id: project.ID, name: project.Name, path: project.PathWithNamespace})
			}
			return items, resp.NextPage, nil
		},
		remove: func(ctx context.Context, item sweepItem) error {
			return DeleteProject(ctx, client, item.id, item.path)
		},
	}
}

// groupTarget lists the owned groups search finds and removes them through
// DeleteGroup. A subgroup goes with its parent, and deleting it first is
// harmless.
func groupTarget(client *gitlabclient.Client, search string) sweepTarget {
	return sweepTarget{
		kind: "group",
		list: func(ctx context.Context, page int64) ([]sweepItem, int64, error) {
			opts := &gl.ListGroupsOptions{Owned: new(true), Search: searchParam(search)}
			opts.Page = page
			opts.PerPage = sweepPageSize
			groups, resp, err := client.GL().Groups.ListGroups(opts, gl.WithContext(ctx))
			if err != nil {
				return nil, 0, err
			}
			items := make([]sweepItem, 0, len(groups))
			for _, group := range groups {
				items = append(items, sweepItem{id: group.ID, name: group.Name, path: group.FullPath})
			}
			return items, resp.NextPage, nil
		},
		remove: func(ctx context.Context, item sweepItem) error {
			return DeleteGroup(ctx, client, item.id, item.path)
		},
	}
}

// snippetTarget lists the token user's personal snippets and removes them
// through deletePersonalSnippet.
//
// The listing takes no search, since GitLab's snippet listing has none, so
// every page is read and the matcher does all the choosing. It also answers
// the project snippets the user wrote, and those are passed over: a project
// snippet goes with its project, which the project target has already dealt
// with. A snippet has no path, so the matcher judges its title alone, which
// keeps the path rule of the prefix sweep, the last segment after a slash,
// off a title that happens to hold one.
func snippetTarget(client *gitlabclient.Client) sweepTarget {
	return sweepTarget{
		kind: "snippet",
		list: func(ctx context.Context, page int64) ([]sweepItem, int64, error) {
			opts := &gl.ListSnippetsOptions{}
			opts.Page = page
			opts.PerPage = sweepPageSize
			snippets, resp, err := client.GL().Snippets.ListSnippets(opts, gl.WithContext(ctx))
			if err != nil {
				return nil, 0, err
			}
			items := make([]sweepItem, 0, len(snippets))
			for _, snippet := range snippets {
				if snippet.ProjectID != 0 {
					continue
				}
				items = append(items, sweepItem{id: snippet.ID, name: snippet.Title})
			}
			return items, resp.NextPage, nil
		},
		remove: func(ctx context.Context, item sweepItem) error {
			return deletePersonalSnippet(ctx, client, item.id, item.name)
		},
	}
}

// userTarget lists the users search finds, matching on the username since a
// user has no path, and hard-deletes them.
func userTarget(client *gitlabclient.Client, search string) sweepTarget {
	return sweepTarget{
		kind: "user",
		list: func(ctx context.Context, page int64) ([]sweepItem, int64, error) {
			opts := &gl.ListUsersOptions{Search: searchParam(search)}
			opts.Page = page
			opts.PerPage = sweepPageSize
			users, resp, err := client.GL().Users.ListUsers(opts, gl.WithContext(ctx))
			if err != nil {
				return nil, 0, err
			}
			items := make([]sweepItem, 0, len(users))
			for _, user := range users {
				items = append(items, sweepItem{id: user.ID, name: user.Username, path: user.Username})
			}
			return items, resp.NextPage, nil
		},
		remove: func(ctx context.Context, item sweepItem) error {
			_, err := client.GL().Users.DeleteUser(item.id, gl.WithContext(ctx))
			if err != nil && !IsStatus(err, http.StatusNotFound) {
				return fmt.Errorf("deleting user %d (%s): %w", item.id, item.name, err)
			}
			return nil
		},
	}
}

// The exit sweep is armed once per process, by the first builder that creates
// something outliving a request. The hook is registered through a variable
// rather than a direct call so a test can arm the sweep afresh and see what
// was registered: in a test process no hook ever runs, since only harness.Main
// runs them, so which builders arm the sweep is otherwise invisible.
var (
	sweepOnce        = new(sync.Once)
	registerExitHook = harness.AtExit
)

// armSweep registers the run-scoped sweep as an exit hook, once per process.
func armSweep(e *harness.Env) {
	sweepOnce.Do(func() {
		registerExitHook(exitSweep(e.Client(), e.RunID(), e.Runtime().Admin))
	})
}

// exitSweep is the hook armSweep registers: SweepRun over the run's own
// identifier, under the sweep's budget.
//
// It runs after every test's own cleanup, so what it finds is what a cleanup
// failed to remove; that is logged as a leak, and the run is failed only when
// the sweep could not remove it either, since a leak the sweep removed is a
// cleanup defect the log now names and not a resource left on the instance.
func exitSweep(client *gitlabclient.Client, runID string, admin bool) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), sweepBudget)
		defer cancel()

		report := SweepRun(ctx, client, runID, admin)
		if report.Found() {
			log.Printf("e2e: sweep for run %s removed leftovers a cleanup did not: %s", runID, report)
			for _, removed := range report.Removed() {
				log.Printf("e2e: sweep removed %s", removed)
			}
		}
		if err := report.Err(); err != nil {
			return fmt.Errorf("sweep for run %s: %w", runID, err)
		}
		return nil
	}
}

// The settings that choose the on-demand sweep, beside the instance and its
// credential, which the test that runs it reads.
const (
	// sweepMinAgeEnv turns the default on-demand sweep on, and says how long
	// ago a run must have started for it to take what that run left.
	sweepMinAgeEnv = "E2E_SWEEP_MIN_AGE"
	// sweepPrefixEnv is the explicit override: sweep by name prefix instead,
	// whatever run made an object and however recently.
	sweepPrefixEnv = "E2E_SWEEP_PREFIX"
)

// orphanScope is what the on-demand sweep deletes: every object whose name
// opens with prefix when one is set, and otherwise every object named after a
// run that started before cutoff.
type orphanScope struct {
	prefix string
	cutoff time.Time
	// minAge is what cutoff was computed from, kept for the log line.
	minAge time.Duration
}

// resolveOrphanScope reads which on-demand sweep a person asked for, and
// reports false when they asked for none: neither setting is set, which is
// what a bare go test of this package looks like, and that must delete
// nothing.
//
// A prefix wins over an age, since it is the override. An age that is not a
// duration, or is negative, is refused rather than read as none: a sweep
// silently skipped for a typo reads exactly like one that found nothing, and
// a negative floor would reach every run including those still going.
func resolveOrphanScope(getenv func(string) string, now time.Time) (orphanScope, bool, error) {
	if prefix := strings.TrimSpace(getenv(sweepPrefixEnv)); prefix != "" {
		return orphanScope{prefix: prefix}, true, nil
	}
	raw := strings.TrimSpace(getenv(sweepMinAgeEnv))
	if raw == "" {
		return orphanScope{}, false, nil
	}
	minAge, err := time.ParseDuration(raw)
	if err != nil {
		return orphanScope{}, true, fmt.Errorf("%s=%q is not a duration such as 2h: %w", sweepMinAgeEnv, raw, err)
	}
	if minAge < 0 {
		return orphanScope{}, true, fmt.Errorf("%s=%q is negative, which would reach runs still going", sweepMinAgeEnv, raw)
	}
	return orphanScope{cutoff: now.Add(-minAge), minAge: minAge}, true, nil
}

// run runs the sweep the scope names.
func (s orphanScope) run(ctx context.Context, client *gitlabclient.Client, admin bool) SweepReport {
	if s.prefix != "" {
		return SweepPrefix(ctx, client, s.prefix, admin)
	}
	return SweepRunsStartedBefore(ctx, client, s.cutoff, admin)
}

// String says what the scope reaches, for the log line of the sweep.
func (s orphanScope) String() string {
	if s.prefix != "" {
		return fmt.Sprintf("names opening with %q", s.prefix)
	}
	return fmt.Sprintf("names of runs started before %s (%s ago)", s.cutoff.UTC().Format(time.RFC3339), s.minAge)
}
