// goreleaser_test.go covers how the release targets are read out of a
// GoReleaser configuration, the repository's own included.
package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeConfig writes a GoReleaser configuration into a directory of its own
// and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".goreleaser.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the configuration: %v", err)
	}
	return path
}

// TestReadBuilds_TheRepositorysReleaseIsSixTargetsOfTheServer pins what the
// gate reads from the configuration the release actually uses.
//
// It fails when the release gains a build, a target or a key this command does
// not read, which is the moment to look at this command again rather than let
// it scan a set of binaries nobody publishes. Its passing also says the
// configuration's overrides change ldflags alone (the Linux loader paths), that
// it sets no gomod and that its one before hook is go mod download, since
// readBuilds refuses anything else in each place.
func TestReadBuilds_TheRepositorysReleaseIsSixTargetsOfTheServer(t *testing.T) {
	t.Parallel()

	builds, err := readBuilds(filepath.Join("..", "..", ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("readBuilds(.goreleaser.yml): %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("the release declares %d builds, want the one this command was written against", len(builds))
	}
	b := builds[0]
	if b.id != "gitlab-mcp-server" || b.main != "./cmd/server" {
		t.Errorf("build = %q from %q, want gitlab-mcp-server from ./cmd/server", b.id, b.main)
	}
	if !slices.Contains(b.env, "CGO_ENABLED=0") {
		t.Errorf("env = %v, want CGO_ENABLED=0 among it", b.env)
	}
	if !slices.Equal(b.flags, []string{"-trimpath", "-buildmode=pie"}) {
		t.Errorf("flags = %v, want -trimpath and -buildmode=pie", b.flags)
	}
	var names []string
	for _, tgt := range b.targets {
		names = append(names, tgt.String())
	}
	want := []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"}
	if !slices.Equal(names, want) {
		t.Errorf("targets = %v, want %v", names, want)
	}
}

// TestReadBuilds_CrossesEveryGoosWithEveryGoarch covers the cross product, in
// the order the lists are written, and the name an entry without an id is
// given.
func TestReadBuilds_CrossesEveryGoosWithEveryGoarch(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, "builds:\n  - main: ./cmd/x\n    goos: [linux, windows]\n    goarch: [amd64, arm64]\n")
	builds, err := readBuilds(path)
	if err != nil {
		t.Fatalf("readBuilds: %v", err)
	}
	if len(builds) != 1 || builds[0].id != "builds[0]" {
		t.Fatalf("builds = %+v, want one named builds[0]", builds)
	}
	var names []string
	for _, tgt := range builds[0].targets {
		names = append(names, tgt.String())
	}
	if want := []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"}; !slices.Equal(names, want) {
		t.Errorf("targets = %v, want %v", names, want)
	}
}

// TestReadBuilds_AcceptsTopLevelSettingsThatLeaveTheModuleSetAlone covers the
// shapes of gomod and before that set nothing this command would have to
// apply: a gomod with no keys, a before with no value, and go mod download
// reached through an alias, which GoReleaser reads as the command it names.
func TestReadBuilds_AcceptsTopLevelSettingsThatLeaveTheModuleSetAlone(t *testing.T) {
	t.Parallel()

	const build = "builds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n"
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "an empty gomod", content: "gomod: {}\n" + build},
		{name: "a before with no value", content: "before:\n" + build},
		{name: "a hook reached through an alias", content: "download: &d go mod download\nbefore:\n  hooks: [*d]\n" + build},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			builds, err := readBuilds(writeConfig(t, tc.content))
			if err != nil || len(builds) != 1 || builds[0].id != "x" {
				t.Fatalf("readBuilds = %+v, %v; want the one build x", builds, err)
			}
		})
	}
}

