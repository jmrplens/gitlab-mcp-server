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
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// The settings that decide the shape of the server a model talks to, and which
// models are asked. The remaining MODELEVAL_* settings, which decide which
// cases are asked and what a run may spend, arrive with the runner.
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
	return runConfig{Surfaces: surfaces, Mode: mode, Tier: tier, MetaParamSchema: schema}, nil
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
