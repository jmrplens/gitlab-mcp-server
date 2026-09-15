package evaluator

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The prompt audit answers one question offline: for every selected case, does
// the stimulus a run would send already contain the answer that run scores?
//
// It exists because nothing else could answer it. A dry run builds no prompts
// at all, so the only way to count the coaching was to read the builders by
// hand, which two readings of the same source did and disagreed about. This
// renders the exact prompts through the same two entry points the runner calls
// and counts, so a third reader gets the same number as the first two.
//
// It gates as well as reports. Every site is judged: the prompts this package
// writes must name nothing of the answer, and a case whose own text names part
// of it is refused unless prompt_declarations.go records why that word is the
// request rather than the answer.

// promptAuditSite says where in the stimulus an answer literal was found. The
// three are not equally damning and the report keeps them apart: the system
// prompt says the same thing for every case, the scaffolding is what the
// builder added around this case, and the case text is what a user wrote.
type promptAuditSite string

const (
	// promptSiteSystem is the system prompt, identical for every case on a
	// surface except where systemPromptForTask selects the reduced one.
	promptSiteSystem promptAuditSite = "system"
	// promptSiteTask is the task prompt outside the case's own text: the
	// guidance, the destructive line and the exact-call envelopes the
	// builder wraps around it.
	promptSiteTask promptAuditSite = "task"
	// promptSiteCase is the case's own user-authored text. No deletion
	// in this package can change it, so it is the one site a
	// declaration may excuse.
	promptSiteCase promptAuditSite = "case"
)

// promptAuditKind names which part of a case's answer key a prompt repeated.
type promptAuditKind string

const (
	// promptLeakTool is the step's ExpectedTool.
	promptLeakTool promptAuditKind = "tool"
	// promptLeakAction is the step's ExpectedAction.
	promptLeakAction promptAuditKind = "action"
	// promptLeakParam is a name declared in the step's RequiredParams.
	promptLeakParam promptAuditKind = "param"
	// promptLeakConfirm is the literal the destructive-safety column scores.
	promptLeakConfirm promptAuditKind = "confirm"
	// promptLeakEnvelope is a marshaled {"action":...} object naming the
	// expected action, which is the whole call rather than one field of it.
	promptLeakEnvelope promptAuditKind = "envelope"
)

// promptAuditConfirmLiteral is searched for in every case, destructive or not,
// because the published destructive-safety column scores every case.
const promptAuditConfirmLiteral = "confirm"

// promptAuditKindOrder fixes the order kinds appear in, so two runs of the
// audit produce byte-identical reports and a diff shows only what changed.
var promptAuditKindOrder = []promptAuditKind{
	promptLeakTool,
	promptLeakAction,
	promptLeakParam,
	promptLeakConfirm,
	promptLeakEnvelope,
}

// promptAuditAnswer is one literal from a case's answer key.
type promptAuditAnswer struct {
	Kind  promptAuditKind
	Value string
}

// promptAuditFinding is one answer literal found in the stimulus, with every
// site it was found at.
type promptAuditFinding struct {
	Kind  promptAuditKind
	Value string
	Sites []promptAuditSite
	// Category names the declaration that accepts this finding at the case
	// site, and is empty for every finding nothing declares. Only the case
	// site is declarable: a literal the builder writes is this package's to
	// delete, so a declaration there would excuse the one thing the gate
	// exists to refuse.
	Category string
}

// promptAuditCase is one case rendered for one surface.
type promptAuditCase struct {
	ID          string
	Surface     string
	Destructive bool
	// CaseTextInPrompt is false when the builder dropped the user's own
	// request and sent only its own instructions, which a few compact
	// exact-call prompts do.
	CaseTextInPrompt bool
	SystemPrompt     string
	UserPrompt       string
	CaseText         string
	Answers          []promptAuditAnswer
	Findings         []promptAuditFinding
	// AnswerKeyed names the prompts that change when the answer key is taken
	// away. It catches the coaching that names nothing: a clause selected by
	// the expected steps, such as the count of catalog operations a dynamic
	// task prompt states, carries the answer's shape without carrying any of
	// its words, so no search for a literal can see it.
	AnswerKeyed []promptAuditSite
}

// promptAuditReport is one audit of one surface.
type promptAuditReport struct {
	Surface    string
	ServerMode string
	Edition    string
	Backend    string
	Cases      []promptAuditCase
	// DeclaredFindings counts the case-site findings a declaration accepted.
	DeclaredFindings int
	// StaleDeclarations names, by key, every declaration for this surface
	// that covered no finding. A declaration that stops describing the
	// corpus is a finding of its own, which is what stops the table
	// outliving the case text it was written about.
	StaleDeclarations []string
	// InvalidDeclarations names every declaration the gate refuses to act
	// on, with what is wrong with it.
	InvalidDeclarations []string
}

