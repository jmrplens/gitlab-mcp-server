// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// reproduce_test.go drives a release's reproduction end to end: a fixture
// repository tagged as a release, its binaries built the way the release
// builds them and published beside a checksum file, and this command
// cloning the tag, rebuilding each one and comparing.
package main

import (
	"bytes"
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// fixtureReleaseConfig is the configuration of a fixture release built for
// each goos crossed with each goarch (comma-separated lists), stamping the
// version and the full commit, with extra appended.
func fixtureReleaseConfig(goos, goarch, extra string) string {
	return "builds:\n  - id: fixture\n    main: .\n    binary: fixture\n    env: [CGO_ENABLED=0]\n    flags: [-trimpath]\n" +
		"    goos: [" + goos + "]\n    goarch: [" + goarch + "]\n" +
		"    ldflags:\n      - -s -w -X main.version={{.Version}} -X main.commit={{.FullCommit}}\n" +
		"archives:\n  - formats: [binary]\n    name_template: \"{{ .Binary }}-{{ .Os }}-{{ .Arch }}\"\n" +
		"checksum:\n  name_template: checksums.txt\n" + extra
}

// hostConfig is the fixture release built for the host alone.
func hostConfig(extra string) string {
	return fixtureReleaseConfig(hostTarget.goos, hostTarget.goarch, extra)
}

// releaseRepo is a fixture repository holding the module, files and the
// configuration in one commit tagged v1.2.3, which it returns.
func releaseRepo(t *testing.T, config string, files map[string]string) (repo, commit string) {
	t.Helper()

	all := maps.Clone(fixtureModule)
	maps.Copy(all, files)
	all[".goreleaser.yml"] = config
	return fixtureRepo(t, all)
}

// hostAsset is the release name of the fixture's binary for the host.
func hostAsset() string {
	return "fixture-" + hostTarget.goos + "-" + hostTarget.goarch + hostTarget.ext()
}

// publishBinary builds the fixture's main package in repo for tgt the way
// the release builds it, stamped with version and commit, and returns the
// binary. It runs the go command itself rather than through this command, so
// a reproduction is compared with a build this command did not make.
func publishBinary(t *testing.T, repo string, tgt target, version, commit string) []byte {
	t.Helper()

	out := filepath.Join(t.TempDir(), "fixture"+tgt.ext())
	ldflags := "-ldflags=-s -w -X main.version=" + version + " -X main.commit=" + commit
	// #nosec G204 G702 -- golist.Executable is the toolchain's own go binary, building the test's fixture
	cmd := exec.CommandContext(context.Background(), golist.Executable(), "build", "-trimpath", ldflags, "-o", out, ".")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0", "GOOS="+tgt.goos, "GOARCH="+tgt.goarch)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the published fixture for %s: %v\n%s", tgt, err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// publishRelease writes files as a release's published assets, beside a
// checksums.txt listing each of them, and returns the directory.
func publishRelease(t *testing.T, files map[string][]byte) string {
	t.Helper()

	dir := t.TempDir()
	var list strings.Builder
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if err := os.WriteFile(filepath.Join(dir, name), files[name], 0o600); err != nil {
			t.Fatal(err)
		}
		list.WriteString(releaseassets.Digest(files[name]) + "  " + name + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(list.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runRelease runs the command against a published directory and returns the
// exit code and both streams.
func runRelease(o options) (code int, stdout, stderr string) {
	var out, errs bytes.Buffer
	code = run(context.Background(), o, &out, &errs)
	return code, out.String(), errs.String()
}

// TestRun_RebuildsAReleaseBitForBit is the command doing its job: the
// release's binary rebuilt from a clone of its tag is the published one byte
// for byte, the file the checksum list carries that no build makes is named
// as not rebuilt, and the rebuilt binary is kept where -out says, at
// GoReleaser's own path for it.
func TestRun_RebuildsAReleaseBitForBit(t *testing.T) {
	t.Parallel()

	repo, commit := releaseRepo(t, hostConfig(""), nil)
	published := publishRelease(t, map[string][]byte{
		hostAsset():           publishBinary(t, repo, hostTarget, "1.2.3", commit),
		"THIRD_PARTY_NOTICES": []byte("notices\n"),
	})
	out := t.TempDir()
	code, stdout, stderr := runRelease(options{dir: repo, config: ".goreleaser.yml", published: published, tag: "v1.2.3", ref: "v1.2.3", out: out})
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"rebuilt v1.2.3 from " + commit,
		"reproduced " + hostAsset() + " (" + hostTarget.String() + " with go",
		"not rebuilt, since the configuration builds none of them: THIRD_PARTY_NOTICES",
		"1 of 1 binaries rebuilt bit for bit",
	} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(stdout, want) {
				t.Errorf("the report lacks %q:\n%s", want, stdout)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(out, hostTarget.distDir("fixture"), "fixture"+hostTarget.ext())); err != nil {
		t.Errorf("the rebuilt binary is not kept under -out: %v", err)
	}
}

// TestRun_ABinaryThatDiffersFromTheRelease covers the verdict that matters:
// a published binary stamped with something the configuration at the tag
// does not stamp fails the run, and the report says the build information
// agrees, since -trimpath keeps the linker flags out of it, and where the
// bytes part.
func TestRun_ABinaryThatDiffersFromTheRelease(t *testing.T) {
	t.Parallel()

	repo, commit := releaseRepo(t, hostConfig(""), nil)
	published := publishRelease(t, map[string][]byte{hostAsset(): publishBinary(t, repo, hostTarget, "1.2.3", "elsewhere")})
	code, stdout, stderr := runRelease(options{dir: repo, config: ".goreleaser.yml", published: published, tag: "v1.2.3", ref: commit})
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"DIFFERS " + hostAsset(),
		"the build information is the same",
		"the bytes first differ at offset",
		"1 of 1 binaries differ from what the release published",
	} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(stdout, want) {
				t.Errorf("the report lacks %q:\n%s", want, stdout)
			}
		})
	}
}

