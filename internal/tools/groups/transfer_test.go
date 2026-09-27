package groups

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/waitpoll"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// pathGroupTransfer is the route a transfer of group 99 is sent to.
	pathGroupTransfer = pathGroup99 + "/transfer"

	// groupAtTopLevel is group 99 where a transfer finds it: at the top level.
	groupAtTopLevel = `{"id":99,"name":"child","path":"child","full_path":"child","visibility":"private"}`
	// groupUnderParent is group 99 after the move under group 42.
	groupUnderParent = `{"id":99,"name":"child","path":"child","full_path":"parent/child","visibility":"private","parent_id":42}`
	// groupUnderOtherParent is group 99 under group 7, a parent the tests
	// never name, so it is where a group is neither promoted nor moved to 42.
	groupUnderOtherParent = `{"id":99,"name":"child","path":"child","full_path":"other/child","visibility":"private","parent_id":7}`
)

// landingBound is the wait a test that expects the move to land gives it. The
// move lands on the second or third read, a few milliseconds in, so the bound
// is generous for a correct handler. It is only a backstop: a test that
// expects the move to land calls the handler with
// [groupTransferGitLab.landingContext], which ends as soon as the handler
// reads past the scripted reads, so a handler that never sees the move land
// fails the test at that read rather than at the bound.
const landingBound = 2 * time.Second

// groupTransferDate is the Date header a test's transfer answer carries when
// the test dates a to-do item against it.
const groupTransferDate = "Sun, 27 Sep 2026 10:00:00 GMT"

// groupTransferAnsweredAt is the instant [groupTransferDate] names.
var groupTransferAnsweredAt = time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)

// failedGroupTransferTodos is a to-do listing holding the item GitLab leaves
// when the transfer of group 99 under group 42 fails, created a second after
// the transfer was answered, beside the item of a descendant's failure, which
// the group's listing carries too.
var failedGroupTransferTodos = fmt.Sprintf(`[`+
	`{"id":6,"action_name":"transfer_failed","target_type":"Namespace","target":{"id":100},"body":"parent/child/sub","created_at":%[1]q},`+
	`{"id":5,"action_name":"transfer_failed","target_type":"Namespace","target":{"id":99},"body":"parent/child","created_at":%[1]q}]`,
	groupTransferAnsweredAt.Add(time.Second).Format(time.RFC3339))

// fastTransferWait makes TransferSubGroup read back every millisecond for at
// most bound, and restores the package's timing when the test ends.
func fastTransferWait(t *testing.T, bound time.Duration) {
	t.Helper()
	oldBound, oldInterval := transferBound, transferInterval
	transferBound, transferInterval = bound, time.Millisecond
	t.Cleanup(func() { transferBound, transferInterval = oldBound, oldInterval })
}

// groupTransferGitLab is a GitLab whose transfer answers with one group and
// whose reads of group 99 answer with the others in order, repeating the last.
// Its to-do listing answers todos, or no items when that is empty, and a
// non-empty date is the Date header of the transfer's answer. It counts the
// reads and the listings, and records what each transfer and read sent.
type groupTransferGitLab struct {
	t        *testing.T
	answer   string
	date     string
	reads    []string
	todos    string
	onRead   func()
	stop     context.CancelFunc
	readHits atomic.Int64
	todoHits atomic.Int64

	mu          sync.Mutex
	bodies      []string
	readQueries []string
}

// landingContext is the context a test that expects the move to land calls
// the handler with. It ends when the handler reads the group more times than
// the test scripted, which a correct handler never does, since the last
// scripted read is where the move lands or the failure is reported.
func (g *groupTransferGitLab) landingContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	g.stop = cancel
	return ctx
}

// sent returns the transfer bodies and the read queries, in order.
func (g *groupTransferGitLab) sent() (bodies, queries []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.bodies...), append([]string(nil), g.readQueries...)
}

