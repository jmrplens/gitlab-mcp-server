// Command godoc_tool is the consolidated Go documentation auditor and fixer.
// It combines the read-only audit (formerly audit_godocs — reports missing or
// malformed doc comments) and the in-place fixer (formerly add_docs — generates
// and inserts godoc-compliant comments) behind two subcommands.
//
// Usage:
//
//	godoc_tool audit [--format markdown|json] [--output path] [--fail-on-findings]
//	godoc_tool fix [--dry-run] [--move-package-doc] <paths...>
//
// The audit subcommand's flags match the former audit_godocs binary exactly.
// The fix subcommand gains a --dry-run flag (prints what would change without
// writing files) that the former add_docs binary lacked, and a
// --move-package-doc mode that moves each package comment into a doc.go of
// its own, which is where the audit expects it.
package main
