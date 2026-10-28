// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package i18n holds the texts this server shows a person directly, in every
// language it serves them in, and renders them in the language a request
// carries (issue 1547).
//
// # What it covers
//
// Only what a client puts in front of a person without the model reading it
// first: the questions and summaries of an elicitation, the titles of the
// fields of its form, and progress messages. Everything the model reads stays
// English (tool and parameter descriptions, results, hints, refusals, the
// instructions), and so does the listed catalog and everything an operator
// reads (startup errors, logs, the help). The model answers the person in the
// person's own language whatever it reads, and a deployment's logs and
// diagnostics are searched for in English.
//
// # How a text is written
//
// Each text has a [Key], declared as a constant in keys.go, and a template per
// language in that language's catalog: catalog_en.go for English, the source
// every translation is held to, and catalog_es.go for Spanish, the first
// translation. A translation holds exactly the English keys with the same
// placeholders and the same Markdown constructions, and keeps to the
// characters its language needs, which the catalog tests check entry by
// entry. A template names its values with placeholders, {name}, filled by the
// arguments of the call: [S] for a text the caller has already escaped for
// where it lands, [N] for an integer, [M] for another catalog message and [L]
// for a list joined with the language's own conjunction. Names rather than
// positions, because a translation puts the values in its own order.
//
// A message is built with [New] (and [Concat] for a summary whose rows are
// optional) and rendered with [Message.In], or with [Message.Text] in the
// language of a request. Every person-facing sink takes a [Text]: the
// questions, fields and titles of internal/elicitation, the progress messages
// of internal/progress and of the waits in internal/tools/waitpoll, and the
// destructive confirmations of internal/toolutil, which take the [Message]
// unrendered since the model reads the same question in English when the
// client cannot ask it. Only this package produces a Text, so a literal written
// at one of those call sites does not compile; [Verbatim] is the one way around
// that and exists for tests.
//
// English is the text the server sent before the catalog existed, byte for
// byte, which the test of the English catalog holds every entry to.
//
// # Where the language comes from
//
// The operator's GITLAB_MCP_LANGUAGE (or --language), en or es, read once by
// internal/config with [Parse], and put on the context of every request by
// the server's middleware with [WithLanguage]. [FromContext] reads it back,
// and answers English for a context that carries none, so a path the
// middleware does not cover is served exactly what it was served before the
// setting existed; [Requested] reads what was stored as it was, and whether
// anything was. The locale of the machine (LANG, LC_ALL, LC_MESSAGES, the
// bare LANGUAGE of gettext) is never read: a deployment that sets nothing is
// served English wherever it runs.
//
// # Why the standard library alone
//
// internal/config reads the setting and does not depend on the MCP SDK, so
// this package cannot either. The catalogs are Go maps rather than an
// embedded file or golang.org/x/text: the compiler refuses a duplicate key and
// an undeclared one, nothing is decoded at startup that could fail, and the
// plural and number formatting x/text exists for is what the per-request
// language proposal (SEP-2792) excludes from what a server translates.
package i18n
