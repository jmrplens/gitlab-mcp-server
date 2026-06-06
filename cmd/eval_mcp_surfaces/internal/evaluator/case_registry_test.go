package evaluator

import (
	"slices"
	"strings"
	"testing"
)

func TestValidateEvalCaseRegistry_DetectsInvalidDefinitions(t *testing.T) {
	cases := []EvalCase{
		{ID: "DUP", Prompt: "valid prompt", Presets: []EvalPreset{EvalPreset(presetDockerRead)}, Partition: EvalPartition(partitionBaseRead), Steps: []ExpectedStep{{ExpectedTool: "gitlab_user", ExpectedAction: "current"}}},
		{ID: "DUP", Prompt: "duplicate prompt", Presets: []EvalPreset{EvalPreset(presetDockerRead)}, Steps: []ExpectedStep{{ExpectedTool: "gitlab_user", ExpectedAction: "current"}}},
		{ID: "EMPTY-PROMPT", Steps: []ExpectedStep{{ExpectedTool: "gitlab_user", ExpectedAction: "current"}}},
		{ID: "EMPTY-STEPS", Prompt: "no steps"},
		{ID: "BAD-DESTRUCTIVE", Prompt: "delete without confirm", Steps: []ExpectedStep{{ExpectedTool: "gitlab_issue", ExpectedAction: "delete", Destructive: true}}},
		{ID: "BAD-PRESET", Prompt: "bad preset", Presets: []EvalPreset{"unknown"}, Steps: []ExpectedStep{{ExpectedTool: "gitlab_user", ExpectedAction: "current"}}},
		{ID: "BAD-OPTIONAL-ACTION", Prompt: "optional action", Steps: []ExpectedStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, OptionalStep: true}, {ExpectedTool: resourceListTool}}},
		{ID: "BAD-OPTIONAL-TERMINAL", Prompt: "optional terminal", Steps: []ExpectedStep{{ExpectedTool: capabilityListTool, OptionalStep: true}}},
	}
	problems := strings.Join(validateEvalCaseRegistry(cases, nil), "\n")
	for _, want := range []string{"duplicate ID", "empty prompt", "no expected steps", "does not list confirm", "unknown preset", "non-capability bridge step as optional", "must be followed by another capability bridge step"} {
		if !strings.Contains(problems, want) {
			t.Fatalf("problems missing %q:\n%s", want, problems)
		}
	}
}

func TestAllEvalCases_ContainsMigratedReadMutatingAndCapabilityCases(t *testing.T) {
	cases := AllEvalCases()
	if len(cases) < 173 {
		t.Fatalf("len(AllEvalCases()) = %d, want at least 173", len(cases))
	}
	for _, id := range []string{"MT-001", "MT-002", "MT-003", "MT-004", "MT-010", "MT-017", "MT-026", "MT-070", "MT-117", "MT-125", "MT-188", "MT-192", "MT-196", "MS-008", "MS-010", "MS-028", "MS-038", "MS-039", "MS-040", "MS-041", "MS-042"} {
		if _, ok := CaseByID(id); !ok {
			t.Fatalf("CaseByID(%s) = false, want migrated typed case", id)
		}
	}
	if got := len(CasesByPreset(presetDockerRead)); got < 40 {
		t.Fatalf("CasesByPreset(docker-read) = %d, want at least 40", got)
	}
	if got := len(CasesByPreset(presetDockerMutatingSafe)); got != 33 {
		t.Fatalf("CasesByPreset(docker-mutating-safe) = %d, want 33", got)
	}
	if got := len(CasesByPreset(presetDockerDestructiveSafe)); got != 65 {
		t.Fatalf("CasesByPreset(docker-destructive-safe) = %d, want 65", got)
	}
	if got := len(CasesByPreset(presetDockerCapabilityDiscovery)); got != 5 {
		t.Fatalf("CasesByPreset(docker-capability-discovery) = %d, want 5", got)
	}
	if got := len(CasesByPreset(presetDockerErrorRecovery)); got != 4 {
		t.Fatalf("CasesByPreset(docker-error-recovery) = %d, want 4", got)
	}
	if got := len(CasesByPreset(presetSchemaEnterprise)); got != 118 {
		t.Fatalf("CasesByPreset(schema-enterprise) = %d, want 118 (all Enterprise cases, incl. 10 MS-ENT-DYN-*)", got)
	}
	if got := len(CasesByPreset(presetDockerEnterpriseRead)); got != 13 {
		t.Fatalf("CasesByPreset(docker-enterprise-read) = %d, want 13 (5 baseline + 8 MS-ENT-DYN-1..8)", got)
	}
	if got := len(CasesByPreset(presetDockerEnterpriseMutatingSafe)); got != 5 {
		t.Fatalf("CasesByPreset(docker-enterprise-mutating-safe) = %d, want 5", got)
	}
	if got := len(CasesByPreset(presetDockerEnterpriseDestructiveSafe)); got != 13 {
		t.Fatalf("CasesByPreset(docker-enterprise-destructive-safe) = %d, want 13", got)
	}
}

