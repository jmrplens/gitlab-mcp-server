// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// checksums_test.go covers the published binaries read against the checksum
// file a release publishes. Reading the file itself is
// cmd/internal/releaseassets', and its cases are there.
package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// digestOfABC is the SHA-256 of "abc", from FIPS 180-2's own example.
const digestOfABC = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// TestReadPublished_HoldsTheFileToItsListedDigest covers a published asset:
// read only when the checksum file lists it, and only when it is the file the
// list says, since what the comparison measures is the published release and
// not whatever sits in the directory.
func TestReadPublished_HoldsTheFileToItsListedDigest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"good": "abc", "tampered": "abd"})
	sums := releaseassets.Checksums{Name: "checksums.txt", Digests: map[string]string{"good": digestOfABC, "tampered": digestOfABC, "missing": digestOfABC}}

	data, err := readPublished(sums, dir, "good")
	if err != nil || string(data) != "abc" {
		t.Errorf("readPublished(good) = %q, %v", data, err)
	}
	for _, tc := range []struct {
		asset, want string
	}{
		{asset: "unlisted", want: "checksums.txt lists no unlisted"},
		{asset: "missing", want: "missing"},
		{asset: "tampered", want: "the published tampered is not the file checksums.txt lists"},
	} {
		t.Run(tc.asset, func(t *testing.T) {
			t.Parallel()

			if _, refused := readPublished(sums, dir, tc.asset); refused == nil || !strings.Contains(refused.Error(), tc.want) {
				t.Errorf("readPublished(%s) = %v, want an error containing %q", tc.asset, refused, tc.want)
			}
		})
	}
}
