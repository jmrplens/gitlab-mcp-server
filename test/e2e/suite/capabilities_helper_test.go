//go:build e2e

// capabilities_helper_test.go defines capability gates and locks used by E2E
// tests that require optional GitLab features or shared mutable instance state.
package suite

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Capability identifies an E2E environment requirement or shared-state scope.
type Capability string

// Capability values describe the resource scopes that need explicit gates.
const (
	CapabilityAdmin            Capability = "admin"
	CapabilityEnterprise       Capability = "enterprise"
	CapabilityRunner           Capability = "runner"
	CapabilityInstanceGlobal   Capability = "instance-global"
	CapabilityCurrentUserState Capability = "current-user-state"
	CapabilitySafeMode         Capability = "safe-mode"
	CapabilitySampling         Capability = "sampling"
	CapabilityElicitation      Capability = "elicitation"
	CapabilityExternalNetwork  Capability = "external-network"
)

// adminCapability caches a successful administrator probe so admin-only tests
// do not repeatedly verify a token that is already known to be privileged.
var adminCapability = struct {
	mu     sync.Mutex
	cached bool
	ok     bool
}{}

// capabilityLocks stores process-local mutexes for capabilities that mutate
// shared GitLab state and therefore must not run concurrently.
var capabilityLocks = struct {
	mu    sync.Mutex
	locks map[Capability]*sync.Mutex
}{locks: map[Capability]*sync.Mutex{}}

// RequireCapabilities skips the test when required E2E capabilities are absent.
func RequireCapabilities(t *testing.T, caps ...Capability) {
	t.Helper()

	for _, capability := range caps {
		switch capability {
		case CapabilityAdmin:
			if ok, err := hasAdminCapability(); !ok {
				if err != nil {
					t.Skipf("admin capability unavailable: %v", err)
				}
				t.Skip("admin capability unavailable")
			}
		case CapabilityEnterprise:
			if !sess.enterprise {
				t.Skip("enterprise capability unavailable")
			}
		case CapabilityRunner:
			if !hasRunner(sess.glClient) {
				t.Skip("runner capability unavailable")
			}
		case CapabilitySafeMode:
			if sess.safeMode == nil {
				t.Skip("safe-mode MCP session not configured")
			}
		case CapabilitySampling:
			if sess.sampling == nil {
				t.Skip("sampling MCP session not configured")
			}
		case CapabilityElicitation:
			if sess.elicitation == nil {
				t.Skip("elicitation MCP session not configured")
			}
		case CapabilityExternalNetwork:
			if !hasExternalNetworkCapability() {
				t.Skip("external-network capability unavailable; prefer Docker fixture endpoints; set E2E_EXTERNAL_NETWORK=true only for tests that must call public URLs")
			}
		case CapabilityInstanceGlobal, CapabilityCurrentUserState:
			// These capabilities represent shared mutable state and are enforced by locks.
		default:
			t.Fatalf("unknown E2E capability %q", capability)
		}
	}
}

// hasExternalNetworkCapability reports whether tests that must call public
// Internet endpoints are explicitly enabled for the current E2E run.
func hasExternalNetworkCapability() bool {
	return strings.EqualFold(os.Getenv("E2E_EXTERNAL_NETWORK"), "true")
}

// RunWithCapabilities centralizes capability checks, locks, and E2E context setup.
func RunWithCapabilities(t *testing.T, caps []Capability, fn func(t *testing.T, e2e *E2EContext)) {
	t.Helper()

	RequireCapabilities(t, caps...)
	unlock := acquireCapabilityLocks(caps)
	t.Cleanup(unlock)

	e2e := NewE2EContext(t)
	fn(t, e2e)
}

// hasAdminCapability reports whether the configured GitLab user is an
// administrator, caching only successful admin probes so transient failures can
// be retried by later tests.
func hasAdminCapability() (bool, error) {
	adminCapability.mu.Lock()
	if adminCapability.cached {
		ok := adminCapability.ok
		adminCapability.mu.Unlock()
		return ok, nil
	}
	adminCapability.mu.Unlock()

	if sess.glClient == nil {
		return false, gitLabClientUnavailableError{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, _, err := sess.glClient.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		return false, err
	}
	if !user.IsAdmin {
		return false, nil
	}

	adminCapability.mu.Lock()
	adminCapability.ok = true
	adminCapability.cached = true
	adminCapability.mu.Unlock()

	return true, nil
}

// acquireCapabilityLocks locks all serialized capabilities in deterministic
// order and returns a cleanup function that releases them in reverse order.
func acquireCapabilityLocks(caps []Capability) func() {
	lockCaps := lockedCapabilities(caps)
	for _, capability := range lockCaps {
		lockForCapability(capability).Lock()
	}

	return func() {
		for i := len(lockCaps) - 1; i >= 0; i-- {
			lockForCapability(lockCaps[i]).Unlock()
		}
	}
}

// lockedCapabilities returns the subset of capabilities that require serialized
// execution because they mutate instance-wide or current-user state.
func lockedCapabilities(caps []Capability) []Capability {
	seen := make(map[Capability]struct{}, len(caps))
	for _, capability := range caps {
		switch capability {
		case CapabilityRunner, CapabilityInstanceGlobal, CapabilityCurrentUserState:
			seen[capability] = struct{}{}
		}
	}

	lockCaps := make([]Capability, 0, len(seen))
	for capability := range seen {
		lockCaps = append(lockCaps, capability)
	}
	slices.Sort(lockCaps)
	return lockCaps
}

// lockForCapability returns the mutex associated with capability, creating it
// lazily while protecting the shared lock map.
func lockForCapability(capability Capability) *sync.Mutex {
	capabilityLocks.mu.Lock()
	defer capabilityLocks.mu.Unlock()

	lock := capabilityLocks.locks[capability]
	if lock == nil {
		lock = &sync.Mutex{}
		capabilityLocks.locks[capability] = lock
	}
	return lock
}

// gitLabClientUnavailableError indicates that an admin capability probe could
// not run because the E2E GitLab client was not configured.
type gitLabClientUnavailableError struct{}

// Error returns the human-readable reason for the unavailable GitLab client.
func (gitLabClientUnavailableError) Error() string { return "gitlab client unavailable" }
