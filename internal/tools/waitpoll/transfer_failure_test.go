package waitpoll

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// transferAnsweredAt is the instant the tests' transfers were answered at.
var transferAnsweredAt = time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)

// transferFailureTodo is a pending transfer_failed item about the object with
// targetID, created offset from [transferAnsweredAt] and naming destination.
func transferFailureTodo(targetID int64, offset time.Duration, destination string) string {
	return fmt.Sprintf(`{"id":5,"action_name":"transfer_failed","target_type":"Project","state":"pending",`+
		`"target":{"id":%d},"body":%q,"created_at":%q}`,
		targetID, destination, transferAnsweredAt.Add(offset).Format(time.RFC3339Nano))
}

// todoListGitLab answers GET /todos with one fixed body and records the
// query of each request.
type todoListGitLab struct {
	t       *testing.T
	status  int
	body    string
	hits    atomic.Int64
	queries atomic.Value
}

// handler is the mock the test client is built on.
func (g *todoListGitLab) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/todos" {
			g.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		g.hits.Add(1)
		g.queries.Store(r.URL.Query())
		testutil.RespondJSON(w, g.status, g.body)
	}
}

// query returns the query of the last request.
func (g *todoListGitLab) query() url.Values {
	q, _ := g.queries.Load().(url.Values)
	return q
}

// TestAnsweredAt_ReadsTheDateHeader verifies that the instant comes from the
// response's Date header, and that a header missing or unreadable gives none.
func TestAnsweredAt_ReadsTheDateHeader(t *testing.T) {
	tests := []struct {
		name string
		date []string
		want time.Time
	}{
		{name: "a date", date: []string{"Sun, 27 Sep 2026 10:00:00 GMT"}, want: transferAnsweredAt},
		{name: "no date", want: time.Time{}},
		{name: "a date that does not parse", date: []string{"yesterday"}, want: time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &gl.Response{Response: &http.Response{Header: http.Header{}}}
			if tt.date != nil {
				resp.Header["Date"] = tt.date
			}
			if got := AnsweredAt(resp); !got.Equal(tt.want) {
				t.Errorf("AnsweredAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTransferFailed_NoAnswerInstant_AsksNothing verifies that without the
// instant the transfer was answered at no item can be told apart from an
// older one, so nothing is asked and nothing is reported.
func TestTransferFailed_NoAnswerInstant_AsksNothing(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	destination, failed, err := TransferFailed(context.Background(), client, TransferTarget{ID: 42}, time.Time{})
	if err != nil || failed || destination != "" {
		t.Errorf("TransferFailed() = %q, %v, %v, want nothing reported and no error", destination, failed, err)
	}
}

// TestTransferFailed_FiltersByTheObject verifies the listing asks GitLab for
// the pending transfer_failed items of the object alone: a project by its
// project_id and target type Project, a group by its group_id and target type
// Namespace, and never the other scope.
func TestTransferFailed_FiltersByTheObject(t *testing.T) {
	tests := []struct {
		name   string
		target TransferTarget
		want   url.Values
	}{
		{
			name: "project", target: TransferTarget{ID: 42},
			want: url.Values{"action": {"transfer_failed"}, "state": {"pending"}, "type": {"Project"}, "project_id": {"42"}, "per_page": {"100"}},
		},
		{
			name: "group", target: TransferTarget{ID: 99, Group: true},
			want: url.Values{"action": {"transfer_failed"}, "state": {"pending"}, "type": {"Namespace"}, "group_id": {"99"}, "per_page": {"100"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitlab := &todoListGitLab{t: t, status: http.StatusOK, body: `[]`}
			client := testutil.NewTestClient(t, gitlab.handler())

			_, failed, err := TransferFailed(context.Background(), client, tt.target, transferAnsweredAt)
			if err != nil || failed {
				t.Fatalf("TransferFailed() = %v, %v, want no failure and no error", failed, err)
			}
			if got := gitlab.query(); got.Encode() != tt.want.Encode() {
				t.Errorf("query = %s, want %s", got.Encode(), tt.want.Encode())
			}
		})
	}
}

// TestTransferFailed_CountsOnlyAnItemOfThisTransfer drives the items a
// listing can carry and holds which of them report this transfer's failure:
// one about this object created no earlier than two seconds before the
// transfer was answered, and no other.
func TestTransferFailed_CountsOnlyAnItemOfThisTransfer(t *testing.T) {
	tests := []struct {
		name   string
		items  []string
		failed bool
	}{
		{name: "created after the answer", items: []string{transferFailureTodo(42, time.Second, "newns/moving")}, failed: true},
		{name: "created just before the answer", items: []string{transferFailureTodo(42, -1500*time.Millisecond, "newns/moving")}, failed: true},
		{name: "created two seconds before the answer", items: []string{transferFailureTodo(42, -2*time.Second, "newns/moving")}, failed: true},
		{name: "left by an earlier failure", items: []string{transferFailureTodo(42, -2500*time.Millisecond, "newns/moving")}},
		{name: "about a descendant", items: []string{transferFailureTodo(43, time.Second, "newns/moving")}},
		{
			name: "after another object's item", failed: true,
			items: []string{transferFailureTodo(43, time.Second, "other"), transferFailureTodo(42, time.Second, "newns/moving")},
		},
		{name: "without a target", items: []string{`{"id":5,"body":"newns/moving","created_at":"2026-09-27T10:00:01Z"}`}},
		{name: "with a target id that is not a number", items: []string{`{"id":5,"target":{"id":"42"},"body":"newns/moving","created_at":"2026-09-27T10:00:01Z"}`}},
		{name: "without a creation date", items: []string{`{"id":5,"target":{"id":42},"body":"newns/moving"}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitlab := &todoListGitLab{t: t, status: http.StatusOK, body: "[" + strings.Join(tt.items, ",") + "]"}
			client := testutil.NewTestClient(t, gitlab.handler())

			destination, failed, err := TransferFailed(context.Background(), client, TransferTarget{ID: 42}, transferAnsweredAt)
			if err != nil {
				t.Fatalf("TransferFailed() error = %v", err)
			}
			if failed != tt.failed {
				t.Errorf("TransferFailed() failed = %v, want %v", failed, tt.failed)
			}
			if want := map[bool]string{true: "newns/moving"}[tt.failed]; destination != want {
				t.Errorf("TransferFailed() destination = %q, want %q", destination, want)
			}
		})
	}
}

// TestTransferFailed_ListingFails_ReturnsTheError verifies that a listing
// GitLab fails to serve is an error, not a report that nothing failed.
func TestTransferFailed_ListingFails_ReturnsTheError(t *testing.T) {
	gitlab := &todoListGitLab{t: t, status: http.StatusBadGateway, body: `{"message":"502 Bad Gateway"}`}
	client := testutil.NewTestClient(t, gitlab.handler())

	_, failed, err := TransferFailed(context.Background(), client, TransferTarget{ID: 42}, transferAnsweredAt)
	if err == nil || failed {
		t.Errorf("TransferFailed() = %v, %v, want the listing's error", failed, err)
	}
}

// TestTransferFailed_CarriesTheCallersContext verifies the listing is sent
// with the caller's context, so an abandoned wait abandons it too.
func TestTransferFailed_CarriesTheCallersContext(t *testing.T) {
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	_, _, err := TransferFailed(ctx, client, TransferTarget{ID: 42}, transferAnsweredAt)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("TransferFailed() error = %v, want context.Canceled", err)
	}
}
