// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"strings"
)

const mib = 1024 * 1024

// Claude Desktop's own limits, read from its extension runtime (2.7032.0,
// the extraction policy it applies to an extension): it refuses an archive
// with more than 100,000 entries, any entry that unpacks past 512 MiB, more
// than 2048 MiB unpacked in all, and an archive that unpacks to more than 50
// times its own size. A bundle past any of them cannot be installed at all,
// so each one fails the build.
const (
	desktopMaxEntries       = 100000
	desktopMaxEntryBytes    = 512 * mib
	desktopMaxUnpackedBytes = 2048 * mib
	desktopMaxRatio         = 50
)

// Directories that score MCP servers stop reading a bundle past 50 MiB to
// download or 256 MiB unpacked (verifymcp.io's inspection limits), and then
// report its provenance, license and maintenance as unverified; 3.1.0's
// single bundle, at 73 MiB and 285 MiB, lost all three that way. Every user
// also downloads the whole file. Neither is a reason to refuse a release, so
// a declared bundle past either size is a warning. The universal bundle is
// not declared anywhere a directory reads, and carries all three systems'
// servers by design, so it is measured and not warned about.
const (
	directoryWarnDownloadBytes = 50 * mib
	directoryWarnUnpackedBytes = 256 * mib
)

// bundleSizes is what one bundle measures: the archive's size in bytes and
// each regular file it carries.
type bundleSizes struct {
	archive int64
	entries []entrySize
}

// entrySize is one regular file of an archive and its unpacked size.
type entrySize struct {
	name string
	size int64
}

// measureBundle reads the sizes of the archive at path.
func measureBundle(path string) (bundleSizes, error) {
	info, err := os.Stat(path) //#nosec G703 -- a bundle this command wrote under the dist directory the operator named
	if err != nil {
		return bundleSizes{}, err
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return bundleSizes{}, err
	}
	defer archive.Close()
	sizes := bundleSizes{archive: info.Size()}
	for _, file := range archive.File {
		if file.Mode().IsRegular() {
			// A bundle this command packed carries entries of a few tens of
			// megabytes, and a size past 2^63-1 could not be held in memory
			// to pack.
			sizes.entries = append(sizes.entries, entrySize{name: file.Name, size: int64(file.UncompressedSize64)}) //#nosec G115 -- see above
		}
	}
	return sizes, nil
}

// mibText is a byte count as MiB with two decimals, rounded half up.
func mibText(bytes int64) string {
	hundredths := (bytes*100 + mib/2) / mib
	return fmt.Sprintf("%d.%02d", hundredths/100, hundredths%100)
}

// checkSize measures one bundle, reports its sizes on stdout and in the row
// the job summary gets, and checks them against Claude Desktop's limits and,
// for a declared bundle, the sizes directories stop reading at.
func (b *mcpbBuild) checkSize(t bundleTarget) {
	output := b.output(t)
	sizes, err := b.measure(b.local(output))
	if err != nil || sizes.archive <= 0 {
		b.failf("%s: the bundle could not be measured", output)
		return
	}
	var unpacked, largest int64
	for _, entry := range sizes.entries {
		unpacked += entry.size
		largest = max(largest, entry.size)
	}
	count := len(sizes.entries)
	tenths := unpacked * 10 / sizes.archive

	fmt.Fprintf(b.stdout, "%s: %s MiB to download, %s MiB unpacked, %d entries, largest %s MiB, ratio %d.%d:1\n",
		t.name, mibText(sizes.archive), mibText(unpacked), count, mibText(largest), tenths/10, tenths%10)
	declared := "yes"
	if !t.declared {
		declared = "no, kept for existing links"
	}
	b.rows = append(b.rows, fmt.Sprintf("| `%s` | %s MiB | %s MiB | %s |", t.name, mibText(sizes.archive), mibText(unpacked), declared))

	if count > desktopMaxEntries {
		b.failf("%s: %d entries; Claude Desktop refuses an extension with more than %d", output, count, desktopMaxEntries)
	}
	if largest > desktopMaxEntryBytes {
		b.failf("%s: an entry unpacks to %d bytes; Claude Desktop refuses an entry over %d (512 MiB)", output, largest, desktopMaxEntryBytes)
	}
	if unpacked > desktopMaxUnpackedBytes {
		b.failf("%s: unpacks to %d bytes; Claude Desktop refuses more than %d (2048 MiB)", output, unpacked, desktopMaxUnpackedBytes)
	}
	if unpacked > desktopMaxRatio*sizes.archive {
		b.failf("%s: unpacks to more than %d times its %d bytes; Claude Desktop refuses it as a zip bomb", output, desktopMaxRatio, sizes.archive)
	}

	if !t.declared {
		return
	}
	// The two warnings keep the words scripts/build-mcpb.sh printed, British
	// spelling included, which the record holds them to.
	if sizes.archive > directoryWarnDownloadBytes {
		b.warnf("%s is %s MiB to download, over the 50 MiB past which directories stop reading a bundle and report its provenance, licence and maintenance as unverified", t.name, mibText(sizes.archive)) //nolint:misspell // the script's words, see above
	}
	if unpacked > directoryWarnUnpackedBytes {
		b.warnf("%s unpacks to %s MiB, over the 256 MiB past which directories stop reading a bundle and report its provenance, licence and maintenance as unverified", t.name, mibText(unpacked)) //nolint:misspell // the script's words, see above
	}
}

// list prints a bundle's entries, each with its mode and unpacked size.
func (b *mcpbBuild) list(t bundleTarget) {
	archive, err := zip.OpenReader(b.local(b.output(t)))
	if err != nil {
		return
	}
	defer archive.Close()
	fmt.Fprintf(b.stdout, "%s:\n", b.output(t))
	for _, file := range archive.File {
		fmt.Fprintf(b.stdout, "  %s %10d  %s\n", file.Mode(), file.UncompressedSize64, file.Name)
	}
}

// writeSummary appends the size table to the job summary, when GitHub
// Actions names one.
func (b *mcpbBuild) writeSummary() error {
	path := b.getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	var text strings.Builder
	fmt.Fprintf(&text, "### Claude Desktop extension bundles %s\n\n", b.version)
	text.WriteString("| Bundle | Download | Unpacked | Declared in server.json |\n")
	text.WriteString("| --- | ---: | ---: | --- |\n")
	for _, row := range b.rows {
		text.WriteString(row + "\n")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //#nosec G304,G703 -- the summary file GitHub Actions names for this step
	if err != nil {
		return err
	}
	_, err = file.WriteString(text.String())
	return errors.Join(err, file.Close())
}
