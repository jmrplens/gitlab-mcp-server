// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// validate_pypi_test.go holds the validate-pypi subcommand to the checks
// scripts/validate_pypi.py made: the wheels the pypi subcommand writes
// validate, and each departure its suite planted is named, over wheels built
// from stand-in binaries the binary checks can read.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pypiFixture builds the wheelhouse of a release tree and returns the tree
// and the wheelhouse.
func pypiFixture(t *testing.T, args ...string) (root, wheels string) {
	t.Helper()
	root = releaseTree(t)
	build(t, root, append([]string{"pypi", "--binaries", "dist", "--version", "9.8.7"}, args...)...)
	return root, filepath.Join(root, pypiOutDir)
}

// validatePypiFixture runs the offline checks over a wheelhouse, with twine
// reported as not installed, and returns the problems.
func validatePypiFixture(t *testing.T, root, wheels string) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	v := newValidation("validate-pypi", root, "9.8.7", testFloor, &stdout, &stderr)
	v.run = func(string, []string, string, ...string) ([]byte, error) { return nil, errors.New("no twine") }
	v.validatePypi(wheels, false)
	return v.problems
}

// TestValidatePypi_TheBuiltWheels_Validate verifies that the six wheels the
// pypi subcommand writes pass every offline check, and that the record of
// what was verified is taken out of the wheelhouse once read, which the
// publish step would refuse.
func TestValidatePypi_TheBuiltWheels_Validate(t *testing.T) {
	t.Parallel()

	root, wheels := pypiFixture(t)
	if problems := validatePypiFixture(t, root, wheels); len(problems) != 0 {
		t.Errorf("problems %q, want none", problems)
	}
	if _, err := os.Stat(filepath.Join(wheels, verifiedManifest)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("verified-binaries.json is still in the wheelhouse (%v)", err)
	}
}