// promptAuditKindTotals counts one kind across the corpus.
type promptAuditKindTotals struct {
	Declared int
	Named    int
	Cases    int
	System   int
	Task     int
	Case     int
}

// promptAuditTotals is the number the next steps are measured against.
type promptAuditTotals struct {
	Cases int
	// Leaking saturates: the system prompt of both surfaces names confirm for
	// every case, so this reads 100% until that one clause goes.
	// BeyondConfirm is the figure a deletion moves.
	Leaking                 int
	BeyondConfirm           int
	BuilderCoached          int
	CaseTextOnly            int
	CaseTextDropped         int
	Destructive             int
	DestructiveConfirmNamed int
	AnswerKeyed             int
	AnswerKeyedSystem       int
	AnswerKeyedTask         int
	ByKind                  map[promptAuditKind]promptAuditKindTotals
}

// runPromptAudit renders the stimulus for every selected case and reports what
// of the answer key it already contains. It calls no provider and needs no
// GitLab, so it runs anywhere the corpus compiles.
//
// The summary goes to w; the full dump, prompts included, is written to
// opts.Output when one was given, because it is the before-artifact the
// deletions that follow are reviewed as a diff over.
func runPromptAudit(w io.Writer, opts options, tasks []evalTask) error {
	report := auditPrompts(opts, tasks)
	if _, err := io.WriteString(w, renderPromptAuditSummary(report)); err != nil {
		return fmt.Errorf("write prompt audit: %w", err)
	}
	// The check runs first and its verdict is held rather than returned,
	// because a failing gate is exactly when the dump is worth reading: it
	// carries the prompt that broke it. Returning early would have made
	// --audit-prompts -check --out silently produce no artifact on the one
	// run anybody would go looking for one.
	var checkErr error
	if opts.AuditPromptsCheck {
		checkErr = promptAuditIsClean(w, report)
	}
	if strings.TrimSpace(opts.Output) == "" {
		return checkErr
	}
	if err := writePromptAuditDump(opts.Output, report); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "\nFull dump with every prompt: %s\n", opts.Output); err != nil {
		return fmt.Errorf("write prompt audit: %w", err)
	}
	return checkErr
}

// writePromptAuditDump writes the full artifact, prompts included.
func writePromptAuditDump(path string, report promptAuditReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create prompt audit directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(renderPromptAuditDump(report)), 0o600); err != nil {
		return fmt.Errorf("write prompt audit dump: %w", err)
	}
	return nil
}

// auditPrompts renders every task on the selected surface and classifies what
// the rendered stimulus repeats of each case's own answer.
func auditPrompts(opts options, tasks []evalTask) promptAuditReport {
	report := promptAuditReport{
		Surface:    opts.ToolSurface,
		ServerMode: evalServerModeLabel(opts.ServerMode),
		Edition:    valueOrUnknown(opts.Edition),
		Backend:    normalizedBackend(opts.Backend),
		Cases:      make([]promptAuditCase, 0, len(tasks)),
	}
	for _, task := range tasks {
		report.Cases = append(report.Cases, auditPromptsForTask(task, opts.ToolSurface))
	}
	slices.SortStableFunc(report.Cases, func(left, right promptAuditCase) int {
		return cmp.Compare(left.ID, right.ID)
	})
	declarePromptFindings(&report)
	return report
}

// declarePromptFindings marks every case-site finding a declaration accepts and
// records the declarations that accepted nothing.
//
// It runs inside auditPrompts rather than in the gate so that the gate, the
// report renderer, the published Stimulus header and the tests all read one
// classification. A finding at the system or task site is deliberately left
// undeclarable: that text is this package's to delete, and a declaration there
// would excuse the one thing the gate exists to refuse.
func declarePromptFindings(report *promptAuditReport) {
	used := make(map[string]bool, len(declaredPromptFindings))
	for caseIndex := range report.Cases {
		audited := &report.Cases[caseIndex]
		for findingIndex := range audited.Findings {
			finding := &audited.Findings[findingIndex]
			if !slices.Contains(finding.Sites, promptSiteCase) {
				continue
			}
			for _, declaration := range declaredPromptFindings {
				if promptDeclarationProblem(declaration) != "" || !declaration.covers(report.Surface, audited.ID, *finding) {
					continue
				}
				finding.Category = declaration.Category
				used[declaration.key()] = true
				report.DeclaredFindings++
				break
			}
		}
	}
	for _, declaration := range declaredPromptFindings {
		if problem := promptDeclarationProblem(declaration); problem != "" {
			report.InvalidDeclarations = append(report.InvalidDeclarations, problem)
			continue
		}
		if declaration.Surface != report.Surface || used[declaration.key()] {
			continue
		}
		report.StaleDeclarations = append(report.StaleDeclarations, declaration.key())
	}
	slices.Sort(report.StaleDeclarations)
	slices.Sort(report.InvalidDeclarations)
}

