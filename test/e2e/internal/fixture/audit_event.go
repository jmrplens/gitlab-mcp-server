//go:build e2e

// audit_event.go builds an audit event, which is the one object here that is
// not created but caused.
//
// No endpoint writes an audit event: GitLab writes one as a side effect of a
// change somebody made, on a background job, so the builder edits a project it
// owns and then waits for the record to appear. That is the same fixture the
// end-to-end suite's own audit event scenario is built on, and it is why a
// world naming an audit event does not have to reserve an identifier nothing
// holds.
//
// The wait is in two steps because the two listings answer different
// questions. The project's own log holds only what happened in that project,
// so the first event it carries is the one this builder caused; the instance
// log holds everything the instance ever recorded, so a non-empty answer says
// nothing and the wait is for the event found above to appear in it.

package fixture

import (
	"context"
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// auditEventDescription is what the builder edits the project's description
// to, which is the change GitLab records.
const auditEventDescription = "e2e audit event fixture"

// The audit event waits: GitLab indexes an event a moment after the change
// that caused it, and a loaded Docker instance takes longer.
const (
	auditEventInterval = 2 * time.Second
	auditEventWait     = 90 * time.Second
)

// auditEventPageSize is how many events one listing page carries. The
// instance log is read newest first, so one page is where a fresh event is.
const auditEventPageSize = 100

// AuditEvent is an event GitLab recorded, as a case addresses it.
type AuditEvent struct {
	// ID is what every audit event read takes.
	ID int64
	// EntityType and EntityID say what the event was about.
	EntityType string
	EntityID   int64
}

// NewInstanceAuditEvent edits the project to cause an audit event, waits for
// GitLab to record it, and waits again for the instance log to carry it.
//
// It fails the test when no event appears, rather than handing back a zero: a
// world that rendered one would ask every model about an event that has never
// existed, which is a refusal charged to the model for something the fixture
// did.
//
// The event goes with the project, so nothing is registered.
func NewInstanceAuditEvent(e *harness.Env, project Project) AuditEvent {
	e.T.Helper()

	_, err := retryTransient(e, "cause an audit event", createRetries, func() (struct{}, error) {
		return struct{}{}, causeProjectAuditEvent(e.Ctx, e.Client(), project.ID)
	})
	if err != nil {
		e.T.Fatalf("editing project %d to cause an audit event: %v", project.ID, err)
	}

	var event AuditEvent
	err = harness.Poll(e.Ctx, auditEventInterval, auditEventWait, func() (bool, string, error) {
		found, ok, readErr := firstProjectAuditEvent(e.Ctx, e.Client(), project.ID)
		if readErr != nil {
			return false, "", readErr
		}
		event = found
		return ok, fmt.Sprintf("project %d has recorded no audit event yet", project.ID), nil
	})
	if err != nil {
		e.T.Fatalf("project %d recorded no audit event within %s: %v", project.ID, auditEventWait, err)
	}

	err = harness.Poll(e.Ctx, auditEventInterval, auditEventWait, func() (bool, string, error) {
		held, readErr := instanceAuditLogHolds(e.Ctx, e.Client(), event.ID)
		if readErr != nil {
			return false, "", readErr
		}
		return held, fmt.Sprintf("the instance log does not carry event %d yet", event.ID), nil
	})
	if err != nil {
		e.T.Fatalf("audit event %d of project %d never reached the instance log within %s: %v",
			event.ID, project.ID, auditEventWait, err)
	}
	return event
}

// causeProjectAuditEvent makes the change GitLab records: an edit of the
// project's description, which is a change with no consequence for anything
// else a case does in that project.
func causeProjectAuditEvent(ctx context.Context, client *gitlabclient.Client, projectID int64) error {
	_, _, err := client.GL().Projects.EditProject(projectID, &gl.EditProjectOptions{
		Description: new(auditEventDescription),
	}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("editing project %d: %w", projectID, err)
	}
	return nil
}

// firstProjectAuditEvent reads the newest event of the project's own log, and
// reports whether there is one at all.
func firstProjectAuditEvent(ctx context.Context, client *gitlabclient.Client, projectID int64) (AuditEvent, bool, error) {
	opts := &gl.ListAuditEventsOptions{}
	opts.PerPage = auditEventPageSize
	events, _, err := client.GL().AuditEvents.ListProjectAuditEvents(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return AuditEvent{}, false, fmt.Errorf("listing the audit events of project %d: %w", projectID, err)
	}
	for _, event := range events {
		if event == nil || event.ID == 0 {
			continue
		}
		return AuditEvent{ID: event.ID, EntityType: event.EntityType, EntityID: event.EntityID}, true, nil
	}
	return AuditEvent{}, false, nil
}

// instanceAuditLogHolds reports whether the instance log's newest page carries
// the event, which is what the instance read a case makes needs.
func instanceAuditLogHolds(ctx context.Context, client *gitlabclient.Client, eventID int64) (bool, error) {
	opts := &gl.ListAuditEventsOptions{}
	opts.PerPage = auditEventPageSize
	events, _, err := client.GL().AuditEvents.ListInstanceAuditEvents(opts, gl.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("listing the instance audit events: %w", err)
	}
	for _, event := range events {
		if event != nil && event.ID == eventID {
			return true, nil
		}
	}
	return false, nil
}
