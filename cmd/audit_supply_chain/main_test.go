// Tests for the supply-chain configuration auditor.
//
// Each test feeds the checker the shape the repository had when the
// corresponding security finding was written, asserts it is reported, then
// feeds it the shape after the fix and asserts silence.
package main

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// testSHA is a real 40-character commit SHA (actions/checkout v7), used
// wherever a fixture needs a reference that is pinned the way the rule wants.
const testSHA = "3d3c42e5aac5ba805825da76410c181273ba90b1"

// TestCheckPinnedUses_MutableReference_IsReported verifies that only
// 40-character commit SHAs count as a pinned action.
//
// A mutable major tag is resolved by the runner at job start, so it is the
// reference an upstream tag hijack (CVE-2025-30066) travels through.
func TestCheckPinnedUses_MutableReference_IsReported(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		reference string
		wantOK    bool
	}{
		{name: "mutable major tag", reference: "actions/checkout@v7", wantOK: false},
		{name: "exact version tag", reference: "sigstore/cosign-installer@v4.1.2", wantOK: false},
		{name: "branch", reference: "some/action@main", wantOK: false},
		{name: "short sha", reference: "some/action@3d3c42e", wantOK: false},
		{name: "commit sha", reference: "actions/checkout@" + testSHA, wantOK: true},
		{name: "commit sha with version comment", reference: "actions/checkout@" + testSHA + " # v7", wantOK: true},
		{name: "subpath action pinned", reference: "github/codeql-action/init@" + testSHA + " # v4", wantOK: true},
		{name: "local action", reference: "./.github/actions/thing", wantOK: true},
		{name: "docker action", reference: "docker://alpine:3.22", wantOK: true},
		{name: "quoted mutable tag", reference: `"actions/checkout@v7"`, wantOK: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			text := "jobs:\n  a:\n    steps:\n      - uses: " + testCase.reference + "\n"
			problems := checkPinnedUses("wf.yml", text)
			if gotOK := len(problems) == 0; gotOK != testCase.wantOK {
				t.Errorf("checkPinnedUses(%q) clean = %t, want %t (%v)", testCase.reference, gotOK, testCase.wantOK, problems)
			}
		})
	}
}

// TestCheckPinnedUses_UnpinnedReference_NamesLineAndReference verifies the
// exact wording and line number of the pinning finding, which is the message a
// maintainer acts on and the one the Python auditor printed before this port.
func TestCheckPinnedUses_UnpinnedReference_NamesLineAndReference(t *testing.T) {
	t.Parallel()

	text := "jobs:\n  a:\n    steps:\n      - uses: actions/checkout@v7\n"
	problems := checkPinnedUses(".github/workflows/ci.yml", text)
	want := ".github/workflows/ci.yml:4: uses: actions/checkout@v7 is not pinned to a 40-character commit SHA"
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkPinnedUses() = %v, want exactly [%q]", problems, want)
	}
}

