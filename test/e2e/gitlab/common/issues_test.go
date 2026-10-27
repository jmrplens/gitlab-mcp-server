// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// issues_test.go covers an issue's create and update through the server,
// which the old suite made only to build the issue its Enterprise
// scenarios stood on: the one an epic collects, the one whose weight
// events are listed.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestIssue_CreateAndUpdate_TitleFollows creates an issue on every surface
// in a shared project, then retitles it and reads the new title off the
// answer.
//
// Replaces: TestMeta_EpicIssues, TestMeta_IssueWeightEvents
func TestIssue_CreateAndUpdate_TitleFollows(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issuecreate"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("issue")

		created := harness.Do[issues.Output](s, actionIssueCreate, withParams(params, map[string]any{"title": title, "description": "created by the e2e suite"}))
		if created.IID == 0 || created.Title != title {
			e.T.Fatalf("issue create answered %+v, want the issue %q with an iid", created, title)
		}

		updated := harness.Do[issues.Output](s, actionIssueUpdate, withParams(params, map[string]any{"issue_iid": created.IID, "title": "Updated " + title}))
		if updated.IID != created.IID || updated.Title != "Updated "+title {
			e.T.Errorf("issue update answered %+v, want issue #%d retitled", updated, created.IID)
		}
	})
}

// TestIssue_Update_EmptyValuesRemoveDescriptionDueDateAssigneesAndLabels
// creates an issue on every surface with a description, a due date, the
// runtime user as its assignee and a label, then sends each of them empty in
// one update and holds the answer and a fresh read to an issue with none of
// them. Each used to be dropped for being empty, so the update reported
// success with the issue unchanged (issue 1297).
func TestIssue_Update_EmptyValuesRemoveDescriptionDueDateAssigneesAndLabels(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issueclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[issues.Output](s, actionIssueCreate, withParams(params, map[string]any{
			"title": e.Name("issueclear"), "description": "to be removed", "due_date": "2030-01-31",
			"assignee_ids": []int64{e.Runtime().UserID}, "labels": []string{"issueclear"},
		}))
		if created.Description == "" || created.DueDate == "" || len(created.Assignees) != 1 || len(created.Labels) != 1 {
			e.T.Fatalf("issue create answered %+v, want a description, a due date, one assignee and one label to remove", created)
		}

		issueParams := withParams(params, map[string]any{"issue_iid": created.IID})
		cleared := harness.Do[issues.Output](s, actionIssueUpdate, withParams(issueParams, map[string]any{
			"description": "", "due_date": "", "assignee_ids": []int64{}, "labels": []string{},
		}))
		read := harness.Do[issues.Output](s, actionIssueGet, issueParams)
		assertCleared := func(call string, out issues.Output) {
			if out.Description != "" || out.DueDate != "" || len(out.Assignees) != 0 || len(out.Labels) != 0 {
				e.T.Errorf("issue %s after the removal answered description %q, due date %q, assignees %v, labels %v; want none",
					call, out.Description, out.DueDate, out.Assignees, out.Labels)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestIssueList_AssigneeNoneAndAny_SplitTheIssuesByAssignment creates one
// issue nobody is assigned and one assigned to the runtime user, then asks
// issue.list for assignee_id None and for any and holds each answer to the
// one issue it names. Both literals used to be refused as not an integer, and
// the 0 a caller reached for instead answered with no issue at all (issue
// 1354).
func TestIssueList_AssigneeNoneAndAny_SplitTheIssuesByAssignment(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("unassigned"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		nobody := harness.Do[issues.Output](s, actionIssueCreate, withParams(params, map[string]any{"title": e.Name("nobody")}))
		somebody := harness.Do[issues.Output](s, actionIssueCreate, withParams(params, map[string]any{
			"title": e.Name("somebody"), "assignee_ids": []int64{e.Runtime().UserID},
		}))
		cases := []struct {
			value string
			want  int64
			not   int64
		}{
			{value: "None", want: nobody.IID, not: somebody.IID},
			{value: "any", want: somebody.IID, not: nobody.IID},
		}
		for _, c := range cases {
			listed := harness.Do[issues.ListOutput](s, actionIssueList, withParams(params, map[string]any{"assignee_id": c.value, "state": "opened"}))
			iids := issueIIDs(listed.Issues)
			if !slices.Contains(iids, c.want) || slices.Contains(iids, c.not) {
				e.T.Errorf("issue list with assignee_id %s answered %v, want #%d and not #%d", c.value, iids, c.want, c.not)
			}
		}
	})
}
