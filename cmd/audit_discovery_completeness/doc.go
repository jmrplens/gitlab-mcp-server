// Command audit_discovery_completeness reports discovery-metadata gaps that
// affect how well models can find and choose the right action. It extends the
// existing R-META auditor (cmd/audit_metadata_completeness) with two new
// dimensions:
//
//   - Check B — Field-level: empty_param_description (input-schema property
//     descriptions that are blank or boilerplate) and empty_output_description
//     (lower priority).
//
//   - Check C — Sibling-cluster / confusable-action: cluster actions by
//     (owner_package, base_action_stem) stripping variant suffixes
//     (_batch/_bulk/_all/_directory/_single) and CRUD verbs. Flag any cluster
//     member that lacks both a sibling in RelatedActions (or a CommonConfusions
//     entry naming a sibling) AND a Usage string that mentions a distinguishing
//     signal. This is the link_create_batch / publish vs publish_directory
//     class of gap that motivated the discovery program.
//
// Check A — Action-level (weak_aliases, generic_usage, empty_related,
// weak_individual_description) reuses the R-META logic. missing_next_steps is
// a Phase 0 stub (see TODO) — Phase 1 will wire it to the markdown formatter
// registry via an exported HasRegisteredMarkdownFormatter helper.
//
// Output JSON shape (plan/discovery-backlog.json):
//
//	{
//	  "schema_version": 1,
//	  "summary": {...},                  // global counts
//	  "clusters": [{"package", "stem", "members"}],
//	  "packages": [{"package", "actions", "findings"}]
//	}
//
// Flags:
//
//	-output PATH        path to write JSON (default "-" stdout)
//	-gaps-only          omit actions without any finding
//	-min-aliases N      threshold for weak_aliases (default 3)
//	-severity LEVEL     error|warning|info for -check threshold (default error)
//	-check              exit non-zero if any finding meets/exceeds -severity
//
// Usage:
//
//	go run ./cmd/audit_discovery_completeness/                 # full report to stdout
//	go run ./cmd/audit_discovery_completeness/ -gaps-only      # only findings
//	go run ./cmd/audit_discovery_completeness/ -check          # CI gate
package main
