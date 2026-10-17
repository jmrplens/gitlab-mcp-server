package waitpoll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// transferAnsweredAt is the instant the tests' transfers were answered at.
var transferAnsweredAt = time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)

// transferFailureTodo is a pending transfer_failed item about the object with
// targetID, created offset from [transferAnsweredAt] and naming destination.
// Its own id is [todoIDFor] the target, so a test can tell which item was
// taken.
func transferFailureTodo(targetID int64, offset time.Duration, destination string) string {
	return fmt.Sprintf(`{"id":%d,"action_name":"transfer_failed","target_type":"Project","state":"pending",`+
		`"target":{"id":%d},"body":%q,"created_at":%q}`,
		todoIDFor(targetID), targetID, destination, transferAnsweredAt.Add(offset).Format(time.RFC3339Nano))
}

// todoIDFor is the id of the item [transferFailureTodo] writes about the
// object with targetID.
func todoIDFor(targetID int64) int64 { return 1000 + targetID }

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

	failure, failed, err := TransferFailed(context.Background(), client, TransferTarget{ID: 42}, time.Time{})
	if err != nil || failed || failure != (TransferFailure{}) {
		t.Errorf("TransferFailed() = %+v, %v, %v, want nothing reported and no error", failure, failed, err)
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

			failure, failed, err := TransferFailed(context.Background(), client, TransferTarget{ID: 42}, transferAnsweredAt)
			if err != nil {
				t.Fatalf("TransferFailed() error = %v", err)
			}
			if failed != tt.failed {
				t.Errorf("TransferFailed() failed = %v, want %v", failed, tt.failed)
			}
			want := map[bool]TransferFailure{true: {TodoID: todoIDFor(42), Destination: "newns/moving"}}[tt.failed]
			if failure != want {
				t.Errorf("TransferFailed() failure = %+v, want %+v", failure, want)
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

// npmReason is the reason GitLab 19.5 records when its worker refuses to move
// a project with npm packages scoped to the old top-level namespace
// (Projects::TransferService#transfer).
const npmReason = "Root namespace can't be updated if the project has NPM packages scoped to the current root level namespace."

// version195 and version194 are the version endpoint's answers of the first
// release that keeps the reason and of the one before it.
const (
	version195 = `{"version":"19.5.0-ee"}`
	version194 = `{"version":"19.4.1-ee"}`
)

// reasonData is GraphQL's data for a to-do item whose reason is reason,
// written as a JSON string.
func reasonData(t *testing.T, reason string) string {
	t.Helper()
	encoded, err := json.Marshal(reason)
	if err != nil {
		t.Fatalf("encoding the reason %q: %v", reason, err)
	}
	return `{"data":{"todo":{"transferFailureReason":` + string(encoded) + `}}}`
}

// noReasonData is GraphQL's data for a to-do item that holds no reason, which
// is what a read finds between the worker creating the item and storing the
// reason.
const noReasonData = `{"data":{"todo":{"transferFailureReason":null}}}`

// retryAfter sets the pause before the item is read once more for one test.
func retryAfter(t *testing.T, pause time.Duration) {
	t.Helper()
	previous := transferReasonRetry
	transferReasonRetry = pause
	t.Cleanup(func() { transferReasonRetry = previous })
}

// reasonGitLab answers the version endpoint and the GraphQL endpoint, each
// with a status and a body, and records what the GraphQL requests sent. A
// version status of 0 fails the test if the version is asked for, and so does
// a GraphQL status of 0 if a document is sent. A laterGraphqlBody answers
// every GraphQL request after the first in graphqlBody's place.
type reasonGitLab struct {
	t                *testing.T
	versionStatus    int
	versionBody      string
	graphqlStatus    int
	graphqlBody      string
	laterGraphqlBody string
	versionHits      atomic.Int64
	graphqlHits      atomic.Int64
	variables        atomic.Value
}

// handler is the mock the test client is built on.
func (g *reasonGitLab) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/version" && g.versionStatus != 0:
			g.versionHits.Add(1)
			testutil.RespondJSON(w, g.versionStatus, g.versionBody)
		case r.Method == http.MethodPost && r.URL.Path == "/api/graphql" && g.graphqlStatus != 0:
			body := g.graphqlBody
			if g.graphqlHits.Add(1) > 1 && g.laterGraphqlBody != "" {
				body = g.laterGraphqlBody
			}
			variables, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				g.t.Errorf("GraphQL body: %v", err)
			}
			g.variables.Store(variables)
			testutil.RespondJSON(w, g.graphqlStatus, body)
		default:
			g.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

// sentVariables returns the variables of the last GraphQL request.
func (g *reasonGitLab) sentVariables() map[string]any {
	variables, _ := g.variables.Load().(map[string]any)
	return variables
}

// TestTransferFailureReason_NineteenFiveOrLater_ReadsTheItemsReason verifies
// that an instance that keeps the reason, a release of 19.5 or a prerelease of
// a later one, is asked for the reason of the item by its global id, and that
// the reason it gives is what the call returns.
func TestTransferFailureReason_NineteenFiveOrLater_ReadsTheItemsReason(t *testing.T) {
	for _, version := range []string{version195, `{"version":"19.6.0-pre"}`} {
		t.Run(version, func(t *testing.T) {
			gitlab := &reasonGitLab{
				t: t, versionStatus: http.StatusOK, versionBody: version,
				graphqlStatus: http.StatusOK, graphqlBody: reasonData(t, npmReason),
			}
			client := testutil.NewTestClient(t, gitlab.handler())

			if got := TransferFailureReason(t.Context(), client, 1042); got != npmReason {
				t.Errorf("TransferFailureReason() = %q, want %q", got, npmReason)
			}
			if got := gitlab.sentVariables()["id"]; got != "gid://gitlab/Todo/1042" {
				t.Errorf("the document was sent id %v, want the item's global id gid://gitlab/Todo/1042", got)
			}
		})
	}
}

// TestTransferFailureReason_OlderInstance_AsksNothing verifies that an
// instance known to run a release before 19.5, which refuses a document naming
// a field it does not have, is sent no document at all, whether its version
// is read now or was read before.
func TestTransferFailureReason_OlderInstance_AsksNothing(t *testing.T) {
	t.Run("read now", func(t *testing.T) {
		gitlab := &reasonGitLab{t: t, versionStatus: http.StatusOK, versionBody: version194}
		client := testutil.NewTestClient(t, gitlab.handler())

		if got := TransferFailureReason(t.Context(), client, 1042); got != "" {
			t.Errorf("TransferFailureReason() = %q, want none from 19.4", got)
		}
		if got := gitlab.versionHits.Load(); got != 1 {
			t.Errorf("the version was read %d times, want once", got)
		}
	})
	t.Run("read before", func(t *testing.T) {
		gitlab := &reasonGitLab{t: t, versionStatus: http.StatusOK, versionBody: version194}
		client := testutil.NewTestClient(t, gitlab.handler())
		if _, answered := client.ReadVersion(t.Context()); !answered {
			t.Fatal("the stand-in's version was not read")
		}

		if got := TransferFailureReason(t.Context(), client, 1042); got != "" {
			t.Errorf("TransferFailureReason() = %q, want none from 19.4", got)
		}
		if got := gitlab.versionHits.Load(); got != 1 {
			t.Errorf("the version was read %d times, want once, before the call", got)
		}
	})
}

// TestTransferFailureReason_ReleaseNotKnown_AsksAnyway verifies that an
// instance whose release this server cannot tell is asked for the reason: one
// whose version endpoint did not answer, one that refused the token Metadata:
// Read, which a fine-grained token without it is answered, and one that named
// something that is not a version. A release before 19.5 refuses the document,
// which costs the call nothing but the reason.
func TestTransferFailureReason_ReleaseNotKnown_AsksAnyway(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "no answer", status: http.StatusBadGateway, body: `{"message":"502 Bad Gateway"}`},
		{
			name: "Metadata: Read refused", status: http.StatusForbidden,
			body: `{"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata: Read]."}`,
		},
		{name: "not a version", status: http.StatusOK, body: `{"version":"latest and greatest"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitlab := &reasonGitLab{
				t: t, versionStatus: tt.status, versionBody: tt.body,
				graphqlStatus: http.StatusOK, graphqlBody: reasonData(t, npmReason),
			}
			client := testutil.NewTestClient(t, gitlab.handler())

			if got := TransferFailureReason(t.Context(), client, 1042); got != npmReason {
				t.Errorf("TransferFailureReason() = %q, want %q", got, npmReason)
			}
			if got := gitlab.graphqlHits.Load(); got != 1 {
				t.Errorf("GraphQL was asked %d times, want once", got)
			}
		})
	}
}

// TestTransferFailureReason_NoReason_IsSilent drives every way the read can
// give no reason on an instance that keeps one, and holds that each is the
// empty string rather than an error a caller would report in place of the
// failed transfer, and that none of them is read again: GitLab failing the
// request, a release that does not know the field after all, a fine-grained
// token refused the item, and an item that is gone. An item that holds no
// reason is read once more, which
// TestTransferFailureReason_ItemWithoutAReasonYet_IsReadOnceMore holds.
func TestTransferFailureReason_NoReason_IsSilent(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "a server error", status: http.StatusInternalServerError, body: `{"message":"500 Internal Server Error"}`},
		{
			name: "an unknown field", status: http.StatusOK,
			body: `{"errors":[{"message":"Field 'transferFailureReason' doesn't exist on type 'Todo'","locations":[{"line":3,"column":5}]}]}`,
		},
		{
			name: "a fine-grained refusal", status: http.StatusOK,
			body: `{"data":{"todo":null},"errors":[{"message":"Access denied: This operation requires a fine-grained personal access token with the following user permissions: [Todo: Read]."}]}`,
		},
		{name: "no item", status: http.StatusOK, body: `{"data":{"todo":null}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryAfter(t, time.Millisecond)
			gitlab := &reasonGitLab{
				t: t, versionStatus: http.StatusOK, versionBody: version195,
				graphqlStatus: tt.status, graphqlBody: tt.body, laterGraphqlBody: reasonData(t, npmReason),
			}
			client := testutil.NewTestClient(t, gitlab.handler())

			if got := TransferFailureReason(t.Context(), client, 1042); got != "" {
				t.Errorf("TransferFailureReason() = %q, want none", got)
			}
			if got := gitlab.graphqlHits.Load(); got != 1 {
				t.Errorf("GraphQL was asked %d times, want once", got)
			}
		})
	}
}

