//go:build e2e

// reads_sweep_test.go exercises every served read-only action whose required
// parameters bind from the shared World, on all three surfaces.
//
// It is a sweep rather than a scenario: it names no action by hand, walks the
// surface the server publishes through gitlab://tools, and binds each action's
// required parameters from the World a read cannot change. An action whose
// requirements the World cannot supply is named in the log rather than swept,
// which is the honest "or names the binding it lacks" the coverage report
// reads beside the reads it did exercise. The sweep calls carry purpose sweep,
// so their credit shows apart from the scenario credit later steps add, and it
// never asserts on an answer: the recorder credits a call by what the server
// did, so a read that answers empty or with a handled error is still counted.
//
// This file also holds the binding and per-surface helpers the previews sweep
// beside it reuses, since both walk the manifest and bind from the same World.

package common

import (
	"encoding/json"
	"maps"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// sweepCandidate is one catalog action a sweep can call: its canonical id and
// the arguments bound from the World, each spelled in the type its schema
// declares.
type sweepCandidate struct {
	id     harness.ActionID
	params map[string]any
}

// unboundEntry is a manifest entry a sweep could not bind, with the reason,
// so the sweep names what it lacks instead of leaving a silent hole.
type unboundEntry struct {
	id     string
	reason string
}

// TestReads_Sweep reads every read-only action whose required parameters bind
// from the World, on the dynamic, meta and individual surfaces.
//
// Replaces: the list-and-get coverage the old per-domain suite spread across
// its TestIndividual_*, TestMeta_* and TestDynamicToolSurface_* families.
func TestReads_Sweep(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)

	candidates, unbound := sweepCandidates(e, world, true)
	if len(candidates) == 0 {
		t.Fatal("no read action bound from the World; the reads sweep would prove nothing")
	}
	for _, u := range unbound {
		t.Logf("read %s not swept: %s", u.id, u.reason)
	}

	sweepSurfaces(t, harness.ModeDefault, func(e *harness.Env, s *harness.Session) {
		swept, notServed, errs := 0, 0, 0
		for _, c := range candidates {
			if !s.Serves(c.id) {
				notServed++
				continue
			}
			if _, err := harness.Try[json.RawMessage](s, c.id, c.params, harness.For(harness.PurposeSweep)); err != nil {
				errs++
			}
			swept++
		}
		e.T.Logf("swept %d reads on %s (%d not served here, %d answered with an error the recorder still credits)",
			swept, s.Surface(), notServed, errs)
	})
}

// sweepCandidates reads the dynamic manifest and returns the actions of one
// kind (read-only when readOnly is true, mutating otherwise) whose required
// parameters bind from the World, plus the ones it could not bind and why.
//
// The dynamic manifest is read because its entry ids are the canonical action
// ids the harness verbs project onto every surface; the meta and individual
// manifests spell the same actions as their own tool names, which the verbs do
// not take. Read-only-ness and the required parameters are catalog properties,
// the same whichever surface serves the action.
func sweepCandidates(e *harness.Env, world *fixture.World, readOnly bool) (candidates []sweepCandidate, unbound []unboundEntry) {
	e.T.Helper()

	manifest := readToolsManifest(e, e.On(harness.SurfaceDynamic))
	for _, entry := range manifest.Entries {
		if entry.Kind != manifestKindDynamicAction || entry.ReadOnly != readOnly || entry.AliasOf != "" {
			continue
		}
		// The standalone actions (discover.project, the interactive flows) are
		// listed in the dynamic manifest but are registered outside the base
		// catalog, so the harness verbs cannot project them; they are covered
		// through Raw in the modes test instead. ActionTier answers from that
		// catalog, so an id it does not know is one to skip here.
		tier, known := e.ActionTier(harness.ActionID(entry.ID))
		if !known {
			continue
		}
		// A licensed instance serves the run's admin token the Premium and
		// Ultimate actions too, and they appear in the manifest here; the
		// common package runs on Free actions on every runtime, so a licensed
		// one is the ee package's to sweep, not this one's. Calling it would be
		// refused by the harness for the tier the package declares.
		if tier.IsEnterprise() {
			continue
		}
		params, missing := bindEntryParams(world, entry)
		if missing != "" {
			unbound = append(unbound, unboundEntry{id: entry.ID, reason: missing})
			continue
		}
		candidates = append(candidates, sweepCandidate{id: harness.ActionID(entry.ID), params: params})
	}
	return candidates, unbound
}

