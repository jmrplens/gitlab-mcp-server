// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// readPublished reads the published asset from dir, and only when the
// checksum file lists it and it is the file the list says: what is compared
// is the release, not whatever a directory happens to hold. The checksum file
// is read by releaseassets.ReadChecksums, which refuses one that leaves any
// asset's digest uncertain.
func readPublished(sums releaseassets.Checksums, dir, asset string) ([]byte, error) {
	want, listed := sums.Digests[asset]
	if !listed {
		return nil, fmt.Errorf("%s lists no %s", sums.Name, asset)
	}
	data, err := os.ReadFile(filepath.Join(dir, asset)) //#nosec G304 -- an asset the checksum file lists, in the operator's own -published directory
	if err != nil {
		return nil, err
	}
	if got := releaseassets.Digest(data); got != want {
		return nil, fmt.Errorf("the published %s is not the file %s lists: its SHA-256 is %s, and the list says %s", asset, sums.Name, got, want)
	}
	return data, nil
}
