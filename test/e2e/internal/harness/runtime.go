//go:build e2e

// runtime.go answers one question before any test writes anything: is this the
// GitLab the package in front of us needs.
//
// A package declares what it needs once, in its Main, and the probe reads what
// the instance actually is. When the two disagree the run stops with a block
// that names both and the target to run instead, rather than with the hundreds
// of 403s and 404s that a licensed package produces against a Free instance.
// The probe runs before the first write for the same reason: the old suite
// disabled rate limiting and warmed the API up before it had established what
// it was talking to.

package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// Requirement is what a package's tests need of the instance they run on.
type Requirement int

const (
	// Any runs on every runtime: the actions it covers are Free ones, and a
	// license changes how they are served rather than whether they exist.
	Any Requirement = iota
	// Free needs an instance with no license, for the handful of facts that
	// are only true there.
	Free
	// Licensed needs a Premium or Ultimate instance.
	Licensed
)

// String names the requirement in a message.
func (r Requirement) String() string {
	switch r {
	case Free:
		return "free (no license)"
	case Licensed:
		return "licensed (Premium or Ultimate)"
	case Any:
		return "any runtime"
	default:
		return "unknown"
	}
}

// token names the requirement in a record: one word, which is what a report
// groups by. String is the sentence a refusal prints and would be a poor key.
func (r Requirement) token() string {
	switch r {
	case Free:
		return "free"
	case Licensed:
		return "licensed"
	case Any:
		return "any"
	default:
		return "unknown"
	}
}

// target names the Makefile target that provides this requirement, which is
// what a refusal tells the reader to run instead: the two Docker targets that
// run the rebuilt packages on the runtime each needs.
func (r Requirement) target() string {
	if r == Licensed {
		return "make test-e2e-ee"
	}
	return "make test-e2e-ce"
}

// runtimeFacts is what the probe learned about the instance under test.
type runtimeFacts struct {
	// URL is the instance the run is pointed at.
	URL string
	// Version is what GET /api/v4/version reported.
	Version string
	// Enterprise is that endpoint's own edition flag: an EE image reports
	// true whether or not a license is installed, which is why it is kept
	// apart from Tier.
	Enterprise bool
	// Tier is what the license resolves to, Free when there is none.
	Tier edition.Tier
	// TierConfirmed says the tier came from a license rather than from the
	// fallback, so a report can tell "Free" from "we could not ask".
	TierConfirmed bool
	// Admin says the token's user is an instance administrator.
	Admin bool
	// Username and UserID identify that user.
	Username string
	UserID   int64
	// Scopes are the token's own, nil when the instance would not say.
	Scopes []string
}

// editionName names the image's edition in a message. It is not called
// edition, though that is what it returns, because the package that defines
// the tier beside it is.
func (f runtimeFacts) editionName() string {
	if f.Enterprise {
		return "Enterprise Edition"
	}
	return "Community Edition"
}

// editionToken names the image's edition in a record: one word, beside the
// sentence editionName prints for a person.
func (f runtimeFacts) editionToken() string {
	if f.Enterprise {
		return "enterprise"
	}
	return "community"
}

// tierDescription says what the tier is and whether a license confirmed it.
func (f runtimeFacts) tierDescription() string {
	if f.TierConfirmed {
		return f.Tier.String() + " (license)"
	}
	return f.Tier.String() + " (no license found)"
}

// instance is everything the bootstrap resolved: what the run was configured
// with, what the instance turned out to be, and the client the harness uses
// for its own questions.
type instance struct {
	settings    settings
	requirement Requirement
	pkg         string
	runID       string
	facts       runtimeFacts
	client      *gitlabclient.Client

	runnerOnce sync.Once
	runner     bool

	snapshot *resourceSnapshot
}

// dockerMode reports whether the run is against the ephemeral Docker stack,
// which is disposable and therefore needs no snapshot guard.
func (inst *instance) dockerMode() bool {
	return strings.EqualFold(inst.settings.get(envMode), "docker")
}

