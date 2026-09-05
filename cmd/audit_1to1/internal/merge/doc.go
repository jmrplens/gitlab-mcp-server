// Package merge combines the three 1:1-audit gap reports — struct
// completeness (R-INPUT/R-OUTPUT), action coverage (R-ACTION), and metadata
// completeness (R-META) — into a single per-package backlog.
//
// The input-mirror types below intentionally capture only the fields the
// merger consumes; struct gaps pass through as json.RawMessage so the merged
// artifact preserves every detail of the source report. The output backlog
// type is the contract downstream tooling (plan/1to1-backlog.json consumers,
// CI dashboards) depends on and must not change shape.
package merge
