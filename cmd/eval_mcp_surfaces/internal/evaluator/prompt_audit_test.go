package evaluator

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// exactCallPromptSignature is the sentence only exactToolTaskPrompt writes. It
// is how a test names the cases that reach that builder without reimplementing
// the predicate that selects it, and it is what makes the assertions below
// fail loudly when the builder is deleted: the set they compare against the
// twelve becomes empty.
const exactCallPromptSignature = "Exact required call: use the "

// answerKeyedDestructiveClause is the dynamic prompt's destructive line.
const answerKeyedDestructiveClause = "when executing the destructive action, include top-level confirm:true on gitlab_execute_action"

// answerKeyedOperationCountClause is the opening of the dynamic prompt's
// operation count, which states how many catalog operations the answer key
// holds.
const answerKeyedOperationCountClause = "For each of the "

// answerKeyedProjectGetClause is the dynamic prompt's project.get clause.
const answerKeyedProjectGetClause = "the requested catalog operation is project.get, not project.list"

// answerKeyedReleaseCompareClause is the dynamic prompt's release-compare
// clause, which no case in today's corpus reaches.
const answerKeyedReleaseCompareClause = "For release-summary workflows that compare refs before generating notes"

// exactCallPromptCases are the cases whose meta task prompt is built by
// exactToolTaskPrompt today, measured against the tree rather than copied from
// a plan.
var exactCallPromptCases = []string{
	"MT-002", "MT-021", "MT-024", "MT-030", "MT-055", "MT-061",
	"MT-065", "MT-068", "MT-104", "MT-108", "MT-109", "MT-113",
}

// promptAuditForSurface audits the whole corpus on one surface, through the
// same catalog normalization a run performs.
func promptAuditForSurface(t *testing.T, surface string) promptAuditReport {
	t.Helper()
	opts := options{ToolSurface: surface, Backend: backendMock, Edition: editionAll, ServerMode: ServerModeDefault}
	_, routes, _, err := loadCatalog(opts)
	if err != nil {
		t.Fatalf("loadCatalog(%s) error = %v", surface, err)
	}
	tasks := normalizeTasksForCatalog(evalTasksFromCases(AllEvalCases()), routes, surface)
	report := auditPrompts(opts, tasks)
	if len(report.Cases) == 0 {
		t.Fatalf("auditPrompts(%s) audited no case", surface)
	}
	return report
}

// casesCarrying lists the cases whose task prompt contains needle.
func casesCarrying(report promptAuditReport, needle string) []string {
	var ids []string
	for _, audited := range report.Cases {
		if strings.Contains(audited.UserPrompt, needle) {
			ids = append(ids, audited.ID)
		}
	}
	return ids
}

// casesWithFindingKind lists the cases with at least one finding of one kind.
func casesWithFindingKind(report promptAuditReport, kind promptAuditKind) []string {
	var ids []string
	for _, audited := range report.Cases {
		if audited.named(kind) > 0 {
			ids = append(ids, audited.ID)
		}
	}
	return ids
}

// TestAuditPrompts_MetaExactCallCases_CarryTheirOwnMarshaledEnvelope pins the
// leak V05 deletes. Twelve meta cases are handed the whole call they are then
// scored on, marshaled as the envelope the model must emit.
//
// The assertion is set equality against the twelve, so the deletion of
// exactToolTaskPrompt fails it with an empty set rather than passing quietly.
func TestAuditPrompts_MetaExactCallCases_CarryTheirOwnMarshaledEnvelope(t *testing.T) {
	report := promptAuditForSurface(t, config.ToolSurfaceMeta)
	exact := casesCarrying(report, exactCallPromptSignature)
	if !slices.Equal(exact, exactCallPromptCases) {
		t.Fatalf("cases reaching exactToolTaskPrompt = %v, want %v", exact, exactCallPromptCases)
	}
	for _, audited := range report.Cases {
		if !slices.Contains(exactCallPromptCases, audited.ID) {
			continue
		}
		assertCarriesOwnEnvelope(t, audited)
	}
}

// assertCarriesOwnEnvelope fails unless the case's task prompt carries a
// marshaled envelope naming an action the case itself expects.
func assertCarriesOwnEnvelope(t *testing.T, audited promptAuditCase) {
	t.Helper()
	for _, finding := range audited.Findings {
		if finding.Kind != promptLeakEnvelope {
			continue
		}
		if !slices.Contains(finding.Sites, promptSiteTask) {
			t.Errorf("%s: envelope %s found at %v, want the task scaffolding", audited.ID, finding.Value, finding.Sites)
		}
		return
	}
	t.Errorf("%s: no envelope finding, yet its prompt carries %q", audited.ID, exactCallPromptSignature)
}

