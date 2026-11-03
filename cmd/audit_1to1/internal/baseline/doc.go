// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package baseline holds the findings of the 1:1 audit that report and never
// gate to a committed record of them, so a change that adds one says so
// (issue 1688, P5).
//
// Most of what the audit finds gates: the sdk, paths, grants, required,
// params, routes and enums scopes each fail on a finding no declaration
// answers. Some of what they find is a report by design, because its fix is
// client-go's or GitLab's, or because it is a lead rather than a verdict, and
// nothing held those lists: a client-go bump or a new handler could add a
// hundred entries to one of them and every gate stayed green. The measurement
// of 2026-10-31 called it the missing ratchet. This package is that ratchet.
//
// # Streams and keys
//
// A stream is one report-only list of one scope, named by the scope and the
// summary counter that counts it (params.unmodeled), and its findings are
// written as keys: the fields that name the finding, joined by " | ", without
// the counts, reasons and operation lists that describe it, so a key changes
// when the finding does and not when GitLab's documentation of it does. The
// streams are
//
//   - structs: the input and output fields an MCP struct lacks of the
//     client-go struct it is paired with, the output fields client-go does not
//     model, and the fields whose types differ;
//   - actions: the methods of a client-go service a handler calls that no
//     handler reaches;
//   - params: every class of a param a reached route declares and no recorded
//     request carries;
//   - paths: the package-grain output fields no endpoint declares, the nested
//     fields of a type GitLab's entity under the key does not declare and the
//     ones it sends that the type does not publish, the params client-go
//     always sends that GitLab leaves optional, and the identifier
//     placeholders the recording only ever stood one value behind;
//   - required: the fields no route of their action was found to carry, and
//     the ones a way of the action could not be read for;
//   - grants: the recorded rows the derivation does not reach, and the
//     derived requests no row records;
//   - enums: the output fields of an enum type whose schema names no value.
//
// What a declaration answers is not here: a declared finding is held by the
// declaration, which goes stale on its own.
//
// # How a stream is read
//
// Each stream is read out of its scope's native JSON report, as merge reads
// the merged three, and held to the counter its summary keeps for it: a list
// that does not hold as many entries as the summary counts is an error, which
// is what a renamed field, a report run with -gaps-only where that flag
// shortens the list, or a section that did not run would otherwise turn into
// an empty stream and a baseline that holds nothing. The identifier leads
// have no counter, and the paths sections say whether they ran, which is
// asked instead. A finding listed twice is one key.
//
// # The record and the check
//
// The record is docs/development/audit-1to1-baseline.json, written by
// -write-baseline (make gen-audit-1to1-baseline, a member of update-all). The
// check, -check-baseline, fails on a key the tree has and the record does not,
// naming its stream and key, and on a key of the record no finding answers,
// so the record cannot outlive what it holds. A change that adds a finding on
// purpose regenerates the record, and the diff of that file is what it adds.
package baseline
