//go:build e2e

// auditevents_test.go covers the audit event reads at their three scopes:
// a project's, a group's and the instance's, each listed and then read back
// one event at a time.
//
// The fixture causes the events it then asks for, by editing a group and a
// project it owns through client-go, so a listing that comes back empty is
// a listing that missed something rather than a scope nothing happened in.
// GitLab writes audit events asynchronously, which is why the first read of
// each scope waits for them.

package ee

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/auditevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The audit event waits: GitLab indexes an event a moment after the change
// that caused it, and a loaded Docker instance takes longer.
const (
	auditEventInterval = 2 * time.Second
	auditEventWait     = 90 * time.Second
)

// auditFixture is a group and a project whose descriptions were changed,
// so each carries at least one audit event of its own.
type auditFixture struct {
	group   fixture.Group
	project fixture.Project
}

// buildAuditFixture creates the two objects and edits each once through
// client-go, which is the mutation GitLab records.
func buildAuditFixture(e *harness.Env) auditFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("audit"))
	project := fixture.NewProject(e, fixture.WithNamePrefix("audit"))

	if _, _, err := e.Client().GL().Groups.UpdateGroup(group.ID, &gl.UpdateGroupOptions{
		Description: new("e2e audit event fixture"),
	}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("editing group %d to cause an audit event: %v", group.ID, err)
	}
	if _, _, err := e.Client().GL().Projects.EditProject(project.ID, &gl.EditProjectOptions{
		Description: new("e2e audit event fixture"),
	}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("editing project %d to cause an audit event: %v", project.ID, err)
	}
	return auditFixture{group: group, project: project}
}

// hasAuditEvents is the condition each listing waits for.
func hasAuditEvents(out auditevents.ListOutput) bool { return len(out.AuditEvents) > 0 }

// TestAuditEvents_EachScope_ListsAndGetsWhatTheFixtureCaused lists the
// audit events of the fixture's project, its group and the instance on
// every surface, waiting for GitLab to index them, and reads the first of
// each back by ID. The instance scope is the administrator's, which the
// run's token is.
//
// Replaces: TestMeta_AuditEvents, TestMeta_AuditEventListActions
func TestAuditEvents_EachScope_ListsAndGetsWhatTheFixtureCaused(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, buildAuditFixture, func(e *harness.Env, surface harness.Surface, f auditFixture) {
		s := e.On(surface)

		projectEvents := harness.Eventually(s, actionAuditEventListProject,
			map[string]any{"project_id": f.project.IDParam(), "per_page": 20}, auditEventInterval, auditEventWait, hasAuditEvents)
		projectEvent := harness.Do[auditevents.Output](s, actionAuditEventGetProject,
			map[string]any{"project_id": f.project.IDParam(), "event_id": projectEvents.AuditEvents[0].ID})
		if projectEvent.ID != projectEvents.AuditEvents[0].ID {
			e.T.Errorf("get_project answered event %d, want the listed %d", projectEvent.ID, projectEvents.AuditEvents[0].ID)
		}

		groupEvents := harness.Eventually(s, actionAuditEventListGroup,
			map[string]any{"group_id": f.group.IDParam(), "per_page": 20}, auditEventInterval, auditEventWait, hasAuditEvents)
		groupEvent := harness.Do[auditevents.Output](s, actionAuditEventGetGroup,
			map[string]any{"group_id": f.group.IDParam(), "event_id": groupEvents.AuditEvents[0].ID})
		if groupEvent.ID != groupEvents.AuditEvents[0].ID {
			e.T.Errorf("get_group answered event %d, want the listed %d", groupEvent.ID, groupEvents.AuditEvents[0].ID)
		}

		instanceEvents := harness.Eventually(s, actionAuditEventListInstance,
			map[string]any{"per_page": 20}, auditEventInterval, auditEventWait, hasAuditEvents)
		instanceEvent := harness.Do[auditevents.Output](s, actionAuditEventGetInstance,
			map[string]any{"event_id": instanceEvents.AuditEvents[0].ID})
		if instanceEvent.ID != instanceEvents.AuditEvents[0].ID {
			e.T.Errorf("get_instance answered event %d, want the listed %d", instanceEvent.ID, instanceEvents.AuditEvents[0].ID)
		}
	})
}