// TestIsCredentialed_EffectivePermissions_DecideTheJob verifies which jobs the
// hardening rules apply to.
//
// A job is credentialed when its effective permissions (its own, else the
// workflow's) grant contents: write, id-token: write or packages: write, the
// three that make it worth attacking, since one rewrites the repository,
// another mints the OIDC identity npm, PyPI, cosign and the MCP Registry all
// trust, and the third pushes the image server.json pins.
func TestIsCredentialed_EffectivePermissions_DecideTheJob(t *testing.T) {
	t.Parallel()

	cases := []struct {
		job  map[string]any
		doc  map[string]any
		name string
		want bool
	}{
		{
			name: "job-level id-token write",
			job:  map[string]any{"permissions": map[string]any{"id-token": "write"}},
			doc:  map[string]any{},
			want: true,
		},
		{
			name: "job-level contents write",
			job:  map[string]any{"permissions": map[string]any{"contents": "write"}},
			doc:  map[string]any{},
			want: true,
		},
		{
			name: "job-level read only",
			job:  map[string]any{"permissions": map[string]any{"contents": "read"}},
			doc:  map[string]any{},
			want: false,
		},
		{
			name: "job-level none",
			job:  map[string]any{"permissions": map[string]any{}},
			doc:  map[string]any{"permissions": map[string]any{"id-token": "write"}},
			want: false,
		},
		{
			name: "inherits workflow write",
			job:  map[string]any{},
			doc:  map[string]any{"permissions": map[string]any{"id-token": "write"}},
			want: true,
		},
		{
			name: "inherits workflow read",
			job:  map[string]any{},
			doc:  map[string]any{"permissions": map[string]any{"contents": "read"}},
			want: false,
		},
		{name: "no permissions anywhere", job: map[string]any{}, doc: map[string]any{}, want: false},
		{
			name: "packages write only",
			job:  map[string]any{"permissions": map[string]any{"packages": "write"}},
			doc:  map[string]any{},
			want: true,
		},
		{
			name: "packages read only",
			job:  map[string]any{"permissions": map[string]any{"packages": "read"}},
			doc:  map[string]any{},
			want: false,
		},
		{
			name: "a write no rule counts",
			job:  map[string]any{"permissions": map[string]any{"pull-requests": "write", "issues": "write"}},
			doc:  map[string]any{},
			want: false,
		},
		{name: "write-all shorthand", job: map[string]any{"permissions": "write-all"}, doc: map[string]any{}, want: true},
		{name: "read-all shorthand", job: map[string]any{"permissions": "read-all"}, doc: map[string]any{}, want: false},
		{
			name: "unparseable permissions value",
			job:  map[string]any{"permissions": []any{"contents"}},
			doc:  map[string]any{},
			want: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := isCredentialed(testCase.doc, testCase.job); got != testCase.want {
				t.Errorf("isCredentialed() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestCheckWorkflowJobs_CheckoutCredential_MustNotPersist verifies that a
// credentialed job's checkout must not persist the token.
//
// actions/checkout defaults persist-credentials to true, leaving the
// write-capable GITHUB_TOKEN in .git/config for every later step — including
// the ones that run third-party code.
func TestCheckWorkflowJobs_CheckoutCredential_MustNotPersist(t *testing.T) {
	t.Parallel()

	cases := []struct {
		with        map[string]any
		name        string
		wantProblem bool
	}{
		{name: "default (true)", with: nil, wantProblem: true},
		{name: "explicit true", with: map[string]any{"persist-credentials": true}, wantProblem: true},
		{name: "explicit false", with: map[string]any{"persist-credentials": false}, wantProblem: false},
		{
			name:        "false beside other inputs",
			with:        map[string]any{"fetch-depth": 0, "persist-credentials": false},
			wantProblem: false,
		},
		{name: "string false", with: map[string]any{"persist-credentials": "false"}, wantProblem: false},
		{name: "string true", with: map[string]any{"persist-credentials": "true"}, wantProblem: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			step := map[string]any{"uses": "actions/checkout@" + testSHA}
			if testCase.with != nil {
				step["with"] = testCase.with
			}
			doc := map[string]any{"jobs": map[string]any{
				"release": map[string]any{
					"permissions": map[string]any{"contents": "write"},
					"steps":       []any{step},
				},
			}}
			problems := newAudit(".", tables{}).checkWorkflowJobs("wf.yml", doc, nil)
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkWorkflowJobs() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestCheckWorkflowJobs_ReadOnlyJob_IsNotSubjectToTheRule verifies that the
// hardening rules leave uncredentialed jobs alone: a lint job that persists a
// read-only token has nothing worth stealing in .git/config.
func TestCheckWorkflowJobs_ReadOnlyJob_IsNotSubjectToTheRule(t *testing.T) {
	t.Parallel()

	doc := map[string]any{"jobs": map[string]any{
		"lint": map[string]any{
			"permissions": map[string]any{"contents": "read"},
			"steps":       []any{map[string]any{"uses": "actions/checkout@" + testSHA}},
		},
	}}
	if problems := newAudit(".", tables{}).checkWorkflowJobs("wf.yml", doc, nil); len(problems) != 0 {
		t.Errorf("checkWorkflowJobs() = %v, want no findings for a read-only job", problems)
	}
}

// TestCheckWorkflowJobs_MultipleJobs_ReportInDocumentOrder verifies that
// findings follow the order the workflow wrote its jobs in.
//
// Go map iteration is randomized, so without the parsed key order two runs
// over the same file would print the same findings in a different order and a
// reviewer could not diff one audit against another.
func TestCheckWorkflowJobs_MultipleJobs_ReportInDocumentOrder(t *testing.T) {
	t.Parallel()

	unpinnedJob := func() map[string]any {
		return map[string]any{
			"permissions": map[string]any{"contents": "write"},
			"steps":       []any{map[string]any{"uses": "actions/checkout@" + testSHA}},
		}
	}
	doc := map[string]any{"jobs": map[string]any{
		"zulu":  unpinnedJob(),
		"alpha": unpinnedJob(),
	}}

	problems := newAudit(".", tables{}).checkWorkflowJobs("wf.yml", doc, []string{"zulu", "alpha"})
	if len(problems) != 2 {
		t.Fatalf("checkWorkflowJobs() = %v, want 2 findings", problems)
	}
	if !strings.Contains(problems[0], "job zulu") || !strings.Contains(problems[1], "job alpha") {
		t.Errorf("checkWorkflowJobs() = %v, want zulu before alpha (document order)", problems)
	}

	sorted := newAudit(".", tables{}).checkWorkflowJobs("wf.yml", doc, nil)
	if len(sorted) != 2 || !strings.Contains(sorted[0], "job alpha") {
		t.Errorf("checkWorkflowJobs() with no document order = %v, want alpha first (sorted fallback)", sorted)
	}
}

// TestCheckWorkflowJobs_JobIsNotAMapping_IsSkipped verifies that a jobs entry
// whose value is not a mapping is passed over while its siblings are still
// audited, so one hand-edited workflow cannot take the whole gate down.
func TestCheckWorkflowJobs_JobIsNotAMapping_IsSkipped(t *testing.T) {
	t.Parallel()

	doc := map[string]any{"jobs": map[string]any{
		"release": "bash scripts/release.sh",
		"publish": map[string]any{
			"permissions": map[string]any{"contents": "write"},
			"steps":       []any{map[string]any{"uses": "actions/checkout@" + testSHA}},
		},
	}}
	problems := newAudit(".", tables{}).checkWorkflowJobs("wf.yml", doc, []string{"release", "publish"})
	if len(problems) != 1 || !strings.Contains(problems[0], "job publish") {
		t.Errorf("checkWorkflowJobs() = %v, want only the job that is a mapping reported", problems)
	}
}

// TestCheckWorkflowJobs_Finding_NamesTheWorkflowAndReadsScriptsFromTheRoot
// verifies which of the two plain strings the job rules take is which.
//
// The workflow's label and the repository root are both strings, so a call that
// passed them the other way round would still produce one finding per offending
// job: the label would read as a directory and the scripts would be looked for
// under the workflow's own name. Only a finding asserted whole, from a root
// that really carries the script it names, tells the two apart.
func TestCheckWorkflowJobs_Finding_NamesTheWorkflowAndReadsScriptsFromTheRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "publish.sh"), []byte("npx --yes thing\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	doc := map[string]any{"jobs": map[string]any{
		"release": map[string]any{
			"permissions": map[string]any{"id-token": "write"},
			"steps":       []any{map[string]any{"run": "bash scripts/publish.sh"}},
		},
	}}

	problems := newAudit(root, tables{}).checkWorkflowJobs(".github/workflows/release.yml", doc, nil)
	want := ".github/workflows/release.yml: job release: scripts/publish.sh matches " + `'\\bnpx\\b'` +
		": npx resolves a dependency tree at run time (use a lockfile and npm ci, or drop the CLI)"
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkWorkflowJobs() = %v, want exactly [%q]", problems, want)
	}
}

// TestOrderedKeys_DocumentOrder_NamesEachJobOnce verifies the order the job
// rules iterate in: the document's own order first, each key once, then
// whatever that order does not name, sorted.
//
// The order is read from the parsed nodes and the jobs from a decoded map, so
// the two can disagree. A name the order carries and the mapping does not would
// otherwise be audited as an empty job, and a name the order carries twice
// would be audited twice and reported twice.
func TestOrderedKeys_DocumentOrder_NamesEachJobOnce(t *testing.T) {
	t.Parallel()

	jobs := map[string]any{"zulu": 1, "alpha": 2, "mike": 3}

	cases := []struct {
		name  string
		order []string
		want  []string
	}{
		{name: "no order at all sorts", order: nil, want: []string{"alpha", "mike", "zulu"}},
		{name: "a full order is kept", order: []string{"zulu", "mike", "alpha"}, want: []string{"zulu", "mike", "alpha"}},
		{name: "what the order omits follows, sorted", order: []string{"zulu"}, want: []string{"zulu", "alpha", "mike"}},
		{name: "a name the mapping lost is dropped", order: []string{"gone", "zulu"}, want: []string{"zulu", "alpha", "mike"}},
		{name: "a name written twice appears once", order: []string{"zulu", "zulu"}, want: []string{"zulu", "alpha", "mike"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := orderedKeys(jobs, testCase.order)
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Errorf("orderedKeys(%v) = %v, want %v", testCase.order, got, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_DownloadedTool_MustBePinned verifies that tools an
// action downloads are pinned, not just the action.
//
// SHA-pinning goreleaser-action or sbom-action fixes the JavaScript that runs;
// the binary it then fetches is chosen by a version input, and 'latest' or
// '~> v2' leaves that binary unpinned in the same job that holds the signing
// identity. There is no pinned case for sbom-action: the action is refused
// outright in a credentialed job whatever syft-version says, because on Linux
// it fetches install.sh from syft's main branch and runs it.
func TestCheckCredentialedJob_DownloadedTool_MustBePinned(t *testing.T) {
	t.Parallel()

	cases := []struct {
		with        map[string]any
		name        string
		action      string
		wantProblem bool
	}{
		{
			name:        "goreleaser range",
			action:      "goreleaser/goreleaser-action",
			with:        map[string]any{"version": "~> v2"},
			wantProblem: true,
		},
		{
			name:        "goreleaser latest",
			action:      "goreleaser/goreleaser-action",
			with:        map[string]any{"version": "latest"},
			wantProblem: true,
		},
		{
			name:        "goreleaser exact",
			action:      "goreleaser/goreleaser-action",
			with:        map[string]any{"version": "v2.13.0"},
			wantProblem: false,
		},
		{
			name:        "goreleaser version absent",
			action:      "goreleaser/goreleaser-action",
			with:        map[string]any{},
			wantProblem: true,
		},
		{name: "syft unpinned", action: "anchore/sbom-action/download-syft", with: map[string]any{}, wantProblem: true},
		{
			name:        "syft floating major",
			action:      "anchore/sbom-action/download-syft",
			with:        map[string]any{"syft-version": "v1"},
			wantProblem: true,
		},
		{
			name:        "syft version pinned, action still refused",
			action:      "anchore/sbom-action/download-syft",
			with:        map[string]any{"syft-version": "v1.36.0"},
			wantProblem: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"uses": testCase.action + "@" + testSHA, "with": testCase.with}},
			}
			problems := newAudit(".", tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkCredentialedJob() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestCheckCredentialedJob_GoreleaserVersion_IsQuotedLikePython verifies the
// exact finding text for an unpinned GoReleaser, including the Python-style
// quoting of the offending version. That quoting is what makes this program's
// output byte-identical to the auditor it replaces.
func TestCheckCredentialedJob_GoreleaserVersion_IsQuotedLikePython(t *testing.T) {
	t.Parallel()

	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps": []any{map[string]any{
			"uses": "goreleaser/goreleaser-action@" + testSHA,
			"with": map[string]any{"version": "~> v2"},
		}},
	}
	problems := newAudit(".", tables{}).checkCredentialedJob("release.yml", "release", job, nil)
	want := "release.yml: job release: step 0: goreleaser-action version '~> v2' is not an exact vX.Y.Z — " +
		"pinning the action does not pin the binary it downloads"
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
	}
}

// TestCheckCredentialedJob_EnvDeclaredVersion_IsResolved verifies that a pin
// held in env: and referenced by a step is still a pin.
//
// Version pins are declared once at the top of the workflow and referenced from
// the step that downloads the tool, so a checker that cannot see through that
// one indirection reads every pin as unpinned and the rule becomes noise a
// maintainer learns to ignore.
func TestCheckCredentialedJob_EnvDeclaredVersion_IsResolved(t *testing.T) {
	t.Parallel()

	cases := []struct {
		env         map[string]any
		name        string
		wantProblem bool
	}{
		{name: "resolves to an exact version", env: map[string]any{"GORELEASER_VERSION": "v2.18.0"}, wantProblem: false},
		{name: "resolves to a range", env: map[string]any{"GORELEASER_VERSION": "~> v2"}, wantProblem: true},
		{name: "names an undefined variable", env: map[string]any{}, wantProblem: true},
		{name: "workflow declares no env block", env: nil, wantProblem: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			doc := map[string]any{}
			if testCase.env != nil {
				doc["env"] = testCase.env
			}
			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps": []any{map[string]any{
					"uses": "goreleaser/goreleaser-action@" + testSHA,
					"with": map[string]any{"version": "${{ env.GORELEASER_VERSION }}"},
				}},
			}
			problems := newAudit(".", tables{}).checkCredentialedJob("wf.yml", "release", job, doc)
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkCredentialedJob() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestCheckCredentialedJob_RunBlock_RejectsRunTimeCode verifies that a
// credentialed job runs nothing resolved at run time.
//
// The release job holds the npm and PyPI trusted-publisher identities, the
// cosign signer and a write-scoped token, and used to invoke `npx --yes`, whose
// nine caret ranges were resolved fresh from the registry on every release. A
// rationale comment mentioning the pattern is not the pattern.
func TestCheckCredentialedJob_RunBlock_RejectsRunTimeCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		run         string
		wantProblem bool
	}{
		{name: "npx", run: "npx --yes @anthropic-ai/mcpb@2.1.2 pack a b", wantProblem: true},
		{name: "go install latest", run: "go install gotest.tools/gotestsum@latest", wantProblem: true},
		{name: "curl piped to sh", run: "curl -fsSL https://example.test/i.sh | sh", wantProblem: true},
		{name: "curl piped to bash", run: "curl -fsSL https://example.test/i.sh | bash", wantProblem: true},
		{name: "pip install unhashed", run: "pip install --quiet jsonschema", wantProblem: true},
		{name: "pip install hashed", run: "pip install --require-hashes -r req.txt", wantProblem: false},
		{name: "comment about npx", run: "# Pinned, not @latest: npx would resolve at run time\nnpm ci", wantProblem: false},
		{name: "pinned npm install", run: "npm install -g npm@11.5.1", wantProblem: false},
		{name: "plain build", run: "go build ./...", wantProblem: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"contents": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(".", tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkCredentialedJob() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestPipInstallWithoutHashes_Line_DecidesTheMatch verifies the RE2 rendering
// of the one rule the original expressed with a lookahead.
//
// Python asked for \bpip\s+install\b(?![^\n]*--require-hashes); Go's regexp is
// RE2 and has no lookahead, so the rule is a match plus a search of the rest of
// the same line. Both halves are load-bearing: without the second, a hashed
// install is a false positive that would push someone to weaken the rule; with
// only the second, the whole rule silently passes everything. The line scoping
// is load-bearing too — a --require-hashes on the *next* line does not make
// this line's install reproducible.
func TestPipInstallWithoutHashes_Line_DecidesTheMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		run  string
		want bool
	}{
		{name: "hashed install passes", run: "pip install --require-hashes -r r.txt", want: false},
		{name: "unhashed install fails", run: "pip install pyyaml", want: true},
		{name: "hashed requirement before the flag", run: "pip install -r r.txt --require-hashes", want: false},
		{name: "python -m pip is still pip install", run: "python3 -m pip install pyyaml", want: true},
		{name: "hashes on the following line do not count", run: "pip install pyyaml\n--require-hashes", want: true},
		{name: "hashes right after the line ends do not count", run: "pip install\n--require-hashes", want: true},
		{name: "one hashed and one unhashed line", run: "pip install --require-hashes -r r.txt\npip install pyyaml", want: true},
		{name: "two hashed lines", run: "pip install --require-hashes -r a.txt\npip install --require-hashes -r b.txt", want: false},
		{name: "unhashed install at end of text", run: "pip install pyyaml", want: true},
		{name: "no pip at all", run: "make build", want: false},
		{name: "pipx is not pip", run: "pipx install ruff", want: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := pipInstallWithoutHashes(testCase.run); got != testCase.want {
				t.Errorf("pipInstallWithoutHashes(%q) = %t, want %t", testCase.run, got, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_ReferencedScript_IsScanned verifies that a rule
// applied to a run: block is applied to the scripts it invokes.
//
// Moving `npx --yes` from a workflow into a shell script the workflow calls
// does not make the dependency tree any more fixed, so the auditor follows the
// call rather than stopping at the YAML.
func TestCheckCredentialedJob_ReferencedScript_IsScanned(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		body        string
		wantProblem bool
	}{
		{name: "script runs npx", body: "npx --yes \"@anthropic-ai/mcpb@2.1.2\" pack a b\n", wantProblem: true},
		{name: "script uses zip", body: "cd bundle && zip -r -X ../out.mcpb .\n", wantProblem: false},
		{name: "script only mentions npx in a comment", body: "# npx would resolve at run time\nzip -r out.mcpb .\n", wantProblem: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			scriptPath := filepath.Join(root, "scripts", "build-thing.sh")
			if err := os.WriteFile(scriptPath, []byte(testCase.body), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": "bash scripts/build-thing.sh 2.7.5"}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkCredentialedJob() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestCheckCredentialedJob_UnresolvedReference_IsReported verifies that a
// script reference this audit cannot read is a finding, whatever the reason.
//
// It used to be skipped with a bare continue, which is how two scripts under
// .github/scripts/ ran in the job that holds the attestation identity without
// ever being judged: the reference resolved to a path that did not exist and
// nothing said so. A gate whose blind spot is silent is one the next script
// steps into. A path that leaves the repository is refused before anything is
// read, so a file of the same name beside the checkout is never judged in the
// repository's place, and a directory is refused before the read too, since a
// named pipe in its place would block the read and hang the audit.
func TestCheckCredentialedJob_UnresolvedReference_IsReported(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	writeFile(t, parent, "scripts/outside.sh", "npx --yes thing\n")
	if err := os.MkdirAll(filepath.Join(root, "scripts", "a-directory.sh"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cases := []struct {
		name     string
		run      string
		written  string
		resolved string
	}{
		{name: "absent", run: "bash scripts/not-here.sh", written: "scripts/not-here.sh", resolved: "scripts/not-here.sh"},
		{
			name: "a directory", run: "bash scripts/a-directory.sh",
			written: "scripts/a-directory.sh", resolved: "scripts/a-directory.sh",
		},
		{
			name: "leaves the repository", run: "bash ../scripts/outside.sh",
			written: "../scripts/outside.sh", resolved: "../scripts/outside.sh",
		},
		{
			name: "absolute", run: "bash /opt/tools/scripts/outside.sh",
			written: "/opt/tools/scripts/outside.sh", resolved: "/opt/tools/scripts/outside.sh",
		},
		{
			name: "a url", run: "echo see https://example.test/scripts/outside.sh",
			written: "/example.test/scripts/outside.sh", resolved: "/example.test/scripts/outside.sh",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			want := unresolvedFinding("wf.yml: job release: step 0", testCase.written, testCase.resolved)
			if len(problems) != 1 || problems[0] != want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
			}
		})
	}
}

// TestCheckCredentialedJob_UnreadableScript_IsReported verifies the arm that
// runs when the shape check passed and the read still failed.
//
// Permissions cannot produce that here: the gate is run by a privileged user in
// a container, for whom a mode-0 file reads fine, so a chmod-based fixture would
// skip exactly where the arm needs exercising. /proc/self/mem is the one path
// that is both: the kernel reports a regular file and refuses the read. The
// test states that precondition and declines rather than passing vacuously
// wherever it does not hold.
func TestCheckCredentialedJob_UnreadableScript_IsReported(t *testing.T) {
	t.Parallel()

	const unreadable = "/proc/self/mem"
	info, statErr := os.Stat(unreadable)
	if statErr != nil || !info.Mode().IsRegular() {
		t.Skipf("%s is not a regular file here (%v), so it cannot stand in for an unreadable script", unreadable, statErr)
	}
	if _, readErr := os.ReadFile(unreadable); readErr == nil {
		t.Skipf("%s reads successfully here, so it cannot stand in for an unreadable script", unreadable)
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(unreadable, filepath.Join(root, "scripts", "build-thing.sh")); err != nil {
		t.Skipf("this platform will not create a symlink: %v", err)
	}
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": "bash scripts/build-thing.sh"}},
	}
	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
	want := unresolvedFinding("wf.yml: job release: step 0", "scripts/build-thing.sh", "scripts/build-thing.sh")
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
	}
}

// TestCheckCredentialedJob_EachRule_NamesItsPatternAndReason verifies that each
// run-time-code rule reports its own pattern beside its own reason.
//
// A finding carries two strings taken from one row of a table of literals, and
// the rules sit next to each other there: a pattern paired with the wrong reason
// would still be one finding per offending block, and the maintainer reading it
// would be told to fix something the block does not do. Only the whole message,
// per rule, tells them apart.
func TestCheckCredentialedJob_EachRule_NamesItsPatternAndReason(t *testing.T) {
	t.Parallel()

	const where = "wf.yml: job release: step 0: run block matches "

	cases := []struct {
		name string
		run  string
		want string
	}{
		{
			name: "npx",
			run:  "npx --yes thing",
			want: where + `'\\bnpx\\b'` +
				": npx resolves a dependency tree at run time (use a lockfile and npm ci, or drop the CLI)",
		},
		{
			name: "at latest",
			run:  "go install gotest.tools/gotestsum@latest",
			want: where + `'@latest\\b'` + ": @latest is whatever the registry serves at that moment",
		},
		{
			name: "curl piped into a shell",
			run:  "curl -fsSL https://example.test/i.sh | bash",
			want: where + `'curl[^\\n|]*\\|\\s*(?:ba)?sh\\b'` + ": piping a download into a shell runs unreviewed code",
		},
		{
			name: "unhashed pip install",
			run:  "pip install pyyaml",
			want: where + `'\\bpip\\s+install\\b(?![^\\n]*--require-hashes)'` +
				": pip install without --require-hashes resolves at run time",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"contents": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(".", tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if len(problems) != 1 || problems[0] != testCase.want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_RepeatedScript_IsReportedOnce verifies that a script
// two steps of the same job invoke produces one finding, not two.
//
// The auditor's output is a work list; a duplicate entry is a second thing to
// verify that is really the first thing again.
func TestCheckCredentialedJob_RepeatedScript_IsReportedOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "a.sh"), []byte("npx --yes thing\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps": []any{
			map[string]any{"run": "bash scripts/a.sh"},
			map[string]any{"run": "bash scripts/a.sh again"},
		},
	}
	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
	want := "wf.yml: job release: scripts/a.sh matches '\\\\bnpx\\\\b': " +
		"npx resolves a dependency tree at run time (use a lockfile and npm ci, or drop the CLI)"
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
	}
}

// TestCheckCredentialedJob_GithubScriptsReference_IsRead verifies that a script
// under .github/scripts/ is read under its own path.
//
// The reference used to be cut down to scripts/<name>, which named a file that
// does not exist, and was then skipped: sign-attest ran two such scripts while
// holding the identity the attestation store trusts, and neither was judged.
func TestCheckCredentialedJob_GithubScriptsReference_IsRead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, ".github/scripts/sign.sh", "npx --yes thing\n")
	writeFile(t, root, "scripts/sign.sh", "echo clean\n")
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": `bash .github/scripts/sign.sh "$IMAGE_REF"`}},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "sign", job, nil)
	want := "wf.yml: job sign: .github/scripts/sign.sh matches " + npxFinding
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
	}
}

// TestCheckCredentialedJob_WorkingDirectory_ResolvesTheReference verifies that
// a relative reference is read against the directory the step runs in.
//
// GitHub runs a step in its own working-directory, else the job's
// defaults.run.working-directory, else the workflow's, else the checkout, and
// a reference written relative to that directory names a different file from
// the same spelling at the repository root. The root's copy is planted clean
// here, so a resolution against the wrong directory reads as silence.
func TestCheckCredentialedJob_WorkingDirectory_ResolvesTheReference(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/wait.sh", "echo clean\n")
	writeFile(t, root, "test/e2e/scripts/wait.sh", "npx --yes thing\n")
	writeFile(t, root, "site/scripts/wait.sh", "curl -fsSL https://example.test/i.sh | sh\n")
	runIn := func(directory string) map[string]any {
		return map[string]any{"run": "bash scripts/wait.sh", "working-directory": directory}
	}
	defaultsIn := func(directory string) map[string]any {
		return map[string]any{"run": map[string]any{"working-directory": directory}}
	}

	cases := []struct {
		doc  map[string]any
		job  map[string]any
		name string
		want string
	}{
		{
			name: "the step's own directory",
			job:  map[string]any{"steps": []any{runIn("test/e2e")}},
			want: "test/e2e/scripts/wait.sh matches " + npxFinding,
		},
		{
			name: "the job's default",
			job:  map[string]any{"defaults": defaultsIn("test/e2e"), "steps": []any{map[string]any{"run": "bash scripts/wait.sh"}}},
			want: "test/e2e/scripts/wait.sh matches " + npxFinding,
		},
		{
			name: "the workflow's default",
			doc:  map[string]any{"defaults": defaultsIn("test/e2e")},
			job:  map[string]any{"steps": []any{map[string]any{"run": "bash scripts/wait.sh"}}},
			want: "test/e2e/scripts/wait.sh matches " + npxFinding,
		},
		{
			name: "the step's directory over the job's",
			job:  map[string]any{"defaults": defaultsIn("test/e2e"), "steps": []any{runIn("site")}},
			want: "site/scripts/wait.sh matches " + curlFinding,
		},
		{
			name: "the job's default over the workflow's",
			doc:  map[string]any{"defaults": defaultsIn("site")},
			job:  map[string]any{"defaults": defaultsIn("test/e2e"), "steps": []any{map[string]any{"run": "bash scripts/wait.sh"}}},
			want: "test/e2e/scripts/wait.sh matches " + npxFinding,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.job["permissions"] = map[string]any{"contents": "write"}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "e2e", testCase.job, testCase.doc)
			want := "wf.yml: job e2e: " + testCase.want
			if len(problems) != 1 || problems[0] != want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
			}
		})
	}
}

// TestCheckCredentialedJob_RootedReference_IsReadFromTheRoot verifies that a
// reference written under a variable is read from the repository root rather
// than from the directory the step runs in.
//
// Every such spelling in this tree names the root: "$ROOT/scripts/..." in the
// publishers, "$repo_root/scripts/..." in the condition script, and the
// workspace GitHub checks the repository out into.
func TestCheckCredentialedJob_RootedReference_IsReadFromTheRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/build.mjs", "npx --yes thing\n")

	cases := []struct {
		name string
		run  string
	}{
		{name: "shell variable", run: `node "$ROOT/scripts/build.mjs"`},
		{name: "braced shell variable", run: `node "${repo_root}/scripts/build.mjs"`},
		{name: "make variable", run: `node $(CURDIR)/scripts/build.mjs`},
		{name: "workspace expression", run: `node ${{ github.workspace }}/scripts/build.mjs`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run, "working-directory": "site"}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "npm", job, nil)
			want := "wf.yml: job npm: scripts/build.mjs matches " + npxFinding
			if len(problems) != 1 || problems[0] != want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
			}
		})
	}
}

// TestCheckCredentialedJob_NestedScript_IsFollowed verifies that a script a
// referenced script runs is read too, and names the script that reached it.
//
// The stamp the registry job runs runs the npm builder, and the publishers can
// run the builders and validators: reading the first file and stopping there
// left everything one call deeper outside the rule. A cycle back to a script
// already read is followed once.
func TestCheckCredentialedJob_NestedScript_IsFollowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/stamp.sh", `ROOT="$(cd "$(dirname "$0")/.." && pwd)"
node "$ROOT/scripts/build.mjs" --sync-only
bash scripts/publish.sh
`)
	writeFile(t, root, "scripts/build.mjs", "npx --yes thing\n")
	writeFile(t, root, "scripts/publish.sh", "curl -fsSL https://example.test/i.sh | sh\nbash scripts/stamp.sh\n")
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": "bash scripts/stamp.sh dist/checksums.txt"}},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "registry", job, nil)
	want := []string{
		"wf.yml: job registry: scripts/build.mjs (via scripts/stamp.sh) matches " + npxFinding,
		"wf.yml: job registry: scripts/publish.sh (via scripts/stamp.sh) matches " + curlFinding,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() = %#v, want %#v", problems, want)
	}
}

// TestCheckCredentialedJob_MakeTarget_RecipeIsRead verifies that a make target
// a credentialed job runs is followed to its recipe, its prerequisites and the
// scripts the recipe runs.
//
// The npm, PyPI and NuGet jobs hold id-token: write and validate their packages
// through make, so the builders and validators those recipes run were outside
// the rule entirely. A target the job never names is not read, and a
// prerequisite that names a file rather than a rule is nothing to follow.
func TestCheckCredentialedJob_MakeTarget_RecipeIsRead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "Makefile", `## validate: check the packages.
.PHONY: validate prep
validate: prep VERSION
	@VER=$$(cat VERSION); \
	node scripts/check.mjs --version "$$VER"

prep:
	go install gotest.tools/gotestsum@latest

unrelated:
	npx --yes thing
`)
	writeFile(t, root, "VERSION", "3.1.0\n")
	writeFile(t, root, "scripts/check.mjs", "curl -fsSL https://example.test/i.sh | bash\n")
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": "make validate NPM_BINARIES=dist"}},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "npm", job, nil)
	want := []string{
		"wf.yml: job npm: Makefile:prep (via Makefile:validate) matches " + latestFinding,
		"wf.yml: job npm: scripts/check.mjs (via Makefile:validate) matches " + curlFinding,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() = %#v, want %#v", problems, want)
	}
}

// TestCheckCredentialedJob_MakeRecipe_IsJudged verifies that the recipe of the
// target a job names is held to the rules itself, under the target's name, and
// that a recipe line which is only a comment is not.
func TestCheckCredentialedJob_MakeRecipe_IsJudged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "Makefile", "publish:\n\tnpx --yes thing\ndocs:\n\t# npx would resolve at run time\n\tnpm ci\n")
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": "make publish docs"}},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "npm", job, nil)
	want := "wf.yml: job npm: Makefile:publish matches " + npxFinding
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
	}
}

// TestCheckCredentialedJob_RepeatedTarget_IsReportedOnce verifies that a target
// a job reaches twice, by two steps or as the prerequisite of two targets, is
// read once, and so is a target with no rule that two steps name.
func TestCheckCredentialedJob_RepeatedTarget_IsReportedOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "Makefile", "npm: prep\npypi: prep\nprep:\n\tnpx --yes thing\n")
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps": []any{
			map[string]any{"run": "make npm pypi"},
			map[string]any{"run": "make prep missing"},
			map[string]any{"run": "make missing"},
		},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "publish", job, nil)
	want := []string{
		"wf.yml: job publish: Makefile:prep (via Makefile:npm) matches " + npxFinding,
		"wf.yml: job publish: step 1: make missing has no rule in Makefile, so its recipe cannot be judged",
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() = %#v, want %#v", problems, want)
	}
}

// TestCheckCredentialedJob_UnfollowableMake_IsReported verifies that a make
// invocation the audit cannot follow is a finding rather than a silence: a
// target with no rule, a Makefile that is not there, a make that names no
// target and so runs whatever the default goal is, a make whose arguments it
// cannot read, and a make behind a wrapper written in a way it does not read.
func TestCheckCredentialedJob_UnfollowableMake_IsReported(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "Makefile", "build:\n\tgo build ./...\n")

	cases := []struct {
		step map[string]any
		name string
		want string
	}{
		{
			name: "no rule",
			step: map[string]any{"run": "make validate-npm-local"},
			want: "wf.yml: job npm: step 0: make validate-npm-local has no rule in Makefile, so its recipe cannot be judged",
		},
		{
			name: "no Makefile there",
			step: map[string]any{"run": "make build", "working-directory": "site"},
			want: "wf.yml: job npm: step 0: make build has no rule in site/Makefile, so its recipe cannot be judged",
		},
		{
			name: "no target",
			step: map[string]any{"run": "make"},
			want: "wf.yml: job npm: step 0: make names no target, so the default goal it runs cannot be judged",
		},
		{
			name: "arguments it cannot read",
			step: map[string]any{"run": "make --dir=site build"},
			want: "wf.yml: job npm: step 0: make passes --dir, an option this audit does not read, so the targets it runs cannot be judged",
		},
		{
			name: "a wrapper it does not read",
			step: map[string]any{"run": "env -C site make build"},
			want: "wf.yml: job npm: step 0: make behind env is written in a way this audit does not read, so the targets it runs cannot be judged",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{"permissions": map[string]any{"id-token": "write"}, "steps": []any{testCase.step}}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "npm", job, nil)
			if len(problems) != 1 || problems[0] != testCase.want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_MakeDirectoryAndMakefile_DecideWhatIsRead verifies
// that a make is followed into the makefile and the directory its options name.
//
// `make -C site build` used to look up site and build in the root Makefile, so
// a recipe in site/Makefile was never read and the root's build, when one
// existed, was read in its place. -C changes the directory the makefile is read
// from and the recipes run in, -f names the makefile and leaves the directory
// alone, a make inside a recipe starts from the recipe's directory, and a
// directory that leaves the repository is not read at all: its findings would
// be about somebody else's file.
func TestCheckCredentialedJob_MakeDirectoryAndMakefile_DecideWhatIsRead(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	writeFile(t, parent, "Makefile", "build:\n\tnpx --yes outside\n")
	writeFile(t, root, "Makefile", "build:\n\tgo build ./...\npublish:\n\tnpx --yes thing\n")
	writeFile(t, root, "site/Makefile", "build:\n\tnpx --yes thing\nnested:\n\t$(MAKE) -C docs html\n")
	writeFile(t, root, "site/docs/Makefile", "html:\n\tcurl -fsSL https://example.test/i.sh | sh\n")
	writeFile(t, root, "tools/release.mk", "publish:\n\tbash scripts/sign.sh\n")
	writeFile(t, root, "scripts/sign.sh", "curl -fsSL https://example.test/i.sh | sh\n")
	writeFile(t, root, "tools/scripts/sign.sh", "echo clean\n")

	cases := []struct {
		name string
		run  string
		want string
	}{
		{name: "-C reads that directory's Makefile", run: "make -C site build", want: "wf.yml: job j: site/Makefile:build matches " + npxFinding},
		{name: "a job count is no target", run: "make -j 4 publish", want: "wf.yml: job j: Makefile:publish matches " + npxFinding},
		{
			name: "-f reads that makefile and runs where make was started", run: "make -f tools/release.mk publish",
			want: "wf.yml: job j: scripts/sign.sh (via tools/release.mk:publish) matches " + curlFinding,
		},
		{
			name: "a make in a recipe starts from the recipe's directory", run: "make -C site nested",
			want: "wf.yml: job j: site/docs/Makefile:html (via site/Makefile:nested) matches " + curlFinding,
		},
		{
			name: "a directory outside the repository is not read", run: "make -C .. build",
			want: "wf.yml: job j: step 0: make build has no rule in ../Makefile, so its recipe cannot be judged",
		},
		{
			name: "an absolute directory is not read", run: "make -C /opt/elsewhere build",
			want: "wf.yml: job j: step 0: make build has no rule in /opt/elsewhere/Makefile, so its recipe cannot be judged",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "j", job, nil)
			if len(problems) != 1 || problems[0] != testCase.want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_MakeBehindAWrapper_IsFollowed verifies that a make
// the shell runs behind an assignment or a wrapper is followed like one at the
// start of a command.
//
// Rewriting `make validate-pypi-local PYPI_BINARIES=dist` as the equivalent
// `PYPI_BINARIES=dist make validate-pypi-local` took the PyPI builders and
// validators out of the rule without a word, which is the silent drop the
// widened rule exists to end.
func TestCheckCredentialedJob_MakeBehindAWrapper_IsFollowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "Makefile", "publish:\n\tnpx --yes thing\n")

	for _, run := range []string{
		"NPM_BINARIES=dist make publish",
		"env FOO=1 make publish",
		"timeout 600 make publish",
		`bash -c "make publish"`,
		"time nice -n 5 make publish",
	} {
		t.Run(run, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "pypi", job, nil)
			want := "wf.yml: job pypi: Makefile:publish matches " + npxFinding
			if len(problems) != 1 || problems[0] != want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
			}
		})
	}
}

// TestCheckCredentialedJob_ScriptBesideItsCaller_IsFollowed verifies that a
// script a file names through its own location is read beside that file.
//
// test/e2e/scripts/run-docker-e2e.sh sources and runs its siblings as
// "${SCRIPT_DIR}/name.sh", which carries no scripts/ component, so a job
// reaching it would have run every one of them unread. The directory comes from the
// file's own assignment, as many directories up as it climbs, from an inline
// dirname of $0, or from $PSScriptRoot in PowerShell; the workspace is the
// root in either spelling; and a path through a scripts/ directory is left to
// the reading that already roots it, so it is read once.
func TestCheckCredentialedJob_ScriptBesideItsCaller_IsFollowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "test/e2e/scripts/run.sh", `SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/lib.sh"
bash "$(dirname "$0")/setup.sh"
export ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
bash "$ROOT/tools/stamp.sh"
`)
	writeFile(t, root, "test/e2e/scripts/lib.sh", "npx --yes thing\n")
	writeFile(t, root, "test/e2e/scripts/setup.sh", "curl -fsSL https://example.test/i.sh | sh\n")
	writeFile(t, root, "tools/stamp.sh", "go install gotest.tools/gotestsum@latest\n")
	writeFile(t, root, "scripts/install.ps1", "& \"$PSScriptRoot/helper.ps1\"\n")
	writeFile(t, root, "scripts/helper.ps1", "npx --yes thing\n")
	writeFile(t, root, "tools/scripts/build.mjs", "npx --yes thing\n")

	cases := []struct {
		name string
		run  string
		want []string
	}{
		{
			name: "the script's own directory", run: "bash test/e2e/scripts/run.sh",
			want: []string{
				"wf.yml: job j: test/e2e/scripts/lib.sh (via test/e2e/scripts/run.sh) matches " + npxFinding,
				"wf.yml: job j: test/e2e/scripts/setup.sh (via test/e2e/scripts/run.sh) matches " + curlFinding,
				"wf.yml: job j: tools/stamp.sh (via test/e2e/scripts/run.sh) matches " + latestFinding,
			},
		},
		{
			name: "PowerShell's own directory", run: "pwsh scripts/install.ps1",
			want: []string{"wf.yml: job j: scripts/helper.ps1 (via scripts/install.ps1) matches " + npxFinding},
		},
		{
			name: "the workspace", run: "bash \"${{ github.workspace }}/tools/stamp.sh\"\nbash \"$GITHUB_WORKSPACE/tools/stamp.sh\"",
			want: []string{"wf.yml: job j: tools/stamp.sh matches " + latestFinding},
		},
		{
			name: "a path through a scripts/ directory", run: `node "$ROOT/tools/scripts/build.mjs"`,
			want: []string{"wf.yml: job j: tools/scripts/build.mjs matches " + npxFinding},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "j", job, nil)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkCredentialedJob() = %#v, want %#v", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_UnresolvableScriptRoot_IsReported verifies that a
// script rooted in a value only the run knows is a finding rather than a
// silence.
//
// A variable assigned something other than the script's own directory or one
// above it, a variable assigned nowhere, a workflow expression other than the
// workspace, and the script's own location named in a run block or a recipe,
// where it means nothing, are each reported; a directory above the script that
// leaves the repository is reported as the path it resolves to.
func TestCheckCredentialedJob_UnresolvableScriptRoot_IsReported(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/run.sh", `SUB="$(cd "$(dirname "$0")/sub" && pwd)"
bash "$SUB/a.sh"
bash "$OTHER/b.sh"
UP="$(cd "$(dirname "$0")/../.." && pwd)"
bash "$UP/c.sh"
`)

	cases := []struct {
		name string
		run  string
		want []string
	}{
		{
			name: "in a script", run: "bash scripts/run.sh",
			want: []string{
				rootFinding("wf.yml: job j: scripts/run.sh", "$SUB/a.sh", "$SUB"),
				rootFinding("wf.yml: job j: scripts/run.sh", "$OTHER/b.sh", "$OTHER"),
				unresolvedFinding("wf.yml: job j: scripts/run.sh", "$UP/c.sh", "../c.sh"),
			},
		},
		{
			name: "in a run block", run: `bash "$RUNNER_TEMP/install.sh"` + "\n" + `bash "${{ runner.temp }}/x.sh"` + "\n" + `bash "$(dirname "$0")/y.sh"`,
			want: []string{
				rootFinding("wf.yml: job j: step 0", "$RUNNER_TEMP/install.sh", "$RUNNER_TEMP"),
				rootFinding("wf.yml: job j: step 0", "${{ runner.temp }}/x.sh", "${{ runner.temp }}"),
				rootFinding("wf.yml: job j: step 0", `$(dirname "$0")/y.sh`, `$(dirname "$0")`),
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "j", job, nil)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkCredentialedJob() = %#v, want %#v", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_StatedLimits_AreNotFollowed pins what rule 2 states
// it cannot see, so that the day it learns to, this test says so.
//
// A make or a script handed to a language's process or path API is an argument
// and not a command or a path, and a make run through a shell function or a
// variable holding its name has nothing in front of it the audit can read. A
// make mentioned in prose is not a command at all, which is why an unread
// mention is not a finding. The first case is the control: the same target
// run as a command is followed.
func TestCheckCredentialedJob_StatedLimits_AreNotFollowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "Makefile", "publish:\n\tnpx --yes thing\n")
	writeFile(t, root, "scripts/helper.sh", "npx --yes thing\n")
	writeFile(t, root, "scripts/helper.mjs", "npx --yes thing\n")
	writeFile(t, root, "scripts/spawn.mjs", `spawnSync("make", ["publish"]);
const helper = path.join(__dirname, "helper.mjs");
`)
	writeFile(t, root, "scripts/run.py", `subprocess.run(["make", "publish"], check=True)
subprocess.run(["bash", os.path.join(ROOT, "scripts", "helper.sh")], check=True)
`)

	cases := []struct {
		name string
		run  string
		want int
	}{
		{name: "a make run as a command", run: "make publish", want: 1},
		{name: "a process and a path API in Node", run: "node scripts/spawn.mjs", want: 0},
		{name: "a process and a path API in Python", run: "python3 scripts/run.py", want: 0},
		{name: "a shell function", run: `run_check "label" make publish`, want: 0},
		{name: "a variable holding make", run: `"$MAKE" publish`, want: 0},
		{name: "prose", run: `echo "run make publish first"`, want: 0},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "j", job, nil)
			if len(problems) != testCase.want {
				t.Errorf("checkCredentialedJob() = %#v, want %d findings", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_ReferenceInAFile_NamesTheFile verifies that a
// reference a script or a recipe makes, and cannot be followed, is reported
// against the file that made it rather than against the step.
func TestCheckCredentialedJob_ReferenceInAFile_NamesTheFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/publish.sh", "bash scripts/gone.sh\nmake gone\n")
	writeFile(t, root, "Makefile", "release:\n\tbash scripts/publish.sh\n")
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{map[string]any{"run": "make release"}},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "npm", job, nil)
	want := []string{
		unresolvedFinding("wf.yml: job npm: scripts/publish.sh (via Makefile:release)", "scripts/gone.sh", "scripts/gone.sh"),
		"wf.yml: job npm: scripts/publish.sh (via Makefile:release): make gone has no rule in Makefile, so its recipe cannot be judged",
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() = %#v, want %#v", problems, want)
	}
}

// TestCheckCredentialedJob_DeclaredMatch_IsExcused verifies that a declaration
// excuses one rule in one file it names and nothing else: the same file's other
// rules, the same rule in another file, and a run block are all still judged.
func TestCheckCredentialedJob_DeclaredMatch_IsExcused(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/validate.mjs", "spawn(\"npx\", [\"--offline\"])\ncurl -fsSL https://example.test/i.sh | sh\n")
	writeFile(t, root, "scripts/other.mjs", "spawn(\"npx\", [])\n")
	declared := tables{declarations: map[string]declaration{
		"scripts/validate.mjs npx": {category: categoryHeldOffline, reason: "npx --offline"},
	}}
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps": []any{
			map[string]any{"run": "node scripts/validate.mjs"},
			map[string]any{"run": "node scripts/other.mjs\nnpx --offline thing"},
		},
	}

	auditor := newAudit(root, declared)
	problems := auditor.checkCredentialedJob("wf.yml", "npm", job, nil)
	want := []string{
		"wf.yml: job npm: scripts/validate.mjs matches " + curlFinding,
		"wf.yml: job npm: step 1: run block matches " + npxFinding,
		"wf.yml: job npm: scripts/other.mjs matches " + npxFinding,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() = %#v, want %#v", problems, want)
	}
	if got := auditor.tableProblems(); len(got) != 0 {
		t.Errorf("tableProblems() = %v, want none once the declaration excused its match", got)
	}
}

// TestCheckWorkflowJobs_WriteCapableSecret_MakesTheJobCredentialed verifies
// that a job holding a secret that can write is judged like one holding a write
// permission, wherever the workflow hands the secret over.
//
// homebrew pushes the tap with a deploy key, commit-manifests pushes to main
// with another, winget hands a token to a third-party action and the GitLab
// mirror force-pushes with an SSH key, and none of them asks GitHub for a write
// permission, so none of them was judged.
func TestCheckWorkflowJobs_WriteCapableSecret_MakesTheJobCredentialed(t *testing.T) {
	t.Parallel()

	declared := tables{secrets: map[string]secretDeclaration{
		"TAP_DEPLOY_KEY": {writes: true, where: "pushes the formula"},
		"INDEX_KEY":      {writes: false, where: "asks for a recrawl"},
	}}
	checkout := map[string]any{"uses": "actions/checkout@" + testSHA}

	cases := []struct {
		doc         map[string]any
		job         map[string]any
		name        string
		wantProblem bool
	}{
		{
			name:        "in a step's env",
			job:         map[string]any{"steps": []any{checkout, map[string]any{"env": map[string]any{"K": "${{ secrets.TAP_DEPLOY_KEY }}"}}}},
			wantProblem: true,
		},
		{
			name:        "in the job's env",
			job:         map[string]any{"env": map[string]any{"K": "${{ secrets.TAP_DEPLOY_KEY }}"}, "steps": []any{checkout}},
			wantProblem: true,
		},
		{
			name:        "as an action's input",
			job:         map[string]any{"steps": []any{checkout, map[string]any{"with": map[string]any{"token": "${{secrets.TAP_DEPLOY_KEY}}"}}}},
			wantProblem: true,
		},
		{
			name:        "in the workflow's env",
			doc:         map[string]any{"env": map[string]any{"K": "${{ secrets.TAP_DEPLOY_KEY }}"}},
			job:         map[string]any{"steps": []any{checkout}},
			wantProblem: true,
		},
		{
			name:        "spelled in lower case",
			job:         map[string]any{"steps": []any{checkout, map[string]any{"env": map[string]any{"K": "${{ secrets.tap_deploy_key }}"}}}},
			wantProblem: true,
		},
		{
			name:        "indexed by name",
			job:         map[string]any{"steps": []any{checkout, map[string]any{"env": map[string]any{"K": "${{ secrets['TAP_DEPLOY_KEY'] }}"}}}},
			wantProblem: true,
		},
		{
			name:        "a secret that writes nothing",
			job:         map[string]any{"steps": []any{checkout, map[string]any{"env": map[string]any{"K": "${{ secrets.INDEX_KEY }}"}}}},
			wantProblem: false,
		},
		{
			name: "a write permission beside a secret that writes nothing",
			job: map[string]any{
				"permissions": map[string]any{"contents": "write"},
				"steps":       []any{checkout, map[string]any{"env": map[string]any{"K": "${{ secrets.INDEX_KEY }}"}}},
			},
			wantProblem: true,
		},
		{
			name:        "named outside an expression",
			job:         map[string]any{"steps": []any{checkout, map[string]any{"run": "echo secrets.TAP_DEPLOY_KEY is set"}}},
			wantProblem: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			doc := map[string]any{"jobs": map[string]any{"homebrew": testCase.job}}
			maps.Copy(doc, testCase.doc)
			problems := newAudit(".", declared).checkWorkflowJobs("wf.yml", doc, nil)
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkWorkflowJobs() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestCheckWorkflowJobs_UnjudgedSecretRead_IsReported verifies that a secret
// the table does not declare, and an expression that hands a step every secret
// at once, are findings, and that either makes the job credentialed: the audit
// cannot say what such a job holds, so it judges it as though it held a write.
func TestCheckWorkflowJobs_UnjudgedSecretRead_IsReported(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "undeclared",
			value: "${{ secrets.MYSTERY }}",
			want:  "wf.yml: job publish: reads secrets.MYSTERY, which the secret table does not declare: say whether it can write and where",
		},
		{
			name:  "every secret at once",
			value: "${{ toJSON(secrets) }}",
			want:  "wf.yml: job publish: reads ${{ toJSON(secrets) }}, which hands over every secret at once and cannot be judged against the secret table",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			doc := map[string]any{"jobs": map[string]any{"publish": map[string]any{
				"env":   map[string]any{"K": testCase.value},
				"steps": []any{map[string]any{"uses": "actions/checkout@" + testSHA}},
			}}}
			problems := newAudit(".", tables{}).checkWorkflowJobs("wf.yml", doc, nil)
			if len(problems) != 2 || problems[0] != testCase.want || !strings.Contains(problems[1], "persist-credentials") {
				t.Errorf("checkWorkflowJobs() = %#v, want [%q, the checkout finding]", problems, testCase.want)
			}
		})
	}
}

// TestJobSecrets_Expressions_NameWhatIsRead verifies the reading of the secrets
// a job's expressions name, each once and sorted, apart from the expressions
// that read all of them.
func TestJobSecrets_Expressions_NameWhatIsRead(t *testing.T) {
	t.Parallel()

	doc := map[string]any{"env": map[string]any{"A": "${{ secrets.ZULU }}"}}
	job := map[string]any{
		"secrets": map[string]any{"token": "${{ secrets.ALPHA }}"},
		"steps": []any{
			map[string]any{"run": "echo ${{ secrets.ALPHA }} ${{ secrets . MIKE || secrets.KILO }}", "with": map[string]any{"x": 1}},
			map[string]any{"env": map[string]any{"B": "${{ secrets[ 'echo' ] }}", "C": "${{ toJSON(secrets) }}"}},
		},
	}

	names, wholesale := jobSecrets(doc, job)
	if strings.Join(names, ",") != "ALPHA,ECHO,KILO,MIKE,ZULU" {
		t.Errorf("jobSecrets() names = %v, want [ALPHA ECHO KILO MIKE ZULU]", names)
	}
	if strings.Join(wholesale, ",") != "toJSON(secrets)" {
		t.Errorf("jobSecrets() wholesale = %v, want [toJSON(secrets)]", wholesale)
	}
}

// TestAuditWorkflows_SecretTable_IsHeldToTheWorkflows verifies the table's own
// two rules: an entry that no workflow reads is stale, and an entry that does
// not say where its secret can write says nothing a reviewer can check.
func TestAuditWorkflows_SecretTable_IsHeldToTheWorkflows(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "ci.yml", "jobs:\n  lint:\n    env:\n      A: ${{ secrets.READ }}\n      B: ${{ secrets.BLANK }}\n    steps: []\n")
	declared := tables{secrets: map[string]secretDeclaration{
		"READ":   {where: "reads a dashboard"},
		"BLANK":  {where: " "},
		"UNREAD": {writes: true, where: "pushes somewhere"},
	}}

	problems, err := auditWorkflows(root, declared)
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	want := []string{
		"cmd/audit_supply_chain/secrets.go: BLANK says nothing about what it can write",
		"cmd/audit_supply_chain/secrets.go: UNREAD is declared and no workflow reads it",
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() = %#v, want %#v", problems, want)
	}
}

// TestAuditWorkflows_Declarations_AreHeldToTheTree verifies the declaration
// table's rules: a key that is not a path and a rule name, a category this
// command does not define, an empty reason, and an entry that excused nothing.
func TestAuditWorkflows_Declarations_AreHeldToTheTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "scripts/used.mjs", "npx --offline thing\n")
	writeWorkflow(t, root, "release.yml", "jobs:\n  npm:\n    permissions:\n      id-token: write\n    steps:\n      - run: node scripts/used.mjs\n")
	declared := tables{declarations: map[string]declaration{
		"scripts/used.mjs npx":       {category: categoryHeldOffline, reason: "--offline"},
		"scripts/stale.mjs npx":      {category: categoryHeldOffline, reason: "--offline"},
		"scripts/used.mjs":           {category: categoryHeldOffline, reason: "--offline"},
		"scripts/used.mjs npm":       {category: categoryHeldOffline, reason: "--offline"},
		" npx":                       {category: categoryHeldOffline, reason: "--offline"},
		"scripts/blank.mjs latest":   {category: categoryHeldOffline, reason: "  "},
		"scripts/unknown.mjs latest": {category: "trust-me", reason: "it is fine"},
	}}

	problems, err := auditWorkflows(root, declared)
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	const label = "cmd/audit_supply_chain/declarations.go: "
	want := []string{
		label + `" npx" is not a path and a rule name (npx, latest, curl-pipe-shell, pip-unhashed) separated by one space`,
		label + `"scripts/blank.mjs latest" gives no reason`,
		label + `"scripts/blank.mjs latest" excuses nothing: no credentialed job reaches that file matching that rule`,
		label + `"scripts/stale.mjs npx" excuses nothing: no credentialed job reaches that file matching that rule`,
		label + `"scripts/unknown.mjs latest": the category "trust-me" is not one this command defines`,
		label + `"scripts/unknown.mjs latest" excuses nothing: no credentialed job reaches that file matching that rule`,
		label + `"scripts/used.mjs" is not a path and a rule name (npx, latest, curl-pipe-shell, pip-unhashed) separated by one space`,
		label + `"scripts/used.mjs npm" is not a path and a rule name (npx, latest, curl-pipe-shell, pip-unhashed) separated by one space`,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("auditWorkflows() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestScanMake_CommandPosition_DecidesTheMatch verifies which mentions of make
// are commands the audit follows.
//
// Only a make the shell runs is one: a prose mention inside an echo, a quoted
// suggestion to run a target by hand, and a word that merely starts with make
// would otherwise drag targets the job never runs into the rule. The shell runs
// a make behind variable assignments and the common wrappers as well as at the
// start of a command, and inside the string sh -c is handed, so each of those
// is followed: the rule used to stop at the start of a command, and
// `PYPI_BINARIES=dist make validate-pypi-local` ran the PyPI validators unread.
func TestScanMake_CommandPosition_DecidesTheMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "targets and assignments", text: "make validate-npm-local NPM_BINARIES=dist -j4 other", want: "[validate-npm-local other]"},
		{name: "a tab after make", text: "make\tvalidate", want: "[validate]"},
		{name: "after a separator", text: "cd site && make a; make b || make c | tee log", want: "[a] [b] [c]"},
		{name: "after a keyword", text: "if ! make a; then make b; fi\nwhile make c; do :; done", want: "[a] [b] [c]"},
		{name: "inside a group", text: "{ make a; } > log", want: "[a]"},
		{name: "a recipe's prefixes", text: "\t@$(MAKE) -C sub a\n\t-${MAKE} b\n\t+make c", want: "-C sub [a] [b] [c]"},
		{name: "a command substitution", text: "x=$(make a)\ny=`make b`", want: "[a] [b]"},
		{name: "a continued line", text: "make a \\\n  b", want: "[a b]"},
		{name: "no target", text: "make\nmake;", want: "[] []"},
		{name: "behind an assignment", text: "NPM_BINARIES=dist make publish", want: "[publish]"},
		{
			name: "behind quoted and substituted assignments",
			text: `A="x y" B='z;w' C=$(cat VERSION) D=a\ b make publish`, want: "[publish]",
		},
		{name: "behind env", text: "env FOO=1 make a; env -i -u HOME --unset=PATH - make b", want: "[a] [b]"},
		{name: "behind exec, command, nohup and time", text: "exec -c make a; command -p make b; nohup make c; time -p make d", want: "[a] [b] [c] [d]"},
		{name: "behind nice", text: "nice make a; nice -n 10 make b; nice -5 make c; nice --adjustment=2 make d", want: "[a] [b] [c] [d]"},
		{
			name: "behind timeout",
			text: "timeout 600 make a; timeout -s KILL --preserve-status -k 5s 10m make b; timeout --signal=TERM 1h make c",
			want: "[a] [b] [c]",
		},
		{name: "behind sudo", text: "sudo -E -u builder make a", want: "[a]"},
		{name: "behind stacked wrappers", text: "time env CI=1 nice make a", want: "[a]"},
		{name: "inside sh -c", text: `bash -c "make publish " && echo "done"` + "\nsh -o pipefail -ec 'make a b'", want: "[publish] [a b]"},
		{name: "inside sh -c with no target", text: `bash -c "make" && echo "done"`, want: "[]"},
		{name: "inside sh -c after a separator", text: `bash -c "cd site && make build"`, want: "[build]"},
		{name: "inside an sh -c string the line does not close", text: "bash -c \"make a\nb\"", want: "[a]"},
		{name: "$(MAKE) behind a shell function", text: "\trun_check \"[1/2] label\" $(MAKE) --no-print-directory check-a; \\", want: "[check-a]"},
		{name: "quoted advice", text: "echo \"Run 'make validate-npm-local' first\"", want: ""},
		{name: "prose", text: "echo built by GoReleaser and make mcpb", want: ""},
		{name: "a longer word", text: "cmake a\nmakefile b\nmake-thing c\nmake.log d\nmake=1 e\nmake: f\nmakea g", want: ""},
		{name: "command -v asks only", text: "command -v make >/dev/null; command -V make\ncommand", want: ""},
		{name: "a wrapper running something else", text: "env FOO=1 ./build.sh cmake amake makea makefile; echo built by make", want: ""},
		{name: "behind env -C", text: "env -C site make build", want: "unread:env"},
		{name: "behind a wrapper after a longer word", text: "env -C site makemake make build", want: "unread:env"},
		{name: "behind xargs and eval", text: "env\nprintf 'a\\n' | xargs make\neval make a", want: "unread:xargs unread:eval"},
		{name: "behind a timeout option it does not read", text: "timeout --signal KILL 60 make a", want: "unread:timeout"},
		{name: "inside sh -c behind a variable", text: `sh -c "$PREFIX make a"`, want: "unread:sh"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := renderMake(testCase.text); got != testCase.want {
				t.Errorf("scanMake(%q) = %s, want %s", testCase.text, got, testCase.want)
			}
		})
	}
}

// TestScanMake_Options_AreReadAsMakeReadsThem verifies that a make command's
// words are read as GNU make reads its arguments.
//
// An option's argument used to be read as a target, so `make -C site build`
// looked up site and build in the root Makefile, and `make -j 4 publish`
// reported a target named 4. -C changes the directory and -f names the
// makefile, so both are kept; every option that takes an argument takes it, in
// each spelling make accepts. What cannot be read safely is refused rather than
// guessed: an option this audit does not know (an abbreviated long option is
// one), an option missing its argument, --eval, and more than one makefile.
func TestScanMake_Options_AreReadAsMakeReadsThem(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "a directory", arguments: "-C sub a", want: "-C sub [a]"},
		{name: "a directory attached", arguments: "-Csub a", want: "-C sub [a]"},
		{name: "a directory in a cluster", arguments: "-sC sub a", want: "-C sub [a]"},
		{name: "a long directory", arguments: "--directory=sub a --directory other", want: "-C sub -C other [a]"},
		{name: "a makefile", arguments: "-f tools/release.mk a", want: "-f tools/release.mk [a]"},
		{name: "a makefile attached", arguments: "-ftools/release.mk a", want: "-f tools/release.mk [a]"},
		{name: "a long makefile", arguments: "--file=x.mk a", want: "-f x.mk [a]"},
		{name: "a makefile by its other long name", arguments: "--makefile x.mk a", want: "-f x.mk [a]"},
		{name: "a job count", arguments: "-j 4 a -j b --jobs 8 c -j8 d", want: "[a b c d]"},
		{name: "an empty job count", arguments: `-j "" a`, want: "[a]"},
		{name: "a load average", arguments: "-l 2.5 a -l .5 b -l c --max-load 3 d -l 0 e", want: "[a b c d e]"},
		{name: "a load average that is no number", arguments: `-l "" a`, want: "[ a]"},
		{name: "an option at the end", arguments: "a -j", want: "[a]"},
		{name: "flags", arguments: "-sk --no-print-directory --keep-going a", want: "[a]"},
		{name: "other arguments", arguments: "-I inc -o old --what-if=new -W new a", want: "[a]"},
		{name: "an attached argument only", arguments: "-Otarget a -O 5 b --debug=b c --debug d --output-sync e", want: "[a 5 b c d e]"},
		{name: "a flag before a number", arguments: "--keep-going 5 a", want: "[5 a]"},
		{name: "the end of the options", arguments: "-- -notanoption", want: "[-notanoption]"},
		{name: "assignments anywhere", arguments: "a X=1 -s b", want: "[a b]"},
		{name: "a lone dash", arguments: "-", want: "[-]"},
		{
			name: "an abbreviated long option", arguments: "--dir=site build",
			want: "(make passes --dir, an option this audit does not read) []",
		},
		{name: "an unknown flag", arguments: "a -x b", want: "(make passes -x, an option this audit does not read) [a]"},
		{name: "a missing argument", arguments: "a -C", want: "(make passes -C without the argument it takes) [a]"},
		{name: "a missing long argument", arguments: "--file", want: "(make passes --file without the argument it takes) []"},
		{
			name: "makefile text", arguments: "-E 'x: ; true' x",
			want: "(make passes -E, which adds makefile text this audit does not read) []",
		},
		{
			name: "long makefile text", arguments: "--eval=x",
			want: "(make passes --eval, which adds makefile text this audit does not read) []",
		},
		{
			name: "two makefiles", arguments: "-f a.mk -f b.mk t",
			want: "-f a.mk -f b.mk (make reads more than one makefile (a.mk, b.mk), which this audit does not merge) [t]",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			text := "make " + testCase.arguments
			if got := renderMake(text); got != testCase.want {
				t.Errorf("scanMake(%q) = %s, want %s", text, got, testCase.want)
			}
		})
	}
}

// TestCommandWords_ShellQuoting_SplitsAsTheShellDoes verifies that the words of
// a make command are split the way the shell splits them.
//
// Splitting at every space read `make VERSION="3 1" publish` as the targets 1"
// and publish, and cutting at every semicolon or parenthesis cut
// `make X=$(cat f) t` short of its target. A quote in the middle of a word is
// where the string the command was written inside closes, as in
// sh -c "cd site && make build", and ends it, as a comment does.
func TestCommandWords_ShellQuoting_SplitsAsTheShellDoes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		rest string
		want string
	}{
		{name: "quoted words", rest: ` a "b c" 'd;e' f`, want: "a|b c|d;e|f"},
		{name: "quoted values", rest: ` X="a b" Y='c' t`, want: "X=a b|Y=c|t"},
		{name: "an escaped quote", rest: ` "x\"y" b`, want: `x\"y|b`},
		{name: "a backslash inside single quotes", rest: ` 'x\' b`, want: `x\|b`},
		{name: "a substitution", rest: ` X=$(cat f) t`, want: "X=$(cat f)|t"},
		{name: "a nested substitution", rest: ` X=$(a $(b c)) t`, want: "X=$(a $(b c))|t"},
		{name: "a substitution after a parenthesis", rest: " a ($(b) c)", want: "a|($(b)|c"},
		{name: "an unterminated substitution", rest: " a $(b c", want: "a|$(b c"},
		{name: "a substitution cut by the line", rest: " a $(b\nc) d", want: "a|$(b"},
		{name: "an escaped space", rest: ` a\ b c`, want: "a b|c"},
		{name: "a trailing backslash", rest: ` a \`, want: `a|\`},
		{name: "a separator", rest: " a; b", want: "a"},
		{name: "a comment", rest: " a # b c", want: "a"},
		{name: "a hash inside a word", rest: " a#b c", want: "a#b|c"},
		{name: "a closing quote", rest: ` a" && echo "x`, want: "a"},
		{name: "a quote nothing closes", rest: ` a "b`, want: "a"},
		{name: "a quote closed on another line", rest: " a \"b\nc\"", want: "a"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := strings.Join(commandWords(testCase.rest), "|"); got != testCase.want {
				t.Errorf("commandWords(%q) = %q, want %q", testCase.rest, got, testCase.want)
			}
		})
	}
}

// TestParseMakefile_Rules_CarryTheirRecipes verifies the reading of a Makefile
// into rules: which lines start a rule, which belong to its recipe, and which
// end it.
//
// A conditional directive inside a recipe does not end it, since GNU make
// reads it as part of the rule; a blank line or a comment does not either. A
// variable assignment is not a rule, however much it looks like one, a line
// continued from a recipe line stays in the recipe, and a target two rules
// name carries the prerequisites and recipe lines of both. A rule line
// continued with a backslash carries the prerequisites on the lines it runs
// on to, which used to be read as recipe text and never followed, and a
// recipe written after a semicolon on the rule line, or a rule written with no
// space after its colon, used to be no recipe at all.
func TestParseMakefile_Rules_CarryTheirRecipes(t *testing.T) {
	t.Parallel()

	rules := parseMakefile(`VERSION := 3.1.0
export GOTOOLCHAIN := local
CACHE ::= x
POSIX :::= y
.PHONY: build push
## build: compile
build push: deps go.mod

	go build ./...
# a comment between recipe lines
ifndef REGISTRY
	$(error REGISTRY is required)
endif
	docker push \
  $(IMAGE)
deps::
	go mod download
LDFLAGS = -s \
	-w
	echo after an assignment
build: extra
	go vet ./...
multi: a \
  b
	npm ci
inline: c ; npx --yes thing
nospace:d
	echo nospace
last: e \`)

	cases := []struct {
		target        string
		prerequisites string
		recipe        string
	}{
		{
			target: "build", prerequisites: "deps go.mod extra",
			recipe: "\tgo build ./...|\t$(error REGISTRY is required)|\tdocker push \\|  $(IMAGE)|\tgo vet ./...",
		},
		{
			target: "push", prerequisites: "deps go.mod",
			recipe: "\tgo build ./...|\t$(error REGISTRY is required)|\tdocker push \\|  $(IMAGE)",
		},
		{target: "deps", recipe: "\tgo mod download"},
		{target: ".PHONY", prerequisites: "build push"},
		{target: "multi", prerequisites: "a b", recipe: "\tnpm ci"},
		{target: "inline", prerequisites: "c", recipe: " npx --yes thing"},
		{target: "nospace", prerequisites: "d", recipe: "\techo nospace"},
		{target: "last", prerequisites: "e \\"},
	}

	for _, testCase := range cases {
		t.Run(testCase.target, func(t *testing.T) {
			t.Parallel()

			rule := rules[testCase.target]
			if rule == nil {
				t.Fatalf("parseMakefile() has no rule %q (rules %v)", testCase.target, rules)
			}
			if got := strings.Join(rule.prerequisites, " "); got != testCase.prerequisites {
				t.Errorf("prerequisites = %q, want %q", got, testCase.prerequisites)
			}
			if got := strings.Join(rule.recipe, "|"); got != testCase.recipe {
				t.Errorf("recipe = %q, want %q", got, testCase.recipe)
			}
		})
	}

	for _, notARule := range []string{"VERSION", "export GOTOOLCHAIN", "GOTOOLCHAIN", "CACHE", "POSIX", "LDFLAGS", "b"} {
		t.Run("not a rule: "+notARule, func(t *testing.T) {
			t.Parallel()

			if rule, ok := rules[notARule]; ok {
				t.Errorf("parseMakefile() read %q as a rule: %+v", notARule, rule)
			}
		})
	}
}

// TestJobPermissions_WriteAll_GrantsEveryCredential verifies that the write-all
// shorthand grants each of the three permissions the rule counts.
func TestJobPermissions_WriteAll_GrantsEveryCredential(t *testing.T) {
	t.Parallel()

	permissions := jobPermissions(map[string]any{"permissions": "write-all"}, map[string]any{})
	for _, name := range []string{"contents", "id-token", "packages"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if permissions[name] != "write" {
				t.Errorf("jobPermissions(write-all)[%q] = %v, want write", name, permissions[name])
			}
		})
	}
}

// TestCheckDependabot_CooldownEcosystem_MustStateItsWindow verifies that every
// cooldown-capable ecosystem states its own window.
//
// An absent cooldown key means 'whatever GitHub defaults to today', which is
// not a property this repository controls. SemVer sub-keys on the docker
// ecosystems are rejected by Dependabot, and a rejected configuration stops
// that ecosystem's updates entirely.
func TestCheckDependabot_CooldownEcosystem_MustStateItsWindow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		entry       map[string]any
		name        string
		wantProblem bool
	}{
		{
			name:        "gomod without cooldown",
			entry:       map[string]any{"package-ecosystem": "gomod", "directory": "/"},
			wantProblem: true,
		},
		{
			name: "gomod with 7 days",
			entry: map[string]any{
				"package-ecosystem": "gomod", "directory": "/",
				"cooldown": map[string]any{"default-days": 7},
			},
			wantProblem: false,
		},
		{
			name: "gomod at the 3 day floor",
			entry: map[string]any{
				"package-ecosystem": "gomod", "directory": "/",
				"cooldown": map[string]any{"default-days": 3},
			},
			wantProblem: false,
		},
		{
			name: "gomod with 1 day",
			entry: map[string]any{
				"package-ecosystem": "gomod", "directory": "/",
				"cooldown": map[string]any{"default-days": 1},
			},
			wantProblem: true,
		},
		{
			name: "docker with semver keys",
			entry: map[string]any{
				"package-ecosystem": "docker", "directory": "/",
				"cooldown": map[string]any{"default-days": 7, "semver-major-days": 30},
			},
			wantProblem: true,
		},
		{
			name: "docker plain",
			entry: map[string]any{
				"package-ecosystem": "docker", "directory": "/",
				"cooldown": map[string]any{"default-days": 7},
			},
			wantProblem: false,
		},
		{
			name:        "docker-compose exempt",
			entry:       map[string]any{"package-ecosystem": "docker-compose", "directory": "/"},
			wantProblem: false,
		},
		{
			name: "cooldown is not a mapping",
			entry: map[string]any{
				"package-ecosystem": "npm", "directory": "/npm",
				"cooldown": 7,
			},
			wantProblem: true,
		},
		{
			name: "default-days is a string",
			entry: map[string]any{
				"package-ecosystem": "github-actions", "directory": "/",
				"cooldown": map[string]any{"default-days": "7"},
			},
			wantProblem: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			problems := checkDependabot(map[string]any{"updates": []any{testCase.entry}})
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkDependabot() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestCheckDependabot_Findings_ReadLikePython verifies the exact wording of the
// two dependabot findings, including the Python-style rendering of the
// offending value and of the rejected key list.
func TestCheckDependabot_Findings_ReadLikePython(t *testing.T) {
	t.Parallel()

	cases := []struct {
		entry map[string]any
		name  string
		want  []string
	}{
		{
			name:  "no cooldown at all",
			entry: map[string]any{"package-ecosystem": "gomod", "directory": "/"},
			want: []string{
				".github/dependabot.yml: gomod (/): no cooldown — the release window is whatever GitHub defaults to today",
			},
		},
		{
			name: "window too short",
			entry: map[string]any{
				"package-ecosystem": "npm", "directory": "/npm",
				"cooldown": map[string]any{"default-days": 1},
			},
			want: []string{
				".github/dependabot.yml: npm (/npm): cooldown.default-days is 1, want an integer >= 3",
			},
		},
		{
			name: "missing window renders as None",
			entry: map[string]any{
				"package-ecosystem": "gomod", "directory": "/",
				"cooldown": map[string]any{"semver-major-days": 30},
			},
			want: []string{
				".github/dependabot.yml: gomod (/): cooldown.default-days is None, want an integer >= 3",
			},
		},
		{
			name: "semver keys on docker",
			entry: map[string]any{
				"package-ecosystem": "docker", "directory": "/",
				"cooldown": map[string]any{"default-days": 7, "semver-major-days": 30, "semver-minor-days": 7},
			},
			want: []string{
				".github/dependabot.yml: docker (/): cooldown carries SemVer keys " +
					"['semver-major-days', 'semver-minor-days'] — Dependabot rejects them for this " +
					"ecosystem and a rejected configuration stops its updates entirely",
			},
		},
		{
			name:  "directory omitted renders as a question mark",
			entry: map[string]any{"package-ecosystem": "gomod"},
			want: []string{
				".github/dependabot.yml: gomod (?): no cooldown — the release window is whatever GitHub defaults to today",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := checkDependabot(map[string]any{"updates": []any{testCase.entry}})
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkDependabot() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

// TestSemVerCooldownKeys_Ecosystem_DecidesTheRule verifies that the SemVer rule
// belongs to the two docker ecosystems and to no other.
//
// Only `docker` reaches it through the audit today, because docker-compose is
// deliberately left out of the cooldown list entirely. The rule still names
// both, since Dependabot rejects those keys for each of them, and the day a
// docker-compose entry states a cooldown the rule has to be right already
// rather than be discovered wrong by a stopped update stream.
func TestSemVerCooldownKeys_Ecosystem_DecidesTheRule(t *testing.T) {
	t.Parallel()

	rejected := map[string]any{"default-days": 7, "semver-minor-days": 7, "semver-major-days": 30}

	cases := []struct {
		cooldown  map[string]any
		name      string
		ecosystem string
		want      []string
	}{
		{
			name:      "docker reports its semver keys, sorted",
			ecosystem: "docker", cooldown: rejected,
			want: []string{"semver-major-days", "semver-minor-days"},
		},
		{
			name:      "docker-compose reports them too",
			ecosystem: "docker-compose", cooldown: rejected,
			want: []string{"semver-major-days", "semver-minor-days"},
		},
		{
			name:      "docker with only the shared key reports nothing",
			ecosystem: "docker", cooldown: map[string]any{"default-days": 7},
			want: nil,
		},
		{name: "gomod is not judged", ecosystem: "gomod", cooldown: rejected, want: nil},
		{name: "npm is not judged", ecosystem: "npm", cooldown: rejected, want: nil},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := semVerCooldownKeys(testCase.ecosystem, testCase.cooldown)
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Errorf("semVerCooldownKeys(%q) = %v, want %v", testCase.ecosystem, got, testCase.want)
			}
		})
	}
}

// TestCheckSecurityPolicy_SupportedTable_TracksVersion verifies that
// SECURITY.md's supported-versions table tracks VERSION.
//
// The self-updater removal edited the paragraph beside the table and stepped
// over the table itself, leaving 1.x advertised as the supported line while the
// repository shipped 2.7.5 — a reporter could not tell whether 2.x was
// receiving fixes at all.
func TestCheckSecurityPolicy_SupportedTable_TracksVersion(t *testing.T) {
	t.Parallel()

	stale := "| Version              | Supported          |\n" +
		"| -------------------- | ------------------ |\n" +
		"| Latest `1.x` release | :white_check_mark: |\n" +
		"| Older `1.x` releases | :x:                |\n"
	current := "| Version              | Supported          |\n" +
		"| -------------------- | ------------------ |\n" +
		"| Latest `2.x` release | :white_check_mark: |\n" +
		"| Older `2.x` releases | :x:                |\n" +
		"| `1.x` and `0.x`      | :x:                |\n"

	cases := []struct {
		name        string
		version     string
		table       string
		wantProblem bool
	}{
		{name: "stale 1.x table on a 2.x release", version: "2.7.5\n", table: stale, wantProblem: true},
		{name: "current 2.x table on a 2.x release", version: "2.7.5\n", table: current, wantProblem: false},
		{name: "same table once 1.x ships", version: "1.9.0\n", table: stale, wantProblem: false},
		{name: "no table at all", version: "2.7.5\n", table: "Report privately.\n", wantProblem: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			problems := checkSecurityPolicy(testCase.version, testCase.table)
			if gotProblem := len(problems) > 0; gotProblem != testCase.wantProblem {
				t.Errorf("checkSecurityPolicy() problem = %t, want %t (%v)", gotProblem, testCase.wantProblem, problems)
			}
		})
	}
}

// TestCheckSecurityPolicy_Findings_NameTheDrift verifies the exact wording of
// the two security-policy findings, so a report tells a maintainer both what
// the table says and what VERSION says.
func TestCheckSecurityPolicy_Findings_NameTheDrift(t *testing.T) {
	t.Parallel()

	stale := "| Version              | Supported          |\n" +
		"| Latest `1.x` release | :white_check_mark: |\n"
	got := checkSecurityPolicy("2.7.5\n", stale)
	want := []string{
		"SECURITY.md: the supported-versions table never names `2.x`, but VERSION says 2.7.5",
		"SECURITY.md: `1.x` is still marked supported while the shipping major is 2",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkSecurityPolicy() = %#v, want %#v", got, want)
	}

	if missing := checkSecurityPolicy("2.7.5\n", "Report privately.\n"); len(missing) != 1 ||
		missing[0] != "SECURITY.md: no supported-versions table found" {
		t.Errorf("checkSecurityPolicy() with no table = %#v, want the no-table finding", missing)
	}
}

// TestCheckInstallers_Signature_MustBeVerified verifies that both installers
// reach for the release's Sigstore bundle.
//
// checksums.txt is fetched from the same mutable release as the binary, so a
// principal who can clobber release assets replaces both consistently and the
// hash comparison passes. Every release already publishes
// checksums.txt.sigstore.json; the installers used to ignore it.
//
// Each finding is asserted whole rather than counted, because the two
// installers carry the same two messages and differ only in the name inside
// them: a rule handed the bodies the other way round would count right and read
// wrong, sending a maintainer to edit the script that was already correct.
func TestCheckInstallers_Signature_MustBeVerified(t *testing.T) {
	t.Parallel()

	without := "dl \"$base/checksums.txt\" \"$tmp/checksums.txt\"\n"
	with := without + "cosign verify-blob --bundle \"$tmp/checksums.txt.sigstore.json\" \"$tmp/checksums.txt\"\n"

	cases := []struct {
		name string
		sh   string
		ps1  string
		want []string
	}{
		{
			name: "neither verifies", sh: without, ps1: without,
			want: []string{
				noSignatureFinding("scripts/install.sh"), noBundleFinding("scripts/install.sh"),
				noSignatureFinding("scripts/install.ps1"), noBundleFinding("scripts/install.ps1"),
			},
		},
		{
			name: "only sh verifies", sh: with, ps1: without,
			want: []string{noSignatureFinding("scripts/install.ps1"), noBundleFinding("scripts/install.ps1")},
		},
		{
			name: "only ps1 verifies", sh: without, ps1: with,
			want: []string{noSignatureFinding("scripts/install.sh"), noBundleFinding("scripts/install.sh")},
		},
		{name: "both verify", sh: with, ps1: with, want: nil},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := checkInstallers(testCase.sh, testCase.ps1)
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkInstallers() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

// TestCheckInstallers_AlternativeTools_AreAccepted verifies that any of the
// three signature checks the installers may use satisfies the rule, so the
// gate constrains the property rather than one implementation of it.
func TestCheckInstallers_AlternativeTools_AreAccepted(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tool string
	}{
		{name: "cosign", tool: "cosign verify-blob"},
		{name: "gh attestation", tool: "gh attestation verify"},
		{name: "powershell helper", tool: "Invoke-Signature"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			body := testCase.tool + " checksums.txt.sigstore.json\n"
			if got := checkInstallers(body, body); len(got) != 0 {
				t.Errorf("checkInstallers() = %v, want no findings when %q is used", got, testCase.tool)
			}
		})
	}
}

// TestLoadWorkflows_Directory_ReadsTextAndDocument verifies that the loader
// returns each workflow's raw text and parsed document, skips what is not a
// workflow file, and records the document order of the jobs mapping.
//
// Two shapes are skipped for different reasons and both are planted here: a
// file whose name is not a workflow's, and a directory whose name is. Reusable
// workflow fragments are sometimes kept in one, and reading it as a file would
// fail the whole audit for a directory nobody asked it to audit.
func TestLoadWorkflows_Directory_ReadsTextAndDocument(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "b.yml", "jobs:\n  zulu:\n    steps: []\n  alpha:\n    steps: []\n")
	writeWorkflow(t, root, "a.yaml", "name: a\n")
	writeWorkflow(t, root, "notes.txt", "ignored\n")
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows", "fragments.yml"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	workflows, err := loadWorkflows(root)
	if err != nil {
		t.Fatalf("loadWorkflows: %v", err)
	}
	if len(workflows) != 2 {
		t.Fatalf("loadWorkflows() returned %d files, want 2 (the .txt and the directory must be skipped)", len(workflows))
	}
	if workflows[0].path != ".github/workflows/a.yaml" || workflows[1].path != ".github/workflows/b.yml" {
		t.Errorf("loadWorkflows() paths = %q, %q, want a.yaml then b.yml", workflows[0].path, workflows[1].path)
	}
	if got := workflows[1].jobOrder; len(got) != 2 || got[0] != "zulu" || got[1] != "alpha" {
		t.Errorf("jobOrder = %v, want [zulu alpha]", got)
	}
	if workflows[0].doc["name"] != "a" {
		t.Errorf("doc = %v, want the parsed mapping", workflows[0].doc)
	}
}

// TestLoadWorkflows_UnparseableFile_IsAnError verifies that a workflow the YAML
// parser rejects stops the audit instead of being silently skipped.
//
// Skipping it would turn a broken workflow into a clean report, which is the
// one outcome this gate must never produce.
func TestLoadWorkflows_UnparseableFile_IsAnError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "broken.yml", "jobs:\n  - a\n   b: [\n")
	if _, err := loadWorkflows(root); err == nil {
		t.Error("loadWorkflows() = nil error, want a parse failure")
	}
}

// TestLoadWorkflows_DuplicateKey_IsRefused verifies that a workflow defining
// the same key twice in one mapping stops the audit.
//
// This is the one place the port deliberately does not reproduce the Python
// auditor, and the direction matters. PyYAML accepts a duplicate key silently
// and keeps the last value, so a step written with two `run:` keys was audited
// as though only one of them existed — the discarded half could have been
// anything, and the audit would still have passed. go-yaml refuses the
// document instead, which fails the gate closed on a workflow whose meaning is
// ambiguous rather than auditing a guess about it.
func TestLoadWorkflows_DuplicateKey_IsRefused(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "dupe.yml",
		"jobs:\n  release:\n    steps:\n      - run: npx --yes thing\n        run: npm ci\n")
	if _, err := loadWorkflows(root); err == nil {
		t.Error("loadWorkflows() = nil error, want a refusal for a duplicated mapping key")
	}
}

// TestLoadWorkflows_NonMappingFile_KeepsPinningOnly verifies that a workflow
// that parses to something other than a mapping still has its text audited for
// pinning while the job rules stand down, matching the original's isinstance
// guard.
func TestLoadWorkflows_NonMappingFile_KeepsPinningOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "list.yml", "- uses: actions/checkout@v7\n")
	writeWorkflow(t, root, "empty.yml", "")

	workflows, err := loadWorkflows(root)
	if err != nil {
		t.Fatalf("loadWorkflows: %v", err)
	}
	for _, file := range workflows {
		if file.doc != nil {
			t.Errorf("%s: doc = %v, want nil for a non-mapping document", file.path, file.doc)
		}
	}
	if problems := checkPinnedUses("list.yml", workflows[1].text); len(problems) != 1 {
		t.Errorf("checkPinnedUses() = %v, want the unpinned reference to still be reported", problems)
	}
}

// TestLoadWorkflows_MissingDirectory_IsAnError verifies that a root with no
// .github/workflows fails loudly rather than reporting a clean audit.
func TestLoadWorkflows_MissingDirectory_IsAnError(t *testing.T) {
	t.Parallel()

	if _, err := loadWorkflows(t.TempDir()); err == nil {
		t.Error("loadWorkflows() = nil error, want a read failure for a missing directory")
	}
}

// TestLoadWorkflows_UnreadableEntry_IsAnError verifies that an entry the
// directory lists under a workflow's name but the loader cannot read stops the
// audit.
//
// os.ReadDir reports a dangling symlink as an ordinary entry, so the name rule
// admits it and the read is what fails. Passing over it would drop a workflow
// from the audit and still print the success line, which is the one outcome
// this gate must never produce.
func TestLoadWorkflows_UnreadableEntry_IsAnError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	directory := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "never-written.yml"), filepath.Join(directory, "dangling.yml")); err != nil {
		t.Skipf("this platform will not create a symlink: %v", err)
	}
	if _, err := loadWorkflows(root); err == nil {
		t.Error("loadWorkflows() = nil error, want a read failure for an entry that cannot be opened")
	}
}

// TestAuditWorkflows_NonMappingWorkflow_KeepsPinningOnly verifies that the
// audit's own loop applies the pinning rule to a workflow whose document is not
// a mapping and stands the job rules down for it.
//
// The loader hands such a file over with a nil document, and reading `jobs` off
// it would be a lookup on nothing; the uses: line inside it is still a line a
// later edit can turn into a step, so it is still audited.
func TestAuditWorkflows_NonMappingWorkflow_KeepsPinningOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkflow(t, root, "list.yml", "- uses: actions/checkout@v7\n")

	problems, err := auditWorkflows(root, tables{})
	if err != nil {
		t.Fatalf("auditWorkflows: %v", err)
	}
	want := ".github/workflows/list.yml:1: uses: actions/checkout@v7 is not pinned to a 40-character commit SHA"
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("auditWorkflows() = %v, want exactly [%q]", problems, want)
	}
}

// TestDocumentMapping_UnusualNode_ReturnsNoMapping verifies the unwrap that
// turns a parsed file into the mapping the jobs lookup walks.
//
// Every refusal here is what keeps that lookup from indexing into a node
// carrying nothing: no node at all, a document with nothing under it, and a
// file whose root is a scalar. A node that is already the mapping is returned
// as it stands, since the unwrap is the only step between the two.
func TestDocumentMapping_UnusualNode_ReturnsNoMapping(t *testing.T) {
	t.Parallel()

	scalar := &yaml.Node{Kind: yaml.ScalarNode, Value: "ci"}
	mapping := &yaml.Node{Kind: yaml.MappingNode}

	cases := []struct {
		node *yaml.Node
		want *yaml.Node
		name string
	}{
		{name: "no node at all", node: nil, want: nil},
		{name: "document with nothing under it", node: &yaml.Node{Kind: yaml.DocumentNode}, want: nil},
		{
			name: "document over a scalar",
			node: &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{scalar}},
			want: nil,
		},
		{name: "scalar with no document around it", node: scalar, want: nil},
		{name: "mapping with no document around it", node: mapping, want: mapping},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := documentMapping(testCase.node); got != testCase.want {
				t.Errorf("documentMapping() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestStripComments_WholeLineComment_IsDropped verifies that only whole-line
// comments are removed, so a rationale about npx is not read as npx while an
// inline `# comment` after real code leaves that code visible to the rules.
func TestStripComments_WholeLineComment_IsDropped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "leading comment", text: "# npx here\nnpm ci", want: "npm ci"},
		{name: "indented comment", text: "  # npx here\nnpm ci", want: "npm ci"},
		{name: "trailing inline comment stays", text: "npm ci # npx", want: "npm ci # npx"},
		{name: "no comments", text: "a\nb", want: "a\nb"},
		{name: "empty", text: "", want: ""},
		{name: "crlf line endings", text: "# npx\r\nnpm ci\r\n", want: "npm ci"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := stripComments(testCase.text); got != testCase.want {
				t.Errorf("stripComments(%q) = %q, want %q", testCase.text, got, testCase.want)
			}
		})
	}
}

