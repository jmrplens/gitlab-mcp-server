// case_fixtures_project_test.go covers the typed fixtures used by the base
// mutating Docker preset: shared resource builders, attempt-name derivation,
// and prompt-template rendering for live fixtures.

package evaluator

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
)

// TestBaseMutatingFixtureSpecs_DefineRequiredResourceBuilders verifies that
// every base mutating fixture ships Ensure, Validate, and Cleanup callbacks
// so case prompts can rely on stable resource lifecycles.
//
// The test enumerates the expected fixture names and asserts each one is
// registered in the base mutating registry with non-nil callbacks. This
// protects the evaluator from silently shipping a fixture whose validate or
// cleanup path was never wired up.
func TestBaseMutatingFixtureSpecs_DefineRequiredResourceBuilders(t *testing.T) {
	fixtures := baseMutatingFixtureSpecs()
	byName := requireFixtureNames(fixtures)
	for _, name := range []string{
		"bootstrap_project",
		"branch",
		"file",
		"issue",
		"merge_request",
		"release",
		"tag",
		"ci_variable",
		"hook",
		"badge",
		"wiki",
		"snippet",
		"feature_flag",
		"deploy_token",
		"deploy_key",
		"package",
		"package_release",
		"pipeline_trigger",
		"pipeline_schedule",
		"member",
	} {
		t.Run(name, func(t *testing.T) {
			fixture, ok := byName[name]
			if !ok {
				t.Fatalf("fixture %q missing from %s", name, fixtureNames(fixtures))
			}
			if fixture.Ensure == nil || fixture.Validate == nil || fixture.Cleanup == nil {
				t.Fatalf("fixture %q callbacks = ensure:%t validate:%t cleanup:%t", name, fixture.Ensure != nil, fixture.Validate != nil, fixture.Cleanup != nil)
			}
		})
	}
}

// TestFixtureOutputFromLiveState_ExposesTypedPromptValues verifies that
// fixtureOutputFromLiveState exposes every typed prompt value used by case
// templates, formatting integers and string slices into the canonical
// pipe-joined representation.
//
// The test builds a populated liveFixtureState and asserts each typed key
// maps to the expected string form, including the comma-joined
// package_release_files slice. This protects case prompts from regressing to
// raw integer or unserialised slice values.
func TestFixtureOutputFromLiveState_ExposesTypedPromptValues(t *testing.T) {
	output := fixtureOutputFromLiveState(&liveFixtureState{
		ProjectID:             123,
		ProjectPath:           liveFixtureProjectPath,
		DefaultBranch:         liveFixtureDefaultRef,
		GroupID:               45,
		IssueIID:              7,
		MergeRequestIID:       8,
		PackageReleaseName:    liveFixturePackageReleaseName,
		PackageReleaseVersion: liveFixturePackageReleaseVersion,
		PackageReleaseTag:     liveFixturePackageReleaseTag,
		PackageReleaseDir:     "/tmp/pkg",
		PackageReleaseFiles:   []string{"a.txt", "b.txt"},
	})
	for key, want := range map[string]string{
		"project_id":              "123",
		"project_path":            liveFixtureProjectPath,
		"group_path":              "",
		"default_branch":          liveFixtureDefaultRef,
		"group_id":                "45",
		"issue_iid":               "7",
		"merge_request_iid":       "8",
		"package_release_name":    liveFixturePackageReleaseName,
		"package_release_version": liveFixturePackageReleaseVersion,
		"package_release_tag":     liveFixturePackageReleaseTag,
		"package_release_dir":     "/tmp/pkg",
		"package_release_files":   "a.txt,b.txt",
	} {
		t.Run(key, func(t *testing.T) {
			if got := output[key]; got != want {
				t.Fatalf("output[%s] = %q, want %q", key, got, want)
			}
		})
	}
}

