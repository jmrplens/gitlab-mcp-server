//go:build e2e

// mcp_completions_test.go drives a completion for every argument of every
// served prompt and every variable of every served resource template.
//
// A completion is addressed by a reference and an argument, so the sweep is
// driven by the two things that carry completable arguments: the prompts and
// the resource templates the full-capability server serves. It sends an empty
// partial value and reads whatever the server offers, including nothing: the
// completion contract is that autocomplete is never blocked, so an argument
// the handler does not recognize answers with an empty list rather than an
// error, and the reference-and-argument pair is covered either way.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestCompletions_Sweep drives a completion for every prompt argument and
// every resource template variable the full-capability server serves.
//
// Replaces: the completion coverage the old suite had none of, since no
// TestMain session drove completions.
func TestCompletions_Sweep(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	specs := s.PromptSpecs()
	templates := s.ResourceTemplates()
	if len(specs) == 0 && len(templates) == 0 {
		t.Fatal("the full-capability session served no prompt or template; the completion sweep would prove nothing")
	}

	promptArgs := 0
	for _, spec := range specs {
		for _, arg := range append(append([]string{}, spec.Required...), spec.Optional...) {
			s.CompletePrompt(spec.Name, arg, "")
			promptArgs++
		}
	}

	templateVars := 0
	for _, template := range templates {
		for _, variable := range templateVariables(template) {
			s.CompleteResource(template, variable, "")
			templateVars++
		}
	}
	t.Logf("drove %d prompt-argument completions across %d prompts and %d template-variable completions across %d templates",
		promptArgs, len(specs), templateVars, len(templates))
}

// templateVariables returns the RFC 6570 variable names of a resource
// template, each stripped of the reserved "+" the file template's path
// variable carries.
func templateVariables(template string) []string {
	var names []string
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			return names
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return names
		}
		names = append(names, strings.TrimPrefix(rest[open+1:open+end], "+"))
		rest = rest[open+end+1:]
	}
}
