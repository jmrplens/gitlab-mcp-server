// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_i18n holds every text this server shows a person directly to
// the message catalog, internal/i18n (issue 1547).
//
// # Why this needs a rule of its own
//
// Once issue 1547 converts them, a person-facing sink, a question of an
// elicitation, the title of a field of its form and a progress message, takes
// an i18n.Text, which only the catalog package makes, so a literal written at
// its call site will not compile. The compiler stops there. It does not see a
// placeholder a call leaves unfilled, which a person reads as written, braces
// and all; a key the catalog holds no template for, which renders as the key
// itself; an untyped literal passed where a key goes, which compiles by
// conversion and names a key no constant declares; the test-only
// i18n.Verbatim called from production code, which serves one text in every
// language; or an i18n.Text or i18n.Message written as a literal, or a join
// of nothing, each of which is the empty text a person is shown as a form
// with no question. And it never says that an entry of the catalog is one
// nothing renders any more.
// Each of those is a text that reaches a person without the catalog, or a
// catalog that holds what no text reaches, and every one of them passes every
// test that does not render the text in a language other than English.
//
// # The rules
//
// A render is a call of i18n.T(ctx, key, args...) or i18n.New(key, args...).
//
//   - I1: a render whose key is a constant the English catalog has a template
//     for passes exactly the {name} placeholders of that template: none left
//     unfilled, none the template does not carry, none filled twice. Each
//     argument is an inline call of i18n.S, N, M or L whose name is a string
//     literal, since an argument built elsewhere, held in a variable or
//     spread into the call cannot be bound to the placeholder it fills by
//     reading the call; such an argument is a finding of its own, and the
//     placeholders it might fill are then not reported unfilled.
//   - I2: the key of a render is an identifier naming a key constant the
//     catalog package declares. Key is a type over string, so an untyped
//     literal compiles in its place by conversion, and a constant of the Key
//     type another package declares compiles as well; both put a key in the
//     tree the catalog's own key file does not name. A render whose one
//     argument is a call with several results, i18n.T(pair(ctx)), which Go
//     accepts for a variadic parameter, takes its key from those results and
//     is a finding too. No value is converted to the Key type outside the
//     catalog package, and T and New are only ever called, never held as a
//     value, since a call made through the value names its key where this
//     gate cannot read it.
//   - I3: every key of the English catalog is referenced from a file that is
//     neither the catalog's key file (keys.go) nor one of its catalog files
//     (catalog_*.go). A reference from another file of the catalog package
//     counts, which is how i18n.L uses common.list.and. And every key
//     constant the package declares has an English template, without which it
//     renders as its own key.
//   - I4: i18n.Verbatim is used only in a test file.
//   - I5: no i18n.Text or i18n.Message is written as a composite literal
//     outside the catalog package, and i18n.Concat is never called with
//     nothing to join. Every field of both types is unexported, so the one
//     literal either can be written as is the empty one, and each of those
//     three is an empty text made on purpose without a key. The zero value of
//     a declared variable is empty too and is not refused: a sink that reads
//     an empty text as none given, as the title of a form field will, takes
//     it as that.
//
// The catalog package's own code is the machinery every render goes through,
// so I1, I2 and I5 are not asked of it: i18n.T builds its message by calling
// New with the key it was handed, which is no constant.
//
// # Where the catalog comes from
//
// The English catalog is read from the loaded source, the composite literal
// the package-level variable english is initialized with, every key and
// template folded by the type checker, and the key constants from the
// package's scope. It is not read from the catalog compiled into this binary,
// so the gate judges the tree it is handed: its tests hand it planted modules
// under testdata whose stand-in catalog holds the dead, missing and misused
// keys the rules exist for. A catalog the reader cannot read that way, one
// with no english variable, one not written as a map literal or one holding
// an entry whose key or template does not fold to a constant, stops the run,
// since every rule would otherwise be asked of nothing.
//
// # Declarations
//
// A function outside the catalog package allowed to convert a value to the
// Key type is declared in declarations.go, keyed `package:Func`, with a
// category and a reason; a conversion in a package-level initializer is
// declared by the first name its spec declares, which is the name its
// finding gives it. The one entry is the Markdown escaping audit's, which
// reads each render's key as the string the type checker folded and asks
// i18n.EnglishTemplate, which takes a Key, for its template. Only a conversion
// can be declared: every other finding describes a text that reaches a person
// without the catalog, which nothing excuses. A declaration that excuses
// nothing is reported, scoped to the packages the run loaded, and so is one
// naming a category nobody defined.
//
// # What it reads, and what it cannot see
//
// ./internal/... and ./cmd/..., loaded through cmd/internal/goprogram without
// test files, with the catalog package added to whatever the patterns name.
// Test files are left out on purpose: a test is where i18n.Verbatim belongs,
// a test of the catalog renders keys it does not hold and passes arguments a
// template does not carry to see what happens, and a reference to a key from
// a test alone does not keep the key alive. The end-to-end packages under
// test/ are outside the patterns, since their library code runs in a test
// process and shows nobody anything.
//
// Whether a key is dead is asked only of a run over the default patterns:
// over part of the tree, a key the rest of it renders would read as one
// nothing renders. A run narrowed to a package still holds it to every other
// rule, and its summary says the dead-key half of I3 was not asked.
//
// A file a build constraint leaves out of the load is not read, and within a
// package the load reached that one is not silent: any such file that imports
// the catalog is reported as unjudged and fails the gate. A test file is not,
// since it would not be read under any constraint.
//
// The rules read the syntax of a render and not where its values come from,
// which is cmd/audit_md_escaping's question: that audit reads each render as a
// sink whose template is the key's English one, and judges the values the
// placeholders carry by the construct their line opens.
//
// Usage:
//
//	go run ./cmd/audit_i18n/          # report
//	go run ./cmd/audit_i18n/ -check   # fail on a finding (CI gate)
//	go run ./cmd/audit_i18n/ -v       # also list the conversions a declaration excuses
//	go run ./cmd/audit_i18n/ ./internal/tools/elicitationtools  # one package, dead keys not asked
package main
