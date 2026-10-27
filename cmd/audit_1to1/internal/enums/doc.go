// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package enums holds the 1:1 audit rule for enumerated values (R-ENUM).
//
// The struct rule (R-INPUT/R-OUTPUT) asks whether every SDK field has an MCP
// counterpart, and it answers "yes" for a field of an enum type the moment a
// same-named scalar exists on our side: a `type XxxValue string` is projected
// to a string in the field comparison, so the VALUES the SDK declares for it
// are never read. A constant added upstream is therefore invisible while the
// field stays covered. That is exactly what a hand-written pin test in the
// domain package used to guard, one enum at a time, by naming each constant,
// which is a list that has to be extended by hand to keep guarding anything.
//
// This rule reads the values instead. For every client-go type of the form
// `type T string` (or an integer kind) with a const block of T values, it finds
// the fields of that type an action exposes, through the same (MCP struct,
// SDK struct) pairs the struct rule diffs, and compares the SDK's constant set
// with the values our surface offers: the input schema's `enum`, or the
// property description where that is how a value set is surfaced. Values the
// SDK declares that we do not offer are reported, and so are values we offer
// that the SDK does not declare.
//
// Like the sdk scope this rule is a gate, with a table of exemptions each
// carrying a reason, and an exemption that no longer matches a field or a gap
// is itself a finding.
package enums
