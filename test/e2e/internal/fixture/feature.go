//go:build e2e

// feature.go pins an instance-global feature flag for the length of one test,
// so a scenario whose subject GitLab serves behind a flag can run at all, and
// puts the instance back the way it was found.
//
// It goes through client-go like every other builder here, and that matters
// more than usual in this one: the flag is the scenario's precondition rather
// than its subject, so a fixture driving admin.feature_set would file calls in
// the run record that no assertion is about. The common suite's own flag
// scenario is the other way round and drives those actions on purpose.
//
// A flag is instance-global, so a test that pins one declares
// harness.LockInstanceGlobal and harness.NeedAdmin: the write is visible to
// every other test running at the same moment, and only an administrator may
// make it.

package fixture

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// FeatureState is how the instance held one feature flag before a test
// touched it.
//
// The three fields are three different states, and two of them read alike in
// a listing. A flag the instance lists nowhere is at its default, so deleting
// it puts it back; a flag listed with a boolean gate is set instance-wide, so
// that value puts it back; and a flag listed with only percentage or actor
// gates is a rollout in progress, which neither a delete nor a set restores.
type FeatureState struct {
	// Listed says the instance carried a record for the flag at all.
	Listed bool
	// Boolean says that record carried an instance-global boolean gate.
	Boolean bool
	// Value is that gate.
	Value bool
}

// On reports whether the instance holds the flag on for everybody, which is
// the only state in which a flagged endpoint answers every caller.
func (f FeatureState) On() bool { return f.Listed && f.Boolean && f.Value }

// Restorable reports whether a set or a delete puts this state back, which is
// false exactly for a rollout gated by percentage or by actor.
func (f FeatureState) Restorable() bool { return !f.Listed || f.Boolean }

// ReadFeature reads how the instance holds one feature flag right now.
func ReadFeature(e *harness.Env, name string) FeatureState {
	e.T.Helper()

	listed, _, err := e.Client().GL().Features.ListFeatures(gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("listing the instance feature flags: %v", err)
	}
	return featureStateOf(listed, name)
}

// FeatureDefined reports whether this GitLab declares the flag at all.
//
// It is the honest form of "is the endpoint behind this flag present in this
// release": a flag the instance defines nowhere belongs to a version this one
// predates, and no value set for it would make the endpoint appear. Asking the
// instance beats comparing version numbers, which says what release introduced
// the flag and not what this instance was built with.
func FeatureDefined(e *harness.Env, name string) bool {
	e.T.Helper()

	defined, _, err := e.Client().GL().Features.ListFeatureDefinitions(gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("listing the instance feature definitions: %v", err)
	}
	for _, definition := range defined {
		if definition != nil && definition.Name == name {
			return true
		}
	}
	return false
}

// PinFeature sets one instance-global feature flag for the rest of the test
// and registers the restore, returning the state it found.
//
// It refuses a rollout it cannot put back rather than guessing, since the only
// guess available (the delete) would drop a rollout the instance's own
// operator configured.
func PinFeature(e *harness.Env, name string, value bool) FeatureState {
	e.T.Helper()

	before := ReadFeature(e, name)
	if !before.Restorable() {
		e.T.Fatalf("feature flag %s is rolled out by percentage or actor, and this fixture can put back only a "+
			"boolean gate; a test has to leave such a flag alone", name)
	}

	e.Defer("feature flag "+name, func(ctx context.Context) error { return restoreFeature(ctx, e, name, before) })

	if _, _, err := e.Client().GL().Features.SetFeatureFlag(name, &gl.SetFeatureFlagOptions{Value: value},
		gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("setting feature flag %s to %t: %v", name, value, err)
	}
	return before
}

// restoreFeature puts one flag back where the test found it: the boolean gate
// it carried, or the delete that returns it to its default when the instance
// listed it nowhere.
func restoreFeature(ctx context.Context, e *harness.Env, name string, before FeatureState) error {
	if !before.Listed {
		if _, err := e.Client().GL().Features.DeleteFeatureFlag(name, gl.WithContext(ctx)); err != nil {
			return fmt.Errorf("deleting feature flag %s: %w", name, err)
		}
		return nil
	}
	if _, _, err := e.Client().GL().Features.SetFeatureFlag(name, &gl.SetFeatureFlagOptions{Value: before.Value},
		gl.WithContext(ctx)); err != nil {
		return fmt.Errorf("restoring feature flag %s to %t: %w", name, before.Value, err)
	}
	return nil
}

// featureStateOf reads how the instance held one flag out of a listing.
func featureStateOf(listed []*gl.Feature, name string) FeatureState {
	for _, flag := range listed {
		if flag == nil || flag.Name != name {
			continue
		}
		state := FeatureState{Listed: true}
		for _, gate := range flag.Gates {
			if gate.Key != "boolean" {
				continue
			}
			state.Value, state.Boolean = gate.Value.(bool)
			break
		}
		return state
	}
	return FeatureState{}
}
