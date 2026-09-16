package modelcorpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"text/template"
)

// Stimulus is everything about a case a run may see: what the model is asked,
// which world it is asked in, what the instance must offer and which surfaces
// the case may run on.
//
// It is a type of its own rather than a [Case] with the key blanked out,
// because a blanked field is a field a later edit can fill in by accident.
type Stimulus struct {
	ID       string
	Prompt   string
	Recipe   Recipe
	Needs    Needs
	Surfaces Restrict
}

// cases returns every case of the corpus, key included. It is unexported, and
// the two accessors below are what the rest of the repository sees.
func cases() []Case {
	all := make([]Case, 0, 256)
	all = append(all, readCases()...)
	all = append(all, mutatingCases()...)
	all = append(all, destructiveCases()...)
	all = append(all, licensedReadCases()...)
	all = append(all, licensedMutatingCases()...)
	all = append(all, licensedDestructiveCases()...)
	return all
}

// Stimuli returns what a run may see, in corpus order.
//
// This is the accessor the runner calls, and the only one it may call. A
// stimulus carries no expectation of any kind, so nothing a run sends a model
// can be derived from the answer it will be scored against.
func Stimuli() []Stimulus {
	all := cases()
	stimuli := make([]Stimulus, 0, len(all))
	for _, one := range all {
		stimuli = append(stimuli, one.stimulus())
	}
	return stimuli
}

// stimulus is the half of a case a run may see.
func (c Case) stimulus() Stimulus {
	return Stimulus{ID: c.ID, Prompt: c.Prompt, Recipe: c.Recipe, Needs: c.Needs, Surfaces: c.Surfaces}
}

// Keys returns the answer for every case, by case ID.
//
// Four packages outside this one may use it, and access_test.go fails on a
// fifth: the scorer, which is the one thing a key is for; the two generators,
// each of which publishes about a key and produces no stimulus; and the fake
// provider, which replays a key to prove the pipe and whose every row the
// publisher refuses. The list that test reads names this package too, since
// the corpus's own gate is written against the key, which is why it holds five
// entries and this sentence names four. It is one of three such lists, one per
// accessor answering about a key: [StepCount] and [Digest] have their own, and
// are allowed where this is not.
func Keys() map[string]Key {
	all := cases()
	keys := make(map[string]Key, len(all))
	for _, one := range all {
		keys[one.ID] = one.key
	}
	return keys
}

// ByID returns one case's stimulus.
func ByID(id string) (Stimulus, bool) {
	for _, one := range cases() {
		if one.ID == id {
			return one.stimulus(), true
		}
	}
	return Stimulus{}, false
}

// IDs returns every case identifier, sorted, which is what a report iterates
// and what a run filters against.
func IDs() []string {
	all := cases()
	ids := make([]string, 0, len(all))
	for _, one := range all {
		ids = append(ids, one.ID)
	}
	sort.Strings(ids)
	return ids
}

// Render renders one case's prompt over the facts its recipe produced.
//
// It is here rather than in the runner because the corpus gate and the run
// must render the same text: a template the gate accepted and the runner then
// rendered differently would be audited in one spelling and sent in another,
// which is the drift the two-text case shape had and this shape exists to end.
//
// A template naming a fact the recipe did not produce is an error rather than
// an empty string, because a stimulus with a hole in it is a paid request that
// measures nothing.
func Render(prompt string, facts map[string]string) (string, error) {
	parsed, err := template.New("stimulus").Option("missingkey=error").Parse(prompt)
	if err != nil {
		return "", fmt.Errorf("parse the prompt template: %w", err)
	}
	var rendered strings.Builder
	if execErr := parsed.Execute(&rendered, struct{ Facts map[string]string }{Facts: facts}); execErr != nil {
		return "", fmt.Errorf("render the prompt template: %w", execErr)
	}
	return rendered.String(), nil
}

// FactPlaceholder is how a prompt interpolates one fact. It is spelled once
// here so that the corpus gate looks for what [Render] resolves.
func FactPlaceholder(key string) string {
	return "{{ .Facts." + key + " }}"
}

