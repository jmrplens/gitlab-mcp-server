// Command audit_binary_vulns builds every binary the release publishes and
// holds each one to the vulnerability database at module grain, which is how
// every scanner that reads a shipped binary or its SBOM judges it.
//
// # Why the source scan is not enough
//
// `make govulncheck` asks whether our code calls a vulnerable symbol, and that
// is the question that decides whether a vulnerability is real. It is not the
// question a user's scanner asks. Trivy, Grype, osv-scanner, Docker Scout,
// Dependency-Track and verifymcp.io read the modules named in a binary's build
// information, or in the SBOM generated from it, and report every advisory
// against any version of any of them, called or not. `govulncheck -mode binary
// -scan module` asks the same question the same way.
//
// The two answers diverged for 3.0.0 and 3.1.0. Those releases no longer linked
// x/crypto's openpgp packages, which the self-update subsystem removed in 3.0.0
// had, but internal/telemetry imported golang.org/x/crypto/hkdf, which kept the
// module in the build information of every server binary and in the SBOM of
// every image, so every one of those scanners reported GO-2026-5932, an
// advisory against openpgp, while the source scan passed, correctly, because
// nothing linked it. This command is the check that would have said so before a
// release rather than after.
//
// # What it builds
//
// The targets are read from .goreleaser.yml rather than listed here, so a
// target added to the release is scanned without anybody remembering to add it:
// every entry of builds, each goos crossed with each goarch, built from the
// entry's main package with its env and flags. The operating system decides the
// module set and a linux-only scan would miss what only the others link:
// measured at 3.1.0, the darwin builds link github.com/ebitengine/purego and
// the windows builds github.com/go-ole/go-ole and github.com/yusufpapurcu/wmi,
// none of which a linux build carries. The architecture decides nothing today,
// and every pair is built anyway, since a dependency is free to split on it.
//
// The entry's ldflags are not passed. They carry GoReleaser templates this
// command does not evaluate, and they set two strings and strip the symbol
// table, none of which changes which modules a binary links, which is all a
// module-grain scan reads. An entry that selects its targets with ignore or
// targets rather than plain goos and goarch lists is refused rather than read
// half right.
//
// # How a finding is judged
//
// govulncheck runs in this process, through golang.org/x/vuln/scan at the
// version go.mod pins, once per binary, with -format json. Findings are merged
// across the binaries by advisory and module, so one advisory against a module
// every binary links is one finding naming every target.
//
// A finding passes only when the declaration table in declarations.go accepts
// it, keyed by the advisory and the module, with a category and a reason. The
// table is empty and is meant to stay so. A declaration that matches no
// finding of the run is itself a failure, since every run scans every target
// the release builds: an entry that outlives what it excused is how an
// allowlist comes to excuse the next advisory against the same module unread.
//
// # Exit codes
//
// 0 when every finding is declared and every declaration matched; 1 when a
// finding is undeclared, a declaration is stale or malformed, or the build or
// the scan could not be done; 2 for arguments that do not parse. The database
// is vuln.go.dev unless -db names another, which is how the tests run offline.
package main
