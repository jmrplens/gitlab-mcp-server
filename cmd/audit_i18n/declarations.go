// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import "github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"

// categoryCatalogReader is the one reason a conversion to the catalog's Key
// type may be excused: an audit that reads a key out of source, where the type
// checker folds it to a string, and asks the catalog compiled into it for that
// key's template, which takes a Key.
const categoryCatalogReader = "catalog-reader"

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryCatalogReader: "an audit asks the catalog compiled into it for the template of a key it read from source as a string",
}

// conversionsOnPurpose are the functions outside the catalog package this
// repository lets convert a value to the catalog's Key type, each with the
// reason that is right: why it converts, in a category and in words.
//
// Only a conversion is excused here. The other rules have no declaration to
// answer them, because each describes a text that reaches a person without
// the catalog: a key that is not a constant, a placeholder a call does not
// name, a text made outside the catalog, an entry nothing renders.
//
// The key is the package the repository names, a colon, and the function as
// its file spells it, or for a package-level initializer the first name its
// spec declares, which is also how a finding names where it sits. An
// entry that excuses nothing is reported, on the terms every declaration
// table here is held to: a declaration that has stopped describing the tree
// is itself a finding.
var conversionsOnPurpose = map[string]sourcegate.Declaration{
	"cmd/audit_md_escaping:catalogEnglishTemplate": {
		Category: categoryCatalogReader,
		Reason:   "the Markdown escaping audit reads each render's key as the constant the type checker folded, and judges the render by the English template i18n.EnglishTemplate returns for it",
	},
}

// declarationKey is how a finding is named in the declaration table.
func declarationKey(finding Finding) string {
	return finding.Package + ":" + finding.Func
}
