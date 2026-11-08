// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"regexp"
	"slices"
)

// upstreamURL matches a full gitlab.com URL to a merge request, an issue, an
// incident, a work item or an epic, up to its number: GitLab reads such a URL
// in a commit message as a reference to that item, whatever project or group
// it belongs to. GitLab's link patterns take the /-/ before the route as
// optional and read a group's address (groups/<path>) for a work item or an
// epic, so the path before the route is any run of segments, the - among
// them. The reference is the first group.
var upstreamURL = regexp.MustCompile(`(?i)\b(https?://gitlab\.com/(?:[\w.-]+/)+(?:merge_requests|issues(?:/incident)?|work_items|epics)/\d+)`)

// upstreamPath matches a reference GitLab resolves across projects: a
// project's full path, its groups and nested groups included, followed by !N
// for a merge request or #N for an issue or a work item, and a group's full
// path followed by &N for an epic. Either may open with the / GitLab reads as
// the start of an absolute path. The path must not continue one already
// started, so a URL or a longer path is not read from its middle; the
// reference itself is the first group.
var upstreamPath = regexp.MustCompile(`(?:^|[^\w./-])(/?(?:[\w.-]+(?:/[\w.-]+)+[!#]|[\w.-]+(?:/[\w.-]+)*&)\d+)\b`)

// upstreamBracketed matches the bracketed spelling GitLab resolves as well:
// [issue:<path>/N], [work_item:<path>/N] and [epic:<path>/N]. An issue's path
// needs a group and a project to leave the namespace of the project the
// commit is in, as with #N; a work item or an epic can belong to a group,
// whose path alone names it. The reference is the first group.
var upstreamBracketed = regexp.MustCompile(`(\[(?:issue:/?[\w.-]+(?:/[\w.-]+)+|(?:work_item|epic):/?[\w.-]+(?:/[\w.-]+)*)/\d+\])`)

// upstreamSpellings are the three spellings, each holding the reference as
// its first group.
var upstreamSpellings = []*regexp.Regexp{upstreamURL, upstreamPath, upstreamBracketed}

// upstreamReferences returns the references to another project's or group's
// merge requests, issues, work items and epics that line carries, in the
// order it carries them and each once. A bare #N or !N is not one, and
// neither is a project's path alone before it: GitLab resolves those in the
// project the commit is in, or in its namespace, which for this repository is
// its own mirror's.
func upstreamReferences(line string) []string {
	type found struct {
		at  int
		ref string
	}
	var all []found
	for _, spelling := range upstreamSpellings {
		for _, match := range spelling.FindAllStringSubmatchIndex(line, -1) {
			all = append(all, found{at: match[2], ref: line[match[2]:match[3]]})
		}
	}
	slices.SortFunc(all, func(a, b found) int { return cmp.Compare(a.at, b.at) })
	var refs []string
	for _, f := range all {
		if !slices.Contains(refs, f.ref) {
			refs = append(refs, f.ref)
		}
	}
	return refs
}
