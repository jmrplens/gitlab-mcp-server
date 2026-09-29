package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The ceiling on the calls the process holds open at once, across every
// credential it serves (register row HLD-011, issue 951).
//
// A call is held from the moment it is dispatched until its handler returns,
// and a call that reaches GitLab can be held for as long as the call runs: a
// pipeline wait for up to an hour, and any call for as long as GitLab keeps it
// waiting for an answer. Measured through cmd/bench_resources' held mode
// against a stand-in GitLab that holds every read, each held call costs the
// process two file descriptors (the caller's connection and the one to
// GitLab), six goroutines, about 51 KiB of live heap and about 190 KiB of
// resident set, linearly: 4000 of them held 8010 descriptors and 873 MiB.
// Started under a descriptor limit of 1024 (soft and hard) and offered 1000
// calls, the process held 503 and then stopped accepting connections at all,
// /health among them, until they ended; 442 of the calls failed with "too many
// open files" at their dial to GitLab.
//
// So the ceiling is sized from the descriptors the process may open, read once
// at startup, rather than written as a number. A Go program does not keep the
// soft limit it was started with: the runtime raises it to the hard one before
// main (go.dev/issue/46279), so a host whose default soft limit is 1024 gives
// the process its hard limit, which is 524288 for a default systemd service
// and was 1048576 in the containers the measurement ran in. A fixed ceiling
// sized for 1024 would cut such a process to a twentieth of what it held
// there, and let one credential's long waits take every slot from every other
// tenant. Sized from the limit, it bounds exactly what runs out: where the
// hard limit is 1024 it is 192, and where the limit is large it stays out of
// the way.
const (
	heldRequestDescriptors  = tenancy.HeldRequestDescriptors  // register row HLD-011
	descriptorSpareDivisor  = tenancy.DescriptorSpareDivisor  // register row HLD-011
	fallbackDescriptorLimit = tenancy.FallbackDescriptorLimit // register row HLD-011
)

// heldRequestsFor is the ceiling for a process that may open descriptors file
// descriptors.
//
// An eighth of the limit is left spare for the idle process, /health and the
// connections being refused, and the listen streams are reserved at their own
// ceiling (HLD-002), one descriptor each, since they are the other kind of
// request the process holds open and are counted there rather than here. What
// is left is divided by what one held call costs. Under a limit of 1024 that is
// (1024 - 128 - 512) / 2 = 192. A limit too small to leave room for one held
// call after the reservation still serves one at a time: a ceiling of zero
// would refuse every call a process under such a limit could have served.
//
// Nothing here overflows or wraps: the reservation is never more than the
// limit, it is at most an eighth of the limit plus 512 anyway, and half of
// what is left of a uint64 fits an int64, which the largest limit there is
// pins in the tests.
func heldRequestsFor(descriptors uint64) int64 {
	reserved := min(descriptors, descriptors/descriptorSpareDivisor+maxListenStreamsPerProcess)
	return max(1, int64((descriptors-reserved)/heldRequestDescriptors)) //nolint:gosec // G115: a uint64 divided by HeldRequestDescriptors (2, pinned) is at most 2^63-1
}

// heldRequestsCeiling is the ceiling for this process, from the descriptor
// limit limit reads, or from the register's fallback where the platform has
// none to read.
func heldRequestsCeiling(limit func() (uint64, bool)) int64 {
	descriptors, ok := limit()
	if !ok {
		descriptors = fallbackDescriptorLimit
	}
	return heldRequestsFor(descriptors)
}

// heldRequests counts the calls the process holds open against a ceiling.
type heldRequests struct {
	open  atomic.Int64
	limit int64
}

// processHeldRequests is the one count every server and gate of this process
// shares: a server serves many credentials, and a listener serves them all.
//
// It is keyed on the process because only a ceiling keyed on the process
// bounds the process: a per-caller number would multiply by however many
// credentials a caller mints. And it is not configurable, because an operator
// who could raise it could undo the bound (INV-004, INV-018). Raising the
// descriptor limit the process runs under raises it, which is the one lever
// that also raises what it protects.
var processHeldRequests = &heldRequests{limit: heldRequestsCeiling(descriptorLimit)}

