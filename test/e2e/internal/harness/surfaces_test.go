//go:build e2e

// surfaces_test.go covers the three ways a scenario is run on more than one
// surface: on all of them, on a stated subset, and on all of them against one
// fixture built by the parent.

package harness

import (
	"slices"
	"strings"
	"testing"
)

// TestEachSurface_RunsOneSubtestPerSurface checks that a scenario written once
// reaches all three surfaces, each in a subtest named after it.
func TestEachSurface_RunsOneSubtestPerSurface(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)

	var ran []Surface
	EachSurface(env, func(child *Env, surface Surface) {
		ran = append(ran, surface)
		if child.T.Name() != t.Name()+"/"+string(surface) {
			t.Errorf("the subtest is named %q, want it named after the surface", child.T.Name())
		}
		if child.T == env.T {
			t.Error("the surface was given the parent's own test rather than its subtest")
		}
	})

	if !slices.Equal(ran, AllSurfaces()) {
		t.Errorf("ran on %v, want %v", ran, AllSurfaces())
	}
}

// TestEachSurface_ChildEnv_IsItsOwn checks that each surface's Env belongs to
// its own subtest, so what it creates is undone when that surface ends rather
// than at the end of the parent.
func TestEachSurface_ChildEnv_IsItsOwn(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)

	names := map[string]bool{}
	EachSurface(env, func(child *Env, _ Surface) {
		name := child.Name("project")
		if names[name] {
			t.Errorf("two surfaces were handed the same name %q", name)
		}
		names[name] = true
		if child.RunID() != env.RunID() {
			t.Errorf("the child run ID is %q, want the parent's %q", child.RunID(), env.RunID())
		}
	})

	if len(names) != len(AllSurfaces()) {
		t.Errorf("%d surfaces handed out names, want %d", len(names), len(AllSurfaces()))
	}
}

// TestOnSurfaces_NamedSurfacesOnly_AndSaysWhyTheRestAreSkipped checks the
// declared subset.
//
// The reason is required because a scenario that runs on one surface is a hole
// in the other two, and a coverage report should be able to say whether the
// hole was chosen.
func TestOnSurfaces_NamedSurfacesOnly_AndSaysWhyTheRestAreSkipped(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)

	var ran []Surface
	OnSurfaces(env, "the pending schema migration seed cannot be multiplied",
		[]Surface{SurfaceDynamic}, func(_ *Env, surface Surface) {
			ran = append(ran, surface)
		})

	if !slices.Equal(ran, []Surface{SurfaceDynamic}) {
		t.Errorf("ran on %v, want only the dynamic surface", ran)
	}
}

// TestOnSurfaces_MissingSurfaces_AreNamed checks the helper the log line is
// built from.
func TestOnSurfaces_MissingSurfaces_AreNamed(t *testing.T) {
	cases := []struct {
		name  string
		given []Surface
		want  []Surface
	}{
		{name: "one given", given: []Surface{SurfaceDynamic}, want: []Surface{SurfaceMeta, SurfaceIndividual}},
		{name: "two given", given: []Surface{SurfaceDynamic, SurfaceMeta}, want: []Surface{SurfaceIndividual}},
		{name: "all given", given: AllSurfaces(), want: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := missingSurfaces(testCase.given); !slices.Equal(got, testCase.want) {
				t.Errorf("missingSurfaces(%v) = %v, want %v", testCase.given, got, testCase.want)
			}
		})
	}
}

// TestJoinSurfaces_NamesThemForALogLine checks the wording of the line that
// records a deliberate hole.
func TestJoinSurfaces_NamesThemForALogLine(t *testing.T) {
	joined := joinSurfaces([]Surface{SurfaceMeta, SurfaceIndividual})

	for _, want := range []string{"meta", "individual"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(joined, want) {
				t.Errorf("joinSurfaces() = %q, want it to name %q", joined, want)
			}
		})
	}
}

// TestSurfacesWith_BuildsTheFixtureOnceForAllThree checks that one parent
// fixture serves the three subtests.
//
// Building it once is the point: a project, a merge request or a pipeline
// costs seconds of a GitLab's time, and three copies of one would treble the
// suite for nothing.
func TestSurfacesWith_BuildsTheFixtureOnceForAllThree(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)

	built := 0
	var seen []string
	SurfacesWith(env, func(parent *Env) string {
		built++
		return parent.Name("fixture")
	}, func(_ *Env, _ Surface, fixture string) {
		seen = append(seen, fixture)
	})

	if built != 1 {
		t.Errorf("the fixture was built %d times, want once", built)
	}
	if len(seen) != len(AllSurfaces()) {
		t.Fatalf("%d surfaces saw the fixture, want %d", len(seen), len(AllSurfaces()))
	}
	for _, fixture := range seen {
		if fixture != seen[0] {
			t.Errorf("the surfaces were given different fixtures: %v", seen)
			break
		}
	}
}
