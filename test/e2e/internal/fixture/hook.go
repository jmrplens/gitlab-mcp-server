//go:build e2e

// hook.go reads what GitLab recorded about a webhook's deliveries, which
// client-go offers no method for: the events endpoint is reached through the
// SDK's own request plumbing, the way the server reaches an endpoint the SDK
// has no wrapper for.

package fixture

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// hookEventWait bounds the wait for a delivery to be recorded: GitLab
// delivers a test event through Sidekiq, and the fixture service answers
// it at once.
const hookEventWait = 60 * time.Second

// hookEventPollInterval is how often the recorded deliveries are read; a
// variable so the package's own tests can ask faster than Sidekiq delivers.
var hookEventPollInterval = 2 * time.Second

// hookEvent is the one field of a recorded delivery a test needs.
type hookEvent struct {
	ID int64 `json:"id"`
}

// WaitForGroupHookEvent waits until GitLab has recorded a delivery of the
// group hook and returns the first one's ID, failing the test when none is
// recorded within the budget. A hook whose test trigger has just been fired
// has a delivery a moment later, once Sidekiq has run it.
func WaitForGroupHookEvent(e *harness.Env, group Group, hookID int64) int64 {
	e.T.Helper()

	id, err := firstGroupHookEvent(e.Ctx, e.Client(), group.ID, hookID)
	if err != nil {
		e.T.Fatalf("waiting for a delivery of hook %d of group %d: %v", hookID, group.ID, err)
	}
	return id
}

// firstGroupHookEvent is WaitForGroupHookEvent without the test, so the
// package's own tests can drive it against a stub.
func firstGroupHookEvent(ctx context.Context, client *gitlabclient.Client, groupID, hookID int64) (int64, error) {
	var first int64
	err := harness.Poll(ctx, hookEventPollInterval, hookEventWait, func() (bool, string, error) {
		events, err := listGroupHookEvents(ctx, client, groupID, hookID)
		if err != nil {
			// The endpoint answers a 404 until the first delivery exists on
			// some releases, so an error is a state to wait through.
			return false, fmt.Sprintf("the hook events endpoint answered: %v", err), nil
		}
		if len(events) == 0 {
			return false, "no delivery recorded yet", nil
		}
		first = events[0].ID
		return true, "", nil
	})
	if err != nil {
		return 0, err
	}
	return first, nil
}

// listGroupHookEvents reads the recorded deliveries of one group hook.
func listGroupHookEvents(ctx context.Context, client *gitlabclient.Client, groupID, hookID int64) ([]hookEvent, error) {
	request, err := client.GL().NewRequest(http.MethodGet, fmt.Sprintf("groups/%d/hooks/%d/events", groupID, hookID),
		nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, fmt.Errorf("building the hook events request: %w", err)
	}
	var events []hookEvent
	if _, err = client.GL().Do(request, &events); err != nil {
		return nil, err
	}
	return events, nil
}
