//go:build e2e

// needs.go holds the three things a test can say about how it must run, and
// they are deliberately three rather than one.
//
// A Need is a fact about the environment: without it the test cannot run at
// all and is skipped with a reason. A Lock is shared GitLab state the test
// will change, so only one holder at a time may touch it; the test runs
// either way. Serial is stronger than any lock: the test runs with no other
// test of its package alive, which is what a change to the instance license
// needs.
//
// The suite this replaces had one vocabulary for all three, and the cost was
// that a reader could not tell a missing runner (skip) from a mutable setting
// (serialize) without reading the switch that handled both.

package harness

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// Option configures one Env.
type Option func(*envOptions)

// envOptions is what the options a caller passed add up to.
type envOptions struct {
	needs  []Need
	locks  []Lock
	serial bool
}

// Need is an environment requirement. A test declaring one it does not get is
// skipped, with the reason the Need gives.
type Need struct {
	// name identifies the requirement in a skip message.
	name string
	// available answers whether this run has it, and says why not when it
	// does not.
	available func(inst *instance) (bool, string)
}

// String returns the requirement's name, so a Need reads as itself in a
// message rather than as a struct.
func (n Need) String() string { return n.name }

// Needs declares environment requirements. A test that declares one the run
// does not provide is skipped before it touches GitLab.
func Needs(needs ...Need) Option {
	return func(o *envOptions) { o.needs = append(o.needs, needs...) }
}

// Locks declares the shared GitLab state this test will change. Every holder
// of one lock runs after the last, in the order the scheduler picks.
func Locks(locks ...Lock) Option {
	return func(o *envOptions) { o.locks = append(o.locks, locks...) }
}

// Serial declares that this test runs with no other Env of its package alive.
//
// It is for a change nothing else can be beside, such as installing or
// removing the instance license: a lock would only exclude the other tests
// that named the same lock, and every other test is affected by a license
// that comes and goes underneath it.
func Serial() Option {
	return func(o *envOptions) { o.serial = true }
}

// Lock names shared state that only one test may change at a time.
type Lock string

// The shared state this suite serializes. Each names something one test
// changes and every other test can observe.
const (
	// LockInstanceGlobal is any application-wide setting.
	LockInstanceGlobal Lock = "instance-global"
	// LockCurrentUserState is the authenticated user's own settings, which
	// every session shares because every session uses that token.
	LockCurrentUserState Lock = "current-user-state"
	// LockRunner is the CI runner, which two pipelines would otherwise
	// contend for.
	LockRunner Lock = "runner"
	// LockInstanceLicense is the license itself.
	LockInstanceLicense Lock = "instance-license"
)

// The environment requirements a test can declare. Each is a fact about the
// instance or the fixture stack, never about the catalog: what the server
// serves is decided by the session, not by a Need.
var (
	// NeedAdmin requires a token whose user is an instance administrator.
	NeedAdmin = Need{name: "admin", available: func(inst *instance) (bool, string) {
		if inst.facts.Admin {
			return true, ""
		}
		return false, "the configured token does not belong to an administrator"
	}}

	// NeedRunner requires a CI runner able to pick a job up, without which a
	// pipeline stays pending until the test's budget runs out.
	NeedRunner = Need{name: "runner", available: func(inst *instance) (bool, string) {
		if inst.hasRunner() {
			return true, ""
		}
		return false, "no instance CI runner is registered"
	}}

	// NeedFixtureService requires the fixture HTTP service the Docker stack
	// runs, which answers the endpoints GitLab itself cannot be made to
	// produce deterministically.
	NeedFixtureService = requiredSetting("fixture service", envFixtureURL)

	// NeedBitbucket requires the Bitbucket fixture the importer scenarios
	// read from.
	NeedBitbucket = requiredSetting("bitbucket fixture", envBitbucketServerURL)

	// NeedGitHubToken requires a GitHub credential, which only the GitHub
	// importer scenarios use.
	NeedGitHubToken = requiredSetting("github token", envGitHubToken)

	// NeedExternalNetwork requires the run to have opted into calling public
	// Internet endpoints. It stays opt-in because a test that reaches the
	// Internet fails for reasons that have nothing to do with this server.
	NeedExternalNetwork = Need{name: "external network", available: func(inst *instance) (bool, string) {
		if strings.EqualFold(inst.settings.get(envExternalNetwork), "true") {
			return true, ""
		}
		return false, "set " + envExternalNetwork + "=true to allow tests that call public URLs"
	}}
)

// Tier requires an instance licensed at least at the given tier. It is a
// constructor rather than a constant because the tier is the argument.
func Tier(want edition.Tier) Need {
	return Need{
		name: "tier " + want.String(),
		available: func(inst *instance) (bool, string) {
			if inst.facts.Tier.AtLeast(want) {
				return true, ""
			}
			return false, "this instance is " + inst.facts.Tier.String() + ", and the test needs " + want.String()
		},
	}
}

