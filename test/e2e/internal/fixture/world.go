//go:build e2e

// world.go builds the state every read-only test shares, once per process,
// and checks at the end that nobody wrote to it.
//
// A read sweep over a thousand actions cannot build a project for each of
// them, so the reads share one World: a group with a project in it, a branch
// with a commit, a merge request, an issue, a label, a milestone. The World is
// read-only by contract, and the contract is enforced after the fact: a
// digest of everything a test could change is taken when the World is built
// and again when it is torn down, and a difference fails the run and names
// the field. A test that mutates the World by mistake breaks unrelated read
// tests in ways that are hard to see; the digest turns that into one line.

package fixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// World is the read-only state a package's tests share.
type World struct {
	// Group is a top-level group; Project lives in it.
	Group Group
	// Project is a private project with a README on its default branch.
	Project Project
	// Branch is a feature branch of Project carrying Commit.
	Branch Branch
	// Commit is the one commit Branch has that the default branch does not.
	Commit Commit
	// MergeRequest merges Branch into the default branch, and is mergeable.
	MergeRequest MergeRequest
	// Issue is an open issue of Project.
	Issue Issue
	// Label is a label of Project.
	Label Label
	// Milestone is an active milestone of Project.
	Milestone Milestone
	// Username and UserID identify the run's own user, for user-scoped reads.
	Username string
	UserID   int64

	// baseline is every mutable field as it read when the World was built,
	// and digest is its hash: the digest decides, the baseline explains.
	baseline map[string]string
	digest   string
}

// The names the World's objects are created under. Each carries the run ID
// through worldName, so the sweep recognizes them.
const (
	worldGroupPrefix     = "e2e-world-group"
	worldProjectPrefix   = "e2e-world-project"
	worldBranch          = "feature/world"
	worldFilePath        = "docs/world.md"
	worldFileContent     = "# World\n\nThe shared read-only fixture of one e2e run.\n"
	worldCommitMessage   = "docs: add the World file"
	worldMergeTitle      = "World merge request"
	worldIssueTitle      = "World issue"
	worldLabelPrefix     = "world-label"
	worldMilestonePrefix = "world-milestone"
)

// worldName scopes one World object's name to the run.
func worldName(runID, prefix string) string {
	return prefix + "-" + runID
}

// worldNames hands a creation retry a fresh name each time it asks: the plain
// World name first, then the same with an attempt number, since a name GitLab
// refused as taken must not be offered again.
func worldNames(runID, prefix string) func() string {
	attempt := 0
	return func() string {
		attempt++
		if attempt == 1 {
			return worldName(runID, prefix)
		}
		return fmt.Sprintf("%s-%d", worldName(runID, prefix), attempt)
	}
}

// errWorldAborted is what a World whose build did not finish is reported as
// to every later caller. The test that built it carries the real failure.
var errWorldAborted = errors.New("the shared World was not built: the test that first asked for it failed while building it")

// shared is this process's World, built at most once.
var shared struct {
	once  sync.Once
	world *World
	err   error
	// builder names the test that built it, for the message a later caller
	// gets when the build failed.
	builder string
}

// SharedWorld returns the process's World, building it on the first call.
//
// The first caller's Env supplies the client and the test the build reports
// to. Whatever the build creates is torn down by an exit hook rather than by
// that test's ledger, since every later test stands on it too. A build that
// fails fails the test that asked first, and every later caller with a
// pointer to it.
func SharedWorld(e *harness.Env) *World {
	e.T.Helper()

	shared.once.Do(func() {
		shared.builder = e.T.Name()
		shared.err = errWorldAborted
		armSweep(e)

		world := &World{Username: e.Runtime().Username, UserID: e.Runtime().UserID}
		shared.world = world
		// Registered before anything is created, so a build that fails
		// halfway still has its group deleted at exit.
		harness.AtExit(func() error { return teardownWorld(e.Client(), world) })

		buildWorld(e, world)
		shared.err = nil
	})

	if shared.err != nil {
		e.T.Fatalf("%v (built by %s)", shared.err, shared.builder)
	}
	return shared.world
}

