//go:build e2e

// access_test.go covers what the fixture library is handed: the accessors on
// Env and the exit hooks Main runs after the last test.

package harness

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// TestEnvAccessors_StubInstance_HandOverWhatTheProbeFound checks that an Env
// exposes the client, the facts and the configuration the bootstrap resolved,
// which is everything a fixture built through client-go needs.
//
// It drives the real probe against a stub GitLab, so the facts are what a run
// would see rather than a literal the test wrote into the struct.
func TestEnvAccessors_StubInstance_HandOverWhatTheProbeFound(t *testing.T) {
	inst := stubInstance(t)
	inst.settings = inst.settings.with("E2E_FIXTURE_URL", "http://e2e-fixture:8080")
	inst.settings = inst.settings.with(envMode, "docker")
	env := newEnv(t, inst)

	if env.Client() != inst.client {
		t.Errorf("Client() is not the instance's client")
	}
	if got := env.Setting("E2E_FIXTURE_URL"); got != "http://e2e-fixture:8080" {
		t.Errorf("Setting(E2E_FIXTURE_URL) = %q, want the configured value", got)
	}
	if got := env.Setting("E2E_NOT_SET"); got != "" {
		t.Errorf("Setting(unset) = %q, want empty", got)
	}
	if !env.DockerMode() {
		t.Errorf("DockerMode() = false, want true with %s=docker", envMode)
	}
	if got := env.Package(); got != "harness" {
		t.Errorf("Package() = %q, want harness", got)
	}
	if !env.HasRunner() {
		t.Errorf("HasRunner() = false, want true in Docker mode")
	}

	runtime := env.Runtime()
	if runtime.URL != inst.facts.URL || runtime.Username != inst.facts.Username || runtime.UserID != inst.facts.UserID {
		t.Errorf("Runtime() = %+v, want the probed facts %+v", runtime, inst.facts)
	}
	if runtime.Tier != edition.Free || runtime.TierConfirmed {
		t.Errorf("Runtime().Tier = %s (confirmed=%t), want Free from a stub with no license", runtime.Tier, runtime.TierConfirmed)
	}
}

// TestEnvRuntime_Scopes_AreACopy checks that a caller changing the scopes it
// was handed does not change what the harness holds, since the served-set
// check reads the harness's own copy.
func TestEnvRuntime_Scopes_AreACopy(t *testing.T) {
	inst := stubInstance(t)
	inst.facts.Scopes = []string{"api", "read_user"}
	env := newEnv(t, inst)

	scopes := env.Runtime().Scopes
	if len(scopes) != 2 {
		t.Fatalf("Runtime().Scopes = %v, want the two probed scopes", scopes)
	}
	scopes[0] = "changed"

	if got := inst.facts.Scopes[0]; got != "api" {
		t.Errorf("the harness's scopes were changed through the copy: %q", got)
	}
}

// TestEnvRepoRoot_FromAPackageDirectory_IsTheModuleRoot checks that the root
// a fixture resolves a file from is the directory holding go.mod, whatever
// package directory the test binary runs in.
func TestEnvRepoRoot_FromAPackageDirectory_IsTheModuleRoot(t *testing.T) {
	env := newEnv(t, stubInstance(t))

	root := env.RepoRoot()
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Errorf("RepoRoot() = %q, which holds no go.mod: %v", root, err)
	}
	if _, err := os.Stat(filepath.Join(root, "test", "e2e", "internal", "harness")); err != nil {
		t.Errorf("RepoRoot() = %q, under which this package is not found: %v", root, err)
	}
}