// handler is the mock the test client is built on.
func (g *groupTransferGitLab) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == pathGroupTransfer:
			raw, _ := io.ReadAll(r.Body)
			g.mu.Lock()
			g.bodies = append(g.bodies, string(raw))
			g.mu.Unlock()
			if g.date != "" {
				w.Header().Set("Date", g.date)
			}
			testutil.RespondJSON(w, http.StatusOK, g.answer)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/todos":
			g.todoHits.Add(1)
			testutil.RespondJSON(w, http.StatusOK, cmp.Or(g.todos, "[]"))
		case r.Method == http.MethodGet && r.URL.Path == pathGroup99:
			n := g.readHits.Add(1)
			g.mu.Lock()
			g.readQueries = append(g.readQueries, r.URL.RawQuery)
			g.mu.Unlock()
			if g.onRead != nil {
				g.onRead()
			}
			if g.stop != nil && n > int64(len(g.reads)) {
				g.stop()
			}
			if len(g.reads) == 0 {
				g.t.Errorf("read %d of group 99, want none", n)
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"no read expected"}`)
				return
			}
			testutil.RespondJSON(w, http.StatusOK, g.reads[min(int(n), len(g.reads))-1])
		default:
			g.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

// TestTransferSubGroup_AnswerAlreadyInPlace_IsReturnedWithoutReadingBack
// holds the GitLab 19.3 behavior: the transfer answers after the move, so an
// answer that already shows the group where it was sent is the result and
// nothing is read back, for a move under a parent and for a promotion to the
// top level alike.
func TestTransferSubGroup_AnswerAlreadyInPlace_IsReturnedWithoutReadingBack(t *testing.T) {
	parent := int64(42)
	tests := []struct {
		name     string
		answer   string
		parentID *int64
		wantBody string
	}{
		{name: "under a parent", answer: groupUnderParent, parentID: &parent, wantBody: `{"group_id":42}`},
		{name: "to the top level", answer: groupAtTopLevel, wantBody: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastTransferWait(t, time.Second)
			gitlab := &groupTransferGitLab{t: t, answer: tt.answer}
			client := testutil.NewTestClient(t, gitlab.handler())

			out, err := TransferSubGroup(gitlab.landingContext(t), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: tt.parentID})
			if err != nil {
				t.Fatalf("TransferSubGroup() error = %v", err)
			}
			if out.TransferQueued || out.ID != 99 {
				t.Errorf("TransferSubGroup() = group %d queued %v, want group 99 applied", out.ID, out.TransferQueued)
			}
			if reads, listings := gitlab.readHits.Load(), gitlab.todoHits.Load(); reads != 0 || listings != 0 {
				t.Errorf("reads = %d and to-do listings = %d, want none: the answer already showed the group in place", reads, listings)
			}
			if bodies, _ := gitlab.sent(); len(bodies) != 1 || !jsonEqual(bodies[0], tt.wantBody) {
				t.Errorf("transfer bodies = %q, want exactly %s", bodies, tt.wantBody)
			}
		})
	}
}

// jsonEqual reports whether two JSON documents decode to the same value.
func jsonEqual(a, b string) bool {
	var av, bv any
	if json.Unmarshal([]byte(a), &av) != nil || json.Unmarshal([]byte(b), &bv) != nil {
		return false
	}
	return fmt.Sprint(av) == fmt.Sprint(bv)
}

// TestTransferSubGroup_MoveLandsLater_AnswersTheMovedGroup holds the GitLab
// 19.4 behavior: the transfer answers with the group where it still is, the
// first reads find it there, and the answer is the read that found it under
// the parent, not the transfer's own. The reads ask for no projects, which
// the transfer's own answer never carries, and each read that did not find
// the move also asked whether GitLab reported it failed.
func TestTransferSubGroup_MoveLandsLater_AnswersTheMovedGroup(t *testing.T) {
	fastTransferWait(t, landingBound)
	gitlab := &groupTransferGitLab{t: t, answer: groupAtTopLevel, reads: []string{
		groupAtTopLevel, groupAtTopLevel, groupUnderParent,
	}}
	client := testutil.NewTestClient(t, gitlab.handler())
	parent := int64(42)

	out, err := TransferSubGroup(gitlab.landingContext(t), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: &parent})
	if err != nil {
		t.Fatalf("TransferSubGroup() error = %v", err)
	}
	if out.TransferQueued || out.ParentID != 42 || out.FullPath != "parent/child" {
		t.Errorf("TransferSubGroup() = queued %v under %d at %q, want applied under 42 at parent/child", out.TransferQueued, out.ParentID, out.FullPath)
	}
	if got := gitlab.readHits.Load(); got != 3 {
		t.Errorf("reads = %d, want 3: two before the move and the one that found it", got)
	}
	if got := gitlab.todoHits.Load(); got != 2 {
		t.Errorf("to-do listings = %d, want 2: one after each read that did not find the move", got)
	}
	if _, queries := gitlab.sent(); len(queries) == 0 || queries[0] != "with_projects=false" {
		t.Errorf("read queries = %q, want with_projects=false", queries)
	}
}

// TestTransferSubGroup_MoveFailsInTheBackground_AnswersTheFailure verifies
// that a transfer the worker refuses, which GitLab reports only as a to-do
// item on the group, ends the wait at the first read that finds the item,
// with an error naming the destination, and that the item of a descendant's
// failure, which the group's listing also carries, is not taken for it.
func TestTransferSubGroup_MoveFailsInTheBackground_AnswersTheFailure(t *testing.T) {
	fastTransferWait(t, landingBound)
	gitlab := &groupTransferGitLab{
		t: t, answer: groupAtTopLevel, date: groupTransferDate,
		reads: []string{groupAtTopLevel}, todos: failedGroupTransferTodos,
	}
	client := testutil.NewTestClient(t, gitlab.handler())
	parent := int64(42)

	_, err := TransferSubGroup(gitlab.landingContext(t), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: &parent})
	if err == nil {
		t.Fatal("TransferSubGroup() error = nil, want the failure GitLab reported")
	}
	if msg := err.Error(); !strings.HasPrefix(msg, "groupTransferSubGroup: ") ||
		!strings.Contains(msg, "the transfer to parent/child failed") || !strings.Contains(msg, hintSubGroupTransferFailed) {
		t.Errorf("TransferSubGroup() error = %q, want the destination the to-do item names and the hint %q", msg, hintSubGroupTransferFailed)
	}
	if reads, listings := gitlab.readHits.Load(), gitlab.todoHits.Load(); reads != 1 || listings != 1 {
		t.Errorf("reads = %d and to-do listings = %d, want 1 each: the first listing found the failure", reads, listings)
	}
}

// TestTransferSubGroup_ReadFails_KeepsWaiting verifies that a read GitLab
// fails to serve neither ends the wait nor leads to a to-do listing: the next
// read finds the group moved.
func TestTransferSubGroup_ReadFails_KeepsWaiting(t *testing.T) {
	fastTransferWait(t, landingBound)
	var reads atomic.Int64
	gitlab := &groupTransferGitLab{t: t, answer: groupAtTopLevel, reads: []string{groupUnderParent}}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroup99 && reads.Add(1) == 1 {
			testutil.RespondJSON(w, http.StatusBadGateway, `{"message":"502 Bad Gateway"}`)
			return
		}
		gitlab.handler()(w, r)
	}))
	parent := int64(42)

	out, err := TransferSubGroup(gitlab.landingContext(t), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: &parent})
	if err != nil {
		t.Fatalf("TransferSubGroup() error = %v", err)
	}
	if out.TransferQueued || out.ParentID != 42 {
		t.Errorf("TransferSubGroup() = queued %v under %d, want applied under 42", out.TransferQueued, out.ParentID)
	}
	if got := gitlab.todoHits.Load(); got != 0 {
		t.Errorf("to-do listings = %d, want none: a failed read says nothing about the move", got)
	}
}

// TestTransferSubGroup_ReadByPath_ReadsBackByTheAnsweredID verifies that the
// read back names the group by the id the transfer answered with, not by the
// path the caller named, which is the path the move is taking away.
func TestTransferSubGroup_ReadByPath_ReadsBackByTheAnsweredID(t *testing.T) {
	fastTransferWait(t, landingBound)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var readPath atomic.Value
	var reads atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusOK, groupAtTopLevel)
		default:
			// The first read finds the move, so a second one is a handler
			// that did not see it, which ends the call rather than the bound.
			if reads.Add(1) > 1 {
				stop()
			}
			readPath.Store(r.URL.Path)
			testutil.RespondJSON(w, http.StatusOK, groupUnderParent)
		}
	}))
	parent := int64(42)

	out, err := TransferSubGroup(ctx, nil, client, TransferSubGroupInput{GroupID: "child", ParentID: &parent})
	if err != nil {
		t.Fatalf("TransferSubGroup() error = %v", err)
	}
	if out.TransferQueued {
		t.Error("TransferSubGroup() reported the move queued, want it applied")
	}
	if got, _ := readPath.Load().(string); got != pathGroup99 {
		t.Errorf("read back %q, want %q", got, pathGroup99)
	}
}

// TestTransferSubGroup_PromotionLandsLater_AnswersTheTopLevelGroup verifies
// that a promotion waits for the group to report no parent, and that a group
// still under a parent has not landed.
func TestTransferSubGroup_PromotionLandsLater_AnswersTheTopLevelGroup(t *testing.T) {
	fastTransferWait(t, landingBound)
	gitlab := &groupTransferGitLab{t: t, answer: groupUnderParent, reads: []string{groupUnderParent, groupAtTopLevel}}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := TransferSubGroup(gitlab.landingContext(t), nil, client, TransferSubGroupInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("TransferSubGroup() error = %v", err)
	}
	if out.TransferQueued || out.ParentID != 0 || out.FullPath != "child" {
		t.Errorf("TransferSubGroup() = queued %v under %d at %q, want applied at the top level", out.TransferQueued, out.ParentID, out.FullPath)
	}
	if got := gitlab.readHits.Load(); got != 2 {
		t.Errorf("reads = %d, want 2", got)
	}
}

// TestTransferSubGroup_UnderAnotherParent_HasNotLanded verifies that a group
// under a parent other than the one named has not landed, whether the
// transfer named a parent or none.
func TestTransferSubGroup_UnderAnotherParent_HasNotLanded(t *testing.T) {
	parent := int64(42)
	tests := []struct {
		name     string
		parentID *int64
		landed   string
	}{
		{name: "moving under a parent", parentID: &parent, landed: groupUnderParent},
		{name: "promoting to the top level", landed: groupAtTopLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastTransferWait(t, landingBound)
			gitlab := &groupTransferGitLab{t: t, answer: groupUnderOtherParent, reads: []string{groupUnderOtherParent, tt.landed}}
			client := testutil.NewTestClient(t, gitlab.handler())

			out, err := TransferSubGroup(gitlab.landingContext(t), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: tt.parentID})
			if err != nil {
				t.Fatalf("TransferSubGroup() error = %v", err)
			}
			if out.TransferQueued || gitlab.readHits.Load() != 2 {
				t.Errorf("TransferSubGroup() = queued %v after %d reads, want applied after 2: parent 7 is not the destination", out.TransferQueued, gitlab.readHits.Load())
			}
		})
	}
}

// TestTransferSubGroup_MoveNeverLands_AnswersQueuedWithTheTransferAnswer
// verifies that a move the wait never sees land is answered, without error,
// with the group where the transfer's own answer placed it and TransferQueued
// set: GitLab accepted the transfer, so it is not a failure to report.
func TestTransferSubGroup_MoveNeverLands_AnswersQueuedWithTheTransferAnswer(t *testing.T) {
	fastTransferWait(t, 50*time.Millisecond)
	gitlab := &groupTransferGitLab{t: t, answer: groupAtTopLevel, reads: []string{groupAtTopLevel}}
	client := testutil.NewTestClient(t, gitlab.handler())
	parent := int64(42)

	out, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: &parent})
	if err != nil {
		t.Fatalf("TransferSubGroup() error = %v, want the queued answer", err)
	}
	if !out.TransferQueued || out.ID != 99 || out.FullPath != "child" {
		t.Errorf("TransferSubGroup() = group %d at %q queued %v, want group 99 at child, queued", out.ID, out.FullPath, out.TransferQueued)
	}
	if gitlab.readHits.Load() == 0 {
		t.Error("reads = 0, want the handler to have read the group back")
	}
}

// TestTransferSubGroup_CallerGoesAwayDuringTheWait_AnswersTheContextError
// verifies that the wait honors the caller's context.
func TestTransferSubGroup_CallerGoesAwayDuringTheWait_AnswersTheContextError(t *testing.T) {
	fastTransferWait(t, landingBound)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gitlab := &groupTransferGitLab{t: t, answer: groupAtTopLevel, reads: []string{groupAtTopLevel}, onRead: cancel}
	client := testutil.NewTestClient(t, gitlab.handler())
	parent := int64(42)

	_, err := TransferSubGroup(ctx, nil, client, TransferSubGroupInput{GroupID: "99", ParentID: &parent})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("TransferSubGroup() error = %v, want context.Canceled", err)
	}
}

// TestTransferSubGroup_AnswerThatDoesNotDecode_IsAnError verifies that a
// transfer answer the captured-field type cannot decode is reported rather
// than returned half-read, and that no wait starts on it.
func TestTransferSubGroup_AnswerThatDoesNotDecode_IsAnError(t *testing.T) {
	fastTransferWait(t, time.Second)
	gitlab := &groupTransferGitLab{t: t, answer: `{"id":99,"name":"child","show_diff_preview_in_email":"yes"}`}
	client := testutil.NewTestClient(t, gitlab.handler())

	_, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99"})
	if err == nil || !strings.Contains(err.Error(), "TransferSubGroup") {
		t.Fatalf("TransferSubGroup() error = %v, want the decode failure", err)
	}
}

// TestTransferSubGroup_TransferRequest_CarriesTheCallersContext verifies that
// the transfer request is sent with the caller's context.
func TestTransferSubGroup_TransferRequest_CarriesTheCallersContext(t *testing.T) {
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupAtTopLevel)
	})

	_, err := TransferSubGroup(ctx, nil, client, TransferSubGroupInput{GroupID: "99"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("TransferSubGroup() error = %v, want context.Canceled", err)
	}
}

// TestTransferSubGroup_Guards verifies a cancelled context and a missing
// group_id are refused before GitLab is asked anything.
func TestTransferSubGroup_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	if _, err := TransferSubGroup(testutil.CancelledCtx(t), nil, client, TransferSubGroupInput{GroupID: "99"}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled context: error = %v, want context.Canceled", err)
	}
	if _, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{}); err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Errorf("missing group_id: error = %v, want the group_id refusal", err)
	}
}

// TestTransferSubGroup_Refusal_IsHintedByWhatGitLabSaid drives each refusal
// GitLab gives a group transfer (app/services/groups/transfer_service.rb and
// the route's own) and holds the hint each gets. A transfer that cannot start
// is told it may be running or the group marked for deletion, a group already
// under that parent or at the top level that there is nothing left to move,
// all three to read the group back; only a path collision is told to rename,
// and the refusal over customer relations contacts, which also says "enough
// permissions", is told about the permission it names rather than the Owner
// role.
func TestTransferSubGroup_Refusal_IsHintedByWhatGitLabSaid(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		want    string
	}{
		{
			name: "transfer already under way", status: http.StatusBadRequest,
			message: "Unable to initiate transfer. The group may already have a transfer in progress.", want: hintSubGroupTransferUnderWay,
		},
		{
			name: "already under the parent", status: http.StatusBadRequest,
			message: "Transfer failed: Group is already associated to the parent group.", want: hintSubGroupTransferInPlace,
		},
		{
			name: "already at the top level", status: http.StatusBadRequest,
			message: "Group is already a root group.", want: hintSubGroupTransferInPlace,
		},
		{
			name: "path taken", status: http.StatusBadRequest,
			message: "The parent group already has a subgroup or a project with the same path.", want: hintSubGroupTransferCollision,
		},
		{
			name: "no permission", status: http.StatusBadRequest,
			message: "You don't have enough permissions.", want: hintSubGroupTransferPermission,
		},
		{
			name: "no permission over the contacts", status: http.StatusBadRequest,
			message: "Group contains contacts/organizations and you don't have enough permissions to move them to the new root group.", want: hintSubGroupTransferCRM,
		},
		{
			name: "into its own subgroup", status: http.StatusBadRequest,
			message: "Cannot transfer group to one of its subgroup.", want: hintSubGroupTransferRefused,
		},
		{name: "forbidden", status: http.StatusForbidden, message: "403 Forbidden", want: hintSubGroupTransferPermission},
		{name: "not found", status: http.StatusNotFound, message: "404 Group Not Found", want: hintSubGroupTransferNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, fmt.Sprintf(`{"message":%q}`, tt.message))
			}))
			_, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99"})
			if err == nil {
				t.Fatal("TransferSubGroup() error = nil, want the refusal")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("TransferSubGroup() error = %q, want the hint %q", err, tt.want)
			}
			for _, other := range allSubGroupTransferHints {
				if other != tt.want && strings.Contains(err.Error(), other) {
					t.Errorf("TransferSubGroup() error = %q, carries the hint %q as well", err, other)
				}
			}
		})
	}
}

// TestTransferSubGroup_RefusalUnderAnotherStatus_CarriesNoTransferHint
// verifies the status is part of the match: GitLab's words under a status
// other than the 400 it refuses a transfer with carry GitLab's message and no
// transfer hint.
func TestTransferSubGroup_RefusalUnderAnotherStatus_CarriesNoTransferHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"The group may already have a transfer in progress."}`)
	}))
	_, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal("TransferSubGroup() error = nil, want the failure")
	}
	if hint := subGroupTransferHintIn(err); hint != "" {
		t.Errorf("TransferSubGroup() error = %q, carries the hint %q under 422", err, hint)
	}
	if !strings.Contains(err.Error(), "transfer in progress") {
		t.Errorf("TransferSubGroup() error = %q, want GitLab's own message kept", err)
	}
}