// TestAuditPrompts_MetaEnvelopes_ComeFromTwoBuildersNotOne records that the
// exact-call builder is not the only path that marshals the expected call into
// a prompt: six further cases get one from the guidance clauses V06 removes.
// V05 takes this population from eighteen to six; V06 takes it to none.
func TestAuditPrompts_MetaEnvelopes_ComeFromTwoBuildersNotOne(t *testing.T) {
	report := promptAuditForSurface(t, config.ToolSurfaceMeta)
	withEnvelope := casesWithFindingKind(report, promptLeakEnvelope)
	exact := casesCarrying(report, exactCallPromptSignature)
	for _, id := range exact {
		if !slices.Contains(withEnvelope, id) {
			t.Errorf("case %s reaches the exact-call builder and carries no envelope finding", id)
		}
	}
	var fromGuidance []string
	for _, id := range withEnvelope {
		if !slices.Contains(exact, id) {
			fromGuidance = append(fromGuidance, id)
		}
	}
	if len(fromGuidance) == 0 {
		t.Fatalf("no case gets an envelope from a guidance clause; the whole population is %v", withEnvelope)
	}
	t.Logf("envelopes: %d cases, %d from the exact-call builder, %d from guidance clauses (%v)",
		len(withEnvelope), len(exact), len(fromGuidance), fromGuidance)
}

// TestAuditPrompts_DynamicPrompts_CarryTheAnswerKeyedClauses pins the three
// clauses of dynamicTaskPrompt that today's corpus reaches. The fourth, the
// release-compare clause, reaches no case and is asserted in the test below
// against a task built for it, so that its deletion fails something.
func TestAuditPrompts_DynamicPrompts_CarryTheAnswerKeyedClauses(t *testing.T) {
	report := promptAuditForSurface(t, config.ToolSurfaceDynamic)
	destructive := casesCarrying(report, answerKeyedDestructiveClause)
	if len(destructive) == 0 {
		t.Error("no dynamic prompt carries the destructive clause")
	}
	for _, audited := range report.Cases {
		carries := strings.Contains(audited.UserPrompt, answerKeyedDestructiveClause)
		if carries != audited.Destructive {
			t.Errorf("%s: destructive clause = %v, case destructive = %v", audited.ID, carries, audited.Destructive)
		}
	}
	if counted := casesCarrying(report, answerKeyedOperationCountClause); len(counted) == 0 {
		t.Error("no dynamic prompt states how many catalog operations the answer key holds")
	}
	if projectGet := casesCarrying(report, answerKeyedProjectGetClause); len(projectGet) == 0 {
		t.Error("no dynamic prompt carries the project.get clause")
	}
	if carried := casesCarrying(report, answerKeyedReleaseCompareClause); len(carried) != 0 {
		t.Errorf("the release-compare clause reached %v; it reached no case when this was written", carried)
	}
	t.Logf("dynamic clauses: destructive %d, operation count %d, project.get %d, release compare 0",
		len(destructive), len(casesCarrying(report, answerKeyedOperationCountClause)), len(casesCarrying(report, answerKeyedProjectGetClause)))
}

// TestDynamicTaskPrompt_ReleaseCompareClause_IsSelectedByTheAnswerKey drives
// the one dynamic clause the corpus does not reach, so its deletion fails a
// test rather than passing unnoticed.
func TestDynamicTaskPrompt_ReleaseCompareClause_IsSelectedByTheAnswerKey(t *testing.T) {
	task := evalTask{
		ID:     "PA-001",
		Prompt: "Summarize what shipped in project `group/project`.",
		Steps: []evalStep{
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "release.list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "repository.compare", RequiredParams: []string{"project_id"}},
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "release.create", RequiredParams: []string{"project_id"}},
		},
	}
	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(prompt, answerKeyedReleaseCompareClause) {
		t.Fatalf("release-compare clause missing from a prompt whose steps select it:\n%s", prompt)
	}
	audited := auditPromptsForTask(task, config.ToolSurfaceDynamic)
	if !slices.Contains(audited.AnswerKeyed, promptSiteTask) {
		t.Errorf("AnswerKeyed = %v, want the task prompt to change when the answer key goes", audited.AnswerKeyed)
	}
}

// TestAuditPrompts_EveryDynamicCase_IsAnswerKeyedInItsTaskPrompt pins the
// coaching that names nothing: every dynamic task prompt states the number of
// catalog operations the answer key holds, so every one of them changes when
// the key is taken away.
func TestAuditPrompts_EveryDynamicCase_IsAnswerKeyedInItsTaskPrompt(t *testing.T) {
	report := promptAuditForSurface(t, config.ToolSurfaceDynamic)
	for _, audited := range report.Cases {
		if !slices.Contains(audited.AnswerKeyed, promptSiteTask) {
			t.Errorf("%s: task prompt did not change when the answer key was removed", audited.ID)
		}
		if slices.Contains(audited.AnswerKeyed, promptSiteSystem) {
			t.Errorf("%s: dynamic system prompt reads the answer key, which it did not when this was written", audited.ID)
		}
	}
}

