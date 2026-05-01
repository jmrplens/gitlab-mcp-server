// Package sampling provides a client for requesting LLM analysis through MCP
// sampling and for executing allow-listed tool calls during iterative analysis.
//
// The [Client] is a value type whose zero value is safe to use and reports
// sampling as unsupported when the connected MCP client does not advertise the
// capability. User-supplied data sent to the LLM is wrapped in unpredictable
// nonce-based XML delimiters and sanitized for common credential patterns before
// transmission.
package sampling
