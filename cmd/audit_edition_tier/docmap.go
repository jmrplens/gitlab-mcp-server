package main

// docAreaForPackage maps an owner package name to the GitLab API doc area
// (the doc/api/<area>.md basename). Returns ok=false when no mapping exists so
// the domain is reported as needing manual triage.
//
// The map is intentionally explicit rather than heuristic: GitLab doc filenames
// do not follow a single rule from the Go package name, and a wrong mapping
// would silently grade an action against the wrong tier.
func docAreaForPackage(pkg string) (area string, ok bool) {
	area, ok = docAreaByPackage[pkg]
	return area, ok
}

// docAreaByPackage is populated in docmap_data.go.
var docAreaByPackage = map[string]string{}
