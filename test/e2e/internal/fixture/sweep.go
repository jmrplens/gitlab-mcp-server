//go:build e2e

// sweep.go deletes what a run left behind, and only what this run left
// behind.
//
// Every name a builder hands out carries the run ID, so a sweep can recognize
// its own leftovers on an instance three packages share and other people use.
// The suite this replaces matched a prefix alone, which on a shared instance
// deleted another package's live projects; the prefix-wide sweep still exists
// for an explicit target a person runs on purpose, and nothing runs it by
// itself.

package fixture

import (
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
	// Projects, Groups and Users name what was deleted.
	Projects []string
	Groups   []string
	Users    []string
	// Errors holds every deletion that failed and every listing that could
	// not be read.
	Errors []error
}

// Found reports whether the sweep had anything to delete.
func (r SweepReport) Found() bool {
	return len(r.Projects)+len(r.Groups)+len(r.Users) > 0
}

// Err folds the failures into one error, nil when there were none.
func (r SweepReport) Err() error {
	return errors.Join(r.Errors...)
}

// String summarizes the report for a log line.
func (r SweepReport) String() string {
	return fmt.Sprintf("%d project(s), %d group(s), %d user(s) removed; %d error(s)",
		len(r.Projects), len(r.Groups), len(r.Users), len(r.Errors))
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

// SweepRun permanently deletes every project, group and, with an
// administrator's token, every user whose name carries runID. It is what the
// exit hook runs; a person runs SweepPrefix instead.
func SweepRun(ctx context.Context, client *gitlabclient.Client, runID string, admin bool) SweepReport {
	if runID == "" {
		return SweepReport{Errors: []error{errors.New("a sweep needs a run ID to scope itself to")}}
	}
	return sweep(ctx, client, runID, func(name, path string) bool { return belongsToRun(name, path, runID) }, admin)
}

// SweepPrefix permanently deletes every owned project and group, and with an
// administrator's token every user, whose name opens with prefix, whatever
// run made it. It is for an explicit target run by hand after a run that
// could not clean up, never for a run's own exit.
func SweepPrefix(ctx context.Context, client *gitlabclient.Client, prefix string, admin bool) SweepReport {
	if prefix == "" {
		return SweepReport{Errors: []error{errors.New("a prefix sweep needs a prefix, or it would delete everything the token owns")}}
	}
	return sweep(ctx, client, prefix, func(name, path string) bool { return hasPrefix(name, path, prefix) }, admin)
}

// sweep lists what search finds and deletes what match accepts: projects
// first, since the ones under a group go with it either way and a project
// that fails to delete then says so on its own line; then groups; then
// users, which only an administrator can list or delete.
func sweep(ctx context.Context, client *gitlabclient.Client, search string, match matcher, admin bool) SweepReport {
	var report SweepReport
	report.Projects, report.Errors = sweepKind(ctx, projectTarget(client, search), match, report.Errors)
	report.Groups, report.Errors = sweepKind(ctx, groupTarget(client, search), match, report.Errors)
	if admin {
		report.Users, report.Errors = sweepKind(ctx, userTarget(client, search), match, report.Errors)
	}
	return report
}

// sweepItem is one object a listing returned, as the matcher and the
// report see it.
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

// sweepKind walks every page of one target, deletes what match accepts, and
// returns the paths removed beside the errors, appended to those given.
func sweepKind(ctx context.Context, target sweepTarget, match matcher, errs []error) (removed []string, failures []error) {
	failures = errs
	var page int64 = 1
	for page != 0 {
		items, next, err := target.list(ctx, page)
		if err != nil {
			return removed, append(failures, fmt.Errorf("listing %ss (page %d): %w", target.kind, page, err))
		}
		for _, item := range items {
			if !match(item.name, item.path) {
				continue
			}
			if removeErr := target.remove(ctx, item); removeErr != nil {
				failures = append(failures, removeErr)
				continue
			}
			removed = append(removed, item.path)
		}
		page = next
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
			opts := &gl.ListProjectsOptions{Owned: new(true), IncludePendingDelete: new(true), Search: new(search)}
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
			opts := &gl.ListGroupsOptions{Owned: new(true), Search: new(search)}
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

// userTarget lists the users search finds, matching on the username since a
// user has no path, and hard-deletes them.
func userTarget(client *gitlabclient.Client, search string) sweepTarget {
	return sweepTarget{
		kind: "user",
		list: func(ctx context.Context, page int64) ([]sweepItem, int64, error) {
			opts := &gl.ListUsersOptions{Search: new(search)}
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

// sweepOnce arms the run's exit sweep the first time a builder creates
// something that outlives a request.
var sweepOnce sync.Once

// armSweep registers the run-scoped sweep as an exit hook, once per process.
//
// It runs after every test's own cleanup, so what it finds is what a cleanup
// failed to remove; that is logged as a leak, and the run is failed only when
// the sweep could not remove it either, since a leak the sweep removed is a
// cleanup defect the log now names and not a resource left on the instance.
func armSweep(e *harness.Env) {
	sweepOnce.Do(func() {
		client, runID, admin := e.Client(), e.RunID(), e.Runtime().Admin
		harness.AtExit(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), sweepBudget)
			defer cancel()

			report := SweepRun(ctx, client, runID, admin)
			if report.Found() {
				log.Printf("e2e: sweep for run %s removed leftovers a cleanup did not: %s", runID, report)
				for _, path := range report.Projects {
					log.Printf("e2e: sweep removed project %s", path)
				}
				for _, path := range report.Groups {
					log.Printf("e2e: sweep removed group %s", path)
				}
				for _, username := range report.Users {
					log.Printf("e2e: sweep removed user %s", username)
				}
			}
			if err := report.Err(); err != nil {
				return fmt.Errorf("sweep for run %s: %w", runID, err)
			}
			return nil
		})
	})
}