// acquire takes a slot, or reports that every slot is taken. It never takes
// more than the limit, even for a moment: a count that went over and came back
// would refuse a call that arrived while it was over, for a slot that was
// never used.
func (h *heldRequests) acquire() bool {
	for {
		held := h.open.Load()
		if held >= h.limit {
			return false
		}
		if h.open.CompareAndSwap(held, held+1) {
			return true
		}
	}
}

// release gives back a slot acquire took.
func (h *heldRequests) release() {
	h.open.Add(-1)
}

// holdsOpen reports whether a call of method is one the ceiling counts: a call
// that reaches GitLab, and so can be held for as long as GitLab keeps it
// waiting, other than a subscriptions/listen.
//
// Which methods reach GitLab is the register's answer, MeterFor's: the ones it
// charges to the tool-call bucket or the completion bucket. The rest answer
// from memory and cannot be held (initialize, the listings, ping). A notification
// is metered to nothing and has no response for a POST to wait on. A listen is
// held for as long as the caller keeps it open and is counted by the listen
// ceilings (HLD-001 and HLD-002); counting it here as well would let the
// streams a deployment serves take the slots of every other call.
func holdsOpen(method string) bool {
	if method == methodSubscriptionsListen {
		return false
	}
	switch tenancy.MeterFor(method) {
	case tenancy.MeterToolResult, tenancy.MeterToolRPC, tenancy.MeterCompletion:
		return true
	default:
		return false
	}
}

// protocolVersionStandardHeaders is the first revision whose Mcp-Method header
// the SDK holds to the method in the body, and on which it refuses a batch: a
// POST there carries one message, and one whose header is missing or names
// another method is answered 400 before any handler runs.
const protocolVersionStandardHeaders = "2026-07-28"

// gateCountsRequest reports whether the gate can take a POST's slot itself,
// before the SDK reads a byte of it.
//
// Only where the header is the method: on protocol 2026-07-28 or later, where a
// POST carries one message and the SDK refuses one whose Mcp-Method does not
// name it. Anywhere else a POST can carry a batch of calls, a response to a
// request the server sent, or a notification, and only the SDK's reading of
// the body says which, so the calls on it are counted one by one where the SDK
// dispatches them ([heldRequestsMiddleware]). A POST whose header names
// something no count takes, a listen or a notification, is left to the same
// place, and so is one whose header lies: the SDK refuses it before dispatch.
func gateCountsRequest(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		r.Header.Get("MCP-Protocol-Version") >= protocolVersionStandardHeaders &&
		holdsOpen(r.Header.Get("Mcp-Method"))
}

// gateHeldSlot is the slot the gate took for a POST it read as one held call,
// which the middleware hands to that call rather than taking a second.
type gateHeldSlot struct {
	claimed atomic.Bool
}

// gateHeldSlotKey is the context key a POST's gateHeldSlot travels under.
type gateHeldSlotKey struct{}

// claimGateSlot reports whether the POST the carrier token names holds a slot
// the gate took that no call has claimed yet, and claims it. The first call on
// such a POST is covered by it; a second, which only a POST the SDK did not
// refuse as a batch could carry, takes its own, so the count never falls short
// of the calls held whatever the SDK let through. A POST that is gone has no
// carrier left to look up, and its calls claim nothing.
func claimGateSlot(token string) bool {
	carrier := mcpCarriers.lookup(token)
	if carrier == nil {
		return false
	}
	slot, _ := carrier.Value(gateHeldSlotKey{}).(*gateHeldSlot)
	return slot != nil && slot.claimed.CompareAndSwap(false, true)
}