// buildWorld creates every object of the World in dependency order, filling
// in the fields as it goes so a build that fails leaves the created ones
// recorded, and takes the digest last.
func buildWorld(e *harness.Env, world *World) {
	e.T.Helper()

	runID := e.RunID()
	group, err := createGroup(e, groupSpec{visibility: gl.PrivateVisibility}, worldNames(runID, worldGroupPrefix))
	if err != nil {
		e.T.Fatalf("building the World: creating its group: %v", err)
	}
	world.Group = group

	spec := projectSpec{description: "The shared read-only World of run " + runID, visibility: gl.PrivateVisibility, readme: true, namespaceID: group.ID}
	project, err := createProject(e, spec, worldNames(runID, worldProjectPrefix))
	if err != nil {
		e.T.Fatalf("building the World: creating its project: %v", err)
	}
	world.Project = project
	waitForDefaultBranch(e, project)

	world.Branch = NewBranch(e, project, worldBranch)
	world.Commit = CommitFile(e, project, worldBranch, worldFilePath, worldFileContent, worldCommitMessage)
	world.MergeRequest = NewMergeRequest(e, project, worldBranch, project.DefaultBranch, worldMergeTitle)
	world.Issue = NewIssue(e, project, worldIssueTitle)
	world.Label = createLabel(e, project, worldName(runID, worldLabelPrefix))
	world.Milestone = createMilestone(e, project, worldName(runID, worldMilestonePrefix))

	state, err := worldState(e.Ctx, e.Client(), world)
	if err != nil {
		e.T.Fatalf("building the World: reading it back for its digest: %v", err)
	}
	world.baseline = state
	world.digest = Digest(state)
	log.Printf("e2e: World built: group %s, project %s, digest %s", group.Path, project.Path, world.digest)
}

// worldTeardownBudget bounds the teardown, digest read included.
const worldTeardownBudget = 3 * time.Minute

// teardownWorld re-reads the World, compares it with what was built, and
// deletes it. It fails on a difference and on a deletion that did not
// happen, and does both halves whatever the first found: a changed World is
// still deleted, and a World that could not be read is still torn down.
func teardownWorld(client *gitlabclient.Client, world *World) error {
	ctx, cancel := context.WithTimeout(context.Background(), worldTeardownBudget)
	defer cancel()

	var failures []error
	if world.digest != "" {
		if err := verifyWorld(ctx, client, world); err != nil {
			failures = append(failures, err)
		}
	}
	if world.Group.ID != 0 {
		if err := DeleteGroup(ctx, client, world.Group.ID, world.Group.Path); err != nil {
			failures = append(failures, fmt.Errorf("tearing down the World: %w", err))
		}
	}
	return errors.Join(failures...)
}

// verifyWorld reports what changed between the World's build and now.
func verifyWorld(ctx context.Context, client *gitlabclient.Client, world *World) error {
	now, err := worldState(ctx, client, world)
	if err != nil {
		return fmt.Errorf("re-reading the World for its digest: %w", err)
	}
	return compareWorld(world.baseline, world.digest, now)
}

// compareWorld is the verdict over two readings: nil when the digests agree,
// and otherwise an error naming every field that differs.
func compareWorld(baseline map[string]string, digest string, now map[string]string) error {
	if Digest(now) == digest {
		return nil
	}
	return fmt.Errorf("%w: %s", errWorldChanged, strings.Join(worldStateDiff(baseline, now), "; "))
}

// errWorldChanged is the failure a run whose read-only World was written to
// ends with.
var errWorldChanged = errors.New("the shared World was changed during the run")