// TestValidatePypi_EachDeparture_IsNamed plants each defect the script's
// suite planted, one at a time, and requires a problem naming it.
func TestValidatePypi_EachDeparture_IsNamed(t *testing.T) {
	t.Parallel()

	distInfo := pypiNorm + "-9.8.7.dist-info"
	metadata, licenses, record := distInfo+"/METADATA", distInfo+"/licenses/", distInfo+"/RECORD"
	linux := releasePlatforms[0]
	wheel := pypiWheelName("9.8.7", linux.wheelTag)
	binaryEntry := pypiNorm + "-9.8.7.data/scripts/" + binaryName
	for _, tc := range []struct {
		name   string
		edit   func([]zipFixtureEntry) []zipFixtureEntry
		before func(t *testing.T, root, wheels string)
		want   string
	}{
		{name: "metadata 2.1", edit: editEntry(metadata, "Metadata-Version: 2.4", "Metadata-Version: 2.1"), want: `Metadata-Version is "2.1"`},
		{name: "another expression", edit: editEntry(metadata, "License-Expression: MIT", "License-Expression: Apache-2.0"), want: `License-Expression is "Apache-2.0"`},
		{name: "legacy field", edit: editEntry(metadata, "License-Expression: MIT", "License-Expression: MIT\nLicense: MIT"), want: "legacy License field"},
		{name: "license classifier", edit: editEntry(metadata, "Classifier: Development", "Classifier: License :: OSI Approved :: MIT License\nClassifier: Development"), want: "License :: OSI Approved :: MIT License"},
		{name: "no License-File for the license", edit: editEntry(metadata, "License-File: LICENSE\n", ""), want: `License-File declares ["THIRD_PARTY_NOTICES"]`},
		{name: "declared and not shipped", edit: dropEntry(licenses + "LICENSE"), want: `holds ["THIRD_PARTY_NOTICES"] but METADATA declares ["LICENSE" "THIRD_PARTY_NOTICES"]`},
		{name: "shipped and not declared", edit: func(e []zipFixtureEntry) []zipFixtureEntry {
			return append(e, zipFixtureEntry{name: licenses + "EXTRA", data: []byte("extra"), mode: 0o100644})
		}, want: `holds ["EXTRA" "LICENSE" "THIRD_PARTY_NOTICES"]`},
		{name: "another license text", edit: replaceEntry(licenses+"LICENSE", []byte("Not the MIT License\n")), want: "is not the repository's LICENSE"},
		{name: "notices without the header", edit: replaceEntry(licenses+noticesName, []byte("some other text\n")), want: "does not open with the generator's header"},
		{name: "other notices", edit: replaceEntry(licenses+noticesName, []byte(noticesHeader+"\nanother body\n")), want: "THIRD_PARTY_NOTICES is sha256"},
		{name: "RECORD without its type bits", edit: setMode(record, 0o644), want: "RECORD is not a regular file (mode 644)"},
		{name: "no Issues URL", edit: editEntry(metadata, "Project-URL: Issues, ", "Project-URL: Tracker, "), want: `Project-URL "Issues" is ""`},
		{name: "another Security URL", edit: editEntry(metadata, "security/policy", "security"), want: `Project-URL "Security" is "https://github.com/jmrplens/gitlab-mcp-server/security"`},
		{name: "a RECORD that lies", edit: editEntry(metadata, "Author: jmrplens", "Author: someone"), want: "RECORD mismatch for " + metadata},
		{name: "the ownership token lost", edit: editEntry(metadata, mcpNameToken, "mcp-name: other"), want: "lost the MCP Registry ownership token"},
		{name: "another tag", edit: editEntry(distInfo+"/WHEEL", "Tag: py3-none-", "Tag: py2-none-"), want: "WHEEL tag disagrees"},
		{name: "purelib", edit: editEntry(distInfo+"/WHEEL", "Root-Is-Purelib: false", "Root-Is-Purelib: true"), want: "must be platlib"},
		{name: "no console script", edit: editEntry(distInfo+"/entry_points.txt", pypiDist+" =", "other ="), want: "console script named after the distribution"},
		{name: "the binary lost its execute bit", edit: setMode(binaryEntry, 0o100644), want: "execute bit"},
		{name: "a swapped binary", edit: replaceEntry(binaryEntry, standInBinary(linux, "swapped")), want: "signed checksums.txt named"},
		{name: "a binary for another machine", edit: replaceEntry(binaryEntry, standInBinary(releasePlatforms[1], "release")), want: "not for linux-amd64"},
		{name: "a binary past the glibc floor", edit: replaceEntry(binaryEntry, append(standInBinary(linux, "release"), "GLIBC_2.34"...)), want: "demands glibc 2.34"},
		{name: "no binary", edit: dropEntry(binaryEntry), want: "bundled binary " + binaryEntry + " missing"},
		{name: "a wheel missing", before: func(t *testing.T, _, wheels string) {
			t.Helper()
			if err := os.Remove(filepath.Join(wheels, wheel)); err != nil {
				t.Fatal(err)
			}
		}, want: "the release needs exactly"},
		{name: "a stray file", before: func(t *testing.T, _, wheels string) {
			t.Helper()
			writeFile(t, wheels, "notes.txt", []byte("scratch"))
		}, want: "files other than wheels"},
		{name: "not a zip", before: func(t *testing.T, _, wheels string) {
			t.Helper()
			writeFile(t, wheels, wheel, []byte("not a zip"))
		}, want: "not a zip archive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, wheels := pypiFixture(t)
			if tc.edit != nil {
				rewriteZip(t, filepath.Join(wheels, wheel), tc.edit)
			}
			if tc.before != nil {
				tc.before(t, root, wheels)
			}
			wantProblem(t, validatePypiFixture(t, root, wheels), tc.want)
		})
	}
}

// TestValidatePypi_AnUnverifiedBuild_IsRefused covers a wheelhouse built
// with --allow-unverified.
func TestValidatePypi_AnUnverifiedBuild_IsRefused(t *testing.T) {
	t.Parallel()

	root := releaseTree(t)
	if err := os.Remove(filepath.Join(root, "dist", "checksums.txt")); err != nil {
		t.Fatal(err)
	}
	build(t, root, "pypi", "--binaries", "dist", "--version", "9.8.7", "--allow-unverified")
	wantProblem(t, validatePypiFixture(t, root, filepath.Join(root, pypiOutDir)), "packaged with --allow-unverified")
}

