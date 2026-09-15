package modelcorpus

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The prompt audit. It renders every stimulus a run would send, on every
// surface the case runs on, and refuses one that spells its own answer.
//
// What counts as the answer is read from the catalog rather than from the
// author's own argument list, which is the hole the audit this replaces had:
// an author who left an argument out of the key also took it out of the
// audit's vocabulary, so the one case most likely to be coached was the one
// least likely to be caught. The vocabulary of a case is therefore the
// canonical action ID of each step, the name that action carries on each
// surface, the action names of its whole catalog group, every argument its
// input schema admits, and the confirmation literal.
//
// Three rules keep it from reporting prose as code:
//
//   - A single-word identifier ("list", "get", "title") counts only where the
//     characters around it are code: a dotted identifier, a JSON string, or a
//     key introducing a value. A corpus of tasks about getting and listing
//     things would otherwise report every case as leaking its own action.
//   - An identifier carrying an underscore or a dot counts wherever it
//     appears, because nothing else spells it.
//   - A multi-word identifier counts as a phrase too, case-folded with the
//     underscore read as a space, which is what turns "state event" from prose
//     into the finding it is.
//
// A finding reports the sentence it sits in, so triage does not mean opening
// the file, and a case-site finding a person has judged is answered by an
// entry in declarations.go rather than by weakening a rule.

// auditSite says where in the stimulus an answer literal was found. The two
// are not equally damning: the contract says the same thing for every case of
// a surface, and the case text is what a person wrote for this one.
type auditSite string

const (
	auditSiteContract auditSite = "contract"
	auditSiteCase     auditSite = "case"
)

// auditKind names which part of a case's answer the stimulus repeated.
type auditKind string

const (
	auditKindAction     auditKind = "action"
	auditKindTool       auditKind = "tool"
	auditKindArgument   auditKind = "argument"
	auditKindSibling    auditKind = "sibling action"
	auditKindConfirm    auditKind = "confirm"
	auditConfirmLiteral           = "confirm"
)

// auditTerm is one literal of a case's answer key, with what it is.
type auditTerm struct {
	Kind  auditKind
	Value string
}

// auditFinding is one answer literal found in one rendering of one stimulus.
type auditFinding struct {
	Case     string
	Surface  Surface
	Site     auditSite
	Kind     auditKind
	Value    string
	Sentence string
}

// String spells a finding for a failure message.
func (f auditFinding) String() string {
	return fmt.Sprintf("%s on %s: the %s site names the %s %q in: %s",
		f.Case, f.Surface, f.Site, f.Kind, f.Value, f.Sentence)
}

// envelopeWords are the fields of a dispatcher's own input object. A surface
// contract may name them, because a model that does not know the envelope
// cannot reach the catalog at all, and the corpus contains actions that take
// an argument of the same name (user.todo_list takes "action"). They are
// exempt at the contract site only: a case text naming them is naming a call.
var envelopeWords = map[string]bool{"action": true, "params": true}

// auditVocabulary is what a stimulus for this case must not spell.
func auditVocabulary(facts catalogFacts, one Case) []auditTerm {
	terms := []auditTerm{{Kind: auditKindConfirm, Value: auditConfirmLiteral}}
	seen := map[string]bool{auditConfirmLiteral: true}
	add := func(kind auditKind, value string) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		terms = append(terms, auditTerm{Kind: kind, Value: value})
	}
	for _, declared := range one.key.Steps {
		if declared.Standalone != "" {
			add(auditKindTool, declared.Standalone)
			for name := range facts.standalone[declared.Standalone] {
				add(auditKindArgument, name)
			}
			continue
		}
		action, known := facts.actions[declared.Action]
		if !known {
			continue
		}
		add(auditKindAction, string(declared.Action))
		add(auditKindAction, action.metaAction)
		add(auditKindTool, action.metaTool)
		add(auditKindTool, action.individualTool)
		for name := range action.arguments {
			add(auditKindArgument, name)
		}
		for _, sibling := range facts.groupActions[action.metaTool] {
			add(auditKindSibling, sibling)
		}
	}
	slices.SortFunc(terms, func(a, b auditTerm) int { return strings.Compare(a.Value, b.Value) })
	return terms
}

// auditStimulus reports every answer literal one rendered stimulus names.
func auditStimulus(one Case, surface Surface, contract, prompt string, terms []auditTerm) []auditFinding {
	var findings []auditFinding
	for _, site := range []struct {
		name auditSite
		text string
	}{{auditSiteContract, contract}, {auditSiteCase, prompt}} {
		for _, term := range terms {
			if site.name == auditSiteContract && envelopeWords[term.Value] {
				continue
			}
			index, found := namesTerm(site.text, term.Value, term.Kind)
			if !found {
				continue
			}
			findings = append(findings, auditFinding{
				Case:     one.ID,
				Surface:  surface,
				Site:     site.name,
				Kind:     term.Kind,
				Value:    term.Value,
				Sentence: sentenceAround(site.text, index),
			})
		}
	}
	return findings
}

