// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// validate_nuget_test.go holds the validate-nuget subcommand to the checks
// scripts/validate_nuget.py made: the packages the nuget subcommand writes
// validate, each defect its suite planted is named, and the signable layout
// check names each damaged record it reads.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// nugetFixture builds the packages of a release tree and returns the tree
// and the package directory.
func nugetFixture(t *testing.T) (root, dir string) {
	t.Helper()
	root = releaseTree(t)
	build(t, root, "nuget", "--binaries", "dist", "--version", "9.8.7")
	return root, filepath.Join(root, nugetOutDir)
}

// validateNugetFixture runs the offline checks and returns the problems.
func validateNugetFixture(t *testing.T, root, dir string) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	v := newValidation("validate-nuget", root, "9.8.7", testFloor, &stdout, &stderr)
	v.validateNuget(dir)
	return v.problems
}

// TestValidateNuget_TheBuiltPackages_Validate verifies that the seven
// packages the nuget subcommand writes pass every offline check, the
// signable layout included, and that the record of what was verified is
// taken out of the directory the publish step pushes.
func TestValidateNuget_TheBuiltPackages_Validate(t *testing.T) {
	t.Parallel()

	root, dir := nugetFixture(t)
	if problems := validateNugetFixture(t, root, dir); len(problems) != 0 {
		t.Errorf("problems %q, want none", problems)
	}
	if _, err := os.Stat(filepath.Join(dir, verifiedManifest)); err == nil {
		t.Error("verified-binaries.json is still beside the packages")
	}
}