// TestSplitLines_TrailingNewline_MatchesPython verifies the line splitting the
// whole auditor is built on.
//
// A trailing newline must not produce an extra empty line, or the pinning
// rule's line numbers and the comment stripper's round trip would both drift
// from what the Python auditor reported.
func TestSplitLines_TrailingNewline_MatchesPython(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "empty string", text: "", want: nil},
		{name: "single newline", text: "\n", want: []string{""}},
		{name: "no trailing newline", text: "a\nb", want: []string{"a", "b"}},
		{name: "trailing newline", text: "a\nb\n", want: []string{"a", "b"}},
		{name: "blank line before the end", text: "a\n\n", want: []string{"a", ""}},
		{name: "crlf", text: "a\r\nb\r\n", want: []string{"a", "b"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := splitLines(testCase.text)
			if strings.Join(got, "|") != strings.Join(testCase.want, "|") || len(got) != len(testCase.want) {
				t.Errorf("splitLines(%q) = %#v, want %#v", testCase.text, got, testCase.want)
			}
		})
	}
}

// TestPythonRepr_Value_MatchesPython verifies the rendering that keeps this
// program's findings byte-identical to the Python auditor's.
//
// Three findings embed repr() output; if the rendering drifts, a report from
// this program and a report from the one it replaced stop comparing, and the
// port can no longer be shown to be a port. The last case is the arm no YAML
// scalar reaches: a decoded document can also carry a sequence or a mapping,
// and the fallback has to render one rather than drop it.
func TestPythonRepr_Value_MatchesPython(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value any
		name  string
		want  string
	}{
		{name: "nil", value: nil, want: "None"},
		{name: "true", value: true, want: "True"},
		{name: "false", value: false, want: "False"},
		{name: "int", value: 1, want: "1"},
		{name: "int64", value: int64(7), want: "7"},
		{name: "uint64", value: uint64(7), want: "7"},
		{name: "float", value: 3.0, want: "3.0"},
		{name: "fractional float", value: 3.5, want: "3.5"},
		{name: "plain string", value: "~> v2", want: "'~> v2'"},
		{name: "empty string", value: "", want: "''"},
		{name: "string with an apostrophe", value: "it's", want: `"it's"`},
		{name: "string with both quotes", value: `it's "x"`, want: `'it\'s "x"'`},
		{name: "string with a backslash", value: `a\b`, want: `'a\\b'`},
		{name: "string with control characters", value: "a\nb\tc\rd", want: `'a\nb\tc\rd'`},
		{name: "a kind the switch does not name", value: []any{1, "a"}, want: "[1 a]"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := pythonRepr(testCase.value); got != testCase.want {
				t.Errorf("pythonRepr(%#v) = %s, want %s", testCase.value, got, testCase.want)
			}
		})
	}
}