// TestValidatePypi_Twine_RunsWhenInstalledAndItsRefusalIsAProblem covers
// twine: skipped and said so where it is not installed, run strict where it
// is, and its refusal a problem.
func TestValidatePypi_Twine_RunsWhenInstalledAndItsRefusalIsAProblem(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		installed     bool
		refuses       bool
		stdout, want  string
		wantArguments string
	}{
		{name: "not installed", stdout: "twine: not installed here"},
		{name: "passes", installed: true, stdout: "twine: check --strict passed for 1 wheels", wantArguments: "-m twine check --strict w.whl"},
		{name: "refuses", installed: true, refuses: true, want: "twine check --strict refused the wheels"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			var ran []string
			v := newValidation("validate-pypi", t.TempDir(), "9.8.7", 1, &stdout, &stderr)
			v.run = func(_ string, _ []string, name string, args ...string) ([]byte, error) {
				ran = append(ran, strings.Join(args, " "))
				if args[0] == "-c" && !tc.installed || args[0] == "-m" && tc.refuses {
					return []byte("refused"), errors.New("exit 1")
				}
				return nil, nil
			}
			v.twineCheck([]string{"w.whl"})
			if !strings.Contains(stdout.String(), tc.stdout) {
				t.Errorf("stdout %q, want %q", stdout.String(), tc.stdout)
			}
			if tc.want != "" {
				wantProblem(t, v.problems, tc.want)
			}
			if tc.wantArguments != "" && ran[len(ran)-1] != tc.wantArguments {
				t.Errorf("twine ran with %q, want %q", ran[len(ran)-1], tc.wantArguments)
			}
		})
	}
}

// TestValidatePypi_Install_DrivesBothConsoleScripts runs the install tier
// against a stand-in python3 and pip: the virtual environment is made, the
// wheel for this host installed, and both console scripts, the stand-in
// server here, answer the handshake. A failing venv or pip is a problem.
func TestValidatePypi_Install_DrivesBothConsoleScripts(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the stand-in console scripts are shell scripts")
	}
	if _, ok := hostPlatform(); !ok {
		t.Skip("no release platform for this host")
	}
	for _, tc := range []struct {
		name, fail, want string
	}{
		{name: "both scripts answer"},
		{name: "venv fails", fail: "venv", want: "python3 -m venv failed"},
		{name: "pip fails", fail: "pip", want: "pip install failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, wheels := pypiFixture(t)
			var stdout, stderr bytes.Buffer
			v := newValidation("validate-pypi", root, "9.8.7", testFloor, &stdout, &stderr)
			v.run = standInPython(t, tc.fail)
			v.validatePypi(wheels, true)
			if tc.want != "" {
				wantProblem(t, v.problems, tc.want)
				return
			}
			if len(v.problems) != 0 {
				t.Fatalf("problems %q, want none", v.problems)
			}
			for _, command := range []string{binaryName, pypiDist} {
				if want := "handshake: " + command + " answered initialize with version 9.8.7"; !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout %q, want %q", stdout.String(), want)
				}
			}
		})
	}
}

// standInPython stands in for python3 and the virtual environment's pip:
// twine is not installed, venv makes the scripts directory, and pip lays out
// both console scripts as the stand-in server. fail names the step that
// fails instead, venv or pip.
func standInPython(t *testing.T, fail string) func(string, []string, string, ...string) ([]byte, error) {
	t.Helper()
	return func(_ string, _ []string, name string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "-c":
			return nil, errors.New("no twine")
		case args[0] == "-m" && args[1] == "venv":
			if fail == "venv" {
				return []byte("no ensurepip"), errors.New("exit 1")
			}
			return nil, os.MkdirAll(filepath.Join(args[2], "bin"), 0o750)
		case filepath.Base(name) == "pip":
			if fail == "pip" {
				return []byte("no matching distribution"), errors.New("exit 1")
			}
			for _, command := range []string{binaryName, pypiDist} {
				writeScript(t, filepath.Join(filepath.Dir(name), command), standInServer(t))
			}
			return nil, nil
		}
		return nil, errors.New("unexpected " + name)
	}
}

// TestRunValidatePypi_ReportsTheOutcome covers the subcommand around the
// checks: a wheelhouse that passes exits 0 and says so, and one that does
// not exits 1 with the count.
func TestRunValidatePypi_ReportsTheOutcome(t *testing.T) {
	t.Parallel()

	root, _ := pypiFixture(t)
	var stdout, stderr bytes.Buffer
	if code := runIn(root, []string{"validate-pypi", "--version", "9.8.7", "--no-install"}, &stdout, &stderr, noEnv); code != 1 {
		t.Errorf("exit %d over stand-ins below the real floor, want 1", code)
	}
	if !strings.Contains(stderr.String(), "release_packages validate-pypi: ") || !strings.Contains(stderr.String(), "failure(s)") {
		t.Errorf("stderr %q, want the failure count", stderr.String())
	}
	var ok bytes.Buffer
	v := newValidation("validate-pypi", root, "9.8.7", testFloor, &ok, &stderr)
	if code := v.finish("all wheels"); code != 0 || !strings.Contains(ok.String(), "validate-pypi: all wheels valid for version 9.8.7") {
		t.Errorf("finish = %d, %q, want 0 and the verdict", code, ok.String())
	}
}
