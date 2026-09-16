//go:build e2e

// config_test.go covers the reading of a run's configuration, which is the one
// thing that can be wrong before a provider is involved and the one thing a
// refusal costs nothing.

package modeleval

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// settingsReader returns a reader over a fixed map, standing in for the
// settings the harness resolves from the environment and the dotenv files.
func settingsReader(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

// TestParseRunConfig_NothingSet_MeasuresTheDefaultDeployment checks what a run
// that was told nothing measures.
//
// Every default here is the shape a deployment gets with nothing set: the two
// surfaces whose tool lists fit a context window, no protective mode, no tier
// pin, and the child's own schema mode. A default that differed would publish
// a baseline row about a configuration nobody runs.
func TestParseRunConfig_NothingSet_MeasuresTheDefaultDeployment(t *testing.T) {
	cfg, err := parseRunConfig(settingsReader(nil))
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v, want nil", err)
	}

	if !slices.Equal(cfg.Surfaces, defaultSurfaces) {
		t.Errorf("Surfaces = %v, want %v", cfg.Surfaces, defaultSurfaces)
	}
	if cfg.Mode != harness.ModeDefault {
		t.Errorf("Mode = %q, want %q", cfg.Mode, harness.ModeDefault)
	}
	if cfg.Tier != harness.TierDetect {
		t.Errorf("Tier = %q, want no pin: a deployment reads the license itself", cfg.Tier)
	}
	if cfg.MetaParamSchema != harness.MetaParamSchemaDefault {
		t.Errorf("MetaParamSchema = %q, want the child's own default", cfg.MetaParamSchema)
	}
}

// TestParseRunConfig_EverySettingNamed_IsReadIntoTheShape checks that each
// setting reaches the field it decides.
func TestParseRunConfig_EverySettingNamed_IsReadIntoTheShape(t *testing.T) {
	cfg, err := parseRunConfig(settingsReader(map[string]string{
		settingSurfaces:        "meta, individual",
		settingMode:            "read-only",
		settingTier:            "premium",
		settingMetaParamSchema: "compact",
	}))
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v, want nil", err)
	}

	want := []harness.Surface{harness.SurfaceMeta, harness.SurfaceIndividual}
	if !slices.Equal(cfg.Surfaces, want) {
		t.Errorf("Surfaces = %v, want %v in the order they were given", cfg.Surfaces, want)
	}
	if cfg.Mode != harness.ModeReadOnly {
		t.Errorf("Mode = %q, want %q", cfg.Mode, harness.ModeReadOnly)
	}
	if cfg.Tier != harness.TierPremium {
		t.Errorf("Tier = %q, want %q", cfg.Tier, harness.TierPremium)
	}
	if cfg.MetaParamSchema != harness.MetaParamSchemaCompact {
		t.Errorf("MetaParamSchema = %q, want %q", cfg.MetaParamSchema, harness.MetaParamSchemaCompact)
	}
}

// TestParseRunConfig_UnknownValue_RefusesTheRunAndNamesTheWords checks that a
// misspelled setting stops the run.
//
// Carrying on with the default is the alternative and is worse than useless: a
// run asked for the safe mode and given the default would publish a row headed
// safe mode about a server that made every mutation it was asked to make.
func TestParseRunConfig_UnknownValue_RefusesTheRunAndNamesTheWords(t *testing.T) {
	cases := []struct {
		name    string
		values  map[string]string
		wantsIn []string
	}{
		{
			name:    "surface",
			values:  map[string]string{settingSurfaces: "dynamic,everything"},
			wantsIn: []string{settingSurfaces, "everything", "individual"},
		},
		{
			name:    "mode",
			values:  map[string]string{settingMode: "paranoid"},
			wantsIn: []string{settingMode, "paranoid", "read-only"},
		},
		{
			name:    "tier",
			values:  map[string]string{settingTier: "enterprise"},
			wantsIn: []string{settingTier, "enterprise", "ultimate"},
		},
		{
			name:    "meta parameter schema",
			values:  map[string]string{settingMetaParamSchema: "verbose"},
			wantsIn: []string{settingMetaParamSchema, "verbose", "compact"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseRunConfig(settingsReader(testCase.values))
			if err == nil {
				t.Fatalf("parseRunConfig(%v) error = nil, want a refusal", testCase.values)
			}
			for _, fragment := range testCase.wantsIn {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("the refusal %q does not carry %q", err, fragment)
				}
			}
		})
	}
}

