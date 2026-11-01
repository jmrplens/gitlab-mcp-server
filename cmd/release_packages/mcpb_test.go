// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// mcpb_test.go holds the mcpb subcommand to what scripts/build-mcpb.sh did.
//
// testdata/build_mcpb.json is the record. Each run laid out a scratch tree
// the way scripts/build_mcpb_sh_test.py did, the repository root with mcpb/,
// LICENSE and a dist/ of per-target builds, every file holding stand-in
// bytes, and ran the script at fe8773534, the revision that removed it, with
// version 9.8.7, under jq 1.6, Info-ZIP's zip 3.0 and unzip 6.00. The runs
// are the suite's cases: both dist/ layouts, each refusal before packing,
// each manifest rule, an entry zip left out (through the suite's stand-in
// for zip) and every size limit and warning (through its stand-in for
// zipinfo, which reports figures no stand-in could reach). For each run the
// record keeps the exit status, stdout up to the last line the script wrote
// itself, stderr, the job summary, which bundles were left, and, for the runs
// that pack something no other run does, every entry of every bundle: name,
// order, external attributes, creator system, time, extra field, size, CRC
// and the SHA-256 of its bytes, and the packed manifest's text.
//
// Each run is repeated here over the same tree, with the size reader and the
// archive writer replaced where the script's stand-ins replaced zipinfo and
// zip. The bundles are equal entry for entry to the script's, which is what
// F4-D6 asks of the port: only the deflated bytes differ, since Go's deflate
// is not Info-ZIP's, and so the archive's size, which every size line and
// summary row of an unfaked run prints, is compared with that figure masked.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// buildRecord is testdata/build_mcpb.json.
type buildRecord struct {
	RecordedFrom string     `json:"recorded_from"`
	Version      string     `json:"version"`
	MakeMcpbDist []string   `json:"make_mcpb_dist"`
	ReleaseDist  []string   `json:"release_dist"`
	RepoFiles    []string   `json:"repo_files"`
	Runs         []buildRun `json:"runs"`
}

// buildRun is one recorded run of the script.
type buildRun struct {
	Name          string                  `json:"name"`
	Manifest      string                  `json:"manifest"`
	Dist          string                  `json:"dist"`
	Break         []treeOp                `json:"break"`
	DropEntry     string                  `json:"drop_entry"`
	Sizes         *fakeSizes              `json:"sizes"`
	GitHubActions bool                    `json:"github_actions"`
	Exit          int                     `json:"exit"`
	Stdout        string                  `json:"stdout"`
	Stderr        string                  `json:"stderr"`
	Summary       *string                 `json:"summary"`
	BundlesLeft   []string                `json:"bundles_left"`
	Bundles       map[string]bundleRecord `json:"bundles"`
}

