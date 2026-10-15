//go:build e2e

// main_test.go covers what Main resolves before any test runs: where the
// configuration comes from, in what order, and what a run with nothing
// configured is told.

package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeEnvFile writes a dotenv file and returns its absolute path.
func writeEnvFile(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// TestLoadSettings_ProcessEnvironment_WinsOverEveryFile checks the top of the
// precedence order.
//
// A developer exporting a value means it for this run; a file that overrode it
// would make the exported value look ignored, which is the class of confusion
// that made the old suite's three dotenv spellings hard to reason about.
func TestLoadSettings_ProcessEnvironment_WinsOverEveryFile(t *testing.T) {
	dir := t.TempDir()
	named := writeEnvFile(t, dir, "named.env", "E2E_HARNESS_KEY=from-the-file")
	t.Setenv(envEnvFile, named)
	t.Setenv("E2E_HARNESS_KEY", "from-the-environment")

	resolved, err := loadSettings()
	if err != nil {
		t.Fatalf("loadSettings() error = %v, want nil", err)
	}

	if got := resolved.get("E2E_HARNESS_KEY"); got != "from-the-environment" {
		t.Fatalf("E2E_HARNESS_KEY = %q, want the process environment's value", got)
	}
}

// TestLoadSettings_NamedFile_FillsWhatNothingElseSet checks that the file
// E2E_ENV_FILE names is read at all, and that it only fills gaps.
func TestLoadSettings_NamedFile_FillsWhatNothingElseSet(t *testing.T) {
	dir := t.TempDir()
	named := writeEnvFile(t, dir, "named.env", "E2E_HARNESS_ONLY_IN_FILE=present")
	t.Setenv(envEnvFile, named)

	resolved, err := loadSettings()
	if err != nil {
		t.Fatalf("loadSettings() error = %v, want nil", err)
	}

	if got := resolved.get("E2E_HARNESS_ONLY_IN_FILE"); got != "present" {
		t.Fatalf("E2E_HARNESS_ONLY_IN_FILE = %q, want the file's value", got)
	}
}

// TestLoadSettings_RelativeEnvFile_IsRefused checks the one configuration
// mistake that would otherwise be silent.
//
// A test binary runs in its own package directory, so a relative path names a
// different file for each of the three packages, and the two that do not have
// one would run with a configuration nobody wrote. Refusing says so; resolving
// it against the repository root would hide a typo instead.
func TestLoadSettings_RelativeEnvFile_IsRefused(t *testing.T) {
	t.Setenv(envEnvFile, filepath.Join("test", "e2e", ".env.docker"))

	_, err := loadSettings()

	if err == nil {
		t.Fatal("loadSettings() accepted a relative env file path")
	}
	if !strings.Contains(err.Error(), envEnvFile) || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("the error should name %s and say it must be absolute: %v", envEnvFile, err)
	}
}

// TestLoadSettings_MissingFile_IsNotAnError checks that an absent file is
// simply nothing.
//
// The repository .env is optional and test/e2e/.env.docker only exists once
// the stack has been provisioned, so a run that has neither is an ordinary run
// against an exported GITLAB_URL.
func TestLoadSettings_MissingFile_IsNotAnError(t *testing.T) {
	t.Setenv(envEnvFile, filepath.Join(t.TempDir(), "not-there.env"))

	if _, err := loadSettings(); err != nil {
		t.Fatalf("loadSettings() error = %v, want nil for a file that is not there", err)
	}
}

// TestSettingsOverlay_ExistingKey_IsLeftAlone checks the overlay rule itself:
// a file fills gaps and never replaces.
func TestSettingsOverlay_ExistingKey_IsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	path := writeEnvFile(t, dir, "overlay.env", "KEPT=file", "ADDED=file")
	resolved := settings{values: map[string]string{"KEPT": "environment"}}

	resolved.overlay(path)

	if resolved.get("KEPT") != "environment" {
		t.Fatalf("KEPT = %q, want the value that was already set", resolved.get("KEPT"))
	}
	if resolved.get("ADDED") != "file" {
		t.Fatalf("ADDED = %q, want the file's value", resolved.get("ADDED"))
	}
}

