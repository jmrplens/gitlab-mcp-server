package tenancy

// Meter is the bucket an MCP method is charged to, which also decides how its
// refusal is carried (register rows RTC-001 to RTC-004).
//
// The rate-limit middleware of internal/toolutil switches on it. The buckets
// themselves, their rates and bursts, the derivation of the completion and
// catalog buckets from the tool-call one, and the refusals stay where the
// requests are counted; what lives here is which method draws on which, so
// that metering one more method, or no longer metering one, is an edit to the
// register and to the oracle test beside it rather than to a switch in a
// layer.
type Meter uint8

// The meters.
const (
	// Unmetered methods are charged to no bucket (RTC-004): initialize,
	// resources/list, prompts/list and every method not named below. They
	// reach no upstream and cost little to answer, and metering something
	// cheap buys nothing and costs a concept.
	Unmetered Meter = iota
	// MeterToolResult is tools/call, charged to the entry's tool-call bucket
	// and refused as a successful result flagged with isError, so the model
	// receives a retryable diagnostic (RTC-001).
	MeterToolResult
	// MeterToolRPC is resources/read, resources/subscribe,
	// subscriptions/listen and prompts/get, charged to the same bucket as
	// tools/call because to GitLab a read is a read whichever MCP method asked
	// for it, and a limit that metered one door left the others open. Their
	// results carry no error flag, so each is refused with a JSON-RPC error
	// carrying [CodeTooManyRequests] (RTC-001).
	MeterToolRPC
	// MeterCatalog is tools/list, charged to the catalog bucket derived from
	// the tool-call one and refused with [CodeTooManyRequests]; the server's
	// own in-memory listings are exempt (RTC-003).
	MeterCatalog
	// MeterCompletion is completion/complete, charged to the completion
	// bucket derived from the tool-call one and refused with an empty
	// completion, never an error (RTC-002).
	MeterCompletion
)

// MeterFor returns the bucket method is charged to.
//
// The method is matched exactly as the SDK delivers it: no case folding and no
// trimming, so a method spelled any other way is [Unmetered], as it was when
// the middleware compared the strings itself. It allocates nothing, and it is
// small enough to inline into the middleware that calls it once per request.
func MeterFor(method string) Meter {
	switch method {
	case "tools/call":
		return MeterToolResult
	case "resources/read", "resources/subscribe", "subscriptions/listen", "prompts/get":
		return MeterToolRPC
	case "tools/list":
		return MeterCatalog
	case "completion/complete":
		return MeterCompletion
	}
	return Unmetered
}
