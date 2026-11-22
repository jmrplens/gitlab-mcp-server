// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// packages_test.go holds the npm, pypi and nuget subcommands to what
// scripts/build-npm.mjs, scripts/build_pypi.py and scripts/build_nuget.py
// did.
//
// testdata/build_packages.json is the record. Each run laid out a scratch
// repository root holding the repository files the scripts read (frozen in
// the record, so a release stamp or a README edit does not move the outputs),
// a stand-in icon and a dist/ of stand-in release assets with their
// checksums.txt, changed by the run's ops, then ran one script at 1c26b3b04,
// the revision of main this port started from. For each run the record keeps
// the exit status, stdout, stderr and every file the run created or changed
// under npm/, pypi/ and nuget/: its mode, size and SHA-256, and for a wheel
// or a NuGet package every entry, in order, with its mode, creator system,
// time, method, data descriptor flag, extra field, size and SHA-256.
//
// Each run is repeated here over the same tree. The packages are equal to the
// script's file for file and entry for entry, which is what the port is held
// to: the archive around the entries is archive/zip's rather than Python's
// zipfile's (other deflated bytes), as writePackageZip says. The differences
// held on purpose are named where they are compared: messages carry no em
// dash, a refused run writes nothing at all, an argument missing is a usage
// error, and the wheels name this command as their generator.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

// packagesRecord is testdata/build_packages.json.
type packagesRecord struct {
	RecordedFrom string            `json:"recorded_from"`
	Version      string            `json:"version"`
	Assets       []string          `json:"assets"`
	RepoFiles    map[string]string `json:"repo_files"`
	Icon         string            `json:"icon"`
	Runs         []packagesRun     `json:"runs"`
}

// packagesRun is one recorded run of a builder.
type packagesRun struct {
	Name   string                    `json:"name"`
	Tool   string                    `json:"tool"`
	Args   []string                  `json:"args"`
	Ops    []treeOp                  `json:"ops"`
	Exit   int                       `json:"exit"`
	Stdout string                    `json:"stdout"`
	Stderr string                    `json:"stderr"`
	Files  map[string]recordedOutput `json:"files"`
}

// recordedOutput is one file a run left.
type recordedOutput struct {
	Partial bool            `json:"partial"`
	Mode    string          `json:"mode"`
	Size    int             `json:"size"`
	SHA256  string          `json:"sha256"`
	Text    *string         `json:"text"`
	Entries []recordedEntry `json:"entries"`
}

// recordedEntry is one entry of a wheel or a NuGet package as Python's
// zipfile read it.
type recordedEntry struct {
	Name           string  `json:"name"`
	Mode           string  `json:"mode"`
	CreateSystem   uint16  `json:"create_system"`
	DateTime       string  `json:"date_time"`
	CompressType   uint16  `json:"compress_type"`
	DataDescriptor bool    `json:"data_descriptor"`
	Extra          string  `json:"extra"`
	Size           int     `json:"size"`
	SHA256         string  `json:"sha256"`
	Text           *string `json:"text"`
}

// packagesDifferences are the recorded runs whose exit status or message the
// command is not held to repeating, each with the stderr it writes instead
// and its exit status.
var packagesDifferences = map[string]struct {
	exit   int
	stderr string
}{
	// A missing flag is a usage error, where node threw.
	"npm: refuses a run without binaries": {2, "release_packages npm: --binaries <dir> is required (or pass --sync-only)\n"},
	// Every asset is read before any package is written, and an absent
	// binary is named, where node threw its ENOENT.
	"npm: refuses a missing binary": {1, "release_packages: missing release binary dist/gitlab-mcp-server-windows-amd64.exe\n"},
	// The license is named by its path in the repository, where the
	// scripts printed the path they built from their own location.
	"npm: refuses a missing LICENSE":   {1, "release_packages: license file LICENSE not found at LICENSE\n"},
	"pypi: refuses a missing LICENSE":  {1, "release_packages: license file LICENSE not found at LICENSE\n"},
	"nuget: refuses a missing LICENSE": {1, "release_packages: license file LICENSE not found at LICENSE\n"},
	// An asset checksums.txt does not list is named with the file's path,
	// as build-npm.mjs named it, where the Python builders named the file
	// alone.
	"pypi: refuses a binary checksums.txt does not list":  {1, "release_packages: gitlab-mcp-server-windows-amd64.exe is not listed in dist/checksums.txt\n"},
	"nuget: refuses a binary checksums.txt does not list": {1, "release_packages: gitlab-mcp-server-windows-amd64.exe is not listed in dist/checksums.txt\n"},
}

