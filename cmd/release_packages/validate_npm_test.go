// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// validate_npm_test.go holds the structural tier of the validate-npm
// subcommand to the checks scripts/validate-npm.mjs made, over packages the
// npm subcommand built from stand-in binaries and packed by a stand-in for
// npm pack, which writes the tarball npm would from the directory: every
// file below package/, with its mode.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakePack tars and gzips dir as npm pack does, below package/ and with
// each file's mode. Windows reports no execute bit on a regular file, so
// there the archive gives the binary 0755 and every other file 0644, the
// modes the npm subcommand wrote and the host could not keep.
func fakePack(_ *validation, dir, dest string) (string, error) {
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	err := filepath.WalkDir(dir, func(file string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(file) //#nosec G122 -- a package directory the test built
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, file)
		mode := int64(info.Mode().Perm())
		if runtime.GOOS == "windows" {
			mode = 0o644
			if name := filepath.Base(file); name == binaryName || name == binaryName+".exe" {
				mode = 0o755
			}
		}
		if stepErr := tw.WriteHeader(&tar.Header{Name: "package/" + filepath.ToSlash(rel), Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); stepErr != nil {
			return stepErr
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		return "", err
	}
	if stepErr := errors.Join(tw.Close(), gz.Close()); stepErr != nil {
		return "", stepErr
	}
	tgz := filepath.Join(dest, filepath.Base(dir)+".tgz")
	return tgz, os.WriteFile(tgz, out.Bytes(), 0o600)
}

// npmFixture builds the npm distribution of a release tree.
func npmFixture(t *testing.T) string {
	t.Helper()
	root := releaseTree(t)
	build(t, root, "npm", "--binaries", "dist", "--version", "9.8.7")
	return root
}

// validateNpmFixture runs the structural checks and returns the problems.
func validateNpmFixture(t *testing.T, root string) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	v := newValidation("validate-npm", root, "9.8.7", testFloor, &stdout, &stderr)
	n := &npmValidation{validation: v, packages: filepath.Join(root, npmPackageDir), main: filepath.Join(root, npmMainDir), work: t.TempDir(), pack: fakePack}
	n.structural()
	return v.problems
}

// TestValidateNpm_TheBuiltPackages_Validate verifies that the seven
// packages the npm subcommand writes pass every structural check.
func TestValidateNpm_TheBuiltPackages_Validate(t *testing.T) {
	t.Parallel()

	if problems := validateNpmFixture(t, npmFixture(t)); len(problems) != 0 {
		t.Errorf("problems %q, want none", problems)
	}
}

// editPackageJSON rewrites one package.json member.
func editPackageJSON(t *testing.T, file, key string, value any) {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var pkg map[string]any
	if stepErr := json.Unmarshal(data, &pkg); stepErr != nil {
		t.Fatal(stepErr)
	}
	if value == nil {
		delete(pkg, key)
	} else {
		pkg[key] = value
	}
	data, err = json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if stepErr := os.WriteFile(file, data, 0o600); stepErr != nil {
		t.Fatal(stepErr)
	}
}

// TestValidateNpm_EachDefect_IsNamed plants each defect the script checked
// for, one at a time, and requires a problem naming it.
func TestValidateNpm_EachDefect_IsNamed(t *testing.T) {
	t.Parallel()

	linux := releasePlatforms[0]
	platformDir := func(root string, p releasePlatform) string { return filepath.Join(root, npmPackageDir, p.npmKey) }
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, root string)
		want   string
	}{
		{name: "another name", damage: func(t *testing.T, root string) {
			t.Helper()
			editPackageJSON(t, filepath.Join(platformDir(root, linux), "package.json"), "name", "other")
		}, want: "linux-x64: name is other"},
		{name: "another version", damage: func(t *testing.T, root string) {
			t.Helper()
			editPackageJSON(t, filepath.Join(platformDir(root, linux), "package.json"), "version", "1.0.0")
		}, want: "linux-x64: version 1.0.0, want 9.8.7"},
		{name: "another os", damage: func(t *testing.T, root string) {
			t.Helper()
			editPackageJSON(t, filepath.Join(platformDir(root, linux), "package.json"), "os", []string{"darwin"})
		}, want: "linux-x64: os"},
		{name: "another cpu", damage: func(t *testing.T, root string) {
			t.Helper()
			editPackageJSON(t, filepath.Join(platformDir(root, linux), "package.json"), "cpu", []string{"arm64"})
		}, want: "linux-x64: cpu"},
		{name: "no libc", damage: func(t *testing.T, root string) {
			t.Helper()
			editPackageJSON(t, filepath.Join(platformDir(root, linux), "package.json"), "libc", nil)
		}, want: "linux-x64: libc"},
		{name: "a package.json that is not JSON", damage: func(t *testing.T, root string) {
			t.Helper()
			writeFile(t, platformDir(root, linux), "package.json", []byte("{"))
		}, want: "linux-x64: package.json cannot be read"},
		{name: "the notices missing", damage: func(t *testing.T, root string) {
			t.Helper()
			if err := os.Remove(filepath.Join(platformDir(root, linux), noticesName)); err != nil {
				t.Fatal(err)
			}
		}, want: "linux-x64: the tarball ships"},
		{name: "another license", damage: func(t *testing.T, root string) {
			t.Helper()
			writeFile(t, platformDir(root, linux), licensePath, []byte("Not the MIT License\n"))
		}, want: "LICENSE in the tarball is not the repository's LICENSE"},
		{name: "other notices", damage: func(t *testing.T, root string) {
			t.Helper()
			writeFile(t, platformDir(root, linux), noticesName, []byte(noticesHeader+"\nanother body\n"))
		}, want: "THIRD_PARTY_NOTICES is sha256"},
		{name: "the execute bit lost", damage: func(t *testing.T, root string) {
			t.Helper()
			if runtime.GOOS == "windows" {
				t.Skip("Windows keeps no execute bit to lose, and fakePack restores the binary's")
			}
			if err := os.Chmod(filepath.Join(platformDir(root, linux), binaryName), 0o644); err != nil { //#nosec G302 -- the defect under test is a binary without its execute bit
				t.Fatal(err)
			}
		}, want: "the binary is not executable in the tarball"},
		{name: "a swapped binary", damage: func(t *testing.T, root string) {
			t.Helper()
			writeFile(t, platformDir(root, linux), binaryName, standInBinary(linux, "swapped"))
		}, want: "signed checksums.txt named"},
		{name: "a binary for another machine", damage: func(t *testing.T, root string) {
			t.Helper()
			writeFile(t, platformDir(root, linux), binaryName, standInBinary(releasePlatforms[1], "release"))
		}, want: "not for linux-amd64"},
		{name: "no binary", damage: func(t *testing.T, root string) {
			t.Helper()
			if err := os.Remove(filepath.Join(platformDir(root, linux), binaryName)); err != nil {
				t.Fatal(err)
			}
		}, want: "binary gitlab-mcp-server not in the tarball"},
		{name: "the launcher pinned to another version", damage: func(t *testing.T, root string) {
			t.Helper()
			editPackageJSON(t, filepath.Join(root, npmMainDir, "package.json"), "optionalDependencies", map[string]string{})
		}, want: "main: optionalDependency @jmrp.io/gitlab-mcp-server-linux-x64 pinned"},
		{name: "the launcher without its bin", damage: func(t *testing.T, root string) {
			t.Helper()
			editPackageJSON(t, filepath.Join(root, npmMainDir, "package.json"), "bin", nil)
		}, want: "main: bin does not point at cli.js"},
		{name: "a stray file in the launcher", damage: func(t *testing.T, root string) {
			t.Helper()
			writeFile(t, filepath.Join(root, npmMainDir), "notes.txt", []byte("scratch"))
		}, want: "main: the tarball ships"},
		{name: "an unverified build", damage: func(t *testing.T, root string) {
			t.Helper()
			writeFile(t, filepath.Join(root, npmPackageDir), verifiedManifest, []byte(`{"version":"9.8.7","verified":false,"notices":"x"}`))
		}, want: "packaged with --allow-unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := npmFixture(t)
			tc.damage(t, root)
			wantProblem(t, validateNpmFixture(t, root), tc.want)
		})
	}
}