// namesTerm reports whether text names value as a machine identifier or as the
// phrase that identifier reads as, and where.
//
// A tool name is matched as an identifier only. Every tool this server
// registers is a gitlab_ name, so the phrase form of one is the product name
// followed by a domain word, and "the current authenticated GitLab user" would
// be a finding against gitlab_user in a sentence that names no tool at all. An
// action name and an argument name have no such prefix, and their phrase form
// is exactly how a person spells them in prose, which is the whole point of
// the rule.
func namesTerm(text, value string, kind auditKind) (int, bool) {
	if index, found := namesIdentifier(text, value); found {
		return index, true
	}
	if kind == auditKindTool {
		return 0, false
	}
	return namesPhrase(text, value)
}

// namesIdentifier reports whether text names value as a machine identifier
// rather than as an English word.
func namesIdentifier(text, value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	selfEvident := strings.ContainsAny(value, "_.")
	for index := 0; ; {
		offset := strings.Index(text[index:], value)
		if offset < 0 {
			return 0, false
		}
		start := index + offset
		end := start + len(value)
		if tokenBounded(text, start, end) && (selfEvident || codeContext(text, start, end)) {
			return start, true
		}
		index = start + 1
	}
}

// namesPhrase reports whether text names a multi-word identifier as the words
// it is made of: state_event as "state event", case-folded.
func namesPhrase(text, value string) (int, bool) {
	if !strings.Contains(value, "_") {
		return 0, false
	}
	phrase := strings.ToLower(strings.ReplaceAll(value, "_", " "))
	lowered := strings.ToLower(text)
	for index := 0; ; {
		offset := strings.Index(lowered[index:], phrase)
		if offset < 0 {
			return 0, false
		}
		start := index + offset
		if tokenBounded(lowered, start, start+len(phrase)) {
			return start, true
		}
		index = start + 1
	}
}

// tokenBounded reports whether the match is a whole token rather than part of
// a longer one, so "id" does not match inside "valid".
func tokenBounded(text string, start, end int) bool {
	if start > 0 && identifierByte(text[start-1]) {
		return false
	}
	return end >= len(text) || !identifierByte(text[end])
}

// codeContext reports whether the characters around the match are code
// punctuation rather than prose.
//
// A full stop counts only when the other side of it is an identifier, which is
// what tells project.get from a sentence ending in "page.". Reading every
// trailing stop as code made every single-word argument at the end of a
// sentence a finding: "with 5 results per page." named the page parameter.
func codeContext(text string, start, end int) bool {
	if start > 0 {
		before := text[start-1]
		if before == '"' || before == '{' || before == ':' {
			return true
		}
		if before == '.' && start >= 2 && identifierByte(text[start-2]) {
			return true
		}
	}
	if end >= len(text) {
		return false
	}
	after := text[end]
	if after == '"' || after == '}' || after == ':' {
		return true
	}
	return after == '.' && end+1 < len(text) && identifierByte(text[end+1])
}

// identifierByte reports whether b can be part of an identifier.
func identifierByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '_':
		return true
	default:
		return false
	}
}

// sentenceAround returns the sentence the match at index sits in, so a finding
// can be judged without opening the corpus.
func sentenceAround(text string, index int) string {
	start := 0
	for before := index - 1; before >= 0; before-- {
		if text[before] == '\n' || (text[before] == '.' && followedByBreak(text, before)) {
			start = before + 1
			break
		}
	}
	end := len(text)
	for after := index; after < len(text); after++ {
		if text[after] == '\n' {
			end = after
			break
		}
		if text[after] == '.' && followedByBreak(text, after) {
			end = after + 1
			break
		}
	}
	return strings.TrimSpace(text[start:end])
}

// followedByBreak reports whether the full stop at index ends a sentence
// rather than sitting inside an identifier or a version number.
func followedByBreak(text string, index int) bool {
	return index+1 >= len(text) || text[index+1] == ' ' || text[index+1] == '\n'
}

// auditCorpus renders every case on every surface it runs on and returns what
// the stimulus named.
func auditCorpus(t *testing.T) []auditFinding {
	t.Helper()
	facts := catalog(t)
	values := auditFacts()
	var findings []auditFinding
	for _, one := range cases() {
		terms := auditVocabulary(facts, one)
		prompt, err := Render(one.Prompt, values)
		if err != nil {
			t.Fatalf("%s: rendering the prompt: %v", one.ID, err)
		}
		for _, surface := range Surfaces() {
			if !one.Surfaces.Runs(surface) {
				continue
			}
			contract, known := Contract(surface)
			if !known {
				t.Fatalf("surface %q has no contract", surface)
			}
			findings = append(findings, auditStimulus(one, surface, contract, prompt, terms)...)
		}
	}
	return findings
}