// auditPromptsForTask renders one case exactly as the runner renders it: the
// task is prepared for the surface first, its prompt is put in the shape a live
// run would send, and both are handed to the same two prompt entry points
// evaluatePreparedCase calls.
func auditPromptsForTask(task evalTask, surface string) promptAuditCase {
	rendered := taskForSurface(task, surface)
	rendered.Prompt = promptAuditSentCaseText(rendered)
	audited := promptAuditCase{
		ID:           rendered.ID,
		Surface:      surface,
		Destructive:  taskHasDestructiveStep(rendered),
		SystemPrompt: systemPromptForTask(rendered, surface),
		UserPrompt:   taskPromptForSurface(rendered, surface),
		CaseText:     strings.TrimSpace(rendered.Prompt),
		Answers:      promptAuditAnswers(rendered, surface),
	}
	scaffold, included := promptAuditScaffold(audited.UserPrompt, audited.CaseText)
	audited.CaseTextInPrompt = included
	audited.AnswerKeyed = promptAuditAnswerKeyedSites(rendered, surface, audited)
	for _, answer := range audited.Answers {
		sites := promptAuditSitesFor(answer, audited, scaffold)
		if len(sites) == 0 {
			continue
		}
		audited.Findings = append(audited.Findings, promptAuditFinding{Kind: answer.Kind, Value: answer.Value, Sites: sites})
	}
	slices.SortStableFunc(audited.Findings, comparePromptAuditFindings)
	return audited
}

// promptAuditSentCaseText is the case's own words as a run sends them.
//
// A case may carry two texts. [EvalCase.Prompt] is the plain one, and
// [EvalCase.PromptTemplate] is what a run against a live instance renders from
// its fixture outputs and puts in the task's place (applyLiveFixtureState), so
// for those cases the plain field is a string nothing sends and auditing it
// measures the wrong stimulus. The two are not paraphrases of each other:
// MT-031's template ends "Call repository.file_delete directly with exactly
// that file_path and branch; do not call repository.tree or switch to a
// different file path", and its plain prompt says none of that, so every
// literal in that sentence was invisible here while being sent on every Docker
// run this harness has ever done.
//
// The template's actions are replaced rather than executed, because the audit
// talks to no GitLab and has no fixture outputs to render from. That loses
// nothing: a fixture output is an identifier GitLab minted for this run, so no
// substitution could introduce a name out of the catalog, and leaving the
// actions in would invent findings instead, since `{{ .Values.file_path }}`
// carries the parameter name `file_path` that the value it stands for does not.
func promptAuditSentCaseText(task evalTask) string {
	plain := strings.TrimSpace(task.Prompt)
	if task.Case == nil {
		return plain
	}
	template := strings.TrimSpace(task.Case.PromptTemplate.Text)
	if template == "" {
		return plain
	}
	return strings.TrimSpace(promptAuditTemplateActions.ReplaceAllString(template, promptAuditFixtureValue))
}

// promptAuditTemplateActions matches one text/template action, the unit
// RenderCasePrompt replaces with a fixture output.
var promptAuditTemplateActions = regexp.MustCompile(`{{[^{}]*}}`)

// promptAuditFixtureValue stands in for whatever a fixture would have put
// there. It is deliberately not a word out of any catalog.
const promptAuditFixtureValue = "the-fixture-value"

// promptAuditAnswerKeyedSites renders the case a second time with its answer
// key taken away and reports which prompts came back different.
//
// Destructiveness is deliberately kept: whether a task destroys something is a
// property of what the user asked for rather than of the answer, so a prompt
// that says so is not reading the key. Everything else the key carries, the
// expected tool, the expected action and the parameter names, is removed.
func promptAuditAnswerKeyedSites(task evalTask, surface string, audited promptAuditCase) []promptAuditSite {
	blind := promptAuditBlindTask(task)
	var sites []promptAuditSite
	if systemPromptForTask(blind, surface) != audited.SystemPrompt {
		sites = append(sites, promptSiteSystem)
	}
	if taskPromptForSurface(blind, surface) != audited.UserPrompt {
		sites = append(sites, promptSiteTask)
	}
	return sites
}

// promptAuditBlindTask returns the task with its answer key removed and the
// user's own request left alone.
func promptAuditBlindTask(task evalTask) evalTask {
	blind := task
	blind.ExpectedTool = ""
	blind.ExpectedAction = ""
	blind.RequiredParams = nil
	blind.OptionalParams = nil
	steps := taskSteps(task)
	blinded := make([]evalStep, 0, len(steps))
	for _, step := range steps {
		step.ExpectedTool = ""
		step.ExpectedAction = ""
		step.RequiredParams = nil
		step.OptionalParams = nil
		blinded = append(blinded, step)
	}
	blind.Steps = blinded
	return blind
}

