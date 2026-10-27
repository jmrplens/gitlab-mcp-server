// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package sdk holds the two 1:1 audit rules whose universe comes from the SDK
// rather than from this repository's own call sites (R-SERVICE, R-GRAPHQL).
//
// The action-coverage scope answers "which methods of a service we call are
// uncovered". It cannot answer "is there a service we call nothing on", because
// its universe is built by walking our call expressions: a service nothing
// references never enters the map, so an entire upstream service can be added
// and the audit still report zero gaps. That is exactly what happened when
// WorkItemSavedViewsService arrived with seven methods.
//
// So this scope enumerates the services from the client-go Client struct and
// holds each one to a decision: covered by a call, or declared an exception
// with a category and a reason. A service that is neither is a finding.
//
// It carries a second rule for the same reason. ADR-0006 admits raw
// GraphQL.Do() for domains WITHOUT a client-go service wrapper. The wrapper
// appearing later is what retires that exemption, and nothing was checking, so
// the exemption was permanent in practice. Every raw GraphQL operation whose
// package maps to a client-go service is therefore held to a decision too,
// per operation rather than per package: packages that use GraphQL for one
// operation and the SDK for the rest are the norm, so a package-level verdict
// would be mostly noise.
//
// The enum rule (R-ENUM, package enums) is folded into this scope's summary
// and gate for the same reason again: its universe is the constants client-go
// declares, and a constant added upstream is invisible to the struct rule,
// which projects an enum type to a scalar and calls the field covered.
//
// The option structs rule is folded in on the same ground: its universe is
// the structs client-go's methods take, and the field diff starts
// from the literals our handlers build, so a struct a helper builds, a
// struct nested in a request, and a copy of one struct into another were
// compared with nothing or with the wrong input. Every struct a service
// method takes, and every struct a raw request carries as its options (typed,
// embedded in a struct of ours, or handed one call up as any), is held to an
// input a model sends, converted from a held struct, copied from one field
// for field, or declared.
//
// The upstream waits rule is folded in because its predicates are about the
// pinned SDK: a workaround carried because client-go does something wrong
// (a raw request, a request option written by hand, a body edit, a raw
// request of the e2e fixture or of an e2e scenario) is held to an entry of
// the waits table, and the entry to a predicate on client-go that says the
// workaround is still needed, so the pin that makes it obsolete fails the
// gate the day it lands. Every reason the walk can read off a workaround (a
// key a raw request carries beside the client-go struct it embeds, a body
// key, a method a request option pages) has to be one of those predicates,
// or a pin that fixes only the reasons written down would retire a
// workaround still needed.
//
// The captures rule is folded in on the same ground: a struct a handler
// decodes off a captured response (ADR-0021) reads what the client-go
// struct decoded from the same answer does not model, and a pin that models
// one of its keys leaves the key read twice with nothing to say so. Every key
// a capture shares with the client-go struct beside it is held to a
// declaration whose category requires a predicate on the pinned SDK, so the
// pin that takes the reason away fails the gate the day it lands.
//
// Unlike the three candidate-backlog scopes, this one is a gate: Run reports
// whether the tree is clean, and the command exits non-zero when it is not.
package sdk
