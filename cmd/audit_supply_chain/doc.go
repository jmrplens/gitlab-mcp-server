// Command audit_supply_chain audits the release supply chain's configuration
// invariants.
//
// Five properties, each of which was false at some point and each of which is
// invisible to every other gate in this repository:
//
//  1. Every uses: in .github/workflows is pinned to a 40-character commit SHA.
//     A mutable tag is resolved by the runner at job start, so a hijacked v7 is
//     consumed with no pull request, no cooldown and no review.
//  2. A job holding contents: write or id-token: write runs no code resolved at
//     run time. That means: no npx, no @latest, no curl piped into a shell, no
//     unhashed pip install — in its own run: blocks or in any scripts/ file
//     those blocks invoke; actions/checkout leaves no credential in
//     .git/config; and a tool the job downloads (GoReleaser, syft) is pinned to
//     an exact version, because SHA-pinning the action that fetches a binary
//     does not pin the binary.
//  3. Dependabot states its cooldown instead of inheriting a platform default
//     that GitHub can change under us.
//  4. SECURITY.md names the major version the repository actually ships.
//  5. Both installers verify the release's Sigstore bundle, not only a
//     checksums.txt fetched from the same mutable release.
//
// Usage:
//
//	go run ./cmd/audit_supply_chain/ [--root <dir>]
//
// Exits non-zero and prints one line per violation.
//
// The auditor is deliberately split in two: pinning is decided on the raw file
// text, so a uses: inside a comment or an unparsed region still counts, while
// job structure comes from the parsed YAML.
//
// This is a port of a Python auditor and reproduces its findings byte for byte,
// down to the repr() quoting three messages embed. It diverges in one place:
// PyYAML accepted a duplicated mapping key and silently kept the last value, so
// a step written with two run: keys was audited as though one of them did not
// exist. This refuses the document instead, failing closed on a workflow whose
// meaning is ambiguous rather than auditing a guess about it.
package main
