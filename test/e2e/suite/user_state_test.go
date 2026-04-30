//go:build e2e

// user_state_test.go snapshots and restores current-user state mutated by E2E
// tests so notification and status changes do not leak across test cases.
package suite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/notifications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// CurrentUserStateSnapshot captures mutable current-user settings touched by E2E tests.
type CurrentUserStateSnapshot struct {
	Status             users.StatusOutput
	GlobalNotification notifications.Output
}

// SnapshotCurrentUserState captures current-user state that selected E2E tests mutate.
func SnapshotCurrentUserState(ctx context.Context, e2e *E2EContext) (CurrentUserStateSnapshot, error) {
	status, err := callToolOn[users.StatusOutput](ctx, e2e.Meta(), "gitlab_user", map[string]any{
		"action": "current_user_status",
		"params": map[string]any{},
	})
	if err != nil {
		return CurrentUserStateSnapshot{}, fmt.Errorf("snapshot current-user status: %w", err)
	}

	globalNotification, err := callToolOn[notifications.Output](ctx, e2e.Meta(), "gitlab_user", map[string]any{
		"action": "notification_global_get",
		"params": map[string]any{},
	})
	if err != nil {
		return CurrentUserStateSnapshot{}, fmt.Errorf("snapshot global notification: %w", err)
	}

	return CurrentUserStateSnapshot{Status: status, GlobalNotification: globalNotification}, nil
}

// RegisterCurrentUserStateRestore snapshots current-user state and restores it during cleanup.
func RegisterCurrentUserStateRestore(ctx context.Context, e2e *E2EContext) CurrentUserStateSnapshot {
	e2e.T.Helper()
	snapshot, err := SnapshotCurrentUserState(ctx, e2e)
	requireNoError(e2e.T, err, "snapshot current-user state")

	e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindCurrentUserState,
		ID:        "current-user",
		Name:      "current-user-state",
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			return RestoreCurrentUserState(cleanupCtx, e2e, snapshot)
		},
	})

	return snapshot
}

// RestoreCurrentUserState best-effort restores mutable current-user settings.
func RestoreCurrentUserState(ctx context.Context, e2e *E2EContext, snapshot CurrentUserStateSnapshot) error {
	var failures []error
	if err := restoreCurrentUserStatus(ctx, e2e, snapshot.Status); err != nil {
		failures = append(failures, err)
	}
	if err := restoreGlobalNotification(ctx, e2e, snapshot.GlobalNotification); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

// restoreCurrentUserStatus restores the GitLab current-user status using the
// raw GitLab client because the status update operation is user-scoped state.
func restoreCurrentUserStatus(ctx context.Context, e2e *E2EContext, status users.StatusOutput) error {
	if e2e.GitLab == nil {
		return fmt.Errorf("restore current-user status: gitlab client not configured; set GITLAB_URL and GITLAB_TOKEN for E2E setup")
	}
	opts := &gl.UserStatusOptions{
		Emoji:   &status.Emoji,
		Message: &status.Message,
	}
	if status.Availability != "" {
		availability := gl.AvailabilityValue(status.Availability)
		opts.Availability = &availability
	}
	_, _, err := e2e.GitLab.GL().Users.SetUserStatus(opts, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("restore current-user status: %w", err)
	}
	return nil
}

// restoreGlobalNotification restores the user's global notification level when
// the snapshot contained one.
func restoreGlobalNotification(ctx context.Context, e2e *E2EContext, notification notifications.Output) error {
	if notification.Level == "" {
		return nil
	}
	_, err := callToolOn[notifications.Output](ctx, e2e.Meta(), "gitlab_user", map[string]any{
		"action": "notification_global_update",
		"params": map[string]any{"level": notification.Level},
	})
	if err != nil {
		return fmt.Errorf("restore global notification: %w", err)
	}
	return nil
}