// TestAttemptNameFixtureOutput_UsesModelRunSuffix verifies that
// attemptNameFixtureOutput composes every attempt-scoped name from the
// model label, run index, and run suffix in a stable, GitLab-friendly form.
//
// The test supplies a qwen model with run index 3 and suffix "abc123" and
// asserts each output key (subgroup name, branch name, variable key, package
// release tag, and so on) matches the expected composed string. This
// protects case prompts from generating unsafe or colliding resource names
// when the same fixture is reused across attempts.
func TestAttemptNameFixtureOutput_UsesModelRunSuffix(t *testing.T) {
	output := attemptNameFixtureOutput(FixtureContext{ModelName: "qwen:qwen3.6-flash", RunIndex: 3, RunSuffix: "abc123"})
	for key, want := range map[string]string{
		"attempt_suffix":           "qwen36flash-r3-abc123",
		"subgroup_name":            "eval-temp-qwen36flash-r3-abc123",
		"mr_source_branch":         "feature/eval-qwen36flash-r3-abc123",
		"file_path":                "tmp/eval.txt-qwen36flash-r3-abc123",
		"ci_variable_key":          "EVAL_TOKEN_qwen36flash_r3_abc123",
		"group_ci_variable_key":    "GROUP_EVAL_TOKEN_qwen36flash_r3_abc123",
		"instance_ci_variable_key": "INSTANCE_EVAL_TOKEN_qwen36flash_r3_abc123",
		"package_release_name":     "eval-release-package-qwen36flash-r3-abc123",
		"package_release_tag":      "v0.0.0-eval-packages-qwen36flash-r3-abc123",
	} {
		t.Run(key, func(t *testing.T) {
			if got := output[key]; got != want {
				t.Fatalf("output[%s] = %q, want %q", key, got, want)
			}
		})
	}
}

// TestAttemptNameFixtureOutput_IsolatesCaseResources verifies that
// attemptNameFixtureOutput appends the case ID to release-related values so
// per-case resources do not collide across attempts that share a model and
// run suffix.
//
// The test sets CaseID on the FixtureContext and asserts the release tag and
// link names include the "ms018" case suffix. This protects destructive
// release workflows from accidentally overwriting shared release artifacts.
func TestAttemptNameFixtureOutput_IsolatesCaseResources(t *testing.T) {
	output := attemptNameFixtureOutput(FixtureContext{ModelName: "qwen:qwen3.6-flash", RunIndex: 1, RunSuffix: "abc123", CaseID: "MS-018"})
	for key, want := range map[string]string{
		"attempt_suffix":    "qwen36flash-r1-abc123-ms018",
		"release_tag_name":  "v0.0.0-eval-qwen36flash-r1-abc123-ms018",
		"release_link_name": "eval-crud-link-qwen36flash-r1-abc123-ms018",
	} {
		t.Run(key, func(t *testing.T) {
			if got := output[key]; got != want {
				t.Fatalf("output[%s] = %q, want %q", key, got, want)
			}
		})
	}
}

// TestBaseMutatingPromptTemplate_RendersAttemptNamesWithoutChangingStoredPrompt
// verifies that rendering a case prompt with attempt-scoped output values
// produces the expected substituted text while leaving the stored prompt
// template unchanged.
//
// The test loads MT-036, builds an attempt-scoped FixtureOutput, renders the
// case prompt, and asserts the rendered string contains the expected tag
// name, project path, and default branch. This protects the prompt-template
// pipeline from accidentally mutating the stored [Case.PromptTemplate.Text]
// when substituting attempt-scoped values.
func TestBaseMutatingPromptTemplate_RendersAttemptNamesWithoutChangingStoredPrompt(t *testing.T) {
	evalCase, ok := CaseByID("MT-036")
	if !ok {
		t.Fatal("CaseByID(MT-036) = false")
	}
	task := taskFromCase(evalCase)
	if task.Prompt != "Create release with tag_name `v0.0.0-eval`, ref `main`, and name `v0.0.0-eval` in project `my-org/tools/gitlab-mcp-server`." {
		t.Fatalf("stored prompt = %q", task.Prompt)
	}
	output := attemptNameFixtureOutput(FixtureContext{ModelName: "openai:gpt-5.4-mini", RunIndex: 1, RunSuffix: "abc123"})
	output["project_path"] = liveFixtureProjectPath
	output["default_branch"] = liveFixtureDefaultRef
	rendered, err := RenderCasePrompt(evalCase, output)
	if err != nil {
		t.Fatalf("RenderCasePrompt() error = %v", err)
	}
	for _, want := range []string{"v0.0.0-eval-gpt54mini-r1-abc123", liveFixtureProjectPath, liveFixtureDefaultRef} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(rendered, want) {
				t.Fatalf("rendered prompt = %q, want %q", rendered, want)
			}
		})
	}
}

