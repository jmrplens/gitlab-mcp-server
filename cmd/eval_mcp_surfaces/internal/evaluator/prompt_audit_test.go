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

// exactCallPromptSignature is the sentence exactToolTaskPrompt used to write,
// kept after V05 deleted that builder because a deletion is only permanent
// while something still looks for what it removed. Any prompt that says this
// again is handing a model the call it is about to be scored on.
const exactCallPromptSignature = "Exact required call: use the "

// exactCallPromptCases are the twelve cases whose meta task prompt was built
// by exactToolTaskPrompt before V05 deleted it, measured against the tree at
// the time rather than copied from a plan.
//
// The list outlives the builder on purpose. A test that only asserted "no
// prompt carries the signature" would pass just as well if these twelve cases
// disappeared from the corpus, which is the opposite of what V05 claims: they
// are still evaluated, and they are evaluated without being told the answer.
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

// TestAuditPrompts_MetaExactCallBuilder_ReachesNoCase pins V05's deletion.
//
// Two assertions, and neither is enough alone. That no prompt carries the
// builder's sentence is the deletion; that the twelve cases which used to
// carry it are still audited is what stops the first assertion passing because
// the corpus lost them. A deletion that also deleted the measurement would
// look identical on the first check and be worthless.
func TestAuditPrompts_MetaExactCallBuilder_ReachesNoCase(t *testing.T) {
	report := promptAuditForSurface(t, config.ToolSurfaceMeta)
	if exact := casesCarrying(report, exactCallPromptSignature); len(exact) > 0 {
		t.Errorf("cases reaching the deleted exact-call builder = %v, want none", exact)
	}
	audited := make(map[string]bool, len(report.Cases))
	for _, one := range report.Cases {
		audited[one.ID] = true
	}
	for _, id := range exactCallPromptCases {
		if !audited[id] {
			t.Errorf("case %s is no longer audited: this test would then pass for the wrong reason", id)
		}
	}
}

