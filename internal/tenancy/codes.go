package tenancy

// The policy refusal codes, which the layers alias.
//
// A code this server allocates for a purpose the MCP specification does not
// define belongs outside the JSON-RPC reserved range (-32768 to -32000), so the
// gate's codes mirror their HTTP status multiplied by -100 and the in-band
// "retry later" code mirrors 429 the same way. [CodeServerBusyLegacy] is the
// exception the specification records: it sits in the legacy sub-range that new
// implementations should not use, and issue 956 is where that is decided.
const (
	// CodeUnauthorized mirrors HTTP 401.
	CodeUnauthorized = -40100
	// CodeForbidden mirrors HTTP 403.
	CodeForbidden = -40300
	// CodeTooManyRequests mirrors HTTP 429, at the gate and in-band.
	CodeTooManyRequests = -42900
	// CodeUnavailable mirrors HTTP 503.
	CodeUnavailable = -50300
	// CodeServerBusyLegacy is the in-band code the listen and watcher
	// ceilings, a GitLab 429 on a subscription's first read and shutdown
	// refuse with.
	CodeServerBusyLegacy = -32000
)

// The protocol codes the rows cite. They are protocol vocabulary rather than
// policy, so the layers keep their own spellings and nothing aliases these; the
// rows name them so a refusal's code is compared by value whichever spelling
// the layer uses.
const (
	// codeParseError is JSON-RPC's "Parse error".
	codeParseError = -32700
	// codeInvalidRequest is JSON-RPC's "Invalid Request".
	codeInvalidRequest = -32600
	// codeMethodNotFound is JSON-RPC's "Method not found". go-sdk replaces
	// its message, so it cannot explain a refusal (INV-011).
	codeMethodNotFound = -32601
	// codeInvalidParams is JSON-RPC's "Invalid params".
	codeInvalidParams = -32602
	// codeInternalError is JSON-RPC's "Internal error".
	codeInternalError = -32603
	// codeHeaderMismatch is MCP's HeaderMismatch.
	codeHeaderMismatch = -32020
	// codeMissingRequiredClientCapability is MCP's
	// MissingRequiredClientCapability.
	codeMissingRequiredClientCapability = -32021
	// codeUnsupportedProtocolVersion is MCP's UnsupportedProtocolVersion.
	codeUnsupportedProtocolVersion = -32022
)
