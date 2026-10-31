// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_vex holds the repository's OpenVEX document,
// vex/gitlab-mcp-server.openvex.json, to the specification it claims and to
// the dependency set it describes.
//
// # Why the document needs a gate
//
// A VEX document is a set of claims about advisories: this one does not
// affect the product, for this reason. Scanners act on it by hiding what it
// covers, so a statement that has stopped being true hides an advisory that
// matters, and one that names an advisory nothing carries any longer is an
// excuse waiting for the next advisory against the same module. The release
// attaches the document to every GitHub release, so whatever it says on the
// tagged tree is what a user's scanner is told about that release.
//
// # What it checks
//
// The document is read as OpenVEX v0.2.0, refusing a member the
// specification does not define or that is written twice, and every
// statement is held to the specification's rules: a known status, a
// justification from the five the specification lists and an impact
// statement for not_affected (the specification asks for either, this
// repository for both), an action statement for affected, and RFC 3339
// times. Two rules are this repository's rather than the specification's.
// fixed is refused, because an advisory a dependency update fixed is one the
// dependency set no longer contains, which is the stale case below, and the
// statement leaves the document instead. And every product is either this
// module at every version, pkg:golang/<the module go.mod names>, or its image
// pinned by the sha256 digest of an index with the repository_url it was
// published to, and every product names the Go module the advisory is
// against as a subcomponent, which is what ties the statement to the
// dependency set. A subcomponent of an image names the version that image
// carries, since a published image is one build.
//
// The dependency set is read the way make govulncheck reads it: govulncheck
// runs in this process, through golang.org/x/vuln/scan at the version go.mod
// pins, over the packages and build tags the command line names (the
// Makefile passes the ones make govulncheck scans), and reports the modules
// the loaded packages come from and every advisory against one of them, at
// the deepest level it reaches: the module, an imported vulnerable package,
// or a called vulnerable function. go.mod's require directives are read
// beside it, since they are what a scanner reading go.mod alone
// (osv-scanner, OpenSSF Scorecard) reports advisories against. Against those
// two, what a statement says of this module's own product is held, and three
// findings fail the run:
//
//   - stale: the scan loads the module and does not report the advisory
//     against it, or go.mod no longer requires the module at all, so the
//     statement describes a dependency set that is gone;
//   - contradicted: the scan reports the advisory's function called, which
//     make govulncheck fails on whatever a statement says, so no statement
//     of any status names it; or a not_affected statement whose justification
//     the scan disproves, vulnerable_code_not_present or
//     component_not_present for an advisory whose package is imported;
//   - unaccounted: the scan reports an advisory against a module and finds
//     none of its functions called, which is a vulnerability in a component
//     that does not affect the project, and no statement about this module
//     names it. OpenSSF Baseline OSPS-VM-04.02 asks for exactly those to be
//     in the document.
//
// A module go.mod requires that no loaded package comes from, because only a
// test or a file under another build tag imports it, is outside what the
// scan can speak for, and a statement about it stands on go.mod's word: that
// is the case a scanner reading go.mod reports and the scan never would.
//
// What a statement says of a published image is not held to the current
// tree at all. The image is pinned by its digest and does not change, so the
// day an advisory's module leaves go.mod, the module's own product leaves the
// statement and the image products stay, for as long as those images are
// published.
//
// # What it cannot see
//
// The scan reads the modules the loaded packages come from, so an advisory
// against a module go.mod requires and no package under the scanned build
// tags imports is never reported, and never unaccounted, although a scanner
// reading go.mod alone reports it; a statement about such a module, or about
// an image, is not checked against the database either, which nothing in the
// scan describes. The command never fetches an image: that a digest is the
// release its statement says, and that the version its subcomponent names is
// the one that release carries, is the reviewer's to check, and the impact
// statement says which release each one is.
//
// # Exit codes
//
// 0 when the document reads as OpenVEX and every statement holds; 1 when it
// does not, when a statement is stale or contradicted, when an advisory is
// unaccounted for, or when the document, go.mod or the scan could not be
// read; 2 for arguments that do not parse. The database is vuln.go.dev
// unless -db names another, which is how the tests run offline.
package main