// TestPythonStr_Value_MatchesPython verifies the str() rendering used when an
// env pin resolves to a non-string scalar, so a workflow that writes
// `GORELEASER_VERSION: 2` is reported with what it actually wrote.
func TestPythonStr_Value_MatchesPython(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value any
		name  string
		want  string
	}{
		{name: "nil", value: nil, want: "None"},
		{name: "string", value: "v2.18.0", want: "v2.18.0"},
		{name: "true", value: true, want: "True"},
		{name: "false", value: false, want: "False"},
		{name: "int", value: 2, want: "2"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := pythonStr(testCase.value); got != testCase.want {
				t.Errorf("pythonStr(%#v) = %q, want %q", testCase.value, got, testCase.want)
			}
		})
	}
}

// TestResolveEnv_Reference_SubstitutesOrStands verifies the one indirection a
// version pin may take, and that an undefined name is left standing so the
// exact-version rule rejects it rather than silently accepting an empty string.
func TestResolveEnv_Reference_SubstitutesOrStands(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value any
		doc   map[string]any
		name  string
		want  string
	}{
		{
			name:  "defined name",
			value: "${{ env.V }}",
			doc:   map[string]any{"env": map[string]any{"V": "v2.18.0"}},
			want:  "v2.18.0",
		},
		{
			name:  "undefined name stands",
			value: "${{ env.V }}",
			doc:   map[string]any{"env": map[string]any{}},
			want:  "${{ env.V }}",
		},
		{name: "no env block", value: "${{ env.V }}", doc: map[string]any{}, want: "${{ env.V }}"},
		{
			name:  "null value renders as None",
			value: "${{ env.V }}",
			doc:   map[string]any{"env": map[string]any{"V": nil}},
			want:  "None",
		},
		{name: "literal value passes through", value: "v2.18.0", doc: map[string]any{}, want: "v2.18.0"},
		{name: "absent value is empty", value: "", doc: map[string]any{}, want: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveEnv(testCase.value, testCase.doc); got != testCase.want {
				t.Errorf("resolveEnv(%#v) = %q, want %q", testCase.value, got, testCase.want)
			}
		})
	}
}

