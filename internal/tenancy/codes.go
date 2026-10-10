package tenancy

// The policy refusal codes, which the layers alias.
//
// A code this server allocates for a purpose the MCP specification does not
// define belongs outside the JSON-RPC reserved range (-32768 to -32000), so
// every one mirrors the HTTP status the standard assigns to what its refusal
// says, multiplied by -100, at the gate and in-band alike (issue 956). A
// refusal that tells its caller it made too many requests is 429's
// [CodeTooManyRequests]; one that says the server cannot serve it right now is
// 503's [CodeUnavailable]. The legacy -32000, which the listen and watcher
// ceilings, a GitLab 429 on a subscription's first read and shutdown used to
// answer with, is retired: MCP 2026-07-28 tells new implementations not to use
// -32000 to -32019 and receivers not to read a meaning into them.
const (
	// CodeUnauthorized mirrors HTTP 401.
	CodeUnauthorized = -40100
	// CodeForbidden mirrors HTTP 403.
	CodeForbidden = -40300
	// CodeTooManyRequests mirrors HTTP 429, at the gate and in-band: the
	// caller made too many requests, or holds too many of something.
	CodeTooManyRequests = -42900
	// CodeUnavailable mirrors HTTP 503, at the gate and in-band: the server
	// cannot serve the request right now, whoever made it.
	CodeUnavailable = -50300
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