// packageStandIn is the bytes a scratch dist/ holds for a release asset,
// different for each so a package packed from the wrong one is told apart;
// the notices open with the generator's header.
func packageStandIn(name string) []byte {
	body := "stand-in for dist/" + name + strings.Repeat("\n", 64)
	if name == noticesName {
		return []byte(noticesHeader + "\n\n" + body)
	}
	return []byte(body)
}

// layOutPackagesTree writes the scratch root a recorded run started from.
func layOutPackagesTree(t *testing.T, record *packagesRecord, ops []treeOp) string {
	t.Helper()
	root := t.TempDir()
	for rel, text := range record.RepoFiles {
		writeFile(t, root, rel, []byte(text))
	}
	icon, err := base64.StdEncoding.DecodeString(record.Icon)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, iconPath, icon)
	var sums strings.Builder
	for _, name := range append(slices.Clone(record.Assets), noticesName) {
		data := packageStandIn(name)
		writeFile(t, root, "dist/"+name, data)
		fmt.Fprintf(&sums, "%s  %s\n", sha256Hex(data), name)
	}
	writeFile(t, root, "dist/checksums.txt", []byte(sums.String()))
	for _, op := range ops {
		file := filepath.Join(root, filepath.FromSlash(op.Path))
		switch op.Op {
		case "remove":
			if stepErr := os.Remove(file); stepErr != nil {
				t.Fatal(stepErr)
			}
		case "write":
			writeFile(t, root, op.Path, []byte(op.Content))
		case "append":
			data, readErr := os.ReadFile(file)
			if readErr != nil {
				t.Fatal(readErr)
			}
			writeFile(t, root, op.Path, append(data, op.Content...))
		default:
			t.Fatalf("unknown op %q", op.Op)
		}
	}
	return root
}