// TestRun_CleanRepository_PrintsTheSuccessLine verifies the command's success
// path end to end: exit code 0 and the one line the Python auditor printed,
// which is what a reader of a CI log compares against.
func TestRun_CleanRepository_PrintsTheSuccessLine(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", root}, &stdout, &stderr, repositoryTables()); code != 0 {
		t.Fatalf("run() = %d, want 0 (stdout %q, stderr %q)", code, stdout.String(), stderr.String())
	}
	want := "supply-chain audit passed: pinned actions, locked release jobs, " +
		"stated cooldowns, current security policy, signature-verifying installers\n"
	if stdout.String() != want {
		t.Errorf("run() stdout = %q, want %q", stdout.String(), want)
	}
}

// TestMain_Arguments_ReachRunAndItsStatusIsTheExit verifies that main hands
// run the arguments after the program name and this repository's own tables,
// and exits with the status run decided rather than one of its own.
//
// A main that dropped the arguments would audit the module root whatever
// --root said, and one that judged by empty tables would pass a workflow
// reading an undeclared secret. It swaps the process's arguments and streams,
// so it does not run in parallel.
func TestMain_Arguments_ReachRunAndItsStatusIsTheExit(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{name: "this repository", args: []string{"--root", repositoryRoot(t)}, wantCode: 0, wantStdout: "supply-chain audit passed"},
		{name: "a tree its tables do not describe", args: []string{"--root", brokenRepository(t)}, wantCode: 1, wantStdout: "secrets.go"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			previousArgs, previousStdout, previousExit := os.Args, os.Stdout, osExit
			t.Cleanup(func() { os.Args, os.Stdout, osExit = previousArgs, previousStdout, previousExit })

			capture, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			t.Cleanup(func() { _ = capture.Close() })
			os.Args = append([]string{"audit_supply_chain"}, testCase.args...)
			os.Stdout = capture
			code := -1
			osExit = func(status int) { code = status }

			main()

			written, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if code != testCase.wantCode || !strings.Contains(string(written), testCase.wantStdout) {
				t.Errorf("main() exited %d with %q, want %d mentioning %q", code, written, testCase.wantCode, testCase.wantStdout)
			}
		})
	}
}