// TestTransferFailureReason_ItemWithoutAReasonYet_IsReadOnceMore verifies
// that an item GitLab answers with no reason is read once more after a pause,
// and no more. GitLab's worker creates the item and stores the reason a moment
// later, in two writes (Projects::TransferWorker and
// Namespaces::Groups::TransferWorker at v19.5.0-ee), so a read between them
// finds the item without the reason; the second answer is what the call
// returns, whether it carries the reason or still none.
func TestTransferFailureReason_ItemWithoutAReasonYet_IsReadOnceMore(t *testing.T) {
	tests := []struct {
		name  string
		later string
		want  string
	}{
		{name: "the reason stored meanwhile", later: reasonData(t, npmReason), want: npmReason},
		{name: "still no reason", later: noReasonData, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryAfter(t, time.Millisecond)
			gitlab := &reasonGitLab{
				t: t, versionStatus: http.StatusOK, versionBody: version195,
				graphqlStatus: http.StatusOK, graphqlBody: noReasonData, laterGraphqlBody: tt.later,
			}
			client := testutil.NewTestClient(t, gitlab.handler())

			if got := TransferFailureReason(t.Context(), client, 1042); got != tt.want {
				t.Errorf("TransferFailureReason() = %q, want %q", got, tt.want)
			}
			if got := gitlab.graphqlHits.Load(); got != 2 {
				t.Errorf("GraphQL was asked %d times, want twice", got)
			}
		})
	}
}