// treeOp is one change a run made to its tree after a first run left
// bundles in it.
type treeOp struct {
	Op      string `json:"op"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// fakeSizes is what the suite's stand-in for zipinfo reported in place of
// what it read: the archive's size, every entry's size, one entry's, and
// more entries than the archive holds, each of size 1.
type fakeSizes struct {
	ArchiveBytes    *int64 `json:"FAKE_ARCHIVE_BYTES"`
	EveryEntryBytes *int64 `json:"FAKE_EVERY_ENTRY_BYTES"`
	Entry           string `json:"FAKE_ENTRY"`
	EntryBytes      *int64 `json:"FAKE_ENTRY_BYTES"`
	ExtraEntries    int    `json:"FAKE_EXTRA_ENTRIES"`
}

// bundleRecord is one bundle as the script packed it.
type bundleRecord struct {
	ArchiveBytes int64         `json:"archive_bytes"`
	Entries      []entryRecord `json:"entries"`
	Manifest     *string       `json:"manifest"`
}

// entryRecord is one entry of a bundle as Python's zipfile read it.
type entryRecord struct {
	Name         string `json:"name"`
	ExternalAttr uint32 `json:"external_attr"`
	CreateSystem uint16 `json:"create_system"`
	DateTime     string `json:"date_time"`
	Extra        string `json:"extra"`
	Size         uint64 `json:"size"`
	CRC          uint32 `json:"crc"`
	SHA256       string `json:"sha256"`
}

// buildDifferences are the recorded runs the command is not held to
// repeating, each with what it does instead and why.
var buildDifferences = map[string]string{
	// The derivation is in the command, so there is no file to miss.
	"a missing derivation": "builds",
	// jq's own parse error, and its exit status 4, are jq's.
	"a manifest that is not JSON": "refuses: mcpb/manifest.json could not be read as JSON",
}

// distStandIn is the bytes a scratch dist/ holds at rel, different for every
// file so a bundle packed from the wrong one is told apart; the notices open
// with the generator's header, which the build checks.
func distStandIn(rel string) []byte {
	body := "stand-in for dist/" + rel + strings.Repeat("\n", 64)
	if rel == noticesName {
		return []byte(noticesHeader + "\n" + body)
	}
	return []byte(body)
}

// writeFile writes data at rel below root, creating its directories.
func writeFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil { //#nosec G703 -- a scratch file below the test's own directory
		t.Fatal(err)
	}
}

// layOutTree builds the scratch tree a recorded run started from.
func layOutTree(t *testing.T, record *buildRecord, run buildRun) string {
	t.Helper()
	root := t.TempDir()
	manifest := run.Manifest
	if manifest == "" {
		manifest = "committed"
	}
	data, err := os.ReadFile(filepath.Join("testdata", "manifests", manifest+".json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, manifestPath, data)
	for _, rel := range record.RepoFiles {
		writeFile(t, root, rel, []byte("stand-in for "+rel+"\n"))
	}
	dist := record.MakeMcpbDist
	if run.Dist == "release" {
		dist = record.ReleaseDist
	}
	for _, rel := range dist {
		writeFile(t, root, "dist/"+rel, distStandIn(rel))
	}
	return root
}

// breakTree applies the changes a run made after its first run.
func breakTree(t *testing.T, root string, ops []treeOp) {
	t.Helper()
	for _, op := range ops {
		switch op.Op {
		case "remove":
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(op.Path))); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		case "write":
			writeFile(t, root, op.Path, []byte(op.Content))
		case "add_dist":
			writeFile(t, root, "dist/"+op.Path, distStandIn(op.Path))
		default:
			t.Fatalf("unknown tree change %q", op.Op)
		}
	}
}

// buildOutput is what one run of the command wrote.
type buildOutput struct {
	code           int
	stdout, stderr string
	summary        *string
}

// runBuild runs the command over root as the recorded run ran the script.
func runBuild(t *testing.T, root, version string, run buildRun) buildOutput {
	t.Helper()
	env := map[string]string{}
	summaryPath := filepath.Join(root, "summary.md")
	if run.Summary != nil {
		env["GITHUB_STEP_SUMMARY"] = summaryPath
	}
	if run.GitHubActions {
		env["GITHUB_ACTIONS"] = "true"
	}
	var stdout, stderr bytes.Buffer
	b := newMcpbBuild(root, "dist", version, &stdout, &stderr, func(key string) string { return env[key] })
	if run.DropEntry != "" {
		b.writeArchive = func(path string, files []bundleFile) error {
			return writeBundle(path, slices.DeleteFunc(files, func(f bundleFile) bool { return f.name == run.DropEntry }))
		}
	}
	if run.Sizes != nil {
		b.measure = func(path string) (bundleSizes, error) {
			sizes, err := measureBundle(path)
			return run.Sizes.apply(sizes), err
		}
	}
	out := buildOutput{code: b.run(), stdout: stdout.String(), stderr: stderr.String()}
	if data, err := os.ReadFile(summaryPath); err == nil {
		text := string(data)
		out.summary = &text
	}
	return out
}

// apply reports the sizes the stand-in for zipinfo reported for an archive
// it was handed: every entry's figure replaced first, then the named entry's,
// then the extra entries added, and the archive's size replaced.
func (f *fakeSizes) apply(sizes bundleSizes) bundleSizes {
	for i := range sizes.entries {
		if f.EveryEntryBytes != nil {
			sizes.entries[i].size = *f.EveryEntryBytes
		}
		if f.Entry != "" && sizes.entries[i].name == f.Entry && f.EntryBytes != nil {
			sizes.entries[i].size = *f.EntryBytes
		}
	}
	for i := range f.ExtraEntries {
		sizes.entries = append(sizes.entries, entrySize{name: "extra" + strconv.Itoa(i), size: 1})
	}
	if f.ArchiveBytes != nil {
		sizes.archive = *f.ArchiveBytes
	}
	return sizes
}

// The lines of each stream the comparison reads: what the script wrote of
// its own, and not zipinfo's listings, diff's hunk headers or the order find
// happened to list two files in.
var (
	sizeLine     = regexp.MustCompile(`^gitlab-mcp-server[^ ]*\.mcpb: `)
	jqError      = regexp.MustCompile(`^jq: error \(at [^)]*\): (.*)$`)
	downloadSize = regexp.MustCompile(`: [0-9]+\.[0-9]{2} MiB to download|ratio [0-9]+\.[0-9]:1|^(\| ` + "`" + `[^` + "`" + `]+` + "`" + ` \| )[0-9]+\.[0-9]{2} MiB`)
)

