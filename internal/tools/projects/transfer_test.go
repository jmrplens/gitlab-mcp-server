// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package projects

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
	// pathProjectTransfer is the route a transfer of project 42 is sent to.
	pathProjectTransfer = pathProject42 + "/transfer"

	// projectInUserNamespace is project 42 where a transfer finds it: in the
	// namespace of user alice, whose id and full path differ from the
	// destination's in both halves, so either matcher could tell them apart.
	projectInUserNamespace = `{"id":42,"name":"moving","path":"moving","path_with_namespace":"alice/moving",` +
		`"namespace":{"id":1,"name":"alice","path":"alice","kind":"user","full_path":"alice"}}`
	// projectInGroupNamespace is project 42 after the move into group newns.
	projectInGroupNamespace = `{"id":42,"name":"moving","path":"moving","path_with_namespace":"newns/moving",` +
		`"namespace":{"id":7,"name":"newns","path":"newns","kind":"group","full_path":"newns"}}`
)

// landingBound is the wait a test that expects the move to land gives it. The
// move lands on the second or third read, a few milliseconds in, so the bound
// is generous for a correct handler. It is only a backstop: a test that
// expects the move to land calls the handler with
// [transferGitLab.landingContext], which ends as soon as the handler reads
// past the scripted reads, so a handler that never sees the move land fails
// the test at that read rather than at the bound.
const landingBound = 2 * time.Second

// transferDate is the Date header a test's transfer answer carries when the
// test dates a to-do item against it, and transferAnsweredAt the same instant.
const transferDate = "Sun, 27 Sep 2026 10:00:00 GMT"

// transferAnsweredAt is the instant [transferDate] names.
var transferAnsweredAt = time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)

// failedTransferTodos is a to-do listing holding the item GitLab leaves when
// the transfer of project 42 to newns fails, created a second after the
// transfer was answered.
var failedTransferTodos = fmt.Sprintf(`[{"id":5,"action_name":"transfer_failed","target_type":"Project","state":"pending",`+
	`"target":{"id":42},"body":"newns/moving","created_at":%q}]`, transferAnsweredAt.Add(time.Second).Format(time.RFC3339))

// fastTransferWait makes Transfer read back every millisecond for at most
// bound, and restores the package's timing when the test ends.
func fastTransferWait(t *testing.T, bound time.Duration) {
	t.Helper()
	oldBound, oldInterval := transferBound, transferInterval
	transferBound, transferInterval = bound, time.Millisecond
	t.Cleanup(func() { transferBound, transferInterval = oldBound, oldInterval })
}

// transferGitLab is a GitLab whose transfer answers with one project and whose
// reads of project 42 answer with the others in order, repeating the last. Its
// to-do listing answers todos, or no items when that is empty, and a non-empty
// date is the Date header of the transfer's answer. A non-empty version is
// what its version endpoint answers, and a non-empty graphql what its GraphQL
// endpoint answers, with graphqlStatus or 200; either asked while empty fails
// the test. It counts the reads, the listings and the GraphQL requests, and
// records the namespace each transfer asked for.
type transferGitLab struct {
	t             *testing.T
	answer        string
	date          string
	reads         []string
	todos         string
	version       string
	graphql       string
	graphqlStatus int
	onRead        func(n int64)
	stop          context.CancelFunc
	readHits      atomic.Int64
	todoHits      atomic.Int64
	graphqlHits   atomic.Int64

	mu         sync.Mutex
	namespaces []string
}

// landingContext is the context a test that expects the move to land calls
// the handler with. It ends when the handler reads the project more times
// than the test scripted, which a correct handler never does, since the last
// scripted read is where the move lands or the failure is reported.
func (g *transferGitLab) landingContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	g.stop = cancel
	return ctx
}

// asked returns the namespaces the transfers asked for, in order.
func (g *transferGitLab) asked() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.namespaces...)
}

