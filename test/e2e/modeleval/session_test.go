//go:build e2e

// session_test.go covers the shape a run asks for, which is offline: a
// ServerConfig is a value, and whether the right one is built decides which
// server the whole run is measured against.

package modeleval

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestServerShape_TheRunsConfiguration_ReachesEveryFieldTheBinaryReads checks
// that each setting arrives at the server.
//
// The schema mode is the one that had nowhere to arrive until this rebuild:
// every session ran on the default, so on the meta surface nothing could ever
// have measured a model that was shown parameter names.
func TestServerShape_TheRunsConfiguration_ReachesEveryFieldTheBinaryReads(t *testing.T) {
	shape := serverShape(runConfig{
		Mode:            harness.ModeSafe,
		Tier:            harness.TierUltimate,
		MetaParamSchema: harness.MetaParamSchemaFull,
	}, harness.SurfaceMeta)

	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "surface", got: shape.Surface.String(), want: harness.SurfaceMeta.String()},
		{name: "mode", got: shape.Mode.String(), want: harness.ModeSafe.String()},
		{name: "tier", got: shape.Tier.String(), want: harness.TierUltimate.String()},
		{name: "schema", got: shape.MetaParamSchema.String(), want: harness.MetaParamSchemaFull.String()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("%s = %q, want %q", testCase.name, testCase.got, testCase.want)
			}
		})
	}
}

// TestServerShape_NothingAsked_LeavesTheBinaryItsOwnDefaults checks that a run
// that named no mode, tier or schema asks the server for nothing.
//
// Each of the three empty values means "decide for yourself" to the harness,
// and that is the measurement the baseline rows are: the catalog a deployment
// is served, from the license the credential can read, with the schemas the
// child publishes by default.
func TestServerShape_NothingAsked_LeavesTheBinaryItsOwnDefaults(t *testing.T) {
	shape := serverShape(runConfig{}, harness.SurfaceDynamic)

	if shape.Mode != "" {
		t.Errorf("Mode = %q, want empty", shape.Mode)
	}
	if shape.Tier != harness.TierDetect {
		t.Errorf("Tier = %q, want no pin", shape.Tier)
	}
	if shape.MetaParamSchema != harness.MetaParamSchemaDefault {
		t.Errorf("MetaParamSchema = %q, want the child's own default", shape.MetaParamSchema)
	}
	if shape.Token != "" {
		t.Errorf("Token = %q, want empty: the run's own credential, since a narrowed one would be a "+
			"different catalog from the one the row names", shape.Token)
	}
	if len(shape.ExcludeTools) != 0 {
		t.Errorf("ExcludeTools = %v, want none", shape.ExcludeTools)
	}
}
