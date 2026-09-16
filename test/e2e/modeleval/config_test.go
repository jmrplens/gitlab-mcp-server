//go:build e2e

// config_test.go covers the reading of a run's configuration, which is the one
// thing that can be wrong before a provider is involved and the one thing a
// refusal costs nothing.

package modeleval

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
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