func TestLoadEvalCases_UsesTypedRegistryOnly(t *testing.T) {
	cases, err := loadEvalCases(options{})
	if err != nil {
		t.Fatalf("loadEvalCases() error = %v", err)
	}
	seen := map[EvalCaseID]int{}
	for _, evalCase := range cases {
		seen[evalCase.ID]++
	}
	if seen["MT-001"] != 1 || seen["MT-004"] != 1 {
		t.Fatalf("seen counts = %+v, want typed MT-001 and MT-004 once", seen)
	}
	mt001, ok := CaseByID("MT-001")
	if !ok {
		t.Fatal("CaseByID(MT-001) = false")
	}
	if cases[0].ID != mt001.ID {
		t.Fatalf("first case = %+v, want typed MT-001", cases[0])
	}
	if _, customErr := loadEvalCases(options{TasksPath: "custom.md"}); customErr == nil {
		t.Fatal("loadEvalCases(custom --tasks) error = nil, want deprecation error")
	}
}

func TestDestructiveTypedFixtures_AttemptScopedForLiveTargets(t *testing.T) {
	checks := map[string]string{
		"MT-017": "mergeable_merge_request",
		"MT-024": "failed_job_artifact",
		"MT-065": "failed_job_artifact",
		"MT-066": "job_token_scope_project",
		"MT-109": "merge_request_award_emoji",
		"MS-028": "branch_protection_lifecycle",
	}
	for id, fixtureName := range checks {
		t.Run(id, func(t *testing.T) {
			evalCase, ok := CaseByID(id)
			if !ok {
				t.Fatalf("CaseByID(%s) = false", id)
			}
			fixtures := requireFixtureNames(evalCase.Fixtures)
			fixture, ok := fixtures[fixtureName]
			if !ok {
				t.Fatalf("%s fixtures = %s, want %s", id, fixtureNames(evalCase.Fixtures), fixtureName)
			}
			if fixture.Scope != FixtureScopeAttempt {
				t.Fatalf("%s fixture scope = %q, want %q", id, fixture.Scope, FixtureScopeAttempt)
			}
			if evalCase.PromptTemplate.Text == "" {
				t.Fatalf("%s missing prompt template", id)
			}
		})
	}
}

func TestDestructiveMergeRequestLiveCasesUseFixtures(t *testing.T) {
	for _, id := range []string{"MS-027", "MS-033"} {
		t.Run(id, func(t *testing.T) {
			evalCase, ok := CaseByID(id)
			if !ok {
				t.Fatalf("CaseByID(%s) = false", id)
			}
			fixtures := requireFixtureNames(evalCase.Fixtures)
			if _, hasFixture := fixtures["merge_request"]; !hasFixture {
				t.Fatalf("%s fixtures = %s, want merge_request", id, fixtureNames(evalCase.Fixtures))
			}
			if !strings.Contains(evalCase.PromptTemplate.Text, "{{ .MergeRequest.IID }}") {
				t.Fatalf("%s prompt template = %q, want merge request fixture IID", id, evalCase.PromptTemplate.Text)
			}
			if strings.Contains(evalCase.PromptTemplate.Text, "MR `7`") {
				t.Fatalf("%s prompt template keeps legacy static MR: %q", id, evalCase.PromptTemplate.Text)
			}
		})
	}
}

func TestReleaseAssetLinkCRUDCaseUsesAttemptScopedURLs(t *testing.T) {
	evalCase, ok := CaseByID("MS-018")
	if !ok {
		t.Fatal("CaseByID(MS-018) = false")
	}
	fixtures := requireFixtureNames(evalCase.Fixtures)
	fixture, ok := fixtures["attempt_names"]
	if !ok {
		t.Fatalf("MS-018 fixtures = %s, want attempt_names", fixtureNames(evalCase.Fixtures))
	}
	if fixture.Scope != FixtureScopeAttempt {
		t.Fatalf("MS-018 attempt_names scope = %q, want %q", fixture.Scope, FixtureScopeAttempt)
	}
	for _, want := range []string{"{{ .Values.release_link_url }}", "{{ .Values.release_link_updated_url }}"} {
		if !strings.Contains(evalCase.PromptTemplate.Text, want) {
			t.Fatalf("MS-018 prompt template = %q, want %q", evalCase.PromptTemplate.Text, want)
		}
	}
	if strings.Contains(evalCase.PromptTemplate.Text, "only after the release exists, add asset link `eval-crud-link`") {
		t.Fatalf("MS-018 prompt template keeps legacy static release link text: %q", evalCase.PromptTemplate.Text)
	}
}

