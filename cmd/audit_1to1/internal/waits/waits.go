// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Entry categories. An upstream entry waits on client-go and names the
// register row it waits on; every other category is a raw request or a
// request option that waits on nothing upstream, for the reason its category
// names.
const (
	// CategoryUpstream: the workaround exists because client-go does
	// something wrong or does not do it, and retires when the row does.
	CategoryUpstream = "upstream"
	// CategoryGenericSlugDispatch: one handler serves every integration
	// slug, so the route is chosen from the input and no single client-go
	// method stands for it.
	CategoryGenericSlugDispatch = "generic-slug-dispatch"
	// CategoryStreamedDownload: the response is streamed to the caller
	// under a size ceiling rather than read whole into a client-go struct.
	CategoryStreamedDownload = "streamed-download"
	// CategoryURLBuilt: the handler returns the URL for the caller to fetch
	// and sends no request at all.
	CategoryURLBuilt = "url-built"
	// CategoryFixtureProtocol: an e2e fixture speaks a package manager's
	// own protocol to set up a scenario, which no client-go method models.
	CategoryFixtureProtocol = "fixture-protocol"
	// CategoryProbe: an e2e fixture sends whatever request a scenario asks
	// it to, to read GitLab's answer to a token.
	CategoryProbe = "probe"
	// CategoryPagination: a request option that follows the link a keyset
	// page answers with, a protocol of the response rather than a parameter
	// any options struct could carry. Paging a method that takes no options
	// is not this: it waits on the method gaining them, as an upstream entry
	// with a TakesNoOptions predicate per method.
	CategoryPagination = "pagination"
)

// rawCategories lists the categories an entry that waits on nothing takes.
var rawCategories = map[string]bool{
	CategoryGenericSlugDispatch: true,
	CategoryStreamedDownload:    true,
	CategoryURLBuilt:            true,
	CategoryFixtureProtocol:     true,
	CategoryProbe:               true,
	CategoryPagination:          true,
}

// Entry is one workaround this repository carries, and why.
type Entry struct {
	// Row is the row of docs/development/upstream-bugs.md an upstream entry
	// waits on, zero for every other category.
	Row int
	// Category is CategoryUpstream or the reason a raw request waits on
	// nothing.
	Category string
	// Sites are the functions that carry the workaround in a form the sdk
	// scope's walk enumerates (a raw request, a request option written by
	// hand, a body edit, a raw request of the e2e fixture), spelled
	// "<package>.<function>": a tool package by its directory name,
	// "toolutil." for the shared package and "fixture." for the e2e
	// fixture.
	Sites []string
	// Unseen are the functions that carry the workaround in a form the walk
	// does not enumerate (an error hint, a URL a handler builds), which are
	// held to existing and nothing more.
	Unseen []string
	// Replaces are the client-go methods ("Service.Method") or whole
	// services ("Service") the workaround stands in for, from which R-ACTION
	// and the service rule take their COVERED_RAW adjudications.
	Replaces []string
	// Predicates are what is true of the pinned client-go while an upstream
	// entry is needed. Each one that stops holding is a predicate to edit,
	// and all of them stopping is a workaround to retire.
	Predicates []Predicate
	// Reason says what the workaround does and what it waits on.
	Reason string
}

// Label names the entry in a report and in a finding: "row 6" for an
// upstream entry, its category for any other.
func (e Entry) Label() string {
	if e.Category == CategoryUpstream {
		return "row " + strconv.Itoa(e.Row)
	}
	return e.Category
}

// Entries returns every entry, the upstream waits first, as a copy a caller
// may keep.
func Entries() []Entry { return slices.Concat(upstreamWaits, rawReasons) }

// CoveredRawMethods returns the client-go methods the entries replace, keyed
// "Service.Method" as acceptedMissingMethods keys them, each with the entry
// that replaces it and why, which R-ACTION adjudicates as COVERED_RAW.
func CoveredRawMethods(entries []Entry) map[string]string {
	return replaced(entries, true)
}

// CoveredRawServices returns the whole client-go services the entries
// replace, keyed by the service's bare name as declaredServices keys them,
// each with the entry that replaces it and why, which the service rule
// reports as a COVERED_RAW declaration.
func CoveredRawServices(entries []Entry) map[string]string {
	return replaced(entries, false)
}

// replaced collects the methods (methods true) or the services the entries
// replace, the first entry naming one winning; [Problems] reports a second.
func replaced(entries []Entry, methods bool) map[string]string {
	out := map[string]string{}
	for _, entry := range entries {
		for _, key := range entry.Replaces {
			if strings.Contains(key, ".") != methods {
				continue
			}
			if _, seen := out[key]; !seen {
				out[key] = entry.Label() + ": " + entry.Reason
			}
		}
	}
	return out
}

// Problems lists what is wrong with the entries as written, whatever the tree
// says: an upstream entry with no row or no predicate, an entry of any other
// category that carries either, a category nothing defines, an entry with no
// site, and a method or service two entries both replace.
func Problems(entries []Entry) []string {
	var out []string
	replacedBy := map[string]string{}
	for _, entry := range entries {
		out = append(out, shapeProblems(entry)...)
		label := entry.Label()
		for _, key := range entry.Replaces {
			if first, twice := replacedBy[key]; twice {
				out = append(out, "wait "+label+" replaces "+key+", which "+first+" replaces too")
				continue
			}
			replacedBy[key] = label
		}
	}
	slices.Sort(out)
	return out
}