// allSubGroupTransferHints is every hint a refused or failed group transfer
// can carry, so a test can hold that a refusal carries its own hint and none
// of the others.
var allSubGroupTransferHints = []string{
	hintSubGroupTransferPermission, hintSubGroupTransferCRM, hintSubGroupTransferNotFound, hintSubGroupTransferUnderWay,
	hintSubGroupTransferInPlace, hintSubGroupTransferCollision, hintSubGroupTransferRefused, hintSubGroupTransferFailed,
}

// subGroupTransferHintIn returns the first of the transfer hints err
// carries, or "" when it carries none.
func subGroupTransferHintIn(err error) string {
	for _, hint := range allSubGroupTransferHints {
		if strings.Contains(err.Error(), hint) {
			return hint
		}
	}
	return ""
}

// TestTransferTiming_IsWaitpolls verifies the handler waits with the shared
// transfer timing, and that the metadata a model reads states the bound the
// handler waits for.
func TestTransferTiming_IsWaitpolls(t *testing.T) {
	if transferBound != waitpoll.TransferBound || transferInterval != waitpoll.TransferInterval {
		t.Errorf("timing = %v every %v, want waitpoll's %v every %v", transferBound, transferInterval, waitpoll.TransferBound, waitpoll.TransferInterval)
	}
	var options toolutil.ActionSpecOptions
	if !applyGroupShareTransferMetadata("gitlab_group_transfer", &options) {
		t.Fatal("no metadata for gitlab_group_transfer")
	}
	stated := fmt.Sprintf("%d seconds", int(waitpoll.TransferBound/time.Second))
	if !strings.Contains(options.Usage, stated) || !strings.Contains(options.IndividualTool.Description, stated) {
		t.Errorf("group.transfer metadata does not state the %s it waits:\nusage: %s\ndescription: %s", stated, options.Usage, options.IndividualTool.Description)
	}
}