// TestSetting_ReadsWhatMainResolved_WithoutAnInstance checks the reader a
// caller with no Env uses.
//
// It exists for the callers whose question is about the configuration rather
// than about GitLab: New runs the bootstrap, which probes the instance, so a
// test that only wants to know whether a provider key is configured could not
// ask through an Env without first requiring a GitLab it has no use for. What
// it must not become is a second source of truth, so it reads the same map
// Env.Setting reads and the process environment is not it.
func TestSetting_ReadsWhatMainResolved_WithoutAnInstance(t *testing.T) {
	previous := state.settings
	t.Cleanup(func() { state.settings = previous })
	state.settings = testSettings(map[string]string{"MODELEVAL_EXAMPLE": "resolved"})

	if got := Setting("MODELEVAL_EXAMPLE"); got != "resolved" {
		t.Errorf("Setting() = %q, want the value Main resolved", got)
	}
	if got := Setting("MODELEVAL_NOTHING_SET"); got != "" {
		t.Errorf("Setting() = %q for a key nothing set, want the empty string", got)
	}

	// The process environment is deliberately not consulted: the settings are
	// resolved once, and a dotenv file this run reads must not be able to
	// configure the harness's own process, since every server child is
	// launched from an environment built from nothing.
	t.Setenv("MODELEVAL_ONLY_IN_THE_ENVIRONMENT", "leaked")
	if got := Setting("MODELEVAL_ONLY_IN_THE_ENVIRONMENT"); got != "" {
		t.Errorf("Setting() = %q, want the empty string: it reads the resolved settings and not os.Getenv", got)
	}
}

// TestRepoRoot_FromThisPackage_FindsTheModuleRoot checks the anchor every path
// in the harness is resolved from.
func TestRepoRoot_FromThisPackage_FindsTheModuleRoot(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot() error = %v, want nil", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr != nil {
		t.Fatalf("repoRoot() returned %q, which holds no go.mod: %v", root, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, dockerEnvFile)); statErr != nil && !os.IsNotExist(statErr) {
		t.Fatalf("the Docker env file path does not resolve under the root: %v", statErr)
	}
}

// TestPackageName_WorkingDirectory_NamesThePackage checks the name every
// resource this run creates carries.
//
// go test runs a package's binary in that package's own directory, so this is
// "harness" here and "common", "ce" or "ee" for the suites.
func TestPackageName_WorkingDirectory_NamesThePackage(t *testing.T) {
	if got := packageName(); got != "harness" {
		t.Fatalf("packageName() = %q, want %q", got, "harness")
	}
}

// TestMismatchSkips_Setting_TurnsARefusalIntoSkips checks the escape hatch the
// refusal block advertises.
//
// Only a runtime mismatch honors it. A run with no GitLab at all is a fatal
// refusal whatever this says, because a release gate that skipped that would
// pass with nothing having run.
func TestMismatchSkips_Setting_TurnsARefusalIntoSkips(t *testing.T) {
	cases := map[string]bool{"skip": true, "SKIP": true, "": false, "fail": false}
	for value, want := range cases {
		t.Run("value "+value, func(t *testing.T) {
			run := &runState{settings: settings{values: map[string]string{envRuntimeMismatch: value}}}
			if got := run.mismatchSkips(); got != want {
				t.Fatalf("mismatchSkips() with %s=%q = %t, want %t", envRuntimeMismatch, value, got, want)
			}
		})
	}
}

// TestRunStateFinish_FatalRefusal_AlwaysFails checks the exit code a package
// that never ran a test returns.
//
// Two is the exit code a refusal takes, so a CI step can tell it from an
// ordinary test failure. A fatal refusal returns it even with the skip setting
// on, which is what stops the gate passing empty.
func TestRunStateFinish_FatalRefusal_AlwaysFails(t *testing.T) {
	run := &runState{
		kind:     refusalFatal,
		settings: settings{values: map[string]string{envRuntimeMismatch: "skip"}},
	}

	if got := run.finish(0); got != 2 {
		t.Fatalf("finish() = %d, want 2", got)
	}
}

