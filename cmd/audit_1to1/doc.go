// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_1to1 is the consolidated 1:1 SDK↔API parity audit. It combines
// three gap streams, struct field mapping (R-INPUT/R-OUTPUT), action coverage
// (R-ACTION) and enum values (R-ENUM), behind a single -scope flag, and merges
// them in-process when all three run together.
//
// Single-scope mode emits that auditor's native JSON shape (matching the legacy
// audit_struct_completeness / audit_action_coverage binaries byte-for-byte).
// All-scopes mode produces the merged per-package backlog that
// gen_1to1_backlog previously generated from separate files, via the same
// merge pipeline (byte-identical by construction).
//
// The discovery metadata stream (R-META) is retired: it counted four flags of
// the ActionSpec catalog and gated nothing, while cmd/audit_discovery_completeness
// raises the same four with a severity and gates the errors among them, so
// -scope=metadata is refused with a message that says so.
//
// The sdk scope is a separate one, deliberately outside the merged backlog:
// the struct and action streams are candidate lists a human adjudicates,
// while sdk is a gate that exits non-zero on a finding. The enum stream is
// both: it is merged into the backlog so the per-package view names every
// value, and folded into the sdk gate so a value the SDK declares and the
// surface does not offer fails the build. Keeping sdk itself out of the merge
// leaves plan/1to1-backlog.json's shape readable by the tooling that already
// reads it.
//
// Usage:
//
//	go run ./cmd/audit_1to1/                                  # merged backlog to stdout
//	go run ./cmd/audit_1to1/ -gaps-only -output plan/1to1-backlog.json
//	go run ./cmd/audit_1to1/ -scope=structs                   # struct report only
//	go run ./cmd/audit_1to1/ -scope=actions -gaps-only
//	go run ./cmd/audit_1to1/ -scope=enums -gaps-only          # enum value gate
//	go run ./cmd/audit_1to1/ -scope=sdk -gaps-only            # SDK parity gate
//	go run ./cmd/audit_1to1/ -scope=params -gaps-only         # route params (R-PARAM)
//	go run ./cmd/audit_1to1/ -scope=routes -gaps-only         # live routes (R-ROUTE)
package main
