// Tests for the judgement of the actions a workflow's steps name: what a
// pinned composite action runs is read from the committed record and held to
// rule 1 and, in a credentialed job, rule 2, as though it were written in the
// workflow itself.
package main

import (
	"strings"
	"testing"
)

// The pinned references the fixtures name, beside the keys record_test.go
// declares.
const (
	checkoutKey = "actions/checkout@" + testSHA
	dockerKey   = "example/container@" + testSHA
)

// TestParseActionReference_Uses_SplitsWhatTheRunnerFetches verifies how a
// uses: value is read as the repository, the directory inside it and the
// commit the runner fetches, and which values are not a pinned remote action
// at all.
func TestParseActionReference_Uses_SplitsWhatTheRunnerFetches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		uses string
		want actionReference
		ok   bool
	}{
		{uses: attestKey, want: actionReference{repository: "actions/attest", sha: testSHA}, ok: true},
		{uses: "github/codeql-action/init@" + testSHA, want: actionReference{repository: "github/codeql-action", directory: "init", sha: testSHA}, ok: true},
		{uses: "example/tools/a/b@" + testSHA, want: actionReference{repository: "example/tools", directory: "a/b", sha: testSHA}, ok: true},
		{uses: "lonely@" + testSHA},
		{uses: "/repo@" + testSHA},
		{uses: "owner/@" + testSHA},
		{uses: "actions/checkout@v7"},
		{uses: "./local/action"},
		{uses: "docker://alpine@sha256:" + strings.Repeat("a", 64)},
	}

	for _, testCase := range cases {
		t.Run(testCase.uses, func(t *testing.T) {
			t.Parallel()

			got, ok := parseActionReference(testCase.uses)
			if got != testCase.want || ok != testCase.ok {
				t.Errorf("parseActionReference(%q) = %#v, %v, want %#v, %v", testCase.uses, got, ok, testCase.want, testCase.ok)
			}
			if ok && (got.key() != testCase.uses || got.name()+"@"+testSHA != testCase.uses) {
				t.Errorf("key() = %q and name() = %q, want them to spell %q", got.key(), got.name(), testCase.uses)
			}
		})
	}
}

// TestAuditWorkflows_PinnedAction_IsUnjudgedWithoutARecord verifies that a
// pinned action the record does not hold is a finding, named once however many
// steps reach it, and that an action whose commit has no metadata file, one
// whose metadata does not parse, one that runs on something this audit does not
// read, and a pin that names no repository are findings too.
//
// This is the issue the record exists for: winget-releaser was pinned to a
// commit, and the commit installed cargo-binstall from a branch head and the
// newest komac. A pin on our side read as closing that, and an action nobody
// opened is the same blind spot under another name.
func TestAuditWorkflows_PinnedAction_IsUnjudgedWithoutARecord(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "ci.yml", `jobs:
  lint:
    steps:
      - uses: `+checkoutKey+`
      - uses: `+checkoutKey+`
      - uses: example/bare@`+testSHA+`
      - uses: example/broken@`+testSHA+`
      - uses: example/strange@`+testSHA+`
      - uses: lonely@`+testSHA+`
      - uses: lonely@`+testSHA+`
      - not a step
  test:
    steps:
      - uses: `+checkoutKey+`
`)
	record := recordOf(map[string]string{
		"example/broken@" + testSHA:  "runs: [unclosed\n",
		"example/strange@" + testSHA: "runs:\n  using: java\n",
	})
	record.Actions["example/bare@"+testSHA] = recordedAction{}
	writeRecord(t, root, record)

	problems, err := auditTree(root, tables{})
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	const step = ".github/workflows/ci.yml: job lint: step "
	want := []string{
		step + "0: uses: " + checkoutKey + " has no entry in " + actionRecordPath + ", so what it runs cannot be judged; run " + recordCommand,
		step + "2: uses: example/bare@" + testSHA + ": that commit has no action.yml or action.yaml, so what it runs cannot be judged",
		"example/broken@" + testSHA + "/action.yml does not parse, so what it runs cannot be judged: yaml: line 1: did not find expected ',' or ']'",
		"example/strange@" + testSHA + `/action.yml: runs.using "java" is not one this audit reads, so what it runs cannot be judged`,
		step + "5: uses: lonely@" + testSHA + " names no repository, so what it runs cannot be judged",
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_NodeAndDockerActions_NeedOnlyTheirRecord verifies that an
// action whose code is the bundle committed at its pin, or a container, is
// recorded and read no further, so it passes with its metadata in the record
// and nothing else.
func TestAuditWorkflows_NodeAndDockerActions_NeedOnlyTheirRecord(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "release.yml", `jobs:
  release:
    permissions:
      contents: write
    steps:
      - uses: `+attestKey+`
      - uses: `+dockerKey+`
      - uses: docker://alpine:3
      - uses: docker://example/image@`+testSHA+`
`)
	writeRecord(t, root, recordOf(map[string]string{
		attestKey: "runs:\n  using: 'node24'\n  main: dist/index.js\n",
		dockerKey: "runs:\n  using: Docker\n  image: Dockerfile\n",
	}))

	problems, err := auditTree(root, tables{})
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("auditWorkflows() = %#v, want no findings", problems)
	}
}

