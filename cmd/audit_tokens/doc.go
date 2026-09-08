// Command audit_tokens measures the LLM context window overhead of all
// registered MCP tool definitions. It reads the individual, meta and dynamic
// surfaces through cmd/internal/mcpsurface, serializes the definitions a
// client would receive to JSON, and counts tokens with the cl100k_base
// tokenizer (see countTokens), falling back to a bytes/4 heuristic only if the
// tokenizer is unavailable. Measuring the listed surface rather than a
// hand-assembled one is what makes the figures the ones a client pays: the
// lockdown and the pagination bounds both change the bytes.
//
// Usage:
//
//	go run ./cmd/audit_tokens/
package main
