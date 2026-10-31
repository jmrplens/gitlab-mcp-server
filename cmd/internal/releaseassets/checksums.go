// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Checksums is a release's checksum file: the digest of every asset it
// lists, by the asset's name.
type Checksums struct {
	// Name is the file's own name, which an error about an asset it lists
	// names.
	Name string
	// Digests is the SHA-256 of each asset, in lowercase hex, by the asset's
	// name.
	Digests map[string]string
}

// Digest is the SHA-256 of data in lowercase hex, which is how GoReleaser
// writes it.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// IsDigest reports whether s is a SHA-256 in lowercase hex.
func IsDigest(s string) bool {
	return len(s) == sha256.Size*2 && strings.Trim(s, "0123456789abcdef") == ""
}

// ReadChecksums reads the checksum file at path, as [ParseChecksums] reads
// it. A file that cannot be read returns the error reading it did.
func ReadChecksums(path string) (Checksums, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- a checksum file the operator named
	if err != nil {
		return Checksums{}, err
	}
	return ParseChecksums(path, data)
}

// ParseChecksums reads a checksum file as GoReleaser writes it: a digest, two
// spaces and the asset's name, one per line, the last ending in a newline. A
// line that is anything else, and a name listed twice, are refused with path
// and the line's number named, since either leaves nothing certain to
// compare against. The file's [Checksums.Name] is the base name of path.
func ParseChecksums(path string, data []byte) (Checksums, error) {
	sums := Checksums{Name: filepath.Base(path), Digests: map[string]string{}}
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		sum, name, found := strings.Cut(line, "  ")
		if !found || !IsDigest(sum) || name == "" {
			return Checksums{}, fmt.Errorf("%s:%d: %q is not a SHA-256 and a file name separated by two spaces", path, i+1, line)
		}
		if _, listed := sums.Digests[name]; listed {
			return Checksums{}, fmt.Errorf("%s:%d: %s is listed twice", path, i+1, name)
		}
		sums.Digests[name] = sum
	}
	return sums, nil
}
