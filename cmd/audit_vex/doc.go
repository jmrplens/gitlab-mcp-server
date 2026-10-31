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
// dependency set.
//
// The dependency set is read the way make govulncheck reads it: govulncheck
// runs in this process, through golang.org/x/vuln/scan at the version go.mod
// pins, over the packages and build tags the command line names (the
// Makefile passes the ones make govulncheck scans), and reports every
// advisory against a module the loaded packages come from, at the deepest
// level it reaches: the module, an imported vulnerable package, or a called
// vulnerable function. Against that, three findings fail the run:
//
//   - stale: a statement names an advisory and a module the scan does not
//     report together, so it describes a dependency set that is gone;
//   - contradicted: a not_affected statement whose justification the scan
//     disproves, vulnerable_code_not_present or component_not_present for an
//     advisory whose package is imported, and
//     vulnerable_code_not_in_execute_path for one whose function is called;
//   - unaccounted: the scan reports an advisory against a module and finds
//     none of its functions called, which is a vulnerability in a component
//     that does not affect the project, and no statement about this module
//     names it. OpenSSF Baseline OSPS-VM-04.02 asks for exactly those to be
//     in the document.
//
// An advisory whose function is called is not required here: make
// govulncheck already fails on it, and a statement would not make it pass.
//
// # What it cannot see
//
// The scan reads the modules the loaded packages come from, so a requirement
// of go.mod that no package under the scanned build tags imports is not in
// the set, although a scanner reading go.mod alone (osv-scanner, OpenSSF
// Scorecard) reports its advisories. Products pinned to a released image are
// held to the current dependency set through their subcomponents, not to the
// image itself, which the command never fetches: that a digest is the image
// its statement describes is the reviewer's to check, and the impact
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
