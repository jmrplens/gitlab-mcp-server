// Command audit_tokens measures the LLM context window overhead of all
// registered MCP tool definitions. It creates in-memory MCP servers in both
// individual and meta-tool modes, serializes tool definitions to JSON, and
// counts tokens with the cl100k_base tokenizer (see countTokens), falling back
// to a bytes/4 heuristic only if the tokenizer is unavailable.
//
// Usage:
//
//	go run ./cmd/audit_tokens/
package main
