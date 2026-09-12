//go:build e2e

// runtime_test.go covers the guard: which instances each requirement accepts,
// and what a refusal tells the person reading it.

package harness

import (
	"errors"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// TestGuardMessage_EveryRuntime_RefusesOnlyTheWrongOnes is the table the guard
// exists for: three requirements against the three runtimes this suite runs
// on.
//
// The unlicensed EE image is the row worth having. It reports Enterprise
// Edition on its version endpoint and has no license, so a rule written on the
// edition flag would send the licensed package at it and the Free package away
// from it, both wrong. The tier is what decides, and this pins that.
func TestGuardMessage_EveryRuntime_RefusesOnlyTheWrongOnes(t *testing.T) {
	ce := runtimeFacts{URL: "http://gitlab.test", Version: "18.0.0", Tier: edition.Free}
	eeUnlicensed := runtimeFacts{URL: "http://gitlab.test", Version: "18.0.0-ee", Enterprise: true, Tier: edition.Free}
	eeLicensed := runtimeFacts{
		URL: "http://gitlab.test", Version: "18.0.0-ee", Enterprise: true,
		Tier: edition.Ultimate, TierConfirmed: true,
	}

	cases := []struct {
		name    string
		facts   runtimeFacts
		req     Requirement
		refused bool
	}{
		{name: "any on a CE image", facts: ce, req: Any},
		{name: "any on an unlicensed EE image", facts: eeUnlicensed, req: Any},
		{name: "any on a licensed instance", facts: eeLicensed, req: Any},
		{name: "free on a CE image", facts: ce, req: Free},
		{name: "free on an unlicensed EE image", facts: eeUnlicensed, req: Free},
		{name: "free on a licensed instance", facts: eeLicensed, req: Free, refused: true},
		{name: "licensed on a CE image", facts: ce, req: Licensed, refused: true},
		{name: "licensed on an unlicensed EE image", facts: eeUnlicensed, req: Licensed, refused: true},
		{name: "licensed on a licensed instance", facts: eeLicensed, req: Licensed},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			message := guardMessage(testCase.facts, testCase.req, "common")
			if testCase.refused && message == "" {
				t.Fatalf("%s was accepted, want a refusal", testCase.name)
			}
			if !testCase.refused && message != "" {
				t.Fatalf("%s was refused:\n%s", testCase.name, message)
			}
		})
	}
}

// TestGuardMessage_Refusal_NamesWhatToDoAboutIt checks the content of the
// block, which is the whole value of the guard.
//
// A refusal that only said "wrong runtime" would be no better than the wall of
// 404s it replaces. It names the instance, its version and edition, the tier
// it found, what the package needs, the target that provides it and the escape
// hatch, so the reader can act without opening the plan.
func TestGuardMessage_Refusal_NamesWhatToDoAboutIt(t *testing.T) {
	facts := runtimeFacts{URL: "http://gitlab.test:8929", Version: "18.0.0", Tier: edition.Free}

	message := guardMessage(facts, Licensed, "ee")

	for _, fragment := range []string{
		"ee package",
		"http://gitlab.test:8929",
		"18.0.0",
		"Community Edition",
		"free (no license found)",
		"licensed (Premium or Ultimate)",
		"make test-e2e-ee",
		envRuntimeMismatch + "=skip",
	} {
		t.Run(fragment, func(t *testing.T) {
			if !strings.Contains(message, fragment) {
				t.Fatalf("the refusal does not name %q:\n%s", fragment, message)
			}
		})
	}
}

