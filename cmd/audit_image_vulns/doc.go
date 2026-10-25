// Command audit_image_vulns holds the operating-system packages of the
// container image to Trivy's vulnerability database, one Trivy report per
// platform, before the image is pushed.
//
// # Why the image needs a gate of its own
//
// The runtime stage of the Dockerfile is alpine pinned by digest plus
// ca-certificates and tzdata, and nothing runs apk upgrade, so the image
// ships whatever Alpine packages that digest and those two installs carry.
// make check-binary-vulns holds the server binary to the Go vulnerability
// database; nothing held the Alpine packages to anything until the image was
// public, when a scanner reading the published tag (trivy image, Grype, a
// directory that scores the server) reported whatever advisory applied to
// them. Trivy is the reference because `trivy image` on the published tag is
// what a user runs, so the release asks the question the way that user will.
//
// # What it reads
//
// The release workflow builds each platform of the image on its own, runs
// it, and then, before the multi-platform push, saves each smoke image and
// scans it with `trivy image --input`, writing Trivy's JSON report with every
// package listed. This command reads those reports and the output of
// `trivy version --format json`, which names the database the scans were
// judged against. It runs nothing and fetches nothing, and imports nothing but
// the standard library, because the job it runs in holds the registry push.
//
// A report is refused rather than read as clean when it cannot vouch for the
// image: one that is not Trivy's schema 2, one that names no platform in its
// image configuration, one in which Trivy detected no operating system, one
// whose distribution is past its end of service life, when advisories stop
// being published for it, one that lists no operating-system package, one
// that rates a finding with a severity Trivy does not define, and one with a
// finding in a class of result other than the image's operating-system
// packages and its language packages, which is all a vulnerability scan
// writes. The reports of
// a run are held to the platforms -platforms names (by default the two the
// release pushes, which a test holds to release.yml): each scanned once,
// none missing, nothing else. The database is refused when trivy version names
// none, and when its own NextUpdate had passed before the run, which is the
// moment Trivy itself calls it stale, so the bound is Trivy's rather than a
// number chosen here. Each refusal is printed on stderr and fails the run.
//
// # How a finding is judged
//
// A finding against an operating-system package fails the run when it has a
// fixed version and Trivy rates it HIGH or CRITICAL, unless the declaration
// table in declarations.go accepts it. Everything else is printed and does not
// gate: a CRITICAL with no fix to take, and a fix rated MEDIUM, LOW or
// UNKNOWN, the last being what Trivy reports when no source rates an advisory
// at all. The rating is the one Trivy selects by default, which is what a user
// running trivy image sees: the advisory's own data source first, which for
// Alpine is the secdb and rates nothing, then NVD, then the rating Trivy's
// database merges from other vendors, and UNKNOWN when nobody rates it. Every
// finding is printed with where its rating came from: Trivy's SeveritySource
// when it names one, and otherwise the vendors whose ratings Trivy merged.
//
// Findings against the server binary's modules are printed too, as trivy image
// prints them, and never judged here: make check-binary-vulns holds the binary
// to the Go vulnerability database at module grain, and each question keeps
// one oracle.
//
// Findings are merged across the platforms by advisory, package and version,
// so one advisory against a package both images carry is one line naming both.
//
// # The declaration table
//
// An entry is keyed by the advisory and the package, separated by one space
// ("CVE-2026-85091 zlib"), and carries a category, a reason and, for one
// category, a date. fix-not-yet-adoptable is the expected one: the base image
// is pinned by digest, so a fixed base package arrives with the Dependabot
// bump of alpine, which waits out the docker ecosystem's cooldown, and the
// entry names the day that bump can land as until; from the next day in UTC
// the entry is expired and fails the run. It is for a package of the base
// image only: ca-certificates and tzdata are installed from Alpine's
// repository when the image is built, so their fix arrives with the next
// build that does not reuse the cached layer, with no bump to wait for.
// not-affected says the image carries the package and does not reach the code
// the advisory concerns, and names no day, since it holds until the image
// changes. A declaration that accepts no gating finding of a run that scans
// every platform the release pushes is stale and fails that run too: an entry
// that outlives what it excused is how a table comes to excuse the next
// advisory against the same package unread. A run that -platforms tells to
// scan fewer cannot tell a stale entry from one that accepts another
// platform's finding, so it prints such an entry as unjudged and does not
// fail on it. The table is empty and is meant to stay so.
//
// # Usage
//
//	go run ./cmd/audit_image_vulns/ [-platforms linux/amd64,linux/arm64] -db trivy-version.json report.json...
//
// Exit codes: 0 when every gating finding is declared and every declaration
// matched, or was left unjudged, and is current; 1 when a finding is
// undeclared, a declaration is
// stale, expired or malformed, or a report or the database is refused; 2 for
// arguments that do not parse. A run that could not be judged says so on
// stderr and a run that found something says it on stdout.
package main