// TestValidateNpm_APackerThatFails_IsAProblem covers npm pack failing and a
// tarball that cannot be read.
func TestValidateNpm_APackerThatFails_IsAProblem(t *testing.T) {
	t.Parallel()

	root := npmFixture(t)
	for _, tc := range []struct {
		name string
		pack func(*validation, string, string) (string, error)
		want string
	}{
		{name: "pack fails", pack: func(*validation, string, string) (string, error) { return "", errors.New("npm pack failed") }, want: "npm pack failed"},
		{name: "not a tarball", pack: func(_ *validation, _, dest string) (string, error) {
			file := filepath.Join(dest, "x.tgz")
			return file, os.WriteFile(file, []byte("not gzip"), 0o600)
		}, want: "cannot be read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			v := newValidation("validate-npm", root, "9.8.7", testFloor, &stdout, &stderr)
			n := &npmValidation{validation: v, packages: filepath.Join(root, npmPackageDir), main: filepath.Join(root, npmMainDir), work: t.TempDir(), pack: tc.pack}
			n.structural()
			wantProblem(t, v.problems, tc.want)
		})
	}
}

// TestNpmPack_ReadsTheTarballNpmNames covers the real packer's reading of
// npm pack's answer: the last line it prints is the tarball's name.
func TestNpmPack_ReadsTheTarballNpmNames(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	v := newValidation("validate-npm", t.TempDir(), "9.8.7", 1, &stdout, &stderr)
	v.run = func(_ string, _ []string, name string, args ...string) ([]byte, error) {
		if name != "npm" || strings.Join(args, " ") != "pack --pack-destination /dest --silent" {
			return nil, errors.New("unexpected " + name + " " + strings.Join(args, " "))
		}
		return []byte("npm notice\nx-9.8.7.tgz\n"), nil
	}
	got, err := npmPack(v, "/pkg", "/dest")
	if err != nil || got != filepath.Join("/dest", "x-9.8.7.tgz") {
		t.Errorf("npmPack = %q, %v, want /dest/x-9.8.7.tgz", got, err)
	}
	v.run = func(string, []string, string, ...string) ([]byte, error) { return []byte("E404"), errors.New("exit 1") }
	if _, packErr := npmPack(v, "/pkg", "/dest"); packErr == nil || !strings.Contains(packErr.Error(), "E404") {
		t.Errorf("npmPack over a failing npm = %v, want its output named", packErr)
	}
}

// TestRunValidateNpm_ReportsTheOutcome covers the subcommand around the
// checks: with --no-install it runs the structural tier alone and says so.
func TestRunValidateNpm_ReportsTheOutcome(t *testing.T) {
	t.Parallel()

	root := npmFixture(t)
	var stdout, stderr bytes.Buffer
	code := runIn(root, []string{"validate-npm", "--version", "9.8.7", "--no-install"}, &stdout, &stderr, noEnv)
	if code != 1 || !strings.Contains(stdout.String(), "runtime: skipped (--no-install)") {
		t.Errorf("exit %d, stdout %q, want 1 over stand-ins and npm missing or below the floor, and the skip said", code, stdout.String())
	}
}
