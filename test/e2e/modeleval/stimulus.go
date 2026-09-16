//go:build e2e

// stimulus.go composes what a model is shown: the contract for the surface it
// was given, and the case's own prompt rendered over the world that was built
// for it.
//
// Nothing here reads an answer, and that is the file's whole job. The corpus
// holds a stimulus and its key in one value with the key unexported, so the
// only way a prompt could carry the answer is for a run to look one up and
// write it in; this is the file where such a lookup would live, so
// stimulus_test.go parses it and fails on any mention of modelcorpus.Keys.
// That is a second lock on a door the corpus's own boundary test already
// bolts, and it is worth having because this is the one file whose output is
// sent to a provider.
//
// A fact has several spellings, and the difference between what is rendered
// and what is compared is deliberate. A project is a numeric id and a path,
// and the scorer accepts either; the prompt shows one, which is the first the
// world listed, because a request that offers a model two names for one thing
// is measuring something nobody asked about.

package modeleval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// stimulus is one case as it was put to a model: the text as sent and the
// facts it was rendered from.
//
// Both halves are written to the record. The text is what the model read, so a
// prompt that gave its own answer away can be found by reading the shard; the
// facts are what makes a failed attempt reproducible, since a prompt naming
// project 42 says nothing without the project that was 42 that day.
type stimulus struct {
	// Text is the rendered prompt.
	Text string
	// Facts are the values it was rendered with, one spelling each.
	Facts map[string]string
}

// renderStimulus renders one case's prompt over the world that was built for
// it.
//
// A template naming a fact the world did not produce is an error rather than a
// hole in the text, which is [modelcorpus.Render]'s own rule: a stimulus with
// a hole in it is a paid request that measures nothing.
func renderStimulus(one modelcorpus.Stimulus, world World) (stimulus, error) {
	facts := renderedFacts(world.Facts)
	text, err := modelcorpus.Render(one.Prompt, facts)
	if err != nil {
		return stimulus{}, fmt.Errorf("case %s: %w", one.ID, err)
	}
	return stimulus{Text: text, Facts: facts}, nil
}

// renderedFacts reduces a world's facts to the one spelling a prompt shows.
//
// The first is the one the world put first, which is the recipe's own
// judgement about which name a person would use: a project is named by its
// path and an issue by its internal id, whatever else the scorer will accept
// for them.
func renderedFacts(facts map[string][]string) map[string]string {
	rendered := make(map[string]string, len(facts))
	for key, spellings := range facts {
		if len(spellings) > 0 {
			rendered[key] = spellings[0]
		}
	}
	return rendered
}

// contractFor returns the text one surface is introduced with.
func contractFor(surface harness.Surface) (string, error) {
	contract, known := modelcorpus.Contract(modelcorpus.Surface(surface))
	if !known {
		return "", fmt.Errorf("no surface contract for %q", surface)
	}
	return contract, nil
}

// contractDigest fingerprints the three surface contracts together.
//
// Together rather than one per surface, because it answers one question: is
// this row comparable with that one. Two runs whose contracts differ at all
// are not comparable even on the surface whose own text did not move, since
// the three are edited as one document and a change to the dynamic text is
// evidence that the reading of what a contract may say has changed.
func contractDigest() string {
	sum := sha256.New()
	for _, surface := range modelcorpus.Surfaces() {
		contract, known := modelcorpus.Contract(surface)
		_, _ = fmt.Fprintf(sum, "%s\x00%t\x00%s\n", surface, known, contract)
	}
	return hex.EncodeToString(sum.Sum(nil))[:digestLength]
}

// digestLength is how much of a hash this package's fingerprints carry, the
// same sixteen hex characters the corpus digest and the tool-schema digest
// use: this is read by a person comparing two rows of a table.
const digestLength = 16

