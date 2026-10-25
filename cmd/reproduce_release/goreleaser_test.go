// goreleaser_test.go covers what this command reads of a GoReleaser
// configuration, and every setting it refuses rather than read half right.
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// fixtureConfig is a configuration shaped like the repository's own: one
// build for six targets with an override per Linux target, a universal
// darwin binary, one archive publishing each binary as it is, and a checksum
// file, beside keys this command does not read (changelog, mod_timestamp).
const fixtureConfig = `version: 2
before:
  hooks:
    - go mod download
builds:
  - id: fixture
    main: ./cmd/fixture
    binary: fixture
    env: [CGO_ENABLED=0]
    flags: [-trimpath, -buildmode=pie]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - &ldflags -s -w -X main.version={{.Version}} -X main.commit={{.FullCommit}}
    mod_timestamp: "{{ .CommitTimestamp }}"
    overrides:
      - goos: linux
        goarch: amd64
        ldflags:
          - *ldflags
          - -I /lib64/ld-linux-x86-64.so.2
      - goos: linux
        goarch: arm64
        goarm64: v8.0
        ldflags:
          - *ldflags
          - -I /lib/ld-linux-aarch64.so.1
universal_binaries:
  - id: fixture-universal
    ids: [fixture]
    name_template: fixture
    replace: false
    mod_timestamp: "{{ .CommitTimestamp }}"
archives:
  - id: binaries
    formats: [binary]
    name_template: "{{ .Binary }}-{{ .Os }}-{{ .Arch }}"
checksum:
  name_template: checksums.txt
  algorithm: sha256
changelog:
  use: github
`

// The pieces of the smallest configuration this command reads, which each
// refusal case below changes one thing in.
const (
	minimalBuild    = "builds:\n  - id: fixture\n    main: .\n    binary: fixture\n    goos: [linux]\n    goarch: [amd64]\n"
	minimalArchive  = "archives:\n  - formats: [binary]\n    name_template: \"{{ .Binary }}-{{ .Os }}-{{ .Arch }}\"\n"
	minimalChecksum = "checksum:\n  name_template: checksums.txt\n"
)

// minimalWith is the smallest configuration with one of its three pieces
// replaced, or with text put in front of all three.
func minimalWith(before, build, archive, checksum string) string {
	return before + build + archive + checksum
}

// writeConfig writes a configuration into a directory of its own and
// returns its path.
func writeConfig(t *testing.T, text string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".goreleaser.yml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("writing the configuration: %v", err)
	}
	return path
}

// TestReadRelease_ReadsWhatDecidesEachBinary reads the fixture whole: the
// build with its targets in GoReleaser's order (each goos crossed with each
// goarch), its overrides with the anchor resolved, the universal binary, the
// archive's name template and the checksum file's.
func TestReadRelease_ReadsWhatDecidesEachBinary(t *testing.T) {
	t.Parallel()

	got, err := readRelease(writeConfig(t, fixtureConfig))
	if err != nil {
		t.Fatalf("readRelease: %v", err)
	}
	ldflags := "-s -w -X main.version={{.Version}} -X main.commit={{.FullCommit}}"
	want := release{
		builds: []build{{
			id:      "fixture",
			main:    "./cmd/fixture",
			binary:  "fixture",
			env:     []string{"CGO_ENABLED=0"},
			flags:   []string{"-trimpath", "-buildmode=pie"},
			ldflags: []string{ldflags},
			overrides: []override{
				{goos: "linux", goarch: "amd64", ldflags: []string{ldflags, "-I /lib64/ld-linux-x86-64.so.2"}},
				{goos: "linux", goarch: "arm64", goarm64: "v8.0", ldflags: []string{ldflags, "-I /lib/ld-linux-aarch64.so.1"}},
			},
			targets: []target{
				{goos: "linux", goarch: "amd64"},
				{goos: "linux", goarch: "arm64"},
				{goos: "darwin", goarch: "amd64"},
				{goos: "darwin", goarch: "arm64"},
				{goos: "windows", goarch: "amd64"},
				{goos: "windows", goarch: "arm64"},
			},
		}},
		universals: []universal{{id: "fixture-universal", name: "fixture", ids: []string{"fixture"}}},
		archive:    "{{ .Binary }}-{{ .Os }}-{{ .Arch }}",
		checksums:  "checksums.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readRelease =\n%#v\nwant\n%#v", got, want)
	}
}

