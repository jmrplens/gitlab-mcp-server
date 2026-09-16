//go:build e2e

// config.go reads what one run was asked to measure.
//
// Every value comes through harness.Setting and never through os.Getenv, for
// the reason the harness reads its own configuration that way: the settings
// are resolved once from the process environment and the dotenv files, and a
// file this run reads must not be able to configure the harness's own process,
// because the server children are launched with an environment built from
// nothing and a leaked variable would reach one by a route nobody declared.
//
// A value that is not one of the words a setting takes refuses the run and
// names the words. The alternative, ignoring it and carrying on with the
// default, is how a run reports on the default surface under the name of the
// one that was asked for.

package modeleval

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// The settings that decide the shape of the server a model talks to, which
// models are asked, which cases they are asked, and what a run may spend.
const (
	// settingModels is the comma-separated list of provider:model;key=value
	// specs a run asks. It replaces the old evaluator's EVAL_MODELS, which
	// named the same providers and is retired with it.
	settingModels = "MODELEVAL_MODELS"
	// settingSurfaces is the comma-separated list of tool surfaces to run.
	settingSurfaces = "MODELEVAL_SURFACES"
	// settingMode is the protective mode every session runs in.
	settingMode = "MODELEVAL_MODE"
	// settingTier is the licensing tier to pin, if any.
	settingTier = "MODELEVAL_TIER"
	// settingMetaParamSchema is the meta-tool input-schema mode to serve.
	settingMetaParamSchema = "MODELEVAL_META_PARAM_SCHEMA"
	// settingCases is the comma-separated list of case identifiers to ask.
	// Empty asks every case the corpus has.
	settingCases = "MODELEVAL_CASES"
	// settingRepeat is how many times each attempt is run, so a per-case pass
	// rate over n is publishable.
	settingRepeat = "MODELEVAL_REPEAT"
	// settingParallel is how many attempts of one model may be in flight at
	// once.
	settingParallel = "MODELEVAL_PARALLEL"
	// settingBudget is the ceiling in US dollars a run stops at.
	settingBudget = "MODELEVAL_BUDGET_USD"
	// settingUnpriced lets a run start with a model the price table has no
	// figure for.
	settingUnpriced = "MODELEVAL_UNPRICED"
	// settingSpend is the consent a run asking a real provider needs.
	settingSpend = "MODELEVAL_SPEND"
	// settingSlice is how many individual tools a model is shown on the
	// individual surface.
	settingSlice = "MODELEVAL_SLICE"
)

// The bounds and defaults the counted settings are held to.
//
// Each ceiling is there to turn a typo into a refusal rather than into a bill:
// a repeat of 1000 or a parallelism of 500 is a mistyped figure every time,
// and the first would multiply a sweep's cost by a thousand while the second
// would open five hundred conversations against one GitLab.
const (
	// defaultRepeat runs each attempt once, which is what a sweep wants.
	defaultRepeat = 1
	// maxRepeat is the most repeats a run may ask for.
	maxRepeat = 50
	// defaultParallel runs one attempt of a model at a time, which is what
	// makes a run reproducible and what the scripted-session lock is sized
	// against.
	defaultParallel = 1
	// maxParallel is the most attempts of one model that may overlap.
	maxParallel = 32
	// defaultSlice is the individual surface's tool budget: the smallest
	// per-request tool cap the four providers are documented to accept.
	defaultSlice = 128
	// maxSlice bounds it at something a request can still carry.
	maxSlice = 2048
)

// runConfig is the server shape one run measures, in the harness's own terms.
type runConfig struct {
	// Surfaces are the tool surfaces to run, in the order given.
	Surfaces []harness.Surface
	// Mode is the protective mode every session runs in.
	Mode harness.Mode
	// Tier pins the licensing tier, or is TierDetect to let the child read the
	// license itself.
	Tier harness.TierPin
	// MetaParamSchema is how much of an action's parameters the meta
	// dispatchers publish, or MetaParamSchemaDefault for the child's own
	// default.
	MetaParamSchema harness.MetaParamSchema
	// Cases selects which of the corpus's cases this run asks. The empty
	// selection asks all of them.
	Cases caseSelection
	// Repeat is how many times each attempt runs.
	Repeat int
	// Parallel is how many attempts of one model may overlap.
	Parallel int
	// BudgetUSD is the ceiling the run stops at, zero for none.
	BudgetUSD float64
	// Unpriced lets a run start with a model the price table has no figure
	// for, which is what a fake run and a newly published model need.
	Unpriced bool
	// Spend is the consent a run asking a real provider needs before it sends
	// anything. The fake needs none.
	Spend bool
	// Slice is how many individual tools a model is shown on the individual
	// surface, where the whole list is over a context window outright.
	Slice int
}

// defaultSurfaces are the surfaces a run measures when it was not told.
//
// Individual is not among them, and not because it is uninteresting: the
// individual tools/list is 682,878 tokens at Ultimate, over the context window
// of at least one provider outright, so it is measured on a deterministic
// domain slice and published as its own comparison class. Running it by
// default would spend a run's budget on a request no provider can serve.
var defaultSurfaces = []harness.Surface{harness.SurfaceDynamic, harness.SurfaceMeta}