// TestRunStateFinish_Mismatch_HonoursTheSkipSetting checks the other half:
// with the setting on, a mismatch is the run's own exit code, and without it
// the run fails.
func TestRunStateFinish_Mismatch_HonoursTheSkipSetting(t *testing.T) {
	skipping := &runState{kind: refusalMismatch, settings: settings{values: map[string]string{envRuntimeMismatch: "skip"}}}
	if got := skipping.finish(0); got != 0 {
		t.Fatalf("finish() with the skip setting = %d, want 0", got)
	}

	failing := &runState{kind: refusalMismatch, settings: settings{values: map[string]string{}}}
	if got := failing.finish(0); got != 2 {
		t.Fatalf("finish() without the skip setting = %d, want 2", got)
	}
}

// TestPrepare_ReleaseNewerThanTheTable_RefusesBeforePreparingTheInstance is
// issue 1155's self-hosted half: a run pointed at a GitLab newer than the
// release the fine-grained table was recorded from is refused in the
// environment setup with the release rule's own text, and nothing is prepared
// on the instance. The setup runs when the first test asks the harness for
// its instance, so that test reports the refusal, every other one skips, and
// the package exits 2. The refusal is fatal, so the setting that turns a
// runtime mismatch into skips does not excuse it: a stale table is no
// mismatch a run may skip its way past.
func TestPrepare_ReleaseNewerThanTheTable_RefusesBeforePreparingTheInstance(t *testing.T) {
	_, _, newer, _ := releasesAroundTheTable(t)
	stub := startStubGitLabAt(t, newer)
	run := &runState{
		settings: testSettings(map[string]string{
			envGitLabURL: stub.URL, envGitLabToken: stubToken, envRuntimeMismatch: "skip",
		}),
		requirement: Any,
		pkg:         "common",
	}
	captureLog(t)

	run.prepare()

	if run.kind != refusalFatal {
		t.Fatalf("a GitLab newer than the table left the run refused as %d, want a fatal refusal", run.kind)
	}
	if run.inst != nil {
		t.Error("an instance was prepared for a run the release rule stops")
	}
	for _, want := range []string{"GitLab " + newer + " is newer", "make gen-api-live", "make gen-action-grants"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(run.refusal, want) {
				t.Errorf("the refusal does not mention %q:\n%s", want, run.refusal)
			}
		})
	}
	if got := run.finish(0); got != 2 {
		t.Errorf("finish(0) = %d, want 2 for a run the release rule stopped", got)
	}
}

// TestPrepare_ReleaseTheTableRecords_GoesOnToTheRuntimeGuard is the other
// branch: a GitLab on the release the table records passes the release rule,
// says in one line which state its fine-grained sessions are in, and reaches
// the runtime guard, which is what refuses it here, since the stub is an
// unlicensed instance and the package asks for a licensed one. The refusal is
// a mismatch, so it is the guard's and not the release rule's.
func TestPrepare_ReleaseTheTableRecords_GoesOnToTheRuntimeGuard(t *testing.T) {
	recorded, _, _, _ := releasesAroundTheTable(t)
	stub := startStubGitLabAt(t, recorded)
	run := &runState{
		settings:    testSettings(map[string]string{envGitLabURL: stub.URL, envGitLabToken: stubToken}),
		requirement: Licensed,
		pkg:         "ee",
	}
	logged := captureLog(t)

	run.prepare()

	if run.kind != refusalMismatch {
		t.Fatalf("a GitLab on the recorded release was refused as %d, want the runtime guard's mismatch:\n%s", run.kind, run.refusal)
	}
	if strings.Contains(run.refusal, "newer than") {
		t.Errorf("the release rule refused a GitLab on the recorded release:\n%s", run.refusal)
	}
	if want := "GitLab " + recorded + " is in the release the fine-grained action table was recorded from"; !strings.Contains(logged.String(), want) {
		t.Errorf("the log does not say which state the run's fine-grained sessions are in (%q):\n%s", want, logged.String())
	}
}

// TestRunStateFinish_NoRefusal_KeepsTheTestsVerdict checks that a run which
// reached its tests returns what they decided.
func TestRunStateFinish_NoRefusal_KeepsTheTestsVerdict(t *testing.T) {
	run := &runState{settings: settings{values: map[string]string{}}}

	if got := run.finish(1); got != 1 {
		t.Fatalf("finish(1) = %d, want the tests' own code", got)
	}
}