// TestFixtureOutputCache_ReusesClonedOutputForSameKey verifies that the
// fixture output cache stores one cloned [FixtureOutput] per idempotency key
// and returns defensive copies so caller mutations cannot poison later
// attempts.
//
// The test increments a counter inside the ensure callback and confirms the
// counter only fires once even when two callers ask for the same key. It
// then mutates the first returned map and asserts that a subsequent lookup
// returns the original cloned value, not the mutated one. This protects
// case attempts from accidentally clobbering shared fixture state.
func TestFixtureOutputCache_ReusesClonedOutputForSameKey(t *testing.T) {
	cache := newFixtureOutputCache()
	var calls int
	output, err := cache.ensure("case:key", func() (FixtureOutput, error) {
		calls++
		return FixtureOutput{"project_id": "123"}, nil
	})
	if err != nil {
		t.Fatalf("first ensure error = %v", err)
	}
	output["project_id"] = "mutated"
	second, err := cache.ensure("case:key", func() (FixtureOutput, error) {
		calls++
		return FixtureOutput{"project_id": "456"}, nil
	})
	if err != nil {
		t.Fatalf("second ensure error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("ensure calls = %d, want 1", calls)
	}
	if got := second["project_id"]; got != "123" {
		t.Fatalf("cached project_id = %q, want original cloned value", got)
	}
}

// TestValidateAttemptNameFixtureOutput_RequiresEveryAttemptName verifies the
// attempt-name fixture validator accepts a complete output and names the first
// missing key, so a case prompt never interpolates a blank resource name.
func TestValidateAttemptNameFixtureOutput_RequiresEveryAttemptName(t *testing.T) {
	complete := attemptNameFixtureOutput(FixtureContext{ModelName: "model", RunIndex: 1, RunSuffix: "suffix"})
	if err := validateAttemptNameFixtureOutput(t.Context(), FixtureContext{}, complete); err != nil {
		t.Fatalf("validateAttemptNameFixtureOutput(complete) error = %v", err)
	}
	incomplete := maps.Clone(complete)
	incomplete["wiki_title"] = "  "
	err := validateAttemptNameFixtureOutput(t.Context(), FixtureContext{}, incomplete)
	if err == nil || !strings.Contains(err.Error(), `missing output "wiki_title"`) {
		t.Fatalf("validateAttemptNameFixtureOutput(incomplete) error = %v, want the missing key", err)
	}
}

// TestValidateLiveCaseFixtureOutput_RequiresProjectOrStandaloneResource
// verifies a live fixture output is accepted when it names a project or a
// standalone resource and rejected when it names neither.
func TestValidateLiveCaseFixtureOutput_RequiresProjectOrStandaloneResource(t *testing.T) {
	cases := []struct {
		name    string
		output  FixtureOutput
		wantErr bool
	}{
		{name: "project", output: FixtureOutput{"project_id": "101"}},
		{name: "snippet", output: FixtureOutput{"snippet_id": "9"}},
		{name: "neither", output: FixtureOutput{"branch_name": "feature/x"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLiveCaseFixtureOutput(t.Context(), FixtureContext{}, tc.output)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateLiveCaseFixtureOutput(%v) error = %v, want error = %t", tc.output, err, tc.wantErr)
			}
		})
	}
}