// handler is the mock the test client is built on.
func (g *transferGitLab) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == pathProjectTransfer:
			var body struct {
				Namespace string `json:"namespace"`
			}
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &body); err != nil {
				g.t.Errorf("transfer body %s: %v", raw, err)
			}
			g.mu.Lock()
			g.namespaces = append(g.namespaces, body.Namespace)
			g.mu.Unlock()
			if g.date != "" {
				w.Header().Set("Date", g.date)
			}
			testutil.RespondJSON(w, http.StatusOK, g.answer)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/todos":
			g.todoHits.Add(1)
			testutil.RespondJSON(w, http.StatusOK, cmp.Or(g.todos, "[]"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/version" && g.version != "":
			testutil.RespondJSON(w, http.StatusOK, g.version)
		case r.Method == http.MethodPost && r.URL.Path == "/api/graphql" && g.graphql != "":
			g.graphqlHits.Add(1)
			testutil.RespondJSON(w, cmp.Or(g.graphqlStatus, http.StatusOK), g.graphql)
		case r.Method == http.MethodGet && r.URL.Path == pathProject42:
			n := g.readHits.Add(1)
			if g.onRead != nil {
				g.onRead(n)
			}
			if g.stop != nil && n > int64(len(g.reads)) {
				g.stop()
			}
			if len(g.reads) == 0 {
				g.t.Errorf("read %d of project 42, want none", n)
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

// TestTransfer_AnswerAlreadyInTheDestination_IsReturnedWithoutReadingBack
// holds the GitLab 19.3 behavior: the transfer answers after the move, so an
// answer that already shows the destination is the result and nothing is read
// back, whichever way the caller named the namespace.
func TestTransfer_AnswerAlreadyInTheDestination_IsReturnedWithoutReadingBack(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
	}{
		{name: "full path", namespace: "newns"},
		{name: "full path in another case", namespace: "NewNS"},
		{name: "numeric id", namespace: "7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastTransferWait(t, time.Second)
			gitlab := &transferGitLab{t: t, answer: projectInGroupNamespace}
			client := testutil.NewTestClient(t, gitlab.handler())

			out, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: tt.namespace})
			if err != nil {
				t.Fatalf("Transfer() error = %v", err)
			}
			if out.TransferQueued || out.PathWithNamespace != "newns/moving" {
				t.Errorf("Transfer() = queued %v at %q, want applied at newns/moving", out.TransferQueued, out.PathWithNamespace)
			}
			if reads, listings := gitlab.readHits.Load(), gitlab.todoHits.Load(); reads != 0 || listings != 0 {
				t.Errorf("reads = %d and to-do listings = %d, want none: the answer already showed the destination", reads, listings)
			}
			if asked := gitlab.asked(); len(asked) != 1 || asked[0] != tt.namespace {
				t.Errorf("transfer asked for namespaces %q, want exactly %q", asked, tt.namespace)
			}
		})
	}
}

// TestTransfer_MoveLandsLater_AnswersTheMovedProject holds the GitLab 19.4
// behavior: the transfer answers with the project where it still is, the
// first reads find it there, and the answer is the read that found it in the
// destination, not the transfer's own. Each read that did not find it moved
// also asked whether GitLab reported the move failed, and the one that found
// it did not.
func TestTransfer_MoveLandsLater_AnswersTheMovedProject(t *testing.T) {
	fastTransferWait(t, landingBound)
	gitlab := &transferGitLab{t: t, answer: projectInUserNamespace, reads: []string{
		projectInUserNamespace, projectInUserNamespace, projectInGroupNamespace,
	}}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if out.TransferQueued {
		t.Error("Transfer() reported the move queued, want it applied")
	}
	if out.PathWithNamespace != "newns/moving" || out.Namespace == nil || out.Namespace.ID != 7 {
		t.Errorf("Transfer() answered %q in namespace %+v, want newns/moving in namespace 7", out.PathWithNamespace, out.Namespace)
	}
	if got := gitlab.readHits.Load(); got != 3 {
		t.Errorf("reads = %d, want 3: two before the move and the one that found it", got)
	}
	if got := gitlab.todoHits.Load(); got != 2 {
		t.Errorf("to-do listings = %d, want 2: one after each read that did not find the move", got)
	}
}