// TestAuditPrompts_ConfirmLiteral_ReachesEveryCaseOnBothSurfaces pins what the
// published destructive-safety column rests on: both system prompts name
// confirm for every case, destructive or not.
func TestAuditPrompts_ConfirmLiteral_ReachesEveryCaseOnBothSurfaces(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta} {
		t.Run(surface, func(t *testing.T) {
			report := promptAuditForSurface(t, surface)
			totals := report.totals()
			if totals.ByKind[promptLeakConfirm].System != totals.Cases {
				t.Errorf("confirm named in the system prompt of %d cases, want all %d",
					totals.ByKind[promptLeakConfirm].System, totals.Cases)
			}
			if totals.Destructive == 0 || totals.DestructiveConfirmNamed != totals.Destructive {
				t.Errorf("destructive cases = %d, of which confirm named = %d", totals.Destructive, totals.DestructiveConfirmNamed)
			}
		})
	}
}

// TestPromptNamesIdentifier_CountsCodeAndNotProse verifies the rule the whole
// count depends on: an answer literal is named where it is written as a
// machine identifier, and an English word that happens to spell it is not.
func TestPromptNamesIdentifier_CountsCodeAndNotProse(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		value string
		want  bool
	}{
		{name: "prose verb is not the action", text: "Please get the project and report back.", value: "get", want: false},
		{name: "dotted identifier is", text: "use project.get for that", value: "get", want: true},
		{name: "json string is", text: `{"action":"get","params":{}}`, value: "get", want: true},
		{name: "key introducing a value is", text: "include confirm:true in params", value: "confirm", want: true},
		{name: "param under params is", text: "put it in params.title", value: "title", want: true},
		{name: "prose noun is not the param", text: "the issue titled My title", value: "title", want: false},
		{name: "underscored name counts anywhere", text: "the project_id is the namespace path", value: "project_id", want: true},
		{name: "longer token does not match", text: "that is a valid choice", value: "id", want: false},
		{name: "prefix of a longer action does not match", text: "use custom_emoji.list here", value: "emoji.list", want: false},
		{name: "empty value is never named", text: "anything", value: "", want: false},
		{name: "value longer than the text", text: "id", value: "project_id", want: false},
		{name: "repeated near miss then a hit", text: "invalid, invalid, params.id", value: "id", want: true},
		{name: "a bare word on its own is prose", text: "get", value: "get", want: false},
		{name: "an underscored name opening the text", text: "project_id is the namespace path", value: "project_id", want: true},
		{name: "an opening brace is code", text: "send {id}", value: "id", want: true},
		{name: "an uppercase letter before it is part of the token", text: "MYid", value: "id", want: false},
		{name: "a digit after it is part of the token", text: "id42", value: "id", want: false},
		{name: "the whole text is the identifier", text: "project_id", value: "project_id", want: true},
		{name: "a match ending at the last byte counts", text: "send params.branch", value: "branch", want: true},
		{name: "a trailing dot is code", text: "say get.now", value: "get", want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := promptNamesIdentifier(testCase.text, testCase.value); got != testCase.want {
				t.Errorf("promptNamesIdentifier(%q, %q) = %v, want %v", testCase.text, testCase.value, got, testCase.want)
			}
		})
	}
}

// TestPromptNamesAnswer_ReadsAnEnvelopeAsTextAndEveryOtherKindAsAnIdentifier
// pins the dispatch the two searches sit behind. An envelope is a marshaled
// fragment and is searched for literally; every other kind is held to the
// identifier rule. Both values below are found by one search and missed by the
// other, so swapping the two fails rather than passing on a value both agree
// about.
func TestPromptNamesAnswer_ReadsAnEnvelopeAsTextAndEveryOtherKindAsAnIdentifier(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		answer promptAuditAnswer
		want   bool
	}{
		{
			name:   "an envelope glued to a word is still the envelope",
			text:   `say a"action":"get"b`,
			answer: promptAuditAnswer{Kind: promptLeakEnvelope, Value: `"action":"get"`},
			want:   true,
		},
		{
			name:   "an action spelled as an English verb is not named",
			text:   "Please get the project.",
			answer: promptAuditAnswer{Kind: promptLeakAction, Value: "get"},
			want:   false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := promptNamesAnswer(testCase.text, testCase.answer); got != testCase.want {
				t.Errorf("promptNamesAnswer(%q, %+v) = %v, want %v", testCase.text, testCase.answer, got, testCase.want)
			}
		})
	}
}