// TestRun_Violations_ReportEveryFinding verifies the command's failure path:
// exit code 1, a header counting the findings, and one indented line per
// finding, in the same shape the Python auditor printed.
func TestRun_Violations_ReportEveryFinding(t *testing.T) {
	t.Parallel()

	root := brokenRepository(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", root}, &stdout, &stderr, tables{}); code != 1 {
		t.Fatalf("run() = %d, want 1 (stdout %q, stderr %q)", code, stdout.String(), stderr.String())
	}
	lines := splitLines(stdout.String())
	if len(lines) == 0 || lines[0] != "supply-chain audit FAILED (1 problems):" {
		t.Fatalf("run() stdout = %q, want a FAILED header counting one problem", stdout.String())
	}
	want := "  x .github/workflows/ci.yml:5: uses: actions/checkout@v7 is not pinned to a 40-character commit SHA"
	if len(lines) != 2 || lines[1] != want {
		t.Errorf("run() stdout = %q, want the single finding %q", stdout.String(), want)
	}
}

// TestRun_UnreadableRoot_ExitsNonZero verifies that an audit that cannot be
// performed is reported on stderr and exits non-zero.
//
// A gate that cannot read its inputs must never look like a gate that passed,
// which is the failure mode a silently skipped directory would create.
func TestRun_UnreadableRoot_ExitsNonZero(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", t.TempDir()}, &stdout, &stderr, repositoryTables()); code != 1 {
		t.Errorf("run() = %d, want 1 for a root with no workflows", code)
	}
	if !strings.Contains(stderr.String(), "audit_supply_chain:") {
		t.Errorf("run() stderr = %q, want the failure named on stderr", stderr.String())
	}
}