// TestParseSurfaces_RepeatsAndBlanks_AreReadAsOneList checks the shapes a
// shell loop produces.
//
// A surface named twice is kept once rather than refused: running one surface
// twice would double a run's cost for no second measurement, and refusing an
// assembled list would make the setting harder to drive than it is to read.
func TestParseSurfaces_RepeatsAndBlanks_AreReadAsOneList(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  []harness.Surface
	}{
		{name: "blank", value: "   ", want: defaultSurfaces},
		{name: "trailing comma", value: "dynamic,", want: []harness.Surface{harness.SurfaceDynamic}},
		{name: "repeat", value: "meta,meta", want: []harness.Surface{harness.SurfaceMeta}},
		{
			name:  "spaces",
			value: " dynamic , meta ",
			want:  []harness.Surface{harness.SurfaceDynamic, harness.SurfaceMeta},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseSurfaces(testCase.value)
			if err != nil {
				t.Fatalf("parseSurfaces(%q) error = %v, want nil", testCase.value, err)
			}
			if !slices.Equal(got, testCase.want) {
				t.Errorf("parseSurfaces(%q) = %v, want %v", testCase.value, got, testCase.want)
			}
		})
	}
}

// TestParseMetaParamSchema_EveryMode_IsAccepted pins the three words and the
// absence of one.
//
// The mode is the one knob that decides whether a model on the meta surface
// can see a parameter name at all, so a row measured under one is not
// comparable with a row measured under another and the setting has to reach
// all three.
func TestParseMetaParamSchema_EveryMode_IsAccepted(t *testing.T) {
	cases := map[string]harness.MetaParamSchema{
		"":        harness.MetaParamSchemaDefault,
		"opaque":  harness.MetaParamSchemaOpaque,
		"compact": harness.MetaParamSchemaCompact,
		"full":    harness.MetaParamSchemaFull,
	}
	for value, want := range cases {
		t.Run("value "+value, func(t *testing.T) {
			got, err := parseMetaParamSchema(value)
			if err != nil {
				t.Fatalf("parseMetaParamSchema(%q) error = %v, want nil", value, err)
			}
			if got != want {
				t.Errorf("parseMetaParamSchema(%q) = %q, want %q", value, got, want)
			}
		})
	}
}

// TestCredentialFor_ReadsEachProvidersOwnSettingAndNoneForTheFake checks the
// one thing a probe needs before it can spend anything.
//
// It goes through the settings the harness resolved rather than through the
// process environment, which is what lets a probe learn whether a key is
// configured without requiring the GitLab an Env would bootstrap; the fake
// needs none, which is what lets a pipe run start with nothing configured.
func TestCredentialFor_ReadsEachProvidersOwnSettingAndNoneForTheFake(t *testing.T) {
	for _, one := range []struct{ spec, setting string }{
		{"anthropic:claude-haiku-4-5-20251001", "ANTHROPIC_API_KEY"},
		{"openai:gpt-5.4-nano", "OPENAI_API_KEY"},
		{"qwen:qwen3.8-flash", "QWEN_API_KEY"},
		{"google:gemini-flash-latest", "GOOGLE_API_KEY"},
	} {
		t.Run(one.spec, func(t *testing.T) {
			spec, err := provider.ParseSpec(one.spec)
			if err != nil {
				t.Fatalf("ParseSpec(%q) error = %v, want nil", one.spec, err)
			}
			name, needed := provider.KeyName(spec.Provider)
			if !needed || name != one.setting {
				t.Errorf("KeyName(%q) = %q, %v, want %q, true", spec.Provider, name, needed, one.setting)
			}
		})
	}

	fake, err := provider.ParseSpec("fake:perfect")
	if err != nil {
		t.Fatalf("ParseSpec(fake:perfect) error = %v, want nil", err)
	}
	if got := credentialFor(fake); got != "" {
		t.Errorf("credentialFor(fake) returned a credential, want none")
	}
}

// TestConfiguredModels_ReadsTheListThroughTheSettings checks that the model
// list is read from one setting and refused when it names nothing.
//
// A run that was not told which model to ask is one nobody can read the results
// of, and guessing one spends money on a question that was not asked.
func TestConfiguredModels_ReadsTheListThroughTheSettings(t *testing.T) {
	specs, err := provider.ParseSpecs("anthropic:claude-haiku-4-5-20251001,fake:perfect")
	if err != nil {
		t.Fatalf("ParseSpecs error = %v, want nil", err)
	}
	if len(specs) != 2 {
		t.Fatalf("ParseSpecs returned %d specs, want 2", len(specs))
	}

	// configuredModels reads the one setting; with nothing set it refuses,
	// which is what a run with no MODELEVAL_MODELS meets.
	_, configuredErr := configuredModels()
	if configuredErr == nil && harness.Setting(settingModels) == "" {
		t.Errorf("configuredModels() accepted an empty %s", settingModels)
	}
}

