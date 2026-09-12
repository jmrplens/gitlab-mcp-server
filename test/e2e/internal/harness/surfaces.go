//go:build e2e

// surfaces.go runs one scenario on more than one surface.
//
// The same action reached through gitlab_execute_action, through a domain
// dispatcher and through a declared individual tool is three different code
// paths in the server: three schemas, three argument shapes, three sets of
// rewrites on the way to the handler. A scenario written once and run on all
// three is the only way the suite says anything about the other two, and the
// suite this replaces had a separate hand-written copy per surface, which is
// why its coverage of the individual surface was the deepest and its coverage
// of the default one the shallowest.
//
// Each surface runs as a subtest with an Env of its own, because an Env is one
// test's handle: its context ends with that subtest, its cleanups run when
// that subtest ends, and its names carry that subtest's name. The parent's Env
// stays the owner of anything built before the split.

package harness

import (
	"fmt"
	"slices"
	"testing"
)

// EachSurface runs fn once per surface, as a subtest named after it.
//
// The Env fn is given belongs to the subtest, so what it creates is cleaned up
// when that surface's run ends rather than at the end of the parent.
func EachSurface(e *Env, fn func(*Env, Surface)) {
	e.T.Helper()
	SurfacesWith(e, func(*Env) struct{} { return struct{}{} }, func(env *Env, surface Surface, _ struct{}) {
		fn(env, surface)
	})
}

// OnSurfaces runs fn on the named surfaces only, and says in the test log why
// the others were left out.
//
// The reason is required rather than optional: a scenario that runs on one
// surface is a hole in the other two, and the coverage report should be able
// to say whether the hole is deliberate. The usual cause is a fixture that
// cannot be multiplied, such as a seed the setup script creates exactly once.
func OnSurfaces(e *Env, reason string, surfaces []Surface, fn func(*Env, Surface)) {
	e.T.Helper()

	if reason == "" {
		e.T.Fatal("OnSurfaces needs a reason: a scenario that skips a surface has to say why")
	}
	if len(surfaces) == 0 {
		e.T.Fatal("OnSurfaces was given no surface to run on")
	}
	for _, surface := range surfaces {
		if !slices.Contains(AllSurfaces(), surface) {
			e.T.Fatalf("OnSurfaces was given an unknown surface %q", surface)
		}
	}

	if skipped := missingSurfaces(surfaces); len(skipped) > 0 {
		e.T.Logf("not run on %s: %s", joinSurfaces(skipped), reason)
	}
	runSurfaces(e, surfaces, func(env *Env, surface Surface, _ struct{}) { fn(env, surface) }, struct{}{})
}

// SurfacesWith builds one fixture on the parent's Env and runs fn on every
// surface against it.
//
// The fixture is built once, on purpose: a project, a merge request or a
// pipeline costs seconds of a GitLab's time, and three copies of one would
// treble the suite for nothing. What each surface then does to that fixture is
// its own, and anything it creates hangs off its own Env.
func SurfacesWith[F any](e *Env, build func(*Env) F, fn func(*Env, Surface, F)) {
	e.T.Helper()

	fixture := build(e)
	runSurfaces(e, AllSurfaces(), fn, fixture)
}

// runSurfaces opens one subtest per surface, each with an Env of its own.
func runSurfaces[F any](e *Env, surfaces []Surface, fn func(*Env, Surface, F), fixture F) {
	e.T.Helper()

	for _, surface := range surfaces {
		e.T.Run(string(surface), func(t *testing.T) {
			fn(newEnv(t, e.inst), surface, fixture)
		})
	}
}

// missingSurfaces returns the surfaces not in the given list.
func missingSurfaces(surfaces []Surface) []Surface {
	var missing []Surface
	for _, surface := range AllSurfaces() {
		if !slices.Contains(surfaces, surface) {
			missing = append(missing, surface)
		}
	}
	return missing
}

// joinSurfaces names a set of surfaces in a log line.
func joinSurfaces(surfaces []Surface) string {
	names := make([]string, 0, len(surfaces))
	for _, surface := range surfaces {
		names = append(names, string(surface))
	}
	return fmt.Sprintf("%v", names)
}
