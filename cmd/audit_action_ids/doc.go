// Command audit_action_ids holds every canonical action ID this repository
// publishes to a model against the IDs the catalog really builds.
//
// Three kinds of string reach a model as an action ID it is invited to call
// next: the RelatedActions list of an ActionSpec, which the dynamic find and
// execute results carry; the first argument of toolutil.HintAction, which a
// Markdown formatter writes into the result a model reads; and a dotted ID
// spelled inside a Usage line or an individual tool's Description. Nothing
// validated any of them. An ID that resolves to nothing answers "unknown
// action" the moment a model follows it, and what the model concludes from
// that is that the capability is missing rather than that the cross-link is
// wrong.
//
// # What it reads
//
// Those three sites, out of ./internal/tools/... loaded through
// cmd/internal/goprogram, the front end four gates already share. Constants
// are folded by the type checker rather than matched as text, and that is the
// whole reason for the loader: the IDs are written as package-local constants
// (actionGet, actionListProject), two packages build one by concatenation
// ("group." + actionGroupExportDownload), and a scan over literals reports the
// prefix "group." as a finding while passing the folded value in silence. A
// value this cannot fold is counted and named rather than passed over, since a
// site the audit could not see must not be reported clean.
//
// # What it compares against
//
// The catalog this tree builds, at the Ultimate tier, twice: once against a
// self-managed stub instance and once against GitLab.com. The union is the
// oracle. Orbit registers only for GitLab.com, so a single self-managed build
// reports its six IDs as dead; taking the union rather than the GitLab.com
// build alone means an action gated the other way would not read as dead
// either. The standalone dynamic actions are added the way cmd/server adds
// them, because gitlab_execute_action takes those IDs too.
//
// # Two limits
//
// A clean run must not be read for more than it is.
//
// It answers whether an ID resolves, never whether it is the right ID. The
// catalog has both snippet.get, which reads a personal snippet, and
// snippet.project_get, which reads a project's; a project-snippet action
// cross-linked to the first is silent here, because the first resolves. What
// this narrows is the field to the IDs that cannot work at all.
//
// An ID that is a registered alias rather than a catalog ID is reported apart,
// under "alias", and is not counted as a finding. gitlab_execute_action
// resolves an alias, so a hint naming one works today; gitlab_find_action
// publishes canonical IDs, so a model that looks the name up in a listing does
// not find it. Which of those two facts should decide is a question for the
// layer that fixes the cross-links, and this command's job is to put both sets
// in front of it rather than to settle it.
//
// # It reports and does not gate
//
// The findings are spread over packages no single change touches, so a gate
// that failed today would fail on code the change introducing it never went
// near. The fixes land in later layers, and the flag that turns this into a
// gate belongs to the layer that can pass it.
//
// Usage:
//
//	go run ./cmd/audit_action_ids/                        # report, work list to plan/action-ids.json
//	go run ./cmd/audit_action_ids/ -v                     # also the aliases and what could not be folded
//	go run ./cmd/audit_action_ids/ ./internal/tools/issues # one package
package main