// npmTransferReason is the reason GitLab 19.5 records when its worker refuses
// to move a project with npm packages scoped to the old top-level namespace,
// and npmReasonAnswer GraphQL's answer for the failure item holding it. The
// reason needs no escaping in JSON.
const (
	npmTransferReason = "Root namespace can't be updated if the project has NPM packages scoped to the current root level namespace."
	npmReasonAnswer   = `{"data":{"todo":{"transferFailureReason":"` + npmTransferReason + `"}}}`
)

// TestTransfer_MoveFailsInTheBackground_AnswersTheFailure holds what GitLab
// does with a transfer its worker refuses: the project stays where it was and
// the only report is a to-do item, so the first read that finds the item ends
// the wait with an error naming the destination rather than a queued answer
// after the whole bound. From 19.5 the item carries the reason, which the
// error quotes ahead of the hint; 19.4 is sent no GraphQL at all, and a read
// of the reason that fails leaves the error 19.4 gets.
func TestTransfer_MoveFailsInTheBackground_AnswersTheFailure(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		graphql       string
		graphqlStatus int
		reason        string
		hint          string
		graphqlHits   int64
	}{
		{name: "19.4", version: `{"version":"19.4.1-ee"}`, hint: hintTransferFailed},
		{
			name: "19.5", version: `{"version":"19.5.0-ee"}`, graphql: npmReasonAnswer,
			reason: npmTransferReason, hint: hintTransferFailedReason, graphqlHits: 1,
		},
		{
			name: "19.5 with the reason unreadable", version: `{"version":"19.5.0-ee"}`,
			graphql: `{"message":"500 Internal Server Error"}`, graphqlStatus: http.StatusInternalServerError,
			hint: hintTransferFailed, graphqlHits: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastTransferWait(t, landingBound)
			gitlab := &transferGitLab{
				t: t, answer: projectInUserNamespace, date: transferDate,
				reads: []string{projectInUserNamespace}, todos: failedTransferTodos,
				version: tt.version, graphql: tt.graphql, graphqlStatus: tt.graphqlStatus,
			}
			client := testutil.NewTestClient(t, gitlab.handler())

			_, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
			if err == nil {
				t.Fatal("Transfer() error = nil, want the failure GitLab reported")
			}
			want := "projectTransfer: GitLab accepted the transfer and then failed it in the background, " +
				"leaving a to-do item that says the transfer to newns/moving failed"
			if tt.reason != "" {
				want += fmt.Sprintf(", for this reason: %q", tt.reason)
			}
			want += ". Suggestion: " + tt.hint
			if msg := err.Error(); msg != want {
				t.Errorf("Transfer() error =\n%q\nwant\n%q", msg, want)
			}
			if reads, listings := gitlab.readHits.Load(), gitlab.todoHits.Load(); reads != 1 || listings != 1 {
				t.Errorf("reads = %d and to-do listings = %d, want 1 each: the first listing found the failure", reads, listings)
			}
			if got := gitlab.graphqlHits.Load(); got != tt.graphqlHits {
				t.Errorf("GraphQL requests = %d, want %d", got, tt.graphqlHits)
			}
		})
	}
}

