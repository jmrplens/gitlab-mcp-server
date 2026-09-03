// case_registry_test.go covers the typed evaluation case registry: case
// validation, expected coverage, fixture scope invariants, and the deprecation
// of legacy --tasks Markdown files.

package evaluator

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestValidateEvalCaseRegistry_DetectsInvalidDefinitions verifies that
// validateEvalCaseRegistry returns descriptive errors for every documented
// case definition issue: duplicate IDs, empty prompts, missing steps,
// destructive steps without confirm, unknown presets, and optional capability
// bridge steps that are not paired correctly.
//
// The test feeds a synthetic slice of EvalCase objects covering each failure
// mode and asserts the joined problem string contains each expected phrase.
// This protects the typed registry from silently accepting malformed cases
// during refactors.
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(problems, want) {
				t.Fatalf("problems missing %q:\n%s", want, problems)
			}
		})
	}
}

// TestAllEvalCases_ContainsMigratedReadMutatingAndCapabilityCases verifies
// that AllEvalCases returns at least the expected minimum set of typed
// evaluation cases and that all migrated IDs remain discoverable through
// CaseByID and the preset groupings.
//
// The test asserts a floor on the total case count, a representative sample
// of case IDs, and the exact counts for every Docker preset and the
// schema-enterprise preset. This protects the registry from quietly losing
// cases during the legacy-to-typed migration.
func TestAllEvalCases_ContainsMigratedReadMutatingAndCapabilityCases(t *testing.T) {
	cases := AllEvalCases()
	if len(cases) < 173 {
		t.Fatalf("len(AllEvalCases()) = %d, want at least 173", len(cases))
	}
	for _, id := range []string{"MT-001", "MT-002", "MT-003", "MT-004", "MT-010", "MT-017", "MT-026", "MT-070", "MT-117", "MT-125", "MT-188", "MT-192", "MT-196", "MS-008", "MS-010", "MS-028", "MS-038", "MS-039", "MS-040", "MS-041", "MS-042"} {
		t.Run(id, func(t *testing.T) {
			if _, ok := CaseByID(id); !ok {
				t.Fatalf("CaseByID(%s) = false, want migrated typed case", id)
			}
		})
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

// TestLoadEvalCases_UsesTypedRegistryOnly verifies that loadEvalCases
// returns the typed registry when no custom --tasks path is supplied and
// rejects legacy Markdown --tasks files with a deprecation error.
//
// The test calls loadEvalCases with an empty options, asserts known case IDs
// appear exactly once and that the first case matches MT-001, then confirms
// that supplying a custom --tasks path produces a deprecation error. This
// protects the CLI from re-introducing the legacy markdown case loader.
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

// TestDestructiveTypedFixtures_AttemptScopedForLiveTargets verifies that
// destructive cases whose cleanup depends on the live GitLab instance use
// attempt-scoped fixtures and a non-empty prompt template, so each run gets
// fresh disposable state.
//
// The test walks a mapping of case IDs to fixture names and asserts each
// case attaches the expected fixture with FixtureScopeAttempt plus a prompt
// template that supplies per-attempt values. This protects live destructive
// runs from reusing shared fixtures that could leak state across runs.
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

// TestDestructiveMergeRequestLiveCasesUseFixtures verifies that destructive
// merge-request cases attach the merge_request fixture and reference
// {{ .MergeRequest.IID }} in the prompt template instead of legacy static
// values like MR `7`.
//
// The test iterates MS-027 and MS-033 and asserts each case exposes the
// merge_request fixture, embeds the fixture IID in the template, and does
// not retain the old static MR marker. This protects destructive MR cases
// from regressing to non-isolated state.
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

// TestReleaseAssetLinkCRUDCaseUsesAttemptScopedURLs verifies that the
// MS-018 release-asset-link CRUD case uses the attempt_names fixture so each
// attempt receives unique link URLs, and that its prompt template references
// the templated Values.release_link_url / Values.release_link_updated_url
// placeholders instead of legacy static URLs.
//
// The test loads MS-018, checks the attempt_names scope, and verifies the
// template contains the templated URL placeholders while no longer carrying
// the legacy static "eval-crud-link" phrasing. This protects the CRUD
// release case from losing attempt isolation.
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(evalCase.PromptTemplate.Text, want) {
				t.Fatalf("MS-018 prompt template = %q, want %q", evalCase.PromptTemplate.Text, want)
			}
		})
	}
	if strings.Contains(evalCase.PromptTemplate.Text, "only after the release exists, add asset link `eval-crud-link`") {
		t.Fatalf("MS-018 prompt template keeps legacy static release link text: %q", evalCase.PromptTemplate.Text)
	}
}

