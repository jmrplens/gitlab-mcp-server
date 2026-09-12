//go:build e2e

// project.go builds the project most tests start from, and knows how to make
// one disappear for good.
//
// Deleting is the harder half. GitLab keeps a deleted project around for a
// retention period and renames its path while it waits, so a test that
// deleted its project and a sweep that found it later have to do the same
// two-step dance: mark, re-read the path GitLab gave it, then remove it
// permanently under that path. The dance lives here once and every deletion
// in the package goes through it.

package fixture

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// DefaultBranch is the branch every project this package creates is
// initialized with. It is spelled once so that a test naming a ref and a
// builder creating one agree.
const DefaultBranch = "main"

// Project is a project a builder created, as a test refers to it.
type Project struct {
	// ID is the numeric identifier every project-scoped action takes.
	ID int64
	// Path is the full path with its namespace, such as e2e-user/name.
	Path string
	// Name is the project's display name.
	Name string
	// DefaultBranch is the branch the repository was initialized with.
	DefaultBranch string
	// HTTPURLToRepo is the clone URL GitLab published for it, which is what
	// a mirror needs when no internal address is configured.
	HTTPURLToRepo string
	// NamespaceID is the namespace it was created in: the group's when a
	// group was named, the user's otherwise.
	NamespaceID int64
}

// IDParam spells the project's ID as the project_id parameter takes it.
//
// Every action's project_id is declared a string, because the parameter also
// accepts a path. The two dispatcher surfaces coerce a number into that
// string before they validate; the individual surface validates what it was
// sent against the schema and refuses a number. A test that sends the same
// call to all three therefore spells the ID the way the schema declares it.
func (p Project) IDParam() string {
	return strconv.FormatInt(p.ID, 10)
}

// projectSpec is what a builder asks GitLab for.
type projectSpec struct {
	name        string
	description string
	visibility  gl.VisibilityValue
	readme      bool
	namespaceID int64
}

// ProjectOption adjusts one project builder.
type ProjectOption func(*projectSpec)

// InGroup creates the project under the given group rather than under the
// authenticated user's own namespace. Iterations, epics and group-level
// policies need a project with a group parent.
func InGroup(group Group) ProjectOption {
	return func(spec *projectSpec) { spec.namespaceID = group.ID }
}

// WithVisibility sets the project's visibility. Private is the default,
// because a public project on a shared instance is visible to everyone else
// running tests there.
func WithVisibility(visibility gl.VisibilityValue) ProjectOption {
	return func(spec *projectSpec) { spec.visibility = visibility }
}

// WithoutReadme creates an empty repository with no default branch. A test of
// the very first commit, or of an import into an empty project, needs one.
func WithoutReadme() ProjectOption {
	return func(spec *projectSpec) { spec.readme = false }
}

// WithNamePrefix changes the prefix the project's generated name opens with.
// The run ID and the test's own name follow it either way.
func WithNamePrefix(prefix string) ProjectOption {
	return func(spec *projectSpec) { spec.name = prefix }
}

// NewProject creates a private project initialized with a README on
// DefaultBranch, waits until that branch is readable, and registers its
// permanent deletion on the Env.
//
// The name carries the run ID and the test's name, so a sweep can tell whose
// it is; the creation is retried on the failures CreateRetryable names.
func NewProject(e *harness.Env, opts ...ProjectOption) Project {
	e.T.Helper()
	armSweep(e)

	spec := projectSpec{name: "proj", description: "e2e: " + e.T.Name(), visibility: gl.PrivateVisibility, readme: true}
	for _, opt := range opts {
		opt(&spec)
	}

	project, err := createProject(e, spec, func() string { return e.Name(spec.name) })
	if err != nil {
		e.T.Fatalf("creating the project fixture: %v", err)
	}
	e.Defer("project "+project.Path, func(ctx context.Context) error {
		return DeleteProject(ctx, e.Client(), project.ID, project.Path)
	})

	if spec.readme {
		waitForDefaultBranch(e, project)
	}
	return project
}

// createProject asks GitLab for a project under a fresh name on every
// attempt, since a name refused as taken must not be offered again.
func createProject(e *harness.Env, spec projectSpec, nextName func() string) (Project, error) {
	e.T.Helper()

	enterprise := e.Runtime().Tier.IsEnterprise()
	return retryWhen(e, "create project", createRetries,
		func(err error) bool { return CreateRetryable(err, enterprise) },
		func() (Project, error) {
			name := nextName()
			opts := &gl.CreateProjectOptions{
				Name:                 new(name),
				Description:          new(spec.description),
				Visibility:           new(spec.visibility),
				InitializeWithReadme: new(spec.readme),
			}
			if spec.readme {
				opts.DefaultBranch = new(DefaultBranch)
			}
			if spec.namespaceID != 0 {
				opts.NamespaceID = new(spec.namespaceID)
			}
			created, _, err := e.Client().GL().Projects.CreateProject(opts, gl.WithContext(e.Ctx))
			if err != nil {
				return Project{}, err
			}
			return projectOf(created), nil
		})
}