// TestValidateNuget_EachDefect_IsReported plants each defect the script's
// suite planted, one at a time, and requires a problem naming it.
func TestValidateNuget_EachDefect_IsReported(t *testing.T) {
	t.Parallel()

	pointer := nugetPackageName(nugetID, "9.8.7")
	rid := func(name string) string { return nugetPackageName(nugetID+"."+name, "9.8.7") }
	settings := "tools/" + nugetToolFramework + "/any/DotnetToolSettings.xml"
	for _, tc := range []struct {
		name    string
		pkg     string
		edit    func([]zipFixtureEntry) []zipFixtureEntry
		damage  func(t *testing.T, root, dir string)
		want    string
		streams bool
	}{
		{name: "a swapped binary", pkg: rid("linux-x64"), edit: replaceEntry("tools/any/linux-x64/gitlab-mcp-server", standInBinary(releasePlatforms[0], "swapped")), want: "signed checksums.txt named"},
		{name: "a binary for another machine", pkg: rid("linux-arm64"), edit: replaceEntry("tools/any/linux-arm64/gitlab-mcp-server", standInBinary(releasePlatforms[0], "release")), want: "not for linux-arm64"},
		{name: "the README lost the token", pkg: pointer, edit: replaceEntry("README.md", []byte("# no token here\n")), want: "ownership token"},
		{name: "the token glued to a longer name", pkg: pointer, edit: replaceEntry("README.md", []byte(mcpNameToken+"-other\n")), want: "ownership token"},
		{name: "the pointer forgets a runtime identifier", pkg: pointer, edit: editEntry(settings, `    <RuntimeIdentifierPackage RuntimeIdentifier="win-arm64" Id="gitlab-mcp-server.win-arm64" />`+"\n", ""), want: "RuntimeIdentifierPackages"},
		{name: "the pointer names an entry point", pkg: pointer, edit: editEntry(settings, `<Command Name="gitlab-mcp-server" />`, `<Command Name="gitlab-mcp-server" EntryPoint="x" />`), want: "with no entry point"},
		{name: "settings of another version", pkg: pointer, edit: editEntry(settings, `Version="2"`, `Version="1"`), want: "not a DotNetCliTool manifest of Version 2"},
		{name: "settings that do not parse", pkg: pointer, edit: replaceEntry(settings, []byte("<")), want: "does not parse"},
		{name: "a stale server.json", pkg: pointer, edit: editEntry(".mcp/server.json", `"version": "9.8.7",`, `"version": "0.0.1",`), want: ".mcp/server.json carries version"},
		{name: "a server.json that is not JSON", pkg: pointer, edit: replaceEntry(".mcp/server.json", []byte("{")), want: ".mcp/server.json is not JSON"},
		{name: "no server.json", pkg: pointer, edit: dropEntry(".mcp/server.json"), want: "no .mcp/server.json"},
		{name: "the license URL dropped", pkg: pointer, edit: editEntry(nugetID+".nuspec", "    <licenseUrl>"+nugetLicenseURL+"</licenseUrl>\n", ""), want: "licenseUrl"},
		{name: "another id", pkg: pointer, edit: editEntry(nugetID+".nuspec", "<id>gitlab-mcp-server</id>", "<id>other</id>"), want: `nuspec id is "other"`},
		{name: "another package type", pkg: pointer, edit: editEntry(nugetID+".nuspec", `"McpServer"`, `"Other"`), want: "nuspec packageTypes are"},
		{name: "no authors", pkg: pointer, edit: editEntry(nugetID+".nuspec", "    <authors>jmrplens</authors>\n", ""), want: "nuspec lacks <authors>"},
		{name: "no readme declared", pkg: pointer, edit: editEntry(nugetID+".nuspec", "    <readme>README.md</readme>\n", ""), want: "declares no <readme>"},
		{name: "an icon declared and missing", pkg: pointer, edit: dropEntry("icon.png"), want: "<icon> names icon.png"},
		{name: "a nuspec that does not parse", pkg: pointer, edit: replaceEntry(nugetID+".nuspec", []byte("<")), want: "does not parse"},
		{name: "a nuspec outside the namespace", pkg: pointer, edit: editEntry(nugetID+".nuspec", nugetNuspecNS, "urn:other"), want: "no <metadata> in the nuspec namespace"},
		{name: "no nuspec", pkg: pointer, edit: dropEntry(nugetID + ".nuspec"), want: "gitlab-mcp-server.nuspec missing"},
		{name: "no content types", pkg: pointer, edit: dropEntry("[Content_Types].xml"), want: "not an OPC container"},
		{name: "no relationships", pkg: pointer, edit: dropEntry("_rels/.rels"), want: "no _rels/.rels"},
		{name: "relationships naming another nuspec", pkg: pointer, edit: editEntry("_rels/.rels", "/gitlab-mcp-server.nuspec", "/other.nuspec"), want: "does not name /gitlab-mcp-server.nuspec"},
		{name: "the pointer carries a binary", pkg: pointer, edit: func(e []zipFixtureEntry) []zipFixtureEntry {
			return append(e, zipFixtureEntry{name: "tools/gitlab-mcp-server", data: []byte("x"), mode: 0o100755})
		}, want: "the pointer carries a binary"},
		{name: "the executable bit lost", pkg: rid("osx-x64"), edit: setMode("tools/any/osx-x64/gitlab-mcp-server", 0o100644), want: "execute bit"},
		{name: "a second payload", pkg: rid("osx-x64"), edit: func(e []zipFixtureEntry) []zipFixtureEntry {
			return append(e, zipFixtureEntry{name: "tools/any/osx-x64/extra", data: []byte("x"), mode: 0o100644})
		}, want: "expected exactly tools/any/osx-x64/gitlab-mcp-server"},
		{name: "a runtime command without its runner", pkg: rid("osx-x64"), edit: editEntry("tools/any/osx-x64/DotnetToolSettings.xml", ` Runner="executable"`, ""), want: "Runner=executable"},
		{name: "no runtime settings", pkg: rid("osx-x64"), edit: dropEntry("tools/any/osx-x64/DotnetToolSettings.xml"), want: "DotnetToolSettings.xml missing"},
		{name: "a runtime package lost its license", pkg: rid("linux-arm64"), edit: dropEntry(licensePath), want: "no LICENSE at the package root"},
		{name: "another license text", pkg: pointer, edit: replaceEntry(licensePath, []byte("Not the MIT License\n")), want: "LICENSE is not the repository's LICENSE"},
		{name: "a license without its file type", pkg: rid("win-x64"), edit: setMode(licensePath, 0o644), want: "LICENSE is not a regular file (mode 644)"},
		{name: "the pointer lost the notices", pkg: pointer, edit: dropEntry(noticesName), want: "no THIRD_PARTY_NOTICES at the package root"},
		{name: "other notices", pkg: rid("osx-x64"), edit: replaceEntry(noticesName, []byte(noticesHeader+"\nanother body\n")), want: "THIRD_PARTY_NOTICES is sha256"},
		{name: "notices without the header", pkg: rid("linux-x64"), edit: replaceEntry(noticesName, []byte("some other text\n")), want: "does not open with the generator's header"},
		{name: "written with data descriptors", pkg: pointer, streams: true, want: "data descriptor"},
		{name: "a package missing", damage: func(t *testing.T, _, dir string) {
			t.Helper()
			if err := os.Remove(filepath.Join(dir, rid("osx-arm64"))); err != nil {
				t.Fatal(err)
			}
		}, want: "needs exactly"},
		{name: "a stray file", damage: func(t *testing.T, _, dir string) {
			t.Helper()
			writeFile(t, dir, "notes.txt", []byte("scratch"))
		}, want: "would try to push"},
		{name: "not a zip", damage: func(t *testing.T, _, dir string) {
			t.Helper()
			writeFile(t, dir, pointer, []byte("not a zip"))
		}, want: "not a zip archive"},
		{name: "already signed", damage: func(t *testing.T, _, dir string) {
			t.Helper()
			rewriteZip(t, filepath.Join(dir, rid("osx-arm64")), func(e []zipFixtureEntry) []zipFixtureEntry {
				return append(e, zipFixtureEntry{name: ".signature.p7s", data: []byte("sig"), mode: 0o100644})
			})
		}, want: "already carries .signature.p7s"},
		{name: "an archive comment", damage: func(t *testing.T, _, dir string) {
			t.Helper()
			file := filepath.Join(dir, rid("linux-x64"))
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, "built by hand"...)
			binary.LittleEndian.PutUint16(data[len(data)-13-2:], 13)
			writeFile(t, dir, rid("linux-x64"), data)
		}, want: "archive comment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, dir := nugetFixture(t)
			switch {
			case tc.streams:
				rewriteZip(t, filepath.Join(dir, tc.pkg), func(e []zipFixtureEntry) []zipFixtureEntry {
					for i := range e {
						e[i].streamed = true
					}
					return e
				})
			case tc.edit != nil:
				rewriteZip(t, filepath.Join(dir, tc.pkg), tc.edit)
			}
			if tc.damage != nil {
				tc.damage(t, root, dir)
			}
			wantProblem(t, validateNugetFixture(t, root, dir), tc.want)
		})
	}
}

