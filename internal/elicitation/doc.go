// Package elicitation provides a Client for requesting structured user input
// via the MCP elicitation protocol.
//
// The Client is a value type — its zero value is safe to use and acts as a
// no-op when the connected MCP client does not support elicitation. This
// mirrors the pattern used by progress.Tracker.
//
// # Validation and Safety
//
// SECURITY: All responses are validated against the expected JSON Schema.
// User input is never trusted and must be sanitized by the caller before
// use in API calls.
package elicitation
