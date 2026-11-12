// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// access_test.go covers what the fixture library is handed: the accessors on
// Env and the exit hooks Main runs after the last test.

package harness

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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

// TestEnvClientFor_SendsTheCredentialItWasGiven checks that a client built for
// another credential reaches the run's instance with that credential and not
// the run's, which is the whole of what a fixture acting as another user and
// a direct probe of a fine-grained token rely on.
func TestEnvClientFor_SendsTheCredentialItWasGiven(t *testing.T) {
	var sent atomic.Value
	stub := startStubGitLab(t, stubRoute{pattern: "/api/v4/projects", handler: func(w http.ResponseWriter, r *http.Request) {
		sent.Store(r.Header.Get("PRIVATE-TOKEN"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}})
	env := newEnv(t, instanceForStub(t, stub))

	client, err := env.ClientFor("glpat-another")
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if client == env.Client() {
		t.Fatal("ClientFor handed back the run's own client")
	}
	if _, _, err = client.GL().Projects.ListProjects(nil, gl.WithContext(t.Context())); err != nil {
		t.Fatalf("listing projects with the other credential: %v", err)
	}
	if got, _ := sent.Load().(string); got != "glpat-another" {
		t.Errorf("the request carried token %q, want the one ClientFor was given", got)
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

// envOnFlakyLicense returns an Env on a stub that serves a Premium license on
// every read but the ones failing marks, which fail answers, and the count of
// the reads. It is the instance the license test meets just after adding or
// removing a license, when the license can answer 502 for a moment
// ([answerBadGateway]) or not come back at all ([answerNever]). The
// bootstrap's probe makes the first read; the client the Env then asks with
// does not retry, so each ask of the re-probe is one read.
func envOnFlakyLicense(t *testing.T, failing func(read int64) bool, fail http.HandlerFunc) (env *Env, reads *atomic.Int64) {
	t.Helper()
	reads = &atomic.Int64{}
	stub := startStubGitLab(t, stubRoute{pattern: "GET /api/v4/license", handler: func(w http.ResponseWriter, r *http.Request) {
		if failing(reads.Add(1)) {
			fail(w, r)
			return
		}
		writeStubJSON(w, map[string]any{"id": 1, "plan": "premium"})
	}})
	inst := instanceForStub(t, stub)
	client, err := gitlabclient.NewClientWithTokenRetries(stub.URL, stubToken, false, true)
	if err != nil {
		t.Fatalf("building a client for the stub: %v", err)
	}
	inst.client = client
	return newEnv(t, inst), reads
}

// answerBadGateway answers a read 502, as a license just added or removed can
// for a moment.
func answerBadGateway(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusBadGateway)
}

// answerNever holds a read until its asker gives up, as a license that does
// not come back at all.
func answerNever(_ http.ResponseWriter, r *http.Request) {
	<-r.Context().Done()
}

// TestEnvReprobeTier_ALicenseThatAnswersLate_IsAskedAgain checks that the
// re-probe reads the tier the way the bootstrap does: a license that answers
// 502 just after the license test changed it, and then answers, is asked
// again until it does, and the tier read is the instance's Premium rather
// than the Free a single ask would have compared with the bootstrap's.
func TestEnvReprobeTier_ALicenseThatAnswersLate_IsAskedAgain(t *testing.T) {
	shortTierRetryPause(t)
	// The bootstrap's read is the first; the two after it are the re-probe's
	// first two asks, which GitLab leaves unanswered.
	env, reads := envOnFlakyLicense(t, func(read int64) bool { return read == 2 || read == 3 }, answerBadGateway)

	now, before := env.ReprobeTier()

	if now != edition.Premium || before != edition.Premium {
		t.Errorf("ReprobeTier() = (%s, %s) after the license answered 502 twice and then answered, want (premium, premium)", now, before)
	}
	if got := reads.Load(); got != 1+tierAttempts {
		t.Errorf("license reads = %d, want the bootstrap's one and the re-probe's %d", got, tierAttempts)
	}
}

// reprobeLicenseNeverAnswers is a fatal case
// [TestEnvReprobeTier_ALicenseThatNeverAnswers_FailsTheTestNamingIt] runs in
// a child process: the bootstrap reads a Premium license, and every read the
// re-probe makes after it is answered 502.
func reprobeLicenseNeverAnswers(t *testing.T) {
	t.Helper()
	shortTierRetryPause(t)
	env, _ := envOnFlakyLicense(t, func(read int64) bool { return read > 1 }, answerBadGateway)
	env.ReprobeTier()
}

// reprobeLicenseHangs is the other fatal case
// [TestEnvReprobeTier_ALicenseThatNeverAnswers_FailsTheTestNamingIt] runs in
// a child process: the bootstrap reads a Premium license, every read the
// re-probe makes after it hangs until its asker gives up, and the test's
// context ends after a moment, standing in for the re-probe's budget running
// out on a license that never comes back.
func reprobeLicenseHangs(t *testing.T) {
	t.Helper()
	env, _ := envOnFlakyLicense(t, func(read int64) bool { return read > 1 }, answerNever)
	ctx, cancel := context.WithTimeout(env.Ctx, 200*time.Millisecond)
	t.Cleanup(cancel)
	env.Ctx = ctx
	env.ReprobeTier()
}

// TestEnvReprobeTier_ALicenseThatNeverAnswers_FailsTheTestNamingIt checks
// the re-probe's refusal: a license GitLab keeps leaving unanswered, with a
// 502 on every ask or with no answer before the re-probe's time runs out,
// fails the test calling it, naming the steps left unread and why it stopped
// asking, rather than handing back the provisional Free for a comparison that
// would blame the license test for a license it never lost.
func TestEnvReprobeTier_ALicenseThatNeverAnswers_FailsTheTestNamingIt(t *testing.T) {
	cases := []struct {
		name      string
		fatalCase string
		want      string
	}{
		{
			name:      "answered 502 on every ask",
			fatalCase: "reprobe license never answers",
			want:      "re-reading the instance's tier: the tier probe left license unanswered " + strconv.Itoa(tierAttempts) + " times in a row",
		},
		{
			// The namespaces are asked after the license, on a context the
			// hanging read has already spent, so they go unread too.
			name:      "no answer before the time ran out",
			fatalCase: "reprobe license hangs",
			want:      "re-reading the instance's tier: the tier probe left license, namespaces unanswered and the time to ask again ran out: context deadline exceeded",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runFatalCase(t, tc.fatalCase)

			if !strings.Contains(out, tc.want) {
				t.Errorf("the child printed:\n%s\nwant %q", out, tc.want)
			}
		})
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
