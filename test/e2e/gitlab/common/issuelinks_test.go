//go:build e2e

// issuelinks_test.go covers the links between two issues through the
// server: link a source issue to a target, find the link in the source's
// listing, read it by its ID, remove it and check the listing is empty
// again. The source is shared; each surface links it to a target of its
// own, so a link one surface could not remove blocks nobody else.

package common

import (
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// issueLinkType is the relation the links here are created with. It is
// spelled out on every call because the individual tool's schema publishes
// link_type as a required property, even though its own description names
// relates_to as the default and GitLab applies that default itself: a call
// that omits it is refused by the SDK before the handler runs.
const issueLinkType = "relates_to"

// linkIDs lists the link IDs of a relations listing.
func linkIDs(relations []issuelinks.RelationOutput) []int64 {
	ids := make([]int64, 0, len(relations))
	for _, relation := range relations {
		ids = append(ids, int64(relation.IssueLinkID))
	}
	return ids
}

// TestIssueLinks_Lifecycle_CreateListGetDelete links the fixture issue to
// a fresh target on every surface, finds the target among the source's
// relations, reads the link back, deletes it and checks the relation is
// gone. The target is named by its iid spelled as a string, which is how
// the schema declares it: the parameter also accepts a path.
//
// Replaces: TestIndividual_IssueLinks, TestMeta_IssueLinks
func TestIssueLinks_Lifecycle_CreateListGetDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		return newIssueFixture(e, "issuelinks")
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		s := e.On(surface)
		params := f.params()
		target := fixture.NewIssue(e, f.project, "link target for the "+string(surface)+" surface")

		created := harness.Do[issuelinks.Output](s, actionIssueLinkCreate, withParams(params, map[string]any{
			"target_project_id": f.project.IDParam(), "target_issue_iid": strconv.FormatInt(target.IID, 10),
			"link_type": issueLinkType,
		}))
		if created.ID == 0 || created.LinkType != issueLinkType || created.TargetIssue == nil || created.TargetIssue.IID != target.IID {
			e.T.Fatalf("link_create answered %+v, want a %s link with an ID to issue #%d", created, issueLinkType, target.IID)
		}
		linkParams := withParams(params, map[string]any{"issue_link_id": created.ID})

		listed := harness.Do[issuelinks.ListOutput](s, actionIssueLinkList, params)
		if !containsID(linkIDs(listed.Relations), int64(created.ID)) {
			e.T.Errorf("the source's relations do not hold link %d: %v", created.ID, linkIDs(listed.Relations))
		}

		got := harness.Do[issuelinks.Output](s, actionIssueLinkGet, linkParams)
		if got.ID != created.ID || got.LinkType != issueLinkType || got.TargetIssue == nil || got.TargetIssue.IID != target.IID {
			e.T.Errorf("link_get answered %+v, want the %s link %d to issue #%d", got, issueLinkType, created.ID, target.IID)
		}

		harness.DoVoid(s, actionIssueLinkDelete, linkParams)
		remaining := harness.Do[issuelinks.ListOutput](s, actionIssueLinkList, params)
		if containsID(linkIDs(remaining.Relations), int64(created.ID)) {
			e.T.Errorf("link %d is still listed after its delete", created.ID)
		}
	})
}
