// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"slices"
	"strconv"
	"strings"
)

// Entry categories. An upstream entry waits on client-go and carries the
// predicates that say so; every other category is a raw request or a request
// option that waits on nothing upstream, for the reason its category names.
const (
	// CategoryUpstream: the workaround exists because client-go does
	// something wrong or does not do it, and retires when its predicates
	// stop holding.
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

// Label names the entry in a report and in a finding: "upstream", the first
// function that carries it and, when it replaces any, the first client-go
// method or service it replaces, for an upstream entry; its category for any
// other. Two upstream entries can share their first function (one helper
// sending the requests of two unrelated waits), so the replaced method is
// what tells them apart, and [Problems] refuses two that still read alike.
func (e Entry) Label() string {
	if e.Category != CategoryUpstream {
		return e.Category
	}
	label := CategoryUpstream
	if sites := slices.Concat(e.Sites, e.Unseen); len(sites) > 0 {
		label += " " + sites[0]
	}
	if len(e.Replaces) > 0 {
		label += " for " + e.Replaces[0]
	}
	return label
}

// Entries returns every entry, the upstream waits first, as a copy a caller
// may keep: each entry's lists are copied too, so writing into a site or a
// predicate of the result leaves the table as it was. A predicate holds
// strings alone, so copying the list copies it.
func Entries() []Entry {
	entries := slices.Concat(upstreamWaits, rawReasons)
	for i := range entries {
		entries[i].Sites = slices.Clone(entries[i].Sites)
		entries[i].Unseen = slices.Clone(entries[i].Unseen)
		entries[i].Replaces = slices.Clone(entries[i].Replaces)
		entries[i].Predicates = slices.Clone(entries[i].Predicates)
	}
	return entries
}

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
// says: an upstream entry with no predicate, an entry of any other category
// that carries one, a category nothing defines, an entry with no site, a
// method or service two entries both replace, and two upstream entries a
// report would name alike, since a finding or a COVERED_RAW reason naming
// that label could not say which wait it means.
func Problems(entries []Entry) []string {
	var out []string
	replacedBy := map[string]string{}
	upstreamLabels := map[string]bool{}
	for _, entry := range entries {
		out = append(out, shapeProblems(entry)...)
		label := entry.Label()
		if entry.Category == CategoryUpstream {
			if upstreamLabels[label] {
				out = append(out, "two upstream waits are both labeled "+label)
			}
			upstreamLabels[label] = true
		}
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
	if upstream && len(entry.Predicates) == 0 {
		out = append(out, "upstream wait "+label+" carries no predicate")
	}
	if !upstream && !rawCategories[entry.Category] {
		out = append(out, "wait "+label+": category "+strconv.Quote(entry.Category)+" is not one the table defines")
	}
	if !upstream && len(entry.Predicates) != 0 {
		out = append(out, "raw reason "+label+" carries a predicate, which only an upstream wait takes")
	}
	if len(entry.Sites)+len(entry.Unseen) == 0 {
		out = append(out, "wait "+label+" names no site")
	}
	return out
}