// TestGuardMessage_FreeRefusal_PointsAtTheUnlicensedTarget checks the other
// direction, where the reader is holding a licensed instance and the package
// needs one without a license.
func TestGuardMessage_FreeRefusal_PointsAtTheUnlicensedTarget(t *testing.T) {
	facts := runtimeFacts{URL: "http://gitlab.test", Version: "18.0.0-ee", Enterprise: true, Tier: edition.Premium, TierConfirmed: true}

	message := guardMessage(facts, Free, "ce")

	if !strings.Contains(message, "      make test-e2e-ce\n") {
		t.Fatalf("the refusal should point at the unlicensed target:\n%s", message)
	}
	if !strings.Contains(message, "premium (license)") {
		t.Fatalf("the refusal should say the license was found:\n%s", message)
	}
}

// TestRequirementString_EveryValue_NamesItself checks that a requirement reads
// as itself in the refusal block, including the value nothing should produce.
func TestRequirementString_EveryValue_NamesItself(t *testing.T) {
	cases := map[Requirement]string{
		Any:             "any runtime",
		Free:            "free",
		Licensed:        "licensed",
		Requirement(99): "unknown",
	}
	for req, want := range cases {
		t.Run(want, func(t *testing.T) {
			if got := req.String(); !strings.Contains(got, want) {
				t.Fatalf("Requirement(%d).String() = %q, want it to contain %q", req, got, want)
			}
		})
	}
}

// TestRequirementSatisfies_UnknownValue_AcceptsNothing checks that a
// requirement nothing produces refuses rather than admits.
//
// The default of a switch over a small enum is the branch a future value lands
// in, and admitting by default would run a package against a runtime nobody
// had checked.
func TestRequirementSatisfies_UnknownValue_AcceptsNothing(t *testing.T) {
	if Requirement(99).satisfies(runtimeFacts{Tier: edition.Ultimate}) {
		t.Fatal("an unknown requirement accepted an instance")
	}
}

// TestMissingCredentialsMessage_NoInstance_NamesEveryConfigurationSource
// checks what a run with nothing configured is told.
//
// This is the first thing a new contributor sees, so it lists the four places
// the configuration can come from and their order rather than naming one.
func TestMissingCredentialsMessage_NoInstance_NamesEveryConfigurationSource(t *testing.T) {
	message := missingCredentialsMessage("common")

	for _, fragment := range []string{"common package", envGitLabURL, envGitLabToken, envEnvFile, ".env.docker", ".env"} {
		t.Run(fragment, func(t *testing.T) {
			if !strings.Contains(message, fragment) {
				t.Fatalf("the message does not name %q:\n%s", fragment, message)
			}
		})
	}
}

// TestUnreachableMessage_FailedProbe_NamesTheInstanceAndNotTheToken checks
// that a connection failure says which instance would not answer and never
// quotes the credential.
func TestUnreachableMessage_FailedProbe_NamesTheInstanceAndNotTheToken(t *testing.T) {
	message := unreachableMessage("ce", "http://gitlab.test", errors.New("connection refused"))

	if !strings.Contains(message, "http://gitlab.test") || !strings.Contains(message, "connection refused") {
		t.Fatalf("the message should name the instance and the error:\n%s", message)
	}
	if strings.Contains(strings.ToLower(message), "token") {
		t.Fatalf("the message should not mention the credential:\n%s", message)
	}
}

// TestSnapshotDifferences_MissingAndRenamed_ReportsBoth checks the guard that
// protects an instance the run does not own.
//
// A deleted resource and a renamed one are different failures and both are
// worth the words: a rename is usually a test that took a name it should have
// scoped to its run.
func TestSnapshotDifferences_MissingAndRenamed_ReportsBoth(t *testing.T) {
	before := &resourceSnapshot{
		groups:   map[int64]string{1: "team", 2: "kept"},
		projects: map[int64]string{10: "team/app", 11: "team/kept"},
	}
	current := &resourceSnapshot{
		groups:   map[int64]string{2: "kept-renamed"},
		projects: map[int64]string{10: "team/app", 11: "team/kept"},
	}

	changes := snapshotDifferences(before, current)

	if len(changes) != 2 {
		t.Fatalf("changes = %v, want the missing group and the renamed one", changes)
	}
	joined := strings.Join(changes, "\n")
	if !strings.Contains(joined, `group "team" (ID=1): missing`) {
		t.Fatalf("the missing group is not reported:\n%s", joined)
	}
	if !strings.Contains(joined, `group ID=2 renamed: "kept" to "kept-renamed"`) {
		t.Fatalf("the renamed group is not reported:\n%s", joined)
	}
}