// cleanupBudget is how long one test's undo work may take.
func (inst *instance) cleanupBudget() time.Duration {
	if inst.facts.Tier.IsEnterprise() {
		return enterpriseCleanupBudget
	}
	return cleanupBudget
}

// probeTimeout bounds each question the probe asks.
const probeTimeout = 30 * time.Second

// probe asks the instance what it is, without changing anything.
func probe(ctx context.Context, client *gitlabclient.Client, url string, skipTLSVerify bool, token string) (runtimeFacts, error) {
	version, err := client.Ping(ctx)
	if err != nil {
		return runtimeFacts{}, err
	}
	facts := runtimeFacts{URL: url, Version: version}

	// The edition flag is read with a request of our own rather than through
	// DetectEnterprise, which sets the client's tier to Premium for any EE
	// image, licensed or not. The harness wants the two facts apart.
	facts.Enterprise = instanceIsEnterprise(ctx, url, token, skipTLSVerify)

	facts.Tier = client.DetectTier(ctx)
	client.SetTier(facts.Tier)
	facts.TierConfirmed = facts.Tier.IsEnterprise()

	user, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		return facts, fmt.Errorf("reading the authenticated user: %w", err)
	}
	facts.Admin = user.IsAdmin
	facts.Username = user.Username
	facts.UserID = user.ID

	facts.Scopes = gitlabclient.DetectScopes(ctx, client.GL())
	return facts, nil
}