// TestReadRelease_AUniversalBinaryWithoutIDsJoinsItsOwnID covers GoReleaser's
// default for ids, which is the entry's own id: here a build carries that id,
// so the configuration reads.
func TestReadRelease_AUniversalBinaryWithoutIDsJoinsItsOwnID(t *testing.T) {
	t.Parallel()

	config := minimalWith("", strings.Replace(minimalBuild, "goos: [linux]", "goos: [darwin]", 1), minimalArchive, minimalChecksum) +
		"universal_binaries:\n  - id: fixture\n    name_template: fixture\n"
	got, err := readRelease(writeConfig(t, config))
	if err != nil {
		t.Fatalf("readRelease: %v", err)
	}
	if want := []universal{{id: "fixture", name: "fixture", ids: []string{"fixture"}}}; !reflect.DeepEqual(got.universals, want) {
		t.Errorf("universals = %#v, want %#v", got.universals, want)
	}
}

// TestReadRelease_RefusesWhatItDoesNotRead covers every refusal. A setting
// this command does not apply would make it rebuild something other than
// what GoReleaser built, and the comparison would then say nothing true, so
// each one stops the run with the setting named.
func TestReadRelease_RefusesWhatItDoesNotRead(t *testing.T) {
	t.Parallel()

	build := func(old, replacement string) string { return strings.Replace(minimalBuild, old, replacement, 1) }
	withOverride := func(entry string) string { return minimalBuild + "    overrides:\n" + entry }
	withUniversal := func(entry string) string {
		return minimalWith("", build("goos: [linux]", "goos: [darwin]"), minimalArchive, minimalChecksum) + "universal_binaries:\n" + entry
	}
	for _, tc := range []struct {
		name, config, want string
	}{
		{name: "YAML that does not parse", config: "builds: [\n", want: ".goreleaser.yml"},
		{name: "a top-level section that rewrites a binary", config: minimalWith("upx:\n  - enabled: true\n", minimalBuild, minimalArchive, minimalChecksum), want: "sets upx"},
		{name: "a top-level key GoReleaser does not define", config: minimalWith("x-entry: 1\n", minimalBuild, minimalArchive, minimalChecksum), want: "sets x-entry"},
		{name: "two top-level sections it does not read", config: minimalWith("upx: []\nnotarize: {}\n", minimalBuild, minimalArchive, minimalChecksum), want: "sets notarize,"},
		{name: "a global env", config: minimalWith("env: [GOFLAGS=-mod=mod]\n", minimalBuild, minimalArchive, minimalChecksum), want: "sets a global env"},
		{name: "a gomod setting", config: minimalWith("gomod:\n  proxy: true\n", minimalBuild, minimalArchive, minimalChecksum), want: "sets gomod.proxy"},
		{name: "a before key other than hooks", config: minimalWith("before:\n  other: 1\n", minimalBuild, minimalArchive, minimalChecksum), want: "sets before.other"},
		{name: "before hooks that are not a list", config: minimalWith("before:\n  hooks: go mod download\n", minimalBuild, minimalArchive, minimalChecksum), want: "before:"},
		{name: "a hook other than go mod download", config: minimalWith("before:\n  hooks: [go generate ./...]\n", minimalBuild, minimalArchive, minimalChecksum), want: "before.hooks[0]"},
		{name: "no builds", config: minimalWith("", "", minimalArchive, minimalChecksum), want: "declares no builds"},
		{name: "a build key it does not read", config: minimalWith("", build("    goos:", "    tags: [x]\n    goos:"), minimalArchive, minimalChecksum), want: "builds[0] sets tags"},
		{name: "a build that does not decode", config: minimalWith("", build("goos: [linux]", "goos: linux"), minimalArchive, minimalChecksum), want: "builds[0]:"},
		{name: "a build with no id", config: minimalWith("", build("id: fixture\n    ", ""), minimalArchive, minimalChecksum), want: "needs an id, a main package and a binary"},
		{name: "a build with no main package", config: minimalWith("", build("main: .\n    ", ""), minimalArchive, minimalChecksum), want: "needs an id, a main package and a binary"},
		{name: "a build with no binary", config: minimalWith("", build("binary: fixture\n    ", ""), minimalArchive, minimalChecksum), want: "needs an id, a main package and a binary"},
		{name: "a build with no goos", config: minimalWith("", build("goos: [linux]\n    ", ""), minimalArchive, minimalChecksum), want: "lists no goos or no goarch"},
		{name: "a build with no goarch", config: minimalWith("", build("    goarch: [amd64]\n", ""), minimalArchive, minimalChecksum), want: "lists no goos or no goarch"},
		{name: "an operating system it names no files for", config: minimalWith("", build("[linux]", "[linux, freebsd]"), minimalArchive, minimalChecksum), want: "freebsd"},
		{name: "an architecture whose level it does not know", config: minimalWith("", build("[amd64]", "[amd64, 386]"), minimalArchive, minimalChecksum), want: "386"},
		{name: "an override key it does not read", config: minimalWith("", withOverride("      - goos: linux\n        goarch: amd64\n        env: [A=1]\n"), minimalArchive, minimalChecksum), want: "overrides[0] sets env"},
		{name: "an override that does not decode", config: minimalWith("", withOverride("      - goos: linux\n        goarch: amd64\n        ldflags: {a: b}\n"), minimalArchive, minimalChecksum), want: "overrides[0]:"},
		{name: "an override with no goos", config: minimalWith("", withOverride("      - goarch: amd64\n        ldflags: [-s]\n"), minimalArchive, minimalChecksum), want: "overrides[0] names no goos or no goarch"},
		{name: "an override with no goarch", config: minimalWith("", withOverride("      - goos: linux\n        ldflags: [-s]\n"), minimalArchive, minimalChecksum), want: "overrides[0] names no goos or no goarch"},
		{name: "a universal key it does not read", config: withUniversal("  - id: u\n    ids: [fixture]\n    name_template: f\n    hooks: {}\n"), want: "universal_binaries[0] sets hooks"},
		{name: "a universal binary that does not decode", config: withUniversal("  - id: u\n    ids: fixture\n    name_template: f\n"), want: "universal_binaries[0]:"},
		{name: "a universal binary with no id", config: withUniversal("  - ids: [fixture]\n    name_template: f\n"), want: "needs an id and a name_template"},
		{name: "a universal binary with no name", config: withUniversal("  - id: u\n    ids: [fixture]\n"), want: "needs an id and a name_template"},
		{name: "a universal binary that replaces the darwin ones", config: withUniversal("  - id: u\n    ids: [fixture]\n    name_template: f\n    replace: true\n"), want: "sets replace"},
		{name: "a universal binary of a build nobody declares", config: withUniversal("  - id: u\n    ids: [other]\n    name_template: f\n"), want: "joins build other"},
		{name: "a universal binary defaulting to an id no build carries", config: withUniversal("  - id: u\n    name_template: f\n"), want: "joins build u"},
		{name: "no archive", config: minimalWith("", minimalBuild, "", minimalChecksum), want: "declares 0 archives"},
		{name: "two archives", config: minimalWith("", minimalBuild, minimalArchive+strings.Replace(minimalArchive, "archives:\n", "", 1), minimalChecksum), want: "declares 2 archives"},
		{name: "an archive key it does not read", config: minimalWith("", minimalBuild, minimalArchive+"    files: [LICENSE]\n", minimalChecksum), want: "archives[0] sets files"},
		{name: "an archive that does not decode", config: minimalWith("", minimalBuild, "archives:\n  - formats: binary\n    name_template: x\n", minimalChecksum), want: "archives[0]:"},
		{name: "an archive of another format", config: minimalWith("", minimalBuild, strings.Replace(minimalArchive, "[binary]", "[tar.gz]", 1), minimalChecksum), want: "tar.gz"},
		{name: "an archive of two formats", config: minimalWith("", minimalBuild, strings.Replace(minimalArchive, "[binary]", "[binary, zip]", 1), minimalChecksum), want: "zip"},
		{name: "an archive with no name", config: minimalWith("", minimalBuild, "archives:\n  - formats: [binary]\n", minimalChecksum), want: "archives[0] names no name_template"},
		{name: "no checksum", config: minimalWith("", minimalBuild, minimalArchive, ""), want: "declares no checksum"},
		{name: "a checksum key it does not read", config: minimalWith("", minimalBuild, minimalArchive, minimalChecksum+"  split: true\n"), want: "checksum sets split"},
		{name: "a checksum that does not decode", config: minimalWith("", minimalBuild, minimalArchive, "checksum:\n  name_template: [a]\n"), want: "checksum:"},
		{name: "a checksum with no name", config: minimalWith("", minimalBuild, minimalArchive, "checksum:\n  algorithm: sha256\n"), want: "checksum names no name_template"},
		{name: "a checksum of another algorithm", config: minimalWith("", minimalBuild, minimalArchive, minimalChecksum+"  algorithm: sha512\n"), want: "sha512"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := readRelease(writeConfig(t, tc.config))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("readRelease = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// TestReadRelease_AFileThatCannotBeRead covers the configuration not being
// there at all, which a run at a commit that predates it meets.
func TestReadRelease_AFileThatCannotBeRead(t *testing.T) {
	t.Parallel()

	if _, err := readRelease(filepath.Join(t.TempDir(), "absent.yml")); !os.IsNotExist(err) {
		t.Errorf("readRelease = %v, want a not-exist error", err)
	}
}

// TestReadRelease_FollowsAnAlias covers an override written once under an
// anchor and named by an alias in a second build, the way the repository's
// configuration writes its ldflags once: GoReleaser reads what the alias
// names, so the second build is given the same override.
func TestReadRelease_FollowsAnAlias(t *testing.T) {
	t.Parallel()

	second := strings.NewReplacer("builds:\n", "", "id: fixture", "id: second", "binary: fixture", "binary: second").Replace(minimalBuild)
	builds := minimalBuild + "    overrides:\n      - &linux {goos: linux, goarch: amd64, ldflags: [-s]}\n" + second + "    overrides: [*linux]\n"
	got, err := readRelease(writeConfig(t, minimalWith("", builds, minimalArchive, minimalChecksum)))
	if err != nil {
		t.Fatalf("readRelease of an aliased override: %v", err)
	}
	want := []override{{goos: "linux", goarch: "amd64", ldflags: []string{"-s"}}}
	if len(got.builds) != 2 || !reflect.DeepEqual(got.builds[1].overrides, want) {
		t.Errorf("builds = %#v, want the second build given %#v", got.builds, want)
	}
}

// TestUnknownKey_FollowsAnAlias covers a refusal through an alias: the keys
// judged are those of the mapping the alias names, since that is what
// GoReleaser reads, so a refused key there is found.
func TestUnknownKey_FollowsAnAlias(t *testing.T) {
	t.Parallel()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("entry: &entry {id: fixture, tags: [x]}\nalias: *entry\n"), &doc); err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	alias := doc.Content[0].Content[3]
	if alias.Kind != yaml.AliasNode {
		t.Fatalf("the fixture's second value is of kind %v, want an alias", alias.Kind)
	}
	if got := unknownKey(alias, buildKeys); got != "tags" {
		t.Errorf("unknownKey = %q, want tags, the refused key of the mapping the alias names", got)
	}
}

// TestReadRelease_ReadsPastTheSectionsThatTouchNoBinary covers the top-level
// sections this command neither reads nor refuses. Each writes files beside
// the binaries or describes the release, so the binaries are the ones a
// configuration without it builds.
func TestReadRelease_ReadsPastTheSectionsThatTouchNoBinary(t *testing.T) {
	t.Parallel()

	for _, section := range []string{
		"version: 2\n",
		"snapshot:\n  version_template: \"{{ .Version }}-next\"\n",
		"changelog:\n  use: github\n",
		"signs:\n  - artifacts: checksum\n",
		"sboms:\n  - artifacts: binary\n",
		"release:\n  draft: true\n",
	} {
		name, _, _ := strings.Cut(section, ":")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := readRelease(writeConfig(t, minimalWith(section, minimalBuild, minimalArchive, minimalChecksum))); err != nil {
				t.Errorf("readRelease = %v, want %s read past", err, name)
			}
		})
	}
}

// TestReadRelease_TheRepositoryConfiguration holds this command to the
// repository's own .goreleaser.yml: it reads, it builds the six assets the
// installers look up by name and the universal darwin binary, and it stamps
// the full commit, which is what makes a release reproducible from any clone
// (an abbreviated commit takes the length a clone happens to need).
func TestReadRelease_TheRepositoryConfiguration(t *testing.T) {
	t.Parallel()

	rel, err := readRelease(filepath.Join("..", "..", ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("readRelease: %v", err)
	}
	commit := strings.Repeat("c", 40)
	thin, fat, err := rel.recipes(fields{Version: "9.8.7", Tag: "v9.8.7", Commit: commit, FullCommit: commit, ShortCommit: "cccccccc"})
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}
	var assets []string
	for _, rc := range thin {
		assets = append(assets, rc.asset)
		if want := "-X main.commit=" + commit; !strings.Contains(strings.Join(rc.flags, " "), want) {
			t.Errorf("%s is built with %q, which does not stamp %q", rc.asset, rc.flags, want)
		}
	}
	for _, fr := range fat {
		assets = append(assets, fr.asset)
	}
	want := []string{
		"gitlab-mcp-server-linux-amd64", "gitlab-mcp-server-linux-arm64",
		"gitlab-mcp-server-darwin-amd64", "gitlab-mcp-server-darwin-arm64",
		"gitlab-mcp-server-windows-amd64.exe", "gitlab-mcp-server-windows-arm64.exe",
		"gitlab-mcp-server-darwin-all",
	}
	if !reflect.DeepEqual(assets, want) {
		t.Errorf("the repository's configuration builds %q, want %q", assets, want)
	}
}

// TestTarget_NamesAndEnvironment covers what a target decides beyond its
// platform: the level GoReleaser builds it for, the file extension, the
// directory of dist/ it lands in, and the environment GoReleaser sets for it,
// where every level variable but the target's own is set empty.
func TestTarget_NamesAndEnvironment(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		target             target
		level, ext, dir    string
		amd64Env, arm64Env string
	}{
		{target: target{goos: "linux", goarch: "amd64"}, level: "v1", dir: "b_linux_amd64_v1", amd64Env: "v1"},
		{target: target{goos: "windows", goarch: "arm64"}, level: "v8.0", ext: ".exe", dir: "b_windows_arm64_v8.0", arm64Env: "v8.0"},
		{target: target{goos: "darwin", goarch: "arm64"}, level: "v8.0", dir: "b_darwin_arm64_v8.0", arm64Env: "v8.0"},
	} {
		t.Run(tc.target.String(), func(t *testing.T) {
			t.Parallel()

			if got := tc.target.String(); got != tc.target.goos+"/"+tc.target.goarch {
				t.Errorf("String() = %q", got)
			}
			if got := tc.target.level(); got != tc.level {
				t.Errorf("level() = %q, want %q", got, tc.level)
			}
			if got := tc.target.ext(); got != tc.ext {
				t.Errorf("ext() = %q, want %q", got, tc.ext)
			}
			if got := tc.target.distDir("b"); got != tc.dir {
				t.Errorf("distDir(b) = %q, want %q", got, tc.dir)
			}
			want := []string{
				"GOOS=" + tc.target.goos, "GOARCH=" + tc.target.goarch, "GOAMD64=" + tc.amd64Env, "GO386=", "GOARM=",
				"GOARM64=" + tc.arm64Env, "GOMIPS=", "GOMIPS64=", "GOPPC64=", "GORISCV64=",
			}
			if got := tc.target.env(); !reflect.DeepEqual(got, want) {
				t.Errorf("env() = %q, want %q", got, want)
			}
		})
	}
}

