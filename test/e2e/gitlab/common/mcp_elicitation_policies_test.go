//go:build e2e

// mcp_elicitation_policies_test.go covers the two elicitation policies the
// scripted scenario does not: a client that answers everything on its own, and
// a client that advertises no elicitation at all.
//
// Both are real client shapes and each proves something the scripted one
// cannot. Auto-accept is what a confirmation prompt meets in practice, and it
// was structurally broken until 2026-09-14: it answered every request with an
// empty content map, which the SDK validates against the requested schema
// before applying defaults, so every auto-accepting call failed with
// InvalidParams and no scenario existed to say so. Advertising nothing is the
// other end: the server has to fail closed rather than proceed, which is the
// behavior a destructive call without confirmation depends on.

package common

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestElicitation_AutoAccept_AnswersEveryPromptFromItsSchema drives the guided
// project flow with a client that accepts each request without being scripted.
//
// The project flow is the one that can be completed this way: every prompt it
// raises either has a value its own schema admits (a visibility enum, the
// readme boolean) or is optional. The issue flow cannot, since it asks for a
// title and an empty one is refused by GitLab, which is why the scripted
// scenario exists beside this one.
//
// What this catches is the class the fix was for: an auto-accepted answer that
// does not satisfy the requested schema never reaches the handler at all, so
// the flow fails in the client's own validation and the server looks innocent.
func TestElicitation_AutoAccept_AnswersEveryPromptFromItsSchema(t *testing.T) {
	e := harness.New(t)

	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceIndividual,
		Elicitation:  harness.ElicitationAutoAccept,
		Capabilities: harness.CapabilitiesMinimal,
		// Private, because the flow creates a project under the run's own user
		// and an auto-accepting session is not one another test should join.
		Private: true,
	})

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      "gitlab_interactive_project_create",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("the auto-accepting flow failed before it answered: %v", err)
	}
	if result == nil {
		t.Fatal("the auto-accepting flow answered nothing")
	}
	// The flow may still end in a refusal GitLab decided, a name collision
	// being the obvious one, and that is not what this covers. What it must
	// never be is a failure to answer the prompts: that is the client's own
	// schema validation refusing an empty answer, and it names the parameters.
	if result.IsError {
		text := rawText(result)
		if containsAny(text, "InvalidParams", "invalid params", "required") {
			t.Errorf("the auto-accepted answers did not satisfy the requested schema: %s", text)
		} else {
			t.Logf("the flow ran and GitLab refused the creation, which this does not cover: %s", text)
		}
	}
}

// TestElicitation_None_FailsClosedOnAFlowThatNeedsIt checks that a client
// advertising no elicitation is refused rather than served.
//
// This is the policy every ordinary session in this suite runs under, and the
// one a destructive call without an explicit confirmation relies on: the server
// has no way to ask, so it must refuse. A server that proceeded instead would
// create the object the user was never asked about, and the refusal is the only
// thing standing between the two.
func TestElicitation_None_FailsClosedOnAFlowThatNeedsIt(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("elicitnone"))

	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceIndividual,
		Elicitation:  harness.ElicitationNone,
		Capabilities: harness.CapabilitiesMinimal,
	})

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      interactiveIssueCreateTool,
		Arguments: map[string]any{"project_id": project.IDParam()},
	})
	if err != nil {
		// A protocol error is an acceptable refusal too: what matters is that
		// the flow did not run.
		return
	}
	if result == nil || !result.IsError {
		t.Fatalf("%s ran for a client that advertises no elicitation, answering %s",
			interactiveIssueCreateTool, rawText(result))
	}
	// The refusal names the alternative, so a model reading it can fall back to
	// the scripted action rather than concluding the capability is missing.
	if text := rawText(result); !containsAny(text, "elicitation", "gitlab_issue") {
		t.Errorf("the refusal is %q, want it to name elicitation or the non-interactive alternative", text)
	}
}

// containsAny reports whether text contains any of the needles.
func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
