package evaluator

import (
	"strings"
	"testing"
)

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