// ownStderr is the lines of stderr a run is compared on, with jq's error
// spelled as the command's: the message after the bundle it was packing.
func ownStderr(stderr string, fromScript bool) (lines, listed []string) {
	for line := range strings.Lines(stderr) {
		line = strings.TrimSuffix(line, "\n")
		switch {
		case strings.HasPrefix(line, "ERROR: "), strings.HasPrefix(line, "WARNING: "),
			strings.HasPrefix(line, "< "), strings.HasPrefix(line, "> "):
			lines = append(lines, line)
		case strings.HasPrefix(line, "dist/"):
			listed = append(listed, line)
		case fromScript && jqError.MatchString(line):
			lines = append(lines, "jq: "+jqError.FindStringSubmatch(line)[1])
		}
	}
	sort.Strings(listed)
	return lines, listed
}

// ownStdout is the lines of stdout a run is compared on, with the
// download figure and the ratio masked when the run did not fake the
// archive's size.
func ownStdout(stdout string, masked bool) []string {
	var lines []string
	for line := range strings.Lines(stdout) {
		line = strings.TrimSuffix(line, "\n")
		if sizeLine.MatchString(line) || strings.HasPrefix(line, "::warning::") || strings.HasPrefix(line, "Built ") {
			lines = append(lines, mask(line, masked))
		}
	}
	return lines
}

// mask replaces the figures that depend on the deflated size.
func mask(line string, masked bool) string {
	if !masked {
		return line
	}
	return downloadSize.ReplaceAllStringFunc(line, func(match string) string {
		switch {
		case strings.HasPrefix(match, ": "):
			return ": - MiB to download"
		case strings.HasPrefix(match, "ratio"):
			return "ratio -:1"
		}
		return match[:strings.LastIndex(match, "| ")+2] + "- MiB"
	})
}

