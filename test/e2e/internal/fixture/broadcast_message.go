//go:build e2e

// broadcast_message.go builds an instance broadcast message.
//
// It is instance-wide and needs an administrator, so a test that builds one
// declares harness.NeedAdmin. It is deliberately scheduled to start in the
// past and end well after any run: a message GitLab considers inactive is
// left out of the listing a read case reads, and one that ends mid-run would
// make that case pass or fail by the clock.

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

// The window every fixture message is live in, measured from the moment it is
// created.
const (
	broadcastMessageStartedAgo = time.Hour
	broadcastMessageLifetime   = 24 * time.Hour
)

// BroadcastMessage is a broadcast message a builder created.
type BroadcastMessage struct {
	// ID is what the broadcast message actions take.
	ID int64
	// Message is the text itself.
	Message string
}

// NewBroadcastMessage creates an active instance broadcast message and
// registers its deletion.
func NewBroadcastMessage(e *harness.Env) BroadcastMessage {
	e.T.Helper()
	requireAdmin(e, "an instance broadcast message")

	text := "e2e broadcast fixture " + e.Name("msg")
	message, err := retryTransient(e, "create broadcast message", createRetries, func() (BroadcastMessage, error) {
		return createBroadcastMessage(e.Ctx, e.Client(), text, time.Now())
	})
	if err != nil {
		e.T.Fatalf("creating a broadcast message: %v", err)
	}

	e.Defer(fmt.Sprintf("broadcast message %d", message.ID), func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteBroadcastMessage(ctx, e.Client(), message.ID)
	})
	return message
}

// createBroadcastMessage asks GitLab for the message, live from an hour ago
// so that no clock skew between this process and the instance can leave it
// scheduled rather than active.
func createBroadcastMessage(ctx context.Context, client *gitlabclient.Client, text string, now time.Time) (BroadcastMessage, error) {
	startsAt := now.Add(-broadcastMessageStartedAgo)
	endsAt := now.Add(broadcastMessageLifetime)
	created, _, err := client.GL().BroadcastMessage.CreateBroadcastMessage(&gl.CreateBroadcastMessageOptions{
		Message:  new(text),
		StartsAt: &startsAt,
		EndsAt:   &endsAt,
	}, gl.WithContext(ctx))
	if err != nil {
		return BroadcastMessage{}, err
	}
	return BroadcastMessage{ID: created.ID, Message: created.Message}, nil
}

// deleteBroadcastMessage removes the message and tolerates one a case
// deleted.
func deleteBroadcastMessage(ctx context.Context, client *gitlabclient.Client, messageID int64) error {
	_, err := client.GL().BroadcastMessage.DeleteBroadcastMessage(messageID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting broadcast message %d: %w", messageID, err)
	}
	return nil
}
