// Package progress provides a Tracker for sending MCP progress notifications
// to the client during long-running tool operations.
//
// SECURITY: Progress tokens are opaque values provided by the client.
// They are forwarded as-is — never logged at a level above Debug and
// never included in error messages returned to the caller.
//
// SPEC: MCP 2025-11-25 requires progress values to strictly increase between
// notifications for the same token. The Tracker enforces this invariant by
// dropping non-monotonic Update calls (logged at Debug level), so misbehaving
// callers cannot violate the protocol contract.
package progress
