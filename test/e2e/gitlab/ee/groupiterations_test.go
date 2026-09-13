//go:build e2e

// groupiterations_test.go covers the one answer of the group iteration
// listing that issues_test.go does not: a group that does not exist is
// refused as not found, with the hint naming the read that finds one.

package ee

import (
	"testing"

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
		assertMentions(e, "the iteration listing of a missing group", refused, "gitlab_group_get", "Premium")
	})
}
