// Package ee holds the end-to-end tests that need a licensed GitLab, Premium
// or Ultimate.
//
// It covers the Premium and Ultimate actions, the resources only a licensed
// instance serves, and the licensed behavior of Free actions where the two
// differ. A test that needs Ultimate in particular declares it with
// harness.Needs(harness.Tier(edition.Ultimate)) and skips on a Premium
// instance; the push-time static gate checks that every test naming an
// Ultimate action carries that declaration.
//
// The licensed run stays local: the license expires and is renewed by hand,
// so no CI job runs this package. The tests carry the e2e build tag; this
// file is what a plain build sees of the package.
package ee