// TestOverride_Matches covers how an override picks its targets, the way
// GoReleaser does: the platform must be the target's, and a level it names
// counts only for the architecture it belongs to, where an empty one means
// GoReleaser's default.
func TestOverride_Matches(t *testing.T) {
	t.Parallel()

	amd64 := target{goos: "linux", goarch: "amd64"}
	arm64 := target{goos: "linux", goarch: "arm64"}
	for _, tc := range []struct {
		name     string
		override override
		target   target
		want     bool
	}{
		{name: "the same platform", override: override{goos: "linux", goarch: "amd64"}, target: amd64, want: true},
		{name: "another operating system", override: override{goos: "darwin", goarch: "amd64"}, target: amd64},
		{name: "another architecture", override: override{goos: "linux", goarch: "arm64"}, target: amd64},
		{name: "the default amd64 level", override: override{goos: "linux", goarch: "amd64", goamd64: "v1"}, target: amd64, want: true},
		{name: "another amd64 level", override: override{goos: "linux", goarch: "amd64", goamd64: "v3"}, target: amd64},
		{name: "the default arm64 level", override: override{goos: "linux", goarch: "arm64", goarm64: "v8.0"}, target: arm64, want: true},
		{name: "another arm64 level", override: override{goos: "linux", goarch: "arm64", goarm64: "v9.0"}, target: arm64},
		{name: "an amd64 level on an arm64 target", override: override{goos: "linux", goarch: "arm64", goamd64: "v3"}, target: arm64, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.override.matches(tc.target); got != tc.want {
				t.Errorf("matches(%s) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}
