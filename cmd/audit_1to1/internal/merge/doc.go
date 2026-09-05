// Package merge combines the four 1:1-audit gap reports — struct
// completeness (R-INPUT/R-OUTPUT), action coverage (R-ACTION), metadata
// completeness (R-META), and enum values (R-ENUM) — into a single per-package
// backlog.
//
// The input-mirror types intentionally capture only the fields the merger
// consumes; struct gaps and enum findings pass through as json.RawMessage so
// the merged artifact preserves every detail of the source report. The output
// backlog type is the contract downstream tooling (plan/1to1-backlog.json
// consumers, CI dashboards) depends on; the enum stream was added as new keys
// beside the existing ones, so a reader of the older shape still finds
// everything it read before.
package merge
