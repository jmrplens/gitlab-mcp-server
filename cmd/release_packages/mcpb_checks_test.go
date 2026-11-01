// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// mcpb_checks_test.go covers the checks a packed bundle is read back with on
// the shapes the record cannot reach: an archive that is not one, an entry
// recorded without its execute bit, a manifest that cannot be read, and
// manifests of kinds the jq program the checks replace refused to read.
package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// checkedBuild returns a run over an empty tree whose dist/ holds the
// archive bytes as the bundle of target.
func checkedBuild(t *testing.T, target bundleTarget, archive []byte) (*mcpbBuild, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "dist/"+target.name, archive)
	var stdout, stderr bytes.Buffer
	return newMcpbBuild(root, "dist", "9.8.7", &stdout, &stderr, noEnv), &stderr
}

// buildZip returns the archive add writes.
func buildZip(t *testing.T, add func(*zip.Writer) error) []byte {
	t.Helper()
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	if err := errors.Join(add(archive), archive.Close()); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// put writes data to an entry's writer, or returns the error that kept the
// entry from being created.
func put(w io.Writer, created error, data string) error {
	if created != nil {
		return created
	}
	_, err := io.WriteString(w, data)
	return err
}

// zipOf writes entries into an archive with the given header changes, the
// manifest one that passes every manifest rule for the darwin bundle.
func zipOf(t *testing.T, entries []string, header func(*zip.FileHeader)) []byte {
	t.Helper()
	return buildZip(t, func(archive *zip.Writer) error {
		for _, name := range entries {
			h := &zip.FileHeader{Name: name, Method: zip.Deflate}
			h.SetMode(0o755)
			header(h)
			data := "x"
			if name == "manifest.json" {
				data = `{"version":"9.8.7","server":{"entry_point":"server/gitlab-mcp-server","mcp_config":{"command":"${__dirname}/server/gitlab-mcp-server"}},"compatibility":{"platforms":["darwin"]},"icon":"icon.png"}`
			}
			w, err := archive.CreateHeader(h)
			if err = put(w, err, data); err != nil {
				return err
			}
		}
		return nil
	})
}

// TestCheckBundle_WhatTheArchiveRecords_IsReadBack covers the read-back
// checks: an archive that does not open, an executable recorded with
// another mode or system, and a manifest entry that cannot be read.
func TestCheckBundle_WhatTheArchiveRecords_IsReadBack(t *testing.T) {
	t.Parallel()

	darwin := bundleTargets()[0]
	for _, tc := range []struct {
		name    string
		archive func(t *testing.T) []byte
		want    []string
	}{
		{
			name:    "an archive that is not one",
			archive: func(*testing.T) []byte { return []byte("not a zip") },
			want:    []string{"ERROR: dist/gitlab-mcp-server-darwin.mcpb: the archive could not be read: zip: not a valid zip file"},
		},
		{
			name: "an executable without its execute bit",
			archive: func(t *testing.T) []byte {
				t.Helper()
				return zipOf(t, darwin.entries, func(h *zip.FileHeader) { h.SetMode(0o644) })
			},
			want: []string{"ERROR: dist/gitlab-mcp-server-darwin.mcpb: server/gitlab-mcp-server is recorded as '-rw-r--r-- unx', expected '-rwxr-xr-x unx'"},
		},
		{
			name: "an executable recorded on another system",
			archive: func(t *testing.T) []byte {
				t.Helper()
				return zipOf(t, darwin.entries, func(h *zip.FileHeader) { h.CreatorVersion = 0<<8 | 20 })
			},
			want: []string{"ERROR: dist/gitlab-mcp-server-darwin.mcpb: server/gitlab-mcp-server is recorded as '-rw-rw-rw- fat', expected '-rwxr-xr-x unx'"},
		},
		{
			name: "an executable recorded on a system zipinfo has no name for",
			archive: func(t *testing.T) []byte {
				t.Helper()
				return zipOf(t, darwin.entries, func(h *zip.FileHeader) { h.CreatorVersion = 99<<8 | 20 })
			},
			want: []string{"ERROR: dist/gitlab-mcp-server-darwin.mcpb: server/gitlab-mcp-server is recorded as '---------- system 99', expected '-rwxr-xr-x unx'"},
		},
		{
			name:    "a manifest whose bytes are not what the archive says",
			archive: corruptManifest,
			want:    []string{"ERROR: dist/gitlab-mcp-server-darwin.mcpb: the packed manifest.json could not be read and checked"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, stderr := checkedBuild(t, darwin, tc.archive(t))
			b.checkBundle(darwin)
			if got := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n"); !slices.Equal(got, tc.want) {
				t.Errorf("stderr\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
			if b.failures != len(tc.want) {
				t.Errorf("failures = %d, want %d", b.failures, len(tc.want))
			}
		})
	}
}

// corruptManifest writes the darwin bundle's entries with a manifest whose
// recorded checksum is not its bytes', which reading it whole refuses.
func corruptManifest(t *testing.T) []byte {
	t.Helper()
	return buildZip(t, func(archive *zip.Writer) error {
		for _, name := range bundleTargets()[0].entries {
			h := &zip.FileHeader{Name: name, Method: zip.Store, CRC32: crc32.ChecksumIEEE([]byte("x")), CompressedSize64: 1, UncompressedSize64: 1}
			h.SetMode(0o755)
			if name == "manifest.json" {
				h.CRC32++
			}
			w, err := archive.CreateRaw(h)
			if err = put(w, err, "x"); err != nil {
				return err
			}
		}
		return nil
	})
}

// TestReadManifest_AnEntryThatCannotBeOpened_IsRefused covers a manifest
// stored with a compression method no reader knows.
func TestReadManifest_AnEntryThatCannotBeOpened_IsRefused(t *testing.T) {
	t.Parallel()

	data := buildZip(t, func(archive *zip.Writer) error {
		w, err := archive.CreateRaw(&zip.FileHeader{Name: "manifest.json", Method: 99, CompressedSize64: 2, UncompressedSize64: 2})
		return put(w, err, "{}")
	})
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if _, readErr := readManifest(reader.File[0]); readErr == nil {
		t.Error("readManifest read an entry of an unknown method")
	}
}

// TestWriteEntryDiff_EntriesInAnotherOrder_AreShownInBothOrders covers the
// one difference the line-by-line listing cannot show: the same entries in
// another order.
func TestWriteEntryDiff_EntriesInAnotherOrder_AreShownInBothOrders(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writeEntryDiff(&out, []string{"a", "b", "c"}, []string{"a", "c", "d"})
	writeEntryDiff(&out, []string{"a", "b"}, []string{"b", "a"})
	want := "< b\n> d\n< a b\n> b a\n"
	if out.String() != want {
		t.Errorf("writeEntryDiff wrote %q, want %q", out.String(), want)
	}
}

// TestManifestProblems_ShapesNoBuildPacks_AreJudgedAsJQJudgedThem covers the
// manifest rules on shapes no committed manifest has: a version or an entry
// point of another kind, a platform listed that is not a string, and the
// kinds the jq program refused to read at all.
func TestManifestProblems_ShapesNoBuildPacks_AreJudgedAsJQJudgedThem(t *testing.T) {
	t.Parallel()

	universal := bundleTargets()[3]
	for _, tc := range []struct {
		name, manifest string
		want           []string
		refused        string
	}{
		{
			name:     "a version and an entry point that are not strings",
			manifest: `{"version":null,"server":{"entry_point":7,"mcp_config":{"command":"${__dirname}/server/gitlab-mcp-server","platform_overrides":{"win32":{"command":"${__dirname}/server/gitlab-mcp-server.exe"},"linux":{"command":"/bin/sh","args":["${__dirname}/server/linux/launch.sh"]}}}},"compatibility":{"platforms":["darwin","win32","linux",7]}}`,
			want: []string{
				"manifest version is null, expected 9.8.7",
				"server.entry_point 7 is not in the archive",
				"compatibility.platforms lists 7 with no platform_overrides entry, so it would start the base command, which serves darwin",
				"compatibility.platforms lists 7, although the archive carries no server for it",
			},
		},
		{
			name:     "base args held as an object",
			manifest: `{"version":"9.8.7","server":{"entry_point":"server/gitlab-mcp-server","mcp_config":{"command":"${__dirname}/server/gitlab-mcp-server","args":{"a":"-v"},"platform_overrides":{"win32":{"command":"${__dirname}/server/gitlab-mcp-server.exe","args":[]},"linux":{"command":"/bin/sh","args":["${__dirname}/server/linux/launch.sh"]}}}},"compatibility":{"platforms":["darwin","win32","linux"]}}`,
			want: []string{
				"platform_overrides.win32 declares args, which would replace the non-empty base args",
				"platform_overrides.linux declares args, which would replace the non-empty base args",
			},
		},
		{
			name:     "a path named inside an argument",
			manifest: `{"version":"9.8.7","server":{"entry_point":"server/gitlab-mcp-server","mcp_config":{"command":"${__dirname}/server/gitlab-mcp-server","args":[],"platform_overrides":{"win32":{"command":"${__dirname}/server/gitlab-mcp-server.exe"},"linux":{"command":"/bin/sh","args":["--config=${__dirname}/server/linux/launch.sh"]}}}},"compatibility":{"platforms":["darwin","win32","linux"]}}`,
			want: []string{
				"--config=${__dirname}/server/linux/launch.sh uses ${__dirname} other than as a leading path component, so it cannot be checked",
			},
		},
		{name: "a config that is a string", manifest: `{"server":{"mcp_config":"x"}}`, refused: `cannot index string with "platform_overrides"`},
		{name: "a server that is a string", manifest: `{"server":"x"}`, refused: `cannot index string with "mcp_config"`},
		{name: "compatibility that is a string", manifest: `{"compatibility":"x"}`, refused: `cannot index string with "platforms"`},
		{name: "overrides that are a list", manifest: `{"server":{"mcp_config":{"platform_overrides":[1]}}}`, refused: "platform_overrides is array and compatibility.platforms array"},
		{name: "platforms that are a string", manifest: `{"compatibility":{"platforms":"darwin"}}`, refused: "platform_overrides is object and compatibility.platforms string"},
		{name: "an override that is null", manifest: `{"server":{"mcp_config":{"platform_overrides":{"linux":null}}}}`, refused: "platform_overrides.linux is null"},
		{name: "base args that are a string", manifest: `{"server":{"mcp_config":{"args":"-v"}}}`, refused: "cannot iterate over string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := parseJSON([]byte(tc.manifest))
			if err != nil {
				t.Fatal(err)
			}
			got, err := manifestProblems(manifest, "9.8.7", universal)
			if tc.refused != "" {
				if err == nil || err.Error() != tc.refused {
					t.Errorf("manifestProblems error = %v, want %q", err, tc.refused)
				}
				return
			}
			if err != nil {
				t.Fatalf("manifestProblems: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("problems\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}

// TestMcpbBuild_ABundleThatCannotBeWritten_RemovesTheSet covers a bundle the
// writer fails on: the run is refused with the writer's error, after the
// bundles before it were written, and none is left behind.
func TestMcpbBuild_ABundleThatCannotBeWritten_RemovesTheSet(t *testing.T) {
	t.Parallel()

	var record buildRecord
	readRecord(t, "build_mcpb.json", &record)
	root := layOutTree(t, &record, buildRun{})
	var stdout, stderr bytes.Buffer
	b := newMcpbBuild(root, "dist", record.Version, &stdout, &stderr, noEnv)
	b.writeArchive = func(path string, files []bundleFile) error {
		if filepath.Base(path) == "gitlab-mcp-server-linux.mcpb" {
			return os.ErrPermission
		}
		return writeBundle(path, files)
	}
	if code := b.run(); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if want := "ERROR: dist/gitlab-mcp-server-linux.mcpb: permission denied\n"; !strings.HasSuffix(stderr.String(), want) {
		t.Errorf("stderr %q, want it to end with %q", stderr.String(), want)
	}
	if left := bundlesLeft(root); len(left) != 0 {
		t.Errorf("bundles left %q, want none", left)
	}
}

// TestMcpbBuild_AManifestThatIsNotAnObject_IsRefused covers a committed
// manifest that is JSON but not an object, which no bundle could carry.
func TestMcpbBuild_AManifestThatIsNotAnObject_IsRefused(t *testing.T) {
	t.Parallel()

	var record buildRecord
	readRecord(t, "build_mcpb.json", &record)
	root := layOutTree(t, &record, buildRun{})
	writeFile(t, root, manifestPath, []byte("[]\n"))
	got := runBuild(t, root, record.Version, buildRun{})
	if want := "ERROR: mcpb/manifest.json could not be read as JSON: it holds array rather than an object\n"; got.code != 1 || got.stderr != want {
		t.Errorf("exit %d and stderr %q, want 1 and %q", got.code, got.stderr, want)
	}
}
