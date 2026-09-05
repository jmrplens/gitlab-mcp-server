// Package structs verifies the 1:1 field mapping between the
// MCP tool input/output structs and the client-go SDK Options/result structs
// they wrap.
//
// For every package under internal/tools it resolves, with full Go type
// information:
//
//   - input pairs: each &gl.XxxOptions{} composite literal constructed inside a
//     handler is attributed to that handler's MCP input struct. The SDK Options
//     fields (by url/json tag) are diffed against the MCP input fields (by json
//     tag) to find SDK fields with no MCP counterpart (R-INPUT) and advisory
//     Go-type divergences.
//   - output pairs: each converter func(...src *gl.Y...) LocalStruct maps an SDK
//     result struct to an MCP output struct. The SDK result fields (by json tag)
//     are diffed against the MCP output fields (R-OUTPUT).
//
// The report is the mechanical backlog that drives the 1:1 audit batches. It is
// intentionally high-signal on *missing fields* (the gap class that the
// client-go bumps repeatedly introduced) and advisory on type divergences,
// because the domain legitimately maps SDK enum/time types onto scalars.
package structs