// promptAuditSitesFor reports every place one answer literal was found.
func promptAuditSitesFor(answer promptAuditAnswer, audited promptAuditCase, scaffold string) []promptAuditSite {
	var sites []promptAuditSite
	if promptNamesAnswer(audited.SystemPrompt, answer) {
		sites = append(sites, promptSiteSystem)
	}
	if promptNamesAnswer(scaffold, answer) {
		sites = append(sites, promptSiteTask)
	}
	// A case's text is only a site when the prompt actually carries it: a
	// builder that dropped the user's request sent none of those words.
	if audited.CaseTextInPrompt && promptNamesAnswer(audited.CaseText, answer) {
		sites = append(sites, promptSiteCase)
	}
	return sites
}

// comparePromptAuditFindings orders findings by kind and then by value, so the
// report is stable across runs and diffable across steps.
func comparePromptAuditFindings(left, right promptAuditFinding) int {
	if order := cmp.Compare(promptAuditKindRank(left.Kind), promptAuditKindRank(right.Kind)); order != 0 {
		return order
	}
	return cmp.Compare(left.Value, right.Value)
}

// promptAuditKindRank gives each kind its fixed position.
func promptAuditKindRank(kind promptAuditKind) int {
	if index := slices.Index(promptAuditKindOrder, kind); index >= 0 {
		return index
	}
	return len(promptAuditKindOrder)
}

// promptAuditAnswers collects the literals that make up a case's answer key on
// the selected surface.
//
// The dispatcher tools of the dynamic surface are left out: find-then-execute
// is the surface's own contract, every case's answer names both tools, and
// counting them would report 100% of cases leaking a tool name while saying
// nothing about any case. The action ID those two tools carry is the answer,
// and it is counted.
func promptAuditAnswers(task evalTask, surface string) []promptAuditAnswer {
	seen := map[promptAuditAnswer]bool{}
	answers := make([]promptAuditAnswer, 0, 8)
	add := func(kind promptAuditKind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		answer := promptAuditAnswer{Kind: kind, Value: value}
		if seen[answer] {
			return
		}
		seen[answer] = true
		answers = append(answers, answer)
	}
	for _, step := range taskSteps(task) {
		if !promptAuditSurfaceTool(step.ExpectedTool, surface) {
			add(promptLeakTool, step.ExpectedTool)
		}
		add(promptLeakAction, step.ExpectedAction)
		for _, param := range step.RequiredParams {
			add(promptLeakParam, param)
		}
		// An optional parameter is part of the call the scorer builds
		// (addExpectedOptionalActionParams), so a stimulus that names one
		// has handed over that much of the answer. confirm is left to its
		// own kind, which is searched for in every case, so the
		// destructive-safety column keeps counting it exactly once.
		for _, param := range step.OptionalParams {
			if strings.EqualFold(strings.TrimSpace(param), promptAuditConfirmLiteral) {
				continue
			}
			add(promptLeakParam, param)
		}
		if strings.TrimSpace(step.ExpectedAction) != "" {
			add(promptLeakEnvelope, promptAuditEnvelopeNeedle(step.ExpectedAction))
		}
	}
	add(promptLeakConfirm, promptAuditConfirmLiteral)
	return answers
}

// promptAuditEnvelopeNeedle is the action field as marshalGuidanceExample
// writes it, which is how an exact-call prompt spells a whole call.
func promptAuditEnvelopeNeedle(action string) string {
	return `"action":"` + strings.TrimSpace(action) + `"`
}

// promptAuditSurfaceTool reports whether a tool name is part of the surface's
// own contract rather than part of this case's answer.
func promptAuditSurfaceTool(tool, surface string) bool {
	if !isDynamicEvalSurface(surface) {
		return false
	}
	return tool == dynamicFindTool || tool == dynamicExecuteActionTool
}

// promptAuditScaffold removes the case's own text from the task prompt, so
// what is left is what the builder wrote. It reports whether the case text was
// there to remove: a few compact exact-call prompts send their envelope and
// drop the user's request entirely.
func promptAuditScaffold(userPrompt, caseText string) (scaffold string, included bool) {
	if caseText == "" {
		return userPrompt, false
	}
	before, after, found := strings.Cut(userPrompt, caseText)
	if !found {
		return userPrompt, false
	}
	return before + after, true
}

// promptNamesAnswer reports whether text carries one answer literal.
func promptNamesAnswer(text string, answer promptAuditAnswer) bool {
	if answer.Kind == promptLeakEnvelope {
		return strings.Contains(text, answer.Value)
	}
	return promptNamesIdentifier(text, answer.Value)
}

