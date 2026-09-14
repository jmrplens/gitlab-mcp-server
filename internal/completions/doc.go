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
//
// Empty rather than nil, always: the schema types values as an array, and a nil
// slice goes on the wire as null, which a client validating the response
// rejects instead of showing an empty list. [toResultWithTotal] is where that
// is settled, since every result is built through it.
//
// One failure is answered with an error instead: a prompt reference naming a
// prompt the server does not serve, refused with -32602 as the specification
// names and [Handler.PublishPrompts] makes possible. The distinction is what
// the caller can do about it. A GitLab hiccup is nothing, and a dropdown that
// stays empty is the right outcome; a prompt name that does not exist is
// something the caller sent, and answering it with live data says the prompt
// is there.
package completions