// shapeProblems lists what is wrong with one entry on its own. Each check is
// an if of its own rather than a case of a switch: a case expression carries
// no statement counter, so a mutation tool leaves its mutant unrun.
func shapeProblems(entry Entry) []string {
	label := entry.Label()
	var out []string
	upstream := entry.Category == CategoryUpstream
	if upstream && entry.Row <= 0 {
		out = append(out, "upstream wait "+label+" names no register row")
	}
	if upstream && len(entry.Predicates) == 0 {
		out = append(out, "upstream wait "+label+" carries no predicate")
	}
	if !upstream && !rawCategories[entry.Category] {
		out = append(out, "wait "+label+": category "+strconv.Quote(entry.Category)+" is not one the table defines")
	}
	if !upstream && (entry.Row != 0 || len(entry.Predicates) != 0) {
		out = append(out, "raw reason "+label+" carries a row or a predicate, which only an upstream wait takes")
	}
	if len(entry.Sites)+len(entry.Unseen) == 0 {
		out = append(out, "wait "+label+" names no site")
	}
	return out
}

// RegisterPath is where the register every upstream entry names a row of
// lives, relative to the repository root.
const RegisterPath = "docs/development/upstream-bugs.md"

// registerRow matches a row of the register's summary table: its number, the
// first cell, and the anchor of the first link on the line, which is the
// row's section.
var registerRow = regexp.MustCompile(`^\| (\d+) \|(?:.*?\]\(#([^)]+)\))?`)

// heading matches a Markdown heading: its depth and its text.
var heading = regexp.MustCompile(`^(#+) (.*)$`)

// ReadRows returns each row the register at path declares, by number, with
// the text that describes it: its line of the summary table and the section
// its title links to, up to the next heading as deep or shallower. The text
// is what ties an entry to its row, so a renumbering that leaves an entry
// pointing at another row's problem names nothing the entry is about. The
// register is written by hand rather than generated, so reading it does not
// tie the scope to a regeneration.
func ReadRows(path string) (map[int]string, error) {
	// #nosec G304 -- path is the register's fixed location under the
	// repository root the caller resolved.
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the upstream register: %w", err)
	}
	lines := strings.Split(string(content), "\n")
	sections := readSections(lines)
	rows := map[int]string{}
	for _, line := range lines {
		match := registerRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		number, _ := strconv.Atoi(match[1])
		rows[number] = strings.Join(slices.Concat([]string{line}, sections[match[2]]), "\n")
	}
	return rows, nil
}

// readSections returns the lines of each section of the register, keyed by
// the anchor its heading is linked by. A second heading spelled like an
// earlier one is linked by the anchor with a suffix, so the bare anchor
// stays the first one's.
func readSections(lines []string) map[string][]string {
	sections := map[string][]string{}
	for i, line := range lines {
		match := heading.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		anchor := headingAnchor(match[2])
		if _, seen := sections[anchor]; seen {
			continue
		}
		depth, end := len(match[1]), i+1
		for end < len(lines) && !closesSection(lines[end], depth) {
			end++
		}
		sections[anchor] = lines[i:end]
	}
	return sections
}

// closesSection reports whether line is a heading as deep as depth or
// shallower, which ends a section of that depth.
func closesSection(line string, depth int) bool {
	match := heading.FindStringSubmatch(line)
	return len(match) == heading.NumSubexp()+1 && len(match[1]) <= depth
}

// headingAnchor spells the anchor a heading is linked by: lower case, each
// space a hyphen, letters, digits, hyphens and underscores kept and every
// other mark dropped.
func headingAnchor(text string) string {
	var anchor strings.Builder
	for _, r := range strings.ToLower(text) {
		if r == ' ' {
			anchor.WriteRune('-')
		}
		if r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			anchor.WriteRune(r)
		}
	}
	return anchor.String()
}

// Names lists what the register row an upstream entry waits on has to
// mention for it to be the row the entry means: what each predicate is about
// and each method or service it replaces, a method also by its name alone,
// and each site in full, an unexported function also by its name alone. An
// exported function's name (Set, Update) is left out, as a word any section
// can hold.
func (e Entry) Names() []string {
	names := map[string]bool{}
	for _, predicate := range e.Predicates {
		addName(names, predicate.Subject())
	}
	for _, key := range e.Replaces {
		addName(names, key)
	}
	for _, site := range slices.Concat(e.Sites, e.Unseen) {
		names[site] = true
		function := site[strings.LastIndex(site, ".")+1:]
		if first, _ := utf8.DecodeRuneInString(function); unicode.IsLower(first) {
			names[function] = true
		}
	}
	return slices.Sorted(maps.Keys(names))
}

// addName adds subject to names and, when it is a method ("Service.Method"),
// the method's name alone. A route ("VERB /path") is the one subject with a
// space, and a dot in its path names no method.
func addName(names map[string]bool, subject string) {
	names[subject] = true
	if strings.Contains(subject, " ") {
		return
	}
	if _, method, isMethod := strings.Cut(subject, "."); isMethod {
		names[method] = true
	}
}

// NamedIn reports whether text mentions one of the entry's [Entry.Names] as
// a whole name, not as the start or the end of a longer one.
func (e Entry) NamedIn(text string) bool {
	for _, name := range e.Names() {
		if regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(name) + `([^A-Za-z0-9_]|$)`).MatchString(text) {
			return true
		}
	}
	return false
}
