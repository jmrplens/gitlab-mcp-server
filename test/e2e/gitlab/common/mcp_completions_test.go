// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// mcp_completions_test.go drives a completion for every argument of every
// served prompt and every variable of every served resource template, on both
// capability surfaces, and holds the answers of the completers whose matching
// GitLab decides: a project typed back by its full path or by a name of two
// characters, an IID typed in a project, and a SHA typed on the commit
// template.
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
	"slices"
	"strconv"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// completionPrompt is a served prompt that takes project_id, the argument
// these scenarios complete. Which prompt does not matter to the completer,
// which dispatches on the argument's name; it has to be one the server serves,
// or the reference is refused.
const completionPrompt = "project_health_check"

// TestCompletions_ProjectID_AShortNameTypedWholeCompletes names a project
// with two characters, types that name whole, and holds the answer to
// offering the project.
//
// GitLab matches a search term in part only when it has three characters or
// more and compares a shorter query with each name whole. With
// search_namespaces it compares the query with the full path and full name,
// which a bare name never equals, so the completion sends such a query
// without the flag; sending the flag with every query lost every project
// whose name is that short.
func TestCompletions_ProjectID_AShortNameTypedWholeCompletes(t *testing.T) {
	const shortName = "z7"
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)
	// A group of its own, so no other project there can hold the name.
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("complete"))
	project := fixture.NewProject(e, fixture.InGroup(group), fixture.WithNamePrefix("complete"))
	if _, _, err := e.Client().GL().Projects.EditProject(project.ID, &gl.EditProjectOptions{Name: new(shortName)}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("renaming the project to %s: %v", shortName, err)
	}

	values := s.CompletePrompt(completionPrompt, "project_id", shortName)
	if !slices.Contains(values, project.Path) {
		e.T.Errorf("project_id completion of %q offered %v, want it to hold %s", shortName, values, project.Path)
	}
}

// TestCompletions_ProjectID_AFullPathCompletesThroughTheNamespace types back
// the full path the project_id completion offers, whole and cut inside the
// namespace, and holds the answer to offering that project.
//
// GitLab's project search matches a project's path, name and description
// unless the request asks for search_namespaces, so the value this completion
// offers could not be typed back: a full path matched nothing on a real
// instance.
func TestCompletions_ProjectID_AFullPathCompletesThroughTheNamespace(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)
	project := fixture.NewProject(e, fixture.WithNamePrefix("complete"))
	// The second spelling starts two characters inside the namespace, so it
	// can only match through the route, and carries the project's own name,
	// so the namespace's other projects cannot crowd it off the page.
	slash := strings.Index(project.Path, "/")
	acrossTheSlash := project.Path[max(slash-2, 0):]

	for _, typed := range []string{project.Path, acrossTheSlash} {
		t.Run(typed, func(t *testing.T) {
			values := s.CompletePrompt(completionPrompt, "project_id", typed)
			if !slices.Contains(values, project.Path) {
				t.Errorf("project_id completion of %q offered %v, want it to hold %s", typed, values, project.Path)
			}
		})
	}
}

// issueTemplate is the resource template whose issue_iid the IID scenario
// completes: no served prompt declares that argument.
const issueTemplate = "gitlab://project/{project_id}/issue/{issue_iid}"

// TestCompletions_IssueIID_AClosedIssueCompletesByItsIID closes an issue,
// types its IID with the project resolved, and holds the answer to offering
// it.
//
// The completer used to read the 20 newest open issues and keep the IIDs that
// began with what was typed, so a closed issue was never offered, nor an open
// one older than the 20 newest.
func TestCompletions_IssueIID_AClosedIssueCompletesByItsIID(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)
	project := fixture.NewProject(e, fixture.WithNamePrefix("complete"))
	issue := fixture.NewIssue(e, project, e.Name("closed"))
	closed := harness.Do[issues.Output](s, actionIssueUpdate, map[string]any{
		"project_id": project.IDParam(), "issue_iid": issue.IID, "state_event": "close",
	})
	if closed.State != "closed" {
		t.Fatalf("the update answered state %q, want the issue closed", closed.State)
	}

	iid := strconv.FormatInt(issue.IID, 10)
	values := s.CompleteResource(issueTemplate, "issue_iid", iid, harness.Resolved(map[string]string{"project_id": project.IDParam()}))
	if !slices.Contains(values, iid) {
		t.Errorf("issue_iid completion of %q in project %s offered %v, want it to hold the closed issue", iid, project.IDParam(), values)
	}
}

// commitTemplate is the resource template whose sha the SHA scenario
// completes: no served prompt declares that argument either.
const commitTemplate = "gitlab://project/{project_id}/commit/{sha}"

// TestCompletions_SHA_ACommitTypedWholeCompletesOnItsTemplate commits a file
// on a branch of its own, types the commit's whole SHA on the commit template
// with the project resolved, and holds the answer to offering its short SHA.
//
// The template's sha used to be answered with nothing, since only four
// template variables reached a completer, and the completer read only the 20
// newest commits of the default branch, which a commit on another branch is
// never among.
func TestCompletions_SHA_ACommitTypedWholeCompletesOnItsTemplate(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)
	project := fixture.NewProject(e, fixture.WithNamePrefix("complete"))
	branch := fixture.NewBranch(e, project, e.Name("complete"))
	commit := fixture.CommitFile(e, project, branch.Name, "COMPLETE.md", "complete\n", "docs: add the completion fixture")

	values := s.CompleteResource(commitTemplate, "sha", commit.SHA, harness.Resolved(map[string]string{"project_id": project.IDParam()}))
	if !slices.Contains(values, commit.ShortID) {
		t.Errorf("sha completion of %s on branch %s offered %v, want it to hold %s", commit.SHA, branch.Name, values, commit.ShortID)
	}
}

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