// TestAuditPrompts_MetaEnvelopes_AreGone pins the end of the envelope leak.
//
// It had two sources and they were removed one step apart. V04 measured
// eighteen meta cases handed a marshaled `{"action":…,"params":{…}}` naming
// their own expected action. V05 deleted the exact-call builder and nine
// remained, three of them cases the builder had been *masking* a guidance
// clause for. V06 deletes those clauses and the population is empty.
//
// Empty is asserted on both surfaces, because the whole claim is that no case
// on either is handed the call it is scored on emitting, and a check of meta
// alone would have passed throughout the period when dynamic was the surface
// the README publishes.
func TestAuditPrompts_MetaEnvelopes_AreGone(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceMeta, config.ToolSurfaceDynamic} {
		t.Run(surface, func(t *testing.T) {
			report := promptAuditForSurface(t, surface)
			if withEnvelope := casesWithFindingKind(report, promptLeakEnvelope); len(withEnvelope) > 0 {
				t.Errorf("cases carrying a marshaled envelope = %v, want none", withEnvelope)
			}
			if exact := casesCarrying(report, exactCallPromptSignature); len(exact) > 0 {
				t.Errorf("cases reaching the deleted exact-call builder = %v, want none", exact)
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
		// A dot ends an English sentence as often as it joins an action ID, and
		// the rule used to count both. These four are the pair that separates
		// them: the dot is code only where an identifier continues past it.
		{name: "a sentence-final period is not a dotted identifier", text: "fetch job `999` trace.", value: "trace", want: false},
		{name: "a sentence-final period after a param is prose too", text: "render that exact prompt name.", value: "name", want: false},
		{name: "a dot followed by an identifier is code", text: "call project.get now", value: "project", want: true},
		{name: "a dot preceded by an identifier is code", text: "call project.get now", value: "get", want: true},
		{name: "an opening brace is code", text: "send {id}", value: "id", want: true},
		{name: "an equals sign names the field before it", text: "merge with `auto_merge=true`", value: "auto_merge", want: true},
		{name: "an equals sign after a bare word counts too", text: "list them first=5 at a time", value: "first", want: true},
		{name: "an equals sign does not make the value a name", text: "pass paused=true", value: "true", want: false},
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

// TestPromptAuditSentCaseText_PrefersTheTemplateARunWouldSend verifies which of
// a case's two texts the audit measures. A case carrying a prompt template has
// that template rendered over its plain prompt before a live run sends
// anything, so the template is the stimulus and the plain field is a string
// nobody receives. The template's own actions stand in for fixture outputs and
// are replaced, since a value GitLab minted carries no catalog name and the
// unreplaced action would carry a parameter name the value does not.
func TestPromptAuditSentCaseText_PrefersTheTemplateARunWouldSend(t *testing.T) {
	tests := []struct {
		name string
		task evalTask
		want string
	}{
		{
			name: "a case with no template is its plain prompt",
			task: evalTask{Prompt: "Delete the file.", Case: &EvalCase{}},
			want: "Delete the file.",
		},
		{
			name: "a case outside the registry is its plain prompt",
			task: evalTask{Prompt: "Delete the file."},
			want: "Delete the file.",
		},
		{
			name: "a template replaces the plain prompt",
			task: evalTask{Prompt: "Delete the file.", Case: &EvalCase{PromptTemplate: CasePromptTemplate{Text: "Delete the file, calling repository.file_delete."}}},
			want: "Delete the file, calling repository.file_delete.",
		},
		{
			name: "a template action stands for a fixture value",
			task: evalTask{Prompt: "x", Case: &EvalCase{PromptTemplate: CasePromptTemplate{Text: "Delete `{{ .Values.file_path }}` from `{{.Project.Path}}`."}}},
			want: "Delete `the-fixture-value` from `the-fixture-value`.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := promptAuditSentCaseText(tc.task); got != tc.want {
				t.Errorf("promptAuditSentCaseText() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAuditPromptsForTask_ReadsTheTemplatesOwnCoaching is the regression for
// what reading the plain prompt hid. MT-031's template tells the model which
// action to call and which to avoid; its plain prompt says none of that, so
// before the audit read the sent text this case reported clean while every
// Docker run sent the sentence.
func TestAuditPromptsForTask_ReadsTheTemplatesOwnCoaching(t *testing.T) {
	task := evalTask{
		ID:     "PA-014",
		Prompt: "Delete file `tmp/eval.txt` from branch `feature/eval` in project `my-org/tools/x`.",
		Case: &EvalCase{PromptTemplate: CasePromptTemplate{
			Text: "Delete file `{{ .Values.file_path }}` from branch `{{ .Branch.Name }}`. Call repository.file_delete directly; do not call repository.tree.",
		}},
		Steps: []evalStep{{ExpectedTool: "gitlab_repository", ExpectedAction: "repository.file_delete", RequiredParams: []string{"project_id"}}},
	}
	audited := auditPromptsForTask(task, config.ToolSurfaceMeta)
	var named bool
	for _, finding := range audited.Findings {
		if finding.Kind == promptLeakAction && finding.Value == "repository.file_delete" && slices.Contains(finding.Sites, promptSiteCase) {
			named = true
		}
	}
	if !named {
		t.Errorf("the template naming its own action was not sited in the case text; findings: %+v", audited.Findings)
	}
}

// TestPromptAuditAnswers_TakeTheOptionalParamsAndCountConfirmOnce verifies the
// width of the answer key. An optional parameter is part of the call the scorer
// builds, so a stimulus naming one has handed that much over; confirm is left
// to its own kind, which is searched for in every case, so the
// destructive-safety column keeps counting it exactly once.
func TestPromptAuditAnswers_TakeTheOptionalParamsAndCountConfirmOnce(t *testing.T) {
	task := evalTask{
		ID:    "PA-015",
		Steps: []evalStep{{ExpectedTool: "gitlab_environment", ExpectedAction: "stop", RequiredParams: []string{"project_id"}, OptionalParams: []string{"force", "confirm"}}},
	}
	answers := promptAuditAnswers(task, config.ToolSurfaceMeta)
	var params, confirms int
	var sawForce bool
	for _, answer := range answers {
		switch answer.Kind {
		case promptLeakParam:
			params++
			if answer.Value == "force" {
				sawForce = true
			}
			if answer.Value == promptAuditConfirmLiteral {
				t.Error("confirm was counted as an ordinary parameter as well as its own kind")
			}
		case promptLeakConfirm:
			confirms++
		case promptLeakTool, promptLeakAction, promptLeakEnvelope:
		}
	}
	if !sawForce {
		t.Error("the optional parameter force is not in the answer key")
	}
	if params != 2 {
		t.Errorf("parameter literals = %d, want 2 (project_id and force)", params)
	}
	if confirms != 1 {
		t.Errorf("confirm literals = %d, want 1", confirms)
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

// TestAuditPrompts_NoStimulusIsAnswerKeyed is what the three V04 tests that
// pinned the leak became.
//
// They asserted the opposite of this, one measurement each, because each was
// written to fail on the step that removed what it measured: that every dynamic
// task prompt changed when the answer key was taken away, that both system
// prompts named confirm for every case whether destructive or not, and that the
// dynamic prompt carried its operation count, its project.get clause and its
// destructive clause. V07 removed all of it, so the three collapse into one
// statement with nothing left to enumerate.
//
// The confirm figure is the one worth keeping an eye on. The published
// destructive-safety column read 100.0% on every row because every stimulus
// said where confirm goes; both surfaces publish that themselves, in a schema
// property on meta and in the execute tool's own description on dynamic, so
// what that column measures now is whether a model reads the surface.
func TestAuditPrompts_NoStimulusIsAnswerKeyed(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta} {
		t.Run(surface, func(t *testing.T) {
			report := promptAuditForSurface(t, surface)
			totals := report.totals()
			if totals.Destructive == 0 {
				t.Fatal("no destructive case audited, so the confirm assertion below would pass for the wrong reason")
			}
			if totals.DestructiveConfirmNamed != 0 {
				t.Errorf("%d destructive cases are told where confirm goes, want none", totals.DestructiveConfirmNamed)
			}
			if named := totals.ByKind[promptLeakConfirm].System; named != 0 {
				t.Errorf("the system prompt names confirm for %d cases, want none", named)
			}
			for _, audited := range report.Cases {
				if slices.Contains(audited.AnswerKeyed, promptSiteTask) {
					t.Errorf("%s: its task prompt still changes when the answer key is removed", audited.ID)
				}
				if slices.Contains(audited.AnswerKeyed, promptSiteSystem) {
					t.Errorf("%s: its system prompt still changes when the answer key is removed", audited.ID)
				}
			}
		})
	}
}

// TestPromptAuditBuilderSitesAreClean_PassesOnBothSurfaces runs the gate the
// CLI runs, through the same function, so `go test ./cmd/...` fails for the
// same reason `make check-eval-prompts` does.
//
// Calling the production function rather than restating its rule is the point:
// a unit test that reimplements a gate can pass while the gate fails, and the
// only way anyone would find out is CI.
func TestPromptAuditBuilderSitesAreClean_PassesOnBothSurfaces(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceMeta, config.ToolSurfaceDynamic} {
		t.Run(surface, func(t *testing.T) {
			var out strings.Builder
			if err := promptAuditIsClean(&out, promptAuditForSurface(t, surface)); err != nil {
				t.Errorf("the prompts this package writes carry an answer: %v", err)
			}
		})
	}
}

// TestPromptAuditBuilderSitesAreClean_NamesWhatFailedIt drives the refusal
// path, which the passing corpus never reaches.
//
// Both ways in are covered because they answer different questions and a gate
// that lost either would still look healthy: a literal finding is a prompt
// naming the answer, and an answer-keyed site is a prompt that came back
// different without naming anything.
func TestPromptAuditIsClean_NamesWhatFailedIt(t *testing.T) {
	tests := []struct {
		name   string
		report promptAuditReport
		want   string
	}{
		{
			name: "a literal named in the task prompt",
			report: promptAuditReport{Cases: []promptAuditCase{{
				ID:       "MT-001",
				Findings: []promptAuditFinding{{Kind: promptLeakAction, Value: "issue.update", Sites: []promptAuditSite{promptSiteTask}}},
			}}},
			want: `MT-001: action "issue.update"`,
		},
		{
			name: "a system prompt that changes without the answer key",
			report: promptAuditReport{Cases: []promptAuditCase{{
				ID:          "MT-002",
				AnswerKeyed: []promptAuditSite{promptSiteSystem},
			}}},
			want: "MT-002: its system prompt changes when the answer key is removed",
		},
		{
			name: "the case's own words fail it too, when nothing declares them",
			report: promptAuditReport{Cases: []promptAuditCase{{
				ID:          "MT-003",
				AnswerKeyed: []promptAuditSite{promptSiteCase},
				Findings:    []promptAuditFinding{{Kind: promptLeakParam, Value: "project_id", Sites: []promptAuditSite{promptSiteCase}}},
			}}},
			want: `MT-003: its own text names param "project_id", and no declaration says why`,
		},
		{
			name: "a declared case literal passes",
			report: promptAuditReport{Cases: []promptAuditCase{{
				ID:       "MT-004",
				Findings: []promptAuditFinding{{Kind: promptLeakAction, Value: "project.get", Sites: []promptAuditSite{promptSiteCase}, Category: promptCategoryLiteralIsTheRequest}},
			}}},
			want: "",
		},
		{
			name:   "a declaration that covers nothing is itself a finding",
			report: promptAuditReport{StaleDeclarations: []string{"meta MS-999 action project.get"}},
			want:   "meta MS-999 action project.get: declared, and nothing on this surface carries it any more",
		},
		{
			name:   "a declaration the gate cannot act on fails before anything else",
			report: promptAuditReport{InvalidDeclarations: []string{"individual MS-040 tool gitlab_project: names a surface the audit never renders"}},
			want:   "names a surface the audit never renders",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			err := promptAuditIsClean(&out, tc.report)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("the gate refused a report whose only repetition is the case's own words: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("the gate accepted a coached report")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to name %q", err, tc.want)
			}
		})
	}
}
