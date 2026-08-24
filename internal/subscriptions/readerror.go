package subscriptions

import (
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TranslateReadError maps a GitLab API error onto the sentinels a watcher
// acts on, leaving everything else untouched so it is treated as transient.
//
// [Reader] implementations call this so the manager itself stays free of
// GitLab specifics: it decides *what to do* about an unreadable resource,
// while this decides *what the failure was*.
//
// The mapping is deliberately coarse. 401, 403 and 404 all become
// [ErrInaccessible] because GitLab answers 404 for a resource the caller
// may not see, precisely so it cannot be distinguished from one that does
// not exist — and a watcher's response is the same either way: stop, rather
// than keep polling with a token that has lost access. Everything that is
// not one of these four statuses stays transient on purpose: a 500, a
// timeout or a severed connection must cost latency, not the subscription,
// which is the whole reason polling is the authoritative path.
func TranslateReadError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case toolutil.IsHTTPStatus(err, http.StatusTooManyRequests):
		return ErrRateLimited
	case toolutil.IsHTTPStatus(err, http.StatusUnauthorized),
		toolutil.IsHTTPStatus(err, http.StatusForbidden),
		toolutil.IsNotFound(err),
		isResourceNotFound(err):
		return ErrInaccessible
	default:
		return err
	}
}

// isResourceNotFound reports whether an error is the MCP protocol's own
// not-found signal.
//
// Reads go through the registered resource handlers, and several of those
// answer with mcp.ResourceNotFoundError rather than passing GitLab's status
// through — a repository file that does not exist, a milestone whose lookup
// came back empty. That error carries no HTTP status and its message says
// only "Resource not found", so without this branch a deleted file or a
// revoked repository token would look transient and the watcher would keep
// polling for it until its absolute lifetime ran out.
func isResourceNotFound(err error) bool {
	var rpcErr *jsonrpc.Error
	// Compared against the SDK's own variable rather than the constant it
	// now points at: MCPGODEBUG=customresnotfounderrcode=1 flips it back to
	// the pre-1.7.0 -32002, and the value this server must recognize is
	// whatever the SDK is emitting, not whichever code the spec settled on.
	//nolint:staticcheck // SA1019: deprecated, but it is the value actually produced
	return errors.As(err, &rpcErr) && rpcErr.Code == mcp.CodeResourceNotFound
}
