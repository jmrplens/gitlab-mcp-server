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
// A fourth kind is the prose the server serves around them, which names
// capabilities in whatever spelling the handler happened to write. See the
// rule over served prose below. A fifth is that prose read back: the
// substrings the e2e suite asserts a served text carries. See the suite that
// quotes it, further down.
//
// # What it reads
//
// The first four kinds out of ./internal/tools/... and ./internal/toolutil,
// loaded through cmd/internal/goprogram, the front end four gates already
// share. Constants are folded by the type checker rather than matched as
// text, and that is the whole reason for the loader: the IDs are written as
// package-local constants (actionGet, actionListProject), two packages build
// one by concatenation ("group." + actionGroupExportDownload), and a scan over
// literals reports the prefix "group." as a finding while passing the folded
// value in silence.
//
// toolutil declares no action and is loaded for the helpers a domain hands
// its hints to: a hint passed to a parameter named as one (the listHint and
// detailHint of NewTemplateRenderer, the hints of NewDiscussionRenderer, the
// hint of ExecGraphQLDestroyNote) is followed out to the domain that wrote it
// from the helper's own signature, and the walk can follow a value only into
// a package it loaded. Without toolutil in the load those parameters were
// never met: the tree read 1330 hints and reported none, and with it read
// 1355 and found eleven naming a tool the dynamic surface does not register.
// The same load reaches ActionRoute.WithRelatedActions, whose related
// parameter is followed out to its callers like any other. Its body merges
// that list into the route's own through a helper, and a call merging a
// recorded list with a list parameter named as related is passed over, like a
// copy of a recorded list, with the copy's hole: an ID the merging helper
// added of its own would not be read. A call handed such parameters alone is
// followed into, so a helper appending an ID to what it was handed is judged
// there, and a string parameter so named is one ID rather than a list, read
// through the body of the helper it is handed to.
//
// The fifth out of ./test/e2e/gitlab/..., in a second load through the same
// front end with the test variants included and the e2e build tag set, which
// the command states itself, the way cmd/audit_e2e_coverage -static does. A
// bare run makes both loads. A run naming patterns makes only the loads they
// name: a pattern under test/e2e/ goes to the suite and any other to the
// served tree, so an explicit ./internal/tools/... reads no suite and prints
// no section for it, and a run naming only suite packages reads no served
// source and says the published-ID and served-prose rules were not run rather than
// printing their counts over nothing. Only a run over the whole suite holds
// the helper table to it: the bare run, one naming ./test/e2e/gitlab/...
// itself, and one naming a wildcard that encloses it, ./... or ./test/...,
// which brings the whole suite into the suite load. The suite patterns are
// compared with ./test/e2e/gitlab/... once those a wildcard of the same list
// encloses are set aside: a suite package named beside the suite or beside
// such a wildcard, which the suite already encloses, leaves the run whole,
// and one outside ./test/e2e/gitlab/..., such as the harness, does not. The
// patterns are compared and not the packages they load, so naming the suite's
// three packages one by one is not judged whole, which the run says. The
// comparison reads slash-separated, so .\test\e2e\gitlab\... on Windows is
// the same run. A pattern given as an absolute path below the repository
// root, or as an import path below this module, is read as the relative
// pattern it names before it is sorted, and that is the pattern the load is
// handed: both spellings used to go to the served load whatever they named,
// which read the suite's packages as three doc.go files and reported them
// clean. A wildcard pattern that encloses the suite, ./... or ./test/...,
// goes to the served load as given and brings the whole suite into the suite
// load besides, since sorted by its prefix alone it produced that same clean
// run. A relative
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
// # The rule over served prose
//
// A model reads the prose a handler hands it exactly as it reads the rest, and
// nothing judged it. A hint saying "verify project_id with gitlab_project_get"
// names a tool the default dynamic surface does not register at all, and on
// meta only the bare domain tools exist, so the name is right for one surface
// of three. The canonical ID is the portable form here for the reason it is
// the portable form in a documentation example: it does not depend on
// GITLAB_MCP_TOOL_SURFACE.
//
// The sinks are a table keyed by a function's full name (proseSinks), each
// with the argument its prose starts at and the kind of site it is counted
// under: the hint of toolutil.WrapErrWithHint, WrapErrWithStatusHint and
// NotFoundResult (error_hint); the message of errors.New, fmt.Errorf,
// toolutil.ErrorResult and toolutil.CancelledResult (message); and the next
// steps toolutil.WriteHints, WriteListFooter and Card.End write (next_step).
// Fields are read by their names: a hint's, NextSteps included (hint_field),
// a message's where what is written into it folds (message), and
// ParameterGuidance's ValueSource and CommonConfusions (param_guidance). The
// jsonschema tag of every struct field is read as the description the schema
// serves (schema_description), and so is the description entry of a schema
// written as a map, which is what an input schema override is
// (toolutil.SchemaPropertyOverride) and what toolutil and dynamic build whole
// schemas from. A Usage line, whose dotted IDs the published-ID rule judges,
// is judged here for tool names too, and one assembled at run time is folded
// as a hint is, a helper that picks it by the action's name followed to every
// branch it returns from, and a call a Usage format is handed is followed the
// same way, which is how badges' scope boundary is read. An individual tool's
// Description, where it is a constant, is judged for tool names everywhere
// but its "See also" clause: gitlab://tools serves a domain action's
// Description verbatim on the dynamic and meta surfaces as well, rewriting
// only that clause into each surface's names, so the clause is the one part
// where an individual tool name is right. A standalone surface tool's is
// served on every surface, as its tool's description on meta and individual
// and as its Usage on dynamic, so the guided flows and project discovery
// write their one text as the Usage too, where it is read whole. Three
// spellings are
// reported: a gitlab_* tool name, a registered alias, and a dotted ID that
// resolves nowhere. A tool the package's own surface registers is declared
// (declaredSurfaceToolMentions), which is dynamic's two tools in dynamic's own
// package and nowhere else, and dynamic's schema descriptions alone may name
// a declared alias (declaredAliasMentions), which keeps the declaration alive.
//
// A sink's own prose parameters are not followed back out, since the sink's
// visit reads every call of it; toolutil's sinks forward their prose to one
// another, and following that reached every caller's argument a second time
// under another kind, whichever route the walk met first deciding which.
//
// A format folds to its format as written, verbs and all, followed by each
// constant argument, and its verbs are masked only in the text judged, so the
// fixer still finds the literal inside the value. An argument named for a hint
// is followed, and so is a parameter named for a message where the site's
// kind follows one (a helper handing on its caller's sentence); a local or a
// field named for a message is GitLab's text (glMsg) and is not. A read of a
// recorded field is a copy, and a sink call is read by its own visit. Every
// other argument is a value the sentence reports, and is passed over and
// counted rather than listed (values_passed_over), which is the rule's one
// deliberate exception to naming its blind spots: with some four hundred
// formats in the tree the list would be GitLab data from end to end. Inside a
// one-line helper such a value is passed over without being counted, since
// the fold reads the helper's body once per call and the value has no site of
// its own to count at (dynamic's queryTooLongMessage). A message field written
// from another struct's field is passed over and counted on the same terms.
//
// A helper handed only parameters named for this kind of prose, and no
// recorded read beside them, is followed into as well as out, which is where
// one that appends a sentence of its own to what it was handed writes it; the
// value variable of a range over a list of strings is followed to that list,
// which is how toolutil's list-footer filter reads. A helper handed a recorded
// read is a copy or a merge, and a sentence its body adds is not read.
//
// It **gates**. The first whole-tree run of the hint rule reported 785
// findings across 137 packages, and -fix-hints closed 712 of them; the run
// that widened it to the served prose reported 328 over 9671 sentences, and
// the tree was rewritten in the change below the one that widened the rule,
// so the gate turned on green. A declaration of either table that excuses
// nothing fails the run, like every declaration table here.
//
// Its own blind spots are counted beside the findings and do NOT fail, which
// is the one place this departs from the rule above. A sentence the type
// checker cannot fold is still text a reader can read, and the twenty-three
// in the tree build one from a helper that branches, a map read, a call into
// another module (accesstokens' operation phrase, whose last branch spells the
// action's name) or a parameter no rule follows, or read one back out of
// rendered text (toolutil's safe-mode preview parser). Five came with the
// run-time Usage lines and the map schemas this rule used to pass in silence,
// and one with following a Usage format's helper calls. A sentence concatenated
// from a literal and a value is folded to its literal halves, and the half it
// leaves unfolded is read on its own: a name is followed to the values it is
// handed, where a tool name is judged, and anything else is counted with the
// sites nothing folds. Keeping only the literal half used to count the
// sentence as read whole, and awardemoji handed three note deletes a list
// tool's name through exactly that shape.
//
// Its limits: a package-level map of prose read through a local, as
// mergerequests' mergeStatusHints is, carries no name the walk follows; the
// operation label a WrapErr* or ErrRequired* call prefixes an error with is
// not read, although some hundred of them spell it as a tool name, since it
// names what failed rather than inviting a call; a value a format reports is
// counted rather than read, as above; a bare meta action name ("Use action
// 'list'") is not read at all, having neither the gitlab_ prefix nor a dot,
// and naming the ID it means needs the domain the sentence belongs to, which
// it does not spell, so it is rewritten to toolutil.HintAction by hand; a
// merge's body adds prose unread, as above; a domain action's individual tool
// Description assembled at run time is read by no rule, and three such said
// "Use this instead of" a tool name until they were rewritten by hand, so a
// test in internal/resources reads every Description the built catalog
// carries outside the clause actioncatalog.SeeAlsoClause matches, which is
// the one this rule passes over too; and
// internal/prompts and internal/resources are outside the load, the review
// prompt being held to the catalog by a test of its own and the resource
// manifests' own prose, around the descriptions they serve, by no gate.
//
// The dotted-ID half of it was never large: the five unresolvable IDs the
// hint rule's first run found were one constant in
// internal/tools/workitemsavedviews naming "work_item_saved_view.list", where
// the catalog registers those actions as routes on the issue domain. That one
// is fixed by spelling the ID through the constant the catalog registers,
// which is the remedy for the class. The five the widened run found were in
// the parameter guidance of invites and pipelinetriggers.
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
// the needles under one of those names, or under a hint's, and hands them to
// a helper in a position that asserts (an assertion, or a predicate under
// `!`) is followed out to its callers.
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
// on the hint rule's terms, and so is the half of a concatenated needle that
// no name answers for: a needle written as a literal plus a value keeps its
// literal halves, and the value is followed to what it is given where it is a
// name, so a tool name concatenated into a needle is judged too. The findings gate, in a section of their own, so a
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
// mentionsAny(...); if !ok` is not judged, and neither is a wrapper that
// returns a predicate's answer, negated or not,
// `func has(...) bool { return mentionsAny(...) }` or
// `func lacks(...) bool { return !mentionsAny(...) }`: the first's inner call
// is not negated, the second's negation is part of what the return hands back
// and is passed over for that reason, and neither's callers are followed,
// since whether the needles are claims is decided at each of them, negated or
// not, and following them all would judge an absence check as a claim. A
// negation inside a function literal a return hands back is still read, since
// that body runs where it is called, unless that body returns it in turn,
// which makes the literal such a wrapper itself. Nothing names such a wrapper
// either, so it is a limit the suite keeps by writing none. And a dotted
// needle is judged as the whole ID it spells, so one that is only the front
// of a longer ID in a domain the catalog uses is refused although it matches
// at run time; the remedy, quoting the whole ID, asserts strictly more.
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
