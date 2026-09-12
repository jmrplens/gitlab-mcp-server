// Command audit_md_escaping fails when a Markdown formatter interpolates a
// value this server did not write into a construct that value can change the
// shape of.
//
// The rule it enforces is already the house rule. toolutil.EscapeMdTableCell
// belongs on every GitLab-authored string that lands between two pipes of a
// table row and on every single-line list value, toolutil.EscapeMdHeading on
// the one value a formatter puts in a heading, and toolutil.MdTitleLink on
// both halves of a link. docs/concepts/security.md names all three, and most
// of the packages that register a formatter call them. Until this audit
// existed the rule was enforced by habit and by review, which is the kind of
// rule that survives until someone writes a domain in one sitting: a whole
// tool domain shipped with eight formatters and no escaping at all, and every
// gate passed.
//
// What the omission costs is not only table geometry. EscapeMdTableCell
// entity-encodes '<' on purpose, so GitLab-authored text cannot open raw HTML
// in a client that renders Markdown, and it strips control bytes. A title of
//
//	<a href="http://attacker.invalid/x">Fix login</a>
//
// reaches such a client as a working link to a host that is not GitLab, and a
// title of
//
//	Fix login](http://attacker.invalid/x)
//
// does the same by closing the label of a link the server wrote around it, on
// this server's own instruction, since HintPreserveLinks tells the model to
// keep those links clickable.
//
// # How it decides
//
// The audit type-checks the packages under internal/, finds every call that
// writes Markdown with a runtime value in it, and asks of each value whether
// it can carry a character that changes the document around it.
//
// A sink is an fmt formatting call whose format argument is a constant (a
// literal or a named constant, both resolved by the type checker), a call of
// toolutil.MarkdownTableRow or MarkdownTableHeader, which have no template at
// all because every argument they take is a cell by construction, or a call
// of the two toolutil.Card writes whose value the caller renders,
// Card.Markdown and CardTable.Row: every other Card method escapes what it is
// given, so a raw value passed to it reaches no construct, while those two
// write the value as given and the hole is the call site. The template is
// parsed with fmt's own grammar, flags, explicit argument indices, '*' widths
// and '%%' included, so a verb is never paired with the wrong expression: one
// formatter in this repository writes "[%[1]s](%[1]s)", which a regular
// expression mispairs. Only %s, %v and %q are judged. %q is judged despite
// quoting because Go's quoting escapes a quote and a backslash and neither a
// pipe nor an angle bracket, and the numeric verbs are skipped because none of
// them can emit any of the three whatever they are handed.
//
// Where a hole sits decides whether it matters, and the line it sits on
// decides where it sits: a pipe first means a table cell, one to six '#' and a
// space means a heading, a bullet or an ordered marker means a list item, an
// unclosed '[' means a link label, and the text after a '](' means a link
// destination. Everything else is prose and is skipped, because a paragraph
// holds a pipe, an angle bracket and a newline without changing shape, and the
// formatters that render GitLab-authored prose route it through WrapGFMBody.
// The -contexts flag narrows the run to some of the six, since the claim a
// table cell makes is stronger than the one a list item makes and the sweep
// can be staged by it.
//
// The sixth context is the exception to "the line decides", and is described
// below.
//
// # Inside a fenced code block
//
// A fenced code block is opened on one line and closed on another, so no line
// of a template says whether the hole on it is inside one. The fence context is
// decided by the writes that came before instead: one pass over each function
// body follows the text written to each strings.Builder or bytes.Buffer in
// source order, opening a block at a line that starts with three or more
// backticks and closing it at a line that starts with at least as many. A hole
// written while a block is open is judged in the fence context, whatever the
// line it sits on would otherwise say, since inside a block a pipe is text and
// a '#' is text and the only thing a value can do is end the block. The info
// string of the opening fence counts as inside it: a newline there ends the
// fence line, and everything the value carries after it is a line of the
// document.
//
// The rule that follows is that a backtick run this server wrote must not be
// the containment around a value it did not write. There is no way to escape a
// value into a fence, so the answer is always the same: build the block with
// toolutil.MarkdownFencedBlock, or size the fence with
// toolutil.MarkdownCodeFence, both of which measure the body and write a fence
// longer than the longest run in it. A block built that way has no literal
// backtick run in the source at all, so this pass sees no fence and judges
// nothing, which is what makes the rule self-enforcing rather than a list of
// approved call sites.
//
// Two deliberate limits keep it from inventing findings. A nested block
// inherits the state it is entered with and hands nothing back, so a chart
// whose fence is written at the top of a function and whose rows are written in
// a loop is judged, while a fence opened in one branch of an if and closed in
// another leaves the outer state closed and is not. And a call this pass does
// not read that is handed the builder gives up the state rather than keeping
// it, because the text that call writes may well be the closing fence. Both
// lose findings rather than invent them, which is the direction a gate has to
// err in.
//
// The value is then followed backwards to where it came from. It is safe when
// it is a compile-time constant, when its static type cannot render as text
// this server did not write (a number, a boolean, a time.Time or a Duration),
// when it has been through one of the toolutil escapers, when it is a nested
// Sprintf whose own holes are all safe, when it is a standard-library
// formatter of a non-textual value or a strings transform of values that are
// themselves safe, when it is a slice allocated by make and built by append
// out of safe values, when every return of the declared function producing it
// is safe, when every assignment to the local holding it is safe, or when
// every caller passes a safe value to the parameter carrying it. A call binds
// its arguments to the callee's parameters, so a helper is judged at the call
// site that reaches it: toolutil.FormatTime returns its argument verbatim when
// neither layout parses, and without that binding the one caller passing a raw
// field would condemn the other hundred and fifty. A parameter no call site
// binds is answered by every caller, and the reason names the caller that made
// it fail, by package, file and line, since the helper's own line is not where
// the fix goes; a caller that leaves a variadic parameter empty passes nothing
// and is skipped. It is unsafe when it bottoms out at a field of a struct
// filled from a GitLab response. Anything else is unresolved, which is
// reported in a bucket of its own and never counted as safe, because a gate
// that quietly called what it could not follow safe would be a gate with a
// hole in it. -fail-unresolved-in holds the named packages to no unresolved
// value at all, and the Makefile holds internal/toolutil to it: a blind spot
// there sits behind every formatter that calls it.
//
// # The card shape
//
// A tool result about one GitLab object is a card, and toolutil.Card is the
// one writer of its rows: a "- **Label**: value" list item per field, escaped
// at the write, with the layout kept whatever the value carries. The rows the
// tree wrote by hand before Card existed are the migration's work list, and
// the "card" rule reports them so that list is measured rather than guessed
// at: a constant line opening "- **Label**:" or "- **Label**" (a flag), with
// or without an emoji before the label; a bullet-less "**Label**:" line, which
// the line rule reads as prose and which renders as one run-on paragraph when
// several follow; a two-cell table row whose first cell is a constant label,
// "| Name | %s |"; and the header of a field table, "| Field | Value |" and
// the Property, Setting and Attribute spellings of it, whether written as
// text or built with MarkdownTableHeader. Matching the header alone is what
// catches a card whose rows are assembled across several writes. A row whose
// label is itself a value, an author item "- **@%s**", a row of a table of
// objects and a metrics table keyed by a map key are not cards and are not
// read as one. The rule reads constant text wherever it is written, in a
// template, a builder write or a print call, and leaves two files alone: the
// prompts, whose lists are the prompt's own layout, and card.go, whose writes
// are the rows every other formatter is asked to use. A card finding is
// reported as it stands, with no verdict to reach and no directive to excuse
// it, since the fix is the same whatever the value.
//
// # The second verdict: flags and instants
//
// The "bool-time" rule asks a different question of the same holes: not where
// the value lands but what it is. A boolean and a timestamp each have a
// display helper, BoolEmoji or Card.Bool for the one and FormatTime or
// Card.Time for the other, and a formatter that bypasses them renders the same
// fact three ways across the tree. It reports a flag printed by %t, a boolean
// or a pointer to one under a textual verb, strconv.FormatBool, and a declared
// helper whose every return is a yes-or-no word; a time.Time or a pointer to
// one under a textual verb, a Format call with a layout of the formatter's
// own, and a string field named like an instant (CreatedAt, ExpiresAt,
// DueDate) printed as GitLab sent it, through the escapers and transforms the
// first verdict sees through. A value inside a fence is not judged, since
// "true" and a Go-formatted instant are exactly what a JSON body says. The two
// verdicts are independent, so a timestamp excused for escaping is still
// reported for display, and each has its own directive:
//
//	//gitlab:allow-raw item.DueDate: a date GitLab sends without a time, shown as the day it names.
//
// A raw directive that excuses nothing is stale only once the rule has run,
// because a run that never asked the question has no grounds to say the
// answer was not needed.
//
// Both rules are staged: "all" names the six gating contexts and neither of
// them, so the gate keeps its exit status while they report, and a rule joins
// the gate when the Makefile names it beside "all".
//
// # Declaring that a value is already safe
//
// Some values that bottom out at a struct field need no escaping, and wrapping
// them would be noise that teaches the next reader the wrong rule: a canonical
// catalog ID compiled in from an ActionSpec is not GitLab-authored text at
// all. Those are declared in the source, in the package that owns the
// formatter, so the exemption is read beside the code it excuses:
//
//	//gitlab:allow-unescaped result.ID: a canonical catalog ID, compiled in from an ActionSpec rather than read from GitLab.
//
// The expression is the one the report prints, and the directive excuses every
// finding in its own package for that expression. A directive that excuses
// nothing is itself a finding, so an exemption cannot outlive the reason it
// was written for and cannot quietly widen the gate.
//
// # What it cannot see
//
// A format string assembled at runtime carries no template to parse, so a
// formatter that builds one is invisible. A value reaching a cell through a
// function value, through a call with several results, or out of a struct
// built positionally is reported unresolved rather than judged. Whether a
// struct is GitLab-derived at all is assumed rather than proven, which is what
// the directive is for.
//
// The fence rule adds limits of its own, beyond the two above that are there to
// keep it quiet. A fence written with tildes is not read as a fence, since
// nothing here writes one. A run shorter than three backticks is not a fence at
// all: it opens an inline code span, which ends with its own line, and the
// value between two of them is already judged by the cell, list item or heading
// that line is. A block whose fence is written in one function and whose body
// is written in another is seen by neither, because the state does not follow
// the builder into a call, and neither is one kept in a struct field rather
// than in a variable. A marker split across two writes ("“" and then "`") is
// read as neither, since each write is scanned as the text it is. And the text
// a value itself carries is taken to
// change nothing, so a hole is judged against the document the server wrote:
// assuming otherwise would mean assuming the breakout the rule exists to
// prevent, and every later hole would be judged against a document that never
// renders.
//
// Usage:
//
//	go run ./cmd/audit_md_escaping/
//	go run ./cmd/audit_md_escaping/ -json plan/md-escaping-backlog.json
//	go run ./cmd/audit_md_escaping/ -check
//	go run ./cmd/audit_md_escaping/ -check -fail-unresolved-in internal/toolutil
//	go run ./cmd/audit_md_escaping/ -contexts table-cell,heading -check
//	go run ./cmd/audit_md_escaping/ -contexts all,card,bool-time -v
package main
