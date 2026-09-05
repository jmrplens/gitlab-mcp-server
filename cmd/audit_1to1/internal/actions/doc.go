// Package actions reports client-go SDK endpoints that no MCP
// action invokes (R-ACTION). For every package under internal/tools it resolves,
// with full Go type information, each call site of the form
// client.GL().{Service}.{Method}(...). The receiver type is a client-go service
// interface; its API methods are those whose signature ends in a variadic
// ...RequestOptionFunc. Methods on a used service that no handler calls are
// reported as candidate missing actions, grouped by the service and the
// internal/tools packages that reference it.
//
// The output is a candidate backlog, not a hard gate: a method may be
// intentionally unexposed, or owned by a sibling package. A human adjudicates
// each entry.
//
// Its universe is this repository's call sites, which bounds what it can see: a
// service no handler references never enters the map, so it cannot report one.
// The sibling sdk package enumerates the services from client-go's Client
// struct and covers exactly that blind spot.
package actions
