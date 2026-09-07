// Command gen_request_inventory merges the request shards the unit suite
// records into the committed inventory of what this server sends GitLab.
//
// The 1:1 audit has five dimensions and every one of them describes the
// surface: the fields we accept, the fields we return, the actions we register,
// the discovery metadata we attach and the enum values we advertise. All five
// compare what we publish against what the SDK and the API documentation offer,
// and not one of them looks at the request a handler builds. That is how nine
// registered tools shipped while being unable to work: a perfect input struct,
// a perfect output struct, a registered action, complete enums, and a request
// GitLab refuses.
//
// This is the first half of the missing dimension. internal/testutil records
// every request a test issues, this merges the shards, and the result is
// committed, so a handler that starts calling a different endpoint or drops a
// parameter shows up in a diff.
//
// # What a row is
//
// One row per package, method and templated endpoint. The identifiers in a
// path are replaced by placeholders so a row describes an endpoint rather than
// a fixture, and the query parameter names are the union of every name the
// suite was seen to send to it. A GraphQL row carries the operation and the
// variables the document declares instead of a path and query keys.
//
// # What it is not
//
// It is not keyed by action, because nothing on the wire names one. See
// internal/testutil for why the honest attribution is the package that built
// the client, and what that misses. The summary this prints therefore counts
// actions whose owning package recorded nothing at all, which is a weaker
// statement than "this action's request was never seen" and the strongest one
// the recording can support today.
//
// It also says nothing about whether GitLab would accept what it lists.
// Comparing these paths with GitLab's own documentation is the check that
// follows this one, and a response our output struct misreads can only be
// caught by a real instance.
//
// Usage:
//
//	make gen-request-inventory                       # record and rewrite
//	go run ./cmd/gen_request_inventory/ -check       # fail when stale
package main
