package main

import (
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
)

// cardRow is one line of constant text shaped like a row of a one-object
// card, which toolutil.Card is the one writer of.
type cardRow struct {
	// text is the line as the source wrote it, verbs included, so a finding
	// names the row a reader can search for.
	text string
	// verb says which shape matched: a row, or the header of a field table.
	verb string
}

// The four shapes a hand-written card takes. Each is matched against one line
// of constant text with its indentation removed, so a row nested under a list
// item is read the same as one at column zero.
var (
	// cardListRow is "- **Label**: value" and "- **Label**" (a flag), with an
	// optional emoji before the label, written as a verb or as a literal
	// glyph: "- %s **Label**", "- <warning> **Label**". The label is constant
	// text: a bold value written by a verb, "- **@%s** (...)", is an author
	// handle in emphasis and not a row.
	cardListRow = regexp.MustCompile(`^[-*+] (?:%[sv] |[^\x00-\x7F]{1,3} )?\*\*[^*%\n]+\*\*(?::|\s*$)`)
	// cardBareRow is "**Label**: value" with no bullet, which contextAt reads
	// as prose and which renders as one run-on paragraph when several follow.
	cardBareRow = regexp.MustCompile(`^\*\*[^*%\n]+\*\*:`)
	// cardPairRow is a two-cell table row whose first cell is a constant label
	// and whose second carries the value: "| Name | %s |". A collection row
	// has a verb in every cell, so a constant first cell is what tells a
	// field table from a table of objects.
	cardPairRow = regexp.MustCompile(`^\|\s*[^|%\n]*?[^|%\s][^|%\n]*?\s*\|[^|\n]*%[^|\n]*\|\s*$`)
	// cardHeader is the header of a field table, whichever of the four names
	// the tree gives the label column. Matching the header alone is what
	// catches a card whose rows are assembled across several writes.
	cardHeader = regexp.MustCompile(`^\|\s*(?:Field|Property|Setting|Attribute)\s*\|\s*Value\s*\|\s*$`)
)

// cardRows reads the card-shaped lines out of one piece of constant text.
func cardRows(text string) []cardRow {
	var rows []cardRow
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimLeft(line, " \t")
		switch {
		case cardHeader.MatchString(line):
			rows = append(rows, cardRow{text: line, verb: "header"})
		case cardListRow.MatchString(line), cardBareRow.MatchString(line), cardPairRow.MatchString(line):
			rows = append(rows, cardRow{text: line, verb: "row"})
		}
	}
	return rows
}

// cardHeaderCells reports whether a header built cell by cell is the header
// of a field table.
func cardHeaderCells(cells []string) bool {
	return len(cells) == 2 && cardHeader.MatchString("| "+cells[0]+" | "+cells[1]+" |")
}

// promptsPath is the package the card rule leaves alone: a prompt message is
// text the model reads as instructions, and its lists are the prompt's own
// layout rather than a tool result about one GitLab object.
const promptsPath = modulePath + "/internal/prompts"

// cardScoped reports whether the card rule applies to a file: everywhere
// under the audited packages except the prompts and the file that declares
// Card itself, whose writes are the rows every other formatter is asked to
// use instead of writing its own.
func cardScoped(pkg *packages.Package, filename string) bool {
	if pkg.PkgPath == promptsPath || strings.HasPrefix(pkg.PkgPath, promptsPath+"/") {
		return false
	}
	return pkg.PkgPath != toolutilPath || filepath.Base(filename) != "card.go"
}