// TestTransfer_ReasonPrecedesTheSuggestion verifies that GitLab's reason is
// read before the hint, so a caller meets what GitLab said first and this
// server's advice after it.
func TestTransfer_ReasonPrecedesTheSuggestion(t *testing.T) {
	fastTransferWait(t, landingBound)
	gitlab := &transferGitLab{
		t: t, answer: projectInUserNamespace, date: transferDate,
		reads: []string{projectInUserNamespace}, todos: failedTransferTodos,
		version: `{"version":"19.5.0-ee"}`, graphql: npmReasonAnswer,
	}
	client := testutil.NewTestClient(t, gitlab.handler())

	_, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err == nil {
		t.Fatal("Transfer() error = nil, want the failure GitLab reported")
	}
	msg := err.Error()
	reason, suggestion := strings.Index(msg, npmTransferReason), strings.Index(msg, "Suggestion:")
	if reason < 0 || suggestion < 0 || reason > suggestion {
		t.Errorf("Transfer() error = %q, want GitLab's reason ahead of the suggestion", msg)
	}
}

// TestTransfer_ItemFromAnEarlierFailure_IsNotThisOne verifies that a pending
// item GitLab left for an earlier failed transfer of the project does not end
// the wait: the move of this transfer lands on the next read.
func TestTransfer_ItemFromAnEarlierFailure_IsNotThisOne(t *testing.T) {
	fastTransferWait(t, landingBound)
	earlier := fmt.Sprintf(`[{"id":4,"target":{"id":42},"body":"newns/moving","created_at":%q}]`,
		transferAnsweredAt.Add(-time.Hour).Format(time.RFC3339))
	gitlab := &transferGitLab{
		t: t, answer: projectInUserNamespace, date: transferDate,
		reads: []string{projectInUserNamespace, projectInGroupNamespace}, todos: earlier,
	}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err != nil {
		t.Fatalf("Transfer() error = %v, want the move that landed", err)
	}
	if out.TransferQueued || out.PathWithNamespace != "newns/moving" {
		t.Errorf("Transfer() = queued %v at %q, want applied at newns/moving", out.TransferQueued, out.PathWithNamespace)
	}
}

// TestTransfer_ReadByPath_ReadsBackByTheAnsweredID verifies that the read
// back names the project by the id the transfer answered with, not by the
// path the caller named, which is the path the move is taking away.
func TestTransfer_ReadByPath_ReadsBackByTheAnsweredID(t *testing.T) {
	fastTransferWait(t, landingBound)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var readPath atomic.Value
	var reads atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, projectInUserNamespace)
		default:
			// The first read finds the move, so a second one is a handler
			// that did not see it, which ends the call rather than the bound.
			if reads.Add(1) > 1 {
				stop()
			}
			readPath.Store(r.URL.Path)
			testutil.RespondJSON(w, http.StatusOK, projectInGroupNamespace)
		}
	}))

	out, err := Transfer(ctx, nil, client, TransferInput{ProjectID: "alice/moving", Namespace: "newns"})
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if out.TransferQueued {
		t.Error("Transfer() reported the move queued, want it applied")
	}
	if got, _ := readPath.Load().(string); got != pathProject42 {
		t.Errorf("read back %q, want %q", got, pathProject42)
	}
}

// TestTransfer_MoveNeverLands_AnswersQueuedWithTheTransferAnswer verifies
// that a move the wait never sees land is answered, without error, with the
// project where the transfer's own answer placed it and TransferQueued set:
// GitLab accepted the transfer, so it is not a failure to report.
func TestTransfer_MoveNeverLands_AnswersQueuedWithTheTransferAnswer(t *testing.T) {
	fastTransferWait(t, 50*time.Millisecond)
	gitlab := &transferGitLab{t: t, answer: projectInUserNamespace, reads: []string{projectInUserNamespace}}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err != nil {
		t.Fatalf("Transfer() error = %v, want the queued answer", err)
	}
	if !out.TransferQueued {
		t.Error("Transfer() did not report the move queued")
	}
	if out.ID != 42 || out.PathWithNamespace != "alice/moving" {
		t.Errorf("Transfer() answered project %d at %q, want 42 at alice/moving", out.ID, out.PathWithNamespace)
	}
	if gitlab.readHits.Load() == 0 {
		t.Error("reads = 0, want the handler to have read the project back")
	}
}

