// Package sampling provides a client for requesting LLM analysis through MCP
// sampling and for executing allow-listed tool calls during iterative analysis.
//
// The [Client] is a value type whose zero value is safe to use and reports
// sampling as unsupported when the connected MCP client does not advertise the
// capability. User-supplied data sent to the LLM is wrapped in unpredictable
// nonce-based XML delimiters and sanitized for common credential patterns before
// transmission.
//
// # Analysis Flow
//
// Tool handlers create a [Client] with [FromRequest], then call
// [Client.Analyze] for a single sampling request or [Client.AnalyzeWithTools]
// when the model may request allow-listed tool calls through a [ToolExecutor].
// The final response is returned as an [AnalysisResult].
//
// The package applies several guardrails before data leaves the server:
//
//   - input is capped by [MaxInputLength];
//   - common credential patterns are redacted;
//   - user data is wrapped in nonce-delimited XML blocks;
//   - confidential payloads can be labeled with [WrapConfidentialWarning].
package sampling
