// case_fixtures_project_test.go covers the typed fixtures used by the base
// mutating Docker preset: shared resource builders, attempt-name derivation,
// and prompt-template rendering for live fixtures.

package evaluator

import (
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
		fixture, ok := byName[name]
		if !ok {
			t.Fatalf("fixture %q missing from %s", name, fixtureNames(fixtures))
		}
		if fixture.Ensure == nil || fixture.Validate == nil || fixture.Cleanup == nil {
			t.Fatalf("fixture %q callbacks = ensure:%t validate:%t cleanup:%t", name, fixture.Ensure != nil, fixture.Validate != nil, fixture.Cleanup != nil)
		}
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
		if got := output[key]; got != want {
			t.Fatalf("output[%s] = %q, want %q", key, got, want)
		}
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
		if got := output[key]; got != want {
			t.Fatalf("output[%s] = %q, want %q", key, got, want)
		}
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
		if got := output[key]; got != want {
			t.Fatalf("output[%s] = %q, want %q", key, got, want)
		}
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
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered prompt = %q, want %q", rendered, want)
		}
	}
}