// TestPromptAuditSurfaceTool_ExcludesTheDispatchersAndNothingElse pins the
// decision the dynamic surface's tool column rests on: find and execute are
// the surface's own contract and belong to no case's answer, while every other
// tool name does, and on meta nothing is excluded at all.
func TestPromptAuditSurfaceTool_ExcludesTheDispatchersAndNothingElse(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		surface string
		want    bool
	}{
		{name: "find on dynamic is the contract", tool: dynamicFindTool, surface: config.ToolSurfaceDynamic, want: true},
		{name: "execute on dynamic is the contract", tool: dynamicExecuteActionTool, surface: config.ToolSurfaceDynamic, want: true},
		{name: "a domain tool on dynamic is an answer", tool: "gitlab_branch", surface: config.ToolSurfaceDynamic, want: false},
		{name: "find on meta is an answer", tool: dynamicFindTool, surface: config.ToolSurfaceMeta, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := promptAuditSurfaceTool(testCase.tool, testCase.surface); got != testCase.want {
				t.Errorf("promptAuditSurfaceTool(%q, %q) = %v, want %v", testCase.tool, testCase.surface, got, testCase.want)
			}
		})
	}
}

// TestPromptAuditSitesFor_SitesTheCaseTextOnlyWhereThePromptCarriesIt pins the
// guard that keeps a dropped request off the report: a builder that never sent
// the user's words sent none of them, however well the case text happens to
// spell the answer.
func TestPromptAuditSitesFor_SitesTheCaseTextOnlyWhereThePromptCarriesIt(t *testing.T) {
	answer := promptAuditAnswer{Kind: promptLeakParam, Value: "project_id"}
	cases := []struct {
		name     string
		audited  promptAuditCase
		scaffold string
		want     []promptAuditSite
	}{
		{
			name:    "the case text names it and the prompt carried it",
			audited: promptAuditCase{CaseTextInPrompt: true, CaseText: "pass project_id"},
			want:    []promptAuditSite{promptSiteCase},
		},
		{
			name:    "the case text names it and the prompt dropped it",
			audited: promptAuditCase{CaseTextInPrompt: false, CaseText: "pass project_id"},
			want:    nil,
		},
		{
			name:    "the prompt carried a case text that names nothing",
			audited: promptAuditCase{CaseTextInPrompt: true, CaseText: "summarize the work"},
			want:    nil,
		},
		{
			name:     "the builder's own scaffolding is a site of its own",
			audited:  promptAuditCase{CaseTextInPrompt: true, CaseText: "summarize the work"},
			scaffold: "send params.project_id",
			want:     []promptAuditSite{promptSiteTask},
		},
		{
			name:    "the system prompt is a site of its own",
			audited: promptAuditCase{CaseTextInPrompt: true, CaseText: "summarize the work", SystemPrompt: "always send params.project_id"},
			want:    []promptAuditSite{promptSiteSystem},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := promptAuditSitesFor(answer, testCase.audited, testCase.scaffold)
			if !slices.Equal(got, testCase.want) {
				t.Errorf("promptAuditSitesFor() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestPromptAuditCase_CoachedByBuilder_TakesEitherSiteOnItsOwn pins the number
// V05 to V07 move: a repetition sited in the system prompt alone is coaching by
// this package just as much as one sited in the task scaffolding alone, and a
// repetition sited only in the case's own text is not.
func TestPromptAuditCase_CoachedByBuilder_TakesEitherSiteOnItsOwn(t *testing.T) {
	cases := []struct {
		name  string
		sites []promptAuditSite
		want  bool
	}{
		{name: "the system prompt alone", sites: []promptAuditSite{promptSiteSystem}, want: true},
		{name: "the task scaffolding alone", sites: []promptAuditSite{promptSiteTask}, want: true},
		{name: "both builder sites", sites: []promptAuditSite{promptSiteSystem, promptSiteTask}, want: true},
		{name: "the case text alone", sites: []promptAuditSite{promptSiteCase}, want: false},
		{name: "no site at all", sites: nil, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			audited := promptAuditCase{Findings: []promptAuditFinding{
				{Kind: promptLeakTool, Value: "gitlab_branch", Sites: testCase.sites},
			}}
			if got := audited.coachedByBuilder(); got != testCase.want {
				t.Errorf("coachedByBuilder() with sites %v = %v, want %v", testCase.sites, got, testCase.want)
			}
		})
	}
}

// TestComparePromptAuditFindings_OrdersByKindBeforeValue pins the order the
// artifact's diffability rests on. Each case is one where kind and value
// disagree, so an ordering that reads only one of the two fails.
func TestComparePromptAuditFindings_OrdersByKindBeforeValue(t *testing.T) {
	cases := []struct {
		name  string
		left  promptAuditFinding
		right promptAuditFinding
		want  int
	}{
		{
			name:  "an earlier kind wins a later value",
			left:  promptAuditFinding{Kind: promptLeakTool, Value: "z"},
			right: promptAuditFinding{Kind: promptLeakAction, Value: "a"},
			want:  -1,
		},
		{
			name:  "a later kind loses to an earlier one",
			left:  promptAuditFinding{Kind: promptLeakEnvelope, Value: "a"},
			right: promptAuditFinding{Kind: promptLeakTool, Value: "z"},
			want:  1,
		},
		{
			name:  "one kind orders by value",
			left:  promptAuditFinding{Kind: promptLeakParam, Value: "branch"},
			right: promptAuditFinding{Kind: promptLeakParam, Value: "project_id"},
			want:  -1,
		},
		{
			name:  "the same finding is equal",
			left:  promptAuditFinding{Kind: promptLeakParam, Value: "branch"},
			right: promptAuditFinding{Kind: promptLeakParam, Value: "branch"},
			want:  0,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := comparePromptAuditFindings(testCase.left, testCase.right); got != testCase.want {
				t.Errorf("comparePromptAuditFindings(%+v, %+v) = %d, want %d", testCase.left, testCase.right, got, testCase.want)
			}
		})
	}
}

// TestPromptAuditKindRank_PlacesEveryKnownKindAndSendsTheRestLast pins the rank
// of every kind, the first one included: a rank table asked only about an
// unknown kind leaves the position of the first free.
func TestPromptAuditKindRank_PlacesEveryKnownKindAndSendsTheRestLast(t *testing.T) {
	for index, kind := range promptAuditKindOrder {
		t.Run(string(kind), func(t *testing.T) {
			if rank := promptAuditKindRank(kind); rank != index {
				t.Errorf("promptAuditKindRank(%s) = %d, want %d", kind, rank, index)
			}
		})
	}
	t.Run("an unknown kind sorts last", func(t *testing.T) {
		if rank := promptAuditKindRank("not a kind"); rank != len(promptAuditKindOrder) {
			t.Errorf("promptAuditKindRank(unknown) = %d, want %d", rank, len(promptAuditKindOrder))
		}
	})
}

// TestPromptAuditTotals_CountTheAnswerKeyedSitesApart drives the counterfactual
// dimension with the four shapes a case can have, so each of its three counters
// is pinned on its own rather than moving with the others.
func TestPromptAuditTotals_CountTheAnswerKeyedSitesApart(t *testing.T) {
	report := promptAuditReport{Surface: config.ToolSurfaceDynamic, Cases: []promptAuditCase{
		{ID: "PA-020", CaseTextInPrompt: true},
		{ID: "PA-021", CaseTextInPrompt: true, AnswerKeyed: []promptAuditSite{promptSiteSystem}},
		{ID: "PA-022", CaseTextInPrompt: true, AnswerKeyed: []promptAuditSite{promptSiteTask}},
		{ID: "PA-023", CaseTextInPrompt: true, AnswerKeyed: []promptAuditSite{promptSiteSystem, promptSiteTask}},
	}}
	totals := report.totals()
	if totals.Cases != 4 || totals.Leaking != 0 {
		t.Errorf("cases/leaking = %d/%d, want 4/0", totals.Cases, totals.Leaking)
	}
	if totals.AnswerKeyed != 3 {
		t.Errorf("answer-keyed cases = %d, want 3 of the 4", totals.AnswerKeyed)
	}
	if totals.AnswerKeyedSystem != 2 || totals.AnswerKeyedTask != 2 {
		t.Errorf("answer-keyed sites = system %d, task %d, want 2 and 2", totals.AnswerKeyedSystem, totals.AnswerKeyedTask)
	}
}

// TestPromptAuditScaffold_SeparatesTheBuilderFromTheCaseText verifies that the
// case's own words are taken out of the task prompt before the scaffolding is
// searched, and that a prompt which never carried them says so.
func TestPromptAuditScaffold_SeparatesTheBuilderFromTheCaseText(t *testing.T) {
	cases := []struct {
		name         string
		prompt       string
		caseText     string
		wantScaffold string
		wantIncluded bool
	}{
		{name: "case text removed once", prompt: "Task A: delete it\nDestructive: yes", caseText: "delete it", wantScaffold: "Task A: \nDestructive: yes", wantIncluded: true},
		{name: "case text absent", prompt: "Exact required call: {}", caseText: "delete it", wantScaffold: "Exact required call: {}", wantIncluded: false},
		{name: "no case text", prompt: "Task A", caseText: "", wantScaffold: "Task A", wantIncluded: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			scaffold, included := promptAuditScaffold(testCase.prompt, testCase.caseText)
			if scaffold != testCase.wantScaffold || included != testCase.wantIncluded {
				t.Errorf("promptAuditScaffold() = (%q, %v), want (%q, %v)", scaffold, included, testCase.wantScaffold, testCase.wantIncluded)
			}
		})
	}
}

// TestPromptAuditBlindTask_DropsTheAnswerAndKeepsTheRequest verifies the
// counterfactual the answer-keyed column rests on.
func TestPromptAuditBlindTask_DropsTheAnswerAndKeepsTheRequest(t *testing.T) {
	task := evalTask{
		ID:     "PA-002",
		Prompt: "Delete branch `tmp` in project `group/project`.",
		Steps: []evalStep{{
			ExpectedTool:   "gitlab_branch",
			ExpectedAction: "delete",
			RequiredParams: []string{"project_id", "branch"},
			OptionalParams: []string{"confirm"},
			Destructive:    true,
		}},
	}
	blind := promptAuditBlindTask(task)
	step := blind.Steps[0]
	if step.ExpectedTool != "" || step.ExpectedAction != "" || step.RequiredParams != nil || step.OptionalParams != nil {
		t.Errorf("blinded step = %+v, want its answer key removed", step)
	}
	if !step.Destructive || blind.Prompt != task.Prompt {
		t.Errorf("blinded task dropped the request: destructive = %v, prompt = %q", step.Destructive, blind.Prompt)
	}
	if original := task.Steps[0]; original.ExpectedAction != "delete" || len(original.RequiredParams) != 2 {
		t.Errorf("promptAuditBlindTask mutated the task it was given: %+v", original)
	}
}

// TestPromptAuditReport_Renders_TheSameBytesTwice verifies the property the
// artifact is for: the report carries no clock and no path, so a deletion is
// reviewed as a diff over it rather than as a diff over its timestamps.
func TestPromptAuditReport_Renders_TheSameBytesTwice(t *testing.T) {
	report := samplePromptAuditReport(t)
	summary := renderPromptAuditSummary(report)
	if summary != renderPromptAuditSummary(report) {
		t.Fatal("renderPromptAuditSummary() is not stable across calls")
	}
	dump := renderPromptAuditDump(report)
	if !strings.HasPrefix(dump, summary) {
		t.Error("the dump does not open with the summary")
	}
	cases := []struct {
		name     string
		rendered string
		want     string
	}{
		{name: "case table header", rendered: summary, want: "| Case | Surface | Destructive |"},
		{name: "case row", rendered: summary, want: "| PA-003 | meta |"},
		{name: "case count", rendered: summary, want: "Cases audited: 1"},
		{name: "kind totals", rendered: summary, want: "| confirm | 1 |"},
		{name: "dump findings", rendered: dump, want: "## Findings"},
		{name: "dump prompts", rendered: dump, want: "## Prompts"},
		{name: "dump case heading", rendered: dump, want: "### PA-003"},
		{name: "dump task prompt", rendered: dump, want: "Task prompt:"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !strings.Contains(testCase.rendered, testCase.want) {
				t.Errorf("rendered report missing %q:\n%s", testCase.want, testCase.rendered)
			}
		})
	}
}

// TestRunPromptAudit_WritesTheSummaryAndTheDumpWhereItSaysItDid verifies the
// command path: the summary reaches the caller's writer, the dump reaches
// --out, and the caller is told where it went.
func TestRunPromptAudit_WritesTheSummaryAndTheDumpWhereItSaysItDid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "audit.md")
	opts := options{ToolSurface: config.ToolSurfaceMeta, Backend: backendMock, Edition: editionAll, Output: path}
	var out bytes.Buffer
	if err := runPromptAudit(&out, opts, []evalTask{samplePromptAuditTask()}); err != nil {
		t.Fatalf("runPromptAudit() error = %v", err)
	}
	if !strings.Contains(out.String(), path) {
		t.Errorf("summary does not name the dump it wrote:\n%s", out.String())
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	if !strings.Contains(string(written), "## Prompts") {
		t.Errorf("dump at %s carries no prompts", path)
	}
	var quiet bytes.Buffer
	if quietErr := runPromptAudit(&quiet, options{ToolSurface: config.ToolSurfaceMeta, Backend: backendMock}, []evalTask{samplePromptAuditTask()}); quietErr != nil {
		t.Fatalf("runPromptAudit() without --out error = %v", quietErr)
	}
	if strings.Contains(quiet.String(), "## Prompts") {
		t.Error("the summary carried the whole dump when no --out was given")
	}
}