// TestValidateNuget_AnUnverifiedBuild_IsRefused covers packages built with
// --allow-unverified.
func TestValidateNuget_AnUnverifiedBuild_IsRefused(t *testing.T) {
	t.Parallel()

	root := releaseTree(t)
	if err := os.Remove(filepath.Join(root, "dist", "checksums.txt")); err != nil {
		t.Fatal(err)
	}
	build(t, root, "nuget", "--binaries", "dist", "--version", "9.8.7", "--allow-unverified")
	wantProblem(t, validateNugetFixture(t, root, filepath.Join(root, nugetOutDir)), "packaged with --allow-unverified")
}

// TestSignableLayoutProblems_NamesEachDamagedRecord reaches the records
// archive/zip would never write, one at a time: no end record, a zip64 end,
// a central directory past the end record, a record that is not one, a
// local header that is not one, and a zip64 extra field in the local header.
func TestSignableLayoutProblems_NamesEachDamagedRecord(t *testing.T) {
	t.Parallel()

	var clean bytes.Buffer
	if err := writeRawEntries(&clean, []packageEntry{{name: "a", data: []byte("aaaa")}, {name: "b", data: []byte("bbbb")}}); err != nil {
		t.Fatal(err)
	}
	pristine := clean.Bytes()
	if problems := signableLayoutProblems(pristine); len(problems) != 0 {
		t.Fatalf("problems %q over an archive packed like the nuget subcommand packs, want none", problems)
	}
	eocd := bytes.LastIndex(pristine, []byte("PK\x05\x06"))
	cd := int(binary.LittleEndian.Uint32(pristine[eocd+16:]))
	for _, tc := range []struct {
		name   string
		damage func(data []byte) []byte
		want   string
	}{
		{name: "no end record", damage: func(data []byte) []byte { return data[:eocd] }, want: "no end-of-central-directory record"},
		{name: "a zip64 end", damage: func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[eocd+16:], 0xffffffff)
			return data
		}, want: "the archive ends in zip64 form"},
		{name: "a directory past the end", damage: func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[eocd+12:], uint32(len(data))) //#nosec G115 -- a fixture of a few hundred bytes
			return data
		}, want: "runs past the end-of-central-directory record"},
		{name: "a record that is not one", damage: func(data []byte) []byte {
			copy(data[cd:], "XXXX")
			return data
		}, want: "no central directory record at offset"},
		{name: "a local header that is not one", damage: func(data []byte) []byte {
			copy(data, "XXXX")
			return data
		}, want: "names a local header at offset 0 that is not one"},
		{name: "a zip64 offset", damage: func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[cd+42:], 0xffffffff)
			return data
		}, want: "a is stored in zip64 form"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			problems := signableLayoutProblems(tc.damage(bytes.Clone(pristine)))
			wantProblem(t, problems, tc.want)
		})
	}
	t.Run("a zip64 extra in the local header", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(t.TempDir(), "p.nupkg")
		if err := os.WriteFile(file, pristine, 0o600); err != nil {
			t.Fatal(err)
		}
		rewriteZip(t, file, func(e []zipFixtureEntry) []zipFixtureEntry {
			e[0].extra = []byte{0x01, 0x00, 0x00, 0x00}
			return e
		})
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		wantProblem(t, signableLayoutProblems(data), "a is stored in zip64 form")
	})
}

