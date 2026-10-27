// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package catalogcards holds card rows written through the message catalog:
// rows a function directive answers, rows nothing answers, lines that only
// look like a row once a placeholder is read as text, and a directive left
// behind by a function that no longer writes any row, which is stale.
//
// The keys are this package's own constants, and the audit's tests hand it
// their templates. The package lives under testdata so no gate of the tree
// reads it as source.
package catalogcards