// TestEnterpriseProtectedEnvironmentCasesUseAttemptScopedNames verifies that
// the MS-052 and MS-053 Enterprise protected-environment cases attach the
// attempt_names fixture, reference .Values.subgroup_name and .Values.subgroup_path
// in the prompt template, and preserve the "Maintainer deploy access"
// guidance needed by the case workflow.
//
// The test runs a subtest per case ID and asserts each fixture and template
// invariant. This protects the protected-environment cases from regressing
// to shared subgroup state and losing the maintainer-deploy-access hint.
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

// TestInteractiveMergeRequestCaseUsesGuidedOnlyParams verifies that the
// interactive merge-request case MT-081 attaches the merge_request_source
// fixture and instructs the model to omit source_branch in favor of the
// guided prompt's implicit branch handling.
//
// The test asserts the fixture is attached and the prompt template includes
// both "Do not pass `source_branch`" and "guided prompts will use source
// branch" guidance. This protects interactive MR workflows from regressing
// to manual source-branch entry that would conflict with guided prompts.
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(evalCase.PromptTemplate.Text, want) {
				t.Fatalf("MT-081 prompt template = %q, want %q", evalCase.PromptTemplate.Text, want)
			}
		})
	}
}

// TestEnterpriseDockerCases_AttachTypedFixtures verifies that every
// Enterprise Docker evaluation case attaches the correct typed Enterprise
// fixture and exposes a non-empty prompt template.
//
// The test iterates a case ID to fixture name mapping covering push-rule
// projects, seeded push-rule projects, project/group service accounts, and
// the matching MS-IDs. Each subtest asserts the fixture is present, requires
// the Enterprise runtime edition, and that the prompt template is populated.
// This protects Enterprise Docker cases from running with the wrong fixture
// edition or an empty template.
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

// TestDestructiveEvalCases_DestructiveStepsRequireConfirm verifies that
// every destructive step in the docker-destructive-safe preset lists
// confirm as either a required or optional parameter, so the model's tool
// call cannot bypass the safety prompt.
//
// The test iterates all destructive cases returned by CasesByPreset and for
// each destructive step asserts confirm is present in the param lists.
// This protects destructive evaluation runs from executing steps that lack
// the confirm gate.
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

// TestValidateEvalCaseRegistry_BuiltInCatalog_ReportsNoProblems verifies the
// exported validator runs over the shipped catalog and finds nothing to
// complain about at all: no duplicate IDs, and no empty prompts, missing
// steps, unknown presets or partitions, destructive steps without confirm or
// misplaced optional steps either.
//
// This assertion used to accept duplicate-ID problems and reject every other
// kind, which let thirteen known collisions
// (https://github.com/jmrplens/gitlab-mcp-server/issues/361) sit in the
// catalog while still failing the build on any other defect. The exemption is
// gone, so a reused ID now fails here like anything else.
func TestValidateEvalCaseRegistry_BuiltInCatalog_ReportsNoProblems(t *testing.T) {
	for _, problem := range ValidateEvalCaseRegistry(nil) {
		t.Run(problem, func(t *testing.T) {
			t.Errorf("ValidateEvalCaseRegistry(nil) reported %q, want no problems", problem)
		})
	}
}

// TestAllEvalCases_BuiltInCatalog_HasNoDuplicateIDs verifies no two shipped
// cases share an ID, asserted directly against the catalog rather than through
// the validator, so weakening the validator cannot hide a collision here.
//
// Partitions and case sets are not namespaces: [All] concatenates them into
// one slice and every consumer keys on the bare ID. The failure names each
// colliding case with its partition and prompt, because the useful question
// when this breaks is which two cases claimed the number.
func TestAllEvalCases_BuiltInCatalog_HasNoDuplicateIDs(t *testing.T) {
	casesByID := map[EvalCaseID][]EvalCase{}
	var collidingIDs []EvalCaseID
	for _, evalCase := range AllEvalCases() {
		if len(casesByID[evalCase.ID]) == 1 {
			collidingIDs = append(collidingIDs, evalCase.ID)
		}
		casesByID[evalCase.ID] = append(casesByID[evalCase.ID], evalCase)
	}
	for _, id := range collidingIDs {
		t.Run(string(id), func(t *testing.T) {
			for _, evalCase := range casesByID[id] {
				t.Logf("partition %q: %s", evalCase.Partition, casePrompt(evalCase))
			}
			t.Errorf("case ID %s names %d cases, want 1", id, len(casesByID[id]))
		})
	}
}