// TestContract_NoStimulusNamesItsOwnAnswer is the audit as a gate. A finding
// here is a case that hands its answer to the model, and the fix is the case
// or, where the literal is the request itself, an entry in declarations.go.
func TestContract_NoStimulusNamesItsOwnAnswer(t *testing.T) {
	for _, finding := range auditCorpus(t) {
		if declaredFinding(finding) {
			continue
		}
		t.Errorf("%s", finding)
	}
}

// declaredFinding reports whether a case-site finding is one a person has
// judged and written down. A contract-site finding is never declared: the
// contract is three strings, and the fix for one is to change them.
func declaredFinding(finding auditFinding) bool {
	if finding.Site != auditSiteCase {
		return false
	}
	for _, declaration := range declaredPromptFindings {
		if declaration.covers(finding.Case, finding.Value) {
			return true
		}
	}
	return false
}

// TestContract_FindsAPlantedAnswer proves the audit can fail. A corpus that
// passes its own audit says nothing unless the audit is known to report the
// thing it exists to report, and the three matching rules are each worth one
// case: the underscored identifier, the phrase it reads as, and the single
// word that counts only in code context.
func TestContract_FindsAPlantedAnswer(t *testing.T) {
	facts := catalog(t)
	planted := Case{
		ID:     "PLANTED-001",
		Recipe: RecipeIssue,
		key: Key{Steps: []Step{
			step("issue.update", project(), req("issue_iid", fact(FactIssueIID))),
		}},
	}
	terms := auditVocabulary(facts, planted)
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{
			name:   "the argument name itself",
			prompt: "Close issue 7 by sending state_event.",
			want:   "state_event",
		},
		{
			name:   "the argument name read as English",
			prompt: "Close issue 7 with the state event that closes it.",
			want:   "state_event",
		},
		{
			name:   "the action in code context",
			prompt: `Call the "update" action on issue 7.`,
			want:   "update",
		},
		{
			name:   "the canonical action ID",
			prompt: "Run issue.update on issue 7.",
			want:   "issue.update",
		},
		{
			name:   "the meta tool name",
			prompt: "Use gitlab_issue to close issue 7.",
			want:   "gitlab_issue",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings := auditStimulus(planted, SurfaceDynamic, "", tc.prompt, terms)
			if !namedInFindings(findings, tc.want) {
				t.Fatalf("the audit reported %v for %q, want a finding naming %q",
					findings, tc.prompt, tc.want)
			}
			for _, finding := range findings {
				if finding.Value == tc.want && !strings.Contains(finding.Sentence, "issue 7") {
					t.Errorf("the finding's sentence is %q, want the sentence the match sits in",
						finding.Sentence)
				}
			}
		})
	}
}

// TestContract_LeavesProseAlone is the other half of the same proof: the rules
// are worth nothing if they report every case about getting and listing things
// as leaking its own action.
func TestContract_LeavesProseAlone(t *testing.T) {
	facts := catalog(t)
	planted := Case{
		ID:     "PLANTED-002",
		Recipe: RecipeIssue,
		key: Key{Steps: []Step{
			step("issue.update", project(), req("issue_iid", fact(FactIssueIID))),
		}},
	}
	terms := auditVocabulary(facts, planted)
	for _, prompt := range []string{
		"Close issue 7 in project my-org/tools and give it the evaluation label.",
		"Update the title of issue 7 so that it describes the bug.",
		"The issue is invalid; mark it closed with a description of why.",
	} {
		t.Run(prompt, func(t *testing.T) {
			if findings := auditStimulus(planted, SurfaceDynamic, "", prompt, terms); len(findings) > 0 {
				t.Errorf("the audit reported %v, want nothing: this is prose", findings)
			}
		})
	}
}

// TestContract_EachSurfaceIsIntroducedOnce checks the three contracts are
// present, distinct and say nothing about where the confirmation goes, which
// the server publishes itself and which the old evaluator's prompt said for it
// on every row of a column that then read 100%.
func TestContract_EachSurfaceIsIntroducedOnce(t *testing.T) {
	seen := map[string]Surface{}
	for _, surface := range Surfaces() {
		contract, known := Contract(surface)
		if !known || strings.TrimSpace(contract) == "" {
			t.Fatalf("surface %q has no contract", surface)
		}
		if first, repeated := seen[contract]; repeated {
			t.Errorf("surfaces %q and %q are introduced with the same text", first, surface)
		}
		seen[contract] = surface
		if _, named := namesTerm(contract, auditConfirmLiteral, auditKindConfirm); named {
			t.Errorf("the %s contract names the confirmation the server publishes itself", surface)
		}
	}
	if _, known := Contract("nonsense"); known {
		t.Error("Contract(nonsense) reported a contract for a surface that does not exist")
	}
}

// namedInFindings reports whether any finding names value.
func namedInFindings(findings []auditFinding, value string) bool {
	for _, finding := range findings {
		if finding.Value == value {
			return true
		}
	}
	return false
}
