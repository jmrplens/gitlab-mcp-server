// Package orbit holds the live tests of the six read-only gitlab_orbit_*
// tools against GitLab.com's experimental Knowledge Graph API, on the
// fixtures scripts/setup-orbit-fixtures.sh provisions under
// test/fixtures/orbit.
//
// The tests are an external test package behind the orbitlive build tag and
// need a GitLab.com token; make test-e2e-gitlab-com provisions the fixtures,
// waits for the indexer and runs them. This file is what a plain build sees of
// the package.
package orbit
