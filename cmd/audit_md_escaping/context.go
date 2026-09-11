package main

import (
	"fmt"
	"sort"
	"strings"
)

// mdContext is where in a Markdown document a value lands, which decides both
// whether it needs escaping and which helper is the right one.
type mdContext int

const (
	// ctxProse is a paragraph. A pipe means nothing there and a newline is
	// legal, so only a heading or a list marker at the start of a line would
	// change the document's structure, and neither can be typed mid-line.
	ctxProse mdContext = iota
	// ctxCell is between two pipes of a table row.
	ctxCell
	// ctxHeading is on a line that opens with one to six '#'.
	ctxHeading
	// ctxListItem is on a line that opens with a bullet or an ordered marker.
	ctxListItem
	// ctxLinkLabel is between '[' and the '](' that closes the label.
	ctxLinkLabel
	// ctxLinkDest is between '](' and the ')' that closes the destination.
	ctxLinkDest
	// ctxFence is inside a fenced code block the formatter opened with a
	// backtick run of its own, the info string of that opening fence included.
	// It is the one context decided by the writes around the hole rather than
	// by the line it sits on, because a fence is opened on one line and closed
	// on another.
	ctxFence
	// ctxCard is a card row a formatter wrote by hand: a "- **Label**:" list
	// item, a bullet-less "**Label**:" line, a two-cell table row with a
	// constant label, or the "| Field | Value |" header of a field table. It
	// is not a place a value lands but a shape the constant text has, and it
	// is reported as it is, because the fix is the same whatever the value:
	// toolutil.Card writes the row, escapes the value and keeps the layout.
	ctxCard
	// ctxRaw is the second verdict on a hole: not where the value lands but
	// what it is. A boolean printed as true or false, and a timestamp printed
	// as Go formats a time.Time or as GitLab sent it, are each a value that
	// has a display helper, and a formatter that bypasses it renders the same
	// fact three different ways across the tree.
	ctxRaw
)

// structuralContexts are the contexts a value can change the shape of, in the
// order a report lists them. Prose is absent because a paragraph holds a pipe,
// an angle bracket and a newline without the document changing shape, and the
// formatters that render GitLab-authored prose route it through WrapGFMBody.
//
// They are what "all" selects and what the gate judges. The two staged
// contexts below are selectable by name and are deliberately not in this list.
var structuralContexts = []mdContext{ctxCell, ctxHeading, ctxListItem, ctxLinkLabel, ctxLinkDest, ctxFence}

// stagedContexts are the rules that report and do not yet gate: the card
// shape and the raw bool or time. Each is asked for by name, so a run that
// says "all" keeps the exit status it had before the rule existed, and the
// Makefile stages a rule into the gate by naming it.
var stagedContexts = []mdContext{ctxCard, ctxRaw}

// contextLabels name each context for the command line and for a report.
var contextLabels = map[mdContext]string{
	ctxProse:     "prose",
	ctxCell:      "table-cell",
	ctxHeading:   "heading",
	ctxListItem:  "list-item",
	ctxLinkLabel: "link-label",
	ctxLinkDest:  "link-destination",
	ctxFence:     "fence",
	ctxCard:      "card",
	ctxRaw:       "bool-time",
}

// String names the context for a report.
func (c mdContext) String() string {
	if label, ok := contextLabels[c]; ok {
		return label
	}
	return "prose"
}

// wants names the helper that belongs in this context, which is what a finding
// tells its reader to reach for.
func (c mdContext) wants() string {
	switch c {
	case ctxCell, ctxListItem:
		return "toolutil.EscapeMdTableCell"
	case ctxHeading:
		return "toolutil.EscapeMdHeading"
	case ctxLinkLabel, ctxLinkDest:
		return "toolutil.MdTitleLink"
	case ctxFence:
		return "toolutil.MarkdownFencedBlock"
	case ctxCard:
		return "toolutil.Card"
	case ctxRaw:
		return "toolutil.BoolEmoji or Card.Bool for a flag, toolutil.FormatTime or Card.Time for a timestamp"
	default:
		return ""
	}
}

// structural reports whether a value landing in this context can change the
// shape of the document around it rather than only its own text.
func (c mdContext) structural() bool {
	return c != ctxProse
}

// allContexts is the value of -contexts that judges every structural context.
// It names the gating set and none of the staged rules, so a staged rule
// joins the gate only when the Makefile names it beside "all".
const allContexts = "all"

// selection is the set of contexts one run judges. Staging the sweep by
// context is a flag rather than a branch because the four contexts carry
// genuinely different strengths of claim: a raw value ends a table cell
// outright, while in a list item it costs the containment and a line break.
type selection struct {
	chosen map[mdContext]bool
	label  string
}