// projectOf reads what a test needs out of what GitLab returned.
func projectOf(p *gl.Project) Project {
	project := Project{
		ID:            p.ID,
		Path:          p.PathWithNamespace,
		Name:          p.Name,
		DefaultBranch: p.DefaultBranch,
		HTTPURLToRepo: p.HTTPURLToRepo,
	}
	if p.Namespace != nil {
		project.NamespaceID = p.Namespace.ID
	}
	return project
}

// waitForDefaultBranch holds the test until the project's default branch is
// readable, failing it when the branch never appears.
//
// GitLab initializes the README in a background job, so the project exists
// before its branch does. Under parallel load (around sixty projects at once)
// that job can take well over thirty seconds, so the budget is long and the
// Sidekiq queue is drained first, since the branch cannot appear before the
// job that writes it has run.
func waitForDefaultBranch(e *harness.Env, project Project) {
	e.T.Helper()

	DrainSidekiq(e.Ctx, e.Client())
	if err := waitForBranch(e.Ctx, e.Client(), project.ID, project.DefaultBranch, budget(e, branchWait, enterpriseBranchWait)); err != nil {
		e.T.Fatalf("waiting for branch %q of project %d: %v", project.DefaultBranch, project.ID, err)
	}
}

// The waits a branch may take to become readable.
const (
	branchWait           = 90 * time.Second
	enterpriseBranchWait = 240 * time.Second
	branchPollInterval   = 500 * time.Millisecond
)

// waitForBranch polls until the branch is readable or the budget is spent. A
// 404, a 429 and every 5xx keep it polling, because each is what GitLab
// answers while it converges; any other answer is a failure of its own.
func waitForBranch(ctx context.Context, client *gitlabclient.Client, projectID int64, branch string, wait time.Duration) error {
	pollCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	return harness.Poll(pollCtx, branchPollInterval, wait, func() (bool, string, error) {
		_, resp, err := client.GL().Branches.GetBranch(projectID, branch, gl.WithContext(pollCtx))
		if err == nil {
			return true, fmt.Sprintf("branch %q readable in project %d", branch, projectID), nil
		}
		state := fmt.Sprintf("branch %q in project %d: %v", branch, projectID, err)
		if resp != nil {
			state = fmt.Sprintf("branch %q in project %d: HTTP %d", branch, projectID, resp.StatusCode)
		}
		if resp == nil && !IsTransientNetwork(err) {
			return false, state, fmt.Errorf("reading branch %q of project %d: %w", branch, projectID, err)
		}
		if resp != nil && !convergingStatus(resp.StatusCode) {
			return false, state, fmt.Errorf("reading branch %q of project %d: %w", branch, projectID, err)
		}
		return false, state, nil
	})
}