// TestAuditWorkflows_CompositeAction_IsHeldToRulesOneAndTwo verifies that the
// steps of a pinned composite action are judged as the workflow's own would
// be: its uses: lines by rule 1 wherever it runs, its nested actions opened in
// turn, and its run blocks, the files they reach and its actions' inputs by
// rule 2 only in a job holding a credential.
//
// The fixture is winget-releaser's shape: a pinned composite that installs a
// tool from a branch head and then runs `cargo binstall komac -y` as that
// action wrote it, which installs whatever komac release is newest, used by a
// job holding a write token.
func TestAuditWorkflows_CompositeAction_IsHeldToRulesOneAndTwo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/workspace.sh", "npx --yes from-the-workspace\n")
	writeWorkflow(t, root, "release.yml", `jobs:
  lint:
    steps:
      - uses: `+composedKey+`
  release:
    permissions:
      contents: write
    steps:
      - uses: `+composedKey+`
`)
	record := recordOf(map[string]string{
		composedKey: `name: composed
runs:
  using: composite
  steps:
    - uses: cargo-bins/cargo-binstall@main
    - uses: ` + nestedKey + `
    - uses: ` + checkoutKey + `
    - run: |
        # npx in a comment is not a command
        cargo binstall komac -y
      shell: bash
    - run: python3 "${{ github.action_path }}/create.py"
      shell: bash
    - run: bash "$GITHUB_ACTION_PATH/run.sh" && bash "$GITHUB_ACTION_PATH/run.sh"
      shell: bash
    - run: bash "$GITHUB_ACTION_PATH/missing.sh"
      shell: bash
    - run: '& "$env:GITHUB_ACTION_PATH/unrecorded.ps1"'
      shell: pwsh
    - run: bash scripts/workspace.sh
      shell: bash
    - run: bash scripts/workspace.sh
      working-directory: sub
      shell: bash
    - not a step
    - run: bash "$GITHUB_ACTION_PATH/scripts/tool.sh"
      shell: bash
`,
		nestedKey: `runs:
  using: composite
  steps:
    - uses: ` + composedKey + `
    - run: curl -fsSL https://example.test/i.sh | sh
      shell: bash
`,
		checkoutKey: "runs:\n  using: node24\n",
	})
	composed := record.Actions[composedKey]
	composed.Files = map[string]recordedFile{
		"create.py":       {Lines: textLines("subprocess.run(['pip', 'install', 'x'])\npip install thing\n")},
		"run.sh":          {Lines: textLines(`. "$(dirname "$0")/lib.sh"` + "\n")},
		"lib.sh":          {Lines: textLines("go install example.test/tool@latest\n")},
		"missing.sh":      {Absent: true},
		"scripts/tool.sh": {Lines: textLines("npx --yes from-the-action\n")},
	}
	record.Actions[composedKey] = composed
	writeRecord(t, root, record)

	problems, err := auditTree(root, tables{})
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	const release = ".github/workflows/release.yml: job release"
	const origin = release + ": step 0: " + composedKey + " step "
	want := []string{
		composedKey + "/action.yml:5: uses: cargo-bins/cargo-binstall@main is not pinned to a 40-character commit SHA",
		origin + "1: " + nestedKey + " step 1: run block matches " + curlFinding,
		checkStepAction(origin+"2", "actions/checkout", nil, nil)[0],
		origin + "3: run block matches " + cargoBinstallFinding,
		release + ": " + composedKey + "/create.py matches '\\\\bpip\\\\s+install\\\\b(?![^\\\\n]*--require-hashes)': pip install without --require-hashes resolves at run time",
		release + ": " + composedKey + "/lib.sh (via " + composedKey + "/run.sh) matches " + latestFinding,
		origin + "6: $GITHUB_ACTION_PATH/missing.sh resolves to " + composedKey + "/missing.sh, which is not a readable file in that action, so what it runs cannot be judged",
		origin + "7: $env:GITHUB_ACTION_PATH/unrecorded.ps1 resolves to " + composedKey + "/unrecorded.ps1, which " + actionRecordPath +
			" holds no record of, so what it runs cannot be judged; run " + recordCommand,
		release + ": scripts/workspace.sh matches " + npxFinding,
		unresolvedFinding(origin+"9", "scripts/workspace.sh", "sub/scripts/workspace.sh"),
		release + ": " + composedKey + "/scripts/tool.sh matches " + npxFinding,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_ScriptTheJobAndItsActionBothRun_IsJudgedOnce verifies
// that the job's own reading and the walk through its actions share what they
// have read: a script the job's run block runs and a composite step runs again
// is one finding, not one per reading.
func TestAuditWorkflows_ScriptTheJobAndItsActionBothRun_IsJudgedOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/shared.sh", "npx --yes thing\n")
	writeWorkflow(t, root, "release.yml", `jobs:
  release:
    permissions:
      contents: write
    steps:
      - run: bash scripts/shared.sh
      - uses: `+composedKey+`
`)
	writeRecord(t, root, recordOf(map[string]string{
		composedKey: "runs:\n  using: composite\n  steps:\n    - run: bash scripts/shared.sh\n      shell: bash\n",
	}))

	problems, err := auditTree(root, tables{})
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	want := []string{".github/workflows/release.yml: job release: scripts/shared.sh matches " + npxFinding}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_ActionScript_ReadsItsActionPath verifies that the
// directory of the action whose step started a script stays the action path
// of every file that script reaches, since the runner exports it to every
// process the step starts: a recorded script that runs a helper through
// $GITHUB_ACTION_PATH has that helper read from the record, not reported as
// rooted in a value only the run knows.
func TestAuditWorkflows_ActionScript_ReadsItsActionPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "release.yml", `jobs:
  release:
    permissions:
      contents: write
    steps:
      - uses: `+composedKey+`
`)
	record := recordOf(map[string]string{
		composedKey: "runs:\n  using: composite\n  steps:\n    - run: bash \"$GITHUB_ACTION_PATH/run.sh\"\n      shell: bash\n",
	})
	composed := record.Actions[composedKey]
	composed.Files = map[string]recordedFile{
		"run.sh":    {Lines: textLines(`bash "$GITHUB_ACTION_PATH/helper.sh"` + "\n")},
		"helper.sh": {Lines: textLines("npx --yes from-the-helper\n")},
	}
	record.Actions[composedKey] = composed
	writeRecord(t, root, record)

	problems, err := auditTree(root, tables{})
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	want := []string{
		".github/workflows/release.yml: job release: " + composedKey + "/helper.sh (via " + composedKey + "/run.sh) matches " + npxFinding,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// The pinned actions the guard fixtures name: one whose input defaults to the
// value that runs its guarded branch, one guarded on a boolean input, and one
// whose metadata does not parse.
const (
	defaultedKey = "example/defaulted@" + testSHA
	flaggedKey   = "example/flagged@" + testSHA
	brokenKey    = "example/broken@" + testSHA
)

// TestAuditWorkflows_GuardedMatch_IsHeldToEveryStepThatNamesTheAction
// verifies a declared guarded match: a match in a pinned composite action's
// run block is excused when the branch it sits in runs only on one value of
// one input, and every step of a credentialed job that names the action is
// held to setting that input to a literal other than that value, or to leaving
// it to a default other than it.
//
// This is sigstore/cosign-installer's shape: its script runs
// `go install .../cosign@main` only when cosign-release is main, and every
// step here names a release. A step that sets main, sets an expression only
// the run knows, or leaves the input to a default of main is a finding, in a
// workflow's step and in a composite's alike, however many steps of one job
// name the action; a step of a job holding no credential is not judged, and
// the action's other matches are judged as ever. A YAML boolean is compared as
// the text the runner hands the action, so `nightly: true` sets "true".
func TestAuditWorkflows_GuardedMatch_IsHeldToEveryStepThatNamesTheAction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "release.yml", `jobs:
  lint:
    steps:
      - uses: `+composedKey+`
        with:
          release: main
  release:
    permissions:
      contents: write
    steps:
      - uses: `+composedKey+`
        with:
          release: v1.2.3
      - uses: `+composedKey+`
        with:
          release: main
      - uses: `+composedKey+`
        with:
          release: ${{ inputs.release }}
      - uses: `+composedKey+`
      - uses: `+nestedKey+`
  defaults:
    permissions:
      contents: write
    steps:
      - uses: `+defaultedKey+`
      - uses: `+flaggedKey+`
        with:
          nightly: false
      - uses: `+flaggedKey+`
        with:
          nightly: true
`)
	writeRecord(t, root, recordOf(map[string]string{
		composedKey: `inputs:
  release:
    default: v1.0.0
runs:
  using: composite
  steps:
    - run: |
        if [ "$RELEASE" = main ]; then
          go install example.test/tool@main
        fi
      env:
        RELEASE: ${{ inputs.release }}
      shell: bash
    - run: go install example.test/other@main
      shell: bash
`,
		nestedKey: `runs:
  using: composite
  steps:
    - uses: ` + composedKey + `
      with:
        release: ${{ inputs.version }}
`,
		defaultedKey: `inputs:
  release:
    default: main
runs:
  using: composite
  steps:
    - run: go install example.test/tool@main
      shell: bash
`,
		flaggedKey: `runs:
  using: composite
  steps:
    - run: '[ "$NIGHTLY" != true ] || go install example.test/tool@main'
      shell: bash
`,
	}))
	declared := tables{guarded: map[string]guardedMatch{
		"example/composite step 0 go-install": {category: categoryInputGuarded, input: "release", value: "main", reason: "runs only on main"},
		"example/defaulted step 0 go-install": {category: categoryInputGuarded, input: "release", value: "main", reason: "runs only on main"},
		"example/flagged step 0 go-install":   {category: categoryInputGuarded, input: "nightly", value: "true", reason: "runs only nightly"},
	}}

	problems, err := auditTree(root, declared)
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	const release = ".github/workflows/release.yml: job release: step "
	const accepted = ", the value that runs the go-install match " + declarationsFile + " accepts in its step 0"
	const unknown = ", which only the run knows, so the go-install match " + declarationsFile + " accepts in its step 0 cannot be ruled out"
	want := []string{
		release + "0: " + composedKey + " step 1: run block matches " + goInstallFinding,
		release + "1: uses: " + composedKey + ` sets release to "main"` + accepted,
		release + "2: uses: " + composedKey + " sets release to ${{ inputs.release }}" + unknown,
		release + "4: " + nestedKey + " step 0: uses: " + composedKey + " sets release to ${{ inputs.version }}" + unknown,
		".github/workflows/release.yml: job defaults: step 0: uses: " + defaultedKey + ` leaves release to its default "main"` + accepted,
		".github/workflows/release.yml: job defaults: step 2: uses: " + flaggedKey + ` sets nightly to "true"` + accepted,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_GuardedMatch_OnUnreadableMetadata_InventsNothing verifies
// that a step leaving a guarded input to its default, of an action whose
// metadata does not parse, is not held to a default nobody can read: the
// metadata is reported where the action is read, and the declaration, which
// then excuses nothing, as stale.
func TestAuditWorkflows_GuardedMatch_OnUnreadableMetadata_InventsNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "release.yml", "jobs:\n  release:\n    permissions:\n      contents: write\n    steps:\n      - uses: "+brokenKey+"\n")
	writeRecord(t, root, recordOf(map[string]string{brokenKey: "runs: [unclosed\n"}))
	declared := tables{guarded: map[string]guardedMatch{
		"example/broken step 0 go-install": {category: categoryInputGuarded, input: "release", value: "main", reason: "runs only on main"},
	}}

	problems, err := auditTree(root, declared)
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	want := []string{
		brokenKey + "/action.yml does not parse, so what it runs cannot be judged: yaml: line 1: did not find expected ',' or ']'",
		declarationsFile + `: "example/broken step 0 go-install" excuses nothing: no credentialed job reaches that step of that action matching that rule`,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_GuardedDeclarations_AreHeldToTheTree verifies the guarded
// match table's rules: a key that is not an action without its commit, the
// word step, a step's index and a rule name; a category this command does not
// define for a guarded match; no input or value; no reason; and an entry that
// excused nothing, which is how a declaration outlives a bump of the action
// that moved or dropped its match. An action of this repository is never
// excused, even by a key that names it, since its run block is fixed where it
// is written.
func TestAuditWorkflows_GuardedDeclarations_AreHeldToTheTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, ".github/actions/own/action.yml", "runs:\n  using: composite\n  steps:\n    - run: go install example.test/tool@main\n      shell: bash\n")
	writeWorkflow(t, root, "release.yml", "jobs:\n  release:\n    permissions:\n      contents: write\n    steps:\n      - uses: "+composedKey+"\n      - uses: ./.github/actions/own\n")
	writeRecord(t, root, recordOf(map[string]string{
		composedKey: "runs:\n  using: composite\n  steps:\n    - run: go install example.test/tool@main\n      shell: bash\n",
	}))
	valid := guardedMatch{category: categoryInputGuarded, input: "release", value: "main", reason: "runs only on main"}
	declared := tables{guarded: map[string]guardedMatch{
		"example/composite step 0 go-install":          valid,
		"example/composite step 1 go-install":          valid,
		"example/composite step 0":                     valid,
		"example/composite stage 0 go-install":         valid,
		"example/composite step x go-install":          valid,
		"example/composite step -1 go-install":         valid,
		"example/composite step 00 go-install":         valid,
		"example/composite step 0 npm":                 valid,
		"example/composite@" + testSHA + " step 0 npx": valid,
		"./.github/actions/own step 0 go-install":      valid,
		"lonely step 0 npx":                            valid,
		"example/other step 0 latest":                  {category: "trust-me", reason: " "},
		"example/composite step 0 go-install and more": valid,
		"example/composite  step 0 go-install":         valid,
		"example/composite step 0 go-install ":         valid,
		"example/composite step 0 curl-pipe-shell":     {category: categoryInputGuarded, input: "release", reason: "no value"},
		"example/composite step 0 invoke-expression":   {category: categoryInputGuarded, value: "main", reason: "no input"},
		"example/composite step 0 cargo-binstall":      {category: categoryInputGuarded, input: "release", value: "main"},
		"example/composite/sub step 0 cargo-install":   valid,
	}}

	problems, err := auditTree(root, declared)
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	const label = declarationsFile + ": "
	const malformed = ` is not an action without its commit, "step", a step's index and a rule name (` + ruleNameList + `) separated by single spaces`
	const stale = " excuses nothing: no credentialed job reaches that step of that action matching that rule"
	const unguarded = " does not name the input and the value that run the branch it accepts"
	want := []string{
		".github/workflows/release.yml: job release: step 1: ./.github/actions/own step 0: run block matches " + goInstallFinding,
		label + `"./.github/actions/own step 0 go-install"` + malformed,
		label + `"example/composite  step 0 go-install"` + malformed,
		label + `"example/composite stage 0 go-install"` + malformed,
		label + `"example/composite step -1 go-install"` + malformed,
		label + `"example/composite step 0"` + malformed,
		label + `"example/composite step 0 cargo-binstall" gives no reason`,
		label + `"example/composite step 0 cargo-binstall"` + stale,
		label + `"example/composite step 0 curl-pipe-shell"` + unguarded,
		label + `"example/composite step 0 curl-pipe-shell"` + stale,
		label + `"example/composite step 0 go-install "` + malformed,
		label + `"example/composite step 0 go-install and more"` + malformed,
		label + `"example/composite step 0 invoke-expression"` + unguarded,
		label + `"example/composite step 0 invoke-expression"` + stale,
		label + `"example/composite step 0 npm"` + malformed,
		label + `"example/composite step 00 go-install"` + malformed,
		label + `"example/composite step 1 go-install"` + stale,
		label + `"example/composite step x go-install"` + malformed,
		label + `"example/composite/sub step 0 cargo-install"` + stale,
		label + `"example/composite@` + testSHA + ` step 0 npx"` + malformed,
		label + `"example/other step 0 latest": the category "trust-me" is not one this command defines for a guarded match`,
		label + `"example/other step 0 latest"` + unguarded,
		label + `"example/other step 0 latest" gives no reason`,
		label + `"example/other step 0 latest"` + stale,
		label + `"lonely step 0 npx"` + malformed,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_LocalAction_IsReadFromTheRepository verifies a uses: that
// names a directory of the workspace: an action this repository holds is
// judged like a pinned one, with its directory as the action's own, and one it
// does not hold (an action written at run time, as pypa's publisher writes the
// container action it then runs) is a finding unless the repository declares
// why it accepts it, a declaration held to the tree like every other.
func TestAuditWorkflows_LocalAction_IsReadFromTheRepository(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, ".github/actions/setup/action.yaml", `runs:
  using: composite
  steps:
    - uses: actions/setup-node@v6
    - run: bash "$GITHUB_ACTION_PATH/install.sh"
      shell: bash
`)
	writeFile(t, root, ".github/actions/setup/install.sh", "npx --yes thing\n")
	writeWorkflow(t, root, "release.yml", `jobs:
  release:
    permissions:
      id-token: write
    steps:
      - uses: ./.github/actions/setup
      - uses: ./.github/actions/setup
      - uses: ./.github/actions/absent
      - uses: ./../outside
      - uses: `+composedKey+`
`)
	writeRecord(t, root, recordOf(map[string]string{
		composedKey: "runs:\n  using: composite\n  steps:\n    - uses: ./generated/action\n    - uses: ./generated/stale\n",
	}))
	declared := tables{unjudged: map[string]declaration{
		"example/composite ./generated/action": {category: categoryGeneratedAtRunTime, reason: "written by a recorded script"},
		"example/composite ./generated/never":  {category: categoryGeneratedAtRunTime, reason: "stale"},
		"example/composite":                    {category: categoryGeneratedAtRunTime, reason: "no reference"},
		"example/composite generated/action":   {category: categoryGeneratedAtRunTime, reason: "not local"},
		" ./generated/action":                  {category: categoryGeneratedAtRunTime, reason: "no writer"},
		"example/other ./x":                    {category: "trust-me", reason: " "},
	}}

	problems, err := auditTree(root, declared)
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	const step = ".github/workflows/release.yml: job release: step "
	const label = "cmd/audit_supply_chain/declarations.go: "
	want := []string{
		".github/actions/setup/action.yaml:4: uses: actions/setup-node@v6 is not pinned to a 40-character commit SHA",
		".github/workflows/release.yml: job release: .github/actions/setup/install.sh matches " + npxFinding,
		step + "2: uses: ./.github/actions/absent names no action.yml or action.yaml in this repository, so what it runs cannot be judged",
		step + "3: uses: ./../outside names no action.yml or action.yaml in this repository, so what it runs cannot be judged",
		step + "4: " + composedKey + " step 1: uses: ./generated/stale names no action.yml or action.yaml in this repository, so what it runs cannot be judged",
		label + `" ./generated/action" is not an action and a ./ reference separated by one space`,
		label + `"example/composite" is not an action and a ./ reference separated by one space`,
		label + `"example/composite ./generated/never" excuses nothing: no step reaches that reference from that action`,
		label + `"example/composite generated/action" is not an action and a ./ reference separated by one space`,
		label + `"example/other ./x": the category "trust-me" is not one this command defines for an unjudged action`,
		label + `"example/other ./x" gives no reason`,
		label + `"example/other ./x" excuses nothing: no step reaches that reference from that action`,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_ActionRecord_IsHeldToThePins verifies that the record is
// judged against the workflows as well as read: an action no step reaches and
// a file no step reads are reported, so a record cannot keep vouching for a
// pin the workflows moved away from.
func TestAuditWorkflows_ActionRecord_IsHeldToThePins(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "ci.yml", "jobs:\n  lint:\n    steps:\n      - uses: "+composedKey+"\n")
	record := recordOf(map[string]string{
		composedKey: "runs:\n  using: composite\n  steps:\n    - run: bash \"$GITHUB_ACTION_PATH/run.sh\"\n      shell: bash\n",
		attestKey:   "runs:\n  using: node24\n",
	})
	composed := record.Actions[composedKey]
	composed.Files = map[string]recordedFile{"run.sh": {Lines: textLines("echo run\n")}}
	record.Actions[composedKey] = composed
	writeRecord(t, root, record)

	problems, err := auditTree(root, tables{})
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	want := []string{
		actionRecordPath + ": " + attestKey + " is recorded and no workflow reaches it; run " + recordCommand,
		actionRecordPath + ": " + composedKey + "/run.sh is recorded and nothing reads it; run " + recordCommand,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestAuditWorkflows_UnreadableRecord_IsAnError verifies that a record the
// audit cannot read stops it, since auditing every pin as unrecorded would
// bury the one problem under a finding per action.
func TestAuditWorkflows_UnreadableRecord_IsAnError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "ci.yml", "jobs: {}\n")
	writeFile(t, root, actionRecordPath, "not json\n")
	if _, err := auditTree(root, tables{}); err == nil {
		t.Error("auditTree() = nil error, want the record's parse failure")
	}
	if _, err := audit(root, tables{}); err == nil {
		t.Error("audit() = nil error, want the record's parse failure")
	}
}

// auditTree audits a fixture tree's workflows against the record it commits,
// the way audit does.
func auditTree(root string, declared tables) ([]string, error) {
	record, err := loadActionRecord(root)
	if err != nil {
		return nil, err
	}
	return auditWorkflows(root, declared, newOfflineStore(record))
}

// TestFollowReferences_ActionPathOutsideAnAction_IsUnresolvable verifies that
// the action directory named in a workflow's own run block, where no action is
// running and the variable is empty, is reported as a root the audit cannot
// resolve in every spelling, and is never read as the repository root the
// other variables in front of a scripts/ path stand for.
func TestFollowReferences_ActionPathOutsideAnAction_IsUnresolvable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/x.sh", "npx --yes thing\n")
	run := `bash "$GITHUB_ACTION_PATH/x.sh"
bash "${{ github.action_path }}/y.sh"
bash "$GITHUB_ACTION_PATH/scripts/x.sh"
bash "${{ github.action_path }}/scripts/x.sh"
& "$env:GITHUB_ACTION_PATH/scripts/x.ps1"
& "${env:GITHUB_ACTION_PATH}/z.ps1"`
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": run}},
	}
	const origin = "wf.yml: job j: step 0"
	want := []string{
		rootFinding(origin, "$GITHUB_ACTION_PATH/scripts/x.sh", "$GITHUB_ACTION_PATH"),
		rootFinding(origin, "${{ github.action_path }}/scripts/x.sh", "${{ github.action_path }}"),
		rootFinding(origin, "$env:GITHUB_ACTION_PATH/scripts/x.ps1", "$env:GITHUB_ACTION_PATH"),
		rootFinding(origin, "$GITHUB_ACTION_PATH/x.sh", "$GITHUB_ACTION_PATH"),
		rootFinding(origin, "${{ github.action_path }}/y.sh", "${{ github.action_path }}"),
		rootFinding(origin, "${env:GITHUB_ACTION_PATH}/z.ps1", "${env:GITHUB_ACTION_PATH}"),
	}
	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "j", job, nil)
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestFollowReferences_PowerShellEnvDrive_IsReadLikeTheVariable verifies that
// PowerShell's spelling of an environment variable roots a script the way the
// POSIX spelling does: the workspace is the repository root, through a
// scripts/ directory or not.
func TestFollowReferences_PowerShellEnvDrive_IsReadLikeTheVariable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "tools/stamp.ps1", "npx --yes thing\n")
	writeFile(t, root, "tools/scripts/build.ps1", "go install example.test/tool@latest\n")
	run := `& "$env:GITHUB_WORKSPACE/tools/stamp.ps1"
& "${Env:GITHUB_WORKSPACE}/tools/scripts/build.ps1"`
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": run}},
	}
	want := []string{
		"wf.yml: job j: tools/scripts/build.ps1 matches " + latestFinding,
		"wf.yml: job j: tools/stamp.ps1 matches " + npxFinding,
	}
	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "j", job, nil)
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestCheckJobActions_NoStore_JudgesNothing verifies that an audit built
// without a record, as the unit tests of the job rules build it, leaves the
// actions alone, so those tests keep describing only the rule they test.
func TestCheckJobActions_NoStore_JudgesNothing(t *testing.T) {
	t.Parallel()

	job := map[string]any{"steps": []any{map[string]any{"uses": checkoutKey}}}
	if problems := newAudit(".", tables{}).checkJobActions("wf.yml", "j", job, nil, true, map[string]bool{}); len(problems) != 0 {
		t.Errorf("checkJobActions() = %#v, want nothing without a store", problems)
	}
}