// TestMcpbBuild_BuildMcpbRecord_SaysAndPacksWhatTheScriptDid verifies every
// recorded run: the exit status (any refusal is 1, where jq refused with 4
// or 5), the bundles left in dist/, every line the script wrote of its own
// on either stream, the job summary, and for the runs that keep them every
// entry of every bundle.
func TestMcpbBuild_BuildMcpbRecord_SaysAndPacksWhatTheScriptDid(t *testing.T) {
	t.Parallel()

	var record buildRecord
	readRecord(t, "build_mcpb.json", &record)
	if len(record.Runs) == 0 {
		t.Fatal("the record holds no run")
	}
	for _, run := range record.Runs {
		t.Run(run.Name, func(t *testing.T) {
			t.Parallel()

			root := layOutTree(t, &record, run)
			if run.Break != nil {
				if first := runBuild(t, root, record.Version, buildRun{}); first.code != 0 {
					t.Fatalf("the first run exited %d: %s", first.code, first.stderr)
				}
				for _, name := range []string{"gitlab-mcp-server-darwin.mcpb", "gitlab-mcp-server.mcpb"} {
					if !regularFile(filepath.Join(root, "dist", name)) {
						t.Fatalf("the first run left no %s", name)
					}
				}
				breakTree(t, root, run.Break)
			}
			got := runBuild(t, root, record.Version, run)
			if difference, ok := buildDifferences[run.Name]; ok {
				assertDifference(t, root, got, difference)
				return
			}
			assertRun(t, root, run, got)
		})
	}
}

// assertRun holds one run of the command to the script's run.
func assertRun(t *testing.T, root string, run buildRun, got buildOutput) {
	t.Helper()
	wantCode := 0
	if run.Exit != 0 {
		wantCode = 1
	}
	if got.code != wantCode {
		t.Errorf("exit %d, want %d (the script exited %d)\nstderr:\n%s", got.code, wantCode, run.Exit, got.stderr)
	}
	if left := bundlesLeft(root); !slices.Equal(left, run.BundlesLeft) {
		t.Errorf("bundles left %q, want %q", left, run.BundlesLeft)
	}
	wantLines, wantListed := ownStderr(run.Stderr, true)
	gotLines, gotListed := ownStderr(got.stderr, false)
	assertStderr(t, gotLines, wantLines)
	if !slices.Equal(gotListed, wantListed) {
		t.Errorf("files listed %q, want %q", gotListed, wantListed)
	}
	masked := run.Sizes == nil || run.Sizes.ArchiveBytes == nil
	if gotOut, wantOut := ownStdout(got.stdout, masked), ownStdout(run.Stdout, masked); !slices.Equal(gotOut, wantOut) {
		t.Errorf("stdout lines\n%s\nwant\n%s", strings.Join(gotOut, "\n"), strings.Join(wantOut, "\n"))
	}
	assertSummary(t, got.summary, run.Summary, masked)
	for name, want := range run.Bundles {
		assertBundle(t, filepath.Join(root, "dist", name), want)
	}
}

// assertDifference holds a run the command is not held to repeating to what
// it does instead.
func assertDifference(t *testing.T, root string, got buildOutput, difference string) {
	t.Helper()
	if difference == "builds" {
		if got.code != 0 || len(bundlesLeft(root)) != 4 {
			t.Errorf("exit %d with %q left, want the four bundles built\nstderr:\n%s", got.code, bundlesLeft(root), got.stderr)
		}
		return
	}
	message := strings.TrimPrefix(difference, "refuses: ")
	if got.code != 1 || len(bundlesLeft(root)) != 0 || !strings.Contains(got.stderr, message) {
		t.Errorf("exit %d with %q left and stderr %q, want 1, nothing left and %q", got.code, bundlesLeft(root), got.stderr, message)
	}
}

