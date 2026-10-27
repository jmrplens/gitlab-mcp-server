// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// issues_test.go covers the one part of an issue create a license changes:
// how many assignees GitLab keeps, which below Premium is the first of those
// the caller names.

package ce

import (
	"slices"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// actionIssueCreate is the issue create the scenario drives.
const actionIssueCreate harness.ActionID = "issue.create"

// assigneePair is a project with two members an issue can be assigned to.
type assigneePair struct {
	project fixture.Project
	first   fixture.User
	second  fixture.User
}

// TestIssueCreate_TwoAssignees_OnTheFreeTier_KeepsOnlyTheFirst creates an issue
// assigned to two project members on every surface and holds the answer to
// the first of them (issue 1240). GitLab's API page marks assignee_ids
// Premium and Ultimate only, but every tier takes the parameter: what needs
// the license is a second assignee, which IssuableBaseService drops unless
// the issue allows several, so the schema offers it on Free and its
// description says that below Premium, on an unlicensed instance such as
// this one or a Free GitLab.com namespace, GitLab keeps the first.
func TestIssueCreate_TwoAssignees_OnTheFreeTier_KeepsOnlyTheFirst(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, buildAssigneePair, func(e *harness.Env, surface harness.Surface, pair assigneePair) {
		created := harness.Do[issues.Output](e.On(surface), actionIssueCreate, map[string]any{
			"project_id":   pair.project.IDParam(),
			"title":        e.Name("pair"),
			"assignee_ids": []int64{pair.first.ID, pair.second.ID},
		})
		got := make([]int64, 0, len(created.Assignees))
		for _, assignee := range created.Assignees {
			got = append(got, assignee.ID)
		}
		if want := []int64{pair.first.ID}; !slices.Equal(got, want) {
			e.T.Errorf("issue #%d was created with assignees %v, want only the first named, %v", created.IID, got, want)
		}
	})
}

// buildAssigneePair creates the project and its two members once on the
// parent Env, since GitLab assigns an issue only to a user who can read it.
func buildAssigneePair(e *harness.Env) assigneePair {
	e.T.Helper()

	pair := assigneePair{
		project: fixture.NewProject(e, fixture.WithNamePrefix("assignees")),
		first:   fixture.NewUser(e, "first"),
		second:  fixture.NewUser(e, "second"),
	}
	fixture.AddProjectMember(e, pair.project, pair.first, gl.DeveloperPermissions)
	fixture.AddProjectMember(e, pair.project, pair.second, gl.DeveloperPermissions)
	return pair
}
