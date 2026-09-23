//go:build e2e

// mcp_completions_test.go drives a completion for every argument of every
// served prompt and every variable of every served resource template, on both
// capability surfaces.
//
// A completion is addressed by a reference and an argument, so the sweep is
// driven by the two things that carry completable arguments: the prompts and
// the resource templates a session serves. It sends an empty partial value and
// reads whatever the server offers, including nothing: the completion contract
// is that autocomplete is never blocked, so an argument the handler does not
// recognize answers with an empty list rather than an error, and the
// reference-and-argument pair is covered either way.
//
// Both capability surfaces, because that is the one coordinate the set varies
// along: the full surface serves every prompt and template, and the minimal
// one serves no prompt and one template, the tool manifest's detail. The
// session line lists the same references as the denominator, spelled by the
// harness from the same listings, so a completion no test asked for is absent
// in the coverage report rather than missing from it.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestCompletions_Sweep drives a completion for every prompt argument and
// every resource template variable each capability surface serves.
//
// It replaces no old test: the old suite had no completion coverage, since
// no TestMain session drove completions.
func TestCompletions_Sweep(t *testing.T) {
	for _, capabilities := range []harness.CapabilitySurface{harness.CapabilitiesFull, harness.CapabilitiesMinimal} {
		t.Run(string(capabilities), func(t *testing.T) {
			e := harness.New(t)
			s := e.Session(harness.ServerConfig{Surface: harness.SurfaceDynamic, Capabilities: capabilities})

			specs := s.PromptSpecs()
			templates := s.ResourceTemplates()
			if len(specs) == 0 && len(templates) == 0 {
				t.Fatalf("the %s session served no prompt or template; the completion sweep would prove nothing", capabilities)
			}

			// Each completion says it is a sweep's, since the values it
			// answers are read by nothing, and the coverage command credits
			// it as sweep-only rather than asserted.
			sweep := harness.For(harness.PurposeSweep)
			promptArgs := 0
			for _, spec := range specs {
				for _, arg := range append(append([]string{}, spec.Required...), spec.Optional...) {
					s.CompletePrompt(spec.Name, arg, "", sweep)
					promptArgs++
				}
			}

			templateVars := 0
			for _, template := range templates {
				for _, variable := range harness.TemplateVariables(template) {
					s.CompleteResource(template, variable, "", sweep)
					templateVars++
				}
			}
			t.Logf("drove %d prompt-argument completions across %d prompts and %d template-variable completions across %d templates on %s",
				promptArgs, len(specs), templateVars, len(templates), capabilities)
		})
	}
}
