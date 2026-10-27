// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checksums is a release's checksum file: the digest of every asset it
// lists, by the asset's name.
type checksums struct {
	// name is the file's own name, which an error names.
	name    string
	digests map[string]string
}

// digest is the SHA-256 of data in lowercase hex, which is how GoReleaser
// writes it.
func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// isDigest reports whether s is a SHA-256 in lowercase hex.
func isDigest(s string) bool {
	return len(s) == sha256.Size*2 && strings.Trim(s, "0123456789abcdef") == ""
}

// readChecksums reads a checksum file as GoReleaser writes it: a digest, two
// spaces and the asset's name, one per line, the last ending in a newline. A
// line that is anything else, and a name listed twice, are refused with the
// line named, since either leaves nothing certain to compare against.
func readChecksums(path string) (checksums, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- the checksum file in the operator's own -published directory
	if err != nil {
		return checksums{}, err
	}
	sums := checksums{name: filepath.Base(path), digests: map[string]string{}}
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		sum, name, found := strings.Cut(line, "  ")
		if !found || !isDigest(sum) || name == "" {
			return checksums{}, fmt.Errorf("%s:%d: %q is not a SHA-256 and a file name separated by two spaces", path, i+1, line)
		}
		if _, listed := sums.digests[name]; listed {
			return checksums{}, fmt.Errorf("%s:%d: %s is listed twice", path, i+1, name)
		}
		sums.digests[name] = sum
	}
	return sums, nil
}

// readPublished reads the published asset from dir, and only when the
// checksum file lists it and it is the file the list says: what is compared
// is the release, not whatever a directory happens to hold.
func readPublished(sums checksums, dir, asset string) ([]byte, error) {
	want, listed := sums.digests[asset]
	if !listed {
		return nil, fmt.Errorf("%s lists no %s", sums.name, asset)
	}
	data, err := os.ReadFile(filepath.Join(dir, asset)) //#nosec G304 -- an asset the checksum file lists, in the operator's own -published directory
	if err != nil {
		return nil, err
	}
	if got := digest(data); got != want {
		return nil, fmt.Errorf("the published %s is not the file %s lists: its SHA-256 is %s, and the list says %s", asset, sums.name, got, want)
	}
	return data, nil
}