// instanceIsEnterprise reads the edition flag off GET /api/v4/version.
//
// An instance that will not answer, or that omits the field as older releases
// do, is reported as Community: the flag only widens what a refusal message
// says, and guessing Enterprise would make that message claim something the
// instance never said.
func instanceIsEnterprise(ctx context.Context, url, token string, skipTLSVerify bool) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/api/v4/version", http.NoBody)
	if err != nil {
		return false
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{Transport: gitlabclient.HTTPTransport(skipTLSVerify), Timeout: probeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var reported struct {
		Enterprise *bool `json:"enterprise"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&reported); err != nil || reported.Enterprise == nil {
		return false
	}
	return *reported.Enterprise
}

// satisfies reports whether facts meet req.
func (r Requirement) satisfies(facts runtimeFacts) bool {
	switch r {
	case Free:
		return !facts.Tier.IsEnterprise()
	case Licensed:
		return facts.Tier.IsEnterprise()
	case Any:
		return true
	default:
		return false
	}
}

// guardMessage returns the block a refused runtime prints, and the empty
// string when the instance satisfies the requirement.
//
// It is a plain function of the two facts so that the message itself can be
// tested: the text is the whole value of the guard, and a refusal that named
// neither what it found nor what to run instead would be no better than the
// failures it replaces.
func guardMessage(facts runtimeFacts, req Requirement, pkg string) string {
	if req.satisfies(facts) {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "this GitLab does not provide what the %s package needs.\n\n", pkg)
	fmt.Fprintf(&b, "  instance     %s\n", facts.URL)
	fmt.Fprintf(&b, "  version      %s (%s)\n", facts.Version, facts.editionName())
	fmt.Fprintf(&b, "  tier         %s\n", facts.tierDescription())
	fmt.Fprintf(&b, "  requirement  %s\n\n", req)
	fmt.Fprintf(&b, "  run this package against a runtime that provides it:\n      %s\n", req.target())
	fmt.Fprintf(&b, "  or set %s=skip to skip the package instead of failing it.", envRuntimeMismatch)
	return b.String()
}

// missingCredentialsMessage is what a run configured with no instance is told.
func missingCredentialsMessage(pkg string) string {
	return fmt.Sprintf("the %s package has no GitLab to run against: %s and %s are both required.\n\n"+
		"  they come from the process environment, from %s, from test/e2e/.env.docker in Docker mode,\n"+
		"  or from the repository .env, in that order of precedence.",
		pkg, envGitLabURL, envGitLabToken, envEnvFile)
}

// unreachableMessage is what a run whose instance did not answer is told. The
// instance is named and the token is not.
func unreachableMessage(pkg, url string, err error) string {
	return fmt.Sprintf("the %s package could not reach the GitLab it was pointed at.\n\n"+
		"  instance     %s\n  error        %v", pkg, url, err)
}

// prepareInstance does everything that has to happen once, after the guard and
// before the first test, and changes GitLab in the process.
//
// Every item here was learned the hard way by the suite this replaces, and the
// comments say which failure each one answers.
func prepareInstance(inst *instance) {
	disableRateLimiting(inst.client)
	if inst.dockerMode() {
		log.Println("e2e: warming up the GitLab API for concurrent load")
		if err := waitForAPIStable(inst.client, apiWarmupTimeout); err != nil {
			log.Printf("e2e: %v; tests may see dropped connections", err)
		}
	}
	materializeGlobalNotificationSetting(inst.client)
	if !inst.dockerMode() {
		captureSnapshot(inst)
	}
}

// disableRateLimiting turns GitLab's throttles off for the run, so a suite
// making thousands of calls is not answered 429 for doing what it was asked to
// do. It needs an administrator; a refusal is logged and is not fatal.
func disableRateLimiting(client *gitlabclient.Client) {
	off := false
	_, _, err := client.GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{
		ThrottleAuthenticatedAPIEnabled:             &off,
		ThrottleAuthenticatedWebEnabled:             &off,
		ThrottleUnauthenticatedAPIEnabled:           &off,
		ThrottleUnauthenticatedWebEnabled:           &off,
		ThrottleAuthenticatedPackagesAPIEnabled:     &off,
		ThrottleAuthenticatedGitLFSEnabled:          &off,
		ThrottleAuthenticatedFilesAPIEnabled:        &off,
		ThrottleUnauthenticatedFilesAPIEnabled:      &off,
		ThrottleAuthenticatedDeprecatedAPIEnabled:   &off,
		ThrottleUnauthenticatedDeprecatedAPIEnabled: &off,
	})
	if err != nil {
		log.Printf("e2e: could not disable rate limiting (it needs an administrator): %v", err)
		return
	}
	log.Println("e2e: rate limiting disabled for this run")
}

// The bounds the Docker warm-up runs under.
const (
	apiWarmupTimeout      = 60 * time.Second
	apiWarmupConcurrency  = 10
	apiWarmupSuccessRuns  = 3
	apiWarmupProbeTimeout = 5 * time.Second
)

// waitForAPIStable holds the run back until GitLab answers a burst of parallel
// requests without dropping any, three rounds running.
//
// A Docker GitLab reports ready while its nginx and puma workers are still
// warming up, and the first thing this suite does is exactly the burst they
// are not ready for. Waiting here turns a scatter of connection resets across
// unrelated tests into one wait with a name.
func waitForAPIStable(client *gitlabclient.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	successRuns := 0
	for time.Now().Before(deadline) {
		var wg sync.WaitGroup
		errs := make([]error, apiWarmupConcurrency)
		for i := range apiWarmupConcurrency {
			wg.Go(func() {
				ctx, cancel := context.WithTimeout(context.Background(), apiWarmupProbeTimeout)
				defer cancel()
				_, errs[i] = client.Ping(ctx)
			})
		}
		wg.Wait()

		if slicesContainError(errs) {
			successRuns = 0
			log.Println("e2e: API warm-up: some connections were dropped, retrying")
			time.Sleep(2 * time.Second)
			continue
		}
		successRuns++
		if successRuns >= apiWarmupSuccessRuns {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("the GitLab API was not stable after %v of concurrent probing", timeout)
}

// slicesContainError reports whether any probe of one warm-up round failed.
func slicesContainError(errs []error) bool {
	for _, err := range errs {
		if err != nil {
			return true
		}
	}
	return false
}

// materializeNotificationAttempts bounds the retries the notification row gets.
const materializeNotificationAttempts = 3

// materializeGlobalNotificationSetting forces the authenticated user's global
// notification row to exist before any test runs.
//
// GitLab creates that row lazily, inside the read: the model does a
// find_or_initialize_by and saves an unpersisted record, so even a plain GET
// writes. Two concurrent readers therefore both insert and the loser is
// refused with a uniqueness error on a request that only read. Reading it once
// here, alone, closes that window for the whole run.
//
// It is defense in depth rather than the mechanism: the tests that change the
// setting hold LockCurrentUserState. A failure is therefore logged and never
// fatal.
func materializeGlobalNotificationSetting(client *gitlabclient.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	var err error
	for attempt := 1; attempt <= materializeNotificationAttempts; attempt++ {
		if _, _, err = client.GL().NotificationSettings.GetGlobalSettings(gl.WithContext(ctx)); err == nil {
			return
		}
		if attempt < materializeNotificationAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	log.Printf("e2e: could not materialize the global notification setting after %d attempts: %v; "+
		"the lock still serializes the tests that use it", materializeNotificationAttempts, err)
}

// resourceSnapshot is what the instance held before the run, so a run against
// an instance somebody else owns can prove it left everything alone.
type resourceSnapshot struct {
	groups   map[int64]string
	projects map[int64]string
}

// snapshotPageSize is how many objects each listing page carries.
const snapshotPageSize = 100

// captureSnapshot records the instance's groups and projects, and refuses the
// run when it cannot: a guard that silently did not run would be worse than no
// guard, because the run would be believed.
func captureSnapshot(inst *instance) {
	snapshot, err := snapshotState(inst.client)
	if err != nil {
		log.Printf("e2e: could not snapshot the instance's existing resources: %v", err)
		return
	}
	inst.snapshot = snapshot
	log.Printf("e2e: snapshot captured: %d groups, %d projects", len(snapshot.groups), len(snapshot.projects))
}

// snapshotState lists every group and project the token can see.
func snapshotState(client *gitlabclient.Client) (*resourceSnapshot, error) {
	snapshot := &resourceSnapshot{groups: map[int64]string{}, projects: map[int64]string{}}

	var page int64 = 1
	for page != 0 {
		opts := &gl.ListGroupsOptions{}
		opts.Page = page
		opts.PerPage = snapshotPageSize
		groups, resp, err := client.GL().Groups.ListGroups(opts)
		if err != nil {
			return nil, fmt.Errorf("listing groups (page %d): %w", page, err)
		}
		for _, group := range groups {
			snapshot.groups[group.ID] = group.FullPath
		}
		page = resp.NextPage
	}

	page = 1
	for page != 0 {
		opts := &gl.ListProjectsOptions{}
		opts.Page = page
		opts.PerPage = snapshotPageSize
		projects, resp, err := client.GL().Projects.ListProjects(opts)
		if err != nil {
			return nil, fmt.Errorf("listing projects (page %d): %w", page, err)
		}
		for _, project := range projects {
			snapshot.projects[project.ID] = project.PathWithNamespace
		}
		page = resp.NextPage
	}

	return snapshot, nil
}

// verifySnapshot re-reads the instance and reports what the run changed about
// resources it did not create.
func verifySnapshot(client *gitlabclient.Client, before *resourceSnapshot) error {
	current, err := snapshotState(client)
	if err != nil {
		return fmt.Errorf("re-reading the instance: %w", err)
	}
	if changes := snapshotDifferences(before, current); len(changes) > 0 {
		return fmt.Errorf("%d resources this run did not create were changed or deleted:\n  %s",
			len(changes), strings.Join(changes, "\n  "))
	}
	return nil
}

// snapshotDifferences names every group and project that disappeared or was
// renamed between the two readings.
func snapshotDifferences(before, current *resourceSnapshot) []string {
	var changes []string
	for id, path := range before.groups {
		switch now, found := current.groups[id]; {
		case !found:
			changes = append(changes, fmt.Sprintf("group %q (ID=%d): missing", path, id))
		case now != path:
			changes = append(changes, fmt.Sprintf("group ID=%d renamed: %q to %q", id, path, now))
		}
	}
	for id, path := range before.projects {
		switch now, found := current.projects[id]; {
		case !found:
			changes = append(changes, fmt.Sprintf("project %q (ID=%d): missing", path, id))
		case now != path:
			changes = append(changes, fmt.Sprintf("project ID=%d renamed: %q to %q", id, path, now))
		}
	}
	return changes
}