// InterpolatedFacts returns the fact keys a prompt interpolates, in the order
// they first appear. It reads the parsed template rather than the text, so a
// spelling [Render] would resolve and a spelling a search would find are the
// same set.
func InterpolatedFacts(prompt string) ([]string, error) {
	parsed, err := template.New("stimulus").Parse(prompt)
	if err != nil {
		return nil, fmt.Errorf("parse the prompt template: %w", err)
	}
	seen := map[string]bool{}
	var keys []string
	for _, node := range parsed.Root.Nodes {
		key, ok := interpolatedFact(node.String())
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys, nil
}

// interpolatedFact reads the fact key out of one rendered action node, which
// text/template prints back as it was written: {{ .Facts.project_path }}.
func interpolatedFact(node string) (string, bool) {
	trimmed := strings.TrimSpace(node)
	body, isAction := strings.CutPrefix(trimmed, "{{")
	if !isAction {
		return "", false
	}
	body, isAction = strings.CutSuffix(body, "}}")
	if !isAction {
		return "", false
	}
	key, isFact := strings.CutPrefix(strings.TrimSpace(body), ".Facts.")
	if !isFact || key == "" || strings.ContainsAny(key, " .") {
		return "", false
	}
	return key, true
}

// Digest returns a fingerprint of the whole corpus, key included.
//
// A run writes it on its record, and the report is scored from the corpus at
// HEAD rather than from anything the run carried: a key that moved between the
// two is a record being scored against an answer it was never put to, and the
// digest is what lets the publisher refuse that instead of publishing it.
//
// It therefore has to cover the key, which is why it is here and not in the
// runner. A run may not read a key, and a digest of the stimuli alone would be
// unmoved by exactly the edit it exists to catch.
//
// Sixteen hex characters, the repository's own length for a comparison
// fingerprint a person reads off two rows of a table.
func Digest() string {
	sum := sha256.New()
	for _, one := range cases() {
		writeCaseDigest(sum, one)
	}
	return hex.EncodeToString(sum.Sum(nil))[:digestLength]
}

// digestLength is how much of the hash [Digest] carries.
const digestLength = 16

// writeCaseDigest folds one case into the hash, field by field.
//
// By hand rather than through encoding/json, because the key is unexported and
// a marshaler would skip it silently: the digest would then be stable across
// the one change it is meant to notice, and nothing would say so.
func writeCaseDigest(sum io.Writer, one Case) {
	_, _ = fmt.Fprintf(sum, "case\x00%s\x00%s\x00%s\x00%s\x00%t\x00%t\x00%t\x00%s\x00%v\n",
		one.ID, one.Prompt, one.Recipe, one.Needs.MinimumTier(),
		one.Needs.Runner, one.Needs.Admin, one.Needs.FixtureService,
		one.Surfaces.Reason, one.Surfaces.Only)
	for _, step := range one.key.Steps {
		_, _ = fmt.Fprintf(sum, "step\x00%s\x00%s\x00%t\x00%v\n",
			step.Action, step.Standalone, step.Optional, step.Produces)
		for _, arg := range step.Args {
			_, _ = fmt.Fprintf(sum, "arg\x00%s\x00%t\x00%s\x00%s\x00%d\x00%s\x00%t\n",
				arg.Name, arg.Required, arg.Truth.Fact, arg.Truth.Literal,
				arg.Truth.Produced.Step, arg.Truth.Produced.Field, arg.Truth.Authored)
		}
	}
}

// StepCount returns how many steps one case's key declares, and whether the
// corpus has that case.
//
// It is one of the two things a run is told about a key, [Domains] being the
// other, and it is told for one reason: the turn cap an attempt is bounded by
// is a multiple of the steps plus a margin, so a case of one step cannot spend
// a conversation's worth of tokens going nowhere. A cap is an ending and never
// a message, so nothing a model is shown can be derived from it; what a run
// learns is how long it may go on, never what it should say.
//
// It is deliberately not a field of [Stimulus]. That type is the enumeration of
// what a run may see, and growing it is how the boundary erodes; a function
// asked for by name is a use a reader can find, and access_test.go holds this
// name to a list of callers of its own, as it holds [Keys] to one. The runner
// is on that list; the file that composes a prompt is refused by a second lock
// of its own.
func StepCount(id string) (int, bool) {
	for _, one := range cases() {
		if one.ID == id {
			return len(one.key.Steps), true
		}
	}
	return 0, false
}

// Domains returns the catalog domains one case's key touches, sorted, and
// whether the corpus has that case.
//
// It is the widest of the three narrow doors, so what it is for has to be
// stated rather than assumed. The individual surface publishes one tool per
// action and its whole list is 682,878 tokens at Ultimate, over the context
// window of at least one provider outright, so a model there is shown a slice:
// every tool of these domains, plus distractors from others, filled to a budget
// seeded by the case ID. A slice chosen without them would be a list the case's
// own tools are missing from, and an attempt against that measures nothing but
// the shuffle.
//
// What it hands over and what it does not. It narrows the field from a thousand
// tools to a few dozen, which is a hint no dynamic or meta attempt gets, and
// that is exactly why an individual row is published as a comparison class of
// its own rather than beside them: choice within a slice is a different
// question from choice across a catalog. It says nothing about which tool of a
// domain a step wants, in what order, or with what arguments, so a case that
// touches issues is shown every issue tool with the right one among them.
//
// A step naming a standalone tool contributes no domain, because such a tool is
// registered outside the catalog under one name on every surface. The slice
// keeps every tool it cannot place in a domain, so the standalone ones are
// shown whether or not a case names one, and this accessor stays a statement
// about the catalog rather than about a particular key's tools.
func Domains(id string) ([]string, bool) {
	for _, one := range cases() {
		if one.ID != id {
			continue
		}
		var domains []string
		for _, step := range one.key.Steps {
			domain, named := domainOf(step.Action)
			if named && !slices.Contains(domains, domain) {
				domains = append(domains, domain)
			}
		}
		sort.Strings(domains)
		return domains, true
	}
	return nil, false
}

// domainOf reads the domain half of a canonical action ID: "issue" of
// "issue.list".
//
// A step with no action, which is how a standalone tool is spelled, names no
// domain and is reported as naming none rather than as naming the empty one:
// the difference decides whether the slice reserves room for a domain that does
// not exist.
func domainOf(action Action) (string, bool) {
	domain, _, dotted := strings.Cut(string(action), ".")
	if !dotted || domain == "" {
		return "", false
	}
	return domain, true
}
