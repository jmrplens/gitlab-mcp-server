//go:build e2e

// assert_helpers_test.go holds the collection-membership assertions the domain
// suites use to observe what a mutation actually did, rather than only that it
// returned without an error.
//
// Build tag: e2e.
package suite

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupboards"
)

// membershipWait bounds how long a membership assertion re-reads a collection
// before giving up. GitLab applies the mutations these assertions follow
// synchronously, so the budget exists only to absorb the replication lag an
// ephemeral instance under parallel load occasionally shows; a correct server
// must not be failed over timing.
const membershipWait = 20 * time.Second

// membershipPollInterval is how often a membership assertion re-reads while it
// waits for that lag to clear.
const membershipPollInterval = 500 * time.Millisecond

// requireListedOn re-reads a collection through the named tool and fails unless
// want is among the identifiers it reports.
//
// It exists because a mutation that answered without an error has not
// necessarily had an effect: a handler that turns a request GitLab refused into
// an empty result satisfies requireNoError and changes nothing, and only
// reading the collection back tells the two apart.
func requireListedOn[O any, K comparable](ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any, ids func(O) []K, want K) {
	t.Helper()
	requireMembership(ctx, t, session, label, tool, input, ids, want, true)
}

// requireNotListedOn is the counterpart of [requireListedOn] for a removal: it
// fails while want is still among the identifiers the collection reports.
func requireNotListedOn[O any, K comparable](ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any, ids func(O) []K, want K) {
	t.Helper()
	requireMembership(ctx, t, session, label, tool, input, ids, want, false)
}

// requireMembership polls the named list tool until want's presence among the
// identifiers matches present, and fails the test with the last observed
// collection when it never does.
//
// A failed re-read is retried rather than raised, because the budget exists to
// absorb exactly this: a rate limit or a 5xx from an ephemeral instance under
// parallel load says nothing about the mutation, and giving up on the first one
// would fail the test for a reason that is not the code. The last error is kept
// so a re-read that never recovers is still reported as itself.
func requireMembership[O any, K comparable](ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any, ids func(O) []K, want K, present bool) {
	t.Helper()
	var (
		last    []K
		callErr error
	)
	pollErr := Poll(ctx, membershipPollInterval, membershipWait, func() (bool, string, error) {
		out, err := callToolOn[O](ctx, session, tool, input)
		if err != nil {
			callErr = err
			return false, fmt.Sprintf("re-read failed: %v", err), nil
		}
		callErr = nil
		last = ids(out)
		return slices.Contains(last, want) == present, fmt.Sprintf("%v", last), nil
	})
	switch {
	case pollErr == nil:
		return
	case callErr != nil:
		t.Fatalf("%s: re-reading the collection still failed after %s: %v", label, membershipWait, callErr)
	case present:
		t.Fatalf("%s: %v is not listed after the mutation; collection holds %v", label, want, last)
	default:
		t.Fatalf("%s: %v is still listed after the mutation; collection holds %v", label, want, last)
	}
}

// requireGoneOn re-reads a single object through the named tool and fails while
// that read still succeeds. It is the counterpart of [requireNotListedOn] for a
// delete whose collection has no list action worth calling, and it insists the
// read fail as a not-found: any other error means the re-read itself broke,
// which proves nothing about the delete. Such an error is retried for the same
// budget as a membership assertion, since a rate limit or a 5xx from a loaded
// instance is not the answer being waited for either.
func requireGoneOn(ctx context.Context, t *testing.T, session *mcp.ClientSession, label, tool string, input any) {
	t.Helper()
	var lastErr error
	pollErr := Poll(ctx, membershipPollInterval, membershipWait, func() (bool, string, error) {
		_, err := callToolWithRetry(ctx, session, tool, input)
		lastErr = err
		switch {
		case err == nil:
			return false, "the object is still readable", nil
		case isNotFoundError(err):
			return true, "not found", nil
		default:
			return false, fmt.Sprintf("re-read failed: %v", err), nil
		}
	})
	switch {
	case pollErr == nil:
		return
	case lastErr == nil:
		t.Fatalf("%s: the object is still readable after the delete", label)
	default:
		t.Fatalf("%s: the re-read still failed with something other than a not-found after %s: %v", label, membershipWait, lastErr)
	}
}

// isNotFoundError reports whether err is GitLab saying the object is not there,
// either as an HTTP 404 or as one of the informational not-found results the
// get handlers return in its place.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if isHTTPStatus(err, http.StatusNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no longer exists")
}

// The listing projections only Community Edition suites read live in
// assert_helpers_ce_test.go, behind the constraint of the tests that use them;
// this file keeps the ones both halves of the suite call.

// groupBoardIDs maps a group issue board listing to the board IDs it holds.
func groupBoardIDs(out groupboards.ListGroupBoardsOutput) []int64 {
	ids := make([]int64, 0, len(out.Boards))
	for _, b := range out.Boards {
		ids = append(ids, b.ID)
	}
	return ids
}
