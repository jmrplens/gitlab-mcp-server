// Package completions provides a CompletionHandler for GitLab-aware autocomplete
// of prompt arguments and resource URI template parameters.
//
// It queries GitLab search and project endpoints to return canonical argument
// values suitable for MCP completion results. Completion values intentionally
// use concrete identifiers such as project paths, usernames, issue IIDs, merge
// request IIDs, labels, milestones, and branch names rather than display labels
// so clients can insert them directly into prompt or resource arguments.
//
// # MCP Contract
//
// MCP completion results contain replacement values, not separate display text.
// This package therefore favors stable GitLab identifiers and returns empty
// completions on transient GitLab errors so autocomplete never blocks the
// calling client.
package completions