// TestRunPromptAudit_ReportsAnUnwritableDumpPath verifies the audit fails
// rather than reporting a dump it could not write.
func TestRunPromptAudit_ReportsAnUnwritableDumpPath(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	opts := options{ToolSurface: config.ToolSurfaceMeta, Backend: backendMock, Output: filepath.Join(blocked, "audit.md")}
	err := runPromptAudit(&bytes.Buffer{}, opts, []evalTask{samplePromptAuditTask()})
	if err == nil {
		t.Fatal("runPromptAudit() error = nil, want a failure naming the dump path")
	}
	if !strings.Contains(err.Error(), "prompt audit") {
		t.Errorf("error = %v, want it to name the prompt audit", err)
	}
}

// TestPromptAuditTotals_SeparateBuilderCoachingFromTheCasesOwnText drives the
// aggregation with cases the corpus cannot produce today: one that repeats
// nothing, one whose only repetition is its own text, and one destructive case
// whose stimulus never names confirm. Those are the states the next steps'
// deletions are supposed to reach, so the totals must already count them.
func TestPromptAuditTotals_SeparateBuilderCoachingFromTheCasesOwnText(t *testing.T) {
	report := promptAuditReport{Surface: config.ToolSurfaceMeta, Cases: []promptAuditCase{
		{
			ID: "PA-010", CaseTextInPrompt: true,
			Answers: []promptAuditAnswer{{Kind: promptLeakAction, Value: "get"}, {Kind: promptLeakConfirm, Value: promptAuditConfirmLiteral}},
		},
		{
			ID: "PA-011", CaseTextInPrompt: true,
			Answers:  []promptAuditAnswer{{Kind: promptLeakAction, Value: "get"}},
			Findings: []promptAuditFinding{{Kind: promptLeakAction, Value: "get", Sites: []promptAuditSite{promptSiteCase}}},
		},
		{
			ID: "PA-012", Destructive: true, CaseTextInPrompt: false,
			Answers:  []promptAuditAnswer{{Kind: promptLeakTool, Value: "gitlab_branch"}, {Kind: promptLeakConfirm, Value: promptAuditConfirmLiteral}},
			Findings: []promptAuditFinding{{Kind: promptLeakTool, Value: "gitlab_branch", Sites: []promptAuditSite{promptSiteSystem, promptSiteTask}}},
		},
	}}
	totals := report.totals()
	if totals.Cases != 3 || totals.Leaking != 2 || totals.BeyondConfirm != 2 {
		t.Errorf("cases/leaking/beyond confirm = %d/%d/%d, want 3/2/2", totals.Cases, totals.Leaking, totals.BeyondConfirm)
	}
	if totals.BuilderCoached != 1 || totals.CaseTextOnly != 1 || totals.CaseTextDropped != 1 {
		t.Errorf("builder/case-only/dropped = %d/%d/%d, want 1/1/1", totals.BuilderCoached, totals.CaseTextOnly, totals.CaseTextDropped)
	}
	if totals.Destructive != 1 || totals.DestructiveConfirmNamed != 0 {
		t.Errorf("destructive = %d, of which confirm named = %d, want 1 and 0", totals.Destructive, totals.DestructiveConfirmNamed)
	}
	action := totals.ByKind[promptLeakAction]
	if action.Declared != 2 || action.Named != 1 || action.Cases != 1 || action.System != 0 || action.Task != 0 || action.Case != 1 {
		t.Errorf("action totals = %+v, want 2 declared, 1 named in 1 case, sited in the case text alone", action)
	}
	tool := totals.ByKind[promptLeakTool]
	if tool.System != 1 || tool.Task != 1 || tool.Case != 0 {
		t.Errorf("tool sites = system %d, task %d, case %d, want 1/1/0", tool.System, tool.Task, tool.Case)
	}
}