// sha256Hex is the SHA-256 of data in lowercase hex.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// snapshotTree reads every file below root, by its slash path.
func snapshotTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(file) //#nosec G122 -- a scratch tree the test wrote
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, file)
		files[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// changedOutputs are the files below npm/, pypi/ and nuget/ that differ from
// before.
func changedOutputs(before, after map[string][]byte) []string {
	var changed []string
	for rel, data := range after {
		if !strings.HasPrefix(rel, "npm/") && !strings.HasPrefix(rel, "pypi/") && !strings.HasPrefix(rel, "nuget/") {
			continue
		}
		if old, had := before[rel]; had && bytes.Equal(old, data) {
			continue
		}
		changed = append(changed, rel)
	}
	sort.Strings(changed)
	return changed
}

// expectedStdout is what a recorded run printed on stdout as the command
// prints it: paths relative to the working directory, and the command's name
// where the scripts named themselves.
func expectedStdout(recorded string) string {
	return strings.NewReplacer("<root>/", "", "build_pypi: ", "release_packages pypi: ", "build_nuget: ", "release_packages nuget: ").Replace(recorded)
}

// expectedStderr is what a recorded run printed on stderr as the command
// prints it: node's thrown message rather than its stack, the command's name
// where the scripts named themselves, and no em dash.
func expectedStderr(recorded string) string {
	var lines []string
	for line := range strings.SplitSeq(recorded, "\n") {
		switch {
		case strings.HasPrefix(line, "Error: "):
			lines = append(lines, toolName+": "+strings.TrimPrefix(line, "Error: "))
		case strings.HasPrefix(line, "build_pypi: "), strings.HasPrefix(line, "build_nuget: "):
			_, message, _ := strings.Cut(line, ": ")
			lines = append(lines, toolName+": "+message)
		case strings.HasPrefix(line, "WARNING: "):
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.ReplaceAll(strings.Join(lines, "\n")+"\n", " "+string(rune(0x2014))+" ", ": ")
}

// TestPackages_BuildersRecord_PacksWhatTheScriptsPacked verifies that every
// recorded run of the three builders is repeated: the same exit status, the
// same lines on stdout and stderr, and the same files, each package entry
// for entry. A refused run writes nothing, where the scripts left the
// packages they had written before they met the fault.
func TestPackages_BuildersRecord_PacksWhatTheScriptsPacked(t *testing.T) {
	t.Parallel()

	var record packagesRecord
	readRecord(t, "build_packages.json", &record)
	if len(record.Runs) < 40 {
		t.Fatalf("the record holds %d runs; it is not the one this test was written against", len(record.Runs))
	}
	for _, run := range record.Runs {
		t.Run(run.Name, func(t *testing.T) {
			t.Parallel()
			repeatPackagesRun(t, &record, run)
		})
	}
}

// repeatPackagesRun runs one recorded run again and holds it to the record.
func repeatPackagesRun(t *testing.T, record *packagesRecord, run packagesRun) {
	t.Helper()
	root := layOutPackagesTree(t, record, run.Ops)
	before := snapshotTree(t, root)
	var stdout, stderr bytes.Buffer
	code := runIn(root, append([]string{run.Tool}, run.Args...), &stdout, &stderr, noEnv)
	after := snapshotTree(t, root)

	wantExit, wantStderr := run.Exit, expectedStderr(run.Stderr)
	if difference, declared := packagesDifferences[run.Name]; declared {
		wantExit, wantStderr = difference.exit, difference.stderr
	}
	if code != wantExit {
		t.Errorf("exit = %d, want %d (stderr %q)", code, wantExit, stderr.String())
	}
	if stderr.String() != wantStderr {
		t.Errorf("stderr:\n got %q\nwant %q", stderr.String(), wantStderr)
	}
	changed := changedOutputs(before, after)
	if run.Exit != 0 {
		if stdout.Len() != 0 || len(changed) != 0 {
			t.Errorf("a refused run printed %q and left %q, want nothing", stdout.String(), changed)
		}
		return
	}
	if want := expectedStdout(run.Stdout); stdout.String() != want {
		t.Errorf("stdout:\n got %q\nwant %q", stdout.String(), want)
	}
	recorded := slices.Sorted(maps.Keys(run.Files))
	if !slices.Equal(changed, recorded) {
		t.Errorf("the run left %q, want %q", changed, recorded)
	}
	for _, rel := range recorded {
		if got, made := after[rel]; made {
			comparePackageOutput(t, root, rel, got, run.Files[rel])
		}
	}
}

// comparePackageOutput holds one file a run left to the recorded one.
func comparePackageOutput(t *testing.T, root, rel string, got []byte, want recordedOutput) {
	t.Helper()
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if mode := fmt.Sprintf("0o%o", info.Mode().Perm()); mode != want.Mode {
			t.Errorf("%s: mode %s, want %s", rel, mode, want.Mode)
		}
	}
	if want.Entries == nil {
		if sha256Hex(got) != want.SHA256 || len(got) != want.Size {
			t.Errorf("%s: %d bytes, sha256 %s, want %d and %s%s", rel, len(got), sha256Hex(got), want.Size, want.SHA256, textDiff(got, want.Text))
		}
		return
	}
	compareArchive(t, rel, got, want.Entries)
}

// compareArchive holds a wheel or a NuGet package to the recorded entries.
func compareArchive(t *testing.T, rel string, got []byte, entries []recordedEntry) {
	t.Helper()
	want := recordedOutput{Entries: entries}
	archive, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	var names, wantNames []string
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	for _, entry := range want.Entries {
		wantNames = append(wantNames, entry.Name)
	}
	if !slices.Equal(names, wantNames) {
		t.Fatalf("%s: entries %q, want %q", rel, names, wantNames)
	}
	var wheelLine string
	for index, file := range archive.File {
		entry := want.Entries[index]
		data := readEntry(t, file)
		attrs := fmt.Sprintf("mode 0o%o, system %d, date %#x time %#x, method %d, descriptor %t, extra %d bytes",
			file.ExternalAttrs>>16, file.CreatorVersion>>8, file.ModifiedDate, file.ModifiedTime, file.Method, file.Flags&0x8 != 0, len(file.Extra))
		wantAttrs := fmt.Sprintf("mode %s, system %d, date %#x time %#x, method %d, descriptor %t, extra %d bytes",
			entry.Mode, entry.CreateSystem, zipEpochDate, zipEpochTime, entry.CompressType, entry.DataDescriptor, len(entry.Extra))
		if entry.DateTime != "1980-01-01 00:00:00" {
			t.Errorf("%s: %s recorded at %s, which the epoch fields do not spell", rel, entry.Name, entry.DateTime)
		}
		if attrs != wantAttrs {
			t.Errorf("%s: %s: %s, want %s", rel, entry.Name, attrs, wantAttrs)
		}
		switch {
		case strings.HasSuffix(entry.Name, ".dist-info/WHEEL"):
			// The wheels name this command as their generator, where the
			// script named itself; the rest is the script's.
			sum := sha256.Sum256(data)
			wheelLine = fmt.Sprintf("%s,sha256=%s,%d", entry.Name, base64.RawURLEncoding.EncodeToString(sum[:]), len(data))
			data = bytes.Replace(data, []byte("Generator: gitlab-mcp-server release_packages"), []byte("Generator: gitlab-mcp-server build_pypi.py"), 1)
		case strings.HasSuffix(entry.Name, ".dist-info/RECORD"):
			// The script's RECORD with the WHEEL line as this command
			// writes it; every other line is held to the recorded entries.
			if record := expectedRecord(t, want.Entries, wheelLine); string(data) != record {
				t.Errorf("%s: RECORD:\n%s", rel, firstDifference(string(data), record))
			}
			continue
		}
		if sha256Hex(data) != entry.SHA256 || len(data) != entry.Size {
			t.Errorf("%s: %s is %d bytes, sha256 %s, want %d and %s%s", rel, entry.Name, len(data), sha256Hex(data), entry.Size, entry.SHA256, textDiff(data, entry.Text))
		}
	}
}

// expectedRecord rebuilds the RECORD the script wrote from the recorded
// entries, with the WHEEL line as this command writes it: if every other
// line matches, the RECORD this command wrote and the script's differ only
// by the generator.
func expectedRecord(t *testing.T, entries []recordedEntry, wheelLine string) string {
	t.Helper()
	var out strings.Builder
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name, "/RECORD") {
			fmt.Fprintf(&out, "%s,,\n", entry.Name)
			continue
		}
		if strings.HasSuffix(entry.Name, "/WHEEL") {
			out.WriteString(wheelLine + "\n")
			continue
		}
		sum, err := hex.DecodeString(entry.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&out, "%s,sha256=%s,%d\n", entry.Name, base64.RawURLEncoding.EncodeToString(sum), entry.Size)
	}
	return out.String()
}

