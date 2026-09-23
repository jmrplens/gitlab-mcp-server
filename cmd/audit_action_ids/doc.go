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
// A fourth kind is the corrective prose an error helper hands a model, which
// names capabilities in whatever spelling the handler happened to write. See
// the rule over error hints below. A fifth is that prose read back: the
// substrings the e2e suite asserts a served text carries. See the suite that
// quotes it, further down.
//
// # What it reads
//
// The first four kinds out of ./internal/tools/... loaded through
// cmd/internal/goprogram, the front end four gates already share. Constants
// are folded by the type checker rather than matched as text, and that is the
// whole reason for the loader: the IDs are written as package-local constants
// (actionGet, actionListProject), two packages build one by concatenation
// ("group." + actionGroupExportDownload), and a scan over literals reports the
// prefix "group." as a finding while passing the folded value in silence.
//
// The fifth out of ./test/e2e/gitlab/..., in a second load through the same
// front end with the test variants included and the e2e build tag set, which
// the command states itself, the way cmd/audit_e2e_coverage -static does. A
// bare run makes both loads. A run naming patterns makes only the loads they
// name: a pattern under test/e2e/ goes to the suite and any other to the
// served tree, so an explicit ./internal/tools/... reads no suite and prints
// no section for it, and a run naming only suite packages reads no served
// source and says the published-ID and hint rules were not run rather than
// printing their counts over nothing. Only a run over the whole suite holds
// the helper table to it: the bare run, or one naming ./test/e2e/gitlab/...
// itself, which the comparison reads slash-separated, so .\test\e2e\gitlab\...
// on Windows is the same run. A pattern given as an absolute path below the repository root, or
// as an import path below this module, is read as the relative pattern it
// names before it is sorted, and that is the pattern the load is handed: both
// spellings used to go to the served load whatever they named, which read the
// suite's packages as three doc.go files and reported them clean. A relative
// pattern keeps its leading ./: without it go list reads test/e2e/gitlab/...
// as an import path, which matches no package of this module, and the run is
// refused with "no packages matched" before anything is judged.
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
// Four things about a published ID, and the first two are the whole point of
// the rule. The two prose rules below add their findings to them, and the
// suite's rule adds its helper table.
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
// # The rule over error hints
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
// It **gates**, and it was staged for exactly one release of the rule before
// it did. The first whole-tree run reported 785 findings across 137 packages
// of the 1327 hints this tree writes, every one of them a tool name, and a
// gate cannot land before the code it judges is clean. -fix-hints closed 712
// of them mechanically, the rest were judgement, and the flip is the layer
// after the count reached zero.
//
// Its own blind spots are counted beside the findings and do NOT fail, which
// is the one place this departs from the rule above. A hint the type checker
// cannot fold is still text a reader can read, and the three sites in that
// state build one from a function call or a format string and carry no tool
// name between them.
//
// The dotted-ID half of it was never large: the five unresolvable IDs the
// first run found were one constant in internal/tools/workitemsavedviews
// naming "work_item_saved_view.list", where the catalog registers those
// actions as routes on the issue domain. That one is fixed by spelling the ID
// through the constant the catalog registers, which is the remedy for the
// class.
//
// # The suite that quotes it
//
// The e2e suite is the one corpus that reads the server's hints back to it,
// and it was the one corpus nothing held to the catalog. Issue 901 was what
// that cost: twenty assertion literals still naming a tool after issue 883
// had moved the hints they quoted to canonical IDs, and 53 failing subtests
// that only the licensed run, which happens on tags, could report a month
// later. A grep for gitlab_* in the suite is the wrong rule, because a tool
// name there is usually right: harness.Raw names one on purpose, and the
// manifest, mode, exclusion and annotation scenarios look tools up by name.
// What made the twenty wrong was the position they sat in.
//
// So the position is what is read: the argument of a helper that asserts a
// served text carries a substring. The helpers are declared in
// servedTextAssertions (suite.go), each with the parameter its substring is
// passed in, found by that name in the callee's own signature: the
// substrings of assertMentions and mentionsAny, the needles of containsAny,
// and the contains of harness.ExpectToolError. A helper matches by name, and
// only when the function a call resolves to is declared under test/e2e/, so
// the copies of assertMentions in common and in ee are one entry and a
// function of the same name anywhere else is none. A suite wrapper that takes
// the needles under one of those names, or under a hint's, and hands them on
// is followed out to its callers.
//
// The two predicates are judged only where their call is negated. `if
// !mentionsAny(...)` fails the test when no needle is there, so each needle
// is a claim about what the server wrote; `if containsAny(...)` is an absence
// check or a classification, and the tool name the exclusion scenario asserts
// a refusal does NOT echo is exactly the literal such a check must spell.
//
// A needle is held to the three spellings a hint is: a gitlab_* tool name, a
// registered alias and a dotted ID nothing resolves. It consults the same
// exemption tables without keeping any entry of them alive, since those
// describe the served source, and one table more than a hint does:
// declaredAliasMentions, because a Usage line may name one of those aliases
// by design and a test quoting the line quotes it faithfully. A needle the
// type checker cannot fold, built from a fixture's name at run time or read
// off a test table's field, is counted and listed under -v and fails nothing,
// on the hint rule's terms. The findings gate, in a section of their own, so a
// reader can tell a defect of the server from a defect of its test.
//
// The helper table is held to the suite as every declaration table here is.
// Any run names a copy of a helper that takes no parameter of the declared
// name, with the package that declares it, and a run over the whole suite
// names an entry nothing calls, which is what a renamed helper looks like from
// here; both fail the gate, because either would stop every call of that
// helper being read without a word. The two are fixed in different places. An
// entry nothing calls is the table's to fix. A parameter that differs is the
// copy's: one entry names the parameter of every copy of its helper, so no
// edit of the table can agree with two copies that disagree, and the row says
// so rather than sending the reader to the entry.
//
// Its limits are the table's. A helper under a name the table does not
// declare is not read until it is declared, and neither is one called through
// a function value or a wrapper whose parameter carries another name, which
// is reported as a needle nothing folds. A copy of a declared helper may take
// its needles as a variadic tail or as a []string of its own, and both are
// read element by element. A bare strings.Contains on a served text is not
// read at all, by design: the suite calls it on its own values as often as on
// the server's, and on the server's for needles that name no tool and no
// action, a status code or one of the server's fixed phrases. The rule the
// suite's README states is therefore narrower than "never": a quotation that
// names a tool, an alias or an action ID goes through the helpers, and one
// written as a bare call is not read. The polarity is syntactic, so `ok :=
// mentionsAny(...); if !ok` is not judged. And a dotted needle is judged as
// the whole ID it spells, so one that is only the front of a longer ID in a
// domain the catalog uses is refused although it matches at run time; the
// remedy, quoting the whole ID, asserts strictly more.
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
//	go run ./cmd/audit_action_ids/ ./test/e2e/gitlab/ee    # one suite package
package main
