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
// What this covers is the class the fix was for: an auto-accepted answer that
// does not satisfy the requested schema never reaches the handler at all, so
// the flow dies inside the client's own validation and the server looks
// innocent. The proof that it does not happen is that the call reaches GitLab,
// whatever GitLab then makes of it.
//
// It deliberately does NOT assert that the flow completes. An earlier version
// of this comment claimed the project flow is the one that can be finished
// this way, because every prompt has a value its own schema admits. That is
// false and the suite found it: `name` is a required string, a policy that
// answers from the schema alone has nothing to put there, and GitLab refuses
// the empty one. No client that accepts without being asked can invent a
// project name, so this is a fact about auto-accept rather than a defect, and
// the scripted scenario beside this one is what covers a completed flow.
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
	// A refusal GitLab decided is the expected ending here and is not what
	// this covers. The two are told apart by where the message comes from
	// rather than by a word in it: an answer that reached the API names the
	// request, and the previous version of this check keyed on "required",
	// which the server's own hint carries ("all required fields are valid"),
	// so a perfectly good run was reported as a schema violation.
	if !result.IsError {
		return
	}
	text := rawText(result)
	switch {
	case containsAny(text, "/api/v4/", "400", "bad request"):
		t.Logf("the answers satisfied every requested schema and GitLab refused the creation, "+
			"which is where an unscripted client ends on a flow that needs a name: %s", firstLine(text))
	case containsAny(text, "InvalidParams", "invalid params", `validating "content"`, "jsonschema"):
		t.Errorf("the auto-accepted answers did not satisfy the requested schema, so no call was made: %s", text)
	default:
		t.Errorf("the flow failed for a reason this scenario cannot classify: %s", text)
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
//
// It runs on the default session of each surface, which is exactly such a
// client: every ordinary session of this suite advertises no elicitation. The
// refusal is a tool result on all three, the dispatcher the dynamic surface
// enters and the tool of its own the other two register answering alike, and
// it must say why, so a model reading it can fall back to the non-interactive
// action rather than concluding the capability is missing.
func TestElicitation_None_FailsClosedOnAFlowThatNeedsIt(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("elicitnone"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		harness.ExpectToolError(e.On(surface), actionInteractiveIssueCreate,
			map[string]any{"project_id": project.IDParam()}, "elicitation")
	})
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