// promptNamesIdentifier reports whether text names value as a machine
// identifier rather than as an English word.
//
// The distinction decides the number this audit reports, so it is stated
// rather than left to a substring search. A meta action is often spelled
// "get", "list" or "create", and a corpus of tasks about getting and listing
// things would otherwise report every case as leaking its own action. A match
// therefore counts only when the characters around it are code: a dotted
// identifier (project.get), a JSON string ("get"), or a key introducing a
// value (confirm:true, params.merge_request_iid:5). A literal that is itself
// unmistakably an identifier, because it carries an underscore or a dot,
// counts wherever it appears.
func promptNamesIdentifier(text, value string) bool {
	if value == "" {
		return false
	}
	selfEvident := strings.ContainsAny(value, "_.")
	// The search is its own bound. value is not empty, so a rejected match
	// moves the start one byte on and the remaining text eventually cannot
	// hold value, which is the one exit. A length bound beside it would be
	// arithmetic that decides nothing: past it the suffix is shorter than
	// value, so the search returns the same "not found" the loop already
	// reads.
	index := 0
	for {
		offset := strings.Index(text[index:], value)
		if offset < 0 {
			return false
		}
		start := index + offset
		end := start + len(value)
		if promptIdentifierBounded(text, start, end) && (selfEvident || promptIdentifierContext(text, start, end)) {
			return true
		}
		index = start + 1
	}
}

// promptIdentifierBounded reports whether the match is a whole token rather
// than part of a longer one, so "id" does not match inside "valid".
func promptIdentifierBounded(text string, start, end int) bool {
	if start > 0 && promptIdentifierByte(text[start-1]) {
		return false
	}
	if end < len(text) && promptIdentifierByte(text[end]) {
		return false
	}
	return true
}

// promptIdentifierContext reports whether the characters around the match are
// code punctuation rather than prose.
//
// A dot needs its far side checked, and that is the whole of what this used to
// get wrong. A dot is what makes `project.get` an action ID, but it is also how
// an English sentence ends, so "fetch job `999` trace." counted `trace` as a
// named action and MT-202 and MS-017 lost `name` and `branch` the same way.
// Three findings that were never about the corpus. It is code only when an
// identifier continues on the other side of it: the `p` before the dot of
// `project.get`, the `g` after it.
func promptIdentifierContext(text string, start, end int) bool {
	if start > 0 {
		if before := text[start-1]; before == '.' {
			if start >= 2 && promptIdentifierByte(text[start-2]) {
				return true
			}
		} else if strings.IndexByte(`"{:`, before) >= 0 {
			return true
		}
	}
	if end >= len(text) {
		return false
	}
	if text[end] == '.' {
		return end+1 < len(text) && promptIdentifierByte(text[end+1])
	}
	// An equals sign is the other way a case writes a parameter down. A
	// sentence asking for `auto_merge=true` or a list `first=5` at a time is
	// naming the field as surely as a JSON key does, and prose does not spell
	// an equation that way.
	return strings.IndexByte(`"}:=`, text[end]) >= 0
}

// promptIdentifierByte reports whether b can be part of an identifier.
func promptIdentifierByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '_':
		return true
	default:
		return false
	}
}

// leaks reports whether the stimulus repeated any of this case's answer.
func (c promptAuditCase) leaks() bool {
	return len(c.Findings) > 0
}

// leaksBeyondConfirm reports whether the stimulus repeated something other
// than the confirm literal, which every case's system prompt carries.
func (c promptAuditCase) leaksBeyondConfirm() bool {
	for _, finding := range c.Findings {
		if finding.Kind != promptLeakConfirm {
			return true
		}
	}
	return false
}

// coachedByBuilder reports whether the repetition came from this package
// rather than from the case's own text. It is the number V05 to V07 move.
func (c promptAuditCase) coachedByBuilder() bool {
	for _, finding := range c.Findings {
		if slices.Contains(finding.Sites, promptSiteSystem) || slices.Contains(finding.Sites, promptSiteTask) {
			return true
		}
	}
	return false
}

// declared counts the answer literals of one kind this case has.
func (c promptAuditCase) declared(kind promptAuditKind) int {
	count := 0
	for _, answer := range c.Answers {
		if answer.Kind == kind {
			count++
		}
	}
	return count
}

// named counts the answer literals of one kind the stimulus repeated.
func (c promptAuditCase) named(kind promptAuditKind) int {
	count := 0
	for _, finding := range c.Findings {
		if finding.Kind == kind {
			count++
		}
	}
	return count
}

// sites merges the sites of every finding of one kind.
func (c promptAuditCase) sites(kind promptAuditKind) []promptAuditSite {
	var merged []promptAuditSite
	for _, site := range []promptAuditSite{promptSiteSystem, promptSiteTask, promptSiteCase} {
		for _, finding := range c.Findings {
			if finding.Kind == kind && slices.Contains(finding.Sites, site) {
				merged = append(merged, site)
				break
			}
		}
	}
	return merged
}

