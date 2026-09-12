// Package ce holds the end-to-end tests that only hold on a GitLab with no
// license.
//
// It is deliberately small. Almost every Free action behaves the same under
// a license, and those tests live in the common package so both runtimes run
// them. What belongs here is a fact that a license changes: an endpoint that
// answers 404 until one is installed, a listing that gains a group. A fact
// that holds only on the Community image itself, and not on an Enterprise
// image left unlicensed, declares that and skips with a reason elsewhere.
//
// The tests carry the e2e build tag; this file is what a plain build sees of
// the package.
package ce