// textDiff shows where a file departs from the recorded text, when the
// record kept it.
func textDiff(got []byte, want *string) string {
	if want == nil {
		return ""
	}
	return "\n" + firstDifference(string(got), *want)
}

// TestBuilders_RefuseACommandLineTheyCannotUse covers the flags the three
// builders share: a flag they do not know, an argument beside the flags, a
// missing version and, for PyPI and NuGet, missing release assets are usage
// errors that write nothing.
func TestBuilders_RefuseACommandLineTheyCannotUse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"npm", "--nope"}, want: "flag provided but not defined: -nope"},
		{args: []string{"pypi", "--version", "1.0.0", "--binaries", "dist", "extra"}, want: `release_packages pypi: unexpected argument "extra"`},
		{args: []string{"nuget", "--binaries", "dist"}, want: "release_packages nuget: --version <x.y.z> is required"},
		{args: []string{"pypi", "--version", "1.0.0"}, want: "release_packages pypi: --binaries <dir> is required"},
		{args: []string{"nuget", "--version", "1.0.0"}, want: "release_packages nuget: --binaries <dir> is required"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			var stdout, stderr bytes.Buffer
			if code := runIn(root, tc.args, &stdout, &stderr, noEnv); code != 2 || !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("exit %d, stderr %q, want 2 and %q", code, stderr.String(), tc.want)
			}
			if entries, _ := os.ReadDir(root); len(entries) != 0 {
				t.Errorf("a refused command line left %d files", len(entries))
			}
		})
	}
}

// TestBuilders_RefuseAChecksumFileTheyCannotRead covers a checksums.txt
// that is there and malformed: refused with the line named, where the
// scripts skipped any line they could not read.
func TestBuilders_RefuseAChecksumFileTheyCannotRead(t *testing.T) {
	t.Parallel()

	for _, tool := range []string{"npm", "pypi", "nuget"} {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()

			root := releaseTree(t)
			writeFile(t, root, "dist/checksums.txt", []byte("not a checksum line\n"))
			var stdout, stderr bytes.Buffer
			code := runIn(root, []string{tool, "--binaries", "dist", "--version", "9.8.7"}, &stdout, &stderr, noEnv)
			if code != 1 || !strings.Contains(stderr.String(), "is not a SHA-256 and a file name separated by two spaces") {
				t.Errorf("exit %d, stderr %q, want 1 and the line refused", code, stderr.String())
			}
		})
	}
}

// TestSyncNpmMain_RefusesALauncherItCannotRead covers --sync-only over a
// launcher package.json that is not there or not a JSON object.
func TestSyncNpmMain_RefusesALauncherItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, content string
	}{
		{name: "absent"},
		{name: "not an object", content: "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if tc.content != "" {
				writeFile(t, root, npmMainDir+"/package.json", []byte(tc.content))
			}
			var stdout, stderr bytes.Buffer
			if code := runIn(root, []string{"npm", "--sync-only", "--version", "9.8.7"}, &stdout, &stderr, noEnv); code != 1 {
				t.Errorf("exit %d, want 1 (stderr %q)", code, stderr.String())
			}
		})
	}
}