// TestTransfer_ReadFails_KeepsWaiting verifies that a read GitLab fails to
// serve does not end the wait: the next read finds the moved project.
func TestTransfer_ReadFails_KeepsWaiting(t *testing.T) {
	fastTransferWait(t, landingBound)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var reads, listings atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, projectInUserNamespace)
		case r.URL.Path == "/api/v4/todos":
			listings.Add(1)
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		default:
			switch reads.Add(1) {
			case 1:
				testutil.RespondJSON(w, http.StatusBadGateway, `{"message":"502 Bad Gateway"}`)
			case 2:
				testutil.RespondJSON(w, http.StatusOK, projectInGroupNamespace)
			default:
				stop()
				testutil.RespondJSON(w, http.StatusOK, projectInGroupNamespace)
			}
		}
	}))

	out, err := Transfer(ctx, nil, client, TransferInput{ProjectID: "42", Namespace: "7"})
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if out.TransferQueued || out.PathWithNamespace != "newns/moving" {
		t.Errorf("Transfer() = queued %v at %q, want applied at newns/moving", out.TransferQueued, out.PathWithNamespace)
	}
	if got := reads.Load(); got != 2 {
		t.Errorf("reads = %d, want 2: the failed one and the one that found the move", got)
	}
	if got := listings.Load(); got != 0 {
		t.Errorf("to-do listings = %d, want none: a failed read says nothing about the move", got)
	}
}

// TestTransfer_ListingFails_KeepsWaiting verifies that a to-do listing GitLab
// fails to serve does not end the wait either: it is not a report that the
// move failed, and the next read finds the project moved.
func TestTransfer_ListingFails_KeepsWaiting(t *testing.T) {
	fastTransferWait(t, landingBound)
	var listings atomic.Int64
	gitlab := &transferGitLab{t: t, answer: projectInUserNamespace, reads: []string{projectInUserNamespace, projectInGroupNamespace}}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/todos" {
			listings.Add(1)
			testutil.RespondJSON(w, http.StatusBadGateway, `{"message":"502 Bad Gateway"}`)
			return
		}
		gitlab.handler()(w, r)
	}))

	out, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if out.TransferQueued || out.PathWithNamespace != "newns/moving" || listings.Load() != 1 {
		t.Errorf("Transfer() = queued %v at %q after %d listings, want applied at newns/moving after one", out.TransferQueued, out.PathWithNamespace, listings.Load())
	}
}

// TestTransfer_CallerGoesAwayDuringTheWait_AnswersTheContextError verifies
// that the wait honors the caller's context: once it ends, the handler
// returns its error rather than a queued answer nobody is waiting for.
func TestTransfer_CallerGoesAwayDuringTheWait_AnswersTheContextError(t *testing.T) {
	fastTransferWait(t, landingBound)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gitlab := &transferGitLab{t: t, answer: projectInUserNamespace, reads: []string{projectInUserNamespace}}
	gitlab.onRead = func(int64) { cancel() }
	client := testutil.NewTestClient(t, gitlab.handler())

	_, err := Transfer(ctx, nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Transfer() error = %v, want context.Canceled", err)
	}
}

// TestTransfer_AnswerWithoutNamespace_IsReadBack verifies that an answer
// carrying no namespace is not taken for the destination: nothing in it says
// where the project is, so the handler reads it back.
func TestTransfer_AnswerWithoutNamespace_IsReadBack(t *testing.T) {
	fastTransferWait(t, landingBound)
	gitlab := &transferGitLab{
		t:      t,
		answer: `{"id":42,"name":"moving","path":"moving","path_with_namespace":"alice/moving"}`,
		reads:  []string{projectInGroupNamespace},
	}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if out.TransferQueued || gitlab.readHits.Load() != 1 {
		t.Errorf("Transfer() = queued %v after %d reads, want applied after one", out.TransferQueued, gitlab.readHits.Load())
	}
}