// TestSnapshotDifferences_ProjectChanges_ReportsThem checks the same two
// classes for projects, which is what a run creates most of.
func TestSnapshotDifferences_ProjectChanges_ReportsThem(t *testing.T) {
	before := &resourceSnapshot{
		groups:   map[int64]string{},
		projects: map[int64]string{10: "team/app", 11: "team/lib"},
	}
	current := &resourceSnapshot{
		groups:   map[int64]string{},
		projects: map[int64]string{11: "team/lib-renamed"},
	}

	changes := strings.Join(snapshotDifferences(before, current), "\n")

	if !strings.Contains(changes, `project "team/app" (ID=10): missing`) {
		t.Fatalf("the missing project is not reported:\n%s", changes)
	}
	if !strings.Contains(changes, `project ID=11 renamed: "team/lib" to "team/lib-renamed"`) {
		t.Fatalf("the renamed project is not reported:\n%s", changes)
	}
}

// TestSnapshotDifferences_NothingChanged_ReportsNothing checks the passing
// case, which is what every run against somebody's own instance should
// produce.
func TestSnapshotDifferences_NothingChanged_ReportsNothing(t *testing.T) {
	snapshot := &resourceSnapshot{groups: map[int64]string{1: "team"}, projects: map[int64]string{10: "team/app"}}

	if changes := snapshotDifferences(snapshot, snapshot); len(changes) != 0 {
		t.Fatalf("changes = %v, want none", changes)
	}
}

// TestInstanceCleanupBudget_LicensedInstance_GetsLonger checks the budget a
// test's undo work runs under.
//
// A licensed instance deletes projects and groups measurably more slowly, and
// a cleanup cut short leaves resources the next run trips over.
func TestInstanceCleanupBudget_LicensedInstance_GetsLonger(t *testing.T) {
	free := &instance{facts: runtimeFacts{Tier: edition.Free}}
	licensed := &instance{facts: runtimeFacts{Tier: edition.Ultimate}}

	if free.cleanupBudget() != cleanupBudget {
		t.Fatalf("free budget = %s, want %s", free.cleanupBudget(), cleanupBudget)
	}
	if licensed.cleanupBudget() != enterpriseCleanupBudget {
		t.Fatalf("licensed budget = %s, want %s", licensed.cleanupBudget(), enterpriseCleanupBudget)
	}
}

// TestInstanceDockerMode_ModeSetting_DecidesIt checks the one switch that
// turns the snapshot guard off and the API warm-up on.
func TestInstanceDockerMode_ModeSetting_DecidesIt(t *testing.T) {
	cases := map[string]bool{"docker": true, "Docker": true, "": false, "self-hosted": false}
	for value, want := range cases {
		t.Run("mode "+value, func(t *testing.T) {
			inst := testInstance(runtimeFacts{}, map[string]string{envMode: value})
			if got := inst.dockerMode(); got != want {
				t.Fatalf("dockerMode() with %s=%q = %t, want %t", envMode, value, got, want)
			}
		})
	}
}

// TestSlicesContainError_OneFailedProbe_IsEnough checks the warm-up's own
// verdict: a round with any dropped connection is not a stable round.
func TestSlicesContainError_OneFailedProbe_IsEnough(t *testing.T) {
	if slicesContainError([]error{nil, nil, nil}) {
		t.Fatal("a round with no failures was reported as failed")
	}
	if !slicesContainError([]error{nil, errors.New("connection reset"), nil}) {
		t.Fatal("a round with one dropped connection was reported as clean")
	}
}