// TestTransferFailureReason_CallerGoesAwayDuringThePause_ReadsNoMore verifies
// that a call whose context ends while it waits to read an item without a
// reason once more answers no reason at once, and does not read the item
// again: the pause is an hour, and the context ends half a second in.
func TestTransferFailureReason_CallerGoesAwayDuringThePause_ReadsNoMore(t *testing.T) {
	retryAfter(t, time.Hour)
	gitlab := &reasonGitLab{
		t: t, versionStatus: http.StatusOK, versionBody: version195,
		graphqlStatus: http.StatusOK, graphqlBody: noReasonData, laterGraphqlBody: reasonData(t, npmReason),
	}
	client := testutil.NewTestClient(t, gitlab.handler())
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	if got := TransferFailureReason(ctx, client, 1042); got != "" {
		t.Errorf("TransferFailureReason() = %q, want none from a call that ended during the pause", got)
	}
	if got := gitlab.graphqlHits.Load(); got != 1 {
		t.Errorf("GraphQL was asked %d times, want once", got)
	}
}

// TestTransferFailureReason_CleansTheReason verifies the reason is quoted as
// one line: control characters and runs of white space are one space, the
// ends are trimmed, and printable text in any script is kept. A reason that
// is nothing but white space is no reason, so the item is read once more and
// gives the same.
func TestTransferFailureReason_CleansTheReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "already clean", reason: npmReason, want: npmReason},
		{
			name:   "line breaks, tabs and control characters",
			reason: "\n Root namespace can't be updated\r\nif the project has\tNPM packages\u0007scoped\u009b to it. \x1b[2J",
			want:   "Root namespace can't be updated if the project has NPM packages scoped to it. [2J",
		},
		{name: "another script", reason: "No se puede transferir el proyecto: ya existe «moving».", want: "No se puede transferir el proyecto: ya existe «moving»."},
		{name: "only white space", reason: " \t\n ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryAfter(t, time.Millisecond)
			gitlab := &reasonGitLab{
				t: t, versionStatus: http.StatusOK, versionBody: version195,
				graphqlStatus: http.StatusOK, graphqlBody: reasonData(t, tt.reason),
			}
			client := testutil.NewTestClient(t, gitlab.handler())

			if got := TransferFailureReason(t.Context(), client, 1042); got != tt.want {
				t.Errorf("TransferFailureReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTransferFailureReason_BoundsTheReason verifies the reason is cut at 500
// characters, the length GitLab truncates it to when it stores it, so every
// reason GitLab writes is quoted whole, and a longer one, which only an
// instance that is not GitLab sends, is cut on a character boundary.
func TestTransferFailureReason_BoundsTheReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "as long as GitLab stores", reason: strings.Repeat("é", 500), want: strings.Repeat("é", 500)},
		{name: "longer", reason: strings.Repeat("é", 501), want: strings.Repeat("é", 500)},
		{name: "cut on a space", reason: strings.Repeat("a", 499) + " b", want: strings.Repeat("a", 499)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitlab := &reasonGitLab{
				t: t, versionStatus: http.StatusOK, versionBody: version195,
				graphqlStatus: http.StatusOK, graphqlBody: reasonData(t, tt.reason),
			}
			client := testutil.NewTestClient(t, gitlab.handler())

			got := TransferFailureReason(t.Context(), client, 1042)
			if got != tt.want || !utf8.ValidString(got) {
				t.Errorf("TransferFailureReason() = %q (%d characters), want %d characters of %q", got, utf8.RuneCountInString(got), utf8.RuneCountInString(tt.want), tt.want)
			}
		})
	}
}

