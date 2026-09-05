// Command audit_1to1 is the consolidated 1:1 SDK↔API parity audit. It combines
// the three gap streams — struct field mapping (R-INPUT/R-OUTPUT), action
// coverage (R-ACTION), and discovery metadata (R-META) — behind a single
// -scope flag, and merges them in-process when all three run together.
//
// Single-scope mode emits that auditor's native JSON shape (matching the legacy
// audit_struct_completeness / audit_action_coverage / audit_metadata_completeness
// binaries byte-for-byte). All-scopes mode produces the merged per-package
// backlog that gen_1to1_backlog previously generated from three separate files,
// via the same merge pipeline (byte-identical by construction).
//
// The sdk scope is a fourth, separate one, deliberately outside the merged
// backlog: the three streams above are candidate lists a human adjudicates,
// while sdk is a gate that exits non-zero on a finding. Keeping it out of the
// merge also leaves plan/1to1-backlog.json's shape untouched for the tooling
// that reads it.
//
// Usage:
//
//	go run ./cmd/audit_1to1/                                  # merged backlog to stdout
//	go run ./cmd/audit_1to1/ -gaps-only -output plan/1to1-backlog.json
//	go run ./cmd/audit_1to1/ -scope=structs                   # struct report only
//	go run ./cmd/audit_1to1/ -scope=actions -gaps-only
//	go run ./cmd/audit_1to1/ -scope=metadata
//	go run ./cmd/audit_1to1/ -scope=sdk -gaps-only            # SDK parity gate
package main
