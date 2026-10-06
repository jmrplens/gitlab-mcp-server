// Command audit_supply_chain audits the release supply chain's configuration
// invariants.
//
// Five properties, each of which was false at some point and each of which is
// invisible to every other gate in this repository:
//
//  1. Every uses: in .github/workflows is pinned to a 40-character commit SHA.
//     A mutable tag is resolved by the runner at job start, so a hijacked v7 is
//     consumed with no pull request, no cooldown and no review.
//  2. A credentialed job runs no code resolved at run time. A job is
//     credentialed when its permissions grant contents: write, id-token: write
//     or packages: write, or when it reads a secret the secret table
//     (secrets.go) says can write. That means no npx, no @latest, no curl
//     piped into a shell and no unhashed pip install, in its own run: blocks or
//     in anything those blocks reach: a script under any scripts/ directory
//     (.github/scripts/ included), read relative to the step's
//     working-directory; a script a file names through its own location
//     ("$(dirname "$0")/x.sh", a variable it assigns from that, as many
//     directories up as it climbs, or $PSScriptRoot), read beside that file;
//     a make target's recipe and prerequisites, read from the directory and
//     the makefile its -C and -f options name; and whatever each of those
//     runs in turn, however deep. A make is followed in command position,
//     behind variable assignments and the wrappers env, exec, command, nohup,
//     time, nice, timeout and sudo, inside the string sh -c is handed, and as
//     $(MAKE) wherever it is written. actions/checkout leaves no credential in
//     .git/config; and a tool the job downloads (GoReleaser, syft) is pinned
//     to an exact version, because SHA-pinning the action that fetches a
//     binary does not pin the binary.
//
// Rule 2 refuses what it cannot read rather than passing over it: a script
// reference that resolves to no readable file in the repository, one rooted in
// a variable or an expression whose value only the run knows, a make target
// with no rule, a make that names no target, passes an option it does not read
// or --eval, or reads more than one makefile, a make behind a wrapper written
// in a way it does not read (env -C, xargs, eval), a secret the table does not
// declare and an expression that reads every secret at once are each a
// finding. Both of its tables are held to the tree as well: a secret the
// workflows no longer read, and a declaration in declarations.go that excuses
// no match, are reported as stale.
//
// What rule 2 does not see, by construction, is a command that is not written
// as one: a make or a script handed to a language's process or path API as an
// argument (spawnSync("make", ...), subprocess.run(["make", ...]),
// os.path.join(ROOT, "scripts", "x.sh"), path.join(__dirname, "x.mjs")), and a
// make run through a shell function or a variable holding its name
// (run_check make target, "$MAKE" target). A make mentioned in prose is not a
// command either, which is why a mention it does not read is not a finding:
// the files a credentialed job reaches mention make in their messages. A test
// pins each of these as not followed.
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
// This began as a port of a Python auditor and still prints the findings that
// auditor printed byte for byte, down to the repr() quoting three messages
// embed; everything rule 2 reads beyond a run: block and the scripts/ file it
// names directly is this program's own. It diverges in one place:
// PyYAML accepted a duplicated mapping key and silently kept the last value, so
// a step written with two run: keys was audited as though one of them did not
// exist. This refuses the document instead, failing closed on a workflow whose
// meaning is ambiguous rather than auditing a guess about it.
package main