// TestRun_AReleaseStampedWithTheShortCommit covers a release whose
// configuration stamps {{.ShortCommit}}, as every release up to 3.3.0 did:
// git abbreviates a commit to as many characters as the clone needs, so the
// clone that built the release may have used another length than this one.
// Rebuilt with this clone's abbreviation the binary differs and the report
// says why; rebuilt with the abbreviation -short-commit gives back, it is
// the published one.
func TestRun_AReleaseStampedWithTheShortCommit(t *testing.T) {
	t.Parallel()

	repo, commit := releaseRepo(t, strings.Replace(hostConfig(""), "{{.FullCommit}}", "{{.ShortCommit}}", 1), nil)
	published := publishRelease(t, map[string][]byte{hostAsset(): publishBinary(t, repo, hostTarget, "1.2.3", commit[:12])})
	t.Run("this clone's abbreviation", func(t *testing.T) {
		t.Parallel()

		code, stdout, stderr := runRelease(options{dir: repo, config: ".goreleaser.yml", published: published, tag: "v1.2.3", ref: "HEAD"})
		if code != 1 || !strings.Contains(stdout, "DIFFERS") || !strings.Contains(stdout, "the configuration stamps {{.ShortCommit}}") {
			t.Errorf("exit %d, want 1 and the abbreviation named as the likely cause\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
	})
	t.Run("the abbreviation given back", func(t *testing.T) {
		t.Parallel()

		code, stdout, stderr := runRelease(options{dir: repo, config: ".goreleaser.yml", published: published, tag: "v1.2.3", ref: "HEAD", shortCommit: commit[:12]})
		if code != 0 || strings.Contains(stdout, "{{.ShortCommit}}") {
			t.Errorf("exit %d, want 0 and no note on the abbreviation\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
	})
}

// TestShortCommitHint_OnlyWhereItCanBeTheCause covers when the note on the
// abbreviation is added: to a reproduction that differs, whose command line
// gave no abbreviation, of a configuration that stamps one.
func TestShortCommitHint_OnlyWhereItCanBeTheCause(t *testing.T) {
	t.Parallel()

	stamps := release{builds: []build{{ldflags: []string{"-X main.commit={{.ShortCommit}}"}}}}
	full := release{builds: []build{{ldflags: []string{"-X main.commit={{.FullCommit}}"}}}}
	differs := report{outcomes: []outcome{{before: "a", after: "b"}}}
	agrees := report{outcomes: []outcome{{before: "a", after: "a"}}}
	for _, tc := range []struct {
		name  string
		rep   report
		given string
		rel   release
		want  bool
	}{
		{name: "a difference with nothing given", rep: differs, rel: stamps, want: true},
		{name: "every binary the same", rep: agrees, rel: stamps},
		{name: "an abbreviation given", rep: differs, given: "abcdef12", rel: stamps},
		{name: "a configuration stamping the full commit", rep: differs, rel: full},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := shortCommitHint(tc.rep, tc.given, tc.rel, fields{ShortCommit: "abc1234"})
			if (len(got) == 1 && strings.Contains(got[0], "rendered as abc1234")) != tc.want || len(got) > 1 {
				t.Errorf("shortCommitHint = %q, want a note: %v", got, tc.want)
			}
		})
	}
}

// TestRelease_UsesShortCommit covers every template of a build that can
// stamp the abbreviation: the binary's name, the env, the flags, the ldflags
// and an override's ldflags.
func TestRelease_UsesShortCommit(t *testing.T) {
	t.Parallel()

	short := "{{ .ShortCommit }}"
	for _, tc := range []struct {
		name  string
		build build
		want  bool
	}{
		{name: "none", build: build{binary: "b", env: []string{"A=1"}, flags: []string{"-trimpath"}, ldflags: []string{"-s"}, overrides: []override{{ldflags: []string{"-w"}}}}},
		{name: "the binary", build: build{binary: "b-" + short}, want: true},
		{name: "the env", build: build{env: []string{"C=" + short}}, want: true},
		{name: "the flags", build: build{flags: []string{"-tags=" + short}}, want: true},
		{name: "the ldflags", build: build{ldflags: []string{"-X c=" + short}}, want: true},
		{name: "an override", build: build{overrides: []override{{ldflags: []string{"-X c=" + short}}}}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := (release{builds: []build{tc.build}}).usesShortCommit(); got != tc.want {
				t.Errorf("usesShortCommit = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRun_AUniversalBinaryIsJoinedInThePublishedOrder covers the universal
// darwin binary: GoReleaser joins the darwin builds in the order they
// finished, which no configuration fixes, so the published header is what
// gives the order, and the binary joined from the rebuilt darwin builds in
// that order is the published one. A release whose checksum file does not
// list it fails.
func TestRun_AUniversalBinaryIsJoinedInThePublishedOrder(t *testing.T) {
	t.Parallel()

	universal := "universal_binaries:\n  - id: fixture-universal\n    ids: [fixture]\n    name_template: fixture\n"
	repo, commit := releaseRepo(t, fixtureReleaseConfig("darwin", "amd64, arm64", universal), nil)
	amd64 := publishBinary(t, repo, target{goos: "darwin", goarch: "amd64"}, "1.2.3", commit)
	arm64 := publishBinary(t, repo, target{goos: "darwin", goarch: "arm64"}, "1.2.3", commit)
	thin := map[string][]byte{"fixture-darwin-amd64": amd64, "fixture-darwin-arm64": arm64}

	t.Run("reproduced", func(t *testing.T) {
		t.Parallel()

		assets := maps.Clone(thin)
		assets["fixture-darwin-all"] = goreleaserFat(arm64, amd64)
		out := t.TempDir()
		code, stdout, stderr := runRelease(options{dir: repo, config: ".goreleaser.yml", published: publishRelease(t, assets), tag: "v1.2.3", ref: "HEAD", out: out})
		if code != 0 || !strings.Contains(stdout, "reproduced fixture-darwin-all (darwin/all)") || !strings.Contains(stdout, "3 of 3 binaries") {
			t.Fatalf("exit %d, want 0 and the universal binary reproduced\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "fixture-universal_darwin_all", "fixture")); err != nil {
			t.Errorf("the joined universal binary is not kept under -out: %v", err)
		}
	})
	t.Run("not listed", func(t *testing.T) {
		t.Parallel()

		code, stdout, stderr := runRelease(options{dir: repo, config: ".goreleaser.yml", published: publishRelease(t, thin), tag: "v1.2.3", ref: "HEAD"})
		if code != 1 || !strings.Contains(stderr, "checksums.txt lists no fixture-darwin-all") {
			t.Errorf("exit %d, want 1 and the missing universal binary named\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
	})
}

// TestRun_RefusesWhatItCannotRebuild covers every step that can stop a
// reproduction before it compares anything: the ref, the configuration at
// it, the templates, the checksum file, the published binary and what it
// records, and the build. Each fails on stderr with exit 1 and prints no
// report, since a reproduction that could not be made is not one that passed.
func TestRun_RefusesWhatItCannotRebuild(t *testing.T) {
	t.Parallel()

	repo, commit := releaseRepo(t, hostConfig(""), nil)
	good := publishRelease(t, map[string][]byte{hostAsset(): publishBinary(t, repo, hostTarget, "1.2.3", commit)})
	other, otherCommit := releaseRepo(t, hostConfig(""), map[string]string{"other.txt": "another commit\n"})
	fromOther := publishRelease(t, map[string][]byte{hostAsset(): publishBinary(t, other, hostTarget, "1.2.3", otherCommit)})
	badChecksum, _ := releaseRepo(t, strings.Replace(hostConfig(""), "name_template: checksums.txt", "name_template: \"{{ .Nope }}\"", 1), nil)
	badArchive, _ := releaseRepo(t, strings.Replace(hostConfig(""), `"{{ .Binary }}-{{ .Os }}-{{ .Arch }}"`, `"{{ .Nope }}"`, 1), nil)
	broken, brokenCommit := releaseRepo(t, strings.Replace(hostConfig(""), "main: .", "main: ./broken", 1),
		map[string]string{"broken/main.go": "package main\n\nfunc main() { undefined() }\n"})
	fromBroken := publishRelease(t, map[string][]byte{hostAsset(): publishBinary(t, broken, hostTarget, "1.2.3", brokenCommit)})

	for _, tc := range []struct {
		name string
		o    options
		want string
	}{
		{name: "a ref that names no commit", o: options{dir: repo, ref: "v9.9.9", published: good}, want: "v9.9.9 names no commit"},
		{name: "a configuration the commit does not hold", o: options{dir: repo, config: "absent.yml", published: good}, want: "absent.yml"},
		{name: "a checksum file name it cannot evaluate", o: options{dir: badChecksum, published: good}, want: "Nope"},
		{name: "an archive name it cannot evaluate", o: options{dir: badArchive, published: good}, want: "archive name_template"},
		{name: "no checksum file", o: options{dir: repo, published: t.TempDir()}, want: "checksums.txt"},
		{name: "a binary the checksum file does not list", o: options{dir: repo, published: publishRelease(t, map[string][]byte{"other": []byte("x")})}, want: "checksums.txt lists no " + hostAsset()},
		{name: "a published file that is no Go binary", o: options{dir: repo, published: publishRelease(t, map[string][]byte{hostAsset(): []byte("x")})}, want: "carries no Go build information"},
		{name: "a binary built from another commit", o: options{dir: repo, published: fromOther}, want: "was built from " + otherCommit + ", and the commit rebuilt is " + commit},
		{name: "a main package that does not build", o: options{dir: broken, published: fromBroken}, want: "building " + hostAsset()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			o := tc.o
			o.tag = "v1.2.3"
			if o.ref == "" {
				o.ref = "HEAD"
			}
			if o.config == "" {
				o.config = ".goreleaser.yml"
			}
			code, stdout, stderr := runRelease(o)
			if code != 1 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit %d, want 1, no report and %q on stderr\nstdout:\n%s\nstderr:\n%s", code, tc.want, stdout, stderr)
			}
		})
	}
}

// TestRun_NoTemporaryDirectory covers the directory the clone and the builds
// go into not being creatable, in both modes, set up by pointing every
// variable the platforms read it from at nothing.
func TestRun_NoTemporaryDirectory(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, absent)
	}
	for name, o := range map[string]options{
		"against a release": {dir: ".", published: ".", tag: "v1.2.3", ref: "HEAD", config: ".goreleaser.yml"},
		"twice":             {twice: true, dir: ".", tag: "v1.2.3", config: ".goreleaser.yml"},
	} {
		t.Run(name, func(t *testing.T) {
			if code, _, stderr := runRelease(o); code != 1 || !strings.Contains(stderr, absent) {
				t.Errorf("exit %d, want 1 and %s named on stderr: %s", code, absent, stderr)
			}
		})
	}
}

// TestReleaseRun_Fat_RefusesWhatItCannotJoin covers the universal binary's
// own steps with stand-in slices, which no darwin build is needed for: the
// published file not being a universal binary, its header not fitting the
// rebuilt slices, the joined file not being writable where -out says, and
// the verdict either way.
func TestReleaseRun_Fat_RefusesWhatItCannotJoin(t *testing.T) {
	t.Parallel()

	amd64, arm64 := syntheticThin(darwinAMD64, 64, 1), syntheticThin(darwinARM64, 64, 2)
	fr := fatRecipe{asset: "fixture-darwin-all", dir: "u_darwin_all", file: "fixture", slices: []recipe{{asset: "fixture-darwin-amd64"}, {asset: "fixture-darwin-arm64"}}}
	built := map[string][]byte{"fixture-darwin-amd64": amd64, "fixture-darwin-arm64": arm64}
	occupied := t.TempDir()
	writeFiles(t, occupied, map[string]string{"u_darwin_all/fixture/inside": "a directory stands where the file goes"})
	fileOut := filepath.Join(t.TempDir(), "file")
	writeFiles(t, filepath.Dir(fileOut), map[string]string{"file": ""})

	fat := func(data []byte) map[string][]byte { return map[string][]byte{"fixture-darwin-all": data} }
	for _, tc := range []struct {
		name      string
		published map[string][]byte
		out       string
		want      string
		same      bool
	}{
		{name: "the same file", published: fat(goreleaserFat(arm64, amd64)), same: true},
		{name: "another slice", published: fat(goreleaserFat(syntheticThin(darwinARM64, 64, 3), amd64))},
		{name: "not listed", published: map[string][]byte{"fixture-darwin-amd64": amd64}, want: "checksums.txt lists no fixture-darwin-all"},
		{name: "not a universal binary", published: fat(amd64), want: "the published fixture-darwin-all is not a universal Mach-O file"},
		{name: "a slice the header lacks", published: fat(goreleaserFat(amd64)), want: "fixture-darwin-all: the published universal binary holds 1 architectures and 2 darwin binaries were rebuilt"},
		{name: "an output under a file", published: fat(goreleaserFat(arm64, amd64)), out: fileOut, want: "file"},
		{name: "a directory where the file goes", published: fat(goreleaserFat(arm64, amd64)), out: occupied, want: "fixture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := publishRelease(t, tc.published)
			sums, err := releaseassets.ReadChecksums(filepath.Join(dir, "checksums.txt"))
			if err != nil {
				t.Fatal(err)
			}
			out := tc.out
			if out == "" {
				out = t.TempDir()
			}
			got, err := releaseRun{published: dir, out: out, sums: sums}.fat(fr, built)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("fat = %v, want an error containing %q", err, tc.want)
				}
				return
			}
			if err != nil || got.same() != tc.same || got.asset != fr.asset || got.target != "darwin/all" {
				t.Errorf("fat = %#v, %v, want the same file: %v", got, err, tc.same)
			}
		})
	}
}

// TestInTree_RelativeToTheModuleOrAbsolute covers where -config is read: a
// relative path inside the module being built, an absolute one as it is.
func TestInTree_RelativeToTheModuleOrAbsolute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "elsewhere.yml")
	for _, tc := range []struct {
		name, path, want string
	}{
		{name: "relative", path: ".goreleaser.yml", want: filepath.Join(root, ".goreleaser.yml")},
		{name: "absolute", path: absolute, want: absolute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := inTree(root, tc.path); got != tc.want {
				t.Errorf("inTree(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestUnbuiltNote_NamesWhatTheChecksumFileListsBeyondTheBinaries covers the
// note on what a reproduction compares nothing against: the SBOMs and the
// notices a release lists beside its binaries, sorted, or no note at all when
// the list holds the binaries alone.
func TestUnbuiltNote_NamesWhatTheChecksumFileListsBeyondTheBinaries(t *testing.T) {
	t.Parallel()

	sums := releaseassets.Checksums{Digests: map[string]string{"bin": "", "z.sbom.json": "", "NOTICES": ""}}
	want := []string{"not rebuilt, since the configuration builds none of them: NOTICES, z.sbom.json"}
	if got := unbuiltNote(sums, []outcome{{asset: "bin"}}); !slices.Equal(got, want) {
		t.Errorf("unbuiltNote = %q, want %q", got, want)
	}
	if got := unbuiltNote(releaseassets.Checksums{Digests: map[string]string{"bin": ""}}, []outcome{{asset: "bin"}}); got != nil {
		t.Errorf("unbuiltNote with nothing left over = %q, want none", got)
	}
}