// TestQueryTransferFailureReason_CarriesTheCallersContext verifies the
// GraphQL read carries the caller's context, so an abandoned call abandons
// it. It is called directly because through TransferFailureReason it is never
// the first request on a client that holds no version.
func TestQueryTransferFailureReason_CarriesTheCallersContext(t *testing.T) {
	answer := reasonData(t, npmReason)
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, answer)
	})

	reason, item, err := queryTransferFailureReason(ctx, client, 1042)
	if !errors.Is(err, context.Canceled) || reason != "" || item {
		t.Errorf("queryTransferFailureReason() = %q, %t, %v, want no reason, no item and context.Canceled", reason, item, err)
	}
}

// TestTransferFailureReason_VersionUnknown_TheVersionReadCarriesTheCallersContext
// verifies that the version read the call starts with on a client that holds
// none carries the caller's context, so an abandoned call ends there: the read
// is abandoned with it, and the document sent after it is refused by the
// transport rather than sent without the context.
func TestTransferFailureReason_VersionUnknown_TheVersionReadCarriesTheCallersContext(t *testing.T) {
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, version195)
	})

	if got := TransferFailureReason(ctx, client, 1042); got != "" {
		t.Errorf("TransferFailureReason() = %q, want none from an abandoned call", got)
	}
}
