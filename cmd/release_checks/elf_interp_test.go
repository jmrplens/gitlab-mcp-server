// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// elf_interp_test.go covers the elf-interp check with every case
// scripts/check_elf_interp_test.py held the script it replaces to: each
// binary judged against the loader its machine is promised, the exit code of
// a run over several, and the table of loaders held to the overrides
// .goreleaser.yml builds with. How an ELF image is read is
// cmd/internal/releaseassets', and the reader's own cases are there.
package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The loaders the release's Linux binaries name, written out rather than
// read from the table under test.
const (
	glibcAMD64 = "/lib64/ld-linux-x86-64.so.2"
	glibcARM64 = "/lib/ld-linux-aarch64.so.1"
)

// elfImage writes a minimal 64-bit little-endian ELF image for machine the
// way a correct build lays one out: the 64-byte file header, a program header
// table right after it holding one PT_LOAD and then one PT_INTERP per
// interpreter, and the interpreter strings after the table, each ending in a
// NUL as the linker writes it.
func elfImage(machine elf.Machine, interpreters ...string) []byte {
	const headerSize, entrySize = 64, 56
	phnum := 1 + len(interpreters)
	headers := make([]byte, headerSize+entrySize*phnum)
	copy(headers, elf.ELFMAG)
	headers[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	headers[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	headers[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(headers[18:], uint16(machine))
	binary.LittleEndian.PutUint32(headers[20:], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint64(headers[32:], headerSize)
	binary.LittleEndian.PutUint16(headers[54:], entrySize)
	binary.LittleEndian.PutUint16(headers[56:], uint16(phnum)) //#nosec G115 -- a fixture's entry count, a handful

	binary.LittleEndian.PutUint32(headers[headerSize:], uint32(elf.PT_LOAD))
	var text []byte
	for i, interp := range interpreters {
		entry := headers[headerSize+entrySize*(1+i):]
		binary.LittleEndian.PutUint32(entry, uint32(elf.PT_INTERP))
		binary.LittleEndian.PutUint64(entry[8:], uint64(len(headers)+len(text))) //#nosec G115 -- an offset inside a fixture of a few hundred bytes
		binary.LittleEndian.PutUint64(entry[32:], uint64(len(interp)+1))         //#nosec G115 -- a fixture's string length
		text = append(text, interp...)
		text = append(text, 0)
	}
	return slices.Concat(headers, text)
}

// writeBinary writes data into dir under name and returns its path.
func writeBinary(t *testing.T, dir, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// TestCheckInterpreter_JudgesEachBinary covers one binary at a time: the
// glibc loader of its machine passes, and another loader, the loader of the
// other machine, no loader at all, a machine the release does not build, a
// file that is not ELF and a file that is not there are each named.
func TestCheckInterpreter_JudgesEachBinary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		data []byte
		want string
		says string
	}{
		{name: "amd64 glibc", data: elfImage(elf.EM_X86_64, glibcAMD64), want: glibcAMD64},
		{name: "arm64 glibc", data: elfImage(elf.EM_AARCH64, glibcARM64), want: glibcARM64},
		{name: "amd64 musl", data: elfImage(elf.EM_X86_64, "/lib/ld-musl-x86_64.so.1"), says: " is an x86-64 binary naming interpreter /lib/ld-musl-x86_64.so.1, want " + glibcAMD64},
		{name: "arm64 with the amd64 loader", data: elfImage(elf.EM_AARCH64, glibcAMD64), says: " is an aarch64 binary naming interpreter " + glibcAMD64 + ", want " + glibcARM64},
		{name: "static", data: elfImage(elf.EM_X86_64), says: " is an x86-64 binary naming no interpreter, want " + glibcAMD64},
		{name: "riscv64", data: elfImage(elf.EM_RISCV, "/lib/ld-linux-riscv64-lp64d.so.1"), says: " is for ELF machine 0xf3, which the release does not build"},
		{name: "not ELF", data: append([]byte("MZ"), make([]byte, 100)...), says: " is not an ELF file"},
		{name: "two interpreters", data: elfImage(elf.EM_X86_64, glibcAMD64, glibcAMD64), says: " has 2 PT_INTERP headers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeBinary(t, dir, strings.ReplaceAll(tc.name, " ", "-"), tc.data)
			got, err := checkInterpreter(path)
			if tc.says == "" {
				if err != nil || got != tc.want {
					t.Errorf("checkInterpreter(%s) = %q, %v, want %q", tc.name, got, err, tc.want)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), path+tc.says) {
				t.Errorf("checkInterpreter(%s) = %q, %v, want an error starting %q", tc.name, got, err, path+tc.says)
			}
			if got != "" {
				t.Errorf("checkInterpreter(%s) refused the binary and returned %q, want nothing", tc.name, got)
			}
		})
	}
}

// TestCheckInterpreter_AMissingFileIsReportedNotRaised covers a binary that
// is not there: the run names it and goes on rather than stopping.
func TestCheckInterpreter_AMissingFileIsReportedNotRaised(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "absent")
	got, err := checkInterpreter(path)
	if err == nil || !strings.HasPrefix(err.Error(), path+" cannot be read: ") {
		t.Errorf("checkInterpreter(absent) = %q, %v, want an error starting %q", got, err, path+" cannot be read: ")
	}
}

// TestRunELFInterp_ExitCodes covers a run over several binaries: none named
// is a usage error, every one right passes and names each on stdout, and one
// wrong among right ones fails with every wrong one named on stderr and
// nothing on stdout, since what passed is printed only when everything did.
func TestRunELFInterp_ExitCodes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	good := writeBinary(t, dir, "good", elfImage(elf.EM_X86_64, glibcAMD64))
	alsoGood := writeBinary(t, dir, "also-good", elfImage(elf.EM_AARCH64, glibcARM64))
	bad := writeBinary(t, dir, "bad", elfImage(elf.EM_AARCH64, "/lib/ld-musl-aarch64.so.1"))
	alsoBad := writeBinary(t, dir, "also-bad", elfImage(elf.EM_X86_64))
	for _, tc := range []struct {
		name    string
		paths   []string
		want    int
		stdout  []string
		stderr  []string
		nothing string
	}{
		{name: "no binary named", paths: nil, want: 2, stderr: []string{"usage: release_checks elf-interp BINARY [BINARY...]\n"}, nothing: "stdout"},
		{
			name: "every binary right", paths: []string{good, alsoGood}, want: 0,
			stdout: []string{
				"release_checks: " + good + " names " + glibcAMD64 + ", the loader of its machine\n",
				"release_checks: " + alsoGood + " names " + glibcARM64 + ", the loader of its machine\n",
			},
			nothing: "stderr",
		},
		{
			name: "one wrong among right", paths: []string{good, bad}, want: 1,
			stderr:  []string{"release_checks: " + bad + " is an aarch64 binary naming interpreter /lib/ld-musl-aarch64.so.1, want " + glibcARM64 + "\n"},
			nothing: "stdout",
		},
		{
			name: "every wrong one named", paths: []string{bad, good, alsoBad}, want: 1,
			stderr: []string{
				"release_checks: " + bad + " is an aarch64 binary naming interpreter /lib/ld-musl-aarch64.so.1, want " + glibcARM64 + "\n",
				"release_checks: " + alsoBad + " is an x86-64 binary naming no interpreter, want " + glibcAMD64 + "\n",
			},
			nothing: "stdout",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := runMain(append([]string{"elf-interp"}, tc.paths...), strings.NewReader(""), &stdout, &stderr); code != tc.want {
				t.Errorf("elf-interp exited %d, want %d (stderr %q)", code, tc.want, stderr.String())
			}
			if got, want := stdout.String(), strings.Join(tc.stdout, ""); got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
			if got, want := stderr.String(), strings.Join(tc.stderr, ""); got != want {
				t.Errorf("stderr = %q, want %q", got, want)
			}
		})
	}
}