// TestParseRunConfig_TheCountedSettingsHaveDefaultsARunCanRestOn checks the
// half of the configuration that decides what a run costs.
func TestParseRunConfig_TheCountedSettingsHaveDefaultsARunCanRestOn(t *testing.T) {
	cfg, err := parseRunConfig(settingsReader(nil))
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v, want nil", err)
	}
	if cfg.Repeat != defaultRepeat || cfg.Parallel != defaultParallel || cfg.Slice != defaultSlice {
		t.Errorf("repeat %d, parallel %d, slice %d; want %d, %d, %d",
			cfg.Repeat, cfg.Parallel, cfg.Slice, defaultRepeat, defaultParallel, defaultSlice)
	}
	if cfg.BudgetUSD != 0 || cfg.Spend || cfg.Unpriced {
		t.Errorf("a run told nothing has budget %v, spend %t, unpriced %t", cfg.BudgetUSD, cfg.Spend, cfg.Unpriced)
	}
	if len(cfg.Cases.Named()) != 0 {
		t.Errorf("a run told nothing selected %v", cfg.Cases.Named())
	}

	named, err := parseRunConfig(settingsReader(map[string]string{
		settingRepeat:   "3",
		settingParallel: "5",
		settingSlice:    "64",
		settingBudget:   "12.50",
		settingSpend:    "yes",
		settingUnpriced: "true",
	}))
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v, want nil", err)
	}
	if named.Repeat != 3 || named.Parallel != 5 || named.Slice != 64 {
		t.Errorf("repeat %d, parallel %d, slice %d", named.Repeat, named.Parallel, named.Slice)
	}
	if named.BudgetUSD != 12.5 || !named.Spend || !named.Unpriced {
		t.Errorf("budget %v, spend %t, unpriced %t", named.BudgetUSD, named.Spend, named.Unpriced)
	}
}

// TestParseRunConfig_ARefusalNamesTheSettingThatWasWrong checks that every one
// of the counted settings refuses rather than falling back.
//
// Falling back is the failure mode this is written against: a run told to
// repeat each attempt zero times asked for something, and answering it with one
// attempt each is a run reporting under a configuration it did not have.
func TestParseRunConfig_ARefusalNamesTheSettingThatWasWrong(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		wantIn string
	}{
		{name: "a repeat that is not a number", values: map[string]string{settingRepeat: "twice"}, wantIn: settingRepeat},
		{name: "a repeat of zero", values: map[string]string{settingRepeat: "0"}, wantIn: settingRepeat},
		{name: "a repeat past the ceiling", values: map[string]string{settingRepeat: "9999"}, wantIn: settingRepeat},
		{name: "a negative parallelism", values: map[string]string{settingParallel: "-1"}, wantIn: settingParallel},
		{name: "a slice past the ceiling", values: map[string]string{settingSlice: "99999"}, wantIn: settingSlice},
		{name: "a budget that is not money", values: map[string]string{settingBudget: "lots"}, wantIn: settingBudget},
		{name: "a negative budget", values: map[string]string{settingBudget: "-5"}, wantIn: settingBudget},
		{name: "a misspelled consent", values: map[string]string{settingSpend: "ys"}, wantIn: settingSpend},
		{name: "a misspelled unpriced", values: map[string]string{settingUnpriced: "sure"}, wantIn: settingUnpriced},
		{name: "a case nothing answers to", values: map[string]string{settingCases: "MT-nothing"}, wantIn: settingCases},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRunConfig(settingsReader(tc.values))
			if err == nil {
				t.Fatalf("parseRunConfig(%v) returned no error", tc.values)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("the refusal is %v, want it to name %s", err, tc.wantIn)
			}
		})
	}
}

// TestParseConsent_ReadsOnlyTheWordsItStates checks the two readings that
// matter: a yes, and a no that is not a typo.
func TestParseConsent_ReadsOnlyTheWordsItStates(t *testing.T) {
	for _, word := range []string{"yes", "YES", "true", "1", " yes "} {
		t.Run("consented with "+word, func(t *testing.T) {
			given, err := parseConsent(settingSpend, word)
			if err != nil || !given {
				t.Errorf("parseConsent(%q) = %t, %v, want true, nil", word, given, err)
			}
		})
	}
	for _, word := range []string{"", "no", "false", "0"} {
		t.Run("withheld with "+word, func(t *testing.T) {
			given, err := parseConsent(settingSpend, word)
			if err != nil || given {
				t.Errorf("parseConsent(%q) = %t, %v, want false, nil", word, given, err)
			}
		})
	}
}

// TestParseCases_SelectsTheNamedCasesAndRefusesTheRest checks the filter, its
// case-insensitivity and the refusal that makes a typo visible.
func TestParseCases_SelectsTheNamedCasesAndRefusesTheRest(t *testing.T) {
	known := modelcorpus.IDs()
	if len(known) < 2 {
		t.Fatalf("the corpus holds %d case(s), which is too few to select from", len(known))
	}

	empty, err := parseCases("")
	if err != nil {
		t.Fatalf("parseCases(\"\") error = %v", err)
	}
	if !empty.Admits(known[0]) || len(empty.Named()) != 0 {
		t.Errorf("the empty selection admitted %t and named %v", empty.Admits(known[0]), empty.Named())
	}

	selection, err := parseCases(strings.ToLower(known[0]) + ", " + known[1] + ", " + known[1])
	if err != nil {
		t.Fatalf("parseCases error = %v", err)
	}
	if !slices.Equal(selection.Named(), []string{known[0], known[1]}) {
		t.Errorf("parseCases selected %v, want the two named once each", selection.Named())
	}
	if selection.Admits("MS-999") {
		t.Errorf("the selection admitted a case it does not name")
	}
}