// TestPromptAuditCells_RenderTheAbsenceOfAFindingAsADash covers the small
// renderers, where an unmeasured cell must not read as a measured zero.
func TestPromptAuditCells_RenderTheAbsenceOfAFindingAsADash(t *testing.T) {
	audited := promptAuditCase{
		Answers: []promptAuditAnswer{
			{Kind: promptLeakParam, Value: "project_id"},
			{Kind: promptLeakParam, Value: "branch"},
		},
		Findings: []promptAuditFinding{{Kind: promptLeakParam, Value: "project_id", Sites: []promptAuditSite{promptSiteSystem, promptSiteCase}}},
	}
	if got := promptAuditCell(audited, promptLeakParam); got != "1/2 system+case" {
		t.Errorf("promptAuditCell(param) = %q, want %q", got, "1/2 system+case")
	}
	if got := promptAuditCell(audited, promptLeakEnvelope); got != "-" {
		t.Errorf("promptAuditCell(envelope) = %q, want a dash", got)
	}
	if got := promptAuditSitesCell(nil); got != "-" {
		t.Errorf("promptAuditSitesCell(nil) = %q, want a dash", got)
	}
	if got := promptAuditSitesCell([]promptAuditSite{promptSiteTask}); got != "task" {
		t.Errorf("promptAuditSitesCell(task) = %q", got)
	}
	if yesNo(true) != "yes" || yesNo(false) != "no" {
		t.Errorf("yesNo() = %q/%q", yesNo(true), yesNo(false))
	}
	if rank := promptAuditKindRank("not a kind"); rank != len(promptAuditKindOrder) {
		t.Errorf("promptAuditKindRank(unknown) = %d, want %d", rank, len(promptAuditKindOrder))
	}
}

