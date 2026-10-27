// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestPrompts_Sweep renders every served prompt whose required arguments bind
// from the World.
//
// It replaces no old test: the old suite had no prompt coverage, since no
// TestMain session registered prompts.
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

// TestPrompts_UnassignedItems_CountsAnIssueWithNoAssignee renders
// unassigned_items for a project holding one issue nobody is assigned to and
// one assigned to the token's own user, and holds the report to counting the
// first and not the second.
//
// The prompt used to ask GitLab for assignee_id=0. GitLab's assignee filter
// reads "no assignee" only from None, and takes any other value as a user id,
// so a real instance answered both lists empty and the report congratulated a
// project whose work nobody owned. A mock that ignores the parameter cannot see
// that, which is why it is held here against an instance.
func TestPrompts_UnassignedItems_CountsAnIssueWithNoAssignee(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	project := fixture.NewProject(e, fixture.WithNamePrefix("unassigned"))
	orphan := fixture.NewIssue(e, project, "Nobody owns this issue")

	me, _, err := e.Client().GL().Users.CurrentUser(gl.WithContext(e.Ctx))
	if err != nil {
		t.Fatalf("reading the token's own user: %v", err)
	}
	owned, _, err := e.Client().GL().Issues.CreateIssue(project.ID, &gl.CreateIssueOptions{
		Title:       new("Somebody owns this issue"),
		AssigneeIDs: &[]int64{me.ID},
	}, gl.WithContext(e.Ctx))
	if err != nil {
		t.Fatalf("creating the assigned issue: %v", err)
	}

	result := s.GetPrompt("unassigned_items", map[string]string{"project_id": project.IDParam()})
	if len(result.Messages) == 0 {
		t.Fatal("unassigned_items answered with no message")
	}
	content, ok := result.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("unassigned_items answered %T, want text", result.Messages[0].Content)
	}
	text := content.Text

	for _, want := range []string{"| Unassigned issues | 1 |", "## Unassigned Issues", orphan.Title} {
		t.Run("holds "+want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("the report does not hold %q:\n%s", want, text)
			}
		})
	}
	for _, unwanted := range []string{owned.Title, "All open items have assignees"} {
		t.Run("leaves out "+unwanted, func(t *testing.T) {
			if strings.Contains(text, unwanted) {
				t.Errorf("the report holds %q, which an assigned issue or an empty answer would write:\n%s", unwanted, text)
			}
		})
	}
}
