// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package merge combines the three 1:1-audit gap reports, struct completeness
// (R-INPUT/R-OUTPUT), action coverage (R-ACTION) and enum values (R-ENUM),
// into a single per-package backlog.
//
// The input-mirror types intentionally capture only the fields the merger
// consumes; struct gaps and enum findings pass through as json.RawMessage so
// the merged artifact preserves every detail of the source report. The output
// backlog type is the contract downstream tooling (plan/1to1-backlog.json
// consumers, CI dashboards) depends on; the enum stream was added as new keys
// beside the existing ones, so a reader of the older shape still finds
// everything it read before. The struct stream's stale declarations were added
// the same way: a summary count and, when there are any, a top-level list,
// since the package a stale key names may be the thing that is gone. The
// discovery metadata stream (R-META) was retired, its four flags raised with
// a severity by cmd/audit_discovery_completeness, and its section and summary
// keys went with it.
//
// The summary counts the tree, as each stream's own summary does, so the
// packages -gaps-only drops change no count: the struct and enum counts are
// read from those streams' summaries, and action_missing_methods counts each
// client-go method once, where packages[] attributes it to every package that
// calls its service.
package merge