// TestRun_BadFlag_ExitsNonZero verifies that an unknown flag is refused rather
// than ignored, and that -h is not itself a failure.
func TestRun_BadFlag_ExitsNonZero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "unknown flag", args: []string{"--nope"}, want: 1},
		{name: "help", args: []string{"-h"}, want: 0},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := run(testCase.args, &stdout, &stderr, repositoryTables()); code != testCase.want {
				t.Errorf("run(%v) = %d, want %d", testCase.args, code, testCase.want)
			}
		})
	}
}

// TestPythonInt_Value_AppliesPythonsBoolRule verifies the integer test the
// cooldown rule uses.
//
// Python's isinstance(days, int) is true for a bool, so `default-days: true`
// was measured as 1 and reported as too short rather than being waved through
// as a non-integer. The port keeps that, because a YAML file that writes a
// boolean where a day count belongs is misconfigured either way.
func TestPythonInt_Value_AppliesPythonsBoolRule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value    any
		name     string
		wantDays int
		wantOK   bool
	}{
		{name: "int", value: 7, wantDays: 7, wantOK: true},
		{name: "int64", value: int64(7), wantDays: 7, wantOK: true},
		{name: "uint64", value: uint64(7), wantDays: 7, wantOK: true},
		{name: "true counts as one", value: true, wantDays: 1, wantOK: true},
		{name: "false counts as zero", value: false, wantDays: 0, wantOK: true},
		{name: "string is not an integer", value: "7", wantOK: false},
		{name: "float is not an integer", value: 7.0, wantOK: false},
		{name: "nil is not an integer", value: nil, wantOK: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			days, ok := pythonInt(testCase.value)
			if ok != testCase.wantOK || (ok && days != testCase.wantDays) {
				t.Errorf("pythonInt(%#v) = (%d, %t), want (%d, %t)", testCase.value, days, ok, testCase.wantDays, testCase.wantOK)
			}
		})
	}
}

