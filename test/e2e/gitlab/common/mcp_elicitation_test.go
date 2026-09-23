//go:build e2e

// mcp_elicitation_test.go covers the MCP elicitation capability end to end:
// the server drives each of its four guided creation flows by asking the
// client for one field at a time, a scripted client answers, and the object
// GitLab creates carries the answers.
//
// Every flow runs on all three surfaces through the ordinary verbs. The flows
// are standalone utilities, tools of their own on meta and individual and
// actions of the execute tool on dynamic, and the projection spells each one
// the way its surface registers it, so the coverage record credits the calls
// to the flows they ran. Until issue 903 the harness could not name them at
// all, and the four flows were driven through Raw on the individual surface
// alone, which the record credits to nothing: the capability was exercised by
// all four and read as absent.
//
// One test per flow rather than one for all four, because the record decides
// that a flow elicited from the test that made the call, and a test driving
// four flows could not tell it which of them had. Each surface opens a session
// of its own, scripted and therefore private: the script belongs to the test
// that wrote it, and the session ends with that test.

package common

import (
	"context"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// What the scripts answer the free-text prompts every flow shares with, so the
// object GitLab created can be held to them.
const (
	elicitedDescription = "Created by the e2e elicitation script"
	elicitedLabel       = "elicited"
)

// elicitScript is one client's answers to one flow, by the property each prompt
// asks for, and the record of what was asked, in order.
//
// A property it was given no answer for is declined and still recorded, so a
// prompt the flow was not expected to make stops the flow and the assertion on
// the sequence names it. It never touches testing.T, because the SDK calls it
// on a goroutine of its own.
type elicitScript struct {
	mu      sync.Mutex
	answers map[string]any
	asked   []string
}

// newElicitScript returns a script that answers with the given values.
func newElicitScript(answers map[string]any) *elicitScript {
	return &elicitScript{answers: answers}
}

// respond answers one elicitation request: every property it asks for from
// the script, or a decline when the script has no answer for one of them.
func (s *elicitScript) respond(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	properties := requestedProperties(req)

	s.mu.Lock()
	defer s.mu.Unlock()
	content := make(map[string]any, len(properties))
	for _, property := range properties {
		s.asked = append(s.asked, property)
		value, scripted := s.answers[property]
		if !scripted {
			return &mcp.ElicitResult{Action: "decline"}, nil
		}
		content[property] = value
	}
	return &mcp.ElicitResult{Action: "accept", Content: content}, nil
}

// askedProperties returns what the flow asked for, in the order it asked.
func (s *elicitScript) askedProperties() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.asked)
}

// requestedProperties returns the property names one request's schema asks
// for, sorted. Every prompt these flows make asks for one.
func requestedProperties(req *mcp.ElicitRequest) []string {
	if req == nil || req.Params == nil {
		return nil
	}
	schema, _ := req.Params.RequestedSchema.(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	return slices.Sorted(maps.Keys(properties))
}

// scriptedSession opens this test's session on one surface, answering
// elicitations from the script.
func scriptedSession(e *harness.Env, surface harness.Surface, script *elicitScript) *harness.Session {
	e.T.Helper()
	return e.Session(harness.ServerConfig{
		Surface:      surface,
		Elicitation:  harness.ElicitationScripted,
		Responder:    script.respond,
		Capabilities: harness.CapabilitiesMinimal,
	})
}

// assertAsked checks that the flow asked for exactly these properties, in this
// order. A yes-or-no prompt asks for "confirmed" whatever the question, so a
// flow's optional switches and its final confirmation all read that way.
func assertAsked(e *harness.Env, script *elicitScript, want ...string) {
	e.T.Helper()
	if got := script.askedProperties(); !slices.Equal(got, want) {
		e.T.Errorf("the flow asked for %v, want %v in that order", got, want)
	}
}

// TestElicitation_IssueCreate_TheAnswersReachTheIssue drives the guided issue
// flow and holds the issue GitLab created to every answer the script gave.
//
// Replaces: TestElicitation
func TestElicitation_IssueCreate_TheAnswersReachTheIssue(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("elicit"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		title := e.Name("elicited-issue")
		script := newElicitScript(map[string]any{
			"title":       title,
			"description": elicitedDescription,
			"labels":      elicitedLabel,
			"confirmed":   true,
		})
		s := scriptedSession(e, surface, script)

		issue := harness.Do[issues.Output](s, actionInteractiveIssueCreate, map[string]any{"project_id": project.IDParam()})

		// Confidentiality, then the final confirmation.
		assertAsked(e, script, "title", "description", "labels", "confirmed", "confirmed")
		if issue.IID == 0 || issue.Title != title || issue.Description != elicitedDescription {
			e.T.Errorf("the flow created issue %d titled %q with description %q, want the elicited %q and %q",
				issue.IID, issue.Title, issue.Description, title, elicitedDescription)
		}
		if !slices.Contains(issue.Labels, elicitedLabel) {
			e.T.Errorf("the issue carries labels %v, want the elicited %q", issue.Labels, elicitedLabel)
		}
		if !issue.Confidential {
			e.T.Error("the issue is not confidential, and the script answered yes to making it so")
		}
	})
}