// bindEntryParams binds an entry's required parameters from the World: every
// top-level requirement, and one alternative group when the entry requires a
// choice among several. It returns the arguments and an empty reason on
// success, or nil and the reason it could not bind.
func bindEntryParams(world *fixture.World, entry resources.ToolSurfaceEntry) (map[string]any, string) {
	params := map[string]any{}
	if missing := bindRequiredGroup(world, entry.RequiredParams, params); missing != "" {
		return nil, missing
	}
	if len(entry.RequiredParamsAnyOf) > 0 {
		if !bindOneAlternative(world, entry.RequiredParamsAnyOf, params) {
			return nil, "no alternative requirement group binds from the World"
		}
	}
	return params, ""
}

// bindRequiredGroup binds every parameter of one group into params, returning
// the reason it could not when a parameter is unbound or its World value
// cannot be spelled in the type the schema declares.
func bindRequiredGroup(world *fixture.World, group []resources.ToolSurfaceRequiredParam, params map[string]any) string {
	for _, param := range group {
		value, bound := world.Bind(param.Name)
		if !bound {
			return "the World has no binding for required parameter " + param.Name
		}
		spelled, ok := spellParam(value, param.Type)
		if !ok {
			return "the World's " + param.Name + " cannot be spelled as " + param.Type
		}
		params[param.Name] = spelled
	}
	return ""
}

// bindOneAlternative binds the first alternative requirement group that binds
// fully, reporting whether any did.
func bindOneAlternative(world *fixture.World, groups [][]resources.ToolSurfaceRequiredParam, params map[string]any) bool {
	for _, group := range groups {
		candidate := map[string]any{}
		if bindRequiredGroup(world, group, candidate) == "" {
			maps.Copy(params, candidate)
			return true
		}
	}
	return false
}

// spellParam spells a World value in the flat schema type a parameter
// declares, so a bound value reaches the individual surface as the type its
// raw-argument validation demands: an integer id sent to a string parameter
// is refused there, while the two dispatchers coerce.
//
// A value the schema cannot hold is refused rather than sent, so the caller
// names the binding it lacks instead of watching the call fail.
func spellParam(value any, schemaType string) (any, bool) {
	switch typed := value.(type) {
	case int64:
		if schemaAccepts(schemaType, "integer") || schemaType == "" {
			return typed, true
		}
		if schemaAccepts(schemaType, "string") {
			return strconv.FormatInt(typed, 10), true
		}
		return nil, false
	case string:
		if schemaAccepts(schemaType, "string") || schemaType == "" {
			return typed, true
		}
		return nil, false
	default:
		return value, true
	}
}

// schemaAccepts reports whether a flat schema type admits one plain type; a
// multi-typed parameter joins its alternatives with "|".
func schemaAccepts(schemaType, want string) bool {
	for part := range strings.SplitSeq(schemaType, "|") {
		if part == want {
			return true
		}
	}
	return false
}

// sweepSurfaces runs fn once per surface, as a parallel subtest with a session
// in the given mode.
//
// The subtests run in parallel because a sweep only reads or previews, and
// the World it stands on is read-only by contract, so nothing one surface does
// can be seen by another; the three-way overlap is what keeps a sweep over
// hundreds of actions to a reasonable wall clock. It does not use
// [harness.EachSurface] because that runs its subtests in sequence.
func sweepSurfaces(t *testing.T, mode harness.Mode, fn func(*harness.Env, *harness.Session)) {
	t.Helper()

	for _, surface := range harness.AllSurfaces() {
		t.Run(string(surface), func(t *testing.T) {
			t.Parallel()
			e := harness.New(t)
			fn(e, e.Session(harness.ServerConfig{Surface: surface, Mode: mode}))
		})
	}
}