func TestEnterpriseProtectedEnvironmentCasesUseAttemptScopedNames(t *testing.T) {
	for _, id := range []string{"MS-052", "MS-053"} {
		t.Run(id, func(t *testing.T) {
			evalCase, ok := CaseByID(id)
			if !ok {
				t.Fatalf("CaseByID(%s) = false", id)
			}
			fixtures := requireFixtureNames(evalCase.Fixtures)
			fixture, ok := fixtures["attempt_names"]
			if !ok {
				t.Fatalf("%s fixtures = %s, want attempt_names", id, fixtureNames(evalCase.Fixtures))
			}
			if fixture.Scope != FixtureScopeAttempt {
				t.Fatalf("%s attempt_names scope = %q, want %q", id, fixture.Scope, FixtureScopeAttempt)
			}
			for _, want := range []string{"{{ .Values.subgroup_name }}", "{{ .Values.subgroup_path }}"} {
				if !strings.Contains(evalCase.PromptTemplate.Text, want) {
					t.Fatalf("%s prompt template = %q, want %q", id, evalCase.PromptTemplate.Text, want)
				}
			}
			if !strings.Contains(evalCase.PromptTemplate.Text, "Maintainer deploy access") {
				t.Fatalf("%s prompt template = %q, want Maintainer deploy access guidance", id, evalCase.PromptTemplate.Text)
			}
		})
	}
}

func TestInteractiveMergeRequestCaseUsesGuidedOnlyParams(t *testing.T) {
	evalCase, ok := CaseByID("MT-081")
	if !ok {
		t.Fatal("CaseByID(MT-081) = false")
	}
	fixtures := requireFixtureNames(evalCase.Fixtures)
	if _, hasFixture := fixtures["merge_request_source"]; !hasFixture {
		t.Fatalf("MT-081 fixtures = %s, want merge_request_source", fixtureNames(evalCase.Fixtures))
	}
	for _, want := range []string{"Do not pass `source_branch`", "guided prompts will use source branch"} {
		if !strings.Contains(evalCase.PromptTemplate.Text, want) {
			t.Fatalf("MT-081 prompt template = %q, want %q", evalCase.PromptTemplate.Text, want)
		}
	}
}

func TestEnterpriseDockerCases_AttachTypedFixtures(t *testing.T) {
	checks := map[string]string{
		"MT-192": "enterprise_push_rule_project",
		"MT-193": "enterprise_push_rule_project_seeded",
		"MT-195": "project_service_account",
		"MT-196": "enterprise_push_rule_project_seeded",
		"MT-197": "enterprise_group_service_account_pat",
		"MT-198": "enterprise_group_service_account",
		"MS-045": "enterprise_push_rule_project",
		"MS-054": "project_service_account",
	}
	for id, fixtureName := range checks {
		t.Run(id, func(t *testing.T) {
			evalCase, ok := CaseByID(id)
			if !ok {
				t.Fatalf("CaseByID(%s) = false", id)
			}
			fixtures := requireFixtureNames(evalCase.Fixtures)
			fixture, ok := fixtures[fixtureName]
			if !ok {
				t.Fatalf("%s fixtures = %s, want %s", id, fixtureNames(evalCase.Fixtures), fixtureName)
			}
			if fixture.RequiredRuntime != EvalCaseEdition(editionEnterprise) {
				t.Fatalf("%s fixture runtime = %q, want enterprise", id, fixture.RequiredRuntime)
			}
			if evalCase.PromptTemplate.Text == "" {
				t.Fatalf("%s prompt template is empty", id)
			}
		})
	}
}

func TestDestructiveEvalCases_DestructiveStepsRequireConfirm(t *testing.T) {
	for _, evalCase := range CasesByPreset(presetDockerDestructiveSafe) {
		t.Run(string(evalCase.ID), func(t *testing.T) {
			if !evalCase.Destructive {
				t.Fatalf("%s Destructive = false", evalCase.ID)
			}
			for i, step := range evalCase.Steps {
				if !step.Destructive {
					continue
				}
				if !slices.Contains(step.OptionalParams, "confirm") && !slices.Contains(step.RequiredParams, "confirm") {
					t.Fatalf("%s step %d (%s.%s) lacks confirm", evalCase.ID, i+1, step.ExpectedTool, step.ExpectedAction)
				}
			}
		})
	}
}
