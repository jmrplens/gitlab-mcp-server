// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

// english is the source catalog. Missing is declared in keys.go and has no
// entry here.
var english = map[Key]string{
	ListAnd:  " and ",
	Pair:     "| {first} | {second} |",
	Dead:     "Nothing renders this",
	TestOnly: "Only a test renders this",
	Literal:  "Hello {name}",
}
