//go:build e2e

// mcp_prompts_test.go renders every prompt the server serves whose required
// arguments bind from the shared World.
//
// Prompts, like resources, are a full-capability-surface capability of the
// server rather than of a tool surface, so the sweep runs on one session. Each
// prompt's arguments come from the listing itself, split into required and
// optional; the sweep binds the required ones from the World and the optional
// ones when the World can supply them. A prompt whose required argument the
// World cannot supply is named rather than rendered, since GetPrompt would
// only fail for the missing argument and prove nothing about the prompt.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestPrompts_Sweep renders every served prompt whose required arguments bind
// from the World.
//
// Replaces: the prompt coverage the old suite had none of, since no TestMain
// session registered prompts.
func TestPrompts_Sweep(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.On(harness.SurfaceDynamic)

	specs := s.PromptSpecs()
	if len(specs) == 0 {
		t.Fatal("the full-capability session served no prompt; the sweep would prove nothing")
	}

	rendered, skipped, errs := 0, 0, 0
	for _, spec := range specs {
		args, missing := bindPromptArguments(world, spec)
		if missing != "" {
			t.Logf("prompt %s not rendered: %s", spec.Name, missing)
			skipped++
			continue
		}
		// TryGetPrompt rather than GetPrompt: a prompt whose data the World
		// does not carry (a pipeline the runnerless World never ran) answers a
		// handled error the recorder credits, which is not a reason to abort
		// the sweep over every other prompt.
		if _, err := s.TryGetPrompt(spec.Name, args); err != nil {
			t.Logf("prompt %s answered an error: %v", spec.Name, err)
			errs++
		}
		rendered++
	}
	t.Logf("rendered %d prompts (%d answered an error, %d the World cannot bind) of %d served",
		rendered, errs, skipped, len(specs))
}

// bindPromptArguments binds a prompt's required arguments from the World and
// adds every optional one the World can supply, returning the reason it could
// not when a required argument is unbound.
func bindPromptArguments(world *fixture.World, spec harness.PromptSpec) (map[string]string, string) {
	args := map[string]string{}
	for _, name := range spec.Required {
		value, bound := world.BindPromptArgument(name)
		if !bound {
			return nil, "the World has no binding for required argument " + name
		}
		args[name] = value
	}
	for _, name := range spec.Optional {
		if value, bound := world.BindPromptArgument(name); bound {
			args[name] = value
		}
	}
	return args, ""
}
