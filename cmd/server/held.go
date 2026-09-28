package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// The ceiling on requests the process holds open at once, across every
// credential it serves (register row HLD-011, issue 951).
//
// A request is held from the moment the gate admits it until its POST ends,
// and a tools/call can hold its POST for as long as the call runs: a pipeline
// wait for up to an hour, and any call for as long as GitLab keeps it waiting
// for an answer. Measured through cmd/bench_resources' held mode against a
// stand-in GitLab that holds every read, each held call costs the process two
// file descriptors (the caller's connection and the one to GitLab), six
// goroutines, about 51 KiB of live heap and about 190 KiB of resident set,
// linearly: 4000 of them held 8010 descriptors and 873 MiB. Nothing bounded
// how many one credential, or all of them, could hold, and the pool's own
// bound is on entries, not on what an entry does. Started with a descriptor
// limit of 1024 and offered 1000 calls, the process held 503 and then stopped
// accepting connections at all, /health among them, until they ended; 442 of
// the calls failed with "too many open files" at their dial to GitLab.
//
// The ceiling is sized against that limit, beside the one on listen streams,
// which is the other kind of request the process holds open: 512 streams at
// one descriptor each, 192 held requests at two, and an eighth of the limit
// spare for the idle process, /health and the connections being refused make
// 1024. It is keyed on the process because only a ceiling keyed on the process
// bounds the process: a per-caller number would multiply by however many
// credentials a caller mints. And it is not configurable, because an operator
// who could raise it could undo the bound (INV-004, INV-018).
const maxHeldRequestsPerProcess = tenancy.HeldRequestsPerProcess // register row HLD-011

// heldRequests counts the requests the process holds open against a ceiling.
type heldRequests struct {
	open  atomic.Int64
	limit int64
}

// processHeldRequests is the one count every gate of this process shares:
// a server serves many credentials, and a listener serves them all.
var processHeldRequests = &heldRequests{limit: maxHeldRequestsPerProcess}

// acquire takes a slot, or reports that every slot is taken. It never takes
// more than the limit, even for a moment: a count that went over and came back
// would refuse a request that arrived while it was over, for a slot that was
// never used.
//
// A nil counter admits everything, and is what a gate built without one holds,
// which only the tests build.
func (h *heldRequests) acquire() bool {
	if h == nil {
		return true
	}
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
	if h == nil {
		return
	}
	h.open.Add(-1)
}

// protocolVersionStandardHeaders is the first revision whose Mcp-Method header
// the SDK holds to the method in the body: it answers a request whose header
// is missing or names another method with a 400 before any handler runs.
const protocolVersionStandardHeaders = "2026-07-28"

// holdsRequest reports whether a request the gate admits takes a slot.
//
// A POST does, since it carries the calls the process holds. A GET or a DELETE
// does not: the SDK answers both with 405 on the stateless transport, a DELETE
// ends a session and returns, and a GET is a session's one standalone stream,
// which the sessions it belongs to bound.
//
// A subscriptions/listen does not either, because the listen ceilings (HLD-001
// and HLD-002) already count it, and counting it twice would let the streams a
// deployment serves take the slots of every other call. The gate cannot read
// the method out of the body without buffering it, so it reads the Mcp-Method
// header, and only on a revision whose header the SDK holds to the body: there
// a POST that names subscriptions/listen is one, or is refused at once. A
// listen sent on an older revision, which no client of one sends, is counted
// here as well as there, which is the safe side to be wrong on.
func holdsRequest(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	return r.Header.Get("MCP-Protocol-Version") < protocolVersionStandardHeaders ||
		r.Header.Get("Mcp-Method") != methodSubscriptionsListen
}

// heldRequestsFailure is the refusal a request meets when every slot is taken:
// 503, in the gate, before the SDK has read anything.
//
// It says what the next action is and no more. The ceiling is the process's
// alone, so a caller refused by it learns that the process is full, which is
// the one bit INV-019 accepts for a bound keyed on the process; it carries no
// count and no identity. Retry-After is the register's fixed pause, the one
// the other ceiling on the process's own work answers with (ADM-014): when a
// slot will free depends on calls the caller cannot see, and thirty seconds
// spreads the retries a full process would otherwise meet at once.
func heldRequestsFailure() *gateFailure {
	return &gateFailure{
		status:  http.StatusServiceUnavailable,
		code:    errCodeUpstreamUnavailable,
		message: "This server is holding as many requests as it serves at once. Retry later.",
		header:  newHeader(headerRetryAfter, strconv.Itoa(int(upstreamRetryAfter.Seconds()))),
	}
}

// refuseHeldRequest writes the refusal and the operator's line for it.
//
// The line is throttled like every refusal a caller can cause at will, and it
// names the scope and the figure so an operator reading it knows which bound
// refused and that no flag raises it.
func refuseHeldRequest(w http.ResponseWriter, r *http.Request) {
	refusalLog.log(r.Context(), slog.LevelWarn, "request refused: too many requests held across the process",
		"scope", "process", "limit_held_requests", maxHeldRequestsPerProcess)
	heldRequestsFailure().write(w, r)
}
