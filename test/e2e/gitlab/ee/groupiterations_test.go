// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// groupiterations_test.go covers the answers of the group iteration listing
// that issues_test.go does not: a group that does not exist is refused as not
// found, with the hint naming the read that finds one, and the ordering and
// keyset arguments its route never reads are refused before any request.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupiterations"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingGroupID is the group number GitLab never assigns.
const missingGroupID = "0"

// TestGroupIterations_MissingGroup_IsRefusedAsNotFound lists the
// iterations of group 0 on every surface and checks the refusal names
// the group read to verify the id with.
//
// Replaces: TestMeta_GroupIterations
func TestGroupIterations_MissingGroup_IsRefusedAsNotFound(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		refused := harness.Refused(s, actionIterationListGroup, map[string]any{"group_id": missingGroupID, "state": "opened"}, harness.FailureNotFound)
		assertMentions(e, "the iteration listing of a missing group", refused, "group.get", "Premium")
	})
}

// groupIterationUnreadArguments are the four list parameters the group
// iteration listing does not take (issues 1165 and 1600), each with a value
// it could be sent with. GET /groups/:id/iterations declares none of them,
// hands IterationsFinder no sort, so the iterations come by due date, title
// and id whatever is sent, and pages by offset.
var groupIterationUnreadArguments = []struct {
	name  string
	value any
}{
	{name: "order_by", value: "title"},
	{name: "sort", value: "asc"},
	{name: "pagination", value: "keyset"},
	{name: "page_token", value: "1"},
}

// TestGroupIterations_ArgumentsTheRouteIgnores_AreRefusedBeforeAnyRequest
// sends each of [groupIterationUnreadArguments] to the iteration listing of a
// group created for the scenario and holds the answer to a refusal naming the
// argument, made before any request: the individual tool's schema admits no
// property it does not list, execute lists the params the action does not
// know, and the meta dispatcher names the field it does not know. Asked by
// page number, the listing still holds the group's iteration.
func TestGroupIterations_ArgumentsTheRouteIgnores_AreRefusedBeforeAnyRequest(t *testing.T) {
	e := harness.New(t)

	type world struct {
		group     fixture.Group
		iteration fixture.Iteration
	}
	harness.SurfacesWith(e, func(e *harness.Env) world {
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("iterargs"))
		return world{group: group, iteration: fixture.NewIteration(e, group)}
	}, func(e *harness.Env, surface harness.Surface, w world) {
		s := e.On(surface)
		params := map[string]any{"group_id": w.group.IDParam(), "state": "all"}

		for _, argument := range groupIterationUnreadArguments {
			call := withParams(params, map[string]any{argument.name: argument.value})
			switch surface {
			case harness.SurfaceIndividual:
				refusal := harness.Refused(s, actionIterationListGroup, call, harness.FailureInvalidParams)
				assertMentions(e, "the refusal of "+argument.name, refusal, argument.name)
			case harness.SurfaceDynamic:
				harness.ExpectToolError(s, actionIterationListGroup, call, "Unknown params: "+argument.name+".")
			default:
				harness.ExpectToolError(s, actionIterationListGroup, call, `unknown field "`+argument.name+`"`)
			}
		}

		listed := harness.Do[groupiterations.ListOutput](s, actionIterationListGroup, withParams(params, map[string]any{"page": 1, "per_page": 100}))
		if !iterationListed(listed.Iterations, w.iteration.IID) {
			e.T.Errorf("the group's iterations by page do not hold %s: %+v", w.iteration, listed.Iterations)
		}
	})
}