// totals aggregates the corpus into the figures the next steps are measured
// against.
func (r promptAuditReport) totals() promptAuditTotals {
	totals := promptAuditTotals{Cases: len(r.Cases), ByKind: map[promptAuditKind]promptAuditKindTotals{}}
	for _, audited := range r.Cases {
		totals.accumulate(audited)
	}
	return totals
}

// accumulate folds one case into the totals.
func (t *promptAuditTotals) accumulate(audited promptAuditCase) {
	if audited.leaks() {
		t.Leaking++
	}
	if audited.leaksBeyondConfirm() {
		t.BeyondConfirm++
	}
	if audited.coachedByBuilder() {
		t.BuilderCoached++
	} else if audited.leaks() {
		t.CaseTextOnly++
	}
	if !audited.CaseTextInPrompt {
		t.CaseTextDropped++
	}
	if len(audited.AnswerKeyed) > 0 {
		t.AnswerKeyed++
	}
	if slices.Contains(audited.AnswerKeyed, promptSiteSystem) {
		t.AnswerKeyedSystem++
	}
	if slices.Contains(audited.AnswerKeyed, promptSiteTask) {
		t.AnswerKeyedTask++
	}
	if audited.Destructive {
		t.Destructive++
		if audited.named(promptLeakConfirm) > 0 {
			t.DestructiveConfirmNamed++
		}
	}
	for _, kind := range promptAuditKindOrder {
		t.accumulateKind(kind, audited)
	}
}

// accumulateKind folds one kind of one case into the totals.
func (t *promptAuditTotals) accumulateKind(kind promptAuditKind, audited promptAuditCase) {
	kindTotals := t.ByKind[kind]
	kindTotals.Declared += audited.declared(kind)
	named := audited.named(kind)
	kindTotals.Named += named
	if named > 0 {
		kindTotals.Cases++
	}
	sites := audited.sites(kind)
	if slices.Contains(sites, promptSiteSystem) {
		kindTotals.System++
	}
	if slices.Contains(sites, promptSiteTask) {
		kindTotals.Task++
	}
	if slices.Contains(sites, promptSiteCase) {
		kindTotals.Case++
	}
	t.ByKind[kind] = kindTotals
}

// renderPromptAuditSummary renders the header, the per-case table and the
// totals: what the verification of this step reads, and what CI would print.
func renderPromptAuditSummary(report promptAuditReport) string {
	var b strings.Builder
	writePromptAuditHeader(&b, report)
	writePromptAuditCaseTable(&b, report)
	writePromptAuditTotals(&b, report)
	return b.String()
}

// renderPromptAuditDump renders the summary followed by every finding and
// every prompt, which is the artifact a deletion is reviewed as a diff over.
func renderPromptAuditDump(report promptAuditReport) string {
	var b strings.Builder
	b.WriteString(renderPromptAuditSummary(report))
	writePromptAuditFindings(&b, report)
	writePromptAuditPrompts(&b, report)
	return b.String()
}

// writePromptAuditHeader states what was audited and what the numbers mean. It
// carries no date and no path, so two runs of the same tree agree byte for
// byte.
func writePromptAuditHeader(b *strings.Builder, report promptAuditReport) {
	fmt.Fprintf(b, "# Prompt audit\n\n")
	fmt.Fprintf(b, "Tool surface: `%s`\n", report.Surface)
	fmt.Fprintf(b, "Server mode: `%s`\n", report.ServerMode)
	fmt.Fprintf(b, "Edition: `%s`\n", report.Edition)
	fmt.Fprintf(b, "Backend: `%s`\n", report.Backend)
	fmt.Fprintf(b, "Cases audited: %d\n", len(report.Cases))
	b.WriteString(`
This is a report and not a gate. For every selected case it renders the system
prompt and the task prompt a run would send, through the same two entry points
the runner calls, and reports which of them already carry that case's own
answer: its expected tool, its expected action, a name it declares as a
required parameter, the literal ` + "`confirm`" + `, and a marshaled
` + "`{\"action\":...,\"params\":{...}}`" + ` envelope naming the expected action.

Three sites are kept apart because they are not equally damning. ` + "`system`" + ` is
the system prompt, which says the same thing for every case on the surface.
` + "`task`" + ` is what the prompt builder wrote around this case: the sharp number,
since it is per-case coaching and it is what the deletions that follow remove.
` + "`case`" + ` is the case's own user-authored text, which no change to this
package can take away.

A literal counts as named only where it is written as a machine identifier and
not as an English word: inside a dotted identifier, inside a JSON string, or as
a key introducing a value. A literal carrying an underscore or a dot counts
wherever it appears. Without that rule a corpus of tasks about getting and
listing things would report every meta case as naming its own action.

On the dynamic surface the two dispatcher tools are the surface's own contract
rather than any case's answer, and are not counted as a tool; the action ID
they carry is.

The last column asks a different question, because some coaching names
nothing. Each case is rendered a second time with its expected tool, expected
action and parameter names taken away and everything the user asked for kept;
the column names the prompts that came back different. A dynamic task prompt
stating how many catalog operations the task needs is answer-keyed in exactly
this way: it carries the shape of the answer and none of its words. Whether a
task is destructive is kept in both renderings, since that is a property of
what was asked rather than of the answer.

`)
}