// TestEnvReprobeTier_AnswersFromTheInstance_NotFromTheRecord checks that a
// re-probe asks the instance again and leaves the harness's own record as it
// was: a license test that changed the tier must be told so by the answer,
// and must not be able to make the change the new baseline by asking.
func TestEnvReprobeTier_AnswersFromTheInstance_NotFromTheRecord(t *testing.T) {
	licensed := atomic.Bool{}
	licensed.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"version": "18.0.0", "revision": "abcdef", "enterprise": true})
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"id": 7, "username": "harness", "name": "Harness", "is_admin": true})
	})
	mux.HandleFunc("/api/v4/license", func(w http.ResponseWriter, _ *http.Request) {
		if !licensed.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeStubJSON(w, map[string]any{"id": 1, "plan": "premium"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	stub := httptest.NewServer(mux)
	t.Cleanup(stub.Close)

	env := newEnv(t, instanceForStub(t, stub))

	now, before := env.ReprobeTier()
	if now != edition.Premium || before != edition.Premium {
		t.Fatalf("ReprobeTier() = (%s, %s) on a Premium stub, want (premium, premium)", now, before)
	}

	// The license goes away underneath the run: the re-probe reports it, and
	// the record the next test reads still says Premium.
	licensed.Store(false)
	now, before = env.ReprobeTier()
	if now != edition.Free {
		t.Errorf("ReprobeTier() now = %s after the license was removed, want free", now)
	}
	if before != edition.Premium || env.Runtime().Tier != edition.Premium {
		t.Errorf("ReprobeTier() before = %s and Runtime().Tier = %s, want the bootstrap's premium in both: a re-probe must not rewrite the record", before, env.Runtime().Tier)
	}
}

// TestExitHooks_Registered_RunOnceLastFirst checks the two properties a
// cleanup registry needs: the hooks run in reverse order, so the World's
// teardown runs before the sweep that would find its leftovers, and a second
// run runs nothing.
func TestExitHooks_Registered_RunOnceLastFirst(t *testing.T) {
	resetExitHooks(t)

	var order []string
	AtExit(func() error { order = append(order, "sweep"); return nil })
	AtExit(func() error { order = append(order, "world"); return nil })

	if failures := runExitHooks(); len(failures) != 0 {
		t.Fatalf("runExitHooks() failures = %v, want none", failures)
	}
	if got, want := strings.Join(order, ","), "world,sweep"; got != want {
		t.Errorf("hook order = %q, want %q", got, want)
	}

	order = nil
	if failures := runExitHooks(); len(failures) != 0 || len(order) != 0 {
		t.Errorf("second runExitHooks() ran %v with failures %v, want nothing", order, failures)
	}
}

// TestExitHooks_LateRegistration_IsDropped checks that a hook registered after
// the hooks ran is not kept for a run that will never happen.
func TestExitHooks_LateRegistration_IsDropped(t *testing.T) {
	resetExitHooks(t)
	runExitHooks()

	ran := false
	AtExit(func() error { ran = true; return nil })

	hooks.mu.Lock()
	kept := len(hooks.fns)
	hooks.mu.Unlock()
	if kept != 0 || ran {
		t.Errorf("late hook kept=%d ran=%t, want dropped", kept, ran)
	}
}

// TestExitHooks_Failure_FailsAPassingRun checks that a hook's error reaches
// the exit code: a World somebody mutated, or a sweep that could not delete,
// must not leave a run looking green.
func TestExitHooks_Failure_FailsAPassingRun(t *testing.T) {
	resetExitHooks(t)

	AtExit(func() error { return nil })
	AtExit(func() error { return errors.New("the World was changed") })
	AtExit(func() error { return nil })

	failures := runExitHooks()
	if len(failures) != 1 {
		t.Fatalf("runExitHooks() failures = %v, want exactly the one hook that failed", failures)
	}

	cases := []struct {
		name string
		code int
		want int
	}{
		{name: "passing run is failed", code: 0, want: 1},
		{name: "failing run keeps its code", code: 1, want: 1},
		{name: "refused run keeps its code", code: 2, want: 2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := exitCodeAfterHooks(testCase.code, failures); got != testCase.want {
				t.Errorf("exitCodeAfterHooks(%d, one failure) = %d, want %d", testCase.code, got, testCase.want)
			}
		})
	}
	if got := exitCodeAfterHooks(0, nil); got != 0 {
		t.Errorf("exitCodeAfterHooks(0, none) = %d, want 0", got)
	}
}

// resetExitHooks gives a test an empty registry and puts the package's own
// back afterwards, so the harness's real hooks are neither run nor lost.
func resetExitHooks(t *testing.T) {
	t.Helper()
	hooks.mu.Lock()
	saved := hooks.fns
	savedDone := hooks.done
	hooks.fns = nil
	hooks.done = false
	hooks.mu.Unlock()
	t.Cleanup(func() {
		hooks.mu.Lock()
		hooks.fns = saved
		hooks.done = savedDone
		hooks.mu.Unlock()
	})
}