// requiredSetting builds the Need satisfied by a configuration key carrying a
// value, which is how every fixture the Docker stack provisions announces
// itself.
func requiredSetting(name, key string) Need {
	return Need{
		name: name,
		available: func(inst *instance) (bool, string) {
			if inst.settings.get(key) != "" {
				return true, ""
			}
			return false, key + " is not set, so the " + name + " is not provisioned for this run"
		},
	}
}

// runnerLookupTimeout bounds the one Runners API call a NeedRunner outside
// Docker mode makes.
const runnerLookupTimeout = 10 * time.Second

// hasRunner reports whether a CI runner can pick a job up, asking GitLab once
// and remembering the answer.
//
// Docker mode answers yes without asking: the compose stack registers a runner
// of its own, and the API listing is not visible to a non-administrator token
// even when the runner is there.
func (inst *instance) hasRunner() bool {
	if inst.dockerMode() {
		return true
	}
	inst.runnerOnce.Do(func() {
		if inst.client == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), runnerLookupTimeout)
		defer cancel()
		runnerType := "instance_type"
		runners, _, err := inst.client.GL().Runners.ListRunners(&gl.ListRunnersOptions{Type: &runnerType},
			gl.WithContext(ctx))
		inst.runner = err == nil && len(runners) > 0
	})
	return inst.runner
}

// requireNeeds skips the test, naming the first requirement this run does not
// provide.
//
// The reason goes to the recorder before the skip, because the testing package
// keeps a skip message to itself: a coverage report that could not say why a
// scenario was absent would read the same whether the fixture stack was
// incomplete or the scenario was never written.
func requireNeeds(t *testing.T, inst *instance, needs []Need, rec *envRecorder) {
	t.Helper()
	for _, need := range needs {
		if ok, reason := need.available(inst); !ok {
			message := fmt.Sprintf("%s unavailable: %s", need.name, reason)
			rec.noteSkip(message)
			t.Skip(message)
		}
	}
}

// lockRegistry holds one mutex per Lock, created when a test first names it.
var lockRegistry = struct {
	mu    sync.Mutex
	locks map[Lock]*sync.Mutex
}{locks: map[Lock]*sync.Mutex{}}

// lockFor returns the mutex for one Lock.
func lockFor(lock Lock) *sync.Mutex {
	lockRegistry.mu.Lock()
	defer lockRegistry.mu.Unlock()

	held := lockRegistry.locks[lock]
	if held == nil {
		held = &sync.Mutex{}
		lockRegistry.locks[lock] = held
	}
	return held
}

// acquireLocks takes every named lock in a fixed order and returns the release
// that gives them back in the reverse one.
//
// The order is what keeps two tests naming the same pair from deadlocking:
// sorting means they always take them the same way round. Duplicates are
// dropped, since a test naming one lock twice would deadlock against itself.
func acquireLocks(locks []Lock) func() {
	wanted := slices.Clone(locks)
	slices.Sort(wanted)
	wanted = slices.Compact(wanted)

	for _, lock := range wanted {
		lockFor(lock).Lock()
	}
	return func() {
		for _, lock := range slices.Backward(wanted) {
			lockFor(lock).Unlock()
		}
	}
}

// serialGate lets a test declared Serial run while no other test of the
// package holds an Env.
//
// It counts holders per top-level test rather than per Env, so a test that
// opens a second Env for a subtest re-enters the gate it already holds instead
// of waiting behind a serial test for a slot it will never be given. That is
// the deadlock a plain sync.RWMutex produces here, and it is not hypothetical:
// one parent fixture running three surface subtests is the shape the suite is
// built around.
type serialGate struct {
	mu     sync.Mutex
	cond   *sync.Cond
	shared map[string]int
	// alone is the top-level test currently running by itself, if any.
	alone string
}

// newSerialGate returns a gate nothing holds.
func newSerialGate() *serialGate {
	g := &serialGate{shared: map[string]int{}}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// gate is the package's one serial gate.
var gate = newSerialGate()

// enter admits one Env of test and returns the function that gives its slot
// back. With alone set, it waits until every other test has left and holds the
// gate shut until the returned function runs.
func (g *serialGate) enter(test string, alone bool) func() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for g.alone != "" && g.alone != test && g.shared[test] == 0 {
		g.cond.Wait()
	}
	if alone {
		g.alone = test
		for g.othersHolding(test) {
			g.cond.Wait()
		}
	}
	g.shared[test]++

	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.shared[test]--
		if g.shared[test] <= 0 {
			delete(g.shared, test)
			if g.alone == test {
				g.alone = ""
			}
		}
		g.cond.Broadcast()
	}
}

// othersHolding reports whether any test but this one holds a slot.
func (g *serialGate) othersHolding(test string) bool {
	for held, count := range g.shared {
		if held != test && count > 0 {
			return true
		}
	}
	return false
}

// topLevelTest returns the name of the test a subtest belongs to, which is the
// grain the serial gate counts at.
func topLevelTest(name string) string {
	parent, _, _ := strings.Cut(name, "/")
	return parent
}