// writePromptAuditCaseTable writes one row per case and surface. Each cell is
// named/declared followed by the sites, or a dash where nothing was repeated.
func writePromptAuditCaseTable(b *strings.Builder, report promptAuditReport) {
	b.WriteString("## Cases\n\n")
	b.WriteString("| Case | Surface | Destructive | Tool | Action | Params | Confirm | Envelope | Answer-keyed |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, audited := range report.Cases {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			audited.ID,
			audited.Surface,
			yesNo(audited.Destructive),
			promptAuditCell(audited, promptLeakTool),
			promptAuditCell(audited, promptLeakAction),
			promptAuditCell(audited, promptLeakParam),
			promptAuditCell(audited, promptLeakConfirm),
			promptAuditCell(audited, promptLeakEnvelope),
			promptAuditSitesCell(audited.AnswerKeyed),
		)
	}
	b.WriteString("\n")
}

// promptAuditCell renders one kind for one case.
func promptAuditCell(audited promptAuditCase, kind promptAuditKind) string {
	named := audited.named(kind)
	if named == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d %s", named, audited.declared(kind), joinPromptAuditSites(audited.sites(kind)))
}

// promptAuditSitesCell renders a site list, or a dash when it is empty.
func promptAuditSitesCell(sites []promptAuditSite) string {
	if len(sites) == 0 {
		return "-"
	}
	return joinPromptAuditSites(sites)
}

// joinPromptAuditSites renders a site list in its fixed order.
func joinPromptAuditSites(sites []promptAuditSite) string {
	parts := make([]string, 0, len(sites))
	for _, site := range sites {
		parts = append(parts, string(site))
	}
	return strings.Join(parts, "+")
}

// writePromptAuditTotals writes the figures V05 to V07 are measured against.
func writePromptAuditTotals(b *strings.Builder, report promptAuditReport) {
	totals := report.totals()
	b.WriteString("## Totals\n\n")
	fmt.Fprintf(b, "Cases audited: %d\n", totals.Cases)
	fmt.Fprintf(b, "Cases whose stimulus repeats their own answer: %d (%s)\n", totals.Leaking, formatMetric(percent(totals.Leaking, totals.Cases)))
	fmt.Fprintf(b, "Cases whose stimulus repeats it beyond the confirm clause: %d (%s)\n", totals.BeyondConfirm, formatMetric(percent(totals.BeyondConfirm, totals.Cases)))
	fmt.Fprintf(b, "Cases coached by this package (system prompt or task scaffolding): %d (%s)\n", totals.BuilderCoached, formatMetric(percent(totals.BuilderCoached, totals.Cases)))
	fmt.Fprintf(b, "Cases whose only repetition is in their own text: %d\n", totals.CaseTextOnly)
	fmt.Fprintf(b, "Case-text findings a declaration accounts for: %d\n", report.DeclaredFindings)
	if len(report.StaleDeclarations) > 0 {
		fmt.Fprintf(b, "Declarations matching nothing on this surface: %s\n", strings.Join(report.StaleDeclarations, ", "))
	}
	if len(report.InvalidDeclarations) > 0 {
		fmt.Fprintf(b, "Declarations the gate cannot act on: %s\n", strings.Join(report.InvalidDeclarations, ", "))
	}
	fmt.Fprintf(b, "Cases whose task prompt drops the user's request entirely: %d\n", totals.CaseTextDropped)
	fmt.Fprintf(b, "Destructive cases: %d, of which the stimulus names confirm: %d\n", totals.Destructive, totals.DestructiveConfirmNamed)
	fmt.Fprintf(b, "Cases whose stimulus changes when the answer key is taken away: %d (system %d, task %d)\n\n",
		totals.AnswerKeyed, totals.AnswerKeyedSystem, totals.AnswerKeyedTask)
	b.WriteString("| Kind | Literals declared | Literals named | Cases | System | Task | Case |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, kind := range promptAuditKindOrder {
		kindTotals := totals.ByKind[kind]
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d | %d |\n",
			kind, kindTotals.Declared, kindTotals.Named, kindTotals.Cases, kindTotals.System, kindTotals.Task, kindTotals.Case)
	}
	b.WriteString("\n")
}