// TestCheckDependabot_MalformedDocument_IsIgnored verifies that a document
// whose updates list is missing, or holds something other than entries,
// produces no findings rather than a crash.
//
// The schema gate for dependabot.yml is Dependabot's own; this auditor only
// adds the cooldown property on top of it, and inventing findings about a
// shape it does not own would be noise.
func TestCheckDependabot_MalformedDocument_IsIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		doc  map[string]any
		name string
	}{
		{name: "no updates key", doc: map[string]any{"version": 2}},
		{name: "updates is not a list", doc: map[string]any{"updates": "gomod"}},
		{name: "entry is not a mapping", doc: map[string]any{"updates": []any{"gomod"}}},
		{name: "ecosystem is not a string", doc: map[string]any{"updates": []any{map[string]any{"package-ecosystem": 7}}}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if problems := checkDependabot(testCase.doc); len(problems) != 0 {
				t.Errorf("checkDependabot() = %v, want no findings", problems)
			}
		})
	}
}

// TestCheckCredentialedJob_MalformedJob_IsIgnored verifies that a job whose
// steps are missing or are not mappings is skipped rather than crashing the
// audit, so one hand-edited workflow cannot take the whole gate down.
func TestCheckCredentialedJob_MalformedJob_IsIgnored(t *testing.T) {
	t.Parallel()

	credentials := map[string]any{"id-token": "write"}
	cases := []struct {
		job  map[string]any
		name string
	}{
		{name: "no steps key", job: map[string]any{"permissions": credentials}},
		{name: "steps is not a list", job: map[string]any{"permissions": credentials, "steps": "build"}},
		{name: "step is not a mapping", job: map[string]any{"permissions": credentials, "steps": []any{"build"}}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if problems := newAudit(".", tables{}).checkCredentialedJob("wf.yml", "release", testCase.job, nil); len(problems) != 0 {
				t.Errorf("checkCredentialedJob() = %v, want no findings", problems)
			}
		})
	}
}

// TestAudit_MissingInput_IsAnError verifies that every file the audit reads is
// required.
//
// Each of these carries one of the five invariants, so a missing one is a
// property that stopped being checked. Reporting it as an error rather than as
// a pass is the difference between a gate and a decoration.
func TestAudit_MissingInput_IsAnError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		removed string
	}{
		{name: "dependabot config", removed: ".github/dependabot.yml"},
		{name: "version file", removed: "VERSION"},
		{name: "security policy", removed: "SECURITY.md"},
		{name: "shell installer", removed: "scripts/install.sh"},
		{name: "powershell installer", removed: "scripts/install.ps1"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := brokenRepository(t)
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(testCase.removed))); err != nil {
				t.Fatalf("Remove %s: %v", testCase.removed, err)
			}
			if _, err := audit(root, tables{}); err == nil {
				t.Errorf("audit() = nil error, want a failure when %s is missing", testCase.removed)
			}
		})
	}
}

// TestAudit_UnparseableDependabot_IsAnError verifies that a dependabot.yml
// which is valid YAML but not a mapping stops the audit, matching the original,
// where the same shape raised on its first .get call.
func TestAudit_UnparseableDependabot_IsAnError(t *testing.T) {
	t.Parallel()

	root := brokenRepository(t)
	path := filepath.Join(root, ".github", "dependabot.yml")
	if err := os.WriteFile(path, []byte("- gomod\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := audit(root, tables{}); err == nil {
		t.Error("audit() = nil error, want a failure for a dependabot.yml that is not a mapping")
	}
}

// TestAudit_UnverifiedInstaller_NamesTheOneThatFailed verifies that the audit
// hands each installer's own text to the installer rule.
//
// The two are read one after the other into two strings of the same type and
// passed positionally, so a swap changes nothing about how many findings come
// back, only which file they accuse. The fixture therefore makes them differ
// and the assertion names the file, rather than counting.
func TestAudit_UnverifiedInstaller_NamesTheOneThatFailed(t *testing.T) {
	t.Parallel()

	root := brokenRepository(t)
	if err := os.WriteFile(filepath.Join(root, "scripts", "install.ps1"),
		[]byte("dl \"$base/checksums.txt\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	problems, err := audit(root, tables{})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	want := []string{
		".github/workflows/ci.yml:5: uses: actions/checkout@v7 is not pinned to a 40-character commit SHA",
		noSignatureFinding("scripts/install.ps1"),
		noBundleFinding("scripts/install.ps1"),
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit() = %#v, want %#v", problems, want)
	}
}

// TestRun_WorkingDirectoryOutsideTheModule_ExitsNonZero verifies that the
// command refuses when it cannot work out which repository to audit.
//
// With no --root the root is the module root at or above the working
// directory, and a working directory outside any module leaves the audit
// nothing to read. It changes the working directory, so it does not run in
// parallel.
func TestRun_WorkingDirectoryOutsideTheModule_ExitsNonZero(t *testing.T) {
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr, repositoryTables()); code != 1 {
		t.Errorf("run() = %d, want 1 outside any module (stdout %q)", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "go.mod not found") {
		t.Errorf("run() stderr = %q, want the missing module named on stderr", stderr.String())
	}
}

// TestResolveRoot_NoFlag_FindsTheModuleRoot verifies that the command run with
// no --root audits the repository it lives in, which is how every make target
// and CI step invokes it.
func TestResolveRoot_NoFlag_FindsTheModuleRoot(t *testing.T) {
	t.Parallel()

	got, err := resolveRoot("")
	if err != nil {
		t.Fatalf("resolveRoot: %v", err)
	}
	if got != repositoryRoot(t) {
		t.Errorf("resolveRoot(\"\") = %q, want the module root %q", got, repositoryRoot(t))
	}
}

// TestJobsKeyOrder_UnusualDocument_ReturnsNoOrder verifies that the document
// order lookup degrades to nothing when a workflow has no jobs mapping, so the
// sorted fallback takes over instead of producing a partial order.
func TestJobsKeyOrder_UnusualDocument_ReturnsNoOrder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
	}{
		{name: "no jobs key", text: "name: ci\n"},
		{name: "jobs is a list", text: "jobs:\n  - build\n"},
		{name: "document is a scalar", text: "ci\n"},
		{name: "document is empty", text: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, order, err := parseWorkflow(testCase.text)
			if err != nil {
				t.Fatalf("parseWorkflow: %v", err)
			}
			if len(order) != 0 {
				t.Errorf("parseWorkflow() job order = %v, want none", order)
			}
		})
	}
}

// TestJobsKeyOrder_UnpairedEntry_ReadsWholePairsOnly verifies that the walk over
// a mapping's content reads keys and values in pairs and stops before an
// entry with no partner.
//
// A parsed document never holds one, but the walk indexes the value after each
// key, so the bound is what keeps a hand-built tree from reading past the end
// of the slice or naming its unpaired last entry as a job.
func TestJobsKeyOrder_UnpairedEntry_ReadsWholePairsOnly(t *testing.T) {
	t.Parallel()

	scalar := func(value string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: value} }
	jobs := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("build"), {Kind: yaml.MappingNode}, scalar("dangling")}}

	cases := []struct {
		root *yaml.Node
		name string
		want string
	}{
		{name: "a jobs key with no value", root: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("jobs")}}, want: ""},
		{name: "a job key with no value", root: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("jobs"), jobs}}, want: "build"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := strings.Join(jobsKeyOrder(testCase.root), ","); got != testCase.want {
				t.Errorf("jobsKeyOrder() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestAudit_Repository_IsClean verifies the audit passes on the repository as
// committed.
//
// This is the gate itself: it fails the build the moment an action is unpinned,
// a release job gains an unlocked download, a cooldown is dropped, the security
// policy goes stale, or an installer stops checking signatures.
func TestAudit_Repository_IsClean(t *testing.T) {
	t.Parallel()

	problems, err := audit(repositoryRoot(t), repositoryTables())
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("audit() found %d problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// noSignatureFinding renders the finding an installer that checks no signature
// carries, for the installer named.
func noSignatureFinding(installer string) string {
	return installer + ": verifies no signature; checksums.txt comes from the same mutable release " +
		"as the binary, so a consistent replacement of both files is accepted"
}

// noBundleFinding renders the finding an installer that never fetches the
// Sigstore bundle carries, for the installer named.
func noBundleFinding(installer string) string {
	return installer + ": never fetches checksums.txt.sigstore.json, which every release publishes"
}

// The pattern and reason halves of the run-time-code findings the fixtures
// trigger, as every finding prints them after "matches ".
const (
	npxFinding    = `'\\bnpx\\b': npx resolves a dependency tree at run time (use a lockfile and npm ci, or drop the CLI)`
	latestFinding = `'@latest\\b': @latest is whatever the registry serves at that moment`
	curlFinding   = `'curl[^\\n|]*\\|\\s*(?:ba)?sh\\b': piping a download into a shell runs unreviewed code`
)

// renderMake renders what scanMake reads in a text: each invocation as its -C
// directories, its -f makefiles, its refusal in parentheses and its targets in
// brackets, then each wrapper whose make it could not read.
func renderMake(text string) string {
	invocations, unread := scanMake(text)
	var rendered []string
	for _, invocation := range invocations {
		var parts []string
		for _, directory := range invocation.directories {
			parts = append(parts, "-C "+directory)
		}
		for _, makefile := range invocation.makefiles {
			parts = append(parts, "-f "+makefile)
		}
		if invocation.refusal != "" {
			parts = append(parts, "("+invocation.refusal+")")
		}
		rendered = append(rendered, strings.Join(append(parts, "["+strings.Join(invocation.targets, " ")+"]"), " "))
	}
	for _, wrapper := range unread {
		rendered = append(rendered, "unread:"+wrapper)
	}
	return strings.Join(rendered, " ")
}

// unresolvedFinding renders the finding a reference the audit cannot read
// carries, for the place that wrote it, the spelling it wrote and the path it
// was read as.
func unresolvedFinding(origin, written, resolved string) string {
	return origin + ": " + written + " resolves to " + resolved +
		", which is not a readable file in this repository, so what it runs cannot be judged"
}

// rootFinding renders the finding a script rooted in a value only the run knows
// carries, for the place that wrote it, the spelling it wrote and its root.
func rootFinding(origin, written, root string) string {
	return origin + ": " + written + " is rooted in " + root +
		", which this audit cannot resolve, so what it runs cannot be judged"
}

// writeFile writes one file into a fixture root, creating its directories.
func writeFile(t *testing.T, root, relative, body string) {
	t.Helper()

	full := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("MkdirAll %s: %v", relative, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", relative, err)
	}
}

// repositoryRoot returns this repository's root, so the gate runs against the
// tree as committed rather than a fixture.
func repositoryRoot(t *testing.T) string {
	t.Helper()

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("RepositoryRoot: %v", err)
	}
	return root
}

// writeWorkflow writes one file into a fixture root's .github/workflows.
func writeWorkflow(t *testing.T, root, name, body string) {
	t.Helper()

	directory := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
}

// brokenRepository builds the smallest tree the audit can read end to end,
// carrying exactly one violation: an unpinned action reference.
func brokenRepository(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeWorkflow(t, root, "ci.yml", "name: CI\njobs:\n  test:\n    steps:\n      - uses: actions/checkout@v7\n")

	files := map[string]string{
		".github/dependabot.yml": "version: 2\nupdates:\n  - package-ecosystem: gomod\n    directory: /\n    cooldown:\n      default-days: 7\n",
		"VERSION":                "2.7.5\n",
		"SECURITY.md":            "| Version | Supported |\n| Latest `2.x` release | :white_check_mark: |\n",
		"scripts/install.sh":     "cosign verify-blob --bundle checksums.txt.sigstore.json\n",
		"scripts/install.ps1":    "cosign verify-blob --bundle checksums.txt.sigstore.json\n",
	}
	for relative, body := range files {
		full := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("MkdirAll %s: %v", relative, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", relative, err)
		}
	}
	return root
}
