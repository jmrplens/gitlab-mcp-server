package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The reasons a declaration may give. Each is a claim about the finding that a
// reviewer can check, and neither says the vulnerability does not matter.
const (
	// categoryFixNotYetAdoptable accepts an advisory against a package of the
	// base image whose fixed version exists and cannot be taken yet: the base
	// image is pinned by digest and nothing runs apk upgrade, so a fixed base
	// package arrives with the Dependabot bump of alpine, which waits out a
	// cooldown. The entry names the day that bump can land, and fails the run
	// from the day after. ca-certificates and tzdata are not candidates: the
	// Dockerfile installs them from Alpine's repository, so a build that does
	// not reuse the cached layer takes their fix.
	categoryFixNotYetAdoptable = "fix-not-yet-adoptable"

	// categoryNotAffected accepts an advisory against a package the image
	// carries when nothing in the image reaches the code it concerns.
	categoryNotAffected = "not-affected"
)

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryFixNotYetAdoptable: "a fixed version of a package the base image carries exists and the image cannot take it until the bump of the base image lands, a package the Dockerfile installs with apk add being no candidate since a build that does not reuse the cached layer takes its fix; the reason names the fixed version and the bump, and until names the day the bump can land",
	categoryNotAffected:        "the image carries the package and nothing in it reaches the code the advisory concerns; the reason names that code and why nothing runs it",
}

// declaration is one accepted finding: why it may ship, in a category and in
// words, and for a fix not yet adoptable the last day, YYYY-MM-DD in UTC, it
// may.
type declaration struct {
	category string
	reason   string
	until    string
}

// acceptedFindings are the findings a release may carry, each with the reason
// that is right, keyed by the advisory's id and the package it was found in,
// separated by one space: "CVE-2026-85091 zlib".
//
// The table is empty, and an entry is an advisory shipped on purpose in every
// image the release publishes, which is why each one needs a category and a
// reason a reviewer can check. The package is part of the key so the same
// advisory reaching the image through another package is a new finding rather
// than one already excused.
var acceptedFindings = map[string]declaration{}

// The shapes of a declaration key's two halves: an advisory id the way Trivy
// reports one against an Alpine package (CVE-2026-85091, GHSA-xxxx-xxxx-xxxx,
// ALPINE-13661), and an Alpine package name.
var (
	advisoryID  = regexp.MustCompile(`^[A-Z][A-Z0-9]*(-[0-9A-Za-z]+)+$`)
	packageName = regexp.MustCompile(`^[a-z0-9][a-z0-9+._-]*$`)
)

// declarationKey is how a finding is named in the declaration table.
func declarationKey(advisory, pkg string) string {
	return advisory + " " + pkg
}

// invalidDeclarations names every declaration that cannot be read as written,
// with what is wrong with it, in a stable order.
func invalidDeclarations(declared map[string]declaration) []string {
	var invalid []string
	for key, entry := range declared {
		if problem := declarationProblem(key, entry); problem != "" {
			invalid = append(invalid, fmt.Sprintf("%q: %s", key, problem))
		}
	}
	sort.Strings(invalid)
	return invalid
}

// declarationProblem is what is wrong with one declaration, or nothing.
func declarationProblem(key string, entry declaration) string {
	advisory, pkg, _ := strings.Cut(key, " ")
	if !advisoryID.MatchString(advisory) || !packageName.MatchString(pkg) {
		return "the key is not an advisory id and a package separated by one space"
	}
	if categories[entry.category] == "" {
		return fmt.Sprintf("the category %q is not one this command defines", entry.category)
	}
	if strings.TrimSpace(entry.reason) == "" {
		return "gives no reason"
	}
	if entry.category == categoryNotAffected {
		if entry.until != "" {
			return fmt.Sprintf("a not-affected entry holds until the image changes and names no until, and this one gives %q", entry.until)
		}
		return ""
	}
	if _, err := time.Parse(time.DateOnly, entry.until); err != nil {
		return fmt.Sprintf("a fix-not-yet-adoptable entry names the day its fix can be taken as until, YYYY-MM-DD, and this one gives %q", entry.until)
	}
	return ""
}