// TestNoopCaseFixtureCleanup_ReturnsNil verifies the shared cleanup used by
// fixtures that leave their resources in place reports no error.
func TestNoopCaseFixtureCleanup_ReturnsNil(t *testing.T) {
	if err := noopCaseFixtureCleanup(t.Context(), FixtureContext{}, FixtureOutput{}); err != nil {
		t.Fatalf("noopCaseFixtureCleanup() error = %v, want nil", err)
	}
}

// TestFixtureNames_RendersRegisteredNames verifies the diagnostic used when a
// fixture lookup misses lists the registered names.
func TestFixtureNames_RendersRegisteredNames(t *testing.T) {
	got := fixtureNames([]CaseFixtureSpec{{Name: "branch"}, {Name: "issue"}})
	if got != "[branch issue]" {
		t.Fatalf("fixtureNames() = %q, want [branch issue]", got)
	}
}

// TestFixtureOutputFromLiveState_NilState_ReturnsEmptyOutput verifies a
// missing preparer state yields an empty output rather than panicking.
func TestFixtureOutputFromLiveState_NilState_ReturnsEmptyOutput(t *testing.T) {
	if got := fixtureOutputFromLiveState(nil); len(got) != 0 {
		t.Fatalf("fixtureOutputFromLiveState(nil) = %v, want empty output", got)
	}
}

// TestFormatInt64_ZeroBecomesEmpty verifies unset numeric fixture identifiers
// render as an empty string so validation catches them.
func TestFormatInt64_ZeroBecomesEmpty(t *testing.T) {
	cases := []struct {
		name  string
		value int64
		want  string
	}{
		{name: "zero", value: 0, want: ""},
		{name: "positive", value: 42, want: "42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatInt64(tc.value); got != tc.want {
				t.Fatalf("formatInt64(%d) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestNewLiveCaseFixturePreparer_WithoutClient_ReturnsError verifies the typed
// fixture preparer refuses to bootstrap without a GitLab client.
func TestNewLiveCaseFixturePreparer_WithoutClient_ReturnsError(t *testing.T) {
	if _, err := newLiveCaseFixturePreparer(t.Context(), FixtureContext{}); err == nil || !strings.Contains(err.Error(), "typed live fixture requires GitLab client") {
		t.Fatalf("newLiveCaseFixturePreparer() error = %v, want missing client error", err)
	}
}

// TestLiveCaseFixture_EnsureFailure_PropagatesError verifies the shared live
// fixture builder reports a provisioning failure instead of a partial output.
func TestLiveCaseFixture_EnsureFailure_PropagatesError(t *testing.T) {
	client := newDestructiveFixtureClient(t)
	fixture := liveCaseFixture("boom", FixtureScopeCase, []string{"project_id"}, func(context.Context, *liveFixturePreparer) error {
		return errors.New("provisioning failed")
	})
	if _, err := fixture.Ensure(t.Context(), FixtureContext{Client: client}); err == nil || !strings.Contains(err.Error(), "provisioning failed") {
		t.Fatalf("Ensure() error = %v, want provisioning failure", err)
	}
}

// TestAttemptCaseSuffix_KeepsAlphanumericsOnly verifies the per-case suffix is
// reduced to characters GitLab accepts in a resource name.
func TestAttemptCaseSuffix_KeepsAlphanumericsOnly(t *testing.T) {
	cases := []struct {
		id   EvalCaseID
		want string
	}{
		{id: "MS-ENT-DYN-10", want: "msentdyn10"},
		{id: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.id), func(t *testing.T) {
			if got := attemptCaseSuffix(tc.id); got != tc.want {
				t.Fatalf("attemptCaseSuffix(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}