// loadRunConfig reads the run's configuration from the settings the harness
// resolved.
func loadRunConfig() (runConfig, error) { return parseRunConfig(harness.Setting) }

// configuredModels reads the models this run was told to ask.
//
// It is apart from [runConfig] because the two are needed at different moments
// by different callers: the server shape is what a session is opened with, and
// the model list is what the contract probe reads without opening one at all.
func configuredModels() ([]provider.Spec, error) {
	return provider.ParseSpecs(harness.Setting(settingModels))
}

// modelsNamed reports whether this run was told to ask anybody at all.
//
// It is read before the configuration is otherwise acted on, because a run
// that names no model is the ordinary state of this package: it is not run by
// CI and costs money when it is, so `go test ./test/e2e/modeleval/` has to be
// a run of the offline halves and nothing else, and it has to reach no GitLab
// to be one.
func modelsNamed() bool {
	return strings.TrimSpace(harness.Setting(settingModels)) != ""
}

// credentialFor reads one provider's credential out of the run's settings.
//
// It goes through harness.Setting like everything else here, which is what lets
// a probe learn whether a key is configured without requiring the GitLab an Env
// would bootstrap.
func credentialFor(spec provider.Spec) string {
	name, needed := provider.KeyName(spec.Provider)
	if !needed {
		return ""
	}
	return strings.TrimSpace(harness.Setting(name))
}

// parseRunConfig is loadRunConfig with the reader passed in, so the parsing is
// testable without a run.
func parseRunConfig(read func(string) string) (runConfig, error) {
	surfaces, err := parseSurfaces(read(settingSurfaces))
	if err != nil {
		return runConfig{}, err
	}
	mode, err := parseMode(read(settingMode))
	if err != nil {
		return runConfig{}, err
	}
	tier, err := parseTier(read(settingTier))
	if err != nil {
		return runConfig{}, err
	}
	schema, err := parseMetaParamSchema(read(settingMetaParamSchema))
	if err != nil {
		return runConfig{}, err
	}
	selection, err := parseCases(read(settingCases))
	if err != nil {
		return runConfig{}, err
	}
	repeat, err := parseCount(settingRepeat, read(settingRepeat), defaultRepeat, maxRepeat)
	if err != nil {
		return runConfig{}, err
	}
	parallel, err := parseCount(settingParallel, read(settingParallel), defaultParallel, maxParallel)
	if err != nil {
		return runConfig{}, err
	}
	slice, err := parseCount(settingSlice, read(settingSlice), defaultSlice, maxSlice)
	if err != nil {
		return runConfig{}, err
	}
	budget, err := parseBudget(read(settingBudget))
	if err != nil {
		return runConfig{}, err
	}
	unpriced, err := parseConsent(settingUnpriced, read(settingUnpriced))
	if err != nil {
		return runConfig{}, err
	}
	spend, err := parseConsent(settingSpend, read(settingSpend))
	if err != nil {
		return runConfig{}, err
	}
	return runConfig{
		Surfaces:        surfaces,
		Mode:            mode,
		Tier:            tier,
		MetaParamSchema: schema,
		Cases:           selection,
		Repeat:          repeat,
		Parallel:        parallel,
		BudgetUSD:       budget,
		Unpriced:        unpriced,
		Spend:           spend,
		Slice:           slice,
	}, nil
}

// caseSelection is the set of cases a run asks, empty for all of them.
type caseSelection struct {
	ids []string
}

// Admits reports whether one case is in the selection.
func (s caseSelection) Admits(id string) bool {
	return len(s.ids) == 0 || slices.Contains(s.ids, id)
}

// Named returns the identifiers the selection names, in the order given.
func (s caseSelection) Named() []string { return slices.Clone(s.ids) }

// parseCases reads the case list, and refuses an identifier the corpus does
// not have.
//
// Refusing rather than ignoring is what makes a mistyped identifier visible:
// an unknown case silently dropped leaves a run that measures the cases that
// happened to be spelled right, reports on those, and says nothing about the
// one the maintainer meant to ask about.
func parseCases(value string) (caseSelection, error) {
	words := splitList(value)
	if len(words) == 0 {
		return caseSelection{}, nil
	}

	known := modelcorpus.IDs()
	var selected []string
	for _, word := range words {
		id := strings.ToUpper(word)
		if !slices.Contains(known, id) {
			return caseSelection{}, fmt.Errorf("%s names %q, which the corpus has no case for", settingCases, word)
		}
		if !slices.Contains(selected, id) {
			selected = append(selected, id)
		}
	}
	return caseSelection{ids: selected}, nil
}

// parseCount reads one whole-number setting, held between one and a ceiling.
//
// Zero and a negative are refused rather than read as the default: a run told
// to repeat each attempt zero times asked for something, and answering it with
// one attempt each is a run reporting under a configuration it did not have.
func parseCount(setting, value string, fallback, ceiling int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	count, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a whole number", setting, value)
	}
	if count < 1 || count > ceiling {
		return 0, fmt.Errorf("%s=%d is outside 1..%d", setting, count, ceiling)
	}
	return count, nil
}

