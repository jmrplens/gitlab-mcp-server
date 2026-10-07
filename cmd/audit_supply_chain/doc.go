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
//     piped into a shell, no unhashed pip install, no cargo binstall (which
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
// finding. Its tables are held to the tree as well: a secret the workflows no
// longer read, and a declaration in any table of declarations.go that excuses
// nothing, are reported as stale.
//
// What rule 2 does not see, by construction, is a command that is not written
// as one: a make or a script handed to a language's process or path API as an
// argument (spawnSync("make", ...), subprocess.run(["make", ...]),
// os.path.join(ROOT, "scripts", "x.sh"), path.join(__dirname, "x.mjs")), and a
// make run through a shell function or a variable holding its name
// (run_check make target, "$MAKE" target). A make mentioned in prose is not a
// command either, which is why a mention it does not read is not a finding:
// the files a credentialed job reaches mention make in their messages. A test
// pins each of these as not followed. Each pattern also reads one line, and an
// install one command of it: a download kept in a variable or a file and run
// on a later line is not matched, while an install whose --locked or
// --require-hashes sits on a continuation line is refused, since the line is
// all the pattern reads.
//  3. Dependabot states its cooldown instead of inheriting a platform default
//     that GitHub can change under us.
//  4. SECURITY.md names the major version the repository actually ships.
//  5. Both installers verify the release's Sigstore bundle, not only a
//     checksums.txt fetched from the same mutable release.
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
// The record holds, for every pinned action a step reaches, the metadata file
// at its commit, and the files of its directory a credentialed job's steps
// read, each as its lines and in ASCII. It is written by -record, which runs
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
// is not read; a container action runs an image named by a tag, or a
// Dockerfile whose base is, which a registry can move and this audit does not
// open; and a reusable workflow named by a job's uses: is not an action and is
// not opened.
//
// Usage:
//
//	go run ./cmd/audit_supply_chain/ [--root <dir>] [-record]
//
// Exits non-zero and prints one line per violation. -record prints how many
// actions it recorded first, and writes nothing when a fetch fails.
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
