// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// archive_test.go covers the bundle writer: the header every entry gets, and
// each way writing an archive can fail, which the build reports for the
// bundle it was writing.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteZip_Entries_AreRecordedAsZipXDRecordedThem verifies the header of
// an entry: a Unix file with its mode, at the zip epoch, deflated, with no
// extra field, in the order given, its bytes read from data or from source.
func TestWriteZip_Entries_AreRecordedAsZipXDRecordedThem(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(source, []byte("from the file"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := writeZip(&out, []bundleFile{
		{name: "manifest.json", mode: 0o644, data: []byte("from data")},
		{name: "server/x", mode: 0o755, source: source},
	})
	if err != nil {
		t.Fatalf("writeZip: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []struct {
		name  string
		attrs uint32
		data  string
	}{
		{name: "manifest.json", attrs: 0o100644 << 16, data: "from data"},
		{name: "server/x", attrs: 0o100755 << 16, data: "from the file"},
	} {
		file := archive.File[i]
		if file.Name != want.name || file.ExternalAttrs != want.attrs || file.CreatorVersion>>8 != 3 ||
			file.Method != zip.Deflate || len(file.Extra) != 0 ||
			file.Modified.Format("2006-01-02 15:04:05") != "1980-01-01 00:00:00" {
			t.Errorf("entry %d is %+v", i, file.FileHeader)
		}
		if got := string(readEntry(t, file)); got != want.data {
			t.Errorf("entry %s holds %q, want %q", want.name, got, want.data)
		}
	}
}

// failingWriter accepts limit bytes and refuses the rest.
type failingWriter struct {
	limit int
}

// Write accepts what fits under the limit and fails past it.
func (w *failingWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		n := w.limit
		w.limit = 0
		return n, errors.New("the disk is full")
	}
	w.limit -= len(p)
	return len(p), nil
}

// incompressible is n bytes deflate cannot shrink, so writing them reaches
// the writer under the archive rather than staying in its buffers.
func incompressible(n int) []byte {
	out := make([]byte, 0, n+sha256.Size)
	block := sha256.Sum256([]byte("seed"))
	for len(out) < n {
		out = append(out, block[:]...)
		block = sha256.Sum256(block[:])
	}
	return out[:n]
}

// TestWriteZip_AWriteThatFails_IsReturned covers each place a write can fail.
// The archive writer buffers what it writes, so a write reaches the writer
// under it only once the buffer fills: a header longer than the buffer, an
// entry's bytes past it, and otherwise the end of the archive, which flushes
// what was buffered; and the copy from a source that cannot be opened or
// read fails on its own.
func TestWriteZip_AWriteThatFails_IsReturned(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, tc := range []struct {
		name  string
		limit int
		files []bundleFile
		want  string
	}{
		{name: "a header longer than the buffer", limit: 0, files: []bundleFile{{name: strings.Repeat("n", 8192), data: []byte("x")}}, want: "the disk is full"},
		{name: "the entry's bytes", limit: 64, files: []bundleFile{{name: "a", data: incompressible(1 << 16)}}, want: "the disk is full"},
		// The operating system words these two, so any error will do.
		{name: "a source that is missing", limit: 1 << 30, files: []bundleFile{{name: "a", source: filepath.Join(dir, "absent")}}},
		{name: "a source that is a directory", limit: 1 << 30, files: []bundleFile{{name: "a", source: dir}}},
		{name: "the end of the archive", limit: 0, files: []bundleFile{{name: "a", data: []byte("x")}}, want: "the disk is full"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := writeZip(&failingWriter{limit: tc.limit}, tc.files)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("writeZip error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

// TestWriteBundle_APathThatCannotBeCreated_IsReturned covers the bundle
// writer when its file cannot be created.
func TestWriteBundle_APathThatCannotBeCreated_IsReturned(t *testing.T) {
	t.Parallel()

	if err := writeBundle(filepath.Join(t.TempDir(), "absent", "x.mcpb"), nil); err == nil {
		t.Error("writeBundle wrote into a directory that does not exist")
	}
}