// judges reports whether this run judges values landing in c.
func (s selection) judges(c mdContext) bool {
	return c.structural() && s.chosen[c]
}

// contextNames lists the accepted -contexts values, for the flag's own help
// and for the error a wrong one produces.
func contextNames() string {
	names := make([]string, 0, len(structuralContexts)+len(stagedContexts))
	for _, c := range structuralContexts {
		names = append(names, c.String())
	}
	for _, c := range stagedContexts {
		names = append(names, c.String())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// parseContexts turns the -contexts value into the set of contexts to judge.
//
// "all" is accepted alone, as the empty value, and as one entry of the list,
// which is how a staged rule is added to the gate without spelling the six
// contexts out: "all,card" judges the gating set and the card shape.
//
// An unknown name is an error rather than an empty selection, because a
// misspelled context would otherwise read as a gate that passed.
func parseContexts(value string) (selection, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = allContexts
	}
	sel := selection{chosen: map[mdContext]bool{}}
	// choose adds one context once, keeping the label in the order the
	// contexts were named.
	choose := func(c mdContext) {
		if sel.chosen[c] {
			return
		}
		sel.chosen[c] = true
		if sel.label != "" {
			sel.label += ", "
		}
		sel.label += c.String()
	}
	for name := range strings.SplitSeq(value, ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "":
			continue
		case allContexts:
			for _, c := range structuralContexts {
				choose(c)
			}
			continue
		}
		ctx, ok := contextNamed(name)
		if !ok {
			return selection{}, fmt.Errorf("unknown Markdown context %q: expected %s, or %s", name, allContexts, contextNames())
		}
		choose(ctx)
	}
	if len(sel.chosen) == 0 {
		return selection{}, fmt.Errorf("no Markdown context selected: expected %s, or %s", allContexts, contextNames())
	}
	return sel, nil
}

// contextNamed resolves a context by the name a report prints for it, the
// staged rules included.
func contextNamed(name string) (mdContext, bool) {
	for _, c := range structuralContexts {
		if c.String() == name {
			return c, true
		}
	}
	for _, c := range stagedContexts {
		if c.String() == name {
			return c, true
		}
	}
	return ctxProse, false
}

// contextAt classifies the Markdown context of the hole at offset within
// template.
//
// The line the hole sits on is what decides it, because every Markdown
// construct this audit cares about is a line-level one: a table row, a heading
// and a list item are each recognized by how their line opens. A link is
// looked for on the line as well, since a link written across two lines is not
// a link.
func contextAt(template string, offset int) mdContext {
	lineStart := strings.LastIndexByte(template[:offset], '\n') + 1
	before := template[lineStart:offset]
	if ctx, ok := linkContext(before); ok {
		return ctx
	}
	opening := strings.TrimLeft(before, " \t")
	switch {
	case strings.HasPrefix(opening, "|"):
		return ctxCell
	case headingPrefix(opening):
		return ctxHeading
	case listPrefix(opening):
		return ctxListItem
	default:
		return ctxProse
	}
}

// linkContext reports whether the text before the hole leaves it inside a
// Markdown link, and in which half.
//
// The scan is over the unclosed brackets on the line: a hole after a '[' that
// nothing has closed is in a label, and a hole after the '](' that closed one
// is in a destination until the ')' arrives.
func linkContext(before string) (mdContext, bool) {
	label := strings.LastIndex(before, "[")
	if label < 0 {
		return ctxProse, false
	}
	rest := before[label:]
	_, destination, closed := strings.Cut(rest, "](")
	if !closed {
		// The label is still open, unless a lone ']' already ended it without
		// opening a destination, which is not a link at all.
		if strings.Contains(rest, "]") {
			return ctxProse, false
		}
		return ctxLinkLabel, true
	}
	if strings.Contains(destination, ")") {
		return ctxProse, false
	}
	return ctxLinkDest, true
}

// headingPrefix reports whether a line opens an ATX heading: one to six '#'
// followed by a space, which is what CommonMark requires.
func headingPrefix(opening string) bool {
	hashes := 0
	for hashes < len(opening) && opening[hashes] == '#' {
		hashes++
	}
	if hashes == 0 || hashes > 6 {
		return false
	}
	return hashes < len(opening) && (opening[hashes] == ' ' || opening[hashes] == '\t')
}

// listPrefix reports whether a line opens a list item, bulleted or ordered.
func listPrefix(opening string) bool {
	if len(opening) >= 2 && strings.IndexByte("-*+", opening[0]) >= 0 && opening[1] == ' ' {
		return true
	}
	digits := 0
	for digits < len(opening) && opening[digits] >= '0' && opening[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits+1 >= len(opening) {
		return false
	}
	return (opening[digits] == '.' || opening[digits] == ')') && opening[digits+1] == ' '
}