// TestTransfer_NumericNamespace_MatchesByIDNotByPath verifies that a numeric
// namespace is read as an id: an answer in namespace 7 has landed even though
// its path is not "7", and one whose path is "7" but whose id is not has not.
func TestTransfer_NumericNamespace_MatchesByIDNotByPath(t *testing.T) {
	pathSeven := `{"id":42,"name":"moving","path":"moving","path_with_namespace":"7/moving",` +
		`"namespace":{"id":3,"name":"7","path":"7","kind":"group","full_path":"7"}}`
	fastTransferWait(t, landingBound)
	gitlab := &transferGitLab{t: t, answer: pathSeven, reads: []string{pathSeven, projectInGroupNamespace}}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(gitlab.landingContext(t), nil, client, TransferInput{ProjectID: "42", Namespace: "7"})
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if out.Namespace == nil || out.Namespace.ID != 7 {
		t.Errorf("Transfer() answered namespace %+v, want the one whose id is 7", out.Namespace)
	}
	if got := gitlab.readHits.Load(); got != 2 {
		t.Errorf("reads = %d, want 2: namespace 3 with path 7 is not namespace 7", got)
	}
}

// TestTransfer_AnswerThatDoesNotDecode_IsAnError verifies that a transfer
// answer the captured-field type cannot decode is reported rather than
// returned half-read, and that no wait starts on it.
func TestTransfer_AnswerThatDoesNotDecode_IsAnError(t *testing.T) {
	fastTransferWait(t, time.Second)
	gitlab := &transferGitLab{t: t, answer: `{"id":42,"name":"moving","description_html":5}`}
	client := testutil.NewTestClient(t, gitlab.handler())

	_, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err == nil || !strings.Contains(err.Error(), "projectTransfer") {
		t.Fatalf("Transfer() error = %v, want the projectTransfer decode failure", err)
	}
}

// TestTransfer_TransferRequest_CarriesTheCallersContext verifies that the
// transfer request is sent with the caller's context, so an abandoned call
// cancels it.
func TestTransfer_TransferRequest_CarriesTheCallersContext(t *testing.T) {
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectInGroupNamespace)
	})

	_, err := Transfer(ctx, nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Transfer() error = %v, want context.Canceled", err)
	}
}

