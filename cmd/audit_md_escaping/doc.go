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
// literal or a named constant, both resolved by the type checker), or a call
// of toolutil.MarkdownTableRow or MarkdownTableHeader, which have no template
// at all because every argument they take is a cell by construction. The
// template is parsed with fmt's own grammar, flags, explicit argument indices,
// '*' widths and '%%' included, so a verb is never paired with the wrong
// expression: one formatter in this repository writes "[%[1]s](%[1]s)", which
// a regular expression mispairs. Only %s, %v and %q are judged. %q is judged
// despite quoting because Go's quoting escapes a quote and a backslash and
// neither a pipe nor an angle bracket, and the numeric verbs are skipped
// because none of them can emit any of the three whatever they are handed.
//
// Where a hole sits decides whether it matters, and the line it sits on
// decides where it sits: a pipe first means a table cell, one to six '#' and a
// space means a heading, a bullet or an ordered marker means a list item, an
// unclosed '[' means a link label, and the text after a '](' means a link
// destination. Everything else is prose and is skipped, because a paragraph
// holds a pipe, an angle bracket and a newline without changing shape, and the
// formatters that render GitLab-authored prose route it through WrapGFMBody.
// The -contexts flag narrows the run to some of the five, since the claim a
// table cell makes is stronger than the one a list item makes and the sweep
// can be staged by it.
//
// The value is then followed backwards to where it came from. It is safe when
// it is a compile-time constant, when its static type cannot render as text
// this server did not write (a number, a boolean, a time.Time or a Duration),
// when it has been through one of the toolutil escapers, when it is a nested
// Sprintf whose own holes are all safe, when it is a standard-library
// formatter of a non-textual value or a strings transform of values that are
// themselves safe, when every return of the declared function producing it is
// safe, when every assignment to the local holding it is safe, or when every
// caller passes a safe value to the parameter carrying it. A call binds its
// arguments to the callee's parameters, so a helper is judged at the call site
// that reaches it: toolutil.FormatTime returns its argument verbatim when
// neither layout parses, and without that binding the one caller passing a raw
// field would condemn the other hundred and fifty. It is unsafe when it
// bottoms out at a field of a struct filled from a GitLab response. Anything
// else is unresolved, which is reported in a bucket of its own and never
// counted as safe, because a gate that quietly called what it could not follow
// safe would be a gate with a hole in it.
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
// the directive is for. And a value breaking out of a fenced code block is out
// of scope entirely: those holes classify as prose, so a value carrying a
// fence of its own is a defect class this audit does not cover.
//
// Usage:
//
//	go run ./cmd/audit_md_escaping/
//	go run ./cmd/audit_md_escaping/ -json plan/md-escaping-backlog.json
//	go run ./cmd/audit_md_escaping/ -check
//	go run ./cmd/audit_md_escaping/ -contexts table-cell,heading -check
package main