// TestCaseByID_BuiltInCatalog_ResolvesEveryCase verifies every shipped case is
// reachable by its own ID.
//
// CaseByID returns the first match, so a duplicate does not fail the lookup,
// it hides the later case behind the earlier one. This test fails on the
// hidden case rather than on the duplication itself, which is the symptom a
// caller of CaseByID or --task actually experiences: MT-110 used to resolve to
// a merge-request listing while an award-emoji deletion sharing the number was
// unreachable.
func TestCaseByID_BuiltInCatalog_ResolvesEveryCase(t *testing.T) {
	for _, evalCase := range AllEvalCases() {
		t.Run(string(evalCase.ID), func(t *testing.T) {
			resolved, ok := CaseByID(string(evalCase.ID))
			if !ok {
				t.Fatalf("CaseByID(%q) ok = false, want true", evalCase.ID)
			}
			if got, want := casePrompt(resolved), casePrompt(evalCase); got != want {
				t.Errorf("CaseByID(%q) resolved to %q, want %q; the case is shadowed by another with the same ID", evalCase.ID, got, want)
			}
		})
	}
}

// TestValidateEvalCaseRegistry_DuplicateIDAcrossPartitions_ReportsProblem
// verifies a reused ID is reported even when the two cases sit in different
// partitions and target different editions.
//
// That is the exact shape of the thirteen collisions the catalog used to
// carry: a CE read case and a destructive or Enterprise case sharing one
// number. A differing partition does not make the reuse safe, because no
// consumer of a case ID carries the partition alongside it, so the validator
// must not treat this pair as a special case.
func TestValidateEvalCaseRegistry_DuplicateIDAcrossPartitions_ReportsProblem(t *testing.T) {
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": {}}}
	step := []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}
	cases := []EvalCase{
		{
			ID:        "MT-110",
			Prompt:    "List merged merge requests.",
			Steps:     step,
			Edition:   EvalCaseEdition(editionCE),
			Partition: EvalPartition(partitionBaseRead),
		},
		{
			ID:        "MT-110",
			Prompt:    "Force-push remote mirror ID 9.",
			Steps:     step,
			Edition:   EvalCaseEdition(editionEnterprise),
			Partition: EvalPartition(partitionEnterpriseDestructive),
		},
	}
	problems := validateEvalCaseRegistry(cases, routes)
	if !slices.Contains(problems, "MT-110 has duplicate ID") {
		t.Fatalf("problems = %v, want to contain %q", problems, "MT-110 has duplicate ID")
	}
}

// TestCaseByID_UnknownID_ReturnsFalse verifies an unknown case ID reports a
// miss instead of an empty case.
func TestCaseByID_UnknownID_ReturnsFalse(t *testing.T) {
	if _, ok := CaseByID("MT-DOES-NOT-EXIST"); ok {
		t.Fatal("CaseByID(unknown) ok = true, want false")
	}
}

// TestValidateEvalCaseRegistry_MalformedCases_ReportsEachProblem verifies the
// validator names every structural defect: empty and duplicate IDs, empty
// prompts, missing steps, unknown presets and partitions, destructive steps
// without confirm, misplaced optional steps, and unregistered routes.
func TestValidateEvalCaseRegistry_MalformedCases_ReportsEachProblem(t *testing.T) {
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": {}}}
	cases := []EvalCase{
		{ID: "", Prompt: "x", Steps: []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}},
		{ID: "MT-DUP", Prompt: "x", Steps: []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}},
		{ID: "MT-DUP", Prompt: "x", Steps: []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}},
		{ID: "MT-EMPTY", Steps: nil, Presets: []EvalPreset{"nope"}, Partition: "nowhere"},
		{ID: "MT-STEPS", Prompt: "x", Steps: []ExpectedStep{
			{ExpectedTool: "", ExpectedAction: "get"},
			{ExpectedTool: "gitlab_project", ExpectedAction: "delete", Destructive: true},
			{ExpectedTool: "gitlab_project", ExpectedAction: "list", OptionalStep: true},
			{ExpectedTool: capabilityListTool, OptionalStep: true},
		}},
	}
	problems := validateEvalCaseRegistry(cases, routes)
	for _, want := range []string{
		"case has empty ID",
		"MT-DUP has duplicate ID",
		"MT-EMPTY has empty prompt",
		"MT-EMPTY has no expected steps",
		`MT-EMPTY uses unknown preset "nope"`,
		`MT-EMPTY uses unknown partition "nowhere"`,
		"MT-STEPS step 1 has empty expected tool",
		"MT-STEPS step 2 is destructive but does not list confirm as a parameter",
		"MT-STEPS step 2 expected route gitlab_project/delete is not registered",
		"MT-STEPS step 3 marks a non-capability bridge step as optional",
		"MT-STEPS step 4 optional capability bridge step must be followed by another capability bridge step",
	} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(problems, want) {
				t.Fatalf("problems = %v, want %q", problems, want)
			}
		})
	}
}

// TestCaseFixturesFromNames_UnknownName_Panics verifies an unregistered
// fixture name is a programming error surfaced at registry build time.
func TestCaseFixturesFromNames_UnknownName_Panics(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil || !strings.Contains(recovered.(string), `unknown evaluator case fixture "nope"`) {
			t.Fatalf("recover() = %v, want unknown fixture panic", recovered)
		}
	}()
	caseFixturesFromNames([]string{"nope"})
}
