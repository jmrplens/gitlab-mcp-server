// Package gatewaycompat rewrites the human-readable text this server lists —
// tool, prompt, resource and resource-template descriptions and titles, and
// the description and title annotations embedded in tool schemas — according
// to operator-defined substitutions.
//
// It exists because MCP gateways validate a server's catalog before admitting
// it, and their rules are the gateway operator's to choose: one production
// gateway (IBM mcp-context-forge before 0.7.0) refused any tool whose
// description contained a semicolon. This server keeps its own text clean of
// the characters known to be rejected (cmd/audit_gateway_chars gates that),
// but the next gateway rule is not this project's to predict. The
// substitution knob lets the operator comply with a rule the day they meet
// it, without waiting for a release.
//
// The knob rewrites catalog metadata and nothing else: names, URIs, schema
// constraints (pattern, const, enum values, defaults) and tool-call payloads
// are never touched, because those are contract, not prose.
package gatewaycompat