// heldRequestsMiddleware counts every held call that arrived on an HTTP POST,
// as the SDK dispatches it.
//
// This is where the ceiling is exact: it sees each call of a batch, never sees
// a response the client sent to a request of the server's own, and names the
// method the SDK read out of the body rather than one a header claimed. A call
// on a POST whose slot the gate already took claims that slot instead of
// taking another. A request that arrived on no POST, which is stdio and every
// in-memory client, is not counted: the ceiling bounds what a listener holds
// open, and a stdio process serves one caller over one pipe.
//
// It runs before the rate limit, so a call it refuses spends none of its
// credential's bucket.
func heldRequestsMiddleware(held *heldRequests) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			token := carrierTokenOf(req)
			if token == "" || !holdsOpen(method) || claimGateSlot(token) {
				return next(ctx, method, req)
			}
			if !held.acquire() {
				return heldRequestsRefusal(ctx, method, held.limit)
			}
			defer held.release()
			return next(ctx, method, req)
		}
	}
}

// heldRefusalText is what every refusal of the ceiling says.
//
// It says what the next action is and no more. The ceiling is the process's
// alone, so a caller refused by it learns that the process is full, which is
// the one bit INV-019 accepts for the refusal of a bound keyed on the process;
// it names no bound, no count and no caller, so it says nothing a caller could
// not read off the refusal happening at all. The log line is the one place
// that says which bound refused.
const heldRefusalText = "This server is busy. Retry later."

// heldRequestsRefusal refuses a call the middleware counted, carried the way
// the rate limit carries its own refusal of the same method: a tools/call as a
// result flagged with isError, so the model reads a retryable diagnostic; a
// completion as an empty completion; any other method as a JSON-RPC error with
// the in-band "retry later" code.
func heldRequestsRefusal(ctx context.Context, method string, limit int64) (mcp.Result, error) {
	logHeldRefusal(ctx, limit)
	switch tenancy.MeterFor(method) {
	case tenancy.MeterToolResult:
		return toolutil.ErrorResultAnnotated(heldRefusalText, toolutil.ContentMutate), nil
	case tenancy.MeterCompletion:
		return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{}}}, nil
	default:
		return nil, &jsonrpc.Error{Code: heldRefusalCode, Message: heldRefusalText}
	}
}

// heldRefusalCode is the in-band "retry later" code the ceiling refuses with
// where the method carries a JSON-RPC error.
const heldRefusalCode = tenancy.CodeTooManyRequests // register row HLD-011

// processBusyFailure is the refusal a request the gate counted meets when a
// ceiling on what the process holds is full: 503, in the gate, before the SDK
// has read anything. The held calls and the standalone streams meet it at this
// ceiling, and a POST that would open a stateful session at the session
// ceiling (register row HLD-010).
//
// It says what heldRefusalText says. Retry-After is the register's fixed
// pause, the one the other ceiling on the process's own work answers with
// (ADM-014): when a slot will free depends on calls the caller cannot see, and
// thirty seconds spreads the retries a full process would otherwise meet at
// once. And the connection is closed with the answer: a refused caller that
// kept it would hold a descriptor of the very limit the ceiling protects, and
// the server keeps an idle connection open for as long as --http-idle-timeout
// says, which is forever by default.
func processBusyFailure() *gateFailure {
	return &gateFailure{
		status:  http.StatusServiceUnavailable,
		code:    errCodeUpstreamUnavailable,
		message: heldRefusalText,
		header: newHeader(
			headerRetryAfter, strconv.Itoa(int(upstreamRetryAfter.Seconds())),
			"Connection", "close",
		),
	}
}

// refuseHeldRequest writes the gate's refusal and the operator's line for it.
func refuseHeldRequest(w http.ResponseWriter, r *http.Request, limit int64) {
	logHeldRefusal(r.Context(), limit)
	processBusyFailure().write(w, r)
}

// logHeldRefusal writes the operator's line for a refusal of the ceiling.
//
// The line is throttled like every refusal a caller can cause at will, and it
// names the scope and the figure so an operator reading it knows which bound
// refused and that no flag raises it.
func logHeldRefusal(ctx context.Context, limit int64) {
	refusalLog.log(ctx, slog.LevelWarn, "request refused: too many requests held across the process",
		"scope", "process", "limit_held_requests", limit)
}