// Digest hashes a World's mutable state: every field, sorted by name, so two
// readings of an unchanged World hash the same whatever order the map was
// filled in. Each key and value is written with its length in front, so a
// separator inside a value cannot make two different states hash the same.
func Digest(state map[string]string) string {
	keys := make([]string, 0, len(state))
	for key := range state {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	hash := sha256.New()
	for _, key := range keys {
		fmt.Fprintf(hash, "%d:%s=%d:%s\n", len(key), key, len(state[key]), state[key])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// worldStateDiff names every field that differs between two readings, in
// field order, as "field: before -> after".
func worldStateDiff(before, after map[string]string) []string {
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}

	var changes []string
	for key := range keys {
		was, had := before[key]
		now, has := after[key]
		switch {
		case had && !has:
			changes = append(changes, fmt.Sprintf("%s: %q -> (gone)", key, was))
		case !had && has:
			changes = append(changes, fmt.Sprintf("%s: (absent) -> %q", key, now))
		case was != now:
			changes = append(changes, fmt.Sprintf("%s: %q -> %q", key, was, now))
		}
	}
	slices.Sort(changes)
	return changes
}

// worldState reads every field of the World a test could change and returns
// them keyed by object and field.
func worldState(ctx context.Context, client *gitlabclient.Client, world *World) (map[string]string, error) {
	state := map[string]string{}
	readers := []func() error{
		func() error { return readGroupState(ctx, client, world.Group, state) },
		func() error { return readProjectState(ctx, client, world.Project, state) },
		func() error { return readBranchState(ctx, client, world, state) },
		func() error { return readMergeRequestState(ctx, client, world, state) },
		func() error { return readIssueState(ctx, client, world, state) },
		func() error { return readLabelState(ctx, client, world, state) },
		func() error { return readMilestoneState(ctx, client, world, state) },
	}
	for _, read := range readers {
		if err := read(); err != nil {
			return nil, err
		}
	}
	return state, nil
}

// readGroupState records the group's name, path and visibility.
func readGroupState(ctx context.Context, client *gitlabclient.Client, group Group, state map[string]string) error {
	got, _, err := client.GL().Groups.GetGroup(group.ID, nil, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading group %d: %w", group.ID, err)
	}
	state["group.name"] = got.Name
	state["group.full_path"] = got.FullPath
	state["group.visibility"] = string(got.Visibility)
	state["group.description"] = got.Description
	return nil
}

// readProjectState records the project's identity and settings.
func readProjectState(ctx context.Context, client *gitlabclient.Client, project Project, state map[string]string) error {
	got, _, err := client.GL().Projects.GetProject(project.ID, nil, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading project %d: %w", project.ID, err)
	}
	state["project.name"] = got.Name
	state["project.path_with_namespace"] = got.PathWithNamespace
	state["project.default_branch"] = got.DefaultBranch
	state["project.visibility"] = string(got.Visibility)
	state["project.description"] = got.Description
	state["project.archived"] = strconv.FormatBool(got.Archived)
	state["project.topics"] = strings.Join(sortedCopy(got.Topics), ",")
	return nil
}

// readBranchState records where the two branches point.
//
// Whether a branch is protected is deliberately not recorded: an instance
// with default branch protection applies it in a background job after the
// project is created, and a digest taken before that job ran would report
// the instance's own work as a change some test made.
func readBranchState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	for _, name := range []string{world.Project.DefaultBranch, world.Branch.Name} {
		got, _, err := client.GL().Branches.GetBranch(world.Project.ID, name, gl.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("reading branch %q of project %d: %w", name, world.Project.ID, err)
		}
		sha := ""
		if got.Commit != nil {
			sha = got.Commit.ID
		}
		state["branch."+name+".sha"] = sha
	}
	return nil
}

// readMergeRequestState records the merge request's identity and state.
func readMergeRequestState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().MergeRequests.GetMergeRequest(world.Project.ID, world.MergeRequest.IID, nil, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading merge request !%d of project %d: %w", world.MergeRequest.IID, world.Project.ID, err)
	}
	state["merge_request.title"] = got.Title
	state["merge_request.state"] = got.State
	state["merge_request.description"] = got.Description
	state["merge_request.source_branch"] = got.SourceBranch
	state["merge_request.target_branch"] = got.TargetBranch
	state["merge_request.labels"] = strings.Join(sortedCopy(got.Labels), ",")
	return nil
}

// readIssueState records the issue's identity and state.
func readIssueState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Issues.GetIssue(world.Project.ID, world.Issue.IID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading issue #%d of project %d: %w", world.Issue.IID, world.Project.ID, err)
	}
	state["issue.title"] = got.Title
	state["issue.state"] = got.State
	state["issue.description"] = got.Description
	state["issue.labels"] = strings.Join(sortedCopy(got.Labels), ",")
	state["issue.confidential"] = strconv.FormatBool(got.Confidential)
	return nil
}

// readLabelState records the label's name, color and description.
func readLabelState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Labels.GetLabel(world.Project.ID, world.Label.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading label %d of project %d: %w", world.Label.ID, world.Project.ID, err)
	}
	state["label.name"] = got.Name
	state["label.color"] = got.Color
	state["label.description"] = got.Description
	return nil
}

// readMilestoneState records the milestone's title and state.
func readMilestoneState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Milestones.GetMilestone(world.Project.ID, world.Milestone.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading milestone %d of project %d: %w", world.Milestone.ID, world.Project.ID, err)
	}
	state["milestone.title"] = got.Title
	state["milestone.state"] = got.State
	state["milestone.description"] = got.Description
	return nil
}

// sortedCopy returns a sorted copy of a list, so a set GitLab returns in no
// particular order digests the same every time.
func sortedCopy(values []string) []string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted
}

// Bindings returns every parameter the World can bind, by the name a
// catalog action's schema gives it. It is what a read sweep binds required
// parameters from, and a parameter it does not carry is one the sweep names
// as unbound.
func (w *World) Bindings() map[string]any {
	return map[string]any{
		"project_id":        w.Project.ID,
		"group_id":          w.Group.ID,
		"namespace_id":      w.Group.ID,
		"issue_iid":         w.Issue.IID,
		"issue_id":          w.Issue.ID,
		"merge_request_iid": w.MergeRequest.IID,
		"mr_iid":            w.MergeRequest.IID,
		"branch":            w.Branch.Name,
		"branch_name":       w.Branch.Name,
		"ref":               w.Project.DefaultBranch,
		"sha":               w.Commit.SHA,
		"commit_sha":        w.Commit.SHA,
		"commit_id":         w.Commit.SHA,
		"file_path":         w.Commit.FilePath,
		"label_id":          w.Label.ID,
		"milestone_id":      w.Milestone.ID,
		"user_id":           w.UserID,
		"username":          w.Username,
	}
}

// Bind returns the World's value for one parameter name, and whether it has
// one.
func (w *World) Bind(param string) (any, bool) {
	value, ok := w.Bindings()[param]
	return value, ok
}