// writePromptAuditFindings lists every finding, which is what makes two
// readers agree on the number in the totals.
func writePromptAuditFindings(b *strings.Builder, report promptAuditReport) {
	b.WriteString("## Findings\n\n")
	b.WriteString("| Case | Kind | Literal | Sites |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, audited := range report.Cases {
		for _, finding := range audited.Findings {
			fmt.Fprintf(b, "| %s | %s | `%s` | %s |\n",
				audited.ID, finding.Kind, finding.Value, joinPromptAuditSites(finding.Sites))
		}
	}
	b.WriteString("\n")
}

// writePromptAuditPrompts writes the stimulus itself, verbatim.
func writePromptAuditPrompts(b *strings.Builder, report promptAuditReport) {
	b.WriteString("## Prompts\n\n")
	for _, audited := range report.Cases {
		fmt.Fprintf(b, "### %s\n\n", audited.ID)
		b.WriteString("System prompt:\n\n")
		b.WriteString(toolutil.MarkdownFencedBlock("text", audited.SystemPrompt))
		b.WriteString("\nTask prompt:\n\n")
		b.WriteString(toolutil.MarkdownFencedBlock("text", audited.UserPrompt))
		b.WriteString("\n")
	}
}

// yesNo renders a flag for a table cell.
func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// promptAuditIsClean is the gate half of the audit: it fails when the stimulus
// a run would send carries the case's own answer.
//
// It judges all three sites, and the three are not held to the same terms. The
// system and task sites are this package's own text, so a finding there is a
// defect with nowhere to hide and nothing may excuse it. The case site is the
// corpus's own words, where a request can be unwritable without the literal it
// carries, so a finding there is refused unless prompt_declarations.go records
// why. A declaration that stops matching anything fails too, which is what
// stops the table outliving the case text it describes.
func promptAuditIsClean(w io.Writer, report promptAuditReport) error {
	var coached []string
	for _, audited := range report.Cases {
		// Two questions, and a gate that asked only the first would miss the
		// shape of the leak it was built for. A literal finding is a prompt
		// naming the answer; AnswerKeyed is a prompt that came back *different*
		// when the answer key was taken away, which catches coaching carrying
		// the answer's shape and none of its words. The dynamic operation
		// count, "For each of the 3 GitLab catalog operations", was exactly
		// that, and its return produces no literal finding at all.
		//
		// What AnswerKeyed can and cannot see is worth being exact about,
		// because it is narrower than it sounds: the blind render blanks each
		// step's fields and keeps the steps themselves, so a count of raw steps
		// survives it unchanged and is not caught here. The original count was
		// caught because it counted steps carrying an expected action, which
		// the blinding removes. Verified both ways rather than assumed.
		if site, keyed := promptAuditFirstBuilderSite(audited.AnswerKeyed); keyed {
			coached = append(coached, fmt.Sprintf("%s: its %s prompt changes when the answer key is removed", audited.ID, site))
			continue
		}
		for _, finding := range audited.Findings {
			if slices.Contains(finding.Sites, promptSiteSystem) || slices.Contains(finding.Sites, promptSiteTask) {
				coached = append(coached, fmt.Sprintf("%s: %s %q at %v", audited.ID, finding.Kind, finding.Value, finding.Sites))
				break
			}
		}
	}
	// The case site is gated too, against prompt_declarations.go. It was
	// reported and not gated while the corpus still carried its own answers:
	// gating it then would have failed on the first run, which is no way to
	// introduce a gate. Issue 778 emptied it, so the refusal now has a corpus
	// it can hold.
	for _, audited := range report.Cases {
		for _, finding := range audited.Findings {
			if !slices.Contains(finding.Sites, promptSiteCase) || finding.Category != "" {
				continue
			}
			coached = append(coached, fmt.Sprintf("%s: its own text names %s %q, and no declaration says why", audited.ID, finding.Kind, finding.Value))
		}
	}
	coached = append(coached, report.InvalidDeclarations...)
	for _, stale := range report.StaleDeclarations {
		coached = append(coached, stale+": declared, and nothing on this surface carries it any more")
	}
	if len(coached) == 0 {
		if _, err := fmt.Fprintf(w, "\nPrompt audit check: no case's stimulus names its own answer, beyond %d declared literal(s).\n", report.DeclaredFindings); err != nil {
			return fmt.Errorf("write prompt audit: %w", err)
		}
		return nil
	}
	return fmt.Errorf("the stimulus carries the answer for %d case(s):\n  %s",
		len(coached), strings.Join(coached, "\n  "))
}

// promptAuditFirstBuilderSite names the first builder site of an answer-keyed
// list, and reports whether there was one. The case site cannot appear in that
// list: it is the user's own words, which the blind render keeps.
func promptAuditFirstBuilderSite(sites []promptAuditSite) (promptAuditSite, bool) {
	for _, site := range sites {
		if site == promptSiteSystem || site == promptSiteTask {
			return site, true
		}
	}
	return "", false
}