// assertStderr compares the stderr lines, a jq error matching the line of
// the command that ends with its message.
func assertStderr(t *testing.T, got, want []string) {
	t.Helper()
	ok := len(got) == len(want)
	for i := 0; ok && i < len(want); i++ {
		if message, isJQ := strings.CutPrefix(want[i], "jq: "); isJQ {
			ok = strings.HasPrefix(got[i], "ERROR: ") && strings.HasSuffix(got[i], ": "+message)
		} else {
			ok = got[i] == want[i]
		}
	}
	if !ok {
		t.Errorf("stderr lines\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// assertSummary compares the job summaries.
func assertSummary(t *testing.T, got, want *string, masked bool) {
	t.Helper()
	switch {
	case want == nil && got == nil:
		return
	case want == nil || got == nil:
		t.Errorf("job summary %v, want %v", got != nil, want != nil)
		return
	}
	var gotLines, wantLines []string
	for line := range strings.Lines(*got) {
		gotLines = append(gotLines, mask(line, masked))
	}
	for line := range strings.Lines(*want) {
		wantLines = append(wantLines, mask(line, masked))
	}
	if !slices.Equal(gotLines, wantLines) {
		t.Errorf("job summary\n%s\nwant\n%s", strings.Join(gotLines, ""), strings.Join(wantLines, ""))
	}
}

// bundlesLeft lists the bundles a run left in dist/, in the order the run
// builds them.
func bundlesLeft(root string) []string {
	var left []string
	for _, target := range bundleTargets() {
		if regularFile(filepath.Join(root, "dist", target.name)) {
			left = append(left, target.name)
		}
	}
	return left
}

// assertBundle holds one bundle to the script's, entry for entry.
func assertBundle(t *testing.T, path string, want bundleRecord) {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer archive.Close()
	if len(archive.File) != len(want.Entries) {
		t.Errorf("%s holds %d entries, want %d", path, len(archive.File), len(want.Entries))
		return
	}
	for i, file := range archive.File {
		entry := want.Entries[i]
		data := readEntry(t, file)
		digest := sha256.Sum256(data)
		got := entryRecord{
			Name:         file.Name,
			ExternalAttr: file.ExternalAttrs,
			CreateSystem: file.CreatorVersion >> 8,
			DateTime:     file.Modified.Format("2006-01-02 15:04:05"),
			Extra:        hex.EncodeToString(file.Extra),
			Size:         file.UncompressedSize64,
			CRC:          file.CRC32,
			SHA256:       hex.EncodeToString(digest[:]),
		}
		if got != entry {
			t.Errorf("%s entry %d is %+v, want %+v", filepath.Base(path), i, got, entry)
		}
		if file.Name == "manifest.json" && (want.Manifest == nil || string(data) != *want.Manifest) {
			t.Errorf("%s manifest differs from the script's", filepath.Base(path))
		}
	}
}

// readEntry reads an archive entry whole.
func readEntry(t *testing.T, file *zip.File) []byte {
	t.Helper()
	reader, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestMcpbBuild_TheSameInputs_PackTheSameBytes verifies the half of F4-D6
// the script could not be held to, since Info-ZIP's bytes are not Go's: two
// runs over the same inputs write identical bundles, whether into two trees
// or twice into one, so the SHA-256 server.json declares for a bundle is a
// function of what it packs and of nothing else.
func TestMcpbBuild_TheSameInputs_PackTheSameBytes(t *testing.T) {
	t.Parallel()

	var record buildRecord
	readRecord(t, "build_mcpb.json", &record)
	build := func(root string) map[string][]byte {
		if out := runBuild(t, root, record.Version, buildRun{}); out.code != 0 {
			t.Fatalf("exit %d: %s", out.code, out.stderr)
		}
		bundles := map[string][]byte{}
		for _, name := range bundlesLeft(root) {
			data, err := os.ReadFile(filepath.Join(root, "dist", name))
			if err != nil {
				t.Fatal(err)
			}
			bundles[name] = data
		}
		return bundles
	}
	tree := layOutTree(t, &record, buildRun{})
	first := build(tree)
	if len(first) != 4 {
		t.Fatalf("built %d bundles, want 4", len(first))
	}
	for name, runs := range map[string]map[string][]byte{
		"a second run over the same tree":            build(tree),
		"a run over another tree of the same inputs": build(layOutTree(t, &record, buildRun{})),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for bundle, data := range first {
				if !bytes.Equal(data, runs[bundle]) {
					t.Errorf("%s differs", bundle)
				}
			}
		})
	}
}
