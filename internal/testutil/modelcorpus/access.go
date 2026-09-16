package modelcorpus

import (
	"fmt"
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
	all := make([]Case, 0, 128)
	all = append(all, readCases()...)
	all = append(all, mutatingCases()...)
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
// publisher refuses. The map that test reads lists this package too, since the
// corpus's own gate is written against the key, which is why it holds five
// entries and this sentence names four.
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
