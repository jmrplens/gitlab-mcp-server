package projects

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

// fastTransferWait makes Transfer read back every millisecond for at most
// bound, and restores the package's timing when the test ends.
func fastTransferWait(t *testing.T, bound time.Duration) {
	t.Helper()
	oldBound, oldInterval := transferBound, transferInterval
	transferBound, transferInterval = bound, time.Millisecond
	t.Cleanup(func() { transferBound, transferInterval = oldBound, oldInterval })
}

// transferGitLab is a GitLab whose transfer answers with one project and whose
// reads of project 42 answer with the others in order, repeating the last. It
// counts the reads and records the namespace each transfer asked for.
type transferGitLab struct {
	t        *testing.T
	answer   string
	reads    []string
	onRead   func(n int64)
	readHits atomic.Int64

	mu         sync.Mutex
	namespaces []string
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
			testutil.RespondJSON(w, http.StatusOK, g.answer)
		case r.Method == http.MethodGet && r.URL.Path == pathProject42:
			n := g.readHits.Add(1)
			if g.onRead != nil {
				g.onRead(n)
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

			out, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: tt.namespace})
			if err != nil {
				t.Fatalf("Transfer() error = %v", err)
			}
			if out.TransferQueued || out.PathWithNamespace != "newns/moving" {
				t.Errorf("Transfer() = queued %v at %q, want applied at newns/moving", out.TransferQueued, out.PathWithNamespace)
			}
			if got := gitlab.readHits.Load(); got != 0 {
				t.Errorf("reads = %d, want none: the answer already showed the destination", got)
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
// destination, not the transfer's own.
func TestTransfer_MoveLandsLater_AnswersTheMovedProject(t *testing.T) {
	fastTransferWait(t, 10*time.Second)
	gitlab := &transferGitLab{t: t, answer: projectInUserNamespace, reads: []string{
		projectInUserNamespace, projectInUserNamespace, projectInGroupNamespace,
	}}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
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
}

// TestTransfer_ReadByPath_ReadsBackByTheAnsweredID verifies that the read
// back names the project by the id the transfer answered with, not by the
// path the caller named, which is the path the move is taking away.
func TestTransfer_ReadByPath_ReadsBackByTheAnsweredID(t *testing.T) {
	fastTransferWait(t, 10*time.Second)
	var readPath atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, projectInUserNamespace)
		default:
			readPath.Store(r.URL.Path)
			testutil.RespondJSON(w, http.StatusOK, projectInGroupNamespace)
		}
	}))

	out, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "alice/moving", Namespace: "newns"})
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
	fastTransferWait(t, 10*time.Second)
	var reads atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, projectInUserNamespace)
			return
		}
		if reads.Add(1) == 1 {
			testutil.RespondJSON(w, http.StatusBadGateway, `{"message":"502 Bad Gateway"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, projectInGroupNamespace)
	}))

	out, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "7"})
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if out.TransferQueued || out.PathWithNamespace != "newns/moving" {
		t.Errorf("Transfer() = queued %v at %q, want applied at newns/moving", out.TransferQueued, out.PathWithNamespace)
	}
	if got := reads.Load(); got != 2 {
		t.Errorf("reads = %d, want 2: the failed one and the one that found the move", got)
	}
}

// TestTransfer_CallerGoesAwayDuringTheWait_AnswersTheContextError verifies
// that the wait honors the caller's context: once it ends, the handler
// returns its error rather than a queued answer nobody is waiting for.
func TestTransfer_CallerGoesAwayDuringTheWait_AnswersTheContextError(t *testing.T) {
	fastTransferWait(t, 10*time.Second)
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
	fastTransferWait(t, 10*time.Second)
	gitlab := &transferGitLab{
		t:      t,
		answer: `{"id":42,"name":"moving","path":"moving","path_with_namespace":"alice/moving"}`,
		reads:  []string{projectInGroupNamespace},
	}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "newns"})
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
	fastTransferWait(t, 10*time.Second)
	gitlab := &transferGitLab{t: t, answer: pathSeven, reads: []string{pathSeven, projectInGroupNamespace}}
	client := testutil.NewTestClient(t, gitlab.handler())

	out, err := Transfer(context.Background(), nil, client, TransferInput{ProjectID: "42", Namespace: "7"})
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

// TestTransfer_Refusal_IsHintedByWhatGitLabSaid drives each refusal GitLab
// gives a transfer (app/services/projects/transfer_service.rb and the route's
// own) and holds the hint each gets. A transfer under way and a project
// already in the namespace are told to wait and read the project back, and
// only a name or path collision is told to rename.
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
			message: "Project is already in this namespace.", want: hintTransferUnderWay,
		},
		{
			name: "name or path taken", status: http.StatusBadRequest,
			message: "Failed to transfer project {:new_namespace=>[\"Project with same name or path in target namespace already exists\"]}", want: hintTransferCollision,
		},
		{
			name: "name or path pending deletion", status: http.StatusBadRequest,
			message: "A project with the same name or path was recently deleted in the target namespace. Please wait for the deletion to complete or permanently delete it first.", want: hintTransferCollision,
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
			for _, other := range []string{hintTransferPermission, hintTransferNamespace, hintTransferUnderWay, hintTransferCollision, hintTransferRefused} {
				if other != tt.want && strings.Contains(err.Error(), other) {
					t.Errorf("Transfer() error = %q, carries the hint %q as well", err, other)
				}
			}
		})
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
			for _, hint := range []string{hintTransferPermission, hintTransferNamespace, hintTransferUnderWay, hintTransferCollision, hintTransferRefused} {
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