// TestFormatTransferSubGroupMarkdown_Applied_IsTheGroupCard verifies that a
// move that landed renders as the group's own card, under its new parent.
func TestFormatTransferSubGroupMarkdown_Applied_IsTheGroupCard(t *testing.T) {
	out := TransferSubGroupOutput{ID: 99, Name: "child", FullPath: "parent/child", ParentID: 42}

	if got, want := FormatTransferSubGroupMarkdown(out), FormatDetailOutputMarkdown(out.DetailOutput); got != want {
		t.Errorf("applied transfer card:\n got %q\nwant %q", got, want)
	}
}

// TestFormatTransferSubGroupMarkdown_Queued_SaysSoAndPointsAtTheReads
// verifies the card of a move the wait did not see land, byte for byte: the
// heading says it is queued, the rows are where the group still is, including
// a row only a single-group route carries, the note says why and not to send
// it again, and the hints are the reads that settle it.
func TestFormatTransferSubGroupMarkdown_Queued_SaysSoAndPointsAtTheReads(t *testing.T) {
	out := TransferSubGroupOutput{
		TransferQueued: true, ID: 99, Name: "child", FullPath: "child", EnabledGitAccessProtocol: "ssh",
	}

	want := "## Group transfer queued: child\n\n" +
		"- **ID**: 99\n" +
		"- **Path**: child\n" +
		"- **Git Access Protocol**: ssh\n" +
		"\n" + subGroupTransferQueuedNote + "\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.get' to read the group back and see its parent\n" +
		"- Use action 'user.todo_list' to see the to-do item GitLab leaves if the move fails\n"
	if got := FormatTransferSubGroupMarkdown(out); got != want {
		t.Errorf("queued transfer card:\n got %q\nwant %q", got, want)
	}
}