// parseBudget reads the spending ceiling, in US dollars.
//
// Zero is no ceiling rather than a ceiling of nothing, which is the reading
// every other optional bound in this repository takes; a run that means to
// spend nothing asks the fake.
func parseBudget(value string) (float64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	budget, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a number of dollars", settingBudget, value)
	}
	if budget < 0 {
		return 0, fmt.Errorf("%s=%s is negative", settingBudget, trimmed)
	}
	return budget, nil
}

// consentWords are the values that read as yes. They are few and spelled out,
// because the thing being consented to is money.
var consentWords = []string{"yes", "true", "1"}

// parseConsent reads a yes-or-no setting, and refuses a word that is neither.
//
// A misspelled consent must not read as no. A run refused for a typo costs a
// second attempt at the command line; a run that read "ys" as no and started
// anyway would be a run that spent nothing and reported that the models
// declined everything.
func parseConsent(setting, value string) (bool, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch {
	case trimmed == "":
		return false, nil
	case slices.Contains(consentWords, trimmed):
		return true, nil
	case trimmed == "no" || trimmed == "false" || trimmed == "0":
		return false, nil
	default:
		return false, unknownValue(setting, value, append(slices.Clone(consentWords), "no", "false", "0"))
	}
}

// parseSurfaces reads the comma-separated surface list.
//
// A surface named twice is kept once rather than refused: a list assembled by
// a shell loop is allowed to repeat itself, and running one surface twice
// would double a run's cost for no second measurement.
func parseSurfaces(value string) ([]harness.Surface, error) {
	words := splitList(value)
	if len(words) == 0 {
		return slices.Clone(defaultSurfaces), nil
	}

	var surfaces []harness.Surface
	for _, word := range words {
		surface := harness.Surface(word)
		if !slices.Contains(harness.AllSurfaces(), surface) {
			return nil, unknownValue(settingSurfaces, word, surfaceNames())
		}
		if !slices.Contains(surfaces, surface) {
			surfaces = append(surfaces, surface)
		}
	}
	return surfaces, nil
}

// surfaceNames spells the surfaces for a refusal message.
func surfaceNames() []string {
	names := make([]string, 0, len(harness.AllSurfaces()))
	for _, surface := range harness.AllSurfaces() {
		names = append(names, surface.String())
	}
	return names
}

// parseMode reads the protective mode. The default is the mode a deployment
// runs in with nothing set.
func parseMode(value string) (harness.Mode, error) {
	switch mode := harness.Mode(strings.TrimSpace(value)); mode {
	case "":
		return harness.ModeDefault, nil
	case harness.ModeDefault, harness.ModeReadOnly, harness.ModeSafe:
		return mode, nil
	default:
		return "", unknownValue(settingMode, value,
			[]string{harness.ModeDefault.String(), harness.ModeReadOnly.String(), harness.ModeSafe.String()})
	}
}

// parseTier reads the tier pin. Empty is no pin at all, which is what lets the
// child read the license itself, as a deployment does.
func parseTier(value string) (harness.TierPin, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return harness.TierDetect, nil
	}
	pin := harness.TierPin(trimmed)
	if !slices.Contains(harness.AllTierPins(), pin) {
		return "", unknownValue(settingTier, value, tierNames())
	}
	return pin, nil
}

// tierNames spells the pins for a refusal message.
func tierNames() []string {
	names := make([]string, 0, len(harness.AllTierPins()))
	for _, pin := range harness.AllTierPins() {
		names = append(names, pin.String())
	}
	return names
}

// parseMetaParamSchema reads the meta-tool input-schema mode.
//
// Empty is the child's own default, which is opaque: params is an object with
// no properties, so a model on the meta surface learns a parameter name from a
// tool description and from a refusal and nowhere else. That is the
// configuration a deployment gets and so the one the baseline row measures;
// the other two modes are rows of their own.
func parseMetaParamSchema(value string) (harness.MetaParamSchema, error) {
	switch schema := harness.MetaParamSchema(strings.TrimSpace(value)); schema {
	case harness.MetaParamSchemaDefault:
		return harness.MetaParamSchemaDefault, nil
	case harness.MetaParamSchemaOpaque, harness.MetaParamSchemaCompact, harness.MetaParamSchemaFull:
		return schema, nil
	default:
		return "", unknownValue(settingMetaParamSchema, value, []string{
			harness.MetaParamSchemaOpaque.String(),
			harness.MetaParamSchemaCompact.String(),
			harness.MetaParamSchemaFull.String(),
		})
	}
}

// splitList reads a comma-separated setting, dropping the empty entries a
// trailing comma or a blank value leaves behind.
func splitList(value string) []string {
	var words []string
	for word := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(word); trimmed != "" {
			words = append(words, trimmed)
		}
	}
	return words
}

// unknownValue refuses a setting and names what it takes.
func unknownValue(setting, value string, accepted []string) error {
	return fmt.Errorf("%s=%q is not one of %s", setting, value, strings.Join(accepted, ", "))
}