// convergingStatus reports whether an HTTP status is one GitLab answers while
// an object it has just been told about is still being written.
func convergingStatus(status int) bool {
	return status == http.StatusNotFound || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// DeleteProject removes a project permanently, whatever state it is in.
//
// It is two steps because GitLab CE with delayed deletion refuses a permanent
// removal of a project that is not yet marked for deletion, and renames the
// path of one that is (appending -deletion_scheduled-<ID>), so the second
// request has to name the path the first one produced. A project already
// marked, or already gone, is not an error: the point is that it ends up
// gone.
//
//nolint:dupl // DeleteGroup is this step for step on a service of its own, and the shared part is deletePermanently
func DeleteProject(ctx context.Context, client *gitlabclient.Client, projectID int64, path string) error {
	projects := client.GL().Projects
	return deletePermanently(ctx, "project", projectID, path, deletionSteps{
		mark: func(ctx context.Context) error {
			_, err := projects.DeleteProject(projectID, nil, gl.WithContext(ctx))
			return err
		},
		currentPath: func(ctx context.Context) (string, error) {
			current, _, err := projects.GetProject(projectID, nil, gl.WithContext(ctx))
			if err != nil {
				return "", err
			}
			return current.PathWithNamespace, nil
		},
		remove: func(ctx context.Context, path string) error {
			_, err := projects.DeleteProject(projectID, &gl.DeleteProjectOptions{
				PermanentlyRemove: new(true),
				FullPath:          new(path),
			}, gl.WithContext(ctx))
			return err
		},
	})
}

// deletionSteps are the three requests a permanent deletion is made of, for
// the one function that orders them.
type deletionSteps struct {
	// mark is the plain DELETE, which marks the object on an instance with
	// delayed deletion and removes it on one without.
	mark func(ctx context.Context) error
	// currentPath re-reads the object, whose path GitLab renamed when it
	// marked it.
	currentPath func(ctx context.Context) (string, error)
	// remove is the permanent DELETE under the path currentPath found.
	remove func(ctx context.Context, path string) error
}

// deletePermanently is the two-step dance both projects and groups go
// through, written once: mark, tolerating an object already marked; re-read
// the path; remove permanently under it. An object that is gone at any step
// is a success, because gone is what was asked for.
func deletePermanently(ctx context.Context, kind string, id int64, path string, steps deletionSteps) error {
	ctx, cancel := withCleanupTimeout(ctx)
	defer cancel()

	if err := steps.mark(ctx); err != nil {
		if IsStatus(err, http.StatusNotFound) {
			return nil
		}
		if !toolutil.ContainsAny(err, "already being deleted", "marked for deletion") {
			return fmt.Errorf("marking %s %d (%s) for deletion: %w", kind, id, path, err)
		}
	}

	current, err := steps.currentPath(ctx)
	switch {
	case err == nil:
		path = current
	case IsStatus(err, http.StatusNotFound):
		return nil
	}

	if removeErr := steps.remove(ctx, path); removeErr != nil && !IsStatus(removeErr, http.StatusNotFound) {
		if IsStatus(removeErr, http.StatusBadRequest) && toolutil.ContainsAny(removeErr, "only available for subgroups") {
			// A top-level group on an instance with delayed deletion cannot be
			// permanently removed through the API, only marked; the mark step
			// above already scheduled it, which is as gone as the API allows
			// and is what the teardown asked for. A subgroup takes the
			// permanent removal, so this tolerance never hides a real failure
			// there. GitLab CE 18 with delayed deletion is where this bites,
			// on the World's own top-level group. The status is part of the
			// match: GitLab refuses this with 400, and the same words under
			// another status would be an answer this code has not seen.
			return nil
		}
		return fmt.Errorf("permanently deleting %s %d (%s): %w", kind, id, path, removeErr)
	}
	return nil
}

// UnprotectBranch removes the protection rule from a branch and waits until
// it is really gone, so the commits a test pushes next are not refused.
//
// The one-shot form of this was racy: under Docker load the unprotect call
// returned without error while the rule was still observable afterwards,
// because the change propagates through a background job, and the next
// commit failed with 403 "You are not allowed to push into this branch". So
// the call is made (it is idempotent), the Sidekiq queue is drained, and the
// rule is polled until GitLab answers 404 for it. A rule that never goes away
// is logged rather than failed, so the test's own assertion produces the
// message that names what it was trying to do.
func UnprotectBranch(e *harness.Env, project Project, branch string) {
	e.T.Helper()

	client := e.Client()
	if _, err := client.GL().ProtectedBranches.UnprotectRepositoryBranches(project.ID, branch, gl.WithContext(e.Ctx)); err != nil {
		e.T.Logf("unprotecting %q (idempotent, may already be unprotected): %v", branch, err)
	}
	DrainSidekiq(e.Ctx, client)

	wait := budget(e, unprotectWait, enterpriseUnprotectWait)
	pollCtx, cancel := context.WithTimeout(e.Ctx, wait)
	defer cancel()

	err := harness.Poll(pollCtx, branchPollInterval, wait, func() (bool, string, error) {
		_, resp, getErr := client.GL().ProtectedBranches.GetProtectedBranch(project.ID, branch, gl.WithContext(pollCtx))
		if getErr == nil {
			return false, fmt.Sprintf("branch %q still protected in project %d", branch, project.ID), nil
		}
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return true, fmt.Sprintf("branch %q unprotected in project %d", branch, project.ID), nil
		}
		if resp == nil || resp.StatusCode >= http.StatusInternalServerError {
			return false, fmt.Sprintf("reading the protection of %q in project %d: %v", branch, project.ID, getErr), nil
		}
		return false, fmt.Sprintf("reading the protection of %q in project %d: HTTP %d", branch, project.ID, resp.StatusCode),
			fmt.Errorf("reading the protection of %q: %w", branch, getErr)
	})
	if err != nil {
		e.T.Logf("unprotecting %q did not converge within %s (not fatal): %v", branch, wait, err)
	}
}

// The waits a branch protection rule may take to disappear.
const (
	unprotectWait           = 30 * time.Second
	enterpriseUnprotectWait = 60 * time.Second
)

// The Sidekiq drain's bounds.
const (
	sidekiqDrainWait     = 15 * time.Second
	sidekiqDrainInterval = 250 * time.Millisecond
)

// DrainSidekiq waits until GitLab's background job queues are empty, or
// fifteen seconds have passed, or the context ends.
//
// Most of what a test waits for afterwards (a branch, a merge check, a
// pipeline, a search index) is written by one of those jobs, so an empty
// queue is the cheapest possible readiness signal and the wait is shorter for
// it. It is best effort: the metrics API needs an administrator, and a token
// that cannot read it simply returns at once.
func DrainSidekiq(ctx context.Context, client *gitlabclient.Client) {
	deadline := time.Now().Add(sidekiqDrainWait)
	for time.Now().Before(deadline) {
		stats, _, err := client.GL().Sidekiq.GetJobStats(gl.WithContext(ctx))
		if err != nil || stats == nil || stats.Jobs.Enqueued == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sidekiqDrainInterval):
		}
	}
	log.Printf("e2e: Sidekiq still had jobs enqueued after %s; continuing", sidekiqDrainWait)
}