// goreleaserOverride is one per-target override of a GoReleaser build.
type goreleaserOverride struct {
	Goos    string   `yaml:"goos"`
	Goarch  string   `yaml:"goarch"`
	Ldflags []string `yaml:"ldflags"`
}

// goreleaserConfig is the part of .goreleaser.yml the loaders are held to:
// each build's ldflags and its per-target overrides.
type goreleaserConfig struct {
	Builds []struct {
		Ldflags   []string             `yaml:"ldflags"`
		Overrides []goreleaserOverride `yaml:"overrides"`
	} `yaml:"builds"`
}

// goarchMachines are the ELF machines of the architectures the release
// builds for linux.
var goarchMachines = map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}

// readGoreleaserConfig decodes the repository's .goreleaser.yml.
func readGoreleaserConfig(t *testing.T) goreleaserConfig {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("reading .goreleaser.yml: %v", err)
	}
	var config goreleaserConfig
	if err = yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("decoding .goreleaser.yml: %v", err)
	}
	return config
}

// interpreterFlag is the loader an ldflags list passes with -I, or "" when it
// passes none.
func interpreterFlag(ldflags []string) string {
	for _, flag := range ldflags {
		if path, ok := strings.CutPrefix(flag, "-I "); ok {
			return path
		}
	}
	return ""
}

