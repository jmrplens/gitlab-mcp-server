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
)

// structuralContexts are the contexts a value can change the shape of, in the
// order a report lists them. Prose is absent because a paragraph holds a pipe,
// an angle bracket and a newline without the document changing shape, and the
// formatters that render GitLab-authored prose route it through WrapGFMBody.
var structuralContexts = []mdContext{ctxCell, ctxHeading, ctxListItem, ctxLinkLabel, ctxLinkDest}

// contextLabels name each context for the command line and for a report.
var contextLabels = map[mdContext]string{
	ctxProse:     "prose",
	ctxCell:      "table-cell",
	ctxHeading:   "heading",
	ctxListItem:  "list-item",
	ctxLinkLabel: "link-label",
	ctxLinkDest:  "link-destination",
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
	names := make([]string, 0, len(structuralContexts))
	for _, c := range structuralContexts {
		names = append(names, c.String())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// parseContexts turns the -contexts value into the set of contexts to judge.
//
// An unknown name is an error rather than an empty selection, because a
// misspelled context would otherwise read as a gate that passed.
func parseContexts(value string) (selection, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == allContexts {
		chosen := make(map[mdContext]bool, len(structuralContexts))
		labels := make([]string, 0, len(structuralContexts))
		for _, c := range structuralContexts {
			chosen[c] = true
			labels = append(labels, c.String())
		}
		return selection{chosen: chosen, label: strings.Join(labels, ", ")}, nil
	}
	chosen := map[mdContext]bool{}
	var labels []string
	for name := range strings.SplitSeq(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		ctx, ok := contextNamed(name)
		if !ok {
			return selection{}, fmt.Errorf("unknown Markdown context %q: expected %s, or %s", name, allContexts, contextNames())
		}
		if !chosen[ctx] {
			labels = append(labels, ctx.String())
		}
		chosen[ctx] = true
	}
	if len(chosen) == 0 {
		return selection{}, fmt.Errorf("no Markdown context selected: expected %s, or %s", allContexts, contextNames())
	}
	return selection{chosen: chosen, label: strings.Join(labels, ", ")}, nil
}

// contextNamed resolves a context by the name a report prints for it.
func contextNamed(name string) (mdContext, bool) {
	for _, c := range structuralContexts {
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