// TestAuditPromptsForTask_WithoutCaseText_SitesNothingInTheCase verifies that
// the case's own text is a site only where the prompt actually carries it.
func TestAuditPromptsForTask_WithoutCaseText_SitesNothingInTheCase(t *testing.T) {
	task := evalTask{ID: "PA-013", Steps: []evalStep{{ExpectedTool: "gitlab_branch", ExpectedAction: "delete", RequiredParams: []string{"project_id"}}}}
	audited := auditPromptsForTask(task, config.ToolSurfaceMeta)
	if audited.CaseTextInPrompt {
		t.Error("a task with no prompt text reported its case text as present")
	}
	for _, finding := range audited.Findings {
		if slices.Contains(finding.Sites, promptSiteCase) {
			t.Errorf("%s %s sited in a case text that does not exist", finding.Kind, finding.Value)
		}
	}
}

// TestRunPromptAudit_ReportsAWriterThatRefusesTheSummary verifies the audit
// fails rather than reporting a summary nobody received, on both writes: the
// summary itself and the line naming the dump.
func TestRunPromptAudit_ReportsAWriterThatRefusesTheSummary(t *testing.T) {
	cases := []struct {
		name      string
		output    string
		failAfter int
	}{
		{name: "summary write", failAfter: 0},
		{name: "dump location write", output: filepath.Join(t.TempDir(), "audit.md"), failAfter: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			opts := options{ToolSurface: config.ToolSurfaceMeta, Backend: backendMock, Output: testCase.output}
			err := runPromptAudit(&refusingWriter{accept: testCase.failAfter}, opts, []evalTask{samplePromptAuditTask()})
			if err == nil || !strings.Contains(err.Error(), "write prompt audit") {
				t.Fatalf("runPromptAudit() error = %v, want it to name the write it could not make", err)
			}
		})
	}
}