// TestLoaders_AreTheInterpretersGoreleaserBuildsWith holds the table of
// loaders to the overrides .goreleaser.yml builds with, which is what lets the
// check carry the table rather than read the configuration in the credentialed
// job: every -I the configuration passes is a loader of the table, each of the
// table's machines has a linux override, and each linux override passes the
// loader of its architecture's machine after the build's own ldflags, which
// an override replaces whole.
func TestLoaders_AreTheInterpretersGoreleaserBuildsWith(t *testing.T) {
	t.Parallel()

	config := readGoreleaserConfig(t)
	var named, want []string
	overridden := map[elf.Machine]bool{}
	for _, build := range config.Builds {
		if path := interpreterFlag(build.Ldflags); path != "" {
			named = append(named, path)
		}
		for _, o := range build.Overrides {
			path := interpreterFlag(o.Ldflags)
			if path != "" {
				named = append(named, path)
			}
			if o.Goos == "linux" {
				overridden[goarchMachines[o.Goarch]] = true
				t.Run(o.Goarch, func(t *testing.T) {
					t.Parallel()

					assertLinuxOverride(t, o, path, build.Ldflags)
				})
			}
		}
	}
	var missing []string
	for machine, l := range loaders {
		want = append(want, l.path)
		if !overridden[machine] {
			missing = append(missing, l.name)
		}
	}
	slices.Sort(named)
	slices.Sort(want)
	if !slices.Equal(named, want) {
		t.Errorf(".goreleaser.yml passes -I %q, want exactly the loaders of the table, %q", named, want)
	}
	if len(missing) != 0 {
		t.Errorf("the table holds %q binaries to a loader no linux override in .goreleaser.yml passes", missing)
	}
}

// assertLinuxOverride holds one linux override to passing path, the loader of
// its architecture's machine, after buildLdflags' first entry, the build's
// own flags.
func assertLinuxOverride(t *testing.T, o goreleaserOverride, path string, buildLdflags []string) {
	t.Helper()

	machine, known := goarchMachines[o.Goarch]
	if !known {
		t.Fatalf("a linux/%s override, an architecture the release does not build", o.Goarch)
	}
	if want := loaders[machine]; path != want.path {
		t.Errorf("the linux/%s override passes -I %q, want %q, the loader the check holds an %s binary to", o.Goarch, path, want.path, want.name)
	}
	if len(o.Ldflags) == 0 || len(buildLdflags) == 0 || o.Ldflags[0] != buildLdflags[0] {
		t.Errorf("the linux/%s override's ldflags %q do not open with the build's own %q", o.Goarch, o.Ldflags, buildLdflags)
	}
}
