// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package structs verifies the 1:1 field mapping between the
// MCP tool input/output structs and the client-go SDK Options/result structs
// they wrap.
//
// For every package under internal/tools it resolves, with full Go type
// information:
//
//   - input pairs: each request literal constructed inside a handler (an
//     &gl.XxxOptions{}, or a struct a service method takes under another name)
//     is attributed to that handler's MCP input struct, a struct of ours that
//     embeds one included; one a helper builds (a function with no input of
//     its own, or one that takes an input and sends nothing) is attributed to
//     the input of each handler that hands the helper that input's fields or
//     an intermediate filled from them; and one of a struct a request reaches
//     through a field is attributed to the input element its values are read
//     from, through the locals they are assigned to. The SDK fields (by url/json tag) are diffed against the
//     MCP input fields (by json tag) to find SDK fields with no MCP
//     counterpart (R-INPUT) and advisory Go-type divergences.
//   - output pairs: each converter func(...src *gl.Y...) LocalStruct maps an SDK
//     result struct to an MCP output struct, whether the package declares it or
//     instantiates a generic one with its own output type
//     (toolutil.CapturedThread[Output]). The SDK result fields (by json tag)
//     are diffed against the MCP output fields (R-OUTPUT). The converters of
//     the shared internal/toolutil package are diffed the same way, under a
//     toolutil entry, since that is where the shapes every domain nests (a
//     merge request's head_pipeline, every author) are filled; CollectPairs
//     and CollectOutputPairings keep to the tool packages.
//
// Beside them it reads the projections (CollectProjections): an output struct
// built field by field in a composite literal out of a client-go struct, with
// no converter, together with the service methods whose answer it is read
// from. The field diff does not read those, since a compact row held against
// the whole SDK struct would report every field it leaves out as a gap of the
// SDK surface; the paths scope reads them to judge the row against the one
// endpoint it comes from.
//
// The report is the mechanical backlog that drives the 1:1 audit batches. It is
// intentionally high-signal on *missing fields* (the gap class that the
// client-go bumps repeatedly introduced) and advisory on type divergences,
// because the domain legitimately maps SDK enum/time types onto scalars.
package structs
