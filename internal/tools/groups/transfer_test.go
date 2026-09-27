package groups

import (
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
// It counts the reads and records what each transfer and read sent.
type groupTransferGitLab struct {
	t        *testing.T
	answer   string
	reads    []string
	onRead   func()
	readHits atomic.Int64

	mu          sync.Mutex
	bodies      []string
	readQueries []string
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
			testutil.RespondJSON(w, http.StatusOK, g.answer)
		case r.Method == http.MethodGet && r.URL.Path == pathGroup99:
			n := g.readHits.Add(1)
			g.mu.Lock()
			g.readQueries = append(g.readQueries, r.URL.RawQuery)
			g.mu.Unlock()
			if g.onRead != nil {
				g.onRead()
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

			out, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: tt.parentID})
			if err != nil {
				t.Fatalf("TransferSubGroup() error = %v", err)
			}
			if out.TransferQueued || out.ID != 99 {
				t.Errorf("TransferSubGroup() = group %d queued %v, want group 99 applied", out.ID, out.TransferQueued)
			}
			if got := gitlab.readHits.Load(); got != 0 {
				t.Errorf("reads = %d, want none: the answer already showed the group in place", got)
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
// the transfer's own answer never carries.
func TestTransferSubGroup_MoveLandsLater_AnswersTheMovedGroup(t *testing.T) {
	fastTransferWait(t, 10*time.Second)
	gitlab := &groupTransferGitLab{t: t, answer: groupAtTopLevel, reads: []string{
		groupAtTopLevel, groupAtTopLevel, groupUnderParent,
	}}
	client := testutil.NewTestClient(t, gitlab.handler())
	parent := int64(42)

	out, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: &parent})
	if err != nil {
		t.Fatalf("TransferSubGroup() error = %v", err)
	}
	if out.TransferQueued || out.ParentID != 42 || out.FullPath != "parent/child" {
		t.Errorf("TransferSubGroup() = queued %v under %d at %q, want applied under 42 at parent/child", out.TransferQueued, out.ParentID, out.FullPath)
	}
	if got := gitlab.readHits.Load(); got != 3 {
		t.Errorf("reads = %d, want 3: two before the move and the one that found it", got)
	}
	if _, queries := gitlab.sent(); len(queries) == 0 || queries[0] != "with_projects=false" {
		t.Errorf("read queries = %q, want with_projects=false", queries)
	}
}

// TestTransferSubGroup_PromotionLandsLater_AnswersTheTopLevelGroup verifies
// that a promotion waits for the group to report no parent, and that a group
// still under a parent has not landed.
func TestTransferSubGroup_PromotionLandsLater_AnswersTheTopLevelGroup(t *testing.T) {
	fastTransferWait(t, 10*time.Second)
	gitlab := &groupTransferGitLab{t: t, answer: groupUnderParent, reads: []string{groupUnderParent, groupAtTopLevel}}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99"})
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
			fastTransferWait(t, 10*time.Second)
			gitlab := &groupTransferGitLab{t: t, answer: groupUnderOtherParent, reads: []string{groupUnderOtherParent, tt.landed}}
			client := testutil.NewTestClient(t, gitlab.handler())

			out, err := TransferSubGroup(context.Background(), nil, client, TransferSubGroupInput{GroupID: "99", ParentID: tt.parentID})
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
	fastTransferWait(t, 10*time.Second)
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
// the route's own) and holds the hint each gets. A transfer under way, a group
// already under that parent and a group already at the top level are told to
// wait and read the group back, and only a path collision is told to rename.
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
			message: "Transfer failed: Group is already associated to the parent group.", want: hintSubGroupTransferUnderWay,
		},
		{
			name: "already at the top level", status: http.StatusBadRequest,
			message: "Group is already a root group.", want: hintSubGroupTransferUnderWay,
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
			message: "Group contains contacts/organizations and you don't have enough permissions to move them to the new root group.", want: hintSubGroupTransferPermission,
		},
		{
			name: "into its own subgroup", status: http.StatusBadRequest,
			message: "Cannot transfer group to one of its subgroup.", want: hintSubGroupTransferRefused,
		},
		{name: "forbidden", status: http.StatusForbidden, message: "403 Forbidden", want: hintSubGroupTransferPermission},
		{name: "not found", status: http.StatusNotFound, message: "404 Group Not Found", want: hintSubGroupTransferNotFound},
	}
	all := []string{hintSubGroupTransferPermission, hintSubGroupTransferNotFound, hintSubGroupTransferUnderWay, hintSubGroupTransferCollision, hintSubGroupTransferRefused}
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
			for _, other := range all {
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

// subGroupTransferHintIn returns the first of the transfer hints err
// carries, or "" when it carries none.
func subGroupTransferHintIn(err error) string {
	for _, hint := range []string{hintSubGroupTransferPermission, hintSubGroupTransferNotFound, hintSubGroupTransferUnderWay, hintSubGroupTransferCollision, hintSubGroupTransferRefused} {
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
