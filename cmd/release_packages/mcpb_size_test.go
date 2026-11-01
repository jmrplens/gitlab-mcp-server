// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// mcpb_size_test.go covers what the record's faked figures cannot reach: a
// bundle that cannot be measured, the rounding of the MiB figures, the
// listing a successful run ends with, and a job summary that cannot be
// written.
package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestMibText_ByteCounts_RoundHalfUp verifies the MiB figure on each side of
// a rounding boundary, the way the script's integer arithmetic rounded it.
func TestMibText_ByteCounts_RoundHalfUp(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		bytes int64
		want  string
	}{
		{bytes: 0, want: "0.00"},
		{bytes: 5242, want: "0.00"},
		{bytes: 5243, want: "0.01"},
		{bytes: mib, want: "1.00"},
		{bytes: mib + 5242, want: "1.00"},
		{bytes: mib + 5243, want: "1.01"},
		{bytes: 2048 * mib, want: "2048.00"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			if got := mibText(tc.bytes); got != tc.want {
				t.Errorf("mibText(%d) = %s, want %s", tc.bytes, got, tc.want)
			}
		})
	}
}

// TestMeasureBundle_AnArchiveThatCannotBeRead_IsAnError covers the size
// reader on a file that is missing and on one that is not a zip.
func TestMeasureBundle_AnArchiveThatCannotBeRead_IsAnError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "not.mcpb", []byte("not a zip"))
	for _, path := range []string{filepath.Join(dir, "absent.mcpb"), filepath.Join(dir, "not.mcpb")} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			if _, err := measureBundle(path); err == nil {
				t.Errorf("measureBundle(%s) measured it", path)
			}
		})
	}
}

// TestCheckSize_ABundleThatCannotBeMeasured_FailsTheBundle covers the size
// check when the reader fails and when it reports an empty archive, which
// no ratio can be taken of.
func TestCheckSize_ABundleThatCannotBeMeasured_FailsTheBundle(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		measure func(string) (bundleSizes, error)
	}{
		{name: "an error", measure: func(string) (bundleSizes, error) { return bundleSizes{}, errors.New("gone") }},
		{name: "an empty archive", measure: func(string) (bundleSizes, error) { return bundleSizes{}, nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			b := newMcpbBuild(t.TempDir(), "dist", "9.8.7", &stdout, &stderr, noEnv)
			b.measure = tc.measure
			b.checkSize(bundleTargets()[0])
			if want := "ERROR: dist/gitlab-mcp-server-darwin.mcpb: the bundle could not be measured\n"; stderr.String() != want || b.failures != 1 || stdout.Len() != 0 {
				t.Errorf("stderr %q, failures %d, stdout %q, want %q, 1 and nothing", stderr.String(), b.failures, stdout.String(), want)
			}
		})
	}
}

// TestMcpbBuild_ASuccessfulRun_ListsEachBundle verifies the listing a run
// ends with, the line the script printed before zipinfo's listings and each
// entry with its mode and size, and that a bundle no longer there is not
// listed.
func TestMcpbBuild_ASuccessfulRun_ListsEachBundle(t *testing.T) {
	t.Parallel()

	var record buildRecord
	readRecord(t, "build_mcpb.json", &record)
	root := layOutTree(t, &record, buildRun{})
	got := runBuild(t, root, record.Version, buildRun{})
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"Built 4 bundles (version 9.8.7)\ndist/gitlab-mcp-server-darwin.mcpb:\n  -rw-r--r--       5689  manifest.json\n",
		"  -rwxr-xr-x        116  server/gitlab-mcp-server\ndist/gitlab-mcp-server-windows.mcpb:\n",
		"dist/gitlab-mcp-server.mcpb:\n",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout does not hold %q:\n%s", want, got.stdout)
		}
	}
	var stdout bytes.Buffer
	b := newMcpbBuild(t.TempDir(), "dist", "9.8.7", &stdout, &stdout, noEnv)
	b.list(bundleTargets()[0])
	if stdout.Len() != 0 {
		t.Errorf("a missing bundle was listed: %q", stdout.String())
	}
}

// TestMcpbBuild_ASummaryThatCannotBeWritten_FailsTheRun covers a job summary
// path the run cannot append to: the bundles are built, and the run says
// why it still failed.
func TestMcpbBuild_ASummaryThatCannotBeWritten_FailsTheRun(t *testing.T) {
	t.Parallel()

	var record buildRecord
	readRecord(t, "build_mcpb.json", &record)
	root := layOutTree(t, &record, buildRun{})
	var stdout, stderr bytes.Buffer
	b := newMcpbBuild(root, "dist", record.Version, &stdout, &stderr, func(key string) string {
		if key == "GITHUB_STEP_SUMMARY" {
			return root
		}
		return ""
	})
	if code := b.run(); code != 1 || !strings.HasPrefix(stderr.String(), "ERROR: the job summary could not be written: ") {
		t.Errorf("exit %d and stderr %q, want 1 and the summary named", code, stderr.String())
	}
}