// TestElicitation_MergeRequestCreate_TheAnswersReachTheMergeRequest drives the
// guided merge request flow, the one with the most prompts.
//
// Only the branches and the title are held to the answers: whether GitLab
// keeps the squash and remove-source switches a caller asked for depends on the
// project's own merge settings, so asserting them would test those.
func TestElicitation_MergeRequestCreate_TheAnswersReachTheMergeRequest(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("elicitmr"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		// A source branch per surface: the project is shared, and GitLab
		// refuses a second open merge request between one pair of branches.
		source := fixture.NewBranch(e, project, e.Name("elicited"))
		title := e.Name("elicited-mr")
		script := newElicitScript(map[string]any{
			"source_branch": source.Name,
			"target_branch": project.DefaultBranch,
			"title":         title,
			"description":   elicitedDescription,
			"labels":        elicitedLabel,
			"confirmed":     true,
		})
		s := scriptedSession(e, surface, script)

		mr := harness.Do[mergerequests.Output](s, actionInteractiveMRCreate, map[string]any{"project_id": project.IDParam()})

		// Remove the source branch, squash, then the final confirmation.
		assertAsked(e, script, "source_branch", "target_branch", "title", "description", "labels",
			"confirmed", "confirmed", "confirmed")
		if mr.IID == 0 || mr.SourceBranch != source.Name || mr.TargetBranch != project.DefaultBranch || mr.Title != title {
			e.T.Errorf("the flow created merge request %d %s into %s titled %q, want %s into %s titled %q",
				mr.IID, mr.SourceBranch, mr.TargetBranch, mr.Title, source.Name, project.DefaultBranch, title)
		}
	})
}

// TestElicitation_ReleaseCreate_TheAnswersReachTheRelease drives the guided
// release flow.
func TestElicitation_ReleaseCreate_TheAnswersReachTheRelease(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("elicitrel"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		// The flow asks for a tag that already exists, so one is made per
		// surface. Nothing deletes it on its own: the release stands on it,
		// and both go with the project, as the release builder's tags do.
		tagName := e.Name("v0")
		if _, _, err := e.Client().GL().Tags.CreateTag(project.ID, &gl.CreateTagOptions{
			TagName: new(tagName),
			Ref:     new(project.DefaultBranch),
		}, gl.WithContext(e.Ctx)); err != nil {
			e.T.Fatalf("creating the tag the release flow asks for: %v", err)
		}
		name := e.Name("elicited-release")
		script := newElicitScript(map[string]any{
			"tag_name":    tagName,
			"name":        name,
			"description": elicitedDescription,
			"confirmed":   true,
		})
		s := scriptedSession(e, surface, script)

		release := harness.Do[releases.Output](s, actionInteractiveReleaseCreate, map[string]any{"project_id": project.IDParam()})

		assertAsked(e, script, "tag_name", "name", "description", "confirmed")
		if release.TagName != tagName || release.Name != name || release.Description != elicitedDescription {
			e.T.Errorf("the flow created release %q named %q with description %q, want %q, %q and %q",
				release.TagName, release.Name, release.Description, tagName, name, elicitedDescription)
		}
	})
}

// TestElicitation_ProjectCreate_TheAnswersReachTheProject drives the guided
// project flow, the one that needs nothing from the caller at all.
func TestElicitation_ProjectCreate_TheAnswersReachTheProject(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		// Named through the run, so the run's sweep finds the project even if
		// the test dies between the create and the cleanup registered below.
		name := e.Name("elicited")
		script := newElicitScript(map[string]any{
			"name":           name,
			"description":    elicitedDescription,
			"selection":      "private",
			"confirmed":      true,
			"default_branch": fixture.DefaultBranch,
		})
		s := scriptedSession(e, surface, script)

		created := harness.Do[projects.Output](s, actionInteractiveProjectCreate, nil)
		if created.ID == 0 {
			e.T.Fatalf("the flow answered no project: %+v", created)
		}
		e.Defer("project "+created.PathWithNamespace, func(ctx context.Context) error {
			return fixture.DeleteProject(ctx, e.Client(), created.ID, created.PathWithNamespace)
		})

		// The README question, then the default branch, then the final
		// confirmation.
		assertAsked(e, script, "name", "description", "selection", "confirmed", "default_branch", "confirmed")
		if created.Name != name || created.Visibility != "private" || created.DefaultBranch != fixture.DefaultBranch {
			e.T.Errorf("the flow created %q, %s, on %q, want %q, private, on %q",
				created.Name, created.Visibility, created.DefaultBranch, name, fixture.DefaultBranch)
		}
	})
}
