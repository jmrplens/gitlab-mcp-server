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
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

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
		// A JSON-RPC error is an acceptable refusal, but only when it is this
		// refusal. Accepting every error would accept a transport failure or
		// a child that died as proof of failing closed, which is the one
		// thing this scenario exists to demonstrate.
		if !containsAny(strings.ToLower(err.Error()), "elicit", "capability", "gitlab_issue") {
			t.Fatalf("%s failed with an error that is not a refusal to elicit: %v", interactiveIssueCreateTool, err)
		}
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

// TestElicitation_InteractiveMRCreate drives the guided merge request flow,
// the second of the four and the one with the most prompts.
//
// It needs a source branch that differs from the target, which is the one
// precondition GitLab checks before anything the flow asked for matters: a
// merge request between a branch and itself is refused with a validation
// error, and the flow would then look broken for a reason that is not the
// flow's.
func TestElicitation_InteractiveMRCreate(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("elicitmr"))
	source := fixture.NewBranch(e, project, e.Name("elicited"))

	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceIndividual,
		Elicitation:  harness.ElicitationScripted,
		Responder:    branchAwareElicitResponder(source.Name, project.DefaultBranch),
		Capabilities: harness.CapabilitiesMinimal,
		Private:      true,
	})

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      "gitlab_interactive_mr_create",
		Arguments: map[string]any{"project_id": project.IDParam()},
	})
	if err != nil {
		t.Fatalf("the guided merge request flow failed: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("the guided merge request flow answered an error: %s", rawText(result))
	}
}

// TestElicitation_InteractiveReleaseCreate drives the guided release flow.
//
// The flow's own description says the tag must already exist, so one is made
// first: a release for a tag GitLab does not have is refused, and the refusal
// would be about the tag rather than about the elicitation this covers.
func TestElicitation_InteractiveReleaseCreate(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("elicitrel"))
	tagName := e.Name("v0")

	if _, _, err := e.Client().GL().Tags.CreateTag(project.ID, &gl.CreateTagOptions{
		TagName: &tagName,
		Ref:     &project.DefaultBranch,
	}); err != nil {
		t.Fatalf("creating the tag the release flow needs: %v", err)
	}

	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceIndividual,
		Elicitation:  harness.ElicitationScripted,
		Responder:    tagAwareElicitResponder(tagName),
		Capabilities: harness.CapabilitiesMinimal,
		Private:      true,
	})

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      "gitlab_interactive_release_create",
		Arguments: map[string]any{"project_id": project.IDParam()},
	})
	if err != nil {
		t.Fatalf("the guided release flow failed: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("the guided release flow answered an error: %s", rawText(result))
	}
}

// branchAwareElicitResponder answers the merge request flow's two branch
// prompts with real branches and everything else from the schema.
//
// Answering them from the schema like the rest would offer a title-shaped
// string, and GitLab refuses a merge request whose source branch does not
// exist — a refusal about the answer rather than about the flow.
func branchAwareElicitResponder(source, target string) func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	return func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		content := scriptedElicitContent(req)
		if _, asked := content["source_branch"]; asked {
			content["source_branch"] = source
		}
		if _, asked := content["target_branch"]; asked {
			content["target_branch"] = target
		}
		return &mcp.ElicitResult{Action: "accept", Content: content}, nil
	}
}

// tagAwareElicitResponder answers the release flow's tag prompt with a tag the
// project has, for the reason above.
func tagAwareElicitResponder(tag string) func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	return func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		content := scriptedElicitContent(req)
		if _, asked := content["tag_name"]; asked {
			content["tag_name"] = tag
		}
		return &mcp.ElicitResult{Action: "accept", Content: content}, nil
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
