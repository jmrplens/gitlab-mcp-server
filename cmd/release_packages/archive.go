// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"compress/flate"
	"errors"
	"io"
	"io/fs"
	"os"
)

// bundleFile is one entry a bundle is packed with: its name in the archive,
// its mode, and its bytes, read from source unless data holds them.
type bundleFile struct {
	name   string
	mode   fs.FileMode
	source string
	data   []byte
}

// The time every entry is recorded with, as MS-DOS writes a date and a time:
// 1980-01-01 00:00:00, the zip epoch, which is what zip -X recorded from the
// files the script touched to it. server.json carries each declared bundle's
// SHA-256, and a hash that changes because the clock moved tells a verifier
// nothing.
const (
	zipEpochDate = 1<<5 | 1
	zipEpochTime = 0
)

// writeBundle writes a .mcpb, a plain zip with manifest.json at its root, to
// path. The same files make the same bytes: the entries are written in the
// order given, each recorded as a Unix file with its mode and the zip epoch
// as its time, deflated, with no extra field and no directory entry, which is
// what zip -X -D wrote. Only the deflated bytes differ from zip's, since Go's
// deflate is not Info-ZIP's.
func writeBundle(path string, files []bundleFile) error {
	out, err := os.Create(path) //#nosec G304,G703 -- a bundle under the dist directory the operator named
	if err != nil {
		return err
	}
	return errors.Join(writeZip(out, files), out.Close())
}

// writeZip writes the archive of files to out, deflated at the best level
// compress/flate has: archive/zip's own level 5 packed the 3.4.0 bundles
// about 5% larger than zip's default did, and every user downloads them
// whole.
func writeZip(out io.Writer, files []bundleFile) error {
	archive := zip.NewWriter(out)
	archive.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})
	for _, file := range files {
		if err := addEntry(archive, file); err != nil {
			return err
		}
	}
	return archive.Close()
}

// addEntry writes one entry. The MS-DOS time is set directly, because
// FileHeader.Modified would add an extended timestamp field zip -X never
// wrote.
func addEntry(archive *zip.Writer, file bundleFile) error {
	header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
	header.ModifiedDate = zipEpochDate //nolint:staticcheck // Modified would write an extra field (see addEntry).
	header.ModifiedTime = zipEpochTime //nolint:staticcheck // Modified would write an extra field (see addEntry).
	header.SetMode(file.mode)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	if file.data != nil {
		_, err = writer.Write(file.data)
		return err
	}
	source, err := os.Open(file.source)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, source)
	return errors.Join(err, source.Close())
}