// TestReadBuilds_RefusesWhatItCannotReadExactly covers every refusal, each of
// which is a configuration read half right if it were accepted instead.
func TestReadBuilds_RefusesWhatItCannotReadExactly(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "not YAML", content: "builds: [", want: ".goreleaser.yml"},
		{name: "no builds", content: "version: 2\n", want: "declares no builds"},
		{name: "no main package", content: "builds:\n  - id: x\n    goos: [linux]\n    goarch: [amd64]\n", want: "x names no main package"},
		{name: "no goos", content: "builds:\n  - id: x\n    main: .\n    goarch: [amd64]\n", want: "x lists no goos or no goarch"},
		{name: "no goarch", content: "builds:\n  - id: x\n    main: .\n    goos: [linux]\n", want: "x lists no goos or no goarch"},
		{name: "ignore", content: "builds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n    ignore:\n      - goos: linux\n", want: "x sets ignore, which this command does not read"},
		{name: "targets", content: "builds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n    targets: [linux_amd64]\n", want: "x sets targets, which this command does not read"},
		{name: "build tags", content: "builds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n    tags: [netgo]\n", want: "x sets tags, which this command does not read"},
		{name: "build dir", content: "builds:\n  - id: x\n    dir: sub\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n", want: "x sets dir, which this command does not read"},
		{name: "go binary", content: "builds:\n  - id: x\n    main: .\n    gobinary: go1.20\n    goos: [linux]\n    goarch: [amd64]\n", want: "x sets gobinary, which this command does not read"},
		{name: "global env", content: "env: [GOEXPERIMENT=boringcrypto]\nbuilds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n", want: "sets a global env, which every build inherits"},
		{name: "gomod proxy", content: "gomod:\n  proxy: true\nbuilds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n", want: "sets gomod.proxy, which changes how every build fetches or builds the module"},
		{name: "a before key other than hooks", content: "before:\n  hooks: [go mod download]\n  other: x\nbuilds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n", want: "sets before.other, which this command does not read"},
		{name: "a before that is not a mapping", content: "before: x\nbuilds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n", want: ".goreleaser.yml: before: yaml: unmarshal errors"},
		{name: "a hook other than go mod download", content: "before:\n  hooks:\n    - go mod download\n    - go generate ./...\nbuilds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n", want: "before.hooks[1] is not \"go mod download\"; a hook may change the source"},
		{name: "a hook written with options", content: "before:\n  hooks:\n    - cmd: go mod download\n      dir: sub\nbuilds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n", want: "before.hooks[0] is not \"go mod download\""},
		{name: "override env", content: "builds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n    overrides:\n      - goos: linux\n        ldflags: [-s]\n      - goos: linux\n        env: [GOEXPERIMENT=boringcrypto]\n", want: "x overrides[1] sets env; an override may change ldflags and nothing else"},
		{name: "override flags", content: "builds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n    overrides:\n      - goos: linux\n        flags: [-tags=other]\n", want: "x overrides[0] sets flags"},
		{name: "override tags", content: "builds:\n  - id: x\n    main: .\n    goos: [linux]\n    goarch: [amd64]\n    overrides:\n      - goos: linux\n        tags: [extra]\n", want: "x overrides[0] sets tags"},
		{name: "a key reached through an alias", content: "base: &b\n  id: x\n  main: .\n  goos: [linux]\n  goarch: [amd64]\n  tags: [netgo]\nbuilds:\n  - *b\n", want: "x sets tags"},
		{name: "a merge key", content: "base: &b\n  goos: [linux]\n  goarch: [amd64]\nbuilds:\n  - id: x\n    main: .\n    <<: *b\n", want: "x sets <<"},
		{name: "an entry that is not a mapping", content: "builds:\n  - ./cmd/x\n", want: "builds[0]: yaml: unmarshal errors"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			builds, err := readBuilds(writeConfig(t, tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("readBuilds = %+v, %v; want a refusal containing %q", builds, err, tc.want)
			}
		})
	}
}

// TestReadBuilds_AFileThatCannotBeRead_IsAnError covers the configuration
// being absent, which a gate must not read as a release with nothing in it.
func TestReadBuilds_AFileThatCannotBeRead_IsAnError(t *testing.T) {
	t.Parallel()

	if _, err := readBuilds(filepath.Join(t.TempDir(), "absent.yml")); err == nil {
		t.Fatal("readBuilds accepted a configuration that does not exist")
	}
}