// TestTransfer_MissingInput_IsRefusedBeforeAnyRequest verifies both required
// inputs are checked before GitLab is asked anything.
func TestTransfer_MissingInput_IsRefusedBeforeAnyRequest(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	tests := []struct {
		name  string
		input TransferInput
		want  string
	}{
		{name: "project", input: TransferInput{Namespace: "newns"}, want: "project_id is required"},
		{name: "namespace", input: TransferInput{ProjectID: "42"}, want: "namespace is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transfer(context.Background(), nil, client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Transfer() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// allTransferHints is every hint a refused or failed transfer can carry, so a
// test can hold that a refusal carries its own hint and none of the others.
var allTransferHints = []string{
	hintTransferPermission, hintTransferNamespace, hintTransferNotFound, hintTransferUnderWay,
	hintTransferInPlace, hintTransferCollision, hintTransferPendingDeletion, hintTransferRefused,
	hintTransferFailed, hintTransferFailedReason,
}

// TestTransfer_Refusal_IsHintedByWhatGitLabSaid drives each refusal GitLab
// gives a transfer (app/services/projects/transfer_service.rb and the route's
// own) and holds the hint each gets. A transfer that cannot start is told it
// may be running or the project marked for deletion, a project already in the
// namespace that there is nothing left to move, both to read the project
// back, a name or path collision to rename, and a name or path a project
// pending deletion still holds, which an instance that transfers inline
// refuses with the request, to wait for that deletion or finish it. A 404 is
// hinted by which of the two lookups failed.
func TestTransfer_Refusal_IsHintedByWhatGitLabSaid(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		want    string
	}{
		{
			name: "transfer already under way", status: http.StatusBadRequest,
			message: "Unable to initiate transfer. The project may already have a transfer in progress.", want: hintTransferUnderWay,
		},
		{
			name: "already in the namespace", status: http.StatusBadRequest,
			message: "Project is already in this namespace.", want: hintTransferInPlace,
		},
		{
			name: "name or path taken", status: http.StatusBadRequest,
			message: "Failed to transfer project {:new_namespace=>[\"Project with same name or path in target namespace already exists\"]}", want: hintTransferCollision,
		},
		{
			name: "name or path pending deletion", status: http.StatusBadRequest,
			message: "A project with the same name or path was recently deleted in the target namespace. Please wait for the deletion to complete or permanently delete it first.", want: hintTransferPendingDeletion,
		},
		{
			name: "no permission on the project", status: http.StatusBadRequest,
			message: "You don't have permission to transfer this project.", want: hintTransferPermission,
		},
		{
			name: "no permission on the namespace", status: http.StatusBadRequest,
			message: "You don't have permission to transfer projects into that namespace.", want: hintTransferPermission,
		},
		{
			name: "another refusal", status: http.StatusBadRequest,
			message: "Please select a new namespace for your project.", want: hintTransferRefused,
		},
		{name: "forbidden", status: http.StatusForbidden, message: "403 Forbidden", want: hintTransferPermission},
		{name: "namespace not found", status: http.StatusNotFound, message: "404 Namespace Not Found", want: hintTransferNamespace},
		{name: "project not found", status: http.StatusNotFound, message: "404 Project Not Found", want: hintTransferNotFound},
		{name: "not found, with no message", status: http.StatusNotFound, want: hintTransferNamespace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, fmt.Sprintf(`{"message":%q}`, tt.message))
			}))
			_, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
			if err == nil {
				t.Fatal("Transfer() error = nil, want the refusal")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Transfer() error = %q, want the hint %q", err, tt.want)
			}
			for _, other := range allTransferHints {
				if other != tt.want && strings.Contains(err.Error(), other) {
					t.Errorf("Transfer() error = %q, carries the hint %q as well", err, other)
				}
			}
		})
	}
}

// TestTransfer_NotFoundWithoutAJSONBody_IsHintedAsTheNamespace verifies that
// a 404 whose body is not GitLab's JSON, a proxy's page for instance, names
// neither lookup and is hinted as the namespace, the one a caller most often
// gets wrong, rather than failing to hint at all.
func TestTransfer_NotFoundWithoutAJSONBody_IsHintedAsTheNamespace(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<html>Project Not Found</html>")
	}))
	_, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err == nil || !strings.Contains(err.Error(), hintTransferNamespace) {
		t.Errorf("Transfer() error = %v, want the namespace hint", err)
	}
}

