// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_supply_chain audits the release supply chain's configuration
// invariants.
//
// Seven properties, each of which was false at some point and each of which is
// invisible to every other gate in this repository:
//
//  1. Every uses: in .github/workflows is pinned to a 40-character commit SHA,
//     and every docker:// image a step names, or a container action's
//     runs.image, to the sha256 digest of its manifest. A mutable tag is
//     resolved by the runner at job start, so a hijacked v7 is consumed with no
//     pull request, no cooldown and no review, and an image tag moves the same
//     way, under the pin of the action that names it as much as in a step.
//     Every uses: is read as text, line by line, and again as YAML, every key
//     named uses at any depth, so a step written as a flow mapping or a flow
//     sequence, behind a quoted key, with a space before the colon, with its
//     value on the next line or through an anchor or an alias is held to the
//     rule as well as the block spelling is (issue 1320).
//  2. A credentialed job runs no code resolved at run time. A job is
//     credentialed when its permissions grant contents: write, id-token: write
//     or packages: write, or when it reads a secret the secret table
//     (secrets.go) says can write. That means no npx, no @latest, no curl
//     piped into a shell, no unhashed pip install (pip3, pip3.12 and a quoted
//     path to pip included), no cargo binstall (which
//     installs a prebuilt binary whatever version it is given), no cargo
//     install without --locked and an exact version or a --path, no go install
//     at anything but an exact version or a full commit, and no download
//     handed to PowerShell's Invoke-Expression, in its own run: blocks or
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
//     .git/config. SHA-pinning the action that fetches a binary does not pin
//     the binary, so goreleaser/goreleaser-action names GoReleaser's exact
//     version, and the actions that fetch syft and Trivy do not run there at
//     all: anchore/sbom-action runs an install script from syft's main
//     branch; aquasecurity/setup-trivy installs the release its version input
//     names, latest by default, through an install script that checks the
//     tarball only against the checksum file of the same release;
//     aquasecurity/trivy-action runs setup-trivy with cache: true by default,
//     which restores the binary from an Actions cache another run saved; and
//     in March 2026 (GHSA-69fq-xp46-6x23) the tags of both were force-pushed
//     to commits that stole the job's credentials. A job fetches syft or
//     Trivy by hand instead, the release tarball pinned by SHA-256 and its
//     checksum file verified with cosign: the digest pins the bytes, and this
//     rule does not read the version the job names.
//
// Of a Go command a credentialed job runs (go run, go build, go install or go
// test, wherever rule 2 reads a command), rule 2 demands that every package
// it builds, or the .go files it is handed, is this module's and imports,
// however deep, nothing outside the standard library and this module, since
// any code the job runs can mint the job's OIDC token or use its write. The
// imports are read from the source, every file whatever its build constraints
// and test files for go test alone, against the module the root's go.mod
// names; the meta-patterns all, tool and work name other modules' packages and
// are refused, std and cmd being the toolchain's own. go generate, which runs
// what its directives name, and a go tool the toolchain does not ship are
// refused, and so is go run of a package at a version (go-run-module), which
// builds a module this checkout does not hold. So is an option that runs
// another program or builds what the source does not show (-toolexec, -exec,
// -overlay, -modfile, -pkgdir, -linkshared, and go vet's -vettool), on the
// command line or on GOFLAGS, which every later go command reads: GOFLAGS is
// read wherever a text rule 2 reads assigns it as a word, and in the env of
// the workflow, the job and every step. actions/setup-go in such a job sets
// cache: false, so what it builds is downloaded and checked against go.sum
// rather than restored from a cache another run saved. A tool another job of
// the workflow builds with go build or go install and uploads is taken only by
// a job that needs that job, from this run, built from a checkout of this
// run's commit, and checked in the step right after its download with
// sha256sum against an output of the builder that a step of it computed with
// sha256sum; and the builder is held to rule 2 as the job running its tool is,
// however long the chain of builders, since what it builds runs with that
// job's credential and the digest vouches for whatever it built. Only its
// checkout is not held to persist-credentials, since the token it would leave
// there writes nothing. The configuration a step of
// goreleaser/goreleaser-action runs is read too, the file --config or -f names
// or the first of GoReleaser's six names in its workdir: every hook, the
// binary every upx entry runs, and the command and arguments of every sign,
// sbom and publisher, each judged in the directory GoReleaser runs it in (an
// sbom in dist, anything else in its own dir or where GoReleaser runs), which
// is how the go run of cmd/gen_third_party_notices in the release job's sboms
// entry is judged; a build made by a builder other than go's or by a tool other
// than the go command, an option on a build's flags or an override's, written
// as a list or as one string, that rule 2 refuses on a command line, and a
// GOFLAGS any of its env lists sets are findings too.
//
// Rule 2 refuses what it cannot read rather than passing over it: a script
// reference that resolves to no readable file in the repository, one rooted in
// a variable or an expression whose value only the run knows, a make target
// with no rule, a make that names no target, passes an option it does not read
// or --eval, or reads more than one makefile, a make behind a wrapper written
// in a way it does not read (env -C, xargs, eval), a go command behind one, a
// go option it does not know, a package only the run knows, a GoReleaser
// template in a GOFLAGS or in a build flag that is not an option, a GoReleaser
// configuration it cannot find or parse, a secret the table does not declare
// and an expression that reads every secret at once are each a finding. Its
// tables are held to the tree as well: a secret the workflows no longer read,
// and a declaration in any table of declarations.go that excuses nothing, are
// reported as stale.
//
// What rule 2 does not see, by construction, is a command that is not written
// as one: a make or a script handed to a language's process or path API as an
// argument (spawnSync("make", ...), subprocess.run(["make", ...]),
// os.path.join(ROOT, "scripts", "x.sh"), path.join(__dirname, "x.mjs")), and a
// make or a go command run through a shell function or a variable holding its
// name (run_check make target, "$MAKE" target, "$GO" run). A cd in front of a
// go command is not followed, so its packages are resolved from the directory
// the text runs in, and one that resolves to no package there is a finding. A
// tool a builder makes through a make target or a script of its own, rather
// than in its run blocks, is not recognized as one, and what GoReleaser's
// builds compile is the release itself, which no step runs and
// cmd/audit_binary_vulns judges. A GOFLAGS a command writes from a value it
// builds (printf 'GOFLAGS=%s'), rather than as GOFLAGS=value, is not read
// either. A make or a go command mentioned in prose is
// not a command either, which is why a mention it does not read is not a
// finding: the files a credentialed job reaches mention make in their
// messages. A test pins each of these as not followed. Each pattern also reads
// one line, and an install one command of it: a download kept in a variable
// or a file and run on a later line is not matched, while an install whose
// --locked or --require-hashes sits on a continuation line is refused, since
// the line is all the pattern reads.
//  3. Dependabot states its cooldown instead of inheriting a platform default
//     that GitHub can change under us.
//  4. SECURITY.md names the major version the repository actually ships.
//  5. Both installers verify the release's Sigstore bundle, not only a
//     checksums.txt fetched from the same mutable release.
//  6. Every script in another language than Go the tree holds is declared in
//     declarations.go with the reason it has to stay, since repository
//     tooling is a Go command under cmd/ (or under test/e2e/ for the e2e
//     orchestration). A script is a file with a shell, Node, Python,
//     PowerShell, Ruby or Perl extension, or one with no extension that opens
//     with #!, found the way the other walkers here find source, except that
//     the dot-directories at the root this repository tracks files in are
//     read (.github, .claude but its worktrees/, .plugin and .vscode): every
//     other dot-directory, a tool's state, and another checkout are skipped,
//     and so are the site, plan/, npm/packages/ and every node_modules, dist
//     and testdata. A script no declaration names fails the
//     gate the day it is added; a declaration that names no script, gives a
//     category this command does not define, gives no reason or names an item
//     of issue 1114 wrongly is a finding, and so is an item of pendingPorts no
//     pending port names any longer.
//  7. The release VERSION names has a written summary,
//     .github/release-notes/v<VERSION>.md, which the release workflow hands
//     GoReleaser with --release-header so the GitHub release opens with it.
//     From 3.0.0 to 3.3.0 a release body was only the commit list GoReleaser
//     generates, which tells a reader nothing about what breaks or what to do
//     when upgrading. Every file of that directory is the notes of one
//     release, named vX.Y.Z.md, and carries the sections Summary, Breaking
//     changes, Upgrading and Security as level-two headings, once each, in
//     that order, each with something under it (None. where there is nothing
//     to say), and nothing above the first; a heading inside a fenced code
//     block is code. No line carries an em or en dash, a command that makes
//     GitHub skip workflows, or a credit to an assistant, since the body is
//     published under the maintainer's name and the text is the one most
//     likely to be pasted into the pull request that moves VERSION.
//
// Rules 1 and 2 do not stop at a uses: line. The action a step names is
// opened and its steps are judged as though the workflow had written them: a
// pinned remote action is read from the committed record,
// docs/development/pinned-actions.json, and a directory of the workspace from
// this repository. A composite action's own uses: lines are held to rule 1
// wherever it runs, the actions they name are opened in turn, and in a job
// holding a credential its steps are held to rule 2, its run blocks reading
// the action's directory where they name GITHUB_ACTION_PATH or
// ${{ github.action_path }}, in a POSIX shell or through PowerShell's env:
// drive. Nothing opened winget-releaser while the job holding WINGET_TOKEN
// ran it (issue 1232); opened, its pinned commit fails both rules: its uses:
// of cargo-binstall at main is rule 1's, and its `cargo binstall komac -y`,
// which installs whatever komac release is newest, is rule 2's.
//
// A run block of somebody else's action cannot be edited here, and a branch of
// it no step's inputs take is not code the job runs, so declarations.go may
// accept one rule in one step of a pinned action under input-guarded: it names
// the input and the value that take the branch the match sits in, and every
// step of a credentialed job that names the action is held to setting that
// input to a literal other than that value, or to leaving it to a default other
// than it; a value written as an expression is one only the run knows, and is
// a finding. Its one entry is sigstore/cosign-installer, whose script builds
// cosign from its main branch with go install only when cosign-release is
// main. A run block of a workflow, or of an action in this repository, is
// never declared.
//
// A container action's runs.image is held to rule 1 wherever it runs, and is
// read as the runner reads it: the docker:// scheme, in any case, names an
// image it pulls, which passes when named by its digest and is a finding when
// named by a tag or by its name alone; a value whose last path segment ends in
// Dockerfile or starts with Dockerfile., in any case, is built from the
// action's files at its pin, with no --pull and no --build-arg, and every
// image that Dockerfile has the build pull is held to its digest the same way:
// each FROM that is neither scratch nor a stage, each COPY --from and RUN
// --mount from= that names an image, and the frontend a syntax directive
// names, read as BuildKit reads the file (keywords in any case, the escape
// directive, continuation lines, heredoc bodies skipped) with the defaults
// of the global ARGs substituted, a variable no default sets being a finding;
// the Dockerfile is read from the record for a pinned action and from the
// workspace for one of this repository; and any other value, which the runner
// refuses, is a finding, as is metadata that names no image. An image that
// somebody else's action names can be accepted under write-free-job in
// declarations.go, keyed by the action and the image without its tag, so a
// bump that moves the tag keeps the declaration and one that moves the image
// elsewhere does not; every job that runs it is then held to a token, declared
// by the job or its workflow, that writes nothing but id-token, to cache-mode
// none and to reading no secret (an undeclared token is the repository's
// default, a setting the tree does not hold), what the workflow hands down
// included, since whatever a push to the tag puts there runs with the job's
// token, its secrets and the runtime token the runner hands every container
// action, which permissions: does not reach and which saves to the Actions
// cache on a push or a schedule unless cache-mode is none. Its one entry is
// ossf/scorecard-action, whose action.yaml runs
// docker://ghcr.io/ossf/scorecard-action at its release tag.
//
// The record holds, for every pinned action a step reaches, the metadata file
// at its commit, the files of its directory a credentialed job's steps read,
// and the Dockerfile a container action builds, each as its lines and in
// ASCII. It is written by -record, which runs
// the same walk fetching each file from raw.githubusercontent.com, so it holds
// exactly what the audit reads, and is read offline by every other run, so the
// gate needs no network. The default run holds the record to the pins: a pin
// it holds no entry for, a file it has no answer for, a commit with no
// action.yml or action.yaml, metadata that does not parse and a runs.using
// this audit does not read are each reported as unjudged, and an action or a
// file no step reaches any longer is reported as stale. A bump of a pin
// therefore fails the gate until -record is run and its diff reviewed, which
// is the point: the new commit is what the job will run. A uses: of a
// directory holding no action, one an action writes at run time, is a finding
// unless declarations.go accepts it with a reason.
//
// What opening an action does not reach, by construction: a JavaScript action
// runs the bundle committed at its pin, which is as pinned as its metadata and
// is not read; what a container action's Dockerfile fetches other than the
// images its build pulls, a RUN line that downloads at build time and an ADD of
// a URL or a git repository, is not read; and a reusable workflow named by a
// job's uses: is not an action and is not opened.
//
// Usage:
//
//	go run ./cmd/audit_supply_chain/ [--root <dir>] [-record]
//
// Exits non-zero and prints one line per violation. -record prints how many
// actions it recorded first, and writes nothing when a fetch fails.
//
// The auditor deliberately reads pinning twice: on the raw file text, so a
// uses: line the parser never reaches counts (one in a document after a
// document that does not parse, or inside a block scalar), and on the parsed
// YAML, so a spelling no line pattern reads counts too, a finding the text
// made on a line not being made again from the document. A commented-out
// uses: is read by neither: it runs nothing, and the edit that uncomments it
// is judged then. Job structure comes from the parsed YAML alone.
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