// TestExtraFieldIDs_ReadsEveryHeaderAndStopsAtAShortTail covers the extra
// field reader.
func TestExtraFieldIDs_ReadsEveryHeaderAndStopsAtAShortTail(t *testing.T) {
	t.Parallel()

	extra := []byte{0x01, 0x00, 0x02, 0x00, 0xaa, 0xbb, 0x55, 0x54, 0x00, 0x00, 0x09}
	if got := extraFieldIDs(extra); len(got) != 2 || got[0] != 0x0001 || got[1] != 0x5455 {
		t.Errorf("extraFieldIDs = %#x, want [0x1 0x5455]", got)
	}
	if !strings.Contains(strings.Join(validateNugetCommandLine(), " "), "--no-install") {
		t.Error("the usage does not name --no-install")
	}
}

// validateNugetCommandLine is the usage line of validate-nuget.
func validateNugetCommandLine() []string {
	for line := range strings.SplitSeq(usage, "\n") {
		if strings.Contains(line, "validate-nuget") {
			return strings.Fields(line)
		}
	}
	return nil
}

// TestValidateNuget_Install_DrivesTheShimAndDnx runs the install tier
// against a stand-in dotnet: dotnet tool install lays the shim out, and the
// shim and dnx, each the stand-in server here, answer the handshake. A
// failing install and one that leaves no shim are problems.
func TestValidateNuget_Install_DrivesTheShimAndDnx(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the stand-in dotnet is a shell script")
	}
	for _, tc := range []struct {
		name, fail, want string
	}{
		{name: "both answer"},
		{name: "the install fails", fail: "install", want: "dotnet tool install failed"},
		{name: "no shim", fail: "shim", want: "dotnet tool install left no gitlab-mcp-server"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, dir := nugetFixture(t)
			var stdout, stderr bytes.Buffer
			v := newValidation("validate-nuget", root, "9.8.7", testFloor, &stdout, &stderr)
			v.dotnet = filepath.Join(t.TempDir(), "dotnet")
			writeScript(t, v.dotnet, standInServer(t))
			v.run = func(_ string, _ []string, _ string, args ...string) ([]byte, error) {
				switch tc.fail {
				case "install":
					return []byte("NU1101"), errors.New("exit 1")
				case "shim":
					return nil, nil
				}
				tools := args[slices.Index(args, "--tool-path")+1]
				writeScript(t, filepath.Join(tools, binaryName), standInServer(t))
				return nil, nil
			}
			v.nugetHandshake(dir)
			if tc.want != "" {
				wantProblem(t, v.problems, tc.want)
				return
			}
			if len(v.problems) != 0 {
				t.Fatalf("problems %q, want none", v.problems)
			}
			for _, want := range []string{"handshake: dotnet tool install answered", "handshake: dnx answered"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout %q, want %q", stdout.String(), want)
				}
			}
		})
	}
}

// TestNugetInstall_WithoutDotnet_IsAProblem covers the install tier on a
// machine with no .NET SDK, over packages the offline checks pass: the one
// problem is the missing SDK, said with how to proceed, and nothing is run.
// It also holds the subcommand's verdict over stand-ins below the real
// floor.
func TestNugetInstall_WithoutDotnet_IsAProblem(t *testing.T) {
	t.Parallel()

	root, dir := nugetFixture(t)
	var stdout, stderr bytes.Buffer
	v := newValidation("validate-nuget", root, "9.8.7", testFloor, &stdout, &stderr)
	v.dotnet = filepath.Join(t.TempDir(), "no-dotnet")
	v.run = func(string, []string, string, ...string) ([]byte, error) {
		t.Error("the install tier ran a program although there is no dotnet")
		return nil, errors.New("not run")
	}
	v.validateNuget(dir)
	if len(v.problems) != 0 {
		t.Fatalf("problems %q before the install tier, want none", v.problems)
	}
	v.nugetInstall(dir)
	if len(v.problems) != 1 || !strings.Contains(v.problems[0], "dotnet is not on PATH; install the .NET 10 SDK or pass --no-install") {
		t.Errorf("problems %q, want the missing SDK said once", v.problems)
	}
	var out, errOut bytes.Buffer
	if code := runIn(root, []string{"validate-nuget", "--version", "9.8.7", "--no-install"}, &out, &errOut, noEnv); code != 1 {
		t.Errorf("exit %d over stand-ins below the real floor, want 1", code)
	}
}
