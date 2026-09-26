package tenancy

// The pseudo-methods a [Refusal] names where no MCP method carries it.
const (
	// MethodGate is the HTTP gate, which answers before the SDK sees the
	// request.
	MethodGate = "http"
	// MethodStartup is the process refusing to start.
	MethodStartup = "startup"
	// MethodEviction is the server ending what a pool entry held when the
	// entry is evicted.
	MethodEviction = "eviction"
	// MethodExpiry is the SDK ending a stateful session that sat idle.
	MethodExpiry = "expiry"
)

// Carriage is one row of the carried-channel matrix: the channels a refusal
// can reach a caller through, for a set of methods in one era.
//
// The matrix is the refusal channel table as data (spec: Refusal channels),
// and it is what go-sdk v1.8.0 does rather than what the protocol would permit:
// a tools/call can be refused as a result with isError because only that
// result has the flag, a listing only as a JSON-RPC error, and a status other
// than the SDK's own 400 and 404 only by the gate in front of it. An SDK
// upgrade that changes what is carried edits this table in the same change.
type Carriage struct {
	// Methods are the MCP methods, or the pseudo-methods [MethodGate],
	// [MethodStartup], [MethodEviction] and [MethodExpiry].
	Methods []string
	// Era is the protocol revision the row holds in; [EraAny] holds in both.
	Era Era
	// Channels are the channels carried.
	Channels []Channel
}

// Carriages returns the carried-channel matrix.
//
// Three channels record narrowing rather than refusal, on the tools/list and
// tools/call rows: a listing may leave out
// what the authorization on the request does not reach (Absent), and a call
// for such an action is answered with its cause (Withheld) or, for the tier,
// as unknown (Unknown). The listing may vary with the authorization and never
// with the connection (INV-009), which is why no listing row carries anything
// a rate or a ceiling could produce except an error.
func Carriages() []Carriage {
	return []Carriage{
		{Methods: []string{"tools/list"}, Channels: []Channel{RPC, Absent}},
		{Methods: []string{"prompts/list", "resources/list", "resources/templates/list"}, Channels: []Channel{RPC}},
		{Methods: []string{"tools/call"}, Channels: []Channel{RPC, ToolError, Withheld, Unknown, Absent}},
		{Methods: []string{"resources/read", "prompts/get"}, Channels: []Channel{RPC}},
		{Methods: []string{"completion/complete"}, Channels: []Channel{RPC, EmptyCompletion}},
		{Methods: []string{"resources/subscribe"}, Era: EraLegacy, Channels: []Channel{RPC}},
		{Methods: []string{"subscriptions/listen"}, Era: EraModern, Channels: []Channel{RPC, ListenEnd}},
		{Methods: []string{"initialize"}, Era: EraLegacy, Channels: []Channel{RPC}},
		{Methods: []string{"server/discover"}, Era: EraModern, Channels: []Channel{RPC}},
		{Methods: []string{"notifications/resources/updated"}, Channels: []Channel{Silent}},
		{Methods: []string{MethodGate}, Channels: []Channel{Gate, Silent}},
		{Methods: []string{MethodEviction, MethodExpiry}, Era: EraLegacy, Channels: []Channel{SessionClose}},
		{Methods: []string{MethodStartup}, Channels: []Channel{Startup}},
	}
}

// Carries reports whether a refusal on channel ch can reach the caller of
// method in era.
//
// A refusal for [EraAny] is carried when some era the method exists in carries
// it. On stdio there is no HTTP status and no session to close, so the gate
// and a session close are never carried there, whatever the method.
func Carries(method string, era Era, ch Channel) bool {
	if era == EraStdio && (ch == Gate || ch == SessionClose) {
		return false
	}
	for _, c := range Carriages() {
		if has(c.Methods, method) && erasMeet(c.Era, era) && has(c.Channels, ch) {
			return true
		}
	}
	return false
}

// erasMeet reports whether a matrix row for era row speaks to a refusal for
// era. A row for either era speaks to every refusal, a refusal for either era
// or for stdio is answered by every row of its method, and otherwise the two
// must be the same.
func erasMeet(row, era Era) bool {
	return row == EraAny || era == EraAny || era == EraStdio || row == era
}

// mcpDefinesCode reports whether the MCP specification defines code in the
// sub-range it reserves for itself, -32020 to -32099. Nothing else there may be
// emitted (INV-011).
func mcpDefinesCode(code int) bool {
	switch code {
	case codeHeaderMismatch, codeMissingRequiredClientCapability, codeUnsupportedProtocolVersion:
		return true
	default:
		return false
	}
}

// standardJSONRPCCode reports whether code is one of JSON-RPC's own, whose
// meaning the protocol fixes rather than this server.
func standardJSONRPCCode(code int) bool {
	switch code {
	case codeParseError, codeInvalidRequest, codeMethodNotFound, codeInvalidParams, codeInternalError:
		return true
	default:
		return false
	}
}