// needsOf turns a case's declared needs into the harness's own, so an instance
// that cannot run the case skips it with a reason instead of failing it.
//
// The tier is parsed rather than mapped, so a tier the corpus spells and the
// edition package does not is a refusal here rather than a case that silently
// runs everywhere.
func needsOf(needs modelcorpus.Needs) ([]harness.Need, error) {
	var declared []harness.Need
	tier, known := edition.ParseTier(string(needs.MinimumTier()))
	if !known {
		return nil, fmt.Errorf("unknown tier %q", needs.MinimumTier())
	}
	if tier != edition.Free {
		declared = append(declared, harness.Tier(tier))
	}
	if needs.Runner {
		declared = append(declared, harness.NeedRunner)
	}
	if needs.Admin {
		declared = append(declared, harness.NeedAdmin)
	}
	if needs.FixtureService {
		declared = append(declared, harness.NeedFixtureService)
	}
	return declared, nil
}

// surfacesFor returns the surfaces one case runs on: the ones this run was
// configured with, narrowed by the case's own restriction.
//
// A case restricted to surfaces the run did not configure runs on none, and
// that is an absence with a reason rather than a failure: a run measuring the
// dynamic surface alone should not fail because a case declared itself meta
// only.
func surfacesFor(one modelcorpus.Stimulus, configured []harness.Surface) []harness.Surface {
	var allowed []harness.Surface
	for _, surface := range configured {
		if one.Surfaces.Runs(modelcorpus.Surface(surface)) {
			allowed = append(allowed, surface)
		}
	}
	return allowed
}

// restrictionReason is what a report is told about the surfaces a case left
// out, and it is required of any case that leaves one out.
//
// The harness states the same rule for its own scenarios and this passes both
// halves through to it. A run that invented a reason here would be publishing
// its own excuse for a hole the corpus declared.
func restrictionReason(one modelcorpus.Stimulus) string {
	return one.Surfaces.Reason
}

// restricted reports whether a case declares a surface restriction at all.
func restricted(one modelcorpus.Stimulus) bool {
	return len(one.Surfaces.Only) > 0
}

// modelName is how a spec names its model in a subtest and in a lock.
//
// A spec carries characters the testing package rewrites in a subtest name
// (a colon, a semicolon, an equals sign), so a name built from one is spelled
// here once rather than per call site: two specs that differed only in an
// option would otherwise produce two subtests whose names the log could not
// tell apart.
func modelName(spec string) string {
	replaced := strings.NewReplacer(":", "-", ";", "-", "=", "-", " ", "-", "/", "-").Replace(spec)
	return strings.Trim(replaced, "-")
}

// attemptID names one attempt across every line of the record that mentions
// it.
//
// It is built from the five things that make an attempt distinct rather than
// from a counter, so a shard read months later says what an identifier was
// without a second file to join against.
func attemptID(runID, caseID, model string, surface harness.Surface, repeat int) string {
	return fmt.Sprintf("%s/%s/%s/%s/r%d", runID, caseID, modelName(model), surface, repeat)
}

// selectedStimuli returns the cases this run asks, in corpus order.
func selectedStimuli(selection caseSelection) []modelcorpus.Stimulus {
	var selected []modelcorpus.Stimulus
	for _, one := range modelcorpus.Stimuli() {
		if selection.Admits(one.ID) {
			selected = append(selected, one)
		}
	}
	return selected
}

// unselectedCases returns the identifiers a selection named that no stimulus
// answered to.
//
// It cannot happen through [parseCases], which refuses an unknown identifier
// against the same corpus. It is checked again here because the two readings
// are of different things: one asks whether the corpus has the case, and this
// asks whether the run is about to attempt it, and a selection that silently
// attempted nothing would be a run reporting on an empty corpus.
func unselectedCases(selection caseSelection, selected []modelcorpus.Stimulus) []string {
	attempted := make([]string, 0, len(selected))
	for _, one := range selected {
		attempted = append(attempted, one.ID)
	}
	var missing []string
	for _, id := range selection.Named() {
		if !slices.Contains(attempted, id) {
			missing = append(missing, id)
		}
	}
	return missing
}
