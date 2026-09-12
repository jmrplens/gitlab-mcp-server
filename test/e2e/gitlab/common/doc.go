// Package common holds the end-to-end tests that run on every GitLab runtime.
//
// The actions it covers are Free ones, and a license changes how they are
// served rather than whether they exist: a licensed catalog prunes schemas
// differently and carries more groups, so both Docker targets run this
// package, on the Community image and on the licensed Enterprise one. What
// only holds without a license lives in the ce package beside it, and what
// needs one in ee.
//
// Every test drives the real cmd/server binary over stdio through the
// harness, names its actions by canonical catalog ID, and runs each scenario
// on the dynamic, meta and individual surfaces as subtests. The tests carry
// the e2e build tag; this file is what a plain build sees of the package.
package common
