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
// A fourth kind is read and reported rather than gated: the corrective prose
// an error helper hands a model, which names capabilities in whatever spelling
// the handler happened to write. See the staged rule below.
//
// # What it reads
//
// Those three sites, out of ./internal/tools/... loaded through
// cmd/internal/goprogram, the front end four gates already share. Constants
// are folded by the type checker rather than matched as text, and that is the
// whole reason for the loader: the IDs are written as package-local constants
// (actionGet, actionListProject), two packages build one by concatenation
// ("group." + actionGroupExportDownload), and a scan over literals reports the
// prefix "group." as a finding while passing the folded value in silence.
//
// # What it compares against
//
// cmd/internal/actionids, shared with the documentation gate so that one
// spelling cannot be right in a Usage line and wrong on the page that teaches
// it: the catalog this tree builds, at the Ultimate tier, self-managed and
// GitLab.com unioned, with the standalone dynamic actions added the way
// cmd/server adds them.
//
// # What -check refuses
//
// Four things, and the first two are the whole point of the rule.
//
// A published ID that resolves to nothing is a cross-link a model cannot
// follow.
//
// A published ID that resolves only as a **registered alias** is refused too,
// which is the demand this gate exists to make: not that the ID resolve, but
// that it be the canonical one. gitlab_execute_action resolves an alias, so
// such a link does work when it is followed, and that is exactly what made the
// class invisible for so long. gitlab_find_action publishes canonical IDs, so
// a model that looks the name up in a listing does not find it; and the
// spellings that were sitting in this bucket were individual tool names
// (gitlab_runner_get in a RelatedActions list), which is a third naming scheme
// in a field documented as carrying catalog IDs. Demanding mere resolvability
// would have left all sixty of them in place. The one shape this would be
// wrong for is a sentence whose subject is the alias, and those are declared
// in declarations.go, consulted for prose alone.
//
// A declaration that excuses nothing is a finding, on the terms every
// declaration table in this repository is held to.
//
// A site the type checker could not fold is a finding as well. It is the
// audit's own blind spot rather than a defect of the tree, and it fails anyway
// because a gate whose blind spot is silent is one any future site can step
// into: an ID assembled at run time would be reported as unreadable and pass.
// The remedy is to spell the ID as a constant, which every site in the tree
// does today.
//
// # The staged rule over error hints
//
// A model reads the prose a handler hands it when a call fails exactly as it
// reads the rest, and nothing judged it. A hint saying "verify project_id with
// gitlab_project_get" names a tool the default dynamic surface does not
// register at all, and on meta only the bare domain tools exist, so the name
// is right for one surface of three. The canonical ID is the portable form
// here for the reason it is the portable form in a documentation example: it
// does not depend on GITLAB_MCP_TOOL_SURFACE.
//
// So the hint argument of toolutil.WrapErrWithHint, WrapErrWithStatusHint and
// NotFoundResult is read too, along with the struct fields such a hint is
// written into on its way to one, since nineteen domains reach those helpers
// through a field of their own output. The two are counted apart, because the
// field rule is the wider of the two and a reader should be able to tell which
// figure is which. Three spellings are reported: a gitlab_* tool name, a
// registered alias, and a dotted ID that resolves nowhere.
//
// It **reports and never gates**, and the reason is the size of it: 790
// findings across 138 packages of the 1327 hints this tree writes. A gate
// cannot land before the code it judges is clean, and until this rule existed
// nobody had the figure at all. Its own blind spots are counted beside the
// findings rather than folded into the gate's, so a hint the type checker
// cannot fold fails nothing.
//
// # The limit of a clean run
//
// It answers whether an ID resolves, never whether it is the right ID. The
// catalog has both snippet.get, which reads a personal snippet, and
// snippet.project_get, which reads a project's; a project-snippet action
// cross-linked to the first is silent here, because the first resolves. What
// this narrows is the field to the IDs that cannot work at all.
//
// That limit is permanent, and the reason is worth stating so nobody tries to
// close it here: the oracle is the set of IDs, and being the right ID is a
// claim about the object an action reaches, which no set of names carries.
// Reading each list against the parameters its own action requires is what
// answers it, and that is a review rather than a rule. A membership check is
// also blind to a cross-link that resolves for this tree and not for the
// session reading it, which is why the projection filters what it publishes
// (Registry.publishedRelatedActions in internal/tools/dynamic) instead of
// leaving that to a gate here.
//
// Usage:
//
//	go run ./cmd/audit_action_ids/                        # report, work list to plan/action-ids.json
//	go run ./cmd/audit_action_ids/ -check                 # the gate: make check-action-ids
//	go run ./cmd/audit_action_ids/ -v                     # also what a clean run judged, by kind
//	go run ./cmd/audit_action_ids/ ./internal/tools/issues # one package
package main