// TestWritePromptAuditDump_ReportsAPathThatIsADirectory covers the other half
// of the dump's failure: the directory exists and the file cannot be written.
func TestWritePromptAuditDump_ReportsAPathThatIsADirectory(t *testing.T) {
	err := writePromptAuditDump(t.TempDir(), promptAuditReport{Surface: config.ToolSurfaceMeta})
	if err == nil || !strings.Contains(err.Error(), "write prompt audit dump") {
		t.Fatalf("writePromptAuditDump(directory) error = %v, want a write failure", err)
	}
}

// refusingWriter accepts a fixed number of writes and then refuses.
type refusingWriter struct {
	accept int
}

// Write accepts until the allowance runs out.
func (w *refusingWriter) Write(p []byte) (int, error) {
	if w.accept <= 0 {
		return 0, errors.New("writer is full")
	}
	w.accept--
	return len(p), nil
}

// samplePromptAuditTask is one small meta case with an answer key to find.
func samplePromptAuditTask() evalTask {
	return evalTask{
		ID:     "PA-003",
		Prompt: "Delete branch `tmp` in project `group/project`.",
		Steps: []evalStep{{
			ExpectedTool:   "gitlab_branch",
			ExpectedAction: "delete",
			RequiredParams: []string{"project_id", "branch"},
			Destructive:    true,
		}},
	}
}

// samplePromptAuditReport audits the sample case, which keeps the rendering
// tests off the corpus and off the catalog.
func samplePromptAuditReport(t *testing.T) promptAuditReport {
	t.Helper()
	opts := options{ToolSurface: config.ToolSurfaceMeta, Backend: backendMock, Edition: editionAll}
	report := auditPrompts(opts, []evalTask{samplePromptAuditTask()})
	if len(report.Cases) != 1 {
		t.Fatalf("auditPrompts() audited %d cases, want 1", len(report.Cases))
	}
	return report
}
