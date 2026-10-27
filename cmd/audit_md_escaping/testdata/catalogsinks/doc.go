// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package catalogsinks holds the shapes the Markdown escaping audit reads a
// render of the message catalog by: a placeholder in each construct, filled
// by a raw value, an escaped one, an integer, a nested message or a list, and
// a value that reaches a placeholder through a message nested in it; the
// arguments and messages the audit cannot bind to a placeholder; a key that
// is not a constant, including one that arrives with the context in the
// results of one call; and a key with no template.
//
// The keys are this package's own constants, and the audit's tests hand it
// their templates, so nothing here depends on what the catalog the server
// ships holds. The package lives under testdata so no gate of the tree reads
// it as source.
package catalogsinks
