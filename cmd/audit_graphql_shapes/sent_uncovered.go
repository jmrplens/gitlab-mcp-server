package main

import (
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// internalDir is the one directory of this module the import paths a pairing
// carries and the paths the request inventory records agree on, and so what
// the two lists are compared through.
const internalDir = "internal"

// uncoveredReason says why a package's GraphQL is outside this walk, in the
// terms a reader needs to judge how much the answer is worth.
const uncoveredReason = "The walk pairs a document with a decoder, and both halves must be in the source it " +
	"loads. A package whose operations client-go builds writes neither: the document text lives in the SDK's " +
	"own module and the response is decoded by the SDK's own struct, so there is no send here to pair and no " +
	"decoder here to compare a schema type with. R-PATH cannot see these either, because a GraphQL-only " +
	"endpoint has no REST operation in GitLab's OpenAPI record, which leaves them in exactly the state this " +
	"dimension was built to end. The set is read from the request inventory, so it is what the unit suite was " +
	"seen to send rather than everything these packages can send."

// uncoveredUnavailable is what the report says instead of a set when the
// record naming it could not be read, since an omitted set and an empty one
// would otherwise be indistinguishable and the empty one is the loud wrong
// answer.
const uncoveredUnavailable = "the request inventory could not be read, so how much GraphQL is outside this walk is unknown here"

// sentUncoveredPackage is one package whose GraphQL this walk never saw.
type sentUncoveredPackage struct {
	// Package is the repository-relative path the request inventory records.
	Package string `json:"package"`
	// Operations names the operations recorded for it, so a reader can judge
	// the size of what is missing rather than only its count.
	Operations []string `json:"operations"`
}

// sentUncovered is the GraphQL the request inventory records that this walk
// asked nothing of, named in the report rather than left for a reader to
// discover from a package list that happens to be short.
type sentUncovered struct {
	// Source names the record the set is read from.
	Source string `json:"source"`
	// Reason says why these are outside the walk.
	Reason string `json:"reason"`
	// Unavailable is set instead of Packages when the record could not be
	// read.
	Unavailable string `json:"unavailable,omitempty"`
	// Packages and Operations are the set and its size.
	Packages   []sentUncoveredPackage `json:"packages,omitempty"`
	Operations int                    `json:"operations"`
}

// uncoveredGraphQL lists the packages the request inventory records GraphQL
// operations for that no pairing of this run touched.
//
// Covered means a pairing was made in the package or the document was handed
// over from it, not that a finding came out of it: a package whose every
// object is traversed rather than read is asked nothing and is still inside
// the walk, and calling it uncovered would confuse a bounded question with an
// absent one.
func uncoveredGraphQL(inventory requestinventory.Inventory, pairings []pairing) sentUncovered {
	covered := map[string]bool{}
	for _, p := range pairings {
		covered[repoRelative(p.Package)] = true
		if p.OriginPackage != "" {
			covered[repoRelative(p.OriginPackage)] = true
		}
	}

	byPackage := map[string][]string{}
	for _, row := range inventory.Requests {
		if row.Kind != requestinventory.KindGraphQL || covered[row.Package] {
			continue
		}
		byPackage[row.Package] = append(byPackage[row.Package], row.Operation)
	}

	uncovered := sentUncovered{Source: requestinventory.Path, Reason: uncoveredReason}
	for pkg, operations := range byPackage {
		sort.Strings(operations)
		uncovered.Packages = append(uncovered.Packages, sentUncoveredPackage{Package: pkg, Operations: operations})
		uncovered.Operations += len(operations)
	}
	sort.Slice(uncovered.Packages, func(i, j int) bool {
		return uncovered.Packages[i].Package < uncovered.Packages[j].Package
	})
	return uncovered
}

// unavailableUncovered is what the report carries when the record could not be
// read, which a fixture run has no reason to ship.
func unavailableUncovered() sentUncovered {
	return sentUncovered{Source: requestinventory.Path, Reason: uncoveredReason, Unavailable: uncoveredUnavailable}
}

// repoRelative trims a Go import path to the repository-relative path the
// request inventory records a package under, which is what the two lists are
// compared by. An import path outside internal is left alone: it matches
// nothing in the inventory either way, and inventing a shorter spelling for it
// could make it match the wrong row.
func repoRelative(pkgPath string) string {
	if at := strings.Index(pkgPath, "/"+internalDir+"/"); at >= 0 {
		return pkgPath[at+1:]
	}
	return pkgPath
}