// TestTransfer_RefusalUnderAnotherStatus_CarriesNoTransferHint verifies the
// status is part of the match: GitLab's words under a status other than the
// 400 it refuses a transfer with are not answered as that refusal, and a
// failure that is no refusal at all carries GitLab's message and no hint.
func TestTransfer_RefusalUnderAnotherStatus_CarriesNoTransferHint(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "unprocessable", status: http.StatusUnprocessableEntity},
		{name: "server error", status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"The project may already have a transfer in progress."}`)
			}))
			_, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
			if err == nil {
				t.Fatal("Transfer() error = nil, want the failure")
			}
			for _, hint := range allTransferHints {
				if strings.Contains(err.Error(), hint) {
					t.Errorf("Transfer() error = %q, want no transfer hint under status %d", err, tt.status)
				}
			}
			if !strings.Contains(err.Error(), "transfer in progress") {
				t.Errorf("Transfer() error = %q, want GitLab's own message kept", err)
			}
		})
	}
}

// TestTransferHints_NameWhatFollowingThemTakes holds two hints to what acting
// on them needs. Deleting the project that holds the name or path for good
// takes full_path set to its path_with_namespace beside permanently_remove,
// which GitLab refuses the delete without (lib/api/projects.rb). A failure
// GitLab gives a reason for is read back with project.get before the transfer
// is sent again, and a reason saying the transfer failed unexpectedly is one
// GitLab's worker raises again so Sidekiq retries it, up to three times
// (Projects::TransferWorker, retry: 3), which a second transfer would race.
func TestTransferHints_NameWhatFollowingThemTakes(t *testing.T) {
	tests := []struct {
		name string
		hint string
		want []string
	}{
		{
			name: "pending deletion", hint: hintTransferPendingDeletion,
			want: []string{"project.delete", "permanently_remove", "full_path", "path_with_namespace"},
		},
		{
			name: "failed with a reason", hint: hintTransferFailedReason,
			want: []string{"project.get", "failed unexpectedly", "retries"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, word := range tt.want {
				if !strings.Contains(tt.hint, word) {
					t.Errorf("hint %q does not name %q", tt.hint, word)
				}
			}
		})
	}
}

// TestTransferTiming_IsWaitpolls verifies the handler waits with the shared
// transfer timing, and that the metadata a model reads states the bound the
// handler waits for.
func TestTransferTiming_IsWaitpolls(t *testing.T) {
	if transferBound != waitpoll.TransferBound || transferInterval != waitpoll.TransferInterval {
		t.Errorf("timing = %v every %v, want waitpoll's %v every %v", transferBound, transferInterval, waitpoll.TransferBound, waitpoll.TransferInterval)
	}
	stated := fmt.Sprintf("%d seconds", int(waitpoll.TransferBound/time.Second))
	meta := projectActionMeta["gitlab_project_transfer"]
	if !strings.Contains(meta.usage, stated) || !strings.Contains(meta.description, stated) {
		t.Errorf("project.transfer metadata does not state the %s it waits:\nusage: %s\ndescription: %s", stated, meta.usage, meta.description)
	}
}

// TestFormatTransferMarkdown_Applied_IsTheProjectCard verifies that a move
// that landed renders as the project's own card, in its new namespace.
func TestFormatTransferMarkdown_Applied_IsTheProjectCard(t *testing.T) {
	out := TransferOutput{
		ID: 42, Name: "moving", PathWithNamespace: "newns/moving",
		Namespace: &NamespaceOutput{ID: 7, FullPath: "newns"},
	}

	if got, want := FormatTransferMarkdown(out), FormatMarkdown(out.Output); got != want {
		t.Errorf("applied transfer card:\n got %q\nwant %q", got, want)
	}
}

// TestFormatTransferMarkdown_Queued_SaysSoAndPointsAtTheReads verifies the
// card of a move the wait did not see land, byte for byte: the heading says
// it is queued, the rows are where the project still is, the note says why
// and not to send it again, and the hints are the reads that settle it.
func TestFormatTransferMarkdown_Queued_SaysSoAndPointsAtTheReads(t *testing.T) {
	out := TransferOutput{
		TransferQueued: true, ID: 42, Name: "moving", PathWithNamespace: "alice/moving",
		Namespace: &NamespaceOutput{ID: 1, FullPath: "alice"},
	}

	want := "## Project transfer queued: moving\n\n" +
		"- **ID**: 42\n" +
		"- **Path**: alice/moving\n" +
		"- **Namespace**: alice\n" +
		"\n" + transferQueuedNote + "\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'project.get' to read the project back and see which namespace it is in\n" +
		"- Use action 'user.todo_list' to see the to-do item GitLab leaves if the move fails\n"
	if got := FormatTransferMarkdown(out); got != want {
		t.Errorf("queued transfer card:\n got %q\nwant %q", got, want)
	}
	if hints := toolutil.ExtractHints(FormatTransferMarkdown(out)); len(hints) != 2 {
		t.Errorf("hints = %q, want the two reads", hints)
	}
}
